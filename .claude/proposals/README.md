# Proposals

The harness learns from its own runs. When a spec is finalized, `/finalize-spec-retrospective` turns what went wrong, or what took more effort than it should have, into **proposals**: concrete changes to a rule, a skill, a hook, the knowledge base or the product backlog. A proposal is a suggestion with its evidence. Nothing here changes the harness until a maintainer accepts it.

| File | Holds | Written by |
|---|---|---|
| `pending.md` | Proposals awaiting a maintainer's decision | `/finalize-spec-retrospective`, inside the finalize pull request |
| `applied.md` | Decided proposals and their outcome (added by the apply side of the loop) | the skill that applies proposals |

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

- **Id.** `P-<spec>-<n>`: the spec that produced it and a sequence number within that spec. `python3 scripts/harness/finalize.py proposal-id --spec <spec>` prints the next free one. Ids never collide between two finalize pull requests open at once, because each spec numbers only its own proposals.
- **Type.** What the change edits. `rule` is `.claude/rules/`, `skill` is `.claude/skills/`, `hook` is `.claude/hooks/` (with its registration and tests), `knowledge` is `knowledge/`, and `product` is a backlog change, which still goes through the product approval flow when it is applied.
- **One change per proposal**, aimed at one target. Two targets make two proposals.
- **Append only.** Add new sections at the end. Never edit or remove another spec's proposal. Deciding a proposal moves it to `applied.md`.
- **Conflicts.** Two finalize pull requests that both append to `pending.md` conflict textually. Merge `origin/main` into the later branch and keep both sections; their ids are distinct by construction.

`scripts/ci/lint-agent-harness.sh` (check `proposals`) enforces the heading, the unique ids, the five fields, the type list, a source spec that exists, a target that exists or is marked `(new file)`, and a non-empty proposed change.

## Where proposals come from

The retrospective proposes a change only when its evidence points at the harness rather than at the task: an ambiguity in a spec format that two tasks resolved differently, a gate that passed a defect the review then found, a skill step that every run skipped or improvised, a CI failure class that recurred. A one-off mistake with no mechanism behind it is recorded under Lessons, not proposed.
