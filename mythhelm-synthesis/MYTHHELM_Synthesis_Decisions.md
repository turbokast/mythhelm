# MYTHHELM synthesis decisions and evidence

Version 1.0 · 5 October 2026 · Prepared by Codex as the single final editor

The [v2 master](MYTHHELM_Master_Spec_v2.md) is the integrated design; the [implementation plan](MYTHHELM_Implementation_Plan.md) derives delivery work. This report explains dispositions and evidence without making either source the automatic technical authority. It records the original synthesis handoff; the later authorised product-layer integration is tracked in the [adoption map](../docs/spec/synthesis-adoption.md). No application implementation, paid evaluation, account change, installation or deployment was authorised or performed for this task.

<a id="provenance"></a>
## 1. Inputs, authority and reading coverage

| Input | Identity | SHA-256 |
|---|---|---|
| [Master](../docs/spec/master-spec.md), abbreviated M | Revision 1.1, 29 September 2026; 2,407 lines | `a6945ff6f40565c45529c139e9b4b779a3ca931c99d155dc52f04b634684fcf0` |
| [Adaptive proposal](../MYTHHELM_Adaptive_Orchestration_Proposal.md), abbreviated P | Version 1.0, prepared 5 October 2026; 1,666 lines | `6180a9664be56c3443c42d0aa3d1aeebc46e98ef4b3f88616a0c6616f2f3eec7` |
| [Synthesis prompt](../MYTHHELM_Best_of_Both_Synthesis_Prompt.md) | User-invoked task and deliverable contract | `0f7a2925c958b39f1c88c29a68984ac816fa07a870ebfd9eeb2c2bcc6c2ae6b3` |

M lines 1–2407 and P lines 1–1666 were read in bounded chunks, including examples, source registers, delivery plans and acceptance tables. Truncated tool windows were reread over smaller ranges. Headings and search snippets were used for navigation, not as substitutes for the source text. Both source files and the prompt are preserved byte-for-byte.

Authority classification:

- The current user request and invoked prompt supply explicit requirements: v2 UR-01–UR-10. They authorise research and these three planning documents.
- The sources' technical mechanisms, defaults, milestones and MUST labels are design proposals to assess. Their backlogs are not execution instructions for this session.
- M's earlier supplied conversation and Revision 1.0 were not separately supplied. Their hashes/claims are historical provenance in M, not independently re-read user instructions.
- Provider documents establish documented mechanisms as of access, not installed compatibility, legal clearance for every distribution, included billing for an account, or measured MYTHHELM performance.
- Research establishes results for its evaluated systems and workloads. It does not establish a MYTHHELM efficiency result. None was independently reproduced here.

The work is documentation, outside the active application backlog. Repository bootstrap instructions were read, and delivery status was inspected without taking over or modifying the existing run. The requested `mythhelm-synthesis/` destination is used as a documentation artifact directory; this task does not edit domain routing, `product/`, `orchestration/`, active implementation specs or the old master.

<a id="repository"></a>
## 2. Inspected repository and changed premises

The fixed inspection baseline is `d77f2e8e145c3c2d161cf5b4d7164af4ad4dcd8e`; the shared main checkout can advance independently. Work was prepared in an isolated documentation worktree. Repository tests and recorded evidence are inspected facts, not tests rerun or certifications supplied by this review.

| Evidence at that revision | Finding | Consequence |
|---|---|---|
| [README](../README.md), `cmd/mythhelm/`, `tests/e2e/`, completed dogfood/TUI specs | Headless workflow and TUI exist; no usable release is claimed. README retains a stale broad introductory claim that everything after dogfood is proposed, despite its specific implemented TUI section. | Plan extends existing code and reconciles status docs during implementation. It does not restart from scaffolding. |
| [Go dependencies](../go.mod) | Go 1.27.0/toolchain 1.27.1, Bubble Tea v1.3.10, TOML library and modernc SQLite v1.59.0 are pinned. | Preserve stack and reviewed pins; do not assume newer upstream rendering behavior is already in this tree. |
| [ADR 0001](../docs/decisions/0001-license-and-contribution-policy.md) | Accepted Apache-2.0 and DCO 1.1. | Licensing is established repository policy, not an open recommendation. |
| [ADR 0003](../docs/decisions/0003-local-state-sqlite.md), [schema v1](../internal/journal/migrations/0001_init.sql) | One user state root and `mythhelm.db`; multiple run supervisors serialize through SQLite. Journal events and projections commit together; append-only journal; versioned backups. | Keep DB location and event semantics; migrate to one service writer, add task/policy/experiment records. Do not create per-repository databases. |
| [Adapter contract](../internal/adapter/adapter.go), [ADR 0004](../docs/decisions/0004-slice-process-model.md), [ADR 0005](../docs/decisions/0005-run-owner-lock-and-ingestion.md) | Worker owns launch, detached process and spool; CLI owns run lock; no daemon/IPC. Windows native tree ownership remains limited. | Preserve worker/launcher seam. A per-user service and full Windows qualification are actual new work. |
| [ADR 0002](../docs/decisions/0002-dogfood-billing-posture.md), `internal/admission/billing.go` | Strict subscription-only always refuses; declared posture is unverified. Native startup inventory has documented gaps. | Existing Claude support is not G05 success. Qualification is the first real-user milestone blocker, with no vendor predetermined as winner. |
| [ADR 0006](../docs/decisions/0006-guarded-branch-apply.md), [ADR 0007](../docs/decisions/0007-stop-and-recovery.md) | Guarded ref creation and reconcile-first recovery already have explicit contracts; some ADR headers still say proposed despite implementation evidence. | Reuse behavior and tests; don't infer maintainer adoption merely from a file's existence or implementation. |
| [Project config](../internal/admission/projectconfig.go) | Actual v1 format covers checks, environment passthrough and Claude tool rules; illustrative source formats are broader. | Introduce explicit config v2 migration. The synthesis example is not parseable by the old app by fiat. |
| [TUI evidence](../docs/tui-slice-g09-evidence.md) | Recorded GNOME Terminal/zsh combination; no human screen-reader qualification; quit-choice and cancellation/detach limitations recorded. | Carry these into first milestone acceptance and the honest platform matrix. |
| Source tree inventory | No `hosts/`, public `protocol/`, `sdk/` or `evals/` implementation at the baseline. | These are planned contract slices, not presumed reusable implementations. |

