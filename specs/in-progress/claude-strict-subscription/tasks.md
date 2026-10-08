## Claude Strict Subscription — Tasks

### Dependencies

- Prerequisite: `specs/*/qualification-registry/` (MH-10) shipped on origin/main: `internal/qualify/` (`Key`, `Record`, `Consult`, `Record`, `InvalidateOnDrift`), `internal/admission/qualify.go` (`ResolveQualification`), `adapters/claudecode` (`ErrNotFirstRoute`). No task starts until that merge lands.
- Order is contract-first: Task 1 ships the evidence types and gap assessment; Task 3 flips admission against Task 1 plus MH-10's shipped APIs. Runnable sets: {1} first; {2, 3} after 1 (disjoint Files); {6} after 3; {4} after 3 and Q1 resolved; {5} after {3, 4} and Q1 resolved; {7} after {2, 3} and Q1 resolved (maintainer task, handed to the operator). No two tasks share a file except through the stated dependencies.
- **Q1 sequencing.** Tasks 1–3 and 6 proceed pre-Q1: the strict gap gate fires with `missing-source` (pinned at `cli.Main` level), non-gap strict codes are pinned below the full path (direct `ResolveQualification`), and the declared path skips the gap check (pinned end to end). Tasks 4, 5 and 7 need Q1's implementation (the `UnresolvedSources` edit): Task 4's drift runs never reach the writer while the gap gate fires, Task 5's admit e2e runs the production binary against the edited list, and Task 7's `LiveRecord` refuses any `Gaps`. The run holds those three until Q1 lands. Q1's PR rewrites the pre-Q1 gap pins it invalidates (D8); missing-registry pins (`no_qualification_record`) hold across Q1 because missing skips the gap check.
- **Gates for every task.** `gofmt -w` on touched Go files first, then from the tree root `gofmt -l .` (must print nothing), `go vet ./...`, `go test -race ./...`, `go mod tidy -diff` (must print nothing), and `scripts/ci/check-public-hygiene.sh`. Live-tagged tests are env-gated and skip in CI. A task is not complete because files exist or an agent reported success; cite the runner output.
- **Completion convention.** Append ` ✅ COMPLETED` to the heading, keep every original field, and add `Status` (`✅ Completed — …; PR #<n>`), `Implementation` (at most 3 lines, plus the commit SHA), `Spec deviations` (`None`, or each deviation with its reason) and `Files modified`. Every task appends a scratchpad note under Discoveries.
- **Commits.** Every commit is signed off (`git commit -s`, DCO). The billing change (Task 3) carries its decision record ADR-0012 (Task 6), since the flip itself stales ADR-0002 item 1 ("always blocks"); a further ADR-0002 amendment lands if Q1 resolves the source gap.

---

## Implementation Tasks

### Task 1 — Effective-configuration inventory

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: None
- **Change**: Add the effective-configuration inventory (credential precedence, managed policy, children routes, extra usage, source gaps) built from probe, manifest and auth evidence with no live calls and no secret reads; collect enabled plugin names into the manifest; add the unresolved-source list and gap assessment.
- **Files**:
  - `adapters/claudecode/effective.go`
  - `adapters/claudecode/effective_test.go`
  - `adapters/claudecode/settings.go` (`Manifest.EnabledPlugins` collection)
  - `adapters/claudecode/settings_test.go` (new; via `InventoryAdmittedProject` with synthetic blobs)
  - `adapters/claudecode/testdata/effective/none.json`
  - `adapters/claudecode/testdata/effective/file.json`
  - `adapters/claudecode/testdata/effective/file-gap.json`
