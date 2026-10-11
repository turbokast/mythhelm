# codex-native-adapter — Hand-off

> One section per task. Each task replaces its own `<!-- pending -->` line in its pull request with what
> dependent tasks need: what it produced (the API and files as shipped), what a later task must
> know, and any deviation that changes a later task's inputs. `/run-spec` puts the sections of a
> task's dependencies into that task's dispatch prompt. Open questions and research stay in
> `scratchpad.md`.

## Task 1 — Seam additions: Acknowledged, Reconnect, ModelMetadata

- **Produces**: `adapter.InterruptReport.Acknowledged string` (`json:"acknowledged,omitempty"`), `adapter.Capabilities.Reconnect` and `.ModelMetadata` (`Tri`, `json:"...,omitempty"`), all in `internal/adapter/adapter.go`. No design difference.
- **For dependents**: an unset `Tri` is the empty string, not `Unknown`; Codex must set `Reconnect` and `ModelMetadata` to `adapter.Unknown` explicitly (D9) and set `Acknowledged` to `"unknown"` after `ClimbLadder` (D8). `ClimbLadder` leaves `Acknowledged` empty.
- `internal/adapter/adapter_test.go` now holds a `fakeProc` (`OwnedProc` that goes away on a chosen signal) in package `adapter_test`; copy it rather than importing it.
- `go test ./internal/cli` fails `TestStrictMainBlocksWriteNothing` on a machine whose real home has user-level hooks or MCP servers (`untrusted_native_config`), identically on `origin/main`; rerun with a clean home before suspecting your change.

## Task 2 — Per-harness decider table, adapter validation and containment refusal

- **Produces**: `admission.AdapterCodex = "codex"`; unexported `harnessDecider{name, adapterID, destination, decide, flags}`, `flagSet`, `defaultDeciders()`, `decideWith(ctx, table, req)`, `validateWith(table, req)`; `internal/admission/export_test.go` exposes `DecideWithHarness` and `Validate`.
- **For Task 8**: replace the `decideCodex` stub in `internal/admission/admission.go` (it returns `adapter_not_available`, `Capability: true`) and swap the local `codexAdapterID = "builtin/codex"` for `codex.AdapterID`, pinning equality with `TestCodexAdapterIDMatches`. The codex row's flags already allow `--trust-native-config` and `--allow-untested-native-version` and refuse `--declare-entitlement` and `--strip-credential-env` in `validate`, the single refusal site.
- **For Task 8**: `consentProfile` runs before the table dispatch and derives the route as `"builtin/"+req.Adapter`, so restricted/inspect with codex are refused (exit 7, `missing coverage: filesystem, process, network, credential`) before `decideCodex` runs; `SeedV1` has no Codex key and `TestRestrictedAndInspectRefuseCodex` fails if one is added.
- **For Task 11**: `tests/e2e/contain_refusal_test.go` already carries the codex refusal cases on every OS; `--adapter codex` with `trusted-host` currently exits 7 with `adapter_not_available`.
- **Trap**: `internal/cli.TestStrictMainBlocksWriteNothing` read the developer's real `~/.claude/settings.json` hooks until `strictRun` set a temp `HOME`; a native-inventory test must isolate `HOME`.

## Task 3 — Codex adapter: probe, compatibility, capabilities

- **Produces**: package `adapters/codex`: `AdapterID = "builtin/codex"`, `New() adapter.Adapter` (unexported `*codexAdapter{probeTimeout}`), `ErrCapability`, `Descriptor` (`Version "1"`, `Harness "codex"`, `Surface "native-app-server (structured)"`), `Probe`, `Capabilities`. Unexported: `descriptor()`, `compatibility(version, allowUntested)`, `fixtureVersion = "0.0.0"`, `maxProbeOutput = 4096`, `fileSHA256`, `runProbe`. No design difference.
- **For Task 4 and 6/7**: `fixtureVersion` is the synthetic `0.0.0`; the version banner `codex-cli X.Y.Z` is a placeholder for OQ1. `Capabilities` sets every native fact (including `Reconnect`, `ModelMetadata`) to `unknown`; `Platform` is `supported` only on linux and darwin. Qualification reads `<compatibility>; planned`.
- **For Task 5**: delete `adapters/codex/stub.go` (it defines `ErrNotImplemented` and the `Prepare`/`Start` stubs on `*codexAdapter`). `codexAdapter` has only `probeTimeout`; add the one-`Start` guard field there. Re-hash with `fileSHA256(p.Executable)` for `native_binary_changed`.
- **Trap (os/exec guard)**: `adapters/fake/fake_test.go` `TestAdaptersDoNotImportOsExec` allows `os/exec` only in `claudecode/probe.go` and `codex/probe.go`; `launch.go` and `decode.go` must not import it.
- **Trap (bounded writer)**: do not embed `bytes.Buffer` in a limiting `io.Writer`: the promoted `ReadFrom` bypasses `Write`, so `io.Copy` (used by `os/exec`) never enforces the limit. `adapters/claudecode/probe.go` `boundedOutput` has this defect (its 1 MiB bound is unenforced); not changed here.
- Tests: `TestMain` in `probe_test.go` re-executes the test binary as the fake native when `GO_WANT_FAKECODEX=1` (env `FAKE_OUT`, `FAKE_EXIT`, `FAKE_SLEEP`, `FAKE_LOG`); reuse `fakeProbeInput` for Prepare tests.

