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
- **For dependents**: `InsertVerification` writes the binding row for every verification, so set `Reason`, `Attestation` (canonical `v2contract.Verification` JSON), `AttestationDigest` and `OverheadMS` on the row you insert; `InsertVerification` refuses, and rolls the whole transaction back for, an attestation that is not the `json.Marshal` encoding of a valid `v2contract.Verification` whose `VerificationID` is the row's ID and whose `Candidate.Value` is the row's `CandidateCommit`, with `AttestationDigest` its `v2contract.Digest`, and a digest with no attestation (the schema CHECK). `Bound == false` appears only for a verification recorded before schema 8: the apply gate treats it as `unbound` (D7).
- **For dependents**: bindings and `policies` rows cannot be updated, deleted or replaced by any statement; a second `EnsurePolicy` with an identical record is a no-op and one with another digest returns `ErrPolicyConflict`. `EnsurePolicy` calls `PolicyVersion.Validate`, so a record needs `SchemaVersion` 2, a policy id, a version of at least 1 and an action family. Task 4 calls it in the admission transaction.
- **For dependents**: `migration_state.schema_version` now reads 8 on a new or upgraded database. A new schema bump must sweep the same mirrors (`internal/migrate/backup.go`, `internal/migrate/preview/plan.go` and `migrationFiles`) and the version pins; backup-name pins derive from `journal.SchemaVersion`, and a schema-7 preview test lives in `internal/cli/migrate_test.go`.
- **Deviations**: none that change a later task's inputs; `internal/migrate/SUPPORT.md` and `docs/user-guide.md` carry the version edits.

## Task 3 — Evaluator seam and three-state check result

