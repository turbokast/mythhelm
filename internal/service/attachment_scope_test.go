package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	goruntime "runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/supervisor"
)

// registerActor adds an existing actor for owner to r's roster, as startEntry
// does, with no work behind it. It is removed before the runtime's cleanup.
func registerActor(t *testing.T, r *runtime, owner *supervisor.ServiceOwner) *actor {
	t.Helper()
	done := make(chan struct{})
	close(done)
	a := &actor{owner: owner, cancel: func() {}, done: done}
	r.mu.Lock()
	r.entries[a] = struct{}{}
	r.mu.Unlock()
	t.Cleanup(func() { unregister(r, a) })
	return a
}

func unregister(r *runtime, a *actor) {
	r.mu.Lock()
	delete(r.entries, a)
	r.mu.Unlock()
}

func ownRun(t *testing.T, r *runtime, runID string) *supervisor.ServiceOwner {
	t.Helper()
	owner, err := supervisor.AcquireServiceOwner(t.Context(), r.journal, r.lease, runID)
	if err != nil {
		t.Fatalf("AcquireServiceOwner(%s): %v", runID, err)
	}
	t.Cleanup(func() { _ = owner.Close() })
	return owner
}

func runLockHeld(t *testing.T, r *runtime, runID string) bool {
	t.Helper()
	release, err := supervisor.AcquireOwner(filepath.Join(r.journal.StateDir(), "runs", runID))
	if err == nil {
		release()
		return false
	}
	if !errors.Is(err, supervisor.ErrOwnerHeld) {
		t.Fatalf("probing run lock %s: %v", runID, err)
	}
	return true
}

// closeReleases reports whether owner.Close returns within five seconds,
// proving no observation is retained. Callers release every fixture-owned
// scope first; Close is always joined, so a leaked observation logs the
// diagnostic and parks rather than returning with Close still running.
func closeReleases(t *testing.T, owner *supervisor.ServiceOwner) bool {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- owner.Close() }()
	prompt := true
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
		return true
	case <-time.After(5 * time.Second):
		prompt = false
	}
	t.Log("owner.Close still parked after 5s: an observation is retained; joining Close")
	if err := <-done; err != nil {
		t.Errorf("Close: %v", err)
	}
	return prompt
}

// gateFree reports whether a write guard is immediately available, proving no
// read or write guard leaked.
func gateFree(t *testing.T, r *runtime) bool {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	release, err := r.authority.acquire(ctx, true)
	if err != nil {
		return false
	}
	release()
	return true
}

// waitParked polls goroutine stacks until one is blocked in frame.
func waitParked(t *testing.T, frame string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		buf := make([]byte, 1<<20)
		buf = buf[:goruntime.Stack(buf, true)]
		for g := range strings.SplitSeq(string(buf), "\n\n") {
			if strings.Contains(g, frame) && (strings.Contains(g, "select") || strings.Contains(g, "sync.(*Cond).Wait(")) {
				return
			}
		}
		goruntime.Gosched()
	}
	t.Fatalf("no goroutine parked in %s", frame)
}

// waitMutexParked polls goroutine stacks until one is blocked acquiring a
// sync.Mutex inside frame.
func waitMutexParked(t *testing.T, frame string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		buf := make([]byte, 1<<20)
		buf = buf[:goruntime.Stack(buf, true)]
		for g := range strings.SplitSeq(string(buf), "\n\n") {
			if strings.Contains(g, frame) && strings.Contains(g, "sync.(*Mutex).Lock") {
				return
			}
		}
		goruntime.Gosched()
	}
	t.Fatalf("no goroutine blocked on a mutex in %s", frame)
}

func seedRunState(t *testing.T, r *runtime, runID, state string) {
	t.Helper()
	now := time.Now().UTC()
	if err := r.journal.Transact(t.Context(), func(tx *sql.Tx) error {
		return journal.InsertRun(t.Context(), tx, journal.RunRow{RunID: runID, State: state, AdapterID: "fake", SourceRepo: "synthetic", TaskSHA256: "synthetic", BillingPosture: "local-scripted", ExecutionProfile: "trusted-host", CreatedAt: now, UpdatedAt: now})
	}); err != nil {
		t.Fatal(err)
	}
}

