## V2 Contract Vocabulary — Tasks

### Dependencies

- Epic: `specs/*/v2-contracts-supervisor/plan.md` (this spec is stream 1; no prerequisite specs). Consumed by `supervisor-service` (record types, error codes), `supervisor-migration` (table/contract shapes, `Envelope`, OQ-9 decision), `supervised-stop-recover` (lifecycle tables, error codes); MH-22 adopts `TaskRevision` verbatim. No spec starts implementation before this one ships its contracts.
- Order is contract-first: Task 1 ships the kernel every later task builds against. Runnable sets: {2, 3, 4, 5, 6, 7} after 1 (disjoint Files, may run in parallel); {8} after {2, 3, 4, 5, 6, 7}. No two tasks share a file.
- Coverage: this spec has no end-user surface (no CLI/TUI/migration); developer-visible strings are owned by Task 5 (`ErrIllegalTransition` wording) and Task 6 (`ControlError.Error` wording).
- **Gates for every task.** `gofmt -w` on touched Go files first, then from the tree root `gofmt -l .` (must print nothing), `go vet ./...`, `go test -race ./...`, `go mod tidy -diff` (must print nothing), and `scripts/ci/check-public-hygiene.sh`, run via `scripts/harness/gate.sh` per `.claude/skills/quality-gates/SKILL.md`. A task is not complete because files exist or an agent reported success; cite the runner output. No OS-specific files, so no cross-`GOOS` vet.
- **Completion convention.** Append ` ✅ COMPLETED` to the heading, keep every original field, and add `Status` (`✅ Completed — …; PR #<n>`), `Implementation` (at most 3 lines, plus the commit SHA), `Spec deviations` (`None`, or each deviation with its reason) and `Files modified`. Every task appends a scratchpad note under Discoveries.
- **Commits.** Every commit is signed off (`git commit -s`, DCO). No ADR lands with these tasks (design D11: no billing/persistence/process-ownership change; the contract-freeze ADR belongs to `supervisor-service`).

---

## Implementation Tasks

