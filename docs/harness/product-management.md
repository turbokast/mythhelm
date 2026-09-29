# Product management

The product layer is the part of the development harness that decides what MYTHHELM builds next. This page is the maintainers' guide to it: the loop it runs, what an agent does and what only a maintainer does. It belongs in `WORKFLOW.md` once that file exists. The agent-facing detail is in [`knowledge/product-management.md`](../../knowledge/product-management.md), and the files themselves are introduced in [`product/README.md`](../../product/README.md).

## The loop

```text
GitHub issues, discussions ──/synthesize-signals──▶ signals.md (themes)
                                                        │
idea or #issue ──/triage──▶ assessment ──/backlog add──▶ backlog.md card MH-<n> + GitHub issue
                                                        │
                           /backlog next ──▶ /create-spec MH-<n> ──▶ specs/unrefined/<name>/   card: specced
                                                        │
                           /refine-spec ──▶ /spec ──▶ run-spec                                 card: implementing
                                                        │
                                               finalize-spec                                   card: shipped
                                                        │
                                   /impact-review (CI, evaluations, reports) ──▶ decisions.md
                                                        │
                                  /quarterly-review ──▶ adjustments ──▶ /backlog rescore, add, drop
```

`/roadmap` redrafts the now, next and later view whenever cards move. Every step that changes `product/` records why in `decisions.md`.

## Who does what

**Agents** read everything, score, draft and file. A draft is a complete proposed file built by `scripts/pm/pm.py`; filing it creates a request under the main checkout's `orchestration/requests/` and changes nothing in `product/`. Agents create a GitHub issue for a new card once the card is approved, when the person running the session asks for it.

**Maintainers** decide. From your own terminal:

```bash
scripts/orchestration/approve.sh init-key          # once per machine: creates your signing key
scripts/orchestration/approve.sh list              # requests and their state
scripts/orchestration/approve.sh show <id>         # summary and diff
scripts/orchestration/approve.sh approve <id> --apply
scripts/orchestration/approve.sh reject <id>
scripts/orchestration/approve.sh audit             # re-verify every signed decision
```

`approve --apply` writes the file for you; without `--apply`, the agent writes exactly the approved content and the hook checks it. Either way, the change reaches `main` through a pull request like any other, and the `product` check of the harness lint runs on it in CI.

An approval covers one file's exact change against its exact current content, and releases one write. If the file changes before the write, the approval goes stale and the agent refiles: nothing is silently overwritten. A person can still edit `product/` by hand in an editor; the hook binds only agent tool calls in Claude Code, and the lint and review bind everyone.

## Cadence

- **Per change**: `pm-sync-core` files the card's status change at each spec lifecycle step, so the backlog moves with the specs.
- **Monthly, or before a release**: `/synthesize-signals` and `/impact-review` for cards shipped at least a month ago.
- **Quarterly**: `/quarterly-review`, which also checks whether the current stage's exit gate has passed. Advancing the stage is an `objective-change` decision filed together with `/backlog rescore`.

## Scoring in one paragraph

`score = (value + urgency + risk) / effort`, each input 1 to 5. Urgency is not chosen: it is 5 for work in the current stage (or earlier), 3 for the next stage, 2 for the one after and 1 beyond, so the spec's gated stages, not calendar dates, set time pressure. The backlog is kept in score order by the tool, and the lint rejects a score that does not match its inputs or its stage.
