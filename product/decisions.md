# Decisions

The append-only log of product decisions: cards added, rejected or dropped, rescoring, lifecycle transitions, objective changes and the conclusions of impact and quarterly reviews. It is the record a later reader uses to understand why the backlog looks the way it does, so entries are never edited or removed. A decision that reverses an earlier one is a new entry that names the one it supersedes.

## Schema

```markdown
### D-<n> — YYYY-MM-DD: <title>
- **Type**: backlog-seed | card-add | card-reject | card-drop | rescore | lifecycle-sync | objective-change | impact-review | signal-triage | strategic-adjustment
- **Decision**: what was decided
- **Rationale**: why, with the evidence
- **Cards**: MH-<n>, MH-<n> (or none)
- **Evidence**: optional links: issues, pull requests, CI runs, evaluation results
```

`D-<n>` comes from `python3 scripts/pm/pm.py next-id D` when the entry is drafted. Entries are appended at the end, so ids increase and dates never go backwards down the file; the `product` check of `scripts/ci/lint-agent-harness.sh` enforces both.

## Log

### D-1 — 2026-09-29: Seed the backlog from the master spec
- **Type**: backlog-seed
- **Decision**: Seed the backlog with 16 cards covering the six initial items of §22.2 and the Stage 1 and Stage 2 deliverables of §20.3 and §20.4, set the current stage to 1, and score every card with the formula documented in backlog.md.
- **Rationale**: The in-flight dogfood-slice spec already covers §22.2 items 2 to 4 and parts of items 1 and 5, so it is one card (MH-1, implementing) and every other card names the dogfood non-goal it picks up. The remaining deliverables became cards for the next slices the spec implies: the TUI (§15), the Herdr bridge (§16.6), a second qualified adapter (§9.4), routing across profiles (§8), the plugin protocol and declarative themes (§14), the release pipeline and documentation deferred in docs/automation.md, the Best Practices badge (G10), compatibility records (§9.14), Windows ownership (§7.4), strict billing qualification (§13.9), contained profiles (§12.2), the task DAG and integration train (§11.5), messages and handoffs (§10) and quota observations (§13.3). Each card has a public GitHub issue so contributors can see and discuss the plan.
- **Cards**: MH-1, MH-2, MH-3, MH-4, MH-5, MH-6, MH-7, MH-8, MH-9, MH-10, MH-11, MH-12, MH-13, MH-14, MH-15, MH-16
- **Evidence**: the card issues [#22](https://github.com/turbokast/mythhelm/issues/22) to [#37](https://github.com/turbokast/mythhelm/issues/37)

### D-2 — 2026-10-02: Accept native multi-profile exhaustion handoff as a new card idea
- **Type**: signal-triage
- **Decision**: Maintainer accepted the plan from the multi-subscription signal: a new card for native multi-profile execution with user-confirmed exhaustion handoff goes to /triage; same-vendor automatic account rotation is not pursued
- **Rationale**: Users clearly want it, but Anthropic's terms forbid third-party developers from collecting, storing or intermediating Claude.ai credentials or session tokens (OpenAI's forbid circumventing rate limits) and the master spec (A04, §13.2, §13.10) forbids identity cycling to defeat a limit; native profile homes plus handoff deliver most of the liked behaviour within both
- **Cards**: MH-16,MH-15,MH-5
- **Evidence**: product/signals.md entry for this theme; https://code.claude.com/docs/en/legal-and-compliance

### D-4 — 2026-10-02: Add MH-17: account profiles and exhaustion handoff
- **Type**: card-add
- **Decision**: MH-17 filed as triaged, stage 2, score 3.7
- **Rationale**: Signal S-4 shows demand for continuing work when one subscription is exhausted; D-2 accepted the spec-compliant shape. No card covers the account-profile model, native-home credential isolation or the §13.10 exhaustion flow; MH-5, MH-15 and MH-16 are consumed, not duplicated.
- **Cards**: MH-17,MH-5,MH-15,MH-16
- **Evidence**: product/signals.md S-4; product/decisions.md D-2

