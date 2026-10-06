# release-tagging — Retrospective

## Review Summary

- **Range**: 6a22808..1c33af6 (PRs #180; fix PRs none)
- **Reviewer**: code-reviewer
- **Findings**: critical 0, important 0, suggestion 3 (confirmed); rejected 0
- **Open critical**: 0
- **Vendor review**: skipped (codex/muse/jev all unavailable — disabled)

| Severity | Finding | Anchor | Disposition |
|---|---|---|---|
| suggestion | Two review-round elaborations (Contents:write qualification, prerelease-flag sentence) consistent with design but unrecorded under Spec deviations: None | `docs/release-process.md:17,28` at 1c33af6 | noted in Deviations below; no action |
| suggestion | Intro line 3 ("records only the version-numbering scheme") stale now the Tag discipline section exists — known cost of the MH-18 byte-untouched pin, twice rebutted in PR review | `docs/release-process.md:3` | follow-up for MH-7's pipeline extension (or a trivial docs touch); not amended here |
| suggestion | Design §4 "non-matching tags never match the trigger filter" vs §2 "bare v* glob admits malformed tags; the format check is the gate" needs reconciling by MH-7 as format-validation-as-gate | design §2 vs §4 | routed to MH-7's spec (forward trace) |

Foreign changes in range: none.

## Acceptance

| Criterion | Result | Evidence |
|---|---|---|
| AC-1.1 format string | met | Tag discipline section: v<semver>, valid/invalid examples with convention distinction; ADR: lightweight-unsigned (PR #180) |
| AC-1.2 trigger in Summary terms | met | Tag-push/tag-triggered terms only; no workflow YAML (acceptance grep empty) |
| AC-2.1 operator-only + permission | met | Operator-only, release-write + Contents:write, pasted ruleset observation, settings stay operator's |
| AC-2.2 release scope | met | Every published release incl. pre-releases tagged; drafts never; prerelease-flag sentence |
| AC-3.1 trigger contract | met | Push-starts-pipeline + tag-context-required, verbatim in section |
| AC-3.2 fail-closed rule | met | Malformed/unauthorized fail-closed rule, verbatim in section |
| AC-4.1 justification text | met | Design §6: ADR 0010, release-process.md, FIRST-RELEASE-TAG placeholder, flip condition |
| AC-4.2 badge untouched | met | Blast pin holds — exactly the two Files entries outside specs/; zero tags |
| NFR-1 prose, one PR | met | PR #180: one task, prose only, CI green |

## Deviations

- Task 1: None recorded in the entry; review confirmed the substance but found two consistent elaborations added in review round 1 that the field does not name (Contents:write permission qualification, prerelease-flag requirement sentence) — record-keeping gap only, see P-release-tagging-1.

## CI history

- PR title on PR #180 @44dc2fa: infra, run cancelled — superseded by push 8fa00f7 (review-round fix; full set success on 8fa00f7).
- PR title on PR #180 @8fa00f7: infra, run cancelled — superseded by push 3387bbc (Status-line commit; full set success on 3387bbc).
- No failure+success on the same headSha: no nondeterministic failure. No fail/retry rows in the run-events log. `finalize.py verify` emitted no `note=history:` lines.

## Effort

dispatched=1 returned=1 failed=0; attempts 1 over 1 tasks; first-pass 0/1; review rounds 2; wall-clock 2026-10-06T17:31:08Z → 2026-10-06T18:04:22Z

| Task | Agent | Attempts | Review rounds | PR | Merged | First pass |
|---|---|---|---|---|---|---|
| 1 | go-implementer | 1 | 2 | #180 | yes | no |

## Lessons

- What worked: failing-if-removed anchored greps again carried a docs task — all acceptance re-verified independently at verify time (PR #180).
- What worked: per-thread adjudication with task-pin evidence settled 6 review threads in 2 rounds (2 real fixes, 4 rebuttals), including a verified-real-but-out-of-scope code finding (semver_key string compare, logged as follow-up) that the blast-radius pin correctly kept out of the PR.
- What to change: review-round clarifications landed as unrecorded micro-divergences — the Spec deviations field said None while two consistent sentences were added post-worker (P-release-tagging-1).
- Follow-up (not a proposal): release-process.md:3 intro is stale by design of the MH-18 byte pin; MH-7's pipeline extension (or a trivial docs touch) must reword it. Also logged: semver_key prerelease string-compare (finalize.py:464-466) verified real during PR review, out of scope for the docs task.

## Proposals

- P-release-tagging-1 — completion entries name review-round clarifications as deviations

Acceptance (spec-wide, adjudicated at head 1c33af6): AC-1.1 met (v<semver> with valid/invalid examples + convention distinction; lightweight-unsigned in ADR); AC-1.2 met (trigger in MH-7 Summary terms only, no workflow YAML); AC-2.1 met (operator-only, release-write + Contents:write, pasted ruleset observation, settings stay operator's); AC-2.2 met (every published release incl. pre-releases tagged, drafts never, prerelease-flag sentence); AC-3.1 met (trigger contract verbatim); AC-3.2 met (fail-closed rule verbatim); AC-4.1 met (design §6 justification with ADR 0010, release-process.md, FIRST-RELEASE-TAG placeholder); AC-4.2 met (blast pin holds — exactly the two Files entries outside specs/; zero tags); NFR-1 met (one PR, prose only). Seams: MH-18 Scheme byte-intact under the new section, format cites the scheme without re-deciding, MH-19 forward pointer now resolves; MH-7 gets push-starts-pipeline + tag-context + v* + fail-closed as fixed input; ADR 0010 ↔ section consistent both directions. Invariants touched: None — confirmed (prose only, zero tags, read-only observation). Design vs shipped: the two suggestion-1 elaborations; otherwise none. Stale documentation: intro line 3 (suggestion 2); otherwise none.
