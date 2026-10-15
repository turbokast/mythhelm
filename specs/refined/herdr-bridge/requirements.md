# Herdr embedded client and scoped bridge — Requirements

> Refined. Ready for /spec herdr-bridge; design and implementation approval remain separate.
>
> First slice of MH-3: attach an embedded MYTHHELM client to existing work, publish a minimal scoped status projection and navigate to trusted core controls. Writable native input takeover is a separately qualified follow-up, explicitly refused here. This slice alone does not complete MH-3 or establish G11. Normative source: `mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md`, version 2.0, §§3, 6, 14–15 and 18; all I/AT/G references below use that revision.

## Context

- **Backlog card**: MH-3
- **Source**: [issue #24](https://github.com/turbokast/mythhelm/issues/24), approved card source v2 §§3, 6, 14–15 and Implementation Plan W05. The issue's numeric sections describe historical Revision 1.1; they are not v2 section numbers.
- **Grounding baseline**: `30062ec` on `origin/main`.

An operator needs an existing run to remain the same run when its Herdr pane disconnects or is restored. Host status must be a scoped presentation of core evidence, and a request from the host must never become permission merely because it originates in the focused pane. The first slice supplies that attachment and projection path without adding another scheduler, process owner, native transport or billing route.

The completed supervisor specs provide building blocks, not evidence that every production CLI path uses the service. The design must close the concrete attachment/control integration gaps below before advertising the embedded path. Successful standalone operation inside a terminal is not proof of Herdr restart safety.

| Claim | Verdict and evidence |
|---|---|
| The card describes a missing Herdr bridge. | HOLDS: `internal/admission/admission.go:604` (`rg -n 'func checkCapabilityFlags' internal/admission/admission.go`) rejects `HostHerdr` with `host_unavailable`; `internal/supervisor/pipeline_test.go:612` (`rg -n TestHostHerdrExit7 internal/supervisor/pipeline_test.go`) exercises the refusal before recording a run. |
| A run can already be attached through a dedicated CLI command. | PARTIAL: review exists, but enumeration of the complete `commands` map at `internal/cli/dispatch.go:31` (`rg -n 'var commands' internal/cli/dispatch.go`) contains run/runs/review/stop/recover/apply/version/demo/doctor/supervisor/migrate and no attach entry. This is a requested capability, not a present one. |
| One durable core lifecycle is ready for a host client. | PARTIAL: `internal/control/client.go:168` (`rg -n 'func RunSupervisor' internal/control/client.go`) acquires the instance lock and serves `NewSupervisorServer`; that registry at `internal/control/server.go:178` contains status/assign/reserve/release/heartbeat/read. Separate stop/recover registration functions exist at `server.go:160`, while `internal/cli/tui.go:153` (`rg -n 'func wireActions|func runLiveTUI' internal/cli/tui.go`) wires legacy supervisor actions and a pipeline goroutine. Attachment must not assume these are a connected production control service. |
| Scoped reads can be reused without more authorization work. | PARTIAL: `internal/control/filter.go:42` (`rg -n 'func ReadHandler' internal/control/filter.go`) bounds attempt-token reads to the ledger repository, but token-less same-user callers choose their repository list. The bridge needs core-issued attachment scope; host-supplied repository strings cannot be that authority. |
| A schema-discoverable Herdr surface exists. | HOLDS for the installed CLI only: read-only `herdr --version` returned `herdr 0.9.1`; `herdr api schema --json` returned schema version 1, protocol 22, request/response/event schemas. This is not a running-server compatibility or lifecycle result. The [socket API documentation](https://herdr.dev/docs/socket-api/) describes bundled schema discovery and optional pane metadata reporting; the [restore documentation](https://herdr.dev/docs/session-state/) distinguishes restoration from surviving execution. Both inspected 2026-10-15; implementation must probe the actual host. |
| Restoring an aggregate pane preserves execution and one input writer today. | UNVERIFIABLE: no live Herdr lifecycle test was run. AT-10 and the eventual input takeover qualification remain required, with no inferred success from a restored layout. |
| Acceptance can fail against today's build. | HOLDS: the new attachment journey cannot pass the enumerated CLI registry; explicit Herdr admission is rejected as above. Synthetic host and fake worker tests can exercise this without credentials or inference spend. |

## Objectives

- **O1**: Reopen one existing run in a Herdr pane without another native launch, cancellation or process-owner transfer.
- **O2**: Show safe status and attention only for the explicitly attached run and its repository.
- **O3**: Reach trusted MYTHHELM review and stop controls without giving pane state approval authority.
- **O4**: Publish exact tested host capabilities and unsupported cases so this slice cannot be mistaken for complete MH-3 delivery.

## Non-Goals (and what still binds)

| ID | Out of scope | Still binding |
|---|---|---|
| N1 | Native writable viewer/takeover, lease handback and automated steering. | I17/I18: refuse writable attachment; never create a second native writer. Full takeover remains a named MH-3 follow-up requiring lifecycle/fidelity/billing qualification under G12. |
| N2 | Host-owned native processes, Herdr agent auto-resume, host live handoff, another scheduler or inference service. | I01/I04/I06/I16/I17: core ownership and pinned admission remain mandatory. |
| N3 | General public plugin ABI, arbitrary host actions and background notification fan-out. | I03/I11: a small allowlist cannot bypass core authority. |
| N4 | Full TUI intervention/accessibility redesign and claiming the whole G11 gate. | I09/I14; MH-23 owns that UX work, while this slice tests its embedded subset and reports remaining AT-36/AT-48 gaps. |
| N5 | Universal host/platform support or new route qualification. | I14/I15: only exact qualified rows are supported; unsupported platforms and native surfaces remain visible and blocked as applicable. |

## Functional Requirements

### FR-1 — Attach without lifecycle effects

- **AC-1.1** [I06/I12/I17/I18, v2 §§3/6/14; AT-10] When the user attaches by an existing run ID, the client shall read the same run and attempt identities from the canonical core, without launching a native process, invoking recovery, or changing the run's owner. A fake worker launch counter and ledger identities shall prove both the initial attach and repeated attach behavior.
- **AC-1.2** [I02/I03/I23, v2 §§3/14] If the run is unknown, its ownership is unresolved, its state root conflicts, or the client cannot authenticate, then attachment shall fail with an explicit reason and zero native launches; it shall not silently create a run or switch roots.
- **AC-1.3** [I06/I17, v2 §14; AT-10] When the pane detaches, reconnects, closes or is restored, the native worker shall receive zero stop/start requests from those presentation events. Deliberate stop remains the separate core action in FR-4. Tests shall include pane-process teardown separately from socket disconnect.
- **AC-1.4** [I12/I17, v2 §14; AT-10] If the installed host cannot register a tested attach-only restoration command, then the client shall retain and document the working `mythhelm attach <run-id>` command as manual fallback and shall not register a raw native resume or a new-run command. An end-to-end test shall invoke that command with host registration unavailable and observe the existing run without another native launch.

- **AC-1.5** [I06/I12/I17, v2 §§14/15; AT-10] When an operator invokes `mythhelm attach <run-id>` through the public CLI, the command shall open the existing-run client, expose its usage through CLI help, and satisfy AC-1.1–AC-1.4. End-to-end tests shall invoke the public command for initial attachment, repeated attachment and the manual fallback; direct calls to internal attachment helpers alone shall not establish this criterion.

### FR-2 — Negotiate and fail safely

- **AC-2.1** [I11/I14, v2 §14] When bridge integration is enabled, the bridge shall record installed CLI version/schema identity and live host compatibility evidence, and use only explicitly recognized methods whose request/response shape matches the negotiated surface. A bundled schema alone shall never mark the running host compatible.
- **AC-2.2** [I06/I09/I14, v2 §14; AT-10] If the socket is missing, access is denied, the schema is obsolete or unknown, or probing times out, then the client shall expose the reason, disable the unsupported bridge feature and retain the embedded client and manual attach path without changing run state or admission.
- **AC-2.3** [I03/I11/I17, v2 §14] When bridge metadata or a host callback is malformed, oversized or unsupported, the bridge shall reject it without invoking native tools, changing authority or substituting a different execution surface.

### FR-3 — Scoped, sanitized, evidence-based projection

- **AC-3.1** [I03/I09/I17/I23, v2 §§3/14] When an operator explicitly attaches a run, the core shall bind the projection to that run, its repository, client identity and current core generation. A bridge request naming another run/repository, widening scope or using stale authority shall return permission/revision failure before names, counts or metadata leave the core. Token-less same-user access shall not be substituted for bridge scope. Scope validation, generation validation and projection data selection shall share one consistent core snapshot; any subsequent revalidation shall cover the complete selected projection. A deterministic competing-authority/generation-change test shall prove that a read cannot combine authorization from one snapshot with data from another or release data selected under a failed validation.
- **AC-3.2** [I07/I09/I17, v2 §§14/15] When core state changes, the projection shall expose opaque run/task identifiers, safe labels, run state, selected harness/model, waiting reason, review state and requested attention, with explicit unknown/unavailable values. Native completion or pane text shall never set a verified/completed acceptance outcome; the projection shall preserve candidate, verified and applied distinctions from core evidence.
- **AC-3.3** [I03/I19, v2 §14] When metadata is projected, an allowlist shall exclude prompts, raw transcripts, private paths, subscription secrets and capability tokens; labels shall remove terminal control sequences. Synthetic sentinel tests shall prove exclusion, including nested payloads and forged host status.
- **AC-3.4** [I09/I23, v2 §§3/14] If the bridge loses connection or core generation changes, then cached status shall be labelled stale until an authenticated fresh read succeeds; replayed host events shall not advance core state or claim a successful action. Each projection shall carry its attachment identity, core generation and a monotonically increasing projection revision for that pair. The client shall discard an older revision delivered after a newer one, treat an identical duplicate as a no-op, and reject conflicting content for the same revision as stale pending a fresh read. A changed attachment/generation shall require authenticated resynchronization before display, and old-generation messages shall be discarded. Tests shall deliver revisions 2 then 1, duplicate 2, conflicting 2 and an old-generation message after resynchronization.

### FR-4 — Trusted navigation and controls

- **AC-4.1** [I03/I07/I17, v2 §§14/15] When the host requests review or stop, the bridge shall navigate to the existing run's trusted MYTHHELM view. Stop shall require a core-authorized typed request with operation ID, expected revision and generation; duplicates shall replay the same durable acknowledgment and expose the same operation result, and stale/foreign requests shall be rejected. Revision/generation comparison, authorization and durable recording of the stop request shall form one atomic operation under the synchronization protecting that mutation. A deterministic competing-update test between request preparation and recording shall produce a revision conflict with no stop recorded or delivered for the stale request. This slice shall not expose host-side apply, publication, approval, provider-switch or launch actions.
- **AC-4.2** [I06/I09, v2 §6.4; AT-07] When the operator confirms stop in the trusted core interface, the embedded client shall display requested/unconfirmed until reconciliation confirms the result. The core shall durably record accepted stop intent before returning its acknowledgment, and shall return accepted/rejected within the NFR-2 deadline independently of worker reconciliation; it shall not await the stopped report to acknowledge acceptance. The eventual reconciled result shall be retained by operation ID and readable through authenticated core status, with the projection showing that result only after reconciliation. A lost acknowledgment shall be recoverable by replay/query of the same operation ID without another stop delivery; loss of the client or bridge shall not cancel accepted reconciliation. If reconciliation is unresolved or times out, the result shall remain explicitly unconfirmed. Tests shall delay worker confirmation beyond 2 seconds and disconnect/reconnect the client, observing timely acknowledgment followed by the same durable operation result. Closing a pane or losing a callback shall not be displayed as confirmed stop.
- **AC-4.3** [I17/I18, v2 §14] When any host action requests native writable input or takeover, the client shall report capability unsupported and send zero input bytes to the native session. A viewer, if exposed, shall be read-only. Unknown input ownership shall block automation; it shall never be resolved by creating another writable terminal.

### FR-5 — Honest qualification and documentation

- **AC-5.1** [I13/I14, v2 §§17/18; G11/AT-10] When this slice is delivered, an evidence matrix shall separate fixture tests from live host results and identify exact host version/protocol, OS/architecture, native surface, and tested lifecycle events. Untested rows shall say untested or unsupported, never passed.
- **AC-5.2** [I06/I14/I17, v2 §14; G11/AT-10] Before an embedded host row is advertised as supported, live tests with an instrumented fake native executable shall demonstrate detach, reconnect, pane teardown, host restart, duplicate restore, missing socket and obsolete schema with exactly one attempt and no presentation-caused start/stop. A failed row shall remain experimental or unsupported with the failure recorded.
- **AC-5.3** [I09/I14, v2 §18.3; G11/AT-36/AT-48] When docs report this slice, they shall name its tested journey, accessibility evidence and limitations, retain the open writable-takeover and complete-journey tracks, and state that MH-3/G11 are incomplete until those tracks pass. Host metadata shall not claim a billing or native qualification upgrade.

## Non-Functional Requirements

- **NFR-1** [I13, v2 §18] All ordinary contributor tests shall run with no Herdr installation, native account or network dependency using synthetic host schemas and an instrumented fake worker; live host checks shall be an explicitly separate suite.
- **NFR-2** [I06/I21, v2 §§3/14] Bridge operations shall have a 2-second deadline, a maximum 64 KiB message and a queue of at most 16 projections per attachment; overflow shall coalesce replaceable status, flag stale state and never block worker/control processing. While a test stalls the host for 10 seconds, an authenticated core status read and a permitted control-request acknowledgment shall each complete within 2 seconds. Acknowledgment means accepted/rejected request, not reconciled stop completion, which remains governed by AC-4.2. A deliberately blocking bridge fixture shall exceed this bound and fail the responsiveness assertion.
- **NFR-3** [I03/I11, v2 §§3/14] Host communication shall remain local, use the installed host's authenticated/restricted transport where established, and never add a TCP listener or copy native credentials. Unknown transport trust shall disable integration.

## Dependencies

- MH-21's `specs/*/v2-contract-vocabulary/`, `supervisor-service/`, `supervisor-migration/` and `supervised-stop-recover/` are completed on the baseline; inspect their actual production wiring, not just the card status. `docs/decisions/0014-service-topology.md` owns the topology decision. No second lifecycle service is permitted.
- MH-22's `specs/*/protected-acceptance/` is in `done/` on the baseline, although its card remains `specced`. Its protected evidence semantics and `docs/decisions/0019-protected-acceptance.md` bind projected outcomes. This spec does not itself repair PM state.
- `specs/*/tui-observatory-view/` (MH-23) owns exit/detach UX and touches `internal/cli/tui.go`, `internal/supervisor/pipeline.go`, workers and TUI. Rebase attachment work onto its lifecycle contract; never restore the legacy double-interrupt detach behavior through this bridge. Complete G11 journeys depend on later MH-23 slices.
- `specs/*/windows-process-ownership/` overlaps worker identity and teardown. Windows Herdr lifecycle claims wait for its proven process boundary; this slice can progress with Linux fixture/live evidence.
- `specs/*/contained-execution-profiles/`, `claude-strict-subscription/` and `codex-native-adapter/` supply existing admission/profile/route constraints; host integration cannot relax them. No alternative interactive surface inherits their qualification.
- No existing Herdr spec is superseded. Full native input takeover is an explicit later MH-3 slice, requiring owner lease acquisition/revocation/handback reconciliation and separately qualified native transport. Do not mark MH-3 shipped with this slice alone.

## Impacted components

- Core: `internal/control/` authenticated reads, request revision/generation validation and production dispatch; `internal/cli/dispatch.go`, `tui.go`, `review.go` for attachment and trusted navigation; `internal/admission/admission.go` for capability gating; `internal/supervisor/receipt.go` for truthful host reporting. Existing anchors appear in Context.
- Host adapter: proposed `hosts/herdr/`, owned by the adapters domain, for installed-schema negotiation and the small metadata/action mapping. It shall consume an exact core contract, not import a second orchestration model.
- TUI: `internal/tui/` for existing-run presentation and stale/unsupported state, coordinated with MH-23.
- Docs: `docs/user-guide.md`, `docs/limitations.md` and a versioned host evidence matrix. Task decomposition must separate core, adapters, TUI and docs domains and obtain architecture review for authority/process boundaries.

## Definition of Done

- [ ] FR-1: the public attach command and manual fallback are exercised end to end; all presentation lifecycle events retain identities and native launch count; unknown/foreign targets fail explicitly.
- [ ] FR-2: missing/obsolete/malformed/slow host fixtures preserve embedded fallback without new authority or execution.
- [ ] FR-3: cross-scope, competing-authority, out-of-order/duplicate, stale, secret-sentinel and forged-completion fixtures fail; valid projections preserve core evidence distinctions.
- [ ] FR-4: trusted review/stop works against production wiring; atomic competing-update rejection, durable asynchronous acknowledgment, replay and reconnect/result reconciliation are checked; writable input is explicitly refused.
- [ ] FR-5: exact fixture/live matrix, user journey and limitations are reviewed; no whole-MH-3/G11 claim is made.
- [ ] NFR-1–3: credential-free contributor tests and bounded hostile-host tests pass. Every negative assertion includes a deliberately violating fixture that proves the assertion detects the forbidden effect.
- [ ] Shared Go/harness gates appropriate to changed domains and independent code/architecture reviews pass; any changed public contract or process/persistence decision has a reviewed ADR before implementation.

## Open Questions

- **OQ-1 — Live host capability evidence**: installed schema discovery is known, but the running server/version, safe metadata method subset and restore registration semantics have not been qualified. Design investigation chooses the smallest demonstrably supported mapping; default is embedded/manual attach with bridge features disabled. Blocks advertising optional integration, not fixture implementation.
- **OQ-2 — Production lifecycle seam**: service registry and CLI still expose distinct migration paths at the cited baseline. Design must trace live owners, ingestion, stop and recover end to end and supply an exact attachment/control contract under the existing topology; if this requires changing the accepted ownership policy, stop for an ADR/maintainer decision. Default is refuse unsupported attachment/control, never substitute legacy mutating callbacks or start another service. Blocks implementation task readiness until design investigation resolves it.
