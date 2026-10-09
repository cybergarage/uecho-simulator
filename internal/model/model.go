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
	Node   uint32 = 0x0EF001
	Light  uint32 = 0x029001
	Aircon uint32 = 0x013001
	Sensor uint32 = 0x001101
)

// Definition declares the implemented property access and announcement behavior.
type Definition struct {
	EPC      byte   `json:"epc"`
	Name     string `json:"name"`
	Writable bool   `json:"writable"`
	Value    string `json:"value"`
	Announce bool   `json:"announce"`
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

// TargetLabel preserves the protocol's undefined setpoint sentinel in every UI.
func (d Device) TargetLabel() string {
	if d.Target == 0xfd {
		return "UNDEFINED"
	}
	return fmt.Sprintf("%d C", d.Target)
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
	LastReceived  string   `json:"lastReceived"`
	LastUpdated   string   `json:"lastUpdated"`
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
	subscribers        map[chan struct{}]struct{}
	lastReceived       string
	lastUpdated        string
	notifications      map[chan Change]struct{}
}

func New() *Store { return newProfiles() }

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
	case 0x9D, 0x9E, 0x9F:
		codes := []byte{}
		for _, def := range d.definitions {
			if epc == 0x9F || epc == 0x9E && def.Writable || epc == 0x9D && def.Announce {
				codes = append(codes, def.EPC)
			}
		}
		slices.Sort(codes)
		// Maps describe the implemented profile, using Format 2 when needed.
		return propertyMap(codes), nil
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
	if len(data) == 0 {
		return fmt.Errorf("EPC %02X needs data", epc)
	}
	valid := validValue(eoj, epc, data)

	if !valid {
		return fmt.Errorf("unsupported value %X for EPC %02X", data, epc)
	}
	if !slices.Equal(d.data[epc], data) {
		d.data[epc] = slices.Clone(data)
		s.revision++
		s.logLocked("STATE", source, fmt.Sprintf("%06X / %02X = %X", eoj, epc, data), nil)
		s.notifyLocked(eoj, epc)
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
		s.notifyLocked(Sensor, 0xE0)
		s.notifyLocked(Aircon, 0xBB)
	}
	return nil
}
func (s *Store) logLocked(kind, source, message string, frame []byte) {
	s.sequence++
	if kind == "RX" {
		s.lastReceived = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if kind == "STATE" || kind == "INPUT" {
		s.lastUpdated = time.Now().UTC().Format(time.RFC3339Nano)
	}
	for ch := range s.subscribers {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
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
	snap := Snapshot{Revision: s.revision, Room: "Living room", AmbientTenths: s.ambient, Events: slices.Clone(s.events), LastReceived: s.lastReceived, LastUpdated: s.lastUpdated}
	for _, eoj := range []uint32{Light, Aircon, Sensor} {
		d := s.devices[eoj]
		v := Device{EOJ: eoj, Name: d.name, Kind: d.kind, Power: d.data[0x80][0] == 0x30}
		if eoj == Light {
			v.Level = int(d.data[0xB0][0])
		}
		if eoj == Aircon {
			v.Target = int(d.data[0xB3][0])
			v.Mode = map[byte]string{0x40: "other", 0x41: "auto", 0x42: "cool", 0x43: "heat", 0x44: "dry", 0x45: "fan"}[d.data[0xB0][0]]
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

// Subscribe coalesces notifications; consumers read the latest detached snapshot.
// Register before reading an initial snapshot to avoid missing concurrent writes.
func (s *Store) Subscribe() (<-chan struct{}, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.subscribers == nil {
		s.subscribers = make(map[chan struct{}]struct{})
	}
	ch := make(chan struct{}, 1)
	s.subscribers[ch] = struct{}{}
	var once sync.Once
	return ch, func() { once.Do(func() { s.mu.Lock(); defer s.mu.Unlock(); delete(s.subscribers, ch); close(ch) }) }
}
