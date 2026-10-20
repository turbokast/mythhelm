## Supervisor attachment control — Tasks

### Dependencies

- Epic: `specs/*/herdr-embedded-client/plan.md`. Prerequisites: reviewed ADR 0021; completed supervisor-service/migration/stop-recover and protected-acceptance building blocks. Ownership extraction, migration/runtime and public-client tasks wait for the relevant merged `specs/*/tui-observatory-view/` Tasks4–9; isolated protocol/persistence tasks may proceed independently. Task7 serializes with Windows worker-boundary tests. Admission and worker adoption may proceed independently after runtime if file lists remain disjoint; scoped-read producers may progress alongside admission. Public stop consumers follow completed stop persistence/delivery/reconciliation producers. Core Tasks16–19 additionally wait for MH-23 Task18 (shared run/demo/TUI launch branches); Task21 waits for MH-23 Task19 and docs-site-restyle Task11 (shared user-guide). Each re-grounds the merged contract before editing; unrelated core tasks remain independently dispatchable.
- Tasks sharing a Files path serialize. No task may consume proposed/unmerged interfaces; re-ground exact dependency contracts before starting. All tasks require independent code review; persistence/process/public interfaces also require architecture review.
- **Gates for every task**: format changed Go with gofmt first (go mod tidy when needed), then `scripts/harness/gate.sh go` for Go tasks, `scripts/harness/gate.sh harness` for harness changes and `scripts/ci/check-public-hygiene.sh` for all changes; spec bookkeeping also runs `scripts/ci/lint-agent-harness.sh --only specs`. Docs tasks run public hygiene and applicable harness lint. Files existing or an agent reporting success is not completion.
- **Completion convention**: append ✅ COMPLETED to the task heading only after merged evidence; add Status (PR), Implementation (SHAs), Spec deviations and Files modified; retain original fields and update scratchpad discoveries.
- **Commits**: signed off with named paths; each implementation task uses an isolated worktree and PR. Decision records are reviewed before dependent implementation; never invent a maintainer/product signature.

## Implementation Tasks

### Task 1 — Review the service composition decision ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: complex
- **Depends on**: None
- **Change**: Finalize the proposed ADR for the exact production ownership, admission, snapshot and operation contracts; record actual architecture/maintainer review before code tasks depend on it.
- **Files**:
  - `docs/decisions/0021-service-attachment-composition.md`
  - `docs/decisions/README.md`
- **Produces**: Reviewed ADR 0021; no change to ADR 0014 singleton topology or ADR 0016 root migration.
- **Acceptance**:
  - The ADR Decision section includes ordinary dispatch, root migration, immutable launch generation and schema2 service adoption, authority-before-replay and ack/result separation; an enrollment-only or concurrent-legacy variant is rejected by architecture review.
  - Review evidence names the concrete accepted revision; status remains proposed without actual acceptance.
- **Test plan**: Synthetic fixtures and deterministic barriers for the named cases; pair negative assertions with a deliberately violating fixture. Public process behavior uses the packaged binary in the integration task, not helper-only evidence.
- **Invariants touched**: I03/I06/I12/I18/I23 (v2 §§3/5/6: no new owner or weakened recovery).

- **Status**: ✅ Completed — recorded exact-revision ADR 0021 acceptance under delegated maintainer authority, with independent architecture and code review; PR #501.
- **Implementation**: Commit `5386aff` binds acceptance to proposal `e50faa8` and independently confirmed original-file SHA-256 `b30067e53dadaddbc416f8501ed67ea127ff8b02f7f6591be0b86fd49ca53393`; Decision text is unchanged. Independent architecture/code review and the coordinator's separate output review returned Ready without blocking findings. Lifecycle PR #498 merged as `b2b46e5` before task bookkeeping.
- **Spec deviations**: Own completion, handoff and scratchpad paths record required lifecycle evidence beyond the docs Files list. The pre-merge completion entry follows WORKFLOW and task-completion, which take precedence over this spec's heading-after-merge wording; the marker does not claim that PR #501 has merged. No contract deviation.
- **Files modified**: `docs/decisions/0021-service-attachment-composition.md`, `docs/decisions/README.md`, `specs/in-progress/supervisor-attachment-control/tasks.md`, `specs/in-progress/supervisor-attachment-control/handoff.md`, `specs/in-progress/supervisor-attachment-control/scratchpad.md`.

### Task 2 — Define typed service and attachment protocols ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: complex
- **Depends on**: Task 1
- **Change**: Add closed typed run and attachment wire contracts and bounded codecs before any consumer.
- **Files**:
  - `internal/control/run_contract.go`
  - `internal/control/attachment.go`
  - `internal/control/attachment_contract_test.go`
  - `internal/control/frame.go`
  - `internal/control/client.go`
  - `internal/control/frame_test.go`
  - `internal/control/run_contract_test.go`
