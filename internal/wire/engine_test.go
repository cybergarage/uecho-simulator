package wire

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/cybergarage/uecho-simulator/internal/model"
)

func frame(t *testing.T, s string) []byte {
	t.Helper()
	b, e := hex.DecodeString(s)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func TestKnownFrames(t *testing.T) {
	e := New(model.New())
	for _, c := range []struct{ request, want string }{
		{"1081123405ff0102900162018000", "1081123402900105ff017201800131"},
		{"1081123505ff010290016101800130", "1081123502900105ff0171018000"},
		{"1081123605ff0102900162018000", "1081123602900105ff017201800130"},
		{"1081123705ff010290016101b00165", "1081123702900105ff015101b00165"},
		{"1081123805ff010290016201ee00", "1081123802900105ff015201ee00"},
		{"1081123905ff010290016201800131", "1081123902900105ff0152018000"},
		{"1081124005ff010011016201e000", "1081124000110105ff017201e00200dc"},
		{"1081124105ff0102900162028000b000", "1081124102900105ff017202800130b0013c"},
	} {
		got, err := e.Handle(frame(t, c.request), "fixture")
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, frame(t, c.want)) {
			t.Fatalf("%s: got %x want %s", c.request, got, c.want)
		}
	}
}
func TestMalformedFrames(t *testing.T) {
	valid := frame(t, "1081123405ff0102900162018000")
	e := New(model.New())
	for n := 0; n < len(valid); n++ {
		if _, err := e.Handle(valid[:n], "truncated"); err == nil {
			t.Fatalf("accepted length %d", n)
		}
	}
	bad := [][]byte{append(bytes.Clone(valid), 0), bytes.Repeat([]byte{1}, 1025)}
	for _, pos := range []int{0, 1, 10, 11, 13} {
		b := bytes.Clone(valid)
		b[pos] = 0xff
		bad = append(bad, b)
	}
	for _, b := range bad {
		if _, err := e.Handle(b, "bad"); err == nil {
			t.Fatalf("accepted %x", b)
		}
	}
	b := bytes.Clone(valid)
	b[7] = 0x99
	if out, err := e.Handle(b, "unknown"); err != nil || out != nil {
		t.Fatalf("unknown device: %x %v", out, err)
	}
}
func TestRequestAndPropertyMaps(t *testing.T) {
	e := New(model.New())
	for _, eoj := range []uint32{model.Light, model.Aircon, model.Sensor} {
		get, err := e.Request(eoj, 0x9f, nil, false, "test")
		if err != nil {
			t.Fatal(err)
		}
		if int(get[0]) != len(get)-1 || get[0] >= 16 {
			t.Fatalf("invalid explicit map %x", get)
		}
		for _, epc := range get[1:] {
			if _, err := e.Request(eoj, epc, nil, false, "test"); err != nil {
				t.Fatal(err)
			}
		}
		set, err := e.Request(eoj, 0x9e, nil, false, "test")
		if err != nil {
			t.Fatal(err)
		}
		for _, epc := range set[1:] {
			if !bytes.Contains(get[1:], []byte{epc}) {
				t.Fatalf("writable code absent from get map")
			}
		}
	}
	if _, err := e.Request(model.Sensor, 0xe0, []byte{1}, true, "test"); err == nil {
		t.Fatal("sensor accepted write")
	}
}
func FuzzHandle(f *testing.F) {
	for _, b := range [][]byte{{}, {0x10, 0x81}, {0x10, 0x81, 0, 1, 5, 255, 1, 2, 144, 1, 0x62, 1, 0x80, 0}} {
		f.Add(b)
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 2048 {
			return
		}
		New(model.New()).Handle(b, "fuzz")
	})
}
