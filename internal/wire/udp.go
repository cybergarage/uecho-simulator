package wire

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"time"
)

// ValidateAddress accepts only literal IPv4 loopback addresses.
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

type UDP struct {
	conn   *net.UDPConn
	engine *Engine
}

func Listen(address string, engine *Engine) (*UDP, error) {
	if err := ValidateAddress(address); err != nil {
		return nil, err
	}
	a, err := net.ResolveUDPAddr("udp4", address)
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenUDP("udp4", a)
	if err != nil {
		return nil, err
	}
	return &UDP{conn, engine}, nil
}
func (s *UDP) Address() string { return s.conn.LocalAddr().String() }
func (s *UDP) Close() error    { return s.conn.Close() }

// Serve replies only to received unicast requests. It never advertises or scans.
func (s *UDP) Serve(ctx context.Context) error {
	defer s.Close()
	buf := make([]byte, 1025)
	for {
		if ctx.Err() != nil {
			return nil
		}
		if err := s.conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond)); err != nil {
			return err
		}
		n, from, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		reply, err := s.engine.Handle(buf[:n], from.String())
		if err != nil || reply == nil {
			continue
		}
		if _, err = s.conn.WriteToUDP(reply, from); err != nil {
			return err
		}
	}
}
