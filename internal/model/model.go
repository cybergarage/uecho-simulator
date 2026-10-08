// Package model owns simulator state. It does not open sockets or access hardware.
package model

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"slices"
	"sync"
	"time"
)

const (
	Light  uint32 = 0x029001
	Aircon uint32 = 0x013001
	Sensor uint32 = 0x001101
)

// Definition is the implemented subset, not a claim of complete MRA compliance.
type Definition struct {
	EPC      byte   `json:"epc"`
	Name     string `json:"name"`
	Writable bool   `json:"writable"`
	Value    string `json:"value"`
}

type Device struct {
	EOJ        uint32       `json:"eoj"`
	Name       string       `json:"name"`
	Kind       string       `json:"kind"`
	Power      bool         `json:"power"`
	Level      int          `json:"level"`
	Mode       string       `json:"mode"`
	Target     int          `json:"target"`
	Properties []Definition `json:"properties"`
}

type Event struct {
	Sequence uint64 `json:"sequence"`
	Time     string `json:"time"`
	Kind     string `json:"kind"`
	Source   string `json:"source"`
	Message  string `json:"message"`
	Frame    string `json:"frame,omitempty"`
}

// Snapshot is a detached copy for TUI, Web, SVG or future HAT adapters.
type Snapshot struct {
	Revision      uint64   `json:"revision"`
	Room          string   `json:"room"`
	AmbientTenths int      `json:"ambientTenths"`
	Devices       []Device `json:"devices"`
	Events        []Event  `json:"events"`
}

type deviceState struct {
	name, kind  string
	definitions []Definition
	data        map[byte][]byte
}

type Store struct {
	mu                 sync.Mutex
	devices            map[uint32]*deviceState
	ambient            int
	revision, sequence uint64
	events             []Event
}

func New() *Store {
	s := &Store{devices: make(map[uint32]*deviceState), ambient: 220, revision: 1}
	for _, d := range []struct {
		eoj        uint32
		name, kind string
	}{
		{Light, "Ceiling light", "light"}, {Aircon, "Air conditioner", "aircon"}, {Sensor, "Temperature sensor", "sensor"},
	} {
		defs := []Definition{{0x80, "Operation status", d.eoj != Sensor, ""}, {0x88, "Fault status", false, ""}, {0x8A, "Experimental manufacturer", false, ""}, {0x9D, "Announcement map (empty)", false, ""}, {0x9E, "Set map", false, ""}, {0x9F, "Get map", false, ""}}
		data := map[byte][]byte{0x80: {0x31}, 0x88: {0x42}, 0x8A: {0xFF, 0xFF, 0xFF}}
		switch d.eoj {
		case Light:
			defs = append(defs, Definition{0xB0, "Brightness (%)", true, ""})
			data[0xB0] = []byte{60}
		case Aircon:
			defs = append(defs, Definition{0xB0, "Mode (cool/heat/fan)", true, ""}, Definition{0xB3, "Setpoint (16..30 C)", true, ""}, Definition{0xBB, "Room temperature (signed C)", false, ""})
			data[0xB0] = []byte{0x42}
			data[0xB3] = []byte{24}
		case Sensor:
			defs = append(defs, Definition{0xE0, "Temperature (signed 0.1 C)", false, ""})
			data[0x80] = []byte{0x30}
		}
		s.devices[d.eoj] = &deviceState{d.name, d.kind, defs, data}
	}
	s.logLocked("INFO", "system", "DEMO: in-memory frames; no network or hardware", nil)
	return s
}

func (s *Store) definition(eoj uint32, epc byte) (*deviceState, Definition, error) {
	d, ok := s.devices[eoj]
	if !ok {
		return nil, Definition{}, fmt.Errorf("unknown EOJ %06X", eoj)
	}
	for _, def := range d.definitions {
		if def.EPC == epc {
			return d, def, nil
		}
	}
	return nil, Definition{}, fmt.Errorf("unsupported EPC %02X", epc)
}

