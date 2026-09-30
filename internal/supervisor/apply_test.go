package supervisor_test

import (
	"database/sql"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/workspace"
)

func applyDB(t *testing.T, state string) *sql.DB {
	t.Helper()
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(filepath.Join(state, journal.DBName))}
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
	if code, _, stderr := f.run(t, f.checkedRun(digest)...); code != 0 {
		t.Fatalf("run exit %d: %s", code, stderr)
	}
	return f, f.onlyRun(t).RunID
}

func TestApplyCreatesOnlyTheBranch(t *testing.T) {
	f, runID := readyApplyFixture(t)
	before := f.fingerprint(t)
	head := f.git(t, "rev-parse", "HEAD")
	config, err := os.ReadFile(filepath.Join(f.repo, ".git", "config"))
	if err != nil {
		t.Fatal(err)
	}
	code, _, stderr := f.run(t, "apply", runID, "--to-branch", "review/demo", "--format", "jsonl")
	if code != 0 {
		t.Fatalf("apply exit %d: %s", code, stderr)
	}
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
	code, _, stderr := f.run(t, "apply", runID, "--to-branch", "main")
	if code != 3 || !strings.Contains(stderr, "already exists") {
		t.Fatalf("apply exit %d: %s", code, stderr)
	}
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
	payload, _ := json.Marshal(map[string]string{"branch": branch, "target_repo": f.repo, "candidate_commit": c.Commit})
	if err := j.Append(t.Context(), journal.Event{SchemaVersion: journal.EnvelopeVersion, EventID: ids.New("evt"),
		RunID: runID, ProducerID: ids.New("sup"), ProducerSequence: 1, Generation: 1,
		ObservedAt: time.Now().UTC(), Type: "apply.intent_recorded", Payload: payload}, nil); err != nil {
		t.Fatal(err)
	}
	p := supervisor.NewProducer(ids.New("sup"), 1)
	if err := supervisor.TransitionRun(t.Context(), j, runID, supervisor.RunApplying, "", p); err != nil {
		t.Fatal(err)
	}
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
