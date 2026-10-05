---
name: quarterly-review
description: A lightweight quarterly check of the product layer — stage progress against the exit gates, backlog health, signals and delivery throughput — ending in recommendations the maintainer accepts one by one as decisions
argument-hint: "[YYYY-QN]"
---

# Quarterly Review

A short, evidence-first look back over a calendar quarter: is the current stage converging on its exit gate, is the backlog healthy, what are users saying, and what shipped. It produces a report and a few recommendations; each one the maintainer accepts becomes a `strategic-adjustment` decision.

## Input

`$ARGUMENTS`: the quarter as `YYYY-QN`; default the quarter that ended most recently (Q1 is January to March).

## Invocation contexts

- **Slash command**: generates the report, then asks about each recommendation separately (a per-item write gate) and files the accepted ones.
- **Model-invoked**: the same.
- **Non-interactive**: generates the report and returns the recommendations unfiled. A blanket pre-approval does not cover them: only recommendations the dispatching prompt accepts by number are filed as decisions.

## Steps

1. **Read** `product/objectives.md` (current stage and its exit gate), `python3 scripts/pm/pm.py list --json`, `product/decisions.md` and `product/signals.md` entries dated in the quarter, the specs that reached `specs/done/` in it (`git log --diff-filter=A --since=<start> --until=<end> --name-only -- specs/done/`), and `python3 scripts/pm/pm.py validate`.
2. **Stage progress.** For each gate of the current stage's exit gate, list the open cards that name it and their statuses. A gate with no card is a gap to flag; a gate whose cards are all shipped still needs the named revision's evidence (v2 §18.3; historical Revision 1.1 §18.7) before it counts as passing. Resolve current authority through `docs/spec/README.md`; historical evidence does not automatically pass a clarified v2 gate.
3. **Backlog health.** Counts by status; `idea` cards older than the quarter that were never triaged; open cards without an issue; cards whose score inputs predate a signal or impact review that bears on them; the spread of scores in the top ten.
4. **Signals.** The quarter's themes and whether each led to a rescore, a card or a stated no-action.
5. **Throughput.** Specs finished, cards shipped, `impact-review` conclusions in the quarter, and CI health on `main` (`gh run list -R turbokast/mythhelm --branch main --workflow ci.yml --created <start>..<end> --limit 200 --json conclusion`).
6. **Recommend** two to five specific adjustments, each citing its evidence: a rescore, a card to add or drop, a gate with no owner card, a stage advance when the exit gate passes (an `objective-change`, which also needs `/backlog rescore`). Say so plainly when nothing needs adjusting.
7. **Decide one by one.** For each accepted recommendation, file a `strategic-adjustment` decision through §File a change (`.claude/skills/pm-sync-core/SKILL.md`); the adjustment itself (a rescore, a new card) then goes through `/backlog`. Rejected recommendations are not recorded.

## Output

```markdown
# Quarterly review: <YYYY-QN>

## Stage <n> progress
## Backlog health
## Signals
## Throughput
## Recommendations
1. <adjustment> — evidence: <links, cards, decisions>

Filed: <request ids for accepted recommendations, or none>
```
