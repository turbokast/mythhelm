# Run the Formatter Before the Gate

- Run the formatter over every file you authored or edited before any check: `gofmt -w <files>` and, when imports or `go.mod` changed, `go mod tidy`. For shell, fix what `shellcheck` reports before running the harness tests.
- Never use `gofmt -l .`, `go mod tidy -diff`, `golangci-lint run` or a full gate run as your first formatting feedback. The check reports; the formatter fixes.
- Format test files in the same pass as the source they test. Newly written table tests and long assertions are the most frequent offenders.

Evidence: `knowledge/rule-evidence/formatter-before-gate.md`.
