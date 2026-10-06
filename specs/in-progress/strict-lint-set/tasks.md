## Strict Lint Set — Tasks

### Dependencies

- Prerequisite specs: `specs/*/openssf-badge/` (shipped; the `warnings_strict` Unmet record this spec prepares to close). No other active spec touches Go files or lint config (design §9).
- Order: fallout Tasks 1–7 first, in parallel (disjoint Files); Task 8 (config flip) after all seven. Fallout-first keeps `main` green at every merge — enabling first would redden it until the fallout lands (design D4).
- Trial command (design §2): `golangci-lint run --no-config --enable-only <L> --max-issues-per-linter 0 --max-same-issues 0 <paths>` at v2.13.2. The uncapped flags are mandatory — the default caps undercount and nondeterministically drop files; any acceptance run without them is void.
- **Gates for every task.** Go tasks (1–7): `gofmt -l .` (must print nothing) · `go vet ./...` · `go test -race` on the task's packages (Task 8 runs the full suite) · `go mod tidy -diff` (must print nothing — stdlib-only fixes, no new dependencies) · the trial commands in each task's acceptance. Task 4's Files include `*_unix.go`: also `GOOS=windows go vet ./...` and `GOOS=darwin go vet ./...`. Release task (8): `go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12` locally, `zizmor` in CI, plus `scripts/ci/check-public-hygiene.sh`. Docs/release hygiene: every task runs `scripts/ci/check-public-hygiene.sh`. Files existing or an agent reporting success is not completion; cite the runner output.
- **Completion convention.** Append ` ✅ COMPLETED` to the heading, keep every original field, and add `Implementation` (at most 3 lines, plus the commit SHA), `Spec deviations` ("None" or a justification) and `Files modified`.
- **Commits.** Every commit is signed off (`git commit -s`, DCO). No decision record needed: no billing, persistence, process-ownership or public-contract change (lint fallout + config enablement only).

---

## Implementation Tasks

### Task 1 — Adapters fallout (revive, modernize) ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: standard (mechanical comment/rename/simplify fallout across 7 files; no new abstraction; 5 acceptance items)
- **Change**: Fix the adapters fallout at source — doc comments, builtin-shadow renames, `strings.Cut`/`AsType` simplifications — behavior-preserving.
- **Files**:
  - `adapters/claudecode/decode_test.go`
  - `adapters/claudecode/launch.go`
  - `adapters/claudecode/launch_test.go`
  - `adapters/claudecode/probe.go`
  - `adapters/claudecode/probe_test.go`
  - `adapters/claudecode/settings.go`
  - `adapters/fake/fake_test.go`
