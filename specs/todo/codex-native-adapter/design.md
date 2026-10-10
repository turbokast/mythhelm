## Codex Native Adapter — Design

Normative source: mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md, version 2.0. Requirements: `specs/*/codex-native-adapter/requirements.md` (refined; OQ1 to OQ10 hold their stated defaults). Scope: single spec, auto-confirmed (non-interactive); see §9.

### 1. Current state

Derived at origin/main `bc65309`; each line is re-checkable by the command beside it.

- Adapters: `ls adapters` shows `claudecode`, `fake`. `internal/workers/worker.go` `var adapters = map[string]func() adapter.Adapter{"builtin/fake": fake.New, "builtin/claudecode": claudecode.New}` (grep `"builtin/claudecode"`). `Start` is `adapters[w.launch.AdapterID]().Start(ctx, lp, l)`; an unknown ID is rejected earlier (grep `adapters\[l.AdapterID\]`).
- Seam (`internal/adapter/adapter.go`): `Adapter{Descriptor, Probe, Capabilities, Prepare, Start}`; `PrepareInput` carries `AllowedTools` and `Passthrough` marked `claudecode only`; `InterruptReport{Sent []StopSignal, Confirmed bool, Errors []string}`; `ClimbLadder(ctx, proc OwnedProc, ladder []StopStep) InterruptReport` appends a rung to `Sent` even when `Signal` errored; `Capabilities{StructuredEvents, Resume, LiveSteer, ApprovalBridge, UsageTokens, QuotaRemaining, HardMonetaryLimit, NativeSubagents}` has no reconnect or model-metadata field. Sessions expose `Observations`, `Interrupt`, `Done` only: no write or steer method.
- Admission (`internal/admission/admission.go`): `Decide` branches `if req.Adapter == AdapterClaudeCode { decideClaudeCode } else { decideFake }`; `validate` lists `claudecode`/`fake` and rejects others with `--adapter must be claudecode or fake, got %q`; `dataDestinations` switches on the two IDs; `decideClaudeCode` runs repo, credential-env screen, probe, `admitNativeConfig`, `claudecode.AuthStatus`, the `UnresolvedSources` gap check (strict only), `ResolveQualification`, `resolveDeclaration`, `ResolveBilling`, `Prepare`.
- Consult (`internal/admission/qualify.go`): `ResolveQualification(ctx, reg, probe, manifest, evidence claudecode.AuthEvidence, billing, profile)` builds the key with `observedKey` (hard-coded `claudecode.New().Descriptor()`), then applies the verdict logic with codes `no_qualification_record`, `entitlement_not_proven`, `stop_at_exhaustion_unproven`, `qualification_drifted`, `ambiguous_qualification_match`, `unsupported_surface`. `knownHarnesses` already contains `codex`. `ResolveBilling` strict branch is harness-neutral; its other branch is Claude-only (`billing.go`, `decl.AdapterID != claudecode.AdapterID`).
- Registry: `internal/qualify/seed.go` `seedHarnesses` has seven rows; `codex` is `ProgressPlanned`. `internal/qualify/seed_test.go` pins 7 records; `internal/cli/doctor_test.go` pins sorted order and the `qualLine` plain format. `qualify.Record.Capabilities` is `map[string]Capability`; `Key` has `Harness, Surface, ExecutableDigest, AdapterProtocol, OS, Arch, ProviderEndpoint, ModelSnapshot, EffortSettings, AuthCategory, ConfigDigest, TrustProfile, WorkspaceClass, EntitlementClass`.
- Containment: `internal/contain/records.go` `SeedV1` keys `profile/os/route` over routes `builtin/fake`, `builtin/claudecode` only; unknown keys are refused (I02).
- Flags: `internal/cli/run.go` help for `--strip-credential-env`, `--trust-native-config`, `--declare-entitlement`, `--allow-untested-native-version` says "claudecode only".
- Billing: `internal/billing/normalize.go` `splitRoutes = map[string]bool{"claude-code": true}`; other routes expose the combined total only.
- Worker route check: `internal/workers/worker.go` (grep `billing_route_mismatch`) interrupts any attempt whose `SessionStarted.APIKeySource != "none"`; this is Claude stream-json behaviour. `NativeAdmissionError` (`internal/admission/billing.go`) maps only `claudecode.ErrCapability` to exit 7. `claudecode.Prepare` refuses Windows (`adapters/claudecode/launch.go`). Trust inputs: `CheckNativeTrust(ctx, j, repoID, manifest claudecode.Manifest, explicitDigest)` and `admitNativeTrust` (`internal/admission/native.go`) read `Digests`, `Digest`, `Hooks`, `MCPServers`, `RequiresTrust`; the pipeline sets `UserConfigPaths` from `claudecode.AdmittedConfigPathsForEnv` and the worker re-inventories with `claudecode.InventorySettingsForEnv`.
- `command -v codex` finds nothing in the authoring environment. Every statement about Codex's native surface is therefore a default (OQ1, OQ6, OQ9, OQ10), never a fact.

