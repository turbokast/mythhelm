## Budget Ledger S1 — Design

> S1 slice of MH-16: typed usage observations, counter normalization, coupled
> quota reservations, finite run envelopes, completion reserves and exhaustion
> without paid fallback, for one-agent delivery. A slice of v2 §§7.3, 10.3.
> Normative source: mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md, version 2.0;
> § numbers, I-IDs, G-IDs and AT-IDs refer to it.

## 0. Scope and sequencing

- S1 only: one agent, one qualified route, one quota bucket. Multi-profile
  (MH-17), protected experiments (MH-27), parallel writers (S3) and
  `metered-allowed` are non-goals (requirements N1–N4).
- Prerequisite, shipped: MH-10 (`specs/*/qualification-registry/`) —
  `qualify.Datum`/`DatumLabel` vocabulary this design builds on.
- MH-21 is the plan `specs/*/v2-contracts-supervisor/plan.md` with stream
  specs in `in-progress/` — explicitly not a prerequisite. This spec's migration
  is `0003_ledger.sql` (next free number while only 0001/0002 exist) with
  run-owner writes (OQ-1 option (a), OQ-4 as answered from ADR-0005), but
  two peers claim 0003 as well (`specs/*/supervisor-service/`'s
  `0003_supervisor.sql` with its own `reservations` table, and MH-13's
  `0003_evaluator.sql`): landing order is coordinated by the renumber rule
  in tasks.md Dependencies (Task 3 re-checks at start; whoever lands second
  renumbers; only one `reservations` DDL lands and the other side adopts).
  Shared files with MH-13 (`specs/*/contained-execution-profiles/`, PR #228,
  landing alongside in `todo/`) are disjoint by region except `journal.go`:
  `admission.go` (here: Request/Decision envelope fields + Decide
  pass-through; MH-13: Decide defaults/consult, disclosure,
  `Proposal.Overrides` append), `run.go` (here: `--envelope-*` flags +
  finish notices; MH-13: help/consent/disclosure), `pipeline.go` (here:
  admission/attempt/conclude gates; MH-13: boundary/inspect enforcement and
  evaluator wiring — second landing rebases), `receipt.go` (disjoint
  members: here `billing`, MH-13 `execution_bundle.boundary`), `worker.go`
  (here: outcome mapping; MH-13: pre-exec re-hash); `journal.go` bumps
  `SchemaVersion` on both sides, so the second landing rebases under the
  renumber rule.
- `specs/*/supervised-stop-recover/` also lands after this spec: it cannot
  start before its own unshipped prerequisites (the supervisor-service
  stream) ship, while this spec is
  implementable now. Ordering: MH-16 S1 first; stop-recover rebases onto it
  (disjoint regions — MH-16 touches admission/attempt/conclude gates and
  outcome mapping; stop-recover touches the spawn site, nonce/ladder and
  reconnect; see tasks.md Dependencies).
- Held: MH-12 Task 7 (maintainer live run). The exhaustion signal ships
  fixture-tested (OQ-2 option (a)); live qualification follows MH-12 T7.
- Downstream: MH-22 (`specs/*/protected-acceptance/`) reads this spec's
  receipt fields, ledger tables and the one new control event
  `run.extension_granted` (§11); it starts after this spec ships.

## 1. Current state

All claims below were opened; the `grep` beside each line re-finds it.

- No ledger exists. `internal/` holds 14 packages
  (`ls internal/`: adapter, admission, buildinfo, cli, ids, integration,
  journal, qualify, security, statedir, supervisor, tui, workers, workspace)
  and none is a billing/ledger package. `mythhelm.db` has 11 tables
  (`internal/journal/migrations/0001_init.sql` + `0002_qualification.sql`),
  none for usage or reservations.
- Provenance vocabulary exists: `qualify.DatumLabel` with five values and
  `Datum{Quantity, Label, Unit, Scope, Source, At}`
  (`internal/qualify/qualify.go:39-94`;
  `grep -n "DatumLabel\|type Datum" internal/qualify/qualify.go`).
- Admission consults qualification: `ResolveQualification` admits strict only
  when live-qualified with proven entitlement and a supported
  `stop_at_exhaustion` marker; unknown quota alone never blocks
  (`internal/admission/qualify.go:88-142`;
  `grep -n "func ResolveQualification" internal/admission/qualify.go`).
  The unknown-quota guard is pinned by
  `TestUnknownQuotaAdmitsStopAtExhaustion`
  (`internal/admission/qualify_test.go:537`).
- Native usage arrives only as an untyped journal payload. The worker writes
  `attempt.native_result` with `usage_native_reported` (per-model token map)
  and `retail_equivalent_estimate_usd`, null when unreported
  (`internal/workers/worker.go:828-843`;
  `grep -n "usage_native_reported" internal/workers/worker.go`). Per-model
  shape is `adapter.TokenUsage{Input, Output, CacheRead, CacheCreation}`,
  all `*int64`, nil = unreported (`internal/adapter/adapter.go:305-310`;
  `grep -rn "type TokenUsage" internal/adapter/`). The claudecode decoder
  fills it from stream-json (`adapters/claudecode/decode.go:354-399`;
  `grep -n "func decodeUsage" adapters/claudecode/decode.go`) and keeps cost
  as an exact decimal literal (`decode.go:338-352`, `decodeCost`;
  `grep -n "func decodeCost" adapters/claudecode/decode.go`).
- The first route reports `UsageTokens: Supported`, `QuotaRemaining: Unknown`,
  `HardMonetaryLimit: Unsupported`
  (`adapters/claudecode/probe.go:139`;
  `grep -n "QuotaRemaining" adapters/claudecode/probe.go`).
- The receipt renders the estimate with its label and nothing else:
  `price` gains `native-reported` only when the event value is present, else
  stays `unknown` (`internal/supervisor/receipt.go:115-123`;
  `grep -n "retail_equivalent_estimate_usd" internal/supervisor/receipt.go`);
  it reads the harness ID from the journaled admission record
  (`receipt.go:168`, `adm.Adapter.Harness`).
