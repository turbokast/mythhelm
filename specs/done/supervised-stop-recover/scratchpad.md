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
- **Task 2 review round 1 — narrowed no-lock fallback (coderabbit Major, valid, fixed)**: `pinAdmission` pinned generation 0 on every `CurrentGeneration` error, but design §3 fails the admission on unreadable metadata and `readMetadata` requires that a torn read never look like an absent holder — only an absent lock or a foreign root (the handoff's documented cases) may pin unfenced. `CurrentGeneration` now maps those two to `ErrNoSupervisorLock`/`ErrRootConflict` and fails closed otherwise; `pinAdmission` falls back to 0 only on the sentinels, returning the error into `admission_pin_failed` otherwise. Evidence: `TestCurrentGenerationFailsOnUnreadableLock` plus `TestPinAdmissionCorruptLockFailsAdmission` (failed admission journals no `attempt.admission_pinned`).
- **Task 3 — one-pass record and the continued handoff**: the record is the latest `attempt.recovery_decided` event per run with evidence markers (attempts, candidates, non-recovery journal max, worker producer generation, spool bytes, worker.json stat, liveness). Same target plus equal markers replays; a target change to the record's own `NewAttemptID` replays while the continuation has journaled nothing (its first journaled event opens its first examination), so concurrent winners/losers record exactly once and a fresh pass never quarantines a newborn. Any other target change re-examines.
- **Task 3 — quarantine from running hops through interrupted**: the v2 table forbids running/waiting/launching→quarantined directly, so the pass applies two checked transitions in one tx; `quarantined→quarantined` restates (the table's explicit row) while `interrupted→interrupted` is skipped.
- **Task 3 — known edges**: a complete (non-partial) candidate on a dead worker counts as clean and continues — production ingest would have concluded that attempt already; launch-then-commit is not atomic, same as any spawn-in-tx.
- **Task 3 review round 1 — launch-in-tx deferred to Task 4 (coderabbit Major, valid, deferred)**: verified against `ledger.go` (`execute` wraps the handler in one `Mutate`, an inner `Mutate` joins it): `RecoverHandler` launches the continuation before `Execute` commits, so a commit failure after a successful spawn orphans a worker whose admission rolled back with the operation — the next pass cannot observe it (no attempt row, no record) and admits and launches a second continuation. No fix in this round: a durable post-commit handoff needs `Execute` support (`ledger.go`, outside Task 3's files) plus the launcher wiring. Safe to land: `recover` is unserved (`RegisterRecover`/`Reconcile` have no non-test callers; `NewSupervisorServer` has no `recover`), pinned by `TestRecoverUnservedUntilLaunchLeavesTransaction`. Task 4 must land the fix with registration. This also corrects the earlier known-edge claim that the next pass quarantines the orphan — it cannot see it.
- **Task 3 review round 1 — spool reads bounded at `spool_offset` (coderabbit Major, fixed)**: `scanSpoolFacts` seeks to the attempt's acknowledged offset and reads through a 1MiB `LimitReader` (the `scanStopped` precedent); over-bound spool quarantines as `spool_unverifiable` under `ownership_unresolved`, never truncated silently.
- **Task 3 review round 1 — gone verdict probes the observed worker (coderabbit Major, fixed)**: `classifyWorker` returns `workerGone` over a mismatching worker.json only when the observed PID is provably gone too, so no continuation is admitted beside a live stranger; the probe joins the markers as `observed_live`.
- **Task 3 review round 1 — newborn replay bounded (coderabbit Major, fixed)**: an admitted continuation that journals nothing past `MaxRecoveryContinuationAge` (15min) is re-examined and quarantines `ownership_unresolved` instead of replaying `continued` forever.
- **Task 3 review round 1 — `Reconcile` keeps admitting continuations (coderabbit Minor, rebutted)**: failing direct `Reconcile` calls closed on `continued` would break Task 3 acceptance (`TestReconcileChoosesOneOutcome` requires `continued` with an admitted `launching` attempt and zero launches) and Task 4's reconnect handshake, which consumes `Reconcile`; the stuck-continuation hazard is bounded by the newborn expiry instead.
- **Task 3 review round 1 follow-up — invalid PIDs probe unknown (coderabbit, fixed)**: both liveness probes validate the PID first (`validPID`: positive, DWORD width on Windows, pid_t elsewhere), so garbage identity data fails closed to `unknown`/quarantine instead of wrapping in the Windows `uint32` conversion or reading a confident `gone` from `/proc` ENOENT.
- **Task 4 — verification environment**: `internal/cli` `TestStrictMainBlocksWriteNothing` fails under the operator's real `$HOME` (`untrusted_native_config` from the user-level native settings) and passes under a clean `HOME`; run Go gates with a scratch `HOME` (keep `GOCACHE`/`GOMODCACHE`/`GOPATH` and a non-asdf `python3` on `PATH`). CI has a clean home. The `internal/supervisor` package alone takes about 400s under `-race`; a full `go test -race ./...` can hit the default 10m package timeout on a loaded machine.
- **Task 4 — inherited deferrals closed**: `Execute` long handlers (`ExecuteLong`), the post-commit recover launch, and generation-0 semantics are covered by tests (see the completion entry). The one open item is `Reconnect` having no production caller; no later task or backlog card tracks it.
- **Task 4 — fixtures must release fenced workers**: since Task 4 a worker with a nonzero generation waits for a fresh `supervisor.beat` after its episode when the beat is stale. Any test whose supervisor exits or detaches before the worker concludes leaves that worker alive holding `spool.jsonl`; Linux and macOS delete the open file, Windows fails TempDir cleanup. `newFixture` now reaps workers by writing beats; a test that spawns workers outside `newFixture` must do the same.
- **Task 4 review round 1 (coderabbit)**: fixed `insertClaim` swallowing ledger failures (now `ON CONFLICT DO NOTHING`, other errors surface), unbounded long handlers (`runLong` cancels at `handlerBudget`, 4m, under the 5m `maxClaimAge`), and `continuationUnspawned` treating a gone worker's identity as unspawned (any identity now blocks a relaunch, I12). Rebutted: the unbounded reconnect wait (the design parks the worker until the supervisor returns; the production return caller is the recorded open item) and the architect-review prose finding (a merge-process step, not code).
- (Tasks append here.)
