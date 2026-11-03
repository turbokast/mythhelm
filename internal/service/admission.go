package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strconv"
	"sync"
	"time"

	"github.com/turbokast/mythhelm/internal/admission"
	ab "github.com/turbokast/mythhelm/internal/admissionbinding"
	"github.com/turbokast/mythhelm/internal/control"
	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/v2contract"
)

// preparationStore is owned by one runtime. mu serialises its bounded RAM
// plans/replay cache and their approved durable confirmation transitions.
type preparationStore struct {
	// beforeEffect observes a fresh boundary in tests; it returns no authority.
	// Production composition leaves it nil; callbacks perform no lifecycle work.
	beforeEffect  func()
	mu            sync.Mutex
	context       *supervisor.ExecutionContext
	policy        ab.RetentionPolicyV1
	plans         map[string]*preparationEntry
	operations    map[string]preparationReplay
	confirmations map[string]confirmationReplay
}
type preparationEntry struct {
	plan     *supervisor.ServicePlan
	metadata supervisor.PlanMetadata
	inputs   supervisor.ServiceInputs
	expires  time.Time
	retained bool
}
type confirmationReplay struct {
	peerID, preparationID, digest string
	state                         journal.PreparationState
}

type preparationReplay struct {
	peerID, preparationID, digest string
	result                        control.Result
}

func (r *runtime) registerPreparedAdmission() error {
	r.preparations = &preparationStore{plans: map[string]*preparationEntry{}, operations: map[string]preparationReplay{}, confirmations: map[string]confirmationReplay{}}
	return r.server.RegisterPreparedAdmission(r.prepareAdmission, r.confirmAdmission)
}

func admissionFailure(code control.Code, message string) error {
	return &control.Error{Code: code, Message: message}
}
func admissionWireError(err error) error {
	if err == nil {
		return nil
	}
	if ce, ok := errors.AsType[*control.Error](err); ok {
		return ce
	}
	if ve, ok := errors.AsType[*v2contract.ControlError](err); ok {
		return admissionFailure(control.Code(ve.Code), "service admission refused; prepare again using the current service context")
	}
	if blocked, ok := errors.AsType[*admission.BlockedError](err); ok {
		code := control.Code("policy_ineligible")
		if blocked.Capability {
			code = control.CodeCapabilityUnsupported
		}
		return admissionFailure(code, blocked.Code+": "+blocked.Action)
	}
	return admissionFailure(control.CodeInvalidContract, "service admission could not resolve the requested contract")
}

// admissionScope authenticates current scope before any existence/replay lookup.
func (r *runtime) admissionScope(ctx context.Context, peer control.Peer, in control.Intent) (string, func(), error) {
	if !peer.SameUser || in.CapabilityToken != "" {
		return "", nil, admissionFailure(control.CodePermissionDenied, "ordinary admission requires the standalone OS peer")
	}
	if err := in.Validate(); err != nil {
		return "", nil, admissionFailure(control.CodeInvalidContract, "invalid admission envelope")
	}
	if len(in.OperationID) > 4096 {
		return "", nil, admissionFailure(control.CodeInvalidContract, "admission operation identity exceeds its bound")
	}
	for _, ch := range in.OperationID {
		if ch < 33 || ch > 126 {
			return "", nil, admissionFailure(control.CodeInvalidContract, "invalid admission operation identity")
		}
	}

	r.mu.Lock()
	closing := r.closing
	r.mu.Unlock()
	if closing || r.journal == nil || r.migrationRequired {
		return "", nil, admissionFailure(control.CodeCapabilityUnsupported, "service admission is unavailable")
	}
	release, err := r.lease.Hold()
	if err != nil {
		return "", nil, admissionFailure(control.Code("dependency_stale"), "service ownership changed")
	}
	fail := func(err error) (string, func(), error) { release(); return "", nil, err }
	if err := r.lease.CheckRootIdentity(ctx, r.journal.StateDir()); err != nil {
		return fail(admissionFailure(control.Code("dependency_stale"), "service root changed"))
	}
	if err := r.journal.CheckStateIdentity(ctx); err != nil {
		return fail(admissionFailure(control.Code("dependency_stale"), "service database changed"))
	}
	if in.Generation == nil || *in.Generation != r.lease.Generation() {
		return fail(admissionFailure(control.CodeRevisionConflict, "service generation changed"))
	}
	sum := sha256.Sum256([]byte(strconv.FormatUint(uint64(peer.UID), 10) + "\x00" + peer.OSUser + "\x00" + strconv.FormatInt(int64(peer.PID), 10)))
	return "peer_" + hex.EncodeToString(sum[:]), release, nil
}

