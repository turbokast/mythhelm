## Immutable local verification inputs — Requirements

> Refined. Run /spec immutable-local-verification to produce the design and tasks.
>
> Bind local gate results to the exact materialized inputs the check executed, with bounded snapshot preparation and honest support labels. This is harness process hardening, not an implementation or product qualification claim. Normative harness sources are WORKFLOW.md and docs/harness/charter.md. Supporting product invariants refer to MYTHHELM Master Specification v2, version 2.0, especially I07/I09/I20/I21 (§2) and trusted-host limits (§8.1).

## Context

- **Source**: Maintainer description requesting a private, deterministic reproduction and requirements-only capture for immutable execution inputs; no backlog/product-state change is included.
- **Grounding revision**: `615e74a6bb6907f87bd2065e2f576f5e18b5dec5` on `origin/main`.
- **Scope boundary**: Frozen use is an ASSUMPTION needed to interpret endpoint equality as unchanged execution inputs; the baseline does not document a continuous frozen-use requirement. The reproduction deliberately violates that assumption. It identifies a limitation of endpoint observations, not evidence that a quiescent, correctly owned worktree fails its endpoint contract, or that an unrestricted same-user host is contained.

Today `run_gate` fingerprints its scope, executes a fixed command with the working tree as `cwd`, and records a successful pass when the post-execution fingerprint equals the first one (`scripts/harness/gatelib.py:233–274`; locate with `rg -n 'def run_gate|before = fingerprint|cwd=root|after = fingerprint' scripts/harness/gatelib.py`). A successful marker is fresh when its scope fingerprint and absolute tree identity match (`marker_state`, lines 203–213). These endpoint comparisons do not observe every intermediate input read.

A bounded private fixture used the real `gate.sh` engine and a real Python fixture consumer, without a substitute compiler, mocked gate result or copied marker. A quiescent recognized BAD input failed. Deterministic barriers then allowed an external fixture actor to expose GOOD only while the consumer read, restore BAD before command exit, and obtain a legitimate successful marker whose endpoint fingerprint still matched BAD. The target marker was reported fresh; the overall status was not globally green because unrelated gates had not run. Raw driver/output/paths stay private. This is a frozen-use-assumption violation used as a counterexample, not a hostile-host trust claim.

The proposed result must describe an immutable input view and its manifest. Merely copying a mutable tree and retaining the original pre/post hash would relocate the ambiguity. A mixed or changed capture cannot be presented as a verified original candidate. A same-user adversary can also alter tools, the controller or receipts; input snapshots do not independently attest that host.

### Premise grounding

| Claim | Verdict | Source and consequence |
|---|---|---|
| Gate success is currently recorded from command exit plus equal endpoint fingerprints. | HOLDS | `gatelib.run_gate`, lines 233–274; the private barrier fixture exercised the real wrapper successfully. |
| Equal endpoint fingerprints can miss a transient recognized-input change when the frozen-use assumption is violated. | HOLDS | The private recognized-input control and deterministic transient-read counterexample distinguish these states. This is the corrected, bounded premise adopted here. |
| Freshness also checks the originating working tree. | HOLDS | `gatelib.marker_state`, lines 203–213; a snapshot result must preserve this provenance rather than become transferable evidence. |
| The current inventory includes tracked and untracked, non-ignored files; symbolic links contribute their link text. | HOLDS | `listed_files`/`fingerprint`, lines 116–143; external symlink target contents are not independently certified by that representation. |
| Current commands include checking forms and arbitrary repository programs; complete write behavior requires per-profile qualification. | HOLDS | The fixed command inventory is at `GATES`, lines 76–87. `gofmt -l` and `go mod tidy -diff` are checking forms; `go test` executes repository tests, `go run` invokes tools and can populate external caches, scripts can create fixtures/output, and the wrapper writes private marker metadata (`marker_path`/`write_json_atomic`, lines 192–222 and 272). Purity or hermeticity cannot be inferred from the gate name. |
| Current markers do not record a complete tool/environment identity; trusted-host limits remain distinct from input evidence. | HOLDS | The emitted marker field inventory is at `run_gate`, lines 267–270. Commands inherit process environment (lines 247–252); Git helpers only remove selected Git override variables (`scripts/harness/runspec.py:78–83`). v2 §8.1 distinguishes trusted-host use from enforced containment. |

