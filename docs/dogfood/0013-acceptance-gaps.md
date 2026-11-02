---
layout: default
title: Native self-use and acceptance gaps — 2026-11-02
---

# Native self-use and acceptance gaps — 2026-11-02

**MYTHHELM is doing real native self-use. Successful native exits and passing
checks have still left rejected candidates and unresolved delivery defects.
The full product is unfinished; at least 20× faster full-product delivery is
not demonstrated.**

This is a fixed **2026-11-02 19:30 UTC snapshot**, based on main `301a36e`.
The [earlier native trial report](0012-native-self-building-start.html) and
[admission blockers report](0011-responsive-layout-blockers.html) remain unchanged
historical records. Later repairs do not erase their failures. These are curated
routine-delivery observations, not a new matched comparison.

## Real progress, with boundaries

[PR #571](https://github.com/turbokast/mythhelm/pull/571) merged the earlier
self-building report and corrected its historical context. It made actual
execution, rejection and unknown charges visible. [PR #574](https://github.com/turbokast/mythhelm/pull/574)
merged a repair isolating a review-stream watchdog from parallel fixture setup.
These are useful documentation and test repairs; neither completes service-owned
dispatch, the renderer or the full product.

Other candidates remain incomplete at this snapshot:

| Work | Observed outcome and remaining boundary |
|---|---|
| Attachment response queue handoff (Task35), first native attempt | MYTHHELM owned the native attempt. Native exit was zero, but the configured formatting check failed. Vet and race checks passed; later independent review found two production defects and a fixture-join defect. The candidate was rejected. A bounded second native attempt was in flight, with no accepted outcome at this snapshot. |
| Recording selector, [draft PR #576](https://github.com/turbokast/mythhelm/pull/576) | Fake-tool checks had passed, but later counterfixtures exposed an unselected media overwrite, omitted embedded-input freshness checks and a deletion-blind snapshot assertion. The publication workflow also assumes one shared pin, conflicting with selected mixed historical pins. Merge is held; no real recording follows. |
| Service fixture, [PR #573](https://github.com/turbokast/mythhelm/pull/573) | All local gates passed. Windows CI still failed in a seeded-journal fixture with SQLite busy. Its cause was unknown; local passes did not establish cross-platform acceptance. |
| Pre-spawn launch provenance (Task32) | All six local gates passed on a private integrated candidate including pending fixture prerequisites, but it was not yet published or merged. It was not a shipped capability. |

The selector's unchanged-unselected-bytes failures matter even though ordinary
fake-tool assertions passed. The response-queue defects matter even though vet
and race checks passed. Native completion, check success, independent acceptance,
CI and merge are separate observations. Earlier review evidence does not make
later counterexamples disappear.

## Task35 first-attempt observations

The first attachment response-queue attempt was actual MYTHHELM-owned native
execution using Claude Code **2.1.316**, with native-reported model
`claude-opus-5-5`; effort was unknown. The route was experimental `trusted-host`
with a `subscription-declared` billing posture. Entitlement was declared, not
independently verified; no containment or billing qualification follows.
The following categories describe different intervals and quantities:

| Observation | Value |
|---|---:|
| Monotonic engine elapsed time | 596.096 seconds |
| Native-reported duration | 538.035 seconds |
| Native-reported permission denials | 5 |
| Reported input tokens | 66 |
| Reported output tokens | 47,595 |
| Reported cache-read tokens | 3,117,963 |
| Reported cache-creation tokens | 127,459 |
| Estimated retail-equivalent usage | USD 2.5954286 |
| Actual billed charges | Unknown |

**Measurement erratum:** the earlier 596.403-second figure included an after-run
authentication-status probe by the outer controller. The corrected 596.096-second
engine interval ends at MYTHHELM command termination; it is not accepted-delivery
latency. Native-reported duration and all other observations remain unchanged.

The cache categories are separately reported usage, not fresh input tokens or
cash charges. The retail-equivalent estimate is not an invoice, a measured
subscription cost or proof of zero spending. Unknown charges remain unknown.
The successful native exit did not override the formatting failure or the later
source defects. The second attempt's outcome must be recorded later rather than
substituted into this snapshot.

## Fair comparison still needs evidence

The [October 21 maintenance pilot](0003-measured-self-hosting.html) already
recorded accepted native self-use and one completed matched development pair.
Direct native-reported execution was **58.182 seconds**, versus **87.093 seconds**
through MYTHHELM. Coordinator waiting confounded the acceptance endpoint; this
was one repository/task-family component and established no performance
advantage. Its later defect follow-up result is unknown here. These November 2
observations are neither the first self-use nor the first comparison.

No new paired comparison was launched for this report. A proposed controlled
parser pilot remained unassigned, with budget, model and configuration parity
unresolved. Routine native and direct fallback work cannot supply a missing
same-contract arm or protected assignment. There is no current evidence of
20×, faster, cheaper or higher-quality full-product delivery.

This report itself used a recorded direct Codex fallback after restricted-route
admission refusal. That documentation work is not MYTHHELM-owned execution.
Curated aggregate outcomes are public; raw native transcripts, private account
and session details remain private. See the [measurement criteria](../benchmarks.html)
and [current limitations](../limitations.html#service-shell-staging). Later
acceptance, repairs and CI results require dated follow-up evidence.
