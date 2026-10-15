# Existing run client — Scratchpad

## Open questions and conservative defaults

- **Q1: Merged producer/TUI signatures.** Owner: implementing agent with architect review. Core attachment contracts and MH-23 are prerequisites; re-ground exact signatures before Task 1/2. Do not create a fallback to direct journal or owner hooks if signatures differ.
- **Q2: Platform lifecycle evidence.** Owner: core lifecycle and qualification maintainers. Default: unsupported until both prerequisite process boundary and this child's packaged tests qualify the exact platform. Skipped PTY tests do not qualify a platform.
- **Q3: Optional restoration/callback support.** Owner: `herdr-scoped-projection`. Default: no host registration, no raw native resume. Public manual attach is required independent of host outcome.
- **Q4: Public command decision record.** Owner: core spec ADR 0021 reviewer. This design supplies command/interrupt/output choices for that ADR; no implementation before the reviewed public/ownership contract. No product approval signature is inferred from conversational authorization.

## Design record

- 2026-10-15: Existing run client child drafted by Codex `/root/herdr_client_design` under parent coordination. Preserves source FR-1 and AC-4.3, with core FR-3/4 normative dependencies. Seven implementation tasks, no code implementation or lifecycle move performed here.
- Grounding at `61d18e2`: CLI command registry, inspect TUI configuration, local snapshot polling and owner action hooks read directly; master v2 §§14–15 checked. Core child §§4–6 supply future interfaces, not claimed implemented symbols.
- Ordinary service-owned production run is mandatory; recovery-only enrollment and helper-seeded public attach tests cannot satisfy FR-1.
- Attached view consumes only typed safe projection. Review navigation shows canonical review state and trusted standalone review guidance; it does not widen host metadata to raw receipts/diffs.
- Host navigation input is a bounded allowlist, distinct from terminal confirmation. StopResult includes immutable acknowledgment so fresh-generation query recovers lost acknowledgment without stale resubmission.
- Contributor tests remain offline and host-independent; no benchmark or live qualification result is claimed by this design.

## Task notes

No implementation tasks completed.

## Validation round 1 fixes

Fresh independent review found missing standalone observation/results, canonical adoption schema mismatch, stop producer ordering, host freshness/resync transport and stable-status TTL renewal. The author added exact typed contracts, canonical schema2 adoption with separate historical decoding, producer-first dependencies, safe freshness fanout/read-only resync and independent qualified publication renewal. Shared docs wait for docs-site-restyle Task11. These are contract/ordering corrections, not source requirement or ownership-policy changes. Round2 fresh validation is pending; this author check is not an independent verdict.

## Final validation round 3 and pending correction

The third independent whole-epic validation returned Ready for the epic, supervisor-attachment-control and herdr-scoped-projection, and Needs revision for existing-run-client. Its sole blocking finding was the client design §4 unknown-run typed not-found/exit 2 contract contradicting the core design §§4–5 unknown/foreign permission_denied privacy contract. Under delegated engineering judgment, the coordinator retained the existing producer contract: unknown and foreign runs uniformly return permission_denied/exit 3 without existence disclosure; invalid arguments remain exit 2. The client design and Task3/Task6 acceptance tests now state that contract explicitly. Source requirements and scope are unchanged.

At the close of the prior run, this correction awaited independent validation. Its three-round limit was exhausted, so no fourth validation, lifecycle move or Ready claim followed that author correction. The drafts remained unrefined pending a newly resumed spec run; lint and hygiene were author checks only.

## Resumed spec run — validation round 1

After the delegated decision to preserve the producer privacy contract, a new spec run received a fresh independent whole-epic review. The epic and all three children returned Ready for implementation with zero findings; specs lint exited 0. This is resumed-run round1, following the prior run's three rounds and correction checkpoint recorded above, not a fourth round in that run. The unknown/foreign run sweep confirms uniform permission_denied/exit3; usage errors remain exit2.

The three child specs now move through the lifecycle script to todo; the epic remains unrefined until its first child starts. Counts remain core21/client7/host8, total36. Archived source hashes and all18 AC plus3 NFR clauses remain unchanged. Readiness covers the implementation plan only: ADR0021 remains proposed, runtime prerequisites remain unimplemented, and live host qualification remains a future acceptance gate. Product and orchestration approval ledgers are untouched.
