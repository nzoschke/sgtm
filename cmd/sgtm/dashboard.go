package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/JuulLabs-OSS/cbgo"
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
	dbPath := fs.String("db", ".context/sgtm.sqlite", "SQLite history database path")
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

	go runDashboardBLE(context.Background(), *addr, *name, *scanTimeout, store)

	mux := http.NewServeMux()
	mux.HandleFunc("/", dashboardPage)
	mux.HandleFunc("/events", store.events)
	mux.HandleFunc("/api/state", store.state)
	log.Printf("dashboard listening on http://localhost%s", displayListen(*listen))
	return http.ListenAndServe(*listen, mux)
}

func newDashboardStore(cfg dashboardConfig, db *readingDB) *dashboardStore {
	return &dashboardStore{
		config:      cfg,
		db:          db,
		status:      "starting",
		subscribers: make(map[chan dashboardEvent]struct{}),
	}
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

func displayListen(listen string) string {
	if strings.HasPrefix(listen, ":") {
		return listen
	}
	return "/" + listen
}

func dashboardPage(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(dashboardHTML))
}

const dashboardHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Sound Level</title>
<style>
:root {
  color-scheme: dark;
  --bg: #07090b;
  --panel: #10161b;
  --text: #f6f8fa;
  --muted: #99a4ad;
  --green: #2ec46f;
  --amber: #e2a32d;
  --red: #ef4444;
  --line: #f7fafc;
}
* { box-sizing: border-box; }
body {
  margin: 0;
  height: 100vh;
  background: var(--bg);
  color: var(--text);
  font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
  overflow: hidden;
}
.app {
  height: 100vh;
  display: grid;
  grid-template-rows: minmax(220px, 32vh) minmax(0, 1fr);
  gap: 14px;
  padding: 18px;
  overflow: hidden;
}
.top {
  display: grid;
  grid-template-columns: minmax(0, 1.4fr) minmax(220px, .6fr);
  gap: 14px;
  align-items: stretch;
  min-height: 0;
}
.reading, .side, .chartShell {
  background: var(--panel);
  border: 1px solid #26313a;
  border-radius: 8px;
}
.reading {
  padding: 24px;
  display: grid;
  grid-template-columns: auto 1fr;
  gap: 14px;
  align-items: center;
  min-height: 0;
  overflow: hidden;
}
.value {
  font-size: 8.75rem;
  font-weight: 800;
  line-height: .82;
  letter-spacing: 0;
}
.unit {
  font-size: 3rem;
  color: var(--muted);
  font-weight: 700;
  padding-bottom: .08em;
}
.band {
  grid-column: 1 / -1;
  font-size: 2.2rem;
  font-weight: 800;
  letter-spacing: 0;
  align-self: end;
}
.band.ideal { color: var(--green); }
.band.watch { color: var(--amber); }
.band.unsafe { color: var(--red); }
.side {
  padding: 22px;
  display: grid;
  grid-template-rows: 1fr auto;
  gap: 10px;
  min-height: 0;
  overflow: hidden;
}
.stats {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 14px 18px;
  align-content: start;
  min-height: 0;
}
.label {
  color: var(--muted);
  font-size: 1rem;
  text-transform: uppercase;
  letter-spacing: 0;
}
.metric {
  font-size: 2.15rem;
  font-weight: 750;
  line-height: 1.05;
}
.metric.small {
  font-size: 1.35rem;
  overflow-wrap: anywhere;
}
.status {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  color: var(--muted);
  font-size: 1.5rem;
}
.dot {
  width: 14px;
  height: 14px;
  border-radius: 999px;
  background: var(--amber);
}
.dot.live { background: var(--green); }
.dot.retrying { background: var(--red); }
.chartShell {
  min-height: 0;
  padding: 12px;
  display: grid;
  overflow: hidden;
}
canvas {
  width: 100%;
  height: 100%;
  min-height: 0;
  display: block;
}
@media (max-width: 800px) {
  .app { padding: 16px; }
  .app { grid-template-rows: minmax(260px, 42vh) minmax(0, 1fr); }
  .top { grid-template-columns: 1fr; }
  .reading { grid-template-columns: 1fr; }
  .value { font-size: 5.5rem; }
  .unit { font-size: 2.25rem; }
  .band { font-size: 2rem; }
  .metric { font-size: 2rem; }
  .unit { padding-bottom: 0; }
}
</style>
</head>
<body>
<main class="app">
  <section class="top">
    <div class="reading">
      <div class="value" id="value">--.-</div>
      <div class="unit" id="unit">dBA</div>
      <div class="band" id="band">Waiting</div>
    </div>
    <aside class="side">
      <div class="stats">
        <div>
          <div class="label">Peak</div>
          <div class="metric" id="peak">--.-</div>
        </div>
        <div>
          <div class="label">Average</div>
          <div class="metric" id="avg">--.-</div>
        </div>
        <div>
          <div class="label">Window</div>
          <div class="metric small" id="window">--</div>
        </div>
        <div>
          <div class="label">Session</div>
          <div class="metric small" id="session">--</div>
        </div>
        <div>
          <div class="label">Battery</div>
          <div class="metric small" id="battery">--</div>
        </div>
        <div>
          <div class="label">Auto-Off</div>
          <div class="metric small" id="autoOff">--</div>
        </div>
        <div>
          <div class="label">Range</div>
          <div class="metric small" id="range">--</div>
        </div>
        <div>
          <div class="label">Meter</div>
          <div class="metric small" id="meterState">--</div>
        </div>
      </div>
      <div class="status"><span class="dot" id="dot"></span><span id="status">starting</span></div>
    </aside>
  </section>
  <section class="chartShell">
    <canvas id="chart"></canvas>
  </section>
