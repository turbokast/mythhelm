# Claude Strict Subscription — Scratchpad

> Open questions and research from authoring. Each implementing task reads this file when it starts and adds an entry under Discoveries when it finishes.

## Open questions

| # | Question | Default in this spec | Owner / blocks |
|---|---|---|---|
| Q1 | Managed-policy source gap resolved, and how? | Gap stays open; `Gaps` data keeps records non-live | Maintainer/architect / any live claim (FR-1) |
| Q2 | Authorised live tests granted (allowance, fixtures, account class)? | No grant until Task 7; fixtures + skips only | Maintainer / Task 7, FR-2/FR-3 evidence |
| Q3 | First route still Claude Code? | Yes, per MH-10 Q1 default | Resolved from MH-10's outcome / FR-1 scoping |
| Q4 | Exact `auth status` fields constituting entitlement proof? | v2 §7.2 Claude focus list (credential precedence, managed policy, children, extra usage) | Native-docs research at implementation / AC-1.1/AC-2.1 detail |

## Research notes

- MH-10's specified contract is the dependency: `qualify.Key/Record/Consult/InvalidateOnDrift`, `ResolveQualification` + §4 key table, `ErrNotFirstRoute`, `authorised-live` method. Re-ground at implementation (MH-10 must be shipped).
- MH-10 §9 rows this spec closes: stable-matching limitation (exact non-drift match, Task 3) and restore-without-retest (persisted invalidation, Task 4).
- The `Decide` early return cannot be conditioned (no key pre-probe); removal + fail-closed consult is the only shape (D1).
- Live suite pattern follows `adapters/claudecode/live_test.go`: `//go:build live`, env gate, maintainer invocation, never CI, never automated.
- Current native docs were not re-fetched at /spec time; Task 2/7 confirm field semantics against installed `claude --version` output and current docs before recording live evidence.

## Discoveries

- Task 1: the manifest carries `managed*` digests only for sources actually present on disk, so `PolicySummary.Sources` is empty on a bare host and `Gaps` still lists the unresolved sources; gap assessment never depends on the manifest.
- Task 4: with the gap open (Q-15), a registry-present strict run returns a plain `*BlockedError` at the gap gate (`decideClaudeCode`, `GapSources(claudecode.UnresolvedSources)`) before `ResolveQualification`, and with no registry there is no record to drift, so no `*DriftError` reaches `executeRun` through `cli.Main`. The drift legs are proven at the seam with the real `*DriftError` from a consult; the `cli.Main` legs wait for Q1.

## Proposed observation-only extension (unapproved)

Task 8 is a local proposal, not maintainer approval, Q1 resolution, a qualification grant or permission to run a native. Q1's conservative default remains the existing gap. Open decisions for later reviewed tasks: exact supported native version and policy semantics, remote-cache and platform reader authority, WSL inheritance, safe source-set binding, runtime mutation boundary, and independent entitlement/stop evidence. Maintainer decides; unknown remains blocked. Do not expand completed task scope or infer later task approval.

## Synthetic Task 8 execution approval

The maintainer authorized continued autonomous delivery after the observation proposal merged in PR #533. Task 8 may implement the disconnected synthetic contract and its acceptance tests within the existing finite attempt and review limits. This does not resolve Q1 or authorize live source readers, native qualification, admission changes, or later tasks. The earlier proposal-only status above records the decision before this approval.
