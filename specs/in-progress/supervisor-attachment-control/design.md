# Supervisor attachment control — Design

## 1. Current state and scope (v2 §§3–6)

Grounded at `61d18e2`. The original requirements are HOLDS; the claim that the production service already executes them is PARTIAL. `internal/control/client.go:168` (`rg -n 'func RunSupervisor' internal/control/client.go`) acquires the instance and directly serves `NewSupervisorServer`. Its complete registry at `internal/control/server.go:178` (`rg -n 'func NewSupervisorServer' internal/control/server.go`) contains status/assign/reserve/release/heartbeat/read, deliberately not stop/recover. `rg -n 'RegisterStop|RegisterRecover' internal` shows definitions and tests; the opened production registry confirms absence. `internal/cli/run.go:218` and `demo.go:111` (`rg -n 'admission.Decide|supervisor.Run' internal/cli/{run,demo,tui}.go`) admit and start CLI-owned pipelines, including live TUI/accessible callbacks.

`internal/supervisor/pipeline.go:167` holds the run owner lock and `:984` watches/ingests; `recover.go:43` (`rg -n 'func RecoverWithHooks' internal/supervisor/recover.go`) fences migration, acquires run ownership, reconciles, watches and freezes/concludes/repairs. It is not a read-only attachment helper. `internal/supervisor/ingest.go:64` requires ownership, and journal.Append commits event/projection/offset together. `internal/control/stop.go:95` owns its independent lock/state path; `execute_long.go:32` waits for eventual result. Registering either unchanged beside the pipeline is unsafe.

`internal/migrate/apply.go:81` and `drain.go:126` (`rg -n 'func (Apply|drainLocked|adoptable)' internal/migrate/{apply,drain}.go`) migrate a complete root under instance/state/all-run locks. Adoptable live workers retain launch identity after legacy owner release. `internal/supervisor/pipeline.go:204` (`rg -n 'func checkMigrationClear' internal/supervisor/pipeline.go`) refuses legacy work after previewed. `internal/control/ledger.go:104` checks only operations revisions. `internal/control/filter.go:42` permits token-less caller-selected repositories. Neither supplies bridge authorization/revision.

`internal/workers/envelope.go:62` (`rg -n 'func CheckEnvelope' internal/workers/envelope.go`) rejects changed nonzero generation, while zero is legacy unfenced. `internal/workers/worker.go:326` (`rg -n 'func RequestStop' internal/workers/worker.go`) creates one first-wins stop.request per attempt, not one per operation. `internal/journal/journal.go:37` declares schema version 8; additive migration 0009 is currently free and must be revalidated before implementation.

This child completes ADR 0014's ordinary production lifecycle and scoped core contract. Its original published Task1–22 IDs and the mandatory producer Tasks23–31 stay in the approved three-stream epic because a partially rewired owner/dispatch/ingest/control path cannot be advertised as an attach-ready service; each task remains independently reviewable. No per-run recover-enrollment mode, second service or recovered-only FR-1 claim is introduced.

### Current admission-binding amendment baseline

This amendment is grounded at main `1fdcc43a0b0cbe1d9946826f0e6d16ba9a82a7f3`, plus inspected unmerged Task4 candidate `3b3c5f5e80a3e0d87a944b43a0fde0881177e36a`. Historical grounding above describes its original revision, not today's schema. `rg -n 'SchemaVersion|loadMigrations' internal/journal/journal.go` and `rg -n 'CREATE TABLE prepared_runs|CREATE TABLE dispatch_outbox|attempt_id TEXT' internal/journal/migrations/0009_attachments.sql` establish schema9 and the existing attempts foreign key. `rg -n 'func .*run|CreateRun|appendAdmission|snapshot|recordLaunchIntent|planAttempt' internal/supervisor/{pipeline,state}.go` establishes admission → snapshot → launch-intent accounting. `git show 3b3c5f5e80a3e0d87a944b43a0fde0881177e36a:internal/supervisor/service_owner.go | rg -n 'func RunOwned|return runLocked'` establishes the candidate's trusted mutable-Decision seam; it is not merged proof.

