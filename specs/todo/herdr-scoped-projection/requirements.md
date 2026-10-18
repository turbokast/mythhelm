## Herdr scoped projection — Requirements

> Normative source: `mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md`, version 2.0. All section, I, AT and G identifiers refer to v2. Source requirements: `specs/*/herdr-bridge/requirements.md`, unchanged archived refinement. Epic: `specs/*/herdr-embedded-client/plan.md`. Source AC identifiers below are retained, including intentional numbering gaps; new FR-6 criteria belong only to the core child. Shared criteria remain binding across streams, with primary implementation and integration ownership in the epic coverage table.

## Context

- **Backlog card**: MH-3
- Source: [issue #24](https://github.com/turbokast/mythhelm/issues/24), refinement merged in `f96cc05`; grounding baseline `61d18e2`.
- This child owns FR-2 and FR-5, integrating FR-1/3/4 unchanged. Requirements moved by approved restructuring; no native writable takeover or whole-MH-3/G11 claim.
- **HOLDS**: existing source requirements are still necessary; design.md records current production gaps and exact grounding. **PARTIAL**: completed supervisor specs provide building blocks, not a production lifecycle. **UNVERIFIABLE**: live Herdr lifecycle qualification remains unproven until the separate live suite passes.

## Objectives

- **O1**: Publish safe scoped status through exactly qualified optional local host capabilities without making the host necessary to execution.
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

### FR-2 — Negotiate and fail safely

- **AC-2.1** [I11/I14, v2 §14] When bridge integration is enabled, the bridge shall record installed CLI version/schema identity and live host compatibility evidence, and use only explicitly recognized methods whose request/response shape matches the negotiated surface. A bundled schema alone shall never mark the running host compatible.
- **AC-2.2** [I06/I09/I14, v2 §14; AT-10] If the socket is missing, access is denied, the schema is obsolete or unknown, or probing times out, then the client shall expose the reason, disable the unsupported bridge feature and retain the embedded client and manual attach path without changing run state or admission.
- **AC-2.3** [I03/I11/I17, v2 §14] When bridge metadata or a host callback is malformed, oversized or unsupported, the bridge shall reject it without invoking native tools, changing authority or substituting a different execution surface.


### FR-5 — Honest qualification and documentation

- **AC-5.1** [I13/I14, v2 §§17/18; G11/AT-10] When this slice is delivered, an evidence matrix shall separate fixture tests from live host results and identify exact host version/protocol, OS/architecture, native surface, and tested lifecycle events. Untested rows shall say untested or unsupported, never passed.
- **AC-5.2** [I06/I14/I17, v2 §14; G11/AT-10] Before an embedded host row is advertised as supported, live tests with an instrumented fake native executable shall demonstrate detach, reconnect, pane teardown, host restart, duplicate restore, missing socket and obsolete schema with exactly one attempt and no presentation-caused start/stop. A failed row shall remain experimental or unsupported with the failure recorded.
- **AC-5.3** [I09/I14, v2 §18.3; G11/AT-36/AT-48] When docs report this slice, they shall name its tested journey, accessibility evidence and limitations, retain the open writable-takeover and complete-journey tracks, and state that MH-3/G11 are incomplete until those tracks pass. Host metadata shall not claim a billing or native qualification upgrade.


FR-3 and FR-4 are normative dependencies from `specs/*/supervisor-attachment-control/requirements.md`. The bridge may only consume the safe projection and navigate to trusted controls. FR-1 public/manual attachment is a normative dependency from `specs/*/existing-run-client/requirements.md`.

## Non-Functional Requirements

- **NFR-1** [I13, v2 §18] All ordinary contributor tests shall run with no Herdr installation, native account or network dependency using synthetic host schemas and an instrumented fake worker; live host checks shall be an explicitly separate suite.
- **NFR-2** [I06/I21, v2 §§3/14] Bridge operations shall have a 2-second deadline, a maximum 64 KiB message and a queue of at most 16 projections per attachment; overflow shall coalesce replaceable status, flag stale state and never block worker/control processing. While a test stalls the host for 10 seconds, an authenticated core status read and a permitted control-request acknowledgment shall each complete within 2 seconds. Acknowledgment means accepted/rejected request, not reconciled stop completion, which remains governed by AC-4.2. A deliberately blocking bridge fixture shall exceed this bound and fail the responsiveness assertion.
- **NFR-3** [I03/I11, v2 §§3/14] Host communication shall remain local, use the installed host's authenticated/restricted transport where established, and never add a TCP listener or copy native credentials. Unknown transport trust shall disable integration.


## Dependencies

`specs/*/supervisor-attachment-control/` must finish before implementation consumes its stable contract.
`specs/*/existing-run-client/` supplies public attach and trusted navigation before bridge integration.

## Definition of Done

- [ ] Every owned AC and shared integration obligation has the named executable evidence in tasks.md; no helper-only public CLI claim.
- [ ] NFR-1–3 and deliberately violating fixtures pass; live evidence is separate and exact.
- [ ] Required Go/harness gates and independent code/architecture reviews pass.
- [ ] No source objective/non-goal/AC is dropped; epic coverage table reconciles all requirements.
- [ ] Changed public/persistence/process contracts have a reviewed ADR before implementation.

## Open Questions

See scratchpad.md for conservative defaults and blocking owners. Unqualified optional host integration is disabled; that never waives public manual attach or the production service prerequisite.
