# Refinement log — windows-process-ownership

dispatched=3 returned=3 failed=0

Verdict: Good enough after 3 rounds (non-interactive, auto-confirmed). No FAIL remained in the final assessment. The round-3 fixes were applied but not re-assessed, because three rounds is the cap.

## Round 1 — Needs human input (D6 FAIL; D1–D5, D7, D8 NEEDS_WORK)

- Fixed 18 auto-fixable items: section references re-targeted to v2 (§6.4, §17.1, G04), exit paths and reasons corrected, `descendant_scan` and test criteria made observable, tags, dependencies and impacted components completed, grounding rows G10 and G11 added.
- Human-required, recorded as open questions with defaults (not guessed): OQ-1 (kill-on-close), OQ-8 (native start time scope), OQ-9 (supervised-stop-recover reconciliation).

## Round 2 — Needs work (D1, D4 PASS; others NEEDS_WORK)

- Fixed 17 items: non-inheritable handle, launch failure cases, breakaway policy test, ladder version change, claudecode refusal wording (exit 7), a separate Windows support matrix, ADR 0004 named as superseded, overlaps with in-flight specs, lifecycle-path citations as `specs/*/<name>/`.
- Human-required, recorded: OQ-11 (what counts as I24 evidence for kill-on-close), plus OQ-10 (follow-up card for deferred scope).

## Round 3 — Needs work (no FAIL)

- Fixed 14 items: Ctrl-C and Ctrl-Break now follow the shipped first-stop, second-detach contract; option (a) recovery no longer claims processes gone without evidence; AC-6.3 boundary order and outcomes; ladder version and `attempt.stopped` fields; matrix file `internal/workers/SUPPORT.md`; citation drift; stale dependency claims; Definition of Done gaps; deciders for OQ-4 and OQ-5.
- No new human-required item.

## Remaining notes for /spec

- OQ-1, OQ-8, OQ-9, OQ-11 need a maintainer decision (ADR-level); the requirements hold under every OQ-1 option and /spec must not choose silently.
- OQ-3 needs a runner spike (job membership on `windows-latest` and `windows-11-arm`, console-event driving); if hosted runners forbid breakaway, FR-6 may be `blocked` there.
- Scope (OQ-7): four domains; /spec decides single versus epic.

## Validation (`/spec`)

Validators dispatched=3 returned=3 failed=0. Round 1: Needs revision (5 blocking). Round 2: Needs revision (2 blocking). Round 3: Needs revision (3 blocking, 14 advisory). The cap of three rounds is reached, so the spec stays in `specs/refined/` and was not moved to `todo/`. The round-3 blocking findings and some advisories were fixed afterwards without a fourth validation; the rest are listed in `scratchpad.md`.