Continuous-input equality and hostile-host attestation are expressly excluded interpretations, not premises to implement. No known current check is waived by this capture.

### Initial supported profile and fixed decisions

The first strong profile is **Linux/amd64, `go-fmt` only**. It executes the actual shared `gatelib.GATES["go-fmt"]` argv, `gofmt -l .`, in a staged independent manifest-bound copy. Success retains both current conditions: exit zero and empty stripped stdout; nonempty formatting output fails even if gofmt exits zero. No Go compiler or inference invocation qualifies this profile. Other gates and platforms remain explicitly legacy-observed until independently qualified; no required CI check is removed.

The verifier child shall execute inside a qualified private Linux read-only mount namespace using a pinned bubblewrap helper (or an equivalently reviewed/pinned existing Linux helper). The profile shall bind helper/tool versions and executable digests, namespace/mount setup and actual write-boundary probes. Source mounts are read-only, without writable aliases; declared private scratch, bounded output, tool/cache and marker metadata are outside that input view. Helper absence, unsupported user namespaces, failed mount/write probes or unavailable enforcement means **immutable-qualified unavailable**, never a renamed writable copy. A read-only mode bit or copied directory alone does not qualify. This mechanism trusts the host, controller and toolchain; it does not protect against hostile same-user code outside the qualified child, host administrators or tool substitution by a hostile host. For this initial profile, capture admits regular files/directories only and rejects symlinks, junctions, special files, escaping paths and colliding normalized names; broader link support is deferred. It includes tracked and non-ignored untracked source inputs and explicit tracked deletions, excludes ignored private runtime/configuration state, and does not copy raw Git metadata. Controller Git observations pin candidate/base identity; gofmt itself requires no Git view. Actual implementation/platform qualification is future work, not earned by this requirements capture.

Initial limits are fixed: 20,000 files, 64 MiB aggregate source bytes, 8 MiB per file, 1 MiB copy buffer, 30 seconds snapshot preparation, 10 seconds gofmt execution, and 4 MiB combined captured output. Each limit is enforced incrementally, with boundary and one-over fixtures; exceeding it refuses acceptance. The initial controller admits one capture/execution at a time. Cancellation allows 2 seconds graceful termination then forced termination with at most 5 seconds join/reconciliation; unresolved process or ownership state quarantines the owned artifacts, refuses acceptance and refuses destructive cleanup.

The private store admits at most 64 snapshots and 10 GiB aggregate retained bytes, including staged/quarantined artifacts. Successful unpinned terminal snapshots expire after 24 hours, failed terminal snapshots after 7 days. Active, unknown-owner or referenced snapshots are never automatically deleted. Admission reserves bounded capacity, and inability to recover capacity safely blocks new strong runs; deletion only reconciles owned terminal artifacts and never follows foreign links.

Marker schema 2 shall explicitly distinguish `immutable-qualified` from `observed-copy`, bind profile/manifest/candidate identity, and refuse unknown critical schema fields/versions. Schema 1 remains usable only under an explicit `legacy-observed` consumer policy; it never satisfies a request for immutable evidence. Existing baseline endpoint behavior is cited above; any separate frozen-use documentation clarification is proposed, not baseline fact.

