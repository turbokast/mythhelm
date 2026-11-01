// Package viewmodel is the TUI's one read seam over the run pipeline: a
// Snapshot built from read-only journal projections plus the receipt, so
// every pane renders one consistent picture.
package viewmodel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/workers"
)

// Progress is the latest attempt.progress counters (native-reported).
type Progress struct {
	AssistantTurns int
	ToolUses       map[string]int
	Retries        int
	RunSeq         int64 // run_sequence of the event it came from
}

// NativeExit is the latest attempt.native_result observation. ExitCode is
// nil when unreported or signalled; Signal is nil when unreported or a
// clean exit (I09: absent, never zero).
type NativeExit struct {
	ExitCode       *int
	Signal         *string
	ResultObserved bool
	RunSeq         int64 // run_sequence of the event it came from
}

// Admission is the latest admission.decided observation.
type Admission struct {
	WorkerDetachment string
	AdapterID        string // admission.decided adapter.id
	AdapterVersion   string // adapter.version
	AdapterSurface   string // adapter.surface
	Qualified        bool   // billing.qualified
	PaidContinuation string // billing.paid_continuation ("off"/"unknown"/...)
	// EntitlementStatement is admission.EntitlementStatement of the recorded
	// billing posture: the verbatim sentence the status strip prints.
	EntitlementStatement string
	RunSeq               int64 // run_sequence of the event it came from
}

// Snapshot is one consistent view of a run. Pointer fields stay nil until
// their part projects; Goal is "unknown" until a verified title resolves.
type Snapshot struct {
	Model              string
	Usage              []journal.UsageRow
	AgentEvents        []journal.Event
	AgentEventsOmitted int
	Supervision        string
	Run                journal.RunRow
	Attempt            *journal.AttemptRow
	Candidate          *journal.CandidateRow
	Verification       *journal.VerificationRow
	Receipt            supervisor.Receipt
	Goal               string
	LatestProgress     *Progress
	NativeExit         *NativeExit
	Admission          *Admission
	LastRunSeq         int64
	At                 time.Time
}

// Load builds runID's snapshot from the state dir's projections, event scan
// and receipt. A missing run wraps journal.ErrNotFound; a state dir with no
// database reports journal.ErrNoDatabase. The database is opened read-only:
// this package never writes the journal (I18).
func Load(ctx context.Context, dir, runID string) (Snapshot, error) {
	return load(ctx, systemTaskReader(), dir, runID)
}

// load is Load over the task reader seam. A receipt-backed run never reads
// the task file. Otherwise the title comes from the task file, digest-checked
// against the run projection the snapshot finally carries.
func load(ctx context.Context, r taskReader, dir, runID string) (Snapshot, error) {
	j, err := journal.OpenReadOnly(ctx, dir)
	if err != nil {
		return Snapshot{}, err
	}
	defer func() { _ = j.Close() }()
	snap, receipt, err := assemble(ctx, j, dir, runID)
	if err != nil || receipt {
		return snap, err
	}
	b, waited, readErr := r.readFile(ctx, filepath.Join(dir, "runs", runID, "task.md"))
	if errors.Is(readErr, context.Canceled) || errors.Is(readErr, context.DeadlineExceeded) {
		return Snapshot{}, readErr
	}
	if waited {
		// The run may have moved on while the read waited: rebuild once, so
		// the snapshot and the digest check reflect the journal after it.
		snap, receipt, err = assemble(ctx, j, dir, runID)
		if err != nil || receipt {
			return snap, err
		}
	}
	goal, err := verifiedTaskGoal(runID, snap.Run.TaskSHA256, b, readErr)
	if err != nil {
		return Snapshot{}, err
	}
	snap.Goal = goal
	return snap, nil
}

