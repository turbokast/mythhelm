## Supervisor attachment control — Tasks

### Dependencies

- Epic: `specs/*/herdr-embedded-client/plan.md`. Prerequisites: reviewed ADR 0021; completed supervisor-service/migration/stop-recover and protected-acceptance building blocks. Ownership extraction, migration/runtime and public-client tasks wait for the relevant merged `specs/*/tui-observatory-view/` Tasks4–9; isolated protocol/persistence tasks may proceed independently. Task7 and Task26 serialize with Windows worker-boundary edits/tests. Appended producers23–31 retain published IDs; Task23/24/25/27/29/30/31 may run before Task4 only after Task22 completes and only without unmerged owner symbols. Task26 waits for Task4; Task28 is a separate docs-domain naming producer before the schema change. Tasks6–9 retain both ADR/follow-on merges and Task22 exact handoff gates. Admission and worker adoption may proceed independently after runtime if file lists remain disjoint; scoped-read producers may progress alongside admission. Public stop consumers follow completed stop persistence/delivery/reconciliation producers. Core Tasks16–19 additionally wait for MH-23 Task18 (shared run/demo/TUI launch branches); Task21 waits for MH-23 Task19 and docs-site-restyle Task11 (shared user-guide). Each re-grounds the merged contract before editing; unrelated core tasks remain independently dispatchable.
- Tasks sharing a Files path serialize. No task may consume proposed/unmerged interfaces; re-ground exact dependency contracts before starting. Task 22 may inspect a pinned unmerged Task 4 candidate to identify design gaps; inspection neither consumes that interface nor completes Task 4 or authorizes runtime work. All tasks require independent code review; persistence/process/public interfaces also require architecture review.
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

### Task 3 — Persist authority, revision and durable operation state ✅ COMPLETED

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

- **Status**: ✅ Completed — implemented additive attachment authority, canonical run revisions and separate durable stop state; PR #507.
- **Implementation**: Source `4379c3e` adds schema 9 and `journal.RunRevision` with same-transaction dependency triggers and immutable acknowledgment/outbox protection. Signed integration `7b68070` includes the verified main-branch Windows fix; reviewed task SQL and tests are unchanged.
- **Spec deviations**:
  - Approved schema-assertion scope: `internal/journal/acceptance_test.go` isolates the exact historical schema-8 inventory; `internal/journal/evaluator_test.go` and `internal/journal/qualification_test.go` retain introduced-schema floors and current-version checks; `internal/journal/journal_test.go` follows the current pragma version; `internal/journal/migrate_tables_test.go` pins schema 9.
  - Approved compatibility scope: `internal/migrate/backup.go` and `internal/migrate/preview/plan.go` move the required schema mirrors and ordered migration list together; `internal/migrate/SUPPORT.md` follows the renamed evidence test; `docs/user-guide.md` updates only the current backup filename.
  - Approved preview-test scope: `internal/cli/migrate_test.go` derives the exact ordered remaining schema-7 migration files, independently of the production preview list.
  - Review corrections consistent with design §5: cover INSERT/UPDATE replacement through every declared unique key and physical row alias; retain unreferenced-victim counterexamples so foreign keys cannot mask omitted guards. Completed-operation duplicate insertion is ignored with zero affected rows, preserving concurrent claim semantics and the entire immutable acknowledgment row; existing completed UPDATE/DELETE guards remain intact.
- **Files modified**: `docs/user-guide.md`, `internal/cli/migrate_test.go`, `internal/control/assign_revision_test.go`, `internal/journal/acceptance_test.go`, `internal/journal/attachment_revision.go`, `internal/journal/attachment_revision_test.go`, `internal/journal/evaluator_test.go`, `internal/journal/journal.go`, `internal/journal/journal_test.go`, `internal/journal/migrate_tables_test.go`, `internal/journal/migrations/0009_attachments.sql`, `internal/journal/qualification_test.go`, `internal/migrate/SUPPORT.md`, `internal/migrate/backup.go`, `internal/migrate/preview/plan.go`, `specs/in-progress/supervisor-attachment-control/tasks.md`, `specs/in-progress/supervisor-attachment-control/handoff.md`, `specs/in-progress/supervisor-attachment-control/scratchpad.md`.

### Task 22 — Review the durable admission and delivery contract

- **Domain/agent**: go-implementer
- **Budget**: complex
- **Depends on**: Task 3
- **Also inspects**: Task 4 PR #527 at `3b3c5f5e80a3e0d87a944b43a0fde0881177e36a`, an unmerged candidate only. Pin the inspected revision in the ADR and re-ground every relevant change before ADR acceptance; a changed candidate invalidates affected design evidence. Task 4 implementation and review remain independently required.
- **Change**: Resolve the admission-binding and callback delivery gaps as an architecture-reviewed ADR before runtime, admission or dispatch work resumes. Keep the Task 4 signature as a trusted internal primitive unless the review explicitly changes it.
- **Files**:
  - `docs/decisions/0022-service-admission-binding.md` (verify number is free on origin/main before drafting)
  - `docs/decisions/README.md`
- **Produces**: Reviewed ADR defining exact versioned non-secret durable binding fields, canonical serialization/hash, Task 8 producer and Task 9 consumer interfaces with failure cases, peer/generation/expiry/confirmation binding, recovery and one-use transaction rules, service-to-supervisor proof interface, and the bounded observer delivery and shutdown lifecycle. Name every required additive migration and implementation path. Secrets and full environments are excluded; reconstruct them using qualified service identity and revalidate evidence.
- **Acceptance**:
  - Architecture review checks a field-by-field inventory of every execution-relevant `admission.Decision` and `RunSpec` value against the representation, including no-checks, repo/revision, task, config/trust, route, billing/envelopes and launch inputs; each field is persisted, re-derived and revalidated, or explicitly rejected with rationale. A digest paired with independently mutable values is not a passing design.
  - The ADR traces a confirmed request through crash-before-commit, commit-before-spawn, ambiguous spawn and restart, showing one canonical run/attempt and fail-closed handling of absent, changed or non-reconstructable evidence. It describes integration with existing `runLocked` initial insertions and schema 9, and exact mutation/bypass controls for Tasks 8 and 9.
  - The ADR specifies the chosen observer delivery mechanism, overflow and evidence semantics, disconnected or stalled client handling, cancel/close ordering and resource bounds. After the ADR design receives independent architecture and maintainer review, a separate spec update defines the exact Task 6 and Task 7 implementation paths and executable acceptance tests, and receives independent review before it is merged. Task 22 is not complete until that follow-on spec update is merged and its exact revision is recorded in the Task 22 handoff. Independent architecture and maintainer review name the exact accepted ADR revision. Until both the ADR and follow-on spec update are reviewed and merged, Tasks 6–9 do not proceed. Task 22's own pull request remains limited to its listed docs files and its completion records; the spec update is a separate pull request.
