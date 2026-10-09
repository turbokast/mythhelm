# contained-execution-profiles — Hand-off

> One section per task. Each task replaces its own `<!-- pending -->` line in its pull request with what
> dependent tasks need: what it produced (the API and files as shipped), what a later task must
> know, and any deviation that changes a later task's inputs. `/run-spec` puts the sections of a
> task's dependencies into that task's dispatch prompt. Open questions and research stay in
> `scratchpad.md`.

## Task 1 — Containment contract package

- **Produces**: `internal/contain/contain.go` exactly as design §4 (Dimension consts, Claim, Coverage, Evidence, Registry, AuthBind, Policy, Availability, ErrUnsupported, ErrMissingCoverage); no `Provider` interface yet.
- **For dependents**: `Coverage.Missing` returns nil (not empty slice) when nothing is missing, keeps the caller's order, and reports unknown dimensions as missing. A zero `Claim` is unenforced. Registry lookups must return `ok == false` for unknown combinations.
- **Environment trap**: set `GOTOOLCHAIN=go1.27.1` for golangci-lint, and unset `ANTHROPIC_BASE_URL` in cloud sessions or a supervisor test fails.
- **Deviations**: None.

## Task 2 — Linux boundary mechanism

<!-- pending -->

## Task 3 — Filtering egress proxy

<!-- pending -->

## Task 4 — `__contain` command and worker wiring

<!-- pending -->

## Task 5 — Admission consult, restricted default, precise refusal

<!-- pending -->

## Task 6 — Inspect read-only enforcement

<!-- pending -->

## Task 7 — Protected check evaluator

<!-- pending -->

## Task 8 — Honesty surfaces and receipt boundary evidence

<!-- pending -->

## Task 9 — Startup boundary and authority fixtures

<!-- pending -->

## Task 10 — Adapter startup-inventory seam

<!-- pending -->

## Task 11 — Adversarial suite and frozen v1 evidence

<!-- pending -->
