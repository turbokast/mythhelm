# TUI Slice — Design

> How the focused Bubble Tea mission view is built on the dogfood run pipeline. Implements master-spec §15.1–§15.10 (slices §15.3 wide view, §15.4 responsive, §15.5 navigation, §15.6–§15.7 motion/capability, §15.8 accessibility, §15.9 render model), feeds G09 (§18.7), and closes §22.2 item 5's TUI half. Normative source: `docs/spec/master-spec.md`; § numbers, I-IDs and G-IDs refer to it. Consumed pipeline: `specs/*/dogfood-slice/` (shipped). Follow-on: `specs/*/docs-site-demos/` FR-3 sequences behind this spec.

## 1. Current state

No TUI exists: `ls internal/tui mods` → both absent (checked 2026-10-02; `internal/` holds adapter, admission, buildinfo, cli, ids, integration, journal, security, statedir, supervisor, workers, workspace).

The run pipeline this view sits on (all opened):

- `mythhelm run` admits then supervises one native attempt: `runRun` (`internal/cli/run.go:24`, grep `^func runRun`) → `executeRun` (`run.go:99`) → `admission.Decide` + `supervisor.Run(ctx, d, supervisor.Hooks{...})` (`run.go:108`).
- `supervisor.Hooks` (`internal/supervisor/pipeline.go:60`, grep `type Hooks struct`) carries `Interrupt`, `Event func(journal.Event)` ("every journaled event of the run, in run_sequence order, as soon as it is journaled"), and `Notice func(string)`. `Outcome` (`pipeline.go:72`) reports where the run stopped.
- Linear output is the `renderer` interface (`internal/cli/render.go:26`, grep `type renderer interface`): `event`/`notice`/`result`/`err`, with `newRenderer` (`render.go:33`) choosing plain or JSONL. `plainRenderer` writes linear text with no cursor movement or colour (`render.go:100-129`). Every stored value passes through `cell` (`internal/cli/runs.go:84`, grep `^func cell`) → `security.TermSafe` (`internal/security/termsafe.go:17`, grep `^func TermSafe`).
- Read seam for the view (all read-only, all opened): `journal.OpenReadOnly` (`internal/journal/projections.go:346`), `Run`/`ListRuns`/`RunsInStates` (`projections.go:379-406`), `LatestAttempt` (`projections.go:409`), `LatestVerification` (`projections.go:422`), `Candidate` (`projections.go:122`), `Events(ctx, runID, afterRunSeq)` (`internal/journal/journal.go:387`), `supervisor.ReadReceipt` (`internal/supervisor/receipt.go:302`), `supervisor.Stop` (`internal/supervisor/stop.go:24`, returns the new state string), `supervisor.RecoverWithHooks` (`internal/supervisor/recover.go:40`), `supervisor.ApplyRun(ctx, j, runID, branch, acceptFlags, acceptUnverified)` (`internal/supervisor/apply.go:63`), `workspace.Git` (`internal/workspace/git.go:49`).
- The candidate diff is produced today by `review.go:134` (grep `no-ext-diff`): `workspace.Git(ctx, dir, false, "diff", "--no-color", "--no-ext-diff", "--no-textconv", base, commit)` over the attempt workspace, with `base`/`commit` validated as 40–64 hex (`review.go:19,127`).
- States: `RunState` 13 values, `AttemptState` 9 values (`internal/supervisor/state.go:20-70`). One run at a time: `activeStates` (`internal/supervisor/pipeline.go:54`).
- CLI verbs: the `commands` map (`internal/cli/dispatch.go:29`, grep `var commands`): run, runs, review, stop, recover, apply, version, demo, doctor. Exit codes `ExitOK..ExitCancelled` (`internal/cli/exit.go:18-28`, §15.10 mapping in `exitCode`/`runExit`). `run` already has `--plain` ("the only text output in this build", `run.go:40`) and every verb except `demo` has `--format plain|jsonl` (`demo` hardcodes the plain renderer, `internal/cli/demo.go:93`, grep `newRenderer`).
- Dependencies (`go.mod`, grep `^require`): BurntSushi/toml v1.6.0, x/sys v0.47.0, modernc sqlite; `mattn/go-isatty v0.0.24` is already an indirect requirement. No Charm modules yet. `go.sum` exists.
- End-to-end pattern: `tests/e2e/main_test.go` builds `cmd/mythhelm` once; tests run the packaged binary against temp homes/states/repos. CI has lint, govulncheck and licence jobs (dogfood tasks 2–3); `.golangci.yml` exists.

## 2. CLI surface: launching and inspecting the view (§15.10, N4)

No new verbs (N4). `run`, `demo` and `review` gain the launch rule; `runs list`, `stop`, `recover`, `apply`, `version`, `doctor` keep byte-identical behaviour.

**Launch rule** (evaluated in this order, in one shared helper so the three verbs agree):

