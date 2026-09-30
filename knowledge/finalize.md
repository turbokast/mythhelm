# Finalize

How a spec is closed after its last task merges, and why the machinery is shaped the way it is. The procedures are in the skills (`/finalize-spec` and its sub-skills, `/update-docs`); this page holds the facts and reasons they rest on. The execution model it builds on is in [`execution.md`](execution.md).

---

## The model

| Element | Fact | Source |
|---|---|---|
| Starting point | The spec is in `specs/unfinalized/` on origin/main: every task merged as its own reviewed pull request, moved there by `/run-spec` | `.claude/skills/run-spec-completion/SKILL.md` |
| Ready to finalize | Every task's entry complete and naming a merged pull request whose merge commit is on origin/main; no unresolved thread on those pull requests; no open task or fix pull request; main green at its tip; required checks and `CI OK`'s needs not reduced | `scripts/harness/finalize.py verify` |
| Review range | From the parent of the spec's first merge commit to its last, restricted to the files its pull requests changed; foreign commits touching those files are listed for attribution | `finalize.py range` |
| The record | `specs/done/<spec>/retrospective.md` (Review Summary, Acceptance, Deviations, CI history, Effort, Lessons, Proposals), `CHANGELOG.md` entries, doc updates, proposals, the epic's move when it was the last work stream | `/finalize-spec-publish` |
| Landing | One pull request, `docs(spec): finalize <spec>`, from a worktree of origin/main, merged on `finalize.py publish-check` → `verdict=ready` | `finalize.py publish-check` |
| Done | The finalize merge commit is on origin/main and main's CI is green for it | `finalize.py ci` |
| Backlog | Cards move to `shipped` through `pm-sync-core` approval requests, after CI is green | `/finalize-spec-pm` |
| Release | Prepared on request; published only when a maintainer asks, inside an armed publish window | `/finalize-spec-tag`, `.claude/hooks/guard-publish.sh` |

## Why each piece exists

Each row is a failure mode seen when agents close out work, and the mechanism that removes it.

| Failure mode | Why it happens | Mechanism |
|---|---|---|
| A finalize commit swept another session's staged work into the spec's record | Finalize ran in a checkout whose index other sessions shared | Finalize works in its own worktree from origin/main and lands a pull request; `publish-check` refuses files outside the spec, its epic, the changelog, docs, knowledge and proposals |
| The retrospective was missing from the shipped record | A path-limited commit names tracked paths; a new, untracked file under them is silently left out | Stage the new directory by name, then `git status --porcelain` must be empty; `publish-check` requires `retrospective.md` in the pull request; the `finalize` lint requires it in `done/` |
| A shared file committed from an older view reverted a peer's change | The committed content encoded the state before the peer landed | Every write rides a pull request; GitHub merges it onto the current `main` and reports conflicts instead of overwriting |
| The review read a diff that did not contain the spec | The base was computed from the local branch after the spec's commits were already on `main`, so the "changed files" were someone else's | The range comes from the spec's merged pull requests, never from the local branch (`finalize.py range`) |
| A red spec was finalized because an older run was green, or a green spec was blocked by a stale red run | Verdicts taken from the wrong commit | The verdict is main's tip, which contains every merge of the spec; runs on earlier merge commits are reported as history (`finalize.py verify`) |
| CI was "flaky", so nobody looked | A retry went green, and that was read as proof of nondeterminism rather than as a symptom to explain | Failed jobs are classed `real`, `infra` or `unknown`, never flake (`finalize.py ci`); a retrospective line that says flake must give the mechanism and both values (`finalize` lint) |
| A poll loop burned its budget in seconds, or a background wait never woke the agent | Sleeps inside subagents were ineffective, and subagents are not notified when background commands end | `finalize.py ci --wait` sleeps and polls inside the script, bounded by `--timeout`, in the foreground |
| A re-run turned a red `main` green without a fix | Re-running is cheaper than diagnosing | Re-runs are publishing actions behind `guard-publish.sh`; an `infra` failure is escalated to the maintainer, who decides to re-run |
| A gate was weakened to get green | Dropping a job from the aggregate check is a one-line change | `verify` compares `CI OK`'s `needs:` before and after the spec, and the ruleset's required checks with the expected list |
| Counts in a retrospective did not match the run | They were written from memory | Every number comes from `runspec.py summary`, `finalize.py` or `gh` output; a number no command produced is `unknown` |
| Two finalizes picked the same proposal id | Ids were allocated globally from the highest one seen | Ids are `P-<spec>-<n>`, numbered within the spec (`finalize.py proposal-id`); the `proposals` lint refuses duplicates |
| A multi-spec backlog card was marked shipped while a sibling was unfinished | The status flip looked only at the spec being finalized | `pm-sync-core` holds a card at `implementing` with a half-shipped note until every spec it names is done |
| A finished epic stayed "in progress" | Nothing moved the plan when its last work stream shipped | `finalize.py epic` decides the rollup from the Work Streams table, and the move rides the finalize pull request |

## Data

The finalize pipeline adds no local data files. It reads `.claude/data/run-events.jsonl` (written by `/run-spec`, in the main checkout, one row per event, append-only), appends `lifecycle` and `verify` rows to it, and writes nothing else under `.claude/data/`. Because there is one append-only file and no per-run shards, nothing needs consolidating, and because the retrospective reads rows by spec, nothing needs rotating to stay fast at the project's scale. Rotation would reintroduce the lost-update race of a read-modify-write on a file other sessions append to. The durable record is the retrospective in `specs/done/`, not the log.
