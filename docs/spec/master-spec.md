# MYTHHELM
## Many agents. One mission.

**Open-source native-agent orchestration, routing and mission control**  
**Master product, architecture and delivery specification · Revision 1.1 · 29 September 2026**

> Keep the agents people trust. Give them a shared mission, safe working boundaries, an honest control plane and a terminal experience worth opening.

**Document status:** researched design specification, not an implemented or benchmarked product. All MYTHHELM commands, interfaces, layouts, policies and performance thresholds below are proposed contracts. Native vendor capabilities are separately attributed to their documentation. No provider integration, operating-system matrix or security boundary has been runtime-certified during this review.

**Basis:** the supplied Muse Spark 1.3 conversation; the requirements for a public GitHub project that is open source, entirely free and extensible; and the subsequent clarifications requiring first-class operation inside Herdr, use of existing subscription allowances rather than separate inference charges, and preservation of native agent harnesses. [U1] [U2]

**Revision 1.1 replaces Revision 1.0 in full.** This is the complete specification, not an addendum. The original 22-section organisation is retained. Native-CLI-first integration, per-harness entitlement qualification and Herdr support are incorporated into the architecture, adapter contracts, billing policy, examples, plugin boundaries and release gates.

> **Controlling decision:** native-agent fidelity takes precedence over adapter convenience. Run the selected, unmodified native executable through its best qualified control or headless interface. Do not replace its agent loop with model API calls. An SDK alternative must independently pass native-fidelity and subscription-entitlement qualification. A transport library talking to an already-running native agent is not a replacement agent harness.

**Default spending policy:** subscription-only means the selected account's included allowance, not merely “signed in”, “using OAuth” or “no API key supplied”. Separately purchased credits, paid overages, metered APIs and paid auxiliary calls are excluded. A route whose inclusion or overage behaviour cannot be established remains blocked under this policy; changing the policy is an explicit user decision.

**Herdr relationship:** Herdr is a first-class terminal host and optional presentation/control integration. MYTHHELM owns its managed mission and worker lifecycle; the native tool owns its agent loop and authentication. No component infers correctness or billing from another component's activity badge.

**Reading route:** Sections 1–2 contain the evidence and critical review. Sections 3–6 establish the product and architecture. Sections 7–17 specify behaviour and interfaces. Sections 18–22 define verification, open-source delivery, staged scope and implementation priorities. The final register contains portable source links.