func (s *preparationStore) expire(now time.Time) {
	for id, entry := range s.plans {
		if !now.Before(entry.expires) {
			delete(s.plans, id)
			for op, replay := range s.confirmations {
				if replay.preparationID == id {
					delete(s.confirmations, op)
				}
			}
			for op, replay := range s.operations {
				if replay.preparationID == id {
					delete(s.operations, op)
				}
			}
		}
	}
}
func preparedResult(operation string, m supervisor.PlanMetadata, state journal.PreparationState) (control.Result, error) {
	if state.Revision > math.MaxInt64 {
		return control.Result{}, admissionFailure(control.CodeInvalidContract, "preparation revision exceeds the ledger range")
	}
	challenges := make([]control.AdmissionChallenge, 0, len(m.Challenges))
	for _, ch := range m.Challenges {
		challenges = append(challenges, control.AdmissionChallenge{ID: ch.ID, Kind: ch.Kind, Text: ch.Text, BindingSHA256: m.BindingSHA256})
	}
	body, err := json.Marshal(control.PreparedRun{SchemaVersion: 2, PreparationID: m.PreparationID, BindingSHA256: m.BindingSHA256, Generation: m.Generation, Challenges: challenges, Ready: state.Ready})
	return control.Result{OperationID: operation, Revision: int64(state.Revision), Body: body}, err
}

func (r *runtime) prepareAdmission(ctx context.Context, peer control.Peer, in control.Intent) (control.Result, error) {
	peerID, release, err := r.admissionScope(ctx, peer, in)
	if err != nil {
		return control.Result{}, err
	}
	defer release()
	wire, err := decodeAdmission[control.RunSpec](in.Params)
	if err != nil {
		return control.Result{}, admissionFailure(control.CodeInvalidContract, "invalid standalone run request")
	}
	if in.Object != "" || in.ExpectedRevision != 0 {
		return control.Result{}, admissionFailure(control.CodeInvalidContract, "prepare requires an unbound revision-zero intent")
	}
	digest, err := control.PreparedAdmissionDigest(in)
	if err != nil {
		return control.Result{}, err
	}
	s := r.preparations
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.context == nil {
		return control.Result{}, admissionFailure(control.CodeCapabilityUnsupported, "no actual service retention policy is configured")
	}
	meta, err := s.context.Metadata()
	if err != nil {
		return control.Result{}, admissionWireError(err)
	}
	s.expire(time.Now().UTC())
	if _, found := s.confirmations[in.OperationID]; found {
		return control.Result{}, admissionFailure(control.CodeRevisionConflict, "operation method changed")
	}
	if replay, ok := s.operations[in.OperationID]; ok {
		if replay.peerID != peerID {
			return control.Result{}, admissionFailure(control.CodePermissionDenied, "preparation belongs to another OS peer")
		}
		if replay.digest != digest {
			return control.Result{}, admissionFailure(control.CodeRevisionConflict, "operation arguments changed")
		}
		entry := s.plans[replay.preparationID]
		if entry == nil {
			return control.Result{}, admissionFailure(control.Code("dependency_stale"), "preparation expired")
		}
		if meta.ContextEpoch != entry.inputs.ContextEpoch || meta.PolicyRevision != entry.inputs.PolicyRevision || meta.RootID != entry.inputs.RootID {
			return control.Result{}, admissionFailure(control.Code("dependency_stale"), "preparation context changed")
		}
		if err := supervisor.ValidateServicePlan(ctx, entry.plan); err != nil {
			return control.Result{}, admissionWireError(err)
		}
		out := replay.result
		out.Body = append(json.RawMessage{}, out.Body...)
		return out, nil
	}
	count := 0
	for _, entry := range s.plans {
		if entry.inputs.PeerID == peerID {
			count++
		}
	}
	if len(s.plans) >= 32 || count >= 8 {
		return control.Result{}, admissionFailure(control.CodeAllowanceExhausted, "volatile preparation capacity reached")
	}
	request, err := admissionRequest(wire)
	if err != nil {
		return control.Result{}, err
	}
	inputs := supervisor.ServiceInputs{RootID: meta.RootID, PeerID: peerID, ContextEpoch: meta.ContextEpoch, PrincipalRef: meta.PrincipalRef, PolicyRevision: meta.PolicyRevision, Generation: meta.Generation, Request: request, TaskArtifactRef: "pending/" + ids.New("task"), Context: s.context}
	plan, err := supervisor.PrepareService(ctx, inputs)
	if err != nil {
		return control.Result{}, admissionWireError(err)
	}
	if s.beforeEffect != nil {
		s.beforeEffect()
	}
	if err := supervisor.ValidateServicePlan(ctx, plan); err != nil {
		return control.Result{}, admissionWireError(err)
	}
	metadata, err := plan.Metadata()
	if err != nil {
		return control.Result{}, admissionWireError(err)
	}
	if _, err := plan.CanonicalBinding(); err != nil {
		return control.Result{}, admissionWireError(err)
	}
	result, err := preparedResult(in.OperationID, metadata, journal.PreparationState{})
	if err != nil {
		return control.Result{}, admissionWireError(err)
	}
	s.plans[metadata.PreparationID] = &preparationEntry{plan: plan, metadata: metadata, inputs: inputs, expires: metadata.ExpiresAt}
	s.operations[in.OperationID] = preparationReplay{peerID: peerID, preparationID: metadata.PreparationID, digest: digest, result: result}
	return result, nil
}

