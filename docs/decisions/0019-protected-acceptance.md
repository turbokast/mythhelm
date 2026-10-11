# 0019. Protected acceptance: bound verdicts and immutable policies

- Status: proposed
- Date: 2026-10-11

## Context

A run's verification verdict is today a row in `verifications` that names the
candidate commit and the checks that ran, and nothing more: it records no
reason for a `NOT RUN` verdict, no canonical attestation of what was judged,
and the apply gate never reads it (it tests the run reason). The canonical
v2 `Verification` attestation and `PolicyVersion` records exist in
`internal/v2contract` with no producer, and the `policies` table created by
schema 7 can be rewritten by any statement.

Protected acceptance (spec `specs/*/protected-acceptance/`, master spec v2
§§4.1, 11.2, 11.3) binds a verdict to the candidate, the acceptance contract,
the check definitions and the environment it judged, and makes the P1 policy
a stored, immutable record. These decisions change persistence and public
contracts (the receipt and the JSONL stream), so they are recorded here. The
spec's design §4 keeps each decision ID below. Task 2 of the spec ships the
persistence decisions (D14, and the storage shape D1 and D2 name); later tasks
ship the rest and Task 12 completes the consequences.

## Decision

**D1: adopt the v2 records verbatim.** The attestation is
`v2contract.Verification` and P1 is `v2contract.PolicyVersion`, each stored as
canonical JSON beside its `v2contract.Digest`. Nothing is extended: a bespoke
binding struct would be a second attestation shape.

**D2: no `tasks` or `task_revisions` row for a new run.** The contract
revision is recorded as `{task_id, revision 1, contract digest}` in the pin
and the attestation. `migrate --apply` and `checkImportMarker` treat any
`task_revisions` row as "already imported", so a new run's row would make a
later migration skip it silently. Rejected: writing the rows and changing the
import marker logic, which reopens the finalized supervisor migration. The
lifecycle migration owns the rows for new runs.

**D5: unavailable checks end `ready_for_review` / `unverified`.** The verdict
is labelled `NOT RUN` with a reason, not `failed` and not v2 §11.2's `blocked`.
The unverified representation, its exit code and `--accept-unverified` already
exist, and the v1 machine's `blocked` is terminal and not appliable, so it
would strand the candidate.

**D7: the apply gate decides on the binding, not on the run reason.** A
verified run recorded before schema 8 has no binding, is `unbound`, and needs
`--accept-unverified`. Missing evidence is never verified (I09). Rejected:
grandfathering old verified runs, which would apply an unbound verdict.

**D8: extend the v1 receipt additively.** One new `acceptance` key and one
`unknown_reasons` key; `schema_version` is unchanged. `internal/v2contract`
defines no receipt type.

**D10: document the journal event types beyond the 23 and refuse the rest.**
`attempt.native_error`, `recovery.ownership_resolved`,
`attempt.recovery_decided`, `run.extension_granted`, `migration.imported` and
`migration.quarantined` are real journal facts a consumer needs. The JSONL
renderer refuses any other undocumented type, so a future undocumented event
is a loud defect rather than silent contract drift. Filtering would hide
recovery evidence.

**D14: schema 8 adds one side table and triggers.** Migration
`internal/journal/migrations/0008_acceptance.sql` creates
`verification_bindings` (reason, attestation, attestation digest, overhead;
`attestation` and `digest` are both present or both absent) and update,
delete and replace-abort triggers on it and on `policies` (I20). `INSERT OR
REPLACE` fires no delete trigger unless `recursive_triggers` is set, so a
BEFORE INSERT trigger closes it. No earlier table is altered (ADR 0016); a
verification without a binding row was recorded before schema 8 and reads
`Bound == false`. `InsertVerification` always writes the binding row, so the
absence means only that. The migration also advances
`migration_state.schema_version` to 8, so the identity that table records
matches `PRAGMA user_version`. `journal.EnsurePolicy` reads first and inserts
only when the row is absent: an identical digest is a no-op and a different
digest returns `journal.ErrPolicyConflict`, because the triggers abort any
insert over an existing key.

**D18: a check-runner error other than cancellation is `unavailable`.** A
start failure, an unsafe managed directory or an evidence write failure ends
`unverified` with `checks_unavailable`, not `failed`: the fault is in the
evaluator, not the candidate, and the candidate stays reviewable. A
cancellation stays `failed` because the user stopped the run. A failure the
partial result already holds is kept.

**D19: `conclude` reads the pin from the journal, never from memory.** A
recovering supervisor has no in-memory pin, and a pin rebuilt at recovery
would pin the evaluator after the candidate exists. A pin-less admission skips
the checks as `NOT RUN` / `pin_missing`.

**D20: an apply over a non-bound status records `acceptance.apply_override`.**
The version-1 receipt says `passed` for a run whose verdict no longer applies;
the override key in the version-2 receipt is the only truthful record of that.

**D21: recovery never re-runs the checks.** A run left `verifying` with no
verification row gets a verdict recorded from its journaled `check.completed`
events (`record_unavailable` unless one failed) only when the pin names the
admitted checks and every one has an event; otherwise it keeps `ErrOwnership`.
A check runs in its own process group with no parent-death signal, so moving
the run out of the active states early could admit a new run beside a
still-writing check (I05, I24).

**D23: an `invalid` verdict ends `failed` / `verification_invalid`.** An
evaluator digest that is missing or differs from the pin means the evaluator
is not the one pinned (G07); offering `--accept-unverified` would offer a
candidate judged by an evaluator nobody pinned. The candidate stays frozen and
reviewable.

## Consequences

- Schema 8 is additive. Every mirror and version pin moves with
  `journal.SchemaVersion`: `internal/migrate/backup.go`,
  `internal/migrate/preview/plan.go` and its `migrationFiles`, and the tests
  that pin versions, which now derive the backup name from
  `journal.SchemaVersion` so the next bump cannot leave a stale pin.
- Pinned by `TestMigration0008AdditiveAndV7DataSurvives`,
  `TestVerificationBindingRoundTrip`, `TestBindingsAndPoliciesImmutable`,
  `TestEnsurePolicyIdempotentAndConflict` and `TestBackupPinsMatchJournal`.
- Policies and bindings can no longer be edited by any statement, including
  a repair script: a correction is a new policy version or a new verification.
- The apply gate, the receipt and the stream changes (D5, D7, D8, D10, D20,
  D23) land in later tasks of the spec; Task 12 completes this record's
  consequences and moves its status to accepted.
