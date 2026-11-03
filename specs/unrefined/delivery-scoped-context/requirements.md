## Scoped context, task dependencies and stale-evidence invalidation — Requirements

> Unrefined. Run /refine-spec delivery-scoped-context before /spec.
>
> Make original evidence available through scoped, bounded retrieval and make dependency changes invalidate affected work before reuse. This requirements-only MH-24/W07 document extends continuity-owned context foundations; it does not qualify S1/S2, launch native work or establish passing G07/G13. Normative source: `mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md`, version 2.0, §§4, 5.3, 9–11 and 18; `mythhelm-synthesis/MYTHHELM_Implementation_Plan.md`, W07. I-, G- and AT-IDs refer to v2.

## Context

- **Backlog card**: MH-24
- **Source issue**: [#130 — Scoped context, task dependencies and stale-evidence invalidation](https://github.com/turbokast/mythhelm/issues/130).
- **Grounding revision**: `615e74a6bb6907f87bd2065e2f576f5e18b5dec5` on `origin/main`. Re-ground citations before refinement/design; paths and symbols identify the relevant definitions.

The original task outcome and constraints must survive summaries, context pressure, dependency changes and handoffs. An index hit, transcript, Markdown task view or interface checkpoint cannot substitute for the exact admitted contract and independently verified integrated result. Users need to see which original sources informed work, what changed, which dependents became stale and why some evidence remains reusable (v2 §§9–11).

MH-15 owns the minimum immutable handoff/mandatory-core, scoped original-reference and native acknowledgement foundations. MH-24 consumes those foundations and adds the wider exact retrieval/project-memory surface, revisioned dependency graph, interface checkpoints and selective invalidation. Optional indexing can improve discovery but cannot become a prerequisite for retrieving an indispensable original or a source of authority. Contract-first implementation can proceed from independently accepted interfaces; it must not misrepresent the producer implementation as accepted.

The issue's initial schema-v1 premise describes historical adoption context. Current source already has canonical task-revision types and schema7 task tables. This specification extends those records rather than inventing a second task authority. Drafting requirements is actionable now; production integration and advertised S2 behavior remain sequenced behind the applicable S1 and continuity producers.

### Premise grounding

| Claim | Verdict | Source and consequence |
|---|---|---|
| W07 requires mandatory originals, bounded exact/optional lexical retrieval and revisioned dependency invalidation. | HOLDS | v2 §§4.4, 9.1, 9.3, 10.3, 11.1; implementation-plan W07. These are requested outcomes, not implemented capability evidence. |
| Schema-v1 has run/attempt/candidate records but no task-revision foundation. | PARTIAL | Historical `internal/journal/migrations/0001_init.sql` has those legacy records; current `0007_v2contracts.sql:6–19` already creates tasks/task_revisions, and `internal/v2contract/records_execution.go` defines TaskRevision/Dependency. Preserve this existing vocabulary and additive compatibility; do not apply the historical absence to current main. |
| Existing manifest/artifact types are reusable scoped-context foundations. | PARTIAL | `internal/v2contract/records_coordination.go:11–39` defines ContextManifest/Handoff, but its Validate checks only schema/manifest identity. `internal/v2contract/records_evidence.go:151` defines repository identity, content hash and retention roots on Artifact. These data shapes do not by themselves prove authenticated retrieval, source integrity or complete mandatory context. |
| Current durable task schemas already implement the complete dependency graph and scoped retrieval workflow. | PARTIAL | Structural enumeration `rg -n '^CREATE TABLE' internal/journal/migrations/*.sql` at the grounding revision shows legacy execution/qualification/billing/control, schema7 task/policy/grant/migration, schema8 verification-binding, schema9 attachment/dispatch and schema10 assignment-epoch tables. Schema7 stores revision records as JSON, not a completed reverse-invalidation/retrieval projection. Existing dependency fields are inputs to extend; no implementation absence is inferred solely from a keyword miss. |
| MH-15 provides the minimum originals/manifest foundation without requiring the whole MH-24 card. | HOLDS | `specs/unrefined/native-session-continuity/requirements.md`, FR-1/FR-2 and Dependencies, assigns the minimum grounded handoff there and the broader graph/retrieval here. It is an unrefined requirement, not a merged runtime producer. |
| S1, complete continuity and G07/G13 are currently qualified. | UNVERIFIABLE | This draft inspected source/specifications, not a complete qualification bundle. `docs/validation-plan.md` records route/journey evidence requirements. Qualification remains a release prerequisite; fixture or document acceptance cannot close it. |
| The issue's “PR126 pending merge” wording is current adoption status. | PARTIAL | That sentence is historical issue provenance. `docs/spec/README.md` identifies v2 as the adopted normative design; adoption does not implement or qualify it. |

### Objectives

- **O1**: Retrieve exact original task, artifact, decision and evidence content within authenticated scope, even with every optional index disabled.
- **O2**: Preserve mandatory originals and their provenance through context limits and continuity boundaries.
- **O3**: Change a dependency contract without silently accepting stale dependent work or discarding still-valid historical evidence.
- **O4**: Enable independently checked interface-first construction while retaining exact integrated acceptance.

### Non-Goals (and what still binds)

| # | Out of scope | Still binding in this spec |
|---|---|---|
| N1 | Reimplementing MH-15 native resume, session input writers, context acknowledgement or minimum handoff storage. | Consume its merged, reviewed producer contracts; I01/I18/I20/I23. Missing prerequisites block dependent runtime use. |
| N2 | MH-14 parallel writers, resource scheduler expansion or reassignment based on expired leases. | Dependency readiness never proves process termination or permits a second writer; I05/I18/I21/I24. |
| N3 | Required embeddings, hosted search, paid summarization, model inference or a mandatory MCP facade. | Exact originals and deterministic local fallback remain useful offline; I03/I13/I15. Any optional facade uses the same ACL. |
| N4 | MH-25 export/erasure/GC implementation or complete erasure claims. | Publish/preserve active provenance and retention roots for that consumer; I20/I23/I25 and §5.3. Storage pressure may block admission rather than delete active dependencies. |
| N5 | Automatic learning/policy promotion, spending grants, protected-check changes or a competing editable wiki. | Retrieved text and proposed memory are data; I03/I07/I20/I22/I23. |
| N6 | Claiming S1/S2, native-route/platform qualification or passing G07/G13 from these requirements. | Applicable release gates require independent current evidence; I09/I14. |

## Functional Requirements

### FR-1 — Consume and preserve the mandatory original core (v2 §§4.1, 9.1–9.2)

- **AC-1.1** [I20/I23, AT-14/AT-16] When constructing context for a task, the system shall consume MH-15's canonical immutable manifest/core and retain original outcome/constraints, exact task/acceptance/source revisions, authority/stop boundaries, accepted decisions, indispensable dependency facts, unresolved blockers and evidence index. A summary or search result shall not replace an original reference; an original-only constraint fixture shall reach the recipient unchanged.
- **AC-1.2** [I02/I09/I20, AT-16] If mandatory content is absent, corrupt, wrong-revision or cannot fit the qualified native window, then dispatch shall block with a bounded safe reason or require an explicit grounded decomposition/session reset; optional-context eviction shall never silently truncate mandatory constraints. Native project instructions remain in their supported location/precedence and their executable effects remain subject to trust.
- **AC-1.3** [I09/I20, AT-14/AT-17] When selecting optional context, the system shall record the exact source revisions, selection provenance and disclosed omissions separately from the mandatory core; native compaction/cache contents not actually observed remain unknown.

### FR-2 — Authorize exact bounded operations before lookup (v2 §§4.4, 8.3, 9)

- **AC-2.1** [I03/I23, G07, AT-16] When get_task, get_artifact, get_decision or get_evidence is requested, the authenticated connection shall supply repository/run/attempt scope and the request shall name exact revision and finite bounds. The system shall authorize scope before resolving content or existence; a caller-supplied repository ID or content hash alone shall not grant access.
- **AC-2.2** [I03/I09, G07, AT-16] If a reference is inaccessible, then responses, cursors, errors and search metadata shall disclose neither foreign content nor names, result counts or hash existence. Paired foreign-existing/foreign-missing fixtures, including equal-byte artifacts in separate repositories, shall be indistinguishable under the declared public response contract. Authorized missing references may receive scoped diagnostic detail.
- **AC-2.3** [I03/I20, G07] When an authorized exact read succeeds, the result shall identify its immutable source/revision, verify declared content identity and respect byte/result/time bounds. Arbitrary host paths, SQL and authority-granting/mark-verified operations shall not be exposed. CLI/file views are read-only projections; proposed repository changes use normal review.

### FR-3 — Optional search never replaces source truth (v2 §9.1, W07)

- **AC-3.1** [I13/I20/I23, AT-17] When optional lexical/FTS5 search is enabled, results shall be scoped, bounded references to exact canonical/source revisions; dereference shall revalidate authorization, freshness and integrity. A stale hit shall not become current evidence merely because its rank is high.
- **AC-3.2** [I09/I13, AT-17] If an index is absent, disabled, stale or corrupt, then the system shall retain exact artifact/path/symbol/decision lookup and bounded deterministic local file search without network/model calls. Unavailable mandatory originals shall still block; fallback shall not silently substitute another file/revision. Index-off and corrupted-index fixtures shall retrieve the same required originals.
- **AC-3.3** [I20/I23, AT-17] When an index is rebuilt, the system shall derive it from authorized canonical records and retained source artifacts, preserve original bytes and publish the rebuild atomically or keep it unavailable. Index edits shall not mutate contracts, accepted memory or verification records.

### FR-4 — Immutable task and dependency contracts (v2 §§4.1, 9.3)

- **AC-4.1** [I20/I23, AT-18] When a task is dispatched, its deliverable, acceptance contract, dependency artifact/contract revisions and relevant interface fields, write scope, risk, resource claims and integration destination shall be immutable under that task revision. A material edit creates an increasing new revision; neither transcripts nor Markdown views may change the canonical revision.
- **AC-4.2** [I20/I23, AT-18] When dependencies are created/revised, forward and reverse edges shall commit consistently in the same canonical ledger. Self-dependencies, cycles, missing required references and incompatible scoped identities shall refuse without a partial graph. Concurrent writers using the same expected revision shall not both install competing heads.
- **AC-4.3** [I20, AT-18] When an edge is recorded, it shall distinguish an independently validated interface/design checkpoint from accepted integrated implementation and bind the exact revision and fields required. A dependency shall not resolve against a mutable branch/path or an unspecified “latest” value.

### FR-5 — Selective invalidation and explicit revalidation (v2 §§9.3, 11.1–11.2)

- **AC-5.1** [I07/I20, AT-18] When a required fact, dependency interface, task contract or material accepted decision changes, affected dependents shall become stale, pending acceptance shall be revoked and new dispatch shall stop before reuse. The recorded traversal shall include affected transitive dependents; unrelated tasks remain usable with a recorded reason. A documentation-only change outside the declared interface shall not invalidate unrelated work merely because its file changed.
- **AC-5.2** [I12/I20/I21/I24, AT-18] If affected work is running, then the controller shall request its existing bounded safe-checkpoint/stop protocol according to ongoing-effect safety. Stale output may remain an immutable candidate, but cannot integrate until explicit justified revalidation against the current dependencies or a new task revision. A graph update/expired lease shall never certify a stopped writer or replay external effects.
- **AC-5.3** [I07/I20, AT-18] When a change races dispatch, candidate submission or acceptance, the transaction shall recheck the pinned task/dependency revisions and reject stale reuse. Retried mutation with the same operation and identical arguments returns the original result; changed arguments/expected revision conflict. Failure mid-update shall leave neither a partially installed graph nor dependents falsely current.
- **AC-5.4** [I07/I09/I20, AT-18] When historical acceptance later becomes unsuitable for a new dependency use, the system shall retain its original accepted evidence and append validity/supersession/revalidation evidence for the new use. Reusable unchanged checks require an explicit cache key covering all relevant inputs and a retained receipt; “same branch” is insufficient.

### FR-6 — Interface checkpoints and exact integrated acceptance (v2 §§10.3, 11)

- **AC-6.1** [I07/I20, AT-18] When an immutable interface checkpoint is independently accepted under its declared artifact/check contract, dependent construction may become ready only for edges permitting that checkpoint. The producer implementation and final dependent integration shall remain unaccepted until their own exact implementation/integration contracts pass.
- **AC-6.2** [I07/I20, AT-18] If the checkpoint changes, then the same reverse-invalidation rules shall apply; a fixture where a consumer builds against checkpoint A but receives implementation B shall refuse stale acceptance even after a clean textual merge.
- **AC-6.3** [I07/I08/I20, AT-18] When code acceptance is requested, it shall bind the exact integrated candidate/tree, current task/dependency revisions, immutable check definitions and environment with independent evidence. Component checks or preliminary checkpoints alone shall not mark final integrated work accepted; conflicting input candidates remain preserved.

### FR-7 — Scoped project memory and proposals (v2 §§4.4, 9.1)

- **AC-7.1** [I03/I20/I23] When submit_handoff, propose_decision or report_blocker is used, it shall use canonical typed records, authenticated scope, operation idempotency and expected revisions; it shall preserve claims versus observations and accepted decisions versus proposals. An agent-written instruction or control intent shall not create authority.
- **AC-7.2** [I03/I20/I23] When proposed project memory is accepted, the ledger shall retain its source evidence, scope, authorized decision owner or applicable standing rule, revision and supersession links. Without that authority/evidence it remains proposed; personal preferences remain user-owned. Generated file views shall not become independently editable competing architecture knowledge.

### FR-8 — Retain active lineage and expose truthful diagnostics (v2 §§5.3, 9.3)

- **AC-8.1** [I20/I23/I25, AT-43 boundary] While a manifest, checkpoint, task dependency, candidate or verification requires an original/provenance artifact, the system shall expose and retain its active roots/links for the MH-25 maintenance consumer. Missing or ownership-ambiguous lineage shall fail closed; storage pressure shall not silently delete an active required original. This proves the context-side retention contract, not export/erasure completion.
- **AC-8.2** [I09/I13/I25] When users inspect context or invalidation, read-only scoped projections shall show exact source/revision, staleness/revalidation reasons and observed retrieval/summary/repeated-read/handoff costs across the task. Missing native token/compaction measurements remain unknown; apparent token reduction shall not be reported as quality or qualification evidence.

## Non-Functional Requirements

- **NFR-1** [I13/I21] The following finite policy applies to exact retrieval, optional search, graph invalidation and derived-index rebuilds. Smaller MH-15 producer/native-qualified limits and the remaining outer admission deadline always win; increasing these ceilings requires a reviewed policy revision. Offline fixtures shall exercise each boundary and cancellation path.
  - An exact/search operation has a five-second deadline and at most 1 MiB encoded response, nesting depth64 and100 results. Original-reference and mandatory-context handling also obey the smaller MH-15 producer limits. Deterministic file fallback scans at most1000 files and16 MiB cumulative source bytes per request; a truncated search is explicitly incomplete and cannot substitute for a missing mandatory original.
  - A graph traversal visits at most10000 nodes,50000 edges and depth256 within five seconds. It checks cancellation between at most100 processed nodes or1 MiB read, whichever applies first. Material-change readiness is fenced before potentially incomplete closure work: if the complete affected set cannot be established, potentially affected readiness stays blocked until a complete traversal/reconciliation succeeds. Unvisited dependents shall never be presumed current. Large legitimate graphs require reviewed decomposition or explicit blocked handling, not silent partial invalidation.
  - A rebuild batch handles at most10000 records and16 MiB cumulative input within30 seconds; a complete job has at most128 batches and a15-minute wall-clock ceiling. Cancellation is checked at the same100-node/1-MiB processing intervals. If a ceiling is reached, the index stays unavailable, retained rebuild checkpoints/originals survive, and no partially built index is advertised as complete. Exact original retrieval remains available under its own bounds.
  - Deadline, size, depth or work exhaustion returns an explicit incomplete/blocked result. Mandatory originals are never silently truncated, optional work cannot extend the outer envelope, and an incomplete projection cannot advance task acceptance.
- **NFR-2** [I03/I20/I23] Shared authorization and canonical revision semantics shall apply identically to CLI/file and any later MCP/search facade; no index, raw host-path escape or content-address equality bypass is allowed.
- **NFR-3** [I09/I14/I20] New durable graph/context representation shall be additive, preserve legacy IDs and separately decoded historical formats, and reject unknown critical schema before mutation. Migration/restart/failure fixtures shall preserve originals and committed revision/invalidation state; no migration number or exact API is preallocated here.
- **NFR-4** [I07/I21] Tests shall include non-empty successful retrieval/dispatch legs plus violating fixtures for mandatory-core truncation, unauthorized existence probes, stale index trust, cycle acceptance, partial invalidation, stale-worker integration and checkpoint-as-implementation acceptance. Retry-until-green and source-label matching are not acceptance.

## Dependencies

- **Now**: requirements capture, independent refinement and later contract design may proceed before full S1; this PR authorizes no production implementation, native trial or qualification claim.
- **Producer ownership / no cycle**: `specs/unrefined/native-session-continuity/requirements.md` FR-1/FR-2 owns the minimum immutable manifest/core, exact scoped originals and continuity delivery/acknowledgement. MH-24 consumes those merged runtime contracts; it does not require MH-15 to consume MH-24's optional indexes or full graph before its minimum handoff can ship. Shared core/ACL/artifact producers must be named and reviewed before dependent implementation; if absent, create a bounded same-domain producer there rather than duplicate authority here. The edge is MH-15 foundations → MH-24 expansion.
- **Production/release**: applicable S1 canonical controller/authority/protected-acceptance foundations and MH-15/W06 continuity are prerequisites for integrated W07 behavior. Full S2 also requires MH-25/W13 export/erasure and the applicable two-route qualification evidence. These are blocked/unproven until independently demonstrated; documents and synthetic tests do not satisfy them.
- **Existing foundations**: extend `specs/done/v2-contract-vocabulary/`, `specs/done/supervisor-migration/`, `specs/done/supervisor-service/` and `specs/done/protected-acceptance/`. Their merged records/migration/evaluator contracts are inputs, not blanket S1 qualification.
- **Concurrent scope**: `specs/in-progress/supervisor-attachment-control/` owns production dispatch/operations/grants/owner lifetime and current journal migrations; `specs/*/existing-run-client/` and `specs/*/herdr-scoped-projection/` own attach-only projections. Re-ground actual merged versions and serialize overlapping files/migrations; do not infer ownership from a context manifest.
- **Downstream**: MH-14 consumes the graph/checkpoint foundations for parallel construction; MH-25 consumes scoped artifact/retention roots; extensions/learning consume validated context interfaces after their own gates. No whole-card reverse dependency is added.
- **Supersession**: no predecessor spec, original source, historical issue or accepted evidence is replaced by this requirements-only document.

## Impacted components

These are investigation areas, not an implementation file list or new package/API commitment. Later design must enumerate actual producers and domain-pure task scopes.

- **Canonical contracts**: `internal/v2contract/records_execution.go` (TaskRevision/Dependency), `records_coordination.go` (ContextManifest/Handoff/Message) and `records_evidence.go` (Artifact/Verification); extend existing identities and explicit unknown semantics.
- **Ledger/persistence**: `internal/journal/` and migrations, whose complete current table inventory was inspected above; atomic graph/revision/invalidation projections and migration compatibility must coexist with current service-control producers.
- **Context/delivery/controller**: existing `internal/control/`, `internal/service/`, `internal/supervisor/` and MH-15's future merged context producer; exact scoped lookup and freshness must gate dispatch/integration without adding a second scheduler or owner.
- **Protected acceptance/integration**: `internal/acceptance/`, `internal/integration/` and journal verification bindings; refinement must bind current dependency revisions to actual acceptance, not only presentation state.
- **User projections/docs**: `internal/cli/`, `internal/tui/`, `tests/e2e/`, `docs/user-guide.md`, `docs/limitations.md`; optional local search stores are derived implementation choices, not canonical authority.

## Open Questions

These are bounded refinement/design obligations with conservative defaults, not requests for founder decisions.

- **OQ-1 — Exact shared producer contracts.** Blocks implementation task planning. Identify MH-15's actual merged manifest/original-reference/ACL/artifact APIs and the additive graph transaction boundary. Default: consume one canonical core producer, never duplicate task/manifest authority or make optional indexing a continuity prerequisite.
- **OQ-2 — Resolved: finite bounds and incomplete traversal.** NFR-1 freezes the retrieval/fallback/graph/rebuild ceilings and conservative readiness fence. Design must map these fixed limits to implementation and boundary fixtures, apply smaller MH-15/native/outer limits, and preserve mandatory originals; this is no longer an undecided policy prerequisite.
- **OQ-3 — Material interface and revalidation policy.** Blocks implementation. Freeze explicit field-level dependency contracts, affected-closure semantics, atomic race checks and independent evidence required for justified reuse. Default: changed required facts invalidate; unrelated documentary changes require recorded justification; no accepted-history rewrite.
- **OQ-4 — Integrated readiness and release evidence.** Blocks production/release claims, not requirement capture. Locate current S1/MH-15 and G07/G13 evidence for the exact supported subset; missing evidence stays unknown/blocked. MH-25's complete maintenance qualification remains separate from this document's active-root contract.

## Definition of Done

- [ ] Independent refinement accepts the grounded premises, no-cycle producer ownership and explicit blocked production/release prerequisites.
- [ ] Design freezes exact core/retrieval/ACL/graph/invalidation/checkpoint contracts and maps the fixed NFR-1 bounds without introducing a second ledger or optional mandatory service.
- [ ] FR-1–FR-3/FR-7 demonstrate original-preserving scoped context, no existence leaks, optional-index failure and authorized project-memory provenance.
- [ ] FR-4–FR-6 demonstrate cycles/concurrent revisions, complete selective invalidation, stale output refusal, justified revalidation and exact integrated acceptance.
- [ ] FR-8's active provenance contract is exercised without claiming MH-25 export/erasure; offline credential-free tests cover successful paths and meaningful violating fixtures.
- [ ] Applicable S1/MH-15/W06 and G07/G13 evidence is independently established before advertised integrated W07/S2 claims; all remaining limitations remain visible.