### D-6 — 2026-10-02: MH-1 → shipped (dogfood-slice)
- **Type**: lifecycle-sync
- **Decision**: MH-1 moved to shipped
- **Rationale**: dogfood-slice reached done; finalize merged, main CI green
- **Cards**: MH-1
- **Evidence**: PR #78 (finalize, b0fcd81); main CI success at b0fcd81

### D-13 — 2026-10-02: Link MH-17 to issue #82
- **Type**: card-add
- **Decision**: MH-17's Issue field and its roadmap line in product/backlog.md and product/roadmap.md point to #82
- **Rationale**: The backlog add procedure creates a card's public issue after the card is approved and merged (PR #80), then records the link so contributors can find the discussion
- **Cards**: MH-17
- **Evidence**: https://github.com/turbokast/mythhelm/issues/82; PR #80 (ac0797d)

### D-14 — 2026-10-02: MH-2 → specced (tui-slice)
- **Type**: lifecycle-sync
- **Decision**: MH-2 moved to specced
- **Rationale**: tui-slice reached unrefined
- **Cards**: MH-2

### D-17 — 2026-10-02: MH-8 → specced (docs-site-demos)
- **Type**: lifecycle-sync
- **Decision**: MH-8 moved to specced
- **Rationale**: docs-site-demos reached unrefined with grounded premise
- **Cards**: MH-8

### D-18 — 2026-10-02: MH-9 → specced (openssf-badge)
- **Type**: lifecycle-sync
- **Decision**: MH-9 moved to specced
- **Rationale**: openssf-badge reached unrefined with grounded premise
- **Cards**: MH-9

### D-19 — 2026-10-02: MH-2 → implementing (tui-slice)
- **Type**: lifecycle-sync
- **Decision**: MH-2 moved to implementing
- **Rationale**: tui-slice reached in-progress; task PRs #89 #91 #92 merged
- **Cards**: MH-2

### D-20 — 2026-10-04: MH-2 → shipped (tui-slice)
- **Type**: lifecycle-sync
- **Decision**: MH-2 moved to shipped
- **Rationale**: tui-slice finalized and merged
- **Cards**: MH-2
- **Evidence**: PR #108 (finalize), specs/done/tui-slice/retrospective.md

### D-21 — 2026-10-04: Add card MH-18
- **Type**: card-add
- **Decision**: File backlog card MH-18
- **Rationale**: openssf-badge task 2: one card per Unmet passing criterion
- **Cards**: MH-18

### D-22 — 2026-10-04: Add card MH-19
- **Type**: card-add
- **Decision**: File backlog card MH-19
- **Rationale**: openssf-badge task 2: one card per Unmet passing criterion
- **Cards**: MH-19

### D-23 — 2026-10-04: Add card MH-20
- **Type**: card-add
- **Decision**: File backlog card MH-20
- **Rationale**: openssf-badge task 2: one card per Unmet passing criterion
- **Cards**: MH-20

### D-24 — 2026-10-04: Link MH-18/19/20 to issues #113/114/115
- **Type**: card-add
- **Decision**: MH-18/19/20 Issue fields and roadmap lines point to #113/114/115
- **Rationale**: The backlog add procedure creates a card's public issue after the card is approved and merged (PR #112), then records the link so contributors can find the discussion
- **Cards**: MH-18,MH-19,MH-20

### D-25 — 2026-10-05: MH-9 → shipped (openssf-badge)
- **Type**: lifecycle-sync
- **Decision**: MH-9 moved to shipped
- **Rationale**: openssf-badge finalized and merged
- **Cards**: MH-9
- **Evidence**: PR #120 (finalize), specs/done/openssf-badge/retrospective.md

