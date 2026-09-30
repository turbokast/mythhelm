---
name: quality-gates
description: Canonical quality-gate commands for Go, the harness and workflows, and the decision matrix for which gates and which kinds of test a change needs
---

# Quality Gates

The one reference for gate commands. Agents, skills and rules link here instead of restating them. CI runs the same commands (`.github/workflows/ci.yml`); a gate that is green locally but not in CI is a finding, not a flake.

## Input

Optional `$ARGUMENTS`: the changed paths or a domain (`go`, `harness`, `workflows`). With none, derive the paths from `git diff --name-only origin/main...HEAD` plus uncommitted changes.

## Invocation contexts

- **Slash command**: selects the gates for the changed paths, runs them and reports the results.
- **Model-invoked**: the same; other skills and agents also read it as a reference without running it.
- **Non-interactive**: the same. It never writes, so it has no gates of its own.

## Steps

1. **Format first** (`.claude/rules/formatter-before-gate.md`): `gofmt -w <changed .go files>`; `go mod tidy` when imports or `go.mod` changed.
2. **Select gates** by changed path with the matrix below. Run every row that matches; a cross-domain change runs all of its rows.
3. **Run each gate through `scripts/harness/gate.sh <gate>`** (or a group: `go`, `harness`, `all` — the gates the change set needs). The wrapper runs the gate's command unpiped from the tree root and writes `.claude/data/gate-marker-<gate>.json` only for an exit-0 run over an unchanged tree; `python3 scripts/harness/gatelib.py status` shows which markers are fresh. A gate run another way records nothing, so capture its own exit status (`${PIPESTATUS[0]}`) and summary line yourself. A gate that did not print a result did not pass.
4. **Report** in the output format. Fix every failure before claiming completion (`.claude/skills/investigating-failures/SKILL.md`).

## Gate matrix

| Changed paths | Gates (from the repository root) |
|---|---|
| `*.go`, `go.mod`, `go.sum` | `gofmt -l .` (must print nothing) · `go vet ./...` · `go test -race ./...` · `go mod tidy -diff` (must print nothing) · `golangci-lint run` once `.golangci.yml` exists |
| `go.mod`, `go.sum` | also `govulncheck ./...` and `CGO_ENABLED=0 go build ./...` |
| OS-specific files (`*_windows.go`, `*_unix.go`, build tags) | also `GOOS=windows go vet ./...` and `GOOS=darwin go vet ./...` |
| `.claude/`, `knowledge/`, `scripts/`, `CLAUDE.md`, `AGENTS.md` | `scripts/ci/lint-agent-harness.sh` (its `evals` check runs the config-regression cases) · `.claude/hooks/tests/run-tests.sh` · `shellcheck <changed .sh>` |
| `.github/workflows/` | `go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12`; `zizmor` runs in CI |
| any change | `scripts/ci/check-public-hygiene.sh` |

## Which tests a change needs

| Change | Test |
|---|---|
| Logic inside a package | Unit tests in the package, table-driven |
| CLI verb, flag, output format or exit code | A test through `cli.Main` asserting stdout, stderr and the exit code; JSONL output parsed line by line |
| Behaviour of the built binary (process launch, detach, recovery, paths with spaces) | An end-to-end test under `tests/e2e` that builds `cmd/mythhelm` |
| Adapter decoding of a native stream | A fixture under the adapter's `testdata/`, marked synthetic until replaced by a sanitised recording |
| Anything needing a live vendor account | Behind the `live` build tag only; never in CI; a maintainer runs it |
| A bug fix | A regression test that fails on the unfixed code (`.claude/rules/red-first.md`) |
| A new check, hook or gate | Its broken input kept in the suite (`.claude/rules/teeth-discipline.md`) |

## Output

```text
gofmt -l .           pass (no output)
go vet ./...         pass
go test -race ./...  pass (ok: 12 packages)
go mod tidy -diff    pass (no output)
harness lint         skipped (no harness paths changed)
```

One line per gate: `pass`, `FAIL` with the first failing lines, or `skipped` with the reason.