## Task 4 — Codex adapter: stream decoder, usage and error mapping

- **Produces**: `codex.Decode(r io.Reader, send func(adapter.Observation)) adapter.NativeExit` and unexported `decodeWith(table map[string]string, lim ndjson.Limits, r, send)`. It sends `ProtocolCounters` last, always, and sets `NativeExit.Err` when no `result` frame arrived or the reader failed. No design difference.
- **For Task 5**: `Decode` never sets `Code` or `Signal`; the session merges the process exit with its `Err`. Call it from the stdout goroutine only, and do not import `os/exec` in `launch.go` or `decode.go`.
- **For Task 5**: `SessionStarted` carries only `SessionID` (empty when unreported); the worker's `billing_route_mismatch` check must stay skipped for `builtin/codex` (D12).
- **For later tasks**: the error table is empty by default (D7); a cited OQ10 source fills it in one place, and `rate_limit` and `allowance_exhausted` are the only non-default classes it may name.
- Fixtures are `testdata/streams/*.jsonl`, every frame marked `_synthetic`; the frame schema is a placeholder for OQ1/OQ10.
- Trap: in this sandbox `ANTHROPIC_BASE_URL` is set and fails `internal/cli` tests with `credential_route_override`; unset it. Four `internal/supervisor` restricted-boundary tests also fail identically on the base here.

## Task 5 — Codex adapter: prepare, launch session, stop ladder, single writer

- **Produces**: `codex.Argv(bin string) []string` (placeholder `{bin, "app-server"}`, D5/OQ1), `Prepare` and `Start` on `*codexAdapter` in `adapters/codex/launch.go`; `stub.go` and `ErrNotImplemented` are gone. No design difference.
- **For dependents**: `Prepare` returns `*BlockedError` codes `native_binary_changed`, `native_tool_rules_unsupported`, `native_passthrough_unsupported`; `Start` returns `input_writer_conflict` on a second call on the same adapter value. Ladder is interrupt 5s, terminate 10s, kill 5s; `Interrupt` sets `Acknowledged:"unknown"`.
- `Prepare` leaves `Billing` and `Manifest` zero for admission (Task 2). Fidelity deltas use `Kind:"fidelity"`, names `instructions skills hooks mcp children context`, value `unobservable`.
- `codexAdapter` has test-only `goos` (empty means host) and an atomic `started` guard; tests build `&codexAdapter{goos: "windows"}`.
- Trap: in a container whose PID 1 reaps slowly, `kill(-pgid,0)` still succeeds on an orphaned zombie, so a real-process ladder test confirms only at the kill grace; keep that grace generous.

## Task 6 — Codex adapter: effective configuration inventory, trust inputs and gaps

