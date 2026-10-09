# supervisor-service — Hand-off

> One section per task. Each task replaces its own `<!-- pending -->` line in its pull request with what
> dependent tasks need: what it produced (the API and files as shipped), what a later task must
> know, and any deviation that changes a later task's inputs. `/run-spec` puts the sections of a
> task's dependencies into that task's dispatch prompt. Open questions and research stay in
> `scratchpad.md`.

## Task 1 — Instance lock and root-conflict refusal

- **Produces**: `control.LockPath() (string, error)`, `control.AcquireInstance(dir string) (release func(), err error)`, `control.ErrInstanceHeld`, `control.ErrRootConflict` in `internal/control/{lock,lock_unix,lock_windows,lock_test}.go` — exactly the task's list.
- **For dependents**: lock path is `$XDG_RUNTIME_DIR/mythhelm-<uid>/supervisor.lock` (Unix, `$TMPDIR` fallback) or `%TEMP%\mythhelm-<user>\supervisor.lock` (Windows); never under any state root.
- **For dependents**: `release` is idempotent; the OS frees the flock on process death, so kills never wedge the lock.
- **For dependents**: lock file holds JSON `{root, pid, started_at, generation}` readable while held on all platforms (Windows locks a sentinel range disjoint from the metadata bytes); every successful acquire bumps generation (starts at 1). Task 5/6 `status` reads pid/generation/root from it.
- **For dependents**: refusals are held flock + same root → `ErrInstanceHeld`, held flock + other root → `ErrRootConflict` (both via `errors.Is`; messages name the root).
- **For dependents**: metadata is written in place on the flocked file — never rename-replace the lock path (a new inode would escape the flock).
- **For dependents**: root comparison uses Abs + best-effort EvalSymlinks; byte compare on Unix, case-insensitive on Windows.
- **For dependents**: lock dir is created 0700, symlinks refused, owner-checked on Unix.
- **For dependents**: tests take the real per-user lock and run sequentially (no `t.Parallel`); the stale test re-execs the test binary (a `TestMain` holder branch) and kills it.
- **Deviations affecting later tasks**: none — adoption without a liveness probe (see tasks.md) keeps the published contract; Tasks 5–6 consume the Produces signatures verbatim.

## Task 2 — Frame codec and NFR-1 ingress enforcement

Produced (PR #251): package `internal/control` (`frame.go`, `frame_test.go`,
`testdata/depth64.json`, `testdata/depth65.json`).

- `control.Frame`: the wire object — embeds `v2contract.RequestEnvelope` plus
  `method`, `params` (raw JSON), `capability_token` (accepted but opaque;
  mint/check ship in Task 5). `Validate` enforces only the envelope.
- `control.Encode(v any) ([]byte, error)`: JSON + 4-byte big-endian length
  prefix. `control.Decode[T v2contract.Validator](frame []byte) (T, error)`:
  prefix check + `v2contract.Decode` (strict, trailing-data rejected).
- `control.CheckIngress(frame []byte) error`: prefix match → strict decode as
  `Frame` → streaming depth/refs scan → `v2contract.CheckFrameLimits`.
  Depth = max simultaneously open containers (top-level object is 1), so 63
  nested objects under `params` measure 64. Refs = string-valued
  `artifact_id` keys anywhere in the payload (non-string values do not count).

What later tasks must know:

- Call `CheckIngress` on every received frame before dispatch (Task 3
  transport, Task 5 server accept). `CheckIngress` takes one complete `[]byte`;
  bounding in-flight reads (prefix + `MaxFrameBytes`) is the transport's job.
- Task 5's `Intent` wire shape MUST use only `Frame`'s known top-level keys
  (`operation_id`, `object`, `expected_revision`, `generation`, `method`,
  `params`, `capability_token`): any other key fails ingress as unknown.
- Deviation: violations are plain errors, not `protocol_mismatch`
  `*v2contract.ControlError` (vocab task 6 unmerged; no parallel code defined).
  Wrap them once vocab task 6 lands; tests assert dimension-mentioning messages.
