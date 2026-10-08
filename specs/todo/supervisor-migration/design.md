# Supervisor Migration — Design

Normative source: mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md, version 2.0.
Consumes `specs/*/v2-contract-vocabulary/design.md` (cited `v2c§N`) and
`specs/*/supervisor-service/design.md` (cited `svc§N`) verbatim.
Citations below are exact signatures from those specs; nothing here redefines
a record type, error code, envelope, limit, or control interface.

## 1. Current state

- Journal schema is v1: `0001_init.sql` holds 10 tables (`grep -n 'CREATE TABLE'
  internal/journal/migrations/0001_init.sql` → runs, attempts, journal,
  producers, candidates, verifications, check_results, trust_grants,
  declarations, applies); `0002_qualification.sql` adds `qualification_records`
  only — the additive-0002 precedent this spec follows.
- Stream 2 (prerequisite, ships first) adds `0003_supervisor.sql` with
  `operations` (idempotency ledger) and `reservations` tables (svc§2, svc D2).
  This spec's migrations sequence after 0003 and touch no v1 table.
- `Append` (`internal/journal/journal.go:281-345`) implements idempotent
  `event_id`, contiguity and generation fencing, but hard-rejects any envelope
  with `schema_version != 1` (`journal.go:368`). Stream 1 decided OQ-9 (v2c D7):
  the same `journal` table stores both versions; v2 is accepted post-migration.
- `Open` (`journal.go:108-170`) is probe-first and read-only: a database newer
  than supported yields `ErrSchemaTooNew` (`journal.go:43`) without writing.
  Pragmas already WAL + `synchronous(FULL)` with `busy_timeout(5000)`
  (`journal.go:38`); backup uses `VACUUM INTO` off-transaction (`journal.go:226`).
- Per-run owners at 4 non-test sites (`grep -rn 'AcquireOwner' internal cmd
  --include='*.go' | grep -v _test`): `pipeline.go:130`, `recover.go:47`,
  `cli/apply.go:67`, `cli/tui.go:185`. Each must drain, adopt or quarantine.
- Receipt-file `schema_version = 2` (`internal/supervisor/apply.go:195`) is a
  separate namespace from the envelope; import must never treat a receipt as a
  contract version.
- Stream-1 vocabulary this spec consumes (exact signatures in `v2c§2–§6`):
  `v2contract.TaskRevision`, `PolicyVersion`, `Grant`/`EffectIntent`,
  `Reservation`, `Envelope` (+`Validate`), `Decode[T Validator]`, `Digest`,
  `CheckSequence`/`CheckDuplicate`/`CheckGeneration` with `ErrDuplicateEvent`/
  `ErrSequenceGap`/`ErrStaleGeneration`, `Code` catalogue (incl.
  `schema_too_new`, `revision_conflict`, `ownership_unresolved`,
  `persistence_unavailable`, `permission_denied`, `invalid_contract`),
  `ControlError` (+`Validate`), `MapAdapterFailure`, lifecycle
  `CheckRunTransition`/`CheckTaskTransition`/`CheckAttemptTransition`,
  `CheckTerminalEntry`, `CheckReconcileIdentity`.
- Stream-2 service this spec consumes (exact signatures in `svc§3–§9`):
  `control.AcquireInstance(dir)`, `ErrInstanceHeld`, `ErrRootConflict`,
  `Transport`/`Listener`/`Conn`, `ErrNoSupervisor`, `Intent`/`Result`/
  `Handler`/`Execute`, `Mutate(ctx, db, fn)`, `Reserve`/`Release`/`Heartbeat`,
  `Filter`, `CheckIngress`, `Peer`.

## 2. Package and migrations (v2 §5.2; AC-7.1; OQ-1)

New package `internal/migrate` (D1): legacy importer, drain orchestrator,
backup/restore, plus hermetic subpackage `internal/migrate/preview` holding
the preview planner. `internal/migrate` calls `control.Mutate` for every write
and never opens SQLite itself outside the supervisor's transaction path;
`preview` imports neither `internal/control` nor network packages.

