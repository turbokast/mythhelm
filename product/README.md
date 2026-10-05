# Product

The product layer decides what MYTHHELM builds next and records why. It sits between the [current master](../mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md), which says what the product must be, and [`specs/`](../specs/README.md), which says how each piece is built. This directory is contributor tooling, like the rest of the development harness: none of it ships in a release.

## Files

| File | What it holds | Changed by |
|---|---|---|
| [`objectives.md`](objectives.md) | The stages, release gates and commitments of the master spec that the backlog scores against, and the current stage. | Maintainers, as an `objective-change` decision. |
| [`backlog.md`](backlog.md) | Every planned piece of work as a card `MH-<n>`: status, stage, gates, score, spec and GitHub issue. The plan of record. | `/backlog`, `/triage`, `/create-spec` and the lifecycle sync, all through an approval. |
| [`decisions.md`](decisions.md) | The append-only log of product decisions, `D-<n>`. | Every skill that changes the backlog or objectives appends one entry. |
| [`roadmap.md`](roadmap.md) | Now, next and later: a view generated from the backlog. | `/roadmap`, redrafted whenever the backlog changes. |
| [`signals.md`](signals.md) | Themes from GitHub issues, discussions and pull requests, `S-<n>`. | `/synthesize-signals`. |

The [specification index](../docs/spec/README.md) preserves historical revisions; the [adoption map](../docs/spec/synthesis-adoption.md) connects W01–W16 to cards. Essential local run/recovery evidence is distinct from opt-in learning and external sharing (v2 §§8.4,12,17). Product-management evidence stays public: CI, published evaluations, issues and discussions; this directory contains no private usage dataset. `/impact-review` gathers it and records the conclusion in `decisions.md`.

Each card has a matching GitHub issue labelled for its area. The issue is where anyone can discuss a card; the card is where its status and score live. `/backlog` keeps the two linked.

## The approval rule

**Agents propose; maintainers approve.** No agent changes a file in this directory without a maintainer's signed approval of that exact change.

In Claude Code the rule has teeth. `.claude/hooks/guard-product-write.sh` blocks every agent write here unless a signed approval matches both the file's current content and the content the write would produce, and each approval releases one write. The hook also blocks agents from running the approval command or touching the approval ledger and key. Agents in other tools have no hooks and follow the same rule by hand. Pull request review is the final gate for everyone.

The flow:

1. **Draft.** An agent drafts the change with `python3 scripts/pm/pm.py <verb> ... --out <scratch file>`. For objectives, this README or the backlog preamble, use `draft-text objectives|readme|backlog-preamble --from <scratch-source> --out <staged-file>`. The tool never writes here itself; validate the complete staged set after drafting and rescoring.
2. **Request.** The agent files it: `python3 scripts/orchestration/approvals.py request <id> --path product/<file> --proposed <scratch file> --summary "<why>"`. The request records the hash of the current file and of the proposed one, and the diff between them.
3. **Approve.** A maintainer, in their own terminal, reviews and signs it: `scripts/orchestration/approve.sh show <id>`, then `scripts/orchestration/approve.sh approve <id>`, or `approve <id> --apply` to also write the file. `reject <id>` declines it.
4. **Apply.** Unless the maintainer applied it, the agent writes exactly the proposed content. The hook releases the write only if the file still has the content the approval saw, so a change that landed in between makes the approval stale instead of being overwritten. Then the change is committed and reviewed in a pull request like any other.

Humans editing these files in their own editor are not affected by the hook; the pull request review and the `product` check of `scripts/ci/lint-agent-harness.sh` apply to every change. How the pieces fit together is in [`docs/harness/product-management.md`](../docs/harness/product-management.md), and the rules agents follow are in `.claude/rules/product-management.md`.
