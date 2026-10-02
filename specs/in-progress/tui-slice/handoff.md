# tui-slice — Hand-off

> One section per task. Each task replaces its own `<!-- pending -->` line in its pull request with what
> dependent tasks need: what it produced (the API and files as shipped), what a later task must
> know, and any deviation that changes a later task's inputs. `/run-spec` puts the sections of a
> task's dependencies into that task's dispatch prompt. Open questions and research stay in
> `scratchpad.md`.

## Task 1 — TUI dependencies

- **Produces**: direct requires `github.com/charmbracelet/bubbletea v1.3.10`, `github.com/charmbracelet/lipgloss v1.1.0`, `github.com/charmbracelet/bubbles v1.0.0`, `github.com/mattn/go-isatty v0.0.24` (promoted indirect→direct) in `go.mod`/`go.sum`; `internal/tui/tools.go` pin file (package `tui`, blank imports); PR #93.
- **For dependents** (tasks 5, 6, 12): import the Charm modules and `go-isatty` directly — the pins are held by `tools.go` until real consumers exist; do not delete `tools.go` until at least one real import of each module lands, or `go mod tidy` drops the require.
- **For dependents**: `govulncheck` reports no findings on the new modules; the `licenses`/dependency-review CI legs cover the allowlist (§19.2/§19.4).
- **Deviations affecting later tasks**: none.

## Task 2 — View-model read seam

- **Produces**: `internal/tui/viewmodel/viewmodel.go` — `Snapshot` (`Run`, `Attempt`/`Candidate`/`Verification`/`Receipt` nil-able, `Goal string`, `LatestProgress *Progress`, `NativeExit *NativeExit`, `Admission *Admission`, `LastRunSeq`, `At`), `Load(ctx, dir, runID) (Snapshot, error)`, `EventsSince(ctx, dir, runID, after) ([]journal.Event, error)`; PR #91.
- **Produces**: `Progress{AssistantTurns, ToolUses, Retries, RunSeq}`, `NativeExit{ExitCode *int, Signal *string, ResultObserved, RunSeq}`, `Admission{AdapterID, AdapterVersion, AdapterSurface, Qualified, PaidContinuation, RunSeq}` — all folded from the latest matching event in `run_sequence` order.
- **For dependents**: `Load` opens the DB read-only per call and closes it; poll by re-invoking `Load` (task 10/12) — there is no second fold path to keep in sync.
- **For dependents**: `Goal` is the receipt's `requested_outcome.title` when a receipt loaded, else the digest-checked `task.md` title, else `"unknown"`; a digest mismatch fails `Load` (tasks 6, 11 consume `Goal` directly).
- **For dependents**: missing run wraps `journal.ErrNotFound`; missing database reports `journal.ErrNoDatabase` (empty state, not an error screen); malformed `attempt.native_result`/`admission.decided` fail naming type + `run_sequence`, malformed progress is skipped.
- **Deviation affecting tasks 6, 11**: `Snapshot` carries the extra `Goal string` field (design §3 text requires it; the struct snippet omits it).

## Task 3 — Terminal capability parsing

- **Produces**: `internal/tui/caps`: `Parse(colour, motion, icons string) (Prefs, error)` with `ErrInvalidMode` (check with `errors.Is`; message names dimension and value, e.g. `invalid --colour mode "rainbow"`); `Resolve(p Prefs, getenv func(string) string) Caps`; `Caps{Colour ColourLevel, Motion MotionLevel, Icons IconSet}`; levels `ColourNever/Basic/Full`, `MotionOff/Reduced/Full`, `IconsASCII/Unicode`. As designed.
- **For dependents** (tasks 5, 6, 12): register `--colour`/`--motion`/`--icons` flag defaults as the literal `"auto"` — `Parse` rejects `""` and wrong case. Pass `os.Getenv` as `getenv` in production. `--color` alias is flag-registration only (task 12), not in `caps`.
- **Precedence** (suppression wins): explicit `never`/`off`/`ascii` always holds; `NO_COLOR` set-and-non-empty forces `ColourNever` even over `always`; `TERM=dumb` (exact match) forces all-minimal even over `always`/`full`/`unicode`. Zero `Caps` is all-suppressing (fail-closed).
- **Traps**: `Resolve` never returns `ColourBasic` in this slice (no terminfo grading; `auto` means full). `COLORFGBG` is never read; no OS motion preference is queried (D5).
- **Deviations affecting later tasks**: none.

## Task 4 — Built-in themes in mods/

