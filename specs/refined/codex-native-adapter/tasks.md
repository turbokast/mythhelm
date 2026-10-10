## Codex Native Adapter — Tasks

### Dependencies

- Prerequisites: `specs/*/qualification-registry/` (done), `specs/*/budget-ledger-s1/` (unfinalized, all tasks done). Sequencing against in-progress specs (requirements § Dependencies): Tasks 2, 8 and 9 edit `internal/admission/*`, `internal/workers/worker.go` and `internal/supervisor/pipeline.go`, so they start after `specs/*/supervised-stop-recover/` Task 4 and `specs/*/claude-strict-subscription/` Task 5 (and its Q1 `UnresolvedSources` edit) have merged; `claude-strict-subscription` Task 7 is a maintainer live run sharing no files and is not a gate. Tasks 1 and 3 to 7 touch only `internal/adapter` and the new `adapters/codex` and may start earlier. `contained-execution-profiles` Task 11 requires a record or refusal test per accepted route, in its own `tests/e2e/contain_refusal_test.go` enumeration: Task 2 carries the Codex refusal test in `internal/admission/boundary_test.go`, and whichever of the two lands second extends the other's enumeration within its own Files (adding `tests/e2e/contain_refusal_test.go` to that task's Files if needed).
- Order: {1, 2} first (disjoint Files); {3} after 1; {4, 6, 7} after 3 (disjoint Files); {5} after 3 and 4; {8} after 2, 5, 6, 7; {9} after 5, 6, 8; {10} after 8; {11} after 9, 10; {12} after 11.
- External waits are prose only (`runspec.py` reads `Depends on` alone): Tasks 2, 8 and 9 carry the waits as notes in their `Depends on` field and the operator holds them.
- Maintainer questions do not block tasks: OQ1/OQ6/OQ9/OQ10 are held at their defaults and every task's tests pin the default behaviour; an answer arrives as a new reviewed requirement, not a silent edit.
- **Gates for every task.** `gofmt -w` on touched Go files first, then from the tree root `gofmt -l .` (prints nothing), `go vet ./...`, `go test -race ./...`, `go mod tidy -diff` (prints nothing), and `scripts/ci/check-public-hygiene.sh`. Architect review for Tasks 2, 8 and 9 (seam crossing).
- **Completion convention.** Append ` ✅ COMPLETED` to the heading, keep every original field, and add `Status` (`✅ Completed — …; PR #<n>`), `Implementation` (at most 3 lines, plus the commit SHA), `Spec deviations` (`None`, or each with its reason) and `Files modified`. Every task appends a scratchpad note under Discoveries.
- **Commits.** Every commit is signed off (`git commit -s`).

---

## Implementation Tasks

### Task 1 — Seam additions: Acknowledged, Reconnect, ModelMetadata

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: None
- **Change**: Add `InterruptReport.Acknowledged` and `Capabilities.Reconnect`/`ModelMetadata` with `omitempty` so Claude and fake serialise unchanged (D8, D9; AC-1.5).
- **Files**:
  - `internal/adapter/adapter.go`
  - `internal/adapter/adapter_test.go`
- **Produces**: `InterruptReport.Acknowledged string`; `Capabilities.Reconnect, ModelMetadata Tri`
- **Acceptance**:
  - `TestInterruptReportJSONOmitsUnsetAcknowledged`: a report from `ClimbLadder` marshals without an `acknowledged` key; dropping `omitempty` makes it fail.
  - `TestCapabilitiesJSONOmitsUnsetEntries`: a `Capabilities` value with the new fields unset marshals to exactly the pre-change key set (`structured_events` to `native_subagents`); setting `Reconnect` adds only `reconnect`; dropping `omitempty` makes it fail.
  - `TestClimbLadderSentAndConfirmed` (new; no test references `ClimbLadder` today): with a fake `OwnedProc` that ignores interrupt, `Sent` lists interrupt, terminate, kill in order and `Confirmed` turns true only once `GroupGone` holds; a ladder that set `Confirmed` after sending fails it.
- **Test plan**: Table tests over a fake `OwnedProc`; inline expected JSON (no goldens).
- **Invariants touched**: I09 (v2 §2: unset is distinct from `unknown`); I06 (`Confirmed` semantics pinned, not changed).

### Task 2 — Per-harness decider table, adapter validation and containment refusal

