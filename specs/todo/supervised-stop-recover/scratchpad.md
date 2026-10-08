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

- (Tasks append here.)
