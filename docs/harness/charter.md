# Development harness charter

MYTHHELM is built by AI coding agents directed by human maintainers. The **development harness** is the machinery that keeps that work disciplined:

- guard hooks and project rules
- agent definitions and skills
- the spec lifecycle
- task-completion gates and the finalize pipeline
- the learning loop, the product layer and autonomous delivery
- optional cross-vendor consults

The harness is contributor tooling. It is **not** part of the MYTHHELM product: nothing under `.claude/`, `knowledge/`, `product/`, `specs/`, `orchestration/` or `scripts/` ships in a release.

This charter fixes the conventions the harness follows. It holds while the harness is being assembled. When it and `WORKFLOW.md` disagree, `WORKFLOW.md` is authoritative once it exists.

## Principles

1. **Mechanism over narrative.** A rule that matters has teeth: a hook, a CI check or a gate. Prose alone is advice.
2. **Fail closed on writes, fail open on advice.** Guards that protect shared state, the public repo or releases block. Advisory machinery never blocks a merge: vendor consults, classifiers and nags.
3. **Free for contributors.** Every mandatory path works with the Claude Code CLI plus standard POSIX tools. Paid third-party vendors (Codex, Muse, Jev) are strictly opt-in. They degrade to a no-op when the CLI, credential or subscription is absent, and the offline test suite never calls them.
4. **Clean text.** Rules and skills state current policy only. No amendment history, dated rulings, ticket IDs or "previously this said…" trails. Record the evidence for a rule in `knowledge/rule-evidence/`, not in the rule.
5. **Public by default.** Everything committed is world-readable. Runtime telemetry and session state are gitignored, and only schemas and empty seeds are tracked.
6. **Tested.** Every hook and script has tests under `.claude/hooks/tests/` or `scripts/**/tests/`. CI runs them all on every PR.

## Repository layout

```text
CLAUDE.md                 Agent entry point (Claude Code)
AGENTS.md                 Pointer for other native agents -> CLAUDE.md
WORKFLOW.md               The human-facing guide to the agent workflow
.claude/
  settings.json           Hook registrations and permissions (tracked)
  agents/                 Agent definitions (frontmatter model pins)
  skills/<name>/SKILL.md  Skills (slash commands + model-invocable)
  rules/                  Always-on and path-conditional rules
  hooks/                  Hook scripts, hook-helpers.sh, INVENTORY.md, tests/
  data/                   Runtime JSONL/state: gitignored except *.schema.json and seeds
  proposals/              pending.md / applied.md (config-improvement loop)
  evals/                  Config regression cases + runner
knowledge/                Curated agent reference (domains, invariants, routing, rule evidence)
product/                  Backlog, decisions, objectives, metrics (PM layer)
specs/{unrefined,refined,todo,in-progress,unfinalized,done,archived}/
orchestration/            Autonomous-delivery state (approvals ledger, run logs)
scripts/
  harness/                Shared harness scripts (arm-main-push, gates, effectiveness)
  orchestration/          lane, autonomy, approve, status, commit-paths, heartbeat
  vendors/                Cross-vendor envelope, lanes, sandbox, quota
  codex/  jev/            Vendor-specific wrappers
  ci/                     Harness self-tests run in GitHub Actions
```

## Domains and agents

The domain map (`knowledge/domains.md`) routes work to agents by path. The map follows spec §19.1:

| Domain | Paths | Implementing agent (model) |
|---|---|---|
| core | `cmd/`, `internal/` except `internal/tui/`, `go.mod` | `go-implementer` (sonnet) |
| adapters | `adapters/`, `hosts/` | `go-implementer` (sonnet) |
| protocol | `protocol/`, `sdk/`, `examples/` | `go-implementer` (sonnet) |
| tui | `internal/tui/`, `mods/` | `tui-implementer` (sonnet) |
| release | `.github/`, `packaging/`, `Makefile`, `.goreleaser*` | `release-engineer` (opus) |
| docs | `docs/`, `*.md` at root | `go-implementer` or the author's agent |
| harness | `.claude/`, `knowledge/`, `scripts/`, `WORKFLOW.md`, `CLAUDE.md` | `agent-config-editor` (opus) |

The planner-tier agents are `architect`, `code-reviewer` and `agent-config-editor` (opus). The mechanical-tier agents are `completion-clerk` and `harness-clerk` (haiku; never pass `effort`). `knowledge/agent-routing.md` holds the canonical mapping, and a CI check pins it.

## Quality gates

| Domain | Gate commands (from the repo root) |
|---|---|
| Go | `gofmt -l .` (must be empty), `go vet ./...`, `go test -race ./...`, `go mod tidy -diff`, `golangci-lint run` (once configured), `govulncheck ./...` |
| Harness | `scripts/ci/lint-agent-harness.sh`, `.claude/hooks/tests/run-tests.sh`, `shellcheck` on changed shell |
| Workflows | `actionlint` |

The formatter always runs before any check (`gofmt -w` / `go mod tidy`). Gate markers (`.claude/data/gate-marker-*.json`) record passes. The Stop-hook completion check reads them.

## Platform: GitHub

- CI is GitHub Actions. `CI OK` is the single aggregate required check, and harness jobs join its `needs:` list.
- `verify-ci` polls with `gh run list` and `gh run watch`, and never with a GitLab MCP.
- **Publishing actions are guarded**, like the production guard in other projects. An agent never runs any of these without an explicit operator arming:
  - `gh release create|edit|delete`
  - `gh workflow run` of release workflows
  - pushing `v*` tags
  - `gh repo edit|delete|rename|archive`
  - `gh api -X DELETE|PATCH|PUT` against repo or org settings
  - `gh secret|variable set|delete`
  - `gh pr merge --admin`
- `main` is ruleset-protected: PR required and checks required. Pushes to `main` are therefore impossible anyway, and the main-push guard exists for local clones and forks. Work lands via PR, with squash or rebase merges only.
- Every commit carries a DCO `Signed-off-by:` (`git commit -s`).

## Tooling assumptions

The harness requires bash 4+, git (with worktrees), jq, python3 (3.10+, stdlib; PyYAML only where noted), coreutils (`timeout`, `sha256sum`), `flock` and `shellcheck` (CI). It also requires `gh` for GitHub operations.

Optional components need extra tools: `bwrap` (vendor-lane sandbox on Linux), `codex`, `muse`, a Jev API key, and systemd user timers (autonomous heartbeat).

Hooks read the payload's `cwd` and `session_id`, resolve the repository with `git rev-parse` rather than a hardcoded path, and use `$CLAUDE_PROJECT_DIR` when it is set. There are no absolute home paths anywhere. Unit files and scripts use `%h`, `$HOME` or repository-relative paths, discovered at runtime.

## Leak policy (public repo)

Never commit any of the following:

- secrets, tokens or credential locations beyond "the vendor's native store";
- personal or third-party emails, apart from the project's published contacts in `SECURITY.md` and `MAINTAINERS.md`;
- `/home/<user>` paths;
- hostnames, IPs or machine IDs;
- customer, prospect or company-private material;
- business metrics;
- raw session transcripts or telemetry.

Example fixtures use `example.com`, `/tmp/...` or `$HOME`-relative paths. CI runs `scripts/ci/check-public-hygiene.sh` over the tree.

## Porting provenance

The harness is a clean re-implementation of an agent harness that the maintainers developed on a private project. The mechanisms and the lessons carry over; the private project's content, history and identifiers do not. Port the idea, and rewrite the text for MYTHHELM's Go/GitHub context.