func attemptsAbsent(t *testing.T, r *runtime, runID string) bool {
	t.Helper()
	_, err := os.Stat(filepath.Join(r.journal.StateDir(), "runs", runID, "attempts"))
	return errors.Is(err, os.ErrNotExist)
}

// attachmentEffects records exact selected ledger rows and every run artefact,
// including missing directories. It observes effects, not invented call counts.
type attachmentEffects struct {
	Ledger map[string][][]any
	Files  map[string][32]byte
}

func snapshotAttachmentEffects(t *testing.T, r *runtime) attachmentEffects {
	t.Helper()
	out := attachmentEffects{Ledger: map[string][][]any{}, Files: map[string][32]byte{}}
	if err := r.journal.Transact(t.Context(), func(tx *sql.Tx) error {
		for table, query := range map[string]string{
			"runs":                  "SELECT * FROM runs ORDER BY rowid",
			"attempts":              "SELECT * FROM attempts ORDER BY rowid",
			"run_assignments":       "SELECT * FROM run_assignments ORDER BY rowid",
			"journal":               "SELECT * FROM journal ORDER BY rowid",
			"run_control_revisions": "SELECT * FROM run_control_revisions ORDER BY rowid",
			"operations":            "SELECT * FROM operations ORDER BY rowid",
		} {
			rows, err := tx.QueryContext(t.Context(), query)
			if err != nil {
				return err
			}
			cols, err := rows.Columns()
			if err != nil {
				_ = rows.Close()
				return err
			}
			for rows.Next() {
				values := make([]any, len(cols))
				dest := make([]any, len(cols))
				for i := range values {
					dest[i] = &values[i]
				}
				if err := rows.Scan(dest...); err != nil {
					_ = rows.Close()
					return err
				}
				for i, v := range values {
					if b, ok := v.([]byte); ok {
						values[i] = string(b)
					}
				}
				out.Ledger[table] = append(out.Ledger[table], values)
			}
			err = rows.Err()
			_ = rows.Close()
			if err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	runs := filepath.Join(r.journal.StateDir(), "runs")
	root, err := os.OpenRoot(runs)
	if errors.Is(err, os.ErrNotExist) {
		return out
	}
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	err = filepath.WalkDir(runs, func(path string, entry fs.DirEntry, err error) error {
		if errors.Is(err, os.ErrNotExist) && path == runs {
			return nil
		}
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(runs, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			out.Files[rel+"/"] = [32]byte{}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		// A regular zero-length file has exactly known empty bytes. Reading
		// byte zero of a held empty owner.lock fails on Windows, whose lock
		// is mandatory. Nonempty artefacts still require actual reads.
		if info.Mode().IsRegular() && info.Size() == 0 {
			out.Files[rel] = sha256.Sum256(nil)
			return nil
		}
		raw, err := root.ReadFile(rel)
		if err != nil {
			return err
		}
		out.Files[rel] = sha256.Sum256(raw)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func requireAttachmentEffectsUnchanged(t *testing.T, r *runtime, before attachmentEffects) {
	t.Helper()
	if after := snapshotAttachmentEffects(t, r); !reflect.DeepEqual(after, before) {
		t.Fatalf("attachment changed exact ledger/run/attempt/launch artefacts: before=%+v after=%+v", before, after)
	}
}

// The runtime proof is backed by a source-call witness: observation paths may
// route/retain but must never call an acquisition or execution entrypoint.
func requireObservationCallsNoExecution(t *testing.T) {
	t.Helper()
	check := func(n ast.Node) {
		ast.Inspect(n, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			name := ""
			switch fun := call.Fun.(type) {
			case *ast.SelectorExpr:
				name = fun.Sel.Name
			case *ast.Ident:
				name = fun.Name
			}
			switch name {
			case "AcquireServiceOwner", "AcquireOwner", "Recover", "RecoverWithHooks", "RecoverOwned", "Run", "RunOwned", "Spawn":
				t.Errorf("observation path calls forbidden execution entrypoint %s", name)
			}
			return true
		})
	}
	for _, path := range []string{"attachment_scope.go", "../supervisor/service_owner.go"} {
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		if path == "attachment_scope.go" {
			check(f)
			continue
		}
		for _, decl := range f.Decls {
			fun, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			switch fun.Name.Name {
			case "MatchesRun", "RetainObservation", "verifyIdentity", "sameRoot":
				check(fun)
			}
		}
	}
}

// TestAttachmentScopeUsesExistingActor: an attachment scope routes to exactly
// one existing actor owner of at most 64 candidates and retains it; it never
// acquires, recovers or launches (I06/I18 (v2 §3): presentation never
// acquires execution authority).
func TestAttachmentScopeUsesExistingActor(t *testing.T) {
	// Sequential: endpoint discovery and the instance lock are process-wide.
	r := testRuntime(t)
	const runID = "run_attach"
	seedRunState(t, r, runID, "blocked") // terminal with a retained valid owner
	owner := ownRun(t, r, runID)
	other := ownRun(t, r, "run_other")
	valid := registerActor(t, r, owner)
	registerActor(t, r, other)
	registerActor(t, r, &supervisor.ServiceOwner{})
	fillers := []*actor{}
	for len(r.entries) < attachmentCandidateLimit {
		fillers = append(fillers, registerActor(t, r, nil))
	}
	requireObservationCallsNoExecution(t)
	initialEffects := snapshotAttachmentEffects(t, r)
	before, err := r.journal.Run(t.Context(), runID)
	if err != nil {
		t.Fatal(err)
	}

	scope, err := r.retainAttachmentScope(t.Context(), runID, false)
	if err != nil {
		t.Fatalf("64 candidates with one valid owner: %v", err)
	}
	t.Cleanup(scope.release)
	if scope.owner != owner || scope.runID != runID {
		t.Fatalf("scope routed to %p/%s; want the existing owner %p/%s", scope.owner, scope.runID, owner, runID)
	}
	scope.release()
	scope.release() // duplicate release is safe
	requireAttachmentEffectsUnchanged(t, r, initialEffects)

	t.Run("cancellation between candidate checks", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		before := snapshotAttachmentEffects(t, r)
		checked := 0
		r.attachmentCandidateChecked = func(n int) {
			checked = n
			if !r.mu.TryLock() {
				t.Fatal("candidate traversal retained the roster mutex")
			}
			r.mu.Unlock()
			if n == 1 {
				cancel()
			}
		}
		defer func() { r.attachmentCandidateChecked = nil }()
		got, err := r.retainAttachmentScope(ctx, runID, false)
		if got != nil {
			got.release()
		}
		if !errors.Is(err, context.Canceled) || got != nil || checked != 1 {
			t.Fatalf("between-candidate cancellation: checked=%d scope=%v err=%v", checked, got, err)
		}
		requireAttachmentEffectsUnchanged(t, r, before)
		if !gateFree(t, r) || !runLockHeld(t, r, runID) {
			t.Fatal("cancellation leaked gate or released owner")
		}
	})
	t.Run("closed prior generation refuses after actual rollover", func(t *testing.T) {
		old := testRuntime(t)
		const staleRun = "run_prior_generation"
		oldOwner := ownRun(t, old, staleRun)
		generation := old.lease.Generation()
		root := old.journal.StateDir()
		if err := oldOwner.Close(); err != nil {
			t.Fatal(err)
		}
		if err := old.shutdown(t.Context()); err != nil {
			t.Fatal(err)
		}
		current, err := openRuntime(t.Context(), root)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := current.shutdown(context.Background()); err != nil {
				t.Error(err)
			}
		})
		a := registerActor(t, current, oldOwner)
		defer unregister(current, a)
		if current.lease.Generation() <= generation || old.lease.Generation() != 0 || !oldOwner.MatchesRun(staleRun) {
			t.Fatalf("fixture prior=%d current=%d old-held=%d matcher=%t", generation, current.lease.Generation(), old.lease.Generation(), oldOwner.MatchesRun(staleRun))
		}
		before := snapshotAttachmentEffects(t, current)
		got, err := current.retainAttachmentScope(t.Context(), staleRun, false)
		if got != nil {
			got.release()
		}
		if !errors.Is(err, supervisor.ErrOwnership) || got != nil {
			t.Fatalf("prior-generation closed owner: scope=%v err=%v", got, err)
		}
		requireAttachmentEffectsUnchanged(t, current, before)
		if !gateFree(t, current) {
			t.Fatal("prior-generation refusal leaked a gate")
		}
	})

	t.Run("matcher true foreign root is not owner authority", func(t *testing.T) {
		foreign := testRuntime(t)
		foreignOwner := ownRun(t, foreign, runID)
		if !foreignOwner.MatchesRun(runID) {
			t.Fatal("foreign real owner must match routing identity")
		}
		unregister(r, valid)
		a := registerActor(t, r, foreignOwner)
		defer func() { unregister(r, a); r.mu.Lock(); r.entries[valid] = struct{}{}; r.mu.Unlock() }()
		before := snapshotAttachmentEffects(t, r)
		got, err := r.retainAttachmentScope(t.Context(), runID, false)
		if got != nil {
			got.release()
		}
		if !errors.Is(err, supervisor.ErrOwnership) || got != nil {
			t.Fatalf("foreign owner: scope=%v err=%v", got, err)
		}
		requireAttachmentEffectsUnchanged(t, r, before)
		if !runLockHeld(t, r, runID) || !runLockHeld(t, foreign, runID) {
			t.Fatal("foreign refusal changed real ownership")
		}
	})

	refusals := []struct {
		name  string
		runID string
		setup func() func()
	}{
		{"65 candidates refuse before inspection", runID, func() func() {
			extra := registerActor(t, r, nil)
			return func() { unregister(r, extra) }
		}},
		{"zero matches", "run_absent", func() func() { return func() {} }},
		{"nil actor key is a nonmatch", "run_absent", func() func() {
			unregister(r, fillers[0])
			r.mu.Lock()
			r.entries[nil] = struct{}{}
			r.mu.Unlock()
			return func() {
				r.mu.Lock()
				delete(r.entries, nil)
				r.entries[fillers[0]] = struct{}{}
				r.mu.Unlock()
			}
		}},
		{"two actor wrappers for one owner", runID, func() func() {
			unregister(r, fillers[0])
			dup := registerActor(t, r, owner)
			return func() { unregister(r, dup); r.mu.Lock(); r.entries[fillers[0]] = struct{}{}; r.mu.Unlock() }
		}},
		{"two actors for owner copies", runID, func() func() {
			unregister(r, fillers[0])
			copied := *owner
			dup := registerActor(t, r, &copied)
			return func() { unregister(r, dup); r.mu.Lock(); r.entries[fillers[0]] = struct{}{}; r.mu.Unlock() }
		}},
		{"held run lock without an actor (ErrHeld) is no authority", "run_locked", func() func() {
			dir := filepath.Join(r.journal.StateDir(), "runs", "run_locked")
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			seedRunState(t, r, "run_locked", "blocked") // terminal, unowned
			release, err := supervisor.AcquireOwner(dir)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := supervisor.AcquireOwner(dir); !errors.Is(err, supervisor.ErrOwnerHeld) {
				t.Fatalf("seeded ErrHeld = %v", err)
			}
			return release
		}},
	}
	for _, c := range refusals {
		restore := c.setup()
		effectsBefore := snapshotAttachmentEffects(t, r)
		scope, err := r.retainAttachmentScope(t.Context(), c.runID, true)
		if scope != nil {
			scope.release()
		}
		requireAttachmentEffectsUnchanged(t, r, effectsBefore)
		restore()
		if !errors.Is(err, supervisor.ErrOwnership) || scope != nil {
			t.Fatalf("%s: scope=%v err=%v; want an ownership_unresolved refusal", c.name, scope, err)
		}
		if !gateFree(t, r) {
			t.Fatalf("%s: refusal leaked an authority guard", c.name)
		}
	}

	type result struct {
		scope *attachmentScope
		err   error
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if scope, err := r.retainAttachmentScope(canceled, runID, false); !errors.Is(err, context.Canceled) || scope != nil {
		if scope != nil {
			scope.release()
		}
		t.Fatalf("canceled before snapshot: scope=%v err=%v", scope, err)
	}
	if guard, err := r.authority.acquire(canceled, false); !errors.Is(err, context.Canceled) {
		if guard != nil {
			guard()
		}
		t.Fatalf("canceled request on a free gate: err=%v; want a refusal", err)
	}

	// Canceled while blocked on the gate mutex after the identity proof: the
	// woken request refuses and unwinds its observation, hold and guard.
	blockedCtx, cancelBlocked := context.WithCancel(t.Context())
	r.authority.mu.Lock()
	unlockGate := sync.OnceFunc(r.authority.mu.Unlock)
	blocked := make(chan result, 1)
	var blockedRes result
	joinBlocked := sync.OnceFunc(func() {
		cancelBlocked()
		unlockGate()
		blockedRes = <-blocked
		if blockedRes.scope != nil {
			blockedRes.scope.release()
		}
	})
	go func() {
		s, err := r.retainAttachmentScope(blockedCtx, runID, false)
		blocked <- result{s, err}
	}()
	t.Cleanup(joinBlocked)
	waitMutexParked(t, "(*authorityGate).acquire")
	joinBlocked()
	if res := blockedRes; !errors.Is(res.err, context.Canceled) || res.scope != nil {
		t.Fatalf("canceled at the gate mutex: scope=%v err=%v; want a refusal", res.scope, res.err)
	}
	if !gateFree(t, r) {
		t.Fatal("canceled gate wait leaked the authority guard")
	}

	// Closing after the snapshot: the request parks on the authority gate
	// with the observation retained, then the runtime starts closing.
	writer, err := r.authority.acquire(t.Context(), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(writer)
	got := make(chan result, 1)
	go func() {
		s, err := r.retainAttachmentScope(t.Context(), runID, false)
		got <- result{s, err}
	}()
	waitParked(t, "(*authorityGate).acquire")
	r.mu.Lock()
	r.closing = true
	r.mu.Unlock()
	writer()
	res := <-got
	if res.scope != nil {
		res.scope.release()
	}
	r.mu.Lock()
	r.closing = false
	r.mu.Unlock()
	if !errors.Is(res.err, supervisor.ErrOwnership) || res.scope != nil {
		t.Fatalf("closing before the final recheck: scope=%v err=%v; want a refusal", res.scope, res.err)
	}
	if !gateFree(t, r) {
		t.Fatal("final closing refusal leaked the authority guard")
	}

	// Matcher-true but closed owner: RetainObservation is mandatory.
	if err := other.Close(); err != nil {
		t.Fatal(err)
	}
	if !other.MatchesRun("run_other") {
		t.Fatal("closed owner no longer matches its run; the case below would not reach retention")
	}
	if scope, err := r.retainAttachmentScope(t.Context(), "run_other", false); !errors.Is(err, supervisor.ErrOwnership) || scope != nil {
		if scope != nil {
			scope.release()
		}
		t.Fatalf("matcher-true closed owner: scope=%v err=%v; want a retention refusal", scope, err)
	}

	after, err := r.journal.Run(t.Context(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if after.State != before.State || !after.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatalf("run row changed %+v -> %+v", before, after)
	}
	if !attemptsAbsent(t, r, runID) || !attemptsAbsent(t, r, "run_locked") {
		t.Fatal("an attachment request launched an attempt")
	}
	if _, err := r.journal.Run(t.Context(), "run_absent"); !errors.Is(err, journal.ErrNotFound) {
		t.Fatalf("refused attachment created a run: %v", err)
	}
	r.mu.Lock()
	for a := range r.entries {
		if a.owner != nil && !a.owner.MatchesRun("run_other") && a.owner != owner {
			delete(r.entries, a) // the zero owner cannot be closed by shutdown
		}
	}
	r.mu.Unlock()
	unregister(r, valid)
	if !closeReleases(t, owner) {
		t.Fatal("an attachment request leaked an owner observation")
	}
}

// TestAttachmentScopeGuardOrdering: retained read and write guards serialise
// on the runtime-local gate, survive until explicit release, and keep the
// owner and the instance lease held through owner.Close and root shutdown.
func TestAttachmentScopeGuardOrdering(t *testing.T) {
	// Sequential: endpoint discovery and the instance lock are process-wide.
	r := testRuntime(t)
	const runID = "run_guard"
	seedVerifying(t, r, runID)
	owner := ownRun(t, r, runID)

	entered, release := make(chan struct{}), make(chan struct{})
	releaseEntry := sync.OnceFunc(func() { close(release) })
	var once sync.Once
	a, err := r.startEntry(t.Context(), owner, func(ctx context.Context, h supervisor.Hooks) error {
		h.Event = func(journal.Event) { once.Do(func() { close(entered) }); <-release }
		_, err := supervisor.RecoverOwned(ctx, r.journal, owner, runID, h)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { releaseEntry(); <-a.done })
	select {
	case <-entered:
	case <-a.done:
		t.Fatalf("owned entry returned before its callback barrier: %v", a.err)
	}

	first, err := r.retainAttachmentScope(t.Context(), runID, false)
	if err != nil {
		t.Fatalf("read scope during a busy entry: %v", err)
	}
	t.Cleanup(first.release)
	second, err := r.retainAttachmentScope(t.Context(), runID, false)
	if err != nil {
		t.Fatalf("concurrent read scope: %v", err)
	}
	t.Cleanup(second.release)
	start := time.Now()
	writeScope, err := r.retainAttachmentScope(t.Context(), runID, true)
	if !errors.Is(err, context.DeadlineExceeded) || writeScope != nil {
		if writeScope != nil {
			writeScope.release()
		}
		t.Fatalf("write behind held reads: scope=%v err=%v; want the request budget to refuse", writeScope, err)
	}
	if waited := time.Since(start); waited < attachmentRequestBudget {
		t.Fatalf("write refused after %v, before its %v budget", waited, attachmentRequestBudget)
	}
	first.release()
	second.release()
	writeScope, err = r.retainAttachmentScope(t.Context(), runID, true)
	if err != nil {
		t.Fatalf("write after reads released: %v", err)
	}
	t.Cleanup(writeScope.release)
	short, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	if s, err := r.retainAttachmentScope(short, runID, false); !errors.Is(err, context.DeadlineExceeded) || s != nil {
		cancel()
		if s != nil {
			s.release()
		}
		t.Fatalf("read behind a held write: scope=%v err=%v", s, err)
	}
	cancel()

	// Root shutdown with a blocked entry and a retained scope releases nothing.
	canceled, stop := context.WithCancel(t.Context())
	stop()
	if err := r.shutdown(canceled); !errors.Is(err, ErrCleanupIncomplete) {
		t.Fatalf("shutdown with a blocked entry = %v", err)
	}
	if s, err := r.retainAttachmentScope(t.Context(), runID, false); !errors.Is(err, supervisor.ErrOwnership) || s != nil {
		if s != nil {
			s.release()
		}
		t.Fatalf("scope while shutting down: scope=%v err=%v", s, err)
	}
	releaseEntry()
	<-a.done
	waitParked(t, "supervisor.(*ServiceOwner).Close(")
	budget, cancelBudget := context.WithTimeout(t.Context(), 200*time.Millisecond)
	err = r.shutdown(budget)
	cancelBudget()
	if !errors.Is(err, ErrCleanupIncomplete) {
		t.Fatalf("shutdown with a retained scope after the entry joined = %v; want incomplete", err)
	}
	if r.lease.Generation() == 0 || !runLockHeld(t, r, runID) {
		t.Fatal("shutdown released the lease or run lock while a scope was retained")
	}
	writeScope.release()
	writeScope.release()
	if err := r.shutdown(t.Context()); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if r.lease.Generation() != 0 || runLockHeld(t, r, runID) {
		t.Fatal("joined shutdown retained the lease or run lock")
	}
}
