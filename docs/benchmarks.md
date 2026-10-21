---
layout: default
title: Benchmarks and dogfooding
---

# Benchmarks and dogfooding

**No current controlled result establishes that MYTHHELM is faster, uses fewer
tokens or produces fewer bugs than direct native agents or other orchestrators.**
We are building reproducible evidence before making that claim.

Contributor backlog delivery requires admitted MYTHHELM execution where available,
a private measurement record for each attempt, and a recorded reason for direct
fallbacks. Blocked launches, errors, retries, missing usage and unfinished work
remain visible. A blocked launch is not a completed native attempt. See the
[measured delivery procedure](harness/measured-delivery.md).

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
