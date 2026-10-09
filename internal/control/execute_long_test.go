package control

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestLongStopFreesOtherIntents pins the Task 1 deferral: a stop waiting
// for its report holds no ledger transaction, so other mutating intents
// proceed instead of wedging past busy_timeout.
func TestLongStopFreesOtherIntents(t *testing.T) {
	t.Parallel()
	f := newStopState(t, "run_long_free", "att_long_free")
	deps := f.deps()
	deps.WaitDeadline = 10 * time.Second
	entered := make(chan string, 4)
	deps.entered = entered
	done := make(chan error, 1)
	var bodyLen atomic.Int64
	go func() {
		res, err := ExecuteLong(f.ctx(t), StopHandler(deps), Peer{}, stopIntent(t, "op_long_free", f.attempt, f.version))
		bodyLen.Store(int64(len(res.Body)))
		done <- err
	}()
	select {
	case got := <-entered:
		if got != "op_long_free" {
			t.Errorf("entered = %q, want the operation_id", got)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the long stop never reached its wait")
	}
	// While the stop waits, an unrelated mutating intent succeeds at once.
	start := time.Now()
	assign := Intent{OperationID: "op_long_assign", Method: "assign", Object: f.runID}
	if _, err := Execute(f.ctx(t), AssignHandler(f.db), Peer{}, assign); err != nil {
		t.Fatalf("assign during a waiting stop: %v", err)
	}
	if elapsed := time.Since(start); elapsed >= 5*time.Second {
		t.Fatalf("assign took %s during a waiting stop, want it unblocked", elapsed)
	}
	writeStopped(t, f, `{"confirmed":true,"sent":[{"signal":"kill","grace":1000000000}],"ladder_version":"stop-ladder/v1"}`, 2)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("long stop: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the long stop never finished after its report")
	}
	if bodyLen.Load() == 0 {
		t.Fatal("the long stop returned an empty receipt")
	}
	if n := count(t, f.db, "operations"); n != 2 {
		t.Fatalf("operations holds %d rows, want the stop plus the assign", n)
	}
}

// TestLongIdenticalRepeatReplaysDuringWait pins the served join: a repeat
// while the first execution still waits blocks, then replays the stored
// Result byte-identically, with one execution and one standing request.
func TestLongIdenticalRepeatReplaysDuringWait(t *testing.T) {
	t.Parallel()
	f := newStopState(t, "run_long_join", "att_long_join")
	deps := f.deps()
	deps.WaitDeadline = 10 * time.Second
	entered := make(chan string, 4)
	deps.entered = entered
	h := StopHandler(deps)
	in := stopIntent(t, "op_long_join", f.attempt, f.version)
	var wg sync.WaitGroup
	var res1, res2 Result
	var err1, err2 error
	started1 := make(chan struct{})
	wg.Go(func() {
		close(started1)
		res1, err1 = ExecuteLong(f.ctx(t), h, Peer{}, in)
	})
	<-started1
	select {
	case <-entered:
	case <-time.After(30 * time.Second):
		t.Fatal("the first long stop never reached its wait")
	}
	started2 := make(chan struct{})
	wg.Go(func() {
		close(started2)
		res2, err2 = ExecuteLong(f.ctx(t), h, Peer{}, in)
	})
	<-started2
	// The repeat is polling, not executing: only the winner entered.
	time.Sleep(200 * time.Millisecond)
	select {
	case extra := <-entered:
		t.Fatalf("a second execution entered the wait (%q), want the repeat to poll", extra)
	default:
	}
	writeStopped(t, f, `{"confirmed":true,"sent":[{"signal":"kill","grace":1000000000}],"ladder_version":"stop-ladder/v1"}`, 2)
	wg.Wait()
	if err1 != nil || err2 != nil {
		t.Fatalf("err1 = %v, err2 = %v, want the shared receipt", err1, err2)
	}
	if len(res1.Body) == 0 || string(res1.Body) != string(res2.Body) {
		t.Fatalf("bodies %s and %s are not byte-identical", res1.Body, res2.Body)
	}
	if n := count(t, f.db, "operations"); n != 1 {
		t.Fatalf("operations holds %d rows, want the one execution", n)
	}
	if got, ok := stopRequestID(t, f); !ok || got != "op_long_join" {
		t.Fatalf("stop.request request_id = %q, want the one standing request", got)
	}
}