- **Produces**: `claudecode.EffectiveConfig{CredentialPrecedence, ManagedPolicy, ChildrenRoutes, ExtraUsage, PurchasedCredits, Sources}`; `claudecode.PolicySummary{Digest, Sources, Gaps}`; `claudecode.InventoryEffective(p adapter.Probe, m Manifest, ev AuthEvidence) (EffectiveConfig, error)`; `claudecode.Manifest.EnabledPlugins []string` (sorted, true entries only); `claudecode.UnresolvedSources` (`[]string{"managed-remote-cache", "macos-mdm-policy"}`); `claudecode.GapSources(unresolved []string) []string` (output in unresolved-input order).
- **Acceptance**:
  - `TestInventoryEffectiveNone`: a manifest with no managed sources yields `Gaps` in unresolved-list order — `managed-remote-cache` first, plus `macos-mdm-policy` on darwin only — and `ExtraUsage`/`PurchasedCredits` `"unknown"` (static inputs carry no live signal — `AuthEvidence` has no extra-usage or credit field); a variant hiding a managed source fails.
  - `TestInventoryEffectiveGapIsData`: the file+gap fixture's `Gaps` contains `managed-remote-cache` (not an error — the gap is data); a variant asserting empty `Gaps` fails.
  - `TestInventoryEffectiveFileOnly`: the file-only fixture yields a `PolicySummary` whose `Gaps` holds only the default unresolved sources for the platform, in order, and a digest recomputing over its sources; a dropped source changes the digest.
  - `TestInventoryPrecedenceAndChildren`: the native-login fixture yields `CredentialPrecedence == ["native-login"]` and empty `ChildrenRoutes` (no static child signal; children are covered by the Task 2 auxiliary inventory); an empty precedence fails.
  - `TestManifestCollectsPluginNames`: settings blobs with enabled plugins yield sorted `EnabledPlugins` names; disabled (`false`) entries are ignored; `RequiresTrust` keeps its current triggers (enabled plugin, hooks, or MCP).
  - `TestGapSourcesRules`: `managed-remote-cache` listed iff present in the unresolved input; `macos-mdm-policy` listed iff present and `runtime.GOOS == "darwin"`; output in unresolved-input order (reversed input reverses the output); plugins never a gap leg (names are inventoried); an empty unresolved input yields no gap.
  - `TestUnresolvedSourcesDefault`: the production default contains exactly `managed-remote-cache` and `macos-mdm-policy`; emptying it without Q1's amendment fails.
  - `TestInventoryReadsNoSecrets`: with `HOME` pointed at an empty temp dir and proxies poisoned (MH-10 Task 5 pattern), output is byte-identical to the unpoisoned run.
  - `TestInventoryRefusesBadProbe`: a probe with a non-absolute executable returns a `ErrCapability`-wrapping error and no config.
  - `TestInventoryLiveSignalsAlwaysUnknown`: every fixture yields `ExtraUsage: "unknown"` and `PurchasedCredits: "unknown"` — static inputs carry no live signal; only the live suite assesses them.
- **Test plan**: Synthetic JSON fixtures (marked synthetic); hermetic execution; table tests over managed-source shapes.
- **Invariants touched**: I02 (v2 §7.3: unknown mandatory sources listed, never assumed); I19 (v2 §7.1: no secret reads); I09 (v2 §7.3: gaps are explicit unknowns).

### Task 2 — Auxiliary inventory and live evidence constructors

- **Domain/agent**: go-implementer
- **Budget**: complex (live suite + several constructors)
- **Depends on**: Task 1
- **Change**: Add the auxiliary/descendant route inventory and the authorised-live entitlement evidence constructor, plus the env-gated live suite that records the first live proof.
- **Files**:
  - `adapters/claudecode/auxiliary.go`
  - `adapters/claudecode/auxiliary_test.go`
  - `adapters/claudecode/live_qualify_test.go`
  - `adapters/claudecode/testdata/qualification/live-shapes.json`
  - `adapters/claudecode/COMPATIBILITY.md` (live-record section for the first route)
