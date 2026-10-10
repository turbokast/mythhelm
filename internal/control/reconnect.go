package control

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// SupervisorBeat is the supervisor.beat payload, mirroring the worker
// heartbeat shape plus the boot generation. The supervisor's watch loop
// writes it after every successful Ingest; the worker polls it for
// supervisor loss and return (design §5).
type SupervisorBeat struct {
	Beat       uint64 `json:"beat"`
	At         string `json:"at"` // RFC3339Nano, UTC
	Generation int64  `json:"generation"`
}

// MaxSupervisorBeatAge bounds beat staleness: 300ms, 3x the 100ms
// watch-tick beat interval (OQ-SR3). It mirrors
// workers.MaxSupervisorBeatAge, since workers must not import control;
// both pin 300ms and must change together.
const MaxSupervisorBeatAge = 300 * time.Millisecond

// supervisorBeatFile is the beat name in the attempt directory, shared
// with workers.ReadSupervisorBeat by exact keys (control asserts the read
// side in TestWriteSupervisorBeat).
const supervisorBeatFile = "supervisor.beat"

// WriteSupervisorBeat writes dir/supervisor.beat (0600) with the caller's
// beat counter, generation and timestamp. The write is atomic (temp file
// plus rename), so a torn read never parses as a beat; it is not synced,
// since a beat is ephemeral — a crash loses it, which correctly reads as
// stale. A beat is best-effort presence: the watch loop ignores a failure
// and retries next tick.
func WriteSupervisorBeat(dir string, generation int64, beat uint64, at time.Time) error {
	body, err := json.Marshal(SupervisorBeat{Beat: beat, At: at.UTC().Format(time.RFC3339Nano), Generation: generation})
	if err != nil {
		return fmt.Errorf("control: encoding the supervisor beat: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".supervisor.beat.*")
	if err != nil {
		return fmt.Errorf("control: writing the supervisor beat: %w", err)
	}
	name := tmp.Name()
	if _, err := tmp.Write(append(body, '\n')); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return fmt.Errorf("control: writing the supervisor beat: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("control: writing the supervisor beat: %w", err)
	}
	if err := os.Rename(name, filepath.Join(dir, supervisorBeatFile)); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("control: writing the supervisor beat: %w", err)
	}
	return nil
}

// ReconnectDeps are the reconnect handshake's dependencies: the ledger
// and the state directory holding the attempt's spool and identity.
type ReconnectDeps struct {
	DB       *sql.DB
	StateDir string
}

// Reconnect runs the supervisor-return handshake for runID: it reconciles
// launch identity and spool through Reconcile, then mints a new-generation
// capability token for the reconnected attempt. The reconcile always runs
// first — a token is minted only on a reconnected outcome, so stale or
// unverifiable evidence never buys fresh authority. Any other outcome
// records its pass and mints nothing: run recover (not the handshake)
// when the worker may be dead, since only the recover handler launches
// continuations. A repeated handshake replays the decision without
// re-recording it, and rotates the token (minting again replaces the live
// token). Like Reconcile, the handshake never launches.
func Reconnect(ctx context.Context, d ReconnectDeps, runID string) (string, RecoveryReport, error) {
	if d.DB == nil || d.StateDir == "" {
		return "", RecoveryReport{}, newError(CodePersistenceUnavailable, "reconnect is not available: no ledger is attached")
	}
	rep, _, _, err := reconcile(ctx, RecoverDeps{DB: d.DB, StateDir: d.StateDir}, runID, handshakeRecoveryEventID(runID))
	if err != nil || rep.Outcome != RecoverReconnected {
		return "", rep, err
	}
	token, err := MintToken(ctx, d.DB, rep.AttemptID)
	if err != nil {
		return "", rep, err
	}
	return token, rep, nil
}

// handshakeRecoveryEventID identifies a handshake-recorded pass. Its
// evt_handshake_ prefix distinguishes spawn-free handshake records from
// handler-recorded ones, so the recover handler can adopt a
// handshake-admitted continuation (which no handshake ever spawns)
// instead of leaving it unspawned.
func handshakeRecoveryEventID(runID string) string {
	var nonce [4]byte
	_, _ = rand.Read(nonce[:])
	return fmt.Sprintf("%s%s_%s", handshakeEventPrefix, runID, hex.EncodeToString(nonce[:]))
}
