package control

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/v2contract"
)

// EventAdmissionPinned is the journal event journaling the admission pin:
// the stop-ladder version and the expected worker-nonce digest. Task 2's
// RecordAdmissionPinned writes it; PinnedAdmission reads it.
const EventAdmissionPinned = "attempt.admission_pinned"

// AdmissionPinnedPayload is the payload of EventAdmissionPinned.
type AdmissionPinnedPayload struct {
	LadderVersion string `json:"ladder_version"`
	NonceSHA256   string `json:"nonce_sha256"` // NonceDigest of the raw nonce in Launch.Nonce
	// LaunchProvenance is the optional nested LaunchProvenanceV1 the
	// pre-Spawn producer journals; historical pins omit it.
	LaunchProvenance json.RawMessage `json:"launch_provenance,omitempty"`
}

// LaunchProvenanceV1 is the immutable launch provenance captured from the
// exact Launch handed to workers.Spawn (design §3.2). A present record
// always carries a non-nil Generation; explicit 0 is a known unfenced
// launch, never the same as an absent record (I09).
type LaunchProvenanceV1 struct {
	SchemaVersion     int       `json:"schema_version"`
	RunID             string    `json:"run_id"`
	AttemptID         string    `json:"attempt_id"`
	LaunchTokenSHA256 string    `json:"launch_token_sha256"`
	NonceSHA256       string    `json:"nonce_sha256"`
	LadderVersion     string    `json:"ladder_version"`
	Generation        *int64    `json:"generation"`
	ExecutionDeadline time.Time `json:"execution_deadline"`
}

// LaunchProvenanceSchemaVersion is the only supported record version.
const LaunchProvenanceSchemaVersion = 1

var launchProvenanceKeys = [...]string{"schema_version", "run_id", "attempt_id", "launch_token_sha256",
	"nonce_sha256", "ladder_version", "generation", "execution_deadline"}

// ReadLaunchProvenance returns the checked launch provenance for attemptID.
// It binds the record to the event's run and attempt, to the attempt row's
// launch token digest and to the outer admission pin; conflicting pins
// refuse rather than latest-wins. A missing record is ownership_unresolved,
// never a zero-valued success.
func ReadLaunchProvenance(ctx context.Context, j *journal.Journal, runID, attemptID string) (LaunchProvenanceV1, error) {
	events, err := j.Events(ctx, runID, 0)
	if err != nil {
		return LaunchProvenanceV1{}, provenanceError(v2contract.CodePersistenceUnavailable, runID, "", "retry after the journal is readable")
	}
	var outer *AdmissionPinnedPayload
	var record *LaunchProvenanceV1
	for _, ev := range events {
		if ev.Type != EventAdmissionPinned || ev.AttemptID != attemptID {
			continue
		}
		if ev.RunID != runID {
			return LaunchProvenanceV1{}, provenanceError(v2contract.CodeInvalidContract, runID, "identity_mismatch", "reconcile the attempt")
		}
		fields, ok := uniqueObject(ev.Payload)
		var p AdmissionPinnedPayload
		if !ok || json.Unmarshal(ev.Payload, &p) != nil || p.LadderVersion == "" || p.NonceSHA256 == "" {
			return LaunchProvenanceV1{}, provenanceError(v2contract.CodeInvalidContract, runID, "malformed", "reconcile the attempt")
		}
		if outer != nil && (outer.LadderVersion != p.LadderVersion || outer.NonceSHA256 != p.NonceSHA256) {
			return LaunchProvenanceV1{}, provenanceError(v2contract.CodeInvalidContract, runID, "conflicting", "reconcile the attempt")
		}
		outer = &p
		if nested, ok := fields["launch_provenance"]; ok {
			rec, err := decodeLaunchProvenance(nested, runID)
			if err != nil {
				return LaunchProvenanceV1{}, err
			}
			if record != nil && !sameProvenance(*record, rec) {
				return LaunchProvenanceV1{}, provenanceError(v2contract.CodeInvalidContract, runID, "conflicting", "reconcile the attempt")
			}
			record = &rec
		}
	}
	if record == nil {
		return LaunchProvenanceV1{}, provenanceError(v2contract.CodeOwnershipUnresolved, runID, "missing", "launch_generation_unknown")
	}
	if record.RunID != runID || record.AttemptID != attemptID ||
		record.NonceSHA256 != outer.NonceSHA256 || record.LadderVersion != outer.LadderVersion {
		return LaunchProvenanceV1{}, provenanceError(v2contract.CodeInvalidContract, runID, "identity_mismatch", "reconcile the attempt")
	}
	row, err := j.Attempt(ctx, attemptID)
	if err != nil {
		if errors.Is(err, journal.ErrNotFound) {
			return LaunchProvenanceV1{}, provenanceError(v2contract.CodeInvalidContract, runID, "identity_mismatch", "reconcile the attempt")
		}
		return LaunchProvenanceV1{}, provenanceError(v2contract.CodePersistenceUnavailable, runID, "", "retry after the journal is readable")
	}
	if row.RunID != runID || row.LaunchTokenSHA256 != record.LaunchTokenSHA256 {
		return LaunchProvenanceV1{}, provenanceError(v2contract.CodeInvalidContract, runID, "identity_mismatch", "reconcile the attempt")
	}
	out := *record
	gen := *record.Generation
	out.Generation = &gen
	return out, nil
}

