# qualification-registry — Hand-off

> One section per task. Each task replaces its own `<!-- pending -->` line in its pull request with what
> dependent tasks need: what it produced (the API and files as shipped), what a later task must
> know, and any deviation that changes a later task's inputs. `/run-spec` puts the sections of a
> task's dependencies into that task's dispatch prompt. Open questions and research stay in
> `scratchpad.md`.

## Task 1 — qualify record types and scales

- **Produces**: `internal/qualify` with `Progress` (+7 constants in v2 §4.2 order), `Verdict` (`Proven`/`NotProven`/`Unknown`), `DatumLabel` (`Reported`/`Observed`/`Estimated`/`UserDeclared`/`DatumUnknown`), `Key` (14 fields), `Evidence`, `Column`, `Record`, `Capability`, `Datum{Quantity, Label, Unit, Scope, Source, At}` — all with explicit snake_case `json` tags; `KeyHash(Key) string` (bare hex); `CanonicalDigest(Record) (string, error)` (`sha256:<hex>` over canonical JSON minus `Digest`/`SupersededAt`); `DecodeRecord([]byte) (Record, error)` (missing→`unknown` defaults). Exactly the design §2 API; no additions or renames.
- **For dependents**: every store caller must derive key hashes only through `qualify.KeyHash` (Task 3 `Registry`); `Lookup`/`List` must decode stored JSON exclusively via `qualify.DecodeRecord`.
- **For dependents**: `DecodeRecord` requires `Progress` — the scale holds no unknown value, so missing/empty/out-of-scale `Progress` returns an error naming `Progress`. Missing verdicts/labels/datum fields default to `unknown`/`DatumUnknown`; a missing `Quota` becomes the unknown datum.
- **For dependents**: any `DecodeRecord`/`CanonicalDigest` error returns the zero value (`Record{}`/`""`) — fail closed, never coerced. Syntax errors name the offset (`at offset %d`); scale errors name the Go field path (e.g. `Quota.Label`, `Entitlement.Evidence[0].Label`).
- **For dependents**: `CanonicalDigest` validates scales before hashing; a `Record` with `Digest`/`SupersededAt` set still recomputes over content only. Evidence `Method`/`Result` strings are NOT scale-validated; capability `Value` validates against `supported|unsupported|unknown`.
- **Deviations that change a later task's inputs**: none.

## Task 2 — SQLite migration 0002 and seven-harness seed

- **Produces**: migration `0002_qualification.sql` (`qualification_records(key_hash, revision, digest, record_json, recorded_at, superseded_at)`, PK `(key_hash, revision)`, partial unique index on current rows); `SchemaVersion == 2`. `(*Journal).InsertQualificationRecord` (supersedes in-transaction), `CurrentQualificationRecord` (absent → `journal: qualification record: %w` over `ErrNotFound`), `ListCurrentQualificationRecords` (key-hash order), `InsertQualificationRecordIfAbsent` (single INSERT OR IGNORE, reports inserted); invalid input → `journal: invalid qualification record`, nothing written. `qualify.SeedV1() []Record` (seven v2 §7.2 records: claude-code blocked, six planned; platform-filled keys; recomputing digests); `qualify.EnsureSeeded(ctx, j) error` (inserts missing rows, verifies per-key, wraps journal errors). Exactly the design §3 API; no renames.
- **For dependents**: the journal stores opaque strings — digest discipline is the caller's. `Registry.Record` must set `rec.Digest = CanonicalDigest(rec)` before marshal and store revision current+1 (seed rows are revision 1, `SchemaVersion: 2`).
- **For dependents**: `EnsureSeeded` never overwrites: present keys stay byte-identical whatever their content; rerun after a partial failure completes the seed. Concurrent first runs are safe (statement-serialised).
- **For dependents**: every `EnsureSeeded` failure wraps the journal error (`qualify: seeding qualification records: %w` / `qualify: verifying qualification seed: %w`); a shortfall surfaces as wrapped `ErrNotFound` for the missing key.
- **For dependents**: import direction is one-way (`qualify` → `journal`). Journal-internal tests cannot import `qualify` (Go rejects the cycle) — digest-asserting journal tests live in external `package journal_test` (see `qualification_revisions_test.go`).
- **Deviations that change a later task's inputs**: none.

## Task 3 — Registry open/query/record and drift invalidation

- **Produces**: `qualify.Open` (migrates, never seeds), `qualify.OpenReadOnly`, `(*Registry).Close/Lookup/List/Record/Consult/InvalidateOnDrift`, `qualify.CheckDrift(DriftInput)`, `qualify.ConfigDigestOf`, `qualify.IsMissing/IsSchemaMismatch`, `qualify.ErrNotFound/ErrSchemaMismatch`. Exactly the design §4 API; no additions or renames.
- **For dependents**: `Record` ignores `Revision` (stores current+1), clears `SupersededAt`, defaults zero fields to unknown markers, derives an empty `Digest`, and refuses a non-empty digest that does not recompute over the stored content. Mutating a record therefore requires clearing `Digest` (or recomputing over the revision `Record` will assign); `SchemaVersion` is preserved as passed.
- **For dependents**: `Lookup`/`List` decode via `DecodeRecord` and verify the digest recomputes — a tampered row errors (`qualify: stored record digest mismatch`), never a coerced record. `List` sorts by harness, surface, then key hash. One corrupt row fails the whole `List`.
- **For dependents**: `Consult` tries the exact key, else the unique stable-identity match (Harness, Surface, OS, Arch, TrustProfile, EntitlementClass, AdapterProtocol); zero → `ErrNotFound`, several → `qualify: ambiguous stable match: ...`. Unknown-pinned digests never drift; unobserved digests fail closed against an established pin. Reasons name the snake_case field.
- **For dependents**: `InvalidateOnDrift` reads only `rec.Key` — the new revision derives from the current row (stale-safe), affected columns drop to `NotProven` (`Unknown` stays) with an `ev_drift_invalidation` marker preserving reason + prior ids, progress becomes `blocked`. Unknown keys read `ErrNotFound`; empty reasons are refused.
- **For dependents**: `OpenReadOnly` on a missing dir names the dir (`IsMissing`); version-0 files and older/newer versions wrap `ErrSchemaMismatch` (newer keeps `ErrSchemaTooNew` in the chain); an existing dir without a database reads as an empty registry and writes nothing. A missing table wraps `ErrSchemaMismatch` from both `List` and `Lookup`.
- **Deviations that change a later task's inputs**: none.

## Task 4 — Admission consults the registry

<!-- pending -->

## Task 5 — First-route authorised-test evidence (claudecode)

<!-- pending -->

## Task 6 — doctor qualification section

<!-- pending -->

## Task 7 — End-to-end packaged-binary tests

<!-- pending -->

## Task 8 — User guide and ADR-0011

<!-- pending -->

## Task 9 — Production registry seeding on admitted runs

<!-- pending -->
