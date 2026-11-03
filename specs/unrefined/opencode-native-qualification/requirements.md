## OpenCode native qualification — Requirements

> Unrefined. Run /refine-spec opencode-native-qualification before /spec.
>
> Requirements-only child of [remaining-native-targets](../remaining-native-targets/plan.md), delivering the OpenCode portion of MH-29/W14. Normative product source: Master Specification v2 §§7,14,17–18, UR-05/UR-06. The epic’s fixed Shared Qualification Contract is binding here. No native call, account operation or qualification pass is authorized or claimed.

## Context

- **Backlog card**: MH-29 (child scope only; the epic owns aggregate completion).
- **Issue**: [#135](https://github.com/turbokast/mythhelm/issues/135).
- **Grounding revision**: `615e74a6bb6907f87bd2065e2f576f5e18b5dec5` on actual origin/main.
- **Target identity**: `opencode` in `internal/qualify/seed.go:62`; retain the existing canonical name/record history.

The master names OpenCode as an explicit target. Its candidate direction is: Installed native server/CLI; thin SDK or native ACP only if independently qualified. This is a design input requiring current primary-source discovery, not an observation of installed behavior. Qualification must address: Provider-specific entitlement; all small/title/compaction/review routes and native plugins; operating a Claude model remains OpenCode, and no other harness subscription secrets may be imported.

Premise grounding: HOLDS — the named seed commitment (`internal/qualify/seed.go:62`) and v2 §7.2 direction exist. PARTIAL — recorded complete `git ls-tree -r --name-only origin/main adapters` enumeration at the grounding revision yields directory set `claudecode, codex, fake`; no `opencode` adapter directory appears. `internal/qualify/seed.go:93` (SeedV1 through line121) initializes unknown independent columns. UNVERIFIABLE — installed executable/version/config/provider/funding/platform behavior and live evidence were not probed. `internal/qualify/qualify.go:57` (Key), `:75` (Evidence), `:105` (Record), `internal/qualify/registry.go:203` (Record) and `internal/journal/migrations/0002_qualification.sql:4` ground the reusable qualification producer/storage; none qualifies this target. The epic Current State cites the Windows implementation-versus-promotion boundary.

### Objectives

- **O1**: Identify the smallest useful exact native OpenCode execution surface and disclose its fidelity limits.
- **O2**: Qualify effective funding, trust, tools and lifecycle independently without substituting another harness or losing process ownership.
- **O3**: Publish exact versioned support/refusal rows and bounded next tests while preserving unknown evidence honestly.

### Non-Goals (and what still binds)

| # | Out of scope | Still binding |
|---|---|---|
| N1 | Other harness implementations; repeated Claude/Codex work. | I01/I14/I16; their evidence is not inherited. |
| N2 | Installation, login, account/credit/plan changes, subscription-secret import or native inference in this requirements change. | I02/I04/I15/I19/I21; later live tests require their separately applicable authority and safe funding boundary. |
| N3 | Automatically supporting every OS/architecture, alternate SDK/cloud profile or host. | I14/I16; every advertised pair/surface has independent evidence. Windows process-support work remains MH-11, Herdr MH-3 and public release MH-7. |
| N4 | New mandatory native-session resume/steering, paid compaction or automatic fallback. | I02/I04/I18; optional operations remain unsupported/unknown until negotiated and qualified; MH-15 owns continuity composition. |

## Functional Requirements

### FR-1 — Bounded discovery and exact bundle identity (v2 §§7.1–7.2)

- **AC-1.1** [I01/I09/I14, G02] When preparing a OpenCode bundle, discovery shall resolve the exact native executable/build/digest, adapter/protocol, surface, OS/architecture, provider/endpoint, observed model/snapshot or moving alias, effort/settings, non-secret auth/entitlement class, effective configuration/instructions/tools/skills/hooks/plugins/MCP digest, trust profile and workspace/environment class. Unknown fields remain explicit; another harness running the same model is not this target.
- **AC-1.2** [I02/I03/I19, G02/G07] If required identity/configuration evidence is missing, opaque, changed or untrusted, then preparation shall refuse before native inference or unapproved startup/tool effects. Safe read-only source inventory shall respect precedence and the epic’s finite limits, not scan/copy secrets, infer no hooks from missing user files or trust a digest without reviewing its effects.
- **AC-1.3** [I09/I14, G02] When documenting the candidate surface, the record shall cite dated official primary documentation for the exact operation/version and distinguish documented-candidate, fixture-tested and actual live evidence. Missing/currently unavailable documentation or binary evidence shall produce an explicit bounded next test, not a fabricated supported surface.

### FR-2 — Fidelity, permissions and target-specific hazards (v2 §§7.1–7.2,8)

- **AC-2.1** [I01/I03/I11/I14, G02/G10] When qualifying native fidelity, the suite shall observe instruction/skill discovery, effective hooks/plugins/MCP, tool/permission/approval interactions, context behavior and required browser/editor/child functions, each marked preserved/restricted/unsupported/unobservable. A credential-free fixture tests MYTHHELM parsing/control only; live parity claims require the exact actual native surface.
- **AC-2.2** [I03/I11/I14/I16, G02/G10/G12] If a thin SDK, ACP, PTY, runtime SDK or host bridge is selected, then its exact native ownership/approval/trust behavior shall be independently qualified; screen text/exit code shall not synthesize permission or prove success. Raw model SDK and separately funded/remote profile substitution shall refuse without new identity/authority/admission.
- **AC-2.3** [I01/I09/I14] Before this target is advertised, the qualification plan shall include successful and violating cases for these concrete master-identified hazards: Provider-specific entitlement; all small/title/compaction/review routes and native plugins; operating a Claude model remains OpenCode, and no other harness subscription secrets may be imported. Each hazard must produce observable exact evidence or a bounded explicit unsupported/blocked result; prose promises do not pass it.

### FR-3 — Funding, owned execution and reconciled outcomes (v2 §§6–7)

- **AC-3.1** [I02/I09/I10/I15/I19, G05] When subscription-only is requested, admission shall establish permitted use and effective included allowance/no-paid-continuation behavior for every selected model route, title/small/compaction/review/browser/plugin/observer/child auxiliary that can invoke a model. Sign-in, an included-plan label, zero reported cost, unknown quota or another target’s entitlement shall not supply missing proof. An unproven route shall block before inference; metered modes require distinct explicit authority and identity.
- **AC-3.2** [I05/I06/I12/I18/I21/I24, G04/G12] When the native starts, one admitted worker/process owner shall hold its immutable launch/session identity and declared workspace/resource tree. Cancellation, client loss, worker/native/child crash and recovery shall reconcile exact PID/start identities and descendant/effect boundaries before confirming stop, releasing ownership or retrying. Lease expiry/exit zero alone shall not confirm success or termination; a surviving or unknown child leaves ownership unresolved.
- **AC-3.3** [I02/I04/I12/I18/I21] If startup/transport/result/input ownership is uncertain or effective provider/configuration changes, then no replacement launch, repeated tool effect, unsupported session input or wider funding route shall occur automatically. Preserve artifacts and disclose a typed bounded refusal/reconciliation outcome; re-admission and applicable authority are required before another episode.

### FR-4 — Independent versioned support and drift invalidation (v2 §§7,14,17)

- **AC-4.1** [I07/I09/I14/I20, G02/G05/G10/G12] When evidence is recorded, use the existing canonical qualification registry with independently evaluated Fidelity/Entitlement/Lifecycle columns, exact key/revision, evidence source/method/result/time/expiry/uncertainty, observed-versus-reported labels and next-test authority. Progress and capability verdicts remain distinct; an unsupported optional capability need not falsely label every route supported or the entire target dropped.
- **AC-4.2** [I09/I14/I20] If executable/config/provider/model/entitlement/trust/platform/host facts materially drift or evidence expires, then affected support shall become unknown/blocked until revalidated under a new exact record; preserved old evidence shall not authorize the changed bundle. Windows amd64/arm64, WSL, Linux/macOS and each host require separate rows; scripted lifecycle, cross-compilation or Wine cannot qualify this native target.
- **AC-4.3** [I07/I09/I14/I25] When users inspect or attempt an unavailable route, CLI/host/public support projections shall show its exact supported subset or typed refusal, missing evidence and safe bounded next test without secrets. Host attachment evidence remains separate; no S1/S2/G02/G05/G10/G12 or release pass follows from this requirements/spec/fixture work.

## Non-Functional Requirements

- **NFR-1** [I13/I21] The epic’s fixed discovery limits apply:8 executable candidates/32 non-secret sources,5s each/30s total,1MiB encoded output per operation/64nest and16MiB sanitized evidence total. Missing required data blocks rather than silently truncating it; cancellation and timeout are observable.
- **NFR-2** [I13/I21] Credential-free fixtures are capped at32 cases per exact target/surface/platform suite,60s each/five minutes total and4 local children per case. Cancellation request occurs within1s, with separate30s safety reconciliation; uncertainty stays explicit. No credentials, network/model calls or uncontrolled executable installation in contributor tests.
- **NFR-3** [I02/I10/I15/I21] Later separately authorized live qualification is capped at one exact bundle, two native episodes and ten minutes execution plus30s safety reconciliation, using smaller grant/native limits. At most one retry for idempotent observations; uncertain side effects never retry automatically. Predeclared billing/auxiliary exposure must be proven before inference; runtime ceilings alone are not spending caps.
- **NFR-4** [I03/I09/I19/I20] Evidence is sanitized and bounded; retain algorithm-qualified identities, exact source/check/environment revisions and uncertainty, not raw credentials/private native history. New formats/API changes require normal reviewed compatibility contracts; the child preallocates no public API or migration number.

## Dependencies

- Consume existing MH-10 `specs/done/qualification-registry/`, `internal/qualify/` and canonical journal storage; existing adapter/control/worker/admission/protected-acceptance contracts remain the only execution/evidence authorities.
- Independent of the other four epic children. Multiple native surfaces/platforms must be split before implementation if one bounded scope cannot qualify them safely. Shared producer changes get separate reviewed domain-pure tasks before consumers.
- Applicable MH-11/`windows-process-ownership` evidence and promoted exact process-support row are required for advertised native Windows lifecycle; actual Windows source implementation or scripted runs do not close native qualification. MH-3 owns Herdr compatibility and MH-7 owns public release evidence.
- MH-15 consumes optional operation-specific continuity evidence; no assumption that this native surface supports resume/steering is made. Existing Claude/MH-12 and Codex/MH-4 implementations are external references and never superseded.
- Requirement/refinement/credential-free design may proceed while live/platform evidence remains blocked. Production launch, account action, inference and full supported-journey promotion require their separately applicable authority and actual gate results.

## Impacted Components

- `internal/adapter/adapter.go:17` (Adapter) and `:149` (Launcher) supply existing seams. Recorded complete `git ls-tree -r --name-only origin/main adapters` output yields only `claudecode, codex, fake` directories at the grounding revision; new target packages are later reviewed choices, not claims about native installation.
- `internal/qualify/qualify.go:57` (Key), `:105` (Record), `internal/qualify/registry.go:203` (revisioned Record), `internal/qualify/seed.go:93` (SeedV1) and `internal/journal/migrations/0002_qualification.sql:4` supply the existing qualification records/store; no second matrix database.
- `internal/admission/admission.go:232` (Decide), `internal/workers/worker.go:217` (Main), `internal/supervisor/pipeline.go:121` (Run), `internal/supervisor/recover.go:39` (Recover), `internal/processsupport/support.go:145` (Windows) and `internal/control/lock.go:85` (AcquireInstance) ground existing producer seams to preserve, not permission to broaden an adapter task.
- `docs/validation-plan.md:42` (scope/critical path) and `docs/limitations.md:228` (Windows evidence) are existing evidence/limitation inputs. Target compatibility docs and CLI/host changes are future investigation areas; later tasks must enumerate actual files/domain ownership before implementation.

## Open Questions

- **OQ-1 — Exact surface/build and evidence availability.** Blocks implementation qualification of this target; default unknown until current dated primary docs and authorized non-secret observations resolve the candidate direction. No user login or live probe is requested here.
- **OQ-2 — Included-only/auxiliary proof.** Blocks subscription-only live execution/support; default refuse if any mandatory effective route or paid continuation boundary is unproven. Do not manufacture proof from secret import, plan marketing or synthetic streams.
- **OQ-3 — Discovery-first design outcome and later implementation gate.** Design may begin now with a bounded discovery contract using the fixed NFR-1 limits. It must deliver either an evidenced exact surface/build/OS/architecture/host candidate inventory or an explicit blocked inventory naming missing facts, safe next tests and authority required; neither outcome invents installed behavior. Unknown initial subset does not block that discovery design. Adapter implementation/live promotion still waits actual mandatory facts and qualified producer contracts; only after a candidate is evidenced may operation/error/protocol contracts, counterexamples and platform/surface implementation tasks be frozen. Wider scope is separately reviewed.

## Definition of Done

- [ ] Independent refinement accepts target-specific hazards, producer boundaries and fixed bounds; design/tasks remain a later reviewed stage.
- [ ] FR-1–FR-4 each have exact source/evidence and non-empty success/violating fixture legs; no synthetic result is relabelled live qualification.
- [ ] Every advertised bundle has independent current fidelity/funding/lifecycle and applicable platform/host gate evidence; missing routes stay explicit commitments with safe next tests.
- [ ] No completed adapter/platform work, native secret, alternate authority store or unsupported funding/profile substitution is introduced.
