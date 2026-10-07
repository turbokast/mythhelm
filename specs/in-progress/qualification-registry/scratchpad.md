# Qualification Registry — Scratchpad

> Open questions and research from authoring. Each implementing task reads this file when it starts and adds an entry under Discoveries when it finishes.

## Open questions

| # | Question | Default in this spec | Owner / blocks |
|---|---|---|---|
| Q1 | Which harness supplies the first included-only route? | Claude Code print/stream-json (existing probe; MH-12 builds on it) | Maintainer via Q-11 / nothing (design proceeds on default) |
| Q2 | What authorised live tests may run for FR-3, whose allowance covers them? | Fixtures only, no live tests; strict admission keeps blocking | Maintainer via Q-11 / MH-12 live path, not this spec |
| Q3 | Where do records live? | SQLite `mythhelm.db`, new `qualification_records` table (ADR-0003, v2 §5.1) | Settled at design (D1 + ADR-0011) |
| Q4 | `doctor` section or new subcommand for AC-5.3? | `doctor` section (D5) | Settled at design |

## Research notes

- v2 §18 holds only §18.1–§18.3; gates at §18.3. Issue #31's older section numbers are superseded by the MH-10 card's v2 §§4, 7–8, 14, 17–18.
- Absence of a registry store grounded by `grep -rin "registr\|qualif" internal/ adapters/ cmd/ --include='*.go'` (2026-10-07): descriptor strings, block codes, default-false flags, TUI palette only.
- `journal.Open` runs embedded `migrations/*.sql` by `PRAGMA user_version`; migration SQL must stay platform-neutral, hence the lazy Go seed (D2).
- `doctor` must stay read-only and never query auth status (`internal/cli/doctor.go:122-124,170-193` lineage); the qualification section uses `OpenReadOnly`.
- Per-column `proven|not-proven|unknown` is this spec's invention to answer "is this column established"; it is distinct from the v2 §4.2 progress scale by design (D3).

- MH-12 (first live writer) must wire the production `InvalidateOnDrift` call and settle restore-without-retest: consult-time drift blocking re-admits a restored matching state without retest, which is moot here (no proven records in production DBs) but load-bearing once live records exist.
- `journal.OpenReadOnly` reports `ErrNoDatabase` for a missing dir and a missing file alike (`internal/journal/projections.go:350-354`); `qualify.OpenReadOnly` stats the dir itself to tell them apart.

## Discoveries

- Task 1 (PR #200): the progress scale holds no `unknown`, so `DecodeRecord` treats a missing `Progress` as a field-naming validation error rather than defaulting it; every other scale defaults missing input to its unknown value. Digest/key goldens were oracle-checked with `sha256sum` over the exact canonical bytes. No store callers exist yet: Tasks 2–3 must route key hashes through `qualify.KeyHash` and stored-JSON reads through `qualify.DecodeRecord`.
