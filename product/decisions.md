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

### D-6 — 2026-10-02: MH-1 → shipped (dogfood-slice)
- **Type**: lifecycle-sync
- **Decision**: MH-1 moved to shipped
- **Rationale**: dogfood-slice reached done; finalize merged, main CI green
- **Cards**: MH-1
- **Evidence**: PR #78 (finalize, b0fcd81); main CI success at b0fcd81