The central result-consumer inventory is: `gatelib.marker_state`; `gatelib`'s `status` CLI and `stop-hook` caller; `.claude/hooks/verify-task-completion.sh`, which invokes that stop-hook rather than reading markers itself; and `scripts/harness/health.py:check_gate_markers`, which invokes `marker_state`. Repository-wide searches for `marker_state`, `gate-marker-` and `gatelib.py status/stop-hook` identify these executable consumers at the grounding revision. `runspec.py` and `finalize.py` do not directly read markers there; procedures that call status are documentation, and marker fixtures/assertions are tests, not extra production readers. Implementation shall retain this centralized inventory and rerun the search for new callers before migration.

### Objectives

- **O1**: A result identifies the exact source/check inputs actually consumed and cannot credit one candidate for a pass obtained from different transient bytes.
- **O2**: Original candidate source files, index, refs and unrelated work remain untouched by the verifier while checks run against a separately owned, supported input view; declared private result artifacts remain outside that input view.
- **O3**: Gate support, write effects, environment/tool assumptions, incomplete execution and cleanup remain explicit and bounded.
- **O4**: Existing shell-capable clients and gate consumers retain one shared verification contract, with qualified incremental rollout.

### Non-Goals (and what still binds)

| # | Out of scope | Still binding in this spec |
|---|---|---|
| N1 | Universal attestation against hostile administrators, same-user arbitrary code, compromised kernels, tools or receipt writers. | Charter principles 1/2; I09 and v2 §8.1: disclose the trusted-host/toolchain assumptions and tested enforcement subset. |
| N2 | Native inference, authentication, secret-store copying, provider qualification, new spending or publication authority. | Charter principle 5 and public-repo hygiene; snapshots/results grant none of these effects. |
| N3 | Replacing product protected-acceptance records, the supervisor, journal or task ledger. | I07/I20/I23: harness markers remain scoped process evidence; product acceptance retains its independent contract. |
| N4 | Silently dropping current gates, converting formatters to validators, or declaring every platform/gate immutable at once. | WORKFLOW quality gates and charter principles 2/3/6 remain mandatory until an explicitly reviewed rollout replaces a supported path. |
| N5 | Publishing snapshot source, raw environment, private configuration, raw reproduction output or host paths. | Charter principle 5/public-repo hygiene; only reviewed public identifiers and sanitized evidence references leave private storage. |

## Functional Requirements

### FR-1 — Explicit input manifests and candidate identity

- **AC-1.1** [Charter principles 1/6; I07/I20 (v2 §2)] When an immutable-mode gate starts, the system shall create a versioned manifest identifying the intended candidate/source revision and base, the included tracked/untracked paths, file types, executable bits and content digests, deletion semantics, input-policy revision, exact gate/check-definition identity and materialized-view digest. `TestVerificationManifestPinsInputs` shall change each binding independently and invalidate reuse.
- **AC-1.2** [Charter principles 1/2; I09 (v2 §2)] If a path, required input, intended candidate identity or capture boundary cannot be resolved consistently, then capture shall refuse before an accepted result. `TestAmbiguousCaptureCannotCreditCandidate` shall distinguish quiescent valid inputs from concurrent replacement, missing inputs and unsupported shapes.
- **AC-1.3** [WORKFLOW gates/markers; I07/I09 (v2 §2)] When capture observes a view different from the requested candidate, the system shall either refuse or identify the actual captured view without crediting the requested candidate. `TestTransientReadCannotCreditBadCandidate` shall retain the real BAD/GOOD/BAD counterexample: a GOOD snapshot pass shall never become a fresh accepted result for the original BAD manifest.

### FR-2 — Separately owned immutable execution view

