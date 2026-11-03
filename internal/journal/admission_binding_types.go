package journal

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"maps"
	"math"
	"slices"
	"time"

	"github.com/turbokast/mythhelm/internal/acceptance"
	ab "github.com/turbokast/mythhelm/internal/admissionbinding"
	"github.com/turbokast/mythhelm/internal/contain"
	"github.com/turbokast/mythhelm/internal/v2contract"
)

// PreparationLookup is authenticated service scope, never a wire authority claim.
type PreparationLookup struct {
	PreparationID, PeerID, RootID, BindingSHA256, ContextEpoch string
	Generation                                                 int64
	ExpectedRevision                                           uint64
	ObservedAt                                                 time.Time
}

// PreparationState reports the authoritative stored preparation revision.
type PreparationState struct {
	PreparationID, BindingSHA256 string
	Revision                     uint64
	Ready, Consumed              bool
}

// PreparationOperation binds an immutable acknowledgement to validated intent arguments.
type PreparationOperation struct{ OperationID, ParamsSHA256 string }

// ConfirmationV1 records one structural answer; its caller authenticates consent.
type ConfirmationV1 struct {
	Operation                                                PreparationOperation
	ChallengeID, BindingSHA256, PeerID, RootID, ContextEpoch string
	Generation                                               int64
	Decision                                                 ab.RetentionDecisionV1
	ObservedAt                                               time.Time
}

// RetainedPreparationInput contains only approved canonical nonsecret evidence.
type RetainedPreparationInput struct {
	Operation        PreparationOperation
	CanonicalBinding []byte
	Policy           ab.RetentionPolicyV1
	Retention        ab.RetentionApprovalV1
	Lookup           PreparationLookup
}

// ConfirmedPreparation is produced only by a checked complete durable read.
type ConfirmedPreparation struct{ state *confirmedPreparationState }
type confirmedPreparationState struct {
	metadata PreparationState
	binding  []byte
	lookup   PreparationLookup
}

func (p ConfirmedPreparation) Metadata() (PreparationState, error) {
	if p.state == nil {
		return PreparationState{}, admissionStorageError(v2contract.CodeInvalidContract)
	}
	return p.state.metadata, nil
}
func (p ConfirmedPreparation) CanonicalBinding() ([]byte, error) {
	if p.state == nil {
		return nil, admissionStorageError(v2contract.CodeInvalidContract)
	}
	return bytes.Clone(p.state.binding), nil
}

// AdmissionCommitInput describes fixed storage effects, never an execution permit.
type AdmissionCommitInput struct {
	Preparation      ConfirmedPreparation
	Lookup           PreparationLookup
	OperationID      string
	CanonicalBinding []byte
	AcceptancePin    acceptance.Pin
	Policy           v2contract.PolicyVersion
}
type AdmissionCommit struct{ state *admissionCommitState }
type admissionCommitState struct{ input AdmissionCommitInput }

func NewAdmissionCommit(in AdmissionCommitInput) (AdmissionCommit, error) {
	in = copyAdmissionInput(in)
	b, err := ab.DecodeV1(in.CanonicalBinding)
	if err != nil {
		return AdmissionCommit{}, admissionLeafError(err)
	}
	if in.Preparation.state == nil || !validStorageID(in.OperationID) || !bytes.Equal(in.Preparation.state.binding, in.CanonicalBinding) {
		return AdmissionCommit{}, admissionStorageError(v2contract.CodeInvalidContract)
	}
	if !in.Preparation.state.metadata.Ready || in.Preparation.state.metadata.Consumed || in.Lookup.ExpectedRevision != in.Preparation.state.metadata.Revision {
		return AdmissionCommit{}, admissionStorageError(v2contract.CodeInvalidContract)
	}
	if err := checkLookupBinding(in.Lookup, b); err != nil {
		return AdmissionCommit{}, err
	}
	pin, err := bindingAcceptancePin(b)
	if err != nil {
		return AdmissionCommit{}, err
	}
	want, got := mustJSON(pin), mustJSON(in.AcceptancePin)
	if !bytes.Equal(want, got) || !bytes.Equal(mustJSON(acceptance.P1()), mustJSON(in.Policy)) {
		return AdmissionCommit{}, admissionStorageError(v2contract.CodeInvalidContract)
	}
	in = copyAdmissionInput(in)
	return AdmissionCommit{&admissionCommitState{in}}, nil
}
func (c AdmissionCommit) Snapshot() (AdmissionCommitInput, error) {
	if c.state == nil {
		return AdmissionCommitInput{}, admissionStorageError(v2contract.CodeInvalidContract)
	}
	return copyAdmissionInput(c.state.input), nil
}

