package supervisor

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"os"
	"os/user"
	"reflect"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/turbokast/mythhelm/internal/adapter"
	"github.com/turbokast/mythhelm/internal/admission"
	ab "github.com/turbokast/mythhelm/internal/admissionbinding"
	"github.com/turbokast/mythhelm/internal/billing"
	"github.com/turbokast/mythhelm/internal/control"
	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/security"
	"github.com/turbokast/mythhelm/internal/v2contract"
)

// ServiceInputs contains trusted scope and a closed standalone request.
type ServiceInputs struct {
	RootID, PeerID, ContextEpoch, PrincipalRef, PolicyRevision string
	Generation                                                 int64
	Request                                                    ab.RunSpecV2
	TaskArtifactRef                                            string
	Context                                                    *ExecutionContext
}

// ExecutionContextOptions comes from trusted service composition only.
type ExecutionContextOptions struct {
	PolicyRevision  string
	RetentionPolicy ab.RetentionPolicyV1
}

// ExecutionContextMetadata contains references, never environment values.
type ExecutionContextMetadata struct {
	RootID, ContextEpoch, PrincipalRef, PolicyRevision, NativeConfigIdentity string
	Generation                                                               int64
}

// ExecutionContext is a service-owned lifetime; copies share revocation.
type ExecutionContext struct{ state *executionContextState }
type executionContextState struct {
	mu       sync.Mutex // guards closed and private context data
	closed   bool
	metadata ExecutionContextMetadata
	journal  *journal.Journal
	lease    *control.InstanceLease
	policy   ab.RetentionPolicyV1
	env      []string
	plans    map[string]*ServicePlan
}

func serviceAdmissionError(code v2contract.Code) error {
	return &v2contract.ControlError{Code: code, Owner: "supervisor", OperationID: "service.admission", Disposition: code.DefaultDisposition(), NextAction: "prepare again using the current standalone service context"}
}

// NewExecutionContext verifies the adopted root and captures only service
// allowlisted environment. It performs no native startup and grants no consent.
func NewExecutionContext(ctx context.Context, j *journal.Journal, lease *control.InstanceLease, options ExecutionContextOptions) (*ExecutionContext, error) {
	if j == nil || lease == nil {
		return nil, serviceAdmissionError(v2contract.CodeInvalidContract)
	}
	release, err := lease.Hold()
	if err != nil {
		return nil, serviceAdmissionError(v2contract.CodeInvalidContract)
	}
	defer release()
	if err := lease.CheckRootIdentity(ctx, j.StateDir()); err != nil {
		return nil, serviceAdmissionError(v2contract.CodeDependencyStale)
	}
	if err := j.CheckStateIdentity(ctx); err != nil {
		return nil, serviceAdmissionError(v2contract.CodeDependencyStale)
	}
	if err := sameRoot(lease.Root(), j.StateDir()); err != nil {
		return nil, serviceAdmissionError(v2contract.CodeInvalidContract)
	}
	var phase string
	err = j.Transact(ctx, func(tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `SELECT phase FROM migration_state WHERE id=1`).Scan(&phase)
	})
	if err != nil || phase != "adopted" {
		return nil, serviceAdmissionError(v2contract.CodeOwnershipUnresolved)
	}
	if err := options.RetentionPolicy.Validate(); err != nil {
		return nil, serviceAdmissionError(v2contract.CodeCapabilityUnsupported)
	}
	if options.PolicyRevision != options.RetentionPolicy.Revision {
		return nil, serviceAdmissionError(v2contract.CodeInvalidContract)
	}
	sum := sha256.Sum256([]byte(lease.Root()))
	rootID := "root_" + hex.EncodeToString(sum[:])
	if options.RetentionPolicy.Content.RootID != rootID {
		return nil, serviceAdmissionError(v2contract.CodeInvalidContract)
	}
	principal, err := executionPrincipal()
	if err != nil {
		return nil, err
	}
	env, err := security.BuildEnv(os.Environ(), nil, nil)
	if err != nil {
		return nil, serviceAdmissionError(v2contract.CodeInvalidContract)
	}
	raw, err := ab.EncodeRetentionPolicyV1(options.RetentionPolicy)
	if err != nil {
		return nil, serviceAdmissionError(v2contract.CodeInvalidContract)
	}
	policy, err := ab.DecodeRetentionPolicyV1(raw)
	if err != nil {
		return nil, serviceAdmissionError(v2contract.CodeInvalidContract)
	}
	metadata := ExecutionContextMetadata{RootID: rootID, ContextEpoch: ids.New("epoch"), PrincipalRef: principal, PolicyRevision: options.PolicyRevision, Generation: lease.Generation()}
	return &ExecutionContext{state: &executionContextState{metadata: metadata, journal: j, lease: lease, policy: policy, env: env, plans: map[string]*ServicePlan{}}}, nil
}

