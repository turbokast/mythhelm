## TUI Observatory View — Tasks

### Dependencies

- Prerequisites: `specs/*/tui-slice/`, `specs/*/protected-acceptance/`, `specs/*/budget-ledger-s1/`, `specs/*/supervised-stop-recover/` and `specs/*/supervisor-service/` shipped. See design §8 for file serialization with Windows process ownership, Claude strict subscription, contained-execution finalization and docs-site-restyle. Windows explicit detach remains capability-gated.
- Q1 in scratchpad records the maintainer-approved AC-6.13 distinction. Task 3 still requires approval of the later concrete ADR; the clarification does not approve unwritten ADR content.
- Tasks 1 and 2 may run in parallel. All other tasks follow their explicit dependency chain; shared TUI/CLI files must never run concurrently. The lifecycle path (Tasks 1–10) precedes visual changes, per confirmed OQ-21. Tasks 19 and 20 remain after the mechanisms they describe.
- **Gates for every task**: format changed Go files with `gofmt -w`, then `scripts/harness/gate.sh go` for Go changes (vet, race tests, tidy-diff, configured lint); OS-specific changes also run Windows and Darwin vet. Run `scripts/harness/gate.sh hygiene` for all changes and `scripts/ci/lint-agent-harness.sh --only specs` for task bookkeeping. Use the gate names accepted by gate.sh and check marker freshness with gatelib.py. No new module; if a module change becomes necessary, stop and record the scope decision. Files existing or an agent reporting success is not completion.
- **Completion convention**: append ` ✅ COMPLETED` to the task heading and add Status (`✅ Completed — …; PR #n`), Implementation with commits, Spec deviations and Files modified, preserving original fields. Append discoveries and acceptance evidence to scratchpad and the task's handoff section; entry-check must pass. Red-first tests keep the failing counterfactual; no unearned acceptance by skipped test.
- **Commits**: signed off with `git commit -s`, named paths only, one isolated worktree/PR per task. Process, persistence and public contract decisions carry Task 2's approved ADR. No direct main pushes; review includes code-reviewer and architect across core/TUI ownership.

## Implementation Tasks

### Task 1 — Capture the unchanged linear output contract ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: None
- **Change**: Capture deterministic pre-change linear/accessibility/JSONL output from this task's base commit before runtime edits, with synthetic provenance and narrowly specified normalization.
- **Files**:
  - `internal/cli/observatory_baseline_test.go` (new)
  - `internal/cli/testdata/observatory-linear.json` (new bundle: named captures, synthetic fixture events and normalization rules)
- **Produces**: The immutable baseline bundle, keyed by review-accessible, review-jsonl, renderer-plain, renderer-accessible, run-plain, run-jsonl and demo-accessible; each capture records producing invocation, base commit and normalized bytes.
- **Acceptance**:
  - `TestObservatoryLinearBaseline` produces every nonempty case from fixed journal/event inputs and compares exact normalized bytes (AC-7.1). Changing one state word, provenance label, exit result or record field fails; normalization removes only run IDs, temporary paths, timestamps and the documented demo sampled-state nondeterminism, never semantic failures or usage values.
  - Inspect the fixture provenance: captures come from executing the unmodified base binary/renderers, not manually writing desired output; existing accessible_test.go/render_test.go/review_test.go assertions are unchanged.
- **Test plan**: Capture and compare twice at base first; then keep the baseline test in all later Go gates. Real paths/accounts are forbidden.
- **Invariants touched**: I09 (v2 §2: labels preserved); I13 (v2 §15.3: credential-free linear modes).

- **Status**: ✅ Completed — captured seven unchanged linear output contracts with synthetic provenance and semantic counterfactuals; PR #485.
- **Implementation**: Commit `a183347`; exact outputs come from two matching executions of unchanged base `e4ab1fc46fb08886af5e8195c4a2812b1b0b6e6f`. The complete Go race gate passed (288.6s); all six markers are fresh. Fresh independent review found no actionable findings; focused race repeat passed twice (11.307s).
- **Spec deviations**:
  - Coordinator-approved generated event/producer/attempt identity and role-bound PID normalisation preserves distinctness/references; generated Git references are replaced only after checking actual candidate changed paths and file content. No semantic values or arbitrary digests are removed.
  - Actual run plain/JSONL captures pin admission refusal (exit 2); richer fixed journal/render cases and successful demo cover lifecycle evidence. Demo uses `--after 1` and bounded slow-consumer timing to preserve events 2–23 exactly; fixed fixtures cover initial summary/created output. This does not fix the observed natural-timing receipt-tail race, documented in scratchpad for later CLI lifecycle work.
  - Own `tasks.md`, `handoff.md` and `scratchpad.md` record completion, downstream inputs and the discovered base race as required by the completion convention.
- **Files modified**: `internal/cli/observatory_baseline_test.go`, `internal/cli/testdata/observatory-linear.json`, `specs/in-progress/tui-observatory-view/tasks.md`, `specs/in-progress/tui-observatory-view/handoff.md`, `specs/in-progress/tui-observatory-view/scratchpad.md`.

### Task 2 — Record the lifecycle and presentation contracts

- **Domain/agent**: go-implementer (docs)
- **Budget**: standard
- **Depends on**: None
- **Change**: Draft the ADR amending process/stop/recover semantics and the theme/flag public contract; add the TUI-only palette table separately from the site-owned table. Keep proposed status until Task 3.
- **Files**:
  - `docs/decisions/0020-observatory-lifecycle-and-themes.md` (new; if occupied at implementation, reserve the next number and update this spec through review)
  - `docs/decisions/README.md`
  - `docs/assets/brand/README.md`
