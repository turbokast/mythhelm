package supervisor

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"time"

	"github.com/turbokast/mythhelm/internal/adapter"
	"github.com/turbokast/mythhelm/internal/billing"
	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/workers"
)

// SpoolFile is the worker's spool inside its attempt directory.
const SpoolFile = "spool.jsonl"

// maxSpoolLine bounds one spooled event. The worker writes identifiers,
// counts and labels only, so a longer line means a corrupt spool.
const maxSpoolLine = 1 << 20

// ErrCorruptSpool reports a spool line that is not an event of the attempt's
// worker. Ingestion stops at that line; nothing after it is journaled.
var ErrCorruptSpool = errors.New("corrupt spool")

// workerEvents are the event types a worker may spool (ADR 0004).
var workerEvents = []string{
	"attempt.state_changed", "attempt.launched", "attempt.native_session", "attempt.progress",
	"attempt.permission_denied", "attempt.protocol_counters", "attempt.native_result",
	"attempt.stop_requested", "attempt.stopped", "attempt.native_error",
}

// AttemptRef locates an attempt's files in the state directory.
type AttemptRef struct {
	StateDir  string
	RunID     string
	AttemptID string
}

// Dir is the attempt directory.
func (a AttemptRef) Dir() string { return workers.AttemptDir(a.StateDir, a.RunID, a.AttemptID) }

func workerProducer(attemptID string) string { return "wrk_" + attemptID }