1. `--format jsonl` → existing JSONL output, unchanged (`demo` offers no `--format`, so step 1 never applies to it — it is evaluated from step 2; it gains presentation flags but no new machine output in this slice).
2. `--plain`, or stdout not a TTY, or `TERM=dumb` → existing linear plain output, unchanged.
3. `--accessible` → the new accessible linear renderer (§12), no cursor movement, no spinners.
4. Otherwise → the Bubble Tea TUI: live mode for `run`/`demo` (wired to the pipeline's `Hooks`), inspect mode for `review <run>` (snapshot plus polling).

TTY detection promotes the already-required `mattn/go-isatty` to a direct dependency (D12).

**New flags** on `run`, `demo`, `review` only: `--colour auto|always|never` with `--color` alias, `--motion auto|full|reduced|off`, `--icons auto|unicode|ascii` (§15.7), `--accessible` (D10), and `--after <seq>` (resume offset for the `--accessible` stream, §12; honoured only on the `--accessible` branch and ignored elsewhere, like other presentation flags off their branch). Bad values (including a non-numeric `--after`) exit 2 through the existing `usageError` path (`exit.go:30-37`, grep `type usageError`). `--plain` keeps its meaning: linear text, no cursor movement, no colour. Only `run` has `--plain` (`run.go:40`); `review` and `demo` gain none, so their on-TTY linear paths are piping, `TERM=dumb`, or `--accessible` — and `review`'s default `--format plain` (`review.go:23`) does not block the TUI (N4). "Explicit plain mode" (AC-2.5) means exactly the `--plain` flag leg of launch-rule step 2: `--format plain` is a machine-format default, not an explicit request, so on a TTY it still launches the TUI. In `review`, the launch rule is evaluated after the run lookup (`review.go:51-54`) but before the receipt gate (`review.go:55-58`): the TUI and `--accessible` branches launch without a receipt, while the linear branches keep the existing gate.

**Test seams** (consumed by task 12): the launch helper takes its dependencies as an explicit struct — never mutable package state (go-conventions):

```go
// internal/cli/tui.go
type tuiDeps struct {
    isTerminal func(w io.Writer) bool
    runTUI     func(ctx context.Context, cfg tui.Config) error
}
```

TTY detection goes through the `isTerminal` field (production: go-isatty on the stdio writer; tests force either branch); the Tea program starts through the `runTUI` field (production: `tui.Run`; tests record the selected branch without needing a PTY). `tui.go` defines the struct, the production value and the launch helper that threads it; tests construct their own `tuiDeps`.

**Exit behaviour.** Exit codes keep their §15.10 meanings. Quitting the TUI while its run is still active does not stop the run: `q` opens exit options ("detach (leave running)" / "request stop" / "cancel"), matching the §15.3 footer ("q exit options"); leaving a live run detaches exactly like the second Ctrl-C today (I06: ownership survives the UI exiting), and the screen names the reattach command (`review <run>`). After the run ends, quitting returns the run's exit code via the existing `runExit` mapping.

## 3. View-model: the one read seam (Q1 → option b) (§15.1, §15.9)

New package `internal/tui/viewmodel`: a small snapshot type over the journal plus receipt, so every pane renders one consistent picture. Direct journal reads in each pane would scatter the "unknown vs absent" logic (I09) and the receipt/state cross-check (`review.go:59-61`); a new query seam in `internal/journal` would churn the shipped pipeline for one consumer.

```go
// internal/tui/viewmodel/viewmodel.go
type Progress struct {
    AssistantTurns int            // latest attempt.progress counters (native-reported)
    ToolUses       map[string]int // per-tool use counts from the latest event
    Retries        int
    RunSeq         int64          // run_sequence of the event it came from
}
type NativeExit struct {
    ExitCode       *int    // nil when unreported or signalled (I09)
    Signal         *string // nil when unreported or a clean exit (I09)
    ResultObserved bool
    RunSeq         int64   // run_sequence of the event it came from
}
type Admission struct {
    AdapterID        string // admission.decided adapter.id
    AdapterVersion   string // adapter.version
    AdapterSurface   string // adapter.surface
    Qualified        bool   // billing.qualified
    PaidContinuation string // billing.paid_continuation ("off"/"unknown"/...)
    RunSeq           int64  // run_sequence of the event it came from
}
type Snapshot struct {
    Run          journal.RunRow
    Attempt      *journal.AttemptRow      // nil when the run has no attempt yet
    Candidate    *journal.CandidateRow    // nil until candidate.frozen projects
    Verification *journal.VerificationRow // nil until verification completes
    Receipt      supervisor.Receipt       // nil until receipt.written journals
    LatestProgress *Progress             // nil when no attempt.progress journaled yet
    NativeExit     *NativeExit           // nil when no attempt.native_result journaled yet
    Admission      *Admission            // nil until admission.decided journals
    LastRunSeq   int64                    // max run_sequence consumed
    At           time.Time                // snapshot time, UTC
}
func Load(ctx context.Context, dir, runID string) (Snapshot, error)
func EventsSince(ctx context.Context, dir, runID string, after int64) ([]journal.Event, error)
```

- `Load` opens the database with `journal.OpenReadOnly` (never the read-write `Open`), reads `Run` (a missing run returns the wrapped `journal.ErrNotFound`), `LatestAttempt`, then `Candidate` (keyed by attempt ID, so it is read after `LatestAttempt`) and `LatestVerification` (each `ErrNotFound` becomes a nil field, never an error), and applies the `review.go:59-61` receipt/state agreement check whenever a receipt was loaded. Goal title comes from `receipt["requested_outcome"]["title"]`, falling back to the digest-checked task title — `Load` reads `<stateDir>/runs/<runID>/task.md` and applies the `receipt.go:151-156` SHA-256 check, using `admission.TaskTitle` on match and "unknown" on missing file — the same check `BuildReceipt` performs, so the title is never unverified (I07).
- Neither `AttemptRow` (no progress fields, `internal/journal/projections.go:47-57`) nor the receipt's `native_result` (no exit code/signal, `internal/supervisor/receipt.go:179-180`) carries what the agents pane (§7) and the `native exit:` label (§14) consume, and the `qualified`/`paid continuation` billing labels live only in the `admission.decided` payload (`admission.Record`, `internal/admission/admission.go:507`), so `Load` additionally scans `Events(ctx, runID, 0)` in `run_sequence` order and folds the latest `attempt.progress` payload (`assistant_turns`, `tool_uses`, `retries` per `internal/workers/worker.go` `progress.flush`) into `LatestProgress`, the latest `attempt.native_result` payload (`exit_code`, `signal`, `result_observed` per `worker.go` `nativeResult`) into `NativeExit`, and the latest `admission.decided` payload (adapter descriptor, `billing.qualified`, `billing.paid_continuation`) into `Admission`. `LastRunSeq` is the max `RunSequence` observed in the scan (0 when the run has no events). A full scan per `Load` is cheap: progress emits at most one event per second (`worker.go` `progressInterval`), so even long runs scan only thousands of rows.
- Receipt handling: `ReadReceipt` (`receipt.go:302`) reports a missing receipt as a wrapped `os.ReadFile` error, never `journal.ErrNotFound`, so `Load` never calls it blindly — it calls `ReadReceipt` only when the scan saw a `receipt.written` event, and leaves `Receipt` nil otherwise. The file is always written before its event is journaled (`pipeline.go:747` before `:751`), so an event without a file means deletion or corruption and its `ReadReceipt` error fails `Load`, as do SHA-mismatch, run-ID-mismatch and JSON errors; a file without an event (the crash window) is unverified and correctly ignored.
- Scan failure cases: an `Events` query error fails `Load` with the wrapped error (same as other read failures); a malformed `attempt.progress` payload (bad JSON or wrong shape) is skipped with the latest good value kept, since progress is advisory — `LastRunSeq` still advances past it; a malformed `attempt.native_result` or `admission.decided` payload fails `Load` with an error naming the event type and `run_sequence`, since completion and decision evidence must never be silently wrong (I07). Unknown payload keys are ignored; missing keys stay nil/`unknown` (I09).
- `EventsSince` opens read-only and returns `Events(ctx, runID, after)` for history search (§13) and the accessible stream (§12). The §13 tick rebuilds the snapshot by re-invoking `Load`, so `LatestProgress`/`NativeExit` stay current with no second fold path.
- Failure cases: a state dir with no database → `journal.ErrNoDatabase` → the TUI shows the "no runs recorded" empty state; unknown run ID → `journal.ErrNotFound` → error screen, exit 1 (same mapping as `review` today).
- The package imports `journal` and `supervisor` (receipt type) for reads only; it never calls `journal.Open`/`Append` (I18 supporting evidence; a task acceptance item asserts this structurally).

## 4. Capabilities (§15.7)

New package `internal/tui/caps` (one file, no OS splits):

```go
// internal/tui/caps/caps.go
type ColourLevel int // ColourNever, ColourBasic, ColourFull
type MotionLevel int // MotionOff, MotionReduced, MotionFull
type IconSet int     // IconsASCII, IconsUnicode
type Prefs struct{ Colour, Motion, Icons string } // raw flag values
type Caps struct{ Colour ColourLevel; Motion MotionLevel; Icons IconSet }
var ErrInvalidMode = errors.New("invalid capability mode")
func Parse(colour, motion, icons string) (Prefs, error)
func Resolve(p Prefs, getenv func(string) string) Caps
```

- `Parse` rejects anything outside the §15.7 enumerations (`ErrInvalidMode`, exit 2). `--color` is accepted as an alias at flag-registration time.
- `Resolve` applies the precedence: explicit `never`/`off`/`ascii` always wins; `NO_COLOR` (set and non-empty) forces `ColourNever` without touching motion (AC-4.3); `TERM=dumb` forces all-minimal; `COLORFGBG` is never consulted — with all flags `auto` and an otherwise empty environment, a set `COLORFGBG` yields the same `Caps` as an unset one (AC-4.4); OS reduced-motion preference is treated as unknown on every platform this slice — §15.7 names the explicit motion flag the portable control, and no OS query is reliable enough to silently override it (D5).
- Colour suppression is enforced by rendering through a restricted style set when `Colour == ColourNever` (no ANSI colour sequences emitted at all, asserted by a test that scans golden output for `\x1b[` colour introductions); Unicode suppression by using the ASCII glyph table (§7).

## 5. Themes and mods/ (§15.2, §14.6, §20.3)

Stage 1 delivers "one declarative theme format" (§20.3) while N4/N3 keep extension points out: `mods/themes/dark.toml` and `mods/themes/light.toml` are declarative data parsed with the already-required BurntSushi/toml, compiled into the binary via `mods/themes/themes.go` (package `themes`, `//go:embed *.toml`, exposing the TOML bytes as `themes.Files`), with no install/enable flow (MH-6 owns that). The `embed` directive lives next to the TOML files because `go:embed` patterns are relative to the source directory and forbid `..`, so `internal/tui/theme/` cannot embed `mods/` directly; the `themes` package holds only the embedded data (following the top-level `adapters/` precedent for a data-only package outside `internal/`), while parsing and validation stay in `internal/tui/theme`. Keymap and layout stay compiled-in Go constants (D6): shipping three half-validated schemas would weaken G08's "cannot hide required controls" review, and §14.6's keymap/layout mods arrive with the plugin protocol.

```go
// internal/tui/theme/theme.go
type Tokens struct {
    Surface, SurfaceRaised, Text, TextMuted string // hex colours
    Focus, Attention, OK, Warning, Err       string
    Border                                   string // "rounded" | "ascii"
}
var (
    ErrUnknownTheme = errors.New("unknown theme")
    ErrInvalidTheme = errors.New("invalid theme")
)
func BuiltIn(name string) (Tokens, error) // "dark" | "light" parsed from themes.Files bytes
func Load(path string) (Tokens, error)    // TOML file; unknown keys rejected
func (t Tokens) Validate() error          // hex shape + text/surface contrast
```

```go
// mods/themes/themes.go
package themes

import "embed"

//go:embed *.toml
var Files embed.FS
```

- `BuiltIn` reads the named TOML bytes from `themes.Files` (`github.com/turbokast/mythhelm/mods/themes`) and parses them exactly as `Load` parses a file, so built-ins and user files share one validation path.
- `Load` rejects unknown keys (a typo must not silently restyle) and non-hex colours with `ErrInvalidTheme` naming the key.
- `Validate` enforces a 4.5:1 luminance contrast ratio for text-on-surface pairs in both built-ins (checked in tests, per §15.2 "checked for contrast in supported colour modes").
- A theme can never hide required controls: required surfaces (approval prompt, spend warning, stop state) always render with text labels independent of theme tokens (G08; acceptance in task 9's `TestRequiredControlsSurviveAdversarialTheme`: render each with a maximally adversarial in-test token set and assert the labels survive — the surfaces only exist from tasks 6/9, so the test cannot live with the theme format).

## 6. Layouts (§15.4, Q5)

The §15.4 table is implemented literally: ≥140 wide → tasks + agents + selected detail (+ optional compact global status); 100–139 → two panes with switchable detail; 80–99 → single focus pane with labelled tabs and persistent critical status; <80 wide or very short → compact task/status view with an offered linear mode. Height is independent of width. "Very short height" (Q5) means **fewer than 10 rows**; the AC-2.4 test uses ≤5 rows.

Resize (AC-2.6, I06) is handled on `WindowSizeMsg` by re-laying out around preserved identity: the selected task ID, focused pane ID and any pending approval dialog survive every resize; the focus index is clamped, never reset (if the list shrank, it clamps to the last row); a resize never moves a destructive action under an already-held key because confirmations are modal dialogs anchored to the screen centre, not to a list position that reflows. Non-TTY/`TERM=dumb`/plain requests never reach this code (launch rule §2).

## 7. Mission view (§15.1, §15.3)

One Bubble Tea program (`internal/tui` root) renders the wide layout's panes from a single `viewmodel.Snapshot`:

- Header: mission goal (receipt title, else digest-checked task title per §3, else `"unknown"`), run ID, adapter lane label, overall state.
- Tasks pane: the run's task row(s) with state chips. Stage 1 runs carry one task; the pane lists it with its state and reason, ready for Stage 2's DAG without a redesign.
- Agents pane: one lane per attempt, each with a readable runtime/profile label (`adapter_id` + execution profile, e.g. `claudecode / trusted-host`), a stable local identifier (the attempt number with a lane letter: `A · attempt 1`), current activity (`Snapshot.LatestProgress` — the latest `attempt.progress` counters, labelled native-reported — or the attempt state), and workspace path. Never a vendor logo as primary navigation (AC-1.3).
- Selected-change pane: the candidate diff (§8) or its labelled absence ("no candidate frozen yet"), plus verification status with the checked revision (§14, I07).
- Status strip: billing posture (`Run.BillingPosture`) and quota labels from `Snapshot.Admission` (qualified flag, paid continuation), and the next required action from this table:

| Run state | Next action |
|---|---|
| created, admission | `admitting — no action` |
| executing, verifying | `none — agent working (stop available)` |
| stopping | `waiting for worker confirmation — stop requested, not confirmed` (I06) |
| ready_for_review (verified) | `review the candidate, then apply or close` |
| ready_for_review (unverified) | `verification unavailable — apply needs explicit unverified acceptance` |
| blocked, failed | `read the reason; recover or start a new run` |
| interrupted | `recover the run` |
| recovering | `recovery running — no action` |
| applying, completed, cancelled | `none — run is terminal` |

- Footer: `Tab focus · / filter · : commands · ? help · Enter inspect · q exit options` (the §15.3 key line).

State presentation (AC-1.4, AC-5.2): every state is text plus a shape/icon with optional colour (`[run ]`, `[wait]`, `[pass]`, `[req ]`, `[fail]` in the §15.3 style); decorative glyphs have ASCII alternatives selected by `Caps.Icons`; estimated, reported, observed and unknown values stay visually distinct (unknown renders as the word `unknown`, never `0` or blank). Nerd Fonts are never required: no glyph outside the ASCII table and widely-supported geometric shapes is load-bearing (AC-1.5, I13).

## 8. Diff viewer (§15.3, AC-1.2)

New file `internal/tui/diff.go`: parses the unified diff bytes produced exactly as `review.go` does (§1) into hunks and renders them width-safe — every line is measured with `lipgloss.Width` (pinned in task 1; present in lipgloss v1.1.0 `size.go:15`, verified via the module proxy), never `len()` — tabs expanded to the next multiple of 8, overlong lines truncated with a visible `…`/`+` marker, never wrapped mid-glyph. Syntax-aware presentation means structural highlighting (hunk headers, `+`/`-` markers, file boundaries) in theme colours where `Caps` allows; there is no per-language grammar highlighting in this slice, and Markdown rendering is never substituted for the diff component (acceptance: a diff over a `.md` file still renders diff chrome, asserted structurally).

```go
// internal/tui/diff.go
type Diff struct {
    Files []DiffFile // parsed from unified diff bytes
    Lines []DiffLine // flattened render rows with widths precomputed
    Truncated bool   // true when the input exceeded maxDiffLines or maxDiffBytes
    TruncateCap string // which cap truncated the input: "lines" or "bytes" (only meaningful when Truncated)
}
const maxDiffLines = 50000
const maxDiffBytes = 4 << 20 // 4 MiB: same order as 50k typical lines, so neither cap dominates normal diffs
func ParseDiff(unified []byte) (Diff, error)
func (d Diff) Render(width int, caps caps.Caps, t theme.Tokens) []string
```

- Inputs beyond `maxDiffLines` set `Truncated`. Inputs beyond `maxDiffBytes` are truncated to the budget (cut back to the last newline within budget, so no partial row is parsed) before splitting, parsing and width measurement, and also set `Truncated` — a single giant line can no longer blow the retained buffer or the measurement work. `ParseDiff` takes bare bytes with no provenance, so the selected-change pane (task 6, which has the `CandidateRow`) labels either cap explicitly, never silently (D7). The label shows the retained line count actually rendered — `len(Diff.Lines)`, not the cap — over the total N counted from the full input (an O(1)-memory scan), and names the cap that fired from `TruncateCap`: "showing R of N lines (50,000-line limit) — full diff via `git -C "<workspace>" diff <base> <commit>`" vs "showing R of N lines (4 MiB byte limit) — full diff via `git -C "<workspace>" diff <base> <commit>`". The byte cap fires first when both would apply, so the label never claims 50,000 rendered lines when the byte budget retained fewer.
- The viewer is virtualised: only visible rows render (§13); the full parsed buffer stays searchable.

## 9. Navigation (§15.5)

Keys (compiled-in defaults, D6): arrows + Tab/Shift-Tab move focus; `j`/`k` move within a list as accelerators (an addition, never required); `/` filters the focused collection; `:` opens the searchable command palette; `?` opens contextual help; `Enter` opens details; `Esc` backs out (closes dialog → clears filter → previous pane, never quits); `q` opens exit options (§2). Mouse is not offered in this slice, so AC-3.2 holds vacuously and every action is keyboard- and plain-output-reachable by construction (D9). Each palette entry maps to a `tui.Action`: `type Action int` with an unexported const block (`actionStop`, `actionRecover`, `actionApply`, `actionExport`, `actionSwitchPane`, `actionTheme`, `actionHelp`, `actionQuit` — the §10 list); the palette lists every registered action.

Focus is visible (theme focus ring plus a `▶`/arrow marker that survives `ColourNever`) and stable: panes and rows carry stable IDs, and selection is re-resolved by ID after every snapshot reload, so a background update never steals or drops focus (AC-5.2). Help lists full action names, never bare keys alone. Long content pages inside a scrollable pane and exports to plain text via the palette (§12, AC-5.3).

## 10. Actions (§15.5, I03, I06, I18)

The palette and key bindings offer: stop, recover, apply, export view, switch tab/pane, toggle theme (dark/light), help, quit options. View-only actions execute immediately; stop, recover, apply and quit-on-live require explicit labelled modal confirmations naming the exact effect (AC-3.3, I03): e.g. apply shows run, candidate commit, target branch, flags accepted, and verification state, with `[Apply to <branch>] [Cancel]` — no single accidental keypress confirms, and `Esc` always cancels.

Mutations reuse the exact supervisor entry points the CLI uses — never a second supervisor, worker or journal writer (I18):

```go
// internal/tui/actions.go — injected, so tests use fakes
type Actions struct {
    Stop    func(ctx context.Context, runID string) (string, error)
    Recover func(ctx context.Context, runID string, h supervisor.Hooks) (supervisor.RecoveryOutcome, error)
    Apply   func(ctx context.Context, runID, branch string, acceptFlags, acceptUnverified bool) (supervisor.Receipt, error)
}
```

- Stop is enabled in `executing`/`verifying`; after the call the run renders `stop requested` until `attempt.stopped` is journaled — requested is never labelled stopped (I06).
- Recover is enabled only in `interrupted`; it runs `RecoverWithHooks` with hooks from `tui.LiveFeed` feeding the TUI live feed (§13), showing live recovery progress.
- Apply is enabled only in `ready_for_review`; the dialog collects the branch name plus the two acceptance toggles and shows the §15.3-style verification table before confirming.
- Export writes the focused pane's full plain-text content to `<stateDir>/exports/<runID>-<pane>.txt` and shows the path (D8): deterministic, no path prompt to mistype, outside any repository.
- Production wiring (task 12, in `internal/cli/tui.go`): the wired `Stop` opens the database read-only per call (mirroring `internal/cli/stop.go:52-57` — the `journal.OpenReadOnly` plus `supervisor.Stop` call in `runStop`, verified live; the same line span in `internal/supervisor/stop.go` is `attempt.stopped` payload parsing, which never opens the journal); `Recover` opens read-write per call (mirroring `recover.go`, grep `journal.Open`); `Apply` opens read-write and holds `supervisor.AcquireOwner(<stateDir>/runs/<runID>)` for the call (mirroring `apply.go:59-72`, since `ApplyRun` requires the caller to hold the lock — `internal/supervisor/apply.go:62`, grep `must hold the run's owner lock`). Every handle is closed before its call returns; no journal handle outlives the action that opened it.

## 11. Motion (§15.6, FR-4, Q2 → option a)

Applicable subset for a single-agent, single-writer product; moments without a trigger are honesty-register rows, not stubs:

| Moment | Implementation | Truth rule kept |
|---|---|---|
| 1 arrival | brief focus reveal while the first snapshot loads | interruptible: any key skips; never delays first paint |
| 2 route selected | admission decision card (adapter descriptor from `Snapshot.Admission`, profile from `Run.ExecutionProfile`, reason `pinned by --adapter`) | shows the recorded adapter + profile; the pinned-route reason is static text (no routing in this slice, cf. receipt `routing.decision`); no fake deliberation |
| 3 dispatch | lane highlight transfer | lane renders running only after `attempt.launched` is journaled |
| 4 live work | small activity pulse on `attempt.progress` | no typewriter delay; pulse means transport alive, not progress proven |
| 6 waiting | restrained indicator + static reason | no countdown: Stage 1 has no known resets/deadlines |
| 8 integration | frozen → checked → ready stages light from `candidate.frozen`/`check.completed`/`verification.completed` | no percentage (no denominator); "combined" has no single-writer trigger (register) |
| 9 delivery | short ready-for-review reveal, ≤200 ms | celebrates only the actual result; disabled under reduced motion |
| 10 failure/recovery | focus moves to the actionable error + preserved artifact | no shake, no flashing, no strobe |

Deferred: 5 handoff and 7 reservation conflict (no multi-agent triggers yet). Reduced/off motion (flag or `Caps`) collapses every moment to an immediate state change with static emphasis (AC-4.2). Transitions run roughly 80–200 ms; there is no permanent animation loop — at most brief ≤60 fps motion (AC-6.1).

## 12. Accessibility (§15.8, Q3 → option a)

`--accessible` selects a linear screen-reader renderer in `internal/cli` (new file `accessible.go`, core domain — it is linear output, not the TUI), consuming the same `viewmodel` snapshots and `EventsSince` stream as the TUI so labels cannot drift between the two surfaces:

```go
// internal/cli/accessible.go
type AccessibleConfig struct {
    RunID string
    StateDir string
    Out io.Writer
    Poll time.Duration // snapshot re-read cadence; default 500ms
    After int64        // resume offset: stream only events with run_sequence > After (0 = from the start)
}
const accessiblePageSize = 1000
func RunAccessible(ctx context.Context, cfg AccessibleConfig) error
```

It emits complete state labels (`run state: executing (reason: none); attempt 1: running; verification: waiting`), never repeats spinners, never rewrites the cursor, and streams meaningful changes in journal order. AC-5.2 holds by construction (text-only, full names, no colour/animation-only meaning). AC-5.3: bidi/combining/emoji/wide glyphs are measured by cell width and truncated safely so approval-adjacent controls never displace; filenames render in full or with labelled truncation. Long runs page through repeated invocations: each invocation streams an initial history page of at most `accessiblePageSize` events after `After` (via `EventsSince`), then follows live until ctx ends; on clean return it prints `next-after: <seq>` as its last line, where `<seq>` is the max streamed `run_sequence` (echoing `After` when nothing streamed), so the next invocation resumes with `--after <seq>` (AC-5.3). The TUI export action (§10) covers in-TUI export.

Advertised support (Q3, AC-5.4, I14): exactly the combinations the evidence task actually tests (default: one — the maintainer's NVDA/VoiceOver/Orca session recorded at run time); every other combination is labelled experimental or unsupported in the README status section (DoD). No claim is made before its test programme runs (N5).

## 13. Render model (§15.9)

Event-driven, never a render loop:

- Live mode (`run`/`demo`): the pipeline's `Hooks.Event`/`Notice` feed the Tea program through one constructor (task 10 `Produces`):
  ```go
  // internal/tui/live.go
  type NoticeBacklog struct {
      mu sync.Mutex
      pending []string
  }
  func (b *NoticeBacklog) Add(s string)    // appends; never blocks, never drops
  func (b *NoticeBacklog) Drain() []string // atomically takes all pending, in order
  func LiveFeed() (events chan journal.Event, notices *NoticeBacklog, hooks supervisor.Hooks)
  ```
  `LiveFeed` creates the events channel with capacity 256 and returns `Hooks` whose `Event` uses a non-blocking send (`select` send with `default` drop — D18), so a paused TUI can never stall native event consumption; the next poll tick replays dropped events from the journal. `Notice` (`func(string)`, `pipeline.go:68` — "status the user must see that is not an event", e.g. `DetachedNotice`'s `mythhelm recover` command) bypasses the drop-on-full channel into the returned `NoticeBacklog`: `Add` never blocks and never drops, so actionable unjournaled status is retained even when the events channel is full. The backlog stays tiny in practice — the pipeline emits notices only on its ~10 error-path call sites (`pipeline.go`, `recover.go`), each a short string fired O(1) times per run. The model drains the backlog on every 200 ms poll tick (task 10 wiring), mapping each drained string to a synthetic event — `journal.Event{Type: "ui.notice", Payload: {"text": s} JSON-encoded, RunSequence: -1, ObservedAt: drain time UTC}` — at drain time; `RunSequence: -1` marks it synthetic (no journal row ever carries it) and the model renders it as a notice line, never as journal progress. `Interrupt` is left unset by `LiveFeed`; the launch wiring (task 12) sets it from `signal.Notify` exactly as `executeRun` does (`run.go:105-108`, capacity 2). A 200 ms `tea.Tick` rebuilds the snapshot with a fresh `Load` (scan included) at most 5 Hz (coalesced view updates); the supervisor preserves critical events independently, so coalescing loses nothing durable.
- Inspect mode (`review`): the same 200 ms tick without the live channel; quitting is instant.
- WindowSizeMsg triggers re-layout only; key messages update focus/dialog state synchronously so input never waits on I/O.
- Virtualisation: lists and diffs render through a viewport showing at most the pane's visible rows; the live stream tail shows at most the visible rows on screen while history search replays the journal via `EventsSince(ctx, dir, runID, 0)` with a caller-side filter — the history is therefore always complete, never a bounded memory copy (D17).
- Every goroutine (live channel pump, tick) is owned by the Tea program's context and joined on quit (go-conventions concurrency).

## 14. State truthfulness (I06, I07, I09)

- I06: stop requested vs stopped (§10); detach wording (§2); resize preservation (§6); the next-action table (§7) never promises what the state machine has not confirmed.
- I07: the verification panel always pairs the checked revision with the candidate revision (`VerificationRow.CandidateCommit` beside `CandidateRow.Commit`); when they differ it reads `checks ran against <short> — candidate is <short>` and never `verified`. `Snapshot.NativeExit` (folded from the latest `attempt.native_result` event — an agent completion report) renders as `native exit: ...`, never as verification — a dedicated test journals a native_result event with exit 0, loads the snapshot through `Load`, and asserts no `verified`/`pass` label appears.
- I09: `LatestProgress` counters render as `N assistant turns (native-reported)`; quota/cost figures carry their kind labels from `Snapshot.Admission` (`qualified: true/false`, `paid continuation: off/unknown`); absent values render `unknown`, never `0` or blank. The existing `payload.text` helper (`render.go:149`) already encodes the unknown convention for linear output; the TUI mirrors it in its cell formatter. Every string the TUI renders passes through `security.TermSafe` (`internal/security/termsafe.go:17`, `func TermSafe(s string) string`, verified live) in `styles.go` before measuring or styling, so control characters and escape sequences from journaled values are inert before they reach the terminal (§12.7).

## 15. Tests and evidence (G09, §18.6, §16.4, Q4)

- Unit (table-driven, in-package): viewmodel against temp state databases (built with `journal.Open` + `Append` fixtures); caps parse/resolve matrix (NO_COLOR/TERM/dumb/flag precedence); theme validation + contrast + adversarial-token control survival; diff parse/render goldens (unicode, wide, tabs, truncation marker); key handling and focus stability by driving `Update`/`View` directly as pure functions — no teatest-style PTY dependency (D13); layout goldens at 60/85/120/160 columns × normal/short heights; confirmation dialog gating (every destructive action needs its labelled confirm, `Esc` cancels).
- Structural: a test asserts `internal/tui/viewmodel` never calls the read-write journal API (via `go list` dependency inspection in-test: `golang.org/x/tools` is NOT added — instead the test shells `go list -deps` — no; simplest honest check: viewmodel's only journal symbols are referenced through a narrow local interface, and a test in the package fails to compile if `journal.Open` is referenced... over-clever). Decision: the check is a `grep`-based test — `TestViewmodelNeverWrites` runs `grep -rn "journal\.Open(\|journal\.Append(" internal/tui/viewmodel/*.go` excluding the test itself and fails on any hit. The open paren anchors the match so the legitimate `journal.OpenReadOnly` call is not flagged. Crude, explicit, and it bites (removing the exclusion or adding a call fails it). Same shape guards `internal/tui` against importing `internal/cli` (layering: `cli` → `tui`, never back). Every such absence-by-grep test asserts its scanned file set is non-empty before asserting absence, so a wrong scan path fails instead of false-passing.
- End-to-end (`tests/e2e`, packaged binary, piped stdout = non-TTY so no PTY needed): the launch-rule matrix — non-TTY `run` stays linear; `--plain` forces linear on a TTY (tested with `script(1)` where available, else unit-tested TTY branch); `--format jsonl` output byte-identical with and without TUI flags present; `--accessible` emits the ordered label stream; invalid `--colour`/`--motion`/`--icons` values exit 2; `review` of an unknown run exits 1 with no TUI started.
- G09 evidence (maintainer task, Q4): the advertised matrix starts as the dogfood CI OS legs (Linux/macOS/Windows incl. ARM) for plain/non-TTY/resize-safe behaviour plus the terminals/shells the maintainer actually records (minimum: the maintainer's own terminal + one screen-reader session). For each recorded combination the evidence file captures the §16.4 fields (colour mode, Unicode/ASCII, resize, paste, focus, keyboard, mouse — recorded as not offered, D9 — copy, alt-screen restore, interrupted exit, plain/screen-reader results). The §18.6 UX session (non-Vim user: demo → native profile → billing → bounded task → attention states → diff → stop/detach → receipt) records completion, confusion, accidental actions and the running/waiting/ready-for-review/applied state-distinction checks. Everything unrecorded stays experimental/unsupported in the README (I14).

## 16. Decisions

| ID | Decision | Rationale |
|---|---|---|
| D1 | TUI by default on TTY for `run`/`demo`/`review`; `--plain`/non-TTY/dumb stay linear | §15.1's focused view is the Stage 1 default; scripts keep byte-identical linear output via the existing flags (N4's "launching the view" only) |
| D2 | Quitting a live TUI detaches (exit options), never stops | I06: ownership survives the UI exiting; matches the second-Ctrl-C semantics users already have |
| D3 | View-model package over the journal (Q1 option b) | One read seam keeps I09 labelling and the receipt/state cross-check in one place; avoids churning the shipped pipeline for a new query seam |
| D4 | 200 ms poll plus in-process wake channel | One mechanism covers live and inspect modes; polling the read-only journal cannot stall writers |
| D5 | OS reduced-motion treated as unknown; the flag is the control | §15.7 explicitly names the explicit flag the portable control; no OS query here is reliable enough to override it silently |
| D6 | Only themes are declarative data in `mods/`; keymap/layout stay code | Three half-validated schemas would weaken the G08 review; keymap/layout data formats arrive with MH-6 (N3) |
| D7 | Diff input capped at 50,000 lines and 4 MiB with explicit labels | Unbounded in-memory diffs risk OOM on huge candidates; each cap is labelled with the exact `git diff` fallback, never silent |
| D8 | Export writes to `<stateDir>/exports/<run>-<pane>.txt` | Deterministic, outside any repo, no path prompt to mistype |
| D9 | No mouse support this slice | Keeps AC-3.2 vacuous and every action keyboard-reachable; mouse arrives only with a full keyboard-parity audit |
| D10 | `--accessible` names the screen-reader mode | Short, matches §15.8 "accessibility"; `--screen-reader` rejected as longer with no extra clarity |
| D11 | Pin `bubbletea v1.3.10`, `lipgloss v1.1.0`, `bubbles v1.0.0` (all MIT) | Latest stable at spec time (queried 2026-10-02 via `go list -m -versions`); §6.4's chosen stack; licences Apache-2.0-compatible, re-verified by the deps task |
| D12 | Promote `mattn/go-isatty` (already indirect) for TTY detection | No new module for one predicate; stdlib has no TTY check |
| D13 | Drive `Update`/`View` directly in tests; no teatest/PTY dep | Pure-function tests cover keys, focus and goldens; interactive PTY coverage comes from the maintainer UX session |
| D14 | "Very short height" is fewer than 10 rows (Q5) | §15.4 leaves it qualitative; 10 rows is unambiguously too short for any pane plus status; test uses ≤5 |
| D15 | Implement the applicable motion subset; register the rest (Q2 option a) | Stubbing handoff/conflict moments with no trigger would fake behaviour §15.6's truth rules forbid |
| D16 | Advertise exactly the tested screen-reader combinations (Q3 option a) | I14: untested combinations are experimental/unsupported, never claimed |
| D17 | History search replays the journal, not a memory copy | AC-6.2's "full stream in searchable history" stays complete with no bound to defend |
| D18 | Live journaled-event callback drops (never blocks) when the TUI lags; unjournaled notices use a drained backlog | A paused terminal must not stall native consumption (§15.9); the next poll tick replays dropped events from the journal and drains retained notices |

No decision changes billing, persistence, process ownership or a public contract, so no ADR is required.

## 17. Honesty register

| Spec demand | Position |
|---|---|
| §15.6 moment 5 (handoff) | Deferred: no multi-agent triggers in a single-agent product; no stub animation |
| §15.6 moment 7 (reservation conflict) | Deferred: one writer, no shared-resource contention to depict |
| §15.6 moment 8 "combined" stage | Partially met: frozen → checked → ready shown; "combined" has no single-writer trigger |
| §15.6 moment 6 countdowns | None shown: Stage 1 has no known resets/deadlines, per the moment's own truth rule |
| §14.6 layout/keymap declarative mods | Deferred to MH-6 (N3); compiled-in defaults now, themes only as data (D6) |
| §15.8 claims beyond tested combinations | Experimental/unsupported until recorded (I14, N5) |
| §16.4 full terminal matrix | Experimental except recorded combinations (I14) |
| §17.4 TUI performance targets | Not measured in this slice; transition budgets (80–200 ms), coalesced rendering and no idle loop bound the shape, but no benchmark claims are made |
| §15.5 mouse | Not offered (D9); AC-3.2 holds vacuously |
| §15.7 OS reduced-motion auto-query | Not queried; explicit flag is the portable control (D5) |
| §15.1 multi-mission dashboard | Out of scope (N2); focused single-mission view only |
| §15.11 Herdr presentation | Out of scope (N1); MH-3 owns the bridge |
| §14 extension points | Out of scope (N3); built-in tokens only |

## 18. Why one spec (14 tasks)

Every task converges on one program that is unusable until the last wiring lands: caps, theme, view-model and diff are layers with no standalone artifact, and navigation, motion, actions and live updates all edit the same model. Splitting would produce specs whose acceptance cannot run (a view-model with no view, a theme with no renderer) and cross-spec drift on the shared `Model`. The tasks are sequential layers, not independent work streams, so this stays a single spec with shared-file ordering notes instead of an epic.