- **Produces**: Proposed ADR for pinned deadline, journal-derived conclusion, stop-less detach/exit6, typed intents, signal owner, theme schema and --theme; separate `## TUI palette` table with theme ID, token and hex columns.
- **Acceptance**:
  - Inspect ADR Decision section against FR-1/FR-2/FR-6: it names exact deadline/requester/outcome contracts, original ADRs 0004/0005/0007, platform limits, Q1's resolved policy and conservative supervision fallback. A record that says detach requests stop or deadline reason is memory-only fails review.
  - Inspect the TUI palette table: every missing TUI-only value in AC-1.10 is included and no existing shared Colours row is changed; Task 11 will execute the equality check against embedded themes.
- **Test plan**: Documentation review plus hygiene; no product decisions or approval ledger edits.
- **Invariants touched**: I06/I12/I18/I21 (v2 §§6.4,15.2: ownership and finite envelope); I11/I13 (v2 §16: declarative theme only).

### Task 3 — Approve the concrete ADR

- **Domain/agent**: maintainer
- **Budget**: standard
- **Depends on**: Task 2
- **Change**: Review and approve the proposed lifecycle/theme decision before runtime implementation, recording the maintainer's decision on the exact ADR revision.
- **Files**:
  - `docs/decisions/0020-observatory-lifecycle-and-themes.md`
- **Acceptance**:
  - The ADR has an explicit maintainer approval reference and accepted status; a worker assertion, preexisting OQ defaults or an unsigned product-state mutation is not approval. Any product change uses the existing signed approval scripts separately.
- **Test plan**: Inspect exact approved content and reference; no agent self-approval.
- **Invariants touched**: I03 (v2 §19: repository prose cannot grant authority).

### Task 4 — Make the worker own its execution deadline

- **Domain/agent**: go-implementer
- **Budget**: complex (process ownership, timing and durable conclusion; ten files are an atomic launch-contract migration including all actual spawn fixtures)
- **Depends on**: Task 1, Task 3
- **Change**: Add and enforce the pinned launch deadline regardless of generation, preserve first-request precedence, and conclude deadline stops from journaled reason in both owner and recover paths.
- **Files**:
  - `internal/workers/worker.go`
  - `internal/workers/worker_test.go`
  - `internal/workers/envelope_test.go`
  - `internal/supervisor/pipeline.go`
  - `internal/supervisor/recover.go`
  - `internal/supervisor/deadline_test.go` (new)
  - `internal/supervisor/faults_test.go`
  - `internal/supervisor/pinning_test.go`
  - `internal/admission/launch_fixture_test.go`
  - `internal/workers/deadline_test.go` (new)
- **Produces**: `workers.Launch.ExecutionDeadline time.Time` (`execution_deadline` JSON); `workers.DeadlineRequester` and `workers.DeadlineReason` string constants from design §2. Existing RequestStop/Spawn signatures unchanged.
- **Acceptance**:
  - `TestLaunchDeadlineRequired` rejects missing/zero/malformed handoffs; `TestExpiredDeadlineDoesNotLaunch` proves an expired launch starts no native (AC-6.11, I21). Enumerated actual spawn fixtures carry finite deadlines; helper-only containment/config Launch literals need no deadline because they do not call readLaunch.
  - `TestMixedWorkerHandoffVersions` submits a synthetic old payload lacking execution_deadline to the new readLaunch and a current payload to a fixture of the existing strict legacy decoder; both refuse before any native launch. Assert no fallback deadline or automatic retry, and preserve existing running-worker recovery tests with unchanged identity/spool data. Record the unsupported admission-to-spawn binary replacement window (design §2).
  - `TestWorkerDeadlineWithoutSupervisor` uses a two-second envelope in generation 0 and nonzero launches, observes requested/stopped/final reason in spool and worker process exit without any client. Removing the timer or leaving fenced AwaitReconnect active makes it fail.
  - `TestDeadlineRecoveryConclusion` ingests/recover after deadline and observes blocked/envelope_deadline_exceeded, not cancelled; `TestDeadlineRequestPrecedence` covers user-file-first, deadline-file-first, already-running user ladder and simultaneous timer/poll handling without a second ladder.
  - `TestDeadlineStopFailureStaysUnresolved` injects request-file failure and unconfirmed/escaped process reports, proving no false blocked/confirmed conclusion or unlimited native. Existing RequestExtension tests still pass.
- **Test plan**: Controlled fake session for branches; real worker process for client absence; bounded polling and cleanup, never sleep-only success. Rerun Task 1 fixture comparison.
- **Invariants touched**: I02/I06/I12/I18/I21 (v2 §§6.4,7: fail-closed launch, one ladder, durable reason, finite native episode).

### Task 5 — Add a stop-less owner detach control

- **Domain/agent**: go-implementer
- **Budget**: complex (process ownership and concurrency)
- **Depends on**: Task 4
- **Change**: Implement Hooks.Detach and the distinct DetachedRunning outcome without changing linear Interrupt behavior or freezing a still-running candidate.
- **Files**:
  - `internal/supervisor/pipeline.go`
  - `internal/supervisor/recover.go`
  - `internal/supervisor/detach_test.go` (new)
  - `internal/cli/exit.go`
  - `internal/cli/run.go`
  - `internal/cli/run_test.go`
