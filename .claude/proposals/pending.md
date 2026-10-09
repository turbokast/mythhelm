# Pending proposals

Proposals awaiting a maintainer's decision. The format, the id scheme and the rules for appending are in [`README.md`](README.md). New proposals go at the end of this file.

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

Append to the MH-12 card scope: the spec must (1) decide seed/live coexistence (supersede, migrate or align seed keys) with a seam test proving a consult reads a seeded-then-recorded record; (2) gate the consult on evidence method/labels so user-declared evidence alone never proves a live column; (3) require ResolveQualification to return Blocked for subscription-only when Record.Quota.Quantity is zero, while preserving AC-3.3 for an unknown quota; (4) refresh NextTest on invalidation; (5) pin Consult (Lookup+List+drift) latency, not just Lookup; (6) pass the Consult-returned record, not the observed key, into InvalidateOnDrift.

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

## P-v2-contract-vocabulary-1 — Track deferred cross-task promises to completion

- **Source spec**: `v2-contract-vocabulary`
- **Type**: skill
- **Target**: `.claude/skills/run-spec-completion/SKILL.md`
- **Rationale**: A task completion entry deferred work to a later task ("Task 5 or 8 retypes them") with no owner; neither task did it, per-task review passed both, and only the finalize review caught the broken promise — after every task had merged. Deferred items need tracking, not prose.
- **Evidence**: specs/done/v2-contract-vocabulary/tasks.md Task 2 Spec deviations (string-State promise); Review Summary finding F2; fix PR #268

**Proposed change:**

Add to the Confirm step: collect every "Task N ..." deferral named in Spec deviations entries; each must resolve to a completing change whose promised edit is present in the referenced diff and whose task or fix PR has merged — a Files modified entry alone, or an open fix PR, is not proof — or the run stops with the unresolved promise named.

## P-v2-contract-vocabulary-2 — Review stale branches by their own diff

- **Source spec**: `v2-contract-vocabulary`
- **Type**: skill
- **Target**: `.claude/skills/run-spec-worktree-merge/SKILL.md`
- **Rationale**: Reviewing a stale branch with a tree-diff against main shows other specs' merged work as deletions, which misreads as the branch destroying them; the mistake recurred across several reviews in one run before the merge-base comparison became habit.
- **Evidence**: PR #261/#262/#263/#264 shepherd reviews (stale-base tree-diffs vs merge-base..HEAD); retrospective Lessons

**Proposed change:**

Add to the verify-before-merge procedure: when the branch base trails origin/main, review the branch's own diff (`git diff <merge-base> HEAD`) for scope and deletions; use the tree-diff against main only to confirm the merge result after updating.

## P-supervisor-service-1 — Re-verify accepted ADRs against the shipped tree at finalize

- **Source spec**: `supervisor-service`
- **Type**: skill
- **Target**: `.claude/skills/finalize-spec-review/SKILL.md`
- **Rationale**: ADR 0014 was accepted mid-spec (task 7) stating the Windows transport was blocked; task 4 then landed it, leaving the accepted record contradicting the tree until finalize review caught it. Any record accepted before the last task lands can decay the same way.
- **Evidence**: Review Summary finding at docs/decisions/0014-service-topology.md:67; retrospective.md; PR #262 (accept) then #266 (land)

**Proposed change:**

Add to the finalize review steps: for each decision record the spec accepted, diff its factual claims (blocked rows, NFR tables, cited files and tests) against the shipped tree at the review head; amend drift in the finalize worktree before publishing.

## P-supervisor-service-2 — Stand-in deviations must name their tracked follow-up

- **Source spec**: `supervisor-service`
- **Type**: skill
- **Target**: `.claude/skills/task-completion/SKILL.md`
- **Rationale**: Four tasks recorded `control.Error` stand-ins "until vocab task 6 lands"; vocab task 6 landed and no follow-up existed, so the parallel error vocabulary the spec norm forbids went live untracked. A stand-in without a tracked trigger is a silent promise.
- **Evidence**: tasks.md Spec deviations of tasks 2, 3, 5, 6; Review Summary finding F1; issue #283

**Proposed change:**

Add to the completion-entry procedure: a Spec deviation that defers to another spec's unmerged work must cite the tracking follow-up (issue or proposal id) that fires when the blocker lands; file that follow-up before the entry is written.
