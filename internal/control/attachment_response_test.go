package control

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeConn is an owned server-side Conn. Respond records its frame and then
// blocks until the test opens the gate; Close never unblocks it, as with an
// OS write the server cannot cancel.
type fakeConn struct {
	peer      Peer
	inbox     chan []byte
	gate      chan struct{}
	gateOnce  sync.Once
	failWrite bool
	entered   chan []byte
	calls     atomic.Int32
	returned  atomic.Int32
	closed    chan struct{}
	closeOnce sync.Once
}

func newFakeConn(peer Peer, blocked bool) *fakeConn {
	c := &fakeConn{peer: peer, inbox: make(chan []byte, 16), gate: make(chan struct{}),
		entered: make(chan []byte, 16), closed: make(chan struct{})}
	if !blocked {
		c.openGate()
	}
	return c
}

func (c *fakeConn) openGate()  { c.gateOnce.Do(func() { close(c.gate) }) }
func (c *fakeConn) Peer() Peer { return c.peer }
func (c *fakeConn) Request(context.Context, []byte) ([]byte, error) {
	return nil, errors.New("fakeConn serves only")
}

func (c *fakeConn) Receive(ctx context.Context) ([]byte, error) {
	select {
	case f := <-c.inbox:
		return f, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.closed:
		return nil, io.EOF
	}
}

func (c *fakeConn) Respond(frame []byte) error {
	if err := checkOutgoing(frame); err != nil {
		return err
	}
	c.calls.Add(1)
	c.entered <- bytes.Clone(frame)
	<-c.gate
	c.returned.Add(1)
	if c.failWrite {
		return io.ErrClosedPipe
	}
	return nil
}

