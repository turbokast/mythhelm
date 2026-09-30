---
name: apply-proposals
description: Decide pending harness proposals one at a time with the maintainer (approve, reject or defer), apply each approved one as its own pull request touching only its target, with an eval case that pins the fix, and record every decision in applied.md; lane-0 knowledge appends apply themselves only when a maintainer enabled them
argument-hint: "[<id>=approve|reject|defer:\"<reason>\"]... [--eval-waived <id>:\"<reason>\"]... [--merge]"
---

# Apply Proposals

The apply side of the learning loop. `/finalize-spec` and `/research-practices` write proposals to `.claude/proposals/pending.md` (format: `.claude/proposals/README.md`); nothing in them changes the harness until a maintainer decides each one. This skill collects those decisions, turns each into a reviewed pull request, and records the outcome in `.claude/proposals/applied.md`. The mechanics are in `scripts/harness/proposals.py`; the reasons are in `knowledge/learning-loop.md`.

## Input

`$ARGUMENTS`, all optional:

- `<id>=approve:"<reason>"`, `<id>=reject:"<reason>"`, `<id>=defer` — a decision for one proposal, with the maintainer's reason. Repeatable, one per id.
- `--eval-waived <id>:"<reason>"` — the maintainer waives the eval case for one approved rule, skill or hook proposal.
- `--merge` — merge each decision's pull request once `proposals.py pr-check` reports `verdict=ready`. Without it, the pull requests are left open for a maintainer to merge.

## Invocation contexts

- **Slash command**: runs every step. Step 2 asks the maintainer for a decision on each proposal the arguments do not decide.
- **Model-invoked**: the same; a person can answer Step 2.
- **Non-interactive**: Step 2 takes only the per-item decisions in `$ARGUMENTS` or the dispatching prompt; a blanket instruction ("apply all", "approve everything") decides nothing. Proposals without a decision are returned as the queue, untouched. Lane-0 items (Step 3) apply in every context, because the tracked configuration is the maintainers' standing approval. Merging (Step 6) needs `--merge`.

## Steps

### 1. Queue

```bash
git fetch -q origin main
git worktree add --detach <q> origin/main          # <q>: a scratch path outside the checkout
python3 scripts/harness/proposals.py list --root <q>
```

Read the queue from `origin/main`, never from a local checkout that may be behind. For each id print `proposals.py show <id> --root <q>`. `list` marks `lane0` on proposals the lane-0 configuration makes eligible. Remove `<q>` (`git worktree remove <q>`) when the run ends.

### 2. Decide each proposal (write gate, per item)

For every proposal not marked `lane0`, show it and ask for one decision: **approve**, **reject** or **defer**, with the maintainer's reason in their words. One question per proposal. A single "approve all" or "looks fine" is not a decision on any item: ask for each one. Never decide a proposal yourself, and never turn a recommendation into a decision; you may say what you would recommend and why, labelled as a recommendation.

- **Defer** writes nothing; the proposal stays pending and is listed in the report.
- For an approved `rule`, `skill` or `hook` proposal, the change must come with an eval case (Step 4). Only the maintainer can waive it, with a reason (`--eval-waived`, or their answer here).

### 3. Lane 0