- **Produces**: All control types, NewRunClient(conn Conn) RunClient, RunClient methods and exact operation/observation unions in design §3.1, plus AttachmentClient methods in §§3–4, plus DialExisting(ctx context.Context) (Conn, error) and NewAttachmentClient(conn Conn) AttachmentClient.
- **Acceptance**:
  - TestAttachmentContractBounds rejects unknown/duplicate fields, oversized frames and callback/environment maps; exact valid requests round-trip.
  - TestRunContractFlagCoverage maps every currently serializable admission.Request flag, rejects missing task binding and preserves nil versus explicit zero ceilings.
  - TestRunOperationObservationCodec round-trips each method/result union and mixed historical/v2 event page; rejects incompatible arms, invalid cursor/bounds, future critical schemas and undocumented wire keys.
- **Test plan**: Synthetic fixtures and deterministic barriers for the named cases; pair negative assertions with a deliberately violating fixture. Public process behavior uses the packaged binary in the integration task, not helper-only evidence.
- **Invariants touched**: I02/I03/I09/I19 (v2 §§4/7/8: typed scope and no secret transport).

- **Status**: ✅ Completed — implemented the closed typed run/attachment contracts, strict bounded codecs and scoped clients; PR #506.
- **Implementation**: Codex go-implementer, source commit `41813fe`. Exact schema2 standalone bodies preserve admission flags and nil/zero ceilings; operation unions and historical/v2 observation validation remain separate from safe attachment projections. Independent code/architecture review returned Ready after correcting the total dial budget; a prepublication author finding corrected pointer/null codec panics and received fresh independent Ready review. Final full race gate passed (420.5s), lint reported zero issues, and all six gate markers were fresh; Windowsamd64/Darwinarm64 control cross-vet exited 0. The coordinator independently reran the three acceptance and three pointer tests under race (1.312s).
- **Spec deviations**: Approved five extra paths (`internal/control/transport.go`, `internal/control/transport_unix.go`, `internal/control/transport_windows.go`, `internal/control/transport_unix_test.go`, `internal/control/transport_windows_test.go`) provide context-aware dialing for design §8's shared end-to-end deadline. Pointer/bounds/null/identity regressions strengthen the same strict codec contract without changing wire types. StopResult.Reason is empty, stop_unconfirmed or an existing canonical v2 code, an independently reviewed finite interpretation of the bounded enum requirement. Own task records are required lifecycle evidence; the pre-merge completion marker follows WORKFLOW/task-completion and does not claim PR #506 merged.
- **Red-first evidence**: Duplicate keys, missing task binding/invalid flags and incompatible result arms failed before strict decoding/validation. Contextless dialing failed cancellation; pointer roundtrips and top-level null failed with caught panic assertions before correction. A temporary skip-Validate overlay mutant failed the retained missing-identity fixture; restored pointer/acceptance tests passed under race. Initial lint findings were corrected with only one documented fixture-inventory false-positive suppression.
- **Files modified**: `internal/control/run_contract.go`, `internal/control/attachment.go`, `internal/control/attachment_contract_test.go`, `internal/control/frame.go`, `internal/control/client.go`, `internal/control/frame_test.go`, `internal/control/run_contract_test.go`, `internal/control/transport.go`, `internal/control/transport_unix.go`, `internal/control/transport_unix_test.go`, `internal/control/transport_windows.go`, `internal/control/transport_windows_test.go`, `specs/in-progress/supervisor-attachment-control/tasks.md`, `specs/in-progress/supervisor-attachment-control/handoff.md`, `specs/in-progress/supervisor-attachment-control/scratchpad.md`.

### Task 3 — Persist authority, revision and durable operation state

- **Domain/agent**: go-implementer
- **Budget**: complex
- **Depends on**: Task 2
- **Change**: Add additive tables and complete same-transaction SQL-trigger run revision maintenance without modifying immutable operation-result rules; enumerate selected-data write sites and connect them to the same revision transaction.
- **Files**:
  - `internal/journal/migrations/0009_attachments.sql`
  - `internal/journal/journal.go`
  - `internal/journal/attachment_revision.go`
  - `internal/journal/attachment_revision_test.go`
  - `internal/control/assign_revision_test.go`
- **Produces**: journal.RunRevision(ctx context.Context, tx *sql.Tx, runID string) (uint64, error); tables and keys in design §5. Revalidate next free migration number.
- **Acceptance**:
  - TestAttachmentMigration preserves legacy unknown fields/results and rolls back failed migration; newer-schema refusal leaves bytes unchanged.
  - TestRunRevisionCoversProjection exercises journal ingestion, assignment, evidence and stop-state changes and observes same-transaction revision increments; fixture omitting one selected dependency fails.
  - TestImmutableStopAck rejects UPDATE to a completed operations.result while separate stop_results advances.
- **Test plan**: Synthetic fixtures and deterministic barriers for the named cases; pair negative assertions with a deliberately violating fixture. Public process behavior uses the packaged binary in the integration task, not helper-only evidence.
- **Invariants touched**: I07/I09/I12/I23 (v2 §§5/6: canonical revision/evidence and immutable replay).

### Task 4 — Extract lock-held lifecycle entrypoints

