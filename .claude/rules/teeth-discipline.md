---
paths:
  - "**/*_test.go"
  # future: adapter fixtures land under testdata/
  - "**/testdata/**"
  - "scripts/**"
  - ".claude/hooks/**"
  - "specs/**"
---

# Every Check Ships With the Broken Input That Proves It Bites

A test, lint, hook or CI check that exists to catch a defect is unverified until it has been seen to fail.

- **Keep the broken input in the suite.** Ship the check with a deliberately broken case asserted to be rejected: a second test case with the precondition inverted, a fixture the lint must flag, a payload the hook must block. A one-time red-first note in a scratchpad or commit message proves the check bit once; the kept case keeps proving it.
- **Absence assertions need a red-first proof.** A check that passes when something did not happen (a refusal, no event, no file, a timeout expiring, an empty field) passes by default when it observes nothing. Violate the condition on purpose, watch the check go red, restore it, watch it go green, and keep the violating case where it can be.
- **Check the shapes that stay green by construction:** the probe is refused with or without the change under test; the test observes a default rather than the configured value; it observes a different process, file or row from the one that changes; the input never reaches the branch the title names; the predicate is also true for the correct behaviour.
- **A check that cannot be driven red.** An absence assertion that cannot be driven red is deleted or replaced in the same change: a permanently green absence check retires a reviewer's suspicion and is worse than none. Any other check with no constructible failing input stays only when marked `cannot-be-falsified: <reason and the seam that would be needed>` at the assertion, and is never reported as a gate.

Evidence: `knowledge/rule-evidence/teeth-discipline.md`.
