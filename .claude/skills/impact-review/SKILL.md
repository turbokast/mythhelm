---
name: impact-review
description: After a card ships, gather public evidence of whether it did what it promised (CI history, evaluation results, issues and discussions since the ship) and file the conclusion as a decision, with any follow-up cards proposed
argument-hint: "[MH-<n>]"
---

# Impact Review

A shipped card promised something: a gate closer to passing, a risk retired, a user need met. There is no telemetry to read (spec §17.2), so the evidence is public: CI runs on `main`, evaluation results, the card's own acceptance evidence, and what users reported after it shipped. This skill gathers it, concludes, and records the conclusion in `product/decisions.md`.

## Input

`$ARGUMENTS`: a card id, or nothing to review every due card. A shipped card is due when its `shipped` lifecycle-sync decision is at least 30 days old, or a release has been cut since, and no `impact-review` decision names it yet.

## Invocation contexts

- **Slash command**: gathers the evidence, proposes the conclusion and files the decision once the person confirms it (write gate). Follow-up cards go through `/backlog add` one by one.
- **Model-invoked**: the same.
- **Non-interactive**: gathers the evidence and files the `impact-review` decision request, which needs no pre-approval because the maintainer signs it later. Follow-up cards are returned as proposals only.

## Steps

1. **Find the due cards.** `python3 scripts/pm/pm.py list --status shipped --json`, then read `product/decisions.md` for each card's `lifecycle-sync` entry that moved it to `shipped` (its date and evidence) and for any `impact-review` entry naming it. No due card: report the next due date and stop.
2. **Recall the promise.** Read the card (`pm.py show MH-<n>`): its gates, source sections and summary, and the requirements of each spec it names (`scripts/harness/spec-lifecycle.sh resolve <spec>`), especially the acceptance criteria and the objectives.
3. **Gather evidence**, read-only, and cite every item:
   - **CI**: `gh run list -R turbokast/mythhelm --branch main --workflow ci.yml --limit 30 --json conclusion,createdAt,url` since the ship date; any failures in the packages or jobs the spec touched (`gh run view <id> --log-failed`).
   - **Evaluations**: results the spec's acceptance evidence names (spec §18: native fidelity, fault injection, the release matrix) re-run or recorded since, where they exist in the repository.
   - **Reports**: `gh issue list -R turbokast/mythhelm --state all --search "<area terms> created:>=<ship date>"` and the card's own issue thread; the discussions query from `.claude/skills/synthesize-signals/SKILL.md`; recent entries in `product/signals.md` that name the card.
   - **Gates**: for each gate the card names, whether its §18.7 pass condition is now closer, met or unchanged, with the test or record that shows it.
4. **Conclude** one of: `held` (the evidence supports the promise), `partial` (some of it; say which), `regressed` (evidence against it; link it) or `inconclusive` (not enough evidence; say what would settle it). Never conclude `held` from the absence of reports alone: absence of complaints is not evidence of use.
5. **Propose follow-ups** where the evidence calls for them: a card for a regression or a gap, a rescore of a related card, or a way to measure what was inconclusive. Each is a one-line idea for `/triage`.
6. **Present and file.** Show the evidence table and the conclusion; on confirmation, file an `impact-review` decision through §File a change (`.claude/skills/pm-sync-core/SKILL.md`): `decide --type impact-review --title "Impact of MH-<n>: <conclusion>" --decision "<conclusion and what it rests on>" --rationale "<the key evidence>" --cards MH-<n> --evidence "<links>"`.

## Output

```markdown
## Impact review: MH-<n> — <title> (shipped <date>)

| Evidence | Source | Reading |
|---|---|---|
| CI on main | <runs link> | <n> of <m> green; failures: … |
| Gate G<nn> | <test or record> | closer | met | unchanged |
| Reports | <issue links> | … |

Conclusion: held | partial | regressed | inconclusive — <why>
Follow-ups: <ideas for /triage, or none>
Request: <id>
```
