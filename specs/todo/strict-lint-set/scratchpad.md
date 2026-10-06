# Strict Lint Set — Scratchpad

> Open questions and research from authoring. Each implementing task reads this file when it starts and adds an entry under Discoveries when it finishes.

## Open questions

| # | Question | Default in this spec | Owner / blocks |
|---|---|---|---|
| 1 | What CI-time budget (minutes) does the lint job get? | Set by the maintainer at the spec checkpoint from the §5 measurements (local baseline ~3.6s, full set ~6.2s cold); Task 8 enforces it. No default number — inventing one would be project policy. | Maintainer at checkpoint / blocks Task 8's timeout value only |

Requirements Q1–Q4 were all resolved during design (see design §7 D1–D7): curated shortlist with per-linter verdicts; local v2.13.2 trials; justification in spec record + PR description; config + per-domain fallout split.

## Research notes

- Trial command (all candidates, 2026-10-06): `golangci-lint run --no-config --enable-only <L> --max-issues-per-linter 0 --max-same-issues 0 ./...` at v2.13.2 (local install matches CI's pinned binary). The uncapped flags are mandatory — the default caps undercount and nondeterministically drop files (caught in validation round 1; the capped numbers were discarded). Combined: `-c` trial config = current `.golangci.yml` + 8 names → 95 issues, 47 files, ~6.2s cold (~1.2s warm).
- Fallout union by domain (measured uncapped): adapters 7 files, core 26 (adapter 1, admission 4, cli 7, integration 2, journal 2, supervisor 5, workers 2, workspace 3), tui 14. Task split follows this union exactly (design §4).
- The three clean enables (copyloopvar, usetesting, godot) contribute 0 findings — pure prevention, verified by the combined run.
- Rejected with costs: testpackage (39 files; white-box access load-bearing, e.g. tui model fields in tests), err113 (101 sites; fights the errorlint-governed contextual-error style).
- New comments are written with trailing periods (godot is clean today and stays so).

## Discoveries
