package cli

import (
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/supervisor"
)

// TestLedgerNoticeGoldens pins the end-of-run ledger notice lines: one usage
// line per row (or none recorded), the reserve estimate, unknown remaining
// and the retry schedule. Values are terminal-sanitised; empties render
// unknown, never blank.
func TestLedgerNoticeGoldens(t *testing.T) {
	t.Parallel()
	minute := time.Minute
	tests := []struct {
		name string
		view ledgerView
		want []string
	}{
		{
			name: "typed rows with an unknown marker",
			view: ledgerView{
				usage: []journal.UsageRow{
					{Scope: "fixture-model", Unit: "tokens", Source: "native-reported",
						Label: "reported", Quantity: "23"},
					{Scope: "retail-equivalent", Unit: "USD", Source: "native-reported",
						Label: "estimated", Quantity: "0.37"},
					{Scope: "retail-equivalent", Label: "unknown", Quantity: "unknown"},
				},
				checks: 2, verifySum: 2 * minute, verifySumKnown: true, repairs: 3,
			},
			want: []string{
				"usage: fixture-model 23 tokens (reported, native-reported)",
				"usage: retail-equivalent 0.37 USD (estimated, native-reported)",
				"usage: retail-equivalent unknown unknown (unknown, unknown)",
				"reserve: verify pass (2 checks, 2m0s) + 3 repairs (estimate, not a reserve of provider quota)",
				"remaining: unknown",
				"next retry: none (used 0 of 3)",
			},
		},
		{
			name: "no rows and a waived suite",
			view: ledgerView{checks: 0, verifySumKnown: true, repairs: 3},
			want: []string{
				"usage: none recorded",
				"reserve: verify pass (0 checks, 0s) + 3 repairs (estimate, not a reserve of provider quota)",
				"remaining: unknown",
				"next retry: none (used 0 of 3)",
			},
		},
		{
			name: "exhausted bucket with unknown reset",
			view: ledgerView{checks: 1, verifySum: 5 * time.Second, verifySumKnown: true, repairs: 3,
				exhausted: true, retryAt: "unknown", retriesUsed: 2},
			want: []string{
				"usage: none recorded",
				"reserve: verify pass (1 checks, 5s) + 3 repairs (estimate, not a reserve of provider quota)",
				"remaining: unknown",
				"next retry: unknown (used 2 of 3)",
			},
		},
		{
			name: "authoritative reset renders its time",
			view: ledgerView{checks: 0, verifySumKnown: true, repairs: 3,
				exhausted: true, retryAt: "2026-10-09T12:00:00Z", retriesUsed: 0},
			want: []string{
				"usage: none recorded",
				"reserve: verify pass (0 checks, 0s) + 3 repairs (estimate, not a reserve of provider quota)",
				"remaining: unknown",
				"next retry: 2026-10-09T12:00:00Z (used 0 of 3)",
			},
		},
		{
			name: "unrepresentable verify sum stays unknown",
			view: ledgerView{checks: 2, verifySumKnown: false, repairs: 3},
			want: []string{
				"usage: none recorded",
				"reserve: verify pass (2 checks, unknown) + 3 repairs (estimate, not a reserve of provider quota)",
				"remaining: unknown",
				"next retry: none (used 0 of 3)",
			},
		},
		{
			name: "control bytes cannot forge lines",
			view: ledgerView{
				usage: []journal.UsageRow{
					{Scope: "evil\nnext retry: never", Unit: "tokens", Source: "native-reported",
						Label: "reported", Quantity: "1"},
				},
				checks: 0, verifySumKnown: true, repairs: 3,
			},
			want: []string{
				`usage: evil\nnext retry: never 1 tokens (reported, native-reported)`,
				"reserve: verify pass (0 checks, 0s) + 3 repairs (estimate, not a reserve of provider quota)",
				"remaining: unknown",
				"next retry: none (used 0 of 3)",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.view.lines()
			if len(got) != len(tt.want) {
				t.Fatalf("lines = %q, want %q", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("line %d = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestLedgerViewForGathersRunLedger pins the gather path behind the notice
// lines: usage rows, the envelope repair ceiling, the admitted checks and
// the bucket schedule read from the journal and the decision.
func TestLedgerViewForGathersRunLedger(t *testing.T) {
	t.Parallel()
	openWorld := func(t *testing.T) (*journal.Journal, string, string) {
		t.Helper()
		dir := t.TempDir()
		j, err := journal.Open(t.Context(), dir)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = j.Close() })
		runID := ids.New("run")
		prod := supervisor.NewProducer(ids.New("sup"), 1)
		run := journal.RunRow{RunID: runID, AdapterID: "builtin/fake", SourceRepo: "/tmp/repo",
			TaskSHA256: "t", BillingPosture: "local-scripted", ExecutionProfile: "trusted-host"}
		if err := supervisor.CreateRun(t.Context(), j, run, prod); err != nil {
			t.Fatal(err)
		}
		return j, dir, runID
	}
	t.Run("usage rows and envelope ceiling", func(t *testing.T) {
		t.Parallel()
		j, dir, runID := openWorld(t)
		at := time.Now().UTC().Format(time.RFC3339Nano)
		if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
			if err := journal.InsertUsageObservation(t.Context(), tx, journal.UsageRow{
				ObservationID: ids.New("obs"), RunID: runID, Scope: "m", Unit: "tokens",
				Source: "native-reported", Label: "reported", Quantity: "4",
				ProducerID: "wrk_a", ProducerSequence: 1, ObservedAt: at,
			}); err != nil {
				return err
			}
			return journal.UpsertRunEnvelope(t.Context(), tx, journal.EnvelopeRow{RunID: runID,
				ExecutionSeconds: 1800, Repairs: 5, Replans: 2, TransportRetries: 5, UpdatedAt: at})
		}); err != nil {
			t.Fatal(err)
		}
		d := admission.Decision{StateDir: dir, ProjectConfig: admission.ProjectConfig{
			Checks: []admission.CheckConfig{{Name: "t", Argv: []string{"true"}, Timeout: "5s"}},
		}}
		view, err := ledgerViewFor(t.Context(), d, runID)
		if err != nil {
			t.Fatal(err)
		}
		if len(view.usage) != 1 || view.usage[0].Quantity != "4" {
			t.Errorf("usage = %+v, want the one row", view.usage)
		}
		if view.checks != 1 || !view.verifySumKnown || view.verifySum != 5*time.Second {
			t.Errorf("verify pass = %d/%s/%t, want 1 check over 5s",
				view.checks, view.verifySum, view.verifySumKnown)
		}
		if view.repairs != 5 {
			t.Errorf("repairs = %d, want 5 from the envelope row", view.repairs)
		}
		if view.exhausted {
			t.Errorf("exhausted = true, want false with no bucket row")
		}
	})
	t.Run("exhausted bucket renders its schedule", func(t *testing.T) {
		t.Parallel()
		j, dir, runID := openWorld(t)
		bucket := `["fake","surface","class","unknown"]`
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if err := j.Transact(t.Context(), func(tx *sql.Tx) error {
			if err := journal.InsertReservation(t.Context(), tx, journal.ReservationRow{
				ReservationID: ids.New("rsv"), RunID: runID, Bucket: bucket, Scope: bucket,
				Owner: runID, Quantity: "unknown", Status: "released",
				ExpiresAt: now, CreatedAt: now,
			}); err != nil {
				return err
			}
			if err := journal.SetBucketExhausted(t.Context(), tx, bucket, now, nil); err != nil {
				return err
			}
			return journal.NoteBucketRetry(t.Context(), tx, bucket)
		}); err != nil {
			t.Fatal(err)
		}
		view, err := ledgerViewFor(t.Context(), admission.Decision{StateDir: dir}, runID)
		if err != nil {
			t.Fatal(err)
		}
		if !view.exhausted || view.retryAt != "unknown" || view.retriesUsed != 1 {
			t.Errorf("retry = %t/%q/%d, want exhausted with unknown reset and 1 used",
				view.exhausted, view.retryAt, view.retriesUsed)
		}
	})
	t.Run("missing rows fall back without inventing", func(t *testing.T) {
		t.Parallel()
		_, dir, runID := openWorld(t)
		view, err := ledgerViewFor(t.Context(), admission.Decision{StateDir: dir}, runID)
		if err != nil {
			t.Fatal(err)
		}
		if len(view.usage) != 0 || view.exhausted || view.retryAt != "" {
			t.Errorf("view = %+v, want empty usage and no schedule", view)
		}
		if view.repairs != 3 {
			t.Errorf("repairs = %d, want the 3 built-in repairs with no envelope row", view.repairs)
		}
	})
	t.Run("overflowing timeout sum stays unknown", func(t *testing.T) {
		t.Parallel()
		_, dir, runID := openWorld(t)
		d := admission.Decision{StateDir: dir, ProjectConfig: admission.ProjectConfig{
			Checks: []admission.CheckConfig{
				{Name: "a", Argv: []string{"true"}, Timeout: "2000000h"},
				{Name: "b", Argv: []string{"true"}, Timeout: "2000000h"},
			},
		}}
		view, err := ledgerViewFor(t.Context(), d, runID)
		if err != nil {
			t.Fatal(err)
		}
		if view.verifySumKnown {
			t.Errorf("verify sum known = %s, want unknown: the sum overflows", view.verifySum)
		}
		if view.checks != 2 {
			t.Errorf("checks = %d, want 2", view.checks)
		}
	})
}

// ledgerRunFixture is a user repository, a task file and an empty state
// directory for an in-process run.
func ledgerRunFixture(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	state := t.TempDir()
	home := t.TempDir()
	task := filepath.Join(t.TempDir(), "task.md")
	if err := os.WriteFile(task, []byte("# Demo task\n\nWrite demo.txt.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(), "HOME="+home, "XDG_CONFIG_HOME="+home, "GIT_CONFIG_NOSYSTEM=1",
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	git("init", "--quiet", "--initial-branch=main")
	git("config", "user.name", "Test")
	git("config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("demo\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git("add", "README.md")
	git("commit", "--quiet", "-m", "initial")
	t.Setenv("MYTHHELM_HOME", state)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Chdir(repo)
	return task
}

// TestCliUsageNotices pins the run's ledger output: a run emits the usage,
// reserve, remaining and next retry lines, a never-exhausted run renders
// next retry none, and no line carries hard-spending phrasing.
func TestCliUsageNotices(t *testing.T) {
	task := ledgerRunFixture(t)
	code, stdout, stderr := runMain("run", "--task-file", task, "--adapter", "fake",
		"--billing", "local-scripted", "--execution-profile", "trusted-host",
		"--non-interactive", "--no-checks", "--plain")
	if code != 5 {
		t.Fatalf("run exit %d, stderr %q; want the unverified 5", code, stderr)
	}
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	var got []string
	for _, line := range lines {
		for _, prefix := range []string{"usage:", "reserve:", "remaining:", "next retry:"} {
			if strings.HasPrefix(line, prefix) {
				got = append(got, line)
			}
		}
	}
	// The fake native reports no usage, so the run carries the single
	// unknown marker for its always-expected retail scope (AC-2.2).
	want := []string{
		"usage: retail-equivalent unknown unknown (unknown, unknown)",
		"reserve: verify pass (0 checks, 0s) + 3 repairs (estimate, not a reserve of provider quota)",
		"remaining: unknown",
		"next retry: none (used 0 of 3)",
	}
	if len(got) != len(want) {
		t.Fatalf("ledger lines = %q, want %q (full output:\n%s)", got, want, stdout)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("ledger line %d = %q, want %q", i, got[i], want[i])
		}
	}
	for _, phrase := range []string{"limit", "cap of", "balance"} {
		for _, line := range got {
			if strings.Contains(strings.ToLower(line), phrase) {
				t.Errorf("ledger line carries hard-spending phrasing %q: %q", phrase, line)
			}
		}
	}
	result := -1
	for i, line := range lines {
		if strings.HasPrefix(line, "result:") {
			result = i
		}
	}
	last := -1
	for i, line := range lines {
		if line == want[len(want)-1] {
			last = i
		}
	}
	if result < 0 || last < 0 || last > result {
		t.Errorf("ledger lines end at %d with result at %d, want the lines before run.result", last, result)
	}
}
