# Supervisor attachment control — Scratchpad

> Draft research and open questions; implementers append discoveries.

## Open questions

| # | Question | Default in this spec | Owner / blocks |
|---|---|---|---|
| 1 | Exact merged MH-23 lifecycle contracts at implementation start | Re-ground Tasks4–9; preserve worker deadline and distinct client detach/stop, never forward owner detach from a service client. | Core implementer; blocks shared lifecycle/CLI tasks, not contracts/storage. |
| 2 | Composition ADR review | ADR 0021 was accepted after exact-revision review in PR #501; no topology weakening or product-signature inference. | Task 1 completed; Task 22 separately gates the proposed admission-binding decision. |
| 3 | Concurrent Windows process-boundary changes | Serialize worker files; Linux evidence may proceed; Windows unsupported until independently proven. | Core/platform owner; blocks Windows claims. |
| 4 | Native environment drift in a long-lived service | No caller environment wire field; revalidate native evidence and return explicit re-admission/restart guidance when service environment cannot satisfy pinned route. | Core admission owner; blocks qualifying affected route, never permits silent fallback. |
| 5 | Oversized core stream | Keep approved three streams; the original 21 core tasks plus proposed Task 22 gate remain independently reviewable because singleton dispatch/ingest/control must agree before attach-ready claim. | Spec reviewer; review task sizing, no enrollment-only fallback. |

## Research notes

- Grounded at61d18e2; source refinement f96cc05. Production service registry is incomplete, so ordinary dispatch is explicit prerequisite work.
- The approved three-stream restructuring preserves original source clauses and IDs; it does not approve any ownership-policy waiver.
- Initial architecture review rejected per-run recover enrollment; root-wide migration can preserve live workers after safe legacy owner release.
- Source archive happens only after clause coverage and byte-preservation checks, using lifecycle script. Independent spec validation remains required.

## Discoveries

- Task 1: Independent architecture/code review accepted the concrete ADR 0021 proposal at `e50faa8` (original-file SHA-256 `b30067e53dadaddbc416f8501ed67ea127ff8b02f7f6591be0b86fd49ca53393`) with a Ready verdict and no critical, important or conflicting-contract findings. The delivery coordinator accepted that exact proposal under direct operator delegation and independently reviewed author output `5386aff` as Ready. PR #501 changes ADR status/authority evidence, the decision index and own task records; the Decision section is byte-preserved. No runtime or platform qualification is claimed.
- Task 1 lifecycle: PR #498 merged as `b2b46e5` before completion records were written. WORKFLOW/task-completion require the entry in the task PR before merge, taking precedence over this child's heading-after-merge wording; dependent code still waits for Task 1's merged PR.

- Task 2: Source `41813fe` supplies exact typed protocols and clients before runtime producers. Full code/architecture review corrected an initially contextless DialExisting into one shared two-second OS-dial/root-check budget; five transport paths were approved within the same core domain. A later author check exposed valid pointer and null decoder panics before publication; retained pointer/bounds/null/identity fixtures and a skip-Validate counterfactual now verify strict validation. Fresh independent reviews returned Ready; first-pass delivery is false because these corrections were necessary.
- Task 2 contract clarification: StopResult reasons are empty, stop_unconfirmed or a valid existing v2 error code, with later producers selecting meaningful subsets. Historical v1/canonical v2 event Type vocabulary stays open per the existing envelope contract; future critical schemas fail capability_unsupported. Projection review/attention values are closed enums, labels are bounded printable Unicode and unknown optional fields remain null. New service handlers and process ownership are not established by this protocol task.
- Task 2 evidence: PR #506; final full race gate 420.5s, zero-issue lint, Windowsamd64/Darwinarm64 cross-vet exit0; coordinator acceptance/pointer race rerun 1.312s. Earlier socket-restricted verification failures were environment errors, not passing tests; all required final gates subsequently passed outside that restriction with isolated state/runtime paths. No live platform, billing or native qualification is inferred.

- Task 3: PR #507, source `4379c3e`, integrated `7b68070`, adds schema 9 and complete transaction-scoped run revisions. Actual write sites are inventoried in its hand-off; run timestamps/spool offsets remain bookkeeping, while all selected identities/evidence and old/new indirect bindings participate. New producer code must expand this inventory when it adds a projection dependency.
- Task 3 review discovery: SQLite replacement can implicitly delete a conflicting row without DELETE triggers. Completed acknowledgments and immutable outboxes now protect every declared unique key and physical row alias for both INSERT and UPDATE. Completed INSERT duplicates use IGNORE to preserve the real concurrent long-claim path. Retained unreferenced-victim cases and guard-drop counterfactuals expose missing guards rather than relying on a foreign-key refusal; explicit negative collisions fail closed and normal positive allocation remains tested. No global recursive-trigger setting or unrelated authority-table redesign was added.
- Task 3 compatibility discovery: A schema bump must move the backup and preview version mirrors and ordered migration list together (ADR0019), as well as current support evidence/backup guidance and independent preview expectations. The historical schema-8 inventory remains exact and isolated. Full race validation passed after these corrections; the earlier failed mirror run is not counted as evidence. Root and fresh independent code/architecture review returned Ready on the final integrated source; service producers remain later tasks.

