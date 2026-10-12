package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/turbokast/mythhelm/internal/billing"
	"github.com/turbokast/mythhelm/internal/control"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/supervisor"
)

func TestExitCodeControlHeldIsOwnership(t *testing.T) {
	t.Parallel()
	for name, err := range map[string]error{
		"root conflict": control.ErrRootConflict,
		"instance held": control.ErrInstanceHeld,
	} {
		if got := exitCode(err); got != ExitOwnership {
			t.Errorf("exitCode(%s) = %d, want %d", name, got, ExitOwnership)
		}
		if got := exitCode(fmt.Errorf("supervisor: %w", err)); got != ExitOwnership {
			t.Errorf("exitCode(wrapped %s) = %d, want %d", name, got, ExitOwnership)
		}
	}
}

func TestExitCodeRefusedLaunchIsBlocked(t *testing.T) {
	t.Parallel()
	for name, err := range map[string]error{
		"envelope gate":       &supervisor.GateError{Reason: "envelope_deadline_exceeded", Err: errors.New("deadline passed")},
		"allowance exhausted": &supervisor.GateError{Reason: "allowance_exhausted", Err: billing.ErrAllowanceExhausted},
	} {
		for _, e := range []error{err, errors.Join(err, errors.New("run blocked"))} {
			if got := exitCode(e); got != ExitBlocked {
				t.Errorf("exitCode(%s) = %d, want %d", name, got, ExitBlocked)
			}
			if got := errorCategory(e); got != "admission_blocked" {
				t.Errorf("errorCategory(%s) = %q, want admission_blocked", name, got)
			}
		}
	}
}

func TestRunExitMapsVerificationInvalid(t *testing.T) {
	t.Parallel()
	code, category := runExit(supervisor.Outcome{State: supervisor.RunFailed, Reason: "verification_invalid"})
	if code != ExitVerify || category != "verification_invalid" {
		t.Fatalf("runExit(failed/verification_invalid) = %d/%q, want %d/verification_invalid", code, category, ExitVerify)
	}
}

func TestRunExitClassifiedFailures(t *testing.T) {
	t.Parallel()
	for reason, want := range map[string]ExitCode{
		"native_failed":       ExitNative,
		"protocol_error":      ExitNative,
		"provider_limit":      ExitNative,
		"verification_failed": ExitVerify,
	} {
		code, _ := runExit(supervisor.Outcome{State: supervisor.RunFailed, Reason: reason})
		if code != want {
			t.Fatalf("runExit(failed/%s) = %d, want %d", reason, code, want)
		}
	}
	for _, reason := range []string{"billing_route_mismatch", "native_auth_or_billing", "active_run_exists"} {
		code, category := runExit(supervisor.Outcome{State: supervisor.RunBlocked, Reason: reason})
		if code != ExitBlocked || category != "admission_blocked" {
			t.Fatalf("runExit(blocked/%s) = %d/%s, want 3/admission_blocked", reason, code, category)
		}
	}
}

// TestFinishFoldsUndocumentedEvent pins that run.result and the process agree
// when the renderer refused an event (I09, v2 §2).
func TestFinishFoldsUndocumentedEvent(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		outcomeErr   error
		wantExit     int
		wantCategory string
		wantNotice   bool
	}{
		{name: "an otherwise successful run exits 1 undocumented_event", wantExit: 1, wantCategory: "undocumented_event"},
		{
			name:       "an outcome error keeps its code and the refusal goes to stderr",
			outcomeErr: &outcomeError{code: ExitVerify, category: "verification_failed", err: errors.New("checks failed")},
			wantExit:   5, wantCategory: "verification_failed", wantNotice: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var out, diag bytes.Buffer
			r := newRenderer("jsonl", Stdio{Out: &out, Err: &diag})
			r.event(jsonlEvent("attempt.surprise"))

			returned := finish(r, supervisor.Outcome{RunID: "run_1", State: supervisor.RunCompleted}, tt.outcomeErr)

			var ev journal.Event
			if err := json.Unmarshal(out.Bytes(), &ev); err != nil || ev.Type != "run.result" {
				t.Fatalf("stdout = %q (%v), want only run.result", out.String(), err)
			}
			var res map[string]any
			if err := json.Unmarshal(ev.Payload, &res); err != nil {
				t.Fatal(err)
			}
			if res["exit_code"] != float64(tt.wantExit) || res["error_category"] != tt.wantCategory {
				t.Fatalf("run.result = %v, want exit_code %d and category %s", res, tt.wantExit, tt.wantCategory)
			}
			if got := exitCode(returned); got != ExitCode(tt.wantExit) {
				t.Fatalf("returned error %v has exit code %d, run.result said %d", returned, got, tt.wantExit)
			}
			if notice := strings.Contains(diag.String(), "attempt.surprise"); notice != tt.wantNotice {
				t.Fatalf("stderr = %q, want the refusal named: %t", diag.String(), tt.wantNotice)
			}
		})
	}
}