func TestLongTransientReleasesClaim(t *testing.T) {
	t.Parallel()
	db, _ := openLedger(t)
	ctx := WithLedger(t.Context(), db)
	var runs atomic.Int32
	h := func() Handler {
		return func(_ context.Context, _ Peer, _ Intent) (Result, error) {
			if runs.Add(1) == 1 {
				return Result{}, errors.New("disk hiccup")
			}
			return Result{Body: json.RawMessage(`{"ok":true}`)}, nil
		}
	}()
	in := intent("op_long_flaky", "probe")
	if _, err := ExecuteLong(ctx, h, Peer{}, in); err == nil {
		t.Fatal("transient failure returned no error")
	} else if _, ok := errors.AsType[*Error](err); ok {
		t.Fatalf("err = %v, want a transient failure, not a stored one", err)
	}
	res, err := ExecuteLong(ctx, h, Peer{}, in)
	if err != nil {
		t.Fatalf("retry after a transient failure: %v", err)
	}
	if string(res.Body) != `{"ok":true}` {
		t.Fatalf("retry body = %s, want the success", res.Body)
	}
	if runs.Load() != 2 {
		t.Fatalf("handler ran %d times, want the failure plus the retry", runs.Load())
	}
	var claimed int
	if err := db.QueryRow(`SELECT count(*) FROM operations WHERE state = 'claimed'`).Scan(&claimed); err != nil {
		t.Fatal(err)
	}
	if claimed != 0 || count(t, db, "operations") != 1 {
		t.Fatalf("operations holds a stuck claim: %d rows, %d claimed", count(t, db, "operations"), claimed)
	}
}

func TestLongStoredFailureReplays(t *testing.T) {
	t.Parallel()
	db, _ := openLedger(t)
	ctx := WithLedger(t.Context(), db)
	var runs atomic.Int32
	h := func(_ context.Context, _ Peer, _ Intent) (Result, error) {
		runs.Add(1)
		return Result{}, newError(CodePermissionDenied, "no")
	}
	in := intent("op_long_fail", "probe")
	for range 2 {
		res, err := ExecuteLong(ctx, h, Peer{}, in)
		requireCode(t, err, CodePermissionDenied)
		if res.Error == nil || res.Error.Code != CodePermissionDenied {
			t.Fatalf("result = %+v, want the stored error", res)
		}
	}
	if runs.Load() != 1 {
		t.Fatalf("handler ran %d times for a stored failure, want 1", runs.Load())
	}
}

func TestLongPanicReleasesClaim(t *testing.T) {
	t.Parallel()
	db, _ := openLedger(t)
	ctx := WithLedger(t.Context(), db)
	func() {
		defer func() { _ = recover() }()
		_, _ = ExecuteLong(ctx, func(_ context.Context, _ Peer, _ Intent) (Result, error) { panic("boom") },
			Peer{}, intent("op_long_panic", "probe"))
	}()
	// The retry runs at once instead of polling a stuck claim.
	var runs atomic.Int32
	res, err := ExecuteLong(ctx, counting(`{"ok":true}`, &runs), Peer{}, intent("op_long_panic", "probe"))
	if err != nil {
		t.Fatalf("retry after a panic: %v", err)
	}
	if string(res.Body) != `{"ok":true}` || runs.Load() != 1 {
		t.Fatalf("retry = (%s, runs %d), want the success after one run", res.Body, runs.Load())
	}
}

