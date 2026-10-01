# ADR 0002 — Dogfood billing posture

- Status: proposed
- Date: 2026-10-01
- Scope: MH-1 / dogfood-slice Task 16; requirements FR-4, design §6.1–6.2.
- Maintainer decisions recorded in the approved scratchpad: 2026-09-29, Q1 and Q7.

## Context

A Claude subscription login does not establish that extra usage, purchased usage
credits or paid continuation are prevented. The native read-only status surface
reports authentication and subscription type; it does not qualify an included-only
execution boundary. I15 and G05 therefore remain unsatisfied for strict mode.

The maintainer chose a separate `subscription-declared` posture for personal
dogfooding. `--billing` remains required. A declaration says which plan class the
user selected and that they checked extra usage is disabled. It never certifies
that claim or the native service's enforcement.

## Decision

1. `subscription-only` always blocks with `entitlement_qualification_unavailable`
   (exit 3), including when the Claude execution integration is unavailable.
2. `subscription-declared` requires first-party subscription status and a current
   declaration of `plan=<pro|max|team|enterprise>,extra-usage=disabled`. Its record
   states `qualified:false`, `paid_continuation:unknown` and `g05:not-passed`.
   Summaries must say: **entitlement: user-declared, NOT verified by MYTHHELM;
   paid continuation: unknown (user declares disabled)**.
3. A declaration is bound to a SHA-256 of an unambiguous, length-prefixed pair of
   native `orgId` and `configDirectory`. A changed account or config root needs a
   fresh declaration. Replacement declarations supersede the previous row in
   the same journal transaction; the existing partial unique index prevents two
   current rows for one adapter/identity.
4. Native status accepts only the established subscription route and supported
   subscription types. It retains typed non-secret evidence and `identity_ref`.
   Email and organisation name are not decoded; raw organisation ID is used only
   to compute the hash. Raw config-directory text is excluded from durable JSON.
   Duplicate keys, including case-folded equivalents that encoding/json would
   match to one field, malformed types, excessive nesting and unrecognised
   evidence fail closed. Fold-aware rejection applies at all object levels as a
   fail-closed over-approximation, including case-sensitive map keys. Probes
   have bounded output and time, and never launch inference.
5. Credential-route environment overrides block with names only. The explicit
   strip option changes the child's environment and records removal names. Native
   settings have no strip option: helpers, conflicting login methods and credential
   env settings block, directing the user to fix their native setup.
6. The maintainer's AC-4.7 OAuth-token exception is a trusted **user-level** choice.
   A caller may pass the opaque complete environment entry to the native child
   only after that opt-in. Project configuration cannot grant it. Task 17 wires
   the user configuration and records `native-subscription-token` provenance;
   first-party subscription status is still required. No token enters the journal,
   receipt, settings manifest or diagnostics.
7. The settings inventory records digests and counts/names, excluding native
   commands, endpoints, environment values and account/session data. Every native
   settings or MCP source requires a digest-bound execution trust grant. Account
   fields in `.claude.json` are ignored by a decode struct containing only MCP
   fields; only the selected user/project MCP definitions affect its digest.
   Both lexical and canonical workspace entries are included when present.
8. Source symlinks are unsupported and refused before opening their targets. File
   identity is checked again before reading to refuse replacement between inspection
   and open. Unsupported `policyHelper` policy sources fail closed without execution.

## Compatibility and limits

The versioned native path is resolved before `--version`, hashed, and reused for
`auth status`; auth rejects a changed executable. The fixture floor is 2.1.284.
Later 2.1 patch versions are visibly untested. Other versions require explicit
experimental opt-in. Native Claude execution is unsupported on Windows.
Task 16 leaves Prepare/Start unavailable and execution capabilities unknown until
Task 17 supplies worker-owned launch and protocol fixtures.

The file inventory covers the design's user/project/local settings, project and
user/per-project MCP, managed settings, managed fragments and managed MCP sources.
Current official documentation also describes remote cached managed policy and
macOS MDM preferences. Their effective contents are not certified by this file
inventory. This source gap needs architect/maintainer resolution before the real
Task 20 run; a trust digest is not proof of complete effective native configuration.
See [managed settings documentation](https://code.claude.com/docs/en/managed-settings)
and [settings documentation](https://code.claude.com/docs/en/settings).

Scratchpad Q5 remains unknown. A read-only local probe observed native version
2.1.285. The attempted connect-only auth-status trace yielded no status JSON and
exit 1 under the sandbox; no offline claim follows from that failed trace. Treat
`auth status` as possibly networked. No inference probe or paid canary was run.

## Evidence

All 16 Task 16 acceptance checks passed against fakeclaude helper processes and
isolated homes. The PII check scans the real journal database/WAL, generated receipt
and structured log. Retained negative fixtures cover mismatched auth, changed
account/config identity, ambiguous JSON, settings credentials and missing trust.

Red-first mutation checks also proved: following a config symlink opens/accesses
native credential storage (Linux inotify); omitting a lexical MCP definition leaves
stale trust valid; skipping auth duplicate/depth validation accepts ambiguous status;
and copying raw organisation ID into status leaks it into the SQLite WAL and receipt.
Restoring the implementation passed those checks. No test needs real credentials,
network inference or an installed native CLI.

## Consequences

Personal dogfooding can proceed only with the explicit declared posture, after the
remaining native launch and delivery tasks and the recorded source-gap checkpoint.
There is no claim that MYTHHELM prevents paid continuation or passes G05. Native
authentication remains native; MYTHHELM does not edit account settings, extract
subscription secrets or substitute a provider/harness.
