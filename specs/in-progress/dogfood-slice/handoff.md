# dogfood-slice — Hand-off

> One section per task. Each task replaces its own `<!-- pending -->` line in its pull request with what
> dependent tasks need: what it produced (the API and files as shipped), what a later task must
> know, and any deviation that changes a later task's inputs. `/run-spec` puts the sections of a
> task's dependencies into that task's dispatch prompt. Open questions and research stay in
> `scratchpad.md`.

## Task 1 — Go module, skeleton and CLI dispatcher

- `internal/cli`: add commands to the `commands` map in `dispatch.go` (use `newFlagSet` + `parseFlags`; `usageErrorf` → exit 2); map new error types in `exitCode()` in `exit.go`. `buildinfo.Get()` gives version/commit/go/os/arch.

## Task 2 — CI quality gates: golangci-lint, govulncheck, CodeQL Go, Dependabot gomod

- golangci-lint (`.golangci.yml`, v2.13.2) and govulncheck (v1.8.0) run under `CI OK`; run them locally with `go run …@<pin>`. Dependabot does not bump `go run` pins. `Analyze (go)` is a required check.

## Task 3 — CI supply chain and runner matrix

- New Go tests must pass on ubuntu-24.04-arm and windows-11-arm (no -race on windows/arm64). Allowed licences live on the single `allow-licenses:` line in .github/workflows/dependency-review.yml (go-licenses reads it). OSV-Scanner (non-required, separate workflow) fails on any known vuln in deps.

## Task 4 — State directory, IDs and the SQLite journal

- State: `statedir.Resolve()` → `statedir.Ensure(dir)` → `journal.Open(ctx, dir)` … `Close()`. `journal.Append` takes a `project` func run inside the same transaction (error = full rollback incl. the journal row). Producer sequences start at 1 and continue across generations; run_sequence is assigned by Append. IDs: `ids.New("run")` adds the underscore. Driver modernc.org/sqlite v1.59.0. ADR 0003 records local-state decisions (Windows owner-only ACL + full-path symlink checks are documented limitations).
- **For Task 5:** `journal.ErrSchemaTooNew` must map to exit 2 in internal/cli/exit.go — the first command that opens the journal (Task 5 `runs list`) adds it.

## Task 5 — State machines, projections and `runs list`

- State changes ONLY via `supervisor.CreateRun`, `RecordLaunchIntent`, `TransitionRun(*Producer, …)`, `TransitionAttempt(*Producer, attemptID, …)`; any other projection write (base rev, pids) goes in `journal.Append`'s `project` callback with its event. One `supervisor.NewProducer(ids.New("sup"), 1)` per supervising process, shared (goroutine-safe); never reuse a producer id. `ErrIllegalTransition`; blocked/failed/interrupted/failed_native require a reason; ready_for_review accepts none or `unverified`. `journal.OpenReadOnly` + `ListRuns` for read paths; `(*Journal).Attempt`, `ErrNotFound`, `ErrNoDatabase`. `runs list` doesn't yet show worker liveness (needs Tasks 9/15). `ErrSchemaTooNew` → exit 2 is now wired.

## Task 6 — Security primitives

- `security.ErrDeniedPassthrough` must map to exit 2 in `internal/cli/exit.go` — whichever task wires `[environment] passthrough` (Task 12) adds it.
- `security.BuildEnv` always removes `CLAUDE_CODE_OAUTH_TOKEN`; for the AC-4.7 opt-in the Claude Code adapter (Tasks 16/17) must copy that one variable from the parent env into the child env afterwards.
- `security.ResolvesOutside` returns an error for loops/unreadable links/missing root — callers (Task 7 preflight, Task 11 freeze) must treat an error as flagged/blocked.
- Secret-shaped test fixtures must be assembled at run time (e.g. "sk-"+"ant-…") or scripts/ci/check-public-hygiene.sh fails.

## Task 7 — Workspace: safe git runner, preflight and snapshot

