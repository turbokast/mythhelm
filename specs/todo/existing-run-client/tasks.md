## Existing run client — Tasks

### Dependencies

- All tasks in `specs/*/supervisor-attachment-control/` must be merged first. Consume its design §§4–6 exactly, including `StopResult.Ack`. `specs/*/tui-observatory-view/` shared CLI/TUI lifecycle work must be merged before Task 2; re-ground signatures and serialize overlapping files. No optional host dependency.
- Task 1 → Task 2 → Task 3 → Task 4; Task 5 follows Task 1 and may run beside Tasks 2–4 (disjoint core/TUI files); Task 6 joins all implementation; Task 7 follows Task 6. Later `herdr-scoped-projection` consumes the completed contracts.
- **Gates for every task**: format changed Go files with gofmt, then `scripts/harness/gate.sh go` for Go changes; `scripts/harness/gate.sh hygiene`; `scripts/ci/lint-agent-harness.sh --only specs` for completion metadata. Documentation tasks run relevant built-site/link/example checks and harness gates when their changed paths require them. Independent code review always; architecture review for control/ownership/public contracts. A file existing or agent success report is not completion.
- **Completion convention**: append `✅ COMPLETED` to the heading only with the task-completion procedure; record Status with PR, Implementation commits, Spec deviations, Files modified and actual acceptance evidence; update scratchpad/handoff. Preserve original fields.
- **Commits**: signed off, named paths, isolated worktree, PR to main after checks. Core ADR 0021 must be reviewed before public/ownership implementation; never fabricate a signed product approval.

## Implementation Tasks

### Task 1 — Scoped attachment controller

- **Domain/agent**: go-implementer (core)
- **Budget**: complex (concurrency, authentication and operation replay)
- **Depends on**: None
- **Change**: Consume the core attachment protocol through a bounded controller with private grants, authenticated revision handling and explicit stop preparation/replay/query. Never use owner hooks or direct ledger reads.
- **Files**:
  - `internal/attachment/client.go` (new)
  - `internal/attachment/client_test.go` (new)
  - `internal/attachment/stop.go` (new)
  - `internal/attachment/stop_test.go` (new)
- **Produces**: Design §2 `State`, `Controller`, `PreparedStop`, `Open`, Current/Refresh/Updates/Subscribe/Close, PrepareStop/ConfirmStop/StopResult/Reconnect signatures; RequestWritable is supplied by Task 5. Typed core errors are preserved, grants stay private.
- **Acceptance**:
  - `TestOpenExistingRunHasNoLifecycleEffects`: initial and repeated Open return the exact core run/attempt identity; instrumented launch/recover/stop/owner-detach boundaries remain zero, and an intentionally effectful client trips the assertion (AC-1.1–1.3).
  - `TestAttachmentScopeAndRevision`: foreign run/attachment, unsupported schema, wrong generation, lower revision, equal revision with changed content, revoked grant and fresh authenticated generation are exercised; only coherent current snapshots publish; stale errors disable stop (FR-3 dependency).
  - `TestAttachmentSafeSubscriberFanout`: view and optional host receive independent ring cursors, both observe disconnect/stale and same-revision authenticated fresh recovery; the shared ring stays at16, excess subscriber allocation is refused and no grant enters a published State.
  - `TestAttachmentQueueBoundAndResync`: 17 updates to a stalled consumer never exceed 16 queued states, visibly mark stale, preserve correlated stop acknowledgment, and clear stale only after fresh Read (NFR-2).
  - `TestAttachmentStopReplayAndQuery`: competing update rejects without delivery; duplicate identical request recovers identical acknowledgment; changed arguments reject; reconnect to new generation queries the original operation with its fresh grant and retrieves `StopResult.Ack`, with one physical stop effect (AC-4.1/4.2 dependency).
  - `TestAttachmentCloseDeadlineAndSecrets`: stalled Read/Close terminate within two seconds; Close joins local polling despite transport failure; logs/errors/state contain no token sentinel; Close sends zero owner signals (AC-1.2/1.3, NFR-2/3).
- **Test plan**: controllable typed core double with barriers and explicit effect counters; real protocol behavior remains the core prerequisite and Task 6 integration. Drive positive calls before asserting negative side effects.
- **Invariants touched**: I03/I17 (§14: host is no authority); I06/I12 (§6: no lifecycle effects or replayed tools); I09/I23 (§§5/14: canonical bounded state); I18 (§3: no competing owner).

