## TUI Slice — Tasks

### Dependencies

- Prerequisite: `specs/*/dogfood-slice/` (shipped) — the run pipeline, journal, receipt, review diff and linear output this TUI renders. Follow-on (not a blocker): `specs/*/docs-site-demos/` FR-3 sequences behind this spec.
- The tasks are listed in dependency order. Parallel groups (same `Depends on` set, disjoint `Files`): {1, 2, 3, 4} may run in parallel (task 1 touches only `go.mod`/`go.sum`); task 11 (core, new files only) may run in parallel with any of tasks 5–10. Tasks 6–10 share `internal/tui/model.go` and never run in parallel with each other; tasks 12–14 are strictly sequential.
- Each task is sized for one agent session. `Domain/agent`: `tui-implementer` for `internal/tui/` and `mods/`, `go-implementer` for `internal/cli/`, `tests/` and `go.mod`/`go.sum`, `maintainer` for the human-run evidence task.
- **Gates for every task.** Run `gofmt -w` on touched files before any check, then `go vet ./...`, `go test -race ./...`, `go mod tidy -diff` (must print nothing), `golangci-lint run`, and `scripts/ci/check-public-hygiene.sh`. Task 1 also runs `govulncheck ./...` and `CGO_ENABLED=0 go build ./...`. A task is not complete because files exist or an agent reported success (§22.2); cite the runner output.
- **Completion convention.** Append ` ✅ COMPLETED` to the heading, keep every original field, and add `Status` (`✅ Completed — …; PR #<n>`), `Implementation` (at most 3 lines, plus the commit SHA), `Spec deviations` ("None" or a justification) and `Files modified`.
- **Commits.** Every commit is signed off (`git commit -s`, DCO). No task in this spec changes billing, persistence or process ownership, so no ADR is required.

---

## Implementation Tasks

### Task 1 — TUI dependencies ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Change**: Add the §6.4 TUI stack at the versions design §16 D11 pins, and promote the already-required `go-isatty` for TTY detection (D12), so later tasks build the view on reviewed modules.
- **Files**:
  - `go.mod` (add requires)
  - `go.sum` (add hashes)
  - `internal/tui/tools.go` (new pin file, creates `internal/tui/`; package `tui` with blank imports of `github.com/charmbracelet/bubbletea`, `github.com/charmbracelet/lipgloss`, `github.com/charmbracelet/bubbles`, `github.com/mattn/go-isatty` plus a comment stating the file exists only to keep the requires tidy-clean; pins per Produces below)
- **Produces**: module versions `github.com/charmbracelet/bubbletea v1.3.10`, `github.com/charmbracelet/lipgloss v1.1.0`, `github.com/charmbracelet/bubbles v1.0.0`, `github.com/mattn/go-isatty v0.0.24` direct (its current indirect version, promoted).
- **Domain exception**: go-implementer owns this `internal/tui/` pin file as incidental to the `go.mod` change (later tasks replace it with real consumers).
- **Acceptance**:
  - `go mod tidy -diff` prints nothing with the pin file present, and `go list -m github.com/charmbracelet/bubbletea github.com/charmbracelet/lipgloss github.com/charmbracelet/bubbles github.com/mattn/go-isatty` shows exactly the four pinned versions; any drift changes that output and fails the task. (Without the pin file `go mod tidy` drops the unimported requires, leaving `go.mod` byte-identical to base.)
  - The PR's dependency-review and `licenses` CI jobs pass with the new modules on the allowlist; a GPL-licensed transitive dependency would fail them.
  - `govulncheck ./...` reports no findings, and `CGO_ENABLED=0 go build ./...` succeeds; before this task the Charm imports do not resolve, so any consumer fails to build.
- **Test plan**: no new Go tests; verification is the gate commands plus the recorded `go list -m` output.
- **Invariants touched**: §19.2 (Apache-2.0-compatible licences only, exact pins), §19.4 (supply-chain review), I13 (§3.3: no font/network/service dependency introduced).
- **Status**: ✅ Completed — the §6.4 TUI stack is pinned at the D11/D12 versions with a tidy-clean pin file; PR #93.
- **Implementation**: `go get` at the four exact pins plus `internal/tui/tools.go` (package `tui`, blank imports of the four modules); `go-isatty` promoted indirect→direct, `go list -m` shows exactly the pins, `tidy -diff` clean. Commit 5948279b33da6b2157bd10e37014228892dc53ff.
- **Spec deviations**: None.
- **Files modified**: `go.mod`, `go.sum`, `internal/tui/tools.go`, `specs/in-progress/tui-slice/tasks.md`, `specs/in-progress/tui-slice/handoff.md`.

### Task 2 — View-model read seam ✅ COMPLETED

- **Domain/agent**: tui-implementer
- **Budget**: complex (14 acceptance items: scan matrix plus failure/gate cases)
- **Depends on**: None
- **Change**: Add `internal/tui/viewmodel` (design §3) so every pane renders one `Snapshot` built from read-only journal projections plus the receipt, with nil fields for not-yet-projected parts.
- **Files**:
  - `internal/tui/viewmodel/viewmodel.go` (creates `internal/tui/`)
  - `internal/tui/viewmodel/viewmodel_test.go`
