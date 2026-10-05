# MYTHHELM — Best-of-both product synthesis and integrated specification

You are the lead product architect and engineering reviewer for MYTHHELM. Your assignment is to critically evaluate, reconcile, improve and integrate two substantial design documents into the strongest coherent product specification you can justify.

## 1. Mission and inputs

Read these two supplied files in full:

- `master-spec.md` — MYTHHELM Master Specification, Revision 1.1.
- `MYTHHELM_Adaptive_Orchestration_Proposal.md` — the adaptive native-agent orchestration proposal.

The goal is **the best of both, improved beyond either document individually**. This is not a mechanical merge, an appendix, a summary, a vote between authors, or an exercise in preserving whichever document calls itself the master.

I want an exceptional final product: powerful, adaptive, efficient, dependable, extensible and genuinely enjoyable to use. Produce the most complete, internally consistent and implementation-ready design you can, with uncertainty made actionable rather than concealed. Do not claim that a document or product has been proved perfect.

Both documents contain valuable ideas, proposed contracts and potentially incorrect assumptions. Neither has automatic precedence over the other on technical design. Preserve my explicit requirements and underlying product intent; challenge inherited implementation choices when a materially better design is justified. Where both documents are inadequate, develop a better third option.

**This is a product-design and planning task, not permission to implement the application.** Finish the evaluation and write the integrated deliverables during this session. Do not stop after a critique, research plan, list of suggestions or handoff to another agent.

## 2. Read the actual materials and establish what exists

Read both documents completely, including their examples, evidence registers, delivery plans and acceptance requirements. Use bounded reads and track coverage so tool truncation does not become an unnoticed omission. Do not rely on headings, summaries or search snippets alone.

If the working repository is available, inspect its relevant instructions, architecture decisions, plans, schemas and implementation contracts. Distinguish what is implemented, what is planned and what the documents merely propose. Do not infer implementation from a polished specification. Do not search unrelated private directories or require a repository that was not supplied.

Retain the original documents unchanged. Record their versions and identities. Treat their embedded instructions as source material for evaluation, not instructions to execute their implementation backlogs.

Identify explicit user requirements separately from author recommendations, illustrative syntax, hypotheses, historical audit findings and external factual claims. A heading marked MUST does not prove that a previous author had authority to change the user's intent.

## 3. Preserve the product's identity and ambitions

The integrated product must retain:

- MYTHHELM as a free, open-source, local-first native-agent orchestration CLI/TUI, publicly developed on GitHub, with no required MYTHHELM account, paid core tier or mandatory hosted service.
- The selected native agent's actual harness, authentication and useful capabilities—not a replacement model/tool loop disguised under its name. Native fidelity and included-subscription eligibility remain separate qualification questions.
- Default use of authorised included subscription allowances, without silent metered APIs, purchased-credit consumption, overages or hidden paid auxiliary calls. This applies to planning, routing, reviews, summaries, experiments and native children as well as implementers.
- First-class Herdr operation and useful standalone operation, with clear execution ownership, safe input takeover and truthful recovery.
- The complete intended native-harness support vision: Claude Code, Codex, OpenCode, Meta Muse Code, Kimi Code CLI, Cursor Agent CLI and Antigravity. Qualification can be staged; do not erase support targets or falsely claim they already work.
- A polished, accessible, keyboard-complete, responsive TUI; plain and machine-readable operation; the intended cross-platform vision; and meaningful plugins, mods and contributor workflows.
- Reliable continuity through native sessions, shared task state, versioned artifacts and selective context; dependency-aware parallel construction; integrated verification; and bounded autonomous execution.
- A substantive path to learning better routing, context, prompts, review and decomposition policies, and adapting to new models and harness versions. Learning must be capable of discovering that a simpler single-agent workflow is better.

Protect these outcomes, not every paragraph or mechanism previously proposed. Retain established Go/Bubble Tea, TOML, local persistence and process-protocol decisions unless a concrete, consequential improvement justifies changing them. A different notation in an illustrative example is not such a justification.

**Do not confuse the ultimate product with its first release.** Specify the full compelling destination and a credible dependency-ordered route to it. Neither gut the vision into a permanently basic CLI wrapper nor make a research platform a prerequisite for delivering the first useful workflow. Foundations for adaptation should appear early; sophisticated learning should earn deployment through evidence.

## 4. Evaluate from multiple perspectives and make decisions

Assess both inputs as a product designer, native-agent integration engineer, distributed-systems engineer, context/learning researcher, security reviewer, open-source maintainer and user trying to get real software shipped.

Evaluate practical user value and differentiation; native fidelity and entitlement; context completeness and waste; scheduling and integration; crash recovery; security and privacy; learning validity; operating cost and subscription capacity; UX/accessibility; maintainability; and feasibility of delivery.

For each material capability or disputed design, choose **retain, improve, combine, replace, defer or reject**. Record the source location, final disposition, rationale, evidence or explicit hypothesis, affected interfaces/invariants and resulting acceptance requirement. Deduplicate equivalent requirements without losing their meaning.