Migration `internal/journal/migrations/0004_v2contracts.sql` (D2): new tables
only — `tasks`, `task_revisions`, `policies`, `grants`, `migration_state` —
storing stream-1 records verbatim as canonical JSON (`Digest`-addressed) plus
index columns (`task_id`, `revision`, `run_id`). No v1 table is altered; v1
rows stay for decode (AC-7.5). `migration_state` holds one row:
`{phase, started_at, backup_path, schema_version, build_version}`.

```go
// Phase is the durable migration phase in migration_state.
type Phase string

const (
    PhaseNotStarted Phase = "not_started"
    PhasePreviewed  Phase = "previewed"
    PhaseDrained    Phase = "drained"
    PhaseImported   Phase = "imported"
    PhaseAdopted    Phase = "adopted" // service owns the ledger
)
```

## 3. Legacy import (v2 §5.2; AC-7.2, AC-7.5)

Each legacy v1 run imports as one v2 task with revision 1: `TaskRevision`
fields derive from the v1 run row (goal, deliverable, write scope from the
admitted bundle); original run/attempt IDs are preserved verbatim as the v2
`RunID`/`AttemptID`; v1 journal rows are untouched and keep decoding under v1.
Billing posture copies the v1 declaration word-for-word:
`subscription-declared` imports as declared, never verified (I15); legacy
`ready_for_review` with unverified evidence imports as a `candidate` TaskState,
never `accepted` (I07, I09).

```go
// ImportOptions selects the runs to import; empty RunIDs means all v1 runs.
type ImportOptions struct {
    RunIDs []string
}

// ImportRun imports one legacy run as a one-task v2 run inside the caller's
// Mutate transaction. Failure cases: revision_conflict (run already
// imported); invalid_contract (v1 row fails v1 decode — never reinterpreted,
// AC-7.5); persistence_unavailable (ledger I/O). It writes the v2 rows and
// the import marker, and appends a v2 Envelope recording the import.
func ImportRun(ctx context.Context, tx *sql.Tx, runID string) (v2contract.TaskRevision, error)

// ImportResult reports one imported run for the preview/receipt.
type ImportResult struct {
    RunID    string `json:"run_id"`
    TaskID   string `json:"task_id"`
    Revision int    `json:"revision"`
    Posture  string `json:"posture"` // copied declaration word, e.g. "subscription-declared"
}
```

v1 decode is preserved structurally: the v1 event scan path keeps its
`schema_version == 1` decoder, and a test decodes a golden v1 journal (with a
`Decision` and every v1 status word) before and after migration, byte-identical
projections.

## 4. Drain, adopt, quarantine (v2 §5.2; AC-7.3)

Migration holds an exclusive migration lock for its whole run: it reuses
`control.AcquireInstance` semantics — while migration runs, the service writer
does not start, and new legacy admissions are refused (the admission path
checks `migration_state.phase` and returns `ownership_unresolved` once past
`previewed`). The 4 `AcquireOwner` sites drain in dependency order:

1. `cli/apply.go:67`, `cli/tui.go:185` (interactive writers): refused first —
   apply during migration returns `ownership_unresolved` naming the phase.
2. `pipeline.go:130` (run execution): in-flight runs finish their admitted
   episode or stop at the next checkpoint; no new run starts.
