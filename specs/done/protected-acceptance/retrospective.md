# protected-acceptance — Retrospective

## Review Summary

- **Range**: 8f2e8f7..cbec60d (PRs #398, #400, #405, #413, #414, #416, #417, #415, #424, #425, #426, #427; fix PRs none)
- **Reviewer**: code-reviewer, architect (dispatched=2 returned=2 failed=0)
- **Findings**: critical 0, important 6, suggestion 14 (confirmed, after merging duplicates between the two reviewers); rejected 0. Every important finding was re-opened at its anchor on head; of the suggestions three were re-opened (dead `Hooks.Timeouts` fields, the unsanitised entitlement line, the `--accept-unverified` override of a rejected verdict) and the rest stand as reported with their anchors.
- **Open critical**: 0
- **Vendor review**: skipped (Codex, Muse and Jev are disabled)

| Severity | Finding | Anchor | Disposition |
|---|---|---|---|
| important | A cancellation during a running check never reaches the cancellation outcome and can strand the run in `verifying` (I09, I06/I12) | `internal/integration/verify.go:273-286`, `internal/supervisor/acceptance.go:194-205` | issue #437 |
| important | The receipt gives a false reason for `billing.reserve.verify_*` and `acceptance.bounds.check_timeouts` (I09, AC-3.4) | `internal/supervisor/unknown_reasons.go:75-76`, `:108`; `internal/supervisor/receipt.go:469-482` | issue #438 |
| important | Live `run --accessible` never shows a declared run's entitlement sentence in full (AC-3.1, I15) | `internal/cli/accessible.go:212-231`, `:399-401`; `internal/cli/render.go:328-341` | issue #439; the user guide's wording corrected in this record |
| important | `review --format jsonl` cannot show a stale verdict (I07, I09) | `internal/cli/review.go:85-88` | issue #440; recorded in `docs/limitations.md` in this record; related to #393 |
| important | The receipt calls the check environment "pinned", but it differs between a live and a recovered run (I09, AC-4.1) | `internal/acceptance/acceptance.go:72`, `internal/supervisor/recover.go:460` | issue #441 |
| important | Recovery after a failed record step strands a run on the skip and runner-fault paths (I06, I12) | `internal/supervisor/recover.go:277-279`, `:316-318` | issue #442 |
| suggestion | The stream's entitlement line is not terminal-sanitised | `internal/cli/render.go:339` | kept |
| suggestion | `Hooks.Timeouts.Render` and `.Gate` are merged but never read | `internal/supervisor/acceptance.go:130-135` | kept (dead code) |
| suggestion | `--accept-unverified` can override a rejected (failed or invalid) verdict, against D23; only the state check prevents it | `internal/supervisor/apply.go:72`, `:168-173` | kept; Task 6 recorded the deviation |
| suggestion | The TUI keeps its own result-label switch and prints an unknown value raw; the CLI and TUI copies of `reasonOrUnknown` differ | `internal/tui/mission.go:262-280`, `internal/cli/render.go:265` | kept (relates to #393) |
| suggestion | The gate trusts the mutable `verifications.result` column, which has no immutability trigger (hardening: derive "passed" from the immutable attestation) | `internal/supervisor/acceptance.go:374-396` | kept |
| suggestion | The attestation body and environment fingerprint live only in `verification_bindings`; the journal event carries only the digest | `internal/supervisor/acceptance.go:246-257` | kept (I23) |
| suggestion | `entitlement.go` hard-codes "(user declares disabled)" instead of reading the declaration | `internal/admission/entitlement.go:18-19` | kept |
| suggestion | P1 takes its schema version from `v2contract.SchemaVersion`; a bump fails every admission, guarded only by `TestP1DigestPinned` | `internal/acceptance/acceptance.go:80` | kept |
| suggestion | Two exit-code tables (`receiptExit`, `runExit`) that this range extends in both | `internal/supervisor/receipt.go:524-546`, `internal/cli/exit.go:104-135` | kept |
| suggestion | Duplicated helpers: the skip predicate and the bounded task read | `internal/supervisor/acceptance.go:54`, `pipeline.go:1158-1163`, `acceptance.go:455-469`, `recover.go:431-441` | kept |
| suggestion | No test asserts that a declared run carries `acceptance.policy_pin` P1 (AC-4.3 declared leg) | `tests/e2e/acceptance_test.go:266-302` | kept |
| suggestion | Any non-timeout `ReadPin` error is recorded permanently as "acceptance pin unreadable" | `internal/supervisor/receipt.go:353-355` | kept |
| suggestion | Unrecorded design divergences: the runner-error path uses `Decide`, not `DecideStatuses`; review prints `rejected (<reason>)` where design §2.7 says `<verdict>` | `internal/supervisor/acceptance.go:215-218`, `internal/cli/review.go:141` | kept; behaviour fails closed |
| suggestion | The JSONL section of the user guide gives a count and a Go function name, not the 29 type names | `docs/user-guide.md` (JSONL section) | kept |
| suggestion | Docs drift left by the tasks: `policy_pin` does not list check timeouts, `binding_unavailable` also covers bind errors, three endings were missing from the verdict section and the exit table | `docs/user-guide.md` | amended in finalize |
| suggestion | ADR 0019 omits the `acceptance_pin_unreadable` and `acceptance_pin_failed` outcomes, the `verification.completed` payload change and its effect on ADR 0005 | `docs/decisions/0019-protected-acceptance.md` | amended in finalize |
| suggestion | Validation findings 7 (a v2 §11.3 review-contents honesty row) and 9 (I21 on Task 7) from the spec's validation were never acted on | `specs/done/protected-acceptance/scratchpad.md` | kept |

Decision-record re-verification: ADR 0019 was `proposed` at head. Every file it cites exists and every test it names is declared in a `_test.go` file (checked on origin/main). The maintainer accepted it on 2026-10-12, so this record sets `Status: accepted`; the drift above is amended in the same change. ADR 0005's line that unavailable checks end `failed` is superseded by ADR 0019 and noted there; ADR 0005 itself is untouched.

Foreign changes in range: 2e0605c (#422, supervisor test parallelism: `acceptance_test.go`, `apply_test.go`, `faults_test.go`, `receipt_test.go`), 475722f (#404, docs: `docs/limitations.md`, `docs/user-guide.md`), 6249f0f (#403, Codex limits: `docs/limitations.md`), 0afb230 (#397, Codex registration: `internal/supervisor/pipeline.go`). No finding is drawn against this spec from them.

## Acceptance

| Criterion | Result | Evidence |
|---|---|---|
| AC-1.1 verdict bound to candidate, contract, check definitions, environment | met | `TestVerdictBoundToCandidateContractChecksEnvironment`, `TestEnvironmentIDStableAndValueFree`; the environment is recorded, not pinned (#441) |
| AC-1.2 a stale verdict is not valid | met | `TestApplyRefusesStaleVerdict`, `TestE2EStaleVerdictBlocksApply`, `TestReviewPlainShowsAcceptanceStatus`; TUI, accessible and JSONL views do not show it (#393, #440) |
| AC-1.3 unavailable or waived checks keep the candidate, labelled | met | `TestUnavailableChecksPreserveUnverifiedCandidate`, `TestSkipPathsRecordReason`, `TestE2EUnavailableChecksKeepCandidate` |
| AC-2.1 candidate edits and task text cannot change the checks | met | `TestEvaluatorChangesWithCheckDefinition`, `TestTaskBytesNeverReachArgv` |
| AC-2.2 an evaluator mismatch is `INVALID` | met | `TestEvaluatorDigestMismatchInvalid` |
| AC-2.3 the run records its policy pin P1 | met | `TestAdmissionJournalsPin`, `TestPolicyRowSeededOnceAcrossRuns`, `TestReceiptPolicyPinAndDeterminism` |
| AC-3.1 declared runs carry the verbatim sentence everywhere | partial | met in the receipt, plain review, the run stream, the TUI and `review --accessible` (`TestEntitlementStatementVerbatim`, `TestE2EDeclaredClaudeRunCarriesSentence`); not met in the live `run --accessible` stream (#439); the qualified form is unit-tested only |
| AC-3.2 an unverified run is `NOT RUN` with its reason | met | `TestReceiptVerificationLabelAndBinding`, `TestReviewPlainShowsEntitlementAndVerification`, `TestMissionNotRunShowsReason` |
| AC-3.3 the JSONL stream emits only documented events | met | `TestStreamEventSetPinned`, `TestEveryEmittedEventTypeIsDocumented`, `TestJSONLRefusesUndocumentedEvent` |
| AC-3.4 unknown fields print `unknown` with a reason | partial | `TestEveryUnknownHasAReason`, `TestReceiptUnknownNeverZero` pass, but two reasons are false (#438) |
| AC-4.1 the flow is deterministic from its pinned inputs | met | `TestSameInputsSameVerdict`; the environment differs on a recovered run (#441) |
| AC-4.2 the P1 flow needs no credentials | met | `TestE2EP1FlowNoCredentials` |
| AC-4.3 qualified admission, no silent fallback | partial | blocked path pinned by `TestE2EStrictStillBlocked`; the positive path (Task 13) and live evidence (Task 14) are carried by #429, blocked on MH-12; the declared leg has no test (suggestion above) |
| AC-4.4 current bounds recorded, enforcement flagged | met | `TestReceiptBoundsEnforcedFlags` (#328 stands) |
| NFR-1 every acceptance step has a stated timeout | met | `TestBindFailureJournalsUnavailable`, `TestRecordStepHonoursTimeout`, `TestBuildReceiptWithinExpires`, `TestCheckAcceptanceWithinExpires`, `TestOverheadRecorded` |
| NFR-2 tests need no agent credentials | met | `TestE2EP1FlowNoCredentials`, credential-free e2e environment |

## Deviations

- Tasks 5, 6, 7: the "within the limit" tests use an already-expired limit (-1 ns or -1 s) instead of 1 ns, because the coarse Windows clock does not expire a 1 ns deadline — recorded in each task entry; no impact beyond the tests.
- Task 2: three mechanical edits outside the Files list (`internal/migrate/SUPPORT.md` and the migrate tests for the schema-8 rename) — recorded in the task entry.
- Task 4: two tests added beside the named ones for the contained and inspect branches — recorded.
- Task 8: `TestClaudeRateLimitStreamsDocumented` keeps no in-test mutant leg because a predicate built from the stream's own types was true by construction — recorded.
- Task 9: `internal/tui/model.go` is outside the Files list for the extra strip row's height accounting — recorded.
- Task 10: the entitlement sentence is its own `--accessible` summary line (the 200-cell cap would cut it) — recorded; the live stream's event line still cuts it (#439) — found by review.
- Task 11: the acceptance's "red against main before Task 1" was shown with production mutants instead of against the pre-Task-1 tree — recorded; a spec-text correction by the coordinator for Task 10's bullet.
- Task 12: ADR 0019 kept `Status: proposed` because accepting it is the maintainer's call — recorded; the maintainer accepted it on 2026-10-12 and this record flips it.
- Tasks 13 and 14: carved out to #429 by the maintainer's decision of 2026-10-12 (PR #430); the spec closes with 12 of 14 tasks.
- Found by review, not recorded by a task: the runner-error path uses `Decide`, not `DecideStatuses`; review prints `rejected (<reason>)`; the receipt's `unknown` reason for check timeouts went stale after Task 4 (#438); the environment is described as pinned (#441).

## CI history

- CI/Go (windows-latest) and Go (windows-11-arm) on PR #405, runs 38182985390 and 38188078803: real, `TestUnavailableCheckIsNotFailed/plus_a_failing_check` returned `unavailable`, wanted `failed`. Deterministic on both runners and both heads; the failing-check fixture needed `/bin/sh`, which Windows lacks, so the "failing" check could not start. Fixed by 0d8f0f8 (fixture without `/bin/sh`), green on the next head.
- CI/Go on PR #414, run 38194229707: real, `TestRecordStepHonoursTimeout` got a nil error, wanted `context.DeadlineExceeded`, on both Windows runners. Mechanism: a 1 ns deadline does not expire on the coarse Windows clock (Task 5 deviation); fixed by 1d09e02 (-1 ns). The same run's windows-11-arm job also failed `TestWatchWritesSupervisorBeatAfterIngest`, outside this spec (see below).
- CI/Go (windows-latest) on PR #417, run 38198740245, and PR #424, run 38204768259: `TestReplanPreservesRepairSlots` (outside this spec, issue #419, fixed by #421). Mechanism: the test's replan deadline derived from a package-level start time, so it expires once the supervisor test binary has run about 9m45s (the figure from #419); the failing log reports `replan deadline 2026-10-12T03:24:45Z passed` (run 38198740245), and a run whose binary finishes earlier passes. Run 38198740245 also ended `panic: test timed out after 10m0s`.
- CI/Go (windows-latest) on PR #426, run 38213174301: `panic: test timed out after 10m0s` in the supervisor package. Mechanism (issue #420, fixed by #422): the package's Windows test time sat about 80 s below the 10 minute `go test` timeout, so a slow runner crosses it; failing value 10m0s, the later passing run on e7ad881 finished within the limit (its duration: unknown).
- CI/Go (windows-11-arm) on main merges of #400 (795afdc) and #416 (7e8fde8), runs 38187673222 and 38200138962: `TestWatchWritesSupervisorBeatAfterIngest` could not open `supervisor.beat` ("The process cannot access the file"). Outside this spec (issue #411, closed). Mechanism: the test's poll races the beat's atomic replace on Windows; failing value an open error 0.2 s into the test, passing value an open that lands between replaces.
- CI/Go (windows-11-arm) on the main merge of #398 (c46b9af), run 38178324934: the job failed with no failing step recorded. Class unknown: not reproduced, and the PR head had been green. No mechanism is known.
- CI/Go (windows-11-arm) on the carve-out PR #430, run 38223898738 (no Go change in the PR): real, `TestE2EAccessibleStream/run_streams_live_with_the_outcome_exit` failed with `viewmodel: admitted task digest mismatch`. Mechanism (issue #433, fixed by #434): the supervisor wrote `task.md` non-atomically and the live viewer, polling during the run, read it mid-write; failing value a digest of a partial file, passing after the atomic write on c2471ac. No headSha has both a failure and a success, so no failure here is a retry that went green.

## Effort

dispatched=12 returned=12 failed=0; attempts 12 over 12 tasks (the run-events log records one attempt per task); first-pass unknown; review rounds unknown (the log records 0, but GitHub shows submitted reviews on #400 (10), #405 (6), #414 (3), #415 (6), #426 (1) and #427 (4)); wall-clock 2026-10-11T20:48:22Z (first `run_start`) → 2026-10-12T07:53:43Z (#427 merged, from GitHub).
The `runspec.py summary` table shows `Merged: no` and `First pass: no` for all 12 tasks, because the log holds no `merge` rows (the maintainer merged outside `/run-spec`); every task's merge is confirmed on origin/main by `finalize.py verify` (12 tasks, last merge cbec60d).

| Task | Agent | Attempts | Review rounds | PR | Merged | First pass |
|---|---|---|---|---|---|---|
| 1 | go-implementer | 1 | 0 | #398 | no | no |
| 2 | go-implementer | 1 | 0 | #400 | no | no |
| 3 | go-implementer | 1 | 0 | #405 | no | no |
| 4 | go-implementer | 1 | 0 | #413 | no | no |
| 5 | go-implementer | 1 | 0 | #414 | no | no |
| 6 | go-implementer | 1 | 0 | #416 | no | no |
| 7 | go-implementer | 1 | 0 | #417 | no | no |
| 8 | go-implementer | 1 | 0 | #415 | no | no |
| 9 | tui-implementer | 1 | 0 | #424 | no | no |
| 10 | go-implementer | 1 | 0 | #425 | no | no |
| 11 | go-implementer | 1 | 0 | #426 | no | no |
| 12 | go-implementer | 1 | 0 | #427 | no | no |

## Lessons

- What worked: the interface hand-offs between tasks (`ReadPin`, the binding row, the receipt keys) held across the combined change; the reviewers found no seam mismatch in them. Recording each Windows-clock deviation in the first task that hit it (Task 5) let Tasks 6 and 7 avoid the same failure.
- What worked: Task 11 proved the whole flow from the packaged binary, and Task 12 re-ran every pasted output block, so the docs matched the code on the main paths.
- What to change: all six failed CI runs on task PRs failed only on the Windows runners (a shell fixture twice, a 1 ns deadline, a package-start clock twice, a test-binary timeout), after the task's gates had passed on Linux; main merges added a beat-file race and the later `task.md` write race. The tasks' gates run `GOOS=windows go vet`, which cannot find them. See P-protected-acceptance-1.
- What to change: `TestEveryUnknownHasAReason` checks that each `unknown` has a reason, not that the reason is true, so a reason made false by Task 4 survived Task 7's review and every later one (#438). See P-protected-acceptance-2.
- What to change: the run-events log has no `merge` rows when the maintainer merges, so the retrospective's effort numbers read `no` for tasks that merged. See P-protected-acceptance-3.
- What to change: the three views of one fact (TUI, accessible stream, JSONL) each decide the verified label on their own, and #393, #439 and #440 are the same gap in three places. A shared read of the acceptance state is the follow-up design.

## Proposals

- P-protected-acceptance-1 — Name the Windows-only test hazards in the test-quality rule
- P-protected-acceptance-2 — A table of reasons is tested for truth, not only for presence
- P-protected-acceptance-3 — Retrospective effort says `unknown` when the log holds no merge rows
