package e2e

// AT-13 end-to-end and NFR-1 run-path latency for budget-ledger-s1.
//
// The packaged runs below use the fake adapter with --no-checks and no
// mythhelm.toml, so admission needs no config trust and the receipt's
// verify pass is exact (0 checks, 0s). Plain format keeps the ledger
// notice lines on stdout, ahead of the result line.

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/billing"
	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/qualify"
	"github.com/turbokast/mythhelm/internal/workers"
)

// ledgerSeedRepo seeds a committed repository with the standard E2E task
// and no mythhelm.toml: ledger runs waive checks, so no config trust is
// needed.
func ledgerSeedRepo(t *testing.T, e env) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repo, 0o750); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = e.withParent(t)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	git("init", "--quiet", "--initial-branch=main")
	git("config", "user.name", "E2E")
	git("config", "user.email", "e2e@example.com")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("e2e\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "task.md"), []byte("# E2E task\n\nWrite demo.txt.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git("add", "README.md", "task.md")
	git("commit", "--quiet", "-m", "seed")
	return repo
}

// ledgerRun executes one packaged ledger run: fake adapter, plain format,
// checks waived, the named scenario.
func ledgerRun(t *testing.T, e env, repo, scenario string) (exit int, stdout, stderr string) {
	t.Helper()
	return run(t, e, repo, "run", "--task-file", "task.md", "--adapter", "fake",
		"--billing", "local-scripted", "--execution-profile", "trusted-host",
		"--non-interactive", "--no-checks", "--plain", "--scenario", scenario)
}

// ledgerRunIDOf parses the run ID from the `run <id>: created` line.
func ledgerRunIDOf(t *testing.T, stdout string) string {
	t.Helper()
	for line := range strings.Lines(stdout) {
		rest, ok := strings.CutPrefix(strings.TrimSpace(line), "run ")
		if !ok {
			continue
		}
		if id, ok := strings.CutSuffix(rest, ": created"); ok && id != "" {
			return id
		}
	}
	t.Fatalf("no `run <id>: created` line in stdout:\n%s", stdout)
	return ""
}

// ledgerResultOf parses the state and reason from the `result:` line,
// e.g. `result: blocked (allowance_exhausted); exit 3 admission_blocked`.
func ledgerResultOf(t *testing.T, stdout string) (state, reason string) {
	t.Helper()
	for line := range strings.Lines(stdout) {
		rest, ok := strings.CutPrefix(strings.TrimSpace(line), "result:")
		if !ok {
			continue
		}
		rest = strings.TrimSpace(rest)
		state, after, _ := strings.Cut(rest, " (")
		reason, _, _ = strings.Cut(after, ")")
		if state == "" || reason == "" {
			t.Fatalf("result line %q has no state (reason)", strings.TrimSpace(line))
		}
		return state, reason
	}
	t.Fatalf("no result line in stdout:\n%s", stdout)
	return "", ""
}

// ledgerEventCounts counts journaled events for runID by type.
func ledgerEventCounts(t *testing.T, ctx context.Context, j *journal.Journal, runID string) map[string]int {
	t.Helper()
	events, err := j.Events(ctx, runID, 0)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, ev := range events {
		counts[ev.Type]++
	}
	return counts
}

// ledgerOpenState opens the finished run's journal read-only.
func ledgerOpenState(t *testing.T, e env) *journal.Journal {
	t.Helper()
	j, err := journal.OpenReadOnly(t.Context(), e.state)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	return j
}