For each proposal marked `lane0`, confirm with `python3 scripts/harness/proposals.py lane0 <id> --root <q>` (`ELIGIBLE <glob>`). Then in its own worktree (Step 4's first command):

```bash
python3 scripts/harness/proposals.py append <id>       # appends the proposed change, nothing else of the proposal
python3 scripts/harness/proposals.py record <id> --decision auto-applied
```

and continue at Step 5. Lane 0 is off unless `.claude/proposals/auto-apply.json` sets `enabled: true` and lists allow globs under `knowledge/`; `proposals.py` reads that file at the base, so a pull request cannot enable itself. A maintainer vetoes a lane-0 change by reverting its pull request.

### 4. Apply one decision per pull request

For each approved proposal (and each rejection, or one pull request for all of this run's rejections):

```bash
git worktree add -b harness/proposal-<id-lowercase> <wt> origin/main
```

In `<wt>`:

1. **Change the target.** Apply the proposal's **Proposed change** to its **Target** and nothing else. A `rule` may also add its `knowledge/rule-evidence/<rule>.md`; a `hook` also updates `.claude/settings.json`, `.claude/hooks/INVENTORY.md` and its test under `.claude/hooks/tests/`. Follow the conventions of the target's area (`.claude/rules/agent-config-conventions.md` for agents, skills, rules and hooks; `.claude/rules/knowledge-conventions.md` for `knowledge/`). A change that needs a second target is a second proposal: stop and report it. A `product` proposal is a backlog change and goes through the product approval flow; record the decision here only after that flow has accepted it.
2. **Pin it with an eval case** (approved `rule`, `skill` or `hook`, unless waived). Write `.claude/evals/cases/<case-id>.json` in the format of `.claude/evals/run_evals.py`, with `"source": "<id>"` and a `hazard` naming the failure the proposal fixes. The case must fail on `origin/main` and pass with the change; `apply-check` runs it both ways.
3. **Record the decision.**

   ```bash
   python3 scripts/harness/proposals.py record <id> --decision approved|rejected \
     --rationale "<the maintainer's reason>" [--eval <case-id> | --eval-waived "<the maintainer's reason>"]
   ```

   It moves the proposal from `pending.md` to the end of `applied.md` with the decision, the date and `Pull request: pending`. Never edit `applied.md` by hand, and never edit or remove an entry already there.
4. **Check.** Run the formatter on what you changed, then `scripts/harness/gate.sh harness` and

   ```bash
   python3 scripts/harness/proposals.py apply-check <id>
   ```

   `apply-check` fails when a file outside the decision's scope changed, the entry is still pending or malformed, `applied.md` changed anything but its end, a lane-0 append is not eligible at the base or not a pure append, or the eval case does not fail at the base and pass here. Fix every reason; never widen the scope to make it pass.

### 5. Pull request

```bash
git add -- <each changed path>                    # named paths only
git status --porcelain                            # must print nothing
git commit -s -m "harness: apply <id>"            # or "harness: reject <id>", "harness: record decisions on <ids>"
git push origin harness/proposal-<id-lowercase>
gh pr create --base main --head harness/proposal-<id-lowercase> --title "<the commit subject>" --body-file <file>
python3 scripts/harness/proposals.py record <id> --pr <n>
git add -- .claude/proposals/applied.md && git commit -s -m "harness: record <id> as #<n>" && git push origin harness/proposal-<id-lowercase>
```

The body is public (`.claude/rules/public-repo-hygiene.md`): the proposal id and title, the decision and its reason, the eval case and its red and green result from `apply-check`, and for lane 0 the line "Applied under lane 0; revert this pull request to veto it."

### 6. Review and merge

```bash
gh pr checks <n> --watch --interval 30
python3 scripts/harness/proposals.py pr-check <id> --pr <n>
```

`pr-check` requires what every pull request needs (open, mergeable, checks green with `CI OK` and `CodeRabbit` present, no unresolved thread, no leak) plus the decision's scope and the entry recording this pull request. Exit 3: poll again. Exit 1: fix or rebut each reason in `<wt>`; answer each review thread and resolve it (the GraphQL calls in `/implement`); `git merge origin/main` when behind (a conflict in `pending.md` or `applied.md` keeps both sides). After three rounds still not ready, stop and report.

On `verdict=ready`: with `--merge`, or for a lane-0 pull request, `gh pr merge <n> --squash --delete-branch` and confirm the merge commit is on `origin/main` (`git merge-base --is-ancestor <oid> origin/main`). Otherwise leave it open for a maintainer and say so. Remove each worktree when its pull request is merged or handed over.

## Output

```text
Apply proposals: N pending, D decided
  P-<spec>-<n>  approved      PR #<n> ready|merged  eval <case-id> (red at base, green here) | waived: <reason>
  P-<spec>-<n>  rejected      PR #<n> merged
  P-<spec>-<n>  auto-applied  PR #<n> merged (lane 0, veto by revert)
  P-<spec>-<n>  deferred
  P-<spec>-<n>  undecided     (non-interactive: no per-item decision)
```

Every number and verdict comes from `proposals.py` or `gh` output; a result you did not observe is `unknown`.
