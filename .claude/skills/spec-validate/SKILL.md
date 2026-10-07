---
name: spec-validate
description: Dispatch a fresh-context code-reviewer to validate spec files against the codebase, the master spec, the invariants and the authoring rules, and return a structured verdict
argument-hint: "<spec-name>[,<spec-name>...]"
---

# Spec Validate

Phase 4 of `/spec`. The author of a spec cannot review it: they read what they meant, not what they wrote. So validation is a dispatched `code-reviewer` that sees only the spec paths and this checklist, never the investigation, the scope reasoning or the design justifications.

## Input

`$ARGUMENTS`: one spec name, or for an epic, the plan's name and every sub-spec name, comma-separated.

## Invocation contexts

- **Slash command**: dispatches the validator and prints its report.
- **Model-invoked**: the same, normally from `/spec`.
- **Non-interactive**: the same. It edits nothing; fixing is `/spec-fix-and-report`'s job.

## Steps

1. **Resolve** each name with `scripts/harness/spec-lifecycle.sh resolve <name>` (`.claude/skills/spec-resolution/SKILL.md`).
2. **Run the mechanical gate** yourself: `scripts/ci/lint-agent-harness.sh --only specs`. Keep its exit code and findings for the report; the validator judges what the lint cannot.
3. **Dry-match embedded test selectors.** Collect every `go test … -run <selector>` command embedded in the spec's task text, in both `-run <selector>` and `-run=<selector>` forms. For each, resolve the selector statically — never by running it — against the base tree plus the test names the task's own text declares it will introduce, and fail validation when it matches zero tests, naming the task and the selector. Match the top-level element by parsing the package's test files for `func Test<Name>`, `func Fuzz<Name>` and `func Example<Name>` declarations — `-run` selects fuzz tests and examples too (never `go test -list`: listing starts the test binary, running package init and `TestMain`); verify each subtest suffix against the `t.Run` names nested beneath the matched parent, not anywhere in the package. Follow `-run` semantics for slash-separated selectors (e.g. `TestDecodeFixture/recorded`). Apply the embedded command's package selection and build constraints (GOOS/GOARCH, `-tags`, constrained filenames) before counting declarations; fail closed when that context cannot be resolved. Fail closed: unparseable command extraction, a match error, a match timeout, a suffix matching no name beneath its parent, or a dynamically named parent whose suffix cannot be checked statically blocks validation exactly like a zero-match selector. A failed dry-match fails validation: report it as a blocking finding and stop before dispatch.
4. **Dispatch** one validator for a single spec, or one for the whole epic (so it can check cross-spec consistency):

   ```text
   Agent(subagent_type: "code-reviewer", description: "Validate spec <name>", prompt: <the prompt below, with the paths filled in>)
   ```

   Record `dispatched=1` before reading the result; a validator that returns nothing is `failed=1`, and no verdict is reported.
5. **Optional second reviewer** (advisory): when a vendor is available, run `/vendor-consult` with the `spec-validate` stage on the spec files (`knowledge/vendors.md`). Open each of its findings at its anchor; a confirmed one joins the report tagged `[vendor]`, a rejected one is dropped with its reason. It never changes the verdict on its own and never gates; an `unavailable` result is recorded and skipped.

## Validator prompt

Pass this with `<SPEC_PATHS>` filled in, and nothing else:

````markdown
You are validating spec(s) before implementation: <SPEC_PATHS>. You have no context from their authoring; judge only what is written and what the repository contains. Edit nothing.

Read first: `knowledge/spec-authoring.md`, `.claude/rules/spec-authoring.md`, `.claude/rules/spec-premise-grounding.md`, `knowledge/invariants.md`, `knowledge/domains.md`, and the specification index (`docs/spec/README.md`) and every section of the named master revision the spec cites. New product specs use v2; historical section numbers and audit IDs stay pinned to their original revision. Then read every file in each spec directory.

For each spec, check:

1. **References.** Every path in design.md and tasks.md exists, or is a new file in an existing directory created by a task. Every cited `file:line`, function and signature matches the code: open each one. Every count or absence claim is grounded structurally, not by a keyword search.
2. **Requirements.** IDs are numbered (O, N, FR, AC-N.M) with no gaps or reuse. Every acceptance criterion is in EARS form with concrete values and tagged with invariant, gate or section IDs that exist in the master spec. Every non-goal says what still binds. The Definition of Done covers every FR.
3. **Design.** The current state is cited. Every interface a task consumes has an exact signature, failure cases included. Every choice with an alternative is in the decisions table. The honesty register lists every master-spec demand the spec cites or reaches but does not fully meet; an unlisted gap is a finding. The design follows the conventions for its domain (`.claude/rules/go-conventions.md` for Go, `.claude/rules/github-workflows.md` for workflows, `.claude/rules/harness-scripts.md` for scripts).
4. **Tasks.** Each task is one logical change in one domain with the right agent; 8 files or fewer unless justified; ordered contract-first with no dependency on a later task; `Produces` exact where consumed; Files complete against its acceptance items; Budget plausible (`.claude/skills/spec-decomposition/SKILL.md`). Each acceptance item names its test or command and a state in which it fails; flag every defect shape in `knowledge/spec-authoring.md` § Patterns that make criteria checkable. Invariants touched are correct and complete for the task's code.
5. **Conflicts.** No spec in `specs/in-progress/` or `specs/todo/` edits the same files without a note on ordering. No requirement contradicts another, a non-goal or the design.
6. **Epic only.** The plan's Work Streams name exactly the sub-specs given; the dependency graph between specs is acyclic; cross-references are bidirectional; the sub-specs cover the plan's scope with no gap and no unexplained file overlap.

Rules for findings: anchor every finding to a check number above and a location (`file:line`, FR or task ID). Zero findings is a valid result; never invent one. No style-only findings. Mark each finding **blocking** (the spec cannot be implemented correctly as written) or **advisory**.

Output, per spec:

### Spec review: <name>
Verdict: Ready for implementation | Needs revision
| Check | Result | Findings |
|---|---|---|
| 1 References | PASS/FAIL | … |
| 2 Requirements | PASS/FAIL | … |
| 3 Design | PASS/FAIL | … |
| 4 Tasks | PASS/FAIL | … |
| 5 Conflicts | PASS/FAIL | … |
| 6 Epic | PASS/FAIL/n/a | … |
#### Findings
1. [blocking|advisory] <check> <location>: <what is wrong> — <the concrete fix, or the question only a person can answer>
````

## Output

```text
dispatched=1 returned=<0|1> failed=<0|1>
Lint (specs): exit <code>; <findings or "clean">
Vendor (spec-validate): <completed, N confirmed / M rejected | unavailable (<reason>) | not run>
<the validator's report, verbatim, plus any confirmed [vendor] findings>
```
