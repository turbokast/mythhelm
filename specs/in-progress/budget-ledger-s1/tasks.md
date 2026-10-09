## Budget Ledger S1 — Tasks

### Dependencies

- Prerequisite specs: `specs/*/qualification-registry/` (MH-10, shipped: `qualify.Datum`/`DatumLabel`). Nothing else must ship first. MH-21 is the plan `specs/*/v2-contracts-supervisor/plan.md` with stream specs in `in-progress/`: `specs/*/supervisor-service/` claims migration `0003_supervisor.sql` (`operations`, `reservations` tables) and `specs/*/supervisor-migration/` sequences 0004 after it. Migration-namespace coordination (three claimants on 0003: MH-21's `0003_supervisor.sql`, MH-13's `0003_evaluator.sql`, this spec's `0003_ledger.sql`): Task 3 re-checks the next free migration number at its start and renumbers if MH-21 or MH-13 landed first (filenames sort positionally, so a stale number silently becomes the wrong version); whoever lands second renumbers, mirroring MH-13's Task 7 protocol. `reservations`-table adoption: both this spec (§2) and MH-21 stream 2 define a `reservations` table and only one DDL may land — if MH-21's lands first, Task 3 first reconciles the contracts (the adopted table and `control.Reserve`/`Release` API must support this spec's `orphaned` recovery transition, design §4 / AC-3.4, and bucket-coupled reservation shape, AC-3.1 — MH-21 as specced uses `held`/`released`/`expired` with no `orphaned`), then drops its own `reservations` DDL and builds `reserve.go` against MH-21's table; if the landed table lacks `orphaned`, Task 3 extends it with an additive follow-up migration (new status value, no DDL rewrite) and records a spec deviation in both cases; if this spec lands first, MH-21 adopts this spec's table. Held: `specs/*/claude-strict-subscription/` Task 7 (live evidence; Task 5 here ships fixture-qualified taxonomy meanwhile — no file overlap: MH-12's open tasks touch only `tests/e2e/qualification_strict_test.go` and `adapters/claudecode/testdata/qualification/live-record.json`). Sequenced alongside, no dependency either way: `specs/*/contained-execution-profiles/` (MH-13 — PR #228, landing to `todo/` — owns trust profiles, not envelope knobs; neither spec reads the other's state; migration order per this paragraph — MH-13's `0005_ledger.sql` narrative is stale w.r.t. this spec's 0003 claim and its own `0003_evaluator.sql`, but its Task 7 takes the next free number with symmetric renumber, which covers either order. Shared files, disjoint regions unless noted: `internal/admission/admission.go` (MH-16 Task 6: Request/Decision envelope fields + Decide pass-through; MH-13: Decide defaults/consult, disclosure, `Proposal.Overrides` append), `internal/cli/run.go` (MH-16 Tasks 6/10: `--envelope-*` flags + finish notices; MH-13: help/consent/disclosure), `internal/supervisor/pipeline.go` (MH-16 Tasks 4/7/8/9: admission/attempt/conclude gates; MH-13: boundary/inspect enforcement and evaluator wiring — second landing rebases), `internal/supervisor/receipt.go` (disjoint members: MH-16 `billing`, MH-13 `execution_bundle.boundary`), `internal/workers/worker.go` (MH-16 Task 8: outcome mapping; MH-13: pre-exec re-hash), `internal/journal/journal.go` (both bump `SchemaVersion` — second landing rebases under the renumber rule)). Ordering with `specs/*/supervised-stop-recover/` (shared `internal/supervisor/pipeline.go` and `internal/workers/worker.go`): MH-16 S1 lands first — stop-recover cannot start before its own unshipped prerequisites (the supervisor-service stream) ship — and stop-recover rebases onto it (disjoint regions: MH-16 edits admission/attempt/conclude gates and outcome mapping; stop-recover edits the spawn site, nonce/ladder and reconnect). Downstream: `specs/*/protected-acceptance/` (MH-22, spec unwritten — slug proposed here) reads §11's contract after this spec ships; the one new control event `run.extension_granted` is a downstream obligation on its event allowlist once specified.
- Order is contract-first: Task 1 ships the `billing` types every later task builds against. Runnable sets: {2, 5} after 1 (disjoint Files); 3 after {1, 2} (projects via `Normalize`); 6 after {1, 3} (envelope config + extension; shares `envelope.go` with 7 and `envelope_test.go` with 7 and 9; sequential only); 4 after {1, 3, 6} (consumes Task 6's Decision envelope layers for the initial envelope write); 7 after {1, 3, 4, 6} (shares `ingest.go` with 3, `pipeline.go`/`pipeline_test.go` with 4, `envelope.go` with 6; sequential only, never in parallel); 8 after {3, 4, 5, 6, 7} (consumes Task 3's reservation/bucket writers; shares `ingest.go` with 3, `reserve.go` with 4, `recover.go` with 6, `pipeline.go`/`ingest.go` with 7; sequential only); 9 after {2, 7, 8} (shares `pipeline.go` with 7 and 8, `envelope_test.go` with 7; sequential only); 10 after {2, 3, 6, 9} (shares `run.go` with 6; sequential only); 11 after {4, 5, 8, 10} (consumes Task 5's scenarios); 12 after {7, 8, 10} (documents the finished mechanism). No other two tasks share a file.
- **Gates for every task.** Run `gofmt -w` on touched Go files before any check, then from the tree root `go vet ./...`, `go test -race ./...`, `go mod tidy -diff` (must print nothing), and `golangci-lint run`; OS-specific files add `GOOS=windows go vet ./...` and `GOOS=darwin go vet ./...`; every task also runs `scripts/ci/check-public-hygiene.sh`. Prefer `scripts/harness/gate.sh go`. A task is not complete because files exist or an agent reported success; cite the runner output.
- **Completion convention.** Append ` ✅ COMPLETED` to the heading, keep every original field, and add `Status` (`✅ Completed — …; PR #<n>`), `Implementation` (at most 3 lines, plus commit SHAs), `Spec deviations` (`None`, or each deviation with its reason) and `Files modified`.
- **Commits.** Every commit is signed off (`git commit -s`, DCO). Billing, persistence and process-ownership changes carry the Task 12 decision record; Tasks 3, 4, 6, 7, 8 name its slug (`budget-ledger-s1`) in their PR description.

---

## Implementation Tasks

### Task 1 — billing kernel: types, envelopes, codes ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: None
- **Change**: Create `internal/billing` with the observation/envelope types, ceiling resolution, deadline arithmetic and S1-local error codes every later task builds against, so the ledger has one typed vocabulary.
- **Files**:
  - `internal/billing/billing.go` (Reading, ScopeKey, ScopeTotal, Normalized, EventID, ErrReadingShape, Code constants, sentinels)
  - `internal/billing/envelope.go` (Ceilings, BuiltInCeilings, ResolveCeilings, PauseSpan, Deadline, AttemptCounts, Check)
  - `internal/billing/billing_test.go`
  - `internal/billing/envelope_test.go`
- **Produces**: `billing.Reading`, `billing.ScopeKey` (`{Scope, Unit, Source}`), `billing.ScopeTotal`, `billing.Normalized` (`Scopes` keyed by `ScopeKey`), `billing.EventID`, `billing.ErrReadingShape`, `billing.Code`, `billing.CodeAllowanceExhausted`, `billing.CodeBudgetExhausted`, `billing.ErrAllowanceExhausted`, `billing.ErrBudgetExhausted`, `billing.Ceilings`, `billing.BuiltInCeilings() Ceilings`, `billing.ResolveCeilings(flags, file *Ceilings) Ceilings`, `billing.PauseSpan`, `billing.Deadline(firstStart time.Time, c Ceilings, pauses []PauseSpan, now time.Time) (time.Time, bool)`, `billing.AttemptCounts`, `(Ceilings).Check(AttemptCounts) error`
- **Acceptance**:
  - `TestResolveCeilingsPrecedence`: flags > file > built-ins per field; a negative field means unset for that layer. Command: `go test ./internal/billing/ -run TestResolveCeilingsPrecedence`. Fails before: package absent.
  - `TestBuiltInCeilingsAreS1Values`: exactly 30m, 3 repairs, 2 replans, 5 transport retries; any other value fails. Fails before: package absent.
  - `TestDeadlineExcludesOnlyQuiescentPause`: a fully quiesced span extends the deadline by exactly its quiesced length; a span with zero quiesced length (still-running turn) extends it by zero. Fails before: package absent.
  - `TestCheckNamesExhaustedKind`: each over-ceiling count returns `ErrBudgetExhausted` via `errors.Is` with the kind in the message; at-ceiling passes. Fails before: package absent.
  - `TestCodesPinV245Strings`: codes equal exactly `allowance_exhausted` and `budget_exhausted` (stability pin for MH-21 adoption); any other string fails. Fails before: package absent.
- **Test plan**: Table tests, no I/O; durations asserted exactly (no sleeps).
- **Invariants touched**: I09 (v2 §2: nil/unknown stays distinct from zero in every type); I10 (v2 §2: no hard-limit advertising in this package — types carry ceilings, never provider caps).
- **Status**: ✅ Completed — `internal/billing` ships the reading/total types, envelope ceilings, deadline arithmetic and the S1 exhaustion codes; PR #240.
- **Implementation**: `ResolveCeilings` treats a negative field as unset and zero as set; `Deadline` extends only by quiesced spans (an unresumed quiesced span runs to `now`) and reports expired when `now` is not before the deadline. Commit 8561511.
- **Spec deviations**: None.
- **Files modified**: `internal/billing/billing.go`, `internal/billing/envelope.go`, `internal/billing/billing_test.go`, `internal/billing/envelope_test.go`, `specs/in-progress/budget-ledger-s1/tasks.md`, `specs/in-progress/budget-ledger-s1/handoff.md`.

### Task 2 — Counter normalization and component split ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 1
- **Change**: Add pure AT-13 normalization over Task 1's types plus the per-route component split, so cumulative/delta/missing counters normalize without double counting and unmapped components stay unknown.
- **Files**:
  - `internal/billing/normalize.go` (`Normalize`, `SplitUsage`, first-route mapping row)
  - `internal/billing/normalize_test.go`
- **Produces**: `billing.Normalize(rs []Reading, expected []string, applied map[EventID]bool) (Normalized, error)`, `billing.SplitUsage(route string, u adapter.TokenUsage) ScopeTotal`
- **Acceptance**:
  - `TestNormalizeNewestCumulativeWins`: two cumulatives for one (scope, unit, source) yield the newest value; asserting the older fails. Command: `go test ./internal/billing/ -run TestNormalize`. Fails before: `Normalize` absent.
  - `TestNormalizeDeltaAppliedOnce`: a delta with (producer, sequence) already in `applied` is skipped; removing the skip double-counts and fails. Fails before: `Normalize` absent.
  - `TestNormalizePreBaselineDeltaSkipped`: an unapplied delta with observation time at or before the baseline cumulative's is skipped as already covered; summing it fails. Fails before: `Normalize` absent.
  - `TestNormalizeIdentityLessDeltaEstimated`: a delta with `HasIdentity=false` lands `Estimated` and is never summed into an exact total; asserting `Reported` fails. Fails before: `Normalize` absent.
  - `TestNormalizeMissingScopeUnknown`: an expected scope with no readings marks it unknown with nil total while other scopes keep values; a zero total fails. Fails before: `Normalize` absent.
  - `TestNormalizeSameScopeDistinctIdentities`: two cumulatives sharing a scope but differing in unit or source yield two distinct totals keyed by full identity; a single merged total fails. Fails before: `Normalize` absent.
  - `TestNormalizeRejectsMalformedReadings`: empty scope/unit/source, both/neither of Cumulative/Delta set, or non-decimal quantity text, returns `ErrReadingShape` naming the index; silent acceptance fails. Fails before: `Normalize` absent.
  - `TestNormalizeDecimalExact`: a `retail-equivalent` USD cumulative of `0.37` normalizes to exactly `0.37`, and decimal deltas sum exactly (`0.10` + `0.27` = `0.37`); any rounded or float-derived text fails. Fails before: `Normalize` absent.
  - `TestNormalizeDeltasSumFromZero`: identity-carrying deltas with no cumulative baseline sum from zero exactly (`0.10` + `0.27` = `0.37`); an error or dropped deltas fail. Fails before: `Normalize` absent.
  - `TestNormalizeLabelsDerive`: a `retail-equivalent` total labels `Estimated`, an AC-2.2 marker labels `unknown`, an all-identified token total labels `Reported`, and any identity-less delta among an identity's inputs makes its total `Estimated`; any other label fails. Fails before: `Normalize` absent.
  - `TestSplitUsageUnmappedCombinedOnly`: an unknown route returns the combined total with all components nil; any non-nil component fails. `TestSplitUsageFirstRouteSplits`: the first-route mapping row (keyed by adapter harness ID, the key Task 3 passes) exposes non-overlapping components citing fixture evidence. Fails before: `SplitUsage` absent.
- **Test plan**: Table tests over inline readings; cross-(scope, unit, source) mismatch cases included so unlike buckets never merge.
- **Invariants touched**: I09 (v2 §7.3: missing is unknown, never zero; identity-less deltas estimated, AC-2.1/AC-2.2).
- **Status**: ✅ Completed — `billing.Normalize` and `billing.SplitUsage` with the `claude-code` mapping row landed; PR #246.
- **Implementation**: Exact decimal sums via scale-aligned `math/big` integers; the baseline literal passes through unchanged. An identity with no baseline whose deltas were all skipped is unknown, not zero. Commit dae117b.
- **Spec deviations**: None.
- **Files modified**: `internal/billing/normalize.go`, `internal/billing/normalize_test.go`, `specs/in-progress/budget-ledger-s1/tasks.md`, `specs/in-progress/budget-ledger-s1/handoff.md`.

### Task 3 — Migration 0003, ledger rows, ingest projection ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: complex (persistence schema migration)
- **Depends on**: Task 1, Task 2
- **Change**: Add `0003_ledger.sql` with the four ledger tables, bump `SchemaVersion` 2→3, add the row accessors plus `Transact`, and project `attempt.native_result` into typed usage rows (per-attempt deltas per §3, `applied` from existing rows) in the same ingest transaction, so observations are durable and queryable. Re-check the next free migration number at task start (0003 while only 0001/0002 exist) and renumber per the Dependencies coordination note if MH-21 or MH-13 landed first. If MH-21's `reservations` table landed first, reconcile states and API per the Dependencies adoption note before adopting (adopted store must support the `orphaned` transition plus bucket coupling, else extend additively); record a spec deviation either way.
- **Files**:
  - `internal/journal/migrations/0003_ledger.sql`
  - `internal/journal/journal.go` (SchemaVersion 2→3 only)
  - `internal/journal/ledger.go` (row types, readers, writers, `Transact`)
  - `internal/journal/ledger_test.go`
  - `internal/supervisor/ingest.go` (project native_result into usage rows via `billing.Normalize`; route key `adm.Adapter.Harness`)
  - `internal/supervisor/ingest_ledger_test.go` (new; native_result → rows cases)
- **Produces**: `usage_observations`, `run_envelopes`, `reservations`, `bucket_state` tables; `UsageRow`, `EnvelopeRow`, `ReservationRow`, `BucketRow` (design §2, exact fields); `func (j *Journal) Transact(ctx context.Context, fn func(*sql.Tx) error) error`, `journal.InsertUsageObservation(ctx, tx, UsageRow) error`, `func (j *Journal) UsageObservations(ctx context.Context, runID string) ([]UsageRow, error)`, `journal.UsageObservationsTx(ctx, tx, runID) ([]UsageRow, error)`, `journal.UpsertRunEnvelope(ctx, tx, EnvelopeRow) error`, `func (j *Journal) RunEnvelope(ctx context.Context, runID string) (EnvelopeRow, error)`, `journal.InsertReservation(ctx, tx, ReservationRow) error`, `func (j *Journal) Reservation(ctx context.Context, reservationID string) (ReservationRow, error)`, `journal.SetReservationReleased(ctx, tx, reservationID, evidence string, at time.Time) error`, `journal.TouchReservation(ctx, tx, reservationID string, at time.Time) error`, `journal.MarkReservationOrphaned(ctx, tx, reservationID string) error`, `journal.SetBucketExhausted(ctx, tx, bucket, exhaustedAt string, resetAt *string) error`, `func (j *Journal) BucketState(ctx context.Context, bucket string) (BucketRow, error)`, `journal.NoteBucketRetry(ctx, tx, bucket string) error`, `journal.ClearBucket(ctx, tx, bucket string) error`
- **Acceptance**:
  - `TestMigration0003CreatesLedgerTables`: a 0002 database migrates; the four tables exist; the 0001/0002 DDL bytes are byte-identical to the golden snapshot. Command: `go test ./internal/journal/ -run TestMigration0003`. Fails before: file absent.
  - `TestMigration0001Untouched`: SHA-256 of `0001_init.sql` equals the committed hash; any edit fails. Fails before only if edited (guard).
  - `TestSchemaVersionIs3`: `SchemaVersion == 3` after 0003. Fails before: version is 2.
  - `TestNativeResultProjectsUsageRows`: ingesting a native_result with per-model tokens and a cost writes Reported token rows plus one Estimated retail row (per-model scopes, `Delta` mode, event `ProducerID`/`ProducerSequence` identity); an identity-less delta passed to the writer directly persists with NULL producer and reads back with empty producer, never erroring; no row aggregates unlike buckets. Command: `go test ./internal/supervisor/ -run TestNativeResultProjectsUsageRows`. Fails before: projection absent (zero rows).
  - `TestNativeResultAttemptsAccumulate`: ingesting native_results from two attempts of one run accumulates same-scope deltas exactly with no double counting; a replaced (non-accumulated) total fails. Fails before: projection absent (zero rows).
  - `TestNativeResultReplayIgnored`: appending the same native_result event twice projects once — the second `Append` journals nothing new by `event_id` and stores no duplicate rows; duplicated rows fail. Fails before: projection absent (zero rows).
  - `TestNativeResultNullsProjectUnknown`: null/absent `usage_native_reported` and cost project to `unknown` quantities with NULL producer, never 0. Fails before: projection absent (zero rows).
  - `TestUnknownLabelRejectedNeverDefaulted`: inserting a row with an empty or out-of-scale label errors; a variant that coerces to a passing label fails. Fails before: accessor absent.
  - `TestEmptyQuantityRejected`: inserting a usage or reservation row with quantity `''` errors (CHECK plus accessor validation); a stored `''` fails. Fails before: accessor absent.
  - `TestBucketStateMissingIsNoRows`: `BucketState` on a never-exhausted bucket returns `sql.ErrNoRows`; any other outcome fails. Fails before: accessor absent.
- **Test plan**: Temp databases migrated from the checked-in fixture chain; golden DDL snapshot; ingest tests through `projectWorkerEvent` with synthetic spool lines.
- **Invariants touched**: I23 (v2 §5.1: single ledger gains tables, no second store); I09 (v2 §7.3: labels checked, quantity `unknown` distinct from zero); G16 (additive only).
- **Status**: ✅ Completed — `0003_ledger.sql`, `SchemaVersion` 3, the ledger accessors with `Transact`, and the `native_result` usage projection landed; PR #252.
- **Implementation**: Each native_result writes one increment row per (scope, unit, source) from `billing.Normalize`, with the run's identified rows as `applied`; missing scopes become `unknown` markers with NULL producer. Route key is the harness ID read from the run's journaled `admission.decided`. Commit a04b539.
- **Spec deviations**: (1) `internal/journal/journal_test.go` and `internal/journal/qualification_test.go` changed (outside Files): they hard-coded schema version 2. (2) Per-model token rows carry the `SplitUsage` combined total; the design DDL has no component columns, so mapped components are not persisted. (3) A cost that is not plain decimal text is treated as unreported (unknown row), so one odd literal cannot stall spool ingestion. 0003 was free and no MH-21 `reservations` table had landed, so no renumbering or adoption applied.
- **Files modified**: `internal/journal/migrations/0003_ledger.sql`, `internal/journal/journal.go`, `internal/journal/journal_test.go`, `internal/journal/qualification_test.go`, `internal/journal/ledger.go`, `internal/journal/ledger_test.go`, `internal/supervisor/ingest.go`, `internal/supervisor/ingest_ledger_test.go`, `specs/in-progress/budget-ledger-s1/tasks.md`, `specs/in-progress/budget-ledger-s1/handoff.md`.

### Task 4 — Admission reservation coupling ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 1, Task 3, Task 6
- **Change**: Resolve ceilings from the Decision envelope layers (Task 6) via `billing.ResolveCeilings`, write the initial run envelope row with `UpsertRunEnvelope`, hold one typed quota reservation coupled to the admitted bucket at admission through the run owner, and refuse admission when any of the three fails, so every S1 run carries its envelope and bucket coupling from birth.
- **Files**:
  - `internal/admission/reserve.go` (`QuotaBucket`, `Reserver`, `NewJournalReserver`, `HoldQuotaReservation`, bucket-refusal check)
  - `internal/admission/reserve_test.go`
  - `internal/supervisor/pipeline.go` (call the hold in `appendAdmission`; block the run on failure)
  - `internal/supervisor/pipeline_test.go` (hold-failure blocking case)
- **Produces**: `admission.QuotaBucket(rec qualify.Record, identityRef string) string`, `admission.Reserver` interface, `admission.NewJournalReserver(j *journal.Journal, c billing.Ceilings) Reserver`, `admission.HoldQuotaReservation(ctx, Reserver, runID string, rec qualify.Record, identityRef string) (string, error)`, `admission.ErrDuplicateHold`
- **Acceptance**:
  - `TestHoldCouplesAdmittedBucket`: the held reservation's scope equals `QuotaBucket` of the admitted record; a mismatched bucket fails. Command: `go test ./internal/admission/ -run TestHoldCouples`. Fails before: hold absent (no reservation).
  - `TestAdmissionWritesInitialEnvelope`: an admitted run gains a `run_envelopes` row equal to `ResolveCeilings` of the Decision layers; a missing row or an unresolved ceiling fails. Command: `go test ./internal/supervisor/ -run TestAdmissionWritesInitialEnvelope`. Fails before: hook absent.
  - `TestHoldFailureBlocksRun`: a failing hold blocks the run in `appendAdmission`; an admitted run with no reservation fails. Command: `go test ./internal/supervisor/ -run TestHoldFailureBlocksRun`. Fails before: hook absent.
  - `TestFailedAdmissionHoldsNothing`: a refused admission leaves zero reservation rows (no partial claims, AC-3.2); any row fails. The success path holds exactly one row, which fails before.
  - `TestOneBucketPerRun`: one run holds at most one live reservation and a second hold for the same run returns `ErrDuplicateHold` (AC-6.4 guard: S1 preauthorises no alternative route). Fails before: hold absent.
  - `TestReservationWordingIsLocalOnly`: the surfaced reservation text contains `local coordination only — not provider availability`; golden-pinned. Fails before: text absent.
  - `TestUnknownQuantityNeverZero`: the held reservation stores quantity `unknown`; a `0` or empty quantity is rejected. Fails before: hold absent.
  - `go test ./internal/admission/ -run TestUnknownQuotaAdmitsStopAtExhaustion`: the existing AC-7.2 guard still passes (cited, not added). Fails before only on regression (guard).
- **Test plan**: Fake `Reserver` for unit tests; pipeline test with temp state dir asserting blocked-on-hold-failure.
- **Invariants touched**: I02 (v2 §7.3: failed hold blocks admission); I10 (v2 §2: local reservation never presented as provider availability, AC-3.3); I04 (v2 §2: one bucket per run, no preauthorised alternative, AC-6.4).
- **Status**: ✅ Completed — every admitted run now gets its resolved `run_envelopes` row in the admission transaction and one `unknown`-quantity reservation coupled to its bucket; a failed hold blocks the run; PR #256.
- **Implementation**: `reserve.go` holds inside the caller's transaction with a count-based duplicate check; `appendAdmission` resolves the Decision layers and, in the one admission append, writes the envelope and holds the reservation, so a failed hold rolls the admission back and blocks the run with `quota_reservation_failed`. The bucket record is built from the Decision. Commits bebda35, review fix in the next commit.
- **Spec deviations**: (1) `Reserver.Reserve(ctx, tx, runID, bucket, owner)` takes the run and the caller's transaction, `HoldQuotaReservation` takes the transaction, and `NewJournalReserver(c)` drops the journal, so the hold commits with the admission (design §4); the design signatures omit the run and use a journal-bound reserver. (2) The bucket's record is built from the Decision (adapter harness and surface, billing posture entitlement class, native auth identity), since the Decision does not carry the consulted record. (3) The duplicate-hold check is a `COUNT` on the caller's transaction in `reserve.go`, not a new journal accessor. (4) The notice names the bucket, not the reservation id, as a random id broke the stable-stderr test `TestE2EJsonlStable`.
- **Files modified**: `internal/admission/reserve.go`, `internal/admission/reserve_test.go`, `internal/supervisor/pipeline.go`, `internal/supervisor/pipeline_test.go`, `specs/in-progress/budget-ledger-s1/tasks.md`, `specs/in-progress/budget-ledger-s1/handoff.md`.

### Task 5 — Exhaustion signal taxonomy and fake scenarios ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: complex (6 files; three are fixtures/tests, but the tier rule is literal)
- **Depends on**: Task 1
- **Change**: Teach the claudecode decoder the fixture-qualified `allowance_exhausted` class with fail-closed mapping, and add the fake-adapter scenarios later tasks consume, so exhaustion is detectable without live evidence.
- **Files**:
  - `adapters/claudecode/decode.go` (new class in `errorClasses` + documented-shape mapping)
  - `adapters/claudecode/decode_test.go`
  - `adapters/claudecode/testdata/exhaustion/allowance_exhausted.json` (synthetic frame, marked synthetic)
  - `adapters/fake/scenarios/usage-counters.json` (per-attempt delta and missing usage frames)
  - `adapters/fake/scenarios/allowance-exhausted.json` (exhaustion frames + reset-unknown shape)
  - `adapters/fake/fake_test.go` (extend `TestScenariosEmbedded`'s pinned list with the two names)
- **Produces**: `allowance_exhausted` error class; `usage-counters`, `allowance-exhausted` fake scenarios
- **Acceptance**:
  - `TestDecodeAllowanceExhausted`: the synthetic frame decodes to class `allowance_exhausted` with reset unknown; any other class fails. Command: `go test ./adapters/claudecode/ -run TestDecodeAllowanceExhausted`. Fails before: class absent.
  - `TestRateLimitKeepsTransientMapping`: a rate-limit frame still maps to `rate_limit` (not exhaustion); mapping it to exhaustion fails. Fails before only on regression (guard pinning D10).
  - `TestUnknownShapeNeverExhaustion`: an undocumented error shape maps to `native_error`, never `allowance_exhausted`. Fails before: class absent (documents fail-closed).
  - `go test ./adapters/fake/ -run TestScenariosEmbedded`: the pinned list gains `usage-counters` and `allowance-exhausted` and both parse; an unlisted or unparseable scenario fails. Fails before: names absent from the list.
- **Test plan**: Synthetic fixtures with `synthetic: true` markers per the adapter rule; scenario playback through the existing fake harness.
- **Invariants touched**: I14 (v2 §2: fixture-tested taxonomy explicitly labelled, not live-qualified); I09 (v2 §7.3: unknown reset stays unknown).
- **Status**: ✅ Completed — the claudecode decoder maps the fixture-qualified `allowance_exhausted` class fail-closed and the `usage-counters` and `allowance-exhausted` fake scenarios are embedded; PR #245.
- **Implementation**: `allowance_exhausted` joins `errorClasses`; the exact class name is the only documented shape, so every other spelling stays `native_error` and `rate_limit` keeps its transient mapping. `NativeError` carries only a class, so reset stays unknown. The synthetic fixture holds three named streams (exhausted, rate-limited, undocumented shape). Commit ffdc9eb.
- **Spec deviations**: None.
- **Files modified**: `adapters/claudecode/decode.go`, `adapters/claudecode/decode_test.go`, `adapters/claudecode/testdata/exhaustion/allowance_exhausted.json`, `adapters/fake/scenarios/usage-counters.json`, `adapters/fake/scenarios/allowance-exhausted.json`, `adapters/fake/fake_test.go`, `specs/in-progress/budget-ledger-s1/tasks.md`, `specs/in-progress/budget-ledger-s1/handoff.md`.

### Task 6 — Envelope configuration and extension decision ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: complex (recover-path interplay, new control event; 10 files justified: one logical change — the envelope-config chain from flags/config through Decide to Decision — with a test file for each new behavior; splitting config from threading would leave flags that do not compile)
- **Depends on**: Task 1, Task 3
- **Change**: Add the `[envelopes]` config table with strict decode, thread the four `--envelope-*` run flags and the file layer through `Decide` onto the `Decision` unresolved, and add the recorded user extension decision (`run.extension_granted` + `CheckExtension`), so Task 4 resolves ceilings from all three layers and extensions are recorded.
- **Files**:
  - `internal/supervisor/envelope.go` (`RequestExtension`, `CheckExtension`; creates the file Task 7 extends)
  - `internal/supervisor/envelope_test.go` (extension cases; Task 7 adds gate cases)
  - `internal/admission/projectconfig.go` (`[envelopes]` table, strict decode into `EnvelopesConfig`)
  - `internal/admission/projectconfig_test.go` (extend with envelopes cases)
  - `internal/admission/admission.go` (`Request.EnvelopeFlags` + `Decision.EnvelopeFlags` + Decide pass-through; MH-13 regions note in Dependencies)
  - `internal/admission/admission_test.go` (new; Decide layer-carriage cases)
  - `internal/cli/run.go` (four `--envelope-*` flags into the admission request)
  - `internal/cli/run_envelope_test.go` (new; flag parsing and precedence)
  - `internal/supervisor/recover.go` (journal `run.extension_granted` when the operator resumes past a ceiling)
  - `internal/supervisor/recover_envelope_test.go` (new; extension-event cases)
- **Produces**: `[envelopes]` TOML table decoded as `admission.EnvelopesConfig` with `func (EnvelopesConfig) ToCeilings() (*billing.Ceilings, error)`, `admission.ProjectConfig.Envelopes EnvelopesConfig`, `admission.Request.EnvelopeFlags *billing.Ceilings`, `admission.Decision.EnvelopeFlags *billing.Ceilings` (both layers carried unresolved; Task 4 resolves), `--envelope-execution`, `--envelope-repairs`, `--envelope-replans`, `--envelope-transport-retries` flags, `run.extension_granted` event `{kind, raised_to, decided_by}` (`raised_to` in row-native units: seconds for `execution`, counts otherwise), `supervisor.RequestExtension(ctx, *journal.Journal, runID, kind string, raisedTo int64, decidedBy string) error`, `supervisor.CheckExtension(ctx, *journal.Journal, runID, kind string) (bool, error)`
- **Acceptance**:
  - `TestEnvelopesStrictDecode`: unknown `[envelopes]` keys, invalid durations and negative counts fail decode; a permissive variant fails. Command: `go test ./internal/admission/ -run TestEnvelopesStrictDecode`. Fails before: table absent.
  - `TestEnvelopeFlagPrecedence` (CLI): `--envelope-repairs 5` sets `Request.EnvelopeFlags` repairs to 5 while `[envelopes] repairs = 1` decodes to the separate file layer; either layer dropped or misparsed fails (precedence itself is Task 1's `ResolveCeilings`, applied by Task 4). Command: `go test ./internal/cli/ -run TestEnvelopeFlagPrecedence`. Fails before: flags absent.
  - `TestDecideCarriesEnvelopeLayers`: `Decide` carries the flag and file envelope layers onto the `Decision` unresolved; a resolved-or-dropped layer fails. Command: `go test ./internal/admission/ -run TestDecideCarriesEnvelopeLayers`. Fails before: fields absent.
  - `TestExtensionGrantsRecordedDecision`: resuming past a ceiling journals `run.extension_granted` with kind, raised_to (seconds for `execution`, counts otherwise) and decided_by, and raises the envelope row. Command: `go test ./internal/supervisor/ -run TestExtensionGrants`. Fails before: event absent.
  - `TestCheckExtensionFailsClosed`: no event → false; journal read error → error, never assumed granted. Fails before: function absent.
  - `TestExtensionRejectsUnknownKind`: an unknown kind or a run not blocked for that kind errors; granting fails. Fails before: function absent.
- **Test plan**: TOML fixtures (valid + each invalid shape); CLI tests through flag parsing plus one `cli.Main` exit-code case; recover tests with temp journals on envelope-blocked runs.
- **Invariants touched**: I02 (v2 §2: strict decode fails closed; unknown extension never assumed); I21 (v2 §10.3: ceilings configurable but always finite); I20 (v2 §2: extension binds the recorded decision, not a mutable grant).
- **Status**: ✅ Completed — the `[envelopes]` table, the four `--envelope-*` flags and the recorded `run.extension_granted` decision landed; the flag and file layers reach the `Decision` unresolved; PR #254.
- **Implementation**: `RequestExtension` journals the event and raises the envelope row in one append; `CheckExtension` is true only when a grant follows the run's latest block for that kind. `Hooks.Extension` carries the operator's decision into `RecoverWithHooks`, which journals it before anything else. Commit 469e067.
- **Spec deviations**: `internal/supervisor/pipeline.go` is outside the task's Files list: `Hooks.Extension` lives there, as `RecoverWithHooks` has no other channel for the operator's decision.
- **Files modified**: `internal/admission/admission.go`, `internal/admission/admission_test.go`, `internal/admission/projectconfig.go`, `internal/admission/projectconfig_test.go`, `internal/cli/run.go`, `internal/cli/run_envelope_test.go`, `internal/supervisor/envelope.go`, `internal/supervisor/envelope_test.go`, `internal/supervisor/pipeline.go`, `internal/supervisor/recover.go`, `internal/supervisor/recover_envelope_test.go`, `specs/in-progress/budget-ledger-s1/tasks.md`, `specs/in-progress/budget-ledger-s1/handoff.md`.

### Task 7 — Envelope counting and launch gate ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: complex (deadline/stop interplay, gate on the launch path)
- **Depends on**: Task 1, Task 3 (shares `internal/supervisor/ingest.go`; sequential only), Task 4 (shares `internal/supervisor/pipeline.go` and `pipeline_test.go`; sequential only), Task 6 (consumes `CheckExtension`; shares `envelope.go`; sequential only)
- **Change**: Count repairs/replans/transport-retries against existing projections and refuse over-ceiling launches with specific blocked reasons, so every run executes inside finite envelopes.
- **Files**:
  - `internal/supervisor/envelope.go` (`GateLaunch`, `IsReplan`, counting, deadline context)
  - `internal/supervisor/envelope_test.go` (gate cases)
  - `internal/supervisor/pipeline.go` (gate in `attempt`, first-start recording, deadline context)
  - `internal/supervisor/ingest.go` (accumulate progress retries into `transport_retries_seen`; sequential after Task 3)
  - `internal/supervisor/pipeline_test.go` (gate refusal through the pipeline)
- **Produces**: `supervisor.GateLaunch(ctx, *journal.Journal, runID string, c billing.Ceilings) error`, `supervisor.IsReplan(ctx, *journal.Journal, runID string) (bool, error)` (consumed by Task 9's reserve hook); run reasons `envelope_repairs_exhausted`, `envelope_replans_exhausted`, `envelope_transport_retries_exhausted`, `envelope_deadline_exceeded`
- **Acceptance**:
  - `TestGateRefusesOverCeilingRepairs`: a run at its repair ceiling refuses the next launch and becomes `blocked` with reason `envelope_repairs_exhausted`; journaling the intent fails. Command: `go test ./internal/supervisor/ -run TestGateRefuses`. Fails before: gate absent (intent journals).
  - `TestGateRefusesOverCeilingReplans`: a run at its replan ceiling refuses the next replan and becomes `blocked` with reason `envelope_replans_exhausted`; journaling the intent fails. Command: `go test ./internal/supervisor/ -run TestGateRefusesOverCeilingReplans`. Fails before: gate absent (intent journals).
  - `TestGateHonoursGrantedExtension`: the same run with a journaled `run.extension_granted` for repairs proceeds. Fails before: gate absent.
  - `TestRepairReplanClassification`: attempt N>1 with a `verifications` row counts a repair, without counts a replan; swapped counts fail. Fails before: gate absent.
  - `TestIsReplanReadsVerifications`: `IsReplan` returns false with a `verifications` row, true without; a journal error errors, never defaults. Command: `go test ./internal/supervisor/ -run TestIsReplan`. Fails before: helper absent.
  - `TestTransportCeilingCountsProgressRetries`: ingested progress retries accumulate and trip `envelope_transport_retries_exhausted` past the ceiling. Fails before: counter absent.
  - `TestDeadlineExpiryBlocks`: an expired deadline blocks with `envelope_deadline_exceeded` via the stop ladder; candidates preserved. Fails before: no deadline (run proceeds).
- **Test plan**: Temp state dirs; frozen clocks for deadline tests (no sleeps).
- **Invariants touched**: I21 (v2 §10.3: finite execution, repair, replan, transport envelopes enforced); I06 (v2 §2: deadline stop runs the existing stop ladder, unconfirmed until reconciled); I02 (v2 §2: over-ceiling launch refused).
- **Status**: ✅ Completed — `GateLaunch` refuses over-ceiling and past-deadline launches before any intent, repairs and replans are counted, progress retries accumulate, and the execution deadline stops the attempt through the stop ladder into a blocked run; PR #259.
- **Implementation**: `planLaunchAt` reads the envelope row, takes the row's raised value for any kind with a recorded extension, and checks the deadline then the counts; `GateLaunch` is that check alone. The launch is counted, and the first start stamped, by `plan.commit` in the launch intent's own transaction, as a conditional in-place `UPDATE` that enforces the ceiling, after re-checking the deadline at commit time. `projectTransportRetries` rejects a malformed `retries` as a corrupt spool line and recomputes the total from each attempt's largest integer report. `watch` arms a deadline timer; `endStop` ends a deadline stop as blocked. Commits 56008ef and the review-fix commit.
- **Spec deviations**: (1) `internal/supervisor/state.go` is outside the task's Files: `RecordLaunchIntent` gains an unexported `recordLaunchIntent` that takes an extra projection, so the launch is counted in the intent's transaction. (2) The pipeline runs one attempt per run, so repair and replan refusals are tested on `GateLaunch` and `blockLaunch`, and the pipeline path with an aged clock (a first-launch refusal) and the deadline stop. (3) Task 4's `TestAdmissionWritesInitialEnvelope` no longer asserts an empty `FirstStartAt`, since the run now records its first start (Task 4's hand-off said Task 7 would). (4) `TestDeadlineExpiryBlocks` uses a 2s ceiling, not 1ms: a ceiling that expires before the intent commits is now refused at launch. (5) The execution ceiling is stored in whole seconds, so the row cannot hold a sub-second ceiling; the gate uses the resolved duration unless an extension was granted.
- **Files modified**: `internal/supervisor/envelope.go`, `internal/supervisor/envelope_test.go`, `internal/supervisor/ingest.go`, `internal/supervisor/pipeline.go`, `internal/supervisor/pipeline_test.go`, `internal/supervisor/state.go`, `specs/in-progress/budget-ledger-s1/tasks.md`, `specs/in-progress/budget-ledger-s1/handoff.md`.

### Task 8 — Exhaustion path: preserve, block, schedule, never pay

- **Domain/agent**: go-implementer
- **Budget**: complex (recovery interplay, bucket-wide refusal)
- **Depends on**: Task 3 (consumes `SetReservationReleased`/`TouchReservation`/`MarkReservationOrphaned`/bucket readers; shares `ingest.go`; sequential only), Task 4 (shares `reserve.go`; sequential only), Task 5, Task 6 (shares `recover.go`; sequential only), Task 7 (shares `pipeline.go`/`ingest.go`; sequential only)
- **Change**: Map the exhaustion signal to preserved candidates, a blocked run, bucket-wide admission refusal and the operator-executed retry schedule, with release-after-stop and orphan-on-recovery reservation hygiene, so allowance exhaustion stops safely instead of charging.
- **Files**:
  - `internal/workers/worker.go` (map `allowance_exhausted` to attempt reason; sticky-poison precedence over rate_limit)
  - `internal/workers/worker_outcome_test.go` (new; outcome-mapping cases)
  - `internal/supervisor/exhaustion.go` (`RetrySchedule`, `EvaluateBucket`, `HandleExhaustion`)
  - `internal/supervisor/exhaustion_test.go`
  - `internal/admission/reserve.go` (refuse new holds on a live exhausted bucket; operator re-admission via `EvaluateBucket`)
  - `internal/supervisor/ingest.go` (heartbeat touch on native_result projection; sequential after Tasks 3, 7)
  - `internal/supervisor/pipeline.go` (call `HandleExhaustion` in conclude after freeze; sequential after Tasks 4, 7)
  - `internal/supervisor/recover.go` (mark unreconciled attempts' reservations orphaned after reconciliation; sequential after Task 6)
- **Produces**: `supervisor.RetrySchedule(resetAt *time.Time, retriesUsed int) (wait time.Duration, giveUp bool)`, `supervisor.EvaluateBucket(row journal.BucketRow, now time.Time) (admit bool, wait time.Duration, giveUp bool)`, `supervisor.HandleExhaustion(ctx, *journal.Journal, *Producer, runID, reservationID string, resetAt *time.Time) error`; attempt reason `allowance_exhausted`; run reason `allowance_exhausted`; admission refusal code `allowance_exhausted`
- **Acceptance**:
  - `TestExhaustionWinsOverRateLimit`: exhaustion plus rate_limit observations yield `allowance_exhausted`, not `provider_limit`. Command: `go test ./internal/workers/ -run TestExhaustionWins`. Fails before: class unmapped.
  - `TestExhaustionPreservesCandidates`: an exhaustion run keeps its frozen candidate and session artifacts; missing artifacts fail. Fails before: run fails instead of blocking.
  - `TestBucketRefusesNewWork`: while the bucket row is live and now precedes the next scheduled time, a new admission refuses with `allowance_exhausted`; admitting fails. Fails before: refusal absent.
  - `TestRetryScheduleResetAndUnknown`: authoritative reset schedules ≤3 retries; unknown reset backs off exactly 1m, 5m, 15m then gives up with timing shown unknown; a 4th retry or invented reset fails. Fails before: scheduler absent.
  - `TestEvaluateBucketAdmitsAndClears`: at/past the scheduled time the operator re-admission admits, increments `retries_used`, and clears the row when consumed (3 used) or `now >= reset_at`; early admission or missing increment fails. Refusals never increment. Fails before: evaluator absent.
  - `TestReleaseOnlyAfterStop`: the run's reservation flips to released only after terminal attempt state, with release evidence; early release fails. Fails before: release unwired.
  - `TestExpiredReservationOrphanedNeverReused`: recovery marks unreconciled rows `orphaned`; a new hold mints a new id and never reuses the orphan. Fails before: orphaning absent.
  - `TestNoSecondNativeLaunch`: an exhaustion-scenario packaged run launches the native exactly once; two launches fail (D13: MYTHHELM itself schedules nothing automatic). Fails before: path absent.
- **Test plan**: Worker outcome unit tests; supervisor tests with temp journals and frozen clocks; launch-count via fake-adapter scenario run.
- **Invariants touched**: I02 (v2 §7.3: exhausted bucket stops new model work); I04 (v2 §2: no alternative route preauthorised; pinned waits, AC-6.4); I12 (v2 §2: reconcile before retry — candidates frozen first, orphans marked after reconciliation).

### Task 9 — Completion reserve gate

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 2, Task 7, Task 8 (shares `pipeline.go`; sequential only)
- **Change**: Quantify one verification pass plus the repair ceiling as estimated data and block material replans the run could not finish verifying, so completion is budgeted, not just drafting.
- **Files**:
  - `internal/billing/reserve.go` (`ReserveEstimate`, `CheckBound`, `EstimateReserve`, `ExecutionRemainder`, `RemainderCoversReserve`)
  - `internal/billing/reserve_test.go`
  - `internal/supervisor/pipeline.go` (evaluate before dispatching any material replan after the first verification, using Task 7's `IsReplan`; repairs exempt; sequential after Tasks 4, 7, 8)
  - `internal/supervisor/envelope_test.go` (gate cases through the shared harness; sequential after Tasks 6, 7)
- **Produces**: `billing.ReserveEstimate`, `billing.CheckBound` (`{Name, Timeout}`), `billing.EstimateReserve(checks []CheckBound, c Ceilings) ReserveEstimate`, `billing.ExecutionRemainder` (`{TimeLeft}` only), `billing.RemainderCoversReserve(rem ExecutionRemainder, est ReserveEstimate) bool` (exact predicate: `TimeLeft >= VerifyTimeoutSum`), pipeline conversion of `[]admission.CheckConfig` via `CheckConfig.Duration()` (failure blocks the launch), dispatched-replan derived deadline (execution deadline minus `VerifyTimeoutSum`, enforced via Task 7's deadline context), run reason `completion_reserve_shortfall`
- **Acceptance**:
  - `TestEstimateQuantifiesVerifyPass`: estimate over a `[]CheckBound` fixture equals its count plus its timeout sum plus the repair ceiling, with `Note` exactly `estimate, not a reserve of provider quota`. Command: `go test ./internal/billing/ -run TestEstimateQuantifies`. Fails before: estimator absent.
  - `TestCheckBoundConversionBlocksOnFailure`: the pipeline hook converts the admitted check list via `CheckConfig.Duration()`; an unparseable timeout blocks the launch instead of estimating. Fails before: conversion absent.
  - `TestRemainderCoversReserveBoundary`: exactly-at-boundary covers; one nanosecond short does not (counts play no part). Fails before: predicate absent.
  - `TestShortfallBlocksOptionalWork`: a remainder that fails the predicate blocks a material replan with `completion_reserve_shortfall` instead of dispatching; dispatching fails. Fails before: gate absent.
  - `TestReplanDeadlineLeavesReserve`: a dispatched replan runs under execution-deadline-minus-`VerifyTimeoutSum`; a replan allowed to run past it fails. Fails before: derived deadline absent.
  - `TestReplanPreservesRepairSlots`: dispatching a replan leaves `repairs_used` unchanged while imposing the derived deadline; a consumed repair slot fails. Fails before: gate absent.
  - `TestRepairsNotReserveGated`: a repair dispatches under GateLaunch counts after an earlier repair consumed part of the ceiling, while time covers the verify pass; blocking it fails. Fails before only on regression (guard pinning the full ceiling stays reachable).
  - `TestFirstAttemptUnaffected`: the first attempt never evaluates the reserve (nothing optional yet); blocking it fails. Fails before only on regression (guard).
  - `TestReserveNeverHardTokenClaim`: the estimate carries no token-quantity field and the Note pins estimate-only; a hard-claim rendering fails (pinned again in Task 10 goldens).
- **Test plan**: Pure estimator tests with synthetic check lists; pipeline gate test with a depleted envelope fixture.
- **Invariants touched**: I10 (v2 §2: reserve is an estimate, never an advertised hard token reserve, AC-5.2); I21 (v2 §10.3: completion budgeted before optional work).

### Task 10 — Receipt and CLI ledger surfaces

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 2, Task 3, Task 6 (shares `run.go`; sequential only), Task 9
- **Change**: Render typed usage rows, the reserve estimate, unknown remaining and the retry schedule in receipts and CLI run output with honest labels, so every S1 surface says what was proven and what was not.
- **Files**:
  - `internal/supervisor/receipt.go` (`usage_observations`, `reserve`, `remaining`, `next_retry` members)
  - `internal/supervisor/receipt_test.go`
  - `internal/cli/render.go` (notice-line shaping, if needed beyond existing Notice path)
  - `internal/cli/render_test.go` (new; notice goldens)
  - `internal/cli/run.go` (emit the four Notice lines from `finish`; sequential after Task 6)
- **Produces**: receipt `billing.usage_observations`, `billing.reserve`, `billing.remaining`, `billing.next_retry`; CLI `usage:` / `reserve:` / `remaining:` / `next retry:` notice lines
- **Acceptance**:
  - `TestReceiptCarriesTypedUsage`: receipt renders each row with scope, unit, source, label, quantity; retail estimate keeps `note: estimate, not a charge`. Command: `go test ./internal/supervisor/ -run TestReceiptCarriesTypedUsage`. Fails before: members absent.
  - `TestReceiptUnknownNeverZero`: missing quantities render `unknown`, never `0` or a balance; any zero fails. Fails before: members absent.
  - `TestReceiptReserveIsEstimateOnly`: `billing.reserve` carries the Task 9 Note verbatim; a hard-claim phrasing fails. Fails before: member absent.
  - `TestReceiptNextRetryShown`: exhausted bucket renders `{at | "unknown", retries_used, retries_max: 3}`; never-exhausted renders null; an invented time fails. Fails before: member absent.
  - `TestCliUsageNotices` (CLI): a run emits exactly the `usage:`/`reserve:`/`remaining: unknown`/`next retry:` lines, with a never-exhausted run rendering `next retry: none (used 0 of 3)`; a missing line or a hard-limit phrase (`limit`, `cap of`, balance) fails. Command: `go test ./internal/cli/ -run TestCliUsageNotices`. Fails before: lines absent.
  - `TestNoHardLimitAnywhere`: golden receipt + golden CLI output contain no hard-limit phrasing; the check anchors to the receipt `billing` object and the notice lines (not file-wide). Fails before: members absent.
- **Test plan**: Golden JSON receipt plus golden notice lines; TUI packages untouched (no TUI files in this task).
- **Invariants touched**: I09 (v2 §7.3: labels preserved to the surface; unknown never zero, AC-1.4/AC-7.1); I10 (v2 §2: no advertised hard limit, O3).

### Task 11 — AT-13 end-to-end and NFR-1 run-path latency

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 4, Task 5 (consumes the `usage-counters`/`allowance-exhausted` scenarios), Task 8, Task 10
- **Change**: Prove AT-13 from the packaged binary on the S1 route and pin the 1 s run-path ledger bound, so the slice's acceptance case is exercised, not asserted.
- **Files**:
  - `tests/e2e/ledger_s1_test.go` (new; AT-13 matrix + exhaustion + latency)
- **Acceptance**:
  - `TestE2ELedgerNormalizesCounters`: packaged run with `usage-counters` scenario records per-attempt deltas accumulated with no double counting (cumulative-baseline behavior stays pinned at unit level by Task 2); journal rows match the expected typed set exactly. Command: `go test ./tests/e2e/ -run TestE2ELedgerNormalizesCounters`. Fails before: rows absent.
  - `TestE2EExhaustionPreservesWithoutFallback`: `allowance-exhausted` run ends `blocked`, candidates intact, exactly one native launch, and none of the paid-continuation markers (`purchased credits`, `paid summary`, `upgrade`, `overage`, `metered`, `billed`) appears in the receipt or CLI outputs (one `grep -q` per marker against the receipt file and the captured CLI output, each exiting non-zero; both outputs asserted non-empty first, so vacuous absence fails; Task 10 goldens are the mechanism). Fails before: path absent.
  - `TestE2ECoupledReservationPresent`: the admitted run holds exactly one reservation coupled to its bucket; zero or two fails. Fails before: hold absent.
  - `TestRunPathLedgerLatency`: admission + record + read complete within 1 s total; a variant with 1.5 s injected delay exceeds the bound and fails, proving discrimination (MH-10 `TestRegistryLookupLatency` shape). Fails before: ledger absent.
- **Test plan**: Build `cmd/mythhelm` once per package run; temp `MYTHHELM_HOME`; fake adapter only (no credentials, no `live` tag).
- **Invariants touched**: I02 (v2 §7.3: coupled quota exercised); I09 (v2 §7.3: normalization exercised); G05 via AT-13 (v2 §18.3, S1 route).

### Task 12 — Ledger user documentation and decision record

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 7, Task 8, Task 10
- **Change**: Document the S1 ledger, envelopes and exhaustion behavior in the user guide and record the billing/persistence/process-ownership decisions, so users and reviewers can tell what S1 proves.
- **Files**:
  - `docs/user-guide.md` (ledger/envelopes subsection under the existing `## Billing` section)
  - `docs/decisions/NNNN-budget-ledger-s1.md` (next free ADR number at implementation; billing, persistence and process-ownership decisions D1–D3, D6, D13, D14)
- **Acceptance**:
  - `sed -n '/^## Billing/,/^## /p' docs/user-guide.md | grep -q "usage_observations"`: the Billing section documents typed usage rows. Exits non-zero before the change.
  - The same section range contains `--envelope-repairs` and `estimate, not a charge` and `remaining: unknown` (three `grep -q` checks, each non-zero before).
  - `set -- docs/decisions/[0-9][0-9][0-9][0-9]-budget-ledger-s1.md; test "$#" -eq 1 && test -f "$1" && grep -q "Status: accepted" "$1" && grep -q "0003_ledger" "$1" && grep -q "run owner" "$1"`: the ADR records the migration, the owned reservations table and the run-owner sole-writer path. Exits non-zero before (file absent).
  - No documented sentence advertises a hard spending or token limit: `sed -n '/^## Billing/,/^## /p' docs/user-guide.md | grep -qiE "hard (limit|cap)|spending limit|token limit"` exits non-zero, and a planted hard-limit sentence fails the check (verified by temporary insertion during review, not committed).
- **Test plan**: No Go test asserts prose; each item is a recorded shell command with a failing counterfactual (absent section before the change).
- **Invariants touched**: None (docs only; Tasks 7, 8 and 10 keep I21/I02/I09/I10 — this task describes, not changes, behavior).
