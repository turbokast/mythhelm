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

<!-- pending -->

## Task 7 — Truthful receipt

<!-- pending -->

## Task 8 — Documented JSONL event contract

- **Produces**: `supervisor.StreamEventTypes() []string` (sorted copy, 29), `supervisor.IsStreamEvent(typ) bool` in `internal/supervisor/events.go`, and `cli.UndocumentedEventError{Type}` in `internal/cli/render.go`; no difference from the design.
- **For dependents**: a new event type emitted by any journal writer must be added to `StreamEventTypes` and to the literal list in `TestStreamEventSetPinned`, or `TestEveryEmittedEventTypeIsDocumented` (AST scan of `internal/supervisor`, `internal/workers`, `internal/control`, `internal/migrate`) and every JSONL run fail. Tasks 6 and 7 add no event types, so they need nothing.
- **For dependents**: the scan collects only a string literal at the event-type position of `newEvent` (4th), a method `append` (2nd) and `applyEvent` (5th), and constants or variables named `Event*`/`ev*` (upper-case next) or `*Event`/`*EventType`. A type built by an expression is not collected; the renderer's refusal is the backstop.
- **For Task 10**: the JSONL renderer now refuses any non-documented type, so a decoder or stream consumer sees only the 29 types plus `run.result`; `finish` makes the process exit 1 (`undocumented_event`) when nothing else failed.
- **Deviations**: none.

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