- **Domain/agent**: go-implementer
- **Budget**: complex
- **Depends on**: Task 3
- **Change**: Extract pipeline/recovery/apply bodies under a proof-bearing ServiceOwner, preserving legacy guards and MH-23 deadline/detach semantics.
- **Files**:
  - `internal/supervisor/service_owner.go`
  - `internal/supervisor/service_owner_test.go`
  - `internal/supervisor/pipeline.go`
  - `internal/supervisor/recover.go`
  - `internal/supervisor/apply.go`
  - `internal/control/instance.go`
  - `internal/control/lock.go`
  - `internal/control/instance_test.go`
- **Produces**: AcquireInstanceLease and lease methods; AcquireServiceOwner, RunOwned, RecoverOwned, ApplyOwned with exact signatures in design §2.
- **Acceptance**:
  - TestServiceOwnerCapability refuses wrong root/run/generation/closed lease and legacy phase without worker or ledger effects.
  - TestOwnedRecoveryUsesOneIngestor reconciles and concludes under one lock; a fixture reacquiring the owned lock or running parallel recovery fails.
  - TestLegacyDetachContractPreserved retains the merged MH-23 worker deadline and stop-less legacy detach behavior.
- **Test plan**: Synthetic fixtures and deterministic barriers for the named cases; pair negative assertions with a deliberately violating fixture. Public process behavior uses the packaged binary in the integration task, not helper-only evidence.
- **Invariants touched**: I05/I06/I12/I18/I21 (v2 §§3/6: exclusive owner and bounded worker).

### Task 5 — Close root migration and legacy writer fences

- **Domain/agent**: go-implementer
- **Budget**: complex
- **Depends on**: Task 4
- **Change**: Keep root-wide exclusive migration and initialize fresh roots through it; reject existing unadopted roots for service mutation and audit resumable phase transitions.
- **Files**:
  - `internal/migrate/apply.go`
  - `internal/migrate/drain.go`
  - `internal/migrate/service_transition.go`
  - `internal/migrate/service_transition_test.go`
  - `internal/supervisor/service_owner.go`
  - `internal/cli/migrate.go`
  - `internal/cli/migrate_test.go`
- **Produces**: migrate.PrepareServiceRoot(ctx context.Context, stateDir string) error; only empty roots auto-initialize, nonempty roots return explicit migration-required refusal.
- **Acceptance**:
  - TestServiceRootMigration adopts verified live worker identity after legacy owner release without launch/stop and excludes new legacy admissions.
  - TestMigrationHeldOwnerRefuses leaves phase/backup/identity unchanged on timeout; no per-run partial adoption.
  - TestImportedResumeKeepsFence proves imported→adopted cannot race a legacy/service writer; changed-root/live-instance conflict refuses.
- **Test plan**: Synthetic fixtures and deterministic barriers for the named cases; pair negative assertions with a deliberately violating fixture. Public process behavior uses the packaged binary in the integration task, not helper-only evidence.
- **Invariants touched**: I05/I12/I18/I23 (v2 §5.2: one root authority and immutable import provenance).

### Task 6 — Compose the production singleton runtime

- **Domain/agent**: go-implementer
- **Budget**: complex
- **Depends on**: Task 5
- **Change**: Build the singleton composition shell and own its root lifetime context without introducing a control→supervisor import cycle; new methods return explicit unsupported until their later producer tasks land.
- **Files**:
  - `internal/service/runtime.go`
  - `internal/service/runtime_test.go`
  - `internal/control/client.go`
  - `internal/control/server.go`
  - `internal/cli/supervisor.go`
  - `internal/cli/supervisor_test.go`
- **Produces**: service.Run(ctx context.Context, stateDir string) error; production SupervisorMain enters this runtime shell. This task does not claim full dispatch/attachment or worker-ingestion wiring; Task20 proves those after their producers.
- **Acceptance**:
  - TestServiceEntrypointShell exercises the real hidden-supervisor entry, root lock and authenticated status transport; root/status reflect the held singleton. Calling not-yet-implemented run.start or attachment.open returns capability_unsupported and writes no ledger/native state.
  - TestServiceRootContextSurvivesClientExit disconnects the status client, reconnects and observes the same service PID/generation; no native worker is launched or claimed as ingestion evidence in this shell test.
  - TestServiceRootConflict creates a conflicting root and observes refusal, no second listener/writer and zero native launches.
- **Test plan**: Synthetic fixtures and deterministic barriers for the named cases; pair negative assertions with a deliberately violating fixture. Public process behavior uses the packaged binary in the integration task, not helper-only evidence.
- **Invariants touched**: I03/I06/I18/I23 (v2 §3: one authenticated durable owner).

### Task 7 — Adopt the same worker under a new service generation

