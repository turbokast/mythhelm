# Qualification Registry — Design

Normative source: mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md, version 2.0.

## 1. Current state

- Strict `--billing subscription-only` always blocks: `ResolveBilling` returns
  `entitlement_qualification_unavailable` (`internal/admission/billing.go:56`;
  grep `entitlement_qualification_unavailable` under `internal/admission/`).
- No versioned registry store exists. Near-misses, all dismissed:
  `CapabilityRecord.Qualification`/`FidelityQualification` are per-probe
  descriptor strings (`internal/adapter/adapter.go:351,357`);
  `entitlement_qualification_unavailable` is a block code
  (`internal/admission/billing.go:56`); `Qualified` bools default false
  (`internal/adapter/adapter.go:143`, `internal/admission/billing.go:52`);
  `actionRegistry` is the TUI palette (`internal/tui/palette.go:35`).
  Grounding: `grep -rin "registr\|qualif" internal/ adapters/ cmd/
  --include='*.go'`.
- The only compatibility record is single-harness Markdown:
  `adapters/claudecode/COMPATIBILITY.md` (`fixture-tested`, no live
  qualification evidence). Probe identity comes from
  `adapters/claudecode/probe.go` → `adapter.Probe{Executable, Version,
  SHA256, OS, Arch, Compatibility}` (`internal/adapter/adapter.go:53`).
- Admission flow: `internal/cli/run.go` → `admission.Decide` (`internal/admission/admission.go:178`) →
  `decideClaudeCode`/`decideFake` (`internal/admission/admission.go:240,273`)
  → `ResolveBilling` (`internal/admission/billing.go:30`). Admission writes
  nothing; the supervisor persists (`internal/admission/admission.go:1-5`).
- Durable state is SQLite via embedded additive migrations
  (`internal/journal/journal.go:54-58`, `internal/journal/migrations/0001_init.sql`),
  `PRAGMA user_version` versioning, `Open` (`internal/journal/journal.go:108`)
  and `OpenReadOnly` (`internal/journal/projections.go:348`). Row pattern: `DeclarationRow` +
  `CurrentDeclaration`/`InsertDeclaration` (`internal/journal/declarations.go`).
  ADR-0003 (`docs/decisions/0003-local-state-sqlite.md`) settles SQLite.
- `doctor` is read-only, plain + JSONL (`internal/cli/doctor.go:36-54`),
  dispatched from `internal/cli/dispatch.go:38`; report built by `diagnose`
  (`internal/cli/doctor.go:70`). It never creates the state dir
  (`internal/cli/doctor.go:170-193`) and never queries auth status
  (`internal/cli/doctor.go:122-124`).

## 2. Record model (v2 §7.1, §4.2; FR-1, FR-2)

New package `internal/qualify` (core domain). One record per harness ×
surface × executable build × OS/arch × entitlement class × trust profile,
keyed exactly per the v2 §7.1 qualification key (AC-1.1).