- Native errors classify to a fixed list including `billing_error` and
  `rate_limit` (`adapters/claudecode/decode.go:42-47`, `errorClasses`); the
  worker maps lone `rate_limit` to attempt reason `provider_limit`
  (`internal/workers/worker.go:801-803`) and anything else non-rate-limit to
  its class (`worker.go:793-796`). No `allowance_exhausted` class exists
  (`grep -rn "allowance_exhausted" internal/ adapters/ cmd/ --include=*.go`
  returns 0 hits).
- Run machine: `executing → blocked` exists and `blocked` is terminal with a
  mandatory reason (`internal/supervisor/state.go:41-55,96-101`;
  `grep -n "runTransitions" internal/supervisor/state.go`). The owner lock is
  run-scoped and its holder is the sole writer (`internal/supervisor/ownerlock.go:23-25`,
  `AcquireOwner`; ADR-0005).
- The run pipeline carries one attempt under one owner lock
  (`internal/supervisor/pipeline.go:95`, `Run`; attempt dispatch at
  `pipeline.go:269`, `p.attempt`; admission at `pipeline.go:204`,
  `appendAdmission`). Launch intents journal through
  `RecordLaunchIntent` (`internal/supervisor/state.go:226`). CLI output flows
  through the `renderer` (`internal/cli/render.go:26-31`) fed by
  `supervisor.Hooks{Event, Notice}` (`internal/supervisor/pipeline.go:61-70`).
  Recovery reconciles through `RecoverWithHooks`
  (`internal/supervisor/recover.go:40`), journaling e.g.
  `recovery.ownership_resolved` (`recover.go:139`).
- Worker transport has no native retry loop; it counts `adapter.Retry`
  observations (`internal/workers/worker.go:663-664,866-893`,
  `progress.retries`).
- `mythhelm.toml` admits a strict config subset with `Digest`-bound trust
  (`internal/admission/projectconfig.go:38-50`, `ProjectConfig`); check
  timeouts parse via `CheckConfig.Duration`
  (`projectconfig.go:60`).
- Migrations are additive and embedded; open probes the version first
  (`internal/journal/journal.go:116`, `probeVersion` call;
  `grep -n "probeVersion(ctx" internal/journal/journal.go`; `SchemaVersion`
  at `journal.go:30`, currently 2). `Append` (`journal.go:281`) takes a
  `project func(*sql.Tx) error` for same-tx projections. `*Journal` has no
  general transaction helper: its 22 methods across `journal.go`,
  `projections.go`, `declarations.go` and `qualification.go` are `Append`
  (with its per-call projector), readers, type-specific inserts and
  lifecycle — `grep -rn "func (j \*Journal)" internal/journal/
  --include=\*.go | grep -v _test` returns 22 hits, none a
  `Transact`-shaped helper — so Task 3 adds `Transact`.
- Fake-adapter scenarios are embedded JSON `steps`
  (`adapters/fake/agent.go:43-44`, `type scenario` / `Steps`;
  `grep -n "Steps \[\]step" adapters/fake/agent.go`); the embedded set is
  pinned by `TestScenariosEmbedded`
  (`adapters/fake/fake_test.go:532`).

## 2. Package and schema

New package `internal/billing` (the Revision 1.1 §19.1 layout names
`billing`; `knowledge/domains.md` resolves `internal/` to core /
`go-implementer`). Pure types, normalization, envelope arithmetic and reserve
estimates live there; row access follows the `declarations.go` precedent
(`InsertX` on `*sql.Tx`, readers on `*Journal`) in
`internal/journal/ledger.go`. The run owner (ADR-0005) is the sole writer;
all writes below run inside `Append` projections or the new `Transact`.

Migration `internal/journal/migrations/0003_ledger.sql` (additive;
`SchemaVersion` 2→3 in `journal.go`):

```sql
CREATE TABLE usage_observations (
  observation_id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES runs(run_id),
  scope TEXT NOT NULL, unit TEXT NOT NULL, source TEXT NOT NULL,
  label TEXT NOT NULL CHECK (label IN ('reported','observed','estimated','user-declared','unknown')),
  quantity TEXT NOT NULL CHECK (quantity <> ''), -- decimal text, or exactly 'unknown'; never '' and never 0-for-unknown
  producer_id TEXT, producer_sequence INTEGER,
  observed_at TEXT NOT NULL
) STRICT;
CREATE TABLE run_envelopes (
  run_id TEXT PRIMARY KEY REFERENCES runs(run_id),
  execution_seconds INTEGER NOT NULL, repairs INTEGER NOT NULL,
  replans INTEGER NOT NULL, transport_retries INTEGER NOT NULL,
  first_start_at TEXT,
  repairs_used INTEGER NOT NULL DEFAULT 0, replans_used INTEGER NOT NULL DEFAULT 0,
  transport_retries_seen INTEGER NOT NULL DEFAULT 0,
  pause_spans TEXT NOT NULL DEFAULT '[]', -- JSON array of {requested_at, quiesced_at, resumed_at}
  updated_at TEXT NOT NULL
) STRICT;
CREATE TABLE reservations (
  reservation_id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES runs(run_id),
  bucket TEXT NOT NULL, scope TEXT NOT NULL, owner TEXT NOT NULL,
  quantity TEXT NOT NULL CHECK (quantity <> ''), -- decimal text or exactly 'unknown'
  status TEXT NOT NULL CHECK (status IN ('held','released','orphaned')), -- adopted MH-21 tables additionally admit 'expired', which this spec reads past but never writes
  expires_at TEXT NOT NULL, heartbeat_at TEXT,
  release_evidence TEXT,
  created_at TEXT NOT NULL
) STRICT;
CREATE TABLE bucket_state (
  bucket TEXT PRIMARY KEY, -- QuotaBucket (§4): harness, surface, entitlement class, identity ref
  exhausted_at TEXT NOT NULL, reset_at TEXT, -- NULL reset stays unknown, never invented
  retries_used INTEGER NOT NULL DEFAULT 0
) STRICT;
```

Row types (exact, in `internal/journal/ledger.go`; fields mirror the DDL,
times RFC3339Nano text as in `formatTime`):