<a id="decisions"></a>
## 3. Material design decisions

Dispositions mean: **retain** the outcome/contract; **improve** it materially; **combine** compatible strengths into one mechanism; **replace** a conflicting mechanism; **defer** a concrete optional mechanism with a trigger; **reject** an unjustified or incompatible mechanism. Evidence references below use the master [source register](MYTHHELM_Master_Spec_v2.md#sources); design defaults are decisions, not reproduced findings. AT identifiers refer to its [acceptance cases](MYTHHELM_Master_Spec_v2.md#acceptance).

| Decision | Source locations and disposition | Rationale, final contract and affected evidence |
|---|---|---|
| D01 Product identity | M §§3,22; P §§1,4,23: **retain**. | Free/local/native/Herdr/accessible/extensible identity and all seven targets are explicit user outcomes, now UR-01–UR-10. Avoid mandatory paid services and account pooling. AT-01–AT-04, AT-36, AT-46. |
| D02 Technical stack | M §6; P §§7.5,17: **retain** Go/Bubble Tea/TOML/SQLite/process protocol; **reject** notation-driven language rewrite. | Real repo already uses these. Translate P's illustrative TypeScript/YAML, preserve the worker/launcher seam, and keep protocol/config/DB versions distinct. v2 §§3–5,16; AT-38, AT-41. |
| D03 Execution owner | M §§6–7,16.6; P §§5,15,16.2: **combine** with one default. | Per-user service coordinates; worker owns native process; host/UI only attaches. Host-owned launch **deferred** until a full transfer/recovery contract earns G12. Herdr restart behavior makes competing auto-resume unsafe (SRC-21/22). I06/I17/I18/I24; AT-06–AT-10. |
| D04 Persistence topology | M §§6.5,7,13.5; P §7.4: **replace** per-repository live DB illustration. | One user/host ledger and global reservations; scoped repository artifacts/indexes. Single instance lock covers alternate active state roots. Current DB migrated, not discarded. SQLite/Git facts do not imply sandboxing. I23; AT-16, AT-20, AT-41–AT-43. |
| D05 Domain model | M §7.1; P §7.1: **combine**. | Goal is Run data; Mission UI alias; one TaskRevision and Attempt model. Split RoutingDecision/DesignDecision; Verification subsumes Attestation. Handoffs are artifacts, not another store. v2 §4; AT-14, AT-18, AT-23, AT-41. |
| D06 Lifecycle vocabulary | M §7.2; P §§9,10.5,15,17: **replace** conflicting status vocabularies with explicit tables. | Waiting reasons/pause are orthogonal, resume creates a new attempt, reconnect does not. Optional Release state is an effect projection, never another controller. Native success is not acceptance. v2 §§6,11; AT-06–AT-09, AT-23/24, AT-44/45. |
| D07 First useful scope | M §20.3; P §§1.3,20.2: **improve** staged route to value. | One fully qualified route first; second harness after a usable complete loop. Existing Claude code is an asset, not proof it should win qualification. Rich learning is not a first milestone dependency. v2 S0/S1/S2; AT-02–AT-04, AT-48. |
| D08 Native identity and auth | M §§9,13; P §6: **retain** separate fidelity and entitlement tests. | Native executable/control first; runtime SDK independently qualified, no secret replay or imported harness identity. Documented partner/auth paths do not override the user's native-owned credential requirement. v2 §7; I01/I15/I16/I19; AT-02–AT-04. |
| D09 Subscription capacity | M §13; P §§6.4–6.5,9.5: **combine**. | Include auxiliaries/children and paid-credit continuation; distinguish unknown quota from unknown protection. Reserve locally without inventing provider caps or a fixed percent reserve. Completion envelope beats exploration. AT-03/04, AT-13, AT-19, AT-26. |
| D10 Honest security | M §§11–12; P §§14.2–14.3: **improve**. | Explicit inspect/restricted/trusted-host profiles. Default restricted; trusted local execution is useful with clear guarantees. Logical broker rules cannot constrain malicious same-user code. Startup configuration is admitted before execution. v2 §8; I03/I05/I11; AT-11/12, AT-47. |
| D11 Approval friction | M §12.3; P §16.5: **combine**. | Standing narrowly scoped grants authorize routine decisive work. Missing permission blocks dependent effects after candidate preparation. No repeated confirmations inside a valid grant, no learner-created authority. v2 §§8,11–12; AT-12, AT-30, AT-44. |
| D12 Canonical context | M §10; P §§7–8: **combine**. | Mandatory original requirements plus selective retrieval, evidence-linked handoffs, native affinity, deliberate resets and revision-aware invalidation. Keep native instructions. No duplicate editable wiki or summary-of-summary authority. v2 §9; AT-14–AT-18. |
| D13 Search scope | M §§10,14; P §§8.4,14.4: **improve**. | Exact references/lexical search first; optional FTS5; embeddings deferred until ablation shows value. ACL filters content and metadata before retrieval. Search outage is not a delivery outage. AT-16/17, AT-43. |
| D14 Routing | M §8; P §§9,11: **combine**. | Plan, admission, ranking, scheduling and verification separate. Deterministic preference default, negotiated native effort, no fixed vendor specialisms. Router proposals cannot add eligibility. v2 §10; AT-25. |
| D15 Parallel construction | M §§8.7,11; P §9: **improve**. | Default one writer; first parallel envelope two. Explicit dependency contracts, isolated mutable services, native-child accounting, fair critical-path scheduling and serialized integration. No live rebases. v2 §§10–11; AT-18–AT-21, AT-26. |
| D16 Protected acceptance | M §§11.5–11.6,18; P §§10,13: **combine**. | Freeze checks and candidate; reject weakened evaluator; exact combined revision accepted. Unverified previews cannot inherit v1 ready-for-review success semantics. Model reviewers advisory unless acceptance contract independently assigns a reviewed role. AT-21–AT-24, AT-30. |
| D17 Production destination | M §§5,11.7,20; P §§10.5,19: **combine**. | Review candidate default; concrete optional deployment/health/rollback path. Prepared release can succeed as a preparation request; deployment request without authority stays blocked. Effects reconcile after uncertainty. v2 §11; AT-09, AT-44/45. |
| D18 Consent defaults | M §§8.9,17.2; P §§11,14.6,18: **replace** ambiguous collection-on/default learning. | Essential operational state on; optional learning off; historical backfill separate; exploration off/budget zero; external telemetry/raw capture/refresh off. Existing config never silently opts in. v2 §§5,8,12; AT-27/28, AT-33, AT-43. |
| D19 Learning substance | M §8.9; P §§11–13: **improve**. | Six distinct layers, exact observations, P1–P4, controlled experiments, protected promotion, active-attempt pins and rollback. Project facts are not route scores; code changes use PR/release governance. v2 §§12–13; AT-27–AT-34. |
| D20 Optimization limits | M §8.6; P §§11.3,20.3: **defer** general combinatorial search; **reject** automatic code/evaluator self-deployment. | Sparse local outcomes do not support an arbitrary workflow optimizer. New mechanisms require a specific unresolved decision, data and comparison against P1–P4. GEPA/RouteLLM motivate limited experiments, not unrestricted autonomy. AT-29/30/34. |
| D21 New models | M §§9.8,20.6; P §12: **improve**. | Discovery, installation, qualification, calibration, canary and promotion are distinct. Test replacing a whole pipeline. Drift blocks affected routes, not all useful fixed execution. v2 §12.4; AT-32/34. |
| D22 Evaluation | M §18; P §§3,13: **combine**. | Strong direct-native/single/fixed/homogeneous baselines; all-started denominator, missingness, delayed regressions, heldouts and task-dependent uncertainty. No borrowed rankings or improvement percentages. v2 §13; AT-28–AT-34. |
| D23 Failure and disk bounds | M §§7,17.5; P §15: **improve**. | Durable intents, bounded retries/reconciliation, quarantine on uncertain ownership, emergency stop channel when DB fails, no lease-only reclaim or blind effect replay. v2 §§5–6,17; AT-06–AT-09, AT-42. |
| D24 Retention versus immutable history | M §17.6; P §§7,14.6,22 A-36; real v1 append-only triggers: **replace** underspecified deletion. | Retained event history immutable; authorized erase uses quiescent validated rewrite plus artifact/index/backup accounting. Prevent false claims of erasing provider history. v2 §5.3; AT-43. |
| D25 UX and motion | M §15; P §16: **combine**. | Preserve actual diff, four responsive regimes, ten truthful motion moments, independent color/motion, accessible linear output and full keyboard journeys. Add route/context/learning detail progressively. AT-35–AT-37, AT-48. |
| D26 Extension system | M §14; P §§14.5,17: **retain** small process/data seams; **defer** sandboxed-code marketplace. | Mods useful immediately, public protocol after exercised consumers, no shell-template pseudo-adapter. Process isolation ≠ security isolation. v2 §16; AT-38/39. |
| D27 Platforms and distribution | M §§16,19; P §§18,20.4: **improve** based on actual repo. | Full three-OS/architecture vision, independent native/terminal/security matrices, Windows ownership work and human accessibility evidence. Existing packaged tests are not blanket native certification. AT-40, AT-46–AT-48. |
| D28 Governance and feasibility | M §§19–22; P §§20–25: **combine**. | Existing Apache-2.0/DCO policy retained; no invented staffing/calendar. S0–S6 build on inspected code; maintainer adoption separate. All additions use existing owner/ledger contracts where possible. AT-41, AT-46. |

The substantial third options are a global ledger with repository-scoped content (D04), unified evidence and decision vocabulary (D05), explicit privacy-preserving journal erasure (D24), and a single-controller bounded policy portfolio (D03/D19). They resolve source conflicts without multiplying services or losing the adaptive destination.

<a id="invariant-mapping"></a>
## 4. Required invariant mappings

### Master M invariants

Every M invariant remains substantively binding. Scope clarifications prevent impossible or misleading guarantees.

| Source ID | Disposition and v2 destination | Acceptance |
|---|---|---|
| I01 | **Improve** → v2 I01, I16, §7: native engine first; runtime-SDK parity independently proved. | AT-02 |
| I02 | **Improve** → I02, §§7–8: admission evidence before launch; distinguish unknown quota/protection. | AT-03/04, AT-11 |
| I03 | **Retain** → I03, I22, §8: content cannot grant authority. | AT-12, AT-30 |
| I04 | **Improve** → I04, §10: standing permission can allow explicit re-admitted changes; no silent substitution. | AT-03/04, AT-25 |
| I05 | **Improve** → I05, I24, §10.3: one writer plus separate resource/isolation guarantees. | AT-19/20 |
| I06 | **Retain** → I06, §6: detach and confirmed stop distinguished. | AT-06–AT-10 |
| I07 | **Improve** → I07, I20, §11: protected evidence at exact final revision. | AT-21–AT-24 |
| I08 | **Retain** → I08, §11.3: preserve original files/index/existing refs; scoped apply. | AT-05 |
| I09 | **Improve** → I09, §§7.3,12.2: add declaration provenance, counters and censoring. | AT-13, AT-28 |
| I10 | **Retain** → I10, §§7.3,10.3: reservations never masquerade as provider hard caps. | AT-04, AT-13, AT-26 |
| I11 | **Retain** → I11, §§8,16: supported APIs constrained, trusted-host code disclosed. | AT-12, AT-38, AT-47 |
| I12 | **Retain** → I12, §§6,11.3: reconcile then retry, never execute from event replay. | AT-06, AT-09 |
| I13 | **Retain** → I13, §§1,15–17: no MYTHHELM service/font/paid-router dependency. | AT-01, AT-36 |
| I14 | **Improve** → I14, §§7,13,17: versioned qualification and drift expiry. | AT-02, AT-32, AT-40 |
| I15 | **Retain** → I15, §7: included-only excludes purchased credits and paid auxiliaries. | AT-03/04, AT-13 |
| I16 | **Retain** → I16, §7: model identity does not establish harness identity. | AT-02, AT-25 |
| I17 | **Retain** → I17, §14: Herdr projection never certifies or launches. | AT-07, AT-10 |
| I18 | **Improve** → I18, §§6,14: explicit fenced input takeover and restore behavior. | AT-06/07, AT-10 |
| I19 | **Retain** → I19, §§7–8: native credentials stay native-owned. | AT-02–AT-04 |

### Proposal P invariants

| Source ID | Disposition and v2 destination | Acceptance |
|---|---|---|
| INV-01 | **Retain** → UR-01/I13, §§1,17: free/local/public product. | AT-01, AT-46 |
| INV-02 | **Combine** → I01/I16/I19, §7: native preservation and auth. | AT-02–AT-04 |
| INV-03 | **Combine** → I15, §7.3: all model-consuming roles and descendants. | AT-03/04, AT-19 |
| INV-04 | **Improve** → I17/I18, §14: first-class Herdr, one default owner. | AT-07, AT-10, AT-48 |
| INV-05 | **Replace** multiple-controller interpretation → I18/I23, §§3,6: one host supervisor generation and fenced worker assignments. | AT-06/07, AT-41 |
| INV-06 | **Combine** → I07/I20, §11: evidence before exact acceptance. | AT-21–AT-24 |
| INV-07 | **Improve** → I21, §§8,10,12: finite default envelopes and standing grants. | AT-08, AT-12, AT-26 |
| INV-08 | **Combine** → I05/I08/I24, §§8,10–11: isolated mutation, honest enforcement. | AT-05, AT-19/20, AT-47 |
| INV-09 | **Combine** → I20, §§4–5: immutable identities/revisions and scoped evidence. | AT-18, AT-23, AT-41 |
| INV-10 | **Improve** → I22, §12: protected gate, no executable/evaluator self-promotion. | AT-29–AT-31 |
| INV-11 | **Combine** → I09/I10, §§7,13: unknown remains unknown in UI and evaluation. | AT-13, AT-28, AT-35 |
| INV-12 | **Improve** → I25, §§5,8,12,15: pin, disable, export, erase, inspect and native exit path. | AT-27, AT-33, AT-43, AT-48 |

<a id="gate-mapping"></a>
## 5. Master release-gate mapping

| Source gate | Disposition and v2 destination | What changed |
|---|---|---|
| G01 | **Retain** → G01, AT-01/46. | Offline core/contributor proof remains independent of paid live qualification. |
| G02 | **Improve** → G02, AT-02/14/15. | Add context continuity evidence without claiming hidden native-state portability. |
| G03 | **Retain** → G03, AT-05/23. | Exact apply and candidate evidence reuse. |
| G04 | **Improve** → G04, AT-06–AT-09/19/40–AT-42. | One service migration, children and persisted uncertainty included. |
| G05 | **Improve** → G05, AT-03/04/13/19/26. | Experiments/auxiliaries, bought-credit protection and shared allowances explicit. |
| G06 | **Improve** → G06, AT-18/21–AT-24. | Stale dependency invalidation and protected checks included. |
| G07 | **Improve** → G07, AT-11/12/16/30/47. | Native startup, context privacy and learner authority attacks covered. |
| G08 | **Retain** → G08, AT-38/39. | Scope staged by mod/plugin class; no pretend process sandbox. |
| G09 | **Improve** → G09, AT-35–AT-37/48. | Includes implemented TUI gaps and new learning/context explanations. |
| G10 | **Retain** → G10, AT-40/46. | Accepted license policy and precise public support matrix. |
| G11 | **Improve** → G11, AT-07/10/36/48. | Actual restart/native-restore collision tested; fallback embedded TUI defined. |
| G12 | **Retain** → G12. | Alternative surfaces separately pass native/billing/lifecycle/security evidence; host ownership deferred. |

New G13–G16 isolate context continuity, adaptive validity, optional production delivery and durable compatibility. They supplement rather than duplicate a second release system.

<a id="proposal-cases"></a>
## 6. Proposal acceptance-case mapping

Every A-01–A-38 has a retained or strengthened outcome; stages are restaged to the v2 dependency graph, not lost. The test text remains normative in the master, so this table is a trace rather than a second specification.

| Source case | Disposition → v2 acceptance | Contract and staging decision |
|---|---|---|
| A-01 | **Retain** → AT-02/03. | Exact harness/model/auth route; S0 onward. |
| A-02 | **Improve** → AT-04. | Effective paid paths rejected/avoided, no secret extraction or vendor auth rewriting; S0. |
| A-03 | **Improve** → AT-04. | Unknown protection blocks all strict dispatch, not only unattended use; S0. |
| A-04 | **Retain** → AT-03. | Meta-model/auxiliary calls share full admission; earliest enabled stage. |
| A-05 | **Improve** → AT-14. | Grounded cross-harness handoff with original constraints; S2 after one useful route. |
| A-06 | **Retain** → AT-14. | Correct session and acknowledged delta; S2. |
| A-07 | **Retain** → AT-15. | Artifact reconstruction with explicit continuity loss; S2. |
| A-08 | **Retain** → AT-22/24. | Native claim insufficient; S1. |
| A-09 | **Improve** → AT-22. | Protected checks/definitions cannot be weakened by candidate; S1. |
| A-10 | **Retain** → AT-23. | Exact candidate/environment/check revisions; S1. |
| A-11 | **Improve** → AT-16. | Deny metadata/count/hash leaks as well as content; base isolation S1, retrieval S2. |
| A-12 | **Improve** → AT-11/47. | Effective startup inventory and profile enforcement before native launch; S0. |
| A-13 | **Retain** → AT-06. | Adopt/reconcile same launch after crash, never duplicate; S1. |
| A-14 | **Retain** → AT-10. | Reconnect view only; S1. |
| A-15 | **Improve** → AT-10. | Disable/reject conflicting host native restore; restart tested independently; S1. |
| A-16 | **Improve** → AT-08. | Partial effects plus bounded stopping/quarantine; S1. |
| A-17 | **Retain** → AT-07. | Late evidence retained quarantined without acceptance authority; S1. |
| A-18 | **Improve** → AT-07/05. | Input lease, automation suspension, source/user-change reconciliation; S1. |
| A-19 | **Improve** → AT-20. | Separate mutable environment, complete resource acquisition, one active writer; S3. |
| A-20 | **Retain** → AT-18. | Reverse dependency invalidation with explicit revalidation; S2. |
| A-21 | **Retain** → AT-21. | Combined revision catches semantic failure and bounds repair; S3. |
| A-22 | **Improve** → AT-13/26. | Stop-at-exhaustion, uncertain quota reserves and no paid rescue; S1 then multi-task S3. |
| A-23 | **Retain** → AT-09. | Reconciliation or uncertainty blocker; S1 existing effects, S6 deployments. |
| A-24 | **Improve** → AT-44/45. | Run requested outcome and release state stay distinct; S6. |
| A-25 | **Retain** → AT-13. | Counter normalization with unknown preserved; S1. |
| A-26 | **Retain** → AT-28 and §13.1 receipt accounting. | Show latency/resource/human costs separately; S3 observations, S4 comparisons. |
| A-27 | **Retain** → AT-30/12. | Learner cannot expand authority or reduce protected quality; S4. |
| A-28 | **Improve** → AT-29. | Heldout, uncertainty, stopping rules and leakage controls; S4. |
| A-29 | **Retain** → AT-32. | Metadata not executable qualification or trial authority; S5. |
| A-30 | **Improve** → AT-32. | Versioned identity and scope-specific suspension; S5 automated discovery, manual drift checks from S0. |
| A-31 | **Retain** → AT-26. | Foreground/repair resources protected, reconcile preemption; S4. |
| A-32 | **Improve** → AT-31. | Restore pointer for new work while active attempts remain pinned; S5. |
| A-33 | **Retain** → AT-33. | No hidden work; full fixed execution continues; S4. |
| A-34 | **Retain** → AT-17. | Exact scoped retrieval remains functional; S2. |
| A-35 | **Improve** → AT-42. | Emergency controlled stop when journaling itself fails; S1. |
| A-36 | **Improve** → AT-43/27. | Concrete erase/backup/derived-model handling with honest provider limits; S2/S4. |
| A-37 | **Improve** → AT-19/03. | Reserve and qualify descendants or visibly restrict/block mode; before any children enabled. |
| A-38 | **Improve** → AT-25/34. | P1 always selectable; new model can displace entire pipeline under evidence; S2/S5. |

<a id="other-coverage"></a>
## 7. Other meaningful source capabilities and historical audits

| Source coverage | Disposition and final destination |
|---|---|
| M §1 research/user needs/competitors; P §§2–3 | **Improve**: separate documentation, reported experience and testable positioning; v2 §§1,7,13. Historical competitor issue reports remain provenance, not fresh claims. No universal native/model winner. |
| M §3 name/tagline/free promise | **Retain**: v2 §1 identity, no paid official tier/mandatory account, explicit external-service costs. Name clearance remains a release check, not an asserted fact. |
| M §5 seven journeys; P §19 todo example | **Combine**: v2 §15.2 complete journeys. **Replace** illustrative vendor-role sequence with contract/eligibility choices; production path explicit in §11.3. |
| M §§6–7 transport/process/platform detail; P §§5,15,17 | **Combine**: v2 §§3–6,14,17. Structured pipes versus PTY is a qualified surface choice, not universal equivalence. |
| M §§8–10 explainable routing, context/messages; P §§7–9,11–12 | **Combine**: v2 §§4,9–10,12. Delivery acknowledgements differ from context understanding; deliberate resets and grounded source references preserved. |
| M §11 dirty Git, worktree/resource claims, apply/reverse; P §§9–10 | **Improve**: v2 §§10–11. Protect original state, freeze before integration, no sibling fast-forward assumption, external resources isolated. |
| M §§12–13 secrets, approvals, egress, config precedence, budgets; P §§6,14,17.4 | **Combine**: v2 §§7–8,15.4. Native auth kept native, project settings cannot widen trust, unknown usage is not zero. |
| M §14 all extension points, manifests, catalogue, SDK examples; P §14.5 | **Retain**: v2 §16. **Defer** sandboxed computational plugins pending need/containment proof. Public process ABI follows exercised internal seams. |
| M §15 real diff, color/light themes, responsive layouts, ten motion moments, keys/plain/screen readers; P §16.4 | **Combine**: v2 §15, AT-35–AT-37/48. This synthesis preserves polish instead of replacing it with a research dashboard. |
| M §16 platform/shell/terminal/distribution/Herdr; P §§16,20.4 | **Improve**: v2 §§14,17. Exact combinations and worker/host independence required; no cross-compile or restore-as-liveness inference. |
| M §17 receipts, performance targets, backpressure, retention; P §18 | **Combine**: v2 §§5,13,17. Performance values remain targets, routine receipts distinct from opt-in learning. |
| M §18 central experiment/fixtures/native-billing/Herdr tests; P §§13,22 | **Combine**: v2 §§13,18, 48 acceptance cases plus G01–G16. Offline versus live evidence separated. |
| M §§19–22 public hygiene/license/CI/ADR/roadmap/risks; P §§20–21,24 | **Improve**: v2 §§18–19 and implementation plan. Actual accepted repository decisions override obsolete “choose license/database” recommendations. |
| P §§11.4–11.7 selection bias, counterfactuals, fast/slow feedback | **Improve**: v2 §§12–13 exact assignment/outcome data, scoped estimators, no unsupported counterfactual gain, delayed outcome correction, full exploration overhead. |
| P §12 new model lifecycle | **Retain** with scope separation: v2 §12.4; no installation from discovery, whole-pipeline comparisons, identity drift tests. |
| P §§23–25 merge instructions/source register | **Replace** append/merge process with this standalone design and explicit trace. Preserve historical evidence, recheck consequential claims selectively, no execution of embedded backlog instructions. |

M's historical audit A01–A32 is distinct from P's A-01–A-38. Its original criticised answer was not supplied separately; this table preserves the corrective outcomes without pretending to independently inspect that earlier answer.

| M historical IDs | Disposition → v2 coverage |
|---|---|
| A01, A02, A03 | **Retain** corrections → §§7,10,13,20: primary evidence, native baseline, no vendor stereotypes. |
| A04, A05 | **Retain** → §7: authorised profiles, quota coupling, native auth, no identity rotation/circumvention. |
| A06, A07, A08 | **Improve** → §§4,7: capability negotiation and independently qualified SDK/PTY surfaces. |
| A09, A10, A11 | **Retain** → §§8,10–11: honest isolation, serialized integration, freeze before composition. |
| A12 | **Improve** → §§3,5–6: single service, durable worker and bounded reconciliation. |
| A13, A14 | **Improve** → §9: actual delivery states and native-context uncertainty. |
| A15, A16, A17, A18 | **Retain** → §§7–8: provenance/limits/egress/native restrictions honestly scoped. |
| A19 | **Retain** → §16: typed process extensions, permissions and conformance. |
| A20, A21 | **Retain** → §15: truthful motion and accessible terminal-owned rendering. |
| A22, A23 | **Retain** → §17: independent Windows lane and measured performance targets. |
| A24, A25, A26 | **Improve** → §§10–11: contract-first work, protected combined checks, authorised production effects. |
| A27 | **Improve** → §§12–13: concrete staged adaptive system beyond nightly weight updates. |
| A28, A29 | **Improve** → §§18–19: complete useful slices and existing public license/governance. |
| A30, A31, A32 | **Retain** → §§7–8,10,17: explicit updates, native child/resources and startup trust. |
| R01, R02, R03 | **Retain** → §7: independent native/billing qualification and credential-provenance semantics. |
| R04, R05, R06 | **Improve** → §§7,14: seven explicit targets, mode-specific permission tests and first-class Herdr. |
| R07, R08 | **Retain** → §§4,6–7,14: real identity and reattach-first single ownership. |

<a id="research"></a>
## 8. Evidence verification and limitations

### 8.1 Current interface and operational research

Primary documentation was read through the relevant pages; no agent was installed, signed in or run. Access date for every source is 5 October 2026. The linked master register supplies URLs. “Checked” below means the documented claim was inspected, not that the implementation or account was qualified.

| Sources | What was checked and how it affects design | Remaining limit |
|---|---|---|
| SRC-01–SRC-03, SRC-36 | Claude programmatic/auth/legal/sandbox distinctions. Bare is not a fidelity-preserving subscription shortcut; non-bare startup requires configuration admission. | Exact installed release, managed configuration and no-extra-usage proof remain live qualification work. Do not universalize an announced future default. |
| SRC-04–SRC-06 | Codex local native app-server/control and auth distinctions; included usage can continue into purchased credits. | Native login/rate metadata does not prove a stop-at-included-limit boundary. New partner-auth possibilities are not permission to extract native secrets. |
| SRC-07–SRC-08 | OpenCode server/SDK and provider auxiliary routes. | Whole effective provider bundle and native-plugin behavior need qualification; a main-model setting is insufficient. |
| SRC-09–SRC-10 | Muse native subscription onboarding versus other keys, native child/background behavior. | Shared checkouts/cooperative cancellation need actual isolation/accounting tests; not all native children imply independent environments. |
| SRC-11–SRC-12 | Kimi prompt permissions/skill/session options and extra-usage continuation. | Account-specific Extra Usage prevention, current interface and complete skill behavior need tests. No copied server-command assumption. |
| SRC-13–SRC-14 | Cursor headless surface and inclusion/on-demand distinction. | CLI/editor/cloud/SDK parity and exact plan protection unproved. No current price/model recommendation imported. |
| SRC-15, SRC-17–SRC-18 | Antigravity headless denial semantics and documented account/provider/overage distinctions. | General plans and CLI provider docs have scope differences; neither page alone certifies the exact CLI route. |
| SRC-19–SRC-20 | ACP session negotiation and MCP security concerns. | Standard transport does not establish billing, native parity or a sandbox. |
| SRC-21–SRC-22 | Herdr installed API schema, detach/restart/session restoration distinction. | Installed-host MYTHHELM registration and safe suppression of duplicate native restore require a live fixture. |
| SRC-16, SRC-23–SRC-24 | SQLite WAL/local writer, documented reset-bug correction, Git worktree semantics, optional FTS5. | Verify the shipped SQLite engine includes the fix (3.51.3 or documented backport), not just the driver module version. Worktrees/FTS never confer ACLs. |

Provider pages are mutable: record actual installed versions and relevant page/date with live results. Documentation plus a user's statement can be useful evidence but cannot be mislabeled independent runtime verification. No precise plan price, quota balance, model ranking or account entitlement was inferred here.

### 8.2 Research claims and applicability

| Source/version and inspection depth | Evaluated setup/baseline/measurement | Decision and limitation |
|---|---|---|
| SRC-25, arXiv:2602.11988v3; methods/results/limitations read | Claude Code/Sonnet 4.5, Codex/GPT-5.2 and GPT-5.1 mini, Qwen Code/Qwen3 Coder 30B; SWE-bench Lite and Python repository-context tasks; no/generated/developer instructions; success and inference cost. | Current revision does not support a blanket success advantage from adding generated instructions or deleting native ones. Developer versus generated context differs. Preserve required instructions; evaluate selection overhead and correctness locally. |
| SRC-26, arXiv:2512.22087v1; setup/results read | Fine-tuned Qwen2.5-Coder-32B compression within an OpenHands setup; SWE-bench Verified, pass@1 versus ReAct/threshold approaches. | A trained compressor is part of the intervention. Reported solve-rate changes do not establish token savings or that an external summary reproduces them. Keep grounded summaries optional and measured. |
| SRC-27, arXiv:2609.32662v1; methods/results read | Nineteen tasks, 52 dependencies, ten model configurations; single/serial and asynchronous private/manager variants; dependency and test-pass measures. | Useful dependency/visibility ablation ideas; small workload and its agent scaffold do not establish native subscription throughput or broad asynchronous superiority. |
| SRC-28, arXiv:2512.08296v3; abstract inspected | Reported 260 configurations, six benchmarks, five architectures and three model families, standardized tools/prompts/compute. | Qualitative task dependence only. No numeric scaling threshold or gain adopted; detailed methods not independently audited here. |
| SRC-29, arXiv:2406.18665v4; abstract inspected | Preference-trained strong/weak model routing against routing baselines; quality/cost tradeoffs. | Supports testing learned selection. Short-form routing evidence does not qualify long-horizon native workflows; no cost-saving percentage reused. |
| SRC-30, arXiv:2507.19457v2; abstract inspected | Reflective prompt optimization across six tasks against GRPO/MIPROv2-style baselines. | Motive for bounded prompt experiments, not permission to alter protected evaluators or deploy controller code. No gain/sample-efficiency number imported. |
| SRC-31, arXiv:1103.4601v2; abstract inspected | Partial-feedback policy evaluation/learning using reward modeling and propensity information. | Motivation only. Actual MYTHHELM estimator choice must establish overlap/assumptions and uncertainty; deterministic logs cannot create missing counterfactuals. |
| SRC-32, arXiv:2502.00674v1; abstract inspected | Same-model versus mixed-model aggregation; reported AlpacaEval 2.0 and reasoning benchmarks. | Homogeneous/strong single baselines deserve testing. These are not native coding throughput experiments. |
| SRC-33; engineering report read for applicable claims | Multi-agent research system versus chat/reporting baselines, resource observations and task-dependency limits. | Observational experience, not a matched coding baseline or causal MYTHHELM speedup. Extra work/coordination belongs in accounting. |
| SRC-34; engineering guidance read | Outcome-based agent evaluations and code/model/human graders. | Adopt evidence separation as a design principle; no universal reliability claim. |
| SRC-35; engineering report read | Repository/tooling/context practices in an agent-assisted software project. | Inform concise maps and grounded artifacts; experience does not establish a controlled productivity multiplier. |

No external experiment was reproduced. No numerical performance result is promised. P's AdaptOrch, Symphony, caching/context-engineering and other references not listed above remain historical leads; this synthesis does not depend on their unverified numerical claims. M's historical issue reports, competitor landscape, sentiment and broad framework comparisons were not completely re-audited. Actual local dependency pins were checked, so newer upstream framework behavior is not attributed to this repository.

<a id="uncertainties"></a>
## 9. Remaining uncertainties with owners and tests

These are unavailable facts or future measurements. The architectural choices themselves are resolved.

| ID | Uncertainty and safe default | Decision owner and concrete next evidence |
|---|---|---|
| U01 | Which native route first satisfies included-only fidelity/lifecycle needs? No predetermined winner; strict real dispatch blocked meanwhile. | Native-integration maintainer: reviewed qualification fixture, exact native build/account/surface, exhaustion/auxiliary/credential precedence evidence. W02; no experiment authorized by this document. |
| U02 | Can each native credential flow coexist with the intended enforced sandbox? Keep honest trusted-host eligibility separate; block demanded unsupported containment. | Security + adapter maintainers: startup/egress/child/credential-boundary adversarial tests per OS. W02/W04/W14. |
| U03 | Which Herdr versions safely restore only MYTHHELM attachments? Embedded TUI works; disable unqualified raw native restore integration. | Host maintainer: installed API schema fixture, detach/restart/takeover and one-owner live test. W05. |
| U04 | How much of native usage/context/model identity is observable? Preserve null/source/confidence; avoid hard-cap or cache guarantees. | Adapter/evaluation owners: counter semantics and alias/version drift fixtures, sanitized live observations. W02/W10/W12. |
| U05 | Can adaptation improve a useful cohort after its own overhead? Fixed P1/approved policy until demonstrated. | Evaluation owner: registered direct-native/P1/portfolio comparison, heldout tasks and canary; inconclusive is valid. W10–W12. |
| U06 | Which native/platform/accessibility combinations can honestly ship? Publish limited matrix, keep full targets visible. | Platform/UX maintainers: Windows tree ownership, macOS/Linux native tests, NVDA/VoiceOver/Orca user sessions. W04/W05/W14. |
| U07 | What retention/disk settings fit real workloads? Bounded storage, stop rather than delete active work; initial targets not measured facts. | Core maintainer: burst, full-disk, purge/backup restore and realistic retention fixtures. W03/W13. |
| U08 | Which destinations support safe deploy reconciliation and rollback? Optional release disabled; unknown non-idempotent effects block. | Release operator: destination-specific idempotency/lookup/health/rollback qualification, explicit grants. W15. |
| U09 | What migration downtime is acceptable for existing users? Explicit drain/quarantine and consistent backup, no zero-downtime claim. | Core maintainer: real schema-v1 fixtures with active/ambiguous workers, stopped adoption, rollback and unsupported-version tests. W03. |
| U10 | Which third-party plugins merit use? Local/explicit install with disclosed host trust; no blanket marketplace safety. | Extension maintainer/user: provenance, actual required capabilities, protocol conformance and independent security evidence if claimed. W09. |
| U11 | When should wider parallelism/search/optimizer complexity be added? One/two writers and exact/lexical retrieval remain defaults. | Scheduler/context/evaluation owners: ablation proving useful gain against simpler policy including integration and human costs. W08/W10/W12; separate spec for larger search. |
| U12 | Has product/name/distribution clearance and final design adoption occurred? No new clearance/adoption claim; originals and repository governance preserved. | Maintainer/release operator: normal product approval, package/name review, signed release evidence. W01/W16. |

<a id="validation"></a>
## 10. Original synthesis validation

One final editor performed separate coverage, contradiction, failure-path and simplification reviews. No independent subagent review or runtime qualification is claimed. The following are design walkthroughs against the finished contracts; software tests remain future acceptance work.

### 10.1 Failure-path walkthroughs and resolved defects

| Case | Walkthrough and result of review | References |
|---|---|---|
| WF01 Simple one-agent fix | P1 admits one route, snapshots source, runs native work, freezes candidate, independently checks exact tree and returns review receipt. Apply is separately bounded. Removed need for a second model/research service in the first milestone. | §§6–7,10–11; AT-05/22/48 |
| WF02 Cross-harness handoff | Old writer stops/finishes; exact task/handoff/evidence enter a new eligible native session. Added original requirement references and explicit continuity loss; no hidden conversation portability. | §9; AT-14/15 |
| WF03 Contract-bounded parallel work | Two ready tasks acquire complete distinct resource sets and stable contracts. Children count toward the envelope. Added validated interface checkpoints distinct from implementation acceptance, external mutable-state isolation and no lease-only resource reclaim. | §10.3; AT-19/20 |
| WF04 Stale upstream input | Contract revision invalidates dependent acceptance before dispatch/integration; running work stops or checkpoints according to effect risk. Retained old artifacts without rewriting historical accepted runs. | §§9.3,11.1; AT-18/23 |
| WF05 Allowance exhausted | Qualified route stops, checkpoint/session retained; wait or re-admit authorised fallback. Added distinction between estimated repair reserve and a provider-enforced cap; no paid summary. | §§7.3,10.3; AT-03/13/26 |
| WF06 Native-session loss | Reconnect cannot become relaunch; a new attempt resumes only compatible native session or reconstructs from artifacts after old ownership is resolved. Added separate reconnect/resume meanings and explicit task/attempt transitions. Interrupted/quarantined attempts can be reconciled to the same live launch; they are not falsely terminal. | §§4.4,6,9.2; AT-06/15 |
| WF07 Herdr reconnect | Pane attaches to same supervisor; projection/attention does not grant input or report acceptance. One input lease survives client churn. | §14; AT-07/10 |
| WF08 Herdr restart | Host can tear down pane process; independent worker remains reconciled through service. Corrected naive “restore implies continuity”: restored aggregate client must not trigger raw native auto-resume. | §§6,14; AT-06/10 |
| WF09 Conflicting integration | Freeze both outputs, compose against current base, detect semantic failure despite clean merge, issue bounded repair from both artifacts. No live rebase or false component acceptance. | §11.1; AT-21/23 |
| WF10 New model candidate | Discovery stores metadata only; install/qualify/calibrate/test/canary independently. Added whole-pipeline P1 replacement trial, rather than fixing new models permanently into old roles. | §12.4; AT-32/34 |
| WF11 Falsely promising policy | More authority/weaker grader is rejected; dev-only win or absent counterfactual support remains unpromotable. Added all-started/censored denominator and registered uncertainty floor. | §§12–13; AT-28–AT-31 |
| WF12 Disabled learning | No learning/backfill/experiments run; required operational evidence persists and fixed policy completes work. Resolved source collection-default conflict explicitly. | §§8.4,12; AT-27/33 |
| WF13 Deployment without authority | Candidate verified/release-ready; run requesting deployment remains blocked until effect authority. Corrected potential “completed” status for an unmet production request. | §11.3; AT-44/45 |
| WF14 DB cannot write | No new effects launch; independent stop channel may act without pretending a new journal entry succeeded; recover ambiguous results later. Resolved persistence-before-stop deadlock. | §§6,17.2; AT-42 |
| WF15 Erase run history | Quiesce, exclude eligible records in validated DB rewrite, clear selected artifacts/indexes/derived data, inventory backups. Resolved immutable-event versus deletion conflict without fake provider erasure. | §5.3; AT-43 |
| WF16 User takeover and stale worker | Automation relinquishes input; fresh user work is reconciled before resuming. Late worker evidence is quarantined; lease timeout never proves termination. | §§6,14; AT-07/20 |

### 10.2 Coverage and contradiction passes

Coverage reviewed M §§1–22/source register, P §§1–25, all 19 M invariants, 12 M gates, 12 P invariants and 38 P acceptance cases. The tables above account for non-ID capabilities and historical corrective audits. Full vision and first delivery are separate; all seven native targets remain named.

Contradiction review checked: one ledger/writer/controller; worker versus host ownership; one Task/Attempt model; routing versus design decisions; resume versus reconnect; queued versus acknowledged context; exact acceptance versus native exit; user consent versus operational state; unknown quota versus unknown entitlement; safety gates versus learned scores; production request versus release preparation; append-only retained history versus erasure. Normative contracts and staged exceptions are stated once in the master and referenced from the plan.

Simplification review removed competing per-repository controllers, a second Attestation store, an independent Goal lifecycle, separate learning/evidence services, vendor-specialist defaults, universal numerical scoring, mandatory embeddings, compulsory second-agent launch, general automatic optimizer and speculative host-owned execution. The retained service is justified by global reservations and detached scheduling; the learner is a bounded module. No capability depends on “add AI later” without a defined observation/action/promotion contract.

### 10.3 Local document checks

These static results were recorded for the original three-artifact handoff, before product-adoption links and governance updates. They establish documentation consistency and repository hygiene only. They do not establish any AT live result, product benchmark, provider entitlement, deployed behavior or maintainer adoption. The later adoption is checked separately against its product draft and changed harness tooling.

| Check performed on 5 October 2026 | Result |
|---|---|
| Deliverable inventory | Exactly the three requested Markdown documents; complete master, traceable decision report and 16-slice implementation plan. |
| Source preservation | SHA-256 matches for both source documents and the invoked prompt in the original checkout and isolated worktree. No source content edited. |
| Required source trace | Explicit complete mappings for 19 M invariants, 12 M gates, 12 P invariants and 38 P acceptance cases: 81 total, with no missing or duplicate mapping IDs. |
| Definition/reference integrity | Unique 10 UR, 25 I, 48 AT and 16 G definitions; 28 decisions, 12 uncertainties, 16 walkthroughs and 16 implementation slices checked. Referenced numeric IDs within defined ranges. |
| Markdown structure | Balanced five fenced blocks, consistent table column counts, unique explicit anchors, and 59 resolving local links/anchor references. |
| Source references | All 36 primary-source definitions present, 35 inline citation uses resolved; external URL syntax checked. Content verification scope is §8, not an automated guarantee of future URL availability. |
| Machine-readable examples | One JSON event and one TOML config parsed with Python standard libraries. Schema version and conservative execution/billing/learning/exploration defaults checked against the normative text. These are proposed contracts, not tests of the current app parser. |
| Lifecycle vocabulary | Literal transition targets resolve within the 15-state Run, 10-state TaskRevision and 12-state Attempt vocabularies. Guards, uncertain ownership, release transitions and bounded recovery additionally reviewed in the walkthrough pass. |
| Whitespace | Named-file `git diff --no-index --check` checks clean after removing trailing Markdown hard-break spaces. The no-index difference exit is expected for new files. |
| Required repository gate | `scripts/harness/gate.sh all` selected and passed the public-hygiene gate for this documentation-only change. No Go/harness implementation changed, so that command did not run application tests or native qualification. |

At the synthesis handoff, the design was internally reviewed and statically checked, with the remaining empirical and authority questions in §9. Product adoption is tracked by the linked adoption map. Application implementation, live qualification and measured adaptive improvement remain subsequent work with the gates stated in the master and plan.
