## V2 Contract Vocabulary — Scratchpad

> Open questions and research from authoring. Each implementing task reads this file when it starts and adds an entry under Discoveries when it finishes.

## Open questions

| # | Question | Default in this spec | Owner / blocks |
|---|---|---|---|
| OQ-9 | v2 envelope coexistence: same `journal` table with v2 accepted post-migration, or v2 rejected until migration completes? | Same table, v2 accepted post-migration, v1 decoded under v1 forever (design D7) | Settled at design; implemented by `supervisor-migration`, blocks nothing here |
| Q1 | Do the authored per-code default dispositions (design §5, D9) survive contact with the supervisor runtime? | Yes as specified; any change is a recorded spec deviation in the consuming task | `supervisor-service` designer / nothing (design proceeds on default) |
| Q2 | Does MH-22 need anything beyond `TaskRevision` + contract digest from this spec? | No: binding-ready revision + digest ship in Task 2 (epic plan Q-14) | MH-22 `/spec` / nothing here |

## Research notes

- v2 §4.5 lists 24 required codes (counted from source), not the 26 the phase-1 findings claimed; the catalogue test pins the exact set so a miscount fails loudly.
- `Append` checks `event_id` duplicates before generation before sequence (`internal/journal/journal.go:291-316`); the pure validators keep that order so spec 3's extension is mechanical.
- Receipt-file `schema_version = 2` is at `internal/supervisor/apply.go:195`; AC-1.3 distinguishes contract version by type name (`Envelope` vs receipt map), never by the bare number.
- `v2contract` must not import `journal` or `qualify` (SQLite driver linkage would break NFR-4); test-only imports of either are allowed and cycle-free (`journal`/`qualify` never import `v2contract`).
- `go list -deps` net-absence is assertable for this package (stdlib-only imports), unlike `qualify` whose `journal` import links `net` transitively.
- The v2 §4.1 table has 15 rows including optional `Release`; AC-1.1 covers the other 14. `Release` stays deferred until its first producer exists.

## Discoveries

- (Implementing tasks append one entry each: what was learned, with file:line or command evidence.)
