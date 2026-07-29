package main

import (
	"encoding/binary"
	"encoding/hex"
	"flag"
	"fmt"
	"math"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/JuulLabs-OSS/cbgo"
)

type seenDevice struct {
	Peripheral       cbgo.Peripheral
	Address          string
	Name             string
	RSSI             int
	ServiceUUIDs     []cbgo.UUID
	ManufacturerData []byte
	ServiceData      []cbgo.AdvServiceData
}

type central struct {
	cbgo.CentralManagerDelegateBase
	cbgo.PeripheralDelegateBase

	cm cbgo.CentralManager

	mu               sync.Mutex
	scanHandler      func(seenDevice)
	connectCh        chan cbgo.Peripheral
	connectErr       error
	servicesCh       chan error
	charsCh          chan error
	descriptorsCh    chan error
	readCharCh       chan readCharResult
	readDescCh       chan readDescResult
	notifyEnabledCh  chan error
	writeCh          chan error
	notifyChars      map[string]bool
	notifyHandler    func(cbgo.Characteristic, []byte)
	logNotifications bool
}

type readCharResult struct {
	char cbgo.Characteristic
	err  error
}

type readDescResult struct {
	desc cbgo.Descriptor
	err  error
}

func main() {
	runtime.LockOSThread()

	var err error
	args := os.Args[1:]
	if len(args) == 0 {
		err = chromeCmd(nil)
	} else {
		switch args[0] {
		case "chrome":
			err = chromeCmd(args[1:])
		case "scan":
			err = scanCmd(args[1:])
		case "inspect":
			err = inspectCmd(args[1:])
		case "dashboard":
			err = dashboardCmd(args[1:])
		default:
			if strings.HasPrefix(args[0], "-") {
				err = chromeCmd(args)
				break
			}
			usage()
			os.Exit(2)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `Usage:
  sgtm [chrome-dashboard flags]
  sgtm chrome [--listen :8090]
  sgtm scan [--duration 15s] [--name text]
  sgtm inspect (--addr UUID | --name text) [--scan-timeout 20s] [--notify 30s] [--write hex[,hex...]]
  sgtm dashboard (--addr UUID | --name text) [--listen :8080]

Examples:
  bin/sgtm
  bin/sgtm --listen :8091
  bin/sgtm scan --duration 20s
  bin/sgtm inspect --name decibel --notify 60s
  bin/sgtm inspect --addr XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX
  bin/sgtm dashboard --name "850019 EM" --listen :8080

`)
}

func scanCmd(args []string) error {
	fs := flag.NewFlagSet("scan", flag.ExitOnError)
	duration := fs.Duration("duration", 15*time.Second, "BLE scan duration")
	nameFilter := fs.String("name", "", "case-insensitive local-name substring filter")
	if err := fs.Parse(args); err != nil {
		return err
	}

	c, err := newCentral()
	if err != nil {
		return err
	}
	c.logNotifications = true

	filter := strings.ToLower(*nameFilter)
	devices := make(map[string]seenDevice)
	err = c.scan(*duration, func(d seenDevice) bool {
		if filter != "" && !strings.Contains(strings.ToLower(d.Name), filter) {
			return false
		}
		devices[d.Address] = d
		return false
	})
	if err != nil {
		return err
	}

	printDevices(devices)
	return nil
}

func inspectCmd(args []string) error {
	fs := flag.NewFlagSet("inspect", flag.ExitOnError)
	addr := fs.String("addr", "", "CoreBluetooth device UUID from scan output")
	name := fs.String("name", "", "case-insensitive local-name substring to discover and connect")
	scanTimeout := fs.Duration("scan-timeout", 20*time.Second, "time to scan when resolving a name or address")
	notifyFor := fs.Duration("notify", 30*time.Second, "time to listen for characteristic notifications")
	writeHex := fs.String("write", "", "comma-separated hex packet(s) to write to the first writable characteristic after subscribing")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *addr == "" && *name == "" {
		return fmt.Errorf("inspect requires --addr or --name")
	}

	c, err := newCentral()
	if err != nil {
		return err
	}

	addrLower := strings.ToLower(*addr)
	nameLower := strings.ToLower(*name)
	var target seenDevice
	err = c.scan(*scanTimeout, func(d seenDevice) bool {
		if addrLower != "" && strings.ToLower(d.Address) == addrLower {
			target = d
			return true
		}
		if nameLower != "" && strings.Contains(strings.ToLower(d.Name), nameLower) {
			target = d
			return true
		}
		return false
	})
	if err != nil {
		return err
	}
	if target.Address == "" {
		if *addr == "" {
			return fmt.Errorf("no matching device found within %s", scanTimeout.String())
		}
		uuid, err := cbgo.ParseUUID(*addr)
		if err != nil {
			return fmt.Errorf("parse --addr: %w", err)
		}
		peripherals := c.cm.RetrievePeripheralsWithIdentifiers([]cbgo.UUID{uuid})
		if len(peripherals) == 0 {
			return fmt.Errorf("no matching device found within %s and address was not cached by CoreBluetooth", scanTimeout.String())
		}
		target = seenDevice{
			Peripheral: peripherals[0],
			Address:    peripherals[0].Identifier().String(),
		}
	}

	fmt.Printf("connecting address=%s name=%q rssi=%d\n", target.Address, target.Name, target.RSSI)
	prph, err := c.connect(target.Peripheral, 15*time.Second)
	if err != nil {
		return err
	}
	defer c.cm.CancelConnect(prph)
	prph.SetDelegate(c)

	var writePackets [][]byte
	if *writeHex != "" {
		writePackets, err = parseWritePackets(*writeHex)
		if err != nil {
			return fmt.Errorf("decode --write: %w", err)
		}
	}

	if err := c.discoverAndRead(prph, *notifyFor, writePackets); err != nil {
		return err
	}
	return nil
}

func newCentral() (*central, error) {
	c := &central{notifyChars: make(map[string]bool)}
	c.cm = cbgo.NewCentralManager(nil)
	c.cm.SetDelegate(c)
	if c.cm.State() == cbgo.ManagerStateUnknown {
		timer := time.NewTimer(10 * time.Second)
		defer timer.Stop()
		for c.cm.State() == cbgo.ManagerStateUnknown {
			select {
			case <-time.After(100 * time.Millisecond):
			case <-timer.C:
				return nil, fmt.Errorf("timeout waiting for Bluetooth state")
			}
		}
	}
	if c.cm.State() != cbgo.ManagerStatePoweredOn {
		return nil, fmt.Errorf("Bluetooth is not powered on or authorized: state=%d", c.cm.State())
	}
	return c, nil
}

func (c *central) scan(duration time.Duration, match func(seenDevice) bool) error {
	done := make(chan struct{})
	c.mu.Lock()
	c.scanHandler = func(d seenDevice) {
		if match(d) {
			c.cm.StopScan()
			select {
			case <-done:
			default:
				close(done)
			}
		}
	}
	c.mu.Unlock()

	c.cm.Scan(nil, &cbgo.CentralManagerScanOpts{AllowDuplicates: false})
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
		c.cm.StopScan()
	}

	c.mu.Lock()
	c.scanHandler = nil
	c.mu.Unlock()
	return nil
}

