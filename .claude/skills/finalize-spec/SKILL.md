---
name: finalize-spec
description: Close a spec whose last task has merged — verify every task and main's CI from GitHub, review the whole spec's diff, write the retrospective and proposals, land one finalize pull request that moves the spec to done with its docs and changelog, watch main's CI for it, sync the backlog, and optionally prepare a release
argument-hint: "<spec-name> [--alias <word>] [--release] [--publish]"
---

# Finalize Spec

In MYTHHELM every task already lands as its own reviewed pull request, so a spec in `specs/unfinalized/` is code on `main`. Finalizing closes it: a review of the whole spec rather than one task at a time, a retrospective that feeds the harness's learning loop, one pull request that files the record (`specs/done/`, docs, changelog, proposals), and a check that `main` stays green afterwards.

Every verdict here comes from `scripts/harness/finalize.py`, which reads git and GitHub; `knowledge/finalize.md` explains what each step protects against. A sub-agent's prose is never evidence.

## Input

`$ARGUMENTS`: `<spec-name> [--alias <word>] [--release] [--publish]`.

- `--alias`: the short name in the spec's branch names and pull-request titles; reuse the one `/run-spec` used (default: the spec name).
- `--release`: also run `/finalize-spec-tag` to prepare release notes. `--publish` passes on to it: the maintainer's request to publish the release (`/finalize-spec-tag` §Publish).

## Invocation contexts

- **Slash command**: runs Steps 0–8. The merges of the finalize pull request and of any fix pull request are this skill's own, and invoking it is the approval for them.
- **Model-invoked**: the same.
- **Non-interactive**: the same, with no questions; every escalation stops the run and returns the stop report. A finalizer that is itself a subagent receives no background notifications: it waits on CI and reviews in the foreground (`finalize.py ci --wait` and `gh pr checks --watch` with a raised Bash timeout), never by ending its turn.

## Sub-skills

| Step | Skill | Writes |
|---|---|---|
| 1 | `/finalize-spec-verify` | nothing |
| 2 | `/finalize-spec-review` | `## Review Summary` in the spec's `retrospective.md` |
| 3 | `/finalize-spec-retrospective` | the rest of `retrospective.md`; `.claude/proposals/pending.md` |
| 4 | `/finalize-spec-publish` (runs `/finalize-spec-epic-rollup` and `/update-docs`) | the finalize pull request |
| 5 | `/finalize-spec-verify-ci` | fix pull requests, when main turns red |
| 6 | `/finalize-spec-pm` | approval requests for the backlog |
| 7 | `/finalize-spec-tag` (with `--release`) | nothing public unless `--publish` |

## Step 0 — Worktree

Finalizing never happens in the shared checkout: other sessions use its index and working tree, and a commit there can sweep their work into yours. Every later step reads and writes in the finalize worktree (`<wt>`).

First decide from GitHub, not from memory, whether this is a resumed run: `git fetch origin`, then `gh pr list --state all --head docs/<alias>-finalize --json number,state,mergeCommit`.

