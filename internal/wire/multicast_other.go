//go:build !linux

package wire

import "net"

func scopeMulticast(conn *net.UDPConn) error { return nil }
