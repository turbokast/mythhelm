# tui-observatory-view — Refinement log

Status: **Blocked, not moved to `refined/`.** The dispatch ledger is `dispatched=3 returned=2 failed=1`: one assessor round ended on a rate-limit error before it produced a report, and `.claude/skills/refine-spec/SKILL.md` step 5 treats a failed round as Blocked. Re-run `/refine-spec tui-observatory-view` (a fresh ledger) to move the spec; the fixes below are already in `requirements.md`.

## Round 1 — returned — Needs human input

Scores: D1 PASS, D2 NEEDS_WORK, D3 NEEDS_WORK, D4 PASS, D5 NEEDS_WORK, D6 NEEDS_WORK, D7 NEEDS_WORK, D8 NEEDS_WORK.

Applied (18 auto-fixable items): Windows detach verdict corrected to PARTIAL and gated (AC-6.10); AC-2.3 states the full fallback order and number; key-bar degradation order so the attention slot is never trimmed (AC-4.6, 4.8); Ctrl-C and `q` behaviour by mode and state in one table (AC-6.12), resolving the AC-6.5 / AC-6.8 conflict; focus-coloured text sites listed and changed (AC-1.5); NFR-2 reworded to bounded work; Definition of Done extended (AC-7.2 to 7.4, NFR-1 to 4, decision records, demo re-record); 13-state vocabulary table (AC-3.4); header run ID, elapsed format and clock-stopping states (AC-4.1); dimming defined as `text_muted` (AC-5.1); tag and citation corrections; `tui-tape-recording` named; terminal-close behaviour (AC-6.13); late terminal answer discarded (AC-2.7).

Human-required, folded in as open questions with defaults: the I21 execution limit after a stop-less detach (OQ-20), the exit code of a deliberate detach with the missing v2 §15.3 sentence (OQ-9), and whether FR-6 ships first (OQ-21).

## Round 2 — failed

The assessor stopped on a rate-limit error (HTTP 429) before reporting. Counted as `failed`; its silence is not read as "no issues".

## Round 3 — returned — Needs human input

Scores: D1 PASS, D2 NEEDS_WORK, D3 NEEDS_WORK, D4 NEEDS_WORK, D5 NEEDS_WORK, D6 NEEDS_WORK, D7 NEEDS_WORK, D8 NEEDS_WORK. No FAIL.

Applied (21 auto-fixable items): ✓ (U+2713) is banned by the existing font test, so AC-3.2 amends both checks and keeps failing cases; AC-7.1 anchored to captured output instead of an empty `testdata/`; stop gate stated state by state with the `supervisor.Stop` grounding, and `s` before launch delivered through `Hooks.Interrupt` (AC-6.9); AC-6.4 uses the full confirmed-stop rule; detach keyed on the adapter's `worker_detachment` record, never `GOOS` (AC-6.10); the worker-side deadline ends the run `blocked` / `envelope_deadline_exceeded` (AC-6.11); an unsupervised run is labelled and its clock frozen (AC-4.12); realistic late-answer fixture (AC-2.7); termenv's constant 5 s wait means the bounded query is project code (AC-2.3, AC-7.4); evidence line format with the receipt's wording (AC-4.5); lane identity includes the execution profile and the raw stream is sanitised (AC-4.4); AC-6.6 prints a recover line; a 59x24 golden; three wrong grep outputs corrected; conflicts with `windows-process-ownership` and `contained-execution-profiles` named, including that spec's Ctrl-C criterion (OQ-1).

Human-required, folded in as open questions with defaults: scope split of FR-6 (OQ-21, unchanged) and whether the overlay border must meet 3:1 (OQ-22).

## Remaining notes for /spec

- The fixes of round 3 were not re-assessed.
- D4 is NEEDS_WORK: slice 1 touches tui, cli, supervisor, workers, docs and `go.mod` and may exceed 15 tasks; if `/spec` produces more, split FR-6 into its own spec ahead of the visual work (OQ-21).
- Maintainer questions with defaults: OQ-1, 3, 4, 7, 8, 9, 14, 15, 18, 19, 20, 21, 22.

## Fresh run — 2026-10-12 (takeover, Muse Code) — Ready

dispatched=1 returned=1 failed=0
Verdict: Ready after 1 round (every dimension PASS; no auto-fixable, human-required or missing-context items).

The previous ledger above (`dispatched=3 returned=2 failed=1`, Blocked on the rate-limited round) is superseded by this fresh ledger, as the log's own instruction required. The assessor checked the maintainer's per-item Decisions (OQ-1, 3, 4, 7, 8, 9, 15, 18, 19, 20, 21, 22) as settled and did not re-ask them; OQ-26 to OQ-30 remain author defaults with recommendations that do not block `/spec` (requirements line 239). Spot-checked citations held at source (theme schema, overlay takeover, inert exit dialog, closed glyph set, structural absence claims). The spec moved `unrefined/` -> `refined/` on this result. Remaining notes for /spec are the D4 row above (slice 1 may exceed the task budget; split FR-6 first if it does).

Review fix after Ready (PR #456, CodeRabbit): AC-2.3 now scopes the `COLORFGBG` fallback to Unix, and AC-2.6's exact help words no longer promise a `COLORFGBG` fallback on Windows, where termenv returns the dark default without consulting it. Wording only; no scope or decision changed.
