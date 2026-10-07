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

- **Produces**: `admission.ResolveQualification(ctx, reg, probe, manifest, evidence, billing, profile) (Eligibility, error)` (nil reg counts as absent), `admission.Eligibility{Verdict, Reason, Record}`, `admission.EligibilityVerdict` (`Eligible`/`Blocked`/`Unsupported`), plus `admission.OpenQualificationRegistry(ctx, dir)` (missing/schema-mismatch → nil registry, other open failures → `qualify: registry unavailable`); `decideClaudeCode` consults after `AuthStatus`, mapping `Blocked`→exit 3 and `Unsupported`→exit 7 `*BlockedError`.
- **For dependents**: strict admits only live-qualified + entitlement proven (≥1 unexpired evidence) + `stop_at_exhaustion` supported (unexpired); unknown `Quota` never blocks. Non-live progress maps to `entitlement_not_proven` even when the column reads proven (AC-2.3); missing/expired/unsupported marker maps to `stop_at_exhaustion_unproven`.
- **For dependents**: declared/local-scripted admit missing, non-live, drifted and ambiguous consults (record attached when unique); only `qualification_unsupported` (unsupported progress or harness outside the seven) refuses every mode. Drifted strict consults block with the CheckDrift reason text; unknown billing modes return `ErrInvalid`.
- **For dependents**: `Eligibility.Reason` is empty when `Eligible`; `Record` is attached whenever one record was consulted (nil for absent/ambiguous/pre-consult). Ambiguity is detected via `errors.Is(err, qualify.ErrAmbiguousMatch)` (review round 1 sentinel; the `qualify: ambiguous stable match: ...` message text is unchanged); `TestTamperedDigestNeverAmbiguous` pins that digest text naming the report never classifies as ambiguity.
- **Review round 1**: `OpenReadOnly` refuses a non-directory dir on every platform (Windows stat of `dir/mythhelm.db` reads ENOENT); the probe-gated `cli.Main`/`Decide` legs of `TestUnsupportedSurfaceMapsUnsupported` and `TestDeclaredUnaffectedByMissingRecord` skip on Windows (the probe refuses it by design) while their `resolve()` legs cover the mappings there.
- **For dependents**: the consult never migrates or seeds; the strict early return in `Decide` still fires first, so strict consult-blocks are unit-tested only until MH-12 removes it. Seed records (surface/trust `unknown`) never stable-match a real probe.
- **Deviations that change a later task's inputs**: Task 7's declared e2e must expect exit 7 + `qualification_unsupported` only with a stable-matching unsupported record seeded; Task 9 note — a v1 database still fails at admission's earlier trust-journal open before the consult, so "v1 maps to absent" holds for the consult entry, not end-to-end through `Decide`.

## Task 5 — First-route authorised-test evidence (claudecode)