- **AC-2.1** [Charter principles 1/2; I07/I08 (v2 §2)] When a supported gate executes, its input reads shall use the finalized materialized view matching the recorded manifest. `TestOriginalChangesDoNotChangeExecutedInputs` shall coordinate changes to the original tree after capture and observe the check consuming the pinned view; the verifier shall not modify original candidate source/index/refs. Declared private result-artifact writes shall remain outside the immutable input view; external author/fixture-actor changes shall be distinguished from verifier effects.
- **AC-2.2** [Charter principles 1/6; v2 §8.1] If the declared execution mechanism cannot keep its source input view immutable for the supported command and write envelope, then immutable mode shall refuse or report unsupported. `TestExecutionViewWriteBoundary` shall include real ordinary writes from the gate process and a positive allowed scratch-output control. Directory modes alone shall not be described as a hostile same-user boundary.
- **AC-2.3** [Charter principles 2/3; I09 (v2 §2)] If only a copied writable tree or endpoint-only checking is available, then the result shall carry that limited mode and shall not satisfy a caller requiring immutable input evidence. `TestWritableCopyDoesNotMasqueradeAsImmutable` shall reject relabeling that fallback.

### FR-3 — Preserve actual gate semantics and declared effects

- **AC-3.1** [WORKFLOW quality gates; Charter principles 3/6] Before a gate profile is enabled, the system shall inventory its actual argv, input reads, Git queries, source-write behavior, temporary/cache writes and external tool/service assumptions. `TestGateProfileMatchesRealCommand` shall execute qualified tools/consumers and bind their definitions; a fixture-only stand-in shall not qualify the real Go compiler, linter or another platform.
- **AC-3.2** [WORKFLOW formatter-before-check; I07/I20 (v2 §2)] When a source-mutating formatter or transform is requested, it shall remain an explicit transform producing a distinct candidate identity, followed by verification of that output. `TestTransformCannotReuseInputPass` shall reject treating a transformed source as a pass of the pre-transform candidate. Existing checking forms such as `gofmt -l` and `go mod tidy -diff` shall retain their exit/output contracts.
- **AC-3.3** [Charter principles 1/2/6; I09 (v2 §2)] When tests or tools need writes, the qualified profile shall direct declared scratch/cache/artifact effects outside immutable source inputs and retain their actual exit/output semantics. `TestDeclaredOutputsAndUndeclaredSourceWrites` shall distinguish allowed output, forbidden source mutation and unavailable isolation. `go test` and arbitrary scripts shall not be assumed read-only from their names.

### FR-4 — Filesystem and Git input completeness

- **AC-4.1** [Charter principles 1/6; I07/I08 (v2 §2)] When the source view includes untracked non-ignored candidate files, their actual bytes and identities shall participate; ignored runtime state shall remain excluded by explicit policy. `TestTrackedUntrackedIgnoredManifest` shall include a meaningful untracked check input, a deleted tracked file and unrelated ignored churn, not an empty universal.
- **AC-4.2** [Charter principle 2; v2 §8.3] If symlinks, junctions, special files, path/case collisions or paths escaping the approved root cannot be represented and materialized safely, then capture shall reject them. Any supported link policy shall name its target boundaries and bind the actual readable input. `TestLinkAndTraversalCapture` shall include target changes and escaping links; hashing only a link string shall not certify external target bytes.
- **AC-4.3** [Charter principles 1/5; I08/I09 (v2 §2)] When a gate depends on Git metadata, the profile shall pin the relevant object/ref/base/index semantics in a private read-only or separately writable Git view, without borrowing mutable original refs/index or copying credential-bearing configuration. `TestPinnedGitViewAndUnrelatedIndex` shall exercise changed original refs/index, missing objects and a gate that genuinely queries Git. Minimal filesystem copies that break these queries shall report unsupported rather than pass a substitute command.

### FR-5 — Honest result and marker binding