- **Produces**: `theme.Tokens{Surface, SurfaceRaised, Text, TextMuted, Focus, Attention, OK, Warning, Err, Border}` (`internal/tui/theme/theme.go`); `theme.BuiltIn("dark"|"light")`, `theme.Load(path)`, `(Tokens) Validate()`, `theme.ErrUnknownTheme`, `theme.ErrInvalidTheme`; `themes.Files embed.FS` (`mods/themes/themes.go`); `mods/themes/dark.toml`, `mods/themes/light.toml`. As designed, no API differences.
- **For dependents**: consume `theme.Tokens` by value from `BuiltIn`/`Load` — both validate before returning, so no re-validation. TOML schema is flat snake_case (`surface_raised`, `text_muted`, …); colours strict `#rrggbb`; `border` is `rounded|ascii`.
- **For dependents**: `Validate` enforces WCAG 4.5:1 on Text/TextMuted × Surface/SurfaceRaised — task 9's adversarial-token test must expect low-contrast sets to fail `Validate` (render adversarial tokens directly, not via `Load`).
- **For dependents**: key and token validation `ErrInvalidTheme` errors name the offending key; branch with `errors.Is`, never string matching. `Load` file-open and oversized-file failures return `ErrInvalidTheme` without a key; reads are size-bounded (64 KiB).
- **Deviations that change a later task's inputs**: none.

## Task 5 — Diff viewer component

- **Produces**: `internal/tui/diff.go` — `tui.Diff{Files, Lines, Truncated, TruncateCap}`, `tui.ParseDiff(unified []byte) (Diff, error)`, `(Diff) Render(width int, caps caps.Caps, t theme.Tokens) []string`, `(Diff) TotalLines() int`; `tui.DiffFile{OldPath, NewPath, Header, Hunks}`, `tui.DiffHunk{Header, OldStart, OldCount, NewStart, NewCount, Lines}`, `tui.DiffLine{Kind, Text}`, `tui.DiffLineKind` (`DiffFileBoundary`, `DiffFileMeta`, `DiffHunkHeader`, `DiffContext`, `DiffAdd`, `DiffDel`); caps `maxDiffLines = 50000`, `maxDiffBytes = 4 << 20`; PR #94.
- **For dependents** (task 6): `Render` fits every row (footer included) to `width` cells with `…`/`+` markers that never split a wide glyph; an empty diff renders the `(empty diff)` label row; a truncated diff appends `showing R of N lines (50,000-line limit)` / `(4 MiB byte limit)` where R is `len(Diff.Lines)` and N is `TotalLines()`.
- **For dependents** (task 6): `ParseDiff` takes bare bytes with no provenance, so the selected-change pane owns the exact-command footer — on `Truncated`, build it from `len(Lines)`/`TotalLines()`/`TruncateCap` plus the `CandidateRow` base/commit (TestTruncationFooterNamesExactCommand). Malformed input returns an unexported error, so branch on error presence, not identity.
- **For dependents**: highlighting uses explicit truecolour sequences from theme tokens (deterministic under test, unlike profile-driven styling); `ColourNever`+`IconsASCII` output is pure ASCII with no escapes. Every rendered row passes through `security.TermSafe` before measuring or styling.
- **Deviations affecting later tasks**: the `TotalLines()` accessor is new (the §8 snippet omits it); no API differences otherwise.


## Task 6 — App shell, layouts and mission view