- **Domain/agent**: go-implementer
- **Budget**: complex
- **Depends on**: Task 6
- **Change**: Reconcile the exact existing worker and record service adoption before enabling run controls; keep worker launch generation/deadline immutable and completed-episode wait separate from authority.
- **Files**:
  - `internal/control/adoption.go`
  - `internal/control/adoption_test.go`
  - `internal/service/reconcile.go`
  - `internal/service/reconcile_test.go`
  - `internal/workers/envelope_test.go`
  - `internal/workers/worker_test.go`
  - `internal/journal/canonical_tx.go`
  - `internal/journal/canonical_tx_test.go`
- **Produces**: control.ServiceAdoption and RecordServiceAdoption(ctx context.Context, tx *sql.Tx, adoption ServiceAdoption) error, plus DecodeServiceAdoption(event journal.Event) (ServiceAdoption, error) and journal.AppendCanonicalTx(ctx context.Context, tx *sql.Tx, envelope v2contract.Envelope, project func(*sql.Tx) error) error; exact schema2 fields/evidence and historical decode separation in design §7.
- **Acceptance**:
  - `TestServiceGenerationAdoption` records new control generation with unchanged launch generation/identity/deadline and one attempt; PID reuse, foreign launch, corrupt identity and unsupported spool refuse.
  - `TestAdoptionCrashDoesNotRelaunch` kills service before/after reconciliation commit and observes one continuing worker, no replacement and retained durable operations.
  - `TestCompletedWorkerWaitGrantsNothing` releases the already-spooled wait using fresh/changed-generation presence and observes zero launch, token mint, effect approval or deadline extension; CheckEnvelope still rejects an authority-bearing generation mismatch.
  - `TestZeroGenerationNeedsMigration` permits legacy observation only after explicit root migration evidence; a fixture trusting heartbeat/file contents alone fails.
  - TestCanonicalAdoptionV2Transaction rejects envelope/payload1 and future critical schema, refuses pre-adoption phase, preserves v1 historical bytes through migration, and commits v2 event plus assignment together; projection failure rolls back both and duplicate event replay reapplies neither.
- **Test plan**: Instrumented fake worker, deterministic commit barriers and a violating heartbeat-as-authority fixture; no native account.
- **Invariants touched**: I03/I06/I12/I18/I21 (v2 §§3/6/8: same-launch observation never grants a new episode).

### Task 8 — Prepare service-side admission and confirmations

- **Domain/agent**: go-implementer
- **Budget**: complex
- **Depends on**: Task 6
- **Change**: Refactor admission into bound preparation/challenges while retaining route, native trust and billing checks; construct environment only inside the service.
- **Files**:
  - `internal/admission/prepare.go`
  - `internal/admission/prepare_test.go`
  - `internal/admission/admission.go`
  - `internal/service/admission.go`
  - `internal/service/admission_test.go`
- **Produces**: run.prepare/run.confirm closed wire results PreparedRun and RunConfirmation from design §3; service-owned preparation store.
- **Acceptance**:
  - TestServiceAdmissionBinding invalidates task/config/repository/native-evidence drift between preparation and confirmation/start; native launch count stays zero.
  - TestServiceAdmissionAuthority rejects host/attachment token, foreign challenge and changed confirmation binding; noninteractive missing decisions retain actionable refusal.
  - TestServiceEnvironmentIsolation proves credential/PATH sentinels from caller maps are rejected and native policies remain pinned without serializing callbacks or full environment.
- **Test plan**: Synthetic fixtures and deterministic barriers for the named cases; pair negative assertions with a deliberately violating fixture. Public process behavior uses the packaged binary in the integration task, not helper-only evidence.
- **Invariants touched**: I02/I03/I04/I15/I19 (v2 §§6/7/8: admission and route authority precede launch).

### Task 9 — Commit and execute durable ordinary dispatch

- **Domain/agent**: go-implementer
- **Budget**: complex
- **Depends on**: Task 7, Task 8
- **Change**: Commit admission/assignment/dispatch intent once and run the extracted pipeline from the service actor; reconcile crash windows before spawn.
- **Files**:
  - `internal/service/dispatch.go`
  - `internal/service/dispatch_test.go`
  - `internal/service/runtime.go`
  - `internal/supervisor/pipeline.go`
  - `internal/supervisor/dispatch_test.go`
- **Produces**: run.start(StartRun) returns immutable RunAccepted; run.operation returns the persisted accepted identity/outcome under standalone authorization.
- **Acceptance**:
  - TestDispatchDuplicateLaunchOnce replays concurrent start operations and observes one run, attempt, launch and assignment.
  - TestDispatchCrashWindows covers before commit, after commit before spawn, after spawn before acknowledgment and after worker identity; only proven unconsumed original launch proceeds, ambiguity quarantines.
  - TestOrdinaryServicePipeline concludes candidate/verification/receipt through existing protected semantics; native completion alone remains candidate.
- **Test plan**: Synthetic fixtures and deterministic barriers for the named cases; pair negative assertions with a deliberately violating fixture. Public process behavior uses the packaged binary in the integration task, not helper-only evidence.
- **Invariants touched**: I02/I06/I07/I12/I18/I23 (v2 §§3/6/11: durable one-launch dispatch).

### Task 10 — Serve typed operation status and bounded observation

