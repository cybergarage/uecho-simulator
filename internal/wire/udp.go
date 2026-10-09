package wire

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"syscall"
	"time"
)

const Port = 3610
const MulticastIPv4 = "224.0.23.0"

// ValidateAddress accepts literal IPv4 loopback addresses; port 0 is useful in tests.
func ValidateAddress(address string) error {
	a, err := netip.ParseAddrPort(address)
	if err != nil {
		return fmt.Errorf("use a literal IPv4:port: %w", err)
	}
	if !a.Addr().Is4() || !a.Addr().IsLoopback() {
		return fmt.Errorf("only IPv4 loopback addresses are supported")
	}
	return nil
}
func ValidateExplicitAddress(address string, allowLAN bool) error {
	if !allowLAN {
		return ValidateAddress(address)
	}
	a, err := netip.ParseAddrPort(address)
	if err != nil {
		return fmt.Errorf("use a literal local IPv4:port: %w", err)
	}
	if !a.Addr().Is4() || a.Addr().IsUnspecified() || a.Addr().IsMulticast() || a.Addr() == netip.MustParseAddr("255.255.255.255") {
		return fmt.Errorf("explicit UDP requires a local unicast IPv4 address")
	}
	return nil
}

// datagramConn allows controller/device integration without opening host sockets.
type datagramConn interface {
	LocalAddr() net.Addr
	ReadFromUDP([]byte) (int, *net.UDPAddr, error)
	WriteToUDP([]byte, *net.UDPAddr) (int, error)
	SetWriteDeadline(time.Time) error
	Close() error
}
type UDP struct {
	conn      datagramConn
	multicast datagramConn
	engine    *Engine
	closeOnce sync.Once
	closeErr  error
}

func Listen(address string, engine *Engine) (*UDP, error) {
	return ListenExplicit(address, engine, false)
}
func ListenExplicit(address string, engine *Engine, allowLAN bool) (*UDP, error) {
	return ListenConfigured(address, engine, allowLAN, "")
}

// Both sockets need address reuse when binding a concrete unicast address and
// a wildcard multicast receiver at the same standard port (notably on Linux).
func listenUDP(address string, reuse bool) (*net.UDPConn, error) {
	if !reuse {
		a, err := net.ResolveUDPAddr("udp4", address)
		if err != nil {
			return nil, err
		}
		return net.ListenUDP("udp4", a)
	}
	config := net.ListenConfig{Control: func(network, address string, raw syscall.RawConn) error {
		var optionErr error
		err := raw.Control(func(fd uintptr) {
			optionErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
		})
		if err != nil {
			return err
		}
		return optionErr
	}}
	packet, err := config.ListenPacket(context.Background(), "udp4", address)
	if err != nil {
		return nil, err
	}
	return packet.(*net.UDPConn), nil
}

func selectMulticastInterface(raw syscall.RawConn, address net.IP) error {
	var optionErr error
	ip := [4]byte{}
	copy(ip[:], address.To4())
	err := raw.Control(func(fd uintptr) {
		optionErr = syscall.SetsockoptInet4Addr(int(fd), syscall.IPPROTO_IP, syscall.IP_MULTICAST_IF, ip)
	})
	if err != nil {
		return err
	}
	return optionErr
}

