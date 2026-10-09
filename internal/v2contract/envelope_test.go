package v2contract_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/v2contract"
)

// validEnvelope returns a Validate-passing Envelope with fixed values.
func validEnvelope() v2contract.Envelope {
	return v2contract.Envelope{
		SchemaVersion:    v2contract.SchemaVersion,
		EventID:          "evt_demo_0001",
		RunID:            "run_demo_0001",
		TaskID:           "task_demo_0001",
		ProducerID:       "supervisor",
		ProducerSequence: 41,
		RunSequence:      1024,
		Generation:       3,
		ObservedAt:       time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC),
		Type:             "task.accepted",
		Payload:          json.RawMessage(`{"task_id":"task_demo_0001","revision":2}`),
	}
}

// AC-4.1/AC-9.1: the golden decodes and re-encodes byte-identical (modulo the
// file's trailing newline); wrong contract versions and non-object payloads fail.
func TestEnvelopeGoldenRoundTrip(t *testing.T) {
	t.Parallel()
	raw := loadGolden(t, "envelope.golden.json")

	env, err := v2contract.Decode[v2contract.Envelope](raw)
	if err != nil {
		t.Fatalf("Decode(golden) error = %v", err)
	}
	re, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("Marshal(decoded golden) error = %v", err)
	}
	if !bytes.Equal(re, bytes.TrimSpace(raw)) {
		t.Errorf("re-encode differs:\n got %s\nwant %s", re, bytes.TrimSpace(raw))
	}
	if env.SchemaVersion != 2 {
		t.Errorf("golden schema_version = %d, want 2", env.SchemaVersion)
	}

	if _, err := v2contract.Decode[v2contract.Envelope](
		bytes.Replace(raw, []byte(`"schema_version":2`), []byte(`"schema_version":1`), 1),
	); err == nil || !strings.Contains(err.Error(), "schema_version") {
		t.Errorf("schema_version 1 error = %v, want naming schema_version", err)
	}

	for _, payload := range []string{`[1,2]`, `"just-a-string"`, `42`, `null`} {
		bad := bytes.Replace(raw,
			[]byte(`"payload":{"task_id":"task_demo_0001","revision":2}`),
			[]byte(`"payload":`+payload), 1)
		if _, err := v2contract.Decode[v2contract.Envelope](bad); err == nil ||
			!strings.Contains(err.Error(), "payload") {
			t.Errorf("payload %s error = %v, want naming payload", payload, err)
		}
	}
}

// I09 (v2 §2): observed_at is required; the zero time is never accepted.
func TestEnvelopeRejectsMissingObservedAt(t *testing.T) {
	t.Parallel()
	raw := loadGolden(t, "envelope.golden.json")
	noObserved := bytes.Replace(raw, []byte(`,"observed_at":"2026-10-08T12:00:00Z"`), nil, 1)
	if bytes.Equal(noObserved, raw) {
		t.Fatal("golden lost its observed_at fixture; update the removal below")
	}
	if _, err := v2contract.Decode[v2contract.Envelope](noObserved); err == nil ||
		!strings.Contains(err.Error(), "observed_at") {
		t.Errorf("missing observed_at error = %v, want naming observed_at", err)
	}

	zero := validEnvelope()
	zero.ObservedAt = time.Time{}
	if err := zero.Validate(); err == nil || !strings.Contains(err.Error(), "observed_at") {
		t.Errorf("zero ObservedAt error = %v, want naming observed_at", err)
	}

	zeroJSON := bytes.Replace(raw,
		[]byte(`"observed_at":"2026-10-08T12:00:00Z"`),
		[]byte(`"observed_at":"0001-01-01T00:00:00Z"`), 1)
	if _, err := v2contract.Decode[v2contract.Envelope](zeroJSON); err == nil ||
		!strings.Contains(err.Error(), "observed_at") {
		t.Errorf("zero-time observed_at error = %v, want naming observed_at", err)
	}
}

// The spec-3 Append extension stays mechanical only while Envelope mirrors
// journal.Event field-for-field (same JSON names, same order, same types).
func TestEnvelopeMirrorsJournalEvent(t *testing.T) {
	t.Parallel()
	got := reflect.TypeFor[v2contract.Envelope]()
	want := reflect.TypeFor[journal.Event]()
	if got.NumField() != want.NumField() {
		t.Fatalf("Envelope has %d fields, journal.Event has %d", got.NumField(), want.NumField())
	}
	for i := range got.NumField() {
		g, w := got.Field(i), want.Field(i)
		if g.Name != w.Name || g.Tag.Get("json") != w.Tag.Get("json") || g.Type != w.Type {
			t.Errorf("field %d: Envelope{%s %s %q} != journal.Event{%s %s %q}",
				i, g.Name, g.Type, g.Tag.Get("json"), w.Name, w.Type, w.Tag.Get("json"))
		}
	}
}

