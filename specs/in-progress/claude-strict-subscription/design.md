# Claude Strict Subscription — Design

Normative source: mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md, version 2.0.

MH-10's registry (`specs/*/qualification-registry/`, approved, landing) is
the specified contract this design builds on: `qualify.Key`,
`qualify.Record` with `proven | not-proven | unknown` columns,
`Registry.Lookup/Consult/Record/InvalidateOnDrift`, `ResolveQualification`
with its §4 key table, and the `authorised-live` evidence method reserved
for this spec.

## 1. Current state

- Strict `--billing subscription-only` blocks twice: `Decide` early-returns
  `entitlement_qualification_unavailable` before dispatch
  (`internal/admission/admission.go:189-191`), and `ResolveBilling` blocks
  the same way (`internal/admission/billing.go:34-36`).
- The declared posture records `Qualified:false`,
  `paid_continuation:unknown`, `g05:not-passed`
  (`internal/admission/billing.go:52`); the apply gate requires
  `--accept-unverified` for unverified candidates
  (`internal/supervisor/apply.go:143-144`).
- Auth evidence is non-secret status: `AuthEvidence{LoggedIn, AuthMethod,
  APIProvider, SubscriptionType, ConfigDirectory, IdentityRef}`
  (`adapters/claudecode/probe.go:146-153`), read via `AuthStatus`
  (`adapters/claudecode/probe.go:157`), which refuses a changed executable
  (`probe.go:161-164`).
- Native config inventory is `claudecode.Manifest{Digests, Digest, Hooks,
  MCPServers, RequiresTrust}` (`adapters/claudecode/settings.go:27-33`)
  with `Digest` = hex sha256 over marshaled digests
  (`settings.go:228-233`); admission converts it at
  (`internal/admission/admission.go:321`) after inventory in
  (`internal/admission/native.go:33`). This spec extends the manifest
  with `EnabledPlugins []string` (sorted names from the `EnabledPlugins`
  map keys at `settings.go:52`, true entries only); `RequiresTrust`
  keeps its current triggers (enabled plugin, `Hooks > 0`, or any MCP
  server — `settings.go:217-226`).
- The effective-configuration source gap is open: remote cached managed
  policy and macOS MDM preferences are not certified by the file inventory
  (ADR-0002, `docs/decisions/0002-dogfood-billing-posture.md:80-85`).
- Live-test precedent: `//go:build live`, env-gated
  (`MYTHHELM_LIVE_CLAUDE=1`), maintainer-invoked, never in CI
  (`adapters/claudecode/live_test.go:1-24`).

## 2. Effective-configuration evidence (v2 §7.3, §8.2; FR-1)

New `adapters/claudecode/effective.go` (adapters domain):

