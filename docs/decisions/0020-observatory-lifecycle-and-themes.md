# 0020. Observatory lifecycle and themes

- Status: accepted
- Date: 2026-10-15
- Approval: delegated maintainer decision recorded below, 2026-10-15.

## Task 3 authorization record

The delivery coordinator accepted the exact reviewed proposal below on 2026-10-15, exercising the maintainer’s explicit standing delegation for in-scope backlog delivery.

The maintainer granted “full autonomy and approval” for the ongoing backlog delivery, following the approved Observatory plan and Q1 client/canonical outcome distinction. This records a decision made under that delegation, not a claim that the maintainer personally inspected the subsequently authored concrete ADR.

The delegated acceptance concerns only the ADR content authored at [54f859d](https://github.com/turbokast/mythhelm/blob/54f859db5de5d51b9062c2b04b23d5e19ff6b1dc/docs/decisions/0020-observatory-lifecycle-and-themes.md), independently reviewed in [PR #484](https://github.com/turbokast/mythhelm/pull/484), and merged unchanged at [946ba3f](https://github.com/turbokast/mythhelm/blob/946ba3f2b967abcf5d04942b2f277f345010c282/docs/decisions/0020-observatory-lifecycle-and-themes.md). The original complete ADR file has SHA-256 `b3f22e5ae1cf34ff48be5313a9dad09dd3c966a875fcf4e660c943734d42ca05`. This digest identifies the proposal before adding this authorization record; it is not a digital signature or a digest of the amended file.

The coordinator reviewed the complete ADR and this authorization wording and accepted the identified content under the standing instruction. Independent architect review found no critical or important issues and established design consistency; the maintainer’s instruction, rather than the review or repository prose, supplies the delegated authority. This record is not a signed product approval or formal autonomy grant and changes neither approval system. It establishes no runtime qualification and does not bypass required implementation tests, PR review or CI.

## Context

The [Observatory requirements](../../specs/in-progress/tui-observatory-view/requirements.md) and [design](../../specs/in-progress/tui-observatory-view/design.md) implement a bounded part of [Master Specification v2](../../mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md) §§6.4, 15 and 16. UI exit must not stop an attempt implicitly or remove its finite envelope. The current per-run pipeline owns ingestion; the detached worker owns native execution. A client-only deadline disappears when that client exits, and a volatile deadline classification cannot survive recovery.

This proposal amends [ADR 0004](0004-slice-process-model.md)'s worker handoff and terminal classification, [ADR 0005](0005-run-owner-lock-and-ingestion.md)'s detach and conclusion rules, and [ADR 0007](0007-stop-and-recovery.md)'s recovery inputs. Their identity checks, first-request precedence, exclusive owner lock, spool ingestion and no-relaunch recovery remain binding. ADR 0007 remains proposed; referencing it does not accept it. This record decides behavior; it supplies neither implementation evidence nor platform qualification.

## Decision

### Pinned deadline and durable conclusion

Add `workers.Launch.ExecutionDeadline time.Time` with JSON key `execution_deadline`. The supervisor assigns its computed deadline after constructing the admitted launch, alongside nonce and generation. The worker rejects absent, zero or malformed deadlines and never starts native execution for an expired deadline. The deadline is pinned to that launch in both generation-zero and fenced operation; recovery cannot replace it. No worker identity schema, journal event type or process owner is added.

The worker schedules its own timer outside the generation fence. Both that timer and the supervisor deadline use `workers.DeadlineRequester = "execution-deadline"`. The worker maps this requester to `workers.DeadlineReason = "envelope_deadline_exceeded"` in the final `attempt.state_changed` event, preserving the existing `attempt.stopped` schema and stop ladder. Timer handling first reads any existing stop request; otherwise it atomically creates a deadline request through RequestStop and re-reads the winning request. A previously accepted user request wins, including an already running ladder. Never start a second ladder. Request creation/read failures use the existing abort and ownership-unresolved path rather than permitting unlimited native execution or claiming confirmed deadline termination.

For confirmed stopped attempts, `pipeline.conclude` derives blocked / `envelope_deadline_exceeded` from the journaled reason in normal and recover paths. Remove the volatile `deadlineHit` classification. A user stop that won remains cancelled; unconfirmed stops, escaped descendants and persistence uncertainty retain interrupted/unresolved handling. A deadline-terminated fenced worker exits after durable terminal spooling, without AwaitReconnect. Non-deadline reconnect episodes retain their existing residual wait. RequestExtension keeps its journaled policy but never changes an existing worker's pinned deadline; dynamic pause compensation is not established here.

The private stdin handoff fails closed in both mixed-version directions: the new decoder rejects old payloads without the deadline, and the old strict decoder rejects the new field. There is no inferred fallback deadline, automatic retry or replacement native launch. Already running workers retain their admitted episode and unchanged identity/spool evidence. Finish or deliberately detach/drain existing supervisors before executable replacement and use matching binaries for new launches. Replacing the executable between an old supervisor's admission and spawn is unsupported; this proposal does not establish v2 seamless upgrade qualification.

### Stop-less detach and canonical outcomes

Add `Hooks.Detach <-chan struct{}` (nil disables it) and `Outcome.DetachedRunning bool`. Consume detach only after worker acceptance while running; return without requesting stop, freezing a candidate or rewriting the active run/attempt, and release the per-run owner lock. Before acceptance, latch the request until acceptance or an earlier terminal outcome. During stopping, return the existing pending-stop Detached outcome without requesting stop again. Recovery continues ingestion under the same exclusive owner lock and never launches another attempt.

`RunningDetachedNotice` is exactly `detached from run %s; run is not complete; worker keeps running; reattach with: mythhelm recover %s`. DetachedRunning and pending-stop Detached both return client exit 6, but retain distinct running-versus-unconfirmed-stop guidance. Success-path continuation checks, including recover and demo, handle both outcomes. Linear callers do not supply the new hook, and their output/interrupt contract remains unchanged.

Deliberate detach is enabled only when the journaled admission capability `worker_detachment` is `supported`, never by GOOS inference. Unknown/unsupported disables it with `detach is not qualified on this platform`. Emergency second Ctrl-C and terminal loss still follow the defined detach policy, disclose the qualification limit and return 6; they do not certify Windows detach. No view read probes the exclusive owner lock. Without independently qualified non-locking evidence, stale/missing supervisor beats show supervision unknown. Fresh beat evidence is advisory; production never infers an absent owner from an unsuccessful observation.

### Typed intents, signals and action lifetime

TUI owns ClientMode (ModeInspect, ModeLive), ClientSignal (SignalInterrupt, SignalTerminalLost), ExitKind (ExitView, ExitDetach, ExitFinishHeadless, ExitUnresolvedAction), and ExitIntent with Kind and Phase, exactly as design §3. Config adds Mode, RequestStop, Detach, Signals and OnExit. The CLI consumes these types; TUI never imports CLI. Callbacks enqueue bounded, coalesced intents promptly and never wait for or signal processes. RequestStop forwards the owner's first Interrupt once; Detach forwards Hooks.Detach once. Returning from the view never synthesizes two interrupts or an implicit stop.

The CLI is the sole OS signal owner; Tea runs with WithoutSignalHandler. A joined signal pump routes external SIGINT and key Ctrl-C through the same per-client state machine. Unix SIGHUP/SIGTERM indicate terminal loss; input EOF does not. Windows interrupt handling must build without Unix-only signal constants. OnExit runs at most once for deliberate exits; startup failure emits no exit intent and joins the healthy pipeline headlessly without requesting stop.

The complete mode/state contract is requirements AC-6.12. q initially selects back; selection, direct keys, palette and confirmations share gates. Live first Ctrl-C requests stop and keeps the view until confirmation; a second detaches only after worker acceptance. Admission and pre-running attempts wait. Already stopping never forwards another stop. Verification/apply offer no worker stop: first Ctrl-C explains the phase and stays, second closes the view and joins the run headlessly with its normal outcome. Inspect owns no run; its stop uses Actions.Stop only where allowed. Interrupt counts persist across state transitions. Explicit double-quit during an in-flight cancellable action cancels then joins cleanup and reports unresolved rather than abandoning a goroutine.

Confirmed action lifetime is independent of rendering. Run owns and joins the action group, preserving an in-flight Stop, recover or apply after terminal loss in live and inspect modes. It starts no replacement action, keeps the existing owner lock through completion and reports the final action result through `Config.OnHeadlessAction func(ActionCompletion)`. ActionCompletion carries Kind (stop/recover/apply), StopState, Receipt, Recovery and Err as specified in design §3. A closed-once completion handle allows both view and shutdown to observe the same result. Unexpected parent cancellation explicitly cancels/joins as unresolved; it is not mistaken for terminal loss.

The maintainer's resolved Q1 (requirements AC-6.13, 2026-10-15) distinguishes the client result from canonical execution: terminal loss maps a successful outcome to client exit 6 while preserving the ledger/receipt; existing nonzero failures retain their codes. During verification/apply, join the existing phase. Before worker acceptance, continue headlessly until safe detach or an earlier outcome, applying the same mapping. Inspect action completion uses its actual final result, not an earlier pipeline outcome. StopState `stopped` permits confirmed-stop guidance; `stop_requested` permits only pending-stop guidance and recovery; missing/unknown state is unresolved even with nil Err. Both terminal-loss stop results return client 6 without rewriting the canonical run. Deliberate second Ctrl-C during verification/apply retains the normal run outcome, as AC-6.9 requires.

Live demo repository/state remain retained after either detach, uncertain join or unconfirmed writer termination. Recovery guidance binds the actual state root with the validated, shell-specific formatter in design §3; a bare command must not silently use caller state. Ordinary recover does not delete retained resources. Explicit operator cleanup waits for terminal/no-live-writer evidence. Linear demo behavior remains unchanged.

### Declarative theme and flag contract

Built-in IDs stay `dark` (Observatory) and `light` (Paper). FR-1's exact nine-token palettes are normative; the separate [TUI palette](../assets/brand/README.md#tui-palette) records values absent from the shared Colours table. Themes contain no code. `border` becomes a validated hex colour, rejecting the old rounded/ascii values. Remove `warning` and migrate all consumers atomically; unknown/missing keys and malformed borders wrap ErrInvalidTheme and name the key. Existing Load and BuiltIn signatures remain unchanged. User-loaded themes/keymaps and reset remain a later slice; this proposal adds no module.

Validate text, text_muted, attention and err at least 4.5:1 on both surfaces; ok at least 4.5:1 on surface only; focus at least 3:1 on both surfaces. Fail with the named pair and ratio. Focus colours one marker cell (▶, ◆ or working ●), never text runs; labels use text/bold, focused titles underline. OK never appears on raised overlay cards. Attention means needs you only, in the key-bar slot or its compact NEXT relocation. Border remains decorative, never alphanumeric content. Only overlays paint surface_raised; never paint surface or chip backgrounds. Colour suppression removes all colour SGR. Contrast assumes the terminal background matches the chosen theme; these constraints do not claim human accessibility qualification.

`run`, `demo` and `review` accept `--theme auto|dark|light`, default auto, validating before every launch branch; invalid values exit 2 naming flag and value, including piped output. Explicit choices win. The resolver reads the already cached background answer through an injected function exactly once only for an enabled colour TUI using auto, with nil answer defaulting dark. It makes zero calls for plain, accessible, JSONL, non-TTY stdout, TERM=dumb, NO_COLOR, colour never, explicit dark and explicit light. No new terminal query, timer or goroutine is added; the existing startup query may still wait five seconds. Toggle starts from the selected theme and survives resize/overlays.

The flag help is: `auto detects the terminal background on Linux and macOS outside tmux and screen; under tmux or screen it uses COLORFGBG and otherwise dark; on Windows it always selects dark, use --theme light for a light terminal`. Other non-Unix backends likewise default dark. Detection remains unqualified pending the real terminal matrix. A bounded late OSC 11 reply filter recognizes only alt-] prefixed replies (at most 64 runes, expiry on the next existing poll after 200 ms); malformed/overlong input releases the suffix as ordinary keys. It never globally suppresses action keys.