type AdmissionMetadata struct {
	OperationID, PreparationID, RootID, PeerID, ContextEpoch, BindingSHA256 string
	RunID, TaskID, AttemptID                                                string
	Generation                                                              int64
	Revision                                                                uint64
}
type CommittedAdmission struct{ state *committedAdmissionState }
type committedAdmissionState struct {
	metadata AdmissionMetadata
	binding  []byte
}

func (c CommittedAdmission) Metadata() (AdmissionMetadata, error) {
	if c.state == nil {
		return AdmissionMetadata{}, admissionStorageError(v2contract.CodeInvalidContract)
	}
	return c.state.metadata, nil
}

type DispatchArmInput struct {
	Admission                                                      CommittedAdmission
	ExpectedPendingRevision                                        uint64
	SnapshotEventID, BindingSHA256, LaunchTokenSHA256, NonceSHA256 string
	LadderVersion, PolicyRevision                                  string
	Generation                                                     int64
	ExecutionDeadline, ObservedAt                                  time.Time
}
type DispatchArm struct{ state *dispatchArmState }
type dispatchArmState struct{ input DispatchArmInput }

func NewDispatchArm(in DispatchArmInput) (DispatchArm, error) {
	if in.Admission.state == nil || (in.ExpectedPendingRevision == 0 || in.ExpectedPendingRevision > math.MaxInt64) || !validStorageID(in.SnapshotEventID) || !validStorageID(in.LadderVersion) || !storageSHA(in.LaunchTokenSHA256) || !storageSHA(in.NonceSHA256) || in.ObservedAt.IsZero() || in.ObservedAt.Year() < 1 || in.ObservedAt.Year() > 9999 || in.ExecutionDeadline.Year() < 1 || in.ExecutionDeadline.Year() > 9999 || !in.ExecutionDeadline.After(in.ObservedAt) {
		return DispatchArm{}, admissionStorageError(v2contract.CodeInvalidContract)
	}
	m := in.Admission.state.metadata
	b, err := ab.DecodeV1(in.Admission.state.binding)
	if err != nil {
		return DispatchArm{}, admissionLeafError(err)
	}
	if in.Generation != m.Generation || in.BindingSHA256 != m.BindingSHA256 || in.PolicyRevision != b.PolicyRevision {
		return DispatchArm{}, admissionStorageError(v2contract.CodeInvalidContract)
	}
	return DispatchArm{&dispatchArmState{in}}, nil
}
func (a DispatchArm) Snapshot() (DispatchArmInput, error) {
	if a.state == nil {
		return DispatchArmInput{}, admissionStorageError(v2contract.CodeInvalidContract)
	}
	return a.state.input, nil
}

type DispatchMetadata struct {
	Admission                                                      AdmissionMetadata
	SnapshotEventID, LaunchTokenSHA256, NonceSHA256, LadderVersion string
	ExecutionDeadline                                              time.Time
	Revision                                                       uint64
}
type CommittedDispatch struct{ state *committedDispatchState }
type committedDispatchState struct{ metadata DispatchMetadata }