- **Produces**: `viewmodel.Snapshot` struct (with `LatestProgress *Progress` / `NativeExit *NativeExit` / `Admission *Admission` folded from the event scan); `viewmodel.Progress{AssistantTurns, ToolUses, Retries, RunSeq}`; `viewmodel.NativeExit{ExitCode, Signal, ResultObserved, RunSeq}`; `viewmodel.Admission{AdapterID, AdapterVersion, AdapterSurface, Qualified, PaidContinuation, RunSeq}`; `viewmodel.Load(ctx context.Context, dir, runID string) (Snapshot, error)`; `viewmodel.EventsSince(ctx context.Context, dir, runID string, after int64) ([]journal.Event, error)`.
- **Acceptance**:
  - `TestLoadSnapshot`: a temp state DB with a run, attempt, candidate, verification and receipt yields a `Snapshot` with all fields populated and `LastRunSeq` equal to the journaled event count; an empty `runID` returns an error wrapping `journal.ErrNotFound`.
  - `TestLoadScansProgressAndNativeExit`: a run with journaled `attempt.progress` (`assistant_turns: 7`) and `attempt.native_result` (`exit_code: 0`) events yields `LatestProgress.AssistantTurns == 7` and `NativeExit.ExitCode` pointing at 0; a run with neither event yields nil `LatestProgress`/`NativeExit` (I09: absent, never zero).
  - `TestLoadScansAdmission`: a run with a journaled `admission.decided` event yields `Admission` with the adapter descriptor, `Qualified` and `PaidContinuation` from the payload; a run without one yields nil `Admission`.
  - `TestLoadScanFailures`: a malformed `attempt.progress` payload is skipped with the latest good value kept and `LastRunSeq` still advanced; a malformed `attempt.native_result` or `admission.decided` payload fails `Load` with an error naming the event type and `run_sequence` (I07).
  - `TestLoadNoReceiptWritten`: a mid-run fixture with progress and native-result events but no `receipt.written` event yields nil `Receipt` and no error (the receipt gate); a variant calling `ReadReceipt` blindly fails this test with a wrapped `os.ReadFile` error.
  - `TestLoadReceiptWrittenWithoutFile`: a fixture with a journaled `receipt.written` event whose `receipt.json` is then deleted yields an error from `Load` (the wrapped `os.ReadFile` failure); a variant leaving `Receipt` nil and returning success fails.
  - `TestLoadCorruptReceipt`: a fixture with a journaled `receipt.written` event whose `receipt.json` is then overwritten with corrupt bytes (SHA-mismatch case, plus a JSON-syntax-error variant) yields an error from `Load`; a variant accepting the corrupt bytes fails.
  - `TestLoadEventsQueryError`: a fixture whose `journal` table is dropped after the build (projection tables intact, so `Run`/`LatestAttempt`/`Candidate`/`LatestVerification` still succeed) yields an error from `Load`; a variant skipping the event scan returns success and fails.
  - `TestLoadPartialRun`: a run with no attempt yields nil `Attempt`/`Candidate`/`Verification`/`Receipt` and no error; a variant returning `ErrNotFound` for the whole snapshot fails this test.
  - `TestReceiptStateMismatch`: a receipt whose state differs from the run projection returns an error (mirrors the `review.go` agreement check); agreeing fixtures pass.
  - `TestEventsSinceIncremental`: after journaling N further events, `EventsSince(after=LastRunSeq)` returns exactly those N in `run_sequence` order.
  - `TestViewmodelNeverWrites`: fails if `journal.Open` or `journal.Append` is referenced in the package (I18 evidence; the test greps the package sources excluding itself, first asserting the scanned file set is non-empty so a wrong scan path fails instead of false-passing, then asserts zero hits — adding a write call turns it red).
  - `TestNoDatabaseEmptyState`: a state dir without `mythhelm.db` yields `journal.ErrNoDatabase` from `Load`, so callers can show the empty state instead of an error.
  - `TestLoadGoalFromTaskFile`: a fixture run in `executing` with no receipt but a digest-matching `task.md` yields the task title as the goal; a digest-mismatched `task.md` fails `Load` with an error.
- **Test plan**: build fixtures with `journal.Open` + `Append` in `t.TempDir()`; table-driven over run states.
- **Invariants touched**: I18 (§7.8: read-only seam, never a second writer), I09 (§13.4: absent projections stay nil/`unknown`, never zero values), I06 (§7.3: requested vs confirmed states pass through unmapped).
- **Status**: ✅ Completed — the view-model read seam renders one read-only `Snapshot` per run; PR #91.
- **Implementation**: `Load` folds the event scan into `LatestProgress`/`NativeExit`/`Admission`, gates `ReadReceipt` on `receipt.written`, and resolves `Goal` from the receipt title or digest-checked task file; malformed progress is skipped, malformed native/admission evidence fails. Commit 13758bf8b32a3cbb678ea66fd275f43e5cb68b7f.
- **Spec deviations**: `Snapshot` gains a `Goal string` field the design §3 struct snippet omits (the §3 text plus task 2/6 acceptances require it); otherwise none.
- **Files modified**: `internal/tui/viewmodel/viewmodel.go`, `internal/tui/viewmodel/viewmodel_test.go`, `specs/in-progress/tui-slice/tasks.md`, `specs/in-progress/tui-slice/handoff.md`.

### Task 3 — Terminal capability parsing ✅ COMPLETED

- **Domain/agent**: tui-implementer
- **Budget**: standard
- **Depends on**: None
- **Change**: Add `internal/tui/caps` (design §4) parsing the §15.7 `--colour`/`--motion`/`--icons` modes and resolving them against `NO_COLOR` and `TERM`, so rendering honours explicit user control.
- **Files**:
  - `internal/tui/caps/caps.go`
  - `internal/tui/caps/caps_test.go`
