# Integrated product adoption

This change adopts [Master Specification v2](../../mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md) as the design for new product work, through the [specification index](README.md). Adoption takes effect when the exact product changes are signed by the maintainer and the reviewed PR merges. Until then this branch is a proposal. It does not implement the application, qualify native services, advance the current delivery stage or authorise live experiments, account changes or deployment.

The three synthesis deliverables remain [master](../../mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md), [decisions](../../mythhelm-synthesis/MYTHHELM_Synthesis_Decisions.md) and [implementation plan](../../mythhelm-synthesis/MYTHHELM_Implementation_Plan.md). The original master, proposal and prompt remain byte-identical. The synthesis's original repository snapshot was `d77f2e8`; this adoption was checked against `eb349c2`, including MH-9's merged shipped status.

## What changes in the product layer

- [Objectives](../../product/objectives.md) derive S0–S6, G01–G16 and ten commitments from v2. Current stage remains 1 as a delivery focus, with outstanding S0 qualification explicitly blocking unsupported claims.
- [Backlog](../../product/backlog.md) preserves the three shipped cards, updates all 17 existing open cards and adds MH-21–MH-30. Each new card has grounded premises, score rationale and prerequisites. Missing public issue links are filled through a second signed sync after approved cards are committed, as the backlog procedure requires.
- [Decisions](../../product/decisions.md) D-26–D-39 record adoption, reconciliation, reprioritisation, the ten additions and the treatment of earlier signals. Earlier entries remain unchanged; D-28 gives each changed score.
- [Roadmap](../../product/roadmap.md) is regenerated from the same staged backlog. Its ranked candidate list is not a claim that prerequisites or release gates passed.
- Agent entrypoints and authoring guidance resolve the specification index. Historical Revision 1.1 specs, ADRs, closed cards and audit IDs keep their original meanings. `docs-site-demos` remains specced; review its v2 reconciliation before implementation resumes.

No existing card is marked shipped merely because its requirements appear in v2. Existing skills and signals do not mandate cheap-first models, kNN, keepalive calls, secret copying or identity rotation. Those suggestions either become bounded hypotheses or are excluded by the v2 contracts.

## Implementation-plan coverage and prerequisites

These are owners of planned work, not completion claims. A broad card becomes reviewed lifecycle specs before implementation. W01 includes this governance adoption, but its schemas, types and executable contracts remain work in MH-21/MH-10.

| Slice | Backlog owners | Prerequisite and delivery boundary |
|---|---|---|
| W01 contracts and adoption | MH-21, MH-10 | This reviewed adoption establishes the design; exact schema/event/config contracts and evidence records still need implementation specs. |
| W02 first included native route | MH-10, MH-12, MH-4, MH-13; MH-29 per candidate | W01; independently qualified fidelity, entitlement, stop and startup trust. Choose the first credible route by evidence, not vendor order. Live tests need separate authority. |
| W03 durable supervisor and migration | MH-21, MH-11, MH-16 | W01; may develop against fake adapters before live qualification. One ledger writer, worker-owned children, explicit v1 migration/restore and supported-platform proof. |
| W04 complete single-agent verified change | MH-22, MH-13, MH-16 | W02/W03; one qualified route, finite envelopes, protected exact-revision checks and truthful candidate/apply receipt. |
| W05 standalone and Herdr completion | MH-3, MH-23, MH-8 | W04; typed controls, input ownership, detach/recovery, accessibility and safe declarative themes. Public executable plugins are not an S1 dependency. |
| W06 second harness and handoff | MH-4, MH-12 or a MH-29 target; MH-5, MH-15, MH-17 | S1 and two independent route qualifications. Native resume and reconstructed cross-harness handoff remain distinct. |
| W07 scoped context and dependencies | MH-24 | W06; original requirements/artifacts, selective retrieval, immutable contracts, interface checkpoints and stale-evidence invalidation. |
| W08 two-writer construction/integration | MH-14, MH-16 | W07; two isolated mutable environments, bounded native children, reconciled reservations and exact combined checks. |
| W09 public extensions | MH-6 | W07 and two built-in interface consumers; W08 only for parallel proposals. Bounded protocol, SDK, trust/revocation and useful examples. |
| W10 evidence and fixed portfolio | MH-26, MH-5 | W07/W13; W08 only for P3. Essential operational receipts stay available with learning off; optional data requires scoped consent. |
| W11 bounded experiments | MH-27, MH-16 | W10, qualified variants and explicit experiment authority; W08 only for parallel variants. Protected evaluators, reserves, all-started outcomes and uncertainty. |
| W12 promotion and new models | MH-28, MH-10, route-specific cards | W11, scope-specific qualification/canary and promotion authority. Active attempts stay pinned; whole pipelines may lose to one better model. |
| W13 export, erasure and restoration | MH-25 | W03/W07; S2 public portability must precede W10 retention/deletion claims. Rebuild append-only storage only through validated quiescent maintenance. |
| W14 native/platform coverage | MH-10, MH-11, MH-12, MH-4, MH-29; MH-3/MH-23 for hosts/terminals | Independent per-target tracks, with required S1 rows first. Preserve all seven targets and Windows/macOS/Linux without claiming untested combinations. |
| W15 optional verified deployment | MH-30 | S1 and destination-specific authority/qualification. Independent of W10–W12; missing authority preserves a release-ready candidate. |
| W16 public releases and maintenance | MH-7, MH-8, MH-18, MH-19, MH-20 | Recurs for each supported subset including S1. MH-9's shipped badge remains historical evidence; actual release/tag/settings actions remain operator-controlled. |

