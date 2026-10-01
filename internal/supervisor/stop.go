package supervisor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/workers"
)

// ErrOwnership means execution ownership cannot be proved. No unverified
// process is signalled, and no replacement native writer is launched.
var ErrOwnership = errors.New("execution ownership is unresolved")

// Stop asks the verified worker to stop its native child. The request file
// is atomic and the worker journals its own acknowledgement; a caller cannot
// acquire the owner lock from the live supervisor just to request a stop.
func Stop(ctx context.Context, j *journal.Journal, runID string) (string, error) {
	if _, err := j.Run(ctx, runID); err != nil {
		return "", err
	}
	a, err := j.LatestAttempt(ctx, runID)
	if err != nil {
		return "", err
	}
	events, err := j.Events(ctx, runID, 0)
	if err != nil {
		return "", err
	}
	// Duplicate stopped events are reachable when a panic interrupts
	// conclude between the stopped line and the terminal state line. The
	// last observation wins, as in recovery and freeze-after-stop.
	var last struct {
		Confirmed      bool   `json:"confirmed"`
		UnresolvedPIDs []int  `json:"unresolved_pids"`
		DescendantScan string `json:"descendant_scan"`
	}
	found := false
	for _, ev := range events {
		if ev.AttemptID != a.AttemptID || ev.Type != "attempt.stopped" {
			continue
		}
		var stop struct {
			Confirmed      bool   `json:"confirmed"`
			UnresolvedPIDs []int  `json:"unresolved_pids"`
			DescendantScan string `json:"descendant_scan"`
		}
		if err := json.Unmarshal(ev.Payload, &stop); err != nil {
			return "", err
		}
		last, found = stop, true
	}
	if found {
		if last.DescendantScan == "failed" {
			return "", fmt.Errorf("%w: descendant scan failed", ErrOwnership)
		}
		if last.Confirmed && len(last.UnresolvedPIDs) == 0 {
			return "stopped", nil
		}
	}
	_, alive, err := recoveryIdentity(ctx, j, a)
	if err != nil || !alive {
		return "", errors.Join(ErrOwnership, err)
	}
	dir := workers.AttemptDir(j.StateDir(), runID, a.AttemptID)
	if err := workers.RequestStop(dir, ids.New("stop")); err != nil {
		return "", err
	}
	return "stop_requested", nil
}

// recoveryIdentity verifies the identity file against the durable launch
// token, any journaled process identity, and the current PID start time.
// A matching file with a gone PID is distinct from a reused or forged PID.
func recoveryIdentity(ctx context.Context, j *journal.Journal, a journal.AttemptRow) (workers.Identity, bool, error) {
	dir := workers.AttemptDir(j.StateDir(), a.RunID, a.AttemptID)
	deadline := time.Now().Add(identityTimeout)
	var id workers.Identity
	var err error
	for {
		id, err = workers.ReadIdentity(dir)
		if err == nil || !retryIdentityRead(err) || time.Now().After(deadline) {
			break
		}
		select {
		case <-ctx.Done():
			return id, false, ctx.Err()
		case <-time.After(identityPollInterval):
		}
	}
	if err != nil {
		return id, false, err
	}
	sum := sha256.Sum256([]byte(id.LaunchToken))
	if id.RunID != a.RunID || id.AttemptID != a.AttemptID || id.PID <= 0 || id.StartTime.IsZero() ||
		hex.EncodeToString(sum[:]) != a.LaunchTokenSHA256 {
		return id, false, errWorkerUnverified
	}
	events, err := j.Events(ctx, a.RunID, 0)
	if err != nil {
		return id, false, err
	}
	for _, ev := range events {
		if ev.AttemptID != a.AttemptID || ev.Type != "attempt.launched" {
			continue
		}
		var launched struct {
			WorkerPID       int       `json:"worker_pid"`
			WorkerStartTime time.Time `json:"worker_start_time"`
		}
		if err := json.Unmarshal(ev.Payload, &launched); err != nil {
			return id, false, err
		}
		if id.PID != launched.WorkerPID || !id.StartTime.Equal(launched.WorkerStartTime) {
			return id, false, errWorkerUnverified
		}
	}
	start, err := workers.ProcessStartTime(id.PID)
	if errors.Is(err, workers.ErrNoProcess) {
		return id, false, nil
	}
	if err != nil {
		return id, false, err
	}
	if !start.Equal(id.StartTime) {
		return id, false, fmt.Errorf("%w: worker PID start time changed", errWorkerUnverified)
	}
	return id, true, nil
}
