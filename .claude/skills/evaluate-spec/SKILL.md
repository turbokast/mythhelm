---
name: evaluate-spec
description: Compare an existing spec with the current codebase and classify it current, needing an update, or to archive; update it in place or archive it, never delete it
argument-hint: "<spec-name>"
---

# Evaluate Spec

Specs go stale: code moves on, a task is done but unrecorded, a design section describes a module that was renamed, or the whole spec is superseded. This skill checks a spec against the repository as it is today and either confirms it, updates it, or archives it.

## Input

`$ARGUMENTS`: the spec name.

## Invocation contexts

- **Slash command**: evaluates, shows the classification and the proposed edits or move, and applies them on confirmation (step 5).
- **Model-invoked**: the same.
- **Non-interactive**: evaluates and classifies. Edits and moves are a write gate: applied only with explicit pre-approval (`--confirm` in `$ARGUMENTS` or the dispatching prompt), otherwise returned as proposals with the note that writes were withheld.

## Steps

1. **Resolve** with `scripts/harness/spec-lifecycle.sh resolve <name>` and read every file in the directory.
2. **Completion status.** For each task: a task marked complete has its implementation in the tree (the files and the commit it cites); an unmarked task is either genuinely open or done but unrecorded (find the commit with `git log --oneline -- <its files>`).
3. **Verify against the code** (`.claude/rules/spec-premise-grounding.md`): for each requirement and design section, does the code exist, does its behaviour match, and has the implementation gone beyond the spec? Open the code; do not infer from names. Check every cross-reference to another spec and to the master spec.
4. **Classify.**
   - **Current**: tasks, design and code agree; references resolve.
   - **Needs update**: the spec is still the plan or the record, but parts are wrong: stale paths or signatures, done-but-unrecorded tasks, a design that no longer matches, missing sections of the current format (`knowledge/spec-authoring.md`).
   - **Archive**: superseded by another spec, describing removed code, never implemented and no longer planned, or duplicating another spec.
5. **Act** (write gate).
   - Needs update: edit in place. Fix stale paths, signatures and current-state text in `design.md`; bring missing sections up to the current format; for a done-but-unrecorded task, add its completion entry with the commit SHA as evidence (never without one); mark a no-longer-needed task in its own entry with the reason, rather than deleting it. Requirements change only to record what was actually decided, with the reason in `scratchpad.md`.
   - Archive: append the reason and the superseding spec or commit to `scratchpad.md` (create it if absent), then `scripts/harness/spec-lifecycle.sh move <name> archived`. Never delete a spec directory: the archive is the record.
   - A spec in `in-progress/` is being implemented by other agents: propose, and apply only with a person's confirmation even in an interactive session.

## Output

```markdown
## Spec evaluation: <name>
Classification: CURRENT | UPDATED | ARCHIVED | PROPOSED (writes withheld)
Evidence: <file:line or commit per finding>
Changes: <file — what changed>, or the proposed changes
Remaining gaps: <what the code could not settle>
```
