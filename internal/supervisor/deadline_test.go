package supervisor_test

import (
	"context"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/workers"
)

func TestDeadlineRecoveryConclusion(t *testing.T) {
	t.Parallel()
	for _, first := range []string{workers.DeadlineRequester, "user-first"} {
		t.Run(first, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			d, _, wait := seedLaunchTimed(t, f, "slow", pinnedPayload, 2*time.Second)
			dir := workers.AttemptDir(f.state, d.RunID, d.AttemptID)
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
