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

<!-- pending -->

## Task 3 — Evaluator seam and three-state check result

- **Produces**: `integration.EvaluatorFor(opts RunOptions, checks []admission.CheckConfig) (contain.Evaluator, error)`, `integration.MaxEvidenceBytes = 1 << 20` and `Verification.Result` in `{passed, failed, unavailable}`; no difference from the design (PR #405).
- **For Task 4**: `EvaluatorFor` forces `ReadOnly = true` and no auth binds itself, so pass the caller's `RunOptions` unmodified (`Policy: &contain.Policy{Profile: "restricted"}` for a contained run). It needs no Workdir, HOME or Linux; the platform and HOME refusals stay in `RunChecksWithPolicy`, so a pin can be computed anywhere.
- **For Task 5**: `Result` is the raw check summary, not the verdict: `Decide` still adds the evaluator comparison. A zero-check list gives `passed` here (as before); `Decide` and `DecideStatuses` map it to `unavailable`/`no_checks_ran`.
- **Pipeline**: `pipeline.go:1213` still ends any non-`passed` result `failed`, with `verification_unavailable` when a check was unavailable; Task 5 replaces that with the verdict table.

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
