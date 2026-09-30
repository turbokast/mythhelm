# Agent routing

The Claude Code mapping from agent to model tier and thinking effort. Every file in `.claude/agents/` conforms to it, and `scripts/ci/lint-agent-harness.sh` (check `routing-pins`) fails when an agent file and this page disagree. Other clients use equivalent roles and their own model settings; these pins do not make Claude Code the default executor.

---

## Routing principle

Planning, review, diagnosis, configuration editing and release engineering route to the opus tier. Implementation from a scoped spec routes to the sonnet tier. Zero-judgment bookkeeping routes to the haiku tier.

Claude Code reads the dispatched agent's frontmatter `model:` and `effort:` when a session calls the Agent tool, so the parent session's model does not decide a subagent's tier. Every dispatch names an explicit `subagent_type` so that the pin engages.

---

## Canonical agent → model mapping

| Agent | Model | Tier | Rationale |
|---|---|---|---|
| `go-implementer` | `sonnet` | Implementer | Go from a scoped spec: core, adapters, protocol, docs |
| `tui-implementer` | `sonnet` | Implementer | Bubble Tea components and mods from a scoped spec |
| `release-engineer` | `opus` | Implementer | CI, supply chain and publishing mistakes are public and hard to reverse |
| `architect` | `opus` | Planner | Cross-domain reasoning against the spec's invariants |
| `code-reviewer` | `opus` | Planner | Recall on subtle correctness and security defects |
| `agent-config-editor` | `opus` | Planner | The harness constrains every other agent; errors propagate |
| `completion-clerk` | `haiku` | Mechanical | Verifies and backfills task-completion fields from a fixed template |
| `harness-clerk` | `haiku` | Mechanical | Counts, greps, status flips and template fills named by a skill |

---

## Tier effort defaults

Each opus and sonnet agent pins its tier's value as frontmatter `effort:`; each haiku agent carries none. The check reads this table, so keep the row shape `` | `<tier>` (<role>) | `<level>` | ``.

| Tier | Default effort | Rationale |
|---|---|---|
| `opus` (planner) | `xhigh` | The tier is chosen for its recall on hard problems; a lower effort spends the reason it was chosen |
| `sonnet` (implementer) | `high` | Enough deliberation for multi-file edits against a design that already made the hard calls |
| `haiku` (mechanical) | *(none — never pass `effort`)* | The haiku tier has no adaptive thinking and rejects an effort argument |

## Per-skill overrides

A skill that dispatches an agent at a model or effort other than its pin records the divergence here and passes it explicitly at the call site.

| Skill | Agent | Pinned | Override | Rationale |
|---|---|---|---|---|

*(No rows: no skill overrides a pin.)*

---

## Haiku authoring rules

Every haiku agent, and every skill that dispatches one, satisfies all four:

- **(a) No effort.** Never pass `effort` to a haiku dispatch. The check `haiku-effort` fails when a haiku dispatch in `.claude/` or `CLAUDE.md` has an effort parameter within ten lines.
- **(b) No reasoning gaps.** Name the exact skill section or write the full procedure in the dispatch prompt. The clerk runs it verbatim; it does not infer a missing step.
- **(c) Bounded input.** Name the specific files and facts the algorithm needs. Never hand a clerk a whole spec directory or an open-ended search.
- **(d) Fixed output.** The final message is the algorithm's report line in its exact shape: no narrative and no restated inputs.

Clerks carry no `Agent` tool, so they cannot dispatch, and they never commit, push or delete.

---

## Adding an agent

Classify the primary work first:

- **Implementation** (writing code from a spec): `model: sonnet`, `effort: high`. No `tools:` restriction unless the role needs one.
- **Review, architecture or diagnosis** (reads and reports): `model: opus`, `effort: xhigh`, and a `tools:` allowlist without `Edit`, `Write` or `NotebookEdit`: `[Read, Glob, Grep, Bash, Agent, Skill]`.
- **Configuration editing or release engineering** (high-stakes writes): `model: opus`, `effort: xhigh`, no `tools:` restriction.
- **Mechanical** (counting, polling, template fill): `model: haiku`, no `effort:`, `tools:` without `Agent`.

Add the row to the mapping table above in the same change as the agent file. The `/add-agent` skill does both.

## Residual risks

The check reads frontmatter only. The `CLAUDE_CODE_SUBAGENT_MODEL` and `CLAUDE_CODE_EFFORT_LEVEL` environment variables, and organisation effort caps, can change what actually runs. Never set them in tracked settings.

## Rationale

The reviewer tier differs from the implementer tier so that a model never grades its own output: an LLM judge rates work it produced more favourably than work it did not. Cost is the secondary reason: implementation is most of the tokens in a spec run, and the implementer tier is cheaper per token.

## Rollback

When a pin causes a regression, revert the commit that changed the agent's `model:` or `effort:` and this table together, then open an issue with the evidence (the failing runs and what the previous tier produced) before trying the change again.