- **Domain/agent**: go-implementer
- **Budget**: complex
- **Depends on**: None (also waits for `specs/*/supervised-stop-recover/` Task 4 and `specs/*/claude-strict-subscription/` Task 5; see Dependencies)
- **Change**: Replace the `claudecode`/`fake` branches in `Decide`, `validate` and `dataDestinations` with a package-private `harnessDecider` table (D1), add `AdapterCodex`, extend the `--adapter` list and per-adapter flag help; the codex entry uses a local `codexAdapterID = "builtin/codex"` constant (pinned equal to `codex.AdapterID` in Task 8) and its `decide` blocks with `adapter_not_available` until Task 8; add the restricted/inspect refusal test for `builtin/codex` (N7) so a CLI that accepts `codex` already has it.
- **Files**:
  - `internal/admission/admission.go` (table, Decide, validate, dataDestinations)
  - `internal/admission/export_test.go` (test-only access to the table)
  - `internal/admission/admission_test.go` (stub-harness and flag tests)
  - `internal/cli/run.go` (flag help text)
  - `internal/supervisor/pipeline_test.go` (unknown-adapter case moves off `codex`)
  - `internal/admission/boundary_test.go` (Codex refusal test; `contain` cannot import `admission`)
- **Produces**: `AdapterCodex = "codex"`; package-private `harnessDecider`
- **Acceptance**:
  - `TestDecideDispatchesByTable`: a stub table passed to `decideWith` has its `decide` called, which calls a stub adapter's `Prepare` and returns its proposal; removing the row makes it fail (AC-1.1).
  - `TestValidateAdapterText`: the error reads `--adapter must be claudecode, codex or fake, got "x"`, and `--adapter codex` passes `validate` (AC-1.3).
  - `TestValidateClaudeFlagRules`: one case per Claude-only flag keeps today's accept/reject for `claudecode` and `fake`; an entry that loosened a rule fails it.
  - `TestCodexDeclarationFlagRefused`: `--declare-entitlement` and `--strip-credential-env` with `--adapter codex` are `ErrInvalid` from `validate`, the single refusal site (OQ8, OQ6 defaults).
  - `TestRestrictedAndInspectRefuseCodex`: both profiles with route `builtin/codex` are refused through `admission.ConsultRegistry`'s unknown-key path naming the route, and `contain.SeedV1` has no Codex key (N7).
  - The existing `internal/admission` and `internal/supervisor` tests pass with only the AC-1.3 edits.
- **Test plan**: Table-driven over `validate`; stub registered in the test only.
- **Invariants touched**: I11 (registration stays package-private); I14 (no capability advertised before evidence); I02 (unknown boundary refused).

### Task 3 — Codex adapter: probe, compatibility, capabilities

- **Domain/agent**: go-implementer
- **Budget**: complex
- **Depends on**: Task 1
- **Change**: Create `adapters/codex` with `New`, `Descriptor`, `Probe` (bounded `--version`, SHA-256, symlinks resolved), version compatibility and `Capabilities` per design §5; `doc.go` states it is not a stable API.
- **Files**:
  - `adapters/codex/doc.go`
  - `adapters/codex/probe.go`
  - `adapters/codex/compat.go`
  - `adapters/codex/probe_test.go`
  - `adapters/codex/compat_test.go`
  - `adapters/codex/stub.go` (temporary `Prepare`/`Start` returning `ErrNotImplemented` so `New` compiles; Task 5 deletes it)
- **Produces**: `codex.AdapterID`, `codex.New() adapter.Adapter`, `codex.ErrCapability`
- **Acceptance**:
  - `TestProbeReturnsIdentityWithoutModelTask`: against a `TestMain` re-exec fixture executable, `Probe` returns absolute path, version, SHA-256, OS, arch and the fixture records one invocation, `--version`, and no model task (AC-2.1).
  - `TestCompatibilityUntestedBlocksWithoutOverride`: an untested version yields `ErrCapability` unless `AllowUntestedNativeVersion` (AC-2.2).
  - `TestCapabilitiesUnknownNeverDefaultSupported`: Resume, LiveSteer, ApprovalBridge, UsageTokens, Reconnect, ModelMetadata are `unknown`; changing one to `Supported` in `Capabilities` fails the test (AC-2.3).
  - `TestProbeBoundedOutput`: a fixture printing over 4 KiB or sleeping past the timeout fails closed.
