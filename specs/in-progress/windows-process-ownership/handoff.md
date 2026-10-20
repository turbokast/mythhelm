# windows-process-ownership — Hand-off

> One section per task. Each task replaces its own `<!-- pending -->` line in its pull request with what
> dependent tasks need: what it produced (the API and files as shipped), what a later task must
> know, and any deviation that changes a later task's inputs. `/run-spec` puts the sections of a
> task's dependencies into that task's dispatch prompt. Open questions and research stay in
> `scratchpad.md`.

## Task 1 — decision record: Windows process ownership

- **Produces**: `docs/decisions/0017-windows-process-ownership.md`, recording the maintainer-approved OQ-1/OQ-8/OQ-11 defaults and D2/D3/D8/D11. It supersedes ADR 0004's named Windows design statements; Task 10 updates the old ADR's status.
- **For dependents**: worker-owned kill-on-close job; Windows-only native creation time in worker.json and event payload; no database migration; worker death does not itself resolve ownership. Preserve unresolved crash-boundary outcomes and the separate Windows stop-ladder version.
- **For Task 3**: real Windows runner measurements and OQ-3/OQ-13 architect review remain required. A kill-on-close parent job on either runner requires maintainer escalation before production changes.
- **Qualification**: no Windows support row is promoted by this ADR. Task 11 remains a maintainer decision supported by real Windows CI evidence.
- **Deviations**: none to the approved design or dependent acceptance criteria.

## Task 2 — fake adapter child-tree fixture for Windows

