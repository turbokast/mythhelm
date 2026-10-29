# Supervisor attachment control — Design

## 1. Current state and scope (v2 §§3–6)

Grounded at `61d18e2`. The original requirements are HOLDS; the claim that the production service already executes them is PARTIAL. `internal/control/client.go:168` (`rg -n 'func RunSupervisor' internal/control/client.go`) acquires the instance and directly serves `NewSupervisorServer`. Its complete registry at `internal/control/server.go:178` (`rg -n 'func NewSupervisorServer' internal/control/server.go`) contains status/assign/reserve/release/heartbeat/read, deliberately not stop/recover. `rg -n 'RegisterStop|RegisterRecover' internal` shows definitions and tests; the opened production registry confirms absence. `internal/cli/run.go:218` and `demo.go:111` (`rg -n 'admission.Decide|supervisor.Run' internal/cli/{run,demo,tui}.go`) admit and start CLI-owned pipelines, including live TUI/accessible callbacks.

`internal/supervisor/pipeline.go:167` holds the run owner lock and `:984` watches/ingests; `recover.go:43` (`rg -n 'func RecoverWithHooks' internal/supervisor/recover.go`) fences migration, acquires run ownership, reconciles, watches and freezes/concludes/repairs. It is not a read-only attachment helper. `internal/supervisor/ingest.go:64` requires ownership, and journal.Append commits event/projection/offset together. `internal/control/stop.go:95` owns its independent lock/state path; `execute_long.go:32` waits for eventual result. Registering either unchanged beside the pipeline is unsafe.

`internal/migrate/apply.go:81` and `drain.go:126` (`rg -n 'func (Apply|drainLocked|adoptable)' internal/migrate/{apply,drain}.go`) migrate a complete root under instance/state/all-run locks. Adoptable live workers retain launch identity after legacy owner release. `internal/supervisor/pipeline.go:204` (`rg -n 'func checkMigrationClear' internal/supervisor/pipeline.go`) refuses legacy work after previewed. `internal/control/ledger.go:104` checks only operations revisions. `internal/control/filter.go:42` permits token-less caller-selected repositories. Neither supplies bridge authorization/revision.

`internal/workers/envelope.go:62` (`rg -n 'func CheckEnvelope' internal/workers/envelope.go`) rejects changed nonzero generation, while zero is legacy unfenced. `internal/workers/worker.go:326` (`rg -n 'func RequestStop' internal/workers/worker.go`) creates one first-wins stop.request per attempt, not one per operation. `internal/journal/journal.go:37` declares schema version 8; additive migration 0009 is currently free and must be revalidated before implementation.

This child completes ADR 0014's ordinary production lifecycle and scoped core contract. Its original 21 bounded tasks and the proposed Task 22 design gate stay in the approved three-stream epic because a partially rewired owner/dispatch/ingest/control path cannot be advertised as an attach-ready service; each task remains independently reviewable. No per-run recover-enrollment mode, second service or recovered-only FR-1 claim is introduced.

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

Until Task 9 lands, `RunOwned` is a trusted internal ownership primitive, not proof of admission. Task 4 may exercise it with fixtures but must not wire it to a production request. Task 6 keeps run methods unsupported. No production dispatch may rely on a supplied mutable `admission.Decision` merely because its paths or configuration agree with itself. Task 9 must close this boundary before exposing start. Task 22 must resolve the synchronous callback and close hazards before Tasks 6 and 7: observer behavior must not deadlock ownership or compromise durable evidence, and shutdown must preserve ownership until active work has exited. The chosen delivery, overflow, cancellation and shutdown mechanism and its executable acceptance tests require a reviewed follow-on spec update before those tasks start. Task 4 tests do not establish this runtime property.

`ServiceOwner` has private identity/generation/lifetime fields; each entry verifies root/run/held capability. It is not a boolean migration bypass. Existing Run/RecoverWithHooks retain legacy guards during the explicit migration period and call extracted lock-held bodies. Service entries never reacquire their held run lock. Hook delivery from the actor uses bounded copies, never the terminal's blocking callback. Apply remains explicit and executes existing guarded candidate/ref rules, not a host action. RecoverOwned owns the full continuation of observation/verification/receipt repair, not a parallel `control.Reconcile` task.

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

Task 8 binds the complete execution-relevant admission inputs and evidence in the service-owned preparation: source repository and revision, task bytes and digest, checks and no-checks, project and native configuration/trust, route and profile, billing and envelopes, confirmations and launch-relevant values. Revalidation covers the entire binding and peer, generation and expiry. Secret values and a caller environment do not enter durable state; later execution reconstructs sensitive native material through the qualified service identity, and refuses if required evidence no longer matches. A digest supplied beside an independently mutable decision is not authority.

