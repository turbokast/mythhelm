## Claude Strict Subscription — Requirements

> Entitlement and overage-prevention evidence that lets strict subscription-only admission pass for Claude Code instead of blocking: effective configuration, all auxiliary/child routes and paid-continuation prevention proven against the MH-10 registry. A slice of v2 §§7–8. Normative source: mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md, version 2.0; § numbers, I-IDs, G-IDs and AT-IDs refer to it.

## Context

- **Backlog card**: MH-12
- **Issue**: [#33](https://github.com/turbokast/mythhelm/issues/33) (card discussion; card holds status, stage, score)

Strict `--billing subscription-only` blocks for Claude Code today, twice over: `Decide` early-returns `entitlement_qualification_unavailable` before dispatch (`internal/admission/admission.go:189-191`), and `ResolveBilling` blocks the same way (`internal/admission/billing.go:34-36`). The dogfood slice ships only the user-declared, never-verified posture (its non-goal N8): `Qualified:false`, `paid_continuation:unknown`, `g05:not-passed` (`internal/admission/billing.go:52`; ADR-0002 `docs/decisions/0002-dogfood-billing-posture.md`). This card earns the strict one (I15): the MH-10 registry supplies the versioned records, and this spec proves the entitlement column for the Claude Code first route — effective configuration established, every model-using auxiliary and descendant covered (v2 §7.3), paid continuation prevented through provider/native enforcement — then removes the early return so a proven record admits.

Two load-bearing limits shape the work. First, the effective-configuration source gap is open: ADR-0002 records that remote cached managed policy and macOS MDM preferences are not certified by the file inventory, needing architect/maintainer resolution before real qualification. Until that closes, no record can claim complete effective configuration. Second, live qualification needs explicit allowance permission with disposable fixtures (v2 §7.1); the card authorises no live usage by itself, so the authorised-live evidence needs a maintainer grant at implementation time.

Grounding verdicts (2026-10-07):

- `strict subscription-only blocks for Claude Code` HOLDS (`internal/admission/admission.go:189-191`, `internal/admission/billing.go:34-36`).
- `dogfood ships only user-declared, never-verified posture (N8)` HOLDS (dogfood-slice requirements N8; `internal/admission/billing.go:52`; ADR-0002).
- `MH-10 supplies versioned records` PARTIAL: the registry is specified and validated (`specs/*/qualification-registry/`, in `in-progress/`, approved) and under implementation; this spec sequences behind it.
- `issue #33 section references (§13.8–§13.10, §18.9, §20.3, gates at §18.7)` PARTIAL: stale numbering; the card's v2 §§7–8 govern.
- `Claude need not be the first route` HOLDS as card statement; consistent with v2 §7.2 (no preferred winner).
- `effective-configuration source gap open` HOLDS (ADR-0002 lines 80–85).

### Objectives

- **O1**: Strict subscription-only admission passes for Claude Code when — and only when — the registry holds a `live-qualified` record with the entitlement column `proven` for the exact probed route.
- **O2**: Every model-using auxiliary, descendant and plugin route of that execution is covered by the entitlement evidence; a hidden paid route keeps blocking.
- **O3**: The existing user-declared dogfood path still admits exactly as before, still labelled unverified.

### Non-Goals (and what still binds)

| # | Out of scope | Still binding in this spec |
|---|---|---|
| N1 | Qualifying any other harness (MH-4 owns Codex; the registry holds the rest as non-live). | I14: no other route's label changes; only the Claude Code first-route record can advance. |
| N2 | `metered-allowed` mode (v2 §7.3 optional). | I10: no hard limit is advertised; stop-at-exhaustion only where exhaustion reliably waits/stops. |
| N3 | Adversarial no-egress/no-spend guarantees (v2 §7.4). | v2 §7.4: `trusted-host` stays honestly non-contained; the claim is included-only billing, not sandboxing. |
| N4 | Closing the managed-policy source gap by code alone if the maintainer has not resolved it. | I02: unknown mandatory effective-configuration evidence blocks; the record stays non-live until the gap closes. |

## Functional Requirements

Acceptance criteria use EARS. Tags in brackets name the invariants, gates and sections each one serves.

### FR-1 — Effective configuration proof (§7.3, I02, I15, G02, G07, AT-11)

- **AC-1.1** [§7.3] When the native credential precedence, full effective managed policy, children routes and extra-usage settings are established through supported non-secret status/configuration for the exact probed binary and account class, the system shall record them as the route's effective-configuration evidence.
- **AC-1.2** [AT-11] If any effective startup hook, MCP server, plugin or managed-configuration source is uninventoried or its digest differs from the trusted pin, then strict admission shall keep blocking with a reason naming the source.
- **AC-1.3** [I02] When a mandatory effective-configuration source from the v2 §7.2 Claude Code focus list is unknown and the maintainer has not resolved the source gap, the entitlement column shall stay `not-proven` and strict admission shall keep blocking.

### FR-2 — Full auxiliary and descendant coverage (§7.3, I15, G05, AT-03)

- **AC-2.1** [AT-03] When authorised tests establish included-only entitlement across all implementer, planner, reviewer, summary, router, child and experiment routes with no hidden paid auxiliary, the system shall record the entitlement column `proven` with the authorising evidence.
- **AC-2.2** [§7.3] If a model-using plugin, MCP tool route or native child is unqualified or its funding is unknown, then the entitlement column shall stay `not-proven` and strict admission shall keep blocking.
- **AC-2.3** [§7.3] When existing purchased credits could still be consumed despite disabled automatic purchases, the system shall treat paid continuation as unprevented and keep blocking.

### FR-3 — Strict admission pass on proof (§7.3, I15, G05, AT-04)

- **AC-3.1** [AT-04] When the registry holds a `live-qualified` record with the entitlement column `proven` matching on MH-10's full v2 §7.1 qualification key (including binary digest, config digest, account class), strict `--billing subscription-only` shall admit instead of blocking.
- **AC-3.2** [AT-04] If entitlement or no-overage evidence rests only on subscription sign-in or user declaration, then strict admission shall keep blocking with `no_qualification_record` (no record for the key) or `entitlement_not_proven` (record without proven entitlement).
- **AC-3.3** [§7.3] When the probed binary or configuration drifts from the proven record's pins, the system shall invalidate the affected evidence and strict admission shall block again until re-tested.

### FR-4 — Declared path preserved (§7.3, I15, G05)

- **AC-4.1** [§7.3] The existing `subscription-declared` path shall admit exactly as before this spec, still recording `qualified:false`, `paid_continuation:unknown` and `g05:not-passed`.
- **AC-4.2** [I15] No output of this spec shall label the declared posture as qualified, verified or strict; the strict and declared labels shall stay distinct in every surface that carries billing labels (admission JSONL output, journal `runs.billing_posture` rows, receipts, `runs` command output).

## Non-Functional Requirements

- **NFR-1** [§17.2] The strict-admission evidence check shall complete within the admission budget (registry lookup under 1 s per MH-10 NFR-1; no live probe beyond the existing bounded `auth status` and version queries).
- **NFR-2** [I19] No step of this spec shall read secret contents, extract subscription secrets, or alter account settings; auth evidence stays non-secret status fields only.

## Definition of Done

- [ ] AC-1.1 to AC-4.2 each have a named test that fails before the change and passes after it.
- [ ] Strict subscription-only admits the proven Claude Code route and still blocks every unproven variant, each with its typed reason.
- [ ] `scripts/harness/gate.sh go` and `gate.sh harness` pass; CI green on Linux, macOS and Windows.
- [ ] MH-12 synced to `specced` (this step), then implemented after MH-10 ships, finalized and PM-synced to `shipped`.

## Open Questions

- **Q1**: Does the maintainer resolve the managed-policy source gap (ADR-0002 lines 80–85) before this spec builds, and how — extended inventory, documented limitation, or platform scoping? Blocks FR-1; default: gap stays open (the sources stay in `UnresolvedSources`), FR-1 builds the complete inventory it can and the record stays non-live until resolved. A resolution lands as an implementation PR editing `UnresolvedSources` with inventory code or a scoping rationale amending ADR-0002; admit-path acceptance sequences behind it (D8). That PR rewrites the pre-Q1 gap pins it invalidates to their post-Q1 codes, and Tasks 5/7 assume its edit leaves no applicable gap on the test platform. The assumption must hold on all three CI OSes (DoD runs everywhere): a partial resolution that leaves a platform-specific gap (e.g. `macos-mdm-policy` on darwin) must platform-gate Task 5's e2e, not just rewrite pins.
- **Q2**: What authorised live tests may run (allowance permission, disposable fixtures, account class), and who grants them? Blocks FR-2/FR-3 evidence; default: no live tests until an explicit grant; fixtures and unit coverage only.
- **Q3**: Is the first route still Claude Code (MH-10 Q1 default), or did MH-10 prove a different harness first? Blocks FR-1 scoping; default: Claude Code print/stream-json, per MH-10's default.
- **Q4**: Which exact `auth status` fields and vendor conditions constitute the entitlement proof for the Claude account classes in play? Blocks AC-1.1/AC-2.1; default: the v2 §7.2 Claude Code focus list (credential precedence, managed policy, children, extra usage) with current native docs cited at /spec time.

## Dependencies

- Prerequisite: `specs/*/qualification-registry/` (MH-10, in `in-progress/`, validated, approved) — this spec's criteria bind to its records and sequences behind its ship; building starts only after MH-10 is done.
- Prerequisite decision: ADR-0002 source-gap resolution (maintainer/architect) for any live claim.
- Conflicting: none; `in-progress/` holds only `qualification-registry` (sequenced as the prerequisite above) and `todo/` is empty.
- Downstream: MH-22 consumes the qualified route.

## Impacted components

- Touched: `internal/admission/` (remove/condition the `Decide` early return: `admission.go:189-191`; extend `ResolveBilling` in `billing.go`; consume MH-10's qualification resolution in `internal/qualify/` once it ships), `adapters/claudecode/` (effective-config inventory and evidence: `probe.go`, `settings.go`, new qualification evidence), `internal/qualify/` (MH-10's registry, consumed read/write).
- Tests: adapter fixtures plus authorised-live tests behind the `live` build tag only (never in CI); no live credentials in contributor tests (I13).
