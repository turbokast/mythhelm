# Decision records

Significant decisions (see [GOVERNANCE.md](../../GOVERNANCE.md)) are recorded here as short numbered Markdown files: `NNNN-short-title.md`.

## Observatory contract

- [0020. Observatory lifecycle and themes](0020-observatory-lifecycle-and-themes.md): worker deadline, stop-less detach, typed client controls and declarative theme contract; accepted under the delegated maintainer decision recorded in Task 3.

## Supervisor attachment control

- [0021. Production singleton composition and scoped attachment control](0021-service-attachment-composition.md): ordinary production dispatch, root-wide migration, coherent scoped reads and durable stop acknowledgment; accepted under the exact-revision review and delegated maintainer decision recorded in Task 1.

## Template

```markdown
# NNNN. Title

- Status: proposed | accepted | superseded by NNNN
- Date: YYYY-MM-DD

## Context
What forces are at play, and what problem needs deciding.

## Decision
What we will do.

## Consequences
What becomes easier or harder, and the tests that pin the behaviour.
```
