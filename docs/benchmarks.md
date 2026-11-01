---
layout: default
title: Benchmarks and dogfooding
---

# Benchmarks and dogfooding

**No current controlled result establishes that MYTHHELM is faster, uses fewer
tokens or produces fewer bugs than direct native agents or other orchestrators.**
We are building reproducible evidence before making that claim.

The [focused validation plan](validation-plan.md) defines the next bounded
feasibility cycle: one native workflow, a strong direct-native baseline, external
repeat use and explicit continue/simplify/stop criteria. Its targets are not results
or authorization to launch experiments.

Contributor backlog delivery requires admitted MYTHHELM execution where available,
a private measurement record for each attempt, and a recorded reason for direct
fallbacks. Blocked launches, errors, retries, missing usage and unfinished work
remain visible. A blocked launch is not a completed native attempt. See the
[measured delivery procedure](harness/measured-delivery.md).

## Delivery target and current gap

The project target is **at least 20× faster accepted delivery of the full product**.
This is a target, not an observed result, forecast or guarantee. A defensible
full-product baseline has not been established and the product is unfinished.
We cannot currently report a measured speedup against this target.

A valid comparison must freeze the product scope, acceptance revision and quality
floor, and include planning, implementation, failed attempts, verification,
review, CI, integration, human intervention and time blocked. For the same accepted
scope, the target is elapsed delivery time no greater than baseline time divided
by 20. Shipping a smaller scope, counting more PRs, or excluding failures and waits
does not establish it. The controlled-comparison requirements below still apply.

The [2026-11-01 delivery wave](dogfood/0006-self-building-delivery-wave.md)
records the accepted design and migration guidance, pending implementation, a
contract-blocked codec and five refused native task launches with direct fallbacks.
Observed local validation retries do not establish a speedup; native usage and
full delivery durations are incomplete. The full product and 20× target remain
unproven.

The [2026-10-31 self-building report](dogfood/0005-self-building-status.md) records
merged maintenance, three failed spec reviews, a required CI failure and repeated
native admission refusals with direct fallbacks. The product remains unfinished;
these outcomes do not demonstrate the delivery target or full self-hosting.

The earlier [2026-10-30 report](dogfood/0004-self-building-status.md) records
an admitted scripted demo, a refused native documentation attempt, direct-session
fallback and an incomplete design attempt. It is observational evidence of then-current
use and failures, not a benchmark. The older successful maintenance pilot below
remains historical evidence; it does not qualify today's route.

## How delivery must improve

The immediate work is to make one real native task-to-accepted-candidate route
reliably usable for MYTHHELM's own changes, then measure repeat delivery against a
strong direct-native baseline. Until that route is admitted, record the refusal
and fallback rather than describing direct execution as self-hosting.

Track time to accepted product behavior, unfinished attempts, rework and blocked
critical-path tasks. Parallelize ready work with disjoint files and merged
contracts. Batch a review round's corrections before rechecking the final tree,
reuse only gate evidence whose fingerprint is still fresh, and stop unsuccessful
work at its declared budget with a useful artifact or a concrete failure record.
Check failures cheaply before spending time on expensive validation where the
required check order permits it; changes to that order need tests and review.

CI waiting and repeated verification are delivery costs. Reducing them must retain
required checks, independent acceptance, authority boundaries and error visibility.
An acceleration claim requires measured outcomes; changing an execution policy,
writing this plan or adding another specification is not the claimed improvement.

## What we measure

| Outcome | Measurement |
|---|---|
| Accepted delivery | Independent checks and review of the exact candidate; native completion reported separately. |
| Time | Request to independent acceptance; separately observed native, verification and review durations. |
| Usage | Reported/observed input, output and cache counters, with provenance and missingness; tokens are not charges. |
| Reliability | Assigned attempts, retries, classified errors, reviewed bugs and delayed regressions. |
| Human effort | Observed interventions and review effort; unmeasured effort stays unknown. |

Controlled pairs hold task, source, native harness/model/effort, tools,
permissions, environment, budget and independent acceptance constant, with order
randomized before execution. Different products/configurations are separate
comparisons. Routine backlog records describe delivery, not randomized effects.

## Evidence available

The [historical comparison](dogfood/0002-comparison-evidence.md) describes six
exploratory task pairs with substantial limitations. It establishes no speed or
quality advantage and is not pooled with new trials. The [first live dogfood
record](dogfood/0001-first-run.md) demonstrates one recorded workflow, not current
qualification or comparative performance.

The new measurement tools have credential-free synthetic tests for recording,
comparison matching and analysis. These are not native benchmark scores. A [measured self-hosting pilot](dogfood/0003-measured-self-hosting.md)
now records a native-authored maintenance change and one completed development
pair. Direct execution was shorter on that pair; coordinator scheduling confounds
the end-to-end comparison. One dependent component and pending defect observation
establish no advantage. Renderer targets and CI durations cannot substitute for
task-quality comparisons.

## Publication criteria

Before assignment, register the population, holdout, independent sampling unit,
useful effect, sample-size design, quality floor, budgets, method and stopping
rule. Group repeated repositories/task families. Report all assigned outcomes,
failures, timeouts, paired effects, uncertainty, missingness and delayed defects.
Insufficient evidence is inconclusive.

The initial implementation uses conservative bounds across independent
repository/task-family components, jointly controls two registered endpoints and
checks prospective sample adequacy. The [method and
limitations](harness/measured-delivery.md#statistical-interpretation-and-publication)
are public; results still require independent evidence review.

A future README/site advantage claim will link a dated report with exact versions,
public fixture/check definitions, all-assigned acceptance, uncertainty and limits.
Until then, we make no comparative performance claim.