func (c *central) connect(prph cbgo.Peripheral, timeout time.Duration) (cbgo.Peripheral, error) {
	c.mu.Lock()
	c.connectCh = make(chan cbgo.Peripheral, 1)
	c.connectErr = nil
	c.mu.Unlock()

	c.cm.Connect(prph, nil)
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case p := <-c.connectCh:
		if c.connectErr != nil {
			return cbgo.Peripheral{}, c.connectErr
		}
		return p, nil
	case <-timer.C:
		c.cm.CancelConnect(prph)
		return cbgo.Peripheral{}, fmt.Errorf("timeout connecting")
	}
}

func (c *central) discoverAndRead(prph cbgo.Peripheral, notifyFor time.Duration, writePackets [][]byte) error {
	c.servicesCh = make(chan error, 1)
	prph.DiscoverServices(nil)
	if err := waitErr(c.servicesCh, 15*time.Second, "discover services"); err != nil {
		return err
	}

	services := prph.Services()
	var writable *cbgo.Characteristic
	for _, svc := range services {
		fmt.Printf("service %s primary=%t\n", svc.UUID(), svc.IsPrimary())
		c.charsCh = make(chan error, 1)
		prph.DiscoverCharacteristics(nil, svc)
		if err := waitErr(c.charsCh, 15*time.Second, "discover characteristics"); err != nil {
			fmt.Printf("  characteristics: %v\n", err)
			continue
		}
		for _, char := range svc.Characteristics() {
			props := char.Properties()
			fmt.Printf("  characteristic %s property=0x%02x (%s)\n", char.UUID(), props, propString(props))
			if writable == nil && (props&cbgo.CharacteristicPropertyWrite != 0 || props&cbgo.CharacteristicPropertyWriteWithoutResponse != 0) {
				charCopy := char
				writable = &charCopy
			}
			if props&cbgo.CharacteristicPropertyRead != 0 {
				b, err := c.readCharacteristic(prph, char, 10*time.Second)
				if err != nil {
					fmt.Printf("    read: %v\n", err)
				} else {
					fmt.Printf("    read: %s\n", describeBytes(b))
				}
			}

			c.descriptorsCh = make(chan error, 1)
			prph.DiscoverDescriptors(char)
			if err := waitErr(c.descriptorsCh, 10*time.Second, "discover descriptors"); err == nil {
				for _, desc := range char.Descriptors() {
					fmt.Printf("    descriptor %s\n", desc.UUID())
					b, err := c.readDescriptor(prph, desc, 10*time.Second)
					if err != nil {
						fmt.Printf("      read: %v\n", err)
					} else {
						fmt.Printf("      read: %s\n", describeBytes(b))
					}
				}
			}

			if notifyFor > 0 && props&cbgo.CharacteristicPropertyNotify != 0 {
				if err := c.setNotify(prph, char, false, 10*time.Second); err != nil {
					fmt.Printf("    notify: %v\n", err)
				}
			}
			if notifyFor > 0 && props&cbgo.CharacteristicPropertyIndicate != 0 {
				if err := c.setNotify(prph, char, true, 10*time.Second); err != nil {
					fmt.Printf("    indicate: %v\n", err)
				}
			}
		}
	}

	if len(writePackets) > 0 {
		if writable == nil {
			return fmt.Errorf("no writable characteristic found for --write")
		}
		for _, writePacket := range writePackets {
			fmt.Printf("writing %s to %s\n", hex.EncodeToString(writePacket), writable.UUID())
			if err := c.writeCharacteristic(prph, *writable, writePacket, 10*time.Second); err != nil {
				return err
			}
			time.Sleep(300 * time.Millisecond)
		}
	}

	if notifyFor > 0 && len(c.notifyChars) > 0 {
		fmt.Printf("listening for notifications for %s\n", notifyFor.String())
		time.Sleep(notifyFor)
	}
	return nil
}

