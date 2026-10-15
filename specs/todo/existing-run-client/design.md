# Existing run client — Design

Normative source: Master Specification v2, especially §§3, 6, 14–15 and I03/I06/I09/I12/I17/I18/I23. This is child B of `specs/*/herdr-embedded-client/plan.md`; original AC identifiers remain unchanged.

## 1. Current state and prerequisites

Grounding at `61d18e2`: `internal/cli/dispatch.go:32` has the complete `commands` registry; reading it shows run/review/stop/recover but no attach command (`rg -n 'var commands|"review"|"recover"' internal/cli/dispatch.go`). `internal/cli/tui.go:270`, `runInspectTUI`, constructs a TUI with local state and `wireActions`; `internal/tui/model.go:30`, `Config`, carries state directory and supervisor actions. `internal/tui/live.go:176`, `reloadSnapshot`, reloads a local journal view. `internal/tui/actions.go:18`, `Actions`, exposes Stop/Recover/Apply, so reusing that owner-capable configuration unchanged is not scoped attachment. `tests/e2e/main_test.go:27`, `TestMain`, builds the actual CLI once for isolated end-to-end tests. Re-ground with `rg -n 'func runInspectTUI|func .*reloadSnapshot|type Config|type Actions|func TestMain' internal/cli/tui.go internal/tui/{live,model,actions}.go tests/e2e/main_test.go` after prerequisite merges.

PARTIAL: the current inspect UI supplies rendering and navigation, not an authenticated scoped attachment lifecycle. HOLDS: public attach and presentation independence require executable evidence. This child starts implementation only after **all** `supervisor-attachment-control` tasks merge, including ordinary public service-owned run lifecycle, root migration, reconciliation and attachment protocol. A helper-seeded ledger or recovery-only run is insufficient. Serialize shared TUI/CLI edits after `tui-observatory-view`; re-ground its merged action lifecycle rather than restoring baseline cancellation behavior. No host installation or adapter is a prerequisite.

## 2. Controller and contracts (v2 §§3, 14)

Create `internal/attachment`, a core package depending only on the core attachment interface, not TUI or hosts. The core child's design §§4–6 is the normative producer contract: `control.AttachmentClient`, `OpenAttachment`, `AttachmentGrant`, `Projection`, `StopRequest`, `StopAck`, and `StopResult` (including `Ack StopAck`). Do not copy protocol structs or substitute direct journal reads. Exact consumer interface:

```go
// internal/attachment/client.go
// State contains sanitized projection data only, never the bearer grant.
type State struct {
    Projection control.Projection
    Stale bool
    Reason string
}
type Controller struct { /* private grant, queue and pending operations */ }
func Open(ctx context.Context, client control.AttachmentClient, runID string) (*Controller, error)
func (c *Controller) Current() State
func (c *Controller) Refresh(ctx context.Context) (State, error)
func (c *Controller) Updates() <-chan State
func (c *Controller) Subscribe() (states <-chan State, unsubscribe func())
func (c *Controller) Close(ctx context.Context) error
// PreparedStop is an opaque controller-issued confirmation handle.
type PreparedStop struct { OperationID string; /* private original request */ }
func (c *Controller) PrepareStop() (PreparedStop, error)
func (c *Controller) ConfirmStop(ctx context.Context, prepared PreparedStop) (control.StopAck, error)
func (c *Controller) StopResult(ctx context.Context, operationID string) (control.StopResult, error)
func (c *Controller) Reconnect(ctx context.Context) (State, error)
func (c *Controller) RequestWritable(ctx context.Context) error
```

Open authenticates the requested run in the resolved existing root, retains the grant privately, and exposes the initial projection. It does not call launch, recover, migration, ensure/start-service, or owner-detach hooks. Use producer control.DialExisting(ctx) and NewAttachmentClient(conn); never control.Connect, which may spawn. A missing endpoint is a typed unavailable result with explicit operator guidance; no lazy service spawn on attachment. Root resolution must fail on conflict, never search another root. Auth, unknown run, unresolved ownership, stale generation, invalid contract, oversized frames and unavailable persistence errors retain safe typed codes. Caller contexts are capped at two seconds per wire operation. An invalidated grant makes the controller stale and disables mutation; only explicit Reconnect authenticates a fresh grant. A pending StopResult query after reconnect uses the same operation ID and the new grant, never resubmits an old-generation request.

