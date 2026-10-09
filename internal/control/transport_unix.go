//go:build linux || darwin

package control

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/turbokast/mythhelm/internal/v2contract"
)

// denyTimeout bounds the refusal of a foreign peer: writing the reply and
// discarding what the peer already sent, so the reply is not lost to a reset.
const denyTimeout = 2 * time.Second

// maxConcurrentDenials bounds the goroutines (each holding a descriptor for up
// to denyTimeout) that refuse foreign peers; further foreign peers are closed
// at once without a reply.
const maxConcurrentDenials = 16

type peerCredFunc func(*net.UnixConn) (Peer, error)

type unixTransport struct {
	cred peerCredFunc // platform credential source; replaced in tests
	uid  uint32       // the supervisor's UID: the only UID admitted
}

// NewUnixTransport returns the Unix-socket Transport: a 0700 socket directory
// and same-UID peer authentication on every connection (I03).
func NewUnixTransport() Transport {
	return &unixTransport{cred: osPeerCred, uid: uint32(os.Geteuid())} //nolint:gosec // G115: UIDs are non-negative
}

func (t *unixTransport) peer(c *net.UnixConn) (Peer, error) {
	p, err := t.cred(c)
	if err != nil {
		return Peer{}, err
	}
	p.SameUser = p.UID == t.uid
	if u, err := user.LookupId(strconv.FormatUint(uint64(p.UID), 10)); err == nil {
		p.OSUser = u.Username
	}
	return p, nil
}

func (t *unixTransport) Listen(path string) (Listener, error) {
	if err := ensureSocketDir(filepath.Dir(path), t.uid); err != nil {
		return nil, err
	}
	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, fmt.Errorf("control: listen %s: %w", path, err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = ln.Close()
		return nil, fmt.Errorf("control: chmod socket %s: %w", path, err)
	}
	return &unixListener{ln: ln, t: t, denySlots: make(chan struct{}, maxConcurrentDenials)}, nil
}

// ensureSocketDir creates dir with mode 0700 or verifies an existing one is a
// real directory owned by uid that grants no group or other access.
func ensureSocketDir(dir string, uid uint32) error {
	if err := os.Mkdir(dir, 0o700); err == nil {
		return os.Chmod(dir, 0o700) //nolint:gosec // G302: a directory needs the owner x bit; the umask must not alter the mode
	} else if !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("control: create socket dir %s: %w", dir, err)
	}
	fi, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("control: stat socket dir %s: %w", dir, err)
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	switch {
	case !fi.IsDir():
		return fmt.Errorf("control: socket dir %s is not a directory", dir)
	case !ok || st.Uid != uid:
		return fmt.Errorf("control: socket dir %s is not owned by uid %d", dir, uid)
	case fi.Mode().Perm()&0o077 != 0:
		return fmt.Errorf("control: socket dir %s has mode %04o, want 0700", dir, fi.Mode().Perm())
	}
	return nil
}

func (t *unixTransport) Dial(path string) (Conn, error) {
	nc, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: path, Net: "unix"})
	switch {
	case errors.Is(err, os.ErrNotExist), errors.Is(err, syscall.ECONNREFUSED):
		return nil, ErrNoSupervisor
	case errors.Is(err, os.ErrPermission):
		return nil, fmt.Errorf("control: dial %s: %s: %w", path, codePermissionDenied, err)
	case err != nil:
		return nil, fmt.Errorf("control: dial %s: %w", path, err)
	}
	p, err := t.peer(nc)
	if err != nil {
		_ = nc.Close()
		return nil, err
	}
	if !p.SameUser {
		_ = nc.Close()
		return nil, fmt.Errorf("control: dial %s: %s: supervisor uid %d is not %d", path, codePermissionDenied, p.UID, t.uid)
	}
	return &unixConn{c: nc, peer: p}, nil
}

type unixListener struct {
	ln        *net.UnixListener
	t         *unixTransport
	denySlots chan struct{}
}

// Accept returns the next same-UID connection. A foreign or unidentifiable
// peer is refused without its frames being read or interpreted.
func (l *unixListener) Accept() (Conn, error) {
	for {
		nc, err := l.ln.AcceptUnix()
		if err != nil {
			return nil, err
		}
		p, err := l.t.peer(nc)
		if err != nil || !p.SameUser {
			l.refuse(nc)
			continue
		}
		return &unixConn{c: nc, peer: p}, nil
	}
}

// refuse hands a foreign peer to deny when a slot is free, else closes it.
func (l *unixListener) refuse(nc *net.UnixConn) {
	select {
	case l.denySlots <- struct{}{}:
		go func() {
			defer func() { <-l.denySlots }()
			deny(nc)
		}()
	default:
		_ = nc.Close()
	}
}