// Metadata returns copied context identity for a live held generation.
func (c *ExecutionContext) Metadata() (ExecutionContextMetadata, error) {
	if c == nil || c.state == nil {
		return ExecutionContextMetadata{}, serviceAdmissionError(v2contract.CodeInvalidContract)
	}
	s := c.state
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ExecutionContextMetadata{}, serviceAdmissionError(v2contract.CodeInvalidContract)
	}
	if s.lease.Generation() != s.metadata.Generation {
		return ExecutionContextMetadata{}, serviceAdmissionError(v2contract.CodeDependencyStale)
	}
	return s.metadata, nil
}

// Close invalidates this epoch and every plan tied to it; it waits for no owner.
func (c *ExecutionContext) Close() error {
	if c == nil || c.state == nil {
		return serviceAdmissionError(v2contract.CodeInvalidContract)
	}
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	c.state.closed = true
	c.state.env = nil
	c.state.plans = nil
	return nil
}

func executionPrincipal() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", serviceAdmissionError(v2contract.CodeCapabilityUnsupported)
	}
	sum := sha256.Sum256([]byte(u.Uid + "\x00" + u.Username))
	return "principal_" + hex.EncodeToString(sum[:]), nil
}

// PlanMetadata identifies immutable, expiring admission evidence.
type PlanMetadata struct {
	PreparationID, BindingSHA256, RootID, PeerID, ContextEpoch, PolicyRevision string
	RunID, TaskID, AttemptID                                                   string
	Generation                                                                 int64
	ExpiresAt                                                                  time.Time
	Challenges                                                                 []ab.ChallengeV1
}

// PermitMetadata is a copied identity of a single-use launch capability.
type PermitMetadata struct {
	Plan              PlanMetadata
	PermitNonceSHA256 string
}

// ServicePlan keeps the decision and canonical authority plan private.
type ServicePlan struct{ state *servicePlanState }
type servicePlanState struct {
	mu         sync.Mutex // guards invalidation and last observed time
	context    *ExecutionContext
	decision   admission.Decision
	binding    ab.BindingV1
	canonical  []byte
	metadata   PlanMetadata
	observedAt time.Time
	repoInfo   os.FileInfo
	invalid    bool
}

// LaunchPermit has one shared consumption state even when copied.
type LaunchPermit struct{ state *launchPermitState }
type launchPermitState struct {
	mu       sync.Mutex // guards consumed
	consumed bool
	plan     *ServicePlan
	decision admission.Decision
	metadata PermitMetadata
}

// Metadata returns copied plan identity without resolving external sources.
func (p *ServicePlan) Metadata() (PlanMetadata, error) {
	if p == nil || p.state == nil {
		return PlanMetadata{}, serviceAdmissionError(v2contract.CodeInvalidContract)
	}
	s := p.state
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.check(); err != nil {
		return PlanMetadata{}, err
	}
	m := s.metadata
	m.Challenges = slices.Clone(m.Challenges)
	return m, nil
}

// CanonicalBinding returns copied nonsecret bytes, never a Decision or Env.
func (p *ServicePlan) CanonicalBinding() ([]byte, error) {
	if p == nil || p.state == nil {
		return nil, serviceAdmissionError(v2contract.CodeInvalidContract)
	}
	s := p.state
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.check(); err != nil {
		return nil, err
	}
	return bytes.Clone(s.canonical), nil
}

func (s *servicePlanState) check() error {
	if s.invalid {
		return serviceAdmissionError(v2contract.CodeDependencyStale)
	}
	if _, err := s.context.Metadata(); err != nil {
		return err
	}
	now := time.Now().UTC()
	if now.Before(s.observedAt) || !now.Before(s.metadata.ExpiresAt) {
		s.invalid = true
		return serviceAdmissionError(v2contract.CodeDependencyStale)
	}
	s.observedAt = now
	return nil
}

// Metadata refuses revoked, expired and consumed permit handles.
func (p *LaunchPermit) Metadata() (PermitMetadata, error) {
	if p == nil || p.state == nil {
		return PermitMetadata{}, serviceAdmissionError(v2contract.CodeInvalidContract)
	}
	s := p.state
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.consumed {
		return PermitMetadata{}, serviceAdmissionError(v2contract.CodeInvalidContract)
	}
	if _, err := s.plan.Metadata(); err != nil {
		return PermitMetadata{}, err
	}
	m := s.metadata
	m.Plan.Challenges = slices.Clone(m.Plan.Challenges)
	return m, nil
}

