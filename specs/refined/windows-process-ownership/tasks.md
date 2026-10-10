## Windows Process-Tree Ownership — Tasks

### Dependencies

- Prerequisite specs: none outside this spec; `specs/*/dogfood-slice/` and `specs/*/supervisor-service/` are shipped. Cross-spec order: `specs/*/supervised-stop-recover/` and `specs/*/supervisor-migration/` have merged every task (both are in `specs/unfinalized/`), so no task waits on them; the files they changed (`internal/workers/worker.go`, `internal/supervisor/pipeline.go`, `internal/control/SUPPORT.md`, `internal/control/support_test.go`) are read from main at bc65309. `docs/decisions/0016-migration-import.md` is on main, so Task 1 takes 0017.
- Order: Task 1 (decisions) and Task 2 (test fixture) have no dependency and touch disjoint Files, so they may run in parallel. Task 3 (job launch) follows both. Tasks 4 and 5 share `worker.go` and `proc_windows.go` with Task 3 and each other and run sequentially: 3 → 4 → 5; Task 6 (recovery, `recover.go` and `proc_windows.go`) follows 5 and Task 7 follows 6 for ordering of behaviour, not for shared files. Task 8 (matrix leaf package) follows Task 4 and may run in parallel with 5–7 (disjoint Files). Task 9 follows 8. Task 10 follows 7 and 9. Task 11 (maintainer) is last.
- **Gates for every task.** Run `gofmt -w` on touched Go files before any check, then from the tree root `go vet ./...`, `go test -race ./...`, `go mod tidy -diff` (must print nothing) and `golangci-lint run`; Windows-only files also run `GOOS=windows go vet ./...` and `GOOS=darwin go vet ./...`; every task runs `scripts/ci/check-public-hygiene.sh`. A Windows-only test counts only with the `windows-latest` job's output cited (a Linux cross-compile is not evidence, v2 §17.1). A task is not complete because files exist or an agent reported success.
- **Completion convention.** Append ` ✅ COMPLETED` to the heading, keep every original field, and add `Status` (`✅ Completed — …; PR #<n>`), `Implementation` (at most 3 lines, plus commit SHAs), `Spec deviations` (`None`, or each deviation with its reason) and `Files modified`.
- **Commits.** Every commit is signed off (`git commit -s`). Process-ownership changes name the Task 1 decision record in the PR description.

---

## Implementation Tasks

### Task 1 — decision record: Windows process ownership

- **Domain/agent**: maintainer
- **Budget**: standard
- **Depends on**: None
- **Change**: Decide OQ-1 (kill-on-close vs surviving job), OQ-8 (native start time scope) and OQ-11 (whether kill-on-close plus confirmed worker death is I24 evidence), and record them, with D8, in an ADR that supersedes the Windows statements of ADR 0004. Tasks 3 to 7 implement the defaults (a, Windows-only, no) unless the ADR says otherwise.
- **Files**:
  - `docs/decisions/0017-windows-process-ownership.md` (0016 is taken by `0016-migration-import.md` on main)
- **Acceptance**:
  - The ADR (whatever number the reconciliation gives it, found with `ls docs/decisions/ | grep windows-process-ownership`) has a `Decision` section where `grep -nwE "OQ-1|OQ-8|OQ-11"` finds all three, and cites v2 §6.4 and I24. Fails before: no file.
  - A `Supersedes` line names ADR 0004 and its process-model, stop-ladder and capability statements (cited by heading; `grep -n "Windows" docs/decisions/0004-slice-process-model.md` finds them). Fails before: no file.
  - If a decision differs from its default, the ADR lists the tasks and ACs that change.
- **Invariants touched**: I06 (v2 §6.4: a stop stays unconfirmed until reconciliation), I18 (one process owner per attempt), I24 (a lease or heartbeat timeout is not proof a writer stopped).

### Task 2 — fake adapter child-tree fixture for Windows

- **Domain/agent**: go-implementer
- **Budget**: complex (cross-platform process creation)
- **Depends on**: None
- **Change**: Add a `spawn_tree` scripted step that starts a child and a grandchild on Windows and reports their PIDs and creation times, so the Windows lifecycle tests have a native that has descendants (the existing `spawn_escapee` is Unix-only).
- **Files**:
  - `adapters/fake/agent.go` (step table: `spawn_tree`, argument `report_path`)
  - `adapters/fake/tree_windows.go` (starts child and grandchild with `os.StartProcess` and the null device for stdin, stdout and stderr, so they hold no pipe of the native; attempts one `CREATE_BREAKAWAY_FROM_JOB` child; defines `spawnEscapee` as an error on Windows)
  - `adapters/fake/tree_other.go` (`//go:build !windows`, returns an error naming the step)
  - `adapters/fake/escapee_other.go` (retag `!unix && !windows`)
  - `adapters/fake/scenarios/tree.json`
  - `adapters/fake/fake_test.go`
