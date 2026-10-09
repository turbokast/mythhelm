package control

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/v2contract"
	"github.com/turbokast/mythhelm/internal/workers"
)

// CodeExternalEffectUncertain extends the control.go catalogue with the exact
// v2 §4.5 string for the code the recover method returns; control.go predates
// it. (CodeOwnershipUnresolved already lives in stop.go.)
const CodeExternalEffectUncertain Code = "external_effect_uncertain"

// RecoverParams is the params object of the "recover" intent.
type RecoverParams struct {
	RunID string `json:"run_id"`
}

// RecoverOutcome is exactly one of the four v2 §6.4 dispositions.
type RecoverOutcome string

const (
	RecoverReconnected      RecoverOutcome = "reconnected"
	RecoverContinued        RecoverOutcome = "continued"
	RecoverPartialCandidate RecoverOutcome = "partial_candidate"
	RecoverQuarantined      RecoverOutcome = "quarantined"
)

// EventRecoveryDecided is the journal event recording one reconciliation
// pass. It doubles as the one-pass record (design D3): a repeat without
// fresh evidence replays the latest such event instead of re-examining.
const EventRecoveryDecided = "attempt.recovery_decided"

// Quarantine reasons recorded in the report. Stale generation quarantines
// under the ownership_unresolved code — fresh ownership cannot be
// established — with the more specific reason retained.
const (
	reasonOwnershipUnresolved     = "ownership_unresolved"
	reasonStaleGeneration         = "stale_generation"
	reasonExternalEffectUncertain = "external_effect_uncertain"
)

// RecoveryReport is one recorded reconciliation pass: the outcome, what it
// acted on, and the evidence markers it saw. The recover intent returns it
// as the Result body and the outcome event carries the same bytes.
type RecoveryReport struct {
	RunID           string          `json:"run_id"`
	AttemptID       string          `json:"attempt_id"`
	Outcome         RecoverOutcome  `json:"outcome"`
	NewAttemptID    string          `json:"new_attempt_id,omitempty"`
	CandidateCommit string          `json:"candidate_commit,omitempty"`
	Reason          string          `json:"reason,omitempty"`
	Markers         RecoveryMarkers `json:"markers"`
}

// RecoveryMarkers captures every input the examination reads, so a repeat
// without fresh evidence replays the stored outcome instead of running a
// second pass. JournalSeq covers every journal input at once: the journal
// is append-only, so an unchanged maximum over non-recovery events means an
// unchanged event set. LiveNow covers the one input no table holds: whether
// the journaled worker process is still the same live process.
type RecoveryMarkers struct {
	Attempts          []AttemptMark   `json:"attempts"`
	Candidates        []CandidateMark `json:"candidates,omitempty"`
	JournalSeq        int64           `json:"journal_seq"`
	WorkerProducerGen int64           `json:"worker_producer_gen"`
	SpoolBytes        int64           `json:"spool_bytes"`
	IdentityStat      string          `json:"identity_stat"`
	LiveNow           string          `json:"live_now"`
}

// AttemptMark is the recoverable projection of one attempt row.
type AttemptMark struct {
	AttemptID     string  `json:"attempt_id"`
	TaskID        string  `json:"task_id"`
	Number        int64   `json:"attempt_number"`
	State         string  `json:"state"`
	LaunchSHA     string  `json:"launch_sha256"`
	WorkerPID     *int64  `json:"worker_pid"`
	WorkerStart   *string `json:"worker_start_time"`
	WorkspacePath string  `json:"workspace_path"`
	SpoolOffset   int64   `json:"spool_offset"`
}

// CandidateMark is the frozen-candidate projection of one attempt.
type CandidateMark struct {
	AttemptID string `json:"attempt_id"`
	Commit    string `json:"commit"`
	Partial   bool   `json:"partial"`
}

// RecoveryLaunch is one continuation spawn: the admitted attempt, the launch
// identity it carries, and the workspace it continues in.
type RecoveryLaunch struct {
	RunID          string `json:"run_id"`
	AttemptID      string `json:"attempt_id"`
	LaunchIDSHA256 string `json:"launch_id_sha256"`
	WorkspacePath  string `json:"workspace_path"`
}

// RecoverDeps are the recover Handler's dependencies.
type RecoverDeps struct {
	DB       *sql.DB
	StateDir string
	// Launch spawns an admitted continuation. RecoverHandler calls it on a
	// fresh continued outcome; Reconcile never calls it — deciding never
	// launches (I12). A nil Launch fails a fresh continuation closed, so
	// no phantom attempt is left admitted but unspawned.
	Launch func(ctx context.Context, l RecoveryLaunch) error
	// ExamineCount, when non-nil, counts examination passes. Replays never
	// bump it. It is a test seam; production leaves it nil.
	ExamineCount *atomic.Int64
}

