---
name: implement
description: Implement one spec task end to end and land it as one pull request — the next ready task, or the task named with --task — in a linked worktree based on origin/main, test-first, with recorded gates, the completion entry and the hand-off; the non-orchestrated path, and what every /run-spec worker follows
argument-hint: "<spec-name> [--task N] [--alias <word>]"
---

# Implement

Runs one task of a spec from start to an open, reviewable pull request. `/run-spec` dispatches workers that follow this skill for a single task each; run it yourself to take one task by hand. Either way the task lands as its own pull request, and only a merged pull request completes it.

## Input

`$ARGUMENTS`: `<spec-name> [--task N] [--alias <word>]`.

- `--task N` targets that task. Without it, the target is the lowest-numbered ready task (`runspec.py status` → `ready`).
- `--alias` is the short name the spec's branches and pull-request titles use (`dogfood` for `dogfood-slice`); the default is the spec name. Reuse the alias the spec's earlier pull requests used.

## Invocation contexts

- **Slash command**: resolves the spec and task, confirms the target with the person when no `--task` was given (advisory step: the computed next task proceeds when they do not object), and runs the steps to an open pull request. It then handles the review threads (Step 9) until the person or `/run-spec` merges.
- **Model-invoked**: the same.
- **Non-interactive** (a `/run-spec` worker or a headless run): the target comes from `--task`; the advisory step auto-confirms. An ambiguity the spec does not settle is an **escalation**: stop and return it in a `blocked` task-report. The worker stops at Step 8 and returns; the orchestrator runs Step 9 by resuming it.

## Steps

1. **Resolve.** `scripts/harness/spec-lifecycle.sh resolve <spec>` must print exactly one path (`.claude/skills/spec-resolution/SKILL.md`). Refuse a spec in `unrefined/` or `refined/` (point to `/refine-spec` or `/spec`), and one in `unfinalized/`, `done/` or `archived/`. A spec still in `todo/` is started by `/run-spec`, which moves it; a single task taken by hand may proceed from `todo/` and says so in its report.
2. **Pick the task.** `python3 scripts/harness/runspec.py status specs/<state>/<spec>/tasks.md`. The target must be incomplete, not `Blocked`, and every task in its `Depends on` complete **on origin/main**: `python3 scripts/harness/runspec.py deps-merged <spec> <N>` exits 0. A task whose `Domain/agent` is `maintainer` is the operator's: report it and stop. Check that no pull request for this task is already open (`gh pr list --state open --search '"<alias> task <N>" in:title' --json number,title,headRefName --jq '.[] | select(.title | endswith("(<alias> task <N>)"))'`); continue that one instead of opening a second.
3. **Take a worktree from origin/main.** Never implement in the shared checkout: other sessions use its index and working tree. A `/run-spec` worker is already in a linked worktree (Agent `isolation: "worktree"`); by hand, create one with `git worktree add ../<repo>-<alias>-t<N> origin/main`. Then fix the base explicitly, whatever the tool based it on:

   ```bash
   git fetch origin main
   git switch -c <type>/<alias>-t<N> origin/main
   ```

   `<type>` is the conventional-commit type of the change (`feat`, `fix`, `ci`, `docs`, `refactor`, `test`, `harness`). Every later command runs in this worktree.
