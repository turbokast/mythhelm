# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- mythhelm doctor qualification section: per-harness progress, columns, evidence revisions and drift triggers in plain and JSONL output (#204)
- Admission consults the qualification registry: strict subscription-only runs block with qualification reasons instead of blanket refusal (#205)

## [0.0.1] - 2026-10-06

### Added

- Headless supervised runs: mythhelm run admits a task file, supervises one Claude Code (or scripted fake) attempt under worker ownership, and records a receipt (#20)
- Candidate freeze and validation flags: the whole working tree is captured as a commit on the admitted base with symlink, binary, size, secret and config-change flags (#49)
- Project checks: mythhelm.toml checks from the admitted snapshot run under digest-bound trust grants, with pass/fail/unverified outcomes (#50)
- mythhelm review and receipt.json: inspect the candidate diff, flags and check evidence (#51)
- mythhelm apply: create a branch from a ready candidate without touching the checkout, with crash reconciliation (#52)
- mythhelm stop and mythhelm recover: request stops and resume interrupted runs without relaunching the agent (#53)
- mythhelm demo: fully offline scripted run in a disposable repository (#57)
- mythhelm doctor: read-only prerequisite report that writes nothing (#57)
- Declared billing posture subscription-declared with first-party auth evidence; strict subscription-only always blocks (#54)
- Claude Code launch with stream-json decoding, in-flight billing-route enforcement and a pinned version record (#56)
- Interactive TUI mission view: `run`, `demo` and `review` launch a focused Bubble Tea interface with responsive layouts, palette/command navigation, labelled confirmations and event-driven motion; plain, JSONL and non-TTY output unchanged (#102)
- `--accessible` linear screen-reader stream for `run`, `demo` and `review`, with `--after` resumption on `review` (`run` and `demo` start a new run, so a cursor from an earlier run does not apply) (#95)
- `--colour`, `--motion` and `--icons` presentation overrides (`NO_COLOR` and `TERM=dumb` still suppress) (#89)
- OpenSSF Best Practices passing badge: assessment linked from the README; unmet SUGGESTED criteria tracked as MH-18/19/20 (#30)
- Published documentation site: user guide, contributor guide, licence, security policy, changelog and limitations register, rebuilt on every merge to main (#139)
- Scripted demo recording embedded in the README and the docs site, reproducible from the checked-in tape via docs/demos/record.sh (#140)
- Scripted TUI recording embedded in the README and the user guide, reproducible from the checked-in tape via docs/demos/record.sh (#164)

### Security

- Native credential values are never read into kept strings, logged, persisted or forwarded (except the unwired AC-4.7 opt-in path, which stays refused) (#9)