// RecoverHandler answers the recover intent: it reconciles the run through
// Reconcile and, on a fresh continued outcome only, launches the admitted
// continuation. Replays of a stored continuation never relaunch.
func RecoverHandler(d RecoverDeps) Handler {
	return func(ctx context.Context, _ Peer, in Intent) (Result, error) {
		var p RecoverParams
		if err := strictParams(in, &p); err != nil {
			return Result{}, err
		}
		if p.RunID == "" {
			return Result{}, newError(CodeInvalidContract, "recover needs run_id")
		}
		if d.DB == nil || d.StateDir == "" {
			return Result{}, newError(CodePersistenceUnavailable, "recover is not available: no ledger is attached")
		}
		rep, fresh, err := reconcile(ctx, d, p.RunID, "evt_recovery_"+in.OperationID)
		if err != nil {
			var ce *Error
			if errors.As(err, &ce) && rep.Outcome != "" {
				// A recorded failure (quarantine): the failure rides in
				// the Result with a nil error, so Execute commits the
				// one-pass record instead of rolling it back (the
				// recordQuarantine pattern). The stored result still
				// replays as the failure.
				body, merr := json.Marshal(rep)
				if merr != nil {
					return Result{}, fmt.Errorf("control: encoding the recovery report: %w", merr)
				}
				return Result{Body: body, Error: ce}, nil
			}
			return Result{}, err
		}
		if fresh && rep.Outcome == RecoverContinued {
			if d.Launch == nil {
				return Result{}, errors.New("control: no launcher is attached for the continuation")
			}
			launch := RecoveryLaunch{RunID: rep.RunID, AttemptID: rep.NewAttemptID,
				LaunchIDSHA256: launchDigest(rep), WorkspacePath: launchWorkspace(rep)}
			if err := d.Launch(ctx, launch); err != nil {
				// Transient: Execute rolls the admission back with the
				// operation, so the operation_id stays retryable and no
				// admitted-but-unspawned attempt survives.
				return Result{}, err
			}
		}
		body, err := json.Marshal(rep)
		if err != nil {
			return Result{}, fmt.Errorf("control: encoding the recovery report: %w", err)
		}
		return Result{Body: body}, nil
	}
}

// Reconcile examines launch intent, process identity, unacknowledged spool
// and outstanding effects for runID, then returns exactly one of the four
// v2 §6.4 outcomes and records the pass in the ledger, so a repeat without
// fresh evidence replays the stored outcome instead of re-running. A
// quarantined outcome returns with its ownership or uncertainty failure;
// refusals (unknown run, forged launch identity) return no outcome and
// record nothing. Reconcile never launches: only RecoverHandler launches,
// on a fresh continued outcome.
func Reconcile(ctx context.Context, d RecoverDeps, runID string) (RecoverOutcome, error) {
	rep, _, err := reconcile(ctx, d, runID, freshRecoveryEventID(runID))
	return rep.Outcome, err
}

// reconcile is the single-tx core both entries share. It gathers every
// input inside one Mutate — which joins Execute's transaction under the
// intent, so concurrent passes serialise on the write lock and the loser
// replays the winner's record — compares markers with the latest recorded
// pass, and either replays it (fresh=false, no examination) or examines,
// records and returns the new outcome (fresh=true).
func reconcile(ctx context.Context, d RecoverDeps, runID, eventID string) (RecoveryReport, bool, error) {
	var out RecoveryReport
	var fresh bool
	var recorded *Error
	err := Mutate(ctx, d.DB, func(tx *sql.Tx) error {
		ev, err := gatherEvidence(ctx, tx, d.StateDir, runID)
		if err != nil {
			return err
		}
		if stored, ok := latestRecoveryReport(tx, runID); ok && shouldReplay(stored, ev) {
			out, fresh = stored, false
			return nil
		}
		if d.ExamineCount != nil {
			d.ExamineCount.Add(1)
		}
		rep, rerr := decideOutcome(ctx, tx, ev, eventID)
		if rerr != nil {
			var ce *Error
			if errors.As(rerr, &ce) && rep.Outcome != "" {
				out, fresh, recorded = rep, true, ce
				return nil
			}
			return rerr
		}
		out, fresh = rep, true
		return nil
	})
	if err != nil {
		// A refusal or a transient failure: nothing was recorded, so
		// there is no outcome to return.
		return RecoveryReport{}, false, err
	}
	if out.Outcome == RecoverQuarantined {
		if fresh {
			return out, fresh, recorded
		}
		// A replayed quarantine mirrors a fresh one: the outcome with
		// its failure, so direct callers cannot mistake it for success.
		return out, false, quarantineError(out.Reason, "", 0, 0)
	}
	return out, fresh, nil
}

// recoveryEvidence is everything one pass reads: the run's attempts and
// candidates, its journal events, the recovered attempt's spool facts and
// observed worker identity, and the markers those inputs reduce to.
type recoveryEvidence struct {
	runID      string
	attempt    AttemptMark
	events     []journal.Event
	pin        *AdmissionPinnedPayload
	launched   *launchedIdentity
	natives    []nativeLaunch
	results    []time.Time
	observed   *workers.Identity
	transient  error // a file read that must abort the pass as retryable
	live       string
	markers    RecoveryMarkers
	spoolFacts []spoolFact
}