- **Merged** finalize pull request (the spec is in `specs/done/` on origin/main): the record has landed. When the run-events log already holds this spec's `lifecycle` row `unfinalized -> done` with `result: ok` (`grep` it in the main checkout's `.claude/data/run-events.jsonl`), the finalize is complete: report that and stop. Otherwise go straight to Step 5 with its merge commit, then Steps 6–8; `finalize-spec-verify` would rightly refuse a spec already in `done/`. The log is local, so another machine re-runs Steps 5–6, which is safe: a green verdict re-verified is still green, and `pm-sync-core` skips a card already `shipped`.
- **Open** finalize pull request: use its worktree, or recreate one from the branch (`git worktree add ../<repo>-<alias>-finalize docs/<alias>-finalize`), and go straight to `/finalize-spec-publish` step 7.
- **None**, or only closed unmerged ones: a fresh run from Step 1, in a worktree that holds only this run's work:
  - No branch `docs/<alias>-finalize` and no worktree at the path: `git worktree add -b docs/<alias>-finalize ../<repo>-<alias>-finalize origin/main`.
  - A stopped earlier run left them: reuse that worktree. Bring it current with `git -C <wt> merge origin/main`; the steps rewrite everything they own (the Review Summary is replaced, the retrospective's sections rewritten). A branch without a worktree is recreated with `git worktree add ../<repo>-<alias>-finalize docs/<alias>-finalize`. Never force-remove either one: a worktree holding changes this skill does not write is an escalation.

Record the start: `python3 scripts/harness/runspec.py event --spec <spec> --kind lifecycle --detail "finalize start"`.

## Steps

1. **Verify** — `/finalize-spec-verify <spec> --alias <alias>`. Continue only on `verdict=ready`.
2. **Review** — `/finalize-spec-review <spec>`. When it reports open critical findings, stop: each is fixed in its own pull request (below), then `git -C <wt> merge origin/main` and re-run from Step 1, passing the fix pull requests to `/finalize-spec-review` as `--extra-pr`.
3. **Retrospective** — `/finalize-spec-retrospective <spec>`.
4. **Publish** — `/finalize-spec-publish <spec> --alias <alias>`: move the spec to `done/`, roll up its epic, update the docs and the changelog, open the finalize pull request, answer its review, merge it on `verdict=ready`, confirm it on origin/main.
5. **Verify CI** — `/finalize-spec-verify-ci <spec> --sha <finalize merge commit> --alias <alias>`.
6. **Backlog** — `/finalize-spec-pm <spec>`. It runs after CI is green, because a card is `shipped` only when the code is merged and main is green.
7. **Release** — with `--release` only: `/finalize-spec-tag <spec> [--publish]`. Without it, report `Release: not requested`.
8. **Clean up and report.** When the finalize worktree still exists: `git -C <wt> status --porcelain` prints nothing, then `git worktree remove <wt>`; when the local branch still exists, `git branch -d docs/<alias>-finalize`. After a squash merge `-d` refuses, because the branch's own commits never reached `main`: leave the branch and name it in the report for the maintainer, never force-delete it. Record `runspec.py event --spec <spec> --kind lifecycle --detail "unfinalized -> done" --result ok`, then print the report.

## Fix pull requests

A confirmed critical review finding (Step 2) or a real CI failure (Step 5) is fixed like a task: one pull request per fix, by the agent that owns the files (`knowledge/domains.md`), dispatched with `subagent_type` and `isolation: "worktree"`, following `/implement` Steps 3–9 but with no task entry. Title it `fix: <what> (<alias> fix)` so `finalize.py verify` sees it while it is open. Merge it on `python3 scripts/harness/finalize.py publish-check --spec <spec> --pr <n> --fix` printing `verdict=ready`, then confirm its merge commit on origin/main. Print `dispatched=N returned=M failed=K` for every batch of fix dispatches.

## Escalations (the run stops)

1. `finalize-spec-verify` is not ready for a reason no fix pull request can remove: a task incomplete, the spec in the wrong state, the required checks changed, the gate weakened.
2. A critical finding the fixer cannot resolve, or one that means the spec itself is wrong.
3. Three review rounds leave the finalize pull request not ready.
4. `/finalize-spec-verify-ci` escalates: an `infra` or `unknown` failure, two fix cycles without green, or a wait budget spent.
5. A guard blocked this session, or a leak reached a title, body or pushed commit.

The stop report names the step, the evidence (verbatim, at most ten lines), what is in flight, and the decision needed. The spec stays where it is on origin/main; re-running `/finalize-spec <spec>` resumes.

## Output

```text
## Finalized: <spec>

Verify:     verdict=ready (<tasks> tasks, last merge <sha7>, main green)
Review:     <range>; findings critical N / important N / suggestion N; open critical 0; vendor <result>
Retro:      specs/done/<spec>/retrospective.md; proposals <ids or none>
Publish:    PR #<n> merged as <sha7>; epic <rolled up | no-op>; docs <files or none>; changelog <entries or none (reason)>
Verify CI:  <green | green after fix PRs #.. | escalated: reason>
Backlog:    <pm-sync lines | PM sync pending (reason)>
Release:    <not requested | notes prepared at <path> | published vX.Y.Z>
dispatched=<N> returned=<M> failed=<K>
```
