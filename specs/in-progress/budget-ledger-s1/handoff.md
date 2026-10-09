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

- **Produces**: `internal/billing/normalize.go`: `Normalize(rs, expected, applied)` and `SplitUsage(route, u)`, as in design §3. `SplitUsage` returns a total (unknown unless all four token counts are reported) and components `output`, `cache_read`, `cache_creation`, `reasoning`; `reasoning` is always nil.
- **For dependents**: Task 3 passes `adm.Adapter.Harness` as the route; the only mapped key is `claude-code`. A group whose deltas were all already applied, with no baseline, comes back unknown, so build `applied` and the readings consistently. Identity-less deltas are summed but label the total `estimated`. An unknown scope is keyed `{scope, "", ""}` with a nil total and label `unknown`.
- **Traps**: `Normalize` and `Normalized` trip the UK-locale misspell linter; the files carry a file-level `//nolint:misspell`. Use `new(expr)` rather than pointer helpers (modernize).

## Task 3 — Migration 0003, ledger rows, ingest projection

- **Produces**: migration `0003_ledger.sql`, `journal.SchemaVersion` 3, and `internal/journal/ledger.go` with `UsageRow`, `EnvelopeRow`, `ReservationRow`, `BucketRow`, `Transact` and every accessor in the task, exactly as in design §2. `ingest.go` projects `attempt.native_result` into usage rows.
- **For dependents**: usage rows are per-event increments (one row per scope, unit, source per native_result), never running totals; sum them with `billing.Normalize` over `Delta` readings. `ReservationRow.Status` admits only held/released/orphaned. Release accepts held or orphaned; touch and orphan accept only held, and a refused transition wraps `sql.ErrNoRows`. `RunEnvelope`, `Reservation` and `BucketState` also wrap `sql.ErrNoRows` when missing. `SetReservationReleased` records its time in `heartbeat_at`.
- **Traps**: the route key for `SplitUsage` is read from the run's `admission.decided` event payload (`adapter.harness`); a run without one has an unmapped route. The cost literal must be plain decimal text to project, else its scope goes unknown. Task 7 and Task 8 share `ingest.go`: add cases to `projectWorkerEvent`'s switch, do not restructure `projectUsage`. Run tests with `ANTHROPIC_BASE_URL` unset and `GOTOOLCHAIN=go1.27.1` for `golangci-lint`; `Normalize` calls need `//nolint:misspell`.

## Task 4 — Admission reservation coupling

- **Produces**: `admission.QuotaBucket`, `Reserver` (`Reserve(ctx, tx, runID, bucket, owner)`), `NewJournalReserver(c)`, `HoldQuotaReservation(ctx, tx, r, runID, rec, identityRef)`, `ErrDuplicateHold`, `ReservationNote` and `ReservationText(bucket)` in `internal/admission/reserve.go`; `appendAdmission` writes the resolved envelope and holds the reservation inside the `admission.decided` append, so all three commit or roll back together.
- **For dependents**: the envelope row is written in the `admission.decided` transaction, so Tasks 7 and 9 can read it with `RunEnvelope` from the first attempt; `FirstStartAt` is empty until Task 7 sets it. The reservation row has scope = bucket, owner = run id, quantity `unknown`, status `held`; Task 8 releases or orphans it by id, found with a query on `run_id` and status. A failed hold rolls the admission back (no `admission.decided`, no envelope row) and the run is blocked with reason `quota_reservation_failed` from the `admission` state. Under `--no-checks` the file layer is empty (Task 6), so those runs resolve flags over the built-ins.
- **Traps**: the bucket is a JSON array of harness, surface, entitlement class and identity ref; the fake adapter's identity is `unknown`. A live-hold test needs a `runs` row first (foreign key). The reservation notice is printed once per run, so any exact-output golden must expect it (Task 10's `TestCliUsageNotices` says "exactly" the four usage lines: allow for this one).

## Task 5 — Exhaustion signal taxonomy and fake scenarios

