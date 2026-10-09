---
name: task-completion
description: Authoritative completion format for a spec task — the gates to record first, the tasks.md completion entry (heading marker, Status with the PR, Implementation, Spec deviations, Files modified), the hand-off section, the claim-to-evidence table and the task-report block that ends the implementer's final message
argument-hint: "<spec-name> <task-number>"
---

# Task Completion

How an implementing agent finishes a task, and what every other skill checks against. A task is complete when its pull request carries recorded gate passes, a well-formed completion entry and a hand-off, and GitHub shows the pull request merged. Files existing, a green local run and an agent saying "done" are not completion.

The checks are mechanical: `scripts/harness/runspec.py entry-check` validates the entry, `scripts/harness/gatelib.py status` reports marker freshness, and `.claude/hooks/verify-task-completion.sh` refuses to let a session that wrote a completion stop until both pass.

## Input

`$ARGUMENTS`: the spec name and the task number. Other skills read this page as a reference without invoking it.

## Invocation contexts

- **Slash command**: checks the named task's entry, markers and hand-off in the current tree and reports what is missing, in the output format below.
- **Model-invoked**: the same; an implementing agent follows the steps to finish its task.
- **Non-interactive**: the same. It never asks: a missing fact (the PR number, a deviation's reason) is a blocker reported in the task-report block with `"status": "blocked"`.

## The completion entry

In the task's block in `specs/<state>/<name>/tasks.md`, keep every original field unchanged, append ` ✅ COMPLETED` to the heading, and add these fields after the last original one:

```markdown
### Task 3 — Journal and state directory ✅ COMPLETED

- (the original fields, unchanged)
- **Status**: ✅ Completed — <one sentence: what landed, in the spec's terms>; PR #<n>.
- **Implementation**: <at most three lines: the decisions a reviewer needs, not a narrative>. Commit <sha>.
- **Spec deviations**: None.
- **Files modified**: `internal/journal/journal.go`, `internal/journal/journal_test.go`, `specs/<state>/<name>/tasks.md`.
```

- **Status** starts with `✅ Completed` and names the pull request as `PR #<n>`. Write it once the pull request exists (Step 3).
- **Implementation** names the commit SHAs. It never restates the task or pastes code; the diff is the detail.
- **Spec deviations** is `None.`, or each deviation with its reason, one sub-bullet each when there are several. Every changed file outside the task's `Files` list is named here with its reason: the merge check refuses an unnamed one. A deviation that defers to another spec's unmerged work (a stand-in "until task N lands") cites the tracked follow-up that fires when that work lands, an issue `#<n>` or a proposal id; file the follow-up before you write the entry, because a stand-in without one is a silent promise.
- **Files modified** lists every path the pull request changes, backticked.
- Optional **CI evidence**: run or job links, for acceptance criteria that only CI can show.

The spec's own "Completion convention" bullet (in the `tasks.md` header) may add fields; it never removes these.

## The hand-off section

`handoff.md` in the spec directory has one `## Task N — <name>` section per task, seeded with `<!-- pending -->`. Replace your task's placeholder with what dependent tasks need, in at most ten bullets:

- **Produces**: the exported API, types, files and commands as shipped, with any difference from the design's `Produces` line.
- **For dependents**: invariants a later task must keep, seams it should use, traps you hit.
- **Deviations that change a later task's inputs**, with the task numbers they affect.

`/run-spec` pastes the sections of a task's dependencies into its dispatch prompt (`runspec.py handoff`). Edit only your own section: each task filling its own placeholder is what lets parallel pull requests merge without conflicts. A spec with no `handoff.md` yet falls back to the scratchpad's Discoveries entries; `runspec.py handoff-seed` creates the file.

`scratchpad.md` keeps open questions and research; add an entry under Discoveries only for a finding the whole spec needs, never `(none)`.

## Steps

1. **Record the gates.** Run the formatter first (`gofmt -w` on changed Go files, `go mod tidy` when imports changed), then the gates through the wrapper, one foreground command each:

   ```bash
   scripts/harness/gate.sh go         # the Go gates the change set needs
   scripts/harness/gate.sh harness    # for .claude/, scripts/, knowledge/ changes
   ```

   The wrapper runs each gate's fixed command from the tree root and writes `.claude/data/gate-marker-<gate>.json` only when it exited 0 and the tree did not change while it ran; a failure deletes the marker. A gate run any other way (`go test ./...` typed directly) proves nothing to the hook. `.claude/skills/quality-gates/SKILL.md` lists the gates; fix every failure (`.claude/skills/investigating-failures/SKILL.md`) and re-run.
2. **Prove the acceptance.** Run every test the task's Acceptance field names and read its result (`go test -race -run 'TestA|TestB' ./internal/pkg/ -v`). For every exported symbol, flag or file the task deleted or renamed, `git grep -n '<token>'` must find nothing outside the task's own changes; fix a hit in this task, never call it pre-existing. For an acceptance criterion that asserts an absence, do not write the banned token into tests, comments or test names: assert the behaviour.
3. **Open the pull request**, then write the entry and the hand-off section naming it (`.claude/skills/implement/SKILL.md` has the branch, commit and pull-request steps).
4. **Check the entry**: `python3 scripts/harness/runspec.py entry-check specs/<state>/<name>/tasks.md <N> --pr <n>` exits 0.
5. **Refresh what the entry invalidated.** The entry and hand-off are tree changes, so the `hygiene` marker is stale: run `scripts/harness/gate.sh all`, then `python3 scripts/harness/gatelib.py status` must exit 0 with every gate `fresh`.
6. **Commit and push** the entry, the hand-off and any scratchpad note to the pull request's branch, naming the paths (`git commit -s -m "<msg>" -- <paths>`).
7. **Report** with the task-report block as the last element of your final message.

## Claim → evidence

A report may claim an outcome only by citing the tool return that must exist for it to be true. Prose never proves itself, and a summary written before the command ran is a guess.

| Claim | The return to cite |
|---|---|
| "gates pass" | `gate.sh`'s own lines: `gate <name>: PASS (exit 0 …); recorded …` for each gate, and `gatelib.py status` printing `fresh` |
| "tests pass" | The runner's tail with real counts (`ok  <pkg>  1.2s`, `--- PASS: TestX`), from a command whose exit status is its own: never `go test … \| tail`, whose `$?` is `tail`'s |
| "acceptance test X exists and passes" | `--- PASS: TestX` in `-v` output; a test that did not run does not pass |
| "committed" | `git` output carrying the SHA |
| "pushed" / "PR open" | `git push` naming the branch; `gh pr create` printing the URL |
| "entry complete" | `runspec.py entry-check … ok` |
| "no leaks" | `scripts/ci/check-public-hygiene.sh` exiting 0 (the `hygiene` gate) |
| "merged" | `runspec.py verify-merged` printing `verdict=merged`; only the orchestrator claims it |

An orchestrator re-derives every claim from GitHub and the tree (`/run-spec` Step 4), so an unsupported claim costs a retry, not a merge.

## The task-report block

End the final message with exactly one fenced `task-report` block whose body is a single-line JSON object, validated by `python3 scripts/harness/runspec.py report` against `.claude/data/task-report.schema.json`:

```task-report
{"task": 3, "status": "pr_open", "pr": 21, "branch": "feat/demo-t3", "first_pass": true, "deviations": "None", "files_modified": ["internal/journal/journal.go", "internal/journal/journal_test.go", "specs/in-progress/demo/tasks.md", "specs/in-progress/demo/handoff.md"], "gates": {"go-fmt": "pass", "go-vet": "pass", "go-test": "pass", "go-mod-tidy": "pass", "golangci-lint": "pass", "hygiene": "pass"}, "budget_overrun": false, "notes": ""}
```

A blocked task reports what stopped it:

```task-report
{"task": 5, "status": "blocked", "pr": null, "branch": "feat/demo-t5", "first_pass": false, "deviations": "None", "files_modified": [], "gates": {}, "budget_overrun": false, "notes": "design D4 and requirement AC-2.3 disagree on the lock order; which governs?"}
```

- `status`: `pr_open` when the pull request is open with the entry, hand-off and recorded gates pushed; `blocked` otherwise. Never `complete`: only the orchestrator decides that, after the merge.
- `first_pass`: `false` when any gate failed during the task.
- `gates`: what `gate.sh` printed, gate by gate.
- `notes`: at most two lines, for the blocker or for what the orchestrator must know that the hand-off does not carry.

The block reports the work; it never replaces it. A `pr_open` report whose pull request lacks the entry is an incomplete task.

## Output

For the slash command: one line per check.

```text
entry      ok | <entry-check problems>
gates      fresh: go-fmt go-vet go-test … | stale: <gates>
hand-off   filled | pending | no handoff.md
```