- **Domain/agent**: go-implementer
- **Budget**: complex
- **Depends on**: Task 9
- **Change**: Implement the exact standalone read-only operation lookup and paged observation producer before ordinary CLI/accessible/TUI consumers; keep authenticated state and cursor selection coherent and independent of client liveness.
- **Files**:
  - `internal/service/observation.go`
  - `internal/service/observation_test.go`
  - `internal/control/run_client.go`
  - `internal/control/run_client_test.go`
  - `internal/service/runtime.go`
  - `internal/journal/observation.go`
  - `internal/journal/observation_test.go`
- **Produces**: NewRunClient(conn Conn) RunClient; run.operation(OperationQuery) returns OperationStatus and run.observe(ObserveRun) returns RunObservation, exact types/union/cursor/error rules in design §3.1. Observer remains read-only when later recover/apply/stop handlers populate their result arms.
- **Acceptance**:
  - TestRunOperationQuery authenticates run/root before lookup, retrieves the same accepted operation after a generation restart, and exposes typed running/completed/failed/unconfirmed outcomes without reexecution; foreign IDs disclose nothing.
  - TestRunObserveCursorSnapshot proves outcome/revision/page share one snapshot, sequences remain ordered,128-event/1MiB bounds hold, empty pages retain cursor and byte-bounded pages return the exact last consumed cursor; oversized single event fails without skipping it.
  - TestRunObserveHistoricalAndV2 decodes historical1 and canonical2 separately, consumes service.adopted without emitting undocumented stream output, and rejects malformed/gapped/new critical schemas with no cursor advance.
  - TestRunObserveFailureAndCancellation covers future cursor, generation change, missing history, permission denial and DB/transport timeout using the exact typed errors; a canceled1second poll holds no transaction/actor lock and never cancels the run.
- **Test plan**: Deterministic competing-write barriers, real journal pages and independent valid/violating protocol fixtures. Packaged renderer/cursor evidence follows in consumer and end-to-end tasks.
- **Invariants touched**: I03/I06/I09/I12/I23 (v2 §§3/4/5/15: authenticated canonical observation never executes work).

### Task 11 — Mint and revoke internal scoped attachment grants

- **Domain/agent**: go-implementer
- **Budget**: complex
- **Depends on**: Task 6, Task 3
- **Change**: Produce transaction-scoped grant mint/validate/revoke primitives with no public Open response or projection dependency; isolate attachment authority from attempt tokens.
- **Files**:
  - `internal/control/attachment_auth.go`
  - `internal/control/attachment_auth_test.go`
- **Produces**: MintAttachmentGrant(ctx context.Context, tx *sql.Tx, peer Peer, runID string, generation int64) (AttachmentGrant, error); ValidateAttachmentGrant(ctx context.Context, tx *sql.Tx, peer Peer, grant AttachmentGrant, generation int64) error; RevokeAttachmentGrant(ctx context.Context, tx *sql.Tx, peer Peer, grant AttachmentGrant, generation int64) error. Their guard/preconditions/error contract is design §4; public Open/Read/Close are Task12.
- **Acceptance**:
  - TestAttachmentGrantScope mints in a held-generation transaction for one canonical run/repository and rejects forged peer/pane/repository identities before names/counts; no public OpenAttachment is fabricated.
  - TestAttachmentGrantMutationIsolation mints/validates/revokes grants and observes unchanged run/attempt/launch/owner; a primitive calling recovery fails the assertion.
  - TestAttachmentGrantRevocationGeneration rejects revoked/old-generation grants and permits a fresh internal mint for current authenticated scope without rotating attempt tokens; failed transaction persists no grant/hash.
- **Test plan**: Real ledger transactions plus deterministic instance-generation guards and safe synthetic identities; public response/projection composition is tested only after its Task12 producer exists.
- **Invariants touched**: I03/I06/I12/I17/I18/I23 (v2 §§8/14: scoped presentation authority only).

### Task 12 — Produce coherent safe projections

- **Domain/agent**: go-implementer
- **Budget**: complex
- **Depends on**: Task 11
- **Change**: Compose public Open/Read/Close with the internal grant primitives and bounded projection producer under one authority/generation snapshot; maintain complete revision dependencies.
- **Files**:
  - `internal/control/attachment_projection.go`
  - `internal/control/attachment_projection_test.go`
  - `internal/journal/attachment_revision.go`
  - `internal/journal/attachment_revision_test.go`
  - `internal/service/attachments.go`
  - `internal/service/attachments_test.go`