- **Produces**: scenario step `{"op":"spawn_tree","report_path":"<workdir-relative file>"}` (decoded with `DisallowUnknownFields`); the file receives `{"role":"child|grandchild","pid":N,"start_time":"<RFC 3339 UTC>"}` per started descendant and `{"role":"breakaway","error":N}` for the breakaway attempt (plus `"pid"`/`"start_time"` if it unexpectedly started).
- **Acceptance**:
  - `TestSpawnTreeReportsDescendants` (Windows): after the step, the report file has a `child` and a `grandchild` line whose PIDs are alive with the reported creation times (checked with `OpenProcess` plus `GetExitCodeProcess == STILL_ACTIVE`), and a `breakaway` line. Fails before: unknown step.
  - `TestSpawnTreeUnsupportedOffWindows` (Linux and macOS): a scenario using the step loads (the op is known) and the run ends with the run-time error from `tree_other.go` naming `spawn_tree`; on main the loader rejects it as an unknown op, so the test fails before.
  - `TestScenariosParse`: `scenarios/tree.json` loads through the existing scenario loader and an unknown step name in a copy of it is rejected.
  - `GOOS=windows go vet ./adapters/fake/` passes.
- **Test plan**: Windows-tagged test using the fake agent re-executed as in the existing escapee test; the off-Windows test is untagged.
- **Invariants touched**: I13 (fixtures need no credentials), I14 (a fixture is not a qualification).

### Task 3 — Job Object launch ownership

- **Domain/agent**: go-implementer
- **Budget**: complex (process ownership and cross-platform behaviour)
- **Depends on**: Task 1, Task 2
- **Change**: Create the Job Object, start the native suspended, assign it, resume it, and fail closed on every step, so no descendant exists outside the job (FR-1). First step: a spike run in CI on both Windows jobs (`windows-latest` and `windows-11-arm`): a throwaway test logs `IsProcessInJob(GetCurrentProcess(), 0)` and, when true, `QueryInformationJobObject(0, JobObjectExtendedLimitInformation)` `LimitFlags`; the run ids and the logged values per runner go in `scratchpad.md` (OQ-3, D11). A compile-only check under `GOOS=windows` is not the spike. `parentJob` is defined in design §2.2: `(false, nil)` when the worker is in no job; otherwise `(LimitFlags & JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE != 0, nil)`; any API error is returned as an error. If the spike shows any Windows runner reporting `(true, nil)` from `parentJob` (it already sits inside a kill-on-close job), Task 3 stops after the spike and escalates to the maintainer before changing production code; the options then are breakaway at spawn, or refusal with the out-of-process tests reworked. Stubbing `parentJob` in the lifecycle tests is not an option, because a stub cannot reach out-of-process workers (D11).
- **Files**:
  - `internal/workers/job_windows.go` (jobObject, newJob, assign, terminate, members, close, resume helper, lazy kernel32 bindings)
  - `internal/workers/job_other.go` (`//go:build !windows`, empty `jobObject`)
  - `internal/workers/proc_windows.go` (`nativeAttr` adds `CREATE_SUSPENDED`; launch hook; `parentJob`)
  - `internal/workers/worker.go` (`ownedProc.job`; `launcher.Launch` calls the hook; `launcher.unconfirmed`; `launch_error` on `attempt.stopped`; the `jobOps` field)
  - `internal/workers/job_windows_test.go`
  - `internal/workers/worker_windows_test.go`