// PrepareService resolves the complete closed request using only context-owned
// environment and seals a bounded RAM plan. It persists neither consent nor work.
func PrepareService(ctx context.Context, in ServiceInputs) (*ServicePlan, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	in.Request = copyServiceRequest(in.Request)
	metadata, env, err := serviceInputs(ctx, in)
	if err != nil {
		return nil, err
	}
	req, err := serviceRequest(in.Request, env, in.Context.state.journal.StateDir())
	if err != nil {
		return nil, err
	}
	d, challenges, err := admission.PrepareService(ctx, req)
	if err != nil {
		return nil, err
	}
	if d.RepoIdentity, err = admission.RepoIdentity(d.Snapshot.SourceRepo); err != nil {
		return nil, serviceAdmissionError(v2contract.CodeDependencyStale)
	}
	now := time.Now().UTC()
	b := ab.BindingV1{SchemaVersion: 1, PreparationID: ids.New("prep"), PeerID: in.PeerID, RootID: metadata.RootID, Generation: metadata.Generation, ContextEpoch: metadata.ContextEpoch, PolicyRevision: metadata.PolicyRevision, ExpiresAt: now.Add(5 * time.Minute).Format(time.RFC3339Nano), RunID: d.RunID, TaskID: d.TaskID, AttemptID: d.AttemptID, Request: in.Request, Challenges: []ab.ChallengeV1{}}
	for _, ch := range challenges {
		ch.ID = ids.New("challenge")
		b.Challenges = append(b.Challenges, ch)
	}
	b.Challenges = append(b.Challenges, ab.ChallengeV1{ID: ids.New("challenge"), Kind: ab.RetentionChallengeKind, Text: serviceRetentionText(in.Context.state.lease.Root())})
	b.Decision, err = serviceDecisionBinding(d, metadata, in.TaskArtifactRef)
	if err != nil {
		return nil, err
	}
	if err := validateServiceRetentionPlan(b.Challenges); err != nil {
		return nil, err
	}
	canonical, err := ab.EncodeV1(b)
	if err != nil {
		return nil, serviceAdmissionError(v2contract.CodeInvalidContract)
	}
	sum, err := ab.DigestV1(b)
	if err != nil {
		return nil, serviceAdmissionError(v2contract.CodeInvalidContract)
	}
	expires, _ := time.Parse(time.RFC3339Nano, b.ExpiresAt)
	m := PlanMetadata{PreparationID: b.PreparationID, BindingSHA256: hex.EncodeToString(sum[:]), RootID: b.RootID, PeerID: b.PeerID, ContextEpoch: b.ContextEpoch, PolicyRevision: b.PolicyRevision, RunID: b.RunID, TaskID: b.TaskID, AttemptID: b.AttemptID, Generation: b.Generation, ExpiresAt: expires, Challenges: slices.Clone(b.Challenges)}
	repoInfo, err := os.Stat(d.Snapshot.SourceRepo)
	if err != nil {
		return nil, serviceAdmissionError(v2contract.CodeDependencyStale)
	}
	sealed, err := ab.DecodeV1(canonical)
	if err != nil {
		return nil, serviceAdmissionError(v2contract.CodeInvalidContract)
	}
	plan := &ServicePlan{state: &servicePlanState{context: in.Context, decision: d, binding: sealed, canonical: canonical, metadata: m, observedAt: now, repoInfo: repoInfo}}
	in.Context.state.mu.Lock()
	defer in.Context.state.mu.Unlock()
	if in.Context.state.closed {
		return nil, serviceAdmissionError(v2contract.CodeInvalidContract)
	}
	for id, prior := range in.Context.state.plans {
		if !now.Before(prior.state.metadata.ExpiresAt) {
			delete(in.Context.state.plans, id)
		}
	}
	if len(in.Context.state.plans) >= 32 {
		return nil, serviceAdmissionError(v2contract.CodeAllowanceExhausted)
	}
	peerCount := 0
	for _, prior := range in.Context.state.plans {
		if prior.state.metadata.PeerID == in.PeerID {
			peerCount++
		}
	}
	if peerCount >= 8 {
		return nil, serviceAdmissionError(v2contract.CodeAllowanceExhausted)
	}
	in.Context.state.plans[b.PreparationID] = plan
	return plan, nil
}

