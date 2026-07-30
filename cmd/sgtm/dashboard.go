package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/JuulLabs-OSS/cbgo"
	"github.com/nzoschke/sgtm/ui"
)

type dashboardConfig struct {
	Listen     string  `json:"listen"`
	IdealMax   float64 `json:"idealMax"`
	UnsafeMin  float64 `json:"unsafeMin"`
	ChartMin   float64 `json:"chartMin"`
	ChartMax   float64 `json:"chartMax"`
	HistorySec int     `json:"historySec"`
	DBPath     string  `json:"dbPath"`
}

type dashboardStore struct {
	mu          sync.Mutex
	config      dashboardConfig
	db          *readingDB
	readings    []soundReading
	latest      *soundReading
	status      string
	session     time.Time
	subscribers map[chan dashboardEvent]struct{}
}

type dashboardEvent struct {
	Type    string           `json:"type"`
	Status  string           `json:"status,omitempty"`
	Config  *dashboardConfig `json:"config,omitempty"`
	Reading *soundReading    `json:"reading,omitempty"`
	History []soundReading   `json:"history,omitempty"`
	Session *time.Time       `json:"session,omitempty"`
	Error   string           `json:"error,omitempty"`
}

func dashboardCmd(args []string) error {
	fs := flag.NewFlagSet("dashboard", flag.ExitOnError)
	addr := fs.String("addr", "", "CoreBluetooth device UUID from scan output")
	name := fs.String("name", "850019 EM", "case-insensitive local-name substring to discover and connect")
	listen := fs.String("listen", ":8080", "HTTP listen address")
	dbPath := fs.String("db", defaultDashboardDBPath(), "SQLite history database path")
	scanTimeout := fs.Duration("scan-timeout", 20*time.Second, "time to scan when resolving a name or address")
	idealMax := fs.Float64("ideal-max", 85, "top of green band, in dBA")
	unsafeMin := fs.Float64("unsafe-min", 95, "start of red band, in dBA")
	chartMin := fs.Float64("chart-min", 35, "chart lower bound")
	chartMax := fs.Float64("chart-max", 120, "chart upper bound")
	history := fs.Duration("history", 30*time.Minute, "history window to keep in memory and display")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *addr == "" && *name == "" {
		return fmt.Errorf("dashboard requires --addr or --name")
	}
	if err := os.MkdirAll(filepath.Dir(*dbPath), 0o755); err != nil {
		return err
	}
	db, err := openReadingDB(*dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	cfg := dashboardConfig{
		Listen:     *listen,
		IdealMax:   *idealMax,
		UnsafeMin:  *unsafeMin,
		ChartMin:   *chartMin,
		ChartMax:   *chartMax,
		HistorySec: int(history.Seconds()),
		DBPath:     *dbPath,
	}
	store := newDashboardStore(cfg, db)
	if err := store.loadRecent(context.Background(), *history, 5000); err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", dashboardPage)
	mux.HandleFunc("/events", store.events)
	mux.HandleFunc("/api/state", store.state)
	listenAddr := *listen
	if runningInAppBundle() && listenAddr == ":8080" {
		listenAddr = "127.0.0.1:0"
	}
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return err
	}
	url := dashboardURL(ln.Addr().String())
	log.Printf("dashboard listening on %s", url)
	go func() {
		if err := http.Serve(ln, mux); err != nil && err != http.ErrServerClosed {
			log.Printf("dashboard server: %v", err)
		}
	}()
	if runningInAppBundle() {
		go runDashboardBLE(context.Background(), *addr, *name, *scanTimeout, store)
		return runDashboardWebView(url)
	}
	openDashboard(url)
	runDashboardBLE(context.Background(), *addr, *name, *scanTimeout, store)
	return nil
}

func newDashboardStore(cfg dashboardConfig, db *readingDB) *dashboardStore {
	return &dashboardStore{
		config:      cfg,
		db:          db,
		status:      "starting",
		subscribers: make(map[chan dashboardEvent]struct{}),
	}
}

func defaultDashboardDBPath() string {
	if !runningInAppBundle() {
		return ".context/sgtm.sqlite"
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "sgtm.sqlite"
	}
	return filepath.Join(dir, "SGTM", "sgtm.sqlite")
}

