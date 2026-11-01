# supervisor-attachment-control — Hand-off

> One section per task. Each task replaces its own `<!-- pending -->` line in its pull request with what
> dependent tasks need: what it produced (the API and files as shipped), what a later task must
> know, and any deviation that changes a later task's inputs. `/run-spec` puts the sections of a
> task's dependencies into that task's dispatch prompt. Open questions and research stay in
> `scratchpad.md`.

## Task 1 — Review the service composition decision

- **Produces**: Accepted ADR 0021 and its decision-index entry, recorded in PR #501. The source proposal is revision `e50faa8`, original-file SHA-256 `b30067e53dadaddbc416f8501ed67ea127ff8b02f7f6591be0b86fd49ca53393`; its Decision section is unchanged.
- **Review**: Independent architecture/code review returned Ready on that exact proposal, and coordinator review of author output `5386aff` returned Ready with no blocking findings. The coordinator separately accepted under the operator's explicit day-long shipping delegation; no personal operator content review, product signature or formal grant is claimed.
- **For dependents**: Preserve ordinary production dispatch, ADR 0014 singleton and ADR 0016 root-wide migration, immutable launch generation, schema2 adoption, authority-before-replay and immutable acknowledgment/separate reconciled result. Enrollment-only and concurrent legacy/service variants remain rejected.
- **Evidence boundary**: ADR acceptance supplies a reviewed design, not runtime/native/billing/host/platform qualification. Keep all task-specific tests and cross-spec lifecycle dependencies; no public attach-ready claim follows from this docs task.
- **Deviations**: Required task records follow WORKFLOW's pre-merge completion-entry convention; the task PR must merge before dependent implementation consumes its acceptance.

## Task 2 — Define typed service and attachment protocols

- **Produces**: PR #506, source `41813fe`: the exact RunClient and AttachmentClient interfaces/constructors, standalone schema2 request/result unions, typed grant/projection/stop records, strict Decode/DecodeParams and DialExisting. Runtime producers consume this contract after merge; this task registers no new service handlers.
- **Admission**: RunSpec inventories all serializable admission flags, requires absolute repository/task paths and task binding, excludes callbacks/environment and preserves nil versus explicit-zero ceiling counters. Preparation/challenge/start bindings and standalone generation stay distinct from attachment authority.
- **Codec boundary**: Reject duplicate/unknown/case-alias keys, malformed/null records and incompatible result arms. Complete attachment frames are at most 64 KiB; core standalone frames retain 1 MiB, depth64 and reference limits. Declared value and pointer records validate identically, including size and mandatory identity checks. Use DecodeParams/Decode for handler data; the older v2contract.Decode alone does not reject duplicates.
- **Observation**: Requests default to64 events, cap128 and wait at most1000ms. Clients reject mixed-run/gapped pages, wrong generation/cursors and unknown future schemas without releasing an advanced cursor. Historical v1 and canonical v2 envelopes remain separately validated; existing open event Type vocabulary is preserved rather than inventing an allowlist. Task10 owns canonical snapshot/history/control-meaning production.
- **Stop reasons**: StopResult.Reason is empty when no reason, otherwise stop_unconfirmed or a valid existing v2contract.Code. Later stop producers choose a relevant subset; further meaningful reason additions require an explicit reviewed vocabulary change. Native error text is not a reason code.
- **Clients and transport**: Typed calls share a two-second deadline and response-identity checks. DialExisting uses one budget from entry through OS DialContext, peer/root checks and response; it never lazy-starts or recovers. Legacy Dial/Connect remain separate. Read cancellation affects the read; accepted operations remain service-owned. A client permits only one outstanding observation.
- **Review/evidence**: Independent full code/architecture, context-fix and pointer-fix reviews returned Ready. The coordinator independently reran exact acceptance plus pointer tests under race (1.312s); final full Go race gate passed (420.5s), lint had zero issues and cross-vet passed for Windowsamd64/Darwinarm64. These establish protocol/build evidence, not runtime/native/host qualification.
- **Deviations**: Five approved transport files/tests enforce the already-required total deadline. Own completion records follow the shared pre-merge convention; downstream tasks still wait for this PR's actual merge. Persistence, admission/dispatch, operation/observation handlers and scoped stop producers remain Tasks3 and8–15 before public consumers16–19.


## Task 3 — Persist authority, revision and durable operation state

