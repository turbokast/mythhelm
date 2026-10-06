# Refinement log: tui-tape-recording (MH-32)

Refined 2026-10-06 in the deliver-backlog run 2026-10-05T14:45:12Z (Muse Code session, no Claude subagents; fresh-context assessors via native child agents with the refine-spec prompt).

dispatched=2 returned=2 failed=0

## Round 1 — Verdict: Needs work

Scores: D1 PASS, D2 PASS, D3 PASS, D4 PASS, D5 NEEDS_WORK, D6 NEEDS_WORK, D7 PASS, D8 NEEDS_WORK.

Auto-fixable (both applied, intent-preserving):
1. D5 — Open Q2 (L74) and Impacted components (L93) labelled a possible `internal/tui/` fixture "core domain"; per knowledge/domains.md `internal/tui/` is the tui domain. Reworded both lines to split by path (tui vs core).
2. D6/D8 — AC-2.1 (L50) "CI shall fail a recording whose artifacts drift from a regen" read to include the rendered GIF contradicted N2 (no render-compare CI) and forced the designer to guess the compare scope. Narrowed to "whose transcript drifts", with the N2 binding named in-line.

Human-required: none. Missing context: none. The assessor re-verified the load-bearing citations against the tree (0 `Run` lines in tui.tape, manifest names demo only, no TUI transcript/artifact, tui-slice and docs-site-demos citations, docs.yml:82, PRs #141/#146/#148, no sibling-spec conflict).

Note: one dispatch attempt before round 1 was aborted — the child was admitted with a restricted tool grant (no file reads) after a prompt redirect, returned no verdict, and was superseded by a clean re-dispatch. It never assessed and is not counted as a round; no verdict rests on it.

## Round 2 — Verdict: Ready

Scores: all eight dimensions PASS. Auto-fixable: none. Human-required: none. Missing context: none. The assessor independently re-opened citations (tui.tape, manifest.json, docs-site-demos requirements/design, tui-slice retrospective, docs.yml/record.sh absence, empty specs/todo/ and specs/in-progress/) and confirmed the round-1 fixes.

## Remaining notes for /spec

- Open Q1–Q5 stand as the bounded design-time agenda: Q1 (default a: tui.gif), Q3 (default a: one declared revision), Q5 (default a: user-guide Quickstart + README) carry defaults; Q2 (fixture vs keystrokes; investigate against internal/tui/ and the launch matrix) and Q4 (transcript shape; investigate against VHS output behaviour) name investigation targets.
- The line-73 drift note (Task 7 handoff cited a grep check for no-filename-referenced that exists in neither docs.yml nor record.sh) is confirmed accurate; /spec decides confirm-or-drop.
