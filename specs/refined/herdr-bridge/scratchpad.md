# Herdr bridge — Scratchpad

## PR #477 review clarifications

All five findings on requirements head `8eef18f` were confirmed as missing precision in the existing first-slice scope. These are requirements changes; implementation and live qualification remain pending.

| Review thread | Disposition | Acceptance evidence now required |
|---|---|---|
| `discussion_r4281883511` — scope/read race | Addressed in AC-3.1. | Scope, generation and projection selection use one consistent snapshot; deterministic authority/generation races cannot mix authorization and data from different snapshots. |
| `discussion_r4281883521` — stale stop mutation | Addressed in AC-4.1. | Authorization and revision/generation comparison are atomic with stop recording; competing updates reject a stale stop before recording/delivery. |
| `discussion_r4281883528` — late stop acknowledgment | Addressed in AC-4.2. | Durable acknowledgment within 2 seconds independently of reconciliation, operation-ID result query/replay, and disconnect/reconnect with worker confirmation delayed beyond 2 seconds. |
| `discussion_r4281883541` — regressing projections | Addressed in AC-3.4. | Attachment/generation-bound monotonic revisions; older and old-generation messages discarded, identical duplicate ignored and conflicting duplicate invalidates freshness. |
| `discussion_r4281883550` — missing user entry point | Addressed in AC-1.4 and new AC-1.5. | Public `mythhelm attach <run-id>` and help, tested end to end for initial/repeated attachment and fallback without host registration. |

Grounding: `internal/control/stop.go:84` (`rg -n 'func StopHandler' internal/control/stop.go`) records the request, calls `workers.RequestStop` and then waits for the worker report before returning. The new embedded acknowledgment contract therefore needs design work; the current synchronous handler is not proof of compliance. `internal/cli/dispatch.go:31` (`rg -n 'var commands' internal/cli/dispatch.go`) still has no attach command, as recorded in requirements Context.

OQ-1 and OQ-2 in requirements remain design investigations with conservative defaults: disable unqualified host features and refuse unsupported attachment/control; the designer resolves exact contracts, with the maintainer deciding any change to accepted ownership policy. No writable native takeover or broader authority was added by these clarifications.
