## TUI Observatory View — Design

> Normative source: `mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md`, version 2.0. Implements the requirements in `specs/*/tui-observatory-view/`. This is an implementation plan, not evidence that the capability shipped. The maintainer approved the terminal-loss client/run distinction on 2026-10-15 (scratchpad Q1).

### 1. Current state and scope

Grounded against `origin/main` at `30062ec` on 2026-10-15. Finders below are repository-root commands; line numbers are hints.

| Fact | Opened source and finder |
|---|---|
| Demo unconditionally deletes its temporary repo/state and checks only Detached before review. | `internal/cli/demo.go:78,83,124`, `rg -n "RemoveAll|reviewOK =|MkdirTemp" internal/cli/demo.go`; recover resolves MYTHHELM_HOME in `internal/cli/recover.go:39` via `internal/statedir/statedir.go:19`. |
| Live TUI quit injects two interrupts, then prints review guidance. | `internal/cli/tui.go:226`, `rg -n 'func runLiveTUI|interrupts <-|reattach with' internal/cli/tui.go` |
| First interrupt stops; the second returns with `Outcome.Detached`. Deadline classification depends on volatile `deadlineHit`. | `internal/supervisor/pipeline.go:1037`, `rg -n 'deadlineHit|case <-p.h.Interrupt|func .*endStop' internal/supervisor/pipeline.go` |
| Worker launch is bounded stdin JSON, strictly decoded; worker owns the ladder and terminal spool reason. | `internal/workers/worker.go:74,251,656,765,913`, `rg -n '^type Launch|^func readLaunch|func .*supervise|func .*requestStop|func .*classify' internal/workers/worker.go` |
| A fenced worker waits after spooling under supervisor loss. | `internal/workers/worker.go:560`, `internal/workers/envelope.go:144`; `rg -n 'awaitIfUnsupervised|func AwaitReconnect' internal/workers/{worker,envelope}.go` |
| Owner observation cannot reuse Acquire: it creates and exclusively locks a file. | `internal/ownerlock/ownerlock.go:25`; `rg -n 'func Acquire|OpenFile' internal/ownerlock/ownerlock.go` |
| Config has no live intents, Ctrl-C quits immediately, q rows ignore selection. | `internal/tui/model.go:32`, `nav.go:23`, `dialogs.go:96`; `rg -n 'type Config|ctrl.c|case dialogExitOptions' internal/tui/{model,nav,dialogs}.go` |
| Snapshot has no usage, model or detachment capability. Load scopes native observations to the latest attempt. | `internal/tui/viewmodel/viewmodel.go:38,55,110`; `rg -n 'type Admission|type Snapshot|currentAttempt|decodeAdmission' internal/tui/viewmodel/viewmodel.go` |
| View returns overlays before drawing the mission; width ladder is already 140/100/80, height threshold 10. | `internal/tui/model.go:277`, `layout.go:9`; `rg -n 'func .*View|wideWidth|minHeight' internal/tui/{model,layout}.go` |
| Theme validates ten keys, border style and four text contrast pairs. Warning has real consumers. | `internal/tui/theme/theme.go:29,94`, `rg -n 'Tokens.Warning|t.Warning|Warning:' internal/tui` finds theme tests, mission, diff and actions tests as well as the schema. |
| Usage is already independently labelled; exact candidate checks already gate verified text. | `internal/journal/ledger.go:16`, `internal/tui/mission.go:16`; `rg -n 'type UsageRow|func verified' internal/journal/ledger.go internal/tui/mission.go` |

The issue premises still hold at this base: the exit options are inert; the caller couples UI exit to a stop; overlays erase the mission; theme startup always loads dark; usage observations are absent from Snapshot. Each has a failing test specified below. The current docs colour table already contains Paper and both ambers (`docs/assets/brand/README.md`, Colours); add a separate TUI table, not duplicate shared rows.

**Single spec, scope auto-confirmed (non-interactive).** OQ-21 is a confirmed maintainer override to keep lifecycle and visual work together. The 20 tasks remain one stream delivering this default view, with the lifecycle fixes landing first. Task count exceeds 14 because process changes, baseline captures, CLI wiring, TUI rendering and human evidence are distinct ownership/verification boundaries; each task is separately reviewable. Do not turn this into the deferred slices 2–4. Those and the approval bridge are not prerequisites.

