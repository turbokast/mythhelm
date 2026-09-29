---
name: spec-investigate
description: Ground a planned spec in the codebase before any scoping or writing — the master-spec sections, the invariants, the domains, the existing packages and data flows, and conflicting specs
argument-hint: "<spec-name | \"feature description\">"
---

# Spec Investigate

Phase 1 of `/spec`, and the phase that decides the spec's quality. It reads; it writes nothing. Its findings feed scoping and creation, and are deliberately **not** passed to the validator, whose value is a reading uninfluenced by this one.

## Input

`$ARGUMENTS`: a description, or the name of a spec in `refined/` whose `requirements.md` is then the ground truth.

## Invocation contexts

- **Slash command**: investigates and prints the findings.
- **Model-invoked**: the same, normally from `/spec`.
- **Non-interactive**: the same. Read-only, so there is nothing to confirm; a question only a person can answer is recorded as an open question, not asked.

## Steps

1. **Read the normative context.**
   - The master-spec sections the change implements or touches (`docs/spec/master-spec.md`; use its table of contents, read the sections, not the whole file). Note every MUST, invariant and gate that applies.
   - `knowledge/invariants.md`: the invariants the change can reach, and how a task cites them.
   - `knowledge/domains.md`: the domains and agents the paths will involve.
   - Decision records under `docs/decisions/` in the same area, so the spec does not re-argue a settled choice.
   - `.claude/skills/spec-decomposition/SKILL.md`, for what the tasks will need to know.
2. **Read the code**, not file names.
   - The packages involved: `go doc ./internal/<pkg>` for the exported surface, then the functions themselves. Record exact signatures, return shapes and failure cases for everything the design will call or change.
   - Trace each affected flow end to end: CLI command → admission or supervisor → workspace, journal or adapter → back to output and exit code.
   - Find the existing patterns the change must follow or replace (search for similar features), and the tests that cover the area today.
   - For every count or absence the spec will state ("the two callers", "no code writes X"), run the search and record the command and its hits (`.claude/rules/spec-premise-grounding.md`).
3. **Find the seams.** Which parts could ship independently? What is the smallest first step that is useful alone? Which contract (a type, an interface, a schema) must land before its consumers?
4. **Research the unknowns.** For an external tool or native harness behaviour, read its current documentation and record the version and the source; what cannot be settled becomes an open question with a conservative default.
5. **Scan for conflicts** in every lifecycle state: `specs/in-progress/` (highest risk), `specs/unfinalized/`, `specs/todo/`, `specs/refined/`, `specs/unrefined/`; and `specs/done/` for decisions to reuse. Note any spec that edits the same files or packages.

## Output

Findings for the next phases, kept in the session:

```text
Master spec: <§ sections>; invariants in reach: <I-IDs>; gates: <G-IDs>
Packages and facts: <path:line — fact>, one per line, each opened
Flows traced: <flow — path:line chain>
Seams: <independent parts and the contracts between them>
Conflicts: <spec — overlapping files> | none
Open questions: <question — conservative default — who decides>
```