```go
type UsageRow struct {
    ObservationID string
    RunID string
    Scope, Unit, Source string
    Label string // one of reported|observed|estimated|user-declared|unknown
    Quantity string // decimal text or exactly "unknown"
    ProducerID string
    ProducerSequence int64
    ObservedAt string
}
type EnvelopeRow struct {
    RunID string
    ExecutionSeconds, Repairs, Replans, TransportRetries int64
    FirstStartAt string
    RepairsUsed, ReplansUsed, TransportRetriesSeen int64
    PauseSpans string // JSON array, default "[]"
    UpdatedAt string
}
type ReservationRow struct {
    ReservationID string
    RunID string
    Bucket, Scope, Owner string
    Quantity string // decimal text or exactly "unknown"
    Status string // held|released|orphaned
    ExpiresAt string
    HeartbeatAt string
    ReleaseEvidence string
    CreatedAt string
}
type BucketRow struct {
    Bucket string
    ExhaustedAt string
    ResetAt *string // nil stays unknown, never invented
    RetriesUsed int64
}
```

Row access (exact, consumed by Tasks 3, 4, 6–10). Tx-taking writers run
inside `Append` projections; `Transact` covers the rest. Failure cases:
duplicate id → the SQLite constraint error wrapped; empty quantity or
empty/out-of-scale label → error naming the field, never a default that
allows (I02); `BucketState` on an unknown bucket → `sql.ErrNoRows`. Null
mapping: the DDL-nullable columns (`producer_id`, `producer_sequence`,
`heartbeat_at`, `release_evidence`, `first_start_at`) map to the plain Go
fields via `sql.Null*` scan intermediates and `nullable()`-style writes
(`""`/`0` ↔ NULL), following `projections.go:327-338` and
`journal.go:381`; only `ResetAt` stays `*string`, where nil means unknown
reset (AC-6.2, shown):

```go
func (j *Journal) Transact(ctx context.Context, fn func(*sql.Tx) error) error
func InsertUsageObservation(ctx context.Context, tx *sql.Tx, o UsageRow) error
func (j *Journal) UsageObservations(ctx context.Context, runID string) ([]UsageRow, error)
func UsageObservationsTx(ctx context.Context, tx *sql.Tx, runID string) ([]UsageRow, error)
func UpsertRunEnvelope(ctx context.Context, tx *sql.Tx, e EnvelopeRow) error
func (j *Journal) RunEnvelope(ctx context.Context, runID string) (EnvelopeRow, error)
func InsertReservation(ctx context.Context, tx *sql.Tx, r ReservationRow) error
func (j *Journal) Reservation(ctx context.Context, reservationID string) (ReservationRow, error)
func SetReservationReleased(ctx context.Context, tx *sql.Tx, reservationID, evidence string, at time.Time) error
func TouchReservation(ctx context.Context, tx *sql.Tx, reservationID string, at time.Time) error
func MarkReservationOrphaned(ctx context.Context, tx *sql.Tx, reservationID string) error
func SetBucketExhausted(ctx context.Context, tx *sql.Tx, bucket, exhaustedAt string, resetAt *string) error
func (j *Journal) BucketState(ctx context.Context, bucket string) (BucketRow, error)
func NoteBucketRetry(ctx context.Context, tx *sql.Tx, bucket string) error
func ClearBucket(ctx context.Context, tx *sql.Tx, bucket string) error
```

## 3. Usage observations and normalization (FR-1, FR-2)

`internal/billing/billing.go`:

```go
// Reading is one ingested counter value. Exactly one of Cumulative / Delta
// is set; nil members are unreported, never zero (I09). Quantities are
// exact decimal text — the same representation as qualify.Datum.Quantity,
// the decoder's CostUSD literal (decode.go:338-352) and the §2 storage
// TEXT — so a USD estimate such as 0.37 passes through verbatim. Token
// counts arrive as integer decimal text, converted losslessly from
// adapter.TokenUsage's *int64 fields at the ingest seam; non-decimal
// quantity text is malformed.
type Reading struct {
    Scope, Unit, Source string
    At time.Time
    Cumulative, Delta *string
    Producer string
    Sequence int64
    HasIdentity bool
}
type ScopeTotal struct {
    Total *string // exact decimal text; nil renders as "unknown"
    Label qualify.DatumLabel
    Components map[string]*string // output, cache_read, cache_creation, reasoning; nil member = unknown
}
// ScopeKey is the full reading identity: totals never merge across units
// or sources, so two identities sharing one scope stay distinct entries.
type ScopeKey struct{ Scope, Unit, Source string }
type Normalized struct{ Scopes map[ScopeKey]ScopeTotal }

// Normalize applies AC-2.1: match on (scope, unit, source); newest cumulative
// is the baseline; apply only unapplied deltas with At strictly after the
// baseline's At — deltas at or before it are already covered and skipped,
// so no delta is counted twice. With no cumulative for an identity, the
// unapplied identity-carrying deltas sum from zero. One total per matched
// identity. Deltas carry (producer, sequence) identity: already-applied
// ones are skipped; identity-less deltas land as Estimated, never summed
// exact — any identity-less delta among an identity's inputs makes that
// identity's total Estimated (the exact decimal sum keeps the estimated
// label). expected names the scopes the caller requires; an expected scope
// with no readings marks unknown under the key {scope, "", ""} and keeps
// the rest (AC-2.2). Deltas sum with exact decimal addition —
// scale-aligned integer arithmetic, never float64 — and a baseline
// cumulative passes its literal through unchanged. Labels derive per
// total: retail-equivalent totals are Estimated, AC-2.2 markers are
// unknown, any other total touched by an identity-less delta is
// Estimated, and all-identified totals are Reported; observed and
// user-declared have no S1 inputs. A reading with an empty scope, unit or source, with
// both/neither of Cumulative/Delta set, or with non-decimal quantity text,
// returns ErrReadingShape naming the reading index.
func Normalize(rs []Reading, expected []string, applied map[EventID]bool) (Normalized, error)
type EventID struct{ Producer string; Sequence int64 }
var ErrReadingShape = errors.New("billing: malformed reading")
```

