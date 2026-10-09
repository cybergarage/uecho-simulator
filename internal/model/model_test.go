package model

import (
	"encoding/binary"
	"sync"
	"testing"
)

func TestValuesAndIsolation(t *testing.T) {
	s := New()
	if err := s.InjectAmbient(-125, "test"); err != nil {
		t.Fatal(err)
	}
	b, err := s.Read(Sensor, 0xe0)
	if err != nil || int16(binary.BigEndian.Uint16(b)) != -125 {
		t.Fatalf("temperature %x %v", b, err)
	}
	b[0] = 99
	c, _ := s.Read(Sensor, 0xe0)
	if c[0] == 99 {
		t.Fatal("read leaked state")
	}
	snap := s.Snapshot()
	snap.Devices[0].Properties[0].Name = "changed"
	snap.Events[0].Message = "changed"
	fresh := s.Snapshot()
	if fresh.Devices[0].Properties[0].Name == "changed" || fresh.Events[0].Message == "changed" {
		t.Fatal("snapshot leaked state")
	}
	before := fresh.Revision
	for _, c := range []struct {
		eoj  uint32
		epc  byte
		data []byte
	}{{Light, 0xb0, []byte{101}}, {Aircon, 0xb3, []byte{15}}, {Aircon, 0xb0, []byte{0}}, {Sensor, 0xe0, []byte{1}}, {Light, 0x80, nil}, {0, 0x80, []byte{0x30}}} {
		if err := s.Write(c.eoj, c.epc, c.data, "bad"); err == nil {
			t.Fatal("accepted invalid write")
		}
	}
	if s.Snapshot().Revision != before {
		t.Fatal("rejected write changed revision")
	}
	if err := s.InjectAmbient(501, "bad"); err == nil {
		t.Fatal("accepted invalid ambient")
	}
}
func TestConcurrentStateAndBoundedEvents(t *testing.T) {
	s := New()
	var wg sync.WaitGroup
	for n := 0; n < 8; n++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				s.Write(Light, 0xb0, []byte{byte((n + i) % 101)}, "race")
				s.Read(Light, 0xb0)
				s.Snapshot()
				s.Log("TEST", "race", "entry", nil)
			}
		}(n)
	}
	wg.Wait()
	events := s.Snapshot().Events
	if len(events) != 100 {
		t.Fatalf("event bound %d", len(events))
	}
	for i := 1; i < len(events); i++ {
		if events[i].Sequence != events[i-1].Sequence+1 {
			t.Fatal("sequence gap")
		}
	}
}

func TestSubscriptionsCoalesceAndUnsubscribe(t *testing.T) {
	store := New()
	changes, unsubscribe := store.Subscribe()
	for i := 0; i < 200; i++ {
		store.Log("DISPLAY", "test", "refresh", nil)
	}
	select {
	case <-changes:
	default:
		t.Fatal("lost change")
	}
	select {
	case <-changes:
		t.Fatal("notifications not coalesced")
	default:
	}
	unsubscribe()
	unsubscribe()
	store.Log("DISPLAY", "test", "after unsubscribe", nil)
	if _, ok := <-changes; ok {
		t.Fatal("subscription not closed")
	}
}
