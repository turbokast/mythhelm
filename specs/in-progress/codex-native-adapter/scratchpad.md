# Codex Native Adapter — Scratchpad

> Open questions and research from authoring. Each implementing task reads this file when it starts and adds an entry under Discoveries when it finishes.

## Open questions

| # | Question | Default in this spec | Owner / blocks |
|---|---|---|---|
| OQ1 | Primary structured Codex surface (app-server vs exec) and fixture baseline version | App-server primary; synthetic fixture version; `exec` gets no code (D5) | Maintainer; blocks the real argv constant and frame schema (Tasks 4, 5) |
| OQ2 | Can Codex prevent paid continuation in an included plan; auxiliary coverage | Strict admission blocks (Task 8) | Maintainer with the security reviewer; blocks any `live-qualified` record |
| OQ3 | Live-test allowance grant for Codex | None; record stays at most `fixture-tested` | Maintainer (allowance grant) |
| OQ4 | Order against `claude-strict-subscription` and `contained-execution-profiles` | Resolved 2026-10-10 by the maintainer (Q-29): no wait on MH-12 Task 5 | Maintainer (delivery order) |
| OQ5 | Codex first vs second route | Evidence-only, no comparison task | Maintainer |
| OQ6 | Startup config sources, managed policy and credential locations | Enumerated sources only; every other source is an unresolved gap that blocks strict (D11) | Maintainer, from vendor documentation; blocks Task 6 paths |
| OQ7 | Shipped progress of the Codex row | `planned`; no seed change (D4) | Maintainer |
| OQ8 | Declared (unverified) posture for Codex | Strict only; declared is refused (D3) | Maintainer (decision record) |
| OQ9 | Native interrupt kind and single input writer | Signal ladder; acknowledgement `unknown`; worker stdin is the sole writer (D8, D10) | Maintainer, from vendor documentation |
| OQ10 | Usage frame and error/exhaustion shapes | Usage nil; every error `native_error`; exhaustion table empty (D6, D7) | Maintainer, from vendor documentation; blocks Task 4's real mappings |

## Research notes

- Validation round 1 findings (20, 8 blocking) were applied in design D12 to D17 and tasks 1 to 12; the main structural consequence is that no Codex run is admittable in this spec (D13), so admission admit-legs are tested at key level.

- `command -v codex` found no executable in the authoring environment; no Codex fact was checked against a live build.
- Claude analogues to read before each adapter task: `adapters/claudecode/probe.go`, `launch.go`, `decode.go`, `effective.go`, `qualify.go`.
- `internal/admission/admission.go` `decideClaudeCode` is the ordered template for `decideCodex`.

## Discoveries

- Task 1: `internal/adapter` had no tests before; its new `adapter_test.go` is external (`adapter_test`) and `fakeProc` there is the pattern for `OwnedProc` fakes. `TestStrictMainBlocksWriteNothing` (`internal/cli/run_drift_test.go`) depends on the real home's user-level native config and fails locally with `untrusted_native_config` where that config declares hooks; it also fails on unmodified `origin/main` code.
- Task 1 (mechanism): `TestStrictMainBlocksWriteNothing` fails with the real HOME (`exit 3 reason "untrusted_native_config"`) and passes in 0.58s with an empty temporary HOME, on origin/main code as well. A `Tri` field added with `omitempty` serialises unset, so tests that walk `Capabilities` by reflection must tolerate an empty `omitempty` entry. Task 2 (#353) now isolates HOME in `strictRun`, so the test no longer depends on it.
- Task 2: the admission table is rebuilt by `defaultDeciders()` on each call, so tests pass their own table to `decideWith` and nothing mutates package state. `validate` defaults `Scenario` to `happy` for any row with the scenario flag (only fake). A native-inventory test in `internal/cli` that does not set `HOME` reads the developer's real `~/.claude` hooks (`TestStrictMainBlocksWriteNothing` failed on unmodified origin/main for that reason).
- Task 3: the Codex probe holds `boundedOutput.buf` by name because `bytes.Buffer` embedded in a limiting writer promotes `ReadFrom`, so `io.Copy` skips the limit; `adapters/claudecode/probe.go` embeds it, which leaves its 1 MiB probe bound unenforced (not changed in this task; a fix needs a test that drives 1 MiB plus one byte through `runProbe`). The version banner `codex-cli X.Y.Z` and the fixture version `0.0.0` are placeholders held at the OQ1 default.
- Task 6: the Codex config inventory digests raw bytes per enumerated source and parses config only for hooks, MCP servers and credential keys. Its paths and JSON format are OQ6 placeholders (`.codex/config.json`), so a real TOML config needs a parser change in `config.go` and new fixtures, not a seam change.
- Task 7: `AuthCategory` has no defined non-secret status source in the design or requirements, so it returns `unknown` unconditionally; the first task that needs `native-managed` must define the source (OQ1/OQ6/OQ10). In the cloud sandbox `go test ./...` also fails four `internal/supervisor` restricted/inspect tests (`native_exit_1`, two reproduced on the unmodified Task 3 head) and, while `ANTHROPIC_BASE_URL` is set, the Claude e2e tests (`credential_route_override`); `golangci-lint` there is built with Go 1.25 and cannot load the Go 1.27.1 config.
- Task 9: a worker stops an attempt on a `NativeError` of class `protocol_mismatch` (`adapter.ProtocolMismatch`, v2 §4.5); only `codex.Decode` sends it, on a malformed `result` or a read error, never on a stream that merely ends without a result. The attempt ends `stopped/protocol_mismatch` and the run `failed/protocol_error`, because a recorded stop can end only `stopped` or `interrupted` (AC-5.7). Claude's `protocol_error` still does not stop the worker (#389). The Codex worker re-verification re-inventories mutable user sources, so a config source added after admission fails the launch; project sources are blob-admitted and skipped.
