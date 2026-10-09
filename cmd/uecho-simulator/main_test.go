package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOfflineDemoAndInteractive(t *testing.T) {
	var out bytes.Buffer
	path := filepath.Join(t.TempDir(), "room.svg")
	if err := run([]string{"--demo", "--plain", "--preview", path}, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "OFFLINE") || !strings.Contains(out.String(), "brightness=75%") {
		t.Fatal(out.String())
	}
	data, err := os.ReadFile(path)
	if err != nil || !bytes.Contains(data, []byte(`width="800"`)) {
		t.Fatal("missing preview")
	}
	out.Reset()
	if err := run([]string{"--plain"}, strings.NewReader("light on\nlight 101\nac heat\nevents\nquit\n"), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "ERROR") || !strings.Contains(out.String(), "mode=heat") {
		t.Fatal(out.String())
	}
	for _, args := range [][]string{{"--demo", "--udp", "127.0.0.1:3610"}, {"--udp", "192.168.1.2:3610"}, {"--display", "0.0.0.0:8080"}, {"--display", "127.0.0.1:8080", "--demo"}, {"--allow-lan"}, {"extra"}} {
		if err := run(args, strings.NewReader(""), &out); err == nil {
			t.Fatal("accepted invalid options")
		}
	}
}