// AC-4.1: contiguous per-producer sequences; a re-sent sequence under a new
// event_id is a gap (idempotency is by event_id), matching Append.
func TestCheckSequence(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		last    int64
		got     int64
		wantGap bool
	}{
		{"next passes", 41, 42, false},
		{"first sequence passes", 0, 1, false},
		{"equal is a gap", 41, 41, true},
		{"regressed is a gap", 41, 40, true},
		{"skipped is a gap", 41, 43, true},
		{"large jump is a gap", 41, 100, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := v2contract.CheckSequence(tc.last, tc.got)
			if !tc.wantGap {
				if err != nil {
					t.Fatalf("CheckSequence(%d, %d) error = %v", tc.last, tc.got, err)
				}
				return
			}
			if !errors.Is(err, v2contract.ErrSequenceGap) {
				t.Fatalf("CheckSequence(%d, %d) error = %v, want ErrSequenceGap", tc.last, tc.got, err)
			}
		})
	}
}

// AC-4.1/AC-4.2: duplicates ack by event_id; stale generations quarantine
// (ErrStaleGeneration), current and newer generations pass.
func TestCheckDuplicateAndGeneration(t *testing.T) {
	t.Parallel()
	if err := v2contract.CheckDuplicate(true); !errors.Is(err, v2contract.ErrDuplicateEvent) {
		t.Errorf("CheckDuplicate(true) error = %v, want ErrDuplicateEvent", err)
	}
	if err := v2contract.CheckDuplicate(false); err != nil {
		t.Errorf("CheckDuplicate(false) error = %v, want nil", err)
	}

	tests := []struct {
		name    string
		current int64
		got     int64
		wanterr error
	}{
		{"older is stale", 3, 2, v2contract.ErrStaleGeneration},
		{"current passes", 3, 3, nil},
		{"newer passes", 3, 4, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := v2contract.CheckGeneration(tc.current, tc.got)
			if tc.wanterr == nil {
				if err != nil {
					t.Fatalf("CheckGeneration(%d, %d) error = %v", tc.current, tc.got, err)
				}
				return
			}
			if !errors.Is(err, tc.wanterr) {
				t.Fatalf("CheckGeneration(%d, %d) error = %v, want %v", tc.current, tc.got, err, tc.wanterr)
			}
		})
	}
}

// AC-9.1: int64 sequences survive a JSON round-trip exactly, even at the
// int64 maximum (past the 1<<53 float boundary).
func TestInt64SequencePrecision(t *testing.T) {
	t.Parallel()
	env := validEnvelope()
	env.ProducerSequence = math.MaxInt64
	env.RunSequence = math.MaxInt64
	env.Generation = math.MaxInt64
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("Marshal error = %v", err)
	}
	if !strings.Contains(string(data), "9223372036854775807") {
		t.Fatalf("marshalled envelope lost int64 precision: %s", data)
	}
	back, err := v2contract.Decode[v2contract.Envelope](data)
	if err != nil {
		t.Fatalf("Decode error = %v", err)
	}
	if back.ProducerSequence != math.MaxInt64 || back.RunSequence != math.MaxInt64 ||
		back.Generation != math.MaxInt64 {
		t.Errorf("round-trip sequences = %d/%d/%d, want all %d",
			back.ProducerSequence, back.RunSequence, back.Generation, int64(math.MaxInt64))
	}
}

// Envelope.Validate names every offending field; optional TaskID/AttemptID/
// CausedBy pass absent.
func TestEnvelopeValidateNamesFields(t *testing.T) {
	t.Parallel()
	mutate := func(e v2contract.Envelope, f func(*v2contract.Envelope)) v2contract.Envelope {
		f(&e)
		return e
	}
	tests := []struct {
		name    string
		env     v2contract.Envelope
		wantErr string
	}{
		{"valid passes", validEnvelope(), ""},
		{"schema_version", mutate(validEnvelope(), func(e *v2contract.Envelope) { e.SchemaVersion = 1 }), "schema_version"},
		{"event_id", mutate(validEnvelope(), func(e *v2contract.Envelope) { e.EventID = "" }), "event_id"},
		{"run_id", mutate(validEnvelope(), func(e *v2contract.Envelope) { e.RunID = "" }), "run_id"},
		{"producer_id", mutate(validEnvelope(), func(e *v2contract.Envelope) { e.ProducerID = "" }), "producer_id"},
		{"type", mutate(validEnvelope(), func(e *v2contract.Envelope) { e.Type = "" }), "type"},
		{"producer_sequence", mutate(validEnvelope(), func(e *v2contract.Envelope) { e.ProducerSequence = -1 }), "producer_sequence"},
		{"run_sequence", mutate(validEnvelope(), func(e *v2contract.Envelope) { e.RunSequence = -1 }), "run_sequence"},
		{"generation", mutate(validEnvelope(), func(e *v2contract.Envelope) { e.Generation = -1 }), "generation"},
		{"payload missing", mutate(validEnvelope(), func(e *v2contract.Envelope) { e.Payload = nil }), "payload"},
		{"payload invalid", mutate(validEnvelope(), func(e *v2contract.Envelope) { e.Payload = json.RawMessage(`{oops`) }), "payload"},
		{"optional absent passes", mutate(validEnvelope(), func(e *v2contract.Envelope) {
			e.TaskID, e.AttemptID, e.CausedBy = "", "", ""
		}), ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.env.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Validate() error = %v, want naming %q", err, tc.wantErr)
			}
		})
	}
}