- **Produces**: `supervisor.Hooks.Detach <-chan struct{}`, `supervisor.Outcome.DetachedRunning bool`, `supervisor.RunningDetachedNotice` as design §2; runExit/describe map deliberate detach to6 and distinguish pending stop.
- **Acceptance**:
  - `TestDetachLeavesRunning` sends Detach after accepted running, observes unchanged executing state, live worker and no stop request; source owner lock can then be acquired by Recover. Mutating detach to send Interrupt fails.
  - `TestDetachDuringStopping` observes existing Detached notice/outcome with no duplicate stop; `TestDetachBeforeAcceptance` queues without returning until worker acceptance or prior terminal outcome.
  - `TestDetachedOutcomeDescription` distinguishes running from unconfirmed-stop wording and maps both to6. Existing first/second interrupt tests and Task 1 fixtures remain unchanged.
- **Test plan**: Real owner/worker fixture, count native launches before/after Recover; no new attempt and no early freeze.
- **Invariants touched**: I05/I06/I08/I12/I18/I21 (v2 §§6.4,15.2: preserve original source, one writer and worker deadline).

### Task 6 — Add journal-backed observatory data

- **Domain/agent**: tui-implementer
- **Budget**: complex (journal projection and bounded evidence)
- **Depends on**: Task 5
- **Change**: Extend Snapshot with model, usage, bounded agent events and conservative supervision; decode admitted detachment capability without GOOS guesses.
- **Files**:
  - `internal/tui/viewmodel/viewmodel.go`
  - `internal/tui/viewmodel/viewmodel_test.go`
  - `internal/tui/viewmodel/supervision.go` (new)
  - `internal/tui/viewmodel/supervision_test.go` (new)
- **Produces**: Fields and `classifySupervision(at, now time.Time, beatKnown bool, owner string) string` in design §4; existing Load and event-reader signatures unchanged. Capability and absent model default unknown.
- **Acceptance**:
  - `TestObservatorySnapshotEvidence` loads native-session, all five provenance labels, reported zero, unknown quantity, repeated scope rows, no rows and malformed durable data. Latest-attempt scope is preserved; a stale prior model never leaks into the current lane (AC-4.1/4.5, AC-6.10).
  - `TestAgentEventTailBound` retains exactly latest 200 of 205 current-attempt events and omitted count5, preserving order; a previous attempt is excluded.
  - `TestSupervisionClassification` distinguishes fresh/stale/missing/held/unheld/unknown using the one-second threshold. Production stale evidence is unknown; the pure unheld fixture proves only conditional rendering capability, not an OS qualification.
  - `TestViewPollingDoesNotOwnRun` polls during real owner acquisition/recover, checks no owner.lock creation or lock interference and no file/journal changes from viewmodel reads (AC-4.12).
- **Test plan**: Synthetic journal and beat fixtures; existing accessible consumers still use unchanged readers. Treat I/O errors as errors/unknown, not zero evidence.
- **Invariants touched**: I05/I07/I09/I12/I14/I23 (v2 §§6.4,7.3,15.1: read-only evidence, missing is unknown).

### Task 7 — Implement safe exit rows and typed TUI controls

- **Domain/agent**: tui-implementer
- **Budget**: complex (one lifecycle state machine; ten files include existing tests that must change with control behavior)
- **Depends on**: Task 6
- **Change**: Implement Config intents, per-session Ctrl-C handling, selectable q rows, direct a/s/d/r/e keys and consistent palette/help gates from the complete mode table.
- **Files**:
  - `internal/tui/lifecycle.go` (new)
  - `internal/tui/lifecycle_test.go` (new)
  - `internal/tui/model.go`
  - `internal/tui/nav.go`
  - `internal/tui/dialogs.go`
  - `internal/tui/actions.go`
  - `internal/tui/palette.go`
  - `internal/tui/help.go`
  - `internal/tui/actions_test.go`
  - `internal/tui/nav_test.go`
- **Produces**: ClientMode, ClientSignal, ExitKind, ExitIntent, ActionCompletion, Config.OnHeadlessAction and other new Config fields, with exact signatures/values from design §3. Run signature unchanged; joined signal pump and WithoutSignalHandler are enabled.
- **Acceptance**:
  - `TestExitModeTable` drives every AC-6.12 row, every listed key and disabled reason; q initially back, Enter cannot accidentally detach/stop, navigation selects and acts, and capability unknown/unsupported disables deliberate detach independent of runtime.GOOS.
  - `TestLiveCtrlCIntents` asserts one typed stop, view stays, second press detaches; verifying/applying emit zero stop calls, show phase then exit headless; admission waits; worker-not-running waits. Counters persist across state transitions (AC-6.5/6.9/6.14).
  - `TestInFlightActionDoubleQuit` proves first q/Ctrl-C leaves action alive and second Ctrl-C cancels its context with unresolved exit intent (AC-6.8); replace old one-press expectation, not the underlying cancellation assertion.
  - Existing `TestSingleKeypressNeverConfirms`, `TestApplyShowsExactEffects` remain unchanged; `TestStopLabelsRequested` preserves all effects except specified reason/hint changes. `TestLayeringNoCliImport` rejects any TUI import of CLI.
  - `TestTerminalLossJoinsApply` starts a slow confirmed apply in live and inspect modes, signals terminal loss, verifies view pumps close but action context is not cancelled, releases the operation and observes one OnHeadlessAction result and a joined action before Run returns. The old unconditional final cancelAction fails this test. `TestExplicitActionQuitCancelsAndJoins` retains AC-6.8 cancellation on deliberate double-quit and proves no surviving action goroutine.
  - `TestHeadlessStopCompletionState` preserves both Actions.Stop returns (stopped,nil and stop_requested,nil) through ActionCompletion.StopState; empty/unrecognized state is unresolved. A nil error with stop_requested must not render confirmed.
  - `TestLifecyclePumpsJoin` floods repeat controls while owner finishes and asserts no blocked event loop, duplicate callbacks or surviving pump.