func decodeLaunchProvenance(raw json.RawMessage, runID string) (LaunchProvenanceV1, error) {
	malformed := provenanceError(v2contract.CodeInvalidContract, runID, "malformed", "reconcile the attempt")
	fields, ok := uniqueObject(raw)
	if !ok {
		return LaunchProvenanceV1{}, malformed
	}
	var version int
	if v, ok := fields["schema_version"]; !ok || json.Unmarshal(v, &version) != nil || isNull(v) {
		return LaunchProvenanceV1{}, malformed
	}
	if version > LaunchProvenanceSchemaVersion {
		return LaunchProvenanceV1{}, provenanceError(v2contract.CodeCapabilityUnsupported, runID, "future", "upgrade mythhelm to read this launch provenance")
	}
	if version != LaunchProvenanceSchemaVersion || len(fields) != len(launchProvenanceKeys) {
		return LaunchProvenanceV1{}, malformed
	}
	for _, k := range launchProvenanceKeys {
		if v, ok := fields[k]; !ok || isNull(v) {
			return LaunchProvenanceV1{}, malformed
		}
	}
	var rec LaunchProvenanceV1
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&rec); err != nil {
		return LaunchProvenanceV1{}, malformed
	}
	if rec.RunID == "" || rec.AttemptID == "" || rec.LaunchTokenSHA256 == "" || rec.NonceSHA256 == "" ||
		rec.LadderVersion == "" || rec.Generation == nil || *rec.Generation < 0 || rec.ExecutionDeadline.IsZero() {
		return LaunchProvenanceV1{}, malformed
	}
	return rec, nil
}

func isNull(v json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(v), []byte("null"))
}

// uniqueObject decodes a JSON object's top-level members, refusing
// duplicate keys, which encoding/json would silently resolve latest-wins.
func uniqueObject(raw json.RawMessage) (map[string]json.RawMessage, bool) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, false
	}
	fields := map[string]json.RawMessage{}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, false
		}
		key, ok := tok.(string)
		if !ok {
			return nil, false
		}
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return nil, false
		}
		if _, dup := fields[key]; dup {
			return nil, false
		}
		fields[key] = v
	}
	if _, err := dec.Token(); err != nil {
		return nil, false
	}
	return fields, true
}

func sameProvenance(a, b LaunchProvenanceV1) bool {
	return a.SchemaVersion == b.SchemaVersion && a.RunID == b.RunID && a.AttemptID == b.AttemptID &&
		a.LaunchTokenSHA256 == b.LaunchTokenSHA256 && a.NonceSHA256 == b.NonceSHA256 &&
		a.LadderVersion == b.LadderVersion && *a.Generation == *b.Generation &&
		a.ExecutionDeadline.Equal(b.ExecutionDeadline)
}

func provenanceError(code v2contract.Code, runID, reason, next string) *v2contract.ControlError {
	e := &v2contract.ControlError{
		Code:        code,
		Owner:       "control",
		OperationID: "launch-provenance.read",
		Object:      runID,
		Disposition: code.DefaultDisposition(),
		NextAction:  next,
	}
	if reason != "" {
		e.Detail = map[string]string{"mythhelm.launch-provenance/reason": reason}
	}
	return e
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
// claim dir's root. An absent lock file reports ErrNoSupervisorLock and
// a lock serving another root reports ErrRootConflict, so the spawn
// site can pin those two unfenced cases generation 0; any other
// failure — an unreadable or unparseable lock file, an unlocatable
// lock — is a plain error, and the spawn site fails the admission
// rather than pinning a guessed generation.
func CurrentGeneration(dir string) (int64, error) {
	path, err := LockPath()
	if err != nil {
		return 0, fmt.Errorf("control: locating the instance lock: %w", err)
	}
	meta, err := readMetadataErr(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, fmt.Errorf("%w at %s", ErrNoSupervisorLock, path)
		}
		return 0, fmt.Errorf("control: reading the supervisor lock at %s: %w", path, err)
	}
	if !sameRoot(meta.Root, canonicalRoot(dir)) {
		return 0, fmt.Errorf("%w: the supervisor serves %s, not %s", ErrRootConflict, meta.Root, dir)
	}
	return int64(meta.Generation), nil //nolint:gosec // G115: boot generations increment from 1 and cannot approach 2^63
}
