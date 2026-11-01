---
layout: default
title: Self-building delivery wave — 2026-11-01
---

# Self-building delivery wave — 2026-11-01

**The full product is unfinished. At least 20× faster full-product delivery is
not demonstrated. Actual self-use exposed admission and acceptance blockers;
implementation continued through direct native sessions.**

This is a coordinator observation, not a controlled benchmark. The fixed snapshot
is **2026-11-01 01:00 UTC**, with main at
[`9c6b05e40c3002e054bedc9fbc4b3fcc492600ab`](https://github.com/turbokast/mythhelm/commit/9c6b05e40c3002e054bedc9fbc4b3fcc492600ab).
The pending candidates below are separate from that merged source. Later merges
do not retrospectively make this snapshot complete. The
[previous report](0005-self-building-status.md), including three failed whole-spec
reviews and a required CI failure, remains the historical record.

## Delivered and still pending

| Work | Evidence and result at the snapshot |
|---|---|
| Service-admission design, Task 22 | [PR #541](https://github.com/turbokast/mythhelm/pull/541) merged at `821a3d249e549ef09a74229cfe858522e412580c`; [main CI succeeded](https://github.com/turbokast/mythhelm/actions/runs/41256131370). PR #541 records design acceptance; the follow-on specification merged separately in [PR #543](https://github.com/turbokast/mythhelm/pull/543). Neither implements service dispatch. |
| Versioned migration backup guidance, Task 28 | [PR #549](https://github.com/turbokast/mythhelm/pull/549) merged at the snapshot main revision. It documents the existing backup/preview naming behavior; it adds no migration or native capability. |
| Nonblocking observer primitive, Task 27 | [PR #551](https://github.com/turbokast/mythhelm/pull/551), candidate `58766aa0972aaa122ac4606fcd8afa1661ba69ce`, was open with CI pending. A primitive is not integrated service delivery. |
| Fixed quota policy/storage extraction, Task 29 | [PR #552](https://github.com/turbokast/mythhelm/pull/552), candidate `19139e673493241df1d84ec6ded1923a343d3454`, was open; required CI and merge were outstanding. It is a policy/storage refactor, not evidence of provider quota availability. |
| Revision-aware standalone client, Task 31 | Local candidate independently reviewed; final full-suite validation was still running, with no PR or merge at the snapshot. Client fixtures do not establish registered-service behavior or a successful native launch. |
| Canonical admission-binding codec, Task 23 | Local gates passed, but independent acceptance remained blocked on exact challenge semantics. [Issue #550](https://github.com/turbokast/mythhelm/issues/550) records the missing producer and comparison obligation. No completed task or semantic consent validation is claimed. |

The exact [task contracts at this source revision](https://github.com/turbokast/mythhelm/blob/9c6b05e40c3002e054bedc9fbc4b3fcc492600ab/specs/in-progress/supervisor-attachment-control/tasks.md)
separate these producers from their later service consumers. Service-owned ordinary
dispatch and qualified Codex execution remain unfinished; see the
[limitations register](../limitations.md).

## Real self-use and the fallback boundary

Before authoring Tasks 23, 27, 28, 29 and 31, the coordinator requested the Codex
route through MYTHHELM with the restricted execution profile. All five refused
**before native launch** with `execution_profile_unavailable` (exit 7,
`capability_unavailable`): filesystem, process, network and credential boundary
coverage were missing. These are actual MYTHHELM admission attempts, not five
completed native runs. The product's refusal was preserved.

Authoring proceeded through explicitly recorded **direct-native fallbacks** in
isolated worktrees. MYTHHELM did not own those native executions. This report's
own requested route also refused before launch and used a separate direct fallback;
it is outside the five-task count above. The
[earlier admitted maintenance pilot](0003-measured-self-hosting.md) retains its
own conditions and does not qualify these refused routes.

Private measurement records distinguish refused routes from direct authoring.
Native token counters and unobserved timing remain unknown, not zero. Raw native
output, private account/configuration evidence and measurement ledgers are not
published. No billing, subscription entitlement or new native qualification is
established by this wave.

## Failures that still count

Passing tests did not establish Task 23's complete contract. Shape checks and
nonempty challenge fields cannot prove that confirmation matches the complete
freshly derived challenge ID, kind, text and authority set. The public issue
requires independent negative fixtures for missing, extra, wrong-kind,
changed-text, swapped-ID and changed-authority challenges. A proposed boundary
explanation is not adopted acceptance evidence; the task remains blocked.

The direct implementation wave also encountered local compile/lint errors and
watchdog test failures in full validation runs. Those failures remain in delivery accounting.
The underlying cause of the watchdog failures is unproven; concurrent validation
and contention are observations, not a demonstrated diagnosis or a reason to
label failures flaky. Validation was then scheduled with less contention while
retaining required checks.

One observed serial validation retry completed in **182.5 seconds**, following a
failed concurrent run observed for **581.4 seconds**. These are local validation
segments with different execution conditions and outcomes, not equal-scope
request-to-accepted-delivery measurements. They establish neither a causal speedup,
a billing saving nor the 20× full-product target. Code authoring, local gate time,
review, CI waiting and merged acceptance are different scopes.

## What this wave establishes

There is delivered design/documentation and implementation in progress, with
honest refusals and an unresolved acceptance gap. There is still no frozen,
completed full-product baseline, matched comparison or complete end-to-end time
and usage evidence. Later defect observation is incomplete. PR counts and fast
local retries cannot fill those gaps.

The next useful evidence is accepted producer behavior, integrated service
consumers and repeat delivery through an admitted native route. The
[measurement and acceleration plan](../benchmarks.md) requires equal accepted
scope, independent review, failures, waits, human effort and later defects before
any comparative performance claim. Required acceptance must remain intact while
delivery becomes faster.