- **Test plan**: Callback counters plus cancel-aware action fakes; table/state fixtures include no snapshot and nil controls. UI-only tests need no packaged process, covered in Task 10.
- **Invariants touched**: I03/I06/I18/I23 (v2 §§6.4,15.2: explicit confirmation, one signal/input owner, truthful stop).

### Task 8 — Wire live and inspect lifecycle in the CLI

- **Domain/agent**: go-implementer
- **Budget**: complex (signals and goroutine ownership across platforms)
- **Depends on**: Task 7
- **Change**: Replace synthetic quit interrupts with the typed coordinator, wire sole signal ownership and preserve startup-failure/headless outcomes and exact recovery notices.
- **Files**:
  - `internal/cli/tui.go`
  - `internal/cli/tui_test.go`
  - `internal/cli/tui_lifecycle.go` (new)
  - `internal/cli/tui_signals_unix.go` (new, matching build constraints)
  - `internal/cli/tui_signals_windows.go` (new)
  - `internal/cli/tui_signals_other.go` (new fallback)
  - `internal/cli/tui_lifecycle_test.go` (new)
  - `internal/cli/exit.go`
- **Produces**: CLI implementation of Config.RequestStop/Detach/OnExit/Signals/OnHeadlessAction, with one owned mailbox and outcome mapping, no signature change to tuiDeps.runTUI.
- **Acceptance**:
  - `TestTUIStartupFailureJoinsWithoutStop` injects immediate program error, counts zero interrupts/detaches, waits for owner outcome and checks exact continues/review notice (AC-6.6).
  - `TestLiveTUIIntentOutcomes` replaces TestQuitLiveDetaches, asserts one exact detach/recover line and code6 without review guidance, separate pending-stop text, and no fabricated stop on plain view return.
  - `TestTerminalLossStateTable` drives every AC-6.13 state including prelaunch/acceptance, verifies deferred detach, headless phase finish, no orphan created/admission state, and Q1's approved exit policy.
  - `TestTerminalLossApplyOutcome` uses a real guarded ApplyRun with a controlled in-flight seam in both live and inspect wiring; terminal loss must not cancel its context or run a second apply. Assert final journal/ref/receipt outcome, released owner lock, joined call and client6 on success (original run pipeline result cannot substitute). A failed apply preserves its nonzero failure; explicit action double-quit still cancels/joins and returns unresolved6.
  - `TestInspectUnresolvedExit` covers pending stop and cancelled recover/apply ->6 with recover line; ordinary inspect quit ->0. `TestSignalSingleOwner` injects one SIGINT and counts one typed stop delivery.
  - `TestInspectStopTerminalLoss` drives stopped,nil versus stop_requested,nil through a headless joined inspect stop; both exit6 after terminal loss but only the former reports confirmed, and the latter prints the state-bound recovery hint. Empty/unknown StopState or error remains unresolved. Verify canonical run state is not rewritten by this output mapping.
  - `TestCoordinatorJoins` races view error, terminal loss and owner completion, asserting one disposition, finite mailbox consumption and no leaked waiter. Task 1 fixtures remain byte-identical.
- **Test plan**: Unit coordinator seams for phase transitions; platform files build under Linux/macOS/Windows; real signal/worker proof follows Task 10.
- **Invariants touched**: I06/I12/I18/I21 (v2 §§6.4,15.2: no implied cancellation/ownership transfer, joined goroutines).

### Task 9 — Preserve detached demo state and usable recovery guidance

- **Domain/agent**: go-implementer
- **Budget**: complex (temporary workspace ownership and cross-platform recovery-command formatting)
- **Depends on**: Task 8
- **Change**: Make demo's live branch retain its private repository/state whenever ownership remains active or uncertain, skip premature review for both detach outcomes and print recovery instructions bound to the retained state root. Ordinary recover preserves evidence; later cleanup is explicitly the operator's action.
- **Files**:
  - `internal/cli/demo.go`
  - `internal/cli/demo_test.go`
  - `internal/cli/demo_lifecycle.go` (new)
  - `internal/cli/demo_lifecycle_test.go` (new)
  - `internal/cli/tui.go`
  - `internal/cli/tui_lifecycle_test.go`
- **Produces**: `recoveryCommand(stateDir, runID, goos string) (string, error)` in demo_lifecycle.go, used by live TUI guidance for an explicit state root; invalid nonabsolute/control-bearing state or run ID outside `^run_[A-Za-z0-9_]{1,128}$` returns an error. Both the state assignment and complete run-ID argument are shell-quoted independently. Design §3 defines platform quoting and retention lifetime. No recover signature, cleanup flag or automatic deletion authority is added.
- **Acceptance**:
  - `TestDemoDetachRetainsWorkspace` injects DetachedRunning and Detached outcomes, asserts code6, no demoReview invocation and neither temporary repository nor state removed; active/unresolved outcomes or startup/coordinator failure after launch also retain both. A normal completed live demo cleans up only after owner join and review. The current unconditional RemoveAll defers and reviewOK=!Detached fail these assertions (AC-6.12 demo row).
  - `TestDemoRecoveryCommand` supplies an unrelated caller MYTHHELM_HOME and retained paths containing spaces, apostrophes and shell metacharacters, checks exact quoting/state binding and that the printed recovery command cannot execute path content. Reject run IDs carrying semicolons, dollar substitution, backticks, quotes, whitespace/newlines or only the run_ prefix; check independently quoted accepted run IDs in explicitly labelled POSIX sh/bash/zsh and PowerShell output. PowerShell cases cover U+0027/U+2018/U+2019/U+201A/U+201B individually, mixtures, and a malicious path using a smart quote followed by an injection attempt; every recognized single-quote code point is doubled without normalization. Unix hints must not claim fish or generic-shell compatibility. Both runID and state root are explicit; an invalid input errors rather than emitting an unsafe command.
  - `TestPowerShellRecoveryCommandExecution` runs the exact generated command using pwsh -NoProfile -NonInteractive in a controlled Windows CI fixture for all five quote characters and their mixtures; a stub mythhelm records the actual MYTHHELM_HOME and argv, and a synthetic malicious suffix would create a sentinel if interpreted. Assert exact original state/run values and no sentinel creation. This executable test is required on the Windows runner; absent pwsh there is a failure, not a skipped qualification. Pure formatting cases run on every OS. `TestRecoveryHintShellLabel` checks POSIX sh/bash/zsh versus PowerShell labels and manual guidance for other shells.
  - `TestDemoRetentionNotice` checks explicit retained-repository/state paths, absence of premature removed/done claims, one usable recovery command and instructions to retain evidence until recovery proves terminal with no live writer. Recovery never silently deletes evidence. Non-TUI demo output and Task1 captures remain unchanged.
