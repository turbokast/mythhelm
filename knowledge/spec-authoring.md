# Spec authoring

The format of a MYTHHELM spec, and the patterns that make one implementable without guessing. The instructions are in `.claude/rules/spec-authoring.md` and `.claude/rules/spec-premise-grounding.md`; the lifecycle is in [`specs/README.md`](../specs/README.md); the task-breakdown method is `.claude/skills/spec-decomposition/SKILL.md`. The reference implementation of this format is the first spec in the repository, `specs/*/dogfood-slice/`.

---

## The files

| File | Written by | Holds |
|---|---|---|
| `requirements.md` | `/create-spec`, `/refine-spec`, or a person | Objectives, non-goals, numbered requirements and acceptance criteria, NFRs, Definition of Done. |
| `design.md` | `/spec` | Current state, the design by area, decisions, honesty register. |
| `tasks.md` | `/spec`; completion entries by the implementing agents | Dependency notes, shared gates, and one block per task. |
| `scratchpad.md` | `/spec` seeds it; every task appends | Open questions with their defaults, research notes, discoveries. |
| `handoff.md` | `/run-spec` seeds one section per task; each task fills its own | What each task produced and what its dependants must know; pasted into their dispatch prompts. |
| `refinement-log.md` | `/refine-spec` | Assessment rounds and the changes each made. |
| `plan.md` | `/spec-create-epic` | An epic's master plan; the only file in an epic directory. |

The master spec (`docs/spec/master-spec.md`) is normative for every spec. Its IDs are the vocabulary for tagging: invariants `I01`–`I19` (restated in `knowledge/invariants.md`), release gates `G01`–`G12` (§18.7), audit dispositions such as `A30` (§2.2), and section numbers such as `§11.2`.

---

## requirements.md

```markdown
## <Title> — Requirements

> One paragraph: what ships and what it is a slice of. Normative source: docs/spec/master-spec.md; § numbers, I-IDs and G-IDs refer to it.

### Objectives

- **O1**: <outcome a person can observe>

### Non-Goals (and what still binds)

| # | Out of scope | Still binding in this spec |
|---|---|---|
| N1 | <deferred item> | <invariant or gate that still applies to what does ship> |

## Functional Requirements

Acceptance criteria use EARS. Tags in brackets name the invariants, gates and sections each one serves.

### FR-1 — <name> (<§ refs>, <I-IDs>)

- **AC-1.1** [<tags>] When <trigger>, the system shall <response>.
- **AC-1.2** [<tags>] If <unwanted condition>, then the system shall <response>.

## Non-Functional Requirements

- **NFR-1** [<tags>] <measurable target>

## Definition of Done

- [ ] <checkable item, traceable to an FR>
```

- **IDs are stable.** `AC-3.2` is criterion 2 of `FR-3`. Tasks, completion entries and reviews cite these IDs, so an ID is retired, never reused or renumbered.
- **EARS forms:** ubiquitous ("The system shall …"), event-driven ("When …"), state-driven ("While …"), unwanted behaviour ("If …, then …"). Each names concrete values: an exit code, a field name, a state.
- **Non-goals defer, they do not waive.** The "still binding" column is what keeps a narrow slice honest.
- **An issue-derived spec** starts with a `## Context` section (the issue link and its load-bearing claims with their grounding verdicts) and ends with `## Open Questions`.

---

## design.md

- **Current state** comes first, cited as `file:line` or command output. A design built on an unverified premise inherits its errors.
- **Sections by area**, each citing the § it implements: package layout, state machines, data, interfaces, CLI, errors, tests, CI.
- **Interfaces are exact.** Every function, type or file format a later task consumes appears with its signature, including the failure cases it returns.
- **Decisions table.** `| ID | Decision | Rationale |`, one row per choice that had a real alternative, naming the alternative and why it lost. Choices about billing, persistence, process ownership or a public contract also get an ADR under `docs/decisions/`.
- **Honesty register.** `| Spec demand | Position |`, one row per master-spec demand in scope that this spec does not fully meet: deferred, blocked with a labelled reason, or partially met and how. It is the list a reviewer checks the receipt and README claims against.

---

## tasks.md

```markdown
## <Title> — Tasks

### Dependencies

- <prerequisite specs, or "None outside this spec">; which tasks may run in parallel.
- **Gates for every task.** <formatter, then the domain's gate commands>
- **Completion convention.** <how a finished task is marked; see below>

---

## Implementation Tasks

### Task N — <name>

- **Domain/agent**: <agent from knowledge/domains.md, or maintainer>
- **Budget**: standard | complex
- **Depends on**: Task M[, Task K] | None
- **Change**: <one or two sentences: what and why>
- **Files**:
  - `<path>` (<note>)
- **Produces**: <signatures later tasks consume> (when any)
- **Acceptance**:
  - <named test or command, and what it observes>
- **Test plan**: <how the tests are built>
- **Invariants touched**: <I-ID (§: what the task does to keep it)>
```

