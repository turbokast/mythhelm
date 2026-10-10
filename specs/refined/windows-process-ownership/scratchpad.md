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
- Task 6 crash points: boundaries (i), (ii) and (iv) have no named injection point; AC-3.2 supervisor restart has no Windows test.
- Task 3 spike must be a real run on `windows-latest` and `windows-11-arm` (parent-job query result, kill-on-close job membership), with defined behaviour per outcome (D11).
- `launch_error` scope: Windows only (N4) and which error text may be recorded (native paths).
- Tasks 10 and 11 each mix docs and core files; `TestDocsMatchWindowsMatrix` needs fixed anchors (the "Supported today" bullet, `docs/limitations.md:14-15`) and all four docs of AC-5.4.
- No named test for NFR-3, NFR-4, AC-2.4, AC-3.3 (crash-behaviour column).
- Acceptance items that already pass on main (Task 4 ladder tests, Task 5 ingest test, Task 7 tests) need a named failing state; Task 7's `blocked` escape for AC-6.4 needs an AC change or a `Hooks.Interrupt` path; Tasks 7 and 9 need a packaged-binary item or a line saying why not.
- `STILL_ACTIVE` is absent from `x/sys` v0.48.0 (add to D7); `requirements.md` impacted components mention `internal/control/support_test.go`, which no task edits.

## Discoveries
