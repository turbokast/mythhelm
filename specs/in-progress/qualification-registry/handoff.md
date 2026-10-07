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

<!-- pending -->

## Task 3 — Registry open/query/record and drift invalidation

<!-- pending -->

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