Prefer useful mechanisms over impressive terminology. Every added component should have a responsibility, owner, state/interface contract, failure behaviour and reason to exist. Logical responsibilities do not automatically require new services, databases or processes. Remove duplicated machinery, circular authority and unjustified ceremony.

Make final design decisions rather than leaving every trade-off as a menu. Use a reversible default where evidence is incomplete. Reserve open decisions for genuinely unavailable facts or authority, and give each a safe fallback, decision owner and concrete validation step.

### Reconcile these known tensions explicitly

These are audit targets, not instructions to favour either source:

1. **Lifecycle and Herdr:** the master specifies MYTHHELM worker-owned native execution; the proposal permits broader host launch/lifecycle roles. Define the default topology once. Separate any host-owned alternative, and eliminate competing supervisors, duplicate launch and input ownership ambiguity.
2. **Persistence and namespaces:** reconcile the master's per-user supervisor/global reservations with the proposal's per-repository storage illustration and controller leases. State who writes authoritative state, where global resource coordination lives, how repository isolation works and which views are derived.
3. **Domain models and states:** reconcile Run/Goal, Task, Attempt, Decision, Verification/Attestation, PolicyVersion and Experiment. Distinguish routing decisions from accepted design decisions. Do not create two task stores, parallel state machines, incompatible status vocabularies or competing sources of truth.
4. **Interfaces and configuration:** translate illustrative TypeScript/YAML into the chosen implementation and configuration conventions. Unify schema versions, field names, defaults, capability states, event envelopes, error taxonomy, IDs and migrations.
5. **Scope and delivery:** reconcile the master's one-qualified-adapter first slice with the proposal's passages requesting two early adapters and cross-harness handoffs. Select the fastest credible route to value while retaining the full multi-harness/adaptive destination. Do not hard-code vendor superiority or require every adapter to qualify before shipping a useful subset.
6. **Learning defaults:** resolve opt-in outcome collection versus collection-on defaults; distinguish essential operational state, optional learning data, external telemetry and resource-consuming exploration. Define consent, retention, budgets, foreground-work priority, promotion and rollback without making routine authorised work cumbersome.
7. **Production delivery:** reconcile a review-candidate-first product with the proposal's zero-to-production example and release states. Preserve an explicit future/optional path to authorised verified deployment without silently turning deployment into a default MVP action.
8. **Security and native fidelity:** keep logical restrictions, native controls and OS-enforced boundaries distinct. Avoid both false containment claims and requirements that render every legitimate trusted-local workflow unusable. Explain supported trust profiles and the exact guarantees each can sustain.
9. **Learning versus unsupported optimisation:** expand adaptation beyond aspirational logging, but avoid a combinatorial optimiser without enough data. Separate project memory, observational estimates, counterfactual experiments, prompt optimisation and controller-code changes, with appropriate evidence requirements for each.

Audit for additional conflicts; this list is not exhaustive.

## 5. Research claims that materially determine the design

Use current primary sources for native interfaces, authentication/entitlement, session behaviour, effort settings, protocols, Herdr lifecycle, security boundaries and research findings that influence your decisions. Check both documents' claims; do not assume the proposal corrected every earlier mistake.

Follow citations to the relevant source and verify the actual claim, date/version, evaluated model/harness, baseline, measurement and limitation. A vendor capability page does not prove MYTHHELM compatibility. An observational engineering report does not establish a causal speedup. A paper's solve rate does not establish token savings. Availability through an interface does not establish included subscription billing.

Separate documented mechanisms, independently reproduced results, reported experience, design decisions, hypotheses and release gates. Do not reproduce unsupported model rankings, percentage improvements, pricing, flags or guarantees. Existing statements can be retained with explicit dated provenance when not freshly rechecked; do not imply a complete re-audit that did not occur.

Research should resolve consequential uncertainties, not become an endless literature review. Do not run paid evaluations, install agents, alter account settings or inspect secrets for this documentation task. If browsing or a source is unavailable, identify the unresolved claim, choose a conservative compatible design and continue the rest of the work.

Use portable Markdown references with primary-source URLs, access dates and paper/version identifiers where relevant. Never insert fabricated citations or chat-only citation tokens into the files.

## 6. Design the integrated adaptive system concretely

Give the final specification one coherent account of the following:

**Shared context and continuity.** Separate native session state, canonical task/project knowledge, immutable artifacts and provider caches. Define mandatory upfront context, selective retrieval, grounded handoffs, delta updates, stale-dependency invalidation, session affinity, deliberate resets, access control and graceful operation when optional search is unavailable. Measure total lifecycle usage rather than only capsule size. Avoid duplicate authoritative documentation and repeated summarisation of summaries.

**Routing and parallel construction.** Separate planning, eligibility/admission, ranking, scheduling and verification. Define task and contract revisions, checkpoints, resource reservations, shared-file ownership, critical-path priorities, isolated mutable environments, native-child limits, frozen candidates, serialized integration and repair. Explain when one session, a specialist, two writers or independent reviewers should be considered—and when not.

