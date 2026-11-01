# Decision records

Significant decisions (see [GOVERNANCE.md](../../GOVERNANCE.md)) are recorded here as short numbered Markdown files: `NNNN-short-title.md`.

## Observatory contract

- [0020. Observatory lifecycle and themes](0020-observatory-lifecycle-and-themes.md): worker deadline, stop-less detach, typed client controls and declarative theme contract; accepted under the delegated maintainer decision recorded in Task 3.

## Supervisor attachment control

- [0021. Production singleton composition and scoped attachment control](0021-service-attachment-composition.md): ordinary production dispatch, root-wide migration, coherent scoped reads and durable stop acknowledgment; accepted under the exact-revision review and delegated maintainer decision recorded in Task 1.

- [0022. Durable service admission binding and bounded observer delivery](0022-service-admission-binding.md): exact design `d8975b35b342bc7f5be34c43bcdbcc39e64ecf52` accepted after independent review and maintainer adoption, with follow-on spec PR #543 merged at `d35073de4e496ac264f17c2833aff094d88a7c81`; runtime Tasks 6–9 retain their independent implementation and Task 4 gates.

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
