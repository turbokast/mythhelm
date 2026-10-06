# Applied proposals

Decided proposals, oldest first. `scripts/harness/proposals.py record` moves a proposal
here from `pending.md` with its decision, in the pull request that carries it. Append
only: never edit or remove an entry. The format is in [`README.md`](README.md).

## P-tui-slice-3 — Implementation skill carries the same DCO exception

- **Decision**: approved
- **Date**: 2026-10-05
- **Pull request**: #152
- **Eval**: `implement-dco-retry-exception`
- **Rationale**: Maintainer approved 2026-10-05, endorsing the recommendation: keeps implement consistent with the P-tui-slice-2 DCO exception.
- **Source spec**: `tui-slice`
- **Type**: skill
- **Target**: `.claude/skills/implement/SKILL.md`
- **Rationale**: P-tui-slice-2 exempts open-PR DCO retries from the dispatch template's no-force-push rule, but retry workers also follow the implementation skill, whose Step 7 forbids force pushes and whose Step 9 forbids rebases outright. Without the same exception there, a worker ordered to `rebase --signoff` plus force-with-lease faces two skills in direct conflict. (Applying both proposals must also reconcile `.claude/hooks/block-destructive.sh`, which blocks force pushes; that hook change rides with whichever proposal a maintainer accepts first.)
- **Evidence**: P-tui-slice-2; `.claude/skills/implement/SKILL.md` Steps 7 and 9; PR #102 DCO cycle.

**Proposed change:**

In Step 7's rules, append to the no-force-push sentence: "Exception: an open-PR DCO retry signs off via `git rebase --signoff` and pushes with `git push --force-with-lease` (see P-tui-slice-2); this is the only rebase or force-push a worker ever performs." In Step 9's behind/conflict rule, append: "The DCO retry is the exception: it rebases with `--signoff` instead of merging."

## P-tui-slice-2 — Dispatch template requires signed-off worker commits

- **Decision**: approved
- **Date**: 2026-10-05
- **Pull request**: #151
- **Eval**: `dispatch-commits-signed-off`
- **Rationale**: Maintainer approved 2026-10-05, endorsing the recommendation: stops the repeated unsigned-commit red-gate cycle.
- **Source spec**: `tui-slice`
- **Type**: skill
- **Target**: `.claude/skills/run-spec-dispatch/SKILL.md`
- **Rationale**: Task 12's worker committed unsigned, breaking the required DCO gate on three consecutive heads; the fix needed a signoff rebase and force-push cycle. The dispatch template's commit rule ("Commit named paths only") never mentions sign-off, so every new worker rediscovers it through a red gate.
- **Evidence**: PR #102 DCO failures on 2b78e3a/31b27e7/31b4548; fix commit 40ee90c; retrospective CI history.

**Proposed change:**

In the first-attempt template's Rules, change "Commit named paths only." to "Commit named paths only, signed off (`git commit -s`)." Add to the retry template's digest rules: a DCO failure is fixed by `git rebase --signoff` plus push, never by an empty sign-off commit. When the retry continues an open PR (branch checked out from `origin/<branch>`), the rebase rewrites published commits, so the push must be `git push --force-with-lease` — the narrow, explicitly named exception to the template's no-force-push rule for this case only. A retry without an open PR starts from `origin/main` and must not force-push.

## P-tui-slice-1 — Tasks name the owner of every stub they leave

- **Decision**: approved
- **Date**: 2026-10-05
- **Pull request**: #150
- **Eval**: `implement-stub-names-owner`
- **Rationale**: Maintainer approved 2026-10-05, endorsing the recommendation: closes the proven inert-stub seam cheaply.
- **Source spec**: `tui-slice`
- **Type**: skill
- **Target**: `.claude/skills/implement/SKILL.md`
- **Rationale**: Task 7 left the exit-options dialog inert with the comment "informational until a later task wires its rows", naming no task; no later task's acceptance covered wiring it, so the design §2 quit contract shipped broken and only the finalize review plus a human G09 session caught it. A stub whose owner is "some later task" is a seam no per-task review sees.
- **Evidence**: `internal/tui/dialogs.go:96-99`; retrospective Deviations (review-found) and Acceptance AC-3.1 (partial); follow-up issue #106.

**Proposed change:**

Add to the implementation rules: a task may land an inert stub (rendered but unwired UI, uncalled helper reserved for later) only if its completion entry names the specific later task whose acceptance covers wiring it, quoting that task's acceptance line; the orchestrator verifies the named task exists and is not yet merged. Otherwise the task wires the stub, removes it, or escalates. A stub comment naming no task number fails review.

