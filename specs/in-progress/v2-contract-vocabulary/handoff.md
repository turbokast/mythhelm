# v2-contract-vocabulary — Hand-off

> One section per task. Each task replaces its own `<!-- pending -->` line in its pull request with what
> dependent tasks need: what it produced (the API and files as shipped), what a later task must
> know, and any deviation that changes a later task's inputs. `/run-spec` puts the sections of a
> task's dependencies into that task's dispatch prompt. Open questions and research stay in
> `scratchpad.md`. Keep each section within 20 lines (`runspec.py` truncates longer sections);
> prioritise the shipped API and deviations that change later inputs.

## Task 1 — Package kernel: scales, claims, strict decode, digest, limits

- **Produces**: package `internal/v2contract` as designed: `SchemaVersion`, `MaxFrameBytes/MaxNestingDepth/MaxArtifactRefs`, `CheckFrameLimits`, `GitObject`, `Budget`, `RevisionRef`, `RequestEnvelope`, `Capability/Progress/Eligibility` (consts `CapabilitySupported`, `ProgressFixtureTested`, `EligibilityEligible` etc.), `Claim[T]`, `Validator`, `Decode[T]`, `Digest`.
- **For dependents**: `loadGolden(t, rel)` in `codec_test.go` reads `testdata/<rel>` (package `v2contract_test`). `Digest` returns "" if the value cannot be encoded, so validate non-finite floats first. `Decode` errors name the unknown key, or the offset for syntax errors and truncation; trailing data is rejected.
- No deviations affecting later tasks.

## Task 2 — Execution records: Run, TaskRevision, Attempt

<!-- pending -->

## Task 3 — Decision and evidence records

<!-- pending -->

## Task 4 — Coordination records

- **Produces**: `v2contract.ContextManifest` (+ `type Handoff = ContextManifest`), `v2contract.Message` (+ open `DeliveryDisposition`), `v2contract.Grant` (+ `type EffectIntent = Grant`; `GrantStanding`/`GrantOneUse` consts for closed `Use`), `v2contract.Reservation` (open `Status`, `Quantity` `"unknown"`-or-value), `v2contract.PolicyVersion` (`Version >= 1`), `v2contract.Experiment` (finite `Splits`, open `Disposition`); all value-receiver `Validate`, design §3 exact fields/tags.
- **For dependents**: six goldens under `internal/v2contract/testdata/records/` (canonical single-line JSON + trailing newline; tests compare against `bytes.TrimSpace`). `Reservation` `"quantity": null` decodes to `""` then fails `Validate` naming `quantity` — Task 8's null-measurement fixture relies on this. Aliases decode via `Decode[Handoff]`/`Decode[EffectIntent]` with equal digests.
- **Deviations affecting later tasks**: none. `codec_test.go` lost Task 1's provisional `//nolint:unused` (comment-only; `loadGolden` signature unchanged).

## Task 5 — Machine-checkable lifecycles

<!-- pending -->

## Task 6 — Error catalogue and adapter mapping

<!-- pending -->

## Task 7 — Event v2 envelope and sequence validators

<!-- pending -->

## Task 8 — Support matrix, hermeticity, and invalid-case evidence

<!-- pending -->