### 2. Decisions

| ID | Decision | Rationale |
|---|---|---|
| D1 | Per-harness decider table in `admission`: `type harnessDecider struct{ name, adapterID string; decide func(context.Context, Request, Decision) (Decision, error); flags flagSet }`, with `claudecode`, `codex`, `fake` entries; `Decide` and `validate` read the table. Registration is package-private (I11). | A `switch` per site (validate, dispatch, dataDestinations) is the current shape and would grow three more arms; one table makes AC-1.1's stub-harness test possible. Rejected: an exported registry (would let code outside the module register an adapter, I11). The table is a value passed to an unexported `decideWith(table, ...)`, so tests supply their own table without mutating package state. |
| D2 | Keep `ResolveQualification` and its signature as the Claude wrapper; add `ResolveQualificationKey(ctx, reg, observed qualify.Key, billing string) (Eligibility, error)` holding the verdict logic. | The 7 existing call sites and tests stay untouched (AC-1.3); `git grep -n 'ResolveQualification(' -- '*.go'` prints 8 lines: the definition in `qualify.go` and the 7 calls (`admission.go`, five in `qualify_test.go`, `run_drift_test.go`). Rejected: changing the signature (edits every Claude test). |
| D3 | Codex runs only under strict `subscription-only`; `subscription-declared` and `local-scripted` are refused for Codex with `billing_unsupported_for_adapter` (OQ8 default; `local-scripted` belongs to the fake adapter). | A declaration path is a billing posture choice that needs a decision record; not decided here. D1 and D3 get a decision record in Task 12. |
| D4 | Do not add Codex to the registry seed; the existing `planned` row stays (OQ7 default). Fixture evidence drafts live in `adapters/codex` and have no production caller, mirroring Claude. | Seed rows insert only when absent, so editing the seed would not update existing registries and would break the 7-record pin. |
| D5 | App-server is the primary structured surface (OQ1 default); the decoder reads synthetic NDJSON frames defined in `adapters/codex/testdata/` and counts unknown frame types in `ProtocolCounters`. The argv constant is a placeholder fixed by the OQ1 answer. `exec` gets no code. | The real protocol cannot be verified here. Pinning only MYTHHELM's own parsing is what v2 §7.1 allows from fixtures. Rejected: guessing the vendor frame schema. The error table of D7 is likewise an argument of an unexported `decodeWith(table, ...)`; no package variable is mutated by tests. |
| D6 | Usage: the decoder sets `TokenUsage` fields only for frames whose non-overlap has a cited source; with no cited source (OQ10 default) every field is nil, `Result.Tokens` is nil and the total reads unknown. `billing.splitRoutes` is not changed. | I09 and v2 §7.3: overlapping counters must not be summed. |
| D7 | Native-error mapping: every error is `NativeError{Class: "native_error"}` until OQ10 supplies shapes. The `rate_limit`/exhaustion branch of AC-4.6 is implemented behind a table that is empty by default, with the red-first test proving that populating it changes the class. | Same reason as D6; keeps AC-4.6's branch real without inventing vendor shapes. |
| D8 | The stop ladder reuses `adapter.ClimbLadder`: native-interrupt rung is `StopInterrupt` (signal) then `StopTerminate`, `StopKill` with graces 5s, 10s, 5s (values in one constant, pinned by test). `InterruptReport` gains `Acknowledged string` (`yes`, `no` or `unknown`); Codex always sets `unknown` (OQ9 default); Claude/fake leave it empty and `omitempty` keeps their JSON unchanged. `Acknowledged` is not added to the journal `stopped` event. | AC-5.2, AC-1.3. Rejected: changing `Confirmed` semantics (Claude depends on them). |
| D9 | `adapter.Capabilities` gains `Reconnect Tri` (`json:"reconnect,omitempty"`) and `ModelMetadata Tri` (`json:"model_metadata,omitempty"`); Claude and fake do not set them, so their serialised records are byte-identical. Codex sets both `unknown` and `Resume`, `LiveSteer`, `ApprovalBridge`, `UsageTokens` `unknown`. | AC-2.3, AC-1.5. An unset entry is "not reported by this adapter", distinct from `unknown`, documented on the field. |
| D10 | Single input writer (AC-5.3): the sole writer is the worker-owned stdin pipe passed at `Start`; `Start` refuses a second call on the same adapter value, before `Launch`, with `*adapter.BlockedError{Code:"input_writer_conflict"}`, and `Session` stays write-method-free. The worker builds a fresh adapter per start, so the cross-worker guarantee rests on the absence of any resume or reconnect path (N4), not on this check. | A refusal keyed on the native session ID would arrive after the second launch (the ID is reported later). |
| D11 | Config inventory is an enumerated list `inventorySources` (project instruction file, project config file, user config file); every source not enumerated is listed in `UnresolvedSources` (managed policy, OS-managed preferences). Project sources are single committed files only, as for Claude (`admittedProjectSources`) and blocks strict admission (AC-3.5). Paths are placeholders confirmed by OQ6. | Mirrors `claudecode.UnresolvedSources`; unknown mandatory evidence blocks (I02). |
| D12 | `SessionStarted.APIKeySource` is left empty by the Codex decoder (not reported), and the worker's `billing_route_mismatch` check (`!= "none"`) is skipped for `builtin/codex`. The route is established at admission by the key (AC-7.5), not by a runtime check. | Setting `"none"` would assert an unverified vendor fact (I09); leaving it empty would stop every attempt at startup. |
| D13 | Observed `AuthCategory == "unknown"` blocks in `decideCodex` with `no_qualification_record` before the registry lookup. The strict gap gate (D11) also blocks first. No Codex run is admittable in this spec; admit legs are asserted at `ResolveQualificationKey`/`ResolveBillingStrict` level with a hand-built native-managed key, never through `Decide`. | Unknown mandatory evidence blocks (I02); a test through `Decide` that expects admission would need evidence that cannot exist offline. |
| D14 | Codex error class names reuse the worker's: `rate_limit` for throttling (v2 §4.5 `provider_throttled`), `allowance_exhausted`, else `native_error`. | The worker branches on `rate_limit`; a new class would be silently ignored. |
| D15 | A non-empty project `Passthrough` is refused for Codex with `native_passthrough_unsupported` until OQ6 names Codex credential variables; the credential screen cannot run without them. | Passing project variables unscreened could carry a metered route (I15). |
| D16 | Fidelity deltas (AC-3.2): `Prepare` records `ConfigDelta{Kind:"fidelity", Name:<aspect>, Value:"unobservable", Reason}` for instruction/skill discovery, hooks, MCP, children and native context behaviour; no aspect is recorded `preserved` without a matched direct-native comparison. | v2 §7.1; I09. |
| D17 | Trust is generalised by a neutral `admission.NativeConfig{Digests map[string]string; Digest string; Hooks int; MCPServers []string; RequiresTrust bool}`; `CheckNativeTrust` keeps its signature and calls `CheckNativeTrustConfig(ctx, j, repoID, cfg NativeConfig, explicitDigest)`. | Existing callers and tests stay untouched (AC-1.3). |

