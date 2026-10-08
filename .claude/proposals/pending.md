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

## P-qualification-registry-1 — MH-12 consult/write preconditions from the seed/consult gap

- **Source spec**: `qualification-registry`
- **Type**: product
- **Target**: `product/backlog.md`
- **Rationale**: Seed rows use unknown stable-identity dimensions, so no real consult ever matches them: admission has never read a seeded record, and the first live writer will create coexisting (not superseding) rows. The same review found the consult never reads evidence method/labels or quota, invalidation preserves NextTest, Record trusts caller progress labels, and the latency test pins Lookup rather than Consult. Each is latent today and load-bearing the moment a production writer exists; MH-12 is that writer and must scope them before building.
- **Evidence**: specs/done/qualification-registry/retrospective.md Review Summary findings 1, 4–9 (seed keys at internal/qualify/seed.go vs stableMatch at internal/qualify/registry.go:298-306; consult at internal/admission/qualify.go; InvalidateOnDrift at internal/qualify/registry.go:320-338; TestRegistryLookupLatency at internal/qualify/registry_test.go:650)

**Proposed change:**

Append to the MH-12 card scope: the spec must (1) decide seed/live coexistence (supersede, migrate or align seed keys) with a seam test proving a consult reads a seeded-then-recorded record; (2) gate the consult on evidence method/labels so user-declared evidence alone never proves a live column; (3) read Record.Quota at consult so a zero quantity cannot admit; (4) refresh NextTest on invalidation; (5) pin Consult (Lookup+List+drift) latency, not just Lookup; (6) pass the Consult-returned record, not the observed key, into InvalidateOnDrift.

## P-qualification-registry-2 — Pin the AC-4.2 trust-before-consult ordering with a test

- **Source spec**: `qualification-registry`
- **Type**: product
- **Target**: `product/backlog.md`
- **Rationale**: AC-4.2 holds only by construction — admitNativeConfig runs before the qualification consult in decideClaudeCode — and nothing fails if a refactor reorders them. MH-12 is about to rework exactly this path (removing the strict early return); the pin must land before that rework so it constrains the new implementation rather than describing it.
- **Evidence**: specs/done/qualification-registry/retrospective.md Review Summary finding 2 and Acceptance row AC-4.2 (ordering at internal/admission/admission.go:293 vs :312; no test names the ordering)

**Proposed change:**

Append to the MH-12 card scope as its first test item: a test that fails if trust admission (admitNativeConfig / CheckNativeTrust) does not run before the qualification consult on the claudecode decide path, and fails if a config change can reach launch without re-evaluation.

## P-qualification-registry-3 — Ground docs command-output examples in executed commands

- **Source spec**: `qualification-registry`
- **Type**: knowledge
- **Target**: `knowledge/spec-authoring.md`
- **Rationale**: A docs task shipped a command-output example with values the command never prints (wrong surface and verdicts); per-task review and one spec reviewer both eyeballed it as correct, and only the second spec reviewer checked it against the code. Examples verified by reading are verified by nobody. The checkable-criteria pattern already demands a failing state for behaviour; docs examples need the same grounding rule.
- **Evidence**: specs/done/qualification-registry/retrospective.md Review Summary finding 3 (docs/user-guide.md:59 vs internal/qualify/seed.go); fixed in the finalize PR

**Proposed change:**

Append to the checkable-criteria patterns: an acceptance item that adds or changes a docs example showing command output must name the executed command (or generating test) that produced the pasted text, and the worker re-runs it before the PR leaves draft; a paste with no producing command is a finding.