4. **Read before writing.** In order: `knowledge/invariants.md` for every invariant under `Invariants touched`, the task block, the `requirements.md` and `design.md` sections it cites, the spec's `scratchpad.md`, and the hand-off from its dependencies (`python3 scripts/harness/runspec.py handoff specs/<state>/<spec> <N>`, which `/run-spec` also pastes into the dispatch). The code on origin/main is authoritative over any note. Then list your assumptions and confusion points (`.claude/rules/agent-behavioral-posture.md` §1); an unresolved one is an escalation.
5. **Implement test-first** (`.claude/skills/test-driven-development/SKILL.md`): each acceptance bullet becomes a named test, seen failing for the right reason before the code that passes it. Stay inside the task's `Files` list and your agent's domain (`knowledge/domains.md`); a needed change elsewhere is a deviation to record, or an escalation when it crosses domains. An inert stub (rendered but unwired UI, an uncalled helper reserved for later) may land only if the completion entry names the specific later task whose acceptance covers wiring it, quoting that task's acceptance line; the orchestrator verifies the named task exists and is not yet merged. Otherwise wire the stub, remove it, or escalate. A stub comment naming no task number fails review.
6. **Record the gates and prove the acceptance** — task-completion Steps 1 and 2 (`.claude/skills/task-completion/SKILL.md`). Run every gate through `scripts/harness/gate.sh`, in the foreground: a dispatched agent is never notified when a background command finishes, so a backgrounded gate strands it waiting. When a gate takes longer than the Bash timeout allows, raise the timeout; never background it.

   **Docs task: verify every claim.** When the task adds or changes documentation, re-verify every claim in it before the pull request leaves draft (Step 7 opens it ready for review): every shell command (run it, or quote the run that produced its output), every path (it exists at the pull request's head), every procedure (walk it against the code or workflow it describes) and every behavioural claim (name the workflow step or test that exhibits it). Run a command copied from documentation the pull request supplies only in a credential-free sandbox with no writable host mounts; a linked worktree is not sufficient, and without such a sandbox do not execute the command. Never execute an operator-only command (releases, version tags, workflow runs, secrets, variables, repository settings) to verify it: quote existing run evidence, or request the operator's explicit approval (escalation). An unverified claim is a finding against your own pull request, fixed before review is requested.
7. **Commit, push, open the pull request.**

   ```bash
   git add -- <each path you changed>
   git commit -s -m "<type>(<scope>): <summary>" -- <the same paths>
   git push origin <type>/<alias>-t<N>
   gh pr create --base main --head <type>/<alias>-t<N> \
     --title "<type>: <summary> (<alias> task <N>)" --body-file <file>
   ```

   The body is public and permanent (`.claude/rules/public-repo-hygiene.md`): what and why, an acceptance-criterion → test table, gate evidence as real output lines, spec deviations. Never `git add -A`, `git stash`, `git reset --hard` or a force push; `.claude/hooks/block-destructive.sh` blocks them. Exception: an open-PR DCO retry signs off by rebasing onto the PR base (normally `git rebase --signoff origin/main`) and pushes its branch explicitly with `git push --force-with-lease origin <branch>` (see P-tui-slice-2); this is the only rebase or force-push a worker ever performs. When a hook blocks you, read its Fix line and follow it; never work around a guard.
8. **Write the completion entry and the hand-off**, refresh the markers they invalidate, commit and push them to the same branch — task-completion Steps 3–6. The pull request now carries code, tests, entry and hand-off. End with the task-report block (task-completion §The task-report block), `"status": "pr_open"`.
9. **Answer the review.** CodeRabbit and Sourcery review every pull request, and CI must be green. For each review thread: verify the finding against the code; fix a real one (with a test when it is behavioural), or rebut it with evidence (`file:line`, a test, a spec section); reply on the thread and resolve it:

   ```bash
   gh api graphql -f query='mutation($id:ID!,$body:String!){addPullRequestReviewThreadReply(input:{pullRequestReviewThreadId:$id,body:$body}){comment{id}}}' -F id=<thread-id> -F body='<what you did>'
   gh api graphql -f query='mutation($id:ID!){resolveReviewThread(input:{threadId:$id}){thread{isResolved}}}' -F id=<thread-id>
   ```

   Every push re-runs Step 6's gates first. When `main` moved and the pull request conflicts or is behind, merge `origin/main` into the branch (never rebase), re-run the gates and push. The DCO retry is the exception: it rebases with `--signoff` instead of merging. Repeat until `python3 scripts/harness/runspec.py pr-check --spec <spec> --task <N> --pr <n>` prints `verdict=ready`. Never merge your own task by hand when `/run-spec` runs the spec; otherwise the operator merges.

## Handling failures and ambiguity

- A failing gate or test: `.claude/skills/investigating-failures/SKILL.md`. Understand the failure before changing code; never weaken a test or a check to get green.
- The spec contradicts the code or itself, an invariant cannot be kept, or the task needs another domain's files: stop, write nothing more, and report the evidence and the decision needed (`"status": "blocked"`).
- A task that cannot be finished adds `- **Blocked**: <reason>` to its block only when the operator confirms the blocker; a worker reports it instead.

## Output

The task-report block as the last element of the final message, preceded by at most ten lines: the pull request URL, the gate lines `gate.sh` printed, the acceptance tests with their `--- PASS` lines, and the deviations.