3. `recover.go:47` (recovery): runs after the others; unreconciled ownership
   quarantines rather than relaunches (I12 — adopt-only-same-launch-identity;
   a dead worker's launch identity is never reused by a new launch).

```go
// DrainReport names every owner site and its outcome.
type DrainReport struct {
    Drained     []string `json:"drained"`     // run IDs that finished cleanly
    Adopted     []string `json:"adopted"`     // run IDs adopted same-launch-identity
    Quarantined []string `json:"quarantined"` // run IDs quarantined with evidence refs
}

// Drain stops new admissions and drains the 4 legacy owner sites.
// Failure cases: ownership_unresolved (a site still holds a lock past its
// deadline — migration aborts, nothing half-adopted); persistence_unavailable.
func Drain(ctx context.Context, db *sql.DB) (DrainReport, error)
```

Quarantine representation (D4): a v2 `Envelope` of type
`migration.quarantined` per run, carrying the run ID, the last known launch
identity and the evidence refs — ledger-native, so stream 4 reconciles it
with the same machinery as any quarantined attempt. No service writer starts
until `Drain` returns with zero held locks; a test asserts this by holding a
run lock during migration and observing refusal.

## 5. Backup, restore, downgrade refusal (v2 §5.2; AC-7.4)

Backup reuses the `VACUUM INTO` mechanism (`journal.go:226`): a full,
consistent copy written before any migration write, beside a sidecar JSON file
recording schema version, build version, digest and timestamp. Restore copies
back and re-opens read-only first; the restore procedure is tested end to end
(write v1 fixture → backup → migrate → restore → byte-identical v1
projections). Downgrade refusal extends the existing `ErrSchemaTooNew` path:
`Open` already refuses newer schemas without writing (`journal.go:108-170`);
migration additionally refuses to run when `migration_state` names a newer
schema or build, returning `schema_too_new` (default disposition
`after_user_action`: upgrade the binary).

```go
// BackupInfo is the sidecar recorded beside every pre-migration backup.
type BackupInfo struct {
    Path          string `json:"path"`
    SchemaVersion int    `json:"schema_version"`
    BuildVersion  string `json:"build_version"`
    SHA256        string `json:"sha256"`
    CreatedAt     string `json:"created_at"`
}

// Backup writes a VACUUM INTO copy plus sidecar; it writes nothing else.
// Failure cases: persistence_unavailable (I/O); invalid_contract (target
// path already holds a backup — never overwritten).
func Backup(ctx context.Context, db *sql.DB, dir string) (BackupInfo, error)

// Restore copies a backup into place after verifying sidecar digest and
// schema/build identities. Failure cases: schema_too_new (backup newer than
// this binary — nothing written); invalid_contract (digest mismatch).
func Restore(ctx context.Context, info BackupInfo, dir string) error
```

## 6. Migrate command and preview (OQ-2; AC-7.6)

OQ-2 decided (D3): explicit `mythhelm migrate` with a preview step. `migrate
--preview` prints the plan (runs to import with postures, owner sites to
drain, schema steps 0003→0004, backup target) as plain text and `--format
jsonl`; it opens the database read-only, takes no agent credentials, performs
no network I/O, and writes nothing — the NFR-4 hermeticity half stream 1
deferred here (v2c honesty register). `migrate --apply` runs
Backup → Drain → Import → Adopt (phase `adopted`, service starts owning the
ledger) and prints a receipt. `--apply` without a prior `--preview` in the
same invocation previews first and requires `--yes` to proceed.

```go
// Plan is the hermetic preview: everything --apply would do, computed
// read-only.
type Plan struct {
    Runs     []ImportResult `json:"runs"`
    Owners   []string       `json:"owners"` // owner sites to drain
    Steps    []string       `json:"steps"`  // schema steps, e.g. "0004_v2contracts.sql"
    BackupTo string         `json:"backup_to"`
}

// PreviewPlan computes the migration plan from a read-only handle.
// Failure cases: schema_too_new (database newer than this binary).
// It must not take credentials, touch the network, or write.
func PreviewPlan(ctx context.Context, db *sql.DB) (Plan, error)

// Apply executes Backup → Drain → Import → Adopt. Failure cases: as the
// step functions; on step failure the phase row names the failed step and
// --apply is resumable (completed steps are idempotent by import markers
// and lock state, never by replaying effects, I12).
func Apply(ctx context.Context, db *sql.DB, plan preview.Plan) (DrainReport, error)
```

`Plan` and `PreviewPlan` live in `internal/migrate/preview`, which imports
neither `internal/control` nor network packages; `TestPreviewHermetic`
asserts this with `go list -deps` on that subpackage (`go list -deps` is
package-granular, so the planner cannot share `internal/migrate` with the
`Mutate`-calling steps). `Apply` stays in `internal/migrate` and consumes
`preview.Plan`.

## 7. Append v2 acceptance (v2 §4.3; OQ-9)

Implements stream 1's OQ-9 decision (v2c D7): `Append` accepts envelopes with
`schema_version == 2` post-migration (phase `imported` or later), validating
through the stream-1 pure validators in `Append`'s check order — `event_id`
duplicate (`CheckDuplicate`), generation (`CheckGeneration`), sequence
(`CheckSequence`) — then allocating `run_sequence` as before. Pre-migration,
`schema_version == 2` is still rejected (unknown critical payloads cannot be
silently stored). Late observations on old generations retain as quarantined
evidence at most (AC-4.2 behavior, enforced at this call site).

No signature change: `Append` keeps its signature; a duplicate `event_id` acks
with nil error and no append (`ErrDuplicateEvent` is not a failure, per v2c
`CheckDuplicate`). The remaining v2 validation errors map to catalogue codes
at the `Append` boundary, where `Append` wraps each stream-1 sentinel into a
`ControlError` whose code name matches the sentinel (`invalid_contract`,
`ownership_unresolved` for generation quarantine decisions). There is no
ID-reuse conflict at `Append` — duplicates ack nil, so `revision_conflict`
for ID-reuse-with-different-args surfaces only at the caller layer
(`Execute`, import markers).

## 8. NFR-2 SQLite posture (AT-42; OQ-8)

- WAL with `synchronous=FULL` and `_txlock=immediate` continue from
  `journal.go:38`; busy timeout stays `5000` ms (named here as the design-time
  value — unchanged, now pinned by a test asserting the pragma string).
- One-transaction-per-mutation scope is `control.Mutate` (svc§7): preconditions
  → append event → update projections → enqueue outbox; no network I/O inside.
  Every ledger-mutating `migrate` write path runs inside it (pinned by Task
  7's `TestMigrateWritesRunInMutate`); preview hermeticity is pinned
  separately by the `preview`-subpackage deps test (Task 6), since
  `internal/migrate` itself imports `control`. `Backup`'s single `VACUUM
  INTO` `Exec` is excluded from the `Mutate` scope: it creates a file copy
  off-transaction (`VACUUM INTO` cannot run inside a transaction, §5) and
  mutates no ledger row.
- Integrity checks: `PRAGMA integrity_check` runs after backup (source deemed
  healthy before copying) and after restore (copy verified before opening for
  write); failure aborts with `persistence_unavailable`.
- OQ-8: a test asserts `SELECT sqlite_version()` from the built binary meets
  the WAL-reset fix floor (≥3.51.3 or a documented backport pinned in
  `scratchpad.md` by the implementing task; floor sourced from
  `mythhelm-synthesis/MYTHHELM_Synthesis_Decisions.md` §8.1, SRC-16 row).
  Until that test passes, no S1 claim is made; the OQ-8 default (unverified)
  stands.

## 9. Tests and CI (NFR-3)

- Unit + golden tests in `internal/migrate/*_test.go` and
  `internal/journal/*_test.go`: v1 golden journal decodes byte-identically
  pre/post migration; import golden (`subscription-declared` stays unverified,
  `ready_for_review` → candidate); lock-held refusal; digest-mismatch restore
  refusal; newer-schema refusal without write (file SHA-256 unchanged).
- Migration e2e against the packaged binary: fixture v1 state dir → `migrate
  --preview` (exit 0, writes nothing — dir hash unchanged) → `migrate --apply
  --yes` → v2 assertions → `restore` → v1 projections byte-identical.
- Platform tests per advertised OS/arch with `CGO_ENABLED=0` (lock behavior,
  backup/restore file semantics); Unix + Windows runners.
- Gates: `gofmt -l .`, `go vet ./...` (plus `GOOS=windows`/`GOOS=darwin` vet
  for platform files), `go test -race ./...`, `go mod tidy -diff`,
  `scripts/ci/check-public-hygiene.sh`.

## 10. Decisions

| ID | Decision | Rationale |
|---|---|---|
| D1 | New package `internal/migrate` for preview/import/drain/backup orchestration | Migration is a bounded, one-shot responsibility with its own CLI surface; folding it into `control` would mix the steady-state service with the transition that retires the legacy path (domains.md: a package appears when a task gives it a working responsibility). |
| D2 | OQ-1: new tables in `0004_v2contracts.sql` (default (a)) | Matches v2 §5.2 ("additive" + "preserve v1") and the 0002 precedent; v1 rows stay decodable in place. In-place evolution (b) risks silent reinterpretation of v1 rows (AC-7.5) and complicates downgrade refusal. |
| D3 | OQ-2: explicit `mythhelm migrate` with `--preview`/`--apply` (default (a)) | Matches W03 "user-invoked migration"; a first-run prompt (b) would migrate implicitly at the worst moment (mid-run) and cannot show a hermetic preview for consent. |
| D4 | Quarantine as ledger-native `migration.quarantined` v2 envelopes | Stream 4 reconciles quarantined attempts from the ledger; a sidecar file would be a second task store (I23). A marker row was considered; an envelope carries the evidence refs and generation the reconciler needs. |
| D5 | Backup via existing `VACUUM INTO` mechanism + JSON sidecar | The mechanism is already tested (`journal.go:226`); a new copy path would need its own consistency story. The sidecar adds the schema/build identities AC-7.4 needs. |
| D6 | `Append` keeps its signature; v2 acceptance is phased by `migration_state` | Callers (stream 2's `Mutate` path) build against a stable signature; a flag-day boolean would split every caller. Phase-gating keeps pre-migration behavior byte-identical. |
| D7 | ADR for the migration/import approach in this spec | Persistence layout (new v2 tables) plus process-ownership transition (per-run → singleton) require a decision record per the spec-authoring rule; stream 2's ADR covers the steady-state topology, not the transition. |

## 11. Honesty register

| Spec demand | Position |
|---|---|
| AC-7.3 "prevent new legacy admissions" post-migration | Partially met: the legacy per-run path is drained and refused during migration; full removal of the legacy path is a follow-up after stream 4 (the old path must not become permanent — requirements Context). |
| NFR-2 "shipped engine WAL-reset fix verified" (OQ-8) | Blocked until the implementing task asserts `SELECT sqlite_version()` from the built binary; default (unverified) stands, no S1 claim before it. |
| Stop/recover of quarantined runs | Deferred to stream 4 (`supervised-stop-recover`); migration quarantines with evidence, never reconciles. |
| `live-qualified` support-matrix rows | None claimed: nothing here has run against a live multi-client deployment. |
| Reservation migration for in-flight work | None in scope: v1 has no reservations (verified: zero `reservation` hits in `migrations/*.sql`); post-migration work reserves fresh under stream-2 rules. |

## 12. Cross-Spec References

- Epic plan: `specs/*/v2-contracts-supervisor/plan.md` (this spec is stream 3; scope FR-7; NFR-2; OQ-1, OQ-2, OQ-8).
- Depends on `specs/*/v2-contract-vocabulary/` (stream 1): record types, `Envelope`, sequence/generation validators, error catalogue, OQ-9 decision. And on `specs/*/supervisor-service/` (stream 2): the service to migrate to, `internal/control` interfaces, migration `0003_supervisor.sql` tables. No implementation task in this spec starts before streams 1 and 2 ship.
- Parallel with `specs/*/supervised-stop-recover/` (stream 4): disjoint files (journal/migrations + import paths here; worker/supervisor control paths there).