### D-26 — 2026-10-05: Adopt the integrated v2 product destination and delivery stages
- **Type**: objective-change
- **Decision**: Resolve new product work to Master Specification v2 through docs/spec/README.md; preserve the original master, proposal and prompt. Adopt S0–S6, G01–G16 and the complete adaptive destination while retaining current-stage focus 1 without asserting passed gates.
- **Rationale**: The requested synthesis preserves explicit native/included-only/Herdr/open-source requirements, challenges both input designs, and separates a useful first delivery from evidence-gated adaptation. Essential local state is required; optional learning, backfill, exploration and telemetry are separate and off by default.
- **Cards**: MH-21,MH-22,MH-23,MH-24,MH-25,MH-26,MH-27,MH-28,MH-29,MH-30
- **Evidence**: mythhelm-synthesis/{MYTHHELM_Master_Spec_v2,MYTHHELM_Synthesis_Decisions,MYTHHELM_Implementation_Plan}.md; docs/spec/synthesis-adoption.md

### D-27 — 2026-10-05: Reconcile open cards with v2 without rewriting shipped history
- **Type**: strategic-adjustment
- **Decision**: Update all open card source contracts and map W01–W16 to explicit owners. Preserve MH-1/MH-2/MH-9 and earlier decision/signal entries unchanged; unfinished Revision 1.1 specs require reviewed reconciliation before implementation resumes.
- **Rationale**: Current code has per-run supervisors, schema v1 and fake/Claude seams; strict subscription-only blocks. Historical shipped slices and proposed walkthroughs cannot stand in for native qualification, human UX evidence or demonstrated learning gains.
- **Cards**: MH-3,MH-4,MH-5,MH-6,MH-7,MH-8,MH-10,MH-11,MH-12,MH-13,MH-14,MH-15,MH-16,MH-17,MH-18,MH-19,MH-20
- **Evidence**: Baseline eb349c2; internal/cli/dispatch.go; internal/admission/billing.go; internal/journal/migrations/0001_init.sql; ADR 0005; issues #72–#76, #106/#107

### D-28 — 2026-10-05: Prioritise first-route proof and separate first delivery from extensions
- **Type**: rescore
- **Decision**: MH-10 value 4→5 gives 5.0; MH-4 stage 2→1 gives 3.3; MH-16 stage 2→1 gives 3.3. MH-6 and MH-14 stage 2→3 each become 1.8. Other original value/risk/effort inputs remain unchanged.
- **Rationale**: First-route qualification is the core S0 blocker, and Codex must not be forced behind unproven Claude qualification. Basic quota envelopes are S1; public plugins and parallel construction are S3. Basic themes have their own S1 owner. Score is prioritisation, not dependency readiness.
- **Cards**: MH-4,MH-6,MH-10,MH-14,MH-16
- **Evidence**: v2 §§7, 10, 16, 18; W02/W04/W08/W09; scoring derived by pm.py at current stage 1

### D-29 — 2026-10-05: Add MH-21: Canonical v2 contracts and durable supervisor migration
- **Type**: card-add
- **Decision**: File MH-21 at stage 1 for the mapped v2 work, with no implementation or live-test authority.
- **Rationale**: W01 contract review precedes dispatch APIs; W03 can use fake adapters before live qualification. Value 5: S1 continuity; risk 5: duplicate launch, loss and stale authority; effort 5: cross-domain epic. Split implementation into reviewed lifecycle specs before task execution.
- **Cards**: MH-21
- **Evidence**: Master Specification v2 §§3–6; Implementation Plan W01/W03; ADRs 0003–0005 and 0007; Confirmed at eb349c2: internal/supervisor and ADR 0005 use per-run owners; internal/journal/migrations/0001_init.sql has no v2 task/policy tables. This is a migration, not an already delivered daemon.

