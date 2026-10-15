# Herdr scoped projection — Design

## 1. Current state (v2 §§14–15)

Grounding baseline `61d18e2`. `internal/admission/admission.go:604` (`rg -n 'func checkCapabilityFlags' internal/admission/admission.go`) rejects Herdr launch admission; it must continue doing so because this spec embeds existing work only. The complete command map in `internal/cli/dispatch.go:31` (`rg -n 'var commands' internal/cli/dispatch.go`) lacks attach at baseline; the existing-run-client prerequisite supplies it. `rg --files hosts` reports no hosts directory at this baseline: Task1 creates `hosts/herdr/` and its synthetic testdata, not an assumed existing adapter.

Read-only installed probes recorded `herdr 0.9.1`; `herdr api schema --json` reports schema_version1/protocol22. Opened schema paths: request.oneOf contains `ping` and `pane.report_metadata`; request.$defs.PingParams is an object; PaneReportMetadataParams requires source/pane_id and permits seq, ttl_ms and at most16 token keys. SuccessResponse requires id/result; ResponseResult includes pong{version,protocol,capabilities?} and ok. This is HOLDS installed shape evidence, not proof of a running server or method-specific success response. Live transport trust, method response correlation and lifecycle remain UNVERIFIABLE until Task7. No session snapshot, credentials, settings mutation or live metadata was read/written during investigation.

