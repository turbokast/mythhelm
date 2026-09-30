# Proposals

The harness learns from its own runs. When a spec is finalized, `/finalize-spec-retrospective` turns what went wrong, or what took more effort than it should have, into **proposals**: concrete changes to a rule, a skill, a hook, the knowledge base or the product backlog. A proposal is a suggestion with its evidence. Nothing here changes the harness until a maintainer accepts it.

| File | Holds | Written by |
|---|---|---|
| `pending.md` | Proposals awaiting a maintainer's decision | `/finalize-spec-retrospective`, inside the finalize pull request; `/research-practices` |
| `applied.md` | Decided proposals and their outcome, append only | `scripts/harness/proposals.py record`, inside each decision's pull request (`/apply-proposals`) |
| `auto-apply.json` | The lane-0 switch and allowlist; off by default | maintainers only |

Both files are public. A proposal names mechanisms and generic hazards, never private incidents, people, hosts or transcripts (`.claude/rules/public-repo-hygiene.md`).

## Format

Each proposal in `pending.md` is one level-2 section:

```markdown
## P-<spec>-<n> — <title, one line>

- **Source spec**: `<spec>`
- **Type**: rule | skill | hook | knowledge | product
- **Target**: `<repository-relative path>` (add ` (new file)` when it does not exist yet)
- **Rationale**: <what went wrong or cost effort, and why this change prevents it>
- **Evidence**: <where it shows: a review finding at file:line, a pull request, a CI run URL, a run-events row, a retrospective section>

**Proposed change:**

<the change itself: the rule text, the skill step, the hook behaviour, the knowledge entry or the backlog card, precise enough to apply without the author>
```

- **Id.** `P-<spec>-<n>`: the spec that produced it and a sequence number within that spec. `python3 scripts/harness/finalize.py proposal-id --spec <spec>` prints the next free one, counting both `pending.md` and `applied.md`, so a decided id is never reused. Ids never collide between two finalize pull requests open at once, because each spec numbers only its own proposals and a spec is finalized once. A proposal from `/research-practices` uses the source `research-<yyyymmdd>` in place of a spec and cites its sources as `https://` URLs in **Evidence**; two research runs on the same day would pick the same id, and the second one's pull request then conflicts on `pending.md` and fails the duplicate-id lint until it is renumbered.
- **Type.** What the change edits. `rule` is `.claude/rules/`, `skill` is `.claude/skills/`, `hook` is `.claude/hooks/` (with its registration and tests), `knowledge` is `knowledge/`, and `product` is a backlog change, which still goes through the product approval flow when it is applied.
- **One change per proposal**, aimed at one target. Two targets make two proposals.
- **Append only.** Add new sections at the end. Never edit or remove another spec's proposal. Deciding a proposal moves it to `applied.md` (below).
- **Conflicts.** Two finalize pull requests that both append to `pending.md` conflict textually. Merge `origin/main` into the later branch and keep both sections; their ids are distinct by construction.

`scripts/ci/lint-agent-harness.sh` (check `proposals`) enforces the heading, the unique ids, the five fields, the type list, a source spec that exists (or a research source with a cited URL), a target that exists or is marked `(new file)`, and a non-empty proposed change.

## Decisions: `applied.md`

`/apply-proposals` asks a maintainer for a decision on each proposal: approve, reject or defer (deferring leaves it pending). Each decision lands as a pull request: an approved proposal's pull request changes its **Target** and nothing else (plus a rule's evidence file, or a hook's registration, inventory and test), and every decision's pull request moves the section from `pending.md` to the end of `applied.md`, where `proposals.py record` puts these fields right under the heading, ahead of the original ones:

```markdown
## P-<spec>-<n> — <title>

- **Decision**: approved | rejected | auto-applied
- **Date**: <YYYY-MM-DD>
- **Pull request**: #<n>
- **Eval**: `<case-id>` | waived — <the maintainer's reason> | n/a
- **Rationale**: <the maintainer's reason>
- **Source spec**: ...
```

- **Append only.** An entry is never edited or removed; `proposals.py apply-check` refuses an `applied.md` whose old text changed. The only exception is filling `Pull request: pending` with the number once the pull request exists (`record <id> --pr <n>`), inside that same pull request.
- **Eval.** An approved `rule`, `skill` or `hook` proposal fixes a harness failure, so it lands with an eval case in `.claude/evals/cases/` whose `source` names the proposal and which fails without the change and passes with it; `apply-check` runs it against both trees. Only a maintainer can waive it, and the waiver's reason is recorded.
- **Lane 0.** A `knowledge` proposal whose target is an existing file matching an allow glob in `auto-apply.json` (`enabled: true`, globs under `knowledge/`) is applied without a per-item decision: its **Proposed change**, and nothing else of the proposal, is appended to the target in its own pull request, recorded as `auto-applied`, and vetoed by reverting that pull request. The configuration is read at the base, so a pull request cannot enable itself.

The `proposals` lint checks `applied.md` too: well-formed headings, unique ids that are not also pending, the decision fields, and an existing eval case or a waiver for every approved rule, skill or hook.

## Where proposals come from

The retrospective proposes a change only when its evidence points at the harness rather than at the task: an ambiguity in a spec format that two tasks resolved differently, a gate that passed a defect the review then found, a skill step that every run skipped or improvised, a CI failure class that recurred. A one-off mistake with no mechanism behind it is recorded under Lessons, not proposed.
