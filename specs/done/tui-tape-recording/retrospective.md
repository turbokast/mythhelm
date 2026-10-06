# tui-tape-recording — Retrospective

## Review Summary

- **Range**: e93f946..7bd8b1e (PRs #163, #164, #168; fix PRs none)
- **Reviewer**: code-reviewer
- **Findings**: critical 0, important 0, suggestion 3 (confirmed); rejected 0
- **Open critical**: 0
- **Vendor review**: skipped (codex/muse/jev all unavailable — disabled)

| Severity | Finding | Anchor | Disposition |
|---|---|---|---|
| suggestion | Demos README normalization paragraph documents only the 9 demo-era rules; the 5 TUI rules undocumented | `docs/demos/README.md:65-68` | fixed in this finalize PR (update-docs) |
| suggestion | Record-command docs omit the squash-merge repin trap (plain record.sh refuses at every committed post-merge state) | `docs/demos/README.md:16-20` | fixed in this finalize PR (update-docs) |
| suggestion | Transcript regen uses recordings[0]'s commit while len(pins)==1 is asserted only later; diverged pins would regen TUI at the demo commit (self-detecting via diff, confusing message) | `.github/workflows/docs.yml:66,113` | filed as issue #186 (workflow change outside finalize scope) |

Foreign changes in range: none.

## Acceptance

| Criterion | Result | Evidence |
|---|---|---|
| AC-1.1 reproducible regen | met | Double-run diff 0, per-take PIPESTATUS, --check fail counterfactual, network tripwire empty (PR #163) |
| AC-1.2 ladder + key flows | met | Rung takes 140/120/90/70 + accessible, 13 executable lines, no q, eyeball + frame spot-check |
| AC-1.3 on-screen verification | met | t=6 spot-check: verified d3aef41/demo:passed; GIF 2.9MB 1650x620 present |
| AC-2.1 pins + drift failure | met | Headers/manifest agree at 179bba4 + v0.12.1, len(pins)==1, per-recording revision loop, biting stale probe |
| AC-2.2 qualifiers enforced | met | 7 capabilities × 1 qualifier, all re-run green, TestDoesNotExist probe fails |
| AC-2.3 transcript byte-match | met | 116-line transcript, exact label counts re-verified (8/1×5/7) |
| AC-3.1 docs-site embed | met | User-guide embed + caption, page-relative, renders (PR #164) |
| AC-3.2 README + demos README | met | Embeds + caption, zero planned in recordings table |
| NFR-1 under 10 min | met | 3m29s / 3m37s, free tooling only |
| NFR-2 size budget | met | 2.9MB < ~5MB, no follow-up |

## Deviations

- Task 1: demo.tape header-pin lines outside Files (one-revision re-record per D3; header-only leg verified) — recorded in the entry.
- Task 2: manifest.json + both tapes pin lines outside Files (--repin on main tip; plain record.sh refuses post-merge) + demos README demo-row pin refresh — recorded in the entry.
- Task 3: single combined transcript step instead of two (demo lines byte-identical within) — recorded in the entry.
- Review found no unrecorded divergences.

## CI history

- Docs checks on merge 179bba4 (#163): real, manifest revision check failed — pins named the pre-commit HEAD and the squash-merge moved main past it; repaired by task 2's --repin to 179bba4 (same mechanism as the record.sh squash-merge trap). No retry-green involved: fail then fix, distinct commits.
- CI + OSV-Scanner + PR title on PR #163 @f696315: infra, runs cancelled — superseded by the review-round push; full set success on the later head.
- CI + OSV-Scanner on PR #164 @6a5feaf: infra, runs cancelled — superseded by the review-round push; full set success on the later head.
- CI + PR title on PR #168 @37bdbf8: infra, runs cancelled — superseded by push; full set success on the later head.
- No failure+success on the same headSha: no nondeterministic failure. No fail/retry rows in the run-events log.

## Effort

dispatched=3 returned=3 failed=0; attempts 3 over 3 tasks; first-pass 3/3; review rounds 2; wall-clock 2026-10-06T11:29:48Z → 2026-10-06T13:46:35Z

| Task | Agent | Attempts | Review rounds | PR | Merged | First pass |
|---|---|---|---|---|---|---|
| 1 | go-implementer | 1 | 1 | #163 | yes | yes |
| 2 | go-implementer | 1 | 1 | #164 | yes | yes |
| 3 | release-engineer | 1 | 0 | #168 | yes | yes |

## Lessons

- What worked: biting scratch-branch probes as acceptance — all three task-3 probes failed for the right reason on their branches (PRs #165-167) and the PR itself stayed green, proving the checks discriminate.
- What worked: the record.sh↔docs.yml intentional mirror is self-detecting — any pipeline drift fails transcript-freshness, so the duplication carries its own alarm.
- What worked: pin-only outside-Files touches with header-only verification legs kept the one-revision discipline honest across two re-records.
- What to change: the squash-merge pin trap bit twice (red main after #163, forced --repin in #164) before its mechanism was written down anywhere but the spec handoff — now documented in docs/demos/README.md by this finalize (Review Summary finding 2). No proposal: the flow works, and auto-repin would weaken the honesty check it serves.

## Proposals

- None

Acceptance (spec-wide, adjudicated at head 7bd8b1e): AC-1.1 met (double-run diff 0, per-take PIPESTATUS, counterfactual recorded, network tripwire empty — re-verified); AC-1.2 met (rung takes 140/120/90/70 + accessible, 13 executable lines, no q, eyeball recorded, frame spot-check t=6/27/73.7); AC-1.3 met (on-screen verified d3aef41/demo:passed at t=6 per recorded spot-check; GIF 2,922,664 bytes 1650x620 present); AC-2.1 met (header/manifest pins agree at 179bba4 + v0.12.1, len(pins)==1 kept, per-recording revision loop, biting stale probe); AC-2.2 met (7 capabilities × 1 qualifier, all 7 re-run green this review, TestDoesNotExist probe fails); AC-2.3 met (116-line transcript byte-matched, exact label counts re-verified: SCRIPTED DEMO 8, goal/run-state/candidate/next-action/actions 1, attempt 7); AC-3.1 met (user-guide embed + caption, page-relative, renders); AC-3.2 met (README + demos README rows, zero planned in recordings table); NFR-1 met (3m29s/3m37s, free tooling); NFR-2 met (2.9MB, no follow-up); DoD met (cross-platform CI rests on the merged PRs' runs). Seams: task 2 consumed task 1's contract verbatim (--repin 09fb9108→179bba4); task 3 mirrored the §3 pipeline line-for-line; record.sh↔docs.yml mirror is self-detecting drift-wise; repin-on-main-tip flow recorded. Invariants I07/I09/I14/I17 + fork-safe lane all hold (labels verbatim, pins exact, qualifiers enforced, proof on-screen, no secrets). Design vs shipped: no unrecorded divergences (combined transcript step, 5 sed rules, pin-only touches all recorded). Stale documentation: the two README gaps above (fixed here); docs-site-demos records correctly untouched as history.
