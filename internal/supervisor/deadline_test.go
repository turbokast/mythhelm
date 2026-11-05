package supervisor_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/workers"
)

func TestDeadlineRecoveryConclusion(t *testing.T) {
	t.Parallel()
	for _, first := range []string{workers.DeadlineRequester, "user-first"} {
		t.Run(first, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			// Both branches recover a native that actually launched. Allow worker
			// startup and verification within the envelope, independently of the
			// 30-second setup bound. The execution branch still waits for the
			// real worker timer; it does not manufacture a deadline stop request.
			execution := time.Minute
			setupCtx, cancelSetup := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancelSetup()
			d, _, wait := seedLaunchTimed(t, f, "slow", pinnedPayload, execution)
			dir := workers.AttemptDir(f.state, d.RunID, d.AttemptID)
			t.Cleanup(func() {
				if t.Failed() {
					f.logWorkerDiagnostics(t)
				}
			})
			waitDeadlineNativeLaunch(setupCtx, t, dir, d.RunID, d.AttemptID)
			if first != workers.DeadlineRequester {
				if err := workers.RequestStop(dir, first); err != nil {
					t.Fatal(err)
				}
				if err := workers.RequestStop(dir, workers.DeadlineRequester); err != nil {
					t.Fatal(err)
				}
			}
			wait() // There is no owner to remember the reason; recovery must ingest it.
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			out, err := supervisor.RecoverWithHooks(ctx, f.journal(t), d.RunID, supervisor.Hooks{})
			if err != nil {
				t.Fatal(err)
			}
			state, reason := supervisor.RunCancelled, ""
			if first == workers.DeadlineRequester {
				state, reason = supervisor.RunBlocked, workers.DeadlineReason
			}
			if out.State != state || out.Reason != reason || out.AttemptState != supervisor.AttemptStopped {
				t.Fatalf("recovered %s: %+v, want %s/%s stopped", first, out, state, reason)
			}
			launches := 0
			for _, ev := range f.events(t, d.RunID) {
				if ev.Type == "attempt.launched" {
					launches++
				}
			}
			if launches != 1 {
				t.Fatalf("recovery launched/replayed %d natives", launches)
			}
		})
	}
}

// Worker identity precedes Start, including Windows ResumeThread. Only the
// critical launched spool event follows successful Start (workers/worker.go).
func waitDeadlineNativeLaunch(ctx context.Context, t *testing.T, dir, runID, attemptID string) {
	t.Helper()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			t.Fatalf("native launch setup expired: %v", err)
		}
		launched, err := deadlineNativeLaunched(filepath.Join(dir, supervisor.SpoolFile), runID, attemptID)
		if err != nil {
			t.Fatal(err)
		}
		if launched {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("native launch was not recorded before the setup deadline: %v", ctx.Err())
		case <-ticker.C:
		}
	}
}

func TestDeadlineNativeLaunchObservation(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	d, _, wait := seedLaunchTimed(t, f, "slow", pinnedPayload, time.Minute)
	dir := workers.AttemptDir(f.state, d.RunID, d.AttemptID)
	waitDeadlineNativeLaunch(ctx, t, dir, d.RunID, d.AttemptID)
	path := filepath.Join(dir, supervisor.SpoolFile)
	for _, scope := range []struct{ runID, attemptID string }{
		{"foreign-run", d.AttemptID}, {d.RunID, "foreign-attempt"},
	} {
		found, err := deadlineNativeLaunched(path, scope.runID, scope.attemptID)
		if err != nil || found {
			t.Fatalf("foreign-scope launch observation = %v, %v", found, err)
		}
	}
	file, err := os.Open(path) // #nosec G304 -- the actual fixture worker spool.
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(io.LimitReader(file, 64<<10))
	_ = file.Close()
	if err != nil {
		t.Fatal(err)
	}
	var launched []byte
	for line := range bytes.SplitSeq(data, []byte{'\n'}) {
		var event journal.Event
		if json.Unmarshal(line, &event) == nil && event.Type == "attempt.launched" {
			launched = line
			break
		}
	}
	if len(launched) == 0 {
		t.Fatal("actual worker launch event missing")
	}
	copyPath := filepath.Join(t.TempDir(), "launch-copy.jsonl")
	for _, c := range []struct {
		name string
		data []byte
		want bool
	}{
		{"incomplete line", launched, false},
		{"complete actual event", append(bytes.Clone(launched), '\n'), true},
		{"outside scan bound", append(bytes.Repeat([]byte{' '}, 64<<10), append([]byte{'\n'}, launched...)...), false},
	} {
		t.Run(c.name, func(t *testing.T) {
			if err := os.WriteFile(copyPath, c.data, 0o600); err != nil {
				t.Fatal(err)
			}
			found, err := deadlineNativeLaunched(copyPath, d.RunID, d.AttemptID)
			if err != nil || found != c.want {
				t.Fatalf("launch observation = %v, %v; want %v", found, err, c.want)
			}
		})
	}
	if err := workers.RequestStop(dir, "observation-test-cleanup"); err != nil {
		t.Fatal(err)
	}
	wait()
}

func deadlineNativeLaunched(path, runID, attemptID string) (bool, error) {
	file, err := os.Open(path) // #nosec G304 -- the current fixture's attempt spool.
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer func() { _ = file.Close() }()
	// Launch is near the start of this fixture's spool. Bound every scan and
	// ignore its incomplete trailing line while the worker appends it.
	reader := bufio.NewReader(io.LimitReader(file, 64<<10))
	for {
		line, err := reader.ReadBytes('\n')
		if errors.Is(err, io.EOF) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		var event journal.Event
		if err := json.Unmarshal(line, &event); err != nil {
			return false, err
		}
		if event.SchemaVersion == journal.EnvelopeVersion && event.Type == "attempt.launched" && event.RunID == runID && event.AttemptID == attemptID {
			return true, nil
		}
	}
}
