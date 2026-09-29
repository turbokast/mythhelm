---
name: refine-spec
description: Iteratively assess and fix an unrefined spec's requirements.md with fresh-context reviews until /spec can design from it without guessing, then move it to specs/refined/
argument-hint: "<spec-name>"
---

# Refine Spec

Takes rough requirements (written by `/create-spec` or by a person) through an assess → fix → re-assess loop, at most three rounds. Each assessment is a fresh-context `code-reviewer` dispatch, so no round is anchored on the previous one's fixes. It edits `requirements.md` only: design and tasks belong to `/spec`.

## Input

`$ARGUMENTS`: the spec name.

## Invocation contexts

- **Slash command**: runs the loop; asks the person the human-required questions (step 4) and continues with their answers.
- **Model-invoked**: the same.
- **Non-interactive**: runs the loop and applies auto-fixes. Human-required issues are an escalation: the run stops, leaves the spec where it is, and returns the questions. An assessor that did not return also stops the run.

## Steps

1. **Resolve** with `scripts/harness/spec-lifecycle.sh resolve <name>` (`.claude/skills/spec-resolution/SKILL.md`). Continue only for `unrefined/` (first refinement) or `refined/` (re-refinement); for any later state, stop and point to `/evaluate-spec <name>`.
2. **Read** `requirements.md` (and `design.md` or `tasks.md` if a person already started them), `knowledge/invariants.md`, `knowledge/domains.md`, and the master-spec sections the requirements cite.
3. **Assess** (fresh context). Dispatch one assessor per round, with only the spec path and this prompt, never your own reasoning or earlier rounds:

   ```text
   Agent(subagent_type: "code-reviewer", description: "Assess spec <name>", prompt: <the prompt below>)
   ```

   > Assess `specs/<state>/<name>/requirements.md` for readiness to design from. Read `knowledge/spec-authoring.md`, `.claude/rules/spec-authoring.md`, `.claude/rules/spec-premise-grounding.md`, `knowledge/invariants.md` and `knowledge/domains.md` first. Score each dimension PASS, NEEDS_WORK or FAIL, citing the line of the spec each issue is on:
   > D1 objectives are specific and observable; D2 every FR has numbered acceptance criteria in EARS form with concrete values, NFRs are measurable, and the Definition of Done covers every FR; D3 every claim about the code is cited and true (open two or three citations and check them; an absence claim needs structural grounding); D4 scope is bounded, non-goals say what still binds, and the work fits one spec or is flagged as an epic; D5 the requirements respect the invariants and domain boundaries they reach, and are tagged with them; D6 no requirement contradicts another, a non-goal or the Definition of Done; D7 dependencies on and conflicts with other specs under `specs/` are named; D8 a designer could write `design.md` and `tasks.md` from it without guessing.
   > Output: `## Assessment: <name>`, `Verdict: Ready | Needs work | Needs human input`, a table `| Dimension | Score | Issues |`, then `### Auto-fixable` (numbered: issue, location, the fix), `### Human-required` (numbered: the question and why only a person can answer it), `### Missing context`. Zero issues is a valid result; never invent findings. No style-only findings.

   Record the dispatch before reading the result: every round adds one to `dispatched`. A round counts as `returned` only when its report has a verdict and a PASS, NEEDS_WORK or FAIL score for each of D1 to D8. A report that is empty, or missing the verdict or any dimension's score, counts as `failed`: it is never read as "no issues" or as "no FAIL".
4. **Fix.**
   - Auto-fixable issues: edit `requirements.md` to make criteria testable, correct stale citations, add missing tags, non-goal bindings, dependencies and Definition-of-Done items. Never change the intent or scope; a fix that would is human-required.
   - Human-required issues (input step): ask, and fold the answers in.
5. **Re-assess** with a new assessor (step 3) after each round of fixes. Only `returned` rounds count toward the exits below; a `failed` round stops the loop as **Blocked**. Stop when:
   - **Ready**: every dimension PASS;
   - **Good enough**: after three rounds, no FAIL remains (the remaining NEEDS_WORK items go into the log for `/spec`);
   - **Blocked**: human-required issues remain unanswered;
   - **Not converging**: after three rounds, a FAIL remains or new issues keep appearing; the requirements need rewriting, not refining.
6. **Finish.**
   - Ready or good enough, and every dispatched round returned (`failed=0`): when the spec is in `unrefined/`, run `scripts/harness/spec-lifecycle.sh move <name> refined`. Write `specs/refined/<name>/refinement-log.md` with the dispatch line, each round's verdict and fixes, and the remaining notes.
   - Blocked (including any failed round) or not converging: leave the spec where it is and report.

## Output

```text
dispatched=<N> returned=<M> failed=<K>
Verdict: <Ready | Good enough | Blocked | Not converging> after <N> rounds
Changes: <one line per change to requirements.md>
Remaining notes for /spec: <items or "none">
Location: specs/<state>/<name>/
Next: /spec <name>   (or the questions that block it)
```

When `returned + failed < dispatched`, name the round whose assessor is unaccounted for, and report no verdict.
