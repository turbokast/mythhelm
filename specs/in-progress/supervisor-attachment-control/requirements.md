# Supervisor attachment control — Requirements

> Normative source: `mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md`, version 2.0. All section, I, AT and G identifiers refer to v2. Source requirements: `specs/*/herdr-bridge/requirements.md`, unchanged archived refinement. Epic: `specs/*/herdr-embedded-client/plan.md`. Source AC identifiers below are retained, including intentional numbering gaps; new FR-6 criteria belong only to the core child. Shared criteria remain binding across streams, with primary implementation and integration ownership in the epic coverage table.

## Context

- **Backlog card**: MH-3
- Source: [issue #24](https://github.com/turbokast/mythhelm/issues/24), refinement merged in `f96cc05`; grounding baseline `61d18e2`.
- This child owns FR-3, FR-4.1/4.2 and the production prerequisite for FR-1. Requirements moved by approved restructuring; no native writable takeover or whole-MH-3/G11 claim.
- **HOLDS**: existing source requirements are still necessary; design.md records current production gaps and exact grounding. **PARTIAL**: completed supervisor specs provide building blocks, not a production lifecycle. **UNVERIFIABLE**: live Herdr lifecycle qualification remains unproven until the separate live suite passes.

## Objectives

- **O1**: Make the accepted singleton own ordinary execution and provide authenticated snapshots and durable asynchronous stop, independently useful to standalone clients.
- **O2**: Preserve all source objectives O1–O4 through the epic coverage matrix; unknown evidence remains unknown.

## Non-Goals (and what still binds)

| ID | Out of scope | Still binding |
|---|---|---|
| N1 | Native writable viewer/takeover, lease handback and automated steering. | I17/I18: refuse writable attachment; never create a second native writer. Full takeover remains a named MH-3 follow-up requiring lifecycle/fidelity/billing qualification under G12. |
| N2 | Host-owned native processes, Herdr agent auto-resume, host live handoff, another scheduler or inference service. | I01/I04/I06/I16/I17: core ownership and pinned admission remain mandatory. |
| N3 | General public plugin ABI, arbitrary host actions and background notification fan-out. | I03/I11: a small allowlist cannot bypass core authority. |
| N4 | Full TUI intervention/accessibility redesign and claiming the whole G11 gate. | I09/I14; MH-23 owns that UX work, while this slice tests its embedded subset and reports remaining AT-36/AT-48 gaps. |
| N5 | Universal host/platform support or new route qualification. | I14/I15: only exact qualified rows are supported; unsupported platforms and native surfaces remain visible and blocked as applicable. |


## Functional Requirements

### FR-3 — Scoped, sanitized, evidence-based projection

- **AC-3.1** [I03/I09/I17/I23, v2 §§3/14] When an operator explicitly attaches a run, the core shall bind the projection to that run, its repository, client identity and current core generation. A bridge request naming another run/repository, widening scope or using stale authority shall return permission/revision failure before names, counts or metadata leave the core. Token-less same-user access shall not be substituted for bridge scope. Scope validation, generation validation and projection data selection shall share one consistent core snapshot; any subsequent revalidation shall cover the complete selected projection. A deterministic competing-authority/generation-change test shall prove that a read cannot combine authorization from one snapshot with data from another or release data selected under a failed validation.
- **AC-3.2** [I07/I09/I17, v2 §§14/15] When core state changes, the projection shall expose opaque run/task identifiers, safe labels, run state, selected harness/model, waiting reason, review state and requested attention, with explicit unknown/unavailable values. Native completion or pane text shall never set a verified/completed acceptance outcome; the projection shall preserve candidate, verified and applied distinctions from core evidence.
- **AC-3.3** [I03/I19, v2 §14] When metadata is projected, an allowlist shall exclude prompts, raw transcripts, private paths, subscription secrets and capability tokens; labels shall remove terminal control sequences. Synthetic sentinel tests shall prove exclusion, including nested payloads and forged host status.
- **AC-3.4** [I09/I23, v2 §§3/14] If the bridge loses connection or core generation changes, then cached status shall be labelled stale until an authenticated fresh read succeeds; replayed host events shall not advance core state or claim a successful action. Each projection shall carry its attachment identity, core generation and a monotonically increasing projection revision for that pair. The client shall discard an older revision delivered after a newer one, treat an identical duplicate as a no-op, and reject conflicting content for the same revision as stale pending a fresh read. A changed attachment/generation shall require authenticated resynchronization before display, and old-generation messages shall be discarded. Tests shall deliver revisions 2 then 1, duplicate 2, conflicting 2 and an old-generation message after resynchronization.


### FR-4 — Trusted navigation and controls

- **AC-4.1** [I03/I07/I17, v2 §§14/15] When the host requests review or stop, the bridge shall navigate to the existing run's trusted MYTHHELM view. Stop shall require a core-authorized typed request with operation ID, expected revision and generation; duplicates shall replay the same durable acknowledgment and expose the same operation result, and stale/foreign requests shall be rejected. Revision/generation comparison, authorization and durable recording of the stop request shall form one atomic operation under the synchronization protecting that mutation. A deterministic competing-update test between request preparation and recording shall produce a revision conflict with no stop recorded or delivered for the stale request. This slice shall not expose host-side apply, publication, approval, provider-switch or launch actions.
- **AC-4.2** [I06/I09, v2 §6.4; AT-07] When the operator confirms stop in the trusted core interface, the embedded client shall display requested/unconfirmed until reconciliation confirms the result. The core shall durably record accepted stop intent before returning its acknowledgment, and shall return accepted/rejected within the NFR-2 deadline independently of worker reconciliation; it shall not await the stopped report to acknowledge acceptance. The eventual reconciled result shall be retained by operation ID and readable through authenticated core status, with the projection showing that result only after reconciliation. A lost acknowledgment shall be recoverable by replay/query of the same operation ID without another stop delivery; loss of the client or bridge shall not cancel accepted reconciliation. If reconciliation is unresolved or times out, the result shall remain explicitly unconfirmed. Tests shall delay worker confirmation beyond 2 seconds and disconnect/reconnect the client, observing timely acknowledgment followed by the same durable operation result. Closing a pane or losing a callback shall not be displayed as confirmed stop.
- **AC-4.3** [I17/I18, v2 §14] When any host action requests native writable input or takeover, the client shall report capability unsupported and send zero input bytes to the native session. A viewer, if exposed, shall be read-only. Unknown input ownership shall block automation; it shall never be resolved by creating another writable terminal.


### FR-6 — Complete the accepted production lifecycle prerequisite

- **AC-6.1** [I02/I05/I06/I18/I23, v2 §§3/5/6] When an ordinary public `mythhelm run` is admitted in a service-owned root, the singleton shall record assignment and execute the existing pipeline with one worker launch; client exit shall leave execution, spool ingestion and deadline enforcement service-owned. Plain, JSONL, accessible and TUI entrypoints shall use this same production path. The demo shall exercise that path without credentials and retain active state on detach.
- **AC-6.2** [I05/I12/I18/I23, v2 §§3/5.2] When an existing root migrates, the existing exclusive root-wide Drain → Backup → Import → Adopt sequence shall fence every legacy writer before the service owns its ledger. A verified live worker may be adopted only with unchanged attempt/launch identity and supported spool schema after legacy owner-lock release; held or ambiguous ownership shall refuse, never steal or spawn. No per-run enrollment exception is permitted.
- **AC-6.3** [I06/I12/I18/I21, v2 §§3/6] When the singleton restarts, it shall reconcile every active assignment, committed dispatch and stop intent before enabling mutation for that run, preserve operation acknowledgments/results, and resume the same pipeline ingestion/conclusion owner. It shall not infer permission to launch a replacement or extend a worker deadline. Invalid identity/generation evidence shall quarantine and block automation.
- **AC-6.4** [I02/I03/I04/I15/I19, v2 §§6/7/8] When service admission receives a request, it shall validate a closed typed schema and revalidate repository/task/configuration/route/confirmation bindings before durable admission; callbacks and the full caller environment shall never cross the wire. Changed facts invalidate prior confirmation and no native launch occurs until applicable authority is established.
- **AC-6.5** [I03/I08/I12/I18, v2 §§6/11/15] When trusted standalone CLI/TUI requests stop, recover or apply in an adopted root, it shall use the singleton's existing owner and durable operation contract; a local legacy writer shall not acquire authority there. Apply remains explicit, bound to exact candidate/ref and unavailable through attachment/host capability. Recovery never starts an unapproved continuation.

### FR-7 — Bound service admission and nonblocking observation

- **AC-7.1** [I02 (v2 §2), I03 (v2 §2), I20 (v2 §2), v2 §2, v2 §6, v2 §8] When a service preparation is encoded, the core shall use the closed version-1 DTO, canonical bytes/hash and per-field persist/rederive/reject inventory of ADR 0022 at `d8975b35b342bc7f5be34c43bcdbcc39e64ecf52`; missing, unknown, duplicate, malformed, inconsistent or changed execution inputs shall refuse before native launch. `TestBindingV1CanonicalInputs` and `TestServiceAdmissionBindingAllInputs` shall include controls dropping one binding at a time.
- **AC-7.2** [I02 (v2 §2), I12 (v2 §2), I18 (v2 §2), I21 (v2 §2), I23 (v2 §2), v2 §3, v2 §6, v2 §8] When Start accepts a confirmed preparation, the core shall commit one run/admission/reservation/assignment/pending intent and immutable acknowledgement; after successful snapshot it shall arm one attempt/launch intent/pin/outbox with execution accounting in a second transaction. Failed transactions shall leave no partial effects; restart with missing live launch context or ambiguous spawn shall never relaunch. `TestDispatchTwoTransactionOrder` and `TestDispatchCrashWindows` shall prove those boundaries.
- **AC-7.3** [I03 (v2 §2), I19 (v2 §2), I20 (v2 §2), v2 §6, v2 §8] If a retained binding field lacks an exact versioned retention classification and applicable operator approval, then preparation shall remain unavailable and write no binding, grant or native state; full environments and native subscription secrets shall be excluded regardless of generic consent. `TestRetentionUnclassifiedRefuses` and `TestRetentionBeforePersistence` shall pair rejected email/path/argv/challenge sentinels with an explicitly approved synthetic control.
- **AC-7.4** [I06 (v2 §2), I09 (v2 §2), I18 (v2 §2), I21 (v2 §2), I23 (v2 §2), v2 §3, v2 §6, v2 §14] When a service observer stalls, disconnects or overflows hints, the owner shall continue canonical ingestion and preserve durable outcome/stop evidence; only the view shall close, and a callback shall never synchronously close its own owner. `TestObserverDeliveryNeverWaitsForClient` and `TestObserverCloseOrdering` shall fail with a blocking delivery or callback-self-close control.
- **AC-7.5** [I09 (v2 §2), I21 (v2 §2), I23 (v2 §2), v2 §3, v2 §14] When Observe pages events, the service shall enforce 128 events/1 MiB/1000 ms requested wait, 64 total slots/16 per peer/8 active read handlers, one pending observation per peer/run connection and a 16-code best-effort hint ring. An oversized event shall return protocol_mismatch without cursor advance; client cancellation shall release view resources without stopping or releasing the execution owner. `TestObserverCapacityAndCursor` shall assert nonempty success and each overflow/failure control.

- **AC-7.6** [I02 (v2 §2), I20 (v2 §2), I23 (v2 §2), v2 §3, v2 §6, v2 §8] When preparation or confirmation returns, revision-aware Go clients shall expose the validated outer Result.Revision without changing schema2 PreparedRun; subsequent confirmations and Start shall use that server-reported precondition. Same-operation retries shall replay the exact acknowledgement, while stale concurrent requests shall mutate nothing and disclose current metadata only under authenticated same-binding scope. `TestPreparedClientRevisionedRoundTrip` and `TestPreparedClientLostAckAndConflict` shall fail body-only/default0/local-counter fixtures.
- **AC-7.7** [I02 (v2 §2), I03 (v2 §2), I19 (v2 §2), I20 (v2 §2), v2 §6, v2 §8] When retention is confirmed, the service shall derive field-set metadata internally from the exact canonical binding and complete immutable policy whose verified content digest the binding commits; existing challenge/binding confirmation fields shall suffice, with no imagined policy/field-set wire member. Missing/tampered policy or a bare hash shall refuse, and a deliberate hash-cycle/independent-value fixture shall fail `TestRetentionFieldSetIsInternalBindingCommitment`.

## Non-Functional Requirements

- **NFR-1** [I13, v2 §18] All ordinary contributor tests shall run with no Herdr installation, native account or network dependency using synthetic host schemas and an instrumented fake worker; live host checks shall be an explicitly separate suite.
- **NFR-2** [I06/I21, v2 §§3/14] Bridge operations shall have a 2-second deadline, a maximum 64 KiB message and a queue of at most 16 projections per attachment; overflow shall coalesce replaceable status, flag stale state and never block worker/control processing. While a test stalls the host for 10 seconds, an authenticated core status read and a permitted control-request acknowledgment shall each complete within 2 seconds. Acknowledgment means accepted/rejected request, not reconciled stop completion, which remains governed by AC-4.2. A deliberately blocking bridge fixture shall exceed this bound and fail the responsiveness assertion.
- **NFR-3** [I03/I11, v2 §§3/14] Host communication shall remain local, use the installed host's authenticated/restricted transport where established, and never add a TCP listener or copy native credentials. Unknown transport trust shall disable integration.


## Dependencies

ADR 0014 and ADR 0016 migration remain binding; ADR 0019 protects acceptance evidence. Rebase lifecycle extraction on the merged MH-23 lifecycle contract and Windows claims on the proven ADR 0017 process boundary. No future PR contract is treated as shipped.

## Definition of Done

- [ ] FR-6 ordinary production launch, migration, recovery, apply and restart tests pass; an enrolled-only fixture does not satisfy FR-1.
- [ ] FR-7 codec, retention, two-transaction dispatch and observer lifecycle tests pass with their violating controls; both ADR/follow-on merges and exact Task22 handoff remain required before runtime.
- [ ] Every owned AC and shared integration obligation has the named executable evidence in tasks.md; no helper-only public CLI claim.
- [ ] NFR-1–3 and deliberately violating fixtures pass; live evidence is separate and exact.
- [ ] Required Go/harness gates and independent code/architecture reviews pass.
- [ ] No source objective/non-goal/AC is dropped; epic coverage table reconciles all requirements.
- [ ] Changed public/persistence/process contracts have a reviewed ADR before implementation.

## Open Questions

See scratchpad.md for conservative defaults and blocking owners. Unqualified optional host integration is disabled; that never waives public manual attach or the production service prerequisite.
