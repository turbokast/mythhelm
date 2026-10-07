## Qualification Registry — Requirements

> A versioned registry of harness compatibility and qualification records covering the seven v2 candidate harnesses, with honestly-labelled qualification progress and drift triggers, so admission and users can see what is qualified, experimental or blocked. A slice of v2 §§4, 7–8, 14, 17–18. Normative source: mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md, version 2.0; § numbers, I-IDs, G-IDs and AT-IDs refer to it.

## Context

- **Backlog card**: MH-10
- **Issue**: [#31](https://github.com/turbokast/mythhelm/issues/31) (card discussion; card holds status, stage, score)

Stage 0 must establish one viable native included route plus the lifecycle/trust tests and v2 contracts (v2 §18.1, S0 row), and every advertised capability must point at versioned evidence or be explicitly experimental/unsupported (I14). Today strict `--billing subscription-only` always blocks (`internal/admission/billing.go:56`), the only compatibility record is a single-harness Markdown note (`adapters/claudecode/COMPATIBILITY.md`, fixture-tested, no live qualification evidence), and there is no versioned registry store any surface can query: `CapabilityRecord.Qualification`/`FidelityQualification` (`internal/adapter/adapter.go:351,357`) are per-probe descriptor strings, `entitlement_qualification_unavailable` (`internal/admission/billing.go:56`) is a block code, `Qualified` bools (`internal/adapter/adapter.go:143`, `internal/admission/billing.go:52`) are unqualified-by-default flags, and `actionRegistry` (`internal/tui/palette.go:35`) is the TUI palette — none is a queryable record store. The dogfood slice's Claude Code probe (`adapters/claudecode/probe.go`) is reusable input, not qualification.

This spec creates the registry: versioned per-harness records with independent fidelity, entitlement and security/lifecycle columns (v2 §7.1), the seven-harness scope of v2 §7.2, drift triggers that invalidate affected evidence on binary or config change (v2 §7.1), and next-test pointers for blocked routes. A first included-only route is proven through authorised tests within this card's authority: no live usage or account changes are authorised (card Notes), so live qualification evidence arrives only where the maintainer has explicitly permitted it, and sign-in or user declaration never stands in for no-paid-continuation evidence (I15).

Grounding verdicts (2026-10-07):

- `subscription-only always blocks today` HOLDS (`internal/admission/billing.go:56`).
- `dogfood slice ships a Claude Code probe` HOLDS (`adapters/claudecode/probe.go`, `COMPATIBILITY.md`).
- `no versioned qualification registry store exists` HOLDS (`grep -rin "registr\|qualif" internal/ adapters/ cmd/ --include='*.go'`: only descriptor strings, block codes, default-false flags and the TUI palette; `COMPATIBILITY.md` is one harness, Markdown only).
- `seven-harness scope` HOLDS (v2 §7.2 lists seven candidate harnesses).
- `unknown mandatory entitlement/no-overage evidence blocks; unknown remaining quota alone need not block a qualified stop-at-exhaustion route` HOLDS as v2 §7.3 source; code blocks strictly today with no qualified route.
- `issue #31 section references (§9.2, §9.14, §18.3, §20.2, §22.2 item 1, gates at §18.7)` PARTIAL: v2 §18 holds only §18.1–§18.3 with gates at §18.3; the demands exist in v2 §§4, 7–8, 14, 17–18 per the card, which govern.
- `no live usage or account changes authorised by this card` HOLDS (card Notes; v2 §7.1 requires explicit permission this card does not grant).

### Objectives

- **O1**: Admission and users can read, for any of the seven candidate harnesses, whether each execution surface is qualified, experimental or blocked, with the versioned evidence behind the label.
- **O2**: A first included-only route is proven through authorised tests, with entitlement and no-paid-continuation evidence independent of sign-in or user declaration.
- **O3**: Binary or relevant config drift invalidates the affected evidence instead of silently surviving it.

### Non-Goals (and what still binds)

| # | Out of scope | Still binding in this spec |
|---|---|---|
| N1 | Earning a live strict admission-pass for any route — MH-12 owns that for Claude Code. This spec delivers the registry, honest labels, and the first route's authorised-test evidence up to but not including a live strict pass; if Q1 stays Claude Code, FR-3's record is the authorised-test basis MH-12 builds on. | I14: every other surface stays labelled `blocked`, `experimental` or `unsupported` with its next test, never implied qualified. |
| N2 | Live billing qualification without maintainer permission. | v2 §7.1: live tests need explicit allowance permission and disposable fixtures; without it the record stays non-live. |
| N3 | Herdr bridge fixtures beyond what §16 needs for attachment records. | I17: host attachment keeps its own compatibility record; pane state never certifies qualification. |
| N4 | Router, scheduler, multi-writer or learning behaviour built on the registry. | I20: records carry immutable revisions; a material change invalidates affected authority/evidence before reuse. |

## Functional Requirements

Acceptance criteria use EARS. Tags in brackets name the invariants, gates and sections each one serves.

### FR-1 — Versioned qualification records (§7.1, I14, I20, G02)

- **AC-1.1** [§7.1] The system shall store one record per harness × surface × executable build × OS/arch × entitlement class × trust profile, keyed exactly per the full v2 §7.1 qualification key (including adapter/protocol, provider/endpoint class, model/snapshot or moving alias, effort settings, auth category, instructions/tools/skills/hooks/plugins/MCP digest, and workspace/environment class).
- **AC-1.2** [§7.1] Each record shall carry independent fidelity, entitlement and security/lifecycle columns, each valued `proven | not-proven | unknown`; a `proven` in one column shall not change the verdict of another.
- **AC-1.3** [I20] Each record revision shall be immutable; a material change shall create a new revision and invalidate evidence bound to the old one.
- **AC-1.4** [I14, G02] When a surface has no passing evidence, the registry shall report it as `blocked`, `experimental` or `unsupported`, never as qualified.

### FR-2 — Seven-harness coverage with honest progress (§7.2, I14, G02, G12)

- **AC-2.1** [§7.2] The registry shall hold records for all seven v2 §7.2 harnesses (Claude Code, Codex, OpenCode, Meta Muse Code, Kimi Code CLI, Cursor Agent CLI, Antigravity).
- **AC-2.2** [§4] Each record shall use the v2 §4 progress scale (`planned | documented-candidate | fixture-tested | live-qualified | experimental | blocked | unsupported`) and the capability scale (`supported | unsupported | unknown`) with evidence, scope and expiry.
- **AC-2.3** [§7.1] A `fixture-tested` record shall not satisfy any check that requires `live-qualified` evidence.

### FR-3 — First included-only route proof (§7.3, I15, I02, G05, AT-03, AT-04)

- **AC-3.1** [AT-03] When authorised tests establish included-only entitlement and no-paid-continuation evidence across all implementer, planner, reviewer, summary, router, child and experiment routes of the first route with no hidden paid auxiliary, the registry shall record the entitlement column as proven with the authorising evidence. Bootstrap order is evidence → record → admission consults the record (AC-5.1). Under the fixtures-only default the `fixture-tested` record is demonstrated via `RecordDraft` plus its registry round-trip tests, never seeded into production databases (which stay `blocked`/`planned`), and strict admission keeps blocking per AC-3.2; with maintainer-granted live-test permission it may advance up to but not including a live strict admission pass, which stays out of scope per N1 (MH-12 owns it).
- **AC-3.2** [I15, AT-04] If entitlement or no-overage evidence is missing or rests only on subscription sign-in or user declaration, then the record shall stay non-qualified and strict admission shall keep blocking.
- **AC-3.3** [§7.3, I02] When remaining quota is unknown but exhaustion reliably waits or stops rather than charges on an otherwise qualified stop-at-exhaustion route, the registry shall record the entitlement column `proven` with the unknown quantity labelled `unknown`, never `0`.

### FR-4 — Drift triggers and next tests (§7.1, §8, G07, AT-11)

- **AC-4.1** [§7.1] When the native binary digest or relevant configuration (effective startup hooks, MCP, plugins, managed configuration, and pinned instruction/tool/skill digests per the v2 §7.1 qualification key) drifts from a record's pinned values, the system shall treat the affected evidence as invalid and re-report the surface as non-qualified at every consult; the `InvalidateOnDrift` revision API persists this once a production writer exists (MH-12).
- **AC-4.2** [AT-11] If effective startup hooks, MCP, plugins or managed configuration change, then trust admission shall re-evaluate before the affected record can admit again.
- **AC-4.3** [§18.1] Each `blocked` record shall name the next test that could advance it and the authority that test needs.

### FR-5 — Registry surface for admission and users (§§7.1, 14, 17–18, I09, G02)

- **AC-5.1** [§7.1] Admission shall resolve eligibility from registry records through a versioned internal interface; no caller shall infer qualification from documentation, sign-in state or user declaration.
- **AC-5.2** [I09] The registry shall label every datum `reported | observed | estimated | user-declared | unknown` with unit, scope, source and timestamp; missing data shall read `unknown`, never `0`, passed or verified.
- **AC-5.3** [§17] Subject to Q4, the `doctor` section shall show each record's progress, columns, evidence revisions and drift triggers in plain and JSONL output.

## Non-Functional Requirements

- **NFR-1** [§17.2] Registry reads during admission shall add less than 1 second to admission latency on the maintainer's Linux machine. (No maintainer area budget covers registry reads; the 5-minute figure in `orchestration/RUN-LOG.md` is MH-20's lint-CI budget.)
- **NFR-2** [I20] Record revisions shall be content-addressed or monotonically versioned so two readers of the same revision always see the same record.