func (c *fakeConn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

func (c *fakeConn) send(t *testing.T, in Intent) {
	t.Helper()
	frame, err := Encode(in)
	if err != nil {
		t.Fatal(err)
	}
	c.inbox <- frame
}

// published returns every frame Respond has been handed so far.
func (c *fakeConn) published() []Result {
	var out []Result
	for {
		select {
		case f := <-c.entered:
			var r Result
			if err := json.Unmarshal(f[prefixLen:], &r); err != nil {
				panic(err)
			}
			out = append(out, r)
		default:
			return out
		}
	}
}

type fakeListener struct {
	conns     chan Conn
	closed    chan struct{}
	closeOnce sync.Once
}

func (l *fakeListener) Accept() (Conn, error) {
	select {
	case c := <-l.conns:
		return c, nil
	case <-l.closed:
		return nil, errors.New("fakeListener closed")
	}
}

func (l *fakeListener) Close() error {
	l.closeOnce.Do(func() { close(l.closed) })
	return nil
}

func (l *fakeListener) Addr() string { return "fake" }

// serveFixture runs the real Serve over one fakeConn. Its cleanup opens the
// conn's gate, cancels Serve and joins it, so a failing test leaks no worker.
type serveFixture struct {
	conn     *fakeConn
	cancel   context.CancelFunc
	done     chan error
	stopOnce sync.Once
}

func startServe(t *testing.T, srv *Server, db *sql.DB, conn *fakeConn) *serveFixture {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	l := &fakeListener{conns: make(chan Conn, 1), closed: make(chan struct{})}
	l.conns <- conn
	f := &serveFixture{conn: conn, cancel: cancel, done: make(chan error, 1)}
	go func() { f.done <- Serve(ctx, l, srv, db) }()
	t.Cleanup(func() { f.stop(t) })
	return f
}

// stop releases the fixture's own I/O, then cancels and joins Serve.
func (f *serveFixture) stop(t *testing.T) {
	t.Helper()
	f.stopOnce.Do(func() {
		f.conn.openGate()
		f.cancel()
		select {
		case err := <-f.done:
			if err != nil {
				t.Errorf("Serve = %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Errorf("Serve did not join its workers after the fixture released its I/O")
			<-f.done
		}
	})
}

// probe counts one response's releases and snapshots state at the first.
type probe struct {
	n      atomic.Int32
	done   chan struct{}
	once   sync.Once
	note   func()
	ctx    context.Context // the handler's request context, set before construction
	called chan struct{}
}

func newProbe() *probe { return &probe{done: make(chan struct{}), called: make(chan struct{})} }

func (p *probe) release() {
	p.n.Add(1)
	p.once.Do(func() {
		if p.note != nil {
			p.note()
		}
		close(p.done)
	})
}

func waitFor(t *testing.T, ch <-chan struct{}, d time.Duration, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(d):
		t.Fatalf("timed out after %v waiting for %s", d, what)
	}
}

// probedHandler answers each operation with a body naming it and a release
// recorded on probes[op].
func probedHandler(probes map[string]*probe) AttachmentHandler {
	return func(ctx context.Context, _ Peer, in Intent) (*AttachmentResponse, error) {
		p := probes[in.OperationID]
		p.ctx = ctx
		close(p.called)
		body, _ := json.Marshal(map[string]string{"answer": in.OperationID})
		return NewAttachmentResponse(Result{Revision: 7, Body: body}, p.release)
	}
}

func sameUser() Peer { return Peer{UID: 1000, PID: 42, SameUser: true} }

// fillQueue puts op_a in flight behind the blocked Respond and op_b in the
// one pending slot, asserting each was released after queue acceptance and
// before its socket write completed.
func fillQueue(t *testing.T, conn *fakeConn, probes map[string]*probe) {
	t.Helper()
	conn.send(t, intent("op_a", "attachment.read"))
	waitFor(t, probes["op_a"].done, 5*time.Second, "op_a released while its Respond is blocked")
	if got := conn.returned.Load(); got != 0 {
		t.Fatalf("op_a released only after %d socket writes completed; want release before write completion", got)
	}
	conn.send(t, intent("op_b", "attachment.read"))
	waitFor(t, probes["op_b"].done, 5*time.Second, "op_b released on pending-queue acceptance")
	if got := conn.calls.Load(); got > 1 {
		t.Fatalf("Respond entered %d times when op_b was released; op_b must be released while still pending", got)
	}
}

// TestAttachmentQueueHandoffBoundary drives real Serve: op_a in flight, op_b
// pending, op_c refused by the full queue. Each release runs once, after
// queue acceptance and never after the socket write, and the refused
// response is released only on its deadline with zero bytes exposed.
// I06/I21 (v2 §§3/14).
func TestAttachmentQueueHandoffBoundary(t *testing.T) {
	t.Parallel()
	probes := map[string]*probe{"op_a": newProbe(), "op_b": newProbe(), "op_c": newProbe()}
	srv := NewServer(nil)
	if err := srv.RegisterAttachment("attachment.read", probedHandler(probes)); err != nil {
		t.Fatal(err)
	}
	conn := newFakeConn(sameUser(), true)
	f := startServe(t, srv, nil, conn)

	fillQueue(t, conn, probes)
	var cExpired, cAfterWrite bool
	probes["op_c"].note = func() {
		cExpired = probes["op_c"].ctx.Err() != nil
		cAfterWrite = conn.returned.Load() > 0
	}
	conn.send(t, intent("op_c", "attachment.read"))
	waitFor(t, probes["op_c"].done, attachmentRequestBudget+3*time.Second, "op_c released")
	if !cExpired && !cAfterWrite {
		t.Fatalf("op_c released while the queue was full and its request was live: release preceded queue acceptance")
	}
	waitFor(t, conn.closed, 5*time.Second, "the server ending the connection after op_c's deadline")

	f.stop(t)
	got := conn.published()
	if len(got) != 1 || got[0].OperationID != "op_a" || string(got[0].Body) != `{"answer":"op_a"}` || got[0].Revision != 7 {
		t.Fatalf("published %+v; want only op_a's frame", got)
	}
	for op, p := range probes {
		if n := p.n.Load(); n != 1 {
			t.Errorf("%s released %d times, want exactly once", op, n)
		}
	}
}

// TestAttachmentQueueBoundAndCancellation pins the per-connection bound: a
// request behind a full queue ends within its two-second budget, exposes no
// bytes, grows nothing, and every failure releases exactly once.
// I06/I21 (v2 §§3/14).
func TestAttachmentQueueBoundAndCancellation(t *testing.T) {
	t.Parallel()
	type ended func(t *testing.T, f *serveFixture)
	cases := []struct {
		name      string
		failWrite bool
		end       ended
		wantErr   error
	}{
		{name: "deadline ends the request within its budget", wantErr: context.DeadlineExceeded},
		{name: "serve cancellation releases the queued request", wantErr: context.Canceled,
			end: func(_ *testing.T, f *serveFixture) { f.cancel() }},
		{name: "failed socket write releases the queued request", failWrite: true, wantErr: context.Canceled,
			end: func(_ *testing.T, f *serveFixture) { f.conn.openGate() }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			probes := map[string]*probe{"op_a": newProbe(), "op_b": newProbe(), "op_c": newProbe()}
			srv := NewServer(nil)
			if err := srv.RegisterAttachment("attachment.read", probedHandler(probes)); err != nil {
				t.Fatal(err)
			}
			conn := newFakeConn(sameUser(), true)
			conn.failWrite = tc.failWrite
			f := startServe(t, srv, nil, conn)
			fillQueue(t, conn, probes)

			start := time.Now()
			conn.send(t, intent("op_c", "attachment.read"))
			waitFor(t, probes["op_c"].called, 5*time.Second, "op_c's handler")
			if tc.end != nil {
				tc.end(t, f)
			}
			waitFor(t, probes["op_c"].done, attachmentRequestBudget+time.Second, "op_c released")
			if elapsed := time.Since(start); elapsed > attachmentRequestBudget+time.Second {
				t.Errorf("op_c ended after %v, beyond its %v budget", elapsed, attachmentRequestBudget)
			}
			if err := context.Cause(probes["op_c"].ctx); !errors.Is(err, tc.wantErr) {
				t.Errorf("op_c ended with %v, want %v", err, tc.wantErr)
			}
			waitFor(t, conn.closed, 5*time.Second, "the connection closing")
			f.stop(t)
			for _, r := range conn.published() {
				if r.OperationID != "op_a" {
					t.Errorf("published %s after the queue was full; only the in-flight op_a may be written", r.OperationID)
				}
			}
			if n := conn.calls.Load(); n != 1 {
				t.Errorf("Respond called %d times, want only the in-flight frame", n)
			}
			for op, p := range probes {
				if n := p.n.Load(); n != 1 {
					t.Errorf("%s released %d times, want exactly once", op, n)
				}
			}
		})
	}
}

// TestAttachmentResponseFailuresReleaseOnce covers the paths that end
// before queue acceptance: an oversize frame, a handler error carrying a
// response and a handler with no response. Each releases once and exposes
// only a constant error.
func TestAttachmentResponseFailuresReleaseOnce(t *testing.T) {
	t.Parallel()
	const sentinel = "synthetic-unexpected-detail-0451"
	oversize := `"` + strings.Repeat("x", AttachmentFrameBytes-2) + `"`
	cases := []struct {
		name    string
		answer  func(p *probe) (*AttachmentResponse, error)
		release int32
		code    Code
	}{
		{name: "oversize encoded frame is discarded", release: 1, code: CodePersistenceUnavailable,
			answer: func(p *probe) (*AttachmentResponse, error) {
				return NewAttachmentResponse(Result{Body: json.RawMessage(oversize)}, p.release)
			}},
		{name: "response beside an error is discarded", release: 1, code: CodePersistenceUnavailable,
			answer: func(p *probe) (*AttachmentResponse, error) {
				r, err := NewAttachmentResponse(Result{Body: json.RawMessage(`{"token":"` + sentinel + `"}`)}, p.release)
				if err != nil {
					return nil, err
				}
				return r, errors.New(sentinel)
			}},
		{name: "control error keeps its code", release: 0, code: CodeRevisionConflict,
			answer: func(*probe) (*AttachmentResponse, error) {
				return nil, newError(CodeRevisionConflict, "stale grant")
			}},
		{name: "nil response is a constant failure", release: 0, code: CodePersistenceUnavailable,
			answer: func(*probe) (*AttachmentResponse, error) { return nil, nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := newProbe()
			srv := NewServer(nil)
			if err := srv.RegisterAttachment("attachment.open", func(context.Context, Peer, Intent) (*AttachmentResponse, error) {
				return tc.answer(p)
			}); err != nil {
				t.Fatal(err)
			}
			conn := newFakeConn(sameUser(), false)
			f := startServe(t, srv, nil, conn)
			conn.send(t, intent("op_fail", "attachment.open"))
			frame := <-conn.entered
			f.stop(t)
			var r Result
			if err := json.Unmarshal(frame[prefixLen:], &r); err != nil {
				t.Fatal(err)
			}
			if r.OperationID != "op_fail" || r.Error == nil || r.Error.Code != tc.code || len(r.Body) != 0 {
				t.Fatalf("reply %+v, want %s for op_fail with no body", r, tc.code)
			}
			if bytes.Contains(frame, []byte(sentinel)) {
				t.Fatalf("reply discloses the raw sentinel: %s", frame)
			}
			if n := p.n.Load(); n != tc.release {
				t.Fatalf("released %d times, want %d", n, tc.release)
			}
		})
	}
}

// TestAttachmentResponseIsImmutableCopy shows the queued bytes come from
// the constructor's copy: mutating the caller's result afterwards changes
// nothing published.
func TestAttachmentResponseIsImmutableCopy(t *testing.T) {
	t.Parallel()
	p := newProbe()
	body := []byte(`{"label":"original"}`)
	resErr := &Error{Code: CodeRevisionConflict, Message: "original"}
	srv := NewServer(nil)
	if err := srv.RegisterAttachment("attachment.read", func(context.Context, Peer, Intent) (*AttachmentResponse, error) {
		r, err := NewAttachmentResponse(Result{OperationID: "op_spoof", Revision: 3, Body: body, Error: resErr}, p.release)
		copy(body, `{"label":"replaced"}`)
		resErr.Message = "replaced"
		return r, err
	}); err != nil {
		t.Fatal(err)
	}
	conn := newFakeConn(sameUser(), false)
	f := startServe(t, srv, nil, conn)
	conn.send(t, intent("op_copy", "attachment.read"))
	frame := <-conn.entered
	f.stop(t)
	want := `{"operation_id":"op_copy","revision":3,"body":{"label":"original"},"error":{"code":"revision_conflict","message":"original"}}`
	if got := string(frame[prefixLen:]); got != want {
		t.Fatalf("published %s, want the constructor's copy %s", got, want)
	}
	if n := p.n.Load(); n != 1 {
		t.Fatalf("released %d times, want 1", n)
	}
}

// TestNewAttachmentResponseRefusesInvalidInput keeps the cleanup obligation
// with the caller when construction fails.
func TestNewAttachmentResponseRefusesInvalidInput(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		result  Result
		release bool
	}{
		{name: "nil release", result: Result{Body: json.RawMessage(`{}`)}},
		{name: "body over the attachment limit", result: Result{Body: json.RawMessage(`"` + strings.Repeat("x", AttachmentFrameBytes) + `"`)}, release: true},
		{name: "body that is not JSON", result: Result{Body: json.RawMessage(`{"open":`)}, release: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var released atomic.Int32
			var release func()
			if tc.release {
				release = func() { released.Add(1) }
			}
			r, err := NewAttachmentResponse(tc.result, release)
			if err == nil || r != nil {
				t.Fatalf("NewAttachmentResponse = %v, %v; want a refusal", r, err)
			}
			if released.Load() != 0 {
				t.Fatalf("refused construction ran the caller's release")
			}
		})
	}
}

