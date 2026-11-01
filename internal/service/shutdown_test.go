package service

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/control"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/supervisor"
)

func TestServiceShutdownWithBlockedEntry(t *testing.T) {
	r := testRuntime(t)
	owner, err := supervisor.AcquireServiceOwner(t.Context(), r.journal, r.lease, "run_blocked")
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	var once sync.Once
	release := make(chan struct{})
	seedVerifying(t, r, "run_blocked")
	a, err := r.startEntry(t.Context(), owner, func(ctx context.Context, h supervisor.Hooks) error {
		h.Event = func(journal.Event) { once.Do(func() { close(entered) }); <-release }
		_, err := supervisor.RecoverOwned(ctx, r.journal, owner, "run_blocked", h)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-a.done:
		close(release)
		t.Fatal("owned entry returned before blocked callback")
	case <-time.After(2 * time.Second):
		close(release)
		<-a.done
		t.Fatal("owned entry never reached blocked callback")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err = r.shutdown(ctx)
	if !errors.Is(err, ErrCleanupIncomplete) {
		close(release)
		<-a.done
		t.Fatalf("blocked shutdown=%v", err)
	}
	if _, err := r.startEntry(t.Context(), nil, func(context.Context, supervisor.Hooks) error { return nil }); !errors.Is(err, &control.Error{Code: control.CodeCapabilityUnsupported}) {
		close(release)
		<-a.done
		t.Fatalf("shutdown admitted new entry: %v", err)
	}
	if r.lease.Generation() == 0 {
		close(release)
		<-a.done
		t.Fatal("timeout released lease")
	}
	unlock, err := supervisor.AcquireOwner(filepath.Join(r.lease.Root(), "runs", "run_blocked"))
	if err == nil {
		unlock()
		close(release)
		<-a.done
		t.Fatal("timeout released run owner")
	}
	if !errors.Is(err, supervisor.ErrOwnerHeld) {
		close(release)
		<-a.done
		t.Fatal(err)
	}
	close(release)
	if err := r.shutdown(t.Context()); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if r.lease.Generation() != 0 {
		t.Fatal("joined shutdown retained lease")
	}
	unlock, err = supervisor.AcquireOwner(filepath.Join(r.journal.StateDir(), "runs", "run_blocked"))
	if err != nil {
		t.Fatalf("joined shutdown did not free owner: %v", err)
	}
	unlock()
}

func TestObserverCloseOrdering(t *testing.T) {
	if fixture := os.Getenv("MYTHHELM_T6_CLOSE_FIXTURE"); fixture != "" {
		observerCloseFixture(t, fixture == "sync")
		return
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, fixture string
		blocked       bool
	}{{"request close returns", "request", false}, {"synchronous close cannot return", "sync", true}} {
		t.Run(c.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, exe, "-test.run=^TestObserverCloseOrdering$", "-test.v")
			cmd.Env = []string{"MYTHHELM_T6_CLOSE_FIXTURE=" + c.fixture, "MYTHHELM_T6_FIXTURE_PARENT=" + t.TempDir(), "SystemRoot=" + os.Getenv("SystemRoot"), "USERNAME=" + os.Getenv("USERNAME"), "TEMP=" + os.TempDir(), "TMP=" + os.TempDir()}
			var out bytes.Buffer
			cmd.Stdout = &out
			cmd.Stderr = &out
			err := cmd.Run()
			blocked := errors.Is(ctx.Err(), context.DeadlineExceeded)
			if blocked != c.blocked {
				t.Fatalf("callback close blocked=%v want%v: %v %s", blocked, c.blocked, err, out.String())
			}
			if !blocked && err != nil {
				t.Fatalf("owned callback fixture failed: %v %s", err, out.String())
			}
		})
	}
}

func observerCloseFixture(t *testing.T, synchronous bool) {
	r := testRuntime(t)
	owner, err := supervisor.AcquireServiceOwner(t.Context(), r.journal, r.lease, "run_callback")
	if err != nil {
		t.Fatal(err)
	}
	callbackReturned := make(chan struct{})
	var once sync.Once
	releaseIngest := make(chan struct{})
	seedVerifying(t, r, "run_callback")
	a, err := r.startEntry(t.Context(), owner, func(ctx context.Context, h supervisor.Hooks) error {
		h.Event = func(journal.Event) {
			if synchronous {
				_ = owner.Close()
			} else {
				h.Observer.RequestClose()
			}
			once.Do(func() { close(callbackReturned) })
			<-releaseIngest
		}
		_, err := supervisor.RecoverOwned(ctx, r.journal, owner, "run_callback", h)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-callbackReturned:
	case <-timer.C:
		close(releaseIngest)
		<-a.done
		t.Fatal("callback waited for owner close")
	}
	if r.lease.Generation() == 0 {
		close(releaseIngest)
		<-a.done
		t.Fatal("callback released instance")
	}
	unlock, err := supervisor.AcquireOwner(filepath.Join(r.lease.Root(), "runs", "run_callback"))
	if err == nil {
		unlock()
		close(releaseIngest)
		<-a.done
		t.Fatal("callback released run owner")
	}
	first, err := r.views.open(t.Context(), "first")
	if err != nil {
		close(releaseIngest)
		<-a.done
		t.Fatal(err)
	}
	second, err := r.views.open(t.Context(), "second")
	if err != nil {
		first.close()
		close(releaseIngest)
		<-a.done
		t.Fatal(err)
	}
	first.close()
	select {
	case <-second.done:
		close(releaseIngest)
		<-a.done
		t.Fatal("one view cancelled another")
	default:
	}
	second.close()
	close(releaseIngest)
	<-a.done
	if err := r.shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func seedVerifying(t *testing.T, r *runtime, runID string) {
	t.Helper()
	now := time.Now().UTC()
	if err := r.journal.Transact(t.Context(), func(tx *sql.Tx) error {
		if err := journal.InsertRun(t.Context(), tx, journal.RunRow{RunID: runID, State: "verifying", AdapterID: "fake", SourceRepo: "synthetic", TaskSHA256: "synthetic", BillingPosture: "local-scripted", ExecutionProfile: "trusted-host", CreatedAt: now, UpdatedAt: now}); err != nil {
			return err
		}
		if err := journal.InsertAttempt(t.Context(), tx, journal.AttemptRow{AttemptID: "att_" + runID, RunID: runID, TaskID: "task_synthetic", AttemptNumber: 1, State: "succeeded_native", LaunchTokenSHA256: "synthetic", WorkspacePath: "synthetic"}); err != nil {
			return err
		}
		return journal.InsertVerification(t.Context(), tx, journal.VerificationRow{ID: "ver_" + runID, RunID: runID, CandidateCommit: "synthetic", ConfigSHA256: "synthetic", Result: "failed", StartedAt: now, FinishedAt: now})
	}); err != nil {
		t.Fatal(err)
	}
}