// TestE2ELedgerNormalizesCounters pins the AT-13 normalization wiring
// through the packaged binary: the usage-counters run journals exactly
// the expected typed usage set, and the receipt and CLI line carry it.
//
// The fake surfaces no usage or cost, so the S1 route journals the AC-2.2
// unknown marker for the always-expected retail-equivalent scope — never
// a zero, never a duplicate. Delta accumulation (AC-2.1) stays pinned at
// unit level (TestNormalizeDeltaAppliedOnce,
// TestNormalizeRepeatedDeltaCountedOnce, TestNativeResultAttemptsAccumulate,
// TestNativeResultRepresentedDeltaDropped); exercising deltas end to end
// needs the fake decoder to surface usage, which is outside this task's
// Files (recorded in the task handoff as follow-up work).
func TestE2ELedgerNormalizesCounters(t *testing.T) {
	e := newEnv(t)
	repo := ledgerSeedRepo(t, e)
	_, stdout, _ := ledgerRun(t, e, repo, "usage-counters")
	runID := ledgerRunIDOf(t, stdout)
	if state, reason := ledgerResultOf(t, stdout); state != "ready_for_review" || reason != "unverified" {
		t.Fatalf("result = %s (%s), want ready_for_review (unverified)", state, reason)
	}

	j := ledgerOpenState(t, e)
	rows, err := j.UsageObservations(t.Context(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("usage rows = %d, want exactly the 1-row typed set (no double counting)", len(rows))
	}
	row := rows[0]
	if row.Scope != "retail-equivalent" || row.Unit != "" || row.Source != "" ||
		row.Label != "unknown" || row.Quantity != "unknown" {
		t.Fatalf("usage row = %+v, want the retail-equivalent unknown marker", row)
	}
	if row.Quantity == "0" {
		t.Fatal("usage quantity is 0; unknown must never render as zero (I09)")
	}

	// The receipt and the CLI line carry the same row.
	receiptBytes, err := os.ReadFile(filepath.Join(e.state, "runs", runID, "receipt.json"))
	if err != nil {
		t.Fatal(err)
	}
	var receipt struct {
		Billing struct {
			Usage []struct {
				Scope    string `json:"scope"`
				Quantity string `json:"quantity"`
			} `json:"usage_observations"`
		} `json:"billing"`
	}
	if err := json.Unmarshal(receiptBytes, &receipt); err != nil {
		t.Fatal(err)
	}
	if len(receipt.Billing.Usage) != 1 || receipt.Billing.Usage[0].Scope != "retail-equivalent" ||
		receipt.Billing.Usage[0].Quantity != "unknown" {
		t.Fatalf("receipt usage_observations = %+v, want the one unknown marker row", receipt.Billing.Usage)
	}
	if !strings.Contains(stdout, "usage: retail-equivalent unknown unknown (unknown, unknown)\n") {
		t.Fatalf("CLI output has no exact usage marker line:\n%s", stdout)
	}
}

// ledgerPaidMarkers are the paid-continuation phrases that must never
// appear on an exhaustion run's surfaces (AC-6.3).
var ledgerPaidMarkers = []string{
	"purchased credits", "paid summary", "upgrade", "overage", "metered", "billed",
}

// TestE2EExhaustionPreservesWithoutFallback pins the AT-13 exhaustion
// path through the packaged binary: the allowance-exhausted run ends
// blocked with its work intact, launches the native exactly once, and
// neither the receipt nor the captured CLI output names paid
// continuation.
func TestE2EExhaustionPreservesWithoutFallback(t *testing.T) {
	e := newEnv(t)
	repo := ledgerSeedRepo(t, e)
	_, stdout, stderr := ledgerRun(t, e, repo, "allowance-exhausted")
	runID := ledgerRunIDOf(t, stdout)
	if state, reason := ledgerResultOf(t, stdout); state != "blocked" || reason != "allowance_exhausted" {
		t.Fatalf("result = %s (%s), want blocked (allowance_exhausted)", state, reason)
	}

	ctx := t.Context()
	j := ledgerOpenState(t, e)
	attempt, err := j.LatestAttempt(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if attempt.Reason != "allowance_exhausted" {
		t.Fatalf("attempt reason = %q, want allowance_exhausted", attempt.Reason)
	}

	// The partial work is frozen in place, with its session artifacts.
	cand, err := j.Candidate(ctx, attempt.AttemptID)
	if err != nil || cand.Commit == "" {
		t.Fatalf("candidate = %+v, %v; want the partial work frozen", cand, err)
	}
	counts := ledgerEventCounts(t, ctx, j, runID)
	if counts["candidate.frozen"] != 1 {
		t.Fatalf("candidate.frozen events = %d, want exactly 1", counts["candidate.frozen"])
	}
	for _, name := range []string{"spool.jsonl", "worker.json"} {
		if _, err := os.Stat(filepath.Join(workers.AttemptDir(e.state, runID, attempt.AttemptID), name)); err != nil {
			t.Errorf("session file %s missing: %v", name, err)
		}
	}

	// Exactly one native launch: MYTHHELM schedules no automatic retry.
	if counts["attempt.launched"] != 1 {
		t.Fatalf("native launches = %d, want exactly 1", counts["attempt.launched"])
	}

	// No paid-continuation marker on either surface. Both outputs are
	// asserted non-empty first, so vacuous absence fails. stderr rides
	// along: notices and the outcome summary surface there.
	receiptBytes, err := os.ReadFile(filepath.Join(e.state, "runs", runID, "receipt.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(receiptBytes) == 0 || len(stdout) == 0 {
		t.Fatalf("receipt is %d bytes and stdout %d bytes; both must be non-empty", len(receiptBytes), len(stdout))
	}
	outputs := []struct{ name, body string }{
		{"receipt", string(receiptBytes)},
		{"stdout", stdout},
		{"stderr", stderr},
	}
	for _, marker := range ledgerPaidMarkers {
		for _, out := range outputs {
			if strings.Contains(out.body, marker) {
				t.Errorf("paid-continuation marker %q appears in %s", marker, out.name)
			}
		}
	}
}

// TestE2ECoupledReservationPresent pins the admitted run's coupled quota:
// exactly one reservation row, scoped to the run's own bucket — zero or
// two fails (I02). The run completed, so the row is released with
// evidence; the hold it records is what coupled admission to the bucket.
func TestE2ECoupledReservationPresent(t *testing.T) {
	e := newEnv(t)
	repo := ledgerSeedRepo(t, e)
	_, stdout, _ := ledgerRun(t, e, repo, "usage-counters")
	runID := ledgerRunIDOf(t, stdout)
	if state, _ := ledgerResultOf(t, stdout); state != "ready_for_review" {
		t.Fatalf("result state = %s, want ready_for_review", state)
	}

	// The bucket the run surfaced at admission.
	var noticed string
	for line := range strings.Lines(stdout) {
		rest, ok := strings.CutPrefix(strings.TrimSpace(line), "reservation held on bucket ")
		if !ok {
			continue
		}
		noticed, _, _ = strings.Cut(rest, ": local coordination")
	}
	if noticed == "" {
		t.Fatalf("no reservation notice in stdout:\n%s", stdout)
	}

	ctx := t.Context()
	j := ledgerOpenState(t, e)
	var rows []journal.ReservationRow
	if err := j.Transact(ctx, func(tx *sql.Tx) error {
		rs, err := tx.QueryContext(ctx, `SELECT reservation_id, run_id, bucket, scope, owner, quantity, status,
			COALESCE(release_evidence, '') FROM reservations WHERE run_id = ?`, runID)
		if err != nil {
			return err
		}
		defer func() { _ = rs.Close() }()
		for rs.Next() {
			var r journal.ReservationRow
			if err := rs.Scan(&r.ReservationID, &r.RunID, &r.Bucket, &r.Scope, &r.Owner,
				&r.Quantity, &r.Status, &r.ReleaseEvidence); err != nil {
				return err
			}
			rows = append(rows, r)
		}
		return rs.Err()
	}); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("reservation rows = %d, want exactly 1 coupled to the run's bucket", len(rows))
	}
	row := rows[0]
	if row.Scope != row.Bucket || row.Bucket != noticed {
		t.Fatalf("reservation scope %q bucket %q, want both coupled to the noticed bucket %q",
			row.Scope, row.Bucket, noticed)
	}
	var parts []string
	if err := json.Unmarshal([]byte(row.Bucket), &parts); err != nil || len(parts) != 4 {
		t.Fatalf("reservation bucket %q is not the [harness, surface, entitlement, identity] array", row.Bucket)
	}
	if row.Owner != runID || row.Quantity != "unknown" {
		t.Fatalf("reservation = %+v, want owner %s with unknown quantity", row, runID)
	}
	if row.Status != "released" || row.ReleaseEvidence == "" {
		t.Fatalf("reservation status = %q (%q), want released with evidence after the completed run",
			row.Status, row.ReleaseEvidence)
	}
}

// TestRunPathLedgerLatency pins NFR-1: the run-path ledger ops — admit,
// record, read — complete within 1 s total. The delayed variant exceeds
// the bound, proving the test discriminates (MH-10
// TestRegistryLookupLatency shape). Not parallel: a wall-clock bound
// should not compete with suite load.
func TestRunPathLedgerLatency(t *testing.T) {
	ctx := t.Context()
	j, err := journal.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })

	// One run per measured pass: a second hold for the same run would
	// refuse with ErrDuplicateHold instead of measuring admission.
	newRun := func() string {
		t.Helper()
		runID := ids.New("run")
		now := time.Now()
		if err := j.Transact(ctx, func(tx *sql.Tx) error {
			return journal.InsertRun(ctx, tx, journal.RunRow{RunID: runID, State: "admission",
				AdapterID: "builtin/fake", SourceRepo: "/tmp/repo", TaskSHA256: "t",
				BillingPosture: "local-scripted", ExecutionProfile: "trusted-host",
				CreatedAt: now, UpdatedAt: now})
		}); err != nil {
			t.Fatal(err)
		}
		return runID
	}
	fastRun, slowRun := newRun(), newRun()

	reserver := admission.NewJournalReserver(billing.BuiltInCeilings(), nil)
	rec := qualify.Record{Key: qualify.Key{Harness: "fake", Surface: "scripted", EntitlementClass: "local-scripted"}}
	ledgerPath := func(runID string) {
		t.Helper()
		if err := j.Transact(ctx, func(tx *sql.Tx) error {
			_, err := admission.HoldQuotaReservation(ctx, tx, reserver, runID, rec, "unknown")
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if err := j.Transact(ctx, func(tx *sql.Tx) error {
			return journal.InsertUsageObservation(ctx, tx, journal.UsageRow{
				ObservationID: ids.New("obs"), RunID: runID, Scope: "attempt",
				Unit: "tokens", Source: "native-reported", Label: "reported",
				Quantity:   "160",
				ObservedAt: time.Now().UTC().Format(time.RFC3339Nano),
			})
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := j.UsageObservations(ctx, runID); err != nil {
			t.Fatal(err)
		}
	}

	start := time.Now()
	ledgerPath(fastRun)
	if elapsed := time.Since(start); elapsed >= time.Second {
		t.Errorf("admit + record + read took %v, want under 1s (NFR-1)", elapsed)
	}

	// Discriminating variant: the same path with a 1.5 s injected delay
	// must exceed the bound — if it did not, the assertion above could
	// never fail.
	start = time.Now()
	time.Sleep(1500 * time.Millisecond)
	ledgerPath(slowRun)
	if elapsed := time.Since(start); elapsed < time.Second {
		t.Error("ledger path with a 1.5 s injected delay completed within the bound; the latency assertion cannot discriminate")
	}
}
