# CLAUDE.md

Guidance for Claude Code agents working in this repository.

## Project

MYTHHELM is an open-source command deck for native coding agents: it orchestrates the agents people already use (Claude Code, Codex and others) with their native harnesses intact, and gives them a shared mission, safe working boundaries, an honest control plane and a terminal UI. The planned stack is a Go core, a Bubble Tea TUI, local SQLite state and a versioned process protocol for plugins.

Status: pre-alpha, design stage. [`docs/spec/master-spec.md`](docs/spec/master-spec.md) is the versioned design reference; read the relevant section before implementing anything it covers. Contributor policy is in [`CONTRIBUTING.md`](CONTRIBUTING.md), and security reports follow [`SECURITY.md`](SECURITY.md).

## Development harness

MYTHHELM is built by AI coding agents directed by human maintainers. [`docs/harness/charter.md`](docs/harness/charter.md) fixes the harness conventions and is binding. `WORKFLOW.md` will describe the agent workflow once it lands; it then takes precedence over the charter.

In Claude Code sessions, guard hooks run on every tool call; other agents get no hook enforcement and must follow the same rules by hand. [`.claude/hooks/README.md`](.claude/hooks/README.md) explains them and the arming recipes, and [`.claude/hooks/INVENTORY.md`](.claude/hooks/INVENTORY.md) lists them. When a hook blocks you, read its Detail and Fix lines and follow the Fix; never work around the guard's intent.

- **Orientation.** Run `/bootstrap` at the start of task work. [`knowledge/`](knowledge/README.md) holds the domain map, the invariants restated from spec §4, and the evidence behind each rule.
- **Agents and routing.** Route work by path with [`knowledge/domains.md`](knowledge/domains.md): `go-implementer` (core, adapters, protocol, docs), `tui-implementer`, `release-engineer`, `agent-config-editor` (harness); `architect` and `code-reviewer` review. [`knowledge/agent-routing.md`](knowledge/agent-routing.md) pins each agent's model and effort; never pass `effort` to a haiku agent (`completion-clerk`, `harness-clerk`).
- **Rules.** `.claude/rules/` holds always-on rules (behavioural posture, coding standards, formatter first, skill invocation contexts, prompt refinement, public-repo hygiene) and rules that load with the files they govern (Go, tests, workflows, harness, vendors).
- **Optional vendors.** Codex, Muse and Jev are opt-in and advisory, never gates. Reach them only through their wrappers ([`knowledge/vendors.md`](knowledge/vendors.md)); never opt a contributor in.

## Conventions

- **Pull requests only.** Work lands on `main` through a pull request with every required check green (`CI OK`), merged by squash or rebase. Never push to `main`.
- **DCO sign-off.** Every commit carries `Signed-off-by:`: use `git commit -s`.
- **Commit your own files.** Name the paths you commit (`git commit -s -m "<msg>" -- <file>...`). In the main checkout never use `git add -A`, `git add .` or `git commit -a`, since other sessions may share its index. Never use `git stash`; use a branch or a linked worktree (`git worktree add`).
- **Publishing is the operator's.** Releases, `v*` tags, workflow runs, secrets, variables and repository settings are changed only when the operator asks, through the armed window described in the hooks README.
- **Public by default.** Never commit secrets, personal emails, `/home/<user>` paths, hostnames, public IP addresses or session transcripts. Fixtures use `example.com`, RFC 5737 addresses (`192.0.2.x`) and `$HOME`-relative or `/tmp` paths.

## Quality gates

Run the formatter before any check: `gofmt -w .` and `go mod tidy` for Go.

| Area | Commands, from the repository root |
|---|---|
| Go | `gofmt -l .` (must print nothing), `go vet ./...`, `go test -race ./...`, `go mod tidy -diff`, `golangci-lint run` (once configured), `govulncheck ./...` |
| Harness | `scripts/ci/lint-agent-harness.sh`, `.claude/hooks/tests/run-tests.sh`, `shellcheck` on changed shell scripts, `scripts/ci/check-public-hygiene.sh` |
| Workflows | `actionlint` |
