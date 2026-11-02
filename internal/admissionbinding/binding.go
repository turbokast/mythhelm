// Package admissionbinding encodes immutable admission evidence. Structural
// validity supplies no authority, retention approval or runtime qualification.
package admissionbinding

// TriV1 records an explicit supported, unsupported or unknown capability.
type TriV1 string

// BindingV1 records the immutable version-1 evidence record.
type BindingV1 struct {
	SchemaVersion                           int64 // exactly 1
	PreparationID, PeerID, RootID           string
	Generation                              int64
	ContextEpoch, PolicyRevision, ExpiresAt string
	RunID, TaskID, AttemptID                string
	Request                                 RunSpecV2
	Decision                                DecisionV1
	Challenges                              []ChallengeV1
}

// ChallengeV1 records challenge identity and display text without the cyclic digest.
type ChallengeV1 struct{ ID, Kind, Text string } // digest is outside to avoid a hash cycle
// RunSpecV2 records the exact standalone request provenance.
type RunSpecV2 struct {
	SchemaVersion                                                        int64 // exactly 2
	Repository, TaskFile, TaskSHA256, Adapter, Billing, ExecutionProfile string
	Revision, Host, Scenario                                             string
	UseCommitted, NoChecks, KeepGoing                                    bool
	TrustProjectConfig, TrustNativeConfig, DeclareEntitlement            string
	StripCredentialEnv, AllowUntestedNativeVersion                       bool
	Ceilings                                                             OptionalCeilingsV1
}

// OptionalCeilingsV1 records nullable per-field execution inputs.
type OptionalCeilingsV1 struct {
	ExecutionNanos, Repairs, Replans, TransportRetries *int64
}

// FiniteCeilingsV1 records the resolved finite execution envelope.
type FiniteCeilingsV1 struct{ ExecutionNanos, Repairs, Replans, TransportRetries int64 }

// DecisionV1 records the closed nonsecret decision evidence.
type DecisionV1 struct {
	StateRootID, RunID, TaskID, AttemptID, RunDir, Workdir, Host, Scenario string
	GitName, GitEmail                                                      string
	ProjectConfig                                                          ProjectConfigV1
	ConfigDigest, RepoIdentity                                             string
	RecordTrust, NoChecks, KeepGoing                                       bool
	EnvelopeFlags                                                          *OptionalCeilingsV1
	ResolvedCeilings                                                       FiniteCeilingsV1
	Declaration                                                            *DeclarationV1
	RecordNativeTrust                                                      bool
	NativeConfigDigest                                                     string
	NativeHooks                                                            *int64
	NativeTrustGrant                                                       string
	NativeAuth                                                             *AuthV1
	Task                                                                   TaskV1
	Snapshot                                                               SnapshotV1
	Profile                                                                ProfileV1
	Adapter                                                                DescriptorV1
	Probe                                                                  ProbeV1
	Proposal                                                               ProposalV1
	Environment                                                            EnvironmentV1
}

// ProjectConfigV1 records the parsed immutable project configuration.
type ProjectConfigV1 struct {
	SchemaVersion             int64
	Passthrough, AllowedTools []string
	Checks                    []CheckV1
	Envelopes                 ConfigEnvelopesV1
}

// CheckV1 records one ordered check and its execution policy.
type CheckV1 struct {
	Name         string
	Argv         []string
	FailOnOutput bool
	Timeout      string
}

// ConfigEnvelopesV1 records nullable project envelope inputs.
type ConfigEnvelopesV1 struct {
	Execution                          string
	Repairs, Replans, TransportRetries *int64
}

// DeclarationV1 records a user assertion rather than verified billing evidence.
type DeclarationV1 struct{ AdapterID, PlanClass, ExtraUsage, IdentityRef, DeclaredAt string }

// AuthV1 records nonsecret native authentication observations.
type AuthV1 struct {
	LoggedIn                                                               bool
	AuthMethod, APIProvider, SubscriptionType, IdentityRef, ConfigIdentity string
}