- **Produces**: `codex.Manifest{Digests, Digest, Hooks, MCPServers, RequiresTrust}` (JSON tags `digests`, `digest`, `hooks`, `mcp_servers`, `requires_trust`), `InventoryAdmittedProject`, `InventorySettingsForEnv`, `AdmittedConfigPathsForEnv`, `UnresolvedSources` (`managed-policy`, `os-managed-preferences`), `GapSources` (returns the input unchanged, in order, on every platform), `ProjectSources` (`project_instructions` -> `AGENTS.md`, `project_config` -> `.codex/config.json`, `project_config_toml` -> `.codex/config.toml`; a present TOML file, project or user, blocks with `native_config_unreadable` until a parser exists, and has no digest), `ErrNonRegularSource`. No design difference.
- **For Task 8/9**: manifest keys are `user_config`, `project_instructions`, `project_config`. `MCPServers` entries are `<source>:<name>`. `InventoryAdmittedProject` treats `blobs` as authoritative for project sources (a nil map means none committed, never a disk read); `readAdmittedBlobs` should be parameterised by the `ProjectSources` keys. `ErrNonRegularSource` and relative/unreadable-home `ErrCapability` come back wrapped; malformed config and credential keys are `*adapter.BlockedError` (`native_config_unreadable`, `native_settings_credential_override`).
- **For Task 8**: `RequiresTrust` is true only when hooks or MCP servers exist; an instruction file or plain config alone does not require trust. `Manifest` converts field for field to `admission.NativeConfig` (D17).
- **Trap**: the paths and JSON schema are placeholders until OQ6 is answered; real Codex config is likely TOML. Only top-level duplicate keys are rejected, and credential keys are checked at the top level only.
- **Trap (environment)**: in the cloud container `ANTHROPIC_BASE_URL` is set and `bwrap` is missing, so `internal/supervisor` tests fail (credential_route_override, native_exit_1) identically on the base branch.

## Task 7 — Codex adapter: fixture qualification drafts and route inventory

- **Produces**: `codex.ObservedKey`, `codex.AuthCategory`, `codex.FixtureDraft`, `codex.FixtureEvidence{Fidelity, Lifecycle, Entitlement qualify.Verdict}`, `codex.Route{Name, Coverage}`, `codex.RouteInventory`, `codex.CheckRoute`, `codex.ErrNotCodexKey`, `codex.ErrFixtureCannotProve`, `codex.ErrUnknownRoute`. `FixtureEvidence` is the one type the design left unspecified.
- **For Task 8**: `AuthCategory(env)` always returns `"unknown"`, so `decideCodex` blocks on D13 until a status source exists; supply a native-managed category through `decideCodexWith`. A key with `AuthCategory` `native-managed` and one with `unknown` hash differently under `qualify.KeyHash`.
- **For Task 8**: `ObservedKey` sets `ProviderEndpoint`, `ModelSnapshot`, `EffortSettings` and `EntitlementClass` to `unknown`; a `FixtureDraft` key must come from `ObservedKey` (or equal one) to match a registry lookup.
- **For Task 8 (`purchasedCreditPreventable`)**: the draft's entitlement evidence has `Result == "purchased_credit:unknown"`, never `preventable`, so it cannot satisfy AC-4.3.
- `FixtureDraft` returns `Digest` unset: pass it to `Registry.Record`, which assigns the revision and computes the digest.
- Route names: `title`, `summary-compaction`, `review`, `child`, `native-observer`, each `coverage: unknown`.

## Task 8 — Admission: ResolveQualificationKey, neutral trust and the Codex decider

- **Produces**: `admission.ResolveQualificationKey`, `ResolveBillingStrict`, `NativeConfig`, `CheckNativeTrustConfig` as specified; `decideCodex` implements design §4. Unexported `decideCodexWith`, `purchasedCreditPreventable`, `eligibilityBlock`, `claudeNativeConfig`, `admitCodexNativeConfig`, `envHome`; `export_test.go` exposes `DecideCodexWith` and `PurchasedCreditPreventable`. No design difference.
- **For Task 9**: no Codex run is admittable (D13), so the worker and pipeline paths cannot be driven through `Decide`; the admitted proposal's `Manifest.Digests` keys are `user_config`, `project_instructions`, `project_config`, and `admission.Record` carries `AllowedTools` from the Claude config only.
- **For #389 F2**: the inventory runs over the `security.BuildEnv` result and takes `HOME`/`USERPROFILE` from it. `CODEX_HOME` is not allowlisted, so a user's value is dropped without a ConfigDelta; the maintainers' decision (keep it for the child, or record a delta) is still open and would change `internal/security/env.go` and `codex.Prepare`.
- **Trap**: `OpenQualificationRegistry` returns a registry for an existing state dir without a database, so the gap gate fires; use a non-existent dir (`missingStateDir`) to test the no-registry leg. The fake native is the test binary copied as `codex` on `PATH`; it logs each invocation to `invocations.log` beside itself.

## Task 9 — Register Codex in workers, supervisor and runtime route check

