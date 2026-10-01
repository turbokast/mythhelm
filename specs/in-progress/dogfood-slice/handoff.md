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

- `integration.Freeze(ctx, workdir, baseRev, CommitMeta)` snapshots tracked and untracked, non-ignored content through an explicit temporary `GIT_INDEX_FILE`. It parents the commit on the admitted base, regardless of workspace HEAD, sets `refs/mythhelm/candidates/<attempt>`, and returns `Candidate{BaseRev, Commit, Tree, PatchSHA256, Changed, Flags, Partial}`. The private index never touches the source checkout. `workspace.GitWithIndex`, `GitPatchSHA256` and `GitBlob` retain the Git runner's sanitized environment and stream large output.
- `journal.Candidate(ctx, attemptID)` reads the projection inserted atomically with `candidate.frozen`. Its `ChangedPaths` and `Flags` are JSON; `CandidateRow.Partial` distinguishes failed/cancelled attempts. The event fires only after `attempt.stopped` confirmed an empty unresolved PID set. The worker already classifies escapees as interrupted; the supervisor never freezes those attempts.
- Admission captures Git user.name/user.email in `Decision` and the `admission.decided` event. Task 15 recovery must read that recorded identity, never current mutable config, before calling `Freeze` after an ownership resolution. Missing identity blocks admission.
- Task 12 can verify the candidate commit from the projected row. Native success still ends `verification_unavailable` until the check runner exists. Task 14 must refuse `apply` for interrupted/unresolved runs, even if a stale candidate ref exists; this task's current CLI acceptance can only assert refusal because `apply` is introduced in Task 14. Candidate commits intentionally have no Signed-off-by; the maintainer adds that after review.

## Task 12 — Project config, trust grants and the check runner

- `admission.Decide` reads only the selected committed `mythhelm.toml` blob, rejects unknown TOML keys and validates a SHA-256 trust grant bound to the source repository realpath. It never takes checks from the mutable source checkout. After cloning, `pipeline.snapshot` loads the snapshot's committed config blob and compares the digest before worker launch; Git checkout line-ending conversion on Windows cannot change that identity. A newly approved grant is projected atomically with `workspace.snapshot_created`; existing grants are read from the local journal.
- `integration.RunChecks` uses the frozen candidate commit in a detached managed worktree. The runner executes the admitted argv without a shell, in order, with per-check timeouts, output policy, redacted evidence and SHA-256. `unavailable` skips later checks unless `--keep-going`; other failed statuses do not skip later checks. `Verification` and check rows are projected with `verification.completed`.
- Native success plus all checks passing ends `ready_for_review`/exit 0; check failure or unavailable ends `failed/verification_failed`/exit 5. `--no-checks` ends `ready_for_review/unverified`/exit 5 and journals `NOT RUN`. Native failure still ends exit 4 with its partial candidate. Task 13 should use the projected verification and candidate records for the receipt/review command. Task 14 must require `--accept-unverified` for unverified apply.
- The root `mythhelm.toml` supplies gofmt, vet and test checks for Task 20. Its digest changes with any config edit, so a new trust grant is required. `internal/cli/exit.go` maps invalid config and denied passthrough to exit 2.

## Task 13 — Receipt and `mythhelm review`

