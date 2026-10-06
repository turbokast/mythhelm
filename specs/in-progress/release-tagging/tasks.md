## Release Tagging — Tasks

### Dependencies

- Prerequisite specs: `specs/*/openssf-badge/` (shipped; the `version_tags` Unmet record this spec prepares to close); `specs/*/version-numbering/` (MH-18: the format carries its scheme — MH-18's Task 1 must have merged so `docs/release-process.md` exists; if absent, Task 1 stops and reports instead of authoring MH-18's section). Feeds into MH-7 (release pipeline): this discipline is its fixed input.
- Order: single task; nothing to parallelize.
- **Gates for every task.** No Go code changes, so the Go gates are skipped with that reason. Docs task: `scripts/ci/check-public-hygiene.sh`. Files existing or an agent reporting success is not completion; cite the runner output.
- **Completion convention.** Append ` ✅ COMPLETED` to the heading, keep every original field, and add `Implementation` (at most 3 lines, plus the commit SHA), `Spec deviations` ("None" or a justification) and `Files modified`.
- **Commits.** Every commit is signed off (`git commit -s`, DCO). No decision record needed beyond the task's own ADR file: no billing, persistence or process-ownership change (the tag discipline is a public-contract decision, recorded as ADR 0010 in this task).

---

## Implementation Tasks

### Task 1 — Tag-discipline records (ADR + release-process section) ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Change**: Record the publish-creates-tag discipline with its rationale as ADR 0010 and as the tag-discipline section of the release-process record (design §2–§5).
- **Files**:
  - `docs/decisions/0010-release-tag-discipline.md` (the mechanics decision per design §5)
  - `docs/release-process.md` (add the tag-discipline section; MH-18's scheme section already present)
- **Produces**: Discipline contract — `v<semver>` with dotted pre-release markers; lightweight tags created only by release publication, never by hand; operator publishes; tag-push trigger + fail-closed behaviors. MH-7 implements this contract as fixed input (forward trace; its acceptance, not this spec's check).
- **Acceptance**:
  - The ADR follows the `docs/decisions/README.md` template: title `# 0010. Release tag discipline`, a `Status: accepted` line and a `Date: YYYY-MM-DD` line (deleting any of the three fails the check), plus `Context`, `Decision` and `Consequences` sections.
  - The ADR states lightweight tags created only by release publication with the never-by-hand rule, cites the finalize-spec-tag publish flow as the mechanism, and records why unsigned (artifact-level SLSA/cosign per the MH-7 plan; no key infrastructure) — each element anchored to its section; removing any fails its grep.
  - `docs/release-process.md` gains the tag-discipline section carrying the format string with the design §2 valid examples (`v0.1.0`, `v1.2.3-rc.1`) and invalid examples (`0.1.0`, `v1.2`, `v1.2.3-rc1` with the convention-vs-SemVer distinction stated), each anchored to the section; deleting any example fails its grep. MH-18's scheme section is byte-untouched (`git diff` shows added lines only outside it).
  - The tag-discipline section states the operator-publishes mechanism with the required release-write permission, the observed ruleset state recorded from a read-only `gh api repos/turbokast/mythhelm/rulesets` run (paste the output in the entry; settings unchanged by this task), the §4 trigger contract behaviors, and the pre-releases-tagged / drafts-never rule (each anchored; deleting any fails its grep). No workflow YAML appears (MH-7's design territory): `sed -n '/^## Tag discipline/,/^## /p' docs/release-process.md | grep -nE '^\s*(-\s+)?(jobs|runs-on|uses|on|push|tags):|secrets[\.:]'` prints nothing (line-anchored YAML-key shape plus secrets references; the section prose avoids these tokens).
  - The prepared `version_tags` justification text sits in design §6 naming ADR 0010, `docs/release-process.md` and the first-tagged-release demonstration condition with its fill-at-flip tag placeholder (deleting any of the three fails the grep); the maintainer's spec checkpoint reviews it before this task merges. The assessment itself stays untouched (AC-4.2): after staging both Files entries, outside `specs/`, `git diff --cached --name-only` lists exactly the two Files entries (staged diff, not worktree diff — the new ADR file is untracked until staged), and the union of `git diff HEAD --name-only` with `git ls-files --others --exclude-standard` lists exactly the same two entries (no unstaged tracked change or stray untracked file outside `specs/` passes either; blast-radius pin — the README badge snippet and every other in-tree surface stay untouched; the live assessment is external and uneditable from here).
  - `scripts/ci/check-public-hygiene.sh` passes (a hygiene violation in either file fails the script).
- **Test plan**: anchored `grep`/template checks run locally; read-only `gh api` for the observed settings record; MH-7 cites this discipline in its own spec.
- **Invariants touched**: None (prose records only: no code, no tags created — tagging stays an explicit operator action per W16, and the assessment stays Unmet until the operator flips it).
- **Implementation**: ADR 0010 records lightweight-tags-only-by-release-publication with the finalize-spec-tag mechanism and unsigned rationale; docs/release-process.md gains the Tag discipline section with format, operator-publishes, ruleset record, trigger contract and pre-release rule. All 50 local probes pass; hygiene clean. Commit c34a922.
- **Spec deviations**: None.
- **Files modified**: `docs/decisions/0010-release-tag-discipline.md`, `docs/release-process.md`, `specs/in-progress/release-tagging/tasks.md`, `specs/in-progress/release-tagging/handoff.md`.
- **Status**: ✅ Completed — ADR 0010 and the Tag discipline section recorded with operator-publishes, ruleset record, trigger contract and pre-release rule; hygiene clean; PR #180.
