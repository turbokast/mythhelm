---
layout: default
title: Observatory and CI failures — 2026-11-01
---

# Observatory and CI failures — 2026-11-01

**At least 20× faster full-product delivery is not demonstrated. The full product
is unfinished. We used MYTHHELM's native admission while building it; admission
refused before launch. Authoring used direct-session fallbacks, not
MYTHHELM-owned execution.**

This fixes the **2026-11-01 11:53 UTC observation**, using PR snapshots recorded
at 11:53:17 and 11:53:18 UTC. Later check results or merges do not rewrite it.
[Report 0009](0009-service-shell-and-delivery-costs.md) preserves the earlier
service-shell acceptance, missed defects and delivery costs.

## Delivered and pending

| Work | State at this observation |
|---|---|
| [PR #559](https://github.com/turbokast/mythhelm/pull/559), Observatory header and actions | Merged in main `e4099f6`. The header and gated actions are delivered producers, not complete responsive Observatory composition. |
| [PR #562](https://github.com/turbokast/mythhelm/pull/562), agent, evidence and verdict panes | Open at `f14983b`; local gates and independent review passed, but required CI was still pending. It was not a delivered merged capability. |
| [PR #561](https://github.com/turbokast/mythhelm/pull/561), CI test timings | Open at repaired head `6919027`; the first attempt failed both Windows CLI jobs. The repair passed local tests, while its new CI remained pending. |

The service shell's [staging limitation](../limitations.md#service-shell-staging)
still applies: successful fresh startup adopts the root, after which legacy local
`run`, `recover` and `apply` refuse it until their service clients land. The shell
serves authenticated status; `run.start` and `attachment.open` remain unsupported.
Service-owned dispatch and native ingestion are unfinished. The pane producers
also leave full responsive layout composition to later work. Neither pending PR
completes that product.

## Pane tests passed before integration defects appeared

The first pane attempt received conditional source review, then failed its full
Go gate after 333.5 seconds: the active slash filter disappeared from the rendered
view. Passing producer tests had not proved the existing viewport still reserved
space for its controls.

The next repair passed focused tests but introduced another concrete bug. Its
compact filter row could displace must-see notices. That attempt remained rejected,
and its gate run was interrupted; focused success was not acceptance. A third
attempt restores mandatory offer rows, then notices, then the filter when space
permits. Its six local gates passed, including the full race suite in
349.7 seconds, and independent review returned Ready on source `496da0a`.
PR #562's required CI and actual merge were still separate, pending obligations.

Sixteen earlier deliberately violating controls remain evidence for source
`6132c63`. They are not relabelled as current-source controls for `496da0a`.
The later viewport repairs have their own source-bound failures and focused checks.
These distinctions matter when a passing test set misses a different integration
problem.

The second pane review also missed its declared deadline: the 11:37:58 cutoff
was exceeded, with its artifact frozen at 11:39:29. A fresh third review froze at
11:41:01 within its own budget. Starting that separate review did not make the
late one timely or erase the rejected source.

## The timing diagnostic broke Windows output

The timing collector's first version, `be00357`, passed 18 local tests but failed
both actual Windows CLI jobs. It decoded Go's JSON output, then tried to write
Unicode punctuation through Python's default cp1252 output stream. U+201B could
not be encoded. The collector killed its Go child, and provider cleanup later
terminated orphan `cli.test` processes. Immediate-child cleanup was not proof of
whole-process-tree cleanup. These failures are not labelled flaky.

The repair, `6919027`, configures the script's stdout and stderr as UTF-8. A new
real-process regression forces cp1252, exercises the actual main entry and
collector with a synthetic JSON producer, and requires exact punctuation and
emoji bytes plus child exit 7. It fails on the old collector; all 19 local tests
pass with the repair. Those results do not substitute for the new Windows CI,
which was still pending in this snapshot.

Timing diagnostics are a way to find costs, not an optimization result. They
preserve the test inventory, race flags, package scheduling and failing exit
status. They establish no measured delivery speedup.

## Measurement gaps remain visible

Some resumed retry and review records used identifiers outside the canonical
measurement convention, and matching dispatch events were missing. Those gaps
remain recorded; they have not been retroactively filled in to manufacture
complete provenance. Later layout-planning work used a canonical event record,
which does not repair the earlier missing observations.

We publish no accepted delivery duration for either pending PR. Source-author
caps, review deadlines, local gate times and request-to-acceptance time describe
different intervals. Overlapping attempts are not independent trials, and their
elapsed times cannot be summed or divided into a speed ratio. Model/version,
token usage, monetary cost and complete phase timings remain unknown, not zero;
later defects can still change the assessment.

The [measurement plan](../benchmarks.md) requires equal accepted scope, failures,
waits, review and later defects before a comparative claim. There is no completed
full-product baseline or matched comparison demonstrating 20×. Restricted native
admission refusal is real product use, but not native execution or MYTHHELM-owned
authoring. No new G09/G11, terminal, native or platform qualification follows from
these fixtures, local gates or pending checks. This report has source checks only;
its rendered-site CI also remains to be established.
