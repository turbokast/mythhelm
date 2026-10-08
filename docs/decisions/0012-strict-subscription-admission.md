# 0012. Strict subscription admission: the early return is removed and the consult fails closed

- Status: accepted
- Date: 2026-10-08

## Context

ADR-0002 item 1 made `--billing subscription-only` block unconditionally with
`entitlement_qualification_unavailable`, because nothing could qualify an
included-only execution boundary. The qualification registry (ADR-0011) now
holds per-route records with per-column verdicts and evidence, so strict
admission can be earned by evidence instead of refused outright. The
declared posture (`subscription-declared`) must stay a user declaration that
MYTHHELM never verifies (I15).

## Decision

The unconditional early return in `ResolveBilling` is removed. Strict
admission now consults the registry and fails closed:

- `ResolveBilling` admits `subscription-only` only on an eligible consult of a
  live-qualified record whose entitlement evidence and stop-at-exhaustion
  capability are unexpired at admission. It ignores any declaration. Every
  other case blocks with exit 3.
- The consult matches exactly on authentication category, provider endpoint,
  workspace class, effort settings and model snapshot. An absent record blocks
  (`no_qualification_record`), a fixture-only or unproven entitlement blocks
  (`entitlement_not_proven`), an unproven stop capability blocks
  (`stop_at_exhaustion_unproven`), several matching records block
  (`ambiguous_qualification_match`), and drift blocks
  (`qualification_drifted`) with a typed error that carries the record's key
  hash. Unknown remaining quota alone does not block.
- While the effective managed-policy inventory has an unresolved source,
  strict blocks with `entitlement_not_proven` and a `missing-source:` field.
- `subscription-declared` is unchanged: `Qualified:false`,
  `paid_continuation:unknown`, `g05:not-passed`.

This stales ADR-0002 item 1 only. Its items 2 to 8 (the declared posture,
identity binding, credential-route screening, trust and inventory rules)
still apply.

## Consequences

- Strict runs block until a maintainer commits a sanitized live-qualified
  record from an authorised live run; until then every strict run exits 3.
- Declared runs are unaffected and still say the entitlement is not verified.
- `docs/user-guide.md` (Billing) tells users which posture they run and what
  each proves.
- Tests that pin this behaviour live in `internal/admission`
  (`billing_test.go`, `qualify_test.go`).