- **Produces**: `type jobObject`; `func newJob() (*jobObject, error)`; `func (*jobObject) assign(windows.Handle) error`; `func (*jobObject) members() ([]ProcessIdentity, error)`; `func (*jobObject) terminate() error`; `func (*jobObject) close() error`; `attempt.stopped.launch_error string`.
- **Acceptance**:
  - `TestJobHandleNotInheritable` (Windows): `GetHandleInformation` shows `HANDLE_FLAG_INHERIT` clear. Fails before: no job.
  - `TestJobHasNoBreakawayLimits` (Windows): the extended limit flags contain neither `BREAKAWAY_OK` nor `SILENT_BREAKAWAY_OK`, and the fixture's `breakaway` line reports `error == 5` (`ERROR_ACCESS_DENIED`); the test fails if the line is missing, carries another error, or reports a PID (AC-1.5, AC-6.2).
  - `TestNativeIsInJobBeforeResume` (Windows, a hook that runs before `ResumeThread`): `members()` already contains the native's PID. Catches an implementation that resumes before assigning.
  - `TestJobCreateFailureEndsLaunchFailed` (Windows, `newJob` field replaced): no native exists afterwards, the attempt ends `failed_native`/`launch_failed` and `attempt.stopped.launch_error` carries the Win32 text and code (AC-1.3).
  - `TestAssignFailureTerminatesSuspendedNative` (Windows, `assign` field replaced): the suspended process is gone by PID and creation time and the same state results (AC-1.3, AC-1.6).
  - `TestUnconfirmedTerminationIsStopUnconfirmed` (Windows, termination injected to fail): the attempt ends `interrupted`/`stop_unconfirmed` (exit 6 path, AC-1.3).
  - `TestKillOnCloseParentJobRefused` (Windows, `parentJob` replaced to report `(true, nil)`): the launch fails `launch_failed`; `TestParentJobQueryErrorFailsLaunch` (`parentJob` returns an error): the launch also fails, never read as "not in a job" (AC-1.6, I02, I09).
  - `TestParentJobSemantics` (Windows, the real `parentJob`, run in a child process re-executed through `TestMain`): a child outside any job of its own making reports its runner's value (logged, and `(false, nil)` where the spike found no job); a child assigned to a job without kill-on-close reports `(false, nil)`; a child assigned to a kill-on-close job reports `(true, nil)`. Fails before: no `parentJob`. Where the runner itself reports `(true, nil)`, the first case asserts that value.
  - `GOOS=linux go vet ./internal/workers/` and `GOOS=darwin go vet ./internal/workers/` pass: the stand-in compiles.
- **Test plan**: Windows-tagged tests using the Task 2 fixture; failure injection through worker struct fields (`go-conventions.md`), never package variables: the `jobOps` struct of function fields (design §2.2), with the six boundary hooks `afterIntent`, `afterJob`, `afterSpawn`, `afterStart`, `beforeResume` and `afterResume` that Task 6 sets to crash the worker. Shown red on main: there is no job. The launch order is intent, parent-job check, job, spawn, identity record, assign, resume (D10).
- **Invariants touched**: I18 (the native is in the job before it can start a child), I06 (a failed assignment cannot leave an unowned process), I12 (a worker launches once, `worker.go:1125`), I02 and I09 (an unknown parent-job state blocks).

### Task 4 — job-wide stop, confirmation and descendant scan

- **Domain/agent**: go-implementer
- **Budget**: complex (process ownership, signals, cross-platform behaviour)
- **Depends on**: Task 3
- **Change**: `StopKill` terminates the job, `GroupGone` means an empty job, the descendant scan becomes `job-object`, and the Windows ladder ships under its own version string (FR-2, AC-4.1).
- **Files**:
  - `internal/workers/proc_windows.go` (`Signal`, `GroupGone`, `descendantScan = "job-object"`, `descendants`)
  - `internal/workers/proc_unix.go` (`descendants` for Unix: the marker scan, moved behind the per-OS function)
  - `internal/workers/worker.go` (`unresolvedDescendants` calls `descendants`; close the job after `attempt.stopped`)
  - `internal/supervisor/pipeline.go` (`stopLadderVersion(goos string) string`)
  - `internal/supervisor/pinning_test.go` (the two `stop-ladder/v1` assertions become per-OS)
  - `internal/supervisor/ladder_version_test.go` (new, `package supervisor`)
  - `internal/workers/worker_windows_test.go`
  - `internal/workers/worker_test.go` (`TestWindowsFakeStopConfirmed` and the Windows skip messages at the former `unavailable` assertions)
  - `internal/adapter/adapter.go` (comment at lines 162-163: Windows `GroupGone` now means an empty job)
