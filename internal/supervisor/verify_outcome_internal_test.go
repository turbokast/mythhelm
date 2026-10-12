package supervisor

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/turbokast/mythhelm/internal/acceptance"
)

// TestVerifyOutcomeTable pins the run outcome table of design §2.3: every
// verdict, a runner error that is not a cancellation, and a cancellation.
func TestVerifyOutcomeTable(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		err        error
		result     acceptance.Result
		wantState  RunState
		wantReason string
	}{
		{name: "passed is ready for review", result: acceptance.ResultPassed, wantState: RunReadyForReview, wantReason: ""},
		{name: "failed fails the run", result: acceptance.ResultFailed, wantState: RunFailed, wantReason: "verification_failed"},
		{name: "invalid fails the run as invalid", result: acceptance.ResultInvalid, wantState: RunFailed, wantReason: "verification_invalid"},
		{name: "unavailable leaves the candidate unverified", result: acceptance.ResultUnavailable, wantState: RunReadyForReview, wantReason: "unverified"},
		{name: "not run leaves the candidate unverified", result: acceptance.ResultNotRun, wantState: RunReadyForReview, wantReason: "unverified"},
		{name: "an unrecognised verdict fails closed", result: acceptance.Result("accepted"), wantState: RunFailed, wantReason: "verification_failed"},
		{name: "an empty verdict fails closed", result: acceptance.Result(""), wantState: RunFailed, wantReason: "verification_failed"},
		{name: "a runner error keeps the verdict decided over its statuses", err: errors.New("start check: permission denied"), result: acceptance.ResultUnavailable, wantState: RunReadyForReview, wantReason: "unverified"},
		{name: "a runner error keeps a failure", err: errors.New("start check: permission denied"), result: acceptance.ResultFailed, wantState: RunFailed, wantReason: "verification_failed"},
		{name: "a cancellation fails the run as unavailable", err: context.Canceled, result: acceptance.ResultUnavailable, wantState: RunFailed, wantReason: "verification_unavailable"},
		{name: "a wrapped cancellation fails the run as unavailable", err: fmt.Errorf("check: %w", context.Canceled), result: acceptance.ResultFailed, wantState: RunFailed, wantReason: "verification_unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			state, reason := verifyOutcome(tc.err, tc.result)
			if state != tc.wantState || reason != tc.wantReason {
				t.Fatalf("verifyOutcome(%v, %q) = %s/%q, want %s/%q", tc.err, tc.result, state, reason, tc.wantState, tc.wantReason)
			}
		})
	}
}
