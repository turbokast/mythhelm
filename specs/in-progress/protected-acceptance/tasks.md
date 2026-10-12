## Protected Acceptance — Tasks

### Dependencies

- Prerequisites shipped on origin/main: `specs/*/qualification-registry/` (MH-10), `specs/*/budget-ledger-s1/` (MH-16 S1), `specs/*/v2-contract-vocabulary/`, `specs/*/supervisor-service/`, `specs/*/supervisor-migration/`, `specs/*/supervised-stop-recover/` (MH-21), and the code of `specs/*/contained-execution-profiles/` (MH-13: all 11 tasks merged, finalize pending; Tasks 3-5 consume `contain.Evaluator` and `contain.EvaluatorDigest` as merged).
- **Blocked-on prerequisites.** Task 13 is blocked-on `specs/*/claude-strict-subscription/` Task 5 (held on that spec's Q1: the managed-policy source gap). Task 14 is blocked-on that spec's Task 7 (maintainer live qualification). Both are last and nothing depends on them; the other 12 tasks do not wait on MH-12. Each carries a `- **Blocked**:` field, which `scripts/harness/runspec.py` reads (`t["blocked"] = "Blocked" in f`) so `/run-spec` skips the task and its dependants; the lifecycle pull request that follows the prerequisite's merge removes the field.
- **Shared files serialize.** `internal/supervisor/pipeline.go` (Tasks 4, 5), `pipeline_test.go` (Tasks 5, 8), `faults_test.go` (Tasks 4, 5), `internal/supervisor/acceptance.go` and `acceptance_test.go` (Tasks 4, 5, 6), `internal/supervisor/receipt.go` (Tasks 5, 7), `internal/cli/render.go` (Tasks 8, 10). They are ordered by `Depends on`; never run two of them in parallel. Specs in flight that share files: `specs/*/codex-native-adapter/` (`internal/supervisor/pipeline.go`, `docs/limitations.md`, `docs/decisions/`), `specs/*/windows-process-ownership/` (`internal/supervisor/pipeline.go`, `recover.go`, `internal/cli/run.go`, `docs/limitations.md`), `specs/*/claude-strict-subscription/` Task 5 (`TestE2EDeclaredUnaffectedStrict` byte-compares declared receipts and labels and `TestE2EDeclaredLabelsDistinct` checks that no surface labels a declared posture qualified/verified/strict, which Tasks 7 and 10 change additively); whichever lands second rebases and updates the goldens.
- **Order is contract-first.** Task 1 ships the pure vocabulary, Task 2 the schema, Task 3 the evaluator seam; Task 4 pins at admission; Task 5 records the bound verdicts; Tasks 6-7 gate and report; Tasks 8-10 fix the stream and every surface (TUI before CLI, since the accessible view reads the viewmodel); Task 11 proves the whole flow from the packaged binary; Task 12 documents the finished behaviour.
- **Runnable sets.** {1, 2, 3} first, in parallel (disjoint Files); {4} after all three; {5} after 4; {6, 7, 8} after 5 (disjoint Files; 8 shares `pipeline_test.go` with 5 only); {9} (TUI) after 2 and 7; {10} (CLI) after 6, 7, 8 and 9; {11} after 9, 10; {12} after 11; {13} after 11 and MH-12 Task 5; {14} after 12, 13 and MH-12 Task 7.
- **Gates for every task.** `gofmt -w` on touched Go files first, then from the tree root `gofmt -l .` (must print nothing), `go vet ./...`, `GOOS=windows go vet ./...` and `GOOS=darwin go vet ./...` when OS-specific files change, `go test -race ./...`, `go mod tidy -diff` (must print nothing), and `scripts/ci/check-public-hygiene.sh`. For Task 12 and the spec files, `scripts/ci/lint-agent-harness.sh --only specs`. Run Go gates with an empty `HOME` (CEP handoff: the cli drift tests read `$HOME`). A task is not complete because files exist or an agent reported success; cite the runner output. Red first: each acceptance test is shown failing against the unchanged tree for the stated reason before the change (`.claude/rules/red-first.md`).
- **Completion convention.** Append ` ✅ COMPLETED` to the heading, keep every original field, and add `Status` (`✅ Completed — …; PR #<n>`), `Implementation` (at most 3 lines, plus the commit SHA), `Spec deviations` (`None`, or each deviation with its reason) and `Files modified`. Every task appends a scratchpad note under Discoveries.
- **Commits.** Every commit is signed off (`git commit -s`, DCO). The persistence and outcome-contract decisions (D1, D2, D5, D7, D8, D10, D14, D18, D19, D20, D21, D23) carry their decision record in Task 2's pull request; Task 12 completes it.

---

## Implementation Tasks

### Task 1 — Acceptance vocabulary package ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: complex (a new package of about ten exported abstractions that every later task builds on)
- **Depends on**: None
- **Change**: Create `internal/acceptance`, the pure package of design §2.1: `Result` and its labels, the P1 `PolicyVersion`, `Pin`, `Binding` with `Diff`, `GitObjectOf`, `EnvironmentID`, `Attest`/`BindingOf`, `Decide`, the step timeouts and the determinism text, so every later task builds on one definition of a verdict and its binding.
- **Files**:
  - `internal/acceptance/acceptance.go`
  - `internal/acceptance/acceptance_test.go`
- **Produces**: the exact API of design §2.1 (`acceptance.Result`, `Result.Label()`, `Result.Unverified()`, `P1() v2contract.PolicyVersion`, `NewPin(taskID, taskSHA256 string) Pin`, `Pin.WithChecks(...)`, `Binding.Diff(want Binding) []string`, `GitObjectOf(hex string) (v2contract.GitObject, error)`, `EnvironmentID(goos, goarch, verifier string, envNames []string) string`, `Attest(...) v2contract.Verification`, `BindingOf(v v2contract.Verification) Binding`, `Decide(pinEvaluator, ranEvaluator string, statuses []string) (Result, string)`, `DecideStatuses(statuses []string) (Result, string)`, `SHA256Ref(hex string) string`, `type Timeouts struct{Bind, Record, Render, Gate time.Duration}`, `DefaultTimeouts() Timeouts`).
- **Acceptance**:
  - `TestDecideOrdering`: table over (pin, ran, statuses): a failed check with an unavailable one yields `failed`, never `unavailable` (red against a `Decide` that returns the first non-passed status); an empty `statuses` yields `unavailable`/`no_checks_ran`, never `passed` (red against `len==0` falling through to passed); an empty pin evaluator yields `invalid`/`evaluator_pin_missing`; a differing ran digest yields `invalid`/`evaluator_digest_mismatch` even when every check passed.
  - `TestDecideStatusesIgnoresEvaluator`: `DecideStatuses` gives the same verdicts as `Decide` with a matching evaluator for every status table row, and never `invalid`.
  - `TestDecideUnknownStatusIsUnavailable`: a status outside the known set makes `Decide` and `DecideStatuses` return `unavailable`, never `passed`.
  - `TestBindingDiffNamesDimensions`: changing only the candidate, only the contract, only the check definition and only the evaluator each yields exactly that dimension name in the documented order; an empty required dimension on either side reports as differing; an empty `want.Environment` is not compared (red against a `Diff` that treats empty as equal).
  - `TestEnvironmentIDStableAndValueFree`: the same inputs give one ID regardless of `envNames` order; changing GOOS changes it.
  - `TestGitObjectOf`: 40 hex gives `sha1`, 64 gives `sha256`, 41 characters and a non-hex byte return errors.
  - `TestAttestRoundTripValidates`: `Attest(...)` passes `v2contract.Verification.Validate`, `BindingOf(Attest(b)) == b`, and `v2contract.Digest` is stable across two builds.
  - `TestPinDigestForms`: `NewPin(...).WithChecks(configHex, name, evalHex, timeouts)` stores `Contract`, `CheckDefinition` and `EvaluatorDigest` as `sha256:`-prefixed values of the bare inputs, and a `Pin` built without `WithChecks` has empty `CheckDefinition` and `EvaluatorDigest`.
  - `TestP1DigestPinned`: `v2contract.Digest(P1())` equals a literal recorded in the test, so editing P1 in place fails; `P1().Validate()` is nil.
  - `TestDefaultTimeoutsValues`: `DefaultTimeouts()` is `{Bind: 10s, Record: 30s, Render: 30s, Gate: 30s}`.
  - `TestResultLabelVerbatim`: `not_run`, `unavailable` and the payload value `NOT RUN` label `NOT RUN`, `invalid` labels `INVALID`, an unlisted value labels `unknown`; `Unverified()` is true for exactly `not_run` and `unavailable`.
- **Test plan**: Pure table tests, no I/O.
- **Invariants touched**: I07 (v2 §2: a verdict is defined only with its candidate, contract, check definition and environment binding); I09 (v2 §2: a missing dimension never matches and an empty check list never passes); I20 (v2 §2: P1 is an immutable, digest-pinned revision).
- **Status**: ✅ Completed — `internal/acceptance` ships the pure verdict, binding, pin, P1 and decision vocabulary of design §2.1; PR #398.
- **Implementation**: The exact §2.1 API; `Decide` returns `checks_failed` for a failed or timed-out verdict and an empty reason for `passed`; `EnvironmentID` hashes length-prefixed fields. Commit 8f29082.
- **Spec deviations**: None.
- **Files modified**: `internal/acceptance/acceptance.go`, `internal/acceptance/acceptance_test.go`, `specs/in-progress/protected-acceptance/tasks.md`, `specs/in-progress/protected-acceptance/handoff.md`, `specs/in-progress/protected-acceptance/scratchpad.md`.

### Task 2 — Schema 8: verification bindings and immutable policies ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: complex (persistence schema and migration)
- **Depends on**: None
- **Change**: Add migration 0008 (design §2.4): the `verification_bindings` side table and update/delete-abort triggers on it and on `policies`; extend `VerificationRow`, `InsertVerification` and `LatestVerification`; add `EnsurePolicy`; advance `SchemaVersion` to 8 and every mirror and version pin (the nine pin and mirror files are mechanical version edits, which is why the Files list is long); record the persistence decisions in the decision record in the same pull request.
- **Files**:
  - `internal/journal/migrations/0008_acceptance.sql`
  - `internal/journal/journal.go`
  - `internal/journal/projections.go`
  - `internal/journal/acceptance_test.go`
  - `internal/journal/journal_test.go`
  - `internal/journal/qualification_test.go`
  - `internal/journal/migrate_tables_test.go`
  - `internal/journal/evaluator_test.go`
  - `internal/migrate/backup.go`
  - `internal/migrate/preview/plan.go` (`supportedSchemaVersion` and `migrationFiles`)
  - `internal/migrate/apply_test.go`
  - `internal/cli/migrate_test.go`
  - `tests/migrate_e2e_test.go`
  - `docs/decisions/0019-protected-acceptance.md` (status `proposed`; D1, D2, D5, D7, D8, D10, D14, D18, D19, D20, D21, D23; the number is re-checked against `ls docs/decisions` at the task's start, 0017 and 0018 are claimed by `specs/*/windows-process-ownership/` and `specs/*/codex-native-adapter/`; take the next free number at the task's start; Task 12 and the other tasks locate the file as `docs/decisions/*-protected-acceptance.md`)
- **Produces**: `journal.SchemaVersion = 8`; `VerificationRow{…, Bound bool, Reason string, Attestation []byte, AttestationDigest string, OverheadMS *int64}`; `func EnsurePolicy(ctx context.Context, tx *sql.Tx, p v2contract.PolicyVersion) error`; `var ErrPolicyConflict`.
- **Acceptance**:
  - `TestMigration0008AdditiveAndV7DataSurvives`: a database built at schema 7 with a verification row opens at 8; its `LatestVerification` has `Bound == false`, empty `Reason`, nil `Attestation`; `0001_init.sql` through `0007_v2contracts.sql` are byte-identical to main (the test hashes them), and the `verifications` and `check_results` DDL in `sqlite_master` is unchanged. Red against a migration that runs `ALTER TABLE verifications ADD COLUMN`.
  - `TestVerificationBindingRoundTrip`: a row inserted with reason, attestation and overhead reads back equal, `Bound == true`; an attestation without a digest is rejected by the CHECK; a row inserted with no reason, attestation or overhead still reads `Bound == true` (red against an `InsertVerification` that skips the row).
  - `TestBindingsAndPoliciesImmutable`: `INSERT OR REPLACE` over an existing key and `UPDATE` and `DELETE` on `verification_bindings` and on `policies` each fail with the abort message; the unmodified row is still readable afterwards (red against a schema with no triggers).
  - `TestEnsurePolicyIdempotentAndConflict`: two calls with the same P1-shaped record leave one row; the same id and version with another digest returns `ErrPolicyConflict` and the stored row is unchanged.
  - `TestBackupPinsMatchJournal`, the version assertions and the preview step list pass at 8 and fail when only `journal.SchemaVersion` is advanced (red until `backup.go`, `preview/plan.go` and its `migrationFiles` move; the preview of a schema-7 database lists `0008_acceptance.sql` as a pending step).
  - `grep -rnE "bak-migration-v[0-9]" --include='*.go' .` prints nothing after the change (it prints matches in three files before): the backup-name pins in `internal/cli/migrate_test.go`, `tests/migrate_e2e_test.go` and `internal/migrate/apply_test.go` derive the name from `journal.SchemaVersion`, so `apply_test.go:142` still tests something at schema 8.
  - `grep -q "Status: proposed" docs/decisions/0019-protected-acceptance.md` and, for each of D1, D2, D5, D7, D8, D10, D14, D18, D19, D20, D21 and D23, `grep -qw "<id>"` on the file succeeds; exits non-zero before (file absent).
- **Test plan**: Real SQLite in temp directories; the schema-7 fixture is built by opening with the shipped migrations up to 0007 through the test helper used by `migrate_tables_test.go`.
- **Invariants touched**: I20 (v2 §2: policies and bindings are immutable by trigger); I23 (v2 §2: one canonical ledger, the binding is a row beside the verification, not a second store); I09 (v2 §2: an absent binding is `Bound == false`, never a default verdict).
- **Status**: ✅ Completed — schema 8 ships `verification_bindings`, abort triggers on it and on `policies`, the extended `VerificationRow`/`InsertVerification`/`LatestVerification`, `EnsurePolicy`/`ErrPolicyConflict`, every version mirror and pin, and ADR 0019 (proposed); PR #400.
- **Implementation**: `InsertVerification` always writes the binding row (after checking the attestation is the canonical encoding of this verification's and candidate's `v2contract.Verification` with a matching digest, because the table is immutable), so `Bound == false` means only "recorded before schema 8"; BEFORE INSERT triggers close `INSERT OR REPLACE`; 0008 also advances `migration_state.schema_version` to 8. Backup-name pins derive from `journal.SchemaVersion`. ADR number 0019 re-checked free (0001-0016 on disk; 0017 and 0018 claimed by other specs). Commit 2993119.
- **Spec deviations**: Three changes outside the Files list, each mechanical: `internal/migrate/SUPPORT.md` (the evidence test `TestSchemaVersionIs7` is renamed `TestSchemaVersionIs8`, and `TestSupportMatrixMatchesEvidence` reads that name), `docs/user-guide.md` (backup name `bak-migration-v8`) and migration 0008's `UPDATE migration_state SET schema_version = 8` (data only; `TestMigrationStateStartsNotStarted` pins that column to `SchemaVersion`). `TestMigration0007CreatesV2Tables` now opens `migrations[:7]` so it keeps testing 0007's objects. The preview step-list test for a schema-7 database (validation finding 10) is the new subtest `TestPreviewMirrorsPinned/schema_7_database_lists_only_migration_0008` in `internal/cli/migrate_test.go`.
- **Files modified**: `internal/journal/migrations/0008_acceptance.sql`, `internal/journal/journal.go`, `internal/journal/projections.go`, `internal/journal/acceptance_test.go`, `internal/journal/journal_test.go`, `internal/journal/qualification_test.go`, `internal/journal/migrate_tables_test.go`, `internal/journal/evaluator_test.go`, `internal/migrate/backup.go`, `internal/migrate/preview/plan.go`, `internal/migrate/apply_test.go`, `internal/migrate/SUPPORT.md`, `internal/cli/migrate_test.go`, `tests/migrate_e2e_test.go`, `docs/decisions/0019-protected-acceptance.md`, `docs/user-guide.md`, `specs/in-progress/protected-acceptance/tasks.md`, `specs/in-progress/protected-acceptance/handoff.md`, `specs/in-progress/protected-acceptance/scratchpad.md`.

### Task 3 — Evaluator seam and three-state check result ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: None
- **Change**: Extract the evaluator computation into `integration.EvaluatorFor` so the admission pin and the ran verification share one function (D12), export `MaxEvidenceBytes`, and make `RunChecksWithPolicy` set `Result` to `failed` only for a failed or timed-out check, `unavailable` when none failed and one was unavailable or not run, `passed` otherwise.
- **Files**:
  - `internal/integration/verify.go`
  - `internal/integration/verify_test.go`
- **Produces**: `func EvaluatorFor(opts RunOptions, checks []admission.CheckConfig) (contain.Evaluator, error)`; `const MaxEvidenceBytes = 1 << 20`; `Verification.Result` in `{passed, failed, unavailable}`.
- **Acceptance**:
  - `TestEvaluatorForEqualsRanEvaluator`: for a host run and (Linux) a contained run, `EvaluatorFor(opts, cfg.Checks)` equals `RunChecksWithPolicy(...).Evaluator`; the contained leg also equals an independently computed `contain.EvaluatorDigest(contain.BoundaryName, contain.BoundaryVersion, contain.Policy{Profile: "restricted", ReadOnly: true}, <check digests>)`, so an `EvaluatorFor` without `ReadOnly = true` turns it red even though both sides of the first equality would move together.
  - `TestEvaluatorChangesWithCheckDefinition`: changing one check's argv, timeout or `fail_on_output` changes the digest; the candidate's own `mythhelm.toml` (a commit that edits it) leaves it unchanged and the executed argv list unchanged (AC-2.1). The candidate-file leg pins behaviour that already holds (green before the change, kept as the AC-2.1 regression pin); the digest-changes legs are red before, because `EvaluatorFor` does not exist.
  - `TestUnavailableCheckIsNotFailed`: a missing executable gives `Result == "unavailable"`; the same run plus one failing check gives `failed` (red against the old rule where any non-passed status is `failed`).
  - `TestPassedNeedsEveryCheckPassed`: two checks, one `not_run` after an unavailable one, give `unavailable`, never `passed`.
- **Test plan**: Fixture check scripts in temp worktrees as in the existing `verify_test.go`; contained leg skips off Linux or where `ProbeLinux` fails, with the skip reason asserted non-empty.
- **Invariants touched**: I07 (v2 §11.2: the candidate cannot change the argv or evaluator it is judged by); I09 (v2 §2: unavailable is not failed and not passed).

- **Status**: ✅ Completed — `EvaluatorFor` is the one evaluator computation, `MaxEvidenceBytes` is exported and `RunChecksWithPolicy` returns the three-state result; PR #405.
- **Implementation**: `evaluatorOf` became `EvaluatorFor(opts, checks)` and forces `ReadOnly` and no auth binds itself; the result rule is a switch over statuses (failed or timed_out, else any other non-passed, else passed). `pipeline.go` is untouched: it still treats any non-`passed` result as not ready. Commit 5fd8a81.
- **Spec deviations**: None.
- **Files modified**: `internal/integration/verify.go`, `internal/integration/verify_test.go`, `specs/in-progress/protected-acceptance/tasks.md`, `specs/in-progress/protected-acceptance/handoff.md`, `specs/in-progress/protected-acceptance/scratchpad.md`.

### Task 4 — Pin the acceptance policy at admission ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 1, Task 2, Task 3
- **Change**: Add `NewAdmissionPayload` (design §2.2): `d.Record()` plus the run's `acceptance.Pin` (policy P1 v1, task revision 1, contract digest, and when checks will run the admitted config digest, evaluator and per-check timeouts), journal it from `appendAdmission` as `acceptance_pin` inside `admission.decided`, seed the P1 policy row in the same transaction, block the run on a pin failure, and add `ReadPin`; switch the recovery fixture to the same payload builder.
- **Files**:
  - `internal/supervisor/pipeline.go`
  - `internal/supervisor/acceptance.go`
  - `internal/supervisor/acceptance_test.go`
  - `internal/supervisor/faults_test.go` (`seedLaunch` journals `NewAdmissionPayload(d)` instead of `d.Record()`, so the recovery tests exercise a pinned admission)
- **Produces**: `type AdmissionPayload struct{admission.Record; AcceptancePin acceptance.Pin}`; `func NewAdmissionPayload(d admission.Decision) (AdmissionPayload, error)`; `func ReadPin(ctx context.Context, j *journal.Journal, runID string) (acceptance.Pin, bool, error)` (`false` when the admission predates the pin); `func checkOptions(d admission.Decision) integration.RunOptions` shared with Task 5; the `acceptance_pin` key of `admission.decided`.
- **Acceptance**:
  - `TestAdmissionJournalsPin`: a fake-adapter run with one admitted check journals `admission.decided` whose `acceptance_pin` has policy `P1`, version 1, the task digest as `sha256:<hex>`, `check_definition == "sha256:"+configDigest`, `evaluator_digest == "sha256:"+<the later verification row's EvaluatorDigest>`, and `check_timeouts` equal to the config's. Red against an unchanged `appendAdmission`.
  - `TestPinOmitsEvaluatorWhenChecksNotRun`: with `--no-checks` the pin has the policy and contract but empty `check_definition` and evaluator (renders as unknown, never a made-up digest).
  - `TestPolicyRowSeededOnceAcrossRuns`: two runs leave exactly one `policies` row for `P1`/1 whose digest equals `v2contract.Digest(acceptance.P1())`.
  - `TestOldAdmissionReadsAsNoPin`: an `admission.decided` payload without the key makes `ReadPin` return `(zero, false, nil)`; the existing `admission.Record` decoders still decode a pinned payload (the TUI `decodeAdmission` and `BuildReceipt` paths).
  - `TestPinAndVerificationEvaluatorAgree`: a restricted-profile run (Linux) and a trusted-host run each have a pin evaluator digest equal to `"sha256:"+` the recorded verification evaluator digest.
  - `TestPinFailureBlocksBeforeWorker`: with a policy row of id `P1`, version 1 and another digest pre-inserted (`ErrPolicyConflict`), the run ends `blocked`/`acceptance_pin_failed`, exit 3, with no attempt row and no worker (red against an `appendAdmission` that ignores the error or leaves the run in `admission`).
- **Test plan**: Existing `newFixture` and `fakeRun` helpers; journal reads through a read-only handle.
- **Invariants touched**: I07 (v2 §2: the evaluator is fixed at admission, before the candidate exists); I20 (v2 §2: the pin names an immutable policy and contract revision); I02 (v2 §2: the pin is journaled with the admission, so no launch precedes it); I09 (v2 §2: a check list not run leaves the pin's evaluator empty, shown as unknown).
- **Status**: ✅ Completed — admission journals the P1 `acceptance_pin` inside `admission.decided`, seeds the P1 policy row in the same transaction and blocks the run `acceptance_pin_failed` on a pin failure; `ReadPin` reads it back; PR #413.
- **Implementation**: `NewAdmissionPayload` pins the evaluator only when checks will run (not `--no-checks`, not inspect); `checkOptions` now feeds both the pin and `conclude`, so the two evaluators come from one call. `ReadPin` reads the last `admission.decided` and treats an absent or `null` key as no pin, a decode failure as an error. Commit 4813de4.
- **Spec deviations**: Added `TestPinEvaluatorFollowsProfile` and `TestPinLeavesChecksUnknownWhenNoneRun` beside the named tests (consistent with design §2.2): they cover the contained and inspect branches without a boundary, which the authoring host lacks, so the Linux-boundary legs of `TestPinAndVerificationEvaluatorAgree` and `TestPinOmitsEvaluatorWhenChecksNotRun` skip there and run in CI.
- **Files modified**: `internal/supervisor/acceptance.go`, `internal/supervisor/acceptance_test.go`, `internal/supervisor/pipeline.go`, `internal/supervisor/faults_test.go`, `specs/in-progress/protected-acceptance/tasks.md`, `specs/in-progress/protected-acceptance/handoff.md`, `specs/in-progress/protected-acceptance/scratchpad.md`.

### Task 5 — Bound verdicts, unverified outcomes and recovery ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: complex (verification state machine and recovery; 11 files, above the usual 8, because one outcome table must change in the pipeline, both recovery routes, the receipt and CLI exit mappings and their tests in a single pull request, or the tree is inconsistent between merges)
- **Depends on**: Task 1, Task 2, Task 3, Task 4
- **Change**: In `conclude`'s run path, read the pin from the journal, bind (design §2.3), `Decide`, and record the verification with its binding row and attestation under the step timeouts; apply the `verifyOutcome` table so unavailable checks end `ready_for_review`/`unverified` (D5, D18); give the skip paths a binding row with their reason and add `checks_unavailable`, `binding_unavailable` and `pin_missing`; map `verification_invalid` to exit 5; make both recovery routes (a `verifying` row and a surviving worker that finishes) use the same table.
- **Files**:
  - `internal/supervisor/pipeline.go`
  - `internal/supervisor/recover.go`
  - `internal/supervisor/receipt.go`
  - `internal/supervisor/acceptance.go`
  - `internal/supervisor/acceptance_test.go`
  - `internal/supervisor/pipeline_test.go`
  - `internal/supervisor/faults_test.go` (the recovery tests above)
  - `internal/supervisor/verify_outcome_internal_test.go` (package `supervisor`, for the unexported `verifyOutcome`)
  - `internal/cli/exit.go` (`runExit`: `verification_invalid` is `ExitVerify`)
  - `internal/cli/exit_test.go`
  - `tests/e2e/tui_test.go` (`attestation_digest` joins `tuiVolatilePayloadKeys`)
- **Produces**: `func verifyOutcome(err error, result acceptance.Result) (RunState, string)` (unexported, table of design §2.3); `verification.completed` payload keys `reason` and `attestation_digest`; `supervisor.Hooks.Timeouts *acceptance.Timeouts` and `Hooks.BindStep func(context.Context, acceptance.Binding) error` (nil means the default).
- **Acceptance**:
  - `TestVerdictBoundToCandidateContractChecksEnvironment`: after a passing run the stored attestation's `Candidate.Value` equals the frozen commit, `AcceptanceContractDigest` equals the pin contract, `CheckDefinition` the pin config digest, `CheckVersion` the evaluator digest and `Environment` a `sha256:` ID; `Results` has one entry per check and `Logs` one evidence identity per check.
  - `TestUnavailableChecksPreserveUnverifiedCandidate`: a missing check executable ends `ready_for_review`/`unverified`, exit 5, with the candidate row present, `result == "unavailable"`, reason `checks_unavailable`. It replaces the failed/`verification_unavailable` expectations of `TestMissingExecutableUnavailableExit5` and `TestUnavailableChecksStayUnverified` (red against the current pipeline, which ends `failed`).
  - `TestFailedBeatsUnavailable`: one failing check and one missing executable end `failed`/`verification_failed`, exit 5.
  - `TestEvaluatorDigestMismatchInvalid`: through the recovery fixture (`seedLaunch`, finished worker), an admission payload built by `NewAdmissionPayload` with `AcceptancePin.EvaluatorDigest` replaced by another `sha256:` value, and every check passing, `recover` ends the run `failed`/`verification_invalid`, exit 5, with a stored row of result `invalid` and no `passed` row, and the journaled `verification.completed` payload has `result: "invalid"` and `reason: "evaluator_digest_mismatch"` (red against a pipeline that compares nothing). The journal is never edited.
  - `TestRecoverConcludesWithJournaledPin`: the unmodified pinned recovery fixture ends `ready_for_review`, exit 0, `passed` (`TestRecoverUsesAdmittedChecksAndIdentity` stays green); a recovery fixture whose admission payload is `admission.Record` only (no pin) ends `ready_for_review`/`unverified` with reason `pin_missing`, no check executed, exit 5.
  - `TestVerifyOutcomeTable`: a table test of `verifyOutcome` over every verdict, the non-cancellation error and the cancellation, pinned to the table of design §2.3.
  - `TestBindFailureJournalsUnavailable` (checks all passed): with `Hooks.Timeouts` set to a 50 ms bind limit and `Hooks.BindStep` replaced by a function that blocks until its context ends (the test calls `supervisor.Run` in process, package `supervisor_test`), the verification is `unavailable`/`binding_unavailable`, has no attestation, the journaled payload has `result: "unavailable"` and `reason: "binding_unavailable"`, and the run is `ready_for_review`/`unverified`.
  - `TestSkipPathsRecordReason` (the `inspect` leg is Linux only, via `requireBoundary` as `pipeline_test.go:1821` does, and skips elsewhere with an asserted reason): `--no-checks` records reason `waived by --no-checks` and an `inspect` run `checks_refused_under_inspect`, each with `Bound == true` and no attestation; the `verification.completed` payloads still carry `result: "NOT RUN"`.
  - `TestRecoverAppliesSameOutcomeTable`: a `verifying` run whose row is `unavailable` recovers to `ready_for_review`/`unverified`; `invalid` recovers to `failed`/`verification_invalid`; `passed` recovers to `ready_for_review`.
  - `TestRecordStepHonoursTimeout`: with `Hooks.Timeouts.Record` set to 1 ns the verification append fails with `context.DeadlineExceeded` and the run stays `verifying` (red against a record step that ignores the limit).
  - `TestRecoverVerifyingWithoutRowAfterChecksReturned`: a `verifying` run with no verification row but one `check.completed` per admitted check (all `passed`) recovers to `ready_for_review`/`unverified` with row `unavailable`/`record_unavailable`, no attestation, exit 5, and a marker check script that must not have run again; the same fixture with one `check.completed` of status `failed` recovers to `failed`/`verification_failed`, exit 5 (red against a recovery that records `record_unavailable` regardless).
  - `TestRecoverVerifyingMidChecksStaysOwnershipUnresolved`: a crash during a still-running marker check (no `check.completed` events), in a pinned run and in a pin-less one (an admission payload of `admission.Record` only), makes `recover` exit 6 and leaves the run `verifying` with the check process untouched; against a recovery that records a verdict for any row-less `verifying` run it exits 5 and admits a new run beside the still-running check.
  - `TestRunnerErrorBeforeEvaluatorIsUnavailableNotInvalid`: a runner precondition error raised before the evaluator is computed (a contained check without `HOME`, Linux only, skipped elsewhere with an asserted reason) ends `ready_for_review`/`unverified` with `checks_unavailable`, never `verification_invalid`.
  - `TestFailedCheckSurvivesRunnerError` (Unix only; skipped on Windows with an asserted reason): one failing check followed by a check script without its executable bit (a start failure that is not ENOENT) ends `failed`/`verification_failed`, exit 5, never `checks_unavailable`; a runner error with only passed checks before it ends `unavailable`/`checks_unavailable`.
  - `TestFailedCheckSurvivesBindFailure`: with `Hooks.BindStep` failing and one check failed, the row is `failed` with no attestation and the run ends `failed`/`verification_failed`; with all checks passed it is `unavailable`/`binding_unavailable`.
  - `TestConcludeReadPinErrorFailsRun`: a corrupt `acceptance_pin` payload ends the run `failed`/`acceptance_pin_unreadable` with no check executed and the candidate frozen, exit 1; it is never treated as `pin_missing` (red against a `conclude` that returns the error and leaves the run `verifying`).
  - `TestRunExitMapsVerificationInvalid`: `runExit` for `failed`/`verification_invalid` returns `ExitVerify` and category `verification_invalid`, and the `recover` process exit equals the receipt's `exit_code` (red against the unchanged `runExit`, which returns 1).
  - `TestOverheadRecorded`: the row's `OverheadMS` is non-nil for a run with bound checks and nil for a skip path.
  - `TestSameInputsSameVerdict`: two fake-adapter runs of the same repository, task and checks give equal `Contract`, `CheckDefinition`, `Evaluator` and `Environment` and the same `Decide` verdict (the candidate commit differs, since its message carries the run ID); a third run with one check's argv changed gives a different `CheckDefinition` and `Evaluator`, so a binding that included a run ID or a clock value would fail the first comparison (AC-4.1).
- **Test plan**: Fake-adapter runs through `cli.Main`; tamper fixtures edit the journal in a temp state directory as `breakWorkspace` does; the contained leg is Linux-only and skips with an asserted reason.
- **Invariants touched**: I07 (v2 §11.2: acceptance is bound evidence, never the native's completion claim); I09 (v2 §2: unavailable is NOT RUN, never failed or passed); I12 (v2 §6.4: recovery applies the recorded verdict and never reruns checks); I05 and I24 (v2 §2: a run whose checks may still be running stays owned and unresolved); I02 (v2 §2: an unreadable pin fails the run, never a default); I20 (v2 §2: a changed evaluator is invalid, not reused).
- **Status**: ✅ Completed — `conclude` reads the journaled pin, binds the verdict and records it with its binding row under the step timeouts; unavailable checks end `ready_for_review`/`unverified`, a changed evaluator `failed`/`verification_invalid` (exit 5), an unreadable pin `failed`/`acceptance_pin_unreadable`; both recovery routes apply the same outcome table; PR #414.
- **Implementation**: `verifyOutcome`, `verifyChecks`, `skipVerification`, the timed bind and record steps and `Hooks.Timeouts`/`BindStep` live in `acceptance.go`; `recover.go` changes only the `verifying` branch and adds `recordFromCheckEvents` (a complete `check.completed` set records `record_unavailable`, otherwise `ErrOwnership`), so `descendantsResolved` and the quarantine path are untouched (#391). `reason` and `attestation_digest` are omitted from `verification.completed` when empty. Commit 86b246c.
- **Spec deviations**: `TestRecordStepHonoursTimeout` and the recovery fixture that needs a failed record step use a limit of -1 ns (already expired) instead of 1 ns: on a coarse monotonic clock (Windows CI) the deadline 1 ns ahead had not passed when the append began, so the record succeeded (windows-latest and windows-11-arm, run 38194229707). The cancellation branch settles the run under a non-cancelled context (review finding). `TestRunnerErrorBeforeEvaluatorIsUnavailableNotInvalid` reaches the pre-evaluator refusal through a recovery of a pin that names the contained profile with HOME unset in the recovering process (no boundary needed; consistent with design §2.3 and Linux only), because a contained native cannot launch without HOME. The pin-less leg of `TestRecoverVerifyingMidChecksStaysOwnershipUnresolved` moves a seeded run to `verifying` by hand with a stand-in check process, since a pin-less admission never reaches the run path. Added a `failed` case to `TestRecoverAppliesSameOutcomeTable` (consistent with design §2.3). A runner fault journals `check.completed` only for checks that ran; no row is invented for the interrupted check (consistent with D18). The Linux inspect leg of `TestSkipPathsRecordReason` skips on a host without the boundary, with its reason asserted by `requireBoundary`.
- **Files modified**: `internal/supervisor/pipeline.go`, `internal/supervisor/recover.go`, `internal/supervisor/receipt.go`, `internal/supervisor/acceptance.go`, `internal/supervisor/acceptance_test.go`, `internal/supervisor/pipeline_test.go`, `internal/supervisor/faults_test.go`, `internal/supervisor/verify_outcome_internal_test.go`, `internal/cli/exit.go`, `internal/cli/exit_test.go`, `tests/e2e/tui_test.go`, `specs/in-progress/protected-acceptance/tasks.md`, `specs/in-progress/protected-acceptance/handoff.md`, `specs/in-progress/protected-acceptance/scratchpad.md`.

### Task 6 — Apply gate on the binding

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 1, Task 2, Task 4, Task 5
- **Change**: Add `CheckAcceptance` (design §2.5) and make `ApplyRun` decide on it instead of the run reason, so a verdict that no longer applies to the candidate, contract or check definition is refused without `--accept-unverified` (D7, D13), and record an override in the version-2 receipt (D20).
- **Files**:
  - `internal/supervisor/acceptance.go`
  - `internal/supervisor/apply.go`
  - `internal/supervisor/apply_test.go`
  - `internal/supervisor/acceptance_test.go` (the `CheckAcceptance` legs)
- **Produces**: `func CheckAcceptanceWithin(ctx context.Context, j *journal.Journal, runID string, limit time.Duration) (AcceptanceStatus, error)`; `type AcceptanceState string` with `AcceptanceBound`, `AcceptanceUnverified`, `AcceptanceStale`, `AcceptanceUnbound`, `AcceptanceRejected`; `type AcceptanceStatus struct{State AcceptanceState; Reason string; Changed []string}`; `func CheckAcceptance(ctx context.Context, j *journal.Journal, runID string) (AcceptanceStatus, error)`.
- **Acceptance**:
  - `TestApplyRefusesStaleVerdict`: after a passing run, (a) rewriting `runs/<id>/task.md`, (b) updating the candidate row's commit, and (c) appending a newer verification whose attestation carries another `CheckDefinition` (written by the test through `journal.InsertVerification`) each make `apply` exit 3 naming `contract`, `candidate` and `check_definition` respectively, with `--accept-unverified` in the message; the same run unmodified applies. Red against the current gate, which applies all three.
  - `TestApplyOverrideRecordedInReceipt`: the stale run of case (a) applies with `--accept-unverified`, and its version-2 receipt has `acceptance.apply_override` with `state == "stale"` and `changed == ["contract"]`; a bound apply's receipt has no `apply_override` key (red against an `ApplyRun` that copies the version-1 receipt unchanged).
  - `TestApplyRefusesUnboundVerified`: a verification row with no binding row (inserted test-side, as a schema-7 run would be) is `unbound` and needs `--accept-unverified`.
  - `TestAcceptanceUnknownResultIsError`: a row with an unrecognised `result` and a binding row makes `CheckAcceptance` return an error, never `AcceptanceBound`.
  - `TestAcceptanceStatePrecedence`: a table over (row result, bound or not, attestation matching or not) pins the order of design §2.5; an old `not_run` row with no binding row is `unverified`, not `unbound`.
  - `TestApplyUnverifiedMessageNamesReason`: an `unavailable` run refuses with `NOT RUN: checks_unavailable` and still contains `--accept-unverified` (the existing `apply_test.go:163` assertion keeps passing).
  - `TestCheckAcceptanceErrorsNeverBound`: a corrupt attestation and an unreadable task file for a reason other than absence each return an error; a run with no verification row returns `AcceptanceUnverified` with reason `no verification recorded`; none returns `AcceptanceBound`.
  - `TestCheckAcceptanceWithinExpires`: `CheckAcceptanceWithin` with a 1 ns limit returns an error wrapping `context.DeadlineExceeded` and never `AcceptanceBound`.
  - `TestResumedApplySkipsGate`: a run with a journaled apply intent whose task file is rewritten after the intent resumes and completes; red against an `ApplyRun` that calls `CheckAcceptance` before checking the intent, which would refuse the half-applied run and strand its branch.
- **Test plan**: `apply_test.go` fixtures; tampering through the journal and run directory of a temp state.
- **Invariants touched**: I07 (v2 §2: an applied candidate has current bound evidence or an explicit user override); I08 (v2 §2: apply still creates only the authorised branch); I20 (v2 §2: a changed contract or check definition invalidates the verdict before reuse); I02 (v2 §2: an unreadable binding blocks).

### Task 7 — Truthful receipt ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: complex (receipt contract, many fields)
- **Depends on**: Task 1, Task 2, Task 3, Task 4, Task 5
- **Change**: Add `admission.EntitlementStatement` (design §2.8) and extend `BuildReceipt` additively (design §2.6): `billing.entitlement_statement`, `verification.{result,label,reason,binding}`, `acceptance.{policy_pin,determinism,overhead_ms,bounds}`, and `unknown_reasons` with the walk that rejects an unexplained `unknown`.
- **Files**:
  - `internal/admission/entitlement.go`
  - `internal/admission/entitlement_test.go`
  - `internal/supervisor/receipt.go`
  - `internal/supervisor/unknown_reasons.go`
  - `internal/supervisor/unknown_reasons_internal_test.go` (package `supervisor`, for the unexported `attachUnknownReasons`)
  - `internal/supervisor/receipt_test.go`
- **Produces**: `func BuildReceiptWithin(ctx context.Context, j *journal.Journal, runID string, limit time.Duration) (Receipt, error)` (`BuildReceipt` calls it with `DefaultTimeouts().Render`); `func EntitlementStatement(b adapter.BillingPosture) string`; `func attachUnknownReasons(r Receipt, reasons map[string]string) (missing []string)`; the receipt keys of design §2.6.
- **Acceptance**:
  - `TestEntitlementStatementVerbatim`: the declared Claude posture returns exactly `entitlement: user-declared, NOT verified by MYTHHELM; paid continuation: unknown (user declares disabled)`; a strict `Qualified`/`G05 passed` posture returns the verified form with `PaidContinuation`; `local-scripted` returns the not-applicable form; an inconsistent posture (`Qualified` true, `G05` not-passed) returns `entitlement: unknown`. Red against any implementation that returns the declared sentence for every posture.
  - `TestReceiptCarriesEntitlementStatement`: a declared-posture receipt's `billing.entitlement_statement` equals the declared sentence; a fake-route receipt carries the not-applicable form.
  - `TestReceiptVerificationLabelAndBinding`: a passed run has `verification.label == "passed"` and `binding.candidate.value` equal to `candidate.commit`; a `--no-checks` run has `label == "NOT RUN"`, `reason == "waived by --no-checks"` and `binding` `unknown`; an unavailable run has `label == "NOT RUN"` and `reason == "checks_unavailable"`; an `invalid` run has `label == "INVALID"`. Red against the current receipt, which has none of the keys.
  - `TestReceiptPolicyPinAndDeterminism`: `acceptance.policy_pin.policy_id == "P1"` with the evaluator digest of the verification; `acceptance.determinism.confined_to == "native session"`; a pre-schema-8 run renders `policy_pin` `unknown`.
  - `TestReceiptBoundsEnforcedFlags`: `acceptance.bounds` has the four envelope values from `run_envelopes`, `execution_seconds.enforced == true`, and the other three `enforced == false` (the test is rewritten in the task that closes #328); `check_timeouts` equals the pin's.
  - `TestEveryUnknownHasAReason`: `attachUnknownReasons` returns the path of an extra `unknown` leaf that has no registered reason (the broken input is kept), and returns nothing for `BuildReceipt` output of an unadmitted run, a blocked run, a failed-native run, a passed run, an unavailable-check run, an exhausted-bucket run with no reset, a restricted-profile run (Linux) and a Claude native-auth run (the last four reach branches the first four never do: `receipt.go:257-262`, `:364-367`, `:114-123`, `:181-185`); `BuildReceipt` renders `no reason recorded` for a missing path and never errors; each `unknown_reasons` entry's key resolves to a leaf that is `unknown` (no orphan reasons; `verification.baseline` is `not-run` and has none).
  - `TestBuildReceiptWithinExpires`: `BuildReceiptWithin` with a 1 ns limit returns an error wrapping `context.DeadlineExceeded` and writes no receipt. The three callers (`settleRun`, recovery's repair, `blockApply`) call `BuildReceipt`, which is `BuildReceiptWithin` by construction (no separate observable).
  - `TestReceiptTopLevelKeys`: the key set is the 19 of `TestReceiptHasAllSection9Keys` plus exactly `acceptance` and `unknown_reasons`, with `schema_version` still 1.
  - `TestReceiptUnknownNeverZero` keeps passing: cost and usage stay `unknown` with a reason, never `0`.
- **Test plan**: Existing `receiptRun` helper plus journaled fixtures for the blocked and failed-native cases.
- **Invariants touched**: I09 (v2 §2: every unknown prints `unknown` with its reason; label and verdict stay distinct); I10 (v2 §2: bounds are not presented as enforced where they are not); I15 (v2 §2: the declared posture is never worded as verified).
- **Status**: ✅ Completed — the receipt carries `billing.entitlement_statement`, `verification.{result,label,reason,binding}`, `acceptance.{policy_pin,determinism,overhead_ms,bounds}` and `unknown_reasons` with the walk that rejects an unexplained `unknown`, and `BuildReceiptWithin` bounds the render step; PR #417.
- **Implementation**: `admission.EntitlementStatement` is one switch (a declared source never reads as verified even when marked qualified); `unknown_reasons.go` holds the reason table keyed by normalised path and the walk, and `buildReceipt` overlays the reasons only the run knows (pre-schema-8, unreadable pin, no attestation). An unreadable pin renders `policy_pin` unknown instead of failing the receipt. Commit fab0635.
- **Spec deviations**: `TestBuildReceiptWithinExpires` uses an already-expired limit (-1 s) instead of 1 ns, as Task 5 found for the coarse Windows clock (consistent with design §2.9). The restricted-profile and native-auth legs of `TestEveryUnknownHasAReason` are journaled admissions rather than real Linux runs, so they run on every host (consistent with the design's fixture list; `TestReceiptCarriesBoundary` still covers the real restricted run). `TestReceiptHasAllSection9Keys` now asserts presence of the 19 keys and the exact set moved to `TestReceiptTopLevelKeys`. The unavailable-check and invalid legs of `TestReceiptVerificationLabelAndBinding` render journaled verification rows rather than real runs, and the passed and waived runs are captured once per test binary: the supervisor test binary took 430 s of the 10 min package timeout on windows-latest before this task, and 15 added real runs took it past the limit (run 38198740245; the timeout also surfaced `TestReplanPreservesRepairSlots`, whose replan deadline derives from the binary's start time, so it fails after about 9m45s of wall clock). Extra tests beyond the acceptance list: `TestRunWithUnreadablePinHasReceiptAndReviews` (validation finding 2), `TestReceiptOverheadCoversBindingStep` (finding 6, NFR-1), `TestReceiptBoundsUnknownWithoutEnvelope`.
- **Files modified**: `internal/admission/entitlement.go`, `internal/admission/entitlement_test.go`, `internal/supervisor/receipt.go`, `internal/supervisor/receipt_test.go`, `internal/supervisor/unknown_reasons.go`, `internal/supervisor/unknown_reasons_internal_test.go`, `specs/in-progress/protected-acceptance/tasks.md`, `specs/in-progress/protected-acceptance/handoff.md`, `specs/in-progress/protected-acceptance/scratchpad.md`.

### Task 8 — Documented JSONL event contract

- **Domain/agent**: go-implementer
- **Budget**: complex (event-set scan, renderer refusal and exit mapping across 8 files)
- **Depends on**: Task 5
- **Change**: Move the documented event set into production code (design §2.10), add the six other types a run's journal can hold to it (D10), make the JSONL renderer refuse an undocumented type with an error that `finish` folds into the exit code, and replace the test-local list by the production set with the three-leg pin.
- **Files**:
  - `internal/supervisor/events.go`
  - `internal/supervisor/events_test.go`
  - `internal/cli/render.go`
  - `internal/cli/run.go` (`finish`)
  - `internal/cli/exit.go` (category `undocumented_event`)
  - `internal/cli/exit_test.go`
  - `internal/cli/render_test.go`
  - `internal/supervisor/pipeline_test.go`
- **Produces**: `func StreamEventTypes() []string` (sorted copy, 29 entries); `func IsStreamEvent(typ string) bool`; `type UndocumentedEventError struct{Type string}` in `internal/cli`.
- **Acceptance**:
  - `TestStreamEventSetPinned`: `StreamEventTypes()` equals the literal sorted list of 29 types in the test (the 23 of the old `documentedEvents` plus `attempt.native_error`, `recovery.ownership_resolved`, `attempt.recovery_decided`, `run.extension_granted`, `migration.imported`, `migration.quarantined`).
  - `TestEveryEmittedEventTypeIsDocumented`: the `go/parser` scan of design §2.10 over `internal/supervisor`, `internal/workers`, `internal/control` and `internal/migrate` finds at least 25 distinct types (constants named `Event*`/`ev*`/`*Event`/`*EventType` and the event-type argument of `newEvent`, `append` and `applyEvent`) and every one is in the set. Two fixture sources are kept in the test: one with an undocumented emitter, which must be flagged, and one with the string `"receipt.json"` in a payload argument, which must not be collected.
  - `TestJSONLRefusesUndocumentedEvent`: feeding the JSONL renderer an event of type `attempt.surprise` writes nothing to stdout and `err()` is an `*UndocumentedEventError` naming it; a documented type is written.
  - `TestFinishFoldsUndocumentedEvent`: `finish` with a nil error and a renderer holding that error writes `run.result` with `exit_code` 1 and category `undocumented_event`, equal to the returned error's code (red against the current `finish`, which writes `exit_code: 0` and then returns the error); with an outcome error of exit 5 and the same renderer error, `run.result.exit_code` stays 5 and the renderer error is written to stderr.
  - `TestNativeErrorStreamsDocumented`: a fake `allowance-exhausted` run with `--format jsonl` streams an `attempt.native_error` line and every streamed line is a documented type (red before the set includes it).
  - `TestClaudeRateLimitStreamsDocumented` (the #74 claudecode JSONL-envelope test; it calls `requireUnixClaude` as its sibling tests do): `installFakeClaude` (`internal/supervisor/claudecode_test.go:86`, same `supervisor_test` package) stages a stream with `{"type":"assistant","error":{"class":"rate_limit"}}` as `TestRateLimitNoInventedCountdown` does (`claudecode_test.go:481`); `run --adapter claudecode --format jsonl` streams an `attempt.native_error` line, every streamed line decodes strictly and is a documented type, and a mutant set without `attempt.native_error` turns it red.
  - `TestRecoveryOwnershipResolvedDocumented` (Linux only: the fixture reuses `TestRecoverFreezesOnlyAfterDescendantsGone`'s setup that skips off Linux, with the skip reason asserted): `recover --format jsonl` over the quarantined-lost-worker fixture streams `recovery.ownership_resolved` and every line is documented.
  - The existing `TestJSONLStdoutOnlyEnvelopes` keeps passing using `supervisor.IsStreamEvent` instead of its local list.
- **Test plan**: AST scan with `go/parser`; the recovery-stream fixture copies the inline setup of `TestRecoverFreezesOnlyAfterDescendantsGone` (`faults_test.go:521`) into `events_test.go`.
- **Invariants touched**: I12 (v2 §2: recovery evidence is a documented event, not dropped); I09 (v2 §2: nothing undocumented reaches a consumer as if it were contract).

### Task 9 — TUI surfaces

- **Domain/agent**: tui-implementer
- **Budget**: complex (two decoders, the mission view and the viewmodel)
- **Depends on**: Task 2, Task 7
- **Change**: Show the verification reason with the `NOT RUN` label and the entitlement statement in the mission view, reading `VerificationRow.Reason` and the admission record (design §2.7), and add `Admission.EntitlementStatement` to the viewmodel for the snapshot and the live-event decoders; the accessible CLI view reads it in the next task.
- **Files**:
  - `internal/tui/mission.go`
  - `internal/tui/viewmodel/viewmodel.go`
  - `internal/tui/motion.go` (`decodeLiveAdmission`)
  - `internal/tui/motion_test.go`
  - `internal/tui/model_test.go`
  - `internal/tui/viewmodel/viewmodel_test.go`
- **Produces**: `viewmodel.Admission.EntitlementStatement string`, set by `decodeAdmission` and `decodeLiveAdmission` from `admission.EntitlementStatement(rec.Billing)`.
- **Acceptance**:
  - `TestMissionNotRunShowsReason`: a snapshot with a `not_run` verification and reason `checks_refused_under_inspect` renders `verification: NOT RUN (checks_refused_under_inspect)`, and with reason `waived by --no-checks` renders that text; an `unavailable` row renders `NOT RUN (checks_unavailable)`. Red against the hard-coded waived text.
  - `TestMissionNotRunWithoutReasonSaysUnknown`: a row with an empty reason (older run) renders `NOT RUN (reason unknown)`, never a blank or an invented reason.
  - `TestMissionStatusStripEntitlement`: the billing strip carries the declared sentence for a declared admission and `entitlement: unknown` when the admission is absent.
  - `TestLiveAdmissionEventCarriesStatement`: feeding a live `admission.decided` event to the model sets the strip's statement in the same frame, not after the next reload (red against a live decoder that leaves the field empty).
  - `TestViewmodelLoadsReasonAndStatement`: `Load` fills the verification reason and the admission entitlement statement from a journaled run.
- **Test plan**: Existing snapshot model tests; no TTY needed.
- **Invariants touched**: I09 (v2 §2: the TUI never renders an unverified run as verified); I15 (v2 §2: declared stays worded as unverified).

### Task 10 — CLI surfaces: review, run stream, accessible

- **Domain/agent**: go-implementer
- **Budget**: complex (review, stream and accessible renderers, 6 files)
- **Depends on**: Task 6, Task 7, Task 8, Task 9
- **Change**: Show the entitlement statement, the verification label with its reason, and the acceptance status on plain `review`, the `run`/`recover` admission and verification lines, and the `--accessible` lines (design §2.7), replacing the hard-coded `(waived by --no-checks)`.
- **Files**:
  - `internal/cli/review.go`
  - `internal/cli/render.go`
  - `internal/cli/accessible.go`
  - `internal/cli/review_test.go`
  - `internal/cli/render_test.go`
  - `internal/cli/accessible_test.go`
- **Acceptance**:
  - `TestReviewPlainShowsEntitlementAndVerification`: plain `review` of a declared-posture run prints a line equal to the declared sentence and a `Verification:` line; of a `--no-checks` run prints `NOT RUN (waived by --no-checks)`; of an inspect run `NOT RUN (checks_refused_under_inspect)`; of a passed run `passed (bound to <candidate>)`; the `Verification:` line is the only place a waived run's reason appears (red against today's output, which prints `State: ready_for_review`, `Checks: none` and no label or reason).
  - `TestReviewPlainShowsAcceptanceStatus`: a bound run prints `Acceptance: bound`; after the task file is rewritten it prints `STALE (changed: contract)`; a pre-binding run prints `unbound`.
  - `TestRunStreamPrintsVerdictLabels`: the plain stream prints `verification: INVALID (evaluator_digest_mismatch)` and `verification: NOT RUN (binding_unavailable)` for payloads with those results and reasons (journaled by Task 5), never `invalid` or `unavailable` raw.
  - `TestRunStreamNotRunCarriesReason`: the plain run stream prints `verification: NOT RUN (<reason>)` with the payload's reason; the `inspect` case no longer says `waived`.
  - `TestAdmissionLinesCarryEntitlement`: the plain admission block has a line equal to two spaces plus `admission.EntitlementStatement(posture)` for the declared and the fake route.
  - `TestAccessibleVerificationLineReason`: the accessible line for a `not_run` row prints `verification: NOT RUN (<reason>)` using `Result.Label()` (red against today's `accessible.go:297`, which tests `"NOT RUN"` against the stored `not_run` and prints `not_run`); it carries the row reason, and the entitlement statement appears in the accessible admission line.
- **Test plan**: `cli.Main` with temp state directories and golden fragments anchored to the line prefixes, not file-wide substrings.
- **Invariants touched**: I09 (v2 §2: NOT RUN is distinct from verified on every CLI surface); I15 (v2 §2: the declared sentence is on every summary); I07 (v2 §2: review shows whether the verdict still applies).

### Task 11 — Credential-free end-to-end flow

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 9, Task 10
- **Change**: Prove the P1 flow (admit, run, check, review, apply) from the packaged binary with no agent credentials, including the unverified, stale and entitlement paths. These are acceptance tests over behaviour Tasks 1-10 ship; each names its failing state against origin/main before Task 1 (the pin key, the label, the `STALE` outcome or the sentence is absent there).
- **Files**:
  - `tests/e2e/acceptance_test.go`
- **Acceptance**:
  - `TestE2EP1FlowNoCredentials`: with every credential variable removed from the child environment (`ANTHROPIC_API_KEY`, `CLAUDE_CODE_OAUTH_TOKEN`, `OPENAI_API_KEY`, asserted absent), the binary's fake-route run with a passing check, `review`, and `apply --to-branch` exit 0, 0, 0; the receipt's `verification.label` is `passed` and its `acceptance.policy_pin.policy_id` is `P1`.
  - `TestE2EUnavailableChecksKeepCandidate`: a check whose executable is missing exits 5; `review` shows `NOT RUN (checks_unavailable)` and the candidate commit; `apply` without `--accept-unverified` exits 3, with it exits 0.
  - `TestE2EStaleVerdictBlocksApply`: after a passing run, rewriting `runs/<id>/task.md` makes `apply` exit 3 with `changed: contract`; `review` prints `STALE`.
  - `TestE2EDeclaredClaudeRunCarriesSentence`: the declared Claude path against the fixture native (`installFakeClaude`, as `TestE2EDeclaredUnaffected`, `tests/e2e/qualification_test.go:266`) exits 0; its receipt's `billing.entitlement_statement`, the plain `review` entitlement line and the run stream's admission block each equal `entitlement: user-declared, NOT verified by MYTHHELM; paid continuation: unknown (user declares disabled)` (red against main, which prints it nowhere user-facing: issue #72). Skipped on Windows for the reason `TestE2EDeclaredUnaffected` gives.
  - `TestE2EScriptedRunEntitlementNotApplicable`: the scripted run's receipt carries the not-applicable form.
  - `TestE2EStrictStillBlocked` (existing, `tests/e2e/qualification_test.go:155`) keeps passing: it is the AC-4.3 blocked-path pin and is not red before this task.
- **Test plan**: `newEnv`/`mkRepo`/`run` helpers of the existing e2e package; builds `cmd/mythhelm` once.
- **Invariants touched**: I13 (v2 §2: the flow needs no agent credentials); I07, I09 (v2 §2: observed end to end on the real binary).

### Task 12 — User documentation and decision record

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 11
- **Change**: Document acceptance binding, the NOT RUN and STALE outcomes, the determinism note, the bounds table and the entitlement sentence in the user guide, update the limitations register (a new `## Acceptance limits` section), and complete the decision record Task 2 created with the consequences the later tasks settled.
- **Files**:
  - `docs/user-guide.md` (also the migration section's backup name `mythhelm.db.bak-migration-v7` at `docs/user-guide.md:230`, which becomes `-v8`; check: `grep -n "bak-migration-v7" docs/user-guide.md` prints nothing)
  - `docs/limitations.md`
  - `docs/decisions/0019-protected-acceptance.md`
- **Acceptance**:
  - `sed -n '/^## Acceptance/,/^## /p' docs/user-guide.md | grep -q "NOT RUN"` and the same section containing `STALE`, `entitlement: user-declared, NOT verified by MYTHHELM; paid continuation: unknown (user declares disabled)` and `determinism`: each exits non-zero before the change (no section) and 0 after.
  - Every output example pasted into the guide names the executed command that produced it (the `TestE2E` run or `mythhelm review` invocation) in a comment line above it, and the worker re-runs it before opening the pull request.
  - `sed -n '/^## Consequences/,$p' docs/decisions/0019-protected-acceptance.md | grep -q "TestUnavailableChecksPreserveUnverifiedCandidate"` and the same for `TestApplyRefusesUnboundVerified`: Task 2 cannot name tests that do not exist yet, so both exit non-zero before this task.
  - `sed -n '/^## Acceptance limits/,/^## /p' docs/limitations.md | grep -q "unknown"` and the same section states the unprotected test-file residual and the unknown tool-version identity; exits non-zero before.
- **Test plan**: Recorded shell commands with their failing counterfactual; `scripts/ci/check-public-hygiene.sh`.
- **Invariants touched**: I14 (v2 §2: only evidenced capability is advertised; the residuals are written down); I09 (v2 §2: docs word unknowns as unknown).

### Task 13 — Qualified-route end-to-end (blocked-on MH-12 Task 5)

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 11
- **Blocked**: `specs/*/claude-strict-subscription/` Task 5 is not merged (it is held on that spec's Q1); this task starts when it merges. Blocked-on: MH-12 Task 5.
- **Change**: Prove, from the packaged binary against a test-seeded live record, that a strict `subscription-only` run completes the P1 flow with the verified entitlement form in its receipt, and that an unqualified route blocks with the registry reason.
- **Files**:
  - `tests/e2e/acceptance_strict_test.go`
- **Acceptance**:
  - `TestE2EStrictP1CarriesVerifiedEntitlement`: the seeded-live strict run reaches `ready_for_review`, its receipt has `billing.qualified == true`, `entitlement_statement` starting `entitlement: verified by MYTHHELM`, and `acceptance.policy_pin.policy_id == "P1"`; the declared-posture run in the same test carries the declared sentence instead (the two differ).
  - `TestE2EUnqualifiedRouteBlocksWithRegistryReason` (red against a variant of admission that silently falls back to the declared route when the strict consult blocks): absent, fixture-only and drifted records each exit 3 with their registry reason (`no_qualification_record`, `entitlement_not_proven`, `qualification_drifted`) in JSONL, and no run row reaches `executing`; no silent fallback to declared.
- **Test plan**: Reuses MH-12 Task 5's seeding helpers from `tests/e2e/qualification_strict_test.go`; names are distinct from its tests.
- **Invariants touched**: I15 (v2 §2: strict admission still needs evidence independent of sign-in); I02 (v2 §2: unknown mandatory entitlement blocks).

### Task 14 — Live qualified-route evidence (maintainer; blocked-on MH-12 Task 7)

- **Domain/agent**: maintainer
- **Budget**: standard
- **Depends on**: Task 12, Task 13
- **Blocked**: `specs/*/claude-strict-subscription/` Task 7 (maintainer live qualification) is not completed. Blocked-on: MH-12 Task 7.
- **Change**: The maintainer runs one qualified-route P1 delivery under the explicit allowance grant and commits a sanitized record of its receipt, so the AC-4.3 claim has authorised live evidence.
- **Files**:
  - `docs/dogfood/0002-protected-acceptance-live.md`
- **Acceptance**:
  - The record contains the run's receipt fields `billing.entitlement_statement`, `verification.label`, `verification.binding` digests and `acceptance.policy_pin`, pasted from the maintainer's run with the command line used.
  - `scripts/ci/check-public-hygiene.sh` passes and `grep -riE "token|secret|password|bearer|/home/" docs/dogfood/0002-protected-acceptance-live.md` exits non-zero.
  - The completion entry names the grant and the route version observed.
- **Test plan**: Maintainer-executed; nothing in CI depends on it.
- **Invariants touched**: I04 (v2 §2: explicit allowance grant for the run); I19 (v2 §2: sanitized record only, no secret material).