// assemble builds runID's snapshot from the projections, the event scan and,
// when the scan saw receipt.written, the receipt; receipt reports that the
// goal came from it.
func assemble(ctx context.Context, j *journal.Journal, dir, runID string) (Snapshot, bool, error) {
	snap := Snapshot{Goal: "unknown", Model: "unknown", Supervision: "unknown", At: time.Now().UTC()}
	run, err := j.Run(ctx, runID)
	if err != nil {
		return Snapshot{}, false, err
	}
	snap.Run = run
	snap.Usage, err = j.UsageObservations(ctx, runID)
	if err != nil {
		return Snapshot{}, false, err
	}
	if attempt, err := j.LatestAttempt(ctx, runID); err == nil {
		snap.Attempt = &attempt
		at, _, known := workers.ReadSupervisorBeat(workers.AttemptDir(dir, runID, attempt.AttemptID))
		// No qualified read-only owner probe exists; absence is never ownership evidence.
		snap.Supervision = classifySupervision(at, snap.At, known, "unknown")
		if candidate, err := j.Candidate(ctx, attempt.AttemptID); err == nil {
			snap.Candidate = &candidate
		} else if !errors.Is(err, journal.ErrNotFound) {
			return Snapshot{}, false, err
		}
	} else if !errors.Is(err, journal.ErrNotFound) {
		return Snapshot{}, false, err
	}
	if verification, err := j.LatestVerification(ctx, runID); err == nil {
		snap.Verification = &verification
	} else if !errors.Is(err, journal.ErrNotFound) {
		return Snapshot{}, false, err
	}
	events, err := j.Events(ctx, runID, 0)
	if err != nil {
		return Snapshot{}, false, err
	}
	// Attempt evidence belongs to the latest attempt only: a retry must not
	// inherit its predecessor's progress or result as launch evidence.
	// Unattributed events (synthetic feeds) still fold; without a latest
	// attempt there is nothing to scope to, so everything folds.
	currentAttempt := ""
	if snap.Attempt != nil {
		currentAttempt = snap.Attempt.AttemptID
	}
	stale := func(ev journal.Event) bool {
		return currentAttempt != "" && ev.AttemptID != "" && ev.AttemptID != currentAttempt
	}
	receiptWritten := false
	for _, ev := range events {
		if ev.RunSequence > snap.LastRunSeq {
			snap.LastRunSeq = ev.RunSequence
		}

		if currentAttempt != "" && ev.AttemptID == currentAttempt {
			if len(snap.AgentEvents) == 200 {
				copy(snap.AgentEvents, snap.AgentEvents[1:])
				snap.AgentEvents[199] = ev
				snap.AgentEventsOmitted++
			} else {
				if snap.AgentEvents == nil {
					snap.AgentEvents = make([]journal.Event, 0, 200)
				}
				snap.AgentEvents = append(snap.AgentEvents, ev)
			}
		}
		switch ev.Type {
		case "attempt.native_session":
			if currentAttempt == "" || ev.AttemptID != currentAttempt {
				continue
			}
			var session struct {
				Model string `json:"model"`
			}
			if err := json.Unmarshal(ev.Payload, &session); err != nil {
				return Snapshot{}, false, fmt.Errorf("viewmodel: event %s at run_sequence %d is malformed: %w", ev.Type, ev.RunSequence, err)
			}
			snap.Model = session.Model
			if snap.Model == "" {
				snap.Model = "unknown"
			}

		case "attempt.progress":
			// Progress is advisory: a malformed payload is skipped with
			// the latest good value kept.
			if !stale(ev) {
				if p, err := decodeProgress(ev); err == nil {
					snap.LatestProgress = p
				}
			}
		case "attempt.native_result":
			if stale(ev) {
				continue
			}
			native, err := decodeNativeExit(ev)
			if err != nil {
				return Snapshot{}, false, err
			}
			snap.NativeExit = native
		case "admission.decided":
			adm, err := decodeAdmission(ev)
			if err != nil {
				return Snapshot{}, false, err
			}
			snap.Admission = adm
		case "receipt.written":
			receiptWritten = true
		}
	}
	// ReadReceipt reports a missing file as a wrapped read error, never
	// ErrNotFound, so it runs only when the scan saw receipt.written.
	if receiptWritten {
		r, _, err := supervisor.ReadReceipt(ctx, j, runID)
		if err != nil {
			return Snapshot{}, false, err
		}
		if r["state"] != run.State {
			return Snapshot{}, false, fmt.Errorf("receipt for %s does not match its run projection", runID)
		}
		snap.Receipt = r
		if title := receiptTitle(r); title != "" {
			snap.Goal = title
		}
		return snap, true, nil
	}
	return snap, false, nil
}