- **Test plan**: Inspect the actual decision fields, schema and pipeline, trace both happy path and counterexample designs; no runtime or platform qualification is claimed by this documentation task.
- **Invariants touched**: I02/I03/I05/I06/I12/I18/I20/I21/I23 (v2 §§2/3/6/8: bound authority, one owner and durable recovery).

### Task 23 — Produce the closed canonical admission binding codec

- **Domain/agent**: go-implementer
- **Budget**: complex
- **Depends on**: Task 22
- **Change**: Add the standard-library-only BindingV1 DTO/codec with the exact ADR0022 field order, validation and source inventory; this leaf consumes no Task4 owner symbols and grants no authority.
- **Files**:
  - `internal/admissionbinding/binding.go`
  - `internal/admissionbinding/codec.go`
  - `internal/admissionbinding/codec_test.go`
- **Produces**: exported `var ErrInvalidContract = errors.New("admissionbinding: invalid contract")` and `var ErrCapabilityUnsupported = errors.New("admissionbinding: capability unsupported")` in codec.go; EncodeV1(BindingV1) ([]byte,error), DecodeV1([]byte) (BindingV1,error), DigestV1(BindingV1) ([32]byte,error), ValidateV1(BindingV1) error; exact DTOs/limits and per-field rules from ADR0022 at d8975b35b342bc7f5be34c43bcdbcc39e64ecf52 and design §3.2.
- **Acceptance**:
  - `TestBindingV1CanonicalInputs` uses independently written nonempty byte/hash goldens, all nested DTO fields, unset/zero/null/empty cases, ordered argv/tools/checks and sorted source sets. Duplicate/unknown/missing keys, malformed UTF-8, overflow/range/enum errors, depth/1MiB bounds, changed field order/domain separator and noncanonical re-encoding refuse; permissive codec mutants fail.
  - `TestBindingV1SentinelErrors` uses named malformed-supported-version, future-schema, unsupported-bound and valid fixtures across all four exported codec/validation APIs; each failure matches exactly its design §3.2 sentinel via errors.Is, including one/two `%w` wrappers, and rejects the opposite sentinel. `TestAdmissionBindingStandardLibraryOnly` checks actual package dependencies exclude all non-standard-library imports, including v2contract. Error-string matching, a bare fresh errors.New at each call, or a typed ControlError-only leaf mutant fails. No failed output is accepted as usable evidence.
  - `TestBindingV1SourceFieldInventory` enumerates actual main Decision, RunSpec and nested launch fields and assigns every one its exact persist/rederive/reject disposition. Adding a source field without a rule, dropping a mapping, or inconsistent outer/Decision.Environment/root/run/task/attempt identities fails.
  - `TestBindingV1HashSpellings` preserves validated raw prefixed/unprefixed/uppercase TaskSHA256, exact lowercase trust flags or absence and normalized derived hashes; NoChecks empty config and absent native evidence remain distinct. A global64hex validator mutant fails the valid controls.
- **Test plan**: No Task4 import, native account, file probing or live authority. Run codec goldens and one deliberately permissive mutant per parser/binding class; full Go gates and independent architecture review.
- **Invariants touched**: I02/I09/I20 (v2 §§2/6/8: closed immutable evidence and honest absence).

### Task 24 — Produce fail-closed retention policy validation

- **Domain/agent**: go-implementer
- **Budget**: complex
- **Depends on**: Task 23
- **Change**: Define versioned exact-field retention policy/approval records and pure validation before any service binding persistence; missing actual operator authority remains unavailable rather than being inferred.
- **Files**:
  - `internal/admissionbinding/retention.go`
  - `internal/admissionbinding/retention_test.go`
  - `internal/admission/retention.go`
  - `internal/admission/retention_test.go`
- **Produces**: exact structural RetentionPolicyV1/RetentionApprovalV1 declarations and validators, verified content-addressed RetentionPolicyContentV1/Rule declarations, strict canonical retention codecs, RetentionPolicyDigestV1, DeriveRetentionFieldsV1 and RetentionFieldSetSHA256 in design §3.2, plus admission aliases and CheckRetentionV1(binding admissionbinding.BindingV1, policy RetentionPolicyV1, approval *RetentionApprovalV1, now time.Time) error; fields and staging in design §3.2. Approval is a validated exact record, not a wildcard or caller boolean, and native subscription secrets/full environments are unconditionally excluded.
- **Acceptance**:
  - `TestRetentionUnclassifiedRefuses` covers each retained field class (Git email/name, task title/artifact, challenge, repository/config path, argv/tools/manifest/override), unknown classification, mismatched policy/purpose/root/peer/context/generation/digest/expiry and missing approval. Each rejects; an exact synthetic non-secret approval passes without writes or native launch.
  - `TestRetentionCannotApproveNativeSecrets` marks native-auth-store/transcript/credential-env/argv provenance forbidden even with affirmative generic approval; an allow-all classification mutant fails. Finite typed provenance cases do not claim universal secret recognition.
  - `TestRetentionLeafAndAdmissionErrorMapping` pairs malformed/future/wrapped structural leaf errors with exact errors.Is sentinels, then tests admission.CheckRetentionV1 maps each to CodeInvalidContract/CodeCapabilityUnsupported via errors.As. Structurally valid forbidden/unclassified/missing-approval controls remain CodePolicyIneligible. Require the exact safe constant Detail maps/slash-namespaced keys from design §3.2 and no writes/native effects; raw decoder/input/wrapped text and string-classification mutants fail.
  - `TestRetentionExactValueBinding` changes one approved field or executable input and refuses rather than redacting it into a different launch; display-only title redaction remains deterministic and version-bound.
  - `TestRetentionRecordsGrantNoAuthority` accepts structurally valid synthetic approval data without writes/launches and verifies existing closed RunSpec decoding rejects extra policy/approval wire fields. Task8/9 own the later authenticated public-handler launch controls; this leaf does not import an unmerged runtime.
  - `TestRetentionFieldSetIsInternalBindingCommitment` derives rows only from exact canonical binding plus full content-addressed policy, never new wire metadata. Tampered/missing policy content, rule/value/provenance/root/scope/challenge changes and copied approval refuse; a bare hash/imagined FieldSet wire field cannot approve anything. An independently written policy→binding→field-set order control proves no hash cycle.
