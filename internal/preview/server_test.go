package preview

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/cybergarage/uecho-simulator/internal/model"
	"github.com/cybergarage/uecho-simulator/internal/wire"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type streamWriter struct {
	header http.Header
	frames chan []byte
}

func (w *streamWriter) Header() http.Header         { return w.header }
func (w *streamWriter) WriteHeader(int)             {}
func (w *streamWriter) Flush()                      {}
func (w *streamWriter) Write(b []byte) (int, error) { w.frames <- bytes.Clone(b); return len(b), nil }
func nextSnapshot(t *testing.T, w *streamWriter) model.Snapshot {
	t.Helper()
	select {
	case b := <-w.frames:
		var v struct {
			Snapshot model.Snapshot `json:"snapshot"`
		}
		if err := json.Unmarshal(bytes.TrimSpace(bytes.TrimPrefix(b, []byte("data: "))), &v); err != nil {
			t.Fatal(err)
		}
		return v.Snapshot
	case <-time.After(2 * time.Second):
		t.Fatal("missing event")
		return model.Snapshot{}
	}
}
func TestStreamChangesReconnectAndCancel(t *testing.T) {
	store := model.New()
	page := &Server{Store: store}
	engine := wire.New(store)
	connect := func() (*streamWriter, context.CancelFunc, <-chan struct{}) {
		ctx, cancel := context.WithCancel(context.Background())
		w := &streamWriter{http.Header{}, make(chan []byte, 100)}
		done := make(chan struct{})
		go func() {
			defer close(done)
			page.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/events", nil).WithContext(ctx))
		}()
		return w, cancel, done
	}
	w, cancel, done := connect()
	initial := nextSnapshot(t, w)
	if initial.LastReceived != "" || initial.LastUpdated != "" {
		t.Fatal("fabricated timestamps")
	}
	for i, level := range []byte{10, 90, 33} {
		frame := []byte{0x10, 0x81, 0, byte(i + 1), 5, 0xff, 1, 2, 0x90, 1, 0x61, 1, 0xb0, 1, level}
		if _, err := engine.Handle(frame, "isolated-controller"); err != nil {
			t.Fatal(err)
		}
		deadline := time.After(2 * time.Second)
		for {
			select {
			case <-deadline:
				t.Fatal("missing state update")
			default:
			}
			s := nextSnapshot(t, w)
			if s.Devices[0].Level == int(level) {
				if s.LastReceived == "" || s.LastUpdated == "" {
					t.Fatal("missing timestamps")
				}
				break
			}
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stream did not close")
	}
	w, cancel, done = connect()
	if s := nextSnapshot(t, w); s.Devices[0].Level != 33 {
		t.Fatal("reconnect lost state")
	}
	cancel()
	<-done
}
func TestPageReadOnlyAndPolicy(t *testing.T) {
	h := (&Server{Store: model.New()}).Handler()
	for _, path := range []string{"/", "/events"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("POST", path, nil))
		if w.Code != 405 {
			t.Fatal(w.Code)
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	for _, text := range []string{"EventSource('/events')", "DISPLAY DISCONNECTED", "CONTROLLER: NOT RECEIVED", "font-weight:800", "width:800px;height:480px"} {
		if !strings.Contains(w.Body.String(), text) {
			t.Fatal(text)
		}
	}
	for _, address := range []string{"0.0.0.0:8080", "192.168.1.2:8080"} {
		if _, err := Listen(address); err == nil {
			t.Fatal("non-loopback HTTP")
		}
	}
}
