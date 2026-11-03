# Routing across authorised profiles — Requirements

> Refined after independent assessment; see [refinement-log.md](refinement-log.md). Proceed to /spec authorised-profile-routing.
>
> Explain deterministic selection among explicitly authorised, independently qualified native route bundles, preserving user pins, included-only admission and finite resource envelopes. Normative source: `mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md`, version 2.0, especially §§4, 7–10, 12–13 and §18; all I/AT/G identifiers below refer to v2. This capture grants no native trial, provider selection, account change or implementation.

## Context

- **Backlog card**: MH-5
- **Issue**: [#26 — Routing across authorised profiles](https://github.com/turbokast/mythhelm/issues/26).
- **Source**: card Summary/Notes, historical issue #26 and signal S-3; v2 §§7, 10, 12–13; Implementation Plan W06/W10.
- **Grounding revision**: `615e74a6bb6907f87bd2065e2f576f5e18b5dec5`; source claims below were read at this revision.

Useful routing starts by removing routes that cannot safely execute the actual task under the user's authority. It then respects an explicit preferred qualified route, explains rejected alternatives and applies a stable configured order when evidence cannot justify another choice. A ranking score, model recommendation, plan label, sign-in or large subscription balance cannot manufacture entitlement, capability or permission.

Issue #26 uses historical Revision 1.1 section numbers and says this card replaces a fixed adapter pin. Under current v2 §10.1 this means adding explicit authorised route choice, not removing a user's binding pin or silently switching harness/provider. Account profiles are distinct from execution-trust profiles, policy families and model/effort choices. Each native surface/configuration/account/trust combination still needs its own current evidence; support in one combination does not imply parity in another.

The current registry, typed usage ledger and finite envelopes supply useful primitives; they do not establish two qualified live routes or a ranking quality advantage. Signal S-3's cheap-first, kNN and model-router ideas remain hypotheses. Deterministic P1 and truthful operational receipts must remain useful with optional smart routing and learning disabled. Advanced estimates, experimentation and adaptive promotion belong to MH-26/MH-27 and cannot become prerequisites for the basic deterministic selector.

### Premise grounding

- **HOLDS — Current dispatch selects one requested adapter.** `internal/admission/admission.go:217` enumerates the native/scripted deciders; `decideWith` selects the requested adapter at line 254, invokes that admission path and checks required proposal capabilities. This is a concrete reusable admission seam, not an existing multi-profile ranking engine.
- **HOLDS — Strict qualification already refuses missing mandatory evidence.** `internal/admission/qualify.go:96` rejects unknown billing/harness cases and, for strict mode, absent/ambiguous/drifted records, unproven entitlement and unproven stop-at-exhaustion. It does not turn unknown remaining quota into zero or sign-in into funding proof. Full route eligibility also requires the surrounding authority/trust/capability checks.
- **PARTIAL — Canonical decision vocabulary exists before routing behavior.** `internal/v2contract/records_evidence.go:41` defines eligible/excluded routes, features, selected route/policy, rationale, uncertainty and optional probability. Its structural validation at line 59 is not proof that admission ran, a score is calibrated or a route was safely launched.
- **HOLDS — Budget and bucket foundations must be preserved.** `internal/admission/reserve.go:67` derives a route/account bucket; `internal/billing/envelope.go:24` resolves immutable per-run ceilings from explicit/file/built-in layers; `internal/billing/reserve.go:37` estimates completion reserves. These are local coordination/estimates, not provider availability or hard quota guarantees.
- **PARTIAL — Historical pin and section language needs v2 reconciliation.** Issue #26's fixed-adapter non-goal and Revision 1.1 §§8/13.2/20.4 map to current §§7/9.2/10/18, preserving user pins and exact re-admission. S-3 is a historical hypothesis source, not current benchmark, native capability or vendor-policy evidence.
- **UNVERIFIABLE — Two current native bundles or a smart policy provide permitted included-only execution and better outcomes.** No live qualification or comparative experiment is performed here. Exact native/profile evidence, task-cohort support, calibrated uncertainty and comparative benefit require separate authorised work; OQ-1/OQ-4 retain those gaps.

## Objectives

- **O1**: Select only an authorised eligible native route, with deterministic preferences and a clear reason when none can run.
- **O2**: Preserve pins, qualified native capabilities, coupled allowance and finite completion resources through selection, waiting and any authorised switch.
- **O3**: Show uncertainty and provenance honestly; optional smart routing failure leaves an eligible deterministic workflow without paid calls or hidden substitutions.
- **O4**: Record immutable pre-assignment decisions and actual outcomes without claiming unproved quality, speed or resource savings.

## Non-Goals (and what still binds)

| # | Out of scope | Still binding in this spec |
|---|---|---|
| N1 | New provider onboarding, accounts, secret import, subscription purchase or a default vendor/model choice. | I01/I04/I15/I19: only configured authorised qualified native routes are candidates; unknown mandatory proof blocks. |
| N2 | Identity cycling to evade limits, metered inference fallback, paid routing/summary/review or silent runtime-SDK substitution. | I04/I15/I16: ranker, planner, reviewer, children and every auxiliary use the same admission/effect boundaries. |
| N3 | Learned-policy promotion, new experiment executor or mandatory local model/router service. | MH-26/MH-27 own evidence/promotion; deterministic P1 remains available and exploration defaults to zero. |
| N4 | Parallel construction or inventing quota capacity from more profiles. | MH-14/MH-17 own those capabilities; existing coupled buckets, ownership and one-writer constraints remain binding. |
| N5 | General native continuity or account-home implementation. | Consume MH-15/MH-17's merged contracts; a route change is not a native-session transfer, profile name is not exact identity and unknown resume stays unsupported. |
| N6 | Guaranteed success probability, speedup, hard token reserve or aggregate subscription-hours prediction. | I09/I10/I14 and §13: evidence scope, missingness, full verification cost and inconclusive results stay visible. |

## Functional Requirements

### FR-1 — Eligibility precedes ranking (§§7.1–7.4, 10.1; I02/I04/I15)

- **AC-1.1** [AT-03/AT-04, G02/G05] Before any ranking or native-assisted routing work, the system shall filter exact harness/build/surface/model/effort/account/configuration/platform/trust bundles against user selection and authority, entitlement/no-paid-continuation evidence, required native capabilities, context suitability, isolation, availability and finite budget; rejected or unknown-mandatory bundles shall never enter the ranker's actionable set.
- **AC-1.2** [I02/I09/I15] If remaining allowance alone is unknown on an otherwise qualified stop-at-exhaustion route, then the system may retain it as eligible with quantity unknown; sign-in, a user-declared quota/extra-usage statement, a paid credential, ambiguous credits or a strong task score shall never substitute for independent funding/prevention evidence across all enabled auxiliaries and descendants.
- **AC-1.3** [I02/I04/I20] Before launch, the system shall revalidate the selected bundle against the actual immutable task/profile/policy/configuration and current evidence/authority revisions; drift, stale identity, newly enabled auxiliary or changed data destination shall invalidate the decision and refuse/recompute within the original envelope, never reuse a stale eligible-set hash as authority.

### FR-2 — Deterministic preferences and pins (§10.1; I04/I16/I25)

- **AC-2.1** [AT-25, I25] When multiple bundles are eligible, the system shall honour an eligible explicit user pin/preference first and otherwise apply a stable configured order with explicit tie-breaking; identical admitted inputs and policy revisions shall produce identical deterministic selection, independent of map iteration or display ordering.
- **AC-2.2** [I04/I16] If a pinned route is exhausted, unavailable or lacks a mandatory capability, then the system shall preserve useful work and expose the precise blocker/wait condition; it shall not silently select another harness, provider, billing mode, account, trust boundary or policy family to make progress.
- **AC-2.3** [AT-12, I04/I20] When a direct user action or applicable standing grant permits alternatives, the system shall bind the fallback set and override to exact profile/policy/destination scope, announce the intended change and re-admit the destination; revocation/expiry stops future selections and an exhausted-limit identity-cycling proposal remains prohibited even with a generic confirmation.

### FR-3 — Task descriptors and qualified capabilities (§§7.1, 10.1–10.2; I01/I09/I14)

- **AC-3.1** [I09/I20] When describing a task for selection, the system shall record immutable pre-assignment task type, domain/language, change/risk/dependency shape, scope, required native capabilities, estimated context and acceptance cost with source/confidence or unknown; repository/agent prose remains untrusted input, not authority or an accepted constraint change.
- **AC-3.2** [AT-02, G02/G12] When a route advertises tools, instructions, hooks/MCP/plugins, session operations, effort/model settings or child capability, the system shall use versioned qualified evidence for the actual native surface and disclose preserved/restricted/unsupported/unobservable differences; no universal effort equivalence, hard-coded vendor-role superiority, editor parity or model-name match shall manufacture support.
- **AC-3.3** [I07/I20] If a descriptor, mandatory capability or acceptance/dependency revision changes materially, then the system shall invalidate affected routing/context evidence before reuse; selection shall not remove original requirements, protected checks or required independent review to make an otherwise unsuitable route eligible.

### FR-4 — Coupled resources and completion envelopes (§§7.3, 10.3; I10/I21/I24)

- **AC-4.1** [AT-13, I09/I10] When candidate profiles share documented account/organization/model/window limits, the system shall retain their coupled bucket set through ranking and scheduling; separate labels/homes/routes or duplicate observations shall not create fictitious independent capacity, and unknown coupling shall be conservatively constrained rather than scored as spare allowance.
- **AC-4.2** [I05/I21/I24] After ranking and before dispatch, the scheduler shall acquire the complete required local resource set transactionally under canonical ownership, retain foreground/completion priority and refuse incomplete/ambiguous claims; a ranking win or expired lease shall not release prior ownership, and local reservations shall not be advertised as provider caps or capacity guarantees.
- **AC-4.3** [AT-13/AT-26, I21] When routing, re-admitting, retrying or switching, the system shall charge any native-assisted router/planner/reviewer work and handoff overhead to the existing run/auxiliary envelope, reserve required verification/repair completion resources, and never reset execution/repair/replan/retry ceilings or purchase continuation because a new route was selected.

### FR-5 — Optional smart routing fails safely (§§10.1, 12–13; I03/I11/I15/I22)

- **AC-5.1** [AT-25/AT-33] When smart routing is disabled, absent, slow, invalid or lacks task-cohort evidence, the system shall select through the same eligible deterministic P1/preference policy; if no P1-compatible authorised bundle exists, it shall report that blocker rather than invent support or invoke a paid fallback. Basic operation shall require no router model, network service or historical training data.
- **AC-5.2** [I02/I03/I11] If an optional router proposes an unknown/duplicate/ineligible route, malformed/nonfinite score, unsupported capability, widened scope or altered protected rule, then the system shall reject the proposal and retain the original eligibility/authority boundary; plugin/model output shall not create a new candidate, grant or launch path.
- **AC-5.3** [AT-03/AT-04/AT-25] Before any native-assisted router call is enabled, the router itself shall be an explicitly authorised independently qualified included-only auxiliary with bounded input/output/deadline and declared context destination; timeout/error shall not trigger another model/paid service, secret extraction or unaccounted retry, and the deterministic alternative shall remain usable.

### FR-6 — Immutable explanations and evidence labels (§§4.1, 7.3, 10.1, 17.2; I07/I09/I20/I23)

- **AC-6.1** [AT-25/AT-35] Before assignment, the system shall record a canonical RoutingDecision referencing exact task/profile/policy/evidence revisions, eligible set and exclusions, selected native identity, preference/override authority, capability/fidelity gaps, coupled buckets, score uncertainty and decision rationale; rejected alternatives shall remain inspectable within authorised repository scope without leaking account secrets or foreign metadata.
- **AC-6.2** [I09, §§7.3/13.1] When presenting usage, cost, quality, latency or success estimates, the system shall distinguish `reported | observed | estimated | user-declared | unknown`, source/scope/timestamp and normalization/evaluator version; missing is not zero, native retail pricing is not a subscription invoice, unlike allowances are not comparable currency and unknown probabilities shall not be fabricated.
- **AC-6.3** [I07/I20] When candidate production and verification complete or fail, the system shall link actual accepted/rejected/blocked/cancelled/timed-out/unverified outcomes and delayed corrections to the original pre-assignment decision without rewriting it; a native completion, router confidence or handoff acknowledgement shall not certify independent acceptance.

### FR-7 — Exhaustion, continuity and user control (§§7.3, 9.2, 10.3; I06/I12/I16/I25)

- **AC-7.1** [AT-13/AT-14/AT-15] When structured exhaustion or capability failure prevents the chosen route, the system shall preserve candidate/artifact/session and original-contract references, stop new work on affected buckets and offer only known-reset waiting or an explicitly authorised independently admitted alternative; unknown reset stays unknown and no summary/repair route may consume paid continuation.
- **AC-7.2** [I04/I12/I18/I25] When switching under valid authority, the system shall reconcile the old writer/effects, preserve the existing run envelope and distinguish compatible native resume from a new session reconstructed through MH-15's validated handoff; cancel/revoke/pin controls shall stop future route changes, and restored client state shall not replay an assignment or grant.

### FR-8 — Discriminating tests and honest comparisons (§13, §18; I07/I14/I22)

- **AC-8.1** [AT-03/AT-04/AT-25/AT-33] Before advertising routing, credential-free effect tests shall show that an attractive but ineligible route loses to an eligible lower-score route, an unavailable pin stays blocked, a valid scoped fallback is re-admitted, shared quotas remain coupled, native/config drift invalidates selection and malformed/timed-out smart routing returns only to eligible deterministic P1; removing the pre-ranking filter or pin check shall make a named test fail.
- **AC-8.2** [I07/I09/I14, G14] Before claiming measured quality/speed/resource benefit or promoting a learned route policy, separate reviewed MH-26/MH-27 evidence shall compare matched direct-native and strong fixed-policy baselines using protected acceptance, complete verification/handoff/review costs, all-started denominators, missing/censored outcomes, independent sampling and uncertainty. Sparse, development-only or inconclusive data shall retain deterministic selection and shall not be called an advantage.

## Non-Functional Requirements

- **NFR-1** [I13/I19] Ordinary contributor tests run with synthetic route/profile/qualification/clock/process fixtures, no agent credentials, inference, external account action, native-store extraction or new provider dependency. Live native and performance qualification requires separate explicit applicable authority.
- **NFR-2** [I21, §4.3] One decision shall consider at most 64 authorised profiles and 128 exact route bundles, a 64 KiB encoded descriptor and 128 capability requirements; its canonical envelope shall remain at most 1 MiB/64 nesting. Oversized input or incomplete eligibility enumeration shall refuse without selecting an unchecked prefix. Optional local evidence queries are bounded to 10,000 already-authorised outcome records; this does not authorise backfill, new collection or retention changes.
- **NFR-3** [I20/I21] Deterministic ranking over an already resolved immutable eligibility snapshot shall complete within 2 seconds; optional router proposal evaluation has a 2-second total deadline and at most one proposal call, with no automatic inference retry. A separate fresh admission/identity/config observation operation has at most 30 seconds including any at-most-two effect-free retries after 100/500 ms, further bounded by the remaining enclosing deadline. Identity observations shall be at most 30 seconds old at start, with unchanged admitted profile/config/evidence/grant revisions through launch; age alone never establishes freshness or closes a read/start race. Existing smaller transport/native deadlines and run retry ceilings win; uncertainty cannot renew a deadline.
- **NFR-4** [I07/I20/I23] Active attempts pin their route/policy/evaluator revisions; new policy revisions affect only authorised future assignments, with no hot-swap or second scheduler/task store. Timing/quality targets are acceptance boundaries to demonstrate, not claims that a route currently meets them; probability thresholds require calibrated task-cohort evidence before use.

## Candidate delivery streams for refinement

| Stream | Useful bounded outcome | Prerequisites |
|---|---|---|
| A — Deterministic eligibility and explanation | Select or precisely refuse a configured bundle using current qualification, pins, stable ordering and immutable receipts. | Merged admission/controller/ledger contracts; synthetic eligibility design can proceed now without live two-route claims. |
| B — Profile/resource-aware continuity | Preserve coupled buckets, completion reserves and explicit authority through wait/re-admission/handoff. | A's frozen contract, MH-16 budget semantics and merged MH-17/MH-15 minimum profile/handoff contracts. |
| C — Optional proposal boundary and comparison | Reject invalid/slow smart proposals and keep deterministic P1; record provenance without unproved performance claims. | A; any native router separately qualifies under the same billing/authority rules. Learned estimates/promotion/experiments remain downstream MH-26/MH-27. |

## Dependencies

- **Capture now; production eligibility later.** Requirements and synthetic contracts do not assert S1 or two qualified routes. Complete MH-5's native multi-route claim only after applicable S1, MH-21 canonical foundations and two independently qualified exact bundles pass relevant G02/G05/G12 evidence.
- **MH-10 (`qualification-registry`, done)** supplies eligibility evidence/consult contracts; `claude-strict-subscription` and relevant native adapter work remain independent route producers. Current managed-policy, identity, credit, auxiliary-route and platform gaps cannot be cleared to make ranking useful.
- **MH-16 (`budget-ledger-s1`, done)** supplies typed observations, coupled reservation/envelope foundations and exhaustion. Preserve the already resolved finite run contract; this card does not silently replace its historic built-ins with different v2 defaults or restart limits at a route boundary.
- **MH-15 (`native-session-continuity`, unrefined at grounding revision)** supplies validated handoff, acknowledgement and exact-session compatibility. **MH-17 account-profile requirements**, captured separately, own exact non-secret identity, native-home isolation and shared-bucket semantics; consume merged minimal contracts rather than require either entire downstream journey before constructing A.
- **Current `supervisor-attachment-control` and related client work** own authenticated operations, one-use admission, canonical ownership and projections. Routing remains a proposal/selection input to those launch boundaries; no client/plugin becomes an alternative process owner.
- **MH-26/MH-27** own observational estimates, controlled experiments and learned promotion; MH-5 basic deterministic operation has no dependency on their whole implementation. MH-14 owns parallel policy construction and native-child envelopes; MH-25 owns scoped retention/erase obligations for any retained routing observations.
- **Supersession**: preserve user pins, canonical decision/contract records and current registry/budget behavior. Historical issue/S-3 references retain their source revision; no capability matrix, benchmark claim or model-router hypothesis silently supersedes v2 admission/evidence requirements. Future design splits core/adapters/TUI/release/docs work by domain.

## Impacted components

- **Admission/qualification**: `internal/admission/admission.go`, `qualify.go`, `reserve.go`, `internal/qualify/`, `internal/admissionbinding/`; exact task/profile/evidence binding and revalidation precede ranking and final launch.
- **Canonical decisions/budgets**: `internal/v2contract/records_evidence.go`, `records_coordination.go`, `internal/journal/`, `internal/billing/{envelope,reserve,quota_schedule}.go`; preserve one ledger, immutable pre-assignment decisions, coupled quotas and full-envelope accounting.
- **Native capabilities and continuity**: `internal/adapter/adapter.go`, individually qualified `adapters/` and merged MH-15/MH-17 contracts. No generic environment/provider rewrite or unsupported session operation is prescribed here.
- **User controls and evidence**: `internal/cli/`, `internal/tui/`, `tests/e2e/` and truthful compatibility/limitations documentation; exact ranking interfaces and package layout belong to the later independently reviewed design.

## Open Questions

These are delegated engineering/evidence investigations, not new founder decisions; conservative numeric limits are already frozen in NFR-2/NFR-3.

- **OQ-1 — Exact first eligible set and authority composition.** Blocks production selection, not requirement capture: bind current independent native qualification, full auxiliary tree, scoped profile identity, pins/grants and task capabilities to the existing admission controller. Options: one eligible pinned route with explicit exclusions; two or more independently qualified alternatives; precise blocked outcome. No new provider or account is selected by this document.
- **OQ-2 — Immutable snapshot and launch-race enforcement.** Blocks implementation interfaces: compose the eligibility snapshot, profile/policy/config/grant/evidence revisions, stable tie order and transactional reservation/launch check with merged controller contracts. Default: any unknown/stale mandatory input refuses; a score or time-based cache TTL cannot replace source binding.
- **OQ-3 — Minimum profile/handoff composition.** Blocks full profile-aware switching: identify the independently useful merged MH-15/MH-17 contract checkpoints, shared-bucket mapping and ownership/uncertain-delivery recovery. Default: deterministic selection can be built with fixtures first; unsupported native continuity stays disclosed and never becomes hidden reconstruction or identity rotation.
- **OQ-4 — Optional evidence and hypothesis evaluation.** Blocks calibrated scores, comparative benefit and learned promotion: define authorised cohorts, permitted retained outcomes, calibration/uncertainty and protected matched evaluations through MH-26/MH-27. Cheap-first, kNN and model-router hypotheses may be tested only there under valid authority; no imported benchmark or arbitrary sample threshold qualifies them. Default: sparse/unknown evidence uses stable deterministic preferences and records uncertainty.

## Definition of Done

- [ ] FR-1–FR-8/NFR-1–NFR-4 have a refined criterion/evidence map and independently reviewed domain-pure design/tasks, retaining published IDs.
- [ ] Effect tests prove eligibility before ranking, user-pin/standing-grant scope, exact-bundle drift refusals and truthful deterministic explanations.
- [ ] Coupled reservations, exhaustion, handoff and finite completion envelopes survive selection/retry/switch without paid fallback or a second owner.
- [ ] Disabled/invalid/slow optional routing retains eligible P1 and cannot widen candidates, authority, native surfaces or protected acceptance.
- [ ] Actual native multi-route claims carry two independent exact-bundle qualifications; comparative/adaptive claims remain gated by separately reviewed evidence with uncertainty and all-started outcomes.
- [ ] Receipts, clients and docs expose unsupported operations, rejected alternatives, missing data and user controls without pretending that a ranking decision is accepted delivery.