- **Produces**: `integration.EvaluatorFor(opts RunOptions, checks []admission.CheckConfig) (contain.Evaluator, error)`, `integration.MaxEvidenceBytes = 1 << 20` and `Verification.Result` in `{passed, failed, unavailable}`; no difference from the design (PR #405).
- **For Task 4**: `EvaluatorFor` forces `ReadOnly = true` and no auth binds itself, so pass the caller's `RunOptions` unmodified (`Policy: &contain.Policy{Profile: "restricted"}` for a contained run). It needs no Workdir, HOME or Linux; the platform and HOME refusals stay in `RunChecksWithPolicy`, so a pin can be computed anywhere.
- **For Task 5**: `Result` is the raw check summary, not the verdict: `Decide` still adds the evaluator comparison. A zero-check list gives `passed` here (as before); `Decide` and `DecideStatuses` map it to `unavailable`/`no_checks_ran`.
- **Pipeline**: `pipeline.go:1213` still ends any non-`passed` result `failed`, with `verification_unavailable` when a check was unavailable; Task 5 replaces that with the verdict table.

## Task 4 — Pin the acceptance policy at admission

- **Produces**: in `internal/supervisor/acceptance.go`: `AdmissionPayload{admission.Record; AcceptancePin acceptance.Pin}` (JSON key `acceptance_pin`), `NewAdmissionPayload(d)`, `ReadPin(ctx, j, runID) (acceptance.Pin, bool, error)` and the unexported `checkOptions(d admission.Decision) integration.RunOptions`; no difference from the design (PR #413).
- **For Task 5**: `ReadPin` returns `(zero, false, nil)` for an admission without the key (or `null`) and for a run with no `admission.decided`; only a read or decode failure is an error. Map that error to `failed`/`acceptance_pin_unreadable`, never to `pin_missing`. It reads the last `admission.decided` of the run.
- **For Task 5**: `conclude`'s run path already calls `checkOptions(p.d)`; a recovery decision rebuilt by `recoveryDecision` carries the journaled `Profile`, so `checkOptions` gives the admitted options there too. The pin's `EvaluatorDigest` and `CheckDefinition` are `sha256:` forms (compare with `acceptance.SHA256Ref(v.Evaluator.Digest)`); a pin from `--no-checks` or an inspect run has them empty, and `CheckTimeouts` nil.
- **For Task 5**: `seedLaunch` (`faults_test.go`) journals `NewAdmissionPayload(d)`; its decision is trusted-host, so a pinned fixture has evaluator name `host`. A pin-less fixture journals `admission.Record` only (`json.Marshal(d.Record())`).
- **For dependents**: the P1 policy row exists from admission on (`journal.EnsurePolicy` in the admission transaction), so a verification binding never has to seed it. A conflicting row for `P1`/1 blocks the run `acceptance_pin_failed` (exit 3) before any attempt.
- **Test seam**: the restricted-profile and inspect legs need a Linux boundary and skip where it is denied; `TestPinEvaluatorFollowsProfile` and `TestPinLeavesChecksUnknownWhenNoneRun` cover the same branches through `NewAdmissionPayload` on any host. A contained check in a run test must not use the test binary (the boundary hides its temp directory): use `fixture.shellConfig` (`/bin/sh -c true`).
- **Deviations**: none that change a later task's inputs.

## Task 5 — Bound verdicts, unverified outcomes and recovery

- **Produces**: `verifyOutcome(err, result) (RunState, string)`, `Hooks.Timeouts *acceptance.Timeouts` and `Hooks.BindStep`, and the `verification.completed` keys `reason` and `attestation_digest` (both omitted when empty; `result` is the verdict `passed|failed|unavailable|invalid`, or `NOT RUN` on a skip path). No difference from the design's `Produces` line.
- **For Task 6**: every verification a schema-8 build records has a binding row (`Bound`); `Reason` is `checks_failed`, `checks_unavailable`, `no_checks_ran`, `binding_unavailable`, `record_unavailable`, `evaluator_digest_mismatch`, `waived by --no-checks`, `checks_refused_under_inspect` or `pin_missing`. Only a verdict that ran the checks to completion and bound successfully carries an attestation, including `failed`, `unavailable` and `invalid` ones; a bind fault, a runner fault, a skip and a `record_unavailable` recovery row carry none, so `CheckAcceptance` must read the result and reason first and the attestation only for `passed`.
- **For Task 6 and Task 7**: unavailable checks are `ready_for_review`/`unverified` (the row is `unavailable`, not `not_run`); `invalid` is `failed`/`verification_invalid` and `acceptance_pin_unreadable` is `failed` with no verification row at all, so the receipt and review must render a failed run with no verification as `unknown` with a reason.
- **For Task 7**: `OverheadMS` is set for every executed verification (the bind step, even when it failed) and nil on a skip path or a recovery row; the row's evaluator is the one that ran (the pin's can differ for `invalid`).
- **For Task 8**: `verification.completed` payloads now carry `result` values `unavailable` and `invalid`, which the plain stream prints as the raw word until Task 10 uses `Result.Label()`; `render.go` was not touched.
- **Recovery**: a `verifying` run with a row applies `verifyOutcome`; with no row it records a verdict only from a complete `check.completed` set (one per name in the pin's `check_timeouts`) and otherwise stays `ErrOwnership` (exit 6). The unreadable-pin failure is not reached from this branch (it stays ownership-unresolved).
- **Test seams**: `seedLaunchWith(t, f, scenario, payloadFunc, trustConfig...)` journals any admission payload (`pinnedPayload`, `pinlessPayload`, `wrongEvaluatorPayload`, `corruptPinPayload`); `fixture.runInProcess(t, digest, hooks)` supervises a checked run in process for `Hooks`; `commitChecks`, `checkBlock`, `testCheck`, `markerCheck` and `missingCheck` build check configurations.
- **Deviations**: none that change a later task's inputs.

## Task 6 — Apply gate on the binding

- **Produces**: in `internal/supervisor/acceptance.go`: `AcceptanceState` (`AcceptanceBound`, `AcceptanceUnverified`, `AcceptanceStale`, `AcceptanceUnbound`, `AcceptanceRejected`), `AcceptanceStatus{State, Reason, Changed}`, `CheckAcceptance(ctx, j, runID)` and `CheckAcceptanceWithin(ctx, j, runID, limit)`; no difference from design §2.5. `ApplyRun` keeps its signature.
- **For Task 7**: the version 2 receipt gets `acceptance.apply_override = {state, reason, changed}` from `ApplyRun`, which merges it into the receipt's existing `acceptance` map (created when absent, so Task 7's map keeps its keys) and sets `changed` to `[]` when there is none. A bound apply, and a resumed apply, writes no such key. `BuildReceipt` must not drop or pre-fill it.
- **For Task 10**: `review` reads the same status from `CheckAcceptance`; `Reason` is the row reason (`checks_unavailable`, `waived by --no-checks`, ...) for `unverified` and `rejected`, `no verification recorded` when there is no row, `no reason recorded` for an old row without one, and a fixed sentence for `stale` and `unbound`. `Changed` uses the `Binding.Diff` names.
- **Behaviour**: a passed row with no attestation, an attestation that does not decode or match its stored digest, a candidate commit that is not a git object id, an unreadable task file and an unrecognised result on a bound row are all errors, not a status; `apply` returns them unwrapped by `ErrApplyBlocked` (exit 1). An unbound row is checked after unverified and rejected, so an old `not_run` row is `unverified`.
- **Resumed apply**: with a journaled intent `ApplyRun` skips the gate and records no override; an override passed to a first call that crashed after the intent is not in the receipt of the resumed call. The journaled intent does not carry it (follow-up if the receipt must show it).
- **Test seams**: `fixture.addVerification(t, base, verificationVariant{result, unbound, attest})` appends a newer, bound or raw-inserted unbound verification copied from `base` (`acceptance_test.go`); `fixture.createBareRun` journals a run with no attempt or verification; `staleCases` in `apply_test.go` are three tamperings plus a deleted task file.
- **Deviations**: none that change a later task's inputs.

## Task 7 — Truthful receipt

- **Produces**: `admission.EntitlementStatement(adapter.BillingPosture) string`; `supervisor.BuildReceiptWithin(ctx, j, runID, limit)` (`BuildReceipt` calls it with `DefaultTimeouts().Render`); unexported `buildReceipt(ctx, j, runID, reasons)`, `unknownReasons()` and `attachUnknownReasons(r, reasons) []string`; receipt keys `billing.entitlement_statement`, `verification.{result,label,reason,binding}`, top-level `acceptance` and `unknown_reasons`. No difference from the design's `Produces` line.
- **For Task 6**: `acceptance` is now a top-level object (`policy_pin`, `determinism`, `overhead_ms`, `overhead_covers`, `bounds`) in every receipt, including the version-1 receipt `ApplyRun` rewrites: set `acceptance.apply_override` inside that existing map, never replace it. A receipt built by `BuildReceipt` has `unknown_reasons` as `map[string]string`; one read back from `receipt.json` has `map[string]any`.
- **For Task 8, 9, 10**: `verification.label` is `Result.Label()` of the stored row result (`passed`, `failed`, `NOT RUN`, `INVALID`, `unknown`) and `verification.reason` is the row's `Reason` (empty for a passed verdict, `unknown` with an `unknown_reasons` entry when none was recorded). `billing.entitlement_statement` is the sentence the plain stream, review and TUI print; use `admission.EntitlementStatement(record.Billing)` for a live event.
- **For Task 10**: `verification.binding` is `{candidate: {format, value}, contract, check_definition, evaluator, environment, attestation_digest}` or `unknown`; every digest in it is the `sha256:` form, while the existing `verification.evaluator.digest` keeps its bare hex.
- **For Task 12**: the receipt's `acceptance.bounds` flags `execution_seconds` enforced and `repairs`, `replans` and `transport_retries` not enforced, with `check_timeouts` from the pin and `evidence_bytes_per_check`; the task that closes #328 rewrites `TestReceiptBoundsEnforcedFlags` and the `bounds` function's three `false` flags.
- **For dependents**: a new `unknown` leaf needs an entry in `unknownReasons()` (`unknown_reasons.go`), keyed by its normalised path (`[]` for array indices); `TestEveryUnknownHasAReason` fails on an unexplained leaf and on an entry no fixture reaches. A native-supplied subtree (`billing.tokens.by_model`, `native_configuration.mcp_servers`) is opaque to the walk.
- **Traps**: an unreadable pin renders `acceptance.policy_pin` `unknown` (reason `acceptance pin unreadable`) and does not fail the receipt, so `settleRun`, recovery's repair and `review` still work for a `failed`/`acceptance_pin_unreadable` run. A verification row with no binding row (recorded before schema 8) renders `binding`, `overhead_ms` and `policy_pin` unknown with `recorded before acceptance binding existed`.
- **Test seams**: `unknown_reasons_internal_test.go` exports `AttachUnknownReasons` and `UnknownReasons` to the external tests; `receiptWorldTo`, `verifiedWorld`, `journalEvent` and `unknownFixtures` in `receipt_test.go` build journaled runs; `passedRun` and `waivedRun` capture a real run once per test binary (a real run costs seconds, and the supervisor test binary is near the 10 min package limit on windows-latest: `TestReplanPreservesRepairSlots` also fails once the binary has run about 9m45s, because `reserveStart` is its start time).
- **Deviations**: none that change a later task's inputs.

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
