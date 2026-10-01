## Dogfood Slice — Tasks

### Dependencies

- None outside this spec. The tasks are listed in dependency order. Tasks that share a `Depends on` set and have disjoint `Files` may run in parallel: 2 after 1, then 3 after 2 (both edit `ci.yml` and `docs/automation.md`); {4, 6, 8} after 1; {5, 7} after their prerequisites.
- Each task is sized for one agent session. `Domain/agent`: `go-implementer` for core and adapters, `release-engineer` for CI and repository automation, `maintainer` for the human-run dogfood.
- **Gates for every task.** Run `gofmt -w` on touched files before any check, then `go vet ./...`, `go test -race ./...` and `go mod tidy -diff` (plus `golangci-lint run` once Task 2 lands). A task is not complete because files exist or an agent reported success (§22.2); cite the runner output.
- **Completion convention.** Append ` ✅ COMPLETED` to the heading, keep every original field, and add `Implementation` (at most 3 lines, plus the commit SHA), `Spec deviations` ("None" or a justification) and `Files modified`.
- **Commits.** Every commit is signed off (`git commit -s`, DCO). Any change to billing, persistence or process ownership carries its decision record in the same PR.

---

## Implementation Tasks

### Task 1 — Go module, skeleton and CLI dispatcher ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Change**: Create `go.mod` (`module github.com/turbokast/mythhelm`, `go 1.27.0`, `toolchain go1.27.1`) and a `mythhelm` binary with the D1 dispatcher, `version`, exit-code mapping and the interspersed-flag parser, so that CI's gated Go job runs green on all three OSes.
- **Files**:
  - `go.mod`
  - `cmd/mythhelm/main.go`
  - `internal/cli/dispatch.go`
  - `internal/cli/exit.go`
  - `internal/cli/dispatch_test.go`
  - `internal/buildinfo/buildinfo.go`
  - `CONTRIBUTING.md` (fill the "Development setup" section: Go version, `go test ./...`, `gofmt`)
- **Produces**: `cli.Main(args []string, stdio cli.Stdio) int`; `cli.ExitCode` constants (`ExitOK=0, ExitInvalid=2, ExitBlocked=3, ExitNative=4, ExitVerify=5, ExitOwnership=6, ExitCapability=7, ExitCancelled=130`); `cli.ParseInterspersed(fs *flag.FlagSet, args []string) (positional []string, err error)`.
- **Acceptance**:
  - `TestParseInterspersedFlags`: `apply run_1 --to-branch x` and `apply --to-branch x run_1` both yield positional `[run_1]` and branch `x`.
  - `TestUnknownCommandExits2`.
  - `TestVersionReportsBuildInfo`: prints the version, commit and Go version; `--format jsonl` prints one JSON object.
  - The CI `Go (ubuntu|macos|windows)` jobs run and pass on the PR. Before this task they are skipped because there is no `go.mod`.
  - The module has no `require` lines yet.
- **Test plan**: table tests for the dispatcher; run `go run ./cmd/mythhelm version` in CI through the test.
- **Invariants touched**: G01 (builds without credentials), §15.10.
- **Status**: ✅ Completed. Adds `go.mod` with no requirements, the stdlib-`flag` dispatcher, `ParseInterspersed`, the §15.10 `ExitCode` constants mapped in `exit.go`, `version` in plain and JSONL form, and `buildinfo` (ldflags, then embedded VCS info); PR #7.
- **Implementation**: `cli.Main` dispatches through a `commands` map, and later tasks register their verbs there. `ParseInterspersed` separates flag tokens from positional arguments in one pass (`fs.Lookup` and `IsBoolFlag`), then calls `fs.Parse` once. `--` ends the flags, and a lone `-` is positional. A failed write on the help paths exits 1. Commits 9976886 and 828734f.
- **Spec deviations**: `ExitInternal = 1` is added next to the listed constants, because design §11 requires exit 1 for unexpected errors.
- **Files modified**: `go.mod`, `cmd/mythhelm/main.go`, `internal/cli/dispatch.go`, `internal/cli/exit.go`, `internal/cli/dispatch_test.go`, `internal/buildinfo/buildinfo.go`, `CONTRIBUTING.md`, `specs/todo/dogfood-slice/tasks.md`.

### Task 2 — CI quality gates: golangci-lint, govulncheck, CodeQL Go, Dependabot gomod ✅ COMPLETED

- **Domain/agent**: release-engineer
- **Budget**: standard
- **Depends on**: Task 1
- **Change**: Add the lint and vulnerability jobs and the configuration from design §13 bullets 1–4. Move those items from Deferred to Active in `docs/automation.md`.
- **Files**:
  - `.golangci.yml`
  - `.github/workflows/ci.yml` (new `lint` and `govulncheck` jobs, added to `ci-ok.needs`)
  - `.github/workflows/codeql.yml` (`language: [actions, go]`)
  - `.github/dependabot.yml` (`gomod`, 7-day cooldown, grouped)
  - `docs/automation.md`
- **Acceptance**:
  - `actionlint` and `zizmor` pass.
  - Every `uses:` is pinned to a 40-character SHA with a `# vX.Y.Z` comment.
  - `golangci-lint run` passes locally and in CI on the Task 1 tree.
  - `govulncheck ./...` reports no findings.
  - The PR shows the `Analyze (go)` check.
  - The PR description tells the maintainer to add `Analyze (go)` to the ruleset's required checks.