- **Produces**: attachment.open/read/close and AttachmentClient.Open/Read/Close: Open returns complete OpenAttachment{Grant, Projection} atomically, Read returns Projection and Close revokes the grant only; null/enum/sanitizer and snapshot rules are design §§4–5.
- **Acceptance**:
  - TestAttachmentOpenReadCloseComposition initial/repeated public open returns same canonical run/attempt with complete grant-bound projection and no launch/recover/owner effects; close revokes only that grant. Injected initial-projection failure rolls back mint/cache writes and emits neither token nor data.
  - TestProjectionSnapshotAuthorityRace uses deterministic barriers around authority/data selection and generation change: one valid snapshot or zero released bytes.
  - TestProjectionCompleteRevision covers candidate/verification/apply and assignment changes; omitted field dependency makes the fixture fail.
  - TestProjectionSanitization places nested secret/path/prompt/transcript/token and terminal-control sentinels in ledger/native payloads and asserts none reach projection; forged completion never sets verified.
  - TestProjectionStableRevision repeats identical content then changes one selected field; duplicate retains revision, change increments it monotonically.
- **Test plan**: Synthetic fixtures and deterministic barriers for the named cases; pair negative assertions with a deliberately violating fixture. Public process behavior uses the packaged binary in the integration task, not helper-only evidence.
- **Invariants touched**: I03/I07/I09/I17/I19/I23 (v2 §§5/8/14: complete scoped evidence).

### Task 13 — Accept atomic stop intents with immutable acknowledgments

- **Domain/agent**: go-implementer
- **Budget**: complex
- **Depends on**: Task 12
- **Change**: Add the short atomic stop executor and scope-validated replay/result query; commit durable outbox before returning within the deadline.
- **Files**:
  - `internal/control/attachment_stop.go`
  - `internal/control/attachment_stop_test.go`
  - `internal/control/ledger.go`
  - `internal/service/attachments.go`
- **Produces**: AttachmentClient.Stop and Operation; StopAck and StopResult in design §4. Existing ExecuteLong is not the acknowledgment path.
- **Acceptance**:
  - TestStopAtomicRevisionRace changes run revision/generation after request preparation and observes conflict with no outbox or delivery.
  - TestStopAckDurableBeforeReply delays worker confirmation beyond2seconds and sees committed operation/outbox before timely ack.
  - TestStopReplayAuthority revokes scope before duplicate lookup, rejects foreign/stale access without revealing result, replays same arguments byte-identically, and rejects changed arguments.
  - TestStopFreshScopeQueryAfterRestart gets the retained acknowledgment/result by operation ID after new authenticated generation without resubmitting stale authority.
- **Test plan**: Synthetic fixtures and deterministic barriers for the named cases; pair negative assertions with a deliberately violating fixture. Public process behavior uses the packaged binary in the integration task, not helper-only evidence.
- **Invariants touched**: I03/I06/I12/I17/I23 (v2 §§5/6/14: durable atomic authorized intent).

### Task 14 — Deliver stop outbox idempotently

- **Domain/agent**: go-implementer
- **Budget**: complex
- **Depends on**: Task 13, Task 7
- **Change**: Deliver committed requests through the sole actor outside transactions and explicitly coalesce the existing first-wins per-attempt stop file.
- **Files**:
  - `internal/service/stop_outbox.go`
  - `internal/service/stop_outbox_test.go`
  - `internal/workers/worker.go`
  - `internal/workers/stop_request_test.go`
- **Produces**: stop dispatcher follows design §6; workers.InspectStopRequest(dir string) (requestID string, exists bool, err error) for durable idempotency evidence.
- **Acceptance**:
  - TestStopDeliveryCrashWindows kills after file creation before outbox update and observes one physical stop effect after restart.
  - TestStopCoalescesDifferentOperations covers two operator IDs and deadline request, preserves original file/requester and resolves only against the same attempt identity.
  - TestStopIdentityMismatchUnconfirmed rejects PID reuse/foreign launch/corrupt existing request and does not deliver to a later attempt.
- **Test plan**: Synthetic fixtures and deterministic barriers for the named cases; pair negative assertions with a deliberately violating fixture. Public process behavior uses the packaged binary in the integration task, not helper-only evidence.
- **Invariants touched**: I06/I12/I18/I21 (v2 §6: bounded idempotent exact-attempt stop).

### Task 15 — Reconcile stop results and bound core responsiveness

- **Domain/agent**: go-implementer
- **Budget**: complex
- **Depends on**: Task 14
- **Change**: Advance operation results only from the actor ingestion evidence, retain unconfirmed failures and bound DB/request/queue waits independently of host/client lifetimes.
- **Files**:
  - `internal/service/stop_result.go`
  - `internal/service/stop_result_test.go`
  - `internal/service/runtime.go`
  - `internal/control/attachment_deadline.go`
  - `internal/control/attachment_deadline_test.go`
- **Produces**: Durable requested/unconfirmed/confirmed result evolution and NFR-2 bounded acknowledgment/status path.
- **Acceptance**:
  - TestStopResultMatchesEvidence leaves unexpected disappearance/unrelated stopped event unconfirmed and confirms only matching process-boundary evidence.
  - TestStopClientLossContinues disconnects before ack and after acceptance, restarts service and retrieves the same operation result without another effect.
  - TestCoreAttachmentDeadline stalls observer10seconds and forces DB contention; status and accept/reject ack each finish under2seconds, with64KiB frames and16 slots. A deliberately blocking actor fixture exceeds the bound and fails the shared assertion.