func serviceInputs(ctx context.Context, in ServiceInputs) (ExecutionContextMetadata, []string, error) {
	if err := ctx.Err(); err != nil {
		return ExecutionContextMetadata{}, nil, err
	}
	metadata, err := in.Context.Metadata()
	if err != nil {
		return metadata, nil, err
	}
	if in.RootID != metadata.RootID || in.ContextEpoch != metadata.ContextEpoch || in.PrincipalRef != metadata.PrincipalRef || in.PolicyRevision != metadata.PolicyRevision || in.Generation != metadata.Generation || in.PeerID == "" {
		return metadata, nil, serviceAdmissionError(v2contract.CodePermissionDenied)
	}
	s := in.Context.state
	if err := s.lease.CheckRootIdentity(ctx, s.journal.StateDir()); err != nil {
		return metadata, nil, serviceAdmissionError(v2contract.CodeDependencyStale)
	}
	if err := s.journal.CheckStateIdentity(ctx); err != nil {
		return metadata, nil, serviceAdmissionError(v2contract.CodeDependencyStale)
	}
	principal, err := executionPrincipal()
	if err != nil || principal != metadata.PrincipalRef {
		return metadata, nil, serviceAdmissionError(v2contract.CodeDependencyStale)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return metadata, nil, serviceAdmissionError(v2contract.CodeInvalidContract)
	}
	return metadata, slices.Clone(s.env), nil
}

func serviceRequest(in ab.RunSpecV2, env []string, stateDir string) (admission.Request, error) {
	if in.SchemaVersion != 2 {
		return admission.Request{}, serviceAdmissionError(v2contract.CodeInvalidContract)
	}
	req := admission.Request{StateDir: stateDir, Repo: in.Repository, TaskFile: in.TaskFile, Adapter: in.Adapter, Billing: in.Billing, ExecutionProfile: in.ExecutionProfile, UseCommitted: in.UseCommitted, Rev: in.Revision, Host: in.Host, Scenario: in.Scenario, Env: env, TrustProjectConfig: in.TrustProjectConfig, NoChecks: in.NoChecks, KeepGoing: in.KeepGoing, StripCredentialEnv: in.StripCredentialEnv, TrustNativeConfig: in.TrustNativeConfig, DeclareEntitlement: in.DeclareEntitlement, AllowUntestedNativeVersion: in.AllowUntestedNativeVersion}
	c := in.Ceilings
	if c.ExecutionNanos != nil || c.Repairs != nil || c.Replans != nil || c.TransportRetries != nil {
		flags := billing.Ceilings{Execution: -1, Repairs: -1, Replans: -1, TransportRetries: -1}
		if c.ExecutionNanos != nil {
			if *c.ExecutionNanos <= 0 {
				return req, serviceAdmissionError(v2contract.CodeInvalidContract)
			}
			flags.Execution = time.Duration(*c.ExecutionNanos)
		}
		for _, v := range []struct {
			in  *int64
			out *int
		}{{c.Repairs, &flags.Repairs}, {c.Replans, &flags.Replans}, {c.TransportRetries, &flags.TransportRetries}} {
			if v.in != nil {
				if *v.in < 0 || int64(int(*v.in)) != *v.in {
					return req, serviceAdmissionError(v2contract.CodeInvalidContract)
				}
				*v.out = int(*v.in)
			}
		}
		req.EnvelopeFlags = &flags
	}
	return req, nil
}

