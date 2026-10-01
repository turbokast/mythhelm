package workers

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"
)

// Event types the worker writes to its spool (design §5).
const (
	evStateChanged     = "attempt.state_changed"
	evLaunched         = "attempt.launched"
	evNativeSession    = "attempt.native_session"
	evProgress         = "attempt.progress"
	evPermissionDenied = "attempt.permission_denied"
	evNativeError      = "attempt.native_error"
	evProtocolCounters = "attempt.protocol_counters"
	evNativeResult     = "attempt.native_result"
	evStopRequested    = "attempt.stop_requested"
	evStopped          = "attempt.stopped"
)

// workerGeneration is the worker producer's generation. An attempt has
// exactly one worker, which is never relaunched (I12).
const workerGeneration = 1

// critical reports whether an event must reach the disk before the worker
// continues. Only coalesced progress and diagnostics counters may be lost
// in a crash (§7.6).
func critical(typ string) bool {
	return typ != evProgress && typ != evProtocolCounters
}

// spool is the worker's durable outbox: one journal.Event per line, in
// producer_sequence order starting at 1. The supervisor that holds the
// run's owner lock ingests it into the journal; the worker never opens the
// database.
type spool struct {
	f               *os.File
	sync            func(*os.File) error // the fsync seam; (*os.File).Sync outside tests
	runID           string
	taskID          string
	attemptID       string
	producerID      string
	seq             int64
	terminalWritten bool // terminal transition may already have been ingested
}

// createSpool creates the attempt's spool. It fails if the spool exists, so
// a second worker for the same attempt never launches a second native
// process (I12, I18).
func createSpool(path, runID, taskID, attemptID string) (*spool, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL|os.O_APPEND, 0o600) //nolint:gosec // G304: path is inside the attempt directory
	if err != nil {
		return nil, fmt.Errorf("creating the spool (a worker launches once per attempt): %w", err)
	}
	return &spool{
		f:          f,
		sync:       (*os.File).Sync,
		runID:      runID,
		taskID:     taskID,
		attemptID:  attemptID,
		producerID: "wrk_" + attemptID,
	}, nil
}

// emit appends one event and, for a critical event, syncs it to disk.
func (s *spool) emit(typ string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encoding %s: %w", typ, err)
	}
	line, err := json.Marshal(journal.Event{
		SchemaVersion:    journal.EnvelopeVersion,
		EventID:          ids.New("evt"),
		RunID:            s.runID,
		TaskID:           s.taskID,
		AttemptID:        s.attemptID,
		ProducerID:       s.producerID,
		ProducerSequence: s.seq + 1,
		Generation:       workerGeneration,
		ObservedAt:       time.Now().UTC(),
		Type:             typ,
		Payload:          raw,
	})
	if err != nil {
		return fmt.Errorf("encoding %s: %w", typ, err)
	}
	if _, err := s.f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("writing %s to the spool: %w", typ, err)
	}
	s.seq++
	if typ == evStateChanged {
		if m, ok := payload.(map[string]any); ok {
			state, _ := m["state"].(string)
			if state != "launching" && state != "running" && state != "stop_requested" {
				s.terminalWritten = true
			}
		}
	}
	if critical(typ) {
		if err := s.sync(s.f); err != nil {
			return fmt.Errorf("syncing %s: %w", typ, err)
		}
	}
	return nil
}

func (s *spool) close() error {
	return s.f.Close()
}
