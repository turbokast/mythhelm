---
layout: default
title: Measured self-hosting pilot
---

# Measured self-hosting pilot — 2026-10-21

MYTHHELM drove a native coding agent to add two real repository safety tests.
The coordinator independently accepted the frozen candidate, then repeated the
same development contract in a small MYTHHELM-versus-direct-native pilot.
**This establishes a working self-hosting path, not a performance advantage.**
The direct agent had shorter native execution on the completed pair.

## Task and reproducible inputs

The task adds regressions proving that the private measurement ledger refuses a
symlink state root and a hardlinked ledger without modifying their targets.
All fixtures are synthetic. The task changes one test file; no production
behavior, account settings or repository permissions change.

- Input revision: `955e2c564cfe319073748b7a51f2d0472e7146d3`, on the
  `bench/measurement-boundaries-20261021` input branch. It contains the public
  measurement implementation and isolated Python checks, not benchmark outputs.
- [Frozen task definition](fixtures/measurement-boundaries-task.txt), SHA-256
  `03e0b625ec145e09dce079d0abb2cd36276ef91d6b7f8a4bac1f93d48a920493`.
- [Frozen check configuration](fixtures/measurement-boundaries-checks.toml),
  SHA-256 `a352efcfd43f0cab3189b8774901ec69ecc73326a4532f3d5dc580d383d6b081`.
- [Registered matched-pilot protocol](fixtures/measurement-boundaries-pilot.json),
  SHA-256 `31b357d271bedcc8a18b4f034c321ab99d687beaedf0f7bababd32743c74ee84`.
- Accepted code artifacts: [first self-hosted patch](fixtures/measurement-boundaries-selfhost.patch),
  [matched MYTHHELM patch](fixtures/measurement-boundaries-matched-mythhelm.patch)
  and [matched direct patch](fixtures/measurement-boundaries-matched-native.patch).

MYTHHELM core revision `19495913d8bfdeb831d68975ed437fb57405abef` drove
Claude Code `2.1.305`. Both matched arms reported model `claude-opus-5-5` and
used the same existing configuration, configured `xhigh` effort, task bytes,
source, tool permissions and common environment. Orchestration context variables
and isolated workspace paths differ by design. Internal reasoning effort is not
observable. Private account/configuration data are not redistributed, so these
public fixtures reproduce the workload, not the complete private host setup.

The billing route was `subscription-declared`, following current operator
confirmation. This is a declaration, not proof of paid-continuation prevention
or strict subscription qualification. No metered fallback was used.

## All pilot cohorts

| Cohort | Native starts | Outcome and retained limitations |
|---|---:|---|
| Initial bootstrap readiness | 0 | Old-schema and untrusted-configuration refusals; automatic approval rejected launch before inference. Direct arm unstarted. |
| First approved self-hosting task | 1 | Candidate accepted; 68.586 s native-reported execution. One fresh entitlement refusal and clarification preceded inference. Direct arm remained unstarted under the equal-budget stopping rule. |
| Fresh matched pilot | 2 | Both candidates accepted after independent checks and review; observations below. |

The first approved task's recorded request-to-closeout time was 530.049 s,
exceeding its 480 s trial limit by 50.049 s. Native execution had ended at
216.273 s from the first request; later coordinator acceptance/bookkeeping caused
the overrun. We retain that deviation and do not use this cohort as a paired
comparison. These cohorts repeat one development contract, not three independent
tasks. The first accepted patch was selected for delivery before the matched
comparison; we did not choose the fastest candidate afterward.

## Completed matched pair

The protocol allowed one native attempt per arm, a 150 s native envelope, a
180 s request-to-independent-acceptance endpoint and a 480 s total trial limit.
The completed trial took 378.428 s including coordinator review and closeout.

| Observation | MYTHHELM + Claude Code | Direct Claude Code |
|---|---:|---:|
| Native-reported execution, seconds | 87.093 | 58.182 |
| CLI request to return, seconds | 95.338 | 62.283 |
| Request to independent acceptance/closeout, seconds | 166.207 | 212.201 |
| Candidate accepted after checks and review | Yes | Yes |
| Within registered 180 s acceptance endpoint | Yes | No |
| Additional native attempts | 0 | 0 |
| Observed permission denials | 3 | 3 |
| Native-reported input tokens | 20 | 18 |
| Native-reported output tokens | 6,227 | 5,309 |
| Native-reported cache-read tokens | 502,030 | 482,993 |
| Native-reported cache-creation tokens | 54,515 | 58,393 |

Both candidates passed the same 26-test suite and independent source review.
In private copies, disabling each relevant production guard made its new test
fail. No defect was found during that bounded review; the seven-day defect
observation window remains open. A missing observation is not zero defects.
The error counts here describe permission denials; total tool/native error counts
are not equally observable and are not claimed to be zero. Token categories stay
separate and are not dollar charges. Neither arm needed a new human intervention
inside this matched trial; configuration approval and entitlement clarification
occurred in the earlier setup cohort.

## Statistical interpretation

There is **one independent repository/task-family component**. The analyzer
returns no confidence interval, no quality-equivalence conclusion and
`joint_criterion_met=false`; publication still requires independent evidence
review. This pilot cannot estimate population performance.

Single-pair balancing places MYTHHELM first, so the run is not counterbalanced.
Vendor prefix-cache occupancy was not controlled. Coordinator waiting, review and
bookkeeping also differed: the direct candidate completed its native work sooner
but received final acceptance later. Its missed acceptance cutoff must not be
presented as a native-quality failure or a MYTHHELM speed advantage. The registered
restricted-time endpoint is therefore confounded by coordinator scheduling.

This is a feasibility measurement and instrumentation exercise, not confirmatory
evidence. General claims need a separately registered representative holdout,
adequate independent components, controlled acceptance scheduling, order balancing,
all-assigned outcomes and completed delayed-defect observation. See the
[statistical method and publication requirements](../harness/measured-delivery.md#statistical-interpretation-and-publication).
