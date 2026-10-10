## Codex Native Adapter — Requirements

> A native Codex adapter and its independent qualification: fidelity, subscription entitlement, stop/recovery and session continuity are recorded separately in the MH-10 registry, so Codex can become the first included-only route or a later second route as evidence decides. A slice of v2 §§4.4, 7 and 18. Normative source: mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md, version 2.0; § numbers, I-IDs, G-IDs and AT-IDs refer to it.

## Context

- **Backlog card**: MH-4
- **Issue**: [#25](https://github.com/turbokast/mythhelm/issues/25) (card discussion; the card holds status, stage and score)

The card (Stage 1, gates G02/G04/G05) asks for Codex to be qualified as a native harness on four independent axes. Today Codex is only a seeded registry row: `planned` (production Go also names `codex` in `internal/admission/qualify.go` `knownHarnesses` and `internal/qualify/seed.go`; `internal/supervisor/pipeline_test.go` pins `--adapter codex` as rejected), with the next test "Qualify native-managed sign-in, approvals and effort metadata; authority: maintainer live-test grant" (`internal/qualify/seed.go`, `seedHarnesses`). `internal/adapter` holds the generic seam and `adapters/` holds `claudecode` and `fake` only. The `codex` strings elsewhere in the tree are the consult/review vendor wrappers (`scripts/codex/`, `scripts/vendors/`), which are advisory tooling, not a product adapter.

The seam is generic but its callers are not (derived with `grep -rln 'claudecode\|claude-code' --include=*.go internal cmd | grep -v _test.go`, 15 files). Admission dispatches to `decideClaudeCode` and imports `claudecode` types for trust, auth evidence and manifests (`admission.go`, `native.go`, `billing.go`, `qualify.go` `ResolveQualification` and `observedKey`, `projectconfig.go`); adapter-name validation and `dataDestinations` are Claude/fake only (`admission.go`, `internal/cli/run.go`); declarations are pinned to the Claude adapter ID; the worker's adapter map holds `builtin/fake` and `builtin/claudecode`; `supervisor/pipeline.go` and `workers/contain.go` use Claude config paths; `contain/records.go` keys boundary records by route; `doctor` inventories Claude settings; `billing.splitRoutes` lists only `claude-code`; and `PrepareInput` carries fields marked `claudecode only`. A second native adapter therefore first needs these call sites made harness-neutral without changing Claude Code's behaviour.

Two limits shape the work. First, every Codex claim about the native surface (app-server protocol, sign-in modes, approvals, effort metadata, credit behaviour) comes from vendor documentation (v2 §20, SRC-04 to SRC-06) and cannot be verified from this repository; the executable is not present in the authoring environment. Fixtures can establish MYTHHELM's parsing and logic only (v2 §7.1); live qualification needs an explicit maintainer allowance grant on disposable fixtures, which this card does not supply. Second, Codex must not be forced behind unproven Claude qualification, and being the second named adapter is not a product requirement (card Notes; `product/decisions.md`).

Grounding verdicts (2026-10-09):

- `Codex has no native adapter today` HOLDS: `adapters/` lists `claudecode` and `fake` (`ls adapters`); `internal/workers/worker.go` adapters map has two entries; registry seed row is `planned`.
- `Requires MH-10 records` HOLDS: `qualification-registry` is in `specs/done/`; `internal/qualify` provides the seven-harness seed, Progress scale and drift (`internal/qualify/qualify.go`, `seed.go`, `drift.go`); backlog marks MH-10 shipped.
- `Requires effective startup/trust controls` PARTIAL: Claude trust/inventory exists (`internal/admission/trust.go`, `native.go`, `claudecode.Inventory*`) and is Claude-specific; no Codex equivalent exists, and Codex's startup/config discovery is unverified.
- `Admission is harness-neutral enough to add Codex without change` REFUTED as an unstated assumption: `decideClaudeCode` (`internal/admission/admission.go`), `AuthEvidence = claudecode.AuthEvidence` (`internal/admission/billing.go`) and the worker adapter map are Claude-bound. The spec includes the generalisation as FR-1.
- `Issue #25 text (stage 2, "same fidelity ... gates as the first adapter", sections §9.4/§13.9/§20.4, gates G02/G05)` PARTIAL: stale revision numbering and stage; the card and v2 §§7, 18 govern, and the card drops the "second adapter" framing.
- `Codex config sources loaded at startup, managed policy and credential locations` UNVERIFIABLE: see OQ6.
- `Codex app-server is the native control surface; exec is smaller` UNVERIFIABLE here (v2 §7.2 states it as a candidate direction): see Open Questions OQ1.
- `Codex can prevent paid continuation within an included plan` UNVERIFIABLE (v2 §7.2, §7.3): see OQ2.
- `The failure is reachable / a criterion can fail today` PARTIAL: FR-1 to FR-6 criteria fail today (no Codex adapter or admission path). Registry-visibility criteria are partly met already (doctor lists the seeded Codex row, `internal/cli/doctor_test.go`), so FR-7 binds only to the new observables it names.

### Objectives

- **O1**: `mythhelm doctor` and admission treat Codex as a native route with its own qualification record: the fidelity, entitlement and lifecycle columns each read proven, not-proven or unknown (never defaulted), stop and recovery evidence sits in the lifecycle column, and reconnect, resume and model metadata are `adapter.Capabilities` entries (`internal/adapter/adapter.go`; doctor does not render capabilities today, so they are asserted on the adapter output, not on doctor) valued supported, unsupported or unknown.
- **O2**: A Codex attempt can be prepared and launched by the unmodified native executable through a structured surface, with its stop ladder, usage and session identity observed, using fixtures and no credentials.
- **O3**: Strict `subscription-only` admission for Codex passes only on a `live-qualified` record with proven entitlement and a supported `stop_at_exhaustion` capability for the exact probed route, and blocks otherwise.
- **O4**: Adding Codex changes nothing observable for Claude Code or the fake adapter, except the exceptions listed in AC-1.3.

### Non-Goals (and what still binds)

| # | Out of scope | Still binding in this spec |
|---|---|---|
| N1 | Live qualification runs, account changes or credit use (needs a maintainer allowance grant; OQ3). | v2 §7.1, I15: without live evidence the Codex record stays below `live-qualified` and strict admission blocks. |
| N2 | Claude Code qualification (MH-12) and the other five harnesses (MH-29). | I14: no other route's record or progress label changes. |
| N3 | Runtime SDK, raw model SDK or remote Codex agent as a substitute surface. | I16, G12: such a surface is a separate qualification and never stands in for the native executable. |
| N4 | Cross-harness handoff and the two-harness workflow (MH-15, W06). | I12, v2 §4.4: native-session resume and reconnect semantics are recorded as capability facts only. |
| N5 | `metered-allowed` mode. | I10, I15: no hard limit is advertised; no paid fallback. |
| N6 | Windows process-tree cancellation (MH-11). | I14: Codex platform rows outside the tested OS stay unsupported or unknown. |
| N7 | Restricted and inspect execution profiles for Codex. | I14, G07: `builtin/codex` is refused for those profiles through the unknown-boundary path, with a refusal test per `contained-execution-profiles` Task 11. |

## Functional Requirements

Acceptance criteria use EARS. Tags in brackets name the invariants, gates and sections each one serves.

### FR-1 — Harness-neutral admission and worker seam (v2 §4.4, I01, I14, I16)

- **AC-1.1** [I14, I11 (v2 §2), v2 §4.4] When admission resolves an adapter, it shall select the decision path by the adapter's descriptor through a registered per-harness path rather than a `claudecode`-only function; registration stays module-internal (`internal/adapter/adapter.go` package comment, I11); a test registers a stub harness and observes it reach `Prepare`.
- **AC-1.2** [I01 (v2 §2)] The worker's adapter map shall start `builtin/codex` by descriptor ID; a test fails if the ID is unknown or resolves to another harness.
- **AC-1.3** [I14 (v2 §2)] When the Claude Code and fake adapters run their existing admission and launch tests, the results shall be unchanged; the only permitted edits are: the `--adapter` list text and the `--adapter`-conditional flag help text ("claudecode only", `internal/cli/run.go`) extended to codex only where Codex uses them (`--allow-untested-native-version` per AC-2.2 and `--trust-native-config` per AC-3.1 extend; `--declare-entitlement` stays claudecode-only per the OQ8 default; `--strip-credential-env` is refused for Codex until OQ6 decides); the `pipeline_test.go` unknown-adapter case, which moves to another name now that `codex` is accepted; the plain doctor line format for every row (AC-7.3, pinned by the `qualLine` regex in `doctor_test.go`); and the seed-count and row-order pins if the registry gains rows (none planned, AC-2.5).
- **AC-1.4** [I01, I03 (v2 §2)] If a Codex `Prepare` receives a non-empty Claude allowed-tools list (`PrepareInput.AllowedTools`) or passthrough, then it shall refuse with a `*BlockedError`; a test shows argv and env unchanged when the list is empty.
- **AC-1.5** [v2 §4.4] The seam shall gain only what FR-2, FR-5 and FR-6 need: reconnect and model-metadata capability entries, and an `acknowledged` field on the interrupt report beside the existing `Sent` and `Confirmed` (`internal/adapter/adapter.go` `InterruptReport`, AC-5.2); the negotiated optional methods (inspect-liveness, reconnect, resume, steering, approval response) and any method that returns `capability_unsupported` are not added here and go to the design's honesty register.

### FR-2 — Codex probe, capabilities and prepared attempt (v2 §4.4, §7.1, I01, I02)

- **AC-2.1** [I01 (v2 §2), AT-02 (v2 §18.2)] When `Probe` runs against a fixture executable, it shall return absolute path, version, SHA-256, OS and arch without starting a model task.
- **AC-2.2** [I09 (v2 §2)] If the native version is not in the fixture-tested set, then `Probe` shall mark it untested and block with `ErrCapability` (exit 7) unless the existing untested-version override is set, as the Claude probe does.
- **AC-2.3** [I09 (v2 §2), v2 §4.4] `Capabilities` shall report each of reconnect, resume, steering, approval response, usage and model metadata as supported, unsupported or `unknown`, never defaulted to supported; a fixture whose native reports nothing for an entry yields `unknown` for it.
- **AC-2.4** [I02 (v2 §2), v2 §4.4] If the executable hash at `Prepare` differs from the probed hash, then it shall fail closed with a `*BlockedError`.
- **AC-2.5** [I01 (v2 §2), v2 §7.1] The adapter shall launch the unmodified native executable through the structured surface chosen under OQ1; `exec` is a different surface key (Key.Surface) that gets a registry row only when evidence for it exists; this spec adds no seed row (the seed holds exactly seven records), no `exec` launch path, and no progress above `planned` for it.

### FR-3 — Fidelity and effective configuration (v2 §7.1, I01, AT-02, AT-11)

- **AC-3.1** [AT-11 (v2 §18.2), I03 (v2 §2)] When Codex startup would load project instructions, hooks, MCP servers or plugins, the system shall inventory the effective non-secret configuration and resolve trust before any native process starts, including the version probe; a fixture with an untrusted project hook records zero fixture-executable invocations and the block code `untrusted_native_config`.
- **AC-3.2** [v2 §7.1, I09 (v2 §2)] Each preserved, restricted, unsupported or unobservable difference from direct-native use shall be recorded as a fidelity delta; a difference the adapter cannot observe is `unobservable`, never `preserved`.
- **AC-3.3** [AT-11 (v2 §18.2)] If the effective configuration digest or executable digest changes after qualification, then affected evidence shall be invalidated through the existing drift path.
- **AC-3.5** [I02 (v2 §2), v2 §7.1] If a Codex startup configuration source cannot be enumerated, then it shall be an unresolved gap that blocks strict admission, mirroring `claudecode.UnresolvedSources`.
- **AC-3.4** [I19 (v2 §2), v2 §7.3] The adapter shall never read credential contents or disable a native authentication method; the adapter opens only files on its enumerated inventory list, and a test plants a sentinel file outside the list and asserts it is never opened (with a red-first variant that opens it).

### FR-4 — Entitlement and billing honesty (v2 §7.3, I02, I15, AT-03, AT-04)

- **AC-4.1** [I15 (v2 §2), AT-04 (v2 §18.2)] If entitlement or no-overage evidence is unknown, then strict `subscription-only` admission for Codex shall block with the shared consult codes `no_qualification_record` or `entitlement_not_proven` (`internal/admission/qualify.go`), the same as Claude Code; "no-overage evidence" means the entitlement column's paid-continuation evidence, and when entitlement is proven but `stop_at_exhaustion` is not supported it blocks with `stop_at_exhaustion_unproven`; sign-in or a user declaration alone shall not admit.
- **AC-4.2** [AT-03 (v2 §18.2), I15 (v2 §2)] The qualification record shall list every model-using auxiliary and descendant route of a Codex attempt (the closed list, a conservative default under OQ2 beyond the AT-03 route classes: title, summary/compaction, review, child, native observer; any auxiliary outside the list is `unknown` and blocks, with a failing fixture) and block on any route whose coverage is not established.
- **AC-4.7** [I15, I04 (v2 §2), v2 §7.2] If the observed auth category is not the native-managed sign-in (an API key or partner auth), then it is a different `AuthCategory` key and strict admission shall block with `no_qualification_record` until that key has its own entitlement evidence; partner auth is not treated as equivalent to native auth.
- **AC-4.3** [v2 §7.3] Where the plan's purchased-credit consumption is distinct from reload, the entitlement-column evidence shall carry `purchased_credit: preventable|not-preventable|unknown`; unless it reads `preventable`, strict admission blocks with `entitlement_not_proven`.
- **AC-4.4** [I09 (v2 §2), G05 (v2 §18.3)] Usage counters shall be normalised with the existing billing normaliser; the decoder shall fill only fields whose non-overlap has a cited source and leave the rest nil, so the total reads `unknown` rather than double-counting (`SplitUsage` sums four fields). Fixtures are marked synthetic.
- **AC-4.5** [I02, I09 (v2 §2)] Unknown remaining quota alone shall not block an otherwise qualified stop-at-exhaustion route, and shall never be mapped to zero.
- **AC-4.6** [AT-13 (v2 §18.2), I09 (v2 §2)] Until OQ10 supplies a cited source, every Codex error shall map to a native-error observation and every usage field shall stay nil; once supplied, the Codex decoder shall map documented exhaustion and throttling shapes to `allowance_exhausted` and `rate_limit` (the internal class for v2 §4.5 `provider_throttled`, on which the worker already branches), and any unrecognised error to a native-error observation; each class has a synthetic fixture and a test that fails if a throttle is read as exhaustion.

### FR-5 — Stop, interruption and recovery (v2 §6.4, I06, I18, AT-08, G04)

- **AC-5.1** [I06 (v2 §2), AT-08 (v2 §18.2)] The adapter shall provide a stop ladder with native interrupt first, then terminate and kill with finite graces, delivered to the process group by the worker; a fixture agent ignoring interrupt receives terminate after the first grace and kill after the second, and its group is gone within the ladder's summed graces (values fixed by the design from `supervised-stop-recover`).
- **AC-5.2** [I06 (v2 §2), v2 §4.4] Native interruption shall report requested, acknowledged and termination evidence separately; the report carries `sent` (signals attempted, as `ClimbLadder` records them today), `acknowledged` (`yes`, `no` or `unknown`; `unknown` under a signal-only ladder) and `confirmed` (process group gone). `acknowledged` stays on the adapter's interrupt report and is not added to the journal `stopped` event, so Claude and fake journal output is unchanged. The run-level stop stays unconfirmed until reconciliation establishes the result.
- **AC-5.3** [I18 (v2 §2)] Each attempt shall have one process owner and each native session at most one input writer; a second `Start` on the same adapter instance is refused with a `*BlockedError` before any launch, and a structural test shows `Session` exposes no write method, so the worker-owned stdin pipe given at `Start` is the sole input writer; the cross-worker guarantee rests on the adapter having no resume or reconnect path (N4).
- **AC-5.4** [I12 (v2 §2), AT-06 (v2 §18.2)] Recovery of a Codex attempt shall follow `supervised-stop-recover` (its reconcile-before-resume criteria); this spec adds only that the Codex adapter exposes no replay path.

### FR-6 — Native session continuity facts (v2 §4.4, §9.2, I12)

- **AC-6.1** [v2 §4.4] The adapter shall capture the native thread/session identity when the native reports it, and record it empty (unknown) when it does not; effort and model metadata remain `unknown` until a later seam change gives `SessionStarted` a field for them (design honesty register). `Capabilities.ModelMetadata` declares a capability and is not a capture path.
- **AC-6.2** [I12 (v2 §2), v2 §4.4] The adapter shall report resume and reconnect in `adapter.Capabilities` as supported, unsupported or `unknown`; and no code path shall patch a private native store. Implementing resume or reconnect behaviour is not in this spec (N4).

### FR-7 — Registry records, doctor and ordering (v2 §7.1, §18.3, I14, G02, G05)

- **AC-7.1** [I14 (v2 §2), G02 (v2 §18.3)] The Codex record shall carry fidelity, entitlement and security/lifecycle evidence as independent columns; a pass in one shall not change another.
- **AC-7.2** [I14 (v2 §2)] Only fixture evidence shall move the record to `fixture-tested`; `live-qualified` requires a live evidence record naming exact build, platform and account class.
- **AC-7.3** [v2 §17.1, G10 (v2 §18.3)] `mythhelm doctor` shall show the Codex row with its progress, each column verdict other than `proven` named as failing, and the next test in plain output as well as JSONL (plain output omits it today), as `failing: <column>[, <column>]` on the row and `next_test: <text>` on the line below (pinned by a test), keeping the existing (harness, surface) row order that `doctor_test.go` pins.
- **AC-7.4** [v2 §7.2] A table test shall show identical registry records under the `claude-code` and `codex` keys yielding identical `ResolveQualificationKey` verdicts (the caller-supplied-key function; `ResolveQualification` stays the Claude wrapper), so no code path prefers a harness by name.
- **AC-7.5** [I04, I16 (v2 §2), G12 (v2 §18.3)] If the observed key's `Harness`, `ProviderEndpoint` or `AuthCategory` (`internal/qualify/qualify.go` `Key`) matches no record, then strict admission shall block with `no_qualification_record` (exit 3 as for Claude Code); a test varies each of the three fields in turn.

## Non-Functional Requirements

- **NFR-1** [I13 (v2 §2)] All tests run offline without Codex credentials; the native executable is a fixture.
- **NFR-2** [I19 (v2 §2), v2 §8.3] No log, record or fixture contains credential contents, prompts or source text.
- **NFR-3** [G10 (v2 §18.3)] `docs/limitations.md` states the Codex limits (surfaces, platforms, unproven columns) alongside any support claim.

## Definition of Done

- [ ] AC-1.1 to AC-7.5 each have a named test or inspectable artifact; Claude Code and fake suites green with only the AC-1.3 exceptions.
- [ ] Codex registry record at `fixture-tested` at most, unless a live evidence record exists (OQ3); the record's progress is as OQ7 decides.
- [ ] FR-1 to FR-7 covered by the list above; each AC-n.m names a test in `/spec`'s tasks.
- [ ] Doctor output and docs state which qualification columns remain unproven.

## Open Questions

- **OQ1**: Which Codex surface is the primary structured one (app-server versus `exec`), and which native version is the fixture baseline? Blocks FR-2 and FR-3 design. Options: app-server primary with `exec` as a smaller second record (v2 §7.2 direction, recommended); `exec` only. Needs maintainer confirmation against current vendor documentation (UNVERIFIABLE here). Default: app-server primary, baseline a synthetic fixture version marked synthetic. Decides: maintainer.
- **OQ2**: Can Codex prevent paid continuation in an included plan by documented native settings, and what covers its auxiliary routes? Blocks FR-4 beyond the blocking default. Options: block strict admission until evidence (default); admit via a documented stop-at-exhaustion route. Decides: maintainer with the security reviewer.
- **OQ3**: Will the maintainer grant a live-test allowance with disposable fixtures for Codex? Blocks any `live-qualified` claim. Default: none, record stays fixture-tested. Decides: maintainer (allowance grant).
- **OQ4**: Do the in-progress admission-touching specs (`claude-strict-subscription`, `contained-execution-profiles`) land before FR-1 begins? Blocks task ordering. Default: sequence FR-1 after them to avoid conflicting edits in `internal/admission/`. Decides: maintainer (delivery order).
- **OQ5**: Is the product-level ordering (Codex first versus second route) decided by evidence only after MH-12 results, or does the maintainer want a comparison task? Default: evidence-only, no comparison task. Decides: maintainer.
- **OQ6**: Which files and settings does Codex load at startup (including managed policy), and where does it store credentials? Vendor facts, unverifiable here. Blocks AC-3.1, AC-3.3, AC-3.4. Default: any source not enumerated is an unresolved gap that blocks strict admission (AC-3.5). Decides: maintainer from current vendor documentation.
- **OQ7**: What progress should the shipped Codex row show: `planned`, `documented-candidate` or `fixture-tested`? A public support claim (I14, G10); seed rows insert only when absent. Default: `planned`; fixture drafts are not wired to a production caller. Decides: maintainer.
- **OQ8**: Does Codex get the user-declared `subscription-declared` posture (never verified), or strict only? Declaration handling is Claude-pinned. Needs a decision record. Default: strict only; O2 is then shown at adapter level with fixtures, and no Codex run is admittable until live evidence exists. Decides: maintainer (decision record).
- **OQ9**: Is the native interrupt a structured-surface request or a signal, and who is the single input writer (I18)? Depends on OQ1. Blocks AC-5.1, AC-5.2, AC-5.3. Default: signal ladder only, acknowledgement `unknown`, the sole input writer is the worker-owned stdin pipe given at `Start`. Decides: maintainer from current vendor documentation.
- **OQ10**: What do Codex usage frames and error/exhaustion shapes look like, and do cached or reasoning counts overlap other fields? Vendor facts, unverifiable here. Blocks AC-4.4 and AC-4.6 beyond the nil/native-error default. Decides: maintainer from vendor documentation.

## Dependencies

- **Prerequisite**: `qualification-registry` (done, MH-10); `budget-ledger-s1` (unfinalized, all tasks done; AC-4.4, AC-4.5, AC-4.6); `supervised-stop-recover` for the stop ladder and recovery (in progress). Its Task 4 edits `internal/workers/worker.go` and `internal/supervisor/pipeline.go`, both FR-1 targets, so FR-1 sequences after it; its "reconnect" is the worker-supervisor handshake, not native-session reconnect.
- **Coordinating**: `contained-execution-profiles` Task 11 freezes route evidence and requires a record or refusal test for every route the CLI accepts, so `builtin/codex` needs one (N7).
- **Conflicting**: `claude-strict-subscription` (MH-12): its Tasks 3 and 4 have merged and only Task 5's `tests/e2e/qualification_strict_test.go` pins remain, which also pin the consult codes FR-4 shares. Its Task 7 is also open, and Task 5 waits for a Q1 edit to `UnresolvedSources`, which `admission.go` reads inside `decideClaudeCode`, the function FR-1 refactors; FR-1 sequences after it.
- **Scope note**: the work spans core, adapters and docs and crosses the `internal/`↔`adapters/` seam, so it needs an `architect` review; `/spec`'s scope step should consider an epic split (FR-1 and seam changes, then the Codex adapter).
- **Superseded**: none.

## Impacted components

- `internal/adapter/adapter.go` (seam types, `PrepareInput` harness-specific fields)
- `internal/admission/` (`admission.go` `decideClaudeCode`, adapter validation, `dataDestinations`; `billing.go`; `native.go`; `qualify.go` `ResolveQualification`, `observedKey`; `projectconfig.go`), `internal/cli/run.go`, `internal/workers/contain.go`, `internal/contain/records.go`
- `internal/workers/worker.go` (adapters map), `internal/supervisor/pipeline.go`
- `internal/qualify/` (`seed.go` Codex row, `drift.go`), `internal/billing/normalize.go` (`splitRoutes`)
- `internal/cli/doctor.go` (Codex row)
- New `adapters/codex/` (probe, capabilities, launch, decode, qualify, fixtures)
- Docs: `docs/limitations.md` (docs domain)
