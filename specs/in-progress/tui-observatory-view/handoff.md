# tui-observatory-view — Hand-off

> One section per task. Each task replaces its own `<!-- pending -->` line in its pull request with what
> dependent tasks need: what it produced (the API and files as shipped), what a later task must
> know, and any deviation that changes a later task's inputs. `/run-spec` puts the sections of a
> task's dependencies into that task's dispatch prompt. Open questions and research stay in
> `scratchpad.md`.

## Task 1 — Capture the unchanged linear output contract

<!-- pending -->

## Task 2 — Record the lifecycle and presentation contracts

- **Produces**: Proposed `docs/decisions/0020-observatory-lifecycle-and-themes.md` and separate TUI-only palette table in `docs/assets/brand/README.md`; PR #484, implementation `54f859d`.
- **For Task 3**: Record explicit approval of the concrete ADR revision before runtime implementation. Fresh independent architect review found no critical or important issues, but is not maintainer approval or runtime evidence.
- **For Tasks 4–10**: Preserve first-request precedence, worker-pinned deadlines, durable final reason, stop-less Detach, joined action lifetime, single signal owner and distinct client/canonical outcomes. Mixed handoff versions fail closed; qualification and reconnect limitations remain explicit.
- **For Task 11**: Nine missing TUI-only rows are supplied. Observatory attention already exists as shared Amber (dark); shared Colours rows remain byte-identical. The equality test must compare embedded themes after their implementation, not claim current themes already match.
- **Verification**: Spec lint and hygiene pass; changed-document local links resolve; shared Colours section comparison and diff whitespace check pass. All ADR runtime test names are future obligations, not passing claims.
- **Deviations**: None.

## Task 3 — Approve the concrete ADR

<!-- pending -->

## Task 4 — Make the worker own its execution deadline

<!-- pending -->

## Task 5 — Add a stop-less owner detach control

<!-- pending -->

## Task 6 — Add journal-backed observatory data

<!-- pending -->

## Task 7 — Implement safe exit rows and typed TUI controls

<!-- pending -->

## Task 8 — Wire live and inspect lifecycle in the CLI

<!-- pending -->

## Task 9 — Preserve detached demo state and usable recovery guidance

<!-- pending -->

## Task 10 — Prove lifecycle through the packaged binary

<!-- pending -->

## Task 11 — Migrate theme tokens and contrast validation

<!-- pending -->

## Task 12 — Add semantic spans and the closed glyph vocabulary

<!-- pending -->

## Task 13 — Render the header and gated NOW/NEXT actions

<!-- pending -->

## Task 14 — Render agent, evidence and verdict content

<!-- pending -->

## Task 15 — Install all responsive ladder views

<!-- pending -->

## Task 16 — Compose overlays with semantic colour checks

<!-- pending -->

## Task 17 — Resolve themes and suppress delayed OSC replies

<!-- pending -->

## Task 18 — Expose --theme across launch branches

<!-- pending -->

## Task 19 — Update user-facing limits and instructions

<!-- pending -->

## Task 20 — Re-record the synthetic TUI demonstration

<!-- pending -->
