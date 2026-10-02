# Backlog

Every planned piece of work larger than a fix, as one card, scored and ordered. The backlog is the plan of record; [roadmap.md](roadmap.md) is a view generated from it. Each card has a public GitHub issue, which is where contributors discuss it. Who may change this file, and how, is in [README.md](README.md).

## Schema

A card is a level-3 heading and a fixed list of fields, in this order:

```markdown
### MH-<n>: <title>
- **Status**: idea | triaged | specced | implementing | shipped | dropped
- **Stage**: 0 | 1 | 2 | 3 | later
- **Gates**: G01, G03 (release gates of spec §18.7 the card moves toward passing), or none
- **Score**: <s> = (value V + urgency U + risk R) / effort E
- **Spec**: `spec-name` (or several, comma-separated), or (none)
- **Issue**: [#<n>](https://github.com/turbokast/mythhelm/issues/<n>), or (none)
- **Source**: the master-spec sections and backlog items that motivate the card
- **Summary**: what the card delivers and why, in one to three sentences
```

Optional fields, after the required ones: **Premise-grounded** (the verdicts recorded before a spec is created from the card), **Half shipped** (a card whose specs ship separately) and **Notes**.

- **Ids.** `MH-<n>` is assigned when the card is filed, by `python3 scripts/pm/pm.py next-id MH`, which reserves it across sessions and branches. A draft says `MH-N`. Ids are never reused, so a dropped card keeps its number and holes are expected.
- **Statuses.** `idea` is captured but unscored; `triaged` is scored and accepted; `specced` has a spec in any pre-implementation state; `implementing` has a spec being built; `shipped` has every named spec in `done/` or `archived/`; `dropped` will not be done, with the reason in a decision entry. A card whose specs ship separately stays `implementing` until the last one ships.
- **Score.** Weighted shortest job first: `(value + urgency + risk) / effort`, rounded half up to one decimal. Each input is 1 to 5:
  - **value**: how much the card advances the product for its users: 5 is the core promise of the current stage, 1 a nicety.
  - **urgency**: derived, never chosen. 5 when the card's stage is the current stage or earlier, 3 for the next stage, 2 for the one after, 1 beyond that or `later`. The current stage is in [objectives.md](objectives.md).
  - **risk**: the risk the card retires: 5 for a safety invariant or an unproven boundary the spec depends on, 1 for none.
  - **effort**: 1 for a few tasks in one domain, 3 for a spec of about ten tasks, 5 for an epic.
- **Order.** Open cards (`idea`, `triaged`, `specced`, `implementing`) sit under `## Open` by score, highest first, then by id. Closed cards (`shipped`, `dropped`) sit under `## Closed` by id. The tool writes this order; the `product` check of `scripts/ci/lint-agent-harness.sh` fails on anything else.

## Open

