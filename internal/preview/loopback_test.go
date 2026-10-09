package preview

import (
	"bufio"
	"context"
	"encoding/json"
	"github.com/cybergarage/uecho-simulator/internal/model"
	"github.com/cybergarage/uecho-simulator/internal/wire"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// Opt-in socket integration binds only 127.0.0.1; default CI uses injected frames.
func TestLoopbackControllerToDisplay(t *testing.T) {
	if os.Getenv("SIMULATOR_LOOPBACK_TEST") != "1" {
		t.Skip("explicit loopback socket test opt-in")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store := model.New()
	udp, err := wire.Listen("127.0.0.1:0", wire.New(store))
	if err != nil {
		t.Fatal(err)
	}
	defer udp.Close()
	listener, err := Listen("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan error, 2)
	go func() { done <- udp.Serve(ctx) }()
	go func() { done <- Serve(ctx, listener, (&Server{Store: store, UDPAddress: udp.Address()}).Handler()) }()
	client := &http.Client{Timeout: 3 * time.Second}
	res, err := client.Get("http://" + listener.Addr().String() + "/events")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	scanner := bufio.NewScanner(res.Body)
	next := func() model.Snapshot {
		for scanner.Scan() {
			if strings.HasPrefix(scanner.Text(), "data: ") {
				var data struct {
					Snapshot model.Snapshot `json:"snapshot"`
				}
				if err := json.Unmarshal([]byte(strings.TrimPrefix(scanner.Text(), "data: ")), &data); err != nil {
					t.Fatal(err)
				}
				return data.Snapshot
			}
		}
		t.Fatal("stream ended", scanner.Err())
		return model.Snapshot{}
	}
	if next().Revision != 1 {
		t.Fatal("initial")
	}
	receiver, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: wire.Port})
	if err != nil {
		t.Fatal(err)
	}
	defer receiver.Close()
	conn, err := net.Dial("udp4", udp.Address())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = receiver.SetDeadline(time.Now().Add(2 * time.Second))
	for _, level := range []byte{15, 85} {
		frame := []byte{0x10, 0x81, 0, 1, 5, 0xff, 1, 2, 0x90, 1, 0x61, 1, 0xb0, 1, level}
		if _, err := conn.Write(frame); err != nil {
			t.Fatal(err)
		}
		b := make([]byte, 1024)
		for {
			n, _, err := receiver.ReadFromUDP(b)
			if err != nil {
				t.Fatal("SetC response", n, err)
			}
			if n >= 12 && b[10] == 0x71 {
				break
			}
		}
		for {
			s := next()
			if s.Devices[0].Level == int(level) {
				break
			}
		}
	}
	res.Body.Close()
	cancel()
	for i := 0; i < 2; i++ {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(time.Second):
			t.Fatal("server shutdown")
		}
	}
}
