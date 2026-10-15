# windows-process-ownership — Hand-off

> One section per task. Each task replaces its own `<!-- pending -->` line in its pull request with what
> dependent tasks need: what it produced (the API and files as shipped), what a later task must
> know, and any deviation that changes a later task's inputs. `/run-spec` puts the sections of a
> task's dependencies into that task's dispatch prompt. Open questions and research stay in
> `scratchpad.md`.

## Task 1 — decision record: Windows process ownership

- **Produces**: `docs/decisions/0017-windows-process-ownership.md`, recording the maintainer-approved OQ-1/OQ-8/OQ-11 defaults and D2/D3/D8/D11. It supersedes ADR 0004's named Windows design statements; Task 10 updates the old ADR's status.
- **For dependents**: worker-owned kill-on-close job; Windows-only native creation time in worker.json and event payload; no database migration; worker death does not itself resolve ownership. Preserve unresolved crash-boundary outcomes and the separate Windows stop-ladder version.
- **For Task 3**: real Windows runner measurements and OQ-3/OQ-13 architect review remain required. A kill-on-close parent job on either runner requires maintainer escalation before production changes.
- **Qualification**: no Windows support row is promoted by this ADR. Task 11 remains a maintainer decision supported by real Windows CI evidence.
- **Deviations**: none to the approved design or dependent acceptance criteria.

## Task 2 — fake adapter child-tree fixture for Windows

<!-- pending -->

## Task 3 — Job Object launch ownership

<!-- pending -->

## Task 4 — job-wide stop, confirmation and descendant scan

<!-- pending -->

## Task 5 — native start time in the launch record

<!-- pending -->

## Task 6 — Windows recovery verdict

<!-- pending -->

## Task 7 — detach and interrupt behaviour on Windows

<!-- pending -->

## Task 8 — Windows process-support matrix (leaf package)

<!-- pending -->

## Task 9 — adapter capability derivation

<!-- pending -->

## Task 10 — Windows documentation

<!-- pending -->

## Task 11 — promote tested Windows rows

<!-- pending -->