// launchedIdentity is the worker identity the latest attempt.launched
// event journals for the recovered attempt.
type launchedIdentity struct {
	pid        int
	start      time.Time
	producer   string
	generation int64
	at         time.Time
}

// nativeLaunch is one launched native process and when it launched.
type nativeLaunch struct {
	at time.Time
}

// spoolFact is one unacknowledged spool line the pass can use: a launched
// event (for its native PID) or a native result (for its time).
type spoolFact struct {
	launched bool
	at       time.Time
	native   bool
}

// recoverableAttempt reports whether state can be the subject of a pass:
// a live, dying or already interrupted/quarantined attempt. Pre-launch and
// terminal attempts have nothing to reconcile.
func recoverableAttempt(state string) bool {
	switch state {
	case "launching", "running", "waiting_native", "waiting_approval",
		"stop_requested", "interrupted", "quarantined":
		return true
	}
	return false
}

// gatherEvidence reads every examination input inside tx: the run and its
// attempts, the run's journal events, the candidates, and — from the
// attempt directory — the spool facts and the observed worker identity,
// plus a live probe of the journaled worker PID.
func gatherEvidence(ctx context.Context, tx *sql.Tx, stateDir, runID string) (*recoveryEvidence, error) {
	var one int
	err := tx.QueryRowContext(ctx, `SELECT 1 FROM runs WHERE run_id = ?`, runID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, newError(CodeInvalidContract, "run %q is unknown", runID)
	}
	if err != nil {
		return nil, newError(CodePersistenceUnavailable, "reading run: %v", err)
	}
	attempts, err := txAttemptMarks(ctx, tx, runID)
	if err != nil {
		return nil, err
	}
	if len(attempts) == 0 {
		return nil, newError(CodeInvalidContract, "run %q has no attempts to recover", runID)
	}
	latest := attempts[len(attempts)-1]
	if !recoverableAttempt(latest.State) {
		return nil, newError(CodeInvalidContract, "run %q attempt %q is %s: nothing to recover",
			runID, latest.AttemptID, latest.State)
	}
	events, err := txEvents(ctx, tx, runID)
	if err != nil {
		return nil, err
	}
	candidates, err := txCandidateMarks(ctx, tx, runID)
	if err != nil {
		return nil, err
	}
	ev := &recoveryEvidence{runID: runID, attempt: latest, events: events}
	for i := range events {
		evt := &events[i]
		if evt.AttemptID != latest.AttemptID {
			continue
		}
		switch evt.Type {
		case EventAdmissionPinned:
			var p AdmissionPinnedPayload
			if err := json.Unmarshal(evt.Payload, &p); err != nil {
				continue
			}
			if p.LadderVersion == "" || p.NonceSHA256 == "" {
				continue
			}
			pinned := p
			ev.pin = &pinned
		case launchedEvent:
			pid, start, native, ok := parseLaunchedPayload(evt.Payload)
			if !ok {
				continue
			}
			ev.launched = &launchedIdentity{pid: pid, start: start,
				producer: evt.ProducerID, generation: evt.Generation, at: evt.ObservedAt}
			if native {
				ev.natives = append(ev.natives, nativeLaunch{at: evt.ObservedAt})
			}
		case "attempt.native_result":
			ev.results = append(ev.results, evt.ObservedAt)
		}
	}
	dir := workers.AttemptDir(stateDir, runID, latest.AttemptID)
	ev.spoolFacts = scanSpoolFacts(filepath.Join(dir, "spool.jsonl"), latest.AttemptID)
	for _, fact := range ev.spoolFacts {
		switch {
		case fact.launched && fact.native:
			ev.natives = append(ev.natives, nativeLaunch{at: fact.at})
		case !fact.launched:
			ev.results = append(ev.results, fact.at)
		}
	}
	observed, terr := workers.ReadIdentity(dir)
	switch {
	case terr == nil:
		ev.observed = &observed
	case errors.Is(terr, os.ErrNotExist):
		// No identity file: the worker never wrote one or it is gone.
		// Liveness alone decides between dead and unverifiable below.
	case isPathError(terr):
		ev.transient = fmt.Errorf("control: reading the worker identity: %w", terr)
	default:
		// A corrupt identity file verifies nothing; like a missing one,
		// it leaves liveness to decide.
	}
	ev.live = probeLiveness(ev.launched)
	gen, err := txProducerGen(ctx, tx, ev.launched)
	if err != nil {
		return nil, err
	}
	var journalSeq int64
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(run_sequence), 0) FROM journal
		WHERE run_id = ? AND type <> ?`, runID, EventRecoveryDecided).Scan(&journalSeq)
	if err != nil {
		return nil, newError(CodePersistenceUnavailable, "reading journal sequence: %v", err)
	}
	ev.markers = RecoveryMarkers{
		Attempts: attempts, Candidates: candidates, JournalSeq: journalSeq,
		WorkerProducerGen: gen, SpoolBytes: statSize(filepath.Join(dir, "spool.jsonl")),
		IdentityStat: statIdentity(filepath.Join(dir, "worker.json")), LiveNow: ev.live,
	}
	return ev, nil
}

func isPathError(err error) bool {
	var pe *os.PathError
	return errors.As(err, &pe)
}

// txAttemptMarks reads the run's attempts ordered by attempt_number.
func txAttemptMarks(ctx context.Context, tx *sql.Tx, runID string) ([]AttemptMark, error) {
	rows, err := tx.QueryContext(ctx, `SELECT attempt_id, task_id, attempt_number, state,
		launch_token_sha256, worker_pid, worker_start_time, workspace_path, spool_offset
		FROM attempts WHERE run_id = ? ORDER BY attempt_number`, runID)
	if err != nil {
		return nil, newError(CodePersistenceUnavailable, "reading attempts: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var out []AttemptMark
	for rows.Next() {
		var m AttemptMark
		var pid sql.NullInt64
		var start sql.NullString
		if err := rows.Scan(&m.AttemptID, &m.TaskID, &m.Number, &m.State, &m.LaunchSHA,
			&pid, &start, &m.WorkspacePath, &m.SpoolOffset); err != nil {
			return nil, newError(CodePersistenceUnavailable, "reading attempts: %v", err)
		}
		if pid.Valid {
			p := pid.Int64
			m.WorkerPID = &p
		}
		if start.Valid {
			s := start.String
			m.WorkerStart = &s
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, newError(CodePersistenceUnavailable, "reading attempts: %v", err)
	}
	return out, nil
}

// txCandidateMarks reads the frozen candidates of the run's attempts.
func txCandidateMarks(ctx context.Context, tx *sql.Tx, runID string) ([]CandidateMark, error) {
	rows, err := tx.QueryContext(ctx, `SELECT c.attempt_id, c.candidate_commit, c.partial
		FROM candidates c JOIN attempts a ON c.attempt_id = a.attempt_id
		WHERE a.run_id = ? ORDER BY c.attempt_id`, runID)
	if err != nil {
		return nil, newError(CodePersistenceUnavailable, "reading candidates: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var out []CandidateMark
	for rows.Next() {
		var m CandidateMark
		var partial int64
		if err := rows.Scan(&m.AttemptID, &m.Commit, &partial); err != nil {
			return nil, newError(CodePersistenceUnavailable, "reading candidates: %v", err)
		}
		m.Partial = partial != 0
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, newError(CodePersistenceUnavailable, "reading candidates: %v", err)
	}
	return out, nil
}

// txEvents reads the run's journal events in run_sequence order through tx,
// so the pass sees the same snapshot its record commits against.
func txEvents(ctx context.Context, tx *sql.Tx, runID string) ([]journal.Event, error) {
	rows, err := tx.QueryContext(ctx, `SELECT event_id, schema_version, run_id, task_id, attempt_id,
		producer_id, producer_sequence, run_sequence, generation, caused_by, observed_at, type, payload
		FROM journal WHERE run_id = ? ORDER BY run_sequence`, runID)
	if err != nil {
		return nil, newError(CodePersistenceUnavailable, "reading journal: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var out []journal.Event
	for rows.Next() {
		var ev journal.Event
		var taskID, attemptID, causedBy sql.NullString
		var observedAt, payload string
		if err := rows.Scan(&ev.EventID, &ev.SchemaVersion, &ev.RunID, &taskID, &attemptID,
			&ev.ProducerID, &ev.ProducerSequence, &ev.RunSequence, &ev.Generation, &causedBy,
			&observedAt, &ev.Type, &payload); err != nil {
			return nil, newError(CodePersistenceUnavailable, "reading journal: %v", err)
		}
		ev.TaskID, ev.AttemptID, ev.CausedBy = taskID.String, attemptID.String, causedBy.String
		parsed, err := time.Parse(time.RFC3339Nano, observedAt)
		if err != nil {
			return nil, newError(CodePersistenceUnavailable, "event %s observed_at: %v", ev.EventID, err)
		}
		ev.ObservedAt = parsed
		ev.Payload = json.RawMessage(payload)
		out = append(out, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, newError(CodePersistenceUnavailable, "reading journal: %v", err)
	}
	return out, nil
}

// txProducerGen reports the stored generation of the worker producer that
// journaled the launch, or -1 when there is nothing to fence against.
func txProducerGen(ctx context.Context, tx *sql.Tx, launched *launchedIdentity) (int64, error) {
	if launched == nil {
		return -1, nil
	}
	var gen int64
	err := tx.QueryRowContext(ctx, `SELECT generation FROM producers WHERE producer_id = ?`,
		launched.producer).Scan(&gen)
	if errors.Is(err, sql.ErrNoRows) {
		return -1, nil
	}
	if err != nil {
		return 0, newError(CodePersistenceUnavailable, "reading producer generation: %v", err)
	}
	return gen, nil
}

// parseLaunchedPayload decodes one attempt.launched payload into the worker
// PID, its start time, and whether a native process launched with it.
// Events with unusable payloads are skipped by the caller, never trusted.
func parseLaunchedPayload(raw json.RawMessage) (pid int, start time.Time, native bool, ok bool) {
	var p struct {
		WorkerPID   int       `json:"worker_pid"`
		WorkerStart time.Time `json:"worker_start_time"`
		NativePID   *int      `json:"native_pid"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return 0, time.Time{}, false, false
	}
	if p.WorkerStart.IsZero() {
		return 0, time.Time{}, false, false
	}
	return p.WorkerPID, p.WorkerStart, p.NativePID != nil && *p.NativePID != 0, true
}