### Task 2 — Projection-only attached TUI

- **Domain/agent**: tui-implementer (tui)
- **Budget**: complex (asynchronous ordering and terminal lifecycle)
- **Depends on**: Task 1
- **Change**: Add the dedicated attached view and typed projection rendering with bounded navigation ingress and detach-only lifecycle. Reuse display primitives without owner-capable snapshot/action wiring.
- **Files**:
  - `internal/tui/attachment.go` (new)
  - `internal/tui/attachment_test.go` (new)
  - `internal/tui/attachment_view.go` (new)
  - `internal/tui/attachment_view_test.go` (new)
- **Produces**: `AttachmentConfig` and `RunAttachment(ctx context.Context, cfg AttachmentConfig) error` exactly as design §3; Navigation accepts review, stop_confirmation, writable_unsupported, never authority. Task 4 completes controls; until then navigation shows unavailable rather than invoking a mutation.
- **Acceptance**:
  - `TestAttachmentViewProjectionOnly`: inject projection with unknown task/model and unsafe controls; renderer shows unknown and sanitized fields, never reads local ledger/native payloads; newer snapshot replaces older without selection rollback (FR-3 dependency).
  - `TestAttachmentDetachEvents`: q/back, Ctrl-C, EOF, terminal loss and context cancellation close only the subscription; stop/start/owner-detach counters stay zero and loop exits without leak (AC-1.3).
  - `TestAttachmentNavigationIsNotConfirmation`: each allowlisted navigation selects its panel; unknown input refuses; none invokes stop, launch, recover or apply, including forged host confirmation text (AC-4.1 dependency).
  - `TestAttachmentAccessibleLayout`: compact/80/100/140-column, short-height, no-color and reduced-motion fixtures keep run identity, stale marker and requested/unconfirmed labels legible without claiming verified from native completion (NFR-1, FR-3 dependency).
- **Test plan**: drive Bubble Tea messages and injected controller snapshots; race test stalled rendering with bounded producer; no TTY/native account dependency.
- **Invariants touched**: I06/I18 (§3: display does not own execution); I09 (§15: unknown and stale visible); I03/I17 (§14: navigation cannot confirm); I13 (§18: offline contributor tests).

### Task 3 — Public attach command and output

- **Domain/agent**: go-implementer (core)
- **Budget**: complex (public CLI and terminal/error modes)
- **Depends on**: Task 1, Task 2
- **Change**: Register the public attach verb, resolve only the configured existing root/endpoint and route TTY to the attached TUI or linear modes to one authenticated snapshot. Provide accurate view dispositions and exit codes.
- **Files**:
  - `internal/cli/dispatch.go`
  - `internal/cli/dispatch_test.go`
  - `internal/cli/attach.go` (new)
  - `internal/cli/attach_test.go` (new)
  - `internal/cli/attach_render.go` (new)
  - `internal/cli/attach_render_test.go` (new)
- **Produces**: `mythhelm attach <run-id>` with design §4 modes/help/errors; unexported `runAttach(args []string, stdio Stdio) error`; no host-specific flags or restore registration.
- **Acceptance**:
  - `TestAttachDispatchAndHelp`: cli.Main help lists attach and command help succeeds without endpoint; missing/extra/malformed run ID or unknown option returns 2 with no endpoint or native launch (AC-1.5).
  - `TestAttachExistingEndpointOnly`: authenticated existing run opens successfully; unknown and foreign runs both return permission_denied/exit 3 with indistinguishable safe diagnostics; missing endpoint, authentication failure, ownership unresolved and root conflict return the explicit design §4 code/reason and never start supervisor, native worker, migration, recovery or search another root (AC-1.1/1.2).
  - `TestAttachModeSelection`: capable TTY invokes RunAttachment; plain/accessible/JSONL/non-TTY/TERM=dumb each perform one snapshot then Close, returning view disposition, not successful run disposition; NO_COLOR and overrides follow existing parser (AC-1.5).
  - `TestAttachJSONLAndExit`: parse every stdout line as the versioned envelope, assert unknown remains null/unknown, final view disposition and revision identity exist, no token/unsafe control sentinel appears; diagnostics stay stderr; lost view/pending stop is 6 and established nonzero action failure is preserved.