func (r *runtime) confirmAdmission(ctx context.Context, peer control.Peer, in control.Intent) (control.Result, error) {
	peerID, release, err := r.admissionScope(ctx, peer, in)
	if err != nil {
		return control.Result{}, err
	}
	defer release()
	answer, err := decodeAdmission[control.RunConfirmation](in.Params)
	if err != nil {
		return control.Result{}, admissionFailure(control.CodeInvalidContract, "invalid bound confirmation")
	}
	if in.Object != answer.PreparationID {
		return control.Result{}, admissionFailure(control.CodeInvalidContract, "confirmation object mismatch")
	}
	digest, err := control.PreparedAdmissionDigest(in)
	if err != nil {
		return control.Result{}, err
	}
	s := r.preparations
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.context == nil {
		return control.Result{}, admissionFailure(control.CodeCapabilityUnsupported, "no actual service retention policy is configured")
	}
	currentContext, err := s.context.Metadata()
	if err != nil {
		return control.Result{}, admissionWireError(err)
	}
	s.expire(time.Now().UTC())
	entry := s.plans[answer.PreparationID]
	if entry == nil {
		return control.Result{}, admissionFailure(control.Code("dependency_stale"), "preparation is unavailable; prepare again")
	}
	if entry.inputs.PeerID != peerID {
		return control.Result{}, admissionFailure(control.CodePermissionDenied, "preparation belongs to another OS peer")
	}
	if currentContext.ContextEpoch != entry.inputs.ContextEpoch || currentContext.PolicyRevision != entry.inputs.PolicyRevision || currentContext.RootID != entry.inputs.RootID {
		return control.Result{}, admissionFailure(control.Code("dependency_stale"), "preparation context changed")
	}
	if replay, ok := s.confirmations[in.OperationID]; ok {
		if replay.peerID != peerID {
			return control.Result{}, admissionFailure(control.CodePermissionDenied, "confirmation belongs to another OS peer")
		}
		if replay.preparationID != answer.PreparationID || replay.digest != digest {
			return control.Result{}, admissionFailure(control.CodeRevisionConflict, "operation arguments changed")
		}
		metadata := entry.metadata
		if in.ExpectedRevision < 0 {
			return control.Result{}, admissionFailure(control.CodeInvalidContract, "negative preparation revision")
		}
		observed := time.Now().UTC()
		lookup := journal.PreparationLookup{PreparationID: metadata.PreparationID, PeerID: peerID, RootID: metadata.RootID, Generation: metadata.Generation, ContextEpoch: metadata.ContextEpoch, BindingSHA256: metadata.BindingSHA256, ExpectedRevision: uint64(in.ExpectedRevision), ObservedAt: observed}
		state, err := r.journal.ConfirmPreparation(ctx, lookup, journal.ConfirmationV1{Operation: journal.PreparationOperation{OperationID: in.OperationID, ParamsSHA256: digest}, ChallengeID: answer.ChallengeID, BindingSHA256: answer.BindingSHA256, PeerID: peerID, RootID: metadata.RootID, ContextEpoch: metadata.ContextEpoch, Generation: metadata.Generation, Decision: ab.RetentionApprove, ObservedAt: observed})
		if err != nil {
			return control.Result{}, admissionWireError(err)
		}
		if state != replay.state {
			return control.Result{}, admissionFailure(control.Code("dependency_stale"), "historical confirmation evidence changed")
		}
		return preparedResult(in.OperationID, metadata, state)
	}
	if _, found := s.operations[in.OperationID]; found {
		return control.Result{}, admissionFailure(control.CodeRevisionConflict, "operation method changed")
	}
	if len(s.confirmations) >= 32*128 {
		return control.Result{}, admissionFailure(control.CodeAllowanceExhausted, "confirmation replay capacity reached")
	}
	if err := supervisor.ValidateServicePlan(ctx, entry.plan); err != nil {
		return control.Result{}, admissionWireError(err)
	}
	metadata, err := entry.plan.Metadata()
	if err != nil {
		return control.Result{}, admissionWireError(err)
	}
	if answer.BindingSHA256 != metadata.BindingSHA256 {
		return control.Result{}, admissionFailure(control.CodeInvalidContract, "confirmation binding changed")
	}
	canonical, err := entry.plan.CanonicalBinding()
	if err != nil {
		return control.Result{}, admissionWireError(err)
	}
	binding, err := ab.DecodeV1(canonical)
	if err != nil {
		return control.Result{}, admissionWireError(err)
	}
	if !reflect.DeepEqual(binding.Request, entry.inputs.Request) || binding.Decision.Task.ArtifactRef != entry.inputs.TaskArtifactRef {
		return control.Result{}, admissionFailure(control.Code("dependency_stale"), "preparation inputs changed")
	}

	var challenge *ab.ChallengeV1
	for i := range binding.Challenges {
		if binding.Challenges[i].ID == answer.ChallengeID {
			challenge = &binding.Challenges[i]
			break
		}
	}
	if challenge == nil {
		return control.Result{}, admissionFailure(control.CodeInvalidContract, "confirmation challenge is foreign")
	}
	if !answer.Approved {
		if err := supervisor.InvalidateServicePlan(ctx, entry.plan); err != nil {
			return control.Result{}, admissionWireError(err)
		}
		return control.Result{}, admissionFailure(control.Code("policy_ineligible"), "confirmation rejected; prepare again")
	}
	if in.ExpectedRevision < 0 {
		return control.Result{}, admissionFailure(control.CodeInvalidContract, "negative preparation revision")
	}
	observed := time.Now().UTC()
	lookup := journal.PreparationLookup{PreparationID: metadata.PreparationID, PeerID: peerID, RootID: metadata.RootID, Generation: metadata.Generation, ContextEpoch: metadata.ContextEpoch, BindingSHA256: metadata.BindingSHA256, ExpectedRevision: uint64(in.ExpectedRevision), ObservedAt: observed}
	operation := journal.PreparationOperation{OperationID: in.OperationID, ParamsSHA256: digest}
	var state journal.PreparationState
	if challenge.Kind == ab.RetentionChallengeKind && !entry.retained {
		if in.ExpectedRevision != 0 {
			return control.Result{}, admissionFailure(control.CodeRevisionConflict, "retention requires preparation revision zero")
		}
		fields, fieldErr := ab.DeriveRetentionFieldsV1(binding, s.policy)
		if fieldErr != nil {
			return control.Result{}, admissionWireError(fieldErr)
		}
		fieldSum, digestErr := ab.RetentionFieldSetSHA256(fields)
		if digestErr != nil {
			return control.Result{}, admissionWireError(digestErr)
		}
		approval := ab.RetentionApprovalV1{SchemaVersion: 1, PreparationID: metadata.PreparationID, ChallengeID: challenge.ID, BindingSHA256: metadata.BindingSHA256, FieldSetSHA256: hex.EncodeToString(fieldSum[:]), PolicyRevision: metadata.PolicyRevision, PeerID: peerID, RootID: metadata.RootID, ContextEpoch: metadata.ContextEpoch, Generation: metadata.Generation, ExpiresAt: binding.ExpiresAt, ObservedAt: observed.Format(time.RFC3339Nano), Decision: ab.RetentionApprove}
		if err := admission.CheckRetentionV1(binding, s.policy, &approval, observed); err != nil {
			return control.Result{}, admissionWireError(err)
		}
		if s.beforeEffect != nil {
			s.beforeEffect()
		}
		if err := supervisor.ValidateServicePlan(ctx, entry.plan); err != nil {
			return control.Result{}, admissionWireError(err)
		}
		state, err = r.journal.StoreRetainedPreparation(ctx, journal.RetainedPreparationInput{Operation: operation, CanonicalBinding: canonical, Policy: s.policy, Retention: approval, Lookup: lookup})
		if err == nil {
			entry.retained = true
		}
	} else {
		if !entry.retained {
			return control.Result{}, admissionFailure(control.Code("policy_ineligible"), "approve exact local binding retention first")
		}
		if s.beforeEffect != nil {
			s.beforeEffect()
		}
		if err := supervisor.ValidateServicePlan(ctx, entry.plan); err != nil {
			return control.Result{}, admissionWireError(err)
		}
		state, err = r.journal.ConfirmPreparation(ctx, lookup, journal.ConfirmationV1{Operation: operation, ChallengeID: challenge.ID, BindingSHA256: metadata.BindingSHA256, PeerID: peerID, RootID: metadata.RootID, ContextEpoch: metadata.ContextEpoch, Generation: metadata.Generation, Decision: ab.RetentionApprove, ObservedAt: observed})
	}
	if err != nil {
		return control.Result{}, admissionWireError(err)
	}
	s.confirmations[in.OperationID] = confirmationReplay{peerID: peerID, preparationID: metadata.PreparationID, digest: digest, state: state}
	return preparedResult(in.OperationID, metadata, state)
}