- **Produces**: PR #507, source `4379c3e`, integrated source `7b68070`: additive schema 9 with `attachment_grants`, `run_control_revisions`, `attachment_projections`, `stop_outbox`, `stop_results`, `prepared_runs` and `dispatch_outbox`; `journal.RunRevision(ctx, tx, runID) (uint64, error)` reads the caller's transaction snapshot and returns `journal.ErrNotFound` for an unknown run. Missing transaction/canceled reads fail rather than fabricating a revision.
- **Revision inventory**: INSERT, unequal selected UPDATE and DELETE trigger maintenance covers runs, attempts, journal, candidates, verifications, verification_bindings, check_results, applies, run_assignments and stop_results. Indirect bindings resolve through attempts/verifications or the immutable stop outbox; both old and new run bindings advance on a valid rebind. New and preexisting runs initialize at revision 1. Run timestamps and attempt spool offsets are bookkeeping, so replay/deduplicated offset acknowledgment alone does not advance the selected revision. Existing immutable tables retain stronger UPDATE/DELETE refusals. Add any new projection dependency to both the trigger and test inventories.
- **Actual writer inventory**: `journal/projections.go` supplies InsertRun/SetRunState, InsertAttempt/SetAttemptState/SetAttemptProcesses/SetNativeSession, InsertCandidate and InsertVerification (including binding/check rows); `journal.storeTx` and raw journal INSERT sites in `control/recover.go`, `migrate/import.go` and `migrate/apply.go` share SQL trigger enforcement. `control.AssignHandler` writes run_assignments inside Execute's transaction. The applies table has no current production SQL writer; Task18 must retain its revision triggers. Stop-result producers are Tasks13/15, not a second journal ingestion path. SetSpoolOffset/AdvanceSpoolOffset are deliberately excluded bookkeeping.
- **Immutable identities**: All declared keys and hidden row aliases are protected against implicit replacement deletion, with foreign keys enabled and no recursive-trigger dependency. Besides ordinary primary keys, attempts has its run/task/number unique key, journal has event/producer-sequence/run-sequence keys and seq=rowid, and dispatch has unique preparation and attempt keys. Completed operation INSERT duplicates are ignored with RowsAffected0; UPDATE into a completed victim aborts. Fresh claim insertion/completion and nonconflicting source updates remain supported. Negative physical collisions fail closed; actual writers allocate positive rows and never depend on a negative auto-allocation sentinel bypass.
- **Stop persistence**: Outbox run/attempt/launch hash, worker PID/start time and accepted generation are immutable, with only delivery state mutable. Results start requested/revision1 and advance through unconfirmed to confirmed, or directly to confirmed, with no regression or confirmed rewrite. Confirmation requires a canonical attempt.stopped event for that exact run/attempt, boolean confirmed and an explicit empty unresolved-PID list. Task15 still owns authenticated ingestion, pinned launch reconciliation and meaningful reason selection; matching evidence may resolve coalesced operations, so event operation-ID equality is not required.
- **Preparation/dispatch**: The preparation store binds peer, generation, evidence digest, revision, closed RunSpec/challenge/confirmation JSON, readiness/consumption and expiry. Task8 owns typed validation and consumption rules; no caller environment or raw token column exists. Dispatch pins operation/preparation/peer/generation/admitted-evidence digest and exact run/attempt/launch hash; only delivery state changes. The current enum values and column names are in the migration; extend only through a reviewed additive migration when required.
- **Deferred producers**: This storage/API task registers no service handler. Task10's acceptance says TestRunObserveCursorSnapshot “proves outcome/revision/page share one snapshot”; Task12's TestProjectionCompleteRevision “covers candidate/verification/apply and assignment changes”; Task13's TestStopAtomicRevisionRace “changes run revision/generation after request preparation” and requires no stale stop effect. Those tasks wire the revision reader into observations, projections and stop preconditions. Tasks8/9 produce preparation/dispatch, Task11 grants and Tasks13–15 stop acceptance/delivery/reconciliation.
- **Compatibility and evidence**: Released migrations and legacy unknowns/results are preserved. Backup/preview schema mirrors, support evidence, current backup filename and version-pinning tests move with schema 9; the schema-8 inventory test stays isolated to its historical migration. Independent code/architecture review and coordinator integrated race acceptance returned Ready. Final full Go race gate passed in 274.2s, zero-issue lint and hygiene passed; all six markers were fresh before own records. Retained declared/physical alias guard-drop fixtures fail without the relevant guard and avoid foreign-key masking. First-pass delivery is false because review and schema-mirror corrections were required; no service/native/platform qualification is claimed.