- **Produces**: `claudecode.EntitlementEvidence() []qualify.Evidence` (7 offline-fixture entries, one per AT-03 route, from the embedded `testdata/qualification/routes.json`; deterministic, no live call, no reads); `claudecode.RecordDraft(qualify.Key) (qualify.Record, error)` (fixture-tested record: entitlement proven with the evidence, fidelity/lifecycle unknown, stop_at_exhaustion unknown, unknown-datum quota, NextTest naming the authorised live suite, digest computed at revision 1); `claudecode.ErrNotFirstRoute`. Exactly the design §5 API; no additions or renames.
- **For dependents**: the refusal predicate is `Harness == "claude-code" && Surface == "native-cli-structured (print, stream-json)"` (digests ignored), tracking `New().Descriptor()` — build the Task 4 agreement key per the design §4 table and it passes; anything else returns `ErrNotFirstRoute` (`errors.Is`) with the zero record.
- **For dependents**: `RecordDraft` output stores into a fresh registry as revision 1 with its digest intact; re-recording a mutated draft needs `Digest` cleared first (Task 3's stale-digest refusal). Evidence ids are `ev_at03_<route>`; each `Suite` is `claudecode-at03-fixtures-<native>` where `<native>` is the inventory's `native_version` (2.1.284); every `Uncertainty` carries its route's passing condition plus the paid-continuation-unknown caveat.
- **For dependents**: `COMPATIBILITY.md` now opens with the registry note (first 10 lines) stating parseable `surface`/`native`/`progress` facts pinned against `RecordDraft`; editing the note, the inventory's `native_version` or the evidence `Suite` format fails `TestCompatibilityFactsMatchDraft` until all three agree.
- **Deviations that change a later task's inputs**: none (the `TestEvidencePerformsNoLiveCall` static-leg mechanism change is test-only; the shipped API and behaviour match the design).

## Task 6 — doctor qualification section

- **Produces**: `doctor` `qualification` section in plain (`qualification:` header + one `<harness> × <surface>: <progress> (fidelity <v>, entitlement <v>, lifecycle <v>) [evidence <n> revs, latest <ev-id>; drift <state>]` line per record, last in the output) and JSONL (`qualification: {"records": [...]}` per-record objects with `harness`, `surface`, `progress`, per-column `verdict` + `evidence` ids, `evidence_revisions` (= current revision), `latest_evidence`, `drift_triggers` (`executable_digest`, `config_digest`), `drift`, `next_test`). Unavailable → `qualification: unavailable (<reason>)` / `{"status": "unavailable (<reason>)"}`, exit 0; existing-but-unseeded → `(no records)` / `{"records": []}`.
- **For dependents**: doctor reads via `qualify.OpenReadOnly` + `List` only (10 s bound), never migrates or seeds; every `OpenReadOnly`/`List` error maps to `unavailable`, never empty and never an error exit.
- **For dependents**: plain `drift` state runs `CheckDrift` against unobserved digests — unknown pins read `clean`, established pins read `<field> unobserved (pinned ...)`; `latest_evidence` is the max-`At` evidence id across columns (`none` when empty). Task 7 asserts seven records from JSONL `qualification.records`; Task 8 documents the `^<harness> × <surface>:` plain shape.
- **Deviations that change a later task's inputs**: none (extra `TestDoctorQualificationEmptyHonest` pins the design §6 empty read; no production shape beyond the design).

## Task 7 — End-to-end packaged-binary tests

- **Produces**: `tests/e2e/qualification_test.go` with `TestE2EDoctorShowsSevenRecords` (seeded home → JSONL `qualification.records` holds the seven v2 §7.2 harnesses, claude-code blocked, six planned, Task 6 per-record shape), `TestE2EStrictStillBlocked` (packaged `run --adapter claudecode --billing subscription-only` → exit 3, `run.result` reason `entitlement_qualification_unavailable`), `TestE2EDeclaredUnaffected` (packaged declared run against a fixture `claude` → exit 0, `ready_for_review`). No production code changed.
- **For dependents**: the declared e2e's fixture native (`installFakeClaude`: `--version` → `2.1.284 (Claude Code)`, `auth status` → max/firstParty JSON, launch → init with `apiKeySource: none` + success result) plus `childEnvWithPath`/`runBin` are reusable for any later packaged claudecode run; Task 9's e2e needs only the fake adapter, not this fixture. The fixture skips on Windows (probe refuses by design).
- **For dependents**: "as before this spec" was verified by running both scenarios against a base binary built from the pre-spec commit — strict exit 3 + reason and declared exit 0 + `ready_for_review` identical on both; a launch-broken fixture ends the declared run `failed`, and `doctor` on an unseeded home reports zero records, so all three tests discriminate.
- **Deviations that change a later task's inputs**: none.

## Task 8 — User guide and ADR-0011

<!-- pending -->

## Task 9 — Production registry seeding on admitted runs

- **Produces**: `supervisor.Run` calls `qualify.EnsureSeeded` after `journal.Open` (before the run dir exists); seed failure returns `persistence_unavailable` and admits nothing. Exactly the design §3 trigger; no new API.
- **For dependents**: every `Run` (`run` and demo paths; `recover` reattaches without calling `Run`) now ensures the seven seed rows first — later specs can rely on the seed existing wherever a run was admitted, and must keep `Run`'s pre-admission path migration-safe (the trigger runs on the just-opened journal).
- **For dependents**: `admission.Decide` alone creates no database — tests staging an unseedable state must `journal.Open` (to migrate) before tampering, then drop/close via raw SQL.
- **Deviations that change a later task's inputs**: none.
