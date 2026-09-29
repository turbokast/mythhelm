# Evidence: teeth-discipline

Record for `.claude/rules/teeth-discipline.md`.

## Hazard

A check that has never been seen to fail may be unable to fail. The common ways:

- the probe is refused whether or not the change under test is present, so it never exercises the path it names;
- the test observes a library default while production is configured differently;
- it watches a different process, file or row from the one the behaviour changes, and "confirms absence" immediately;
- a fake's message shape misses the branch the test names, so deleting the guard leaves the suite green;
- the asserted predicate is also true for the correct behaviour.

Absence assertions are the worst case. A presence check that observes nothing fails loudly; an absence check that observes nothing passes, and an instant pass looks exactly like a fast success.

A red-first note recorded once (in a scratchpad, a commit message or a review thread) proves the check bit on one commit. Nothing re-checks it, and a later refactor can make the check vacuous without anyone noticing.

## Mechanism

A kept broken input turns vacuity into a failing test: if a change makes the check stop biting, the check's own suite goes red. The harness scripts follow the pattern: each check in `scripts/ci/lint-agent-harness.sh` is driven by a broken fixture in `scripts/ci/tests/test_lint_agent_harness.sh` that must fail, beside a clean control that must pass.

The practice is known as the teeth discipline in work on falsifiable release gates: for each invariant a deliberately broken variant is added, and the checker is asserted to catch it.

## Instances

None recorded in this repository yet.

## Loosening criteria

A check whose broken input genuinely cannot be constructed is kept only under the rule's `cannot-be-falsified:` marker; that is the only carve-out.
