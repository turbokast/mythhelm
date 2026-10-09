# 0015. Budget ledger S1: billing, persistence and process ownership

- Status: proposed
- Date: 2026-10-09

## Context

The budget-ledger S1 slice gives every run a durable ledger — typed usage
observations, one coupled quota reservation, one envelope row and, on
exhaustion, one bucket row — plus finite run envelopes and a stop-safe
exhaustion path. The spec's design §12 records fourteen decisions (D1–D14);
this record fixes the billing, persistence and process-ownership ones
(D1–D3, D6, D13, D14) that later slices must not silently change. Each keeps
its spec decision ID below.

## Decision

**D1 — one billing package.** Ledger types, counter normalization, envelope
ceilings and the completion-reserve estimator live in `internal/billing`.
Storage stays in `internal/journal`: the billing package carries semantics
(ceilings, labels, estimates) and never opens SQLite. Rejected: `internal/ledger`
(the layout names `billing`) and stuffing the types into `journal` (mixes
storage with billing semantics).

**D2 — additive migration `0003_ledger.sql`.** The four ledger tables
(`usage_observations`, `run_envelopes`, `reservations`, `bucket_state`) land
as additive migration `0003_ledger.sql`, bumping `SchemaVersion` 2→3; 0001 and
0002 are byte-identical. 0003 was the next free number while only 0001/0002
existed, coordinated by the renumber rule: MH-21's supervisor stream and MH-13
also claimed 0003, so whoever lands second renumbers, and the second
`reservations` DDL is adopted rather than duplicated. Rejected: waiting for
MH-21 (unbounded stall) and an uncoordinated 0003 (landing order would silently
break a peer).

**D3 — the run owner is the sole ledger writer.** No service exists in S1, so
whichever process holds the run's owner lock (ADR 0005) writes that run's
ledger rows inside its own transactions — usage rows with the ingest append,
the envelope row and the reservation with the admission append. MH-21 adopts
the tables later. Rejected: inventing a service inside this slice (MH-21's
scope).

**D6 — S1 built-in ceilings are 30m/3/2/5.** The fallback envelope is 30
minutes of execution, 3 repairs, 2 replans and 5 transport retries per run,
resolved flags, then `[envelopes]` file, then built-ins. Execution is tighter
than the standing contract's 2h, repairs and transport looser (3 > 2, 5 > 3),
replans equal; the contract permits explicit configurable ceilings, so the
slice stays compliant. Transport retries count ingested `attempt.progress`
observations per run.

**D13 — retries are operator-executed, never automatic.** S1 records the
AC-6.2 retry schedule, shows it (`billing.next_retry`, the `next retry:` line)
and enforces it at re-admission, but nothing in the one-agent tree can wake a
run, so the operator runs each retry. Unknown reset backs off exactly 1m, 5m,
15m then stops; at most 3 retries; an exhausted native launch happens exactly
once. Rejected: a background scheduler inside S1 (service scope) and silent
auto-retry (would contradict the exactly-once launch pins).

**D14 — the reserve gate covers material replans only.** Before dispatching a
material replan after the first verification, the run must show execution time
left covering one more verification pass, else it blocks with
`completion_reserve_shortfall`. Repairs are completion work, not optional work:
gating them on the full repair ceiling would allow at most one repair and
contradict the admitted 3-repair ceiling. Rejected: gating every
post-verification attempt (the contradiction) and a count-based replan
predicate (replans consume no repairs; the launch gate already enforces the
replan ceiling).

## Consequences

- One typed vocabulary (`billing.Reading`, `ScopeKey`, `Ceilings`,
  `ReserveEstimate`) shared by admission, ingest, gates, receipts and the CLI;
  exhaustion codes are the exact strings `allowance_exhausted` and
  `budget_exhausted` for MH-21 to adopt.
- One ledger store: the journal gains tables, never a second database; every
  ledger write commits with the run event it describes, so a failed hold rolls
  the admission back instead of leaving a partial claim.
- Unknown stays unknown to the surface: receipts and CLI lines render
  `unknown`, never zero or an invented balance, and money is always an
  estimate, never a charge.
- **Tests that pin this behaviour**: `TestBuiltInCeilingsAreS1Values`,
  `TestResolveCeilingsPrecedence`, `TestMigration0003CreatesLedgerTables`,
  `TestMigration0001Untouched`, `TestSchemaVersionIs3`,
  `TestHoldCouplesAdmittedBucket`, `TestAdmissionWritesInitialEnvelope`,
  `TestOneBucketPerRun`, `TestRetryScheduleResetAndUnknown`,
  `TestEvaluateBucketAdmitsAndClears`, `TestNoSecondNativeLaunch`,
  `TestReceiptUnknownNeverZero`, `TestNoHardLimitAnywhere`,
  `TestE2EExhaustionPreservesWithoutFallback`.