### D-30 — 2026-10-05: Add MH-22: Protected acceptance and a complete v2 one-agent workflow
- **Type**: card-add
- **Decision**: File MH-22 at stage 1 for the mapped v2 work, with no implementation or live-test authority.
- **Rationale**: Requires the durable supervisor/contracts card, MH-10 and at least one independently qualified route, MH-13 and the S1 portion of MH-16. Value 5: accepted delivery; risk 5: false acceptance/authority; effort 4: extends multiple existing paths.
- **Cards**: MH-22
- **Evidence**: Master Specification v2 §§4, 8, 10–11, 15; Implementation Plan W04; issues #72–#76; Confirmed: internal/admission/billing.go blocks subscription-only; the current dogfood check/apply flow and schema v1 do not implement v2 TaskRevision or policy pins. Issues #72–#76 record remaining receipt and recovery gaps; implementation must re-check each issue before duplicating a fix.

### D-31 — 2026-10-05: Add MH-23: Complete TUI intervention, accessibility and safe declarative themes
- **Type**: card-add
- **Decision**: File MH-23 at stage 1 for the mapped v2 work, with no implementation or live-test authority.
- **Rationale**: Requires durable core control and the v2 one-agent workflow for complete journey acceptance; pairs with MH-3 for Herdr. Value 4: core usability; risk 4: misleading lifecycle/hidden controls; effort 4: interaction and human evidence.
- **Cards**: MH-23
- **Evidence**: Master Specification v2 §§14–16; Implementation Plan W05; issues #106/#107; specs/done/tui-slice/retrospective.md; Confirmed: MH-2 is shipped as the historical TUI slice; README and issues #106/#107 record exit/detach gaps and limited human accessibility evidence. The remaining work does not reopen or relabel MH-2.

### D-32 — 2026-10-05: Add MH-24: Scoped context, task dependencies and stale-evidence invalidation
- **Type**: card-add
- **Decision**: File MH-24 at stage 2 for the mapped v2 work, with no implementation or live-test authority.
- **Rationale**: Requires S1 and MH-15 continuity; bounded graph/context foundations precede MH-14 parallel writers. Value 5: reliable continuity; risk 4: leakage/stale acceptance; effort 4: canonical data, retrieval and invalidation.
- **Cards**: MH-24
- **Evidence**: Master Specification v2 §§4, 9–11; Implementation Plan W07; Confirmed: schema v1 records runs/attempts/candidates but no task revisions, dependency graph or scoped retrieval index. Native transcripts and Markdown views must not become a second task authority.

### D-33 — 2026-10-05: Add MH-25: Scoped export, retention, erasure and tested restoration
- **Type**: card-add
- **Decision**: File MH-25 at stage 2 for the mapped v2 work, with no implementation or live-test authority.
- **Rationale**: Requires durable migration and scoped context; must ship before optional-learning retention/deletion claims. Value 4: control and portability; risk 5: irreversible loss/leakage; effort 4: recovery and derived-state invalidation.
- **Cards**: MH-25
- **Evidence**: Master Specification v2 §§5, 8–9, 12; Implementation Plan W13; Confirmed: schema v1 and current CLI dispatch have no v2 scoped export/erasure lifecycle; append-only audit rules require an explicit maintenance path rather than ordinary event deletion.

### D-34 — 2026-10-05: Add MH-26: Consented outcome evidence and the fixed execution portfolio
- **Type**: card-add
- **Decision**: File MH-26 at stage 4 for the mapped v2 work, with no implementation or live-test authority.
- **Rationale**: Requires scoped context and export/erasure; MH-14 only for P3. Value 4: defensible policy choice; risk 4: biased metrics; effort 4: instrumentation and offline evaluation. Urgency derives from S4; no collection or live experiment is authorised by this card.
- **Cards**: MH-26
- **Evidence**: Master Specification v2 §§10, 12–13; Implementation Plan W10; Confirmed: the current core has no policy/experiment/outcome schema or evaluation runner. Vendor capability descriptions and synthesis walkthroughs are design evidence, not measured product superiority.

