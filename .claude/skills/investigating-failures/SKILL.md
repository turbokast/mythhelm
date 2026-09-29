---
name: investigating-failures
description: Diagnose before fixing — use on any test failure, build or vet error, lint finding, race report, CI failure or runtime crash during implementation
argument-hint: "[failing command or test name]"
---

# Investigating Failures

When something breaks, understand the failure before changing code. Speculative edits cause cascading damage: each one changes the state the next hypothesis is tested against.

## Input

`$ARGUMENTS`: optional. The failing command, test name or CI job.

## Invocation contexts

- **Slash command**: runs the loop and reports.
- **Model-invoked**: the same, whenever a gate or test fails.
- **Non-interactive**: the same; the "When to stop" report (an escalation step) is returned to the dispatcher instead of asking.

## Steps

### The loop

```text
READ THE ERROR -> FORM ONE HYPOTHESIS -> VERIFY IT -> MAKE ONE CHANGE -> RE-RUN
```

1. **Read** the full output: the first error, not the last; the whole stack or race report. Re-run the single failing test in isolation (`go test -run '^TestName$' ./pkg/...`) to get a clean signal.
2. **Hypothesise** one mechanism that explains every symptom.
3. **Verify** it with a cheap check before editing: a print, a narrower test, or the same test run against `origin/main` in a separate worktree (`git worktree add /tmp/<dir> origin/main`; never `git stash`).
4. **Change one thing**, re-run, and revert it if it did not fix the failure before trying the next hypothesis.

### Decision trees

**Build or vet error**

- In your code: read the line; check names, types, imports and build tags.
- In a package you did not touch: your change broke a caller. Search for uses of what you changed (`grep -rn 'Name(' --include='*.go'`) and fix the callers, or revert the interface change.
- Only on one OS in CI: a platform file is missing its counterpart or a build constraint is wrong. Reproduce with `GOOS=<os> go vet ./...`.

**Test assertion failure**

- A test you just wrote failing as expected: that is red; go to green.
- An existing test: check whether it tests intended behaviour against the spec's acceptance criteria. If yes, the code is wrong: fix the code. If the behaviour changed on purpose, update the test and record why in the completion entry.
- Never weaken an assertion (`== 42` to `!= 0`) to pass.

**Timeout or hang**

- A goroutine waiting on a channel nobody sends to, a missing `cancel()`, a child process whose pipe is never drained, a lock taken twice. Run with `-timeout 30s` to get the goroutine dump and read it.
- Never raise the timeout as the first fix: a timeout is a structural problem.

**Race detector report**

- Read both stacks: the write and the conflicting access. Find the shared variable and decide which single mechanism guards it (a mutex, a channel, confinement to one goroutine). Never silence the detector.

**Flaky failure**

- Follow `.claude/rules/agent-behavioral-posture.md` §7: record failing and passing values; a constant failing value is a structural quantum, not load.

**Lint finding** (`go vet`, `golangci-lint`, `shellcheck`)

- Read the rule's documentation and fix the code. A `//nolint` or `# shellcheck disable` needs the specific rule and a reason on the same line.

**CI-only failure**

- Read the job log (`gh run view <id> --log-failed`). Compare the environment: OS, Go version, `-race`, working directory, file permissions, path separators, line endings. Reproduce the difference locally before changing code.

**Runtime crash**

- Read the full stack; identify the function and its inputs. A nil dereference or index out of range means data of an unexpected shape reached code that trusted it: fix the boundary that let it in, not the symptom.

### Anti-patterns

| Anti-pattern | Instead |
|---|---|
| Several changes at once | One change, one re-run |
| Weakening an assertion | Make the code produce the expected value |
| Recovering a panic to hide it | Fix the input or the caller |
| Raising a timeout | Find the blocked goroutine |
| Assuming the test is wrong | Check the acceptance criteria first |
| A fourth guess after three failures | Stop and report |

### When to stop

After three honest attempts on the same failure (escalation step):

1. Stop changing code.
2. Report: the failure (quoted), each attempt and its result, the current hypothesis, the files modified, and what evidence would distinguish the remaining hypotheses.
3. Ask for guidance; in a non-interactive run, return the report to the dispatcher and stop.

## Output

Either the fix with the re-run output that shows it passing, or the "When to stop" report.