## P-openssf-badge-1 — verify-ci passes the full commit SHA (short SHAs miss)

- **Decision**: approved
- **Date**: 2026-10-05
- **Pull request**: #153
- **Eval**: `verify-ci-full-sha`
- **Rationale**: Maintainer approved 2026-10-05, endorsing the recommendation: removes the false-pending trap at the call site.
- **Source spec**: `openssf-badge`
- **Type**: skill
- **Target**: `.claude/skills/finalize-spec-verify-ci/SKILL.md`
- **Rationale**: `gh run list --commit <short-sha>` silently matches zero runs even when completed runs exist for that commit, and `finalize.py ci` reports the miss as `verdict=pending` ("no push run on main yet") — a false negative that sends the operator down a re-wait loop. Pinning the step to full 40-char SHAs removes the trap at the call site regardless of when the script learns to normalize.
- **Evidence**: openssf-badge retrospective §CI history: `finalize.py ci --sha d77f2e8 --wait` → pending with 6 completed runs present; `gh run list --branch main --commit d77f2e8` → 0 runs; full-SHA invocation → `verdict=green`, runs=6.

**Proposed change:**

In Step 1, change the command template to use a full SHA and add the warning: "Pass the full 40-character SHA (`git rev-parse <sha>` when starting from a short one): `gh run list --commit` does not match short SHAs, and a short SHA yields a false `verdict=pending` even when the runs are green."

## P-docs-site-demos-1 — spec-decomposition: user-visible design claims need a task owner

- **Decision**: approved
- **Date**: 2026-10-05
- **Pull request**: #154
- **Eval**: `decomposition-owns-visible-claims`
- **Rationale**: Maintainer approved 2026-10-05, endorsing the recommendation: prevents user-visible claims dropping silently.
- **Source spec**: `docs-site-demos`
- **Type**: skill
- **Target**: `.claude/skills/spec-decomposition/SKILL.md`
- **Rationale**: design §5 required the docs site to show the word "planned" where the TUI recording will go, but no task's Files list owned a site page for that claim — Task 7 owned only the tape scaffold and the (unrendered) demos README row. No task built it, and no task recorded skipping it, so the gap surfaced only at finalize review. A decomposition check that every user-visible design claim has a task owner prevents silent drops.
- **Evidence**: docs-site-demos retrospective §Review Summary (suggestion 2) and §Deviations (Review row); `specs/done/docs-site-demos/design.md` §5 vs Task 7 Files in `tasks.md`; PR #137 merged without it.

**Proposed change:**

Add to the decomposition steps, after task Files lists are drafted: "Coverage pass: for every sentence in design.md that states what a user sees (page text, labels, placeholders, error wording), name the task whose Files list contains the file carrying that text. A user-visible claim with no owning task is a gap: assign it to a task's Files or record it as an explicit follow-up with its card or issue. The finalize review re-checks this mapping."

## P-dogfood-slice-1 — JSONL acceptance tests assert the type discriminator

- **Decision**: approved
- **Date**: 2026-10-06
- **Pull request**: pending
- **Eval**: `jsonl-tests-assert-discriminator`
- **Rationale**: maintainer approved via deliver-backlog checkpoint 2026-10-06
- **Source spec**: `dogfood-slice`
- **Type**: skill
- **Target**: `.claude/skills/test-driven-development/SKILL.md`
- **Rationale**: Task 13's `TestReviewJSONLSingleObject` passed while `review --format jsonl` omitted the required `"type":"receipt"` field, because the test asserted object shape but not the discriminator. The spec-wide review found the same class twice more (demo #70, doctor #71). A gate passed a defect the review found; the red-first discipline needs a discriminator rule so envelope tests pin the contract that routers and consumers match on.
- **Evidence**: retrospective Acceptance AC-1.3/AC-8.2 (partial); review findings at `internal/cli/review.go:62`, `internal/cli/demo.go:49`, `internal/cli/doctor.go:51`; follow-up issues #69, #70, #71.

**Proposed change:**

Add to the skill's test-writing rules: every acceptance test covering a JSONL or envelope output must assert the `type` discriminator value (or the schema's equivalent routing field) of each emitted object, not just that output parses or has the right shape. A test named `*JSONL*` / `*Envelope*` without a discriminator assertion is incomplete.