// probeLiveness asks the OS about the journaled worker PID: a "live:"
// marker carrying the probed start time, "gone" for a dead or reused PID,
// or "unknown" when nothing can be proven.
func probeLiveness(launched *launchedIdentity) string {
	if launched == nil {
		return "unknown"
	}
	start, err := workers.ProcessStartTime(launched.pid)
	if errors.Is(err, workers.ErrNoProcess) {
		return "gone"
	}
	if err != nil || !start.Equal(launched.start) {
		if err != nil {
			return "unknown"
		}
		// The PID was reused by another process: the journaled worker
		// is gone, and the stranger is never touched.
		return "gone"
	}
	return "live:" + start.UTC().Format(time.RFC3339Nano)
}

// statSize reports the file's size in bytes, or -1 when it cannot be read.
func statSize(path string) int64 {
	st, err := os.Stat(path)
	if err != nil {
		return -1
	}
	return st.Size()
}

// statIdentity reports the worker.json freshness marker: "missing" when
// absent, "unreadable" when it cannot be statted, else size and mtime.
func statIdentity(path string) string {
	st, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "missing"
	}
	if err != nil {
		return "unreadable"
	}
	return fmt.Sprintf("%d:%d", st.Size(), st.ModTime().UnixNano())
}

// scanSpoolFacts collects the unacknowledged spool lines the pass can use:
// launched events (for a native PID) and native results (for their time).
// A missing or unreadable spool means nothing unacknowledged. Oversize or
// torn lines are skipped, never trusted.
func scanSpoolFacts(spoolPath, attemptID string) []spoolFact {
	data, err := os.ReadFile(spoolPath) //nolint:gosec // G304: fixed spool name under the ledger-resolved attempt directory
	if err != nil {
		return nil
	}
	complete, _ := splitCompleteLines(data)
	var out []spoolFact
	for _, line := range complete {
		if !strings.Contains(string(line), "attempt.launched") && !strings.Contains(string(line), "attempt.native_result") {
			continue
		}
		dec := json.NewDecoder(bytes.NewReader(line))
		dec.DisallowUnknownFields()
		var ev journal.Event
		if err := dec.Decode(&ev); err != nil {
			continue
		}
		if ev.AttemptID != attemptID || ev.ObservedAt.IsZero() {
			continue
		}
		switch ev.Type {
		case launchedEvent:
			_, _, native, ok := parseLaunchedPayload(ev.Payload)
			if !ok {
				continue
			}
			out = append(out, spoolFact{launched: true, at: ev.ObservedAt, native: native})
		case "attempt.native_result":
			out = append(out, spoolFact{at: ev.ObservedAt})
		}
	}
	return out
}

