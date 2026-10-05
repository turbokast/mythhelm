---
name: roadmap
description: Redraft product/roadmap.md (now, next, later) from the backlog and file it for maintainer approval, or report whether the published roadmap is current
argument-hint: "[--check]"
---

# Roadmap

`product/roadmap.md` is a view of the backlog, never a separate plan: `scripts/pm/pm.py roadmap` generates it from the cards and the current stage.

- **Now**: cards with a spec (`specced`, `implementing`).
- **Next**: the five highest-scored `idea` or `triaged` cards in an earlier stage, the current stage or the next stage.
- **Later**: every other open card, by score.

Because it is generated, the roadmap carries no decisions of its own: changing what is on it means changing the cards (`/backlog`), and this skill only redrafts the view.

## Input

`$ARGUMENTS`: empty to redraft, or `--check` to report only.

## Invocation contexts

- **Slash command**: redrafts, shows the difference and files the request.
- **Model-invoked**: the same.
- **Non-interactive**: the same. Filing the request is not a product write (§File a change in `.claude/skills/pm-sync-core/SKILL.md`), so it needs no pre-approval; the maintainer signs it later.

## Steps

1. **Check.** `python3 scripts/pm/pm.py validate`. A warning that `product/roadmap.md` is stale means the backlog changed since the roadmap was drafted. With `--check`, report that and stop.
2. **Draft.** Stage the product files and generate the roadmap: `STAGE="$(mktemp -d)"; python3 scripts/pm/pm.py stage "$STAGE"; python3 scripts/pm/pm.py --product-dir "$STAGE" roadmap --out "$STAGE/roadmap.md"`.
3. **Compare.** `diff -u product/roadmap.md "$STAGE/roadmap.md"`. No difference: report that the roadmap is current and stop.
4. **Present** the changes in terms of cards: which moved into Now, Next or Later, and the card change that moved each one (a status flip, a rescore, a new card).
5. **File** it: `python3 scripts/orchestration/approvals.py request roadmap-<date> --path product/roadmap.md --proposed "$STAGE/roadmap.md" --summary "Redraft the roadmap after <cause>"`, and hand off per §File a change step 8. No decision entry: the roadmap records no decision.

## Output

```text
Roadmap: current | stale → request roadmap-<date>
Now: MH-<n>, …   Next: MH-<n>, …   Later: <count> cards
Changes: <card> moved <from> → <to> because <cause>
```
