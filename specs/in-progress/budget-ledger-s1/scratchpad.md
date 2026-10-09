## Budget Ledger S1 — Scratchpad

> Open questions and research from authoring. Each implementing task reads this file when it starts and adds an entry under Discoveries when it finishes.

## Scope

`scope: single`, 12 tasks, one work stream (S1 ledger for one-agent
delivery), domains: core (Tasks 1–4, 6–11), adapters (Task 5), docs
(Task 12). Why not an epic: nothing ships alone — reservations need the
billing types, enforcement needs the store, surfaces need the gates, and the
e2e needs everything; the adapters/docs tasks are single dependent leaves,
not parallel subsystems. 12 tasks is within the single-spec limit, so no
split statement is needed. `auto-confirmed (non-interactive)` — the dispatch
directive said single spec.

## Open questions

| # | Question | Default in this spec | Owner / blocks |
|---|---|---|---|
| 1 | MH-21 coordination: MH-21 stream specs in `in-progress/` claim 0003/0004 with their own `reservations` table. | Landing order follows the renumber/adoption rule in tasks.md Dependencies (Task 3 re-checks at start; single `reservations` DDL). | Task 3 owner / blocks Task 3 start |
| 2 | First-route mapping evidence: which decoded fields map to which non-overlapping components? | Task 2 ships the row against the synthetic stream-json fixtures and cites them; MH-12 T7 live evidence revises the row if fields differ. | Task 2 owner / blocks Task 2 mapping row only |
| 3 | Exhaustion native shapes: which documented error surfaces mean "allowance exhausted, retry at reset"? | Task 5 maps only shapes documented in the current native docs, fixtures marked synthetic; anything undocumented stays `native_error` (fail closed). | Task 5 owner / blocks Task 5 |

Resolved at authoring (from requirements Open Questions): OQ-1 → option
(a), `0003_ledger.sql` now, adoptable by MH-21 later (design D2); OQ-2 →
option (a) with fixtures, default kept (design D4); OQ-3 → resolved by
AC-5.1, quantified in design §6/D7; OQ-4 → run-owner sole writer per
ADR-0005, standing (design D3). AC-4.1's "run-profile configuration" surface
is named in design §5/D5 (`mythhelm.toml [envelopes]`). AC-4.2's extension
is the recorded user decision `run.extension_granted` (no standing-grant
table in S1; honesty register). AC-6.2's retries are operator-executed
against the recorded schedule (no scheduler daemon in S1; D13).

## Research notes

- Investigation findings (authoring session, tree at `git status` clean
  apart from untracked spec dirs): master v2 §§7.3, 7.4, 10.3, 4.1, 4.5,
  5.1; invariants in reach I02, I04, I06, I09, I10, I12, I14, I21, I23;
  gate G05 (AT-13 in scope; AT-03/AT-04 consumed from MH-10/MH-12).
- Grounded seams: `qualify.go:39-94` labels/Datum; `qualify.go:88-142`
  (admission) strict consult; `worker.go:828-843` native_result shaping;
  `worker.go:663-664` Retry counting; `worker.go:793-803` outcome mapping;
  `adapter.go:305-310` TokenUsage; `decode.go:42-47,341-399` classes/cost/
  usage; `probe.go:139` route caps; `receipt.go:115-123` estimate shaping;
  `state.go:41-55,96-101,226` machine/reasons/launch intent;
  `pipeline.go:61-70,95,269,391` hooks/Run/attempt/freeze;
  `ingest.go:155` projection; `journal.go:28-30,281` version/Append;
  `render.go:26-31` renderer; `run.go:27-30` flag style;
  `projectconfig.go:38-60` config subset; `recover.go:40` recovery.
- Absence claims verified structurally: no `internal/billing` among the 14
  `internal/` packages; no usage/reservation tables in the 11-table
  `mythhelm.db`; no `allowance_exhausted` outside MH-21 todo specs and the
  admission marker name; no native retry loop in the worker (count only).
- Conflicts scanned: `specs/todo/` (MH-21 stream specs claim
  0003_supervisor.sql with their own `reservations` table and 0004 after it;
  coordinated by the renumber/adoption rule in tasks.md Dependencies, not by
  waiting; `supervised-stop-recover` in todo shares pipeline.go/worker.go —
  ordering note in tasks.md Dependencies),
  `specs/in-progress/` (MH-12 open tasks touch disjoint files only),
  `specs/todo/` (MH-13 alongside), `specs/refined/` (MH-22 downstream),
  `specs/done/`
  (dogfood `tasks.md` used as the format reference; MH-10
  `TestRegistryLookupLatency` as the NFR-1 shape).

## Discoveries

- Task 10: the fake adapter surfaces neither usage nor cost (`fake.go` maps
  `UsageTokens: Unsupported` and ignores the `usage-counters` scenario's
  per-turn usage), so every fake run to date journals exactly one usage
  row — the always-expected `retail-equivalent` scope as an `unknown`
  marker. Task 11's AT-13 counter test needs a `fake.go` decoder extension
  (a spec deviation, as Task 8's `fake.error` frame was) or it can only
  assert the marker, not accumulated deltas.