- **Test plan**: CI run on the PR; paste the job URLs in the completion entry.
- **Invariants touched**: §19.4 (pinned, least-privilege CI), G10.
- **Status**: ✅ Completed — `.golangci.yml` (v2, design §13 linter set, G204 excluded in tests), CI `golangci-lint` and `govulncheck` jobs in `CI OK`, CodeQL `go`, Dependabot `gomod`; PR #8.
- **Implementation**: golangci-lint-action v9.3.0 runs lint v2.13.2; govulncheck runs as `go run golang.org/x/vuln/cmd/govulncheck@v1.8.0`; the CodeQL Go leg runs `setup-go` from `go.mod` before `init`. The maintainer adds `Analyze (go)` to the ruleset. Commit 49d22cb.
- **CI evidence** (merge commit 26c2cbe): [golangci-lint](https://github.com/turbokast/mythhelm/actions/runs/36613272255/job/109560081258) (`0 issues.`), [govulncheck](https://github.com/turbokast/mythhelm/actions/runs/36613272255/job/109560081285) (`No vulnerabilities found.`), [Analyze (go)](https://github.com/turbokast/mythhelm/actions/runs/36613272237/job/109559790588), [Lint workflows](https://github.com/turbokast/mythhelm/actions/runs/36613272255/job/109559791216), [zizmor](https://github.com/turbokast/mythhelm/actions/runs/36613272609/job/109559791963).
- **Spec deviations**: golangci-lint v2.13.2, not v2.14.0, because v2.14.0 (2026-09-24) is inside the 7-day cooldown. govulncheck runs through `go run`, because `golang/govulncheck-action` is not on the repository's action allow-list. The CodeQL Go leg adds `setup-go`, so the extractor uses the toolchain pinned in `go.mod`.
- **Files modified**: `.golangci.yml`, `.github/workflows/ci.yml`, `.github/workflows/codeql.yml`, `.github/dependabot.yml`, `docs/automation.md`, `specs/todo/dogfood-slice/tasks.md`.

### Task 3 — CI supply chain and runner matrix ✅ COMPLETED

- **Domain/agent**: release-engineer
- **Budget**: standard
- **Depends on**: Task 2 (shares `ci.yml` and `docs/automation.md`; never run in parallel with it)
- **Change**: Add OSV-Scanner (SARIF upload), a go-licenses check against the dependency-review allowlist, and the `ubuntu-24.04-arm` and `windows-11-arm` Go runners (design §13 bullets 5–6), and update `docs/automation.md`.
- **Files**:
  - `.github/workflows/osv-scanner.yml`
  - `.github/workflows/ci.yml` (matrix, `licenses` job)
  - `docs/automation.md`
- **Acceptance**:
  - `actionlint` and `zizmor` pass.
  - The `licenses` job fails on a scratch branch that adds a GPL module. Record the run URL, then drop that branch.
  - The arm64 Go jobs pass.
- **Test plan**: CI runs on the PR.
- **Invariants touched**: NFR-3, §19.2, §19.4.
- **Status**: ✅ Completed — OSV-Scanner workflow (SARIF upload), a `go-licenses` job in `CI OK` checked against the dependency-review allow-list, and the `ubuntu-24.04-arm` and `windows-11-arm` Go runners; PR #13.
- **Implementation**: OSV-Scanner v2.6.0 and go-licenses v2.0.1 run through `go run` at pinned versions. go-licenses reads `allow-licenses` from `dependency-review.yml` at run time and checks with `--include_tests`. `windows-11-arm` runs `go test` without `-race`. Commit 4244d71.
- **CI evidence**: [go-licenses failing on the scratch GPL branch](https://github.com/turbokast/mythhelm/actions/runs/36616296064/job/109570149904) (`Not allowed license 'GPL-3.0' found for library 'github.com/evilsocket/islazy/str'`; probe PR #14 closed, branch deleted), [Go (ubuntu-24.04-arm)](https://github.com/turbokast/mythhelm/actions/runs/36616291969/job/109570137238), [Go (windows-11-arm)](https://github.com/turbokast/mythhelm/actions/runs/36616291969/job/109570137336), [go-licenses](https://github.com/turbokast/mythhelm/actions/runs/36616291969/job/109570137123), [OSV-Scanner](https://github.com/turbokast/mythhelm/actions/runs/36616292060/job/109570022136).
- **Spec deviations**: OSV-Scanner and go-licenses run through `go run`, because their actions are not on the action allow-list. `windows-11-arm` has no `-race`, because the race detector does not support windows/arm64. go-licenses notice generation and a macOS x64 runner stay deferred until the first release.
- **Files modified**: `.github/workflows/osv-scanner.yml`, `.github/workflows/ci.yml`, `docs/automation.md`, `specs/todo/dogfood-slice/tasks.md`.

### Task 4 — State directory, IDs and the SQLite journal ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: complex
- **Depends on**: Task 1
- **Change**: Implement `statedir`, `ids` and `journal`, covering open with the pragmas, migration 0001, the append-only journal with dedup and sequence checks, and downgrade refusal (design §5). Write ADR 0003 (local state: driver, schema v1, migration policy).
- **Files**:
  - `internal/statedir/statedir.go`
  - `internal/ids/ids.go`
  - `internal/journal/journal.go`
  - `internal/journal/migrations/0001_init.sql`
  - `internal/journal/journal_test.go`
  - `internal/ids/ids_test.go`
  - `docs/decisions/0003-local-state-sqlite.md`
  - `go.mod`, `go.sum`
- **Produces**:
  - `statedir.Resolve() (string, error)`
  - `statedir.Ensure(dir string) error` (0700)
  - `ids.New(prefix string) string`
  - `journal.Open(ctx, dir string) (*Journal, error)`
  - `(*Journal).Append(ctx, Event, project func(*sql.Tx) error) error`
  - `(*Journal).Events(ctx, runID string, afterRunSeq int64) ([]Event, error)`
  - `journal.ErrSchemaTooNew`
  - `journal.ErrSequenceGap`
- **Acceptance**:
  - `TestJournalRejectsUpdateAndDelete`: the triggers abort.
  - `TestAppendIsIdempotentOnEventID`.
  - `TestAppendRejectsProducerSequenceGap`.
  - `TestAppendRejectsStaleGeneration`.
  - `TestRunSequenceIsPerRunMonotonic`.
  - `TestOpenRefusesNewerSchema`: with `user_version=99`, `Open` returns `ErrSchemaTooNew`, and the file's SHA-256 is unchanged.
  - `TestPragmasApplied`: reads back WAL, `foreign_keys=1`, `synchronous=2`.
  - `TestStateDirMode0700` (Unix).
  - `TestIDsSortByTime`.
  - A build with `CGO_ENABLED=0` passes in CI.
- **Test plan**: temp-dir databases; a second connection for trigger checks.
- **Invariants touched**: §6.4, §7.5, §7.6, I09 (payload fields are typed and labelled).
- **Status**: ✅ Completed. Adds `statedir` (per-OS resolution, `MYTHHELM_HOME`, 0700), `ids` (prefixed ULID-format IDs) and `journal` (modernc.org/sqlite v1.59.0, pragmas, schema v1, an append-only journal with dedup, sequence and generation checks, read-only downgrade refusal and a `VACUUM INTO` backup before upgrades), plus ADR 0003; PR #10.
- **Implementation**: `Append` runs dedup, generation, sequence, `run_sequence` assignment, the projection and the commit in one `BEGIN IMMEDIATE` transaction. `Open` probes `user_version` over a read-only connection with no pragmas before the WAL connection pool touches the file. `TestBuildWithoutCgo` builds the module with `CGO_ENABLED=0` inside `go test`. Commit 81baaac.
- **Spec deviations**:
  - `internal/statedir/statedir_test.go` is added to hold `TestStateDirMode0700`.
  - The extra exported API is `ErrStaleGeneration`, `ErrInvalidEvent`, `DBName`, `SchemaVersion`, `EnvelopeVersion` and `Close`.
  - The `CGO_ENABLED=0` build is a test, not a `ci.yml` step, because `ci.yml` belongs to Tasks 2 and 3.
  - `ids.New("run")` adds the `_` itself.
  - The payload must be a JSON object (I09).
  - The exit-2 mapping for `ErrSchemaTooNew` in `internal/cli/exit.go` is left to the first command that opens the journal (Task 5).
  - `Open` does not create the directory.
  - `.github/workflows/dependency-review.yml` allows `LicenseRef-scancode-google-patent-license-golang`, the Go patent grant on `golang.org/x/sys`, which the required dependency review otherwise rejects.
  - `statedir.Ensure` refuses a symlinked state directory.
- **Files modified**: `.github/workflows/dependency-review.yml`, `go.mod`, `go.sum`, `internal/statedir/statedir.go`, `internal/statedir/statedir_test.go`, `internal/ids/ids.go`, `internal/ids/ids_test.go`, `internal/journal/journal.go`, `internal/journal/journal_test.go`, `internal/journal/migrations/0001_init.sql`, `docs/decisions/0003-local-state-sqlite.md`, `specs/todo/dogfood-slice/tasks.md`.

### Task 5 — State machines, projections and `runs list` ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 4
- **Change**: Encode the run and attempt transition tables from design §4. Each transition appends its journal event and its projection update in one transaction. Implement `mythhelm runs list` in plain and JSONL.
- **Files**:
  - `internal/supervisor/state.go`
  - `internal/supervisor/state_test.go`
  - `internal/journal/projections.go`
  - `internal/cli/runs.go`
  - `internal/cli/runs_test.go`
- **Produces**:
  - `supervisor.TransitionRun(ctx, j *journal.Journal, runID string, to RunState, reason string, producer Producer) error`
  - `supervisor.TransitionAttempt(...)` (same shape)
  - `journal.ListRuns(ctx, limit int) ([]RunRow, error)`
- **Acceptance**:
  - `TestIllegalRunTransitionRejected`: for example, `created → verifying` errors and appends nothing.
  - `TestTransitionAtomicWithJournal`: an injected projection error leaves no journal row.
  - `TestRunsListReadsProjectionsOnly`: runs with a read-only database handle.
  - `TestRunsListJSONLOneObjectPerLine`.
  - `BenchmarkRunsList100`: reported, advisory (NFR-5).
- **Test plan**: seeded journals; golden plain output.
- **Invariants touched**: §7.2, AC-9.3, I06 (state names stay truthful).
- **Status**: ✅ Completed — the design §4 run and attempt transition tables, with each transition's event and projection committed in one transaction; projection helpers and a read-only journal handle; `runs list` in plain and JSONL (`run_row`); `ErrSchemaTooNew` exits 2; PR #16.
- **Implementation**: The project callback reads the current state, checks it against the table and updates the projection inside `Append`'s `BEGIN IMMEDIATE` transaction, so an illegal transition or a failed projection journals nothing and concurrent transitions serialise. `Producer` spends a sequence number only on commit. `OpenReadOnly` is a `mode=ro`, `query_only` handle. `BenchmarkRunsList100` measures about 4 ms/op locally. Commit 4b98849.
- **Spec deviations**:
  - `TransitionRun` and `TransitionAttempt` take `*Producer`.
  - `TransitionAttempt` takes `attemptID` in place of `runID`.
  - `ListRuns` is a method on a `*journal.Journal` opened with the new `OpenReadOnly`.
  - The extra exported API is `CreateRun`, `RecordLaunchIntent`, `NewProducer`, `ErrIllegalTransition`, the projection helpers, `(*Journal).Attempt`, `ErrNotFound` and `ErrNoDatabase`.
  - An attempt can become `interrupted` from any non-terminal state (§7.3, AC-11.3).
  - `blocked`, `failed`, `interrupted` and `failed_native` require a reason, and `ready_for_review` accepts only an empty reason or `unverified`.
  - `internal/cli/dispatch.go` registers `runs` and prints the usage hint only for usage errors.
  - `runs list` does not yet show worker liveness (Tasks 9 and 15).
- **Files modified**: `internal/supervisor/state.go`, `internal/supervisor/state_test.go`, `internal/journal/projections.go`, `internal/cli/runs.go`, `internal/cli/runs_test.go`, `internal/cli/dispatch.go`, `internal/cli/exit.go`, `specs/todo/dogfood-slice/tasks.md`.

### Task 6 — Security primitives ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 1
- **Change**: Implement the redacting slog handler and `Redact`, `TermSafe`, the env allowlist and denylist builder (design §6.3, §11), and the symlink-escape path check.
- **Files**:
  - `internal/security/redact.go`
  - `internal/security/termsafe.go`
  - `internal/security/env.go`
  - `internal/security/paths.go`
  - `internal/security/security_test.go`
- **Produces**:
  - `security.Redact(string) string`
  - `security.NewRedactingHandler(h slog.Handler) slog.Handler`
  - `security.TermSafe(string) string`
  - `security.BuildEnv(parent []string, passthrough []string, set map[string]string) ([]string, error)`
  - `security.ErrDeniedPassthrough`
  - `security.ResolvesOutside(root, path string) (bool, error)`
- **Acceptance**:
  - `TestRedactKnownSecretShapes`: one case per pattern.
  - `TestTermSafeStripsEscapesAndC1`: covers OSC 8 hyperlinks, CSI, a bare ESC and `\x9b`; `\n` and `\t` are kept.
  - `TestBuildEnvAllowlistOnly`.
  - `TestBuildEnvDenylistWinsOverPassthrough`: `ANTHROPIC_API_KEY` in passthrough gives `ErrDeniedPassthrough`.
  - `TestResolvesOutsideSymlinkEscape`.
- **Test plan**: table tests; fuzz `TermSafe` (`FuzzTermSafe`, seed corpus only in CI).
- **Invariants touched**: I19, §12.5, §12.7.
- **Status**: ✅ Completed — `internal/security` with `Redact` + `RedactingHandler`, `TermSafe` (C0/C1/ESC sequences, OSC 8), `BuildEnv` allowlist with the denylist winning (`ErrDeniedPassthrough`) and `ResolvesOutside` (component-wise symlink resolution, dangling links included); PR #9
- **Implementation**: Backfilled after merge; the design decisions are in PR #9 and the spec deviations below. Squash-merge commit a58bde2.
- **Spec deviations**: key/value redaction also matches quoted keys and masks a quoted value to its closing quote (superset); `ResolvesOutside` errors on an unreadable symlink (fail closed); credential-named attributes are dropped by suffix (`…TOKEN`, `…APIKEY`, `…SECRET`, …) so `apiKeySource` and `input_tokens` survive; `BuildEnv` always denies `CLAUDE_CODE_OAUTH_TOKEN` (the AC-4.7 opt-in is the adapter's to apply after `BuildEnv`) and also checks `set` names; control strings end at a newline; `ErrDeniedPassthrough` → exit 2 mapping in `internal/cli/exit.go` is left to the task that wires `[environment] passthrough`.
- **Files modified**: `internal/security/env.go`, `internal/security/paths.go`, `internal/security/redact.go`, `internal/security/security_test.go`, `internal/security/termsafe.go`, `specs/todo/dogfood-slice/tasks.md`.

### Task 7 — Workspace: safe git runner, preflight and snapshot ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: complex
- **Depends on**: Task 6
- **Change**: Implement the git runner with the fixed `-c` safety flags and `GIT_OPTIONAL_LOCKS=0`. Add preflight for git ≥ 2.30, repository state, dirty detection and unsupported features. Add snapshot clone (no hardlinks, detached, remotes removed) and `SourceFingerprint` (design §8).
- **Files**:
  - `internal/workspace/git.go`
  - `internal/workspace/preflight.go`
  - `internal/workspace/snapshot.go`
  - `internal/workspace/fingerprint.go`
  - `internal/workspace/workspace_test.go`
- **Produces**:
  - `workspace.Preflight(ctx, repo string) (Preflight{Top, Branch, HeadRev string; Dirty bool; Unsupported []string}, error)`
  - `workspace.Snapshot(ctx, src, rev, dst string) error`
  - `workspace.SourceFingerprint(repo string) (string, error)`
  - `workspace.Git(ctx, dir string, userRepo bool, args ...string) ([]byte, error)`
- **Acceptance**:
  - `TestPreflightDetectsDirtyTrackedAndUntracked`.
  - `TestPreflightRejectsSubmoduleLFSSparseShallow`: one subtest each.
  - `TestSnapshotHasNoRemotesAndDetachedAtRev`.
  - `TestSnapshotExcludesUntrackedAndIgnored`.
  - `TestPreflightDoesNotWriteUserIndex`: the fingerprint and `.git/index` mtime are unchanged after `Preflight` on a repo with a stale stat cache.
  - `TestAgentPlantedHookNotRun`: a `post-checkout` hook written into the clone does not run during the runner's `worktree add`.
- **Test plan**: temp repos built with the git CLI; skip with a clear message if `git` is missing.
- **Invariants touched**: I08, G03, §11.2.
- **Status**: ✅ Completed — `internal/workspace` with the safe `Git` runner (fixed `-c` flags, empty hooks path, `GIT_*` scrub, `GIT_OPTIONAL_LOCKS=0` for the user repo), `Preflight` (git ≥ 2.30 via `ErrGitTooOld`, top/branch/HEAD, dirty, shallow/submodules/lfs/sparse-checkout), `Snapshot` (no hardlinks, detached, all remotes removed) and `SourceFingerprint`; PR #15
- **Implementation**: Backfilled after merge; the design decisions are in PR #15 and the spec deviations below. Squash-merge commit 8f514d4.
- **Spec deviations**: result type is `PreflightResult` (Go cannot name a func and a type `Preflight` in one package); empty hooks path is `os.DevNull` on Unix and a per-process empty temp dir on Windows; the runner drops inherited `GIT_*` variables and sets `GIT_TERMINAL_PROMPT=0` (Task 11 must set its temporary `GIT_INDEX_FILE` explicitly); submodules detected from HEAD-tree gitlinks plus `.gitmodules`, LFS from committed `.gitattributes`, instead of `submodule status`; `Snapshot` resolves `rev` to a full OID first and removes every remote, not only `origin`; the fingerprint covers every working-tree entry (untracked and ignored included) and file permissions, and follows linked-worktree `.git` files.
- **Files modified**: `internal/workspace/fingerprint.go`, `internal/workspace/git.go`, `internal/workspace/preflight.go`, `internal/workspace/snapshot.go`, `internal/workspace/workspace_test.go`, `specs/todo/dogfood-slice/tasks.md`.

### Task 8 — Adapter seam, NDJSON framing and the fake adapter ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: complex
- **Depends on**: Task 1
- **Change**: Define `internal/adapter` (interface, observations, capability record, `BlockedError`) and the bounded `ndjson.Reader`. Implement `adapters/fake` and the hidden `__fake-agent` with the embedded scenarios from design §7.
- **Files**:
  - `internal/adapter/adapter.go`
  - `internal/adapter/ndjson/reader.go`
  - `internal/adapter/ndjson/reader_test.go`
  - `adapters/fake/fake.go`
  - `adapters/fake/agent.go`
  - `adapters/fake/scenarios/*.json`
  - `adapters/fake/fake_test.go`
  - `cmd/mythhelm/main.go` (register `__fake-agent`)
- **Produces**:
  - The `adapter.Adapter`, `Launcher`, `Session`, `Observation` and `CapabilityRecord` types, as in design §7.
  - `ndjson.NewReader(r io.Reader, lim Limits) *Reader` and `(*Reader).Next() ([]byte, error)`.
  - `ndjson.Counters`.
  - `fake.New() adapter.Adapter`.
- **Acceptance**:
  - `TestReaderOversizedFrameDiscarded`: memory stays bounded; `testing.AllocsPerRun` stays under 2× the limit.
  - `TestReaderDepthLimit`.
  - `TestReaderInvalidUTF8Counted`.
  - `TestFakeScenarioHappyEditsFile`.
  - `TestAdaptersDoNotImportOsExec`: `go/parser` over non-test files under `adapters/` must find no `os/exec` import, except `adapters/claudecode/probe.go`.
  - `TestCapabilityRecordUnknownNotOptimistic`: the fake's `billing.entitlement` is `local-scripted`; `hard_monetary_limit` is `unsupported`.
- **Test plan**: the fake agent runs as a real child via the test binary re-exec.
- **Invariants touched**: §9.1, §9.2, §9.7, I11, I14.
- **Status**: ✅ Completed. Adds the `internal/adapter` seam, the bounded `ndjson.Reader` and the `fake` adapter with 13 embedded scenarios, played by `__fake-agent` as a real child; PR #11.
- **Implementation**: Backfilled after merge; the design decisions are in PR #11 and the spec deviations below. Squash-merge commit d1e03a5.
- **Spec deviations**:
  - The build-tagged `adapters/fake/escapee_{unix,other}.go` are added, because `Setsid` exists only on Unix and `os/exec` is banned.
  - `OwnedProc` is defined as `{Stdout, Signal, Wait, GroupGone}`, and `NativeExit` gains `Err`.
  - The capability record gains a `platform` block (NFR-4): `unknown` on Unix until Task 9's tests, and ownership `unsupported` on Windows.
  - The fake agent exits 130 on SIGINT and 143 on SIGTERM unless the scenario ignores the signal.
  - `emit` has four variants: `frame`, `raw`, `raw_base64` and `synth`.
  - `__fake-agent` is dispatched in `main.go`, not the `commands` map.
  - `Prepare` requires a scenario.
  - The oversized-frame bound is measured in bytes, and the object count separately with `AllocsPerRun`.
  - The `doc.go` note is in the package comment.
  - `ProbeInput` is empty.
- **Files modified**: `adapters/fake/agent.go`, `adapters/fake/escapee_other.go`, `adapters/fake/escapee_unix.go`, `adapters/fake/fake.go`, `adapters/fake/fake_test.go`, `adapters/fake/scenarios/bad-utf8.json`, `adapters/fake/scenarios/check-fails.json`, `adapters/fake/scenarios/deep.json`, `adapters/fake/scenarios/denied.json`, `adapters/fake/scenarios/escapee.json`, `adapters/fake/scenarios/exit-before-result.json`, `adapters/fake/scenarios/happy.json`, `adapters/fake/scenarios/ignore-sigint.json`, `adapters/fake/scenarios/ignore-term.json`, `adapters/fake/scenarios/malformed.json`, `adapters/fake/scenarios/native-fails.json`, `adapters/fake/scenarios/oversized.json`, `adapters/fake/scenarios/slow.json`, `cmd/mythhelm/main.go`, `internal/adapter/adapter.go`, `internal/adapter/ndjson/reader.go`, `internal/adapter/ndjson/reader_test.go`, `specs/todo/dogfood-slice/scratchpad.md`, `specs/todo/dogfood-slice/tasks.md`.

### Task 9 — Worker ownership: detached worker, process group, spool, stop ladder ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: complex (this is the hardest task; the ceiling may be exceeded)
- **Depends on**: Task 4, Task 8
- **Change**: Implement `mythhelm __worker`, per design §3. It detaches (Setsid on Unix; creation flags on Windows), writes `worker.json` with identity and start time, and launches the native process in its own process group through `Launcher`. It maps observations to envelope events in `spool.jsonl` and fsyncs critical events. It writes a heartbeat, polls `stop.request`, and runs the adapter's stop ladder, confirming the process group is gone. Write ADR 0004 (process model).
- **Files**:
  - `internal/workers/worker.go`
  - `internal/workers/proc_unix.go`
  - `internal/workers/proc_linux.go`
  - `internal/workers/proc_darwin.go`
  - `internal/workers/proc_windows.go`
  - `internal/workers/spool.go`
  - `internal/workers/worker_test.go`
  - `docs/decisions/0004-slice-process-model.md`
- **Produces**:
  - `workers.Main(args []string) int`
  - `workers.ProcessStartTime(pid int) (time.Time, error)`
  - `workers.ReadIdentity(dir string) (Identity, error)`
  - `workers.RequestStop(dir, requestID string) error`
  - The spool line format is one `journal.Event` per line.
- **Acceptance**:
  - `TestWorkerSurvivesParentExit` (Linux, macOS): kill the spawning test helper; the worker keeps writing heartbeats.
  - `TestStopLadderEscalatesToKill`: the `ignore-term` scenario goes SIGINT → SIGTERM → SIGKILL, and `attempt.stopped{confirmed:true}` is reached.
  - `TestFakeStopExit130ClassifiedStopped`: a child that exits 130 on the first SIGINT is recorded as `stopped`, not `failed_native`.
  - `TestStoppedExitEmitsNativeResultWithNulls`: a stopped attempt's `attempt.native_result` has `result_observed:false` and explicit `null` result fields, and its attempt state is `stopped`.
  - `TestUnresolvedDescendantReported` (Linux, macOS): the `escapee` scenario reports a PID in `unresolved_pids`.
  - `TestSpoolSequencesContiguous`.
  - `TestCriticalEventsFsynced`: through the injected `syncer` seam.
  - `TestProcessStartTimeStable`: two reads agree; a different PID differs.
  - `TestWindowsFakeStopConfirmed`: Windows CI; `Process.Kill` then `Wait`.
- **Test plan**: the test binary re-exec plays both the worker and the fake agent. Platform-specific tests use build tags and `t.Skip` with a reason elsewhere.
- **Invariants touched**: I06, I12, I18, §7.3, §7.4, G04 (Linux and macOS only).
- **Status**: ✅ Completed. Adds `internal/workers`: the detached `__worker`, which writes the `worker.json` identity, spools with contiguous sequences and fsyncs critical events, heartbeats, polls `stop.request`, runs the ladder on the process group and reports escaped descendants. Also adds ADR 0004, and the fake's platform facts are now `supported` on Linux and macOS; PR #17.
- **Implementation**: Backfilled after merge; the design decisions are in PR #17 and the spec deviations below. Squash-merge commit adaf2ce.
- **Spec deviations**:
  - `attempt.stopped` ends every attempt, because `unresolved_pids` must also be reported for a self-exiting escapee. The attempt state is `stopped` only after a stop request.
  - `attempt.stopped` adds `signals_sent` and `descendant_scan`, and `attempt.protocol_counters` adds `progress_dropped`.
  - The worker receives `workers.Launch` as JSON on its stdin, never on disk. `Spawn` and `AttemptDir` are added.
  - Extra files: `worker.log`, `proc_other.go` and `worker_unix_test.go`.
  - New reason codes: `launch_failed`, `worker_persistence_failed`, `native_signal_<name>`, and the native error class.
  - Files touched outside the list: `adapters/fake/fake.go` and `fake_test.go`, `cmd/mythhelm/main.go` and `go.mod`.
- **Files modified**: `adapters/fake/fake.go`, `adapters/fake/fake_test.go`, `cmd/mythhelm/main.go`, `docs/decisions/0004-slice-process-model.md`, `go.mod`, `internal/workers/proc_darwin.go`, `internal/workers/proc_linux.go`, `internal/workers/proc_other.go`, `internal/workers/proc_unix.go`, `internal/workers/proc_windows.go`, `internal/workers/spool.go`, `internal/workers/worker.go`, `internal/workers/worker_test.go`, `internal/workers/worker_unix_test.go`, `specs/todo/dogfood-slice/scratchpad.md`, `specs/todo/dogfood-slice/tasks.md`.

### Task 10 — Supervisor run pipeline, first against the fake adapter ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: complex
- **Depends on**: Task 5, Task 7, Task 9
- **Change**: Implement `mythhelm run`. It covers:
  - Generic admission (AC-2.1–2.4): adapter probe, `trusted-host` consent, active-run refusal, dirty-checkout offer, preflight, and the `local-scripted` billing posture for the fake.
  - Snapshot, and launch intent with a token.
  - Spawning the worker, verifying its identity, and ingesting the spool into the journal from the stored offset.
  - Holding `owner.lock`, and Ctrl-C handling (AC-5.5).
  - Plain and JSONL renderers, and exit codes 0, 3, 4, 6, 7 and 130 for the stages that exist so far. Native success ends in a temporary `verifying` stub until Task 12.
- **Files**:
  - `internal/admission/admission.go`
  - `internal/supervisor/pipeline.go`
  - `internal/supervisor/ingest.go`
  - `internal/supervisor/ownerlock.go`
  - `internal/cli/run.go`
  - `internal/cli/render.go`
  - `internal/supervisor/pipeline_test.go`
- **Produces**:
  - `admission.Decide(ctx, Request) (Decision, error)`
  - `supervisor.Run(ctx, Decision, Hooks) (Outcome, error)`
  - `supervisor.Ingest(ctx, j, attempt) (lastSeq int64, err error)`
  - `supervisor.AcquireOwner(runDir string) (release func(), err error)`
  - `supervisor.ErrOwnerHeld`
- **Acceptance**:
  - `TestRunNoAdapterFlagExits2`.
  - `TestRunSecondActiveRunBlocked` (exit 3).
  - `TestDirtyCheckoutNonInteractiveBlocked` (exit 3); `--use-committed` proceeds.
  - `TestRestrictedProfileExit7`.
  - `TestHostHerdrExit7`.
  - `TestAdmissionDecidedBeforeWorkerSpawn`: the journal order is `admission.decided` < `attempt.launch_intent_recorded` < `attempt.launched`.
  - `TestIngestResumesFromOffsetWithoutDuplicates`.
  - `TestCtrlCOnceStopsAndExits130`.
  - `TestCtrlCTwiceDetachesExit6`.
  - `TestJSONLStdoutOnlyEnvelopes`: every stdout line parses as an `Event`, and the last line is `run.result`.
- **Test plan**: signal tests send SIGINT to a child `mythhelm run` (Unix); Windows uses `GenerateConsoleCtrlEvent` or skips with a reason.
- **Invariants touched**: I02, I04, I05, I06, I08, §5.6, §15.10.
- **Status**: ✅ Completed — `mythhelm run` admits a task and supervises its one fake-adapter attempt under the run's owner lock: admission decided, snapshot, launch intent, worker spawn and identity check, spool ingestion from the stored offset, Ctrl-C stop (exit 130) and detach (exit 6), and plain and JSONL renderers; PR #20.
- **Implementation**: `admission.Decide` refuses on flags, capability, consent, preflight and the dirty checkout before anything is written. `supervisor.Run` records the run, checks for other active runs after that (race-free), then snapshots, spawns and ingests until the attempt ends, mapping the outcome to exit 3, 4, 5, 6 or 130 in `internal/cli/exit.go`. An ingestion failure leaves the run `interrupted`/`ingest_failed`. ADR 0005 records the owner lock and ingestion. Commits 2706e9b, 8c386e2, efceef2.
- **Spec deviations**:
  - A native success ends `verifying` → `failed`/`verification_unavailable` with exit 5, not 0. No frozen or verified candidate exists yet (I07), and a run left in `verifying` would block every later run as active. Tasks 11–12 replace this.
  - `--adapter claudecode` exits 7 (`adapter_unavailable`) until Tasks 16–17.
  - `run.result` adds `attempt_reason`, the native reason design §6.4 says it carries, such as `result_unobserved`.
  - A worker that has already exited and been reaped is accepted on its PID and token when its start time can no longer be read (ADR 0005).
  - Flags for later tasks are refused (exit 2), not ignored. `Ingest` takes `AttemptRef{StateDir, RunID, AttemptID}`.
  - `internal/journal/projections.go`: the supervisor cannot query the database except through the journal package. It adds the spool offset (read, set, advance), process IDs, session ID, `RunsInStates` and `ProducerSequence`.
  - `internal/supervisor/state.go` and `state_test.go`: adds `launching → failed_native`, the worker's `launch_failed` path in ADR 0004, which the table lacked. `TransitionAttempt` now shares `setAttemptState` with ingest.
  - `internal/cli/dispatch.go` and `internal/cli/exit.go`: they register `run`, and all exit-code mapping lives in one place.
  - `internal/supervisor/ownerlock_unix.go` and `ownerlock_windows.go`: the lock calls differ per platform.
  - `docs/decisions/0005-run-owner-lock-and-ingestion.md`: the process-ownership decision record required in the same PR.
- **Files modified**: `docs/decisions/0005-run-owner-lock-and-ingestion.md`, `internal/admission/admission.go`, `internal/cli/dispatch.go`, `internal/cli/exit.go`, `internal/cli/render.go`, `internal/cli/run.go`, `internal/journal/projections.go`, `internal/supervisor/ingest.go`, `internal/supervisor/ownerlock.go`, `internal/supervisor/ownerlock_unix.go`, `internal/supervisor/ownerlock_windows.go`, `internal/supervisor/pipeline.go`, `internal/supervisor/pipeline_test.go`, `internal/supervisor/state.go`, `internal/supervisor/state_test.go`, `specs/todo/dogfood-slice/tasks.md`.

### Task 11 — Candidate freeze and validation flags ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 10
- **Change**: Implement §11.5 steps 1–3 (design §8 Freeze): capture the whole tree through a temporary index into a commit on the base, set a candidate ref, and collect changed paths and blob IDs, the patch SHA-256 and the validation flags. Wire it into the pipeline after a native exit is confirmed. Freeze a `partial` candidate on failure or cancellation.
- **Files**:
  - `internal/integration/candidate.go`
  - `internal/integration/flags.go`
  - `internal/integration/candidate_test.go`
  - `internal/supervisor/pipeline.go`
- **Produces**:
  - `integration.Freeze(ctx, workspace, baseRev string, meta CommitMeta) (Candidate, error)`
  - `integration.Candidate{BaseRev, Commit, Tree, PatchSHA256 string; Changed []ChangedPath; Flags []Flag; Partial bool}`
- **Acceptance**:
  - `TestFreezeIncludesUncommittedAndUntracked`.
  - `TestFreezeIgnoresAgentMovedHEAD`: the agent committed and then reset; the candidate still reflects the working tree, with its parent on the admitted base.
  - `TestFreezeWaitsForStopConfirmation`.
  - `TestFreezeRefusedWithUnresolvedDescendants` (Linux, macOS): with the `escapee` scenario, the run ends `interrupted`/`unresolved_descendants` with exit 6. No candidate row exists, and `apply` refuses.
  - `TestFlagsSymlinkEscapeBinaryLargeSecretConfigChange`: one subtest per flag.
  - `TestCandidateHasNoSignedOffBy`.
- **Test plan**: fixture workspaces; secret fixtures use obviously fake values.
- **Invariants touched**: I07, §11.5, §11.6, §12.7.
- **Status**: ✅ Completed — a confirmed worker stop freezes the complete workspace into a candidate commit on the admitted base; failed and cancelled attempts get partial candidates, while unresolved ownership gets none; PR #49.
- **Implementation**: `integration.Freeze` uses a private Git index, streams the binary patch hash and blob scans, sets `refs/mythhelm/candidates/<attempt>`, and returns changed blob IDs plus path-only validation flags. The supervisor journals `candidate.frozen` and inserts its candidate projection atomically after `attempt.stopped`. Admission captures the source Git identity before launch. Named acceptance tests cover working-tree capture, moved HEAD, stop ordering, unresolved escapees, flags, no DCO trailer, and CRLF normalization.
- **Spec deviations**:
  - Verification still ends `failed`/`verification_unavailable` on native success until Task 12; freezing alone is not verification.
  - `apply` remains unavailable until Task 14, so the unresolved-descendant acceptance test confirms the current CLI refuses it and creates no branch; Task 14 must retain that refusal after implementing `apply`.
  - A missing Git user.name or user.email blocks admission with `git_identity_unavailable`, so a candidate never falls back to an inferred machine identity.
  - `internal/admission/admission.go` captures and journals that identity; `internal/journal/projections.go` adds the candidate row API; `internal/security/redact.go` exposes names for the existing secret patterns; `internal/workspace/git.go` adds safe temporary-index and streaming Git calls; `internal/supervisor/pipeline_test.go` exercises stop ordering, partial results and escapees. These supporting files are required for the listed freeze API and pipeline change.
- **Files modified**: `internal/admission/admission.go`, `internal/integration/candidate.go`, `internal/integration/flags.go`, `internal/integration/candidate_test.go`, `internal/journal/projections.go`, `internal/security/redact.go`, `internal/supervisor/pipeline.go`, `internal/supervisor/pipeline_test.go`, `internal/workspace/git.go`, `specs/in-progress/dogfood-slice/tasks.md`, `specs/in-progress/dogfood-slice/handoff.md`.

### Task 12 — Project config, trust grants and the check runner ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: complex
- **Depends on**: Task 11
- **Change**: Parse `mythhelm.toml` strictly from the admitted snapshot, with the digest as trust key and unknown keys rejected. Add `project_config` trust grants (interactive, or `--trust-project-config`). Run checks with argv in a MYTHHELM worktree of the candidate, with timeouts, bounded redacted evidence and the status set from design §8. Wire in `verifying → ready_for_review | failed` (exit 5), `--no-checks` and `--keep-going`.
- **Files**:
  - `internal/admission/projectconfig.go`
  - `internal/admission/trust.go`
  - `internal/integration/verify.go`
  - `internal/integration/verify_test.go`
  - `internal/admission/projectconfig_test.go`
  - `internal/supervisor/pipeline.go`
  - `mythhelm.toml` (MYTHHELM's own checks: gofmt, vet, test)
  - `go.mod`, `go.sum` (BurntSushi/toml)
- **Produces**:
  - `admission.LoadProjectConfig(snapshotDir string) (ProjectConfig, digest string, err error)`
  - `admission.HasTrust(ctx, j, kind, repoID, digest string) (bool, error)`
  - `integration.RunChecks(ctx, cand Candidate, cfg ProjectConfig, env []string) (Verification, error)`
- **Acceptance**:
  - `TestUnknownTOMLKeyExits2`.
  - `TestChecksReadFromSnapshotNotCandidate`: the agent edits `mythhelm.toml`, the admitted checks still run, and the `check_config_changed` flag is set.
  - `TestUntrustedChecksNonInteractiveBlocked` (exit 3).
  - `TestGofmtFailOnOutput`.
  - `TestCheckTimeoutKillsGroup`.
  - `TestMissingExecutableUnavailable` (exit 5).
  - `TestNativeSuccessChecksFailExit5`.
  - `TestNativeSuccessChecksPassExit0ReadyForReview`.
  - `TestNoChecksExit5Unverified`: `--no-checks` gives `ready_for_review`/`unverified` with exit 5, and `apply` without `--accept-unverified` exits 3.
  - `TestEvidenceRedactedAndHashed`.
- **Test plan**: checks are argv to the test binary's helper modes, so they are portable across OSes.
- **Invariants touched**: I03, I07, G06, §11.6, §12.4.
- **Status**: ✅ Completed — strict, digest-bound project config and trust grants; candidate checks run in a managed detached worktree; pass/fail/unverified results and exit codes are wired to the supervisor; PR #50.
- **Implementation**: Admission reads `mythhelm.toml` from the selected committed revision and compares its digest with the snapshot before launch. A grant is keyed by source realpath and digest and projected with `workspace.snapshot_created`. Checks use admitted argv, a separate candidate worktree, process-group timeouts, bounded redacted evidence and the five design statuses. `--no-checks` labels the result `NOT RUN`, ends `ready_for_review`/`unverified` and exits 5. Named acceptance tests cover strict keys, trust, candidate config edits, output policy, group timeout, unavailable executable, pass/fail outcomes and evidence hashes.
- **Spec deviations**:
  - `apply` and `--accept-unverified` arrive in Task 14. The current CLI still refuses `apply`; Task 14 must assert that an unverified run requires the flag.
  - `internal/admission/admission.go`, `internal/cli/run.go`, `internal/cli/exit.go`, `internal/cli/render.go`, `internal/integration/candidate.go`, `internal/integration/process_unix.go`, `internal/integration/process_windows.go`, `internal/journal/projections.go`, `internal/supervisor/pipeline_test.go` and `docs/decisions/0005-run-owner-lock-and-ingestion.md` are supporting edits needed to carry the admitted config, expose CLI flags, label `NOT RUN`, execute checks, project results, update the decision record and test end-to-end outcomes.
- **Files modified**: `go.mod`, `go.sum`, `docs/decisions/0005-run-owner-lock-and-ingestion.md`, `internal/admission/admission.go`, `internal/admission/projectconfig.go`, `internal/admission/projectconfig_test.go`, `internal/admission/trust.go`, `internal/cli/exit.go`, `internal/cli/render.go`, `internal/cli/run.go`, `internal/integration/candidate.go`, `internal/integration/process_unix.go`, `internal/integration/process_windows.go`, `internal/integration/verify.go`, `internal/integration/verify_test.go`, `internal/journal/projections.go`, `internal/supervisor/pipeline.go`, `internal/supervisor/pipeline_test.go`, `mythhelm.toml`, `specs/in-progress/dogfood-slice/tasks.md`, `specs/in-progress/dogfood-slice/handoff.md`.

### Task 13 — Receipt and `mythhelm review` ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 12
- **Change**: Build `receipt.json` (design §9), written atomically and journaled with its hash. `mythhelm review <run>` renders the summary, flags, evidence paths and the candidate diff through `TermSafe`; `--format jsonl` emits the receipt object.
- **Files**:
  - `internal/supervisor/receipt.go`
  - `internal/supervisor/receipt_test.go`
  - `internal/cli/review.go`
  - `internal/cli/review_test.go`
- **Produces**:
  - `supervisor.BuildReceipt(ctx, j, runID string) (Receipt, error)`
  - `supervisor.WriteReceipt(dir string, r Receipt) (sha256 string, err error)`
- **Acceptance**:
  - `TestReceiptHasAllSection9Keys`: a golden key set.
  - `TestReceiptUnknownsNeverZero`: absent usage gives `"unknown"`, not `0`.
  - `TestReceiptCostLabelledEstimate`.
  - `TestNoCredentialValuesPersisted`: planted fake secrets in env and settings never appear anywhere under the state dir (AC-4.6).
  - `TestReviewSanitisesMaliciousFilename`: a filename containing `\x1b]8;;` comes out inert.
  - `TestReviewJSONLSingleObject`.
- **Test plan**: golden files; fake-adapter pipeline runs.
- **Invariants touched**: I09, I19, §17.1, §12.7.
- **Status**: ✅ Completed — version 1 receipt, journaled SHA-256 and read-only review command; PR #51.
- **Implementation**: Terminal runs build a receipt from journal events and projected candidate/check records, write it atomically, and journal its hash. `review` verifies that hash, renders flags, evidence and a TermSafe candidate diff, or emits one JSONL object. Acceptance tests cover unknown measurements, cost labelling, credential non-persistence, malicious filenames, JSONL shape and a real fake-adapter diff. Commit ee82dc0.
- **Spec deviations**: `internal/admission/admission.go` exports the task-title parser so the receipt can read the digest-checked `task.md` without journaling task text; `internal/journal/journal.go` exposes the opened state directory. `internal/journal/projections.go`, `internal/supervisor/pipeline.go` and `internal/cli/dispatch.go` add the read projections, terminal receipt step and command registration. `internal/supervisor/identity_error_windows.go`, `internal/supervisor/identity_error_other.go` and `internal/supervisor/identity_error_windows_test.go` restrict the Windows worker identity retry to transient sharing violations during atomic replacement (CI exposed an existing race). Fields the current adapters cannot establish remain `"unknown"` (including hooks and allowed tools).
- **Files modified**: `internal/admission/admission.go`, `internal/cli/dispatch.go`, `internal/cli/review.go`, `internal/cli/review_test.go`, `internal/journal/journal.go`, `internal/journal/projections.go`, `internal/supervisor/identity_error_other.go`, `internal/supervisor/identity_error_windows.go`, `internal/supervisor/identity_error_windows_test.go`, `internal/supervisor/pipeline.go`, `internal/supervisor/receipt.go`, `internal/supervisor/receipt_test.go`, `specs/in-progress/dogfood-slice/tasks.md`, `specs/in-progress/dogfood-slice/handoff.md`.

### Task 14 — Guarded apply ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 13
- **Change**: `mythhelm apply <run> --to-branch <b>`, per design §8 Apply: reconcile first, preflight, journal the intent, `fetch` with no force into the user's repository, verify, journal the result, then transition to `completed` and write receipt v2.
- **Files**:
  - `internal/workspace/apply.go`
  - `internal/workspace/apply_test.go`
  - `internal/cli/apply.go`
  - `internal/supervisor/receipt.go`
- **Produces**: `workspace.ApplyBranch(ctx, userRepo, workspace, candidateRef, branch, candidateCommit string) error`
- **Acceptance**:
  - `TestApplyCreatesOnlyTheBranch`: the fingerprint differs only by the new ref. Working tree, index, `HEAD`, config and every other ref are identical, and there is no `FETCH_HEAD`.
  - `TestApplyRefusesExistingBranch` (exit 3).
  - `TestApplyRefusesNotReadyForReview`.
  - `TestApplyRefusesUnacceptedFlags`.
  - `TestApplyReconcilesAfterCrash`: the branch already equals the candidate, so the result is `completed` without a second fetch.
  - `TestApplyRefusesInvalidRefName`.
  - `TestApplyDoesNotRunUserHooks`: a `reference-transaction` hook in the user repo, which would touch a marker file, does not run.
- **Test plan**: temp user repos; simulate a crash by journaling the intent and then running apply again.
- **Invariants touched**: I08, G03, §11.7, §7.5.
- **Status**: ✅ Completed — guarded branch creation, crash reconciliation and version 2 receipt; PR #52.
- **Implementation**: `apply` requires a ready candidate, a valid new branch, an existing admitted base commit, and explicit acceptance of validation flags or unverified checks. It journals intent before object import and atomic branch creation; a matching branch completes without another fetch, and in-flight policy conflicts become terminal blocked with a refreshed receipt. It preserves the journal-verified version 1 receipt and atomically writes and journals version 2 with the branch effect. Tests cover source fingerprint, existing/concurrent branches, run state, flags, unverified acceptance, crash reconciliation, invalid names, user hooks and the success/refusal `apply.result` JSONL schema. Commits f1b40c9, 90f3f03, bc3ef30 8b812a1 and bb52b37.
- **Spec deviations**: `internal/workspace/apply.go` imports objects with a source-only, no-force fetch and creates the branch in a prepared, no-deref `update-ref` transaction, rejecting symbolic destinations while the ref lock is held. The design's destination fetch refspec can fast-forward a branch created concurrently after preflight; the atomic creation closes that race. `internal/workspace/apply_test.go` tests ordinary and symbolic competing writes and reflog shorthand rejection. `internal/workspace/git.go` shares the safe command factory with the interactive ref transaction. `docs/decisions/0006-guarded-branch-apply.md` records this security and persistence decision. `internal/supervisor/apply.go` owns apply orchestration and journal events under the run owner lock; `internal/supervisor/apply_test.go` exercises the real fake-adapter CLI and crash path. `internal/cli/dispatch.go` registers the command. Initial apply refuses all existing branches (AC-8.3); only recorded intent permits matching-branch reconciliation (AC-8.4), tightening the design's broadly stated reconcile-first step. `internal/cli/review.go` uses the shared receipt digest verifier introduced in `internal/supervisor/receipt.go`.
- **Files modified**: `docs/decisions/0006-guarded-branch-apply.md`, `internal/workspace/apply.go`, `internal/workspace/apply_test.go`, `internal/workspace/git.go`, `internal/cli/apply.go`, `internal/cli/dispatch.go`, `internal/cli/review.go`, `internal/supervisor/apply.go`, `internal/supervisor/apply_test.go`, `internal/supervisor/receipt.go`, `specs/in-progress/dogfood-slice/tasks.md`, `specs/in-progress/dogfood-slice/handoff.md`.

### Task 15 — Stop, recover, and the §18.4 fault-injection subset ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: complex
- **Depends on**: Task 12
- **Change**: `mythhelm stop <run>` (AC-10.4) and `mythhelm recover <run>`, with the three outcomes of AC-10.1, identity verification and the Linux `/proc` orphan scan. It never relaunches the native process. Complete the named fault suite (AC-11.3).
- **Files**:
  - `internal/supervisor/recover.go`
  - `internal/cli/stop.go`
  - `internal/cli/recover.go`
  - `internal/supervisor/faults_test.go`
  - `internal/workers/orphan_linux.go`
- **Produces**: `supervisor.Recover(ctx, j, runID string) (RecoveryOutcome, error)`
- **Acceptance** (each test uses the fake adapter):
  - `TestCrashAfterLaunchIntentBeforeAck`: exactly one worker, reattached.
  - `TestWorkerPIDReuseTreatedAsLost`: the forged start time is never signalled.
  - `TestRecoverContinuesAfterNativeExit`.
  - `TestRecoverQuarantinesLostWorker`: no new writer is launched.
  - `TestRecoverNeverRelaunchesNative`: the fake records its launch count as 1.
  - `TestStopReportsRequestedUntilConfirmed`.
  - `TestHeadlessPermissionDenialNoHang`: the `denied` scenario completes and the denial is in the receipt.
  - `TestMalformedDeepOversizedBadUTF8Bounded`: one subtest each; the attempt fails `protocol_error` or counts, and memory stays bounded.
  - `TestLockedDatabaseStopsAdmission`: exit 3, and no worker is spawned.
  - `TestOwnerLockHeldByLiveProcess`: `recover` exits 6.
- **Test plan**: harness helpers from design §12 (forge identity, hold lock, hold an exclusive transaction).
- **Invariants touched**: I06, I12, §7.7, §17.5, §18.4, G04.
- **Status**: ✅ Completed — owner-locked stop/recovery, the §18.4 fault suite and apply receipt crash repair; PR #53.
- **Implementation**: `recover` re-reads state under the owner lock, reattaches or quarantines without native relaunch, and reports stop_requested until confirmed. Applying/completed runs reconcile through the journaled intent: resumable states delegate to ApplyRun (absent branches complete, conflicts block with a fresh v1 receipt), and a v2 file without a journaled digest is adopted only after the preserved v1 verifies against the journal and every v2 field proves genuine; anything else stays ownership unresolved with no effects. Commits 5eba2ee, b8dd134, 71e2aa2, aeb96d3, bfc9ca8, 458068d and 4477f5d.
- **Spec deviations**: Supporting changes outside Files, required for the task's ownership and fault acceptance: `internal/cli/dispatch.go` and `internal/cli/exit.go` (command registration, exit mapping); `internal/supervisor/pipeline.go` (continuation, freeze-after-stop), `internal/supervisor/receipt.go` (terminal repair) and `internal/supervisor/stop.go` (verified request path); `internal/workers/worker.go` and `internal/workers/worker_test.go` (descendant identities, panic boundary), `internal/workers/spool.go` (terminal-write tracking), `internal/workers/proc_linux.go` (zombie recognition), `internal/workers/orphan.go`, `internal/workers/orphan_other.go` and `internal/workers/orphan_contract_test.go` (portable marker helpers, fail-closed scan contract); `adapters/fake/agent.go`, `adapters/fake/fake_test.go` and `adapters/fake/scenarios/count-launches.json` (launch-count fixture); `docs/decisions/0007-stop-and-recovery.md` (decision record). A recovered quarantined partial candidate ends failed/recovered_partial (exit 4); it is never verified success. An unfinished check without a durable verification result remains ownership unresolved instead of repeating external effects. A panic after a terminal spool line is written preserves that potentially ingested state and logs worker_panic instead of appending an illegal second terminal transition (design §11 boundary clarification). The journaled apply intent is the standing instruction to complete that explicitly requested apply, so recovery creates an absent intent-bound branch rather than stranding the run in applying; a completed run's disturbed destination is manual reconciliation, never silent repair. Stop decides from the last journaled stopped event, matching recovery; a freeze that classifies the run itself keeps its reason instead of a failed→failed transition.
- **Files modified**: `adapters/fake/agent.go`, `adapters/fake/fake_test.go`, `adapters/fake/scenarios/count-launches.json`, `docs/decisions/0007-stop-and-recovery.md`, `internal/cli/dispatch.go`, `internal/cli/exit.go`, `internal/cli/recover.go`, `internal/cli/stop.go`, `internal/supervisor/faults_test.go`, `internal/supervisor/pipeline.go`, `internal/supervisor/receipt.go`, `internal/supervisor/recover.go`, `internal/supervisor/stop.go`, `internal/workers/orphan.go`, `internal/workers/orphan_contract_test.go`, `internal/workers/orphan_linux.go`, `internal/workers/orphan_other.go`, `internal/workers/proc_linux.go`, `internal/workers/spool.go`, `internal/workers/worker.go`, `internal/workers/worker_test.go`, `specs/in-progress/dogfood-slice/handoff.md`, `specs/in-progress/dogfood-slice/tasks.md`.

### Task 16 — Claude Code probe, auth evidence and billing admission ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: complex
- **Depends on**: Task 12
- **Change**: `adapters/claudecode` probe and capabilities (design §6.1). Add admission for billing posture (§6.2 steps 1–4): environment overrides, the settings inventory with `native_config` trust grants, `auth status` evidence, declarations, `subscription-only` blocking, and `subscription-declared` labelling. Windows is blocked. Write ADR 0002 (dogfood billing posture), recording the maintainer decisions of 2026-09-29: `subscription-declared` is the dogfood posture (`--billing` stays required, AC-4.1) and the opt-in `CLAUDE_CODE_OAUTH_TOKEN` route follows AC-4.7.
- **Files**:
  - `adapters/claudecode/probe.go`
  - `adapters/claudecode/compat.go`
  - `adapters/claudecode/settings.go`
  - `internal/admission/billing.go`
  - `adapters/claudecode/probe_test.go`
  - `internal/admission/billing_test.go`
  - `docs/decisions/0002-dogfood-billing-posture.md`
- **Produces**:
  - `claudecode.New() adapter.Adapter`
  - `claudecode.InventorySettings(home, workspace string) (Manifest, error)`
  - `admission.ResolveBilling(ctx, mode string, evidence AuthEvidence, decl *Declaration) (BillingPosture, error)`
- **Acceptance**:
  - `TestStrictSubscriptionOnlyAlwaysBlocks` (exit 3, `entitlement_qualification_unavailable`).
  - `TestDeclaredPostureLabelledUnqualified`: `qualified:false`, `paid_continuation:"unknown"`.
  - `TestAPIKeyEnvBlocksNamesOnly`: the message has the name but not the value.
  - `TestStripCredentialEnvRecordedAsOverride`.
  - `TestSettingsEnvCredentialBlocksNoStripOption`.
  - `TestApiKeyHelperBlocks`.
  - `TestAuthStatusMismatchNeedsNativeSetup`: fakeclaude returns `authMethod:"console"`.
  - `TestAuthStatusPIIDropped`: `email` and `orgName` are never persisted. The raw `orgId` fixture value appears nowhere in the database, the receipt or the logs; only `identity_ref` does.
  - `TestUntestedMinorVersionExit7`.
  - `TestResolvedVersionedBinaryLaunched`.
  - `TestWindowsClaudecodeExit7`.
  - `TestProjectHooksRequireNativeTrust`.
  - `TestUserScopeMCPRequiresNativeTrust`: an MCP server defined in the `~/.claude.json` fixture, at user scope and per project, blocks without a grant.
  - `TestClaudeJSONAccountDataNeverDecoded`: the planted session and account fields in `~/.claude.json` are absent from the decode struct (it has no such fields), and the planted values never appear in the manifest or under the state dir.
  - `TestDeclarationBoundToIdentity`: changing `orgId`, or changing `configDirectory`, gets the old declaration rejected with `declaration_identity_mismatch`. Each is a separate subtest.
  - `TestSingleCurrentDeclaration`: declaring twice leaves exactly one row with `superseded_at IS NULL`, and a direct second insert violates `declarations_current`.
- **Test plan**: fakeclaude helper mode (design §12) serves `--version` and `auth status` fixtures; settings fixtures live in temp homes.
- **Invariants touched**: I01, I02, I03, I04, I15 (honest non-satisfaction), I16, I19, §9.3, §13.10, G05 (recorded not-passed).

- **Status**: ✅ Completed — Native probe, PII-safe auth/config evidence, declared billing and transactional declarations implemented; PR #54.
- **Implementation**: Pinned native resolution, strict bounded status/config parsing, complete selected MCP digests and atomic declaration replacement; strict billing remains blocked and declared posture stays unqualified. Commit b301600.
- **Spec deviations**:
  - `internal/adapter/adapter.go` extends the native probe input contract; `internal/admission/admission.go` orders the strict refusal before unavailable adapter checks and carries nullable typed native auth evidence.
  - `internal/journal/declarations.go` supplies transaction-bound projection/read APIs for the existing schema; `internal/supervisor/receipt.go` emits allowlisted native auth for the explicit PII persistence acceptance.
  - `adapters/claudecode/doc.go`, `auth_test.go` and `settings_linux_test.go` separate the required API disclaimer, real auth/receipt fixtures and inotify credential-read proof from the probe tests. Own handoff and scratchpad entries record dependent seams and limits.
  - Native config source symlinks are unsupported; raw config-directory text is omitted from durable JSON while its identity hash remains. Every present settings/MCP source needs trust; ambiguous JSON/auth and unsupported policyHelper sources fail closed. Remote cached policy and macOS MDM are not certified; Q5 network behavior remains unknown after a failed sandboxed trace.
- **Files modified**: `adapters/claudecode/doc.go`, `adapters/claudecode/probe.go`, `adapters/claudecode/compat.go`, `adapters/claudecode/settings.go`, `adapters/claudecode/probe_test.go`, `adapters/claudecode/auth_test.go`, `adapters/claudecode/settings_linux_test.go`, `internal/adapter/adapter.go`, `internal/admission/admission.go`, `internal/admission/billing.go`, `internal/admission/billing_test.go`, `internal/journal/declarations.go`, `internal/supervisor/receipt.go`, `docs/decisions/0002-dogfood-billing-posture.md`, `specs/in-progress/dogfood-slice/tasks.md`, `specs/in-progress/dogfood-slice/handoff.md`, `specs/in-progress/dogfood-slice/scratchpad.md`.
- **Acceptance evidence**: All 16 named checks passed; the full race suite passed through the gate wrapper. Red-first: wrong-value stubs failed every acceptance; symlink-following, omitted lexical MCP digest, skipped strict auth JSON validation and raw-orgId persistence mutants failed retained regressions; restored implementation passed.

### Task 17 — Claude Code launch, stream-json decoding and result mapping ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: complex
- **Depends on**: Task 16, Task 9
- **Change**: Implement `Prepare` and `Start` for `claudecode`: the exact argv, env and stdin prompt (design §6.3), the stream-json decoder over `ndjson.Reader` with its observation mapping (§6.4), the in-flight `apiKeySource` check, the result-mapping table and the SIGTERM → SIGKILL ladder. Add synthetic doc-derived fixtures and a build-tagged live canary.
- **Files**:
  - `adapters/claudecode/launch.go`
  - `adapters/claudecode/decode.go`
  - `adapters/claudecode/decode_test.go`
  - `adapters/claudecode/launch_test.go`
  - `adapters/claudecode/testdata/streams/*.jsonl`
  - `adapters/claudecode/live_test.go` (`//go:build live`)
- **Produces**: `claudecode.Argv(bin string, allowedTools []string) []string` (exported for tests and the receipt).
- **Acceptance**:
  - `TestArgvExact`: golden argv. No `--bare`, `--dangerously-skip-permissions`, `bypassPermissions` or prompt text appears in the argv.
  - `TestPromptOnStdinOnly`: fakeclaude asserts it.
  - `TestChildEnvIsAllowlisted`: planted `ANTHROPIC_API_KEY` and `AWS_SECRET_ACCESS_KEY` are absent from the child; the three MYTHHELM overrides are present.
  - `TestDecodeFixture/{success,error_max_turns,permission_denials,startup_failure,api_retry,hooks_before_init}`.
  - `TestApiKeySourceMismatchStopsAttempt`: the run ends `blocked`/`billing_route_mismatch`, exit 3.
  - `TestBillingMismatchDuringUserStopStillBlocks`: the violation dominates a concurrent user stop (stopping exits to blocked).
  - `TestAuthFailedMapsToBlocked`, with the class journaled as `attempt.native_error` evidence.
  - `TestRateLimitNoInventedCountdown`.
  - `TestNativeErrorPoisonIsSticky`: a later rate limit never washes auth/billing/protocol poison.
  - `TestWorkerRefusesSwappedNative`: launch-time SHA-256 re-verification refuses a swapped binary.
  - `TestAssistantBeforeInitIsProtocolError`.
  - `TestDecodeExplicitNullIsAbsent`, `TestDecodeDuplicateInitIsProtocolError`, `TestDecodeSuccessWithoutInitIsProtocolError`, `TestDecodeAliasConflictIsProtocolError`, `TestDecodeCostGrammar`, `TestDecodeStringBounds`.
  - `TestDecodeInitWithoutCredentialIsDropped` (fake adapter: init without `api_key_source` is dropped).
  - `TestTrustedAllowedToolsReachChildArgv`: trusted `mythhelm.toml` rules reach the child argv tail.
  - `TestReceiptReportsToolsHooksAndTrustGrant`: receipt carries allowed tools, hook count and the native trust grant.
  - `TestCostStoredAsEstimateString`: decimal string, never `float64`, in the journal.
  - `go vet -tags live ./adapters/claudecode` compiles, and CI never passes `-tags live`.
- **Test plan**: fakeclaude replays each fixture through the real worker and launcher.
- **Invariants touched**: I01, I06, I07, I09, I15, §9.3, §9.7, §12.3.

- **Status**: ✅ Completed — Native launch, stream decoding, result mapping and admission wiring implemented; review round adjudicated (architect + code-reviewer); PR #56.
- **Implementation**: Exact argv/stdin/allowlisted env in `Prepare`, owned `Start`/session with a SIGTERM→SIGKILL ladder, fail-closed stream-json decoder on a per-field status model, worker AC-4.4 in-flight stop with sticky poison and launch-time binary re-verification, pipeline blocked mappings, journaled native-error evidence, and the full `run --adapter claudecode` admission path (credential screen, probe, blob-based native inventory with trust, auth evidence, declaration, billing, launch).
- **Spec deviations**:
  - `internal/adapter/adapter.go` extends `PrepareInput` with the pinned `Probe`, `AllowedTools` and `Passthrough` the native launch needs, and hosts the shared `ClimbLadder` both sessions use; `adapters/fake/fake.go` adopts it, and its decoder drops init frames without `api_key_source` (no credential, no session).
  - `internal/workers/worker.go` registers `builtin/claudecode`, runs the AC-4.4 worker-initiated stop, records `stop_requested` for stops discovered while draining (stopped is unreachable from running), journals every `NativeError` as `attempt.native_error` evidence, keeps poison sticky across later rate limits, re-verifies the native SHA-256 at `Start`, and orders classification per the §6.4 table (auth/protocol poison fails, success beats a lone rate_limit which maps to `provider_limit`).
  - `internal/supervisor/pipeline.go` maps `stopped`/`billing_route_mismatch` and auth-class failures to `blocked`, and `protocol_error`/`provider_limit` to failed with their reasons; it also persists fresh declarations and native trust grants in the `admission.decided` transaction. `internal/cli/exit.go` maps run reason `provider_limit` to exit 4.
  - `internal/supervisor/state.go` and design §4 admit `stopping → blocked` (`billing_route_mismatch` only): AC-4.4 is unconditional, so the route violation dominates a concurrent user stop.
  - `internal/supervisor/receipt.go` reports the trusted `allowed_tools`, the inventoried hook count and the `native_config:sha256:…` trust grant from the journaled admission record instead of `unknown`.
  - `internal/admission/admission.go`, `native.go` and `billing.go` plus `internal/cli/run.go` wire the claudecode `Decide` path and its four flags; `internal/admission/projectconfig.go` validates `allowed_tools` at the config gate; `adapters/claudecode/settings.go` adds blob-based inventory of the admitted revision's project sources (the clone does not exist at admission time).
  - Auth status runs with the source checkout as cwd (the future workspace carries the same committed project settings). The AC-4.7 OAuth opt-in stays closed on both paths: admission hardcodes `oauthOptIn=false` and `security.BuildEnv` always denies `CLAUDE_CODE_OAUTH_TOKEN`; the slice has no trusted user-level config loader (Task 20 checkpoint).
  - A result frame before init is accepted only when it reports an error (startup failures may arrive without a session); success-without-init, assistant-before-init and duplicate init are protocol violations. Explicit JSON null means absent; disagreeing aliases and bad types fail the frame. Cost stays a literal-preserving decimal string, never `float64`. Unknown native error classes map to `native_error` and poison like auth classes. MCP selection walks to the nearest existing ancestor and rejects `--` rules.
  - Review-round and acceptance support files: `adapters/claudecode/probe.go` and `adapters/claudecode/probe_test.go` (the probe no longer returns `ErrUnavailable`; capabilities report supported for the owned launch); `internal/supervisor/ingest.go` and `internal/workers/spool.go` (the `attempt.native_error` evidence event); `internal/supervisor/state_test.go` (the stopping→blocked row); `adapters/claudecode/redact_test.go` (live-frame redaction pinned outside the live tag); `internal/supervisor/claudecode_test.go` (the fakeclaude run-level suite); and the co-changed tests `settings_linux_test.go`, `adapters/fake/fake_test.go`, `internal/admission/billing_test.go`, `internal/admission/projectconfig_test.go`, `internal/cli/exit_test.go`, `internal/supervisor/faults_test.go`, `internal/supervisor/pipeline_test.go`, `internal/workers/worker_test.go`.
- **Files modified**: `adapters/claudecode/launch.go`, `adapters/claudecode/decode.go`, `adapters/claudecode/decode_test.go`, `adapters/claudecode/launch_test.go`, `adapters/claudecode/testdata/streams/*.jsonl`, `adapters/claudecode/live_test.go`, `adapters/claudecode/redact_test.go`, `adapters/claudecode/probe.go`, `adapters/claudecode/probe_test.go`, `adapters/claudecode/settings.go`, `adapters/claudecode/settings_linux_test.go`, `internal/adapter/adapter.go`, `internal/workers/worker.go`, `internal/workers/worker_test.go`, `internal/supervisor/pipeline.go`, `internal/supervisor/state.go`, `internal/supervisor/state_test.go`, `internal/supervisor/ingest.go`, `internal/supervisor/receipt.go`, `internal/supervisor/claudecode_test.go`, `internal/supervisor/pipeline_test.go`, `internal/workers/spool.go`, `internal/admission/admission.go`, `internal/admission/native.go`, `internal/admission/billing.go`, `internal/admission/billing_test.go`, `internal/admission/projectconfig.go`, `internal/admission/projectconfig_test.go`, `internal/cli/run.go`, `internal/cli/exit.go`, `internal/cli/exit_test.go`, `adapters/fake/fake.go`, `adapters/fake/fake_test.go`, `specs/in-progress/dogfood-slice/tasks.md`, `specs/in-progress/dogfood-slice/handoff.md`, `specs/in-progress/dogfood-slice/scratchpad.md`, `specs/in-progress/dogfood-slice/design.md`.
- **Acceptance evidence**: All named checks passed; fakeclaude sidecar drives the real CLI/worker/decoder/pipeline end to end. Red-first: decoder and launch tests failed before implementation; the drain-discovered stop produced an illegal running→stopped transition until conclude recorded stop_requested. A live native is asserted dead by the ladder in the mismatch test (signals_sent non-empty). Review round: architect + code-reviewer needs-changes adjudicated — B1 stopping→blocked transition added with design §4 amendment; sticky poison, fail-closed decoder, launch-time re-verification, journaled native-error evidence, receipt honesty, allowedTools E2E and canonical MCP selection implemented with tests; OAuth stays doubly closed and transcript retention stays a Task 20 checkpoint; ADR 0002 §6 amendment deferred to Task 19 docs.

### Task 18 — Offline demo and read-only doctor

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 13, Task 16
- **Change**: `mythhelm demo` builds a temporary repository whose `mythhelm.toml` uses the binary's own `__demo-check` helper, runs the pipeline with a temporary state dir and the fake adapter, prints the receipt and diff, and labels everything `SCRIPTED DEMO`. `mythhelm doctor` reports the facts in AC-12.1 and writes nothing.
- **Files**:
  - `internal/cli/demo.go`
  - `internal/cli/doctor.go`
  - `internal/cli/demo_test.go`
  - `internal/cli/doctor_test.go`
  - `cmd/mythhelm/main.go` (register `__demo-check`)
- **Acceptance**:
  - `TestDemoOfflineNoCredentials`: runs with `HTTP(S)_PROXY` pointed at a closed port and an empty `PATH` apart from git; exit 0.
  - `TestDemoCheckFailsScenarioExit5`.
  - `TestDemoLabelsEveryScreen`.
  - `TestDoctorIsReadOnly`: the home, state and repo fixture hashes are unchanged; a nonexistent state dir is not created.
  - `TestDoctorNeverPrintsCredentialValues`.
  - `TestDoctorDoesNotRunAuthStatus`: it may contact the network, so doctor reports `run admission to query`.
- **Test plan**: packaged-binary style, calling `cli.Main` with a temp env.
- **Invariants touched**: G01, I13, A30, §5.1.

### Task 19 — Packaged-binary end-to-end suite and README status

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 14, Task 15, Task 17, Task 18
- **Change**: Add `tests/e2e`, which builds `cmd/mythhelm` once and exercises it against the real binary (§22.2). Update the README status with the supported and not-supported lists.
- **Files**:
  - `tests/e2e/main_test.go`
  - `tests/e2e/run_test.go`
  - `tests/e2e/demo_test.go`
  - `README.md`
- **Acceptance**:
  - `TestRunLeavesSourceCheckoutUntouched`: the fake adapter runs through `run`, `review` and a failed `apply`; the fingerprint is identical throughout (AC-3.4).
  - `TestE2EDemoAllOS`.
  - `TestE2EJSONLContract`: stdout lines validate against the envelope and stderr is not empty.
  - `TestE2EPathsWithSpacesAndUnicode`.
  - `TestE2EKillCLIWorkerSurvivesThenRecover` (Linux, macOS).
  - All pass on the five-runner CI matrix. Windows skips only the Unix-tagged tests, each with a reason.
- **Test plan**: `TestMain` runs `go build -o $TMP/mythhelm[.exe]`.
- **Invariants touched**: G01, G03, G04, G06, I08.

### Task 20 — Dogfood: run MYTHHELM on MYTHHELM and record the evidence

- **Domain/agent**: maintainer (human), assisted by go-implementer for the fixture and compatibility files
- **Budget**: standard
- **Depends on**: Task 19
- **Change**: The maintainer runs `MYTHHELM_LIVE_CLAUDE=1 go test -tags live ./adapters/claudecode -run TestLiveCanary`. They then run one real, small MYTHHELM task in their MYTHHELM checkout, for example a follow-up issue from this spec: `mythhelm run --task-file task.md --adapter claudecode --billing subscription-declared --execution-profile trusted-host`. Next they review it, apply it with `mythhelm apply <run> --to-branch dogfood/<topic>`, and sign off under the DCO. Finally, commit the sanitised evidence.
- **Files**:
  - `docs/dogfood/0001-first-run.md` (command, outcome, receipt excerpt with home paths, session IDs and org hash replaced by placeholders, and friction found)
  - `adapters/claudecode/testdata/streams/recorded-<version>.jsonl` (sanitised: prompt, assistant and tool text replaced; IDs placeholdered)
  - `adapters/claudecode/COMPATIBILITY.md` (§9.8/§9.14 record: version, OS, surface, status `fixture-tested`, plus one `live-canary` line with date, and limitations)
- **Acceptance**:
  - The live canary passes once.
  - The dogfood run reaches `ready_for_review` (exit 0), or its failure is written up with its exit code and receipt.
  - `apply` creates the branch, and the user checkout is otherwise unchanged (`git status`, `git stash list` and the reflog are unchanged apart from the new branch).
  - `TestDecodeFixture/recorded` passes on the recorded stream.
  - `gitleaks` (CodeRabbit) and a manual `grep -rE "$HOME|@|sk-ant" docs/dogfood adapters/claudecode/testdata` find nothing.
  - Friction points are filed as GitHub issues and linked from the doc.
- **Test plan**: n/a (human run); the recorded fixture joins CI through the Task 17 tests.
- **Invariants touched**: I08, I19 (sanitisation), I14 (first versioned live evidence), G02 (partial: fixture and canary only, no direct-native comparison).

---

## Open Questions

Open questions are tracked in `scratchpad.md` §Open questions. Q1 and Q2 need a maintainer decision before Task 16 and Task 20 respectively.
