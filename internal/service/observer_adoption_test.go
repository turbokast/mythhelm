package service

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/control"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/workers"
)

func awaitAdoptionCondition(t *testing.T, fn func() bool) {
	t.Helper()
	until := time.Now().Add(10 * time.Second)
	for !fn() {
		if time.Now().After(until) {
			t.Fatal("adoption progress barrier not reached")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestAdoptionWithStalledObserver(t *testing.T) {
	f := newAdoptionFixture(t, "paced")
	a, err := f.r.reconcileRun(t.Context(), f.run)
	if err != nil {
		t.Fatal(err)
	}
	// Transfer the real actor sink's sole reader, leaving its delivery endpoint
	// open and deliberately unconsumed. No fake success predicate is installed.
	if err := a.adapter.reader.Close(); err != nil {
		t.Fatal(err)
	}
	<-a.adapter.done
	reader, err := a.adapter.sink.OpenReader()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		a.cancel()
		timeout := time.NewTimer(5 * time.Second)
		defer timeout.Stop()
		for {
			select {
			case <-a.done:
				_ = reader.Close()
				return
			case <-reader.Wakeups():
			case <-timeout.C:
				t.Error("stalled-observer entry did not join during cleanup")
				return
			}
		}
	})
	var initial int64
	awaitAdoptionCondition(t, func() bool {
		row, err := f.r.journal.Attempt(t.Context(), f.attempt)
		if err == nil && row.SpoolOffset > 0 {
			initial = row.SpoolOffset
			return true
		}
		select {
		case <-a.done:
			t.Fatalf("recovery exited: %v", a.err)
		default:
		}
		return false
	})
	awaitAdoptionCondition(t, func() bool {
		row, err := f.r.journal.Attempt(t.Context(), f.attempt)
		return err == nil && row.SpoolOffset > initial
	})
	adopted := 0
	progress := 0
	awaitAdoptionCondition(t, func() bool {
		events, err := f.r.journal.Events(t.Context(), f.run, 0)
		if err != nil {
			t.Fatal(err)
		}
		adopted, progress = 0, 0
		for _, e := range events {
			if e.Type == "service.adopted" {
				adopted++
			}
			if e.Type == "attempt.progress" {
				progress++
			}
		}
		return progress > 0
	})
	id, err := workers.ReadIdentity(workers.AttemptDir(f.r.journal.StateDir(), f.run, f.attempt))
	if err != nil || id.PID != f.proc.Pid || adopted != 1 || progress == 0 {
		t.Fatalf("identity/progress: pid%d want%d adoption%d progress%d %v", id.PID, f.proc.Pid, adopted, progress, err)
	}
	a.cancel()
	select {
	case <-a.done:
	case <-time.After(5 * time.Second):
		t.Fatal("entry did not join")
	}
}

func TestAdoptionCancellationKeepsOwnerUntilIngestionExit(t *testing.T) {
	f := newAdoptionFixture(t)
	ctx := t.Context()
	ready := make(chan struct{})
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	var releaseOnce sync.Once
	releaseCallback := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseCallback)
	var a *actor
	var err error
	a, err = f.r.reconcileRunObserved(ctx, f.run, func(journal.Event) {
		<-ready
		once.Do(func() { close(entered); a.adapter.sink.RequestClose(); a.adapter.sink.RequestClose(); <-release })
	})
	if err != nil {
		t.Fatal(err)
	}
	close(ready)
	select {
	case <-entered:
	case <-a.done:
		t.Fatalf("recovery exited before callback: %v", a.err)
	case <-time.After(5 * time.Second):
		t.Fatal("no real recovery callback")
	}
	view, err := f.r.views.open(ctx, "adoption-view")
	if err != nil {
		t.Fatal(err)
	}
	view.close()
	view.close()
	timeout, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	err = f.r.shutdown(timeout)
	cancel()
	if !errors.Is(err, ErrCleanupIncomplete) {
		t.Fatalf("shutdown acknowledged cleanup while callback remains active: %v", err)
	}
	runDir := filepath.Join(f.r.journal.StateDir(), "runs", f.run)
	if releaseOwner, err := supervisor.AcquireOwner(runDir); !errors.Is(err, supervisor.ErrOwnerHeld) {
		if err == nil {
			releaseOwner()
		}
		t.Fatalf("owner released at active callback: %v", err)
	}
	if replacement, err := control.AcquireInstanceLease(f.r.journal.StateDir()); err == nil {
		_ = replacement.Close()
		t.Fatal("root lease released before actor joined")
	}
	// A changed-generation presence file is not owner authority or early release.
	if err := control.WriteSupervisorBeat(workers.AttemptDir(f.r.journal.StateDir(), f.run, f.attempt), f.r.lease.Generation()+1, 999, time.Now()); err != nil {
		t.Fatal(err)
	}
	id, err := workers.ReadIdentity(workers.AttemptDir(f.r.journal.StateDir(), f.run, f.attempt))
	if err != nil || id.PID != f.proc.Pid {
		t.Fatalf("late presence changed worker: %+v %v", id, err)
	}
	a.adapter.sink.Committed(f.run, 999)
	releaseCallback()
	joined, cancelJoin := context.WithTimeout(ctx, 5*time.Second)
	defer cancelJoin()
	if err := f.r.shutdown(joined); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if releaseOwner, err := supervisor.AcquireOwner(runDir); err != nil {
		t.Fatalf("owner not released after join: %v", err)
	} else {
		releaseOwner()
	}
}
