# Contained Execution Profiles — Scratchpad

> Open questions and research from authoring. Each implementing task reads this file when it starts and adds an entry under Discoveries when it finishes.

## Open questions

| # | Question | Default in this spec | Owner / blocks |
|---|---|---|---|
| 1 | macOS/Windows native mechanism qualification (Seatbelt profile? AppContainer?) | v1 refuses both with named missing coverage; follow-up spec qualifies | Maintainer / nothing in v1 |
| 2 | Destination-aware network blocking (netns plumbing / eBPF) | v1 = proxy pin + disclosed direct-egress residual; no user-requirement mechanism, so no refusal fires for users wanting no-egress (see row 3) | Maintainer / nothing in v1 |
| 3 | `--require-*` flag for users who need the no-egress guarantee as a single command | Not in this spec (honesty register); MH-22 or follow-up may add | Maintainer / nothing in v1 |
| 4 | Exact first-party endpoint host:port for the proxy allowlist | Resolved from the qualification record at admission; unknown endpoint blocks contained admission (I02) | Task 3+5 / proxy allowlist |
| 5 | Journal migration number: 0003 in the current tree (only 0001/0002 at `SchemaVersion = 2`); MH-21 streams plan 0003/0004 and MH-16 pins `0005_ledger.sql` on that chain | Task 7 takes the next free number at its start; whoever lands second renumbers | Task 7 |

## Research notes

- Q1–Q3 decided by the maintainer 2026-10-08 (requirements.md Open
  Questions): per-platform native refusing where unavailable; shared
  mechanism for `inspect`; evaluator isolation here only.
- Go `os/exec` supports Linux user+mount namespaces via
  `SysProcAttr{Cloneflags, UidMappings, GidMappings}` plus
  `/proc/self/setgroups` handling; mount setup itself needs the `__contain`
  re-exec because Go has no fork/exec hook (D1).
- `x/sys/unix` (already vendored at v0.48.0) exposes `Mount`, `Unshare` and
  `Prctl` for the `EnterLinux` path; no new module dependency planned.
- Verification already runs post-reap (`pipeline.go` reap wait precedes the
  verify stage), so the evaluator threat is candidate content, not a live
  worker — the check policy (§2.7) targets exactly that.
- MH-22 (`specs/*/protected-acceptance/`) consumes receipt
  `execution_bundle.boundary`, `verification.evaluator`, and the new
  verification-row evaluator columns; it also sequences behind MH-21
  (TaskRevision shape), which this spec does not need.

## Discoveries
