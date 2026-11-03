## Immutable local verification inputs — Tasks

### Dependencies

- Baseline `f18ff8c` includes gate dependency-scope repair and four-worker harness-test argv; re-ground before implementation and preserve both. No external spec completion required for the initial harness profile. `specs/*/contained-execution-profiles/` and `specs/*/protected-acceptance/` supply vocabulary, not qualification or replacement authorities.
- DAG: 1 → 2 → 3 → 4 → 5 → 6 → 7. No parallel implementation is planned: the bounded result contract must stabilize before its consumers. Each task reads the preceding merged implementation and records deviations.
- **Gates for every task**: preserve Python/Bash style; shell files use `set -euo pipefail` and pass shellcheck. Format any changed Go fixture generator outputs with actual gofmt when appropriate. Run `scripts/harness/gate.sh harness` and `scripts/harness/gate.sh hygiene` for harness paths, plus `scripts/ci/lint-agent-harness.sh --only specs` for completion metadata. Focused shell entrypoints run real tools where qualification is claimed; orchestration mocks alone never qualify a profile. A file existing or an agent reporting success is not completion.
- **Completion convention**: append ` ✅ COMPLETED` to the task heading, then add Status (PR number), Implementation (commit), Spec deviations and Files modified fields; preserve original fields. Append discoveries to scratchpad and handoff according to the shared task-completion procedure.
- **Commits**: signed off, named paths, isolated task worktrees; land through reviewed PRs and required CI. This spec adds no product persistence or native process-owner authority. If implementation changes those contracts, stop for architecture review and an ADR rather than extending this harness scope.
- Every test name below is a planned Python unittest method with the exact stated name, implemented in that task's named test file. Shell entrypoints invoke the named file and propagate failures. No embedded Go test selectors are used.

## Implementation Tasks

### Task 1 — Define strict manifest, profile and result contracts

- **Domain/agent**: agent-config-editor
- **Budget**: complex (strict security-sensitive codecs and compatibility variants)
- **Depends on**: None
- **Change**: Add shared frozen record types, canonical digests and bounded versioned codecs from design §3, without executing a tool at import time.
- **Files**:
  - `scripts/harness/verification.py` (new)
  - `scripts/harness/tests/test_verification_records.py` (new)
  - `scripts/harness/tests/test_verification_records.sh` (new)
- **Produces**: `VerificationError`, `Limits`, `Candidate`, `Snapshot`, `Profile`, `Execution`, `MarkerVerdict`; `encode_record(record: dict) -> bytes`, `decode_record(data: bytes, *, kind: str) -> dict`, `record_digest(record: dict) -> str`, as exactly specified in design §3. Error `.code` uses its closed enum. Schema-1 records are accepted only in the legacy-policy decoding branch.
- **Acceptance**:
  - `TestVerificationManifestPinsInputs`: changing path/type/executable bit/content/tombstone, candidate/base/index, policy, definition or materialized-view binding changes the expected digest; a deliberately omitted-field digest variant fails the test.
  - `TestResultBindsExecutedView`: equal legacy endpoint fingerprints cannot make a different candidate, snapshot, tool/profile or argv binding acceptable; valid complete immutable and observed records remain distinguishable.
  - `TestVerificationStrictSchemas`: duplicate/unknown keys, unknown critical schema/version, nonfinite values, oversized records, invalid enums and malformed reference lists refuse; minimal valid and exact-limit records decode.
  - `TestToolAndEnvironmentProvenance`: unknown tool version remains explicit observation and refuses strong qualification; changed config/executable identity invalidates a qualified binding.
- **Test plan**: synthetic structured fixtures, including nonempty entries and independent one-field mutations. Verify the codec rejects, rather than silently drops, every critical field class. No compiler/native/provider calls.
- **Invariants touched**: I07/I20 (v2 §2: exact candidate/check/environment bindings); I09 (unknown and observed states remain distinct); charter principles 1/2/5/6 (strict bounded private formats).

### Task 2 — Implement private bounded snapshot ownership and retention

- **Domain/agent**: agent-config-editor
- **Budget**: complex (crash recovery, filesystem ownership and cancellation lifecycle)
- **Depends on**: Task 1
- **Change**: Add the private store, single-controller admission, capacity reservations, ownership transitions and safe reconciliation/collection from design §4.
- **Files**:
  - `scripts/harness/verification_store.py` (new)
  - `scripts/harness/tests/test_verification_store.py` (new)
  - `scripts/harness/tests/test_verification_store.sh` (new)