## Task 4 — Extract lock-held lifecycle entrypoints

<!-- pending -->

Draft dependent constraint: Task 4's proposed owner is ownership evidence only; `RunOwned` accepts a mutable in-process decision and is not independently an admission boundary. No production run handler may invoke it until Task 9 supplies the reviewed committed admission binding. Task 4 fixture acceptance alone does not establish AC-6.4; bounded callback delivery and shutdown need runtime evidence. This note is a proposed amendment, not evidence that Task 4 has merged or passed review.

## Task 5 — Close root migration and legacy writer fences

<!-- pending -->

## Task 6 — Compose the production singleton runtime

<!-- pending -->

## Task 7 — Adopt the same worker under a new service generation

<!-- pending -->

## Task 8 — Prepare service-side admission and confirmations

<!-- pending -->

## Task 9 — Commit and execute durable ordinary dispatch

<!-- pending -->

## Task 10 — Serve typed operation status and bounded observation

<!-- pending -->

## Task 11 — Mint and revoke internal scoped attachment grants

<!-- pending -->

## Task 12 — Produce coherent safe projections

<!-- pending -->

## Task 13 — Accept atomic stop intents with immutable acknowledgments

<!-- pending -->

## Task 14 — Deliver stop outbox idempotently

<!-- pending -->

## Task 15 — Reconcile stop results and bound core responsiveness

<!-- pending -->

## Task 16 — Route public run through service admission

<!-- pending -->

## Task 17 — Route demo and accessible clients through service

<!-- pending -->

## Task 18 — Route trusted mutation clients to one actor

<!-- pending -->

## Task 19 — Wire launch TUI as a service client

<!-- pending -->

## Task 29 — Extract the fixed pure quota policy and storage producer

- **Produces**: `billing.EvaluateQuotaSchedule`, closed `journal.QuotaReservationInput`, `QuotaRefusal`, `ErrDuplicateQuotaHold`, `ReserveRunQuota` and `InsertRunQuotaHold` with the exact design §3.2 signatures. No Task4 owner interface is consumed.
- **Policy order**: The pure evaluator preserves the legacy evaluator's retry-ceiling precedence; the transaction producer, like legacy admission, clears a passed authoritative reset before evaluating the schedule, including after spent retries. Negative retry counts refuse without indexing; large int64 counts cannot wrap into a fresh schedule.
- **Storage**: ReserveRunQuota reads the current bucket and atomically clears/counts scheduling plus checks duplicates/inserts. Inputs are trusted internal clock/envelope data, not authority or a wire contract; Task30 must derive them from the verified binding and a fresh clock inside its transaction. Failed storage or later admission failure must propagate out of the enclosing transaction so all effects roll back.
- **Legacy seam**: Admission retains its custom evaluator and custom owner behavior. The ordinary RunID-owned path shares InsertRunQuotaHold; custom owners retain the legacy insert branch. Both paths keep unknown quantity and execution-plus-one-hour expiry; legacy duplicate errors preserve the checked-storage cause.
- **Dependency correction**: Existing billing→qualify→journal imports made journal→billing cyclic. The shared `internal/datum` label leaf removes that edge, with `qualify.DatumLabel` retained as a source-compatible alias and existing provenance names/values unchanged. Normalization behavior remains covered by its existing tests.
- **Evidence**: Named schedule/SQLite/authority acceptance tests include due/future reset, spent schedule, duplicate hold, exact row fields and rollback after insert/later failure. Always-admit policy, duplicate-check omission, fixed-policy bypass and invented-zero quantity mutants were each observed failing; restored code passes the targeted race suite.
- **For Task30**: The producer is intentionally low-level. Its acceptance line “TestServiceAdmissionAtomicStorage injects each admission and arming statement/projection failure and observes complete rollback” owns wiring these effects into service admission. No provider availability, permission grant, wire method or process execution is established here.

## Task 20 — Prove ordinary production composition end to end

<!-- pending -->

## Task 21 — Document production ownership and migration

<!-- pending -->


## Task 22 — Review the durable admission and delivery contract