Refresh validates schema, RunID, attachment ID and generation against its private grant before publishing. For the same grant, lower ProjectionRevision is discarded; an equal revision with different content is an invalid-contract error, not a replacement. A new authenticated grant atomically resets the revision namespace. RunRevision and ProjectionRevision remain distinct. Null model/task/waiting fields render unknown. Bounded snapshots contain no paths, credentials, prompts, terminal controls or raw receipt payloads; the typed core allowlist is the sole source.

The controller has one bounded ring of at most16 safe State values per attachment, with independent subscriber cursors; Updates is the primary view cursor, Subscribe creates another cursor over that same ring and returns an idempotent unsubscribe. At most2 subscribers (view and optional host) are admitted; an excess Subscribe returns a closed channel and no-op unsubscribe, without allocating a queue. Subscribers never compete by reading the same channel and no per-subscriber copy backlog expands the ring. Cursor overrun yields a coalesced current State with Stale=true/Reason=queue_overflow. A successful authenticated Refresh publishes its safe State even when ProjectionRevision/content is unchanged, so freshness recovery cannot disappear under duplicate-content suppression. Error/disconnect transitions also publish State.Stale/Reason independently of canonical projection revision. Subscribers receive immutable state copies only; the grant remains private.

The controller has at most 16 replaceable queued states. Overflow coalesces status and marks stale; only a fresh authenticated Read clears that marker. A slow renderer never blocks control calls or worker ingestion. Stop acknowledgments/results use directly correlated method returns, not the replaceable queue. One controller polling loop is joined on Close; Close revokes its grant within the request deadline and frees local resources even if transport fails. It never stops the service or releases its ownership locks. Token values never enter errors, logging, exported state, argv or restore strings.

## 3. Attached TUI and navigation contract (v2 §§14–15)

Create a dedicated attachment model using the existing theme/capability/layout primitives without routing through `viewmodel.Load` or live owner hooks. Its exact public entry is:

```go
// internal/tui/attachment.go
// Navigate accepts only review, stop_confirmation, writable_unsupported.
type AttachmentConfig struct {
    Controller *attachment.Controller
    Caps caps.Caps
    Tokens theme.Tokens
    Navigation <-chan string
}
func RunAttachment(ctx context.Context, cfg AttachmentConfig) error
```

The optional host child adapts its own typed action into this bounded, nonblocking navigation input. Unknown actions are refused. Navigation changes only selected panel or opens a confirmation dialog. It never calls ConfirmStop. The local terminal owns the confirmation event; host-provided strings, pane IDs and focus cannot synthesize it. The review panel displays the selected run's canonical sanitized review state, harness/model and limitations. It neither claims a full receipt/diff is present in the projection nor adds apply/publication/recovery controls. It gives the working separate `mythhelm review <run-id>` command for the operator's fuller trusted review surface; that command is guidance, never executed by host navigation.

Stop confirmation visibly binds the selected run, attempt, generation and run revision. PrepareStop captures the immutable request and fresh operation ID; only the deliberate terminal confirmation invokes ConfirmStop. A competing update must return revision_conflict, close the old confirmation, and require refresh plus new explicit confirmation. Lost acknowledgment remains unconfirmed; retry in the same generation uses identical request/operation ID. After generation change, Reconnect plus StopResult recovers the durable acknowledgment and result without re-delivery. Show requested/unconfirmed until core-confirmed, including client disconnect and timeout. Query failure never means stopped. The selected pending operation ID remains visible for authenticated standalone status recovery; process teardown may lose local memory but not core reconciliation.

The attached client's q/back, Ctrl-C, input EOF, terminal loss and process teardown are detach-only unless an explicit stop was already confirmed. They close the subscription and never signal the owner or synthesize stop. Accepted stop reconciliation remains service-owned after view loss. This context-specific Ctrl-C behavior is documented; it does not alter foreground run cancellation. Rendering preserves MH-23 accessibility/motion/color rules and sanitizes again at the display boundary. No raw native viewer or input writer exists. `writable_unsupported` reports capability unsupported with zero session bytes and no new terminal; unknown owner remains blocked.

## 4. Public CLI and errors (v2 §15)