// TestRegisterAttachmentRefusesInvalidRegistration restricts the table to
// attachment.open/read/close with one non-nil handler each.
func TestRegisterAttachmentRefusesInvalidRegistration(t *testing.T) {
	t.Parallel()
	h := func(context.Context, Peer, Intent) (*AttachmentResponse, error) { return nil, nil }
	srv := NewServer(nil)
	if err := srv.RegisterAttachment("attachment.read", h); err != nil {
		t.Fatalf("first registration: %v", err)
	}
	for _, tc := range []struct {
		name, method string
		h            AttachmentHandler
	}{
		{name: "duplicate", method: "attachment.read", h: h},
		{name: "attachment.stop is not transport-only", method: "attachment.stop", h: h},
		{name: "ordinary method", method: "status", h: h},
		{name: "unknown method", method: "attachment.subscribe", h: h},
		{name: "nil handler", method: "attachment.open", h: nil},
	} {
		if err := srv.RegisterAttachment(tc.method, tc.h); err == nil || !strings.Contains(err.Error(), tc.method) {
			t.Errorf("%s: RegisterAttachment(%q) = %v, want an error naming the method", tc.name, tc.method, err)
		}
	}
}

// TestAttachmentExecutorBypassesOperationPersistence serves a synthetic
// token-bearing attachment reply through real Serve on a real ledger: no
// operations row and no raw token byte reach the database files, while an
// ordinary request on the same connection still records its operation.
// I18/I23 (v2 §§8/14): no durable raw grant.
func TestAttachmentExecutorBypassesOperationPersistence(t *testing.T) {
	t.Parallel()
	const sentinel = "synthetic-raw-token-sentinel-7f3a"
	db, dir := openLedger(t)
	var handled atomic.Int32
	p := newProbe()
	srv := NewServer(map[string]Handler{"ping": func(context.Context, Peer, Intent) (Result, error) {
		return Result{Body: json.RawMessage(`{"pong":true}`)}, nil
	}})
	if err := srv.RegisterAttachment("attachment.read", func(context.Context, Peer, Intent) (*AttachmentResponse, error) {
		handled.Add(1)
		return NewAttachmentResponse(Result{Body: json.RawMessage(`{"token":"` + sentinel + `"}`)}, p.release)
	}); err != nil {
		t.Fatal(err)
	}

	conn := newFakeConn(sameUser(), false)
	f := startServe(t, srv, db, conn)
	conn.send(t, intent("op_att", "attachment.read"))
	conn.send(t, intent("op_unknown", "attachment.close"))
	conn.send(t, intent("op_ord", "ping"))
	var replies []Result
	for range 3 {
		var r Result
		if err := json.Unmarshal((<-conn.entered)[prefixLen:], &r); err != nil {
			t.Fatal(err)
		}
		replies = append(replies, r)
	}
	f.stop(t)
	if r := replies[0]; r.OperationID != "op_att" || r.Error != nil || string(r.Body) != `{"token":"`+sentinel+`"}` {
		t.Fatalf("attachment reply %+v, want the synthetic body for op_att", r)
	}
	if r := replies[1]; r.OperationID != "op_unknown" || r.Error == nil || r.Error.Code != CodeCapabilityUnsupported {
		t.Fatalf("unregistered attachment.close reply %+v, want capability_unsupported", r)
	}
	if r := replies[2]; r.OperationID != "op_ord" || r.Error != nil || string(r.Body) != `{"pong":true}` {
		t.Fatalf("ordinary reply %+v, want pong", r)
	}
	var ops []string
	rows, err := db.Query(`SELECT operation_id FROM operations ORDER BY operation_id`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ops = append(ops, id)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	if len(ops) != 1 || ops[0] != "op_ord" {
		t.Fatalf("operations rows %v, want only the ordinary op_ord", ops)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		raw, err := root.ReadFile(e.Name())
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(raw, []byte(sentinel)) {
			t.Fatalf("%s stores the raw token sentinel", e.Name())
		}
	}
	if n := handled.Load(); n != 1 {
		t.Fatalf("attachment handler ran %d times, want 1", n)
	}
	if n := p.n.Load(); n != 1 {
		t.Fatalf("released %d times, want 1", n)
	}
}

// TestAttachmentRefusalsPrecedeTheHandler: an unauthenticated peer is
// refused before the handler, and direct Dispatch cannot reach the table.
// I03 (v2 §2).
func TestAttachmentRefusalsPrecedeTheHandler(t *testing.T) {
	t.Parallel()
	db, _ := openLedger(t)
	var handled atomic.Int32
	srv := NewServer(nil)
	if err := srv.RegisterAttachment("attachment.read", func(context.Context, Peer, Intent) (*AttachmentResponse, error) {
		handled.Add(1)
		return NewAttachmentResponse(Result{}, func() {})
	}); err != nil {
		t.Fatal(err)
	}
	conn := newFakeConn(Peer{UID: 1001, PID: 7}, false)
	f := startServe(t, srv, db, conn)
	conn.send(t, intent("op_foreign", "attachment.read"))
	var r Result
	if err := json.Unmarshal((<-conn.entered)[prefixLen:], &r); err != nil {
		t.Fatal(err)
	}
	f.stop(t)
	if r.OperationID != "op_foreign" || r.Error == nil || r.Error.Code != CodePermissionDenied || len(r.Body) != 0 {
		t.Fatalf("foreign peer reply %+v, want permission_denied", r)
	}

	res, err := srv.Dispatch(WithLedger(t.Context(), db), sameUser(), intent("op_direct", "attachment.read"))
	requireCode(t, err, CodeCapabilityUnsupported)
	if len(res.Body) != 0 {
		t.Fatalf("direct Dispatch returned body %s", res.Body)
	}
	if n := handled.Load(); n != 0 {
		t.Fatalf("handler ran %d times, want 0", n)
	}
	if n := count(t, db, "operations"); n != 0 {
		t.Fatalf("operations rows %d, want 0", n)
	}
}

// TestReviewExpiredRequestWithEmptyQueuePublishesNothing: the request
// context expires before queue acceptance, with the queue empty.
func TestReviewExpiredRequestWithEmptyQueuePublishesNothing(t *testing.T) {
	for i := range 16 {
		p := newProbe()
		srv := NewServer(nil)
		if err := srv.RegisterAttachment("attachment.read", func(ctx context.Context, _ Peer, _ Intent) (*AttachmentResponse, error) {
			<-ctx.Done()
			return NewAttachmentResponse(Result{Body: json.RawMessage(`{"expired":"synthetic-expired-sentinel"}`)}, p.release)
		}); err != nil {
			t.Fatal(err)
		}
		conn := newFakeConn(sameUser(), false)
		f := startServe(t, srv, nil, conn)
		conn.send(t, intent("op_expired", "attachment.read"))
		exposed := false
		select {
		case <-conn.entered:
			exposed = true
		case <-conn.closed:
		case <-time.After(attachmentRequestBudget + time.Second):
			f.stop(t)
			t.Fatal("expired request neither closed nor answered")
		}
		f.stop(t)
		if exposed || len(conn.published()) > 0 {
			t.Fatalf("attempt %d: actual Serve published response whose request context expired before handoff into an empty queue", i)
		}
		if p.n.Load() != 1 {
			t.Fatalf("release count=%d, want 1", p.n.Load())
		}
	}
}

func TestReviewOversizedTypedErrorNeverReachesRespond(t *testing.T) {
	srv := NewServer(nil)
	if err := srv.RegisterAttachment("attachment.read", func(context.Context, Peer, Intent) (*AttachmentResponse, error) {
		return nil, newError(CodeRevisionConflict, "%s", strings.Repeat("synthetic-error-", 5000))
	}); err != nil {
		t.Fatal(err)
	}
	conn := newFakeConn(sameUser(), false)
	f := startServe(t, srv, nil, conn)
	conn.send(t, intent("op_bigerror", "attachment.read"))
	var frame []byte
	select {
	case frame = <-conn.entered:
	case <-conn.closed:
	case <-time.After(3 * time.Second):
		f.stop(t)
		t.Fatal("no bounded error response or closed connection")
	}
	f.stop(t)
	if len(frame) > AttachmentFrameBytes {
		t.Fatalf("actual Serve handed Respond %d-byte attachment error frame; maximum %d", len(frame), AttachmentFrameBytes)
	}
}

// TestOversizedTypedErrorKeepsItsCode pins the bounded replacement frame: the
// handler's error code survives, its oversized message does not.
func TestOversizedTypedErrorKeepsItsCode(t *testing.T) {
	srv := NewServer(nil)
	if err := srv.RegisterAttachment("attachment.read", func(context.Context, Peer, Intent) (*AttachmentResponse, error) {
		return nil, newError(CodeRevisionConflict, "%s", strings.Repeat("synthetic-error-", 5000))
	}); err != nil {
		t.Fatal(err)
	}
	conn := newFakeConn(sameUser(), false)
	f := startServe(t, srv, nil, conn)
	conn.send(t, intent("op_bigerror", "attachment.read"))
	var frame []byte
	select {
	case frame = <-conn.entered:
	case <-time.After(3 * time.Second):
		f.stop(t)
		t.Fatal("no bounded error response")
	}
	f.stop(t)
	var r Result
	if err := json.Unmarshal(frame[prefixLen:], &r); err != nil {
		t.Fatal(err)
	}
	if r.OperationID != "op_bigerror" || r.Error == nil || r.Error.Code != CodeRevisionConflict {
		t.Fatalf("error frame = %+v, want op_bigerror with code %s", r, CodeRevisionConflict)
	}
	if strings.Contains(r.Error.Message, "synthetic-error-") {
		t.Fatalf("bounded error frame still carries the oversized message")
	}
}

// The earliest transport authentication refusal is identical for a registered
// target and an unknown one; neither lookup nor handler may reveal a scope.
func TestAttachmentForeignPeerConstantPublicRefusal(t *testing.T) {
	t.Parallel()
	var handled atomic.Int32
	var want []byte
	for _, run := range []string{"run_known", "run_unknown"} {
		srv := NewServer(nil)
		if err := srv.RegisterAttachment("attachment.read", func(context.Context, Peer, Intent) (*AttachmentResponse, error) {
			handled.Add(1)
			return NewAttachmentResponse(Result{Body: json.RawMessage(`{}`)}, func() {})
		}); err != nil {
			t.Fatal(err)
		}
		conn := newFakeConn(Peer{UID: 1001, PID: 7}, false)
		f := startServe(t, srv, nil, conn)
		in := intent("op_same", "attachment.read")
		in.Object = run
		conn.send(t, in)
		frame := <-conn.entered
		f.stop(t)
		if want == nil {
			want = bytes.Clone(frame)
		} else if !bytes.Equal(want, frame) {
			t.Fatalf("foreign refusal differs: %s != %s", want, frame)
		}
		var result Result
		if err := json.Unmarshal(frame[prefixLen:], &result); err != nil {
			t.Fatal(err)
		}
		if result.Error == nil || result.Error.Code != CodePermissionDenied || result.Error.Message != "attachment unavailable" || result.Revision != 0 || (len(result.Body) != 0 && string(result.Body) != "null") {
			t.Fatalf("foreign public refusal: %+v", result)
		}
	}
	if handled.Load() != 0 {
		t.Fatal("foreign peer reached attachment handler")
	}
}
