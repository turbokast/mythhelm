---
name: finalize-spec-epic-rollup
description: When the spec being finalized is the last open work stream of an epic, move the epic's plan directory to done in the same finalize pull request; a no-op for a standalone spec or an unfinished epic, and a block when a decided rollup cannot move
argument-hint: "<spec-name>"
---

# Finalize Spec — Epic Rollup

Run by `/finalize-spec-publish` Step 2, in the finalize worktree, after the spec itself has moved to `specs/done/`. An epic's `plan.md` directory leaves `in-progress/` when its last sub-spec ships, and it rides the same pull request as that sub-spec, so `main` never shows a finished epic still in progress.

## Input

`$ARGUMENTS`: the name of the spec just moved to `done/`.

## Invocation contexts

- **Slash command**: decides and, on a rollup, moves the epic directory in the working tree it runs in; it stages the move and commits nothing.
- **Model-invoked**: the same.
- **Non-interactive**: the same. The decision is mechanical; there is no question to ask.

## Steps

1. Decide:

   ```bash
   python3 scripts/harness/finalize.py epic --spec <spec>
   ```

   It reads every epic `plan.md` (a directory with `plan.md` and no `requirements.md`) through the same Work Streams parser the `specs` lint uses, and prints one line:

   | Line | Meaning |
   |---|---|
   | `NOOP no-parent` | no epic names this spec |
   | `NOOP pending-sibling:<a,b>` | the epic still has work streams outside `done/` and `archived/` |
   | `NOOP already-done` | the epic is already in `done/` |
   | `ROLLUP <epic> <state>` | every work stream is in `done/` or `archived/`; move the epic |

   Exit 2 is a refusal, not a decision: the spec is not (only) in `done/` yet, or two epics claim it. Report it and stop; never pick one.

2. On `ROLLUP`: `scripts/harness/spec-lifecycle.sh move <epic> done`. It allows an epic to reach `done/` only from `in-progress/`. A refusal (an epic still in `unrefined/` or `refined/`, or a directory already at the destination) is a **block**: report its stanza and stop `/finalize-spec-publish`, since a decided rollup that cannot happen means the epic's lifecycle is out of step with its work streams and a maintainer must decide.

3. Report the epic name for `finalize.py publish-check --epic <epic>`, whose scope check otherwise refuses the epic's paths.

## Output

`Epic rollup: specs/<state>/<epic> -> specs/done/<epic>`, `Epic rollup: no-op (<reason>)`, or the block stanza.
