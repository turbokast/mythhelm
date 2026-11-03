# Account profiles and exhaustion handoff — Requirements

> Refined after independent assessment; see [refinement-log.md](refinement-log.md). Proceed to /spec account-profiles-exhaustion.
>
> Capture named, explicitly authorised native account profiles and safe continuation after allowance exhaustion. Normative source: `mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md`, version 2.0, especially §§4, 6–10 and §18; all I/AT/G identifiers below refer to v2. This document grants no account operation, native trial, implementation or qualification.

## Context

- **Backlog card**: MH-17
- **Issue**: [#82 — Account profiles and exhaustion handoff](https://github.com/turbokast/mythhelm/issues/82).
- **Source**: the card's Summary/Notes; issue #82; historical `product/signals.md` S-4 and `product/decisions.md` D-2/D-4; v2 §§7–10 and Implementation Plan W06.
- **Grounding revision**: `615e74a6bb6907f87bd2065e2f576f5e18b5dec5`; source claims below were read at this revision.

A user should be able to name an authorised native account configuration, see which account/route and coupled allowance it represents, and preserve useful work when that allowance is exhausted. Native authentication remains owned by the harness. Selecting a profile is a change of execution identity and potentially provider, policy, trust or destination; it requires fresh admission, not a credential swap or routing shortcut.

The historical source names native-home selectors and the display label `waiting_for_allowance`. Their current syntax, platform coverage and isolation guarantees must be qualified per native bundle; issue text is not proof of them. The v2 canonical model uses a blocked run with `allowance_exhausted` reason; waiting is a projection of that reason, not another lifecycle. S-4's community-tool and vendor-policy claims remain historical context, not current permission for account rotation. Manual confirmation or a standing grant cannot make prohibited identity cycling permissible.

The existing budget ledger is a useful predecessor, not an implementation of this card. It already preserves exhausted work and couples local reservations to a bucket. MH-15's newly captured native-session-continuity requirements provide the handoff boundary; they do not establish a qualified native resume or two-harness journey. Requirements and synthetic fixture design can proceed before those runtime prerequisites are complete.

### Premise grounding

- **HOLDS — Explicit authorisation and no secret copying are the adopted scope.** Card MH-17, issue #82, D-2/D-4 and v2 §§7.3, 8.2 and I04/I19 agree; no generic account-rotation facility is authorised.
- **PARTIAL — Native-home selection has source seams, not general qualified profile isolation.** `internal/security/env.go:16` allowlists an ambient Claude config selector; `adapters/claudecode/settings.go:83` resolves its configuration roots. `internal/admission/projectconfig.go:37` defines the admitted project schema, and `internal/admission/admission.go:138` defines an execution-trust Profile. Neither type is an account-profile contract; native selector names in the issue remain candidates for qualification.
- **PARTIAL — Account reference exists, but its identity scope must not be overstated.** `adapters/claudecode/probe.go:181` derives an identity reference from native organization/config-root evidence; `internal/admission/reserve.go:68` combines a route's harness/surface/entitlement class and identity reference into a bucket. An organization/config-root digest alone does not demonstrate an individual account or independent allowance.
- **HOLDS — Exhaustion and typed observations are reusable.** `internal/journal/ledger.go:54` stores an exhausted bucket with optional reset, `internal/billing/quota_schedule.go:7` bounds re-admission, and `internal/supervisor/exhaustion.go:49` persists exhausted-bucket/run/reservation effects transactionally. `specs/done/budget-ledger-s1/requirements.md` FR-6 deliberately defers multi-profile handoff.
- **PARTIAL — Historical lifecycle/section references need v2 mapping.** Issue #82's Revision 1.1 §§13.2/13.3/13.10 are historical; v2 §§6.2, 7.3, 9.2 and 10.3 govern this change, including `waiting_for_allowance` as a display label only.
- **UNVERIFIABLE — A particular profile/home/surface supports permitted, isolated, included-only continuation today.** No native qualification is performed by this capture. Supported non-secret account identity, policy coverage, native-home semantics, shared allowance coupling, session compatibility and current applicable conditions require exact-bundle evidence during refinement/qualification; OQ-1/OQ-2 retain the gap.

## Objectives

- **O1**: Select and inspect named native account profiles without handling native subscription secrets or conflating them with execution-trust profiles.
- **O2**: Preserve work on exhaustion and make waiting or an authorised, independently admitted continuation explicit and auditable.
- **O3**: Show truthful account identity, coupled quotas, native-continuity loss and qualification gaps; prevent paid fallback and identity cycling to evade limits.

## Non-Goals (and what still binds)

| # | Out of scope | Still binding in this spec |
|---|---|---|
| N1 | Credential import/export, refresh proxies, token/keychain swapping, automatic logout/login or account purchasing/upgrading. | I19: authentication remains native; isolated homes cannot be manufactured by copying native stores. |
| N2 | Automatic same-vendor identity rotation, limit evasion or choosing a new home merely to reset quota state. | I04/I15 and §7.3 bind explicit switches and standing grants too. |
| N3 | General smart routing, automatic load balancing, experiments or concurrent multi-profile construction. | MH-5/MH-14 own those later features; every proposed route still passes admission and existing reservations. |
| N4 | Provider hard quotas, invented reset countdowns or proof that unrelated native clients cannot consume allowance. | I09/I10: local claims, measurements and provider enforcement remain distinct. |
| N5 | Implementing MH-15's general handoff/session protocol or claiming all native targets support account homes/resume. | Consume its merged contracts; exact unsupported operations remain unavailable, with disclosed reconstruction where qualified. |
| N6 | Optional `metered-allowed` mode. | `subscription-only` must reject unknown entitlement, purchased-credit consumption and paid auxiliaries; it never falls back to metered inference. |

## Functional Requirements

### FR-1 — Explicit native account profiles (§§4.1, 7.1, 8.2; I02/I04/I19)

- **AC-1.1** [I02/I04/I20] When an account profile is registered or revised, the system shall retain a stable profile ID and immutable revision, user-visible name, selected native harness/surface, qualified native selection mechanism, non-secret account/identity evidence reference, quota-coupling references and applicable authority scope; it shall distinguish these from execution-trust profiles and run-envelope configuration.
- **AC-1.2** [I02/I19, G05] If native selection or exact identity cannot be established for the requested platform/build/configuration, then the profile shall remain inspectable but unavailable for strict launch, with the missing capability/evidence stated; a directory name, organization label, sign-in, user declaration or copied status shall not certify the account.
- **AC-1.3** [I19, §8.3] When registering, inspecting or selecting a profile, the system shall neither extract/copy/replay native secrets nor modify native authentication, private session stores or account settings; native login/setup remains a separate explicit native operation and unneeded account identifiers/paths shall not enter shared receipts or exports.

### FR-2 — Qualified native selection and identity freshness (§§7.1, 8.2; I01/I14/I20)

- **AC-2.1** [AT-02/AT-04, G05/G12] Before a native profile-selection mechanism is enabled, evidence shall bind its exact executable/surface/platform/configuration and supported non-secret identity observations, including alternate-home credential isolation and precedence of ambient settings/helpers/managed policy; fixtures alone shall not qualify a live native route.
- **AC-2.2** [I02/I04/I20] When selecting a destination, the system shall re-observe the effective non-secret identity and meaningful configuration before launch, bind those observations to the admitted profile revision, and refuse mismatches, ambiguous identity, stale qualification or changed native/provider routes before model work starts.
- **AC-2.3** [I05/I18/I24] If selecting a profile would mutate a running attempt's native environment or share an unqualified mutable native home/session, then the system shall refuse that operation; a live attempt stays pinned to its admitted profile and writer, and a different profile uses a separately admitted attempt after ownership reconciliation.

### FR-3 — Included-only admission and authority (§§7.3–7.4, 8.2, 10.1; I02/I04/I15/I16)

- **AC-3.1** [AT-03/AT-04, G05] Before any destination is ranked or launched, the system shall independently establish permitted native usage, effective included entitlement and prevention of purchased-credit/paid continuation for that exact bundle and all enabled model-using auxiliaries/descendants; unknown mandatory evidence blocks even if remaining quantity or extra-usage state is user-declared.
- **AC-3.2** [AT-12, I04/I16] When a switch is requested, the system shall require direct user authority or a valid standing grant covering the exact destination profile/policy, harness/provider, data destination, trust scope and finite resource envelope; a pin shall wait unless that authority explicitly permits its replacement, and the announced switch shall record its authority and exclusions.
- **AC-3.3** [I03/I04/I15] If a proposed switch would cycle identities to evade a service limit, violate applicable native/external policy or widen authority, then the system shall reject it even with a user-confirmation click or model-generated recommendation; provider permission is evidence to qualify, never inferred from the ability to launch.

### FR-4 — Coupled quota identity and truthful observations (§§7.3, 10.3; I09/I10)

- **AC-4.1** [AT-13, I09/I10] When profiles refer to the same account or a documented shared organization/model/window allowance, the system shall couple their admission/reservations to the applicable shared bucket set; distinct names, homes, harness surfaces or profile revisions shall not create independent allowance without evidence. Unknown coupling shall not be presented as extra capacity and shall conservatively restrict simultaneous admissions until assessed.
- **AC-4.2** [AT-13, I09] When displaying per-profile usage, remaining allowance or reset, the system shall preserve unit, scope, source, timestamp/freshness, normalization version and `reported | observed | estimated | user-declared | unknown`; missing/stale readings shall not become zero or an invented reset/countdown, and unlike buckets shall not be summed into fictitious availability.
- **AC-4.3** [I10/I21/I24] When a destination is admitted, the system shall transactionally obtain its required coupled local reservations using the existing ledger, without releasing the prior owner's claims before confirmed stop/effect reconciliation; profile switching shall not reset the run's finite execution, repair, retry or completion-reserve envelope or claim provider-side enforcement.

### FR-5 — Structured exhaustion preserves work (§§6.2, 7.3, 10.3; I06/I12/I15)

- **AC-5.1** [AT-13, G04/G05] When a qualified structured allowance-exhaustion observation arrives, the system shall preserve original requirements, candidate/workspace/artifact and native-session references, stop new model work on every affected coupled bucket, and show the canonical blocked reason `allowance_exhausted`; `waiting_for_allowance` may be a display label only. An exit code, arbitrary prose or transient transport/rate-limit failure alone shall not manufacture that observation.
- **AC-5.2** [I06/I12/I18] If the exhausted attempt's process/input/effect ownership is unresolved, then the system shall preserve the pending continuation but block another writer or replacement launch until reconciliation establishes a safe boundary; restart/replay shall not duplicate a native input, account action or admission effect.
- **AC-5.3** [I09/I15/I21, AT-13] When waiting on an exhausted bucket, the system shall honor an authoritative reset or the existing finite unknown-reset backoff with timing labelled unknown, permit cancellation, and never invoke a paid summary, stronger paid repair, purchased credits or hidden account fallback to prepare the handoff.

### FR-6 — Grounded exhaustion handoff (§9.2; I07/I16/I20)

- **AC-6.1** [AT-14/AT-15, G13] When the user or an applicable standing grant chooses an eligible alternative, the system shall validate a handoff referencing the exact original/task/contract, base/candidate, decisions, actual check evidence, unresolved work and acknowledged context frontier; stale, foreign-scope or missing mandatory inputs block delivery rather than silently dropping requirements.
- **AC-6.2** [I01/I04/I18, G04/G13] When continuation starts, the system shall record a new admitted attempt and distinguish compatible exact-native-session resume from a fresh native session reconstructed from artifacts; a profile change shall never imply session compatibility, and cross-harness continuation shall disclose native context loss without transferring private native state.
- **AC-6.3** [I07/I20] If handoff delivery or acknowledgement is uncertain, then the system shall record that uncertainty and reconcile before replay; neither handoff completion, a generated summary nor native success shall certify task acceptance or carry stale verification across candidate/contract changes.

### FR-7 — Inspection, control and receipt truth (§§15.1–15.3, 17.2; I09/I25)

- **AC-7.1** [I09/I25, AT-35] When inspecting a profile or an exhausted run in CLI/plain/JSONL and supported TUI views, the user shall see the selected non-secret identity reference, qualification/provenance and freshness, relevant coupled buckets, known or unknown reset, active pin/grant, rejected alternatives and the precise next evidence or wait condition, with sensitive account details redacted by scope.
- **AC-7.2** [I04/I25] When the user waits, cancels, revokes a fallback grant or returns to the native harness, the system shall stop future automatic switches covered by that authority, reconcile active work normally, and retain useful artifacts and truthful historical attempt identities; saved UI state shall not grant or replay a switch.

### FR-8 — Discriminating qualification and recovery evidence (§18; I07/I14)

- **AC-8.1** [AT-03/AT-04/AT-13] Before advertising this capability, credential-free effect tests shall distinguish two independent buckets from two profiles sharing one bucket, unknown identity from exact identity, reported reset from unknown timing, eligible alternative from unknown/paid continuation, and actual structured exhaustion from arbitrary prose/transport throttling; removing a refusal or breaking bucket coupling shall make the relevant test fail.
- **AC-8.2** [AT-06–AT-09, AT-14/AT-15] Before advertising a particular native combination, separately authorised, bounded qualification shall cover its account/home isolation, identity/config drift, compatible resume or disclosed reconstruction, pin/authority checks and interrupted/repeated-switch recovery using disposable work; no test shall exhaust allowance, copy credentials, create an account or purchase usage as an implicit prerequisite.

## Non-Functional Requirements

- **NFR-1** [I13/I19] Ordinary contributor tests require no native account, inference, provider secrets, network or native-store extraction; profile selection uses synthetic protocol/process fixtures until exact native qualification is separately authorised.
- **NFR-2** [I20/I23, G16] Profile/authority/evidence revisions and handoff lineage use the canonical ledger's additive compatibility and retention contracts; no new editable account scheduler or task store becomes a second authority. Earlier runs remain readable without being retroactively declared profile-qualified.
- **NFR-3** [I20/I21, §4.3] The system shall permit at most 64 registered profiles per repository and 64 KiB of encoded non-secret metadata per profile. Each profile inspection/admission/switch operation shall complete within 30 seconds including identity/configuration observation, further bounded by the remaining enclosing admission/run deadline. Identity evidence shall be observed within 30 seconds of launch and the admitted profile/configuration revisions shall remain unchanged through start; elapsed time alone never proves source freshness or prevents a read/start race. A handoff shall reference at most 256 artifacts totalling at most 16 MiB of referenced payload, with the encoded envelope bounded by 1 MiB and 64 nesting levels; any smaller merged MH-15/native limits take precedence. An effect-free transient observation may retry at most twice, after 100 ms and 500 ms, inside the original deadline and enclosing retry ceiling. Uncertain external effects shall not replay automatically; exhaustion/reset scheduling retains the existing separately bounded bucket policy. These conservative limits are frozen requirements; material changes require reviewed revision, not runtime widening.
- **NFR-4** [I09/I14, G04/G05] Public compatibility claims identify exact native build/surface/platform and evidence scope; unsupported selection/identity/session operations remain unavailable, and a profile's label never implies independent account capacity or zero third-party charges.

## Candidate delivery streams for refinement

| Stream | Useful bounded outcome | Prerequisites |
|---|---|---|
| A — Profile identity and inspection | Immutable non-secret profile records, scope-safe inspection and explicit unknown/unsupported states. | Extend merged contracts/ledger and freeze qualification identity requirements; fixtures can proceed before live profile eligibility. |
| B — Coupled quota admission and exhaustion | Profile-aware bucket coupling, preserved work and truthful waiting/refusal. | A's contract plus existing MH-10/MH-16 admission/ledger; no new live route is inferred. |
| C — Authorised continuity journey | Re-admitted destination, preserved originals, compatible native resume or disclosed reconstruction and recovery evidence. | A/B, MH-15's merged grounded-handoff core, applicable S1 foundations and independently qualified source/destination bundles. |

## Dependencies

- **Capture now; qualify later.** This requirements-only change does not assert passed prerequisites or authorise product implementation/native use. Full production delivery needs applicable S1/G04/G05 and exact native account/surface qualification; an authored spec or historical shipped registry is not a route qualification.
- **MH-10 (`qualification-registry`, done)** supplies independent qualification columns/drift contracts; current `claude-strict-subscription` work and relevant Codex/native adapters remain route-specific evidence producers. Do not clear their unknown entitlement, managed-policy, purchased-credit or auxiliary-route gaps to enable this card.
- **MH-16 (`budget-ledger-s1`, done)** supplies typed observations, quota reservations, finite envelopes and structured exhaustion; extend its coupling/identity semantics without replaying its completed implementation or resetting its limits.
- **MH-15 (`native-session-continuity`, unrefined at grounding revision)** owns original-evidence handoff, native session compatibility and acknowledgement. This card consumes its minimum handoff/session contracts; MH-15 does not depend on this whole card, avoiding a construction cycle.
- **Canonical controller/control** consumes merged MH-21 foundations and the eventual `supervisor-attachment-control` operation/grant contracts. Continue through one process owner and canonical run; attach views and profile pickers do not become launch authorities.
- **Adjacent owners**: MH-5 owns broader eligible-route ranking, MH-14 parallel construction, MH-24 retrieval/dependency context and MH-25 retention/export. Account data must respect their scoped contracts; none is an excuse for secret copying or a new routing engine here.
- **Supersession/conflicts**: no predecessor is silently replaced. Historical issue sections and ADR 0002 remain historical; strict qualification governs production admission. Future design must split core, adapters, TUI and docs tasks by domain and avoid overlapping concurrent native-policy/admission work.

## Impacted components

- **Core contracts and persistence**: `internal/v2contract/records_execution.go`, `records_coordination.go`; `internal/journal/ledger.go` and additive migrations. Profile/account identities must be distinct from execution-trust Profile and canonical attempt/session references.
- **Admission and budgets**: `internal/admission/admission.go`, `qualify.go`, `reserve.go`, `internal/admissionbinding/`, `internal/billing/quota_schedule.go`; re-admit exact profile/configuration and couple actual shared limits.
- **Native evidence and environments**: `internal/adapter/adapter.go`, `internal/security/env.go`, `adapters/claudecode/{probe,settings,effective}.go`, `adapters/codex/config.go` and other individually qualified adapters. Exact native-selection operations are design-stage findings, not prescribed generic environment overrides.
- **Ownership and clients**: `internal/supervisor/exhaustion.go`, `internal/workers/`, `internal/cli/`, `internal/tui/`, `tests/e2e/` and related compatibility/limitations documentation. Each implementation task must remain domain-pure and consume reviewed upstream contracts.

## Open Questions

These are delegated engineering/evidence investigations for refinement, not new founder decision requests.

- **OQ-1 — Native profile-selection/identity boundary.** Blocks enabling a real combination: establish current primary documentation, exact native build/platform, supported selector and credential/config isolation, and a stable non-secret account observation sufficient to detect wrong-account reuse. Options: qualify a supported native home selector; qualify another documented native profile mechanism; keep selection unavailable. Default: unknown identity/isolation blocks, with no private-store reader or credential migration.
- **OQ-2 — Quota coupling and permitted continuation.** Blocks claiming independent capacity or eligible switches: determine authoritative account/organization/model/window coupling and current applicable policy for the proposed purpose. Options: independently evidenced bucket sets; conservatively coupled unknown capacity; unavailable alternative. Default: neither user naming nor confirmation establishes independence or permission to evade limits.
- **OQ-3 — Controller/schema composition.** Blocks implementation planning: define immutable profile schema/revisions, scope/redaction, source-set binding through launch and transactional admission/handoff composition against the merged controller. NFR-3 resolves the conservative count/byte/freshness/deadline/retry bounds now; design must map them to source-specific enforcement and honor any smaller merged MH-15/native limits. Options must preserve canonical ownership, idempotency and existing run envelopes; no second state machine or automatic replay of uncertain effects.
- **OQ-4 — First end-to-end evidence scope.** Blocks advertised production completion: choose the smallest independently qualified source/destination combination under the existing delegated project authority and prove wait/switch/pin/drift/recovery journeys. Synthetic multi-profile tests may proceed independently; a complete cross-harness claim requires two separately qualified bundles and MH-15 continuity evidence.

## Definition of Done

- [ ] FR-1–FR-8 and NFR-1–NFR-4 have a refined criterion/evidence map and independently reviewed domain-pure design/tasks, retaining published criterion IDs.
- [ ] Effect tests discriminate shared/independent/unknown quota coupling, identity/config drift, strict funding/authority refusals and structured versus unstructured exhaustion.
- [ ] Exhaustion preserves original work and safe ownership, while wait/cancel/switch paths retain finite envelopes, pins, provenance and no paid fallback.
- [ ] Native profile/home/account and continuity claims have separately authorised versioned evidence; unknowns remain blocked and no secret/private-session transfer is performed.
- [ ] Recovery/repeated delivery cannot create a duplicate native input, replacement writer or account-switch effect; independent acceptance binds actual candidate/contract/check revisions.
- [ ] Receipts/docs truthfully disclose unsupported platforms, lost native continuity, shared allowance/freshness limitations and return-to-native behavior.
