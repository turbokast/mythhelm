package supervisor_test

import (
	"database/sql"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/v2contract"
	"github.com/turbokast/mythhelm/internal/workspace"
)

func applyDB(t *testing.T, state string) *sql.DB {
	t.Helper()
	path := filepath.ToSlash(filepath.Join(state, journal.DBName))
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	u := url.URL{Scheme: "file", Path: path}
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func readyApplyFixture(t *testing.T) (fixture, string) {
	t.Helper()
	f := newFixture(t)
	digest := f.config(t, "pass")
	// f.run already logs the redacted worker diagnostic on failure.
	if code, _, stderr := f.run(t, f.checkedRun(digest)...); code != 0 {
		t.Fatalf("run exit %d: %s", code, stderr)
	}
	return f, f.onlyRun(t).RunID
}

func checkApplyJSONL(t *testing.T, out, runID string, code int) {
	t.Helper()
	if strings.Count(out, "\n") != 1 {
		t.Fatalf("apply JSONL must contain one result: %q", out)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(err)
	}
	if result["type"] != "apply.result" || result["run_id"] != runID || result["exit_code"] != float64(code) {
		t.Fatalf("unexpected apply result: %v", result)
	}
	wantCategory := ""
	if code == 3 {
		wantCategory = "admission_blocked"
	}
	if result["error_category"] != wantCategory {
		t.Fatalf("apply category = %v, want %q", result["error_category"], wantCategory)
	}
}

func TestApplyCreatesOnlyTheBranch(t *testing.T) {
	f, runID := readyApplyFixture(t)
	before := f.fingerprint(t)
	head := f.git(t, "rev-parse", "HEAD")
	config, err := os.ReadFile(filepath.Join(f.repo, ".git", "config"))
	if err != nil {
		t.Fatal(err)
	}
	code, out, stderr := f.run(t, "apply", runID, "--to-branch", "review/demo", "--format", "jsonl")
	if code != 0 {
		t.Fatalf("apply exit %d: %s", code, stderr)
	}
	checkApplyJSONL(t, out, runID, code)
	if f.onlyRun(t).State != string(supervisor.RunCompleted) {
		t.Fatal("run did not complete")
	}
	if got := f.git(t, "rev-parse", "HEAD"); got != head {
		t.Fatalf("HEAD changed: %s -> %s", head, got)
	}
	if afterConfig, err := os.ReadFile(filepath.Join(f.repo, ".git", "config")); err != nil || string(config) != string(afterConfig) {
		t.Fatalf("config changed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(f.repo, ".git", "FETCH_HEAD")); !os.IsNotExist(err) {
		t.Fatalf("FETCH_HEAD written: %v", err)
	}
	f.git(t, "update-ref", "-d", "refs/heads/review/demo")
	if after := f.fingerprint(t); after != before {
		t.Fatalf("source fingerprint changed beyond new ref: %s -> %s", before, after)
	}
	for _, name := range []string{"receipt.json", "receipt.v1.json"} {
		if _, err := os.Stat(filepath.Join(f.state, "runs", runID, name)); err != nil {
			t.Fatal(err)
		}
	}
	r, _, err := supervisor.ReadReceipt(t.Context(), f.journal(t), runID)
	if err != nil || r["schema_version"] != float64(2) || r["state"] != string(supervisor.RunCompleted) {
		t.Fatalf("v2 receipt: %v, %v", r, err)
	}
}

func TestApplyRefusesExistingBranch(t *testing.T) {
	f, runID := readyApplyFixture(t)
	before := f.fingerprint(t)
	code, out, stderr := f.run(t, "apply", runID, "--to-branch", "main", "--format", "jsonl")
	if code != 3 || !strings.Contains(stderr, "already exists") {
		t.Fatalf("apply exit %d: %s", code, stderr)
	}
	checkApplyJSONL(t, out, runID, code)
	if f.fingerprint(t) != before || f.onlyRun(t).State != string(supervisor.RunReadyForReview) {
		t.Fatal("refusal mutated source or run")
	}
}

func TestApplyRefusesNotReadyForReview(t *testing.T) {
	f := newFixture(t)
	digest := f.config(t, "fail")
	if code, _, stderr := f.run(t, f.checkedRun(digest)...); code != 5 {
		t.Fatalf("run exit %d: %s", code, stderr)
	}
	code, _, stderr := f.run(t, "apply", f.onlyRun(t).RunID, "--to-branch", "review/demo")
	if code != 3 || !strings.Contains(stderr, "expected ready_for_review") {
		t.Fatalf("apply exit %d: %s", code, stderr)
	}
}

func TestApplyRefusesUnacceptedFlags(t *testing.T) {
	f, runID := readyApplyFixture(t)
	j := f.journal(t)
	attempt, err := j.LatestAttempt(t.Context(), runID)
	if err != nil {
		t.Fatal(err)
	}
	// A flagged candidate is a projection condition; the CLI must check it
	// before recording intent or fetching into the source repository.
	db := applyDB(t, f.state)
	if _, err := db.ExecContext(t.Context(), `UPDATE candidates SET flags = ? WHERE attempt_id = ?`,
		`[{"name":"large_file","path":"candidate.txt"}]`, attempt.AttemptID); err != nil {
		t.Fatal(err)
	}
	before := f.fingerprint(t)
	code, _, stderr := f.run(t, "apply", runID, "--to-branch", "review/demo")
	if code != 3 || !strings.Contains(stderr, "--accept-flags") {
		t.Fatalf("apply exit %d: %s", code, stderr)
	}
	if f.fingerprint(t) != before || typeIndex(f.events(t, runID), "apply.intent_recorded") >= 0 {
		t.Fatal("unaccepted flags caused an effect")
	}
}

func TestApplyRefusesUnacceptedUnverified(t *testing.T) {
	f := newFixture(t)
	if code, _, stderr := f.run(t, f.fakeRun()...); code != 5 {
		t.Fatalf("run exit %d: %s", code, stderr)
	}
	runID := f.onlyRun(t).RunID
	before := f.fingerprint(t)
	code, _, stderr := f.run(t, "apply", runID, "--to-branch", "review/demo")
	if code != 3 || !strings.Contains(stderr, "--accept-unverified") {
		t.Fatalf("apply exit %d: %s", code, stderr)
	}
	if f.fingerprint(t) != before {
		t.Fatal("unaccepted unverified candidate changed source")
	}
	code, _, stderr = f.run(t, "apply", runID, "--to-branch", "review/demo", "--accept-unverified")
	if code != 0 {
		t.Fatalf("accepted apply exit %d: %s", code, stderr)
	}
}

func TestApplyRefusesInvalidRefName(t *testing.T) {
	f, runID := readyApplyFixture(t)
	code, _, stderr := f.run(t, "apply", runID, "--to-branch=bad..name")
	if code != 2 || !strings.Contains(stderr, "invalid destination branch") {
		t.Fatalf("apply exit %d: %s", code, stderr)
	}
	if f.onlyRun(t).State != string(supervisor.RunReadyForReview) {
		t.Fatal("invalid branch changed run state")
	}
}

func recordApplyEvent(t *testing.T, f fixture, runID, branch, commit, eventType string, state supervisor.RunState) {
	t.Helper()
	j := f.journal(t)
	payload, err := json.Marshal(map[string]string{"branch": branch, "target_repo": f.onlyRun(t).SourceRepo, "candidate_commit": commit})
	if err != nil {
		t.Fatal(err)
	}
	if err := j.Append(t.Context(), journal.Event{SchemaVersion: journal.EnvelopeVersion, EventID: ids.New("evt"),
		RunID: runID, ProducerID: ids.New("sup"), ProducerSequence: 1, Generation: 1,
		ObservedAt: time.Now().UTC(), Type: eventType, Payload: payload}, nil); err != nil {
		t.Fatal(err)
	}
	p := supervisor.NewProducer(ids.New("sup"), 1)
	if err := supervisor.TransitionRun(t.Context(), j, runID, state, "", p); err != nil {
		t.Fatal(err)
	}
}

func recordApplyIntent(t *testing.T, f fixture, runID, branch, commit string) {
	t.Helper()
	recordApplyEvent(t, f, runID, branch, commit, "apply.intent_recorded", supervisor.RunApplying)
}

func TestApplyRetryBlocksConflictingBranch(t *testing.T) {
	f, runID := readyApplyFixture(t)
	j := f.journal(t)
	attempt, err := j.LatestAttempt(t.Context(), runID)
	if err != nil {
		t.Fatal(err)
	}
	c, err := j.Candidate(t.Context(), attempt.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	branch := "review/conflict"
	recordApplyIntent(t, f, runID, branch, c.Commit)
	f.git(t, "branch", branch)
	before := f.fingerprint(t)
	code, _, stderr := f.run(t, "apply", runID, "--to-branch", branch)
	if code != 3 {
		t.Fatalf("conflicting retry exit %d: %s", code, stderr)
	}
	run := f.onlyRun(t)
	if run.State != string(supervisor.RunBlocked) || run.Reason != "branch_exists" {
		t.Fatalf("conflicting retry left run %s/%s active", run.State, run.Reason)
	}
	if f.fingerprint(t) != before {
		t.Fatal("conflicting retry changed source")
	}
	r, _, err := supervisor.ReadReceipt(t.Context(), j, runID)
	if err != nil || r["state"] != string(supervisor.RunBlocked) {
		t.Fatalf("blocked receipt = %v: %v", r, err)
	}
}

func TestApplyReconcilesAfterCrash(t *testing.T) {
	f, runID := readyApplyFixture(t)
	j := f.journal(t)
	attempt, err := j.LatestAttempt(t.Context(), runID)
	if err != nil {
		t.Fatal(err)
	}
	c, err := j.Candidate(t.Context(), attempt.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	branch := "review/recovered"
	recordApplyIntent(t, f, runID, branch, c.Commit)
	if err := workspace.ApplyBranch(t.Context(), f.repo, attempt.WorkspacePath,
		"refs/mythhelm/candidates/"+attempt.AttemptID, branch, c.Commit); err != nil {
		t.Fatal(err)
	}
	// Make the managed clone unavailable: reconciliation must complete from
	// the existing branch and journal without issuing another fetch.
	if _, err := applyDB(t, f.state).ExecContext(t.Context(), `UPDATE attempts SET workspace_path = ? WHERE attempt_id = ?`,
		filepath.Join(t.TempDir(), "missing"), attempt.AttemptID); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := f.run(t, "apply", runID, "--to-branch", branch)
	if code != 0 || f.onlyRun(t).State != string(supervisor.RunCompleted) {
		t.Fatalf("reconciled apply exit %d: %s", code, stderr)
	}
}

func TestApplyRetryBlocksSymbolicBranch(t *testing.T) {
	f, runID := readyApplyFixture(t)
	j := f.journal(t)
	attempt, err := j.LatestAttempt(t.Context(), runID)
	if err != nil {
		t.Fatal(err)
	}
	c, err := j.Candidate(t.Context(), attempt.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	branch := "review/symbolic-conflict"
	recordApplyIntent(t, f, runID, branch, c.Commit)
	f.git(t, "symbolic-ref", "refs/heads/"+branch, "refs/heads/untouched")
	before := f.fingerprint(t)
	code, _, stderr := f.run(t, "apply", runID, "--to-branch", branch)
	if code != 3 {
		t.Fatalf("symbolic retry %d: %s", code, stderr)
	}
	run := f.onlyRun(t)
	if run.State != "blocked" || run.Reason != "branch_exists" {
		t.Fatalf("symbolic retry still active: %s/%s", run.State, run.Reason)
	}
	if f.fingerprint(t) != before {
		t.Fatal("symbolic destination changed")
	}
}

func TestApplyRefusesExistingMatchingBranchWithoutIntent(t *testing.T) {
	f, runID := readyApplyFixture(t)
	j := f.journal(t)
	attempt, err := j.LatestAttempt(t.Context(), runID)
	if err != nil {
		t.Fatal(err)
	}
	c, err := j.Candidate(t.Context(), attempt.AttemptID)
	if err != nil {
		t.Fatal(err)
	}
	f.git(t, "fetch", "--no-write-fetch-head", attempt.WorkspacePath, "refs/mythhelm/candidates/"+attempt.AttemptID)
	f.git(t, "update-ref", "refs/heads/already-present", c.Commit)
	before := f.fingerprint(t)
	if code, _, stderr := f.run(t, "apply", runID, "--to-branch", "already-present"); code != 3 {
		t.Fatalf("existing matching branch %d: %s", code, stderr)
	}
	if f.onlyRun(t).State != "ready_for_review" || f.fingerprint(t) != before {
		t.Fatal("initial refusal changed run or source")
	}
	for _, ev := range f.events(t, runID) {
		if ev.Type == "apply.intent_recorded" {
			t.Fatal("initial refusal recorded apply intent")
		}
	}
}

// staleCases tamper with a ready run so its passed verdict no longer applies;
// each names the one binding dimension that changed.
var staleCases = []struct {
	name, dimension string
	tamper          func(t *testing.T, f fixture, runID string)
}{
	{name: "the task file is rewritten", dimension: "contract", tamper: func(t *testing.T, f fixture, runID string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(f.state, "runs", runID, "task.md"), []byte("a different task\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}},
	{name: "the task file is deleted", dimension: "contract", tamper: func(t *testing.T, f fixture, runID string) {
		t.Helper()
		if err := os.Remove(filepath.Join(f.state, "runs", runID, "task.md")); err != nil {
			t.Fatal(err)
		}
	}},
	{name: "the candidate row moves to another commit", dimension: "candidate", tamper: func(t *testing.T, f fixture, runID string) {
		t.Helper()
		attempt, err := f.journal(t).LatestAttempt(t.Context(), runID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := rawDB(t, f.state).ExecContext(t.Context(), `UPDATE candidates SET candidate_commit = ? WHERE attempt_id = ?`,
			f.git(t, "rev-parse", "HEAD"), attempt.AttemptID); err != nil {
			t.Fatal(err)
		}
	}},
	{name: "a newer verification attests another check definition", dimension: "check_definition", tamper: func(t *testing.T, f fixture, runID string) {
		t.Helper()
		f.addVerification(t, f.latestVerification(t, runID), verificationVariant{result: "passed",
			attest: func(a *v2contract.Verification) { a.CheckDefinition = otherDigest() }})
	}},
}

func TestApplyRefusesStaleVerdict(t *testing.T) {
	t.Parallel()
	t.Run("an unmodified run applies", func(t *testing.T) {
		t.Parallel()
		f, runID := readyApplyFixture(t)
		if code, _, stderr := f.run(t, "apply", runID, "--to-branch", "review/demo"); code != 0 {
			t.Fatalf("unmodified apply exit %d: %s", code, stderr)
		}
	})
	for _, tc := range staleCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f, runID := readyApplyFixture(t)
			tc.tamper(t, f, runID)
			before := f.fingerprint(t)
			code, _, stderr := f.run(t, "apply", runID, "--to-branch", "review/demo")
			if code != 3 || !strings.Contains(stderr, "(changed: "+tc.dimension+")") || !strings.Contains(stderr, "--accept-unverified") {
				t.Fatalf("apply exit %d, stderr %q; want exit 3 naming (changed: %s) and --accept-unverified", code, stderr, tc.dimension)
			}
			if f.fingerprint(t) != before || typeIndex(f.events(t, runID), "apply.intent_recorded") >= 0 {
				t.Fatal("a stale verdict caused an apply effect")
			}
		})
	}
}

func applyOverride(t *testing.T, f fixture, runID string) (map[string]any, bool) {
	t.Helper()
	r, _, err := supervisor.ReadReceipt(t.Context(), f.journal(t), runID)
	if err != nil {
		t.Fatal(err)
	}
	if r["schema_version"] != float64(2) {
		t.Fatalf("receipt schema_version = %v, want 2", r["schema_version"])
	}
	acc, _ := r["acceptance"].(map[string]any)
	override, ok := acc["apply_override"].(map[string]any)
	return override, ok
}

func TestApplyOverrideRecordedInReceipt(t *testing.T) {
	t.Parallel()
	t.Run("a stale verdict applied over is recorded", func(t *testing.T) {
		t.Parallel()
		f, runID := readyApplyFixture(t)
		staleCases[0].tamper(t, f, runID)
		if code, _, stderr := f.run(t, "apply", runID, "--to-branch", "review/demo", "--accept-unverified"); code != 0 {
			t.Fatalf("overridden apply exit %d: %s", code, stderr)
		}
		override, ok := applyOverride(t, f, runID)
		if !ok {
			t.Fatal("the version 2 receipt has no acceptance.apply_override")
		}
		changed, _ := override["changed"].([]any)
		if override["state"] != "stale" || !slices.Equal(changed, []any{"contract"}) {
			t.Fatalf("apply_override = %v, want state stale and changed [contract]", override)
		}
	})
	t.Run("a bound apply records no override", func(t *testing.T) {
		t.Parallel()
		f, runID := readyApplyFixture(t)
		if code, _, stderr := f.run(t, "apply", runID, "--to-branch", "review/demo"); code != 0 {
			t.Fatalf("bound apply exit %d: %s", code, stderr)
		}
		if override, ok := applyOverride(t, f, runID); ok {
			t.Fatalf("a bound apply recorded apply_override %v", override)
		}
	})
}

func TestApplyRefusesUnboundVerified(t *testing.T) {
	t.Parallel()
	f, runID := readyApplyFixture(t)
	f.addVerification(t, f.latestVerification(t, runID), verificationVariant{result: "passed", unbound: true})
	before := f.fingerprint(t)
	code, _, stderr := f.run(t, "apply", runID, "--to-branch", "review/demo")
	if code != 3 || !strings.Contains(stderr, "recorded before acceptance binding existed") || !strings.Contains(stderr, "--accept-unverified") {
		t.Fatalf("apply exit %d, stderr %q; want exit 3 naming the missing binding and --accept-unverified", code, stderr)
	}
	if f.fingerprint(t) != before {
		t.Fatal("an unbound verdict changed source")
	}
	if code, _, stderr := f.run(t, "apply", runID, "--to-branch", "review/demo", "--accept-unverified"); code != 0 {
		t.Fatalf("overridden apply exit %d: %s", code, stderr)
	}
	if override, ok := applyOverride(t, f, runID); !ok || override["state"] != "unbound" {
		t.Fatalf("apply_override = %v (present %v), want state unbound", override, ok)
	}
}

func TestApplyUnverifiedMessageNamesReason(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	digest := f.commitChecks(t, missingCheck(t, "missing"))
	if code, _, stderr := f.run(t, f.checkedRun(digest)...); code != 5 {
		t.Fatalf("run exit %d: %s", code, stderr)
	}
	code, _, stderr := f.run(t, "apply", f.onlyRun(t).RunID, "--to-branch", "review/demo")
	if code != 3 || !strings.Contains(stderr, "NOT RUN: checks_unavailable") || !strings.Contains(stderr, "--accept-unverified") {
		t.Fatalf("apply exit %d, stderr %q; want exit 3 with NOT RUN: checks_unavailable and --accept-unverified", code, stderr)
	}
}

func TestResumedApplySkipsGate(t *testing.T) {
	t.Parallel()
	f, runID := readyApplyFixture(t)
	c := f.candidateOf(t, runID)
	branch := "review/resumed"
	recordApplyIntent(t, f, runID, branch, c.Commit)
	staleCases[0].tamper(t, f, runID)
	if code, _, stderr := f.run(t, "apply", runID, "--to-branch", branch); code != 0 {
		t.Fatalf("resumed apply exit %d: %s", code, stderr)
	}
	if f.onlyRun(t).State != string(supervisor.RunCompleted) {
		t.Fatalf("resumed run is %s, want completed", f.onlyRun(t).State)
	}
	if got := f.git(t, "rev-parse", "refs/heads/"+branch); got != c.Commit {
		t.Fatalf("branch at %s, want the candidate %s", got, c.Commit)
	}
}
