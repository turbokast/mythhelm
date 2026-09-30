# Evidence: proposals-and-evals

Record for `.claude/rules/proposals-and-evals.md`.

## Hazard

A harness that proposes changes to itself can also apply them to itself. Three things go wrong when the apply side is loose:

- **Consent by batch.** "Approve all" reads as a decision on every item, including those nobody read, so a rule can change with no one having decided it.
- **Shared ledgers.** Proposal and decision files are edited by several sessions. Committing them whole, or rewriting them, carries a peer's in-flight edits into the wrong commit or drops them.
- **Fixes that do not stick.** A prose fix is not re-read at the moment it matters, and nothing notices when a later edit undoes it. An eval case that already passed before the fix, or whose glob matches nothing, pins nothing and still looks green.

The lane-0 shortcut adds two more: an applier that pastes the whole proposal block into a curated file, and a switch that the change being applied could flip.

## Mechanism

- `/apply-proposals` asks per item; `skill-invocation-contexts.md` makes a blanket confirmation insufficient for a per-item gate.
- `scripts/harness/proposals.py record` is the only writer of `applied.md`; `apply-check` refuses a change to its existing text, a file outside the decision's scope, and an eval case that does not fail at the base and pass at the head; `pr-check` repeats the scope and entry checks on the pull request.
- The `proposals` lint validates `applied.md` and `auto-apply.json`; the `evals` lint runs every case in CI, and each target glob must match a file.
- Lane 0 appends only the proposed change body, refuses bodies carrying proposal headings or fields, and reads its configuration at the base.

## Instances

None recorded in this repository yet.

## Loosening criteria

Allowing an unpinned rule, skill or hook change needs a class of change that repeatedly has no behaviour an eval can express, shown by three recorded waivers for the same reason.
