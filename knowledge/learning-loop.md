# Learning loop

How the harness improves itself from its own runs, and why the machinery is shaped the way it is. The procedures are in the skills (`/apply-proposals`, `/health-check`, `/research-practices`); the finalize side that writes proposals is in [`finalize.md`](finalize.md).

---

## The model

| Element | Fact | Source |
|---|---|---|
| A proposal | One concrete change to one harness target, with its evidence, written by a finalize retrospective or a practice scan | `.claude/proposals/README.md` |
| Waiting | `pending.md` on `origin/main`; a session start prints the count and the oldest age | `.claude/hooks/notify-proposals.sh`, `scripts/harness/proposals.py notify` |
| A decision | A maintainer's approve, reject or defer for one proposal, with a reason; never a blanket confirmation | `/apply-proposals` Step 2 |
| Applied | The decision's pull request is merged: the target changed (approve), the section moved to the end of `applied.md` with the decision, date, pull request, eval and rationale | `proposals.py record`, `apply-check`, `pr-check` |
| Pinned | An approved rule, skill or hook change ships with an eval case that fails without it and passes with it | `proposals.py apply-check`, `.claude/evals/run_evals.py` |
| Lane 0 | Knowledge appends to allowlisted files, applied without a per-item decision and vetoed by revert; off unless a maintainer enables it | `.claude/proposals/auto-apply.json`, `proposals.py lane0` |
| Health | A read-only report on budget, evals, proposal age, stuck specs, stale markers, vendor opt-in, required checks, main's CI, open threads and merged branches | `scripts/harness/health.py` |

## Why each piece exists

Each row is a failure mode seen when an agent harness tries to learn from its own runs, and the mechanism that removes it.

| Failure mode | Why it happens | Mechanism |
|---|---|---|
| A proposal changed the harness without anyone deciding it | Batch approval ("apply all") reads as consent for every item, including the ones nobody read | Decisions are per item; the skill never decides and a blanket instruction decides nothing (`skill-invocation-contexts.md`, write gates) |
| A fix held for a few runs, then quietly regressed | Prose-only fixes are not re-read at the point of action, and nothing notices when a later edit undoes them | Every approved rule, skill or hook change lands with an eval case, and CI runs every case through the harness lint (check `evals`) |
| An eval case "passed" the fix it was meant to pin | The case asserted something that was already true, or its glob matched nothing | `apply-check` runs the case against the base (it must fail) and the change (it must pass); every target glob must match a file, so a renamed target fails the case instead of passing it vacuously |
| Applying one proposal swept other sessions' work into its commit | Ledger files were committed whole from a shared checkout, carrying a peer's in-flight edits | Each decision is its own pull request from a worktree of `origin/main`; GitHub merges it onto the current `main` and reports conflicts instead of overwriting |
| The decision record lost entries | A read-modify-write of a shared ledger dropped or reverted appends | `applied.md` is append-only: `apply-check` refuses a change to its old text, and only `proposals.py record` writes it |
| Two proposals got the same id | Ids were allocated from the highest one a session had seen | Ids are numbered within their source spec and count both files (`finalize.py proposal-id`); the `proposals` lint refuses duplicates and ids both pending and decided |
| An automatic apply pasted the whole proposal, metadata and all, into a curated file | The applier appended the proposal block instead of the change it proposed | Lane 0 appends only the **Proposed change** body, refuses bodies that carry proposal headings or fields, and refuses anything but a pure append |
| An automatic apply enabled itself | The switch lived in a file the change could edit | `apply-check` reads `auto-apply.json` at the base, and the file is outside every decision's scope |
| A health report looked clean because checks never ran | Omitting zero-finding sections makes a skipped check identical to a passing one, and a model asked to run many checks runs some and reports success | `health.py` is a deterministic script that prints every check with a status on every run (`ok`, `warn`, `fail`, `unknown`, `skipped`); it deletes nothing, so a wrong finding costs a reading, not a file |
| The rule text grew until every turn paid for it | Each fix added a paragraph to an always-on rule | The `rule-budget` lint caps always-on bytes; examples and justification live in `knowledge/rule-evidence/`; `health.py` reports the headroom |
| The list of required checks drifted between tools | Each script kept its own copy | One tracked file, `.claude/data/required-checks.json`, read by `finalize.py` and `health.py`; the `required-checks` lint keeps it equal to what `docs/automation.md` documents |

## Eval cases

A case is one JSON file, `.claude/evals/cases/<id>.json`, in the format documented at the top of `.claude/evals/run_evals.py`: an `id`, a one-sentence `hazard`, a `source` (the proposal id, or the harness area whose guard it pins), `targets` (globs, each of which must match a file) and a `grader`:

| Grader | Passes when |
|---|---|
| `must-match` | every matched file (`scope: each`, the default) or at least one (`any`) contains the regex |
| `must-not-match` | no matched file contains the regex |
| `script` | the command, run from the repository root with no shell and the given stdin, exits with the expected status and its output matches the given regexes |

Cases pin behaviours of the harness configuration, not product code: a guard still blocks a publishing command, a template still names what it must, a budget still holds, an advisory hook still never blocks. They run offline in seconds. A deliberate change to a pinned behaviour updates its case in the same pull request, so the change is visible in review. Run them with `python3 .claude/evals/run_evals.py` (`--case <id>`, `--json`); CI runs them as the `evals` check of `scripts/ci/lint-agent-harness.sh`.

## Health checks

`scripts/harness/health.py` (and `/health-check`) reads the tree, git and GitHub and writes nothing. The GitHub checks (`required-checks`, `main-ci`, `open-prs`, `merged-branches`) are `skipped` with `--offline` and `unknown` when `gh` fails. Thresholds: a proposal or in-flight spec is reported after `--stale-days` (default 7) days; the budget warns under 1024 bytes of headroom. Local branches whose pull request merged are listed for a maintainer to remove; the report never removes them.

## Data

| File | Tracked | Written by |
|---|---|---|
| `.claude/proposals/pending.md` | yes | finalize and research pull requests |
| `.claude/proposals/applied.md` | yes | `proposals.py record`, inside each decision's pull request |
| `.claude/proposals/auto-apply.json` | yes | maintainers |
| `.claude/evals/cases/*.json` | yes | the pull request that pins a behaviour |
| `.claude/data/required-checks.json` | yes (seed) | maintainers, with `docs/automation.md` |

The loop keeps no local state: ages come from `git blame`, decisions from `applied.md`, CI and threads from GitHub.