### 2. Runtime ownership and deadline (v2 §§6.4, 7; I06, I12, I18, I21)

Task 1 captures linear-mode baselines **before** runtime edits. Task 2 proposes an ADR amending 0004/0005/0007 and recording the theme/flag contract; Task 3 is maintainer approval. No new journal event, launch identity schema, process owner or module is introduced.

Extend `workers.Launch` with `ExecutionDeadline time.Time` tagged `json:"execution_deadline"`. `readLaunch(io.Reader) (Launch, error)` rejects zero or malformed dates. The supervisor assigns its computed `p.deadline` immediately after `launchForAttempt`, with nonce/generation; no worker.json or attempt.launched field changes. Already expired deadlines must not start native execution. The deadline is pinned per launch; recovery observes it and never sends a replacement deadline into an existing worker.

The worker schedules one deadline timer outside the generation fence. Deadline expiry enters its existing `requestStop`, ladder and spool sequence. Introduce exported `workers.DeadlineRequester = "execution-deadline"` and `workers.DeadlineReason = "envelope_deadline_exceeded"`. The supervisor deadline writes that requester rather than `user-interrupt`. The worker maps it to DeadlineReason in its final state event; `attempt.stopped` keeps its schema. First-request precedence applies equally to the file poll and the timer: timer handling first inspects an existing stop.request, and uses its requester if present; otherwise atomically creates the deadline request through RequestStop and re-reads the winner. A user request already accepted by the worker also wins. Errors creating/reading the deadline request do not leave native execution unlimited: use the existing abort/ownership-unresolved path and preserve the error, never assert a confirmed deadline stop.

`pipeline.conclude` maps a confirmed stopped attempt with DeadlineReason to blocked/DeadlineReason, including recovery; remove `deadlineHit` and its endStop branch. Unconfirmed/escaped processes retain interrupted handling. A deadline-terminated fenced worker exits after durable terminal spooling rather than entering `AwaitReconnect`. This is the specific AC-6.11 exception to the general reconnect wait; non-deadline episodes retain that limitation. Keep RequestExtension's journaled policy; no extension mutates an already launched worker's pinned deadline.

Add `Hooks.Detach <-chan struct{}`. Nil is disabled. The owner consumes it only once a worker is accepted and running, returns without a stop or freeze, releases the owner lock and leaves the worker/attempt/run intact. A queued terminal-loss request before launch waits for that safe point; admission refusal or terminal completion wins naturally. Introduce `Outcome.DetachedRunning bool` and `RunningDetachedNotice = "detached from run %s; run is not complete; worker keeps running; reattach with: mythhelm recover %s"`. Keep `Outcome.Detached` and DetachedNotice for pending stop. When a Detach arrives in stopping it uses the existing pending-stop result, without a second stop request. All success-path early-return sites that currently check Detached must also check DetachedRunning, including recover's watch continuation. Linear callers never set Hooks.Detach and retain their text.

### 3. Typed client controls and signal ownership (v2 §15.2)

Keep `tui.Run(ctx context.Context, cfg Config) error` and existing `Actions` signatures. Add these TUI-owned contracts in `internal/tui/lifecycle.go` (CLI consumes them; no reverse import):

```go
type ClientMode uint8
const (
	ModeInspect ClientMode = iota
	ModeLive
)
type ClientSignal uint8
const (
	SignalInterrupt ClientSignal = iota
	SignalTerminalLost
)
type ExitKind uint8
const (
	ExitView ExitKind = iota
	ExitDetach
	ExitFinishHeadless
	ExitUnresolvedAction
)
type ExitIntent struct {
	Kind ExitKind
	Phase string
}
```