- **Test plan**: Pure closed-record fixtures with no real credentials or granted live policy; independently mutate one approval binding/classification at a time. Task8 owns volatile-before-confirmation integration.
- **Invariants touched**: I02/I03/I19/I20 (v2 §§6/8: classification/authority before retention; no copied subscription secrets).

### Task 29 — Extract the fixed pure quota policy and storage producer

- **Domain/agent**: go-implementer
- **Budget**: complex
- **Depends on**: Task 22
- **Change**: Separate the current quota schedule and duplicate/insert storage from runtime admission before new journal transactions consume them; preserve the legacy evaluator seam and all existing valid-row behavior.
- **Files**:
  - `internal/billing/quota_schedule.go`
  - `internal/billing/quota_schedule_test.go`
  - `internal/journal/quota_reservation.go`
  - `internal/journal/quota_reservation_test.go`
  - `internal/journal/ledger_test.go`
  - `internal/admission/reserve.go`
  - `internal/admission/reserve_test.go`
  - `internal/supervisor/exhaustion.go`
  - `internal/supervisor/exhaustion_test.go`
- **Produces**: billing.EvaluateQuotaSchedule(exhaustedAt string,resetAt *string,retriesUsed int64,now time.Time) (admit bool,wait time.Duration,giveUp bool), plus closed journal.QuotaReservationInput and ReserveRunQuota/InsertRunQuotaHold signatures in design §3.2. The pure billing package imports no journal/admission/supervisor, and journal imports no runtime producer. This extraction consumes no Task4 owner symbol.
- **Acceptance**:
  - `TestFixedQuotaScheduleMatchesLegacy` covers authoritative reset before/after retry ceiling, unknown-reset1m/5m/15m, give-up after3 retries, malformed/negative/overflow schedule inputs and unchanged valid controls. A zero-wait/always-admit mutant fails.
  - `TestServiceQuotaMatchesLegacy` compares legacy production and fixed service storage for held/duplicate reservations, due/future reset, spent schedule, failed storage and full transaction rollback. Scope/owner/quantity/expiry remain exact, quantity unknown stays unknown, and the legacy custom evaluator test seam keeps its current behavior.
  - `TestQuotaStorageIsNotAuthority` proves that constructor/input data and a caller-computed success flag cannot bypass the fixed transaction schedule/duplicate rules. Task30 derives inputs from the confirmed binding and fresh clock; no new generic evaluator/projection callback is accepted.
- **Test plan**: Independent schedule fixtures and actual SQLite transaction rows, existing legacy tests plus explicit violating controls. These are low-level trusted storage APIs, not a new wire method or claim of provider availability.
- **Invariants touched**: I02/I09/I20/I21/I23 (v2 §§5/6/8: current fixed quota policy and honest one-hold effects).

### Task 27 — Produce nonblocking observer wakeups and typed hints

- **Domain/agent**: go-implementer
- **Budget**: complex
- **Depends on**: Task 22
- **Change**: Add the isolated supervisor ObserverSink primitive with fixed bounds and no owner/client-I/O dependency; Task6 supplies the runtime adapter and durable evidence classification.
- **Files**:
  - `internal/supervisor/observer.go`
  - `internal/supervisor/observer_test.go`
  - `internal/supervisor/observer_external_test.go`
- **Produces**: NewObserverSink() *ObserverSink; Committed(runID string,sequence int64), Hint(NoticeCode), RequestClose() with private shared lifetime state, capacity1 wakeup,16-code informational ring and saturating hints_dropped. exported OpenReader/ObserverReader.Wakeups/Done/DrainHints/Close and HintBatch with the exact single-reader, bounds, close/reopen and copy semantics in design §3.2; Closed NoticeCode values/consumption semantics are design §3.2/ADR0022; no event payload, authority or terminal outcome is stored in the hint ring.
- **Acceptance**:
  - `TestObserverPrimitiveNeverBlocks` stops consuming wakeups/hints, submits more than each bound and observes prompt return, exactly bounded storage and retained coalesced wakeup; replacing nonblocking send with blocking delivery fails.
  - `TestObserverHintOverflowIsNotEvidence` verifies oldest informational code replacement, saturating counter, idempotent close and no cursor/outcome advance; treating hints as canonical journal events fails.
  - `TestObserverPrimitiveImportsNoOwner` compiles without Task4 or service types and exercises copied handles sharing one close lifetime, concurrent close/hint/late wakeup and unknown/authority-bearing code refusal under race.
  - `TestObserverExternalReaderLifecycle` is in package supervisor_test and uses only exported constructors/methods: nonempty hint drain, duplicate-reader typed refusal, read-only wakeup/Done channels, copied batches, invalid drain limits, concurrent close/hint, post-close drain, idempotent reader close/reopen and sink close refusal. Requiring private-field access or dropping the consumer interface fails compilation/behavior.
- **Test plan**: Deterministic channels/barriers and bounded queues, no timing-only sleep or fake native claim. This leaf changes no pipeline callback until Task6.
- **Invariants touched**: I06/I09/I18/I21/I23 (v2 §§3/6/14: bounded display work and canonical evidence remain separate).

### Task 28 — Describe the existing versioned backup naming contract

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 22
- **Change**: Replace the current hardcoded v9 backup filename with the actual existing version-parameterized naming/preview-target contract before the additive migration changes the current version; do not claim schema10 or future runtime support already exists.
- **Files**:
  - `docs/user-guide.md`
  - `docs/limitations.md`
- **Produces**: Source-grounded guidance for `mythhelm.db.bak-migration-v<N>` plus sidecar and preview-selected target, with current service preparation/qualification limits. Task25 depends on this docs producer, so no mechanism update lands beside stale current backup text.
- **Acceptance**:
  - Inspect `internal/migrate/backup.go` and `internal/migrate/preview/plan.go` at the exact merged source: both format the versioned backup target. The current user-guide migration section has a nonempty matching naming example and explains that N is the actual schema recorded by Backup from PRAGMA user_version and in its sidecar, while preview reports its planned target; it does not hardcode a future/current9 filename. Historical examples remain labelled historical.
  - Public hygiene and docs checks pass; limitations retain unsupported service launch and unknown native/platform qualification. The text does not say schema10 bindings or a new live route are implemented.
  - Coordinate the migration-section reservation with any active MH23Task19 docs/user-guide edits using the shared docs:user-guide:migration lane and an explicit changed-section manifest. Preserve unrelated TUI examples byte-for-byte; re-ground this section on the actual merged base before edit/push/merge. If that section overlaps, serialize those edits only; do not wait for the entire unrelated TUI task to finish. Advisory lanes do not enforce arbitrary writers, so independent diff/review checks remain required.
