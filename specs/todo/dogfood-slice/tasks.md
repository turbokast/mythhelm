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
- **Implementation**: `cli.Main` dispatches through a `commands` map, and later tasks register their verbs there. `ParseInterspersed` separates flag tokens from positional arguments in one pass (`fs.Lookup` and `IsBoolFlag`), then calls `fs.Parse` once. `--` ends the flags, and a lone `-` is positional. Commit 9976886.
- **Spec deviations**: `ExitInternal = 1` is added next to the listed constants, because design §11 requires exit 1 for unexpected errors.
- **Files modified**: `go.mod`, `cmd/mythhelm/main.go`, `internal/cli/dispatch.go`, `internal/cli/exit.go`, `internal/cli/dispatch_test.go`, `internal/buildinfo/buildinfo.go`, `CONTRIBUTING.md`, `specs/todo/dogfood-slice/tasks.md`.

### Task 2 — CI quality gates: golangci-lint, govulncheck, CodeQL Go, Dependabot gomod

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

### Task 3 — CI supply chain and runner matrix

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

### Task 4 — State directory, IDs and the SQLite journal

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

### Task 5 — State machines, projections and `runs list`

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

### Task 6 — Security primitives

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

### Task 7 — Workspace: safe git runner, preflight and snapshot

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

### Task 8 — Adapter seam, NDJSON framing and the fake adapter

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

### Task 9 — Worker ownership: detached worker, process group, spool, stop ladder

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

### Task 10 — Supervisor run pipeline, first against the fake adapter

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

### Task 11 — Candidate freeze and validation flags

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

### Task 12 — Project config, trust grants and the check runner

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

### Task 13 — Receipt and `mythhelm review`

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

### Task 14 — Guarded apply

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

### Task 15 — Stop, recover, and the §18.4 fault-injection subset

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

### Task 16 — Claude Code probe, auth evidence and billing admission

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

### Task 17 — Claude Code launch, stream-json decoding and result mapping

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
  - `TestAuthFailedMapsToBlocked`.
  - `TestRateLimitNoInventedCountdown`.
  - `TestAssistantBeforeInitIsProtocolError`.
  - `TestCostStoredAsEstimateString`: decimal string, never `float64`, in the journal.
  - `go vet -tags live ./adapters/claudecode` compiles, and CI never passes `-tags live`.
- **Test plan**: fakeclaude replays each fixture through the real worker and launcher.
- **Invariants touched**: I01, I06, I07, I09, I15, §9.3, §9.7, §12.3.

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
