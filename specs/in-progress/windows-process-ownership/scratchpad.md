# Windows Process-Tree Ownership — Scratchpad

> Open questions and research from authoring. Each implementing task reads this file when it starts and adds an entry under Discoveries when it finishes.

## Open questions

| # | Question | Default in this spec | Owner / blocks |
|---|---|---|---|
| OQ-1 | Does the job kill its processes when the worker dies, or survive? | (a) worker-owned kill-on-close job (D1) | Maintainer, ADR (Task 1); blocks Tasks 4, 6 |
| OQ-2 | Which graceful Windows rungs exist for a detached console child? | Kill-only ladder (D6) | Spike in Task 4 |
| OQ-3 | Launch method and breakaway policy under runner/terminal jobs | Suspended create + assign + resume; refuse when assignment fails (D2) | `architect` review; blocks Task 3 |
| OQ-4 | Is `worker_detachment` derived from AC-3.1 evidence? | Derived, one source for both adapters | `architect` review of the seam; Tasks 8-9 |
| OQ-8 | Native start time on Windows only or on every OS? | Windows only (D4) | Maintainer, ADR; Task 5 |
| OQ-9 | Reconcile `supervised-stop-recover`'s Windows Job Object rows | This spec owns them; that spec reconciles through its lifecycle | Owner of that spec; Task ordering |
| OQ-10 | Follow-up card for filesystem, terminal and shell rows (N5) | File a card after approval | Maintainer; MH-11 shipping claim |
| OQ-11 | Is kill-on-close plus confirmed worker death I24 evidence? | No; ownership stays unresolved | Maintainer, ADR; Task 6 |
| OQ-13 | Accept the residual suspended-native window, or create the process directly in the job (`PROC_THREAD_ATTRIBUTE_JOB_LIST`)? | Accept the recorded-but-unassigned window; recovery names but never signals the native. Before PID and `native_start_time` are recorded, ownership stays unresolved (AC-6.3) | `architect` review with OQ-3; blocks Task 3 and Task 6 |
| OQ-12 | ADR number 0016 collides with `0016-migration-import.md` on main; the spec uses 0017 | Reconcile at merge if 0017 is taken | Task 1 |

## Research notes