### 3. Seam changes (core, `internal/adapter`)

```go
// adapter.go
type InterruptReport struct {
	Sent         []StopSignal `json:"sent"`
	Acknowledged string       `json:"acknowledged,omitempty"` // "yes", "no", "unknown"; empty when the adapter does not report
	Confirmed    bool         `json:"confirmed"`
	Errors       []string     `json:"errors,omitempty"`
}
type Capabilities struct { /* existing fields */
	Reconnect     Tri `json:"reconnect,omitempty"`
	ModelMetadata Tri `json:"model_metadata,omitempty"`
}
```

Nothing else changes in `internal/adapter`. Negotiated optional methods (inspect-liveness, reconnect, resume, steer, approval response) are not added (honesty register).

### 4. Admission (core, `internal/admission`)

- `harnessDecider` table (D1): `Decide` looks up `req.Adapter` (`claudecode`, `codex`, `fake`); `validate` derives the adapter list text and the per-adapter flag rules from the entry's `flags` (`strip-credential-env`, `trust-native-config`, `declare-entitlement`, `allow-untested-native-version`, `scenario`). Error text: `--adapter must be claudecode, codex or fake, got %q`; the "(claudecode or fake)" required-flag text lists all three.
- `const AdapterCodex = "codex"`. `dataDestinations` gains `codex.AdapterID: "first-party native API over the network"`; the case is added through the table.
- `ResolveQualificationKey` (D2). `observedKey` stays Claude's.
- `decideCodex(ctx, req, d)` calls the unexported `decideCodexWith(ctx, req, d, authCategory string)` with `codex.AuthCategory(req.Env)`; `export_test.go` exposes `decideCodexWith` so a test can supply a native-managed category. Order: (1) refuse `req.Billing != subscription-only` with `billing_unsupported_for_adapter` (D3); (2) `admitRepo`; (3) refuse non-empty project `Passthrough` (D15) and the trusted project's tool rules (the value the Claude path passes as `PrepareInput.AllowedTools`) with `native_tool_rules_unsupported`, before any env is built (`DeclareEntitlement` and `StripCredentialEnv` are rejected once, in `validate`); (4) `security.BuildEnv(req.Env, nil, nil)` (the Codex credential screen is skipped, OQ6); (5) `codex.InventoryAdmittedProject` and trust (`admitNativeTrust` over `NativeConfig`, D17), so no native process of any kind starts first (AC-3.1); (6) `a.Probe`; (7) `authCategory == "unknown"` blocks with `no_qualification_record` (D13); (8) open the registry; only when it opens (as for Claude), the strict gap gate `codex.GapSources(codex.UnresolvedSources)` blocks with `entitlement_not_proven`; (9) `codex.ObservedKey` and `ResolveQualificationKey`; (10) the Codex-only check `purchasedCreditPreventable(rec qualify.Record, now time.Time) bool` (exposed through `export_test.go` and tested directly, since steps (8) and (9) block first in every offline state): the entitlement column needs an unexpired `Evidence` entry with `Result == "purchased_credit:preventable"`, else `entitlement_not_proven` (AC-4.3); (11) `ResolveBillingStrict`; (12) `a.Prepare`. `AuthCategory(env)` returns `"unknown"` unless the native-managed category is established from non-secret status (never reads credential contents, I19).
- `ResolveBillingStrict(ctx, elig Eligibility) (BillingPosture, error)` holds the strict branch already in `ResolveBilling` (which now calls it); behaviour unchanged for Claude.
- Block codes for Codex are the existing consult codes; a non-native `AuthCategory` yields `no_qualification_record` through the key match (AC-4.7); a key varying `Harness`, `ProviderEndpoint` or `AuthCategory` yields the same (AC-7.5).
- `PrepareInput` fields `AllowedTools`/`Passthrough`: `codex.Prepare` returns `*adapter.BlockedError{Code: "native_tool_rules_unsupported"}` when `AllowedTools` is non-empty (AC-1.4).

