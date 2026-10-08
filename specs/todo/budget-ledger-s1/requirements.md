## Budget Ledger S1 — Requirements

> Ships the S1 slice of the budget ledger for one-agent delivery: typed usage observations, coupled quota reservations, finite run envelopes, completion reserves and safe exhaustion without paid fallback. A slice of v2 §§7.3, 10.3. Normative source: mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md, version 2.0; § numbers, I-IDs, G-IDs and AT-IDs refer to it. Tags use the shorthand `[§X, INN]` form throughout; every §/I/G/AT ID so tagged is version-qualified to v2.0 by this declaration.

## Context

- **Backlog card**: MH-16
- **Issue**: https://github.com/turbokast/mythhelm/issues/37 (labels: enhancement, billing; no comments)
- **S1 scoping**: The card Notes scope this spec to the basic ledger/envelopes for one-agent delivery. Multi-profile and experiment scheduling extend it in S2/S4 (MH-17 account profiles, MH-27 protected experiments) and are explicit non-goals here (N1, N2). The card's Source line (v2 §§7–8, 10, 12–13; W03/W04/W11) covers the whole card; this spec binds only §7.3 and §10.3 plus the W03/W04 S1 delivery path. §§12–13 and W11 belong to the S4 experiment extension.
- **Problem**: The dogfood slice records native cost only as a labelled retail-equivalent estimate and native token counts as native-reported (non-goal N13), with no ledger behind them. One-agent S1 delivery (W04) needs more: usage observations typed by provenance, quota reservations coupled to the admitted bucket, finite execution/repair/replan envelopes, a completion reserve held before optional work, and exhaustion behaviour that preserves work and stops safely instead of charging. Without this, the first useful product cannot meet G05 billing honesty (AT-03, AT-04, AT-13) for its one qualified route.
- **Grounding verdicts** (step 4; no vendor second reader available in this session):
  - Dogfood slice stores native cost only as a labelled estimate (issue, citing N13) — HOLDS (`specs/*/dogfood-slice/requirements.md:29` N13; `internal/supervisor/receipt.go:116-117,122`; `internal/workers/worker.go:831`; `grep -n "retail_equivalent_estimate_usd" internal/supervisor/receipt.go internal/workers/worker.go`).
  - Card Source sections (v2 §§7–8, 10, 12–13; W03/W04/W11) describe the quota/ledger mechanism — HOLDS (v2 `§7.3` usage labels and exhaustion, `§10.3` finite budgets and completion reserves, `§4.1` `Reservation` type; W03/W04 S1, W11 S4).
  - MH-10 billing semantics are available as a prerequisite — HOLDS (card status `shipped`, `specs/*/qualification-registry/`; `internal/qualify/qualify.go:39-49` five `DatumLabel` values, `Record.Quota Datum`; `grep -n "DatumLabel" internal/qualify/qualify.go`).
  - Durable supervisor contracts are available as a prerequisite — PARTIAL: MH-21 exists only as the unrefined epic plan `specs/*/v2-contracts-supervisor/plan.md` with stream specs in `todo/` (`supervisor-migration`, `supervised-stop-recover`, `v2-contract-vocabulary`, `supervisor-service`); nothing shipped. Drift: MH-21 unshipped, so this spec lands first under the coordinated renumber/adoption rule (see Dependencies; design D2).
  - No budget ledger or quota reservation mechanism exists today — HOLDS (structural: no `internal/billing` or ledger package among the 14 under `internal/`; `grep -rliE "ledger|quota|budget|reservation" internal/ cmd/ adapters/` hits 30 files, all benign on inspection — TUI render budgets, the `stop_at_exhaustion` marker string in `internal/admission/qualify.go:59`, `qualify.Datum` scales, `adapter.Capabilities` Tri flags, entitlement inventory and their tests; `mythhelm.db` migrations create 11 tables, none for usage or reservations).
  - An acceptance check for this spec can be made to fail today — HOLDS (no usage/reservation tables in `internal/journal/migrations/0001_init.sql`, `0002_qualification.sql` — `grep -inE "CREATE TABLE (usage|reservation)" internal/journal/migrations/0001_init.sql internal/journal/migrations/0002_qualification.sql` returns nothing, the one `usage` hit being the `extra_usage` entitlement field at `0001_init.sql:52`; no exhaustion pause path outside the admission marker).
  - Unknown remaining quota must stay distinct from zero and no hard cap may be claimed from local state — HOLDS (`knowledge/invariants.md` I09/I10; `adapters/claudecode/probe.go:139` reports `QuotaRemaining: Unknown`, `HardMonetaryLimit: Unsupported` for the first route; `grep -n "QuotaRemaining" adapters/claudecode/probe.go`).

