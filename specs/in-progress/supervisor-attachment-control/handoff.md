# supervisor-attachment-control — Hand-off

> One section per task. Each task replaces its own `<!-- pending -->` line in its pull request with what
> dependent tasks need: what it produced (the API and files as shipped), what a later task must
> know, and any deviation that changes a later task's inputs. `/run-spec` puts the sections of a
> task's dependencies into that task's dispatch prompt. Open questions and research stay in
> `scratchpad.md`.

## Task 1 — Review the service composition decision

- **Produces**: Accepted ADR 0021 and its decision-index entry, recorded in PR #501. The source proposal is revision `e50faa8`, original-file SHA-256 `b30067e53dadaddbc416f8501ed67ea127ff8b02f7f6591be0b86fd49ca53393`; its Decision section is unchanged.
- **Review**: Independent architecture/code review returned Ready on that exact proposal, and coordinator review of author output `5386aff` returned Ready with no blocking findings. The coordinator separately accepted under the operator's explicit day-long shipping delegation; no personal operator content review, product signature or formal grant is claimed.
- **For dependents**: Preserve ordinary production dispatch, ADR 0014 singleton and ADR 0016 root-wide migration, immutable launch generation, schema2 adoption, authority-before-replay and immutable acknowledgment/separate reconciled result. Enrollment-only and concurrent legacy/service variants remain rejected.
- **Evidence boundary**: ADR acceptance supplies a reviewed design, not runtime/native/billing/host/platform qualification. Keep all task-specific tests and cross-spec lifecycle dependencies; no public attach-ready claim follows from this docs task.
- **Deviations**: Required task records follow WORKFLOW's pre-merge completion-entry convention; the task PR must merge before dependent implementation consumes its acceptance.

## Task 2 — Define typed service and attachment protocols

<!-- pending -->

## Task 3 — Persist authority, revision and durable operation state

<!-- pending -->

## Task 4 — Extract lock-held lifecycle entrypoints

<!-- pending -->

## Task 5 — Close root migration and legacy writer fences

<!-- pending -->

## Task 6 — Compose the production singleton runtime

<!-- pending -->

## Task 7 — Adopt the same worker under a new service generation

<!-- pending -->

## Task 8 — Prepare service-side admission and confirmations

<!-- pending -->

## Task 9 — Commit and execute durable ordinary dispatch

<!-- pending -->

## Task 10 — Serve typed operation status and bounded observation

<!-- pending -->

## Task 11 — Mint and revoke internal scoped attachment grants

<!-- pending -->

## Task 12 — Produce coherent safe projections

<!-- pending -->

## Task 13 — Accept atomic stop intents with immutable acknowledgments

<!-- pending -->

## Task 14 — Deliver stop outbox idempotently

<!-- pending -->

## Task 15 — Reconcile stop results and bound core responsiveness

<!-- pending -->

## Task 16 — Route public run through service admission

<!-- pending -->

## Task 17 — Route demo and accessible clients through service

<!-- pending -->

## Task 18 — Route trusted mutation clients to one actor

<!-- pending -->

## Task 19 — Wire launch TUI as a service client

<!-- pending -->

## Task 20 — Prove ordinary production composition end to end

<!-- pending -->

## Task 21 — Document production ownership and migration

<!-- pending -->
