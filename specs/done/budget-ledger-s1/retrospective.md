# budget-ledger-s1 — Retrospective

## Review Summary

- **Range**: 85049b2..312d3ff (PRs #240, #245, #246, #252, #254, #256, #259, #263, #270, #288, #293, #295; fix PR #317)
- **Reviewer**: code-reviewer, architect (dispatched=2 returned=2 failed=0)
- **Findings**: critical 1, important 11, suggestion 11 (confirmed); rejected 0
- **Open critical**: 0
- **Vendor review**: skipped (optional, not run)

| Severity | Finding | Anchor | Disposition |
|---|---|---|---|
| critical | A recovered (reattached) run had no execution deadline: `RecoverWithHooks` built the pipeline without `deadline` or `ceilings`, so `watch` armed no timer; I21 and AC-4.1 broke once the launching supervisor died. Re-adjudicated on the new head: `recoverDeadline` runs before `watch` on every worker-running recovery mode and uses the launch path's `effectiveCeilings` and `envelopeDeadline`; `TestRecoverArmsExecutionDeadline` pins it. | `internal/supervisor/recover.go:229`, `internal/supervisor/recover.go:261`, `internal/supervisor/faults_test.go:222` | fixed by #317 |
| important | Two blocked-at-launch paths return an error after the run is already `blocked`, so `runExit` is skipped and the exit is 1 internal instead of 3 (design §9); the receipt says 3. | `internal/supervisor/pipeline.go:369`, `internal/supervisor/pipeline.go:409`, `internal/cli/run.go:231`, `internal/cli/exit.go:73` | issue #325 |
| important | A run refused at admission loses its retry schedule display (`next retry: none (used 0 of 3)`): `RunBucket` finds the bucket through the reservation row the refused hold rolled back (I09). | `internal/supervisor/receipt.go:281`, `internal/cli/run.go:299` | issue #326 |
| important | One exhaustion locks the bucket out for good, and successful runs consume the retry schedule: the row clears only after an authoritative reset passes, the S1 route never records one, and no command clears it. Open AC-6.2 against AC-7.2 question (Q-20). The user guide now states the permanence (finalize PR). | `internal/admission/reserve.go:155`, `internal/supervisor/pipeline.go:391`, `internal/journal/ledger.go:268`, `docs/user-guide.md:194` | issue #327 (awaiting maintainer decision, Q-20); guide text amended in the finalize PR |
| important | Only the execution deadline binds a live run: each run launches one attempt, and the repair, replan and transport-retry ceilings and the completion-reserve gate are checked at launch only, so they are unreachable in production. The user guide presented them as active. | `internal/supervisor/envelope.go:178`, `internal/supervisor/pipeline.go:488`, `internal/supervisor/ingest.go:229`, `docs/user-guide.md:170` | issue #328; guide and `docs/limitations.md` amended in the finalize PR |
| important | No command can record an extension (`Hooks.Extension` has no producer) and `blocked` is terminal in recovery, so AC-4.2's "extension by recorded decision" cannot happen; the guide said it is supported. | `internal/cli/recover.go:51`, `internal/supervisor/recover.go:87`, `docs/user-guide.md:174` | issue #329; guide amended in the finalize PR |
| important | Reservations of runs that end at `blockLaunch`, `stopBeforeSpawn`, a failed preflight, a pin failure or a spawn failure stay `held` forever with no release evidence (release only in `conclude`). | `internal/supervisor/pipeline.go:305`, `internal/supervisor/pipeline.go:409`, `internal/supervisor/pipeline.go:1054` | issue #330 |
| important | Ledger SQL lives outside `internal/journal`, and the run-scoped reservation queries ignore the writer: the service control plane also writes `reservations` (foreign #258), so release, orphan and `held[0]` can pick another writer's row. | `internal/admission/reserve.go:112`, `internal/admission/reserve.go:141`, `internal/supervisor/exhaustion.go:103`, `internal/control/reserve.go:78` | issue #331 |
| important | The exhaustion signal is a synthetic fixture and the user guide and `docs/limitations.md` did not say so (I14). | `docs/user-guide.md:190`, `adapters/claudecode/decode.go:41`, `adapters/claudecode/testdata/exhaustion/allowance_exhausted.json:2` | fixed in the finalize PR (guide and limitations amended) |
| important | AT-13 delta is not exercised end to end and `usage-counters.json` carries fields no decoder reads. Recorded by Task 11 deviation 1; the follow-up had no owner. | `adapters/fake/scenarios/usage-counters.json:5`, `tests/e2e/ledger_s1_test.go:143` | issue #332 |
| important | New. A deadline stop that is interrupted concludes as `cancelled`, not `blocked (envelope_deadline_exceeded)`: the cause lives in memory (`deadlineHit`) and the stop is journaled with the user requester, so a fresh recovery pipeline cannot tell (AC-4.2, I06). Medium confidence; no test covers it. | `internal/supervisor/pipeline.go:1006`, `internal/supervisor/pipeline.go:400`, `internal/supervisor/recover.go:77` | issue #333 |
| important | New. #317's error path abandons the run: with no `run_envelopes` row (a run admitted before schema v3) `recoverDeadline` fails after the run is `executing`, leaving the worker unwatched and blocking every later `run` with `active_run_exists`; a NULL `first_start_at` yields a zero deadline. Not critical: the row exists for every run admitted since the envelope landed, and the failure is loud. | `internal/supervisor/recover.go:194`, `internal/supervisor/recover.go:229`, `internal/supervisor/recover.go:262`, `internal/supervisor/envelope.go:305` | issue #334 |
| suggestion | `[envelopes]` in `mythhelm.toml` is ignored on `--no-checks` runs. Matches design §5 (the table is digest-bound by the trust path); only the guide was wrong. Lowered from important by the reviewer. | `internal/admission/trust.go:50` | fixed in the finalize PR (guide amended) |
| suggestion | The `retail-equivalent` scope string and the decimal pattern are each defined more than once. | `internal/billing/normalize.go:16`, `internal/supervisor/ingest.go:251`, `internal/supervisor/receipt.go:274` | kept |
| suggestion | The ledger notices swallow a ledger read error (`remaining: unknown` vanishes without a sign). | `internal/cli/run.go:244` | kept |
| suggestion | Exhaustion that coincides with a user or deadline stop releases the reservation and never records the bucket. | `internal/supervisor/pipeline.go:1056` | kept |
| suggestion | The slow variant of the latency test sleeps 1.5 s and asserts at least 1 s elapsed, so it cannot fail; NFR-1 is measured on a stand-in path. | `tests/e2e/ledger_s1_test.go:378` | kept |
| suggestion | `GateLaunch` is exported and called only by tests. | `internal/supervisor/envelope.go:149` | kept |
| suggestion | `SplitUsage` labels the combined total `reported` for unmapped routes. | `internal/billing/normalize.go:179` | kept |
| suggestion | `orphaned` is marked on claims whose owner is unresolved, not gone; ADR 0015 should define it before MH-21 adopts it. | `internal/supervisor/recover.go:185`, `internal/journal/ledger.go:252` | kept |
| suggestion | v2 §7.3 "normalization version" and v2 §4.1 reservation "generation" are absent from AC-1.1, AC-3.1 and the DDL. | `internal/journal/migrations/0003_ledger.sql` | kept |
| suggestion | Stale text: ADR 0015 D3 ("no service exists in S1", "every ledger write commits with the run event") and tasks.md and handoff.md saying the ADR is `proposed` where it is `accepted`. | `docs/decisions/0015-budget-ledger-s1.md:35`, `specs/done/budget-ledger-s1/tasks.md:340`, `specs/done/budget-ledger-s1/handoff.md:77` | fixed in the finalize PR |
| suggestion | A recovered replan does not re-derive the shorter replan deadline, so it would spend the verification reserve. Unreachable while each run has one attempt. | `internal/supervisor/pipeline.go:640` | kept |

Foreign changes in range: 20d1ccc supervised-stop-recover task 4 (#311): supervisor pipeline.go; a15cc99 this spec's lifecycle move (#300): spec files; 4cb2c4a supervised-stop-recover task 2 (#298): cli render, supervisor pipeline; 8334337 cep task 9 (#294): admission, supervisor pipeline and receipt tests; ac0f315 migration task 4 (#292): supervisor pipeline.go, recover.go; 443ecea migration task 2 (#286): journal.go; 148084b migration task 1 (#277): journal.go and tests; 96a216c cep task 8 (#278): admission.go, cli/run.go, supervisor receipt and pipeline tests; a45d0d9 this spec's fix (#276): envelope_test.go; b6447b0 cep task 7 (#272); 9bf9397 cep task 6 (#261); 74c3128 supervisor-service task 6 (#258): journal files; a8628c8 cep task 5 (#260); c49d5d3 supervisor-service task 5 (#255): journal files. The architect's observations on the control plane's own reservation handling (#258) belong to that spec and are not counted here.

## Acceptance

| Criterion | Result | Evidence |
|---|---|---|
| AC-1.1 usage rows with unit, scope, source, label | met | `TestNativeResultProjectsUsageRows`, 0003 CHECK constraints; v2's normalization version is not carried (suggestion) |
| AC-1.2 missing quantity is `unknown` | met | `TestNativeResultNullsProjectUnknown`, `TestNormalizeMissingScopeUnknown`, `TestReceiptUnknownNeverZero` |
| AC-1.3 no aggregate of unlike buckets | met | `TestNormalizeSameScopeDistinctIdentities` |
| AC-1.4 retail estimates exact and labelled | met | `TestNormalizeDecimalExact`, `TestReceiptCarriesTypedUsage` |
| AC-2.1 cumulative and delta matching | partial | unit-tested only (`TestNormalizeDeltaAppliedOnce`, `TestNativeResultAttemptsAccumulate`); not exercised end to end (issue #332) |
| AC-2.2 missing scope marked `unknown` | met | `TestNormalizeMissingScopeUnknown` |
| AC-2.3 component totals only where mapped | partial | `SplitUsage` unit-tested; components never stored or shown (Task 3 deviation 2) |
| AC-3.1 typed reservation coupled to the bucket | met | `TestHoldCouplesAdmittedBucket`, `TestE2ECoupledReservationPresent` |
| AC-3.2 transactional acquisition, no partial claim | met | `TestFailedAdmissionHoldsNothing` |
| AC-3.3 reservation never presented as provider availability | met | `TestReservationWordingIsLocalOnly` |
| AC-3.4 release only after stop; no reclaim before reconcile | met | `TestReleaseOnlyAfterStop`, `TestExpiredReservationOrphanedNeverReused`; early-end paths never release (issue #330) |
| AC-4.1 finite ceilings on every run | partial | execution time enforced at launch and, after #317, on recovery (`TestRecoverArmsExecutionDeadline`); the other three ceilings never bind a live run (issue #328) |
| AC-4.2 blocked with a specific reason; extension by recorded decision | partial | function level (`TestGateRefuses*`, `TestExtensionGrantsRecordedDecision`); no producer of the decision (issue #329); an interrupted deadline stop ends `cancelled` (issue #333) |
| AC-4.3 only quiescent pause time excluded | partial | `TestDeadlineExcludesOnlyQuiescentPause`; no pause control exists |
| AC-5.1 reserve before optional work | partial | `TestShortfallBlocksOptionalWork`, `TestReplanDeadlineLeavesReserve`; production never dispatches a replan (issue #328) |
| AC-5.2 reserve shown as an estimate | met | `TestReceiptReserveIsEstimateOnly`, `TestReserveNeverHardTokenClaim`, `TestCliUsageNotices` |
| AC-6.1 preserve work, stop admitting on the bucket | met | `TestExhaustionPreservesCandidates`, `TestBucketRefusesNewWork`, `TestE2EExhaustionPreservesWithoutFallback`; the signal is a synthetic fixture (documented), and a refused run exits 1 (issue #325) |
| AC-6.2 up to 3 retries, then stop | partial | awaiting maintainer decision (Q-20): the schedule is pinned (`TestRetryScheduleResetAndUnknown`, `TestSpentScheduleGivesUp`, `TestPassedResetClearsSpentBucket`), but one exhaustion refuses its bucket permanently because S1 records no reset (issue #327); a refused run's display is wrong (issue #326) |
| AC-6.3 never escalate to paid routes | met | `TestNoSecondNativeLaunch`, `TestE2EExhaustionPreservesWithoutFallback` |
| AC-6.4 re-admission before any alternative route | met | `TestOneBucketPerRun` |
| AC-7.1 unknown remaining shown as `unknown` | met | `TestReceiptUnknownNeverZero`, `TestCliUsageNotices`; a refused run shows `used 0 of 3` (issue #326) |
| AC-7.2 unknown quota alone does not block a qualified route | met | `TestUnknownQuotaAdmitsStopAtExhaustion`; in tension with AC-6.2 (Q-20) |
| NFR-1 ledger operations within 1 s | partial | `TestRunPathLedgerLatency`; its discriminating half cannot fail (suggestion) |
| NFR-2 additive migration in `mythhelm.db` | met | `TestMigration0003CreatesLedgerTables`, `TestMigration0001Untouched` |
| Definition of done | partial | the AT-13 delta end to end is unmet (issue #332); the hard-limit greps return 0 hits and no TUI file changed |

## Deviations

- Task 3: journal tests outside Files hard-coded schema version 2; per-model token rows carry the combined total only, so mapped components are not persisted; a non-decimal cost is treated as unreported — recorded in the task entry; components never shown (AC-2.3 partial).
- Task 4: `Reserve` takes the run and the caller's transaction and `NewJournalReserver` drops the journal; the duplicate-hold check is a `COUNT` in `reserve.go` — recorded; the other raw reservation SQL outside `internal/journal` was not recorded (issue #331).
- Task 6: `Hooks.Extension` lives in `pipeline.go`, outside Files — recorded; the hook has no CLI producer, found by review (issue #329).
- Task 7: one attempt per run, so repair and replan refusals are tested on `GateLaunch`; the execution ceiling is stored in whole seconds — recorded; the transport ceiling's non-enforcement was not recorded (issue #328).
- Task 8: `checkBucket` clears the row only when an authoritative reset passed, not when retries are spent (review round 1, round 2) — recorded; the permanent-lockout consequence was found by review (issue #327).
- Task 9: `EstimateReserve` returns an error on overflow; the `reserveStart` fixture floats with the test clock; the replan deadline rides the commit path — recorded.
- Task 10: the receipt's verify pass is exact only for `--no-checks` runs; lines emit from `executeRun`; the full-screen TUI and accessible stream do not render the ledger lines — recorded.
- Task 11: per-attempt deltas are not exercised end to end — recorded, follow-up unowned (issue #332).
- Task 12: ADR 0015 landed `proposed` and was accepted by the maintainer afterwards; tasks.md and handoff.md corrected in the finalize PR.
- Found by review, recorded by no task: design §9 exit 3 for a refused launch (issue #325); design §4 release only at `conclude` (issue #330); design §5 recover-driven extension (issue #329); the recovery path's deadline, fixed by #317.

## CI history

- CI/Go (windows-11-arm) and CI/CI OK on merge 56ff412 (PR #270, Task 9): real, `TestFirstLaunchStartsClockOnce` failed with `envelope_deadline_exceeded`. Mechanism: the test launched at a fixed 2026-10-09T12:00Z start with a 600 s ceiling but committed with the real clock; failing value: the real clock at the failure was 12:10:18Z against the 12:10:00Z deadline in the error; passing value: a real clock before 12:10:00Z, which is why the task PR's CI was green. Fixed by #276, which floats the start with the test clock. Deterministic after the cutoff, so not nondeterminism of the code under test.
- CI, PR title, zizmor and OSV-Scanner on the task PR branches (#240, #245, #246, #252, #254, #256, #259, #263, #270, #288, #293, #295): runs with conclusion `cancelled` only, each on a head SHA replaced by a later push; no failed run on any task branch. Class: superseded, not a failure.
- Main at 312d3ff (fix PR #317 merged): `finalize.py ci` verdict=green runs=7; main at 97f00dd (tip when verified): verdict=green runs=7.
- Fix PR #317: the author recorded `internal/cli` `TestStrictMainBlocksWriteNothing` failing on unmodified main with a real `$HOME` native config (`untrusted_native_config`) and passing with a clean `$HOME`. Class: unknown (not reproduced here); mechanism claimed by the author: the test reads the developer's real home; failing value: real `$HOME`, passing value: clean `$HOME`. Left under Lessons.

## Effort

dispatched=12 returned=11 failed=0; attempts 12 over 12 tasks; first-pass 7/12 as the log records them; review rounds 3; wall-clock 2026-10-09T00:41:24Z → 2026-10-09T20:15:45Z (first `run_start`, last logged `merge`; the log has no `merge` row for tasks 2, 3, 4, 6, 7, so the summary shows them unmerged though origin/main holds their merges)

```text
dispatched=12 returned=11 failed=0
unaccounted=task 8 (1 dispatch)
unaccounted=task 9 (1 dispatch)
| Task | Agent | Attempts | Review rounds | PR | Merged | First pass |
|---|---|---|---|---|---|---|
| 1 | cloud-sonnet | 1 | 0 | #240 | yes | yes |
| 2 | cloud-sonnet | 1 | 1 | #246 | no | no |
| 3 | - | 1 | 0 | #252 | no | no |
| 4 | - | 1 | 0 | #256 | no | no |
| 5 | cloud-sonnet | 1 | 0 | #249 | yes | yes |
| 6 | - | 1 | 0 | #254 | no | no |
| 7 | - | 1 | 0 | #259 | no | no |
| 8 | - | 1 | 0 | #263 | yes | yes |
| 9 | - | 1 | 0 | #270 | yes | yes |
| 10 | - | 1 | 1 | #288 | yes | yes |
| 11 | go-implementer | 1 | 1 | #293 | yes | yes |
| 12 | - | 1 | 0 | #295 | yes | yes |
```

## Lessons

- What worked: red-first records and mutation checks in the task entries (Task 8 `TestSpentScheduleGivesUp`, Task 9 `TestCommitRefusesPastReplanDeadline`) caught real defects in review rounds; the whole-spec review caught the recovery-path critical that every per-task gate passed, and the fix PR (#317) landed with a red-first test before finalize.
- What to change: the user guide was written from the design and described four behaviours that no command reaches (proposal P-budget-ledger-s1-3); two tests pinned a wall-clock date against a real-clock commit path and one reached main red (P-budget-ledger-s1-1); a per-run guarantee was enforced on the launch path only (P-budget-ledger-s1-2).
- Open question for maintainers (not a finding against this spec): the execution deadline is enforced only while a supervisor is alive. Between a supervisor crash and `mythhelm recover`, and for quarantined attempts, the native runs unbounded; whether the honesty-register row "separate stop/reconciliation safety deadline" covers this is undecided.
- Open question Q-20: AC-6.2 (stop after three retries) against AC-7.2 and I02 (unknown quota alone must not block a qualified route); see issue #327.
- Environment note: `internal/cli` `TestStrictMainBlocksWriteNothing` depends on the developer's real `$HOME` native configuration, per the author of #317; not reproduced here.

## Proposals

- P-budget-ledger-s1-1 — Name fixed-date fixtures that meet a real-clock commit path
- P-budget-ledger-s1-2 — List every entry path of a per-run guarantee in its task
- P-budget-ledger-s1-3 — Document only behaviour a command can reach
