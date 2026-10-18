# Existing run client — Requirements

> Normative source: `mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md`, version 2.0. All section, I, AT and G identifiers refer to v2. Source requirements: `specs/*/herdr-bridge/requirements.md`, unchanged archived refinement. Epic: `specs/*/herdr-embedded-client/plan.md`. Source AC identifiers below are retained, including intentional numbering gaps; new FR-6 criteria belong only to the core child. Shared criteria remain binding across streams, with primary implementation and integration ownership in the epic coverage table.

## Context

- **Backlog card**: MH-3
- Source: [issue #24](https://github.com/turbokast/mythhelm/issues/24), refinement merged in `f96cc05`; grounding baseline `61d18e2`.
- This child owns FR-1 and FR-4.3, with core FR-3/4 consumed unchanged. Requirements moved by approved restructuring; no native writable takeover or whole-MH-3/G11 claim.
- **HOLDS**: existing source requirements are still necessary; design.md records current production gaps and exact grounding. **PARTIAL**: completed supervisor specs provide building blocks, not a production lifecycle. **UNVERIFIABLE**: live Herdr lifecycle qualification remains unproven until the separate live suite passes.

## Objectives

- **O1**: Open existing work through the public attach command with trusted review/stop and presentation-only lifecycle.
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

### FR-1 — Attach without lifecycle effects

- **AC-1.1** [I06/I12/I17/I18, v2 §§3/6/14; AT-10] When the user attaches by an existing run ID, the client shall read the same run and attempt identities from the canonical core, without launching a native process, invoking recovery, or changing the run's owner. A fake worker launch counter and ledger identities shall prove both the initial attach and repeated attach behavior.
- **AC-1.2** [I02/I03/I23, v2 §§3/14] If the run is unknown, its ownership is unresolved, its state root conflicts, or the client cannot authenticate, then attachment shall fail with an explicit reason and zero native launches; it shall not silently create a run or switch roots.
- **AC-1.3** [I06/I17, v2 §14; AT-10] When the pane detaches, reconnects, closes or is restored, the native worker shall receive zero stop/start requests from those presentation events. Deliberate stop remains the separate core action in FR-4. Tests shall include pane-process teardown separately from socket disconnect.
- **AC-1.4** [I12/I17, v2 §14; AT-10] If the installed host cannot register a tested attach-only restoration command, then the client shall retain and document the working `mythhelm attach <run-id>` command as manual fallback and shall not register a raw native resume or a new-run command. An end-to-end test shall invoke that command with host registration unavailable and observe the existing run without another native launch.

- **AC-1.5** [I06/I12/I17, v2 §§14/15; AT-10] When an operator invokes `mythhelm attach <run-id>` through the public CLI, the command shall open the existing-run client, expose its usage through CLI help, and satisfy AC-1.1–AC-1.4. End-to-end tests shall invoke the public command for initial attachment, repeated attachment and the manual fallback; direct calls to internal attachment helpers alone shall not establish this criterion.


### FR-4 — Trusted navigation and controls (shared source contract)

- **AC-4.3** [I17/I18, v2 §14] When any host action requests native writable input or takeover, the client shall report capability unsupported and send zero input bytes to the native session. A viewer, if exposed, shall be read-only. Unknown input ownership shall block automation; it shall never be resolved by creating another writable terminal.

AC-4.1 and AC-4.2 are normative dependencies from `specs/*/supervisor-attachment-control/requirements.md`; this client must exercise their typed request, durable replay/query and honest reconciliation display. FR-3 is likewise a normative dependency, including monotonic revision handling.

## Non-Functional Requirements

- **NFR-1** [I13, v2 §18] All ordinary contributor tests shall run with no Herdr installation, native account or network dependency using synthetic host schemas and an instrumented fake worker; live host checks shall be an explicitly separate suite.
- **NFR-2** [I06/I21, v2 §§3/14] Bridge operations shall have a 2-second deadline, a maximum 64 KiB message and a queue of at most 16 projections per attachment; overflow shall coalesce replaceable status, flag stale state and never block worker/control processing. While a test stalls the host for 10 seconds, an authenticated core status read and a permitted control-request acknowledgment shall each complete within 2 seconds. Acknowledgment means accepted/rejected request, not reconciled stop completion, which remains governed by AC-4.2. A deliberately blocking bridge fixture shall exceed this bound and fail the responsiveness assertion.
- **NFR-3** [I03/I11, v2 §§3/14] Host communication shall remain local, use the installed host's authenticated/restricted transport where established, and never add a TCP listener or copy native credentials. Unknown transport trust shall disable integration.


## Dependencies

`specs/*/supervisor-attachment-control/` must finish before implementation consumes its stable contract.

## Definition of Done

- [ ] Every owned AC and shared integration obligation has the named executable evidence in tasks.md; no helper-only public CLI claim.
- [ ] NFR-1–3 and deliberately violating fixtures pass; live evidence is separate and exact.
- [ ] Required Go/harness gates and independent code/architecture reviews pass.
- [ ] No source objective/non-goal/AC is dropped; epic coverage table reconciles all requirements.
- [ ] Changed public/persistence/process contracts have a reviewed ADR before implementation.

## Open Questions

See scratchpad.md for conservative defaults and blocking owners. Unqualified optional host integration is disabled; that never waives public manual attach or the production service prerequisite.
