# protected-acceptance — Hand-off

> One section per task. Each task replaces its own `<!-- pending -->` line in its pull request with what
> dependent tasks need: what it produced (the API and files as shipped), what a later task must
> know, and any deviation that changes a later task's inputs. `/run-spec` puts the sections of a
> task's dependencies into that task's dispatch prompt. Open questions and research stay in
> `scratchpad.md`.

## Task 1 — Acceptance vocabulary package

- **Produces**: `internal/acceptance` with the exact API of design §2.1 (PR #398); no difference from the design's `Produces` line.
- **Reasons**: `Decide` and `DecideStatuses` return `checks_failed` for `failed`, `checks_unavailable` or `no_checks_ran` for `unavailable`, `evaluator_pin_missing` or `evaluator_digest_mismatch` for `invalid`, and an empty reason for `passed`.
- **Binding.Diff**: dimension names are `candidate`, `contract`, `check_definition`, `evaluator`, `environment`; use them verbatim in gate and review messages.
- **Digests**: `NewPin(taskID, bareHex)` and `WithChecks(bareHex...)` take bare hex and store `sha256:` forms; `Pin.PolicyDigest` is already prefixed (`v2contract.Digest`). `Attest` stores the slices and maps it is given.
- **Immutability**: `TestP1DigestPinned` pins the P1 digest; a change to P1 is a new `PolicyVersion`, never an edit.
- **Unverified**: the journaled payload value `NOT RUN` labels `NOT RUN` but `Unverified()` is false for it; convert with `ResultNotRun` where the predicate is needed.

## Task 2 — Schema 8: verification bindings and immutable policies

- **Produces**: `journal.SchemaVersion = 8`; migration `internal/journal/migrations/0008_acceptance.sql`; `journal.VerificationRow{…, Bound bool, Reason string, Attestation []byte, AttestationDigest string, OverheadMS *int64}` exactly as designed; `journal.EnsurePolicy(ctx, tx, v2contract.PolicyVersion) error` and `journal.ErrPolicyConflict`. ADR is `docs/decisions/0019-protected-acceptance.md` (status proposed; Task 12 completes it).
- **For dependents**: `InsertVerification` writes the binding row for every verification, so set `Reason`, `Attestation` (canonical `v2contract.Verification` JSON), `AttestationDigest` and `OverheadMS` on the row you insert; an attestation without a digest (or the reverse), or non-JSON attestation, fails the CHECK and the whole transaction. `Bound == false` appears only for a verification recorded before schema 8: the apply gate treats it as `unbound` (D7).
- **For dependents**: bindings and `policies` rows cannot be updated, deleted or replaced by any statement; a second `EnsurePolicy` with an identical record is a no-op and one with another digest returns `ErrPolicyConflict`. `EnsurePolicy` calls `PolicyVersion.Validate`, so a record needs `SchemaVersion` 2, a policy id, a version of at least 1 and an action family. Task 4 calls it in the admission transaction.
- **For dependents**: `migration_state.schema_version` now reads 8 on a new or upgraded database. A new schema bump must sweep the same mirrors (`internal/migrate/backup.go`, `internal/migrate/preview/plan.go` and `migrationFiles`) and the version pins; backup-name pins derive from `journal.SchemaVersion`, and a schema-7 preview test lives in `internal/cli/migrate_test.go`.
- **Deviations**: none that change a later task's inputs; `internal/migrate/SUPPORT.md` and `docs/user-guide.md` carry the version edits.

## Task 3 — Evaluator seam and three-state check result

<!-- pending -->

## Task 4 — Pin the acceptance policy at admission

<!-- pending -->

## Task 5 — Bound verdicts, unverified outcomes and recovery

<!-- pending -->

## Task 6 — Apply gate on the binding

<!-- pending -->

## Task 7 — Truthful receipt

<!-- pending -->

## Task 8 — Documented JSONL event contract

<!-- pending -->

## Task 9 — TUI surfaces

<!-- pending -->

## Task 10 — CLI surfaces: review, run stream, accessible

<!-- pending -->

## Task 11 — Credential-free end-to-end flow

<!-- pending -->

## Task 12 — User documentation and decision record

<!-- pending -->

## Task 13 — Qualified-route end-to-end (blocked-on MH-12 Task 5)

<!-- pending -->

## Task 14 — Live qualified-route evidence (maintainer; blocked-on MH-12 Task 7)

<!-- pending -->