- **Produces**: `func stopLadderVersion(goos string) string` (`"stop-ladder/v1"` for every OS but `windows`, `"stop-ladder/v2-windows"` for it); `func descendants(p *ownedProc, attemptID string) ([]int, []ProcessIdentity, error)` per OS (the same signature in the design).
- **Acceptance**:
  - `TestStopKillsChildAndGrandchild` (Windows): the Task 2 fixture's descendants are gone after stop (the Windows probe: `OpenProcess` fails, or `GetExitCodeProcess` is not `STILL_ACTIVE`, with a matching creation time; `ProcessStartTime` alone is not enough because a held handle keeps returning a time, `worker_test.go:693`), `attempt.stopped.confirmed` is true and `descendant_scan` is exactly `job-object`. On main the grandchild survives and the scan reads `unavailable` (AC-6.1, AC-2.2, AC-4.1).
  - `TestCreateNewProcessGroupDescendantDies` (Windows): the `CREATE_NEW_PROCESS_GROUP` descendant dies with the job (AC-6.2, first leg).
  - `TestStopReportsSurvivorsWithIdentity` (Windows, `terminate` replaced with a no-op): the attempt ends `interrupted`/`stop_unconfirmed` (the native survives, `worker.go:962`) with `confirmed` false and the survivors named; a second case with the native dead and a grandchild alive (`members` returning it) also ends `interrupted`/`stop_unconfirmed`, with `unresolved_identities` listing the grandchild's PID and creation time (AC-2.3). The worker concludes through the bounded stop wait (D12), so neither case hangs.
  - `TestEnumerationFailureIsFailedScan` (Windows, `members` replaced with an error): `descendant_scan` is `failed`, `GroupGone` is false, and the attempt is `interrupted`/`stop_unconfirmed` (AC-4.1; `classify` order).
  - `TestWindowsLadderHasOnlyKill` and `TestSignalRejectsNonKillOnWindows`: the ladder is `[StopKill 5s]` and `Signal(StopTerminate)` errors (AC-2.5, OQ-2 default).
  - `TestStopLadderVersionPerOS`: `windows` returns `stop-ladder/v2-windows`, `linux`, `darwin` and `freebsd` return `stop-ladder/v1`, and `attempt.stopped.ladder_version` on Windows equals the former (AC-2.1, N4).
  - `TestUnixDescendantScanUnchanged` (Unix): the existing `TestUnresolvedDescendantReported` and `TestOrphanPIDsContract` pass unchanged through the new per-OS function.
  - `TestStopTimeoutEndsStopUnconfirmed` (Windows, `terminate` replaced with a no-op, the `slow` fixture holding the native): the worker concludes `interrupted`/`stop_unconfirmed` within the ladder grace plus `waitDelay` (5 s) and sends no second ladder, instead of waiting on the native's 600 s sleep (NFR-1, D12).
- **Test plan**: as Task 3; the version tests are cross-platform.
- **Invariants touched**: I06 (confirmed only when the job is empty), I09 (an unavailable scan is never read as clean), I14 (the kill rung is not labelled graceful).

### Task 5 — native start time in the launch record

- **Domain/agent**: go-implementer
- **Budget**: complex (persisted launch identity across platforms)
- **Depends on**: Task 4
- **Change**: Record `native_start_time` beside `native_pid` in worker.json and the `attempt.launched` payload on Windows only (D4, D8), so recovery matches PID and creation time (AC-1.4, AC-4.4). No schema change; Unix output is byte-identical.
- **Files**:
  - `internal/workers/worker.go` (`Identity.NativeStartTime`, `Launch` write, `evLaunched` payload)
  - `internal/workers/identity_write_windows_test.go`
  - `internal/workers/launch_record_test.go` (new: Unix golden check)
  - `internal/workers/testdata/worker-json-unix.golden` (new directory; captured from main before the change)
  - `internal/supervisor/ingest_ledger_test.go`
- **Produces**: `Identity.NativeStartTime *time.Time` (`json:"native_start_time,omitempty"`); the same key in the `attempt.launched` payload on Windows.
- **Acceptance**:
  - `TestLaunchRecordsNativeStartTimeWindows` (Windows): worker.json and the `attempt.launched` payload carry `native_start_time` equal to `ProcessStartTime(native_pid)`. Fails before: field absent.
  - `TestUnixLaunchRecordUnchanged` (Unix): the serialized Identity for a fixed fixture equals the golden file captured on main, so the field is absent and not zero (N4, I09).
  - `TestStartTimeReadFailureTerminatesJob` (Windows, `startTime` replaced with an error): the failure happens right after `cmd.Start()`, before assignment and resume, so the suspended process is terminated and confirmed gone (nothing ran) and the job is empty, the attempt ends `failed_native`/`launch_failed`, and with termination also failing it ends `interrupted`/`stop_unconfirmed`. Fails before: the field and seam are absent.
  - `TestIngestAcceptsLaunchedWithStartTime`: an `attempt.launched` event with the extra field ingests without error and leaves the `attempts` row columns as before.
