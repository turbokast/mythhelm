# Refinement log: strict-lint-set (MH-20)

Refined 2026-10-06 in the deliver-backlog run 2026-10-05T14:45:12Z (Muse Code session, no Claude subagents; fresh-context assessors via native child agents with the refine-spec prompt).

dispatched=3 returned=3 failed=0

Verdict: Good enough after 3 rounds (no FAIL; round-3 NEEDS_WORK items were mechanical citation nits, fixed post-round by the refiner with tree verification — no 4th assessment round per the 3-round cap).

## Round 1 — Verdict: Needs human input

Scores: D1 PASS, D2 NEEDS_WORK, D3 NEEDS_WORK, D4 PASS, D5 NEEDS_WORK, D6 NEEDS_WORK, D7 PASS, D8 NEEDS_WORK.

Auto-fixable (all four applied, intent-preserving):
1. D2/D6/D8 — AC-2.3 (L50) demanded the lint job green "on Linux, macOS and Windows", but the CI lint job runs only on `ubuntu-latest` (`ci.yml:73`; the multi-OS matrix is the Go job) — uncheckable as written, and forcing a matrix change contradicted L89 (`ci.yml` read-only unless trials prove otherwise). Reworded to the lint job green on its runner, with multi-OS green covered by the Definition of Done.
2. D2 — NFR-1 (L64) "existing job budget on all three runners": no `timeout-minutes` exists in `ci.yml` and the lint job is single-runner. Scoping fixed; budget question resolved as below.
3. D5 — FR-4/AC-4.1 `[§19.3]` tags dangled: the header resolves § numbers against v2, but v2 §19 has no subsections. Qualified as Revision 1.1 pins (matching openssf-badge's master-spec citations).
4. D3 — L20 cited `tasks.md:53` for project 15212; it is at `tasks.md:31-32`. Fixed.

Human-required (resolved without asking, by naming the decider instead of inventing the number):
- Round 1 asked for the concrete CI-time/false-positive budget behind NFR-1 and AC-1.2, arguing any agent-written number invents policy. Resolution: no number was invented — AC-1.2 and NFR-1 now route the cost judgment to the maintainer at the spec checkpoint (per-candidate measurements mandatory in the trial matrix), the same checkpoint-decider pattern the sibling specs' Open Questions use. Round 2 accepted this routing ("the designed human-input point — no additional pre-design human answer is needed") and required only that the checkpoint-agreed budget be recorded as a number and enforced in-tree, which round 2's fixes did.

Plus one self-spotted carry-over (same staleness shape as the MH-18/MH-19 rounds): L18 "No spec covers MH-20" was stale — the card is specced by this spec (`product/backlog.md:56-61`). Re-grounded with the create-spec-time note preserved.

Missing context: none.

## Round 2 — Verdict: Needs work

Scores: D2 NEEDS_WORK, D7 NEEDS_WORK, D8 NEEDS_WORK, all others PASS. All three fixed:
1. D2 — NFR-1 set no measurable target (checkpoint "confirms acceptable" but recorded/enforced no number). Reworded: /spec records the agreed budget in minutes, the implementer enforces it via `timeout-minutes` on the lint job, the named check compares measured time against it.
2. D7 — L84 conflicts line omitted the refined-sibling sweep. Extended (refined siblings checked-disjoint, unrefined holds only this spec, todo/in-progress/unfinalized/archived empty).
3. D8 — Q1 had no default while blocking FR-1. Default (b) curated shortlist set, with (a) full-catalogue rejected as the default for unbounded sweep size; verdicts stay checkpoint decisions per AC-1.2.

Human-required: none. Missing context: none.

## Round 3 — Verdict: Needs work (no FAIL)

Scores: D3 NEEDS_WORK, D7 NEEDS_WORK, all others PASS. Three mechanical nits, fixed post-round by the refiner with tree verification (no 4th round):
1. D7 — L84 reciprocity claim false for tui-tape-recording (zero "lint" hits; its conflicts line covers only todo/in-progress — verified). Reworded: version-numbering and release-tagging reciprocally name this spec; tui-tape-recording checked and disjoint by subject. (tui-tape-recording itself was not reopened: already refined Ready; a cross-reference nicety is not cause to churn a finished spec.)
2. D3 — L64 `timeout-minutes`-absent claim true but ungrounded. Grounded: `grep -n "timeout" .github/workflows/ci.yml` returns no hits, checked 2026-10-06.
3. D3 — L19 lint-job range `ci.yml:70-81` off (job opens L69, pin at L83 — verified). Fixed to `ci.yml:69-83`.

Missing context: issue #115 labels/comments (read 2026-10-05) were not re-checked by the round-3 assessor (no GitHub access from its context) — re-ground at /spec if the issue changed. (Rounds 1–2 verified it via `gh`.)

## Remaining notes for /spec

- Open Q1 (default b: curated shortlist, /spec enumerates), Q2 (default a: local golangci-lint v2.13.2 matching CI's pinned binary, trial CI runs as referee), Q3 (default a: justification text in the spec record + task PR description), Q4 (default a: one config task + per-domain fallout tasks; /spec decides from the trial spread).
- NFR-1: the spec checkpoint sets the CI-time budget in minutes from /spec's trial measurements; the implementer enforces it via `timeout-minutes`.
- Re-check issue #115 state at /spec time (round-3 assessor could not reach GitHub).
