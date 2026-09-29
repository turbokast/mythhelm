---
name: spec
description: Produce a validated spec (requirements, design, tasks, scratchpad) in specs/todo/ from a refined spec or a description, detecting whether the work is one spec or an epic of several
argument-hint: "<spec-name | \"feature description\">"
---

# Spec

Turns requirements into an implementable spec in one session: investigate the code, decide single spec or epic, write the design and the tasks, have a fresh-context reviewer validate them, fix what it finds, and move the result to `specs/todo/`. Each phase is its own skill, run in order.

## Input

`$ARGUMENTS`, detected before phase 1 with `scripts/harness/spec-lifecycle.sh resolve` (`.claude/skills/spec-resolution/SKILL.md`):

- **Mode A**: a spec name in `refined/`. Its `requirements.md` is the ground truth; this skill writes `design.md`, `tasks.md` and `scratchpad.md` beside it.
- **Mode B**: a spec name in `unrefined/`. Refuse: "`<name>` is unrefined; run `/refine-spec <name>` first." Nothing else runs.
- **Mode C**: a description with no spec of that name. The spec is drafted in `specs/unrefined/<name>/` and moves to `todo/` only after validation passes.
- A name already in `todo/` or later: refuse and point to `/evaluate-spec <name>`.

## Invocation contexts

- **Slash command**: runs phases 1–5; the scope determination (phase 2) and an epic's plan (phase 3b) are presented for confirmation or override.
- **Model-invoked**: the same.
- **Non-interactive**: runs phases 1–5. The scope and the epic plan are advisory steps: an explicit directive in `$ARGUMENTS` or the dispatching prompt ("single spec", "epic") is applied, otherwise the computed choice proceeds and is recorded as `auto-confirmed (non-interactive)`. Human-required validation findings (phase 5) stop the run and are returned, with the spec left in `unrefined/` or `refined/`.

## Phases

| Phase | Skill | Result |
|---|---|---|
| 1. Investigate | `/spec-investigate` | Grounded findings: packages, `file:line` facts, invariants in reach, conflicts |
| 2. Scope | `/spec-scope` | Single spec or epic, with the reason |
| 3a. Create single | `/spec-create-single` | `requirements.md` (mode C), `design.md`, `tasks.md`, `scratchpad.md` |
| 3b. Create epic | `/spec-create-epic` | `plan.md` plus one full spec per work stream |
| 4. Validate | `/spec-validate` | A fresh-context review report with a verdict |
| 5. Fix or report | `/spec-fix-and-report` | Fixes applied, the rest escalated; on Ready, the move to `todo/` |

1. **Investigate.** Run `/spec-investigate` with the description or spec name. In mode A, the refined requirements are not re-questioned; the investigation checks them against the code and gathers what the design needs.
2. **Scope.** Run `/spec-scope` (advisory step).
3. **Create.** Run `/spec-create-single` or `/spec-create-epic`. Both follow `/spec-decomposition` for `tasks.md`. Exit gate: `scripts/ci/lint-agent-harness.sh --only specs` passes. A spec with a missing task field or a dependency cycle is fixed now, not sent to review.
4. **Validate.** Run `/spec-validate` on the new spec directories.
5. **Fix or report.** Run `/spec-fix-and-report`. It fixes auto-fixable findings, re-runs phase 4 on what it changed (at most three validation rounds in all), and on Ready moves each validated spec to `todo/` with `scripts/harness/spec-lifecycle.sh move <name> todo`. An epic's `plan.md` stays in `unrefined/` until its first sub-spec starts.

## Principles

- **Investigate before decomposing.** The spec is only as good as the reading behind it. Every path, symbol and behaviour it names was opened, not assumed (`.claude/rules/spec-premise-grounding.md`).
- **Concrete over plausible.** Real paths, exact signatures, IDs from the master spec. An unknown goes in Open Questions with its conservative default; it is never papered over.
- **Each spec stands alone.** An agent implementing any one task has everything it needs from that spec, the files it cites and its declared dependencies.
- **Smaller is better.** A spec that can ship in two independent parts is two specs.
- **Don't re-read what this session has already read** and nothing has changed since (check `git status`). A dispatched validator always starts fresh; that is the point of it.

## Naming

Spec directories are lowercase kebab-case, named for the change: "Receipt shows unknown costs" becomes `receipt-unknown-costs`. An epic's sub-specs are siblings of each other in the lifecycle directories, never nested inside the epic's directory.

## Output

The report from `/spec-fix-and-report`: the spec (or epic plan and sub-specs), where each now lives, the validation verdict, what was fixed, and what needs a person.
