---
name: spec-decomposition
description: Task-breakdown method for tasks.md — sizing, dependency ordering, budget tiers, Files lists, Domain/agent routing, invariants touched and acceptance-criteria quality, in the task-block format the specs lint enforces
argument-hint: "[spec-name]"
---

# Spec Decomposition

How `/spec-create-single` and `/spec-create-epic` break a design into tasks. A task is what one agent completes in one session and one pull request; if "done" cannot be stated in one sentence, the task is too big. The task-block format is `knowledge/spec-authoring.md` § tasks.md, and `specs/*/dogfood-slice/tasks.md` is the reference instance.

## Input

`$ARGUMENTS`: optional spec name. Normally read inline by the creation skills with the design in context.

## Invocation contexts

- **Slash command**: with a spec name, reviews that spec's `tasks.md` against this method and lists the departures; it edits nothing.
- **Model-invoked**: the creation skills follow it to write `tasks.md`.
- **Non-interactive**: the same as model-invoked; it has no interactive steps.

## Sizing

A well-sized task:

- makes one logical change, testable on its own;
- touches roughly 2 to 8 files, all in one domain;
- can be implemented, tested and reviewed in one session.

Split when a task touches more than 8 files, spans two domains, joins unrelated changes with "and", needs several unrelated test files, or would add more than about 300 lines of non-test code. Do not over-split: a type plus its constructor plus its tests is one task, and so is an interface plus its first implementation. A single-file task that could fold into its neighbour in the same package is folded, unless it is genuinely atomic.

**Budget** (`- **Budget**:`), one of two tiers, optionally with a note:

| Tier | When |
|---|---|
| `standard` | Up to about 5 files, at most one new abstraction, up to about 6 acceptance items. |
| `complex` | 6 or more files, several new abstractions, or any of: process ownership or signals, persistence schema or migrations, concurrency, security primitives, cross-platform behaviour, or a design section that calls the task hard. |

Estimates run low, and the underestimate is worst on the hard classes, so a task in one of the `complex` classes above is `complex` even when it looks small, and a note says when even that ceiling may be exceeded.

## Ordering

Order tasks so each can trust that the tasks before it are complete and their interfaces stable. For the Go core:

1. module, types, interfaces and error values (the contracts);
2. storage and state: schema, journal, state machines;
3. domain logic: admission, workspace, supervisor, adapters;
4. the user surface: CLI commands and output, then the TUI;
5. end-to-end tests against the packaged binary, then documentation that describes the finished behaviour.

- **Contract before consumer.** Cross-domain work is split so the earlier task ships the contract (a type, an interface, a schema, a workflow input) and the later task, in the other domain, consumes it.
- **Documentation after the mechanism** it describes; otherwise the task that changes the mechanism re-checks those documents.
- **`Depends on`** lists every task whose output this task uses: `Task N`, comma-separated, or `None`. The first task may omit it. A parenthesised note may follow (`Task 2 (shares ci.yml; never in parallel)`). No cycles.
- **Parallelism.** Tasks with the same dependencies and disjoint Files may run in parallel. State which ones in the Dependencies section, and name any shared file that forbids it.

## Interfaces

A task whose output later tasks consume has a `Produces` field with exact Go signatures, error values and file formats: `journal.Open(ctx context.Context, dir string) (*Journal, error)`, `journal.ErrSchemaTooNew`. The consuming task builds against that text, so the implementing agent changes it only as a recorded spec deviation.

## Files, domain and agent

- **Files** lists every file the task creates or edits, including each test file an acceptance item names and each caller a signature change breaks (search for them).
- The Files resolve to one domain in `knowledge/domains.md`; **Domain/agent** names that domain's agent (`go-implementer`, `tui-implementer`, `release-engineer`, `agent-config-editor`), or `maintainer` for a human-run step. A mixed list is split into two tasks joined by a dependency.
- The task that completes the spec's own bookkeeping adds nothing to Files for `tasks.md` or `scratchpad.md`: every task updates both as part of completion.

Coverage pass: for every sentence in design.md that states what a user sees (page text, labels, placeholders, error wording), name the task whose Files list contains the file carrying that text. A user-visible claim with no owning task is a gap: assign it to a task's Files or record it as an explicit follow-up with its card or issue. The finalize review re-checks this mapping.

## Invariants touched

List the invariants (`knowledge/invariants.md`) and master-spec sections the task can affect, each with what the task does to keep it: `I09 (v2 §7.3: absent values stay "unknown")`. A task that reaches none says `None (<why>)`. A task that cannot keep an invariant is not written; the conflict goes to the honesty register and to a person.

## Acceptance

Every acceptance item is checkable by a named test, a command or an inspectable artifact, and names the observable that fails before the change and passes after it.

- Good: `TestOpenRefusesNewerSchema`: with `user_version=99`, `Open` returns `ErrSchemaTooNew` and the file's SHA-256 is unchanged.
- Good: `actionlint` and `zizmor` pass, and every `uses:` is pinned to a 40-character SHA.
- Bad: "the journal works"; "tests pass"; "the command exists".

Check each item against the defect shapes in `knowledge/spec-authoring.md` § Patterns that make criteria checkable: a missing failing counterfactual, a failure-only emitter, an unanchored text check, an empty universal, an open quantifier, element instead of effect, missing input shapes, uncovered error branches, self-confirming fixtures. Behaviour visible outside the process (CLI output, exit codes, files a user sees) also gets a packaged-binary end-to-end item, or a line saying why not.

## tasks.md header

The file starts with `## <Title> — Tasks` and a `### Dependencies` section holding:

- prerequisite specs (`specs/*/<name>/`) or "None outside this spec", and the parallel groups;
- **Gates for every task**: the formatter, then the domain's gate commands from `.claude/skills/quality-gates/SKILL.md`, and the reminder that files existing or an agent reporting success is not completion;
- **Completion convention**: how a finished task is marked (the heading suffix and the fields added);
- **Commits**: signed off; decisions on billing, persistence or process ownership carry their decision record.

Then `## Implementation Tasks` with the task blocks.

## Checklist

- [ ] Every task has Domain/agent, Budget, Depends on (or is first), Change, Files, Acceptance and Invariants touched; `scripts/ci/lint-agent-harness.sh --only specs` passes.
- [ ] Each task's Files stay in one domain, and 8 or fewer files unless the note says why.
- [ ] Ordered contract-first; no task depends on a later one; parallel groups named.
- [ ] Every consumed interface has an exact `Produces` signature.
- [ ] Every acceptance item names its test or command and its failing counterfactual.
- [ ] More than 12 tasks only with `design.md` stating why the spec was not split (`/spec-scope`).

## Output

Model-invoked: the task blocks for `tasks.md`. Slash command: the list of departures from this method, each with the task and the fix.
