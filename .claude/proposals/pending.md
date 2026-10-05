# Pending proposals

Proposals awaiting a maintainer's decision. The format, the id scheme and the rules for appending are in [`README.md`](README.md). New proposals go at the end of this file.

## P-dogfood-slice-1 — JSONL acceptance tests assert the type discriminator

- **Source spec**: `dogfood-slice`
- **Type**: skill
- **Target**: `.claude/skills/test-driven-development/SKILL.md`
- **Rationale**: Task 13's `TestReviewJSONLSingleObject` passed while `review --format jsonl` omitted the required `"type":"receipt"` field, because the test asserted object shape but not the discriminator. The spec-wide review found the same class twice more (demo #70, doctor #71). A gate passed a defect the review found; the red-first discipline needs a discriminator rule so envelope tests pin the contract that routers and consumers match on.
- **Evidence**: retrospective Acceptance AC-1.3/AC-8.2 (partial); review findings at `internal/cli/review.go:62`, `internal/cli/demo.go:49`, `internal/cli/doctor.go:51`; follow-up issues #69, #70, #71.

**Proposed change:**

Add to the skill's test-writing rules: every acceptance test covering a JSONL or envelope output must assert the `type` discriminator value (or the schema's equivalent routing field) of each emitted object, not just that output parses or has the right shape. A test named `*JSONL*` / `*Envelope*` without a discriminator assertion is incomplete.

## P-dogfood-slice-2 — Spec validation executes embedded test selectors

- **Source spec**: `dogfood-slice`
- **Type**: skill
- **Target**: `.claude/skills/spec-validate/SKILL.md`
- **Rationale**: The Task 20 text told the maintainer to run `go test -run TestLiveCanary`, but the test is named `TestLiveClaudeCanary`, so the documented command silently ran nothing. A wrong command in task text wastes a human session and erodes trust in the spec. Validation should execute (or dry-list) every `go test -run` selector embedded in task text and fail when a selector matches zero tests.
- **Evidence**: issue #60 (fixed by the dogfood candidate itself); Task 20 Spec deviations in `specs/done/dogfood-slice/tasks.md`.

**Proposed change:**

Add a validation step: collect every `go test … -run <selector>` command embedded in the spec's task text; for each, dry-match it against the tree that results from the task's merge and fail validation when it matches zero tests, naming the task and the selector. Never validate against the base tree: a task may introduce the selected test, so base-tree matching would reject valid tasks before implementation. Follow `-run` semantics for slash-separated subtest selectors (e.g. `TestDecodeFixture/recorded`): `-list` matches top-level tests only, so a top-level match alone must not count as a match for the full selector. Split the selector: match the top-level element with `-list`, and verify each subtest suffix statically against the `t.Run` names declared in the package's test files (go/parser, no execution). Never dry-match by running the selector (`-count=1` executes test bodies, including side effects). Fail closed: unparseable command extraction, a match error, a match timeout, a suffix matching no declared name, or a dynamically-named parent whose suffix cannot be checked statically blocks validation exactly like a zero-match selector.

## P-dogfood-slice-3 — Windows atomic-replace retry pattern in knowledge

- **Source spec**: `dogfood-slice`
- **Type**: knowledge
- **Target**: `knowledge/execution.md`
- **Rationale**: Two main-CI failures (one CI failure class seen twice) shared one mechanism: on Windows, an atomic file replacement (`rename` over the destination) intermittently fails with `Access is denied` when a parallel reader/observer holds the file, failing the whole `Go (windows-latest)` job. The fix (retry transient sharing violations, #55) is product-side, but every future task touching atomic file writes on Windows needs the pattern up front; otherwise each task rediscovers it through a red main.
- **Evidence**: retrospective CI history (runs 36842187196, 36844977281; `writing worker.json: rename …: Access is denied`); fix PR #55; Task 13 identity-retry files.

**Proposed change:**

Append a short entry: on Windows, atomic file replacement via rename can fail transiently with a sharing violation when another handle is open; product code that replaces identity/state files must retry transient sharing violations with backoff (fail closed on persistent errors), and tests that write-then-immediately-replace files should expect the retry path. Cite the worker.json incident as the example.

## P-dogfood-slice-4 — run-spec dispatch emits complete run-events rows

- **Source spec**: `dogfood-slice`
- **Type**: skill
- **Target**: `.claude/skills/run-spec-dispatch/SKILL.md`
- **Rationale**: `.claude/data/run-events.jsonl` starts mid-spec (first row 2026-09-30T23:20:49Z, tasks 14–20 only; no `run_start` row), so the retrospective's wall-clock start is unknown and tasks 1–13 have no dispatch/return/attempt attribution. Dispatches happened without logging them. The dispatch procedure should require emitting the run-events rows it owns, so effort data exists for every task.
- **Evidence**: retrospective Effort section; `.claude/data/run-events.jsonl` row range vs the 20 merged task PRs (#7–#67).

**Proposed change:**

Add a dispatch step: before starting a task attempt, append the `run_start` row (once per spec run) and the task's `dispatch` row to `.claude/data/run-events.jsonl`; after the attempt ends, append the `return` row (and `merge` after the merge). A dispatch without its rows is incomplete. Before finalizing, the orchestrator may backfill rows for already-merged tasks only from authoritative records (merged PRs, merge commits). Backfilled rows must carry provenance marking them as reconstructed, and consumers must distinguish them from observed rows when computing attempt metrics and attribution. Use the schema's `null` and `unknown` values for unavailable attribution or results; never treat the generated `ts` as the original event time when that time cannot be recovered.

## P-docs-site-demos-1 — spec-decomposition: user-visible design claims need a task owner

- **Source spec**: `docs-site-demos`
- **Type**: skill
- **Target**: `.claude/skills/spec-decomposition/SKILL.md`
- **Rationale**: design §5 required the docs site to show the word "planned" where the TUI recording will go, but no task's Files list owned a site page for that claim — Task 7 owned only the tape scaffold and the (unrendered) demos README row. No task built it, and no task recorded skipping it, so the gap surfaced only at finalize review. A decomposition check that every user-visible design claim has a task owner prevents silent drops.
- **Evidence**: docs-site-demos retrospective §Review Summary (suggestion 2) and §Deviations (Review row); `specs/done/docs-site-demos/design.md` §5 vs Task 7 Files in `tasks.md`; PR #137 merged without it.

**Proposed change:**

Add to the decomposition steps, after task Files lists are drafted: "Coverage pass: for every sentence in design.md that states what a user sees (page text, labels, placeholders, error wording), name the task whose Files list contains the file carrying that text. A user-visible claim with no owning task is a gap: assign it to a task's Files or record it as an explicit follow-up with its card or issue. The finalize review re-checks this mapping."
