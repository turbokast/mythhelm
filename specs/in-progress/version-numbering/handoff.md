# version-numbering — Hand-off

> One section per task. Each task replaces its own `<!-- pending -->` line in its pull request with what
> dependent tasks need: what it produced (the API and files as shipped), what a later task must
> know, and any deviation that changes a later task's inputs. `/run-spec` puts the sections of a
> task's dependencies into that task's dispatch prompt. Open questions and research stay in
> `scratchpad.md`.

## Task 1 — Version-numbering records (ADR + release-process scheme)

- **Produces**: Scheme contract — `docs/decisions/0009-version-numbering-scheme.md` (SemVer 2.0.0 ratified, AC-1.1 rationale, releases-only scope, CHANGELOG.md:6 cited standing) and `docs/release-process.md` (scheme section with `v0.1.0`/`v1.2.3` examples, MH-19 owns tag format/mechanics, MH-7 implements the tag-triggered GoReleaser pipeline, pipeline sections land with MH-7).
- **For dependents**: MH-19 and MH-7 cite these records instead of re-deciding; no CHANGELOG edit was made (ratify path — `git status --porcelain CHANGELOG.md` is clean); `release-process.md` deliberately names no pipeline detail beyond the card-fixed "tag-triggered" and "GoReleaser" terms.
- **Deviations that change a later task's inputs**: none; single-task spec, no further tasks depend on it.
