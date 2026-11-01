---
layout: default
title: Service shell and delivery costs — 2026-11-01
---

# Service shell and delivery costs — 2026-11-01

**The service shell has merged. The full product is unfinished, and at least
20× faster full-product delivery remains unproven. We used MYTHHELM's native
admission while building it, but admission refused before launch. Authoring used
direct-session fallbacks; MYTHHELM did not own that execution.**

This is a fixed **2026-11-01 10:24 UTC observation**, on main
[`9d1eea4c85c4a08e5c575bf637326d26547ffcd1`](https://github.com/turbokast/mythhelm/commit/9d1eea4c85c4a08e5c575bf637326d26547ffcd1).
[Report 0008](0008-delivery-blockers-and-self-use.md) retains the earlier failed
checks, missed scanner cleanup defect and delivery waits. Its historical snapshot
is unchanged; subsequent acceptance does not erase its failures.

## What delivered, and what has not

[PR #558](https://github.com/turbokast/mythhelm/pull/558) merged the production
singleton service shell. The accepted PR head was
`7523ae5807e283322882134f6242319abae6410b`; six local gates passed, including the
full race suite in 333.5 seconds. Required final PR checks passed under the
existing canonical policy, external findings were resolved, and independent
review was retained. Main's own
[CI run](https://github.com/turbokast/mythhelm/actions/runs/41296060579)
was observed successful on the merged revision. These are distinct local,
review, PR and merged-tree observations, not interchangeable evidence.

The shell owns the root lifetime and serves authenticated status. A status request
writing no operation rows does **not** make startup read-only: successful fresh
startup initialises and adopts the root. Legacy local `run`, `recover` and `apply`
then refuse it until their service clients land. `run.start` and `attachment.open`
remain unsupported. Public observer delivery and service-owned native ingestion
are still absent. The [staging warning](../limitations.md#service-shell-staging)
remains applicable.

[PR #559](https://github.com/turbokast/mythhelm/pull/559), the gated Observatory
header and actions, remained **open and blocked**, at
`c32af25224d2ca5650fb865ffa39348c1004cdb2`. Its
[Windows Go job](https://github.com/turbokast/mythhelm/actions/runs/41296282947/job/124818030159)
was still running; no failing checks were observed in that read. Pending CI is
not a failure, merge or delivered capability. This report does not predict its
later result.

## Passing gates missed three review defects

The external review found three concrete code or test problems after earlier
local gates had passed:

- Transport and endpoint resolution happened after root preparation, so a resolver
  failure could follow adoption. Resolution now precedes preparation, with caller
  cancellation checked first. Injected resolver failures exercise that same path
  and leave the state root absent. They do not qualify a real unsupported OS or
  establish an actual OS endpoint failure.
- Three diagnostic-persistence failure paths discarded the original operation
  error. Worker acceptance, candidate freeze and spool ingestion now preserve
  both causes. Freeze and ingestion have dynamic failure controls; the worker
  acceptance change was inspected in source, not dynamically fault-injected.
- The stalled-view test used an unrelated timer rather than consuming observer
  output. It now connects a real sink/reader to a synthetic blocked consumer,
  checks producer and dispatch acknowledgments within two seconds, and closes the
  view before joining. A blocking callback control fails the intended assertion.
  This is a synthetic observer test, not public observation or native ingestion.

The first error-cause test also let an ingestion mutant survive because the
secondary diagnostic already wrapped two errors. The repaired assertion checks
the distinct diagnostic cause; both freeze and ingestion controls then fail when
the original cause is dropped. A stale handoff was corrected separately.

Lint rejected the first runtime attempt and later review-test code needed a lint
correction. Those failures remain part of delivery, alongside the late checkpoint
and scanner finding in report 0008. Successful later gates qualify their own
source revisions; they do not retroactively qualify earlier ones.

## Coordination also cost time

The coordinator initially demanded a fresh CodeRabbit `SUCCESS`, although the
unchanged canonical policy accepts `NEUTRAL`. That extra condition caused waiting
and was corrected without changing settings or weakening a gate. The rate-limited neutral result meant
no new automated review ran; it is not substantive approval. An earlier actual
external review produced the findings above, and independent review still mattered.

The coordinator also pushed revision `4107369` before its renewed local gates had
finished, contrary to the required order. That exact revision subsequently passed
before merge, but later success does not make the earlier push correctly ordered.
The [workflow](https://github.com/turbokast/mythhelm/blob/9d1eea4c85c4a08e5c575bf637326d26547ffcd1/WORKFLOW.md) remains the delivery contract.

Four accepted artifact records contributed to the **same PR #558**:

| Artifact | Elapsed wall seconds, rounded |
|---|---:|
| Synchronous-close fixture repair | 4,636.662 |
| Review-test lint correction | 2,137.881 |
| Startup-warning layout repair | 4,624.817 |
| Self-use report summary repair | 4,300.655 |

These intervals overlap and include coordination, verification, review and CI
waiting. They are not four independent trials, cannot be summed into total work,
and provide no speed ratio. Source-author caps and end-to-end acceptance time
measure different intervals. Cached test durations also are not delivery benchmarks.
Model/version, token usage, monetary cost and complete phase timings remain
unknown, not zero. Later defect observation remains open.

## Self-use still stops at admission

Restricted native admission was actually attempted and refused before launch.
Filesystem, process, network and credential boundary coverage remained absent for
the attempted route. Refusal is real product use; it is not native execution,
security qualification or MYTHHELM-owned authoring. No new platform, terminal,
G09/G11 or full-product qualification follows from synthetic fixtures, race tests
or successful CI.

The [measurement plan](../benchmarks.md) still requires equal accepted scope,
failures, waits, review and later defects before any comparative claim. There is
no completed full-product baseline or matched comparison demonstrating 20×.
This report's source checks also do not establish its rendered-site result:
new documentation CI remains pending until publication.