- **Produces**: `open_store(root: str) -> Store`; `Store.reserve(*, source_bytes: int, limits: Limits) -> str`, `load(digest: str, *, kind: str) -> dict`, `save(record: dict, *, kind: str) -> str`, `transition(snapshot_id: str, *, expected: str, target: str, updates: dict) -> None`, `reconcile(snapshot_id: str) -> str`, `collect(*, now_s: float) -> list[str]`. Explicit refusal codes: busy/store-full/ownership-unknown/cleanup-failed/invalid-record/io-error.
- **Acceptance**:
  - `TestSnapshotStoreBounds`: 64th retained snapshot and exact 10 GiB accounting boundary admit under synthetic bounded accounting; one over refuses before allocation. Real small-cap fixtures verify accounted metadata/staging/output/tool/scratch/quarantine bytes actually block writes/admission.
  - `TestRetentionProtectsOwnersAndReferences`: only unreferenced owned terminal successes ≥24h and failures/cancellations ≥7d collect; active, referenced, quarantined and unknown-owner sentinels survive even under capacity pressure.
  - `TestCaptureCancelAndCleanupOwnership`: interrupted-copy and cleanup-error states preserve an unrelated sentinel, refuse foreign-link/token/dirfd replacement and remain resumable without rerunning execution.
  - `TestPrivateVerificationArtifacts`: modes are private, store-inside-any-worktree refuses, synthetic secret sentinels appear only in intentionally private source fixtures and never in public diagnostics or whitelisted environment records.
  - `TestStoreCrashTransitions`: before/after atomic replacement faults preserve one durable state; stale expected-state update and second concurrent controller refuse. Unknown child start identity quarantines rather than kills a reused PID or deletes its files.
- **Test plan**: temporary synthetic repos and outside-root store, injectable wall clock and fault points; actual symlink/directory replacement fixtures. Do not allocate 10 GiB merely to test accounting, but retain nonempty actual-byte enforcement at reduced caps.
- **Invariants touched**: I08 (v2 §2: unrelated checkout/index/refs untouched); I09 (unknown ownership remains visible); I21 (all retention/admission/cancel states bounded); I23 (harness records do not replace product ledger); charter 2/5/6.

### Task 3 — Capture exactly requested and actually selected source inputs

- **Domain/agent**: agent-config-editor
- **Budget**: complex (filesystem races, input completeness and real consumer selection)
- **Depends on**: Task 1, Task 2
- **Change**: Implement the explicit requested-candidate descriptor, no-follow independent copy, selector-closure refusal and observed candidate comparison from design §2.
- **Files**:
  - `scripts/harness/verification_capture.py` (new)
  - `scripts/harness/tests/test_verification_capture.py` (new)
  - `scripts/harness/tests/test_verification_capture.sh` (new)
- **Produces**: `observe_candidate(root: str, *, gate: str, gate_definition: str, store: Store, limits: Limits) -> Candidate`; `capture(root: str, candidate: Candidate, *, store: Store, limits: Limits, cancelled, clock=time.monotonic, barrier=None) -> Snapshot`; `compare_candidate(root: str, candidate: Candidate, *, limits: Limits) -> bool`. Callbacks are monotonic clock, cancellation predicate and optional named barrier callback; production uses no test barriers. Raises typed capture-changed/candidate-mismatch/unmodeled-consumer-input/unsupported-path/limit-exceeded/cancelled errors.
- **Acceptance**:
  - `TestAmbiguousCaptureCannotCreditCandidate`: deterministic barriers replace a path/parent, change size/bytes or delete a required file during capture; every changed/unresolved capture refuses and a quiescent requested candidate materializes byte-identically.
  - `TestTrackedUntrackedIgnoredManifest`: a meaningful nonignored untracked Go file and tracked tombstone participate; ignored JSON churn does not. An ignored `.claude/data/private.go` makes strong capture refuse, and actual `gofmt -l .` demonstrably sees the deliberately unformatted file. Dot-prefixed Go basename is skipped by the pinned real-tool probe, not by guesswork.
  - `TestLinkAndTraversalCapture`: actual escaping symlink, changed symlink target, hardlink, special file, unsupported name/normalization collision and over-depth path refuse without touching foreign bytes. A safe ordinary directory/file control succeeds.
  - `TestSnapshotBudgets`: two files/64 bytes succeed; a third/65 bytes refuse. Production 20,000-entry, 64 MiB aggregate, 8 MiB file, 1 MiB buffer, depth64/node40,000 and 30-second deadline boundaries each have exact/one-over controls. Fault clocks must not turn deadline expiry into success.
  - `TestPinnedGitViewAndUnrelatedIndex`: controller observations of changed original refs/index make a selected descriptor mismatch while preserving original bytes; strong capture never provides raw Git metadata or credential-bearing config. A genuine Git-query gate remains unsupported by this initial profile.