## Definition of Done

- [ ] AC-1.1 to AC-5.3 each have a named test that fails before the change and passes after it.
- [ ] The first included-only route record carries authorised-test evidence via `RecordDraft` and its tests; production databases stay `blocked`/`planned` and no other route is labelled qualified.
- [ ] `scripts/harness/gate.sh go` and `gate.sh harness` pass; CI green on Linux, macOS and Windows.
- [ ] MH-10 synced to `specced` (this step), then implemented, finalized and PM-synced to `shipped`.

## Open Questions

- **Q1**: Which harness supplies the first included-only route — Claude Code (existing probe, MH-12 follows) or another v2 §7.2 candidate? Blocks FR-3 scoping; default: Claude Code print/stream-json surface, since MH-12 builds on it.
- **Q2**: What authorised live tests may run for FR-3, and whose allowance permission covers them? Blocks AC-3.1 evidence; default: fixtures only until the maintainer grants explicit live-test permission.
- **Q3**: Where do records live — SQLite state (ADR-0003), versioned files, or both? Blocks FR-1 design; default: SQLite with content revisions, per ADR-0003.
- **Q4**: Does the registry surface ship as a `mythhelm` subcommand or a `doctor` section first? Blocks AC-5.3 CLI shape; default: `doctor` section reusing its truthful-output contract.

## Dependencies

- Prerequisite: none outside this spec; dogfood-slice (done) supplies the probe and blocking admission this registry builds on.
- Conflicting: none in `todo/` or `in-progress/` (both empty).
- Superseded: `adapters/claudecode/COMPATIBILITY.md` as the sole compatibility record — absorbed into the registry, kept as a per-harness view if still useful.
- Downstream: MH-12 (strict Claude Code route) and MH-22 (one-agent workflow) consume this registry; both sequence behind it.

## Impacted components

- New: `internal/qualify/` (registry store, record types, drift detection) — exact package shape left to `/spec`.
- Touched: `internal/admission/` (resolve eligibility from records: `admission.go`, `billing.go`), `internal/cli/` (user-facing surface: `doctor.go` or new subcommand), `adapters/claudecode/` (first-route evidence: `probe.go`, `COMPATIBILITY.md`).
- Tests: `tests/` golden/acceptance additions per AC; no live credentials required (I13).
