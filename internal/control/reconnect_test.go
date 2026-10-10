package control

import (
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/workers"
)

func TestMaxSupervisorBeatAgeControl(t *testing.T) {
	t.Parallel()
	// Mirrors workers.MaxSupervisorBeatAge (workers must not import
	// control); both pin OQ-SR3's 300ms.
	if MaxSupervisorBeatAge != 300*time.Millisecond {
		t.Fatalf("MaxSupervisorBeatAge = %s, want 300ms", MaxSupervisorBeatAge)
	}
}

func TestWriteSupervisorBeat(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	at := time.Now().UTC().Truncate(time.Microsecond)
	if err := WriteSupervisorBeat(dir, 3, 41, at); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "supervisor.beat")) //nolint:gosec // G304: fixed beat name in a test temp dir
	if err != nil {
		t.Fatal(err)
	}
	var beat SupervisorBeat
	if err := json.Unmarshal(raw, &beat); err != nil {
		t.Fatalf("supervisor.beat is not a SupervisorBeat: %v", err)
	}
	if beat.Beat != 41 || beat.Generation != 3 || beat.At != at.Format(time.RFC3339Nano) {
		t.Fatalf("beat = %+v, want {41, %s, 3}", beat, at.Format(time.RFC3339Nano))
	}
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		st, err := os.Stat(filepath.Join(dir, "supervisor.beat"))
		if err != nil {
			t.Fatal(err)
		}
		if st.Mode().Perm() != 0o600 {
			t.Fatalf("supervisor.beat mode = %o, want 0600", st.Mode().Perm())
		}
	}
	// The worker reads what the supervisor writes: the cross-package
	// format check (workers cannot import control, so control asserts
	// the read side here).
	gotAt, gen, ok := workers.ReadSupervisorBeat(dir)
	if !ok || gen != 3 || !gotAt.Equal(at) {
		t.Fatalf("workers.ReadSupervisorBeat = (%s, %d, %v), want (%s, 3, true)", gotAt, gen, ok, at)
	}
	// The counter is owned by the caller: a second write replaces the first.
	if err := WriteSupervisorBeat(dir, 3, 42, at); err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile(filepath.Join(dir, "supervisor.beat")) //nolint:gosec // G304: fixed beat name in a test temp dir
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &beat); err != nil || beat.Beat != 42 {
		t.Fatalf("beat after rewrite = %+v, want counter 42", beat)
	}
	if err := WriteSupervisorBeat(filepath.Join(dir, "no-such-dir"), 1, 1, at); err == nil {
		t.Fatal("WriteSupervisorBeat into a missing directory returned nil")
	}
}

// spoolLaunched appends one attempt.launched spool line with a usable
// worker identity payload: unacknowledged spool for the handshake.
func spoolLaunched(t *testing.T, dir, runID, attemptID, eventID string, pid int, start time.Time) {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"worker_pid": pid, "worker_start_time": start})
	if err != nil {
		t.Fatal(err)
	}
	appendSpoolEvent(t, dir, journal.Event{
		SchemaVersion: journal.EnvelopeVersion, EventID: eventID,
		RunID: runID, AttemptID: attemptID, ProducerID: "wrk_" + attemptID,
		ProducerSequence: 99, Generation: 1, ObservedAt: time.Now().UTC(),
		Type: "attempt.launched", Payload: payload,
	})
}

// snapshotRows dumps whole tables for before/after comparison.
func snapshotRows(t *testing.T, db *sql.DB, tables ...string) map[string]string {
	t.Helper()
	out := map[string]string{}
	queries := map[string]string{
		"reservations":    `SELECT * FROM reservations ORDER BY 1`,
		"run_assignments": `SELECT * FROM run_assignments ORDER BY 1`,
		"candidates":      `SELECT * FROM candidates ORDER BY 1`,
	}
	for _, table := range tables {
		query, ok := queries[table]
		if !ok {
			t.Fatalf("snapshotRows: unexpected table %q", table)
		}
		rows, err := db.Query(query)
		if err != nil {
			t.Fatal(err)
		}
		var lines []string
		cols, err := rows.Columns()
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(vals)
			if err != nil {
				t.Fatal(err)
			}
			lines = append(lines, string(raw))
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
		out[table] = strings.Join(lines, "\n")
	}
	return out
}

func seedBudgetAndOwnership(t *testing.T, db *sql.DB, runID string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO reservations (reservation_id, run_id, bucket, scope, owner, quantity, status, expires_at, created_at)
		VALUES ('res_1', ?, 'b', 's', 'o', '7', 'held', 't', 't')`, runID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO operations (operation_id, method, digest, state, claimed_at)
		VALUES ('op_seed_assign', 'assign', 'd', 'claimed', 't')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO run_assignments (run_id, owner, assigned_at, operation_id)
		VALUES (?, 'supervisor', 't', 'op_seed_assign')`, runID); err != nil {
		t.Fatal(err)
	}
}