| Field | Required | Rules | Enforced by |
|---|---|---|---|
| `Domain/agent` | yes | The first word is an agent in `.claude/agents/` or `maintainer`; the Files list resolves to that agent's domain. | lint (agent name); validator (domain purity) |
| `Budget` | yes | `standard` or `complex`, optionally followed by a parenthesised note. | lint |
| `Depends on` | yes, except on the first task | `None`, or `Task N` references separated by commas; a parenthesised note may follow. Every reference exists, and there is no cycle. | lint |
| `Change` | yes | What changes and why, in one or two sentences. | lint (present) |
| `Files` | yes | Every file the task creates or edits, including test files its acceptance items name. | lint (present); validator (paths) |
| `Acceptance` | yes | Items that each name an observable that fails before the change and passes after it. `Acceptance criteria` is accepted as the field name. | lint (present); validator (quality) |
| `Invariants touched` | yes | IDs with the section relied on and what the task does to keep them, or `None (<why>)`. | lint (present); validator |
| `Produces`, `Test plan` | when relevant | Exact signatures; how tests are built. | validator |

The lint is `scripts/ci/lint-agent-harness.sh` (check `specs`); it parses `### Task N` headings outside code fences and `- **Field**:` bullets. The validator is the fresh-context review in `/spec-validate`.

**Completion entries.** An implementing agent appends ` ✅ COMPLETED` to its task's heading and adds `Status` (`✅ Completed — …; PR #<n>`), `Implementation` (with commit SHAs), `Spec deviations` (`None`, or each deviation with its reason) and `Files modified`, keeping every original field. `.claude/skills/task-completion/SKILL.md` is the authoritative format and `scripts/harness/runspec.py entry-check` checks it; the spec's own "Completion convention" bullet may add fields.

### Epic plans

An epic directory holds only `plan.md`: problem summary, current state, a Work Streams table, the dependency graph between sub-specs, the implementation order, open questions and risks. The Work Streams table is machine-read: column 2 holds each sub-spec's directory name in backticks, and the lint fails when a row has no backticked name or names a spec that has no directory. The plan's directory is never in `todo/` or `unfinalized/`.

```markdown
### Work Streams

| # | Spec | Scope | Domain | Dependencies |
|---|---|---|---|---|
| 1 | `<sub-spec-name>` | <one line> | core | None |
```

---

## Patterns that make criteria checkable

Each pattern is a defect shape that passes a casual review, and what catches it. `/spec-validate` checks for all of them.

- **No failing counterfactual.** An acceptance item for which no concrete state of the code makes it fail tests nothing. Naming that state is what shows the item bites.
- **Failure-only emitters.** An item watching a counter, event or log line that only the failure or retry path emits reads the same when the feature works as when it never ran; the success path has to execute the emitting line.
- **Unanchored text checks.** A file-wide match passes on a mention in prose or a history note. A check anchored to the region that must hold the deliverable (a section, a table, a fenced block) does not.
- **Empty universals.** "Every match is X" is true of zero matches, so it needs a non-empty leg. A count compared to zero exits non-zero when it prints 0; `! grep -q` does not.
- **Open quantifiers.** "Blocks any X" over an open space (a shell grammar, an input language) cannot be implemented by a static matcher. A bound requirement enumerates the classes it covers, gives each a failing fixture, and names the uncovered residual in the design.
- **Element instead of effect.** A button, flag or command that is present but does nothing passes a presence check. Its test observes what it changes: a journal row, an exit code, a file.
- **Missing input shapes.** A parser tested only on the shape its author imagined misses the shapes the producing component writes, including the absent field (which I09 requires to read `unknown`, never `0`).
- **Uncovered error branches.** A function whose documented failure returns have no acceptance item ships those branches untested.
- **Self-confirming fixtures.** A tool that generates fixtures, tested only on its own output, proves consistency with itself and nothing about real input.
- **Measurements that cannot tell their states apart.** A query or count without a fixture where the result differs from the all-clean value cannot show it discriminates anything.

## Patterns that keep Files lists complete

