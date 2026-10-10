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
