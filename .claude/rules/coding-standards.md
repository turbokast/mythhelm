# Coding Standards

## Style

- Write the direct, self-evident solution with the least abstraction. Less code is better code.
- Follow the existing style of the file and package, even where you would choose differently.
- Keep structure flat: return early, avoid deep nesting.
- Prefer composition, small interfaces defined by the consumer, and pure functions.
- Do not repeat yourself, but never abstract before the second real use.

## Errors

- Fail fast on true errors. Never swallow an error or replace it with a default to keep going.
- No defensive checks for inputs that cannot occur; validate at trust boundaries (user input, files, native agent output, plugin messages) and trust the types inside.
- Unknown is never allowed: a missing, unparseable or unrecognised value in admission, billing, ownership or permission code blocks (invariant I02).

## Go

Details load from `go-conventions.md` when you edit Go.

- Wrap errors with context using `fmt.Errorf("...: %w", err)`; compare with `errors.Is` and `errors.As`, never with strings.
- `context.Context` is the first parameter of any function that blocks, does I/O or starts work; never store one in a struct.
- No `panic` in library code; return an error. Only `main` and a worker's top-level recovery may handle a panic.
- No mutable package-level state. Pass dependencies explicitly; tests must be able to run in parallel.
- Table-driven tests with named cases; `t.Helper()` in helpers; `t.TempDir()` and `t.Setenv()` for isolation.

## Testing

- Test behaviour, not implementation. Fake only what crosses the process boundary: native agents, the network, the clock where it matters. Use real files, real git and real SQLite in temporary directories.
- A test that only asserts liveness or non-failure (`err == nil` and nothing else, a process is still running, a value is non-nil) does not cover the behaviour in its title. Assert the observable the behaviour produces: the value, the event, the file, the exit code. If there is none, say so in the test name or drop the test.
- An assertion that accepts every branch cannot fail: pin the expected branch. A broken input that exercises data the code never reads proves nothing: show it red against the unfixed code.
- External behaviour (CLI output and exit codes, JSONL contracts, files written) is tested through the built binary or `cli.Main`, not only through internal functions.

## Comments and documentation

- No comments unless the logic is non-obvious or an edge case needs explaining. Names and structure carry the meaning.
- Delete dead code. Replace outdated documentation with accurate documentation in the same change.

## Logging

- Log what helps diagnose a failure: identifiers, counts, states and decisions. Never log prompts, source text, tokens or credentials (spec §12.7).