### D-35 — 2026-10-05: Add MH-27: Bounded protected experiments for routes, context and prompts
- **Type**: card-add
- **Decision**: File MH-27 at stage 4 for the mapped v2 work, with no implementation or live-test authority.
- **Rationale**: Requires fixed-portfolio outcome evidence and qualified variants; parallel variants additionally require MH-14. Value 4: useful evidence; risk 5: spending/contamination; effort 4: controlled allocation and statistics. No automatic promotion in this slice.
- **Cards**: MH-27
- **Evidence**: Master Specification v2 §§8, 10, 12–13; Implementation Plan W11; Confirmed: no experiment executor or learning policy tables exist at eb349c2. Reuse the canonical run/attempt machinery; do not introduce an independent executor.

### D-36 — 2026-10-05: Add MH-28: Scoped policy promotion, rollback and new-model qualification
- **Type**: card-add
- **Decision**: File MH-28 at stage 5 for the mapped v2 work, with no implementation or live-test authority.
- **Rationale**: Requires protected experiments, route qualification and promotion authority; MH-14 only for parallel claims. Value 5: full adaptive promise; risk 5: regressions/authority drift; effort 5: cross-domain epic. Learning can be disabled without losing fixed-policy delivery.
- **Cards**: MH-28
- **Evidence**: Master Specification v2 §§7, 12–13; Implementation Plan W12; Confirmed: adaptive promotion is new work, with no runtime implementation or live efficacy evidence in the current repository. Executable updates remain reviewed software releases.

### D-37 — 2026-10-05: Add MH-29: Remaining native harness qualification and platform coverage tracks
- **Type**: card-add
- **Decision**: File MH-29 at stage 2 for the mapped v2 work, with no implementation or live-test authority.
- **Rationale**: Requires MH-10 registry/contracts; individual tracks can qualify earlier when they unlock S1, with approved reprioritisation. MH-11 owns Windows process work, MH-3 Herdr, MH-7 release matrix. Value 4: seven-target/platform promise; risk 4: misleading support; effort 5: split by target before implementation. S2 is a continuing coverage track, not a promise all five ship in S2.
- **Cards**: MH-29
- **Evidence**: Master Specification v2 §§7, 14, 17; Implementation Plan W14; explicit UR-05/UR-06; Confirmed: internal/adapter contains claudecode and fake only; hosts/protocol/sdk packages are absent. No current native target has passed the full v2 included-only qualification contract.

### D-38 — 2026-10-05: Add MH-30: Optional verified production delivery with effect reconciliation
- **Type**: card-add
- **Decision**: File MH-30 at stage 6 for the mapped v2 work, with no implementation or live-test authority.
- **Rationale**: Depends on S1 and destination qualification, independently of S4/S5 learning. Value 4: completed software outcomes; risk 5: uncertain external effects; effort 4: destination-specific lifecycle. Stage 6 numbering must not force learning as a dependency.
- **Cards**: MH-30
- **Evidence**: Master Specification v2 §§8, 11, 17–18; Implementation Plan W15; Confirmed: the product CLI exposes no deployment command or qualified destination adapter. This is optional future application behavior, not authority to deploy during planning.

### D-39 — 2026-10-05: Preserve useful routing and profile signals as scoped hypotheses
- **Type**: signal-triage
- **Decision**: Retain S-2/S-3/S-4 as historical signals. Cache-aware ordering may operate within native semantics; cheap-first/kNN routing requires comparison evidence; account profiles never mean keepalive traffic, secret copying or identity rotation.
- **Rationale**: The integrated product preserves deterministic pins and fixed P1, scoped learning consent and independent billing qualification. Signals inform experiments and cannot override these requirements.
- **Cards**: MH-5,MH-16,MH-17,MH-26,MH-27
- **Evidence**: product/signals.md S-2–S-4; v2 §§7, 9–10, 12–13

