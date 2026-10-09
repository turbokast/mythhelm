package v2contract

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"
)

// Envelope is the v2 canonical event (v2 §4.3). It mirrors journal.Event
// field-for-field (same JSON names, order and types) so the spec-3 Append
// extension is mechanical; the types differ only by required schema_version.
// Payload is JSON-canonical and excluded from TOML (toml:"-"): TOML tags on
// records cover configuration shapes; event payloads are JSON only.
type Envelope struct {
	SchemaVersion    int             `json:"schema_version" toml:"schema_version"` // must be 2
	EventID          string          `json:"event_id" toml:"event_id"`
	RunID            string          `json:"run_id" toml:"run_id"`
	TaskID           string          `json:"task_id,omitempty" toml:"task_id,omitempty"`
	AttemptID        string          `json:"attempt_id,omitempty" toml:"attempt_id,omitempty"`
	ProducerID       string          `json:"producer_id" toml:"producer_id"`
	ProducerSequence int64           `json:"producer_sequence" toml:"producer_sequence"`
	RunSequence      int64           `json:"run_sequence" toml:"run_sequence"`
	Generation       int64           `json:"generation" toml:"generation"`
	CausedBy         string          `json:"caused_by,omitempty" toml:"caused_by,omitempty"`
	ObservedAt       time.Time       `json:"observed_at" toml:"observed_at"`
	Type             string          `json:"type" toml:"type"`
	Payload          json.RawMessage `json:"payload" toml:"-"`
}

var (
	// ErrDuplicateEvent reports an event_id already journaled: ack without
	// append. Idempotency is by event_id, never by sequence.
	ErrDuplicateEvent = errors.New("v2contract: duplicate event")
	// ErrSequenceGap reports a producer_sequence that is not exactly one
	// more than the producer's last appended sequence.
	ErrSequenceGap = errors.New("v2contract: producer sequence gap")
	// ErrStaleGeneration reports an event from a producer generation older
	// than one already journaled: retain as quarantined evidence at most,
	// never an accepted result (AC-4.2).
	ErrStaleGeneration = errors.New("v2contract: stale generation")
)

// Validate enforces the v2 §4.3 envelope contract, naming the offending
// field: schema_version == 2; EventID/RunID/ProducerID/Type non-empty;
// sequences and generation >= 0; ObservedAt non-zero (an omitted
// observed_at fails, never decodes as year 1); Payload a JSON object.
// The event-type vocabulary is open (D6): unknown critical types cannot be
// ignored, which is ingestion behaviour in spec 3, not a closed enum here.
// TaskID, AttemptID and CausedBy are optional and pass absent.
func (e Envelope) Validate() error {
	if e.SchemaVersion != SchemaVersion {
		return fmt.Errorf("v2contract: envelope schema_version %d is not %d", e.SchemaVersion, SchemaVersion)
	}
	if e.EventID == "" {
		return errors.New("v2contract: envelope event_id is empty")
	}
	if e.RunID == "" {
		return errors.New("v2contract: envelope run_id is empty")
	}
	if e.ProducerID == "" {
		return errors.New("v2contract: envelope producer_id is empty")
	}
	if e.ProducerSequence < 0 {
		return fmt.Errorf("v2contract: envelope producer_sequence %d is negative", e.ProducerSequence)
	}
	if e.RunSequence < 0 {
		return fmt.Errorf("v2contract: envelope run_sequence %d is negative", e.RunSequence)
	}
	if e.Generation < 0 {
		return fmt.Errorf("v2contract: envelope generation %d is negative", e.Generation)
	}
	if e.ObservedAt.IsZero() {
		return errors.New("v2contract: envelope observed_at is missing or zero")
	}
	if e.Type == "" {
		return errors.New("v2contract: envelope type is empty")
	}
	trimmed := bytes.TrimSpace(e.Payload)
	if len(trimmed) == 0 {
		return errors.New("v2contract: envelope payload is missing")
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &obj); err != nil || obj == nil {
		return fmt.Errorf("v2contract: envelope payload is not a JSON object: %s", trimmed)
	}
	return nil
}

// CheckSequence requires got to be exactly last+1, else ErrSequenceGap.
// Duplicates are NOT detected here: idempotency is by event_id at the
// ledger (CheckDuplicate), matching Append's check order (event_id first,
// then generation, then sequence).
func CheckSequence(last, got int64) error {
	if last == math.MaxInt64 || got != last+1 {
		return fmt.Errorf("%w: last %d, got %d", ErrSequenceGap, last, got)
	}
	return nil
}

// CheckDuplicate reports ErrDuplicateEvent when the event_id was already
// journaled (ack, no append); else nil.
func CheckDuplicate(seen bool) error {
	if seen {
		return ErrDuplicateEvent
	}
	return nil
}

// CheckGeneration requires got to be at least the current generation, else
// ErrStaleGeneration: quarantine the event at most (AC-4.2).
func CheckGeneration(current, got int64) error {
	if got < current {
		return fmt.Errorf("%w: current %d, got %d", ErrStaleGeneration, current, got)
	}
	return nil
}
