# 0022. Durable service admission binding and bounded observer delivery

- Status: proposed; no architecture or maintainer acceptance recorded
- Date: 2026-10-31
- Inspected main revision: `a5a993739812acb022d502c1788077fc14d323cb`
- Inspected Task 4 candidate: `3b3c5f5e80a3e0d87a944b43a0fde0881177e36a` (PR #527; unmerged, not accepted)

## Context and approval boundary

Task 22 of `supervisor-attachment-control` requires this decision before service admission and dispatch resume. ADR 0021 establishes one production owner, not a serializable admission proof. The inspected `RunOwned` accepts a trusted internal `admission.Decision`; its config/path consistency checks do not seal that mutable decision or prove prior admission. This ADR proposes an additional service boundary, not a claim that Task 4 has solved its outstanding review findings.

This proposal changes no runtime or qualification. Task 4 must independently merge. Tasks 6–9 remain blocked until an exact ADR revision has independent architecture and maintainer acceptance and a separately reviewed follow-on spec update for Tasks 6 and 7 merges. Task 22 is not complete until that update's exact merged revision is recorded in its handoff. Candidate or relevant source drift requires re-grounding this inventory and review.

Normative anchors: Master Specification v2 §§2/3/6/8, invariants I02/I03/I05/I06/I12/I18/I20/I21/I23; ADRs 0014, 0016 and 0021. The wire input remains the closed schema-2 `control.RunSpec`; schema-9 JSON validity alone is not semantic authority.

## Proposed decision

Use an immutable, versioned non-secret binding plus an opaque, one-use admission capability. Store no serialized `Decision`, function, reader, caller environment or native subscription secret. A durable digest identifies bytes; it does not authorize independently mutable launch inputs. Fresh admission reconstructs and compares every launch-affecting input before the service can mint the opaque capability. Missing or changed evidence requires a new preparation and confirmations, never an optimistic repair.

### Representation and codec

Add `internal/admissionbinding/binding.go` and `codec.go`, depending only on the standard library. Both admission and journal may depend on this leaf package; it imports neither of them, avoiding the existing admission→journal dependency cycle. Define a closed `BindingV1` composed of the sections in the inventory below, with explicit Go DTO structs, strings, booleans, bounded int64 values, ordered slices and explicit nullable pointers. No `any`, raw JSON or untyped extension map is permitted. `ConfigManifest.Digests` becomes a slice of `{source,digest}` sorted by bytewise source; reject duplicates. Capability and containment objects are copied into closed DTOs enumerating their source fields, not embedded live types.

`EncodeV1(BindingV1) ([]byte,error)` validates before encoding. UTF-8 strings must be valid, contain no NUL, and meet the corresponding source/wire limits. Preserve argument, tool and check order; do not sort ordered execution inputs. Preserve unset versus explicit zero/false via pointers for ceilings. Canonical paths are absolute, resolved under the owning OS, and identity-bound; do not lowercase or Unicode-normalize path text. A root's physical identity is checked again, not inferred from text equality.

Canonical bytes are compact JSON in the fixed declared DTO field order, no insignificant whitespace, no omitted fields, UTF-8, no HTML escaping, no trailing newline. Required collections use `[]`, not null; optional objects use explicit null. Timestamps use UTC RFC3339Nano; durations use exact nonnegative integer nanoseconds, with unset represented separately. Derived content digests and binding digests are lowercase 64-hex strings; raw request hash/trust strings follow the per-field rules below. Commit IDs are full resolved object IDs. Reject NaN, floats, integer overflow and unknown schema/enum values. `DecodeV1` bounds input to 1 MiB before allocation, rejects duplicate object keys at every depth and unknown/missing fields, validates all limits, re-encodes and requires byte equality. Ordinary `encoding/json` duplicate-key acceptance is insufficient.

`DigestV1` is SHA-256 of `MYTHHELM:service-admission-binding:v1\n` followed by canonical bytes. Store bytes and digest together and verify both on every read; copying a digest onto different values must fail. A codec version change requires a new schema and domain separator, not reinterpretation of old bytes. Maximum task input remains `admission.MaxTaskBytes`; task contents are not a JSON binding field.

### Closed V1 DTO declarations

The following field order and names are normative for canonical JSON. JSON object keys are the exact case-sensitive Go field identifiers shown here, with no tags or `omitempty`; these private binding bytes are separate from the existing snake-case control wire. All DTO types live in the leaf package, as independent declarations rather than aliases/embeddings of mutable runtime types. `TriV1` is a validated string with the exact optional/unknown/unsupported/supported distinctions in the source contract. Fields listed together retain left-to-right declaration order.

```go
type TriV1 string
type BindingV1 struct {
    SchemaVersion int64 // exactly 1
    PreparationID, PeerID, RootID string
    Generation int64
    ContextEpoch, PolicyRevision, ExpiresAt string
    RunID, TaskID, AttemptID string
    Request RunSpecV2
    Decision DecisionV1
    Challenges []ChallengeV1
}
type ChallengeV1 struct { ID, Kind, Text string } // digest is outside to avoid a hash cycle
type RunSpecV2 struct {
    SchemaVersion int64 // exactly 2
    Repository, TaskFile, TaskSHA256, Adapter, Billing, ExecutionProfile string
    Revision, Host, Scenario string
    UseCommitted, NoChecks, KeepGoing bool
    TrustProjectConfig, TrustNativeConfig, DeclareEntitlement string
    StripCredentialEnv, AllowUntestedNativeVersion bool
    Ceilings OptionalCeilingsV1
}
type OptionalCeilingsV1 struct {
    ExecutionNanos, Repairs, Replans, TransportRetries *int64
}
type FiniteCeilingsV1 struct { ExecutionNanos, Repairs, Replans, TransportRetries int64 }
type DecisionV1 struct {
    StateRootID, RunID, TaskID, AttemptID, RunDir, Workdir, Host, Scenario string
    GitName, GitEmail string
    ProjectConfig ProjectConfigV1
    ConfigDigest, RepoIdentity string
    RecordTrust, NoChecks, KeepGoing bool
    EnvelopeFlags *OptionalCeilingsV1
    ResolvedCeilings FiniteCeilingsV1
    Declaration *DeclarationV1
    RecordNativeTrust bool
    NativeConfigDigest string
    NativeHooks *int64
    NativeTrustGrant string
    NativeAuth *AuthV1
    Task TaskV1
    Snapshot SnapshotV1
    Profile ProfileV1
    Adapter DescriptorV1
    Probe ProbeV1
    Proposal ProposalV1
    Environment EnvironmentV1
}
type ProjectConfigV1 struct {
    SchemaVersion int64
    Passthrough, AllowedTools []string
    Checks []CheckV1
    Envelopes ConfigEnvelopesV1
}
type CheckV1 struct { Name string; Argv []string; FailOnOutput bool; Timeout string }
type ConfigEnvelopesV1 struct { Execution string; Repairs, Replans, TransportRetries *int64 }
type DeclarationV1 struct { AdapterID, PlanClass, ExtraUsage, IdentityRef, DeclaredAt string }
type AuthV1 struct { LoggedIn bool; AuthMethod, APIProvider, SubscriptionType, IdentityRef, ConfigIdentity string }
type TaskV1 struct { SHA256, Title, ArtifactRef string; Bytes int64 }
type SnapshotV1 struct { SourceRepo, Branch, BaseRev string; Dirty bool; Source string }
type ProfileV1 struct { Name string; Contained bool; Consent, Disclosure string; Boundary *BoundaryV1 }
type BoundaryV1 struct { Profile, OS, Route, Boundary, Version, Owner string; Coverage CoverageV1 }
type CoverageV1 struct { Filesystem, Process, Network, Credential ClaimV1 }
type ClaimV1 struct { Name, Version string; Enforced bool; Detail string }
type DescriptorV1 struct { ID, Version, Harness, Surface string }
type ProbeV1 struct { Executable, Version, SHA256, OS, Arch, Compatibility string }
type ProposalV1 struct {
    Path string
    Args []string
    Dir string
    StopLadder []StopV1
    Overrides []OverrideV1
    Billing BillingV1
    Manifest []DigestSourceV1
    Capabilities CapabilityV1
}
type StopV1 struct { Signal string; GraceNanos int64 }
type OverrideV1 struct { Kind, Name, Value, Reason string }
type DigestSourceV1 struct { Source, Digest string }
type BillingV1 struct {
    Mode, CredentialProvenance, EntitlementClass, EntitlementSource string
    PaidContinuation, PaidContinuationUserDeclaration string
    Qualified bool
    G05 string
}
type CapabilityV1 struct {
    SchemaVersion int64
    AdapterID, AdapterVersion, RuntimeVersion, Mode, ExecutionSurface string
    HarnessID, FidelityQualification string
    Billing BillingCapabilityV1
    HostIntegration HostIntegrationV1
    Capabilities OperationsV1
    Sandbox SandboxV1
    Platform PlatformV1
    Qualification string
}
type BillingCapabilityV1 struct { Entitlement string; IncludedOnlySupported, PaidOveragePrevention TriV1; EvidenceID *string }
type HostIntegrationV1 struct { HerdrEmbedded, HerdrStateBridge, NativeInteractiveAttach TriV1 }
type OperationsV1 struct {
    StructuredEvents, Resume, LiveSteer, ApprovalBridge, UsageTokens TriV1
    QuotaRemaining, HardMonetaryLimit, NativeSubagents TriV1
    Reconnect, ModelMetadata *TriV1
}
type SandboxV1 struct { Status TriV1; Scope string }
type PlatformV1 struct { OS string; WorkerDetachment, ProcessTreeOwnership TriV1 }
type EnvironmentV1 struct {
    PolicyRevision, ContextEpoch, PrincipalRef, NativeConfigIdentity string
    RequiredNames, Destinations []string
}
```

`StateRootID` replaces serialization of `Decision.StateDir`; the held root resolver supplies its physical directory. `ConfigIdentity` replaces `NativeAuth.ConfigDirectory`, with fresh resolution required. `Request.Ceilings.ExecutionNanos` encodes validated `ExecutionMillis * 1,000,000` exactly (overflow refuses); the wire request's explicit null versus value is preserved. Unresolved `EnvelopeFlags` negative sentinel fields become null, not a negative effective ceiling. `NativeHooks` is null when no current inventory established a measurement; a measured zero is a non-nil zero. Optional reconnect/model-metadata's empty source value becomes null; `unknown` remains an explicit enum string. `Task.ArtifactRef` is a root-relative owned artifact identifier, never an arbitrary filename. `Environment` has names/references only, no value map or aggregate environment hash. RequiredNames/Destinations are sorted unique sets; other arrays keep semantic source order.

In addition to the 1 MiB byte cap: at most 128 challenges, 64 checks, 256 arguments per command, 256 tool/passthrough/manifest entries, 16 stop steps and 4096 UTF-8 bytes per ordinary string; title is at most 256 bytes, IDs/digests use their stricter existing grammar and challenge text at most 4096 bytes. Any route requiring larger values refuses as unsupported rather than truncating. All nested collections count against total bytes. Expiry is 5 minutes from preparation using the service's observed UTC clock, shortened by any underlying authority expiry; confirmation/revalidation requires both that timestamp and the unchanged live context epoch. Clock uncertainty/backward movement invalidates the preparation. These are proposed bounded service limits, not claims about current CLI acceptance.

`ValidateV1` additionally requires cross-field consistency: outer run/task/attempt IDs match `Decision.RunID`/`TaskID`/`AttemptID`, outer `RootID` matches `Decision.StateRootID`, and outer `ContextEpoch`/`PolicyRevision` match `Decision.Environment.ContextEpoch`/`PolicyRevision`; Request selectors/booleans/ceiling inputs agree with their resolved Decision values under the pinned resolver; task artifact hash/length match; adapter/probe/proposal/capability versions and executable match; no-checks requires the complete zero project config; and required challenge IDs/kinds/text match the corresponding authority changes. Hashing an internally inconsistent pair of request and decision must refuse. A challenge's wire `BindingSHA256` is computed after the binding's challenge ID/kind/text list, avoiding a self-referential digest. Confirmations are separate immutable records referencing that digest, not fields that change the digest they approve.

Per-field hash rules: `Request.TaskSHA256` preserves the exact validated request spelling, including the optional `sha256:` prefix and uppercase hex accepted by the current control validator. `Decision.Task.SHA256`, artifact hashes and other derived content hashes use normalized lowercase unprefixed 64-hex; comparison decodes the request hash to bytes, then compares bytes. `Request.TrustProjectConfig`/`TrustNativeConfig` preserve their exact supplied text: empty means no explicit flag; nonempty must satisfy the pinned admission flag grammar and exact expected value `sha256:<lowercase 64 hex>` and cannot be rewritten into a different consent. Validate that grammar through the pinned admission validator, not the looser generic control digest helper. Derived config/native-manifest digests are normalized lowercase unprefixed values. `Decision.ConfigDigest` is exactly empty under NoChecks, and required nonempty otherwise; no-checks ProjectConfig is its complete zero value. `Decision.NativeConfigDigest` and NativeTrustGrant may be empty only when the admitted adapter path did not require/inventory/grant that evidence; do not manufacture a zero digest or grant. Optional evidence references remain explicit null/empty according to their source type. Tests must cover prefixed/unprefixed and uppercase task hashes, exact trust-flag preservation/rejection, no-checks empty config, absent native evidence and mismatched normalized hash bytes.

Retention is a separate fail-closed admission policy, not an assumption that strings are non-secret. Git identity/email, task title/artifact reference, challenge text, repository/config paths, check argv, allowed tools, manifest labels and override strings must each pass the versioned retention policy before P storage. The policy explicitly classifies which exact fields/values may be retained and under what operator-approved local scope; unknown classification, secret-bearing content, or absent approval rejects the preparation. Never redact or hash a secret executable input and pretend the altered value is the same launch. Display-only title redaction is deterministic and policy-version-bound. Raw native subscription secrets remain forbidden regardless of a generic retention approval. The follow-on spec must provide executable policy fixtures including email/path/argv/challenge sentinels; this ADR does not certify a classifier or grant retention authority.

### Field inventory

Disposition codes: **P** persists a validated non-secret value in the immutable binding; **R** re-derives from pinned inputs and revalidates before launch; **X** is rejected at this boundary. P alone never replaces the stated revalidation. The following is the closed source inventory; adding a source struct field must fail a reflection-based completeness test until assigned a disposition.

| `admission.Decision` field | Disposition and rule |
|---|---|
| `StateDir` | R from held adopted-root lease; P root identity reference. Must match the journal's physical root. |
| `RunID`, `TaskID`, `AttemptID` | P fresh canonical IDs allocated once for the preparation; never caller chosen or reallocated after commit. |
| `RunDir`, `Workdir` | R separately: `RunDir=<root>/runs/<run_id>` and `Workdir=<RunDir>/workspace`; P expected logical paths; reject escapes, symlink substitution and another run's directory. |
| `Host` | P `standalone` only; unsupported hosts X. |
| `Scenario` | P validated fake scenario; non-fake scenario X. |
| `GitName`, `GitEmail` | P policy-permitted identity captured from service-owned repository context, not caller environment; exact immutable values used for snapshot/candidate work. If identity cannot be retained under policy, X. |
| `ProjectConfig` | R by strict parse of immutable committed `mythhelm.toml`; P closed parsed schema and blob digest. Inventory below applies even when a value does not currently reach launch. |
| `ConfigDigest` | P exact committed config-byte digest; re-read blob at pinned commit and compare parsed structure and digest. |
| `RepoIdentity` | P canonical repository identity and its physical identity evidence; R from repository, never merely path text. |
| `RecordTrust` | P explicit project-grant intent bound to exact repository/config/challenge; no grant before its specified transaction. |
| `NoChecks` | P explicit boolean. True requires empty `ProjectConfig` and empty `ConfigDigest`, and a matching explicit no-checks challenge; do not silently load or execute project checks. |
| `KeepGoing` | P exact boolean and any required operator confirmation; a retry cannot widen it. |
| `EnvelopeFlags` | P nullable execution/repair/replan/transport-retry inputs including unset sentinels; P resolved finite ceilings and resolution-policy revision. R against config/defaults and remaining ledger budget. |
| `Declaration` | P nullable adapter/plan/extra-usage/hashed-identity/declared-at assertion, clearly user-declared. R current supported non-secret auth identity and declaration validity. An old assertion cannot attest current billing facts. |
| `RecordNativeTrust` | P native trust-grant intent with exact digest/challenge; no pre-confirmation write. |
| `NativeConfigDigest` | P exact inventoried config digest; R inventory under the service execution identity immediately before dispatch. |
| `NativeHooks` | P measured hook count with provenance; R inventory; unknown is not zero. |
| `NativeTrustGrant` | P exact current grant reference or explicit absence; R authority, identity, revocation and digest. |
| `NativeAuth` | P nullable non-secret auth status fields; details below. R through the qualified native status surface under service identity. |
| `Task` | P SHA-256 and sanitized title plus immutable service-owned task artifact reference; R bounded exact content/hash from that artifact, delivered on stdin. Never use mutable original TaskFile as later launch authority. |
| `Snapshot` | P `SourceRepo`, `Branch`, `BaseRev`, `Dirty`, `Source`; R physical repo identity and full commit. Dirty/source are observations, not permission to include uncommitted bytes. |
| `Profile` | P `Name`, `Contained`, `Consent`, `Disclosure`, nullable `Boundary`; R current boundary probe/qualification. Unsupported or insufficient coverage X; never downgrade to trusted-host. |
| `Adapter` | P `ID`, `Version`, `Harness`, `Surface`; R exact registered adapter and supported native surface. |
| `Probe` | P `Executable`, `Version`, `SHA256`, `OS`, `Arch`, `Compatibility`; R resolved executable identity/hash, supported OS/arch and qualification. |
| `Proposal` | P/R per launch inventory below; raw live `ProcSpec` is not persisted or accepted as proof. |

`ProjectConfig` nested inventory: P/R `SchemaVersion`; `Environment.Passthrough` names in order; `Adapters.ClaudeCode.AllowedTools` in order; each `Checks` entry's `Name`, ordered `Argv`, `FailOnOutput`, `Timeout`; `Envelopes.Execution`, nullable `Repairs`, `Replans`, `TransportRetries`. Reparse the exact committed blob, compare every value, and resolve timeouts/ceilings with pinned policy. No-checks requires the entire parsed value zero/empty, not just a digest. Check argv and passthrough values that cannot meet the non-secret retention policy are X rather than redacted into different executable content.

`NativeAuth` nested inventory: P `LoggedIn`, `AuthMethod`, `APIProvider`, `SubscriptionType`, `IdentityRef`; R `ConfigDirectory` from the service's approved configuration context, with its identity/config digest in the binding. Do not persist raw status output, email, organization ID or native authentication material. Missing identity or unsupported status is a refusal, not sign-in inference. `Declaration` uses all five `DeclarationRow` fields and remains an assertion, never verified paid-continuation prevention.

`Profile.Boundary` inventory: P/R `Profile`, `OS`, `Route`, `Boundary`, `Version`, `Owner` and every `Coverage` fields `Filesystem`, `Process`, `Network`, `Credential`, each with `Name`, `Version`, `Enforced`, `Detail`. Require current matching registry and runtime availability; its old copied claim cannot qualify another host. Containment policy's workdir is derived; auth binds, proxy choices or new endpoints not established by the admitted route are X.

| `control.RunSpec` field | Disposition and binding |
|---|---|
| `SchemaVersion` | P exactly 2; other versions X until a separate decoder is qualified. |
| `Repository`, `Revision`, `UseCommitted` | P request values plus resolved `Snapshot` and repository identity; R commit/repository. Preserve empty requested revision versus resolved full commit. |
| `TaskFile`, `TaskSHA256` | P original path as provenance, digest and owned artifact reference; R original bounded read during preparation, then use owned exact bytes. Original path is not a post-confirmation input. |
| `Adapter`, `Billing`, `ExecutionProfile`, `Host`, `Scenario` | P exact explicit selectors and resolved route/profile/billing fields; unsupported combinations X. |
| `NoChecks`, `KeepGoing` | P unchanged booleans plus binding-specific challenges. |
| `TrustProjectConfig`, `TrustNativeConfig` | P exact requested digest and challenge decision; R actual inventory/blob and current authority. A digest alone is not consent. |
| `DeclareEntitlement` | P parsed validated declaration, or explicit absence; R exact current identity. Never invent a plan or extra-usage setting. |
| `StripCredentialEnv` | P exact boolean and environment-policy revision; R service-owned child environment filtering. It cannot disable native auth to manufacture eligibility. |
| `AllowUntestedNativeVersion` | P exact experimental-version consent plus resolved probe; R unchanged executable/version. No qualification promotion follows. |
| `Ceilings` | P each nullable `ExecutionMillis`, `Repairs`, `Replans`, `TransportRetries`, plus resolved finite ceilings; reject rounding that changes the effective bound. |

### Launch inventory and execution context

`LaunchProposal.Spec.Path`, ordered `Args` and `Dir` are P only when demonstrably non-secret and derived from the pinned adapter/task/config; R by running the qualified prepare logic and comparing exact values. Credentials in argv are X. `Spec.Stdin` is X as a reader; R a new reader over the owned exact task artifact. `Spec.Env` is X as a durable full environment, even hashed wholesale: names or low-entropy values can themselves be sensitive. Persist only the approved environment-policy revision, required variable names, context epoch and non-secret binding references. The service alone constructs the environment from its own qualified execution context; no caller environment overlay crosses the wire. Policy-permitted non-secret values may be retained in a private, owned, immutable RAM capsule for that preparation; secret native subscription material must never be extracted, copied or replayed by MYTHHELM. If existing adapter preparation needs forbidden material, that route is unsupported here.

Revalidation requires an unchanged live service execution-context epoch, the same authenticated OS principal, approved native home/config identity, policy version and destination set. Do not treat a fresh process with the same username as the same environment. A restart invalidates all unconsumed preparations and RAM capsules; it does not reconstruct values from a transcript or old client. Before launch, unknown or non-reconstructable environment/config/auth input produces `dependency_stale` or the specific admission refusal and consumes no new attempt. Already committed uncertain dispatch is reconciled as described below, never relaunched from missing RAM.

Remaining proposal fields are P/R: ordered `StopLadder` signal/grace pairs; every `Overrides` entry's `Kind`, `Name`, `Value`, `Reason` only if non-secret and approved (otherwise X); sorted `Manifest.Digests` source/digest pairs; all `Billing` fields `Mode`, `CredentialProvenance`, `EntitlementClass`, `EntitlementSource`, `PaidContinuation`, `PaidContinuationUserDeclaration`, `Qualified`, `G05`; and the complete versioned `Capabilities` record. The latter includes its schema/adapter/runtime/mode/surface/harness/fidelity/qualification strings; all billing entitlement/included-only/overage/evidence fields; all host integration fields; structured-events/resume/live-steer/approval/usage/quota/hard-limit/subagent/reconnect/model-metadata values; sandbox status/scope; and platform OS/detachment/process-tree ownership. Preserve absent optional capability values separately from `unknown` and `unsupported`. All mandatory capabilities must be supported under current matching qualification.

`workers.Launch` values are constructed once from the sealed launch recipe: token is fresh and only its hash is persisted in journal binding/outbox/claim rows; the existing protected worker handoff and `worker.json` retain the raw coordination token/nonce as described below; task/adapter IDs, executable hash/path/args/dir, prompt path, stop ladder, config digests and config source paths must match the binding. `Containment` is derived from the bound profile/workdir and revalidated, never supplied by a client. `ProxyAllow` is re-derived from the admitted route; the currently empty contained allowlist stays empty (deny all), and an unestablished endpoint is rejected. `UserConfigPaths` is re-derived by the pinned adapter inventory and must have an exact one-to-one match with `UserConfigDigests`; missing/extra sources refuse. This is a proposed stricter producer-mapping contract: existing static source mappings may include undigested paths. The follow-on adapter/worker task must change that mapping and test missing, extra and swapped sources before service dispatch is supported. Runtime proxy address, nonce, execution deadline, stop-ladder version and worker generation are generated/pinned by the existing launch-policy path and committed with launch intent before spawn; they are not mutable service request fields. New worker launch fields fail the completeness test until reviewed.

### Producer, consumer and proof APIs

These are proposed signatures and ownership boundaries, not existing APIs. `internal/service/admission.go` owns authenticated transport orchestration; `internal/admission/service.go` owns reusable fresh resolution; `internal/supervisor/service_admission.go` owns the private sealed decision, plan and permit; `internal/journal/admission_bindings.go` owns typed durable records. `internal/service` imports control/admission/journal/supervisor, not the reverse. The DTO leaf package contains no authority-minting function.

```go
// internal/admissionbinding; all structs have the closed V1 inventory above.
func EncodeV1(BindingV1) ([]byte, error)
func DecodeV1([]byte) (BindingV1, error)
func DigestV1(BindingV1) ([32]byte, error)

// internal/supervisor: opaque values, private fields; copies share one lifetime.
type ServicePlan struct { /* private immutable binding and required challenges */ }
type LaunchPermit struct { /* private sealed deep copy, identity and one-use state */ }
func PrepareService(context.Context, ServiceInputs) (*ServicePlan, error)
func RevalidateService(context.Context, journal.ConfirmedPreparation,
    ServiceInputs) (*LaunchPermit, error)

// internal/journal: returned records have private fields and copy-only getters.
type ConfirmedPreparation struct { /* validated canonical bytes and confirmations */ }
type CommittedAdmission struct { /* committed admission and pending intent identity */ }
type CommittedDispatch struct { /* armed exact attempt/launch identity */ }
func (j *Journal) ReadConfirmedPreparation(context.Context,
    PreparationLookup) (ConfirmedPreparation, error)
func (j *Journal) CommitServiceAdmission(context.Context,
    AdmissionCommit) (CommittedAdmission, error)
func (j *Journal) ArmServiceDispatch(context.Context,
    DispatchArm) (CommittedDispatch, error)

// internal/supervisor; no caller-supplied Decision parameter.
func RunAdmittedOwned(context.Context, *journal.Journal, *ServiceOwner,
    *LaunchPermit, journal.CommittedAdmission, *ObserverSink) (Outcome, error)
```

The concrete input records have this closed field inventory (all identity/digest strings use their existing validated grammar): `ServiceInputs` = `RootID`, `PeerID`, `ContextEpoch`, `PrincipalRef`, `PolicyRevision`, positive `Generation`, exact `RunSpecV2 Request`, `TaskArtifactRef` and an opaque `*ExecutionContext` held by the service. That context contains only private resolver state and a lifecycle lock, exposes no environment getter, and is not serializable. `PreparationLookup` = `PreparationID`, `PeerID`, `RootID`, `BindingSHA256`, `ContextEpoch`, positive `Generation` and `ObservedAt time.Time`. `AdmissionCommit` and `DispatchArm` are opaque journal request types with private fields/copy-only accessors: admission contains the validated confirmed-preparation handle, exact canonical bytes/digest, operation ID and prepared run/task/planned-attempt IDs; arming additionally contains expected pending-intent revision, owner/generation, fresh token/nonce hashes, resolved deadline/ladder/policy references and the validated snapshot-event reference. Neither accepts arbitrary projection callbacks. Their validated constructors reject missing/mismatched identities and cannot substitute an authority decision; the admission permit and the held owner are checked independently by the consumer. `CommittedAdmission`/`CommittedDispatch` expose only matching canonical IDs, generation, revision and digest after a successful transaction; constructing a row-shaped JSON object does not produce either handle.

`ServiceInputs` is a fresh supervisor-owned resolver context supplied through the service composition, not a request environment: adopted root/physical identity, live instance generation, service execution-context epoch, authenticated peer identity, exact closed RunSpec, pinned policy revision and a reference to the owned task artifact. Concrete DTO structs expose only the fields in the inventory; no arbitrary callback, unchecked config or client `Decision` is accepted. `ServicePlan` exposes deep-copy getters for canonical binding bytes/digest and required challenges. Preparing may inventory/probe within already approved authority, but cannot grant trust, create a run/reservation, emit admission or spawn. Unsupported startup effects must block preparation before an unapproved native/config action.

`supervisor.PrepareService` calls staged admission resolution: collect required challenges without treating them as approved. Do not call the existing monolithic `Decide` with an always-true confirmation callback to simulate a user. Task 8 extracts side-effect-free resolution and challenge planning, then final resolution uses the exact durable accepted confirmations. Re-reading any source changes the tentative binding and invalidates confirmations. A mutable `ServicePlan` exposed to the client is not the producer contract.

`PreparationLookup` names preparation ID, authenticated peer ID, held physical root, current generation, expected binding digest and observed time. `ReadConfirmedPreparation` strictly decodes schema-9 `spec`, `challenges` and `confirmations`, verifies the new immutable binding, ready/not-consumed state, current peer/root/generation/context, expiry and complete exact approvals. It must not return an object assembled from independent caller fields. Each challenge ID is fresh and single-binding; approval covers preparation ID, challenge ID, binding digest, peer, generation and expiry. Duplicate conflicting confirmations, unknown challenges, rejection, missing affirmative consent, changed task/config/route and expiry refuse. An old ready bit or supplied digest alone is insufficient.

`supervisor.RevalidateService` calls the pinned admission resolver and reruns supported admission, task/config/native/billing/boundary and policy checks under the same live context. It compares canonical reconstructed bytes with the durable binding and confirms exact trusted decision side effects. It returns `LaunchPermit` only on byte equality. The permit owns deep copies of every slice/map and immutable task bytes; later mutations of request/DTO/config/argv/env sources cannot change it. A permit is bound to root, peer, preparation, generation, context epoch, run/task/attempt IDs, binding digest, expiry and nonce. It cannot be decoded from JSON, minted by a bool, or recovered from a digest. Its synchronized private state permits one transition from validated to consumed; copies share that state.

`AdmissionCommit` is constructed from the permit's copy-only canonical getters and the authenticated start identity; it contains no Decision/env/reader. `CommitServiceAdmission` verifies the preparation again inside the single transaction below, refusing changed confirmations, epoch, root, generation or consumption. `CommittedAdmission` is minted only after the admission commit succeeds, not from a row-shaped caller value. A post-commit read error is ambiguous, so query the same operation identity; do not allocate another attempt.

`RunAdmittedOwned` first validates owner, journal root, generation, committed admission/pending-intent identity and permit identity, then consumes the permit exactly once. The sealed Decision is a private field owned and consumed entirely within package supervisor; there is no exported Decision getter, callback or cross-package private-field access. The service receives only opaque supervisor plan/permit handles and never that copy. Production acceptance uses the pinned evaluator/default timeouts, nil test-only `BindStep`, no unapproved `Extension`, and no caller `Hooks` object; changing acceptance policy or limits requires a newly bound policy. The wrapper revalidates actual executable/config/task/workspace inputs at the worker handoff as the existing launch checks require. It has no path accepting a second mutable Decision or bypass boolean. Wrong root/run/attempt/peer/generation/digest, consumed/expired permit or changed execution-context epoch refuses before worker effects.

Task 4 `RunOwned` remains a trusted in-process primitive with its existing signature and independent review obligations. It is not registered as a transport method, not a rehydration API and not called directly by production service dispatch. Trusted Go code, unsafe memory writes or a same-user administrator are not an adversarial boundary established by this design. Tests pin that every ordinary service run/start route reaches the sealed wrapper and no host/plugin request can select the trusted primitive.

### Additive durable schema and one-use transaction

Schema 9 already supplies `prepared_runs`, immutable `dispatch_outbox` identity, attempts/run revisions and operation replay. It stores a RunSpec and evidence digest, not the complete immutable field inventory; do not reinterpret those columns as an existing proof. The follow-on journal producer adds migration `0010_service_admission_bindings.sql` (verify next migration number again before implementation):

- `service_admission_bindings`: preparation ID primary/foreign key, codec version, immutable canonical bytes, binding digest, root identity reference, execution-context epoch, policy revision, task artifact digest/reference and explicit expiry. Bounded content with semantic codec validation; unknown versions refuse.
- `service_admission_consumptions`: preparation ID unique/foreign key, operation ID unique, run ID unique, attempt ID unique, consumed generation and committed binding digest. It is an append-only cross-link, not an alternative task/run ledger. The planned attempt ID is unique text with no premature attempts foreign key; arming validates its exact match before adding the attempts/outbox foreign-key chain. Actual state remains canonical journal/run/attempt/operation rows.
- `service_pending_dispatch`: operation ID primary/foreign key, preparation ID unique, run ID unique/foreign key, planned attempt ID unique, binding digest, generation and state (`pending`, `armed`, `unconfirmed`, `settled`). This durable intent owns a planned attempt identity before an attempts row exists; it is not evidence of launch or a new state in the attempt state machine. It is the Start transaction's dispatch intent.
- `service_dispatch_claims`: operation ID primary/foreign key, claim generation, state (`claimed`, `spawn_intent`, `identified`, `unconfirmed`, `settled`) and exact worker-identity/evidence references. Identity fields immutable; state transitions and evidence relations are checked. Store launch token/nonce hashes, never raw values.

Use STRICT tables, foreign keys and normal INSERT with unique constraints; no `OR REPLACE`. Add no-update/no-delete/no-rowid-replacement guards for immutable identities and binding bytes. Evolving claim state has explicit legal transitions, matching evidence and generation checks. Include all new execution-affecting rows in `run_control_revisions` triggers; a request cannot retain an old revision after binding/claim authority changes. Existing schema-9 preparation rows without a binding are unavailable for service start, not upgraded by guessing. Existing legacy records are byte-preserved. Import/adopt tooling must preserve new rows and reject unsupported/newer schemas; no migration occurs during a read-only doctor operation.

Task 9 Start executes under authenticated peer/root/generation/revision validation and one SQLite admission transaction. It rechecks not consumed, ready, unexpired, exact binding/confirmations, no conflicting active run and currently admissible reservation. Allocate run/task/planned-attempt identities once in preparation. The outer operation ID is the authenticated Start idempotency key; it does not let a different operation consume the same preparation. Atomically:

1. Insert the canonical run row and initial events through existing event/projection helpers, including `run.created` and admission state transition. Do **not** insert an attempt or launch-intent event yet: schema 9's existing attempt machine begins at launch intent, after snapshot. A planned attempt ID is not a launched/created attempt row.
2. Append exactly one `admission.decided` carrying the bound admission record and `acceptance.Pin`. In the same transaction apply `EnsurePolicy`, fresh declaration/native trust, resolved finite envelope and quota hold. Do not start execution time or increment launch counters here. Project trust remains conditional on successful snapshot evidence below.
3. Insert the canonical run assignment and a new schema-2 `service.run_assigned` event in the same projection transaction (run/operation/generation/owner identity and planned attempt ID, with no claimed worker identity). This is distinct from `service.adopted`, which reconciles an existing worker. Insert immutable binding-consumption link, operation acknowledgement and `service_pending_dispatch` intent. Set `prepared_runs.consumed` by compare-and-swap from 0 to 1. Verify affected row counts; collision/staleness rolls everything back. The acknowledgement's planned AttemptID identifies this intent, not process existence; clients show execution/attempt state unknown until canonical attempt evidence exists.

This new pending-intent table is necessary because schema 9's `dispatch_outbox.attempt_id` references an existing attempt. Do not insert a fake `launch_intent_recorded` attempt before snapshot, defer a non-deferrable foreign key, or start the execution envelope early to satisfy that constraint. `ArmServiceDispatch` is a **second**, guarded transaction after snapshot: it checks the same pending intent/operation/admission binding and owner/generation, applies `planAttempt` and its launch counter/deadline policy, inserts exactly one attempt and `attempt.launch_intent_recorded`, pins worker admission/nonce/ladder/generation, inserts schema-9 dispatch_outbox and the dispatch claim, and moves pending→armed together. The raw launch token/nonce are carried by the owned live launch capsule and the existing protected worker handoff/`worker.json` identity record. `workers.Identity.LaunchToken` and `Nonce` are currently persisted there for reconciliation; only the journal binding/outbox/claim stores hashes. This proposal does not remove or reinterpret those worker identity fields, and does not claim the raw values are RAM-only. Any change to that persistence requires separately scoped worker/recovery work and tests. These MYTHHELM-generated coordination values are distinct from forbidden native subscription secrets. A failure rolls back arming and spawns nothing. Concurrent arming conflicts; it never increments the reservation or launch count twice.

The run initializers and `appendAdmission` in current `pipeline.run`/`runLocked` cannot execute again after admission commit. Task 9 adds `internal/supervisor/service_admitted.go` and extracts transaction-building helpers from `pipeline.go`/`state.go`: `createRunTx`, `appendAdmissionTx` and `recordLaunchIntentTx` are proposed internal helpers shared by the legacy path and service transactions. `runAdmittedLocked` verifies exact committed rows, restores pinned ceilings/producer state and enters the post-admission snapshot path. It never calls the initial-insertion portion of `runLocked` or ignores a duplicate CreateRun error.

Snapshotting stays outside SQLite transactions. Atomically write owned task bytes into the run directory and clone the pinned committed revision into the owned workdir; validate config again. Append `workspace.snapshot_created` and its project-trust side effect together only after success. Crash/failure before this event grants no project trust. Immediately before arming, revalidate all external launch inputs against the sealed binding and unchanged live context. No worker spawns until snapshot evidence, exact launch intent and worker acceptance pin are durable. This preserves the existing order: admission/reservation → snapshot → launch-intent accounting/deadline → spawn. If snapshot fails, the run records failure, the pending intent settles without creating an attempt, and existing budget settlement releases/records the reservation exactly once.

Migration and pipeline extraction are mandatory implementation work, not an `alreadyAdmitted` boolean. The follow-on spec must name both transaction boundaries, paths, legal pending-intent transitions, event order, mutation/fault tests and legacy regression checks before Tasks 8/9 proceed. No released migration or historical event is rewritten.

### Crash, restart and replay trace

| Boundary | Required result |
|---|---|
| Preparation or confirmation fails, crash before admission commit | No run/attempt/reservation/trust/admission/dispatch-intent effects. Expire the old preparation after restart/context change. Prepare anew; do not reuse its confirmations. |
| Admission transaction fails at any insertion, pin, quota hold, trust/declaration or compare-and-swap | Roll back all new transactional effects. Report the specific block/persistence failure, not RunAccepted. A successful snapshot event has not yet happened. |
| Commit succeeds but response is lost | Repeated Start with the same authenticated operation/preparation returns the stored exact acknowledgement after scope/generation validation. No new IDs, reservation, declaration or launch. A different operation consuming the same preparation conflicts. |
| Commit-before-snapshot or commit-before-spawn, same live context | The actor queries the same operation/pending intent or armed claim and binding. Before entry it may consume the original still-live permit once. After entry only that active owner continues its internal snapshot/arming flow; a returned/consumed permit is not reusable. It cannot create another intent. Arming is atomic and unique; no armed claim is blindly spawned again. |
| Service restart before proven spawn | Original RAM capsule/permit is gone; old preparation is not a launch source. Preserve admitted rows and mark dispatch unconfirmed/interrupted via explicit reconciliation evidence. Do not rebuild credential/env material or spawn a replacement. A new preparation is a new independently authorized operation after ownership reconciliation. |
| Spawn intent recorded, process creation/worker acceptance ambiguous | Set `unconfirmed`; reconcile exact launch token/nonce hashes, PID plus process creation time, generation and committed spool. Do not infer no process from a missing file, elapsed lease or closed UI. No second spawn. |
| Worker identity and launch proven, service restarts | Adopt/reconcile the same canonical run/attempt under the new instance generation according to ADR 0021 and Task 4 recovery, preserving original launch generation/deadline. This is watch/ingest/recover, not admission replay or new native launch. Unknown identity remains ownership_unresolved. |
| Worker settles; acknowledgement/result replay | Read immutable operation result and current authorized outcome. Native exit is not acceptance, stop requested is not confirmed, and no replay invokes tools/prompts/external effects. |

Exactly-once process creation across a crash is not claimed. Durable one-use intent plus conservative uncertainty prevents a second automatic launch; unavailable work is an explicit operational cost. Stop/recover/apply remain the existing independently authenticated operations with their own evidence and owner checks.

### Bounded observer delivery and shutdown

Choose journal pull, not synchronous client callbacks. Add `internal/supervisor/observer.go` and `internal/service/observe.go`. `ObserverSink` is declared in package supervisor with private shared state, created by its exported constructor and lifetime-owned by the service; clients cannot supply Go callbacks. Its callback-facing methods have no client/socket/disk work:

```go
// callback inputs are copied only when needed for bounded diagnostic hints.
func NewObserverSink() *ObserverSink // supervisor constructor; fixed policy bounds
func (*ObserverSink) Committed(runID string, sequence int64)
func (*ObserverSink) Hint(NoticeCode)
func (*ObserverSink) RequestClose() // nonblocking, idempotent; not owner.Close
func (s *Service) Observe(ctx context.Context, peer Peer, req control.ObserveRun) (control.RunObservation, error)
```

`Committed` sends a coalescible wakeup to one capacity-1 actor channel using a nonblocking send; it never enqueues the event payload or increments a consumer cursor. The canonical journal remains the only event history. Pull handlers read a consistent authorized snapshot using the authenticated standalone peer/root/generation and requested cursor; attachment clients retain their separate sanitized projection route. A wakeup loss is safe because polls query canonical sequence, not a callback's delivery count. At most one pending Observe per peer/run connection; a new overlapping request is refused rather than spawning another waiter.

The service reserves at most 64 observation slots total and at most 16 per authenticated peer, serviced by at most 8 concurrent read handlers. Exhausted capacity returns a typed `capability_unsupported` with bounded `observer_capacity` reason and no run mutation. Limits are constants in the follow-on spec/tests and are independent of native output volume. Observe retains schema-2 limits: at most 128 events, a 1 MiB encoded response and 1000 ms requested long poll. Clamp a zero requested limit to the defined default 128, not unlimited. Empty-poll clients back off at least 500 ms. Transport read/write deadlines remain the existing qualified limits; blocked OS/database calls are not advertised as forcibly cancellable or a hard real-time response guarantee.

Only successfully consumed contiguous canonical events advance the client's cursor. Return a page ending before the byte limit; `More` reports further available events. One event exceeding the response limit is `protocol_mismatch` with an oversize-event reason and unchanged cursor, never silently skipped or truncated into a valid event. Client cancellation/disconnection releases only the observation slot and pending request; it neither stops native execution nor releases run ownership. Native spool ingestion and durable acknowledgements cannot wait for observation reads, socket writes or a client queue. CLI accessible history preserves its per-invocation 1000-event bound, fetching pages of at most 128 and deriving trailers only from events actually consumed. Optional TUI display refreshes may coalesce; outcome-relevant event processing may not.

Classify `Hooks.Notice` at its producer, not by parsing arbitrary strings. Operational evidence must be durable state/event data before any hint: quota-reservation failure/exhaustion; worker identity/acceptance failure; worker exit before attempt end; spool-ingestion failure; candidate-freeze or verification/binding failure; stop intent/deadline/confirmation; recovery ownership uncertainty. Use typed existing state/reason events where they fully express the fact; otherwise introduce a schema-2 `service.diagnostic` event with a closed notice code and matching run/attempt/generation/evidence references. Raw errors, paths, native output, account identifiers and caller text are not diagnostic payloads. If required diagnostic persistence fails, the actor reports persistence unavailable/unconfirmed and does not pretend the fact was recorded.

Informational reservation wording, a reattached-worker hint and repeated rendering hints are best-effort, non-authoritative `NoticeCode` values. Hold at most 16 such codes in a fixed ring; overflow increments a saturating `hints_dropped` diagnostic counter and replaces the oldest hint. This counter describes display loss only, never lost journal events. No hint contains authority or is the sole evidence of a terminal/stop outcome. New Notice call sites must be assigned to durable evidence or a bounded hint by a source-completeness test. Service actors supply no terminal `Hooks.Detach` or `Hooks.Interrupt`; accepted stop is its separately authenticated operation.

Close ordering is explicit:

1. A callback or transport handler may call `RequestClose`, which atomically marks the view closed and wakes the service loop. It cannot synchronously call `ServiceOwner.Close`, `InstanceLease.Close`, join the actor, or wait for its own callback.
2. The service coordinator stops accepting new observations for that view, cancels that view's wait contexts and releases their slots once handlers return. Run ingestion/ownership continues; closing the last view has no effect on the native.
3. Whole-service shutdown first refuses new mutations/observations, records or reconciles outstanding accepted operations under the existing shutdown policy, cancels the owning pipeline context if shutdown requires it, and joins callback/ingestion execution from outside that execution. Only after owned entry return may the coordinator close `ServiceOwner` and release its lease hold. Unresolved process stop remains unconfirmed; do not fabricate settlement to release an owner.
4. A blocked OS wait, unreconciled worker or unreturned owned entry prevents a claim of clean shutdown. Report incomplete cleanup and retain ownership evidence; a timer is not proof the writer stopped. Hard termination/escalation remains an explicit operator/process policy, not a callback trick or detached goroutine pretending cleanup completed.

This design avoids the inspected Task 4 callback-self-close deadlock by forbidding synchronous reentrant close in the service adapter. It does not assert that Task 4's primitive itself is reentrant or cancellable; its independent findings remain open until fixed or explicitly resolved through that task's review.

### Required follow-on paths and executable acceptance

The separate spec update must resolve precise file ownership and introduce tasks for these additive paths before runtime work:

| Owner | Required paths and acceptance |
|---|---|
| DTO/codec producer | `internal/admissionbinding/binding.go`, `codec.go`, `codec_test.go`: fixed byte/hash goldens, strict duplicate/unknown/missing fields, all size/enum/range errors, exact unset/empty/zero distinctions and nested field-completeness inventory. |
| Admission producer (Task 8) | `internal/admission/service.go`, `service_test.go`, `internal/supervisor/service_admission.go`, `service_admission_test.go`; refactor `admission.go`, `billing.go`, `projectconfig.go` as separately scoped tasks where needed. Preparation cannot silently approve a challenge; fresh revalidation seals exact bytes; mutate every Decision/RunSpec/nested launch field and prove rejection or safe rederivation. Include no-checks, missing config, swapped native/config/task, changed peer/context/epoch/policy/expiry, extra-usage unknown and missing execution coverage. |
| Durable journal producer | `internal/journal/migrations/0010_service_admission_bindings.sql`, `admission_bindings.go`, `admission_bindings_test.go`, migration registration, and revision/import tests. Crash each admission insertion and side effect; assert no partial run/attempt/admission/reservation/trust. Reject raw/SQL replace, rowid collision and changed immutable columns. Legacy schema 9 preparation stays unavailable, not upgraded into authority. |
| Pipeline consumer (Task 9) | `internal/supervisor/service_admitted.go`, its tests, shared helpers in `pipeline.go`, `state.go`, `acceptance.go` and budget/worker-pin code as actually needed. One admitted run/attempt, no duplicate insertion or envelope/reservation/launch counting, no raw-Decision production service path, and no second spawn after lost acknowledgement/ambiguous process start. Existing legacy Run/Recover/Apply tests remain required. |
| Service composition | `internal/service/admission.go`, `dispatch.go`, `runtime.go` and their tests: authenticated ordinary route, exact duplicate-operation replay, concurrent consumption, physical-root/generation/revision/expiry drift, restart before spawn, restart with a verified existing worker, and scope-checked operation query. No attachment/host/plugin capability can prepare/confirm/start. |
| Observer producer (Tasks 6/7 after the separate update) | `internal/supervisor/observer.go`, `internal/service/observe.go`, relevant actor/transport/client tests. Stalled/disconnected client, full hint ring, lost wakeup, max slots, oversized event, mixed historical/new schemas, contiguous cursor paging, source-complete notice classification and callback close/cancel races. Verify ingestion and stop acknowledgement progress while clients never read. |

Counterexamples that must fail: attach a digest to a changed argv/env/config object; set ready without exact confirmations; substitute another service epoch or peer; replace a task at its old path; make no-checks carry checks; persist a native auth secret or recover it from a transcript; restart a dispatch with missing RAM and spawn again; let an observer's close release the owner from inside its callback; drop a stop/verification failure as an informational hint; run the original `runLocked` insertion path after durable service admission. Unit, transaction/fault-injection and packaged integration evidence must name exact source/candidate/check revisions. Documentation, a successful fake demo or a hook alone qualifies no native/platform/billing capability.

## Consequences and remaining review decisions

The service can become unavailable after a crash rather than reconstruct forbidden launch material or risk duplicate execution. That is deliberate conservative behavior with an explicit operational cost, not successful self-hosting. More additive schema, staged admission and a separate supervisor entry are required; this is not a wire-only feature or a signature substitution.

Before acceptance, architecture review must check the concrete DTO declarations/limits and the proposed two-transaction extraction against the inventories. The explicit pending-dispatch table resolves schema 9's attempt foreign-key ordering without starting execution accounting during snapshot; executable implementation must prove that ordering and exact once-only settlement. This is a proposed design, not evidence that the migration/extraction already works.

The concrete package direction is supervisor→admission→journal→admissionbinding, plus supervisor→journal/control and service→supervisor/control. Admission never imports supervisor or service. Its reusable `ResolveService` returns a fresh Decision to trusted supervisor code; only supervisor seals, deep-copies, revalidates and consumes that Decision inside its own package. A trusted Go caller could call internal resolver APIs, so this is not a hostile-Go-code boundary; production transport registration and route tests establish that ordinary clients cannot bypass the sealed path. The proposed opaque constructors still need executable cycle/wiring review: ordinary production dispatch must consume the sealed proof, while trusted internal primitives stay unavailable to transport registration. No accepted revision, maintainer decision, merged follow-on spec, runtime qualification or completed Task 22 is recorded by this document.