func (s *dashboardStore) loadRecent(ctx context.Context, window time.Duration, limit int) error {
	readings, err := s.db.Recent(ctx, time.Now().Add(-window), limit)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.readings = readings
	if len(readings) > 0 {
		latest := readings[len(readings)-1]
		s.latest = &latest
	}
	return nil
}

func (s *dashboardStore) publish(reading soundReading) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	if err := s.db.Insert(ctx, reading); err != nil {
		log.Printf("insert reading: %v", err)
	}
	cancel()

	s.mu.Lock()
	cutoff := reading.Time.Add(-time.Duration(s.config.HistorySec) * time.Second)
	s.readings = append(s.readings, reading)
	keep := 0
	for keep < len(s.readings) && s.readings[keep].Time.Before(cutoff) {
		keep++
	}
	if keep > 0 {
		s.readings = append([]soundReading(nil), s.readings[keep:]...)
	}
	s.latest = &reading
	event := dashboardEvent{Type: "reading", Reading: &reading}
	s.broadcastLocked(event)
	s.mu.Unlock()
}

func (s *dashboardStore) setStatus(status string, err error) {
	event := dashboardEvent{Type: "status", Status: status}
	if err != nil {
		event.Error = err.Error()
	}
	s.mu.Lock()
	s.status = status
	s.broadcastLocked(event)
	s.mu.Unlock()
}

func (s *dashboardStore) setSessionStarted(t time.Time) {
	s.mu.Lock()
	s.session = t
	session := s.session
	s.broadcastLocked(dashboardEvent{Type: "session", Session: &session})
	s.mu.Unlock()
}

func (s *dashboardStore) broadcastLocked(event dashboardEvent) {
	for ch := range s.subscribers {
		select {
		case ch <- event:
		default:
		}
	}
}

func (s *dashboardStore) snapshot() dashboardEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	history := append([]soundReading(nil), s.readings...)
	var latest *soundReading
	if s.latest != nil {
		v := *s.latest
		latest = &v
	}
	cfg := s.config
	var session *time.Time
	if !s.session.IsZero() {
		v := s.session
		session = &v
	}
	return dashboardEvent{
		Type:    "snapshot",
		Status:  s.status,
		Config:  &cfg,
		Reading: latest,
		History: history,
		Session: session,
	}
}

func (s *dashboardStore) state(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.snapshot())
}

func (s *dashboardStore) events(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	ch := make(chan dashboardEvent, 32)
	s.mu.Lock()
	s.subscribers[ch] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.subscribers, ch)
		s.mu.Unlock()
		close(ch)
	}()

	writeSSE(w, s.snapshot())
	flusher.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case event := <-ch:
			writeSSE(w, event)
			flusher.Flush()
		}
	}
}

func writeSSE(w http.ResponseWriter, event dashboardEvent) {
	b, _ := json.Marshal(event)
	fmt.Fprintf(w, "data: %s\n\n", b)
}

func runDashboardBLE(ctx context.Context, addr, name string, scanTimeout time.Duration, store *dashboardStore) {
	for {
		if err := streamDT95Once(ctx, addr, name, scanTimeout, store); err != nil {
			store.setStatus("retrying", err)
			select {
			case <-time.After(3 * time.Second):
			case <-ctx.Done():
				return
			}
			continue
		}
		return
	}
}

