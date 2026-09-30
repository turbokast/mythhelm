---
paths:
  - "specs/**/tasks.md"
  # future: run-spec seeds handoff.md when a spec starts implementation
  - "specs/**/handoff.md"
---

# Task Completion Entries

- Mark a task complete only in that task's own pull request, after `scripts/harness/gate.sh` recorded every gate its change set needs. `.claude/hooks/verify-task-completion.sh` blocks the session's stop otherwise.
- Write the entry exactly as `.claude/skills/task-completion/SKILL.md` specifies: ` ✅ COMPLETED` on the heading, then `Status` (`✅ Completed — …; PR #<n>`), `Implementation`, `Spec deviations` and `Files modified`. Keep every original field unchanged. Check it with `scripts/harness/runspec.py entry-check`.
- Name every changed file outside the task's `Files` list in `Spec deviations`, with its reason. A formatter-only change is never a spec deviation.
- Edit only your own task: its block in `tasks.md` and its section in `handoff.md`. Never another task's entry, never another section.
- Never write a completion from a report, a summary or a worker's claim. A task is complete when its pull request is merged and `runspec.py verify-merged` confirms it on origin/main.

Evidence: `knowledge/rule-evidence/task-completion.md`.
