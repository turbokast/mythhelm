## Strict Lint Set — Design

> Implements `specs/*/strict-lint-set/requirements.md` (MH-20). Normative source: mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md, version 2.0; § numbers, I-IDs, G-IDs and AT-IDs refer to it. W-IDs refer to the waves in mythhelm-synthesis/MYTHHELM_Implementation_Plan.md.

### 1. Current state

- `warnings_strict` (SUGGESTED) is Unmet: the set is golangci-lint standard plus bodyclose, errorlint, gosec, misspell (UK) and nolintlint (`.golangci.yml`), with one test-scoped G204 exclusion. Gap record: `specs/*/openssf-badge/design.md:31`, recorded at `badge_level == passing` (`:39`).
- CI enforces via `golangci/golangci-lint-action` v9.3.0 with binary v2.13.2, single runner `ubuntu-latest`, no `timeout-minutes` (`.github/workflows/ci.yml:69-83`; `grep -n "timeout"` over the file returns nothing).
- Baseline measured 2026-10-06: `golangci-lint run ./...` at v2.13.2 (local install, same version as CI) reports 0 issues in ~3.6s elapsed.
- No doc-comment convention exists (no comment rule in `.claude/rules/go-conventions.md`); white-box test access is load-bearing (e.g. `internal/tui/nav_test.go` reads `m.focusPane`); error style is contextual inline errors governed by errorlint (`%w` wrapping).

### 2. Trial matrix (FR-1, W16, G10)

Every candidate trial-run 2026-10-06 at exactly v2.13.2 (Q2-a; trial CI runs stay the referee per Q2-b) with the uncapped trial command (AC-1.1) — golangci-lint's default output caps (`--max-issues-per-linter 50`, `--max-same-issues 3`) undercount and nondeterministically drop files, so every trial disables them:

```sh
golangci-lint run --no-config --enable-only <L> --max-issues-per-linter 0 --max-same-issues 0 ./...
```

Verdicts stay maintainer-checkpoint decisions per AC-1.2; the table below is /spec's recommendation:

| Linter | Findings | Files | Time | Verdict | Reason |
|---|---|---|---|---|---|
| revive | 48 | 28 | 0.5s | enable | True: `exported` + `package-comments` (docs) plus `redefines-builtin-id`, `unused-parameter`, `empty-block` (substantive). Cost acceptable, all behavior-preserving. |
| gocritic | 3 | 3 | 1.3s | enable | True, including one real `appendAssign` bug pattern (`cmd.Env = append(env, …)`); two `ifElseChain` style fixes. Trivial. |
| perfsprint | 10 | 8 | 0.6s | enable | True mechanical (`Errorf`→`errors.New`, `Sprint`→`strconv`); auto-fixable. |
| dupl | 2 | 2 | 0.4s | enable | True test-helper duplication (17-line helper across two supervisor test files); dedupe cheap. |
| modernize | 32 | 21 | 0.7s | enable | True mechanical simplifications (`strings.Cut`, `errors.As`→`AsType`, range-over-int, `t.Context`); auto-fixable. |
| copyloopvar | 0 | 0 | 0.4s | enable | Clean; pure prevention at zero fallout cost. |
| usetesting | 0 | 0 | 0.7s | enable | Clean; pure prevention at zero fallout cost. |
| godot | 0 | 0 | 0.5s | enable | Clean; pure prevention (new comments written with periods). |
| testpackage | 39 | 39 | 0.4s | reject | Converting 39 files to `_test` packages guts load-bearing white-box access (tui model fields, cli internals); the cost disqualifies it. |
| err113 | 101 | 43 | 0.7s | reject | Hoisting 101 contextual call-site errors (including test sentinels) to package statics fights the errorlint-governed style and hurts locality; the cost disqualifies it. |

Time is wall-clock per trial on the spec machine 2026-10-06 (single runs, warm cache); it shows no candidate is slow — the enabled eight sum to ~5s locally, so the checkpoint's NFR-1 budget binds on CI overhead, not linter cost. CI stays the referee per Q2-b.

