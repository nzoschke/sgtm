package main

import (
	"encoding/binary"
	"fmt"
	"time"
)

type soundReading struct {
	Time         time.Time `json:"time"`
	Value        float64   `json:"value"`
	Display      float64   `json:"display"`
	Unit         string    `json:"unit"`
	RangeLow     int       `json:"rangeLow"`
	RangeHigh    int       `json:"rangeHigh"`
	Overload     string    `json:"overload"`
	MaxMin       string    `json:"maxMin"`
	LowPower     bool      `json:"lowPower"`
	AutoPowerOff bool      `json:"autoPowerOff"`
	Backlight    bool      `json:"backlight"`
	Hold         bool      `json:"hold"`
}

type dt95Framer struct {
	buf []byte
}

func (f *dt95Framer) Push(chunk []byte) ([]soundReading, error) {
	f.buf = append(f.buf, chunk...)
	var readings []soundReading
	for {
		start := -1
		for i, b := range f.buf {
			if b == 0xd5 {
				start = i
				break
			}
		}
		if start < 0 {
			f.buf = f.buf[:0]
			return readings, nil
		}
		if start > 0 {
			f.buf = f.buf[start:]
		}
		if len(f.buf) < 2 {
			return readings, nil
		}

		var packetLen int
		switch f.buf[1] {
		case 0xf0:
			if len(f.buf) < 4 {
				return readings, nil
			}
			packetLen = int(binary.BigEndian.Uint16(f.buf[2:4])) + 5
		case 0xa1:
			if len(f.buf) < 3 {
				return readings, nil
			}
			packetLen = int(f.buf[2]) + 4
		default:
			f.buf = f.buf[1:]
			continue
		}
		if packetLen <= 0 {
			f.buf = f.buf[1:]
			continue
		}
		if len(f.buf) < packetLen {
			return readings, nil
		}

		packet := append([]byte(nil), f.buf[:packetLen]...)
		f.buf = f.buf[packetLen:]
		if packet[len(packet)-1] != 0x0d {
			continue
		}
		reading, ok, err := parseDT95Reading(packet)
		if err != nil {
			return readings, err
		}
		if ok {
			readings = append(readings, reading)
		}
	}
}

func parseDT95Reading(packet []byte) (soundReading, bool, error) {
	if len(packet) < 11 || packet[0] != 0xd5 || packet[1] != 0xf0 {
		return soundReading{}, false, nil
	}
	n := int(binary.BigEndian.Uint16(packet[2:4]))
	if len(packet) < n+5 {
		return soundReading{}, false, fmt.Errorf("short DT95 packet: got %d want %d", len(packet), n+5)
	}
	data := packet[4 : 4+n]
	if len(data) < 6 {
		return soundReading{}, false, nil
	}

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
	overload := "none"
	switch {
	case flags&0x40 != 0:
		overload = "high"
	case flags&0x20 != 0:
		overload = "low"
	}

	return soundReading{
		Time:         time.Now(),
		Value:        float64(binary.BigEndian.Uint16(data[0:2])) / 10,
		Display:      float64(binary.BigEndian.Uint16(data[2:4])) / 10,
		Unit:         unit,
		RangeLow:     rangeLow,
		RangeHigh:    rangeHigh,
		Overload:     overload,
		MaxMin:       maxMin,
		LowPower:     status&0x08 != 0,
		AutoPowerOff: status&0x04 != 0,
		Backlight:    status&0x02 != 0,
		Hold:         status&0x01 != 0,
	}, true, nil
}