// EventsSince returns runID's events after after, in run_sequence order,
// for history search and the accessible stream.
func EventsSince(ctx context.Context, dir, runID string, after int64) ([]journal.Event, error) {
	j, err := journal.OpenReadOnly(ctx, dir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = j.Close() }()
	return j.Events(ctx, runID, after)
}

// EventsSinceLimit is EventsSince capped at limit rows, for paged readers
// that probe for overflow with one extra row.
func EventsSinceLimit(ctx context.Context, dir, runID string, after int64, limit int) ([]journal.Event, error) {
	j, err := journal.OpenReadOnly(ctx, dir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = j.Close() }()
	return j.EventsLimit(ctx, runID, after, limit)
}

func decodeProgress(ev journal.Event) (*Progress, error) {
	var m struct {
		AssistantTurns int            `json:"assistant_turns"`
		ToolUses       map[string]int `json:"tool_uses"`
		Retries        int            `json:"retries"`
	}
	if err := json.Unmarshal(ev.Payload, &m); err != nil {
		return nil, err
	}
	return &Progress{AssistantTurns: m.AssistantTurns, ToolUses: m.ToolUses, Retries: m.Retries, RunSeq: ev.RunSequence}, nil
}

func decodeNativeExit(ev journal.Event) (*NativeExit, error) {
	var m struct {
		ExitCode       *int    `json:"exit_code"`
		Signal         *string `json:"signal"`
		ResultObserved bool    `json:"result_observed"`
	}
	if err := json.Unmarshal(ev.Payload, &m); err != nil {
		return nil, fmt.Errorf("viewmodel: event %s at run_sequence %d is malformed: %w", ev.Type, ev.RunSequence, err)
	}
	return &NativeExit{ExitCode: m.ExitCode, Signal: m.Signal, ResultObserved: m.ResultObserved, RunSeq: ev.RunSequence}, nil
}

func decodeAdmission(ev journal.Event) (*Admission, error) {
	var rec admission.Record
	if err := json.Unmarshal(ev.Payload, &rec); err != nil {
		return nil, fmt.Errorf("viewmodel: event %s at run_sequence %d is malformed: %w", ev.Type, ev.RunSequence, err)
	}
	detachment := string(rec.Capabilities.Platform.WorkerDetachment)
	if detachment == "" {
		detachment = "unknown"
	}
	return &Admission{
		WorkerDetachment: detachment,
		AdapterID:        rec.Adapter.ID, AdapterVersion: rec.Adapter.Version, AdapterSurface: rec.Adapter.Surface,
		Qualified: rec.Billing.Qualified, PaidContinuation: rec.Billing.PaidContinuation,
		EntitlementStatement: admission.EntitlementStatement(rec.Billing), RunSeq: ev.RunSequence,
	}, nil
}

// receiptTitle is the receipt's requested-outcome title, or "" when absent.
func receiptTitle(r supervisor.Receipt) string {
	outcome, _ := r["requested_outcome"].(map[string]any)
	title, _ := outcome["title"].(string)
	return title
}

// verifiedTaskGoal resolves the goal from the task file read for runID: the
// title when its digest matches wantSHA, "unknown" when no file was
// recorded, an error on a read failure or a digest mismatch (I07: the title
// is never unverified).
func verifiedTaskGoal(runID, wantSHA string, b []byte, err error) (string, error) {
	if errors.Is(err, os.ErrNotExist) {
		return "unknown", nil
	}
	if err != nil {
		return "", fmt.Errorf("viewmodel: reading admitted task for %s: %w", runID, err)
	}
	sum := sha256.Sum256(b)
	if hex.EncodeToString(sum[:]) != wantSHA {
		return "", fmt.Errorf("viewmodel: admitted task digest mismatch for %s", runID)
	}
	if title := admission.TaskTitle(b); title != "" {
		return title, nil
	}
	return "unknown", nil
}