Combined run with all eight winners (`-c` trial config = current file + 8 names, same uncapped flags): exactly 95 issues (2+3+32+10+48; the three clean linters contribute 0 — per-linter sums match, so no cross-linter interaction), 47 files, ~6.2s elapsed cold (~1.2s warm cache) vs ~3.6s baseline. Full output kept out-of-tree (`/tmp/trial-full.txt`, session scratch); the matrix above is the spec record (AC-1.1).

### 3. Enablement (FR-2, W16, G10)

Task 8 adds the eight names to `.golangci.yml`'s `enable` list with no new settings (trials ran defaults; misspell UK stays; the G204 test-scoped rule stays). Target: `golangci-lint run ./...` at the CI-pinned version reports zero findings. Exclusion policy (AC-2.2): trials show every finding fixable at source, so zero new exclusions are expected; any exclusion the implementer needs is path-scoped with a dated rationale comment, and a whole-tree exclusion is a stop-and-report, never silent. `//nolint` policy (AC-3.1): none expected; any added must satisfy the already-enabled nolintlint (machine-checked form with reason — self-enforcing).

### 4. Fallout (FR-3, W16)

95 findings in 47 files, split per Q4-a into one config task plus per-domain fallout tasks (Tasks 1–7 below carry the exact file lists from the measured union: adapters 7, core 7+7+7+5, tui 8+6). Every task keeps whole packages except the tui pair, which splits `internal/tui` by file (its acceptance scopes by path instead of package exit code):

- adapters (7 files): `adapters/claudecode/{decode_test,launch,launch_test,probe,probe_test,settings}.go`, `adapters/fake/fake_test.go`.
- core-A (7 files): `internal/adapter/adapter.go`, `internal/admission/{billing,native,projectconfig,trust}.go`, `internal/integration/{candidate,verify}.go`.
- core-B (7 files): `internal/cli/{accessible,accessible_test,exit,review,runs_test,tui,tui_test}.go`.
- core-C (7 files): `internal/supervisor/{apply_test,faults_test,pipeline_test,recover,state}.go`, `internal/workers/{proc_unix,worker}.go`.
- core-D (5 files): `internal/workspace/{apply,apply_test,snapshot}.go`, `internal/journal/{declarations,projections}.go`.
- tui-A (8 files): `internal/tui/actions_test.go`, `internal/tui/caps/caps.go`, `internal/tui/dialogs.go`, `internal/tui/diff.go`, `internal/tui/live.go`, `internal/tui/mission.go`, `internal/tui/model.go`, `internal/tui/motion.go`.
- tui-B (6 files): `internal/tui/{history,motion_test,nav,nav_test,tools}.go`, `internal/tui/viewmodel/viewmodel_test.go`.

Every fix is behavior-preserving (comments, local renames, `_` parameters, call-site simplifications, test-helper dedupe); any fix that cannot stay so becomes a follow-up issue instead of a deviation (AC-3.2).

Ordering (D4): fallout tasks land FIRST (each verified with the trial commands, since the config does not name the new linters yet), the config task LAST. Enabling first would redden `main` until the fallout merges; the main-green invariant wins over the usual contract-first order, stated here explicitly.

### 5. CI time (NFR-1, W16)

Measured locally: baseline ~3.6s, full new set ~6.2s cold cache (~1.2s warm). The checkpoint sets the minute budget from these measurements (no budget pre-exists); the checkpoint records the agreed number, and Task 8 enforces it in-tree via `timeout-minutes` on the lint job with the named check comparing measured time against it. NFR-2 (I13 analog): every enabled linter runs offline with free tooling — the trials above ran locally with the free v2.13.2 binary; CI remains the referee for the verdict.

### 6. Prepared assessment justification (FR-4, §19.3 Revision 1.1, G10)

The spec record carries the paste-ready `warnings_strict` → Met text (Q3-a; the implementing task also pastes it into its PR description). The operator pastes it into the assessment after the CI lint job goes green with the new set (N1); until then the live badge stays passing and this spec touches no assessment (AC-4.2):