func (c *central) readCharacteristic(prph cbgo.Peripheral, char cbgo.Characteristic, timeout time.Duration) ([]byte, error) {
	c.readCharCh = make(chan readCharResult, 1)
	prph.ReadCharacteristic(char)
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case result := <-c.readCharCh:
		c.readCharCh = nil
		if result.err != nil {
			return nil, result.err
		}
		return append([]byte(nil), result.char.Value()...), nil
	case <-timer.C:
		c.readCharCh = nil
		return nil, fmt.Errorf("timeout reading characteristic")
	}
}

func (c *central) readDescriptor(prph cbgo.Peripheral, desc cbgo.Descriptor, timeout time.Duration) ([]byte, error) {
	c.readDescCh = make(chan readDescResult, 1)
	prph.ReadDescriptor(desc)
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case result := <-c.readDescCh:
		c.readDescCh = nil
		if result.err != nil {
			return nil, result.err
		}
		return append([]byte(nil), result.desc.Value()...), nil
	case <-timer.C:
		c.readDescCh = nil
		return nil, fmt.Errorf("timeout reading descriptor")
	}
}

func (c *central) writeCharacteristic(prph cbgo.Peripheral, char cbgo.Characteristic, data []byte, timeout time.Duration) error {
	c.writeCh = make(chan error, 1)
	withResponse := char.Properties()&cbgo.CharacteristicPropertyWrite != 0
	prph.WriteCharacteristic(data, char, withResponse)
	if withResponse {
		err := waitErr(c.writeCh, timeout, "write characteristic")
		c.writeCh = nil
		return err
	}
	c.writeCh = nil
	return nil
}

func (c *central) setNotify(prph cbgo.Peripheral, char cbgo.Characteristic, _ bool, timeout time.Duration) error {
	c.notifyEnabledCh = make(chan error, 1)
	prph.SetNotify(true, char)
	if err := waitErr(c.notifyEnabledCh, timeout, "enable notify"); err != nil {
		return err
	}
	c.notifyChars[char.UUID().String()] = true
	return nil
}

func waitErr(ch <-chan error, timeout time.Duration, action string) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-ch:
		if err != nil {
			return fmt.Errorf("%s: %w", action, err)
		}
		return nil
	case <-timer.C:
		return fmt.Errorf("timeout: %s", action)
	}
}