```go
// EffectiveConfig is the established effective configuration for one
// probed route: credential precedence, managed policy, children routes,
// extra-usage settings and purchased-credit consumability, each traced to
// its non-secret source.
type EffectiveConfig struct {
    CredentialPrecedence []string // ordered route names, e.g. ["native-login"]
    ManagedPolicy        PolicySummary
    ChildrenRoutes       []string // known native child/subagent routes
    ExtraUsage           string   // "disabled" (assessed live) or "unknown"
    PurchasedCredits     string   // assessed live: "non-consumable", "plan-granted",
                                  // "separately-purchased", or "unknown"
    Sources              map[string]string // field → status/config source, "unknown" when none
}

// PolicySummary is the effective managed policy relevant to billing.
type PolicySummary struct {
    Digest      string   // sha256 over the inventoried managed sources
    Sources     []string // inventoried source names
    Gaps        []string // applicable sources NOT inventoried (ADR-0002 gap)
}

// InventoryEffective builds the effective configuration from a probe,
// manifest and auth evidence. It performs no live call and reads no secret.
// Static inputs provably lack live signals (Probe, Manifest and AuthEvidence
// carry no funding, extra-usage or credit fields), so the static inventory
// always reports ExtraUsage "unknown" and PurchasedCredits "unknown"; only
// the authorised live suite assesses them (below).
func InventoryEffective(p adapter.Probe, m Manifest, ev AuthEvidence) (EffectiveConfig, error)

`CredentialPrecedence` names the observed method's route as a
single-element list (`["native-login"]` for native login; static inputs
carry no second method to order). `ChildrenRoutes` is empty from static
inputs (no child signal exists there; child-route coverage lives in the
auxiliary inventory, §3).

// UnresolvedSources is the production list of gap sources Q1 has not
// resolved. Q1's resolution lands as an implementation PR editing this
// list — removing a source only with (a) new inventory code covering it
// or (b) a scoping rationale amending ADR-0002. AC-1.3's "the maintainer
// has not resolved the source gap" is exactly membership in this list.
var UnresolvedSources = []string{"managed-remote-cache", "macos-mdm-policy"}

// GapSources lists gap sources that apply to this platform and are
// still unresolved: "managed-remote-cache" iff present in unresolved
// (its location is unknown, so absence is unprovable);
// "macos-mdm-policy" iff present in unresolved and runtime.GOOS ==
// "darwin". Output follows the order of the unresolved input, so the
// first gap — the one named in `missing-source:` — is deterministic.
// Plugins are not a gap: enabled plugin names are inventoried in
// Manifest.EnabledPlugins, and their unknown funding blocks via AC-2.2
// instead. MCP servers are identity-inventoried (no gap; same AC-2.2
// rule). No manifest input: no current leg depends on manifest
// properties. Pure over (GOOS, unresolved) — no file reads, fully
// CI-testable with explicit lists.
func GapSources(unresolved []string) []string
```

Production callers (`InventoryEffective`, `decideClaudeCode`) pass
`claudecode.UnresolvedSources`; tests pass explicit lists. The admit
path is unreachable while any listed source applies, so the admit-path
acceptance (Tasks 3/5/7) sequences behind Q1: unit-level record-branch
tests use explicit lists, and the end-to-end admit proof runs only after
Q1's implementation edits the list.

Failure cases: unusable probe/executable (`ErrCapability`-wrapped,
matching `probe.go:163`); empty managed-source inventory where managed
sources apply → `Gaps` non-empty (not an error — the gap is data).
`Gaps` carries the ADR-0002 open sources until Q1 resolves; any non-empty
`Gaps` keeps the entitlement column `not-proven` (AC-1.3).

Fixtures under `adapters/claudecode/testdata/effective/` pin one inventory
per managed-source shape (none, file-only, file+gap), each marked
synthetic.

## 3. Auxiliary and descendant coverage (v2 §7.3; FR-2)

`adapters/claudecode/auxiliary.go`:

```go
// AuxiliaryRoute is one model-using route that shares the funding tree.
type AuxiliaryRoute struct {
    Name     string // AT-03 seven, "plugin:<name>", "plugin:unknown", or "mcp:<server>"
    Funding  string // "included" (proven), "unknown"
    Evidence string // evidence id, "plugin-count:<n>", or "unknown"
}

// InventoryAuxiliary lists every known model-using route for the first-route
// surface. Enabled plugin names are inventoried in
// Manifest.EnabledPlugins (Task 1); each yields one `plugin:<name>`
// route. When the session reports plugins the manifest did not name
// (`s.PluginCount` exceeds the named routes), the surplus collapses to
// one `plugin:unknown` route with `plugin-count:<n>` provenance (the
// manifest and `SessionStarted` carry no finer identity —
// `PluginCount` only, internal/adapter/adapter.go:255). MCP yields
// server-level `mcp:<server>` routes. All plugin/MCP routes carry
// `Funding: "unknown"`: names identify the route, but only the live
// suite can prove funding, and no live plugin-funding proof exists in
// this spec. Unknown routes are listed as unknown, never omitted
// silently.
//
// Name construction: the seven AT-03 names are string constants;
// `<name>` keeps the manifest's map key verbatim; `<server>` keeps the
// manifest's `"source:name"` form verbatim (settings.go:245).
// Zero-case rule: no `plugin:*` route emits when both the manifest
// names list and `PluginCount` are empty; `mcp:<server>` emits only
// for listed servers. A bare install yields exactly the AT-03 seven —
// no phantom zero-count route ever blocks it.
//
// Funding assessment is a live-suite observation, not a static inference:
// the static inventory always reports `Funding: "unknown"`; the authorised
// live suite exercises each route and records `included` only when the
// route completes on the included plan with no paid auxiliary (provider or
// native enforcement observed). `LiveRecord` gates
// completeness (refuses any remaining `unknown`) but never observes.
func InventoryAuxiliary(m Manifest, s adapter.SessionStarted) []AuxiliaryRoute

// LiveRecord builds the complete live-qualified first-route record from
// assessed inputs (the live suite observes; this constructor gates and
// assembles — pure, CI-testable). Production recording goes through
// Registry.Record by the maintainer-run live suite (Task 7).
func LiveRecord(key qualify.Key, cfg EffectiveConfig, aux []AuxiliaryRoute) (qualify.Record, error)
```

