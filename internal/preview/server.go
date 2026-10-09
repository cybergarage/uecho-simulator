// Package preview serves a read-only room display and event-driven snapshots.
package preview

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/cybergarage/uecho-simulator/internal/model"
	"github.com/cybergarage/uecho-simulator/internal/wire"
)

//go:embed room.html
var assets embed.FS

type Server struct {
	Store      *model.Store
	UDPAddress string
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; connect-src 'self'; img-src 'self' data:")
		b, _ := assets.ReadFile("room.html")
		_, _ = w.Write(b)
	})
	mux.HandleFunc("/events", s.events)
	return mux
}
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	f, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "stream unavailable", 500)
		return
	}
	changes, unsubscribe := s.Store.Subscribe()
	defer unsubscribe()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	send := func() bool {
		payload := struct {
			Snapshot   model.Snapshot `json:"snapshot"`
			UDPAddress string         `json:"udpAddress"`
		}{s.Store.Snapshot(), s.UDPAddress}
		b, err := json.Marshal(payload)
		if err != nil {
			return false
		}
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(5 * time.Second))
		if _, err = fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
			return false
		}
		f.Flush()
		return true
	}
	if !send() {
		return
	}
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case _, ok := <-changes:
			if !ok || !send() {
				return
			}
		case <-heartbeat.C:
			_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(5 * time.Second))
			if _, err := fmt.Fprint(w, ": heartbeat\n\n"); err != nil {
				return
			}
			f.Flush()
		}
	}
}

// Listen always binds literal IPv4 loopback; browser access never exposes LAN.
func Listen(address string) (net.Listener, error) {
	if err := wire.ValidateAddress(address); err != nil {
		return nil, err
	}
	return net.Listen("tcp4", address)
}
func Serve(ctx context.Context, listener net.Listener, handler http.Handler) error {
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = server.Close()
		case <-done:
		}
	}()
	err := server.Serve(listener)
	if err == http.ErrServerClosed || ctx.Err() != nil {
		return nil
	}
	return err
}