func parseWritePackets(s string) ([][]byte, error) {
	fields := strings.Split(s, ",")
	packets := make([][]byte, 0, len(fields))
	for _, field := range fields {
		clean := strings.NewReplacer(" ", "", "\t", "", "\n", "", ":", "", "-", "").Replace(field)
		if clean == "" {
			continue
		}
		packet, err := hex.DecodeString(clean)
		if err != nil {
			return nil, err
		}
		packets = append(packets, packet)
	}
	if len(packets) == 0 {
		return nil, fmt.Errorf("empty packet list")
	}
	return packets, nil
}

func (c *central) DidDiscoverPeripheral(_ cbgo.CentralManager, prph cbgo.Peripheral, adv cbgo.AdvFields, rssi int) {
	d := seenDevice{
		Peripheral:       prph,
		Address:          prph.Identifier().String(),
		Name:             adv.LocalName,
		RSSI:             rssi,
		ServiceUUIDs:     append([]cbgo.UUID(nil), adv.ServiceUUIDs...),
		ManufacturerData: append([]byte(nil), adv.ManufacturerData...),
		ServiceData:      cloneServiceData(adv.ServiceData),
	}
	c.mu.Lock()
	handler := c.scanHandler
	c.mu.Unlock()
	if handler != nil {
		handler(d)
	}
}

func (c *central) DidConnectPeripheral(_ cbgo.CentralManager, prph cbgo.Peripheral) {
	c.connectCh <- prph
}

func (c *central) DidFailToConnectPeripheral(_ cbgo.CentralManager, _ cbgo.Peripheral, err error) {
	c.connectErr = err
	c.connectCh <- cbgo.Peripheral{}
}

func (c *central) DidDiscoverServices(_ cbgo.Peripheral, err error) {
	c.servicesCh <- err
}

func (c *central) DidDiscoverCharacteristics(_ cbgo.Peripheral, _ cbgo.Service, err error) {
	c.charsCh <- err
}

func (c *central) DidDiscoverDescriptors(_ cbgo.Peripheral, _ cbgo.Characteristic, err error) {
	c.descriptorsCh <- err
}

func (c *central) DidUpdateValueForCharacteristic(_ cbgo.Peripheral, char cbgo.Characteristic, err error) {
	if c.readCharCh != nil {
		c.readCharCh <- readCharResult{char: char, err: err}
		return
	}
	if c.notifyChars[char.UUID().String()] {
		if err != nil {
			fmt.Printf("    notify %s: %v\n", char.UUID(), err)
			return
		}
		value := append([]byte(nil), char.Value()...)
		if c.notifyHandler != nil {
			c.notifyHandler(char, value)
		}
		if c.logNotifications {
			fmt.Printf("    notify %s: %s\n", char.UUID(), describeBytes(value))
		}
	}
}

func (c *central) DidUpdateValueForDescriptor(_ cbgo.Peripheral, desc cbgo.Descriptor, err error) {
	c.readDescCh <- readDescResult{desc: desc, err: err}
}

func (c *central) DidWriteValueForCharacteristic(_ cbgo.Peripheral, _ cbgo.Characteristic, err error) {
	if c.writeCh != nil {
		c.writeCh <- err
	}
}

func (c *central) DidUpdateNotificationState(_ cbgo.Peripheral, char cbgo.Characteristic, err error) {
	if err == nil && !char.IsNotifying() {
		err = fmt.Errorf("notification state did not enable")
	}
	c.notifyEnabledCh <- err
}

func cloneServiceData(in []cbgo.AdvServiceData) []cbgo.AdvServiceData {
	out := make([]cbgo.AdvServiceData, len(in))
	for i, item := range in {
		out[i] = cbgo.AdvServiceData{
			UUID: item.UUID,
			Data: append([]byte(nil), item.Data...),
		}
	}
	return out
}

