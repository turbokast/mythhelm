---
layout: default
title: Responsive layout blockers — 2026-11-02
---

# Responsive layout blockers — 2026-11-02

**The full product is unfinished. At least 20× faster full-product delivery is
not demonstrated. Actual MYTHHELM admission refused before launch; authoring
used direct-session fallbacks, not MYTHHELM-owned execution.**

This is a fixed **2026-11-02 09:03 UTC snapshot**. Later repairs and merges do not
rewrite it. [Report 0010](0010-observatory-and-ci-failures.md) preserves the
previous observation, including the failed pane attempts and Windows collector
failures.

## What shipped

[PR #562](https://github.com/turbokast/mythhelm/pull/562) merged the agent,
evidence and verdict pane producers on November 1 at 11:57 UTC, in `fa1516d`.
It did not complete the responsive composition.

[PR #561](https://github.com/turbokast/mythhelm/pull/561) merged the test timing
collector and deterministic drain deadline coverage at 13:18 UTC, in `de655cd`.
Its final 33 PR checks passed, and all six main workflows succeeded. Earlier
Windows Unicode, newline, drain-test and DCO failures remain in the history.
These diagnostics establish no delivery speedup.

## Layout tests passed while the full gate failed

Task15's unmerged checkpoint `f319475` contains 22 complete literal captures
across the eleven adopted sizes in Unicode and ASCII. Its TUI race suite and
five other canonical gates passed. The full repository race gate failed after
353.5 seconds: two packaged PTY cleanup cases timed out waiting for a complete
run ID. The adopted narrow Tasks column clips that ID. A short actual-view
witness confirmed the clipping; increasing the timeout would not fix it.

The cleanup fixture needs a readiness signal that does not depend on incidental
column capacity, while retaining exact run identity and child cleanup checks.
It belongs to the core test domain and is being handled separately from the
TUI correction. Neither change had merged at this snapshot.

A second check found a deeper problem. A private counter in the actual Diff
formatting loop observed **10,000 formatted rows for a terminal height of 30**.
The output-height tests still passed because clipping happened after formatting.
They did not prove the requirement to format only visible rows on a key press.
The next correction must bound work before formatting while preserving full
exports, retained totals and physical-truncation evidence. Passing output bounds
is not evidence of bounded rendering work.

These are integration defects, not flakes. Task15 remained unaccepted, with its
two allowed repair batches exhausted. No PR or dependent Task16 dispatch was
claimed for that attempt. A fresh bounded correction started on November 2;
its completion and CI were still pending.

## Coordination also cost time

The coordinator session became unavailable during the work. The worker reported
waiting for the requested static-check slot, continued within its existing
bounded authorization, froze the failed candidate before its deadline and did
not publish without independent review. Review and measurement reconciliation
resumed the next morning. The interruption was not productive execution time.

The coordinator also unnecessarily requested approval for two omitted TUI test
paths and a later task's model-state path. The existing implementation procedure
already permits explicit, reviewed deviations within the same domain. After an
independent check, the agent cancelled its redundant requests and resumed the
layout work without changing the original task fields or dependencies. Those
cancellations were agent corrections, not founder approvals of spec amendments.
The delay remains part of the delivery history.

## Self-use and measurement limits

The new correction, review and documentation attempts used normal MYTHHELM
admission with a restricted Codex route. Admission refused with
`execution_profile_unavailable`; no native work launched through that route.
Direct fallback was explicitly recorded. This is real use of admission, but it
does not establish MYTHHELM-owned authoring or a qualified native execution path.

The rejected layout attempt has no complete terminal monotonic timing receipt.
Its accepted-delivery duration is therefore unknown; the overnight interruption
was not silently counted as measured execution. Full-gate durations and source
caps describe different intervals. Token counts, model/version, monetary cost
and complete phase timings also remain unknown. Historical unfinished records
remain unresolved rather than being assigned invented outcomes. Overlapping
records are not independent trials, and later defects remain possible.

No completed full-product baseline or controlled comparison establishes 20×.
See the [measurement plan](../benchmarks.md). The
[service shell staging limitation](../limitations.md#service-shell-staging)
also remains: fresh startup adopts the root and legacy local run/recover/apply
then refuse it until their service clients land. The shell serves authenticated
status; run.start and attachment.open remain unsupported. Service-owned dispatch
and native ingestion are unfinished. Fixture and local-test results do not add
native or platform qualification.
