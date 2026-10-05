# Backlog

Every planned piece of work larger than a fix, as one card, scored and ordered. Open cards cite [Master Specification v2](../mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md); the [adoption map](../docs/spec/synthesis-adoption.md) connects W01–W16 and their prerequisites. Closed cards and older decision/signal entries retain Revision 1.1 section meanings. A shipped historical slice does not imply v2 gate qualification. The backlog is the plan of record; [roadmap.md](roadmap.md) is a view generated from it. Each card has a public GitHub issue, which is where contributors discuss it. Who may change this file, and how, is in [README.md](README.md).

## Schema

A card is a level-3 heading and a fixed list of fields, in this order:

```markdown
### MH-<n>: <title>
- **Status**: idea | triaged | specced | implementing | shipped | dropped
- **Stage**: 0 | 1 | 2 | 3 | 4 | 5 | 6 | later
- **Gates**: G01, G03 (release gates G01–G16 of v2 §18.3 the card moves toward passing), or none
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

### MH-32: TUI terminal recording (FR-3 follow-up)
- **Status**: idea
- **Stage**: 1
- **Gates**: G10
- **Score**: 8.0 = (value 2 + urgency 5 + risk 1) / effort 1
- **Spec**: (none)
- **Issue**: (none)
- **Source**: historical R1.1 recording contract (master spec §§14.9, 22.3; docs-site-demos FR-3/AC-3.2/AC-3.3, design §5); current contract Master Specification v2 §15 via W05; TUI evidence in specs/done/tui-slice/ (shipped 2026-10-04)
- **Summary**: Record the shipped TUI on tape: drive the mission view, responsive layouts down to linear mode and keyboard flows with the TUI's own verification status on screen, limited to exactly the covered tui-slice behaviour; ship the full tape with header pins, a manifest entry naming qualifying tests, and a normalized transcript per the docs-site-demos tape contract, never pane text as proof.

### MH-10: Harness compatibility records and qualification registry
- **Status**: triaged
- **Stage**: 0
- **Gates**: G02, G05, G07, G12
- **Score**: 5.0 = (value 5 + urgency 5 + risk 5) / effort 3
- **Spec**: (none)
- **Issue**: [#31](https://github.com/turbokast/mythhelm/issues/31)
- **Source**: Master Specification v2 §§4, 7–8, 14, 17–18; W01/W02/W14; historical issue #31
- **Summary**: Create a versioned seven-harness qualification registry with independent fidelity, entitlement, lifecycle/trust and platform/host evidence, drift triggers and next tests for blocked routes. Prove a credible first included-only route through authorised tests; neither subscription sign-in nor a user declaration establishes no paid continuation.
- **Notes**: S0 blocker and first product prerequisite; currently subscription-only always blocks. Unknown mandatory entitlement/no-overage evidence blocks, while unknown remaining quota alone need not block an otherwise qualified stop-at-exhaustion route. No live usage or account changes are authorised by this card.

### MH-20: Strictest practical lint set
- **Status**: triaged
- **Stage**: 1
- **Gates**: G10
- **Score**: 5.0 = (value 3 + urgency 5 + risk 2) / effort 2
- **Spec**: (none)
- **Issue**: [#115](https://github.com/turbokast/mythhelm/issues/115)
- **Source**: OpenSSF passing criterion warnings_strict (SUGGESTED); openssf-badge design §3; Master Specification v2 §§17–18; W16
- **Summary**: The lint set is golangci-lint standard plus bodyclose, errorlint, gosec, misspell and nolintlint (verified in .golangci.yml) — solid but not maximal, so warnings_strict is Unmet. Evaluate and enable the strictest practical further linters and fix the resulting fallout.

### MH-7: Release pipeline: GoReleaser archives, attestations and signatures
- **Status**: triaged
- **Stage**: 1
- **Gates**: G01, G10
- **Score**: 4.3 = (value 4 + urgency 5 + risk 4) / effort 3
- **Spec**: (none)
- **Issue**: [#28](https://github.com/turbokast/mythhelm/issues/28)
- **Source**: Master Specification v2 §§1, 17–18; W16; historical issue #28
- **Summary**: A tag-triggered release that builds cross-platform archives with GoReleaser, checksums and an SBOM, build-provenance attestations, keyless signatures and licence notices, publishing from a protected environment only. It turns the Stage 1 binary into something users can install and verify.
- **Notes**: Release evidence recurs for every supported subset, including S1; it does not wait for the full adaptive destination. Actual releases, signing settings and tags still require operator action.

### MH-11: Windows process-tree ownership
- **Status**: triaged
- **Stage**: 1
- **Gates**: G04
- **Score**: 4.3 = (value 4 + urgency 5 + risk 4) / effort 3
- **Spec**: (none)
- **Issue**: [#32](https://github.com/turbokast/mythhelm/issues/32)
- **Source**: Master Specification v2 §§6, 17; W03/W14; historical issue #32
- **Summary**: Job Objects and process-tree ownership for workers on Windows, so detach, stop, crash and orphan handling pass there as they do on Unix. The dogfood slice keeps the seam and blocks the native adapter on Windows (its non-goal N7).
- **Notes**: Qualify Windows process-tree ownership, cancellation, detach, filesystem and terminal combinations explicitly. Advertise only tested subsets; Unix tests do not establish Windows support.

### MH-18: Adopt SemVer or CalVer for releases
- **Status**: triaged
- **Stage**: 1
- **Gates**: G10
- **Score**: 4.0 = (value 2 + urgency 5 + risk 1) / effort 2
- **Spec**: (none)
- **Issue**: [#113](https://github.com/turbokast/mythhelm/issues/113)
- **Source**: OpenSSF passing criterion version_semver (SUGGESTED); openssf-badge design §3; Master Specification v2 §§17–18; W16
- **Summary**: No releases exist yet (verified: no git tags) and no version-numbering format is adopted, so version_semver is Unmet. Decide SemVer vs CalVer (with micro level for CalVer) and record it in the release process before MH-7 ships the first release.

### MH-19: Tag every release in git
- **Status**: triaged
- **Stage**: 1
- **Gates**: G10
- **Score**: 4.0 = (value 2 + urgency 5 + risk 1) / effort 2
- **Spec**: (none)
- **Issue**: [#114](https://github.com/turbokast/mythhelm/issues/114)
- **Source**: OpenSSF passing criterion version_tags (SUGGESTED); openssf-badge design §3; Master Specification v2 §§17–18; W16
- **Summary**: No releases exist yet (verified: no git tags), so version_tags is Unmet. Establish the tag-per-release discipline (tag format, who tags, GoReleaser trigger) alongside the MH-7 release pipeline.

### MH-12: Strict subscription-only qualification for Claude Code
- **Status**: triaged
- **Stage**: 1
- **Gates**: G02, G05, G07
- **Score**: 3.8 = (value 5 + urgency 5 + risk 5) / effort 4
- **Spec**: (none)
- **Issue**: [#33](https://github.com/turbokast/mythhelm/issues/33)
- **Source**: Master Specification v2 §§7–8; W02/W14; historical issue #33
- **Summary**: Entitlement and overage-prevention evidence that lets strict subscription-only admission pass for Claude Code instead of blocking. The dogfood slice ships only a user-declared, never-verified posture (its non-goal N8); this card earns the strict one (I15).
- **Notes**: MH-10 supplies versioned records. Existing subscription-declared Claude dogfood remains unverified; preserve strict blocking until effective configuration, all auxiliary/child routes and paid-continuation prevention are proven. Claude need not be the first route.

### MH-22: Protected acceptance and a complete v2 one-agent workflow
- **Status**: triaged
- **Stage**: 1
- **Gates**: G01, G03, G05, G06, G07
- **Score**: 3.8 = (value 5 + urgency 5 + risk 5) / effort 4
- **Spec**: (none)
- **Issue**: [#128](https://github.com/turbokast/mythhelm/issues/128)
- **Source**: Master Specification v2 §§4, 8, 10–11, 15; Implementation Plan W04; issues #72–#76
- **Summary**: Extend the existing snapshot/check/review/apply flow to deterministic P1 with strict qualified admission, protected acceptance bound to exact revisions, finite repair/resource envelopes and truthful receipts. Preserve an unverified candidate when checks are unavailable; candidate edits cannot weaken its protected evaluator.
- **Premise-grounded**: Confirmed: internal/admission/billing.go blocks subscription-only; the current dogfood check/apply flow and schema v1 do not implement v2 TaskRevision or policy pins. Issues #72–#76 record remaining receipt and recovery gaps; implementation must re-check each issue before duplicating a fix.
- **Notes**: Requires the durable supervisor/contracts card, MH-10 and at least one independently qualified route, MH-13 and the S1 portion of MH-16. Value 5: accepted delivery; risk 5: false acceptance/authority; effort 4: extends multiple existing paths.

### MH-17: Account profiles and exhaustion handoff
- **Status**: triaged
- **Stage**: 2
- **Gates**: G04, G05
- **Score**: 3.7 = (value 4 + urgency 3 + risk 4) / effort 3
- **Spec**: (none)
- **Issue**: [#82](https://github.com/turbokast/mythhelm/issues/82)
- **Source**: Master Specification v2 §§7–10; W06; historical issue #82 and S-4
- **Summary**: Support explicitly authorised native account profiles and exhaustion handoff without copying secrets or rotating identities to evade service limits. Re-admit the destination profile and distinguish compatible native resume from cross-harness reconstruction.
- **Notes**: Requires MH-10, MH-15 and MH-16; native profile/home selection is enabled only where separately qualified. Unknown entitlement blocks even when remaining allowance is user-declared.

### MH-13: Contained execution profiles: restricted and inspect
- **Status**: triaged
- **Stage**: 1
- **Gates**: G07
- **Score**: 3.5 = (value 4 + urgency 5 + risk 5) / effort 4
- **Spec**: (none)
- **Issue**: [#34](https://github.com/turbokast/mythhelm/issues/34)
- **Source**: Master Specification v2 §§7–8, 11; W02/W04; historical issue #34
- **Summary**: Implement and adversarially test restricted/inspect execution boundaries, including native startup hooks/MCP/plugins, file/network scope and protected verification. A trusted-host profile discloses residual same-user-code trust; reservations and UI toggles cannot masquerade as containment.
- **Notes**: Required for claimed S1 enforcement. Preserve native authentication and useful tools within qualified profiles; unknown effective configuration or boundary evidence blocks affected launch.

### MH-3: Herdr bridge: single-pane experience with scoped status
- **Status**: triaged
- **Stage**: 1
- **Gates**: G11
- **Score**: 3.3 = (value 4 + urgency 5 + risk 4) / effort 4
- **Spec**: (none)
- **Issue**: [#24](https://github.com/turbokast/mythhelm/issues/24)
- **Source**: Master Specification v2 §§3, 6, 14–15; W05; historical issue #24
- **Summary**: Deliver the embedded-client and minimal schema-negotiated Herdr bridge with scoped status, attention, safe controls and input takeover. Attach/reconnect uses the one core lifecycle owner; restored panes never start another attempt.
- **Notes**: Requires MH-21 and MH-22; pair with MH-23. Alternative native surfaces need their own G12 qualification; no Herdr-only inference or second scheduler.

### MH-4: Codex native adapter and included-only qualification
- **Status**: triaged
- **Stage**: 1
- **Gates**: G02, G04, G05
- **Score**: 3.3 = (value 5 + urgency 5 + risk 3) / effort 4
- **Spec**: (none)
- **Issue**: [#25](https://github.com/turbokast/mythhelm/issues/25)
- **Source**: Master Specification v2 §7; W02/W06/W14; historical issue #25
- **Summary**: Qualify Codex as a native harness independently across fidelity, subscription entitlement, stop/recovery and session continuity. It may become the first credible included-only route or a later second route; qualification evidence determines order, with no SDK or same-model substitution.
- **Notes**: Requires MH-10 records and effective startup/trust controls. Restaged to S1 so first-route selection is evidence-led; being the second named adapter is not a product requirement.

### MH-16: Quota observations and the budget ledger
- **Status**: triaged
- **Stage**: 1
- **Gates**: G05
- **Score**: 3.3 = (value 4 + urgency 5 + risk 4) / effort 4
- **Spec**: (none)
- **Issue**: [#37](https://github.com/turbokast/mythhelm/issues/37)
- **Source**: Master Specification v2 §§7–8, 10, 12–13; W03/W04/W11; historical issue #37
- **Summary**: Record typed observed/reported/estimated/declared/unknown usage and coupled quota reservations, finite run limits and foreground completion reserves. Enforce only established boundaries; exhaustion pauses or stops safely without paid fallback, while unknown remaining quota stays distinct from zero.
- **Notes**: S1 needs the basic ledger/envelopes for one-agent delivery; multi-profile and experiment scheduling extend it in S2/S4. Requires MH-10 billing semantics and durable supervisor contracts; no provider hard-cap claim from cancellation alone.

### MH-23: Complete TUI intervention, accessibility and safe declarative themes
- **Status**: triaged
- **Stage**: 1
- **Gates**: G08, G09
- **Score**: 3.3 = (value 4 + urgency 5 + risk 4) / effort 4
- **Spec**: (none)
- **Issue**: [#129](https://github.com/turbokast/mythhelm/issues/129)
- **Source**: Master Specification v2 §§14–16; Implementation Plan W05; issues #106/#107; specs/done/tui-slice/retrospective.md
- **Summary**: Finish detach, stop confirmation, recovery and review journeys on typed core controls, with keyboard-complete accessible output and human screen-reader evidence. Add safe declarative themes/keymaps and visible critical controls without making the S3 executable plugin protocol a prerequisite.
- **Premise-grounded**: Confirmed: MH-2 is shipped as the historical TUI slice; README and issues #106/#107 record exit/detach gaps and limited human accessibility evidence. The remaining work does not reopen or relabel MH-2.
- **Notes**: Requires durable core control and the v2 one-agent workflow for complete journey acceptance; pairs with MH-3 for Herdr. Value 4: core usability; risk 4: misleading lifecycle/hidden controls; effort 4: interaction and human evidence.

### MH-8: Documentation site and scripted terminal demos
- **Status**: specced
- **Stage**: 1
- **Gates**: G10
- **Score**: 3.0 = (value 3 + urgency 5 + risk 1) / effort 3
- **Spec**: `docs-site-demos`
- **Issue**: [#29](https://github.com/turbokast/mythhelm/issues/29)
- **Source**: Master Specification v2 §§1, 15, 17–18; W05/W16; historical issue #29
- **Summary**: Published user and contributor guides extracted from the spec, and reproducible terminal recordings of the demo and TUI made with Charm VHS for the docs and README. The recordings depend on the TUI slice.
- **Premise-grounded**: 2026-10-02 — 12 claims, 12 HOLDS, 0 PARTIAL, 0 UNVERIFIABLE; TUI recording sequences behind tui-slice (MH-2)
- **Notes**: docs-site-demos remains specced against Revision 1.1. Review and reconcile its requirements/design before resuming implementation; do not silently reinterpret old section numbers. Demonstrate actual supported capabilities and distinguish S1 from the adaptive destination.

### MH-21: Canonical v2 contracts and durable supervisor migration
- **Status**: triaged
- **Stage**: 1
- **Gates**: G04, G16
- **Score**: 3.0 = (value 5 + urgency 5 + risk 5) / effort 5
- **Spec**: (none)
- **Issue**: [#127](https://github.com/turbokast/mythhelm/issues/127)
- **Source**: Master Specification v2 §§3–6; Implementation Plan W01/W03; ADRs 0003–0005 and 0007
- **Summary**: Define revisioned task, event, policy and authority contracts, then migrate to one per-user supervisor with authenticated local control and global reservations. Preserve worker-owned processes and v1 evidence through explicit backup, drain/adopt/quarantine and restore; UI exit must not own execution.
- **Premise-grounded**: Confirmed at eb349c2: internal/supervisor and ADR 0005 use per-run owners; internal/journal/migrations/0001_init.sql has no v2 task/policy tables. This is a migration, not an already delivered daemon.
- **Notes**: W01 contract review precedes dispatch APIs; W03 can use fake adapters before live qualification. Value 5: S1 continuity; risk 5: duplicate launch, loss and stale authority; effort 5: cross-domain epic. Split implementation into reviewed lifecycle specs before task execution.

### MH-24: Scoped context, task dependencies and stale-evidence invalidation
- **Status**: triaged
- **Stage**: 2
- **Gates**: G07, G13
- **Score**: 3.0 = (value 5 + urgency 3 + risk 4) / effort 4
- **Spec**: (none)
- **Issue**: [#130](https://github.com/turbokast/mythhelm/issues/130)
- **Source**: Master Specification v2 §§4, 9–11; Implementation Plan W07
- **Summary**: Add mandatory original context, immutable source manifests, exact/lexical retrieval and versioned task dependencies with selective invalidation. Interface checkpoints may unblock dependent construction while final acceptance still requires the exact integrated implementation; optional search failure falls back to originals.
- **Premise-grounded**: Confirmed: schema v1 records runs/attempts/candidates but no task revisions, dependency graph or scoped retrieval index. Native transcripts and Markdown views must not become a second task authority.
- **Notes**: Requires S1 and MH-15 continuity; bounded graph/context foundations precede MH-14 parallel writers. Value 5: reliable continuity; risk 4: leakage/stale acceptance; effort 4: canonical data, retrieval and invalidation.

### MH-25: Scoped export, retention, erasure and tested restoration
- **Status**: triaged
- **Stage**: 2
- **Gates**: G13, G16
- **Score**: 3.0 = (value 4 + urgency 3 + risk 5) / effort 4
- **Spec**: (none)
- **Issue**: [#131](https://github.com/turbokast/mythhelm/issues/131)
- **Source**: Master Specification v2 §§5, 8–9, 12; Implementation Plan W13
- **Summary**: Implement scoped export and dependency-aware retention, with explicit erasure through a quiescent validated ledger rewrite and recoverable swap. Rebuild derived stores, preserve active dependencies and report surviving backup/native/provider copies honestly.
- **Premise-grounded**: Confirmed: schema v1 and current CLI dispatch have no v2 scoped export/erasure lifecycle; append-only audit rules require an explicit maintenance path rather than ordinary event deletion.
- **Notes**: Requires durable migration and scoped context; must ship before optional-learning retention/deletion claims. Value 4: control and portability; risk 5: irreversible loss/leakage; effort 4: recovery and derived-state invalidation.

### MH-5: Routing across authorised profiles
- **Status**: triaged
- **Stage**: 2
- **Gates**: G02, G05
- **Score**: 2.5 = (value 4 + urgency 3 + risk 3) / effort 4
- **Spec**: (none)
- **Issue**: [#26](https://github.com/turbokast/mythhelm/issues/26)
- **Source**: Master Specification v2 §§7, 10, 12–13; W06/W10; historical issue #26
- **Summary**: Rank only authorised qualified routes with deterministic preferences, capability filtering and recorded uncertainty. Preserve user pins and standing grants; invalid optional smart routing falls back to eligible P1 without paid calls or hidden harness substitution.
- **Notes**: Requires two qualified routes, MH-21, MH-16 and MH-15. Learned ranking follows MH-26/MH-27; cheap-first and kNN ideas in S-3 remain testable hypotheses.

### MH-27: Bounded protected experiments for routes, context and prompts
- **Status**: triaged
- **Stage**: 4
- **Gates**: G05, G14
- **Score**: 2.5 = (value 4 + urgency 1 + risk 5) / effort 4
- **Spec**: (none)
- **Issue**: [#133](https://github.com/turbokast/mythhelm/issues/133)
- **Source**: Master Specification v2 §§8, 10, 12–13; Implementation Plan W11
- **Summary**: Register bounded experiments with protected evaluators, held-out splits, full lifecycle costs, foreground priority and reconciled preemption. Exploration defaults to zero; every auxiliary call needs the same billing and authority evidence as production work, and inconclusive results remain inconclusive.
- **Premise-grounded**: Confirmed: no experiment executor or learning policy tables exist at eb349c2. Reuse the canonical run/attempt machinery; do not introduce an independent executor.
- **Notes**: Requires fixed-portfolio outcome evidence and qualified variants; parallel variants additionally require MH-14. Value 4: useful evidence; risk 5: spending/contamination; effort 4: controlled allocation and statistics. No automatic promotion in this slice.

### MH-30: Optional verified production delivery with effect reconciliation
- **Status**: triaged
- **Stage**: 6
- **Gates**: G07, G15
- **Score**: 2.5 = (value 4 + urgency 1 + risk 5) / effort 4
- **Spec**: (none)
- **Issue**: [#136](https://github.com/turbokast/mythhelm/issues/136)
- **Source**: Master Specification v2 §§8, 11, 17–18; Implementation Plan W15
- **Summary**: Deliver an exact verified release to an explicitly authorised destination with independent health evidence, idempotency or reconciliation and bounded rollback. Without deployment authority, retain a reviewable release-ready candidate and report the blocked effect; deployment stays off by default.
- **Premise-grounded**: Confirmed: the product CLI exposes no deployment command or qualified destination adapter. This is optional future application behavior, not authority to deploy during planning.
- **Notes**: Depends on S1 and destination qualification, independently of S4/S5 learning. Value 4: completed software outcomes; risk 5: uncertain external effects; effort 4: destination-specific lifecycle. Stage 6 numbering must not force learning as a dependency.

### MH-26: Consented outcome evidence and the fixed execution portfolio
- **Status**: triaged
- **Stage**: 4
- **Gates**: G14
- **Score**: 2.3 = (value 4 + urgency 1 + risk 4) / effort 4
- **Spec**: (none)
- **Issue**: [#132](https://github.com/turbokast/mythhelm/issues/132)
- **Source**: Master Specification v2 §§10, 12–13; Implementation Plan W10
- **Summary**: Implement P1/P2/P4 and later qualified P3, with optional scoped learning collection, pre-assignment provenance and all-started outcome comparisons against direct-native and simpler baselines. Essential receipts work with learning off; missingness, failed runs and delayed regressions remain visible.
- **Premise-grounded**: Confirmed: the current core has no policy/experiment/outcome schema or evaluation runner. Vendor capability descriptions and synthesis walkthroughs are design evidence, not measured product superiority.
- **Notes**: Requires scoped context and export/erasure; MH-14 only for P3. Value 4: defensible policy choice; risk 4: biased metrics; effort 4: instrumentation and offline evaluation. Urgency derives from S4; no collection or live experiment is authorised by this card.

### MH-28: Scoped policy promotion, rollback and new-model qualification
- **Status**: triaged
- **Stage**: 5
- **Gates**: G02, G05, G14
- **Score**: 2.2 = (value 5 + urgency 1 + risk 5) / effort 5
- **Spec**: (none)
- **Issue**: [#134](https://github.com/turbokast/mythhelm/issues/134)
- **Source**: Master Specification v2 §§7, 12–13; Implementation Plan W12
- **Summary**: Promote immutable runtime policies only through protected scoped evidence gates, pin active attempts, canary new assignments and roll back under registered rules. Qualify new model/harness combinations and compare a new single agent against whole pipelines; a simpler winner may replace the pipeline.
- **Premise-grounded**: Confirmed: adaptive promotion is new work, with no runtime implementation or live efficacy evidence in the current repository. Executable updates remain reviewed software releases.
- **Notes**: Requires protected experiments, route qualification and promotion authority; MH-14 only for parallel claims. Value 5: full adaptive promise; risk 5: regressions/authority drift; effort 5: cross-domain epic. Learning can be disabled without losing fixed-policy delivery.

### MH-29: Remaining native harness qualification and platform coverage tracks
- **Status**: triaged
- **Stage**: 2
- **Gates**: G02, G05, G10, G12
- **Score**: 2.2 = (value 4 + urgency 3 + risk 4) / effort 5
- **Spec**: (none)
- **Issue**: [#135](https://github.com/turbokast/mythhelm/issues/135)
- **Source**: Master Specification v2 §§7, 14, 17; Implementation Plan W14; explicit UR-05/UR-06
- **Summary**: Maintain owned independent tracks for OpenCode, Meta Muse Code, Kimi Code CLI, Cursor Agent CLI and Antigravity alongside MH-12 Claude Code and MH-4 Codex. Publish per-surface fidelity, billing, lifecycle, trust and OS/host evidence; blocked targets remain explicit product commitments with next tests.
- **Premise-grounded**: Confirmed: internal/adapter contains claudecode and fake only; hosts/protocol/sdk packages are absent. No current native target has passed the full v2 included-only qualification contract.
- **Notes**: Requires MH-10 registry/contracts; individual tracks can qualify earlier when they unlock S1, with approved reprioritisation. MH-11 owns Windows process work, MH-3 Herdr, MH-7 release matrix. Value 4: seven-target/platform promise; risk 4: misleading support; effort 5: split by target before implementation. S2 is a continuing coverage track, not a promise all five ship in S2.

### MH-15: Native-session continuity and grounded cross-harness handoffs
- **Status**: triaged
- **Stage**: 2
- **Gates**: G04, G13
- **Score**: 2.0 = (value 3 + urgency 3 + risk 2) / effort 4
- **Spec**: (none)
- **Issue**: [#36](https://github.com/turbokast/mythhelm/issues/36)
- **Source**: Master Specification v2 §§4, 6, 9; W06; historical issue #36
- **Summary**: Continue a mission through acknowledged native-session deltas or an explicitly new admitted attempt, and reconstruct cross-harness handoffs from original requirements, decisions and immutable artifacts. Typed messages and context views project the canonical ledger instead of becoming a competing blackboard.
- **Notes**: Requires S1, MH-21 and two independent route qualifications. Unsupported sessions preserve artifacts and declare loss; no private native-store patching or summary-only requirements.

### MH-6: Public plugin protocol, SDK and declarative workflow mods
- **Status**: triaged
- **Stage**: 3
- **Gates**: G08
- **Score**: 1.8 = (value 4 + urgency 2 + risk 3) / effort 5
- **Spec**: (none)
- **Issue**: [#27](https://github.com/turbokast/mythhelm/issues/27)
- **Source**: Master Specification v2 §§8, 16; W09; historical issue #27
- **Summary**: Stabilise versioned process plugins and declarative mods after built-in consumers exercise the interfaces, with trust, bounded transport, revocation and conformance examples. Core admission, evidence and effects remain authoritative; a useful plugin journey must work without paid credentials.
- **Notes**: Requires MH-24; MH-14 only for parallel extensions. Basic S1 themes/keymaps belong to MH-23; public executable extensions are S3.

### MH-14: Bounded two-writer construction and serialized integration
- **Status**: triaged
- **Stage**: 3
- **Gates**: G04, G05, G06
- **Score**: 1.8 = (value 4 + urgency 2 + risk 3) / effort 5
- **Spec**: (none)
- **Issue**: [#35](https://github.com/turbokast/mythhelm/issues/35)
- **Source**: Master Specification v2 §§6, 9–11; W08; historical issue #35
- **Summary**: Use versioned task dependencies to run at most two independently reserved writers in isolated mutable environments, then serialize exact-candidate integration and protected checks. Native children share declared billing/resource envelopes; semantic conflicts enter bounded repair rather than concurrent fallback.
- **Notes**: Requires MH-24, MH-15, MH-16 and qualified child/stop behavior. Task graph and interface checkpoints start in MH-24; this card adds S3 parallel execution. Expired leases never prove a writer stopped.

## Closed

### MH-1: Dogfood slice: one native Claude Code attempt, from task to applied candidate
- **Status**: shipped
- **Stage**: 1
- **Gates**: G01, G03, G04, G06, G07
- **Score**: 3.0 = (value 5 + urgency 5 + risk 5) / effort 5
- **Spec**: `dogfood-slice`
- **Issue**: [#22](https://github.com/turbokast/mythhelm/issues/22)
- **Source**: master spec §20.3 and §22.2 items 2 to 4 (ownership, source protection, the delivery loop), the scripted fake adapter of §22.2 item 1, and the plain and JSONL output and offline demo of item 5
- **Summary**: The first runnable MYTHHELM: admission, a managed snapshot, a detached worker owning one native Claude Code attempt, configured checks, a receipt and a guarded apply, with the offline demo and read-only doctor. It proves the task to reviewable artifact loop on MYTHHELM itself before any second adapter, TUI or routing work.

### MH-2: TUI slice: the focused mission view
- **Status**: shipped
- **Stage**: 1
- **Gates**: G09
- **Score**: 3.3 = (value 5 + urgency 5 + risk 3) / effort 4
- **Spec**: `tui-slice`
- **Issue**: [#23](https://github.com/turbokast/mythhelm/issues/23)
- **Source**: master spec §15 (visual language, mission view, responsive layouts, navigation, motion, accessibility, render model), §20.3 and §22.2 item 5
- **Summary**: The polished focused TUI that Stage 1 requires, on top of the run pipeline the dogfood slice delivers: the mission view, responsive layouts down to the linear accessible mode, keyboard flows and event-driven motion. The dogfood slice deliberately defers it (its non-goal N1).
- **Premise-grounded**: 2026-10-02 — run pipeline exists HOLDS; dogfood N1 defers TUI HOLDS; Stage 1 requires polished TUI HOLDS; §15 coverage HOLDS; §22.2 item 5 HOLDS; G09 HOLDS; Bubble Tea stack HOLDS; no TUI exists yet HOLDS

### MH-9: OpenSSF Best Practices badge
- **Status**: shipped
- **Stage**: 1
- **Gates**: G10
- **Score**: 9.0 = (value 2 + urgency 5 + risk 2) / effort 1
- **Spec**: `openssf-badge`
- **Issue**: [#30](https://github.com/turbokast/mythhelm/issues/30)
- **Source**: master spec §19.3, §19.4 and the G10 public-release gate of §18.7
- **Summary**: Complete the OpenSSF Best Practices questionnaire for the passing level and show the badge in the README, recording any criterion the project does not yet meet as its own card.
- **Premise-grounded**: 2026-10-02 — C1 HOLDS; C2 HOLDS; C3 HOLDS; C4 HOLDS; C5 PARTIAL; C6 HOLDS (external); C7 PROCEDURE
