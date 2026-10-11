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

## P-budget-ledger-s1-1 — Name fixed-date fixtures that meet a real-clock commit path

- **Source spec**: `budget-ledger-s1`
- **Type**: rule
- **Target**: `.claude/rules/test-quality.md`
- **Rationale**: Two tests in one spec launched at a hard-coded timestamp while the production path under test committed with the real clock and enforced a deadline. Once the real clock passed the fixture's deadline, the tests failed everywhere. The first (Task 9's reserve fixture) was caught in a review round; the second (Task 7's first-launch test) reached main and turned its CI red until a fix pull request floated the start. The existing "Deterministic" bullet says to inject time but does not name this shape, and both authors followed the letter of it by fixing the start instead of the clock.
- **Evidence**: tasks.md Task 9 Spec deviations (2); main CI run on the Task 9 merge commit (job Go (windows-11-arm), `TestFirstLaunchStartsClockOnce`: deadline 12:10:00Z passed at 12:10:18Z); fix PR #276

**Proposed change:**

Add to the "Deterministic" bullet: a fixture that pins a wall-clock timestamp must also pin the clock the code under test reads. When the code under test reads the real clock, derive the fixture's times from `time.Now()` (or inject the clock); a hard-coded date plus a deadline or expiry is a test that fails on a schedule.

## P-budget-ledger-s1-2 — List every entry path of a per-run guarantee in its task

- **Source spec**: `budget-ledger-s1`
- **Type**: skill
- **Target**: `.claude/skills/spec-decomposition/SKILL.md`
- **Rationale**: The execution-deadline task armed the deadline on the launch path and listed I21 under Invariants touched. The recovery path builds its own pipeline, so a run reattached after the launching supervisor died had no deadline. Every per-task review and gate passed; only the whole-spec review found it, as a critical, and a fix pull request was needed before finalize. A task that enforces an invariant on "every run" names no paths, so nothing prompts the author or the reviewer to enumerate them.
- **Evidence**: Review Summary critical finding, `internal/supervisor/recover.go` before fix PR #317; tasks.md Task 7 Invariants touched

**Proposed change:**

Add to the task-breakdown method: when a task enforces an invariant or acceptance criterion that must hold for every run (a ceiling, a stop, a record), its Acceptance lists each path that reaches the run (launch, recover, resume, retry) and names one test per path, or says why a path cannot occur.

## P-budget-ledger-s1-3 — Document only behaviour a command can reach

- **Source spec**: `budget-ledger-s1`
- **Type**: skill
- **Target**: `.claude/skills/update-docs/SKILL.md`
- **Rationale**: The user guide for the ledger described four ceilings, envelope extensions, the exhaustion schedule and the `[envelopes]` table as working, written from the design. The code behind each was reachable only from tests, ignored on a documented path, or fed by a synthetic fixture. The finalize review raised four important findings of this one class, and the guide was corrected at finalize. Step 2 of the skill reads statements against the code at the head but never asks whether a command reaches that code.
- **Evidence**: Review Summary findings on `docs/user-guide.md` (ceilings, extension, exhaustion signal, `--no-checks`); issues #328, #329

**Proposed change:**

Add to step 3: for each behaviour a page describes as available, name the command or flag that reaches it today. State when a behaviour is reachable only through tests or a library call and not through the documented command or flag, and include the tracking issue. State a signal that rests on a synthetic fixture as such (I14).


## P-supervisor-migration-1 — Test a cross-task guarantee at the composed entry point

- **Source spec**: `supervisor-migration`
- **Type**: skill
- **Target**: `.claude/skills/spec-decomposition/SKILL.md`
- **Rationale**: A requirement that a lock stays held from the drain through the backup and import spanned two tasks: one produced `Drain`, the next consumed it in `Apply`. Each task tested its own function, and eight per-task reviews and gates passed while the lock was released between the two calls. Only the whole-spec review found it, as a critical, and a fix pull request was needed before finalize. No task owned a test of the composed behaviour, so nothing prompted one.
- **Evidence**: Review Summary critical finding 2 (`internal/migrate/drain.go`, `internal/migrate/apply.go` before fix PR #324); tasks.md Task 4 Spec deviations (lock held alongside the instance lock); `TestApplyHoldsLocksAcrossDrainAndBackup` fails against the pre-fix code

**Proposed change:**

Add to the task-breakdown method: when an acceptance criterion needs a state to hold across steps produced by different tasks (a lock held, a snapshot taken, a phase unchanged), the task that composes the steps lists a test through the composed entry point that asserts the state between the steps, for example by a seam that runs between them. A test of each step alone does not satisfy it.

## P-supervisor-migration-2 — Name the finalize range artifact after its spec

- **Source spec**: `supervisor-migration`
- **Type**: skill
- **Target**: `.claude/skills/finalize-spec-review/SKILL.md`
- **Rationale**: Step 1 writes the range to `<tmp>/range.json`, a fixed name. Two finalize runs sharing one session scratchpad overwrote each other's file, and both reviewers were handed another spec's base and head. They noticed because the brief also named the range; a brief without it would have reviewed the wrong spec's diff.
- **Evidence**: the code-reviewer and architect reports of this run each name a base `0ffc42d` and head `82b6fd3` for the file that this run wrote with base `3e5907c`; the file later failed to parse as JSON (`Extra data`)

**Proposed change:**

In step 1, write the range to a run-unique path such as `<tmp>/<run-id>/<spec>-range.json`, and in step 3 require the dispatch prompt to state the base and head it expects, so a reviewer that reads a file with different values stops instead of reviewing it.

## P-supervised-stop-recover-1 — Test the next owner against what a new writer leaves behind

- **Source spec**: `supervised-stop-recover`
- **Type**: skill
- **Target**: `.claude/skills/spec-decomposition/SKILL.md`
- **Rationale**: Two criticals reached the whole-spec review because each task tested a handler against its own fixtures. A control handler wrote attempt state that a live legacy owner holds, and wrote it without a journal event or spool ingest, so the next legacy owner failed ingest and blocked admissions. The per-task review saw only the writer. Naming the reader of the state the task writes, and testing it, would have shown the fault in the task.
- **Evidence**: Review Summary criticals C3 and N1 (`internal/control/runowner.go:20-33`, `internal/control/server.go:178-187`), fixed by #336 and #350; `specs/done/supervised-stop-recover/tasks.md` Tasks 1, 3 and 4 acceptance, which pin the handlers and never run a later owner against their output.

**Proposed change:**

Add to the task-breakdown method: when a task adds a writer to state that another component reads or owns (a projection column, a table, a spool, a lock-protected directory), the task's Acceptance lists a test that runs the next reader or owner against what the writer left, through the registered production entry point, and a test that the writer refuses while another owner holds the state. A task that cannot name the reader says so in its Spec deviations.

## P-supervised-stop-recover-2 — Fixture processes are reaped by identity and released before cleanup

- **Source spec**: `supervised-stop-recover`
- **Type**: rule
- **Target**: `.claude/rules/test-quality.md`
- **Rationale**: Three test failures on three platforms came from fixtures that spawn long-lived processes and leave cleanup to the end of the test. A parked worker held a file open and Windows could not delete the temporary directory. A cleanup that identified a worker by PID alone waited on a recycled PID. A probe scanned for a fresh process once and read its environment before the kernel had recorded it. Each needed its own test-only fix after the task merged.
- **Evidence**: `specs/done/supervised-stop-recover/tasks.md` Task 4 Spec deviations (`TestIngestFailureInterruptsRunExit6`, CI run 38003475427); #340 (Windows ARM, run 38038748783); #337 (Linux ARM, run 38034542400, mechanism measured in the pull request).

**Proposed change:**

Add under Required: A fixture that starts a process registers its release when it starts, not at the end of the test. The release matches the process by PID and start identity before it signals or waits on it, wakes anything that parks the process until the test ends, and fails the test when the process outlives the deadline. A probe that looks for a process it just started polls with a deadline until the process is visible instead of scanning once.
