# 0004. Slice process model: detached worker, spool and stop ladder

- Status: accepted
- Date: 2026-09-29

## Context

The master specification gives each attempt a dedicated worker that owns the native child, its transport and its process identifiers, and outlives the UI (§7.3). A requested stop must never be shown as confirmed before the process is gone (I06). Recovery must never relaunch the native agent (I12), and an agent has one lifecycle owner (I18). Unix ownership is a process group with a staged interrupt, terminate, then kill; Windows uses Job Objects (§7.4). Where ownership cannot be proven, MYTHHELM must say so.

The dogfood slice has no per-user supervisor daemon and no local IPC (N11), and Windows process-tree ownership is deferred (N7). This record fixes how the slice meets §7.3 and §7.4 without them (design §3, decision D6).

## Decision

**Worker process.** Each attempt runs `mythhelm __worker --state <dir> --run <id> --attempt <id>` (`workers.Spawn`, `workers.Main`). On Unix it is started with `Setsid`, so it is in its own session and process group. A terminal hangup, or a signal to the CLI's job, does not reach it. On Windows it is started with `CREATE_NEW_PROCESS_GROUP | DETACHED_PROCESS`. Its stdout is the null device and its stderr is `worker.log` in the attempt directory, so it never writes to a terminal that may have gone. The worker never opens SQLite.

**Launch hand-off.** The supervisor passes the admitted launch (`workers.Launch`: token, adapter ID, path, argv, working directory, environment, prompt path and stop ladder) as JSON on the worker's stdin, at most 1 MiB. The launch never touches the disk, because its environment may carry the AC-4.7 opt-in credential. The worker starts the adapter's session with that exact proposal; it does not call `Prepare` again.

**At most one native launch per attempt.** The worker creates `spool.jsonl` with `O_EXCL` before anything else. A second worker for the same attempt fails there and launches nothing. The worker's launcher also refuses a second `Launch`.

**Identity (§7.3).** `worker.json` holds the worker's PID, its start time, the launch token, `native_launch_intent` (written before the native process is created), and the native PID and process group (written after). It is replaced atomically (temp file, fsync, rename). The supervisor accepts a worker only if the token, the PID and the start time all match (`workers.ReadIdentity`, `workers.ProcessStartTime`). Start times come from:

- Linux: `/proc/stat` `btime` plus field 22 of `/proc/<pid>/stat` (USER_HZ = 100).
- macOS: `p_starttime` from `sysctl kern.proc.pid`.
- Windows: the creation time from `GetProcessTimes`.

**Native process.** The launcher runs the native with an argv array and no shell, with exactly the admitted environment (an empty list never inherits the worker's), the prompt file on stdin, stdout on a pipe, and stderr in a 256 KiB ring that is redacted into `stderr.log` when the native exits. On Unix the native leads its own process group (`Setpgid`). On Windows it gets its own console process group and no window.

**Spool.** `spool.jsonl` is the worker's durable outbox: one `journal.Event` per line, producer `wrk_<attempt_id>`, generation 1, `producer_sequence` from 1 with no gaps. `run_sequence` is left for ingestion. Every event except `attempt.progress` and `attempt.protocol_counters` is fsynced before the worker continues. Progress is coalesced to at most one event per second. Whichever process holds the run's owner lock ingests the spool.

**Heartbeat and stop request.** The worker rewrites `heartbeat` (`{"beat":n,"at":…}`) every 2 s. A heartbeat means the worker is alive, not that the model is making progress. The worker polls `stop.request` every 250 ms. `workers.RequestStop` creates that file whole, by a hard link from a synced temporary file, and only if no request exists, so the first request stands. The worker records it as `attempt.stop_requested{requested_by}` and `attempt.state_changed{stop_requested}`, then runs the adapter's stop ladder.

**Stop ladder and confirmation.** The worker delivers each rung to the whole process group (`kill(-pgid, sig)`) and waits the rung's grace for the group to be gone. The group is gone only when `kill(-pgid, 0)` fails with `ESRCH`. The session reaps the leader after reading its stdout to EOF, so a zombie leader still counts as present. On Windows the only rung is `Process.Kill`, confirmed once `Wait` returns.

**Ending every attempt.** After the native exits, the worker emits `attempt.native_result`: explicit `null` result fields when no result frame was seen, and `exit_code` `null` when a signal ended the process. If no stop was requested, it then runs the ladder once more, which sends nothing when the group is already gone, so a group member left behind is still stopped before freezing. It then scans for known descendants that left the group. It always emits `attempt.stopped{confirmed, unresolved_pids, signals_sent, descendant_scan}`, followed by the terminal `attempt.state_changed`, which is decided in this order:

1. Group not confirmed gone: `interrupted` / `stop_unconfirmed`.
2. A known descendant still runs, or the scan failed: `interrupted` / `unresolved_descendants`.
3. A stop was requested: `stopped`, whatever the exit code or signal.
4. Otherwise the native result mapping (AC-5.7): `succeeded_native` only for a successful result frame and exit 0. Anything else is `failed_native` with reason `<native error class>`, `<result subtype>`, `result_unobserved`, `native_exit_<n>` or `native_signal_<name>`.

**Escaped descendants.** Every native child carries `MYTHHELM_ATTEMPT_ID=<attempt>`. On Linux the worker scans `/proc/*/environ` for it (`descendant_scan: proc-environ`), and on macOS `sysctl kern.procargs2` of each process (`sysctl-procargs2`). The kernel only shows a process's environment to its owner, which covers descendants running as the user. On Windows the scan is `unavailable`, and the worker claims nothing about descendants. The worker reports the PIDs it finds and never signals them.

**Capability record.** The fake adapter reports `worker_detachment` and `process_tree_ownership` as `supported` on Linux and macOS, where this record's tests pass on CI. On Windows they are `unknown` and `unsupported`, and on other systems both are `unknown`.

## Consequences

- The CLI can exit, lose its terminal or be killed while the native keeps running under an owner, and `stop` or `recover` can still reach that owner through files in the attempt directory.
- A crash of the worker between `native_launch_intent` and the native PID leaves `worker.json` showing the intent without a PID; recovery reports `ownership_unresolved` (Task 15).
- **Known limitations.**
  - A clock step moves Linux `btime`, so a live worker may then fail its identity check. It is treated as lost and never signalled, which is the safe direction.
  - A descendant that leaves the process group while holding the native's stdout keeps EOF from arriving, and the session never reaps the leader. The fake's escapee uses the null device. Whether any Claude Code descendant can hold that pipe is unverified.
  - Once the leader is reaped and the whole group is gone, the PGID may be reused. The ladder checks the group before each rung, so the window is one grace period.
  - Windows ownership is a single process until Job Objects arrive (N7). `claudecode` is blocked there.
  - Other Unix systems cannot read a process start time yet, so the worker refuses to run there.
- **Tests that pin this behaviour** (`internal/workers`): `TestWorkerSurvivesParentExit` (Linux, macOS), `TestStopLadderEscalatesToKill`, `TestFakeStopExit130ClassifiedStopped`, `TestStoppedExitEmitsNativeResultWithNulls`, `TestUnresolvedDescendantReported` (Linux, macOS), `TestSpoolSequencesContiguous`, `TestCriticalEventsFsynced`, `TestProcessStartTimeStable`, `TestWindowsFakeStopConfirmed` (Windows), `TestWorkerRefusesSecondLaunch` and `TestWorkerRejectsInvalidLaunch`.
