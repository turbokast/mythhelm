package control

import (
	"fmt"
	"net"

	"golang.org/x/sys/unix"
)

func osPeerCred(c *net.UnixConn) (Peer, error) {
	raw, err := c.SyscallConn()
	if err != nil {
		return Peer{}, fmt.Errorf("control: peer credentials: %w", err)
	}
	var cred *unix.Ucred
	var credErr error
	if err := raw.Control(func(fd uintptr) {
		cred, credErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	}); err != nil {
		return Peer{}, fmt.Errorf("control: peer credentials: %w", err)
	}
	if credErr != nil {
		return Peer{}, fmt.Errorf("control: SO_PEERCRED: %w", credErr)
	}
	return Peer{UID: cred.Uid, PID: cred.Pid}, nil
}