Extend Config with `Mode ClientMode`, `RequestStop func()`, `Detach func()`, `Signals <-chan ClientSignal` and `OnExit func(ExitIntent)`. Callbacks enqueue an intent and return promptly; they never signal or wait for a process. Nil live controls disable their action with a visible reason. `OnExit` is called at most once by the model before deliberate quit; unexpected program failure calls none. TUI consumes Signals through a joined pump like its journal pump and routes SignalInterrupt through the same state machine as a key Ctrl-C. SignalTerminalLost requests headless continuation/detach according to AC-6.13. The CLI owns OS signals and queues these typed inputs; `tea.WithoutSignalHandler()` prevents duplicate handling. Inspect wiring also owns its interrupt channel. Windows provides interrupt handling; Unix-only SIGHUP tests/files do not leak syscall constants into Windows builds. Never equate input EOF to a lost terminal.

The CLI owns a single bounded lifecycle mailbox and a goroutine that joins on pipeline completion. RequestStop is coalesced and sends the owner's first Interrupt once; Detach sends Hooks.Detach once. Neither callback sends two interrupts. CLI does not infer a stop from the view returning. The callback channels remain writable or cancellation-aware until runTUI and signal pumps have joined. Repeated keys cannot block the event loop or accumulate goroutines.

Implement AC-6.12 as one table/state machine, shared by dialog rows, direct keys, palette reasons and Ctrl-C. `ModeLive` is explicit for run/demo, `ModeInspect` for review. A live stop uses the owner intent; inspect stop uses Actions.Stop. Preserve the complete table, including prelaunch, worker acceptance, unsupported detachment, stopping, verifying/applying, terminal, interrupted and in-flight action rows. Global interrupt count does not reset on run-state transitions; action double-quit arming is separate. q during a cancellable action arms the warning; its next Ctrl-C cancels and reports unresolved. Each a/s/d/r confirmation starts on Cancel, checks its gate again on confirmation, and records exact apply effects. q initially focuses back; its detach row is itself the explicit selection (d opens a confirmation).

Once the view closes headless, the CLI keeps its own lifecycle coordinator alive: early terminal loss latches detach until worker acceptance; verifying/applying joins the existing pipeline. Startup failure has no OnExit and joins with the notice `run <run> continues in this process; inspect with: mythhelm review <run>`, no interrupt. Deliberate detach prints exactly one notice, exits 6 and never calls the old describe branch about an unconfirmed stop. Pending-stop detach prints DetachedNotice once. `runExit` in `internal/cli/exit.go:104` (finder: `rg -n "func runExit" internal/cli/exit.go`) remains the single result-to-exit mapper; inspect unresolved-action errors map to 6 without pretending to own the live run.

**Approved Q1:** terminal loss during a phase that eventually succeeds returns client exit 6, with the canonical run outcome preserved. Nonzero run failures retain their code. The maintainer approved this distinction on 2026-10-15; AC-6.13 now states the mapping explicitly. Deliberate second Ctrl-C during verifying/applying still uses AC-6.9's normal outcome.

**Demo lifetime and recovery destination (Task 9).** The demo-as-live requirement includes its temporary resources. Replace unconditional TUI-branch RemoveAll defers with explicit cleanup ownership: before any pipeline starts, setup failures may remove its owned temporary directories; once live supervision starts, retain both directories by default until the pipeline has joined and its outcome/evidence proves no active or uncertain writer. Both Detached and DetachedRunning retain repository, managed workspace, journal and spool, and skip demoReview. An unconfirmed stop, interrupted outcome, failed join or uncertain startup failure also retains them. Normal terminal completion may review then clean as before. The TUI-only banner must say temporary files are removed only after confirmed completion; detached paths say retained, never removed or done. Non-TUI demo output/cleanup behavior stays unchanged under AC-7.1.

The live-TUI notice is formatted in CLI using `recoveryCommand(stateDir, runID, goos string) (string, error)` after validating absolute state and run ID. Every TUI recovery hint binds the actual state directory; this is essential for demo, whose private state differs from caller MYTHHELM_HOME. On Linux/macOS emit `env 'MYTHHELM_HOME=<absolute-state>' mythhelm recover <run>` with standard POSIX single-quote escaping (an embedded apostrophe closes the quote, emits a quoted apostrophe, then reopens it). On Windows emit the PowerShell equivalent setting `$env:MYTHHELM_HOME` with single quotes doubled, followed by `mythhelm recover <run>`, explicitly labelled PowerShell. Formatting never executes the text. Detach notices preserve their running-versus-stop-pending meaning and appear once; adapt the UI's pipeline-notice presentation at the CLI boundary, not linear supervisor notice constants or their existing output. Startup-error review guidance binds the same state root. Tested malicious path content is inert, not stripped into another directory.

