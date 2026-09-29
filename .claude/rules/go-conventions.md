---
paths:
  - "**/*.go"
  - "go.mod"
  # future: go.sum appears with the first dependency
  - "go.sum"
---

# Go Conventions

MYTHHELM supervises processes that spend money and change code, so these are correctness rules, not style. The spec section for each is authoritative.

## Layout (spec §19.1)

- `cmd/mythhelm` holds only `main`, which calls `internal/cli`. All logic lives in packages.
- Implementation goes under `internal/`. Only `protocol/` and `sdk/` are public API; `adapters/*` state in `doc.go` that they are not a stable API.
- A package has one responsibility and appears when a task gives it one. No empty packages, no `util`, `common` or `helpers`.
- Package dependencies point inward: `cli` → domain packages → primitives (`ids`, `security`, `statedir`). Never import `internal/cli` from a domain package, and never form a cycle through an interface declared in the wrong package; declare interfaces where they are consumed.

## Processes (spec §7.4, §9.7, §12.5; invariants I01, I18)

- Launch every process with `exec.CommandContext(ctx, path, args...)` and an argument slice. Never run a shell (`sh -c`, `cmd /c`), and never interpolate a prompt, path or user value into a command string.
- Build the child environment from an allowlist; never pass `os.Environ()` through. Never set, read or log credential variables beyond what the design names.
- The process that starts a child owns it: record its identity, wait on it, and stop it through the owner's stop ladder. Never start a second supervisor or input writer for the same agent.

## Parsing untrusted input (spec §9.7, §12.7)

- Treat native agent output, plugin messages, repository files and configuration as untrusted. Read them with explicit bounds: maximum frame or line size, maximum nesting depth, maximum count, and a timeout.
- Use `bufio.Reader` with a size check, or `bufio.Scanner` with `Buffer` set; never read an unbounded stream with `io.ReadAll`.
- Decode strictly: reject unknown fields and duplicate keys where the format allows, and never map a parse failure to a default that allows (invariant I02).
- Sanitise control characters and escape sequences before anything reaches a terminal.

## Errors and logging (spec §12.7)

- Wrap errors with `%w` and context; define sentinel errors or typed errors for conditions callers branch on. Map errors to exit codes in one place (`internal/cli`).
- Log with `log/slog` through the redacting handler. Log identifiers, counts and states; never prompts, file contents, tokens, credential values or raw native transcripts.
- Keep estimated, reported, observed and unknown values in distinct fields or types (invariant I09); never use zero for unknown.

## Dependencies (spec §19.2, §19.4)

- Standard library first. Add a module only when a decision (design or ADR) names it; record why the standard library is not enough.
- Only licences compatible with Apache-2.0. Pin exact versions and run `go mod tidy`. Never vendor or copy code from another project without recording its licence.
- Builds need no cgo (`CGO_ENABLED=0` must pass) unless an ADR says otherwise.

## Cross-platform (spec §16)

- Code builds and its tests pass on Linux, macOS and Windows. Put OS-specific code in `_unix.go`, `_linux.go`, `_darwin.go` or `_windows.go` files with matching `//go:build` constraints, each with the same exported surface.
- Use `filepath`, never string concatenation with `/`, for filesystem paths; `path` only for slash-separated identifiers.
- A capability unavailable on a platform returns a typed "unsupported" error; it never silently degrades (invariant I14).

## Concurrency

- Every goroutine has an owner that waits for it and a context that stops it. No fire-and-forget goroutines.
- Protect shared state with one mechanism, and document which lock guards which fields. Tests run with `-race`.
