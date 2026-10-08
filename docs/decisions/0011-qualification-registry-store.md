# 0011. Qualification registry store: a table in mythhelm.db

- Status: accepted
- Date: 2026-10-07

## Context

The qualification registry (MH-10) needs versioned per-harness records with
immutable revisions: one record per harness × surface × executable build ×
OS/arch × entitlement class × trust profile, each revision content-addressed
(master spec v2 §7.1). The open question was where those records live: the
SQLite state database settled by ADR-0003, versioned files under the state
directory, or both (spec question Q3).

ADR-0003 already places durable orchestration state — runs, attempts,
trust grants, billing declarations and the event journal — in `mythhelm.db`
with additive numbered migrations and `PRAGMA user_version` versioning.
Master spec v2 §5.1 requires a single canonical ledger to own orchestration
state (I23): exports, indexes and Markdown views must not become competing
task stores. A file-backed record set would be exactly that: a second store
with its own writer discipline, locking, backup story and corruption modes,
alongside the SQLite journal every other surface reads.

## Decision

Qualification records live in `mythhelm.db`, in the new
`qualification_records` table created by additive migration
`0002_qualification.sql` (keyed by key hash plus revision, current row marked
by a null `superseded_at`). Versioned files are rejected as a competing
store per I23: the table is the one queryable record store admission and
`doctor` read through the `internal/qualify` registry. The per-harness
Markdown note (`adapters/claudecode/COMPATIBILITY.md`) stays only as a
per-harness view, never as the record of qualification.

The migration follows the ADR-0003 policy: additive only, `0001_init.sql`
never edited, `SchemaVersion` bumped 1→2, `STRICT` table. Old revisions stay
immutable; a material change writes a new revision.

## Consequences

- One writer, one locking story, one backup story: the registry inherits
  SQLite's transactions and the `VACUUM INTO` pre-migration backup, instead
  of growing a parallel file protocol.
- Readers share one contract: every store caller derives key hashes through
  `qualify.KeyHash` and decodes rows through `qualify.DecodeRecord`, so
  admission, `doctor` and later surfaces cannot drift apart.
- Anything that needs the records without SQLite must go through the
  registry API; there is no file tree to scrape, which keeps I23 honest by
  construction.
- Tests that pin this behaviour: `TestMigration0002Applies` (migration
  applies, `0001_init.sql` byte-identical, fresh opens report version 2),
  `TestRecordRevisionImmutability` (new revisions leave old rows unchanged,
  both digests recompute), `TestAbsentIsNotFound` (absence is `ErrNotFound`,
  never a zero record) and `TestSeedHasSevenHarnesses` (the seven v2 §7.2
  seed records).