- **Produces**: `internal/tui/model.go` — `tui.Config{RunID, StateDir, Caps, Tokens, Events, Notices, Actions}`, `tui.Run(ctx, cfg) error` (sync `viewmodel.Load`, then alt-screen Tea program; `ErrNoDatabase` shows the empty state, other load failures return), `tui.Model` with `Update`/`View`, `tui.NoticeBacklog` with nil-safe `Add`/`Drain`; `mission.go` (header, tasks, agents, selected-change, status strip, footer, `nextAction` §7 table); `layout.go` (140/100/80-column and 10-row breakpoints, resize identity preservation); `styles.go` (`cell`/`cellOrUnknown` via `security.TermSafe`, geometric-shapes table `─│▶●·…—`); `actions.go` (`Actions` struct only); PR #97.
- **For dependents** (tasks 7–10, 12): drive `Update`/`View` directly in tests (D13); construct via `newModel(cfg)` + `setSnapshot` + `WindowSizeMsg`. `Update` currently handles `WindowSizeMsg` and Ctrl-C only — task 7 owns all other key dispatch. Only Ctrl-C quits; `q` and the palette arrive in tasks 7/9.
- **For dependents**: the selected-change pane drops `Diff.Render`'s in-pane counts footer when truncated and shows the full-width two-line provenance notice instead (counts + cap + exact `` git -C "<workspace>" diff <base> <commit> ``); never render both. The two-pane detail panel shows agents unless `focusPane == "detail"`.
- **For dependents**: `Model.dialog` is a string confirmation id (`""` = none) that resize preserves — task 9 replaces it with rich confirmation prompts. `setSnapshot` preserves `selectedTask` across reloads (defaults to attempt `TaskID`, else run ID) and reloads the diff through the `loadDiff` seam (default: review-style `git diff` over the attempt workspace with 40–64 hex revision validation); tests stub `loadDiff`, never poke `diff`/`diffErr`.
- **For dependents**: fit plain text with `fitLine` before `styleText` — never cut a styled row (the ANSI-splitting hazard); `joinColumns` is the only styled-row joiner. New non-ASCII glyphs must join the `geometricShapes` table or `TestNoFontDependentGlyphs` fails; new files must not name the CLI package or `TestLayeringNoCliImport` fails (layering runs one way).
- **Deviations affecting later tasks**: `NoticeBacklog` lives in `model.go` until task 10 moves it to `live.go` with `LiveFeed` (same API); the truncation notice spans the full width below the columns (task 10's virtualisation must keep it visible when the detail pane shows a truncated diff).

## Task 7 — Navigation, palette and help

- **Produces**: `internal/tui/nav.go` — `handleKey` dispatch (Ctrl-C alone quits), Tab/Shift-Tab + Left/Right pane cycling with `focusPrev`, Up/Down/`j`/`k` index moves, `/` filter input, `Enter` details, `q` exit-options dialog, Esc back-out order dialog→palette→help→filter→previous pane (never quits); `palette.go` — `tui.Action` int enum (`actionStop`, `actionRecover`, `actionApply`, `actionExport`, `actionSwitchPane`, `actionTheme`, `actionHelp`, `actionQuit`), 8-entry `actionRegistry`, substring `filteredActions`, view-only selection routing; `help.go` — contextual full-name overlay; PR #98.
- **For dependents** (tasks 8–10): `Update` routes every key through `handleKey`; add new key branches there, never in `model.go`. Dialog/palette/help/filter states layer (each `Esc` unwinds one); `View` renders the topmost overlay full-screen in dialog→palette→help priority.
- **For dependents** (task 9): palette `Enter` on stop/recover/apply/export closes the palette with no effect — wire effects in `runPaletteSelection` (beware the zero-value `actionDef`, which aliases `actionStop`; the selection guard returns early out of range). `dialogLines` renders only `dialogExitOptions`; replace with `dialogs.go` rich prompts and add confirm keys (dialog keys except `Esc` are currently ignored).
- **For dependents**: the `/` filter applies to tasks/agents rendered rows via `applyFilter` in `tasksColumn`/`agentsColumn`/`focusedPaneLines` (model.go); the detail pane is deliberately exempt so verification status stays visible under any filter (I07) — keep it exempt.
- **For dependents**: `setSnapshot` preserves `selectedTask` and the new `selectedLane` (attempt ID, else run ID) across reloads; `focusPane`/`focusIndex` survive untouched. Theme toggles via `themeName` (`dark`/`light`) reloading `theme.BuiltIn`; overlays and the filter line use only the `geometricShapes` glyphs, so no font-table change was needed.
- **Deviations that change a later task's inputs**: none.

## Task 8 — Truthful motion

<!-- pending -->

## Task 9 — Palette actions and confirmations

<!-- pending -->

## Task 10 — Live render loop and history

<!-- pending -->

## Task 11 — Accessible linear renderer

- **Produces**: `internal/cli/accessible.go` — `cli.AccessibleConfig{RunID, StateDir, Out, Poll, After}` (`Poll <= 0` means 500 ms; `After` streams only `run_sequence > After`), `cli.RunAccessible(ctx, cfg) error`, unexported `accessiblePageSize = 1000`; PR #95.
- **For dependents** (task 12): map `--after <K>` straight to `After`. `RunAccessible` follows live only when the initial history fits one page; an over-page history yields exactly that page plus the trailer and returns without following, so `next-after` never skips unstreamed history. Otherwise it blocks following live until ctx ends, then writes `next-after: <last>` (echoing `After` when nothing streamed) as its last line and returns nil — the launch wiring owns the ctx lifetime (signal cancel for `run`/`demo`, prompt return for `review`). A poll racing cancellation still shuts down clean with the trailer; other `Load`/`EventsSince`/write failures return an error with no trailer.
- **For dependents**: the snapshot summary (goal, run/attempt/candidate/verification/progress/admission/native-exit, next action, full-name actions) emits only when `After == 0` with at most one page of history; over-page and resume invocations are pure `event <seq>: ...` continuations so labels never repeat across pages. Event text reuses `plainEvent` flattened to one line (`; `-joined). Every line is capped at 200 cells with a `... (truncated)` marker, bidi controls become U+FFFD, and changed filenames fit whole with a `... and N more` remainder — no new width dependency (stdlib-only table).
- **Deviation affecting later tasks**: no `approval:` summary line exists (the pipeline has no approval state; `waiting_approval` is never entered) — task 12 must not document or grep for one.

## Task 12 — CLI launch wiring

<!-- pending -->

## Task 13 — End-to-end launch and fallback

<!-- pending -->

## Task 14 — G09 evidence, UX session and README status

<!-- pending -->