- **Test plan**: Inject run results and cleanup/review seams with invocation counters. Test every cleanup decision with actual temporary directory sentinels. Packaged proof and exact command execution are owned by Task10; operator cleanup is tested only after proven termination, on the test's owned fixture directories.
- **Invariants touched**: I05/I06/I08/I09/I12/I18/I21 (v2 §§6.4,15.2: retain live workspace and evidence, correct recovery destination, explicit cleanup authority).

### Task 10 — Prove lifecycle through the packaged binary

- **Domain/agent**: go-implementer
- **Budget**: complex (PTY, real processes, signals and recovery)
- **Depends on**: Task 9
- **Change**: Exercise externally visible lifecycle guarantees with a paced fake and bounded real-process tests before visual work changes the terminal.
- **Files**:
  - `tests/e2e/tui_test.go`
  - `tests/e2e/demo_test.go`
  - `tests/e2e/tui_lifecycle_unix_test.go` (new)
  - `tests/e2e/tui_deadline_test.go` (new)
  - `tests/e2e/main_test.go`
  - `tests/e2e/tui_pty.py` (new test-only standard-library driver)
- **Acceptance**:
  - `TestTUIDetachRecoverBinary` uses q/d confirmations, verifies exited client6 and exact recover command, live recorded worker and no attempt.stop_requested, then executes that printed command and proves no second native launch (AC-6.2/6.7).
  - `TestTUIStopDetachRecoverBinary` confirms stop then detaches while confirmation is pending, runs recover against stopping and observes truthful final state. `TestTUISIGINTOnceBinary` sends OS SIGINT and observes exactly one stop request.
  - `TestTUITerminalLossBinary` sends SIGHUP and SIGTERM on Linux/macOS while each early/running/stopping/verifying/applying state is synchronized, checks no extra stop, worker liveness when appropriate, no stranded early states and approved exit codes.
  - `TestDetachedDeadlineRecoveryBinary` uses two-second envelopes with fenced/unfenced worker, waits for durable reason and worker exit, runs recover and checks blocked/envelope_deadline_exceeded; user-stop-first remains cancelled.
  - `TestDemoDetachRecoverBinary` runs a paced demo through the PTY under a temporary root containing spaces and an apostrophe; detaches, observes client6, live native/worker and intact repository, state, journal and spool, with no stop or premature review. Extract and execute the exact printed recovery command in a controlled environment with an unrelated MYTHHELM_HOME; assert it recovers the same run without a second native launch and leaves terminal evidence available. Then the test, acting as the explicit operator and only after proving no live writer, removes its own two retained directories and asserts only those directories were removed. A missing state prefix or unconditional demo cleanup fails before recovery.
- **Test plan**: Build cmd/mythhelm through the existing e2e harness, synthetic repo with spaces, paced fake and slow verifier helper; poll named journal/process conditions, keep finite cleanup. Unsupported Unix-only tests are explicitly platform-tagged; Windows runs portable state-machine tests and never claims qualified detach.
- **Invariants touched**: I05/I06/I08/I12/I14/I18/I21 (v2 §§6.4,15.2: real owner continuity and finite execution, no source mutation).

### Task 11 — Migrate theme tokens and contrast validation

- **Domain/agent**: tui-implementer
- **Budget**: complex (eight coordinated schema/consumer files)
- **Depends on**: Task 10, Task 2
- **Change**: Install Observatory/Paper palettes, validate border colour and all contrast pairs, remove Warning with its compile-time consumers, and check the separate docs palette table.
- **Files**:
  - `internal/tui/theme/theme.go`
  - `internal/tui/theme/theme_test.go`
  - `mods/themes/dark.toml`
  - `mods/themes/light.toml`
  - `internal/tui/mission.go`
  - `internal/tui/diff.go`
  - `internal/tui/actions_test.go`
  - `internal/tui/theme/palette_docs_test.go` (new)
- **Produces**: Nine-key Tokens schema; unchanged BuiltIn/Load signatures and error sentinels; exact FR-1 colours and validation pairs.
- **Acceptance**:
  - `TestObservatoryPaperTokens` compares every palette field; `TestThemeContrastMatrix` rejects each failing required pair and names it with ratio/minimum, including attention3.0 and focus2.0 fixtures (AC-1.1/1.4).
  - `TestThemeBorderAndKeys` rejects rounded/ascii, malformed/missing border and every unknown/missing key including warning, with ErrInvalidTheme wrapping (AC-1.2/1.3/1.9).
  - `TestTUIPaletteDocsMatch` parses nonempty TUI palette rows and existing shared rows and compares exact embedded token values; duplicate/incomplete mappings fail (AC-1.10). Do not edit the site-owned shared table.