func TestReconnectReconcilesFirst(t *testing.T) {
	t.Parallel()
	f := newRecoverState(t, "run_hs_first", "att_hs_first")
	// Unacknowledged spool: a result settling the worker's output, plus
	// the same launched line twice — duplicates the handshake must ack
	// once, never double-accept.
	appendSpoolEvent(t, f.attemptDir(), spoolEvent(f.runID, f.attempt, "attempt.native_result", "evt_sp_hs_result"))
	spoolLaunched(t, f.attemptDir(), f.runID, f.attempt, "evt_sp_hs_launched", f.pid, f.start)
	spoolLaunched(t, f.attemptDir(), f.runID, f.attempt, "evt_sp_hs_launched", f.pid, f.start)
	seedBudgetAndOwnership(t, f.db, f.runID)
	before := snapshotRows(t, f.db, "reservations", "run_assignments", "candidates")

	token, rep, err := Reconnect(t.Context(), ReconnectDeps{DB: f.db, StateDir: f.dir}, f.runID)
	if err != nil {
		t.Fatalf("Reconnect: %v", err)
	}
	if rep.Outcome != RecoverReconnected {
		t.Fatalf("outcome = %q, want reconnected", rep.Outcome)
	}
	// The new-generation token is minted after the reconcile, and it checks.
	if len(token) != 64 {
		t.Fatalf("token %q is not 32 hex bytes", token)
	}
	if _, err := hex.DecodeString(token); err != nil {
		t.Fatalf("token %q is not hex: %v", token, err)
	}
	if err := CheckToken(t.Context(), f.db, token, f.attempt); err != nil {
		t.Fatalf("the minted token does not check: %v", err)
	}
	// The same-launch lineage is untouched: the attempts digest still
	// names the original launch (the T3 hand-off decision: capability_tokens
	// carries the live token, the digest does not rotate).
	rows := attemptRows(t, f.db, f.runID)
	if len(rows) != 1 || rows[0].Launch != shahex(f.launchToken) {
		t.Fatalf("attempts = %+v, want the one attempt on its original launch digest", rows)
	}
	// Duplicates acked: the record covers every spooled byte, including
	// the duplicate line. No double-accept: exactly one outcome recorded.
	st, err := os.Stat(filepath.Join(f.attemptDir(), "spool.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if rep.Markers.SpoolBytes != st.Size() {
		t.Fatalf("markers spool_bytes = %d, want the %d spooled bytes incl. duplicates", rep.Markers.SpoolBytes, st.Size())
	}
	evs := recoveryEvents(t, f.j, f.runID)
	if len(evs) != 1 {
		t.Fatalf("journal holds %d recovery outcomes, want one: no double-accept", len(evs))
	}
	// Golden reconnect event: the stable projection of the journaled decision.
	decided := decodeReport(t, evs[0].Payload)
	golden, err := json.Marshal(map[string]any{"type": evs[0].Type, "run_id": decided.RunID,
		"attempt_id": decided.AttemptID, "outcome": decided.Outcome, "reason": decided.Reason})
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"attempt_id":"att_hs_first","outcome":"reconnected","reason":"","run_id":"run_hs_first","type":"attempt.recovery_decided"}`; string(golden) != want {
		t.Fatalf("golden event %s, want %s", golden, want)
	}
	// Budgets and input ownership are unchanged after the adoption.
	after := snapshotRows(t, f.db, "reservations", "run_assignments", "candidates")
	for table, want := range before {
		if after[table] != want {
			t.Fatalf("%s changed over the handshake:\nbefore %q\nafter %q", table, want, after[table])
		}
	}
	// A repeated handshake replays the decision (no second outcome) and
	// rotates the token: minting again replaces the live token, so the
	// first one no longer checks.
	token2, rep2, err := Reconnect(t.Context(), ReconnectDeps{DB: f.db, StateDir: f.dir}, f.runID)
	if err != nil {
		t.Fatalf("second Reconnect: %v", err)
	}
	if rep2.Outcome != RecoverReconnected {
		t.Fatalf("second outcome = %q, want reconnected", rep2.Outcome)
	}
	if len(recoveryEvents(t, f.j, f.runID)) != 1 {
		t.Fatal("the repeated handshake recorded a second outcome")
	}
	if token2 == token {
		t.Fatal("the repeated handshake returned the same token, want a rotation")
	}
	if err := CheckToken(t.Context(), f.db, token, f.attempt); !errors.Is(err, &Error{Code: CodePermissionDenied}) {
		t.Fatalf("the rotated token still checks: %v", err)
	}
	if err := CheckToken(t.Context(), f.db, token2, f.attempt); err != nil {
		t.Fatalf("the rotated token does not check: %v", err)
	}
}

func TestOldGenerationNeverFresh(t *testing.T) {
	t.Parallel()
	f := newRecoverState(t, "run_hs_stale", "att_hs_stale")
	// Live same worker, but its producer moved to generation 2 while its
	// journaled launch sits at 1: stale evidence presented as fresh.
	if _, err := f.db.Exec(`UPDATE producers SET generation = 2 WHERE producer_id = ?`,
		"wrk_"+f.attempt); err != nil {
		t.Fatal(err)
	}
	token, rep, err := Reconnect(t.Context(), ReconnectDeps{DB: f.db, StateDir: f.dir}, f.runID)
	requireCode(t, err, CodeOwnershipUnresolved)
	if token != "" {
		t.Fatalf("stale handshake minted %q, want no token", token)
	}
	if n := count(t, f.db, "capability_tokens"); n != 0 {
		t.Fatalf("capability_tokens holds %d rows, want none", n)
	}
	if rep.Outcome != RecoverQuarantined || rep.Reason != "stale_generation" {
		t.Fatalf("report = %+v, want quarantined/stale_generation", rep)
	}
	// Retained for reconciliation: the stale launch is still journaled,
	// and the record names the stale generation.
	events, err := f.j.Events(t.Context(), f.runID, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ev := range events {
		if ev.Type == "attempt.launched" && ev.AttemptID == f.attempt && ev.Generation == 1 {
			found = true
		}
	}
	if !found {
		t.Fatal("the stale generation-1 launched event is gone: evidence must be retained")
	}
	if rep.Markers.WorkerProducerGen != 2 {
		t.Fatalf("markers worker_producer_gen = %d, want the retained current 2", rep.Markers.WorkerProducerGen)
	}
	// Rejected as completion: no candidate frozen, no terminal completion
	// state, no spool acked.
	if n := count(t, f.db, "candidates"); n != 0 {
		t.Fatalf("candidates holds %d rows, want none", n)
	}
	if got := attemptState(t, f.db, f.attempt); got != "quarantined" {
		t.Fatalf("attempt state = %q, want quarantined, never a completed state", got)
	}
	var offset int64
	if err := f.db.QueryRow(`SELECT spool_offset FROM attempts WHERE attempt_id = ?`,
		f.attempt).Scan(&offset); err != nil {
		t.Fatal(err)
	}
	if offset != 0 {
		t.Fatalf("spool_offset = %d, want 0: stale spool is retained, never acked", offset)
	}
	// Fresh evidence re-examines against the retained staleness instead of
	// healing past it.
	appendSpoolEvent(t, f.attemptDir(), spoolEvent(f.runID, f.attempt, "attempt.progress", "evt_sp_hs_progress"))
	_, rep2, err := Reconnect(t.Context(), ReconnectDeps{DB: f.db, StateDir: f.dir}, f.runID)
	requireCode(t, err, CodeOwnershipUnresolved)
	if rep2.Outcome != RecoverQuarantined || rep2.Reason != "stale_generation" {
		t.Fatalf("second report = %+v, want the retained stale verdict again", rep2)
	}
	if evs := recoveryEvents(t, f.j, f.runID); len(evs) != 2 {
		t.Fatalf("journal holds %d recovery outcomes, want one per pass", len(evs))
	}
}

func TestReconnectMintsOnlyOnReconnected(t *testing.T) {
	t.Parallel()
	t.Run("continued mints nothing", func(t *testing.T) {
		t.Parallel()
		f := newRecoverState(t, "run_hs_cont", "att_hs_cont")
		killWorker(t, f, nil, 2)
		token, rep, err := Reconnect(t.Context(), ReconnectDeps{DB: f.db, StateDir: f.dir}, f.runID)
		if err != nil {
			t.Fatalf("Reconnect: %v", err)
		}
		if rep.Outcome != RecoverContinued {
			t.Fatalf("outcome = %q, want continued", rep.Outcome)
		}
		if token != "" {
			t.Fatalf("continued handshake minted %q, want no token", token)
		}
		if n := count(t, f.db, "capability_tokens"); n != 0 {
			t.Fatalf("capability_tokens holds %d rows, want none", n)
		}
	})
	t.Run("unknown run is invalid", func(t *testing.T) {
		t.Parallel()
		f := newRecoverState(t, "run_hs_unknown", "att_hs_unknown")
		token, rep, err := Reconnect(t.Context(), ReconnectDeps{DB: f.db, StateDir: f.dir}, f.runID+"nope")
		requireCode(t, err, CodeInvalidContract)
		if token != "" || rep.Outcome != "" {
			t.Fatalf("unknown-run handshake = (%q, %+v), want no token and no outcome", token, rep)
		}
		if evs := recoveryEvents(t, f.j, f.runID); len(evs) != 0 {
			t.Fatalf("journal holds %d recovery outcomes, want none", len(evs))
		}
	})
}

// TestHermeticNoCredentialsOrNetwork pins NFR-6 for the new code: with a
// planted credential in the environment the handshake passes and no
// golden's decoded string values contain the planted value or any
// secret-shaped hit (values walked, not keys); the new non-test code
// dials no network service.
func TestHermeticNoCredentialsOrNetwork(t *testing.T) {
	// Serial: it plants credentials in the process environment.
	const planted = "sk-planted-test-key-99"
	t.Setenv("ANTHROPIC_API_KEY", planted)
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "secret-planted-token-99")

	f := newRecoverState(t, "run_hs_herm", "att_hs_herm")
	token, rep, err := Reconnect(t.Context(), ReconnectDeps{DB: f.db, StateDir: f.dir}, f.runID)
	if err != nil {
		t.Fatalf("Reconnect with planted credentials: %v", err)
	}
	if rep.Outcome != RecoverReconnected || token == "" {
		t.Fatalf("handshake = (%q, %+v), want a minted reconnected token", token, rep)
	}
	// Every golden the new tests decode: the stable handshake report plus
	// the journaled decision's stable projection.
	reportJSON, err := json.Marshal(rep)
	if err != nil {
		t.Fatal(err)
	}
	evs := recoveryEvents(t, f.j, f.runID)
	if len(evs) != 1 {
		t.Fatalf("journal holds %d recovery outcomes, want one", len(evs))
	}
	var values []string
	var walk func(v any)
	walk = func(v any) {
		switch v := v.(type) {
		case map[string]any:
			for _, item := range v {
				walk(item)
			}
		case []any:
			for _, item := range v {
				walk(item)
			}
		case string:
			values = append(values, v)
		}
	}
	for _, raw := range []json.RawMessage{reportJSON, evs[0].Payload} {
		var decoded any
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatal(err)
		}
		walk(decoded)
	}
	if len(values) == 0 {
		t.Fatal("the golden walk visited no values: it proves nothing")
	}
	for _, v := range values {
		for _, hit := range []string{planted, "sk-", "secret", "token", "apiKey"} {
			if strings.Contains(v, hit) {
				t.Fatalf("golden value %q contains %q", v, hit)
			}
		}
	}
	// The walker is load-bearing: a decoy golden carrying a planted
	// secret trips it.
	var decoy any
	if err := json.Unmarshal([]byte(`{"nested":[{"k":"sk-decoy-value"}]}`), &decoy); err != nil {
		t.Fatal(err)
	}
	tripped, visited := false, 0
	var walkDecoy func(v any)
	walkDecoy = func(v any) {
		switch v := v.(type) {
		case map[string]any:
			for _, item := range v {
				walkDecoy(item)
			}
		case []any:
			for _, item := range v {
				walkDecoy(item)
			}
		case string:
			visited++
			if strings.Contains(v, "sk-decoy") {
				tripped = true
			}
		}
	}
	walkDecoy(decoy)
	if visited == 0 || !tripped {
		t.Fatalf("the decoy walk visited %d values and tripped=%v, want the planted sk-decoy found", visited, tripped)
	}

	// Structural: no net.Dial/DialContext call in the new non-test code.
	// (control necessarily imports net for its Unix-socket listener, so
	// the go list -deps absence pattern cannot apply here.)
	for _, name := range []string{"reconnect.go", "execute_long.go", filepath.Join("..", "workers", "envelope.go")} {
		raw, err := os.ReadFile(filepath.Clean(name))
		if err != nil {
			t.Fatal(err)
		}
		if hits := dialCalls(t, name, raw); len(hits) != 0 {
			t.Fatalf("%s dials the network: %s", name, strings.Join(hits, ", "))
		}
	}
	// The check is load-bearing: a fixture with a planted net.Dial fails it.
	fixture := "package p\nimport \"net\"\nfunc f() { _, _ = net.Dial(\"tcp\", \"x\"); _, _ = net.Dial(\"tcp\", \"y\") }\n"
	if hits := dialCalls(t, "fixture.go", []byte(fixture)); len(hits) != 2 {
		t.Fatalf("the planted fixture reported %d dial calls, want 2", len(hits))
	}
}

// dialCalls lists the net.Dial/DialContext call sites in one Go source file.
func dialCalls(t *testing.T, name string, src []byte) []string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, name, src, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", name, err)
	}
	var hits []string
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		id, ok := sel.X.(*ast.Ident)
		if !ok || id.Name != "net" {
			return true
		}
		if sel.Sel.Name == "Dial" || sel.Sel.Name == "DialContext" {
			hits = append(hits, fmt.Sprintf("net.%s at %s", sel.Sel.Name, fset.Position(call.Pos())))
		}
		return true
	})
	return hits
}