The first complete useful product needs W01–W05, its applicable W14 support rows and W16 evidence. S2 adds W06/W07/W13; S3 adds W08/W09. S4 data and experiments do not require parallel construction unless using P3. S5 requires demonstrated, reversible scoped improvement. S6 is an optional independent branch from S1. None of these stages automatically constitutes a stable version 1.0 release.

## Explicit-requirement coverage

| Requirement | Product owners and retained outcome |
|---|---|
| UR-01 free/open-source | Objectives C1–C4; MH-7/MH-8/MH-18–MH-20; free offline contribution and no MYTHHELM account or paid feature tier. |
| UR-02 native fidelity | MH-10/MH-12/MH-4/MH-29; chosen harness stays chosen, with independent surface evidence. |
| UR-03 included allowance | MH-10/MH-12/MH-4/MH-13/MH-16/MH-27; all child/auxiliary roles covered, no silent metered or purchased-credit continuation. |
| UR-04 Herdr and standalone | MH-3/MH-21/MH-23; one lifecycle owner, safe input lease and truthful reconnect. |
| UR-05 seven native targets | MH-10 registry; MH-12 Claude Code; MH-4 Codex; MH-29 OpenCode, Meta Muse Code, Kimi Code CLI, Cursor Agent CLI and Antigravity. |
| UR-06 polished accessible cross-platform UI | MH-23/MH-3/MH-11/MH-29; actual keyboard/human/platform evidence, qualified subset releases. |
| UR-07 plugins/mods/protocol | MH-23 basic themes; MH-6 public protocol/SDK/mods, credential-free examples and contribution. |
| UR-08 reliable continuity and delivery | MH-21/MH-22/MH-24/MH-25/MH-14/MH-15/MH-16/MH-17; durable contracts, finite autonomy, original context, isolated work and protected acceptance. |
| UR-09 substantive adaptation | MH-26/MH-27/MH-28 with MH-5; routes, context, prompts, review and decomposition evaluated against stronger simpler baselines. |
| UR-10 coherent incremental delivery | Objectives and MH-21/MH-22 distinguish the full destination from S1; W dependencies above prevent later research becoming a prerequisite for one useful route. |

All invariants I01–I25 remain binding on shipped scope. G01–G16 are indexed in objectives and attached to relevant cards; AT-01–AT-48 remain normative acceptance requirements in v2, not claimed results of this documentation task.

## Grounded implementation baseline

| Evidence at eb349c2 | Consequence for adoption |
|---|---|
| `internal/admission/billing.go` resolves `subscription-only` to a blocker; declared subscription posture is unqualified. | No native route is advertised as v2 included-only qualified. The first paid-continuation boundary is still an external evidence question. |
| `internal/adapter/` contains Claude Code and fake seams; no Codex/other-five adapters. | Retain the useful existing seam; keep separate route qualification cards and explicit blockers. |
| `internal/supervisor/`, ADR 0005 and the current CLI use run-scoped ownership. | MH-21 must deliver the per-user supervisor migration and safe detach; changing documentation does not change runtime ownership. |
| `internal/journal/migrations/0001_init.sql` records runs/attempts/candidates/checks but has no v2 TaskRevision, policy or experiment tables. | Add migrations with backup/restore and original evidence preservation; never relabel old runs as qualified or accepted. |
| `internal/cli/dispatch.go` has run/runs/review/stop/recover/apply/version/demo/doctor; no context/learning/deployment commands. | Context, optional learning, portability and production effects are planned application capabilities. |
| `hosts/`, `protocol/`, `sdk/` and `evals/` do not exist at this baseline. | Herdr bridge, public SDK and evaluation runner need complete slices, not claims of integration. |
| `specs/done/tui-slice/retrospective.md`, README, issues #106/#107 describe incomplete exit/detach and limited interactive evidence. | MH-2 remains shipped as its original slice; MH-23 covers remaining v2 journeys and human accessibility proof. |
| MH-1, MH-2 and MH-9 are shipped; MH-8 is specced. | Preserve those lifecycle facts. Adoption does not advance app tasks or the delivery-run state. |

The synthesis researched public documentation and inspected source. No live-native spending, paid experiment, account operation, deployment or product benchmark was performed for adoption. Existing release, autonomy and signed-product boundaries remain in force.

## Decisions requiring later evidence

1. **First usable included-only route.** Native effective configuration, allowance exhaustion, purchased-credit prevention, auxiliary calls and model aliases need exact-version qualification. Unknown entitlement blocks; unknown remaining quota alone is different.
2. **Lifecycle and platform enforcement.** Migration, worker adoption, DB/disk failure, Windows process trees, native children and same-user trust boundaries require fault and platform tests.
3. **Herdr and accessibility.** Installed host schemas, input takeover, terminal combinations and real screen-reader journeys need supported-environment evidence.
4. **Learning benefit.** Data volume, usable outcome signals and whole-pipeline improvement are unproven. Keep P1 useful; permit inconclusive experiments and rejection of all challengers.
5. **Scope sizing.** MH-21/MH-28/MH-29 are broad cards that must be decomposed into reviewed specs before implementation. Ordinal score does not override these prerequisites or authorise experiments.

## Adoption checks

The staged product set must pass `pm.py validate` with a freshly generated roadmap. Review checks all W01–W16 and UR-01–UR-10 ownership, all seven target names, all 16 gate references, unchanged closed cards and historical log prefixes, preserved source hashes, and current-reference resolution. Harness tests exercise stages 4–6, new gate IDs, invalid stage inputs, safe prose drafting and the prohibition on direct product writes. Required harness/hygiene gates and independent architecture/code review precede merge. Native and runtime acceptance remain planned work.