- **AC-5.1** [WORKFLOW gates/markers; Charter principles 1/2; I07/I20 (v2 §2)] When a gate succeeds, its result shall bind the exact executed-view manifest, gate definition/argv, tool/environment profile identity and actual exit/output evidence. `TestResultBindsExecutedView` shall reject another snapshot, altered check definition and wrong candidate even when source endpoint fingerprints coincide.
- **AC-5.2** [Charter principles 1/3; I09 (v2 §2)] When status/completion/PR verification evaluates a result, it shall distinguish verified snapshot identity from an observed comparison with a mutable worktree. `TestResultConsumerRequiresExactInputIdentity` shall refuse wrong-scope/candidate results; an endpoint comparison shall not claim continuous ownership or universal freshness of the original checkout.
- **AC-5.3** [WORKFLOW gates/markers; Charter principles 2/6] If execution fails, is interrupted, times out, changes a protected input, loses its manifest or lacks required evidence, then no accepted immutable marker shall remain. `TestFailedSnapshotRunCannotRetainPass` shall include a previously successful marker, incomplete output and cancellation controls. Schema-2 mode/critical-field validation and the explicit schema-1 legacy-observed policy above shall apply to every inventoried consumer; unknown critical schemas refuse. Legacy schema-1 markers shall never be silently promoted to immutable evidence.

### FR-6 — Bounded lifecycle, cancellation and cleanup

- **AC-6.1** [Charter principles 1/2; I21 (v2 §2)] When capture/execution is admitted, it shall have explicit finite file/byte/depth, duration, concurrency and retained-artifact bounds. `TestSnapshotBudgets` shall use limits of two files/64 bytes and reject a third file/65 bytes without a successful marker; the fixed initial production limits above shall also have exact-boundary and one-over controls.
- **AC-6.2** [Charter principles 1/6; I08/I09 (v2 §2)] When cancellation or a copy/tool failure occurs, the system shall stop/reconcile its owned processes, preserve original checkout state and clean only its own bounded temporary artifacts. `TestCaptureCancelAndCleanupOwnership` shall include interrupted copy, a running consumer, cleanup failure and an unrelated sentinel. Unconfirmed cleanup/process state shall remain explicit, not success.
- **AC-6.3** [Charter principles 2/5; I09 (v2 §2)] If artifacts cannot be safely removed or are retained for diagnosis, then their bounded ownership, location and disposition shall be reported privately with a resumable reconciliation path. Automatic deletion shall not traverse foreign paths or discard evidence merely to obtain a clean result.

### FR-7 — Toolchain/environment limits and privacy

- **AC-7.1** [Charter principles 3/5; v2 §8.1] When a result records execution context, it shall distinguish observed tool identity/version, declared policy and unknown external/cache state. `TestToolAndEnvironmentProvenance` shall cover a changed executable/configuration and unknown tool version. A trusted-toolchain declaration shall not be represented as cryptographic host attestation or deterministic network provenance.
- **AC-7.2** [Charter principle 5; public-repo hygiene; v2 §8.3] When manifests, snapshots or receipts are retained/exported, the system shall use private bounded storage, omit raw environment and native-secret/config stores, and require an explicit safe policy for necessary non-secret settings. `TestPrivateVerificationArtifacts` shall use synthetic secret sentinels and verify no public artifacts/log fields contain them. Redaction shall not be advertised as proof arbitrary logs are safe.

### FR-8 — Incremental shared-engine rollout

- **AC-8.1** [Charter principles 3/6; WORKFLOW shared gates] When a gate/platform profile is enabled, its implementation shall reuse the shared `gate.sh`/gatelib command-selection, execution and result-consumer contracts, with a versioned compatibility policy and credential-free regression fixtures. `TestSharedGateModeCompatibility` shall exercise direct shell and existing marker consumers without relying on a client-specific hook.
- **AC-8.2** [Charter principles 1/2; I09 (v2 §2)] When only some gates/platforms are qualified, reporting shall name that subset and preserve the remaining current frozen-worktree workflow without asserting immutable verification for it. Rollout shall not remove a required check or bypass an unsupported profile. `TestPartialRolloutLabelsAndRequiredGates` shall keep unsupported, legacy, failed and verified states distinct.

## Non-Functional Requirements

