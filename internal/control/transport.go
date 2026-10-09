package control

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/turbokast/mythhelm/internal/v2contract"
)

// Peer is the authenticated identity of a control client, observed from the
// platform at accept or connect time; it is never read from a payload (I03).
type Peer struct {
	UID      uint32 // Unix peer credential UID; 0 with OSUser on Windows
	OSUser   string // resolved username where available, else ""
	PID      int32  // peer PID where the platform reports one, else -1
	SameUser bool   // peer UID == supervisor UID (Unix) / same logon (Windows)
}

// Transport dials and serves one local control endpoint. No TCP
// implementation exists; adding one is a design change, not a configuration.
type Transport interface {
	// Listen serves path (socket path / pipe name). It fails when the
	// address is in use or the 0700 socket directory cannot be created.
	Listen(path string) (Listener, error)
	// Dial connects to a supervisor at path. It returns ErrNoSupervisor when
	// none is running and a permission error when the endpoint is not
	// reachable by this user.
	Dial(path string) (Conn, error)
}

// ErrNoSupervisor is returned by Dial when nothing is listening at the path.
var ErrNoSupervisor = errors.New("control: no supervisor running")

// Listener accepts authenticated control connections. Accept returns only
// connections whose peer passed authentication; others are refused with
// permission_denied inside Accept.
type Listener interface {
	Accept() (Conn, error)
	Close() error
	Addr() string
}

// Conn is one authenticated control connection. Peer reports the identity
// established at accept or connect time. Every received frame has passed the
// NFR-1 limits; the length prefix is bounded before any payload is read.
type Conn interface {
	Peer() Peer
	// Request sends one frame and returns the response frame.
	Request(ctx context.Context, frame []byte) ([]byte, error)
	// Receive reads the next request frame on a connection returned by
	// Accept and checks it with CheckIngress.
	Receive(ctx context.Context) ([]byte, error)
	// Respond writes the response frame to the request last received.
	Respond(frame []byte) error
	Close() error
}

// permissionDenied is the wire reply sent to a refused peer.
type permissionDenied struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

const codePermissionDenied = "permission_denied"

// streamConn is the Conn over any stream socket or pipe, shared by the Unix
// and Windows transports.
type streamConn struct {
	c    net.Conn
	peer Peer
}

func (c *streamConn) Peer() Peer   { return c.peer }
func (c *streamConn) Close() error { return c.c.Close() }

func (c *streamConn) Request(ctx context.Context, frame []byte) ([]byte, error) {
	if err := checkOutgoing(frame); err != nil {
		return nil, err
	}
	defer c.watch(ctx)()
	if _, err := c.c.Write(frame); err != nil {
		return nil, fmt.Errorf("control: send request: %w", ctxErr(ctx, err))
	}
	return c.readFrame(ctx, false)
}

func (c *streamConn) Receive(ctx context.Context) ([]byte, error) {
	defer c.watch(ctx)()
	return c.readFrame(ctx, true)
}

func (c *streamConn) Respond(frame []byte) error {
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
func (c *streamConn) watch(ctx context.Context) func() {
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
func (c *streamConn) readFrame(ctx context.Context, ingress bool) ([]byte, error) {
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