The exact reviewed/adopted ADR design is [ADR0022 candidate d8975b3](https://github.com/turbokast/mythhelm/blob/d8975b35b342bc7f5be34c43bcdbcc39e64ecf52/docs/decisions/0022-service-admission-binding.md). PR541 is still a merge prerequisite, not a file asserted to exist on this baseline. This separately reviewed amendment may merge first; Task22 completion and runtime Tasks6–9 remain held until both merge and the exact merged follow-on revision is recorded in Task22's handoff. The accepted content is not native/platform qualification or acceptance of PR527.

## 2. Composition and ownership (v2 §§3, 5.2, 6)

Add `internal/service/` as the executable composition root above control and supervisor; do not introduce a second daemon. `cli.SupervisorMain` calls `service.Run`; control keeps the transport and instance primitives and never imports supervisor. One lifetime root context belongs to the service, not any client request. Work actors are assignments inside this process. Global mutations use the same ledger transaction discipline; a per-run mutex serializes actor decisions, not native I/O or worker waits.

Proposed concrete interfaces (types defined here are new, not claims about today's code):

```go
// internal/control/instance.go: fields private; only successful acquisition mints it.
func AcquireInstanceLease(stateDir string) (*InstanceLease, error)
func (l *InstanceLease) Generation() int64
func (l *InstanceLease) Root() string
func (l *InstanceLease) Close() error

// internal/service/runtime.go
func Run(ctx context.Context, stateDir string) error

// internal/supervisor/service_owner.go: constructor verifies live instance lease,
// canonical root and adopted phase before acquiring the compatibility run lock.
func AcquireServiceOwner(ctx context.Context, j *journal.Journal, lease *control.InstanceLease, runID string) (*ServiceOwner, error)
func (o *ServiceOwner) Close() error
func RunOwned(ctx context.Context, j *journal.Journal, owner *ServiceOwner, d admission.Decision, h Hooks) (Outcome, error)
func RecoverOwned(ctx context.Context, j *journal.Journal, owner *ServiceOwner, runID string, h Hooks) (RecoveryOutcome, error)
func ApplyOwned(ctx context.Context, j *journal.Journal, owner *ServiceOwner, runID, branch string, acceptFlags, acceptUnverified bool) (Receipt, error)
```

Implementation staging is explicit: Task6 produces the singleton composition shell and its production entrypoint, authenticated existing status/root-conflict behavior and service-owned root context. At that stage, new run/attachment methods whose producers have not landed return capability_unsupported with zero mutation; no placeholder handler is advertised as complete composition. Actual admission/dispatch/adoption arrives in Tasks7–9, observation in Task10, public attachment in Task12 and durable stop in Tasks13–15. Full production registry/continued-worker-ingestion evidence is deferred to Task20 after those producers and public clients. Task6 tests do not seed or inject a worker and call that production execution evidence.

Until Task 9 lands, `RunOwned` is a trusted internal ownership primitive, not proof of admission. Task 4 may exercise it with fixtures but must not wire it to a production request. Task 6 keeps run methods unsupported. No production dispatch may rely on a supplied mutable `admission.Decision` merely because its paths or configuration agree with itself. Task 9 must close this boundary before exposing start. ADR0022 and this amendment define the synchronous callback and close contract before Tasks 6 and 7: observer behavior must not deadlock ownership or compromise durable evidence, and shutdown must preserve ownership until active work has exited. The exact delivery, overflow, cancellation and shutdown mechanism and its executable acceptance tests are in §3.2 and Tasks6/7/10/27 below; their reviewed merges and Task22 handoff remain start gates. Task 4 tests do not establish this runtime property.

`ServiceOwner` has private identity/generation/lifetime fields; each entry verifies root/run/held capability. It is not a boolean migration bypass. Existing Run/RecoverWithHooks retain legacy guards during the explicit migration period and call extracted lock-held bodies. Service entries never reacquire their held run lock. Hook adapters issue only nonblocking capacity-1 journal wakeups and bounded typed informational hints; canonical event bodies are pulled, never pushed through a terminal callback. Apply remains explicit and executes existing guarded candidate/ref rules, not a host action. RecoverOwned owns the full continuation of observation/verification/receipt repair, not a parallel `control.Reconcile` task.

On a fresh empty root, initialize the root migration phase through the same locked migration path before opening the writer service. On an existing unadopted root, service status may report migration_required but must not admit/assign/stop/apply against that root; public mutation commands direct the operator to explicit preview/apply migration. Do not automatically migrate existing work on attach. Migration holds the instance lock and excludes an already-running writer; coordinate shutdown of a status-only listener before explicit migration without stopping workers. No service process owns a writable ledger concurrently with migration. Adopted roots refuse every legacy writer. Failed/partially imported migrations remain unavailable until the existing resumable migration finishes, including verification that the imported→adopted shortcut cannot evade the required ownership fence.

Startup enumerates durable assignments and incomplete dispatch/stop operations, acquires their service owner capabilities, reconciles pinned identities and committed spool offsets, then enables that run's controls. Reconcile failures expose ownership_unresolved with sanitized reason codes. Unknown worker identity never becomes a fresh assignment. Terminal runs can be read without running repair as a side effect of attach; startup repair is independently owned service work. No startup or attachment path launches an unapproved replacement. Apply-in-progress uses existing recoverApply reconciliation and never silently creates a different ref.

## 3. Ordinary admission and dispatch (v2 §§3, 6–8)

Use the existing control.Intent envelope and private local transport. New standalone-only methods are `run.prepare`, `run.confirm`, `run.start`, `run.operation`, `run.observe`, `run.recover`, `run.apply`; attachment tokens cannot call them. Fields below encode with lower_snake_case names, reject unknown/duplicate keys, and never contain callbacks, generic maps or serialized admission.Decision. Native payloads remain outside this control contract.

```go
// internal/control/run_contract.go
// nil ceiling fields mean unspecified; numbers are nonnegative and duration positive.
type RunCeilings struct { ExecutionMillis *int64; Repairs, Replans, TransportRetries *int }
type RunSpec struct {
    Repository, TaskFile, TaskSHA256 string
    Adapter, Billing, ExecutionProfile, Revision, Host, Scenario string
    UseCommitted, NoChecks, KeepGoing bool
    TrustProjectConfig, TrustNativeConfig, DeclareEntitlement string
    StripCredentialEnv, AllowUntestedNativeVersion bool
    Ceilings RunCeilings
}
type AdmissionChallenge struct { ID, Kind, Text, BindingSHA256 string }
type PreparedRun struct {
    PreparationID, BindingSHA256 string
    Generation int64
    Challenges []AdmissionChallenge
    Ready bool
}
type RunConfirmation struct { PreparationID, ChallengeID, BindingSHA256 string; Approved bool }
type StartRun struct { PreparationID, BindingSHA256 string }
type RunAccepted struct { OperationID, RunID, AttemptID string; Generation int64; Revision uint64 }
type RecoverRun struct { RunID string; ExpectedRevision uint64; Generation int64 }
type ApplyRun struct {
    RunID, CandidateSHA256, Branch string
    AcceptFlags, AcceptUnverified bool
    ExpectedRevision uint64
    Generation int64
}
```

`RunSpec` deliberately covers the current CLI request's serializable flags; `Repository` and `TaskFile` are canonical absolute local paths accepted only over same-user standalone control. The service reopens and hashes the task, independently resolves repository/configuration/native evidence and applies all current admission qualification. `Host` remains standalone for launch; embedding an existing run never upgrades a route or enables Herdr launch admission. A missing task digest or changed task/config/repository/native grant invalidates preparation before launch.

Preparation state is service-owned and generation-bound. A challenge has a stable kind and evidence binding; answering an arbitrary question string is not authority. Confirmations bind authenticated peer, preparation, challenge and binding digest, are single-purpose and durable until consumed/expired. Noninteractive callers block with the existing actionable flag guidance. The trusted CLI renders challenges; host/attachment capabilities cannot prepare or confirm. Refactor the existing Decide confirmation seam into deterministic prepared decisions plus scoped answers; do not block an actor awaiting terminal input. Revalidate the entire binding before committing start; changed evidence requires new preparation. Existing admission refusal categories and pinned harness/billing/trust rules remain unchanged.

Environment is built service-side through the existing qualified adapter policy, from the service execution identity and native store. Caller environment is not serialized or copied to a durable preparation. Only command flags above express policy; a changed required native environment must cause explicit re-admission/restart guidance rather than borrowing another client's credentials. Tests cover PATH/native-configuration changes, credential-route sentinels and existing route qualification fixtures. Task content and native secrets never enter projected metadata or operations' public result.

Task 8 binds the complete execution-relevant admission inputs and evidence in the service-owned preparation: source repository and revision, task bytes and digest, checks and no-checks, project and native configuration/trust, route and profile, billing and envelopes, confirmations and launch-relevant values. Revalidation covers the entire binding and peer, generation and expiry. Secret values and a caller environment do not enter durable state; later execution reconstructs only policy-permitted service-owned inputs and leaves native authentication to the native surface; MYTHHELM never extracts/copies/replays native subscription material and refuses missing/non-reconstructable required evidence. A digest supplied beside an independently mutable decision is not authority.

Start validates preparation in one transaction, records the immutable admitted bundle, assignment, operation acknowledgment and dispatch intent. Task 9 must execute only a decision reconstructed from, and verified against, that immutable committed binding; missing, stale, altered or mismatched data refuses before worker spawn. Integrate the current pipeline's initial run/attempt insertion with the precommitted records so a duplicate insert, retry or recovery cannot open an alternate admission path. Where schema 9's stored binding and digest cannot reconstruct or verify a required field, add a reviewed additive migration, never modify a released migration. The exact representation, Task8 producer-to-Task9 consumer contract and service-to-supervisor opaque proof APIs are ADR0022 and §3.2 below, implemented by producer Tasks23–31 before their consumers; until then the production start remains unsupported. Worker spawn remains the existing durable one-use launch protocol. A dispatcher crash before/after launch reconciles identity/spool evidence before deciding whether the same committed unconsumed launch may proceed; ambiguous launch is quarantined, never blindly replayed. New replacement/continuation launches require a new explicit admitted operation. The production service calls supervisor.RunAdmittedOwned with its own lifetime context and opaque supervisor permit; RunOwned remains a trusted internal primitive, not a transport dispatch path. Client death after acceptance does not cancel it.

Public run/plain/JSONL/accessible/TUI and demo are clients of these methods and the exact operation/observation contract below and observe service-owned outcomes. Preserve established linear bytes/exit semantics where applicable; any new accepted-operation status is explicit and tested. Demo temporary data survives a live detached worker. Trusted CLI stop/recover/apply use the same run actor. No hidden local fallback after a service error. Legacy paths remain usable only before migration with explicit compatibility diagnostics, never while service owns the root.

### 3.1 Standalone operation results and observation wire

Task 2 defines these exact producer types and codecs before handlers or clients. All new standalone request/result bodies carry `schema_version=2` and explicit snake_case JSON tags; omitted/null union members are different from known empty values. Existing control.Intent/Result transport envelopes remain the existing versioned contract. This standalone same-user channel is deliberately separate from attachment authorization; attachment grants and host callbacks cannot call any `run.*` method. No caller-selected repository widens authority.

```go
type ActionAccepted struct {
    SchemaVersion int
    OperationID, RunID, Method string
    Generation int64
    Revision uint64
}
type RunOutcome struct {
    RunID, AttemptID, State, Reason, AttemptState, AttemptReason string
    Settled bool
}
type ApplyOutcome struct {
    RunID, Branch, CandidateCommit, State string
    Applied bool
}
type OperationQuery struct { SchemaVersion int; RunID, OperationID string }
type OperationStatus struct {
    SchemaVersion int
    OperationID, RunID, Method, State string
    Start *RunAccepted
    Action *ActionAccepted
    Outcome *RunOutcome
    Apply *ApplyOutcome
    Stop *StopResult
    Failure *OperationFailure
}
type OperationFailure struct { Code, Reason, NextAction string }
type ObserveRun struct {
    SchemaVersion int
    RunID string
    Generation int64
    AfterSequence int64
    Limit, WaitMillis int
}
type RunObservation struct {
    SchemaVersion int
    RunID string
    Generation int64
    RunRevision uint64
    Outcome RunOutcome
    Events []journal.Event
    NextSequence int64
    More bool
}
type RunClient interface {
    Prepare(ctx context.Context, spec RunSpec) (PreparedRun, error)
    Confirm(ctx context.Context, confirmation RunConfirmation) (PreparedRun, error)
    Start(ctx context.Context, operationID string, expectedRevision uint64, request StartRun) (RunAccepted, error)
    Recover(ctx context.Context, operationID string, request RecoverRun) (ActionAccepted, error)
    Apply(ctx context.Context, operationID string, request ApplyRun) (ActionAccepted, error)
    Operation(ctx context.Context, query OperationQuery) (OperationStatus, error)
    Observe(ctx context.Context, request ObserveRun) (RunObservation, error)
}
func NewRunClient(conn Conn) RunClient // existing constructor remains; no duplicate definition
// Go-only metadata, never a new schema2 JSON body.
type PreparedReply struct { Run PreparedRun; Revision uint64 }
type PreparedRunClient interface {
    RunClient
    PrepareRevisioned(ctx context.Context, operationID string, spec RunSpec) (PreparedReply, error)
    ConfirmRevisioned(ctx context.Context, operationID string, expectedPreparationRevision uint64,
        confirmation RunConfirmation) (PreparedReply, error)
}
func NewPreparedRunClient(conn Conn) PreparedRunClient
// Private fields/copy getter; client-created only after authenticated reply validation.
type PreparationConflictError struct { current PreparedReply; cause *Error; hasCurrent bool }
func (e *PreparationConflictError) Error() string
func (e *PreparationConflictError) Unwrap() error
func (e *PreparationConflictError) Current() (PreparedReply, bool)
```

Add `SchemaVersion int` to RunSpec, RunConfirmation, StartRun, PreparedRun, RunAccepted, RecoverRun and ApplyRun; all require2. Generation for Start/Confirm/Prepare is the current root service generation in the outer control envelope (Prepare first resolves authenticated status); Prepare returns it and subsequent requests must match. Revisioned preparation/confirmation callers allocate one explicit operation ID per logical intent and retain it with exact arguments for retries; mutation preconditions remain in control.Intent. ConfirmRevisioned and Start expected_revision address the actual server-reported preparation revision, initially0, and is consumed exactly once at Start with operation acknowledgment; RunAccepted.Revision is the resulting run revision. RecoverRun/ApplyRun already carry exact run revision/generation; a request cannot substitute a projection revision.

The existing outer control.Result.Revision is the sole preparation-revision source; PreparedRun has no Revision and schema2 body fields remain unchanged. Task31 modifies the actual typedConnection.invoke in internal/control/client.go to add private invokeResult(ctx context.Context, method,operationID,object string, rev uint64,gen *int64,token string,params any,limit int) (Result,error) after frame/operation/nonnegative-revision validation. For a wire-valid reply invokeResult returns the full Result even when Result.Error is non-nil; transport/frame/operation/revision failures return no usable envelope. Existing invoke delegates and retains its prior body/error behavior for other methods; revisioned calls inspect the verified outer envelope before decoding PreparedRun. Revision must be explicitly present (including numeric0), not null or inferred from an absent int64 field. invokeResult uses a private closed envelope decode with a *int64 revision-presence field, rejects duplicate/unknown keys, then converts to existing Result only after presence/range/operation checks. Transport/protocol failures return no usable revision. NewPreparedRunClient uses the same existing runClient implementation, not a shadow constructor/type. Legacy RunClient.Prepare/Confirm fail closed with capability_unsupported/revisioned_preparation_required and zero request/state; their old signatures remain for source compatibility. Existing RunClient.Start already takes an explicit revision and remains guarded server-side. All new production consumers in Tasks16–19 use PreparedRunClient and the revisioned methods; they never estimate a counter or use a hardcoded0 after a reply.

A successful PrepareRevisioned returns revision0 from its volatile server acknowledgement. Retention confirmation returns the verified stored outer revision1; each later changed confirmation returns the actual committed revision, and a consistent duplicate may return the unchanged current revision. Consumers pass only the latest validated server revision with the same preparation/binding/generation. They retain original operation ID, expected revision and exact request across transport uncertainty; same-op replay authenticates scope, verifies the immutable argument digest and returns the exact original acknowledgement/revision before stale-precondition rejection. A replayed acknowledgement is historical evidence, not a claim of current freshness; the next mutation may legitimately conflict after another client advanced state.

For a new stale mutation to an otherwise authenticated same-generation, same-binding unconsumed preparation, the service returns revision_conflict with its coherent current PreparedRun in Result.Body and current preparation revision in Result.Revision, with zero mutation. The revisioned client validates body identity/binding/generation and outer revision before constructing PreparationConflictError; Current returns a deep copy. A consumer may use that server-authoritative reply to reconcile an unchanged intended answer and issue a fresh logical operation, never increment a counter. Foreign/unknown scope, changed binding/generation, consumed preparation, operation-ID argument conflict or missing current evidence releases no current body/revision hint; it returns the ordinary typed error. Service restart invalidates volatile plans/old context epochs; reconnect reads current service generation and refuses old preparation with dependency_stale, never guesses a revision. Accepted-run operation query/replay retains its separate fresh-authority contract.

Task31 updates actual control/client.go, client_test.go, run_contract.go, run_contract_test.go and existing attachment_contract_test.go helper/call sites. Test framed outer replies through the real exported client, including retention plus multiple confirmations then Start, lost-ack same-op replay, concurrent stale callers, historical ack followed by authoritative conflict refresh, conflicting reuse, negative/missing metadata and reconnect. Preserve Task2 completion as historical; this amendment adds Go metadata/API behavior rather than retroactively claiming it existed.

Wire method mapping: run.prepare→PreparedRun; run.confirm→PreparedRun after revalidation; run.start→immutable RunAccepted; run.recover/run.apply→immutable ActionAccepted; run.operation(OperationQuery)→OperationStatus; run.observe(ObserveRun)→RunObservation. Prepare/Confirm carry typed control.Intent bodies through the dedicated prepared-admission dispatcher described below; Start, Recover and Apply use their checked durable execution paths; Operation and Observe are read handlers and never allocate an operations row or advance a run revision. Mutations require an outer operation ID and immutable digest of exact typed arguments. Failed preconditions return a typed control error and no effect. Accepted recover/apply reconcile asynchronously under the actor; they do not hold a request open until terminal. Client death never cancels accepted work.

OperationStatus.State is accepted, running, completed, failed or unconfirmed. Exactly one immutable acknowledgment arm matches Method: Start for run.start, Action for run.recover/run.apply, or Stop.Ack for an accepted stop. Outcome is populated for run.start/recover once observed, Apply for run.apply once reconciled, Stop for stop; incompatible arms are invalid_contract. Failure is populated only for a known failed/unconfirmed operation with a bounded canonical reason code. Running/accepted is not completion; unconfirmed is not failure or successful stop. RunOutcome.State/AttemptState use the existing supervisor state vocabulary without adding synthetic terminal states; Reason/AttemptReason retain bounded documented reason codes. CLI derives existing runExit semantics from this typed outcome; no daemon imports CLI renderer code. ApplyOutcome mirrors the existing guarded apply's exact branch and candidate commit. On replay/query, validate same-user/root/run authority before revealing the acknowledgment/result. Query of a prior-generation accepted operation is allowed under fresh authenticated current authority; it never resubmits or mutates that operation. Unknown/foreign operation returns permission_denied without cross-run existence disclosure.

Observation is bounded pull/long-poll, not a server-owned persistent subscription. Historical Task2 planning used default64/capability_unsupported for oversize; this reviewed ADR0022 amendment intentionally changes the future producer/consumer behavior to default128/protocol_mismatch. Task2 completion evidence is preserved as historical, not rewritten as evidence that this new runtime behavior exists. A client subscribes by repeating run.observe with its returned NextSequence. Limit is0..128; zero means default128; WaitMillis is0..1000; AfterSequence is a nonnegative committed per-run journal sequence (0 is beginning). The handler first authenticates root/current generation/run, then reads outcome, revision and the event page from one SQL snapshot. It releases the snapshot before an optional wait, and takes a fresh authenticated snapshot on wakeup. It never waits while holding a transaction or actor lock. One response is at most1MiB on this standalone method using the existing core frame ceiling; attachment/host traffic retains the stricter64KiB ceiling. Return only the bounded prefix fitting that response, with NextSequence equal to the last returned event; if none fits because an individual supported historical event exceeds the frame limit, return protocol_mismatch with the safe reason event_too_large and do not advance the cursor. More means committed events remain after that cursor. Empty successful page retains the input cursor; terminal outcome is separate and may be returned with an empty page.

Events retain their original journal.Event fields/schema_version/payload and only the named run's committed sequence. The decoder selects the existing v1 event contract or v2contract.Envelope according to schema_version; unknown critical/newer versions fail capability_unsupported, malformed/gapped events fail invalid_contract, with no cursor advance. These ordinary operator events are not the host projection: clients apply the existing supervisor.IsStreamEvent output allowlist and current renderer sanitization; internal service.adopted is consumed for continuity but is not emitted as an undocumented JSONL stream event. No event is ever forwarded to hosts. A supported internal event may advance the scanned cursor after the caller has processed its control meaning; the accessible next-after trailer is the last actually consumed sequence, never a skipped unread page. Unknown/foreign run is permission_denied or ownership_unresolved per canonical visibility policy; future cursor/generation is revision_conflict; history missing under an otherwise valid cursor is capability_unsupported with history_unavailable, never a silent jump. Database/transport timeout is persistence_unavailable and does not advance a cursor. Retention is unchanged by this spec.

Each Observe/Operation call has an end-to-end2second timeout including a maximum1second long poll; cancel affects only that read. Allow at most one outstanding observation per client/run connection, no unbounded push queue, and back off empty polls to the existing500ms cadence. CLI accessible history retains its1000-event per-invocation bound by paging at most128 at a time; only consumed events determine its trailer. TUI clients maintain a bounded latest-state cache and may coalesce display refreshes but cannot skip event processing required for an outcome. Native stdout/spool ingestion never waits for observation clients. Frame boundaries, snapshots, cursor continuity, mixed historical/new event versions and each typed failure are owned by the observation producer task before the public client tasks.

### 3.2 Exact ADR0022 implementation contract and producer order

The closed DTO declarations/field order, per-field dispositions and source-specific validation at ADR candidate d8975b3 are normative input to Task23, not a runtime import from an unmerged file. Canonical JSON has case-sensitive DTO identifiers, every field present, ordered executable arrays, sorted unique source/name sets, explicit nulls, no whitespace/newline/HTML escaping and SHA-256 over `MYTHHELM:service-admission-binding:v1\n` plus bytes. The strict 1 MiB decoder rejects duplicate/unknown/missing fields and noncanonical re-encoding. Per-field raw TaskSHA256 spelling may retain accepted prefix/uppercase; derived hashes are lowercase/unprefixed; native/project trust flags preserve exact admitted `sha256:<lowercase hex>` or empty, and NoChecks requires empty ConfigDigest plus complete zero config. The full numeric/string/count/expiry limits and nested DTO inventory are copied exactly from that candidate and pinned by independent golden/mutation fixtures.

Task23 is a standard-library-only `internal/admissionbinding` producer: `EncodeV1(BindingV1) ([]byte,error)`, `DecodeV1([]byte) (BindingV1,error)`, `DigestV1(BindingV1) ([32]byte,error)`, plus strict cross-field `ValidateV1`. It imports no admission, control, journal, supervisor or service package. It may follow Task22 without Task4 because these existing source structs are all on main and its new leaf package consumes no owner capability. Task31 independently produces the revision-aware Go client contract after Task22 without consuming a Task4 owner; Task24 adds field-specific retention checking with the conservative policy below; Task25 adds atomic durable schema/mirrors, Task29 extracts pure quota/storage behavior and Task30 produces the closed journal transaction APIs; Task26 activates exact worker config source mapping after Task4; Task27 adds isolated nonblocking observer primitives. All runtime consumers retain Task22 and Task4-derived dependencies.

#### Standard-library leaf errors and effectful mapping

The standard-library-only package owns these exact exported sentinels in `internal/admissionbinding/codec.go`; no v2contract import or typed wire error is permitted in that leaf:

```go
var ErrInvalidContract = errors.New("admissionbinding: invalid contract")
var ErrCapabilityUnsupported = errors.New("admissionbinding: capability unsupported")
```

`ValidateV1(BindingV1) error`, `EncodeV1`, `DecodeV1` and `DigestV1` return errors matching these identities through `errors.Is`, including after caller wrapping with `%w`; callers never classify error strings. Malformed/duplicate/unknown/missing/noncanonical fields, inconsistent supported-version data and invalid hashes match ErrInvalidContract. An otherwise well-formed unsupported future SchemaVersion or an ADR0022 explicitly unsupported bound/capability matches ErrCapabilityUnsupported; malformed input is not promoted to a future capability. The two classifications are exclusive, successful controls return nil, and no failed decode/encode/digest result is usable evidence. Leaf retention codecs/structural validators use the same sentinel contract; structural validity is never retention authorization.

At effectful admission, journal, supervisor and service entry boundaries, translate `errors.Is(err, admissionbinding.ErrInvalidContract)` to `v2contract.CodeInvalidContract` and ErrCapabilityUnsupported to `v2contract.CodeCapabilityUnsupported`, returning an error discoverable by `errors.As` as `*v2contract.ControlError`. Preserve the existing authenticated scope/operation identity and disposition rules; use only constant Detail maps `map[string]string{"mythhelm.admission_binding/reason": "invalid_contract"}` or `map[string]string{"mythhelm.admission_binding/reason": "capability_unsupported"}` (the slash is required by `internal/v2contract/errors.go` namespacedKey), never decoder text, input bytes, paths or the wrapped error message. Unexpected leaf errors fail closed as invalid_contract with the same safe constant detail; they grant no authority and cause no persistence/native effect. Actual retention-policy/approval refusal at `admission.CheckRetentionV1` remains CodePolicyIneligible with constant Detail map `map[string]string{"mythhelm.retention/reason": "policy_ineligible"}`, including structurally valid but forbidden/unclassified/pin-mismatched records. A consumer must not turn policy refusal into a codec error or treat a sentinel/digest as a grant. The leaf exposes no ControlError constructor and importing v2contract into it is a test failure.

#### Retention and staged preparation

Task24 defines closed RetentionPolicyV1/RetentionApprovalV1 data in internal/admissionbinding/retention.go and aliases/checking in internal/admission/retention.go. A static policy rule identifies exact BindingV1 field path, classification (`non_secret` or `forbidden`) and provenance, with root/purpose/execution-policy scope in its content-addressed artifact; candidate value hashes are derived separately from the bound data; unknown/missing rows reject. Approval binds the exact canonical candidate digest, classified field/value set, authenticated standalone peer, root, generation/context epoch, expiry and affirmative `retain-local-binding` challenge. No wildcard, broad host permission or repository text grants retention. Native subscription material and full environment are always forbidden; provenance from auth stores/transcripts or credential-valued environment/argv refuses even with generic approval. These are finite typed-field/provenance controls, not a claim to recognize every possible secret in arbitrary text.

Task8 holds the unapproved request/plan/task artifact in bounded service RAM (at most32 preparations total/8 per authenticated peer, each bounded by the1MiB canonical binding and1MiB task input caps, with the ADR5minute expiry; these count/byte limits are not a hard process-memory claim) and emits the exact retention challenge only over the authenticated standalone channel; no preparation JSON, binding bytes, GitEmail/title/path/check-argv/challenge text or grant is durably stored before approval. Pre-approval restart forgets that RAM preparation and returns dependency_stale; it cannot recover approval from a digest. After affirmative exact retention consent, Task30 atomically persists the approved preparation/retention answer and recomputes Ready from the entire immutable required-answer set in that same transaction. Retention-only consent has zero remaining answers and is Ready=true immediately; Ready=false applies only while another required answer is missing. Subsequent applicable confirmations are durably bound there and recompute the same predicate. No trust/admission/native effect follows from retention consent alone. The trusted CLI renders approved values; attachment/host methods cannot prepare, confirm or retrieve them. If actual policy/approval is unavailable, native service preparation remains unsupported; synthetic fixture approval is not a real operator grant. The first default policy denies every unclassified value; this spec neither provisions live retention authority nor infers native-account facts.

#### Exact retention data types and authority distinction

The following are new closed structural records in `internal/admissionbinding/retention.go`, not runtime grants; canonical keys use the same case-sensitive Go identifiers as BindingV1. Every slice is copied on validation; sorted Rules and derived field rows are unique by exact DTO field path, at most256 entries, with ordinary strings<=4096bytes and existing ID/hash/expiry rules.

```go
// package admissionbinding
type RetentionClassV1 string
const RetentionNonSecret RetentionClassV1 = "non_secret"
const RetentionForbidden RetentionClassV1 = "forbidden"
type RetentionProvenanceV1 string
const RetentionOperatorInput RetentionProvenanceV1 = "operator_input"
const RetentionServiceConfig RetentionProvenanceV1 = "service_config"
const RetentionNativeStatus RetentionProvenanceV1 = "native_status_nonsecret"
const RetentionNativeAuth RetentionProvenanceV1 = "native_auth_store"
const RetentionTranscript RetentionProvenanceV1 = "native_transcript"
const RetentionCredentialEnv RetentionProvenanceV1 = "credential_environment"
type RetentionFieldV1 struct {
    FieldPath, ValueSHA256 string
    Classification RetentionClassV1
    Provenance RetentionProvenanceV1
}
type RetentionRuleV1 struct {
    FieldPath string
    Classification RetentionClassV1
    Provenance RetentionProvenanceV1
}
type RetentionPolicyContentV1 struct {
    SchemaVersion int64 // exactly1
    RootID, Purpose, ExecutionPolicyRevision string // Purpose local_run_binding
    Rules []RetentionRuleV1 // complete sorted unique exact paths; no values/hashes
}
type RetentionPolicyV1 struct {
    Revision string // sha256:<lowercase digest of Content>, verified, never a label
    Content RetentionPolicyContentV1
}
type RetentionDecisionV1 string
const RetentionApprove RetentionDecisionV1 = "approve"
const RetentionReject RetentionDecisionV1 = "reject"
type RetentionApprovalV1 struct {
    SchemaVersion int64 // exactly1
    PreparationID, ChallengeID, BindingSHA256, FieldSetSHA256 string
    PolicyRevision, PeerID, RootID, ContextEpoch string
    Generation int64
    ExpiresAt, ObservedAt string
    Decision RetentionDecisionV1
}
func (p RetentionPolicyV1) Validate() error
func (a RetentionApprovalV1) Validate() error
func RetentionPolicyDigestV1(RetentionPolicyContentV1) ([32]byte, error)
func DeriveRetentionFieldsV1(BindingV1, RetentionPolicyV1) ([]RetentionFieldV1, error)
func RetentionFieldSetSHA256([]RetentionFieldV1) ([32]byte, error)
func EncodeRetentionPolicyV1(RetentionPolicyV1) ([]byte, error)
func DecodeRetentionPolicyV1([]byte) (RetentionPolicyV1, error)
func EncodeRetentionApprovalV1(RetentionApprovalV1) ([]byte, error)
func DecodeRetentionApprovalV1([]byte) (RetentionApprovalV1, error)
// package admission; aliases preserve these declarations without a dependency cycle.
type RetentionPolicyV1 = admissionbinding.RetentionPolicyV1
type RetentionApprovalV1 = admissionbinding.RetentionApprovalV1
func CheckRetentionV1(binding admissionbinding.BindingV1,
    policy RetentionPolicyV1, approval *RetentionApprovalV1, now time.Time) error
```

No field-set or policy object is added to AdmissionChallenge or RunConfirmation: their schema2 fields remain exactly those already implemented. The compatible commitment is internal and deterministic:

1. Canonically encode the complete RetentionPolicyContentV1 using the exact declarations, sorted unique Rules, strict duplicate/unknown/missing-field rejection and1MiB cap. Its domain-separated digest is SHA-256 of `MYTHHELM:retention-policy:v1\n` plus those bytes. RetentionPolicyV1.Revision must equal `sha256:` plus that lowercase digest, and BindingV1.PolicyRevision/Decision.Environment.PolicyRevision must equal that exact reference. ExecutionPolicyRevision separately pins the full existing execution/acceptance policy by verified content digest, checked against the known policy passed to admission commit; no bare friendly label is accepted.
2. The trusted service loads the complete immutable policy content by that reference, verifies its bytes/digest and root/purpose, then resolves the exact canonical BindingV1 field paths covered by Rules. FieldPath uses the declared case-sensitive DTO paths and explicit decimal array indices (no wildcards); each retained field must be covered exactly once, unknown/unclassified/forbidden/missing/extra rules refuse. For each covered value, derive RetentionFieldV1 with ValueSHA256 over its exact canonical JSON value and the rule's classification/provenance. These resolved value rows are **not** part of the policy Content or BindingV1; their sorted canonical encoding yields the internal RetentionFieldSetSHA256 under the separate retention-fields:v1 domain.
3. Challenge.Text discloses the actual local retention root/purpose and field/value categories requiring consent, using the existing bounded Text field. It embeds neither its containing binding digest nor field-set digest. The existing wire BindingSHA256 commits the complete binding, including the policy content-address reference, challenge ID/kind/Text and candidate values. Existing RunConfirmation echoes only preparation/challenge/binding/Approved; the authenticated service checks those exact values. No imagined wire metadata or independent field-set authority is used.
4. The service derives the internal RetentionApprovalV1 from that authenticated answer and recomputed policy/field-set, then the confirmation transaction reloads/verifies the pinned full policy and binding and recomputes the same rows/digest before storing. CheckRetentionV1 compares the internal record against this derived metadata, never a field-set hash supplied by a client. Tampering policy bytes under a retained reference, changing any value/rule/provenance/root/purpose, using a missing policy, changing challenge text or copying approval across binding rejects with zero mutation/launch. A hash-only consent fixture with no policy content must fail.

The dependency order avoids a hash cycle: static policy content contains rule paths/classifications/provenance and the separate execution-policy reference, never BindingSHA256/FieldSetSHA256, candidate values or challenge text. Its digest is fixed first; challenge text and canonical binding are fixed next; resolved field-set and internal approval are computed last outside the binding. Retention codecs use distinct retention-policy:v1/retention-approval:v1 domains and exact copied records. Forbidden native-auth/transcript/credential-env provenance always refuses; absence/unknown remains unavailable. This finite typed-field/provenance design is not universal secret detection.

CheckRetentionV1 and record constructors check shape/consistency, not whether a human authorized anything. A caller-constructed RetentionApprove string, Approved bool, policy or digest is never authority. The only production approval source is the ordinary authenticated standalone run.confirm handler, which matches its OS-transport-derived peer/root/generation and the live pending preparation/challenge. It maps the existing RunConfirmation.Approved answer to the closed decision after those checks; it never accepts a RetentionApprovalV1 or policy blob from a new wire method, host, plugin or repository. Service stores that answer in its private volatile preparation until retention checks pass, then writes the exact confirmation through the Task30 journal APIs below. Missing configured policy/real approval remains unavailable; test fixture construction grants no live authority. Trusted Go code/same-user administrators are not adversarially isolated by these types.

The exact order is: (1) bounded RAM request/plan and challenge, no durable sensitive preparation; (2) authenticate and verify retention answer against that live binding/field-set/policy; (3) StoreRetainedPreparation writes approved bytes plus retention confirmation and computes Ready from the complete required set (true immediately for retention-only); (4) ConfirmPreparation records each remaining authenticated bound answer and recomputes Ready by the same rule; (5) ReadConfirmedPreparation validates that durable ready/not-consumed record; (6) supervisor.RevalidateService freshly resolves and seals it; (7) Start's admission transaction rechecks/consumes it. Rejection/expiry/context drift invalidates the preparation, never auto-approves it. Each write rechecks current identity/revision and rolls back on failure. Retention consent alone does not grant project/native trust, admit, reserve quota or launch.

#### Atomic readiness, including zero remaining answers

Readiness is derived state, never a caller-set bool or a later repair write. Both StoreRetainedPreparation and ConfirmPreparation, within their confirmation/ack transaction, load the exact nonempty required challenge set from the immutable canonical binding, validate unique recognized IDs/kinds and exactly one mandatory retain-local-binding challenge, then validate answers against that preparation/peer/root/context/generation/binding and applicable expiry/policy. Unknown/conflicting/rejected answers refuse/invalidate; no Ready=true is produced from them. Retention must be approved before any binding persistence. Only after those guards is Ready the conjunction that each required ID has one accepted affirmative answer. Missing additional answers mean false; no remaining IDs means true. Existing explicit authority can remove a non-retention challenge from the required set at preparation time, but cannot change that set after binding/confirmation.

The first durable transaction inserts the retention answer, computes this predicate against all required IDs and writes row revision1, Ready and the matching immutable PreparedRun acknowledgement/outer Result.Revision together. For one required challenge (retention only), revision1 is already Ready=true and ReadConfirmedPreparation may succeed immediately; no synthetic extra confirmation is required. For retention plus additional required answers, revision1 is false until the final valid answer, whose own acknowledgement reports true. A consistent replay returns the exact originally committed readiness/revision; a new stale mutation follows the existing conflict/current-metadata rules, not a local increment. All row/ack/readiness effects roll back together on any failure. Final fresh RevalidateService and the Start transaction's consumed/root/generation/policy rechecks remain mandatory even when Ready is true.

Zero remaining and zero total are deliberately different. This protocol always requires its bound retention challenge; a zero-total set lacks that mandatory authority path and fails closed with policy_ineligible/missing_retention_authority, persisting neither preparation nor grant. There is no new standing-retention-grant mechanism or vacuous empty-set success. Test zero-total refusal, retention-only success, additional-answer progression, rejection/conflict and all failure barriers separately.

#### Private prepared-admission dispatch and durable confirmation replay

Current `Server.Dispatch` sends normal handlers through `Execute` (`rg -n 'func .*Dispatch|return Execute' internal/control/server.go`); Execute stores the handler's result. Persisting a pre-retention PreparedRun challenge there would violate the volatile stage. Task8 therefore adds only this closed private dispatch seam:

```go
// package control; trusted internal registration, no new wire methods.
func (s *Server) RegisterPreparedAdmission(prepare Handler, confirm Handler) error
func PreparedAdmissionDigest(intent Intent) (string, error)
```

Registration binds exactly run.prepare/run.confirm, rejects nil/duplicate handlers and never exposes a general caller-selected bypass. Dispatch first validates the envelope and same-user standalone peer and refuses a capability token; the service handler then checks the current held root/generation and exact typed body before any lookup/replay/data release. These two methods bypass generic Execute/ExecuteLong: prepare and pre-retention confirmation use bounded volatile operation-ID/digest replay only; restart loses them and returns dependency_stale. The other methods' authentication/durable semantics remain unchanged. No raw challenge/plan is smuggled into operations.result before retention approval.

After retention approval, Task30 StoreRetainedPreparation/ConfirmPreparation own their durable confirmation transaction and immutable operation acknowledgement together. PreparationOperation contains the actual outer operation ID and PreparedAdmissionDigest computed by the trusted dispatch from the validated original intent; that helper reuses the exact current intentDigest algorithm (including compact params/object/generation) and rejects methods other than prepare/confirm. It is metadata/idempotency, not a permission grant. Journal checks matching preparation/root/peer/generation/revision and bound answer before mutation, stores the schema2 PreparedRun acknowledgement derived from the same canonical binding/state in operations.result, and returns that exact result on a consistent duplicate only after current authenticated scope validation. It never imports control: its closed internal acknowledgement serialization is independently golden-compared with the existing control.PreparedRun/Result codec. Different arguments under one operation ID conflict. Initial retention confirmation advances RAM preparation revision0 to stored revision1; its immutable acknowledgement carries the Ready value computed in that same transaction (true for retention-only, false if additional answers remain). Remaining distinct confirmations advance revision when their state changes and compute Ready before recording their immutable acknowledgement. A consistent duplicate answer under a new operation ID may record an acknowledgement of current state without changing the confirmation; a conflicting answer refuses. No claimed row or partial ack survives a failed confirmation transaction.

Task8 integration must prove generic operation persistence is never entered for an unapproved preparation, that scopes are checked before RAM/durable replay, and that post-retention confirmation plus ack roll back together on either write failure. Structural policy/constructor data alone cannot invoke this handler or mint a supervisor permit.

#### Package/API ownership and transactions

Task8 owns staged resolution in `internal/admission/service.go` and private sealing in `internal/supervisor/service_admission.go`: `supervisor.PrepareService(ctx, ServiceInputs) (*ServicePlan,error)` and `supervisor.RevalidateService(ctx,journal.ConfirmedPreparation,ServiceInputs) (*LaunchPermit,error)`. `ServiceInputs`, exact closed source mapping, context and expiry are ADR0022's inventory. Plan/permit private fields and their one-use state live wholly in supervisor; service gets only opaque handles/copy-only canonical metadata, never a Decision getter. Admission remains a reusable trusted resolver and imports no supervisor/service. This is a production registration and type/wiring control, not isolation from hostile same-process Go code.

Task30 owns typed records/transactions after Task25 schema and Task29 quota producers in `internal/journal/admission_bindings.go`: `ReadConfirmedPreparation(ctx,PreparationLookup) (ConfirmedPreparation,error)`, `CommitServiceAdmission(ctx,AdmissionCommit) (CommittedAdmission,error)` and `ArmServiceDispatch(ctx,DispatchArm) (CommittedDispatch,error)`. Closed constructor fields are exactly ADR0022's preparation/root/peer/generation/context/binding/policy/ID/snapshot/deadline/nonce inventory; no arbitrary projection callback or serialized Decision/env. Journal imports the DTO leaf and existing pure billing/acceptance/v2contract vocabulary, never admission/supervisor/service. Its fixed transaction helpers extract the current declaration/trust/envelope/quota/policy operations without reversing that dependency; legacy admission retains its existing evaluator seam while sharing checked duplicate/insert storage helpers. The fixed service policy uses pure billing.EvaluateQuotaSchedule; supervisor.EvaluateBucket delegates after mapping BucketRow fields, and malformed inputs refuse. Tests compare both production paths. No callback is passed into the new service APIs to evade validation. Fault/mutation tests prove the same current quota/refusal behavior and transaction rollback rather than accepting a caller-computed reservation result.

The mandatory additive migration is `internal/journal/migrations/0010_service_admission_bindings.sql`, recheck free number before implementation. It adds ADR0022's immutable bindings/consumptions, pending dispatch and evolving dispatch claims. The service_admission_bindings row also has distinct immutable canonical retention-policy and retention-approval bytes/digests, written only after exact consent; these do not alter the BindingV1 digest or expand closed RunSpec/RunConfirmation JSON. Old schema9 records lack them and refuse. It adds their source relations, unique/rowid/replace guards and run-revision triggers. Schema9 legacy preparations lacking those records cannot start. Update current pragma, migration list, backup/preview/import/version mirrors and their independent tests together in Task25; do not edit 0009. Task28 first replaces the existing hardcoded public backup-version example with the source-grounded version-parameterized naming contract; Task25 depends on that docs producer and updates all actual current source mirrors together. This four-file spec amendment changes no schema/version mirror and claims no schema10 runtime implementation.

#### Exact opaque metadata and journal construction seam

All following declarations are planned, not existing APIs. NewExecutionContext validates the held adopted root/OS principal/generation, captures service-owned policy-permitted context and creates a fresh unpredictable epoch; it has no caller Env/config-value overlay, performs no unapproved native startup/probe and grants no retention/trust authority. Options come from trusted service composition, not RunSpec/host/repository fields; absent actual retention policy keeps the route unavailable. Close invalidates the epoch, exposes no environment getter and waits for no owned pipeline from a callback. Private state types below are unexported concrete structs owned by their declaring package; no private field is accessed from another package. Getter return values are copies; later source/slice/map mutations cannot alter a stored plan/permit/record. A nil, closed or invalid handle returns a typed invalid_contract; stale/consumed handles are rechecked at every effectful operation.

```go
// package supervisor
type ServiceInputs struct {
    RootID, PeerID, ContextEpoch, PrincipalRef, PolicyRevision string
    Generation int64
    Request admissionbinding.RunSpecV2
    TaskArtifactRef string
    Context *ExecutionContext
}
type ExecutionContextOptions struct {
    PolicyRevision string
    RetentionPolicy admissionbinding.RetentionPolicyV1
}
type ExecutionContextMetadata struct {
    RootID, ContextEpoch, PrincipalRef, PolicyRevision, NativeConfigIdentity string
    Generation int64
}
type ExecutionContext struct { state *executionContextState }
func NewExecutionContext(context.Context, *journal.Journal,
    *control.InstanceLease, ExecutionContextOptions) (*ExecutionContext, error)
func (c *ExecutionContext) Metadata() (ExecutionContextMetadata, error)
func (c *ExecutionContext) Close() error
type PlanMetadata struct {
    PreparationID, BindingSHA256, RootID, PeerID, ContextEpoch, PolicyRevision string
    RunID, TaskID, AttemptID string
    Generation int64
    ExpiresAt time.Time
    Challenges []admissionbinding.ChallengeV1
}
type PermitMetadata struct {
    Plan PlanMetadata
    PermitNonceSHA256 string
}
func (p *ServicePlan) Metadata() (PlanMetadata, error)
func (p *ServicePlan) CanonicalBinding() ([]byte, error)
func (p *LaunchPermit) Metadata() (PermitMetadata, error)
// No method returns admission.Decision, ProcSpec, a full Env or mutable private state.

// package journal; imports admissionbinding, acceptance and v2contract,
// never admission, supervisor, service or control.
type PreparationLookup struct {
    PreparationID, PeerID, RootID, BindingSHA256, ContextEpoch string
    Generation int64
    ExpectedRevision uint64
    ObservedAt time.Time
}
type PreparationState struct {
    PreparationID, BindingSHA256 string
    Revision uint64
    Ready, Consumed bool
}
type PreparationOperation struct { OperationID, ParamsSHA256 string }
type ConfirmationV1 struct {
    Operation PreparationOperation
    ChallengeID, BindingSHA256, PeerID, RootID, ContextEpoch string
    Generation int64
    Decision admissionbinding.RetentionDecisionV1 // approve/reject; structural answer
    ObservedAt time.Time
}
type RetainedPreparationInput struct {
    Operation PreparationOperation
    CanonicalBinding []byte
    Policy admissionbinding.RetentionPolicyV1
    Retention admissionbinding.RetentionApprovalV1
    Lookup PreparationLookup
}
func (j *Journal) StoreRetainedPreparation(context.Context,
    RetainedPreparationInput) (PreparationState, error)
func (j *Journal) ConfirmPreparation(context.Context,
    PreparationLookup, ConfirmationV1) (PreparationState, error)
type ConfirmedPreparation struct { state *confirmedPreparationState }
func (p ConfirmedPreparation) Metadata() (PreparationState, error)
func (p ConfirmedPreparation) CanonicalBinding() ([]byte, error)
func (j *Journal) ReadConfirmedPreparation(context.Context,
    PreparationLookup) (ConfirmedPreparation, error)
type AdmissionCommitInput struct {
    Preparation ConfirmedPreparation
    Lookup PreparationLookup
    OperationID string
    CanonicalBinding []byte
    AcceptancePin acceptance.Pin
    Policy v2contract.PolicyVersion
}
type AdmissionCommit struct { state *admissionCommitState }
func NewAdmissionCommit(AdmissionCommitInput) (AdmissionCommit, error)
func (c AdmissionCommit) Snapshot() (AdmissionCommitInput, error)
type AdmissionMetadata struct {
    OperationID, PreparationID, RootID, PeerID, ContextEpoch, BindingSHA256 string
    RunID, TaskID, AttemptID string
    Generation int64
    Revision uint64
}
type CommittedAdmission struct { state *committedAdmissionState }
func (c CommittedAdmission) Metadata() (AdmissionMetadata, error)
func (j *Journal) CommitServiceAdmission(context.Context,
    AdmissionCommit) (CommittedAdmission, error)
type DispatchArmInput struct {
    Admission CommittedAdmission
    ExpectedPendingRevision uint64
    SnapshotEventID, BindingSHA256, LaunchTokenSHA256, NonceSHA256 string
    LadderVersion, PolicyRevision string
    Generation int64
    ExecutionDeadline, ObservedAt time.Time
}
type DispatchArm struct { state *dispatchArmState }
func NewDispatchArm(DispatchArmInput) (DispatchArm, error)
func (a DispatchArm) Snapshot() (DispatchArmInput, error)
type DispatchMetadata struct {
    Admission AdmissionMetadata
    SnapshotEventID, LaunchTokenSHA256, NonceSHA256, LadderVersion string
    ExecutionDeadline time.Time
    Revision uint64
}
type CommittedDispatch struct { state *committedDispatchState }
func (c CommittedDispatch) Metadata() (DispatchMetadata, error)
func (j *Journal) ArmServiceDispatch(context.Context,
    DispatchArm) (CommittedDispatch, error)
```

NewAdmissionCommit/NewDispatchArm deep-copy their closed inputs and validate canonical identity/hash/pin/policy/snapshot/deadline shapes; Snapshot returns another copy. They do not authenticate a peer, mint supervisor LaunchPermit, mark a preparation ready, reserve quota or authorize execution. Confirmed/Committed handles are produced only by successful checked journal operations, not JSON decoding or a public row-shaped constructor. Journal uses actual current rows in its transaction, never trust in an old handle/Ready bool. The service's authenticated ordinary handler supplies Lookup from OS transport/current root context rather than accepting its claimed peer/root from untrusted params. Supervisor independently requires held ServiceOwner plus its own freshly resolved one-use LaunchPermit before any execution. Typed storage handles identify validated storage effects; they do not isolate hostile Go code or replace that authority chain.

CommitServiceAdmission derives run IDs, route/identity/bucket/ceilings and the canonical admission record from the verified binding and its fixed source mapping; AcceptancePin/Policy must equal the pinned definitions and derived check evaluator/timeouts, never an arbitrary caller choice. It uses fixed quota logic from Task29, not a supplied success result/evaluator callback. ArmServiceDispatch verifies SnapshotEventID belongs to the same committed run/config/task and the exact pending intent, checks deadline/counter policy against current envelope and commits all arming effects together. Unknown, extra or internally inconsistent constructor fields refuse.

Typed failure mapping is closed: invalid_contract for shape/unknown field/constructor inconsistency or invalid handle; permission_denied for unauthenticated/foreign scope (uniform non-disclosure); revision_conflict for expected-revision/generation/consumption conflict; dependency_stale for expiry/context or required-evidence drift; policy_ineligible for forbidden/unclassified retention or ineligible pinned policy; capability_unsupported for future codec/missing mandatory capability/capacity; persistence_unavailable for database/transport failure; allowance_exhausted for quota refusal; ownership_unresolved for absent/stale owner; protocol_mismatch/event_too_large for the oversized observation case. Effectful producers return errors discoverable by errors.As as *v2contract.ControlError with bounded namespaced diagnostic detail and no raw values. The standard-library-only admissionbinding leaf instead returns its two errors.Is sentinels defined above; effectful boundaries perform the explicit mapping, so this typed-error rule does not add a v2contract dependency to that leaf. The service maps those codes to the existing control wire Error; no raw native/error text leaves it. Constructors' valid structural controls must be paired with unauthorized public-start fixtures proving no native/state effect without the authenticated confirmation+fresh-permit+owner chain.

Task9 calls `supervisor.RunAdmittedOwned(ctx,j,owner,permit,committedAdmission,sink)`; no client Decision, caller Hooks, test BindStep or unapproved extension enters it. Admission transaction creates one run/admission/acceptance pin, reservation/envelope/declaration/native trust, canonical assignment plus schema2 `service.run_assigned`, pending intent and immutable operation acknowledgement, then consumes preparation. Planned AttemptID is not an attempts row. Snapshot/task/config validation and `workspace.snapshot_created` plus project trust occur next outside that transaction. Post-snapshot arming atomically creates exactly one attempt/launch-intent/counter/deadline/worker pin and schema9 outbox/claim. Existing `runLocked` initial insertions cannot execute again. Extract shared fixed helpers in pipeline/state/acceptance/budget/worker-pin code, retaining legacy behavior. Crash before commit rolls back; lost response replays the exact acknowledgement; missing live permit/environment epoch after restart or ambiguous spawn becomes unconfirmed/interrupted, never a fresh launch. Worker identity/token/nonce persistence remains existing worker.json behavior; only journal records are hash-only.

Task26 makes the current `admittedConfigPaths` mapping strict: derive the available source→path inventory through the pinned adapter's existing inventory API, select only exact admitted manifest sources, reject missing/unknown sources, and bind final payload keys one-to-one with digests. Worker rederives expected source paths under its pinned adapter/home/environment before hashing, rejects extra/missing/swapped payload sources even if two files have equal bytes, and still rechecks digests. Static available inventories may contain unadmitted candidate paths; they must not leak into the final launch. No auth-store bytes or subscriptions are copied and no new adapter qualification follows. Core paths suffice; if an adapter API must change, implementation stops for a separately scoped adapter producer rather than widening Task26.

#### Independent quota producer (Task29)

```go
// package billing; standard-library-only policy input, no journal type.
func EvaluateQuotaSchedule(exhaustedAt string, resetAt *string,
    retriesUsed int64, now time.Time) (admit bool, wait time.Duration, giveUp bool)
// package journal; trusted storage input, not a permission grant.
type QuotaReservationInput struct {
    RunID, Bucket string
    ExecutionNanos int64
    ObservedAt time.Time
}
type QuotaRefusal struct { Bucket string; Wait time.Duration; GiveUp bool }
func (e *QuotaRefusal) Error() string
func (e *QuotaRefusal) Unwrap() error // billing.ErrAllowanceExhausted
var ErrDuplicateQuotaHold = errors.New("duplicate quota hold")
func ReserveRunQuota(context.Context, *sql.Tx, QuotaReservationInput) (string, error)
func InsertRunQuotaHold(context.Context, *sql.Tx, QuotaReservationInput) (string, error)
```

ReserveRunQuota validates current bucket scheduling via the pure policy then calls the common checked duplicate/insert helper; owner/scope are derived from RunID/Bucket, quantity is unknown, expiry is the exact existing execution-plus-one-hour rule. Input time is internal observed clock data, never a wire timestamp; production CommitServiceAdmission takes a fresh service/journal clock observation inside its transaction. Legacy admission retains its custom evaluator seam, performs that check first and shares only InsertRunQuotaHold; production EvaluateBucket delegates to the same pure schedule. Legacy adapters translate the new duplicate/refusal errors to their existing public errors without erasing causes. Task30 maps low-level quota refusal to allowance_exhausted. Tests compare both actual production paths and rollback before any new service writer is exposed. Task25's remaining migration/guard/current-version/import mirrors are an atomic compatibility unit; Task29 policy extraction and Task30 typed APIs are independently tested producers, not an unbounded sizing exception.

#### Observer lifecycle contract for Tasks6/7/10/27

Task27 defines `supervisor.ObserverSink` with private shared state, `NewObserverSink() *ObserverSink`, `Committed(runID string,sequence int64)`, `Hint(NoticeCode)` and idempotent nonblocking `RequestClose()`. Committed only wakes a capacity-1 channel; authoritative payloads stay in the journal. Hint uses a 16-code ring, replaces oldest on overflow and saturates hints_dropped; no outcome depends on a hint. NoticeCode has only reservation_held, worker_reattached and render_refresh informational values; Hint rejects every other value without cursor/outcome effects. A separate DiagnosticCode has quota_unavailable, worker_identity_unresolved, worker_exited_early, spool_ingestion_failed, candidate_freeze_failed, verification_unavailable, verification_binding_unavailable, stop_requested, deadline_reached and recovery_ownership_unresolved. Unknown DiagnosticCode refuses persistence as invalid_contract. These require durable evidence for quota/worker-identity/spool/early-exit/freeze/verification/stop/deadline/recovery failures. Required evidence is journaled as an existing typed state/event, or a strict schema2 service.diagnostic containing only code and matching run/attempt/generation/evidence refs, before hinting. Persistence failure remains unavailable/unconfirmed. No raw errors, paths, account/native text or unknown notice category may enter diagnostic/hint storage. The actor owns its callback sink; public observers own separate service view contexts/slots. Cancelling one public view never calls RequestClose on the actor sink or cancels another view. A callback may request closure of its own delivery lifetime; public pull reads remain journal-based and actor ownership is unchanged. The coordinator joins/closes the actor sink only outside its callbacks.

The exact consumer API is also in package supervisor; the service cannot inspect private sink fields:

```go
type NoticeCode string
const NoticeReservationHeld NoticeCode = "reservation_held"
const NoticeWorkerReattached NoticeCode = "worker_reattached"
const NoticeRenderRefresh NoticeCode = "render_refresh"
type HintBatch struct {
    Codes []NoticeCode
    Dropped, Rejected uint64 // cumulative saturating display-only counters
    Closed bool
}
type ObserverReader struct { state *observerReaderState }
func (s *ObserverSink) OpenReader() (*ObserverReader, error)
func (r *ObserverReader) Wakeups() <-chan struct{}
func (r *ObserverReader) Done() <-chan struct{}
func (r *ObserverReader) DrainHints(limit int) (HintBatch, error)
func (r *ObserverReader) Close() error
```

One service adapter reader lease exists per actor sink; a second OpenReader returns revision_conflict/observer_reader_active until the first closes. Public Observe handlers do not acquire this reader or drain its hints: they use canonical journal pages and independent view slots. Wakeups exposes the capacity1 read-only channel, never closes it, and callers select with Done; this prevents send-on-closed-channel races. Done is closed once when that reader closes or the sink RequestClose is processed. DrainHints is nonblocking, deep-copies/removes up to16 oldest hints under a short mutex (limit0 means16; negative/>16 is invalid_contract), returns cumulative counters and Closed, and never advances an event cursor. Unknown/authority-bearing Hint input increments Rejected and stores no code; closed-sink Hint/Committed calls are no-ops. A sink-closed reader may drain its remaining bounded hints once/again to empty; after Reader.Close, DrainHints is invalid_contract. Reader.Close is idempotent, releases only its lease/signals Done, and neither closes the sink nor waits for/joins the actor; a new reader may open while sink remains live. RequestClose makes the sink closed atomically, signals its active reader and refuses future OpenReader with dependency_stale/observer_closed, without closing Wakeups or touching ownership. Concurrent calls/copies share one synchronized state. Nil sink producers are no-ops; nil OpenReader/DrainHints/Close report invalid_contract where an error return exists. A nil Reader.Wakeups returns nil and nil Reader.Done returns an already-closed channel, so consumers can exit without panic. No errorless method invents a success/error record. Reader lease identity prevents an old copied reader from draining a later reopened lease; all close/drain/counter operations are linearized under the short mutex. The service adapter owns and closes the reader outside callbacks and uses only these exported methods; no function-field callback or Go friend access is assumed.

Task6 wires that primitive in `internal/service/observer_adapter.go`, tracks view/slot state in `internal/service/observe.go`, classifies current pipeline/recover/acceptance Notice call sites and supplies no client callback, terminal Detach or Interrupt to owned execution. Source-completeness tests require every Notice producer to declare durable evidence or bounded hint treatment. Callback/transport close only signals RequestClose; the coordinator cancels that view's waits and releases its resources outside callback execution. Last-view close does not stop native work or release ownership. Whole-service shutdown refuses new requests, reconciles outstanding accepted work, cancels the owning context when required, joins active ingestion/callbacks outside those callbacks and only then closes ServiceOwner/lease. A blocked OS operation or active owned entry reports incomplete shutdown and cannot be treated as cancelled merely because a deadline expired. Task6's shell tests prove wiring/lifetime with deterministic synthetic callback barriers, not ordinary native ingestion; Task7 tests actual adoption/ingestion under a stalled observer.

Task10 modifies the actual existing runClient.Observe in internal/control/client.go (rg for its method and the r.Limit=64 rewrite), changing zero to128 and checking through NewRunClient and NewPreparedRunClient; it does not create control/run_client.go or redefine NewRunClient. It implements `Service.Observe(ctx,Peer,control.ObserveRun) (control.RunObservation,error)` in `observe.go`, leaving operation-status logic in existing planned `observation.go`. One pending Observe per peer/run connection, 64 total slots/16 per peer/8 read handlers; refuse exhausted capacity with capability_unsupported/observer_capacity and zero mutation. Pages are a consistent authenticated root/generation/cursor snapshot, <=128 events and <=1 MiB, requested wait<=1000ms, default limit128 and empty-poll backoff>=500ms. Oversized single event returns protocol_mismatch without cursor advance; no truncation or silent skip. Wakeups may coalesce, journal consumption may not. Client disconnect releases only view resources. Stalled response writes never block actor ingestion or acknowledgement. The existing two-second attachment/64KiB projection contract is separate. Neither context nor requested wait establishes forcible cancellation of arbitrary OS/database calls.

### 3.3 Integration task sizing and finite work envelopes

Tasks6–10 remain complex integration units after independent codec/retention/schema/quota/observer/client producers23–31 were split out. Their file counts below are derived from each current task's Files field (`scripts/harness/runspec.py tasks <tasks.md>` or an anchored TaskN/Files parser), not repository-wide search counts. This is a reviewed sizing decision, not permission to widen files or omit acceptance.

| Task | Listed files | Coupled acceptance boundary and why one integration PR remains useful |
|---|---:|---|
| 6 | 15 | Runtime root context, callback adapter/notice evidence, transport entry and join-before-owner-close must pass one shell/lifetime contract. Primitive queue behavior is already independent Task27; this task does not claim ordinary dispatch/worker qualification. A partial adapter/close path could otherwise look like a working production shell while deadlocking/releasing its owner. |
| 7 | 9 | One actual same-worker adoption/assignment transaction must agree with canonical append, recovery ingestion, immutable worker generation/deadline and observer-close barriers. Splitting the observed identity from its committed adoption would lose the same-launch crash control; storage/client primitives remain prerequisites. |
| 8 | 12 | Staged fresh resolution, supervisor-private sealing and dedicated prepare/confirm dispatch form one retention-before-persistence integration contract. Retention rules, journal transactions and Go revisioned client APIs already land separately in24/30/31. This task must show the real registered handler/SQLite stages together, not call a data constructor successful admission. |
| 9 | 14 | The production Start wrapper must integrate one admission transaction, out-of-transaction snapshot, one post-snapshot arming transaction and the extracted legacy pipeline without duplicate insertion/launch. Typed storage/quota/config mapping are separate25/29/30/26 producers; the remaining coupled crash trace and legacy regression is the integration unit. |
| 10 | 11 | Authenticated SQL snapshot/cursor selection, bounded handler slots and actual exported client/frame consumption must agree on the same page/default/error contract. Client metadata and sink primitives already land in31/27; a helper-only page or new shadow client cannot satisfy this producer. |

For each of these five tasks, dispatch records one initial total execution/work envelope of at most45minutes with a concrete15minute artifact checkpoint and at most2 repair passes, each at most15minutes, **inside the same total envelope**. Format/check/review work performed by that worker counts in its envelope. Independent acceptance and CI waiting are separately measured coordination phases with a60minute observation checkpoint; pending external checks then remain pending/parked while independent work continues, never silently passed or automatically rerun. A new native attempt or budget extension requires the existing workflow/authority path; the complex label does not authorize endless retries. Qualified MYTHHELM enforcement must be established before calling a limit hard; direct fallback uses recorded coordinator checkpoints and discloses their enforcement limits. If the first checkpoint shows another independently testable producer or files outside the listed scope, freeze a concrete partial result and propose the smallest split before dispatching it. These budgets bound future work; they are not measured duration, throughput or a claim that this integration has shipped.

## 4. Attachment protocol (v2 §§3, 8, 14–15)

Private methods: `attachment.open`, `attachment.read`, `attachment.stop`, `attachment.operation`, `attachment.close`. They are routed through a dedicated authenticated executor, not ordinary Execute's lookup-first fast path. Open authenticates the same local operator, resolves only the specified run and mints a random token; it does not reconcile or assign. Failure uses existing typed code vocabulary: permission_denied, revision_conflict, invalid_contract, capability_unsupported, persistence_unavailable, and ownership_unresolved/root conflict through the canonical control error mapping. Never return raw internal errors.

```go
// internal/control/attachment.go; JSON uses the lower_snake_case spelling of each field.
type AttachmentGrant struct {
    AttachmentID, ClientID, RunID, RepositoryID, Token string
    Generation int64
}
type Projection struct {
    SchemaVersion int
    AttachmentID string
    Generation int64
    ProjectionRevision, RunRevision uint64
    RunID string
    TaskID, AttemptID, Label, Harness, Model, WaitingReason *string
    RunState, ReviewState, Attention string
    Stop *StopResult
}
type OpenAttachment struct { Grant AttachmentGrant; Projection Projection }
type StopRequest struct { OperationID, RunID, AttemptID string; ExpectedRevision uint64; Generation int64 }
type StopAck struct { OperationID string; Accepted bool; RunRevision uint64; ErrorCode string }
type StopResult struct { OperationID, State, Reason string; AttemptID string; Ack StopAck }
type AttachmentClient interface {
    Open(ctx context.Context, runID string) (OpenAttachment, error)
    Read(ctx context.Context, grant AttachmentGrant) (Projection, error)
    Stop(ctx context.Context, grant AttachmentGrant, request StopRequest) (StopAck, error)
    Operation(ctx context.Context, grant AttachmentGrant, operationID string) (StopResult, error)
    Close(ctx context.Context, grant AttachmentGrant) error
}
func DialExisting(ctx context.Context) (Conn, error)
func NewAttachmentClient(conn Conn) AttachmentClient
```

Task11 produces only transaction-scoped internal grant primitives in control/attachment_auth.go, not public Open or a placeholder OpenAttachment. Exact signatures:

```go
func MintAttachmentGrant(ctx context.Context, tx *sql.Tx, peer Peer, runID string, generation int64) (AttachmentGrant, error)
func ValidateAttachmentGrant(ctx context.Context, tx *sql.Tx, peer Peer, grant AttachmentGrant, generation int64) error
func RevokeAttachmentGrant(ctx context.Context, tx *sql.Tx, peer Peer, grant AttachmentGrant, generation int64) error
```

Their caller holds the service instance-generation lifecycle guard and supplies its verified generation and one transaction. Mint resolves canonical run/repository scope from that transaction, verifies authenticated peer and resolved ownership, persists token hash, and returns the raw grant only to the trusted calling handler. Validate checks current hash/binding/revocation and exact caller scope; Revoke performs the same validation then changes attachment authority only. Unknown/foreign identity is permission_denied without disclosure, stale generation is revision_conflict, ambiguous ownership is ownership_unresolved, and database failure is persistence_unavailable. No primitive recovers/assigns/launches a run or releases raw grant data to transport before commit.

Task12 composes these primitives with its projection producer and registers attachment.open/read/close. Public Open mints the grant and selects its complete initial Projection in one guarded transaction, then commits before returning OpenAttachment; a projection failure rolls back grant/cache writes and returns neither token nor selected data. The required Projection is never null, fabricated or deferred to a later call. Read revalidates/selects coherently; Close revokes only attachment authority. The AttachmentClient interface remains unchanged.

DialExisting verifies the existing endpoint and canonical root with the current transport but never calls dialOrSpawn/Connect, starts a service, migrates or recovers. Attach/reconnect use it; a missing service returns explicit unavailable guidance. Ordinary launch clients retain the separate lazy-start Connect path.

Tokens remain solely in process memory and the authenticated transport, never argv, Herdr metadata or durable client restore descriptors. Store only the token hash. ClientID is core-minted and not the pane identifier. RepositoryID is an opaque canonical checkout identity, not a host-supplied path. Closed attachment tokens are revoked; close never changes execution. Fresh explicit reconnect authenticates again and receives a new grant; old-generation tokens fail. Stop/result query verifies requested object belongs to the current grant before revealing existence or outcome. Opening unknown/foreign IDs does not enumerate repository rows or leak counts.

Projection includes only the above fields, no generic raw payload map. Null fields render unknown/unavailable, never zero/empty as known. Label is normalized printable text, ≤160 Unicode scalar values, no C0/C1/ESC/OSC or bidi override controls; source file paths and prompt/task content are never labels. Use an opaque identifier-derived label if no explicitly safe label exists. Harness/model/waiting/review/attention values derive from canonical evidence, not host/native completion text. `ReviewState` is candidate, verified, applied or unknown; verified requires the exact ADR 0019 protected binding. `Attention` is none, review, stop_unconfirmed, blocked or unknown. StopResult state is requested, unconfirmed or confirmed, and Reason is a bounded enum code, never a native error.

## 5. Persistence, snapshots and revisions (v2 §§5, 8, 14)

Add an additive migration (currently `0009_attachments.sql`, recheck number) with attachment_grants, run_control_revisions, attachment_projections, stop_outbox, stop_results and prepared_runs/dispatch_outbox. Exact key contracts:

- attachment_grants: attachment_id PK, token_sha256 UNIQUE, client_id, run_id FK, repository_id, generation, revocation_revision, revoked; no raw token.
- run_control_revisions: run_id PK/FK, revision monotonically increasing. Add same-transaction SQL revision triggers for INSERT/changed UPDATE/DELETE on the closed projection-source set: runs, attempts, journal, candidates, verifications, verification_bindings, check_results, applies, run_assignments and stop_results. Resolve indirect run IDs through attempts or verifications; derive stop_results run through the immutable outbox. Runs INSERT initializes its revision row. Triggers ignore unchanged selected values; duplicate journal IDs remain deduplicated. Multiple relevant writes in one transaction may advance more than once; readers see only the committed revision. Scope revocation and instance generation are additionally validated from their own authority records. The journal.RunRevision helper reads this revision inside the caller transaction. Adding another projection dependency requires expanding the trigger/test inventory in the same change. No worker spool append alone is a canonical advance.
- attachment_projections: attachment_id+generation PK, projection_revision, source_run_revision, content_sha256, sanitized serialized content. Equal content retains revision; changed complete content increments it. Grant metadata writes do not mutate run execution state. Durable revisions survive transport reconnect within the same attachment; a new grant has a new sequence.
- stop_outbox: operation_id PK/FK to operations, run_id, attempt_id, launch_token_sha256, worker_pid/start_time, accepted_generation, delivery_state; no raw secret. stop_results: operation_id PK/FK, state, reason, confirming_event_id nullable, result_revision. Allowed state evolution requested→unconfirmed→confirmed, requested→confirmed; never confirmed without matching evidence.
- prepared_runs and dispatch_outbox bind operation/preparation/peer/generation, immutable admitted evidence digest and exact launch identity. They do not persist the caller environment or raw token.

Use operations.result for immutable StopAck only. Existing immutable result triggers remain intact. Migration preserves legacy rows/unknowns and newer-schema refusal. Schema tasks provide typed journal helpers rather than exposing journal's private DB field to host/client code.

One read transaction selects current grant authority, generation and all projection inputs. The instance lease's lifecycle read guard stays valid until the result is committed to the transport response queue (not until the socket finishes writing). Revocation/generation changes take the corresponding write guard; a read ordered before revocation may finish with that snapshot, but a failed revalidation releases zero selected bytes. Deterministic barriers prove either one valid linearization or a closed refusal, never mixed authority/data. A SQLite snapshot plus an unsynchronized read of lock metadata is insufficient.

Persist sanitized projection revisions atomically with authority validation; the read may maintain attachment cache/revision metadata, but it cannot mutate run/attempt execution or recover a worker. Snapshot content hash covers every selected field other than its own revision; adding a field requires expanding this coverage. RunRevision is the atomic stop precondition and is itself part of selected content; its change therefore advances the complete projection revision even when other displayed fields are unchanged. Cache/resync consumers compare attachment+generation+projection revision and do not use host sequence numbers as authority.

## 6. Atomic stop, replay and outbox (v2 §§6.4, 14–15)

A short transaction authenticates grant/scope, validates current generation, compares complete RunRevision and pinned attempt identity, records immutable acknowledgment and outbox, and advances run revision. Commit precedes acknowledgment. A competing update after client preparation produces revision_conflict and no outbox/stop.request. Authorization is checked before even reading/releasing a replay result. No worker I/O, watcher join or spool wait occurs in this transaction.

For a new operation, all preconditions apply. For an existing operation, first validate current scope and submitted generation, then compare the immutable command digest (run, attempt, original expected revision, submitted generation and method; excludes fresh bearer token/attachment ID); identical replay returns the same acknowledgment bytes without applying the old expected revision again. Changed arguments conflict. A generation change rejects old-generation resubmission; after fresh authenticated open, `attachment.operation` retrieves the same durable result by ID. The immutable acknowledgment remains queryable through standalone operation status. This explicitly separates recovering a lost acknowledgment from submitting a stale new command.

Dispatcher takes only the target actor's short coordination lock, verifies the still-pinned identity, then delivers outside SQL transactions. Existing workers.RequestStop is create-once per attempt. If its file already names this operation, treat delivery as already performed. If it names a different operator/deadline request, record this operation as coalesced with that exact stop effect; never replace the file or claim a separate physical delivery. Matching attempt.stopped evidence can resolve all accepted stop requests for that exact attempt; retain original requesting operation attribution separately. Corrupt/foreign request files or identity changes remain unconfirmed and block further delivery.

After crash between filesystem delivery and marking outbox delivered, inspect the create-once file plus worker evidence before any action. Missing file with matching stopped evidence resolves without delivery; ambiguous evidence stays unconfirmed. Resume accepted reconciliation independent of the originating client. Journal ingestion remains the only producer of worker-state transitions; dispatcher does not pre-mark attempt stopped. Unexpected death without evidence of process-boundary termination is not confirmed stop. No operation can target a later attempt after retry.

## 7. Service adoption and immutable worker envelope (v2 §§3, 6)

Actual grounding changes the minimal contract: `internal/workers/worker.go:535–574` (`rg -n 'awaitIfUnsupervised|func .*conclude' internal/workers/worker.go`) calls awaitIfUnsupervised only after the native episode ended and its result was spooled. `internal/workers/envelope.go:144` (`rg -n 'func AwaitReconnect|func CheckEnvelope' internal/workers/envelope.go`) permits a fresh heartbeat of any generation to release that completed-worker wait. It starts no episode and approves no effect. A file-driven worker-generation rehome handshake is therefore unnecessary for this slice and is not introduced.

Preserve immutable worker launch generation and execution deadline. Distinguish them from the new singleton's current control generation. Under held instance/run capabilities, service restart verifies canonical root, assignment, run/attempt IDs, stored launch-token hash, identity PID/start time against OS process identity and journal launch observations, supported spool schema and committed ingest offset. It durably records `ServiceAdoption` before enabling mutation for the run or writing presence. This record is a service reconciliation result, never a host/worker file authorizing ownership.

Exact core type in `internal/control/adoption.go`: `ServiceAdoption{SchemaVersion int; RunID, AttemptID, LaunchSHA256 string; WorkerPID int; WorkerStartedAt time.Time; LaunchGeneration, ControlGeneration int64; CommittedSpoolOffset int64; ReconciliationEventID string}`. `RecordServiceAdoption(ctx context.Context, tx *sql.Tx, adoption ServiceAdoption) error` appends the canonical reconciliation event and updates assignment within the same transaction after the actor has acquired its capability and verified evidence; raw launch tokens are excluded. Persist new event type `service.adopted` with envelope schema_version2 and payload schema_version2 (v2contract.SchemaVersion), plus the assignment generation, in one transaction. Do not use journal.EnvelopeVersion, which remains1 for historical event contracts. Actual journal.Append/appendTx already routes schema2 through appendV2 at `internal/journal/journal.go:290,429` (`rg -n 'func .*Append|func appendV2' internal/journal/journal.go`), subject to the migration phase gate. Add `journal.AppendCanonicalTx(ctx context.Context, tx *sql.Tx, envelope v2contract.Envelope, project func(*sql.Tx) error) error` in internal/journal/canonical_tx.go as a narrow schema2-only wrapper over the existing transaction-scoped append path; it validates envelope2 and phase, allocates run_sequence and applies assignment changes atomically. RecordServiceAdoption supplies a contiguous per-service-producer sequence, current control generation, observed time and the reconciliation event ID; duplicate event IDs do not reapply the assignment.

Add `control.DecodeServiceAdoption(event journal.Event) (ServiceAdoption, error)` with strict unknown/duplicate-key rejection, payload schema2, event type, run/attempt, generation and identity cross-checks. A service.adopted envelope or payload marked1 is invalid_contract; unknown future schema is capability_unsupported. General event reading dispatches historical schema1 to the existing v1 path and schema2 to v2contract; it does not reinterpret old bytes as adoption records or rewrite any historical envelope, payload, producer sequence or launch generation. Existing migration-owned v2 types retain their drained-phase exception; service.adopted requires imported/adopted and the service owner additionally requires adopted. No blanket EnvelopeVersion change or migration of historical native spool formats is allowed. All other newly introduced canonical event types in this child use schema2 as well. Typed append/decoder tests exercise pre-migration refusal, mixed v1/v2 history, unknown schema, malformed identity, rollback and duplicate replay before the runtime consumes them. Mismatched identities, unsupported spool or uncertain process state produce ownership_unresolved/quarantine and no new authority. A changed control generation never rewrites the worker launch or substitutes a new attempt.

Heartbeat remains presence/ingestion observation only. A forged, stale or changed-generation beat cannot launch work, mint an attachment/attempt capability, extend a deadline, approve an effect or certify a stop. The worker's completed wait may finish after a fresh beat, but core result classification still requires canonical matching process-boundary evidence. Continue rejecting nonzero-generation mismatch in CheckEnvelope for authority-bearing envelope checks; do not weaken it to accept any generation. No new attempt-token mint/rotation is required by this completed-episode wait, and attachment tokens stay separate. Arbitrary same-UID executable access under trusted-host is a disclosed host trust limitation, not a secret-file authentication claim. Contained native execution retains its established boundary; no new grant is accepted from native-accessible files.

Zero-generation legacy workers may be observed/adopted only after the explicit root migration identity/spool verification; zero never supplies new authority. Tests cover changed service generation with unchanged worker launch generation, PID reuse, corrupt identity, interrupted ingestion, service death before/after the adoption commit and completed wait release with zero new launches/effects/deadline extension. New native continuation or arbitrary worker-generation reauthorization remains outside this adoption operation.

## 8. Responsiveness and compatibility (v2 §§14–15)

Attachment operations have a two-second end-to-end deadline, 64 KiB frame ceiling and at most 16 queued projections per attachment. SQL busy handling is shorter than the remaining request budget (do not retain the current unqualified five-second busy timeout). Long worker work runs outside request tasks. Client/host queues carry immutable safe projections and never hold actor/transaction/network locks together. Overflow replaces pending status, marks stale and requires authenticated resync; stop acknowledgments are correlated operations, never discarded as replaceable status. A ten-second host stall must leave status and accepted/rejected control acknowledgment under two seconds; a deliberately blocking fixture must fail the same assertion.

MH-23 `tui-observatory-view` Tasks4–9 touch workers/pipeline/recover/CLI; ownership extraction waits for the relevant merged lifecycle contracts. Its later Task18 also edits internal/cli/tui.go, run.go and demo.go for launch-mode/theme wiring: this spec's public-client Tasks16–19 must serialize after that Task18 merge and re-ground mode/theme/signals before editing those branches. Retain worker deadline and distinct stop/detach intents; isolated contracts/storage/attachment primitives need not wait for unrelated TUI presentation work. A service client's detach closes its subscription only; do not forward Hooks.Detach to the service actor or release service ownership. Pre-migration legacy owner detach still uses MH-23's stop-less lock release and explicit recovery guidance. Attached existing-run views use inspect mode with no owner-detach hook. Linear explicit interrupts preserve their documented intentional stop behavior; pane teardown does not synthesize them. Existing CLI apply/recover and TUI trusted actions are routed to the service on adopted roots without granting those methods to attachment/host capabilities.

Windows ownership work may change worker/transport files; serialize those tasks and re-ground the proven boundary. Linux fixtures do not qualify Windows. No native fidelity/billing or protected acceptance qualification is inherited merely by moving orchestration into the service.

## 9. Decisions

ADR0021 records D1–D8. ADR0022 content at the exact reviewed/adopted candidate d8975b3 records D10–D13 below; its PR541 merge, this separate amendment merge and Task22 exact handoff remain outstanding gates until independently verified.

| ID | Decision | Rationale |
|---|---|---|
| D1 | Finish production singleton composition under ADR 0014. | Recovery-only per-run enrollment would narrow FR-1 and bypass current root migration. |
| D2 | Separate composition package and ownership capabilities. | Control importing supervisor cycles; boolean guard bypasses cannot prove ownership. |
| D3 | Closed service admission schema with bound confirmations. | Serializing Request callbacks/environment or trusting client Decision grants unjustified authority. |
| D4 | Separate scoped attachment grants. | Attempt token rotation and token-less same-user reads do not define attachment authority. |
| D5 | Complete run revision plus coherent projection snapshot. | operations MAX revision misses worker ingestion and evidence changes. |
| D6 | Immutable accepted acknowledgment plus mutable reconciled result/outbox. | ExecuteLong blocks acknowledgment; editing immutable operations.result breaks durable replay. |
| D7 | First-wins per-attempt stop with explicit coalescing. | One stop.request cannot represent independent repeated physical effects. |
| D8 | Root migration and canonical same-launch service adoption. | The worker launch generation stays immutable; presence alone cannot grant authority. |
| D9 | Permit design-only Task 22 review after Task 3 using a pinned Task 4 candidate. | Waiting for Task 4 to merge delays the design needed to assess that candidate. Inspection and re-grounding enable review without consuming an unmerged interface or relaxing runtime gates. |
| D10 | Closed immutable V1 binding and supervisor-owned private permit/Decision. | A digest beside mutable input or cross-package private-field access cannot authorize execution. |
| D11 | Pending admission intent, then post-snapshot arming. | Schema9 outbox needs an attempt; creating it early would start execution accounting during snapshot. |
| D12 | Journal pull with capacity-1 wakeups and 16 informational hints. | Client stalls and callback self-close must not block ingestion or certify an outcome. |
| D13 | Explicit fail-closed retention and strict config source mapping. | Unknown classification/source identity cannot be inferred safe from a hash or generic consent. |

Accepted ADR `docs/decisions/0021-service-attachment-composition.md` records D1–D8. Task 1's exact-revision architecture and code review and delegated acceptance are recorded in PR #501; this acceptance does not establish runtime or platform qualification.


## 10. Honesty register

Until the ADR/follow-on/Task22 gates merge and Tasks23–31 plus Task9 implement the precise contract, service start is unsupported and Task 4 alone does not establish AC-6.4.


| Spec demand | Position |
|---|---|
| Ordinary production attach-ready lifecycle | Required by this child; cannot be reported complete using internal helpers or recovered-only runs. |
| Existing active legacy owner | Explicit migration boundary: wait or safe owner detach, never attach-induced transfer; root-wide migration required before service writer. |
| Full MH-3 native writable takeover | Deferred; no native input authority or bytes through attachment. |
| Complete G11/AT-36/AT-48 | Partial after this epic; broader UX remains MH-23, exact evidence required. |
| Windows lifecycle | Unsupported until process-boundary implementation and evidence; accepted ADR is not execution evidence. |
| Herdr live support | Not established here; optional adapter child qualifies exact rows. |
| All v2 scheduling/native routes | This closes the existing admitted-run service seam, not a new general scheduler or expanded billing/native qualification. |

Core Task21 documentation serializes after both `specs/*/docs-site-restyle/` Task11 and `specs/*/tui-observatory-view/` Task19 merge, then re-ground its user-guide/example/label conventions before editing docs/user-guide.md or docs/limitations.md. This does not block independent core implementation.

## Cross-Spec References

Epic: `specs/*/herdr-embedded-client/plan.md`. Dependants: `specs/*/existing-run-client/` and `specs/*/herdr-scoped-projection/`. Source: `specs/*/herdr-bridge/requirements.md`. Prerequisite lifecycle: `specs/*/tui-observatory-view/` Tasks4–9 before shared lifecycle extraction, Task18 before core Tasks16–19 public-client edits, and Task19 before core Task21 shared documentation; Windows process tasks before Windows claims. ADRs 0014, 0016, 0017, 0019 remain binding.

### Design-review sequencing

Task 22 may design and review against merged Task 3 and a pinned, inspected Task 4 PR candidate before Task 4 merges. This separates identifying the missing contract from consuming it in runtime code. The ADR records the exact inspected revision and re-grounds affected evidence if it changes; an unmerged candidate supplies no admission or ownership authority. Task 4 retains its own implementation and review obligations, Task 5 still requires Task 4, and Tasks 6–9 retain their Task 22 ADR and separately reviewed follow-on spec gates. No unresolved PR finding is resolved by this sequencing change.