- **Test plan**: Synthetic fixtures and deterministic barriers for the named cases; pair negative assertions with a deliberately violating fixture. Public process behavior uses the packaged binary in the integration task, not helper-only evidence.
- **Invariants touched**: I06/I09/I12/I21 (v2 §§6/14: honest reconciliation and bounded optional work).

### Task 16 — Route public run through service admission

- **Domain/agent**: go-implementer
- **Budget**: complex
- **Depends on**: Task 9, Task 15, Task 10
- **External ordering**: Wait for merged `specs/*/tui-observatory-view/` Task18 after its lifecycle Tasks4–9; re-ground shared run/demo/TUI launch mode/theme/signal branches and preserve their contract.
- **Change**: Make ordinary run modes prepare/confirm/start through the singleton and observe accepted outcomes without owning the pipeline.
- **Files**:
  - `internal/cli/run.go`
  - `internal/cli/run_service.go`
  - `internal/cli/run_test.go`
  - `internal/cli/run_service_test.go`
  - `internal/cli/render.go`
- **Produces**: runService(ctx context.Context, req control.RunSpec, stdio Stdio) (control.RunAccepted, error); all production run modes share it.
- **Acceptance**:
  - TestPublicRunUsesService executes cli.Main for plain/JSONL modes and observes singleton assignment and one worker; local supervisor.Run fallback makes it fail.
  - TestRunConfirmationRoundTrip verifies exact bound prompts and no launch on declined/changed challenge.
  - TestRunLinearCompatibility compares the merged MH-23 baseline for existing result fields/exit statuses and tests new accepted-operation output explicitly.
- **Test plan**: Synthetic fixtures and deterministic barriers for the named cases; pair negative assertions with a deliberately violating fixture. Public process behavior uses the packaged binary in the integration task, not helper-only evidence.
- **Invariants touched**: I02/I06/I09/I18 (v2 §§3/15: ordinary public client retains truthful output).

### Task 17 — Route demo and accessible clients through service

- **Domain/agent**: go-implementer
- **Budget**: complex
- **Depends on**: Task 16, Task 10
- **External ordering**: Wait for merged `specs/*/tui-observatory-view/` Task18 after its lifecycle Tasks4–9; re-ground shared run/demo/TUI launch mode/theme/signal branches and preserve their contract.
- **Change**: Use the same admitted production service path for demo/accessible and retain temporary state for still-live detached work.
- **Files**:
  - `internal/cli/demo.go`
  - `internal/cli/demo_test.go`
  - `internal/cli/accessible.go`
  - `internal/cli/accessible_test.go`
  - `internal/cli/service_observer.go`
  - `internal/cli/service_observer_test.go`
- **Produces**: service observation through existing renderer/event output; no fixture-only service launcher.
- **Acceptance**:
  - TestDemoServiceDetachRetention observes code6 and retained repository/state while worker lives; completion cleanup occurs only after no live writer and review.
  - TestAccessibleServiceOutput preserves ordered bounded event output/cursor behavior without renderer backpressure owning the actor.
  - TestDemoNoCredentials uses ordinary public demo with local-scripted fake worker and no Herdr/native account.
- **Test plan**: Synthetic fixtures and deterministic barriers for the named cases; pair negative assertions with a deliberately violating fixture. Public process behavior uses the packaged binary in the integration task, not helper-only evidence.
- **Invariants touched**: I06/I09/I13/I18 (v2 §§3/15: client-only lifecycle, useful offline demo).

### Task 18 — Route trusted mutation clients to one actor

- **Domain/agent**: go-implementer
- **Budget**: complex
- **Depends on**: Task 9, Task 15, Task 10
- **External ordering**: Wait for merged `specs/*/tui-observatory-view/` Task18 after its lifecycle Tasks4–9; re-ground shared run/demo/TUI launch mode/theme/signal branches and preserve their contract.
- **Change**: Implement standalone recover/apply dispatch under the service owner and remove adopted-root local mutation fallbacks; preserve exact apply authority.
- **Files**:
  - `internal/service/actions.go`
  - `internal/service/actions_test.go`
  - `internal/cli/recover.go`
  - `internal/cli/apply.go`
  - `internal/cli/stop.go`
  - `internal/cli/service_actions_test.go`
  - `internal/supervisor/apply.go`
  - `internal/supervisor/recover.go`
- **Produces**: run.recover(RecoverRun) and run.apply(ApplyRun) return immutable ActionAccepted; run.operation returns exact OperationStatus/RunOutcome/ApplyOutcome in design §3.1. CLI stop consumes completed durable attachment.stop/operation producers.
- **Acceptance**:
  - TestAdoptedRootNoLegacyWriter exercises recover/apply/stop callers and rejects direct run lock mutation after adoption.
  - TestServiceApplyExactCandidate preserves destination/ref/candidate preconditions and foreign work; attachment authority cannot invoke apply.
  - TestServiceRecoverSameLaunch reconciles existing worker without new native launch; unapproved continuation remains blocked.