- `supervisor.BuildReceipt(ctx, j, runID)` reads the run, latest attempt, candidate and verification projections plus allowlisted admission/native observations. It leaves absent usage and cost as `"unknown"`; reported retail-equivalent cost has `"source":"native-reported"` and `"note":"estimate, not a charge"`. The requested-outcome title comes from the already stored `task.md` only after its SHA-256 matches the admitted task digest. The title and task text do not enter the journal.
- `pipeline.runTo` writes `runs/<id>/receipt.json` for ready_for_review, blocked, failed and cancelled states, then appends `receipt.written{schema_version,sha256,path}`. `WriteReceipt` creates a 0600 temp file, syncs it and renames it over the receipt. `review` opens the journal read-only, verifies the latest journaled hash and matches receipt run ID/state to the current projection, then reads the frozen candidate diff through `workspace.Git` and `security.TermSafe`; `--no-diff` avoids Git and `--format jsonl` emits a single compact object.
- **For Task 14:** apply should read the v1 receipt only after verifying its journaled SHA-256, preserve it as `receipt.v1.json`, add the external effect and completed state, then atomically write/journal v2. The current receipt writer always targets `receipt.json`; extend it for the preserved version instead of bypassing the atomic path. A new terminal state transition to completed is outside Task 13's `pipeline.runTo` path, so Task 14 must explicitly write v2 after apply reconciliation.
- **For Task 15:** a terminal state is journaled before `receipt.json` is written. If building, writing or journaling the receipt fails, the run remains terminal without a reviewable receipt. The recovery command must detect that state and regenerate the version 1 receipt from the journal and projections under the owner lock, then journal its hash. Do not treat a `receipt.failed` marker by itself as recovery.
- Task 13 acceptance tests live in `internal/supervisor/receipt_test.go` and `internal/cli/review_test.go`; the end-to-end fake run also confirms the candidate diff actually renders. The cost fixture constructs a reported value after a fake run because the fake adapter deliberately reports no usage or cost. Secret-shaped test input is assembled at runtime to satisfy public hygiene.
- Windows CI exposed a sharing violation when the supervisor read `worker.json` during the worker's atomic replacement. `acceptWorker` now retries only Windows sharing/lock errors under its existing bounded identity poll; all other read errors still refuse worker acceptance. This is a supporting fix for the Windows acceptance gate.

## Task 14 — Guarded apply

- `workspace.ApplyBranch(ctx, userRepo, workspace, candidateRef, branch, candidateCommit)` validates the exact frozen ref, refuses a branch at a different commit, and reconciles a branch already at the candidate without fetching only with matching recorded intent. Initial apply refuses every existing branch. It disables hooks, fsmonitor, automatic maintenance, commit-graph writes and submodule recursion; fetch does not force or write `FETCH_HEAD`. The fetch imports objects without a destination ref, then a prepared no-deref `update-ref` transaction locks the exact destination, rejects ordinary and symbolic existing refs, and atomically creates the branch. Exact full-ref validation rejects reflog shorthand such as `@{-1}`. This avoids the design's destination-refspec race, which could fast-forward a concurrently created branch. `workspace.BranchCommit` and `CheckBranch` support preflight. `ErrBranchExists` maps to exit 3 and `ErrInvalidBranch` to exit 2.
- `supervisor.ApplyRun` is called under `AcquireOwner` by the CLI. It requires ready_for_review (or a journaled in-flight intent), accepted candidate flags, `--accept-unverified` for unverified runs, a source base commit, and a digest-verified v1 receipt. It journals `apply.intent_recorded`, transitions through `applying`, applies/reconciles the branch, journals `apply.completed`, transitions to `completed`, and writes and journals receipt v2. It preserves the exact verified v1 bytes in `receipt.v1.json`. `ReadReceipt` now centralizes the journal SHA-256 verification for both `review` and `apply`.
- **For Task 15:** a crash after replacing `receipt.json` with v2 but before journaling its SHA-256 leaves a v2 file whose latest journaled digest is v1. Recovery should use the preserved `receipt.v1.json` only after checking it against the v1 journal digest, then finish or repair the v2 receipt under the owner lock. The same repair path should cover the pre-existing Task 13 terminal receipt-write failure.
- The fake-adapter acceptance tests are in `internal/supervisor/apply_test.go`; the Git effect and hook tests are in `internal/workspace/apply_test.go`.
- `apply --format jsonl` emits one `apply.result` object with the process exit code and error category for success and refusal (AC-1.3). The success also names the completed state and candidate commit; it does not dump the receipt.
- A conflicting ordinary or symbolic destination, or missing source base after a recorded intent ends the in-flight run as `blocked` with a fresh journaled v1 receipt. A refusal before intent leaves the run ready for a different branch. This prevents a failed apply from keeping the active-run gate occupied indefinitely.

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