### Task 1 — Package kernel: scales, claims, strict decode, digest, limits ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: None
- **Change**: Create `internal/v2contract` with `SchemaVersion`, the NFR-1 limit constants, the §4.2 vocabularies, the generic strict `Decode`/`Digest` codec and shared shapes, so every later task builds on one exact kernel.
- **Files**:
  - `internal/v2contract/v2contract.go` (package doc, `SchemaVersion`, limit constants, `CheckFrameLimits`, `GitObject`, `Budget`, `RevisionRef`, `RequestEnvelope`)
  - `internal/v2contract/scales.go` (`Capability`, `Progress`, `Eligibility`, `ScaleValue`, `Claim[T]`)
  - `internal/v2contract/codec.go` (`Validator`, `Decode[T]`, `Digest`)
  - `internal/v2contract/scales_test.go`
  - `internal/v2contract/codec_test.go` (also holds the `loadGolden` helper later tasks' tests reuse)
- **Produces**: `v2contract.SchemaVersion == 2`; `MaxFrameBytes == 1<<20`, `MaxNestingDepth == 64`, `MaxArtifactRefs == 128`; `v2contract.CheckFrameLimits(encodedLen, depth, refs int) error`; `v2contract.GitObject{Format, Value}`; `v2contract.Budget{Name, Limit, Unit}`; `v2contract.RevisionRef{Kind, ID, Revision, Digest}` with `Validate() error`; `v2contract.RequestEnvelope{OperationID, Object, ExpectedRevision, Generation}` with `Validate() error`; `v2contract.Capability` (`supported|unsupported|unknown`), `v2contract.Progress` (7 §4.2 words), `v2contract.Eligibility` (`eligible|blocked|unsupported`), each with `Valid() bool`; `v2contract.Claim[T ScaleValue]{Value, Evidence, Scope, ExpiresAt}` with `Validate() error` (expiry required); `v2contract.Validator` (`Validate() error`); `v2contract.Decode[T Validator](data []byte) (T, error)` (unknown fields rejected, then `Validate`); `v2contract.Digest(v any) string` (`sha256:<hex>` over canonical JSON).
- **Acceptance**:
  - `TestScalesMatchMasterWords`: the three scales equal the v2 §4.2 words exactly, and equal the `qualify` package's scale words (D3 pin); renaming one word fails the test.
  - `TestClaimRequiresEvidenceScopeExpiry`: a `Claim[Capability]` with empty evidence or scope fails `Validate` naming the field; an out-of-scale value fails naming the value; nil `ExpiresAt` and zero `ExpiresAt` each fail naming `expires_at`.
  - `TestRevisionRefValidate`: `Kind: "other"` fails naming `kind`; `Revision: 0` fails naming `revision`; empty `ID` fails naming `id`; a valid ref passes.
  - `TestRequestEnvelopeRequiresIdentity`: empty `OperationID` fails; negative `ExpectedRevision` fails; absent `Object`/`Generation` pass; a valid envelope passes (N1 envelope).
  - `TestDecodeRejectsUnknownField`: decoding into a kernel struct with an unknown key fails naming the key; truncated JSON fails naming the offset.
  - `TestDigestGolden`: `Digest` of a fixed struct equals its golden `sha256:<hex>` (oracle-checked with `sha256sum` over the exact bytes); flipping one byte changes the digest.
  - `TestFrameLimitsExact`: the three constants equal `1<<20`, `64`, `128`; `CheckFrameLimits` accepts the boundary values and rejects each over-limit dimension naming it.
- **Test plan**: Table tests over the scales; golden digest via `sha256sum`; boundary table for limits.
- **Invariants touched**: I09 (v2 §2: unknown stays distinct from zero/empty); I20 (v2 §2: digests content-addressed); I14 (v2 §2: honest-label scales).
- **Status**: ✅ Completed — `internal/v2contract` kernel (scales, claims, strict decode, digest, limits) with all seven acceptance tests passing; PR #234.
- **Implementation**: Generic `Decode[T]` uses `DisallowUnknownFields`, names offsets, rejects trailing data. `Digest` returns "" for unencodable values. Capability words pinned to qualify through `CanonicalDigest`. Commit 0763e55.
- **Spec deviations**: None.
- **Files modified**: `internal/v2contract/v2contract.go`, `internal/v2contract/scales.go`, `internal/v2contract/codec.go`, `internal/v2contract/scales_test.go`, `internal/v2contract/codec_test.go`, `specs/in-progress/v2-contract-vocabulary/tasks.md`, `specs/in-progress/v2-contract-vocabulary/handoff.md`.

### Task 2 — Execution records: Run, TaskRevision, Attempt ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 1
- **Change**: Add the execution records with strict validation and golden fixtures, so the ledger vocabulary (and MH-22's verbatim `TaskRevision` adoption) has its core types.
- **Files**:
  - `internal/v2contract/records_execution.go`
  - `internal/v2contract/records_execution_test.go`
  - `internal/v2contract/testdata/records/run.golden.json`
  - `internal/v2contract/testdata/records/task_revision.golden.json`
  - `internal/v2contract/testdata/records/attempt.golden.json`
- **Produces**: `v2contract.Run`, `v2contract.TaskRevision`, `v2contract.Attempt` (design §3, exact fields) with value-receiver `Validate() error` (schema_version == 2, IDs non-empty, `TaskRevision.Revision >= 1`, every dependency via `RevisionRef.Validate`).
- **Acceptance**:
  - `TestExecutionGoldensRoundTrip`: the three goldens decode, re-encode byte-identical, and carry `schema_version: 2`; a golden with `schema_version: 1` fails to decode.
  - `TestTaskRevisionRejectsZeroRevision`: `Revision: 0` fails `Validate` naming `revision`.
  - `TestTaskRevisionValidatesDependencies`: a dependency with `Kind: "other"` or `Revision: 0` fails `Validate` naming the dependency field; valid dependencies pass.
  - `TestRevisionDigestsDiffer`: two revisions of one task have different `Digest` values; identical bytes have equal digests (AC-1.2 structural pin).
  - `TestRecordTagsAreSnakeCase`: marshaling each record contains `"run_id"`/`"task_id"`/`"attempt_id"` and no `"RunID"`/`"TaskID"`/`"AttemptID"`.
  - `TestMissingIDsRejected`: empty `RunID`/`TaskID`/`AttemptID` each fail `Validate` naming the field, never defaulted.
- **Test plan**: Golden round-trips via the Task 1 `loadGolden` helper; table tests for validation; synthetic IDs via `ids.New`.
- **Invariants touched**: I20 (v2 §2: revisions required, digests differ); I09 (v2 §2: missing IDs rejected, never zero); I23 (v2 §5.1: records shaped for the single canonical ledger).
- **Status**: ✅ Completed — `Run`, `TaskRevision`, `Attempt` with strict `Validate` and three goldens; all six acceptance tests passing; PR #244.
- **Implementation**: Value-receiver `Validate` checks schema_version 2, non-empty IDs and lifecycle, `Revision >= 1` and every dependency via `RevisionRef.Validate` (error names `dependencies[i]`). Goldens are compact canonical JSON. Commit 1061475.
- **Spec deviations**: `internal/v2contract/codec_test.go` (outside Files): removed Task 1's `//nolint:unused` on `loadGolden`, now flagged unused-directive because this task's tests call it. `State` fields are `string`, not `RunState`/`TaskState`/`AttemptState`: those types are Task 5's (`lifecycle.go`, outside this task's Files, not on main). Wire shape is identical; Task 5 or 8 retypes them.
- **Files modified**: `internal/v2contract/records_execution.go`, `internal/v2contract/records_execution_test.go`, `internal/v2contract/testdata/records/run.golden.json`, `internal/v2contract/testdata/records/task_revision.golden.json`, `internal/v2contract/testdata/records/attempt.golden.json`, `internal/v2contract/codec_test.go`, `specs/in-progress/v2-contract-vocabulary/tasks.md`, `specs/in-progress/v2-contract-vocabulary/handoff.md`, `specs/in-progress/v2-contract-vocabulary/scratchpad.md`.

### Task 3 — Decision and evidence records ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: complex (7 files; one mechanical pattern across 5 record shapes)
- **Depends on**: Task 1
- **Change**: Add `RoutingDecision`, `DesignDecision`, `Artifact`, `Observation` and `Verification` with strict validation and golden fixtures.
- **Files**:
  - `internal/v2contract/records_evidence.go`
  - `internal/v2contract/records_evidence_test.go`
  - `internal/v2contract/testdata/records/routing_decision.golden.json`
  - `internal/v2contract/testdata/records/design_decision.golden.json`
  - `internal/v2contract/testdata/records/artifact.golden.json`
  - `internal/v2contract/testdata/records/observation.golden.json`
  - `internal/v2contract/testdata/records/verification.golden.json`
- **Produces**: `v2contract.RoutingDecision`, `v2contract.DesignDecision` (+ `DesignDisposition` `proposed|accepted|superseded`), `v2contract.Artifact`, `v2contract.Observation` (+ `ObservationKind` `controller|tool|native`), `v2contract.Verification` (design §3, exact fields) with value-receiver `Validate() error`.
- **Acceptance**:
  - `TestEvidenceGoldensRoundTrip`: the five goldens decode, re-encode byte-identical, and carry `schema_version: 2`.
  - `TestOutOfScaleEnumsRejected`: a `DesignDecision` with disposition `"approved"` and an `Observation` with kind `"agent"` each fail naming the field, never coerced.
  - `TestRoutingDecisionPreservesUncertainty`: a golden with scores absent and `uncertainty` set round-trips with uncertainty intact and no zero scores invented (I09).
  - `TestRoutingDecisionRejectsNonFiniteFloats`: NaN and ±Inf in `Scores` values or `SelectionProbability` each fail `Validate` naming the field; finite values pass.
  - `TestArtifactRequiresContentIdentity`: an empty SHA256 fails naming the field, as do non-hexadecimal and wrong-length values; exactly 64 lowercase hex chars pass; `ValidUntil` absent omits from JSON (never zero time).
  - `TestArtifactSizeBytesPresence`: `size_bytes` null and missing each fail naming the field; `size_bytes: 0` passes and round-trips as 0 (zero-byte artifacts valid; null never read as 0).
- **Test plan**: Golden round-trips via `loadGolden`; enum table tests; absence-vs-zero assertions on optional fields.
- **Invariants touched**: I09 (v2 §2: uncertainty preserved, absence never zero); I20 (v2 §2: `DesignDecision` revisions ordered via `Supersedes`); I07 lineage (v2 §2: `Verification` is the sole attestation shape).
- **Status**: ✅ Completed — the five decision and evidence records with strict `Validate`, goldens and all six acceptance tests passing; PR #239.
- **Implementation**: Unexported helpers carry an `evidence` prefix (`evidenceSchema` etc.) so parallel record tasks in the same package cannot collide. `DesignDecision.Supersedes` must name an earlier revision. The routing golden is the uncertainty case (scores absent, `uncertainty` set). Commits fec56d2, and the lint fix after it. The two files carry a file-level misspell exemption for the spec-mandated `Artifact` name.
- **Spec deviations**: `internal/v2contract/codec_test.go` (Task 1) lost its `//nolint:unused` directive on `loadGolden`: this task's tests now call the helper, so `nolintlint` flags the directive as unused.
- **Files modified**: `internal/v2contract/codec_test.go`, `internal/v2contract/records_evidence.go`, `internal/v2contract/records_evidence_test.go`, `internal/v2contract/testdata/records/routing_decision.golden.json`, `internal/v2contract/testdata/records/design_decision.golden.json`, `internal/v2contract/testdata/records/artifact.golden.json`, `internal/v2contract/testdata/records/observation.golden.json`, `internal/v2contract/testdata/records/verification.golden.json`, `specs/in-progress/v2-contract-vocabulary/tasks.md`, `specs/in-progress/v2-contract-vocabulary/handoff.md`.

### Task 4 — Coordination records ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: complex (8 files; one mechanical pattern across 6 record shapes)
- **Depends on**: Task 1
- **Change**: Add `ContextManifest`/`Handoff`, `Message`, `Grant`/`EffectIntent`, `Reservation`, `PolicyVersion` and `Experiment` with strict validation and golden fixtures.
- **Files**:
  - `internal/v2contract/records_coordination.go`
  - `internal/v2contract/records_coordination_test.go`
  - `internal/v2contract/testdata/records/context_manifest.golden.json`
  - `internal/v2contract/testdata/records/message.golden.json`
  - `internal/v2contract/testdata/records/grant.golden.json`
  - `internal/v2contract/testdata/records/reservation.golden.json` (uses `"quantity": "unknown"`)
  - `internal/v2contract/testdata/records/policy_version.golden.json`
  - `internal/v2contract/testdata/records/experiment.golden.json`
- **Produces**: `v2contract.ContextManifest` (+ `type Handoff = ContextManifest`), `v2contract.Message` (+ open `DeliveryDisposition`), `v2contract.Grant` (+ `type EffectIntent = Grant`; `Use` `standing|one_use`), `v2contract.Reservation` (open `Status`), `v2contract.PolicyVersion`, `v2contract.Experiment` (design §3, exact fields) with value-receiver `Validate() error`.
- **Acceptance**:
  - `TestCoordinationGoldensRoundTrip`: the six goldens decode, re-encode byte-identical, and carry `schema_version: 2`.
  - `TestGrantUseEnumClosed`: `Use: "permanent"` fails naming `use`; `standing` and `one_use` pass.
  - `TestAliasesDecodeIdentically`: the manifest golden decodes as both `ContextManifest` and `Handoff`, the grant golden as both `Grant` and `EffectIntent`, with equal digests.
  - `TestReservationUnknownQuantity`: the reservation golden's `"unknown"` quantity decodes and round-trips; empty quantity fails naming the field.
  - `TestOpenStringsValidatedNonEmpty`: empty `DeliveryDisposition`, `Reservation.Status` and `Experiment.Disposition` each fail naming the field (D6: open but never empty).
  - `TestExperimentRejectsNonFiniteSplits`: NaN and ±Inf in `Splits` values fail `Validate` naming the field; finite splits pass.
- **Test plan**: Golden round-trips via `loadGolden`; alias digest equality; enum/open-string table tests.
- **Invariants touched**: I09 (v2 §2: unknown stated explicitly, never empty); I20 (v2 §2: `PolicyVersion` immutability fields); I03 (v2 §2: `Grant` is the authority shape; strict decode).
- **Status**: ✅ Completed — six coordination records with strict validation and golden fixtures; PR #237.
- **Implementation**: Goldens are canonical single-line JSON (no `<`/`>`/`&`: `encoding/json` escapes them, breaking byte-identity). `Grant.Use` closed via `GrantStanding`/`GrantOneUse`; open strings validated non-empty. Commit 64c0cef.
- **Spec deviations**: `internal/v2contract/codec_test.go` (outside Files): removed Task 1's provisional `//nolint:unused` on `loadGolden`, now used — keeping it fails `golangci-lint` (`nolintlint`). Comment-only, no behaviour change.
- **Files modified**: `internal/v2contract/records_coordination.go`, `internal/v2contract/records_coordination_test.go`, `internal/v2contract/testdata/records/context_manifest.golden.json`, `internal/v2contract/testdata/records/message.golden.json`, `internal/v2contract/testdata/records/grant.golden.json`, `internal/v2contract/testdata/records/reservation.golden.json`, `internal/v2contract/testdata/records/policy_version.golden.json`, `internal/v2contract/testdata/records/experiment.golden.json`, `internal/v2contract/codec_test.go`, `specs/in-progress/v2-contract-vocabulary/tasks.md`, `specs/in-progress/v2-contract-vocabulary/handoff.md`, `specs/in-progress/v2-contract-vocabulary/scratchpad.md`.

### Task 5 — Machine-checkable lifecycles ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 1
- **Change**: Add the v2 §6.1–§6.2 state types with exact transition tables, the terminal-entry guard and the reconcile-identity guard, so illegal transitions fail in code with names.
- **Files**:
  - `internal/v2contract/lifecycle.go`
  - `internal/v2contract/lifecycle_test.go`
- **Produces**: `v2contract.RunState` (+ 15 constants), `v2contract.TaskState` (+ 10), `v2contract.AttemptState` (+ 12); `v2contract.ErrIllegalTransition`; `v2contract.CheckRunTransition(from, to, saved RunState) error`; `v2contract.CheckTaskTransition(from, to, saved TaskState) error`; `v2contract.CheckAttemptTransition(from, to AttemptState) error`; `v2contract.IsTerminalRunState(s RunState) bool`; `v2contract.CheckTerminalEntry(to RunState, ownershipResolved bool) error`; `v2contract.CheckReconcileIdentity(oldLaunchID, newLaunchID string) error`.
- **Acceptance**:
  - `TestRunTableMatchesSpec`: every §6.1 row's allowed pairs pass (including `planning`/`integrating`, absent from v1); `created→verifying`, `completed→executing`, terminal exits and unknown states fail with the from→to pair named.
  - `TestTaskTableMatchesSpec`: every §6.2 task row's allowed pairs pass; `accepted→ready`, `pending→accepted` and unknown states fail named.
  - `TestAttemptTableMatchesSpec`: every §6.2 attempt row's allowed pairs pass (including `reserved` entry and `waiting_native↔waiting_approval` movement); `running→reserved`, terminal exits and unknown states fail named.
  - `TestBlockedResumeRequiresSaved`: `blocked→executing` with `saved=executing` passes; with `saved=verifying` fails; `blocked→stopping` needs no saved phase (D8); `blocked→completed` with `saved=completed` fails (saved ineligible); `saved` unknown, terminal or edge-less (e.g. `stopping`) fails even when `to == saved`.
  - `TestTerminalEntryRefusesUnresolved`: `completed`/`cancelled`/`failed` with `ownershipResolved=false` fail; with `true` pass; nonterminal targets ignore the flag (AC-2.2).
  - `TestReconcileRequiresSameLaunch`: mismatched launch IDs fail; equal IDs pass (AC-2.3).
- **Test plan**: Tables transcribed from design §4.1 (each row: allowed set + sampled forbidden pairs); guard table tests.
- **Invariants touched**: I06 (v2 §2: terminal entry fenced on resolved ownership); I12 (v2 §2: same-launch reconcile); G04 (v2 §18.3: lifecycle gate tables).
- **Status**: ✅ Completed — v2 §6.1–§6.2 lifecycle tables with terminal-entry and reconcile guards in `internal/v2contract`, all six acceptance tests passing; PR #238.
- **Implementation**: Explicit tables plus the blanket active-phase stop/interrupt row and the recovering→reconciled-phase rule; blocked `to == saved` always requires saved eligibility. Commit 62d8495.
- **Spec deviations**: One interpretation (no stated requirement contradicted): "active phase" read as the 7 forward phases admission…applying; created is pre-admission and blocked/stopping/interrupted/recovering keep their explicit rows. Pinned in `TestRunTableMatchesSpec`.
- **Files modified**: `internal/v2contract/lifecycle.go`, `internal/v2contract/lifecycle_test.go`, `specs/in-progress/v2-contract-vocabulary/tasks.md`, `specs/in-progress/v2-contract-vocabulary/handoff.md`.
- **CI evidence**: `go-test` fails only on pre-existing `internal/cli TestStrictMainBlocksWriteNothing`, which fails identically on an untouched origin/main checkout; `internal/cli` does not reference `v2contract`.

### Task 6 — Error catalogue and adapter mapping

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 1
- **Change**: Add the 24-code catalogue with default dispositions, the rank-ordered widening check, `ControlError` and the namespaced adapter mapping.
- **Files**:
  - `internal/v2contract/errors.go`
  - `internal/v2contract/errors_test.go`
  - `internal/v2contract/testdata/error.golden.json`
- **Produces**: `v2contract.Code` (+ 24 constants, design §5) with `Valid() bool` and `DefaultDisposition() Disposition`; `v2contract.Disposition` (+ 5 constants); `v2contract.ControlError` (design §5, exact fields) with `Error() string` (`"v2contract: <code>: <next_action>"`) and pointer-receiver `Validate() error` (code in catalogue; disposition rank ≤ default; detail keys namespaced); `v2contract.MapAdapterFailure(code Code, owner, operationID, namespace string, cause error) *ControlError`.
- **Acceptance**:
  - `TestCatalogueHas24Codes`: the constant set equals the §4.5 list exactly; a 25th code or a renamed code fails the test.
  - `TestDefaultDispositions`: all 24 code→disposition mappings equal design §5's table.
  - `TestValidateRejectsWidenedDisposition`: `entitlement_ineligible` with `bounded_transient` fails; the default passes; a lower rank (e.g. `never` for `tool_failed`) passes (D9).
  - `TestValidateRejectsUnnamespacedDetail`: a `Detail` key without `/` fails naming the key; `adapter/claudecode/cause` passes.
  - `TestMapAdapterFailure`: the mapped error carries the given code, its default disposition, `Detail` key `<namespace>/cause`, and `Error()` in the exact shape; code (not message) is what a transition switch matches.
  - `TestErrorGoldenRoundTrip`: the golden decodes via `Decode[*ControlError]`, re-encodes byte-identical, and omits absent `revision`.
- **Test plan**: Set-equality test for the catalogue; full 24-row disposition table; golden round-trip.
- **Invariants touched**: I03 (v2 §2: adapter detail grants no authority; dispositions never widen); G04 (v2 §18.3: code-driven transitions).

### Task 7 — Event v2 envelope and sequence validators ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 1
- **Change**: Add the v2 `Envelope` mirroring `journal.Event` plus pure sequence/duplicate/generation validators in `Append`'s check order, deciding OQ-9's contract side.
- **Files**:
  - `internal/v2contract/envelope.go`
  - `internal/v2contract/envelope_test.go`
  - `internal/v2contract/testdata/envelope.golden.json`
- **Produces**: `v2contract.Envelope` (design §6, exact fields; `Payload` `toml:"-"`) with value-receiver `Validate() error` (schema_version == 2; IDs/type non-empty; sequences/generation ≥ 0; `ObservedAt` non-zero; payload a JSON object); `v2contract.ErrDuplicateEvent`, `v2contract.ErrSequenceGap`, `v2contract.ErrStaleGeneration`; `v2contract.CheckSequence(last, got int64) error`; `v2contract.CheckDuplicate(seen bool) error`; `v2contract.CheckGeneration(current, got int64) error`.
- **Acceptance**:
  - `TestEnvelopeGoldenRoundTrip`: the §4.3-shaped golden decodes, re-encodes byte-identical; `schema_version: 1` fails; non-object payload fails.
  - `TestEnvelopeRejectsMissingObservedAt`: envelope JSON without `observed_at` fails `Validate` naming `observed_at`; the zero time is never accepted.
  - `TestEnvelopeMirrorsJournalEvent`: the JSON tag sets of `Envelope` and `journal.Event` are equal field-for-field (spec-3 `Append` extension stays mechanical); any drift fails.
  - `TestCheckSequence`: `last+1` passes; equal, regressed, or skipped sequences fail with `ErrSequenceGap` (a re-sent sequence with a new event_id is a gap, not a duplicate — matching `Append`).
  - `TestCheckDuplicateAndGeneration`: `seen=true` yields `ErrDuplicateEvent`; older generation yields `ErrStaleGeneration` (quarantine at most, AC-4.2); current/newer generations pass.
  - `TestInt64SequencePrecision`: `9223372036854775807` sequences survive a JSON round-trip exactly (AC-9.1 sequence round-trips).
- **Test plan**: Golden round-trip; reflection tag-set comparison against `journal.Event` (test-only import); validator table tests incl. boundary int64.
- **Invariants touched**: I23 (v2 §5.1: envelope shaped for the single canonical ledger); I12 (v2 §2: stale generations quarantined, never accepted).
- **Status**: ✅ Completed — v2 `Envelope` mirroring `journal.Event` plus pure sequence/duplicate/generation validators in `Append` check order; PR #241.
- **Implementation**: `Validate` requires schema_version 2, non-empty IDs/type, non-negative sequences/generation, non-zero `ObservedAt`, JSON-object payload; `CheckSequence` guards `math.MaxInt64` overflow. Commit 35c0d64.
- **Spec deviations**: `internal/v2contract/codec_test.go` (outside Files): removed the now-stale `//nolint:unused` on `loadGolden`, which this task's tests call for the first time (nolintlint flags the unused directive).
- **Files modified**: `internal/v2contract/envelope.go`, `internal/v2contract/envelope_test.go`, `internal/v2contract/testdata/envelope.golden.json`, `internal/v2contract/codec_test.go`, `specs/in-progress/v2-contract-vocabulary/tasks.md`, `specs/in-progress/v2-contract-vocabulary/handoff.md`, `specs/in-progress/v2-contract-vocabulary/scratchpad.md`.

### Task 8 — Support matrix, hermeticity, and invalid-case evidence

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 2, Task 3, Task 4, Task 5, Task 6, Task 7
- **Change**: Publish the support matrix with a consistency test, pin contract hermeticity, and cover malformed/unknown keys, null measurements and TOML tags with fixtures.
- **Files**:
  - `internal/v2contract/SUPPORT.md`
  - `internal/v2contract/contract_test.go`
  - `internal/v2contract/testdata/invalid/unknown_authority_key.json`
  - `internal/v2contract/testdata/invalid/null_measurement.json`
  - `internal/v2contract/testdata/policy_version.golden.toml`
- **Produces**: `SUPPORT.md` (one row per deliverable: `fixture-tested` with test names, or `blocked`; zero `live-qualified` claims).
- **Acceptance**:
  - `TestSupportMatrixMatchesEvidence`: the matrix lists exactly the 14 records, 3 lifecycle tables, error catalogue, envelope and frame limits (adding a deliverable without a matrix row fails) and contains zero `live-qualified` claims (AC-9.2).
  - `TestContractHasNoNetworkDependency`: `go list -deps ./internal/v2contract` contains no `net` or `modernc.org/sqlite` line; importing either in non-test code fails the test (NFR-4).
  - `TestUnknownAuthorityKeyRejected`: the invalid `Grant` fails `Decode` naming the unknown key (AC-9.1).
  - `TestNullMeasurementsRejected`: the invalid `Reservation`/`Artifact` fail naming the field, and null is never read as `0` or `""` (AC-9.1; I09).
  - `TestTOMLTagsRoundTrip`: the TOML golden decodes via `BurntSushi/toml` with unknown keys rejected and re-encodes with identical keys (AC-1.1, AC-9.1).
  - `TestFixturesCarryNoSecrets`: every golden's decoded string values contain no `sk-`/`secret`/`token`/`apiKey` hit (values walked, not keys; NFR-4).
- **Test plan**: Matrix parsed from `SUPPORT.md` and compared to the deliverable list; `go list` subprocess test; invalid-fixture table tests; TOML round-trip.
- **Invariants touched**: I14 (v2 §2: versioned evidence, honest support states); I13 (v2 §2: no network service, no credentials); I09 (v2 §2: nulls rejected, never zero).