func (c CommittedDispatch) Metadata() (DispatchMetadata, error) {
	if c.state == nil {
		return DispatchMetadata{}, admissionStorageError(v2contract.CodeInvalidContract)
	}
	return c.state.metadata, nil
}
func copyAdmissionInput(in AdmissionCommitInput) AdmissionCommitInput {
	in.CanonicalBinding = bytes.Clone(in.CanonicalBinding)
	in.AcceptancePin.CheckTimeouts = maps.Clone(in.AcceptancePin.CheckTimeouts)
	in.Policy.Prompts = maps.Clone(in.Policy.Prompts)
	in.Policy.Context = maps.Clone(in.Policy.Context)
	in.Policy.Review = maps.Clone(in.Policy.Review)
	in.Policy.Decomposition = maps.Clone(in.Policy.Decomposition)
	in.Policy.EligibleCohorts = slices.Clone(in.Policy.EligibleCohorts)
	in.Policy.Evidence = slices.Clone(in.Policy.Evidence)
	return in
}
func admissionStorageError(code v2contract.Code) error {
	return &v2contract.ControlError{Code: code, Owner: "journal", OperationID: "service-admission.storage", Disposition: code.DefaultDisposition(), NextAction: "refresh the authenticated preparation or reconcile durable state", Detail: map[string]string{"mythhelm.admission_binding/reason": string(code)}}
}
func admissionLeafError(err error) error {
	if errors.Is(err, ab.ErrCapabilityUnsupported) && !errors.Is(err, ab.ErrInvalidContract) {
		return admissionStorageError(v2contract.CodeCapabilityUnsupported)
	}
	return admissionStorageError(v2contract.CodeInvalidContract)
}
func retentionStorageError() error {
	return &v2contract.ControlError{Code: v2contract.CodePolicyIneligible, Owner: "journal", OperationID: "service-admission.storage", Disposition: v2contract.CodePolicyIneligible.DefaultDisposition(), NextAction: "prepare again with exact nonsecret retention consent", Detail: map[string]string{"mythhelm.retention/reason": "policy_ineligible"}}
}
func validStorageID(s string) bool {
	if len(s) == 0 || len(s) > 4096 {
		return false
	}
	for _, r := range s {
		if r < 33 || r > 126 {
			return false
		}
	}
	return true
}
func storageSHA(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, r := range s {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}
func storageDigest(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }
func mustJSON(v any) []byte           { b, _ := json.Marshal(v); return b }
func checkLookupScope(l PreparationLookup, b ab.BindingV1) error {
	if !validStorageID(l.PreparationID) || !validStorageID(l.PeerID) || !validStorageID(l.RootID) || !validStorageID(l.ContextEpoch) || !storageSHA(l.BindingSHA256) || l.Generation <= 0 || l.ObservedAt.IsZero() || l.ExpectedRevision > math.MaxInt64 {
		return admissionStorageError(v2contract.CodeInvalidContract)
	}
	if l.PreparationID != b.PreparationID || l.RootID != b.RootID || l.PeerID != b.PeerID {
		return admissionStorageError(v2contract.CodePermissionDenied)
	}
	if l.Generation != b.Generation {
		return admissionStorageError(v2contract.CodeRevisionConflict)
	}
	if l.ContextEpoch != b.ContextEpoch {
		return admissionStorageError(v2contract.CodeDependencyStale)
	}
	sum, err := ab.DigestV1(b)
	if err != nil {
		return admissionLeafError(err)
	}
	if l.BindingSHA256 != hex.EncodeToString(sum[:]) {
		return admissionStorageError(v2contract.CodeRevisionConflict)
	}
	return nil
}
func checkLookupBinding(l PreparationLookup, b ab.BindingV1) error {
	if err := checkLookupScope(l, b); err != nil {
		return err
	}
	expires, err := time.Parse(time.RFC3339Nano, b.ExpiresAt)
	if err != nil {
		return admissionLeafError(err)
	}
	if !l.ObservedAt.Before(expires) {
		return admissionStorageError(v2contract.CodeDependencyStale)
	}
	return nil
}

// bindingAcceptancePin uses the legacy evaluator's closed canonical check
// representation. A producer golden guards this package boundary against drift.
func bindingAcceptancePin(b ab.BindingV1) (acceptance.Pin, error) {
	d := b.Decision
	p := acceptance.NewPin(b.TaskID, d.Task.SHA256)
	if d.NoChecks || d.Profile.Name == "inspect" {
		return p, nil
	}
	digests := make([]string, len(d.ProjectConfig.Checks))
	timeouts := make(map[string]string, len(digests))
	for i, c := range d.ProjectConfig.Checks {
		raw := mustJSON(struct {
			Name         string   `json:"name"`
			Argv         []string `json:"argv"`
			FailOnOutput bool     `json:"fail_on_output"`
			Timeout      string   `json:"timeout"`
		}{c.Name, c.Argv, c.FailOnOutput, c.Timeout})
		digests[i] = storageDigest(raw)
		timeouts[c.Name] = c.Timeout
	}
	policy := contain.Policy{}
	boundary, version := "host", ""
	if d.Profile.Contained {
		policy = contain.Policy{Profile: "restricted", ReadOnly: true}
		boundary, version = contain.BoundaryName, contain.BoundaryVersion
	}
	ev := contain.EvaluatorDigest(boundary, version, policy, digests)
	return p.WithChecks(d.ConfigDigest, ev.Name, ev.Digest, timeouts), nil
}
