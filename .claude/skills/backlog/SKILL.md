---
name: backlog
description: View the product backlog in score order, recommend the next card to specify, file a new card with its GitHub issue, rescore cards or drop one — every change filed for maintainer approval
argument-hint: "show | next | add \"<idea or #issue>\" | rescore [MH-<n>] | drop MH-<n> \"<reason>\""
---

# Backlog

`product/backlog.md` is the plan of record: one card per piece of work, scored by `(value + urgency + risk) / effort` and kept in score order by `scripts/pm/pm.py`. The schema and the scoring rubric are at the top of that file. This skill reads it freely and changes it only through §File a change in `.claude/skills/pm-sync-core/SKILL.md`, which files the change for a maintainer's approval.

## Input

`$ARGUMENTS`: a subcommand. With none, `show`.

- `show`: the backlog in score order.
- `next`: the card to specify next, and the commands that take it there.
- `add "<idea>"` or `add #<issue>`: score and file a new card.
- `rescore [MH-<n>]`: rescore one card, or every open card after the current stage changed.
- `drop MH-<n> "<reason>"`: close a card that will not be done.

## Invocation contexts

- **Slash command**: `show` and `next` only read. `add`, `rescore` and `drop` present the draft and its scoring, and file approval requests once the person confirms (write gate); creating the GitHub issue for a new card is a second write gate, after the card is approved.
- **Model-invoked**: the same.
- **Non-interactive**: `show` and `next` run as usual. For `add`, `rescore` and `drop`, filing the approval requests is allowed without pre-approval, because nothing reaches `product/` until the maintainer signs; the draft and the request ids are the result. Creating a GitHub issue is public and needs explicit pre-approval in `$ARGUMENTS` or the dispatching prompt; without it, report the `gh issue create` command instead of running it.

## show

Run `python3 scripts/pm/pm.py list` (add `--status <s>` to filter, `--json` for fields). Present it grouped: in flight (`specced`, `implementing`), ready (`triaged`), ideas (`idea`), then closed. For each card give the id, title, score, stage and issue link.

## next

1. `python3 scripts/pm/pm.py next`: the highest-scored `triaged` or `idea` card whose stage is the current or next one ([`product/objectives.md`](../../../product/objectives.md)).
2. Read its linked issue (`gh issue view <n> -R turbokast/mythhelm --comments`) for discussion the card does not reflect yet.
3. Check that nothing already covers it: `grep -rlE "MH-<n>([^0-9]|$)" specs/` and the specs in `specs/in-progress/` and `specs/todo/`.
4. Present the card, why it is first (its score inputs against the runner-up) and the path to shipping it:

```text
Next: MH-<n> — <title> (score <s>, stage <stage>, issue #<i>)
Why now: <one or two sentences from the score inputs and the stage exit gate>
Then: /create-spec MH-<n> → /refine-spec <name> → /spec <name> → run-spec <name> → finalize-spec <name>
```

## add

1. **Source.** For `#<issue>`, read it with `gh issue view <n> -R turbokast/mythhelm --json number,title,body,labels,comments,url`. For free text, use it as given.
2. **Triage** it with `.claude/skills/triage/SKILL.md` (duplicates, commitments, score). Stop at a duplicate or a commitment conflict and report it: that is the answer, not a card.
3. **Ground the premise** (`.claude/rules/product-management.md` §Premise grounding): a Summary that says something is missing or broken today is checked at source before it is filed.
4. **Present** the draft card: title, stage, gates, the four score inputs with one line of reasoning each, the resulting score, where it would rank (`pm.py list`), source sections and summary. Ask to file it (write gate).
5. **File** with §File a change: `pm.py --product-dir "$STAGE" add --title ... --stage ... --gates ... --value ... --risk ... --effort ... --source ... --summary ... [--issue <n>] --out "$STAGE/backlog.md"` (the verb reserves the id), then `decide --type card-add` naming the card, then the roadmap. The id exists only from this step on.
6. **Issue.** Once the card is approved and committed, and only if it has no issue yet, create one (second write gate):
   `gh issue create -R turbokast/mythhelm --title "<title> (MH-<n>)" --label <labels> --body-file <file>`. Labels come from the existing set (`gh label list -R turbokast/mythhelm`): `enhancement` plus the area (`adapter`, `host`, `routing`, `tui`, `billing`, `plugins`, `protocol`, `ci`, `documentation`, `security`, `harness`). The body links the card and cites the spec sections, in public-safe words (`.claude/rules/public-repo-hygiene.md`). Never create milestones or projects. Then file `pm.py ... set MH-<n> issue <number>` through §File a change.

When the idea came from an issue, step 5 passes `--issue <n>` and step 6 is skipped; comment on the issue only when the person asks.

## rescore

1. `rescore MH-<n>`: re-derive value, risk and effort from current evidence (new signals, impact reviews, a changed dependency), present old and new inputs with the reason for each change, and on confirmation file `pm.py ... rescore MH-<n> [--value N] [--risk N] [--effort N] [--stage S]` plus a `rescore` decision.
2. `rescore` with no card: after `product/objectives.md`'s current stage changes, urgencies are stale and the `product` lint fails. `pm.py ... rescore` recomputes every open card's urgency; file it together with the `objective-change` decision that moved the stage.
3. Present the new order (`pm.py --product-dir "$STAGE" list`) and the cards whose rank changed.

## drop

Confirm the reason, then file `set-status MH-<n> dropped`, a `card-drop` decision whose rationale gives the reason, and the roadmap. Close the card's issue only when the person asks, with a comment linking the decision.

## Output

The listing, the recommendation, or for a change: the draft, the request ids, and the maintainer's commands (`scripts/orchestration/approve.sh show <id>` then `approve <id> --apply`).