### Objectives

- **O1**: A run on the one qualified S1 route records typed usage observations that survive the run and distinguish reported, observed, estimated, user-declared and unknown values.
- **O2**: Allowance exhaustion preserves work, stops new model work on that bucket and never falls back to paid continuation.
- **O3**: No surface advertises a hard spending or token limit the evidence does not establish.

### Non-Goals (and what still binds)

| # | Out of scope | Still binding in this spec |
|---|---|---|
| N1 | Multi-profile routing and account/exhaustion handoff (MH-17, S2). | I02: admission still resolves the one S1 bucket; I04: a profile change needs new admission. |
| N2 | Experiment scheduling and protected experiments (MH-27, S4/W11, §§12–13). | §10.3: completion reserves still outrank any optional work; foreground demand preempts through the normal stop protocol. |
| N3 | `metered-allowed` mode (v2 §7.3 optional). | I10: no hard limit is advertised; stop-at-exhaustion only. |
| N4 | Parallel/multi-writer and native-child reservations (S3, AT-19/AT-20). | I21: the one-agent envelopes still bound execution, repair, replan and resources. |
| N5 | Provider-side quota APIs or inferred remaining balances. | I09: unknown remaining quota stays `unknown`, never `0` or an invented figure. |

## Functional Requirements

Acceptance criteria use EARS. Tags in brackets name the invariants, gates and sections each one serves.

### FR-1 — Typed usage observations (§7.3, I09)

- **AC-1.1** [§7.3, I09] When native usage output is ingested, the system shall record each quantity with its unit, scope, source, timestamp and one label of `reported | observed | estimated | user-declared | unknown`.
- **AC-1.2** [§7.3, I09] If a quantity is missing, then the system shall record it as `unknown`, never `0`, `passed` or `verified`.
- **AC-1.3** [§7.3] The system shall not aggregate unlike allowance buckets into a single fictitious tokens, currency or hours-remaining figure.
- **AC-1.4** [§7.3, I09] Native retail estimates shall be stored in the receipt and shown only as estimates in receipts and CLI run output, never as invoices or actual subscription charges, preserving the native decimal value exactly (no floating-point rounding); TUI surfaces are unchanged in S1.

### FR-2 — Counter normalization (AT-13, §7.3)

- **AC-2.1** [AT-13] When cumulative and delta counters arrive for the same scope, the system shall match readings on (scope, unit, source), treat the newest cumulative reading as the baseline, and apply only deltas whose event identity (producer, sequence) is not yet applied and whose observation time is strictly after the baseline's; deltas at or before the baseline are already covered and shall be skipped; a delta without identity shall be recorded as `estimated`, never summed into an exact total.
- **AC-2.2** [AT-13] If counter input is missing for an expected scope (identified against the caller-supplied expected scope set), then the system shall mark that scope `unknown` and keep the remaining scopes usable.
- **AC-2.3** [§7.3] The system shall expose non-overlapping cache/reasoning/output totals only where per-route qualified evidence maps native fields to those components; where unmapped, it shall record the combined total only and mark the components `unknown`.

### FR-3 — Coupled quota reservations for one agent (§10.3, §4.1, I10)

- **AC-3.1** [§4.1, §10.3] When a run is admitted, the system shall hold a typed reservation (resource/billing bucket, scope, owner, quantity or unknown, status, expiry) coupled to the admitted bucket.
- **AC-3.2** [§10.3] The system shall acquire the run's required resource set transactionally and shall not hold partial claims while waiting indefinitely.
- **AC-3.3** [I10, §7.3] A local reservation shall never be presented as provider availability or as a cap on consumption by other machines or independently launched clients.
- **AC-3.4** [§10.3] The system shall not release a reservation until the owning work has stopped, and shall never reclaim an expired reservation before reconciling its previous owner and effects.