func serviceDecisionBinding(d admission.Decision, m ExecutionContextMetadata, artefact string) (ab.DecisionV1, error) {
	// Native status/auth/config are unavailable without an actual approved policy.
	if d.NativeAuth != nil || d.Declaration != nil || d.NativeConfigDigest != "" || d.Profile.Boundary != nil {
		return ab.DecisionV1{}, serviceAdmissionError(v2contract.CodeCapabilityUnsupported)
	}
	cfg := d.ProjectConfig
	pc := ab.ProjectConfigV1{SchemaVersion: int64(cfg.SchemaVersion), Passthrough: append([]string{}, cfg.Environment.Passthrough...), AllowedTools: append([]string{}, cfg.Adapters.ClaudeCode.AllowedTools...), Checks: []ab.CheckV1{}, Envelopes: ab.ConfigEnvelopesV1{Execution: cfg.Envelopes.Execution, Repairs: serviceInt(cfg.Envelopes.Repairs), Replans: serviceInt(cfg.Envelopes.Replans), TransportRetries: serviceInt(cfg.Envelopes.TransportRetries)}}
	for _, check := range cfg.Checks {
		pc.Checks = append(pc.Checks, ab.CheckV1{Name: check.Name, Argv: append([]string{}, check.Argv...), FailOnOutput: check.FailOnOutput, Timeout: check.Timeout})
	}
	file, err := cfg.Envelopes.ToCeilings()
	if err != nil {
		return ab.DecisionV1{}, serviceAdmissionError(v2contract.CodeInvalidContract)
	}
	c := billing.ResolveCeilings(d.EnvelopeFlags, file)
	p := d.Proposal
	proposal := ab.ProposalV1{Path: p.Spec.Path, Args: append([]string{}, p.Spec.Args...), Dir: p.Spec.Dir, StopLadder: []ab.StopV1{}, Overrides: []ab.OverrideV1{}, Manifest: []ab.DigestSourceV1{}, Billing: ab.BillingV1{Mode: p.Billing.Mode, CredentialProvenance: p.Billing.CredentialProvenance, EntitlementClass: p.Billing.EntitlementClass, EntitlementSource: p.Billing.EntitlementSource, PaidContinuation: p.Billing.PaidContinuation, PaidContinuationUserDeclaration: p.Billing.PaidContinuationUserDeclaration, Qualified: p.Billing.Qualified, G05: p.Billing.G05}, Capabilities: serviceCapabilities(p.Capabilities)}
	for _, step := range p.StopLadder {
		proposal.StopLadder = append(proposal.StopLadder, ab.StopV1{Signal: string(step.Signal), GraceNanos: int64(step.Grace)})
	}
	for _, delta := range p.Overrides {
		proposal.Overrides = append(proposal.Overrides, ab.OverrideV1{Kind: delta.Kind, Name: delta.Name, Value: delta.Value, Reason: delta.Reason})
	}
	for source, digest := range p.Manifest.Digests {
		proposal.Manifest = append(proposal.Manifest, ab.DigestSourceV1{Source: source, Digest: digest})
	}
	slices.SortFunc(proposal.Manifest, func(a, b ab.DigestSourceV1) int { return strings.Compare(a.Source, b.Source) })
	names := append([]string{}, cfg.Environment.Passthrough...)
	slices.Sort(names)
	names = slices.Compact(names)
	result := ab.DecisionV1{StateRootID: m.RootID, RunID: d.RunID, TaskID: d.TaskID, AttemptID: d.AttemptID, RunDir: d.RunDir, Workdir: d.Workdir, Host: d.Host, Scenario: d.Scenario, GitName: d.GitName, GitEmail: d.GitEmail, ProjectConfig: pc, ConfigDigest: d.ConfigDigest, RepoIdentity: d.RepoIdentity, RecordTrust: d.RecordTrust, NoChecks: d.NoChecks, KeepGoing: d.KeepGoing, ResolvedCeilings: ab.FiniteCeilingsV1{ExecutionNanos: int64(c.Execution), Repairs: int64(c.Repairs), Replans: int64(c.Replans), TransportRetries: int64(c.TransportRetries)}, Task: ab.TaskV1{SHA256: d.Task.SHA256, Title: d.Task.Title, Bytes: int64(len(d.Task.Content)), ArtifactRef: artefact}, Snapshot: ab.SnapshotV1{SourceRepo: d.Snapshot.SourceRepo, Branch: d.Snapshot.Branch, BaseRev: d.Snapshot.BaseRev, Dirty: d.Snapshot.Dirty, Source: d.Snapshot.Source}, Profile: ab.ProfileV1{Name: d.Profile.Name, Contained: d.Profile.Contained, Consent: d.Profile.Consent, Disclosure: d.Profile.Disclosure}, Adapter: ab.DescriptorV1{ID: d.Adapter.ID, Version: d.Adapter.Version, Harness: d.Adapter.Harness, Surface: d.Adapter.Surface}, Probe: ab.ProbeV1{Executable: d.Probe.Executable, Version: d.Probe.Version, SHA256: d.Probe.SHA256, OS: d.Probe.OS, Arch: d.Probe.Arch, Compatibility: d.Probe.Compatibility}, Proposal: proposal, Environment: ab.EnvironmentV1{PolicyRevision: m.PolicyRevision, ContextEpoch: m.ContextEpoch, PrincipalRef: m.PrincipalRef, NativeConfigIdentity: m.NativeConfigIdentity, RequiredNames: names, Destinations: []string{"local state directory", "no network (scripted fake agent)"}}}
	if d.EnvelopeFlags != nil {
		flags := ab.OptionalCeilingsV1{}
		f := d.EnvelopeFlags
		if f.Execution >= 0 {
			v := int64(f.Execution)
			flags.ExecutionNanos = &v
		}
		if f.Repairs >= 0 {
			v := int64(f.Repairs)
			flags.Repairs = &v
		}
		if f.Replans >= 0 {
			v := int64(f.Replans)
			flags.Replans = &v
		}
		if f.TransportRetries >= 0 {
			v := int64(f.TransportRetries)
			flags.TransportRetries = &v
		}
		result.EnvelopeFlags = &flags
	}
	return result, nil
}