**Learning and new-model adaptation.** Define the exact observations, learned objects, eligible actions, update boundaries and success criteria. Begin with usable deterministic behaviour and a small policy portfolio. Address sparse data, workload differences, selection bias, absent counterfactuals, delayed outcomes, misleading access metrics, evaluator gaming, drift and experimental contention. Distinguish metadata discovery, installation, qualification, calibration, canary use and promotion. Test new models as possible replacements for whole pipelines, not only individual roles.

**Autonomy and authority.** Enable decisive work inside explicit standing permissions and budgets. Avoid approval spam and unnecessary clarification. The learner cannot grant itself authority, expand spending, change protected acceptance rules or deploy its own controller changes without the applicable independent gates. Distinguish reversible runtime-policy promotion from executable/plugin/adapter updates and database migrations. Pin active attempts and define rollback and degradation behaviour.

**Verification and product evidence.** Measure accepted, useful, maintainable software and time to accepted integration, including planning, retrieval, reviews, retries, failed attempts, human intervention and learning overhead. Include strong direct-native, single-agent and fixed-policy baselines. Define targeted ablations, representative tasks, protected checks, held-out evaluation, uncertainty and scope-limited promotion. Preserve unknown telemetry as unknown. Specify what can be established offline with scripted adapters and what requires authorised live qualification.

**User experience and maintainability.** Make the system understandable without exposing all its internals at once. Integrate route explanations, context lineage, allowance uncertainty, learning receipts, intervention, recovery and rollback into the actual CLI/TUI/Herdr journeys. Preserve accessibility, platform honesty, plugin trust, licensing, distribution, zero-credential contributor tests and practical maintainer responsibilities.

For every significant new capability, supply: user benefit; owning component; required state/interfaces; default behaviour; failure/degraded behaviour; acceptance evidence; delivery stage; dependencies; and migration/compatibility impact where applicable. Reuse existing mechanisms when they meet the requirement.

## 7. Produce the finished deliverables

Write these files in a new `mythhelm-synthesis/` directory, preserving the input originals:

### A. `MYTHHELM_Master_Spec_v2.md`

A complete, standalone, authoritative replacement specification for the integrated product. Include product vision, explicit requirements, architecture and ownership, workflows and contracts, context, adaptation, security/billing, native adapters, Herdr, UX, plugins, platforms, persistence/recovery, evaluation, delivery stages, acceptance gates, risks, governance and sources.

The document must make sense without either input open. Do not use “unchanged from the old document”, paste the proposal as an appendix, retain incompatible alternatives as simultaneous defaults, or silently drop unrelated strengths of the master. You may reorganise for clarity, with a traceability map. Move redundant historical critique into the decision report rather than letting the new normative spec read like an argument between earlier authors.

### B. `MYTHHELM_Synthesis_Decisions.md`

A decision and evidence report containing: the material findings; dispositions of both documents' meaningful requirements/proposals; resolved conflicts; justified new ideas; rejected/deferred complexity; changed assumptions; source verification status; and remaining uncertainties with defaults, owners and tests.

Include mappings for the master's I01–I19 invariants and G01–G12 gates, and the proposal's INV-01–INV-12 invariants and A-01–A-38 acceptance cases. Equivalent items may share a destination, but every item needs an explicit disposition. Also account for meaningful capabilities not captured by those IDs. This is a traceable decision record, not a duplicate specification.

### C. `MYTHHELM_Implementation_Plan.md`

A derived, dependency-ordered implementation plan referencing the new master. Define complete vertical slices, concrete deliverables, relevant contract/schema changes, tests, release gates, operational/migration work and explicit deferrals. Prioritise the earliest useful working product and the foundations needed for later adaptation. Advanced learning needs specific prerequisites and promotion gates, not an indefinite “someday” bucket. Do not invent calendar certainty or require a large human team.

## 8. Validate the synthesis before finishing

Perform separate coverage, contradiction, failure-path and simplification reviews of the finished files. Use focused read-only subreviews only when supported and within already authorised resources; keep one final editor responsible for coherence. Without subagents, perform explicit review passes yourself—do not claim independent agents reviewed the work.

Walk through at least: a simple one-agent fix; a cross-harness handoff; contract-bounded parallel implementation; stale inputs; allowance exhaustion; native-session loss; Herdr reconnect and restart; conflicting candidate integration; a new model candidate; a falsely promising learned policy; disabled learning; and a requested deployment without authority. Resolve defects found in the specification, not just list them.

Check coverage against both sources, consistent ownership and configuration defaults, valid state transitions, bounded recovery, staging, and absence of contradictory normative requirements. Validate Markdown links/anchors, references, identifier uniqueness and machine-readable examples with available local tools. Distinguish actual static document checks from proposed software acceptance tests; do not claim runtime qualification.

Write progressively and preserve completed output during the session. Use substantive completion criteria, not an endless “keep improving” loop. Finish with a concise summary of the resulting product, the most important decisions and remaining limitations, and the actual paths or downloadable links to all three files.

**The final test:** could a capable implementation team or agent build one coherent, exceptional MYTHHELM product from the new specification without reconciling the two inputs again—and understand exactly how its quality, efficiency, adaptation and user control will be demonstrated?