Task 3 registers `mythhelm attach <run-id>` and help. Reuse the existing TUI option parser for `--plain`, `--accessible`, `--format plain|jsonl`, color/colour, motion and icons; validate exactly one bounded run ID using the merged MH-23 run-ID validator. TTY defaults to the attached TUI. Plain, accessible, JSONL, non-TTY and TERM=dumb perform one authenticated snapshot and detach, with explicit `view_closed` disposition rather than run completion. This bounded default gives scripts/manual fallback a stable command without a hidden watch lifecycle. JSONL uses the repository versioned envelope with attachment identity/revisions, safe projection, stale state and final attachment result; never raw grant. `--help` needs no core endpoint. No host flags are added in this child; the optional host child owns them.

Successful snapshot or deliberate detach returns 0 for the view operation only. Invalid arguments return 2, authorization refusal 3, unresolved ownership/transport uncertainty/lost terminal 6, unsupported capability 7, following existing exit mapping for internal failures. Unknown and foreign runs uniformly return permission_denied mapped to 3, without disclosing whether the run exists; no typed not-found distinction is exposed. A pending unresolved explicit stop returns 6; confirmed stop is reported separately from run success. A previously established nonzero action failure survives terminal loss. Full run/candidate/verification disposition is never inferred from client exit. Status output names the safe reason and manual operator action without copying internal errors.

## 5. Evidence and boundaries (v2 §18)

Unit tests include forged projections, reordered/equal revisions, fresh-generation low revision, overflow, timeout, revocation and changed attempt between confirmation and mutation. TUI tests drive navigation and actual terminal confirmation separately. End-to-end tests start an ordinary admitted fake run through the packaged public CLI and the real singleton composition; observe one initial native launch and unchanged run/attempt/owner identity across initial/repeated/manual attachments, socket disconnect and separate client-process kill. A positive explicit-stop control proves the launch/stop counters are wired. Finish each fixture by explicit test-owned control and join all processes; never kill a real user supervisor.

A ten-second stalled consumer must not prevent a concurrent authenticated status or permitted stop acknowledgment within two seconds. A deliberately blocking transport/consumer variant must violate the same assertion, proving the timing detector works. Contributor evidence uses no credentials, live host or network. Windows/Linux/macOS results are qualified separately against the core lifecycle evidence; skips are not support claims.

## 6. Decisions

| ID | Decision | Rationale |
|---|---|---|
| D1 | Scoped controller and dedicated attached model. | Existing inspect model reads a broader ledger and owns action hooks unsuitable for host-scoped projections. |
| D2 | Navigation only from host; terminal confirmation for stop. | Pane text/focus and callback delivery cannot grant mutation authority. |
| D3 | One snapshot in linear modes; interactive view on capable TTY. | A bounded scripting operation has an honest exit contract without assuming run completion. |
| D4 | Detach-only attached-client interrupt. | Execution remains service-owned; foreground run cancellation is a different explicit context. |
| D5 | Manual public attach is always available without Herdr. | Untested restoration cannot be replaced by raw native resume. |

Public command and ownership choices are recorded in core proposed ADR 0021 before implementation; this child does not invent a separate owner-transfer policy. Any reviewed contract change updates producer and consumers before dispatch.

## 7. Honesty register

| Spec demand | Position |
|---|---|
| Ordinary existing production run attachment | Required; helper-only or recovery-only evidence cannot complete this child. |
| Native writable takeover and G12 | Unsupported; zero native input and no competing terminal. |
| Full MH-3/G11 and AT-36/AT-48 | Partial slice only; MH-23 and qualified host evidence remain necessary. |
| Native full diff/receipt in host metadata | Not exposed; selected review state plus explicit trusted standalone review guidance. |
| Live Herdr restoration | Optional later child; public manual attach remains working fallback. |
| Platform support | Only platforms with core lifecycle and this child’s executable evidence; no qualification inherited from a spec. |

## Cross-Spec References

Epic: `specs/*/herdr-embedded-client/plan.md`. Prerequisite: `specs/*/supervisor-attachment-control/` (all production lifecycle and attachment/control contracts). Dependant: `specs/*/herdr-scoped-projection/` (safe projection and trusted navigation only). Source: `specs/*/herdr-bridge/requirements.md`. Shared TUI lifecycle references `specs/*/tui-observatory-view/` as an implementation prerequisite, not assumed shipped behavior.
