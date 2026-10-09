package control

import (
	"context"
	"errors"
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
