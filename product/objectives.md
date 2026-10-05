# Objectives

What MYTHHELM is trying to achieve, derived from [Master Specification v2](../mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md): stages and gates in §18, explicit requirements in §1 and invariants in §2. The [specification index](../docs/spec/README.md) distinguishes the current design from preserved historical sources. The spec is normative; this file is the index the backlog scores against. Changing it requires a signed maintainer decision.

> Current stage: 1

This is the current delivery focus, not a claim that every earlier boundary proof passed. S0 native fidelity, entitlement and lifecycle obligations still block any advertised capability lacking evidence. The existing dogfood and TUI slices do not establish the complete S1 product. No stage advances in this adoption.

The current-stage line is read by `scripts/pm/pm.py`; card urgency follows its distance from that stage. A later stage advance is an `objective-change` decision paired with a tool-generated rescore. Stage numbers express dependencies rather than calendar promises: optional S6 delivery can proceed after its S1 prerequisites without waiting for S4/S5 learning.

## Stages

| Stage | Name | Complete outcome | Exit gate |
|---|---|---|---|
| 0 | Boundary proof | One viable native included-only route, fidelity/lifecycle/trust evidence, v2 contracts and a safe migration design. The first route is chosen by qualification, not brand. | AT-02–AT-04, AT-11 and AT-47 for the proposed route; remaining uncertainty is explicit and blocks affected claims. |
| 1 | First useful product | One qualified native route; one durable supervisor and worker-owned execution; isolated single-agent work, protected checks, review/apply/receipt; polished standalone and Herdr use, plain/JSONL/accessibility and declarative themes. | Applicable G01–G11, G16 and G12 for any enabled alternative surface. The complete task-to-verified-candidate journey works without manual state repair. |
| 2 | Continuity | A second independently qualified harness; native session recovery and grounded handoffs; scoped retrieval, task/dependency revisions, export and erasure. Seven-harness qualification continues independently. | G02/G05 for each added route, G13/G16, plus inherited safety gates; AT-14–AT-18 and AT-43 pass. |
| 3 | Bounded construction and extensions | Two isolated writers under stable contracts; global resource/child accounting, serialized integration and bounded repair; versioned process plugins and broader declarative mods. | G04–G08 for enabled features, AT-19–AT-21, AT-26 and AT-38/39; combined correctness and useful user outcomes are measured. |
| 4 | Evaluated portfolio | P1–P4 fixed policies, complete outcome accounting, consented observations, protected bounded experiments and recommendations with uncertainty. P3 requires S3. | G14 data/experiment cases AT-25–AT-30/33; no promotion from development-only wins, hidden exploration or weakened acceptance. |
| 5 | Adaptive product | Scoped policy/prompt promotion, rollback, new-model qualification/calibration/canaries and whole-pipeline replacement, including a simpler single agent. | Full G14: AT-31/32/34 plus a demonstrated useful scoped live improvement against the strongest relevant simpler baseline. Inconclusive evidence keeps fixed behavior and an experimental label. |
| 6 | Optional verified production delivery | Explicitly authorised exact-build deployment, independent destination health checks, effect reconciliation and bounded rollback. Disabled by default; independent of learning. | G15 per destination, including AT-09/44/45. Missing authority yields a preserved release-ready candidate and a blocked deployment request. |
| later | Separately justified mechanisms | Wider parallelism, general combinatorial optimization, optional embeddings, host-owned execution, distributed scheduling and sandboxed plugin marketplaces. | A separate spec demonstrates need, qualified authority/security/billing and benefit against the simpler design. These mechanisms are not prerequisites for S5 adaptation. |

Version 1.0 is a separate maintainer release decision for an honestly advertised subset, not another stage. All applicable gates must pass; adaptive claims additionally require S5 evidence. Qualifying all seven harnesses is the full target, not a prerequisite for shipping one useful route.

## Release gates

The authoritative acceptance cases and scope are v2 §18.2–§18.3. A card's gate list names the gates it advances, not gates it has already passed.