- **Produces**: `claudecode.AuxiliaryRoute{Name, Funding, Evidence}`; `claudecode.InventoryAuxiliary(m Manifest, s adapter.SessionStarted) []AuxiliaryRoute`; `claudecode.LiveRecord(key qualify.Key, cfg EffectiveConfig, aux []AuxiliaryRoute) (qualify.Record, error)`; `claudecode.ErrEvidenceIncomplete`.
- **Acceptance**:
  - `TestAuxiliaryCoversAT03`: inventory covers the seven AT-03 routes plus one `plugin:<name>` route per manifest name and one `mcp:<server>` route per manifest server, all with `Funding: "unknown"`; a session-reported surplus count yields one `plugin:unknown` route with `plugin-count:<n>` evidence; dropping a route or omitting a manifest name fails the test.
  - `TestLiveShapesCoverVariants`: `live-shapes.json` holds the proven-record, drifted-variant and purchased-credit cases (including `plan-granted`); dropping any case fails the test.
  - `TestLiveRecordRefusesIncomplete`: any `Gaps` entry, `unknown` funding, `ExtraUsage != "disabled"` or `PurchasedCredits` of `"separately-purchased"`/`"unknown"` returns `ErrEvidenceIncomplete` and no record (one case each).
  - `TestLiveRecordRefusesOtherKeys`: a non-first-route key returns `ErrNotFirstRoute` (`errors.Is`); the first-route predicate agrees with `RecordDraft` (accepted-by-one ⟺ accepted-by-both over key mutations).
  - `TestLiveRecordSuccessPath`: a complete assessed fixture (no gaps, all funding `included`, `ExtraUsage: disabled`, `PurchasedCredits: non-consumable`) yields a `live-qualified` record with entitlement `proven`, one `authorised-live` evidence entry, the four dimension capabilities `supported`, unknown `Quota`, and `stop_at_exhaustion` supported by `documented-mechanism` evidence.
  - `TestLiveRecordAcceptsPlanGranted`: the same complete fixture with `PurchasedCredits: plan-granted` yields the `live-qualified` record (plan-granted credits are included funding, not paid continuation).
  - `TestLiveQualifySkipsWithoutGrant`: without `MYTHHELM_LIVE_QUALIFY=1` the live suite skips (never fails, never calls); with a bogus grant value it still skips.
  - `TestLiveQualifyEntitlement` (`//go:build live`, env-gated): under `MYTHHELM_LIVE_QUALIFY=1` runs the authorised entitlement proof with disposable fixtures and records the evidence through `Registry.Record`; without the grant it skips.
  - `TestCompatibilityLiveSection`: the `## Live record` section of `adapters/claudecode/COMPATIBILITY.md` (match scoped to that heading) names the evidence method `authorised-live` and the Task 7 maintainer run; removing the section fails the test.
  - `TestAuxiliaryBareInstallEmitsOnlyAT03`: zero plugins and no MCP servers yield exactly the seven AT-03 routes — no `plugin:*` route, no `mcp:` route; a phantom zero-count route fails the test.
- **Test plan**: Unit tests always run; `live_qualify_test.go` carries `//go:build live` and an env gate (MH-10 canary pattern); fixtures pin both purchased-credit outcomes.
- **Invariants touched**: I15 (v2 §7.3: proof across the whole funding tree); I04 (allowance consumed only under explicit grant); I13 (contributor tests need no credentials).

### Task 3 — Strict admission flip with closed matching

