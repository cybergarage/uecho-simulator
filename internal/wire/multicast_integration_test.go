package wire

import (
	"context"
	"github.com/cybergarage/uecho-simulator/internal/model"
	"net"
	"os"
	"testing"
	"time"
)

// Only the CI's isolated Linux namespace opts in. Refuse any other interface,
// so accidentally setting the environment variable on a host cannot send to LAN.
func TestIsolatedIPv4Multicast(t *testing.T) {
	if os.Getenv("SIMULATOR_ISOLATED_MULTICAST_TEST") != "1" {
		t.Skip("isolated network namespace opt-in")
	}
	interfaces, err := net.Interfaces()
	if err != nil {
		t.Fatal(err)
	}
	for _, iface := range interfaces {
		if iface.Name != "lo" && iface.Name != "simtest0" {
			t.Fatalf("refuse non-isolated interface %s", iface.Name)
		}
	}
	iface, err := net.InterfaceByName("simtest0")
	if err != nil {
		t.Fatal(err)
	}
	group := &net.UDPAddr{IP: net.ParseIP(MulticastIPv4), Port: Port}
	announcements, err := net.ListenMulticastUDP("udp4", iface, group)
	if err != nil {
		t.Fatal(err)
	}
	defer announcements.Close()
	controller, err := listenUDP("192.0.2.20:3610", true)
	if err != nil {
		t.Fatal(err)
	}
	defer controller.Close()
	raw, err := controller.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	// Use the same interface for the controller's outbound discovery datagram.
	if err = selectMulticastInterface(raw, net.ParseIP("192.0.2.20")); err != nil {
		t.Fatal(err)
	}
	store := model.New()
	device, err := ListenConfigured("192.0.2.10:3610", New(store), true, "simtest0")
	if err != nil {
		t.Fatal(err)
	}
	defer device.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- device.Serve(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(time.Second):
			t.Error("device did not stop")
		}
	})
	receive := func(conn *net.UDPConn, esv, epc byte) []byte {
		t.Helper()
		if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
			t.Fatal(err)
		}
		buf := make([]byte, 1025)
		for {
			n, _, err := conn.ReadFromUDP(buf)
			if err != nil {
				t.Fatal(err)
			}
			if n >= 14 && buf[10] == esv && buf[12] == epc {
				return append([]byte(nil), buf[:n]...)
			}
		}
	}
	startup := receive(announcements, 0x73, 0xd5)
	if startup[14] != 3 {
		t.Fatalf("startup %x", startup)
	}
	if _, err := controller.WriteToUDP(frame(t, "1081123405ff010ef0006201d600"), group); err != nil {
		t.Fatal(err)
	}
	discovery := receive(controller, 0x72, 0xd6)
	if discovery[2] != 0x12 || discovery[3] != 0x34 || discovery[6] != 1 || discovery[14] != 3 {
		t.Fatalf("discovery %x", discovery)
	}
	addr, _ := net.ResolveUDPAddr("udp4", device.Address())
	if _, err := controller.WriteToUDP(frame(t, "1081123505ff010290016101800130"), addr); err != nil {
		t.Fatal(err)
	}
	receive(controller, 0x71, 0x80)
	changed := receive(announcements, 0x73, 0x80)
	if changed[14] != 0x30 {
		t.Fatalf("change %x", changed)
	}
	if _, err := controller.WriteToUDP(frame(t, "1081123605ff0102900163018000"), addr); err != nil {
		t.Fatal(err)
	}
	requested := receive(announcements, 0x73, 0x80)
	if requested[2] != 0x12 || requested[3] != 0x36 {
		t.Fatalf("INF_REQ %x", requested)
	}
}
