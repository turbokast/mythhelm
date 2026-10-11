# supervised-stop-recover — Retrospective

## Review Summary

- **Range**: 0ffc42d..8caedd4 (PRs #289, #296, #298, #311; fix PRs #336, #337, #340, #344, #350)
- **Reviewer**: code-reviewer, architect
- **Findings**: critical 5, important 18, suggestion 9 (confirmed); rejected 0
- **Open critical**: 0 (0 before the spec can ship)
- **Vendor review**: skipped (not requested)

Anchors are on origin/main 8caedd4. Two review rounds: the first at 82b6fd3 found the five criticals, the second re-adjudicated the new head (dispatched=2 returned=2 failed=0) and found no new critical. Main's push CI at 8caedd4 is green. Foreign changes are attributed below. Since #350 (maintainer decision Q-27, follow-up #348) S1 does not serve `stop` or `recover`: the control handlers are tested but have no production caller, so the criteria that only they meet are recorded as handler-level in the Acceptance table.

| Severity | Finding | Anchor | Disposition |
|---|---|---|---|
| critical | A confirmed ladder was recorded `stopped` and `confirmed:true` despite unresolved descendants or a failed scan (C1) | `internal/control/stop.go:505-506` | fixed in #336 (34d766c) |
| critical | `recover` quarantined a live identity-matched worker whenever its native was running (C2) | `internal/control/recover.go:841-843` | fixed in #336 (34d766c) |
| critical | Served `stop` and `recover` wrote attempt state for attempts a live legacy pipeline owns, without run ownership (C3) | `internal/control/runowner.go:20-33`, `internal/control/stop.go:108-112` | fixed in #336 (34d766c) for the concurrent case; the residual is the next row (#350) |
| critical | Served `stop` and `recover` set `attempts.state` as a projection with no journal event and no spool ingest; the next legacy owner ingests the worker's own spooled `stop_requested` from `stopped`, which the supervisor table forbids, so the run ends `interrupted/ingest_failed` and blocks new admissions (N1; I23, service contract 7) | `internal/control/server.go:178-187`, `internal/control/client.go:168-203` | fixed in #350 (8caedd4): S1 does not serve `stop` or `recover` (Q-27); journal-routed serving is #348 |
| critical | A served `stop` counted a stopped report spooled before the stop request as its answer, so an attempt that had already finished could be relabelled `stopped` with `confirmed:true` (N2; AC-1.2, I06, I09) | `internal/control/stop.go:420-428` | fixed in #344 (0cf615e), with run-id validation |
| important | A worker-reported unconfirmed ladder ends `interrupted/stop_unconfirmed` with a success result, not `cancel_incomplete` plus quarantine (AC-1.3; maintainer decision Q-25) | `internal/control/stop.go:305-308` | issue #355; AC-1.3 recorded partial |
| important | The production descendant scan `markedPIDs` skips an empty `/proc/<pid>/environ` and unreadable entries as clean, so a child exec'd just before a stop's single scan can be missed and the receipt reads `confirmed:true` (residual of the #337 probe fix) | `internal/workers/proc_linux.go:76-93`, `internal/workers/worker.go:935-951` | issue #354 |
| important | A descendant scan reported `unavailable` (Windows, other Unix) reads as resolved and the receipt drops the scan label (I09, AC-1.2) | `internal/control/stop.go:505-506`, `internal/workers/worker.go:936-938` | issue #357 |
| important | Recovery classifies "gone" by the worker PID alone and ignores the native group and unresolved PIDs of a prior stop; it accepts `stop_requested` (NFR-5, I24) | `internal/control/recover.go:872-897`, `internal/control/recover.go:391-398` | issue #358 |
| important | `recover` quarantines a live matched worker as `spool_unverifiable` when its unacknowledged spool exceeds 1 MiB (AC-2.1) | `internal/control/recover.go:845-847` | issue #359 |
| important | The reconnect handshake has no production caller and does not acknowledge the spool or bind the token to a generation (AC-3.3 met at function level only) | `internal/control/reconnect.go:85-98` | issue #360 |
| important | The exported `Reconcile` and `Reconnect` skip run ownership and run-id validation that the handlers take; wiring either reopens the C3 case (I18, I23) | `internal/control/reconnect.go:85-98`, `internal/control/recover.go:287-290` | issue #361 |
| important | The versioned ladder is not enforced in production: `Validate` has no non-test caller, the version is a label over adapter steps and the rung and deadline bounds never apply (AC-1.1, I20) | `internal/control/stopladder.go:25`, `internal/supervisor/pipeline.go:804` | issue #362 |
| important | `AwaitReconnect` is unbounded, and a fenced worker can park in it after a healthy run: the worker checks the beat once after `conclude`, the watch writes its last beat and returns, and the reap wait writes none, so a read more than 300 ms later reads stale (AC-3.1, I21) | `internal/workers/envelope.go:117-133`, `internal/workers/worker.go:543`, `internal/supervisor/pipeline.go:554-559` | issue #363 |
| important | `internal/control/SUPPORT.md` overclaims (I14): the `envelope-reconnect` row claims an uncalled handshake, no `blocked` row for Windows, and the CLI is said to run `stop` under run ownership although `mythhelm stop` takes no owner lock | `internal/control/SUPPORT.md:35`, `internal/control/SUPPORT.md:44-48` | issue #364 |
| important | Served `recover` with a nil launcher committed the continuation as `launching` before failing | `internal/control/recover.go:206-214` | resolved by #350: no served path, `client.go` no longer passes a nil launcher |
| important | A persisted stop can be stranded by a supervisor restart; only the same operation after `maxClaimAge` delivers it | `internal/control/stop.go:117-124`, `internal/control/execute_long.go:66-76` | issue #366; unreachable in S1 since #350 |
| important | A third hand-written journal-envelope writer skips the append path's checks | `internal/control/recover.go:1075-1121` | issue #365 |
| important | `TestHermeticNoCredentialsOrNetwork` proves its decoy with a copy of the walker and scans three files (NFR-6) | `internal/control/reconnect_test.go:417-449` | issue #367 |
| important | #336 changed process ownership (control intents hold the run owner lock; new `internal/ownerlock`) with no decision record, contradicting design D1 and D5 and ADR 0005 | `internal/ownerlock/ownerlock.go:25-40`, `specs/unfinalized/supervised-stop-recover/design.md:436` | issue #368 |
| important | After migration reaches `adopted`, legacy `recover` is refused and the service does not serve it, so migrated runs have no recover path that stream 4 was to provide | `internal/supervisor/recover.go:54`, `internal/supervisor/pipeline.go:245-250` | issue #369; stated in `docs/limitations.md` |
| important | Open items have no tracking issue (service-side beat writer, return detection, token delivery, continuation launcher) | `specs/unfinalized/supervised-stop-recover/tasks.md:166` | issue #370; #348 covers only served stop and recover |
| important | Q-27 is not recorded in the spec: requirements Definition of Done, `design.md` lines 53, 116, 283, 444-452, `tasks.md` lines 150-156 and `handoff.md` lines 41-44 still describe served stop and recover as shipped | `specs/unfinalized/supervised-stop-recover/requirements.md:68`, `specs/unfinalized/supervised-stop-recover/design.md:451` | recorded in this retrospective (Deviations and Acceptance); the spec text is kept as the record of the approved design |
| limitation | Maintainer decision Q-23: `stop` and `recover` authorise by same-user peer identity in S1 | `internal/control/stop.go`, `internal/control/recover.go` | documented in `docs/limitations.md`; follow-up issue #356 |
| suggestion | `CheckEnvelope` compares the envelope with itself | `internal/workers/envelope.go:118` | kept |
| suggestion | `StopReceipt.Quarantined` is never set and the deadline path records no receipt | `internal/control/stopladder.go:68` | kept |
| suggestion | Duplicated helpers and four spool readers with three oversize-line policies | `internal/control/stop.go:392-440`, `internal/control/recover.go:759-790` | kept |
| suggestion | Two assertions cannot fail | `internal/workers/envelope_test.go:326-328`, `internal/control/stop_test.go:352-357` | kept |
| suggestion | Sleeps synchronise tests | `internal/workers/envelope_test.go:222`, `internal/control/execute_long_test.go:99` | kept |
| suggestion | The stop deadline is read from the `stop.request` file, not a ledger-recorded deadline | `internal/control/stop.go:281-294` | kept |
| suggestion | Stale documentation: `design.md` lines 127, 149, 268, 310, 362-364, 388-391; the "served path" doc comments in `internal/control/stop.go:69-83` and `internal/control/recover.go:147-171` | `specs/unfinalized/supervised-stop-recover/design.md:362` | kept |
| suggestion | The stopped-report gate accepts any `stop_requested` of the attempt rather than the one for this request | `internal/control/stop.go:424-425` | kept; bind to the request id when #348 re-serves stop |
| suggestion | Production-dead code kept on purpose by Q-27: `StopHandler`, `RecoverHandler`, `Reconcile`, `Reconnect`, `ExecuteLong`, `RegisterLong` | `internal/control/execute_long.go`, `internal/control/server.go:157-169` | kept; #348 owns wiring or deleting them |

Foreign changes in range: aa959d7 (#316): spec bookkeeping files; 8334337 (#294): internal/supervisor/pipeline.go, internal/supervisor/pipeline_test.go, internal/workers/worker.go; ac0f315 (#292): internal/supervisor/pipeline.go, internal/supervisor/pipeline_test.go; ede0fab (#347): internal/control/lock.go, internal/control/lock_test.go; 5fe5282 (#335): docs/limitations.md.

## Acceptance

After #350 (Q-27) S1 does not serve `stop` or `recover`. Criteria that only the control handlers meet are marked partial: the handlers and their tests are on main, but no production caller reaches them until #348.

| Criterion | Result | Evidence |
|---|---|---|
| AC-1.1 ladder pinned at admission, climbed rung by rung | partial | Pin and worker climb in production: `TestSpawnMintsAndJournalsNonce`, `TestWorkerClimbsPinnedLadder`, `TestStopLadderEscalatesToKill`. Version-mismatch refusal only in the unserved handler (`TestStopVersionMismatchWritesNoRequest`); `Validate` has no production caller (#362) |
| AC-1.2 stop receipt with version, signals sent, unresolved PIDs; unknown stays unknown | partial | Worker spools `ladder_version`, `sent`, `unresolved_pids`; legacy ingest journals them (`internal/workers/worker.go:899-907`). Receipt logic with the #336 and #344 fixes is handler-only: `TestStopUnresolvedDescendantsAreNotConfirmed`, `TestStopIgnoresStoppedReportSpooledBeforeTheRequest`. Scan gaps: #354, #357 |
| AC-1.3 deadline without confirmed termination ends `cancel_incomplete`, quarantines, signals nothing more | partial | Deadline path only in the handler (`TestStopDeadlineQuarantines`); an unconfirmed worker report ends `interrupted/stop_unconfirmed` (maintainer decision Q-25), follow-up #355 |
| AC-2.1 recovery chooses exactly one outcome | partial | `TestReconcileChoosesOneOutcome`, handler-level; production `mythhelm recover` is the legacy path with no one-pass bound; #348, #359, #369 |
| AC-2.2 one pass, then stop until fresh evidence | partial | `TestSecondPassWithoutFreshEvidenceReplays`, `TestQuarantineHealsOnFreshEvidence`, handler-level; #348 |
| AC-2.3 continuation keeps the launch identity, no replay | partial | `TestContinuationKeepsLaunchIdentity`, `TestRecoveryNeverReplaysEffects`, handler-level; no launcher is wired (#370) |
| AC-3.1 supervisor loss pins the worker to its admitted episode | met | `TestSupervisorBeatLossAndReturn`, `TestUnsupervisedWorkerStartsNothing`, `TestUILossHasNoExecutionEffect`, `TestWatchWritesSupervisorBeatAfterIngest`; the wait is unbounded and loss is checked only at episode end (#363) |
| AC-3.2 spool failure stops a blind worker | met | `TestSpoolFailureStopsBlindWorker` |
| AC-3.3 supervisor return reconciles before issuing a new token | partial | `TestReconnectReconcilesFirst`, `TestOldGenerationNeverFresh`; `Reconnect` has no production caller (#360, #361) |
| NFR-5 PID, start identity and nonce, never PID alone | partial | `TestMatchIdentityRequiresPidStartAndNonce`, `TestStopIdentityMismatchOwnershipUnresolved`; recovery classifies "gone" by PID alone (#358) |
| NFR-6 no credentials, no network | met | `TestHermeticNoCredentialsOrNetwork`; its decoy proof is weak (#367) |
| Definition of Done: `stop` and `recover` are idempotent control intents (N1) | unmet | Idempotent at handler level (`TestStopIntentIdempotent`, `TestStopRepeatDuringWaitJoins`); not served in S1 by decision Q-27, follow-up #348 |

## Deviations

- Task 1: the stop wait runs inside `Execute`'s transaction (design section 3 says none is held); `stop` is registered through `RegisterStop`; new control codes and the `ladder_version` payload field are declared in `stop.go` — recorded in the task entry; Task 4 added long-handler support.
- Task 2: unfenced spawns pin generation 0 instead of failing admission; legacy-shaped handoffs climb without the identity check; `internal/control/pinning.go` and `lock.go` changed outside the task's list (review round 1) — recorded in the task entry; generation 0 is defined in Task 4.
- Task 3: `Reconcile` takes `RecoverDeps`, not a bare `*sql.DB`; ambiguous-effect uncertainty records a `quarantined` outcome with reason `external_effect_uncertain` — recorded in the task entry; none changes a criterion.
- Task 4: `ExecuteLong`, served registration of `stop` and `recover` with a nil launcher, `MaxSupervisorBeatAge` mirrored in two packages, worker loss evaluated once at episode end, `Reconnect` without a production caller — recorded in the task entry; the last two leave AC-3.1 and AC-3.3 narrower than their text (#363, #360).
- Spec-wide: Q-27 (#350) removed served `stop` and `recover`, so requirements Definition of Done, design sections 2, 3 and the honesty register, tasks 4 lines 150-156 and the hand-off still describe a served path — found by review; recorded here, impact: FR-2 and AC-1.3 are handler-level until #348.
- Spec-wide: #336 added `internal/ownerlock` and made control intents hold the run owner lock, contrary to design D1 and D5 — found by review; no decision record (#368).

## CI history

- CI on PR #289 (`fb3037b`): real, `go vet` failed on every platform with `CodeProcessLost redeclared in this block` (`internal/control/stop.go:28`) after #284 landed the same constant in `control.go` during the task (the Task 1 deviation records it); fixed in the branch before merge.
- OSV-Scanner on PR #289 (`9b0e54a`, `fb3037b`): real, 2 known vulnerabilities in `golang.org/x/text` 0.3.8 (GO-2026-5970, GO-2026-6629) reported through `go.mod`; the scan did not fail on the merge commit.
- CI on PR #311 (`7459b5f`): real, `Go (windows-latest)` `go test -race ./...` failed `TestIngestFailureInterruptsRunExit6` (4.92 s). Cause per the Task 4 deviation: a fenced worker whose supervisor is gone waits for its return holding the spool open, which Windows cannot delete during TempDir cleanup; fixed by `929ccee` (release fenced workers before fixture cleanup), later commits green. This is the mechanism of #363 in production code.
- Docs on merge `6dbbdaf` (PR #289): real, `Check recording transcripts are fresh at the declared revision` (from `finalize.py verify`, `note=history:`); later main runs green.
- Main at the tip `8caedd4`: every workflow completed with success (CI, CodeQL, Docs, OSV-Scanner, zizmor, Release drafter, OpenSSF Scorecard).
- No failure and success on the same `headSha` for any of the spec's pull requests, so no nondeterministic failure was recorded on a task PR. Two nondeterministic failures hit main after the tasks merged and each got a test-only fix:
  - `Go (ubuntu-24.04-arm)` on main `4888161` (run 38034542400, fixed by #337): `TestUnsupervisedWorkerStartsNothing` failed with "a marker-carrying sleeper was not found: the probe is blind" while the same commit passed on amd64, macOS and both Windows jobs. Mechanism: `/proc/<pid>/environ` reads empty until the kernel records the new image's environment; failing value an empty read, passing value a populated one, time until populated median 94us, p99 312us, max 520us over 3000 starts (measurements in #337). The production scan has the same window (#354).
  - `Go (windows-11-arm)` on PR #339's run 38038748783 (fixed by #340): `TestApplyCreatesOnlyTheBranch` failed with "worker 8976 ... outlived the test" after the 1-minute cleanup deadline. Mechanism (inferred by elimination in #340, not observed): a recycled PID answered for the dead worker, so the cleanup waited on another process; failing value one worker outliving a 60 s deadline, passing value 92 of 93 local cleanups finding the worker already gone with zero beats.

## Effort

dispatched=5 returned=4 failed=2; attempts 5 over 4 tasks; first-pass 2/4; review rounds 5; wall-clock 2026-10-08T23:31:47Z to 2026-10-10T01:58:32Z (last task merge). The five fix pull requests (#336, #337, #340, #344, #350) ran outside the run-events log and carry no effort rows: unknown.

dispatched=5 returned=4 failed=2
unaccounted=task 1 (1 dispatch)
| Task | Agent | Attempts | Review rounds | PR | Merged | First pass |
|---|---|---|---|---|---|---|
| 1 | - | 2 | 3 | #289 | yes | no |
| 2 | ssr-t2-continue | 1 | 1 | #298 | yes | yes |
| 3 | go-implementer | 1 | 1 | #296 | yes | yes |
| 4 | - | 1 | 0 | #311 | yes | no |

## Lessons

- What worked: the task-4 CI failure (Windows TempDir, #311) exposed the parked-worker behaviour before merge, and the fixture release helper (`releaseWorkers`) was added for it.
- What worked: the whole-spec review found five criticals no per-task review had, and each was closed by its own fix pull request with a pinned test (#336, #344, #350).
- What to change: handlers that write shared attempt state were reviewed task by task against their own tests, never against the next owner that reads that state (C3 and N1 surfaced only at finalize; #336, #350). Proposal P-supervised-stop-recover-1.
- What to change: tests that spawn long-lived processes left cleanup to the end of the test and passed on PID alone, so one failed on Windows (#311) and two more needed test-only fixes (#337, #340). Proposal P-supervised-stop-recover-2.
- What to change: an accepted maintainer decision (Q-27) changed what the spec delivers after its tasks merged, and the spec text still describes the old design; the retrospective records the difference (Deviations) but the spec documents stay stale.

## Proposals

- P-supervised-stop-recover-1 — Test the next owner against what a new writer leaves behind
- P-supervised-stop-recover-2 — Fixture processes are reaped by identity and released before cleanup
