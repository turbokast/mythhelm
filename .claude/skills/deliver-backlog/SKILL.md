---
name: deliver-backlog
description: Delivery orchestrator — drive a set of backlog cards and specs through create-spec, refine-spec, spec, run-spec and finalize-spec in dependency-safe waves, resume-safe across sessions, with maintainer checkpoints at spec approval; runs unattended only inside the maintainer's autonomy grant
argument-hint: "[status | start <MH-n,spec:name,...> | approve-spec <spec> | max-items N]"
---

# Deliver Backlog

Takes a set of backlog cards (and specs without a card) from wherever each one stands to merged, finalized code, through the project's own lifecycle: `/create-spec` → `/refine-spec` → `/spec` → `/run-spec` → `/finalize-spec`. This skill orchestrates; it never implements. The lifecycle skills and the agents they dispatch write the specs, the code, the reviews and the product requests. The orchestrator writes run state only through `scripts/orchestration/delivery.py`.

**Resume-safe.** The run lives on disk in the main checkout's `orchestration/` (`INTENT.md`, `RUN-LOG.md`, `QUESTIONS.md`; formats in [`orchestration/README.md`](../../../orchestration/README.md)). Every invocation starts at Step 0, which reconciles that state with ground truth: spec directories and `tasks.md` on origin/main, pull requests on GitHub. Where they disagree, ground truth wins and the recorded stage is corrected. Re-invoking after a crash, a cleared context or a finished session continues the run; it never restarts it or repeats a completed stage.

## Input

`$ARGUMENTS` selects the mode:

| Arguments | Mode |
|---|---|
| (empty) | Resume the run and keep executing. With no run on disk, use the grant's scope as `start`, or ask for a scope |
| `start <ids>` | Start a run over those cards and specs (`MH-3,spec:foo`). Under a grant, every id must be in its scope |
| `status` | Step 0 only: reconcile and report, advance nothing |
| `approve-spec <spec>` | The maintainer's spec checkpoint for one spec, then resume |
| `max-items N` | Resume, advance at most N stage transitions, then report and stop |

`approve-spec` is the maintainer's decision. It counts only when the maintainer typed it or the dispatching prompt carries it from them; this skill never issues it to itself.

## Invocation contexts

- **Slash command**: runs Step 0 and then the loop. Without a grant the maintainer is present: a spec checkpoint or a blocking question may be asked directly, then recorded with `delivery.py`.
- **Model-invoked**: the same.
- **Non-interactive** (a headless or heartbeat-started session, or any session under a live grant): never asks. Every question is filed with `delivery.py question`, the affected items are parked, and independent work continues; a spec checkpoint becomes a question and the item waits at `approval`. `.claude/hooks/guard-blocking-ask.sh` blocks the synchronous form. When nothing actionable remains, the final report ends with the line `AWAITING MAINTAINER: <reason>`.

## Rules for the whole run

1. **The grant is the maintainer's.** `scripts/orchestration/autonomy.sh status` shows it. Never grant, renew or edit it (`.claude/hooks/guard-autonomy.sh` blocks it); revoking is allowed. What a grant permits is in [`knowledge/autonomy.md`](../../../knowledge/autonomy.md): continuing unattended within the scope, merging pull requests that pass `autonomy.py merge-check`, and, when the maintainer passed `--allow-pm-sync`, the lifecycle status moves of granted cards. Nothing else: no push to `main`, no tag, release or workflow run, no other `product/` change, no heartbeat installation.
2. **Scope is fixed.** Work only on the run's items, and under a grant only on items in its scope. A card that should join the run is a question for the maintainer, never a silent addition.
3. **The lifecycle is authoritative.** Enter each item at its current stage; never repeat or skip one. Each lifecycle skill does its own bookkeeping (lifecycle pull requests, PM-sync requests, retrospectives). The per-stage commands are in the loop below.
4. **Merges under a grant** go through `python3 scripts/orchestration/autonomy.py merge-check` and `gh pr merge <n> --squash --match-head-commit <sha>` (`/run-spec-worktree-merge`, `/finalize-spec-publish`). A pull request it refuses is not merged by any other route: fix what it names, or file a question.
5. **Lanes for shared resources.** At the start of the invocation mint a token (`scripts/orchestration/lane.sh token`) and keep it to yourself. Hold `run:delivery` for the whole invocation; take `index` around any commit in the main checkout, `spec:<name>` around a lifecycle move you make by hand, and `product` around a product write. Never pass the token to a worker, and tell workers never to call `lane.sh`.
6. **Run state through the script.** `delivery.py item|run|log|question` are the only writers of `orchestration/`; each stamps its line with the clock. Never write a timestamp yourself, never edit those files, and cite entries by question id or timestamp, never by line number. Workers never write `orchestration/`: they report, and you record.
7. **Trust nothing reported.** After every dispatch, check what actually happened: `git -C <main checkout> rev-parse HEAD`, `git fetch origin` and the pull request on GitHub. A skill's own commit-and-push instructions can override a read-only dispatch prompt, so an unexpected commit or push is a finding to report, not a surprise to absorb.
8. **Uncommitted files you did not write** in the main checkout belong to another session. Never commit, move or discard them. Before calling them a conflict, check whether each matches a blob already on origin/main (`git hash-object <file>` against `git rev-parse <commit>:<file>` over recent commits): stale residue of merged work is common and needs no action from you.