| Gate | Pass condition | First applies |
|---|---|---|
| G01 | Free build, offline demo and normal contributor tests without MYTHHELM account or paid credentials. | S1 |
| G02 | Exact native harness/surface fidelity, documented deltas and appropriate fixture/live continuity evidence. | S0 qualification; each advertised route |
| G03 | Source files/index/refs protected; apply and verification reject stale state. | S1 |
| G04 | One owner, durable launch, bounded stop/recovery, children and supported-platform lifecycle evidence. | S1; every enabled expansion |
| G05 | Included-only admission across all model-using roles/children; no bought-credit/overage continuation; honest quota evidence. | S0 qualification; every route |
| G06 | Protected checks at the exact accepted integration revision; stale and unverified outputs cannot self-certify. | S1; parallel cases at S3 |
| G07 | Startup trust, exact grants, scoped data access and substantiated enforcement boundaries; learner cannot widen authority. | S0 qualification; relevant features thereafter |
| G08 | Declarative mods cannot execute code/hide controls; process plugins have explicit trust, pins and conformance. | S1 themes; S3 executable plugins |
| G09 | Honest CLI/JSONL results, keyboard/accessibility/resize/streaming and complete user journeys. | S1 per supported environment |
| G10 | Public licensing, security, provenance, contribution and precise compatibility/limitation evidence. | Every public release |
| G11 | Herdr remains a scoped client; input ownership, detach/restart and no-duplicate-launch evidence. | S1 |
| G12 | Each alternative SDK, interactive or host-owned surface independently passes fidelity, billing, lifecycle and security gates. | Whenever enabled |
| G13 | Grounded handoffs, session loss, mandatory context, ACLs, search degradation, stale dependency invalidation and scoped portability. | S2 |
| G14 | Comparable outcome data, protected experiments, evidence-gated scoped promotion, rollback and simpler-pipeline selection. | S4 portions; full adaptive claims at S5 |
| G15 | Authorised deployment, exact-build independent health, uncertain-effect reconciliation and scoped rollback. | Optional S6 |
| G16 | Safe v1/v2 migration, single writer, compatible backup/restore, scoped export and honest erasure. | S1 migration; S2 portability |

## Commitments

- **C1 — Free complete core (v2 §1, UR-01).** Official core, adapters, safety controls, TUI, headless operation, official mods, SDK and local routing have no paid tier or required MYTHHELM account.
- **C2 — No mandatory paid or hosted service (v2 §§1,8,17; I13).** Offline demo and normal contributor tests need no paid credentials, custom font, central router, catalogue or telemetry service.
- **C3 — Included allowance by default (v2 §7; I15).** Metered inference, purchased credits, paid overages and auxiliaries never appear silently; optional metered profiles require explicit consent and are never fallback routes.
- **C4 — No capability or preference for sale (v2 §1).** Donations and sponsorship cannot unlock official features or influence routing preference; third-party costs remain explicit.
- **C5 — Local-first operational state (v2 §§5,8.4,17).** Essential receipts and recovery evidence exist while executing. Optional learning, backfill, exploration and external sharing are separately controlled; there is no central telemetry requirement.
- **C6 — Honest capability and performance claims (v2 §§7,13,18; I09/I14).** Unqualified surfaces stay blocked/experimental, unknown stays unknown, and accepted useful outcomes are compared with direct-native and strong simpler baselines.
- **C7 — Native identity and complete support vision (v2 §§1,7; I01/I16/I19).** Preserve the actual chosen harness, native-owned authentication and useful features. Retain Claude Code, Codex, OpenCode, Meta Muse Code, Kimi Code CLI, Cursor Agent CLI and Antigravity as independently qualified targets.
- **C8 — First-class control and accessibility (v2 §§14–17).** Standalone and Herdr journeys share one lifecycle owner and input lease; keyboard, plain/machine output, accessibility and Windows/macOS/Linux remain product commitments with honest evidence matrices.
- **C9 — Substantive bounded adaptation (v2 §§9–13; I20–I25).** Grounded continuity and a small policy portfolio can improve routing, context, prompts, review and decomposition. Learning may select a simpler workflow, cannot expand authority, and remains optional.
- **C10 — Verified outcomes within authority (v2 §§8,11).** Protected exact-revision acceptance precedes success. Standing grants enable decisive work; applying, publishing and deployment remain distinct effects, with optional production delivery requiring independent health evidence.
