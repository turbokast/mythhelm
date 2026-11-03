# TUI slice G09 evidence

Spec: `tui-slice` (MH-2). Build under test: `dfc0f22` (origin/main,
tasks 1–13 merged) unless a row says otherwise.

**Status: complete 2026-10-03.** Interactive matrix row (§1.2),
UX session (§2), screen-reader position (§3, experimental) and
traceability (§4) are recorded. Per I14, a combination without a
full row below is not claimed — see `README.md` ("TUI status"). This is a
historical evidence record, not a G09 pass for the current Observatory view.
Section 5 records subsequent automated evidence and the remaining gaps.

## 1. Terminal/shell matrix (§16.4)

The advertised matrix (design §15, Q4) is the dogfood CI OS legs for
plain/non-TTY/resize-safe behaviour, plus the interactive
terminals/shells recorded in §1.2.

### 1.1 CI-covered behaviour (non-interactive)

These rows are covered by automated tests on every CI OS leg (Linux
x86_64 + ARM, macOS, Windows x86_64 + ARM); the interactive columns
do not apply to them.

| Behaviour | Tests | CI legs |
|---|---|---|
| Non-TTY `run` stays linear; `--plain` forces linear; `--format jsonl` byte-stable; `--accessible` ordered stream; invalid modes exit 2 | `TestE2ENonTTYStaysLinear`, `TestE2EPlainForcesLinear`, `TestE2EJsonlStable`, `TestE2EAccessibleStream`, `TestE2EInvalidFlagsExit2` (`tests/e2e/tui_test.go`); `TestLaunchRuleMatrix`, `TestInvalidModesExit2` (`internal/cli/tui_test.go`) | ubuntu-latest, ubuntu-24.04-arm, macos-latest, windows-latest, windows-11-arm (PR #103) |
| Resize never loses a pending approval or moves a destructive action; focus stable across reloads | `TestResizePreservesIdentity` (`internal/tui/layout_test.go`), `TestFocusStableAcrossReload` (`internal/tui/nav_test.go`) | all Go CI legs (PR #103) |
| Layout breakpoints (compact/single/two-pane/wide) render width-safe | `TestLayoutBreakpoints`, `TestShortHeightCompact` (`internal/tui/layout_test.go`) | all Go CI legs (PR #103) |
| Capability precedence (NO_COLOR, TERM=dumb, explicit flags) | `TestNoColorSuppressesColourOnly`, `TestDumbTermMinimal`, `TestExplicitNeverWins`, `TestAutoDefaults` (`internal/tui/caps/caps_test.go`) | all Go CI legs (PR #103) |

### 1.2 Interactive terminals (maintainer-recorded)

Minimum: the maintainer's own terminal plus §3. Copy the row per
combination; every column needs a result or the combination is not
claimed. Mouse is "not offered" by design (D9) — record that, do not
test it.

| # | Date | OS + version | Terminal + minimum version | Shell | Colour mode | Unicode/ASCII | Resize | Paste | Focus | Keyboard | Mouse | Copy | Alt-screen restore | Interrupted exit | Plain (`--plain`) | Screen reader |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| 1 | 2026-10-03 | Ubuntu 24.04.5 LTS | GNOME Terminal 3.52.0 (VTE 0.76.0), TERM=xterm-256color, 190x45 | zsh 5.9 | auto: distinct colours; `--colour never`: monochrome, no leakage (pass) | Unicode ─ │ ▶ ● render; `--icons ascii` switches to `- \| >` (pass) | drag 190→~100→190 mid-run: layout adapts, no garbling, no stuck approval (pass) | multi-line paste lands in `/` filter (pass) | Alt-Tab away/back: renders fine (pass) | arrows/Tab cycle panes; up/down within-pane (no-op on single-item demo); `/ : ? q` Esc respond; exit-option rows not selectable — unwired stub, only Esc acts (partial, see §5) | not offered (D9) | select then middle-click paste or Shift+Ctrl+C copies (selection goes to primary buffer per terminal semantics, not app behaviour) (pass) | previous terminal content intact after quit (pass) | Ctrl-C after completion: exit 0, review+done banners print (pass); mid-run Ctrl-C: run cancelled, exit 130, no detach/reattach line (see §5 F-2) | non-TTY piped run: linear banners, zero escape bytes, exit 0 (pass) | see §3 |

Field meanings: colour mode = the resolved mode (`auto` resolution or
forced flag); Unicode/ASCII = `--icons` rendering incl. wide/bidi
safety; resize = mid-run resize keeps approvals and focus; paste =
bracketed multi-line paste; focus = focus events where the terminal
sends them; keyboard = arrows, `/`, `:`, `?`, `q`, Esc back-out;
copy = selecting/copying visible text; alt-screen restore = previous
screen content intact after quit; interrupted exit = Ctrl-C behaviour
and exit code.

## 2. UX session (§18.6)

Participant profile (required): a non-Vim user unfamiliar with
orchestration vocabulary. Screenshot-only records fail — every check
needs observed interaction.

- Participant: software engineer, primarily a Claude Code and VS Code
  user (non-Vim, unfamiliar with orchestration vocabulary)
- Date / build / terminal: 2026-10-03 / `dfc0f22` / maintainer's
  terminal (same as §1.2 row 1)
- Facilitator notes: the participant had seen the maintainer run the
  demo before their own session; all interaction below is the
  participant driving.

| Check | Completed | Confusion | Accidental actions | Notes |
|---|---|---|---|---|
| Demo run (`demo`) | yes (had seen it run first) | sees tasks/agents/selected-change panes but does not intuitively know what is going on | none | needs an orienting first-run cue |
| Native-profile discovery | no — guessed `builtin/fake` | does not know what a profile is | none | label not discoverable as a profile |
| Billing understanding | no | the billing line is meaningless to them; wants to see an actual cost amount | none | kind-labels (AC-7.3) do not answer "what does it cost" |
| Bounded task start | no — gave up | tried Enter, tabbing, applying; did not know what was going on at all | attempted an apply without understanding it | no discoverable task-start path in the slice |
| Attention-state recognition | partial | reads ready-for-review as "something wants review" but not who, what or why | none | state visible, referent missing |
| Diff inspection | partial | guesses the agent changed `demo.txt` but not what changed | none | diff panel found, content not read |
| Stop / detach | no — gave up | tried `q` then every key to exit; nothing acted (consistent with §5 F-1) | none | exit path undiscoverable while the dialog is unwired |
| Receipt location | no | no idea where the proof is | none | receipt not surfaced in the TUI flow |

State-distinction checks (the participant names each state unaided):

| State shown | Participant's reading | Correct? |
|---|---|---|
| running | "example running" | partial — recognised activity, not the state vocabulary |
| waiting | not distinctly named | no |
| ready for review | "waiting for a review" | yes — mapped to "something wants review" (see check 5) |
| applied | "exit" | no — conflated end-of-flow with the applied state |

The participant narrated the flow as "example running, then waiting
for a review, then exit" rather than naming four distinct states.

## 3. Screen-reader session (AC-5.4)

One session against `--accessible` with NVDA, VoiceOver or Orca, or —
until such a session is recorded — all screen-reader combinations
stay experimental.

- Screen reader + version / terminal / date / build: none — the
  maintainer has no screen-reader access and knows no screen-reader
  user (2026-10-03)
- Result: **experimental** — all screen-reader combinations stay
  experimental per AC-5.4 until a session is recorded. The
  `--accessible` stream itself is covered by automated tests
  (`TestAccessibleOrderedStream`, `TestAccessibleNoChatter`,
  `TestAccessibleNoCursorCodes`, `TestE2EAccessibleStream`); only the
  human screen-reader session is missing.

## 4. Traceability (AC-1.1 to AC-7.3 → tests from tasks 2–13)

Every criterion traces to at least one named merged test. AC-5.4 is
satisfied by §3 above (or the experimental marking), not by a Go test.

| AC | Requirement (short) | Tests |
|---|---|---|
| AC-1.1 | Mission view shows goal, next action, activity, diff, verification | `TestMissionShowsRequiredFacts`, `TestNextActionTable` (`internal/tui/model_test.go`, task 6) |
| AC-1.2 | Selected-change panel renders a real width-safe diff | `TestParseHunks`, `TestRenderWidthSafe`, `TestRenderAsciiFallback` (`internal/tui/diff_test.go`, task 5) |
| AC-1.3 | Lanes carry readable runtime/profile labels, never vendor internals | `TestLaneLabels` (`internal/tui/model_test.go`, task 6) |
| AC-1.4 | State as text plus shape/icon with ASCII alternatives | `TestStatusStripKindLabels` (`internal/tui/model_test.go`, task 6), `TestRenderAsciiFallback` (task 5) |
| AC-1.5 | Fonts never required; Nerd Fonts optional | `TestNoFontDependentGlyphs` (`internal/tui/model_test.go`, task 6) |
| AC-2.1 – AC-2.4 | Wide / two-pane / single / compact layouts by width and height | `TestLayoutBreakpoints`, `TestShortHeightCompact` (`internal/tui/layout_test.go`, task 6) |
| AC-2.5 | Non-TTY, dumb, machine or plain output stays stable-linear | `TestLaunchRuleMatrix` (`internal/cli/tui_test.go`, task 12), `TestE2ENonTTYStaysLinear`, `TestE2EPlainForcesLinear`, `TestE2EJsonlStable` (`tests/e2e/tui_test.go`, task 13) |
| AC-2.6 | Resize never loses approvals or moves destructive actions | `TestResizePreservesIdentity` (task 6), `TestFocusStableAcrossReload` (`internal/tui/nav_test.go`, task 7) |
| AC-3.1 | Arrow/focus navigation, `/` filter, `:` palette, `?` help | `TestKeyFlows`, `TestPaletteSearchable`, `TestHelpFullNames` (`internal/tui/nav_test.go`, task 7) |
| AC-3.2 | No mouse-only action (mouse not offered) | `TestNoMouseOffered` (task 7) |
| AC-3.3 | Destructive/costly actions need labelled confirmation | `TestSingleKeypressNeverConfirms`, `TestApplyShowsExactEffects` (`internal/tui/actions_test.go`, task 9) |
| AC-4.1 | Signature moments (arrival, dispatch, live, delivery) | `TestArrivalInterruptible`, `TestDispatchNeedsLaunchAck`, `TestLivePulseHasNoTypewriterDelay`, `TestDeliveryRevealsActualResult` (`internal/tui/motion_test.go`, task 8) |
| AC-4.2 | Reduced motion honoured | `TestReducedMotionStatic` (task 8) |
| AC-4.3 | `--colour` / `--motion` / `--icons` flags | `TestAutoDefaults`, `TestParseRejectsUnknown` (`internal/tui/caps/caps_test.go`, task 3), `TestInvalidModesExit2` (task 12) |
| AC-4.4 | Explicit overrides; never `COLORFGBG` alone | `TestExplicitNeverWins`, `TestColorFgBgNeverConsulted` (task 3) |
| AC-5.1 | Linear screen-reader mode, no chatter, no cursor codes | `TestAccessibleOrderedStream`, `TestAccessibleNoChatter`, `TestAccessibleNoCursorCodes` (`internal/cli/accessible_test.go`, task 11) |
| AC-5.2 | Stable visible focus, full-name help, colour never sole | `TestFocusStableAcrossReload`, `TestHelpFullNames` (task 7), `TestRequiredControlsSurviveAdversarialTheme` (task 9) |
| AC-5.3 | Long content pageable/exportable; wide/bidi safe | `TestAccessibleWideSafe`, `TestAccessibleResumeFromCursor` (task 11), `TestExportWritesStateDir` (task 9) |
| AC-5.4 | Screen-reader claims only for tested combinations | This file §3 (or experimental marking); no Go test |
| AC-6.1 | Coalesced live updates, critical events preserved | `TestCoalescedReload`, `TestNoticeBacklogDrainsToNoticeLines` (`internal/tui/live_test.go`, task 10) |
| AC-6.2 | Long lists/diffs virtualised | `TestViewportVirtualises`, `TestCompactReservesNoticeRows` (task 10) |
| AC-7.1 | Requested-but-unconfirmed stop labelled requested | `TestStopLabelsRequested` (task 9) |
| AC-7.2 | Verification status attached to its exact revision | `TestVerificationMismatchLabelsRevisions`, `TestNativeResultNeverVerified` (task 6) |
| AC-7.3 | Cost/quota figures kind-labelled; unknown never zero | `TestStatusStripKindLabels` (task 6) |

## 5. Current source evidence and remaining gaps

The observations in §§1–3 remain bound to `dfc0f22`; the participant session
has not been repeated. F-1 (unwired `q` rows, [#106](https://github.com/turbokast/mythhelm/issues/106))
and F-2 (the coupled two-interrupt exit path, [#107](https://github.com/turbokast/mythhelm/issues/107))
are closed as current implementation defects by the lifecycle work below.
The historical row and UX failures are preserved. Current Ctrl-C deliberately
requests stop first; use Detach to leave a supported running worker alone.

| Current claim | Merged implementation / named automated evidence |
|---|---|
| Selectable mode/phase exit rows; stop differs from detach; action cancellation joins cleanup | [PR #513](https://github.com/turbokast/mythhelm/pull/513); `TestExitModeTable`, `TestLiveCtrlCIntents`, `TestEmergencyDetachQualification`, `TestExplicitActionQuitCancelsAndJoins` in [lifecycle_test.go](https://github.com/turbokast/mythhelm/blob/main/internal/tui/lifecycle_test.go) |
| State-bound detach/recover, single SIGINT, terminal loss, retained demo resources and recovery | [PR #521](https://github.com/turbokast/mythhelm/pull/521); `TestTUIDetachRecoverBinary`, `TestTUIStopDetachRecoverBinary`, `TestTUISIGINTOnceBinary`, `TestTUITerminalLossBinary`, `TestDemoDetachRecoverBinary` in [packaged Unix PTY tests](https://github.com/turbokast/mythhelm/blob/main/tests/e2e/tui_lifecycle_unix_test.go) (scripted fake agent, real processes) |
| Startup failure does not stop the run; phase/terminal-loss exits preserve canonical results | [PR #516](https://github.com/turbokast/mythhelm/pull/516); `TestTUIStartupFailureJoinsWithoutStop`, `TestTerminalLossStateTable`, `TestTerminalLossApplyOutcome`, `TestInspectStopTerminalLoss` in [CLI lifecycle tests](https://github.com/turbokast/mythhelm/blob/main/internal/cli/tui_lifecycle_test.go) |
| Demo retains repository/state; exact shell-labelled recovery and explicit later cleanup | [PR #518](https://github.com/turbokast/mythhelm/pull/518); `TestDemoDetachRetainsWorkspace`, `TestDemoRetentionNotice`, `TestPOSIXRecoveryCommandExecution`, `TestPowerShellRecoveryCommandExecution` in [demo lifecycle tests](https://github.com/turbokast/mythhelm/blob/main/internal/cli/demo_lifecycle_test.go); native PowerShell execution is Windows-only |
| Worker enforces pinned deadline without a supervisor; mixed-version handoffs reject before launch | [PR #497](https://github.com/turbokast/mythhelm/pull/497); `TestWorkerDeadlineWithoutSupervisor`, `TestMixedWorkerHandoffVersions`, `TestDeadlineRequestPrecedence` in [worker deadline tests](https://github.com/turbokast/mythhelm/blob/main/internal/workers/deadline_test.go); `TestDetachedDeadlineRecoveryBinary` in [packaged deadline tests](https://github.com/turbokast/mythhelm/blob/main/tests/e2e/tui_deadline_test.go) |
| NOW / NEXT and responsive Observatory panes, hidden keys and bounded Diff viewport | [PR #566](https://github.com/turbokast/mythhelm/pull/566); `TestObservatoryLadderGoldens`, `TestHiddenKeysStillWork`, `TestObservatoryDiffViewportWork` in [renderer tests](https://github.com/turbokast/mythhelm/blob/main/internal/tui/observatory_goldens_test.go) |
| Cards preserve mission context and semantic colours | [PR #587](https://github.com/turbokast/mythhelm/pull/587); `TestObservatoryOverlayCards`, `TestOverlayKeepsUpdating`, `TestSemanticSGRRuns` in [overlay tests](https://github.com/turbokast/mythhelm/blob/main/internal/tui/overlays_test.go) |
| Cached theme selection, delayed OSC replies, actual CLI launch/help and unchanged linear baseline | [PR #604](https://github.com/turbokast/mythhelm/pull/604), [PR #618](https://github.com/turbokast/mythhelm/pull/618); `TestThemeChoiceCountingSeam` in [theme tests](https://github.com/turbokast/mythhelm/blob/main/internal/tui/theme_choice_test.go), `TestLateOSCReplyNotKeys` in [OSC reply tests](https://github.com/turbokast/mythhelm/blob/main/internal/tui/osc_reply_test.go); `TestThemeFlagLaunchMatrix`, `TestThemeHelpPlatformLimits`, `TestThemeFlagInvalidLaunchBranches`, `TestUntouchedVerbsStable` in [CLI tests](https://github.com/turbokast/mythhelm/blob/main/internal/cli/tui_test.go); `TestThemeFlagBinary` in [packaged tests](https://github.com/turbokast/mythhelm/blob/main/tests/e2e/tui_test.go) |

These are synthetic/unit/packaged-binary checks. Renderer goldens and parsed
colours establish no human screen-reader usability, real-terminal detection,
live-native qualification or complete G09 pass. The [user guide](user-guide.md#the-terminal-view)
describes the current controls and phase outcomes; acceptance's existing
candidate-only TUI verification limit remains there.

Remaining gaps:

- **Detection and platform matrix:** the current light/dark detection path is
  unqualified. `auto` can choose dark on a light terminal under tmux/screen or
  on Windows; the existing startup query can wait 5 seconds. The only human
  terminal row remains the historical §1.2 combination. Contrast assumes a
  matching terminal background.
- **Human accessibility:** NVDA, VoiceOver and Orca sessions and the current
  G09 UX re-run remain missing; all screen-reader combinations stay
  experimental. Declarative user theme/keymap files and reset are deferred.
- **Windows detach:** deliberate detach requires admission's recorded
  `worker_detachment: supported`. Unknown/unsupported records disable it;
  emergency exit says unqualified and does not establish support.
- **Reconnect wait:** fenced non-deadline episodes can wait indefinitely after
  spooling for a supervisor to reconnect. Deadline-expired workers exit after
  terminal spooling; that fix does not close the general reconnect gap.
- **Verification cancellation:** worker stop cannot cancel verification or
  applying. Those TUI controls are disabled and a second Ctrl-C finishes the
  phase headless. The linear-path cancellation limitation remains.
- **Supervision observation:** a stale or missing beat means unknown, not
  proof that the owner is absent; no production unlocked-owner observation
  has been qualified.
- **Upgrade handoff:** mixed supervisor/worker payloads fail closed; finish or
  drain old supervisors before replacing the binary. Already running workers
  retain their pins, but seamless admission-to-spawn replacement is unsupported.
- **Recording:** Task 20 still awaits real recording tools, generated artifacts
  and human inspection. [PR #576](https://github.com/turbokast/mythhelm/pull/576)
  tests the selector with fake tools (193 assertions), and
  [PR #585](https://github.com/turbokast/mythhelm/pull/585) tests per-recording
  CI pins with 15 two-commit fixtures. Neither supplies a new recording or
  human evidence; [recording provenance](demos/README.md) stays per artifact.