- **Domain/agent**: go-implementer
- **Budget**: complex (billing decision + persistence-adjacent matching)
- **Depends on**: Task 1 (consumes `GapSources`/`UnresolvedSources` from Task 1's `effective.go`, plus MH-10's shipped APIs)
- **Change**: Remove the `Decide` early return, admit strict billing on a live-qualified/proven record, and close MH-10's stable-matching limitation with exact non-drift-dimension matching for the live path.
- **Files** (9: the flip breaks MH-10's e2e guard the moment it lands, so the guard update ships in this task rather than leaving main red until Task 5; every addition past the core seven is a mechanical update):
  - `internal/admission/admission.go`
  - `internal/admission/billing.go`
  - `internal/admission/billing_test.go` (new `elig` param call sites + declared-label guard + dogfood strict-test retirement)
  - `internal/admission/qualify.go`
  - `internal/admission/qualify_test.go`
  - `internal/qualify/registry.go`
  - `internal/qualify/registry_test.go`
  - `adapters/claudecode/auth_test.go` (mechanical `elig` param call-site update; cross-domain exception — same agent, test-only, keeps the tree compiling within this task)
  - `tests/e2e/qualification_test.go` (MH-10's `TestE2EStrictStillBlocked` re-pinned to the consult reason; Task 5 owns only the new strict e2e file)
- **Produces**: strict `subscription-only` admits iff `ResolveQualification` returns `Eligible` on a `live-qualified` record with entitlement `proven` and matching digests; `Registry.Consult` requires exact match on AuthCategory, ProviderEndpoint, WorkspaceClass, EffortSettings and ModelSnapshot (in addition to the stable set) before drift comparison; `admission.DriftError{Blocked, KeyHash, Reason}` with `Unwrap`; `(*Registry).InvalidateByHash(ctx, keyHash, reason) error`; `ResolveBilling(ctx, mode, evidence, decl, elig Eligibility)`.
- **Acceptance**:
  - `TestStrictAdmitsLiveProven`: direct `ResolveQualification` + `ResolveBilling` (unit level, bypassing the `decideClaudeCode` gap gate): a `live-qualified`/`proven` record for the exact observed key admits strict billing with `Qualified:true, G05:"passed"`. The full-path admit is untestable pre-Q1 (the gap gate fires first); Task 5's e2e proves it post-Q1.
  - `TestStrictGapBlocksEndToEnd`: via `cli.Main` pre-Q1, a complete-proof config against a present registry blocks with `entitlement_not_proven` and `Field: missing-source:managed-remote-cache` on every OS (first in unresolved order); gaps are never stored — the config is gap-bearing, not the record. Q1's PR rewrites this pin to the post-Q1 code (D8).
  - `TestStrictConsultReasonsDirect`: direct `ResolveQualification` (below the gap gate) pins the non-gap mapping — absent record → `no_qualification_record`; fixture/declaration-only evidence → `entitlement_not_proven` without `Field`; drifted record → `qualification_drifted`. Pre-Q1 safe; Task 5 proves each end to end post-Q1.
  - `TestDeclaredIgnoresGaps`: via `cli.Main` pre-Q1, a declared run with a gap-bearing config against a present registry admits exactly as before (exit codes, labels, receipt fields) — the gap gate never fires for declared billing (FR-4).
  - `TestStrictDeclarationOnlyEvidenceBlocks`: sign-in-only evidence with a missing registry blocks via `Decide` with `no_qualification_record` (missing skips the gap check — Q1-proof); declaration-only evidence on a non-live record in a present registry blocks with `entitlement_not_proven`, pinned at direct-`ResolveQualification` level pre-Q1 (AC-3.2).
  - `TestResolveBillingStrictDefenseInDepth`: direct `ResolveBilling` with a non-`Eligible` verdict blocks strict with `entitlement_qualification_unavailable` (unreachable via `decideClaudeCode` behind the short-circuit, pinned against future callers).
  - `TestAuthTestsPassNewSignature`: the existing `auth_test.go` cases pass against the 5-arg `ResolveBilling` with equivalent verdicts; a stale 4-arg call fails compilation.
  - `TestStrictAlwaysBlocksRetired`: dogfood's `TestStrictSubscriptionOnlyAlwaysBlocks` is rewritten to the consult codes (its always-blocks name and old-reason e2e half are gone); the old test name appears nowhere.
  - `TestConsultRequiresNonDriftDimensions`: a record differing only in `AuthCategory` is not matched (`ErrNotFound`, never a clean cross-key match).
  - `TestEarlyReturnGone`: `Decide` with strict billing and a missing registry reaches the consult — blocks with `no_qualification_record`, not the old pre-probe reason (missing skips the gap check, so the pin holds pre- and post-Q1).
  - `TestRegistryUnreadableStillFailsClosed`: via `Decide`, a permission/IO/corruption registry failure (not missing, not schema-mismatched — those take the absent path per §4 step 2) errors the admission — never admits, never gap-blocks (open-first order: infrastructure errors outrank the gap gate).
  - `TestDriftErrorUnwraps`: a drifted direct-`ResolveQualification` consult returns a `*DriftError` whose `Code` is `qualification_drifted` with the matched `KeyHash` and field-naming `Reason`; `errors.As` finds both `*DriftError` and `*BlockedError`, and the exit mapping stays 3.
  - `TestInvalidateByHashRoundTrip`: `InvalidateByHash` on a known hash stores a reset revision; an unknown hash returns `qualify: unknown key hash: ...` and writes nothing.
  - `TestInvalidateByHashCorruptJSON`: a row with corrupt `record_json` returns the `DecodeRecord` error and writes nothing.
  - `TestDeclaredLabelsUnchanged`: the declared `BillingPosture` still records `qualified:false`, `paid_continuation:unknown`, `g05:not-passed` through the new `elig` param.
  - MH-10's `TestE2EStrictStillBlocked` is updated in place (name kept — strict still blocks): it runs against a missing registry and pins `no_qualification_record`, not the removed pre-probe reason; no e2e in `qualification_test.go` pins the old reason (the tree stays green at this task's landing, and the pin holds across Q1 because missing skips the gap check).
  - MH-10's `TestStrictStillBlocksEndToEnd` (in `qualify_test.go`, this task's file) is updated in place, not renamed: same missing-registry setup pinning `no_qualification_record`; the old pre-probe reason appears nowhere in the file.