// decideOutcome chooses exactly one outcome from gathered evidence and
// records it with the pass's transitions, in that order:
//
//  1. Effects first (I12): a native launched with no later result,
//     journaled or spooled, is ambiguous and irreversible — quarantine with
//     external_effect_uncertain, never retry by replay.
//  2. Identity and liveness (NFR-5): the same live worker reconnects; a
//     provably gone worker continues or exposes its partial candidate;
//     anything else quarantines as ownership unresolved.
//  3. On the live path only: the observed launch token must reconcile with
//     the admitted one (a forgery refuses with revision_conflict and
//     records nothing), and the journal generation must fence fresh (stale
//     evidence quarantines, retained, never reconnected).
//
// A returned report with a *Error is a recorded failure; a *Error without
// one is a refusal that recorded nothing.
func decideOutcome(ctx context.Context, tx *sql.Tx, ev *recoveryEvidence, eventID string) (RecoveryReport, error) {
	if ev.transient != nil {
		return RecoveryReport{}, ev.transient
	}
	if ambiguousEffect(ev) {
		return recordQuarantineOutcome(ctx, tx, ev, eventID, reasonExternalEffectUncertain, "", 0, 0)
	}
	switch classifyWorker(ev) {
	case workerSameLive:
		return decideLive(ctx, tx, ev, eventID)
	case workerGone:
		return decideGone(ctx, tx, ev, eventID)
	default:
		return recordQuarantineOutcome(ctx, tx, ev, eventID, reasonOwnershipUnresolved, "", 0, 0)
	}
}