func serviceInt(v *int) *int64 {
	if v == nil {
		return nil
	}
	out := int64(*v)
	return &out
}

func serviceCapabilities(c adapter.CapabilityRecord) ab.CapabilityV1 {
	tri := func(v adapter.Tri) ab.TriV1 { return ab.TriV1(v) }
	optional := func(v adapter.Tri) *ab.TriV1 {
		if v == "" {
			return nil
		}
		out := tri(v)
		return &out
	}
	var evidence *string
	if c.Billing.EvidenceID != nil {
		value := *c.Billing.EvidenceID
		evidence = &value
	}
	return ab.CapabilityV1{SchemaVersion: int64(c.SchemaVersion), AdapterID: c.AdapterID, AdapterVersion: c.AdapterVersion, RuntimeVersion: c.RuntimeVersion, Mode: c.Mode, ExecutionSurface: c.ExecutionSurface, HarnessID: c.HarnessID, FidelityQualification: c.FidelityQualification, Billing: ab.BillingCapabilityV1{Entitlement: c.Billing.Entitlement, IncludedOnlySupported: tri(c.Billing.IncludedOnlySupported), PaidOveragePrevention: tri(c.Billing.PaidOveragePrevention), EvidenceID: evidence}, HostIntegration: ab.HostIntegrationV1{HerdrEmbedded: tri(c.HostIntegration.HerdrEmbedded), HerdrStateBridge: tri(c.HostIntegration.HerdrStateBridge), NativeInteractiveAttach: tri(c.HostIntegration.NativeInteractiveAttach)}, Capabilities: ab.OperationsV1{StructuredEvents: tri(c.Capabilities.StructuredEvents), Resume: tri(c.Capabilities.Resume), LiveSteer: tri(c.Capabilities.LiveSteer), ApprovalBridge: tri(c.Capabilities.ApprovalBridge), UsageTokens: tri(c.Capabilities.UsageTokens), QuotaRemaining: tri(c.Capabilities.QuotaRemaining), HardMonetaryLimit: tri(c.Capabilities.HardMonetaryLimit), NativeSubagents: tri(c.Capabilities.NativeSubagents), Reconnect: optional(c.Capabilities.Reconnect), ModelMetadata: optional(c.Capabilities.ModelMetadata)}, Sandbox: ab.SandboxV1{Status: tri(c.Sandbox.Status), Scope: c.Sandbox.Scope}, Platform: ab.PlatformV1{OS: c.Platform.OS, WorkerDetachment: tri(c.Platform.WorkerDetachment), ProcessTreeOwnership: tri(c.Platform.ProcessTreeOwnership)}, Qualification: c.Qualification}
}

func copyServiceRequest(in ab.RunSpecV2) ab.RunSpecV2 {
	clone := func(p *int64) *int64 {
		if p == nil {
			return nil
		}
		v := *p
		return &v
	}
	in.Ceilings = ab.OptionalCeilingsV1{ExecutionNanos: clone(in.Ceilings.ExecutionNanos), Repairs: clone(in.Ceilings.Repairs), Replans: clone(in.Ceilings.Replans), TransportRetries: clone(in.Ceilings.TransportRetries)}
	return in
}