```go
// Progress is the v2 §4.2 qualification scale.
type Progress string

const (
    ProgressPlanned             Progress = "planned"
    ProgressDocumentedCandidate Progress = "documented-candidate"
    ProgressFixtureTested       Progress = "fixture-tested"
    ProgressLiveQualified       Progress = "live-qualified"
    ProgressExperimental        Progress = "experimental"
    ProgressBlocked             Progress = "blocked"
    ProgressUnsupported         Progress = "unsupported"
)

// Verdict is one column's value (AC-1.2).
type Verdict string

const (
    Proven    Verdict = "proven"
    NotProven Verdict = "not-proven"
    Unknown   Verdict = "unknown"
)

// DatumLabel is the v2 §7.3 data provenance label (AC-5.2).
type DatumLabel string

const (
    Reported     DatumLabel = "reported"
    Observed     DatumLabel = "observed"
    Estimated    DatumLabel = "estimated"
    UserDeclared DatumLabel = "user-declared"
    DatumUnknown DatumLabel = "unknown"
)

// Key is the v2 §7.1 qualification key.
type Key struct {
    Harness           string // v2 §7.2 id: claude-code, codex, opencode, muse, kimi, cursor, antigravity
    Surface           string // e.g. "native-cli-structured"
    ExecutableDigest  string // "sha256:<hex>" of the probed binary
    AdapterProtocol   string // adapter id + protocol, e.g. "builtin/claudecode+stream-json"
    OS                string
    Arch              string
    ProviderEndpoint  string // endpoint class, e.g. "first-party-subscription"; "unknown" when unestablished
    ModelSnapshot     string // observed snapshot or explicitly moving alias; "unknown" when opaque
    EffortSettings    string // native effort/speed settings digest or "none"
    AuthCategory      string // non-secret auth category + account/entitlement class
    ConfigDigest      string // instructions/tools/skills/hooks/plugins/MCP digest, "sha256:<hex>" or "unknown"
    TrustProfile      string // "trusted-host", "restricted", "inspect"
    WorkspaceClass    string // environment class, e.g. "local-checkout"
    EntitlementClass  string // e.g. "included-plan"
}

// Evidence is one versioned support for a column verdict (v2 §13).
type Evidence struct {
    ID          string     // "ev_<opaque>"; display prefix, not authority (v2 §4.2)
    Method      string     // "offline-fixture", "authorised-live", "documented-mechanism", "reported-external"
    Suite       string     // fixture suite or live suite identity; "unknown" when none
    Result      string     // "pass", "fail", "blocked"
    Uncertainty string     // free text; "unknown" when unassessed
    At          time.Time  // RFC3339; zero means unknown, never the epoch
    Expiry      *time.Time // nil means no expiry claimed
    Label       DatumLabel // provenance of this evidence; DatumUnknown when unassessed
    Source      string     // mechanism or suite that produced it; "unknown" when none
}

// Datum is one labeled quantitative datum (AC-5.2, v2 §7.3): every
// quantity carries its label, unit, scope, source and timestamp.
type Datum struct {
    Quantity string     // the value; "unknown" when unestablished, never "" treated as 0
    Label    DatumLabel // reported|observed|estimated|user-declared|unknown
    Unit     string     // e.g. "tokens", "credits"; "unknown" when unestablished
    Scope    string     // what the quantity covers; "unknown" when unestablished
    Source   string     // evidence id or mechanism; "unknown" when none
    At       time.Time  // zero means unknown, never the epoch
}

// Column is one independently-recorded qualification column (AC-1.2).
type Column struct {
    Verdict  Verdict
    Evidence []Evidence // empty means no evidence; verdict must then be Unknown or NotProven
}

// Record is one immutable revision of a qualification record (AC-1.3).
type Record struct {
    SchemaVersion int // 2 for records introduced here (v2 §4.2)
    Revision      int // increases within the key; content identity is Digest
    Digest        string
    Key           Key
    Progress      Progress
    Fidelity      Column
    Entitlement   Column
    Lifecycle     Column // security/lifecycle
    Capabilities  map[string]Capability // v2 §4.2 supported|unsupported|unknown + evidence, scope, expiry
    Quota         Datum   // remaining-allowance quantity for the route (AC-3.3, AC-5.2)
    NextTest      string // the next test that could advance a blocked record + authority it needs (AC-4.3)
    SupersededAt  *time.Time
}

// Capability is one v2 §4.2 capability value with its support.
type Capability struct {
    Value    string // "supported", "unsupported" or "unknown"
    Evidence string // evidence id or "unknown"
    Scope    string
    Expiry   *time.Time
}
```

`Digest` is `sha256:<hex>` over the canonical JSON of the record minus
`Digest`/`SupersededAt`. All structs carry explicit snake_case `json` tags
(`model_snapshot`, `evidence`, …); canonical JSON uses them. Failure case:
constructors return an error naming the offending field when an enum value
is outside its scale; unknown enum input never coerces to a passing value.
Decoding is one named function so every reader shares the defaulting rule:

```go
// DecodeRecord decodes canonical record JSON, applying missing→"unknown"
// defaults recursively (Key fields, Datum fields, Evidence Label/Source,
// Quota). Syntax errors name the offset; out-of-scale enum values name
// the offending field.
func DecodeRecord(data []byte) (Record, error)
```

## 3. Store: migration 0002 and seed (v2 §5.1, ADR-0003; FR-1, FR-2)

Additive migration `internal/journal/migrations/0002_qualification.sql`
(never edit `0001_init.sql`, v2 §5.2):

```sql
CREATE TABLE qualification_records (
    key_hash TEXT NOT NULL,      -- sha256 over canonical Key JSON
    revision INTEGER NOT NULL,
    digest TEXT NOT NULL,
    record_json TEXT NOT NULL,   -- canonical Record JSON, schema_version 2
    recorded_at TEXT NOT NULL,   -- RFC3339Nano
    superseded_at TEXT,          -- NULL while current
    PRIMARY KEY (key_hash, revision)
) STRICT;
CREATE UNIQUE INDEX idx_qualification_current ON qualification_records(key_hash) WHERE superseded_at IS NULL;
```

`STRICT` follows the v1 convention (every table in `0001_init.sql` is
`STRICT`, per ADR-0003).

Journal helpers in `internal/journal/qualification.go` are `*Journal` methods
owning their own transactions (the `db` field is unexported,
`internal/journal/journal.go:98`, and `Journal` exposes no `Begin`; grep
`func (j \*Journal)` under `internal/journal/`). Opaque strings cross the
package boundary so the import direction stays one-way — `qualify` imports
`journal`, never the reverse (no Go import cycle):

```go
func (j *Journal) InsertQualificationRecord(ctx context.Context, keyHash string, revision int, digest, recordJSON string) error
func (j *Journal) CurrentQualificationRecord(ctx context.Context, keyHash string) (recordJSON string, revision int, err error)
func (j *Journal) ListCurrentQualificationRecords(ctx context.Context) (records []string, err error)

// InsertQualificationRecordIfAbsent inserts the row only when no
// (key_hash, revision) row exists, in a single INSERT OR IGNORE
// statement; it never supersedes. It reports whether it inserted.
func (j *Journal) InsertQualificationRecordIfAbsent(ctx context.Context, keyHash string, revision int, digest, recordJSON string) (inserted bool, err error)
```

`Insert-` refuses empty `keyHash`/`digest`/`recordJSON` or `revision < 1`
(`journal: invalid qualification record`) and otherwise supersedes the
previous current row for the key inside its own transaction.
`Insert-IfAbsent` applies the same input validation, then the single
non-superseding write; concurrent same-key callers are serialized by
the statement, exactly one inserting. Digest
verification lives in `qualify`: `Registry.Record` recomputes via
`CanonicalDigest` and refuses tampering (`qualify: record digest
mismatch: ...`). `Current-` returns `ErrNotFound` wrapped as
`journal: qualification record: %w` when absent — absence is not a zero
record (I09). The migration bumps `SchemaVersion` 1→2
(`internal/journal/journal.go:30`); without the bump `OpenReadOnly` rejects
every migrated database with `ErrSchemaTooNew`
(`internal/journal/projections.go:365-367`). Migration failure returns
`applying migration 2: %w` (`internal/journal/journal.go:213`).

Seed: `qualify.SeedV1() []Record` returns the seven v2 §7.2 harness records
for the local platform, each `ProgressBlocked` or `ProgressPlanned` (never
`fixture-tested` or better) with `NextTest` naming the concrete first
evidence step and its authority, in the form `<step>; authority: <who
grants it>`. Seeding runs only through `EnsureSeeded`
— never at import time, never on plain `Open`, never rewriting existing
rows. The fixture-tested first-route
record exists only
behind `RecordDraft` plus its registry round-trip tests (Task 5) until
MH-12: this spec ships no production writer of proven records, so no
production database holds anything above `blocked`/`planned`. The
`COMPATIBILITY.md` fixture facts feed `RecordDraft`, and the Markdown file
stays as the per-harness view.