The published [socket API](https://herdr.dev/docs/socket-api/) and [session restoration](https://herdr.dev/docs/session-state/) references in the source requirements are context, not a substitute for the installed/live schemas. Copy only synthetic fixtures for the accepted subset. A restored pane is presentation, never proof of continuing execution.

## 2. Package and contract (v2 §§3, 8, 14)

`hosts/herdr` is an optional client-side adapter, not a service. It accepts safe `Update` values carrying only control.Projection plus explicit freshness/reason, a bounded authenticated read-only resync callback, and a separate UI navigation callback, never AttachmentGrant, raw token, ledger handle, admission request, native transport or supervisor owner. `internal/cli/attach_host.go` composes it after the trusted core attachment succeeds; its lifetime is subordinate only to this client. Host failure cancels the adapter context, not core execution or accepted stop reconciliation.

```go
// New hosts/herdr package. All statuses are bounded enums, never raw host errors.
type Options struct { SocketPath, PaneID string }
type Compatibility struct {
    InstalledVersion, SchemaSHA256, LiveVersion string
    SchemaVersion, Protocol int
    Metadata, Callbacks, Restore bool
    Reason string
}
type Action struct { Kind string } // review, stop_confirmation, writable_unsupported only
type Update struct { Projection control.Projection; Stale bool; Reason string }
type Publication struct { Source string; Sequence uint64; Update Update }
func Probe(ctx context.Context, options Options) (Compatibility, error)
func Report(ctx context.Context, options Options, compatibility Compatibility, publication Publication) error
func ParseAction(raw []byte) (Action, error)
func Run(ctx context.Context, options Options, updates <-chan Update, resync func(context.Context) (Update, error), navigate func(Action) error) error
```

The CLI owns the private attachment.Controller and maps attachment.State into Update. It supplies a host-only bounded subscription using Controller.Subscribe, not a second competing reader of Controller.Updates. The resync closure calls Controller.Refresh with the supplied deadline and maps the returned sanitized State; it exposes no grant, token, repository selection, mutation or native handle. Refresh performs an authenticated read using the existing grant, never implicit Reconnect, service start or recovery. Generation/permission errors remain stale until the operator explicitly reconnects. Callback errors use the same bounded reason enums; `Reason` is empty when fresh, otherwise core_disconnected, authority_stale, queue_overflow, revision_conflict, host_unavailable or unknown. The host cannot choose arguments to widen this callback's run scope.

An input Stale=true or adapter queue overflow immediately latches adapter freshness stale. It cancels pending fresh publication, attempts at most one bounded stale publication and schedules a single-flight resync outside all queue/core locks. Only a successful resync result with Stale=false and the current attachment/generation may clear that latch; an ordinary duplicate status event cannot. A fresh authenticated response containing exactly the same projection revision/content as the cached one **does** clear it and is published as a freshness transition. An input authority/generation transition remains stale until the controller's explicit authenticated reconnect has supplied the new identity. A failed read never renews fresh host metadata. This is an explicit channel for freshness, not an inference from projection revision changes.

Exported failure sentinels `ErrUnavailable`, `ErrUntrusted`, `ErrIncompatible`, `ErrInvalidMessage`, `ErrUnsupportedAction` support errors.Is; CLI maps each to sanitized fixed diagnostic text. No raw error string is projected. SocketPath is local operator configuration, never a host-supplied core scope. PaneID directs reporting only and is bounded to1..128 printable ASCII characters excluding controls; it cannot choose the attached run/repository. Missing/unknown endpoint discovery refuses optional integration rather than guessing sockets or inspecting unrelated sessions.

## 3. Negotiation and local transport (v2 §14; NFR-3)

`Probe` runs installed version/schema discovery as direct argv, no shell, with a two-second overall deadline and bounded output. Installed schema discovery may use a separate bounded parser budget up to2MiB because the complete installed schema includes unrelated APIs; no host bridge frame exceeds64KiB. Extract only the two allowlisted methods and required response fields; unknown schema_version/protocol disables integration. Accepted initial profile is CLI0.9.1/schema1/protocol22, exact schema subset digest recorded in fixture manifest; matching a version string alone is insufficient.

Establish trust before sending any host message. Unix endpoint must be a socket in a user-controlled non-world/group-writable parent, owned by the current user, with same-user peer identity established using platform facilities where supported. Windows requires a user-restricted pipe and proven peer/logon restriction; until that is qualified the host integration row is unsupported, not a permission fallback. Resolve symlinks safely and reject changed endpoint identity across validation/connect. Use the actual installed host's documented endpoint; unknown trust disables integration. No TCP, unauthenticated fallback, permission widening or native credential access.

Send one `{"id":"<random request id>","method":"ping","params":{}}` frame. Require matched id and result.type=pong, protocol22 and exact live version/profile compatibility; optional capabilities do not grant extra methods. Error responses are mapped to reason codes, never rendered verbatim. A bundled schema with a failed/missing live pong is incompatible. Arbitrary host events and unmatched IDs are rejected. Framing, peer-proof and method-specific metadata acknowledgment must be captured by Task1's installed documentation/schema investigation and Task7 live test before a compatible row is enabled; if the installed transport cannot establish them, Metadata=false and manual attach remains functional. No guessed API method is an acceptable implementation.

The only permitted outgoing method names are ping and pane.report_metadata. Reject `command.invoke`, pane input, server stop/handoff, agent resume/start, publication, approval and provider-switch regardless of host claims. No session enumeration or snapshot is needed to report to the explicit PaneID.

## 4. Safe metadata mapping (v2 §§8, 14)

Send this exact request subset after compatible live negotiation:

```json
{"id":"request_synthetic","method":"pane.report_metadata","params":{"source":"mythhelm-session_synthetic","pane_id":"pane_synthetic","seq":2,"ttl_ms":5000,"tokens":{"mh_run":"run_synthetic","mh_state":"executing","mh_review":"unknown","mh_attention":"none","mh_freshness":"fresh"}}}
```

IDs above are synthetic. Allowed params are source, pane_id, seq, ttl_ms, tokens; do not send title, agent, display_agent, arbitrary state_labels, applies_to_source or clear flags. `source` is `mythhelm-` plus a fresh collision-resistant session ID generated by the adapter after successful negotiation, never a credential. `seq` is a strictly increasing uint64 **publication sequence within that source namespace**, starting at1 and incrementing for every fresh, stale or renewal report; it is not ProjectionRevision. A reconnect/new attachment/generation creates a new source namespace; no old report is replayed into it. Run serializes reports (one in flight), discards late acknowledgments by request ID and retires the prior namespace without renewing it. Sequence exhaustion fails closed and requires a fresh negotiated namespace.

TTL is5000ms. Healthy stable content is renewed every1000ms even if its core projection revision has not changed: a renewal cycle first calls the authenticated resync callback, then sends the resulting fresh Update with a **new transport publication sequence**, all within one shared2second cycle deadline. An unchanged core projection remains a no-op for core state/order, but never suppresses a scheduled transport renewal or freshness transition. mh_revision continues to carry the actual unchanged core projection revision. If resync or transport fails, mark stale, cease fresh renewal, optionally send one stale report if the remaining deadline permits, and let the prior report expire. On host disconnect/process exit, no renewal is sent; metadata expires within5seconds of the last report accepted by that host.

The qualified profile must demonstrate that same-content reports with increasing seq renew TTL, seq is ordered per source, a newly negotiated source can report from1, and retired/expired source data cannot supersede current newer-session metadata. Schema acceptance of optional seq/source/ttl_ms alone proves none of those semantics. Task7's live suite must observe all four properties; otherwise Metadata=false for that profile, with manual attach working. No durable host sequence file or second orchestration store is introduced. Metadata TTL is presentation, never a worker/authority lease.

The closed token keys (≤16 total) are mh_run, mh_task, mh_attempt, mh_state, mh_harness, mh_model, mh_waiting, mh_review, mh_attention, mh_attachment, mh_generation, mh_revision and mh_freshness. State/evidence values derive only from Update.Projection; mh_freshness derives from Update.Stale plus the adapter stale latch, not from a host acknowledgment. Absent pointer fields map to literal unknown. The schema's term tokens means display string values here, never a capability token. Do not include Label until a dedicated safe-label fixture proves the same core sanitized label; initial mapping omits it. Bound each string to160 scalars and the whole serialized frame to64KiB, without truncating identifiers into a different identity. Invalid identifier/enum/size disables that projection and marks stale. No nested maps or raw JSON/native payload pass through.

Core projection revision handling belongs to the existing-run-client controller. Adapter also rejects a mismatched attachment/generation and cannot send an older revision after a newer one; identical projection duplicates do not change canonical content, while successful authenticated resync freshness transitions and timed transport renewals still publish. Conflicting same revision latches stale and invokes the bounded read-only resync callback. Report acknowledgments indicate only metadata transport success; they never change run state, review state or stop result. Method-specific response is initially required to be matched id/result.type=ok, contingent on qualified profile evidence; any different actual response remains unsupported until a reviewed exact profile is added.

## 5. Navigation, manual fallback and restoration (v2 §§14–15)

The initial profile has no demonstrated callback or attach-only restore-registration surface: Callbacks=false, Restore=false. It does not register speculative methods. The standalone client keeps keyboard review and stop confirmation functional; manual `mythhelm attach <run-id>` is always tested. A reported host action, if supplied by a later qualified explicit callback transport, parses only `{"action":"review"}` or `{"action":"stop_confirmation"}`; `{"action":"writable_takeover"}` maps to writable_unsupported and produces zero native input. Max action payload4096 bytes, no unknown/duplicate keys, no command string, run override, repository, token, approval or operation ID. Without qualified callback transport, even a valid action is unavailable through the host and cannot invoke navigate; synthetic tests exercise the parser and trusted navigation separately without calling this shipped support.

The trusted callback changes UI focus only. Stop requires a fresh core snapshot, user confirmation and the typed operation with expected revision/generation from the core child. Never forward a host approval bit or reuse pane focus as confirmation. Review opens the operator's already bound run review surface; its richer local view never flows back into host metadata. No host-side apply/publication/launch route exists.

CLI Task6 adds only attach flags `--host=herdr`, `--herdr-socket=<local endpoint>` and `--herdr-pane=<opaque pane id>`; ordinary attach defaults to standalone. Invalid flag combinations fail as usage without changing core run state. Missing/denied/obsolete/unknown/slow host leaves the successful core attachment running, with a fixed reason and manual fallback. The launch command's Herdr capability refusal remains unchanged. The pane restore descriptor, if a future exact registration profile is qualified, may contain only explicit state-root identity and argv for `mythhelm attach <run-id>`, never raw native resume/run, tokens or shell text; implementing that new host method is outside this initial profile and must update design/fixtures first. Manual fallback is delivered now, not deferred behind that investigation.

## 6. Isolation, bounds and qualification (v2 §§14, 18)

Each host request has a two-second deadline. The queue has at most16 replaceable Update slots, coalesces overflow to latest and latches stale before the bounded resync callback; it never blocks the core/client producer. No core transaction/actor/client grant lock is held across host I/O. Do not treat accepted stop acknowledgment as replaceable status. Core stop/query uses a separate authenticated path even when Report stalls. Test a ten-second host stall while a different trusted client performs status/read and confirmed stop request: both accepted/rejected acknowledgments stay below2seconds; deliberately blocking adapter fixture fails the shared assertion.

The renewal timer, resync worker and transport publication each have at most one in-flight item; they do not accumulate missed timer ticks or unbounded retries. Cancel joins these client-side workers and unsubscribes without touching the service.

Fixture suite uses synthetic schemas, fake local transport and an instrumented fake native executable with launch/stop/input counters. Task7 separately runs a live-tagged suite with the installed host, exact OS/arch, CLI/live version/protocol and native fake surface. It must demonstrate detach, reconnect, pane-process teardown, host restart, duplicate restore, missing socket and obsolete schema with one attempt and no presentation-caused start/stop. Duplicate restore in the initial profile is two explicit invocations of the manual attach restoration command, labelled manual (not automatic host registration). Real host restart/pane teardown must be performed in a disposable isolated host session; do not inspect or modify unrelated user panes. A merely restarted socket fixture is not live host restart evidence.

The evidence matrix separates fixture, live observed, untested and unsupported cells for each capability/event. No installed schema probe counts as live compatibility or lifecycle. Failed live row remains experimental/unsupported with reason. Accessibility evidence is inherited only from the exact existing-run-client keyboard/linear journey and extended with visible host-degraded/stale indicators; full AT-36/AT-48 and G11 stay open.

## 7. Decisions

| ID | Decision | Rationale |
|---|---|---|
| D1 | Optional client-side adapter with safe Projection only. | A host daemon/ledger reader would create another authority or scope leak. |
| D2 | Two outgoing methods in one exact profile. | Discoverable arbitrary methods do not confer permission. |
| D3 | Live pong plus restricted local peer proof. | Bundled schema/version cannot establish running-server compatibility or trust. |
| D4 | Safe Update freshness, read-only resync and independent publication sequence with authenticated TTL renewal. | Bare projections hide disconnect/overflow; unchanged core revisions must neither imply freshness nor prevent healthy renewal. |
| D5 | Manual attach and unsupported automatic restore/callbacks initially. | No exact qualified registration/callback surface was established; invented API calls are unsafe. |
| D6 | Fixture/live evidence kept separate. | Synthetic lifecycle does not qualify actual host restart behavior. |

The composition ADR0021 records the scoped host-facing boundary. Any added host action, restore registration method or changed public option requires reviewed contract/evidence update; no ownership/topology change is implied.

## 8. Honesty register

| Spec demand | Position |
|---|---|
| FR-1 manual/public attach | Required completed prerequisite and retested in live/manual fallback. |
| Live Herdr metadata support | Untested at design time; no supported row until exact transport/method/lifecycle suite passes. |
| Automatic restore registration | Unsupported initial profile; public manual fallback required and tested. |
| Host callbacks | Unsupported initial profile; trusted keyboard navigation remains available. |
| Native writable takeover | Explicitly refused with zero native input bytes. |
| Complete MH-3/G11/AT-36/AT-48 | Partial; exact embedded evidence only, later full/native tracks remain open. |
| Windows | Unsupported until process ownership and host transport trust are qualified together. |
| Billing/native fidelity | No new route/surface qualification is claimed by metadata. |

## Cross-Spec References

Epic: `specs/*/herdr-embedded-client/plan.md`. Prerequisites: `specs/*/supervisor-attachment-control/` for AttachmentClient/Projection and durable controls; `specs/*/existing-run-client/` for public attach/controller/navigation. Source: `specs/*/herdr-bridge/requirements.md`. No dependant child. Shared internal/cli/attach paths serialize after client completion; docs merge after core/client documentation and `specs/*/docs-site-restyle/` Task11 plus `specs/*/tui-observatory-view/` Task19, re-grounding user-guide labels/example conventions before editing.