type workerClass int

const (
	workerUnverifiable workerClass = iota
	workerSameLive
	workerGone
)

// classifyWorker sorts the recovered attempt's worker: the same live
// worker (journaled and observed identity agree via MatchIdentity and the
// process is live), a provably gone one (the journaled PID is dead or
// reused — lease expiry never proves this, only the probe does), or
// unverifiable (anything else, including a live stranger).
func classifyWorker(ev *recoveryEvidence) workerClass {
	if ev.launched == nil || ev.pin == nil || ev.observed == nil {
		if ev.launched != nil && ev.live == "gone" {
			return workerGone
		}
		return workerUnverifiable
	}
	expected := workers.Identity{PID: ev.launched.pid, StartTime: ev.launched.start, Nonce: ev.pin.NonceSHA256}
	if !workers.MatchIdentity(expected, *ev.observed) {
		if ev.live == "gone" {
			return workerGone
		}
		return workerUnverifiable
	}
	if !strings.HasPrefix(ev.live, "live:") {
		if ev.live == "gone" {
			return workerGone
		}
		return workerUnverifiable
	}
	return workerSameLive
}

// ambiguousEffect reports whether a native process launched with no later
// native result anywhere: an irreversible effect whose outcome is unknown.
// Settled evidence counts journaled or spooled — a result the worker saw
// but ingest has not journaled yet still settles the effect.
func ambiguousEffect(ev *recoveryEvidence) bool {
	var latest *time.Time
	for i := range ev.natives {
		at := ev.natives[i].at
		if latest == nil || at.After(*latest) {
			cp := at
			latest = &cp
		}
	}
	if latest == nil {
		return false
	}
	for _, res := range ev.results {
		if res.After(*latest) {
			return false
		}
	}
	return true
}

// decideLive reconciles a live worker that matched identity: its launch
// token must equal the admitted one, and its journal generation must be
// fresh. Either failure quarantines or refuses; both passing reconnects.
func decideLive(ctx context.Context, tx *sql.Tx, ev *recoveryEvidence, eventID string) (RecoveryReport, error) {
	observed := tokenDigest(ev.observed.LaunchToken)
	if err := v2contract.CheckReconcileIdentity(ev.attempt.LaunchSHA, observed); err != nil {
		return RecoveryReport{}, newError(CodeRevisionConflict,
			"run %q attempt %q claims a launch identity that does not match the admitted one",
			ev.runID, ev.attempt.AttemptID)
	}
	current := ev.launched.generation
	if ev.markers.WorkerProducerGen >= 0 {
		current = ev.markers.WorkerProducerGen
	}
	if err := v2contract.CheckGeneration(current, ev.launched.generation); err != nil {
		return recordQuarantineOutcome(ctx, tx, ev, eventID, reasonStaleGeneration,
			ev.launched.producer, ev.launched.generation, current)
	}
	return recordOutcome(ctx, tx, ev, eventID, RecoveryReport{
		RunID: ev.runID, AttemptID: ev.attempt.AttemptID,
		Outcome: RecoverReconnected, Markers: ev.markers,
	})
}

// decideGone reconciles a provably dead worker: a partial frozen candidate
// is exposed, else a continuation is admitted carrying the same launch
// identity. Neither path launches; the handler launches a fresh
// continuation after the record commits.
func decideGone(ctx context.Context, tx *sql.Tx, ev *recoveryEvidence, eventID string) (RecoveryReport, error) {
	for _, c := range ev.markers.Candidates {
		if c.AttemptID == ev.attempt.AttemptID && c.Partial {
			if err := interruptIfDirect(ctx, tx, ev.attempt.AttemptID,
				ev.attempt.State, string(RecoverPartialCandidate)); err != nil {
				return RecoveryReport{}, err
			}
			return recordOutcome(ctx, tx, ev, eventID, RecoveryReport{
				RunID: ev.runID, AttemptID: ev.attempt.AttemptID,
				Outcome: RecoverPartialCandidate, CandidateCommit: c.Commit, Markers: ev.markers,
			})
		}
	}
	next := ev.attempt.AttemptID + "-c" + strconv.FormatInt(ev.attempt.Number+1, 10)
	cont := AttemptMark{
		AttemptID: next, TaskID: ev.attempt.TaskID, Number: ev.attempt.Number + 1,
		State: string(v2contract.AttemptLaunching), LaunchSHA: ev.attempt.LaunchSHA,
		WorkspacePath: ev.attempt.WorkspacePath,
	}
	if err := journal.InsertAttempt(ctx, tx, journal.AttemptRow{
		AttemptID: cont.AttemptID, RunID: ev.runID, TaskID: cont.TaskID,
		AttemptNumber: cont.Number, State: cont.State,
		LaunchTokenSHA256: cont.LaunchSHA, WorkspacePath: cont.WorkspacePath,
	}); err != nil {
		return RecoveryReport{}, newError(CodePersistenceUnavailable, "admitting the continuation: %v", err)
	}
	if err := interruptIfDirect(ctx, tx, ev.attempt.AttemptID,
		ev.attempt.State, string(RecoverContinued)); err != nil {
		return RecoveryReport{}, err
	}
	return recordOutcome(ctx, tx, ev, eventID, RecoveryReport{
		RunID: ev.runID, AttemptID: ev.attempt.AttemptID,
		Outcome: RecoverContinued, NewAttemptID: next, Markers: ev.markers,
	})
}