- Note: the Go 1.27 stdlib JSON decoder also caps nesting (~10k levels);
  pathological frames fail closed at strict decode before the NFR-1 depth
  check. The binding limit stays `MaxNestingDepth` (64), enforced via
  `CheckFrameLimits`.

## Task 3 — Unix transport and peer authentication

- **Produces** (PR #253): `control.Transport`, `Listener`, `Conn`, `Peer`, `ErrNoSupervisor`, `control.NewUnixTransport()` (linux/darwin only; `transport.go` itself builds everywhere, so Task 4's Windows transport implements the same interfaces).
- **For dependents**: `Listener.Accept` returns only same-UID peers; `Conn.Receive(ctx)` reads one request frame (prefix bounded by `MaxFrameBytes`, then `CheckIngress`) and `Conn.Respond(frame)` writes the reply; `Conn.Request` is the client call. `Dial` returns `ErrNoSupervisor` for a missing or refused socket. `Listen` creates the parent dir 0700 and refuses an existing dir with group/other bits; it does not remove a stale socket (instance lock, Task 1, owns that).
- **Deviations**: `Conn` has `Receive`/`Respond` beyond the design; refusal is a plain `{"code":"permission_denied","message":...}` frame until vocab task 6 provides `ControlError`; two extra platform files (`transport_peercred_{linux,darwin}.go`).

## Task 4 — Windows named-pipe transport

<!-- pending -->

## Task 5 — Intent server, idempotency ledger and sole-writer transactions

- **Produces** (PR #255): `control.Intent` (+`Validate`), `Result`, `Handler`, `Execute`, `Mutate`, `Server` (`NewServer`, `Register`, `Dispatch`), `StatusHandler`, `AssignHandler`, `MintToken`, `CheckToken`, `EndpointPath`, `WithLedger`, and `control.Error` with `Code*` constants, in `control.go`, `ledger.go`, `server.go`. Migration `0004_supervisor.sql` (`operations`, `capability_tokens`, `run_assignments`), `SchemaVersion` 4.
- **For dependents**: attach the ledger with `control.WithLedger(ctx, db)` before `Dispatch`/`Execute`; without it they fail closed. `Execute` returns a handler's `*control.Error` as both `Result.Error` and the error, and stores it: a repeat replays the same failure. Any other handler error is transient: nothing is stored and a retry runs the handler again. Return `Result.Revision` (the object's new revision); `Execute` requires `expected_revision` to equal the object's latest recorded revision. `Execute` holds one write transaction across the handler: write only through `control.Mutate(ctx, db, ...)` with the handler's ctx (it joins that transaction, so the writes commit with the result or not at all); a handler that writes through `db` directly waits on the write lock and fails. Reads through `db` do not see the transaction's uncommitted writes.
- **For dependents**: `reservations` was created by `0003_ledger.sql` (budget-ledger-s1), not by this migration. It has no execution-host key and no `expired` status; Task 6 must rebuild or extend it before keying by host (design §8). Its accessors live in `internal/journal/ledger.go`.
- **For dependents**: `control.Error` replaces `*v2contract.ControlError` until v2-contract-vocabulary task 6 lands; swap it then and keep the code strings. `control.Error` matches by code under `errors.Is`.
- **For dependents**: there are no claim rows: a crash before the commit leaves nothing, so a retry runs the handler again, and concurrent duplicates wait on the SQLite write lock (5 s busy timeout) and then replay the winner's result.
- **For dependents**: capability token digests live in `capability_tokens`, not on `attempts`; `MintToken` rotates on re-mint. `CheckToken` is for Task 6's handlers to call per request. The control socket path is `EndpointPath()`, beside the instance lock.
- **Traps**: `internal/workers` already depends on `modernc.org/sqlite` through `journal.Event`; AC-6.1 is pinned by a source scan, not `go list`. Run tests with `ANTHROPIC_BASE_URL` unset and lint with `GOTOOLCHAIN=go1.27.1`; the status test sets `XDG_RUNTIME_DIR`, so it is not parallel.

## Task 6 — Reservations, capability tokens, authority filtering and lazy start

- **Produces** (PR #258): `control.Reserve`/`Release`/`Heartbeat` and `ReserveHandler`/`ReleaseHandler`/`HeartbeatHandler`/`ReadHandler` (`reserve.go`, `filter.go`); `control.Filter`/`Row`; `NewSupervisorServer(db)` binding status, assign, reserve, release, heartbeat and read; `OpenLedger`, `Serve`, `Connect`, `Call`, `RunSupervisor`, `DefaultTransport`, `SupervisorCommand` (`server.go`, `client.go`, `default_{unix,other}.go`); `mythhelm supervisor status [--format jsonl]` and the hidden `__supervisor` entry (`internal/cli/supervisor.go`, `cmd/mythhelm/main.go`). Migration `0005_reservation_host.sql`, `SchemaVersion` 5.
- **For dependents**: reserve params are `{bucket, scope, quantity, owner, bound?}`, release `{reservation_id, evidence}`, heartbeat `{reservation_id}`, read `{repos}`; unknown keys are `invalid_contract`. The reservation id is `res_<operation_id>`. The host is the supervisor machine's name; a client cannot set it. A reservation owned by an attempt needs that attempt's capability token on the intent; one owned by a run needs only the same-user peer.
- **For dependents**: only one `held` reservation exists per (execution_host, bucket, scope). Rows with an empty host (any writer that does not set `execution_host`, such as `journal.InsertReservation`) are outside that rule, so budget-ledger-s1's admission reservations are not blocked by it; set the host to opt in. `expired` is not a status here: use `orphaned` for the reconcile input. Nothing reaps expired reservations yet.
- **For dependents**: the supervisor serves one `Result` reply per request frame, with failures in `Result.Error` (a `control.Error` code); `control.Call` returns that error too. `Serve` calls `WithLedger` itself. A new method needs a binding in `NewSupervisorServer`.
- **For dependents**: `Connect` starts a detached supervisor (own session, environment limited to `MYTHHELM_HOME`, `XDG_RUNTIME_DIR`, `XDG_STATE_HOME`, `HOME`, `TMPDIR`) and polls for up to 10 s. Racing starters lose the instance lock and exit 0. `Connect` asks the supervisor for its state root (one `status` intent) and returns `ErrRootConflict` unless it equals this process's resolved root, so a client never attaches to another root's ledger; callers get a verified connection. The supervisor has no idle shutdown; stop it with a signal. Windows has no transport yet: `DefaultTransport` returns `ErrSpawnUnsupported` and the command exits 7 until task 4.
- **Traps**: the packaged-binary tests build `./cmd/mythhelm` once per test process and need a short `XDG_RUNTIME_DIR` (Unix socket path limit); they are Unix-only. Reading `runs` rows in `read` is the only read surface. There is no per-repository policy store: a token-less same-user peer (the operator, who can read the state directory anyway) is scoped by the `repos` it presents, while a request with an attempt capability token is bounded to that attempt's run repository (the list can only narrow it; a list with none of it is `permission_denied`). A real entitlement source needs the v2 §5.1 repository records (opaque IDs bound to canonical repository identity), which do not exist yet.

## Task 7 — Topology ADR and support matrix

- **Produces** (PR #262): `docs/decisions/0014-service-topology.md` (status `proposed`), `internal/control/SUPPORT.md` and `internal/control/support_test.go` (`TestSupportMatrixMatchesEvidence`, `TestWindowsRowMatchesLanding`).
- **For dependents**: a new control method needs its `method:<name>` row in `SUPPORT.md` in the same change, with evidence tests that exist in `internal/control`; a new non-method deliverable adds its row there and its ID to `shippedDeliverables` in `support_test.go`. A row's `Platforms` (`all` or `linux, darwin`) must match the build tags of its tests. Statuses are `fixture-tested` or `blocked` only.
- **For dependents**: when task 4 lands `transport_windows.go`, change the `transport-windows-pipe` row to `fixture-tested` with its tests (platform `windows`-built tests need a third platform label in `testPlatforms`), and the ADR's Windows line; `TestWindowsRowMatchesLanding` fails until both agree. The ADR needs the maintainer's review before its status becomes `accepted`.