Production seeding trigger (Task 9): the supervisor calls
`qualify.EnsureSeeded` after `journal.Open` in `Run`
(`internal/supervisor/pipeline.go:109-116`), so the first admitted run seeds
the seven records; refused runs seed nothing. Seeding is idempotent
(inserts only missing rows; a complete table is untouched) and a seed
failure fails the run through the existing
`persistence_unavailable` path — state init is fail-closed. Reads before the
first admitted run honestly show no records (O1's seven are readable from
the first admission on).

```go
// EnsureSeeded inserts the SeedV1() records missing from the table via
// per-key InsertQualificationRecordIfAbsent, then verifies seven current
// rows. It completes partial seeds (a partial failure never blocks a
// later call) and is safe under concurrent first runs (the single
// statement serializes same-key inserts; count verified). Fewer than
// seven afterwards returns the journal error, wrapped.
func EnsureSeeded(ctx context.Context, j *journal.Journal) error
```

## 4. Registry queries and drift (v2 §7.1, §8; FR-4, AC-5.1)

`internal/qualify/registry.go`:

```go
// Open opens the registry over the state dir's mythhelm.db, read-write,
// running pending migrations. It never creates qualification evidence.
func Open(ctx context.Context, dir string) (*Registry, error)

// OpenReadOnly opens without migrating or seeding; used by doctor.
// Failure cases: missing dir → error (qualify: state dir ...: ...);
// dir exists with no database file → an empty registry whose List returns
// []; database file present but version 0 (journal.ErrNoDatabase on an
// existing file) → error, never mistaken for empty; version mismatch or
// unreadable DB → the journal error, wrapped. It stats the dir and the
// database file itself because journal.OpenReadOnly reports ErrNoDatabase
// for a missing dir, a missing file and a version-0 file alike
// (internal/journal/projections.go:350-354, 366-368).
func OpenReadOnly(ctx context.Context, dir string) (*Registry, error)

// ErrSchemaMismatch marks a registry database whose schema this binary
// cannot read (older or newer user_version, or a missing table).
var ErrSchemaMismatch = errors.New("qualify: schema mismatch")

// IsSchemaMismatch reports errors.Is(err, ErrSchemaMismatch).
// OpenReadOnly wraps journal version errors and List wraps a missing
// table with this sentinel.
func IsSchemaMismatch(err error) bool

func (r *Registry) Close() error // Close only; errors are returned, never hidden

// Lookup returns the current record for the exact key, or ErrNotFound.
func (r *Registry) Lookup(ctx context.Context, k Key) (Record, error)

// Consult finds the record for an observed identity: exact key first,
// else the unique stable-identity match (Harness, Surface, OS, Arch,
// TrustProfile, EntitlementClass, AdapterProtocol — pinned digests and
// snapshots ignored) for drift comparison. Zero matches → ErrNotFound;
// several stable matches → error (qualify: ambiguous stable match: ...).
// drifted reports CheckDrift on the matched record.
func (r *Registry) Consult(ctx context.Context, observed Key) (rec Record, drifted bool, reason string, err error)

// IsMissing reports whether err is a missing-dir-or-database condition
// (as opposed to unreadable or version-mismatched state).
func IsMissing(err error) bool

// List returns every current record, ordered by harness then surface.
// A database without the table (foreign or corrupt: migration 2 always
// creates it) → error (qualify: qualification table missing: ...).
func (r *Registry) List(ctx context.Context) ([]Record, error)

// Record stores rec as revision current+1 (1 for a new key), ignoring
// rec.Revision; callers never pick revisions. InvalidateOnDrift likewise
// stores current+1. All key hashes derive through qualify.KeyHash.
// Record rejects a column valued Proven with empty Evidence
// (qualify: proven verdict without evidence: <column>).
// Single-writer: concurrent same-key Records collide on the primary key
// and fail closed; callers must not retry blindly.
func (r *Registry) Record(ctx context.Context, rec Record) error

// Record stores a new revision, superseding the current one for its key.
func (r *Registry) Record(ctx context.Context, rec Record) error

var ErrNotFound = errors.New("qualify: no record for key")
```