func printDevices(byAddr map[string]seenDevice) {
	if len(byAddr) == 0 {
		fmt.Println("no BLE advertisements found")
		return
	}
	devices := make([]seenDevice, 0, len(byAddr))
	for _, d := range byAddr {
		devices = append(devices, d)
	}
	sort.Slice(devices, func(i, j int) bool {
		if devices[i].RSSI == devices[j].RSSI {
			return devices[i].Name < devices[j].Name
		}
		return devices[i].RSSI > devices[j].RSSI
	})
	for _, d := range devices {
		name := d.Name
		if name == "" {
			name = "<unnamed>"
		}
		fmt.Printf("%s rssi=%d name=%q\n", d.Address, d.RSSI, name)
		if len(d.ServiceUUIDs) > 0 {
			fmt.Printf("  services: %s\n", joinUUIDs(d.ServiceUUIDs))
		}
		if len(d.ManufacturerData) > 0 {
			fmt.Printf("  manufacturer: %s\n", hex.EncodeToString(d.ManufacturerData))
		}
		for _, s := range d.ServiceData {
			fmt.Printf("  service-data %s: %s\n", s.UUID, hex.EncodeToString(s.Data))
		}
	}
}

func joinUUIDs(uuids []cbgo.UUID) string {
	parts := make([]string, 0, len(uuids))
	for _, uuid := range uuids {
		parts = append(parts, uuid.String())
	}
	return strings.Join(parts, ", ")
}

func propString(p cbgo.CharacteristicProperties) string {
	var parts []string
	for _, item := range []struct {
		prop cbgo.CharacteristicProperties
		name string
	}{
		{cbgo.CharacteristicPropertyBroadcast, "broadcast"},
		{cbgo.CharacteristicPropertyRead, "read"},
		{cbgo.CharacteristicPropertyWriteWithoutResponse, "write-no-response"},
		{cbgo.CharacteristicPropertyWrite, "write"},
		{cbgo.CharacteristicPropertyNotify, "notify"},
		{cbgo.CharacteristicPropertyIndicate, "indicate"},
		{cbgo.CharacteristicPropertyAuthenticatedSignedWrites, "signed-write"},
		{cbgo.CharacteristicPropertyExtendedProperties, "extended"},
		{cbgo.CharacteristicPropertyNotifyEncryptionRequired, "notify-encryption-required"},
		{cbgo.CharacteristicPropertyIndicateEncryptionRequired, "indicate-encryption-required"},
	} {
		if p&item.prop != 0 {
			parts = append(parts, item.name)
		}
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ",")
}

func describeBytes(b []byte) string {
	if len(b) == 0 {
		return "len=0"
	}
	parts := []string{
		fmt.Sprintf("len=%d", len(b)),
		"hex=" + hex.EncodeToString(b),
	}
	if text, ok := printableASCII(b); ok {
		parts = append(parts, fmt.Sprintf("ascii=%q", text))
	}
	if len(b) >= 2 {
		parts = append(parts,
			fmt.Sprintf("u16le=%d", binary.LittleEndian.Uint16(b[:2])),
			fmt.Sprintf("i16le=%d", int16(binary.LittleEndian.Uint16(b[:2]))),
			fmt.Sprintf("u16be=%d", binary.BigEndian.Uint16(b[:2])),
			fmt.Sprintf("i16be=%d", int16(binary.BigEndian.Uint16(b[:2]))),
		)
	}
	if len(b) >= 4 {
		le := binary.LittleEndian.Uint32(b[:4])
		be := binary.BigEndian.Uint32(b[:4])
		parts = append(parts,
			fmt.Sprintf("u32le=%d", le),
			fmt.Sprintf("f32le=%g", math.Float32frombits(le)),
			fmt.Sprintf("u32be=%d", be),
			fmt.Sprintf("f32be=%g", math.Float32frombits(be)),
		)
	}
	if decoded, ok := describeDT95(b); ok {
		parts = append(parts, decoded)
		return strings.Join(parts, " ")
	}
	if decoded, ok := describeDT1317(b); ok {
		parts = append(parts, decoded)
	}
	return strings.Join(parts, " ")
}