Component split (AC-2.3): `func SplitUsage(route string, u adapter.TokenUsage) ScopeTotal`.
Only routes with qualified per-field evidence get a mapping row; S1 ships one
row, keyed by the adapter descriptor harness ID, mapping the decoded
`input/output/cache_read/cache_creation` fields to non-overlapping
components. Every other route records the combined total only with all
components nil (unknown). The caller is Task 3's `native_result` projection;
it passes `adm.Adapter.Harness` from the journaled admission record (the
same field the receipt reads at `receipt.go:168`), so the key Task 3 passes
is exactly the key Task 2's row is filed under. `SplitUsage` converts the
`*int64` token fields to integer decimal text losslessly (nil stays
nil/unknown); no float64 appears on the path. The native-overlap caveat
(v2 §7.3: cache/reasoning/output may overlap) is why unmapped stays combined.

Ingest seam: `projectWorkerEvent` (`internal/supervisor/ingest.go:155`)
handles only `state_changed`/`launched`/`native_session` today;
`native_result` is spooled (allowlist `ingest.go:33-37`) but unprojected.
Task 3 adds the `native_result` case: on it, build `Delta` `Reading`s —
never `Cumulative`: the worker emits one `native_result` per attempt (the
single `evNativeResult` emit site, `worker.go:727`, inside `conclude`'s
terminalWritten-guarded path), and the native `total_cost_usd` and
per-model token counts each describe that attempt's own invocation, so
every projected quantity is that attempt's increment — from
`usage_native_reported` (one reading per model key: scope is the model
name, unit `tokens`, source `native-reported`, integer decimal text) and
`retail_equivalent_estimate_usd` (scope `retail-equivalent`, unit `USD`,
source `native-reported`, carrying the native `CostUSD` decimal literal
verbatim with its unit preserved through storage and receipts),
`Normalize` with the expected scope set (the payload's model keys plus
`retail-equivalent`, which is always expected), and insert rows in the
same `Append` transaction. Distinct attempts accumulate: same-identity
deltas from different attempts (`wrk_<attemptID>` producers keep them
distinct) sum exactly; nothing replaces. Every projected reading sets
`At` to the event's `observed_at` and carries `HasIdentity` with the
event's (`ProducerID`, `ProducerSequence`) (`spool.go:40-41,67,83-84`,
`journal.go:86-87,91`); Task 3 builds `applied` from the run's existing
rows via `UsageObservationsTx`, skipping empty-producer rows, so any
re-presented delta is dropped. Replayed spool lines never re-project
anyway: `Append` ignores already-journaled `event_id`s without running
the projector (`journal.go:292-299`; `ingest.go:54-56`). A null/absent
quantity yields no `Reading` and its scope goes `unknown` by AC-2.2,
never 0 (I09); usage unattributable to a model never reaches the
projection (flat aggregates stay unreported in the decoder), so no scope
is expected for it and none is recorded — S1 does not guess attribution.
`unknown` markers persist as rows with quantity exactly `'unknown'` and
NULL producer. Identity-less `Reading`s reach `Normalize` only from
direct callers, never from an event projection. AC-1.3: no
cross-bucket aggregation anywhere — one row per (scope, unit, source);
the receipt renders rows, never a summed figure.

## 4. Reservations (FR-3)

`internal/admission/reserve.go`:

```go
// QuotaBucket names the admitted billing bucket: harness, surface,
// entitlement class and hashed identity ref, joined unambiguously.
func QuotaBucket(rec qualify.Record, identityRef string) string

// Reserver holds quota reservations behind an interface declared where
// consumed (go-conventions), so tests use a fake.
type Reserver interface {
    // Reserve holds one quota reservation for runID on bucket. A second live
    // hold for the same run returns ErrDuplicateHold; storage failure wraps.
    Reserve(ctx context.Context, bucket, owner string) (reservationID string, err error)
}
var ErrDuplicateHold = errors.New("run already holds a reservation")

// NewJournalReserver returns the production Reserver writing the §2
// reservations table through the run owner. expires_at = hold time +
// execution ceiling + 1h grace, from the resolved Ceilings the caller
// passes in.
func NewJournalReserver(j *journal.Journal, c billing.Ceilings) Reserver

// HoldQuotaReservation holds the run's typed reservation coupled to the
// admitted bucket. Called from pipeline.appendAdmission right after the
// admission event journals; failure blocks the run (I02).
func HoldQuotaReservation(ctx context.Context, r Reserver, runID string, rec qualify.Record, identityRef string) (string, error)
```

Ceiling source (the resolved-ceilings chain): `Decide` carries both
envelope layers on the `Decision` without resolving (Task 6:
`Decision.EnvelopeFlags` plus `Decision.ProjectConfig.Envelopes`, §5), and
`appendAdmission` — which holds `p.d` (`pipeline.go:147-148,205`) —
resolves via `billing.ResolveCeilings`, performs the initial
`UpsertRunEnvelope` in the admission transaction, then holds the
reservation with the resolved ceilings. Task 4 owns all three calls, in
that order; failure of any blocks the run (I02).

