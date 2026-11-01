package control

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"math"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/v2contract"
)

// RunCeilings is the closed standalone run wire contract.
type RunCeilings struct {
	ExecutionMillis  *int64 `json:"execution_millis"`
	Repairs          *int   `json:"repairs"`
	Replans          *int   `json:"replans"`
	TransportRetries *int   `json:"transport_retries"`
}

// RunSpec is the closed standalone run wire contract.
type RunSpec struct {
	SchemaVersion              int         `json:"schema_version"`
	Repository                 string      `json:"repository"`
	TaskFile                   string      `json:"task_file"`
	TaskSHA256                 string      `json:"task_sha256"`
	Adapter                    string      `json:"adapter"`
	Billing                    string      `json:"billing"`
	ExecutionProfile           string      `json:"execution_profile"`
	Revision                   string      `json:"revision"`
	Host                       string      `json:"host"`
	Scenario                   string      `json:"scenario"`
	UseCommitted               bool        `json:"use_committed"`
	NoChecks                   bool        `json:"no_checks"`
	KeepGoing                  bool        `json:"keep_going"`
	TrustProjectConfig         string      `json:"trust_project_config"`
	TrustNativeConfig          string      `json:"trust_native_config"`
	DeclareEntitlement         string      `json:"declare_entitlement"`
	StripCredentialEnv         bool        `json:"strip_credential_env"`
	AllowUntestedNativeVersion bool        `json:"allow_untested_native_version"`
	Ceilings                   RunCeilings `json:"ceilings"`
}

// AdmissionChallenge is the closed standalone run wire contract.
type AdmissionChallenge struct {
	ID            string `json:"id"`
	Kind          string `json:"kind"`
	Text          string `json:"text"`
	BindingSHA256 string `json:"binding_sha256"`
}

// PreparedRun is the closed standalone run wire contract.
type PreparedRun struct {
	SchemaVersion int                  `json:"schema_version"`
	PreparationID string               `json:"preparation_id"`
	BindingSHA256 string               `json:"binding_sha256"`
	Generation    int64                `json:"generation"`
	Challenges    []AdmissionChallenge `json:"challenges"`
	Ready         bool                 `json:"ready"`
}

// RunConfirmation is the closed standalone run wire contract.
type RunConfirmation struct {
	SchemaVersion int    `json:"schema_version"`
	PreparationID string `json:"preparation_id"`
	ChallengeID   string `json:"challenge_id"`
	BindingSHA256 string `json:"binding_sha256"`
	Approved      bool   `json:"approved"`
}

// StartRun is the closed standalone run wire contract.
type StartRun struct {
	SchemaVersion int    `json:"schema_version"`
	PreparationID string `json:"preparation_id"`
	BindingSHA256 string `json:"binding_sha256"`
}

// RunAccepted is the closed standalone run wire contract.
type RunAccepted struct {
	SchemaVersion int    `json:"schema_version"`
	OperationID   string `json:"operation_id"`
	RunID         string `json:"run_id"`
	AttemptID     string `json:"attempt_id"`
	Generation    int64  `json:"generation"`
	Revision      uint64 `json:"revision"`
}

