# Immutable local verification — Refinement log

`dispatched=2 returned=2 failed=0`

Verdict: **Ready after 2 rounds**. Both assessments used fresh contexts and returned a verdict plus all eight dimension scores. No human-required questions remain. This is requirements readiness only; no enforcement mechanism or platform support has been implemented or qualified.

## Round 1 — immutable_verification_assessor

Verdict: **Needs work**. D1, D4, D5 and D6 PASS; D2, D3, D7 and D8 NEEDS_WORK.

The initial requirements left concrete profile/mechanism, budgets, retention and compatibility decisions for design; code-grounding and dependency/consumer scope also needed correction. The original capture, assessment and deterministic counterexample remain private evidence.

The revision fixed the first profile to Linux/amd64 `go-fmt`, with actual `gofmt -l .` argv and the existing exit-zero plus empty-stripped-stdout contract. Execution uses an independently staged manifest-bound source copy inside a qualified private read-only mount namespace with a pinned bubblewrap or equivalently reviewed existing Linux helper. Missing enforcement means unavailable strong verification, never a writable-copy relabel. Host/controller/toolchain trust remains explicit; other gates/platforms remain legacy-observed until separately qualified, with no required CI check removed.

Concrete limits are 20,000 files, 64 MiB aggregate source, 8 MiB/file, 1 MiB copy buffer, 30 seconds capture, 10 seconds gofmt and 4 MiB combined output, with one capture/execution at a time. Cancellation allows 2 seconds grace and 5 seconds join/reconciliation before quarantine/refusal if ownership or process state remains unresolved. Store bounds are 64 snapshots/10 GiB; unpinned successful terminal snapshots expire after 24 hours and failed terminal snapshots after 7 days. Active, unknown-owner and referenced artifacts are never automatically deleted; safely unavailable capacity blocks new strong runs.

The centralized executable consumer inventory now names `gatelib.marker_state`, its status/stop-hook callers, the `verify-task-completion.sh` wrapper, and `health.py:check_gate_markers`; documentation/tests are distinguished from production readers. Schema 2 distinguishes immutable-qualified and observed-copy evidence; unknown critical schema refuses, and schema 1 requires explicit legacy-observed policy and never satisfies immutable evidence.

Frozen worktree use was corrected to an **assumption**, rather than a documented baseline requirement. Current code establishes endpoint comparison, not continuous input identity. The private real-gate counterexample exposes GOOD only at the fixture consumer read and restores BAD before exit: its actual successful target marker is fresh for BAD, while overall status exits 1 because unrelated markers are missing. It violates that frozen-use assumption and establishes neither hostile-host containment nor globally green verification. Tool/cache/marker metadata remain outside the source view; no private paths, raw configuration or reproduction output are published.

## Round 2 — immutable_verification_assessor_r2

Verdict: **Ready**. D1–D8 PASS; no findings, missing context or human-required questions. The corrected requirements bound design entry to the initial Linux/amd64 gofmt profile. Actual host enforcement and helper/tool artifacts must still be qualified by implementation evidence.

After assessment, only the lifecycle banner changed in requirements; behavior and acceptance criteria remained unchanged. The spec moved through the shared lifecycle script from unrefined to refined.

## Remaining notes for /spec

None blocking design entry. Preserve all fixed requirements, centralized consumer compatibility, trusted-host limits and explicit legacy/unavailable states. Requirements readiness does not earn implementation acceptance or stronger verification markers.

Next: `/spec immutable-local-verification`.
