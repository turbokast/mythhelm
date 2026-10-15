# 0017. Windows process-tree ownership

- Status: accepted (implementation and Windows qualification pending)
- Date: 2026-10-15
- Authority: the maintainer explicitly approved the Windows ownership proposal's defaults for Task 1; this record transcribes that decision.
- Supersedes: [ADR 0004](0004-slice-process-model.md), Decision sections **Worker process**, **Native process**, **Stop ladder and confirmation** and **Capability record**, for their Windows process-model, stop-ladder and capability statements only. Its historical implementation evidence remains historical; Unix behavior is unchanged.

## Context

[Master Specification v2](../../mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md) §6.4 requires owned process trees, confirmed stop evidence and reconciliation before retry. I06 keeps an unconfirmed stop visible; I18 assigns one process owner per attempt; I24 requires external-resource ownership to be reconciled before reassignment and rejects a lease timeout as proof a writer stopped.

The approved `specs/*/windows-process-ownership/` design extends the Windows single-process stop into Job Object ownership. Task 1 resolves its maintainer decisions before Tasks 3–7 implement them. Acceptance of this ADR does not implement that design or establish a supported Windows route.

## Decision

- **OQ-1 / D1 — worker-owned kill-on-close job.** The worker will own a non-inheritable Job Object with `KILL_ON_JOB_CLOSE`, without breakaway permission. Closing the last job handle ends its assigned process tree. Native survival and reconnection after a worker crash are not promised on Windows. A surviving job or separate holder is outside this slice.
- **OQ-8 / D4 — Windows-only native creation time.** Record `native_start_time` alongside the native PID on Windows, using creation-time evidence to distinguish reused PIDs. Preserve Unix identity and persistence behavior.
- **OQ-11 — no automatic ownership resolution after worker death.** Kill-on-close plus confirmed worker death is not, by itself, sufficient I24 evidence that every writer stopped. Recovery will retain `ownership_unresolved` and quarantine the attempt. Matching a live native requires both PID and creation time; an unmatched or unreadable identity is not proof of death. Recovery neither signals a named orphan nor starts a replacement native on that evidence.
- **D2 — suspended launch, identity, assignment, resume.** Persist launch intent before creating the job or native. Start the native suspended, record its PID and creation time, assign it to the job, then resume it. Fail closed on errors; a failed launch with unconfirmed cleanup remains `stop_unconfirmed`. This ordering retains the design's residual suspended-process windows: before identity persistence an inert native may be unnamed, and before assignment a recorded inert native may survive outside the job. Neither window permits recovery to infer resolved ownership. The OQ-3/OQ-13 architect review and launch-boundary tests remain required.
- **D3 — a distinct Windows stop-ladder version.** Use `stop-ladder/v2-windows` for the job-wide kill rung; Unix retains `stop-ladder/v1`. Graceful Windows console rungs require separate passing evidence. Stop confirmation requires the native to have been waited for and the job membership query to confirm emptiness; errors and survivors remain unconfirmed.
- **D8 — no database migration.** Keep `native_start_time` in `worker.json` and the `attempt.launched` event payload. Do not add an `attempts` column. Recovery reads the identity file; a missing field remains unknown.
- **D11 — refuse launch inside a kill-on-close parent job.** Check the worker's parent-job condition at native launch, after intent persistence and before creating a job or native. Refuse on a kill-on-close parent job or a failed query; do not silently treat a query error as absence. Do not enable worker breakaway in this slice. Task 3 first measures the real condition on both `windows-latest` and `windows-11-arm` and records run IDs and observed flags. If either runner reports a kill-on-close parent job, stop before production changes and return a concrete revised design to the maintainer. Stubbing away the condition in out-of-process lifecycle tests or silently accepting universal refusal is not a resolution.

These choices match the approved design defaults. No task acceptance criterion changes. Tasks 3–7 remain gated by the task dependency graph; Task 2 is independent of this decision.

## Consequences and evidence still required

The intended Windows model has a single worker owner and tree-wide termination, but ownership after a crash remains unresolved. It does not promise Unix-equivalent worker-crash survival, graceful console interruption, or ownership of containers and ports. Native PID and creation time do not establish descendant nonce evidence. Existing Windows implementation and support claims remain unchanged until the relevant tasks land.

Tasks 3–7 must provide real Windows launch, stop, crash-boundary, recovery and detach evidence. Cross-compilation does not qualify behavior. The planned support matrix starts without supported rows; Task 11 remains a maintainer decision requiring passing Windows CI evidence for each architecture and surface. A runner unable to exercise console events leaves worker detachment unknown. Claude Code on Windows remains unsupported pending separate adapter qualification.

Task 10 will update ADR 0004's status and user documentation to the tested subset. This record supersedes its design choices now without presenting the planned Job Object behavior as already shipped.
