# Pending proposals

Proposals awaiting a maintainer's decision. The format, the id scheme and the rules for appending are in [`README.md`](README.md). New proposals go at the end of this file.


## P-release-tagging-1 — completion entries name review-round clarifications as deviations

- **Source spec**: `release-tagging`
- **Type**: skill
- **Target**: `.claude/skills/task-completion/SKILL.md`
- **Rationale**: The Spec deviations field said None while two review-round sentences (a permission qualification, a release-flag requirement) landed post-worker. Both were consistent with the design, so no defect shipped — but the finalize review had to rediscover them by diffing, and the entry claims a none-divergence record that is not quite true. Naming consistent clarifications keeps the entry an accurate map of what review added.
- **Evidence**: PR #180 review round 1 (2 fixed threads); specs/done/release-tagging/retrospective.md Review Summary finding 1 and Deviations section; task entry Spec deviations: None.

**Proposed change:**

In the Spec deviations rule, add: a review-round addition that stays inside the task's Files list and is consistent with the design is still named, one line each, marked consistent (e.g. "- Added the Contents:write permission qualification in review round 1 (consistent with design §3)"). None. is reserved for a task whose merged diff the worker's own commits fully describe.

## P-strict-lint-set-1 — gate parallel run-spec waves on FD headroom

- **Source spec**: `strict-lint-set`
- **Type**: skill
- **Target**: `.claude/skills/run-spec-dispatch/SKILL.md`
- **Rationale**: A 6-worker parallel wave exhausted the box's file descriptors (EMFILE) mid-run, killing 2 attempts outright and forcing 4 orchestrator cancels; recovery meant serializing and re-dispatching over ~2 hours (6 failed attempts total). Parallel subagents each running Go builds/tests multiply FD pressure far past one worker. A pre-dispatch headroom check would have serialized the wave before it collapsed.
- **Evidence**: run-events fail rows (tasks 2+4, environmental) and lost rows (tasks 1+3, then 1+2) 11:54–12:00Z; clean serial recovery from 13:55Z; specs/done/strict-lint-set/retrospective.md CI history and Effort sections (dispatched=14 returned=8 failed=6).

**Proposed change:**

Add a dispatch-gating step: before launching more than one worker, (1) read the box's default per-process fd limit (Linux: RLIMIT_NOFILE via prlimit/ulimit — workers inherit it; EMFILE is the per-process error, ENFILE the system-wide one) and require it above a floor the skill names, since one worker's parallel Go builds/tests approach low defaults; (2) compare system-wide headroom (file-nr vs file-max) against the total estimated peak FD demand of the proposed wave (per-worker estimate times wave size) and cap concurrency to what the headroom supports, serializing when it covers only one worker; on non-Linux platforms use a conservative worker cap instead of (1)-(2); (3) fail closed — unreadable metrics, unparseable output or a check timeout blocks parallel dispatch. A wave already dispatched that hits EMFILE sheds load to one worker (cancel where the client supports aborting workers; otherwise let running workers finish and queue the rest serially). Record the gating decision in the run-events detail.
