package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/supervisor"
)

func reviewFixture(t *testing.T, malicious string) (string, string) {
	t.Helper()
	return reviewFixtureWith(t, malicious, nil)
}

// reviewFixtureWith journals a ready_for_review run whose receipt is the
// fixture's after mutate edits it.
func reviewFixtureWith(t *testing.T, malicious string, mutate func(supervisor.Receipt)) (string, string) {
	t.Helper()
	dir := stateHome(t)
	j := openJournal(t, dir)
	defer func() { _ = j.Close() }()
	producer := supervisor.NewProducer(ids.New("sup"), 1)
	runID := createRun(t, j, producer, "/tmp/repo")
	for _, state := range []supervisor.RunState{supervisor.RunAdmission, supervisor.RunExecuting, supervisor.RunVerifying, supervisor.RunReadyForReview} {
		if err := supervisor.TransitionRun(t.Context(), j, runID, state, "", producer); err != nil {
			t.Fatal(err)
		}
	}
	runDir := filepath.Join(dir, "runs", runID)
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	r := supervisor.Receipt{
		"schema_version": 1, "run_id": runID, "state": "ready_for_review", "exit_code": 0,
		"requested_outcome": map[string]any{"title": "Demo task"},
		"candidate":         map[string]any{"commit": "unknown", "flags": []any{map[string]any{"path": malicious, "flag": "large_file"}}},
		"verification":      map[string]any{"checks": []any{map[string]any{"name": "test", "status": "passed", "evidence": malicious, "sha256": "abc"}}},
	}
	if mutate != nil {
		mutate(r)
	}
	sha, err := supervisor.WriteReceipt(runDir, r)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{"schema_version": 1, "sha256": sha, "path": "receipt.json"})
	if err != nil {
		t.Fatal(err)
	}
	err = j.Append(t.Context(), journal.Event{
		SchemaVersion: journal.EnvelopeVersion, EventID: ids.New("evt"), RunID: runID,
		ProducerID: ids.New("sup"), ProducerSequence: 1, Generation: 1,
		ObservedAt: time.Now().UTC(), Type: "receipt.written", Payload: payload,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return dir, runID
}

func TestReviewSanitisesMaliciousFilename(t *testing.T) {
	_, runID := reviewFixture(t, "evil\x1b]8;;https://example.test\a.txt")
	code, out, stderr := runMain("review", runID, "--no-diff")
	if code != 0 {
		t.Fatalf("review exit %d: %s", code, stderr)
	}
	if strings.Contains(out, "\x1b") || strings.Contains(out, "\a") {
		t.Fatalf("terminal control reached output: %q", out)
	}
	if !strings.Contains(out, "evil") || !strings.Contains(out, "Checks:") {
		t.Fatalf("review omitted evidence: %q", out)
	}
}

func TestReviewJSONLSingleObject(t *testing.T) {
	_, runID := reviewFixture(t, "safe.txt")
	code, out, stderr := runMain("review", "--format", "jsonl", runID)
	if code != 0 {
		t.Fatalf("review exit %d: %s", code, stderr)
	}
	if strings.Count(out, "\n") != 1 {
		t.Fatalf("JSONL must have one line: %q", out)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatal(err)
	}
	if m["run_id"] != runID || m["schema_version"] != float64(1) {
		t.Fatalf("wrong receipt: %v", m)
	}
}

func TestReviewRejectsTamperedReceipt(t *testing.T) {
	dir, runID := reviewFixture(t, "safe.txt")
	p := filepath.Join(dir, "runs", runID, "receipt.json")
	if err := os.WriteFile(p, []byte(`{"run_id":"forged"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := runMain("review", runID, "--format", "jsonl")
	if code == 0 || !strings.Contains(stderr, "does not match") {
		t.Fatalf("tampered receipt exit %d: %s", code, stderr)
	}
}

func TestReviewRejectsStaleReceiptState(t *testing.T) {
	dir, runID := reviewFixture(t, "safe.txt")
	db := rawDB(t, dir)
	if _, err := db.ExecContext(t.Context(), `UPDATE runs SET state = 'completed' WHERE run_id = ?`, runID); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := runMain("review", runID, "--format", "jsonl")
	if code == 0 || !strings.Contains(stderr, "does not match its run projection") {
		t.Fatalf("stale receipt exit %d: %s", code, stderr)
	}
}

var createdRun = regexp.MustCompile(`run (run_[0-9A-Z]+): created`)

// realRun drives `mythhelm run` on the fake route in the fixture's repository
// and returns the run it created and the exit code.
func realRun(t *testing.T, task string, extra ...string) (string, int) {
	t.Helper()
	args := append([]string{"run", "--task-file", task, "--adapter", "fake", "--billing", "local-scripted",
		"--execution-profile", "trusted-host", "--non-interactive", "--plain"}, extra...)
	code, stdout, stderr := runMain(args...)
	m := createdRun.FindStringSubmatch(stdout)
	if m == nil {
		t.Fatalf("run exit %d created no run:\n%s\n%s", code, stdout, stderr)
	}
	return m[1], code
}

// passingCheckArgs commits a one-check project configuration to the working
// repository and returns the flags that trust it. The check is this test
// binary's own demo helper, which exits 0.
func passingCheckArgs(t *testing.T) []string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	config := "schema_version = 1\n[[checks]]\nname = \"demo\"\nargv = [" + strconv.Quote(exe) + ", " +
		strconv.Quote(DemoCheckCommand) + ", \"pass\"]\ntimeout = \"30s\"\n"
	repo, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "mythhelm.toml"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "mythhelm.toml"}, {"-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "checks"}} {
		if out, err := gitIn(repo, args...); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	_, digest, err := admission.ParseProjectConfig([]byte(config))
	if err != nil {
		t.Fatal(err)
	}
	return []string{"--trust-project-config", "sha256:" + digest}
}

// linesWith returns every output line that starts with prefix.
func linesWith(out, prefix string) []string {
	var got []string
	for line := range strings.Lines(out) {
		if line = strings.TrimRight(line, "\n"); strings.HasPrefix(line, prefix) {
			got = append(got, line)
		}
	}
	return got
}

// wantOneLine fails unless exactly one line starts with prefix and it equals want.
func wantOneLine(t *testing.T, out, prefix, want string) {
	t.Helper()
	got := linesWith(out, prefix)
	if len(got) != 1 || got[0] != want {
		t.Errorf("lines starting %q = %q, want exactly [%q]\nfull output:\n%s", prefix, got, want, out)
	}
}

const declaredSentence = "entitlement: user-declared, NOT verified by MYTHHELM; paid continuation: unknown (user declares disabled)"

// addOldVerification journals a verification the way a build before schema 8
// left it: a row with no binding. Its finish time orders it after every row
// the run recorded before it.
func addOldVerification(t *testing.T, dir, runID, result string, finished time.Time) {
	t.Helper()
	db := rawDB(t, dir)
	defer func() { _ = db.Close() }()
	at := finished.UTC().Format(time.RFC3339Nano)
	_, err := db.ExecContext(t.Context(), `INSERT INTO verifications
		(verification_id, run_id, candidate_commit, config_sha256, result, started_at, finished_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, ids.New("ver"), runID, strings.Repeat("a", 40), strings.Repeat("b", 64), result, at, at)
	if err != nil {
		t.Fatal(err)
	}
}

func TestReviewPlainShowsAcceptanceStatus(t *testing.T) {
	review := func(t *testing.T, runID string) string {
		t.Helper()
		code, out, stderr := runMain("review", runID, "--no-diff")
		if code != 0 {
			t.Fatalf("review exit %d: %s", code, stderr)
		}
		return out
	}

	t.Run("a bound run goes stale when its task file changes, and an old row is unbound", func(t *testing.T) {
		task := ledgerRunFixture(t)
		runID, code := realRun(t, task, passingCheckArgs(t)...)
		if code != 0 {
			t.Fatalf("checked run exit %d, want 0", code)
		}
		wantOneLine(t, review(t, runID), "Acceptance:", "Acceptance: bound")

		taskPath := filepath.Join(os.Getenv("MYTHHELM_HOME"), "runs", runID, "task.md")
		if err := os.WriteFile(taskPath, []byte("# Demo task\n\nA different contract.\n"), 0o600); err != nil { // #nosec G703 -- the run's own task file under the fixture's state directory
			t.Fatal(err)
		}
		wantOneLine(t, review(t, runID), "Acceptance:", "Acceptance: STALE (changed: contract); re-evaluation required")

		addOldVerification(t, os.Getenv("MYTHHELM_HOME"), runID, "passed", time.Now().Add(time.Hour))
		wantOneLine(t, review(t, runID), "Acceptance:", "Acceptance: unbound (verification recorded before acceptance binding existed)")
	})

	t.Run("a run with no verification, a rejected one and an unverified one", func(t *testing.T) {
		dir, runID := reviewFixture(t, "safe.txt")
		wantOneLine(t, review(t, runID), "Acceptance:", "Acceptance: unverified (no verification recorded)")

		addOldVerification(t, dir, runID, "failed", time.Now().Add(time.Hour))
		wantOneLine(t, review(t, runID), "Acceptance:", "Acceptance: rejected (failed)")

		addOldVerification(t, dir, runID, "not_run", time.Now().Add(2*time.Hour))
		wantOneLine(t, review(t, runID), "Acceptance:", "Acceptance: unverified (no reason recorded)")
	})
}

func TestReviewPlainShowsEntitlementAndVerification(t *testing.T) {
	receipts := []struct {
		name         string
		statement    any
		verification map[string]any
		reasons      map[string]any
		wantStmt     string
		wantVerif    string
	}{
		{name: "declared posture prints its sentence and a bound pass", statement: declaredSentence, wantStmt: declaredSentence,
			verification: map[string]any{"label": "passed", "reason": "", "binding": map[string]any{"candidate": map[string]any{"format": "git_commit", "value": "c0ffee"}}},
			wantVerif:    "Verification: passed (bound to c0ffee)"},
		{name: "an inspect run says why nothing ran", statement: declaredSentence, wantStmt: declaredSentence,
			verification: map[string]any{"label": "NOT RUN", "reason": "checks_refused_under_inspect", "binding": "unknown"},
			wantVerif:    "Verification: NOT RUN (checks_refused_under_inspect)"},
		{name: "an invalid verdict is labelled INVALID with its reason", statement: declaredSentence, wantStmt: declaredSentence,
			verification: map[string]any{"label": "INVALID", "reason": "evaluator_digest_mismatch", "binding": "unknown"},
			wantVerif:    "Verification: INVALID (evaluator_digest_mismatch)"},
		{name: "a failed verdict prints failed", statement: declaredSentence, wantStmt: declaredSentence,
			verification: map[string]any{"label": "failed", "reason": "checks_failed", "binding": "unknown"},
			wantVerif:    "Verification: failed"},
		{name: "a run with no verification says why it is unknown", statement: declaredSentence, wantStmt: declaredSentence,
			verification: map[string]any{"label": "unknown", "reason": "unknown", "binding": "unknown"},
			reasons:      map[string]any{"verification.label": "no verification was recorded"},
			wantVerif:    "Verification: unknown (no verification was recorded)"},
		{name: "a NOT RUN with no recorded reason says so, never blank", statement: declaredSentence, wantStmt: declaredSentence,
			verification: map[string]any{"label": "NOT RUN", "reason": "unknown", "binding": "unknown"},
			wantVerif:    "Verification: NOT RUN (reason unknown)"},
		{name: "a receipt without a statement prints entitlement unknown", wantStmt: "entitlement: unknown",
			verification: map[string]any{"label": "failed", "reason": "checks_failed", "binding": "unknown"},
			wantVerif:    "Verification: failed"},
	}
	for _, tc := range receipts {
		t.Run(tc.name, func(t *testing.T) {
			// Not parallel: the fixture points MYTHHELM_HOME at its state directory.
			_, runID := reviewFixtureWith(t, "safe.txt", func(r supervisor.Receipt) {
				r["verification"] = tc.verification
				if tc.statement != nil {
					r["billing"] = map[string]any{"entitlement_statement": tc.statement}
				}
				if tc.reasons != nil {
					r["unknown_reasons"] = tc.reasons
				}
			})
			code, out, stderr := runMain("review", runID, "--no-diff")
			if code != 0 {
				t.Fatalf("review exit %d: %s", code, stderr)
			}
			wantOneLine(t, out, "entitlement:", tc.wantStmt)
			wantOneLine(t, out, "Verification:", tc.wantVerif)
		})
	}

	t.Run("real runs", func(t *testing.T) {
		task := ledgerRunFixture(t)
		checks := passingCheckArgs(t)
		waived, code := realRun(t, task, "--no-checks")
		if code != 5 {
			t.Fatalf("--no-checks run exit %d, want 5", code)
		}
		passed, code := realRun(t, task, checks...)
		if code != 0 {
			t.Fatalf("checked run exit %d, want 0", code)
		}
		const scripted = "entitlement: not applicable (local-scripted: no inference, no network)"

		_, out, stderr := runMain("review", waived, "--no-diff")
		if !strings.Contains(out, "Run: "+waived) {
			t.Fatalf("review of the waived run:\n%s\n%s", out, stderr)
		}
		wantOneLine(t, out, "entitlement:", scripted)
		wantOneLine(t, out, "Verification:", "Verification: NOT RUN (waived by --no-checks)")
		// The reason belongs to the verdict and to the gate that reads it
		// (design §2.7); no other line, such as Checks, repeats it.
		var withReason []string
		for line := range strings.Lines(out) {
			if strings.Contains(line, "waived by --no-checks") {
				withReason = append(withReason, strings.TrimRight(line, "\n"))
			}
		}
		wantReasonLines := []string{"Verification: NOT RUN (waived by --no-checks)", "Acceptance: unverified (waived by --no-checks)"}
		if !slices.Equal(withReason, wantReasonLines) {
			t.Errorf("lines naming the waiver = %q, want %q", withReason, wantReasonLines)
		}

		_, jsonl, _ := runMain("review", passed, "--format", "jsonl")
		var receipt struct {
			Candidate struct {
				Commit string `json:"commit"`
			} `json:"candidate"`
		}
		if err := json.Unmarshal([]byte(jsonl), &receipt); err != nil || len(receipt.Candidate.Commit) < 40 {
			t.Fatalf("receipt of the passed run: %v %q", err, jsonl)
		}
		_, out, _ = runMain("review", passed, "--no-diff")
		wantOneLine(t, out, "Verification:", "Verification: passed (bound to "+receipt.Candidate.Commit+")")
		wantOneLine(t, out, "Candidate:", "Candidate: "+receipt.Candidate.Commit)
	})
}