// TaskV1 records owned task artefact identity and bounded size.
type TaskV1 struct {
	SHA256, Title, ArtifactRef string
	Bytes                      int64
}

// SnapshotV1 records the pinned source repository observation.
type SnapshotV1 struct {
	SourceRepo, Branch, BaseRev string
	Dirty                       bool
	Source                      string
}

// ProfileV1 records the selected profile and boundary evidence.
type ProfileV1 struct {
	Name                string
	Contained           bool
	Consent, Disclosure string
	Boundary            *BoundaryV1
}

// BoundaryV1 records versioned boundary claims.
type BoundaryV1 struct {
	Profile, OS, Route, Boundary, Version, Owner string
	Coverage                                     CoverageV1
}

// CoverageV1 records the four containment dimensions.
type CoverageV1 struct{ Filesystem, Process, Network, Credential ClaimV1 }

// ClaimV1 records one observed enforcement claim.
type ClaimV1 struct {
	Name, Version string
	Enforced      bool
	Detail        string
}

// DescriptorV1 records the registered adapter identity.
type DescriptorV1 struct{ ID, Version, Harness, Surface string }

// ProbeV1 records resolved native executable evidence.
type ProbeV1 struct{ Executable, Version, SHA256, OS, Arch, Compatibility string }

// ProposalV1 records the nonsecret immutable launch recipe.
type ProposalV1 struct {
	Path         string
	Args         []string
	Dir          string
	StopLadder   []StopV1
	Overrides    []OverrideV1
	Billing      BillingV1
	Manifest     []DigestSourceV1
	Capabilities CapabilityV1
}

// StopV1 records one ordered stop signal and exact grace duration.
type StopV1 struct {
	Signal     string
	GraceNanos int64
}

// OverrideV1 records a policy-permitted nonsecret configuration delta.
type OverrideV1 struct{ Kind, Name, Value, Reason string }

// DigestSourceV1 records one sorted unique manifest source.
type DigestSourceV1 struct{ Source, Digest string }

// BillingV1 records billing evidence with declarations kept distinct.
type BillingV1 struct {
	Mode, CredentialProvenance, EntitlementClass, EntitlementSource string
	PaidContinuation, PaidContinuationUserDeclaration               string
	Qualified                                                       bool
	G05                                                             string
}

// CapabilityV1 records the closed versioned capability inventory.
type CapabilityV1 struct {
	SchemaVersion                                                     int64
	AdapterID, AdapterVersion, RuntimeVersion, Mode, ExecutionSurface string
	HarnessID, FidelityQualification                                  string
	Billing                                                           BillingCapabilityV1
	HostIntegration                                                   HostIntegrationV1
	Capabilities                                                      OperationsV1
	Sandbox                                                           SandboxV1
	Platform                                                          PlatformV1
	Qualification                                                     string
}

// BillingCapabilityV1 records billing capability evidence and its nullable reference.
type BillingCapabilityV1 struct {
	Entitlement                                  string
	IncludedOnlySupported, PaidOveragePrevention TriV1
	EvidenceID                                   *string
}

// HostIntegrationV1 records explicit host capability observations.
type HostIntegrationV1 struct{ HerdrEmbedded, HerdrStateBridge, NativeInteractiveAttach TriV1 }

// OperationsV1 records operation capabilities with nullable optional observations.
type OperationsV1 struct {
	StructuredEvents, Resume, LiveSteer, ApprovalBridge, UsageTokens TriV1
	QuotaRemaining, HardMonetaryLimit, NativeSubagents               TriV1
	Reconnect, ModelMetadata                                         *TriV1
}

// SandboxV1 records containment capability evidence.
type SandboxV1 struct {
	Status TriV1
	Scope  string
}

// PlatformV1 records platform ownership capabilities.
type PlatformV1 struct {
	OS                                     string
	WorkerDetachment, ProcessTreeOwnership TriV1
}

// EnvironmentV1 records names and references without environment values.
type EnvironmentV1 struct {
	PolicyRevision, ContextEpoch, PrincipalRef, NativeConfigIdentity string
	RequiredNames, Destinations                                      []string
}
