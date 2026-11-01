---
layout: default
title: Delivery blockers and self-use — 2026-11-01
---

# Delivery blockers and self-use — 2026-11-01

**At least 20× faster full-product delivery is a target, not a demonstrated result.
The full product is unfinished. We used MYTHHELM's native admission while building
it; admission refused before launch, so authoring used direct-session fallbacks.
MYTHHELM did not own that execution.**

This report fixes the **2026-11-01 08:49 UTC observation**. It extends, rather than
rewrites, [report 0007](0007-integration-delivery.md). Main was
[`e9fb98dc8b233f6a3d2c18f017bc292dd4d64e24`](https://github.com/turbokast/mythhelm/commit/e9fb98dc8b233f6a3d2c18f017bc292dd4d64e24);
its [CI run](https://github.com/turbokast/mythhelm/actions/runs/41288417232)
was observed successful. Pending PRs are not merged capabilities, and later
results do not change this historical snapshot.

## Delivered and pending

| Work | Observed state and limit |
|---|---|
| [#527](https://github.com/turbokast/mythhelm/pull/527), internal owned entrypoints | Merged; an internal ownership producer, not complete service dispatch. |
| [#555](https://github.com/turbokast/mythhelm/pull/555), service root adoption fencing | Merged; preserves the explicit migration boundary. |
| [#521](https://github.com/turbokast/mythhelm/pull/521), packaged lifecycle coverage | Merged; synthetic agents and real processes, not live-native qualification. |
| [#554](https://github.com/turbokast/mythhelm/pull/554), Observatory theme validation | Merged; does not finish the renderer. |
| [#557](https://github.com/turbokast/mythhelm/pull/557), seven test-fixture files | Merged ordinary prerequisite repairs. Accepted fixture work is not seven new product features. |
| [#556](https://github.com/turbokast/mythhelm/pull/556), semantic spans and status glyphs | Open; CI OK was successful and CodeRabbit neutral. Neither observation is a merge or completed mandatory review. |
| [#558](https://github.com/turbokast/mythhelm/pull/558), production service shell | Open at `7b2fc15f68b4e0c43fd3331cef159d35ee1262ff`; blocked by actual documentation-site, macOS Go and Windows ARM Go failures. |

The failed #558 jobs are [documentation site](https://github.com/turbokast/mythhelm/actions/runs/41289256658/job/124797639985),
[macOS](https://github.com/turbokast/mythhelm/actions/runs/41289256658/job/124797182111)
and [Windows ARM](https://github.com/turbokast/mythhelm/actions/runs/41289256658/job/124797182136).
Their causes were not established for this snapshot. They cannot be labelled
transient or unrelated. This report has not passed rendered-site CI either.

The shell staging warning in the README and [limitations](../limitations.md#service-shell-staging)
remains material: fresh service startup adopts the root, after which legacy local
execution refuses it until service clients land. Authenticated status is not
service-owned dispatch or native ingestion. Immutable generation/storage producers
for service Task 7 and retained ownership handoff for service Task 11 remain
undelivered. A proposal for prerequisites is not approval or implementation.

## Failures that still count

Several fixture attempts were incomplete or failed before acceptance. A Windows
journal `SQLITE_BUSY` failure remains unexplained; a bounded contention-pair
fixture does not prove its Windows cause. A migration setup failure after an
80 ms wait also has no established cause. Passing focused checks cannot erase
those failed observations.

Task 6's first source attempt was rejected by lint. Its separate lint repair
missed its artifact checkpoint; that lateness remains recorded. A later full Go
run failed after 395.8 seconds: starting the new service on a fresh root adopted
it, and the legacy packaged run then refused with `ownership_unresolved`, exit 6.
A read-only diagnostic confirmed the adopted phase. The repaired deadline fixture
uses an owned helper holding a real instance lease and positive generation,
without starting the service or editing migration phase or heartbeats. It retains
the real packaged worker's two-second envelope, spool, exit, recovery, single
launch and user-stop assertions. It qualifies generation fencing in that fixture,
not service-owned native execution.

Review then found an unjoined readiness scanner **after local gates had passed**.
That fixture attempt was rejected. A separate repair added an explicit scanner
join on every cleanup path, closing its owned pipe reader if the child could not
be joined. A controlled blocked-scanner proof passed with the join and failed its
completion assertion when only that join was removed. This is evidence that
review caught a defect the passing gates missed, not evidence of an observed
production leak or cancellation of arbitrary OS I/O.

## Delivery time and remaining bottlenecks

Five accepted prerequisite artifacts contributed to the same #557 integration:

| Artifact | Observed elapsed wall seconds |
|---|---:|
| Runtime authority fixture repair | 7,374.362 |
| Root identity fixture repair | 9,428.112 |
| Migration deadline fixture repair | 6,751.260 |
| Journal contention fixture repair | 4,264.699 |
| Deadline generation fixture repair | 2,334.233 |

These intervals overlap and include coordination, verification, review and CI
waiting. They are not five independent trials and cannot be added or divided
into a speedup. Finite source-author caps cover a different interval from accepted
end-to-end delivery. Cached Go gate durations are not comparative benchmarks.
Model/version, token usage, cost, complete phase timings and later defect outcomes
remain unknown; unknown does not mean zero. Defect observation remains open.

Go gate markers also do not cover every input: 47 non-Go assets and build inputs
were checked separately by manual hash comparison. That is limited manual
coverage, not a universal gate guarantee. Mandatory external CodeRabbit review
was rate-limited, with observed included capacity of one review per hour; no paid
setting was changed. Parallel authoring cannot remove that review bottleneck.

The [measurement plan](../benchmarks.md) still requires equal accepted scope,
failures, waits, review and later defects before any comparative claim. There is
no completed full-product baseline or matched comparison demonstrating the 20×
target. The useful next evidence remains integrated behavior and repeat accepted
delivery through an admitted native route, with these shortcomings kept visible.