func describeDT95(b []byte) (string, bool) {
	if len(b) < 5 || b[0] != 0xd5 {
		return "", false
	}
	switch b[1] {
	case 0xa1:
		if len(b) < 4 {
			return "dt95=name incomplete", true
		}
		n := int(b[2])
		if len(b) < n+4 {
			return fmt.Sprintf("dt95=name incomplete want-len=%d", n+4), true
		}
		return fmt.Sprintf("dt95=name %q", strings.TrimSpace(string(b[3:3+n]))), true
	case 0xf0:
		if len(b) < 11 {
			return "dt95=data incomplete", true
		}
		n := int(binary.BigEndian.Uint16(b[2:4]))
		if len(b) < n+5 {
			return fmt.Sprintf("dt95=data incomplete payload-len=%d want-len=%d", n, n+5), true
		}
		data := b[4 : 4+n]
		if len(data) < 6 {
			return fmt.Sprintf("dt95=data payload-len=%d raw=%s", n, hex.EncodeToString(data)), true
		}
		value := float64(binary.BigEndian.Uint16(data[0:2])) / 10
		display := float64(binary.BigEndian.Uint16(data[2:4])) / 10
		flags := data[4]
		status := data[5]
		unit := "dBA"
		if flags&0x04 != 0 {
			unit = "dBC"
		}
		rangeLow, rangeHigh := dt95Range(flags)
		maxMin := "none"
		switch flags & 0x03 {
		case 1:
			maxMin = "max"
		case 2:
			maxMin = "min"
		}
		ol := "none"
		switch {
		case flags&0x40 != 0:
			ol = "high"
		case flags&0x20 != 0:
			ol = "low"
		}
		return fmt.Sprintf("dt95=data value=%.1f%s display=%.1f%s range=%d-%d ol=%s maxmin=%s low-power=%t auto-power-off=%t backlight=%t hold=%t flags=0x%02x status=0x%02x",
			value, unit, display, unit, rangeLow, rangeHigh, ol, maxMin, status&0x08 != 0, status&0x04 != 0, status&0x02 != 0, status&0x01 != 0, flags, status), true
	default:
		return "", false
	}
}

func dt95Range(flags byte) (int, int) {
	switch (flags >> 3) & 0x03 {
	case 1:
		return 80, 130
	case 2:
		return 35, 130
	case 3:
		return 35, 80
	default:
		return 50, 100
	}
}

func describeDT1317(b []byte) (string, bool) {
	if len(b) < 7 || b[0] != 0xd5 {
		return "", false
	}
	version := b[1]
	payloadLen := int(binary.BigEndian.Uint32(b[2:6]))
	packetLen := payloadLen + 7
	if packetLen > len(b) {
		return fmt.Sprintf("dt1317=incomplete version=%d want-len=%d", version, packetLen), true
	}
	cmd := b[6]
	if cmd == 0x90 && len(b) >= 9 {
		return fmt.Sprintf("dt1317=ack version=%d data=0x%02x", version, b[8]), true
	}
	if cmd != 0xc3 || len(b) < 14 {
		return fmt.Sprintf("dt1317=packet version=%d payload-len=%d cmd=0x%02x", version, payloadLen, cmd), true
	}

	gear := b[7]
	showCount := b[8]
	dataCount := b[9]
	if version >= 5 {
		showCount = dataCount
	}
	dataSize := int(b[10])
	globalMark := b[11]
	userType := b[12]
	parts := []string{
		fmt.Sprintf("dt1317=data version=%d payload-len=%d gear=0x%02x show-count=%d data-count=%d data-size=%d global=0x%02x user=0x%02x",
			version, payloadLen, gear, showCount, dataCount, dataSize, globalMark, userType),
	}
	if dataSize <= 0 {
		return strings.Join(parts, " "), true
	}
	for i := 0; i < int(dataCount); i++ {
		off := 14 + i*dataSize
		if off+dataSize > len(b) {
			parts = append(parts, fmt.Sprintf("record%d=incomplete", i))
			break
		}
		parts = append(parts, describeDT1317Record(i, b[off:off+dataSize]))
	}
	return strings.Join(parts, " "), true
}

func describeDT1317Record(i int, b []byte) string {
	if len(b) < 8 {
		return fmt.Sprintf("record%d=%s", i, hex.EncodeToString(b))
	}
	raw := int32(binary.BigEndian.Uint32(b[:4]))
	point := b[4]
	value := float64(raw)
	display := ""
	switch {
	case point == 224:
		display = `""`
	case point == 225:
		display = "-"
	case point == 255:
		display = "OL"
	case point <= 9:
		value /= math.Pow10(int(point))
		display = fmt.Sprintf("%.4g", value)
	default:
		display = fmt.Sprintf("raw=%d point=%d", raw, point)
	}
	return fmt.Sprintf("record%d value=%s raw=%d point=%d fun=0x%02x unit=0x%02x other=0x%02x",
		i, display, raw, point, b[5], b[6], b[7])
}

func printableASCII(b []byte) (string, bool) {
	s := strings.TrimSpace(string(b))
	if s == "" {
		return "", false
	}
	for _, r := range s {
		if r > unicode.MaxASCII || (!unicode.IsPrint(r) && !unicode.IsSpace(r)) {
			return "", false
		}
	}
	return s, true
}