- **Test plan**: `cli.Main` and direct consult tests with temp state dirs and constructed live records; update MH-10 guard tests in place (including the e2e guard in this task's Files, re-pinned to the consult reason). Re-ground every MH-10 stable-match fixture against shipped MH-10 at implementation (`TestConsultMatchesStableIgnoresDigests`, `TestAmbiguousMatchBlocks`, `TestResolveQualificationFindsRecordDraft`, `TestDriftedRecordBlocks`): where two records differ on a newly-exact dimension, update the fixture to cover both the still-ambiguous and the narrowed case (record as deviation if behavior differs from design).
- **Invariants touched**: I15 (v2 §7.3: strict passes only on proof); I02 (v2 §7.3: unknown mandatory evidence blocks); I07 (evidence bound to the exact probed candidate); I20 (v2 §7.1: invalidation is a new revision).

### Task 4 — Drift-invalidation writer on the run path

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 3
- **Also waits for**: Q1 resolved — pre-Q1 every strict run blocks at the gap gate with a plain `*BlockedError`, so no `DriftError` ever reaches the helper; the full-path drift proofs need the edited list (see Dependencies)
- **Change**: Persist an invalidation revision from `run` when strict admission blocks on drift, best-effort after the block, closing MH-10's restore-without-retest question.
- **Files**:
  - `internal/cli/run.go`
  - `internal/cli/run_drift_test.go`
- **Acceptance**:
  - `TestDriftBlockPersistsInvalidation`: a strict run blocked with `qualification_drifted` leaves a new revision with affected columns reset and progress `blocked`; the old revision's bytes are unchanged.
  - `TestPersistFailureNeverMasksBlock`: with an unwriteable registry, the run still exits 3 with the drift reason and logs the persist failure to stderr.
  - `TestNonDriftBlocksWriteNothing`: blocks with reasons `no_qualification_record`, `entitlement_not_proven` and `ambiguous_qualification_match` (one fixture each) leave the registry byte-identical (hash comparison).
  - `TestRestoredStateReadmitsOnlyAfterRetest`: after invalidation, restoring the matching config still blocks until a new revision re-proves it (no silent re-admission).
- **Test plan**: `cli.Main` tests with temp state dirs and seeded drifted records; hash-before/after for write discipline.
- **Invariants touched**: I20 (v2 §7.1: invalidation is a new revision); I06 lineage (refusal path never lifts the block it reports).

### Task 5 — Declared guards and strict end-to-end

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 3, Task 4
- **Also waits for**: Q1 resolved — starts only after Q1's implementation edits `UnresolvedSources` (see Dependencies)
- **Change**: Pin the declared path unchanged and prove the strict admit/block matrix from the packaged binary.
- **Files**:
  - `tests/e2e/qualification_strict_test.go`
- **Acceptance**:
  - `TestE2EStrictAdmitsSeededLive`: the built binary admits `--billing subscription-only` against a test-seeded live record (test-side seeding; the real proof is Task 7).
  - `TestE2EStrictBlocksVariants`: per-variant codes — absent → `no_qualification_record`, fixture-only → `entitlement_not_proven`, drifted → `qualification_drifted`, two records matching on the narrowed set → `ambiguous_qualification_match` (proves the code survives the short-circuit end to end), sign-in-only (with a seeded non-live record present; without one the code is `no_qualification_record`) → `entitlement_not_proven` — each exiting 3 with its reason in JSONL.
  - `TestE2EDeclaredUnaffectedStrict`: the declared dogfood path admits exactly as before (exit codes, labels, receipt fields byte-compared against goldens inline in `qualification_strict_test.go`). Named apart from MH-10's `TestE2EDeclaredUnaffected` (same `e2e` package; no redeclaration).
  - `TestE2EDeclaredLabelsDistinct`: each surface (admission JSONL, journal rows, receipts, `runs` output) carries at least one declared-posture row, and none labels a declared posture qualified/verified/strict.
- **Test plan**: Build `cmd/mythhelm` once per package run; temp `MYTHHELM_HOME`; golden comparisons for the declared path.
- **Invariants touched**: I15 (v2 §7.3: declared stays unverified); I02 (v2 §7.3: end-to-end blocking preserved).

### Task 6 — Strict billing user documentation

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 3
- **Change**: Document strict vs declared billing in the user guide and record the billing-flip decision, so users can tell which posture they run and what each proves.
- **Files**:
  - `docs/user-guide.md`
  - `docs/decisions/0012-strict-subscription-admission.md`
- **Acceptance**:
  - `sed -n '/^## Billing/,/^## /p' docs/user-guide.md | grep -q "subscription-only"`: a `## Billing` section exists documenting both postures. Exits non-zero before the change.
  - `sed -n '/^## Billing/,/^## /p' docs/user-guide.md | grep -q "live-qualified" && sed -n '/^## Billing/,/^## /p' docs/user-guide.md | grep -q "never verified"`: the section states the strict pass conditions and that declared means user-declared, never verified. Both exit non-zero before the change.
  - `grep -q "Status: accepted" docs/decisions/0012-strict-subscription-admission.md && grep -q "ADR-0002" docs/decisions/0012-strict-subscription-admission.md && grep -q "early return" docs/decisions/0012-strict-subscription-admission.md`: ADR-0012 records the flip (early return removed, consult fails closed) and stales ADR-0002 item 1. Exits non-zero before the change (file absent).
- **Test plan**: No Go test asserts prose; each item is a recorded shell command with a failing counterfactual (absent section before the change).
- **Invariants touched**: None (docs only; Task 3 keeps I15/I02 — this task describes, not changes, behavior).

### Task 7 — Maintainer live qualification run

- **Domain/agent**: maintainer
- **Budget**: standard
- **Depends on**: Task 2, Task 3
- **Also waits for**: Q1 resolved — `LiveRecord` refuses any `Gaps`, so the live run needs the `UnresolvedSources` edit (see Dependencies)
- **Change**: The maintainer runs the live entitlement suite with an explicit allowance grant and commits the sanitized live record, replacing synthetic fixtures with authorised evidence.
- **Files**:
  - `adapters/claudecode/testdata/qualification/live-record.json`
- **Produces**: the sanitized `live-qualified` entitlement record (no secrets, no account identifiers beyond the hashed identity ref).
- **Acceptance**:
  - `go test -tags live ./adapters/claudecode/ -run TestLiveQualifyEntitlement` passes under `MYTHHELM_LIVE_QUALIFY=1` with disposable fixtures, observed and recorded by the maintainer (run output pasted in the completion entry).
  - `live-record.json` validates through `qualify.DecodeRecord`, carries `authorised-live` evidence with the suite identity, and contains no secret material: `grep -riE "token|secret|password|bearer" live-record.json` exits non-zero, and a field-allowlist review permits only the hashed `IdentityRef`, `AuthCategory` and non-secret status fields (recorded in the completion entry).
  - A strict run against the recorded live state admits (observed by the maintainer; receipt attached to the completion entry).
- **Test plan**: Maintainer-executed; the run hands this task over when ready and continues with everything not depending on it.
- **Invariants touched**: I04 (explicit allowance grant for the run); I15 (v2 §7.3: the earned strict pass); I19 (no secret extraction — sanitized record only).
