---
layout: default
title: Self-building status — 2026-10-31
---

# Self-building status — 2026-10-31

**The full product is unfinished. At least 20× faster full-product delivery has
not been demonstrated. MYTHHELM is only partially self-hosting.**

This is a dated coordinator observation at main revision
`bb9c99fd2a9582e1e703eb9234291cf577d5dc74`, with the separately identified
specification candidate below, followed by the fixture-repair merge identified
separately below. It records failures as well as accepted changes.
Subsequent fixes do not turn these failed attempts into first-pass successes.
Raw private ledgers, account/configuration data and native output are excluded.

## What actually reached main

| Change | Result and practical limit |
|---|---|
| [Fail-fast Go gates and review-base handling, #538](https://github.com/turbokast/mythhelm/pull/538) | Local gate ordering and base handling improved. Follow-up main CI failed on an existing Windows task-file read; merging the change did not establish a healthy complete product. |
| [Separate Windows supervisor/rest test shards, #540](https://github.com/turbokast/mythhelm/pull/540) | The suites run on separate Windows runners while retaining the existing race/ownership checks and refusing skipped required Go jobs. Main CI passed afterward. This is a CI improvement, not native Windows qualification or measured delivery acceleration. |
| [Pinned browser driver update, #542](https://github.com/turbokast/mythhelm/pull/542) | The independently signed maintenance update passed checks and merged. Main CI at the revision above succeeded. A dependency update does not deliver the broader multi-agent product. |

The implemented CLI/TUI slice and its unsupported cases remain listed in the
[README at this source revision](https://github.com/turbokast/mythhelm/blob/bb9c99fd2a9582e1e703eb9234291cf577d5dc74/README.md#dogfood-slice-status) and
[limitations register](../limitations.md). Service-owned ordinary dispatch,
qualified Codex execution and the broader adaptive product remain unfinished at
this snapshot.

## What failed or remains unaccepted

The service-admission follow-on proposal in
[PR #543](https://github.com/turbokast/mythhelm/pull/543), candidate
`ab1d6a9f9f5d4536b3dbd2bcdd81a06c40d0de9e`, went through three whole-spec
reviews, each finding blocking defects. They included incomplete consumer interfaces,
missing preparation-revision handling, nonexistent wire metadata and a
retention-only request that could never become ready. The
[candidate scratchpad](https://github.com/turbokast/mythhelm/blob/ab1d6a9f9f5d4536b3dbd2bcdd81a06c40d0de9e/specs/in-progress/supervisor-attachment-control/scratchpad.md)
retains that history. One bounded correction then received two focused Ready
reviews; those do not relabel the three whole-spec failures as passes.

Local checks passed: 36 harness suites, 2,076 checks, harness lint, shellcheck and
public hygiene. The [required Linux CI job](https://github.com/turbokast/mythhelm/actions/runs/41247137136/job/124672817437)
nevertheless failed before a malformed-input
test's copied child executable could start: the OS reported a busy executable.
That result blocked the spec merge. A separate fixture repair was prepared for
independent review and fresh CI; the failed check was neither waived nor manually
rerun. The failure does not establish which process or kernel mechanism made the
file busy.

Publication review of this report also found a broken published-site README link
and missing links from the docs landing and measurement pages. The first draft
was not accepted; this correction fixes those documentation defects. They remain
part of this delivery's failed first-pass record.

After that snapshot, [fixture repair #545](https://github.com/turbokast/mythhelm/pull/545)
merged at `7b4429a45369d2a4baa8bcf4981d062bc4dfecd9`. It avoids writing a new
Unix executable for the recovery fixture, retains the real Windows executable
copy and strengthens the malformed-argument check. Required platform CI passed,
and the merged tree matched the tested candidate. The earlier failure remains
recorded; its underlying writer or kernel cause was not established, and local
repetitions of the old fixture passed. The updated spec still requires its own
fresh CI and merge.

At this snapshot, the separately merged follow-on spec and the ADR acceptance
record required for Task 22 are still outstanding. Task 4's ownership review also
remains separate. An architecture document, passing helper tests or a proposed
spec is not completed service dispatch.

## How MYTHHELM was used while building itself

Before the implementation/documentation attempts, the coordinator requested the
Codex route through `mythhelm run` with `subscription-only` billing and the
restricted execution profile. Admission refused before native launch because
filesystem, process, network and credential boundary coverage were missing.
The refusal was retained; host trust, billing facts and configuration approval
were not inferred to get around it.

Actual authoring then used explicitly recorded direct Codex-session fallbacks,
including this report. Those edits are not MYTHHELM-owned native execution.
Admission attempts are real product use and expose a product limitation; they
are not accepted native-authored work. The
[earlier admitted maintenance pilot](0003-measured-self-hosting.md) and
[previous diagnostic/demo report](0004-self-building-status.md) retain their own
conditions and limits.

Each attempt has a separate private measurement record. Missing native usage,
model/version information and unobserved timing remain unknown. Failed reviews,
CI, waiting and integration belong in delivery accounting; PR counts alone do
not measure accepted full-product output.

## What must improve

The immediate work is to validate the integrated spec, merge the separately
reviewed contracts and implement their independent producer tasks while retaining
the ownership and admission gates. More accepted native-authored changes through
an admitted route are needed to establish dependable self-use.

There is still no frozen, completed full-product baseline or matched comparison
that establishes the **at least 20×** target. The
[measurement and acceleration plan](../benchmarks.md) requires equal accepted
scope, failures, review, CI, human effort, waits and later defects. These routine
observations demonstrate problems and some delivered maintenance; they provide
no defensible performance ratio.
