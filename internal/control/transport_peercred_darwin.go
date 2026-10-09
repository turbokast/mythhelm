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
	var cred *unix.Xucred
	var credErr error
	if err := raw.Control(func(fd uintptr) {
		cred, credErr = unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
	}); err != nil {
		return Peer{}, fmt.Errorf("control: peer credentials: %w", err)
	}
	if credErr != nil {
		return Peer{}, fmt.Errorf("control: LOCAL_PEERCRED: %w", credErr)
	}
	return Peer{UID: cred.Uid, PID: -1}, nil
}