// recordQuarantineOutcome moves the attempt to quarantined and records the
// pass. It returns the recorded report with its failure, so callers commit
// the record through the failure-in-Result pattern.
func recordQuarantineOutcome(ctx context.Context, tx *sql.Tx, ev *recoveryEvidence, eventID, reason, producer string, got, current int64) (RecoveryReport, error) {
	if err := quarantineAttempt(ctx, tx, ev.attempt.AttemptID, ev.attempt.State, reason); err != nil {
		return RecoveryReport{}, err
	}
	rep, err := recordOutcome(ctx, tx, ev, eventID, RecoveryReport{
		RunID: ev.runID, AttemptID: ev.attempt.AttemptID,
		Outcome: RecoverQuarantined, Reason: reason, Markers: ev.markers,
	})
	if err != nil {
		return RecoveryReport{}, err
	}
	return rep, quarantineError(reason, producer, got, current)
}

// quarantineError maps a recorded quarantine reason to its failure.
func quarantineError(reason, producer string, got, current int64) *Error {
	switch reason {
	case reasonExternalEffectUncertain:
		return newError(CodeExternalEffectUncertain,
			"an irreversible effect has no known outcome; recovery stops instead of replaying it")
	case reasonStaleGeneration:
		return newError(CodeOwnershipUnresolved,
			"producer %s journaled at generation %d but is at %d: the evidence is stale, not fresh ownership",
			producer, got, current)
	default:
		return newError(CodeOwnershipUnresolved, "the worker identity cannot be verified")
	}
}

// quarantineAttempt moves the attempt to quarantined through checked
// transitions: directly where the lifecycle allows, else hopping through
// interrupted. Quarantined restates itself (the table's explicit row).
func quarantineAttempt(ctx context.Context, tx *sql.Tx, attemptID, cur, reason string) error {
	from := v2contract.AttemptState(cur)
	target := v2contract.AttemptQuarantined
	if from == target {
		return setState(ctx, tx, attemptID, target, reason)
	}
	if v2contract.CheckAttemptTransition(from, target) == nil {
		return setState(ctx, tx, attemptID, target, reason)
	}
	mid := v2contract.AttemptInterrupted
	if err := v2contract.CheckAttemptTransition(from, mid); err != nil {
		return newError(CodeRevisionConflict, "attempt %q cannot move from %s to quarantined", attemptID, cur)
	}
	if err := v2contract.CheckAttemptTransition(mid, target); err != nil {
		return newError(CodeRevisionConflict, "attempt %q cannot move from %s to quarantined", attemptID, cur)
	}
	if err := setState(ctx, tx, attemptID, mid, reason); err != nil {
		return err
	}
	return setState(ctx, tx, attemptID, target, reason)
}

// interruptIfDirect concludes the recovered attempt as interrupted where
// the lifecycle allows a direct hop, and leaves it otherwise: a
// quarantined attempt keeps its verdict while the outcome records what the
// new pass found.
func interruptIfDirect(ctx context.Context, tx *sql.Tx, attemptID, cur, reason string) error {
	from := v2contract.AttemptState(cur)
	if from == v2contract.AttemptInterrupted {
		return nil
	}
	if v2contract.CheckAttemptTransition(from, v2contract.AttemptInterrupted) != nil {
		return nil
	}
	return setState(ctx, tx, attemptID, v2contract.AttemptInterrupted, reason)
}

func setState(ctx context.Context, tx *sql.Tx, attemptID string, to v2contract.AttemptState, reason string) error {
	if err := journal.SetAttemptState(ctx, tx, attemptID, string(to), reason); err != nil {
		return newError(CodePersistenceUnavailable, "recording %s: %v", to, err)
	}
	return nil
}