func (l *unixListener) Close() error { return l.ln.Close() }
func (l *unixListener) Addr() string { return l.ln.Addr().String() }

// deny sends permission_denied, then discards (never parses) up to one frame
// of what the peer sent: closing with unread data would reset the connection
// and could destroy the reply before the peer reads it.
func deny(nc *net.UnixConn) {
	defer func() { _ = nc.Close() }()
	_ = nc.SetDeadline(time.Now().Add(denyTimeout))
	reply, err := Encode(permissionDenied{Code: codePermissionDenied, Message: "peer is not the supervisor's user"})
	if err != nil {
		return
	}
	if _, err := nc.Write(reply); err != nil {
		return
	}
	_ = nc.CloseWrite()
	_, _ = io.CopyN(io.Discard, nc, v2contract.MaxFrameBytes+prefixLen)
}

type unixConn struct {
	c    *net.UnixConn
	peer Peer
}

func (c *unixConn) Peer() Peer   { return c.peer }
func (c *unixConn) Close() error { return c.c.Close() }

func (c *unixConn) Request(ctx context.Context, frame []byte) ([]byte, error) {
	if err := checkOutgoing(frame); err != nil {
		return nil, err
	}
	defer c.watch(ctx)()
	if _, err := c.c.Write(frame); err != nil {
		return nil, fmt.Errorf("control: send request: %w", ctxErr(ctx, err))
	}
	return c.readFrame(ctx, false)
}

func (c *unixConn) Receive(ctx context.Context) ([]byte, error) {
	defer c.watch(ctx)()
	return c.readFrame(ctx, true)
}

func (c *unixConn) Respond(frame []byte) error {
	if err := checkOutgoing(frame); err != nil {
		return err
	}
	if _, err := c.c.Write(frame); err != nil {
		return fmt.Errorf("control: send response: %w", err)
	}
	return nil
}

// watch applies ctx's deadline and cancellation to the connection and returns
// the function that releases them.
func (c *unixConn) watch(ctx context.Context) func() {
	if d, ok := ctx.Deadline(); ok {
		_ = c.c.SetDeadline(d)
	}
	stop := context.AfterFunc(ctx, func() { _ = c.c.SetDeadline(time.Unix(1, 0)) })
	return func() {
		stop()
		_ = c.c.SetDeadline(time.Time{})
	}
}

// readFrame reads one length-prefixed frame. The prefix is checked against
// MaxFrameBytes before the payload is allocated or read (NFR-1); ingress
// frames additionally pass CheckIngress.
func (c *unixConn) readFrame(ctx context.Context, ingress bool) ([]byte, error) {
	var prefix [prefixLen]byte
	if _, err := io.ReadFull(c.c, prefix[:]); err != nil {
		return nil, fmt.Errorf("control: read frame prefix: %w", ctxErr(ctx, err))
	}
	n := binary.BigEndian.Uint32(prefix[:])
	if n > v2contract.MaxFrameBytes {
		return nil, fmt.Errorf("control: frame bytes %d exceed limit %d", n, v2contract.MaxFrameBytes)
	}
	frame := make([]byte, prefixLen+int(n))
	copy(frame, prefix[:])
	if _, err := io.ReadFull(c.c, frame[prefixLen:]); err != nil {
		return nil, fmt.Errorf("control: read frame payload: %w", ctxErr(ctx, err))
	}
	if ingress {
		if err := CheckIngress(frame); err != nil {
			return nil, err
		}
	}
	return frame, nil
}

// ctxErr prefers the context's error: cancellation reaches the connection as
// an I/O deadline error that hides the cause.
func ctxErr(ctx context.Context, err error) error {
	if cerr := ctx.Err(); cerr != nil {
		return cerr
	}
	return err
}

// checkOutgoing rejects a frame whose length prefix is missing, does not match
// the bytes that follow, or exceeds MaxFrameBytes: such a frame would
// desynchronise the stream for every later frame.
func checkOutgoing(frame []byte) error {
	if len(frame) < prefixLen {
		return fmt.Errorf("control: outgoing frame has %d bytes, shorter than the %d-byte prefix", len(frame), prefixLen)
	}
	n := binary.BigEndian.Uint32(frame[:prefixLen])
	if n > v2contract.MaxFrameBytes {
		return fmt.Errorf("control: outgoing frame bytes %d exceed limit %d", n, v2contract.MaxFrameBytes)
	}
	if int(n) != len(frame)-prefixLen {
		return fmt.Errorf("control: outgoing frame prefix says %d bytes, payload has %d", n, len(frame)-prefixLen)
	}
	return nil
}