## Validation round 1 fixes

Fresh independent review found missing standalone observation/results, canonical adoption schema mismatch, stop producer ordering, host freshness/resync transport and stable-status TTL renewal. The author added exact typed contracts, canonical schema2 adoption with separate historical decoding, producer-first dependencies, safe freshness fanout/read-only resync and independent qualified publication renewal. Shared docs wait for docs-site-restyle Task11. These are contract/ordering corrections, not source requirement or ownership-policy changes. Round2 fresh validation is pending; this author check is not an independent verdict.

## Validation round 2 fixes

- Internal grant primitives precede public Open+projection composition; Task11 produces no incomplete public response, and Task12's rollback test covers projection failure after mint.
- Task6 tests only the singleton shell and surviving root context. Full production registry and continuing worker ingestion are tested by Task20 after dispatch/control producers and public consumers.
- Core Tasks16–19 serialize after MH-23 Task18 shared CLI edits; Task21 serializes after MH-23 Task19 and docs-site-restyle Task11 shared documentation. Independent contract/storage work is not blocked by unrelated presentation work.
- All changes are producer-ordering/evidence/coordination corrections. No requirement, topology or authority-policy change. Round3 independent validation is pending.

## Resumed spec run — validation round 1

After the delegated decision to preserve the producer privacy contract, a new spec run received a fresh independent whole-epic review. The epic and all three children returned Ready for implementation with zero findings; specs lint exited 0. This is resumed-run round1, following the prior run's three rounds and correction checkpoint recorded above, not a fourth round in that run. The unknown/foreign run sweep confirms uniform permission_denied/exit3; usage errors remain exit2.

At this validation round, the three child specs moved through the lifecycle script to todo; the epic remained unrefined until its first child started. Counts were core21/client7/host8, total36, and ADR0021 was still proposed. Archived source hashes and all18 AC plus3 NFR clauses remained unchanged. Runtime prerequisites and live host qualification were future acceptance gates. Product and orchestration approval ledgers were untouched. Task 1 later accepted ADR 0021 in PR #501; the proposed reconciliation below adds a 22nd core task, making the current proposed inventory core22/client7/host8, total37.


## Proposed admission-binding reconciliation (draft; not approved)

Task 4's retained PR #527 is unmerged. At its inspected head `2726054c7de25f4ecf2074a4878f1f134fdf73a9`, `internal/supervisor/service_owner.go:182-209` passes the provided decision to `runLocked`; `internal/supervisor/pipeline.go:182-187` constructs the pipeline and calls `p.run`, and `:290-302` starts by creating the run. Recheck with `git show 2726054c7de25f4ecf2074a4878f1f134fdf73a9:internal/supervisor/service_owner.go | rg -n 'func RunOwned|return runLocked'` (observed lines 184 and 209) and `git show 2726054c7de25f4ecf2074a4878f1f134fdf73a9:internal/supervisor/pipeline.go | rg -n 'func runLocked|err := p.run|func \(p \*pipeline\) run\(|CreateRun\('` (observed lines 184, 187, 290 and 292). These describe the PR candidate, not `origin/main`. At `origin/main` `48edc40925862f0eaf3ed80f41c7648c6c11e063`, Task 3 schema 9 (`internal/journal/migrations/0009_attachments.sql:90-109`) declares `prepared_runs`, `dispatch_outbox` and `admitted_evidence_sha256`; `git show 48edc40925862f0eaf3ed80f41c7648c6c11e063:internal/journal/migrations/0009_attachments.sql | rg -n 'CREATE TABLE prepared_runs|CREATE TABLE dispatch_outbox|admitted_evidence_sha256'` returns lines 90, 103 and 109 among other references. These fields alone do not demonstrate reconstruction of every admission value. This is a design gap, not evidence of a production bypass. At the same main revision, `git show 48edc40925862f0eaf3ed80f41c7648c6c11e063:specs/in-progress/supervisor-attachment-control/tasks.md | rg -n '^### Task 4|^### Task 6'` shows both tasks incomplete at lines 91 and 136.

Open question: choose and review the exact non-secret immutable representation, reconstruction, proof-bearing service-to-supervisor API and any additive migration before Task 9. Conservative default: service start remains unsupported until the committed binding is verified and its recovery path is tested; a mutable decision plus its own digest is not authorization. Task 22 owns the ADR; exact-revision architecture and maintainer review own the decision before Tasks 6–9 proceed. The Task 4 synchronous callback/close reentrancy and unbounded wait findings remain separate review issues; wording here does not resolve them.