S1 holds one quota reservation per run inside the admission transaction, so
partial claims cannot exist (AC-3.2 is met structurally for the one-item
set; the acceptance test asserts no row survives a failed admission).
Release: `SetReservationReleased` runs only after the attempt reaches a
terminal state (conclude path, Task 8). Heartbeat: `TouchReservation` runs
in the `native_result` projection (Task 8's ingest edit). Expiry (AC-3.4):
reservations are never reused across runs — a new hold always mints a new
id — and `RecoverWithHooks` marks rows of unreconciled attempts `orphaned`
after reconciliation, before any new hold on that run (Task 8 names the
call site). AC-3.3 wording: reservation surfaces say
`local coordination only — not provider availability`; a golden test pins
the sentence.

## 5. Envelopes (FR-4)

`internal/billing/envelope.go`:

```go
type Ceilings struct{ Execution time.Duration; Repairs, Replans, TransportRetries int }

// BuiltInCeilings is the AC-4.1 fallback: 30 min, 3 repairs, 2 replans, 5 transport retries.
func BuiltInCeilings() Ceilings

// ResolveCeilings applies explicit run flags > run configuration > built-ins.
// A nil flags/file struct, or a negative field, means unset for that layer.
func ResolveCeilings(flags, file *Ceilings) Ceilings

type PauseSpan struct{ RequestedAt, QuiescedAt, ResumedAt time.Time }

// Deadline computes the execution deadline: first start + ceiling, excluding
// only quiescent paused spans (AC-4.3). A still-running turn keeps consuming.
func Deadline(firstStart time.Time, c Ceilings, pauses []PauseSpan, now time.Time) (deadline time.Time, expired bool)

type AttemptCounts struct{ Repairs, Replans, TransportRetries int }
func (c Ceilings) Check(counts AttemptCounts) error // ErrBudgetExhausted naming the kind
```

Configuration surface (the "run-profile configuration" AC-4.1 leaves to
`/spec`): the admitted `mythhelm.toml` gains an optional table, decoded
strictly (unknown keys fail, v2 §4.2; invalid duration or negative count
fails) and digest-bound by the existing trust path —

```toml
[envelopes]
execution = "30m"   # Go duration string, must be positive and finite
repairs = 3
replans = 2
transport_retries = 5
```

— with explicit run flags overriding per field:
`--envelope-execution`, `--envelope-repairs`, `--envelope-replans`,
`--envelope-transport-retries` (kebab-case, matching `--task-file` /
`--execution-profile` in `internal/cli/run.go:27-30`). No MH-13 keys are
involved: MH-13 owns trust profiles, not envelope knobs.

Threading (Task 6): `Request.EnvelopeFlags *billing.Ceilings` (nil when no
flag given) flows `run.go` → `Decide` → `Decision.EnvelopeFlags`; the file
layer flows `ProjectConfig.Envelopes` (`admission.EnvelopesConfig`, strict
TOML decode) → `Decision.ProjectConfig`. `Decide` passes both through
without resolving — resolution plus the initial `UpsertRunEnvelope` happen
in `appendAdmission` (Task 4, §4).
`EnvelopesConfig.ToCeilings() (*billing.Ceilings, error)` converts the file
layer; an invalid duration or negative count errors, never defaults (I02).
Migration: when the unified config v2 (§15.4) lands, `[envelopes]` maps
field-for-field onto `[execution]` and the S1 table is removed (D5).

Counting rules against the existing tables:

- Attempt N>1 of a run is a **repair** if a `verifications` row exists for
  the run (a check suite ran), else a **replan**. Both read from existing
  projections; no new classification store.
- **Transport retries** are the ingested `attempt.progress` `retries`
  counters (worker counts `adapter.Retry` at `worker.go:663-664`); the
  ingest projection accumulates them into
  `run_envelopes.transport_retries_seen`.
- The **deadline** starts at first attempt launch. The supervisor derives a
  child context with the remaining budget for native execution; expiry runs
  the existing stop ladder and the run becomes `blocked` with reason
  `envelope_deadline_exceeded`.

The D8 rule is exported for Task 9's reserve hook, which must tell replans
from repairs before `GateLaunch` runs (§6 gate order):

```go
// IsReplan reports whether the run's next attempt is a material replan:
// true iff no `verifications` row exists for the run (D8). Journal read
// failure returns an error; the pipeline hook blocks the launch (I02).
func IsReplan(ctx context.Context, j *journal.Journal, runID string) (bool, error)
```

Enforcement seam: `pipeline.attempt` (`pipeline.go:269`) checks
`Ceilings.Check` plus the deadline before `RecordLaunchIntent`; a refusal
transitions the run to `blocked` with a specific reason
(`envelope_repairs_exhausted`, `envelope_replans_exhausted`,
`envelope_transport_retries_exhausted`, `envelope_deadline_exceeded`) instead
of journaling the intent (AC-4.2):

```go
// GateLaunch refuses an over-ceiling or past-deadline launch with the
// specific blocked reason. Journal failures wrap; a granted extension
// (CheckExtension true) passes.
func GateLaunch(ctx context.Context, j *journal.Journal, runID string, c Ceilings) error
```

Extending a run past a ceiling requires a recorded user decision (AC-4.2;
no standing-grant table exists in S1 — see honesty register). The operator
resumes via `mythhelm recover`, which journals one control event:

```go
// run.extension_granted payload: {kind, raised_to, decided_by}, with
// raised_to in the envelope row's native unit: seconds of execution for
// kind `execution` (the new `execution_seconds`), counts for `repairs`,
// `replans` and `transport_retries`.
// RequestExtension journals it and raises the run's envelope row, in one
// Transact call. Unknown kind, or a run not blocked for that kind, errors.
func RequestExtension(ctx context.Context, j *journal.Journal, runID, kind string, raisedTo int64, decidedBy string) error

// CheckExtension reports whether a run.extension_granted event for kind
// postdates the run's block. Journal read failure returns an error (fail
// closed: no grant is assumed).
func CheckExtension(ctx context.Context, j *journal.Journal, runID, kind string) (bool, error)
```

This is the spec's only new journaled event type; downstream obligation:
MH-22 must add this single type to its event allowlist when it specifies one (§11).

## 6. Completion reserve (FR-5)

`internal/billing/reserve.go`:

```go
// ReserveEstimate quantifies one verification pass plus the repair ceiling.
// It is estimated data, never provider quota (I10).
type ReserveEstimate struct {
    VerifyPassChecks int
    VerifyTimeoutSum time.Duration // sum of admitted CheckConfig.Duration bounds
    RepairCeiling int
    Note string // always "estimate, not a reserve of provider quota"
}
// CheckBound is billing's own input type: billing must not import admission
// (admission imports billing for Ceilings), so the caller converts.
type CheckBound struct {
    Name string
    Timeout time.Duration
}
func EstimateReserve(checks []CheckBound, c Ceilings) ReserveEstimate

type ExecutionRemainder struct{ TimeLeft time.Duration }

// RemainderCoversReserve is the exact gate predicate: the remainder covers
// the reserve iff TimeLeft >= VerifyTimeoutSum. Counts are deliberately
// absent: repairs are completion work drawn from the reserve itself, so
// gating them on the full repair ceiling would allow at most one repair
// (ceiling 3: the second repair sees RepairsLeft 2 < 3 and always blocks),
// contradicting AC-4.1; per-kind counts are enforced by GateLaunch (§5),
// and replans consume no repairs, so the repair ceiling needs no
// replan-gate check. The repair component of the reserve is attempt slots,
// not execution time: dispatch preserves the slots for GateLaunch, while
// repair execution draws from the shared execution deadline — a repair
// with no time left blocks with envelope_deadline_exceeded. S1 sets no
// per-repair time budget (no grounded bound exists); AC-5.1 protects
// repair slots plus verification time, never repair execution time.
func RemainderCoversReserve(rem ExecutionRemainder, est ReserveEstimate) bool
```

This answers refinement OQ-3's "`/spec` quantifies the pass": one
verification pass = one execution of the admitted check list within its
configured timeouts. S1 optional work = material replans after the first
verification (D14); repairs are completion work drawn from the reserve and
proceed under GateLaunch counts plus the deadline. Before dispatching a
replan — classified by Task 7's `IsReplan` (D8) — the supervisor
evaluates `EstimateReserve` against the remaining envelope; when
`RemainderCoversReserve` is false, the run becomes `blocked` with
reason `completion_reserve_shortfall` instead of starting work it could not
finish verifying (AC-5.1). Task 9's pipeline hook converts the admitted
`[]admission.CheckConfig` to `[]CheckBound` via `CheckConfig.Duration()`;
a conversion failure blocks the launch (I02). Gate order on the replan path is reserve first,
then `GateLaunch`, so a short replan reports `completion_reserve_shortfall`
while an over-count replan still reports `envelope_replans_exhausted`.
A dispatched replan runs under a derived deadline — the execution deadline
minus the estimated verification reserve (`VerifyTimeoutSum`), i.e. now +
TimeLeft − reserve at dispatch — so the replan cannot consume the time the
preflight reserved; Task 9's pipeline hook derives it and Task 7's deadline
context enforces it. Repair execution itself is not time-reserved: repairs
run in the shared execution pool and block with
`envelope_deadline_exceeded` when it is spent, even with repair slots left.
Where provider quota cannot be reserved — always
in S1 — the reserve appears in receipts and CLI output only through
`ReserveEstimate` with its `Note`, and stop-at-exhaustion remains the
protection (AC-5.2).

## 7. Exhaustion (FR-6, FR-7)

Signal (OQ-2 option (a), fixture-qualified): the claudecode decoder gains
the class `allowance_exhausted` for documented native shapes meaning
"allowance exhausted, retry at reset" (new entries in `errorClasses`,
`decode.go:42-47`, with synthetic fixtures under
`adapters/claudecode/testdata/` marked synthetic until MH-12 T7 replaces
them). Classification fails closed: only documented shapes map to
exhaustion; `rate_limit` keeps its transient `provider_limit` mapping and
everything else keeps its class. A reset timestamp is recorded only when the
native signal carries one (authoritative reset); otherwise reset stays
unknown.

Supervisor path (`internal/supervisor/exhaustion.go`):

```go
// RetrySchedule: at most 3 retries at an authoritative reset; unknown reset
// backs off 1m, 5m, 15m, then giveUp (AC-6.2). Pure:NFR-free, clock passed in.
func RetrySchedule(resetAt *time.Time, retriesUsed int) (wait time.Duration, giveUp bool)

// EvaluateBucket enforces the schedule at re-admission: admit iff now has
// reached the next scheduled time (reset_at, or exhausted_at +
// backoff[retries_used]); otherwise refuse with the wait remaining. giveUp
// is terminal only after 3 consumed retries with unknown reset.
func EvaluateBucket(row journal.BucketRow, now time.Time) (admit bool, wait time.Duration, giveUp bool)
```

S1 performs no automatic retries — no scheduler daemon exists in the
one-agent tree, so there is nothing to wake a run (see D13 and the honesty
register). The schedule is recorded in `bucket_state`, shown on the receipt
and CLI (§8), and enforced at re-admission: an operator re-admission while
the bucket row is live calls `EvaluateBucket`; admit → `NoteBucketRetry`
and clear-if-consumed (`ClearBucket` when 3 retries are consumed or
`now >= reset_at`); refuse → code `allowance_exhausted` with the next time.
Clearing is safe: if the bucket is still exhausted, the native signal
re-blocks on the next attempt. `retries_used` increments only on admitted
re-admissions during the exhausted window — never on refusals — so the
exactly-once autonomous-launch pins (Tasks 8, 11) and the schedule agree:
MYTHHELM itself launches nothing more; the operator spends the ≤3 scheduled
retries.

Hook (exact): the conclude path observes the terminal attempt reason; on
`allowance_exhausted` it calls, after the existing freeze:

```go
// HandleExhaustion records bucket_state, transitions the run to blocked
// with reason allowance_exhausted, and releases the run's reservation only
// after the attempt is terminal (AC-3.4). Freeze must already have run;
// journal failure returns an error and journals nothing further.
func HandleExhaustion(ctx context.Context, j *journal.Journal, p *Producer, runID, reservationID string, resetAt *time.Time) error
```

On exhaustion the worker maps the class to attempt reason
`allowance_exhausted`; candidates are preserved through the existing freeze
path (`freezeCandidate`, `pipeline.go:391`); the run becomes `blocked`;
the admission path refuses new model work on that bucket while the row
is live (Task 8's bucket check in the hold path, `reserve.go`; AC-6.1). AC-6.4: S1 preauthorises no alternative route, so the clause
constrains future admission only; a pinned profile waits (I04 re-admission
already required for any profile change). AC-6.3 (no paid fallback) holds
structurally — no code path purchases, upgrades, rotates identity or
changes harness — and is pinned by an e2e test: an exhaustion-scenario run
ends `blocked` with candidates intact and launches no second native
execution.

AC-7.1/7.2: unknown remaining renders as `unknown` in receipts and CLI
output (never `0` or a balance); unknown alone never blocks a qualified
stop-at-exhaustion route (`TestUnknownQuotaAdmitsStopAtExhaustion`,
`qualify_test.go:537`, cited as the guard in Task 4).

## 8. Receipt and CLI surfaces

Receipt `billing` object gains four members (Task 10):

- `usage_observations`: the run's `usage_observations` rows as
  `{scope, unit, source, label, quantity, observed_at}`; retail estimates
  keep `source: native-reported` and `note: estimate, not a charge`
  (existing `receipt.go:115` shape extended, not replaced).
- `reserve`: the `ReserveEstimate` with its `Note`.
- `remaining`: always `unknown` in S1 (AC-7.1), with the bucket identity.
- `next_retry`: the AC-6.2 schedule as shown data — `{at | "unknown",
  retries_used, retries_max: 3}`; absent (null) when the bucket was never
  exhausted.

CLI run output: at the end of `run`, `finish` (`run.go:179`) emits
`Notice` lines (plain renderer prints them; JSONL carries them as stderr
diagnostics, so no new envelope event type is introduced for display):

```text
usage: <scope> <quantity> <unit> (<label>, <source>)
reserve: verify pass (<n> checks, <timeout sum>) + <m> repairs (estimate, not a reserve of provider quota)
remaining: unknown
next retry: <at | unknown | none> (used <k> of 3)
```

A never-exhausted bucket renders `next retry: none (used 0 of 3)` (the
receipt nulls `next_retry`; the CLI line is always present). No surface
advertises a hard spending or token limit: golden tests pin the
receipt and the notice lines, and fail on any `limit`, `cap of`, or
remaining-balance phrasing. TUI packages are untouched in S1 (AC-1.4).

## 9. Errors and exit codes

```go
// internal/billing/billing.go:
// S1-local codes with the exact v2 §4.5 strings; MH-21's catalogue adopts
// these strings when it lands (D12).
type Code string
const (
    CodeAllowanceExhausted Code = "allowance_exhausted"
    CodeBudgetExhausted    Code = "budget_exhausted"
)
var ErrAllowanceExhausted = errors.New(string(CodeAllowanceExhausted))
var ErrBudgetExhausted = errors.New(string(CodeBudgetExhausted))
```

Mapping reuses the existing outcome path (`runExit`,
`internal/cli/exit.go:98`): exhaustion/blocked outcomes exit 3
(`ExitBlocked`, `exit.go:23`), matching `receiptExit`
(`receipt.go:239-240`). No new exit code.

## 10. Tests and CI

- Unit: table-driven per package (`Normalize` matrices incl. identity-less
  deltas, missing scopes, unmapped components, malformed readings,
  decimal-exact totals, deltas-from-zero sums, derived labels;
  `ResolveCeilings` layer precedence incl. negative-field-unset; `Deadline`
  with quiescent vs still-running spans; `RetrySchedule`/`EvaluateBucket`
  reset/unknown paths incl. clearing; `RemainderCoversReserve` boundary).
- Fixtures: synthetic native exhaustion frames
  (`adapters/claudecode/testdata/`, marked synthetic); fake-adapter
  scenarios `usage-counters.json` and `allowance-exhausted.json`
  (`adapters/fake/scenarios/`, JSON `steps` per
  `adapters/fake/agent.go:43-44`).
- E2E (packaged binary, temp `MYTHHELM_HOME`, fake adapter): AT-13 —
  per-attempt delta/missing counters normalize without double counting
  (cumulative-baseline pinned at unit level);
  coupled quota and exhaustion preserve work without paid fallback
  (candidates intact, no second native launch, run `blocked`).
- NFR-1: run-path ledger ops (admit + record + read) complete within 1 s
  total, mirroring MH-10's `TestRegistryLookupLatency` with an injected-delay
  variant proving the test discriminates.
- NFR-2: rows live in `mythhelm.db` via `0003_ledger.sql` only; `0001_init.sql`
  byte-identical (SHA test in Task 3).
- Gates per task: `gofmt -w` on touched files, then `go vet ./...`,
  `go test -race ./...`, `go mod tidy -diff`, `golangci-lint run`
  (`.claude/skills/quality-gates/SKILL.md`); OS-specific files add the
  cross-`GOOS` vets (none expected).

## 11. Downstream contract (MH-22)

MH-22 (`specs/*/protected-acceptance/`, planned — see requirements
Dependencies) reads without reimplementing: `usage_observations` /
`run_envelopes` / `reservations` / `bucket_state` tables, the §2 readers,
receipt `billing` members (`usage_observations`, `reserve`, `remaining`,
`next_retry`), the recorded execution bounds, and the one new control
event `run.extension_granted` (a downstream obligation on its event
allowlist when it specifies one). MH-21 adopts the §2 tables and
§9 code strings when it lands.

## 12. Decisions

| ID | Decision | Rationale |
|---|---|---|
| D1 | New package `internal/billing` for ledger types, normalization, envelopes, reserves | Revision 1.1 §19.1 names `billing`; one responsibility per package (go-conventions). Rejected: `internal/ledger` (unnamed by the layout) and stuffing types into `journal` (mixes storage with billing semantics). Billing/persistence/process-ownership impact recorded in the Task 12 ADR. |
| D2 | OQ-1 → option (a): additive `0003_ledger.sql` now, coordinated by renumber rule | Next free number while only 0001/0002 exist; MH-21 stream 2 and MH-13 claim 0003 too, so tasks.md Dependencies coordinates (Task 3 re-checks at start; second landing renumbers; single `reservations` DDL adopted by the other side). Rejected: waiting for MH-21 (unbounded stall) and a hard uncoordinated 0003 (landing order would silently break a peer). |
| D3 | Ledger writes by the run owner (ADR-0005), MH-21 adopts the tables later | The OQ-4 answer stands: no service exists, so the owner lock is the sole-writer discipline (v2 §5.1's requirement, S1's mechanism). Rejected: inventing a service inside this spec (MH-21's scope). |
| D4 | OQ-2 → option (a): fixture-qualified exhaustion taxonomy in this spec | MH-12 Task 7 (live evidence) is held, so option (b) cannot supply proof. Fixtures are marked synthetic per the adapter rule; live qualification follows MH-12 T7 (honesty register). Rejected: blocking S1 on the live run. |
| D5 | AC-4.1 middle layer = admitted `mythhelm.toml [envelopes]` table | Concrete, digest-bound by the existing trust path (`projectconfig.go`), strictly decoded. Considered: v2 §15.4 `[execution]` keys (`run_deadline_seconds`, `max_repair_attempts`, `max_replans`, `max_transport_retries`) — rejected for S1 because the unified config v2 is proposed-only ("The old parser cannot consume it") and the admitted `ProjectConfig` subset carries no `[execution]` table; `[envelopes]` is the admitted-subset slice. Migration: when config v2 lands, `[envelopes]` maps field-for-field onto `[execution]` (`execution`→`run_deadline_seconds`, `repairs`→`max_repair_attempts`, `replans`→`max_replans`, `transport_retries`→`max_transport_retries`) and the S1 table is removed. Rejected: MH-13 profile keys (MH-13 owns trust profiles and is alongside, not prerequisite) and flags-only (would leave AC-4.1's middle layer unmet). |
| D6 | S1 built-ins 30m/3/2/5 per AC-4.1 against v2 §10.3's 2h/2/2/3 | AC-4.1 is the ground truth for this slice; execution is tighter (30m < 2h), repairs looser (3 > 2), replans equal, transport looser (5 > 3) and counted per run where v2 budgets per operation — v2 §10.3 permits explicit configurable ceilings, so the slice is compliant either way. Recorded here, not in the honesty register. |
| D7 | One verification pass = one execution of the admitted check list within its configured timeouts | Quantifies refinement OQ-3 using `CheckConfig.Duration` (`projectconfig.go:60`); time bound = sum of check timeouts. Rejected: wall-clock constant (ignores the admitted suite). |
| D8 | Attempt N>1 is a repair iff a `verifications` row exists for the run, else a replan | Derivable from existing projections with no new classification store. Rejected: trigger-string plumbing through the worker (crosses the spool contract for metadata the journal already has). |
| D9 | Transport-retries ceiling counts ingested `attempt.progress` retries | The worker counts `adapter.Retry` (`worker.go:663-664`) but runs no native retry loop; counting observations enforces the ceiling without inventing a loop. Rejected: worker-side retry budget (no loop exists to budget). |
| D10 | Exhaustion classification fails closed; `rate_limit` keeps its transient mapping | Only documented shapes mean exhaustion. Rejected: mapping all provider limits to exhaustion (would schedule reset-retries for throttles) and the reverse (would fail runs that should wait). |
| D11 | CLI usage lines ride `Notice`, not new journaled events; one control event for extension | Display lines stay out of the journaled envelope set; the extension decision needs durability, so `run.extension_granted` is journaled (downstream obligation on MH-22's event allowlist). Rejected: new event types for display (allowlist churn) and undocumented extension (AC-4.2 requires a recorded decision). |
| D12 | Error codes defined S1-local with exact v2 §4.5 strings | No catalogue package exists in the tree; MH-21 adopts these strings when it lands. Rejected: blocking on MH-21's catalogue and parallel string codes (drift). |
| D13 | S1 records and enforces the retry schedule; retries are operator-executed, never automatic | No scheduler daemon exists in the one-agent tree, so nothing could wake a run; AC-6.2's schedule is recorded, shown (§8) and enforced at re-admission (§7). Rejected: inventing a background scheduler inside this spec (MH-21/service scope) and silent auto-retry (would contradict the exactly-once launch pins). |
| D14 | Reserve gate covers material replans only; repairs are completion work | Gating repairs on the full repair ceiling allows at most one repair (the ceiling-3 trace in §6), contradicting AC-4.1; v2 §10.3's reserve is verify-plus-repairs (repairs as attempt slots, not execution time) *before optional work*, and in S1 only a new-direction replan is optional. Rejected: gating every post-verification attempt (the contradiction) and a count-based replan predicate (replans consume no repairs; GateLaunch already enforces the replan ceiling). |

## 13. Honesty register

| Spec demand | Position |
|---|---|
| v2 §7.3 exhaustion taxonomy, live-qualified | Partially met: fixture-tested in this spec (D4); live qualification follows MH-12 Task 7. Fixtures marked synthetic (I14). |
| v2 §10.3 AC-4.3 pause exclusion with a live pause control | Partially met: `Deadline` implements and unit-tests the quiescent-only exclusion rule, but S1 ships no user pause command, so no live exclusion occurs. The rule is specified for the pause mechanism when it lands. |
| v2 §10.3 automatic retry execution at reset | Partially met: the AC-6.2 schedule is recorded, shown and enforced at re-admission, but retries are operator-executed; no automatic launcher exists in S1 (D13). |
| v2 §10.3 extension by standing grant (AC-4.2 half) | Partially met: extension requires the recorded user decision (`run.extension_granted`); no standing-grant table exists until MH-21. |
| v2 §10.3 separate stop/reconciliation safety deadline | Partially met: S1 reuses the existing stop ladder with no separate reconciliation deadline; reconciliation runs inside the attempt lifecycle (§7, `RecoverWithHooks`), and the gap is recorded here, not closed. |
| v2 §7.3 `metered-allowed` | Deferred per N3; stop-at-exhaustion only. |
| v2 multi-profile handoff / experiments / parallel writers | Deferred per N1, N2, N4 (MH-17, MH-27, S3). |
| v2 §10.3 per-route component semantics beyond the first route | Partially met: mapping row only where qualified evidence exists (S1: the first route); all other routes record combined totals with unknown components (AC-2.3). |
| G05 billing honesty (AT-03, AT-04, AT-13) | AT-13 exercised for the S1 route in Task 11; AT-03/AT-04 rest on MH-10/MH-12 qualification evidence, consumed not re-proven. |
