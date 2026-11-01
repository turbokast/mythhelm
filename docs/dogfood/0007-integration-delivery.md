---
layout: default
title: Self-building integration delivery — 2026-11-01
---

# Self-building integration delivery — 2026-11-01

**MYTHHELM is being used while it is built, but this work was not executed by
MYTHHELM. Native admission refused before launch, and authoring used direct
sessions. The full product is unfinished; at least 20× faster full-product
delivery remains unproven.**

This is a routine delivery report. Its fixed source snapshot is **2026-11-01
02:48:50 UTC**, main
[`4cc5315a60055e239db7fb2fec5edfa4330cfb25`](https://github.com/turbokast/mythhelm/commit/4cc5315a60055e239db7fb2fec5edfa4330cfb25).
The [earlier wave](0006-self-building-delivery-wave.md) preserves its 01:00 UTC
snapshot, pending work and failures. Later merges do not rewrite it.

## What actually merged

These eight PRs are the reported scope, not a count of all project work, finished
products or independent trials. Merge state and exact revisions were checked
against GitHub and independent acceptance records.

| Merged work | Public evidence | Scope and limit |
|---|---|---|
| Service-admission design, Task 22 | [#541](https://github.com/turbokast/mythhelm/pull/541), `821a3d249e54` | Accepted design; no service dispatch implementation. |
| Versioned migration backup guidance, Task 28 | [#549](https://github.com/turbokast/mythhelm/pull/549), `9c6b05e40c30` | Documents existing naming, with no new migration capability. |
| Fixed quota policy/storage extraction, Task 29 | [#552](https://github.com/turbokast/mythhelm/pull/552), `a482a7ea5027` | Internal producer; no provider-quota qualification. |
| Bounded nonblocking observer, Task 27 | [#551](https://github.com/turbokast/mythhelm/pull/551), `c71053625fd9` | Primitive; production observer wiring remains separate. |
| Revision-aware standalone client, Task 31, and earlier delivery report | [#553](https://github.com/turbokast/mythhelm/pull/553), `cbf28be8fe33` | Preserves server preparation revisions; client fixtures do not prove service delivery. Code and report shared one PR. |
| Task-file reload cancellation repair | [#528](https://github.com/turbokast/mythhelm/pull/528), `74a049aef400` | Preserves cancellation during asynchronous reload; no native qualification. |
| Packaged lifecycle proof, TUI Task 10 | [#521](https://github.com/turbokast/mythhelm/pull/521), `b31009a5490d` | Synthetic agents with real processes/PTYs test ownership, detach and recovery; no live-native qualification or completed Observatory renderer. |
| Owned lifecycle entrypoints, service Task 4 | [#527](https://github.com/turbokast/mythhelm/pull/527), `4cc5315a6005` | Trusted internal ownership extraction; no production owned caller or sealed admission. |

Required checks passed on the final PR heads before merge. That does not establish
successful push CI on the merged revision: the snapshot's
[main CI run](https://github.com/turbokast/mythhelm/actions/runs/41266698750)
was still **pending** when checked for this report. No main-CI pass is inferred
from PR checks or another revision's result.

## Self-use exposed boundaries

Actual restricted-profile requests through MYTHHELM refused before native launch
with `execution_profile_unavailable`: filesystem, process, network and credential
boundary coverage were missing. The refusal was retained, followed by explicit
direct-native fallbacks in isolated worktrees. MYTHHELM did not own their
execution. This report's own admission request also refused before launch.

The [historical admitted maintenance pilot](0003-measured-self-hosting.md) remains
valid within its recorded conditions; it does not qualify these refused routes.
No new billing, entitlement or platform qualification follows from this work.
Model, native version and token usage are unknown, not zero. Private configuration,
raw output and measurement ledgers remain unpublished.

## Bad news and incomplete work

The coordinator failed to refresh PR #527's base before publication. GitHub's
stale base made already-merged main commits appear in its diff: 81 files appeared
instead of the intended ten, and DCO and scope checks failed. Correcting the base
restored the intended scope; a signed update recording actual final acceptance
evidence triggered fresh checks through a normal push. The final head passed.
The earlier failure remains recorded; no manual workflow rerun or waiver erased
it. The earlier missing merge sign-off and validation failures remain in
[report 0006](0006-self-building-delivery-wave.md#failures-that-still-count).

Task 10's PR description also contained an unsupported blanket assertion about
historical cleanup mutants. It was corrected. Five **current** deliberate test
violations produced assertion failures, and restored controls passed; the five
historical outcomes remain **unknown**.

The admission-binding codec, Task 23, remains unmerged: its shape-only candidate
fails the adopted exact-challenge contract. A proposed boundary clarification
awaits approval; [issue #550](https://github.com/turbokast/mythhelm/issues/550)
tracks the gap and is not a waiver or an amendment to the adopted contract.
Passing shape checks did not establish semantic consent validation. Task 4's
mutable `admission.Decision` is not sealed admission. Calling an owner's `Close`
from its own callback really deadlocks; close/filesystem operations may wait
without forced cancellation. These are retained production-runtime constraints
for Tasks 6/7/9, not fixed or qualified behavior. The existing focused mission TUI is implemented. Production service dispatch and
the planned Observatory renderer remain incomplete; see the [limits](../limitations.md).

## Measurements and what they mean

| Accepted integration | Observed elapsed delivery | Scoped validation evidence |
|---|---|---|
| TUI Task 10 | 1,796.336 seconds | All 13 named race tests: author 66.153 s, independent acceptance 62.329 s; full race suite 287.0 s. Five current mutants failed assertions; restored controls passed in 4.521 s. |
| Service Task 4 | 2,155.621 seconds | Full race suite 336.9 s; three named independent race tests 13.372 s. |

The combined merged tree also passed **targeted** race checks: supervisor
15.709 s and end-to-end 73.900 s. These were not a full-suite run on the combined
tree. Synthetic fixtures and local gate passes do not qualify a real native route.

Elapsed delivery includes observed coordination, verification, review, CI waiting
and integration through independent acceptance. Separate native, verification and
review durations were not measured. Token counters, complete error counts and
human effort remain unknown; later defect observation is open. These two tasks
share one repository and are not independent samples or controlled comparison
arms. Test durations are different scopes and cannot be divided into a delivery
speedup.

There is no frozen, completed full-product baseline, matched comparison or
uncertainty estimate establishing the 20× target. PR counts and passing synthetic
tests cannot supply one. The next useful evidence is integrated production
behavior and repeat accepted delivery through an admitted native route. The
[measurement and acceleration plan](../benchmarks.md) requires equal accepted
scope, failures, waits, independent review and later defects before publishing
any comparative advantage.