### D-40 — 2026-10-05: Link the v2 adoption cards to their public issues
- **Type**: card-add
- **Decision**: Link MH-21–MH-30 to issues #127–#136 and regenerate the roadmap without changing scope, scores or lifecycle status.
- **Rationale**: The backlog procedure creates public issues after the maintainer approves and the agent commits the cards; these links complete that required second step.
- **Cards**: MH-21,MH-22,MH-23,MH-24,MH-25,MH-26,MH-27,MH-28,MH-29,MH-30
- **Evidence**: Approved product adoption commit 6a837d3; PR #126; public issues #127–#136.

### D-41 — 2026-10-05: Clarify lifecycle qualification and roadmap eligibility
- **Type**: strategic-adjustment
- **Decision**: Add G04 to MH-4 so its listed gates include the already required stop/recovery evidence. Regenerate the roadmap with wording that includes earlier stages, matching the existing selection rule.
- **Rationale**: PR review identified two metadata omissions; the corrections change no card scope, score, stage, status, issue link or selection behaviour.
- **Cards**: MH-4
- **Evidence**: PR #126 review: https://github.com/turbokast/mythhelm/pull/126#discussion_r4182195654 and https://github.com/turbokast/mythhelm/pull/126#discussion_r4182195664; v2 G04; pm.py urgency_for and render_roadmap.

### D-42 — 2026-10-05: Add card MH-32
- **Type**: card-add
- **Decision**: File backlog card MH-32
- **Rationale**: docs-site-demos task 7: FR-3 follow-up recording for the shipped TUI
- **Cards**: MH-32

### D-43 — 2026-10-05: MH-8 → shipped (docs-site-demos)
- **Type**: lifecycle-sync
- **Decision**: MH-8 moved to shipped
- **Rationale**: docs-site-demos finalized and merged
- **Cards**: MH-8
- **Evidence**: PR #148 (a1ae317); retrospective in specs/done/docs-site-demos/

### D-44 — 2026-10-05: MH-32 → specced (tui-tape-recording)
- **Type**: lifecycle-sync
- **Decision**: MH-32 moved to specced
- **Rationale**: tui-tape-recording reached unrefined with grounded requirements
- **Cards**: MH-32
- **Evidence**: specs/unrefined/tui-tape-recording/requirements.md

### D-45 — 2026-10-05: MH-20 → specced (strict-lint-set)
- **Type**: lifecycle-sync
- **Decision**: MH-20 moved to specced
- **Rationale**: strict-lint-set reached unrefined with grounded requirements
- **Cards**: MH-20
- **Evidence**: specs/unrefined/strict-lint-set/requirements.md

### D-46 — 2026-10-05: MH-18 → specced (version-numbering)
- **Type**: lifecycle-sync
- **Decision**: MH-18 moved to specced
- **Rationale**: version-numbering reached unrefined with grounded requirements
- **Cards**: MH-18
- **Evidence**: specs/unrefined/version-numbering/requirements.md

### D-47 — 2026-10-05: MH-19 → specced (release-tagging)
- **Type**: lifecycle-sync
- **Decision**: MH-19 moved to specced
- **Rationale**: release-tagging reached unrefined with grounded requirements
- **Cards**: MH-19
- **Evidence**: specs/unrefined/release-tagging/requirements.md

### D-48 — 2026-10-05: MH-8 Notes correction (docs-site-demos shipped record)
- **Type**: lifecycle-sync
- **Decision**: MH-8 Notes replaced with the shipped-state record
- **Rationale**: The shipped sync left pre-ship guidance (remains specced against R1.1); the spec is done and reconciled (PR #146), so the card now records the shipped state and the MH-32 follow-up
- **Cards**: MH-8
- **Evidence**: PR #149 (176fead); specs/done/docs-site-demos/reconciliation.md

### D-51 — 2026-10-06: MH-32 → shipped (tui-tape-recording)
- **Type**: lifecycle-sync
- **Decision**: MH-32 moved to shipped
- **Rationale**: tui-tape-recording reached done
- **Cards**: MH-32
- **Evidence**: PR #187 (finalize, merged 08daa78); main green at 08daa78