- **Test plan**: `threading.Event`/pipe barriers with bounded waits, independent expected digests and real gofmt selection fixtures; missing gofmt reports the real integration as unavailable, never qualified. All portable structural tests still run. Fixture actors' edits are distinguished from verifier writes.
- **Invariants touched**: I07/I20 (v2 §2: requested and materialized identities match); I08 (original source/index/refs preserved); I09 (capture observations are not continuous proof); I21 (incremental bounds); charter 1/2/5/6.

### Task 4 — Enforce and qualify the initial read-only Linux profile

- **Domain/agent**: agent-config-editor
- **Budget**: complex (Linux namespaces, real-tool boundary probes and process teardown)
- **Depends on**: Task 1, Task 2, Task 3
- **Change**: Add the pinned Linux/amd64 static-gofmt bwrap profile and actual qualification/execution probes from design §5; absence of coverage refuses strong verification.
- **Files**:
  - `scripts/harness/verification_linux.py` (new)
  - `scripts/harness/tests/test_verification_linux.py` (new)
  - `scripts/harness/tests/test_verification_linux.sh` (new)
- **Produces**: `qualify_profile(*, helper: str, tool: str, expected_helper_sha256: str, expected_tool_sha256: str, store: Store, limits: Limits) -> Profile`; `execute(snapshot: Snapshot, profile: Profile, *, argv: list[str], store: Store, limits: Limits, cancelled) -> Execution`. Unavailable kernel/helper/mount/static-tool features yield profile-unavailable; drift yields tool-drift; timeout/output overflow/cancel/unresolved ownership never produce accepted execution.
- **Acceptance**:
  - `TestGateProfileMatchesRealCommand`: actual shared `gofmt -l .` executes the copied pinned executable; formatted source passes, unformatted exit-zero output fails, malformed source fails. Changed argv, stand-in executable and wrong tool digest cannot supply real qualification.
  - `TestExecutionViewWriteBoundary`: actual real gofmt `-w` boundary probe fails against immutable `/input` while the same actual tool succeeds writing a nonempty `/scratch` fixture; source digest remains unchanged and the positive scratch output differs.
  - `TestDeclaredOutputsAndUndeclaredSourceWrites`: qualification records actual mount/read-only observations, rejects writable source aliases/FDs and absent namespace enforcement, permits bounded declared tool scratch, and mounts no host original/store/home/config paths. An intentionally writable-bind profile variant fails the boundary test.
  - `TestOriginalChangesDoNotChangeExecutedInputs`: barriers change original after snapshot finalization; actual gofmt consumes the pinned materialization, and verifier-only original source/index/ref hashes remain unchanged.
  - `TestLinuxProfileUnavailable`: missing/changed helper, denied user namespaces, failed mount/write probe, wrong architecture, dynamic tool or unknown version yield unavailable and no pass; supported-host positive qualification uses the real tools. Unsupported-host skip reports only integration unavailable and never records qualified support.
  - `TestLinuxExecutionBudgetsAndTeardown`: exact 4 MiB combined output versus one-over, 10-second deadline and 2-second TERM/5-second join bounds use deterministic controlled child fixtures for orchestration; separately require real qualified pidns teardown and actual gofmt evidence. PID reuse/unknown descendants quarantine; no native/provider invocation occurs.
- **Test plan**: portable fail-closed unit fixtures plus a separate real-helper integration class; strict qualification invocation fails unavailable rather than treating a unittest skip as qualification. Keep process-fixture orchestration evidence distinct from actual gofmt tool evidence. No compiler substitution, package download or inference.
- **Invariants touched**: I07/I09/I20 (v2 §2: real executed view/tool identities and honest subset); I08 (original tree unchanged); I21 (finite ownership/output/time/scratch bounds); v2 §8.1 (trusted-host/controller residual); charter 1/2/3/5/6.

