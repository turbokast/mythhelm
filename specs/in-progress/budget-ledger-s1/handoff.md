# budget-ledger-s1 — Hand-off

> One section per task. Each task replaces its own `<!-- pending -->` line in its pull request with what
> dependent tasks need: what it produced (the API and files as shipped), what a later task must
> know, and any deviation that changes a later task's inputs. `/run-spec` puts the sections of a
> task's dependencies into that task's dispatch prompt. Open questions and research stay in
> `scratchpad.md`.

## Task 1 — billing kernel: types, envelopes, codes

- **Produces**: `internal/billing/billing.go` (`Reading`, `ScopeTotal`, `ScopeKey`, `Normalized`, `EventID`, `ErrReadingShape`, `Code`, `CodeAllowanceExhausted`, `CodeBudgetExhausted`, `ErrAllowanceExhausted`, `ErrBudgetExhausted`) and `envelope.go` (`Ceilings`, `BuiltInCeilings`, `ResolveCeilings`, `PauseSpan`, `Deadline`, `AttemptCounts`, `Ceilings.Check`), as in design §§3, 5, 9. `Normalize` is Task 2's.
- **For dependents**: `Check` returns `ErrBudgetExhausted` wrapped with the kind text `repairs`, `replans` or `transport_retries`; it does not cover execution time (use `Deadline`). A ceiling of 0 is a set value, only negatives are unset.
- **Traps**: `Normalized` needs the `//nolint:misspell` directive (UK-locale linter). Locally, `golangci-lint` needs `GOTOOLCHAIN=go1.27.1`, and `ANTHROPIC_BASE_URL` must be unset or `TestClaudeUntrustedNativeConfigBlocks` fails.

## Task 2 — Counter normalization and component split

<!-- pending -->

## Task 3 — Migration 0003, ledger rows, ingest projection

<!-- pending -->

## Task 4 — Admission reservation coupling

<!-- pending -->

## Task 5 — Exhaustion signal taxonomy and fake scenarios

<!-- pending -->

## Task 6 — Envelope configuration and extension decision

<!-- pending -->

## Task 7 — Envelope counting and launch gate

<!-- pending -->

## Task 8 — Exhaustion path: preserve, block, schedule, never pay

<!-- pending -->

## Task 9 — Completion reserve gate

<!-- pending -->

## Task 10 — Receipt and CLI ledger surfaces

<!-- pending -->

## Task 11 — AT-13 end-to-end and NFR-1 run-path latency

<!-- pending -->

## Task 12 — Ledger user documentation and decision record

<!-- pending -->
