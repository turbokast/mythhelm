---
name: update-docs
description: Bring README, docs/, knowledge/, WORKFLOW.md and CLAUDE.md in line with what a shipped spec or a merged change actually did — find every statement the change made stale, fix it where readers look, and add only the new facts they need
argument-hint: "[<spec-name> | --range <base>..<head>]"
---

# Update Docs

Keeps the documentation true after a change ships. `/finalize-spec-publish` runs it for every spec; run it on its own after any change that altered commands, flags, files, formats or workflow.

## Input

`$ARGUMENTS`: a spec name (the range comes from `python3 scripts/harness/finalize.py range --spec <spec>`), or `--range <base>..<head>` for any change on `main`. With neither, ask for one; in a non-interactive run, stop and return that question.

## Invocation contexts

- **Slash command**: edits the documents in the working tree it runs in and lists what it changed. It commits only inside `/finalize-spec-publish`, which names the files.
- **Model-invoked**: the same.
- **Non-interactive**: the same edits, in the tree it was given; a missing range is an input step that stops with the question. It never commits in the shared checkout.

## Where each kind of fact lives

| Document | Update when the change | Never |
|---|---|---|
| `README.md` | changes what MYTHHELM can do, its status, install or first-run steps | describe internals |
| `docs/` (user and contributor docs) | changes a command, flag, exit code, file format, config key or behaviour they describe | edit `docs/spec/master-spec.md`: the design reference changes only through its own reviewed change |
| `knowledge/` | changes a fact a page states: a path, a table row, a mechanism, a failure mode | add instructions; they belong in rules (`.claude/rules/knowledge-conventions.md`) |
| `WORKFLOW.md` | changes how maintainers drive the agents | restate skill procedures |
| `CLAUDE.md` | changes a convention or command every session needs | add anything that fits in a rule, a skill or `knowledge/`: its bytes count against the always-on budget |

## Steps

1. **List what changed**: `git diff --name-only <base>..<head>` and `git diff <base>..<head>` for the files that matter. Collect the names a reader would search for: commands, flags, exit codes, file and directory names, config keys, skill and script names, table headings.
2. **Find every stale statement**: `git grep -n -e '<name>' -- README.md docs knowledge WORKFLOW.md CLAUDE.md AGENTS.md` for each collected name, plus the pages that describe the changed area (`knowledge/README.md` indexes them). Read each hit against the code at `<head>`. Also find the absences: a new command, flag or file that the page listing its siblings does not mention. Enumerate that page's list, never assume from one missed keyword.
3. **Edit** the stale statements to match the code, in the page's existing style and table shapes. Add a new fact only where its siblings already live. Remove text about what the change deleted.
4. **Check** what you touched:

   ```bash
   scripts/ci/lint-agent-harness.sh --only references,rule-budget,abs-paths
   scripts/ci/check-public-hygiene.sh
   ```

   A `rule-budget` failure means `CLAUDE.md` grew: move the new text into a rule with `paths:`, a skill or `knowledge/` instead.

## Output

One line per changed document: `<path>: <what changed>`, or `Docs: no update needed`, followed by the names checked in step 2 so the claim can be audited.