- **Test plan**: Source/section-anchored naming comparison and existing docs checks; quote existing passing backup/preview test evidence instead of executing operator-only commands. Task21 serializes after this bounded naming update before broader guide edits.
- **Invariants touched**: I09/I14 (v2 §§14/18: accurate current migration and capability limits).

### Task 25 — Produce the additive binding schema and atomic version mirrors

- **Domain/agent**: go-implementer
- **Budget**: complex
- **Depends on**: Task 23, Task 28
- **Change**: Add ADR0022 binding/consumption/pending-dispatch/claim tables, guards and schema/version/import mirrors as one atomic compatibility change; no runtime owner or Task4 symbol is consumed.
- **Files**:
  - `internal/journal/migrations/0010_service_admission_bindings.sql` (verify next free number; never edit0009)
  - `internal/journal/service_admission_migration_test.go`
  - `internal/journal/journal.go`
  - `internal/journal/attachment_revision.go`
  - `internal/journal/attachment_revision_test.go`
  - `internal/journal/migrate_tables_test.go`
  - `internal/journal/journal_test.go`
  - `internal/journal/acceptance_test.go`
  - `internal/journal/evaluator_test.go`
  - `internal/journal/qualification_test.go`
  - `internal/migrate/backup.go`
  - `internal/migrate/backup_test.go`
  - `internal/migrate/import.go`
  - `internal/migrate/import_test.go`
  - `internal/migrate/preview/plan.go`
  - `internal/migrate/SUPPORT.md`
  - `internal/cli/migrate_test.go`
- **Produces**: Exact ADR0022 schema10 tables/foreign keys/identity and rowid guards/revision triggers, current migration registration/pragma/backup/preview/import compatibility. No Task4 owner or service runtime is consumed; Task30 produces typed constructors/storage methods after these tables exist.
- **Acceptance**:
  - `TestServiceAdmissionMigration` checks exact new tables/guards/version/migration list, preserves historical rows and schema8 fixture inventory, leaves unsupported/newer schemas unchanged and keeps old schema9 preparations unavailable. Independent backup/preview/import/CLI mirrors track the new version and preserve all new relations.
  - `TestServiceBindingImmutability` rejects SQL INSERT/UPDATE OR REPLACE collisions through every unique key and rowid alias, identity/byte/digest mutation, immutable byte/hash/root/peer/context/generation relations and invalid claim transitions directly in SQL; Task30 owns semantic expiry/confirmation decoding. A removed-guard fixture with an unreferenced victim fails.
- **Test plan**: In-memory/temp SQLite, direct hostile SQL and deterministic failure barriers; fake data only. The remaining schema/guard/version/import/backup files are one compatibility unit: registering the new schema without any current mirror or guard is the violating fixture; splitting their activation would make the binary accept an unsupported/mis-backed-up state. Task30 APIs and Task29 quota logic are independent producers, not hidden in this sizing exception. Compile imports to prove journal never imports admission/supervisor/service; mirror checks are independent of production constants. Task28 must first remove the stale-version coupling from current public backup guidance; all code/version/import mirrors change atomically here. No runtime source mirror is deferred.
- **Invariants touched**: I02/I09/I12/I20/I21/I23 (v2 §§5/6/8: atomic immutable admission, current quotas and canonical one-use intent).

### Task 30 — Produce closed preparation and admission transaction APIs

- **Domain/agent**: go-implementer
- **Budget**: complex
- **Depends on**: Task 23, Task 24, Task 25, Task 29
- **Change**: Implement exact copied structural constructors/getters and checked durable preparation/admission/arming methods after their leaf/schema/quota producers, without importing service/admission/supervisor into journal.
- **Files**:
  - `internal/journal/admission_binding_types.go`
  - `internal/journal/admission_binding_types_test.go`
  - `internal/journal/admission_bindings.go`
  - `internal/journal/admission_bindings_test.go`
- **Produces**: All exact design §3.2 declarations: PreparationLookup/State, PreparationOperation, ConfirmationV1, RetainedPreparationInput, ConfirmedPreparation getters, StoreRetainedPreparation/ConfirmPreparation/ReadConfirmedPreparation; NewAdmissionCommit/Snapshot, AdmissionMetadata/CommittedAdmission.Metadata and CommitServiceAdmission; NewDispatchArm/Snapshot, DispatchMetadata/CommittedDispatch.Metadata and ArmServiceDispatch. Records/constructors validate structure and identify storage effects; authenticated service confirmation, fresh supervisor permit and held owner remain separate authority requirements.
- **Acceptance**:
  - `TestClosedAdmissionConstructorsAndGetters` exercises nonempty valid controls, exact canonical-copy/pin/policy/identity fields and nil/closed/invalid handle cases from package journal_test using only exported APIs. Mutating every input/getter byte/slice/map leaves the stored private record unchanged; private-field/friend access is impossible.
  - `TestRetainedPreparationStages` writes no binding before approved exact retention input, stores that confirmation and recomputes Ready atomically: retention-only is true immediately at revision1; additional missing answers keep it false until complete. It exposes a confirmed handle only for the valid complete set. Forged Ready/Approved/digest, unknown challenge, rejected answer, stale revision/peer/root/context/generation/expiry and old schema9 record refuse with exact typed failure codes.
  - `TestServiceAdmissionAtomicStorage` injects each admission and arming statement/projection failure and observes complete rollback, pending→armed/unconfirmed/settled legal transitions, exact run revision advances and immutable acknowledgement replay. No attempts row or execution clock exists before arming; a constructor alone never changes state.
  - `TestAdmissionTypedFailureMapping` checks every declared constructor/store/read/commit/arm failure in §3.2 using errors.As, bounded safe diagnostic detail and explicit no-effect assertions. Named malformed-supported-version/future-schema and one/two-level wrapped leaf sentinels map by errors.Is to CodeInvalidContract/CodeCapabilityUnsupported; unexpected leaf errors fail closed as invalid_contract. Check exact safe constant Detail maps/slash-namespaced keys (and ControlError.Validate at complete operation boundaries), retain policy_ineligible for valid-but-refused retention, and reject raw decoder/input/wrapped-text leakage and string-classification mutants. The valid structural control still requires Task8/9 authenticated public dispatch to launch; low-level typed handles are not hostile-Go isolation.
  - `TestPreparationRevisionAndPolicyCommitment` exercises the exact outer Result.Revision acknowledgement/replay/conflict contract and rederives full pinned policy/field-set inside the confirmation transaction. A Ready/Approved/digest constructor or mismatched stored policy cannot grant authority or alter a confirmation.
  - `TestStoredRetentionOnlyReadiness` pairs a zero-total required set (missing mandatory retention, policy_ineligible, no rows/grant) with a retention-only nonempty set (first durable revision1/Ready=true/ack match and immediate confirmed read) and retention plus one/multiple additional challenges (false until final valid answer). Verify same-op/lost-ack replay, stale concurrency, rejection/conflict and injected row/ack rollback. A forced initial Ready=false or vacuous-empty-all-approved mutant fails; fresh revalidation remains required and no storage constructor launches a native.
