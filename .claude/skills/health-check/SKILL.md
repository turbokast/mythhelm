---
name: health-check
description: Read-only health report on the development harness — always-on budget headroom, eval cases, pending proposal age, specs stuck in flight, stale gate markers, vendor opt-in, required checks, main's CI, open pull requests with unresolved threads, and local branches whose pull request merged
argument-hint: "[--offline] [--json] [--stale-days N]"
---

# Health Check

One report on whether the harness is in a state to trust. It changes nothing: every finding comes with the command that would act on it, for a maintainer or another skill to run. The checks and their statuses are defined in `scripts/harness/health.py`; `knowledge/learning-loop.md` explains where each fits in the loop.

## Input

`$ARGUMENTS`, all optional: `--offline` (skip the GitHub checks), `--json` (the machine-readable report), `--stale-days N` (the age at which a proposal or an in-flight spec is reported, default 7).

## Invocation contexts

- **Slash command**: runs both steps and prints the report.
- **Model-invoked**: the same.
- **Non-interactive**: the same; the report is the final message. The skill writes nothing, so it has no gates.

## Steps

1. **Report.** Unless `--offline`, refresh the remote refs first so `origin/main` is current (a fetch changes no branch or file):

   ```bash
   git fetch -q origin
   python3 scripts/harness/health.py $ARGUMENTS
   ```

   The script prints every check with one status: `ok`, `warn`, `fail`, `unknown` or `skipped`. A check that could not run is `unknown` with its reason, never omitted, so the report shows `checks=10` on every run. Quote its output; do not re-derive or summarise away a status.
2. **Next steps.** For each `warn`, `fail` or `unknown`, name the action, without taking it:

   | Check | Action |
   |---|---|
   | `budget` | move always-on text to path-conditional rules or `knowledge/` (`.claude/rules/agent-config-conventions.md`) |
   | `evals` | the named case lost its behaviour: fix the change that broke it, or update the case in the same pull request that deliberately changes the behaviour |
   | `proposals` | `/apply-proposals` |
   | `specs` | `/run-spec <spec>` or `/finalize-spec <spec>` to resume, or `/evaluate-spec <spec>` |
   | `gate-markers` | `scripts/harness/gate.sh <scope>` in that worktree before any completion claim |
   | `required-checks` | a maintainer restores the ruleset, or updates `.claude/data/required-checks.json` and `docs/automation.md` together |
   | `main-ci` | `real`: a fix pull request; `infra` or `unknown`: the maintainer decides on a re-run |
   | `open-prs` | answer and resolve the threads on each pull request |
   | `merged-branches` | the maintainer removes the branch (`git branch -d <branch>`) and its worktree (`git worktree remove <path>`) |

   Never delete a branch or worktree, re-run a job, merge or edit anything from this skill.

## Output

The script's report verbatim, followed by the next-step lines for every check that is not `ok` or `skipped`:

```text
Harness health
  ok       budget           always-on 13291 of 16384 bytes, headroom 3093
  warn     proposals        2 pending, oldest 21 days; decide them with /apply-proposals
  ...
checks=10 ok=7 warn=2 fail=0 unknown=1 skipped=0
Next: proposals -> /apply-proposals; ...
```
