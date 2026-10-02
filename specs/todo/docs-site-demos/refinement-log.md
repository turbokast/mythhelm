# Refinement log — docs-site-demos

Loop: assess → fix → re-assess; each round a fresh-context assessor
dispatched with only the spec path and the standard prompt.

dispatched=2 returned=2 failed=0

## Round 1 — Verdict: Needs work

Scores: D1 PASS; D2 NEEDS_WORK; D3 FAIL; D4 NEEDS_WORK; D5 PASS;
D6 PASS; D7 NEEDS_WORK; D8 NEEDS_WORK. No human-required issues.

Auto-fixable findings and fixes applied to `requirements.md`:

1. Stale `tui-slice` lifecycle path (L17, L28, L100). The assessor reported
   `specs/unrefined/tui-slice/` does not exist; the driver verified the spec
   moved to `specs/refined/tui-slice/` (requirements.md + refinement-log.md
   present). Fixed all three citations to `specs/refined/tui-slice/`, and
   "unrefined" to "refined" (L17, L28).
2. Workflow count (L30): "12 workflows" refuted — 13 files observed in
   `.github/workflows/`, none Pages-related. Fixed 12 → 13.
3. AC-1.1 (L55) named no extracted sections/pages. Now enumerates the user
   guide (installation, quickstart/demo, limitations) and contributor guide
   (governance, contribution path, DCO sign-off) with sources (§14.9, §19.3,
   §22.3; README/CONTRIBUTING/SECURITY/CHANGELOG/GOVERNANCE.md) already
   named in Context. No scope change.
4. AC-2.1 (L62) demanded "byte-reproducibly" from VHS output, which embeds
   encoder timestamps. Replaced with defined bar: "transcript-reproducibly —
   the same commands, outputs and exit codes". No scope change.
5. AC-3.1 (L68) triggered on an external event ("when the TUI slice has
   landed"). Reworded as event-driven EARS on running the record command
   after the TUI slice lands; same transcript-reproducibility bar as AC-2.1.
6. AC-4.1 (L74) gave CI no checkable signal for "the exact binary revision
   the tape declares". Now requires each tape to declare the exact binary
   revision (version and commit); CI verifies the recording was produced
   from it.
7. AC-4.2 (L75) gave CI no detectable signal for unqualified features. Now
   requires each recording to declare featured capabilities and their
   qualifying versioned test results; CI fails a recording missing a
   qualifier for a shown, unmarked capability.

D4 note: the assessor's epic/phasing concern rested on the premise that
`tui-slice` does not exist yet; with the corrected path to the refined spec,
FR-3 stays sequenced but plannable, and the DoD L86 deferral leg remains a
fallback, not the stated position.

## Round 2 — Verdict: Ready

Scores: D1–D8 all PASS. Four non-blocking nits recorded as remaining notes
for /spec (below). No further edits made after the Ready verdict.

## Remaining notes for /spec

1. L29 cites `internal/cli/demo.go:49` for `runDemo`, which starts at line 48
   (line 49 is inside the function, so the hint still lands). Confirm or
   adjust when writing design.md current state.
2. NFR-1 (L79) is tagged `[G10]`; its substance is the §19.4 fork-safe lane
   (already cited in the text). `[§19.4]` would be the precise tag.
3. Cross-spec citations use lifecycle paths (`specs/refined/tui-slice/`,
   `specs/done/dogfood-slice/`, `specs/unrefined/openssf-badge/`). The
   self-citation rule (`.claude/rules/spec-authoring.md`: cite as
   `specs/*/<name>/`) suggests lifecycle paths go stale on move — as
   happened here. Prefer the `specs/*/<name>/` form in design.md/tasks.md.
4. DoD L86's deferral leg mentions "the tape scaffold" without defining it;
   design.md should state what merges (skeleton tape vs placeholder page).
5. Sequencing caution: L28's "tui-slice is refined" holds by directory
   location, but `specs/refined/tui-slice/requirements.md:3` still carries a
   stale "Unrefined. Run /refine-spec tui-slice before /spec." banner.
   Confirm that spec's true state before scheduling FR-3 work.
6. Out of scope, flagged for the orchestrator: `specs/unrefined/openssf-badge/requirements.md:80`
   cites the same stale `specs/unrefined/tui-slice/` path.
