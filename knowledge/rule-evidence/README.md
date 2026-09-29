# Rule evidence

Why each rule in `.claude/rules/` exists. A rule carries only the instruction and a one-line pointer here, because rules load into agent context and every byte is paid on every turn. The evidence is read only by someone auditing a rule, proposing to loosen it or checking that it still fits.

One file per rule, named after the rule file. Nothing here is an instruction: every "do", "never" and "always" stays in the rule.

## What an evidence file holds

- **Hazard**: the failure the rule prevents, described generically. What goes wrong, how it hides, and why the obvious check does not catch it.
- **Mechanism**: why the rule's instruction prevents it, and which hook, CI check or gate gives it teeth, if any.
- **Instances**: observed occurrences in this repository, each with a link to the issue, pull request or commit. This list starts empty and grows.
- **Loosening criteria**: the evidence that would justify relaxing the rule.

Public by default: no private incidents, people, hostnames or dates of rulings. An instance links to something public in this repository or it is not recorded.

## Template

```markdown
# Evidence: <rule-name>

Record for `.claude/rules/<rule-name>.md`.

## Hazard

<What goes wrong and why it is easy to miss.>

## Mechanism

<Why the rule prevents it. Name the enforcing check, or say that it is prose only.>

## Instances

None recorded in this repository yet.

## Loosening criteria

<What evidence would justify relaxing it, for example three recorded false positives.>
```
