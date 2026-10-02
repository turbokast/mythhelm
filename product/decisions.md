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