- **Produces**: error class `allowance_exhausted` in `adapters/claudecode/decode.go` (`errorClasses`), emitted as `adapter.NativeError{Class: "allowance_exhausted"}`; fake scenarios `usage-counters` and `allowance-exhausted`; synthetic fixture `adapters/claudecode/testdata/exhaustion/allowance_exhausted.json`.
- **For dependents**: only the exact class name maps to exhaustion; `NativeError` has no reset field, so reset is always unknown from the decoder (a later task adding a reset needs `internal/adapter`, outside this task's Files). The fake decoder ignores extra frame fields: `usage-counters` carries per-turn `usage` deltas on `fake.progress` (turn 2 omits it) and `allowance-exhausted` carries `error.class` on its `fake.result`, but `adapters/fake/fake.go` does not surface either yet. The task that consumes them must extend that decoder.

## Task 6 — Envelope configuration and extension decision

- **Produces**: as designed: `admission.EnvelopesConfig` with `ToCeilings()`, `ProjectConfig.Envelopes`, `Request.EnvelopeFlags` and `Decision.EnvelopeFlags`, the four `--envelope-*` flags, and `supervisor.RequestExtension`, `CheckExtension` and the `EventExtensionGranted` constant in `internal/supervisor/envelope.go`. Added: `supervisor.ExtensionGrant` and `Hooks.Extension` (in `pipeline.go`), which `RecoverWithHooks` acts on.
- **For dependents**: both `ToCeilings` and the flag layer mark an unset field negative and return nil when nothing is set, so pass them straight to `billing.ResolveCeilings`. `RequestExtension` accepts only a run blocked with `envelope_deadline_exceeded` (execution), `envelope_repairs_exhausted`, `envelope_replans_exhausted` or `envelope_transport_retries_exhausted`; Task 7's gate must block with exactly those reasons. `CheckExtension` is true only for a grant after the latest block for that kind, so `GateLaunch` can call it after the run resumes. `Recover` leaves the run `blocked`: moving it on is the launch gate's job.
- **Traps**: under `--no-checks` admission never reads `mythhelm.toml`, so `Decision.ProjectConfig.Envelopes` is empty and the file layer is nil. `internal/cli/run.go` has a local `billing` variable, so it imports the package as `billingpkg`. No CLI flag feeds `Hooks.Extension` yet; wiring one into `mythhelm recover` is not in any task's Files.

## Task 7 — Envelope counting and launch gate

- **Produces**: `supervisor.GateLaunch(ctx, j, runID, c)` (check only) returning `*GateError{Reason, Err}` (wraps `billing.ErrBudgetExhausted`), `supervisor.IsReplan`, `GateError`, and the unexported `planLaunchAt` / `launchPlan.commit` (check, then count in the intent transaction), `gateLaunchAt`, `effectiveCeilings`, `recordLaunchIntent` (in `state.go`) and `pipeline.blockLaunch` / `endStop`. Ingest projects `attempt.progress` into `run_envelopes.transport_retries_seen`.
- **For dependents**: Task 9's reserve hook calls `IsReplan`, then `GateLaunch` with `p.ceilings` (set in `appendAdmission`); calling it more than once is harmless because it counts nothing. The pipeline counts the launch (repair or replan) and starts the clock inside the launch intent's transaction, through `plan.commit`, with a conditional `UPDATE` that enforces the ceiling, after re-checking the plan's deadline against the clock at commit; a refusal there (ceiling taken by another launch, or deadline passed since the plan) is a `*GateError` and blocks the run. A refusal that is not a `*GateError` blocks with `envelope_unavailable`. Task 8 can reuse `endStop`'s pattern for stops the pipeline requests. `p.deadline` is zero until the first launch; Task 9's derived replan deadline can overwrite it before `watch` starts.
- **Traps**: the worker's `attempt.progress.retries` is cumulative per attempt, so the projection takes each attempt's maximum, never a sum, and a `retries` that is not a non-negative integer fails ingestion as `ErrCorruptSpool`. `run_envelopes` stores seconds, so a sub-second ceiling reads as 0 from the row. The pipeline runs a single attempt, so the repair and replan counters first matter when a later task adds a second attempt.
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