func admissionRequest(w control.RunSpec) (ab.RunSpecV2, error) {
	request := ab.RunSpecV2{SchemaVersion: int64(w.SchemaVersion), Repository: w.Repository, TaskFile: w.TaskFile, TaskSHA256: w.TaskSHA256, Adapter: w.Adapter, Billing: w.Billing, ExecutionProfile: w.ExecutionProfile, Revision: w.Revision, Host: w.Host, Scenario: w.Scenario, UseCommitted: w.UseCommitted, NoChecks: w.NoChecks, KeepGoing: w.KeepGoing, TrustProjectConfig: w.TrustProjectConfig, TrustNativeConfig: w.TrustNativeConfig, DeclareEntitlement: w.DeclareEntitlement, StripCredentialEnv: w.StripCredentialEnv, AllowUntestedNativeVersion: w.AllowUntestedNativeVersion}
	if p := w.Ceilings.ExecutionMillis; p != nil {
		if *p > math.MaxInt64/int64(time.Millisecond) {
			return request, admissionFailure(control.CodeInvalidContract, "execution duration overflow")
		}
		v := *p * int64(time.Millisecond)
		request.Ceilings.ExecutionNanos = &v
	}
	convert := func(p *int) *int64 {
		if p == nil {
			return nil
		}
		v := int64(*p)
		return &v
	}
	request.Ceilings.Repairs = convert(w.Ceilings.Repairs)
	request.Ceilings.Replans = convert(w.Ceilings.Replans)
	request.Ceilings.TransportRetries = convert(w.Ceilings.TransportRetries)
	return request, nil
}

func decodeAdmission[T v2contract.Validator](raw json.RawMessage) (T, error) {
	var zero T
	frame, err := control.Encode(raw)
	if err != nil {
		return zero, err
	}
	return control.Decode[T](frame)
}
