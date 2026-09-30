# Orchestration

Local state for agent-driven delivery. Everything in this directory except this file is gitignored and stays on the maintainer's machine, in the main checkout, where its linked worktrees share it. The procedure that uses it is `/deliver-backlog` ([`.claude/skills/deliver-backlog/SKILL.md`](../.claude/skills/deliver-backlog/SKILL.md)); the reasons behind it are in [`knowledge/autonomy.md`](../knowledge/autonomy.md).

| File | What it is | Written by |
|---|---|---|
| `INTENT.md` | The delivery run: its status, its scope, and one row per item with its lifecycle stage | `scripts/orchestration/delivery.py` only |
| `RUN-LOG.md` | Append-only, one timestamped line per event | `delivery.py` only |
| `approvals.jsonl` | The approval ledger: signed maintainer decisions on requested changes, and a `consumed` row for each write an approval released. Append-only. | `scripts/orchestration/approvals.py` only |
| `requests/<id>/` | One request per proposed change: `request.json`, the full `proposed` file and its `diff`. | Agents, through `approvals.py request` |
| `QUESTIONS.md` | Questions for the maintainer, and their answers | `delivery.py question` files; the maintainer answers |

Agents never edit these files directly: `.claude/hooks/guard-autonomy.sh` blocks Edit, Write and shell writes to the run's three files, and `guard-product-write.sh` does the same for the approval ledger (the product approval flow is in [`.claude/hooks/README.md`](../.claude/hooks/README.md) §Product approvals). So every entry carries the clock's time at the moment it was written, the log only grows, and answers stay the maintainer's.

## INTENT.md

```markdown
# Delivery intent

- **Run**: 2026-01-05T09:00:00Z
- **Status**: active
- **Scope**: MH-3, MH-5, spec:status-json
- **Started**: 2026-01-05T09:00:00Z
- **Updated**: 2026-01-05T11:42:10Z

## Items

| Item | Spec | Stage | Wave | Depends on | Note |
|---|---|---|---|---|---|
| MH-3 | worker-journal | run-spec | 1 | — | — |
| MH-5 | status-panel | approval | 2 | MH-3 | Q-2 |
| spec:status-json | status-json | refine-spec | 1 | — | — |
```

- **Status**: `active` (the loop runs), `paused` (the circuit breaker tripped, or the maintainer paused it), `complete` (every item done; a new run may start).
- **Item**: a backlog card `MH-<n>`, or `spec:<name>` for a spec without a card.
- **Stage**, in lifecycle order: `create-spec`, `refine-spec`, `spec`, `approval` (waiting for the maintainer's spec checkpoint), `run-spec`, `finalize-spec`, `done`; and `parked` (waiting on the question in its Note, or an external blocker).
- **Actionable** means an agent can advance the item now: its stage is one of `create-spec`, `refine-spec` or `spec`, or it is `run-spec` or `finalize-spec` and every item in its Depends on is `done`. Specifying runs ahead of dependencies; building does not. `delivery.py actionable` lists them, and the Stop hook and the heartbeat act only when the list is not empty.

## RUN-LOG.md

```markdown
- 2026-01-05T09:00:00Z — run 2026-01-05T09:00:00Z started: scope MH-3, MH-5, spec:status-json
- 2026-01-05T10:15:32Z — MH-3: stage spec -> run-spec
- 2026-01-05T11:42:10Z — question Q-2 filed (MH-5): Approve spec status-panel
```

One line per event, oldest first, each stamped by `delivery.py` from the clock (`YYYY-MM-DDTHH:MM:SSZ`, UTC). Stage changes log themselves; `delivery.py log "<text>"` records anything else (a decision taken within the run's authority, a stop report, an unexpected commit found after a dispatch). Cite an entry by its timestamp, never by line number.

## QUESTIONS.md

```markdown
## Q-2 — Approve spec status-panel
- **Opened**: 2026-01-05T11:42:10Z
- **Status**: open
- **Items**: MH-5
- **Context**: specs/todo/status-panel/: 6 tasks; the panel reads the journal from MH-3.
- **Recommended default**: approve as written
- **Answer**: approved
- **Answered**: 2026-01-05T18:03:44Z
```

Ids are `Q-<n>`, numbered in the file. **Status** is `open` or `answered`. The maintainer answers with `python3 scripts/orchestration/delivery.py answer Q-<n> --text "<answer>"` from their own terminal (inside Claude Code, with the `!` prefix), or by editing the entry in their editor: setting Status to `answered` and adding the Answer line. The next `/deliver-backlog` applies the answer and unparks the items.
