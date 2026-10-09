package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/journal"
)

// openPinJournal opens a scratch state database for pinning tests.
func openPinJournal(t *testing.T) *journal.Journal {
	t.Helper()
	j, err := journal.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	return j
}

// appendPin journals one admission-pinned event with literal wire keys, so a
// drifted payload struct cannot pass.
func appendPin(t *testing.T, j *journal.Journal, runID, attemptID, version, digest string, seq int64) {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"ladder_version": version, "nonce_sha256": digest})
	if err != nil {
		t.Fatal(err)
	}
	if err := j.Append(t.Context(), journal.Event{
		SchemaVersion:    journal.EnvelopeVersion,
		EventID:          fmt.Sprintf("evt_pin_%s_%d", attemptID, seq),
		RunID:            runID,
		AttemptID:        attemptID,
		ProducerID:       "sup_pin_test_" + attemptID,
		ProducerSequence: seq,
		Generation:       1,
		ObservedAt:       time.Now().UTC(),
		Type:             "attempt.admission_pinned",
		Payload:          payload,
	}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestPinnedAdmissionReadsLatestPinning(t *testing.T) {
	t.Parallel()
	if EventAdmissionPinned != "attempt.admission_pinned" {
		t.Fatalf("EventAdmissionPinned = %q, want the wire type", EventAdmissionPinned)
	}
	j := openPinJournal(t)
	ctx := context.Background()
	const runID, attemptID = "run_pin", "att_pin"
	appendPin(t, j, runID, attemptID, "stop-ladder/v1", "digest-first", 1)
	appendPin(t, j, runID, attemptID, "stop-ladder/v2", "digest-second", 2)

	version, digest, err := PinnedAdmission(ctx, j, runID, attemptID)
	if err != nil {
		t.Fatal(err)
	}
	if version != "stop-ladder/v2" || digest != "digest-second" {
		t.Fatalf("PinnedAdmission = (%q, %q), want the latest pin", version, digest)
	}

	// An unknown attempt, and one with journaled events but no pinning
	// (pre-pinning shape), both read as invalid_contract.
	if _, _, err := PinnedAdmission(ctx, j, "run_unknown", "att_unknown"); !errors.Is(err, &Error{Code: CodeInvalidContract}) {
		t.Fatalf("unknown attempt err = %v, want invalid_contract", err)
	}
	if err := j.Append(ctx, journal.Event{
		SchemaVersion: journal.EnvelopeVersion, EventID: "evt_launched_1", RunID: runID,
		AttemptID: "att_legacy", ProducerID: "wrk_att_legacy", ProducerSequence: 1,
		Generation: 1, ObservedAt: time.Now().UTC(), Type: "attempt.launched",
		Payload: json.RawMessage(`{"worker_pid":7}`),
	}, nil); err != nil {
		t.Fatal(err)
	}
	if _, _, err := PinnedAdmission(ctx, j, runID, "att_legacy"); !errors.Is(err, &Error{Code: CodeInvalidContract}) {
		t.Fatalf("unpinned attempt err = %v, want invalid_contract", err)
	}
}

func TestPinnedAdmissionRejectsCorruptPin(t *testing.T) {
	t.Parallel()
	j := openPinJournal(t)
	ctx := context.Background()
	const runID = "run_corrupt"
	for name, pin := range map[string][2]string{
		"empty version": {"", "digest"},
		"empty digest":  {"stop-ladder/v1", ""},
	} {
		appendPin(t, j, runID, "att_"+name, pin[0], pin[1], 1)
		if _, _, err := PinnedAdmission(ctx, j, runID, "att_"+name); !errors.Is(err, &Error{Code: CodeInvalidContract}) {
			t.Errorf("%s: err = %v, want invalid_contract", name, err)
		}
	}
	var payload AdmissionPinnedPayload
	if err := json.Unmarshal([]byte(`{"ladder_version":"stop-ladder/v1","nonce_sha256":"digest"}`), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.LadderVersion != "stop-ladder/v1" || payload.NonceSHA256 != "digest" {
		t.Fatalf("AdmissionPinnedPayload decodes %+v, want the wire keys", payload)
	}
}

func TestCurrentGenerationReadsBootGeneration(t *testing.T) {
	// not parallel: t.Setenv redirects the per-user lock directory.
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	root := t.TempDir()

	release, err := AcquireInstance(root)
	if err != nil {
		t.Fatal(err)
	}
	gen, err := CurrentGeneration(root)
	release()
	if err != nil {
		t.Fatal(err)
	}
	if gen != 1 {
		t.Fatalf("CurrentGeneration = %d, want 1", gen)
	}

	again, err := AcquireInstance(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(again)
	gen, err = CurrentGeneration(root)
	if err != nil {
		t.Fatal(err)
	}
	if gen != 2 {
		t.Fatalf("CurrentGeneration after re-acquire = %d, want 2", gen)
	}
}

func TestCurrentGenerationFailsWithoutLock(t *testing.T) {
	// not parallel: t.Setenv redirects the per-user lock directory.
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())

	_, err := CurrentGeneration(t.TempDir())
	if err == nil {
		t.Fatal("CurrentGeneration without a lock = nil, want a plain error")
	}
	if _, ok := errors.AsType[*Error](err); ok {
		t.Fatalf("CurrentGeneration error = %v, want a plain error, not a coded one", err)
	}
}

func TestCurrentGenerationRefusesForeignRoot(t *testing.T) {
	// not parallel: t.Setenv redirects the per-user lock directory.
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	rootA, rootB := t.TempDir(), t.TempDir()

	release, err := AcquireInstance(rootA)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)

	if _, err := CurrentGeneration(rootB); err == nil {
		t.Fatal("CurrentGeneration for a foreign root = nil, want an error")
	}
	if _, err := CurrentGeneration(rootA); err != nil {
		t.Fatalf("CurrentGeneration for the claimed root: %v", err)
	}
}