func (s *Store) valueLocked(eoj uint32, epc byte) ([]byte, error) {
	d, _, err := s.definition(eoj, epc)
	if err != nil {
		return nil, err
	}
	switch epc {
	case 0x9D:
		return []byte{0}, nil // No notification service is implemented.
	case 0x9E, 0x9F:
		codes := []byte{}
		for _, def := range d.definitions {
			if epc == 0x9F || def.Writable {
				codes = append(codes, def.EPC)
			}
		}
		slices.Sort(codes)
		// Current explicit profiles all fit Format 1. Never inherit the full MRA.
		if len(codes) >= 16 {
			return nil, fmt.Errorf("prototype profile exceeds Format 1 limit")
		}
		return append([]byte{byte(len(codes))}, codes...), nil
	case 0xE0:
		b := make([]byte, 2)
		binary.BigEndian.PutUint16(b, uint16(int16(s.ambient)))
		return b, nil
	case 0xBB:
		return []byte{byte(int8(math.Round(float64(s.ambient) / 10)))}, nil
	}
	return slices.Clone(d.data[epc]), nil
}

func (s *Store) Read(eoj uint32, epc byte) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.valueLocked(eoj, epc)
}

func (s *Store) Write(eoj uint32, epc byte, data []byte, source string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, def, err := s.definition(eoj, epc)
	if err != nil {
		return err
	}
	if !def.Writable {
		return fmt.Errorf("EPC %02X is read-only", epc)
	}
	if len(data) != 1 {
		return fmt.Errorf("EPC %02X needs one byte", epc)
	}
	valid := false
	switch epc {
	case 0x80:
		valid = data[0] == 0x30 || data[0] == 0x31
	case 0xB0:
		if eoj == Light {
			valid = data[0] <= 100
		} else {
			valid = data[0] == 0x42 || data[0] == 0x43 || data[0] == 0x45
		}
	case 0xB3:
		valid = data[0] >= 16 && data[0] <= 30
	}
	if !valid {
		return fmt.Errorf("unsupported value %X for EPC %02X", data, epc)
	}
	if !slices.Equal(d.data[epc], data) {
		d.data[epc] = slices.Clone(data)
		s.revision++
		s.logLocked("STATE", source, fmt.Sprintf("%06X / %02X = %X", eoj, epc, data), nil)
	}
	return nil
}

// InjectAmbient is a local scenario input, never an ECHONET write to the sensor.
func (s *Store) InjectAmbient(tenths int, source string) error {
	if tenths < -200 || tenths > 500 {
		return fmt.Errorf("ambient range is -20.0..50.0 C")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ambient != tenths {
		s.ambient = tenths
		s.revision++
		s.logLocked("INPUT", source, fmt.Sprintf("ambient %.1f C", float64(tenths)/10), nil)
	}
	return nil
}
func (s *Store) logLocked(kind, source, message string, frame []byte) {
	s.sequence++
	s.events = append(s.events, Event{s.sequence, time.Now().Format("15:04:05"), kind, source, message, hex.EncodeToString(frame)})
	if len(s.events) > 100 {
		s.events = slices.Clone(s.events[len(s.events)-100:])
	}
}
func (s *Store) Log(kind, source, message string, frame []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logLocked(kind, source, message, frame)
}
func (s *Store) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap := Snapshot{Revision: s.revision, Room: "Living room", AmbientTenths: s.ambient, Events: slices.Clone(s.events)}
	for _, eoj := range []uint32{Light, Aircon, Sensor} {
		d := s.devices[eoj]
		v := Device{EOJ: eoj, Name: d.name, Kind: d.kind, Power: d.data[0x80][0] == 0x30}
		if eoj == Light {
			v.Level = int(d.data[0xB0][0])
		}
		if eoj == Aircon {
			v.Target = int(d.data[0xB3][0])
			v.Mode = map[byte]string{0x42: "cool", 0x43: "heat", 0x45: "fan"}[d.data[0xB0][0]]
		}
		for _, def := range d.definitions {
			b, _ := s.valueLocked(eoj, def.EPC)
			def.Value = hex.EncodeToString(b)
			v.Properties = append(v.Properties, def)
		}
		snap.Devices = append(snap.Devices, v)
	}
	return snap
}
