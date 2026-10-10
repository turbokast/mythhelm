## Windows Process-Tree Ownership — Design

> Scope: `auto-confirmed (non-interactive)`: single spec, 11 tasks, one work stream (the Windows worker's owned tree). It touches core, adapters and docs, but the adapters and docs tasks cannot ship without the workers task, so it is not an epic. Normative source: mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md §6.3, §6.4, §17.1; I06, I12, I14, I18, I24; G04.

### 1. Current state (origin/main 7bf96c4)

- The native starts through `exec.Cmd` in `launcher.Launch` (`internal/workers/worker.go:1093-1129`): launch intent is written, `cmd.Start()` runs with `nativeAttr()`, then `native_pid`/`native_pgid` are recorded. `ownedProc` holds `cmd`, `stdout`, `pgid *int`, `waited`, `release` (`worker.go:1133-1141`).
- Windows: `nativeAttr` is `CREATE_NEW_PROCESS_GROUP|CREATE_NO_WINDOW`, `nativeGroup` returns nil, `Signal` accepts only `StopKill` and calls `Process.Kill`, `GroupGone` is `waited`, `descendantScan = "unavailable"`, `markedPIDs` returns `ErrUnsupported` (`internal/workers/proc_windows.go:17-67`). `detachedAttr` sets `CREATE_NEW_PROCESS_GROUP|DETACHED_PROCESS` for the worker.
- `unresolvedDescendants` returns early on `descendantScan == "unavailable"`; a scan error returns `failed`, which `classify` turns into `interrupted`/`unresolved_descendants` (`worker.go:905-935`). `attempt.stopped` carries `confirmed`, `unresolved_pids`, `sent`, `ladder_version`, `signals_sent`, `descendant_scan`, `unresolved_identities` (`worker.go:866-876`).
- The stop ladder is per OS in `stopLadder(goos)`: Windows is `[StopKill 5s]` (`adapters/fake/fake.go:171-173`); the version string is one constant, `stopLadderVersion = "stop-ladder/v1"` (`internal/supervisor/pipeline.go:804`), pinned at `:517`.
- Recovery consumes `workers.OrphanPIDs` (`internal/supervisor/recover.go:189,242,295`); off Linux it returns `ErrUnsupported` (`internal/workers/orphan_other.go:9`).
- Capability values come only from `adapters/fake/fake.go:82-88` and `adapters/claudecode/probe.go:126-133`; `claudecode` refuses Windows (`launch.go:70-72`, `probe.go:55-57`).
- `golang.org/x/sys` v0.48.0 is already a dependency (`go.mod:14`) and is used for `LockFileEx` and process times on Windows.
- Windows CI: `windows-latest` with `-race`, `windows-11-arm` without (`.github/workflows/ci.yml:60,73-77`).

### 2. Design

#### 2.1 Package layout (v2 §6.4)

Worker code stays in `internal/workers`. New files there: `job_windows.go` (the Job Object), `job_other.go` (`//go:build !windows`, the stand-in so `ownedProc` compiles everywhere). New leaf package `internal/processsupport` holds the Windows process-support matrix (§2.6): `internal/workers` already imports both `adapters/fake` (`worker.go:28,65`) and `adapters/claudecode` (`contain.go:16`), so the adapters cannot import `internal/workers`; the leaf imports only `internal/adapter`. `adapters/fake` gains a Windows child-tree step (§2.7). No new dependency.

#### 2.2 The job (AC-1.1–1.6)

```go
// job_windows.go
type jobObject struct{ h windows.Handle }

func newJob() (*jobObject, error)                     // CreateJobObject(nil attrs → non-inheritable); KILL_ON_JOB_CLOSE; no BREAKAWAY_OK / SILENT_BREAKAWAY_OK
func (j *jobObject) assign(proc windows.Handle) error // AssignProcessToJobObject
func (j *jobObject) terminate() error                 // TerminateJobObject(h, 1)
func (j *jobObject) members() ([]ProcessIdentity, error) // JobObjectBasicProcessIdList + ProcessStartTime per PID (identity type: internal/workers/orphan.go:14-17)
func (j *jobObject) close() error
```

`job_other.go` declares `type jobObject struct{}`; `ownedProc` gains `job *jobObject` (nil off Windows). `GetHandleInformation`, `IsProcessInJob`, the process-id-list struct and the `STILL_ACTIVE` constant (259, defined locally) are not in `golang.org/x/sys` v0.48.0; they are bound through `windows.NewLazySystemDLL("kernel32.dll")` in `job_windows.go` (D7).

Launch on Windows (default method, D2): `newJob` → `cmd.Start()` with `CREATE_SUSPENDED` added to `nativeAttr()` → record `native_pid` and `native_start_time` in worker.json at once (`ProcessStartTime`, before any assignment or resume) → open the process → `job.assign` → resume the main thread (`CreateToolhelp32Snapshot`, `Thread32First/Next`, `OpenThread`, `ResumeThread`). Recording first closes the unowned-process window as far as Windows allows: nothing kills a child when its parent dies, so a worker that dies between `cmd.Start()` and `assign` leaves a suspended native outside the job, but its primary thread has not run (it is inert and has no descendants) and its PID and start time are already on disk, so recovery names it as unresolved with identity instead of losing it. If `ProcessStartTime` fails, the launch is treated as failed: the worker terminates the process and job, confirms them gone and ends `failed_native`/`launch_failed`; if emptiness cannot be confirmed the attempt ends `interrupted`/`stop_unconfirmed` (a native whose identity cannot be recorded must not run, as `identityErr` does today, `worker.go:1087-1090`). The `jobOps` struct carries a `startTime func(pid int) (time.Time, error)` field for the test. The native cannot start a child before it is in the job. The job handle is created non-inheritable (AC-1.2). The worker already inside a kill-on-close job it has not left is detected by `QueryInformationJobObject` with a nil handle (the spike in Task 3 confirms the call and its not-in-a-job result); it and any assignment failure fail the launch (AC-1.6, OQ-3 default).

Failure reporting: `launcher.Launch` returns the error, so `w.start` fails and `conclude` runs with `launchFailed: true` (`worker.go:517-521`). The Win32 error text and code travel in a new `launch_error` string on `attempt.stopped`. When a suspended native could not be confirmed gone after an assignment failure, `Launch` sets a `launcher.unconfirmed` flag (the `identityErr` pattern, `worker.go:1087-1090`) and the `launchFailed` branch concludes with `Confirmed: false`, which `classify` ends `interrupted`/`stop_unconfirmed`.

Failure injection uses a `jobOps` struct of function fields on the worker (`newJob func() (*jobObject, error)`, `assign func(*jobObject, windows.Handle) error`, `terminate func(*jobObject) error`, `members func(*jobObject) ([]ProcessIdentity, error)`, `parentJob func() (killOnClose bool, err error)`, `beforeResume func(*jobObject) error`, `afterIntent func() error`, `afterStart func(pid int) error`, run after `cmd.Start()` and the identity record, before `assign`), defaulted to the real implementations and replaced by tests as `runInProcess` does (`worker_test.go:705`); never package variables (`go-conventions.md`). A `parentJob` error is a launch failure, never read as "not in a job" (I02, I09). The `native_launch_intent` write happens first, then `newJob`, then the spawn (job first, D10); `afterIntent` and `beforeResume` let tests crash the worker at each boundary.

#### 2.3 Stop and confirmation (AC-2.1–2.5, AC-4.1)

- `Signal(StopKill)` calls `job.terminate()` instead of `Process.Kill`; every other signal still errors (OQ-2 default).
- `GroupGone` is `waited && members empty`; if `members` returns an error it returns false (never read as gone). Because `classify` checks confirmation before descendants (`worker.go:932-935`), every Windows survivor or enumeration failure ends `interrupted`/`stop_unconfirmed`; `unresolved_descendants` is unreachable on Windows (honesty register).
- Bounded stop wait (Windows only, D12): after the final rung's grace the worker stops waiting for the native to exit, using the bounded `Done` wait `abort` already uses (`worker.go:603-611`), and concludes with `Confirmed: false`; the Unix supervise loop (`worker.go:633`) is untouched (N4). Without it a native that survives `TerminateJobObject` would hang the worker (`waitDelay` bounds only pipe draining after exit).
- Ladder: the Windows rung list `[StopKill 5s]` keeps its shape, but now reaches the tree, so it ships as `stop-ladder/v2-windows` (D3). `stopLadderVersion` becomes `func stopLadderVersion(goos string) string` in `internal/supervisor/pipeline.go`; Unix stays `stop-ladder/v1`.
- `descendantScan` is `"job-object"` on Windows. `unresolvedDescendants` calls a per-OS function `descendants(p *ownedProc, attemptID string) ([]int, []ProcessIdentity, error)`; with no process or job (a launch failure, where `conclude` still runs the scan, `worker.go:517-521`) it returns empty lists and no error, so `launch_failed` is not turned into a scan failure; on Windows it lists every live job member including the native; on Unix and Linux it keeps the marker scan. An error returns `failed` (the existing fail-closed path). Windows identities feed `unresolved_identities` through the existing `ProcessIdentity` (no duplicate type).
- `job.close()` runs only after `attempt.stopped` is emitted, so kill-on-close cannot hide survivors from the scan.

#### 2.4 Launch identity (AC-1.4, AC-3.2)

`Identity` (`worker.go:111-127`) gains `NativeStartTime *time.Time` (`json:"native_start_time,omitempty"`), set on Windows from `ProcessStartTime(pid)`; the `attempt.launched` payload (`worker.go:527-530`) carries the same field on Windows. Unix omits it (OQ-8 default, D4). No schema change (D8): the supervisor's `attempt.launched` handler decodes an anonymous struct (`internal/supervisor/ingest.go:176-188`), which ignores the extra field, so the value stays in the journaled event payload and in worker.json; recovery reads worker.json. The journal's `attempts` columns are unchanged, and a reader of the event payload treats an absent field as `unknown`.

#### 2.5 Recovery (FR-3, FR-4)

OQ-1 default (a): the job is worker-owned with `KILL_ON_JOB_CLOSE`, so a worker crash ends the tree. Recovery on Windows reads worker.json, checks the native by PID and `native_start_time` (`ProcessStartTime` returns `ErrNoProcess` when gone), and, under OQ-11's default, still ends the attempt `ownership_unresolved` instead of treating kill-on-close as proof (I24, I09). Descendants are not recorded, so none are matched. `workers.OrphanPIDs` stays `ErrUnsupported` on Windows; the supervisor prints the identities and keeps ownership unresolved as it does off Linux. The launch-boundary outcomes (AC-6.3): job created before spawn → nothing to find; suspended spawn before the identity is recorded → `ownership_unresolved` (the residual window: an unnamed inert suspended native); identity recorded, not yet assigned → recovery names the suspended native by PID and start time and keeps ownership unresolved (it is never signalled, I24); assigned → same launch identified. The detach contract (AC-3.1): the worker keeps `DETACHED_PROCESS`; the CLI's first interrupt requests a stop, the second detaches (existing `pipeline.go:68-69,1000-1011`).

#### 2.6 Capability and support matrix (FR-5)

`internal/processsupport/SUPPORT.md` is a table `| arch | surface | process_tree_ownership | worker_detachment | crash behaviour | tests | CI job |`. `internal/processsupport/support.go` embeds it:

```go
type Support struct{ ProcessTreeOwnership, WorkerDetachment adapter.Tri }

// Windows returns Supported for a field only when a row names it for
// (goarch, surface); every other pair is Unknown.
func Windows(goarch, surface string) Support // uses the embedded table

type Matrix struct{ /* parsed rows */ }
func Parse(table []byte) (Matrix, error)      // error on a malformed table
func (Matrix) Windows(goarch, surface string) Support
```

The fake adapter holds a `Matrix` field (default: the embedded one) so tests can inject rows without package-level state.

`TestSupportMatrixMatchesEvidence` parses the `_windows_test.go` files with `go/parser` (as `internal/control/support_test.go:108` does, because `go test -list` cannot see Windows-only tests on other runners) and fails when a row names a test that does not exist, or `Windows` returns `Supported` without a row; a malformed embedded table makes `Windows` return `Unknown` for every pair (pinned by a test); mutation cases (as `support_test.go:194-215`) show it can fail. A row's CI-job cell must name the Windows job (`windows-latest` or `windows-11-arm`) and a run id. With no rows the function returns `Unknown` everywhere: that is the state this spec ships until Task 11 adds rows. The fake adapter's Windows branch calls `Windows(arch, "scripted-child-process")`; `adapters/fake/fake.go:165` builds the Prepare-time probe without `Arch`, so Task 9 fills `Arch` there (`adapter.Probe.Arch`, `adapter.go:58`). `claudecode` keeps `Unsupported` (AC-5.3). `Platform` gains no field here (D5).

#### 2.7 Test fixture and tests (FR-6)

The fake agent's only child-spawning step, `spawn_escapee`, is Unix-only (`adapters/fake/escapee_other.go:1,10-12`). Task 2 adds a `spawn_tree` step: it starts a child that starts a grandchild (the child with `CREATE_NEW_PROCESS_GROUP`, the grandchild inheriting it) and then attempts one further child with `CREATE_BREAKAWAY_FROM_JOB`, lingers, and writes JSON lines to the workdir-relative file named by the step's `report_path` argument (validated like `write`, `adapters/fake/agent.go:116-118`; the step is keyed `"op"`, `agent.go:48-49`). A line is `{"role":"child|grandchild","pid":N,"start_time":"…"}` for a started process and `{"role":"breakaway","error":N}` carrying the Win32 error of the breakaway attempt (expected 5, `ERROR_ACCESS_DENIED`); a successful breakaway also reports its PID so the test can fail on it. The step uses `os.StartProcess`, since adapters may not import `os/exec` (`adapters/fake/fake_test.go:667`); `tree_windows.go` also defines `spawnEscapee` as an error (Unix keeps its own), because `agent.go:217` calls it unconditionally. The Unix side keeps `spawn_escapee`; `escapee_other.go` is retagged `!unix && !windows`. Windows-only tests live in `job_windows_test.go`, `worker_windows_test.go`, `recover_windows_test.go`, and are written first and shown red on current main (the grandchild survives, `descendant_scan` is `unavailable`).

### 3. Decisions

| ID | Decision | Rationale |
|---|---|---|
| D1 | Kill-on-close, worker-owned job (OQ-1 option a) as the default | Smallest slice that satisfies "stop ends the whole tree"; a holder process (b) or named job (c) adds a second owner (I18) or a same-user-openable object. Lost: Unix-equal survive-and-reconnect on a worker crash; recorded in the honesty register. |
| D2 | `CREATE_SUSPENDED` + assign + resume | `syscall.StartProcess` has no job parameter and closes the main thread handle (OQ-3); a direct `CreateProcess` job-list attribute would leave `exec.CommandContext` (`go-conventions.md`). Thread enumeration keeps `exec.Cmd`. |
| D3 | New Windows-only ladder version | The kill rung changes reach (process → tree); a receipt must not claim `stop-ladder/v1` semantics. Unix untouched (N4). |
| D4 | Windows-only `native_start_time` | OQ-8 default; a common field changes Unix persistence (N4). |
| D5 | No `Platform` struct change here | Seam change needs an `architect` review (AC-5.1); the matrix function keeps this spec shippable without it. |
| D6 | Kill-only Windows ladder | OQ-2 default; no graceful rung without a passing Windows test (AC-2.5). |
| D7 | Lazy-loaded kernel32 bindings for calls `x/sys` lacks | A new dependency or cgo is worse than three `NewLazySystemDLL` procs; the missing set was checked against v0.48.0. |
| D8 | `native_start_time` stays in worker.json and the event payload, no migration | A column adds a schema change on every OS (N4) and a journal reader API for a value only Windows recovery reads. Alternative rejected: migration 0008. |
| D10 | Launch order: intent write, then job, then spawn | A crash after the intent but before the job leaves a recoverable record (nothing to find); the reverse would leave a job with no intent record. |
| D11 | Parent-job policy: check at native launch and refuse; the worker does not break away at spawn | Alternative: `CREATE_BREAKAWAY_FROM_JOB` in `detachedAttr` so the worker leaves a terminal's kill-on-close job. Rejected for the first slice because breakaway is itself refused in many runner jobs; the Task 3 spike on the Windows runners decides, and `architect` reviews it (OQ-3). If runners turn out to sit in a kill-on-close job, Task 3 records `blocked` rows rather than refusing every launch silently. |
| D12 | Windows-only bounded stop wait | Without a deadline a surviving native hangs the worker; making the wait common changes the Unix loop (N4). |
| D9 | Matrix in a leaf package `internal/processsupport` | `internal/workers` imports both adapters, so a matrix there would create an import cycle; the leaf serves both adapters from one source (AC-5.2). |

An ADR (`docs/decisions/0017-windows-process-ownership.md`, 0016 is taken by `0016-migration-import.md` on main) records D1–D4, D8, D11 and OQ-11, and supersedes the Windows statements of ADR 0004 (Task 1).

### 4. Honesty register

| Spec demand | Position |
|---|---|
| v2 §6.4 Windows Job Objects "and appropriate console semantics" | Partially met: the job and the `DETACHED_PROCESS` launch ship; graceful console rungs are deferred (OQ-2); detach evidence depends on what hosted runners can drive. |
| v2 §6.4 worker survival and reconnect after a crash | Windows differs from Unix by D1 until OQ-1 is decided; ownership is quarantined, not reconnected. |
| v2 §17.1 Windows ARM64 and the combination matrix | Only the rows with a passing run are `supported`; ARM64 runs without `-race` (N6). Terminal, shell and filesystem rows are not covered (N5, OQ-10). |
| I14 for the `claudecode` surface on Windows | Not met by design: the adapter keeps refusing (AC-5.3, OQ-6). |
| `unresolved_descendants` reason on Windows | Unreachable by construction (survivors make the stop unconfirmed); the Unix reason is unchanged. |
| AT-06 on Windows | Partially covered: the launch-boundary crash tests cover the boundaries the hooks reach; supervisor-restart recovery with a live native has no Windows test because kill-on-close ends the native (D1). |
| v2 §6.4 "PID and start identity and nonce" | Partially met: Windows matches the native on PID and creation time only; descendants carry no nonce or marker (AC-4.4). |
| v2 §6.3 step 4 start-time evidence | Met on Windows only; Unix keeps PID and PGID (D4, N4, G11). |
| AC-5.2 one source for both adapters | Partially met: `claudecode` stays hard-coded `unsupported` while its refusal stands; the matrix becomes its source when the refusal is lifted (OQ-6). |
| AT-07, AT-09, AT-41, AT-42 on Windows | Not addressed here; G04's other cases keep their existing Unix evidence. |
| v2 §6.4 background native agents, launched containers and ports on Windows | Not met: only processes in the job are owned; containers and held ports are not detected (as on Unix, they are recorded as unresolved at best). |
| Service-path recovery on Windows (`internal/control/recover.go` `Reconcile`) | Not changed here; it keeps its existing PID-width and liveness behaviour. A follow-up decides whether it reads `native_start_time`. |
| O3's visible outcome (a `supported` Windows row) | Deferred to Task 11: rows are added by a maintainer from real CI runs, so this spec ships `unknown` everywhere until then. If hosted runners cannot drive console events (AC-6.4) `worker_detachment` stays `unknown` and the detach wording in docs says so. |
