package preview

import (
	"context"
	"github.com/cybergarage/uecho-simulator/internal/model"
	"github.com/gdamore/tcell/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTerminalKeysAndShutdown(t *testing.T) {
	for _, mode := range []string{"keys", "cancel", "ctrl-c"} {
		t.Run(mode, func(t *testing.T) {
			screen := tcell.NewSimulationScreen("UTF-8")
			store := model.New()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			path := filepath.Join(t.TempDir(), "room.svg")
			done := make(chan error, 1)
			observed := &observedScreen{screen, make(chan string, 20)}
			go func() { done <- Terminal(ctx, observed, store, "http://127.0.0.1:8080", path) }()
			wait := func(want string) {
				t.Helper()
				deadline := time.After(2 * time.Second)
				for {
					select {
					case value := <-observed.frames:
						if strings.Contains(value, want) {
							return
						}
					case <-deadline:
						t.Fatal("missing screen text: " + want)
					}
				}
			}
			wait("UECHO")
			if mode == "cancel" {
				cancel()
			} else if mode == "ctrl-c" {
				screen.InjectKey(tcell.KeyCtrlC, 0, 0)
			} else {
				screen.InjectKey(tcell.KeyRune, '?', 0)
				wait("stop")
				screen.InjectKey(tcell.KeyRune, 'r', 0)
				wait("redrawn")
				screen.InjectKey(tcell.KeyRune, 's', 0)
				wait("Saved")
				b, err := os.ReadFile(path)
				if err != nil || !strings.Contains(string(b), `width="800"`) {
					t.Fatal("save failed", err)
				}
				screen.InjectKey(tcell.KeyRune, 'q', 0)
			}
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("shutdown blocked")
			}
			if store.Snapshot().Revision != 1 {
				t.Fatal("display keys changed devices")
			}
		})
	}
}

type observedScreen struct {
	tcell.SimulationScreen
	frames chan string
}

func (s *observedScreen) Show() {
	s.SimulationScreen.Show()
	cells, _, _ := s.GetContents()
	var b strings.Builder
	for _, c := range cells {
		b.WriteString(string(c.Runes))
	}
	s.frames <- b.String()
}
