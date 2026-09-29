# Orchestration

Local state for agent-driven delivery. Everything here except this file is gitignored and stays on the maintainer's machine, in the main checkout, shared by its linked worktrees.

| Path | What it is | Written by |
|---|---|---|
| `approvals.jsonl` | The approval ledger: signed maintainer decisions on requested changes, and a `consumed` row for each write an approval released. Append-only. | `scripts/orchestration/approvals.py` only |
| `requests/<id>/` | One request per proposed change: `request.json`, the full `proposed` file and its `diff`. | Agents, through `approvals.py request` |

The flow and the schemas are in [`.claude/hooks/README.md`](../.claude/hooks/README.md) (§Product approvals), and the rule it serves is in [`product/README.md`](../product/README.md). Agents never write the ledger or run `scripts/orchestration/approve.sh`; `guard-product-write.sh` blocks both.
