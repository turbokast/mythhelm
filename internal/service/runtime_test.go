package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	base, err := os.MkdirTemp(os.Getenv("MYTHHELM_T6_FIXTURE_PARENT"), "mh-runtime-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	rt := filepath.Join(base, "rt")
	if err := os.Mkdir(rt, 0700); err != nil {
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
	before, err := os.ReadFile(filepath.Join(root, journal.DBName))
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
	after, err := os.ReadFile(filepath.Join(root, journal.DBName))
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
			replacement, err := os.ReadFile(filepath.Join(other, journal.DBName))
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
				return os.WriteFile(replacementPath, replacement, 0600)
			})
			if err == nil {
				_ = admitted.shutdown(context.Background())
				t.Fatal("unprepared replacement reached writable runtime")
			}
			after, readErr := os.ReadFile(replacementPath)
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
