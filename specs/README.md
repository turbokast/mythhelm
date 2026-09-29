# Specs

Every planned change larger than a single fix is a spec: a directory named in kebab-case (`specs/<state>/<name>/`) that carries it from an idea to shipped code. The directory's parent is its lifecycle state. Moving between states renames the directory and changes nothing inside it.

How to write one is in [`knowledge/spec-authoring.md`](../knowledge/spec-authoring.md). The lifecycle skills live in `.claude/skills/` and are listed under [Who moves specs](#who-moves-specs).

## States

| State | Holds | Required files |
|---|---|---|
| `unrefined/` | A captured idea or issue: rough requirements, not yet assessed. | `requirements.md` |
| `refined/` | Requirements assessed until a design can be written from them without guessing. | `requirements.md` |
| `todo/` | A validated spec, ready to implement. | `requirements.md`, `design.md`, `tasks.md` |
| `in-progress/` | A spec being implemented; some tasks are complete. | `requirements.md`, `design.md`, `tasks.md` |
| `unfinalized/` | Every task complete; awaiting the finalize review and CI verification. | `requirements.md`, `design.md`, `tasks.md` |
| `done/` | Finalized and shipped. Kept as the record of what was built and why. | `requirements.md`, `design.md`, `tasks.md` |
| `archived/` | Abandoned or superseded, never deleted. | `requirements.md` |

A spec directory also holds `scratchpad.md` (discoveries appended by each task, read at the start of the next) and, after refinement, `refinement-log.md`. An **epic** is work too large for one spec: its directory holds only `plan.md`, whose Work Streams table names the sub-specs. The sub-specs are ordinary spec directories, siblings of each other, never nested inside the epic's directory.

## Transitions

| From | To | By |
|---|---|---|
| (new) | `unrefined/` | `/create-spec` from an issue or a description; `/spec` while it drafts from a description |
| `unrefined/` | `refined/` | `/refine-spec`, when its assessment passes |
| `refined/` | `refined/` | `/refine-spec` again, to re-polish |
| `unrefined/` or `refined/` | `todo/` | `/spec`, after design and tasks pass validation |
| `todo/` | `in-progress/` | the implementation run, when it starts the first task |
| `in-progress/` | `unfinalized/` | the implementation run, when the last task completes |
| `unfinalized/` | `done/` | the finalize step, after review and CI |
| any state | `archived/` | `/evaluate-spec`, when a spec is stale, superseded or abandoned |

An epic's `plan.md` directory moves `unrefined/` → `refined/` (optional) → `in-progress/` when its first sub-spec starts, and → `done/` when its last one ships. It never enters `todo/` or `unfinalized/`.

## Who moves specs

Only the lifecycle skills move a spec, and they move it with `scripts/harness/spec-lifecycle.sh move <name> <state>`, which refuses any move not in the table above. Never move a spec directory by hand with a plain `mv`: it leaves the old copy in the git index, where the next checkout brings it back.

| Skill | Role |
|---|---|
| `/create-spec` | Issue or idea → `unrefined/<name>/requirements.md` |
| `/refine-spec` | Assess and fix requirements until ready → `refined/` |
| `/spec` | Investigate, choose single spec or epic, write design and tasks, validate → `todo/` |
| `/evaluate-spec` | Compare a stale spec with the code → update it, or archive it |
| `/spec-resolution` | Reference: how every skill finds a spec by name |

`/spec` runs its phases through `/spec-investigate`, `/spec-scope`, `/spec-create-single` or `/spec-create-epic` (which follow `/spec-decomposition`), `/spec-validate` and `/spec-fix-and-report`.

## Invariants of this tree

- A spec name exists in exactly one state, across the working tree and the index. `scripts/harness/spec-lifecycle.sh resolve <name>` finds it and refuses duplicates.
- A spec never cites its own lifecycle path: it writes `specs/*/<name>/`, which stays true when the directory moves.
- `scripts/ci/lint-agent-harness.sh` (check `specs`) enforces the required files per state, the task-block fields in `tasks.md`, resolvable task dependencies with no cycle, epic Work Streams that name real specs, and the single-copy rule.
