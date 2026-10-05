---
paths:
  - "specs/**"
  - ".claude/skills/spec*/**"
  - ".claude/skills/create-spec/**"
  - ".claude/skills/refine-spec/**"
  - ".claude/skills/evaluate-spec/**"
---

# Spec Authoring

A spec is the contract an implementing agent works from and a reviewer judges against. Write it so neither has to guess. The format, with a worked example, is in `knowledge/spec-authoring.md`; `specs/README.md` has the lifecycle.

## requirements.md

- Number every objective (`O1`), non-goal (`N1`), requirement (`FR-1`) and acceptance criterion (`AC-1.1`, the requirement number, then the criterion). Never renumber a published ID: retire it and add a new one.
- Tag every acceptance criterion with what it serves, in brackets before the text: version-qualified master invariants (`I02`), gates (`G01`), acceptance cases (`AT-05`) and sections (`v2 §11.3`). Resolve the current master through `docs/spec/README.md`; historical audit IDs such as `A30` remain explicitly Revision 1.1. A harness spec cites the charter principle or rule it serves instead. A criterion that serves nothing is scope creep or a missing citation; find out which.
- Write criteria in EARS form ("When <trigger>, the system shall <response>"; "If <unwanted condition>, then …") with concrete values. Each one is checkable by a named test, a command or an inspectable artifact.
- Every non-goal names what still binds: the invariant or gate that applies to whatever the spec does ship. A spec narrows scope; it never quietly narrows an invariant.

## design.md

- Start from the current state, cited as `file:line` or command output (`spec-premise-grounding.md`).
- Record every choice that had a real alternative in a decisions table: `| ID | Decision | Rationale |`, IDs `D1`, `D2` and so on. A decision that changes billing, persistence, process ownership or a public contract also gets a decision record under `docs/decisions/`.
- End with an honesty register: a table of every master-spec demand in scope that this spec does not meet, and its position (deferred, blocked with a labelled reason, partially met). An unmet demand missing from the register is a defect, not a simplification.

## tasks.md

- Follow the task-block format in `knowledge/spec-authoring.md`. `scripts/ci/lint-agent-harness.sh` (check `specs`) enforces the fields, the dependency references and the absence of cycles.
- Each task's Files list resolves to one domain in `knowledge/domains.md`, and its Domain/agent names that domain's agent. Split cross-domain work into tasks joined by a dependency.
- Each acceptance item names the observable that goes red before the change and green after it. "Tests pass" or "file exists" alone is not a criterion.

## Everywhere in a spec

- Cite the spec itself as `specs/*/<name>/`, never by its current lifecycle path, which is wrong as soon as it moves.
- A line number is a hint: give the `grep` that finds the line by content next to it.
- An open question goes in `scratchpad.md` with its conservative default and who decides it. An open question is never permission to omit an invariant's implementation (v2 §19).
- Only the lifecycle skills create, move or archive a spec; implementing agents touch only their own task's completion entry and scratchpad note.

Evidence: `knowledge/rule-evidence/spec-authoring.md`.