### FR-4 — Finite run envelopes (§10.3, I21)

- **AC-4.1** [§10.3, I21] Every run shall execute inside explicit finite ceilings for execution time, repair attempts, material replans and transient transport retries, resolved explicit run flags > run-profile configuration (`mythhelm.toml [envelopes]`, design §5) > conservative built-ins (30 min, 3 repairs, 2 replans, 5 transport retries).
- **AC-4.2** [§10.3] When an envelope is exhausted, the run shall preserve its candidates and become `blocked` with a specific reason; extending the run shall require a recorded user or standing-grant decision.
- **AC-4.3** [§10.3] While a run is user-paused, only quiescent paused time shall be excluded from the execution deadline; a still-running turn shall keep consuming it.

### FR-5 — Completion reserve (§10.3, I10)

- **AC-5.1** [§10.3] Before starting optional work, the system shall reserve one verification pass plus the FR-4 repair ceiling (AC-4.1 repair attempts); no separate configured envelope exists in S1. S1 optional work means material replans after the first verification (design §6): repairs are completion work drawn from the reserve, per-kind counts stay under the FR-4 ceilings, and a replan dispatches only when remaining execution time covers the verification pass.
- **AC-5.2** [§10.3, I10] Where provider quota cannot be reserved, the system shall show the reserve in receipts and CLI run output as a conservative estimate, rely on qualified stop-at-exhaustion protection, and never advertise a hard token reserve.

### FR-6 — Exhaustion without paid fallback (§7.3, AT-13, I02)

- **AC-6.1** [§7.3, AT-13] On allowance exhaustion, the system shall preserve work and session state and stop admitting new model work on that bucket.
- **AC-6.2** [§7.3] On exhaustion, the system shall schedule at most 3 retries at an authoritative reset; when the reset is unknown, it shall back off 1 min, 5 min, 15 min and then stop, showing the timing as `unknown` throughout.
- **AC-6.3** [§7.3, AT-13] On exhaustion, the system shall never escalate to purchased credits, a paid summary, a stronger paid repair, a silently changed harness/provider or rotated identities.
- **AC-6.4** [§7.3, I04] No alternative route is preauthorised in S1, so this clause constrains future admission only, without implementing N1's deferred handoff: when a preauthorised alternative route exists, the system shall admit it only after re-admission of the destination profile; a pinned profile shall wait.

### FR-7 — Unknown quota stays unknown (§7.3, I02, I09)

- **AC-7.1** [I09, §7.3] Unknown remaining quantity shall be shown in receipts and CLI run output as `unknown`, never as zero or a remaining balance.
- **AC-7.2** [I02, §7.3] Unknown remaining quantity alone shall not block a qualified stop-at-exhaustion route; the route is allowed only when exhaustion reliably waits or stops rather than charges.

## Non-Functional Requirements

- **NFR-1** [§17.2] Ledger reads and writes on the run path shall complete within 1 s total per admission/run operation (same bound as the MH-10 registry lookup it neighbours), measured by the run-path latency test.
- **NFR-2** [I23, §5.1] Ledger rows shall live in `mythhelm.db` through additive numbered migrations; no competing file-backed task or budget store.

## Definition of Done

