# CLAUDE.md

Guidance for Claude Code agents working in this repository.

## Project

MYTHHELM is an open-source command deck for native coding agents: it orchestrates the agents people already use (Claude Code, Codex and others) with their native harnesses intact, and gives them a shared mission, safe working boundaries, an honest control plane and a terminal UI. The planned stack is a Go core, a Bubble Tea TUI, local SQLite state and a versioned process protocol for plugins.

Status: pre-alpha, design stage. [`docs/spec/master-spec.md`](docs/spec/master-spec.md) is the versioned design reference; read the relevant section before implementing anything it covers. Contributor policy is in [`CONTRIBUTING.md`](CONTRIBUTING.md), and security reports follow [`SECURITY.md`](SECURITY.md).

## Development harness

MYTHHELM is built by AI coding agents directed by human maintainers. [`WORKFLOW.md`](WORKFLOW.md) is the guide to the agent workflow and takes precedence over [`docs/harness/charter.md`](docs/harness/charter.md), which fixes the harness conventions.

In Claude Code sessions, guard hooks run on every tool call; other agents get no hook enforcement and must follow the same rules by hand. [`.claude/hooks/README.md`](.claude/hooks/README.md) explains them and the arming recipes, and [`.claude/hooks/INVENTORY.md`](.claude/hooks/INVENTORY.md) lists them. When a hook blocks you, read its Detail and Fix lines and follow the Fix; never work around the guard's intent.

- **Orientation.** Run `/bootstrap` at the start of task work. [`knowledge/`](knowledge/README.md) holds the domain map, the invariants restated from spec §4, and the evidence behind each rule.
- **Agents and routing.** Route work by path with [`knowledge/domains.md`](knowledge/domains.md): `go-implementer` (core, adapters, protocol, docs), `tui-implementer`, `release-engineer`, `agent-config-editor` (harness); `architect` and `code-reviewer` review. [`knowledge/agent-routing.md`](knowledge/agent-routing.md) pins each agent's model and effort; never pass `effort` to a haiku agent (`completion-clerk`, `harness-clerk`).
- **Spec lifecycle.** Work moves through `specs/<state>/<name>/` via `/create-spec`, `/refine-spec`, `/spec` and `/evaluate-spec`: [`specs/README.md`](specs/README.md), [`knowledge/spec-authoring.md`](knowledge/spec-authoring.md).
- **Execution.** `/run-spec <spec>` runs a spec's tasks: one pinned agent, worktree and pull request per task, merged when green with no open thread. `/implement` takes one task; its entry follows `/task-completion`, and the Stop hook blocks a claim without fresh gate markers. `/finalize-spec` closes a spec after its last merge.
- **Autonomy.** `/deliver-backlog` runs unattended only inside the maintainer's grant: [`knowledge/autonomy.md`](knowledge/autonomy.md).
- **Rules.** `.claude/rules/` holds always-on rules and rules that load with the files they govern ([`knowledge/domains.md`](knowledge/domains.md)).
- **Optional vendors.** Codex, Muse and Jev are opt-in and advisory, never gates. Reach them only through their wrappers ([`knowledge/vendors.md`](knowledge/vendors.md)); never opt a contributor in.

## Conventions

- **Pull requests only.** Work lands on `main` through a pull request with every required check green (`CI OK`), merged by squash or rebase. Never push to `main`.
- **DCO sign-off.** Every commit carries `Signed-off-by:`: use `git commit -s`.
- **Commit your own files.** Name the paths you commit (`git commit -s -m "<msg>" -- <file>...`). In the main checkout never use `git add -A`, `git add .` or `git commit -a`, since other sessions may share its index. Never use `git stash`; use a branch or a linked worktree (`git worktree add`).
- **Publishing is the operator's.** Releases, `v*` tags, workflow runs, secrets, variables and repository settings are changed only when the operator asks, through the armed window described in the hooks README.
- **Public by default.** Never commit secrets, personal emails, `/home/<user>` paths, hostnames, public IP addresses or session transcripts. Fixtures use `example.com`, RFC 5737 addresses (`192.0.2.x`) and `$HOME`-relative or `/tmp` paths.

## Quality gates

Run the formatter first (`gofmt -w .`, `go mod tidy`), then the gates through `scripts/harness/gate.sh go|harness|all`, which records each pass as a gate marker. The commands per area are in [`.claude/skills/quality-gates/SKILL.md`](.claude/skills/quality-gates/SKILL.md).