func TestLongStaleClaimAdopted(t *testing.T) {
	t.Parallel()
	db, _ := openLedger(t)
	ctx := WithLedger(t.Context(), db)
	in := intent("op_long_stale", "probe")
	in.Params = json.RawMessage(`{"a":1}`)
	old := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO operations (operation_id, method, object, digest, state, claimed_at)
		VALUES (?, ?, '', ?, 'claimed', ?)`, in.OperationID, in.Method, intentDigest(in), old); err != nil {
		t.Fatal(err)
	}
	var runs atomic.Int32
	res, err := ExecuteLong(ctx, counting(`{"adopted":true}`, &runs), Peer{}, in)
	if err != nil {
		t.Fatalf("adopting a stale claim: %v", err)
	}
	if string(res.Body) != `{"adopted":true}` || runs.Load() != 1 {
		t.Fatalf("adopted run = (%s, runs %d), want the success after one run", res.Body, runs.Load())
	}
	var state string
	if err := db.QueryRow(`SELECT state FROM operations WHERE operation_id = ?`, in.OperationID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "done" {
		t.Fatalf("adopted operation state = %q, want done", state)
	}
}

func TestLongConflictingReuseConflicts(t *testing.T) {
	t.Parallel()
	db, _ := openLedger(t)
	ctx := WithLedger(t.Context(), db)
	var runs atomic.Int32
	h := counting(`{}`, &runs)
	first := intent("op_long_reuse", "probe")
	first.Params = json.RawMessage(`{"a":1}`)
	if _, err := ExecuteLong(ctx, h, Peer{}, first); err != nil {
		t.Fatal(err)
	}
	other := first
	other.Params = json.RawMessage(`{"a":2}`)
	_, err := ExecuteLong(ctx, h, Peer{}, other)
	requireCode(t, err, CodeRevisionConflict)
	if runs.Load() != 1 {
		t.Fatalf("handler ran %d times, want only the first", runs.Load())
	}
	// A conflicting reuse against a live claim fails at once, without
	// running and without waiting for the claim.
	claim := intent("op_long_claimed", "probe")
	claim.Params = json.RawMessage(`{"a":1}`)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO operations (operation_id, method, object, digest, state, claimed_at)
		VALUES (?, ?, '', ?, 'claimed', ?)`, claim.OperationID, claim.Method, intentDigest(claim), now); err != nil {
		t.Fatal(err)
	}
	clash := claim
	clash.Params = json.RawMessage(`{"a":2}`)
	start := time.Now()
	_, err = ExecuteLong(ctx, h, Peer{}, clash)
	requireCode(t, err, CodeRevisionConflict)
	if elapsed := time.Since(start); elapsed >= 5*time.Second {
		t.Fatalf("conflicting reuse waited %s on a live claim, want an immediate refusal", elapsed)
	}
	if runs.Load() != 1 {
		t.Fatalf("handler ran %d times, want only the first", runs.Load())
	}
}

func TestLongConcurrentDuplicateExecutesOnce(t *testing.T) {
	t.Parallel()
	db, _ := openLedger(t)
	ctx := WithLedger(t.Context(), db)
	var runs atomic.Int32
	entered, proceed := make(chan struct{}), make(chan struct{})
	h := func(_ context.Context, _ Peer, _ Intent) (Result, error) {
		if runs.Add(1) == 1 {
			close(entered)
		}
		<-proceed
		return Result{Body: json.RawMessage(`{"winner":true}`)}, nil
	}
	const callers = 8
	out := make([]string, callers)
	errs := make([]error, callers)
	var wg, started sync.WaitGroup
	for i := range callers {
		wg.Add(1)
		started.Add(1)
		go func() {
			defer wg.Done()
			started.Done()
			res, err := ExecuteLong(ctx, h, Peer{}, intent("op_long_race", "probe"))
			out[i], errs[i] = wire(t, res), err
		}()
	}
	<-entered
	started.Wait()
	close(proceed)
	wg.Wait()
	if runs.Load() != 1 {
		t.Fatalf("handler ran %d times for %d concurrent duplicates, want 1", runs.Load(), callers)
	}
	for i := range callers {
		if errs[i] != nil || out[i] != out[0] {
			t.Fatalf("caller %d got %q (%v), want the shared stored result %q", i, out[i], errs[i], out[0])
		}
	}
}

// TestServedStopAndRecoverAreLong pins the fold: the supervisor serves the
// stop and recover methods through the long path with their dependencies
// threaded, so a served stop completes on its report and a served recover
// reconnects a live worker without launching.
func TestServedStopAndRecoverAreLong(t *testing.T) {
	t.Parallel()
	f := newRecoverState(t, "run_srv_fold", "att_srv_fold")
	writeStopped(t, f.stopState, `{"confirmed":true,"sent":[{"signal":"kill","grace":1000000000}],"ladder_version":"stop-ladder/v1"}`, 2)
	srv := NewSupervisorServer(f.db, f.j, f.dir, f.launcher.launch)
	res, err := srv.Dispatch(f.ctx(t), Peer{}, stopIntent(t, "op_srv_stop", f.attempt, f.version))
	if err != nil {
		t.Fatalf("served stop: %v", err)
	}
	var receipt StopReceipt
	if err := json.Unmarshal(res.Body, &receipt); err != nil {
		t.Fatal(err)
	}
	if !receipt.Confirmed {
		t.Fatalf("served stop receipt = %+v, want confirmed", receipt)
	}
	g := newRecoverState(t, "run_srv_fold2", "att_srv_fold2")
	srv2 := NewSupervisorServer(g.db, g.j, g.dir, g.launcher.launch)
	res, err = srv2.Dispatch(g.ctx(t), Peer{}, recoverIntent(t, "op_srv_rec", g.runID))
	if err != nil {
		t.Fatalf("served recover: %v", err)
	}
	if rep := decodeReport(t, res.Body); rep.Outcome != RecoverReconnected {
		t.Fatalf("served recover outcome = %q, want reconnected", rep.Outcome)
	}
	if n := g.launcher.count(); n != 0 {
		t.Fatalf("launcher ran %d times on a reconnect, want zero", n)
	}
	_, err = srv.Dispatch(f.ctx(t), Peer{}, Intent{OperationID: "op_srv_nope", Method: "nope"})
	requireCode(t, err, CodeCapabilityUnsupported)
}