Drift (`internal/qualify/drift.go`, AC-4.1/AC-4.2):

```go
// DriftInput is the pinned-vs-observed comparison for one record.
type DriftInput struct {
    Record         Record
    ExecutableDigest string // observed now
    ConfigDigest     string // observed now
}

// CheckDrift reports whether the record's pinned binary digest or relevant
// configuration (effective startup hooks, MCP, plugins, managed config,
// pinned instruction/tool/skill digests) drifted. It writes nothing.
func CheckDrift(in DriftInput) (drifted bool, reason string)

// ConfigDigestOf derives the comparable config digest from a manifest's
// digests map: "sha256:" + hex(sha256(json.Marshal(digests))). Encoding/json
// sorts map keys, so insertion order never affects the digest; this mirrors
// the existing sha256-of-marshaled-map construction at
// adapters/claudecode/settings.go:228-233.
func ConfigDigestOf(digests map[string]string) string

// InvalidateOnDrift stores a new revision with affected columns reset to
// Unknown/NotProven and Progress to blocked, preserving the old evidence
// ids in the new record's Uncertainty text for audit. It ships as tested
// API with no production caller in this spec: production databases hold no
// proven records to invalidate (see §3), and admission writes nothing
// (internal/admission/admission.go:1-5). MH-12, the first live writer,
// wires the production invalidation call and settles the
// restore-without-retest question.
func (r *Registry) InvalidateOnDrift(ctx context.Context, rec Record, reason string) error
```

Admission consult (`internal/admission/qualify.go`, AC-5.1):

```go
// ResolveQualification maps a probed native identity plus the registry to
// an eligibility verdict. Unknown mandatory evidence blocks; unknown
// remaining quota alone does not block an otherwise qualified
// stop-at-exhaustion route (v2 §7.3, I02). A nil registry (missing dir or
// database per IsMissing) counts as absent record; evidence is the
// AuthStatus result, since the consult point follows it. profile is the
// consented execution profile name (d.Profile.Name); the resolver cannot
// see the owning decision, so the caller passes it.
func ResolveQualification(ctx context.Context, reg *qualify.Registry, probe adapter.Probe, manifest adapter.ConfigManifest, evidence claudecode.AuthEvidence, billing string, profile string) (Eligibility, error)

type Eligibility struct {
    Verdict EligibilityVerdict // Eligible, Blocked, Unsupported (v2 §4.2)
    Reason  string             // typed reason code, e.g. "entitlement_not_proven"
    Record  *qualify.Record    // the record consulted, nil when none
}

type EligibilityVerdict string

const (
    Eligible    EligibilityVerdict = "eligible"
    Blocked     EligibilityVerdict = "blocked"
    Unsupported EligibilityVerdict = "unsupported"
)
```

`decideClaudeCode` opens the registry with `qualify.OpenReadOnly`
(`internal/admission/admission.go:307-310` area, after `AuthStatus` at lines
300-306 so provider and account class are known, before `ResolveBilling` at
line 311) and passes `d.Profile.Name` as `profile`. A missing dir or
database (`qualify.IsMissing`) or an unreadable schema
(`qualify.IsSchemaMismatch`: older/newer `user_version`, missing table)
maps to a nil registry, which `ResolveQualification` treats as absent
record — a v1 database genuinely holds no qualification records, so this
is honest, and it lets existing users reach the supervisor, which migrates
and seeds on the admitted run. Permission, IO and corruption errors fail
the admission (`qualify: registry unavailable: %w`). The consult never
migrates or seeds — admission writes nothing. (`doctor`, unlike admission,
reports every `OpenReadOnly`/`List` error as `unavailable`, never as
empty.)