After the exact printed recover command finishes, retain evidence for inspection/export: ordinary recover must not delete a demo repository, candidate or receipt. Print the two retained paths and tell the operator to remove only those demo directories after recovery establishes a terminal outcome and no live writer; an interrupted/unresolved recovery is not permission to clean. No cleanup flag, marker-driven recursive delete or inferred user authority is added. Task 10's fixture simulates that explicit operator cleanup after proving process exit and terminal evidence, then checks unrelated caller state is untouched. Task 19 documents this procedure. Retained temporary storage is an explicit consequence, not a leak concealed behind a removed-afterwards notice.

### 4. Snapshot additions and supervision observation (v2 §§7.3, 15.1; I09, I23)

Extend `viewmodel.Admission` with `WorkerDetachment string` (journaled `capabilities.platform.worker_detachment`, empty -> unknown); preserve admission provenance and existing error behavior. Extend Snapshot with `Model string`, `Usage []journal.UsageRow`, `AgentEvents []journal.Event`, `AgentEventsOmitted int`, `Supervision string`. Model defaults unknown and comes only from the latest attempt.native_session model; stale attempt observations cannot overwrite it. Usage is read with `j.UsageObservations(ctx, runID)` without arithmetic or relabelling; read failure returns an error. AgentEvents retains the latest 200 journaled events for the current attempt in order, counting omitted rows; rendering selects meaningful state/denial/error sentences or the raw event stream from the same bounded tail. `Load` remains read-only and keeps its signature. Do not change EventsSince/EventsSinceLimit, used by accessible output.

Supervision is advisory: values `supervised`, `unsupervised`, `unknown`. The display threshold is one second (greater than workers.MaxSupervisorBeatAge, 300 ms). Read the existing beat using `workers.ReadSupervisorBeat(workers.AttemptDir(...))`; a readable fresh beat may display supervised, stale/missing/malformed evidence is unknown unless an independent no-lock observation proves the owner is not held. Never call ownerlock.Acquire from a viewer. At this slice's production boundary there is **no qualified no-lock probe**, so a stale beat displays `supervision unknown` on all supported platforms. Preserve the pure classification seam `classifySupervision(at, now time.Time, beatKnown bool, owner string) string`, with owner `held|unheld|unknown`, to test the required unsupervised rendering only when an actual unheld observation is supplied. Production passes unknown, never fabricates unheld. The stale/no-owner integration fixture is conditional on a qualified probe existing, exactly as AC-4.12 permits; do not claim it ran on this implementation. Fresh/stale/missing/held cases remain tested, and concurrent recover must never be refused because the viewer polled. Linux proc-lock absence is not accepted: its PID namespace filtering can hide a holder ([Linux man-pages, proc_locks(5)](https://www.man7.org/linux/man-pages/man5/proc_locks.5.html)).

Only inspect mode interprets Supervision; a live view follows its owner. An injected proven unsupervised observation freezes elapsed, suppresses live wording and gives `mythhelm recover <run>` plus needs you. A supervision unknown label makes no ownership assertion and grants no action. Current Snapshot.At drives elapsed at one-second resolution with no extra timer; journal UpdatedAt freezes settled or proven unsupervised runs. Clamp negative elapsed to zero, and format m:ss/h:mm:ss.

### 5. Theme and render contracts (v2 §§15.1, 16)

Use the exact nine-token palettes/contrast matrix in FR-1. Delete Warning atomically with all its Go consumers and test literals. Border becomes a colour. BuiltIn and Load retain their signatures and ErrInvalidTheme wrapping. Rules/separators can use border; only overlays paint SurfaceRaised; do not paint Surface. Add no dependency. Keep dark/light IDs and Observatory/Paper display names.