func streamDT95Once(ctx context.Context, addr, name string, scanTimeout time.Duration, store *dashboardStore) error {
	store.setStatus("bluetooth", nil)
	c, err := newCentral()
	if err != nil {
		return err
	}
	target, err := resolveTarget(c, addr, name, scanTimeout)
	if err != nil {
		return err
	}
	store.setStatus("connecting", nil)
	prph, err := c.connect(target.Peripheral, 15*time.Second)
	if err != nil {
		return err
	}
	defer c.cm.CancelConnect(prph)
	prph.SetDelegate(c)

	var framer dt95Framer
	var framerMu sync.Mutex
	c.notifyHandler = func(_ cbgo.Characteristic, chunk []byte) {
		framerMu.Lock()
		readings, err := framer.Push(chunk)
		framerMu.Unlock()
		if err != nil {
			log.Printf("parse DT95: %v", err)
			return
		}
		for _, reading := range readings {
			store.publish(reading)
		}
	}

	writable, err := c.discoverDashboardCharacteristics(prph)
	if err != nil {
		return err
	}
	store.setStatus("live", nil)
	store.setSessionStarted(time.Now())
	start := []byte{0xd5, 0xfc, 0x11, 0x0d}
	log.Printf("writing %s to %s", hex.EncodeToString(start), writable.UUID())
	if err := c.writeCharacteristic(prph, *writable, start, 10*time.Second); err != nil {
		return err
	}
	<-ctx.Done()
	return ctx.Err()
}

func resolveTarget(c *central, addr, name string, scanTimeout time.Duration) (seenDevice, error) {
	addrLower := strings.ToLower(addr)
	nameLower := strings.ToLower(name)
	var target seenDevice
	if err := c.scan(scanTimeout, func(d seenDevice) bool {
		if addrLower != "" && strings.ToLower(d.Address) == addrLower {
			target = d
			return true
		}
		if nameLower != "" && strings.Contains(strings.ToLower(d.Name), nameLower) {
			target = d
			return true
		}
		return false
	}); err != nil {
		return seenDevice{}, err
	}
	if target.Address != "" {
		return target, nil
	}
	if addr == "" {
		return seenDevice{}, fmt.Errorf("no matching device found within %s", scanTimeout)
	}
	uuid, err := cbgo.ParseUUID(addr)
	if err != nil {
		return seenDevice{}, fmt.Errorf("parse --addr: %w", err)
	}
	peripherals := c.cm.RetrievePeripheralsWithIdentifiers([]cbgo.UUID{uuid})
	if len(peripherals) == 0 {
		return seenDevice{}, fmt.Errorf("no matching device found within %s and address was not cached by CoreBluetooth", scanTimeout)
	}
	return seenDevice{
		Peripheral: peripherals[0],
		Address:    peripherals[0].Identifier().String(),
	}, nil
}

func (c *central) discoverDashboardCharacteristics(prph cbgo.Peripheral) (*cbgo.Characteristic, error) {
	c.servicesCh = make(chan error, 1)
	prph.DiscoverServices(nil)
	if err := waitErr(c.servicesCh, 15*time.Second, "discover services"); err != nil {
		return nil, err
	}
	var writable *cbgo.Characteristic
	var notifyChars int
	for _, svc := range prph.Services() {
		c.charsCh = make(chan error, 1)
		prph.DiscoverCharacteristics(nil, svc)
		if err := waitErr(c.charsCh, 15*time.Second, "discover characteristics"); err != nil {
			continue
		}
		for _, char := range svc.Characteristics() {
			props := char.Properties()
			if writable == nil && props&cbgo.CharacteristicPropertyWrite != 0 {
				charCopy := char
				writable = &charCopy
			}
			if props&cbgo.CharacteristicPropertyNotify != 0 {
				if err := c.setNotify(prph, char, false, 10*time.Second); err != nil {
					return nil, err
				}
				notifyChars++
			}
		}
	}
	if writable == nil {
		return nil, fmt.Errorf("no writable characteristic found")
	}
	if notifyChars == 0 {
		return nil, fmt.Errorf("no notify characteristic found")
	}
	return writable, nil
}

func dashboardURL(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "http://localhost:8080"
	}
	if host == "" || host == "::" || host == "0.0.0.0" || host == "[::]" {
		host = "localhost"
	}
	return "http://" + net.JoinHostPort(host, port)
}

func openDashboard(url string) {
	if err := exec.Command("open", url).Start(); err != nil {
		log.Printf("open dashboard: %v", err)
	}
}

func dashboardPage(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := w.Write([]byte("<!doctype html>")); err != nil {
		log.Printf("write dashboard doctype: %v", err)
		return
	}
	if err := ui.DashboardPage().Render(context.Background(), w); err != nil {
		log.Printf("render dashboard: %v", err)
	}
}