### Task 5 — Integrate strong execution and migrate shared consumers

- **Domain/agent**: agent-config-editor
- **Budget**: complex (schema migration, failure invalidation and compatibility)
- **Depends on**: Task 1, Task 2, Task 3, Task 4
- **Change**: Wire candidate/run/status policies into the shared engine, retain the shell adapter, and move health/stop-hook onto the central policy evaluator from design §6.
- **Files**:
  - `scripts/harness/gatelib.py`
  - `scripts/harness/gate.sh`
  - `scripts/harness/health.py`
  - `.claude/hooks/verify-task-completion.sh`
  - `scripts/harness/tests/test_verification_consumers.py` (new)
  - `scripts/harness/tests/test_verification_consumers.sh` (new)
  - `scripts/harness/tests/test_health.py`
  - `.claude/hooks/tests/test_verify_task_completion.sh`
- **Produces**: `evaluate_marker(root: str, gate: str, fps: dict, *, policy: str) -> MarkerVerdict`; compatible `marker_state(root, gate, fps, *, policy='legacy-observed')`; extended `run_gate(root, gate, *, mode='legacy-observed', candidate=None, profile=None)`. Exact CLI and JSON row contracts are design §§2/6; existing argv/group order/four-worker behavior remains.
- **Acceptance**:
  - `TestResultConsumerRequiresExactInputIdentity`: direct evaluate/marker_state, real shell status, stop-hook adapter and health all reject wrong tree/scope/candidate/profile/check definition and missing referenced artifacts under immutable-required; equal legacy fingerprints cannot substitute for manifest identity.
  - `TestFailedSnapshotRunCannotRetainPass`: start from a valid marker then fail setup/tool/output/capture/cancellation/publication; every path clears its prior target marker and exposes failed/unknown disposition with bounded reconciliation.
  - `TestSharedGateModeCompatibility`: direct `gate.sh`, default status, hook and health explicitly accept schema1 only as legacy-observed; immutable-required rejects it. Corrupt schema2, unknown critical keys/version and observed-copy relabeling refuse centrally.
  - `TestPartialRolloutLabelsAndRequiredGates`: Linux/amd64 go-fmt alone can be qualified; other gates/platforms stay observed or strong-unavailable. Existing group selection/order, four-worker argv and required gates remain unchanged. JSON/text expose mode and observational current comparison; health does not count legacy as immutable.
  - `TestTransformCannotReuseInputPass`: source-transform argv refuses the strong profile; a distinct transformed descriptor cannot reuse a prior marker. `TestWritableCopyDoesNotMasqueradeAsImmutable` rejects an observed-copy record under immutable-required.
- **Test plan**: disposable Git fixture integration through the actual shell/Python consumer paths, with strict JSON decoding and both nonempty valid/failing evidence. Re-run the repository-wide production-reader inventory and retain it in handoff; newly found executable readers join the shared migration before completion.
- **Invariants touched**: I07/I20 (v2 §2: all consumers share exact evidence); I09 (observed and qualified distinct); I08/I21 (failure cleanup/reservations preserved); charter 1/2/3/6 and WORKFLOW marker checks.

### Task 6 — Prove transient-read rejection and the complete acceptance matrix

- **Domain/agent**: agent-config-editor
- **Budget**: complex (deterministic concurrency and real-tool qualification evidence)
- **Depends on**: Task 1, Task 2, Task 3, Task 4, Task 5
- **Change**: Add end-to-end regression and strict local qualification entrypoint covering every frozen AC/NFR without promoting unsupported-host skips to success.
- **Files**:
  - `scripts/harness/tests/test_immutable_verification.py` (new)
  - `scripts/harness/tests/test_immutable_verification.sh` (new)
  - `scripts/harness/qualify_verification.py` (new)
  - `scripts/harness/tests/test_qualify_verification.py` (new)
  - `scripts/harness/tests/test_qualify_verification.sh` (new)