```text
Met — the repository lints clean with zero findings under golangci-lint
v2.13.2 (standard set plus bodyclose, errorlint, gosec, misspell,
nolintlint, revive, gocritic, perfsprint, dupl, modernize, copyloopvar,
usetesting, godot), enforced by the CI golangci-lint job (green run
<CI-RUN-URL>). Trial matrix with per-linter counts and reject reasons:
specs/done/strict-lint-set/design.md §2. Fill the run URL at flip time;
the flip happens only after that green run.
```

### 7. Decisions

| ID | Decision | Rationale |
|---|---|---|
| D1 | Curated shortlist trial-run one by one (Q1-b) | Bounded trial and fallout volume with a named verdict per linter. Rejected: (a) enable-all-then-trim (unbounded sweep; the catalogue is ~100 linters). |
| D2 | Local trials at exactly v2.13.2, CI as referee (Q2-a) | The installed binary matches CI's pinned version byte-for-byte in behavior; ten trials ran in seconds. Rejected: trial-CI-only (a branch + CI cycle per candidate). |
| D3 | Enable 8, reject testpackage + err113 with recorded costs | §2 matrix: winners show true findings at acceptable cost (95 fixable = 48+3+10+2+32); testpackage guts white-box tests (39 files); err113 fights the errorlint-governed style (101 sites in 43 files). Checkpoint confirms each verdict per AC-1.2. |
| D4 | Fallout tasks first, config task last | Enabling first reddens `main` until fallout lands. Fallout tasks verify via the trial commands; the config task flips `.golangci.yml` onto a clean tree. Main-green wins over contract-first here. |
| D5 | Config + per-domain fallout split, 8 tasks (Q4-a) | 47 files across 4 domains cannot fit fewer tasks without breaking the 8-file / one-domain rules. Split follows the measured union exactly. |
| D6 | `timeout-minutes` enforcement lands in the config task | The checkpoint sets the number from §5 measurements; enforcement is release-domain work beside the enablement. |
| D7 | Justification text in this design §6 + task PR description (Q3-a) | The assessment is external, so no tree file holds its live state; `done/` keeps the prepared text permanently. Rejected: a docs note beside the badge (implies live state the tree cannot hold). |

### 8. Honesty register

| Spec demand | Position |
|---|---|
| `warnings_strict` → Met | Prepared, not flipped: justification text in §6, operator action post-green-CI (N1). |
| Strictest set | Bounded-practical, not maximal: testpackage and err113 evaluated and rejected with costs in §2. Re-enabling either needs its own cost re-argument, not silence. |
| NFR-1 minute budget | Unset until the checkpoint: §5 measurements are the input, the checkpoint records the number, Task 8 enforces it. |
| Silver-tier criteria | Out of scope (N3): passing-tier answers must not regress; nothing here claims beyond-passing. |
| Fallout behavior preservation | Every fix preserves behavior or becomes a follow-up issue (AC-3.2); the implementer proves it with the packages' tests, Task 8 with the full suite. |

### 9. Cross-spec references

- Builds on: `specs/*/openssf-badge/` (MH-9, shipped) — the `warnings_strict` Unmet record (design §3), the card-per-Unmet mechanism (FR-3), and the live-assessment verification precedent.
- Conflicts: none. No other active spec touches Go files or lint config: `specs/todo/tui-tape-recording/` touches `docs/demos/`, embeds, `docs.yml` and `docs/automation.md`; `specs/todo/version-numbering/` and `specs/todo/release-tagging/` touch `docs/decisions/` + `docs/release-process.md`; `specs/refined/` holds only this spec; `specs/unrefined/`, `specs/in-progress/` and `specs/unfinalized/` are empty. Shared file with `specs/todo/tui-tape-recording/` Task 3: `docs/automation.md` — row-disjoint (golangci-lint row here, Docs-site/VHS rows there); the later run rebases onto the earlier. Fallout fixes coordinate by package: each file belongs to exactly one task below.