The current `line{text, colour}` cannot represent a single coloured marker plus bold text. Extend it compatibly with `spans []span`; add `type span struct { text, colour string; bold, underline bool }`. A row with spans uses them, otherwise retains its text/colour behavior. Add `renderSpans(c caps.Caps, spans []span, width int, background string) string`: fit plain cells before applying SGR, clip whole runes/markers, reset and restore card background between spans. Empty background means no painted surface. ColourNever emits no colour SGR, including background; emphasis may remain. Sanitize stored values before constructing spans; do not sanitize trusted SGR assembled by the renderer. Diff rows retain their existing bounded viewport rather than reparsing styled strings as untrusted input.

Focus is one cell only (▶, ◆ or working ●); labels use bold Text, focused titles also underline. OK never appears on raised cards; dimmed base/overlay verification text uses TextMuted/Text there. Attention appears solely in the needs-you slot, or its compact NEXT relocation. Unknown quantities are the literal underlined word. Hunk headers use TextMuted and @@; mismatch uses Err plus `mismatch`; truncation and other former warnings use TextMuted. New glyphs/fallbacks are ◆/#, ○/o, ◌/., ✓/v; keep the existing closed set and explicit rejection of U+2714/U+2705. State vocabulary is AC-3.4 verbatim, with verified(snapshot) guarding ready-for-review OK.

### 6. Layout and overlays (v2 §15.1; AT-37)

Use frames 1a/1f/1g/1h/1i as fixture sources, applying the Decisions overrides: no chip backgrounds, no per-action invented file/command, no x discard, accepted glyphs only, Cancel/back safety, no amber NOT PROVEN, and q always retained. Goldens are synthetic fixture outputs, not pasted claims of native work.

Four logical pane IDs: tasks, agent, evidence, diff. At >=140, Tasks/Agent/Evidence; Enter replaces the focused pane with Diff and Esc restores it. At 100–139, Tasks plus the focus-selected Agent/Evidence/Diff pane; at 80–99, four tabs with the same stable identities. In the settled states specified by AC-4.13, 80–139 uses verdict groups instead. Keep focus/task/dialog identity across rung changes. Pane widths at >=140: Tasks one fifth of usable width (minimum 24), Agent two fifths, Evidence remainder after two separator cells. At 100–139 Tasks one third, detail remainder after separator. All bodies are viewport bounded.

Header is one row, then rule, NOW/NEXT row, rule, body, rule and final key bar. Reserve chrome first; body clips/scrolls before mandatory chrome. At compact rung reserve header/now/next/use/key bar; extra space between use and final key bar adds verification, billing, entitlement and notices in that order. Terminals shorter than five rows prioritize header, next/attention, final key bar without claiming the five-row guarantee. NOW/NEXT widths split the available row in half; each trims independently and preserves its label. Header right-side identity is reserved only >=140; wordmark only >=100; state/time reserved before title. Use the requirement's key-drop thresholds and reverse-order removal, move attention to next when needed, and keep q plus compact offer. For physically narrower than their literal combined length, retain the full help/offer in the linear fallback; the acceptance widths start at 59 columns and the 5-row guarantee is tested at 72 columns. This physical lower bound is documented, never solved by writing past the terminal edge.

AGENT displays journaled identity/counters/sanitized sentences with a 200-event tail and older count. v toggles raw/summary without losing selection; both modes show native-reported provenance and never infer a file target. EVIDENCE displays admission/working/frozen/checked/ready with proof-sensitive glyphs, each usage observation separately, entitlement and candidate/check information. Verdict groups bind checks to the current candidate; mismatch never enters PROVEN. Unknown fields are exactly AC-4.13's list plus no-verification checks: not run. NEXT derives from the same action gates, not a second guessed table; failed/blocked never suggest gated TUI recovery.

Overlay cards fit only at >=100x20 and only if their content plus title/rule fits the body between persistent chrome. Otherwise show the one-line header and full-screen overlay. Center cards in the available body rectangle. Border uses only horizontal/vertical glyphs, no extra corners; cards carry title and rule so the low-contrast border is decorative. Recompose the default view on every snapshot update under the overlay, recolouring it TextMuted without faint. Apply overlay background to its rectangle only. Never mutate focus, pending effect identity or theme when composing/resizing.