### MH-9: OpenSSF Best Practices badge
- **Status**: triaged
- **Stage**: 1
- **Gates**: G10
- **Score**: 9.0 = (value 2 + urgency 5 + risk 2) / effort 1
- **Spec**: (none)
- **Issue**: [#30](https://github.com/turbokast/mythhelm/issues/30)
- **Source**: master spec §19.3, §19.4 and the G10 public-release gate of §18.7
- **Summary**: Complete the OpenSSF Best Practices questionnaire for the passing level and show the badge in the README, recording any criterion the project does not yet meet as its own card.

### MH-10: Harness compatibility records and qualification registry
- **Status**: triaged
- **Stage**: 0
- **Gates**: G02, G05
- **Score**: 4.7 = (value 4 + urgency 5 + risk 5) / effort 3
- **Spec**: (none)
- **Issue**: [#31](https://github.com/turbokast/mythhelm/issues/31)
- **Source**: master spec §9.2, §9.14, §18.3, §20.2 and §22.2 item 1
- **Summary**: The evidence fixtures Stage 0 calls for beyond the dogfood slice's Claude Code probe: per-harness capability, fidelity and entitlement records, a registry that shows qualified, experimental and blocked surfaces honestly, and Herdr bridge fixtures. Every advertised capability then points at a versioned result (I14).

### MH-7: Release pipeline: GoReleaser archives, attestations and signatures
- **Status**: triaged
- **Stage**: 1
- **Gates**: G01, G10
- **Score**: 4.3 = (value 4 + urgency 5 + risk 4) / effort 3
- **Spec**: (none)
- **Issue**: [#28](https://github.com/turbokast/mythhelm/issues/28)
- **Source**: master spec §16.5, §19.4, §19.5 and §20.3 (installable binary); the release rows of the Deferred table in docs/automation.md
- **Summary**: A tag-triggered release that builds cross-platform archives with GoReleaser, checksums and an SBOM, build-provenance attestations, keyless signatures and licence notices, publishing from a protected environment only. It turns the Stage 1 binary into something users can install and verify.

### MH-11: Windows process-tree ownership
- **Status**: triaged
- **Stage**: 1
- **Gates**: G04
- **Score**: 4.3 = (value 4 + urgency 5 + risk 4) / effort 3
- **Spec**: (none)
- **Issue**: [#32](https://github.com/turbokast/mythhelm/issues/32)
- **Source**: master spec §7.4, §18.4, §20.2 and §22.2 item 2
- **Summary**: Job Objects and process-tree ownership for workers on Windows, so detach, stop, crash and orphan handling pass there as they do on Unix. The dogfood slice keeps the seam and blocks the native adapter on Windows (its non-goal N7).

### MH-12: Strict subscription-only qualification for Claude Code
- **Status**: triaged
- **Stage**: 1
- **Gates**: G05
- **Score**: 3.8 = (value 5 + urgency 5 + risk 5) / effort 4
- **Spec**: (none)
- **Issue**: [#33](https://github.com/turbokast/mythhelm/issues/33)
- **Source**: master spec §13.8, §13.9, §13.10, §18.9 and §20.3
- **Summary**: Entitlement and overage-prevention evidence that lets strict subscription-only admission pass for Claude Code instead of blocking. The dogfood slice ships only a user-declared, never-verified posture (its non-goal N8); this card earns the strict one (I15).

### MH-17: Account profiles and exhaustion handoff
- **Status**: triaged
- **Stage**: 2
- **Gates**: G04, G05
- **Score**: 3.7 = (value 4 + urgency 3 + risk 4) / effort 3
- **Spec**: (none)
- **Issue**: (none)
- **Source**: master spec §13.2, §13.3, §13.10, §8.3 and §20.4; signal S-4; decision D-2
- **Summary**: Named account profiles bound to documented native homes (CLAUDE_CONFIG_DIR, CODEX_HOME) with native sign-in and tested credential isolation, never reading or copying a token (I19), each with an explicit quota-bucket id. On a structured limit error the run preserves its work as waiting_for_allowance and offers a user-confirmed switch to another authorised profile through a handoff package (MH-15); cross-provider failover only under a pre-approved policy (I04, I16). Same-vendor automatic rotation is excluded (§13.2).

### MH-13: Contained execution profiles: restricted and inspect
- **Status**: triaged
- **Stage**: 1
- **Gates**: G07
- **Score**: 3.5 = (value 4 + urgency 5 + risk 5) / effort 4
- **Spec**: (none)
- **Issue**: [#34](https://github.com/turbokast/mythhelm/issues/34)
- **Source**: master spec §12.2, §12.3, §18.5 and §20.3
- **Summary**: The `restricted` and `inspect` execution profiles with boundaries that pass adversarial tests, and a clear refusal wherever an operating system cannot provide them. The dogfood slice admits only `trusted-host` with explicit consent (its non-goal N9).

### MH-2: TUI slice: the focused mission view
- **Status**: triaged
- **Stage**: 1
- **Gates**: G09
- **Score**: 3.3 = (value 5 + urgency 5 + risk 3) / effort 4
- **Spec**: (none)
- **Issue**: [#23](https://github.com/turbokast/mythhelm/issues/23)
- **Source**: master spec §15 (visual language, mission view, responsive layouts, navigation, motion, accessibility, render model), §20.3 and §22.2 item 5
- **Summary**: The polished focused TUI that Stage 1 requires, on top of the run pipeline the dogfood slice delivers: the mission view, responsive layouts down to the linear accessible mode, keyboard flows and event-driven motion. The dogfood slice deliberately defers it (its non-goal N1).

### MH-3: Herdr bridge: single-pane experience with scoped status
- **Status**: triaged
- **Stage**: 1
- **Gates**: G11
- **Score**: 3.3 = (value 4 + urgency 5 + risk 4) / effort 4
- **Spec**: (none)
- **Issue**: [#24](https://github.com/turbokast/mythhelm/issues/24)
- **Source**: master spec §16.6 (topology, scoped discovery, state mapping, input ownership, detach and restore), §5.7, §15.11, §18.10, §20.3 and §22.2 item 5
- **Summary**: First-class use inside Herdr: launch in a pane, project scoped MYTHHELM state, keep one input writer and restore attachments rather than relaunching work. Herdr may present state but never certifies completion or creates a second process owner (I17, I18). Lane inspectors and native attachment follow in Stage 2.

### MH-1: Dogfood slice: one native Claude Code attempt, from task to applied candidate
- **Status**: implementing
- **Stage**: 1
- **Gates**: G01, G03, G04, G06, G07
- **Score**: 3.0 = (value 5 + urgency 5 + risk 5) / effort 5
- **Spec**: `dogfood-slice`
- **Issue**: [#22](https://github.com/turbokast/mythhelm/issues/22)
- **Source**: master spec §20.3 and §22.2 items 2 to 4 (ownership, source protection, the delivery loop), the scripted fake adapter of §22.2 item 1, and the plain and JSONL output and offline demo of item 5
- **Summary**: The first runnable MYTHHELM: admission, a managed snapshot, a detached worker owning one native Claude Code attempt, configured checks, a receipt and a guarded apply, with the offline demo and read-only doctor. It proves the task to reviewable artifact loop on MYTHHELM itself before any second adapter, TUI or routing work.

### MH-8: Documentation site and scripted terminal demos
- **Status**: triaged
- **Stage**: 1
- **Gates**: G10
- **Score**: 3.0 = (value 3 + urgency 5 + risk 1) / effort 3
- **Spec**: (none)
- **Issue**: [#29](https://github.com/turbokast/mythhelm/issues/29)
- **Source**: master spec §14.9, §19.3 and §22.3; the GitHub Pages and VHS rows of the Deferred table in docs/automation.md
- **Summary**: Published user and contributor guides extracted from the spec, and reproducible terminal recordings of the demo and TUI made with Charm VHS for the docs and README. The recordings depend on the TUI slice.

### MH-4: Codex adapter: the second qualified native adapter
- **Status**: triaged
- **Stage**: 2
- **Gates**: G02, G05
- **Score**: 2.8 = (value 5 + urgency 3 + risk 3) / effort 4
- **Spec**: (none)
- **Issue**: [#25](https://github.com/turbokast/mythhelm/issues/25)
- **Source**: master spec §9.4, §9.9, §13.9, §18.8, §20.4
- **Summary**: A native Codex adapter qualified to the same fidelity, entitlement, cancellation and configuration gates as the first adapter, so the public alpha has two independently qualified harnesses. Unqualified routes stay visibly experimental or blocked.

### MH-16: Quota observations and the budget ledger
- **Status**: triaged
- **Stage**: 2
- **Gates**: G05
- **Score**: 2.8 = (value 4 + urgency 3 + risk 4) / effort 4
- **Spec**: (none)
- **Issue**: [#37](https://github.com/turbokast/mythhelm/issues/37)
- **Source**: master spec §13.3 to §13.6 and §20.4
- **Summary**: Quota observations, the budget ledger with reservations and the hard-limit versus soft-guardrail distinction, keeping estimated, reported, observed and unknown values apart (I09, I10). The dogfood slice stores native cost only as a labelled estimate (its non-goal N13).

### MH-5: Routing across authorised profiles
- **Status**: triaged
- **Stage**: 2
- **Gates**: G02, G05
- **Score**: 2.5 = (value 4 + urgency 3 + risk 3) / effort 4
- **Spec**: (none)
- **Issue**: [#26](https://github.com/turbokast/mythhelm/issues/26)
- **Source**: master spec §8 (task descriptors, admission before scoring, ranking, routing explanations), §13.2 and §20.4
- **Summary**: Deterministic, explained selection among the user's authorised execution profiles, with admission before scoring and no silent move to another provider, billing route or sandbox (I04, I16). The dogfood slice pins one adapter per run (its non-goal N3); this card replaces the pin.

### MH-6: Plugin protocol and declarative themes
- **Status**: triaged
- **Stage**: 2
- **Gates**: G08
- **Score**: 2.0 = (value 4 + urgency 3 + risk 3) / effort 5
- **Spec**: (none)
- **Issue**: [#27](https://github.com/turbokast/mythhelm/issues/27)
- **Source**: master spec §14 (extension classes and points, process protocol, manifest, declarative mods, installation and revocation, SDK), §20.3 (one declarative theme format), §20.4 and §22.2 item 6
- **Summary**: The versioned plugin process protocol, the trusted executable-plugin flow and the declarative theme, keymap, layout and workflow mods, with conformance tests. Plugins can never override admission, approval, evidence or publication (I11). The theme format is a Stage 1 deliverable and can be split out when the TUI slice needs it.

### MH-14: Task DAG, two-writer mode and the integration train
- **Status**: triaged
- **Stage**: 2
- **Gates**: G06
- **Score**: 2.0 = (value 4 + urgency 3 + risk 3) / effort 5
- **Spec**: (none)
- **Issue**: [#35](https://github.com/turbokast/mythhelm/issues/35)
- **Source**: master spec §8.7, §11.3, §11.4, §11.5 and §20.4
- **Summary**: Dependency-aware decomposition with bounded parallelism, a second writer in its own workspace, resource reservations and the serialized integration train that checks frozen candidates at the final revision. The dogfood slice keeps one writer (its non-goal N4).

### MH-15: Messages, blackboard and handoff packages
- **Status**: triaged
- **Stage**: 2
- **Gates**: none
- **Score**: 2.0 = (value 3 + urgency 3 + risk 2) / effort 4
- **Spec**: (none)
- **Issue**: [#36](https://github.com/turbokast/mythhelm/issues/36)
- **Source**: master spec §10 and §20.4
- **Summary**: Message semantics with real delivery and provenance, the blackboard as an evidence store, handoff packages and context budgeting, so parallel agents share evidence rather than a pretended shared mind.

## Closed

No closed cards.