- **Test plan**: cli.Main tests with injectable existing endpoint and launch selector; assert output plus counter effects, including successful control fixtures; packaged behavior follows in Task 6.
- **Invariants touched**: I02/I03 (§3: authenticate exact root); I06/I12 (§14: effect-free attach); I09 (§15: view success differs from run success); I13 (§18: host-independent CLI).

### Task 4 — Trusted review navigation and explicit stop

- **Domain/agent**: tui-implementer (tui)
- **Budget**: complex (confirmation races and durable-result display)
- **Depends on**: Task 1, Task 2, Task 3
- **Change**: Complete the review and stop-confirmation panels in the attached view. Host navigation can focus a trusted panel; only a local deliberate terminal event submits the prepared typed stop.
- **Files**:
  - `internal/tui/attachment.go`
  - `internal/tui/attachment_controls.go` (new)
  - `internal/tui/attachment_controls_test.go` (new)
  - `internal/tui/attachment_view.go`
- **Produces**: Working review/stop_confirmation/writable_unsupported navigation behavior from design §3, with no new host-visible capabilities.
- **Acceptance**:
  - `TestAttachmentTrustedReview`: navigation exposes the same run's canonical review state and literal safe `mythhelm review <run-id>` guidance; it does not launch that command or expose apply/publication/provider-switch actions (AC-4.1 dependency).
  - `TestAttachmentStopRequiresTerminalConfirmation`: host navigation opens a dialog with run/attempt/generation/revision; only deliberate terminal confirmation sends the prepared operation; escape/focus changes/repeated callback produce zero mutations.
  - `TestAttachmentStopConflict`: deterministic competing update after dialog preparation returns revision_conflict with no accepted stop; refresh plus another terminal confirmation is required, never auto-retry with fresh revision.
  - `TestAttachmentLostAckAndDelayedStop`: acknowledgment within two seconds shows requested; confirmation delayed beyond two seconds remains unconfirmed; disconnect/reconnect queries the same operation, eventually displays confirmed only on core evidence, and never delivers another stop (AC-4.2 dependency).
  - `TestAttachmentPendingStopViewLoss`: terminal loss does not cancel accepted reconciliation or claim stop confirmed; unresolved result remains 6, established failure retains its code, and operation ID/recovery guidance are available without credentials.
- **Test plan**: real controller over deterministic protocol double, separate host navigation and keyboard event injection, delayed-confirmation barriers and negative controls. No wall-clock inference of stop success.
- **Invariants touched**: I03/I17 (§14: trusted local confirmation only); I06/I09 (§6.4: acknowledgment is not confirmed stop); I07 (§15: review does not fabricate verification); I12 (§6: replay cannot repeat effects).

### Task 5 — Refuse writable native attachment

- **Domain/agent**: go-implementer (core)
- **Budget**: standard
- **Depends on**: Task 1
- **Change**: Expose an explicit unsupported writable-capability result without a native-input transport, writer lease or terminal creation path.
- **Files**:
  - `internal/attachment/capabilities.go` (new)
  - `internal/attachment/capabilities_test.go` (new)
- **Produces**: `func (c *Controller) RequestWritable(ctx context.Context) error`, returning the canonical capability_unsupported error consumed by host/client integrations; unknown ownership never grants input.
- **Acceptance**:
  - `TestAttachmentWritableUnsupported`: requested takeover on active, completed, stale and unknown-owner controller states returns capability_unsupported, with zero native input bytes, terminal creations or automation resumes (AC-4.3).
  - `TestAttachmentReadOnlyDoesNotGrantWriter`: valid read/navigation/stop result does not enable RequestWritable; a deliberately writable test double sends sentinel bytes and fails the same zero-input assertion.
- **Test plan**: instrument actual input boundary and terminal factory in the fixture rather than only checking an enum; ordinary tests require no native harness.
- **Invariants touched**: I17/I18 (§14: host cannot obtain or duplicate native input ownership); I09/I14 (§18: unsupported is explicit).

### Task 6 — Packaged production attachment journeys