### 7. Theme selection and late OSC input (v2 §15.1)

Add `theme.ParseChoice(value string) (string, error)` validating auto/dark/light and wrapping ErrUnknownTheme; add `tui.ResolveTheme(choice string, enabled bool, c caps.Caps, backgroundDark func() bool) (name string, tokens theme.Tokens, err error)`. Validate choice even when disabled; explicit choices bypass backgroundDark. Auto uses dark unless enabled and colour permitted, then reads the injected cached answer once. Nil answer defaults dark. Production passes lipgloss.HasDarkBackground; call only after selectLaunch. No new terminal query, goroutine or timer. CLI records ThemeName on Config so the first toggle moves away from the chosen theme. All three verbs share addTUIFlags and validate before branching. Help uses the exact AC-2.6 platform wording.

Add bounded OSC-reply filtering before key routing: only an alt-] prefix arms it; recognize 11;rgb: and up to three four-hex groups ending BEL or ESC-backslash, maximum 64 input runes, expiring on the next existing poll after 200 ms. Discard the matched sequence; malformed/overlong candidates release the buffered suffix as ordinary keys (the original alt prefix remains inert). No global d/r suppression. Test the exact delayed-reply sequence in AC-2.7 and the prefix-free control case.

### 8. Verification, documentation and ordering

Unit/model tests pin all states and modes; deterministic full-colour goldens parse SGR runs as well as visible cells. Eleven exact viewport sizes in AC-4.6, each unicode/ASCII, plus 160x10 and 72x5, cover boundaries. Full-colour palette/apply/diff/overlay captures cover semantic colouring. Existing no-font, resize, no-permanent-loop and viewport tests keep their effects; replace obsolete label assertions with the new semantic equivalents. Benchmarks `BenchmarkObservatoryView` and `BenchmarkObservatoryOverlay` use 10,000 diff lines and 200 retained events; record ns/op and allocations outside race, report median across ten runs on the same machine, target <=16 ms/frame but do not treat this machine-dependent number as an acceptance gate. Count rendered rows to enforce bounded key work instead. This is renderer evidence, not a native-CLI performance comparison.

Packaged-binary tests build cmd/mythhelm and use the credential-free fake, a test-only Python standard-library pty driver launched with an argv array (no shell; python3 must be present on Unix CI), explicit process synchronization and finite timeouts. They prove detach/no-stop/liveness/recover, deadline after detach, startup failure, SIGINT single delivery, and Unix HUP/TERM in every state. Use test seams for phases that need controlled blocking, but keep at least one actual binary path for each externally visible lifecycle. No native-account trial is required.

Ordering conflicts: `specs/*/windows-process-ownership/` edits worker.go, pipeline.go and recover.go; do not dispatch overlapping tasks concurrently. Rebase and revalidate launch fixtures/control signatures after its relevant task merges. `specs/*/claude-strict-subscription/` overlaps run.go; serialize CLI wiring/flags against those tasks. Contained-execution-profiles is unfinalized with code already merged; this design uses that code, and must follow any finalization fixes. Docs-site-restyle owns user-guide markup and brand shared table: docs tasks sequence after its open edits, preserve shared colour tests and regenerate demo poster if changed. No current todo/in-progress task claims TUI/theme implementation (finders: `rg -n 'internal/tui/|mods/themes/' specs/in-progress/*/tasks.md specs/todo/*/tasks.md`).

User-visible wording owners: Tasks 5/8 own runtime notices and exit description; Task 9 owns demo retention/recovery/cleanup wording; Task 7 owns dialog/key/help reasons; Task 12 owns state words; Tasks 13–15 own NOW/NEXT, evidence/agent/verdict/compact/key-bar copy; Task 16 owns overlay labels; Tasks 17/18 own theme/flag errors and help; Task 19 owns docs and palette table. Task 20 records demo artifacts manually. No result docs claim G09 or Windows detachment passed without its independent evidence.