- **Produces**: `caps.Parse(colour, motion, icons string) (Prefs, error)` with `caps.ErrInvalidMode`; `caps.Resolve(p Prefs, getenv func(string) string) Caps`; `caps.Caps{Colour, Motion, Icons}` levels.
- **Acceptance**:
  - `TestParseRejectsUnknown`: each of `--colour rainbow`, `--motion turbo`, `--icons emoji` returns `ErrInvalidMode`; every §15.7 enumeration value parses.
  - `TestNoColorSuppressesColourOnly`: with `NO_COLOR=1`, `Resolve` yields `ColourNever` while `MotionFull`/`IconsUnicode` survive (AC-4.3); a variant mapping `NO_COLOR` to motion suppression fails.
  - `TestExplicitNeverWins`: `--colour never` with full-colour terminfo still yields `ColourNever`; `--motion off` yields `MotionOff` regardless of environment.
  - `TestDumbTermMinimal`: `TERM=dumb` resolves to all-minimal levels even with `auto` flags.
  - `TestAutoDefaults`: empty environment with `auto` flags yields full colour, full motion, unicode icons.
  - `TestColorFgBgNeverConsulted`: `COLORFGBG=15;0` with `auto` flags and an otherwise empty environment yields the same `Caps` as the empty environment (AC-4.4); an implementation keying colour detection off `COLORFGBG` fails.
- **Test plan**: table tests over (flags × env) with a fake `getenv`; no TTY needed.
- **Invariants touched**: None (pure parsing; no state read, no values rendered).
- **Status**: ✅ Completed — `internal/tui/caps` parses §15.7 modes and resolves them against `NO_COLOR`/`TERM`; PR #89.
- **Implementation**: Strict `Parse` (rejects `""`/wrong case; errors name dimension+value, wrap `ErrInvalidMode`); suppression-wins `Resolve` (`NO_COLOR` beats `always`, `TERM=dumb` beats enabling flags; zero `Caps` suppresses). Commit a425654.
- **Spec deviations**: None.
- **Files modified**: `internal/tui/caps/caps.go`, `internal/tui/caps/caps_test.go`, `specs/in-progress/tui-slice/tasks.md`, `specs/in-progress/tui-slice/handoff.md`.

### Task 4 — Built-in themes in mods/ ✅ COMPLETED

- **Domain/agent**: tui-implementer
- **Budget**: standard
- **Depends on**: None
- **Change**: Add the declarative theme format with built-in dark and light token files under `mods/themes/` plus the loader/validator (design §5), so the TUI styles from data no code change can silently alter.
- **Files**:
  - `mods/themes/dark.toml` (creates `mods/themes/`)
  - `mods/themes/light.toml`
  - `mods/themes/themes.go` (package `themes`, `//go:embed *.toml` exposing `themes.Files`)
  - `internal/tui/theme/theme.go`
  - `internal/tui/theme/theme_test.go`
- **Produces**: `theme.Tokens` struct; `theme.BuiltIn(name string) (Tokens, error)` (parsed from `themes.Files` bytes); `theme.Load(path string) (Tokens, error)`; `theme.ErrUnknownTheme`, `theme.ErrInvalidTheme`; `(Tokens) Validate() error`; `themes.Files embed.FS` in `mods/themes/themes.go`.
- **Acceptance**:
  - `TestBuiltInsValidate`: `BuiltIn("dark")` and `BuiltIn("light")` both `Validate` clean, including the 4.5:1 text/surface contrast check; a token set with low-contrast text fails `Validate`.
  - `TestLoadRejectsUnknownKeys`: a TOML file with a `backgroud` typo returns `ErrInvalidTheme` naming the key; exact-schema files load.
  - `TestLoadRejectsBadColours`: a non-hex colour value returns `ErrInvalidTheme` naming the key.
  - `TestUnknownTheme`: `BuiltIn("aurora")` returns `ErrUnknownTheme`.
