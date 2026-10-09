//go:build linux

package wire

import (
	"net"
	"syscall"
)

// Linux defaults to receiving groups joined by other sockets on any interface.
// Limit this receiver to its own membership and selected interface.
func scopeMulticast(conn *net.UDPConn) error {
	raw, err := conn.SyscallConn()
	if err != nil {
		return err
	}
	var optionErr error
	err = raw.Control(func(fd uintptr) { optionErr = syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IP, 49, 0) }) // IP_MULTICAST_ALL
	if err != nil {
		return err
	}
	return optionErr
}