// ListenConfigured joins the ECHONET group only when an interface is explicitly
// selected. The unicast bind address must belong to that interface, at port 3610.
func ListenConfigured(address string, engine *Engine, allowLAN bool, interfaceName string) (*UDP, error) {
	if err := ValidateExplicitAddress(address, allowLAN); err != nil {
		return nil, err
	}
	a, _ := net.ResolveUDPAddr("udp4", address)
	var iface *net.Interface
	if interfaceName != "" {
		if !allowLAN || a.IP.IsLoopback() || a.Port != Port {
			return nil, fmt.Errorf("multicast requires --allow-lan and a local non-loopback IPv4:3610")
		}
		var err error
		iface, err = net.InterfaceByName(interfaceName)
		if err != nil {
			return nil, err
		}
		addresses, err := iface.Addrs()
		if err != nil {
			return nil, err
		}
		found := false
		for _, addr := range addresses {
			ip, _, err := net.ParseCIDR(addr.String())
			if err == nil && ip.Equal(a.IP) {
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("UDP bind address does not belong to interface %s", interfaceName)
		}
	}
	conn, err := listenUDP(address, iface != nil)
	if err != nil {
		return nil, err
	}
	s := &UDP{conn: conn, engine: engine}
	if iface != nil {
		// Select the same interface for outbound multicast, avoiding the default route.
		raw, err := conn.SyscallConn()
		if err != nil {
			s.Close()
			return nil, err
		}
		if err := selectMulticastInterface(raw, a.IP); err != nil {
			s.Close()
			return nil, err
		}
		multicast, err := net.ListenMulticastUDP("udp4", iface, &net.UDPAddr{IP: net.ParseIP(MulticastIPv4), Port: Port})
		if err != nil {
			s.Close()
			return nil, err
		}
		s.multicast = multicast
	}
	return s, nil
}
func (s *UDP) Address() string { return s.conn.LocalAddr().String() }
func (s *UDP) Close() error {
	s.closeOnce.Do(func() {
		s.closeErr = s.conn.Close()
		if s.multicast != nil {
			if err := s.multicast.Close(); s.closeErr == nil {
				s.closeErr = err
			}
		}
	})
	return s.closeErr
}

type datagram struct {
	data []byte
	from *net.UDPAddr
	err  error
}

// Serve never scans. Multicast mode advertises D5 at startup and announces
// changes to the group. Unicast-only mode announces to recently active peers.
func (s *UDP) Serve(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	var readers sync.WaitGroup
	defer func() { cancel(); s.Close(); readers.Wait() }()
	changes, unsubscribe := s.engine.Store().SubscribeNotifications()
	defer unsubscribe()
	incoming := make(chan datagram, 32)
	read := func(conn datagramConn) {
		defer readers.Done()
		buf := make([]byte, MaxFrameSize+1)
		for {
			n, from, err := conn.ReadFromUDP(buf)
			packet := datagram{data: append([]byte(nil), buf[:n]...), from: from, err: err}
			select {
			case incoming <- packet:
			case <-ctx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}
	readers.Add(1)
	go read(s.conn)
	group := &net.UDPAddr{IP: net.ParseIP(MulticastIPv4), Port: Port}
	send := func(data []byte, to *net.UDPAddr) error {
		if err := s.conn.SetWriteDeadline(time.Now().Add(time.Second)); err != nil {
			return err
		}
		if _, err := s.conn.WriteToUDP(data, to); err != nil {
			return err
		}
		s.engine.Store().Log("TX", to.String(), "UDP datagram", data)
		return nil
	}
	if s.multicast != nil {
		readers.Add(1)
		go read(s.multicast)
		if err := send(s.engine.Notification(s.engine.Store().Startup()), group); err != nil {
			return err
		}
	}
	peers := make(map[string]time.Time)
	for {
		select {
		case <-ctx.Done():
			return nil
		case change, ok := <-changes:
			if !ok {
				return fmt.Errorf("required notification queue overflow")
			}
			data := s.engine.Notification(change)
			if s.multicast != nil {
				if err := send(data, group); err != nil {
					return err
				}
			} else {
				for ip, seen := range peers {
					if time.Since(seen) > 5*time.Minute {
						delete(peers, ip)
						continue
					}
					if err := send(data, &net.UDPAddr{IP: net.ParseIP(ip), Port: Port}); err != nil {
						return err
					}
				}
			}
		case packet := <-incoming:
			if packet.err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return packet.err
			}
			if packet.from == nil || packet.from.IP.To4() == nil || packet.from.IP.IsMulticast() || packet.from.IP.IsUnspecified() {
				continue
			}
			replies, err := s.engine.HandleAll(packet.data, packet.from.String())
			if err != nil {
				continue
			}
			// Register only a recognized request to one of our objects, never responses.
			esv := packet.data[10]
			if esv == 0x60 || esv == 0x61 || esv == 0x62 || esv == 0x63 || esv == 0x6e || esv == 0x74 {
				target := uint32(packet.data[7])<<16 | uint32(packet.data[8])<<8 | uint32(packet.data[9])
				if len(s.engine.Store().Targets(target)) > 0 {
					ip := packet.from.IP.String()
					if _, exists := peers[ip]; exists || len(peers) < 64 {
						peers[ip] = time.Now()
					}
				}
			}
			for _, reply := range replies {
				to := &net.UDPAddr{IP: packet.from.IP, Port: Port}
				if esv == 0x63 && reply[10] == 0x73 && s.multicast != nil {
					to = group
				}
				if err := send(reply, to); err != nil {
					return err
				}
			}
		}
	}
}
