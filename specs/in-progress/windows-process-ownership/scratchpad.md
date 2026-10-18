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
