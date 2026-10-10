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

<!-- pending -->

## Task 4 — Codex adapter: stream decoder, usage and error mapping

<!-- pending -->

## Task 5 — Codex adapter: prepare, launch session, stop ladder, single writer

<!-- pending -->

## Task 6 — Codex adapter: effective configuration inventory, trust inputs and gaps

<!-- pending -->

## Task 7 — Codex adapter: fixture qualification drafts and route inventory

<!-- pending -->

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
