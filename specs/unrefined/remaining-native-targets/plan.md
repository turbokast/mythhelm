## Remaining native harness targets — Master Plan

> Unrefined requirements epic. Refine each child before design/tasks. This captures MH-29, not implemented adapters or passing qualification. Normative source: Master Specification v2 §§7,14,17–18, explicit UR-05/UR-06 and implementation-plan W14. Independent target/surface/platform evidence is mandatory; no requirement here authorizes inference, login, installation or account changes.

### Context and Problem Summary

- **Backlog card**: MH-29
- **Issue**: [#135](https://github.com/turbokast/mythhelm/issues/135).
- **Grounding revision**: `615e74a6bb6907f87bd2065e2f576f5e18b5dec5` on actual `origin/main`.

The seven-target commitment requires owned qualification tracks for OpenCode, Meta Muse Code, Kimi Code CLI, Cursor Agent CLI and Antigravity alongside the existing Claude Code/MH-12 and Codex/MH-4 work. A working parser, model available through another harness, scripted Windows test or registry seed is not qualification of the native target. Each exact supported execution bundle needs independently versioned fidelity, entitlement and security/lifecycle evidence; blocked targets remain visible with a bounded next test and required authority.

This is an epic because target executables, native protocols, account/funding rules and platform interfaces can advance independently. Five child requirements share the existing qualification contract and do not depend on one another. Multiple surfaces or OS/architecture routes within a child must be independently enumerated and split again before implementation if they exceed one bounded reviewed task envelope. S2 is a continuing coverage track, not a deadline claiming all five targets ship together.

### Current State and Premise Grounding

| Claim | Verdict | Source and consequence |
|---|---|---|
| Only the historical internal claudecode/fake adapter layout exists. | PARTIAL | Historical adoption context has drifted. Recorded complete `git ls-tree -r --name-only origin/main adapters` enumeration at the grounding revision yields directory set `claudecode, codex, fake`; no other adapters directory appears. `internal/adapter/adapter.go:17` (Adapter) and `:149` (Launcher) own the shared interfaces. Preserve existing adapters rather than repeat their completed work. |
| Five remaining native targets are already represented as product commitments. | HOLDS | `internal/qualify/seed.go:50` (all seven seedHarnesses through line85) and `:93` (SeedV1 through line121), seeds all seven names with unknown column evidence and bounded next-test descriptions; these planned rows are not live qualification. |
| A complete key and independent evidence columns already have a canonical producer. | HOLDS | `internal/qualify/qualify.go:57` (Key), `:75` (Evidence), `:99` (Column), `:105` (Record) and `:122` (Capability), includes surface/build/protocol/OS/arch/provider/model/effort/auth/config/trust/workspace/entitlement identity, independent Fidelity/Entitlement/Lifecycle, provenance and expiry. `internal/qualify/registry.go:203` (Record) plus `internal/journal/migrations/0002_qualification.sql:4` provide durable revision storage. Consume MH-10 rather than create another support database. |
| Windows has no process-tree ownership implementation. | PARTIAL | The older Windows requirements are historical grounding. Current `internal/workers/proc_windows.go:24` (nativeAttr), `:32` (Signal) and `:47` (GroupGone), creates the native suspended and uses a Job Object; `docs/decisions/0017-windows-process-ownership.md` records the design. This is implementation/lifecycle evidence, not native-target qualification. |
| Windows lifecycle tests establish general native support. | PARTIAL | `docs/limitations.md:228` (Windows lifecycle evidence), distinguishes actual amd64/arm64 scripted evidence and blocked console cases. `internal/processsupport/SUPPORT.md:3` explicitly states no pair qualified and `:16` begins an empty promotion table and says untested pairs remain unknown. `specs/in-progress/windows-process-ownership/tasks.md:304` (Task11), owns promotion. Preserve that boundary; no target or architecture inherits a scripted result. |
| Current official native surfaces/funding and complete G02/G05/G10/G12 evidence are available for all five targets. | UNVERIFIABLE | No native/provider probes or account access were performed. Master §7.2 lists candidate directions, not a certified current support matrix. Each child must gather dated primary documentation and exact authorized observation before promoting a route. |
| The issue’s PR126-pending wording describes current adoption. | PARTIAL | Historical issue text is preserved; `docs/spec/README.md:5` identifies v2 as adopted. Design adoption provides no runtime, allowance or qualification evidence. |

### Work Streams

| # | Spec | Scope | Domain | Dependencies |
|---|---|---|---|---|
| 1 | `opencode-native-qualification` | Exact OpenCode surface/platform discovery, evidence and fail-closed native integration requirements. | adapters | Existing MH-10 qualification registry/contracts; applicable lifecycle/authority producers. |
| 2 | `muse-native-qualification` | Exact Meta Muse Code surface/platform discovery, evidence and fail-closed native integration requirements. | adapters | Existing MH-10 qualification registry/contracts; applicable lifecycle/authority producers. |
| 3 | `kimi-native-qualification` | Exact Kimi Code CLI surface/platform discovery, evidence and fail-closed native integration requirements. | adapters | Existing MH-10 qualification registry/contracts; applicable lifecycle/authority producers. |
| 4 | `cursor-agent-native-qualification` | Exact Cursor Agent CLI surface/platform discovery, evidence and fail-closed native integration requirements. | adapters | Existing MH-10 qualification registry/contracts; applicable lifecycle/authority producers. |
| 5 | `antigravity-native-qualification` | Exact Antigravity surface/platform discovery, evidence and fail-closed native integration requirements. | adapters | Existing MH-10 qualification registry/contracts; applicable lifecycle/authority producers. |

### Dependency Graph

MH-10 qualification registry and applicable canonical admission/worker/process/acceptance contracts → each of the five children independently. MH-11 supplies Windows process-support facts, MH-3 supplies Herdr host evidence and MH-7 owns public release matrices. A row requires the evidence for the exact target/surface/OS/architecture/host combination it advertises. Native-session continuity/MH-15 consumes operation-qualified session evidence; it is not proof the target already resumes. No child depends on qualification of another target; shared adapter interfaces remain existing producer inputs or separately reviewed domain-pure changes.

### Implementation Order

1. Begin each child’s design with a bounded discovery contract, even while the initial surface/platform is unknown. The first design outcome is either an evidenced exact candidate inventory or an explicit blocked inventory naming missing facts, safe next tests and authority required. Use only authorized read-only primary-document/source observations within the fixed discovery bounds; no installation, login, secret access or inference is implicit. A blocked inventory is an honest usable discovery deliverable, not an implemented or qualified adapter.
2. After discovery identifies an evidenced candidate, freeze its smallest useful exact local surface and credential-free adapter/protocol contracts, including command/schema/config/error/ownership and platform/surface task splits. Adapter implementation tasks and live promotion remain blocked until their required real facts and producer contracts exist. Discovery-first design does not guess a build, endpoint, account, surface or platform to bypass those gates.
3. Only after applicable explicit live-test authority, finite allowance/resource envelope and disposable fixtures exist, run the exact native fidelity/funding/lifecycle tests. Never buy credits, exhaust allowance, alter account settings or hide paid auxiliary use to obtain a pass.
4. Independently accept each evidence bundle and update versioned scoped support rows through the normal PR process. Preserve blocked targets and completed Claude/Codex records. Public release readiness requires the applicable whole journey and MH-7 gate evidence, not five source implementations or one platform job.

### Shared Qualification Contract

Every child requirements file carries FR-1–FR-4 below. Each child binds the full v2 §7.1 key; verdicts supported/unsupported/unknown and progress planned/documented-candidate/fixture-tested/live-qualified/experimental/blocked/unsupported are separate dimensions. Independent columns cannot promote each other. Expected gates are G02/G05/G10/G12 as applicable, with G04 lifecycle and relevant G07 trust/AT cases; planned gates are never recorded as passed.

Discovery/read-only probes: at most8 executable candidates and32 non-secret configuration sources per bundle; five seconds per local probe,30 seconds total; at most1 MiB captured output per operation/64 nesting and16 MiB total sanitized evidence. Timeouts, opaque configuration or missing mandatory evidence stay unknown/blocked; no unbounded directory/home scan or silent truncation. Do not copy raw secrets/native history; inventory source identity/precedence and redacted effective configuration only through qualified safe observations.

Credential-free qualification fixtures: at most32 cases per target/surface/platform suite, five minutes total,60 seconds per case and at most4 local child processes per fixture; request cancellation within1 second and complete reconciliation within a separate30-second safety envelope or report cancel_incomplete/ownership_unresolved. A target needing a larger native child tree is unsupported in this initial envelope until a separately reviewed extension. Parent and model-using descendants/auxiliaries share the admitted funding/resource tree.

Any later live qualification is separately authorized for at most one exact bundle and two native episodes per qualification attempt, ten minutes total execution with a separate30-second safety-reconciliation ceiling; use the smaller remaining grant/native/platform bounds. At most one transport retry per idempotent observation; never automatically retry an uncertain launch/input/tool/external effect. A live procedure must predeclare and prove its billing/auxiliary exposure boundary or remain blocked before inference; these numerical execution caps do not establish a provider spending cap. Repetition to erase an unfavorable result is forbidden. Existing evidence survives failure. Bounds are conservative requirements and may be lowered; increasing them is a reviewed contract revision.

### Non-Goals and Honesty Boundaries

No additional raw-model/runtime-SDK substitute, private subscription-secret import, remote host/cloud profile, vendor default selection, forced universal S2 promise, release, account change or native qualification is performed by these documents. Existing Claude/Codex work is referenced, not superseded/requalified by this epic. Job Objects manage lifecycle and do not establish hostile-code containment. WSL/macOS/Linux/native Windows and every architecture/host require their own evidence. No synthetic fixture, Wine/cross-compile result or signed-in account establishes full live fidelity, included-only funding or process ownership.

### Open Questions

- Exact current installed/native surfaces and public operations — per child; default unknown/unsupported until dated official primary documentation and authorized non-secret observation bind the exact build. No current vendor behavior is inferred from master candidate text.
- Applicable effective funding/auxiliary and no-paid-continuation mechanism — per child; default block subscription-only if any required route is unproven. A differently funded route needs distinct identity and authority.
- Initial platform/host subset — resolved through each child’s bounded discovery-first design outcome: an evidenced exact candidate or explicit blocked inventory. Unknown subset does not block designing that discovery outcome; it continues to block unsupported adapter implementation/live promotion. Every absent pair remains unknown, never inherits another architecture’s result. MH-11/MH-3/MH-7 retain their producer scope.
- Implementation decomposition — discovery-contract design may proceed now; after an evidenced candidate is available, freeze exact independent producers/consumers and split by surface/platform before adapter implementation tasks. A blocked inventory preserves the unresolved facts and next test rather than inventing implementation readiness. No API or migration number is preallocated.
- Epic structure proceeds under explicit project delegation; independent root review remains required before commit. No founder decision is requested and no reviewed lifecycle state is implied.

### Risks

- Credential/config discovery can execute startup hooks or disclose secrets — require safe allowlisted read-only observations, effective-source review and bounded redaction; block opaque mandatory sources.
- A target’s free/included plan can still invoke paid auxiliaries — qualify the whole execution tree and effective provider/model routes; absence of reported cost is unknown, not zero.
- A native command may exit zero after denial or lose children on cancellation — require semantic outcome and exact process-boundary evidence, retaining uncertainty and no automatic relaunch.
- Shared fixture/adapter/platform changes can invalidate evidence — pin exact revisions/config/platform and revalidate affected rows; do not rewrite historical evidence or untested support.

### Definition of Done

- [ ] Independent refinement accepts the five child scopes, exact upstream producer boundaries and fixed finite limits.
- [ ] Each child has its own reviewed design/tasks and a versioned target/surface/platform evidence inventory, even when blocked.
- [ ] All advertised support is independently bound to exact current evidence; unknown/refused routes expose a bounded next test and authority requirement.
- [ ] No completed Claude/Codex, Windows implementation or registry work is duplicated; public support advances only through the applicable actual gates.
