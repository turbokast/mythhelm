---
name: finalize-spec-verify-ci
description: After the finalize pull request merges, watch main's CI for that merge commit, class each failed job real, infra or unknown (never flake without a mechanism), and run a bounded remediate-or-escalate loop through fix pull requests
argument-hint: "<spec-name> --sha <merge-commit> [--alias <word>]"
---

# Finalize Spec — Verify CI

Step 5 of `/finalize-spec`. A pull request's checks ran on its own head; `main` after the merge is a different tree, since other work landed in between. This step confirms that `main` is green with the spec's record in it, and repairs it through pull requests when it is not.

## Input

`$ARGUMENTS`: `<spec-name> --sha <the finalize merge commit> [--alias <word>]`. Run alone after a recovery, pass the commit to re-verify, usually main's tip.

## Invocation contexts

- **Slash command**: waits, classes, and runs the remediation loop. Merging a fix pull request is its own step's write; `/finalize-spec` invoking this skill is the approval, and standalone it needs the person's `--merge`, else it stops at the gate with the fix pull request open.
- **Model-invoked**: the same.
- **Non-interactive**: the same, waiting in the foreground only. Every escalation stops and returns the report.

## Steps

1. **Wait for a verdict** on the commit, in the foreground, with a Bash timeout of 600000:

   ```bash
   python3 scripts/harness/finalize.py ci --sha <full-sha> --wait --timeout 540
   ```

   Pass the full 40-character SHA (`git rev-parse <sha>` when starting from a short one): `gh run list --commit` does not match short SHAs, and a short SHA yields a false `verdict=pending` even when the runs are green.

   The script polls `gh run list --branch main --commit <sha>` itself, every 30 seconds, and exits when every push run on that commit has completed. Exit 3 (`verdict=pending`) means its time ran out: run it again, at most eight times in all; after that escalate `ci-timeout` with the runs still pending. Waiting inside the script, not by an agent's `sleep` or by yielding the turn, keeps the delay real and the budget countable.

2. **Read the verdict.**
   - 0, `verdict=green`: done.
   - 2: GitHub could not be read. Retry once, then escalate: missing data is never green.
   - 1, `verdict=red`: every failed job is one `job=` line with its class. Decide on all of them together:

   | Classes present | Action |
   |---|---|
   | only `real` | remediate (step 3) |
   | any `infra` | escalate: the job never judged the code (a setup step failed, the run was cancelled, the runner never started). Ask the maintainer to re-run it: `gh run rerun` is a publishing action behind `.claude/hooks/guard-publish.sh`, and the agent never arms it for itself |
   | any `unknown` | escalate with the job lines and the log tail (`gh run view <run-id> --log-failed`, last 60 lines): a timeout or an unreadable failure can be the code's own hang |

   A failure is never recorded as a flake. If the same job fails and later passes on the same commit, that is nondeterminism; its record names the mechanism with the failing and passing values, or says `unexplained nondeterminism` (`.claude/rules/agent-behavioral-posture.md` §7).

3. **Remediate a real failure**, at most two cycles.
   1. Find what broke: the failing step's command and log (`gh run view <run-id> --log-failed`), and whether the parent commit was green (`finalize.py ci --sha <sha>^`). A parent that was already red means the break predates this merge; fix it all the same and say so.
   2. Dispatch one fix pull request as `/finalize-spec` §Fix pull requests describes: the owning agent by the failing files, `isolation: "worktree"`, title `fix(ci): <what> (<alias> fix)`, with the job line, the log tail and the evidence of step 1 in the prompt. Record `dispatched=1 returned=M failed=K`.
   3. Merge on `python3 scripts/harness/finalize.py publish-check --spec <spec> --pr <n> --fix` printing `verdict=ready`, confirm its merge commit on origin/main, and go back to step 1 with that commit.
   4. Never weaken a test, a gate or the `CI OK` job's `needs:` to turn main green; that is a finding for the maintainer, not a fix.

   After two cycles that end red, escalate `retry-cap` with every cycle's job lines and pull requests.

4. **Record** the outcome: `python3 scripts/harness/runspec.py event --spec <spec> --kind verify --detail "verify-ci <outcome> <sha7>" --result <ok|fail>`. When a fix was needed, its pull request also adds one line to `specs/done/<spec>/retrospective.md` §CI history naming the failure, its class and the fix; the dispatch prompt says so.

## Output

```text
Verify CI: <green | green after fix PRs #a, #b | escalated: ci-timeout | infra | unknown | retry-cap>
Commit: <sha7>; runs <n>; cycles <used>/2
<the job= lines of the last red verdict, when any>
```