</main>
<script>
let cfg = {idealMax: 85, unsafeMin: 95, chartMin: 35, chartMax: 120, historySec: 1800};
let readings = [];
let sessionStarted = null;
const el = id => document.getElementById(id);
const canvas = el('chart');
const ctx = canvas.getContext('2d');

function setState(state) {
  if (state.config) cfg = state.config;
  if (state.history) {
    readings = state.history;
  } else if (state.reading) {
    addReading(state.reading);
  }
  if (state.session) sessionStarted = Date.parse(state.session);
  if (state.status) setStatus(state.status);
  update();
}

function addReading(reading) {
  readings.push(reading);
  const cutoff = Date.now() - cfg.historySec * 1000;
  readings = readings.filter(r => Date.parse(r.time) >= cutoff);
}

function setStatus(status) {
  el('status').textContent = status;
  el('dot').className = 'dot ' + status;
}

function bandFor(v) {
  if (!Number.isFinite(v)) return ['Waiting', ''];
  if (v >= cfg.unsafeMin) return ['Too High', 'unsafe'];
  if (v > cfg.idealMax) return ['Watch', 'watch'];
  return ['Ideal', 'ideal'];
}

function update() {
  const latest = readings[readings.length - 1];
  if (latest) {
    el('value').textContent = latest.display.toFixed(1);
    el('unit').textContent = latest.unit;
    const [label, cls] = bandFor(latest.display);
    el('band').textContent = label;
    el('band').className = 'band ' + cls;
    el('battery').textContent = latest.lowPower ? 'Low' : 'OK';
    el('autoOff').textContent = latest.autoPowerOff ? 'Enabled' : 'Disabled';
    el('range').textContent = latest.rangeLow + '-' + latest.rangeHigh;
    el('meterState').textContent = latest.hold ? 'Hold' : (latest.overload !== 'none' ? latest.overload : 'Live');
  }
  const values = readings.map(r => r.display).filter(Number.isFinite);
  const peak = values.length ? Math.max(...values) : NaN;
  const avg = values.length ? values.reduce((a, b) => a + b, 0) / values.length : NaN;
  el('peak').textContent = Number.isFinite(peak) ? peak.toFixed(1) : '--.-';
  el('avg').textContent = Number.isFinite(avg) ? avg.toFixed(1) : '--.-';
  el('window').textContent = Math.round(cfg.historySec / 60) + ' min';
  el('session').textContent = sessionStarted ? formatDuration(Date.now() - sessionStarted) : '--';
  draw();
}