- **Produces**: strict qualification CLI `python3 scripts/harness/qualify_verification.py --helper PATH --tool PATH --helper-sha256 HEX --tool-sha256 HEX --root DIR --json`, exit 0 only for complete real qualification, 1 unavailable/failed, 2 usage. JSON contains `disposition`, `profile_digest` (null unless qualified), `reason` and sanitized evidence reference. It supplies no grant or automatic production activation.
- **Acceptance**:
  - `TestTransientReadCannotCreditBadCandidate`: retain quiescent real-consumer GOOD pass and BAD failure, then deterministic BAD/GOOD/BAD barriers through the real shared engine. The baseline real Python barrier-consumer fixture demonstrates endpoint limitation; strong run either refuses mismatched capture or records a GOOD manifest that cannot satisfy the previously pinned BAD candidate. For gofmt use capture/finalization/launch barriers as design §6 specifies, without claiming an instrumented internal read; use actual real gofmt for supported-profile end-to-end controls; no fake compiler or copied marker.
  - `TestImmutableVerificationAcceptanceMatrix`: explicitly enumerate nonempty positive/negative test evidence for AC-1.1–AC-8.2 and NFR-1–3, referencing the preceding exact test names. Deliberately disable manifest comparison, read-only bind or consumer policy in retained fault variants; each must make a corresponding check fail rather than merely alter a log.
  - `TestQualificationRequiresRealSupportedHost`: strict CLI refuses absent helper/kernel/tool, skipped probe or incomplete probe inventory, never emits profile digest on unavailable and succeeds only with complete actual supported-host evidence. Unit success alone cannot activate a profile.
  - `TestPublicQualificationSummaryPrivacy`: synthetic secret/config/path sentinels and arbitrary tool output remain private; public summary contains only whitelisted labels/digests/relative references. A deliberately copied raw output field makes the test fail.
  - `TestImmutableCliExitAndCleanup`: actual candidate/run/status shell flows exercise invalid input, unsupported profile, marker loss and child interruption with bounded waits and cleanup sentinels; no original index/ref/source mutation by the verifier.
- **Test plan**: deterministic named read/copy/exit barriers, deadlines and finally cleanup; avoid sleep races. The old recognized-input fixture is an endpoint-observation counterexample, not hostile-host evidence. Portable CI may state real Linux integration unavailable; strict qualification remains nonzero and strong production mode disabled there. Existing portable tests still run, and all required gates remain required.
- **Invariants touched**: I07/I09/I20 (v2 §2: independent exact evidence, bounded observed claims); I08/I21 (isolated fixtures/cancellation); v2 §8.1 (no hostile-host attestation); charter 1/2/5/6.

### Task 7 — Document shipped modes and exact qualification limits

- **Domain/agent**: agent-config-editor
- **Budget**: standard
- **Depends on**: Task 5, Task 6
- **Change**: Update harness usage only after its implementation and real qualification evidence exist; show explicit legacy/strong behavior and safe blocked-state recovery.
- **Files**:
  - `WORKFLOW.md`
  - `knowledge/execution.md`
  - `docs/harness/immutable-verification.md` (new)
  - `.claude/skills/quality-gates/SKILL.md`
  - `scripts/harness/tests/test_immutable_verification_docs.py` (new)
  - `scripts/harness/tests/test_immutable_verification_docs.sh` (new)
- **Produces**: contributor documentation of exact CLI, policies, profile support subset, private ownership/reconciliation, ignored `.go` refusal, bounds and explicit trusted-host limits. No private host paths or raw qualification records.
- **Acceptance**:
  - `TestImmutableVerificationDocumentation`: anchored mode/support/required-gate sections describe the actual CLI and keep all unsupported platforms/gates explicitly observed/unavailable; an intentionally universal-immutable claim or omitted required gate fails.
  - Execute synthetic candidate/run/status examples against the shipped wrapper and paste only sanitized generated output; missing qualified host shows unavailable, never invented success. Retain producing commands in the docs and test their field structure through the actual CLI.
  - Public hygiene and reference lint pass; documentation states endpoint comparison is observational and does not imply continuous original-tree equality or hostile-host attestation.
- **Test plan**: temporary source fixture plus strict actual CLI output parsing; documentation checks are anchored to claimed behavior and nonempty. Do not read or publish real source/config/telemetry.
- **Invariants touched**: I09 (v2 §2: limits/unknowns and observed qualification remain explicit); I07/I20 (claims match actual result identity); charter 3/5/6 and WORKFLOW required gates.