### Decisions

| ID | Decision | Rationale |
|---|---|---|
| D1 | Single spec with lifecycle tasks first. | Maintainer OQ-21 overrides an epic split; bounded task PRs still isolate risk. |
| D2 | Worker pinned deadline plus existing owner timer. | A client timer alone disappears on detach; journaled reason survives recover. |
| D3 | Deadline terminal workers skip reconnect wait. | AC-6.11 specifically requires fenced workers to exit; other episodes retain the residual wait. |
| D4 | Separate DetachedRunning and Detach hook. | Reusing two interrupts changes execution and lies about a pending stop. |
| D5 | One CLI signal owner, typed TUI mailbox. | Bubble Tea plus CLI signal handlers could double-stop or exit before reconciliation. |
| D6 | Unknown supervision when owner cannot be observed without locking. | Explicit AC-4.12 fallback; flock probing could disrupt recover and proc absence is namespace-limited. |
| D7 | Span rows, no general background. | Whole-row foreground cannot keep Paper focus to one cell; requirements reject painted surface/chips. |
| D8 | Existing cached background answer, direct lipgloss import only. | A second OSC query adds latency; no new module allowed. |
| D9 | One-second stale display threshold. | More conservative than 300 ms worker threshold; advisory only, never authority. |
| D10 | Terminal-loss zero becomes client exit6, approved Q1. | Maintainer-approved distinction between canonical completed run and client uncertainty; AC-6.13 records it. |
| D11 | Human demo recording is a terminal task. | Agent-generated screenshots cannot claim human accessibility or terminal qualification. |
| D12 | Retain detached demo files until explicit operator cleanup after recovery. | Automatic recover deletion would destroy evidence and infer cleanup authority; private state needs an explicitly bound recovery command. |

### Honesty register

| Spec demand | Position |
|---|---|
| v2 §15.1 complete observatory UX | Slice 1 only. Per-action activity, stop ladder/recovery/receipt screens are slice 2; approval bridge/card is slice 3; fleet attention is outside this run view. |
| G09 / AT-36 / AT-39 | Experimental: human NVDA/VoiceOver/Orca, qualified light/dark matrix, declarative user theme/keymap/reset remain slice 4. |
| v2 §6.4 service-owned lifecycle | Deliberate per-run supervisor loss until issue #348; no alternate writer or ownership transfer on UI exit. |
| I14 Windows detach | Disabled unless admission records supported; emergency detach says not qualified and exits6. |
| AC-4.12 supervision evidence | No production unlocked observation qualified. Stale/missing beats show supervision unknown; no unsafe probe or false unsupervised claim. |
| v2 §15.1 contrast | Guaranteed only on matching terminal background; Paper focus is nontext one-cell only, OK excluded from overlays; decorative border ratios 2.58:1/2.20:1 are below 3:1. |
| v2 §15.1 terminal startup | Existing Bubble Tea init query can wait 5s, even linear modes; tmux/screen skip OSC, Windows defaults dark. This slice adds no query and claims no detection qualification. |
| v2 §15.2 controlled cancellation | Verifying/applying cannot be cancelled through worker stop; disable misleading actions, finish headless. Linear-path limitation remains. |
| v2 §6.4 fenced reconnect wait | Non-deadline episodes may wait indefinitely after spooling; deadline-expired episodes exit as required by AC-6.11. |
| I21 pinned execution deadline | Later pause spans/extensions cannot update a live worker's pin. A stale deadline fails conservatively; no claim of dynamic pause compensation. |
| Detached demo storage | Repository/state are retained for recovery and evidence inspection; explicit operator cleanup after terminal/no-live-writer proof is required. Ordinary recover never deletes them. |
| AC-6.13 terminal loss | Client non-success after terminal loss does not relabel a successful canonical run; approved Q1 and AC-6.13 specify the distinction. |
| v2 §15.1 arbitrarily tiny terminals | Literal q/offer cannot both fit below their combined width; never overflow; five-row acceptance uses a width that fits them. |
| I09 native activity and billing | Counters only, target unknown; usage rows never summed; model/exit/entitlement unknown remains explicit. |