- **NFR-1** [Charter principles 1/6]: Regression proof shall use deterministic read/copy/exit barriers and a real gate/consumer; it shall include quiescent good and bad controls, bounded deadlines and process cleanup. No fake compiler, copied passing marker or timing-only sleep shall establish support.
- **NFR-2** [Charter principles 3/6]: Qualification shall state exact supported OS/filesystem/tool combinations. A Linux fixture shall not qualify macOS/Windows, Git metadata semantics, arbitrary test write behavior or another isolation mechanism.
- **NFR-3** [Charter principles 1/5]: Result and artifact formats shall be bounded/versioned and preserve unknown data explicitly. Public documentation shall contain generic mechanisms and reviewed synthetic examples, not private reproduction paths or raw session/configuration data.

## Dependencies

- Existing `scripts/harness/gate.sh`, `gatelib.GATES`, fingerprint/marker consumers and canonical gate tests are the baseline to extend; no replacement executor or second acceptance authority is proposed.
- The dependency-scope repair tracked separately as `delivery-github-gate-scope` covers actual workflow/helper/test inputs. Dependency completeness and immutable execution are distinct: neither substitutes for the other. Re-ground its exact merged revision during refinement; it is not asserted merged here.
- The separately tracked local four-worker gate activation changes scheduling, not input identity. This spec shall preserve its qualified argv/resource contract when present; it is not a functional dependency of the requirements capture.
- `contained-execution-profiles` and `protected-acceptance` offer relevant boundary/evidence vocabulary. This harness slice does not earn their native/platform gates or replace product evaluators.
- Rollout may be split into shared manifest/result contracts, one proven read-only gate profile, and separately qualified Git-dependent/test/transform profiles. A partial rollout must retain FR-8's honest labels and required checks.

## Impacted components

- Harness: `scripts/harness/gatelib.py`, `gate.sh`, their focused tests, marker schemas and the actual result consumers in `runspec.py`, `finalize.py`, health/completion hooks; the centralized executable-consumer inventory above is normative and must be rechecked during implementation.
- Harness documentation: `WORKFLOW.md`, `knowledge/execution.md`, `docs/harness/` and affected gate procedures, updated only when their mechanism ships.
- No application Go packages, native authentication stores, product cards, orchestration state or GitHub repository settings change in this requirements-only capture.

## Refinement decisions and deferred qualification

There are no unresolved choices blocking design of the initial Linux/amd64 go-fmt profile: mechanism, command/empty-output contract, budgets, retention, cancellation and schema migration policy are fixed above. Implementation must pin the reviewed helper/gofmt artifacts and prove their actual supported host prerequisites before enabling that profile; no support is inferred from availability on one developer host. The controller/host/toolchain remain trusted assumptions.

Git-dependent, source-mutating, cache-writing test gates and other platforms are deferred separate qualification slices, with legacy-observed behavior retained in the meantime. They shall not inherit go-fmt qualification. Any future broader profile requires its own explicit metadata/write envelope, bounded limits and real-tool positive/negative evidence, without weakening current required checks.

## Definition of Done

- [ ] Premises and the centralized consumer inventory are re-grounded at the implementation baseline; the initial Linux/amd64 go-fmt command, helper boundary and all fixed limits are proven.
- [ ] AC-1.1–AC-8.2 each has a nonempty positive control and retained violating input; the transient-read counterexample cannot credit the wrong candidate.
- [ ] Qualified mechanisms enforce their declared input/write boundary, with bounded copy/cancel/cleanup and no verifier mutation of original candidate source/index/refs.
- [ ] Versioned results and shared consumers agree on exact input/check identity, failure, legacy and unsupported modes.
- [ ] Toolchain/host, Git/environment, supported-platform and privacy limits are explicit; no universal or product-stage attestation is claimed.
- [ ] Required gates and public hygiene pass; implementation reaches main through reviewed PRs after qualification. This requirements file alone completes none of those implementation checks.
