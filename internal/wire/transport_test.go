package wire

import (
	"bytes"
	"context"
	"github.com/cybergarage/uecho-simulator/internal/model"
	"net"
	"sync"
	"testing"
	"time"
)

type fakeDatagram struct {
	data []byte
	addr *net.UDPAddr
}
type memoryUDP struct {
	input, output chan fakeDatagram
	closed        chan struct{}
	once          sync.Once
}

func newMemoryUDP() *memoryUDP {
	return &memoryUDP{make(chan fakeDatagram, 32), make(chan fakeDatagram, 32), make(chan struct{}), sync.Once{}}
}
func (c *memoryUDP) LocalAddr() net.Addr {
	return &net.UDPAddr{IP: net.ParseIP("192.0.2.10"), Port: Port}
}
func (c *memoryUDP) ReadFromUDP(b []byte) (int, *net.UDPAddr, error) {
	select {
	case d := <-c.input:
		return copy(b, d.data), d.addr, nil
	case <-c.closed:
		return 0, nil, net.ErrClosed
	}
}
func (c *memoryUDP) WriteToUDP(b []byte, a *net.UDPAddr) (int, error) {
	clone := *a
	clone.IP = append(net.IP(nil), a.IP...)
	select {
	case c.output <- fakeDatagram{append([]byte(nil), b...), &clone}:
		return len(b), nil
	case <-c.closed:
		return 0, net.ErrClosed
	}
}
func (c *memoryUDP) SetWriteDeadline(time.Time) error { return nil }
func (c *memoryUDP) Close() error                     { c.once.Do(func() { close(c.closed) }); return nil }
func receiveDatagram(t *testing.T, c *memoryUDP) fakeDatagram {
	t.Helper()
	select {
	case d := <-c.output:
		return d
	case <-time.After(time.Second):
		t.Fatal("no response")
		return fakeDatagram{}
	}
}
func startMemoryServer(t *testing.T, s *UDP) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Serve(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(time.Second):
			t.Error("shutdown did not close both readers")
		}
	})
}
func TestInjectedMulticastControllerDevice(t *testing.T) {
	store := model.New()
	unicast, group := newMemoryUDP(), newMemoryUDP()
	server := &UDP{conn: unicast, multicast: group, engine: New(store)}
	startMemoryServer(t, server)
	startup := receiveDatagram(t, unicast)
	if startup.addr.String() != MulticastIPv4+":3610" || startup.data[10] != 0x73 || startup.data[12] != 0xd5 || !bytes.Equal(startup.data[4:10], frame(t, "0ef0010ef001")) {
		t.Fatalf("startup %x to %v", startup.data, startup.addr)
	}
	controller := &net.UDPAddr{IP: net.ParseIP("192.0.2.20"), Port: 49152}
	group.input <- fakeDatagram{frame(t, "1081123405ff010ef0006201d600"), controller}
	discovery := receiveDatagram(t, unicast)
	if discovery.addr.String() != "192.0.2.20:3610" || discovery.data[10] != 0x72 || discovery.data[6] != 1 || !bytes.Equal(discovery.data[14:], frame(t, "03029001013001001101")) {
		t.Fatalf("discovery %x to %v", discovery.data, discovery.addr)
	}
	unicast.input <- fakeDatagram{frame(t, "1081123505ff010290016102800130b0014b"), controller}
	reply := receiveDatagram(t, unicast)
	if reply.data[10] != 0x71 || reply.addr.Port != Port {
		t.Fatalf("SetC %x %v", reply.data, reply.addr)
	}
	for _, epc := range []byte{0x80, 0xb0} {
		d := receiveDatagram(t, unicast)
		if d.data[10] != 0x73 || d.data[12] != epc || d.addr.String() != MulticastIPv4+":3610" {
			t.Fatalf("announcement %x %v", d.data, d.addr)
		}
	}
	unicast.input <- fakeDatagram{frame(t, "1081123605ff0102900163018000"), controller}
	inf := receiveDatagram(t, unicast)
	if inf.data[10] != 0x73 || inf.addr.String() != MulticastIPv4+":3610" {
		t.Fatalf("INF_REQ broadcast %x %v", inf.data, inf.addr)
	}
	unicast.input <- fakeDatagram{frame(t, "1081123705ff010290016301ee00"), controller}
	sna := receiveDatagram(t, unicast)
	if sna.data[10] != 0x53 || sna.addr.String() != "192.0.2.20:3610" {
		t.Fatalf("INF_REQ failure %x %v", sna.data, sna.addr)
	}
	if err := store.InjectAmbient(-125, "test"); err != nil {
		t.Fatal(err)
	}
	for _, epc := range []byte{0xe0, 0xbb} {
		d := receiveDatagram(t, unicast)
		if d.data[12] != epc || d.addr.String() != MulticastIPv4+":3610" {
			t.Fatalf("ambient %x %v", d.data, d.addr)
		}
	}
}
func TestInjectedUnicastFixedReplyPortAndNotifications(t *testing.T) {
	store := model.New()
	conn := newMemoryUDP()
	server := &UDP{conn: conn, engine: New(store)}
	startMemoryServer(t, server)
	conn.input <- fakeDatagram{frame(t, "1081123405ff0102900062018000"), &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 49152}}
	reply := receiveDatagram(t, conn)
	if reply.addr.String() != "127.0.0.1:3610" || reply.data[10] != 0x72 {
		t.Fatalf("reply %x %v", reply.data, reply.addr)
	}
	for _, v := range []byte{0x30, 0x31, 0x30} {
		if err := store.Write(model.Light, 0x80, []byte{v}, "test"); err != nil {
			t.Fatal(err)
		}
	}
	for _, v := range []byte{0x30, 0x31, 0x30} {
		d := receiveDatagram(t, conn)
		if d.data[10] != 0x73 || d.data[14] != v || d.addr.Port != Port {
			t.Fatalf("lost transition %x %v", d.data, d.addr)
		}
	}
}
func TestMulticastConfigurationRejectsBeforeSocket(t *testing.T) {
	for _, c := range []struct {
		address string
		allow   bool
	}{{"127.0.0.1:3610", true}, {"192.0.2.10:0", true}, {"192.0.2.10:3610", false}} {
		if s, err := ListenConfigured(c.address, New(model.New()), c.allow, "en0"); err == nil {
			s.Close()
			t.Fatalf("accepted %+v", c)
		}
	}
}