## Step 0: Resume

1. `scripts/orchestration/status.sh`: the grant, the run, lanes, specs in flight, open pull requests with unresolved-thread counts, main's CI. Do not re-read the state files end to end; the digest is the resume.
2. Take the run lane: `scripts/orchestration/lane.sh acquire run:delivery --owner <token> --note "deliver-backlog"`. Refused means another live session runs this delivery: report that and stop.
3. No run on disk: go to Plan. Otherwise `python3 scripts/orchestration/delivery.py reconcile` and correct every `<- differs` row with `delivery.py item <id> --stage <stage>` (ground truth wins; the correction is logged).
4. Answered questions (`QUESTIONS.md` entries whose Status is `answered` since the last run): apply each answer, unpark its items with `delivery.py item`, and log what you did.
5. Route by `$ARGUMENTS`: `status` reports and stops (release the lane); `approve-spec <spec>` moves that item from `approval` to `run-spec` and logs "approved by the maintainer"; otherwise continue with the loop.

## Plan (a new run)

1. Resolve the scope: `start <ids>`, else the grant's scope. Refuse ids outside the grant.
2. `python3 scripts/orchestration/delivery.py init --scope <ids>`.
3. For each card, read it in `product/backlog.md` and its Spec field. For each spec, find its state (`scripts/harness/spec-lifecycle.sh resolve <name>`). Record spec and stage with `delivery.py item <id> --spec <name> --stage <stage>`: no spec is `create-spec`; `unrefined/` is `refine-spec`; `refined/` is `spec`; `todo/` is `approval` (or `run-spec` when the grant skips the checkpoint or the spec was already approved); `in-progress/` is `run-spec`; `unfinalized/` is `finalize-spec`; `done/` is `done`.
4. Dependencies: from each card's Summary and each spec's requirements and design, record `--depends` edges and a `--wave` number in topological order. A cycle, or an edge you cannot ground in the text, is a question.

## The loop

Repeat until nothing is actionable or `max-items` is reached:

1. `python3 scripts/orchestration/delivery.py actionable` lists the items an agent can advance now. Specifying runs ahead of dependencies; `run-spec` and `finalize-spec` wait until every dependency is `done`. Take the lowest wave first. Run at most one `/run-spec` and one `/finalize-spec` at a time; specifying other items meanwhile is fine.
2. Advance the item one stage and record it:

   | Stage | Do | Then |
   |---|---|---|
   | `create-spec` | `/create-spec MH-<n>` | `--spec <name> --stage refine-spec` |
   | `refine-spec` | `/refine-spec <name>` | `--stage spec` |
   | `spec` | `/spec <name>` | `--stage run-spec` when the grant's spec checkpoint is off; otherwise file one question "Approve spec <name>" (the spec's path and a summary of its tasks) and `--stage approval --note Q-<n>` |
   | `approval` | Nothing: the maintainer approves with `approve-spec <name>` or by answering the question | Step 0 moves it to `run-spec` |
   | `run-spec` | `/run-spec <name>` | `--stage finalize-spec` when it reports the spec in `unfinalized/` |
   | `finalize-spec` | `/finalize-spec <name>` (never `--release`) | `--stage done` |

3. A lifecycle skill that stops (a task out of attempts, a review that will not converge, a question the spec does not answer): record its stop report with `delivery.py log`, file a question when the maintainer must decide, park the item (`--stage parked --note Q-<n>`), and continue with the next actionable item.
4. **Circuit breaker.** Two items parked for the same kind of failure, or a third of the run parked: stop taking new work (`delivery.py run --status paused`), file one question describing the pattern, and report.

## Escalation

Decide yourself, and record it with `delivery.py log`: the implementation approach, test strategy, ordering within the dependency graph, and recommended defaults for choices the specs leave open inside their own scope.

Escalate with `delivery.py question` (context, a recommended default, the affected items), park only those items and continue: conflicting or ambiguous requirements, anything that changes scope or a card, destructive or irreversible steps, anything needing credentials, a release or a push to `main`, and any case where two reasonable readings would build different things. Never take the risky default without an answer.

## Finish

When every item is `done` or parked with a question: `delivery.py run --status complete` (only when all are `done`), release the run lane, and revoke the grant if the whole scope is done (`scripts/orchestration/autonomy.sh revoke --reason "run complete"`). Remind the maintainer of `scripts/harness/prune-merged.sh` for the merged branches.

## Output

A report: the grant's state and time left; items advanced in this invocation with their stage transitions and pull request numbers; parked items with their question ids; open questions; the next action the following invocation will take. When nothing actionable remains, end with a final line `AWAITING MAINTAINER: <reason>`, which lets `.claude/hooks/continue-run.sh` release the session.
