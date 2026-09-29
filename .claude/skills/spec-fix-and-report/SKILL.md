---
name: spec-fix-and-report
description: Process a spec validation report — apply the fixes that need no judgment, escalate the rest, sweep the spec for stale copies of every changed claim, re-validate, and move a Ready spec to specs/todo/
argument-hint: "<spec-name>[,<spec-name>...]"
---

# Spec Fix and Report

Phase 5 of `/spec`. Takes the `/spec-validate` report, fixes what can be fixed without a person's judgment, and escalates the rest. A fix applied only at the cited line leaves every other statement of the same claim standing, and that residue becomes the next round's findings, so every fix batch ends with a sweep of the whole spec directory.

## Input

`$ARGUMENTS`: the spec name(s) just validated, with the validation report in context.

## Invocation contexts

- **Slash command**: applies the fixes, re-validates, and asks the person the escalated questions.
- **Model-invoked**: the same.
- **Non-interactive**: applies the fixes and re-validates. Escalated findings are not asked: the run stops with the spec left in `unrefined/` or `refined/`, and returns the questions.

## Steps

1. **Classify** each blocking and advisory finding.
   - **Auto-fix**: a wrong path or citation (correct it from the code), a missing `Produces` signature, a vague acceptance item (make it name a test or command and its failing state), an oversized or mixed-domain task (split it), a missing Files entry, a missing tag, non-goal binding, decisions row or honesty-register row whose content the investigation already established, a missing section or cross-reference.
   - **Escalate** (escalation step): an ambiguous requirement, a change of scope or intent, a conflict with an in-progress spec, a disagreement with the master spec, an invariant the spec cannot keep, a gap in an epic's coverage.
2. **Apply** the auto-fixes to the spec files.
3. **Sweep** the spec directory, for every claim the batch changed:
   - search each spelling of the claim (the symbol, the count, the citation, the wording) across every file in the directory with `grep -rn '<spelling>' specs/<state>/<name>/`, and make every hit agree with the fix: `tasks.md` is the file the implementer executes, so agreement between `requirements.md` and `design.md` alone is not enough;
   - delete superseded text rather than leaving it beside its replacement; an implementer reading top to bottom follows the last instruction it sees;
   - re-derive every changed count at the source, and write the command and its result beside the count.
4. **Re-check.** Run `scripts/ci/lint-agent-harness.sh --only specs`, then re-run `/spec-validate` on the changed specs. Stop after three validation rounds in all; if blocking findings remain, report them instead of a fourth round.
5. **Escalate** the remaining findings (escalation step): ask in an interactive session and loop to step 2 with the answers; in a non-interactive run, stop and return them.
6. **Move** each spec whose verdict is Ready: `scripts/harness/spec-lifecycle.sh move <name> todo`. An epic's `plan.md` stays where it is.

## Output

Single spec:

```markdown
## Spec: <name>
Location: specs/<state>/<name>/
Validation: <rounds> round(s); lint (specs) <exit code>; verdict <Ready for implementation | Needs human input>

### Fixed
- <finding> — <what changed>

### Sweep
- `grep -rn '<spelling you searched>' specs/<state>/<name>/` — <N> hits; <file:line> <agrees | rewritten | deleted>
- <count claim>: `<command>` — <observed value>

### Needs human input
- <finding> — <the decision needed>

Next: <the implementation run for <name> | answer the questions above, then /spec <name>>
```

An epic reports the plan's location, a table of sub-specs (name, location, task count, verdict), the cross-spec results (dependency graph, cross-references, coverage, overlaps), the fixes and sweep per sub-spec, the questions, the total task count and the recommended order.
