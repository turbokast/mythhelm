---
name: finalize-spec-pm
description: Move a finalized spec's backlog cards to shipped through the pm-sync-core approval flow once its record is merged and main is green, or report that PM sync is pending when the product layer is not installed
argument-hint: "<spec-name>"
---

# Finalize Spec — Backlog Sync

Step 6 of `/finalize-spec`. The backlog in `product/` records what shipped. Agents never write it directly: `pm-sync-core` drafts the change and files it for a maintainer's signed approval.

## Input

`$ARGUMENTS`: the spec name. The spec is in `specs/done/` on origin/main, and `/finalize-spec-verify-ci` reported green.

## Invocation contexts

- **Slash command**: invokes `pm-sync-core` and reports its lines, including the maintainer's approve commands.
- **Model-invoked**: the same.
- **Non-interactive**: the same. `pm-sync-core` only files approval requests, which need no pre-approval; the backlog itself changes only when a maintainer approves. This skill never waits for that approval.

## Steps

1. **Is the product layer installed?** In the main checkout, `test -f .claude/skills/pm-sync-core/SKILL.md && test -f scripts/pm/pm.py`. When either is missing, report `PM sync: pending (pm-sync-core is not installed)` and stop without error: the finalize is complete, and the card is synced when the layer lands.
2. **Sync.** Invoke `pm-sync-core` with the Skill tool, `args="<spec> --to shipped"`. It finds the spec's cards, holds any card that names another unshipped spec at `implementing` with a half-shipped note, and files one approval request per changed product file.
3. **Relay** its report lines verbatim, and the request ids with the maintainer's commands. A `dropped` card, or any escalation `pm-sync-core` raises, is reported as its own line and does not undo the finalize.

## Output

`pm-sync-core`'s lines (`PM sync: MH-<n> → shipped (requests <ids>)`, `... skipped ...`, `... held at implementing ...`), or `PM sync: skipped (no card names <spec>)`, or `PM sync: pending (<reason>)`.
