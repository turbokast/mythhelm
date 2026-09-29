---
name: finalize-spec-verify
description: Decide from git and GitHub whether a spec can be finalized — every task complete with a merged pull request on origin/main, main's CI green at its tip, the required checks unchanged, no unresolved review thread and no open fix pull request
argument-hint: "<spec-name> [--alias <word>]"
---

# Finalize Spec — Verify

Step 1 of `/finalize-spec`, and runnable on its own to ask "can this spec be finalized now?". It writes nothing.

## Input

`$ARGUMENTS`: `<spec-name> [--alias <word>]`. The alias is the one the spec's pull-request titles carry (default: the spec name).

## Invocation contexts

- **Slash command**: runs the check and prints the verdict with its reasons.
- **Model-invoked**: the same; `/finalize-spec` continues only on `verdict=ready`.
- **Non-interactive**: the same. It has no write and no question, so it behaves identically in every context.

## Steps

1. Run the check from any checkout of the repository:

   ```bash
   python3 scripts/harness/finalize.py verify --spec <spec> --alias <alias>
   ```

   It fetches origin and reads `tasks.md` **on origin/main**, never a local copy, then requires:

   | Fact | Reason line when it fails |
   |---|---|
   | The spec is in `specs/unfinalized/` on origin/main | `state:` |
   | Every task's entry is complete and well-formed and names its pull request (the predicates of `runspec.py verify-merged`) | `task N: ...` |
   | Each of those pull requests is merged, with its merge commit on origin/main | `task N: PR #n is ...` |
   | No unresolved review thread on any of them | `thread-unresolved:` |
   | No open pull request titled `(<alias> task N)` or `(<alias> fix)` | `open-pr:` |
   | main's push runs at its tip, which contains every merge of the spec, completed and green | `main-tip:` |
   | The `CI OK` job still needs every job it needed before the spec's first merge | `gate-weakened:` |
   | main's ruleset still requires `CI OK`, `DCO sign-off`, `Dependency review`, `Analyze (actions)` and `Analyze (go)` | `required-checks:` |

   `note=history:` lines name failed or unfinished runs on the spec's merge commits below the tip. They do not block, because the tip's green run covers the same code and more, but they are the retrospective's CI history. `note=required-checks:` names a required check the expected list lacks; update the list in `scripts/harness/finalize.py` in its own pull request.

2. Read the exit status:
   - 0, `verdict=ready`: finalize may proceed.
   - 3, `verdict=unknown`: a run is still queued or in progress. Wait for it with `python3 scripts/harness/finalize.py ci --sha <the sha named> --wait` (Bash timeout 600000), then run step 1 again. Never proceed on 3.
   - 1, `verdict=not-ready`: report the `reason=` lines. A red `main-tip` goes to `/finalize-spec-verify-ci` for classification before anything else; the other reasons are escalations for the maintainer.
   - 2: git or GitHub could not be read. That is missing data, not a verdict: retry once, then escalate.

## Output

The script's output verbatim, then one line: `Verify: <verdict> (<N> reasons)`.
