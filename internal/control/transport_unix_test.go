//go:build linux || darwin

package control

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/v2contract"
)

func euid() uint32 { return uint32(os.Geteuid()) } //nolint:gosec // G115: UIDs are non-negative

func testFrame(t *testing.T) []byte {
	t.Helper()
	f, err := Encode(Frame{
		OperationID: "op_SYNTHETIC1",
		Method:      "echo",
	})
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// shortDir returns a fresh directory directly under /tmp. A Unix socket path
// must fit sun_path (104 bytes on macOS, 108 on Linux), which t.TempDir()
// under /var/folders exceeds: bind and dial then fail with EINVAL.
func shortDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "mh") //nolint:usetesting // t.TempDir() is too long for sun_path on macOS
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func socketPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(shortDir(t), "run", "control.sock")
}

func ctx5(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestUnixRoundTrip(t *testing.T) {
	t.Parallel()
	tr := NewUnixTransport()
	ln, err := tr.Listen(socketPath(t))
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	served := make(chan error, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			served <- err
			return
		}
		t.Cleanup(func() { _ = c.Close() })
		req, err := c.Receive(ctx5(t))
		if err != nil {
			served <- err
			return
		}
		served <- c.Respond(req)
	}()

	c, err := tr.Dial(ln.Addr())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	want := testFrame(t)
	got, err := c.Request(ctx5(t), want)
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("response = %q, want echo %q", got, want)
	}
	if err := <-served; err != nil {
		t.Fatalf("server: %v", err)
	}
	if p := c.Peer(); !p.SameUser || p.UID != euid() {
		t.Errorf("client-side peer = %+v, want same-user uid %d", p, os.Geteuid())
	}
}

// I03 (v2 §2): only the owning user reaches the control endpoint.
func TestSocketDirIs0700(t *testing.T) {
	t.Parallel()
	tr := NewUnixTransport()
	path := socketPath(t)
	ln, err := tr.Listen(path)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	for p, want := range map[string]os.FileMode{filepath.Dir(path): 0o700, path: 0o600} {
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if got := fi.Mode().Perm(); got != want {
			t.Errorf("%s mode = %04o, want %04o", p, got, want)
		}
	}

	open := filepath.Join(shortDir(t), "open")
	if err := os.Mkdir(open, 0o755); err != nil { //nolint:gosec // the 0755 dir is the broken input
		t.Fatal(err)
	}
	if err := os.Chmod(open, 0o755); err != nil { //nolint:gosec // the 0755 dir is the broken input
		t.Fatal(err)
	}
	if l, err := tr.Listen(filepath.Join(open, "control.sock")); err == nil {
		_ = l.Close()
		t.Error("Listen accepted a 0755 socket directory")
	} else if !strings.Contains(err.Error(), "0700") {
		t.Errorf("Listen(0755 dir) error = %v, want naming 0700", err)
	}
}

// I03 (v2 §2): the peer's platform-observed UID decides; a foreign UID is
// refused before any frame is read, and the accept path never yields it.
func TestForeignUIDRejected(t *testing.T) {
	t.Parallel()
	server := &unixTransport{
		uid:  euid(),
		cred: func(*net.UnixConn) (Peer, error) { return Peer{UID: euid() + 1, PID: -1}, nil },
	}
	ln, err := server.Listen(socketPath(t))
	if err != nil {
		t.Fatal(err)
	}
	accepted := make(chan Conn, 1)
	acceptErr := make(chan error, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			acceptErr <- err
			return
		}
		accepted <- c
	}()

	nc, err := net.Dial("unix", ln.Addr())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = nc.Close() })
	req := testFrame(t)
	if _, err := nc.Write(req); err != nil {
		t.Fatal(err)
	}
	_ = nc.SetReadDeadline(time.Now().Add(5 * time.Second))
	prefix := make([]byte, prefixLen)
	if _, err := io.ReadFull(nc, prefix); err != nil {
		t.Fatalf("read refusal: %v", err)
	}
	body := make([]byte, binary.BigEndian.Uint32(prefix))
	if _, err := io.ReadFull(nc, body); err != nil {
		t.Fatalf("read refusal body: %v", err)
	}
	var reply permissionDenied
	if err := json.Unmarshal(body, &reply); err != nil || reply.Code != "permission_denied" {
		t.Fatalf("refusal = %q (%v), want code permission_denied", body, err)
	}

	_ = ln.Close()
	select {
	case c := <-accepted:
		_ = c.Close()
		t.Fatal("Accept returned a foreign-UID connection to the request handler")
	case <-acceptErr:
	case <-time.After(5 * time.Second):
		t.Fatal("Accept did not return after Close")
	}
}

func TestDialWithoutSupervisor(t *testing.T) {
	t.Parallel()
	if _, err := NewUnixTransport().Dial(socketPath(t)); !errors.Is(err, ErrNoSupervisor) {
		t.Errorf("Dial(no socket) error = %v, want ErrNoSupervisor", err)
	}
}

