package control

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/turbokast/mythhelm/internal/journal"
)

// EventAdmissionPinned is the journal event journaling the admission pin:
// the stop-ladder version and the expected worker-nonce digest. Task 2's
// RecordAdmissionPinned writes it; PinnedAdmission reads it.
const EventAdmissionPinned = "attempt.admission_pinned"

// AdmissionPinnedPayload is the payload of EventAdmissionPinned.
type AdmissionPinnedPayload struct {
	LadderVersion string `json:"ladder_version"`
	NonceSHA256   string `json:"nonce_sha256"` // NonceDigest of the raw nonce in Launch.Nonce
}

// PinnedAdmission returns the admission-pinned ladder version and expected
// nonce digest for attemptID from its latest EventAdmissionPinned event,
// read via j.Events over the same *journal.Journal store that
// RecordAdmissionPinned writes — there is no separate database binding,
// so a recorded pin is always visible to the lookup. Failure cases:
// invalid_contract (unknown attempt, or an attempt admitted before
// pinning, which has no such event); persistence_unavailable (ledger I/O).
func PinnedAdmission(ctx context.Context, j *journal.Journal, runID, attemptID string) (ladderVersion, nonceSHA256 string, err error) {
	events, err := j.Events(ctx, runID, 0)
	if err != nil {
		return "", "", newError(CodePersistenceUnavailable, "reading the admission pin: %v", err)
	}
	// Events arrive in run_sequence order; the last pin for the attempt wins.
	found := false
	var payload AdmissionPinnedPayload
	for _, ev := range events {
		if ev.AttemptID != attemptID || ev.Type != EventAdmissionPinned {
			continue
		}
		var p AdmissionPinnedPayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			return "", "", newError(CodeInvalidContract, "attempt %q admission pin is malformed: %v", attemptID, err)
		}
		payload, found = p, true
	}
	if !found {
		return "", "", newError(CodeInvalidContract, "attempt %q has no admission pin", attemptID)
	}
	if payload.LadderVersion == "" || payload.NonceSHA256 == "" {
		return "", "", newError(CodeInvalidContract, "attempt %q admission pin is incomplete", attemptID)
	}
	return payload.LadderVersion, payload.NonceSHA256, nil
}

// CurrentGeneration reports this supervisor's boot generation from the
// stream-2 lock-file metadata for dir. The lock must exist, parse, and
// claim dir's root; otherwise the generation is a guess and a plain
// error reports it, so the spawn site fails the admission rather than
// pinning a guessed generation.
func CurrentGeneration(dir string) (int64, error) {
	path, err := LockPath()
	if err != nil {
		return 0, fmt.Errorf("control: locating the instance lock: %w", err)
	}
	meta, ok := readMetadata(path)
	if !ok {
		return 0, fmt.Errorf("control: no live supervisor lock at %s", path)
	}
	if !sameRoot(meta.Root, canonicalRoot(dir)) {
		return 0, fmt.Errorf("control: the supervisor serves %s, not %s", meta.Root, dir)
	}
	return int64(meta.Generation), nil //nolint:gosec // G115: boot generations increment from 1 and cannot approach 2^63
}
