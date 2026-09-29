# Evidence: test-quality

Record for `.claude/rules/test-quality.md`.

## Hazard

- **Sleep-based synchronisation** makes a test either slow or flaky, and usually both: the sleep is tuned on one machine and fails on a loaded CI runner or a slower OS.
- **Trivially true and mock-call-only assertions** raise the test count without covering behaviour, and they retire a reviewer's suspicion that the path is untested.
- **Leaked process state** (environment variables, package-level variables, the working directory) produces failures that depend on test order and parallelism, which look like flakes and are misdiagnosed as such.
- **Tagged tests** (`live`, `e2e`) do not run under the default `go test ./...`, so a change can be green on every default run while the tagged tier is broken.
- **Fixture fixes that hide production bugs.** When a test goes from red to green by changing its fixture rather than the code, the most common reason is that the fixture now matches the broken behaviour. Without the "why is production not broken" question, the green suite becomes the final word and nothing downstream re-asks it.
- **Invariant brackets.** A test that asserts the same predicate before and after an event cannot fail if the predicate is invariant across the event.

## Mechanism

Prose, applied when writing tests, and checked by `code-reviewer`. CI runs `go test -race` on Linux, macOS and Windows, which exposes data races and platform dependence. It does not randomise test order (that needs `go test -shuffle=on`), and neither exposes vacuity.

## Instances

None recorded in this repository yet.

## Loosening criteria

A banned pattern may gain a carve-out when three recorded cases show the ban forced a worse test.
