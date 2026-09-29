---
paths:
  - "**/*_test.go"
  - "scripts/**/tests/**"
  - ".claude/hooks/tests/**"
---

# Red First, for the Right Reason

## Every change starts from a failing test

- A new behaviour starts with a test that fails because the behaviour is missing: an assertion failure, not a compile error, a setup crash or a timeout. For a new function, add a stub returning the wrong value and watch the assertion fail.
- **Every bug fix starts with a regression test that reproduces the bug** and fails against the unfixed code. The fix makes it pass, and it stays in the suite.
- A test that passes on first run tests existing behaviour. Fix the test before writing code.

## Rule out passing for the wrong reason

Before recording a red-first result, answer each question for the new test. A test that fails one is rewritten before the task is complete.

1. **Would it pass against a plausible wrong fix?** Name the shortcut a reviewer would try, run the test against it, and watch it go red.
2. **Is the asserted predicate true in both the broken and the fixed state?** Assert the value that changes, on the object that changes: never a whole-output substring another line can satisfy.
3. **Is the broken input kept?** A note about a reverted mutant is not a kept case; keep an inverted-precondition case in the suite (`teeth-discipline.md`).
4. **Does the input reach the branch the title names?** Show that it flows through the code under test, not around it.

Record the mutants in the completion entry or the pull request: `Red-first: <mutant> -> <test> FAIL; restored -> PASS`.