- **Acceptance**:
  - Red→green per linter over `./adapters/...` with the trial command: before fixes revive and modernize exit 1 (record the outputs in the entry); after fixes each exits 0. (gocritic/perfsprint/dupl report nothing under `./adapters/...` — their trial runs exit 0 there today and must still exit 0.)
  - `go test -race ./adapters/...` passes and `go build ./...` passes (no behavior change beyond what the findings require; anything larger becomes a follow-up issue instead of a deviation).
  - The current-config full run stays green: `golangci-lint run ./...` (existing set incl. nolintlint) exits 0 — any new `//nolint` therefore already carries the machine-checked form nolintlint enforces, or the run fails.
  - `.golangci.yml` untouched: `git status --porcelain .golangci.yml` prints nothing (enablement is Task 8's).
  - `scripts/ci/check-public-hygiene.sh` passes (a hygiene violation in a touched file fails the script).
- **Test plan**: trial-command red→green per linter; package tests + full build; existing-set regression run.
- **Invariants touched**: None (behavior-preserving lint fallout in adapter code; package tests + build prove no behavior change).
- **Status**: ✅ Completed — adapters tree lints clean under revive and modernize with behavior preserved; PR #170.
- **Implementation**: 13 findings fixed at source across the 7 files (2 doc comments, 2 builtin-shadow renames, 1 empty-block bind, 8 modernize simplifications); no `//nolint` added. All five trial commands exit 0; `go test -race ./adapters/...` and `go build ./...` pass. Commit 3863cad.
- **Spec deviations**: None.
- **Files modified**: `adapters/claudecode/decode_test.go`, `adapters/claudecode/launch.go`, `adapters/claudecode/launch_test.go`, `adapters/claudecode/probe.go`, `adapters/claudecode/probe_test.go`, `adapters/claudecode/settings.go`, `adapters/fake/fake_test.go`, `specs/in-progress/strict-lint-set/tasks.md`, `specs/in-progress/strict-lint-set/handoff.md`.
- **Red baseline** (pre-fix trial output over `./adapters/...`, golangci-lint v2.13.2, uncapped flags): revive exit 1, 5 issues — `probe.go:28 exported const AdapterID`, `probe.go:41 exported func New`, `probe_test.go:183 redefines-builtin-id cap`, `settings.go:148 redefines-builtin-id real`, `fake_test.go:432 empty-block`; modernize exit 1, 8 issues — `decode_test.go:23 stringsseq`, `decode_test.go:44 newexpr i64`, `decode_test.go:291,298 rangeint`, `launch.go:132 stringscut`, `launch_test.go:93 errorsastype`, `launch_test.go:257 stringsseq`, `probe_test.go:290 stditerators`; gocritic/perfsprint/dupl exit 0. Post-fix: all five exit 0.

### Task 2 — Core fallout A (adapter, admission, integration) ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: standard (mechanical comment/rename/simplify fallout across 7 files; no new abstraction; 5 acceptance items)
- **Depends on**: None (parallel with Tasks 1, 3–7 — disjoint Files)
- **Change**: Fix the core-A fallout at source — doc comments, builtin-shadow renames, unused parameters, `ifElseChain`, `Errorf`→`errors.New`, `strings.Cut`/`AsType` simplifications — behavior-preserving.
- **Files**:
  - `internal/adapter/adapter.go`
  - `internal/admission/billing.go`
  - `internal/admission/native.go`
  - `internal/admission/projectconfig.go`
  - `internal/admission/trust.go`
  - `internal/integration/candidate.go`
  - `internal/integration/verify.go`
- **Acceptance**:
  - Red→green per linter over the task's packages (`./internal/adapter/... ./internal/admission/... ./internal/integration/...`) with the trial command: before fixes revive, gocritic, perfsprint and modernize exit 1 (record the outputs); after fixes each exits 0. (dupl reports nothing on these packages — still exits 0.)
  - `go test -race` on the task's three package trees passes and `go build ./...` passes (no behavior change beyond what the findings require; anything larger becomes a follow-up issue instead of a deviation).
  - The current-config full run stays green: `golangci-lint run ./...` exits 0 (new `//nolint`, if any, already machine-checked or this fails).
  - `.golangci.yml` untouched: `git status --porcelain .golangci.yml` prints nothing.
  - `scripts/ci/check-public-hygiene.sh` passes (a hygiene violation in a touched file fails the script).
- **Test plan**: trial-command red→green per linter; package tests + full build; existing-set regression run.
- **Invariants touched**: None (behavior-preserving lint fallout; package tests + build prove no behavior change).
- **Status**: ✅ Completed — core-A tree lints clean under revive, gocritic, perfsprint and modernize with behavior preserved; PR #171.
- **Implementation**: 17 findings fixed at source across the 7 files (11 doc comments incl. 3 const-block comments, 1 builtin-shadow rename, 1 if-else→switch, 1 Errorf→errors.New, 3 modernize simplifications); no `//nolint` added. All five trial commands exit 0; `go test -race` on the three trees and `go build ./...` pass. Commit 7271b80.
- **Spec deviations**: None.
- **Files modified**: `internal/adapter/adapter.go`, `internal/admission/billing.go`, `internal/admission/native.go`, `internal/admission/projectconfig.go`, `internal/admission/trust.go`, `internal/integration/candidate.go`, `internal/integration/verify.go`, `specs/in-progress/strict-lint-set/tasks.md`, `specs/in-progress/strict-lint-set/handoff.md`.
- **Red baseline** (pre-fix trial output over `./internal/adapter/... ./internal/admission/... ./internal/integration/...`, golangci-lint v2.13.2, uncapped flags): revive exit 1, 12 issues — `adapter.go:107 StopInterrupt block`, `adapter.go:333 Supported block`, `billing.go:16 NativeConfigTrustKind`, `billing.go:18 AuthEvidence`, `billing.go:19 Declaration`, `billing.go:20 BillingPosture`, `projectconfig.go:23 ProjectConfigFile block`, `trust.go:14 ProjectConfigTrustKind`, `trust.go:18 redefines-builtin-id real`, `verify.go:24 CheckResult`, `verify.go:34 Verification`, `verify.go:51 RunChecksWithOptions`; gocritic exit 1, 1 issue — `verify.go:123 ifElseChain`; perfsprint exit 1, 1 issue — `candidate.go:137 Errorf`; modernize exit 1, 3 issues — `billing.go:62 stringscut`, `billing.go:133 errorsastype`, `native.go:190 stringsseq`; dupl exit 0. Post-fix: all five exit 0.

### Task 3 — Core fallout B (cli)

- **Domain/agent**: go-implementer
- **Budget**: standard (mechanical comment/rename/simplify fallout across 7 files; no new abstraction; 5 acceptance items)
- **Depends on**: None (parallel with Tasks 1–2, 4–7 — disjoint Files)
- **Change**: Fix the cli fallout at source — doc comments, `Errorf`→`errors.New`, range/`strings.Cut`/`AsType` simplifications — behavior-preserving.
- **Files**:
  - `internal/cli/accessible.go`
  - `internal/cli/accessible_test.go`
  - `internal/cli/exit.go`
  - `internal/cli/review.go`
  - `internal/cli/runs_test.go`
  - `internal/cli/tui.go`
  - `internal/cli/tui_test.go`
- **Acceptance**:
  - Red→green per linter over `./internal/cli/...` with the trial command: before fixes revive, perfsprint and modernize exit 1 (record the outputs); after fixes each exits 0. (gocritic/dupl report nothing on this tree — still exit 0.)
  - `go test -race ./internal/cli/...` passes and `go build ./...` passes (no behavior change beyond what the findings require; anything larger becomes a follow-up issue instead of a deviation).
  - The current-config full run stays green: `golangci-lint run ./...` exits 0 (new `//nolint`, if any, already machine-checked or this fails).
  - `.golangci.yml` untouched: `git status --porcelain .golangci.yml` prints nothing.
  - `scripts/ci/check-public-hygiene.sh` passes (a hygiene violation in a touched file fails the script).
- **Test plan**: trial-command red→green per linter; package tests + full build; existing-set regression run.
- **Invariants touched**: None (behavior-preserving lint fallout; package tests + build prove no behavior change).

### Task 4 — Core fallout C (supervisor, workers)

- **Domain/agent**: go-implementer
- **Budget**: standard (mechanical comment/rename/simplify fallout across 7 files; no new abstraction; 5 acceptance items)
- **Depends on**: None (parallel with Tasks 1–3, 5–7 — disjoint Files)
- **Change**: Fix the core-C fallout at source — the `appendAssign` bug pattern, test-helper dedupe, doc comments, `rangeint`/struct-literal simplifications — behavior-preserving.
- **Files**:
  - `internal/supervisor/apply_test.go`
  - `internal/supervisor/faults_test.go`
  - `internal/supervisor/pipeline_test.go`
  - `internal/supervisor/recover.go`
  - `internal/supervisor/state.go`
  - `internal/workers/proc_unix.go`
  - `internal/workers/worker.go`
- **Acceptance**:
  - Red→green per linter over `./internal/supervisor/... ./internal/workers/...` with the trial command: before fixes revive, gocritic, dupl and modernize exit 1 (record the outputs); after fixes each exits 0. (perfsprint reports nothing on these packages — still exits 0.) The `appendAssign` fix assigns the append to its own slice (reverting that hunk re-trips gocritic).
  - `go test -race` on the task's two package trees passes and `go build ./...` passes (no behavior change beyond what the findings require; anything larger becomes a follow-up issue instead of a deviation).
  - The current-config full run stays green: `golangci-lint run ./...` exits 0 (new `//nolint`, if any, already machine-checked or this fails).
  - `.golangci.yml` untouched: `git status --porcelain .golangci.yml` prints nothing.
  - `scripts/ci/check-public-hygiene.sh` passes (a hygiene violation in a touched file fails the script).
- **Test plan**: trial-command red→green per linter; package tests + full build; existing-set regression run. Files include `*_unix.go`: also `GOOS=windows go vet ./...` and `GOOS=darwin go vet ./...` per the gates header.
- **Invariants touched**: None (behavior-preserving lint fallout; package tests + build prove no behavior change).

### Task 5 — Core fallout D (workspace, journal)

- **Domain/agent**: go-implementer
- **Budget**: standard (mechanical comment/rename/simplify fallout across 5 files; no new abstraction; 5 acceptance items)
- **Depends on**: None (parallel with Tasks 1–4, 6–7 — disjoint Files)
- **Change**: Fix the workspace/journal fallout at source — doc comments, `Errorf`→`errors.New`, `Sprint`→`strconv`, `AsType` simplifications — behavior-preserving.
- **Files**:
  - `internal/workspace/apply.go`
  - `internal/workspace/apply_test.go`
  - `internal/workspace/snapshot.go`
  - `internal/journal/declarations.go`
  - `internal/journal/projections.go`
- **Acceptance**:
  - Red→green per linter over `./internal/workspace/... ./internal/journal/...` with the trial command: before fixes revive, perfsprint and modernize exit 1 (record the outputs); after fixes each exits 0. (gocritic/dupl report nothing on these packages — still exit 0.)
  - `go test -race` on the task's two package trees passes and `go build ./...` passes (no behavior change beyond what the findings require; anything larger becomes a follow-up issue instead of a deviation).
  - The current-config full run stays green: `golangci-lint run ./...` exits 0 (new `//nolint`, if any, already machine-checked or this fails).
  - `.golangci.yml` untouched: `git status --porcelain .golangci.yml` prints nothing.
  - `scripts/ci/check-public-hygiene.sh` passes (a hygiene violation in a touched file fails the script).
- **Test plan**: trial-command red→green per linter; package tests + full build; existing-set regression run.
- **Invariants touched**: None (behavior-preserving lint fallout; package tests + build prove no behavior change).

### Task 6 — TUI fallout A (mission, model, views)

- **Domain/agent**: tui-implementer
- **Budget**: standard (mechanical comment/rename/simplify fallout across 8 files; no new abstraction; 5 acceptance items)
- **Depends on**: None (parallel with Tasks 1–5, 7 — disjoint Files)
- **Change**: Fix the tui-A fallout at source — doc comments, `ifElseChain`, `Sprint`→`strconv`, range simplifications — behavior-preserving.
- **Files**:
  - `internal/tui/actions_test.go`
  - `internal/tui/caps/caps.go`
  - `internal/tui/dialogs.go`
  - `internal/tui/diff.go`
  - `internal/tui/live.go`
  - `internal/tui/mission.go`
  - `internal/tui/model.go`
  - `internal/tui/motion.go`
- **Acceptance**:
  - Red→green per linter over `./internal/tui/...` with the trial command (uncapped flags mandatory — caps can falsely green a grep check): before fixes the revive, gocritic, perfsprint and modernize runs show hits on this task's 8 paths (record the outputs); after fixes grepping the full-tree output for exactly these 8 paths prints nothing (the Task 7 files still trip the package run, so package exit-0 belongs to Task 8, not here).
  - `go test -race ./internal/tui/...` passes and `go build ./...` passes (no behavior change beyond what the findings require, including the `viewmodel` subpackage Task 7 owns; anything larger becomes a follow-up issue instead of a deviation).
  - The current-config full run stays green: `golangci-lint run ./...` exits 0 (new `//nolint`, if any, already machine-checked or this fails).
  - `.golangci.yml` untouched: `git status --porcelain .golangci.yml` prints nothing.
  - `scripts/ci/check-public-hygiene.sh` passes (a hygiene violation in a touched file fails the script).
- **Test plan**: trial-command red→green per linter (file-scoped grep with pre-fix hits as the non-empty leg); package tests + full build; existing-set regression run.
- **Invariants touched**: None (behavior-preserving lint fallout; package tests + build prove no behavior change).

### Task 7 — TUI fallout B (nav, tools, tests)

- **Domain/agent**: tui-implementer
- **Budget**: standard (mechanical comment/rename/simplify fallout across 6 files; no new abstraction; 5 acceptance items)
- **Depends on**: None (parallel with Tasks 1–6 — disjoint Files)
- **Change**: Fix the tui-B fallout at source — package comment, doc comments and the remaining simplifications — behavior-preserving.
- **Files**:
  - `internal/tui/history.go`
  - `internal/tui/motion_test.go`
  - `internal/tui/nav.go`
  - `internal/tui/nav_test.go`
  - `internal/tui/tools.go`
  - `internal/tui/viewmodel/viewmodel_test.go`
- **Acceptance**:
  - Red→green per linter like Task 6 with the trial command (uncapped flags mandatory), scoped to this task's 6 paths: pre-fix trial output shows hits on them, post-fix grep of the full-tree output for exactly these paths prints nothing (package exit-0 belongs to Task 8).
  - `go test -race ./internal/tui/...` passes and `go build ./...` passes (no behavior change beyond what the findings require; anything larger becomes a follow-up issue instead of a deviation).
  - The current-config full run stays green: `golangci-lint run ./...` exits 0 (new `//nolint`, if any, already machine-checked or this fails).
  - `.golangci.yml` untouched: `git status --porcelain .golangci.yml` prints nothing.
  - `scripts/ci/check-public-hygiene.sh` passes (a hygiene violation in a touched file fails the script).
- **Test plan**: trial-command red→green per linter (file-scoped grep with pre-fix hits as the non-empty leg); package tests + full build; existing-set regression run.
- **Invariants touched**: None (behavior-preserving lint fallout; package tests + build prove no behavior change).

### Task 8 — Enable the strict set + enforce the time budget

- **Domain/agent**: release-engineer
- **Budget**: standard
- **Depends on**: Task 1, Task 2, Task 3, Task 4, Task 5, Task 6, Task 7
- **Change**: Enable the eight trial-winning linters in `.golangci.yml` and enforce the checkpoint-agreed CI time budget on the lint job.
- **Files**:
  - `.golangci.yml` (add the eight names; no new settings, no new exclusions)
  - `.github/workflows/ci.yml` (`timeout-minutes` on the lint job only)
  - `docs/automation.md` (golangci-lint row names the eight new linters; row-disjoint from tui-tape-recording Task 3's Docs-site/VHS edits to the same file — rebase onto whichever run lands first)
- **Acceptance**:
  - `.golangci.yml`'s linters `enable` list names revive, gocritic, perfsprint, dupl, modernize, copyloopvar, usetesting and godot — each checked inside the linters block only (`awk '/^linters:/{f=1} f && /^[[:space:]]*enable:/{e=1;next} e && /^[[:space:]]*settings:/{exit} e' .golangci.yml | grep -c '    - <name>'` prints 1 per name; a comment mentioning a name, or the formatters block, does not satisfy it). No new `settings` block appears, and added lines outside the `enable:` list are only a path-scoped `exclusions` rule with dated rationale recorded in the entry (the AC-2.2 escape hatch — `git diff -U0 .golangci.yml | grep -E '^\+[^+]'` shows each added line for review; a whole-tree exclusion or any other addition fails).
  - Full-tree verification: `golangci-lint run ./...` at the CI-pinned version reports zero findings (exit 0), timed with `time` and recorded in the entry (NFR-1 local leg; design §5 measured ~6.2s cold for the same set pre-fix), and `go test -race ./...` passes (suite-wide behavior proof after all fallout has merged).
  - The CI lint job is green on the PR with the new set and carries `timeout-minutes` set to the checkpoint-agreed minutes from this spec's approval record (anchored to the `golangci-lint` job block; a non-numeric or missing value fails the check); the entry records the CI job time beside the local time (NFR-1 referee leg).
  - `docs/automation.md`'s golangci-lint row names the eight newly enabled linters (anchored: each name grepped on the `| golangci-lint` row; deleting any fails its check).
  - The task's PR description quotes the design §6 justification text verbatim (reviewer confirms the paste on the PR before merge; a missing or altered quote fails review).
  - `scripts/ci/check-public-hygiene.sh` passes (a hygiene violation in a touched file fails the script).
- **Test plan**: config grep checks; full local lint + full test suite, both timed; green CI lint job on the PR.
- **Invariants touched**: None (config-only enablement on a clean tree; no product behavior changes).
