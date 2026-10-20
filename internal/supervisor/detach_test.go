package supervisor_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/billing"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/workers"
)

// The interrupt variant is a retained counterexample: the same live worker
// must stop when explicitly interrupted, but remain running after detach.
func TestDetachLeavesRunning(t *testing.T) {
	// In-process owner fixtures set MYTHHELM_RUNTIME_DIR with t.Setenv.
	for _, control := range []string{"detach", "interrupt"} {
		t.Run(control, func(t *testing.T) {
			f := newFixture(t)
			before := f.fingerprint(t)
			d := detachDecision(t, f, "slow")
			detach := make(chan struct{}, 1)
			interrupts := make(chan os.Signal, 1)
			sent := false
			var notices []string
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			out, err := f.runInProcessWith(ctx, t, d, supervisor.Hooks{Detach: detach, Interrupt: interrupts,
				Notice: func(s string) { notices = append(notices, s) },
				Event: func(ev journal.Event) {
					if !sent && ev.Type == "attempt.state_changed" && payloadOf(t, ev)["state"] == "running" {
						sent = true
						if control == "detach" {
							detach <- struct{}{}
						} else {
							interrupts <- os.Interrupt
						}
					}
				},
			})
			if control == "interrupt" {
				if err != nil || !sent || out.State != supervisor.RunCancelled || out.DetachedRunning {
					t.Fatalf("interrupt outcome %+v, sent=%t, err=%v", out, sent, err)
				}
				if _, err := f.journal(t).Candidate(t.Context(), d.AttemptID); err != nil {
					t.Fatal(err)
				}
				return
			}
			if !sent || !out.DetachedRunning || out.Detached || out.State != supervisor.RunExecuting || out.AttemptState != supervisor.AttemptRunning {
				t.Fatalf("detach outcome %+v, sent=%t, err=%v; want executing/running detached without stop", out, sent, err)
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(notices) == 0 || notices[len(notices)-1] != fmt.Sprintf(supervisor.RunningDetachedNotice, d.RunID, d.RunID) {
				t.Fatalf("notices %q", notices)
			}
			assertRunningDetached(t, f, d)
			if after := f.fingerprint(t); before != after {
				t.Fatal("detach modified source checkout")
			} // I08 (v2 §2)

			// Recover takes the released owner lock, ingests the same worker, and can
			// itself detach without freezing it or creating a replacement attempt.
			detach <- struct{}{}
			recovered, err := supervisor.RecoverWithHooks(ctx, f.journal(t), d.RunID, supervisor.Hooks{Detach: detach})
			if err != nil || !recovered.DetachedRunning || recovered.State != supervisor.RunExecuting || recovered.AttemptID != d.AttemptID {
				t.Fatalf("recover %+v: %v", recovered, err)
			}
			assertRunningDetached(t, f, d)
			counts := eventTypeCounts(t, f, d.RunID)
			if counts["attempt.launched"] != 1 || counts["attempt.launch_intent_recorded"] != 1 {
				t.Fatalf("launch counts after recovery %v", counts)
			} // I12/I18 (v2 §2)
		})
	}
}

func detachDecision(t *testing.T, f fixture, scenario string) admission.Decision {
	t.Helper()
	d, err := admission.Decide(t.Context(), admission.Request{StateDir: f.state, Repo: f.repo, TaskFile: f.task,
		Adapter: admission.AdapterFake, Billing: admission.BillingLocalScripted, ExecutionProfile: admission.ProfileTrustedHost,
		NoChecks: true, Scenario: scenario, Env: os.Environ(), EnvelopeFlags: &billing.Ceilings{Execution: 30 * time.Second}})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func assertRunningDetached(t *testing.T, f fixture, d admission.Decision) {
	t.Helper()
	dir := workers.AttemptDir(f.state, d.RunID, d.AttemptID)
	id, err := workers.ReadIdentity(dir)
	if err != nil {
		t.Fatal(err)
	}
	if workerGone(id) {
		t.Fatal("detached worker is gone")
	}
	if id.NativePID == nil {
		t.Fatal("detached worker lacks native identity")
	}
	if _, err := workers.ProcessStartTime(*id.NativePID); err != nil {
		t.Fatalf("detached native is gone: %v", err)
	} // I06/I18/I21 (v2 §2)
	if _, err := os.Stat(filepath.Join(dir, "stop.request")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stop request after detach: %v", err)
	}
	if _, err := f.journal(t).Candidate(t.Context(), d.AttemptID); !errors.Is(err, journal.ErrNotFound) {
		t.Fatalf("candidate before native stopped: %v", err)
	}
	row, err := f.journal(t).Run(t.Context(), d.RunID)
	if err != nil || row.State != string(supervisor.RunExecuting) {
		t.Fatalf("run projection %+v: %v", row, err)
	}
}

func TestDetachDuringStopping(t *testing.T) {
	// In-process supervision changes the runtime environment.
	f := newFixture(t)
	d := detachDecision(t, f, "ignore-term")
	interrupts := make(chan os.Signal, 1)
	detach := make(chan struct{}, 1)
	var notices []string
	sent := false
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	out, err := f.runInProcessWith(ctx, t, d, supervisor.Hooks{Interrupt: interrupts, Detach: detach,
		Event: func(ev journal.Event) {
			if !sent && ev.Type == "attempt.state_changed" && payloadOf(t, ev)["state"] == "running" {
				sent = true
				interrupts <- os.Interrupt
			}
		},
		Notice: func(s string) {
			notices = append(notices, s)
			if s == supervisor.StopRequestedNotice {
				detach <- struct{}{}
			}
		},
	})
	pending := out.Detached && !out.DetachedRunning && out.State == supervisor.RunStopping
	// The Windows fake uses one immediate Kill rung (fake.stopLadder), so
	// durable terminal evidence can win before the next owner ingest. Unix
	// ignore-term instead keeps the ladder pending and must prove detachment.
	terminal := runtime.GOOS == "windows" && !out.Detached && !out.DetachedRunning &&
		out.State == supervisor.RunCancelled && out.AttemptState == supervisor.AttemptStopped
	if err != nil || !sent || (!pending && !terminal) {
		t.Fatalf("pending stop or confirmed Windows terminal outcome %+v, err=%v", out, err)
	}
	if pending && notices[len(notices)-1] != fmt.Sprintf(supervisor.DetachedNotice, d.RunID) {
		t.Fatalf("notices %q", notices)
	}
	dir := workers.AttemptDir(f.state, d.RunID, d.AttemptID)
	request, err := os.ReadFile(filepath.Join(dir, "stop.request")) // #nosec G304 -- fixed control filename under the synthetic fixture attempt directory
	if err != nil {
		t.Fatal(err)
	}
	if pending {
		if _, err := f.journal(t).Candidate(t.Context(), d.AttemptID); !errors.Is(err, journal.ErrNotFound) {
			t.Fatalf("freeze before stop confirmed: %v", err)
		}
		// A recovered pending stop must detach without consuming another
		// queued interrupt (which could otherwise request a duplicate stop).
		extraInterrupt := make(chan os.Signal, 1)
		extraInterrupt <- os.Interrupt
		detach <- struct{}{}
		detachedAgain, err := supervisor.RecoverWithHooks(ctx, f.journal(t), d.RunID, supervisor.Hooks{Detach: detach, Interrupt: extraInterrupt})
		pendingAgain := detachedAgain.Detached && detachedAgain.State == supervisor.RunStopping
		finished := detachedAgain.State == supervisor.RunCancelled && detachedAgain.AttemptState == supervisor.AttemptStopped
		if err != nil || detachedAgain.DetachedRunning || len(extraInterrupt) != 1 || (!pendingAgain && !finished) {
			t.Fatalf("recovered pending detach %+v, queued interrupts=%d: %v", detachedAgain, len(extraInterrupt), err)
		}
		// Join the existing stop without sending another control request.
		recovered, err := supervisor.RecoverWithHooks(ctx, f.journal(t), d.RunID, supervisor.Hooks{})
		if err != nil || recovered.State != supervisor.RunCancelled || recovered.AttemptState != supervisor.AttemptStopped {
			t.Fatalf("recover %+v: %v", recovered, err)
		}
	}
	// Both paths must finish with a durable confirmed stop and candidate.
	attempt, err := f.journal(t).Attempt(t.Context(), d.AttemptID)
	if err != nil || attempt.State != string(supervisor.AttemptStopped) {
		t.Fatalf("durable attempt %+v: %v", attempt, err)
	}
	if _, err := f.journal(t).Candidate(t.Context(), d.AttemptID); err != nil {
		t.Fatal(err)
	}
	if n := eventTypeCounts(t, f, d.RunID)["attempt.launched"]; n != 1 {
		t.Fatalf("native launches %d, want 1", n)
	}
	after, err := os.ReadFile(filepath.Join(dir, "stop.request")) // #nosec G304 -- fixed control filename under the synthetic fixture attempt directory
	if err != nil || string(after) != string(request) {
		t.Fatalf("stop request changed: %q -> %q (%v)", request, after, err)
	}
	if n := eventTypeCounts(t, f, d.RunID)["attempt.stop_requested"]; n != 1 {
		t.Fatalf("stop requests %d, want 1", n)
	}
}

func TestDetachBeforeAcceptance(t *testing.T) {
	// In-process supervision changes the runtime environment.
	for _, finishFirst := range []bool{false, true} {
		t.Run(fmt.Sprintf("terminal_first_%t", finishFirst), func(t *testing.T) {
			f := newFixture(t)
			d := detachDecision(t, f, "slow")
			detach := make(chan struct{}, 1)
			interrupts := make(chan os.Signal, 1)
			queued, running := false, false
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			out, err := f.runInProcessWith(ctx, t, d, supervisor.Hooks{Detach: detach, Interrupt: interrupts, Event: func(ev journal.Event) {
				if !queued && ev.Type == "run.created" {
					queued = true
					detach <- struct{}{}
					if finishFirst {
						interrupts <- os.Interrupt
					}
				}
				if ev.Type == "attempt.state_changed" && payloadOf(t, ev)["state"] == "running" {
					running = true
				}
			}})
			if !queued {
				t.Fatal("did not reach pre-admission queue")
			}
			if finishFirst {
				if err != nil || out.State != supervisor.RunCancelled || out.Detached || out.DetachedRunning || running {
					t.Fatalf("prior terminal outcome %+v, running=%t err=%v", out, running, err)
				}
				if n := eventTypeCounts(t, f, d.RunID)["attempt.launched"]; n != 0 {
					t.Fatalf("prior cancellation launched %d natives", n)
				}
			} else {
				if err != nil || !running || !out.DetachedRunning || out.AttemptState != supervisor.AttemptRunning {
					t.Fatalf("early detach %+v, running=%t err=%v", out, running, err)
				}
				assertRunningDetached(t, f, d)
			}
		})
	}
}

func TestDetachTerminalEvidenceWins(t *testing.T) {
	// In-process supervision changes the runtime environment.
	f := newFixture(t)
	d := detachDecision(t, f, "happy")
	detach := make(chan struct{}, 1)
	queued := false
	out, err := f.runInProcessWith(t.Context(), t, d, supervisor.Hooks{Detach: detach, Event: func(ev journal.Event) {
		if !queued && ev.Type == "attempt.state_changed" && payloadOf(t, ev)["state"] == "succeeded_native" {
			queued = true
			detach <- struct{}{}
		}
	}})
	if err != nil || !queued || out.DetachedRunning || out.Detached || out.State != supervisor.RunReadyForReview {
		t.Fatalf("terminal result %+v, queued=%t err=%v", out, queued, err)
	}
	if _, err := f.journal(t).Candidate(t.Context(), d.AttemptID); err != nil {
		t.Fatal(err)
	}
}

// Ready controls have no cross-channel select order. Repeat both enqueue
// orders so a first stop cannot disappear when a rapid detach is also ready.
func TestQueuedStopSurvivesDetach(t *testing.T) {
	// In-process supervision changes the runtime environment.
	for trial := range 12 {
		t.Run(fmt.Sprintf("ready_controls_%d", trial), func(t *testing.T) {
			f := newFixture(t)
			d := detachDecision(t, f, "slow")
			detach := make(chan struct{}, 1)
			interrupts := make(chan os.Signal, 1)
			sent := false
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			out, err := f.runInProcessWith(ctx, t, d, supervisor.Hooks{Detach: detach, Interrupt: interrupts, Event: func(ev journal.Event) {
				if !sent && ev.Type == "attempt.state_changed" && payloadOf(t, ev)["state"] == "running" {
					sent = true
					if trial%2 == 0 {
						interrupts <- os.Interrupt
						detach <- struct{}{}
					} else {
						detach <- struct{}{}
						interrupts <- os.Interrupt
					}
				}
			}})
			if err != nil || !sent || out.DetachedRunning || (out.State != supervisor.RunStopping && out.State != supervisor.RunCancelled) {
				t.Fatalf("queued stop lost: %+v, sent=%t err=%v", out, sent, err)
			} // I06 (v2 §2), ADR0020 first-request precedence.
			data, err := os.ReadFile(filepath.Join(workers.AttemptDir(f.state, d.RunID, d.AttemptID), "stop.request")) // #nosec G304 -- synthetic attempt control file
			if err != nil {
				t.Fatal(err)
			}
			var request struct {
				RequestID string `json:"request_id"`
			}
			err = json.Unmarshal(data, &request)
			if err != nil || request.RequestID != "user-interrupt" {
				t.Fatalf("first stop not persisted: %+v, %v", request, err)
			}
		})
	}
}
