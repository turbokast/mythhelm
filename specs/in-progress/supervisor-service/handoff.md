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

<!-- pending -->

## Task 4 — Windows named-pipe transport

<!-- pending -->

## Task 5 — Intent server, idempotency ledger and sole-writer transactions

<!-- pending -->

## Task 6 — Reservations, capability tokens, authority filtering and lazy start

<!-- pending -->

## Task 7 — Topology ADR and support matrix

<!-- pending -->
