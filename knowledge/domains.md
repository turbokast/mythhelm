# Domains

Every path in the repository belongs to one domain, and every domain has one implementing agent. The map follows the repository structure in spec §19.1 and the table in [`docs/harness/charter.md`](../docs/harness/charter.md) §Domains and agents, which also fixes the harness row.

---

## Path → domain → agent

The first matching row wins, top to bottom.

| Domain | Paths | Agent |
|---|---|---|
| tui | `internal/tui/`, `mods/` | `tui-implementer` |
| core | `cmd/`, `internal/` (all but `internal/tui/`), `tests/`, `evals/`, `go.mod`, `go.sum` | `go-implementer` |
| adapters | `adapters/`, `hosts/` | `go-implementer` |
| protocol | `protocol/`, `sdk/`, `examples/` | `go-implementer` |
| release | `.github/`, `packaging/`, `Makefile`, `.goreleaser*`, `.golangci.yml`, `.coderabbit.yaml`, `.editorconfig`, `.gitattributes`, `.gitignore`, `.shellcheckrc`, `docs/automation.md` | `release-engineer` |
| harness | `.claude/`, `knowledge/`, `scripts/`, `docs/harness/`, `WORKFLOW.md`, `CLAUDE.md`, `AGENTS.md` | `agent-config-editor` |
| docs | `docs/` (all but `docs/harness/` and `docs/automation.md`), `*.md` at the root not listed above | `go-implementer`, or the agent whose change the docs describe |
| specs | `specs/` | the session running the lifecycle skills ([`specs/README.md`](../specs/README.md)) for `requirements.md`, `design.md` and `tasks.md`; the implementing agent for its own task's completion entry and scratchpad note |

The charter's table lists the same rows; a change to either updates both.

---

## Routing rules

- **One domain per task.** A task's file list resolves to one domain. Split cross-domain work into tasks joined by a dependency: the earlier task ships the contract (a type, an interface, a schema) that the later one consumes.
- **Cross-domain review.** A change that touches two or more domains, or crosses the `internal/` ↔ `adapters/` seam, the public protocol, persistence, billing or process ownership, gets an `architect` review before merge.
- **Every change gets a review.** `code-reviewer` reviews every completed task against its spec and the conventions.
- **Out-of-domain edits.** An agent that needs a change outside its domain stops and reports which agent is needed and what must change. It never widens its own scope.
- **Unmapped paths.** A path no row matches is a gap in this map: stop and ask, and fix the map in the same change.

---

## Rules that load per domain

Always-on rules load in every session. These load when a matching file is read or edited:

| Domain | Path-conditional rules |
|---|---|
| core, adapters, protocol, tui | `go-conventions.md`; for `*_test.go` also `test-quality.md`, `red-first.md`, `teeth-discipline.md` |
| release | `github-workflows.md` |
| harness | `strict-by-default.md`, `agent-config-conventions.md`, `knowledge-conventions.md`, `harness-scripts.md`, `teeth-discipline.md` |
| specs | `spec-authoring.md`, `spec-premise-grounding.md`, `teeth-discipline.md` |

## Package layout

Spec §19.1 proposes the `internal/` packages (admission, routing, scheduler, supervisor, workers, workspace, integration, journal, billing, security, tui) and the top-level trees. The names are organisation, not an instruction to create empty packages: a package appears when a task gives it a working responsibility. The active spec's `design.md` lists the packages it creates.