- **Test plan**: Real SQLite plus deterministic transaction barriers, closed external-package constructor tests and guard/binding omission mutants. Compile imports and independently verify actual current-row rechecks rather than trusting a stale handle snapshot.
- **Invariants touched**: I02/I03/I12/I20/I21/I23 (v2 §§5/6/8: checked immutable effects and authority separate from structure).

### Task 31 — Produce revision-aware standalone client metadata

- **Domain/agent**: go-implementer
- **Budget**: complex
- **Depends on**: Task 2, Task 22
- **Change**: Add Go-only preparation reply metadata and explicit revisioned client operations over existing closed schema2 bodies; modify the actual client implementation rather than introducing a shadow file/constructor.
- **Files**:
  - `internal/control/client.go`
  - `internal/control/client_test.go`
  - `internal/control/run_contract.go`
  - `internal/control/run_contract_test.go`
  - `internal/control/attachment_contract_test.go`
- **Produces**: PreparedReply, PreparedRunClient, NewPreparedRunClient, PreparationConflictError.Current and private typedConnection.invokeResult exactly as design §3.1. Existing NewRunClient remains the same implementation; legacy Prepare/Confirm fail closed, while explicit Start/run revisions and schema2 wire DTOs remain unchanged. No Task4 owner or service implementation is consumed.
- **Acceptance**:
  - `TestPreparedClientRevisionedRoundTrip` invokes the actual exported factory over independently framed outer revisions0,1,2 and later. Retention plus multiple confirmations then Start use exactly returned revisions and stable operation IDs; body-only/default0/local-counter/discarded-envelope fixtures fail. Legacy Prepare/Confirm return revisioned_preparation_required without sending a request.
  - `TestPreparedClientLostAckAndConflict` commits a synthetic server confirmation, loses its reply and retries identical operation/arguments/revision to receive the exact original acknowledgement once. Concurrent new stale requests release only authenticated same-binding current metadata with zero effects. A historical ack followed by conflict refresh uses only the validated outer revision; foreign/consumed/changed-generation/conflicting-ID responses release no current body. Reconnect refreshes generation and invalidates old context after restart.
  - `TestPreparedReplyMetadataValidation` rejects mismatched operation/preparation/binding/generation, negative or missing usable revision, unsupported body schema and unsafe conflict metadata; Current/getters copy challenge slices. Existing control attachment/client tests still compile and retain their behavior.
- **Test plan**: Real exported client/framing path and independent server response fixtures, no generic fake counter inferred by the client. Task8 owns registered-service/SQLite revision production and Task9 owns the actual successful launch control. This is an additive Go API amendment, not a new wire field or native qualification.
- **Invariants touched**: I02/I09/I20/I23 (v2 §§3/6/8: server-authoritative preconditions and exact idempotent replay).

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

### Task 26 — Bind exact worker configuration source maps

- **Domain/agent**: go-implementer
- **Budget**: complex
- **Depends on**: Task 4, Task 23
- **Change**: Project the pinned adapter's available config-source inventory onto the exact admitted manifest and require the worker to validate that exact mapping before launch; preserve native authentication and legacy launch behavior.
- **Files**:
  - `internal/supervisor/config_sources.go`
  - `internal/supervisor/config_sources_test.go`
  - `internal/supervisor/pipeline.go`
  - `internal/supervisor/pipeline_launch_test.go`
  - `internal/workers/worker.go`
  - `internal/workers/worker_test.go`
  - `internal/workers/config_sources_test.go`
- **Produces**: exactBoundConfigSources(available map[string]string, admitted map[string]string) (map[string]string,error) for the core launch builder; worker pre-exec mapping verification uses the existing pinned adapter inventory API and requires keys/paths to match the admitted source set exactly before digest checks.
- **Acceptance**:
  - `TestExactBoundConfigSources` permits unadmitted candidate paths in the raw static inventory but emits only exact manifest keys; missing/unknown admitted source and empty/invalid path refuse. Source/digest-empty cases retain honest no-inventory behavior.
  - `TestWorkerConfigSourceIdentity` rejects missing/extra/swapped source→path payloads before native launch, including two files with equal content/digests. A fixture checking only hash equality fails; unchanged exact mappings launch an instrumented fake once.
  - `TestLegacyConfigLaunchStillBound` retains both adapter source inventories, worker rehash and existing launch field pin tests; no subscription secret/auth file is copied, native auth disabled, destination widened or adapter qualification inferred.
- **Test plan**: Synthetic files/config inventories and fake launch counter; actual source APIs already exist. Re-ground Task4 pipeline changes before editing; if an adapter API change is needed, stop for a separately scoped adapter producer instead of editing adapters in this task.
- **Invariants touched**: I02/I19/I20 (v2 §§6/8: exact config evidence and unchanged native credential ownership).

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
- **Budget**: complex (≤45minutes total worker envelope,15minute artifact checkpoint, at most2 repair passes≤15minutes each within that total; design §3.3)
- **Depends on**: Task 5, Task 22, Task 27, Task 28
- **Sizing**: The exact file count, coupled integration boundary, separated producers and finite acceptance/CI checkpoint are design §3.3; crossing that scope/envelope freezes a partial result rather than waiving checks.
- **Change**: Build the singleton composition shell and own its root lifetime context without introducing a control→supervisor import cycle; new methods return explicit unsupported until their later producer tasks land.
- **Files**:
  - `internal/service/runtime.go`
  - `internal/service/runtime_test.go`
  - `internal/service/observer_adapter.go`
  - `internal/service/observer_adapter_test.go`
  - `internal/service/observe.go`
  - `internal/service/shutdown_test.go`
  - `internal/supervisor/diagnostics.go`
  - `internal/supervisor/observer_notice_test.go`
  - `internal/supervisor/pipeline.go`
  - `internal/supervisor/recover.go`
  - `internal/supervisor/acceptance.go`
  - `internal/control/client.go`
  - `internal/control/server.go`
  - `internal/cli/supervisor.go`
  - `internal/cli/supervisor_test.go`