// fresh rederives every currently supported source before a service boundary.
// Planned IDs are service-owned; rebinding only their fixed fake recipe slots
// prevents newly allocated IDs from disguising genuine source drift.
func (s *servicePlanState) fresh(ctx context.Context) (admission.Decision, error) {
	if s.invalid {
		return admission.Decision{}, serviceAdmissionError(v2contract.CodeDependencyStale)
	}
	m, err := s.context.Metadata()
	if err != nil {
		return admission.Decision{}, err
	}
	b := s.binding
	info, err := os.Stat(b.Decision.Snapshot.SourceRepo)
	if err != nil || s.repoInfo == nil || !os.SameFile(s.repoInfo, info) {
		return admission.Decision{}, serviceAdmissionError(v2contract.CodeDependencyStale)
	}
	s.context.state.mu.Lock()
	env := slices.Clone(s.context.state.env)
	s.context.state.mu.Unlock()
	req, err := serviceRequest(b.Request, env, s.context.state.journal.StateDir())
	if err != nil {
		return admission.Decision{}, err
	}
	d, challenges, err := admission.PrepareService(ctx, req)
	if err != nil {
		return admission.Decision{}, serviceAdmissionError(v2contract.CodeDependencyStale)
	}
	if d.Adapter.ID != "builtin/fake" {
		return admission.Decision{}, serviceAdmissionError(v2contract.CodeCapabilityUnsupported)
	}
	if err := validateServiceRetentionPlan(b.Challenges); err != nil {
		return admission.Decision{}, err
	}
	if len(challenges)+1 != len(b.Challenges) {
		return admission.Decision{}, serviceAdmissionError(v2contract.CodeDependencyStale)
	}
	for i, ch := range challenges {
		if ch.Kind != b.Challenges[i].Kind || ch.Text != b.Challenges[i].Text {
			return admission.Decision{}, serviceAdmissionError(v2contract.CodeDependencyStale)
		}
	}
	retention := b.Challenges[len(b.Challenges)-1]
	if retention.Kind != ab.RetentionChallengeKind || retention.Text != serviceRetentionText(s.context.state.lease.Root()) {
		return admission.Decision{}, serviceAdmissionError(v2contract.CodeDependencyStale)
	}
	d.RunID, d.TaskID, d.AttemptID = b.RunID, b.TaskID, b.AttemptID
	d.RunDir, d.Workdir = b.Decision.RunDir, b.Decision.Workdir
	if len(d.Proposal.Spec.Args) != 5 || d.Proposal.Spec.Args[0] != "__fake-agent" || d.Proposal.Spec.Args[3] != "--workdir" {
		return admission.Decision{}, serviceAdmissionError(v2contract.CodeDependencyStale)
	}
	d.Proposal.Spec.Args[4] = d.Workdir
	d.Proposal.Spec.Dir = d.Workdir
	for i, v := range d.Proposal.Spec.Env {
		if strings.HasPrefix(v, "MYTHHELM_ATTEMPT_ID=") {
			d.Proposal.Spec.Env[i] = "MYTHHELM_ATTEMPT_ID=" + d.AttemptID
		}
	}
	for i, v := range d.Proposal.Overrides {
		if v.Kind == "env" && v.Name == "MYTHHELM_ATTEMPT_ID" {
			d.Proposal.Overrides[i].Value = d.AttemptID
		}
	}
	d.RepoIdentity, err = admission.RepoIdentity(d.Snapshot.SourceRepo)
	if err != nil {
		return admission.Decision{}, serviceAdmissionError(v2contract.CodeDependencyStale)
	}
	decision, err := serviceDecisionBinding(d, m, b.Decision.Task.ArtifactRef)
	if err != nil {
		return admission.Decision{}, err
	}
	if !reflect.DeepEqual(decision, b.Decision) {
		return admission.Decision{}, serviceAdmissionError(v2contract.CodeDependencyStale)
	}
	// The original file is only a drift observer. The frozen, service-owned
	// RAM artefact remains the sole task-content input to a fresh permit.
	artefact := s.decision.Task
	taskSum := sha256.Sum256(artefact.Content)
	if len(artefact.Content) > admission.MaxTaskBytes || int64(len(artefact.Content)) != b.Decision.Task.Bytes || hex.EncodeToString(taskSum[:]) != b.Decision.Task.SHA256 || artefact.SHA256 != b.Decision.Task.SHA256 || artefact.Title != b.Decision.Task.Title || !bytes.Equal(d.Task.Content, artefact.Content) {
		return admission.Decision{}, serviceAdmissionError(v2contract.CodeDependencyStale)
	}
	d.Task = admission.Task{Content: bytes.Clone(artefact.Content), SHA256: artefact.SHA256, Title: artefact.Title}

	canonical, err := ab.EncodeV1(b)
	if err != nil || !bytes.Equal(canonical, s.canonical) || !reflect.DeepEqual(b.Challenges, s.metadata.Challenges) {
		return admission.Decision{}, serviceAdmissionError(v2contract.CodeDependencyStale)
	}
	return d, nil
}

func serviceRetentionText(root string) string {
	return "Retain the local run binding in " + root + " for local run preparation; review repository paths, Git identity, task title, command arguments and challenge text under the configured retention policy."
}