### 5. `adapters/codex` (adapters)

Files and exported surface (package doc states it is not a stable API):

```go
package codex
const AdapterID = "builtin/codex"
func New() adapter.Adapter
// Descriptor: ID AdapterID, Version "1", Harness "codex", Surface "native-app-server (structured)"
var ErrCapability error                       // version/platform refusal -> CLI exit 7
func ObservedKey(p adapter.Probe, m adapter.ConfigManifest, authCategory, profile string) qualify.Key
func AuthCategory(env []string) string        // "unknown" unless established from non-secret status
var UnresolvedSources []string
func GapSources(unresolved []string) []string
type Manifest struct{ Digests map[string]string; Digest string; Hooks int; MCPServers []string; RequiresTrust bool }
func InventoryAdmittedProject(home, workdir string, blobs map[string][]byte, env []string) (Manifest, error)
func InventorySettingsForEnv(home, workdir string, env []string) (Manifest, error)
func AdmittedConfigPathsForEnv(home, workdir string, env []string) map[string]string
func FixtureDraft(k qualify.Key, ev FixtureEvidence) (qualify.Record, error) // ErrNotCodexKey, ErrFixtureCannotProve
```

- Probe: resolves `codex` on PATH (or the test override rejected outside tests, same rule as Claude), runs `--version` bounded (5s, 4 KiB), hashes the file, resolves symlinks; `compatibility(version, allowUntested)` marks only the fixture version `"fixture-tested on <v>"`; others `"... untested"` and blocked unless `AllowUntestedNativeVersion`.
- Prepare: refuses Windows with `ErrCapability` (N6); refuses non-empty `AllowedTools` (AC-1.4) and non-empty `Passthrough` (D15); records the D16 fidelity deltas; re-hashes the executable, refuses a changed hash (`*BlockedError{Code:"native_binary_changed"}`), builds the argv (D5 constant), the allowlisted env plus `MYTHHELM_ATTEMPT_ID`, `Stdin = in.Prompt`, ladder per D8, `Billing` left for admission, `Manifest` for admission to fill.
- Start: one `Launch` through `adapter.Launcher`; session goroutine decodes stdout via the bounded NDJSON reader (`internal/adapter/ndjson`), sends observations, never writes to the native. `Interrupt` = `adapter.ClimbLadder` plus `Acknowledged:"unknown"`.
- Decode: reads through the shared bounded `internal/adapter/ndjson` Reader, so oversize frames are counted in `ProtocolCounters`, never an error, and an unterminated final line is delivered as a frame; a stream that ends without a `result` frame sets `NativeExit.Err` (incomplete). Frames `session`, `turn`, `tool`, `error`, `result` from the synthetic schema; `result` maps `Result{Subtype, IsError, StopReason}`; `Tokens` stays nil (D6); `error` frames become `NativeError{Class}` per D7; unknown types counted.
- Capabilities: D9 values plus `StructuredEvents: Unknown` (a fixture proves only the decoder; support needs vendor evidence of the emitted frames, OQ1; admission's required-capability check therefore blocks any Codex run with `capability_unavailable`, consistent with D13), `Platform.ProcessTreeOwnership` as Claude's on Unix and `unknown` elsewhere (N6), `Billing.IncludedOnlySupported: Unknown`, `PaidOveragePrevention: Unknown`, `Qualification: "planned"` until a record says otherwise.
- `ObservedKey` fields: `Harness` "codex"; `Surface` the descriptor's; `ExecutableDigest` `"sha256:"+probe.SHA256`; `AdapterProtocol` `AdapterID+"+app-server-fixture"`; `OS`/`Arch` from the probe; `ProviderEndpoint`, `ModelSnapshot`, `EffortSettings`, `EntitlementClass` `"unknown"`; `AuthCategory` the argument; `ConfigDigest` `qualify.ConfigDigestOf(manifest.Digests)`; `TrustProfile` the profile; `WorkspaceClass` `"local-checkout"`.
- Qualification drafts: `FixtureDraft` builds a `fixture-tested` record whose Fidelity and Lifecycle columns are `unknown` or `not-proven` with the fixture suite cited (it refuses a `proven` verdict from fixtures with `ErrFixtureCannotProve`) and whose Entitlement column is `not-proven` carrying `purchased_credit: unknown`; it refuses any key whose `Harness != "codex"`. The closed auxiliary route list (AC-4.2) is `testdata/qualification/routes.json` (title, summary/compaction, review, child, native observer; each `coverage: unknown`).

### 6. Workers, supervisor, containment (core)

- `internal/workers/worker.go`: `adapters` map gains `"builtin/codex": codex.New`.
- `internal/supervisor/pipeline.go` (line using `claudecode.AdmittedConfigPathsForEnv`) selects by `d.Adapter.ID`; `internal/workers/contain.go` (`claudecode.InventorySettingsForEnv`) selects by `l.AdapterID`; `internal/workers/worker.go` skips the `billing_route_mismatch` check for `builtin/codex` (D12); for `builtin/codex` the Codex inventory (D11). No behaviour change for the other two.
- `internal/contain/records.go` `SeedV1`: no `builtin/codex` records. A Codex run with `--execution-profile restricted|inspect` is refused through the existing unknown-key path; Task 2 adds the refusal tests required by `contained-execution-profiles` Task 11 (merged, #310; N7): `TestRestrictedAndInspectRefuseCodex` and the codex cases of `TestUnsupportedRefusalPrecise` in `tests/e2e/contain_refusal_test.go`.

### 7. Doctor (core)

`qualificationPlain` appends, per record: ` failing: <col>[, <col>]` when any of fidelity/entitlement/lifecycle verdict is not `proven`, and a second line `  next_test: <text>` when the record has one. The change is made for every row, which is why the AC-1.3 exceptions list the plain format and the `qualLine` anchored regex is updated. JSONL is unchanged.

### 8. Tests

- Fixture native: `adapters/codex/testdata/` synthetic NDJSON streams (success, error, unknown-type, truncated) and a `TestMain` re-exec helper as Claude's probe tests do; all marked synthetic.
- Unit tests per package; e2e `tests/e2e/codex_qualification_test.go` runs the production binary with `--adapter codex` (every billing mode blocked or refused with its code) and doctor against a seeded registry. Admit legs run only at key level in `internal/admission` (D13).
- Red-first variants (teeth-discipline): sentinel-open (AC-3.4), throttle-read-as-exhaustion (AC-4.6), untrusted-hook-launch-count (AC-3.1), identical-records parity (AC-7.4).

### 9. Why this is one spec of 12 tasks

FR-1 (seam) and the adapter are separable in code but neither ships alone with value: the seam work without Codex is a refactor with no observable benefit, and the adapter cannot be admitted without it. Both sit on one dependency chain, so they cannot be built in parallel by different agents; this fails the "independent work stream" test of `/spec-scope`. Twelve tasks is the single-spec ceiling in `/spec-scope` (more than 12 requires a stated reason); no part ships alone, so it is not split.

### 10. Honesty register

| Spec demand | Position |
|---|---|
| v2 §7.1 live qualification of the Codex bundle | Not met: fixtures only; record stays at most `fixture-tested` (N1, OQ3). |
| v2 §7.2 Codex app-server and native-managed sign-in facts | Deferred: unverifiable offline (OQ1, OQ6, OQ10); decoder and key use defaults and synthetic frames. |
| v2 §7.3 entitlement and no-paid-continuation | Not met for Codex: strict admission blocks (OQ2). |
| v2 §4.4 negotiated optional interfaces (inspect-liveness, reconnect, resume, steer, approval response) | Reported as capability facts only; no methods added (N4). |
| v2 §4.4 `PreparedAttempt` one-use launch token | Out of scope here; follows the supervisor specs. |
| v2 §6.4 native stop acknowledgement | `Acknowledged` is `unknown` (OQ9). |
| v2 §7.1 matched direct-native fidelity comparison (AT-02 L) | Not met: no live comparison; fidelity deltas recorded from fixtures only. |
| G07 / AT-47 restricted and inspect profiles for Codex | Deferred: refused (N7). |
| Windows process tree for Codex (G04 on Windows) | Deferred: `unknown` (N6, MH-11). |
| G10 public claim | `docs/limitations.md` states Codex is planned/fixture-tested only (NFR-3). |
| No Codex run is admittable in this spec | Unmet by design (D13): strict gap gate, unknown auth category, no live record; admission legs are tested at key level only. |
| AT-11 startup before trust | Met in order (inventory and trust precede the probe), but the enumerated sources are defaults (OQ6) and everything beyond them is a blocking gap. |
| v2 §7.3 purchased-credit prevention / AT-13 exhaustion for Codex | Unmet by design (D6, D7): usage is nil and exhaustion is never classified until OQ10 supplies shapes; the purchased-credit check is a Codex-only rule that no fixture-derived record can satisfy. |
| AC-6.1 effort metadata | Not met: `SessionStarted` has no effort field and `ObservedKey.EffortSettings` is `unknown`; capturing effort needs a seam field in a later spec. |
| Process ownership / billing-route decisions D10, D12 | Recorded in the Task 12 decision record. |
| Runtime route check (v2 §7.3 / AC-4.4 of the Claude spec) | Absent for Codex (D12): `APIKeySource` is not reported; the route rests on the admission key. |
| `StructuredEvents` for Codex | Reported `unknown`: the synthetic schema (D5) proves the decoder, not what the executable emits. |
| AT-02 fidelity deltas | Recorded `unobservable` (D16); none `preserved` without a direct-native comparison. |
| AC-3.3 drift invalidation, AC-4.2 route list | The Codex-specific legs are the digest change and a fixture route list with no production caller (D4); invalidation itself uses the existing registry drift path, covered by `internal/qualify` tests. |
| Windows launch for Codex | Refused with `ErrCapability` (N6). |
