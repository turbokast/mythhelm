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
	AdapterID        string // admission.decided adapter.id
	AdapterVersion   string // adapter.version
	AdapterSurface   string // adapter.surface
	Qualified        bool   // billing.qualified
	PaidContinuation string // billing.paid_continuation ("off"/"unknown"/...)
	RunSeq           int64  // run_sequence of the event it came from
}

// Snapshot is one consistent view of a run. Pointer fields stay nil until
// their part projects; Goal is "unknown" until a verified title resolves.
type Snapshot struct {
	Run            journal.RunRow
	Attempt        *journal.AttemptRow
	Candidate      *journal.CandidateRow
	Verification   *journal.VerificationRow
	Receipt        supervisor.Receipt
	Goal           string
	LatestProgress *Progress
	NativeExit     *NativeExit
	Admission      *Admission
	LastRunSeq     int64
	At             time.Time
}

// Load builds runID's snapshot from the state dir's projections, event scan
// and receipt. A missing run wraps journal.ErrNotFound; a state dir with no
// database reports journal.ErrNoDatabase. The database is opened read-only:
// this package never writes the journal (I18).
func Load(ctx context.Context, dir, runID string) (Snapshot, error) {
	snap := Snapshot{Goal: "unknown", At: time.Now().UTC()}
	j, err := journal.OpenReadOnly(ctx, dir)
	if err != nil {
		return Snapshot{}, err
	}
	defer func() { _ = j.Close() }()
	run, err := j.Run(ctx, runID)
	if err != nil {
		return Snapshot{}, err
	}
	snap.Run = run
	if attempt, err := j.LatestAttempt(ctx, runID); err == nil {
		snap.Attempt = &attempt
		if candidate, err := j.Candidate(ctx, attempt.AttemptID); err == nil {
			snap.Candidate = &candidate
		} else if !errors.Is(err, journal.ErrNotFound) {
			return Snapshot{}, err
		}
	} else if !errors.Is(err, journal.ErrNotFound) {
		return Snapshot{}, err
	}
	if verification, err := j.LatestVerification(ctx, runID); err == nil {
		snap.Verification = &verification
	} else if !errors.Is(err, journal.ErrNotFound) {
		return Snapshot{}, err
	}
	events, err := j.Events(ctx, runID, 0)
	if err != nil {
		return Snapshot{}, err
	}
	receiptWritten := false
	for _, ev := range events {
		if ev.RunSequence > snap.LastRunSeq {
			snap.LastRunSeq = ev.RunSequence
		}
		switch ev.Type {
		case "attempt.progress":
			// Progress is advisory: a malformed payload is skipped with
			// the latest good value kept.
			if p, err := decodeProgress(ev); err == nil {
				snap.LatestProgress = p
			}
		case "attempt.native_result":
			native, err := decodeNativeExit(ev)
			if err != nil {
				return Snapshot{}, err
			}
			snap.NativeExit = native
		case "admission.decided":
			adm, err := decodeAdmission(ev)
			if err != nil {
				return Snapshot{}, err
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
			return Snapshot{}, err
		}
		if r["state"] != run.State {
			return Snapshot{}, fmt.Errorf("receipt for %s does not match its run projection", runID)
		}
		snap.Receipt = r
		if title := receiptTitle(r); title != "" {
			snap.Goal = title
		}
		return snap, nil
	}
	goal, err := taskGoal(dir, runID, run.TaskSHA256)
	if err != nil {
		return Snapshot{}, err
	}
	snap.Goal = goal
	return snap, nil
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
	return &Admission{
		AdapterID: rec.Adapter.ID, AdapterVersion: rec.Adapter.Version, AdapterSurface: rec.Adapter.Surface,
		Qualified: rec.Billing.Qualified, PaidContinuation: rec.Billing.PaidContinuation, RunSeq: ev.RunSequence,
	}, nil
}

// receiptTitle is the receipt's requested-outcome title, or "" when absent.
func receiptTitle(r supervisor.Receipt) string {
	outcome, _ := r["requested_outcome"].(map[string]any)
	title, _ := outcome["title"].(string)
	return title
}

// taskGoal resolves the goal from the digest-checked task file: the title on
// a digest match, "unknown" when no file was recorded, an error on a digest
// mismatch (I07: the title is never unverified).
func taskGoal(dir, runID, wantSHA string) (string, error) {
	b, err := os.ReadFile(filepath.Join(dir, "runs", runID, "task.md")) // #nosec G304 -- fixed name under the state dir and journaled run ID
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
