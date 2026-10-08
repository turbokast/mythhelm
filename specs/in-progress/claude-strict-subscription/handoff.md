# claude-strict-subscription — Hand-off

> One section per task. Each task replaces its own `<!-- pending -->` line in its pull request with what
> dependent tasks need: what it produced (the API and files as shipped), what a later task must
> know, and any deviation that changes a later task's inputs. `/run-spec` puts the sections of a
> task's dependencies into that task's dispatch prompt. Open questions and research stay in
> `scratchpad.md`.

## Task 1 — Effective-configuration inventory

- **Produces**: `claudecode.EffectiveConfig`, `PolicySummary{Digest, Sources, Gaps}`, `InventoryEffective`, `UnresolvedSources`, `GapSources` (`adapters/claudecode/effective.go`) and `Manifest.EnabledPlugins` (JSON `enabled_plugins`, sorted, deduplicated, empty non-nil when none), as designed.
- **For dependents**: `GapSources` returns a non-nil empty slice for no gap and follows unresolved-input order, so the first gap is deterministic for `missing-source:`. `InventoryEffective` always sets `ExtraUsage` and `PurchasedCredits` to `"unknown"` and `ChildrenRoutes` to empty; `LiveRecord` (Task 2) must refuse those, assessed values come only from the live suite.
- **For dependents**: `UnresolvedSources` is a package-level var; tests pass explicit lists to `GapSources` rather than mutating it.
- **Trap**: this container exports `ANTHROPIC_BASE_URL`, which fails unrelated `internal/supervisor` tests by design; run the go gate with it unset. golangci-lint needs `GOTOOLCHAIN=go1.27.1`.

## Task 2 — Auxiliary inventory and live evidence constructors

<!-- pending -->

## Task 3 — Strict admission flip with closed matching

- **Produces**: `admission.DriftError{Blocked, KeyHash, Reason}` with `Unwrap`; `(*qualify.Registry).InvalidateByHash`; `ResolveBilling(ctx, mode, evidence, decl, elig)`; `Consult` exact on AuthCategory, ProviderEndpoint, WorkspaceClass, EffortSettings, ModelSnapshot.
- **For dependents (Task 4)**: strict drift is returned by `ResolveQualification` itself as `(Eligibility{Blocked, "qualification_drifted"}, *DriftError)`; `Decide` passes it up, so use `errors.As` for `*DriftError` in `run.go`.
- **For dependents (Tasks 5, 7)**: the gap gate in `decideClaudeCode` runs only with a non-nil registry; a state dir that exists without a database is a present, empty registry and gap-blocks pre-Q1. Use a nonexistent dir for the missing-registry case.
- **For dependents (Task 5)**: a strict run with `--declare-entitlement` still sets `Decision.Declaration`; unreachable pre-Q1, so decide whether strict should skip it.
- **Q1 rewrites**: `TestStrictGapBlocksEndToEnd` and `TestDeclaredIgnoresGaps` skip once the gap list is empty; Q1's PR replaces them with the post-Q1 admit proofs.

## Task 4 — Drift-invalidation writer on the run path

<!-- pending -->

## Task 5 — Declared guards and strict end-to-end

<!-- pending -->

## Task 6 — Strict billing user documentation

- **Produces**: `## Billing` in `docs/user-guide.md` and `docs/decisions/0012-strict-subscription-admission.md` (accepted).
- **For dependents (Q1, Task 7)**: the guide says no live-qualified record exists and every strict run blocks today, and ADR-0012 says strict blocks until a live-qualified record is committed. Update both statements when Q1 and Task 7 land.

## Task 7 — Maintainer live qualification run

<!-- pending -->
