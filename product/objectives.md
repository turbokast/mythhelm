# Objectives

What MYTHHELM is trying to achieve, derived from the [master spec](../docs/spec/master-spec.md): the delivery stages of §20, the release gates G01–G12 of §18.7 and the open-source commitments of §3. The spec is normative; this file is the index the backlog scores against, and it defers to the spec on any difference. Changing it is a maintainer decision (see [README.md](README.md)).

> Current stage: 1

The line above is read by `scripts/pm/pm.py`: a card's urgency depends on how far its stage lies beyond the current stage. Advancing the stage is an `objective-change` decision, and it is filed together with the rescored backlog that `pm.py rescore` drafts.

## Stages

The spec replaces calendar dates with complete vertical slices, each closed by an exit gate (§20.1). A stage is done when its exit gate passes for everything the project advertises, not when its deliverables exist.

| Stage | Name | Delivers (summary) | Exit gate |
|---|---|---|---|
| 0 | Prove the uncertain boundaries (§20.2) | A native-integration and process-ownership spike: headless paths of the candidate harnesses against disposable fixtures, fidelity, entitlement and overage records, Herdr launch and restore, worker survival and stop on Windows and Unix, a small TUI fixture. | The initial adapter surface is chosen from measured compatibility, authentication constraints are resolved, and every capability claim the spike cannot justify is removed. |
| 1 | A complete single-agent product (§20.3) | Installable binary, offline demo, read-only doctor, one qualified native adapter with included-allowance and overage evidence, one writer, managed snapshot, local routing, durable run and worker ownership, candidate diff, configured verification, receipt, stop, detach and recovery, a focused TUI, plain and JSONL output, one declarative theme format, the single-pane Herdr experience. | G01, G03–G07, G09 and G11 for advertised combinations; G02 for the shipped native surface; G10 for the public preview; G12 for any enabled alternative surface. The task → native work → checks → reviewable artifact loop works without manual repair. |
| 2 | Multi-harness public alpha (§20.4) | At least two independently qualified native adapters, authorised profiles, deterministic selection, a dependency-aware task DAG, two-writer mode, messages and handoffs, resource reservations, the serialized integration train, explicit quota and cost uncertainty, public plugin and host-bridge protocols, trusted executable plugins, declarative workflow, keymap and layout mods, Herdr lane inspectors. | Parallel and cross-provider tasks outperform or simplify representative workflows without losing source safety, recovery or billing honesty. G08 joins the core gates. |
| 3 | Version 1.0 (§20.5) | Published host, adapter and terminal support matrix; stable configuration and plugin protocol; accessible linear interface; native version management; migration and update path; optional GitHub publisher; local evaluation and routing history; storage management; documented limitations and governance. | All applicable G01–G12 requirements pass, including Herdr and per-surface entitlement evidence; the provider compliance review is recorded; positioning has survived user testing. |
| later | Justified by evidence (§20.6) | Measured local learning, funded remote routing, qualified SDK-runtime alternatives, richer native controls, sandboxed computational plugins, advanced UI mods, remote execution. | Each needs its own evidence; team-shared state, cloud offload and multi-tenant hosting need separate threat models. |

Scope cuts that protect delivery (§20.7) bind every stage: native fidelity, included-allowance admission, correct cancellation, source protection, permission clarity, honest cost uncertainty, a reviewable artifact, a usable TUI and first-class Herdr behaviour are never cut for the advertised topology.

## Release gates

The acceptance matrix of §18.7. A card names the gates it moves toward passing.

| Gate | Pass condition (summary of §18.7) | First required at |
|---|---|---|
| G01 | Free baseline: install, build, offline demo and contributor tests need no account, paid credentials or cloud service. | Stage 1 |
| G02 | Native preservation: each advertised harness surface has a fidelity record, a direct-native comparison and evidence; no hidden substitution. | Stage 1 (shipped surface) |
| G03 | Source protection: default runs never modify the original checkout; dirty state and apply races are handled. | Stage 1 |
| G04 | Lifecycle: detach, crash, orphan, cancellation and resume tests pass on every advertised platform. | Stage 1 |
| G05 | Billing honesty: strict subscription-only admits only qualified included allowance with overage prevention. | Stage 1 |
| G06 | Integration correctness: frozen outputs are combined and checked at the final revision. | Stage 1 |
| G07 | Trust enforcement: approval binding and declared sandbox boundaries pass adversarial tests. | Stage 1 |
| G08 | Mod safety: declarative mods cannot execute code or hide required controls; executable-plugin trust is explicit. | Stage 2 |
| G09 | Terminal usability: the supported terminal and shell matrix, plain mode, resize and keyboard flows have recorded results. | Stage 1 |
| G10 | Public release: licence, security policy, contribution path, provenance, changelog and limitations are published. | Stage 1 (public preview) |
| G11 | Herdr: embedded behaviour, scoped state, input ownership, detach, restore and no-duplicate-launch tests pass. | Stage 1 |
| G12 | Alternative surfaces: any enabled SDK-runtime or alternative surface passes its own gates. | Stage 1 (when enabled) |

## Commitments

The open-source promise of §3. A card that would break one is dropped, not scored.

- **C1 — Free, complete core (§3.3).** The core, built-in adapters, safety controls, TUI, headless mode, official themes, plugin SDK and local routing carry no licence fee, paid tier, feature gate or required account.
- **C2 — No paid service required (§3.3, I13).** No paid router, hosted registry, analytics service or cloud coordination; the offline demo and the contributor test suite need no paid credentials.
- **C3 — Included allowance by default (§3.3, I15).** The default workflow never requires separate inference payments or paid overages; a metered profile exists only by explicit consent and is never a fallback.
- **C4 — No capability for sale (§3.3).** Donations and sponsorship never unlock capabilities or influence routing defaults; no affiliate-biased routing.
- **C5 — Local-first, one trusted user (§3.4, §17.2).** The security model is one operating-system user supervising authorised native tools. Metrics are collected locally and only when enabled; there is no central telemetry, so product evidence comes from GitHub issues and discussions, CI and evaluation runs.
- **C6 — Honest claims (§3.5, §22.3, I14).** An unimplemented feature stays marked unimplemented and an unqualified one unverified; one native agent plus verification is the baseline to beat.
