package control

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"

	"github.com/turbokast/mythhelm/internal/v2contract"
)

const (
	pipePrefix = `\\.\pipe\`
	// denyTimeout bounds the refusal of a foreign peer: writing the reply and
	// discarding what the peer already sent.
	denyTimeout = 2 * time.Second
	// maxConcurrentDenials bounds the goroutines that refuse foreign peers;
	// further foreign peers are closed at once without a reply.
	maxConcurrentDenials = 16
	// dialTimeout bounds waiting for a busy pipe instance.
	dialTimeout = 2 * time.Second
	pipeBufSize = 64 << 10
)

// pipeIdentity is the user and logon session of a process, read from its
// token. Two processes are the same logon only when both fields match.
type pipeIdentity struct {
	SID     string
	LogonID uint64
	PID     uint32
}

func (id pipeIdentity) sameLogon(o pipeIdentity) bool {
	return id.SID == o.SID && id.LogonID == o.LogonID
}

type pipeTransport struct {
	self pipeIdentity
	// peerOf reads the identity of the process at the other end of c; server
	// is true when c is the supervisor's end. Replaced in tests.
	peerOf func(c net.Conn, server bool) (pipeIdentity, error)
}

// NewPipeTransport returns the named-pipe Transport: a pipe whose DACL grants
// access to the current user only, created as the first instance, remote
// clients rejected, and the logon session of every peer checked (I03, I18).
func NewPipeTransport() (Transport, error) {
	self, err := currentIdentity()
	if err != nil {
		return nil, err
	}
	return &pipeTransport{self: self, peerOf: osPipePeer}, nil
}

func currentIdentity() (pipeIdentity, error) {
	id, err := tokenIdentity(windows.GetCurrentProcessToken())
	id.PID = windows.GetCurrentProcessId()
	return id, err
}

// tokenIdentity reads the user SID and logon session LUID from a token.
func tokenIdentity(tok windows.Token) (pipeIdentity, error) {
	u, err := tok.GetTokenUser()
	if err != nil {
		return pipeIdentity{}, fmt.Errorf("control: reading the token user: %w", err)
	}
	// TOKEN_STATISTICS starts with TokenId and AuthenticationId, two LUIDs of
	// {LowPart uint32, HighPart int32}.
	var buf [64]byte
	var n uint32
	if err := windows.GetTokenInformation(tok, windows.TokenStatistics, &buf[0], uint32(len(buf)), &n); err != nil {
		return pipeIdentity{}, fmt.Errorf("control: reading the token logon session: %w", err)
	}
	return pipeIdentity{
		SID:     u.User.Sid.String(),
		LogonID: binary.LittleEndian.Uint64(buf[8:16]),
	}, nil
}

// osPipePeer reads the peer's identity from its process token.
func osPipePeer(c net.Conn, server bool) (pipeIdentity, error) {
	f, ok := c.(interface{ Fd() uintptr })
	if !ok {
		return pipeIdentity{}, errors.New("control: pipe connection exposes no handle")
	}
	h := windows.Handle(f.Fd())
	var pid uint32
	var err error
	if server {
		err = windows.GetNamedPipeClientProcessId(h, &pid)
	} else {
		err = windows.GetNamedPipeServerProcessId(h, &pid)
	}
	if err != nil {
		return pipeIdentity{}, fmt.Errorf("control: reading the pipe peer process: %w", err)
	}
	ph, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return pipeIdentity{}, fmt.Errorf("control: opening peer process %d: %w", pid, err)
	}
	defer func() { _ = windows.CloseHandle(ph) }()
	var tok windows.Token
	if err := windows.OpenProcessToken(ph, windows.TOKEN_QUERY, &tok); err != nil {
		return pipeIdentity{}, fmt.Errorf("control: opening the token of peer process %d: %w", pid, err)
	}
	defer func() { _ = tok.Close() }()
	id, err := tokenIdentity(tok)
	id.PID = pid
	return id, err
}

// pipeName maps an endpoint path to the pipe it is served on. A path already
// in the pipe namespace is used as is; any other path is hashed, so the pipe
// name never carries a filesystem path.
func pipeName(path string) string {
	if strings.HasPrefix(path, pipePrefix) {
		return path
	}
	sum := sha256.Sum256([]byte(strings.ToLower(path)))
	return pipePrefix + "mythhelm-" + hex.EncodeToString(sum[:16])
}

func (t *pipeTransport) peer(c net.Conn, server bool) (Peer, error) {
	id, err := t.peerOf(c, server)
	if err != nil {
		return Peer{}, err
	}
	p := Peer{PID: int32(id.PID), SameUser: t.self.sameLogon(id)} //nolint:gosec // G115: Windows PIDs fit in int32
	if sid, err := windows.StringToSid(id.SID); err == nil {
		if account, domain, _, err := sid.LookupAccount(""); err == nil {
			p.OSUser = domain + `\` + account
		}
	}
	return p, nil
}

func (t *pipeTransport) Listen(path string) (Listener, error) {
	name := pipeName(path)
	cfg := &winio.PipeConfig{
		SecurityDescriptor: "D:P(A;;GA;;;" + t.self.SID + ")",
		InputBufferSize:    pipeBufSize,
		OutputBufferSize:   pipeBufSize,
	}
	ln, err := winio.ListenPipe(name, cfg)
	if err != nil {
		return nil, fmt.Errorf("control: listen %s: %w", name, err)
	}
	return &pipeListener{ln: ln, name: name, t: t, denySlots: make(chan struct{}, maxConcurrentDenials)}, nil
}

func (t *pipeTransport) Dial(path string) (Conn, error) {
	name := pipeName(path)
	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	defer cancel()
	nc, err := winio.DialPipeContext(ctx, name)
	switch {
	case errors.Is(err, windows.ERROR_FILE_NOT_FOUND), errors.Is(err, os.ErrNotExist):
		return nil, ErrNoSupervisor
	case errors.Is(err, windows.ERROR_ACCESS_DENIED), errors.Is(err, os.ErrPermission):
		return nil, fmt.Errorf("control: dial %s: %s: %w", name, codePermissionDenied, err)
	case err != nil:
		return nil, fmt.Errorf("control: dial %s: %w", name, err)
	}
	p, err := t.peer(nc, false)
	if err != nil {
		_ = nc.Close()
		return nil, err
	}
	if !p.SameUser {
		_ = nc.Close()
		return nil, fmt.Errorf("control: dial %s: %s: the supervisor is not this user's logon session", name, codePermissionDenied)
	}
	return &streamConn{c: nc, peer: p}, nil
}

type pipeListener struct {
	ln        net.Listener
	name      string
	t         *pipeTransport
	denySlots chan struct{}
}

// Accept returns the next same-logon connection. A foreign or
// unidentifiable peer is refused without its frames being interpreted.
func (l *pipeListener) Accept() (Conn, error) {
	for {
		nc, err := l.ln.Accept()
		if err != nil {
			return nil, err
		}
		p, err := l.t.peer(nc, true)
		if err != nil || !p.SameUser {
			l.refuse(nc)
			continue
		}
		return &streamConn{c: nc, peer: p}, nil
	}
}

// refuse hands a foreign peer to deny when a slot is free, else closes it.
func (l *pipeListener) refuse(nc net.Conn) {
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

func (l *pipeListener) Close() error { return l.ln.Close() }
func (l *pipeListener) Addr() string { return l.name }

// deny sends permission_denied, then discards (never parses) up to one frame
// of what the peer sent, so the peer reads the reply rather than a reset.
func deny(nc net.Conn) {
	defer func() { _ = nc.Close() }()
	_ = nc.SetDeadline(time.Now().Add(denyTimeout))
	reply, err := Encode(permissionDenied{Code: codePermissionDenied, Message: "peer is not the supervisor's logon session"})
	if err != nil {
		return
	}
	if _, err := nc.Write(reply); err != nil {
		return
	}
	_, _ = io.CopyN(io.Discard, nc, v2contract.MaxFrameBytes+prefixLen)
}