- **Acceptance items imply files.** A test file an item names is part of the task's Files list; omitting it turns planned work into a deviation.
- **Counts and "unaffected" claims are derived.** A stated number of call sites comes with the search that produced it; a site declared unaffected comes with the failure mode that was checked (`spec-premise-grounding.md`).
- **Widening a shared helper reaches its sibling callers**, which fail silently when a pattern match falls through; they belong in the same task or are fenced out explicitly.
- **Renames reach every inbound reference.** A heading, flag or symbol rename changes every file that cites the old name.
- **Documentation fixes order after the mechanism they describe**; otherwise one commit ships the fix beside text saying it does not exist.
- **Prescribed code must survive the formatter.** A body in `design.md` that `gofmt` would change forces its task to fail the gate or deviate.

---

## Worked example

A small single spec, shown in full, as `/spec` would leave it in `todo/`.

**requirements.md**

```markdown
## Doctor State Report — Requirements

> `mythhelm doctor` reports the state directory's location, permissions and free space, read-only. A slice of §5.1 and §17.5. Normative source: docs/spec/master-spec.md.

### Objectives

- **O1**: A user whose runs fail on a full disk can see why from `mythhelm doctor` without reading the journal.

### Non-Goals (and what still binds)

| # | Out of scope | Still binding in this spec |
|---|---|---|
| N1 | Cleanup of old runs (§17.6). | §17.6: nothing is deleted; doctor writes nothing (A30). |
| N2 | Free space on Windows. | I09: reported as `unknown`, never `0`. |

## Functional Requirements

### FR-1 — State directory report (§5.1, §17.5, A30, I09)

- **AC-1.1** [A30] `mythhelm doctor` shall not create, modify or delete any file; a test hashes the state directory before and after.
- **AC-1.2** [§17.5] When the platform reports free space, doctor shall print `state_dir_free_bytes` with the observed value and the label `observed`.
- **AC-1.3** [I09] If the platform cannot report free space, then doctor shall print `state_dir_free_bytes: unknown`, never `0`.
- **AC-1.4** [§5.6] With `--format jsonl`, doctor shall emit one `doctor.result` object carrying the same three fields.

## Definition of Done

- [ ] AC-1.1 to AC-1.4 each have a named test; CI green on Linux, macOS and Windows.
```

**design.md** (excerpt)

```markdown
### 1. Current state

- `internal/statedir/statedir.go`: `Resolve()` and `Ensure()`; nothing reports usage (every exported function listed with `go doc ./internal/statedir`).

### 4. Decisions

| ID | Decision | Rationale |
|---|---|---|
| D1 | `golang.org/x/sys/unix.Statfs` on Linux and macOS | Already a dependency; `syscall.Statfs` differs per OS and is frozen. |
| D2 | Windows reports `unknown` in this spec | `GetDiskFreeSpaceEx` needs its own tests on the Windows runner; deferred, not faked. |

### 5. Honesty register

| Spec demand | Position |
|---|---|
| §17.5 disk watermarks | Reported only; no watermark enforcement yet. |
| §17.5 on Windows | Free space `unknown` (D2). |
```

**tasks.md** (task blocks)

```markdown
### Task 1 — Free-space probe in statedir

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: None
- **Change**: Add `statedir.Free(dir string) (bytes uint64, known bool, err error)`, `known=false` where the OS cannot report, so doctor can label unknowns (I09).
- **Files**:
  - `internal/statedir/free_unix.go`
  - `internal/statedir/free_other.go`
  - `internal/statedir/statedir_test.go`
- **Produces**: `statedir.Free(dir string) (uint64, bool, error)`
- **Acceptance**:
  - `TestFreeReportsObservedBytes` (Unix): a temp dir returns `known=true` and a non-zero value.
  - `TestFreeUnknownOffUnix` (build-tagged): returns `known=false`, and the test fails if `free_other.go` returns `0, true`.
- **Test plan**: temp directories; build tags per OS.
- **Invariants touched**: I09 (§13.4: unknown stays distinct from zero).

### Task 2 — Doctor reports the state directory

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 1
- **Change**: Print the state directory path, mode and free space in `doctor`'s plain and JSONL output, read-only.
- **Files**:
  - `internal/cli/doctor.go`
  - `internal/cli/doctor_test.go`
- **Acceptance**:
  - `TestDoctorWritesNothing`: the SHA-256 of every file in the state directory is unchanged, and a doctor variant that touches a file makes the test fail.
  - `TestDoctorJSONLCarriesFreeBytes`: one `doctor.result` object with `state_dir_free_bytes` as a number or the string `unknown`.
- **Test plan**: call `cli.Main` with a temp state directory; golden plain output.
- **Invariants touched**: I09 (§13.4); A30 (read-only doctor).
```

What makes it pass review: every criterion names its test and the state that turns it red; the non-goals say what still binds; the Windows gap is a decision and a register row instead of a silent `0`; Task 2 depends on the exact signature Task 1 produces; each task's Files stay in one domain.
