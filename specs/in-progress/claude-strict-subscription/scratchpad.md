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
