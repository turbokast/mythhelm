---
paths:
  - "**/*_test.go"
  # future: adapter fixtures land under testdata/
  - "**/testdata/**"
---

# Test Quality

Fast, deterministic tests that assert behaviour. `code-reviewer` checks the same list.

## Banned

- **Sleeping to synchronise.** No `time.Sleep` to wait for a goroutine, process or file. Wait on the signal itself (a channel, `sync.WaitGroup`, a process exit, a polled condition with a deadline), or inject a clock. A sleep is allowed only inside a bounded poll loop.
- **Trivially true assertions.** `len(x) >= 0`, `err == nil` as the only check, "did not panic". Assert the specific value.
- **Testing the standard library or a dependency.** Test your code's use of it.
- **Mock-call-only tests.** A fake that records calls, with no assertion on the output or side effect, proves wiring, not behaviour.
- **Production code that exists for tests.** No exported `ResetForTest` in a production package. A test-only seam is unexported, or guarded by `testing.Testing()`, and rejected outside tests.
- **Leaking state.** Never mutate package-level variables or the process environment without `t.Setenv` or a `t.Cleanup` that restores them. `t.Setenv` and `t.Chdir` forbid `t.Parallel()`; a test using them says so by not being parallel.

## Required

- **Table-driven** for input variations, each case named for what it proves: `{name: "unknown billing blocks admission", ...}`.
- **Isolated.** `t.TempDir()` for files, a fresh git repository per test, a fresh SQLite file per test. Never touch the real state directory or `$HOME`.
- **Parallel by default** (`t.Parallel()`) unless the test changes process-wide state, with a comment naming it.
- **Helpers call `t.Helper()`**; use `t.Fatalf` when later lines depend on the result, `t.Errorf` otherwise.
- **Deterministic.** Seed or inject randomness and time. A test that depends on wall-clock order or map iteration order is broken.
- **Cross-platform.** Build paths with `filepath`, compare with `filepath.ToSlash` where output is normalised, and gate OS-specific tests with build tags or `runtime.GOOS` skips that say why.
- **Sanitised recordings.** A recorded native stream committed as a fixture carries no account IDs, emails, org names, paths or prompts (`public-repo-hygiene.md`).
- **External behaviour through the binary.** CLI output and exit codes, JSONL contracts and files written are tested through the built binary or `cli.Main`, not only through internal functions.
- **Tags are gates too.** Tests behind a build tag (`live`, `e2e`) do not run under the default `go test ./...`. A change that can break them runs them explicitly, and a green default run is never evidence about them.

## A test change that turns red green justifies production

When a change to a fixture, golden file, fake or test setup (not the code under test) makes a failing test pass, the change says why production is not also broken:

- the fixture misrepresented something production really produces (name the producer at `file:line`), or
- the assertion tested something the feature never promised (cite the requirement).

"The suite is green now" is not an answer. If you cannot give one, you found a production defect: stop and report it.

**Invariant brackets.** When a test asserts the same predicate before and after an event, it can fail only if the event can change the subject. Show that it can: keep a case where the event does change it, and watch the after-assertion go red (`teeth-discipline.md`). Otherwise make the two assertions differ in subject, expected value or a monotonic counter.

Evidence: `knowledge/rule-evidence/test-quality.md`.