Start validates preparation in one transaction, records the immutable admitted bundle, assignment, operation acknowledgment and dispatch intent. Task 9 must execute only a decision reconstructed from, and verified against, that immutable committed binding; missing, stale, altered or mismatched data refuses before worker spawn. Integrate the current pipeline's initial run/attempt insertion with the precommitted records so a duplicate insert, retry or recovery cannot open an alternate admission path. Where schema 9's stored binding and digest cannot reconstruct or verify a required field, add a reviewed additive migration, never modify a released migration. The exact representation, Task 8 producer-to-Task 9 consumer contract and service-to-supervisor proof-bearing API are resolved and recorded by Task 22 before Tasks 6–9 proceed; until then the production start remains unsupported. Worker spawn remains the existing durable one-use launch protocol. A dispatcher crash before/after launch reconciles identity/spool evidence before deciding whether the same committed unconsumed launch may proceed; ambiguous launch is quarantined, never blindly replayed. New replacement/continuation launches require a new explicit admitted operation. The service calls RunOwned with its own lifetime context. Client death after acceptance does not cancel it.

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
func NewRunClient(conn Conn) RunClient
```

Add `SchemaVersion int` to RunSpec, RunConfirmation, StartRun, PreparedRun, RunAccepted, RecoverRun and ApplyRun; all require2. Generation for Start/Confirm/Prepare is the current root service generation in the outer control envelope (Prepare first resolves authenticated status); Prepare returns it and subsequent requests must match. Confirmation operation IDs are generated per answer; mutation preconditions remain in control.Intent. Start's expected_revision addresses its preparation revision, initially0, and is consumed exactly once with operation acknowledgment; RunAccepted.Revision is the resulting run revision. RecoverRun/ApplyRun already carry exact run revision/generation; a request cannot substitute a projection revision.

Wire method mapping: run.prepare→PreparedRun; run.confirm→PreparedRun after revalidation; run.start→immutable RunAccepted; run.recover/run.apply→immutable ActionAccepted; run.operation(OperationQuery)→OperationStatus; run.observe(ObserveRun)→RunObservation. Prepare, Confirm, Start, Recover and Apply pass their typed bodies through control.Intent; Operation and Observe are read handlers and never allocate an operations row or advance a run revision. Mutations require an outer operation ID and immutable digest of exact typed arguments. Failed preconditions return a typed control error and no effect. Accepted recover/apply reconcile asynchronously under the actor; they do not hold a request open until terminal. Client death never cancels accepted work.

OperationStatus.State is accepted, running, completed, failed or unconfirmed. Exactly one immutable acknowledgment arm matches Method: Start for run.start, Action for run.recover/run.apply, or Stop.Ack for an accepted stop. Outcome is populated for run.start/recover once observed, Apply for run.apply once reconciled, Stop for stop; incompatible arms are invalid_contract. Failure is populated only for a known failed/unconfirmed operation with a bounded canonical reason code. Running/accepted is not completion; unconfirmed is not failure or successful stop. RunOutcome.State/AttemptState use the existing supervisor state vocabulary without adding synthetic terminal states; Reason/AttemptReason retain bounded documented reason codes. CLI derives existing runExit semantics from this typed outcome; no daemon imports CLI renderer code. ApplyOutcome mirrors the existing guarded apply's exact branch and candidate commit. On replay/query, validate same-user/root/run authority before revealing the acknowledgment/result. Query of a prior-generation accepted operation is allowed under fresh authenticated current authority; it never resubmits or mutates that operation. Unknown/foreign operation returns permission_denied without cross-run existence disclosure.

Observation is bounded pull/long-poll, not a server-owned persistent subscription. A client subscribes by repeating run.observe with its returned NextSequence. Limit is1..128 (default64); WaitMillis is0..1000; AfterSequence is a nonnegative committed per-run journal sequence (0 is beginning). The handler first authenticates root/current generation/run, then reads outcome, revision and the event page from one SQL snapshot. It releases the snapshot before an optional wait, and takes a fresh authenticated snapshot on wakeup. It never waits while holding a transaction or actor lock. One response is at most1MiB on this standalone method using the existing core frame ceiling; attachment/host traffic retains the stricter64KiB ceiling. Return only the bounded prefix fitting that response, with NextSequence equal to the last returned event; if none fits because an individual supported historical event exceeds the frame limit, return capability_unsupported with the safe reason event_too_large and do not advance the cursor. More means committed events remain after that cursor. Empty successful page retains the input cursor; terminal outcome is separate and may be returned with an empty page.

Events retain their original journal.Event fields/schema_version/payload and only the named run's committed sequence. The decoder selects the existing v1 event contract or v2contract.Envelope according to schema_version; unknown critical/newer versions fail capability_unsupported, malformed/gapped events fail invalid_contract, with no cursor advance. These ordinary operator events are not the host projection: clients apply the existing supervisor.IsStreamEvent output allowlist and current renderer sanitization; internal service.adopted is consumed for continuity but is not emitted as an undocumented JSONL stream event. No event is ever forwarded to hosts. A supported internal event may advance the scanned cursor after the caller has processed its control meaning; the accessible next-after trailer is the last actually consumed sequence, never a skipped unread page. Unknown/foreign run is permission_denied or ownership_unresolved per canonical visibility policy; future cursor/generation is revision_conflict; history missing under an otherwise valid cursor is capability_unsupported with history_unavailable, never a silent jump. Database/transport timeout is persistence_unavailable and does not advance a cursor. Retention is unchanged by this spec.

Each Observe/Operation call has an end-to-end2second timeout including a maximum1second long poll; cancel affects only that read. Allow at most one outstanding observation per client/run connection, no unbounded push queue, and back off empty polls to the existing500ms cadence. CLI accessible history retains its1000-event per-invocation bound by paging at most128 at a time; only consumed events determine its trailer. TUI clients maintain a bounded latest-state cache and may coalesce display refreshes but cannot skip event processing required for an outcome. Native stdout/spool ingestion never waits for observation clients. Frame boundaries, snapshots, cursor continuity, mixed historical/new event versions and each typed failure are owned by the observation producer task before the public client tasks.

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

ADR 0021 records the existing D1–D8 composition decisions. Task 22 will draft a separate ADR for the unresolved admission-binding and observer-lifecycle design; no ADR 0022 is claimed to exist or be accepted yet.

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

Accepted ADR `docs/decisions/0021-service-attachment-composition.md` records D1–D8. Task 1's exact-revision architecture and code review and delegated acceptance are recorded in PR #501; this acceptance does not establish runtime or platform qualification.

## 10. Honesty register

Until Task 22 resolves the precise Task 8→9 contract and Task 9 implements it, service start is unsupported and Task 4 alone does not establish AC-6.4.


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