// NFR-1: the prefix is bounded before the payload is read or allocated, and
// received frames pass the ingress check.
func TestReceiveEnforcesFrameLimits(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		raw     []byte
		wantErr string
	}{
		{"oversized prefix", binary.BigEndian.AppendUint32(nil, v2contract.MaxFrameBytes+1), "exceed limit"},
		{"unknown frame key", mustEncode(t, map[string]any{"operation_id": "op", "bogus": 1}), "bogus"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tr := NewUnixTransport()
			ln, err := tr.Listen(socketPath(t))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = ln.Close() })
			got := make(chan error, 1)
			go func() {
				c, err := ln.Accept()
				if err != nil {
					got <- err
					return
				}
				t.Cleanup(func() { _ = c.Close() })
				_, err = c.Receive(ctx5(t))
				got <- err
			}()
			nc, err := net.Dial("unix", ln.Addr())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = nc.Close() })
			if _, err := nc.Write(tc.raw); err != nil {
				t.Fatal(err)
			}
			if err := <-got; err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("Receive error = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

func mustEncode(t *testing.T, v any) []byte {
	t.Helper()
	b, err := Encode(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// tcpLiterals returns the string literals in src that name a TCP network.
func tcpLiterals(t *testing.T, name, src string) []string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), name, src, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	var found []string
	ast.Inspect(f, func(n ast.Node) bool {
		if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING && strings.Contains(strings.ToLower(lit.Value), "tcp") {
			found = append(found, lit.Value)
		}
		return true
	})
	return found
}

// I03 (v2 §2): the control surface is local-only; no TCP path exists.
func TestNoTCPListener(t *testing.T) {
	t.Parallel()
	broken := "package x\nfunc f() { _ = " + "net.Listen(\"t" + "cp\", \":0\") }\n"
	if got := tcpLiterals(t, "broken.go", broken); len(got) != 1 {
		t.Fatalf("scanner missed a TCP literal in a broken source: %v", got)
	}

	files, err := filepath.Glob("*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("glob package files: %v (%d files)", err, len(files))
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f) //nolint:gosec // G304: f comes from globbing this package's directory
		if err != nil {
			t.Fatal(err)
		}
		if got := tcpLiterals(t, f, string(src)); len(got) != 0 {
			t.Errorf("%s contains TCP network literals %v", f, got)
		}
	}
}

// refusedReply reads what a refused peer receives: the permission_denied
// frame, or EOF when the listener closed it without a reply.
func refusedReply(t *testing.T, addr string) (code string, closedSilently bool) {
	t.Helper()
	nc, err := net.Dial("unix", addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = nc.Close() })
	_ = nc.SetReadDeadline(time.Now().Add(5 * time.Second))
	prefix := make([]byte, prefixLen)
	if _, err := io.ReadFull(nc, prefix); err != nil {
		if errors.Is(err, io.EOF) {
			return "", true
		}
		t.Fatalf("read refusal: %v", err)
	}
	body := make([]byte, binary.BigEndian.Uint32(prefix))
	if _, err := io.ReadFull(nc, body); err != nil {
		t.Fatalf("read refusal body: %v", err)
	}
	var reply permissionDenied
	if err := json.Unmarshal(body, &reply); err != nil {
		t.Fatalf("refusal %q: %v", body, err)
	}
	return reply.Code, false
}

// Foreign peers beyond the concurrent-refusal bound are closed at once; a
// freed slot refuses with permission_denied again.
func TestDenialsAreBounded(t *testing.T) {
	t.Parallel()
	server := &unixTransport{
		uid:  euid(),
		cred: func(*net.UnixConn) (Peer, error) { return Peer{UID: euid() + 1, PID: -1}, nil },
	}
	l, err := server.Listen(socketPath(t))
	if err != nil {
		t.Fatal(err)
	}
	ln := l.(*unixListener)
	t.Cleanup(func() { _ = ln.Close() })
	ln.denySlots = make(chan struct{}, 1)
	go func() { _, _ = ln.Accept() }()

	ln.denySlots <- struct{}{} // every slot busy
	if code, silent := refusedReply(t, ln.Addr()); !silent {
		t.Errorf("peer past the bound got reply %q, want a silent close", code)
	}
	<-ln.denySlots // slot freed
	if code, silent := refusedReply(t, ln.Addr()); silent || code != "permission_denied" {
		t.Errorf("peer within the bound: code %q silent %v, want permission_denied", code, silent)
	}
}

// A malformed outgoing frame is rejected before anything is written, so it
// cannot desynchronise the stream for the next frame.
func TestOutgoingFramesAreValidated(t *testing.T) {
	t.Parallel()
	tr := NewUnixTransport()
	ln, err := tr.Listen(socketPath(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	srvc := make(chan Conn, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			close(srvc)
			return
		}
		srvc <- c
	}()
	client, err := tr.Dial(ln.Addr())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	server := <-srvc
	if server == nil {
		t.Fatal("Accept failed")
	}
	t.Cleanup(func() { _ = server.Close() })

	valid := testFrame(t)
	tooShort := binary.BigEndian.AppendUint32(nil, uint32(len(valid)-prefixLen+5)) //nolint:gosec // G115: small test length
	tooShort = append(tooShort, valid[prefixLen:]...)
	tests := []struct {
		name    string
		frame   []byte
		wantErr string
	}{
		{"shorter than prefix", []byte{0, 0}, "shorter than"},
		{"prefix longer than payload", tooShort, "prefix says"},
		{"trailing bytes after payload", append(append([]byte{}, valid...), 'x'), "prefix says"},
		{"over MaxFrameBytes", binary.BigEndian.AppendUint32(nil, v2contract.MaxFrameBytes+1), "exceed limit"},
	}
	for _, tc := range tests {
		if _, err := client.Request(ctx5(t), tc.frame); err == nil || !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("Request(%s) error = %v, want containing %q", tc.name, err, tc.wantErr)
		}
		if err := server.Respond(tc.frame); err == nil || !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("Respond(%s) error = %v, want containing %q", tc.name, err, tc.wantErr)
		}
	}

	// Nothing from the rejected frames reached the stream: the next valid
	// request is the first thing the server reads.
	go func() { _, _ = client.Request(ctx5(t), valid) }()
	got, err := server.Receive(ctx5(t))
	if err != nil || !bytes.Equal(got, valid) {
		t.Errorf("Receive after rejected frames = %q, %v; want the valid frame", got, err)
	}
}