- **Test plan**: Kept invalid TOML/contrast fixtures in test literals; old schema tests adapted, no dropped failure case. Required consumers compile atomically; semantic SGR checks follow Task 16.
- **Invariants touched**: I11/I13 (v2 §16: strict declarative schema); I09 (v2 §15.1: warning replacements keep their text).

### Task 12 — Add semantic spans and the closed glyph vocabulary

- **Domain/agent**: tui-implementer
- **Budget**: complex (render representation shared by all panes)
- **Depends on**: Task 11
- **Change**: Add cell-safe mixed-style spans, the four approved glyphs and exact state vocabulary, preserving existing row rendering while restricting focus to one-cell markers.
- **Files**:
  - `internal/tui/styles.go`
  - `internal/tui/model.go`
  - `internal/tui/mission.go`
  - `internal/tui/diff.go`
  - `internal/tui/model_test.go`
  - `internal/tui/styles_test.go` (new)
  - `internal/tui/diff_test.go`
- **Produces**: `span`, line.spans and `renderSpans(c caps.Caps, spans []span, width int, background string) string` (design §5); glyph helpers and proof-sensitive run state presentation used by subsequent panes.
- **Acceptance**:
  - `TestObservatoryStateVocabulary` enumerates every RunState in supervisor/state.go against AC-3.4, verifies pairwise text distinction without colour, and denies OK ready-for-review without matching passed verification.
  - `TestObservatoryGlyphFallbacks` checks new ASCII #/o/./v differ from old fallbacks; `TestNoFontDependentGlyphs` permits exactly new runes and still rejects U+2714/U+2705 (AC-3.1–3.5).
  - `TestSpanCellBounds` covers wide runes, combining text, zero/one-cell widths and SGR resets/background restoration; every focus span is one cell, ColourNever has no colour escapes, malformed token never emits an escape (AC-1.5/1.8).
  - `TestDiffSemanticColours` checks hunk headers TextMuted, truncation TextMuted and mismatch Err plus word mismatch, with no attention use in diff (AC-1.7/1.9).
- **Test plan**: Deterministic widths and token parsing; retain diff viewport behavior. Renderer-only tests are adequate here; final integration captures follow Tasks 14/15.
- **Invariants touched**: I07/I09/I13 (v2 §15.1: proof-sensitive glyphs, font-independent text); I03 (v2 §16: untrusted terminal controls inert).

### Task 13 — Render the header and gated NOW/NEXT actions

- **Domain/agent**: tui-implementer
- **Budget**: standard
- **Depends on**: Task 12
- **Change**: Build shared header, elapsed clock, NOW/NEXT and key-bar content from recorded evidence and the lifecycle gates, retaining truthful unknown target and supervision wording.
- **Files**:
  - `internal/tui/observatory.go` (new)
  - `internal/tui/observatory_test.go` (new)
  - `internal/tui/mission.go`
  - `internal/tui/live.go`
  - `internal/tui/motion.go`
- **Produces**: `func (m *Model) observatoryHeader(width int) line`, `func (m *Model) nowNext(width int) []line`, `func (m *Model) observatoryKeys(width int, compact bool) []line`; key renderer returns an optional attention relocation row before the final key row, consumed by Task 15.
- **Acceptance**:
  - `TestHeaderIdentityAndElapsed` checks title/model/profile disclosure, unknown model, width-specific wordmark/identity, run ID in task row, m:ss/h:mm:ss and freeze at UpdatedAt in settled/unsupervised states (AC-4.1).
  - `TestNowNextActionGates` covers every run state and live/inspect mode; mentions only enabled TUI actions, stopping never says stopped, blocked/failed never suggest gated r recover, and counters never invent file/command (AC-4.2/4.3).
  - `TestObservatoryKeySlots` checks relative order and state gates, bold key/TextMuted label, no chip background, attention only for required states, required drop order and q retention (AC-4.8/4.11).
  - `TestClockAndAttentionTruth` proves no timer added, one-second displayed granularity, no progress percentage/countdown without denominator, unknown supervision never becomes unsupervised and a conditional proven-unsupervised fixture names CLI recover (AC-4.12, AC-7.2/7.3).
- **Test plan**: Fixed times and snapshots, same action gates as Task 7; existing NoPermanentLoop remains green.
- **Invariants touched**: I01/I06/I07/I09/I13 (v2 §§7.3,15.1: identity, state, truthful activity and elapsed evidence).

### Task 14 — Render agent, evidence and verdict content

- **Domain/agent**: tui-implementer
- **Budget**: complex (six files integrate pane rendering and model/navigation state)
- **Depends on**: Task 13
- **Change**: Add bounded agent summary/raw toggle, provenance-preserving evidence/stages and proof-sensitive verdict groups.
- **Files**:
  - `internal/tui/observatory_panes.go` (new)
  - `internal/tui/observatory_panes_test.go` (new)
  - `internal/tui/model.go`
  - `internal/tui/mission.go`
  - `internal/tui/motion.go`
  - `internal/tui/nav.go`
- **Produces**: `func (m *Model) observatoryAgent(width int) []line`, `func (m *Model) observatoryEvidence(width int) []line`, `func (m *Model) observatoryVerdict(width int) []line`; model raw-stream toggle state used by nav v.
- **Acceptance**:
  - `TestAgentSummaryAndRaw` renders state/permission/error events, counters and identity, tests 200-row cap/older count, and verifies escape payload inert in both modes; v changes content and second v restores it (AC-4.4).
  - `TestUsageProvenanceRows` distinguishes reported0, unknown, estimated, every label, repeated scope and no rows; no aggregation; retail-equivalent retains estimate-not-charge wording (AC-4.5).
  - `TestEvidenceStageProof` denies checked tick for failed/not_run or mismatched candidate, preserves check/candidate/mismatch information, and pins stage source events (AC-4.5, I07).
  - `TestVerdictGroups` covers passed+failed+unknown, all-passed, cancelled-before-launch and mismatched candidate; no empty group and no invented Windows-path limitations (AC-4.13).