- `syscall.StartProcess` on Windows closes the main thread handle and `SysProcAttr` has no job field, hence the suspended create and thread enumeration (D2; `$GOROOT/src/syscall/exec_windows.go:446`).
- `golang.org/x/sys` v0.48.0, checked by the validator: present are `CreateJobObject`, `AssignProcessToJobObject`, `TerminateJobObject`, `QueryInformationJobObject`, `SetInformationJobObject`, `CreateToolhelp32Snapshot`, `Thread32First/Next`, `OpenThread`, `ResumeThread`, `JobObjectBasicProcessIdList` and the `JOB_OBJECT_LIMIT_*`/`CREATE_*` flags. Absent: `GetHandleInformation`, `IsProcessInJob` and the `JOBOBJECT_BASIC_PROCESS_ID_LIST` type; Task 3 binds them with `windows.NewLazySystemDLL("kernel32.dll")` (D7) and defines the list struct locally.
- Whether hosted runners run steps inside a kill-on-close or no-breakaway job, how the worker detects a kill-on-close parent job (`QueryInformationJobObject` with a nil handle queries the caller's job), and whether console events can be driven there, are unknown until Task 3 runs on `windows-latest` and `windows-11-arm`.

## Validation findings left open (3 rounds used)

Round 3 (the last allowed) returned `Needs revision`: 3 blocking findings (GroupGone versus `classify`, no stop deadline mechanism, no way to inject matrix rows) were fixed afterwards but not re-validated; 14 advisory findings are open for the next `/spec windows-process-ownership` run:

- `jobOps` seam: no `windows.Handle` in cross-platform `worker.go`; seam for failing `TerminateProcess` on the suspended native; signature of Task 6's identity probe helper.
- Task 6 crash points: resolved in the Q-26 pass (six hooks, a workers-package record test and a supervisor-package outcome test, design §2.5); AC-3.2 supervisor restart still has no Windows test beyond boundary (iii).
- Task 3 spike: now specified as a CI run on both Windows jobs with `parentJob` defined exactly (design §2.2, D11, Task 3); the run ids and logged values are recorded here when the spike runs. Q-30 (maintainer): if any runner reports `(true, nil)`, Task 3 stops after the spike and escalates (breakaway at spawn, or refusal with the out-of-process tests reworked); no `parentJob` stub.
- `launch_error` scope: Windows only (N4) and which error text may be recorded (native paths).
- Tasks 10 and 11 each mix docs and core files; `TestDocsMatchWindowsMatrix` needs fixed anchors (the "Supported today" bullet, `docs/limitations.md:14-15`) and all four docs of AC-5.4.
- No named test for NFR-3, NFR-4, AC-2.4, AC-3.3 (crash-behaviour column).
- Acceptance items that already pass on main (Task 4 ladder tests, Task 5 ingest test, Task 7 tests) need a named failing state; Task 7's `blocked` escape for AC-6.4 needs an AC change or a `Hooks.Interrupt` path; Tasks 7 and 9 need a packaged-binary item or a line saying why not.
- `STILL_ACTIVE` is absent from `x/sys` v0.48.0 (add to D7); `requirements.md` impacted components mention `internal/control/support_test.go`, which no task edits.

## Discoveries

### Task 1 — approved ownership decisions

The maintainer approved the design defaults for OQ-1 (worker-owned kill-on-close), OQ-8 (Windows-only native creation time) and OQ-11 (worker death plus kill-on-close does not resolve ownership). ADR 0017 records these decisions with D2, D3, D8 and D11. No acceptance criteria change. OQ-3/OQ-13 architect review, Task 3's real Windows runner spike and its conditional maintainer escalation remain required; Task 11 still controls support-row promotion. ADR 0004's status line remains Task 10's change. Recorded by Codex as a transcription of the maintainer decision, without implementing Windows behavior.

Local Task 1 validation initially failed unchanged Go tests: three control lock tests encountered an already-running supervisor, and `TestResolvedVersionedBinaryLaunched` reported `native_version_probe_failed`. The probe test passed when rerun alone; its initial cause remains unknown. The three control tests all passed with a private absolute `XDG_RUNTIME_DIR`, matching `internal/control/lock.go`'s lock-directory selection. The full-suite retry uses fresh temporary runtime and state directories, sets `XDG_RUNTIME_DIR` and `XDG_STATE_HOME`, and removes `MYTHHELM_HOME` only from the test process environment. It does not alter `HOME`, user configuration or the live supervisor. The isolated full race suite passed (go-test gate, 244.7s); the original failed suite exited before the retry recorded its marker. This is local test evidence, not Windows qualification.

### Task 3 — parent-job prerequisite measurement

The prerequisite probe at `81e37ebca04ea18a468add9c40c643e086f16e9d` ran in [CI run 38737162542](https://github.com/turbokast/mythhelm/actions/runs/38737162542). The native AMD64 job [116429336158](https://github.com/turbokast/mythhelm/actions/runs/38737162542/job/116429336158) logged `in_job=true limit_flags=0x00000000 kill_on_close=false` at 12:47:30 UTC on 2026-10-15. The native ARM64 job [116429336392](https://github.com/turbokast/mythhelm/actions/runs/38737162542/job/116429336392) logged the same values at 12:48:06 UTC. Both passed `TestWindowsRunnerParentJobSpike`. These real observations satisfy D11's prerequisite on these runners; they do not qualify launch ownership, detach or any native harness.

The orchestrator approved the bounded release-domain deviation: a Windows-only uncached verbose probe step in `ci.yml`, plus its matching `docs/automation.md` matrix description. Successful package-level tests otherwise hide `t.Log` measurements. Existing full test jobs and permissions remain in place. Task 3 introduces `Identity.NativeStartTime` because its required pre-assignment identity persistence cannot wait for Task 5; Task 5 retains the launched-event payload and equality/Unix-omission evidence.

Development-only Wine checks are not native evidence. Its job-limit query reported zero after a successful kill-on-close setter, and a membership query returned access denied despite the documented limited-query right. Required Windows assertions remain strict; native CI decides those results.

### Task 3 — launch cleanup review

A fresh process-ownership review found that failure to open the native's wait handle returned before using the process and job handles already held by the worker. Cleanup now attempts both available termination operations and retains `stop_unconfirmed` when its confirmation handle is unavailable. `TestConfirmationHandleFailureStillTerminatesNative` failed against the early return (the suspended native stayed alive) and passed after the fix in the same isolated Wine environment. A separate mutation removing `CREATE_SUSPENDED` failed `TestNativeCreatedSuspended`; restoring the flag passed. These development checks do not replace native Windows evidence.

The bounded CI deviation now includes verbose Task 3 launch acceptance alongside the parent-job probe on both existing Windows jobs. Full-suite checks and permissions are unchanged. The launch-only slice leaves job-wide stop, descendant reporting, recovery and capability promotion to Tasks 4–11.

### Task 3 — identity-persistence test contract

At `77edd7a`, native [CI run 39107634267](https://github.com/turbokast/mythhelm/actions/runs/39107634267) passed every named launch acceptance on both architectures, but each full Windows suite failed the existing `TestIdentityFailureStopsNative`: it expected the previous post-launch persistence-abort return, while Windows now rejects identity persistence before assignment/resume and records `launch_failed`. Unix retains the original returned error and persistence-abort assertions. The approved `worker_test.go` deviation now requires Windows confirmed launch cleanup, no ladder signals, no `attempt.launched` and the persistence cause. A separate Windows test measures the primary thread's actual suspend count, verifies resume is never reached and proves the retained native handle is signalled after cleanup. Both tests passed in isolated Wine; replacement native CI remains required. No production behaviour was changed to satisfy the older expectation.

### Task 3 — native acceptance evidence

At `5d05701`, [CI run 39108953755](https://github.com/turbokast/mythhelm/actions/runs/39108953755) passed all 18 named launch/identity tests on both native architectures. [AMD64 job 117621040510](https://github.com/turbokast/mythhelm/actions/runs/39108953755/job/117621040510) also passed the full race suite (workers package 14.945s); [ARM64 job 117621040503](https://github.com/turbokast/mythhelm/actions/runs/39108953755/job/117621040503) passed the full non-race suite (workers package 12.823s). The earlier identity-test mismatch is resolved with the stronger pre-assignment assertions. The merged palette-parser fix clears the unrelated docs-build failure. These are Task 3 launch facts, not FR-2–FR-4 or harness qualification; support promotion remains Task 11.

### Task 4 — stop evidence and bounded wait

Codex implemented the routed go-implementer task and independent code/architecture reviewers found no remaining findings. Windows retains the same kill-only five-second rung, with version `stop-ladder/v2-windows`. Stop confirmation now requires a waited native and an empty job. The final scan uses the identities from that membership query directly, invalidates an earlier confirmation on survivors or errors, and persists the stopped receipt before closing the job. Unix retains known PIDs even when their identity lookup fails.

Development red-first evidence: the tree-stop test failed with `descendant_scan: unavailable`; the original supervise loop exceeded the 12-second bounded-stop regression deadline. A counterfactual single-process stop with false confirmation left both descendants active at receipt persistence; the strengthened test rejected it before kill-on-close could conceal them. A false late confirmation failed both survivor and error cases; permitting graceful signals failed the kill-only checks. Restored tree, failed-scan, late-scan and kill-only tests pass in Wine. These development runs are not native Windows evidence.

An isolated diagnostic reproduced Wine's `IsProcessInJob` error 5 with the existing `PROCESS_QUERY_LIMITED_INFORMATION` member handle. In that isolated copy only, using `PROCESS_QUERY_INFORMATION` made both survivor cases pass, including the 10.75-second bounded native-survival case. Production retains the Task 3 access rights and strict survivor assertions; native Windows CI is required. No support row is promoted by Task 4.

The orchestrator approved extending the existing CI named acceptance selector and its automation-docs row, plus removing the now-unused `nolint:unused` directive on the non-Windows `jobOps` stand-in. The first full race gate passed (301.0s); lint then identified only four obsolete suppressions after the shared worker began retaining the job fields. Those directives were removed and the gates refreshed. The test-only Unix acceptance wrapper calls the unchanged escapee and orphan-contract cases.

### Task 4 — pre-session panic cleanup

The job-wide Signal change exposed a pre-existing panic path's assumption: a suspended native not yet assigned to the job could survive job termination while the handler blocked in `Wait`. The new regression observed that native still active before test-owned cleanup. Windows now reuses the existing bounded launch cleanup, terminating both owned handles and returning its observed kill/confirmation report; the original Unix panic path remains unchanged. Tests cover pre-assignment termination, assigned descendants and a failed-termination timeout that stays unconfirmed. All three passed in the final development run (0.28s, 0.94s and 5.25s), together with the existing shared panic cases. Fresh code and architecture reviews both returned Ready. The orchestrator approved the bounded `job_windows.go`/`job_other.go` deviation needed to return cleanup evidence without duplicating the existing termination logic; native CI still owns Windows acceptance.

### Task 4 — native stop acceptance and baseline integration

At `27ccedf`, native [CI run 39145264307](https://github.com/turbokast/mythhelm/actions/runs/39145264307) passed all 29 named launch/stop tests on AMD64 job `117727735279` and ARM64 job `117727735388`, including strict survivor identity enumeration and bounded waits. The full workers suites passed (AMD64 race: 31.824s; ARM64: 30.659s). Each full platform suite failed only `TestObservatoryLinearBaseline/demo-accessible`: the expected admission line still named `stop-ladder/v1`, while Task 4 correctly emitted `stop-ladder/v2-windows`. This is an integration contract mismatch, not nondeterministic output.

The orchestrator approved the bounded `internal/cli/observatory_baseline_test.go` deviation. The immutable captured bundle remains unchanged; an explicit Windows-only expected-value overlay changes exactly the demo's event 7 admission ladder line. Actual output is not normalized. A new test first failed with the old expectation, then passed while proving that the incorrect Windows v1 capture is rejected and every other capture field/line/platform remains exact. Existing demo counterfactuals now also require a wrong ladder version to remain different. The focused race baseline suite passed (5.758s); fresh source review returned Ready. Replacement native full-suite evidence remains required.

### Task 4 — independent-run JSONL stop evidence (2026-10-18)

- Hosted replacement CI run `39146464904`, ARM64 job `117731221299`, head `7b1dfa8`, passed the corrected observatory baseline but failed `TestE2EJsonlStable`: one independent happy run recorded no signal, the other recorded kill with 5s grace; both recorded confirmed empty job ownership. AMD64 passed its full suite. This is a fixture assumption exposed by real ownership observations, not an ignored failure.
- Mechanism: fake session waits for native exit before Done; conclude then calls the ladder to clean remaining job ownership. Its first job query can be empty (no signal) or nonempty/error (kill attempted), followed by confirmed empty. The native log does not reveal which query condition caused the ARM64 difference; no specific cause is claimed. Deterministic post-Wait member/error/empty sequences prove the distinct valid receipts.
- Approved bounded deviation: `tests/e2e/tui_test.go` now compares each entire stopped envelope, without normalisation, to its own durable journal event. Other envelopes retain strict cross-run comparisons and event count/order checks. Counterfactuals reject altered confirmation, omitted/invented kill evidence, event identity and extra fields. Inert comparator observed RED before implementation; focused packaged JSONL and evidence counterfactual race tests PASS (3.362s).
- `TestNaturalExitJobSnapshotDeterminesSentSignals` exercises happy native exit and scripted first job snapshots. Wine development checks PASS for empty/member/error (0.46/0.44/0.45s); initial assertion used the run-level success label and was corrected to the existing worker state `succeeded_native`. Hosted native evidence for this new regression remains pending. Production semantics and qualification rows are unchanged. Root reviewed the oracle as sound; its import-grouping finding is fixed.

### Task 4 — native full-suite proof and completion record (2026-10-18)

- CI [39148306697](https://github.com/turbokast/mythhelm/actions/runs/39148306697), source `1f21d0936207cd775b343a0a5562a8cdd5a61d35`, is green. Both hosted architectures passed all 29 named launch/stop tests. AMD64 job `117736819762` passed the full race suite (workers30.352s, E2E91.325s); ARM64 job `117736819760` passed the full non-race suite (workers30.342s, E2E120.152s). This verifies the strict survivor membership tests and both integration corrections without promoting qualification rows.
- Independent final E2E-oracle review returned Ready; coordinator rerun of its focused race tests passed2.861s. Integrated local full race gate passed85.3s, all six markers fresh before lifecycle edits. Signed merge `c3f3f5d` incorporates main `2b0013b` documentation changes only; Go markers remain fresh. Completion/handoff record the actual production APIs, pending later-task contracts and all approved deviations. Final metadata-head CI and PR merge remain the coordinator's responsibility.
- `first_pass:false` is retained: the initial lint gate rejected obsolete unused suppressions; earlier native full suites exposed the precise Windows baseline and independent-run signal comparison assumptions, both corrected and now verified on hosted Windows. No error was dismissed as unrelated or normalized out of stop evidence.

### Task 8 — empty evidence matrix and leaf ownership

Codex implemented the routed go-implementer task in `418c4a3` (PR #500). Runtime parsing is bounded and atomic; malformed rows cannot retain earlier support, and exact architecture/surface lookup never inherits another pair's capabilities. A zero matrix has two explicit unknown fields. No support rows are shipped; Task 11 remains responsible for native qualification and FR-1–FR-4 coverage.

Source-test existence belongs to the AST exactness test, since an installed runtime cannot inspect repository source. Review clarified that the bounded search must include Windows supervisor, fake and E2E sources as well as workers, so Tasks 5–7 evidence will be usable without an import cycle. The test checks test-function signatures and keeps missing-name, malformed-table, CI-job/run, wrong-architecture and wrong-surface fixtures. Counterfactuals ignoring architecture, retaining partially parsed evidence, discarding test references, bypassing CI validation, collapsing capability fields and adding an actual fake-adapter dependency all failed their corresponding checks; restored focused race suite passed all seven tests (1.263s).

The first full gate failed on sandbox-denied local sockets and containment HOME fixtures beneath the temporary worktree's /tmp mount. The unchanged source and assertions passed the full race suite (386.0s) after relocation to an isolated repository-local worktree with local socket access and private runtime/state directories. Lint then required constant errors to use `errors.New`; that mechanical correction passed lint with zero issues (12.3s), and the refreshed race gate passed (148.1s). `first_pass:false` retains both failures. These local parser and suite results are not native Windows qualification evidence.

### Task 9 — capability evidence remains surface-specific

PR #503 (`a6c8572`) connects the fake adapter to the leaf matrix without importing workers. Its optional per-instance matrix distinguishes the embedded default from an explicitly empty injected matrix. Both return unknown with no matching row; architecture, surface and the two capability values are never inferred. Prepare supplies runtime architecture. Unexported platform seams exercise the same Windows Prepare branches on contributor hosts while public entrypoints select the runtime OS.

Claude Code remains unsupported and blocked before executable access because native-harness qualification is missing, even though worker Job Object ownership exists. Task 11 must not promote its native surface from fake lifecycle evidence. All three named acceptance tests passed under race; seven overlay mutants proved exact lookup, independent fields, architecture wiring and both native refusals. Independent code/architecture review returned Ready. Full race passed in 213.1s; no Windows qualification row was added.


### Task 5 — creation-time event contract and native acceptance (2026-10-20)

Task 3 already persisted `Identity.NativeStartTime` before assignment and retained read-failure cleanup. Task 5 adds only the Windows launched-event field in production. Its equality test originally failed because that event omitted the field while worker.json matched the live native. Unix identity golden/event omission and ingestion projection checks remain exact; counterfactuals reject null/zero creation time, strict extra-field decoding and ignored read errors.

Initial hosted Task 5 CI exposed an E2E comparison between separate native creation times. The approved oracle correction binds each raw launched event to its own journal and independently persisted worker.json identity before normalizing only its Windows timestamp. Missing/changed/extra evidence and Unix field presence remain rejected. Later full suites exposed the existing stop-test watchdog assumptions; standalone PR505 fixed its measurement boundary, late ownership cleanup and observer clock, added failure diagnostics, isolated its termination regression and bounded Windows package scheduling. The original hosted stall phase was not identified; slower package execution supports a contention hypothesis, not proof of its cause. No Task 5 production scope expanded to address those fixtures.

Source CI [39416955906](https://github.com/turbokast/mythhelm/actions/runs/39416955906), head `b665237`, is green. Both native architectures pass `TestLaunchRecordsNativeStartTimeWindows` and both cases of `TestStartTimeReadFailureTerminatesJob`. AMD64 full race workers45.730s/E2E66.170s; ARM64 full suite workers37.205s/E2E71.691s. Wine remains development-only. The source merge consumed the final independently reviewed PR505 fixture; Task 5 producer and E2E source remained byte-identical to reviewed `067c453`. Final metadata-head CI is still required before merge, and Task 6 remains untouched until Task 5's merge is verified.


### Task 6 — retained process objects and crash evidence (2026-10-20)

A development Wine regression showed that `GetProcessTimes` can still return creation time for an exited process when another handle retains its kernel object. The original probe returned a timestamp with nil error; `TestProcessStartTimeRejectsExitedWindows` failed in 0.22s. A zero-time wait on a pinned process handle now rejects signalled objects, while access and query errors remain unknown. This is a liveness observation only: immediate subsequent exit cannot authorize signalling or release ownership. Production consumers were checked across workers, supervisor, control and migration.

The new six-hook worker crash tests pass in Wine (2.42s), including a live suspended pre-assignment identity (0.37s). A delayed-persistence overlay fails that assertion with both native fields absent (0.32s); removing kill-on-close fails the assigned suspended boundary because the native remains live (0.56s). Exact-process cleanup reaps the suspended residual. A separate PID-only matching overlay fails the retained mismatch case on Linux. Restored focused Linux race recovery tests pass in 4.236s. These are development checks, not hosted Windows qualification.

Independent code and architecture review returned Ready on source `20d86fc`. The exact public PR508 prerequisite `7bc90af` was integrated for validation by signed merge `b1943c6`; all eight reviewed task/CI/docs source blobs stayed identical. Task 6 must wait for the prerequisite's actual merge before its own merge. Initial lint rejected British-spelling comments, an unused fixture parameter and a missing own-binary suppression; those mechanical corrections passed with zero issues. Native CI and the final source gates remain the merge checks, and no support row changes.

The full local race gate on integration `b1943c6` passed in 335.2s: supervisor314.176s, CLI126.741s, workers23.899s and E2E31.726s. Formatting, vet and tidy gates passed as well. This validates the reviewed prerequisite source used for that run; subsequent prerequisite changes still require matching gate markers before publication.

Final public prerequisite `9ab4ba9` adds only two invariant-citation comment lines and was signed-merged as `2aebcec`, again preserving all eight reviewed task source blobs. The actual ordinary wrapper refresh passed with fingerprint `70e107105f7c`: race146.5s (CLI139.390s, migration71.474s, remaining packages cached from the full passing run), formatting0.3s, vet0.9s and tidy0.1s. Whole-repository Windows AMD64 and Darwin ARM64 vet also pass. Publication still waits for PR508's actual merge and signed integration of main; unchanged source gates remain fresh only when their fingerprints match that final tree.
