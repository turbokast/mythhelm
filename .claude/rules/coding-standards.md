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
- Unknown mandatory admission, entitlement, ownership or authority evidence blocks (v2 I02). Unknown remaining quota alone need not block an independently qualified stop-at-exhaustion route; never map it to zero or an invented allowance.

## Go

The Go rules load from `go-conventions.md` and `test-quality.md` when you read Go files; read them before writing a new Go file.

## Testing

- Test behaviour, not implementation. Fake only what crosses the process boundary: native agents, the network, the clock where it matters. Use real files, real git and real SQLite in temporary directories.
- A test that only asserts liveness or non-failure (`err == nil` and nothing else, a process is still running, a value is non-nil) does not cover the behaviour in its title. Assert the observable the behaviour produces: the value, the event, the file, the exit code. If there is none, say so in the test name or drop the test.
- An assertion that accepts every branch cannot fail: pin the expected branch. A broken input that exercises data the code never reads proves nothing: show it red against the unfixed code.

## Comments and documentation

- No comments unless the logic is non-obvious or an edge case needs explaining. Names and structure carry the meaning.
- Delete dead code. Replace outdated documentation with accurate documentation in the same change.

## Logging

- Log what helps diagnose a failure: identifiers, counts, states and decisions. Never log prompts, source text, tokens or credentials (v2 §§8.3–8.4).