- **Invariants touched**: I09 (absent is `unknown`), I12 (launch identity is matched, not replayed), I18 (match on PID and start identity).

### Task 6 — Windows recovery verdict

- **Domain/agent**: go-implementer
- **Budget**: complex (process ownership and recovery)
- **Depends on**: Task 5
- **Change**: On Windows, recovery reads worker.json, checks the native by PID and `native_start_time`, and under the Task 1 defaults keeps ownership unresolved after a worker crash; pins the launch-boundary crash outcomes (FR-3, FR-4). Descendants are not recorded, so none are matched.
- **Files**:
  - `internal/workers/proc_windows.go` (identity probe helper)
  - `internal/supervisor/recover.go` (Windows branch of the orphan lookup; per-boundary outcome; the Unix path is unchanged)
  - `internal/supervisor/recover_other_test.go` (new: Unix and macOS recovery outcomes equal the existing ones)
  - `internal/workers/recover_windows_test.go`
  - `internal/workers/worker_test.go` (the `TestMain` crash-worker mode)
  - `internal/supervisor/recover_windows_test.go`
- **Acceptance**:
  - `TestRecoverAfterWorkerCrashWindows`: the worker is crashed with a live fake native; the native and every Task 2 descendant are gone by PID and creation time (checked from the test side), and recovery's outcome is `ownership_unresolved` unless Task 1 decided otherwise (AC-6.5, AC-4.2, AC-4.3).
  - `TestRecoverNeverMatchesReusedPID`: a live process holding the recorded PID with a different creation time is not reported as the native (not in `UnresolvedPIDs` or the notice) and nothing is signalled (AC-4.4). Fails if matching were by PID alone.
  - `TestSuspendedNativeIsRecordedBeforeAssign` (workers package): the worker is crashed in the `afterStart` hook (after `cmd.Start()` and the identity record, before `assign`); worker.json then holds `native_pid` and `native_start_time` of a live suspended process, and the Task 2 fixture has written no report line (its code never ran). Teeth: the same test with the identity record moved after `assign` finds no `native_pid` and fails. Recovery's side of this boundary is row (iii) of `TestRecoverLaunchBoundaryOutcomes`.
  - `TestLaunchBoundaryCrashRecords` (workers package, `internal/workers/recover_windows_test.go`): for each boundary the worker is re-executed through a `TestMain` worker mode that builds the worker as `runInProcess` does (`worker_test.go:652`), sets the one `jobOps` hook for the boundary to `os.Exit(3)`, waits for the exit, and asserts from the test side what remains: worker.json and the process by PID and creation time, never recovery. Boundary, crash point and the expected leftovers:
    - (i) job created, native not spawned: `afterJob`; worker.json has the intent and no `native_pid`; no process was spawned (a hook-written marker file is absent). `afterIntent`, a crash before the job, persists the same state and is a second case of this row.
    - (ii) spawned suspended, identity not yet recorded: `afterSpawn` (the hook writes the PID it receives to a side file); worker.json has the intent and no `native_pid`, identical to (i); the suspended process is alive by that PID, outside any job. This is the OQ-13 residual, and the test asserts it so it cannot widen unnoticed.
    - (iii) identity recorded, not yet assigned: `afterStart`; worker.json has `native_pid` and `native_start_time`; the suspended process is alive and its start time equals the record.
    - (iv) assigned, not yet resumed: `beforeResume`; the same worker.json record as (iii); the native is gone by PID and creation time (kill-on-close ended it with the worker).
    - (v) resumed: `afterResume`; the same record; the native and every Task 2 descendant are gone by PID and creation time.
    - (i) and (ii) have byte-identical worker.json apart from the timestamps, and (iii) to (v) have the identical `native_pid` and `native_start_time` fields; the test compares them. Fails before: no hooks.
  - `TestRecoverLaunchBoundaryOutcomes` (supervisor package, `internal/supervisor/recover_windows_test.go`): recovery runs over worker.json files the test writes with the exported `workers.Identity`, one per persisted state, because the supervisor package cannot reach the workers package's unexported hooks; the crash points and hooks are those of `TestLaunchBoundaryCrashRecords`. Rows: (i) and (ii), intent only: the outcome is `ownership_unresolved`, no PID is named, and the two rows are one case, since recovery cannot tell them apart. (iii), `native_pid` and `native_start_time` of a live suspended process the test started with `CREATE_SUSPENDED` (the test is its parent, so it is genuinely outside any job): `ownership_unresolved`, the PID appears in the notice and in `UnresolvedPIDs`, and the process is still alive afterwards (never signalled, I24). (iv) and (v), the same record for a process that no longer exists: `ownership_unresolved`, nothing is named (the PID appears in the notice only when both PID and `native_start_time` match a live process). In every row no second native is started (the attempt's launch intent is unchanged, the number of attempt rows is unchanged, and the journal holds no second `attempt.launched`; recovery itself moves the attempt to `interrupted` and then `quarantined`, so the row's state is not asserted unchanged). The observable behind `ownership_unresolved` is `Mode == "interrupted_and_quarantined"` and `errors.Is(err, ErrOwnership)` (AC-6.3, AC-3.2).