- **Produces**: service.Run(ctx context.Context, stateDir string) error; production SupervisorMain enters this runtime shell. This task does not claim full dispatch/attachment or worker-ingestion wiring; Task20 proves those after their producers.
- **Acceptance**:
  - TestServiceEntrypointShell exercises the real hidden-supervisor entry, root lock and authenticated status transport; root/status reflect the held singleton. Calling not-yet-implemented run.start or attachment.open returns capability_unsupported and writes no ledger/native state.
  - TestServiceRootContextSurvivesClientExit disconnects the status client, reconnects and observes the same service PID/generation; no native worker is launched or claimed as ingestion evidence in this shell test.
  - TestServiceRootConflict creates a conflicting root and observes refusal, no second listener/writer and zero native launches.
  - `TestObserverDeliveryNeverWaitsForClient` stalls a view writer for ten seconds while synthetic canonical commits and status/stop-intent rejection acknowledgements progress through the runtime within the existing two-second request budget; replacing the adapter with a blocking callback fails. This shell-stage test does not claim ordinary native ingestion.
  - `TestObserverCloseOrdering` calls RequestClose from inside an active callback, observes prompt callback return with owner/lease held, then releases the ingestion barrier and closes outside the callback. A synchronous owner.Close mutant deadlocks/fails the bounded test; view cancellation never stops or releases the owner.
  - `TestObserverNoticeSourceInventory` enumerates every Notice call in pipeline/recover/acceptance and requires a durable typed event/reason or one of the finite informational codes. A new unclassified call, dropped stop/verification evidence, raw error/path/account sentinel or persistence failure cannot look recorded; deleting one inventory entry fails.
  - `TestServiceShutdownWithBlockedEntry` refuses new requests and reports incomplete cleanup while an instrumented owned entry remains blocked, retaining ownership; after release it joins and closes in order. A timeout-alone release fixture fails. No cancellation of arbitrary OS I/O is advertised.
- **Test plan**: Synthetic fixtures and deterministic barriers for the named cases; pair negative assertions with a deliberately violating fixture. Public process behavior uses the packaged binary in the integration task, not helper-only evidence.
- **Invariants touched**: I03/I06/I18/I23 (v2 §3: one authenticated durable owner).

### Task 7 — Adopt the same worker under a new service generation

- **Domain/agent**: go-implementer
- **Budget**: complex (≤45minutes total worker envelope,15minute artifact checkpoint, at most2 repair passes≤15minutes each within that total; design §3.3)
- **Depends on**: Task 6
- **Sizing**: The exact file count, coupled integration boundary, separated producers and finite acceptance/CI checkpoint are design §3.3; crossing that scope/envelope freezes a partial result rather than waiving checks.
- **Change**: Reconcile the exact existing worker and record service adoption before enabling run controls; keep worker launch generation/deadline immutable and completed-episode wait separate from authority.
- **Files**:
  - `internal/control/adoption.go`
  - `internal/control/adoption_test.go`
  - `internal/service/reconcile.go`
  - `internal/service/reconcile_test.go`
  - `internal/service/observer_adoption_test.go`
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
  - `TestAdoptionWithStalledObserver` adopts the existing instrumented fake worker through the real actor/recovery entry while its observer never reads; committed spool sequence and the same launch identity advance with zero replacement launch. A blocking delivery mutant fails that progress assertion.
  - `TestAdoptionCancellationKeepsOwnerUntilIngestionExit` closes the view and requests service shutdown at deterministic ingestion/callback barriers. Callback close returns, worker/owner identity remains held until the active entry exits, then coordinator joins/releases once. Duplicate close, late wakeup and changed generation cannot release early or replay a launch; a lease-timeout-as-proof mutant fails.
- **Test plan**: Instrumented fake worker, deterministic commit barriers and a violating heartbeat-as-authority fixture; no native account.
- **Invariants touched**: I03/I06/I12/I18/I21 (v2 §§3/6/8: same-launch observation never grants a new episode).

### Task 8 — Prepare service-side admission and confirmations

- **Domain/agent**: go-implementer
- **Budget**: complex (≤45minutes total worker envelope,15minute artifact checkpoint, at most2 repair passes≤15minutes each within that total; design §3.3)
- **Depends on**: Task 6, Task 24, Task 30, Task 31
- **Sizing**: The exact file count, coupled integration boundary, separated producers and finite acceptance/CI checkpoint are design §3.3; crossing that scope/envelope freezes a partial result rather than waiving checks.
- **Change**: Refactor admission into bound preparation/challenges while retaining route, native trust and billing checks; construct environment only inside the service.
- **Files**:
  - `internal/admission/service.go`
  - `internal/admission/service_test.go`
  - `internal/admission/admission.go`
  - `internal/admission/billing.go`
  - `internal/admission/projectconfig.go`
  - `internal/supervisor/service_admission.go`
  - `internal/supervisor/service_admission_test.go`
  - `internal/service/admission.go`
  - `internal/service/admission_test.go`
  - `internal/control/admission_dispatch.go`
  - `internal/control/admission_dispatch_test.go`
  - `internal/control/server.go`