- **Test plan**: `TestMain` re-exec helper as in `adapters/claudecode/probe_test.go`; synthetic version strings.
- **Invariants touched**: I01 (unmodified executable); I09 (unknown stays unknown); I14.

### Task 4 — Codex adapter: stream decoder, usage and error mapping

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 3
- **Change**: Decode the synthetic NDJSON frames into `adapter.Observation`s with bounded reads; usage stays nil; errors map per D6/D7/D14 with the empty exhaustion table; `APIKeySource` stays empty (D12); unknown frame types counted.
- **Files**:
  - `adapters/codex/decode.go`
  - `adapters/codex/decode_test.go`
  - `adapters/codex/testdata/streams/success.jsonl`
  - `adapters/codex/testdata/streams/error.jsonl`
  - `adapters/codex/testdata/streams/unknown_type.jsonl`
  - `adapters/codex/testdata/streams/truncated.jsonl`
- **Produces**: `codex.Decode(r io.Reader, send func(adapter.Observation)) adapter.NativeExit`
- **Acceptance**:
  - `TestDecodeSuccessFixture`: `SessionStarted`, `Progress`, `Result` equal the golden; `Result.Tokens` is nil, not zero; `SessionStarted.APIKeySource` is empty (AC-4.4, AC-6.1, D12).
  - `TestDecodeMissingSessionIDIsUnknown`: a stream without a session frame yields an empty `SessionID`, never a synthesised one (AC-6.1).
  - `TestDecodeErrorsAreNativeError`: with the default table every error frame is `NativeError{Class:"native_error"}`; with a table passed to `decodeWith` a throttle frame maps to `rate_limit` and an exhaustion frame to `allowance_exhausted`; a mapping that reads the throttle as exhaustion fails (AC-4.6).
  - `TestDecodeCountsUnknownAndIncomplete`: unknown types land in `ProtocolCounters.UnknownTypes`; an oversize line is counted in `Counters.Oversized` and never errors; a stream that ends without a `result` frame sets `NativeExit.Err`, while an unterminated but complete final `result` frame is delivered; reading through any reader other than `internal/adapter/ndjson` leaves the counters at zero and fails the test.
  - `TestFixturesCarryNoCredentialStrings`: a scan of `testdata/` for credential-shaped strings finds none, and a planted string in a temp copy is found (NFR-2).
- **Test plan**: Streams are synthetic, marked by a `_synthetic` field on every frame; the shared `internal/adapter/ndjson` Reader.
- **Invariants touched**: I09; I03 (model output grants no authority: the decoder only emits observations); I19.

### Task 5 — Codex adapter: prepare, launch session, stop ladder, single writer

- **Domain/agent**: go-implementer
- **Budget**: complex
- **Depends on**: Task 3, Task 4
- **Change**: Implement `Prepare` (Windows refusal, hash recheck, argv constant per D5, allowlisted env, tool-rule and passthrough refusal, D16 fidelity deltas) and `Start` (one Launch, `Decode` pump, `Interrupt` via `ClimbLadder` with `Acknowledged:"unknown"`, D10 refusal).
- **Files**:
  - `adapters/codex/launch.go`
  - `adapters/codex/launch_test.go`
  - `adapters/codex/launch_unix_test.go`
  - `adapters/codex/stub.go` (deleted)
