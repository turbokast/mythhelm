package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/control"
	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/migrate"
	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/v2contract"
)

// Endpoint discovery is process-wide, so these fixtures are not parallel.
func isolatedRoot(t *testing.T) string {
	t.Helper()
	base, err := os.MkdirTemp(os.Getenv("MYTHHELM_T6_FIXTURE_PARENT"), "mh-runtime-") //nolint:usetesting // Short socket paths and parent-owned subprocess cleanup require this fresh child directory.
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(base) }) // #nosec G703 -- MkdirTemp created this fresh owned child; cleanup never removes the supplied parent.
	rt := filepath.Join(base, "rt")
	if err := os.Mkdir(rt, 0700); err != nil { // #nosec G703 -- rt is a fixed child of the fresh owned fixture directory.
		t.Fatal(err)
	}
	t.Setenv("XDG_RUNTIME_DIR", rt)
	t.Setenv("TEMP", rt)
	t.Setenv("TMP", rt)
	t.Setenv("HOME", base)
	root := filepath.Join(base, "state")
	t.Setenv("MYTHHELM_HOME", root)
	return root
}
func testRuntime(t *testing.T) *runtime {
	t.Helper()
	r, err := openRuntime(t.Context(), isolatedRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := r.shutdown(context.Background()); err != nil && !errors.Is(err, context.Canceled) {
			t.Error(err)
		}
	})
	return r
}
func statusResult(t *testing.T, r *runtime) map[string]any {
	t.Helper()
	res, err := r.server.Dispatch(t.Context(), control.Peer{SameUser: true}, control.Intent{OperationID: ids.New("op"), Method: "status"})
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(res.Body, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestServiceRootConflict(t *testing.T) {
	r := testRuntime(t)
	root := filepath.Join(t.TempDir(), "different-root")
	_, err := openRuntime(t.Context(), root)
	var ce *v2contract.ControlError
	if !errors.Is(err, control.ErrRootConflict) && (!errors.As(err, &ce) || ce.Code != v2contract.CodeOwnershipUnresolved) {
		t.Fatalf("conflicting root admitted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, journal.DBName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("conflicting root gained database: %v", err)
	}
	if got := statusResult(t, r); got["root"] != r.lease.Root() {
		t.Fatalf("winner changed root: %v", got)
	}
}

func TestServiceRootContextSurvivesClientExit(t *testing.T) {
	root := isolatedRoot(t)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, root) }()
	defer func() {
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	tr, err := control.DefaultTransport()
	if err != nil {
		t.Fatal(err)
	}
	endpoint, err := control.EndpointPath()
	if err != nil {
		t.Fatal(err)
	}
	var first map[string]any
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		c, err := tr.Dial(endpoint)
		if err == nil {
			res, err := control.Call(t.Context(), c, control.Intent{OperationID: ids.New("op"), Method: "status"})
			_ = c.Close()
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(res.Body, &first); err != nil {
				t.Fatal(err)
			}
			break
		}
		select {
		case <-deadline.C:
			t.Fatal("service did not listen")
		case <-tick.C:
		}
	}
	clientCtx, clientCancel := context.WithCancel(t.Context())
	c, err := tr.DialContext(clientCtx, endpoint)
	if err != nil {
		t.Fatal(err)
	}
	clientCancel()
	_ = c.Close()
	c, err = tr.Dial(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	res, err := control.Call(t.Context(), c, control.Intent{OperationID: ids.New("op"), Method: "status"})
	if err != nil {
		t.Fatal(err)
	}
	var second map[string]any
	if err := json.Unmarshal(res.Body, &second); err != nil {
		t.Fatal(err)
	}
	if first["pid"] != second["pid"] || first["generation"] != second["generation"] || first["root"] != second["root"] || second["pid"] != float64(os.Getpid()) {
		t.Fatalf("view exit changed runtime: %v -> %v", first, second)
	}
}

func TestServiceUnadoptedStatusDoesNotMutate(t *testing.T) {
	root := isolatedRoot(t)
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	j, err := journal.Open(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(root, journal.DBName)) // #nosec G304 -- Fixed database name in this fixture's freshly created root.
	if err != nil {
		t.Fatal(err)
	}
	r, err := openRuntime(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.shutdown(context.Background()) }()
	if !r.migrationRequired || r.journal != nil {
		t.Fatal("unadopted root obtained a writer")
	}
	if got := statusResult(t, r); got["migration_required"] != true {
		t.Fatalf("status=%v", got)
	}
	if _, err := r.server.Dispatch(t.Context(), control.Peer{}, control.Intent{OperationID: "foreign", Method: "status"}); !errors.Is(err, &control.Error{Code: control.CodePermissionDenied}) {
		t.Fatalf("foreign status accepted: %v", err)
	}
	for _, method := range []string{"run.start", "attachment.open", "assign", "reserve", "stop"} {
		if _, err := r.server.Dispatch(t.Context(), control.Peer{SameUser: true}, control.Intent{OperationID: ids.New("op"), Method: method}); !errors.Is(err, &control.Error{Code: control.CodeCapabilityUnsupported}) {
			t.Fatalf("%s=%v", method, err)
		}
	}
	after, err := os.ReadFile(filepath.Join(root, journal.DBName)) // #nosec G304 -- Same fixed database in the owned root; compares refusal effects.
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("read-only status/unsupported changed ledger bytes")
	}
}

func TestAcceptedEntryIgnoresClientCancellation(t *testing.T) {
	r := testRuntime(t)
	client, cancel := context.WithCancel(t.Context())
	started := make(chan struct{})
	inspect := make(chan struct{})
	checked := make(chan error, 1)
	release := make(chan struct{})
	a, err := r.startEntry(client, nil, func(ctx context.Context, _ supervisor.Hooks) error {
		close(started)
		<-inspect
		checked <- ctx.Err()
		<-release
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	cancel()
	close(inspect)
	err = <-checked
	close(release)
	<-a.done
	if err != nil {
		t.Fatalf("client cancellation reached accepted work: %v", err)
	}
}

func TestPreparedReplacementNeverOpensWriter(t *testing.T) {
	for _, replaceRoot := range []bool{false, true} {
		t.Run(map[bool]string{false: "unadopted database", true: "root"}[replaceRoot], func(t *testing.T) {
			root := isolatedRoot(t)
			other := t.TempDir()
			j, err := journal.Open(t.Context(), other)
			if err != nil {
				t.Fatal(err)
			}
			_ = j.Close()
			replacement, err := os.ReadFile(filepath.Join(other, journal.DBName)) // #nosec G304 -- Fixed synthetic database created in t.TempDir above.
			if err != nil {
				t.Fatal(err)
			}
			replacementPath := ""
			admitted, err := openRuntimePrepared(t.Context(), root, func(ctx context.Context, dir string) error {
				if err := migrate.PrepareServiceRoot(ctx, dir); err != nil {
					return err
				}
				if replaceRoot {
					if err := os.Rename(dir, dir+"-old"); err != nil {
						return err
					}
					if err := os.Mkdir(dir, 0700); err != nil {
						return err
					}
				}
				replacementPath = filepath.Join(dir, journal.DBName)
				return os.WriteFile(replacementPath, replacement, 0600) // #nosec G703 -- Intentional replacement uses a fixed database name in the fresh owned test root.
			})
			if err == nil {
				_ = admitted.shutdown(context.Background())
				t.Fatal("unprepared replacement reached writable runtime")
			}
			after, readErr := os.ReadFile(replacementPath) // #nosec G304 -- Read only the intentional replacement database in the fresh owned test root.
			if readErr != nil {
				t.Fatal(readErr)
			}
			if string(after) != string(replacement) {
				t.Fatal("refused replacement database was mutated")
			}
		})
	}
}

func TestViewSlotsReleaseOnlyDisplayResources(t *testing.T) {
	r := testRuntime(t)
	views := []*view{}
	for peer := range 4 {
		for range 16 {
			v, err := r.views.open(t.Context(), fmt.Sprintf("peer%d", peer))
			if err != nil {
				t.Fatal(err)
			}
			views = append(views, v)
		}
	}
	if _, err := r.views.open(t.Context(), "fifth"); !errors.Is(err, &control.Error{Code: control.CodeCapabilityUnsupported}) {
		t.Fatalf("capacity accepted: %v", err)
	}
	views[0].close()
	v, err := r.views.open(t.Context(), "peer0")
	if err != nil {
		t.Fatalf("closed slot not reusable: %v", err)
	}
	v.close()
	for _, v := range views {
		v.close()
	}
	if r.lease.Generation() == 0 {
		t.Fatal("view closure released runtime lease")
	}
	r.views.close()
	if _, err := r.views.open(t.Context(), "closed"); err == nil {
		t.Fatal("closed view registry accepted observer")
	}
}

func TestCanceledStartupHasNoStateEffects(t *testing.T) {
	root := isolatedRoot(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := openRuntime(ctx, root)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled startup=%v", err)
	}
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cancelled startup created root: %v", err)
	}
}

// blockedStatusConn models a response writer that does not observe root
// cancellation. Its owning fixture always releases and joins it explicitly.
type blockedStatusConn struct {
	frame   []byte
	used    bool
	writing chan struct{}
	release chan struct{}
}

func (c *blockedStatusConn) Peer() control.Peer                              { return control.Peer{SameUser: true} }
func (c *blockedStatusConn) Request(context.Context, []byte) ([]byte, error) { return nil, io.EOF }
func (c *blockedStatusConn) Receive(ctx context.Context) ([]byte, error) {
	if c.used {
		return nil, io.EOF
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c.used = true
	return c.frame, nil
}
func (c *blockedStatusConn) Respond([]byte) error { close(c.writing); <-c.release; return nil }
func (c *blockedStatusConn) Close() error         { return nil }

type singleStatusListener struct {
	conn   control.Conn
	once   sync.Once
	closed chan struct{}
}

func (l *singleStatusListener) Accept() (control.Conn, error) {
	if l.conn != nil {
		c := l.conn
		l.conn = nil
		return c, nil
	}
	<-l.closed
	return nil, io.EOF
}
func (l *singleStatusListener) Close() error { l.once.Do(func() { close(l.closed) }); return nil }
func (l *singleStatusListener) Addr() string { return "synthetic-status" }

func TestRootCancellationStartsShutdownBeforeTransportJoin(t *testing.T) {
	r := testRuntime(t)
	owner, err := supervisor.AcquireServiceOwner(t.Context(), r.journal, r.lease, "run_root_cancel")
	if err != nil {
		t.Fatal(err)
	}
	seedVerifying(t, r, "run_root_cancel")
	entered, cancelled, releaseEntry := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var first sync.Once
	a, err := r.startEntry(t.Context(), owner, func(ctx context.Context, h supervisor.Hooks) error {
		h.Event = func(journal.Event) {
			first.Do(func() { close(entered); <-ctx.Done(); close(cancelled); <-releaseEntry })
		}
		_, err := supervisor.RecoverOwned(ctx, r.journal, owner, "run_root_cancel", h)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	frame, err := control.Encode(control.Intent{OperationID: "op_status", Method: "status"})
	if err != nil {
		t.Fatal(err)
	}
	c := &blockedStatusConn{frame: frame, writing: make(chan struct{}), release: make(chan struct{})}
	l := &singleStatusListener{conn: c, closed: make(chan struct{})}
	rootCtx, cancel := context.WithCancel(t.Context())
	serveDone := make(chan error, 1)
	go func() { serveDone <- serveRuntime(rootCtx, r, l) }()
	var entryOnce, writerOnce sync.Once
	cleanup := func() {
		cancel()
		writerOnce.Do(func() { close(c.release) })
		entryOnce.Do(func() { close(releaseEntry) })
		<-a.done
		err := <-serveDone
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Error(err)
		}
	}
	defer cleanup()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("owned callback was not reached")
	}
	select {
	case <-c.writing:
	case <-time.After(2 * time.Second):
		t.Fatal("status writer was not reached")
	}
	cancel()
	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("root cancellation waited for stalled transport before cancelling owned entry")
	}
	r.mu.Lock()
	closing := r.closing
	r.mu.Unlock()
	if !closing {
		t.Fatal("root cancellation did not refuse new work")
	}
	select {
	case <-r.joined:
		t.Fatal("shutdown released ownership before callback joined")
	default:
	}
	if r.lease.Generation() == 0 {
		t.Fatal("stalled owned callback lost its lease")
	}
	entryOnce.Do(func() { close(releaseEntry) })
	<-a.done
	select {
	case <-r.joined:
	case <-time.After(2 * time.Second):
		t.Fatal("joined entry did not finish ownership cleanup")
	}
	if r.lease.Generation() != 0 {
		t.Fatal("joined ownership remained held")
	}
	// The writer is still held: execution cleanup never waits for a view.
	select {
	case <-c.release:
		t.Fatal("fixture writer unexpectedly released")
	default:
	}
}

// Injected resolver failures exercise the same path Run uses with real platform
// resolvers. They do not qualify an unavailable OS or native endpoint setup.
func TestServiceResolverFailureHasNoStateEffects(t *testing.T) {
	for _, failTransport := range []bool{true, false} {
		t.Run(fmt.Sprintf("transport_%v", failTransport), func(t *testing.T) {
			root := isolatedRoot(t)
			sentinel := errors.New("resolver unavailable")
			endpointCalled := false
			err := runResolved(t.Context(), root, func() (control.Transport, error) {
				if failTransport {
					return nil, sentinel
				}
				return control.DefaultTransport()
			}, func() (string, error) { endpointCalled = true; return "", sentinel })
			if !errors.Is(err, sentinel) {
				t.Fatalf("resolver failure=%v", err)
			}
			if endpointCalled == failTransport {
				t.Fatal("resolver ordering changed")
			}
			if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("failed preflight created state root: %v", err)
			}
		})
	}
	root := isolatedRoot(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := runResolved(ctx, root, func() (control.Transport, error) { t.Fatal("cancelled startup resolved transport"); return nil, nil }, func() (string, error) { t.Fatal("cancelled startup resolved endpoint"); return "", nil })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled preflight=%v", err)
	}
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cancelled preflight created root: %v", err)
	}
}
