---
paths:
  - ".claude/**"
  - "scripts/**"
---

# Strict by Default

New enforcement starts strict and loosens later, with evidence.

## Hooks and checks

- A new guard blocks (exit 2) on what it guards. When unsure, block.
- Advisory output exits 0 and uses `hookSpecificOutput.additionalContext`, and only when the advice cannot produce a wrong result. Never exit 1 to warn: nobody sees it.
- A guard that cannot parse its input, or whose tools are missing, blocks the calls it guards and allows the rest (see `.claude/hooks/README.md`). A crash is not a verdict.
- Every block teaches. Its stderr is the four-line stanza `BLOCK:` (the guard), `Command:` or `File:` (the offending input), `Detail:` (the violated rule), `Fix:` (the concrete next action). Never a bare "blocked".
- Advisory machinery (reviewers, classifiers, vendor consults, nags) never blocks a merge. Guards on shared state, the public repository and releases always do.

## Rule files

- State requirements as facts: "Do X", "Never Y". No "consider", "optionally", "where practical", "you may" or "it is recommended".
- Put exceptions in the rule as explicit carve-outs, never as a general softening.
- State current policy only: no amendment history, dates, ticket IDs or "previously this said". Evidence goes in `knowledge/rule-evidence/<rule>.md`.

## Loosening

Relax a guard or rule only with recorded evidence of false positives (at least three legitimate operations blocked, each linked in its evidence file), through a pull request that cites that evidence. Never loosen ad hoc.
