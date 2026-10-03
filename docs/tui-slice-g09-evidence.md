# TUI slice G09 evidence

Spec: `tui-slice` (MH-2). Build under test: `dfc0f22` (origin/main,
tasks 1–13 merged) unless a row says otherwise.

**Status: complete 2026-10-03.** Interactive matrix row (§1.2),
UX session (§2), screen-reader position (§3, experimental) and
traceability (§4) are recorded. Per I14, a combination without a
full row below is not claimed — see `README.md` ("TUI status").

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

## 5. Findings for follow-up

- **F-1 (exit-options dialog unwired):** `q` renders the exit-options
  rows (`internal/tui/dialogs.go`, `dialogExitOptions`) but key
  selection was never wired — "informational until a later task wires
  its rows; only Esc acts" — and no later task did. Observed
  2026-10-03: left/right/Tab/Enter all inert in the dialog; Esc closes
  it; Ctrl-C quits. The README claims no selectable exit options.
  Needs a follow-up task or backlog card to wire the rows (detach /
  request stop / cancel) per the design exit behaviour.
- **F-2 (mid-run Ctrl-C cancels instead of detaching):** task 12's
  contract is "quit-while-active detaches via two interrupts with the
  `review <run>` reattach line" (`TestQuitLiveDetaches` passes), but
  three real mid-run Ctrl-C presses on the scripted demo (2026-10-03)
  all cancelled the run with exit 130 and no detach line. Likely
  mechanism: on a fast run the stop confirms before the detach logic
  can engage, so the detach path is unobservable via `demo`. Needs a
  slow-adapter retest or a design clarification of what TUI Ctrl-C
  promises. (One very-early Ctrl-C attempt printed nothing after `^C`;
  exit code not captured.)