// Ingest journals the complete spool lines of attempt that follow its stored
// spool offset, in order, and returns the worker's last journaled
// producer_sequence. Each event commits in one transaction with its
// projection update and the new offset, so a crash between two events
// resumes at the first one not journaled, and an event journaled twice is
// ignored by its event_id (AC-9.2). A torn last line is left for the next
// call. Only the run's owner-lock holder may call it (design §3).
func Ingest(ctx context.Context, j *journal.Journal, attempt AttemptRef) (lastSeq int64, err error) {
	row, err := j.Attempt(ctx, attempt.AttemptID)
	if err != nil {
		return 0, err
	}
	producer := workerProducer(attempt.AttemptID)
	f, err := os.Open(filepath.Join(attempt.Dir(), SpoolFile))
	switch {
	case errors.Is(err, os.ErrNotExist):
		return j.ProducerSequence(ctx, producer)
	case err != nil:
		return 0, fmt.Errorf("opening the spool: %w", err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Seek(row.SpoolOffset, io.SeekStart); err != nil {
		return 0, fmt.Errorf("seeking the spool: %w", err)
	}
	r := bufio.NewReader(f)
	offset := row.SpoolOffset
	skipped := false // a line whose event was already journaled
	for {
		line, complete, err := readLine(r)
		if err != nil {
			return 0, fmt.Errorf("reading the spool at byte %d: %w", offset, err)
		}
		if !complete {
			break
		}
		next := offset + int64(len(line))
		ev, err := decodeSpoolLine(line, attempt, producer)
		if err != nil {
			return 0, fmt.Errorf("spool byte %d: %w", offset, err)
		}
		applied := false
		if err := j.Append(ctx, ev, func(tx *sql.Tx) error {
			applied = true
			if err := projectWorkerEvent(ctx, tx, ev); err != nil {
				return err
			}
			return journal.SetSpoolOffset(ctx, tx, attempt.AttemptID, next)
		}); err != nil {
			return 0, err
		}
		skipped = skipped || !applied
		offset = next
	}
	if skipped {
		if err := j.AdvanceSpoolOffset(ctx, attempt.AttemptID, offset); err != nil {
			return 0, err
		}
	}
	return j.ProducerSequence(ctx, producer)
}

// readLine returns the next newline-terminated line, or complete=false at a
// torn or absent last line.
func readLine(r *bufio.Reader) (line []byte, complete bool, err error) {
	var buf []byte
	for {
		chunk, err := r.ReadSlice('\n')
		buf = append(buf, chunk...)
		if len(buf) > maxSpoolLine {
			return nil, false, fmt.Errorf("%w: line exceeds %d bytes", ErrCorruptSpool, maxSpoolLine)
		}
		switch {
		case err == nil:
			return buf, true, nil
		case errors.Is(err, bufio.ErrBufferFull):
		case errors.Is(err, io.EOF):
			return nil, false, nil
		default:
			return nil, false, err
		}
	}
}

func decodeSpoolLine(line []byte, attempt AttemptRef, producer string) (journal.Event, error) {
	dec := json.NewDecoder(bytes.NewReader(line))
	dec.DisallowUnknownFields()
	var ev journal.Event
	if err := dec.Decode(&ev); err != nil {
		return journal.Event{}, fmt.Errorf("%w: %w", ErrCorruptSpool, err)
	}
	switch {
	case ev.RunID != attempt.RunID, ev.AttemptID != attempt.AttemptID, ev.ProducerID != producer:
		return journal.Event{}, fmt.Errorf("%w: event %s belongs to run %q, attempt %q, producer %q",
			ErrCorruptSpool, ev.EventID, ev.RunID, ev.AttemptID, ev.ProducerID)
	case !slices.Contains(workerEvents, ev.Type):
		return journal.Event{}, fmt.Errorf("%w: a worker does not emit %q", ErrCorruptSpool, ev.Type)
	}
	ev.RunSequence = 0 // assigned by Append
	return ev, nil
}

// projectWorkerEvent applies a spooled event's projection update inside tx.
// A worker's attempt state changes obey the same state machine as the
// supervisor's (design §4).
func projectWorkerEvent(ctx context.Context, tx *sql.Tx, ev journal.Event) error {
	switch ev.Type {
	case "attempt.state_changed":
		var p struct {
			State  string  `json:"state"`
			Reason *string `json:"reason"`
		}
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			return fmt.Errorf("%w: %s payload: %w", ErrCorruptSpool, ev.Type, err)
		}
		reason := ""
		if p.Reason != nil {
			reason = *p.Reason
		}
		return setAttemptState(ctx, tx, ev.AttemptID, AttemptState(p.State), reason)
	case "attempt.launched":
		var p struct {
			WorkerPID       int       `json:"worker_pid"`
			WorkerStartTime time.Time `json:"worker_start_time"`
			NativePID       *int      `json:"native_pid"`
			NativePGID      *int      `json:"native_pgid"`
		}
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			return fmt.Errorf("%w: %s payload: %w", ErrCorruptSpool, ev.Type, err)
		}
		return journal.SetAttemptProcesses(ctx, tx, ev.AttemptID, journal.LaunchedProcesses(p))
	case "attempt.native_session":
		var p struct {
			SessionID string `json:"session_id"`
		}
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			return fmt.Errorf("%w: %s payload: %w", ErrCorruptSpool, ev.Type, err)
		}
		return journal.SetNativeSession(ctx, tx, ev.AttemptID, p.SessionID)
	case "attempt.native_result":
		return projectUsage(ctx, tx, ev)
	}
	return nil
}

const retailEquivalent = "retail-equivalent"

