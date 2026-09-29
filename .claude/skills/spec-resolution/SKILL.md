---
name: spec-resolution
description: Reference for finding a spec by name across the lifecycle directories, asserting that exactly one copy exists, and moving it between states; the lifecycle skills read it inline, and invoking it resolves one spec
argument-hint: "<spec-name>"
---

# Spec Resolution

How every lifecycle skill turns a spec name into a directory, and how it moves that directory. The mechanism is `scripts/harness/spec-lifecycle.sh`; this page says when to use which command and what each refusal means. The states and the transition table are in `specs/README.md`.

## Input

`$ARGUMENTS`: a spec name in kebab-case. Optional when another skill reads this page as a reference.

## Invocation contexts

- **Slash command**: resolves the named spec and reports its path, state, kind and missing required files.
- **Model-invoked**: the same; other skills also read the sections below inline and run the commands themselves.
- **Non-interactive**: the same. Read-only; a refusal (step 2) is returned as the result.

## Resolving

```bash
scripts/harness/spec-lifecycle.sh resolve <name>
```

- It prints `specs/<state>/<name>`. States are searched in the order `in-progress`, `unfinalized`, `todo`, `refined`, `unrefined`, `done`, `archived`: active work first, the record last.
- Before printing, it asserts that exactly one copy exists across the working tree **and** the git index. A copy left in the index by a plain `mv` comes back on the next checkout; while it exists, every lookup refuses (next bullet) instead of choosing one.
- Exit 1 with a `BLOCK:` stanza when the spec is missing, has two or more copies, or exists only in the index. Two copies are an **escalation**: stop and report both paths. Never pick one by guessing, and never delete one to make the check pass without knowing which copy is live.
- A directory holding `plan.md` and no `requirements.md` is an epic plan; any other is a single spec.

## Refusal contracts

Each lifecycle skill refuses a spec in the wrong state, and names the skill that owns the next step:

| Skill | Refuses when the spec is in | Message names |
|---|---|---|
| `/spec <name>` | `unrefined/` (a spec it is drafting in the same run is exempt) | `/refine-spec <name>` |
| `/spec <name>` | `todo/` or later | `/evaluate-spec <name>` |
| `/refine-spec <name>` | `todo/` or later | `/evaluate-spec <name>` |
| implementation of `<name>` | `unrefined/` or `refined/` | `/refine-spec` or `/spec` |

## Moving

```bash
scripts/harness/spec-lifecycle.sh move <name> <to-state>
```

- It resolves the spec first (so duplicates block the move), checks the transition against the table in `specs/README.md`, refuses an existing destination, and renames with `git mv` so the index follows. An untracked spec is renamed with plain `mv`.
- It stages the rename of tracked files and nothing else, and commits nothing. Files that were untracked before the move (a new `scratchpad.md`, or a whole spec that was never added) are still untracked at the new path.
- The change that completes the lifecycle step stages the new directory, then commits the move with the spec's edits, naming its paths. For a spec that was tracked before the move, name both paths so the commit records the removal of the old one:

  ```bash
  git add -- specs/<new-state>/<name>
  git commit -s -m "<msg>" -- specs/<old-state>/<name> specs/<new-state>/<name>
  ```

  For a spec that was never tracked, git knows no old path, so name only the new one: `git commit -s -m "<msg>" -- specs/<new-state>/<name>`.
- Never move a spec directory with a bare `mv`, and never copy it.

## Citing a spec

A spec and every document that refers to it cite it as `specs/*/<name>/`. The glob stays correct at every state, and the single-copy assertion guarantees it names one directory.

## Steps

1. **Resolve**: run the resolve command for `$ARGUMENTS`.
2. **Report the refusal** when it exits 1 (escalation step): return the stanza unchanged.
3. **Describe**: state, kind (single or epic), and the files present, checked against the required files for that state in `specs/README.md`.

## Output

`specs/<state>/<name>`, the state, the kind and any missing required file; or the refusal stanza.