- **Test plan**: Synthetic journal-shaped fixtures independent of renderer output, with malicious control strings and absent/null values. Count rendered rows, not speed.
- **Invariants touched**: I03/I07/I09/I15 (v2 §§7.3,15.1: no terminal injection, native result distinct from checks, no invented billing).

### Task 15 — Install all responsive ladder views

- **Domain/agent**: tui-implementer
- **Budget**: complex (shared layout integration; eleven files include four existing suites with old label contracts)
- **Depends on**: Task 14
- **Change**: Compose the new header/content/key bar on all rungs, preserve stable selection/focus/dialogs and replace obsolete labels with equivalent semantic assertions.
- **Files**:
  - `internal/tui/model.go`
  - `internal/tui/layout.go`
  - `internal/tui/mission.go`
  - `internal/tui/nav.go`
  - `internal/tui/help.go`
  - `internal/tui/layout_test.go`
  - `internal/tui/model_test.go`
  - `internal/tui/nav_test.go`
  - `internal/tui/motion_test.go`
  - `internal/tui/actions_test.go`
  - `internal/tui/observatory_goldens_test.go` (new; embedded expected frames, no new testdata directory)
- **Acceptance**:
  - `TestObservatoryLadderGoldens` derives synthetic expected captures from frames 1a/1f/1g/1h/1i plus Decisions overrides and pins all eleven AC-4.6 sizes in unicode/ASCII. Every line lipgloss.Width <= terminal; 100x24 Paper covers verdict groups, compact blocked fixtures show needs you and linear offer (NFR-1).
  - `TestObservatoryHeightBoundary` asserts three panes at160x10, compact at160x9, and complete five-row compact chrome at72x5; key bar is always last (AC-4.6/4.7/4.10).
  - `TestResizePreservesIdentity` extends existing assertions through all rung boundaries and four tabs, Enter-Diff/Esc, selected task and open-dialog identity (AC-4.9).
  - `TestHiddenKeysStillWork` at88x24 invokes : and / despite omitted key-bar labels; help lists them, closed actions remain visibly disabled, state-changing letters still confirm (AC-4.8/4.11).
  - Existing `TestViewportVirtualises` and `TestNoPermanentLoop` still assert bounded rows and settled motion; old AGENTS/SELECTED CHANGE/Next action assertions are replaced with equivalent new text, never simply removed.
- **Test plan**: Fixed time/snapshot fixtures. Embedded goldens are expected literals independently reviewed against source frames; no auto-accepting generator. Rendered-row counters for 10,000-line diff.
- **Invariants touched**: I03/I06/I07/I09/I13 (v2 §15.1, AT-37: stable actions, bounded rendering, truthful content and closed glyph set).

### Task 16 — Compose overlays with semantic colour checks

- **Domain/agent**: tui-implementer
- **Budget**: complex (shared compositor and SGR validation)
- **Depends on**: Task 15
- **Change**: Draw raised cards over an updating dimmed mission at sufficient size, retain header in fallback and enforce final per-span colour constraints.
- **Files**:
  - `internal/tui/overlays.go` (new)
  - `internal/tui/overlays_test.go` (new)
  - `internal/tui/model.go`
  - `internal/tui/dialogs.go`
  - `internal/tui/palette.go`
  - `internal/tui/live.go`
  - `internal/tui/motion.go`
  - `internal/tui/observatory_bench_test.go` (new)
- **Acceptance**:
  - `TestObservatoryOverlayCards` at140x40 retains header/NOW/NEXT/key bar under help/palette/exit/apply cards; SurfaceRaised only in card, no other background, border glyphs only; small fallback retains one-line title/state (AC-5.1/5.2).
  - `TestOverlayKeepsUpdating` journals a state change with help open and asserts dimmed header changes without closing/changing overlay; resize through fallback preserves pending effect and theme (AC-5.4/4.9).
  - `TestSemanticSGRRuns` parses both themes' default160x40/88x24, palette/apply/diff and overlay captures: focus exactly one cell, OK absent from overlay, border has no alphanumeric text, Attention only attention slot/relocated marker, unknown underlined, no colour under suppression (AC-1.5–1.9).
  - Existing confirmation tests retain their specified effects (AC-5.3). `BenchmarkObservatoryView` and `BenchmarkObservatoryOverlay` report ten non-race samples with ns/op/allocations and median; record hardware-neutral method and local target assessment, never a statistical native-CLI claim (NFR-2).
- **Test plan**: Golden cells and parsed SGR, both modes/themes; one benign fake live event is the positive witness for background updates. Keep benchmark results distinct from acceptance gates.
- **Invariants touched**: I03/I07/I09/I13 (v2 §§15.1,16: explicit effect safety, contrast and evidence remain readable).

### Task 17 — Resolve themes and suppress delayed OSC replies

- **Domain/agent**: tui-implementer
- **Budget**: complex (six files; spec-decomposition sizing rule)
- **Depends on**: Task 16
- **Change**: Resolve auto/dark/light from the cached injected answer with zero extra terminal queries and filter only bounded OSC11-shaped delayed replies.
- **Files**:
  - `internal/tui/theme/choice.go` (new)
  - `internal/tui/theme_choice.go` (new)
  - `internal/tui/theme_choice_test.go` (new)
  - `internal/tui/nav.go`
  - `internal/tui/osc_reply.go` (new)
  - `internal/tui/osc_reply_test.go` (new)
