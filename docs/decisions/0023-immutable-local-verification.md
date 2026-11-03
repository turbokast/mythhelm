# 0023. Immutable local verification inputs and owned evidence storage

- Status: proposed
- Date: 2026-11-03
- Source: `specs/*/immutable-local-verification/`, Task 8; independently reviewed design revision `1ebabd6a500613665ecdad9f1ca56d74fe81b7fa`, delivered by [PR #619](https://github.com/turbokast/mythhelm/pull/619) at `6a81f6036bab0fcc7410b03ff65d845763d33c3c`.
- Design content SHA-256: `e69693d7e0f3a97eba2d2607edc6e40a3bc266749b1a40e0aa6d3c213f5d3d46`.

## Context

The shared gate engine runs checks from a mutable working tree and accepts an exit-zero result when its scope fingerprints agree before and after execution. `scripts/harness/gatelib.py` defines this behavior in `run_gate`, `fingerprint` and `marker_state`; `go-fmt` additionally requires empty stripped stdout. Endpoint equality does not identify every intermediate read. A candidate can therefore require evidence about the separately materialized bytes actually consumed, while the original checkout remains untouched.

The requirements retain existing checks, explicit legacy evidence and a narrow initial stronger profile. They do not request universal hermetic execution, native inference, spending, publication, product acceptance or protection against a hostile host. The relevant obligations are harness charter principles 1/2/3/5/6 and Master Specification v2 I07/I08/I09/I20/I21/I23 (§2), with the trusted-host and filesystem/privacy limits in §§8.1/8.3. [WORKFLOW.md](../../WORKFLOW.md) governs delivery; [GOVERNANCE.md](../../GOVERNANCE.md) requires a decision record for persistence and security boundaries even when they belong to contributor tooling.

This proposal adopts the source design's §§2–6 contracts and D1–D7 decisions. It introduces durable private manifests, ownership and crash reconciliation before their implementation. Proposed interfaces, codecs and qualification probes remain future work. No helper, tool, kernel, filesystem or production profile has been qualified by writing this record.

## Decision

### Candidate identity and bounded capture (D1/D2)

Require a caller-selected immutable candidate descriptor before strong execution. It binds the originating tree, observed HEAD/base/index, included source paths/types/executable bits/content hashes and tracked-deletion tombstones, selected-path closure, input policy and current checker definition. HEAD alone or hashes computed only after copying cannot select the intended dirty bytes. Captured and finalized bytes must match that previously selected descriptor; ambiguous capture refuses rather than crediting a different requested candidate. Git provenance and current-tree comparisons remain observations, without asserting an atomic historical dirty-tree state or continuous quiescence.

Use the design's separate incremental `verification_capture.observe_git` path before any buffered legacy root/base/inventory/fingerprint helper. One `PreparationBudget` starts before the first root, Git, store-placement or filesystem observation and covers the entire invocation: HEAD/base/index/worktree observations, enumeration, capture, re-enumeration and rehash. It is never reset per subprocess or phase. Only fixed read-only Git subcommands are admitted, with sanitized environment and no shell, candidate-supplied command, hook, filter, pager, external diff or network operation. Only the specified absent optional origin/main ref may select the verified HEAD fallback; timeout, malformed data and other command failures cannot.

Input policy `linux-amd64-go-fmt-input-v1` includes tracked and nonignored untracked regular files, ancestor directories and tombstones, without applying the legacy Go fingerprint scope filter. It excludes raw Git metadata and ignored runtime/configuration contents. A separate bounded names/types walk includes ignored and dot directories because actual gofmt recursively selects non-dot basenames ending in `.go`. If any selected path is absent from the requested inventory, including an ignored private Go file, refuse `unmodeled-consumer-input`; neither copy its private contents nor silently omit its effect. Finalized selection must equal requested selection. Qualification repeats real-tool selector probes rather than trusting an inventory approximation.

Reject unsupported links, hardlink aliases, special files, escaping/invalid/control-character paths, unsupported encodings, excessive depth and NFC-plus-casefold collisions. Traverse through no-follow directory/file descriptors; copy into independently created owned files with no links back to the source. Verify file and parent identity before/after copying, hash copied bytes and rehash the finalized independent view. Observable replacement, deletion or mutation refuses. No capture or verifier step modifies original source, index, refs or unrelated work. Source transforms require a new candidate; the initial strong profile refuses transform argv and Git-dependent checks.

### Durable private store and ownership (D6)

Choose a private harness record store outside every registered Git worktree, beneath platform state home at `mythhelm/verification`. Directories are mode 0700 and files 0600. Refuse symlinks and placement inside any registered worktree. The same bounded observer supplies the finite worktree-root set before `open_store`; the store itself launches no Git subprocess. Snapshot source cannot live under worktree-local ignored state because gofmt can still discover its Go files. Existing worktree marker locations retain JSON metadata only.

Use the exact `Store`, `open_store`, capture, execution and evaluator signatures and closed record schemas in source design §3, without creating a second public API or persistence authority. Frozen validated records use canonical UTF-8 JSON and SHA-256 bindings; duplicate/unknown keys, unsupported versions, invalid enums/nonfinite values and over-limit inputs refuse. Records retain private tree identity and bounded references, never raw ambient environment or credential-bearing Git configuration. This store owns harness evidence, not the product journal, task ledger or orchestration state.

A global store flock admits one capture/execution at a time without waiting indefinitely. It coordinates cooperative controllers; it does not exclude hostile same-user writers. Admission reserves capacity before allocation. Stable random snapshot IDs and 256-bit owner tokens bind directory ownership. Under the lock, compare-state transitions use atomic JSON replacement and file/directory fsync:

`reserved → capturing → finalized → executing → succeeded | failed | cancelled`

Any unresolved interruption becomes `quarantined`. Failure before allocation leaves no artifacts. Recheck ownership, modes, tokens and no-follow dirfds before cleanup; a pathname alone never authorizes deletion. Atomic transition faults must leave a recoverable durable state. Reservations remain charged until terminal reconciliation, including staging, tools, output and quarantine.

### Finite envelopes and reconciliation

The initial production limits are fixed; a candidate cannot raise them. Design §§2–4 define the complete limits and strict record/byte parsing rules.

| Resource | Limit |
|---|---|
| Candidate/materialized entries, including tombstones | 20,000 |
| Present aggregate source / individual file / copy buffer | 64 MiB / 8 MiB / 1 MiB |
| Path / depth / inspected closure nodes | 4096 UTF-8 bytes / 64 components / 40,000 nodes |
| Preparation / execution | One shared 30-second preparation deadline / separate 10-second execution deadline |
| Combined raw stdout and stderr / scratch | 4 MiB / 16 MiB tmpfs with a finite observed inode ceiling |
| Retained snapshots / total retained and reserved storage | 64 / 10 GiB |
| Private record / entries per record | 8 MiB / 20,000 |
| Copied helper and tool artifacts | At most 64 MiB each |
| Git inventory stdout / stderr / pending record | 8 MiB / 64 KiB / 4096 bytes; count raw records before deduplication |
| Metadata stdout / records; OID output | 4097 bytes / one record; 128 bytes for validated object IDs |
| Registered worktree inventory | 1024 records, 256 worktrees, 4096 bytes per record, 1 MiB aggregate stdout |
| Streamed index hashing | 8 MiB total, at most 1 MiB per read |

The observer reads at most 64 KiB per operation and waits no longer than the smaller of 50 ms or remaining preparation time, checking deadline/cancellation before reads and parsing. Missing terminators, overflow and nonzero/malformed required observations refuse even if a child has already exited successfully. Reservation covers source, up to two 8 MiB metadata records, helper/tool copies, scratch and output; account shared profile artifacts and quarantined content against the same store cap. Capacity that cannot safely be reclaimed blocks admission.

Record process PID/start identity and pidfd. Cancellation, timeout or overflow stops useful work, sends TERM to the owned session/process group, allows at most 2 seconds grace, then KILL with at most 5 additional seconds for join/reconciliation. These allowances are cleanup time, never an extension of preparation or acceptance. Close/drain descriptors only within finite bounds. Leader exit alone does not prove descendants or pipe owners stopped. A qualified PID namespace/reaper must establish teardown for execution; unresolved identity or descendants produce `ownership-unknown`/quarantine and prohibit accepted markers and destructive cleanup. The same rule applies to Git children before snapshot allocation, with a bounded private reconciliation record.

After a crash, reconciliation verifies process start identity, child/namespace exit and actual artifact ownership before cleanup; it never blindly kills a reused PID, reruns a check or infers teardown from elapsed time. `reconcile` is idempotent. Successful unpinned terminal snapshots expire after 24 hours; failed/cancelled terminal snapshots after 7 days. Active, quarantined, unknown-owner or referenced snapshots never expire automatically. Collection removes only owned, terminal, unreferenced artifacts through no-follow dirfds. Accepted markers hold references; replacement/clearing updates them under the lock. Failed publication reconciles orphan references.

### Qualified execution and evidence chain (D3/D4)

The first possible strong profile is `linux-amd64-go-fmt-ro-v1`, for an actually qualified Linux x86-64 local filesystem/mount combination. It uses the real pinned ELF64 x86-64 static gofmt without PT_INTERP and a reviewed root-owned, non-group/other-writable bubblewrap helper. Copy/hash the actual tool before and after staging, execute that artifact, and record observed versions plus expected/actual SHA-256 identities. Unknown versions, dynamic tools, absent kernel/helper features or incomplete qualification are unavailable. PATH discovery and portable stand-ins cannot qualify or automatically activate a production profile.

The namespace has read-only `/input` and `/tool/gofmt`, private procfs/minimal devices and bounded writable `/scratch`; its root is remounted read-only after mountpoint setup. It exposes no original checkout, store root, raw Git view, host home/configuration, unrelated temporary directory or writable source alias. Close nonessential inherited descriptors and use null stdin. Design §5 fixes user/PID/IPC/UTS/network isolation, dropped capabilities, disabled nested user namespaces and clear environment. The sole environment is its safe whitelist for PATH, home, temporary, configuration and cache directories, disabled Go telemetry, C locale and `GOMAXPROCS=1`.

Preserve actual shared gate argv `gofmt -l .`, cwd `/input`, and the existing acceptance predicate: exit 0 and empty stripped stdout. Bind namespace wrapper argv separately rather than relabeling it as the gate command. Source-write denial uses actual gofmt write probes; a positive scratch-write control proves the tool is functioning. Both the immutable input boundary and allowed scratch effects need real evidence.

The independently recomputed current `gate_definition_digest` covers canonical gate name/scope/argv/output predicate, actual shared engine and all four verification implementation modules, and input-policy revision. Bind it through the acyclic definition → provisional plan → probe artifacts → qualification → profile chain, then candidate/manifest/execution/marker. Recompute the complete chain on load and recheck the current definition independently in qualification, execution and consumption. A new candidate under changed checker code cannot reuse an old qualification, even with unchanged source/tool/argv. No changed definition is accepted merely because the caller supplied its new digest.

All seven fixed probes must pass: `real-command`, `selector-closure`, `source-write-denied`, `scratch-write-allowed`, `no-source-alias`, `isolation-unavailable`, `pidns-teardown`. Their closed evidence binds the plan/definition, actual helper/tool, kernel, architecture and filesystem/mount observations. Unavailable/failed/skipped probes cannot qualify. Recheck executable identities and bounded mount/write enforcement on every run. The trusted host/controller/helper/toolchain and private receipt writers remain assumptions; mount enforcement and rehashing do not attest an adversarial host or continuous original-tree equality.

### Consumer migration and compatibility (D5/D7)

One `evaluate_marker` implements acceptance with explicit `legacy-observed` or `immutable-required` policy. Keep schema 1 usable only under legacy policy, with baseline observed scope-fingerprint semantics and no inferred immutable identity. Schema 2 distinguishes `immutable-qualified`, `observed-copy` and `legacy-observed`; observed-copy reading never becomes an automatic weaker executor fallback. Immutable-required accepts only complete schema-2 immutable evidence matching candidate, manifest, tree/scope/gate, checker, profile and execution, with retained referenced artifacts. Unknown critical schema/keys, corrupt or missing artifacts and changed bindings refuse. Old immutable qualification cannot be downgraded to observed success under either policy.

Migrate the actual shared consumers: gatelib status/stop-hook and compatibility `marker_state`, `scripts/harness/health.py`, and the delegating `.claude/hooks/verify-task-completion.sh` adapter. Rerun the production-reader inventory during implementation. Defaults explicitly retain legacy policy and current required gates, argv, group ordering and four-worker harness-test behavior; status additionally supports immutable-required. Report mode separately from state and label current-candidate comparison observational. Health counts modes separately. Preserve stop-hook override/repeat behavior. No new PR or product-acceptance authority is added to runspec/finalize.

Strong runs clear the previous target marker before setup. Only complete successful execution atomically publishes schema 2 and transfers references under the store lock. Setup/capture/tool failures, unsupported profiles, timeout/output overflow, cancellation and publication failure cannot preserve a prior accepted marker. Other platforms, Git-dependent checks, dynamic tools, source transforms and cache/source-writing tests remain legacy-observed or strong-unavailable until separately qualified; no required check disappears.

Task 8 first produces the independently reviewed, governance-accepted ADR revision and confirms it merged on origin/main. Tasks 1–7 then implement contracts, storage, capture, enforcement, consumer migration, end-to-end proof and documentation in dependency order. Proposed status or source-design readiness does not release Task 1. Any change beyond the accepted persistence/ownership boundary needs renewed architecture review and an appropriate decision record.

## Consequences

| Alternative | Reason for the chosen boundary and cost |
|---|---|
| Keep endpoint-only markers | Retained explicitly as useful legacy observations; they cannot prove which transient bytes a check consumed. Strong mode adds capture/storage and qualification overhead. |
| Writable copies or mode bits alone | Do not enforce the supported tool's immutable source view. Qualified read-only namespace mounts cost platform/helper dependencies and can make strong verification unavailable. |
| Copy ignored inputs or omit them silently | Copying risks private state; omission changes real gofmt selection. Refusal preserves privacy and semantics but makes some otherwise usable trees unavailable for strong mode. |
| Reuse ignored worktree-local storage | Retained snapshot Go files can become tool inputs. Outside-worktree storage adds placement, capacity, ownership and retention management. |
| Reuse the product journal/SQLite store | Would couple contributor receipts to product authority/schema and a separate task ledger. Bounded private JSON records keep the harness independent, at the cost of implementing atomic persistence, locking and conservative reconciliation. |
| Require strong mode for all existing gates now | Would remove useful checks without qualified replacements. Explicit legacy policy preserves them while making the initial supported subset visible. |

Unavailable qualification, conservative quarantine and retained references can exhaust capacity and block new strong runs. This is an explicit failure disposition, never a successful cleanup or hidden fallback. Private artifacts may contain intended repository source; they remain bounded/private, with no raw environment/native-secret/configuration-store copies or automatic export. Public summaries expose only whitelisted labels/digests/sanitized relative references. Redaction cannot certify arbitrary logs safe. No network service, credentials, inference, new spending or publication authority is introduced.

### Validation owned by implementation

These are planned acceptance obligations from the merged task contract, not executed results of this ADR:

| Task | Tests that pin the decision |
|---|---|
| 1 — contracts | `TestQualificationDefinitionBinding`, `TestVerificationManifestPinsInputs`, `TestResultBindsExecutedView`, `TestVerificationStrictSchemas`, `TestToolAndEnvironmentProvenance`: independently mutate critical fields, reject malformed/unknown data and prevent profile/definition substitution. |
| 2 — storage | `TestSnapshotStoreBounds`, `TestRetentionProtectsOwnersAndReferences`, `TestCaptureCancelAndCleanupOwnership`, `TestPrivateVerificationArtifacts`, `TestStoreCrashTransitions`: exact/one-over capacity, real reduced-cap writes, ownership sentinels, atomic faults, replacement and PID reuse. |
| 3 — capture | `TestStrongGitInventoryBudgets`, `TestBlockedGitDeadlineAndCancel`, `TestAmbiguousCaptureCannotCreditCandidate`, `TestTrackedUntrackedIgnoredManifest`, `TestLinkAndTraversalCapture`, `TestSnapshotBudgets`, `TestPinnedGitViewAndUnrelatedIndex`: bounded real Git, one shared deadline, real selector completeness, no-follow capture and original-state preservation. |
| 4 — enforcement | `TestQualificationRejectsCheckerDefinitionDrift`, `TestGateProfileMatchesRealCommand`, `TestExecutionViewWriteBoundary`, `TestDeclaredOutputsAndUndeclaredSourceWrites`, `TestOriginalChangesDoNotChangeExecutedInputs`, `TestLinuxProfileUnavailable`, `TestLinuxExecutionBudgetsAndTeardown`: actual tools/mounts, positive scratch control, old-qualification refusal and reconciled descendants. |
| 5 — consumers | `TestConsumersRejectOldQualificationForNewChecker`, `TestResultConsumerRequiresExactInputIdentity`, `TestFailedSnapshotRunCannotRetainPass`, `TestSharedGateModeCompatibility`, `TestPartialRolloutLabelsAndRequiredGates`, `TestTransformCannotReuseInputPass`, `TestWritableCopyDoesNotMasqueradeAsImmutable`: shared real consumer paths and no invalid-evidence downgrade. |
| 6 — complete proof | `TestTransientReadCannotCreditBadCandidate`, `TestImmutableVerificationAcceptanceMatrix`, `TestQualificationRequiresRealSupportedHost`, `TestPublicQualificationSummaryPrivacy`, `TestImmutableCliExitAndCleanup`: deterministic barriers, real GOOD/BAD controls, retained violating variants and strict unavailable qualification. |
| 7 — shipped documentation | `TestImmutableVerificationDocumentation` and sanitized output generated through the shipped CLI: exact support subset, retained required gates, producing commands and explicit observational/trusted-host limits. |

Task 6 must retain nonempty positive/negative evidence for all AC-1.1–AC-8.2 and NFR-1–3. Portable orchestration/codec tests do not supply actual Linux/tool qualification; unavailable integration stays unavailable. Documentation follows the mechanism and actual evidence. This proposed record contains no new runnable workflow or sample success output; the future interfaces and commands are specified in source design §§2–6.

### Acceptance prerequisite

Independent architecture review and authorized governance acceptance must bind this ADR's exact revision, with merge confirmation on origin/main recorded in Task 8 handoff. This proposed artifact records neither of those events. Acceptance will establish the design decision only; implementation, actual helper/tool/kernel/filesystem qualification and all runtime boundary evidence remain future work. The supported host/artifact choices and any newly discovered consumer drift retain the conservative defaults in the source scratchpad until implementation proves them.
