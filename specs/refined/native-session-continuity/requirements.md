## Native-session continuity and grounded handoffs — Requirements

> Refined after independent assessment; see [refinement-log.md](refinement-log.md). Proceed to /spec native-session-continuity.
>
> Preserve useful native conversations through compatible admitted attempts, or reconstruct an explicitly new session from original requirements and immutable evidence. This is the full MH-15 requirements envelope, not a claim that S1, two routes, native resume or context acknowledgement have qualified. Normative source: `mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md`, version 2.0, §§4, 6, 9 and 18; implementation-plan W06. I-, G- and AT-IDs below refer to v2.

## Context

- **Backlog card**: MH-15
- **Source issue**: [#36 — Messages, blackboard and handoff packages](https://github.com/turbokast/mythhelm/issues/36).
- **Grounding revision**: `eef0855983659db003cc5ea8fd7c69ac8552e640` on `origin/main`. Recheck these citations during refinement; line numbers are hints and symbol searches below locate the cited definitions.

A mission (`Run`) can outlive one native execution episode. Reconnecting a proven live launch, resuming a compatible harness-owned conversation in a new admitted attempt, and rebuilding context in a fresh session have different ownership, billing and information-loss consequences. A recorded session identifier or successful process exit proves none of the other operations. The same run must retain its original outcome, constraints, contract revisions, decisions, artifacts, unresolved work and independent verification lineage across these boundaries (v2 §§4.1, 6.2, 9.1–9.2).

The existing foundations include worker-owned starts, native-session observations, durable recovery, canonical v2 record types and task revisions. They do not establish the complete continuity workflow. The card requires S1, MH-21 and two independently qualified routes; source-level schemas and fixture-only adapter behavior cannot satisfy those prerequisites. Specification and credential-free contract fixtures may progress while the native/build/claim gates remain explicit (Dependencies below).

Issue #36 predates v2 and describes a blackboard and parallel agents using historical section numbers. Preserve its useful message/provenance/context-budget objectives through canonical ledger records and scoped projections. Do not create another editable task store, require parallel construction for S2, or reinterpret its old references as current v2 numbers. No original issue context is discarded by this reconciliation.

### Premise grounding

| Claim | Verdict | Source and consequence |
|---|---|---|
| MH-15 needs acknowledged native deltas, explicit new attempts and original-evidence handoffs. | HOLDS | `product/backlog.md`, MH-15; v2 §§4.4, 6.2, 9.1–9.3; W06. This is the requested behavior, not current capability evidence. |
| Native session identity and durable recovery foundations exist. | HOLDS | `internal/workers/worker.go:905` emits `attempt.native_session`; `internal/journal/projections.go:360` stores the native ID; `internal/supervisor/recover.go:75` reconciles existing ownership. Locate with `rg -n 'case adapter.SessionStarted|func SetNativeSession|func recoverLocked' internal/workers/worker.go internal/journal/projections.go internal/supervisor/recover.go`. Identity storage is not resume authorization. |
| Existing adapter capability declarations provide a foundation for negotiated native continuity. | PARTIAL | There are capability fields, but `Adapter` has Probe/Capabilities/Prepare/Start only and `Session` has Observations/Interrupt/Done only (`internal/adapter/adapter.go:17,175,383`). `adapters/claudecode/probe.go:139` reports resume/live steering unsupported; `adapters/codex/compat.go:53` leaves them unknown. Their actual launch builders select print/text input or the fixture-tested app-server placeholder (`adapters/claudecode/launch.go:49`, `adapters/codex/launch.go:43`). Locate with `rg -n '^type (Adapter|Session|Capabilities)|^func Argv|Resume:' internal/adapter/adapter.go adapters/claudecode/{probe,launch}.go adapters/codex/{compat,launch}.go`. New operation-specific qualification is required. |
| The canonical vocabulary includes handoffs and messages. | HOLDS | `internal/v2contract/records_coordination.go:11,37,44` defines ContextManifest/Handoff and Message. Locate with `rg -n '^type (ContextManifest|Handoff|Message)' internal/v2contract/records_coordination.go`. These values do not prove durable delivery or source-reference validation. |
| Existing ContextManifest/Message/Attempt types provide reusable continuity foundations. | PARTIAL | ContextManifest validates schema/identity and Message validates nonempty fields (`internal/v2contract/records_coordination.go:25,59`); Attempt already carries native_session_ref/context_manifest_id (`internal/v2contract/records_execution.go:82`). Task/contract foundation tables exist in `internal/journal/migrations/0007_v2contracts.sql`. Enumerating `rg -n '^CREATE TABLE' internal/journal/migrations/*.sql` at the grounding revision finds no continuity manifest/message/session-writer delivery projection; no absence is inferred from a single keyword search. Refinement must extend the existing ledger, not duplicate it. |
| Full S1 and two independently qualified native routes are ready. | UNVERIFIABLE | No complete S1/two-route qualification bundle is established by this requirements work. The exact blockers in `docs/validation-plan.md:59` remain; Codex admission rejects unknown sign-in evidence and requires live purchased-credit prevention (`internal/admission/admission.go:332–393`), while Claude strict admission checks unresolved managed sources (`internal/admission/admission.go:487`). Treat readiness as blocked/unproven until actual evidence closes each applicable gate. |
| Historical blackboard/parallel-agent terminology directly describes the current architecture. | PARTIAL | [Issue #36](https://github.com/turbokast/mythhelm/issues/36) is preserved historical context; current v2 §§4, 9 and I23 require one canonical ledger. Parallel construction is MH-14/S3, not a prerequisite added to this card. |

### Native documentation evidence

The [official OpenAI non-interactive documentation](https://learn.chatgpt.com/docs/non-interactive-mode) documents exact-session-ID `codex exec resume`; the [official CLI reference](https://learn.chatgpt.com/docs/developer-commands?surface=cli) distinguishes resume and fork. The [official Claude Code programmatic documentation](https://code.claude.com/docs/en/headless#continue-conversations) documents resuming a particular conversation by session ID. These pages were read for this draft on 2026-11-03. They identify documented candidates, not proof of the installed version, MYTHHELM's chosen native surface, exclusive input ownership, delivered-delta acknowledgement, entitlement, or safe workspace/configuration reuse. No command was executed and no native store was read. Adapter-specific refinement must bind official operation evidence to its exact qualified bundle without substituting another SDK, API or execution surface.

### Objectives

- **O1**: Continue a mission with its original outcome, constraints, acceptance contract and original-evidence lineage intact.
- **O2**: Prefer a compatible qualified native conversation where useful, with explicit new-attempt admission and exclusive input ownership; show the difference from reconnect or reconstruction.
- **O3**: Preserve work and disclose context loss when a native session cannot be continued; re-admit a fresh same-harness or authorised cross-harness session from verified artifacts.
- **O4**: Deliver typed, scoped messages and truthful context acknowledgements from one canonical ledger; a user can inspect what was sent, acknowledged, queued, rejected or uncertain.

### Non-Goals (and what still binds)

| # | Out of scope | Still binding in this spec |
|---|---|---|
| N1 | Reading, copying, patching or replaying private native conversation databases, subscription secrets, provider caches or hidden reasoning. | I01/I16/I19 (v2 §2): use supported qualified native operations and canonical original artifacts; unsupported continuity is disclosed. |
| N2 | Account creation, account rotation, buying credits, changing plans, routing policy optimization or automatically changing the user's pinned harness/profile. | I02/I04/I15/I21; MH-5/MH-17 own routing/profile expansion. Every destination is admitted against applicable standing authority and finite envelopes. |
| N3 | Parallel construction, a second scheduler, writable native input takeover, or a second editable blackboard/task store. | I05/I18/I23/I24; all attempts and messages use the canonical supervisor/ledger and one session input writer. MH-14 and host-takeover work remain separate. |
| N4 | The complete retrieval/dependency graph, optional FTS/embedding indexes, automatic project-memory promotion, export/erasure or learning. | I03/I09/I20/I22; mandatory original context and exact scoped references are required here. MH-24/MH-25 own their broader lifecycle; active provenance must remain retainable. |
| N5 | Claiming that these requirements, synthetic fixtures, route declarations or one native adapter earn S1, S2, G02/G05/G13/G16 or another platform's qualification. | I07/I09/I14; real supported-subset and complete two-route evidence remain explicit release prerequisites. |

## Functional Requirements

### FR-1 — Immutable handoff and mandatory core (v2 §§4.1, 9.1–9.2)

- **AC-1.1** [I07/I20/I23 (v2 §2), AT-14 (v2 §18.2)] When a continuation or handoff is prepared, the system shall persist an immutable manifest referencing the original outcome/constraints, exact task/contract and source/base/candidate revisions, accepted decisions with evidence, changed files/interfaces, actual check results, unresolved work and risks. Original requirements remain addressable; a summary alone is insufficient. A fixture with original-only constraints must retain those constraints at the recipient.
- **AC-1.2** [I03/I09 (v2 §2), AT-14 (v2 §18.2)] When a manifest contains proposed approaches, agent statements or suggested next actions, the system shall distinguish claims and suggestions from observations, accepted decisions and canonical Verification records. Delivering a handoff shall not accept a task or create authority.
- **AC-1.3** [I20 (v2 §2), AT-16 (v2 §18.2)] If the mandatory core exceeds the qualified context envelope, then the system shall preserve every requirement and report a bounded decomposition/reset requirement or block; it shall not silently truncate constraints, acceptance criteria, authority/stop boundaries, indispensable dependencies or the evidence index.

### FR-2 — Authenticated scope and freshness (v2 §§4.4, 8.3, 9)

- **AC-2.1** [I03/I20/I23 (v2 §2), AT-16 (v2 §18.2)] When a sender or recipient requests a referenced requirement, artifact, decision or evidence record, the system shall authorize repository/run/attempt scope before resolving it and enforce explicit revision and size bounds. Foreign content, names, counts and hash-existence metadata remain inaccessible; arbitrary host paths and SQL are not context operations.
- **AC-2.2** [I07/I20 (v2 §2), AT-14/AT-18 (v2 §18.2)] If a required reference is missing, wrong-scope, corrupt, superseded or inconsistent with the exact task/candidate, then delivery shall refuse with the affected reference and reason while preserving the manifest and original sources for repair; stale data shall not advance acceptance.
- **AC-2.3** [I09/I13 (v2 §2), AT-17 (v2 §18.2)] If optional search/index support is absent, corrupt or stale, then continuation shall still use validated exact original-artifact lookup without a hidden network/model dependency; it shall block only when required originals themselves are unavailable. This does not claim MH-24's full retrieval/dependency implementation.

### FR-3 — Reconnect, native resume and new-session identity (v2 §§4.4, 6.2–6.4)

- **AC-3.1** [I06/I12/I18 (v2 §2), G04 (v2 §18.3)] When a previously admitted live launch is reconnected, the system shall retain its attempt/launch identity, process owner, writer and consumed envelope; it shall issue no replacement native start and reset no attempt/repair budget. A launch counter and exact identity fixture must discriminate reconnect from resume.
- **AC-3.2** [I02/I12/I18/I21 (v2 §2), AT-15 (v2 §18.2)] When a compatible native session is resumed after its previous writer has been reconciled gone, the system shall create a new admitted attempt/number bound to that exact session, current contract, workspace/configuration bundle, authority and remaining envelopes, and acquire its exclusive native-session writer lease before input. A live or unknown previous writer shall block resume; an expired lease alone is not proof of quiescence.
- **AC-3.3** [I01/I14/I16 (v2 §2), AT-14/AT-15 (v2 §18.2)] If the exact native operation/version/surface cannot safely resume, or the returned session differs from the requested identity, then the system shall refuse the operation before further input, preserve artifacts and classify continuity as unavailable or uncertain. It shall not use a most-recent-session selector or silently replace the native harness/surface.

### FR-4 — Exact deltas and native acknowledgement (v2 §§4.1, 9.1–9.2)

- **AC-4.1** [I09/I20 (v2 §2), AT-14 (v2 §18.2)] When a compatible session has acknowledged an exact context revision, the next dispatch shall include the mandatory core, indispensable small dependency facts upfront and original references, plus precisely the relevant unacknowledged deltas after that revision, bound to the recipient session and current manifest. A receipt from another session or an older contract shall not advance its acknowledgement frontier.
- **AC-4.2** [I09/I12 (v2 §2), AT-14 (v2 §18.2)] If delivery has no authoritative native acknowledgement, then the system shall record it as uncertain or queued as appropriate, retain unacknowledged deltas and avoid claiming the native remembered them. A successful transport write, native exit, UI paint or repeated summary shall not count as acknowledgement.
- **AC-4.3** [I03/I18/I20 (v2 §2), G13 (v2 §18.3)] When acknowledgement is observed, the system shall persist the exact delivered manifest/delta identity, native session, producing surface/version and observed acknowledgement before changing future context selection. Acknowledgement proves that identified delivery, not semantic understanding or later retention. Duplicate observations are idempotent; an interrupted/ambiguous delivery is reconciled before retry.

### FR-5 — Grounded reconstruction and cross-harness continuation (v2 §§6.4, 7, 9.2)

- **AC-5.1** [I02/I04/I16 (v2 §2), AT-15 (v2 §18.2)] When a session is lost, incompatible, explicitly reset or unavailable, an authorised eligible destination may start a fresh admitted attempt/session from validated original artifacts. Its receipt shall disclose the continuity mode, reason, source attempt/session and expected native-context loss; unsupported reconstruction remains blocked with preserved artifacts.
- **AC-5.2** [I04/I15/I16 (v2 §2), AT-03/AT-04/AT-25 (v2 §18.2)] Before changing harness, account/profile, provider, execution host, funding route or security posture, the system shall re-admit the exact destination against applicable user authority, pinned preferences, effective configuration and billing qualification. Exhaustion or a handoff suggestion shall not authorize paid continuation, identity rotation or a substitute route.
- **AC-5.3** [I03/I09/I19 (v2 §2), AT-14/AT-15 (v2 §18.2)] When reconstructing across harnesses, the system shall deliver verified canonical references and disclosed claims only; it shall transfer no private native-store format, secret, hidden reasoning or provider cache. If originals lack a necessary decision, the affected work shall report precisely that missing decision while independent authorised work can continue.

### FR-6 — Typed messages and serialized delivery (v2 §§4.1–4.3, 9.3)

- **AC-6.1** [I09/I18/I23 (v2 §2), G13 (v2 §18.3)] When a message is created, it shall carry sender/recipient scope, one of `information`, `question`, `blocker`, `handoff`, `control_intent`, causal parent when applicable, references, sensitivity and expiry, and a canonical delivery disposition from `pending`, `delivered`, `acknowledged`, `queued_for_next_turn`, `rejected`, `expired`, `uncertain`. A nonempty arbitrary string alone is insufficient validation at the enabled operation boundary.
- **AC-6.2** [I03/I18 (v2 §2), AT-07 (v2 §18.2)] While a native surface is busy or lacks qualified safe steering, the supervisor shall queue eligible messages for the next safe turn and retain their disposition instead of injecting characters into a busy shell or granting another writer. A fixture with a blocked/busy input channel shall show no second native write.
- **AC-6.3** [I03/I12/I20 (v2 §2), G04/G13 (v2 §18.3)] When delivery/control is retried or replayed, an identical operation shall return its durable prior disposition; a reused operation identity with different arguments shall conflict. Stale expected revisions/generations and expired/revoked authority shall refuse control intents. Message text never grants an effect or marks work accepted.

### FR-7 — Bounded recovery and read-only projections (v2 §§6.4, 9.3, 17.2)

- **AC-7.1** [I06/I12/I21 (v2 §2), G04 (v2 §18.3)] If a continuation or message transfer is interrupted, recovery shall reconcile the exact prior launch/session/writer and possible effects before retry. The default transport allowance is at most three bounded transient retries; an unresolved ownership/effect reconciliation stops further attempts until fresh evidence or user action exists. It shall not replay prompts/native tools from event history or reset envelopes.
- **AC-7.2** [I09/I23 (v2 §2), G13 (v2 §18.3)] When CLI/TUI/file context views show pending, delivered, acknowledged or reconstructed work, they shall project the same canonical ledger revision and distinguish those statuses from acceptance. Editing a generated view shall not mutate the canonical delivery/task state.
- **AC-7.3** [I20/I23 (v2 §2), AT-43 (v2 §18.2)] While a handoff/context/message is required by active work, its original artifacts, acknowledgement lineage and reference dependencies shall remain retention roots. Older runs remain readable after additive migration; legacy session IDs shall not be promoted to resumable or acknowledged merely because they exist. Full export/erasure remains MH-25.

### FR-8 — Supported-subset evidence and complete journeys (v2 §§7, 15.2, 18)

- **AC-8.1** [I09/I13/I14 (v2 §2), AT-14/AT-15 (v2 §18.2)] Credential-free scripted-adapter journeys shall prove same-session new-attempt semantics, wrong-session refusal, exact delta acknowledgement, unsupported steering queueing, crash/uncertain delivery, new-session reconstruction, original-constraint preservation and source protection. These fixtures shall remain visibly distinct from live native-route evidence.
- **AC-8.2** [I02/I14/I15 (v2 §2), G02/G05/G13 (v2 §18.3)] Before advertising the complete two-harness workflow, S1 readiness and both independently qualified route bundles shall have current required evidence, including operation-specific resume/fallback, fidelity, lifecycle, input ownership, effective configuration and billing boundaries. One successful sign-in, external documentation or a fixture-only adapter shall not satisfy these gates.
- **AC-8.3** [I07/I09/I25 (v2 §2), G04/G13 (v2 §18.3)] When a user inspects a completed continuity journey, its receipt shall identify source/destination route and session/attempt identities, exact manifest revisions, acknowledgement status, loss disclosures, remaining unknowns, actual checks and blocked effects, and provide a safe return-to-native path without a MYTHHELM service dependency. No claimed quality/speed improvement follows solely from context/token reduction.

- **AC-8.4** [I09/I21 (v2 §2), v2 §9.3] When reporting context/resource consumption across a mission, the system shall retain separately labelled observed/reported/estimated/unknown initial inputs, deltas, retrieval, native compaction when reported, summaries, repeated reads, handoffs, retries and verification. Missing native counters remain unknown and never zero; token reduction alone shall not establish useful continuity, cost savings or quality.

## Non-Functional Requirements

- **NFR-1** [I13 (v2 §2)] Normal contributor tests use scripted/native-protocol fixtures only, with no agent credentials, paid inference, account setup or network requirement. Any separately authorised live qualification is opt-in and version-bound.
- **NFR-2** [I21 (v2 §2), v2 §§4.3, 9.1] The following finite delivery policy applies to core fixtures and enabled operations. A qualified adapter may impose smaller limits; the effective limit is the minimum of these limits, the qualified adapter limit and the remaining admitted envelope. An unknown required native context/envelope limit blocks that operation. A retry or continuation never extends an attempt, authority or stop deadline. Mandatory requirements are never silently dropped to satisfy a size target.

  | Bound | Required behavior |
  |---|---|
  | Operation deadline | 30 seconds from request receipt, further capped by remaining admission/authority deadlines. A queued message does not hold an operation open: enqueue returns its durable disposition within that deadline. A timed-out read is rejected; a potentially submitted native input/effect becomes `uncertain` until reconciled. Timeout is not proof that the writer or effect stopped. |
  | Encoded input | At most 1 MiB per encoded message/manifest and 64 nesting levels, including metadata; reject overflow before persistence/native input. |
  | Delivered context | At most 256 KiB of UTF-8 context bytes per delivery, counting mandatory core and deltas. If the mandatory core cannot fit the effective byte and qualified native context limits, report decomposition/reset required or block with preserved originals. No summary or reference substitutes for mandatory core. |
  | Original references | At most 256 references per manifest; each resolved artifact is at most 1 MiB and their cumulative resolved bytes at most 16 MiB. Bound reads before allocation; refuse an oversized/unavailable mandatory original and report decomposition rather than truncate it. Optional excess references are explicitly rejected, never reported delivered. |
  | Pending queue | At most 64 messages and 4 MiB encoded bytes per recipient session, and 256 messages and 16 MiB per run, whichever is reached first; include `pending` and `queued_for_next_turn` in the totals. Enqueue is atomic: refuse the new message with a queue-full reason and preserve existing entries. No drop-oldest behavior. |
  | Expiry | Default 15 minutes from creation, maximum 24 hours, further capped by explicit message expiry and remaining destination authority deadline. At the earliest bound, undelivered messages become `expired` before native input. Restart does not reset creation/expiry; wall-clock rollback cannot extend the original lifetime. Expiry preserves the ledger receipt and original references; it is not erasure. Existing stop/control intent deadlines and authority checks still bind and are never deferred by this ordinary-message policy. |
  | Retry | At most two additional attempts for reads proven effect-free or an exact-id durable mutation reconciled as not applied, with 100 ms then 500 ms backoff inside the original operation deadline. Native input, resume/start and external effects receive no automatic replay after an uncertain outcome; reconcile first, then require the applicable admission and remaining envelope. Completed exact-id operations return their stored result without another effect. |

- **NFR-3** [I09/I14 (v2 §2), G04/G13 (v2 §18.3)] Each advertised native/platform/surface combination has explicit versioned evidence or is marked unknown/unsupported/experimental. Linux fixtures do not establish Windows/macOS/native-interface qualification.
- **NFR-4** [I07/I20/I23 (v2 §2)] Every decision/evidence/delivery revision is immutable or causally superseded in the existing ledger, with algorithm-qualified content identities and explicit Git object formats as required by v2 §4.2. Restart/replay cannot duplicate a native input/effect, create another task authority, or convert a native completion into independent acceptance.

## Candidate delivery streams for refinement

These are suggested independently useful construction boundaries, not created child specs or authority to implement before their prerequisites.

| Stream | Independently useful outcome | Requirements and dependencies |
|---|---|---|
| A — Grounded handoff core | Validate/preserve original-requirement manifests and inspect durable scoped message/context receipts even when native resume is unavailable. | FR-1/FR-2, FR-6/FR-7; extend existing v2 contracts/ledger. Synthetic contracts may be designed now; production use needs the applicable S1 control/authority/acceptance foundations. |
| B — One qualified native session | Resume a compatible exact session as a separately admitted attempt, with exclusive input and truthful delta acknowledgement or explicit reconstruction. | FR-3/FR-4, same-harness FR-5; consumes A's frozen contracts and one operation-qualified native bundle. Adapter investigations may proceed independently; no provider is a default owner. |
| C — Two-harness continuity journey | Switch only to an authorised independently qualified route, preserve originals, expose context loss and verify the resulting candidate normally. | Remaining FR-5/FR-8; full S1, A, B where supported, and two independent route qualifications. Unsupported resume may use an explicitly qualified reconstruction route; it cannot be relabelled native resume. |

## Dependencies

- **Specification is actionable now**: requirement capture and source/fixture design do not assert that runtime prerequisites passed. No implementation or native trial is authorised by this requirements-only change.
- **S1 and MH-21**: complete applicable G01–G11/G16 (and G12 for any selected alternative surface) for the actual first useful route and exact public journey; MH-21's finalized canonical/supervisor foundations are inputs, not proof that complete S1 passed. `product/objectives.md` stage1 and `docs/validation-plan.md:59` retain the readiness boundary.
- **MH-10, MH-12/MH-4/MH-29**: use registry evidence and independently qualify two native harness bundles before the complete card's two-route claim. At the grounding revision Claude reports resume/steering unsupported and strict billing still checks unresolved managed sources; Codex reports resume/steering unknown, uses a fixture-only app-server proposal and has an unknown-auth-category admission blocker plus required live purchased-credit prevention. Native documentation does not close these source-grounded gates.
- **Protected acceptance**: extend `specs/*/protected-acceptance/` contract/check bindings. A new task contract invalidates affected acceptance; continuation does not reuse a stale verification as current evidence.
- **Concurrent core work**: `specs/*/supervisor-attachment-control/` owns production singleton dispatch, durable operations, projection/grant state and stop delivery; `specs/*/existing-run-client/` and `specs/*/herdr-scoped-projection/` own attach-only views. Reuse their merged contracts; native writable input takeover remains outside this spec.
- **Adjacent later work**: MH-24 consumes continuity/context foundations for the broader retrieval/dependency graph; MH-25 consumes provenance/retention roots for export/erasure. MH-5/MH-17 own routing/profile choices, MH-14 owns parallel construction. This spec must not create a dependency cycle by requiring those whole cards to build its minimum original-evidence handoff.
- **Supersession**: extends finalized `specs/*/v2-contract-vocabulary/`, `specs/*/supervisor-service/`, `specs/*/supervisor-migration/`, `specs/*/supervised-stop-recover/` and native adapter slices. No existing spec or historical issue is silently replaced.

## Impacted components

These are grounded integration areas for refinement; exact interfaces, schema migrations and domain-pure task scopes belong to the later design stage.

- **Core contracts/ledger**: `internal/v2contract/records_coordination.go`, `records_execution.go`; `internal/journal/` and its existing migration set. Extend ContextManifest/Handoff/Message/Attempt provenance and durable projections rather than introduce an external task store.
- **Native operations**: `internal/adapter/adapter.go` and `adapters/{claudecode,codex}/` launch/probe/decode/qualification paths. Negotiate optional native-session operations and explicit acknowledgements; preserve worker-owned process creation and the existing qualified surface.
- **Controller/worker/admission**: `internal/supervisor/recover.go`, `internal/supervisor/ingest.go`, `internal/workers/worker.go`, `internal/admission/`, `internal/qualify/`; retain exact operation/launch generations, writer identity, route qualification and finite resource envelopes.
- **Clients/evidence/docs**: `internal/cli/`, `internal/tui/`, `tests/e2e/`, `docs/user-guide.md`, `docs/limitations.md` and adapter compatibility evidence. Each future task remains one domain; user-visible continuity modes must reflect canonical evidence.

## Open Questions

- **OQ-1 — First operation-qualified native surface and acknowledgement evidence.** Blocks native implementation/advertising for that surface, not core requirement capture. Options: supported exact-session native resume with observable acknowledgement; supported resume with delivery explicitly uncertain; artifact reconstruction with disclosed loss. Conservative default: unknown operation evidence stays blocked, never inferred from CLI help, a session ID or private stores. Refinement records the qualified bundle and how its native observation binds the delivered manifest; maintainers decide scope through the ordinary reviewed spec lifecycle.
- **OQ-2 — Resolved bounded context/delivery policy.** NFR-2 fixes finite defaults, effective smaller-limit composition, overflow/timeout/expiry dispositions and safe retry behavior. Root selected these conservative values under the 3 November project delegation; future qualification may reduce them but cannot silently enlarge admitted envelopes or remove mandatory context. Tests must exercise each boundary and restart/uncertain-effect behavior before enabling the affected operation.
- **OQ-3 — Persisted native-session writer and message projection composition.** Blocks persistence/ownership design, not schema-level fixtures. Extend the canonical supervisor's merged lease/operation contracts and additive migration model; do not add an independent scheduler or weaken process/writer reconciliation. An architecture decision must bind stale generation, crash and unknown-owner behavior before implementation.
- **OQ-4 — Complete S1/two-route evidence bundle.** Blocks complete MH-15 production delivery/qualification until exact current records and end-to-end evidence exist. Accepted historical slices and authored requirements do not answer this question. Preserve the per-stream useful fixture outcomes and disclose their limits while upstream qualification proceeds.

## Definition of Done

- [ ] FR-1–FR-8 and NFR-1–NFR-4 have a refined source-to-criterion map and independently reviewed domain-pure design/tasks; published criterion IDs are retained.
- [ ] Scripted journeys discriminate reconnect, new-attempt native resume and explicit fresh-session reconstruction, with original-constraint and wrong-session controls.
- [ ] Durable message/context states, exact acknowledgement frontiers, scope/freshness refusals, idempotency and interrupted-delivery reconciliation have effect-level evidence.
- [ ] Full S1 and two independent operation-qualified route bundles are demonstrated before the complete two-harness card is advertised; unknown/live/fixture evidence stays distinct.
- [ ] Independent acceptance remains tied to the actual candidate/contract/check revisions; no handoff, native completion or generated view grants acceptance.
- [ ] Docs and receipts retain all unsupported/native-loss/platform/context-budget boundaries, active provenance retention requirements and return-to-native instructions.