- **Produces**: `spawn_tree` with validated workdir-relative `report_path`; embedded scenario `tree` writes `tree.jsonl` and holds the native for ten minutes. Off Windows the scenario loads and returns a runtime `spawn_tree needs Windows` error wrapping `errors.ErrUnsupported`.
- **Topology**: native → child (`CREATE_NEW_PROCESS_GROUP`) → grandchild (inherited group). The native also attempts a `CREATE_BREAKAWAY_FROM_JOB` child. Descendants have null stdin/stdout/stderr and linger for ten minutes unless terminated.
- **Reports**: JSONL records carry `role`, `pid`, RFC 3339 UTC `start_time`; the `breakaway` record always carries numeric `error`, and includes identity only on success (`error: 0`). Report order is asynchronous: wait for child, grandchild and breakaway roles before acting.
- **For Tasks 3–7**: the fixture does not establish job ownership or require breakaway denial itself. Tests using a no-breakaway job must assert error 5 and no breakaway identity; always clean up any successful breakaway by its reported identity. Confirm liveness with exit code plus creation time, not creation time alone.
- **Report lifecycle**: the native truncates the report once before spawning; child/grandchild writers only append through `os.Root` confinement. Reusing a report path does not retain identities from a prior invocation.
- **Failure cleanup**: setup errors roll back retained process handles and descendants whose parent/creation-time interval matches. Child-side report failure cleans its own newly started grandchild before returning. Teardown does not rely on readable JSONL; successful fixture execution installs no job and descendants survive native exit.
- **Evidence**: source `dffab2f`, run [38730062877](https://github.com/turbokast/mythhelm/actions/runs/38730062877), Windows AMD64 job [116405401494](https://github.com/turbokast/mythhelm/actions/runs/38730062877/job/116405401494) passed the fake package with race detection (4.539s), and ARM64 job [116405401467](https://github.com/turbokast/mythhelm/actions/runs/38730062877/job/116405401467) passed (1.096s). Descendant/setup-failure tests have no runtime skip. Symlink cases alone may skip for missing Windows symlink privilege; non-verbose logs do not disclose their outcome. This is fixture execution evidence, not platform qualification (I14, v2 §17.1).
- **Deviation**: Windows acceptance lives in the added `adapters/fake/tree_windows_test.go`; no dependent API contract changes.

## Task 3 — Job Object launch ownership

- **Produces**: worker-owned `jobObject` with non-inheritable kill-on-close handle, `assign`, `terminate`, bounded `members` and `close`; Windows suspended launch persists identity before assignment and resumes only after assignment. `job_other.go` preserves Unix launch and persistence behaviour.
- **Identity seam**: `Identity.NativeStartTime` is now present and populated on Windows before assignment. Task 5 still owns `attempt.launched.native_start_time`, equality evidence and Unix omission checks.
- **Failure seam**: `jobOps` is per worker; nil fields use real Windows APIs. Six boundary hooks remain in design order: `afterIntent`, `afterJob`, `afterSpawn`, `afterStart`, `beforeResume`, `afterResume`. Task 6 can use them to exercise its crash-boundary acceptance.
- **Cleanup evidence**: failed launch terminates both the job and the retained native, waits on a pinned native handle and queries members before confirming. Failure to obtain that wait handle still attempts both terminations but remains unconfirmed; the regression exercises that distinction. `launch_error` carries redacted Win32 text/code on `attempt.stopped` only on Windows.
- **For Task 4**: normal `Signal`, `GroupGone` and descendant reporting retain the old semantics. Task 4's `TestStopKillsChildAndGrandchild` acceptance requires descendants gone, `attempt.stopped.confirmed` true and `descendant_scan` exactly `job-object`; that wiring is not claimed by Task 3. The job closes after conclusion, so it must remain open during Task 4's survivor scan.
- **Parent-job policy**: real checks refuse kill-on-close parents or query errors before creating a job/native. Both hosted Windows runners passed the prerequisite with parent membership true and zero limit flags; no stub replaces their parent-job state.
- **Native evidence**: source `5d05701`, [CI run 39108953755](https://github.com/turbokast/mythhelm/actions/runs/39108953755), AMD64 job `117621040510` (full race suite) and ARM64 job `117621040503` (full non-race suite): all 18 named launch/identity tests pass.
- **Evidence and scope**: normal PR CI exposes the named launch acceptance in `Verify Windows launch ownership` on AMD64 and ARM64, followed by each full platform suite. Task 11 still owns support promotion; launch evidence alone does not qualify stop, recovery, detach or a native harness.
- **Identity failure contract**: `TestIdentityFailureStopsNative` keeps its original Unix persistence-abort assertions; Windows now requires the earlier, confirmed launch-failure path with no ladder and no launched event. The Windows-only companion measures real suspension and retained-handle exit.
- **Deviations**: the approved CI/automation-docs extension exposes successful test output. The shared identity-failure test required a bounded `worker_test.go` deviation to migrate the Windows contract. Task 3 introduced the creation-time field needed for safe ordering; Task 5 retains the remaining event contract. Fresh review added the wait-handle injection and cleanup regression without changing the approved ownership policy.


## Task 4 — job-wide stop, confirmation and descendant scan

- **Produces**: Windows `Signal(StopKill)` terminates the owned job; `GroupGone` requires native Wait and an error-free empty job query. Per-OS `descendants(p, attemptID)` returns PID/creation-time identities; Windows uses `job-object`, Unix retains marker scanning. `stopLadderVersion(goos)` selects `stop-ladder/v2-windows` only on Windows.
- **Receipt ordering**: retain the job through the final scan and stopped-receipt persistence; close only afterward. Later survivor/error evidence invalidates earlier confirmation. Keep identities from the same job snapshot; never reacquire by PID to reconstruct them. No-process/no-job launch failure still scans empty so confirmed cleanup preserves `launch_failed`.
- **Bounded stop**: Windows concludes after the one pinned ladder plus existing `waitDelay` when the native remains live. No second ladder; unresolved ownership becomes `stop_unconfirmed`. Unix supervision remains unchanged.
- **For Tasks 5–7**: job operations remain per-worker injected fields, retained on ownedProc. Preserve Task 3 hook ordering and retained native cleanup before assignment: pre-session panic uses bounded failedLaunch evidence, terminating job and retained native, with timeout unconfirmed. Task 5 still owns launched-event creation time; recovery and detach are not qualified here.
- **Observation semantics**: after natural native exit, an initial empty job query sends no signal; a nonempty or failed first snapshot may cause kill before a later empty query confirms. Signals record actual attempts. Do not normalize these differences away; compare displayed receipts to their own durable event.
- **Integration contracts**: new Windows demo admissions expect v2-windows through one exact expected-line overlay; captured bundle and output normalization remain unchanged. JSONL tests preserve complete stopped-envelope evidence against the journal and strict cross-run comparisons for other envelopes.
- **For Tasks 8–11**: Task 4 is stop evidence only, not a support-matrix promotion. Keep Windows kill-only labeling, separate ladder version, failed scan semantics and maintainer qualification responsibility.
- **Evidence**: implementation `27ccedf`, baseline correction `7b1dfa8`, durable-envelope correction `1085e72`, integrated head `1f21d09`; CI `39148306697`, both named acceptance sets pass, ARM64 full job `117736819760` passed; AMD64 full race job `117736819762` passed (workers30.352s, E2E91.325s); ARM64 workers30.342s/E2E120.152s; whole CI green. Documentation-only main merged as `c3f3f5d` afterward. Local full race85.3s and six fresh markers; independent final E2E race2.861s. Earlier integration failure causes/corrections are in scratchpad; the exact transient job-query condition behind the earlier ARM64 signal difference was not logged.
- **Approved deviations**: CI selector/automation evidence, per-OS job helper cleanup reporting, precise CLI expected overlay and durable E2E receipt oracle; original worker/supervisor production scope otherwise retained. Independent code/architecture and final test-oracle reviews Ready. Task report must retain `first_pass:false` because initial lint failed on obsolete suppressions.

## Task 5 — native start time in the launch record

- **Produces**: Windows `attempt.launched.native_start_time` equals the native creation time in worker.json and `ProcessStartTime(native_pid)`. `Identity.NativeStartTime` and pre-assignment read/cleanup were supplied by Task 3; no new exported API or database column is introduced here.
- **For Task 6**: match PID and persisted creation time, preserving missing/unreadable identity as unresolved. Do not infer recovery ownership from this event or successful cleanup tests; ADR 0017 and Task 6 boundary acceptance still govern recovery.
- **Unix compatibility**: fixed Identity golden and exact launched-event payload retain omission of creation time; never emit null or zero as observed evidence. Ingestion preserves the extra field in the journal without changing attempt projections.
- **Test seams**: existing per-worker `startTime`, termination and membership hooks retain pre-assignment failure coverage, including unconfirmed termination; equality is observed while the native is live. The E2E oracle binds raw stream/journal timestamps to independent worker.json identity before event-specific normalization.
- **Evidence**: commits `e052905`, `067c453`, integrated source `b665237`; CI [39416955906](https://github.com/turbokast/mythhelm/actions/runs/39416955906) passes both named tests and full Windows suites: AMD64 job `118608248903` (race workers45.730s/E2E66.170s), ARM64 job `118608249088` (workers37.205s/E2E71.691s). Local full race238.9s, Unix record/event race1.372s, ingestion race1.396s; source reviews Ready.
- **Deviations and limits**: approved CI/docs acceptance exposure and E2E per-run creation-time binding; unchanged Unix event regression is consistent with N4. Shared stop fixture fixes arrived independently through PR505/main. No Windows qualification matrix is promoted. Retain `first_pass:false` for earlier local environmental gate failures and corrected native integration failures. Final metadata-head CI and merge remain the coordinator's responsibility.

## Task 6 — Windows recovery verdict

- **Produces**: Windows recovery reports only a live native matching both recorded PID and `native_start_time`. All worker-crash boundaries retain `interrupted_and_quarantined` and `ErrOwnership`; no recovery path starts another native or signals the observed one.
- **For Task 7**: `workers.OrphanPIDs` still returns `ErrUnsupported` on Windows. Descendant ownership is unobserved after worker death; kill-on-close is not reconciliation evidence under ADR 0017. Missing, mismatched, unreadable and exited native observations do not resolve ownership.
- **Process probe**: `ProcessStartTime` checks a pinned process handle for exit before reading creation time. An exited object retained by another handle returns `ErrNoProcess`; denied/query/wait failures remain errors. The observation may become stale immediately and grants no signalling or ownership authority.
- **Crash boundaries**: intent-only records remain indistinguishable before identity persistence. A suspended native outside the job can remain live before assignment, named only when its creation time is recorded. Assigned and resumed natives die with the worker; tests independently observe actual child/grandchild identities before the resumed crash.
- **Compatibility**: Unix/macOS marker lookup, notice text and existing error handling are retained. Linux tests use a real marked child and retain the older quarantine, worker-PID reuse and no-relaunch cases.
- **Test seams**: the workers test binary's `__crash-worker` mode selects the existing six `jobOps` hooks and exits without deferred worker cleanup. Side-channel spawn identities are test evidence only; production persists no new launch phase or job identifier. Suspended residual cleanup pins and verifies the exact process before terminating it.
- **Scope**: the approved CI/docs deviation exposes six named Windows tests and the supervisor package in the existing verbose step. Full suites and scheduling stay intact. PR508's CLI journal-read fixture correction is an independent prerequisite, integrated without taking ownership of its two files.
- **Evidence limits**: local Wine crash-boundary and creation-time checks are development evidence. Hosted Windows AMD64/ARM64 CI remains required; no process-support matrix row or native-harness qualification is promoted by this task.
- **Red-first and lint**: delayed identity persistence, PID-only matching and removal of kill-on-close each fail retained assertions; restored focused checks pass. The initial lint rejection was corrected mechanically; retain `first_pass:false` in the task report.
- **Native review round 1**: source CI `39430928707` exposed two test-oracle errors on both architectures: checking native death immediately after worker exit, and treating projection of an already durable spooled launch as a new launch. Corrected tests retain the existing five-second exact-handle confirmation bound, compare full pre/post journal/spool launch identities except assigned `RunSequence`, and keep spool/intent/attempt-count/worker-identity assertions. Missing kill-on-close still fails the resumed-tree row before cleanup; a fresh valid launch event still fails the launch-evidence assertion. Production recovery is unchanged.


## Task 7 — detach and interrupt behaviour on Windows

<!-- pending -->

## Task 8 — Windows process-support matrix (leaf package)

- **Produces**: `processsupport.Support`, `Matrix`, `Parse([]byte) (Matrix, error)`, `Matrix.Windows(goarch, surface)` and package `Windows(goarch, surface)`, exactly as design §2.6. `SUPPORT.md` is embedded and has no rows; the control-method matrix now points here for lifecycle evidence.
- **For Task 9**: package `Windows` reads the embedded table; injected matrices come from `Parse`. A zero matrix and absent exact pairs return explicit `adapter.Unknown` for both fields. Lookup never infers architecture or surface. The leaf imports only the adapter contract among project packages, avoiding adapter/worker cycles.
- **Table contract for Tasks 10–11**: seven columns; amd64 pairs require `windows-latest`, arm64 pairs require `windows-11-arm`. The CI cell is the job name followed by a repository Actions run URL with a positive numeric run ID. Capability values stay separate; crash behaviour is `kill_on_close` or `survives`; evidence is a comma-separated list of backticked test names. Duplicate pairs, malformed syntax or unbound CI evidence reject the whole table, including previously valid rows.
- **Qualification boundary**: parser validation checks syntax and CI binding, not passing run outcomes. The AST exactness oracle discovers actual Windows test declarations in workers, supervisor, fake and E2E sources. Task 11 must review FR-1–FR-4 coverage, the matching native runner and landed commit before adding rows, and update the deliberately empty-matrix expectation. Synthetic parser fixtures do not promote support.
- **Evidence and review**: source `418c4a3`, PR #500; seven focused race tests passed (1.263s), full race gate passed and all six gate markers were fresh. Coordinator code/architecture review and independent acceptance rerun found no remaining blocker. Root expansion is a review clarification consistent with design §2.6; no API or task-file scope changed.

## Task 9 — adapter capability derivation

- **Produces**: fake Windows capabilities now use `processsupport.Windows(Arch, "scripted-child-process")`; the Prepare-time probe supplies the runtime architecture. No exported API changes. Source `a6c8572`, PR #503.
- **For Tasks 10–11**: the embedded table is still empty, so both fake Windows capability fields are unknown on every architecture. Exact architecture and surface evidence is mandatory; neither field inherits the other. Synthetic test matrices are not qualification rows.
- **Native harness boundary**: Claude Code still reports both Windows platform capabilities unsupported and refuses probe and Prepare with `ErrCapability`, naming missing Windows native-harness qualification. Worker Job Object ownership does not qualify a native harness; the compatibility note says so.
- **Tests and review**: all three named race acceptance tests passed, seven overlay counterfactuals failed, the full race gate passed (213.1s), and independent code/architecture review returned Ready. The per-instance optional matrix and unexported platform seams keep tests free of mutable package state and native credentials. No change to later task inputs.

## Task 10 — Windows documentation

<!-- pending -->

## Task 11 — promote tested Windows rows

<!-- pending -->