- **Produces**: ADR 0022 and its index acceptance metadata in PR #541; exact independently reviewed/adopted technical proposal `d8975b35b342bc7f5be34c43bcdbcc39e64ecf52`, original-file SHA-256 `8a5ba89975044132af27362778529087f6b88a1b949c9441e0a4bce291ea8f2d`. No normative technical decision bytes change.
- **Follow-on spec**: Separately reviewed PR #543 actually merged as `d35073de4e496ac264f17c2833aff094d88a7c81`. Its merged requirements/design/tasks/scratchpad are the implementation contract, including producer Tasks23–31 and amended Tasks6–10; preserve all original task IDs and completed records.
- **Review/adoption**: Independent architecture and source review returned Ready on the exact proposal; maintainer continuation after its presentation authorized adoption of that content as the design direction. Configuration, billing and runtime/platform qualification remain separate gates. Closeout review/checks remain bound to the final PR #541 head.
- **For dependents**: Use the closed standard-library-only binding codec, exact retention and revision-aware client producers, opaque supervisor-owned plan/permit boundary, schema10 pending-intent/post-snapshot arming and bounded observer lifecycle from the merged follow-on contract. A digest or structurally valid record alone grants no authority.
- **Gates**: PR #541 must merge before dependent work consumes this acceptance. Task4 PR #527 at `3b3c5f5e80a3e0d87a944b43a0fde0881177e36a` remains unmerged and independently unresolved; runtime Tasks6–9 keep every exact dependency. Re-ground candidate/source drift before implementation.
- **Evidence boundary**: Design acceptance and spec merge are documentation evidence, not implemented runtime/native/billing/host/platform qualification, successful MYTHHELM-owned execution or a native configuration grant. Prior blocked/time-limited Task22 attempts retain their original outcomes; this is a separate measured closeout attempt.
- **Deviations**: Required own-task records accompany the two listed docs paths; no follow-on normative spec edit or other task completion is included in this PR.

## Task 28 — Describe the existing versioned backup naming contract

- **Produces**: PR #549 describes `mythhelm.db.bak-migration-v<N>` and its `.json` sidecar in the migration guide and limitations register.
- **For dependents**: Backup reads `PRAGMA user_version` when copying and records that actual value in `schema_version`; preview independently names its planned target using the binary's supported schema. Preserve this distinction when Task25 updates the schema/version mirrors.
- **Scope**: Only the guide's migration section and one limitations bullet change; unrelated TUI examples are byte-for-byte preserved. The `docs:user-guide:migration` advisory lane coordinates this section; later writers still need independent diff checks.
- **Evidence boundary**: No migration apply was run and no service/native/platform qualification is added. Existing synthetic backup tests pass; runtime ownership restrictions remain in the guide and limitations register.

## Task 27 — Produce nonblocking observer wakeups and typed hints

- **Produces**: PR #551, source `ed4e085`: `supervisor.NewObserverSink`, `ObserverSink.Committed/Hint/RequestClose/OpenReader`, `ObserverReader.Wakeups/Done/DrainHints/Close`, the three informational `NoticeCode` constants and copied `HintBatch` values, matching design §3.2. One shared lifetime supports copied sink/reader handles, one reader lease, a capacity-one wakeup and a fixed sixteen-code FIFO ring with cumulative saturating display counters.
- **For dependents**: `Committed` retains neither run identity nor sequence. Wakeups coalesce and never close; select with `Done` and read canonical journal pages for evidence. Unknown/authority-bearing hints increment `Rejected`, overflow replaces oldest and increments `Dropped`, and neither counter is canonical evidence. Sink closure permits draining remaining hints; reader closure invalidates that lease and permits reopening only while the sink remains live.
- **Errors**: Invalid handles/bounds return `*v2contract.ControlError` with `invalid_contract`; duplicate lease returns `revision_conflict`/`observer_reader_active`, and a closed sink returns `dependency_stale`/`observer_closed`. Constant detail uses `mythhelm.observer/reason`; errors contain no input/native data.
- **Deferred runtime wiring**: Task 6 owns `internal/service/observer_adapter.go` and source-complete durable notice classification. Its acceptance requires: “`TestObserverDeliveryNeverWaitsForClient` stalls a view writer for ten seconds while synthetic canonical commits and status/stop-intent rejection acknowledgements progress through the runtime within the existing two-second request budget; replacing the adapter with a blocking callback fails.” Task 6's `TestObserverCloseOrdering` additionally checks callback return before closing the owner outside callbacks. This primitive does not supply that adapter or qualify ordinary native ingestion.
- **Evidence boundary**: Tests exercise synthetic bounded delivery and exported consumer lifecycle; no service/native/billing/platform qualification follows. Public observation handlers must not acquire or drain this actor-adapter lease.
- **Deviations**: None in the API or primitive behavior. Own handoff/scratchpad records accompany the three listed Go files; the pre-merge completion entry does not claim PR #551 merged.
