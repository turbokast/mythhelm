---
name: add-rule
description: Create a new rule at .claude/rules/<name>.md, path-conditional by default, with its evidence file in knowledge/rule-evidence/
argument-hint: "<rule-name> [what it enforces]"
---

# Add Rule

Scaffold a rule that follows `.claude/rules/agent-config-conventions.md` and `.claude/rules/strict-by-default.md`: short, imperative, current policy only, and loaded only where it is needed.

## Input

`$ARGUMENTS`: the rule name in kebab-case, optionally followed by what it enforces.

## Invocation contexts

- **Slash command**: asks for any missing answer in step 3, then writes after confirmation (step 6).
- **Model-invoked**: the same, sourcing answers from the conversation first.
- **Non-interactive**: returns the unanswered step 3 questions if any required answer is missing; writes only with explicit pre-approval (`--confirm` in `$ARGUMENTS` or the dispatching prompt), otherwise returns the drafts and states that writes were withheld.

## Steps

1. **Check for a clash.** If `.claude/rules/<name>.md` exists, or an existing rule covers the same concern, stop and propose editing that rule instead.
2. **Read references.** `.claude/rules/go-conventions.md` (path-conditional) and `.claude/rules/coding-standards.md` (always-on), and `knowledge/rule-evidence/README.md`.
3. **Gather requirements** (input step): from `$ARGUMENTS` and the context first; ask only for what is missing.
   - What must agents do or never do? (required)
   - Which files, when read or edited, make the rule relevant? These become the `paths:` globs. A rule loads in every session only when it governs every session; say why.
   - What failure does it prevent? This becomes the evidence file.
   - Can a hook or CI check enforce it? If so, the rule points at that check, and the check is separate work with its own test.
4. **Draft the rule** at `.claude/rules/<name>.md`:

   ```markdown
   ---
   paths:
     - "<repository-relative glob>"
   ---

   # <Title>

   - <Do X.>
   - <Never Y.> <Carve-out, stated explicitly.>

   Evidence: `knowledge/rule-evidence/<name>.md`.
   ```

   No hedging words, no dates, no ticket IDs, no history. A glob for a path that does not exist yet goes under a `# future: <reason>` comment line.
5. **Draft the evidence file** at `knowledge/rule-evidence/<name>.md` from the template in `knowledge/rule-evidence/README.md`: hazard, mechanism, instances (usually "None recorded"), loosening criteria.
6. **Write** (write gate): show both drafts and write on confirmation.
7. **Verify.** Run `scripts/ci/lint-agent-harness.sh`: `frontmatter`, `paths-globs` and, for an always-on rule, `rule-budget` must pass.

## Output

The paths written (or the drafts) and the lint result, including the always-on byte total when the rule has no `paths:`.