// RevalidateService reloads the current confirmed row and reconstructs every
// source before sealing one fresh, private, single-use permit. It never starts.
func RevalidateService(ctx context.Context, confirmed journal.ConfirmedPreparation, in ServiceInputs) (*LaunchPermit, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_, _, err := serviceInputs(ctx, in)
	if err != nil {
		return nil, err
	}
	raw, err := confirmed.CanonicalBinding()
	if err != nil {
		return nil, err
	}
	b, err := ab.DecodeV1(raw)
	if err != nil {
		return nil, serviceAdmissionError(v2contract.CodeInvalidContract)
	}
	meta, err := confirmed.Metadata()
	if err != nil {
		return nil, err
	}
	if !meta.Ready || meta.Consumed || !reflect.DeepEqual(b.Request, in.Request) || b.Decision.Task.ArtifactRef != in.TaskArtifactRef || b.RootID != in.RootID || b.PeerID != in.PeerID || b.Generation != in.Generation || b.ContextEpoch != in.ContextEpoch || b.PolicyRevision != in.PolicyRevision {
		return nil, serviceAdmissionError(v2contract.CodeDependencyStale)
	}
	lookup := journal.PreparationLookup{PreparationID: b.PreparationID, PeerID: in.PeerID, RootID: in.RootID, ContextEpoch: in.ContextEpoch, Generation: in.Generation, BindingSHA256: meta.BindingSHA256, ExpectedRevision: meta.Revision, ObservedAt: time.Now().UTC()}
	current, err := in.Context.state.journal.ReadConfirmedPreparation(ctx, lookup)
	if err != nil {
		return nil, err
	}
	currentRaw, err := current.CanonicalBinding()
	if err != nil || !bytes.Equal(currentRaw, raw) {
		return nil, serviceAdmissionError(v2contract.CodeDependencyStale)
	}
	// The original live plan is mandatory; durable bytes alone cannot recreate
	// a private authority plan after restart or context replacement.
	in.Context.state.mu.Lock()
	plan := in.Context.state.plans[b.PreparationID]
	in.Context.state.mu.Unlock()
	if plan == nil {
		return nil, serviceAdmissionError(v2contract.CodeDependencyStale)
	}
	s := plan.state
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.check(); err != nil {
		return nil, err
	}
	if !bytes.Equal(raw, s.canonical) {
		return nil, serviceAdmissionError(v2contract.CodeDependencyStale)
	}
	d, err := s.fresh(ctx)
	if err != nil {
		s.invalid = true
		return nil, err
	}
	nonce := sha256.Sum256([]byte(ids.New("permit")))
	m := s.metadata
	m.Challenges = slices.Clone(m.Challenges)
	return &LaunchPermit{state: &launchPermitState{plan: plan, decision: d, metadata: PermitMetadata{Plan: m, PermitNonceSHA256: hex.EncodeToString(nonce[:])}}}, nil
}

// ValidateServicePlan checks fresh sources against the immutable original plan
// before an acknowledgement or confirmation effect. It grants no authority.
func ValidateServicePlan(ctx context.Context, plan *ServicePlan) error {
	if plan == nil || plan.state == nil {
		return serviceAdmissionError(v2contract.CodeInvalidContract)
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	s := plan.state
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.check(); err != nil {
		return err
	}
	c := s.context.state
	if err := c.lease.CheckRootIdentity(ctx, c.journal.StateDir()); err != nil {
		s.invalid = true
		return serviceAdmissionError(v2contract.CodeDependencyStale)
	}
	if err := c.journal.CheckStateIdentity(ctx); err != nil {
		s.invalid = true
		return serviceAdmissionError(v2contract.CodeDependencyStale)
	}
	principal, err := executionPrincipal()
	if err != nil || principal != c.metadata.PrincipalRef {
		s.invalid = true
		return serviceAdmissionError(v2contract.CodeDependencyStale)
	}
	if _, err := s.fresh(ctx); err != nil {
		s.invalid = true
		return err
	}
	return ctx.Err()
}

func validateServiceRetentionPlan(challenges []ab.ChallengeV1) error {
	count := 0
	for _, challenge := range challenges {
		if challenge.Kind == ab.RetentionChallengeKind {
			count++
		}
	}
	if len(challenges) == 0 || count != 1 {
		return &v2contract.ControlError{Code: v2contract.CodePolicyIneligible, Owner: "supervisor", OperationID: "service.admission", Disposition: v2contract.CodePolicyIneligible.DefaultDisposition(), NextAction: "prepare again with exact local retention authority", Detail: map[string]string{"mythhelm.retention/reason": "missing_retention_authority"}}
	}
	return nil
}

// InvalidateServicePlan revokes one shared private plan and its outstanding
// permits without changing another context, durable evidence or an owner.
func InvalidateServicePlan(ctx context.Context, plan *ServicePlan) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if plan == nil || plan.state == nil {
		return serviceAdmissionError(v2contract.CodeInvalidContract)
	}
	plan.state.mu.Lock()
	defer plan.state.mu.Unlock()
	plan.state.invalid = true
	return nil
}