// TestServedStopWaitHoldsNoLedgerLock pins the registration, not just the
// executor: a stop dispatched through the supervisor server waits for its
// report without holding the ledger's write lock, so a served assign
// completes while the stop is still standing.
func TestServedStopWaitHoldsNoLedgerLock(t *testing.T) {
	t.Parallel()
	f := newStopState(t, "run_srv_free", "att_srv_free")
	srv := NewSupervisorServer(f.db, f.j, f.dir, nil)
	done := make(chan error, 1)
	go func() {
		_, err := srv.Dispatch(f.ctx(t), Peer{}, stopIntent(t, "op_srv_free", f.attempt, f.version))
		done <- err
	}()
	deadline := time.Now().Add(30 * time.Second)
	for {
		if _, ok := stopRequestID(t, f); ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the served stop never delivered its request")
		}
		time.Sleep(5 * time.Millisecond)
	}
	start := time.Now()
	assign := Intent{OperationID: "op_srv_free_assign", Method: "assign", Object: f.runID}
	if _, err := srv.Dispatch(f.ctx(t), Peer{}, assign); err != nil {
		t.Fatalf("served assign during a waiting served stop: %v", err)
	}
	if elapsed := time.Since(start); elapsed >= 4*time.Second {
		t.Fatalf("served assign took %s beside a waiting stop, want it unblocked", elapsed)
	}
	writeStopped(t, f, `{"confirmed":true,"sent":[{"signal":"kill","grace":1000000000}],"ladder_version":"stop-ladder/v1"}`, 2)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("served stop: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the served stop never finished after its report")
	}
}

func TestLongRejectsMalformedIntent(t *testing.T) {
	t.Parallel()
	db, _ := openLedger(t)
	ctx := WithLedger(t.Context(), db)
	var runs atomic.Int32
	noMethod := intent("op_long_bad1", "")
	badParams := intent("op_long_bad2", "probe")
	badParams.Params = json.RawMessage(`{"a":`)
	for name, in := range map[string]Intent{"no method": noMethod, "params not JSON": badParams} {
		_, err := ExecuteLong(ctx, counting(`{}`, &runs), Peer{}, in)
		if !errors.Is(err, &Error{Code: CodeInvalidContract}) {
			t.Errorf("%s: err = %v, want invalid_contract", name, err)
		}
	}
	if runs.Load() != 0 || count(t, db, "operations") != 0 {
		t.Fatalf("handler ran %d times, operations rows %d; a malformed intent must do neither", runs.Load(), count(t, db, "operations"))
	}
}

func TestLongStaleExpectedRevisionConflicts(t *testing.T) {
	t.Parallel()
	db, _ := openLedger(t)
	ctx := WithLedger(t.Context(), db)
	var runs atomic.Int32
	h := func(_ context.Context, _ Peer, in Intent) (Result, error) {
		runs.Add(1)
		return Result{Revision: in.ExpectedRevision + 1}, nil
	}
	step := func(op string, expected int64) error {
		in := intent(op, "probe")
		in.Object, in.ExpectedRevision = "run_1", expected
		_, err := ExecuteLong(ctx, h, Peer{}, in)
		return err
	}
	if err := step("op_long_rev1", 0); err != nil {
		t.Fatal(err)
	}
	requireCode(t, step("op_long_rev2", 0), CodeRevisionConflict)
	if err := step("op_long_rev4", 1); err != nil {
		t.Fatalf("current revision refused: %v", err)
	}
	if runs.Load() != 2 {
		t.Fatalf("handler ran %d times, want the two accepted intents only", runs.Load())
	}
}
