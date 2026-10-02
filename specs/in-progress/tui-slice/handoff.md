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

<!-- pending -->

## Task 5 — Diff viewer component

<!-- pending -->

## Task 6 — App shell, layouts and mission view

<!-- pending -->

## Task 7 — Navigation, palette and help

<!-- pending -->

## Task 8 — Truthful motion

<!-- pending -->

## Task 9 — Palette actions and confirmations

<!-- pending -->

## Task 10 — Live render loop and history

<!-- pending -->

## Task 11 — Accessible linear renderer

<!-- pending -->

## Task 12 — CLI launch wiring

<!-- pending -->

## Task 13 — End-to-end launch and fallback

<!-- pending -->

## Task 14 — G09 evidence, UX session and README status

<!-- pending -->
