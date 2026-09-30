---
name: mythhelm-deliver-backlog
description: Resume or coordinate a MYTHHELM backlog run across specs, tasks, reviews and pull requests with maintainer checkpoints.
---

# Deliver backlog

Use this for `status`, `start <MH-n,spec:name,...>`, `resume`, or a bounded `max-items N` request. The run is stored in `orchestration/`; `scripts/orchestration/delivery.py` is its only writer. Read `AGENTS.md`, `WORKFLOW.md` (Autonomous delivery), `orchestration/README.md` and the relevant stage procedure before advancing an item. The detailed Claude adapter at `.claude/skills/deliver-backlog/SKILL.md` is a procedural reference; translate slash commands and dispatch calls to the native tools available in this client.

## Resume and plan

1. Run `scripts/orchestration/status.sh --no-fetch`. Acquire `run:delivery` with a private token from `scripts/orchestration/lane.sh token`; stop if another live session owns the lane.
2. Run `python3 scripts/orchestration/delivery.py reconcile`. Correct each recorded stage that differs from the actual spec and PR state. Read answered questions before choosing work.
3. With no run, initialize only the requested scope through `delivery.py init --scope <ids>`, then record each item's current stage and grounded dependencies. Never silently add a card.
4. Run `delivery.py actionable`. Advance the lowest dependency wave. Specify ahead of dependencies; build or finalize only after dependencies are done.

## Advance one stage at a time

| Stage | Procedure document | Record when successful |
|---|---|---|
| `create-spec` | `.claude/skills/create-spec/SKILL.md` | `refine-spec` |
| `refine-spec` | `.claude/skills/refine-spec/SKILL.md` | `spec` |
| `spec` | `.claude/skills/spec/SKILL.md` | `approval` or `run-spec` when the maintainer explicitly skipped the checkpoint |
| `approval` | Wait for the maintainer's decision; do not approve your own spec | `run-spec` after approval |
| `run-spec` | `.claude/skills/run-spec/SKILL.md` | `finalize-spec` after the lifecycle PR merges |
| `finalize-spec` | `.claude/skills/finalize-spec/SKILL.md` | `done` after its PR and CI verification |

Use `delivery.py item|run|log|question` for state changes; never edit run state by hand. Stage procedures specify verifiable outcomes. Check actual GitHub PRs, `origin/main`, task entries, gate markers and acceptance tests before recording success. If a stage stops, log the evidence, park only affected items with a question, and continue independent work. Apply the circuit breaker in the Claude adapter when repeated failures affect the run.

## Authority and client capabilities

The operator alone grants or renews autonomy, approves specs and product requests, and authorizes publishing. In a client without a tested session-bound guard adapter, run interactively: stop at maintainer checkpoints, do not use a grant for unattended merging or product pre-approval, and never claim that Claude hooks protected a tool call. `scripts/harness/runspec.py pr-check`, `scripts/harness/finalize.py publish-check`, CI and `scripts/ci/check-public-hygiene.sh` are usable from any client. Merge only a PR that passes the appropriate check at the head being merged and falls inside authorized scope.

Always release the delivery lane when stopping. Report stage transitions, PR numbers, parked questions and the next action. With nothing actionable, explain what awaits the maintainer.