- **Produces**: supervisor.PrepareService(ctx,ServiceInputs) (*ServicePlan,error) and RevalidateService(ctx,journal.ConfirmedPreparation,ServiceInputs) (*LaunchPermit,error), using exact ADR0022/§3.2 inputs. Supervisor owns the private deep-copied Decision and one-use permit; no exported Decision getter. The service exposes only existing schema2 PreparedRun/RunConfirmation through authenticated standalone handlers. Initial unapproved preparation is bounded RAM only. Exact retention approval permits a durable preparation/retention confirmation whose Ready is computed from all required answers (true immediately for retention-only); remaining confirmations become durable there and recompute that predicate; only the valid complete record is ready and then supervisor.RevalidateService reads it before Start. Retention consent alone never admits or launches.
- **Acceptance**:
  - TestServiceAdmissionBinding invalidates task/config/repository/native-evidence drift between preparation and confirmation/start; native launch count stays zero.
  - TestServiceAdmissionAuthority rejects host/attachment token, foreign challenge and changed confirmation binding; noninteractive missing decisions retain actionable refusal.
  - TestServiceEnvironmentIsolation proves credential/PATH sentinels from caller maps are rejected and native policies remain pinned without serializing callbacks or full environment.
  - TestServiceAdmissionBindingAllInputs changes repository, revision, task bytes/digest, no-checks, project/native configuration and trust, route/profile, billing/envelopes and confirmation separately after prepare and at a deterministic pre-commit barrier. Each changed input invalidates the preparation with zero native launches; an unchanged control remains ready without launching; Task 9 owns the successful start control. Dropping any binding must fail its corresponding case.
  - `TestRetentionBeforePersistence` leaves only bounded volatile state until exact retain-local-binding approval, then persists computed readiness (retention-only true, otherwise false) and separately records any remaining confirmations before final RevalidateService; rejected/unknown/expired policy, email/path/argv/challenge sentinels and native auth/full-env provenance persist no binding or grant. A synthetic approved control writes the exact allowed record; it is not live authority.
  - `TestServicePreparationCapacity` fills32 total/8 peer volatile preparations, rejects overflow with zero durable/native effects and expires/cancels only unaccepted plans without changing accepted runs; an unbounded-store mutant fails. Binding/task byte caps and5minute expiry are separate from actual process-memory claims.
  - `TestServicePermitPrivateCopy` mutates each original slice/map/task/argv/config after revalidation and verifies unchanged private content/launch or explicit refusal, with copies sharing one-use state; a public Decision getter/always-true Confirm fixture fails ordinary-route tests.
  - `TestServiceRetentionConfirmationOrder` records no durable preparation before retention consent; then observes the correctly computed readiness with its authenticated retention confirmation, successive required answers if any, the complete ready=true record, ReadConfirmedPreparation and finally fresh RevalidateService. Each wrong-order/forged Ready/Approved/digest attempt refuses with zero launch; a valid staged control reaches ready without launching.
  - `TestServicePlanMetadataCopies` calls the exact public metadata/canonical getters from service-package tests, mutates returned challenges/bytes and observes unchanged private plan/permit; nil/closed/stale handles return typed failures and no Decision getter exists.
  - `TestPreparedAdmissionVolatileDispatch` invokes the actual registered prepare/confirm methods and observes no operations/preparation row before retention consent, bounded same-ID RAM replay, dependency_stale after restart, and peer/root/generation/token rejection before replay or data release. A generic Execute-before-consent fixture fails; post-retention answer/ack failures roll back together.
  - `TestServicePreparedOuterRevisions` runs the actual registered retention/multiple-confirmation handlers through PreparedRunClient and SQLite, matching every outer Result.Revision to the committed row, preserving exact same-op replay and refusing concurrent stale requests with no mutation. Start remains unsupported until Task9, which owns the real final launch control.
  - `TestServiceRetentionOnlyReadiness` pairs a zero-total required set (missing mandatory retention, policy_ineligible, no rows/grant) with a retention-only nonempty set (first durable revision1/Ready=true/ack match and immediate confirmed read) and retention plus one/multiple additional challenges (false until final valid answer). Verify same-op/lost-ack replay, stale concurrency, rejection/conflict and injected row/ack rollback. A forced initial Ready=false or vacuous-empty-all-approved mutant fails; fresh revalidation remains required and no storage constructor launches a native.
- **Test plan**: Synthetic fixtures and deterministic barriers for the named cases; pair negative assertions with a deliberately violating fixture. Public process behavior uses the packaged binary in the integration task, not helper-only evidence.
- **Invariants touched**: I02/I03/I04/I15/I19 (v2 §§6/7/8: admission and route authority precede launch).

### Task 9 — Commit and execute durable ordinary dispatch

- **Domain/agent**: go-implementer
- **Budget**: complex (≤45minutes total worker envelope,15minute artifact checkpoint, at most2 repair passes≤15minutes each within that total; design §3.3)
- **Depends on**: Task 7, Task 8, Task 30, Task 26, Task 28
- **Sizing**: The exact file count, coupled integration boundary, separated producers and finite acceptance/CI checkpoint are design §3.3; crossing that scope/envelope freezes a partial result rather than waiving checks.
- **Change**: Commit admission/assignment/dispatch intent once and run the extracted pipeline from the service actor; reconcile crash windows before spawn.
- **Files**:
  - `internal/service/dispatch.go`
  - `internal/service/dispatch_test.go`
  - `internal/service/runtime.go`
  - `internal/supervisor/pipeline.go`
  - `internal/supervisor/state.go`
  - `internal/supervisor/acceptance.go`
  - `internal/supervisor/exhaustion.go`
  - `internal/supervisor/service_admitted.go`
  - `internal/supervisor/service_admitted_test.go`
  - `internal/supervisor/dispatch_test.go`
  - `internal/supervisor/service_owner.go`
  - `internal/supervisor/service_owner_test.go`
  - `internal/journal/admission_bindings.go`
  - `internal/journal/admission_bindings_test.go`
- **Produces**: supervisor.RunAdmittedOwned(ctx,j,owner,permit,committedAdmission,sink) and shared fixed createRunTx/appendAdmissionTx/recordLaunchIntentTx extraction from existing pipeline/state/acceptance/budget code; run.start(StartRun) returns immutable RunAccepted; run.operation returns the persisted accepted identity/outcome under standalone authorization.
- **Acceptance**:
  - TestDispatchDuplicateLaunchOnce replays concurrent start operations and observes one run, attempt, launch and assignment.
  - TestDispatchCrashWindows covers before commit, after commit before spawn, after spawn before acknowledgment and after worker identity; only proven unconsumed original launch proceeds, ambiguity quarantines.
  - TestOrdinaryServicePipeline concludes candidate/verification/receipt through existing protected semantics; native completion alone remains candidate.
  - TestDispatchCommittedDecisionBinding changes each stored or reconstructed execution input after preparation/commit, including no-checks, repository/revision, task, configuration/trust, route/profile, billing/envelopes and launch inputs. Each mismatch refuses before worker launch; the unchanged accepted control launches once. Missing and unrecognizable committed evidence refuse without launch. A fixture that bypasses the committed binding fails the matching assertion.
  - TestProductionRunCannotBypassAdmission invokes each registered start path without a committed accepted preparation and with foreign peer, stale generation or mismatched binding. Each refuses with zero native launches; a valid confirmed control starts once. Concurrent duplicate starts and crash recovery preserve one canonical run/attempt and never reconstruct execution authority from mutable caller input.
  - `TestDispatchTwoTransactionOrder` observes one admission/run/reservation/assignment/pending intent/ack with no attempts row or native clock before snapshot; project trust follows snapshot evidence; atomic arming then adds one attempt/launch intent/pin/outbox and starts accounting once. A fake early-attempt/deadline fixture fails. Snapshot failure settles/releases once without an attempt.
  - `TestDispatchEveryTransactionFailureRollsBack` fails each run/event/pin/trust/declaration/envelope/quota/assignment/consumption/pending-intent insertion and every arming insertion/projection/CAS. Failed admission leaves no partial admission; failed arming leaves no attempt/outbox/accounting advance and zero spawn. Lost responses query the exact operation; no OR REPLACE or duplicate ignored CreateRun path.
  - `TestDispatchRestartMissingLiveContextRefuses` restarts before proven spawn with lost permit/environment epoch and records unconfirmed/interrupted without reconstructing native secrets or new launch; verified same-worker recovery preserves original generation/deadline. `TestLegacyPipelineNotInsertedTwice` retains original Run/Recover/Apply behavior and makes replay of runLocked initializers fail.
