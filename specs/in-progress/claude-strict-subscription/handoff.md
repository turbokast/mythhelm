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

- **Produces**: `claudecode.AuxiliaryRoute{Name, Funding, Evidence}`, `InventoryAuxiliary`, `LiveRecord`, `ErrEvidenceIncomplete` (`adapters/claudecode/auxiliary.go`); `testdata/qualification/live-shapes.json`; `## Live record` in `COMPATIBILITY.md`; `live_qualify_test.go` (`-tags live`, `MYTHHELM_LIVE_QUALIFY=1`).
- **For dependents**: `LiveRecord` reuses `ErrNotFirstRoute` and the `RecordDraft` predicate (harness and surface only). Evidence `Suite` is `live-qualify:<executable-digest>`. The record's capabilities are `credential-precedence`, `managed-policy`, `extra-usage`, `purchased-credits` and `stop_at_exhaustion`.
- **For Task 7**: the live suite needs `MYTHHELM_LIVE_STATE_DIR`, `MYTHHELM_LIVE_EXTRA_USAGE` and `MYTHHELM_LIVE_PURCHASED_CREDITS` (maintainer account observations). It was never grant-run; expect to adjust it on first execution.
- **Trap**: only the seven AT-03 routes can be `included`; configs with plugins or MCP servers keep `unknown` routes and `LiveRecord` refuses them.

## Task 3 — Strict admission flip with closed matching

<!-- pending -->

## Task 4 — Drift-invalidation writer on the run path

<!-- pending -->

## Task 5 — Declared guards and strict end-to-end

<!-- pending -->

## Task 6 — Strict billing user documentation

<!-- pending -->

## Task 7 — Maintainer live qualification run

<!-- pending -->
