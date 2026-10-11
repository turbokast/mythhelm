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

<!-- pending -->

## Task 5 — Codex adapter: prepare, launch session, stop ladder, single writer

<!-- pending -->

## Task 6 — Codex adapter: effective configuration inventory, trust inputs and gaps

<!-- pending -->

## Task 7 — Codex adapter: fixture qualification drafts and route inventory

- **Produces**: `codex.ObservedKey`, `codex.AuthCategory`, `codex.FixtureDraft`, `codex.FixtureEvidence{Fidelity, Lifecycle, Entitlement qualify.Verdict}`, `codex.Route{Name, Coverage}`, `codex.RouteInventory`, `codex.CheckRoute`, `codex.ErrNotCodexKey`, `codex.ErrFixtureCannotProve`, `codex.ErrUnknownRoute`. `FixtureEvidence` is the one type the design left unspecified.
- **For Task 8**: `AuthCategory(env)` always returns `"unknown"`, so `decideCodex` blocks on D13 until a status source exists; supply a native-managed category through `decideCodexWith`. A key with `AuthCategory` `native-managed` and one with `unknown` hash differently under `qualify.KeyHash`.
- **For Task 8**: `ObservedKey` sets `ProviderEndpoint`, `ModelSnapshot`, `EffortSettings` and `EntitlementClass` to `unknown`; a `FixtureDraft` key must come from `ObservedKey` (or equal one) to match a registry lookup.
- **For Task 8 (`purchasedCreditPreventable`)**: the draft's entitlement evidence has `Result == "purchased_credit:unknown"`, never `preventable`, so it cannot satisfy AC-4.3.
- Route names: `title`, `summary-compaction`, `review`, `child`, `native-observer`, each `coverage: unknown`.

## Task 8 — Admission: ResolveQualificationKey, neutral trust and the Codex decider

<!-- pending -->

## Task 9 — Register Codex in workers, supervisor and runtime route check

<!-- pending -->

## Task 10 — Doctor: failing columns and next test in plain output

<!-- pending -->

## Task 11 — End-to-end: Codex stays blocked and refused

<!-- pending -->

## Task 12 — Document the Codex limits and record the decisions

<!-- pending -->
