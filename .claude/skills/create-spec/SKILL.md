---
name: create-spec
description: Turn a GitHub issue or a free-text idea into a grounded, unrefined requirements.md in specs/unrefined/<name>/, ready for /refine-spec
argument-hint: "<issue-number | issue-url | \"feature description\">"
---

# Create Spec

Captures an idea as the first document of a spec: `specs/unrefined/<name>/requirements.md`. It front-loads the context that `/refine-spec` and `/spec` would otherwise reconstruct, and it grounds every claim the source makes about the code before anything is built on it.

## Input

`$ARGUMENTS`, one of:

- a GitHub issue of this repository: `123`, `#123` or its URL;
- a free-text description of the change.

## Invocation contexts

- **Slash command**: runs every step and writes the requirements file; stops at step 4 when a claim is refuted.
- **Model-invoked**: the same.
- **Non-interactive**: the same. A missing input (step 1) or a refuted claim (step 4) ends the run and returns the questions or the refutation as the result; nothing is written.

## Steps

1. **Read the source** (input step). For an issue, read it with `gh issue view <n> --json number,title,body,labels,comments,url`. It is read-only; this skill never comments on, labels or edits an issue. For a description, use the text. If `$ARGUMENTS` is empty, ask for an issue or a description.
2. **Name the spec.** Derive a short kebab-case name from the subject, not from the issue number: "Doctor reports state directory usage" becomes `doctor-state-report`.
3. **Check for an existing spec.** Search every state: `ls -d specs/*/<name>` and `grep -rlE "issues/<n>([^0-9]|$)" specs/`. If a spec already covers this issue or subject, report its path and state and stop; `/evaluate-spec` updates an existing spec. If only the name clashes with an unrelated spec, append `-2`.
4. **Ground the premise** (`.claude/rules/spec-premise-grounding.md`; escalation step). List the load-bearing claims in the source: the mechanism it describes, each file or behaviour it cites, whether the failure is reachable today, and whether an acceptance check could be made to fail today. Open the source for each and record one verdict: `HOLDS`, `PARTIAL` (write down the drift), `REFUTED` or `UNVERIFIABLE`.
   - Any `REFUTED`: stop. Write nothing. Report the claim, the source that refutes it (`file:line`), and a corrected statement the author can put in the issue.
   - `UNVERIFIABLE`: carry it into Open Questions.
   - Failure not reachable: the spec either binds its criteria to a path that can fail today or sequences behind the blocker; say which under Dependencies.
5. **Gather engineering context.**
   - `knowledge/invariants.md` and the master-spec sections the change touches (`docs/spec/master-spec.md`).
   - `knowledge/domains.md`: the domains and agents the change will involve.
   - Decision records under `docs/decisions/` in the same area.
   - Specs in `specs/in-progress/` and `specs/todo/` that touch the same packages (possible conflicts), and in `specs/done/` that this extends or supersedes.
   - Name the packages and files involved; leave exact signatures to `/spec`'s investigation.
6. **Write** `specs/unrefined/<name>/requirements.md`, following `knowledge/spec-authoring.md` § requirements.md, with these additions for an unrefined spec:
   - a status line under the title: `> Unrefined. Run /refine-spec <name> before /spec.`;
   - `## Context`: the issue link (or "from a description"), the problem in two to four paragraphs, and the grounding verdicts from step 4, one line per claim;
   - rough `FR-N` / `AC-N.M` criteria, already tagged with invariant, gate and section IDs where known;
   - `## Open Questions` (each with what it blocks and the options), `## Dependencies` (prerequisite, conflicting and superseded specs) and `## Impacted components` (packages and files, cited).
7. **Report.**

## Output

```text
Created: specs/unrefined/<name>/requirements.md
Source: <issue URL | description>
Grounding: <N> claims, <n> HOLDS, <n> PARTIAL, <n> UNVERIFIABLE
Requirements: <N> FRs, <N> acceptance criteria; open questions: <N>
Next: /refine-spec <name>
```

Or, when stopped at step 4: the refuted claims with their source evidence and the proposed correction, and no file written.