- **Produces**: `codex.Argv(bin string) []string`; `Prepare`/`Start` on the adapter
- **Acceptance**:
  - `TestPrepareRefusesChangedHash`: swapping the fixture binary between Probe and Prepare returns `*BlockedError{Code:"native_binary_changed"}` (AC-2.4).
  - `TestPrepareRefusesToolRulesAndPassthrough`: non-empty `AllowedTools` returns `native_tool_rules_unsupported`, non-empty `Passthrough` returns `native_passthrough_unsupported` (D15); with both empty, argv and env equal the golden (AC-1.4).
  - `TestPrepareRefusesWindows`: `Prepare` with `goos=windows` returns an error wrapping `ErrCapability`; the Unix path does not (N6).
  - `TestPrepareRecordsFidelityDeltas`: the proposal's `Overrides` carry a `fidelity` delta per aspect, every `Value` is `unobservable`, none `preserved` (AC-3.2, D16).
  - `TestPrepareEnvAllowlistDropsCanary`: a parent env containing a canary credential-named variable yields a child env without it (NFR-1).
  - `TestStopLadderEscalates` (Unix build tag): a fixture ignoring SIGINT receives terminate after the first grace and kill after the second and its group is gone within the summed graces; the report has three `Sent` entries and `Acknowledged:"unknown"` (AC-5.1, AC-5.2).
  - `TestSecondStartOnSameAdapterRefused`: a second `Start` on the same adapter value returns `input_writer_conflict` before any `Launch` call (counting launcher); a structural test shows `Session` has no write method (AC-5.3).
  - `TestAdapterHasNoReplayPath`: the adapter value's exported method set equals the `adapter.Adapter` interface exactly, so no Resume/Reconnect/Steer method exists (AC-5.4, AC-6.2).
  - `TestStartLaunchesOnlyThroughLauncher`: `launch.go` and `decode.go` import no `os/exec`; `Start` with a counting launcher shows exactly one `Launch`.
- **Test plan**: Fake `Launcher` and `OwnedProc` as in the Claude launch tests; ladder test with a Unix build tag.
- **Invariants touched**: I06 (sent is not confirmed); I18 (one owner, one writer); I01; I15 (no unscreened passthrough).

### Task 6 — Codex adapter: effective configuration inventory, trust inputs and gaps

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 3
- **Change**: Implement `InventoryAdmittedProject`, `InventorySettingsForEnv`, `AdmittedConfigPathsForEnv`, `UnresolvedSources`/`GapSources` and the credential-safety rule (D11, D17; AC-3.1, AC-3.3, AC-3.4, AC-3.5, AC-6.2).
- **Files**:
  - `adapters/codex/config.go`
  - `adapters/codex/config_test.go`
  - `adapters/codex/testdata/config/project.json`