// recordOutcome journals the report as the run's recovery_decided event in
// the same transaction as the pass's projections (design D3), following
// the migrate import's in-tx insert: producer advance, run-sequence
// assignment, insert, producer upsert. Recovery events carry generation 1;
// nothing fences on them.
func recordOutcome(ctx context.Context, tx *sql.Tx, ev *recoveryEvidence, eventID string, rep RecoveryReport) (RecoveryReport, error) {
	// The record describes the world as the pass leaves it: re-read the
	// attempts after the pass's transitions and admission, so the pass's
	// own writes never count as fresh evidence on the next pass. Nothing
	// else the pass writes affects the markers: the outcome event is
	// excluded from JournalSeq and lands on its own producer row.
	marks, err := txAttemptMarks(ctx, tx, ev.runID)
	if err != nil {
		return RecoveryReport{}, err
	}
	rep.Markers.Attempts = marks
	payload, err := json.Marshal(rep)
	if err != nil {
		return RecoveryReport{}, fmt.Errorf("control: encoding the recovery report: %w", err)
	}
	producer := "ctl_recover_" + ev.runID
	var lastSeq, generation int64
	err = tx.QueryRowContext(ctx, `SELECT last_sequence, generation FROM producers WHERE producer_id = ?`,
		producer).Scan(&lastSeq, &generation)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		lastSeq, generation = 0, 1
	case err != nil:
		return RecoveryReport{}, newError(CodePersistenceUnavailable, "reading producer: %v", err)
	case generation < 1:
		generation = 1
	}
	var runSeq int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(run_sequence), 0) + 1 FROM journal WHERE run_id = ?`,
		ev.runID).Scan(&runSeq); err != nil {
		return RecoveryReport{}, newError(CodePersistenceUnavailable, "assigning run sequence: %v", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO journal (event_id, schema_version, run_id, task_id, attempt_id,
		producer_id, producer_sequence, run_sequence, generation, caused_by, observed_at, type, payload)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		eventID, journal.EnvelopeVersion, ev.runID, nullStr(ev.attempt.TaskID), nullStr(ev.attempt.AttemptID),
		producer, lastSeq+1, runSeq, generation, nullStr(eventID),
		time.Now().UTC().Format(time.RFC3339Nano), EventRecoveryDecided, string(payload)); err != nil {
		return RecoveryReport{}, newError(CodePersistenceUnavailable, "inserting event %s: %v", eventID, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO producers (producer_id, last_sequence, generation) VALUES (?, ?, ?)
		ON CONFLICT (producer_id) DO UPDATE SET last_sequence = excluded.last_sequence, generation = excluded.generation`,
		producer, lastSeq+1, generation); err != nil {
		return RecoveryReport{}, newError(CodePersistenceUnavailable, "advancing producer %s: %v", producer, err)
	}
	return rep, nil
}

// latestRecoveryReport returns the run's latest recorded pass. A corrupt
// payload counts as no record: the pass re-examines rather than replaying
// bytes it cannot read.
func latestRecoveryReport(tx *sql.Tx, runID string) (RecoveryReport, bool) {
	var payload string
	err := tx.QueryRow(`SELECT payload FROM journal WHERE run_id = ? AND type = ?
		ORDER BY run_sequence DESC LIMIT 1`, runID, EventRecoveryDecided).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return RecoveryReport{}, false
	}
	if err != nil {
		return RecoveryReport{}, false
	}
	var rep RecoveryReport
	if err := json.Unmarshal([]byte(payload), &rep); err != nil {
		return RecoveryReport{}, false
	}
	if rep.Outcome == "" || rep.AttemptID == "" {
		return RecoveryReport{}, false
	}
	return rep, true
}

// shouldReplay reports whether the stored pass still stands. Same target
// and unchanged markers replay; anything else re-examines — except our
// own continuation: while the admitted attempt has journaled nothing, it
// stands as admitted however its files look, so a fresh pass never
// quarantines a newborn for having no evidence yet. Its first journaled
// event opens its first examination.
func shouldReplay(stored RecoveryReport, ev *recoveryEvidence) bool {
	if stored.AttemptID == ev.attempt.AttemptID {
		return markersEqual(stored.Markers, ev.markers)
	}
	if stored.Outcome == RecoverContinued && stored.NewAttemptID == ev.attempt.AttemptID {
		for i := range ev.events {
			if ev.events[i].AttemptID == ev.attempt.AttemptID {
				return false
			}
		}
		return true
	}
	return false
}

// markersEqual reports whether the pass sees exactly the evidence the
// stored pass saw, by comparing canonical JSON.
func markersEqual(a, b RecoveryMarkers) bool {
	ra, err := json.Marshal(a)
	if err != nil {
		return false
	}
	rb, err := json.Marshal(b)
	if err != nil {
		return false
	}
	return bytes.Equal(ra, rb)
}

// launchDigest and launchWorkspace recover the continuation's launch
// inputs from the recorded report: the continuation carries the recovered
// attempt's launch digest and workspace.
func launchDigest(rep RecoveryReport) string {
	for _, m := range rep.Markers.Attempts {
		if m.AttemptID == rep.AttemptID {
			return m.LaunchSHA
		}
	}
	return ""
}

func launchWorkspace(rep RecoveryReport) string {
	for _, m := range rep.Markers.Attempts {
		if m.AttemptID == rep.AttemptID {
			return m.WorkspacePath
		}
	}
	return ""
}

func freshRecoveryEventID(runID string) string {
	var nonce [4]byte
	_, _ = rand.Read(nonce[:])
	return fmt.Sprintf("evt_recovery_%s_%s", runID, hex.EncodeToString(nonce[:]))
}

func nullStr(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}
