## Qualification Registry — Tasks

### Dependencies

- Prerequisite specs: `specs/*/dogfood-slice/` (done; supplies probe, blocking admission, SQLite journal). None in `todo/` or `in-progress/` at authoring time; re-check at dispatch.
- Order is contract-first: Task 1 ships the `qualify` types every later task builds against. Runnable sets: {2} after 1; {3} after 2; {5, 6} after 3 (disjoint Files); {4} after {3, 5} (Task 4's number precedes its dependency Task 5; the direction consumer→producer is correct); {7} after {4, 5, 6}; {9} after {2, 6}; 8 after {4, 6}. No two tasks share a file except through the stated dependencies.
- **Gates for every task.** `gofmt -w` on touched Go files first, then from the tree root `gofmt -l .` (must print nothing), `go vet ./...`, `go test -race ./...`, `go mod tidy -diff` (must print nothing), and `scripts/ci/check-public-hygiene.sh`. A task is not complete because files exist or an agent reported success; cite the runner output. OS-specific files additionally need `GOOS=windows go vet ./...` and `GOOS=darwin go vet ./...`.
- **Completion convention.** Append ` ✅ COMPLETED` to the heading, keep every original field, and add `Status` (`✅ Completed — …; PR #<n>`), `Implementation` (at most 3 lines, plus the commit SHA), `Spec deviations` (`None`, or each deviation with its reason) and `Files modified`. Every task appends a scratchpad note under Discoveries.
- **Commits.** Every commit is signed off (`git commit -s`, DCO). Task 2's persistence change is recorded by ADR-0011, which lands with Task 8.

---

## Implementation Tasks

### Task 1 — qualify record types and scales ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: None
- **Change**: Add `internal/qualify` with the v2 §7.1 record model (Key, Record, Column, Evidence, Capability) and the §4.2 progress, column-verdict and datum-label scales, so later tasks share one exact contract.
- **Files**:
  - `internal/qualify/qualify.go`
  - `internal/qualify/qualify_test.go`
- **Produces**: `qualify.Progress` (+ 7 constants), `qualify.Verdict` (`Proven`, `NotProven`, `Unknown`), `qualify.DatumLabel` (+ 5 constants), `qualify.Key`, `qualify.Evidence`, `qualify.Column`, `qualify.Record`, `qualify.Capability`; `qualify.Datum{Quantity, Label, Unit, Scope, Source, At}`; `qualify.KeyHash(k Key) string` (hex sha256 over canonical Key JSON); all structs with explicit snake_case `json` tags; `qualify.CanonicalDigest(r Record) (string, error)` — recomputes `sha256:<hex>` over canonical JSON minus Digest/SupersededAt, error naming the offending field on an out-of-scale enum value; `qualify.DecodeRecord(data []byte) (Record, error)` — decodes canonical JSON with missing→`"unknown"` defaults; syntax errors name the offset, out-of-scale enum values name the offending field.
- **Acceptance**:
  - `TestProgressScaleMatchesV2`: the seven progress constants equal the v2 §4.2 words in order; renaming one fails the test.
  - `TestOutOfScaleEnumRejected`: a record with `Progress: "approved"` returns an error naming `Progress`, and never coerces to a passing value.
  - `TestDigestRecomputes`: `CanonicalDigest` of a fixed record equals its golden `sha256:<hex>`; flipping one byte of the record changes the digest.
  - `TestKeyHashDeterministic`: `KeyHash` of a fixed key equals its golden hex; changing one field changes it; every store caller derives key hashes only through it.
  - `TestCanonicalJSONUsesSnakeCase`: marshaling a fixed record contains `"model_snapshot"` and no `"ModelSnapshot"`; `DecodeRecord` of JSON missing `model_snapshot` yields `"unknown"` for the field.
  - `TestMissingReadsUnknown`: `DecodeRecord` of JSON missing `model_snapshot` carries `"unknown"`, never `""` treated as established or `0`; truncated JSON returns an error naming the offset; an out-of-scale enum returns an error naming the field.
  - `TestDatumDefaults`: `DecodeRecord` of JSON missing `quota` and evidence `label`/`source` yields `DatumUnknown` labels and `"unknown"` quantity/unit/scope/source; an out-of-scale datum label returns an error naming the field.
  - `TestColumnIndependence`: a record with fidelity `proven`, entitlement `not-proven` and lifecycle `unknown` round-trips through JSON with each column verdict preserved; no column's value changes another (AC-1.2).
- **Test plan**: Table tests over the scales; golden JSON fixtures for the digest test.
- **Invariants touched**: I09 (v2 §7.3: absent values stay `unknown`); I20 (v2 §7.1: revisions content-addressed); I14 (v2 §7.2: scales for honest labels).
- **Status**: ✅ Completed — `internal/qualify` ships the v2 record model, honest-label scales and canonical digest/decode contract; PR #200.
- **Implementation**: Decode validates the Progress/Verdict/DatumLabel/Capability scales with field-naming errors and applies missing→`unknown` defaults (I09); digests are SHA-256 over canonical JSON minus Digest/SupersededAt. Goldens oracle-verified with `sha256sum`. Commit 21ad65d.
- **Spec deviations**: None.
- **Files modified**: `internal/qualify/qualify.go`, `internal/qualify/qualify_test.go`, `specs/in-progress/qualification-registry/tasks.md`, `specs/in-progress/qualification-registry/handoff.md`, `specs/in-progress/qualification-registry/scratchpad.md`.

### Task 2 — SQLite migration 0002 and seven-harness seed ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: complex (persistence schema + migration)
- **Depends on**: Task 1
- **Change**: Add migration `0002_qualification.sql` with the records table, the journal row helpers, and the lazy seven-harness seed, so the registry has durable versioned storage.
- **Files**:
  - `internal/journal/migrations/0002_qualification.sql`
  - `internal/journal/journal.go` (`SchemaVersion` 1→2)
  - `internal/journal/journal_test.go` (fresh-`Open` `user_version` assertion `"1"`→`"2"`)
  - `internal/journal/qualification.go`
  - `internal/journal/qualification_test.go`
  - `internal/qualify/seed.go`
  - `internal/qualify/seed_test.go`
- **Produces**: `func (j *Journal) InsertQualificationRecord(ctx context.Context, keyHash string, revision int, digest, recordJSON string) error`; `func (j *Journal) CurrentQualificationRecord(ctx context.Context, keyHash string) (recordJSON string, revision int, err error)` (absent → `journal: qualification record: %w` wrapping `ErrNotFound`); `func (j *Journal) ListCurrentQualificationRecords(ctx context.Context) (records []string, err error)`; `func (j *Journal) InsertQualificationRecordIfAbsent(ctx context.Context, keyHash string, revision int, digest, recordJSON string) (inserted bool, err error)` (single-statement INSERT OR IGNORE, never supersedes); `qualify.SeedV1() []Record` (seven blocked/planned records with NextTest, platform-filled); `qualify.EnsureSeeded(ctx context.Context, j *journal.Journal) error` (records the seed rows missing from the table — no-op when complete, completes partial seeds; seed failure returns the journal error wrapped). One import direction: `qualify` imports `journal`, never the reverse.
- **Acceptance**:
  - `TestMigration0002Applies`: a database at `user_version=1` opens at version 2 with the table present; the migration file `0001_init.sql` is byte-identical to `main` (hash check); `SchemaVersion == 2` and a fresh `Open` reports `user_version` `"2"`; `OpenReadOnly` opens the migrated database without `ErrSchemaTooNew`.
  - `TestRecordRevisionImmutability`: inserting revision 2 for a key leaves revision 1's `record_json` and digest unchanged (excluding the `superseded_at` metadata the insert sets), `Current-` returns revision 2, and both rows' digests recompute via `qualify.CanonicalDigest`; NFR-2 holds.
  - `TestInsertRefusesInvalid`: empty `keyHash`, empty `digest`, empty `recordJSON` or `revision < 1` each return `journal: invalid qualification record` and write nothing.
  - `TestAbsentIsNotFound`: `Current-` on an unknown key returns `ErrNotFound`, never a zero record.
  - `TestSeedHasSevenHarnesses`: `SeedV1` returns exactly the seven v2 §7.2 harness ids, every record `blocked` or `planned` (none `fixture-tested` or better); at least one record is `blocked`, and every `blocked` record carries a `NextTest` matching `<step>; authority: <grant>` with both halves non-empty.
  - `TestEnsureSeededIdempotent`: `EnsureSeeded` on an empty table records seven; on a complete table leaves every row untouched (hash comparison); it completes a partial seed (3 of 7 rows) to seven; two concurrent calls on separate journal connections to the same database both return nil and leave exactly seven (race detector on).
  - `TestInsertIfAbsentNoOverwrite`: on an absent key `InsertQualificationRecordIfAbsent` returns `inserted=true` and the row reads back; on a present key it returns `inserted=false` with the existing row byte-identical (hash comparison) and no new revision; invalid input returns `journal: invalid qualification record` like `Insert-`.
- **Test plan**: Temp-dir SQLite databases via `journal.Open`; golden `user_version` assertions; failure injection by tampering digests.
- **Invariants touched**: I20 (v2 §5.2: additive migration, old revisions immutable); I09 (v2 §7.3: absence is `ErrNotFound`, not zero); I23 (v2 §5.1: ledger owns the state).
- **Status**: ✅ Completed — migration 0002 with the qualification_records table, journal row helpers and the lazy seven-harness seed; PR #201.
- **Implementation**: Insert supersedes the previous current row in its own transaction; IfAbsent is a single INSERT OR IGNORE, so concurrent seeds serialise on the statement. EnsureSeeded inserts missing seed rows then reads each seed key back through Current, wrapping journal errors; a complete table is byte-identical afterwards. Seed: claude-code blocked, six planned, platform-filled keys, every digest recomputes. Commit 60b83fd.
- **Spec deviations**:
  - `internal/journal/qualification_revisions_test.go` (extra file, not in Files): Go rejects the qualify import in an internal journal test (`import cycle not allowed in test`, probed), so TestRecordRevisionImmutability lives in external `package journal_test` with a read-only row-inspection helper. No production impact.
  - `TestEnsureSeededFailureWrapsJournalError` in `internal/qualify/seed_test.go` (extra test): pins the Produces parenthetical "seed failure returns the journal error wrapped", for which the acceptance names no test.
- **Files modified**: `internal/journal/migrations/0002_qualification.sql`, `internal/journal/journal.go`, `internal/journal/journal_test.go`, `internal/journal/qualification.go`, `internal/journal/qualification_test.go`, `internal/journal/qualification_revisions_test.go`, `internal/qualify/seed.go`, `internal/qualify/seed_test.go`, `specs/in-progress/qualification-registry/tasks.md`, `specs/in-progress/qualification-registry/handoff.md`, `specs/in-progress/qualification-registry/scratchpad.md`.

### Task 3 — Registry open/query/record and drift invalidation ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: complex (several abstractions: registry, drift, consult matching, digest derivation; 13 acceptance items)
- **Depends on**: Task 2
- **Change**: Add the `qualify.Registry` with read-write and read-only opens, lookup/list/record, the pure drift check and drift invalidation, so admission and doctor consult one versioned source. `Lookup`/`List` decode stored JSON exclusively via `qualify.DecodeRecord` (Task 1).
- **Files**:
  - `internal/qualify/registry.go`
  - `internal/qualify/drift.go`
  - `internal/qualify/registry_test.go`
  - `internal/qualify/drift_test.go`
- **Produces**: `qualify.Open(ctx context.Context, dir string) (*Registry, error)` (migrates; never seeds); `qualify.OpenReadOnly(ctx context.Context, dir string) (*Registry, error)`; `(*Registry).Close() error`; `(*Registry).Lookup(ctx context.Context, k Key) (Record, error)` (absent → `qualify.ErrNotFound`); `(*Registry).List(ctx) ([]Record, error)` (harness, surface order); `(*Registry).Record(ctx, Record) error`; `(*Registry).Consult(ctx context.Context, observed Key) (rec Record, drifted bool, reason string, err error)`; `qualify.CheckDrift(qualify.DriftInput) (bool, string)`; `qualify.ConfigDigestOf(digests map[string]string) string`; `qualify.IsMissing(err error) bool`; `qualify.ErrSchemaMismatch`; `qualify.IsSchemaMismatch(err error) bool`; `(*Registry).InvalidateOnDrift(ctx, Record, string) error`; `qualify.ErrNotFound`.
- **Acceptance**:
  - `TestOpenMigratesWithoutSeeding`: opening a fresh temp state dir migrates the table and `List` reports empty; only `EnsureSeeded` adds records.
  - `TestConsultMatchesStableIgnoresDigests`: `Consult` with an observed key whose digests drifted from the recorded one returns the record with `drifted=true` and a reason naming the field; an unknown harness returns `ErrNotFound`; two stable matches return `qualify: ambiguous stable match: ...`.
  - `TestConfigDigestOfDeterministic`: the same digests map inserted in two orders yields the identical `"sha256:<hex>"`; changing one value changes it.
  - `TestOpenReadOnlyWritesNothing`: `OpenReadOnly` on a fresh dir migrates and seeds nothing — directory hash unchanged — and `List` reports empty without error.
  - `TestOpenReadOnlyMissingDirErrors`: `OpenReadOnly` on a nonexistent dir returns an error naming the dir (doctor keys `unavailable` off it); `List` against a database without the table returns an `ErrSchemaMismatch`-wrapping error.
  - `TestOpenReadOnlyZeroVersionErrors`: a database file with `user_version=0` returns an error, never an empty registry.
  - `TestLookupRoundTrip`: `Record` then `Lookup` returns revision 1 with a recomputing digest (a passed-in `Revision: 99` is ignored and stored as 1); lookup of an unknown key returns `ErrNotFound`.
  - `TestRecordRefusesTamperedDigest`: recording a record whose `Digest` does not recompute returns `qualify: record digest mismatch: ...` and writes nothing.
  - `TestRecordRefusesEvidenceFreeProven`: a record with an entitlement column `proven` but empty evidence returns `qualify: proven verdict without evidence: entitlement` and writes nothing.
  - `TestDriftDetectsBinaryAndConfigChange`: changed `ExecutableDigest` and changed `ConfigDigest` each report drifted with a reason naming the field; identical digests report clean.
  - `TestInvalidateResetsColumns`: `InvalidateOnDrift` stores a new revision with affected columns `Unknown`/`NotProven`, progress `blocked`, and the reason preserved; the old revision's `record_json` and digest are unchanged (excluding `superseded_at`).
  - `TestRegistryLookupLatency`: cold `Lookup` on a temp-dir registry completes in under 1 s (NFR-1); a variant wrapping `Lookup` with a 1.5 s injected delay exceeds the bound and fails, proving the test discriminates.
  - `TestOpenReadOnlyVersionMismatchErrors`: `OpenReadOnly` on a database with a newer `user_version` returns the wrapped `ErrSchemaTooNew`; doctor keys `unavailable` off it.
- **Test plan**: Temp state dirs; hex-dump comparison for read-only test; table tests for drift field coverage.
- **Invariants touched**: I20 (v2 §7.1: invalidation is a new revision, old evidence preserved); I09 (v2 §7.3: empty registry reads empty, never zero rows); A30 lineage (read-only open cannot mutate).
- **Status**: ✅ Completed — `qualify.Registry` with read-write/read-only opens, lookup/list/record, stable-match consult, pure drift check and drift invalidation; PR #202.
- **Implementation**: Record derives empty digests, refuses tampered ones and defaults zero fields to unknown; Lookup/List decode via DecodeRecord and verify digests recompute; InvalidateOnDrift derives from the current row with audit markers. Commit 4875031.
- **Spec deviations**: None.
- **Files modified**: `internal/qualify/registry.go`, `internal/qualify/drift.go`, `internal/qualify/registry_test.go`, `internal/qualify/drift_test.go`, `specs/in-progress/qualification-registry/tasks.md`, `specs/in-progress/qualification-registry/handoff.md`, `specs/in-progress/qualification-registry/scratchpad.md`.

### Task 4 — Admission consults the registry

- **Domain/agent**: go-implementer
- **Budget**: standard (14 small verdict-mapping items; several fold into one table test)
- **Depends on**: Task 3, Task 5 (the agreement test consumes `RecordDraft`)
- **Change**: Add `ResolveQualification` and call it from `decideClaudeCode` after `AuthStatus` and before billing, so eligibility comes from versioned records while strict subscription-only keeps blocking under the fixtures-only default.
- **Files**:
  - `internal/admission/qualify.go`
  - `internal/admission/admission.go`
  - `internal/admission/qualify_test.go`
- **Produces**: `admission.ResolveQualification(ctx context.Context, reg *qualify.Registry, probe adapter.Probe, manifest adapter.ConfigManifest, evidence claudecode.AuthEvidence, billing string, profile string) (admission.Eligibility, error)` (nil reg counts as absent record); `admission.Eligibility{Verdict, Reason, Record}`; `admission.EligibilityVerdict` (`Eligible`, `Blocked`, `Unsupported`).
- **Acceptance**:
  - `TestNoRecordBlocks`: unknown probe identity with `--billing subscription-only` yields `Blocked` with reason `no_qualification_record`, never `Eligible`.
  - `TestNonLiveRecordBlocksStrict`: a `fixture-tested` record with `not-proven` entitlement yields `Blocked` with reason `entitlement_not_proven` for `--billing subscription-only`.
  - `TestAmbiguousMatchBlocks`: two stable matches yield `Blocked` with reason `ambiguous_qualification_match` for `--billing subscription-only`.
  - `TestUnsupportedSurfaceMapsUnsupported`: a record with `Progress == unsupported`, or a harness outside the seven v2 §7.2 ids, yields `Unsupported` (exit 7) under every billing mode including `subscription-declared`.
  - `TestExpiredEvidenceIgnored`: a `proven` column whose only evidence has past `Expiry` counts as `not-proven` for the consult; unexpired evidence still qualifies.
  - `TestExpiredCapabilityIgnored`: a `stop_at_exhaustion` capability with past `Expiry` counts as `unknown` for the consult even when `Value` is `supported`; an unexpired one still qualifies.
  - `TestConsultSchemaMismatchMapsAbsent`: a v1-schema database maps to absent record (strict `Blocked`, declared `Eligible`), letting the run reach the supervisor migration; a permission-denied registry still errors.
  - `TestDeclaredUnaffectedByMissingRecord`: with `--billing subscription-declared`, a missing or non-live record yields `Eligible` (record attached when present); the dogfood path never blocks on the registry.
  - `TestUnknownQuotaAdmitsStopAtExhaustion`: a record with `stop_at_exhaustion: supported` capability and `Quota` unknown-datum yields `Eligible` with the quantity labelled `unknown` (AC-3.3); without the marker it stays `Blocked`.
  - `TestRegistryUnreadableErrors`: a registry whose directory exists but is unreadable (permissions) yields `qualify: registry unavailable: %w`, never silent eligibility.
  - `TestConsultMissingMapsAbsent`: a nil registry yields `Eligible` for `subscription-declared` and `Blocked` with `no_qualification_record` for `subscription-only` (missing dir/DB never errors the consult).
  - `TestResolveQualificationFindsRecordDraft`: with `RecordDraft` (built per the design §4 key table from fixture probe values) stored via `Registry.Record`, `ResolveQualification` on the same fixture finds it (`Blocked` with `entitlement_not_proven` for strict, not `no_qualification_record`); drifting the fixture digest yields a drift reason instead.
  - `TestStrictStillBlocksEndToEnd`: `cli.Main` with `--adapter claudecode --billing subscription-only` still exits 3 with `entitlement_qualification_unavailable` in JSONL (existing-behavior guard).
  - `TestDriftedRecordBlocks`: a record whose pinned digest differs from the probe yields `Blocked` with a drift reason (AC-4.1 path through admission).
- **Test plan**: `cli.Main` tests with temp state dirs for the end-to-end items; direct unit tests for verdict mapping; existing admission tests must pass unchanged.
- **Invariants touched**: I02 (v2 §7.3: unknown mandatory evidence blocks); I15 (v2 §7.3: strict stays blocked without proof); I07 lineage (admission evidence bound to the probed candidate).

### Task 5 — First-route authorised-test evidence (claudecode)

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 3
- **Change**: Add the claudecode first-route evidence constructor and AT-03 fixture inventory, so FR-3's fixture-tested record rests on enumerated authorised-test evidence with no hidden paid auxiliary.
- **Files**:
  - `adapters/claudecode/qualify.go`
  - `adapters/claudecode/qualify_test.go`
  - `adapters/claudecode/testdata/qualification/routes.json`
  - `adapters/claudecode/COMPATIBILITY.md` (header note: the registry is the queryable record, this file the per-harness view)
- **Produces**: `claudecode.EntitlementEvidence() []qualify.Evidence` (offline-fixture, no live call, no secret read; each with `Label`/`Source` set); `claudecode.RecordDraft(key qualify.Key) (qualify.Record, error)` (fixture-tested first-route record with unknown-datum Quota; any other key returns `claudecode.ErrNotFirstRoute`); `claudecode.ErrNotFirstRoute`.
- **Acceptance**:
  - `TestEvidenceCoversAT03Routes`: evidence covers implementer, planner, reviewer, summary, router, child and experiment routes; deleting one route from `routes.json` fails the test.
  - `TestNoHiddenPaidAuxiliary`: the inventory asserts no paid auxiliary per route; a fixture variant adding one fails the test.
  - `TestRecordDraftRefusesOtherKeys`: a key with another harness or surface returns `ErrNotFirstRoute` (`errors.Is`) and no record; same harness+surface with different digests passes.
  - `TestEvidencePerformsNoLiveCall`: `go list -deps` on the test's own package shows no `net`/`net/http` membership (no socket API reachable, so no direct connection is possible); additionally, with proxies poisoned to unroutable `http://127.0.0.1:9` and `HOME` at an empty temp dir, output is byte-identical. Either leg failing fails the test.
  - `TestDraftRecordsThroughRegistry`: `RecordDraft` output round-trips through `Registry.Record`/`Lookup` with a recomputing digest.
  - `TestDraftQuotaLabelledUnknown`: `RecordDraft`'s `Quota` is the unknown Datum (`Quantity`/`Unit`/`Scope`/`Source` `"unknown"`, `Label` `DatumUnknown`); every evidence entry carries a non-empty `Source`.
  - `TestCompatibilityNoteAnchored`: the first 10 lines of `adapters/claudecode/COMPATIBILITY.md` name the qualification registry as the queryable record and the file as the per-harness view; removing the note fails the test.
  - `TestCompatibilityFactsMatchDraft`: a Go test parses the fixture facts the note states (surface, native version, progress) and asserts each equals the corresponding `RecordDraft` field; a drifted fact fails the test.
- **Test plan**: Synthetic JSON fixtures (marked synthetic until MH-12); hermetic execution; registry round-trip against temp state dir.
- **Invariants touched**: I15 (v2 §7.3: no sign-in/declaration evidence); I19 (v2 §7.1: no secret reads); I13 (no credentials needed).

### Task 6 — doctor qualification section ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 3
- **Change**: Add the `qualification` section to `doctor`'s plain and JSONL output via a read-only registry open, so users see each record's progress, columns, evidence revisions and drift triggers.
- **Files**:
  - `internal/cli/doctor.go`
  - `internal/cli/doctor_test.go`
- **Acceptance**:
  - `TestDoctorShowsQualification`: with a seeded temp state dir, plain output holds one line per record matching `^<harness> × <surface>: <progress> \(fidelity <v>, entitlement <v>, lifecycle <v>\) \[evidence <n> revs, latest <ev-id>; drift <clean|reason>\]$` anchored to the qualification section.
  - `TestDoctorQualificationJSONL`: `--format jsonl` parses to one object whose `qualification` map carries, per record, `progress`, per-column `verdict` + `evidence` ids, `evidence_revisions` and `drift_triggers`; a record missing any field fails the test.
  - `TestDoctorQualificationUnavailableHonest`: with no state dir, the section reads `unavailable (<reason>)`, exit stays 0, and no rows are fabricated.
  - `TestDoctorStillWritesNothing`: against a seeded non-empty temp state dir, warm one read-only open first (a WAL reader may create `-shm`/`-wal` sidecars on first touch), then assert the SHA-256 of every file is unchanged across the run; a variant that writes a marker file into the dir fails the test (read-only guard with a red variant).
- **Test plan**: `cli.Main` tests with temp `MYTHHELM_HOME`; golden anchored section match; hash-before/after for read-only.
- **Invariants touched**: I09 (v2 §7.3: missing reads `unavailable`/`unknown`); I13 lineage (doctor needs no credentials); A30 lineage (read-only doctor).
- **Status**: ✅ Completed — `doctor` shows the qualification section in plain and JSONL via a read-only registry open, with honest unavailable/empty reads; PR #204.
- **Implementation**: One `Qualification` map feeds both outputs; drift runs `CheckDrift` against unobserved digests (unknown pins read clean, established pins read unobserved, fail-closed). Red-first: display tests failed pre-change; a dropped-drift-slot mutant failed the plain tests. Commit 2415894.
- **Spec deviations**:
  - `TestDoctorQualificationEmptyHonest` in `internal/cli/doctor_test.go` (extra test): pins the design §6 empty-read rule (existing but unseeded dir reads `(no records)`/`[]`, never null), for which the acceptance names no test.
- **Files modified**: `internal/cli/doctor.go`, `internal/cli/doctor_test.go`, `specs/in-progress/qualification-registry/tasks.md`, `specs/in-progress/qualification-registry/handoff.md`, `specs/in-progress/qualification-registry/scratchpad.md`.

### Task 7 — End-to-end packaged-binary tests

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 4, Task 5, Task 6
- **Change**: Add `tests/e2e` coverage that builds `cmd/mythhelm` and proves the registry, the blocking admission and the doctor surface behave from a real binary.
- **Files**:
  - `tests/e2e/qualification_test.go`
- **Acceptance**:
  - `TestE2EDoctorShowsSevenRecords`: the built binary's `doctor --format jsonl` against a seeded temp home lists seven records with honest progress values.
  - `TestE2EStrictStillBlocked`: the built binary's `run --adapter claudecode --billing subscription-only` exits 3 with reason `entitlement_qualification_unavailable` in JSONL.
  - `TestE2EDeclaredUnaffected`: the built binary's user-declared dogfood path still admits exactly as before this spec (no exit-code or label change).
- **Test plan**: Build `cmd/mythhelm` once per package run; temp `MYTHHELM_HOME`; assert stdout, stderr and exit codes.
- **Invariants touched**: I02 (v2 §7.3: end-to-end blocking preserved); I13 (no credentials in e2e).

### Task 8 — User guide and ADR-0011

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 4, Task 6
- **Change**: Record ADR-0011 (registry in `mythhelm.db`) and document the `doctor` qualification section in the user guide, so the shipped behavior is described where users and later specs look.
- **Files**:
  - `docs/decisions/0011-qualification-registry-store.md`
  - `docs/user-guide.md`
- **Acceptance**:
  - `grep -q "mythhelm.db" docs/decisions/0011-qualification-registry-store.md && grep -q "I23" docs/decisions/0011-qualification-registry-store.md`: ADR-0011 records the D1 decision (registry table in `mythhelm.db`, versioned files rejected as a competing store per I23). Exits 0 after the change, non-zero before (file absent).
  - `grep -q "Status: accepted" docs/decisions/0011-qualification-registry-store.md && grep -q "^## Context" docs/decisions/0011-qualification-registry-store.md && grep -q "^## Decision" docs/decisions/0011-qualification-registry-store.md && grep -q "^## Consequences" docs/decisions/0011-qualification-registry-store.md`: the ADR follows the `docs/decisions/README.md` template (status plus all three headings).
  - `sed -n '/^## Doctor/,/^## /p' docs/user-guide.md | grep -q "× .*fidelity"`: the user guide's `## Doctor` section documents the qualification output with a plain-format example line matching Task 6's `^<harness> × <surface>:` shape. Exits non-zero before the change (no such section).
- **Test plan**: No Go test asserts prose; each acceptance item is a recorded shell command with a failing counterfactual (absent file or text before the change). The completion entry cites each command's exit-0 output.
- **Invariants touched**: None (docs only; the mechanism tasks keep I14/I20 — this task describes, not changes, behavior).

### Task 9 — Production registry seeding on admitted runs

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 2, Task 6
- **Change**: Call `qualify.EnsureSeeded` in the supervisor after `journal.Open`, so the first admitted run seeds the seven records and refused runs seed nothing.
- **Files**:
  - `internal/supervisor/pipeline.go`
  - `internal/supervisor/pipeline_test.go`
  - `tests/e2e/qualification_seed_test.go`
- **Acceptance**:
  - `TestRunSeedsRegistry`: `supervisor.Run` on a fresh temp state dir with an admitted fake decision leaves seven current records; a second run records none (count stays seven).
  - `TestSeedFailureFailsClosed`: `supervisor.Run` with an unseedable database (table dropped out from under it) fails through `persistence_unavailable` instead of running unseeded.
  - `TestE2EFreshHomeSeedsOnRun`: the built binary against a fresh temp home runs `run --adapter fake --billing local-scripted` (per the `tests/e2e/run_test.go` helper pattern) with no test-side seeding, after which `doctor --format jsonl` lists seven records.
- **Test plan**: Temp state dirs for the unit items; packaged-binary e2e with `MYTHHELM_HOME` pointed at a fresh temp dir.
- **Invariants touched**: I14 (v2 §7.2: the seven honest labels exist from the first admission); I02 lineage (state init fail-closed).
