package main

import (
	"bytes"
	"context"
	"fmt"
	"github.com/cybergarage/uecho-simulator/internal/wire"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestNetworkSelection(t *testing.T) {
	originalDiscover, originalChoose := discoverInterfaces, chooseInterface
	defer func() { discoverInterfaces, chooseInterface = originalDiscover, originalChoose }()
	candidates := []networkInterface{{"en0", []string{"192.0.2.10", "192.0.2.11"}}}
	discoverInterfaces = func() ([]networkInterface, error) { return candidates, nil }
	address, name, err := networkDefaults("")
	if err != nil || address != "192.0.2.10:3610" || name != "en0" {
		t.Fatal(address, name, err)
	}
	candidates = append(candidates, networkInterface{"en1", []string{"198.51.100.1"}})
	chooseInterface = func(c []networkInterface) (networkInterface, error) { return c[1], nil }
	_, name, err = networkDefaults("")
	if err != nil || name != "en1" {
		t.Fatal(name, err)
	}
	_, name, err = networkDefaults("en0")
	if err != nil || name != "en0" {
		t.Fatal(name, err)
	}
	if _, _, err = networkDefaults("missing"); err == nil {
		t.Fatal("missing interface accepted")
	}
	candidates = nil
	if _, _, err = networkDefaults(""); err == nil || !strings.Contains(err.Error(), "--offline") {
		t.Fatal(err)
	}
	discoverInterfaces = func() ([]networkInterface, error) { t.Fatal("offline/help enumerated network"); return nil, nil }
	for _, args := range [][]string{{"--help"}, {"--offline", "--plain"}, {"--demo"}} {
		if err := run(args, strings.NewReader(""), &bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
	}
}
func TestInterfaceKeyboard(t *testing.T) {
	candidates := []networkInterface{{"en0", []string{"192.0.2.10"}}, {"en1", []string{"198.51.100.10"}}}
	for _, test := range []struct {
		keys   []tcell.Key
		name   string
		cancel bool
	}{{[]tcell.Key{tcell.KeyEnter}, "en0", false}, {[]tcell.Key{tcell.KeyDown, tcell.KeyEnter}, "en1", false}, {[]tcell.Key{tcell.KeyEscape}, "", true}} {
		screen := tcell.NewSimulationScreen("UTF-8")
		ready := make(chan struct{})
		wrapped := &readyScreen{Screen: screen, ready: ready}
		done := make(chan struct{})
		go func() {
			defer close(done)
			<-ready
			for _, key := range test.keys {
				screen.PostEventWait(tcell.NewEventKey(key, 0, tcell.ModNone))
			}
		}()
		selected, err := selectInterface(wrapped, candidates)
		<-done
		if (err != nil) != test.cancel || selected.name != test.name {
			t.Fatal(selected, err)
		}
	}
}

type readyScreen struct {
	tcell.Screen
	ready chan struct{}
}

func (s *readyScreen) Init() error { err := s.Screen.Init(); close(s.ready); return err }

type fakeTransport struct{ closed bool }

func (f *fakeTransport) Address() string                 { return "192.0.2.10:3610" }
func (f *fakeTransport) Serve(ctx context.Context) error { <-ctx.Done(); return nil }
func (f *fakeTransport) Close() error                    { f.closed = true; return nil }
func TestDefaultPlainNetworkAndRollback(t *testing.T) {
	originalDiscover, originalListen := discoverInterfaces, listenTransport
	defer func() { discoverInterfaces, listenTransport = originalDiscover, originalListen }()
	discoverInterfaces = func() ([]networkInterface, error) {
		return []networkInterface{{"simtest0", []string{"192.0.2.10"}}}, nil
	}
	fake := &fakeTransport{}
	listenTransport = func(address string, engine *wire.Engine, allowLAN bool, name string) (transport, error) {
		if address != "192.0.2.10:3610" || !allowLAN || name != "simtest0" {
			t.Fatal(address, allowLAN, name)
		}
		return fake, nil
	}
	if err := run([]string{"--plain"}, strings.NewReader("quit\n"), &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if !fake.closed {
		t.Fatal("transport leaked")
	}
	fake = &fakeTransport{}
	if err := run([]string{"--plain", "--preview", t.TempDir()}, strings.NewReader(""), &bytes.Buffer{}); err == nil {
		t.Fatal("export should fail")
	}
	if !fake.closed {
		t.Fatal("transport leaked on startup failure")
	}
	listenTransport = func(string, *wire.Engine, bool, string) (transport, error) {
		return nil, fmt.Errorf("bind 3610: address already in use")
	}
	if err := run([]string{"--plain"}, strings.NewReader(""), &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "3610") {
		t.Fatal(err)
	}
}
