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

- **Produces**: PR #527, signed source `00fe03b`: `control.AcquireInstanceLease`, lease generation/root/hold/close methods, and `supervisor.AcquireServiceOwner`, `RunOwned`, `RecoverOwned` and `ApplyOwned`. The service entrypoints reuse the existing lock-held pipeline/recovery/apply implementations without re-acquiring their run lock; legacy entrypoints retain their guards.
- **Lifetime**: Copies of either capability share one lifetime. Lease close refuses new holds and waits for active holds; service-owner close refuses new entries, waits for active work and releases both the run lock and lease hold before concurrent closers return. Zero, closed, wrong-root, wrong-generation and wrong-run capabilities refuse ownership.
- **For dependents**: Acquire the instance lease before the service owner, use an adopted root and pass the matching run to owned entrypoints. Each owner admits one ingestion path; owned entrypoints refuse terminal detach/interrupt hooks. Tasks 5–7 still own migration/runtime/adoption integration; these internal APIs do not establish a public attachment service.
- **Evidence**: Independent production and repair reviews returned Ready. The three named Task 4 acceptance tests pass under the race detector; the concurrent-close test also passes 20 race repetitions. The final canonical formatting, vet, full race, module-tidy, lint (zero issues) and public-hygiene gates pass.
- **Review correction**: Concurrent post-close test probes are serialized so they cannot contend with one another; concurrent close and active-entry barriers remain intact. A later explicit bound and terminating return make the positive generation assertion visible to the lint analyzer; this was a lint finding, not a demonstrated runtime overflow. The original candidate and later repairs are distinct measured attempts, so this is not first-pass delivery.
- **Completion boundary**: Own records use WORKFLOW's required pre-merge completion convention. The task PR remains pending until its actual merge; dependent implementation must wait for that merge. No native, host, platform or packaged-process qualification is claimed.

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

## Task 20 — Prove ordinary production composition end to end

<!-- pending -->

## Task 21 — Document production ownership and migration

<!-- pending -->