- **Test plan**: golden token structs for the built-ins; adversarial token fixtures; luminance unit cases. (The adversarial-token control-survival test lives in task 9 — the approval/spend/stop surfaces it renders only exist from tasks 6/9.)
- **Invariants touched**: I11 (§14.11: the token format carries no code paths to approval/evidence surfaces; the survival proof is task 9's `TestRequiredControlsSurviveAdversarialTheme`), I13 (§3.3: no font requirement in the format).
- **Status**: ✅ Completed — declarative dark/light token files plus the shared loader/validator landed; PR #90.
- **Implementation**: `BuiltIn`/`Load` share one strict parse+validate path; WCAG 4.5:1 gate on Text/TextMuted × Surface/SurfaceRaised. Commit 7ef2641f67f715719eb77141aa552460aff422d0.
- **Spec deviations**: None.
- **Files modified**: `mods/themes/dark.toml`, `mods/themes/light.toml`, `mods/themes/themes.go`, `internal/tui/theme/theme.go`, `internal/tui/theme/theme_test.go`, `specs/in-progress/tui-slice/tasks.md`, `specs/in-progress/tui-slice/handoff.md`.

### Task 5 — Diff viewer component

- **Domain/agent**: tui-implementer
- **Budget**: complex (width-safe Unicode handling plus truncation semantics)
- **Depends on**: Task 1, Task 3, Task 4
- **Change**: Add the width-safe unified-diff parser and renderer (design §8) consumed by the selected-change pane, with structural highlighting where caps allow and an explicit truncation label past the line/byte caps. (Task 1 first: `diff.go` imports `lipgloss`, so without the pinned modules the package does not build.)
- **Files**:
  - `internal/tui/diff.go`
  - `internal/tui/diff_test.go`
- **Produces**: `tui.Diff{Files, Lines, Truncated, TruncateCap}`, `tui.maxDiffLines = 50000`, `tui.maxDiffBytes = 4 << 20`, `tui.ParseDiff(unified []byte) (Diff, error)`, `(Diff) Render(width int, caps caps.Caps, t theme.Tokens) []string`.
- **Acceptance**:
  - `TestParseHunks`: a two-file unified diff yields both files with correct hunk/line counts; malformed input returns an error rather than a half-diff.
  - `TestRenderWidthSafe`: every rendered row is at most `width` cells wide with tabs expanded and wide glyphs uncut (a wide glyph at the boundary truncates with the marker, never splits); a `len()`-based implementation fails the wide-glyph case.
  - `TestRenderAsciiFallback`: with `IconsASCII` + `ColourNever`, output contains no non-ASCII bytes and no escape introductions; the unicode/full-caps variant contains structural highlighting.
  - `TestTruncationLabelled`: input past `maxDiffLines` sets `Truncated` with `TruncateCap == "lines"`, and the rendered footer carries the truncation label with the retained/total line counts naming the line cap; silently dropped lines fail this test. (The exact-command assertion lives in task 6: `ParseDiff` takes bare bytes with no provenance, so only the selected-change pane — which has the `CandidateRow` — can name base and commit.)
  - `TestByteCapBoundsSingleLine`: a single-line input larger than `maxDiffBytes` sets `Truncated` with `TruncateCap == "bytes"`, with zero parsed rows (no newline falls within budget, so trimming to the last newline leaves nothing to parse — no partial row), and the rendered footer showing the retained count (0, fewer than 50,000) over the total while naming the byte cap; a variant parsing the whole input or any partial row, or a footer claiming 50,000 shown lines, fails (D7).
  - `TestMarkdownDiffKeepsDiffChrome`: a diff over a `.md` file renders hunk headers and `+`/`-` markers structurally (AC-1.2; a Markdown-prose rendering fails).
- **Test plan**: inline fixtures plus generated wide/tab/bidi cases; golden rows.
- **Invariants touched**: I09 (§13.4: absent diff renders as labelled absence, never empty success), §12.7 (control characters sanitised before render — a fixture with ANSI escapes in diff content renders them inert).

### Task 6 — App shell, layouts and mission view

- **Domain/agent**: tui-implementer
- **Budget**: complex (several new abstractions: model, layout ladder, five panes)
- **Depends on**: Task 1, Task 2, Task 3, Task 4, Task 5
- **Change**: Add the Bubble Tea program with the §15.4 layout ladder and the §15.3 mission panes (design §6–§7), rendering one `Snapshot` with text-plus-icon states and the next-action table.
- **Files**:
  - `internal/tui/model.go` (program, `Run`, message dispatch)
  - `internal/tui/actions.go` (`Actions` struct only; behavior arrives in task 9)
  - `internal/tui/layout.go` (breakpoints, resize preservation)
  - `internal/tui/mission.go` (header, tasks, agents, selected-change, status, footer)
  - `internal/tui/styles.go` (caps/theme application, cell formatter)
  - `internal/tui/model_test.go`
  - `internal/tui/layout_test.go`
- **Produces**: `tui.Config{RunID, StateDir string, Caps caps.Caps, Tokens theme.Tokens, Events <-chan journal.Event, Notices *NoticeBacklog, Actions Actions}`; `tui.Run(ctx context.Context, cfg Config) error`; `tui.Actions{Stop, Recover, Apply}` struct with the exact field types of design §10 (no behavior yet — task 9 implements it); `tui.Model` with `Update`/`View` driving the layout ladder.
- **Acceptance**:
  - `TestLayoutBreakpoints`: golden views at 60/85/120/160 columns show compact/single/two-pane/three-pane arrangements per the §15.4 table; a 139-column render with three panes fails.
  - `TestShortHeightCompact`: heights of 5 and 9 rows render the compact view with the linear-mode offer (AC-2.4); 10 rows keeps the width-driven layout.
  - `TestResizePreservesIdentity`: driving `WindowSizeMsg` sequences keeps the selected task ID, focused pane and pending dialog; a variant resetting focus to the first row fails (AC-2.6).
  - `TestMissionShowsRequiredFacts`: a snapshot with goal, `LatestProgress`, candidate and checks renders all five §15.1 facts (goal, next action, activity, diff, verification) in the wide layout (AC-1.1); removing any fact from the view fails.
  - `TestNextActionTable`: every run state maps to its design §7 string, including `stopping` → "waiting for worker confirmation" (I06); a `stopping` → "stopped" label fails.
  - `TestLaneLabels`: lanes show `adapter_id` + execution profile + lane letter + attempt number, never a logo (AC-1.3); unknown adapter renders `unknown`.
  - `TestStatusStripKindLabels`: a snapshot with `Admission` (qualified true, paid continuation "off") renders `qualified: true` and `paid continuation: off` in the status strip; a snapshot with nil `Admission` renders `unknown` for both, never blank or `0` (AC-7.3, AC-1.4 unknown-leg). A bare figure without its kind label fails, as does a `0` for an absent value.
  - `TestNativeResultNeverVerified`: a fixture run whose journaled events include `attempt.native_result` with exit code 0 (and no verification events), loaded through `viewmodel.Load`, renders `native exit: 0` with `NativeExit` populated and shows no `verified`/`pass` label (I07). The fixture is built by journaling the event and calling `Load` — never by hand-constructing the `Snapshot` — so the test proves the scanned path cannot render verification; a variant labelling the exit `verified` fails.
  - `TestVerificationMismatchLabelsRevisions`: a fixture with candidate commit B and a verification row against superseded commit A (built with `InsertCandidate`/`InsertVerification` in a journal tx, as task 2's fixtures) renders `checks ran against <short-A> — candidate is <short-B>` with no `verified` label (AC-7.2); a variant showing `verified` fails.
  - `TestTruncationFooterNamesExactCommand`: a truncated diff over a fixture candidate renders a footer naming the exact fallback command `git -C "<workspace>" diff <base> <commit>` with base and commit from the `CandidateRow`; a footer without the exact command fails (moved from task 5, whose `ParseDiff` takes bare bytes with no provenance). The test covers a byte-truncated fixture as well as a line-truncated one: each footer shows its retained line count over the total and names the cap that fired (`TruncateCap`), so the byte-truncated footer never claims 50,000 shown lines.
  - `TestEscapesRenderedInert`: a snapshot fixture with ANSI escape sequences in task/reason text and diff content renders no escape introductions in `View` output (`styles.go` routes rendered text through `security.TermSafe`, §12.7); raw passthrough fails.
  - `TestNoFontDependentGlyphs`: `IconsASCII`+`ColourNever` goldens contain no byte ≥ 0x80; `IconsUnicode` goldens contain no private-use (U+E000+) or emoji rune, and every other non-ASCII rune comes from the geometric-shapes table in `styles.go` (AC-1.5, I13). A Nerd-Font glyph or emoji in any golden fails, as does an ASCII golden missing a text label its unicode twin carries.
  - `TestLayeringNoCliImport`: fails if `internal/cli` is referenced anywhere under `internal/tui/` (same grep shape as task 2's write guard, including the non-empty scanned-file-set assertion).
- **Test plan**: drive `Update`/`View` directly with synthetic snapshots and `WindowSizeMsg`s; golden strings per breakpoint.
- **Invariants touched**: I06 (§7.3: stopping/requested wording, resize preservation), I07 (§11.6: verification paired with revision, never inferred), I09 (§13.4: `unknown` labels, kind labels on figures), I13 (§3.3: ASCII glyph table asserted load-bearing in `ColourNever`+`IconsASCII` goldens), §12.7 (control characters sanitised before render — `styles.go` routes rendered text through `security.TermSafe`).

### Task 7 — Navigation, palette and help

- **Domain/agent**: tui-implementer
- **Budget**: standard
- **Depends on**: Task 6 (shares `model.go`; never in parallel with tasks 8–10)
- **Change**: Add keyboard focus, `/` filter, `:` command palette, `?` help and `Enter`/`Esc` flows (design §9), so every action is keyboard-reachable with visible, stable focus.
- **Files**:
  - `internal/tui/nav.go`
  - `internal/tui/palette.go`
  - `internal/tui/help.go`
  - `internal/tui/nav_test.go`
  - `internal/tui/model.go` (wire key dispatch)
- **Produces**: `tui.Action` (`type Action int` with an unexported const block: `actionStop`, `actionRecover`, `actionApply`, `actionExport`, `actionSwitchPane`, `actionTheme`, `actionHelp`, `actionQuit` — `actionSwitchPane` is design §10's "switch tab/pane"), consumed by task 9; palette lists every registered action.
- **Acceptance**:
  - `TestKeyFlows`: driving key messages covers arrows/Tab focus, `/` filter, `:` palette, `?` help, `Enter` details, `Esc` back-out order (AC-3.1); any unreachable surface fails.
  - `TestEscapeNeverQuits`: `Esc` from every depth (dialog, filter, pane) never quits the program; a quit-on-`Esc` variant fails.
  - `TestPaletteSearchable`: typing in the palette filters the action list; every `Action` ID appears by substring (so task 9's actions are reachable).
  - `TestHelpFullNames`: help output names full action names, not bare keys (AC-5.2); a keys-only help fails.
  - `TestFocusStableAcrossReload`: a snapshot reload re-resolves selection by stable ID — focus stays on the same task/lane; a variant dropping to no selection fails (AC-5.2).
  - `TestJKOptional`: `j`/`k` move within a list but every flow in `TestKeyFlows` also passes with arrows alone.
  - `TestNoMouseOffered`: a structural grep over `internal/tui/` sources (excluding the test itself) finds no mouse wiring (`MouseMsg`, `WithMouseCellMotion`/`WithMouseAllMotion`, or equivalent): the test first asserts the scanned file set is non-empty so a wrong scan path fails instead of false-passing. Mouse is not offered in this slice, so AC-3.2 holds vacuously (AC-3.2, D9); any mouse subscription fails.
- **Test plan**: `Update`-level key-message tests plus `View` goldens for palette/help overlays.
- **Invariants touched**: None (navigation changes focus, not state; confirmations arrive in task 9).

### Task 8 — Truthful motion

- **Domain/agent**: tui-implementer
- **Budget**: complex (10 acceptance items: one truth rule per motion moment)
- **Depends on**: Task 7 (shares `model.go`; never in parallel with tasks 9–10)
- **Change**: Add the §15.6 applicable motion subset with its truth rules plus reduced-motion collapse (design §11), so the view feels alive without ever depicting unconfirmed state.
- **Files**:
  - `internal/tui/motion.go`
  - `internal/tui/motion_test.go`
  - `internal/tui/model.go` (wire moment triggers)
- **Acceptance**:
  - `TestDispatchNeedsLaunchAck`: the lane renders running only after an `attempt.launched` event is consumed; a pre-launch tick shows the pre-running state (moment 3 truth rule).
  - `TestRouteCardShowsRecordedAdmission`: the route card shows the adapter id/version/surface from `Snapshot.Admission`, the profile from `Run.ExecutionProfile`, and the static `pinned by --adapter` reason, with no deliberation spinner or invented rationale (moment 2 truth rule); a card showing a rationale not in the snapshot fails.
  - `TestLivePulseHasNoTypewriterDelay`: the activity pulse on `attempt.progress` renders the full counters in the same update the event is consumed (moment 4 truth rule); a character-at-a-time reveal fails.
  - `TestDeliveryRevealsActualResult`: the ready-for-review reveal renders the recorded outcome (verified vs unverified vs failed) and settles within ≤200 ms simulated (moment 9 truth rule); generic success copy on a failed run fails.
  - `TestArrivalInterruptible`: any key during the arrival moment skips to the loaded view; first paint never waits on the moment.
  - `TestNoWaitingCountdown`: the waiting indicator for a run with no known reset/deadline contains no digits-based countdown (moment 6 truth rule).
  - `TestNoIntegrationPercentage`: the integration stages show stage names only, never a percentage (moment 8 truth rule).
  - `TestFailureWithoutSpectacle`: failure rendering contains no ANSI blink/reverse-flash sequences and moves focus to the error plus preserved artifact (moment 10).
  - `TestReducedMotionStatic`: with `MotionReduced`/`MotionOff`, triggering every moment yields immediate state changes with static emphasis and zero tick-driven frames (AC-4.2); any animation frame fails.
  - `TestNoPermanentLoop`: with `MotionFull` and no new events, the program schedules no further motion frames after transitions settle (~200 ms simulated). The test asserts over motion `Cmd`s only and excludes the permanent 200 ms poll tick, because that tick is specified (design §13 live/inspect mode) as a snapshot reload, not animation — counting it would fail a correct implementation. A self-rescheduling motion tick fails (AC-6.1).
- **Test plan**: `Update`-level tests feeding synthetic event messages under each `MotionLevel`; frame scheduling observed via returned `tea.Cmd` nil-ness.
- **Invariants touched**: I06 (§7.3: moments never depict requested as confirmed — arrival/dispatch/waiting cases above).

### Task 9 — Palette actions and confirmations

- **Domain/agent**: tui-implementer
- **Budget**: standard
- **Depends on**: Task 8 (shares `model.go`; never in parallel with task 10)
- **Change**: Implement the palette actions with explicit labelled confirmations over injected supervisor calls (design §10), so stop/recover/apply/export execute the same mutations as the CLI with no second writer.
- **Files**:
  - `internal/tui/actions.go` (behavior over task 6's `Actions` struct)
  - `internal/tui/dialogs.go`
  - `internal/tui/actions_test.go`
  - `internal/tui/model.go` (wire action dispatch)
- **Produces**: palette dispatch, confirmation dialogs and export behavior over `tui.Actions` (struct defined in task 6; consumed by task 12's wiring).
- **Acceptance**:
  - `TestStopLabelsRequested`: with a fake `Stop`, confirming stop renders `stop requested` and never `stopped` until an `attempt.stopped` event arrives (I06); stop is disabled outside `executing`/`verifying`.
  - `TestRecoverOnlyWhenInterrupted`: recover is enabled only in `interrupted`; in any other state the palette shows it disabled with the reason.
  - `TestApplyShowsExactEffects`: the apply dialog shows run, candidate commit, target branch, acceptance toggles and verification state with `[Apply to <branch>] [Cancel]`; confirming calls `Apply` once with those exact values (I03).
  - `TestSingleKeypressNeverConfirms`: no dialog confirms on its first keypress other than explicit confirm; `Esc` cancels every dialog with no call made (AC-3.3).
  - `TestExportWritesStateDir`: export writes the focused pane's plain text to `<stateDir>/exports/<run>-<pane>.txt` and shows the path; nothing is written inside any repository (AC-5.3).
  - `TestActionErrorSurfaces`: a failing fake `Stop` renders the error in place with the run state unchanged; errors are never swallowed.
  - `TestRequiredControlsSurviveAdversarialTheme`: with a maximally adversarial in-test token set, the approval prompt, spend warning and stop-state labels render with their text intact (G08; moved from task 4, whose Files cannot render these task-6/9 surfaces). A theme that could hide them fails.
- **Test plan**: fake `Actions` recording calls; `Update`-level dialog tests; temp state dirs for export.
- **Invariants touched**: I03 (§12.3: confirmations bind to exact effects), I06 (§7.3: requested vs confirmed), I11 (§14.11: adversarial-token control survival), I18 (§7.8: mutations only through supervisor entry points — fakes prove the seam).

### Task 10 — Live render loop and history

- **Domain/agent**: tui-implementer
- **Budget**: complex (concurrency ownership plus virtualisation)
- **Depends on**: Task 9 (shares `model.go`)
- **Change**: Wire the event-driven render loop — bounded live channel, 200 ms coalesced polling, viewport virtualisation and journal-replay history search (design §13) — so live and inspect modes stay responsive without stalling the pipeline.
- **Files**:
  - `internal/tui/live.go`
  - `internal/tui/history.go`
  - `internal/tui/live_test.go`
  - `internal/tui/model.go` (wire live messages)
- **Produces**: `tui.LiveFeed() (events chan journal.Event, notices *NoticeBacklog, hooks supervisor.Hooks)` (capacity-256 channel; `Event` sends as-is, non-blocking drop-on-full; `Notice` appends to the backlog, never blocking or dropping; `Interrupt` left unset for task 12); `tui.NoticeBacklog` with `Add(s string)` / `Drain() []string`; model drains the backlog each poll tick into `ui.notice` render lines; consumed by task 12's live-mode wiring.
- **Acceptance**:
  - `TestLiveCallbackNeverBlocks`: `LiveFeed`'s `Hooks.Event` adapter uses non-blocking send — with a full channel and no reader, 10,000 rapid events return immediately and the next poll replays from the journal (AC-6.2 slow-terminal case); `LiveFeed`'s `Hooks.Notice` appends to the backlog with the same non-blocking guarantee — 1,000 rapid notices with a full channel and no reader return immediately and are all retained in order.
  - `TestNoticeBacklogDrainsToNoticeLines`: `Drain` returns every retained notice in order and empties the backlog; a model tick with a non-empty backlog renders each string as a notice line via the `ui.notice` path (`RunSequence == -1`, payload `{"text":...}`), never as journal progress — a dropped-notice variant fails.
  - `TestCoalescedReload`: rapid event bursts rebuild the snapshot at most once per tick (event counter vs reload counter assertion); per-event rebuilds fail (AC-6.1).
  - `TestViewportVirtualises`: a 10,000-line stream renders at most the pane's visible rows; the row count in `View` output is bounded by pane height (AC-6.2).
  - `TestHistorySearchComplete`: searching history finds a match from the oldest journaled event, proving replay-from-zero rather than a bounded memory copy; a ring-buffer implementation fails on old matches.
  - `TestQuitJoinsGoroutines`: quitting with live mode active joins the channel pump (no goroutine leak under `-race` with `NumGoroutine` delta assertion).
- **Test plan**: synthetic event producers (blocking and flooding); `View` row counting; goroutine accounting.
- **Invariants touched**: I06 (§7.3: UI lag never stalls ownership — drop-and-replay), I05 (§11.3: read-only polling adds no writer).

### Task 11 — Accessible linear renderer ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 2
- **Change**: Add the `--accessible` screen-reader renderer in `internal/cli` (design §12) over the same view-model snapshots, so the linear mode carries complete state labels with no cursor or spinner dependence.
- **Files**:
  - `internal/cli/accessible.go`
  - `internal/cli/accessible_test.go`
- **Produces**: `cli.AccessibleConfig{RunID, StateDir string, Out io.Writer, Poll time.Duration, After int64}`; `cli.accessiblePageSize = 1000`; `cli.RunAccessible(ctx context.Context, cfg AccessibleConfig) error`.
- **Acceptance**:
  - `TestAccessibleLabels`: output for a mid-run snapshot contains complete `run state: …`, `attempt …: …`, `verification: …` labels with full action names (AC-5.1/AC-5.2); label-only-state codes fail.
  - `TestAccessibleNoChatter`: repeated polls with no journal change emit no repeated lines (no spinner chatter); a per-tick heartbeat fails.
  - `TestAccessibleNoCursorCodes`: output contains no cursor-movement or alternate-screen sequences (AC-5.1); any `\x1b[` movement sequence fails.
  - `TestAccessibleOrderedStream`: events journaled out of viewed order still stream in `run_sequence` order (AC-5.1).
  - `TestAccessibleWideSafe`: bidi/combining/emoji/wide fixtures truncate by cell width without displacing the approval-adjacent status line (AC-5.3); a byte-sliced implementation fails.
  - `TestAccessibleResumeFromCursor`: over a fixture run with more than `accessiblePageSize` events, a first invocation (`After: 0`) cancelled after the history page streams returns exactly the first page plus a trailing `next-after: <K>` line; a second invocation with `After: K` streams the remainder with no overlap or gap, in `run_sequence` order, plus its own trailer (AC-5.3); a variant without the trailer or with overlapping pages fails.
- **Test plan**: temp state DB fixtures; golden label streams; byte-scan for escape sequences.
- **Invariants touched**: I09 (§13.4: same `unknown`/kind labelling as the TUI), I14 (§16.1: support claimed only for tested combinations — no claim text in this task).
- **Status**: ✅ Completed — the `--accessible` screen-reader renderer streams complete state labels plus the ordered event stream with paged resume; PR #95.
- **Implementation**: `RunAccessible` emits the snapshot summary (fresh caught-up invocations only), `event <seq>:` lines reusing `plainEvent` labels, and a `next-after: <K>` trailer on context end; stdlib-only cell-width truncation at 200 cells with bidi neutralisation. Commit 224a3dd030d926d9e1bcae0c8bc66495374fed54.
- **Spec deviations**: The summary carries no `approval:` line (the acceptance text's "approval-adjacent status line"): the pipeline has no approval state — `waiting_approval` is never entered (N10) — so inventing one would violate truthfulness; the wide-safe test defends the status line adjacent to hostile content (the run-state line) instead. Review found the uncapped history read let live follow emit pre-existing overflow past the page, so the fix adds `journal.EventsLimit` + `viewmodel.EventsSinceLimit` (additive, SQL-enforced cap; initial read fetches `accessiblePageSize+1` and an over-page invocation returns after one page plus trailer with no follow).
- **Files modified**: `internal/cli/accessible.go`, `internal/cli/accessible_test.go`, `internal/journal/journal.go`, `internal/journal/journal_test.go`, `internal/tui/viewmodel/viewmodel.go`, `specs/in-progress/tui-slice/tasks.md`, `specs/in-progress/tui-slice/handoff.md`.

### Task 12 — CLI launch wiring

- **Domain/agent**: go-implementer
- **Budget**: complex (9 acceptance items: launch matrix plus wiring proofs)
- **Depends on**: Task 6, Task 10, Task 11
- **Change**: Add the launch rule plus `--colour`/`--color`/`--motion`/`--icons`/`--accessible` flags to `run`, `demo` and `review` (design §2), dispatching to the TUI, the accessible renderer, or the existing linear output.
- **Files**:
  - `internal/cli/tui.go` (launch rule, flag registration, TTY detection)
  - `internal/cli/run.go` (wire flags + dispatch)
  - `internal/cli/demo.go` (wire flags + dispatch)
  - `internal/cli/review.go` (wire flags + dispatch)
  - `internal/cli/tui_test.go`
- **Acceptance**:
  - `TestLaunchRuleMatrix`: at two levels: non-TTY/`--plain`/`--format`/`--accessible` legs through `cli.Main` with piped stdio, and the TTY-selection legs in-package against the launch helper in `tui.go` with a forced-`isTerminal` `tuiDeps` plus recording `runTUI` — `--format jsonl` stays JSONL; `--plain`/non-TTY select linear; `--accessible` selects the accessible stream; TTY-forced stdio (via a `tuiDeps` with forced `isTerminal`) selects the TUI entry (asserted via `tuiDeps.runTUI` recording the branch, since tests cannot run Tea without a PTY); the recording `runTUI` also asserts `cap(cfg.Events) == 256`, proving the live branch feeds `tui.Config.Events` from `tui.LiveFeed` (task 10, design §13) with `Interrupt` set as in `executeRun`, and that `cfg.Notices` is non-nil with a working `Add`/`Drain` round-trip — a wiring that built its own channel, left `Events` nil, or left `Notices` nil fails. `demo` follows the same rule — forced TTY without `--plain` selects the TUI — but offers no `--format` flag, so step 1 never applies to it. Forced TTY with `--format plain` still selects the TUI entry: explicit plain mode is `--plain` only (design §2).
  - `TestInvalidModesExit2`: `--colour rainbow`, `--motion turbo`, `--icons emoji` each exit 2 naming the flag (AC-4.3/AC-4.4); `--after xyz` exits 2 naming the flag.
  - `TestAfterFlagResumesAccessible`: through `cli.Main` with piped stdio, `review <run> --accessible --after <K>` over a fixture run streams only events after K plus the matching `next-after:` trailer; the same invocation without `--accessible` leaves output unchanged (`--after` ignored off its branch).
  - `TestColorAlias`: `--color never` behaves as `--colour never`.
  - `TestUntouchedVerbsStable`: `runs list`, `stop`, `recover`, `apply`, `version`, `doctor` outputs are byte-identical with the new flags absent (N4; golden comparison against pre-change output).
  - `TestQuitLiveDetaches`: quitting the TUI branch while the run is active leaves the run running and prints the `review <run>` reattach line (I06; in-package against the launch helper with a stubbed `runTUI` that records detach vs stop).
  - `TestReviewUnknownRunExits1`: `review run_missing` exits 1 with no TUI started (error path before launch).
  - `TestReviewReceiptlessLaunchesTUI`: `review <run>` of a fixture run in `executing` with no `receipt.written` event and forced TTY selects the TUI entry via the recording `runTUI`; a wiring that hits the receipt gate first exits 1 and fails.
  - `TestActionsWiringHoldsOwnerLockForApply`: with the run's owner lock held in-test, the wired `Apply` returns the owner-held error; with the lock free on a non-ready fixture run, it reaches `ApplyRun` semantics (`ErrApplyBlocked`) instead — proving the wiring acquires the lock per call.
  - `TestActionsWiringStopReadOnly`: the wired `Stop` against a fixture run leaves `mythhelm.db` byte-identical (SHA-256 before/after); a wiring that opened read-write and journaled would fail.
- **Test plan**: `cli.Main` tests asserting stdout/stderr/exit code; JSONL parsed line by line; a `tuiDeps` seam for the Tea program entry so tests do not need a PTY.
- **Invariants touched**: I06 (§7.3: detach semantics preserved), I14 (§16.1: TTY-gated launch never claims unsupported terminals), §15.10 (exit codes unchanged).

### Task 13 — End-to-end launch and fallback

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 12
- **Change**: Cover the packaged binary's launch rule, flag validation and fallback paths end to end (design §15), so packaging or stdio behaviour cannot silently change what users and scripts see.
- **Files**:
  - `tests/e2e/tui_test.go`
- **Acceptance**:
  - `TestE2ENonTTYStaysLinear`: `run` with piped stdout over the fake adapter emits the linear stream with no cursor codes; exit code matches the linear path.
  - `TestE2EPlainForcesLinear`: `--plain` output equals the non-TTY linear output for the same scenario (byte comparison).
  - `TestE2EJsonlStable`: `--format jsonl` with TUI flags present is line-identical to `--format jsonl` without them (N4: machine output unchanged).
  - `TestE2EAccessibleStream`: `--accessible` emits the ordered label stream with no cursor codes and exit code equal to the run outcome.
  - `TestE2EInvalidFlagsExit2`: each invalid `--colour`/`--motion`/`--icons`/`--after` value exits 2 on the packaged binary.
  - `TestE2EReviewUnknownExits1`: `review run_missing` exits 1 with the not-found message on the packaged binary.
- **Test plan**: `tests/e2e` harness (built binary, temp homes/states/repos, fake adapter scenarios); no PTY required.
- **Invariants touched**: §15.10 (exit codes on the final binary).

### Task 14 — G09 evidence, UX session and README status

- **Domain/agent**: maintainer
- **Budget**: standard
- **Depends on**: Task 13
- **Change**: Run the human-only programme — the §16.4 terminal/shell matrix recordings, the §18.6 UX session with a non-Vim user, and one screen-reader session — and publish the README status section (DoD), so every advertised claim has versioned evidence (I14).
- **Files**:
  - `docs/tui-slice-g09-evidence.md` (new evidence record)
  - `README.md` (TUI status section only — disjoint from `docs-site-demos`' demo embed; rebase onto `main` at implementation time if it landed first)
- **Acceptance**:
  - The evidence file records, per tested OS/terminal/shell combination, the §16.4 fields (minimum version, colour mode, Unicode/ASCII, resize, paste, focus, keyboard, mouse — recorded as not offered, D9 — copy, alt-screen restore, interrupted exit, plain and screen-reader results); a combination without a full row is not claimed.
  - The evidence file records the §18.6 session: demo run, native-profile discovery, billing understanding, bounded task start, attention-state recognition, diff inspection, stop/detach, receipt location — with completion, confusion, accidental actions, and the state-distinction checks (the user correctly distinguishes "running", "waiting", "ready for review" and "applied"); a screenshot-only record fails (interaction testing required).
  - The evidence file records one screen-reader/terminal session (NVDA, VoiceOver or Orca) against `--accessible`, or marks all screen-reader combinations experimental (AC-5.4).
  - `README.md` gains a TUI status section listing exactly the recorded combinations as supported and everything else as experimental/unsupported (DoD, I14); a claim without an evidence row fails review.
  - AC-1.1 to AC-7.3 each trace to a named test from tasks 2–13 (traceability table in the evidence file); an untraced AC fails the task.
- **Test plan**: human-run sessions; paste terminal versions, dates and result rows into the evidence file; reviewer checks every claim against a row.
- **Invariants touched**: I14 (§9.14: every advertised capability carries its versioned test result).