function formatDuration(ms) {
  const total = Math.max(0, Math.floor(ms / 1000));
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  if (h > 0) return h + 'h ' + String(m).padStart(2, '0') + 'm';
  return m + ':' + String(s).padStart(2, '0');
}

function resizeCanvas() {
  const rect = canvas.getBoundingClientRect();
  const dpr = window.devicePixelRatio || 1;
  canvas.width = Math.max(1, Math.floor(rect.width * dpr));
  canvas.height = Math.max(1, Math.floor(rect.height * dpr));
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
}

function draw() {
  resizeCanvas();
  const w = canvas.clientWidth;
  const h = canvas.clientHeight;
  const pad = {l: 58, r: 18, t: 18, b: 42};
  const x0 = pad.l, y0 = pad.t, cw = w - pad.l - pad.r, ch = h - pad.t - pad.b;
  ctx.clearRect(0, 0, w, h);
  ctx.fillStyle = '#0b1014';
  ctx.fillRect(0, 0, w, h);
  const min = cfg.chartMin, max = cfg.chartMax;
  const y = v => y0 + (max - v) / (max - min) * ch;
  ctx.fillStyle = 'rgba(239,68,68,.22)';
  ctx.fillRect(x0, y0, cw, Math.max(0, y(cfg.unsafeMin) - y0));
  ctx.fillStyle = 'rgba(226,163,45,.18)';
  ctx.fillRect(x0, y(cfg.unsafeMin), cw, Math.max(0, y(cfg.idealMax) - y(cfg.unsafeMin)));
  ctx.fillStyle = 'rgba(46,196,111,.18)';
  ctx.fillRect(x0, y(cfg.idealMax), cw, Math.max(0, y0 + ch - y(cfg.idealMax)));
  ctx.strokeStyle = '#26313a';
  ctx.lineWidth = 1;
  ctx.fillStyle = '#99a4ad';
  ctx.font = '14px system-ui';
  for (let v = Math.ceil(min / 10) * 10; v <= max; v += 10) {
    const yy = y(v);
    ctx.beginPath();
    ctx.moveTo(x0, yy);
    ctx.lineTo(x0 + cw, yy);
    ctx.stroke();
    ctx.fillText(String(v), 14, yy + 5);
  }
  const now = Date.now();
  const start = now - cfg.historySec * 1000;
  const x = t => x0 + (Date.parse(t) - start) / (cfg.historySec * 1000) * cw;
  ctx.strokeStyle = 'rgba(247,250,252,.95)';
  ctx.lineWidth = 3;
  ctx.beginPath();
  let moved = false;
  for (const r of readings) {
    const xx = x(r.time);
    const yy = y(r.display);
    if (xx < x0 || xx > x0 + cw || !Number.isFinite(yy)) continue;
    if (!moved) {
      ctx.moveTo(xx, yy);
      moved = true;
    } else {
      ctx.lineTo(xx, yy);
    }
  }
  if (moved) ctx.stroke();
  ctx.strokeStyle = '#5b6670';
  ctx.strokeRect(x0, y0, cw, ch);
  ctx.fillStyle = '#f6f8fa';
  ctx.font = '16px system-ui';
  ctx.fillText('Ideal', x0 + 12, y(cfg.idealMax) + 26);
  ctx.fillText('Too High', x0 + 12, y0 + 26);
}

window.addEventListener('resize', update);
fetch('/api/state').then(r => r.json()).then(setState);
const events = new EventSource('/events');
events.onmessage = ev => setState(JSON.parse(ev.data));
setInterval(update, 1000);
</script>
</body>
</html>`