**Key updated sections:** [integration hierarchy](#61-three-viable-options-and-the-selected-integration-hierarchy), [SDK qualification](#99-sdk-admission-and-native-fidelity-qualification), [Antigravity adapter](#913-antigravity-adapter-native-cli-first), [per-harness billing](#139-per-harness-entitlement-and-overage-requirements), and [Herdr integration](#166-first-class-herdr-integration).

## Contents

- [1. Research summary](#1-research-summary)
- [2. Critical evaluation of the supplied answer](#2-critical-evaluation-of-the-supplied-answer)
- [3. Product identity and open-source promise](#3-product-identity-and-open-source-promise)
- [4. Design principles and invariants](#4-design-principles-and-invariants)
- [5. User journeys and proposed CLI sessions](#5-user-journeys-and-proposed-cli-sessions)
- [6. Architecture and technology choice](#6-architecture-and-technology-choice)
- [7. Execution lifecycle, persistence and recovery](#7-execution-lifecycle-persistence-and-recovery)
- [8. Routing, scheduling and learning](#8-routing-scheduling-and-learning)
- [9. Native harness adapters](#9-native-harness-adapters)
- [10. Collaboration, context and the blackboard](#10-collaboration-context-and-the-blackboard)
- [11. Workspace isolation, Git and integration](#11-workspace-isolation-git-and-integration)
- [12. Security, trust and privacy](#12-security-trust-and-privacy)
- [13. Authentication, subscriptions, quotas and budgets](#13-authentication-subscriptions-quotas-and-budgets)
- [14. First-class plugin and mod architecture](#14-first-class-plugin-and-mod-architecture)
- [15. CLI and TUI experience](#15-cli-and-tui-experience)
- [16. Cross-platform, shell and terminal support](#16-cross-platform-shell-and-terminal-support)
- [17. Observability, retention and performance](#17-observability-retention-and-performance)
- [18. Evaluation and acceptance plan](#18-evaluation-and-acceptance-plan)
- [19. GitHub project, licensing and maintenance](#19-github-project-licensing-and-maintenance)
- [20. Delivery roadmap: ambitious, gated and shippable](#20-delivery-roadmap-ambitious-gated-and-shippable)
- [21. Risks, unresolved questions and decision ownership](#21-risks-unresolved-questions-and-decision-ownership)
- [22. Traceability and implementation handoff](#22-traceability-and-implementation-handoff)
- [Source register](#source-register)

---

## 1. Research summary

### 1.1 What the research does and does not establish

The original response offered a compelling outline, but its “research synthesis” did not provide checkable citations. It mixed product capabilities, individual opinions, historical complaints, speculative performance figures and universal conclusions. In particular, it did not establish that one vendor is consistently best at tests or another consistently best at front-end work. [U1] (lines 57–123 and 191–206)

This review uses official documentation, original project repositories, original engineering publications and identifiable issue reports. Product documentation establishes advertised mechanisms; it does not independently establish comparative quality. An issue report establishes that someone reported a problem, not its prevalence, current reproducibility or applicability to a different version.

There is **no representative Reddit/Hacker News/X/G2/Gartner sentiment survey in this document**. Consequently, the feature synthesis below is a grounded product-requirements analysis, not a statistically ranked “most loved” league table. Future user interviews can change priorities without changing safety invariants.

Evidence labels used throughout:

| Label | Meaning |
|---|---|
| **Verified documentation** | A primary source documents the mechanism as of the research date. Installation-specific support still requires a probe and contract test. |
| **Reported experience** | A traceable first-person issue or report; not independently reproduced here. |
| **Design decision** | A proposed MYTHHELM requirement, not an external fact or existing implementation. |
| **Hypothesis** | A proposition to measure; not a routing rule disguised as evidence. |
| **Release gate** | Evidence required before advertising a capability as supported. |

### 1.2 Native harness preservation: the controlling requirement

**Retain the user's premise as a product requirement:** orchestration must keep the selected coding agent in its own native harness, rather than substitute the same model inside MYTHHELM's homemade tool loop. [U1] [U2]

The empirical claim remains narrower: quality depends on the model, runtime, instructions, tools, context, permissions and task. Harness engineering is important, but the cited material does not establish that every native product outperforms every alternative on every task. Preserve the native bundle and compare against direct-native baselines instead of promising universal superiority. [S01] [S02] [S03]

A **harness** is the agent's execution machinery: tool selection/execution, default agent scaffolding, context handling, persistence, permissions and delegation. A **CLI/TUI** is a user/control surface. A **subshell** interprets commands; a **PTY** supplies terminal semantics. Neither a terminal window nor a shell is, by itself, the agent's harness.

**Design decision:** prefer the actual native executable and a documented structured interface. A native control server or a thin SDK client controlling that executable can satisfy this decision. An SDK that creates a separately configured agent runtime needs additional qualification; a raw model client does not satisfy native-harness preservation. The term “SDK” alone settles neither fidelity nor billing.

Keep three identities separate: **harness**, **model/inference provider**, and **billing entitlement**. Selecting a Claude model in OpenCode remains an OpenCode execution bundle, not Claude Code. Likewise, Kimi-backed inference in another supported harness does not become Kimi Code CLI. Users may deliberately select such combinations; the router must never substitute them under another native harness's name.

### 1.3 Native integration feasibility and support targets

The following is an **integration shortlist**, not a certification matrix. Every route must pass both the fidelity gate in Section 9 and the included-allowance gate in Section 13, plus the existing security/platform gates.

| Target harness | Primary-source mechanism | MYTHHELM default decision |
|---|---|---|
| Claude Code | Native headless execution; the Agent SDK also drives Claude Code machinery. [S04] [S05] | Unmodified installed CLI with native account sign-in. Do not offer SDK-backed subscription access by default; see Sections 9.3 and 13.1. |
| Codex | Native app-server control and `exec` interfaces. [S07] [S08] | Managed native process, preferring app-server when qualified; verify the actual account and inference route. |
| OpenCode | Native server and SDK client surface. [S18] [S67] | Operate OpenCode itself. Qualify each configured provider/entitlement separately; OpenCode is not a proxy for other harness identities. |
| Meta Muse Code | Native headless execution and continuation. [S12] | Run the installed Muse CLI with the subscription-associated native credential, not an arbitrary Meta API key. |
| Kimi Code CLI | Native ACP and print-mode interfaces. [S70] [S71] | Prefer a qualified native ACP process where permission interaction is needed; print mode has a significant permission difference described in Section 9.11. |
| Cursor Agent CLI | Native interactive and headless interfaces. [S21] [S74] | Run the resolved Cursor executable; distinguish local CLI, editor and cloud execution. Subscription-only eligibility remains account/mode-specific. |
| Antigravity CLI | Native `agy` and structured headless interaction. [S55] [S56] | Native CLI first, with account-backed allowance and overages disabled. SDK configurations are separately qualified alternatives, not the default. |

Herdr is a **terminal-host integration**, not an eighth inference provider or replacement harness. Its compatibility is specified separately in Section 16.6.

No row establishes complete interactive/headless/SDK equivalence. Required features include native instructions, skills, tools, MCP, hooks, sandbox, approvals, model selection, persistence, context handling, subagents and any task-relevant editor/browser facilities. Unsupported differences are disclosed before dispatch, not discovered silently after a degraded run.

### 1.4 Seven useful jobs to be done

These are requirements inferred from documented workflows, not unverified claims about universal user preference.

| User need | Source-supported pattern | MYTHHELM requirement |
|---|---|---|
| Finish a coherent engineering change | Native agents expose multi-step execution and inspectable results. [S04] [S07] | Own the task-to-candidate-to-verification loop. A fluent response is not delivery. |
| Understand and review what changed | Aider documents Git integration and repository mapping. [S16] [S17] | Show the actual base revision, changed files, diff, validation evidence and undo boundary. |
| Retain control without micromanaging | Codex provides a structured control surface; Continue and Cline document configurable agent workflows. [S08] [S19] [S20] | Make pending decisions explicit; preserve native controls rather than inventing a universal permission bypass. |
| Continue work after leaving the interface | OpenCode documents a server-based integration surface. [S18] | Separate session execution from the visible TUI. Define recovery independently of terminal lifetime. |
| Use the right tool without forced migration | Cursor documents headless operation; multiple other products expose native integration surfaces. [S04] [S07] [S21] | Integrate existing native agents instead of making users abandon their chosen runtime. |
| Navigate quickly and learn progressively | Lazygit and k9s document panel navigation, filtering and contextual commands. [S22] [S23] | Keyboard-complete interaction, contextual help, search, clear focus and a command palette. |
| Recover from mistakes | Cline documents checkpoints, while Aider makes Git central to its workflow. [S17] [S20] | Provide explicit checkpoints and reviewed reversal, without claiming to reverse external side effects. |

### 1.5 Complaints translated into testable requirements

| Evidence and limitation | Design response | Evidence of success |
|---|---|---|
| A historical Cline report described substantial checkpoint disk growth; it is a report about a particular setup, not proof that current releases always leak storage. [S24] | Account for snapshots, logs and workspaces; bounded retention and an understandable cleanup preview. | Disk-quota and interrupted-cleanup tests; no silent deletion of active work. |
| An OpenCode issue reported unexpected auxiliary-model charges. This is not a general accusation about every current release. [S25] | Include router, planner, summariser, reviewer and title-generation calls in billing policy and telemetry. | Subscription-only mode cannot start an auxiliary metered request. |
| A recent OpenCode issue reports repeated compaction in headless work; it has not been reproduced here. [S26] | Track observable context events and progress independently; distinguish provider behaviour from MYTHHELM's own handoff context. | Bounded retries, loop detection, and an explicit “native context state unknown” display. |
| Claude Code issue reports illustrate that streaming and configuration behaviour may differ across integration surfaces. [S27] [S28] | Versioned fidelity tests; no inferred universal live-messaging or profile-isolation support. | Per-surface contract fixtures plus opt-in live canaries. |
| Windows rendering has generated historical framework-specific issue reports. An old report does not establish a current framework defect. [S29] | Native Windows is an independent release lane, not an inference from a successful Go cross-compile. | Windows process, resize, input, Unicode, rendering and cancellation tests. |

Cost surprises, context loss, collisions and lock-in remain relevant design risks. The product should address them through demonstrable behaviour, not absolute promises such as “no pruning”, “no conflicts” or “hard cost caps” without an enforceable mechanism.

### 1.6 The competitive landscape is already populated

Claude Squad, Superset and Agent Orchestrator already occupy parts of multi-agent terminal execution, workspace management and orchestration. Their repositories are evidence that this category exists; this review does not certify their latest capabilities or benchmark them. [S30] [S31] [S32]

Therefore, “multiple agents in Git worktrees” is not sufficient differentiation. The MYTHHELM positioning hypothesis is a **combination** of native-runtime fidelity, credible native Windows support, honest subscription/budget semantics, durable execution, evidence-backed integration and a moddable, exceptionally clear TUI.

The first user test must determine whether this combination removes enough friction to justify another layer between a developer and their agents. A beautiful dashboard that adds interruptions, context transfers and merge work is not a successful product.

### 1.7 TUI evidence and framework implications

Borrow contextual navigation from Lazygit and command/filter discoverability from k9s; do not reproduce their information density without regard to task count. [S22] [S23]

Bubble Tea's current documented renderer is cell-based. The draft's unreferenced string-rendering objection and numerical Go-versus-Rust performance comparison should not be retained as current evidence. Ink is a real React-based CLI framework, and Textual is a substantial Python TUI framework; neither should be dismissed as inherently incapable of sophisticated interfaces. [S33] [S35] [S36]

**Decision:** choose Go and Bubble Tea provisionally on architecture and distribution grounds, then measure the actual MYTHHELM workload. Do not choose a language based on invented memory, binary-size or hiring statistics.

---

## 2. Critical evaluation of the supplied answer

### 2.1 Overall judgement

The supplied answer identifies the right top-level components and correctly emphasises native agents, a shared control surface, workspace separation and enjoyable interaction. It is useful as a brainstorm. It is not yet a trustworthy implementation specification: several central promises are stronger than the proposed mechanisms, and important product, lifecycle, security, extensibility and verification contracts are absent. [U1]

Severity here means implementation priority: **P0** is a release-blocking correctness, security, permission or billing issue; **P1** materially affects reliability or usefulness; **P2** affects polish, scope or maintainability.

### 2.2 Audit and disposition

| ID | Perspective / severity | Original issue and source locator | Required correction |
|---|---|---|---|
| A01 | Research / P1 | Uncited praise, complaints and rankings; lines 57–123. | Separate documented capabilities, reported experience and hypotheses. Supply primary references. |
| A02 | Evaluation / P1 | Native-harness advantage treated as universal; lines 5, 54. | Evaluate the whole runtime bundle against direct-native baselines. |
| A03 | Routing / P1 | Hard-coded “Claude for UX, Codex for tests”; lines 192–203. | User preference plus task-specific measured outcomes; no permanent vendor stereotypes. |
| A04 | Legal/product / P0 | Subscription “pool” and automatic rotation on 429; lines 139–143, 216, 277. | Authorised profiles, shared quota-bucket awareness, native sign-in and cooldowns; no circumvention. |
| A05 | Authentication / P0 | Unverified `claude --profile` and casual environment rotation; line 216. | Documented configuration surfaces, tested credential isolation, no copied OAuth tokens. |
| A06 | Architecture / P1 | SDK embedding rejected as necessarily losing harness behaviour; line 189. | Compare native subprocess, native control protocol and native SDK sidecar on their actual semantics. |
| A07 | Adapter correctness / P0 | Every adapter assumed to support injected messages, usage and clean termination; lines 209–217. | Capability negotiation with unsupported and unknown states. |
| A08 | Transport / P1 | PTY and NDJSON capture conflated; lines 213–215. | Structured pipes by default; interactive attachment is a separate mode. |
| A09 | Security / P0 | Worktrees and file leases imply prevented overwrites; lines 76, 133, 231. | Explain advisory coordination versus filesystem isolation versus OS sandbox enforcement. |
| A10 | Git correctness / P0 | Disjoint sibling branches treated as fast-forwardable; line 231. | Serialized integration against an explicit base, conflict handling and tests of the combined revision. |
| A11 | Concurrency / P0 | Auto-rebase a working agent or replay a loser onto the winner; lines 158, 231. | Quiesce, freeze artifacts, then integrate in a separate workspace. |
| A12 | Recovery / P0 | Session persistence without process ownership, orphan detection or crash semantics; lines 168, 217. | Durable supervisor/worker protocol, launch reconciliation and explicit interrupted states. |
| A13 | Collaboration / P1 | Messages in a bus assumed to enter an agent's actual context; lines 227–229. | Distinguish queued, transport-delivered and explicitly acknowledged messages. |
| A14 | Context / P1 | Exact context visibility and “never silently prunes”; lines 75, 206. | Report MYTHHELM-controlled context exactly; native compaction visibility is capability-dependent. |
| A15 | Cost / P0 | Fabricated quota readings, precise estimates and “$0.00 sub”; lines 135, 139–149. | Distinguish subscription inclusion, actual metered spending, estimates and unknown exposure. |
| A16 | Budget / P0 | Universal budget caps with opaque native spending; lines 110, 269–271. | Hard-cap support requires a proven enforcement boundary; otherwise explicitly soft. |
| A17 | Privacy / P0 | “No code leaves except to native harnesses”; line 114. | Inventory native endpoints, MCP, plugins, optional routers, telemetry and package/test traffic. |
| A18 | Permissions / P0 | Per-task tool/MCP stripping assumes consistent flags and semantics; lines 113, 215. | Preserve effective native configuration by default; restrict only through tested, visible policy. |
| A19 | Extensibility / P0 | YAML command templates and regex parsers treated as a complete plugin model; line 218. | Versioned manifests, typed protocol, trust boundaries, permission review, pinning and conformance tests. |
| A20 | TUI truthfulness / P1 | Fake typewriter progress, invented completion percentages and premature success; lines 234–251. | Motion reflects observable state; delivery status reflects verified artifacts. |
| A21 | Accessibility / P1 | Font control, NO_COLOR/motion conflation and fictitious narration; lines 234, 252. | Terminal-owned fonts, independent colour/motion settings, tested accessible linear output. |
| A22 | Windows / P0 | Native support inferred from a single binary; lines 112, 256–266. | Separate host, adapter, sandbox, subprocess and shell support matrices. |
| A23 | Performance / P2 | Unsubstantiated ratios, sizes and mandatory continuous 60 fps; lines 122, 252, 266. | Event-driven rendering and measured product-specific thresholds. |
| A24 | Safety / P0 | “Zero-downtime migration” followed by automatic cross-provider/cloud escalation; lines 162–166. | Treat production operations and data destination changes as separately authorised effects. |
| A25 | Verification / P0 | Agent exit and successful tests taken as completion; lines 149, 165–166, 269. | Independent verification of exact integrated artifacts and explicit acceptance criteria. |
| A26 | Quality / P1 | Parallelism starts before dependency contracts; lines 145–148. | Default sequential execution; parallelise only work with compatible snapshots and contracts. |
| A27 | Learning / P1 | Nightly reweighting without bias control or rollback; line 206. | Versioned local learning, holdouts, shadow recommendations and reversible promotion. |
| A28 | Product / P1 | Broad 6-week MVP without evidence gates; lines 268–273. | A small complete delivery loop first, with ambitious features staged behind acceptance gates. |
| A29 | Open source / P1 | No licence, contribution, governance or extension distribution contract. | Public GitHub project, explicit free commitments, licence policy and contributor-friendly tests. |
| A30 | Operations / P1 | Automatic updater and doctor mutations insufficiently specified; lines 236, 264. | Read-only diagnostics by default; explicit, verified updates and previewed repairs. |
| A31 | Resource control / P0 | Native subagents and external resources omitted. | Account for child-agent multiplication, ports, databases, caches, process trees and disk usage. |
| A32 | Repository trust / P0 | Native instructions, hooks and MCP configuration inherited without a trust model. | Show and approve effective executable configuration; reject privilege escalation from repository content. |

### 2.3 Revision 1.0 gaps closed by the follow-up requirements

These findings concern MYTHHELM's previous specification, not statements attributed to Muse's original answer.

| ID | Perspective / severity | Gap | Revision 1.1 correction |
|---|---|---|---|
| R01 | Native fidelity / P0 | “An SDK preserves the runtime” could be mistaken for complete default-agent and feature parity. | Native executable first; two independent qualification gates; explicit configuration-delta manifest. |
| R02 | Billing / P0 | Native sign-in could be mistaken for included-only spending. | Separate included allowance, paid overages, purchased credits and metered inference; fail closed on unknown exposure. |
| R03 | Authentication / P0 | Treating all API keys as metered would misclassify some native subscription credentials. | Classify entitlement by provider, product, credential provenance and destination—not credential syntax. |
| R04 | Host integration / P0 | Herdr was missing from the operating model. | First-class host bridge, terminal/input ownership, restore semantics, truthful state projection and acceptance tests. |
| R05 | Adapter coverage / P1 | OpenCode, Kimi, Cursor and Antigravity lacked explicit admission contracts. | Add per-harness integration, billing and permission requirements with published qualification status. |
| R06 | Execution / P0 | A structured CLI mode could be assumed to preserve interactive permissions. | Test mode-specific permission defaults and soft denials; block or select a different qualified native surface. |
| R07 | Product identity / P1 | Same-model execution could be confused with same-harness execution. | Record harness, inference provider, model, runtime surface and entitlement independently. |
| R08 | Host recovery / P0 | Restoring a terminal could accidentally relaunch a running agent. | Reattach-first recovery and a single process owner; presentation restore is never execution replay. |

### 2.4 What should remain

Retain the native-agent premise, explainable routing, local-first operation, coherent task histories, keyboard-first TUI, Git-centred review, extension points, headless operation, purposeful animation and cross-platform ambition. Replace unsupported guarantees, not the ambition.

The crucial change is to make **a successful engineering outcome** the centre of the product, not the number of concurrent agents or the apparent activity of the dashboard.

---

## 3. Product identity and open-source promise

### 3.1 Name: MYTHHELM

**MYTHHELM** evokes a helm directing a fleet of powerful specialists. It is more distinctive and ambitious than `meta-harness` while retaining a practical, pronounceable command.

- **Project:** MYTHHELM.
- **Executable:** `mythhelm`.
- **Tagline:** **Many agents. One mission.**
- **Descriptor:** The open-source command deck for native coding agents.
- **Vocabulary:** missions, tasks, agents, workspaces, routes and receipts. Use ordinary engineering terms in detailed interfaces; do not turn every command into fantasy terminology.

The name is a creative recommendation, not a claim of trademark clearance, domain availability or registry reservation. Before publication, check the GitHub owner/repository, relevant package registries, search results and trademark conflicts. No specific domain is required to build or use the project.

### 3.2 Product definition

MYTHHELM is a free, local-first orchestration CLI and TUI that runs coding tasks through supported native agent runtimes. It selects among authorised execution profiles, coordinates bounded parallel work, keeps durable evidence, and presents a clear path from request to reviewable, verified change. It must work well inside Herdr as well as standalone. Existing included subscription allowance is the default funding source; preserving a native harness and establishing that allowance are independent requirements.

It is **not** another model-provider abstraction with a replacement tool loop. It is also not an account-sharing service, an unlimited-compute promise, a hosted credential proxy, a mandatory SaaS dashboard or a production-deployment bot by default.

### 3.3 “Totally free” means something precise

The official project must provide its complete core, built-in adapters, safety controls, TUI, headless mode, official themes, plugin SDK/specification and local routing without a licence fee, paid tier, feature gate or required MYTHHELM account.

No paid router, hosted registry, analytics service or cloud coordination service is required. An offline scripted demonstration and the full normal contributor test suite must work without paid agent credentials.

External coding services may require the user's own subscriptions. **The default MYTHHELM workflow must not require separate inference API payments or paid overages.** An optional metered profile remains available only through explicit user configuration and consent; it is never a fallback for an exhausted or unverified subscription. A community local-agent adapter can provide a separately labelled local-only workflow where suitable software and hardware exist; do not mislabel it as subscription usage or imply that hardware and electricity cost nothing.

Donations and sponsorship may support the project. They must not unlock capabilities or influence routing defaults. The official application must not insert affiliate-biased routing or undisclosed paid recommendations.

### 3.4 Target users and boundaries

The initial audience is an individual developer or small trusted engineering team already using native coding agents and wanting better task-level control. The initial security model is **one operating-system user supervising authorised native tools**, not a hostile multi-tenant compute service.

An organisation-wide hosted execution service, shared account pool, remote secrets platform and distributed team blackboard are different products with different isolation and contractual requirements. They are not implicit extensions of the first local release.

### 3.5 Non-goals for the first stable release

Do not build a replacement IDE, a new inference engine, a cloud worker marketplace, automatic account creation, automatic limit circumvention, arbitrary remote code execution as a service, or autonomous production deployment.

Do not chase a claim that more agents are always better. One well-chosen native agent plus verification is the default baseline to beat.

---

## 4. Design principles and invariants

These requirements are normative. **MUST** denotes a release-blocking contract; **SHOULD** permits a documented exception; **MAY** denotes an optional capability.

| ID | Invariant |
|---|---|
| I01 | The selected native harness remains the execution backend. Prefer its unmodified executable through a qualified control/headless interface; an SDK runtime is admitted only after explicit fidelity and entitlement qualification. Disclose material configuration, feature or context differences. |
| I02 | No task starts until identity, workspace, data destination, permission policy, billing posture and required capabilities are resolved. Unknown is not equivalent to allowed. |
| I03 | Repository text, model output and plugin messages MUST NOT authorise permissions, spending or publication. |
| I04 | MYTHHELM MUST NOT silently move a task to a new provider, paid billing route, remote environment or less restrictive sandbox. |
| I05 | There is one active writer per managed working directory. Logical file reservations are coordination aids, not security boundaries. |
| I06 | A terminated UI MUST NOT make execution ownership ambiguous. A requested stop MUST NOT be presented as a confirmed stop before reconciliation. |
| I07 | Agent completion and task correctness are different events. Verification attaches to an exact artifact revision. |
| I08 | The user's original checkout remains untouched until an explicitly requested, validated apply operation. |
| I09 | Estimated, reported, observed and unknown measurements remain distinguishable in storage, APIs and the TUI. |
| I10 | Hard limits are advertised only where their enforcement and possible in-flight exposure are established. |
| I11 | Plugins cannot override core admission, approval, evidence or publication rules through supported extension APIs. Arbitrary trusted-host code is separately disclosed as full host trust. |
| I12 | Recovery reconciles external side effects before retrying. Replaying events MUST NOT replay native tool calls. |
| I13 | The product remains useful without network services owned by MYTHHELM, without custom terminal fonts and without optional paid routing. |
| I14 | Every advertised platform/adapter capability has a versioned test result or is visibly marked experimental/unsupported. |
| I15 | `subscription-only` MUST reject separately metered inference, paid auxiliary work, purchased-credit consumption and paid overages. Missing entitlement/enforcement evidence blocks admission; a subscription login alone is insufficient. |
| I16 | A user's native harness selection MUST NOT be replaced by the same model in another harness, an SDK-created variant or a remote service without explicit authorisation and re-admission. |
| I17 | Herdr may present and submit authorised control intents, but its pane status, terminal text and restored layout MUST NOT certify task completion, grant permissions, prove billing or create duplicate attempts. |
| I18 | A managed agent has one lifecycle owner and at most one interactive input writer. Host integration and native viewers MUST NOT create competing process supervisors or writers. |
| I19 | Neither the core nor a plugin may extract native subscription secrets, replay them against model APIs or disable native authentication methods to manufacture a subscription route. |

**Priority order:** explicit user constraints and security boundaries first; correctness and recoverability second; useful latency and total effort third; compute efficiency fourth; visual delight throughout, without changing the truth of the state presented.

---

## 5. User journeys and proposed CLI sessions

All MYTHHELM commands below describe the intended interface; they are not commands available from a shipped package today. Session output is illustrative, not real quota, timing or benchmark data.

### 5.1 First launch: understand the product before granting access

```text
mythhelm demo
mythhelm doctor
mythhelm init
mythhelm profiles discover
```

`demo` uses deterministic scripted agents and a disposable example repository. It demonstrates routing, a question, concurrent progress, a failed check, repair and a reviewable diff with no network access.

`doctor` reports versions, terminal features, Git support, native agent installation, available sandbox mechanisms and unresolved permissions. It does not log in, install agents, edit PATH or repair files automatically.

`init` creates a minimal project policy after showing the proposed file. Profile discovery reports existing native installations without importing their credentials into a MYTHHELM vault.

### 5.2 Use existing subscriptions without creating a billing surprise

```text
mythhelm profiles list
mythhelm run "Add pagination to the audit log and verify it" --billing subscription-only
```

Illustrative profile view:

```text
PROFILE          NATIVE RUNTIME  AUTH SOURCE        AVAILABILITY         QUOTA
claude-personal  Claude Code     native sign-in     needs billing check  unknown
codex-personal   Codex           native sign-in     needs billing check  unknown
muse-personal    Muse Code       native credential  not yet tested       unknown

No MYTHHELM-held subscription tokens. No API, paid-credit or overage fallback.
Sign-in alone does not establish eligibility. Complete profile qualification first.
```

Illustrative route explanation:

```text
Selected: codex-personal
Reason: your preferred eligible profile; repository policy satisfied.
Billing: included-allowance route qualified; paid overage path disabled.
Quota: remaining allowance unknown; provider must stop when inclusion ends.
Delivery: isolated candidate + configured checks + review. No push or deployment.
```

A second same-vendor profile is usable only if separately authorised and tested. The application does not present two subscriptions as one doubled-throughput entitlement.

### 5.3 Parallel work where independence is real

```text
mythhelm plan --task-file task.md
mythhelm run --task-file task.md --max-writers 2
```

For “implement OAuth and tests”, do not immediately assign implementation and speculative tests to separate agents. First establish the interface, security requirements and test contract. Then admit independent work against a stable baseline, or keep the work sequential.

A plan must explain each parallel branch, dependency, expected shared resource and integration checkpoint. The user sees “waiting for API contract” rather than an agent pretending to make useful progress on unknown behaviour.

### 5.4 Stop, detach and recover without losing control

```text
mythhelm runs list
mythhelm attach run_01
mythhelm stop run_01
mythhelm recover run_01
```

Quitting an active TUI offers **Detach and keep working**, **Stop work and exit**, and **Cancel**. Detach identifies the still-running mission and its billing policy. Closing a terminal unexpectedly does not cancel or duplicate work silently; the supervisor owns the run.

Recovery inspects the worker, native session, workspace and launch record. It may resume, produce a reviewable partial artifact, or mark an interrupted attempt. It must not merely repeat the last prompt because the UI lost connection.

### 5.5 Make it your own without trusting arbitrary code accidentally

```text
mythhelm mods inspect ./aurora-theme
mythhelm mods install ./aurora-theme
mythhelm theme set community/aurora
mythhelm plugins inspect ./local-router
mythhelm plugins test ./local-router
```

Installing a theme does not grant filesystem or process access. An executable plugin requires separate trust/permission review and a compatible execution mode. Nothing found in a cloned repository auto-installs or auto-runs.

### 5.6 Automation and review without the TUI

```text
mythhelm run --task-file task.md --plain --format jsonl --non-interactive
mythhelm review run_01
mythhelm apply run_01 --to-branch feature/audit-pagination
```

JSONL output contains only documented events on standard output; diagnostics go to standard error. A non-interactive run encountering a new approval requirement stops in a reported blocked state rather than waiting forever or approving itself.

The default deliverable is a verified review candidate. Creating a GitHub pull request is a separate, explicitly authorised action with its own credential and data-egress scope.

### 5.7 Run inside Herdr without changing the underlying agent

In an existing Herdr terminal pane, the same proposed MYTHHELM commands work:

```text
mythhelm doctor --host herdr
mythhelm run --task-file task.md --host herdr --billing subscription-only
mythhelm attach run_01 --host herdr
```

`--host auto|standalone|herdr` selects presentation integration, **not** the inference provider, execution host or billing source. `auto` enables the bridge only after validating the actual Herdr context. It never launches Herdr, creates panes or installs plugins silently. Explicit `--host herdr` reports a missing/incompatible bridge rather than pretending integration succeeded; standalone operation remains available.

The first supported experience is one MYTHHELM pane. Optional lane panes show clearly labelled read-only MYTHHELM attempt views. A native interactive view is offered only when that runtime/mode supports it safely. Do not launch a second native agent just to populate a pane.

The status must show the actual selected harness and included-allowance admission. A Herdr badge can report attention, while the MYTHHELM receipt remains the authority for checks, review readiness and external effects. Closing the pane detaches the default managed workflow; explicit stop remains a separate action. Section 16.6 defines recovery and the exceptions for host-owned experimental execution.

---

## 6. Architecture and technology choice

### 6.1 Three viable options and the selected integration hierarchy

A raw model API with a MYTHHELM-owned replacement tool loop is excluded from the native-agent workflow.

| Option | Strengths | Weaknesses | Decision |
|---|---|---|---|
| **A. Go supervisor operating unmodified native executables** | Retains native product identity; supports structured headless/control protocols; independent TUI and Herdr presentation; direct worker ownership. | Version-specific capability, authentication and permission qualification remains necessary. | **Default architecture.** Use native control interfaces such as app-server/ACP where they drive the selected native executable. |
| **B. Native interactive CLI in a managed PTY** | Keeps the interactive surface when a task needs capabilities absent from headless mode; supports direct user interaction. | Screen text is not a reliable machine control protocol; approval automation, result correlation and recovery are less observable. | Qualified alternate mode for specific requirements. Never auto-approve prompts by matching screen text. |
| **C. Native agent SDK runtime or sidecar** | May expose richer typed controls while retaining genuine native agent machinery. | May alter defaults/features, require another runtime or use a different entitlement. SDK authenticity is not parity evidence. | Non-default alternative requiring Sections 9.9 and 18.8 qualification. A thin protocol client for Option A remains Option A. |

**Selection rule:** among routes satisfying the task's required native capabilities, permissions and included-allowance policy, prefer the installed native executable's strongest documented control interface. Prefer structured pipes over a PTY when capabilities are adequate. If headless mode drops a required capability, select a qualified native interactive path or block; do not silently drop the capability. SDK convenience never overrides this order without a recorded, user-visible exception.

A shell is not required for normal native launches. Use direct executable/argument invocation; a PTY is required only for terminal semantics. Do not add shell interpretation to preserve a harness that already runs as a process.

### 6.2 Recommended system

```text
   Standalone terminal                 Herdr terminal host
           |                        panes / focus / notifications
           |                                  |
           +------ MYTHHELM CLI / TUI ---------+
                         |             host bridge (optional)
                         |                    |
                 authenticated local control IPC
                         |
 +-----------------------------------------------------------------+
 | Per-user, per-execution-host MYTHHELM supervisor                |
 |                                                                 |
 | Admission: native fidelity + entitlement + security             |
 |        -> router -> scheduler -> durable attempt workers        |
 |                          |                                      |
 |         journal / approvals / reservations / artifacts          |
 |                          |                                      |
 |         frozen candidates -> integration -> verification        |
 +-------------------------+---------------------------------------+
                           |
             worker-owned native agent processes
          structured protocol OR explicitly selected PTY
                           |
    Claude Code | Codex | OpenCode | Muse | Kimi | Cursor | agy
                           |
          native tools, context, permissions and delegation
                           |
              managed workspaces + declared sandbox
                           |
                review candidate / explicit apply
```

**Responsibility split:** MYTHHELM owns task-level planning, routing, scheduling, workspace boundaries, handoffs and outcome verification. Each native harness owns how its agent performs the admitted task. Herdr hosts the user-facing terminal and optional state projections; it does not become MYTHHELM's model router or credential broker.

Give a native agent a coherent task and relevant constraints, then let its native loop operate. Do not reduce it to an executor for a newly invented MYTHHELM read/edit/tool micro-loop. Do not replace native context compaction, default system scaffolding or tool selection to simplify event normalisation.

The UI and Herdr bridge render projections of durable state. Neither is the parent of the default managed native processes or the authority for execution admission.

### 6.3 Component ownership

| Component | Owns | Must not own |
|---|---|---|
| Admission policy | Hard constraints, consent, trust and capability decisions. | Model-written exceptions to those decisions. |
| Router | Ranked execution proposals and explanations. | Credentials, unrestricted process creation or final authorisation. |
| Scheduler | Task DAG, slot reservations, retries and dependency readiness. | Unreviewed Git mutation in active agent workspaces. |
| Attempt worker | Native process/transport, bounded output spool and lifecycle acknowledgement. | Global policy changes. |
| Adapter | Versioned native mapping, capability reports and protocol interpretation. | Invented support for absent native guarantees. |
| Workspace manager | Snapshot preparation, paths, isolation metadata and cleanup eligibility. | Automatic destructive edits to the source checkout. |
| Integration train | Frozen candidate combination and verification pipeline. | Treating independent agent success as integrated correctness. |
| Context store | Provenance-linked artifacts, decisions and messages. | Treating untrusted text as policy. |
| Budget ledger | Reservations, measurements, uncertainty and admission decisions. | Claiming a provider spend cap it cannot enforce. |
| Plugin broker | Validated extension contracts and granted broker operations. | Pretending an unsandboxed same-user process is contained. |
| TUI/CLI | Interaction, display, intent submission and receipts. | Hidden execution side effects from rendering. |

### 6.4 Language and TUI framework

**Choose Go for the core and Bubble Tea for the initial TUI**, with the exact supported toolchain and dependencies pinned when implementation begins. Do not inherit the draft's stale Go version as a requirement.

| Candidate | Why it is viable | Product-specific trade-off |
|---|---|---|
| Go + Bubble Tea / Lip Gloss | Event/message-oriented UI fits a supervisor, and the current project documents a cell-based renderer. [S33] | Recommended; validate Unicode, accessibility and Windows under real load. |
| Rust + Ratatui | Strong terminal layout/control toolkit. [S34] | A credible alternative if a measured Go prototype misses product targets; not rejected on assumed developer difficulty. |
| TypeScript + Ink | React component model and real interactive CLI ecosystem. [S35] | Attractive contributor familiarity; evaluate runtime/distribution overhead and long-lived worker separation rather than declaring it unsuitable. |
| Python + Textual | Rich application model and styling. [S36] | Good prototyping option; a different packaging/runtime trade-off from the desired one-binary core. |
| TypeScript/Zig or a custom terminal engine | Could offer specialised rendering. | Not justified before a benchmark demonstrates an unmet requirement; maintaining a renderer is not the product. |

Use a pure-Go SQLite implementation if it passes correctness, size and performance gates. SQLite is local state, not a network-shared coordination database; WAL has relevant local-filesystem constraints. [S37]

Go's in-process plugin mechanism has platform/build compatibility constraints. It is not the public plugin ABI for a portable community ecosystem. Use a versioned process protocol instead. [S38]

### 6.5 Local transport and ownership

Use a private Unix-domain socket on supported Unix systems and a named pipe with user-restricted ACLs on Windows. Store control endpoints in protected per-user runtime locations. Use peer identity checks where available and session-scoped nonces for worker attachment. Do not expose a TCP listener by default.

One supervisor per user schedules all local repositories so that quota, disk and process limits are not accidentally multiplied by opening multiple terminals. This coordination is per execution host; it does not reserve provider allowance globally against another machine, independently launched native sessions or other users of a shared organisation. Account-wide inclusion and overage enforcement remain provider/native responsibilities, with external consumption treated as observable uncertainty. There is no cross-user trust implication: same-user native code is already inside the host trust domain unless separately sandboxed.

Remote supervision may later be implemented as a separately authenticated protocol. It is not achieved by binding the local endpoint to all interfaces.

### 6.6 Host bridge and runtime adapter are separate extension seams

A **runtime adapter** operates one qualified native harness surface. A **host bridge** presents MYTHHELM in a terminal environment such as Herdr. Their compatibility, trust grants and failure handling are independent. Swapping host bridges cannot change the admitted runtime, credentials, model, permissions or billing route.

In the default Herdr topology, the MYTHHELM worker owns the native process and any PTY; Herdr owns only the client/view terminal. An experimental Herdr-owned native execution backend is a separate lifecycle profile requiring new ownership and restore tests. Do not mix those ownership models inside one attempt.

Persist the execution-host identity separately from display-host identity. A Herdr UI connected to another machine does not silently move local work there or carry local subscription credentials to it.

---

## 7. Execution lifecycle, persistence and recovery

### 7.1 Durable domain objects

The smallest useful domain model is a mission/run containing a task DAG; each task has one or more attempts; each attempt belongs to one immutable execution bundle and one workspace.

| Object | Required information |
|---|---|
| `Run` | Identifier, user goal, requested deliverable, source repository identity, admitted snapshot, policy revision, billing policy, overall state. |
| `Task` | Goal, acceptance criteria, dependencies, inputs, expected outputs, risk class, resource claims and status. |
| `Attempt` | Task, attempt number, immutable execution bundle, selected harness/inference provider/model, runtime surface, fidelity revision, entitlement evidence, worker, execution-host identity, workspace, launch token and lifecycle state. |
| `Decision` | Inputs used, rejected candidates and reasons, selected candidate, uncertainty, policy/router versions and user overrides. |
| `Artifact` | Content hash, size, producer attempt, base revision, provenance, retention class and access policy. |
| `Message` | Sender, intended recipient, type, artifact references, causal parent, delivery state and expiry. |
| `Approval` | Exact proposed effect, approving user, constraints, expiry, one-use token and consumed/revoked state. |
| `Reservation` | Budget, quota or exclusive-resource bucket, quantity where known, owner, generation/fence and lifecycle. Included allowance and purchased-credit balances are distinct buckets. |
| `Verification` | Candidate revision, verifier identity/version, frozen check definition, execution environment, result and evidence artifacts. |

Identities and state transitions are owned by the supervisor. A native agent can submit a task suggestion or result; it cannot fabricate a trusted verification row or approve its own external effects.

### 7.2 State machines

A run moves through these meaningful states:

```text
created -> admission -> planning -> executing -> integrating -> verifying
                |          |           |              |            |
                +----------+-----------+--------------+------------+
                                      |
                                blocked / failed

verifying -> ready_for_review -> applying -> completed
     |               |             |
     +-- repair -----+             +-> blocked / failed

Any active state -> stopping -> cancelled
Any active state -> interrupted -> recovering -> a reconciled valid state
```

`ready_for_review` is a successful requested deliverable when the user asked for a candidate and evidence. `completed` means all explicitly requested effects, such as applying to a branch, have been verified. Neither state implies that a pull request was created or a deployment happened unless that was part of the request.

An attempt uses finer states: `reserved`, `launch_intent_recorded`, `launching`, `running`, `waiting_native`, `waiting_approval`, `stop_requested`, `stopped`, `succeeded_native`, `failed_native`, `interrupted`, and `quarantined`.

A native exit code of zero records a native result. The supervisor evaluates the task's acceptance criteria separately.

### 7.3 Supervisor and worker responsibilities

Each attempt has a dedicated worker process, implemented by a hidden subcommand of the same main executable where practical. The worker owns the native child, its structured transport, output spool and native process identifiers. Its lifetime is independent of the TUI.

The supervisor owns admission, global state and integration. During a supervisor restart, a surviving worker can reconnect with its launch identity and replay unacknowledged events. It must not accept a connection based only on a matching PID: process start time, nonce and run identity must also match.

A worker heartbeat says the worker is reachable, not that the model is thinking or making progress. An idle native stream may be a legitimate long tool run. Progress watchdogs consider tool status and elapsed phase time; they do not kill every quiet process after an arbitrary universal timeout.

If the worker is lost, the supervisor marks the attempt interrupted, quarantines its workspace and reconciles the native process/session. It never launches a replacement writer into the same workspace until ownership is resolved.

### 7.4 Process lifecycle across operating systems

Use argument arrays and direct process creation for ordinary native launches. Go's execution API does not implicitly perform shell expansion; preserve that property instead of interpolating the user's request into a command string. [S39]

For Unix, establish process-group/session ownership and a staged graceful-interrupt, terminate, then force-stop policy. For Windows, use an appropriately configured Job Object and tested console/process behaviour. Job Objects are a process-management mechanism, not a substitute for a security sandbox. [S40]

The implementation must handle native launchers that spawn helpers, scripts, containers or detached descendants. Where ownership cannot be proven, say so. A “stop requested” indicator must remain until process death or an explicit unresolved-orphan state is established.

Releasing an exclusive database or device reservation while an old process may still use it is unsafe. Expiration alone is not proof that the former owner stopped.

### 7.5 Persistence and external side effects

The supervisor is the sole logical SQLite writer. Persist commands and state transitions transactionally; publish corresponding events through a durable outbox. Readers consume projections or snapshots rather than mutating authoritative rows.

Process creation, Git operations, HTTP calls and SQLite transactions are not one atomic transaction. Therefore:

1. Persist a uniquely identified intent and its expected preconditions.
2. Execute the effect through an owner that records the same identity.
3. Persist the observed result and evidence.
4. After a crash, reconcile the effect before retrying it.

The contract is **at-least-once delivery with idempotent handling where possible**, not universal exactly-once execution. A failed connection after a GitHub request, for example, requires checking whether the intended pull request already exists before creating another.

Database migrations use versioned schemas, a verified backup and a recovery path. Downgrading across an incompatible schema is refused with a clear restoration instruction. Active runs pin their configuration and plugin artifacts across updates.

### 7.6 Event envelope

The following JSON is an illustrative event instance, not a vendor event schema:

```json
{
  "schema_version": 1,
  "event_id": "evt_demo_0042",
  "run_id": "run_demo_0001",
  "task_id": "task_demo_0002",
  "attempt_id": "attempt_demo_0003",
  "producer_id": "worker_demo_0003",
  "producer_sequence": 42,
  "run_sequence": 108,
  "generation": 2,
  "caused_by": "evt_demo_0039",
  "observed_at": "2026-09-29T12:00:00Z",
  "type": "message.delivery_changed",
  "payload": {
    "message_id": "msg_demo_0011",
    "delivery": "queued_for_next_turn",
    "reason": "live_steering_not_supported"
  }
}
```

Required rules:

- Deduplicate by event identity; reject stale worker generations for control effects.
- Preserve per-producer ordering. `run_sequence` is journal ingestion order, not proof of real-world causal order across workers.
- Wall-clock timestamps are for display and audit; use monotonic clocks for local durations and backoff.
- Critical lifecycle, approval and verification events are durable. Streaming progress may be coalesced, with loss/coalescing counters visible to diagnostics.
- Unknown optional event types may be retained in a bounded vendor namespace. Unknown mandatory protocol features or unparseable approval events stop the affected attempt safely.
- Replay reconstructs state and the UI. It never re-executes a tool call.

### 7.7 Recovery outcomes

`recover` produces one of four explicit outcomes: reattached to a live owned worker; resumed a compatible native session after checking workspace and permissions; preserved a partial candidate for review; or stopped/quarantined an unrecoverable attempt.

A native conversation resume is not a filesystem rollback or a guarantee that background commands were undone. Native session identity, working directory and admitted snapshot must be compatible before continuation. A user can always inspect the preserved evidence before authorising a new attempt.

### 7.8 Terminal hosts, native viewers and restoration

A TUI/Herdr client disconnect is not an instruction to relaunch, stop or change a managed agent. Persist attachment metadata separately from execution intent. Restore must first reconcile the run and worker, then reattach the view. A saved command must not replay the original `run` request and create another writer.

Every host attachment is scoped to an execution host, Herdr server/session when applicable, pane/terminal identity and MYTHHELM run/attempt identity. Revalidate that binding after a pane move, replacement, reconnect or server restart. Never use a remembered short pane name as globally unique identity.

For native PTY views, the worker retains lifecycle ownership and grants one input lease; additional viewers are read-only. MYTHHELM must not concurrently inject prompts while a user owns native input. Reclaim control only at an acknowledged task boundary and re-check permissions/configuration if the user changed them.

A headless process cannot be assumed to turn into an interactive process on demand. If the native surface cannot attach a TUI to that same live session, show the structured inspector. A deliberate native-mode transition requires quiescing the old process, recording the candidate/session, qualifying supported resume and re-admitting the new mode before it may write. Never spawn another agent into the same workspace merely for visual access.

---

## 8. Routing, scheduling and learning

### 8.1 Separate decisions that the draft conflated

**Planning** determines work and dependencies. **Routing** proposes which eligible execution bundle should do a task. **Scheduling** decides when admitted work can run. **Admission** decides whether it is allowed at all. These are separate components so that an attractive router score cannot override policy.

The first release uses local deterministic routing. A small model is optional, not a dependency that adds cost and another data destination before delivering value.

### 8.2 Task descriptors

Describe a task along independent dimensions rather than one label such as “complex”:

| Dimension | Examples |
|---|---|
| Intent | Investigate, implement, refactor, test, document, review, repair. |
| Risk | Read-only; bounded source edit; dependency change; security-sensitive; data migration; external effect. |
| Scope | Known files, subsystem, cross-cutting change, unknown investigation. |
| Context | Required artifacts, repository languages, dependency snapshot, estimated MYTHHELM context size, native context visibility. |
| Verification | Unit tests, build, browser checks, contract tests, manual acceptance, security review. |
| Constraints | Data destinations, required native tools, OS, sandbox, available identities, billing route. |
| Dependencies | Upstream artifacts, interfaces, shared resources and integration order. |

Prompt text alone is insufficient for a reliable file-count, duration or token prediction. Cheap local inspection may refine descriptors; its work and access remain visible.

### 8.3 Admission before scoring

For each candidate execution bundle, evaluate:

1. User-authorised profile, exact native harness/surface, fidelity qualification and applicable included-allowance entitlement.
2. Allowed provider, region/environment and data-egress destinations.
3. Native model and required capability availability.
4. Workspace and sandbox compatibility.
5. Billing route, paid-credit/overage exposure and enforcement requirements; strict subscription-only rejects unknown exposure.
6. Quota/cooldown state, per-profile concurrency and global resource limits.
7. Plugin trust and version compatibility.

Ineligible candidates are not “low-scoring”; they are excluded with a reason. Unknown quota does not automatically mean zero availability: it can be admitted only when the provider/native route is qualified to stop rather than spend when included usage ends. It prevents precise capacity claims and may reduce concurrency. Unknown identity, entitlement/overage behaviour or a missing mandatory sandbox capability blocks admission.

### 8.4 Ranking without invented expertise

At cold start, prefer the user's chosen eligible native profile and preserve an existing suitable session. Use explicit rules for objective capability differences, such as a required platform or control surface. Do not silently initialise vendor-specific quality rankings from anecdotes.

After sufficient local evidence, rank on a constrained objective:

```text
minimise:
  expected completion latency
  + weighted marginal monetary cost
  + weighted subscription-capacity consumption
  + expected repair and human-review effort
  + handoff/context-transfer overhead
  + uncertainty penalty

subject to:
  required quality and all admission constraints
```

Weights are user-visible presets or explicit policy values. Quantities must be normalised to documented scales before combining them. An unknown input is a range or missing feature, not a fabricated point estimate.

“Quality first”, “fastest” and “economical” alter this ranking, not safety constraints. A quality-first request may deliberately use an independent reviewer, but still cannot spend through an unapproved billing route.

The proposed selector is `--optimise balanced|quality|latency|capacity`, with `balanced` as the default. `capacity` favours conserving scarce subscription allowance and avoiding unnecessary calls; monetary policy remains independently controlled by `--billing` and `--spend-limit`. `--profile <name>` pins an eligible native profile rather than overriding admission. The effective weights and uncertainty handling are available through `config explain` and the route receipt.

```text
mythhelm run --task-file task.md --optimise quality --billing subscription-only
mythhelm run --task-file task.md --optimise latency --max-writers 2
mythhelm run --task-file task.md --profile codex-personal --optimise capacity
```

Choosing latency does not prove that a task is parallelisable or force two writers to start. Choosing quality does not silently create a paid reviewer.

### 8.5 Routing explanations

Every decision records the chosen profile, rejected alternatives, applicable hard constraints, evidence source, uncertainty and policy version.

Good: “Kept the current Claude session: it has the relevant context; switching is unlikely to justify another handoff. Other profile is cooling down.”

Bad: “Claude wins frontend, $0.41, 2.1 minutes” without measured support.

Predicted success probabilities and confidence percentages appear only when calibration has been measured. Otherwise use “limited evidence”, “user preference” or a concrete range with its sample count.

### 8.6 Optional smart router

A router plugin may use a local classifier, embeddings, a small model or a service such as Jev. Jev's documentation describes structured decision-oriented capabilities; that does not independently establish its accuracy or total cost for this workload. RouteLLM provides a relevant routing research/code reference, not a pre-trained solution for native-agent engineering outcomes. [S42] [S43]

A remote router requires explicit endpoint approval, data minimisation and a billing reservation. By default it receives task metadata and sanitised summaries, not repository contents. It must return a schema-validated proposal. Malformed or slow responses fall back to local policy without silently trying a more expensive model.

Router, planner, summariser and reviewer calls are first-class attempts or metered activities. They are never invisible “small overhead”.

### 8.7 Decomposition and bounded parallelism

Default to one writer unless independent work is established. The initial parallel milestone allows at most two top-level writers; read-only reviewers may have a separate explicit limit.

A parallel task needs an input revision, output contract, likely touched areas, integration dependencies and shared-resource declarations. A task with an unknown contract may first run a bounded investigation. Large tasks must not recursively expand into an unbounded forest of subagents.

Native agents may themselves spawn child agents. The effective concurrency includes those children where observable. If child concurrency cannot be measured or constrained, disclose that the top-level lane limit is not a total model-call limit. A policy requiring a hard total limit must reject that execution mode unless its descendants can be bounded.

A scheduler may run independent tasks from different missions, but global profile, account, disk and external-resource limits still apply. Two terminals cannot each reserve the same supposedly unused capacity.

### 8.8 Failure classification and escalation

| Failure class | Default action |
|---|---|
| Authentication or entitlement invalid | Block the profile and direct the user to the native sign-in flow. No token copying or automatic identity substitution. |
| Provider limit / cooldown | Honour authoritative retry timing when available; otherwise bounded backoff. Queue or select an already authorised alternative permitted by policy. |
| Transient transport failure | Reconcile whether work started; retry only when safe. |
| Unsupported protocol/capability | Stop that mode, retain evidence, and offer a tested lesser-capability mode only when it still satisfies the task. |
| Failed verification | Produce failure evidence; allow a bounded repair attempt with the same acceptance criteria. |
| Policy or security denial | Remain blocked until an authorised user changes the applicable policy or proposal. |
| No useful progress / loop | Pause and surface evidence; a different agent is a deliberate new attempt, not an infinite automatic cascade. |

Default repair budget: at most two autonomous repair attempts per task, configurable downward or upward with explicit resource policy. Transient connection retries have their own small bounded budget and do not reset repair or monetary budgets.

Cross-provider handoff carries an explicit context manifest and obeys data-egress policy. It does not automatically upload the whole blackboard or complete raw transcript.

### 8.9 Learning from outcomes

Collect local outcomes only when enabled: exact execution bundle, task descriptors, verification result, elapsed work, observed billing, retry count, integration difficulty and user review feedback.

Use versioned models/policies with local holdouts. A policy proposal can run in shadow mode without launching extra paid attempts: it suggests a route, while the existing policy executes. Shadow agreement is not evidence of counterfactual quality because the unchosen agents did not do the work.

Comparative exploration requires explicit consent, a bounded budget and low-risk evaluation tasks. Do not learn from hidden paid retries, optimise purely for test-count inflation, or promote a new router nightly without evidence and a rollback path. Separate cohorts after significant runtime/model/configuration changes.

---

## 9. Native harness adapters

### 9.1 Contract, not lowest-common-denominator behaviour

An adapter translates supervisory intents into supported native operations and native events into typed observations. It does not replace the native tool loop, secretly rewrite repository instructions or simulate a capability it lacks.

| Operation | Required or optional | Contract |
|---|---|---|
| `probe` | Required | Resolve executable identity, version, supported platforms, installation health and configuration roots without launching a paid task. |
| `capabilities` | Required | Return a versioned capability record with supported, unsupported or unknown values and evidence. |
| `prepare` | Required | Resolve effective configuration, workspace, invocation, trust boundaries and billing route; return a launch proposal. |
| `start` | Required | Launch once under a unique attempt token and return native/worker identifiers. |
| `events` | Required | Emit typed lifecycle observations; streaming text may be optional. |
| `interrupt` | Required best effort | Request stop and report what was actually acknowledged; lifecycle owner confirms termination separately. |
| `resume` | Optional | Resume only with compatible native session, workspace and configuration. |
| `steer` | Optional | Deliver input through a supported control path; report actual delivery semantics. |
| `approval_bridge` | Optional | Bind a native approval request to a trusted user decision without broadening its scope. |
| `usage` / `quota` | Optional | Return units, source, timestamps, scope and uncertainty. Absence is not zero. |
| `export` | Optional | Export permitted native history with explicit sensitivity and retention handling. |

### 9.2 Capability record

A capability is scoped to the exact adapter version, native version, operating system, launch mode and authentication mode. The same installed binary may support a feature interactively but not headlessly.

```json
{
  "schema_version": 1,
  "adapter_id": "builtin/example-native",
  "adapter_version": "0.1.0",
  "runtime_version": "probe-required",
  "mode": "headless",
  "execution_surface": "native-cli-structured",
  "harness_id": "example-native",
  "fidelity_qualification": "unknown",
  "billing": {
    "entitlement": "unknown",
    "included_only_supported": "unknown",
    "paid_overage_prevention": "unknown",
    "evidence_id": null
  },
  "host_integration": {
    "herdr_embedded": "unknown",
    "herdr_state_bridge": "unknown",
    "native_interactive_attach": "unsupported"
  },
  "capabilities": {
    "structured_events": "supported",
    "resume": "unknown",
    "live_steer": "unsupported",
    "approval_bridge": "unsupported",
    "usage_tokens": "unknown",
    "quota_remaining": "unknown",
    "hard_monetary_limit": "unsupported",
    "native_subagents": "unknown"
  },
  "sandbox": {
    "status": "unknown",
    "scope": "not-established"
  },
  "qualification": "illustrative fixture, not a certified vendor adapter"
}
```

Production records replace illustrative values with probed identities and conformance evidence. Security-critical unknowns must not be converted into optimistic defaults.

### 9.3 Claude Code adapter

**Default:** the unmodified installed Claude Code CLI, using a tested native headless/control surface and the user's native subscription login. Derive flags from the versioned compatibility implementation; do not interpolate prompts into a shell command. [S04] [S06]

Anthropic documents the Agent SDK as running Claude Code's agent machinery, but its SDK documentation restricts offering Claude.ai login/rate limits in third-party products without approval. Therefore SDK runtime support is **not the default subscription adapter**. A technical ability to load a credential does not establish permission to offer that path. [S05]

Anthropic's authentication reference says a configured `ANTHROPIC_API_KEY` is used in non-interactive mode, and documents other higher-precedence credential sources. Browser-authenticated Console access is also distinct from a Claude subscription. Check the effective route, not simply whether `/login` succeeded. Configuration roots have documented account-separation behaviour and exceptions; qualify the installed version. [S61]

**Implementation requirements:** inventory provider selection, relevant environment/configuration overrides, helper commands and account type through supported surfaces. Block a mismatch or offer an explicitly previewed, per-child environment selection; never modify the user's global native setup silently. Check the separate overage gate in Section 13.9. Do not use a stripped-down/bare mode merely for performance without a new fidelity and billing review.

Preserve approved native instructions, skills, plugins, MCP and hooks. If headless approvals cannot be bridged, use a preapproved restrictive envelope, a qualified interactive mode or a blocked result—not a blanket permission bypass. Do not invent flags or assume every native mode has the same defaults.

### 9.4 Codex adapter

**Default:** launch the installed native `codex` process through its app-server interface when qualified; `codex exec --json` remains a smaller-capability alternative. This is native executable integration, even when a typed client library carries the protocol. The app-server exposes account and rate-limit operations as well as session control. [S07] [S08]

ChatGPT sign-in and API-key execution are distinct routes. The actual configured provider/account must match the admitted entitlement, not merely the name of the executable. [S09] Check the additional credit/enterprise distinctions in Section 13.9.

**Implementation requirements:** retain supported native configuration, model, reasoning/speed settings, permissions and session identity. Use runtime-owned sign-in; do not import or replay ChatGPT tokens through an external model client. Reject unexpected account/provider updates and re-admit before further turns. A generic OpenAI model or hosted-agent client is not a transparent replacement for this local Codex profile.

`CODEX_HOME`, native credential storage and concurrent profiles require installation-specific tests. Native Windows and its selected sandbox are independent qualification targets. [S09] [S10]

### 9.5 Meta Muse Code adapter

**Default:** the installed `muse` executable, using its documented headless/session interface. Bind every continuation to an explicit native session and managed workspace. [S12]

Meta documents subscription entitlement as applying to the special Muse Code credential associated during native onboarding; additional keys are pay-as-you-go. A subscription credential may therefore be called an **API key** without being an ordinary metered API route. [S13] The authentication documentation also describes environment/stored-key precedence over browser sign-in. [S14]

**Implementation requirements:** classify credential provenance and product entitlement without copying secrets into MYTHHELM. An arbitrary `META_API_KEY` is not accepted as subscription evidence. Resolve native-onboarding and effective-precedence evidence together; if the distinction cannot be established, block strict subscription-only admission. Never forward the native subscription credential to a general Meta inference client.

Treat native children as part of the process/resource model; do not assume child workspace isolation or interactive permission parity. Preserve configuration and record observability limits. A model family name does not identify a harness, a subscription or the runtime's actual default model.

### 9.6 Configuration and native fidelity

For each attempt, retain a redacted configuration manifest: harness/runtime surface, native instruction and default-scaffolding provenance, skills/plugins, enabled tool/MCP references, hooks, model/inference provider, reasoning settings, permission mode, sandbox scope, native settings root, authentication route/entitlement evidence and all intentional MYTHHELM overrides. Record baseline versus effective configuration; “native defaults” is not a substitute for inspecting relevant differences.

An unavailable browser/editor integration or a changed default system scaffold is a capability difference, not a cosmetic detail. Preserve native context handling and persistent conversations where qualified; MYTHHELM's handoff package supplements the task, not the native system prompt. Never silently replace native instructions with a MYTHHELM persona or create a fresh process for every individual tool step.

Native project files may be read from a managed snapshot, but their executable effects are not implicitly trusted. Show changed hooks, MCP endpoints, executable settings and instruction digests before granting corresponding access. Avoid silently editing `AGENTS.md`, `CLAUDE.md` or equivalent project files to inject orchestration instructions. Prefer supported transient prompt/configuration surfaces.

Handoffs include the information the next task needs, not a demand to bypass the native harness's instruction hierarchy. Prompt content never outranks system-enforced policy.

### 9.7 Transport, streaming and bounded parsing

Use pipes for machine-readable protocols. Keep standard output and standard error separate. Each adapter defines framing, maximum frame size, timeout semantics, supported encodings and recovery from malformed data.

Bound line lengths, nesting, message rates and spool size. Large legitimate artifacts should use referenced files rather than giant unbounded events. Terminal escape sequences and untrusted hyperlinks are sanitised before rendering. Raw protocol capture is an opt-in diagnostic feature, not a default promise of replayable safe text.

When a user chooses a qualified native interactive attachment, suspend the outer TUI and hand terminal input to the native view through the single-input-owner path in Section 7.8. The worker retains process ownership. A headless attempt without same-session interactive attachment remains an inspector view unless an explicitly approved, safely quiesced transition is supported. Structured lifecycle monitoring remains separate; the outer interface must not claim to parse reliable tool decisions from decorative terminal output.

### 9.8 Compatibility policy

Maintain an adapter compatibility manifest containing the exact tested versions/platforms, protocol revisions, required flags, known limitations and last successful canary result. A native update does not automatically inherit certification.

For a newly observed version, permit only capabilities justified by a conservative compatibility rule and successful probes. Security-critical schema uncertainty fails closed. An explicit experimental override may relax product compatibility, but cannot manufacture a missing security guarantee.

Adapters are independently releasable through the same signed/pinned distribution mechanisms as other executable extensions. Their version remains recorded with every run.

### 9.9 SDK admission and native-fidelity qualification

Classify the integration before using the word “SDK” in a capability claim:

| Class | Meaning | Admission decision |
|---|---|---|
| Native CLI/control process | MYTHHELM owns an unmodified vendor executable; native code owns the agent loop. | Preferred, subject to mode-specific fidelity, billing and security qualification. |
| Thin control SDK/client | Library serialises messages to that same native process/server. | May be used as transport convenience; inherits no guarantees beyond the controlled native surface. |
| Agent-runtime SDK | Library starts or embeds native agent machinery with its own configuration/defaults. | Separately qualify; non-default. |
| Raw model SDK/client | Library sends inference requests; application supplies its own agent loop. | Not a valid substitute for a selected native harness. |
| Remote managed-agent API | Provider hosts an agent elsewhere. | Separate environment/entitlement profile; never an implicit local CLI fallback. |

An alternative must satisfy **both** gates:

**Fidelity gate:** identify the real runtime and versions; compare native scaffolding, instruction loading, tools, skills, hooks, MCP, context/compaction, session recovery, permissions, subagents and task-relevant browser/editor features. Classify every difference as preserved, intentionally restricted, unsupported, unobservable or unknown. Run comparable direct-native and integrated tasks against the same workspace/input/configuration. Capability parity alone is not proof of equal quality; performance claims require outcome evidence.

**Entitlement gate:** establish that the actual runtime, account, destination, model and selected surface are authorised to consume the user's included allowance; exclude paid credits/overages and hidden auxiliary calls. Vendor documentation, applicable permission and installation evidence must agree. Shared code, a common login screen, a successful request or a familiar credential format does not satisfy this gate.

Keep security as a third independent constraint: preserving the native default does not authorise an unsafe default. Where safety restrictions reduce native features, make that reduction visible and retain the native engine rather than quietly substituting another harness.

SDK alternatives need an explicit adapter identity/surface, compatibility record and user-visible enablement. Do not silently select one after a CLI error, quota exhaustion or native update.

### 9.10 OpenCode adapter

**Default:** operate the installed OpenCode native process/server. Its official SDK is a client interface to the OpenCode server, illustrating why a control SDK need not replace the harness. [S18] [S67]

OpenCode documents multiple inference providers and a separate OpenCode Go subscription option. There is no single universal entitlement attached to the executable's name. [S65] [S66]

**Implementation requirements:** qualify the complete OpenCode/provider/model/account bundle. Inspect main, small/auxiliary, title, summarisation, compaction, reviewer and fallback model configuration wherever observable. All possible inference routes must satisfy the billing policy; disabling only MYTHHELM's router is insufficient. A model-picker filter is not assumed to be a billing enforcement boundary.

Accept subscription-backed third-party providers only when that provider explicitly supports the use and the route passes inclusion/overage tests. Do not copy Claude subscription tokens into OpenCode, install credential-extraction plugins or advertise OpenCode as Claude Code. Native OpenCode plugins are part of its effective trust and billing configuration. Local-model profiles are separately labelled and are not proof of cloud subscription compatibility.

### 9.11 Kimi Code CLI adapter

**Default:** the installed `kimi` runtime. Prefer its native ACP entry point for a workflow requiring structured sessions and permission interaction; qualify the exact supported protocol. [S71]

The command reference documents print output but says non-interactive prompting uses automatic permission handling, with static denies retained. It also states that `--skills-dir` replaces automatically discovered skill directories. These are real fidelity/security differences, not equivalent defaults. [S70]

**Implementation requirements:** do not select print mode as the restrictive default merely because it emits JSON. Use a qualified ACP/native interactive path or prove a suitable static-policy/outer-sandbox envelope. Preserve skill discovery unless the user approved an explicit replacement. A native read-only mode name is not an enforcement claim.

Kimi Code is a membership-backed coding service with supported subscription-key access. [S68] Its clients and keys can share allowance; additional Extra Usage can continue requests after quota exhaustion. [S69] Use native login or the qualified Kimi Code subscription endpoint/credential, not an arbitrary Moonshot API configuration. Serving a Kimi model through another harness is a different execution bundle.

A future server adapter may use the documented local REST/WebSocket surfaces, which are explicitly experimental. Pin live schema/version evidence and protect the local bearer credential; do not turn that into a remote public service or reuse cloud subscription secrets outside the native runtime. [S72]

### 9.12 Cursor Agent CLI adapter

**Default:** the installed Cursor Agent CLI, with deterministic executable provenance. Its generic executable name must not be confused with another program on PATH. Headless/local execution and native session continuation are documented surfaces. [S21] [S74]

Cursor supports native browser authentication and Cursor-issued API keys; those keys are not automatically equivalent to an Anthropic/OpenAI inference API key. Authentication status includes account/endpoint information. [S73] Cursor pricing distinguishes included usage pools from additional usage. [S75]

**Implementation requirements:** qualify local CLI inclusion for the user's actual plan/model and reject paid on-demand continuation under subscription-only policy. Until the installed/account-specific evidence establishes that path, mark it pending qualification rather than promising that every subscription or CLI credential is included.

Do not treat Cursor's editor, local CLI, SDK-created agents, Bugbot, cloud agents or other cloud services as interchangeable billing/capability surfaces. Native CLI access does not imply that editor-specific context or tools exist in a headless workspace. Model selection within Cursor remains a Cursor execution bundle. No silent cloud handoff, background service activation, BYO-provider substitution or permission-expanding flag is allowed.

### 9.13 Antigravity adapter: native CLI first

**Default:** operate the installed, unmodified `agy` CLI. Google describes the CLI and desktop application as sharing the agent core. The terminal interface is not itself that core. [S55]

Its headless documentation includes persistent JSON input/output, native conversation IDs and per-turn results. It does not support arbitrary control messages or interactive slash commands in the stream; approval-dependent tools may be soft-denied while the process still exits successfully. [S56]

The preferred candidate invocation is:

```text
agy --input-format stream-json --output-format stream-json
```

This is a documented vendor command, **not a completed MYTHHELM adapter**. Implementation must drain stdout/stderr concurrently, frame bounded events, correlate native conversation/turn results, account correctly for cumulative counters, and distinguish soft permission denial from accepted task completion. Do not equate native exit zero with successful verification. A safe blocked state is preferable to adding a blanket skip-permissions flag.

Account/keyring sign-in and explicit Gemini API-key provider configuration are distinct native CLI paths. Google's CLI instructions require an explicit provider setting as well as the key for that API route; a key's mere presence is not a universal billing detector. [S57] The included-allowance candidate must also satisfy the overage gate in Section 13.9.

**SDK assessment:** the Antigravity SDK contains genuine agent machinery, so rejecting it as a mere raw Gemini client would be inaccurate. Its documented quickstart uses a Gemini API key, with a separate Google Cloud configuration; that does not establish consumer subscription inclusion. [S58] Its custom system-instruction mode can bypass default scaffolding/environment context. [S59]

**Decision:** do not use that SDK runtime as the default subscription integration. Admit a future alternative only through Section 9.9; retain the native CLI path regardless. Documented common machinery is not proof of matching prompts, features, permissions, quality or billing. Do not rewrite the native system scaffold or replace its tool loop to obtain a cleaner MYTHHELM API.

### 9.14 Qualification registry and honest coverage

Maintain one record per **harness × surface × native version × operating system × account/entitlement class × security profile**. Minimum fields are: upstream evidence/date; executable identity; fidelity differences; required approvals; auth/entitlement route; overage prevention evidence; quota-observation scope; child/auxiliary behaviour; compatibility fixtures; live-canary evidence; Herdr attachment status; and unresolved limitations.

Status values are `planned`, `documented-candidate`, `fixture-tested`, `live-qualified`, `experimental`, `blocked` and `unsupported`. Record billing qualification independently from feature qualification: a native adapter can work technically while remaining ineligible for strict subscription-only execution.

All seven named harnesses are product support targets. None is certified merely by appearing in this document. A release can ship a qualified subset and retain specific blocked reasons for the rest; it cannot claim “all subscriptions supported” from successful command discovery.

---

## 10. Collaboration, context and the blackboard

### 10.1 The blackboard is an evidence store, not a magic shared mind

Store structured tasks, decisions, messages, interface contracts and artifact references. Human-readable Markdown views are derived projections for inspection; they are not a second mutable source of truth.

A record states who produced it, against which revision, and whether it is a claim, a decision, a measured result or a user instruction. An agent saying “tests pass” is a claim until the verifier attaches actual execution evidence.

### 10.2 Message semantics

Message kinds include `question`, `answer`, `handoff`, `dependency_ready`, `reservation_conflict`, `finding` and `status_note`. Messages have a recipient, causal parent, referenced artifacts, expiry and sensitivity class.

Delivery states are deliberately precise:

| State | Meaning |
|---|---|
| `recorded` | The supervisor durably accepted the message. |
| `queued_for_next_turn` | The native mode cannot accept it now; it will be considered at a compatible boundary. |
| `submitted_to_native` | A supported native input operation accepted the payload. |
| `acknowledged` | An explicit native/user response references the message. This is not a guarantee of model comprehension. |
| `expired` / `rejected` | Delivery is no longer valid or violates policy; the reason is visible. |

A direct message is direct in addressing, not a bypass around the broker. Agents do not receive unauthenticated sockets into each other's native processes.

Where native tooling supports a suitably scoped MCP integration, a MYTHHELM collaboration tool can expose message and artifact operations using attempt-scoped capabilities. It is optional: MVP handoffs can occur through task boundaries and supported prompt injection. Adding an MCP server is itself a reviewed configuration and context change.

### 10.3 Handoff package

Every handoff should be compact, source-grounded and tied to revisions:

```text
Task goal and acceptance criteria
Current status and unresolved decisions
Input revision and immutable artifact references
Output contract and known dependencies
Relevant files with reasons and content hashes
Verification executed, results and limitations
Failed approaches worth avoiding
Required permissions and approved data destinations
Next concrete action
```

The recipient can request referenced material within its permissions. Do not stuff all historical transcripts into every new context or expose hidden reasoning that the native runtime does not provide.

### 10.4 Context budgeting

MYTHHELM can account exactly for bytes it packages and can estimate tokens when an appropriate tokenizer is available. It cannot promise exact visibility into a proprietary runtime's internal prompt, cache, compaction or hidden state.

Display separate fields: packaged context size; tokenizer estimate and identity; native-reported context/usage; native compaction events if exposed; unknown native overhead. Hash and version each handoff so that subsequent failures can be traced to the actual material sent.

When reducing a MYTHHELM context package, preserve acceptance criteria, policy references, interfaces, key decisions and unresolved failures. Keep source references available. Record what was omitted; “summarised” is not synonymous with “all detail preserved”.

### 10.5 Workspace awareness and staleness

Agents may inspect approved metadata about other tasks: intended areas, base revisions, published artifacts, claimed resources and current state. They do not read another live workspace by default.

A published artifact is immutable. If a producer changes it, the new artifact has a new hash and dependent tasks receive a staleness event. The scheduler decides whether to continue, invalidate or replan; agents cannot silently assume that their dependency changed beneath them.

A shared plan update uses optimistic concurrency against its previous version. Concurrent edits produce an explicit conflict rather than last-write-wins loss of a decision.

---

## 11. Workspace isolation, Git and integration

### 11.1 Three boundaries that must not be confused

| Mechanism | What it provides | What it does not provide |
|---|---|---|
| Logical reservation | Coordination intent for paths, interfaces or external resources. | Enforcement against arbitrary native file writes. |
| Separate working directory / Git worktree | Separate checked-out files and index for ordinary work. | Isolation from shared Git metadata, host files, credentials or network. |
| OS/container/VM sandbox | The specific enforced filesystem/process/network restrictions of that mechanism. | Automatic correctness, safe prompts or unqualified protection outside its actual scope. |

Git documents the relationships between linked worktrees and their shared repository data. The design must account for that shared metadata instead of treating a worktree as a security boundary. [S41]

### 11.2 Preserve the source checkout

The default workflow snapshots an explicit committed source revision into a MYTHHELM-managed run repository and creates attempt workspaces there. The user's original checkout is not the agent's working directory.

For trusted local workloads, managed linked worktrees are acceptable coordination isolation. For stronger isolation, use independent managed clones plus a verified outer sandbox. A clone without an OS boundary still does not contain hostile code.

An initial implementation may use independent clones for simplicity and stronger Git-metadata separation, then introduce shared object caches/worktrees only after measuring the benefit and testing their failure modes. Correctness does not depend on maximising checkout speed.

Do not auto-stash, auto-reset or auto-commit a user's dirty checkout. If it is dirty, admission offers: use a specified committed revision; wait for the user to prepare it; or explicitly snapshot selected uncommitted files with a preview. Ignored/untracked files and likely secrets are excluded unless deliberately included. A snapshot is identified separately from the source branch's HEAD.

Submodules, Git LFS, sparse checkouts, symlinks, case-sensitive filename pairs and very large repositories require explicit support declarations. Unsupported repository features produce a clear preflight result, not a silently incomplete snapshot.

### 11.3 One writer and advisory ownership

Each working directory has exactly one active writer owner. Logical reservations can cover file globs, interfaces, dependency manifests, migration numbers, generated outputs or external services.

Reservations help the planner avoid conflicts. They do not prevent a native agent from touching an unreserved file unless a real enforcement mechanism is configured. Compare the final diff with declared scope; unexpected sensitive changes block integration pending review.

Lease renewal is based on worker ownership, not arbitrary file activity alone. Use generation/fencing tokens for supervisor-mediated operations. An expired lease triggers reconciliation; it does not grant permission to a second writer while the first may still exist.

### 11.4 Shared resources beyond files

Parallel tests and development servers can collide through ports, local databases, container names, caches, package-manager locks, browser profiles, GPU memory and external test accounts.

Provide a resource registry with typed reservations and per-attempt namespacing. Prefer dedicated test databases, assigned port ranges and isolated browser profiles. Shared read-only caches are allowed only when the underlying tooling supports them safely; writable caches need their own policy.

External-resource locks require expiry and ownership recovery suitable for the resource. A dead supervisor does not automatically make a still-running database migration safe to duplicate.

### 11.5 Frozen candidates and the integration train

The integration procedure is normative:

1. Wait for the native attempt and owned file-writing descendants to stop or reach a verified quiescent checkpoint.
2. Record the attempt's base revision, complete candidate tree, changed paths and artifact hashes. Preserve uncommitted output; do not depend on the agent having committed correctly.
3. Validate scope, secrets, unexpected binary/generated changes, repository integrity and applicable policy.
4. Queue the frozen candidate in the serialized integration train.
5. Apply/merge it into a separate integration workspace based on the current integration revision.
6. Resolve textual conflicts as an isolated, reviewable task using base and both candidate versions. Never rewrite an active producer's workspace.
7. Execute the required checks against the exact combined revision.
8. Publish a review candidate with its diff, evidence and unresolved limitations.

Disjoint branches still require integration and semantic verification. Textual merge success is not proof that two changes work together. A dependency update and an apparently unrelated test change can conflict semantically without touching the same file.

### 11.6 Verification must resist self-certification

The admission snapshot defines the baseline check configuration. Changes to tests, build scripts or validators are legitimate possible work, but must be visible and reviewed. An agent cannot make a failed task succeed by deleting tests or weakening the validator unnoticed.

Record pre-existing baseline failures separately. Distinguish passed, failed, flaky, skipped, unavailable and not-run checks. Rerun checks after the final merge, not merely in each agent's branch.

For security-sensitive tasks, add explicit security acceptance criteria and human review where required. A second model's approval is an additional signal, not a substitute for actual checks or authorised review.

### 11.7 Apply, publish and reverse

Applying a review candidate requires the target branch and expected current revision. If the target changed, integrate again or stop for review; never force-push by default. The source checkout must be clean or otherwise explicitly handled before applying.

A rollback previews the exact revert or restoration operation in managed history. It does not silently discard unrelated work and does not claim to reverse external API calls, sent messages, payments or production data changes.

Pull requests use an explicit optional publisher integration. Production deployment and data migration execution require their own workflow, credentials, dry-run evidence and approval rules; they are outside the initial default workflow.

---

## 12. Security, trust and privacy

### 12.1 Threat model

Protect against accidental destructive changes, prompt injection from repositories or tools, malicious extensions, terminal-output attacks, credential leakage, unapproved spending, stale ownership, dependency supply-chain compromise and ambiguous recovery.

The initial product does not claim to contain a malicious process already running unrestricted as the same OS user. This limitation matters for native agents, trusted executable plugins, test scripts and the supervisor itself.

| Boundary | Example threat | Required defence and limitation |
|---|---|---|
| Repository content to policy | A README instructs the agent to disable tests or send secrets. | Treat content as data; only trusted user/core policy can authorise effects. |
| Native agent to filesystem/network | Tool calls escape the intended task scope. | Tested native/outer sandbox and least privilege; declare exact coverage. |
| Plugin to host | A router plugin reads credential files directly. | Real OS/WASM isolation where supported, or explicit full-host trust. Broker permissions alone do not stop direct syscalls. |
| Native stream to TUI | Escape sequences forge prompts, alter clipboard or mislead review. | Parse, escape and render through trusted components; block arbitrary terminal control. |
| Agent output to verification | Agent claims completion or alters checks to pass. | Independent candidate verification and changed-validator review. |
| Crash to external effect | A retry repeats a process launch or publication. | Durable intent, identifiers and effect reconciliation. |
| Diagnostics to outside recipients | Logs contain code, prompts or tokens. | Minimal default logging, local retention, previewed export and explicit upload. |

### 12.2 Execution profiles

Expose meaningful execution profiles rather than one misleading “safe” switch:

| Profile | Meaning |
|---|---|
| `inspect` | Read-only task under an established read-only mechanism. If read-only cannot be enforced for required tools, admission refuses this guarantee. |
| `restricted` | Verified native or outer sandbox with explicitly declared tool/filesystem/network coverage and no automatic unsandboxed fallback. |
| `trusted-host` | Native tools run with the user's host authority within disclosed native controls. Requires explicit trust; not advertised as containment. |

An adapter may offer only some profiles on a given platform. Built-in Claude sandbox documentation, for example, distinguishes macOS/Linux/WSL2 support from native Windows; the existence of a native CLI is not evidence of identical sandbox support. [S45]

MVP must never solve a missing security capability by automatically adding a “skip permissions” flag. Restrictive execution on an unsupported platform is blocked or explicitly replaced by a different approved profile.

### 12.3 Approvals bind to exact effects

An approval specifies the command/operation, argument array, working directory, identity, destination, resource scope, monetary exposure where relevant, artifact revision and expiry. It is one-use unless the user explicitly creates a narrowly scoped persistent rule.

Changing any security-relevant detail invalidates the approval. A model cannot convert approval for “run unit tests” into “install packages and deploy”. UI text generated by an agent or plugin cannot impersonate the trusted approval component.

Native approval bridges preserve native request identity and semantics. When the integration surface cannot support a necessary interactive decision safely, the run becomes blocked. Do not silently broaden tool permissions to avoid an interruption.

### 12.4 Effective configuration is part of repository trust

At admission, identify native hooks, MCP endpoints, helper executables, instructions, package scripts and test commands that could execute code or send data. Present the effective changes that matter, not a wall of every ordinary file.

Trust approvals are tied to content/configuration digests. A changed executable hook or new remote MCP endpoint requires renewed review. A repository cannot self-declare trusted plugins or credential scopes through its own configuration file.

Tests and package installation run with task credentials and sandbox restrictions appropriate to untrusted repository code. Never inject production secrets merely because an agent asked to verify something.

### 12.5 Secrets and authentication material

Leave native OAuth/subscription authentication under native control. MYTHHELM may store a profile's native configuration location and non-secret identity metadata; it must not create a cross-provider database of copied subscription tokens.

Optional API credentials or publisher tokens use OS-backed secret storage when available, referenced by identifier rather than embedded in project files. Plaintext fallback requires explicit informed opt-in, restrictive filesystem permissions and clear limitations. Native tools may have their own storage rules that MYTHHELM cannot silently change.

Do not put secret-bearing prompts or tokens on command lines when a supported input channel can avoid them. Redaction is defence in depth, not proof that an arbitrary raw log is secret-free.

### 12.6 Egress and the meaning of local-first

“Local-first” describes where MYTHHELM's state and coordination live. It does not mean native coding agents are offline.

Show an egress inventory covering selected native providers, native telemetry if known, MCP servers, optional routers, plugins, package registries, test services and publisher endpoints. Unknown native egress must be disclosed. Enforce network restrictions only through an actual supporting boundary, not by omitting destinations from a UI list.

`demo` and local rule routing can be fully offline. A strict offline execution policy must reject a native/cloud mode whose network activity cannot be disabled or contained. Automatic update checks, telemetry and remote registry refreshes are disabled in strict offline mode.

### 12.7 Logs, paths and terminal safety

Default logs contain structured status and redacted summaries, not complete prompts, source files or raw native transcripts. Diagnostic raw capture is per-run opt-in, bounded, labelled sensitive and excluded from ordinary support bundles unless separately selected.

Reject archive path traversal, unexpected symlinks, Windows junction escapes, absolute output paths outside allowed roots and ambiguous case-colliding paths. Path validation must be supplemented by OS enforcement where adversarial races matter; a pre-check alone is not a filesystem sandbox.

Sanitise control characters and escape sequences in filenames, diffs, tool output and plugin content. Clipboard operations and opening links are explicit host-controlled actions. A malicious filename must not become a button or a forged permission prompt.

### 12.8 External security review

Before a stable security claim, commission or solicit focused review of process ownership, plugin containment, approval binding, archive extraction, credential handling and cross-platform path behaviour. Publish the threat model and known limitations even when no external audit has yet occurred.

The absence of an audit is not an excuse to omit safeguards; it is a reason not to market unverified containment.

---

## 13. Authentication, subscriptions, quotas and budgets

### 13.1 Native sign-in, SDKs and provider permission

Anthropic's documentation distinguishes end-user sign-in to an **unmodified Claude Code binary** from a third-party product offering Claude.ai credentials/rate limits through its own application or SDK integration. It requires native authentication, direct end-user billing, an unmodified binary and applicable agreements; it also says built-in authentication methods must not be removed or disabled. [S46]

**MYTHHELM decision:** support the native-executable path, subject to those conditions. Do not present an SDK's ability to run the same agent code as permission to offer subscription-backed SDK access. No MYTHHELM login form, credential vault or token relay stands between the user and native sign-in. An SDK-specific approval or an explicitly authorised API-funded mode is a separately documented integration, not the default.

A subscription-only **launch policy** means refusing an ineligible invocation or selecting a user-approved child environment. It does not mean patching the vendor executable, hiding its login methods or preventing the user from operating it separately with another credential.

For every provider, a release owner records the current applicable terms, supported surface and unresolved restrictions. Being free or open source does not itself confer permission to redistribute binaries, use internal endpoints, share accounts or extend subscription entitlements. MYTHHELM's policy is to avoid those mechanisms; it does not claim a universal legal clearance. [S47]

### 13.2 Profiles are authorised execution contexts

A profile contains a stable label, exact native harness/runtime surface, adapter choice, native configuration reference, non-secret identity description, allowed projects, model/inference provider, billing entitlement and overage policy, data policy, fidelity record and capability evidence. It does not contain copied OAuth tokens or native subscription keys.

Separate personal and work profiles are useful, but their isolation must be tested against native file storage, keychains, environment variables and concurrent login behaviour. Discovering a configuration directory does not establish that it represents a separately authorised account.

Multiple legitimate profiles may share an account, organisation, provider limit or billing source. Model these relationships with explicit quota-bucket identifiers. Never assume profile count equals independent capacity.

When a limit is reached, honour the reported cooldown. Selecting a different already-authorised provider within an approved workflow is different from cycling identities to defeat a limit. MYTHHELM must not implement or market the latter.

### 13.3 Quota observations

Represent quota state with source and freshness:

```text
scope: provider/account/organisation/profile/model, as actually known
source: provider_reported | user_declared | locally_estimated | unknown
observed_at: timestamp
valid_until: timestamp or unknown
remaining: quantity and units, or unknown
reset_at: authoritative timestamp, estimated range, or unknown
cooldown_reason: structured classification
```

Do not fabricate “hours left”, a reset countdown or token capacity from elapsed wall time. A provider error can reveal a cooldown without revealing remaining quota. An unknown reset time appears as “waiting; retry policy active”, not a made-up clock.

Quota refresh itself must not trigger billable test prompts without consent. Use documented non-billable signals where available; otherwise retain uncertainty.

### 13.4 Three kinds of cost must remain separate

| Display | Meaning |
|---|---|
| **Subscription-included execution** | The qualified native route consumes the selected plan's included allowance. Remaining capacity may be unknown, but an unqualified paid-credit/overage path is not allowed. |
| **Metered spend** | Currency-denominated usage supported by provider/native reports or a clearly labelled pricing estimate. |
| **Retail-equivalent estimate** | An optional hypothetical comparison using a dated price table. It is not an invoice and must not be added to subscription spending as though charged. |

Avoid “free” or `$0.00` as a universal label for subscription work. An execution path may be included in a fixed payment while still consuming scarce allowance and time.

No native token metric, API price or plan fee should be hard-coded as current truth in the architecture. Version price data separately and show when it was last updated. Cross-currency aggregation requires an explicit dated exchange rate; otherwise show currencies separately.

### 13.5 Budget ledger and reservations

The ledger tracks actual reported spend, estimated accrued spend, pending reservations, released reservations and unknown/unbounded exposure. Use exact decimal or integer monetary representation with explicit currency, not floating-point currency arithmetic.

Reserve budget atomically across tasks before launching a metered activity. Include planning, routing, repair, review, handoff summarisation and auxiliary model calls. Reconcile actual results without double-counting cumulative native reports as per-event deltas.

A reservation is an internal scheduling control, not a provider spend cap. If actual spending exceeds a provisional reservation, record the deficit, block further admission and disclose it. Do not hide it by clipping displayed spend to the budget.

### 13.6 Hard limits versus soft guardrails

A **hard monetary limit** is available only when an enforcement mechanism can prevent spending above the declared cap, including relevant retries and in-flight exposure. A reported token count followed by process cancellation is usually insufficient to establish this guarantee.

A **soft guardrail** estimates and monitors spending, reserves expected usage and stops future work or requests cancellation when the threshold is approached. Its possible overshoot must be shown as a bounded range or explicitly unknown.

Proposed commands:

```text
mythhelm run --task-file task.md --billing subscription-only
mythhelm run --task-file task.md --billing metered-allowed --spend-limit USD:2.00
mythhelm run --task-file task.md --billing metered-allowed --spend-limit USD:2.00 --require-hard-limit
```

The second command reports whether the available enforcement is soft or hard before execution. The third refuses every route that cannot prove the requested hard cap. Currency examples are interface illustrations, not current service prices.

Subscription-only mode excludes all paid auxiliary inference and paid-credit/overage continuation, not just a MYTHHELM API fallback. Credential syntax alone does not establish billing: use the entitlement/admission contract below. A native subscription failure must never activate a metered route silently.

### 13.7 Configuration precedence and an example

Resolve configuration in this order: immutable core invariants; administrator/user security ceiling where configured; explicit trusted user choices; trusted project policy; ordinary defaults. Lower-trust configuration may narrow permissions but cannot broaden the ceiling.

The proposed project format is TOML. This example describes intent; native profile discovery must resolve actual supported installation details.

```toml
schema_version = 1

[execution]
default_profile = "codex-personal"
surface_preference = "native-executable-first"
require_native_fidelity = true
allow_runtime_sdk_alternative = false
max_writers = 1
max_repair_attempts = 2
sandbox_profile = "restricted"

[billing]
mode = "subscription-only"
paid_fallback = false
paid_overages = false
purchased_credit_use = false
unknown_entitlement = "block"
unknown_overage_prevention = "block"
require_hard_limit = false

[host]
mode = "auto"
herdr_bridge = "auto"
auto_create_panes = false
restore_action = "reattach"
share_native_credentials = false

[routing]
policy = "local-preference"
remote_router = false
preserve_session_locality = true

[delivery]
mode = "review-candidate"
auto_push = false
auto_deploy = false

[privacy]
telemetry = false
raw_transport_logs = false
remote_registry_refresh = false

[ui]
theme = "system"
colour = "auto"
motion = "auto"
icons = "auto"
```

If `restricted` or the included-only billing requirement is unsupported for the selected profile, this configuration blocks execution; it does not silently choose `trusted-host` or a chargeable route. `require_hard_limit = false` does not weaken subscription-only admission; it controls monetary-cap enforcement for deliberately metered profiles only. Unknown keys and invalid enum values produce actionable errors. Show the effective merged configuration and its provenance with `mythhelm config explain`.

### 13.8 Included allowance is the default—not merely subscription authentication

**Normative definition:** `subscription-only` admits inference charged against the user's existing included entitlement and no separate consumption purchase. Included credits delivered as part of a plan may qualify when their provenance is established. Separately purchased credit balances, usage-credit top-ups, overage wallets, additional on-demand usage and chargeable speed/model options do not qualify. Promotional or ambiguous credits are not silently treated as included allowance.

The relevant distinction is **entitlement and cost**, not “uses HTTP”, “an SDK exists”, “API key versus OAuth”, or “the price table is expressed per token”. Native applications necessarily contact services; MYTHHELM must not replace their authentication/inference path or cause separately funded usage without approval.

Policy applies to the complete execution tree: native agents/subagents, auxiliary models, planners, repair/review attempts, summarisation, routing, title generation and enabled inference plugins. Tool/MCP calls that can incur separate service charges also require an explicit allowed-effect policy; the inference admission check does not certify every external tool as free.

Exhaustion results in a preserved partial state plus `waiting_for_allowance` or `blocked_billing`. These are structured reason codes within the existing waiting/blocked lifecycle states, not additional unversioned top-level states. The run and attempt still use the state machines in Section 7.2. An already authorised alternative profile may be proposed or selected within a preapproved provider/harness routing policy after receiving the necessary task context. A pinned harness is not silently replaced. No automatic upgrade, credit purchase, reload, API fallback, identity cycling or disabling of native limits is permitted.

### 13.9 Per-harness entitlement and overage requirements

Primary-source observations below establish risks and candidate routes, not live MYTHHELM certification.

| Harness | Evidence relevant to billing | Admission requirement |
|---|---|---|
| Claude Code | Native subscription and Console/API routes differ; optional usage credits can fund work beyond included limits. [S61] [S62] | Require qualified native subscription execution; reject active API/Console/cloud-provider substitutions and chargeable extras. A native login alone is insufficient. |
| Codex | ChatGPT account access can include allowance; additional purchased credits can fund continuation, and enterprise agreements can have different billing models. [S63] [S64] | Qualify the actual plan/workspace and native route. Prevent consumption of purchased credits, not just automatic purchases; reject metered enterprise profiles under this policy. |
| OpenCode | Provider configuration determines the funding route; OpenCode Go is a distinct subscription offering. [S65] [S66] | Qualify the provider's included models, auxiliary routes and exhaustion behaviour; do not infer inclusion for arbitrary configured providers or every model. |
| Muse Code | The onboarding-associated Muse Code credential carries subscription entitlement; additional keys are separately billed. [S13] | Preserve that native product-specific credential path, establish precedence and stop at allowance exhaustion. |
| Kimi Code CLI | Native clients and supported subscription keys can share membership allowance; enabled Extra Usage draws from an additional balance when limits are reached. [S69] | Use the verified Kimi Code entitlement and establish that Extra Usage cannot fund this attempt. Do not confuse it with a metered Moonshot endpoint. |
| Cursor Agent CLI | Native account authentication is distinct from the choice of included pool/additional usage; account pricing provides for paid continuation. [S73] [S75] | Require evidence for local CLI/model inclusion and paid-continuation prevention on the actual account. Otherwise leave subscription-only support pending/blocked. |
| Antigravity CLI | The plans page provides baseline allowance and an “AI Credit Overages” setting with `Never` and `Always`; `Never` waits for baseline allowance instead of consuming credits automatically. [S60] | Use native account-backed execution, establish effective overages=`Never` and exclude the explicit Gemini API-key route. Qualify that the selected CLI/account/model honours those settings. |

**No broad credential stripping:** Muse and Kimi demonstrate why an API-key-shaped credential can represent a product subscription. Conversely, a browser/OAuth login can lead to a separately chargeable service. Inspect only supported non-secret status/configuration evidence where possible; do not read and classify raw token contents as a substitute for product provenance.

**Existing credit balances matter:** disabling automatic reload does not necessarily prevent spending a balance already purchased. A provider-side zero-purchase setting alone is not evidence that the attempt cannot draw from extra credits. Qualification must address consumption, including concurrent usage outside MYTHHELM and in-flight exposure.

### 13.10 Billing evidence, admission and configuration drift

The proposed billing record contains:

```text
harness_id / runtime_surface / executable_version
inference_provider / endpoint_class / model_selection
native_identity_reference / workspace_or_organisation
credential_provenance: native-login | native-subscription-key | other | unknown
entitlement_class: included-plan | purchased-credits | metered | local | unknown
included_allowance_bucket / auxiliary_and_child_routes
paid_continuation: prevented | possible | unknown
prevention_mechanism / scope / evidence_id / observed_at / expires_at
last_terms_review / fidelity_record / configuration_digest
admission: eligible | needs-native-setup | blocked | unsupported
```

The record stores references and non-secret evidence, not credential values. Entitlement class and credential provenance are separate fields.

Admission sequence:

1. Resolve the actual executable, native surface, model/provider and complete effective configuration. Do not assume a clean environment inside Herdr, a shell, a container or an IDE.
2. Obtain native authentication/status evidence without launching a billable diagnostic prompt. Identify conflicting provider settings, account types, custom endpoints, helper scripts and auxiliary routes.
3. Establish the allowed included entitlement and whether paid continuation is prevented for the execution scope. Require provider/native enforcement where a hard no-extra-spend claim is made; local token estimates are insufficient.
4. Where account settings need adjustment, provide the native setup path and re-check. Do not mutate subscription/credit settings automatically. User-confirmed settings can be recorded as such, but do not promote unverified assertions into a hard enforcement claim.
5. Under strict subscription-only, admit only a qualified included route with the required prevention evidence. Unknown remaining quantity is acceptable only if exhaustion cannot switch the task to chargeable funding. Otherwise block with the specific missing check and a concrete native setup action.
6. Pin the approved configuration and monitor observable identity/provider/settings changes. Reconcile before additional turns, retries, child launches or reconnects. Never promise to intercept opaque intra-turn provider changes: that guarantee must come from the qualified provider/native boundary.
7. After execution, reconcile native reports and retain uncertainty. A retail-equivalent token-cost display is not proof of an invoice or proof that no credits were consumed.

A stale account setting, an unsupported billing probe or inconsistent documentation reduces qualification, rather than silently relaxing policy. For example, Antigravity's general plans and CLI authentication pages differ in their descriptions of BYO-key availability; use the specific CLI authentication documentation to identify a possible paid route and qualify the installed behaviour, without extending one page's entitlement claims to another surface. [S57] [S60]

**Release honesty:** the design goal is included-only execution. This document does not prove that every named provider exposes enough controls to certify it today. An adapter can be available in an explicitly metered mode while its strict subscription mode remains unavailable. The UI must explain that distinction without pressuring the user to spend.

---

## 14. First-class plugin and mod architecture

### 14.1 Design goal

People should be able to change how MYTHHELM looks, plans, routes, verifies, integrates with native agents and presents results without maintaining a fork of the core. They should not need to trust executable code merely to change a colour scheme.

The extension system is part of the product architecture from the start. Its stable surface should be small enough to maintain; not every internal Go interface becomes public API.

### 14.2 Three extension classes

| Class | Examples | Execution and trust |
|---|---|---|
| **Declarative mods** | Themes, keymaps, layouts, workflow recipes, routing-rule packs, prompt templates. | Parsed as data by the core. No install scripts, arbitrary imports or shell hooks. Recipes still require review because they can influence future authorised work. |
| **Executable plugins** | Native adapters, routers, context providers, verifier integrations, publishers/exporters. | Versioned process protocol. MVP third-party executable plugins require explicit trusted-host approval unless an independently verified sandbox is available. |
| **Sandboxed computational plugins** | Pure scoring functions, transformations and bounded context selection. | Later WASM or equivalent with explicit capabilities, memory/time limits and no ambient host access. Native CLI spawning is not automatically available inside this class. |

A process boundary provides crash isolation and a protocol boundary. It is **not** an OS security boundary. The go-plugin project is a useful precedent for process-based extensibility, not evidence that arbitrary child processes cannot access the host. [S48]

### 14.3 Supported extension points

| Extension point | Receives | Returns | Core retains |
|---|---|---|---|
| Native adapter | Approved attempt context and native configuration references. | Fidelity/entitlement evidence, invocation/transport mapping and native observations. | Admission, user consent, worker ownership, global limits and evidence policy. |
| Router | Permitted task descriptors and eligible candidate metadata. | Ranked proposal with reasons and uncertainty. | Hard constraints and final admission. |
| Planner/workflow | Goal, accepted artifacts and allowed workflow vocabulary. | Bounded task DAG and acceptance criteria proposals. | Task validation, recursion/concurrency limits and permission checks. |
| Context source | Explicit scoped query and artifact capabilities. | Provenance-linked content with sensitivity labels. | Access policy, egress and retention decisions. |
| Verifier | Frozen candidate and allowed check request. | Observations and evidence references. | Classification of trusted versus advisory evidence, required checks and final acceptance. |
| Publisher/exporter | Approved immutable artifact and exact requested destination. | External effect receipt and resulting identifiers. | Effect approval, idempotency/reconciliation and credentials. |
| UI panel mod | Sanitised read-only event projection. | Declarative host-rendered components/actions. | Rendering, focus, accessibility and trusted approval surfaces. |
| Terminal-host bridge | Scoped host attachment and sanitised run/attempt projection. | Pane presentation, attention/status proposals and authenticated user intents. | Native process ownership, admission, billing, approvals, task completion and durable state. |

An extension cannot declare its own output to be core-trusted verification just by naming it `passed`. A project chooses which verifier identities satisfy which checks, and changing that policy requires trusted approval.

### 14.4 Protocol and process model

Use JSON-RPC 2.0 over dedicated standard-input/output pipes for executable plugins. Standard error carries bounded diagnostics. Protocol major versions are negotiated during a mandatory handshake; feature capabilities are negotiated separately.

Required protocol concepts: initialisation, manifest identity, version negotiation, request IDs, cancellation, deadlines, health, structured errors, bounded events, graceful shutdown and maximum message sizes. Unknown required methods fail clearly; optional features can remain absent.

Plugins return structured proposals rather than pre-rendered terminal bytes or shell command strings. Native invocation proposals use executable identities, argument arrays, environment allowlists and explicit working directories. The trusted worker performs launches wherever the integration permits this model.

An executable adapter with arbitrary host access remains part of the trusted computing base even when its supported API looks narrow. The UI must say so.

The core must tolerate plugin crashes, deadlocks, malformed frames, cancellation refusal and excessive output without crashing the mission controller. Such failures may fail or block a task, but cannot auto-authorise a fallback or erase evidence.

### 14.5 Manifest example

This example is deliberately labelled as trusted host code; its narrow broker requests do not imply sandboxed execution.

```json
{
  "manifest_version": 1,
  "id": "example/local-router",
  "version": "0.1.0",
  "kind": "router",
  "licence": "Apache-2.0",
  "core_compatibility": ">=0.1.0 <1.0.0",
  "protocol": {
    "name": "mythhelm-plugin",
    "major": 1
  },
  "runtime": {
    "type": "native-process",
    "entrypoints": {
      "linux-amd64": "bin/router-linux-amd64",
      "darwin-arm64": "bin/router-darwin-arm64",
      "windows-amd64": "bin/router-windows-amd64.exe"
    }
  },
  "execution_trust": "trusted-host",
  "requested_broker_capabilities": [
    "task.read_metadata",
    "profiles.read_capabilities",
    "routing.propose"
  ],
  "configuration_schema": "schemas/config.json"
}
```

The installation lockfile records actual artifact hashes, source identity, verification status and the user's granted capabilities. Integrity is established from the downloaded artifact and trusted metadata, not by trusting a plugin's self-declared hash or publisher string.

Architecture coverage is explicit. Missing macOS x86 or Linux ARM binaries mean those variants are unsupported for this example, not silently emulated.

### 14.6 Declarative mods are genuinely useful

A theme controls semantic colours, border styles, spacing presets and icon choices within accessibility limits. A layout chooses approved panels and breakpoints. A keymap binds approved actions without arbitrary shell evaluation. A workflow recipe proposes named phases, native role preferences, artifact contracts and checks.

A prompt pack can define specialist instructions, but cannot alter security ceilings, authentication, billing, trusted approval UI or required verification. Prompt packs are reviewed as instruction-bearing content, even though they are not native executables.

Mods may compose through explicit dependencies pinned in the lockfile. Cycles, missing dependencies and incompatible semantic tokens fail validation. A theme cannot hide the active spend warning or make a required approval invisible. Users can always reset to the built-in theme/keymap using a documented command independent of the current bindings.

### 14.7 Installation, updates and revocation

The installation flow is **inspect → verify → review changes → install → enable**. Installation does not imply enablement. Project-local references cannot bypass user approval.

Download only from an explicitly chosen source. Validate archive paths, total extracted size, file types and executable inventory. Do not run package installation scripts from an extension bundle. Show runtime dependencies before enabling a plugin that needs Node, Python or another interpreter.

Pin versions and content digests. Verify release signatures/attestations where available, while explaining what identity was verified. A signature is provenance, not proof of harmless code.

Updates show capability, licence and executable changes. New permissions require fresh consent. Active runs keep their pinned artifact unless a user chooses a controlled interruption. Security advisories may recommend disabling a plugin; fetching advisories is optional network activity and must not be hidden in offline mode.

### 14.8 Distribution and registry

Use an optional public GitHub-hosted index with downloadable static metadata; users can install from local paths or explicit repositories without that index. The index is not an execution service and does not require a MYTHHELM account.

Separate “listed”, “identity verified”, “conformance tested” and “security reviewed” statuses. None is a universal safety guarantee. The official catalogue remains free; third-party licences and costs must be displayed. The core licence does not grant maintainers the power to promise that every external plugin is free.

### 14.9 SDK, examples and contributor experience

Publish a language-neutral protocol specification, JSON Schemas, golden fixtures and a conformance runner. Provide a Go reference SDK first; add a small TypeScript/Python example only when it helps real contributors rather than multiplying maintenance prematurely.

Ship three zero-credential examples: a declarative theme/layout pack, a deterministic routing-policy plugin, and a scripted fake native adapter for lifecycle testing. A sample verifier should demonstrate the distinction between advisory findings and trusted checks.

`mythhelm plugins test` exercises handshake failures, cancellation, oversized output, unsupported versions, invalid proposals and permissions. Passing conformance tests does not certify that arbitrary trusted-host code is safe.

### 14.10 Extension scope by release

The first usable slice includes a documented internal adapter seam, declarative themes and the scripted adapter. The public parallel alpha adds the versioned external process protocol, local executable-plugin inspection and explicitly trusted-host installation. A sandboxed arbitrary-code plugin marketplace is **not** required to deliver the core product and must not be falsely advertised in the MVP.

### 14.11 Extensibility must not erase native identity or billing boundaries

A community adapter must declare its real harness and runtime surface. A plugin using model APIs cannot advertise itself as a native Claude Code/Codex/Muse/Kimi/Cursor/Antigravity adapter merely because it selects the same model or reproduces a prompt.

The official Herdr bridge is a host integration: no inference credentials, subscription tokens or authority to spawn unadmitted native attempts. It receives only approved host context and projection data. Core functionality remains usable without that bridge; the bridge and its reference tests remain free and open source.

Workflow/prompt mods may personalise task instructions and supported native configuration after review. Replacing default scaffolding, removing native tools or changing the selected harness creates a visible configuration variant and invalidates incompatible fidelity evidence. A mod cannot disable the two admission gates, disguise metered usage as included, or relabel an SDK-created variant as the unchanged native agent.

Trusted-host executable plugins remain arbitrary code within the previously declared threat boundary. Broker rules alone do not prevent them from spending via credentials they can read on the host. Strong “no additional spending” claims therefore require appropriate credential/environment containment or exclusion of unqualified executable plugins.

---

## 15. CLI and TUI experience

### 15.1 Experience thesis

The interface should feel like a finely made command deck: legible, calm, responsive and alive. The delight comes from clarity, momentum and tactility, not from hiding uncertainty behind spectacle.

Default to a focused single-mission view. Show the user's goal, the next required action, current agent activity, the candidate diff and verification status before showing abstract metrics. An advanced dashboard is available when multiple tasks or missions justify it.

### 15.2 Visual language

Use a restrained “observatory at night” visual direction for the default dark theme: deep neutral surfaces, cool luminous focus accents and warm attention accents. The light theme uses clean paper-like surfaces with the same semantic hierarchy. Exact palette values are implementation design tokens to be checked for contrast in supported colour modes.

Avoid vendor logos as the primary navigation system. Each lane has a readable runtime/profile label and a stable local identifier. State uses text plus shape/icon and optional colour. Decorative glyphs always have ASCII alternatives.

The terminal owns font selection and font rendering. MYTHHELM may recommend a comfortable monospaced font in documentation, but must not require, install, bundle or claim control over it. Nerd Fonts are optional, never necessary for comprehension.

### 15.3 Wide mission view

Illustrative layout, not a screenshot or a promise of fixed terminal width:

```text
 MYTHHELM   audit-service / run_01        2 active   REVIEW CANDIDATE
 Goal: Add pagination without changing existing audit-log behaviour

 TASKS                    AGENTS                         SELECTED CHANGE
 ----------------------   ----------------------------   ----------------------
 [done] Agree API         A  Codex / personal             src/audit/list.ts
 [run ] Implementation      Running configured checks    base: admitted snapshot
 [run ] Documentation       Workspace: task-02           diff: +38 / -9
 [wait] Combined checks     Scope: audit endpoint
                                                         Verification
 Dependency: API v1       B  Claude Code / personal       [pass] Unit checks
 contract published         Updating API examples        [wait] Combined build
                                                         [req ] Human review

 Route: kept A's existing context; B has independent documentation input
 Billing: included route qualified | quota unknown | paid continuation OFF
 Next action: none. Outputs remain in managed workspaces.

 Tab focus   / filter   : commands   ? help   Enter inspect   q exit options
```

The selected change panel is a real diff viewer with syntax-aware, width-safe presentation where supported. Markdown rendering is used for prose; it is not a substitute for a proper diff component.

### 15.4 Responsive layouts

| Available terminal size | Layout |
|---|---|
| At least 140 columns and sufficient height | Tasks, agents and selected detail; optional compact global status. |
| 100–139 columns | Two panes with a switchable detail panel. |
| 80–99 columns | Single focus pane with labelled tabs and persistent critical status. |
| Under 80 columns or very short height | Compact task/status view; offer linear output. No broken half-panels. |
| Non-TTY, `TERM=dumb`, machine output or explicit plain mode | Stable linear output without cursor movement or decorative control codes. |

Height is considered independently of width. Resize never loses a pending approval, moves a destructive action beneath an already-held key, or changes the selected task unexpectedly.

### 15.5 Navigation and actions

Provide arrow keys and standard focus navigation; optional Vim-style `j/k` accelerators are an addition, not a prerequisite. `/` filters the current collection. `:` opens a searchable command palette. `?` shows contextual help. `Enter` opens the selected item's details; `Escape` backs out safely.

Mouse support is optional and must never be the only route to an action. Copying text, inspecting a full path and opening a diff must remain easy with keyboard and plain output.

Potentially destructive or costly actions use explicit labelled confirmations. A convenient shortcut must not transform an accidental keypress into a push, permission expansion, spend escalation or irreversible cleanup.

### 15.6 Ten signature animated moments

Durations are design targets, not measured results. Motion never delays real output or blocks input.

| Moment | Purposeful motion | Truth and accessibility rule |
|---|---|---|
| 1. Arrival | A brief emblem/focus reveal while actual data loads. | No mandatory boot animation; immediately interruptible. |
| 2. Route selected | Candidate row resolves into a compact decision card. | Show the actual reason; no fake model “thinking” sequence after the decision is known. |
| 3. Dispatch | Task highlight transfers to the assigned lane. | A lane becomes running only after launch acknowledgement. |
| 4. Live work | Small activity pulse and efficient text append. | No artificial typewriter delay; distinguish active transport from proven progress. |
| 5. Handoff | Brief connector/highlight between sender and recipient. | Label queued versus submitted; animation does not imply the recipient understood. |
| 6. Waiting | Slow, restrained state indicator with static reason. | A countdown appears only when a reset/deadline is known. |
| 7. Reservation conflict | Shared resource highlights and the waiting task explains why. | Say “overlap detected; waiting”, not “all conflicts prevented”. |
| 8. Integration | Real stages illuminate: frozen, combined, checked, ready. | No progress percentage unless there is a meaningful denominator. |
| 9. Delivery | A short checkmark/receipt reveal. | Celebrate “Ready for review” when that is the actual result; optional flourish can be disabled. |
| 10. Failure/recovery | Attention moves to the actionable error and preserved artifact. | No screen shake, flashing red strobe or shame-inducing spectacle. |

Use roughly 80–200 ms for short transitions; occasional success emphasis may last longer without delaying interaction. Avoid constant motion in every lane. Reduced-motion mode uses immediate state changes and static emphasis.

### 15.7 Colour, motion and terminal capability

Support `--colour auto|always|never`, `--motion auto|full|reduced|off`, and `--icons auto|unicode|ascii`. Accept `--color` as an alias for compatibility with common conventions.

Respect `NO_COLOR` for colour suppression. It does not itself mean “disable animation”; colour and motion are independent preferences. [S49]

Automatic capability detection is best effort: true colour, 256/16 colours, Unicode width, background and terminal features can be wrong or unavailable. Expose explicit overrides and never rely only on `COLORFGBG`. Honour OS reduced-motion preferences where reliably queryable, but document the explicit motion flag as the portable control.

### 15.8 Accessibility

Provide a linear screen-reader mode with complete state labels, no repeated spinner chatter, no cursor-rewrite dependence and an ordered stream of meaningful changes. Test it with NVDA, VoiceOver and Orca in representative terminals before claiming accessibility support.

Focus is visible and stable. Help describes full action names. Colour is not the sole indicator. Important text is not conveyed only through animation, Unicode icons, hover or a changing progress bar.

Long content can be paged or exported as plain text. Bidirectional text, combining characters, emoji and wide glyphs must not displace trusted approval controls or hide filenames. An application cannot create screen-reader support merely by binding a “narrate” shortcut.

### 15.9 Render model and responsiveness

The UI subscribes to coalesced view updates while the supervisor preserves critical events. Prefer event-driven rendering with a modest active animation cadence; allow up to 60 fps for brief motion when the terminal and device can support it, rather than demanding a permanent 60 fps loop.

Virtualise long lists and diffs. Limit on-screen streaming to a useful tail with searchable history. Backpressure and disk policy protect process I/O independently of rendering. A paused or slow terminal must not stall native event consumption indefinitely.

### 15.10 Command taxonomy and exit codes

Keep top-level verbs understandable:

```text
run, plan, demo, doctor, init, attach, stop, recover
runs, review, apply, profiles, config, plugins, mods, theme
logs, export, gc, completion, version, upgrade
```

`gc` previews eligible cleanup; destructive confirmation is explicit. `upgrade` is user-initiated and verifies provenance. `doctor` is read-only unless a separate repair command is chosen and previewed. No shell initialisation is required for basic operation.

Proposed stable exit meanings:

| Code | Meaning |
|---|---|
| 0 | Requested command/deliverable succeeded; a run result explicitly states whether it is a review candidate or applied change. |
| 2 | Invalid arguments or configuration. |
| 3 | Admission/policy/approval blocked; no implicit approval was taken. |
| 4 | Native execution failed after permitted recovery/repair. |
| 5 | Verification failed or required verification remained unavailable. |
| 6 | Interrupted/recovery required or ownership unresolved. |
| 7 | Required adapter/platform capability unavailable. |
| 130 | User cancellation completed for a foreground run. |

A `plan` command using a paid planner is not a free local operation and must be admitted accordingly. JSON output carries a structured error category in addition to the process exit code.

### 15.11 Herdr presentation and native-agent inspection

Inside Herdr, retain the same polished, responsive mission view. Avoid a redundant second full workspace navigator; the host already has a surrounding pane/workspace context. The MYTHHELM pane still explains tasks, selected native harness, approvals, actual changes, verification and included-allowance status.

Always label a lane view as one of **MYTHHELM structured inspector**, **native interactive terminal**, or **historical transcript**. A reconstructed event stream must not masquerade as the native CLI UI. A history viewer must not be reported as a running agent just because its text contains a vendor name or old “working” output.

Proposed controls:

```text
mythhelm attach run_01 --host herdr
mythhelm attach run_01 --attempt attempt_02 --view inspector
mythhelm attach run_01 --attempt attempt_02 --view native
```

`--view native` succeeds only with a qualified live attachment or an explicitly approved safe native-mode transition. It never launches another writer automatically. Show capability limitations and offer the inspector when live native attachment is unsupported.

Permission decisions originate in one trusted approval path. Herdr notifications identify the task needing attention but do not offer a generic “approve everything” shortcut. Restoring, focusing, resizing, marking seen or closing a notification never approves a tool call or spends more allowance.

---

## 16. Cross-platform, shell and terminal support

### 16.1 Separate four support claims

Publish separate compatibility fields for the **MYTHHELM host binary**, **native adapter**, **sandbox profile**, and **terminal interaction**. A successful host launch establishes none of the other three automatically.

Initial intended platform targets are Windows 11 or later on x86-64; macOS on Apple Silicon and x86-64 where the selected supported OS releases and native runtimes allow it; and mainstream Linux on x86-64 and ARM64. The final support policy pins actual tested OS releases and architecture combinations. Windows ARM64 can be added when native-agent and process tests justify it; it is not implied by Windows 11 wording alone.

Do not claim universal Linux compatibility. Record libc requirements, filesystem assumptions and supported distributions. WSL is a useful separate environment, not a substitute for meeting a native Windows claim.

### 16.2 Platform-specific concerns

| Platform | Required tests and considerations |
|---|---|
| Windows | Job Objects, console/ConPTY attachment, named-pipe ACLs, executable/script resolution, path spaces, junctions, case behaviour, long paths, CRLF, Ctrl-C and native sandbox coverage. |
| macOS | ARM/x86 release artifacts, process groups, terminal input differences, keychain behaviour, sandbox prerequisites, signing/notarisation decisions and supported OS versions. |
| Linux | Distro/libc targets, process groups, credential-store availability, terminal variations, filesystem permissions, sandbox dependencies and headless operation. |

For every platform, test user paths containing spaces and Unicode, read-only repositories, disk exhaustion, unexpected process death and a native CLI update.

### 16.3 Shell compatibility

The CLI accepts ordinary arguments consistently, but each shell has its own parsing rules. Prefer `--task-file` for long or complex instructions. The application must not pretend it can retroactively fix shell quoting that occurred before it received the argument vector.

| Shell | Support commitment |
|---|---|
| bash, zsh, fish | Native command invocation, shell-specific completion generation and documented quoting examples. |
| PowerShell 7+ | Native command invocation, generated completion, spaces/Unicode tests and explicit argument-array behaviour. |
| cmd.exe | Working native invocation and documented quoting/path examples. No promise of shell completion features the shell does not provide equivalently. |
| Nushell | Native external command invocation, completion integration where supported and tested argument handling. |

Resolve native executables deterministically. Prefer the actual binary over a shim when safe and supported. `.cmd`, `.bat` or shell-script launchers require a tested platform-specific strategy; they must not cause untrusted prompt text to be reinterpreted as shell code.

Do not automatically edit shell profiles or PATH. Installation/completion setup is explicit, reversible and explained.

### 16.4 Terminal validation matrix

The target matrix includes Windows Terminal; macOS Terminal; iTerm2; Ghostty; Alacritty; Kitty; GNOME Terminal; VS Code's integrated terminal; JetBrains' integrated terminal; and **Herdr as a first-class host** on each claimed combination. Also test SSH and common multiplexer sessions where feasible. Herdr integration is not deferred to the generic “multiplexer” test bucket.

For each applicable OS/terminal combination, record: minimum supported version if needed, colour mode, Unicode/ASCII behaviour, resize, paste, focus, keyboard, mouse, copy, alternative-screen restoration, interrupted exit and plain/screen-reader results.

This is a **required validation plan**, not a declaration that all combinations have already passed. Release notes distinguish tested, community-reported, experimental and unsupported combinations.

### 16.5 Distribution

Start with versioned release archives and one documented installation path per primary OS. The main CLI/TUI/supervisor/worker binary should not require Go, Node or Python at runtime. Native agent executables, Git and any required sandbox components remain explicit external prerequisites.

Expand to Homebrew, winget/Scoop and Linux packages after the release pipeline is reliable. An npm downloader shim is optional distribution convenience, not a reason to make Node mandatory. Do not promise every package manager in the first release.

Checksums, signatures or attestations, dependency notices and uninstall instructions accompany releases. The update command never runs silently and never replaces the binary while its workers are in an unsafe transition.

### 16.6 First-class Herdr integration

#### 16.6.1 Verified host behaviour and limits

Herdr documents real terminal panes, foreground-agent detection and differing state authorities: some integrations report lifecycle state, while others depend on screen detection. A wrapper can obscure the foreground agent. A custom screen manifest alone does not necessarily register a new executable as an agent kind. [S52]

Its automation interface separates layout creation, raw pane interaction and recognised-agent control. Lifecycle waits are not per-turn acknowledgements; a timeout can occur after input was submitted. Treat these as host observations, not reliable inference/task receipts. [S53]

Its socket API provides `pane.report_agent`, separate native-session reporting and display-only metadata. Host session/socket selection has documented precedence, and lifecycle event subscriptions do not replay prior events. Those are candidate integration surfaces to qualify, not proof that MYTHHELM is already supported. [S54]

#### 16.6.2 Required topology and ownership

**Default supported topology:** one MYTHHELM client in an ordinary Herdr pane, connected to the local MYTHHELM supervisor. The worker owns the native process. A bridge reports MYTHHELM mission state for its own pane; optional read-only lane viewers can be added after explicit user action. Native runtime credentials remain local to the execution host and under native control.

Do not call Herdr's native-agent launcher and also launch that agent in a MYTHHELM worker. Do not invent `--kind mythhelm` or assume a `HERDR_AGENT` hint makes the MYTHHELM TUI look like Claude/Codex. A native-agent hint may only describe a real matching native foreground surface through a qualified wrapper path—not an aggregate MYTHHELM dashboard.

The bridge must qualify registration/state reporting for the actual installed Herdr version. If rich registration is unavailable, ordinary terminal execution still works, but advertise **embedded-terminal compatibility only**, not full lifecycle integration. First-class rich integration remains a tracked release gate rather than being faked through a vendor label.

#### 16.6.3 Scoped discovery and connection

Treat host environment values as hints, not authenticated authority. Resolve the documented current session/socket and pane through Herdr's supported CLI/API; verify that the endpoint belongs to the expected user/execution host and that the pane still belongs to the attaching client. Do not hard-code one global socket, assume pane IDs are globally unique, or inherit a launcher's host context into every native child indiscriminately.

Bind an attachment to the Herdr server/session, terminal/pane, MYTHHELM run and optional attempt, plus a reconnect generation. Follow authoritative identifiers after moves and revalidate after replacement/restart. Do not retarget an existing run because the user selected another machine in the host UI.

The per-user supervisor must not retain the environment of whichever Herdr pane happened to start it as the global host identity for future runs. Store host context per attachment; use an explicit environment allowlist for native children. Keep host control endpoints and plugin credentials out of agent-accessible environments unless a narrowly scoped feature requires them.

#### 16.6.4 State and metadata mapping

This is a **proposed MYTHHELM projection**, to be tested against the actual host reporting contract. It is not a new Herdr state schema.

| MYTHHELM condition | Proposed host semantic projection | Display/behaviour |
|---|---|---|
| Planning, native execution, integration or checks active | `working` | Include concise task phase; no invented percentage. |
| A real user decision is required | `blocked` | Exact attention reason; focus the corresponding trusted approval or billing/setup screen. |
| Waiting for a known dependency/quota reset without required input | `idle` plus waiting metadata | Do not invent a blocked approval or a working model; show retry/reset uncertainty. |
| Candidate ready for review; no active execution | `idle` plus review-ready metadata | Herdr may derive its own unseen-completion display. This is not proof of applied changes. |
| Run failed or stopped; no unresolved approval | `idle` plus failure/stopped metadata | Keep the actionable outcome visible without forging a native permission prompt. |
| Disconnected/stale host binding | Bridge unavailable; no fresh projection | Do not write through stale identifiers or turn missing updates into success. |

Only the bridge source owns MYTHHELM's pane projection. It must not override lifecycle reports for unrelated native-agent panes or globally rewrite Herdr detection/integration settings. Display metadata is not allowed to change the underlying semantic authority.

Sequence/coalesce updates; use short-lived display metadata where supported and re-establish current state after reconnect. Protect against delayed reports, stale “blocked” badges and duplicate notifications. Show core verification/billing truth in the MYTHHELM UI even when the host exposes only a coarser state vocabulary.

#### 16.6.5 Input, approvals, resizing and terminal behaviour

Keep Herdr's configured terminal controls intact and publish the tested keybinding interactions. MYTHHELM must not auto-enter tmux, capture host-level shortcuts, switch input owners on resize or steal focus repeatedly. Test bracketed paste, large multiline prompts, Unicode, alternate-screen restoration, scrollback, mouse selection and plain output inside actual Herdr panes.

A bridge transports typed MYTHHELM intents, not shell-evaluated text. Do not use host screen scraping or synthetic Enter/Y sequences to authorise security or billing actions. If a deliberate native-interactive flow requires raw terminal keys, the user owns that input lease and sees the native prompt; MYTHHELM cannot claim a structured approval bridge that the native surface does not provide.

Any host-driven control request needs an operation ID and run/attempt binding. On timeout, reconcile the operation before retrying. Host `idle`, matching old terminal output or a pane's survival does not establish that a specific task prompt was accepted or completed.

#### 16.6.6 Detach, restore and remote sessions

Closing a display pane or Herdr client normally detaches the managed MYTHHELM workflow. Explicit stop is sent to the MYTHHELM supervisor and confirmed through worker ownership, not merely a host UI transition. If the OS session, machine or terminal server also terminates processes, reconcile and preserve interrupted state; do not claim indefinite survival across every host failure.

Restore a **run attachment**, never replay an original launch instruction blindly. Reconnect to surviving workers, rebuild presentation from the journal, and require supported native resume for interrupted attempts. A host-restored native session and a MYTHHELM-restored worker must not both become active writers.

Remote Herdr presentation is permitted only for a separately identified execution host with its own native installations/authentication and explicit data policy. Do not sync native home directories or credentials merely to make a remote pane work. A host-owned experimental runtime declares its different lifetime rules before launch and cannot inherit the default worker-survival claim.

#### 16.6.7 Distribution and qualification

Provide a free official Herdr bridge and an optional host plugin package where supported. Installation, enabling, hook changes and pane creation require explicit consent and reversible instructions. Do not silently replace native Herdr integrations or install both competing lifecycle reporters.

The bridge can use the core's terminal-host extension contract, but has no model-routing or credential-broker responsibilities. Ship a focused compatibility record: Herdr client/server versions, OS, attachment topology, reporting authority, resize/input/restore tests and limitations. Rich integration must pass Section 18.10; an ordinary successful terminal launch is not enough.

---

## 17. Observability, retention and performance

### 17.1 A receipt for every mission

Every run ends with, or can export, a local receipt containing the requested outcome; admitted source snapshot; execution bundles and native-fidelity differences; runtime surfaces; entitlement/overage evidence; routing decisions; native result states; candidate revision; checks and evidence; approvals; known spending and included-allowance use; uncertainty; external effects; execution-host/Herdr attachment identities; and remaining human action. Never include credential values.

The receipt should answer: **What happened? What changed? Why this route? What was verified? What might it have cost? What remains unsafe or unfinished?**

It must not contain secrets by default or imply that every native internal action was observable. Provenance and observability gaps are useful information, not errors to hide.

### 17.2 Metrics that improve the product

Collect locally, when enabled:

- Time in admission, queue, native execution, blocked approval, repair, integration and review-ready states.
- Observed native usage, auxiliary usage, subscription inclusion and cost-source confidence.
- Candidate acceptance, repair count, baseline/introduced failures and integration conflicts.
- Context package size, handoff count, native context events when exposed and discarded/coalesced progress events.
- Core memory/CPU, event latency, disk use, plugin failures and process recovery outcomes.

Do not collect keystrokes, complete source, full prompts or identifiable provider account details for product analytics by default. There is no central telemetry requirement. Opt-in diagnostic export is separate from local metrics collection.

### 17.3 Efficiency policy

Optimise delivered outcomes, not just tokens. The preferred sequence is: avoid unnecessary agent calls; preserve useful sessions; pass concise referenced context; avoid speculative decomposition; reuse verified artifacts; reduce redundant checks where safe; and parallelise only when independence justifies it.

A router that saves model tokens but adds substantial repair or human review time may be worse. A nominally free subscription call can still consume the allowance needed for a more valuable task. Record these trade-offs explicitly.

The core must not invent titles, summaries or celebratory messages through paid model calls merely to decorate the TUI. Local deterministic formatting is sufficient for routine interface text.

### 17.4 Performance targets, not benchmark claims

Measure these initial targets on a published reference laptop/desktop, with exact OS, terminal, repository fixture and build configuration recorded. Exclude native agent/model processes from core-only measurements and report them separately.

| Target | Initial acceptance objective |
|---|---|
| Warm local status command | p95 under 200 ms on the reference fixture. |
| First useful TUI frame | p95 under 500 ms, without waiting for provider availability checks. |
| Visible critical-event latency | p95 under 150 ms after supervisor ingestion under the normal load fixture. |
| Idle UI/core CPU | Under 1% of one core in the reference idle test, with no active animation or background scan. |
| Core + TUI memory | Under 150 MiB RSS for four displayed lanes and a bounded history fixture; native runtimes excluded. |
| Normal input response | No perceptible sustained input lag under streaming load; instrument p95 under 100 ms. |
| Output burst robustness | A synthetic sustained burst and oversized-frame fixture cannot crash the supervisor or lose an approval/result event silently. |

These numbers are design gates subject to measured adjustment, not claims about Go, Rust or any shipped implementation. A failed target prompts profiling and a documented trade-off, not an automatic renderer rewrite.

### 17.5 Backpressure and disk exhaustion

Separate critical events from high-frequency progress. Apply bounded queues, coalescing and a disk spool with explicit limits. On overload, retain critical state, reduce visual updates and record progress loss/coalescing. Do not claim unlimited lossless logs with finite memory and storage.

If the durable journal cannot be written, stop admitting new work and move active work toward a controlled safe stop where possible. Display that persistence is degraded. A native task already executing may have effects before cancellation; record the resulting uncertainty after recovery.

### 17.6 Retention and cleanup

Proposed defaults: retain routine redacted streaming logs for 14 days, retain mission receipts and reviewable artifacts until the user's configured retention period or explicit cleanup, and keep raw diagnostic capture off. Actual storage limits are configurable and shown during onboarding.

Maintain per-run and total disk accounting. Warn before thresholds, stop admission at a configured reserve watermark, and never delete an active or ownership-ambiguous workspace to free space automatically.

`mythhelm gc --dry-run` explains each eligible object, retained dependency and estimated reclaimed space. Cleanup is idempotent and interruption-safe. Removing a history view does not imply that every provider-side copy or native session has been deleted; disclose those separate retention domains.

---

## 18. Evaluation and acceptance plan

### 18.1 The central product experiment

**Hypothesis:** MYTHHELM can preserve or improve task completion quality while reducing total completion effort or resource cost relative to a developer invoking the same native agents directly.

Evaluate four conditions:

| Condition | Purpose |
|---|---|
| Direct native execution | Baseline, using the same admitted repository/task and comparable native configuration. |
| MYTHHELM with one fixed native profile | Measures orchestration overhead and native-fidelity loss independently of routing. |
| MYTHHELM with routing, one writer | Measures selection and handoff value without parallel integration effects. |
| MYTHHELM with bounded parallelism | Measures whether decomposition and integration provide net value. |

Do not compare a powerful expensive native baseline with a cheaper weaker routed model and call the resulting difference “harness performance”. Record the full execution bundle and all interventions.

### 18.2 Evaluation tasks and scoring

Use a versioned public fixture set and optional private local tasks. Include bug fixing, bounded feature work, test repair, documentation, cross-cutting refactors, UI changes, security-sensitive changes and tasks where a correct answer is to request missing information or decline an unsafe effect.

Use fresh snapshots, frozen acceptance checks, repeated runs and held-out repositories where feasible. Avoid training the router on the same task variants used to advertise success. Document sample sizes, variance and manual judgement criteria.

Primary outcome: accepted delivery under the task's acceptance criteria. Secondary outcomes: total time, known cost, quota consumption where observable, number of repairs, human interventions, review effort and integration failures. Token counts are a diagnostic metric, not the sole success measure.

Measure context leakage and policy violations separately from correctness. One successful unsafe run does not count as a product success. Publishing performance results requires reproducible task definitions and an honest account of excluded failures.

### 18.3 Native fidelity matrix

For each supported adapter/version/platform, verify:

| Feature | Required evidence |
|---|---|
| Model selection | Actual resolved native model/settings recorded, or explicit native-default uncertainty. |
| Repository instructions | A fixture proves intended instruction sources are discovered in managed workspaces. |
| Skills and MCP | Enabled features work in the declared mode; absent or intentionally disabled features are disclosed. |
| Permission mode | Denied operations remain denied; unsupported approval interactions block rather than hang or bypass. |
| Sandbox | Coverage and fallback behaviour are demonstrated on that platform. |
| Resume | Correct session/workspace continuation; incompatible resumes fail safely. |
| Steering | Delivery semantics match the capability declaration. |
| Usage/billing | Units and cumulative/delta accounting are correct; unknowns stay unknown. |
| Native children | Process/concurrency visibility and cancellation limitations are recorded. |
| Exit/failure | Native success, tool failure, protocol failure and final task verification remain distinct. |

These tests establish compatibility, not universal agent quality. Paid live canaries are opt-in maintainer/user runs; the normal test suite uses scripted fixtures and must not spend money.

### 18.4 Fault-injection suite

The deterministic fake adapter must reproduce these cases without a paid model:

| Fault | Required behaviour |
|---|---|
| Crash after launch intent, before launch acknowledgement | Reconcile one actual launch; do not duplicate a writer. |
| Supervisor restarts while worker continues | Reattach or quarantine based on authenticated ownership; no lost approval. |
| Worker dies and PID is reused | Reject stale process identity. |
| Native child ignores cancellation | Escalate through tested lifecycle policy; show unresolved descendants honestly. |
| Limit response with/without reset time | Honour known cooldown; never invent a precise unknown countdown. |
| Cumulative usage repeated | Deduplicate/reconcile without double charging the internal ledger. |
| Permission request in headless mode | Bridge correctly or return blocked; never wait indefinitely. |
| Malformed JSON, excessive nesting, oversized output, invalid encoding | Bound resource use; fail the affected integration safely. |
| Disk full, locked database or interrupted migration | Stop admission, preserve available evidence and produce a recoverable state. |
| Plugin crash, hang, forged approval or incompatible protocol | Isolate failure and reject untrusted control effects. |
| Stale lease while old writer remains alive | No replacement writer or exclusive-resource release until reconciled. |
| Context artifact changes during dependent work | Mark dependency staleness and require scheduler decision. |

### 18.5 Git and security acceptance tests

Test same-file conflicts, disjoint-file semantic conflicts, dependency manifest collisions, generated-file conflicts, changed test validators, untracked outputs, dirty source snapshots, branch movement during apply and failed checks after a clean merge.

Test malicious filenames, terminal escape sequences, symlink/junction traversal, archive extraction attacks, repository-specified plugin activation, secret-bearing stderr, denied network destinations, native sandbox unavailability and plugin attempts to invoke ungranted broker operations.

Where execution uses trusted-host code, tests must verify the warning and trust flow rather than falsely asserting containment. For a sandboxed mode, tests must actually attempt forbidden filesystem/network/process effects and demonstrate enforcement.

### 18.6 UX acceptance tests

A new user must be able to run the offline demo, discover a native profile, understand billing, start a bounded task, identify whether attention is needed, inspect the actual diff, stop/detach deliberately and locate the resulting receipt.

Test this with users who are not familiar with Vim or agent-orchestration vocabulary. Record task completion, confusion, accidental actions and whether users correctly distinguish “running”, “waiting”, “ready for review” and “applied”.

A separate accessibility session covers linear output, keyboard-only navigation, contrast, reduced motion and selected screen-reader/terminal combinations. Do not substitute screenshots for interaction testing.

### 18.7 Release acceptance matrix

| Gate | Pass condition |
|---|---|
| G01 — Free baseline | Install/build, offline demo and contributor tests require no MYTHHELM account, paid credentials or mandatory cloud service. |
| G02 — Native preservation | Each advertised harness/surface has a fidelity record, direct-native comparison and fixture/live evidence; no hidden model-only, SDK-runtime or alternate-harness substitution. |
| G03 — Source protection | Default runs do not modify the original checkout; dirty-state and apply races are handled safely. |
| G04 — Lifecycle | Detach, crash, orphan, cancellation and resume tests pass on every advertised platform. |
| G05 — Billing honesty | Strict subscription-only admits only qualified included allowance with paid-credit/overage prevention; all auxiliary/child routes are covered; unsupported guarantees block admission. |
| G06 — Integration correctness | Frozen outputs are combined and checked at the final revision; agent exit zero is insufficient. |
| G07 — Trust enforcement | Approval binding and declared sandbox boundaries pass adversarial tests; unsupported containment is not advertised. |
| G08 — Mod safety | Declarative mods cannot execute code or hide required controls; executable-plugin trust and version pinning are explicit. |
| G09 — Terminal usability | Supported terminal/shell matrix, plain mode, resize and keyboard flows have recorded results. |
| G10 — Public release | Licence, security policy, contribution path, provenance, changelog and limitations are published. |
| G11 — Herdr | Embedded-terminal behaviour, scoped state projection, input/approval ownership, detach/restore and no-duplicate-launch tests pass for advertised rich integrations. |
| G12 — Alternative surfaces | Any enabled SDK-runtime, native-interactive or host-owned alternative passes its own fidelity, billing, lifecycle and security gates; no unsupported parity claim. |

A capability may remain experimental rather than block unrelated safe functionality. However, a release must not use an experimental label to conceal violations of its fundamental billing, source-protection or approval contracts.

### 18.8 Direct-native versus integrated fidelity qualification

For each advertised surface, compare the same native agent directly and through MYTHHELM with the same version, model, input revision, task, permission envelope and authorised native configuration. Include a standalone terminal and Herdr. Record any unavoidable environment/context difference rather than calling it an identical baseline.

Minimum fixtures cover native instruction and skill discovery, hooks/plugins/MCP, user-selected agent/model, native tools, session continuation, context management where observable, subagents, blocked permissions, workspace path effects and task-relevant browser/editor functionality. Include an intentionally stripped/replaced scaffold variant to prove the registry detects and labels it differently.

For an SDK-runtime candidate, add package/runtime provenance, default configuration comparison, actual entitlement evidence and repeated outcome evaluation. A successful “hello world”, a shared binary hash or identical text on one task is not enough. A thin transport SDK controlling a qualified native executable needs transport tests without being mistaken for a different harness.

Test SDK/CLI feature failures without substituting another model client, and test that a pinned harness cannot be replaced by the same model in OpenCode or another runtime. Comparison results should separate observed feature preservation from hypotheses about performance.

### 18.9 Subscription-only adversarial qualification

Use deterministic fixture tests for admission and explicitly authorised live canaries where provider behaviour must be established. Do not consume purchased credits as a test without specific spend authorisation. Keep financial/account evidence private and publish only sanitised compatibility results.

| Scenario | Required result |
|---|---|
| Native login plus conflicting metered credential/provider configuration | Detect the effective route; block or apply a previewed user-approved child configuration; no silent global mutation. |
| Subscription-associated key versus an ordinary API key | Correct entitlement classification without relying on key shape or exporting the secret. |
| Account login succeeds but plan/model inclusion is unknown | Subscription-only admission blocks with an actionable missing-evidence reason. |
| Included allowance exhausted mid-task | Preserve work; provider-qualified stop/wait; no API, paid credit, overage or implicit harness fallback. |
| Paid wallet exists but auto-reload is disabled | Do not treat disabled purchases as blocked consumption. Qualify consumption prevention or refuse the route. |
| Native small model, title generator, reviewer, subagent or plugin has another funding route | Reject or explicitly remove the incompatible component with a disclosed fidelity delta. |
| Provider/account changes between preparation and launch or between turns | Invalidate evidence and re-admit; never continue on a newly chargeable route. |
| Account usage outside MYTHHELM consumes allowance concurrently | Remain protected by the provider/native stop boundary; local reservations are not enough. |
| Provider status unavailable, cached evidence stale or undocumented billing behaviour | Block strict mode; no guessed inclusion or invented quota. |
| Console/browser login, API credential or local inference is mistaken for a subscription | Correct class/label; no false included-subscription receipt. |
| SDK runtime shares native code but lacks permitted included entitlement | Reject subscription-only SDK admission even when feature tests pass. |
| Agent/mod asks to enable credit purchases, fast chargeable mode or paid router | Trusted explicit billing approval required; instruction text cannot grant it. |

The no-extra-spend gate is about the actual admitted execution scope. Unsandboxed code with ambient access to other paid credentials must be disclosed and, where it defeats the claimed boundary, excluded or contained. A guardrail that observes spend after it occurred is not a zero-extra-spend mechanism.

### 18.10 Herdr acceptance and fault tests

The first-class Herdr release lane must demonstrate all of the following on each claimed client/server/OS topology:

| Test | Required result |
|---|---|
| Ordinary launch, with no host plugin installed | Same useful MYTHHELM workflow in a real pane; no extra mandatory runtime or account. |
| Rich bridge enabled | Accurate scoped state/attention updates and clearly separate display metadata; no false native agent impersonation. |
| Worker is active while UI is idle, slow or detached | Native execution and event draining continue under worker ownership; host status is not the scheduler. |
| Pane close, client disconnect, supervisor restart or host restart | Reattach/reconcile the same attempt; never duplicate work or silently claim a stopped process is still running. |
| Pane move, ID reuse, named sessions and multiple servers | Bind/rebind authoritative identity; no status writes or approvals delivered to another pane/user/run. |
| Both native Herdr integration and MYTHHELM bridge are present | No competing authority on the same pane; unrelated agent panes remain untouched. |
| User requests native view for a headless attempt | Supported same-session attachment or clear limitation; no second writer as a visual workaround. |
| Two clients request interactive input | One explicit input owner, others read-only; automated steering pauses while the user owns input. |
| Late, duplicate or missing lifecycle events | Core journal remains authoritative; snapshot/reconciliation restores display state without replaying prompts. |
| Host wait/input timeout after submission | Reconcile before retry; no duplicate task prompt. |
| Terminal resize, focus, paste, Unicode, shortcuts, alternate-screen and accessibility modes | No broken approvals, stolen host controls, command reinterpretation or lost terminal state. |
| Billing mismatch injected through host environment/configuration | Same fail-closed admission as standalone; Herdr never bypasses the entitlement gate. |
| Host bridge crash or incompatible version | Core work/evidence preserved; explicit degraded host integration and usable standalone attachment. |

Publish results separately for **embedded terminal**, **rich state bridge**, **native interactive attachment** and any experimental **Herdr-owned execution**. Passing one does not establish the others.

---

## 19. GitHub project, licensing and maintenance

### 19.1 Repository structure

Use one public repository initially. Keep the core approachable, with internal implementation details separate from stable extension contracts.

```text
mythhelm/
  cmd/mythhelm/              CLI, TUI and internal worker entry points
  internal/
    admission/              Policy, consent and effective configuration
    routing/                Local ranking and decision explanations
    scheduler/              DAG, reservations and retries
    supervisor/             Run lifecycle and worker reconciliation
    workers/                Native process/transport ownership
    workspace/              Snapshot and isolation management
    integration/            Frozen candidates and verification train
    journal/                SQLite, migrations, projections and outbox
    billing/                Usage reconciliation and reservations
    security/               Trust, path and approval enforcement
    tui/                    Host-rendered components and motion
  adapters/                 Built-in, versioned native integrations
  hosts/herdr/              Official terminal-host bridge and compatibility tests
  protocol/                 Public plugin/control schemas and fixtures
  sdk/                      Small reference plugin SDK
  mods/                     Official themes and declarative workflows
  examples/                 Zero-credential plugin and adapter examples
  tests/                    Fault, platform and contract suites
  evals/                    Tasks, baseline harnesses and result formats
  docs/                     Guides, architecture decisions and limitations
  packaging/                Release manifests and installer definitions
  .github/                  CI, issue templates and release workflows
  README.md
  LICENSE
  NOTICE
  SECURITY.md
  CONTRIBUTING.md
  GOVERNANCE.md
  CODE_OF_CONDUCT.md
```

The names are proposed organisation, not an instruction to create empty boilerplate folders without functioning responsibilities. Split packages as implementation warrants. Keep this master specification as a versioned design reference, while extracting concise contributor/user guides as the software becomes real.

### 19.2 Licence recommendation

Use **Apache-2.0 for the official core, official adapters, SDK, documentation and original official themes**, with third-party materials retaining their own notices. Apache-2.0 is permissive and includes an express patent-licence framework; its terms, not a marketing summary, govern use. [S50]

This supports broad adoption, modification and compatible commercial use. It does not prohibit someone from charging for a fork. “The official project is entirely free” is a project commitment, not a commercial-use prohibition hidden in the licence.

Record dependency and asset licences, audit compatibility and avoid copying implementation from nearby projects merely because they are on GitHub. Extensions may have their own licences, but their distribution and linking/bundling implications need review. Do not call a licence open source while adding incompatible restrictions through a contradictory extra notice.

### 19.3 Governance and contribution

Publish maintainers, decision ownership, release authority and a lightweight proposal process. Architectural changes to security, billing, public protocol and persistence require a short recorded decision and tests. Small fixes should not require a large design document.

Use a Developer Certificate of Origin or another clearly stated contribution policy chosen by maintainers; do not silently demand copyright assignment. Accept AI-assisted contributions based on evidence, provenance and quality, with a responsible contributor attesting that they reviewed the result and have rights to submit it.

Provide good first issues using the fake adapter, UI fixtures, accessibility tests and documentation. Contributors must not need several paid subscriptions to make useful changes.

### 19.4 CI and supply-chain integrity

Run formatting, static checks, unit/contract tests, schema compatibility, migration tests, dependency review and the supported OS build matrix. Use platform-specific tests for process ownership and path behaviour rather than relying only on cross-compilation.

Forked pull-request jobs must not receive release or provider credentials. Pin privileged CI dependencies/actions, minimise token permissions and separate untrusted build/test steps from signing/publishing authority.

Release archives include checksums, version/build metadata, a software bill of materials where practical, licence notices and provenance. GitHub documents artifact attestations suitable for establishing build provenance; verification still requires understanding the asserted builder/workflow identity. [S51]

Aim for reproducible builds and publish the reproducibility procedure. Do not claim bit-for-bit reproducibility until it is demonstrated for each relevant artifact.

### 19.5 Updates, security reports and compatibility

Use semantic versions for the public protocol and user-facing configuration. Internal database migrations have their own monotonic versioning and recovery tests. Support a stated compatibility window and explicit deprecation periods for public interfaces.

`SECURITY.md` defines a private reporting route, supported releases and how advisories are published. Avoid promising a response-time service level the volunteer project cannot maintain. Security-relevant adapter changes receive expedited releases with clear affected-version notes.

No required auto-update daemon. Users may opt into update notifications; installation still requires an explicit action. Existing native agent binaries are not bundled, modified or redistributed without reviewing their applicable terms.

---

## 20. Delivery roadmap: ambitious, gated and shippable

### 20.1 Replace calendar certainty with complete vertical slices

The original 4–6 week MVP window is a planning aspiration, not an evidence-backed delivery promise. A useful local orchestration slice may be built rapidly; a polished cross-platform multi-agent platform with robust plugin isolation, accurate billing and crash recovery has more independent verification work. [U1] (lines 268–273)

Keep the ambition, but deliver complete loops and advertise only what has passed its gate. Do not postpone all visual quality until after the architecture, and do not postpone recovery until after users have long-running paid jobs.

### 20.2 Stage 0 — Prove the uncertain boundaries

**Deliver:** a tiny native-integration and process-ownership spike, not a large application skeleton.

Exercise Claude Code's native headless path, Codex app-server/exec and the native Muse, OpenCode, Kimi, Cursor and Antigravity candidate surfaces against disposable fixtures, prioritising a complete qualified route over seven shallow demos. Record runtime/default-feature fidelity, effective entitlement, overage prevention, permission differences, cancellation and configuration inheritance. Native sign-in remains native; no token copying.

Prove ordinary launch inside Herdr and the bridge's scoped reporting/restore contract. Antigravity's native structured CLI and Kimi's approval-capable native route get explicit spikes. A candidate that only works through an API-funded SDK remains blocked for the default included-allowance workflow.

Also prove a worker can survive a TUI exit, reconnect after a supervisor restart and stop its owned native process tree on Windows and a Unix platform. Build a small TUI fixture demonstrating resize, Unicode, plain output and event-driven motion.

**Exit gate:** choose the initial adapter surface from measured compatibility, resolve authentication constraints, and remove any capability claim the spike cannot justify. This work answers specific risks rather than becoming an indefinite research phase.

### 20.3 Stage 1 — A complete single-agent product

**Deliver:** installable binary; offline demo; read-only doctor; one qualified native executable adapter with included-allowance/overage evidence; one writer; managed snapshot; local routing; durable run/worker ownership; candidate diff; configured verification; receipt; deliberate stop/detach/recovery; polished focused TUI; plain/JSONL mode; one declarative theme format; and the first-class single-pane Herdr experience with qualified scoped status bridging.

Use the best-qualified initial native adapter from Stage 0. Keep interfaces ready for additional adapters, but do not delay a useful release merely because a third vendor's rich control mode is unfinished.

**Exit gate:** G01, G03–G07, G09 and G11 for advertised combinations; G02 for the shipped native surface; G10 for the public preview; G12 for any enabled alternative surface. The normal “task → native work → checks → reviewable artifact” workflow works without manual internal repair.

### 20.4 Stage 2 — Multi-harness public alpha

**Deliver:** at least two independently qualified native adapters with included-only routes; authorised profiles; deterministic selection; dependency-aware task DAG; two-writer mode; message/handoff semantics; resource reservations; serialized integration train; explicit quota/cost uncertainty; public plugin and host-bridge protocols; trusted executable-plugin flow; declarative workflow/keymap/layout mods; optional Herdr lane inspectors and qualified native attachment.

Expand across the seven named harness targets as each meets the same fidelity, entitlement, security and lifecycle gates. Native protocol/CLI support takes precedence over SDK-runtime alternatives. A partially qualified adapter is visibly experimental; unqualified subscription-only routes remain blocked rather than silently chargeable.

**Exit gate:** parallel and cross-provider tasks outperform or meaningfully simplify representative workflows without losing source safety, recovery or billing honesty. G08 joins the core gates.

### 20.5 Stage 3 — Version 1.0

**Deliver:** the published host/adapter/terminal support matrix; stable configuration and plugin protocol; accessible linear interface; robust native version management; clear migration/update path; optional GitHub publisher; local evaluation and routing history; storage management; documented limitations and contributor governance.

**Exit gate:** all applicable G01–G12 requirements pass, including Herdr and per-surface entitlement evidence; current provider compliance review is recorded; and the product's positioning has survived user testing. No broad superiority claim is required to release a useful tool.

### 20.6 Later extensions, justified by evidence

Possible later work includes measured local learning, explicitly funded remote smart routing, separately qualified SDK-runtime alternatives, richer native controls, ACP expansion, sandboxed computational plugins, advanced UI mods, further development-environment integrations and explicitly designed remote execution. Herdr baseline integration and strict included-allowance qualification are not deferred to this list.

Team-shared state, cloud offload, automatic production deployment, distributed credentials and multi-tenant hosting require separate threat models and operational ownership. Voice interfaces, a marketplace and a custom renderer are not prerequisites for the central product to succeed.

### 20.7 Scope cuts that protect delivery

Do not build all package-manager channels, every native agent, arbitrary plugin sandboxing, a trained router, a cloud backend and all terminal combinations simultaneously. Preserve the extension seams and acceptance contracts, but ship a supported subset honestly.

Do not cut native-harness fidelity, included-allowance/overage admission, correct cancellation, source protection, permission clarity, honest cost uncertainty, a reviewable artifact, a usable TUI or first-class Herdr behaviour for the advertised topology. Those are the product, not optional polish.

---

## 21. Risks, unresolved questions and decision ownership

| Risk or question | Current decision | Evidence/owner required before expanding claims |
|---|---|---|
| Native integration terms change | Keep native auth and unmodified runtimes; no account-sharing proxy. | Release maintainer records current terms and surface-specific review. |
| SDK/native CLI diverge | Native executable first; alternatives have independent evidence. | Adapter owner compares scaffolding, permissions, features, outcomes and entitlement. |
| Subscription login can consume extra credits | Strict included-only admission requires actual consumption prevention. | Billing owner qualifies account/model/mode, including existing balances and concurrent usage. |
| Credential format misclassifies funding | Product/endpoint/provenance determines entitlement. | Native billing evidence; no token inspection or API-key-only heuristic. |
| Herdr restores a duplicate writer | Restore attachments, not original execution commands. | Host/lifecycle owner passes pane move/reconnect/restart and double-launch tests. |
| Host display impersonates a native agent | Separate MYTHHELM aggregate state, native views and historical transcripts. | Bridge owner verifies real registration/state authority; no forged agent-kind label. |
| Two same-vendor profiles share credentials/limits | Treat them as potentially coupled until tested. | Adapter maintainer tests native configuration/keychain and bucket semantics. |
| Native event schemas drift | Pinned compatibility manifests and conservative negotiation. | Adapter contract fixtures and opt-in live canaries. |
| Strict sandbox unavailable on an OS | Refuse that profile; offer an explicitly different supported posture. | Platform/security owner demonstrates actual boundaries. |
| Hard spend cap unavailable | Provide labelled soft controls; reject `--require-hard-limit`. | Billing/adapter owner proves enforcement and in-flight bound. |
| Native children evade top-level limits | Show top-level versus total concurrency; restrict strict modes. | Adapter/process owner establishes descendant control. |
| Parallelism increases repair work | Default one writer; compare against direct and single-agent baselines. | Evaluation owner publishes total outcome/effort measurements. |
| A model changes tests to self-certify | Frozen baseline checks and reviewed validator changes. | Verification owner supplies adversarial fixtures. |
| Plugin permissions look safer than they are | Explicit trusted-host label or verified sandbox. | Security owner reviews execution mode, not just manifest. |
| Terminal features break accessibility | Plain/screen-reader route remains first-class. | UX/platform owner runs actual assistive-technology tests. |
| Storage grows without bound | Bounded logs, accounting, watermarks and previewed cleanup. | Persistence owner passes retention/disk-full tests. |
| Managed snapshots are slow on huge repos | Correct independent snapshots first; measured cache optimisation later. | Workspace owner benchmarks representative repositories. |
| Name collides with an existing project/mark | MYTHHELM is provisional until publication checks. | Maintainer checks namespace, search and relevant legal conflicts. |
| Maintainer workload exceeds capacity | Small certified surface, modular adapters, free deterministic tests. | Governance documents supported versions and ownership. |
| Users prefer direct native agents | Treat added-layer value as a falsifiable product hypothesis. | User testing shows reduced friction or measurable outcome benefit. |

Open questions are not permission to omit implementation. Each item has a conservative default and a gate for changing it. Unknown native details stay out of marketing claims until resolved.

---

## 22. Traceability and implementation handoff

### 22.1 Original requirement coverage

| Original requirement | Final disposition |
|---|---|
| Preserve proprietary/native harnesses | Retained and made precise: Sections 1, 6 and 9. |
| Intelligent low-cost routing | Retained with local rules first and optional measured smart routing: Section 8. |
| Multiple existing subscriptions | Retained as authorised profiles; strict included-allowance, no paid-overage/credit fallback: Section 13. |
| Use inside Herdr | First-class host bridge, single input/process ownership and restore qualification: Sections 5.7, 6.6, 7.8, 15.11, 16.6 and 18.10. |
| Preserve native harness rather than merely model access | Native executable first; SDK/CLI separation and configuration/outcome tests: Sections 1.2, 6.1, 9.9 and 18.8. |
| Claude Code, Codex, OpenCode, Muse, Kimi, Cursor and Antigravity | Explicit per-harness candidate surfaces, limitations and entitlement gates: Sections 1.3, 9 and 13.9. |
| Parallel native agents | Retained with dependency contracts, bounded concurrency and integration evidence: Sections 8 and 11. |
| Direct messages and blackboard | Retained with real delivery/provenance semantics: Section 10. |
| No collisions | Replaced absolute guarantee with concrete prevention, detection, isolation and integration contracts: Section 11. |
| Windows, macOS and Linux | Retained, with separate host/adapter/sandbox/terminal qualification: Section 16. |
| Beautiful joyful TUI | Expanded into visual, interaction, motion, accessibility and responsive specifications: Section 15. |
| Shell and terminal breadth | Explicit target/test matrix and fallbacks: Section 16. |
| Praise/complaint/TUI research | Rebuilt with traceable evidence and research limitations: Section 1. |
| 2–3 viable architecture options | Three native-preserving alternatives compared: Section 6. |
| Lifecycle, security and cost | Substantially expanded: Sections 7, 12 and 13. |
| MVP versus later releases | Replaced speculative certainty with complete gated slices: Section 20. |
| Public GitHub and entirely free | Added explicit product commitments and release/governance model: Sections 3 and 19. |
| Plugin/mod architecture | Added first-class typed extension platform and useful declarative mods: Section 14. |
| New epic name | MYTHHELM, with practical CLI identity and clearance gate: Section 3. |
| Final complete Markdown artifact | This self-contained document, including audit, design, acceptance gates and references. |

### 22.2 Initial implementation backlog

The first development agent should read this specification and create a small dependency-ordered backlog, then execute Stage 0 and Stage 1 rather than scaffolding every future module.

1. **Establish evidence fixtures:** native capability/fidelity/entitlement probes, a scripted fake adapter, per-harness compatibility records and Herdr bridge fixtures. Do not start with an SDK-only architecture.
2. **Prove ownership:** supervisor, per-attempt worker, durable launch identity, cancellation and recovery on Windows and Unix.
3. **Protect the source:** explicit snapshot, one managed writer, frozen artifact capture and guarded apply.
4. **Close the delivery loop:** native execution, independent configured checks, review candidate and receipt.
5. **Make it usable:** focused TUI standalone and inside Herdr, scoped host status and reattachment, plain/JSONL output, included-allowance/overage labels and the offline demo.
6. **Open the seams:** minimal adapter protocol, declarative theme format, conformance tests and contributor documentation.

Every implementation task must name its acceptance evidence and affected invariant. Do not mark a task complete because files exist, a mock test passed or the agent reported success. Tests must exercise the final packaged build wherever the claim depends on packaging or process behaviour.

### 22.3 Required implementation outputs

A subsequent implementation effort should deliver actual source, tests, build instructions, release artifacts for claimed platforms, a compatibility matrix, a limitations register and verification evidence. The source tree must build without private infrastructure, and routine tests must run without paid credentials.

A feature not implemented remains clearly unimplemented. A feature implemented but not runtime-qualified remains unverified. A provider-documented capability is not automatically a tested MYTHHELM feature.

### 22.4 Final architectural decision

Build **MYTHHELM** as a free, open-source local control plane around the user's native coding agents, usable standalone and first-class inside Herdr. Use a Go supervisor with durable workers, **unmodified native-executable-first adapters**, managed workspaces, a serialized verification/integration path, strict included-allowance admission and a polished Bubble Tea TUI.

The native tool keeps its own agent loop, default machinery, approved configuration, conversation and native authentication. MYTHHELM supplies mission coordination and evidence—not a replacement model/tool loop. Herdr supplies the terminal host—not a second billing or execution authority. SDK-runtime alternatives must earn admission through independent fidelity and subscription-entitlement proof; no CLI or SDK label automatically supplies either.

Make the first experience excellent with one agent; make parallelism earn its complexity; make extensions powerful without disguising trust; and make every success claim point to a real artifact and evidence.

**Many agents. One mission. The helm stays in the user's hands.**

---

## Source register

### Source material

<a id="source-material"></a>

**[U1] Supplied conversation:** `Pasted text(6).txt`, 282 source lines as supplied in the conversation. Original task: lines 1–44. Other model's response: lines 47–282. The new GitHub/free/plugin requirements were supplied directly by the user alongside that conversation. File SHA-256: `f8ec719fcdd106ade80e8b0dc41af9c9e870cfc81ca1385dfc47c1c166609e95`.

<a id="follow-up-requirements"></a>

**[U2] Follow-up requirements supplied in this conversation:** first-class operation inside Herdr; execution against users' native included subscription allowances instead of separately billed inference; target harnesses Claude Code, Codex, OpenCode, Meta Muse, Kimi Code, Cursor agents and Antigravity; and clarification that the native agent's harness must not be replaced for SDK convenience. Revision 1.1 integrates those instructions throughout this full document.

**Revision provenance:** based on the complete `MYTHHELM_Master_Spec.md` Revision 1.0 supplied in this conversation. Original SHA-256: `9c2fd14631d5b07be0d66bc902a0a258fd484e390adf4586391951303909c3a6`. No provider integration or Herdr lifecycle was executed as part of this document edit.

The user-supplied conversation is the source for statements about what the original brief or other model said. External sources below support factual corrections and feasibility observations. MYTHHELM's proposed architecture, policies, interfaces and targets are new design work, not claims that those sources already implement them.

### Primary references

Research date: **29 September 2026**. Revision 1.1 specifically rechecked the native/SDK authentication, entitlement and Herdr issues covered by [S05], [S08], [S09], [S13], [S14], [S21], [S46] and the new references [S52]–[S75]. Other references and unrelated design sections are retained from Revision 1.0; they were not all freshly re-audited during this update.

Documentation and repository default branches can change. Before implementation, record exact native/dependency/Herdr versions and archive relevant compatibility evidence. Conflicting or account-specific documentation is a qualification issue, not permission to infer support. Issue reports remain individual reports, not prevalence measurements. Redirecting documentation links refer to the official documentation reached during research.

| Ref | Source and relevance |
|---|---|
| [S01] | Anthropic, effective harnesses for long-running agents — harness design as an engineering variable. |
| [S02] | Anthropic, demystifying evaluations for AI agents — outcome/evaluation design. |
| [S03] | OpenAI, harness engineering — agent-oriented engineering workflow context. |
| [S04] | Claude Code, headless/programmatic execution — native non-interactive entry points. |
| [S05] | Claude Agent SDK overview — native runtime integration versus a raw API client. |
| [S06] | Claude Code CLI reference — documented flags and mode-specific controls. |
| [S07] | Codex non-interactive mode — `exec` and structured output. |
| [S08] | Codex app-server — native structured control interface. |
| [S09] | Codex authentication — native sign-in, API and credential-storage distinctions. |
| [S10] | Codex Windows documentation — native Windows execution/sandbox context. |
| [S11] | Muse Code documentation — native coding-harness identity and platform context. |
| [S12] | Muse Code extending/headless documentation — execution, session and child-agent behaviour. |
| [S13] | Muse Code subscriptions — CLI subscription versus API billing context. |
| [S14] | Muse Code authentication — native identity configuration. |
| [S15] | Agent Client Protocol introduction — standardised agent/client control. |
| [S16] | Aider repository map documentation — repository context/navigation. |
| [S17] | Aider Git integration — review/history workflow. |
| [S18] | OpenCode server documentation — separate server/control surface. |
| [S19] | Continue original repository — configurable open-source agent workflow. |
| [S20] | Cline checkpoints documentation — recovery UX pattern. |
| [S21] | Cursor headless CLI documentation — native automation surface. |
| [S22] | Lazygit original repository — terminal navigation/review patterns. |
| [S23] | k9s command documentation — contextual commands and filtering. |
| [S24] | Cline issue 3790 — historical user-reported checkpoint disk usage; not a current universal claim. |
| [S25] | OpenCode issue 4579 — user-reported auxiliary-model billing surprise. |
| [S26] | OpenCode issue 47485 — reported headless compaction behaviour; not independently reproduced here. |
| [S27] | Claude Code issue 24594 — streaming integration/documentation report. |
| [S28] | Claude Code issue 30538 — reported configuration-root difference across integration surfaces. |
| [S29] | Bubble Tea issue 1019 — historical Windows rendering report, not a benchmark of current releases. |
| [S30] | Claude Squad original repository — adjacent multi-agent/worktree product. |
| [S31] | Superset original repository — adjacent agent workspace product. |
| [S32] | Agent Orchestrator original repository — adjacent orchestration product. |
| [S33] | Bubble Tea original repository — current framework/renderer architecture. |
| [S34] | Ratatui official documentation — Rust TUI alternative. |
| [S35] | Ink original repository — React-based interactive CLI framework. |
| [S36] | Textual official documentation — Python TUI alternative. |
| [S37] | SQLite WAL documentation — local persistence constraints. |
| [S38] | Go `plugin` package documentation — in-process plugin portability/compatibility considerations. |
| [S39] | Go `os/exec` documentation — native argument/process semantics. |
| [S40] | Microsoft Job Objects documentation — Windows process ownership mechanisms. |
| [S41] | Git worktree documentation — linked worktree/shared repository semantics. |
| [S42] | TypeSafe/Jev documentation — optional structured decision service. |
| [S43] | RouteLLM original repository — routing research/framework reference. |
| [S44] | Claude Code environment-variable reference — documented configuration-root surface. |
| [S45] | Claude Code sandboxing documentation — platform and enforcement scope. |
| [S46] | Claude Code legal/compliance documentation — native integration and authentication conditions. |
| [S47] | OpenAI Europe Terms of Use — regional usage/account restrictions; applicable terms must be checked for the user and service. |
| [S48] | HashiCorp go-plugin original repository — process-based plugin architecture precedent. |
| [S49] | NO_COLOR specification — colour suppression convention. |
| [S50] | Apache License 2.0 — authoritative licence text. |
| [S51] | GitHub artifact attestation documentation — build-provenance mechanism. |
| [S52] | Herdr agents — real terminal panes, foreground detection, state authority and wrapper/manifest limitations. |
| [S53] | Herdr agent automation — layout/pane/agent distinction, input submission and lifecycle-wait limitations. |
| [S54] | Herdr socket API — scoped reporting, session references, display metadata, socket/session resolution and event subscriptions. |
| [S55] | Antigravity CLI overview — native CLI and shared agent-core description. |
| [S56] | Antigravity headless mode — structured native process, persistent prompts, unsupported controls and soft-denied permissions. |
| [S57] | Antigravity CLI installation/authentication — account/keyring and explicit Gemini API-key provider routes. |
| [S58] | Antigravity SDK overview — genuine agent runtime; documented Gemini API-key and Google Cloud setup. |
| [S59] | Antigravity SDK personas — custom system-instruction mode can bypass default scaffolding/environment context. |
| [S60] | Antigravity plans — baseline allowance and AI Credit Overages settings; general page has scope differences from CLI auth. |
| [S61] | Claude Code authentication — credential precedence, subscription versus Console/cloud routes and configuration-root behaviour. |
| [S62] | Claude usage credits — paid continuation beyond included allowance. |
| [S63] | OpenAI flexible-usage credits — purchased credits, consumption after allowance and automatic reload. |
| [S64] | Codex with ChatGPT plans — plan/workspace distinctions, allowance and enterprise billing differences. |
| [S65] | OpenCode providers — provider-specific credentials/configuration and entitlement distinction. |
| [S66] | OpenCode Go — native provider subscription offering; not universal coverage for all OpenCode providers. |
| [S67] | OpenCode SDK — typed client interface to its native server. |
| [S68] | Kimi Code overview — membership service and subscription-key access. |
| [S69] | Kimi Code membership — shared allowance and optional Extra Usage continuation. |
| [S70] | Kimi CLI command reference — native print mode, automatic permissions and skill-directory replacement semantics. |
| [S71] | Kimi native ACP reference — native executable control mode. |
| [S72] | Kimi local server API — experimental local REST/WebSocket contracts and authenticated status surfaces. |
| [S73] | Cursor CLI authentication — native browser and Cursor-issued key routes, status/account/endpoint information. |
| [S74] | Cursor CLI overview — native interactive/headless/session surfaces and separate cloud handoff. |
| [S75] | Cursor models and pricing — included usage pools and paid additional usage. |

[U1]: #source-material
[U2]: #follow-up-requirements
[S01]: https://www.anthropic.com/engineering/effective-harnesses-for-long-running-agents
[S02]: https://www.anthropic.com/engineering/demystifying-evals-for-ai-agents
[S03]: https://openai.com/index/harness-engineering/
[S04]: https://code.claude.com/docs/en/headless
[S05]: https://code.claude.com/docs/en/agent-sdk/overview
[S06]: https://code.claude.com/docs/en/cli-reference
[S07]: https://developers.openai.com/codex/noninteractive/
[S08]: https://developers.openai.com/codex/app-server/
[S09]: https://developers.openai.com/codex/auth/
[S10]: https://developers.openai.com/codex/windows/
[S11]: https://dev.meta.ai/docs/muse-code
[S12]: https://dev.meta.ai/docs/muse-code/extending
[S13]: https://dev.meta.ai/docs/muse-code/subscriptions
[S14]: https://dev.meta.ai/docs/muse-code/auth
[S15]: https://agentclientprotocol.com/get-started/introduction
[S16]: https://aider.chat/docs/repomap.html
[S17]: https://aider.chat/docs/git.html
[S18]: https://opencode.ai/docs/server/
[S19]: https://github.com/continuedev/continue
[S20]: https://docs.cline.bot/core-workflows/checkpoints
[S21]: https://cursor.com/docs/cli/headless
[S22]: https://github.com/jesseduffield/lazygit
[S23]: https://k9scli.io/topics/commands/
[S24]: https://github.com/cline/cline/issues/3790
[S25]: https://github.com/anomalyco/opencode/issues/4579
[S26]: https://github.com/anomalyco/opencode/issues/47485
[S27]: https://github.com/anthropics/claude-code/issues/24594
[S28]: https://github.com/anthropics/claude-code/issues/30538
[S29]: https://github.com/charmbracelet/bubbletea/issues/1019
[S30]: https://github.com/smtg-ai/claude-squad
[S31]: https://github.com/superset-sh/superset
[S32]: https://github.com/Untrivial-ai/agent-orchestrator
[S33]: https://github.com/charmbracelet/bubbletea
[S34]: https://ratatui.rs/
[S35]: https://github.com/vadimdemedes/ink
[S36]: https://textual.textualize.io/
[S37]: https://sqlite.org/wal.html
[S38]: https://pkg.go.dev/plugin
[S39]: https://pkg.go.dev/os/exec
[S40]: https://learn.microsoft.com/en-us/windows/win32/procthread/job-objects
[S41]: https://git-scm.com/docs/git-worktree
[S42]: https://docs.typesafe.ai/introduction
[S43]: https://github.com/lm-sys/RouteLLM
[S44]: https://code.claude.com/docs/en/env-vars
[S45]: https://code.claude.com/docs/en/sandboxing
[S46]: https://code.claude.com/docs/en/legal-and-compliance
[S47]: https://openai.com/policies/eu-terms-of-use/
[S48]: https://github.com/hashicorp/go-plugin
[S49]: https://no-color.org/
[S50]: https://www.apache.org/licenses/LICENSE-2.0
[S51]: https://docs.github.com/en/actions/how-tos/secure-your-work/use-artifact-attestations/use-artifact-attestations

[S52]: https://herdr.dev/docs/agents/
[S53]: https://herdr.dev/docs/agent-automation/
[S54]: https://herdr.dev/docs/socket-api/
[S55]: https://antigravity.google/docs/cli/overview/
[S56]: https://antigravity.google/docs/cli/headless/
[S57]: https://antigravity.google/docs/cli/install/
[S58]: https://antigravity.google/docs/sdk/overview/
[S59]: https://antigravity.google/docs/sdk/personas/
[S60]: https://antigravity.google/docs/plans
[S61]: https://code.claude.com/docs/en/authentication
[S62]: https://support.claude.com/en/articles/12429409-manage-usage-credits-for-paid-claude-plans
[S63]: https://help.openai.com/en/articles/12642688-using-credits-for-flexible-usage-in-chatgpt-personal-plans
[S64]: https://help.openai.com/en/articles/11369540-using-codex-with-your-chatgpt-plan
[S65]: https://opencode.ai/docs/providers/
[S66]: https://opencode.ai/docs/go/
[S67]: https://opencode.ai/docs/sdk/
[S68]: https://www.kimi.com/code/docs/en/
[S69]: https://www.kimi.com/code/docs/en/kimi-code/membership.html
[S70]: https://www.kimi.com/code/docs/en/kimi-code-cli/reference/kimi-command.html
[S71]: https://www.kimi.com/code/docs/en/kimi-code-cli/reference/kimi-acp.html
[S72]: https://www.kimi.com/code/docs/en/kimi-code-cli/reference/server-api.html
[S73]: https://cursor.com/docs/cli/reference/authentication
[S74]: https://cursor.com/docs/cli/overview
[S75]: https://cursor.com/docs/models-and-pricing