- [ ] AC-1.1 to AC-7.2 each have a named test that fails before the change and passes after it.
- [ ] AT-13 exercised for the S1 route: cumulative/delta/missing counters normalize, coupled quota and exhaustion preserve work without paid fallback.
- [ ] `scripts/harness/gate.sh go` green; no advertised hard limit anywhere in CLI, TUI or receipts (CLI/receipts pinned by `TestNoHardLimitAnywhere`; TUI pinned by zero hits for `grep -rliE 'hard (limit|cap)|spending limit|token limit' internal/tui/` plus no TUI file in any task's Files).

## Open Questions

- **OQ-1** (blocked design §2 schema; decided by design D2 as option (a)): Where do the usage/reservation tables land while MH-21 is unshipped — additive `0003_*` migration in the current journal-owned `mythhelm.db`, or tables owned from the start by the MH-21 supervisor service design? Options: (a) journal migration now, adopted by the service later; (b) wait for MH-21 tables and sequence strictly behind it. Blocked FR-1–FR-3. Decision: (a), consistent with NFR-2; decider: designer; Task 3 start re-confirms landing order under the renumber rule.
- **OQ-2** (blocked FR-6; resolved by design D4): What is the exhaustion signal for the first route — which native error/exit surface reliably means "allowance exhausted, retry at reset" versus a failure? Options: (a) qualify the native error taxonomy in this spec; (b) consume it from MH-12 evidence. Blocked AC-6.1/AC-6.2. Default: (a) with fixtures — (b) cannot supply live proof while MH-12 Task 7 is held per Q-15; decider: designer.
- **OQ-3** (resolved by AC-5.1): the completion reserve is one verification pass plus the FR-4 repair ceiling; `/spec` quantifies the pass. Decider: refine loop.
- **OQ-4** (blocks FR-3): Single-writer discipline for reservations before MH-21 ships — does the run owner hold the write lock (ADR-0005) or is there a narrower reservation writer? Blocks AC-3.2/AC-3.4. Answer from ADR-0005: the run owner (holder of `runs/<run_id>/owner.lock`, taken before anything is recorded) is the sole writer; workers never open SQLite. Default: run-owner writes; decider: designer confirms at `/spec`.

## Dependencies

- Prerequisite, shipped: MH-10 (`specs/*/qualification-registry/`) — `Datum`/`DatumLabel` provenance scales and the registry this ledger's quota coupling reads.
- Not a prerequisite — coordinated landing order: MH-21 (epic plan `specs/*/v2-contracts-supervisor/plan.md`, stream specs in `todo/`, unshipped) — durable supervisor contracts and the single-writer service; this spec lands first and MH-21 adopts its tables, coordinated by the renumber/adoption rule (design D2, tasks.md Dependencies); OQ-4 fixes run-owner writes meanwhile.
- Prerequisite, in progress: MH-12 (`specs/*/claude-strict-subscription/`, Tasks 1–4 and 6 complete, Task 5 open, Task 7 maintainer live run) — the one qualified S1 route and its stop-at-exhaustion evidence; OQ-2 may consume its error taxonomy.
- Downstream consumer (refined, waiting): MH-22 `specs/*/protected-acceptance/requirements.md` sequences behind MH-16 S1 and reads its evidence without reimplementing it.
- Sequenced alongside (`todo/`): MH-13 `specs/*/contained-execution-profiles/` — S1 needs both specs, but neither is the other's prerequisite (MH-13 N3 scopes ledger mechanics out; no MH-13 AC reads ledger state).
- Sibling extensions (not this spec): MH-17 (S2 multi-profile handoff), MH-27 (S4 protected experiments), MH-5 (S2 routing across profiles).
- Conflicting: none found. MH-12 non-goal N2 (`metered-allowed`) and N3 (no-egress guarantees) align with N3 here.
- Superseded: none.

## Impacted components

- New ledger package under `internal/` (core domain, `go-implementer`; exact name and signatures left to `/spec`): typed usage records, reservation lifecycle, envelope counters, exhaustion transitions.
- `internal/journal/migrations/` (core): additive `0003_*` migration for usage/reservation tables if OQ-1 resolves to (a); current tables in `0001_init.sql`/`0002_qualification.sql` carry no budget rows.
- `internal/journal/` (core): ledger read/write path shared with runs/attempts/declarations (`journal.go`, `declarations.go` as neighbouring shapes).
- `internal/supervisor/` (core): exhaustion pause/resume and candidate preservation (`receipt.go:116-122` native-reported estimate shaping; `stop.go`, `recover.go` for the stop protocol).
- `internal/admission/` (core): reservation coupling at admission beside the `stop_at_exhaustion` marker (`qualify.go:59`, AC-3.3 semantics).
- `internal/qualify/` (core, read-only consumer): `Datum`/`DatumLabel` (`qualify.go:39-94`) and `Record.Quota` as the provenance vocabulary; no changes expected.
- `adapters/claudecode/probe.go:139` (adapters): first route reports `UsageTokens: Supported`, `QuotaRemaining: Unknown` — the evidence baseline OQ-2 builds on.
- `internal/cli/` (core): run-result rendering for estimates, reserves and `unknown` quantities (AC-1.4, AC-5.2, AC-7.1); TUI packages unchanged in S1.
