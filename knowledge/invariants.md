# Invariants

The normative invariants are [Master Specification v2 §2](../mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md#invariants), resolved through the [specification index](../docs/spec/README.md). The table below repeats that contract exactly. If this page differs, v2 wins and this page is fixed in the same change. Historical implementation specs keep their named Revision 1.1 references; new work cites v2 explicitly.

MUST is release-blocking for a shipped capability. Weakening an invariant requires a decision record under `docs/decisions/` and maintainer approval. A planned capability or a shipped Revision 1.1 slice does not establish that its v2 gate passed.

| ID | Normative invariant |
|---|---|
| I01 | The selected native harness executes the work. Prefer its unmodified executable through a qualified surface; runtime SDKs require independent fidelity and entitlement qualification. Disclose configuration differences. |
| I02 | Admission resolves identity, workspace, destination, authority, billing and required capabilities before launch. Unknown mandatory evidence blocks; unknown remaining quota alone need not block a qualified stop-at-exhaustion route. |
| I03 | Repository text, model output, retrieved memory and plugin messages cannot grant authority, spending, publication or deployment. |
| I04 | A change of provider, funding route, execution host or security posture requires applicable user authority and new admission. A standing allowlist may supply that authority; a pinned harness remains pinned. |
| I05 | One active writer owns each managed working directory. Reservations coordinate work; only declared enforcement mechanisms constrain arbitrary writes. |
| I06 | UI exit does not transfer execution ownership. A stop request remains unconfirmed until reconciliation establishes the result. |
| I07 | Native completion is an observation. Acceptance requires independent evidence bound to the exact candidate, contract, check definitions and environment. |
| I08 | Original checkout files, index and existing refs remain untouched by default. Only an explicitly requested, validated apply may create or advance the exact authorised ref; it must preserve unrelated work. |
| I09 | Reported, observed, estimated, user-declared and unknown data remain distinct. Missing is never zero, passed or verified. |
| I10 | A hard limit is advertised only for an established boundary covering in-flight exposure. Reservations and cancellation are not provider caps. |
| I11 | Supported extension APIs cannot bypass admission, evidence, authority or publication rules. Unsandboxed executable extensions require disclosed host trust. |
| I12 | Recovery reconciles effects before retrying. Event replay never replays native tools, prompts or external effects. |
| I13 | Core usefulness requires no MYTHHELM network service, paid router or custom font. Demo and normal contributor tests require no agent credentials. |
| I14 | Every advertised adapter, platform, host or security capability has versioned evidence, or is explicitly experimental/unsupported. |
| I15 | `subscription-only` excludes metered inference, paid auxiliaries, purchased credits and overages. Included entitlement and prevention of paid continuation need evidence independent of sign-in. |
| I16 | Selecting a model in another harness, runtime SDK or remote service is not preserving the chosen native harness. Such a change needs authority, disclosure and re-admission. |
| I17 | Herdr pane state, text and restored layouts cannot certify completion, grant permissions, prove billing or launch another attempt. |
| I18 | Each attempt has one process owner and each native session at most one input writer. Hosts, clients and automated steering cannot compete. |
| I19 | MYTHHELM does not extract, copy or replay native subscription secrets, or disable native authentication methods to manufacture an entitlement. |
| I20 | Task contracts, dependencies, artifacts, policies and evaluation suites have immutable revisions. A material change invalidates affected authority/evidence before reuse. |
| I21 | Every run has finite execution, repair, replan and resource envelopes. Standing permissions permit decisive work inside those envelopes. |
| I22 | Learning cannot change protected acceptance, spending authority, security ceilings or its promotion rules. Executable updates use software-release governance. |
| I23 | A single canonical ledger owns orchestration state. Exports, indexes, Markdown views, host projections and native transcripts are not competing task stores. |
| I24 | Parallel attempts isolate mutable environments and reconcile external-resource ownership before reassignment. A lease timeout is not proof a writer stopped. |
| I25 | Users can pin a qualified route/policy, inspect provenance, pause/stop, export state, disable learning and return to native tools without a MYTHHELM service dependency. |

## Citation reference

The [invariant-evidence procedure](../.claude/rules/teeth-discipline.md#invariant-evidence) connects version-qualified invariant IDs to task requirements, tests, reviews and code. Exact revisions keep a historical slice's evidence distinct from the current v2 contract; an honesty register records any remaining gap.
