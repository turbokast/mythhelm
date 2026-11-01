package supervisor

import (
	"context"
	"encoding/json"
	"time"

	"github.com/turbokast/mythhelm/internal/control"
	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/v2contract"
	"github.com/turbokast/mythhelm/internal/workers"
)

// DiagnosticCode is the closed operational evidence vocabulary. It is never a
// display hint, public stream type, raw error or native transcript.
type DiagnosticCode string

const (
	DiagnosticQuotaUnavailable               DiagnosticCode = "quota_unavailable"
	DiagnosticWorkerIdentityUnresolved       DiagnosticCode = "worker_identity_unresolved"
	DiagnosticWorkerExitedEarly              DiagnosticCode = "worker_exited_early"
	DiagnosticSpoolIngestionFailed           DiagnosticCode = "spool_ingestion_failed"
	DiagnosticCandidateFreezeFailed          DiagnosticCode = "candidate_freeze_failed"
	DiagnosticVerificationUnavailable        DiagnosticCode = "verification_unavailable"
	DiagnosticVerificationBindingUnavailable DiagnosticCode = "verification_binding_unavailable"
	DiagnosticStopRequested                  DiagnosticCode = "stop_requested"
	DiagnosticDeadlineReached                DiagnosticCode = "deadline_reached"
	DiagnosticRecoveryOwnershipUnresolved    DiagnosticCode = "recovery_ownership_unresolved"
)
const serviceDiagnosticEventType = "service.diagnostic"

type diagnostic struct {
	Code         DiagnosticCode `json:"code"`
	RunID        string         `json:"run_id"`
	AttemptID    string         `json:"attempt_id,omitempty"`
	Generation   int64          `json:"generation"`
	EvidenceRefs []string       `json:"evidence_refs"`
}

func validDiagnosticCode(code DiagnosticCode) bool {
	switch code {
	case DiagnosticQuotaUnavailable, DiagnosticWorkerIdentityUnresolved, DiagnosticWorkerExitedEarly, DiagnosticSpoolIngestionFailed, DiagnosticCandidateFreezeFailed, DiagnosticVerificationUnavailable, DiagnosticVerificationBindingUnavailable, DiagnosticStopRequested, DiagnosticDeadlineReached, DiagnosticRecoveryOwnershipUnresolved:
		return true
	}
	return false
}
func (d diagnostic) validate(runID, attemptID string, generation int64) error {
	if !validDiagnosticCode(d.Code) || !workers.ValidID(d.RunID) || d.RunID != runID || d.AttemptID != attemptID || generation <= 0 || d.Generation != generation {
		return &v2contract.ControlError{Code: v2contract.CodeInvalidContract, Owner: "supervisor", OperationID: "service.diagnostic", Disposition: v2contract.CodeInvalidContract.DefaultDisposition(), NextAction: "use a classified matching diagnostic"}
	}
	if d.AttemptID != "" && !workers.ValidID(d.AttemptID) {
		return journal.ErrInvalidEvent
	}
	if len(d.EvidenceRefs) > 16 {
		return journal.ErrInvalidEvent
	}
	for _, ref := range d.EvidenceRefs {
		if !workers.ValidID(ref) {
			return journal.ErrInvalidEvent
		}
	}
	return nil
}

// diagnostic persists safe closed evidence before it wakes delivery. Legacy
// terminal callbacks are not observers and retain their established output;
// service adapters never receive or parse that free-form terminal text.
func (p *pipeline) diagnostic(ctx context.Context, code DiagnosticCode, text string) error {
	if !validDiagnosticCode(code) {
		return diagnostic{Code: code}.validate(p.d.RunID, p.d.AttemptID, p.prod.generation)
	}
	if p.h.Observer != nil {
		d := diagnostic{Code: code, RunID: p.d.RunID, AttemptID: p.d.AttemptID, Generation: p.prod.generation, EvidenceRefs: []string{}}
		if err := d.validate(p.d.RunID, p.d.AttemptID, p.prod.generation); err != nil {
			return err
		}
		payload, err := json.Marshal(d)
		if err != nil {
			return err
		}
		ev := journal.Event{SchemaVersion: 2, Generation: p.prod.generation, EventID: ids.New("evt"), RunID: p.d.RunID, AttemptID: p.d.AttemptID, TaskID: p.d.TaskID, ObservedAt: time.Now().UTC(), Type: serviceDiagnosticEventType, Payload: payload}
		if err := validateDiagnosticEvent(ev, p.d.RunID, p.d.AttemptID, p.prod.generation); err != nil {
			return err
		}
		if err := p.prod.append(ctx, p.j, ev, nil); err != nil {
			return err
		}
		p.h.Observer.Committed(p.d.RunID, 0)
		p.h.Observer.Hint(NoticeRenderRefresh)
	} else if p.h.Notice != nil {
		p.h.Notice(text)
	}
	return nil
}
func (p *pipeline) inform(code NoticeCode, text string) {
	switch code {
	case NoticeReservationHeld, NoticeWorkerReattached, NoticeRenderRefresh:
	default:
		return
	}
	if p.h.Observer != nil {
		p.h.Observer.Hint(code)
	} else if p.h.Notice != nil {
		p.h.Notice(text)
	}
}

func (d diagnostic) Validate() error { return d.validate(d.RunID, d.AttemptID, d.Generation) }
func validateDiagnosticEvent(ev journal.Event, runID, attemptID string, generation int64) error {
	if ev.SchemaVersion != 2 || ev.Type != serviceDiagnosticEventType || ev.RunID != runID || ev.AttemptID != attemptID || ev.Generation != generation {
		return journal.ErrInvalidEvent
	}
	d, err := control.DecodeParams[diagnostic](ev.Payload)
	if err != nil {
		return err
	}
	return d.validate(runID, attemptID, generation)
}
