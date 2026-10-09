# Supervised Stop and Recover — Scratchpad

Seeded at spec creation. Every task appends its discoveries here; the
lifecycle skills own the other files.

## Open questions

- **OQ-SR1**: Ladder version scheme — opaque string (`stop-ladder/v1`) vs
  semver with compatibility rules? Default: opaque string; a mismatch is
  `revision_conflict`, never a compatibility guess. Decides: Task 1
  implementer; reviewer confirms.
- **OQ-SR2**: Per-surface default signals and graces (Unix groups, Windows
  Job Objects, native agents that trap SIGINT)? Default: keep the current
  interrupt → terminate → kill order with the existing graces as the
  `stop-ladder/v1` fixture, marked `fixture-tested`. Decides: Task 2
  implementer with platform-test evidence.
- **OQ-SR3**: Supervisor-loss detection threshold — how long a missed
  generation-heartbeat window before the worker treats the supervisor as
  lost? Default: 3× the heartbeat interval, bounded and named in code.
  Decides: Task 4 implementer.
  - Fix-run resolution: the window is `MaxSupervisorBeatAge` (300ms = 3×
    the 100ms supervisor-beat interval) over `supervisor.beat`
    (design §5); the implementer keeps the 3× default.
- **OQ-SR4**: go-winio dependency for the Windows control transport (epic
  OQ-7) — if still unapproved when Task 2's Windows test needs the service
  path, the Windows ladder test runs against the request-file path and the
  service-path row stays `blocked`. Decides: maintainer (dependency);
  Task 2 implementer records the outcome here.

## Research notes

- v2 §6.4 stop/recover text: `mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md`
  lines 352–362 (ladder, one-pass recovery, supervisor-loss envelope,
  new-generation token on return).
- Current ladder seam: `ClimbLadder` (`internal/adapter/adapter.go:199`),
  `StopStep` (`adapter.go:115`), `RequestStop` first-request-stands
  (`internal/workers/worker.go:263`).
- Current recovery seam: `RecoverWithHooks` (`internal/supervisor/recover.go:40`),
  `quarantineRecovery` (`recover.go:331`).
- Consumed verbatim: `specs/*/v2-contract-vocabulary/design.md` (lifecycle,
  errors, generation checks), `specs/*/supervisor-service/design.md` (intent
  machinery, `Mutate`, tokens). This spec redefines none of them.

## Discoveries

- **Task 1 — OQ-SR1 resolved**: keep the opaque-string default. `StopParams.LadderVersion` must equal the pinned version exactly; any mismatch is `revision_conflict`, never a compatibility guess. Reviewer to confirm.
- **Task 1 — `Execute` failure-with-writes pattern**: a handler that returns a `*Error` gets its writes rolled back (`runHandler` savepoint), so a failure that must persist state (quarantine here, the one-pass record in Task 3) returns the failure in the `Result` with a nil error instead. The stored result still replays as the failure.
- **Task 1 — no `stop` token check**: the design lists no `permission_denied` case, so the handler does not check capability tokens; same-user peers act as the operator (the `read`-intent precedent). Token-scoping for `stop` is a future hardening, not this spec.
- **Task 1 — stopped-report `ladder_version`**: the design §3 payload gains the field because acceptance requires rejecting wrong-version reports; Task 2 emits it and the handler skips reports that fail validation (wrong version, missing `sent`) until the deadline.
- **Task 1 review round 2 — stop wait holds `Execute`'s tx (coderabbit Major, valid, deferred)**: verified against `ledger.go` (`execute` wraps the handler in one `Mutate`, `runHandler` attaches the tx, an inner `Mutate` joins it) plus `_txlock=immediate`/`busy_timeout(5000)`: the wait holds the write lock for up to `MaxStopDeadline` and other mutating intents fail `persistence_unavailable`. No fix in this round: it needs `Execute` long-handler support (`ledger.go`, outside Task 1's files) or a second idempotency shell, either breaking Task 1 acceptance (`TestStopRepeatDuringWaitJoins` pins block-then-replay). Safe to land: `stop` is unserved (`RegisterStop` has no non-test callers; `NewSupervisorServer` has no `stop`), pinned by `TestStopUnservedUntilWaitLeavesTransaction`. Task 4 must land the fix with registration.
- **Task 2 — OQ-SR2 resolved**: keep the default. The per-surface proposal ladders travel as `stop-ladder/v1` (Unix interrupt 3s → terminate 3s → kill 5s; Windows kill 5s alone), pinned by the spawn site. Evidence: `TestWorkerClimbsPinnedLadder` on unix (group confirmation, both rungs recorded) and on Windows (kill confirmation); multi-rung confirmation and job-object scoping stay `blocked` on Windows (`Signal` supports kill only; no Job Object per N7). Reviewer to confirm.
- **Task 2 — OQ-SR4 outcome**: go-winio v0.6.3 is already in `go.mod` (landed via supervisor-service task 4, #266), so the dependency question is moot; the T2 Windows ladder test runs against the request-file path regardless — no service-path coverage was needed.
- **Task 2 — no-lock fallback (design §3 deviation)**: `pinAdmission` pins generation 0 (unfenced) when `CurrentGeneration` fails instead of failing the admission. The legacy run path runs lockless by design during migration (svc D1: each `mythhelm run` is its own supervisor), and a leaf run on a fresh root or a demo on a temp root cannot hold the lock — failing would brick both with no remedy, and a demo-side acquisition would poison the lock file for later roots. Zero is never a real boot generation (they start at 1), so it marks unfenced rather than guessing one; the pin (version + nonce digest) is still journaled, so the `stop` path is unaffected. Task 4 defines 0's envelope meaning.
- **Task 2 — legacy-shaped handoffs climb unchecked**: the worker's identity gate arms only when `Launch.Nonce` is set; an empty nonce (direct test spawns, pre-pin workers) climbs exactly as before. Supervisor-side PID/start/token checks still guard those paths.
- (Tasks append here.)
