package service

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
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

const observerCloseEntered = "T6 observer callback entered before close"

// exec serializes writes when stdout and stderr share this writer. The parent
// reads the buffer only after Wait has joined both output copies.
type observerCloseOutput struct {
	buf     bytes.Buffer
	tail    []byte
	entered chan struct{}
	once    sync.Once
}

func (w *observerCloseOutput) Write(p []byte) (int, error) {
	n, err := w.buf.Write(p)
	marker := []byte("\n" + observerCloseEntered + "\n")
	w.tail = append(w.tail, p...)
	if bytes.Contains(w.tail, marker) {
		w.once.Do(func() { close(w.entered) })
	}
	if len(w.tail) >= len(marker) {
		w.tail = w.tail[len(w.tail)-len(marker)+1:]
	}
	return n, err
}

func runObserverCloseProcess(cmd *exec.Cmd, setupBudget, closeBudget time.Duration) (string, bool, error) {
	out := &observerCloseOutput{entered: make(chan struct{})}
	cmd.Stdout, cmd.Stderr = out, out
	if err := cmd.Start(); err != nil {
		return "", false, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	setup := time.NewTimer(setupBudget)
	defer setup.Stop()
	select {
	case <-out.entered:
	case err := <-done:
		// Wait may win the select after a fast callback and child exit.
		select {
		case <-out.entered:
			return out.buf.String(), false, err
		default:
			return out.buf.String(), false, fmt.Errorf("child exited before close callback (exit=%v)", err)
		}
	case <-setup.C:
		_ = cmd.Process.Kill()
		err := <-done
		return out.buf.String(), false, fmt.Errorf("setup deadline before close callback: %w", err)
	}
	// Setup, including SQLite and entry admission, does not consume this budget.
	action := time.NewTimer(closeBudget)
	defer action.Stop()
	select {
	case err := <-done:
		return out.buf.String(), false, err
	case <-action.C:
		_ = cmd.Process.Kill()
		err := <-done
		return out.buf.String(), true, err
	}
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
			cmd := exec.CommandContext(t.Context(), exe, "-test.run=^TestObserverCloseOrdering$", "-test.v", "-test.timeout=30s")
			cmd.Env = []string{"MYTHHELM_T6_CLOSE_FIXTURE=" + c.fixture, "MYTHHELM_T6_FIXTURE_PARENT=" + t.TempDir(), "SystemRoot=" + os.Getenv("SystemRoot"), "USERNAME=" + os.Getenv("USERNAME"), "TEMP=" + os.TempDir(), "TMP=" + os.TempDir()}
			out, blocked, err := runObserverCloseProcess(cmd, 20*time.Second, 5*time.Second)
			if blocked != c.blocked {
				t.Fatalf("callback close blocked=%v want%v: %v %s", blocked, c.blocked, err, out)
			}
			if !blocked && err != nil {
				t.Fatalf("owned callback fixture failed: %v %s", err, out)
			}
		})
	}
}

func TestObserverCloseProcessPhases(t *testing.T) {
	if mode := os.Getenv("MYTHHELM_T6_PHASE_FIXTURE"); mode != "" {
		if mode == "missing" {
			os.Exit(0)
		}
		var gate [1]byte
		if _, err := io.ReadFull(os.Stdin, gate[:]); err != nil {
			t.Fatal(err)
		}
		// Separate writes also exercise marker fragments crossing output reads.
		_, _ = fmt.Fprint(os.Stdout, "\n"+observerCloseEntered[:12])
		_, _ = fmt.Fprintln(os.Stdout, observerCloseEntered[12:])
		os.Exit(7)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"gated", "missing", "setup timeout"} {
		t.Run(mode, func(t *testing.T) {
			cmd := exec.CommandContext(t.Context(), exe, "-test.run=^TestObserverCloseProcessPhases$", "-test.timeout=10s")
			cmd.Env = append(os.Environ(), "MYTHHELM_T6_PHASE_FIXTURE="+mode)
			gate, err := cmd.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			defer gate.Close()
			released := make(chan struct{})
			if mode == "gated" {
				go func() {
					defer close(released)
					// Hold setup longer than the entire action budget without changing it.
					timer := time.NewTimer(1100 * time.Millisecond)
					defer timer.Stop()
					select {
					case <-timer.C:
						_, _ = gate.Write([]byte{1})
					case <-t.Context().Done():
					}
				}()
			} else {
				close(released)
			}
			setupBudget := 5 * time.Second
			if mode == "setup timeout" {
				setupBudget = 100 * time.Millisecond
			}
			out, blocked, err := runObserverCloseProcess(cmd, setupBudget, time.Second)
			<-released
			if blocked {
				t.Fatalf("setup consumed close budget: %v %s", err, out)
			}
			switch mode {
			case "gated":
				var exit *exec.ExitError
				if !errors.As(err, &exit) || exit.ExitCode() != 7 || !bytes.Contains([]byte(out), []byte(observerCloseEntered)) {
					t.Fatalf("callback output/exit lost: %v %s", err, out)
				}
			case "missing":
				if err == nil || !bytes.Contains([]byte(err.Error()), []byte("exited before close callback")) {
					t.Fatalf("missing marker accepted: %v %s", err, out)
				}
			case "setup timeout":
				if err == nil || !bytes.Contains([]byte(err.Error()), []byte("setup deadline before close callback")) {
					t.Fatalf("setup failure misclassified: %v %s", err, out)
				}
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
	callbackEntered := make(chan struct{})
	callbackReturned := make(chan struct{})
	var once, returnedOnce sync.Once
	releaseIngest := make(chan struct{})
	seedVerifying(t, r, "run_callback")
	a, err := r.startEntry(t.Context(), owner, func(ctx context.Context, h supervisor.Hooks) error {
		h.Event = func(journal.Event) {
			// Only the first actual callback begins the close-return window.
			once.Do(func() {
				_, _ = fmt.Fprintln(os.Stdout, observerCloseEntered)
				close(callbackEntered)
			})
			if synchronous {
				_ = owner.Close()
			} else {
				h.Observer.RequestClose()
			}
			returnedOnce.Do(func() { close(callbackReturned) })
			<-releaseIngest
		}
		_, err := supervisor.RecoverOwned(ctx, r.journal, owner, "run_callback", h)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	setup := time.NewTimer(20 * time.Second)
	defer setup.Stop()
	select {
	case <-callbackEntered:
	case <-a.done:
		t.Fatal("owned entry returned before close callback")
	case <-setup.C:
		// The subprocess timeout remains the final bound if entry cleanup stalls.
		close(releaseIngest)
		<-a.done
		t.Fatal("setup deadline before close callback")
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