`LiveRecord` refuses (`claudecode.ErrNotFirstRoute`, MH-10 Task 5) unless
the key matches the first-route predicate (`Harness == "claude-code" &&
Surface == "native-cli-structured (print, stream-json)"`, digests ignored
— MH-10's `RecordDraft` predicate, restated here); refuses with
`ErrEvidenceIncomplete` when any `Gaps` entry or `unknown` funding
remains, when `ExtraUsage != "disabled"`, or when `PurchasedCredits` is
neither `"non-consumable"` nor `"plan-granted"` (AC-2.2/AC-2.3).
`"plan-granted"` (credits the plan itself grants with established
provenance, per the v2 §7.3 allowance) is included funding, not paid
continuation; `"separately-purchased"` and `"unknown"` refuse. On
success it returns `Progress: live-qualified` with the entitlement
column `proven`, one `authorised-live` evidence entry (`Suite:
"live-qualify:<native-version>"`, `Label: Observed`, `Source:
"live-qualify"`), per-dimension `Capabilities` entries
(`credential-precedence`, `managed-policy`, `extra-usage`,
`purchased-credits`, each `supported` with the evidence id), an unknown
`Quota` datum (AC-3.3: unknown quantity), and the
`stop_at_exhaustion` capability `supported` by `documented-mechanism`
evidence citing the vendor's exhaustion behavior. Purchased-credit
consumability is established from the native account state the authorised
live suite observes; fixtures pin every outcome.

## 4. Strict admission flip (v2 §7.3; FR-3)

`internal/admission/qualify.go` (MH-10 Task 4, consumed here):

- `Decide`'s early return (`internal/admission/admission.go:189-191`) is
  removed: it runs before probing, so no observed key exists to consult
  with — conditioning it there is unimplementable. The post-probe consult
  in `decideClaudeCode` (MH-10 wiring: after `AuthStatus`, before
  `ResolveBilling`) fails closed instead, in this pinned order:
  1. Open the registry. Permission/IO/corruption errors → admission
     error in every mode — broken infrastructure outranks
     data-driven blocks.
  2. MH-10's nil-registry cases — a missing registry or a
     schema-mismatched one (`IsSchemaMismatch`) — take the
     absent-record path with no gap check (there is no consult to
     qualify): strict `Blocked` with `no_qualification_record`,
     declared `Eligible` per MH-10 (whose
     `TestConsultSchemaMismatchMapsAbsent` pin is preserved
     unchanged).
  3. Registry present + strict mode → the gap gate (below), then the
     registry lookup.
  4. Registry present + declared mode → the MH-10 lookup/bypass
     unchanged: the gap gate never fires for declared billing.
  Nothing admits past a broken registry.
- `decideClaudeCode` consults via `ResolveQualification` first: any
  non-`Eligible` verdict returns its `*BlockedError` immediately
  (short-circuit — `ResolveBilling` never runs, so per-reason codes
  `no_qualification_record`, `entitlement_not_proven`,
  `ambiguous_qualification_match` and `qualification_drifted` survive).
  `Unsupported` maps across billing modes per MH-10 (unsupported record or
  harness); the declared bypass covers missing or merely non-live records
  only.
  On `Eligible` it calls `ResolveBilling` with the eligibility attached:
  `ResolveBilling(ctx, mode, evidence, decl, elig Eligibility)` (new
  trailing param; dogfood's three call files — `admission.go:311`,
  `billing_test.go`, `auth_test.go` — updated in Task 3).
  Gap-bearing strict consults block with Code `entitlement_not_proven`
  and Field `missing-source:<first-gap-source>`, satisfying AC-1.2's
  source-naming. Gaps are assessed at consult time, not stored in records
  (no specified writer ever stores a gap-bearing record — the constructor
  refuses them): `decideClaudeCode` calls
  `claudecode.GapSources(claudecode.UnresolvedSources)` and, in strict
  mode only, blocks when non-empty, before the registry lookup —
  `missing-source:managed-remote-cache`, first in unresolved order on
  every OS. Admission already imports the adapter package, so no new
  dependency arises. Pre-Q1 every strict consult against a present
  registry blocks here — the gap gate is the specified behavior until
  Q1's implementation edits the list. Consequences: pre-Q1, non-gap strict
  codes against a present registry are provable only below the full
  path (direct `ResolveQualification`); full-path strict proofs other
  than the gap block and the missing-registry `no_qualification_record`
  (missing skips the gap check) wait for Q1.
  Matching runs through `Registry.Consult` with MH-12's narrowing for
  the live path: exact match on AuthCategory, ProviderEndpoint,
  WorkspaceClass, EffortSettings and ModelSnapshot (MH-10 §9's set) in
  addition to the stable set, before drift comparison — closing MH-10's
  registered cross-identity limitation (honesty register).
  `ResolveBilling`'s strict branch re-checks `elig.Verdict == Eligible` as
  defense-in-depth (dead at system level behind the short-circuit, pinned
  by a direct unit test).
- `ResolveBilling`'s strict branch (`internal/admission/billing.go:34-36`)
  admits iff `elig.Verdict == Eligible` for a `live-qualified`/`proven`
  record, recording exactly:
  ```go
  BillingPosture{
      Mode: BillingSubscriptionOnly,
      CredentialProvenance: "native-login",
      EntitlementClass: "included-plan",
      EntitlementSource: "registry:live-qualified",
      PaidContinuation: "prevented",
      PaidContinuationUserDeclaration: "",
      Qualified: true,
      G05: "passed",
  }
  ```
  Any other strict case blocks with `entitlement_qualification_unavailable`
  (defense-in-depth fallback, dead behind the short-circuit — AC-3.2's
  live codes come from the consult above); the strict branch ignores
  any declaration
  (`resolveDeclaration` in `internal/admission/native.go:153-166` returns
  non-nil whenever the flag is passed, regardless of mode) and records
  `PaidContinuationUserDeclaration: ""` unconditionally.
- Drift: a drifted observed key → `Blocked` with the drift reason and the
  invalidation is persisted through the Task 4 writer (AC-3.3); strict
  admission blocks again until re-tested.

`BillingPosture` gains no new mode: strict stays `subscription-only`, now
capable of passing. The `subscription-declared` path is untouched (FR-4).

## 5. Persisting drift invalidations (AC-3.3, AC-4.1 lineage)

MH-10 ships `InvalidateOnDrift` with no production caller and notes MH-12
as the first live writer. The writer is `internal/cli/run.go`: when strict
admission blocks with a drift reason, `run` opens the registry read-write
and records the invalidation revision for the matched record. Rules:

- Best-effort after the block: a persist failure is logged to stderr and
  never masks or lifts the block.
- Only drift blocks write, and only the matched record's revision; no other
  failure path writes.
- The consult itself stays read-only (admission writes nothing).

The cross-boundary carrier is a typed error (Task 3, in
`internal/admission/qualify.go`): `run.go` cannot reconstruct the observed
key (probe/manifest/evidence live inside admission), so the decision
carries the matched record's hash out:

```go
// DriftError reports a drift block with the data needed to persist the
// invalidation. It unwraps to the *BlockedError so exit mapping is unchanged.
type DriftError struct {
    Blocked *BlockedError // Code "qualification_drifted"
    KeyHash string        // qualify.KeyHash of the matched record
    Reason  string        // drift reason naming the field
}
func (e *DriftError) Error() string { return e.Blocked.Error() + ": drift " + e.Reason }
func (e *DriftError) Unwrap() error { return e.Blocked }
```

`decideClaudeCode` returns `&DriftError{...}` for the drift path. The
`Code: "qualification_drifted"` pin is MH-12's (MH-10 left drift reasons
unpinned strings); it extends MH-10's reason set without changing it.
Persistence runs through a hash-keyed API (Task 3, in
`internal/qualify/registry.go`):

```go
// InvalidateByHash stores an invalidation revision for the current record
// with the given key hash. Unknown hash → error (qualify: unknown key
// hash: ...); corrupt JSON → the DecodeRecord error. It never creates a
// record.
func (r *Registry) InvalidateByHash(ctx context.Context, keyHash, reason string) error
```

`run.go` (Task 4) matches with `errors.As` (never a type assertion —
admission wraps) and calls a helper at all three `Decide` sites (TUI at
`run.go:98`, accessible at `run.go:108`, `executeRun` at `run.go:130`,
shared with `demo`): the helper no-ops unless `errors.As` finds a
`*DriftError`, opens the registry read-write, and calls
`InvalidateByHash`. Unknown-hash and persist failures are logged to
stderr; the block stands either way. (`demo.go`'s two `Decide` sites at
lines 111/126 need no helper: hardcoded `fake`/`local-scripted` at
`demo.go:98-102` makes the `decideClaudeCode` drift path unreachable.)

## 6. Declared path preserved (FR-4)

No code change to the declared path is planned; this spec pins it with
guards:

- `TestDeclaredLabelsUnchanged`: the declared `BillingPosture` still
  records `qualified:false`, `paid_continuation:unknown`, `g05:not-passed`.
- Label-distinctness sweep: admission JSONL output, journal
  `runs.billing_posture` rows, receipts and `runs` output never label a
  declared posture qualified/verified/strict (AC-4.2 surfaces from
  refine).
- End-to-end: the declared dogfood flow admits exactly as before (exit
  codes, labels, receipt fields).

## 7. Live suite and fixtures (v2 §7.1; FR-2, FR-3)

- Unit/fixture coverage (CI, no credentials): every AC has a named test;
  fixtures under `adapters/claudecode/testdata/qualification/`
  (created by MH-10 Task 5, extended here by Task 2 with
  `live-shapes.json`: proven record JSON, drifted variants,
  purchased-credit variants).
- Live suite (`adapters/claudecode/live_qualify_test.go`, `//go:build
  live`, env `MYTHHELM_LIVE_QUALIFY=1`): runs the authorised entitlement
  proof against the real native CLI with disposable fixtures, then records
  the `live-qualified` evidence through `Registry.Record`. Never in CI;
  never automated; consumes the maintainer's allowance only under their
  explicit invocation (v2 §7.1, I04).
- A `maintainer` task (Task 7) runs the live suite and attaches the
  sanitized evidence; the run hands it over when ready and continues with
  everything not depending on it. The strict-admits e2e (Task 5) runs
  against a seeded live record in CI (test-side seeding, as MH-10 Task 9
  divides trigger from display) after Q1 resolves; the maintainer task
  proves the real one. Pre-Q1 the gap block, the missing-registry
  `no_qualification_record`, and the declared path (which skips the
  gap check) are provable end to end, which Task 3 pins at the
  `cli.Main` level; every other strict code against a present
  registry is pinned below the full path until Q1.

## 8. Decisions

| ID | Decision | Rationale |
|---|---|---|
| D1 | Remove the `Decide` early return; the post-probe consult fails closed | The early return runs before probing, so no observed key exists to consult with. The `decideClaudeCode` consult (missing → absent → strict `Blocked`; unreadable → admission error) blocks everything the early return blocked, and only a live-proven record passes. MH-10's e2e guards are updated where they pinned the early return's reason. |
| D2 | Drift-invalidation writer lives in `run.go`, best-effort after the block | Admission writes nothing (invariant); blocked runs have no supervisor; the CLI owns the failure path. Best-effort so a persist failure never masks the block. |
| D3 | New `MYTHHELM_LIVE_QUALIFY=1` gate, separate from `MYTHHELM_LIVE_CLAUDE` | The canary consumes allowance for launch proof; qualification consumes allowance for billing proof. Separate gates let the maintainer grant each independently (I04 least authority). |
| D4 | `Gaps` is data, not an error | An uninventoried source is a known unknown (I09); returning it as data lets the record state exactly what is missing and lets AC-1.3 block on it honestly. |
| D5 | Name-specific auxiliary routes; live record covers the bare config | Enabled plugin names are inventoried in `Manifest.EnabledPlugins`, so each yields a `plugin:<name>` route (`plugin:unknown` only for session-reported surplus); MCP keeps server-level routes. Names identify but do not fund: all carry `unknown` funding. The earned live record covers the inventoried bare config (AT-03 seven, funding assessed live); plugin/MCP-present configs stay blocked per AC-2.2 until a future spec proves plugin funding. |
| D6 | Strict admits through the same `Consult` + key table as MH-10 | One matching rule, tested by MH-10's agreement test (`TestResolveQualificationFindsRecordDraft`, MH-10 Task 4); MH-12 adds the live-record branch plus exact non-drift-dimension narrowing (§4), not a second matcher. |
| D7 | Gaps assessed at consult time, never stored | No writer stores gap-bearing records (the constructor refuses them), so a stored-gap encoding would have no producer. Gaps are observed-config properties; recomputing them each consult keeps records verifiable snapshots. |
| D8 | Q1's resolution threads through `UnresolvedSources`; admit-path acceptance sequences behind it | An out-of-band decision cannot gate code. AC-1.3's "resolved" is exactly removal from the `UnresolvedSources` list by Q1's implementation PR (inventory or scoping rationale required), so the gate is testable: pre-Q1 strict consults against a present registry block with `missing-source`, post-Q1 they reach the registry. Unit record-branch tests use explicit lists; the e2e admit proof waits for Q1. Q1's PR rewrites the pre-Q1 gap pins it invalidates (Task 3's `missing-source` expectations, `TestUnresolvedSourcesDefault`) to their post-Q1 codes. |

## 9. Honesty register

| Spec demand | Position |
|---|---|
| v2 §7.2 Claude focus: full effective managed policy | Partially met: file-inventoried sources proven; ADR-0002 gap sources (remote cached policy, macOS MDM) stay in `Gaps` and block liveness until Q1 resolves. |
| v2 §7.1 authorised-live evidence | Met only when the maintainer runs Task 7; without the grant, the record stays `fixture-tested` and strict keeps blocking. |
| v2 §7.3 purchased-credit non-consumability | Proven through the authorised live suite's account observation; fixtures pin every outcome for CI. |
| v2 §7.3 credit assessment over time | Point-in-time only: consumability is assessed once at live-record time; a later credit purchase has no consult-time carrier (`AuthEvidence`/config digest do not observe it). The record stands until drift or a maintainer re-run; no automatic re-assessment is specified. |
| v2 §7.3 plan-granted allowance | `plan-granted` credits count as included funding (not paid continuation), with the authorised-live observation as the provenance. Inference: the master allows them "with established provenance"; this spec treats the suite's account observation as that provenance. |
| AT-03 all-routes pass | Met for the inventoried route set; a route outside the inventory blocks rather than passes silently. |
| v2 §7.3 plugin/MCP-present liveness | Not met: name-specific routes still carry `unknown` funding, so plugin/MCP-present configs stay blocked per AC-2.2. The earned pass covers the bare config only (D5). |
| MH-10 stable-matching limitation (§9 row) | Must be closed here for the live path: exact match on non-drift dimensions is required before any live record is consulted (Task 3). |
| MH-10 restore-without-retest question | Closed here: drift persists an invalidation revision (Task 4), so a restored state re-admits only after re-test re-proves it. |

## 10. Why one spec

Seven tasks, one work stream: the evidence constructors without the
admission flip prove nothing end to end; the flip without the evidence has
nothing to admit; the live suite without both has nowhere to record. The
maintainer live run and the docs consume the same contract but deliver
nothing alone (`/spec-scope`: single spec, recorded with this rationale).
