---
name: spec-create-single
description: Write a single spec's requirements.md, design.md, tasks.md and scratchpad.md from the investigation findings, in the format of knowledge/spec-authoring.md
argument-hint: "<spec-name | \"feature description\">"
---

# Spec Create Single

Phase 3a of `/spec`: writes the files of one spec. The format of every file, with a worked example, is `knowledge/spec-authoring.md`; the instructions are `.claude/rules/spec-authoring.md` and `.claude/rules/spec-premise-grounding.md`. Read all three before writing.

## Input

`$ARGUMENTS`: the spec name (mode A, `refined/`) or the description (mode C), with the phase-1 findings in context.

## Invocation contexts

- **Slash command**: writes the files, then hands over to validation.
- **Model-invoked**: the same.
- **Non-interactive**: the same. The files are drafts in `unrefined/` or `refined/` until validation passes, so writing them needs no confirmation; an unanswerable question goes into `scratchpad.md` as an open question with its default.

## Steps

1. **Locate.**
   - Mode C: create `specs/unrefined/<name>/` and write all four files.
   - Mode A: write into `specs/refined/<name>/`. Keep `requirements.md` and `refinement-log.md` as they are; a requirement that turns out wrong during design is a finding for `/spec-fix-and-report` to escalate, not an edit made here.
2. **requirements.md** (mode C only): objectives, a non-goals table with what still binds, `FR-N` sections with `AC-N.M` criteria in EARS form tagged with invariant, gate and section IDs, NFRs and a Definition of Done.
3. **design.md**:
   - current state, cited as `file:line` from the investigation;
   - the design by area, each section citing the master-spec § it implements; exact signatures for every interface a later task consumes, failure cases included;
   - a decisions table (`D1`…) for every choice that had an alternative;
   - an honesty register listing every in-scope master-spec demand the spec does not fully meet;
   - for work above 12 tasks that stays one spec, why it was not split (`/spec-scope`);
   - cross-spec references as `specs/*/<other>/`.
4. **tasks.md**: run `/spec-decomposition` and write its result. The header carries the Dependencies notes (prerequisite specs, which tasks may run in parallel), the gates every task runs, and the completion convention. Every task block has the fields listed in `knowledge/spec-authoring.md` § tasks.md.
5. **scratchpad.md**: skip if it exists. Otherwise seed it:

   ```markdown
   # <Title> — Scratchpad

   > Open questions and research from authoring. Each implementing task reads this file when it starts and adds an entry under Discoveries when it finishes.

   ## Open questions

   | # | Question | Default in this spec | Owner / blocks |
   |---|---|---|---|

   ## Research notes

   ## Discoveries
   ```

   Move every unresolved question from the investigation into the table, each with the conservative default the spec already assumes.
6. **Keep it lean.** Every task and review re-reads these files. Cut restated context, rejected alternatives beyond one line in the decisions table, and history; keep every constraint, every exact signature and every failing counterfactual.
7. **Exit gate**: `scripts/ci/lint-agent-harness.sh --only specs` passes.

## Output

The paths written, the task count, and the lint result.