// RecoverRun is the closed standalone run wire contract.
type RecoverRun struct {
	SchemaVersion    int    `json:"schema_version"`
	RunID            string `json:"run_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	Generation       int64  `json:"generation"`
}

// ApplyRun is the closed standalone run wire contract.
type ApplyRun struct {
	SchemaVersion    int    `json:"schema_version"`
	RunID            string `json:"run_id"`
	CandidateSHA256  string `json:"candidate_sha256"`
	Branch           string `json:"branch"`
	AcceptFlags      bool   `json:"accept_flags"`
	AcceptUnverified bool   `json:"accept_unverified"`
	ExpectedRevision uint64 `json:"expected_revision"`
	Generation       int64  `json:"generation"`
}

// ActionAccepted is the closed standalone run wire contract.
type ActionAccepted struct {
	SchemaVersion int    `json:"schema_version"`
	OperationID   string `json:"operation_id"`
	RunID         string `json:"run_id"`
	Method        string `json:"method"`
	Generation    int64  `json:"generation"`
	Revision      uint64 `json:"revision"`
}

// RunOutcome is the closed standalone run wire contract.
type RunOutcome struct {
	RunID         string `json:"run_id"`
	AttemptID     string `json:"attempt_id"`
	State         string `json:"state"`
	Reason        string `json:"reason"`
	AttemptState  string `json:"attempt_state"`
	AttemptReason string `json:"attempt_reason"`
	Settled       bool   `json:"settled"`
}

// ApplyOutcome is the closed standalone run wire contract.
type ApplyOutcome struct {
	RunID           string `json:"run_id"`
	Branch          string `json:"branch"`
	CandidateCommit string `json:"candidate_commit"`
	State           string `json:"state"`
	Applied         bool   `json:"applied"`
}

// OperationQuery is the closed standalone run wire contract.
type OperationQuery struct {
	SchemaVersion int    `json:"schema_version"`
	RunID         string `json:"run_id"`
	OperationID   string `json:"operation_id"`
}

// OperationStatus is the closed standalone run wire contract.
type OperationStatus struct {
	SchemaVersion int               `json:"schema_version"`
	OperationID   string            `json:"operation_id"`
	RunID         string            `json:"run_id"`
	Method        string            `json:"method"`
	State         string            `json:"state"`
	Start         *RunAccepted      `json:"start,omitempty"`
	Action        *ActionAccepted   `json:"action,omitempty"`
	Outcome       *RunOutcome       `json:"outcome,omitempty"`
	Apply         *ApplyOutcome     `json:"apply,omitempty"`
	Stop          *StopResult       `json:"stop,omitempty"`
	Failure       *OperationFailure `json:"failure,omitempty"`
}

// OperationFailure is the closed standalone run wire contract.
type OperationFailure struct {
	Code       string `json:"code"`
	Reason     string `json:"reason"`
	NextAction string `json:"next_action"`
}

// ObserveRun is the closed standalone run wire contract.
type ObserveRun struct {
	SchemaVersion int    `json:"schema_version"`
	RunID         string `json:"run_id"`
	Generation    int64  `json:"generation"`
	AfterSequence int64  `json:"after_sequence"`
	Limit         int    `json:"limit"`
	WaitMillis    int    `json:"wait_millis"`
}

// RunObservation is the closed standalone run wire contract.
type RunObservation struct {
	SchemaVersion int             `json:"schema_version"`
	RunID         string          `json:"run_id"`
	Generation    int64           `json:"generation"`
	RunRevision   uint64          `json:"run_revision"`
	Outcome       RunOutcome      `json:"outcome"`
	Events        []journal.Event `json:"events"`
	NextSequence  int64           `json:"next_sequence"`
	More          bool            `json:"more"`
}

// RunClient sends typed standalone operator requests; it grants no host authority.
type RunClient interface {
	Prepare(context.Context, RunSpec) (PreparedRun, error)
	Confirm(context.Context, RunConfirmation) (PreparedRun, error)
	Start(context.Context, string, uint64, StartRun) (RunAccepted, error)
	Recover(context.Context, string, RecoverRun) (ActionAccepted, error)
	Apply(context.Context, string, ApplyRun) (ActionAccepted, error)
	Operation(context.Context, OperationQuery) (OperationStatus, error)
	Observe(context.Context, ObserveRun) (RunObservation, error)
}

func contractError(reason string) error { return newError(CodeInvalidContract, "%s", reason) }
func schema2(version int) error {
	if version > 2 {
		return newError(CodeCapabilityUnsupported, "unsupported contract schema")
	}
	if version != 2 {
		return contractError("schema_version must be 2")
	}
	return nil
}
func required(values ...string) error {
	for _, s := range values {
		if s == "" || len(s) > 4096 || strings.IndexFunc(s, unicode.IsControl) >= 0 {
			return contractError("missing, oversized or unsafe contract field")
		}
	}
	return nil
}
func digest(s string) bool {
	s = strings.TrimPrefix(s, "sha256:")
	b, err := hex.DecodeString(s)
	return err == nil && len(b) == 32
}
func generation(g int64) error {
	if g <= 0 {
		return contractError("generation must be positive")
	}
	return nil
}
func revision(r uint64) error {
	if r > math.MaxInt64 {
		return contractError("revision exceeds canonical ledger range")
	}
	return nil
}

// Validate checks explicit counters and a positive finite duration.
func (c RunCeilings) Validate() error {
	if c.ExecutionMillis != nil && (*c.ExecutionMillis <= 0 || *c.ExecutionMillis > math.MaxInt64/int64(1000000)) {
		return contractError("invalid execution_millis")
	}
	for _, p := range []*int{c.Repairs, c.Replans, c.TransportRetries} {
		if p != nil && *p < 0 {
			return contractError("negative ceiling")
		}
	}
	return nil
}

// Validate requires task binding, standalone paths and bounded flag values.
func (s RunSpec) Validate() error {
	if err := schema2(s.SchemaVersion); err != nil {
		return err
	}
	if err := required(s.Repository, s.TaskFile, s.Adapter, s.Billing); err != nil {
		return err
	}
	if !filepath.IsAbs(s.Repository) || !filepath.IsAbs(s.TaskFile) || !digest(s.TaskSHA256) {
		return contractError("absolute repository/task paths and task digest required")
	}
	if s.Host != "" && s.Host != "standalone" {
		return contractError("host cannot admit native execution")
	}
	for _, v := range []string{s.ExecutionProfile, s.Revision, s.Scenario, s.TrustProjectConfig, s.TrustNativeConfig, s.DeclareEntitlement} {
		if v != "" {
			if err := required(v); err != nil {
				return err
			}
		}
	}
	return s.Ceilings.Validate()
}

// Validate checks challenge identity and its evidence binding.
func (c AdmissionChallenge) Validate() error {
	if err := required(c.ID, c.Kind, c.Text); err != nil {
		return err
	}
	if !digest(c.BindingSHA256) {
		return contractError("challenge binding required")
	}
	return nil
}

// Validate checks generation, evidence binding and bounded unique challenges.
func (p PreparedRun) Validate() error {
	if err := schema2(p.SchemaVersion); err != nil {
		return err
	}
	if err := required(p.PreparationID); err != nil {
		return err
	}
	if !digest(p.BindingSHA256) {
		return contractError("preparation binding required")
	}
	if err := generation(p.Generation); err != nil {
		return err
	}
	if p.Challenges == nil || len(p.Challenges) > 128 {
		return contractError("challenges must be a bounded known list")
	}
	seen := make(map[string]bool, len(p.Challenges))
	for _, c := range p.Challenges {
		if err := c.Validate(); err != nil {
			return err
		}
		if seen[c.ID] {
			return contractError("duplicate challenge")
		}
		seen[c.ID] = true
		if c.BindingSHA256 != p.BindingSHA256 {
			return contractError("challenge binding mismatch")
		}
	}
	return nil
}

// Validate requires a bound preparation and challenge.
func (c RunConfirmation) Validate() error {
	if err := schema2(c.SchemaVersion); err != nil {
		return err
	}
	if err := required(c.PreparationID, c.ChallengeID); err != nil {
		return err
	}
	if !digest(c.BindingSHA256) {
		return contractError("confirmation binding required")
	}
	return nil
}

// Validate requires the exact preparation and evidence binding.
func (s StartRun) Validate() error {
	if err := schema2(s.SchemaVersion); err != nil {
		return err
	}
	if err := required(s.PreparationID); err != nil {
		return err
	}
	if !digest(s.BindingSHA256) {
		return contractError("start binding required")
	}
	return nil
}

// Validate checks immutable launch acknowledgment identity and revision.
func (a RunAccepted) Validate() error {
	if err := schema2(a.SchemaVersion); err != nil {
		return err
	}
	if err := required(a.OperationID, a.RunID, a.AttemptID); err != nil {
		return err
	}
	if err := generation(a.Generation); err != nil {
		return err
	}
	return revision(a.Revision)
}

// Validate checks the target run, current generation and expected revision.
func (r RecoverRun) Validate() error {
	if err := schema2(r.SchemaVersion); err != nil {
		return err
	}
	if err := required(r.RunID); err != nil {
		return err
	}
	if err := generation(r.Generation); err != nil {
		return err
	}
	return revision(r.ExpectedRevision)
}

// Validate checks target identity, candidate binding and revision.
func (a ApplyRun) Validate() error {
	if err := schema2(a.SchemaVersion); err != nil {
		return err
	}
	if err := required(a.RunID, a.Branch); err != nil {
		return err
	}
	if !digest(a.CandidateSHA256) {
		return contractError("candidate binding required")
	}
	if err := generation(a.Generation); err != nil {
		return err
	}
	return revision(a.ExpectedRevision)
}

// Validate checks an immutable recover/apply acknowledgment.
func (a ActionAccepted) Validate() error {
	if err := schema2(a.SchemaVersion); err != nil {
		return err
	}
	if a.Method != "run.recover" && a.Method != "run.apply" {
		return contractError("invalid action method")
	}
	if err := required(a.OperationID, a.RunID); err != nil {
		return err
	}
	if err := generation(a.Generation); err != nil {
		return err
	}
	return revision(a.Revision)
}
func knownRunState(s string) bool {
	switch s {
	case "created", "admission", "executing", "verifying", "ready_for_review", "applying", "completed", "stopping", "cancelled", "interrupted", "recovering", "blocked", "failed":
		return true
	}
	return false
}
func knownAttemptState(s string) bool {
	switch s {
	case "", "launch_intent_recorded", "launching", "running", "succeeded_native", "failed_native", "stop_requested", "stopped", "interrupted", "quarantined":
		return true
	}
	return false
}
func boundedReason(s string) bool {
	return len(s) <= 128 && strings.IndexFunc(s, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' && r != '.' && r != '-'
	}) < 0
}

// Validate checks canonical run/attempt states and bounded reasons.
func (o RunOutcome) Validate() error {
	if err := required(o.RunID); err != nil {
		return err
	}
	if !knownRunState(o.State) || !knownAttemptState(o.AttemptState) || !boundedReason(o.Reason) || !boundedReason(o.AttemptReason) {
		return contractError("invalid canonical outcome")
	}
	if (o.AttemptID == "") != (o.AttemptState == "") {
		return contractError("attempt identity/state mismatch")
	}
	return nil
}

// Validate checks the guarded apply result identity and state.
func (a ApplyOutcome) Validate() error {
	if err := required(a.RunID, a.Branch, a.CandidateCommit); err != nil {
		return err
	}
	if !knownRunState(a.State) || a.Applied && a.State != "completed" {
		return contractError("invalid apply outcome")
	}
	return nil
}

// Validate requires a named run and operation.
func (q OperationQuery) Validate() error {
	if err := schema2(q.SchemaVersion); err != nil {
		return err
	}
	return required(q.RunID, q.OperationID)
}

// Validate checks a canonical code and bounded failure guidance.
func (f OperationFailure) Validate() error {
	if !v2contract.Code(f.Code).Valid() || f.Reason == "" || !boundedReason(f.Reason) {
		return contractError("invalid operation failure")
	}
	return required(f.NextAction)
}

// Validate enforces the method-specific acknowledgment/result union.
func (o OperationStatus) Validate() error {
	if err := schema2(o.SchemaVersion); err != nil {
		return err
	}
	if err := required(o.OperationID, o.RunID); err != nil {
		return err
	}
	switch o.State {
	case "accepted", "running", "completed", "failed", "unconfirmed":
	default:
		return contractError("invalid operation state")
	}
	if o.Failure != nil {
		if o.State != "failed" && o.State != "unconfirmed" {
			return contractError("failure on nonfailure state")
		}
		if err := o.Failure.Validate(); err != nil {
			return err
		}
	} else if o.State == "failed" {
		return contractError("failed operation lacks failure")
	}
	switch o.Method {
	case "run.start":
		if o.Start == nil || o.Action != nil || o.Apply != nil || o.Stop != nil {
			return contractError("invalid start result union")
		}
		if err := o.Start.Validate(); err != nil {
			return err
		}
		if o.Start.OperationID != o.OperationID || o.Start.RunID != o.RunID {
			return contractError("start acknowledgment identity mismatch")
		}
	case "run.recover", "run.apply":
		if o.Action == nil || o.Start != nil || o.Stop != nil || o.Method == "run.recover" && o.Apply != nil || o.Method == "run.apply" && o.Outcome != nil {
			return contractError("invalid action result union")
		}
		if err := o.Action.Validate(); err != nil {
			return err
		}
		if o.Action.OperationID != o.OperationID || o.Action.RunID != o.RunID || o.Action.Method != o.Method {
			return contractError("action acknowledgment identity mismatch")
		}
	case "stop", "attachment.stop":
		if o.Stop == nil || o.Start != nil || o.Action != nil || o.Apply != nil || o.Outcome != nil {
			return contractError("invalid stop result union")
		}
		if err := o.Stop.Validate(); err != nil {
			return err
		}
		if o.Stop.OperationID != o.OperationID || !o.Stop.Ack.Accepted {
			return contractError("stop acknowledgment identity mismatch")
		}
	default:
		return contractError("unknown operation method")
	}
	if o.Outcome != nil {
		if err := o.Outcome.Validate(); err != nil {
			return err
		}
		if o.Outcome.RunID != o.RunID {
			return contractError("outcome run mismatch")
		}
	}
	if o.Apply != nil {
		if err := o.Apply.Validate(); err != nil {
			return err
		}
		if o.Apply.RunID != o.RunID {
			return contractError("apply run mismatch")
		}
	}
	if o.State == "completed" {
		switch o.Method {
		case "run.start", "run.recover":
			if o.Outcome == nil || !o.Outcome.Settled {
				return contractError("completed operation lacks settled outcome")
			}
		case "run.apply":
			if o.Apply == nil {
				return contractError("completed apply lacks result")
			}
		default:
			if o.Stop.State != "confirmed" {
				return contractError("completed stop lacks confirmation")
			}
		}
	}
	return nil
}

// Validate checks the schema, run, generation and bounded cursor request.
func (o ObserveRun) Validate() error {
	if err := schema2(o.SchemaVersion); err != nil {
		return err
	}
	if err := required(o.RunID); err != nil {
		return err
	}
	if err := generation(o.Generation); err != nil {
		return err
	}
	if o.AfterSequence < 0 || o.AfterSequence == math.MaxInt64 || o.Limit < 0 || o.Limit > 128 || o.WaitMillis < 0 || o.WaitMillis > 1000 {
		return contractError("invalid observation cursor or bounds")
	}
	return nil
}

// Validate checks a bounded contiguous page and matching outcome.
func (o RunObservation) Validate() error {
	if err := schema2(o.SchemaVersion); err != nil {
		return err
	}
	if err := required(o.RunID); err != nil {
		return err
	}
	if err := generation(o.Generation); err != nil {
		return err
	}
	if err := revision(o.RunRevision); err != nil {
		return err
	}
	if o.NextSequence < 0 || o.Events == nil || len(o.Events) > 128 {
		return contractError("invalid observation page")
	}
	if err := o.Outcome.Validate(); err != nil {
		return err
	}
	if o.Outcome.RunID != o.RunID {
		return contractError("observation outcome mismatch")
	}
	for i, e := range o.Events {
		if e.RunID != o.RunID || e.RunSequence <= 0 || i > 0 && e.RunSequence != o.Events[i-1].RunSequence+1 {
			return contractError("event page gap or identity mismatch")
		}
		if err := validateObservedEvent(e); err != nil {
			return err
		}
	}
	if len(o.Events) > 0 && o.NextSequence != o.Events[len(o.Events)-1].RunSequence {
		return contractError("observation cursor mismatch")
	}
	return nil
}
func validateObservedEvent(e journal.Event) error {
	switch e.SchemaVersion {
	case 1:
		if e.EventID == "" || e.RunID == "" || e.ProducerID == "" || e.Type == "" || e.ObservedAt.IsZero() || e.Generation < 0 || !json.Valid(e.Payload) || !strings.HasPrefix(strings.TrimSpace(string(e.Payload)), "{") {
			return contractError("malformed historical event")
		}
	case 2:
		if err := v2contract.Envelope(e).Validate(); err != nil {
			return contractError("malformed canonical event")
		}
	default:
		if e.SchemaVersion < 1 {
			return contractError("invalid event schema")
		}
		return newError(CodeCapabilityUnsupported, "unsupported event schema")
	}
	return nil
}

// PreparedReply carries verified outer metadata; it adds no wire-body fields.
type PreparedReply struct {
	Run      PreparedRun
	Revision uint64
}

// PreparedRunClient uses explicit operation identities and server revisions.
type PreparedRunClient interface {
	RunClient
	PrepareRevisioned(context.Context, string, RunSpec) (PreparedReply, error)
	ConfirmRevisioned(context.Context, string, uint64, RunConfirmation) (PreparedReply, error)
}

// PreparationConflictError exposes only validated same-binding current metadata.
type PreparationConflictError struct {
	current    PreparedReply
	cause      *Error
	hasCurrent bool
}

func (e *PreparationConflictError) Error() string { return e.cause.Error() }
func (e *PreparationConflictError) Unwrap() error { return e.cause }

// Current returns an independent copy of validated current preparation metadata.
func (e *PreparationConflictError) Current() (PreparedReply, bool) {
	if e == nil || !e.hasCurrent {
		return PreparedReply{}, false
	}
	out := e.current
	out.Run.Challenges = append([]AdmissionChallenge{}, out.Run.Challenges...)
	return out, true
}