- `internal/workspace`: `Git(ctx, dir, userRepo, args...) ([]byte, error)` (non-zero exit → `*GitError{Args, ExitCode, Stderr}`; stdout capped 256 MiB → `ErrOutputTooLarge`; strips all inherited GIT_* and sets GIT_TERMINAL_PROMPT=0; adds only the design's four -c flags); `Preflight(ctx, repo) (PreflightResult, error)` (lists unsupported features FeatureShallow/Submodules/LFS/Sparse — admission maps non-empty → exit 7; `Dirty` drives AC-3.2); `Snapshot(ctx, src, rev, dst) error` (resolves rev to full SHA, removes all remotes, refuses existing dst); `SourceFingerprint(repo) (string, error)` → "sha256:<hex>". `ErrGitTooOld` must map to exit 7 in internal/cli/exit.go (whoever first surfaces it — Task 10 admission).
- **For Task 11:** Pass the temp `GIT_INDEX_FILE` explicitly (runner strips GIT_*; add an env-taking runner variant). `diff --binary` >256 MiB fails with ErrOutputTooLarge — hash the patch as a stream. Windows snapshots get CRLF via system core.autocrlf — don't let that show up as spurious changes in the candidate.
- **For Task 14:** pass `-c fetch.writeCommitGraph=false` before the subcommand in args.
- internal/workspace tests use t.Setenv (HOME, XDG_CONFIG_HOME) → no t.Parallel() there.

## Task 8 — Adapter seam, NDJSON framing and the fake adapter

- Adapter seam in `internal/adapter/adapter.go`; `OwnedProc` = {Stdout, Signal, Wait, GroupGone}; `NativeExit` has `Err`. NDJSON bounded reader in `internal/adapter/ndjson`. Fake adapter `adapters/fake` with 13 scenarios in `adapters/fake/scenarios/*.json`; hidden `__fake-agent` subcommand dispatched in cmd/mythhelm/main.go (not the commands map). Adapters may NOT import os/exec.
- **For Task 9:** Launcher must not hand the stdout pipe to descendants; session reads stdout to EOF then calls Wait() exactly once. `GroupGone()` is the stop-confirmation authority (Unix: kill(-pgid,0)==ESRCH); `Session.Interrupt` escalates the ladder until it holds. Flip the fake's platform facts (worker detachment, process-group ownership) from `unknown` to `supported` on Linux/macOS once tests prove them. The escapee inherits MYTHHELM_ATTEMPT_ID (findable via /proc/*/environ on Linux).
- Consumers must drain `Observations()` (only progress frames drop on a full channel); `Done()` fires after it closes.
- **For Task 18:** scenario `happy` writes demo.txt containing "ok"; `check-fails` writes "fail" — key the demo check on that file.
- Tests that re-exec the -race test binary: set GORACE=atexit_sleep_ms=0 in the child env (else +1s per exit).

## Task 9 — Worker ownership: detached worker, process group, spool, stop ladder

- `internal/workers`: `Spawn(exe, stateDir, runID, attemptID, workers.Launch{…})` returns the worker process (caller must Wait or Release). Launch is handed to the worker as JSON on stdin, never on disk (it may carry the AC-4.7 token). Accept a worker only if `ReadIdentity(dir)` matches `ProcessStartTime(pid)` AND sha256(token) matches the journaled one. Every spool ends with `attempt.native_result` → `attempt.stopped` (always emitted; carries unresolved_pids, signals_sent, descendant_scan) → terminal `attempt.state_changed`. `RequestStop(attemptDir, id)` (first request wins). Reason codes incl. launch_failed, worker_persistence_failed, native_signal_<name>, stop_unconfirmed, unresolved_descendants. ADR 0004 = slice process model. Fake capability now `supported` for detachment/process-group on Linux/macOS.
- **For Task 15:** Linux /proc marker scan = `markedPIDs` in internal/workers/proc_linux.go (reuse for orphan scan). Worker panic journaling (design §11) NOT done yet — Task 15 should add it.
- **For Task 17:** register `claudecode` in the `workers.adapters` map; in-flight apiKeySource stop (AC-4.4) and protocol_error classification are Task 17's.

## Task 10 — Supervisor run pipeline, first against the fake adapter

- `admission.Decide(ctx, Request)` returns a fully resolved `Decision` or blocks before writing state. `supervisor.Run(ctx, Decision, Hooks)` creates the run, journals admission and snapshot, records launch intent, spawns and verifies one worker, then ingests its spool under `owner.lock`. `Hooks.Event` receives journal events in run-sequence order; JSONL adds `run.result` outside the journal. `internal/cli/run.go` owns flags, signals and exit mapping (ADR 0005).
- **For Tasks 11–12:** Extend `pipeline.attempt` after `watch` has ingested the worker's terminal state and before `conclude` reports the result. A confirmed native success currently moves `executing → verifying → failed` with `verification_unavailable` (exit 5); Task 11 freezes the candidate, and Task 12 replaces that temporary verification fallback. Never freeze while `out.Detached`, `RunInterrupted`, or the worker's stop is unconfirmed. The admitted snapshot is `Decision.Workdir`; the source checkout is never an execution workspace.
- **For Tasks 15–17:** The owner lock and spool offset live in `internal/supervisor/ownerlock.go` and `ingest.go`; recovery must reconcile them rather than replay a native call. A queued Ctrl-C before `workers.Spawn` cancels without launching; once spawned and accepted, `watch` requests a stop and reports cancellation only after confirmation. An unaccepted worker is marked lost without a stop request, because its identity is unverified. An interrupted attempt with `cancelled_before_spawn` has a launch intent but no worker to recover. Registering another adapter also requires its worker-side entry in `internal/workers`.

## Task 11 — Candidate freeze and validation flags

<!-- pending -->

## Task 12 — Project config, trust grants and the check runner

<!-- pending -->

## Task 13 — Receipt and `mythhelm review`

<!-- pending -->

## Task 14 — Guarded apply

<!-- pending -->

## Task 15 — Stop, recover, and the §18.4 fault-injection subset

<!-- pending -->

## Task 16 — Claude Code probe, auth evidence and billing admission

<!-- pending -->

## Task 17 — Claude Code launch, stream-json decoding and result mapping

<!-- pending -->

## Task 18 — Offline demo and read-only doctor

<!-- pending -->

## Task 19 — Packaged-binary end-to-end suite and README status

<!-- pending -->

## Task 20 — Dogfood: run MYTHHELM on MYTHHELM and record the evidence

<!-- pending -->

## Cross-cutting notes

- `gh pr edit` is broken on this repo (Projects classic deprecation): update PR bodies with `gh api -X PATCH repos/turbokast/mythhelm/pulls/<n> -F body=@file`. Bare `git push` is blocked by a guard: always `git push origin <branch>`.
- Required checks: CI OK, DCO sign-off, Dependency review, Analyze (actions), Analyze (go). golangci-lint (.golangci.yml) and govulncheck run in CI under CI OK — run them locally too: `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2 run` and `go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...`.
