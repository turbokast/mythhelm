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

- **Produces**: `contain.ProbeLinux`, `EnterLinux(ContainSpec) error`, `ContainSpec{Path, Args, Dir, Env, Policy}` (`Args` is the full argv), `PolicyFor`, plus exported `ProbeEnv` and `RunProbeChild`; unexported `nsSysProcAttr()` returns the user+mount `SysProcAttr` (nil off Linux).
- **For dependents**: `EnterLinux` must run in a process already started with `nsSysProcAttr()`; it needs an absolute `HOME` in `Env`, existing workdir and `HOME` paths outside `/tmp`, and auth-bind targets under `HOME`. It never returns on success.
- **Task 4 must**: call `RunProbeChild` from `contain.Main` / `cmd/mythhelm` when `ProbeEnv` is set (`ProbeLinux` runs `<self> __contain`), or `ProbeLinux` reports a spurious failure; export or reuse `nsSysProcAttr` for the worker spawn.
- **Tests**: the contain test binary acts as its own helper (`TestMain` modes) — the same pattern fits Task 4.
- **Deviations**: see the entry; the wiring gap above is the only one that changes a later task's input.

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
