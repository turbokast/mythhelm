## Windows Process-Tree Ownership — Requirements

> Gives a MYTHHELM worker an owned process tree on Windows (a Job Object), so detach, stop, crash and orphan handling are tested there as they are on Linux and macOS, and only then lets the Windows capability record say `supported`. A slice of Stage 1 platform coverage (work stream W14, with W03 lifecycle). Normative source: mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md, version 2.0; § numbers, I-IDs, G-IDs and AT-IDs refer to it unless a reference is marked Rev 1.1.

## Context

- **Backlog card**: MH-11
- **Issue**: [#32](https://github.com/turbokast/mythhelm/issues/32), "Windows process-tree ownership (MH-11)", a card-tracking issue with no further discussion.

The card asks for Job Objects and process-tree ownership for workers on Windows, so that detach, stop, crash and orphan handling "pass there as they do on Unix". The dogfood slice kept the seam and blocked the native adapter on Windows (its non-goal N7, `specs/*/dogfood-slice/requirements.md:23`).

Today a Windows worker owns exactly one process. It starts the native in a new process group (`CREATE_NEW_PROCESS_GROUP`), has no group handle, can only `Process.Kill` it, cannot enumerate descendants and has no orphan scanner. Admission therefore refuses the `claudecode` adapter on Windows with exit 7 (`adapters/claudecode/launch.go:70-72`; exit 7 is `ExitCapability`, `internal/cli/exit.go:28`), and the fake adapter reports `process_tree_ownership: unsupported`. Windows CI (`windows-latest`, `windows-11-arm`) exercises the fake adapter and unit tests only; those do not establish process-lifecycle support (v2 §17.1: a cross-compiled binary is not process-lifecycle qualification).

This spec closes the gap in the worker and its stop, recovery and orphan paths, and defines the evidence that lets the capability be advertised. It does not qualify any native harness on Windows, and it does not contain hostile code (a Job Object manages lifecycle, v2 §6.4).

### Grounding (`.claude/rules/spec-premise-grounding.md`)

Verified against origin/main at 7bf96c4.

| # | Claim | Verdict | Evidence |
|---|---|---|---|
| G1 | A Windows worker has no process-tree ownership; the native runs alone in a process group. | HOLDS | `internal/workers/proc_windows.go:27` (`nativeAttr`), `:32` (`nativeGroup` returns nil) |
| G2 | The Windows stop ladder has one rung. | HOLDS | `proc_windows.go:36` (`Signal` rejects everything except `StopKill`); `internal/workers/worker_windows_test.go:13` pins the one-rung ladder |
| G3 | Stop confirmation on Windows only means the native was waited for. | HOLDS | `proc_windows.go:47` (`GroupGone` = `waited`) against the Unix `kill(-pgid, 0)` confirmation in `proc_unix.go` |
| G4 | Descendants cannot be found, so the worker "claims nothing about them". | HOLDS | `proc_windows.go:17` (`descendantScan = "unavailable"`); `worker.go:905-908` returns early |
| G5 | Orphan recovery has no scanner on Windows. | HOLDS | `internal/workers/orphan_other.go:9`, `proc_windows.go:66` return `errors.ErrUnsupported`; the Linux scanner is `orphan_linux.go:6`; consumers are `internal/supervisor/recover.go:189,242,295` |
| G6 | The `claudecode` adapter is blocked on Windows. | HOLDS | `adapters/claudecode/launch.go:70-72` and `probe.go:55-57` refuse with `ErrCapability`; `probe.go:131-133` records `unsupported` |
| G7 | Worker detachment on Windows is undecided: the fake adapter says `unknown`, the claudecode adapter says `unsupported`. | PARTIAL | `adapters/fake/fake.go:82-88`, `adapters/claudecode/probe.go:126-133`. The worker does set `DETACHED_PROCESS` (`proc_windows.go:21`), but no test establishes detach, so neither value is evidence |
| G8 | The issue's section references (§7.4, §18.4, §20.2, §22.2 and §20.3; the card itself cites v2 §§6, 17) name the intended requirements. | PARTIAL | The issue body uses Rev 1.1 numbering (`docs/spec/master-spec.md:566` §7.4 process lifecycle, `:1899` §18.4 fault injection, `:2095` §20.2 Stage 0, `:2199` §22.2 backlog). In v2, §7.4 is "Billing posture versus trust". The v2 homes are §6.4 (stop and recover), §17.1 (honest compatibility), §18.3 (G04) and AT-06–AT-09, AT-40–AT-42. This spec cites v2 |
| G9 | The failure is reachable and an acceptance check can fail today. | HOLDS | Windows is in the CI matrix (`.github/workflows/ci.yml:60`); a test that starts a native with a child and expects `confirmed` stop with no survivor fails today because the child is untracked and `Kill` does not reach it |
| G10 | No Windows process-tree capability is claimed as supported anywhere in the product. | PARTIAL | Producers of `Platform` values are only `adapters/fake/fake.go:82-88` and `adapters/claudecode/probe.go:126-133` (both `unsupported`/`unknown`). Other Windows process code: `internal/integration/process_windows.go` (a `taskkill /T` kill for verification checks, not worker ownership) and `internal/control/recover.go:565-579` (PID width). Drift: `docs/limitations.md:13-15` says supervision and recovery work on Windows, and `internal/control/SUPPORT.md:37-40` says there is no Windows detach evidence. AC-5.4 reconciles them |
| G11 | The native's start time is not recorded with its PID today (the absence AC-1.4 relies on). | HOLDS | `worker.json` and `attempt.launched` carry `native_pid` only (`internal/workers/worker.go:111-127,527-528`; `internal/supervisor/ingest.go:176-182`). The worker's own start time is recorded; the native's is not, on any OS. AC-1.4 therefore adds a field (OQ-8) |

Optional vendor second reading (`premise-ground`): not run; auto-confirmed (non-interactive). No vendor consult requested.

## Objectives

- **O1**: On Windows a worker places the native and every descendant it starts in one Job Object, so a stop ends the whole tree or reports exactly what survived.
- **O2**: After a supervisor restart, a lost terminal or a worker crash on Windows, recovery either finds the attempt's surviving processes and reconciles them, or knows the job ended them (which of the two is OQ-1), and releases ownership only on that evidence.
- **O3**: The Windows capability record (`Platform.ProcessTreeOwnership` in `internal/adapter/adapter.go:395-399`) changes from `unsupported`/`unknown` to `supported` only for the exact (architecture, `CapabilityRecord.ExecutionSurface`) combinations whose lifecycle tests pass on a Windows runner of that architecture, evidenced by a support-matrix row (AC-5.1).

## Non-Goals (and what still binds)

| # | Out of scope | Still binding in this spec |
|---|---|---|
| N1 | Qualifying Claude Code or any other native harness on Windows (billing, auth, instruction handling, ConPTY behaviour of a specific CLI), including a `supported` row for the claudecode surface. | I14, I02: this spec supplies only rows for the scripted fake surface (`scripted-child-process`, `adapters/fake/fake.go:95`); the claudecode surface (`native-cli-structured (print, stream-json)`) stays non-`supported` and the adapter stays blocked on Windows until its own qualification (OQ-6). |
| N2 | Hostile-code containment, AppContainer or any sandbox. | I14, AT-47, v2 §6.4: no UI, receipt or capability text calls a Job Object a security boundary. Contained profiles remain `specs/*/contained-execution-profiles`. |
| N3 | WSL. | I14: WSL is qualified separately (v2 §17.1); a WSL run proves nothing for native Windows. |
| N4 | macOS and Linux behaviour changes. | I06, I18: Unix ownership is unchanged and its tests keep passing. |
| N5 | Terminal, shell and filesystem matrix rows (Windows Terminal, PowerShell/cmd, long paths, junctions, case behaviour). | I14, AT-40: each row is advertised only with its own tests; this spec covers the process rows. |
| N6 | Windows ARM64 beyond what a runner can show. | I14: an architecture without a passing runner stays `unknown`, never inherits the AMD64 result. |

## Functional Requirements

Acceptance criteria use EARS. Tags in brackets name the invariants, gates and sections each one serves.

### FR-1 — Owned tree at launch (v2 §6.3, §6.4; I06, I18)

- **AC-1.1** [I18, §6.4] When a Windows worker launches a native, the system shall create a Job Object and assign the native to it before the native can start any child, so that no descendant exists outside the job.
- **AC-1.2** [I18] The system shall not make the job handle inheritable, so that no native or descendant holds an inherited handle to the job; a test shall assert the handle is created non-inheritable. (The job is lifecycle management, not a security boundary, N2.)
- **AC-1.3** [I02, I06, §6.4] If the Job Object cannot be created, then the system shall not spawn the native and shall end the attempt `failed_native` with reason `launch_failed`, carrying the Windows error code in the `launch_error` field of `attempt.stopped` (today a launch failure is only logged, `internal/workers/worker.go:519`). If the native was spawned suspended but cannot be assigned to the job, then the system shall terminate it, confirm it gone and end the same way; if that cannot be confirmed, the attempt ends `interrupted` with the existing reason `stop_unconfirmed` (`worker.go:932-933`; exit 6, `internal/cli/exit.go:27`).
- **AC-1.4** [§6.3, §6.4] The Windows launch record (`worker.json` and `attempt.launched`) shall carry `native_start_time` (the native's creation time, next to `worker_start_time`, `internal/workers/worker.go:527`) alongside `native_pid`. Whether the field is Windows-only or common to every OS is OQ-8; N4 forbids changing Unix behaviour unless OQ-8 decides otherwise.
- **AC-1.5** [I18, §6.4] The system shall configure the job with neither `JOB_OBJECT_LIMIT_BREAKAWAY_OK` nor `JOB_OBJECT_LIMIT_SILENT_BREAKAWAY_OK`, so a descendant cannot leave it.
- **AC-1.6** [§6.4, I06] If the worker is inside a job with `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE` that it has not left, or `AssignProcessToJobObject` fails for the native, then the system shall refuse to run the native and report `launch_failed` with the cause (AC-1.3). Whether the check runs at worker spawn or at native launch, and the breakaway policy that keeps AC-3.1 true, are OQ-3.

### FR-2 — Stop ladder and confirmation (v2 §6.4; I06; AT-08)

- **AC-2.1** [AT-08, §6.4] When a stop is requested, the system shall run the adapter's Windows ladder in order and shall make terminating the Job Object (`TerminateJobObject`) the final rung. Replacing `Process.Kill` with job termination changes what the `StopKill` rung reaches, so it counts as a ladder change: it ships under a new Windows-only ladder version string named in design (for example `stop-ladder/v2-windows`), Unix stays on `stop-ladder/v1` (the constant `stopLadderVersion`, `internal/supervisor/pipeline.go:804`, becomes per-OS; N4), the receipt records the version and the rungs sent, and the change is coordinated with the owner of `specs/*/supervised-stop-recover` OQ-SR2.
- **AC-2.2** [I06] The system shall report a stop `confirmed` only when the job reports zero active processes, not when the first process exits.
- **AC-2.3** [AT-08] If processes remain after the deadline, then the system shall report every surviving job member, the native included, in `attempt.stopped` as `unresolved_pids` with each one's creation time in `unresolved_identities`, end the attempt `interrupted`/`stop_unconfirmed` (`internal/workers/worker.go:927-935`), quarantine it and not signal any process outside the job.
- **AC-2.4** [AT-08] The system shall classify a native exit during an active stop as `stopped`, as it does on Unix.
- **AC-2.5** [§6.4, I14] The Windows ladder shall contain only `StopKill` (job termination) unless a graceful rung has a passing Windows test; it shall never label a kill as graceful (default of OQ-2).

### FR-3 — Detach and abrupt loss (v2 §6.4, §17.1; AT-06, AT-40)

- **AC-3.1** [AT-40, I06] When the CLI that started a run exits or closes its console, the system shall keep the worker and its job running and reconnectable. When the CLI receives Ctrl-C or Ctrl-Break, the console event shall not reach the worker or the job; the first interrupt requests a stop through the ladder (`attempt.stop_requested`, exit 130 once confirmed) and the second detaches with exit 6 and the worker alive, as `specs/*/dogfood-slice` AC-5.5 requires on Unix. Windows shall not behave differently from Unix here unless the maintainer decides so.
- **AC-3.2** [AT-06, I12, I18] When the supervisor restarts or recovery runs after a crash at a launch-intent boundary, the system shall identify the same launch by PID, creation time and nonce and shall not start a second native (a worker launches its native once and refuses a second launch, `internal/workers/worker.go:1094-1095`).
- **AC-3.3** [§6.4, I06] If the worker dies while the job holds live processes, then the system shall behave as OQ-1 decides and shall record that crash behaviour (`kill_on_close` or `survives`) in the AC-5.1 matrix row; adding a capability-record field for it is a design decision for the seam's `architect` review. Tests per AC-6.5.

### FR-4 — Descendant and orphan discovery (v2 §6.4; AT-08; I24)

- **AC-4.1** [AT-08, I02, I09] When the worker finishes or stops an attempt and the job enumeration succeeds, the system shall report `descendant_scan: job-object`. If the enumeration fails, then the system shall report `failed`, treat the stop as unconfirmed (`GroupGone` false) and end the attempt `interrupted`/`stop_unconfirmed`, because `classify` checks confirmation before descendants (`internal/workers/worker.go:932-935`); `unresolved_descendants` is unreachable on Windows since `GroupGone` includes the job being empty. The system shall never report `unavailable` on Windows once this ships; a test shall inject an enumeration failure.
- **AC-4.2** [I24, §6.4] Where a job outlives its worker (OQ-1 options b and c), when recovery runs with no live worker, the system shall find the attempt's surviving processes through the job and return their PID and creation time to the supervisor in place of `ErrUnsupported` (`internal/supervisor/recover.go:189,242,295`). Where the job ends with its worker (option a), recovery shall keep ownership unresolved unless OQ-11 accepts the OS kill-on-close guarantee plus a confirmed worker death as evidence (I09: it never reports processes gone without evidence).
- **AC-4.3** [I24, §6.4] If the job is gone and its processes cannot be shown to have ended, then the system shall keep ownership unresolved, name the evidence it lacks, and never release a resource on a lease or heartbeat timeout alone.
- **AC-4.4** [§6.4] The system shall match a process on PID and creation time, and, where a job outlives its worker (OQ-1 options b and c), on membership of the attempt's job as well (the Windows counterpart of the attempt marker, `internal/workers/orphan_linux.go:6`); under option (a) only the native's recorded PID and `native_start_time` exist to match. A reused PID is never signalled and PID alone never matches.

### FR-5 — Truthful capability record (v2 §17.1; I14)

- **AC-5.1** [I14, §17.1] The system shall report `process_tree_ownership: supported` for Windows only for an (architecture, `ExecutionSurface`) pair that has a row in `internal/processsupport/SUPPORT.md` (new, with its exactness test in the leaf package `internal/processsupport`, design D9), owned by this spec and separate from `internal/control/SUPPORT.md` (that matrix covers control methods and "does not claim a passing run"), naming the FR-1–FR-4 tests and the CI run of the commit that landed them. An exactness test shall fail a Windows `supported` without a matching row. Untested pairs report `unknown`. Whether `Platform` gains architecture and surface fields is a design decision that changes the seam and gets an `architect` review.
- **AC-5.2** [I14] The fake adapter and the claudecode adapter shall derive the Windows `process_tree_ownership` and `worker_detachment` values from the same evidence source (see G7, OQ-4), with no hard-coded `supported`, the fake returning `supported` only for rows in the AC-5.1 matrix and, while the refusal of AC-5.3 stands, the claudecode adapter returning `unsupported` (the matrix is its evidence source once the refusal is lifted); a direct unit test of the claudecode `Capabilities` shall assert `unsupported` for Windows (matching AC-5.3 and `probe.go:131-132`) while the refusal stands, because `Prepare` refuses Windows before reaching it (`adapters/claudecode/launch.go:70-72,116`).
- **AC-5.3** [I14, I02] The `claudecode` adapter shall keep refusing Windows in `launch.go` and `probe.go` (exit 7, `native_capability_unavailable`, `internal/admission/billing.go:144-145`) whatever the platform row says, and its message shall name the missing Windows qualification rather than process-tree ownership. Lifting the refusal belongs to the claudecode Windows qualification (OQ-6), not to this spec.
- **AC-5.4** [I14, AT-40, §17.1] The Windows statements in `docs/limitations.md:13-15,41`, `README.md:27`, `adapters/claudecode/COMPATIBILITY.md:69` and `internal/control/SUPPORT.md:37-40` shall list the tested Windows subset by architecture and surface and name what is untested; the `limitations.md` claim that recovery works on Windows shall be corrected to the tested subset.

### FR-6 — Windows lifecycle test evidence (G04; AT-06, AT-08, AT-40)

- **AC-6.1** [G04, AT-08] A Windows-only test shall start a fake native that spawns a child and a grandchild and reports their PIDs and creation times, stop it, and assert that neither is alive afterwards by PID and creation time, that the stop is `confirmed`, and that `descendant_scan` names the job mechanism. On current main the grandchild survives and `descendant_scan` is `unavailable` (`internal/workers/worker_windows_test.go:22`), so it fails at the assertion.
- **AC-6.2** [G04, AT-08] A Windows-only test shall start one descendant with `CREATE_NEW_PROCESS_GROUP` and one that requests `CREATE_BREAKAWAY_FROM_JOB`, stop the attempt, and assert the first is dead afterwards and that the breakaway create fails with `ERROR_ACCESS_DENIED` (the test fails if it succeeds; AC-1.5).
- **AC-6.3** [G04, AT-06, I12, I18] Windows-only tests shall crash the worker at each launch boundary, in the order the design fixes between the `native_launch_intent` write (ADR 0004) and job creation, and assert the stated outcome with no duplicate native: (i) job created, native not spawned: nothing to find; (ii) where the launch method spawns suspended (OQ-3), native spawned but not assigned: ownership unresolved (outside any job, AC-4.3); (iii) assigned, PID not recorded: ownership unresolved; (iv) PID and `native_start_time` recorded: recovery identifies the same launch. Boundaries recovery cannot tell apart end in `ownership_unresolved`.
- **AC-6.4** [G04, AT-40] Windows-only tests shall cover CLI exit and console close (worker alive by PID and creation time, reconnect identifies the same launch), a first Ctrl-C and Ctrl-Break (stop requested, confirmed, exit 130) and a second interrupt (detach, exit 6, worker alive). Where hosted runners cannot drive console events, the test is marked `blocked` with the reason, not skipped silently.
- **AC-6.5** [G04, AT-08] A Windows-only test shall crash the worker and assert, from the test side by PID and creation time as in AC-6.1, that the native and every fake descendant are gone (option a) or still alive and returned by recovery with PID and creation time (options b and c). Under option (a) recovery's own outcome is `ownership_unresolved`/quarantine unless OQ-11 accepts kill-on-close plus a confirmed worker death as evidence; recovery never reports processes gone on its own.
- **AC-6.6** [AT-40] The tests shall run with `go test -race` on `windows-latest` and `go test` without `-race` on `windows-11-arm` (`.github/workflows/ci.yml:73-77`), and each support-matrix row cites the run and its architecture.

## Non-Functional Requirements

- **NFR-1** [§6.4] A stop on Windows shall finish within the ladder's total grace plus the worker's `waitDelay` of 5 s (`internal/workers/worker.go:45`); a timeout ends the attempt `interrupted`/`stop_unconfirmed` and quarantined with no second ladder (the control-service `cancel_incomplete` code belongs to the supervisor's `stop` intent, `internal/control/stop.go:286,294`, and is outside this spec).
- **NFR-2** [I14] No Windows behaviour is inferred from Unix tests or from a cross-compile; every `supported` row cites a passing Windows run.
- **NFR-3** [I13] The tests need no agent credentials and no network.
- **NFR-4** [§17.1, `knowledge/execution.md:62`] File operations under test tolerate Windows sharing violations with the existing retry, and fail closed on a persistent error.

## Definition of Done

- [ ] AC-1.1 to AC-6.6 each have a named test or a cited document; CI green on Linux, macOS and Windows (AMD64 and ARM64 as the matrix runs).
- [ ] AC-6.1 is shown red on current main before the change.
- [ ] `process_tree_ownership` and `worker_detachment` for Windows derive from evidence (AC-5.1, AC-5.2); the `claudecode` adapter still refuses Windows until its own record exists (AC-5.3).
- [ ] Docs list the tested Windows subset and what remains untested (AC-5.4).
- [ ] NFR-1 to NFR-4 each have a named test or check.
- [ ] An ADR under `docs/decisions/` records OQ-1 and OQ-11 and supersedes the Windows statements of ADR 0004.

## Open Questions

- **OQ-1** (blocks FR-3, FR-4 design; maintainer decision, ADR-level because it changes crash behaviour and process ownership): When the worker dies, does the job kill its processes (kill-on-close) or let them survive? On Unix the native outlives a crashed worker and on Linux recovery quarantines it by marked PID (`internal/supervisor/recover.go:184-190`, `internal/workers/orphan_linux.go:6`; macOS `OrphanPIDs` returns `ErrUnsupported`, `orphan_other.go:1`). Options: (a) the worker owns the job with kill-on-close: no orphans, but no survivor to reconnect to and Unix and Windows differ; (b) a separate holder process keeps the job so the native survives a worker crash, matching Unix; (c) no kill-on-close and recovery adopts a named job. Recommendation: (a) for the first slice, (b) as a later track. The requirements are written to hold under every option (AC-1.2, AC-3.3, AC-4.2, AC-6.5); a decision narrows them.
- **OQ-2** (blocks FR-2; spike needed, owner: implementer of the ladder task): Which graceful rungs exist for a console child when the worker is `DETACHED_PROCESS` (`CTRL_BREAK_EVENT` needs a shared console)? Options: a hidden console, ConPTY, or kill only. Default: kill only (AC-2.5).
- **OQ-3** (blocks FR-1, FR-3; spike on the runners): Go's `syscall.StartProcess` on Windows takes no job in `SysProcAttr`, so assigning the native before it can spawn needs `CREATE_SUSPENDED` plus resuming the main thread, or a direct `CreateProcess` with a job-list attribute, which departs from the rule to launch through `exec.CommandContext` (`go-conventions.md`). Also: a worker inside a terminal's or runner's kill-on-close job dies with it unless it breaks away, and nesting may be refused. What is the launch method and breakaway policy? Default: refuse with `launch_failed` (AC-1.6); decider: `architect` review of the design.
- **OQ-4** (blocks FR-5): Is `worker_detachment` derived from the AC-3.1 evidence or recorded separately? Default: derived, one source for both adapters. Decider: the `architect` review of the seam (AC-5.1).
- **OQ-5** (blocks FR-6): Windows ARM64 has no race detector. This is a platform constraint (`.github/workflows/ci.yml:75`), not a decision: non-race runs satisfy AC-6.6 there, labelled by architecture (N6).
- **OQ-6** (blocks nothing): After this lands, the `claudecode` Windows qualification and ConPTY behaviour need their own card; none exists. Default: file it with `/backlog add` after approval.
- **OQ-10** (blocks MH-11 shipping claim): N5 defers the filesystem, terminal and shell combinations the card's Notes ask for. Default: file a follow-up card with `/backlog add` after approval, and do not mark MH-11 shipped until it exists or the maintainer narrows the card.
- **OQ-11** (blocks AC-4.2/AC-4.3 under OQ-1 option (a); maintainer decision, ADR-level with OQ-1): Does the OS kill-on-close guarantee plus a confirmed worker death (PID and start time) count as I24 reconciliation evidence, so ownership can be released after a worker crash? Descendants are not recorded and an unnamed job cannot be reopened by recovery. Default: no; ownership stays unresolved (quarantine) after any worker crash under (a) until decided.
- **OQ-7**: One spec or an epic? The work touches core (`internal/workers`, `internal/supervisor`), adapters, release (CI) and docs, four domains by `knowledge/domains.md`. `/spec`'s scope step decides; recommendation: single, with the adapters and docs tasks depending on the workers task.
- **OQ-8** (blocks AC-1.4; maintainer decision, persistence scope): Is the native's creation time recorded on Windows only, or on every OS (G11)? Every OS changes `worker.json`, the spool event and the journal and contradicts N4. Default: Windows only, with Unix matching unchanged.
- **OQ-9** (blocks task ordering; owner of `supervised-stop-recover`): That spec's `design.md:449,452` and its NFR-5 row say Windows Job Object handling ships there, while its `scratchpad.md:51` records it as blocked. Default: this spec owns the Windows job rows and the other spec's rows are reconciled through its reviewed lifecycle, not edited here.

## Dependencies

- **Prerequisite**: `specs/*/dogfood-slice` and `specs/*/supervisor-service` (the worker, owner lock and Windows transport it builds on) are shipped. MH-10 registry and contracts (`specs/*/qualification-registry`) are shipped and supply the capability record shape.
- **In flight, same area** (conflict check):
  - `specs/*/supervised-stop-recover` has open work on `internal/workers/worker.go` (its Task 4, including the reconnect handshake that overlaps AC-3.2) and plans Windows Job Object rows (OQ-9); its Task 4 also edits `internal/supervisor/pipeline.go` (which AC-2.1 changes), `internal/control/SUPPORT.md` and `internal/control/support_test.go`, which AC-5.4 edits too. `specs/*/supervisor-migration`'s only open task (Task 8) touches `internal/migrate/` and a new `docs/decisions/` file, so the ADR numbers of the two must be coordinated. Task ordering in `/spec` must either wait for them or keep Files disjoint (`runspec.py batch`).
  - `specs/*/claude-strict-subscription`: its admission work is complete; its open tasks touch only `tests/e2e/qualification_strict_test.go` and `adapters/claudecode/testdata/qualification/live-record.json`. No file overlap with this spec.
  - `specs/*/contained-execution-profiles` records Windows Job Objects/AppContainer as a containment option (its Q1, per-platform native, refusing where unavailable). This spec covers lifecycle only (N2); the two must not both define the Windows job. If containment later adopts a Job Object, it reuses the one built here.
  - `specs/*/budget-ledger-s1` does not touch the worker.
- **Superseded**: the Windows single-process ownership, Kill-only ladder, `descendant_scan: unavailable` and capability values recorded in `docs/decisions/0004-slice-process-model.md` (lines 14, 26, 32, 43, 45, 55). The new process-ownership ADR (OQ-1) supersedes them. No spec is superseded. The `process_tree_ownership: unsupported` Windows rows from the dogfood slice are the behaviour this changes, by design (its N7 deferred it).

## Impacted components

- `docs/decisions/0004-slice-process-model.md` (superseded by a new ADR), `internal/workers/proc_windows.go`, `contain.go:53-63` (the spawn site that applies `nativeAttr()`; contained-execution-profiles edited it in tasks since completed), `worker.go`, `orphan_other.go` (a Windows scanner), `worker_windows_test.go`, `identity_write_windows.go`
- `internal/supervisor/recover.go`, `pipeline.go`, `stop.go` (consumers of the orphan and descendant results)
- `internal/control/recover.go` (PID validity on Windows)
- `internal/adapter/adapter.go:398` (`Platform.ProcessTreeOwnership`), `adapters/fake/fake.go:82,171-173` (the fake's Windows ladder), `adapters/claudecode/probe.go:126`, `launch.go:70`
- `.github/workflows/ci.yml` (Windows matrix, if a dedicated lifecycle job is wanted); docs and support statements named in AC-5.4
- Domains: core, adapters, release (CI), docs (four); route per `knowledge/domains.md`, with an `architect` review because the change touches process ownership.
