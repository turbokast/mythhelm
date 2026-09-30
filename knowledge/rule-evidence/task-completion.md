# Evidence: task-completion

Record for `.claude/rules/task-completion.md`.

## Hazard

A completion marker is cheap to write and expensive to trust. The failures it hides:

- **Completion without evidence.** An agent writes the ✅ after a gate it ran in a form that never ran the full suite, a piped command whose exit status was the last stage's, or no gate at all, and reports success. Reviewers then read the marker, not the run.
- **Completion without bookkeeping.** An agent does the work correctly and skips the record: a non-canonical marker (`✓ DONE`), missing fields, no pull-request number. A scheduler that keys on the canonical marker never sees the task as done, so its dependants never start: a false deadlock.
- **Completion in the wrong place.** With two copies of a spec (one left in the git index by a plain `mv`), an agent writes its marker into the stale copy. With parallel branches, an agent edits a neighbour's entry or appends to a shared section, and the merge conflicts or silently reorders history.
- **Completion by claim.** An orchestrator accepts "Task complete" from a worker's final message while the pull request is missing, red or unreviewed.

## Mechanism

- `scripts/harness/gate.sh` writes a gate marker only for a gate that exited 0 over an unchanged tree, and `.claude/hooks/verify-task-completion.sh` blocks a stop whose session marked a task complete while a needed marker is missing or stale, or the entry is malformed (`.claude/hooks/tests/test_verify_task_completion.sh`).
- `scripts/harness/runspec.py entry-check` defines "well-formed"; `runspec.py pr-check` refuses to call a pull request ready without the entry in the pull request's own `tasks.md`, naming that pull request, and without every out-of-list file named in `Spec deviations` (`scripts/harness/tests/test_runspec.py`).
- `runspec.py verify-merged` is the only source of "complete" for `/run-spec`: merged, the merge commit on origin/main, the entry there.
- Each task edits only its own block and its own seeded `handoff.md` section, and `test_runspec.py` merges two such branches to show those edits never conflict.
- `scripts/harness/spec-lifecycle.sh resolve` refuses a spec with two copies across the working tree and the index.

## Instances

None recorded in this repository yet. The first spec's own completion convention predates this rule: `runspec.py status` reports its tasks that carry only one of heading and Status as `marker_mismatch`.

## Loosening criteria

Three recorded completions that the stop hook or `pr-check` blocked although the entry was correct and every gate fresh, each linked, with the check's output.