- **Invariants touched**: I06, I09 (an unmatched or unreadable identity is unresolved, not gone), I12 (reconcile before retry), I18, I24 (a timeout is not proof of death).

### Task 7 — detach and interrupt behaviour on Windows

- **Domain/agent**: go-implementer
- **Budget**: complex (console signals and detach)
- **Depends on**: Task 6
- **Change**: Pin and, where needed, fix the Windows behaviour of CLI exit, console close, Ctrl-C/Ctrl-Break and the second-interrupt detach against the shipped Unix contract (AC-3.1, AC-6.4).
- **Files**:
  - `internal/cli/run.go` (only if a console event reaches the worker; the first interrupt requests a stop, the second detaches)
  - `internal/supervisor/interrupt_windows_test.go` (new)
  - `internal/workers/detach_windows_test.go` (new)
- **Acceptance**:
  - `TestSurvivesCLIExitAndConsoleClose` (Windows): after the CLI process exits or its console closes, the worker is alive by PID and creation time and reconnect identifies the same launch.
  - `TestFirstInterruptRequestsStop` (Windows): one Ctrl-C and one Ctrl-Break each record `attempt.stop_requested` and exit 130 once confirmed.
  - `TestSecondInterruptDetaches` (Windows, using the fake scenario that holds the native past the first interrupt, the `ignore-sigint` analogue): a second interrupt exits 6 with the worker alive. Because the Windows ladder confirms almost at once, this case is `blocked` (named in the matrix as detach untested) unless the scenario can hold the stop open; it is never skipped silently.
  - Where hosted runners cannot drive console events, the test is marked `blocked` with the runner reason in its skip message and the matrix leaves `worker_detachment` `unknown` (honesty register); it is never skipped silently.
- **Invariants touched**: I06 (UI exit does not transfer ownership), I18.

### Task 8 — Windows process-support matrix (leaf package)

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 4
- **Change**: Add `internal/processsupport` with the evidence-backed matrix and the function that derives the Windows capability from it (FR-5, D9). The matrix ships with no rows. The Windows note in `internal/control/SUPPORT.md` is corrected here (`supervised-stop-recover` has merged, so the file is read from main).
- **Files**:
  - `internal/processsupport/SUPPORT.md` (header and no rows)
  - `internal/processsupport/support.go` (`Support`, `Windows`, embedded table parser)
  - `internal/processsupport/support_test.go`
  - `internal/control/SUPPORT.md` (lines 41-44, point to the new matrix)
- **Produces**: `type Support struct{ ProcessTreeOwnership, WorkerDetachment adapter.Tri }`; `func Windows(goarch, surface string) Support` (embedded table); `type Matrix`; `func Parse(table []byte) (Matrix, error)`; `func (Matrix) Windows(goarch, surface string) Support`.
- **Acceptance**:
  - `TestSupportMatrixMatchesEvidence`: with a fixture row naming a `_windows_test.go` test that exists (found by `go/parser`), `Windows` returns `Supported` for that pair only; with a row naming a missing test the test fails; with no row `Windows` returns `Unknown` for both fields. Mutation fixtures show it fails when a row is edited (AC-5.1). Fails before: package absent.
  - `TestUntestedPairIsUnknown`: driven by fixture rows, a pair absent from the table returns `Unknown` (an arm64 pair when only an amd64 row exists, and the claudecode surface when only the scripted one has a row) (N6, N1); `TestMalformedTableIsUnknown`: `Parse` of a malformed table errors and `Windows` on it returns `Unknown` for every pair.
  - `TestRowRequiresWindowsJobAndRunID`: a row whose CI-job cell lacks `windows-latest` or `windows-11-arm`, or a run id, is rejected (NFR-2).
  - `TestNoLeafCycle`: `go list -deps ./internal/processsupport` contains `internal/adapter` and neither `adapters/fake`, `adapters/claudecode` nor `internal/workers`.
