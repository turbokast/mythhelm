package workers

// Supervisor-loss envelope (design §5): what a worker may do without a
// supervisor, and how it detects the loss. Loss and return are file
// signals: the supervisor's watch loop writes supervisor.beat after every
// successful Ingest, and the worker polls it. A worker whose episode ends
// under a stale beat finishes only that episode, spools its results, and
// waits for reconnection; it starts no new task and approves no new
// effect. UI/Herdr loss never triggers this: only the beat counts.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// supervisorBeatFile is the supervisor→worker presence signal in the
// attempt directory, mirroring the worker heartbeat shape plus the boot
// generation. The supervisor writes it (control.WriteSupervisorBeat); the
// worker only reads it.
const supervisorBeatFile = "supervisor.beat"

// MaxSupervisorBeatAge bounds beat staleness: 300ms, 3x the 100ms
// watch-tick beat interval (OQ-SR3). control mirrors this value as
// control.MaxSupervisorBeatAge, since workers must not import control;
// both pin 300ms and must change together.
const MaxSupervisorBeatAge = 300 * time.Millisecond

// awaitPollInterval is how often AwaitReconnect re-reads the beat: well
// under the staleness bound, so a return releases the wait promptly.
const awaitPollInterval = 50 * time.Millisecond

// EpisodeEnvelope pins what the worker may do without a supervisor. It is
// built from values the worker already holds: the attempt ID from argv
// --attempt, the one-use launch identity from Launch.LaunchToken, and the
// supervisor boot generation pinned at admission in Launch.Generation.
// A zero Generation is unfenced (Task 2 pins 0 when no supervisor holds
// the lock): the worker behaves exactly as before envelopes, with no
// loss detection, since there is no supervisor to lose.
type EpisodeEnvelope struct {
	AttemptID  string `json:"attempt_id"`
	LaunchID   string `json:"launch_id"`
	Generation int64  `json:"generation"`
}

// errRevisionConflict reports an envelope mismatch in the control-code
// vocabulary (the errOwnershipUnresolved precedent): the worker treats
// any failure as "await reconnection", never as permission to improvise.
var errRevisionConflict = errors.New("revision_conflict")

// CheckEnvelope allows only the pinned episode: the attempt, launch and
// generation must all agree, except that an unfenced envelope (generation
// 0) accepts any supervisor generation. Any mismatch is revision_conflict.
func CheckEnvelope(env EpisodeEnvelope, attemptID, launchID string, generation int64) error {
	switch {
	case env.AttemptID == "" || attemptID == "" || env.AttemptID != attemptID:
		return fmt.Errorf("%w: attempt %q is outside the pinned episode %q", errRevisionConflict, attemptID, env.AttemptID)
	case env.LaunchID == "" || launchID == "" || env.LaunchID != launchID:
		return fmt.Errorf("%w: launch %q is outside the pinned episode", errRevisionConflict, launchID)
	case env.Generation != 0 && generation != env.Generation:
		return fmt.Errorf("%w: generation %d is outside the pinned episode generation %d", errRevisionConflict, generation, env.Generation)
	default:
		return nil
	}
}

// beatFresh reports whether a beat observed at at is fresher than
// MaxSupervisorBeatAge at now.
func beatFresh(at, now time.Time) bool {
	return now.Sub(at) <= MaxSupervisorBeatAge
}

// ReadSupervisorBeat reports the latest supervisor beat: when it was
// written and under which boot generation. ok is false when the file is
// missing or unparsable, which the caller treats as stale; a stale but
// legible beat still parses, since staleness is the caller's verdict.
func ReadSupervisorBeat(dir string) (at time.Time, generation int64, ok bool) {
	raw, err := os.ReadFile(filepath.Join(dir, supervisorBeatFile)) //nolint:gosec // G304: fixed beat name in the attempt directory
	if err != nil {
		return time.Time{}, 0, false
	}
	var beat struct {
		At         string `json:"at"`
		Generation int64  `json:"generation"`
	}
	if err := json.Unmarshal(raw, &beat); err != nil {
		return time.Time{}, 0, false
	}
	at, err = time.Parse(time.RFC3339Nano, beat.At)
	if err != nil {
		return time.Time{}, 0, false
	}
	return at, beat.Generation, true
}

// SupervisorBeatStale reports whether dir has no beat fresher than
// MaxSupervisorBeatAge at now: a missing, unparsable or old beat all read
// stale.
func SupervisorBeatStale(dir string, now time.Time) bool {
	at, _, ok := ReadSupervisorBeat(dir)
	return !ok || !beatFresh(at, now)
}

// AwaitReconnect polls supervisor.beat until the supervisor returns or
// ctx ends. A beat fresher than MaxSupervisorBeatAge, of any generation,
// ends the wait: a changed generation means the supervisor restarted, and
// the reconcile-then-mint handshake re-homes the worker, which never
// treats the mismatch as permission to improvise. It starts no new task,
// approves no new effect, and writes nothing: cancelling ctx ends the
// wait without touching the spool. Callers enter it only after the
// episode is finished and spooled. A malformed envelope refuses the wait
// instead of waiting under garbage.
func AwaitReconnect(ctx context.Context, dir string, env EpisodeEnvelope) error {
	if err := CheckEnvelope(env, env.AttemptID, env.LaunchID, env.Generation); err != nil {
		return err
	}
	tick := time.NewTicker(awaitPollInterval)
	defer tick.Stop()
	for {
		if at, _, ok := ReadSupervisorBeat(dir); ok && beatFresh(at, time.Now()) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}
	}
}