- **Domain/agent**: go-implementer (core)
- **Budget**: complex (real subprocess lifecycle and responsiveness)
- **Depends on**: Task 1, Task 2, Task 3, Task 4, Task 5
- **Change**: Exercise public attachment against ordinary service-owned fake runs using the packaged binary, including independent client teardown, failure and explicit stop journeys.
- **Files**:
  - `tests/e2e/attach_test.go` (new)
  - `tests/e2e/attach_lifecycle_test.go` (new)
  - `tests/e2e/attach_responsiveness_test.go` (new)
- **Produces**: Executable AC-1.1–1.5/4.3, shared FR-3/4 and NFR-1–3 evidence with per-platform outcomes; no live Herdr claim.
- **Acceptance**:
  - `TestAttachPublicInitialRepeatedManual`: start an ordinary public run through the real service composition and instrumented fake worker; three public attach invocations (initial, repeated, host-unavailable manual) show identical run/attempt/owner and exactly one launch, zero presentation stop requests (AC-1.1/1.4/1.5).
  - `TestAttachPublicRefusals`: unknown and foreign runs both return permission_denied/exit 3 without an existence distinction; invalid arguments return 2; wrong authentication, conflicting root and unresolved ownership return their design §4 safe reason and expected nonzero code, all with unchanged launch counters (AC-1.2).
  - `TestAttachClientProcessTeardown`: kill only the attachment client after ready synchronization, separately close its control socket, then attach again; service and worker identities stay alive, deadline unchanged, native launch count remains one and stop requests zero (AC-1.3). A deliberate core stop afterward changes the observed counter and reconciles, proving instrumentation.
  - `TestAttachStopLostAck`: drive the public TTY stop confirmation through a local PTY where supported, lose the acknowledgment, reconnect/query original operation, and observe one durable accepted intent and one physical effect; no helper-only public journey claim. Unsupported platform PTY cases are reported without qualification.
  - `TestAttachStalledViewDoesNotBlockControl`: stall view consumption for ten seconds while authenticated status and permitted stop acknowledgment each finish within two seconds; the deliberately blocking fixture fails the identical responsiveness predicate (NFR-2).
  - `TestAttachProjectionNoNativeInput`: projection with secret/path/control sentinels excludes them from stdout/metadata; unsupported writable navigation sends zero native input; tests run with no host binary, native credentials or network (AC-4.3, NFR-1/3).
- **Test plan**: existing TestMain builds the binary; isolated runtime/state roots and instrumented fake scenario; no seeded shortcut replacing public service admission. Barrier synchronization, retained process identities, bounded cleanup and independent ledger/stop-file observations. Preserve failing counterexamples in the suite.
- **Invariants touched**: I06/I12/I18 (§3: owner survives clients); I03/I17 (§14: no host authority); I09/I14 (§18: exact executable platform evidence); I13 (§18: credential-free tests); I21 (§6: immutable deadline).

### Task 7 — Document existing-run attachment and limits

- **Domain/agent**: go-implementer (docs)
- **Budget**: standard
- **Depends on**: Task 6
- **Change**: Document the shipped public attach command, detached view semantics, trusted stop receipt/result and manual fallback with exact evidence and remaining qualification limits.
- **Files**:
  - `docs/attach.md` (new user guide page)
  - `docs/_data/navigation.yml` (add the page under Use, following the existing name/path/group record format)
- **Produces**: Public manual attachment guide with commands/output generated from the Task 6 packaged fixture and exact evidence references; no raw private runtime paths or credentials.
- **Acceptance**:
  - Execute `mythhelm attach --help`, then `mythhelm attach <fixture-run-id> --plain` and `--format jsonl` against a disposable service-owned fake run; cited generated output matches the guide's command/output blocks and the final result explicitly concerns the view.
  - The guide's lifecycle section distinguishes attach-client Ctrl-C/detach/teardown from foreground-run cancellation, and requested/unconfirmed stop from confirmed stop; manual fallback command is runnable without Herdr and never uses native resume.
  - The guide's support table records actual platform evidence and unsupported writable takeover/live host restoration, links the optional host child and MH-23, and does not claim complete MH-3/G11/AT-36/AT-48; built-site navigation resolves the new page.
- **Test plan**: execute documented commands against the packaged offline fixture, verify JSONL and exit codes, run docs build/link/example checks; inspect anchored lifecycle/support sections for exact labels and evidence references.
- **Invariants touched**: I09/I14 (§18: evidence-qualified claims); I06/I17 (§14: accurate detach/restore instructions); I13 (§18: offline manual path).
