# 0014. Service topology: one per-user supervisor replaces one supervisor per run

- Status: proposed (awaiting maintainer review; supervisor-service task 7)
- Date: 2026-10-09

## Context

[ADR 0005](0005-run-owner-lock-and-ingestion.md) made `mythhelm run` the
supervisor of its own run: an OS lock at `runs/<run_id>/owner.lock`, no daemon.
Four non-test sites take that lock today (`pipeline.go`, `recover.go`,
`cli/apply.go`, `cli/tui.go`). That was right for one run, and it cannot hold
what v2 §3.1 and §5.1 require: global reservations keyed by execution host and
resource, a single logical writer for the ledger, idempotent control requests
that survive client exit (I06), and no second authority when two roots or two
clients meet (AC-5.1, AC-5.4). It changes process ownership, so it needs a
record (spec-authoring rule; design D8).

## Decision

- **One supervisor per user per execution host.** It holds a per-user instance
  lock (`control.AcquireInstance`) at a path derived from the OS user, never
  from `MYTHHELM_HOME`. The lock records `{root, pid, started_at,
  generation}`; adopting a stale lock bumps the generation. A live lock on a
  different root is `ErrRootConflict`, never a second authority.
- **Lazy start, detached.** The first `mythhelm supervisor status` (or any
  client of `control.Connect`) starts the supervisor as `mythhelm __supervisor`
  in its own session, and a racing starter loses the instance lock and exits.
  A client verifies the supervisor serves its own state root before using it.
  The supervisor has no idle shutdown and no service registration (follow-up).
- **Private, authenticated local control only.** A 0700 Unix socket with a
  same-UID peer check (a Windows named pipe is the same design, task 4). No TCP
  listener exists. Attempt-scoped capability tokens bound what an attempt may
  do.
- **Idempotent intents, sole writer.** Every mutation is an intent with an
  `operation_id` and `expected_revision`; `Execute` stores its result in the
  `operations` ledger in the same transaction as the handler's writes, so a
  repeat replays and a conflicting reuse fails without executing (I12). Workers
  never call the journal's database API.
- **Run ownership becomes an assignment** under the supervisor (`assign`). The
  four per-run `AcquireOwner` sites stay until the migration stream drains
  them; the service and the legacy path do not write the same run at once
  (spec `supervisor-migration`).

## Alternatives rejected

- **Stay per-run (ADR 0005 as is).** Each `mythhelm run` keeps its own
  supervisor. Rejected: nothing owns a cross-run reservation, so two runs can
  both read the same bucket as free and both proceed (I05); there is no place
  for a ledger writer that outlives a client; and a TUI, an `apply` and a `run`
  on one root have no common authority to ask. Per-run ownership stays correct
  inside one run and is kept for the migration period.
- **Remote or TCP control.** Rejected (I03, N5): a network listener makes the
  transport an authority.
- **A separate store for operations and reservations.** Rejected: two writers
  and two failure modes; the ledger already is the single store (I23).
- **A Windows service now.** Deferred: a lazy process gives Unix and Windows
  one singleton and adoption design.

## Consequences

- One long-lived process per user now holds the ledger writer. Its failure
  modes (a crashed supervisor, a stale lock, a socket left behind) are handled
  by lock adoption and by removing a stale socket under the held lock.
- The ledger gains migrations 0004 (`operations`, `capability_tokens`,
  `run_assignments`) and 0005 (`reservations.execution_host`); both are
  additive.
- Not done here: Windows transport (blocked on the go-winio dependency
  decision), the stop and recover control methods (stream 4), draining the
  per-run owner sites (stream 3), and a per-repository entitlement store.

NFR-3 platform rows (Go tests that pin each claim; `internal/control/SUPPORT.md`
is the checked matrix, and `TestSupportMatrixMatchesEvidence` keeps it in step
with the code):

| NFR-3 row | Platforms | Tests |
|---|---|---|
| Filesystem: instance lock, adoption after a kill | linux, darwin, windows (CI matrix) | `TestSecondInstanceHeld`, `TestRootConflictRefused`, `TestStaleLockAdoptedWithGenerationBump` |
| Process: lazy spawn, single supervisor under a start race | linux, darwin | `TestLazySpawnThenStatus`, `TestConcurrentSpawnBecomesClient`, `TestRootsRacingOneWinnerNeverBothAttach` |
| Detach: the supervisor stays up and serves a new request after its client exits | linux, darwin | `TestSupervisorSurvivesClientExit` |
| Transport and peer authentication | linux, darwin | `TestUnixRoundTrip`, `TestForeignUIDRejected`, `TestNoTCPListener` |
| Windows named pipe | windows | blocked: `TestWindowsRowMatchesLanding` pins the row to the landing of task 4 |

All run with `CGO_ENABLED=0`. No row is live-qualified.
