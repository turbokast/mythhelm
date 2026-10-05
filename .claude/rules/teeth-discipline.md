---
paths:
  - "**/*_test.go"
  # future: adapter fixtures land under testdata/
  - "**/testdata/**"
  - "scripts/**"
  - ".claude/hooks/**"
  - "specs/**"
  - "knowledge/invariants.md"
---

# Every Check Ships With the Broken Input That Proves It Bites

A test, lint, hook or CI check that exists to catch a defect is unverified until it has been seen to fail.

- **Keep the broken input in the suite.** Ship the check with a deliberately broken case asserted to be rejected: a second test case with the precondition inverted, a fixture the lint must flag, a payload the hook must block. A one-time red-first note in a scratchpad or commit message proves the check bit once; the kept case keeps proving it.
- **Absence assertions need a red-first proof.** A check that passes when something did not happen (a refusal, no event, no file, a timeout expiring, an empty field) passes by default when it observes nothing. Violate the condition on purpose, watch the check go red, restore it, watch it go green, and keep the violating case where it can be.
- **Check the shapes that stay green by construction:** the probe is refused with or without the change under test; the test observes a default rather than the configured value; it observes a different process, file or row from the one that changes; the input never reaches the branch the title names; the predicate is also true for the correct behaviour.
- **A check that cannot be driven red.** An absence assertion that cannot be driven red is deleted or replaced in the same change: a permanently green absence check retires a reviewer's suspicion and is worse than none. Any other check with no constructible failing input stays only when marked `cannot-be-falsified: <reason and the seam that would be needed>` at the assertion, and is never reported as a gate.

## Invariant evidence

The normative invariant table and its rationale are in `knowledge/invariants.md`.

- **In `tasks.md`.** List every invariant the task can affect under `Invariants touched`, by ID with the spec section it relies on: `I09 (v2 §7.3: billing evidence stays typed and labelled)`. Name what the task does to keep it, not the invariant's title.
- **In tests.** Name the test after the behaviour that keeps the invariant, and put the version-qualified ID in a comment beside the assertion or in the test name: `TestAdmissionBlocksUnknownBilling // I02 (v2 §2)`. The test must fail when the invariant is broken.
- **In a review.** Cite the version-qualified invariant ID and the `file:line` that breaks it. Treat a plausible-but-unverified risk as a question, not a finding.
- **In code.** Cite an invariant only where the code would otherwise look wrong: a deliberately fail-closed branch, a refusal that looks over-cautious, or an extra reconciliation step.
- **When a task cannot keep one.** Stop and escalate. Record the gap in the spec's honesty register; an implementer never decides to weaken an invariant alone.

Evidence: `knowledge/rule-evidence/teeth-discipline.md`.
