# docs-site-demos — Retrospective

## Review Summary

- **Range**: e0ecdbc..7b9f971 (PRs #124, #125, #137, #138, #139, #140, #142; fix PRs none)
- **Reviewer**: orchestrator, inline (no code-reviewer agent in this client; brief per finalize-spec-review)
- **Findings**: critical 0, important 0, suggestion 2 (confirmed); rejected 0
- **Open critical**: 0
- **Vendor review**: skipped (codex/muse/jev all unavailable: disabled)

| Severity | Finding | Anchor | Disposition |
|---|---|---|---|
| suggestion | GIF-to-tape binding stays transcript + manifest revision + human review; no render-compare CI, so a swapped-but-plausible GIF passes every check | `docs/demos/demo.gif` | kept residual: disclosed in design §8 honesty register, delivery Q-10 and reconciliation §4; render-compare is a follow-up if recordings multiply |
| suggestion | Design §5 says the site shows "planned" where the TUI recording will go, but no rendered site page does — the "planned" label lives only in `docs/demos/README.md`, which has no front matter and is not a site page | `docs/user-guide.md:36-38` | kept: no false claim exists (I14 holds — the site presents no TUI recording at all); add the slot when MH-32 lands its real recording rather than churning a placeholder now |

Foreign changes in range: 1dad146 docs: adopt the integrated MYTHHELM v2 product specification (#126): README.md (v2 paragraph + site URL; attributed, no finding). PRs #141 (product/ only) and #146 (reconciliation.md only) merge inside the range but touch none of the reviewed files.

Acceptance criteria (spec-wide): AC-1.1 (site on every main merge + guides) met by #138/#139, built-site proof in CI; AC-1.2 (G10 items from nav) met by #138, mirrors-rendered assertion in CI; AC-1.3 (DCO + good-first-issue, no paid subscription) met by #138 (`docs/contributing.md:38-43`); AC-1.4 (no owned runtime service, no site fetch) met by #139, verified: no Go reference to the site host; AC-2.1 (transcript-reproducible tape) met by #125, enforced by Task 6 regen `diff`; AC-2.2 (binary's labels kept) met by #125, `normalize.sed` matches design §3 exactly; AC-2.3 (README + site embed with provenance) met by #140, all three embeds captioned with tape + `record.sh`; AC-3.1/3.2/3.3 scaffold met by #137 (zero executable lines in `tui.tape`, full tape filed as MH-32); AC-4.1/4.2 met by #125 manifest + #142 CI checks, proven live by failing probes #143/#144/#145; NFR-1 (fork-safe) met: 0 `secrets.*`, least-privilege permissions; NFR-2 met on Linux (`record.sh` 20.9s, free tooling), macOS timing unmeasured with no macOS VHS leg in CI (disclosed in design §8).

## Acceptance

| Criterion | Result | Evidence |
|---|---|---|
| AC-1.1 site on every main merge + guides | met | PR #138 (pages) + #139 (workflow); `_site` mirror assertions in `docs.yml` |
| AC-1.2 G10 items reachable from nav | met | PR #138; all 7 nav routes resolve; build asserts mirrors rendered |
| AC-1.3 DCO + good-first-issue, no paid subscription | met | PR #138; `docs/contributing.md:38-47` |
| AC-1.4 no owned runtime service, no site fetch | met | PR #139; no Go source references the site host |
| AC-2.1 transcript-reproducible tape | met | PR #125; byte-identical regen; CI regen `diff` (#142) |
| AC-2.2 binary's labels kept | met | PR #125; `normalize.sed` matches design §3 exactly |
| AC-2.3 README + site embed with provenance | met | PR #140; 3 embeds, each captioned with tape + `record.sh` |
| AC-3.1/3.2/3.3 TUI recording | met (scaffold per DoD) | PR #137; 0 executable lines; full tape filed as MH-32 (PR #141) |
| AC-4.1 revision declared, stale fails | met | PR #125 manifest + #142 CI; stale probe #143 failed as designed |
| AC-4.2 qualifiers declared, unqualified fails | met | PR #125 manifest + #142 CI; probes #144/#145 failed as designed |
| NFR-1 fork-safe CI | met | 0 `secrets.*`, pinned `uses:`, least-privilege permissions |
| NFR-2 record under 10 min, free tooling | met (Linux) | `record.sh` 20.9s; macOS timing unmeasured (design §8 discloses no macOS VHS leg) |

## Deviations

- Task 1: None.
- Task 2: review round touched `GOVERNANCE.md` (absolute MAINTAINERS link) and `design.md` (adapter wording) outside Files — recorded in the task entry with justification; byte-mirror constraint forced the root fix.
- Task 3: None. (Entry + handoff written by the orchestrator after the worker stopped at the Pages-disabled build failure.)
- Task 4: tape uses `Type`+`Enter` (VHS 0.12.1 has no `Run`); `binary_version` is the clean-tree version string, not the schema example's `devel` — recorded in the task entry; contract unchanged.
- Task 5: user-guide embed page-relative vs README root-relative; review round added the `docs/index.md` embed — recorded in the task entry; design §2 assigns it to Task 5.
- Task 6: check order is revision → transcript → qualifiers (design lists transcript second) for fail-fast; one-sentence `docs/automation.md` touch outside Files — recorded in the task entry.
- Task 7: None. (Limitations re-run N/A: Task 2 unmerged in the parallel group.)
- Review: design §5's site-visible "planned" TUI slot was never built and no task recorded the skip — found by review; kept as suggestion (MH-32 lands the real recording).

## CI history

- Docs / Build docs site on PR #139 (@b0c7277): real — failed because the Pages source was not GitHub Actions yet; Docs checks passed on the same sha. Mechanism: operator setup ordering (run 37293185337 failed pre-enablement; later shas green after the maintainer enabled Pages). No same-sha rerun; not nondeterminism.
- All other task-PR runs (PRs #124, #125, #137, #138, #139, #140, #142): success or cancelled (superseded pushes) only. No nondeterministic failures on task branches. No `fail`/`retry` rows in the run-events log.

## Effort

dispatched=7 returned=7 failed=0; attempts 7 over 7 tasks; first-pass 7/7; review rounds 0 (worker) + orchestrator-adjudicated bot threads on #140, #142, #146; wall-clock unknown → 2026-10-05T12:52:15Z (no `run_start` row: run adopted across sessions)

| Task | Agent | Attempts | Review rounds | PR | Merged | First pass |
|---|---|---|---|---|---|---|
| 1 | go-implementer | 1 | 0 | #124 | yes | yes |
| 2 | go-implementer | 1 | 0 | #138 | yes | yes |
| 3 | release-engineer | 1 | 0 | #139 | yes | yes |
| 4 | go-implementer | 1 | 0 | #125 | yes | yes |
| 5 | go-implementer | 1 | 0 | #140 | yes | yes |
| 6 | release-engineer | 1 | 0 | #142 | yes | yes |
| 7 | go-implementer | 1 | 0 | #137 | yes | yes |

## Lessons

- What worked: failing probes as check-in proof — Task 6's stale/unqualified/unknown-revision probes (#143/#144/#145) showed each new CI check fails before it passes (PR #142).
- What worked: bot review on spec-doc PRs catches real errors — all 3 CodeRabbit threads on the v2 reconciliation (#146) were actionable, including two wrong AC crosswalk rows I had written.
- What worked: per-task Files discipline with recorded out-of-Files justifications (Tasks 2, 5, 6) kept every touch attributable at finalize review.
- What worked: D7 declared-revision staleness survived implementation — the manifest still pins e0ecdbc across later merges with CI green.
- What to change: design §5's site-visible "planned" claim had no task owner, so no task built it or recorded skipping it (review suggestion above; proposal P-docs-site-demos-1).
- What to change: NFR-2 names Linux and macOS but only Linux was timed; the honesty register discloses the missing macOS leg, but nothing flags the platform gap as an NFR caveat.
- What to change (adjacent, out of scope): product PR #141 (product/-only diff) saw Go/windows-latest fail in `TestConcurrentAppendsFromTwoHandles` (`SQLITE_BUSY`) then pass on a same-sha rerun (run 37300543298). Mechanism unknown; needs a mechanism before any further label. Second windows-only Go incident after #103's lost runner (different mechanism: runner loss).

## Proposals

- P-docs-site-demos-1 — spec-decomposition: user-visible design claims need a task owner
