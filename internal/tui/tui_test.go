package tui

import (
	"bytes"
	"testing"

	"github.com/cybergarage/uecho-simulator/internal/model"
	"github.com/cybergarage/uecho-simulator/internal/wire"
)

func TestScenarioAndCommands(t *testing.T) {
	s := model.New()
	u := UI{Store: s, Engine: wire.New(s), Plain: true}
	var out bytes.Buffer
	if err := u.Demo(); err != nil {
		t.Fatal(err)
	}
	snap := s.Snapshot()
	if !snap.Devices[0].Power || snap.Devices[0].Level != 75 || !snap.Devices[1].Power || snap.AmbientTenths != 265 {
		t.Fatalf("scenario state %+v", snap)
	}
	for _, cmd := range []string{"light off", "ac heat", "ac 21", "temp -12.5", "events", "help"} {
		if _, err := u.Command(cmd, &out); err != nil {
			t.Fatal(err)
		}
	}
	for _, cmd := range []string{"ac undefined", "ac 253", "light 101", "light cool", "ac 51", "temp NaN", "temp +Inf", "temp 22.22", "temp 51", "unknown", "quit extra"} {
		if _, err := u.Command(cmd, &out); err == nil {
			t.Fatalf("accepted %q", cmd)
		}
	}
	u.Draw(&out)
	if bytes.Contains(out.Bytes(), []byte("\x1b")) {
		t.Fatal("plain output contains escapes")
	}
	if quit, err := u.Command("quit", &out); !quit || err != nil {
		t.Fatal("quit failed")
	}
}

func TestProtocolSetpointRangeAndUndefinedDisplay(t *testing.T) {
	s := model.New()
	u := UI{Store: s, Engine: wire.New(s), Plain: true}
	var out bytes.Buffer
	for _, cmd := range []string{"ac 0", "ac 50", "ac auto", "ac dry", "ac other"} {
		if _, err := u.Command(cmd, &out); err != nil {
			t.Fatal(cmd, err)
		}
	}
	u.Draw(&out)
	if !bytes.Contains(out.Bytes(), []byte("target=50 C")) {
		t.Fatal(out.String())
	}
	if (model.Device{Target: 0xfd}).TargetLabel() != "UNDEFINED" {
		t.Fatal("sentinel display")
	}
}