- **Produces**: `theme.ParseChoice(value string) (string, error)` and `tui.ResolveTheme(choice string, enabled bool, c caps.Caps, backgroundDark func() bool) (name string, tokens theme.Tokens, err error)` from design §7.
- **Acceptance**:
  - `TestThemeChoiceCountingSeam` checks both answers, zero calls when disabled/colour suppressed/explicit choice and exactly one for auto-enabled; invalid choice errors even when disabled; nil answer defaults dark (AC-2.1–2.4).
  - `TestThemeTogglePersists` checks initial chosen ID, toggle, resize and overlay close/reopen (AC-2.5).
  - `TestLateOSCReplyNotKeys` feeds AC-2.7's exact alt-prefixed sequence and proves no action, palette/filter/dialog; identical suffix without prefix acts normally. Malformed, overlong and expired sequences release ordinary keys, and bound memory64 runes.
- **Test plan**: Pure counting callback plus actual tea.KeyMsg sequences; no terminal network/query; no change to go.mod/go.sum.
- **Invariants touched**: I03/I13/I14 (v2 §15.1: no injected key action, no font/module dependency, honest fallback).

### Task 18 — Expose --theme across launch branches

- **Domain/agent**: go-implementer
- **Budget**: complex (six files; spec-decomposition sizing rule)
- **Depends on**: Task 17
- **Change**: Validate the shared theme flag before branch selection and invoke the theme resolver only when a permitted TUI launch needs it.
- **Files**:
  - `internal/cli/tui.go`
  - `internal/cli/run.go`
  - `internal/cli/demo.go`
  - `internal/cli/review.go`
  - `internal/cli/tui_test.go`
  - `tests/e2e/tui_test.go`
- **Acceptance**:
  - `TestThemeFlagLaunchMatrix` covers run/demo/review, invalid value ->2 naming flag/value even piped, explicit dark/light and all nine AC-2.4 bypass cases with zero callback counts; ordinary TUI auto consults once (FR-2).
  - `TestThemeHelpPlatformLimits` asserts exact AC-2.6 help wording keyed by GOOS and does not claim terminal qualification; `TestThemeFlagBinary` checks packaged help/invalid flag exit and dark/light dispatch.
  - Task 1 baseline stays byte-identical; no new direct or indirect module (`git diff` against this task's base for go.mod/go.sum is empty).
- **Test plan**: cli.Main exit/output assertions plus built-binary checks; all theme selection through Task17's seam, no mutable global test state.
- **Invariants touched**: I09/I13/I14 (v2 §§15.1,15.3: suppressed output unchanged, platform capability disclosed).

### Task 19 — Update user-facing limits and instructions

- **Domain/agent**: go-implementer (docs)
- **Budget**: standard
- **Depends on**: Task 18
- **Change**: Document implemented view, controls, detach/recover semantics, theme choice and remaining evidence gaps, preserving docs-site structure and existing linear examples.
- **Files**:
  - `README.md`
  - `docs/user-guide.md`
  - `docs/tui-slice-g09-evidence.md`
  - `docs/demos/README.md`
- **Acceptance**:
  - Inspect TUI/known-limits sections against task evidence: remove only closed q/two-interrupt issues, include exact detach/phase/qualification outcomes, the retained demo state-root recovery command and explicit post-recovery operator cleanup procedure, mixed-version handoff rejection and the instruction to finish/drain old supervisors before binary replacement, --theme syntax/platform help, contrast-background caveat and existing5s startup query (FR-6 DoD, AC-1.6/2.6).
  - Evidence §5 links actual PR/test evidence and explicitly retains unqualified detection, human screen-reader matrix, unsupported Windows detach, non-deadline reconnect wait, verification cancellation and stale-beat unknown limits. No claim that G09 passed from agent goldens.
  - Any newly pasted command output comes from a named executed fake/binary test; old plain/accessibility/JSONL examples remain unchanged. Docs say recording awaits Task20 until artifacts actually exist.
- **Test plan**: Render/read changed document sections, hygiene gate, verify source links and named evidence; no inferred benchmark advantage.
- **Invariants touched**: I06/I09/I13/I14/I25 (v2 §§15,19: explicit limits and return-to-native guidance).

### Task 20 — Re-record the synthetic TUI demonstration

- **Domain/agent**: maintainer
- **Budget**: complex (six files; spec-decomposition sizing rule)
- **Depends on**: Task 19
- **Change**: Re-record the existing credential-free demo after all visual changes and regenerate the poster/manifest with the documented recording workflow.
- **Files**:
  - `docs/demos/tui.tape`
  - `docs/demos/tui.gif`
  - `docs/demos/tui.transcript.txt`
  - `docs/demos/tui-poster.png`
  - `docs/demos/manifest.json`
  - `docs/demos/README.md`
- **Acceptance**:
  - The recording shows the shipped Observatory view and working safe q lifecycle, with synthetic content only; transcript and poster correspond to the regenerated GIF, and existing linear demo artifacts are unchanged.
  - Record the exact generating command/version and inspect the rendered artifact before marking complete. If tooling is unavailable, keep this task incomplete with its concrete blocker; no placeholder recording or claimed human accessibility qualification.
- **Test plan**: Follow `specs/*/tui-tape-recording/` and docs/demos/README.md; hygiene plus artifact inspection.
- **Invariants touched**: I09/I13/I14 (v2 §§15.1,19: demonstration provenance and no expanded qualification claim).