- **Test plan**: Synthetic fixtures and deterministic barriers for the named cases; pair negative assertions with a deliberately violating fixture. Public process behavior uses the packaged binary in the integration task, not helper-only evidence.
- **Invariants touched**: I03/I08/I12/I18 (v2 §§6/11/15: explicit effects under single owner).

### Task 19 — Wire launch TUI as a service client

- **Domain/agent**: go-implementer
- **Budget**: complex
- **Depends on**: Task 16, Task 17, Task 18, Task 10
- **External ordering**: Wait for merged `specs/*/tui-observatory-view/` Task18 after its lifecycle Tasks4–9; re-ground shared run/demo/TUI launch mode/theme/signal branches and preserve their contract.
- **Change**: Rewire the CLI TUI coordinator onto service subscriptions and preserve the merged MH-23 signal/intents contract without owner-detach propagation.
- **Files**:
  - `internal/cli/tui.go`
  - `internal/cli/tui_test.go`
  - `internal/cli/service_tui.go`
  - `internal/cli/service_tui_test.go`
- **Produces**: CLI implementations of the merged TUI Config callbacks; explicit stop intent goes to service, detach only closes the client subscription.
- **Acceptance**:
  - TestServiceTUIDetachKeepsOwner closes/tears down the TUI and observes unchanged worker/start/stop counters and continued ingestion; forwarding Hooks.Detach fails.
  - TestServiceTUIIntentRouting distinguishes deliberate stop, deliberate view detach, signals and client teardown using the merged mode table.
  - TestServiceTUIActions confines recover/apply to trusted standalone controls and never enables them through attachment grant.
- **Test plan**: Synthetic fixtures and deterministic barriers for the named cases; pair negative assertions with a deliberately violating fixture. Public process behavior uses the packaged binary in the integration task, not helper-only evidence.
- **Invariants touched**: I03/I06/I18 (v2 §§3/15: presentation cannot transfer owner).

### Task 20 — Prove ordinary production composition end to end

- **Domain/agent**: go-implementer
- **Budget**: complex
- **Depends on**: Task 15, Task 19
- **Change**: Exercise the packaged public run/supervisor/control paths with fake native process counters, root migration and service crash barriers.
- **Files**:
  - `tests/e2e/service_composition_test.go`
  - `tests/e2e/service_stop_test.go`
  - `tests/e2e/service_migration_test.go`
- **Produces**: Credential-free public production evidence consumed by existing-run-client, not an internal fixture launcher.
- **Acceptance**:
  - TestProductionServiceComposition now exercises the completed production registry through the packaged hidden-supervisor entry and ordinary public admission; leaving only the original six handlers or unsupported placeholders makes this test fail.
  - TestServiceActorSurvivesClientExit starts ordinary public fake work, disconnects every client and observes continued worker ingestion and immutable deadline enforcement; a client-owned actor context fails. This is production composition evidence deferred from Task6 until every producer is available.
  - TestE2EOrdinaryRunServiceOwner invokes public run, sees one assignment/attempt/launch and continued service ingestion after launching client dies.
  - TestE2EServiceStopDurability observes timely accepted status, delayed confirmation, lost ack/replay/query and restart without duplicate stop.
  - TestE2ELiveLegacyMigration retains verified live worker identity after safe owner release; held owner/root conflict refuses with zero new launch.
  - TestE2ENoLegacyMutationAfterAdoption exercises public run/demo/recover/apply/stop/TUI modes against the adopted root and detects a deliberately reintroduced local writer.
- **Test plan**: Synthetic fixtures and deterministic barriers for the named cases; pair negative assertions with a deliberately violating fixture. Public process behavior uses the packaged binary in the integration task, not helper-only evidence.
- **Invariants touched**: I02/I06/I12/I13/I18/I23 (v2 §§3/5/6: public production evidence).

### Task 21 — Document production ownership and migration

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 20
- **External ordering**: Wait for both `specs/*/docs-site-restyle/` Task11 and `specs/*/tui-observatory-view/` Task19 merge, then re-ground docs/user-guide.md labels/examples and preserve that convention.
- **Change**: Document the shipped standalone service journey, explicit migration and diagnostic limits with executed examples; do not claim Herdr or Windows qualification.
- **Files**:
  - `docs/user-guide.md`
  - `docs/limitations.md`
  - `docs/architecture.md`
- **Produces**: User guidance for ordinary service-owned run, explicit migration and durable stop/query; source examples from Task 20 packaged commands.
- **Acceptance**:
  - Documentation examples are regenerated from named Task 20 invocations and show accepted versus confirmed stop distinctly; no enrollment-only example substitutes for normal run.
  - Limitations identify live legacy-owner migration boundary, Windows unqualified rows, no whole MH-3/G11 and no host/native route upgrade.
- **Test plan**: Synthetic fixtures and deterministic barriers for the named cases; pair negative assertions with a deliberately violating fixture. Public process behavior uses the packaged binary in the integration task, not helper-only evidence.
- **Invariants touched**: I06/I09/I14 (v2 §§14/15/18: evidence-bounded claims).