## Consequences and verification

This is still the per-run ownership slice, not service-owned attach/control. I06/I12/I18/I21 require finite worker execution and reconciliation before any retry; I11/I13 keep presentation declarative and credential-free. No theme, pane or view exit gains authority. Existing linear/accessibility/JSONL output is pinned by Task 1 before runtime changes.

These are future acceptance obligations, not passing test claims:

| Contract | Implementing task and required evidence |
|---|---|
| Pinned deadline, first-request race, mixed versions, recovery conclusion | Task 4: TestLaunchDeadlineRequired, TestMixedWorkerHandoffVersions, TestWorkerDeadlineWithoutSupervisor, TestDeadlineRequestPrecedence, TestDeadlineRecoveryConclusion and failure fixtures |
| Stop-less detach and retained ownership | Task 5: TestDetachLeavesRunning, TestDetachDuringStopping, TestDetachBeforeAcceptance |
| Conservative read-only supervision | Task 6: TestSupervisionClassification and TestViewPollingDoesNotOwnRun |
| Modes, one signal owner, joined actions and client exit distinction | Tasks 7–8 model/CLI tests; Task 10 packaged lifecycle and Unix terminal-loss tests |
| Demo retention and safe recovery destination | Tasks 9–10 malicious path/run-ID, actual PowerShell execution, retained-state recovery and explicit cleanup tests |
| Palette equality, theme validation and semantic colour | Task 11 equality against embedded themes; Tasks 12–16 state/glyph/render and parsed-SGR tests |
| Theme flag, cached answer and delayed reply | Tasks 17–18 counting seam, OSC input and packaged help/flag tests |

Task 3 records delegated acceptance of the identified concrete ADR before runtime implementation. G09, human screen-reader support, real light/dark terminal qualification, Windows detachment, service-owned lifecycle and non-deadline reconnect bounds are not established by accepting this proposal. Renderer microbenchmarks are not evidence of an advantage over native coding CLIs.