- **Invariants touched**: I14 (a capability has versioned evidence), I09 (untested is `unknown`).

### Task 9 — adapter capability derivation

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 8
- **Change**: The fake adapter derives its Windows `process_tree_ownership` and `worker_detachment` from `processsupport.Windows` (filling `Probe.Arch` at its Prepare-time probe); the claudecode adapter keeps refusing Windows and reports `unsupported`, with a refusal message naming the missing Windows qualification (FR-5).
- **Files**:
  - `adapters/fake/fake.go` (Windows branch; `Arch` at the probe near line 165)
  - `adapters/fake/fake_test.go`
  - `adapters/claudecode/launch.go` (message text)
  - `adapters/claudecode/probe.go` (message text)
  - `adapters/claudecode/probe_test.go`
  - `adapters/claudecode/COMPATIBILITY.md` (line 69: Windows statement)
- **Acceptance**:
  - `TestFakeWindowsCapabilityFollowsMatrix`: with a `Matrix` field parsed from a fixture row the fake reports `supported` for Windows; without it `unknown`; the existing `"windows"` case in the capability table test is updated (AC-5.2).
  - `TestClaudecodeCapabilitiesWindowsUnsupported`: a direct call to `Capabilities` with `OS: "windows"` returns `unsupported` for both fields, while `Prepare` and `probe` still refuse with `ErrCapability` and the message names the missing Windows qualification, not process-tree ownership (AC-5.3). Fails before: the message names ownership.
  - `TestPrepareProbeCarriesArch`: the probe built at the fake's Prepare call has a non-empty `Arch`.
- **Invariants touched**: I14 (the claudecode surface is not advertised), I02 (a missing capability blocks before launch).

### Task 10 — Windows documentation

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 7, Task 9
- **Change**: Correct the Windows statements in the user docs to the tested subset (AC-5.4), state that a launch is refused inside a kill-on-close job (D11, honesty register), and mark ADR 0004 as superseded.
- **Files**:
  - `docs/limitations.md` (the Windows lines claiming recovery and exit 7)
  - `README.md` (the Windows line)
  - `docs/decisions/0004-slice-process-model.md` (status line pointing to the superseding ADR)
  - `internal/processsupport/docs_test.go` (new)
- **Acceptance**:
  - `TestDocsMatchWindowsMatrix` (new, in `internal/processsupport/docs_test.go`): every Windows (arch, surface) pair the three docs name as supported is a row of the matrix, the docs name the untested subset and the launch refusal inside a kill-on-close job, and `docs/limitations.md` contains no unqualified sentence that recovery works on Windows (checked by anchoring on its Windows paragraph, not a file-wide match). Fails before: the doc claims recovery on Windows.
  - `scripts/ci/check-public-hygiene.sh` passes.
- **Invariants touched**: I14 (documentation claims only tested subsets).

### Task 11 — promote tested Windows rows

- **Domain/agent**: maintainer
- **Budget**: standard
- **Depends on**: Task 10
- **Change**: After Tasks 3 to 7 have passed on the Windows runners, add the matrix rows for the pairs whose tests passed, each citing the run id and job, and update the docs; leave any pair without a passing run absent.
- **Files**:
  - `internal/processsupport/SUPPORT.md` (rows)
  - `internal/processsupport/support_test.go` (only if a row changes a fixture expectation)
  - `docs/limitations.md` (the tested-subset list)
- **Acceptance**:
  - `go test ./internal/processsupport/` passes with the rows present, and each row cites a `windows-latest` or `windows-11-arm` run id (AC-5.1, AC-6.6, NFR-2).
  - A row exists only for a pair with a passing run; `Windows("arm64", "scripted-child-process")` is `Unknown` unless the `windows-11-arm` run passed (N6).
- **Invariants touched**: I14.