- **Produces**: `codex.Manifest{Digests map[string]string; Digest string; Hooks int; MCPServers []string; RequiresTrust bool}`; `InventoryAdmittedProject(home, workdir string, blobs map[string][]byte, env []string) (Manifest, error)`; `InventorySettingsForEnv(home, workdir string, env []string) (Manifest, error)` (failure cases: `ErrNonRegularSource` for a source that is not a regular committed file, `ErrCapability` for an unreadable home); `AdmittedConfigPathsForEnv(home, workdir string, env []string) map[string]string`; `UnresolvedSources []string`; `GapSources(unresolved []string) []string`; `ProjectSources map[string]string` (source name to repo-relative path; the keys parameterise `readAdmittedBlobs` and the worker's on-disk re-hash skip list)
- **Acceptance**:
  - `TestInventoryDigestsChangeWithSource`: changing any enumerated source changes its digest and the combined `Digest` (AC-3.3).
  - `TestInventoryOpensOnlyEnumeratedFiles`: with an injected opener, a sentinel credential-named file outside the list is never opened; a variant that globs the home directory fails the test (AC-3.4).
  - `TestInventoryAndPathsAgree`: every key of `Manifest.Digests` has an entry in `AdmittedConfigPathsForEnv`, so the worker's re-inventory can verify it; a manifest key with no path fails.
  - `TestGapsBlockStrict`: `GapSources(UnresolvedSources)` is non-empty and keeps input order (AC-3.5).
  - `TestHooksCountedAndTrustRequired`: a fixture project with hooks reports `Hooks > 0` and `RequiresTrust`; one without reports neither.
  - `TestNoNativeStoreWrites`: non-test sources of the package contain no `os.WriteFile`, `os.Create` or `os.OpenFile`; a planted sample file with one is flagged (AC-6.2).
- **Test plan**: Injected file opener; synthetic project fixture; no credential values in fixtures.
- **Invariants touched**: I19 (never read credential contents); I02 (unknown mandatory evidence blocks); I03.

### Task 7 — Codex adapter: fixture qualification drafts and route inventory

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 3
- **Change**: Add `ObservedKey`, `AuthCategory`, `FixtureDraft` and the closed AC-4.2 route inventory; fixtures cannot prove a column and entitlement stays not-proven with `purchased_credit: unknown`.
- **Files**:
  - `adapters/codex/qualify.go`
  - `adapters/codex/qualify_test.go`
  - `adapters/codex/testdata/qualification/routes.json`
- **Produces**: `codex.ObservedKey(p adapter.Probe, m adapter.ConfigManifest, authCategory, profile string) qualify.Key`; `codex.AuthCategory(env []string) string`; `codex.FixtureDraft(k qualify.Key, ev FixtureEvidence) (qualify.Record, error)`; `codex.ErrNotCodexKey`, `codex.ErrFixtureCannotProve`; `codex.Route{Name, Coverage string}`; `codex.RouteInventory() ([]Route, error)` (the closed list); `codex.CheckRoute(name string) (Route, error)` returning `codex.ErrUnknownRoute`
- **Acceptance**:
  - `TestFixtureDraftRefusesOtherHarness`: a `claude-code` key returns `ErrNotCodexKey`; the Codex key succeeds (AC-7.1).
  - `TestFixtureDraftCannotProve`: an input asking for a `proven` fidelity, lifecycle or entitlement verdict returns `ErrFixtureCannotProve`; the accepted draft is `fixture-tested`, entitlement `not-proven` with `purchased_credit: unknown`, never `live-qualified` (AC-7.2, AC-4.3).
  - `TestFixtureDraftColumnsIndependent`: changing the fidelity input alters only the fidelity column of the draft (AC-7.1).
  - `TestRouteInventoryClosedAndUnknownBlocks`: the five listed routes each carry `coverage: unknown`; `CheckRoute` of a route outside the list returns `ErrUnknownRoute` (AC-4.2).
  - `TestAuthCategoryUnknownByDefault`: with no non-secret status the category is `unknown`, and `qualify.KeyHash` of the key differs from a native-managed key's (AC-4.7).
  - The existing `internal/qualify` seed pin (7 records, `codex` `planned`) passes unmodified, so no `exec` row was added (AC-2.5).
- **Test plan**: Pure functions; keys compared with `qualify.KeyHash`.
- **Invariants touched**: I14 (versioned evidence); I15 (entitlement not inferred from sign-in); I09.

### Task 8 — Admission: ResolveQualificationKey, neutral trust and the Codex decider

- **Domain/agent**: go-implementer
- **Budget**: complex
- **Depends on**: Task 2, Task 5, Task 6, Task 7 (also waits for `specs/*/supervised-stop-recover/` Task 4 and `specs/*/claude-strict-subscription/` Task 5; see Dependencies)
- **Change**: Split `ResolveQualification` into a wrapper and `ResolveQualificationKey` (D2), extract `ResolveBillingStrict`, add `NativeConfig`/`CheckNativeTrustConfig` (D17), map `codex.ErrCapability` to exit 7, and implement `decideCodex` (design §4) in the table.
- **Files**:
  - `internal/admission/qualify.go` (ResolveQualificationKey)
  - `internal/admission/billing.go` (ResolveBillingStrict, NativeAdmissionError)
  - `internal/admission/native.go` (admitNativeTrust over NativeConfig)
  - `internal/admission/admission.go` (decideCodex)
  - `internal/admission/codex_test.go` (new)
  - `internal/admission/qualify_test.go` (parity test)
  - `internal/admission/export_test.go` (`decideCodexWith`)
- **Produces**: `admission.ResolveQualificationKey(ctx, reg, observed qualify.Key, billing string) (Eligibility, error)`; `admission.ResolveBillingStrict(ctx, elig Eligibility) (BillingPosture, error)`; `admission.NativeConfig{Digests map[string]string; Digest string; Hooks int; MCPServers []string; RequiresTrust bool}`; `admission.CheckNativeTrustConfig(ctx, j, repoID string, cfg NativeConfig, explicitDigest string) (bool, error)`
- **Acceptance**:
  - `TestResolveQualificationKeyParity`: identical records under `claude-code` and `codex` keys give identical verdicts for every billing mode and block code (AC-7.4); the existing `ResolveQualification` tests pass unmodified.
  - `TestCodexStrictKeyLevel`: at `ResolveQualificationKey`/`ResolveBillingStrict`, with a hand-built native-managed key, no registry, a `planned` and a `fixture-tested` record each block with `no_qualification_record` or `entitlement_not_proven`; a test-only `live-qualified` record with proven entitlement but no `stop_at_exhaustion` blocks with `stop_at_exhaustion_unproven`; only both together admit (AC-4.1, O3); `purchasedCreditPreventable(rec, now)` is false for a live-qualified record lacking an unexpired `purchased_credit:preventable` entitlement evidence entry (and for an expired one) and true with it; a variant ignoring expiry or the evidence `Result` fails (AC-4.3); a record with proven entitlement and stop support but no quota datum at all still admits, and a variant requiring a quota datum fails (AC-4.5).
  - `TestCodexKeyVariationBlocks`: varying `Harness` (to another name in `knownHarnesses`, e.g. `opencode`), `ProviderEndpoint` or `AuthCategory` of an otherwise admitting record blocks with `no_qualification_record` (AC-7.5, AC-4.7).
  - `TestCodexDecideOrder`: `Decide` with `--adapter codex --billing subscription-only --execution-profile trusted-host` blocks with `no_qualification_record` when the auth category is `unknown` (D13); through `decideCodexWith` with a native-managed category and an opened registry it blocks with `entitlement_not_proven` from the gap gate (D11), and with no registry the gap gate is skipped and the consult blocks with `no_qualification_record`; no Codex run is admitted (honesty register); non-empty passthrough is refused before any fixture invocation (D15).
  - `TestCodexUntrustedConfigBlocksBeforeProbe`: a fixture project with an untrusted hook blocks with `untrusted_native_config` and the fixture executable records zero invocations; a variant that probes before trust records one and fails (AC-3.1).
  - `TestCodexNonStrictBillingRefused`: `subscription-declared` and `local-scripted` with `--adapter codex` are refused with `billing_unsupported_for_adapter` (D3).
  - `TestNativeAdmissionErrorMapsCodexCapability`: an error wrapping `codex.ErrCapability` maps to a `Capability: true` block (exit 7) (AC-2.2).
  - `TestCheckNativeTrustParity`: `CheckNativeTrust` and `CheckNativeTrustConfig` agree on every existing trust fixture.
  - `TestCodexBlockFieldsNameTheGate`: the unknown-auth block carries `Field` naming the auth category, the gap-gate block carries `missing-source:<name>`, and the consult block carries `codex × <surface>`; a swap of order changes a `Field` and fails.
  - `TestCodexAdapterIDMatches`: the `internal/admission` constant `codexAdapterID` equals `codex.AdapterID`.
- **Test plan**: Temp-dir registry with hand-built records; fixture executable from Task 3; real files and git in temp dirs.
- **Invariants touched**: I02, I15 (strict blocks on unknown); I04 (key change is a new route); I19; I01.

### Task 9 — Register Codex in workers, supervisor and runtime route check

- **Domain/agent**: go-implementer
- **Budget**: complex
- **Depends on**: Task 5, Task 6, Task 8 (also waits for `specs/*/supervised-stop-recover/` Task 4; see Dependencies)
- **Change**: Add `builtin/codex` to the worker adapters map, select config-path sources by adapter ID in `pipeline.go` (`d.Adapter.ID`) and `contain.go` (`l.AdapterID`, including the list of blob source names the worker does not re-hash against disk), and skip the Claude `billing_route_mismatch` check for `builtin/codex` (D12).
- **Files**:
  - `internal/workers/worker.go` (adapters map, route-check skip)
  - `internal/workers/contain.go` (per-adapter inventory)
  - `internal/supervisor/pipeline.go` (per-adapter config paths)
  - `internal/workers/worker_test.go` (map and route-check tests)
  - `internal/supervisor/pipeline_launch_test.go` (codex path)
- **Acceptance**:
  - `TestWorkerStartsCodexByDescriptor`: `builtin/codex` starts the Codex adapter and an unknown ID is rejected (AC-1.2).
  - `TestCodexSessionNotInterruptedForEmptyAPIKeySource`: a Codex session whose `SessionStarted.APIKeySource` is empty is not interrupted, while a Claude session with `APIKeySource` `"apiKey"` still ends with `billing_route_mismatch`; removing the per-adapter skip fails the first leg (D12).
  - `TestCodexConfigPathsAndInventoryUsed`: the pipeline sets `UserConfigPaths` from the Codex function for a Codex proposal and the worker's re-inventory uses the Codex inventory; swapping the selector makes a Codex manifest key fail with no path.
  - `TestCodexBlobSourcesNotRehashedAgainstDisk`: Codex project-blob digests are skipped by the on-disk re-hash exactly as Claude's are; adding a Codex blob source name to the skip list is what makes it pass.
  - `TestClaudeConfigPathsUnchanged`: the existing Claude launch and contain tests pass unmodified; the Claude digest-to-path map is byte-equal to its pre-change output.
- **Test plan**: Fixture executable; existing worker harness.
- **Invariants touched**: I14; I02; I18; I15 (route check not silently weakened for Claude).

### Task 10 — Doctor: failing columns and next test in plain output

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 8
- **Change**: Render ` failing: <col>[, <col>]` and a `next_test: <text>` line below each record row for every record in plain doctor output, keep the JSONL `qualification` object byte-equal (record its pre-change output as a new golden first) and update the `qualLine` pin (AC-7.3).
- **Files**:
  - `internal/cli/doctor.go`
  - `internal/cli/doctor_test.go`
  - `internal/cli/testdata/doctor_qualification.jsonl`
- **Acceptance**:
  - `TestDoctorPlainShowsFailingAndNextTest`: the seeded Codex row prints `failing: fidelity, entitlement, lifecycle` and the seed's `next_test` text; a registry with all columns `proven` prints no `failing:` token.
  - `TestDoctorOrderUnchanged`: row order equals `antigravity, claude-code, codex, cursor, kimi, muse, opencode`.
  - `TestDoctorJSONLUnchanged`: the `qualification` object of the JSONL output (host-specific facts excluded) equals `testdata/doctor_qualification.jsonl`, recorded from the unmodified code before the change; adding a field to a record fails it.
- **Test plan**: Existing doctor harness with the seeded registry; the golden is captured in the task's first commit.
- **Invariants touched**: I09 (unknown renders as failing, never proven); I14.

### Task 11 — End-to-end: Codex stays blocked and refused

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 9, Task 10
- **Change**: Run the production binary with `--adapter codex` and a fixture executable: every billing mode ends blocked or refused with its code, and doctor shows the Codex row.
- **Files**:
  - `tests/e2e/codex_qualification_test.go`
- **Acceptance**:
  - `TestCodexBillingModesAllRefuseOrBlock`: with `--execution-profile trusted-host`, `--billing subscription-only` exits blocked with `no_qualification_record` (unknown auth category, D13) and `subscription-declared` and `local-scripted` exit blocked with `billing_unsupported_for_adapter` (D3); a variant admitting the declared mode fails the second leg.
  - `TestCodexDoctorRowEndToEnd`: `mythhelm doctor` on a state directory seeded through `qualify.EnsureSeeded` prints the Codex row `planned` with `failing: fidelity, entitlement, lifecycle` and its `next_test` line.
- **Test plan**: Existing e2e helpers; fixture executable built in `TestMain`; scrubbed parent environment.
- **Invariants touched**: I13 (no credentials in tests); I15; I02.

### Task 12 — Document the Codex limits and record the decisions

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 11
- **Change**: State in `docs/limitations.md` and `README.md` that Codex has fixture-tested adapter code but its registry row stays `planned`, which columns are unproven and why strict admission blocks (NFR-3), and record D1, D3, D10 and D12 in a decision record.
- **Files**:
  - `docs/limitations.md`
  - `README.md` ("Supported now" list)
  - `docs/decisions/0016-codex-adapter-admission.md`
- **Acceptance**:
  - `grep -n 'codex' README.md` anchored to the "Supported now" list prints a line stating Codex is not yet a supported adapter, and the list still names `--adapter fake|claudecode`; with the line removed the anchored grep prints nothing.
  - `grep -n '^### Codex' docs/limitations.md` prints one heading, and the section under it names fidelity, entitlement and lifecycle as unproven (a region-anchored grep over that section prints three matches; with the section removed it prints none).
  - `docs/decisions/0016-codex-adapter-admission.md` has Status, Context, Decision, Consequences sections and cites D1, D3, D10 and D12; `ls docs/decisions` shows no other file with the same number (renumber to the next free number if taken); `scripts/ci/check-public-hygiene.sh` exits 0.
- **Test plan**: Commands run from the tree root; sections anchored by heading.
- **Invariants touched**: I14, G10 (published limitations); I15 (the billing decision D3 is recorded).
