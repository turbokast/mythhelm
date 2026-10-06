# strict-lint-set — Hand-off

> One section per task. Each task replaces its own `<!-- pending -->` line in its pull request with what
> dependent tasks need: what it produced (the API and files as shipped), what a later task must
> know, and any deviation that changes a later task's inputs. `/run-spec` puts the sections of a
> task's dependencies into that task's dispatch prompt. Open questions and research stay in
> `scratchpad.md`.

## Task 1 — Adapters fallout (revive, modernize)

- **Produces**: `./adapters/...` clean under all five fallout trial linters (revive, modernize, gocritic, perfsprint, dupl — each exits 0 with the uncapped trial command). Shipped: doc comments on `AdapterID`/`New`, `caps`/`canonical` renames, drain-loop bind in `waitStarted`, `SplitSeq`/`new(n)`/`for range N`/`strings.Cut`/`errors.AsType`/`typ.Fields()` simplifications across the 7 files.
- **For dependents**: Task 8's full-tree run covers these paths with zero findings expected from them. No `//nolint` was added, so no machine-checked-form debt. New comments carry trailing periods (godot stays clean).
- **Deviations that change a later task's inputs**: none — no API, behavior, or file-list change; Task 8 needs nothing from this task beyond its merged green state.

## Task 2 — Core fallout A (adapter, admission, integration)

- **Produces**: `./internal/adapter/...`, `./internal/admission/...` and `./internal/integration/...` clean under all five fallout trial linters (revive, gocritic, perfsprint, modernize, dupl — each exits 0 with the uncapped trial command). Shipped: doc comments on `StopInterrupt`/`Tri` blocks, `NativeConfigTrustKind`, the three billing aliases, the `ProjectConfigFile` block, `ProjectConfigTrustKind`, `CheckResult`/`Verification`/`RunChecksWithOptions`; `canonical` rename in `RepoIdentity`; start-error `switch` in `runCheck`; `errors.New` in `parseChanged`; `strings.Cut`/`errors.AsType` in `billing.go` and `SplitSeq` in `native.go`.
- **For dependents**: Task 8's full-tree run covers these paths with zero findings expected from them. No `//nolint` was added, so no machine-checked-form debt. New comments carry trailing periods (godot stays clean).
- **Deviations that change a later task's inputs**: none — no API, behavior, or file-list change; Task 8 needs nothing from this task beyond its merged green state.

## Task 3 — Core fallout B (cli)

- **Produces**: `./internal/cli/...` clean under all five fallout trial linters (revive, perfsprint, modernize, gocritic, dupl — each exits 0 with the uncapped trial command). Shipped: `Package cli` rewords in `accessible.go`/`tui.go`, `ExitOK` const-block comment, `errors.New` in `accessible.go`/`review.go`, `AsType` in `exit.go`/`tui_test.go`, `range N` in `accessible_test.go`, `SplitSeq` in `runs_test.go`/`tui_test.go`, `t.Context` in `TestWaitForRunJournaledSeesJournaledRun`.
- **For dependents**: Task 8's full-tree run covers these paths with zero findings expected from them. No `//nolint` was added, so no machine-checked-form debt. New comments carry trailing periods (godot stays clean). The package now carries three `Package cli` file comments (dispatch, accessible, tui) — legal and revive-clean.
- **Deviations that change a later task's inputs**: none — no API, behavior, or file-list change; Task 8 needs nothing from this task beyond its merged green state.

## Task 4 — Core fallout C (supervisor, workers)

- **Produces**: `./internal/supervisor/...` and `./internal/workers/...` clean under all five fallout trial linters (revive, gocritic, dupl, modernize, perfsprint — each exits 0 with the uncapped trial command) and under copyloopvar, usetesting and godot. Shipped: doc comments on the `RunState`/`AttemptState` const blocks, `_ = ob` drain-loop binds in `faults_test.go` and the worker abort path, own-slice `appendAssign` in the pipeline fixture, a shared `recordApplyEvent` helper behind `recordApplyIntent`/`recordApplyCompleted`, plus `SplitSeq`, two range-over-int, a promoted-field `RecoveryOutcome` literal and `new(pid)` in unix `nativeGroup`.
- **For dependents**: Task 8's full-tree run covers these paths with zero findings expected from them. No `//nolint` was added, so no machine-checked-form debt. New comments carry trailing periods (godot stays clean). The `newexpr` fix touched only the unix `nativeGroup` definition — that cleared the `worker.go:968` call-site diagnostic too, so the call site still dispatches per OS and Windows still gets a nil pgid.
- **Deviations that change a later task's inputs**: none — no API, behavior, or file-list change; Task 8 needs nothing from this task beyond its merged green state.

## Task 5 — Core fallout D (workspace, journal)

<!-- pending -->

## Task 6 — TUI fallout A (mission, model, views)

<!-- pending -->

## Task 7 — TUI fallout B (nav, tools, tests)

<!-- pending -->

## Task 8 — Enable the strict set + enforce the time budget

<!-- pending -->