- **Produces**: `builtin/codex` in the worker adapter map (`internal/workers/worker.go`); `adapter.ProtocolMismatch = "protocol_mismatch"` (`internal/adapter/adapter.go`); unexported `admittedConfigPaths` (pipeline), `Launch.blobSource` and `Launch.verifyCodexInventory` (`internal/workers/contain.go`). Design difference: the `protocol_mismatch` seam (#389 F1) is not in the design.
- **For Task 11**: a Codex attempt whose stream stops early (malformed `result` or a read error) ends `stopped/protocol_mismatch` with `requested_by` `worker`, and the run ends `failed/protocol_error` (exit 4), never `cancelled`; a stop already running (a user stop, including `mythhelm stop` and the TUI, or a route-violation stop) keeps its requester and an empty or its own reason, so a user stop still ends `cancelled`. `Decode` sends the class once, before `ProtocolCounters`. A stream that merely ends without a result still ends as before (`result_unobserved` or the native exit).
- **For Task 11**: the e2e fixture native for the worker is the test binary re-executed (`codexFixtureCommand` in `internal/workers/worker_test.go`, modes `success` and `malformed-result`); no Codex run is admittable through `Decide` (D13), so a worker or pipeline test builds a `workers.Launch` or `admission.Decision` directly.
- **For dependents**: the drained-line count after an early stop is in the exit error text (`N lines (M bytes) discarded after the stream stopped`), logged by the worker as `native exit not fully observed`; it is not a journal event. The Claude adapter is unchanged: its `protocol_error` observation does not stop the worker and it still stalls on a full pipe (#389).
- **Trap**: the worker re-inventories Codex config over `launch.Env` HOME and `launch.Dir`; a config source that appears between admission and exec (for example `~/.codex/config.toml`) fails the launch with `native_config_unreadable`, and project sources are skipped (blob-admitted). The re-inventory runs for every Codex launch, including one admitted with no config: a source present at exec that admission did not hold fails the launch, and a Codex launch env must carry `HOME`.
## Task 10 — Doctor: failing columns and next test in plain output

- **Produces**: in `internal/cli/doctor.go`, plain qualification rows end with ` failing: <column>[, <column>]` (column order fidelity, entitlement, lifecycle; any verdict other than `proven`, including unknown, is failing) and the next line is `  next_test: <text>` when the record has one. Unexported `failingColumns`. JSONL is unchanged, pinned by `testdata/doctor_qualification.jsonl`. No design difference.
- **For Task 11**: a doctor e2e over a seeded registry sees the Codex row as `codex × unknown: planned (fidelity unknown, entitlement unknown, lifecycle unknown) [evidence 1 revs, latest none; drift clean] failing: fidelity, entitlement, lifecycle`, then the seeded `next_test` line; match the row on the `failing:` suffix, not the old `]$` ending.
- **For Task 12**: `docs/user-guide.md` Doctor section already describes the failing text and the next-test line; `README.md` and `docs/limitations.md` need no doctor change.
- Test helpers in `doctor_test.go`: `recordColumns(t, dir, harnesses, fidelity, entitlement, lifecycle)` records new revisions with chosen verdicts; `qualificationSection` returns rows plus next_test lines, `qualificationLines` only the rows.

## Task 11 — End-to-end: Codex stays blocked and refused

- **Produces**: `tests/e2e/codex_qualification_test.go` with `TestCodexBillingModesAllRefuseOrBlock`, `TestCodexDoctorRowEndToEnd` and the helpers `codexFixtureMain`, `installFakeCodex` (returns the PATH directory) and `fixtureInvocations`; `tests/e2e/main_test.go` `TestMain` dispatches a binary named `codex` or `codex.exe` to the fixture. No design difference.
- **For Task 12**: the doctor text the guide describes matches what the e2e pins: the Codex row ends ` failing: fidelity, entitlement, lifecycle` and the next line is `  next_test: Qualify native-managed sign-in, approvals and effort metadata; authority: maintainer live-test grant`.
- **Trap**: no Codex run is admittable through the binary (D13), so the e2e cannot cover the `protocol_mismatch` stop or the worker config re-inventory (#389 F1/F2); those stay in the Task 9 worker tests. A later task that makes a Codex run admittable must extend the fixture, which answers only `--version` and exits 9 on any other argv.

## Task 12 — Document the Codex limits and record the decisions

<!-- pending -->
