# dogfood-slice — Retrospective

## Review Summary

- **Range**: 5d16a7b..1b58139 (PRs #7, #8, #13, #10, #16, #9, #15, #11, #17, #20, #49, #50, #51, #52, #53, #54, #56, #57, #58, #67; fix PRs #55, #59, #66)
- **Reviewer**: code lens + architecture lens (general-purpose subagents; this client has no pinned code-reviewer/architect definitions)
- **Findings**: critical 0, important 8, suggestion 10 in 1 batch (all confirmed); rejected 0
- **Open critical**: 0
- **Vendor review**: skipped (codex/muse/jev all disabled)

| Severity | Finding | Anchor | Disposition |
|---|---|---|---|
| important | review JSONL omits required `"type":"receipt"` (AC-vs-§9 conflict) | `internal/cli/review.go:62` | issue #69 |
| important | doctor JSONL wrong shape (no `.result`/exit fields) | `internal/cli/doctor.go:51` | issue #71 |
| important | demo has no `--format jsonl`; flags diverge from §10 | `internal/cli/demo.go:49` | issue #70 |
| important | AC-4.2 verbatim sentence nowhere user-facing | `internal/supervisor/receipt.go:173` | issue #72 |
| important | §10/§11 surface gaps (--max-duration, --capture-raw, --log-level, liveness) | `internal/cli/run.go:24` | issue #73 |
| important | native_error/ownership_resolved escape §5 envelope set | `internal/supervisor/pipeline_test.go:602` | issue #74 |
| important | NOT RUN label missing from receipt/review (AC-7.4) | `internal/supervisor/receipt.go:183` | issue #75 |
| important | No stop on ingest failure; attemptless runs trap admission (AC-9.5) | `internal/supervisor/pipeline.go:526` | issue #76 |
| suggestion | Batch: exit buckets, helper dup, stale texts, wording conflicts | `internal/cli/exit.go:98` | issue #77 |

Foreign changes in range: 9acc0b1 docs(spec): start dogfood-slice implementation (#48); 1955e50 docs backfill (#43); d523c03, bcff9a5, 14d45e1, 1bd0cd3 harness phases (#41, #19, #12, #4). No findings drawn from foreign lines.

## Acceptance

65 criteria: met 59, partial 6, unmet 0. "Met" means a named merged test or a green merge head; "partial" means the behavior shipped with a spec-gap filed as a follow-up issue.

| Criterion | Result | Evidence |
|---|---|---|
| AC-1.1 nine commands + hidden entry points | met | Tasks 1/5/10/12–15/18; PRs #7 #16 #20 #50–#54 #56 #57 |
| AC-1.2 §15.10 exit meanings | met | `exit.go` mapping + outcome tests (T10/T12/T14) |
| AC-1.3 JSONL schemas + run.result | partial | run/recover/version/runs-list met (T1/T5/T10); review/demo/doctor gaps → #69 #70 #71 |
| AC-1.4 --non-interactive exits 3, never waits | met | TestUntrustedChecksNonInteractiveBlocked, TestDirtyCheckoutNonInteractiveBlocked (T10/T12) |
| AC-1.5 plain linear output, NO_COLOR | met | T10 renderers; T19 packaged-binary plain checks |
| AC-2.1 admission.decided persisted before launch | met | TestAdmissionDecidedBeforeWorkerSpawn (T10) |
| AC-2.2 explicit --adapter, no fallback | met | TestRunNoAdapterFlagExits2 (T10); claudecode path (T17) |
| AC-2.3 trusted-host consent; restricted/inspect exit 7 | met | TestRestrictedProfileExit7, TestHostHerdrExit7 (T10) |
| AC-2.4 second active run blocked with its ID | met | TestRunSecondActiveRunBlocked (T10) |
| AC-2.5 native-config inventory + digest grants | met | T16 hooks/MCP/declaration-binding tests (PR #54) |
| AC-3.1 committed HEAD recorded as snapshot | met | T7 preflight/snapshot + T10 admission (PR #15 #20) |
| AC-3.2 dirty checkout offer, never stash/reset | met | TestDirtyCheckoutNonInteractiveBlocked (T10) |
| AC-3.3 detached clone, every remote removed | met | TestSnapshotHasNoRemotesAndDetachedAtRev (T7) |
| AC-3.4 checkout byte-identical until apply | met | TestRunLeavesSourceCheckoutUntouched (T19) |
| AC-3.5 submodules/LFS/sparse/shallow exit 7 | met | TestPreflightRejectsSubmoduleLFSSparseShallow (T7) |
| AC-4.1 --billing required; strict always blocks | met | TestStrictSubscriptionOnlyAlwaysBlocks (T16) |
| AC-4.2 declared posture + verbatim sentence | partial | declaration/auth/labelling met (T16 tests, T20 live run); verbatim sentence nowhere user-facing → #72 |
| AC-4.3 credential-route overrides block, names only | met | T16 env/settings/helper tests (PR #54) |
| AC-4.4 apiKeySource≠none interrupts to blocked | met | TestApiKeySourceMismatchStopsAttempt (T17) |
| AC-4.5 cost as labelled estimate, unknown never 0 | met | TestReceiptCostLabelledEstimate, TestReceiptUnknownsNeverZero, TestCostStoredAsEstimateString (T13/T17) |
| AC-4.6 no credential values read or persisted | met | TestNoCredentialValuesPersisted (T13); TestAuthStatusPIIDropped (T16) |
| AC-4.7 OAuth token opt-in only | met | doubly closed + unwired (T17, ADR 0002 §6); T20 native-login checkpoint |
| AC-5.1 launch-intent token + identity match | met | T9 worker identity + T10 admission-order test |
| AC-5.2 detached worker survives CLI death (Unix) | met | TestWorkerSurvivesParentExit (T9); TestE2EKillCLIWorkerSurvivesThenRecover (T19) |
| AC-5.3 argv launch, prompt on stdin | met | TestArgvExact, TestPromptOnStdinOnly (T17) |
| AC-5.4 allowlisted child env | met | TestChildEnvIsAllowlisted (T17) |
| AC-5.5 Ctrl-C stop→130, second→detach 6 | met | TestCtrlCOnceStopsAndExits130, TestCtrlCTwiceDetachesExit6 (T10) |
| AC-5.6 stop ladder + unresolved descendants | met | TestStopLadderEscalatesToKill, TestUnresolvedDescendantReported (T9) |
| AC-5.7 native_result semantics incl. explicit nulls | met | TestStoppedExitEmitsNativeResultWithNulls (T9); T17 result-mapping tests |
| AC-5.8 denials recorded, no bypass flag | met | TestHeadlessPermissionDenialNoHang (T15) |
| AC-6.1 freeze after confirmed stop; quarantine on unresolved | met | TestFreezeWaitsForStopConfirmation, TestFreezeRefusedWithUnresolvedDescendants (T11) |
| AC-6.2 whole-tree candidate on admitted base | met | TestFreezeIncludesUncommittedAndUntracked, TestFreezeIgnoresAgentMovedHEAD (T11) |
| AC-6.3 validation flags + --accept-flags | met | TestFlagsSymlinkEscapeBinaryLargeSecretConfigChange (T11); T14 gate |
| AC-7.1 checks from snapshot, candidate edits flagged | met | TestChecksReadFromSnapshotNotCandidate (T12) |
| AC-7.2 digest-bound trust before checks/tools/env | met | TestUntrustedChecksNonInteractiveBlocked, TestProjectConfigTrustBoundToDigest (T12) |
| AC-7.3 argv checks, timeouts, 5 statuses, hashed evidence | met | T12 timeout/unavailable/evidence tests (PR #50) |
| AC-7.4 outcome/exit mapping + NOT RUN | partial | exits/unverified/--accept-unverified met (T12); NOT RUN missing from receipt/review → #75 |
| AC-8.1 receipt.json on terminal/ready runs | met | TestReceiptHasAllSection9Keys (T13) |
| AC-8.2 review renders sanitised receipt + diff | partial | plain render + sanitise met (T13); JSONL object lacks type:receipt → #69 |
| AC-8.3 guarded branch apply, checkout otherwise untouched | met | T14 apply tests (PR #52) |
| AC-8.4 interrupted apply reconciles | met | TestApplyReconcilesAfterCrash (T14) |
| AC-9.1 SQLite WAL/0700/MYTHHELM_HOME | met | TestPragmasApplied, TestStateDirMode0700 (T4) |
| AC-9.2 append-only journal + dedup + sequences | met | T4 trigger/idempotency/gap tests (PR #10) |
| AC-9.3 atomic transition+event; list reads projections | met | TestTransitionAtomicWithJournal, TestRunsListReadsProjectionsOnly (T5) |
| AC-9.4 newer schema exits 2 without writing | met | TestOpenRefusesNewerSchema (T4) |
| AC-9.5 unwritable journal blocks admission, safe stop | partial | locked-DB admission block met (T15); no stop on ingest failure → #76 |
| AC-10.1 recover: reattach/continue/quarantine | met | T15 recover tests (PR #53) |
| AC-10.2 recovery never relaunches native | met | TestRecoverNeverRelaunchesNative (T15) |
| AC-10.3 PID reuse treated as lost, never signalled | met | TestWorkerPIDReuseTreatedAsLost (T15) |
| AC-10.4 stop reports requested until confirmed | met | TestStopReportsRequestedUntilConfirmed (T15) |
| AC-11.1 offline scripted demo on all OSes | met | T18 demo tests + TestE2EDemoAllOS (T19) |
| AC-11.2 fake runs as real child via same interface | met | TestFakeScenarioHappyEditsFile (T8); __fake-agent |
| AC-11.3 named fault-scenario coverage | met | T15 §18.4 fault suite (PR #53) |
| AC-12.1 doctor reports version/env/facts | met | T18 doctor tests (PR #57) |
| AC-12.2 doctor read-only, no session/network | met | TestDoctorIsReadOnly, TestDoctorDoesNotRunAuthStatus (T18) |
| NFR-1 bounded parsing (16MiB/64-deep/UTF-8/ring) | met | ndjson reader tests (T8); T15 malformed/deep/oversized/bad-UTF8 tests |
| NFR-2 redacted logs; opt-in raw capture | partial | redaction met (T6/T13); --capture-raw flag missing → #73 |
| NFR-3 exact 3 allowlisted direct deps | met | go-licenses job (T3); go.mod pins |
| NFR-4 3-OS build/vet/race + capability record | met | CI matrix green at all 20 merge heads; T8 platform block |
| NFR-5 runs-list-100 under 200ms (advisory) | met | BenchmarkRunsList100 ~4ms/op local (T5) |
| DoD all tasks complete, 3-OS CI | met | 20/20 merged; merge heads green |
| DoD lint + vuln + dep review green | met | golangci/govulncheck/dep-review success at merge heads |
| DoD ADRs 0002/0003/0004 | met | PRs #54 #10 #17 |
| DoD maintainer dogfood run + evidence | met | T20 PR #67; canary + recorded-2.1.285.jsonl |
| DoD README status section | met | T19 PR #58 |

## Deviations

- Task 1: added `ExitInternal = 1` next to the listed constants — recorded in the task entry; design §11 requires exit 1 for unexpected errors, no behavior conflict.
- Task 2: golangci-lint v2.13.2 not v2.14.0 (7-day cooldown), govulncheck via `go run` (action not allow-listed), CodeQL Go leg adds `setup-go` — recorded in the task entry; same coverage, pinned toolchain.
- Task 3: OSV-Scanner/go-licenses via `go run` (actions not allow-listed), no `-race` on windows-11-arm (unsupported), notices/macOS-x64 deferred — recorded in the task entry; coverage as designed minus unsupported/deferred items.
- Task 4: extra test file, extra exported API, CGO check as a test not a CI step, `ids.New` adds `_`, payload must be a JSON object, exit-2 mapping deferred to Task 5, `Open` does not create dirs, dependency-review allow-list entry for the Go patent grant, `Ensure` refuses symlinked state dirs — recorded in the task entry; all supersets or hardening, no AC weakened.
- Task 5: pointer/signature adjustments, `interrupted` reachable from any non-terminal state, reasons required on terminal states, `runs list` without worker liveness — recorded in the task entry; liveness arrived in Tasks 9/15.
- Task 6: quoted-key redaction superset, fail-closed symlink/credential handling, `CLAUDE_CODE_OAUTH_TOKEN` always denied by `BuildEnv` (AC-4.7 opt-in is the adapter's), exit-2 mapping deferred — recorded in the task entry; stricter than specified throughout.
- Task 7: `PreflightResult` rename, per-OS empty hooks path, `GIT_*` scrub + `GIT_TERMINAL_PROMPT=0`, tree-based submodule/LFS detection, full-OID snapshot with all remotes removed, fingerprint covers untracked/ignored + permissions — recorded in the task entry; stricter and more portable.
- Task 8: build-tagged escapee helpers, `OwnedProc`/`NativeExit` shapes, capability `platform` block, fake exit codes, 4 emit variants, `Prepare` requires a scenario — recorded in the task entry; interface growth the design anticipated.
- Task 9: `attempt.stopped` ends every attempt with new fields, worker launch over stdin JSON, new reason codes, files outside the list — recorded in the task entry; §7 event-model clarifications, ADR 0004 carries them.
- Task 10: native success temporarily ends failed/`verification_unavailable` (exit 5) until Tasks 11–12, claudecode exits 7 until Tasks 16–17, `run.result` gains `attempt_reason`, reaped-worker acceptance, later-task flags refused, journal/supervisor/CLI supporting edits, ADR 0005 — recorded in the task entry; placeholders replaced by Tasks 11–12, none survived.
- Task 11: verification still stubbed until Task 12, `apply` still refused until Task 14 (with a retention requirement), git identity required at admission — recorded in the task entry; sequencing, no scope loss.
- Task 12: `apply`/`--accept-unverified` deferred to Task 14 (with a retention requirement), supporting edits across admission/CLI/integration/journal — recorded in the task entry; sequencing.
- Task 13: supporting read-projection/registration edits, Windows identity-retry restricted to transient sharing violations (CI-exposed race), unestablishable fields stay `"unknown"` — recorded in the task entry; the retry is the first half of the #55 race fix.
- Task 14: atomic `update-ref` branch creation instead of the design's destination fetch refspec (closes a concurrent-create race), `ApplyRun` orchestration, apply refuses all existing branches initially, ADR 0006 — recorded in the task entry; strictly safer than the design.
- Task 15: supporting edits across CLI/supervisor/workers/fake plus ADR 0007; quarantined-partial ends failed/`recovered_partial`, unfinished checks stay ownership-unresolved, post-terminal panic preserves state, intent-bound branches complete on recovery, stop/freeze reason semantics — recorded in the task entry; §7.7/§11 boundary clarifications, all fail-closed.
- Task 16: probe-input extension, declaration projection APIs, auth-test split, symlinks unsupported, raw config text omitted from durable JSON, remote policy/MDM uncertified, Q5 network behavior unknown — recorded in the task entry; fail-closed posture, open items explicit.
- Task 17: `PrepareInput` extension + shared `ClimbLadder`, worker AC-4.4 stop with sticky poison + binary re-verification, pipeline blocked mappings, `stopping → blocked` transition (design §4 amended), receipt tools/hooks/grant from journal, claudecode `Decide` wiring, blob-based project inventory, OAuth doubly closed, fail-closed decoder rules, MCP/allowed-tools injection guards — recorded in the task entry; the transition and decoder rules are the substantive spec amendments.
- Task 18: shared `executeRun`, command registration, exported `CredentialEnvNames` — recorded in the task entry; pure refactor plus a names-only accessor.
- Task 19: 2 extra demo checks, deferred ADR 0002 §6 amendment lands here — recorded in the task entry; additive.
- Task 20: recording is a rate-limit encounter not a success stream (success stream unrecoverable — spool holds observations, not raw stdout), compat string `recorded` for 2.1.285, Task 20 text's own `-run` bug fixed by the candidate — recorded in the task entry; decoder error shape pinned (fails pre-#59), success recording is future work.
- Review-found: design §10/§11 CLI surface gaps (`--max-duration`, `--capture-raw`, `--log-level`, runs-list liveness) — found by review; filed as #73.
- Review-found: `native_error`/`ownership_resolved` envelopes escape the design §5 set — found by review; filed as #74.
- Review-found: suggestion batch (exit buckets, helper duplication, stale texts, wording conflicts) — found by review; filed as #77.

## CI history

Merge incidents (from `finalize.py verify` history notes, mechanisms established by log forensics):

- CI/Go (windows-latest) on merge 65c3d1e (PR #52): real, `TestFreezeWaitsForStopConfirmation/happy` ended failed_native (launch_failed). Mechanism: Windows file-sharing race on atomic worker.json replacement (rename → Access is denied under parallel-test load); failing value run 36842187196; passing value the same test green on PR head c031dfd and on post-fix main. Fixed by #55.
- CI/CI OK on merge 65c3d1e (PR #52): real, rollup failure (Run exit 1) of the windows-latest job above.
- CI/Go (windows-latest) on merge 8e20861 (PR #54): real, `TestApplyReconcilesAfterCrash` launch_failed; the worker diagnostic shows `writing worker.json: rename …: Access is denied`. Mechanism: the same sharing race; failing value run 36844977281; passing value the same test green on PR head c4747c2 and on post-fix main. Fixed by #55.
- CI/CI OK on merge 8e20861 (PR #54): real, rollup failure (Run exit 1) of the windows-latest job above.
- OSV-Scanner on merge 0d23a7b (PR #49): unknown, step Report findings and fail (scan outcome failure). Single occurrence; failed-log unavailable; re-scanning 0d23a7b with osv-scanner v2.6.0 today reports No issues found; every later OSV run is green with a superset of dependencies; govulncheck clean throughout. Not reproduced — transient scanner/DB state, no mechanism established.

PR-branch intermediates (each real, fixed by a later push, merge head green; no same-headSha failure+success pair exists on any of the 20 task branches, so none qualifies as nondeterministic):

- PR #10 (T4): Dependency review failed on 350fb35 and 837d3e8 (Go patent-grant license until the allow-list entry); golangci-lint failed on dfe9a57 (gosec G115/G304/G301/G302).
- PR #15 (T7) e0275ed: Go (windows-11-arm + windows-latest), TestSnapshotExcludesUntrackedAndIgnored and TestSnapshotHasNoRemotesAndDetachedAtRev.
- PR #17 (T9) ee599a1: Go (windows-11-arm); failed-log unavailable.
- PR #20 (T10) d2076ac: Go (windows-latest), TestIngestFailureInterruptsRunExit6.
- PR #49 (T11): Harness job on 86ef8ce (failed-log unavailable); Go (windows-11-arm) on adafae1 (failed-log unavailable).
- PR #50 (T12) 1c0c93e: Go (windows-11-arm + windows-latest), 4 supervisor tests (MissingExecutable, NativeSuccess×2, ProjectConfigTrustBoundToDigest).
- PR #51 (T13) 76da8ef: Go (windows-11-arm + windows-latest), TestAdmissionDecidedBeforeWorkerSpawn and TestRunOutcomeExitCodes.
- PR #52 (T14): 8166082 failed apply tests (TestApplyReconcilesAfterCrash, TestApplyRefusesUnacceptedFlags) on windows-11-arm, windows-latest and macos-latest; a685e2f failed Go (windows-latest) with failed-log unavailable.
- PR #53 (T15) 4446751: Go (windows-latest), TestApplyRefusesInvalidRefName and TestApplyRefusesNotReadyForReview.
- PR #56 (T17) 77362c3: Go (windows-11-arm + windows-latest), broad claudecode suite failures (34 FAIL lines incl. ChildEnv, PromptOnStdin, ApiKeySourceMismatch, AuthFailed, CostStored).
- PR #57 (T18) b78c713: Go (windows-11-arm + windows-latest), TestDoctorUnreadableStateDirIsNotAbsent.
- run-events fail/retry rows: none.

## Effort

dispatched=2 returned=1 failed=0; attempts 2 over 7 logged tasks (14–20; tasks 1–13 predate the log); first-pass 1/7 logged (task 20; tasks 17–19 merged but unattributed in the log); review rounds 1 (run-events row, task 15) plus task-entry rounds on T16 (code/architect), T17 (1) and T19 (2); wall-clock unknown → 2026-10-02T10:26:16Z (no run_start row; earliest log row 2026-09-30T23:20:49Z)

```
dispatched=2 returned=1 failed=0
unaccounted=task 20 (1 dispatch)
| Task | Agent | Attempts | Review rounds | PR | Merged | First pass |
|---|---|---|---|---|---|---|
| 14 | - | 0 | 0 | #52 | no | no |
| 15 | - | 0 | 1 | #53 | no | no |
| 16 | codex-go-implementer | 1 | 0 | #54 | no | no |
| 17 | - | 0 | 0 | #56 | yes | no |
| 18 | - | 0 | 0 | #57 | yes | no |
| 19 | - | 0 | 0 | #58 | yes | no |
| 20 | - | 1 | 0 | #67 | yes | yes |
```

Note: the table above is verbatim `runspec.py summary` output: its Review rounds column counts run-events rows only, so T16/T17/T19 show 0 there while the task-entry rounds (T16 code/architect, T17 one, T19 two) come from the tasks.md completion entries, a separate source. The Merged column is stale for #52/#53/#54 (all three merged: 65c3d1e, 4fd69ae, 8e20861); the log rows predate their merges.

## Lessons

- What worked: red-first named acceptance tests per task caught defects before merge (T16's 16 checks red pre-implementation; T19's 4 wrong test expectations corrected with no product change).
- What worked: the two main-CI Windows failures were root-caused to one sharing race via CI logs and fixed in product code (Task 13 retry + #55) rather than re-run until green.
- What worked: per-task review rounds with an independent code/architect lens adjudicated findings into tests and design amendments (T17 B1 stopping→blocked; T19 CodeRabbit threads fixed and resolved).
- What worked: the spec-wide review routed all 8 important findings to tracked follow-up issues (#69–#76) and 10 suggestions to one batch issue (#77) with zero criticals outstanding.
- What worked: the maintainer dogfood run reached ready_for_review and produced versioned live evidence; recorded-2.1.285.jsonl pins the decoder's error shape and fails pre-#59.
- What to change: JSONL acceptance tests asserted shape but not the `type` discriminator, so Task 13's suite passed while review JSONL omitted required `type:receipt` (same class: #70, #71) → P-dogfood-slice-1.
- What to change: the Task 20 text's own `-run TestLiveCanary` selector matched no test and silently ran nothing (#60) → P-dogfood-slice-2.
- What to change: Windows-only failures dominated intermediate CI (10 of 11 failing branches failed a Windows Go job); the sharing-race mechanism is now known but lives only in this retrospective → P-dogfood-slice-3.
- What to change: run-events starts mid-spec (2026-09-30T23:20:49Z, no run_start row, tasks 1–13 absent), so the wall-clock start is unknown and early effort unattributed → P-dogfood-slice-4.
- What to change: the single OSV-Scanner failure was never diagnosed because its log expired before capture; failed-run logs need prompt capture (one-off, no mechanism — stays a lesson).

## Proposals

- P-dogfood-slice-1 — JSONL acceptance tests assert the type discriminator
- P-dogfood-slice-2 — Spec validation executes embedded test selectors
- P-dogfood-slice-3 — Windows atomic-replace retry pattern in knowledge
- P-dogfood-slice-4 — run-spec dispatch emits complete run-events rows
