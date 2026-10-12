# codex-native-adapter — Retrospective

## Review Summary

- **Range**: 8caedd4..6249f0f (PRs #352, #353, #373, #375, #376, #377, #388, #390, #396, #397, #402, #403; fix PRs none)
- **Reviewer**: code-reviewer, architect (dispatched=2 returned=2 failed=0)
- **Findings**: critical 0, important 7, suggestion 11 (confirmed); rejected 0
- **Open critical**: 0
- **Vendor review**: skipped (codex, muse and jev disabled in `vendors.py status`)

Neither reviewer found a way to admit or launch a Codex run: unknown auth category, the gap gate, the purchased-credit rule and the required `structured_events` capability each block independently, and the seed row stays `planned`. Every important finding is latent while that block holds and must be settled before a later spec gives Codex an auth-category source. The findings are adjudicated `auto-confirmed (non-interactive)`; each was opened at its anchor on 6249f0f.

| Severity | Finding | Anchor | Disposition |
|---|---|---|---|
| important | Credential screen skipped for Codex: `OPENAI_*` is dropped silently while `--strip-credential-env` is refused (I01, ADR-0002 item 5) | `internal/admission/admission.go:350` | issue #408 item 1 |
| important | `ProviderEndpoint` and `EntitlementClass` are hard-coded `unknown` and match a record keyed `unknown`; only `AuthCategory` is gated (I02, I09, I15, AC-7.5) | `adapters/codex/qualify.go:69` | issue #408 item 2 |
| important | The version probe runs in the source checkout after trust was decided on committed files (AC-3.1, AT-11, I03) | `internal/admission/admission.go:363` | issue #408 item 3 |
| important | A symlinked or unreadable user config source exits 1 or 7 where Claude exits 3 `native_config_unreadable` (v2 §15.10) | `adapters/codex/config.go:248` | issue #408 item 4 |
| important | A dropped `CODEX_HOME` is not recorded as a configuration difference (AC-3.2, I01); the maintainer question lived only in `handoff.md` | `adapters/codex/launch.go:79` | issue #408 item 5, comment on #389 (F2 second half) |
| important | ADR 0018 (proposed) disagrees with the tree: "one table entry" claim, preconditions list, D18 rationale, limitations wording (I14) | `docs/decisions/0018-codex-adapter-admission.md:110` | issue #408 item 6; the maintainer accepts the ADR, so it is not edited here |
| important | Claude probe output bound is unenforced: the embedded `bytes.Buffer` promotes `ReadFrom` (I21); found in Task 3, recorded only in the scratchpad | `adapters/claudecode/probe.go:206` | issue #409 |
| suggestion | Adapter-ID branching in core, Windows gate placement, parent-env `AuthCategory`, ADR-0002 in block messages, `dataDestinations` wording, `FixtureDraft` card name, three home helpers, AC-3.4 guard scope, TOML sources missing from design D11, stale "both adapters" in a todo spec; Claude empty-manifest re-inventory gap | see issue | issue #408 suggestions; empty-manifest gap issue #410 |

Open follow-ups already filed in #389 are not repeated: A3, A4, F3 to F5, Q1 to Q5 and the two stop-precedence and convergence questions. F1 is settled by #397 and A5 by ADR 0018 D18 and design D18.

Foreign changes in range: 795afdc docs/user-guide.md; c38f20d internal/admission/qualify_test.go, internal/cli/run_drift_test.go; 7fd0fcc internal/supervisor/pipeline.go, internal/supervisor/pipeline_test.go; 0f17181 docs/limitations.md.

## Acceptance

| Criterion | Result | Evidence |
|---|---|---|
| AC-1.1 to AC-1.5 (decider table, worker starts `builtin/codex`, Claude and fake unchanged, tool-rule refusal, seam additions) | met | `TestDecideDispatchesByTable`, `TestWorkerStartsCodexByDescriptor`, `TestPrepareRefusesToolRulesAndPassthrough`; the AC-1.5 `ProtocolMismatch` addition is design D18 |
| AC-2.1 to AC-2.5 (probe, untested-version block, unknown capabilities, hash re-check, no seed row) | met | `TestProbeReturnsIdentityWithoutModelTask`, `TestCompatibilityUntestedBlocksWithoutOverride`, `TestCapabilitiesUnknownNeverDefaultSupported`, `TestPrepareRefusesChangedHash` |
| AC-3.1 (inventory and trust before any native process) | partial | `TestCodexUntrustedConfigBlocksBeforeProbe`; the probe runs in an uninventoried directory (#408 item 3) |
| AC-3.2 (every difference a fidelity delta) | partial | `TestPrepareRecordsFidelityDeltas`; a dropped `CODEX_HOME` is not recorded (#408 item 5, #389 F3) |
| AC-3.3 (config drift invalidates evidence) | partial | key level: `TestInventoryDigestsChangeWithSource`; through `Decide` the gap gate blocks first, recorded in the honesty register |
| AC-3.4, AC-3.5 (no credential reads, gaps block) | met | `TestInventoryOpensOnlyEnumeratedFiles`, `TestGapsBlockStrict` |
| AC-4.1, AC-4.3 to AC-4.7 (strict blocks, purchased credit, usage nil, errors, auth key) | met | `TestCodexStrictKeyLevel`, `TestPurchasedCreditPreventableNeedsUnexpiredPreventableEvidence`, `TestDecodeErrorsAreNativeError`, `TestCodexKeyVariationBlocks` |
| AC-4.2 (closed auxiliary route list blocks unknown routes) | partial | `TestRouteInventoryClosedAndUnknownBlocks`; nothing in production calls `RouteInventory` (D4, honesty register) |
| AC-5.1 to AC-5.4 (ladder, acknowledged unknown, one writer, no replay) | met | `TestStopLadderEscalates`, `TestStopLadderValues`, `TestSecondStartOnSameAdapterRefused`, `TestAdapterHasNoReplayPath` |
| AC-6.1, AC-6.2 (session identity, capabilities, no private-store writes) | met | `TestDecodeSuccessFixture`, `TestDecodeMissingSessionIDIsUnknown`, `TestNoNativeStoreWrites` |
| AC-7.1 to AC-7.5 (independent columns, fixture evidence only, doctor row, key parity, key variation blocks) | met | `TestFixtureDraftColumnsIndependent`, `TestFixtureDraftCannotProve`, `TestDoctorPlainShowsFailingAndNextTest`, `TestResolveQualificationKeyParity`, `TestCodexKeyVariationBlocks` |
| NFR-1 to NFR-3, N7 | met | offline fixture natives, `TestFixturesCarryNoCredentialStrings`, `docs/limitations.md` Codex section, `TestRestrictedAndInspectRefuseCodex` |

The three partial rows are the honest outcome of OQ1, OQ6, OQ9 and OQ10 staying at their defaults: no Codex executable was available, so the adapter is fixture-tested only, the registry row stays `planned`, and every Codex run is blocked.

## Deviations

- Task 1: `adapters/fake/fake_test.go` edited outside Files because `checkTri` rejected the unset `omitempty` entries — recorded in the task entry; no behaviour change.
- Task 2: `internal/cli/run_drift_test.go` `strictRun` isolates `HOME`; the `--adapter fake` flag error text is derived from the table; `dataDestinations` reads a table field — all recorded; impact none on Claude or fake output.
- Task 3: the `os/exec` import exemption in `adapters/fake/fake_test.go` now names `codex/probe.go` — recorded.
- Tasks 4 and 5: branches named by the session, not `feat/...-tN`; Task 5 added probe fields outside Files — recorded.
- Task 6: the inventory also detects `config.toml` sources (`user_config_toml`, `project_config_toml`) that block when present — not in the task's Spec deviations (it says None) and not in design D11; found by review (#408 suggestions). Fails closed.
- Task 8: `billing_unsupported_for_adapter` is a policy block (exit 3); `codexAdapterID` replaced by `codex.AdapterID` — recorded.
- Task 9: the worker-initiated `protocol_mismatch` stop (D18) settles #389 F1 and extends the seam beyond AC-1.5's list; Codex re-inventories on an empty manifest (A2/A6) — recorded; impact: Claude behaviour unchanged, empty-manifest gap for Claude filed as #410.
- Task 10: `docs/user-guide.md` updated outside Files because the doctor example is pinned to the `qualLine` format — recorded.
- Task 12: design.md amended for D18 and the ADR records D18 beside D1, D3, D10 and D12 — recorded.

## CI history

- CI on PR #352 head 20b18594: real, Go job failed `TestCapabilityRecordUnknownNotOptimistic` (`adapters/fake`); fixed in the same PR by `checkTri` accepting an empty `omitempty` entry (Task 1 deviation). Deterministic.
- CI on PR branches for all 12 tasks: `cancelled` runs (PR title, OSV-Scanner, zizmor, CI) on superseded heads. Mechanism: `.github/workflows/ci.yml` sets `cancel-in-progress` for `pull_request` events, so a push cancels the previous head's run. Not failures.
- CI on merge 6103564 (PR #375) and 56968dc (PR #377): `finalize.py` says infra, but runs 38137813579 and 38138885062 were `cancelled` with zero jobs, so nothing infrastructural is shown. Likely mechanism, not proven: the main-branch concurrency group keeps one pending run and the pushes of 7fd0fcc (run created 12:10:38Z) and 54b7b65 (12:21:06Z) replaced them; each cancelled run was updated one second after the replacing run was created (12:10:39Z, 12:21:07Z), and a manual cancel cannot be excluded. Neither commit was tested on its own; the later runs on c38f20d and 31cb991 passed, which tests those trees, not these commits. Verdict for both commits: `unknown`, covered by the green tip at 6249f0f (P-codex-native-adapter-4).
- CI on merge 8f2e8f7 (PR #396): real, `Go (windows-11-arm)` `Run go test ./...`, `TestWatchWritesSupervisorBeatAfterIngest` failed 0.17 s in with a sharing violation opening `supervisor.beat`. Mechanism: the beat writer renames a temp file over the target (`internal/control/reconnect.go:61`), a Windows open that races the rename returns a sharing violation, and the test's poll tolerates only `ErrNotExist` (derived from the code, not reproduced). Failing value: a collision on the poll at 0.17 s; passing value: no collision on the passing runs of 0afb230, 6aa6f84 and 6249f0f. The test is from supervised-stop-recover (#311), not this spec; filed as #411.
- Run-events log, 2026-10-10: PR #352 paused on proxy.golang.org `INTERNAL_ERROR` in the Lint workflows (class infra, recorded by the run).
- Finalize PR #412 head 7fc63a1: real, `Docs checks` failed `Check root-guide mirrors are fresh` because `docs/mirror/CHANGELOG.md` was not refreshed with the changelog entries; deterministic, fixed by the copy (P-codex-native-adapter-5).
- Main at 6249f0f: `finalize.py ci` verdict=green, 7 runs.

## Effort

dispatched=3 returned=2 failed=0 unaccounted=task 3 (1 dispatch) per `runspec.py summary`: the Task 3 dispatch row (2026-10-11T07:20:06Z, stacked on PR #352) has no `return`, `fail` or `lost` row, although PR #373 merged on 2026-10-11T11:26:38Z. The log holds events for Tasks 1 to 3 only: Tasks 4 to 12 have no rows in it, so per-task attempts, review rounds and first-pass counts for them are unknown. From GitHub: 12 task PRs, 103 commits, 43 submitted reviews, first PR opened 2026-10-10T16:16Z, last merge 2026-10-12T00:34Z (about 32 hours).

| Task | Agent | Attempts | Review rounds | PR | Merged | First pass |
|---|---|---|---|---|---|---|
| 1 | go-implementer | 1 | 2 | #352 | no | no |
| 2 | go-implementer | 1 | 0 | #353 | yes | yes |
| 3 | go-implementer | 1 | 0 | - | no | no |

(The table is the log's own output; the Merged column reads the log, and all 12 PRs are merged on origin/main.)

## Lessons

- What worked: the decider table (Task 2) let Codex join admission without a per-site switch, and the doctor, key-parity and e2e tests (Tasks 8, 10, 11) pinned that every Codex run stays blocked. Architect review of #388 and #397 caught the unobserved-native-after-protocol-failure hazard (F1, A1, A2) before Codex became startable.
- What worked: designing for no vendor evidence (everything fixture-tested, `planned` row, every OQ held at its default) kept the claim small, and the whole-spec reviewers found no overclaim.
- What to change: findings discovered mid-task lived only in `scratchpad.md` or `handoff.md` (the Claude probe bound, the `CODEX_HOME` question) and would have been archived unfiled; the finalizer had to file them (#409, #408 item 5).
- What to change: Tasks 4 to 12 left no run-events rows, so the Effort section cannot be produced from the log for 9 of 12 tasks.
- What to change: the spec was left in `in-progress/` after its last merge, so the finalize run had to open the lifecycle pull request (#406) itself before `finalize.py verify` could pass.
- What to change: a test pinned a latent hazard as expected behaviour (`TestCodexStrictKeyLevel` accepts a key with unknown endpoint and entitlement class as `Eligible`); the per-task review accepted it because D13 blocked first.

## Proposals

- P-codex-native-adapter-1 — Open the lifecycle pull request when a spec is finalized from `in-progress/`
- P-codex-native-adapter-2 — File out-of-scope discoveries as issues before the task's pull request merges
- P-codex-native-adapter-3 — Derive Effort from GitHub when the run-events log is incomplete
- P-codex-native-adapter-4 — Report a zero-job cancelled main run as cancelled, not infra, and never as a descendant's verdict
- P-codex-native-adapter-5 — Finalize copies the changelog to its docs mirror