// plainDecimal is the quantity text the ledger stores: the decoder may pass
// other JSON number forms, which stay unreported rather than being converted.
var plainDecimal = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?$`)

// projectUsage turns a native_result into usage rows inside the ingest
// transaction (budget-ledger-s1 design §3). Every quantity is this attempt's
// own increment, so readings are deltas carrying the event's identity.
// Rows of earlier events are the applied set; a scope with nothing reported
// is stored as an unknown marker, never as zero (I09 (v2 §7.3)).
func projectUsage(ctx context.Context, tx *sql.Tx, ev journal.Event) error {
	var p struct {
		Usage map[string]adapter.TokenUsage `json:"usage_native_reported"`
		Cost  *string                       `json:"retail_equivalent_estimate_usd"`
	}
	if err := json.Unmarshal(ev.Payload, &p); err != nil {
		return fmt.Errorf("%w: %s payload: %w", ErrCorruptSpool, ev.Type, err)
	}
	harness, err := admittedHarness(ctx, tx, ev.RunID)
	if err != nil {
		return err
	}

	reading := func(scope, unit, quantity string) billing.Reading {
		return billing.Reading{Scope: scope, Unit: unit, Source: "native-reported", At: ev.ObservedAt, Delta: &quantity,
			Producer: ev.ProducerID, Sequence: ev.ProducerSequence, HasIdentity: true}
	}
	expected := []string{retailEquivalent}
	var readings []billing.Reading
	for _, model := range slices.Sorted(maps.Keys(p.Usage)) {
		expected = append(expected, model)
		if total := billing.SplitUsage(harness, p.Usage[model]).Total; total != nil {
			readings = append(readings, reading(model, "tokens", *total))
		}
	}
	if p.Cost != nil && plainDecimal.MatchString(*p.Cost) {
		readings = append(readings, reading(retailEquivalent, "USD", *p.Cost))
	}

	existing, err := journal.UsageObservationsTx(ctx, tx, ev.RunID)
	if err != nil {
		return err
	}
	applied := map[billing.EventID]bool{}
	for _, o := range existing {
		if o.ProducerID != "" {
			applied[billing.EventID{Producer: o.ProducerID, Sequence: o.ProducerSequence}] = true
		}
	}
	n, err := billing.Normalize(readings, expected, applied) //nolint:misspell // the spec names billing.Normalize
	if err != nil {
		return fmt.Errorf("projecting usage of %s: %w", ev.EventID, err)
	}

	for _, k := range slices.SortedFunc(maps.Keys(n.Scopes), func(a, b billing.ScopeKey) int {
		return cmp.Or(cmp.Compare(a.Scope, b.Scope), cmp.Compare(a.Unit, b.Unit), cmp.Compare(a.Source, b.Source))
	}) {
		st := n.Scopes[k]
		row := journal.UsageRow{ObservationID: ids.New("obs"), RunID: ev.RunID, Scope: k.Scope, Unit: k.Unit, Source: k.Source,
			Label: string(st.Label), Quantity: "unknown", ObservedAt: ev.ObservedAt.UTC().Format(time.RFC3339Nano)}
		switch {
		case st.Total != nil:
			row.Quantity, row.ProducerID, row.ProducerSequence = *st.Total, ev.ProducerID, ev.ProducerSequence
		case k.Unit != "":
			continue // every delta of this identity was already applied
		}
		if err := journal.InsertUsageObservation(ctx, tx, row); err != nil {
			return err
		}
	}
	return nil
}

// admittedHarness is the harness ID of the run's journaled admission record,
// the key SplitUsage maps components by. A run with no admission record has
// an unmapped route.
func admittedHarness(ctx context.Context, tx *sql.Tx, runID string) (string, error) {
	var payload string
	err := tx.QueryRowContext(ctx, `SELECT payload FROM journal WHERE run_id = ? AND type = 'admission.decided' ORDER BY run_sequence DESC LIMIT 1`, runID).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("reading the admission record of %s: %w", runID, err)
	}
	var adm struct {
		Adapter struct {
			Harness string `json:"harness"`
		} `json:"adapter"`
	}
	if err := json.Unmarshal([]byte(payload), &adm); err != nil {
		return "", fmt.Errorf("decoding the admission record of %s: %w", runID, err)
	}
	return adm.Adapter.Harness, nil
}

// setAttemptState moves attemptID to state to inside tx, if the state
// machine allows it.
func setAttemptState(ctx context.Context, tx *sql.Tx, attemptID string, to AttemptState, reason string) error {
	if err := checkReason(string(to), reason); err != nil {
		return err
	}
	from, err := journal.CurrentAttemptState(ctx, tx, attemptID)
	if err != nil {
		return err
	}
	if !slices.Contains(attemptTransitions[AttemptState(from)], to) {
		return fmt.Errorf("%w: attempt %s cannot move from %s to %s", ErrIllegalTransition, attemptID, from, to)
	}
	return journal.SetAttemptState(ctx, tx, attemptID, string(to), reason)
}