`ResolveQualification` builds the observed `Key` field-by-field:

| Key field | Source |
|---|---|
| Harness | `"claude-code"` (adapter descriptor, `adapters/claudecode/probe.go:45`) |
| Surface | `Descriptor().Surface` (`"native-cli-structured (print, stream-json)"`) |
| ExecutableDigest | `"sha256:" + probe.SHA256` |
| AdapterProtocol | `Descriptor().ID + "+stream-json"` |
| OS, Arch | `probe.OS`, `probe.Arch` |
| ProviderEndpoint | `"first-party-subscription"` when `evidence.APIProvider == "firstParty"`, else `"unknown"` |
| ModelSnapshot | `"unknown"` (vendor identity opaque, v2 §7.1) |
| EffortSettings | `"none"` |
| AuthCategory | `evidence.AuthMethod + "/" + evidence.SubscriptionType` (e.g. `"claude.ai/max"`) |
| ConfigDigest | `qualify.ConfigDigestOf(manifest.Digests)` |
| TrustProfile | `profile` param (the caller passes `d.Profile.Name`, the consented execution profile, `internal/admission/admission.go:195`) |
| WorkspaceClass | `"local-checkout"` |
| EntitlementClass | `"included-plan"` |

At the consult point only `claudecode.Manifest` exists
(`internal/admission/native.go:33`), so the conversion follows the
`admission.go:321` pattern. Matching runs through `Registry.Consult`: exact
key, else the unique stable-identity match for drift comparison. Drifted →
`Blocked` with the drift reason (AC-4.1 path). Evidence with `Expiry` set
and past is ignored (treated as absent); a `proven` verdict with no
unexpired supporting evidence counts as `not-proven` for the consult.
A capability with `Expiry` set and past counts as `unknown` for the
consult, whatever its `Value` claims.
Records with `Progress == unsupported`, or harnesses outside the seven v2
§7.2 ids, map to `Unsupported` (exit 7) under every billing mode — the
declared bypass covers missing or merely non-live records only. Otherwise,
for `subscription-only`: absent record → `Blocked` with
`no_qualification_record` (never eligible by default); ambiguous stable
match (Consult error) → `Blocked` with `ambiguous_qualification_match`;
found but non-live record → `Blocked` with `entitlement_not_proven` (or
the column-specific reason). For `subscription-declared` and
`local-scripted` → `Eligible`, attaching the record when one exists. The stop-at-exhaustion property (AC-3.3) is the
`stop_at_exhaustion` entry of the record's `Capabilities` map (`supported`
plus its evidence); AC-3.3's unknown quantity is `Record.Quota` with
`Quantity`/`Label` unknown. A `Blocked`/`Unsupported` verdict becomes
`*BlockedError` (exit 3 / exit 7 per `Capability`). Note the strict
early return in `Decide` (`internal/admission/admission.go:189-191`):
`subscription-only` returns `entitlement_qualification_unavailable` before
`decideClaudeCode` runs, so the new consult is unreachable for strict billing
in this spec — correct per N1/AC-3.2, and the implementer must preserve it
(the Task 4/7 end-to-end guards pin it). The strict-mode mapping above is
unit-tested directly; MH-12 removes the early return to reach it. Under the
fixtures-only default every record is non-live, so
strict subscription-only keeps blocking exactly as today (AC-3.2); no
existing test's exit code changes. New failure cases: registry unreadable
(non-missing open error) → the admission error (`qualify: registry
unavailable: %w`); missing dir/database → nil registry, treated as absent
record; for `subscription-only`, absent record → `Blocked` with reason
`no_qualification_record`, never eligible by default.

## 5. First-route evidence (v2 §7.3, I15; FR-3)

Task 5 (adapters domain) adds `adapters/claudecode/qualify.go`:

```go
// EntitlementEvidence returns the offline-fixture entitlement evidence for
// the claudecode × print/stream-json surface: every AT-03 route covered,
// no hidden paid auxiliary, paid continuation unknown. It performs no live
// call and reads no secret.
func EntitlementEvidence() []qualify.Evidence

// RecordDraft returns the first-route Record at fixture-tested with the
// evidence attached and NextTest naming the authorised live suite.
// Callers build key per the §4 table (tests use fixture probe values);
// Quota is the unknown Datum (fixtures observe no allowance quantity).
// Refusal predicate: key.Harness == "claude-code" && key.Surface ==
// "native-cli-structured (print, stream-json)", digests ignored; any other
// key returns ErrNotFirstRoute.
var ErrNotFirstRoute = errors.New("claudecode: not the first-route key")

func RecordDraft(key qualify.Key) (qualify.Record, error)
```

The fixture `adapters/claudecode/testdata/qualification/routes.json`
enumerates the AT-03 routes (implementer, planner, reviewer, summary, router,
child, experiment) with the auxiliary-route inventory each covers; each route
entry names its passing condition. `RecordDraft` refuses
(`fmt.Errorf("claudecode: qualify: %w", qualify.ErrNotFound)`-style typed
error) when the key is not the first-route key, so evidence for one surface
can never label another. Recording into the registry happens through
`Registry.Record` (Task 3's API) from a test helper and, later, MH-12's live
path — this spec ships the evidence constructor and the fixture-tested
record, not a live pass (N1).

## 6. User surface (v2 §15, §17; AC-5.3)

`doctor` gains a `qualification` section (default of Q4): `diagnose` in
`internal/cli/doctor.go:70` adds `Qualification map[string]any` to
`doctorReport`, read via `qualify.OpenReadOnly` against the resolved state
dir. An `OpenReadOnly`/`List` error (missing dir, unreadable or
version-mismatched DB, missing table) → `"status": "unavailable (<reason>)"`,
never an error exit and never fabricated rows (I09). An existing but
unseeded dir reads as an empty record list — honestly no records yet, since
doctor never seeds. Plain output shows one line per record:
`<harness> × <surface>: <progress> (fidelity <v>, entitlement <v>,
lifecycle <v>) [evidence <n> revs, latest <ev-id>; drift <state>]`, where
`<state>` is `clean` or the drift reason. JSONL carries, per record,
`progress`, per-column `verdict` + `evidence` ids, `evidence_revisions`
and `drift_triggers`. Plain output adds one
line per record: `<harness> × <surface>: <progress> (fidelity <v>,
entitlement <v>, lifecycle <v>)`. JSONL carries the same map. Doctor stays
read-only: `OpenReadOnly` migrates and seeds nothing (AC-12.2 lineage).

## 7. CLI, errors, tests, CI

- No new subcommand: AC-5.3 rides `doctor`. Exit codes unchanged; new
  `*BlockedError` codes from §4 map through the existing `exit.go` contract
  (policy → 3, capability → 7).
- NFR-1: `TestRegistryLookupLatency` opens a temp-state-dir registry and
  asserts a cold `Lookup` completes in under 1 s (local SQLite read; bound
  is generous, not a benchmark).
- NFR-2: `TestRecordRevisionImmutability` writes two revisions and asserts
  the first row's bytes are unchanged and both digests recompute.
- Live tests: none in this spec (I13, card Notes). Fixtures are synthetic
  until MH-12 replaces them with sanitised authorised recordings.
- CI: existing Go jobs; no workflow change. `tests/e2e` gains packaged-
  binary tests: `doctor --format jsonl` shows seven records on a seeded
  home (Task 7, test-side seeding: display, not trigger); `run --billing
  subscription-only` still exits 3 with `entitlement_qualification_unavailable`;
  fresh home → admitted fake run → `doctor` shows seven with no test-side
  seeding (Task 9: the production trigger).

## 8. Decisions

| ID | Decision | Rationale |
|---|---|---|
| D1 | Records live in `mythhelm.db` (new table), not versioned files | v2 §5.1: the canonical ledger owns orchestration state; ADR-0003 settles SQLite; one writer, one backup story. Files would be a second competing store (I23). Carries ADR-0011 in the docs task. |
| D2 | Lazy Go seed via `EnsureSeeded`, not in migration SQL | Migration SQL must stay deterministic and platform-neutral; the seed depends on `runtime.GOOS/GOARCH` and probe facts. Seeding is idempotent (inserts only missing rows, completing partial seeds), triggered explicitly by the supervisor on admitted runs. |
| D3 | Per-column `proven\|not-proven\|unknown`, separate from the §4.2 progress scale | AC-1.2 verdicts answer "is this column established"; progress answers "how far along is this surface". Conflating them reintroduces the pass-in-one-implies-another error v2 §7.1 forbids. |
| D4 | Drift check is pure (`CheckDrift`); invalidation is an explicit write | Lets admission and tests compare without side effects; every invalidation is a deliberate new revision with its reason preserved (I20). |
| D5 | `doctor` section, not a new `mythhelm qualification` verb | AC-5.3's Q4 default: doctor already owns truthful read-only environment reporting; a verb with write ambitions would need its own admission story. |
| D6 | First-route evidence constructor lives in `adapters/claudecode`, registry stays in `internal/qualify` | Domain purity: harness-specific facts stay behind the adapters seam; the registry consumes `qualify.Evidence` values without importing any adapter (I11, cross-domain review at the seam). |
| D7 | `ResolveQualification` runs before `ResolveBilling`, both consulted | Registry eligibility (route exists and is qualified) precedes billing posture (this run's funding); either can block, and the reason code says which. |
| D8 | No live qualification path in this spec | Card Notes authorise no live usage; v2 §7.1 requires explicit allowance permission. The `authorised-live` method value exists in the type so MH-12 has somewhere to put its evidence. |

## 9. Honesty register

| Spec demand | Position |
|---|---|
| v2 §7.3 included-only admission passing for a route | Not met: every record is non-live under the fixtures-only default; strict admission keeps blocking (AC-3.2). MH-12 earns the first pass. |
| v2 §7.1 live qualification evidence | Not met: only `offline-fixture` and `documented-mechanism` methods ship; `authorised-live` needs Q2's grant. |
| AC-4.1 persisted drift invalidation in production | Deferred: drift blocks at consult time and `InvalidateOnDrift` ships tested, but no production caller persists invalidations — production DBs hold no proven records, and admission writes nothing. MH-12 wires the writer and settles restore-without-retest. |
| Stable matching vs non-drift dimensions | Limitation registered: `Consult`'s stable set drops AuthCategory, ProviderEndpoint, WorkspaceClass, EffortSettings and ModelSnapshot while `CheckDrift` compares digests only, so a drifted probe could match another key's record as clean. Unreachable in-spec (all records non-live; declared paths attach-only). MH-12 must require exact match on non-drift dimensions or extend drift comparison before live use. |
| v2 §7.2 seven-harness parity | Partially met: all seven have records, but only the first route has fixture evidence; the rest are `blocked`/`planned` with next tests. |
| v2 §14 host attachment compatibility records (Herdr) | Deferred to N3: host-integration facts live in `adapter.CapabilityRecord.HostIntegration`; no Herdr evidence is gathered into the registry. |
| AT-03 all-routes pass included-only admission | Partially met: fixture inventory covers the AT-03 route list; no live pass. |
| AT-04 paid-continuation prevention | Partially met: unknown stays blocking; prevention itself is unproven (MH-12). |
| v2 §16 plugin ABI surfaces for qualification | Out of scope: internal Go only (dogfood N5 lineage, I11). |

## 10. Why one spec

Nine tasks, one work stream: the store without the admission consult is
unreadable, the consult without the store has nothing to read, and the
surface without both shows nothing. The adapters evidence task, the seeding
trigger and the docs task consume the core contract but deliver nothing
usable alone, so they are tasks with dependencies, not sub-specs
(`/spec-scope`: single spec, auto-confirmed non-interactive with this
rationale recorded).
