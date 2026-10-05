# Applied proposals

Decided proposals, oldest first. `scripts/harness/proposals.py record` moves a proposal
here from `pending.md` with its decision, in the pull request that carries it. Append
only: never edit or remove an entry. The format is in [`README.md`](README.md).

## P-tui-slice-1 — Tasks name the owner of every stub they leave

- **Decision**: approved
- **Date**: 2026-10-05
- **Pull request**: pending
- **Eval**: `implement-stub-names-owner`
- **Rationale**: Maintainer approved 2026-10-05, endorsing the recommendation: closes the proven inert-stub seam cheaply.
- **Source spec**: `tui-slice`
- **Type**: skill
- **Target**: `.claude/skills/implement/SKILL.md`
- **Rationale**: Task 7 left the exit-options dialog inert with the comment "informational until a later task wires its rows", naming no task; no later task's acceptance covered wiring it, so the design §2 quit contract shipped broken and only the finalize review plus a human G09 session caught it. A stub whose owner is "some later task" is a seam no per-task review sees.
- **Evidence**: `internal/tui/dialogs.go:96-99`; retrospective Deviations (review-found) and Acceptance AC-3.1 (partial); follow-up issue #106.

**Proposed change:**

Add to the implementation rules: a task may land an inert stub (rendered but unwired UI, uncalled helper reserved for later) only if its completion entry names the specific later task whose acceptance covers wiring it, quoting that task's acceptance line; the orchestrator verifies the named task exists and is not yet merged. Otherwise the task wires the stub, removes it, or escalates. A stub comment naming no task number fails review.
