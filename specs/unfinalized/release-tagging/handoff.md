# release-tagging — Hand-off

> One section per task. Each task replaces its own `<!-- pending -->` line in its pull request with what
> dependent tasks need: what it produced (the API and files as shipped), what a later task must
> know, and any deviation that changes a later task's inputs. `/run-spec` puts the sections of a
> task's dependencies into that task's dispatch prompt. Open questions and research stay in
> `scratchpad.md`.

## Task 1 — Tag-discipline records (ADR + release-process section)

- **Produces**: Discipline contract — `docs/decisions/0010-release-tag-discipline.md` (lightweight tags created only by release publication, never by hand, via the finalize-spec-tag flow; unsigned with artifact-level SLSA/cosign per the MH-7 plan) and the `## Tag discipline` section in `docs/release-process.md` (`v<semver>` with dotted markers, valid `v0.1.0`/`v1.2.3-rc.1`, invalid `0.1.0`/`v1.2`/`v1.2.3-rc1` with the convention-vs-SemVer distinction, operator-publishes with release-write, observed `Protect main` ruleset baseline, §4 trigger contract, pre-releases-tagged/drafts-never).
- **For dependents**: MH-7 implements this contract as fixed input without re-deciding; design §6 justification pre-exists on `main` naming ADR 0010, `docs/release-process.md` and `<FIRST-RELEASE-TAG>`; no tags were created and no settings were changed (read-only `gh api` observation only).
- **Deviations that change a later task's inputs**: none; single-task spec, no further tasks depend on it.
