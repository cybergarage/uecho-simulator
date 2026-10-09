package model

import (
	"crypto/rand"
	"slices"
	"sync"
)

const Appendix = "Release R rev.4"
const Middleware = "ECHONET Lite v1.14"

func definition(epc byte, name string, writable, announce bool) Definition {
	return Definition{EPC: epc, Name: name, Writable: writable, Announce: announce}
}
func newProfiles() *Store {
	s := &Store{devices: make(map[uint32]*deviceState), ambient: 220, revision: 1}
	id := make([]byte, 17)
	id[0] = 0xfe
	id[1] = 0xff
	id[2] = 0xff
	id[3] = 0xff
	_, _ = rand.Read(id[4:])
	for _, d := range []struct {
		eoj        uint32
		name, kind string
	}{{Light, "Ceiling light", "light"}, {Aircon, "Air conditioner", "aircon"}, {Sensor, "Temperature sensor", "sensor"}} {
		defs := []Definition{
			definition(0x80, "Operation status", d.eoj != Sensor, true),
			definition(0x81, "Installation location", true, true),
			definition(0x82, "Appendix R revision 4", false, false),
			definition(0x83, "Experimental process identification", false, false),
			definition(0x88, "Fault status", false, true),
			definition(0x8a, "Experimental manufacturer", false, false),
			definition(0x9d, "Announcement map", false, false),
			definition(0x9e, "Set map", false, false),
			definition(0x9f, "Get map", false, false),
		}
		objectID := slices.Clone(id)
		objectID[14] = byte(d.eoj >> 16)
		objectID[15] = byte(d.eoj >> 8)
		objectID[16] = byte(d.eoj)
		data := map[byte][]byte{0x80: {0x31}, 0x81: {0}, 0x82: {0, 0, 'R', 4}, 0x83: objectID, 0x88: {0x42}, 0x8a: {0xff, 0xff, 0xff}}
		switch d.eoj {
		case Light:
			defs = append(defs, definition(0xb0, "Brightness (%)", true, true),
				definition(0xb6, "Lighting mode", true, false))
			data[0xb0] = []byte{60}
			data[0xb6] = []byte{0x42}
		case Aircon:
			defs = append(defs, definition(0x8f, "Power saving", true, true),
				definition(0xa0, "Air flow", true, true),
				definition(0xb0, "Operation mode", true, true),
				definition(0xb3, "Setpoint (0..50 C)", true, true),
				definition(0xbb, "Room temperature (signed C)", false, true))
			data[0x8f] = []byte{0x42}
			data[0xa0] = []byte{0x41}
			data[0xb0] = []byte{0x42}
			data[0xb3] = []byte{24}
		case Sensor:
			defs = append(defs, definition(0xe0, "Temperature (signed 0.1 C)", false, true))
			data[0x80] = []byte{0x30}
		}
		s.devices[d.eoj] = &deviceState{d.name, d.kind, defs, data}
	}
	nodeDefs := []Definition{
		definition(0x80, "Node operation status", false, true),
		definition(0x82, "Middleware version 1.14 Format 1", false, false),
		definition(0x83, "Experimental process identification", false, false),
		definition(0x8a, "Experimental manufacturer", false, false),
		definition(0x9d, "Announcement map", false, false),
		definition(0x9e, "Set map", false, false),
		definition(0x9f, "Get map", false, false),
		definition(0xd3, "Device instance count", false, false),
		definition(0xd4, "Class count including profile", false, false),
		definition(0xd5, "Instance list notification", false, true),
		definition(0xd6, "Device instance list", false, false),
		definition(0xd7, "Device class list", false, false),
	}
	instances := []byte{3, 2, 0x90, 1, 1, 0x30, 1, 0, 0x11, 1}
	s.devices[Node] = &deviceState{"Node profile", "node", nodeDefs, map[byte][]byte{0x80: {0x30}, 0x82: {1, 14, 1, 0}, 0x83: id, 0x8a: {0xff, 0xff, 0xff}, 0xd3: {0, 0, 3}, 0xd4: {0, 4}, 0xd5: instances, 0xd6: slices.Clone(instances), 0xd7: {3, 2, 0x90, 1, 0x30, 0, 0x11}}}
	s.logLocked("INFO", "system", "Virtual profiles: "+Middleware+" / "+Appendix+"; volatile state, no hardware", nil)
	return s
}

func validValue(eoj uint32, epc byte, data []byte) bool {
	if epc == 0x81 {
		return len(data) == 1 && data[0] != 1 && (data[0] == 0 || data[0] >= 8) || len(data) == 17 && data[0] == 1
	}
	if len(data) != 1 {
		return false
	}
	v := data[0]
	switch epc {
	case 0x80:
		return v == 0x30 || v == 0x31
	case 0x8f:
		return v == 0x41 || v == 0x42
	case 0xa0:
		return v == 0x41 || v >= 0x31 && v <= 0x38
	case 0xb0:
		if eoj == Light {
			return v <= 100
		}
		return v >= 0x40 && v <= 0x45
	case 0xb3:
		return v <= 50
	case 0xb6:
		return v == 0x42 // Main lighting only; no automatic/night/color functions.
	}
	return false
}
func propertyMap(codes []byte) []byte {
	if len(codes) < 16 {
		return append([]byte{byte(len(codes))}, codes...)
	}
	out := make([]byte, 17)
	out[0] = byte(len(codes))
	for _, epc := range codes {
		out[1+int(epc&15)] |= 1 << ((epc >> 4) - 8)
	}
	return out
}

// Targets expands class instance 00; each response uses its concrete instance.
func (s *Store) Targets(eoj uint32) []uint32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []uint32{}
	for _, candidate := range []uint32{Node, Light, Aircon, Sensor} {
		if candidate == eoj || eoj&255 == 0 && candidate>>8 == eoj>>8 {
			out = append(out, candidate)
		}
	}
	return out
}

type Change struct {
	EOJ  uint32
	EPC  byte
	Data []byte
}

func (s *Store) notifyLocked(eoj uint32, epc byte) {
	_, def, err := s.definition(eoj, epc)
	if err != nil || !def.Announce {
		return
	}
	b, _ := s.valueLocked(eoj, epc)
	for ch := range s.notifications {
		select {
		case ch <- Change{eoj, epc, slices.Clone(b)}:
		default:
			close(ch)
			delete(s.notifications, ch)
		}
	}
}

// SubscribeNotifications preserves transitions. A slow consumer is closed rather
// than silently dropping required announcements, so the transport can fail cleanly.
func (s *Store) SubscribeNotifications() (<-chan Change, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.notifications == nil {
		s.notifications = make(map[chan Change]struct{})
	}
	ch := make(chan Change, 256)
	s.notifications[ch] = struct{}{}
	var once sync.Once
	return ch, func() {
		once.Do(func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			if _, ok := s.notifications[ch]; ok {
				delete(s.notifications, ch)
				close(ch)
			}
		})
	}
}
func (s *Store) Startup() Change { b, _ := s.Read(Node, 0xd5); return Change{Node, 0xd5, b} }
