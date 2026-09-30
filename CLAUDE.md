# CLAUDE.md

Guidance for Claude Code agents working in this repository.

## Project

MYTHHELM is an open-source command deck for native coding agents: it orchestrates the agents people already use (Claude Code, Codex and others) with their native harnesses intact, and gives them a shared mission, safe working boundaries, an honest control plane and a terminal UI. The planned stack is a Go core, a Bubble Tea TUI, local SQLite state and a versioned process protocol for plugins.

Status: pre-alpha, design stage. [`docs/spec/master-spec.md`](docs/spec/master-spec.md) is the versioned design reference; read the relevant section before implementing anything it covers. Contributor policy is in [`CONTRIBUTING.md`](CONTRIBUTING.md), and security reports follow [`SECURITY.md`](SECURITY.md).

## Development harness

MYTHHELM is built by AI coding agents directed by human maintainers. [`WORKFLOW.md`](WORKFLOW.md) is the guide to the agent workflow and takes precedence over [`docs/harness/charter.md`](docs/harness/charter.md).

In Claude Code sessions, guard hooks run on every tool call ([`.claude/hooks/INVENTORY.md`](.claude/hooks/INVENTORY.md)); other agents must follow the same rules by hand. When a hook blocks you, follow its Fix line; never work around the guard's intent.

- **Orientation.** Run `/bootstrap` at the start of task work. [`knowledge/`](knowledge/README.md) holds the domain map, the invariants and the evidence behind each rule.
- **Routing.** Route work by path with [`knowledge/domains.md`](knowledge/domains.md); [`knowledge/agent-routing.md`](knowledge/agent-routing.md) pins each agent's model and effort. Never pass `effort` to a haiku agent (`completion-clerk`, `harness-clerk`).
- **Specs.** `/create-spec`, `/refine-spec`, `/spec`, then `/run-spec` (one pinned agent, worktree and pull request per task) and `/finalize-spec`: [`specs/README.md`](specs/README.md).
- **Product.** Agents propose `product/` changes; maintainers approve: [`product/README.md`](product/README.md).
- **Completion.** A task entry follows `/task-completion`; the Stop hook blocks a claim without fresh gate markers.
- **Learning loop.** `/apply-proposals` decides harness proposals one by one, each applied as its own pull request with an eval case; `/health-check` reports drift ([`knowledge/learning-loop.md`](knowledge/learning-loop.md)).
- **Autonomy.** `/deliver-backlog` runs unattended only inside the maintainer's grant: [`knowledge/autonomy.md`](knowledge/autonomy.md).
- **Optional vendors.** Codex, Muse and Jev are opt-in and advisory, never gates; reach them only through their wrappers ([`knowledge/vendors.md`](knowledge/vendors.md)).

## Conventions

- **Pull requests only.** Work lands on `main` through a pull request with every required check green (`CI OK`), merged by squash or rebase. Never push to `main`.
- **DCO sign-off.** Every commit carries `Signed-off-by:`: use `git commit -s`.
- **Commit your own files.** Name the paths you commit (`git commit -s -- <file>...`). Never `git add -A`, `git add .`, `git commit -a` or `git stash`: other sessions share the main checkout's index; use a linked worktree.
- **Publishing is the operator's.** Releases, `v*` tags, workflow runs, secrets, variables and repository settings are changed only when the operator asks, through the armed window described in the hooks README.
- **Public by default.** Everything committed is world-readable (`.claude/rules/public-repo-hygiene.md`).

## Quality gates

Run the formatter first (`gofmt -w .`, `go mod tidy`), then the gates through `scripts/harness/gate.sh go|harness|all`, which records each pass as a gate marker. The commands per area are in [`.claude/skills/quality-gates/SKILL.md`](.claude/skills/quality-gates/SKILL.md).