- **Test plan**: Synthetic fixtures and deterministic barriers for the named cases; pair negative assertions with a deliberately violating fixture. Public process behavior uses the packaged binary in the integration task, not helper-only evidence.
- **Invariants touched**: I02/I03/I06/I07/I12/I18/I20/I23 (v2 §§3/6/11: immutable admission binding and durable one-launch dispatch).

### Task 10 — Serve typed operation status and bounded observation

- **Domain/agent**: go-implementer
- **Budget**: complex (≤45minutes total worker envelope,15minute artifact checkpoint, at most2 repair passes≤15minutes each within that total; design §3.3)
- **Depends on**: Task 9
- **Sizing**: The exact file count, coupled integration boundary, separated producers and finite acceptance/CI checkpoint are design §3.3; crossing that scope/envelope freezes a partial result rather than waiving checks.
- **Change**: Implement the exact standalone read-only operation lookup and paged observation producer before ordinary CLI/accessible/TUI consumers; keep authenticated state and cursor selection coherent and independent of client liveness.
- **Files**:
  - `internal/service/observation.go`
  - `internal/service/observe.go`
  - `internal/service/observer_capacity_test.go`
  - `internal/service/observation_test.go`
  - `internal/control/client.go`
  - `internal/control/client_test.go`
  - `internal/control/run_contract.go`
  - `internal/control/run_contract_test.go`
  - `internal/service/runtime.go`
  - `internal/journal/observation.go`
  - `internal/journal/observation_test.go`
- **Produces**: the actual existing NewRunClient/NewPreparedRunClient implementation in control/client.go retains its constructors and gains the planned Observe behavior; run.operation(OperationQuery) returns OperationStatus and run.observe(ObserveRun) returns RunObservation, exact types/union/cursor/error rules in design §3.1. Observer remains read-only when later recover/apply/stop handlers populate their result arms.
- **Acceptance**:
  - TestRunOperationQuery authenticates run/root before lookup, retrieves the same accepted operation after a generation restart, and exposes typed running/completed/failed/unconfirmed outcomes without reexecution; foreign IDs disclose nothing.
  - TestRunObserveCursorSnapshot proves outcome/revision/page share one snapshot, sequences remain ordered,128-event/1MiB bounds hold, empty pages retain cursor and byte-bounded pages return the exact last consumed cursor; a zero limit returns the default128 when enough events exist, and an oversized single event returns protocol_mismatch/event_too_large with no cursor advance. A historical64/default or capability_unsupported mutant fails.
  - TestRunObserveHistoricalAndV2 decodes historical1 and canonical2 separately, consumes service.adopted without emitting undocumented stream output, and rejects malformed/gapped/new critical schemas with no cursor advance.
  - TestRunObserveFailureAndCancellation covers future cursor, generation change, missing history, permission denial and DB/transport timeout using the exact typed errors; a canceled1second poll holds no transaction/actor lock and never cancels the run.
  - `TestObserverCapacityAndCursor` exercises nonempty successful pages plus 64-total/16-peer/8-reader/one-connection-pending boundaries, zero-limit default128, 128-event/1MiB/1000ms limits, 500ms empty-poll backoff, and oversize-event protocol_mismatch with unchanged cursor. Missing scope/generation, lost wakeup and mixed schemas cannot skip required events; overflow/skip-limit mutants fail.
  - `TestObserveDisconnectDoesNotOwnExecution` stalls or cancels socket writes and releases only view slots while actor ingestion and accepted stop acknowledgement continue; it never waits for the observer, drops durable evidence or closes the owner. CLI history still stops at its existing1000-event bound after consuming contiguous pages.
  - `TestExportedObserveZeroLimit128` sends Limit0 through both actual exported client factories, serves a nonempty128-event framed response and observes wire limit128 plus correct cursor. The current client.go rewrite to64 must fail this test; direct handler-only tests do not satisfy it. Oversize handler reply remains protocol_mismatch/event_too_large with unchanged cursor.
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
  - Use the revised PreparedRunClient for any preparation path: explicit stable operation IDs, latest verified outer-result preparation revision, exact binding/generation and typed conflict refresh. `TestProductionClientsUseServerPreparationRevision` in the task's existing client test file covers staged confirmations/retry without guessed counters or legacy Prepare/Confirm calls; recover/apply-only paths retain their separate run revision contract.
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
  - Use the revised PreparedRunClient for any preparation path: explicit stable operation IDs, latest verified outer-result preparation revision, exact binding/generation and typed conflict refresh. `TestProductionClientsUseServerPreparationRevision` in the task's existing client test file covers staged confirmations/retry without guessed counters or legacy Prepare/Confirm calls; recover/apply-only paths retain their separate run revision contract.
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
  - Use the revised PreparedRunClient for any preparation path: explicit stable operation IDs, latest verified outer-result preparation revision, exact binding/generation and typed conflict refresh. `TestProductionClientsUseServerPreparationRevision` in the task's existing client test file covers staged confirmations/retry without guessed counters or legacy Prepare/Confirm calls; recover/apply-only paths retain their separate run revision contract.
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
  - Use the revised PreparedRunClient for any preparation path: explicit stable operation IDs, latest verified outer-result preparation revision, exact binding/generation and typed conflict refresh. `TestProductionClientsUseServerPreparationRevision` in the task's existing client test file covers staged confirmations/retry without guessed counters or legacy Prepare/Confirm calls; recover/apply-only paths retain their separate run revision contract.
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
- **Depends on**: Task 20, Task 28
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
