# Evidence: spec-authoring

Record for `.claude/rules/spec-authoring.md`.

## Hazard

A spec is written once and then re-read by every task that implements it and every review that judges it. Its defects compound instead of staying local:

- An acceptance criterion with no observable ("works correctly", "tests pass") lets an implementer declare done with nothing verified, and a reviewer has nothing to mark as failed.
- An unnumbered or renumbered requirement breaks every reference to it: task acceptance items, completion entries, review findings.
- A requirement that does not say which invariant it serves cannot be checked for coverage. A slice that quietly drops an invariant looks identical to one that keeps it.
- A non-goal without a "still binding" note reads as permission to ignore the invariant in the part that does ship.
- A design that lists only the chosen option hides the alternatives, so the next spec re-argues them; one with no honesty register presents a partial implementation as a full one.
- A spec that cites its own lifecycle path (`specs/todo/<name>/…`) is wrong after the next normal lifecycle move, and a task whose command uses that path fails on its first run.
- A line-number citation into a file the same spec edits drifts with the first insertion above it, and a literal-minded task then edits the wrong lines.
- A task whose files span two domains lands with an agent that owns only one of them, which then either edits outside its domain or stops.

## Mechanism

The rule fixes the shape: numbered IDs, tagged criteria in EARS form, a decisions table and an honesty register, and the task-block fields. `scripts/ci/lint-agent-harness.sh` (check `specs`) enforces the machine-checkable part: required files per lifecycle state, the task fields, dependency references and cycles, epic work streams and the single-copy rule, each with a broken fixture in `scripts/ci/tests/test_lint_agent_harness.sh`. Criterion quality, invariant tagging and the honesty register are judged by the fresh-context validator in `/spec-validate`; they are prose-enforced.

The format follows the first spec written in this repository, `specs/*/dogfood-slice/`, which uses every element the rule names.

## Instances

None recorded in this repository yet.

## Loosening criteria

Drop a required element only when three specs in a row show it adding no information a reviewer used, each linked here.
