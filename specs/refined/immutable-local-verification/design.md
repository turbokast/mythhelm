# Immutable local verification inputs — Design

> Candidate design, awaiting independent validation. Requirements and refinement log are unchanged. Normative harness sources: WORKFLOW.md and docs/harness/charter.md; supporting invariants refer to Master Specification v2, version 2.0, §2 and trusted-host limits §8.1.

## 1. Current state and scope

Investigation baseline: `f18ff8c` on `origin/main`. Opened definitions below can be relocated with `rg -n '^def |^GATES|^GO_PREFIXES|^HARNESS_PREFIXES' scripts/harness/gatelib.py`.

- `scripts/harness/gatelib.py:96`: `GATES['go-fmt']` is exactly `['gofmt', '-l', '.']`, `scope='go'`, `empty_output=True`. Harness tests retain `--jobs 4`; group preflights retain their existing ordering.
- `gatelib.py:137–165`: `listed_files(root)` inventories tracked/nonignored untracked paths; `fingerprint(root, scope)` includes deletion/link/type/execute-bit/content representation. It does not enumerate ignored files.
- `gatelib.py:238–297`: `gate_command(root, gate)` returns the fixed argv (shellcheck alone expands paths); `run_gate(root, gate)` invokes from the mutable root, checks exit/output and endpoint equality, writes schema 1. Persistent-change rejection is observational.
- `gatelib.py:215`: `marker_state(root, gate, fps)` centralizes three-state freshness. Its production callers are status (`main`, around line 530), `stop_hook` (around line 428) and `scripts/harness/health.py:139–166`. `.claude/hooks/verify-task-completion.sh` delegates to stop-hook. Reproduce inventory: `rg -n 'marker_state|gate-marker-|gatelib.py.*(status|stop-hook)' scripts .claude/hooks`. Remaining hits are producer helpers or tests; `runspec.py`/`finalize.py` have no direct marker reader at this baseline.
- `scripts/harness/tests/test_gatelib.sh:1–34` deliberately substitutes tools to test orchestration. Keep these portable tests, but never use their stand-ins to qualify immutable gofmt. `scripts/harness/run_tests.py:37–53` discovers `scripts/**/tests/test_*.sh`; new Python suites need colocated shell entrypoints.
- `scripts/vendors/lane.py:73–110` checks root-owned non-group/world-writable bubblewrap and probes namespaces. Its vendor mounts include writable worktrees and native state: do not reuse those mounts for this profile. No vendor invocation is needed.
- Current dependency-scope changes are present: `HARNESS_PREFIXES` includes workflow/helper/test roots; `GO_PREFIXES` includes runtime/test assets. Preserve these rather than reimplement their selection. Adjacent four-worker activation is already reflected in `GATES`.

A disposable synthetic local check executed actual `gofmt -l .`: an unformatted `.claude/data/private.go` was listed, while `.hidden.go` was not. The installed Go source's `cmd/gofmt/gofmt.go`, `isGoFilename` and `gofmtMain`, uses recursive `filepath.WalkDir` without pruning dot directories; it excludes dot-prefixed *file basenames*. Tool qualification must repeat this probe with the pinned real executable. Marker exclusion and actual consumer input selection are different.

**Scope: single spec, seven dependent harness tasks, auto-confirmed (non-interactive).** Contract, capture, enforcement and shared consumers form one result-validity boundary. None independently enables a strong profile; the legacy path remains useful throughout. No application Go, product-state, orchestration, CI configuration or native/provider change. The full historical executable inventory is preserved; only Linux/amd64 `go-fmt` can earn stronger evidence.

## 2. Candidate and consumer input contract (FR-1/3/4; charter 1/2/6)

A candidate is an immutable *description of requested bytes*, not HEAD alone and not a claim that a dirty checkout occupied one atomic historical state. Introduce a two-stage explicit CLI:

```text
python3 scripts/harness/gatelib.py candidate --gate go-fmt --root DIR --json
scripts/harness/gate.sh go-fmt --mode immutable-qualified --candidate DIGEST --profile DIGEST
python3 scripts/harness/gatelib.py status --policy immutable-required --json
```

`candidate` writes a private versioned descriptor and prints its digest plus sanitized relative manifest reference; root defaults to `.` as existing commands. It observes Git HEAD/base/index identity, inventories intended paths, hashes bytes and records an observational descriptor. This command earns no pass. Caller review/selection of this immutable descriptor establishes the requested identity. Strong run requires an existing descriptor belonging to the exact tree; it never silently replaces it with newly observed bytes. Existing `run` without mode retains explicit `legacy-observed` defaults. Explicit strong mode accepts only the single gate `go-fmt`; groups/multiple gates or another platform refuse `unsupported-profile` without running a weaker substitute. `--profile` selects a prequalified private receipt, not a self-issued grant.

Input policy `linux-amd64-go-fmt-input-v1`:

1. Use a separate strong-path incremental observer for the same fixed NUL-delimited Git inventory (`git -C ROOT ls-files -z --cached --others --exclude-standard`), as specified below; never call existing buffered `listed_files` or its `git` helper on this path. Include every tracked/nonignored untracked regular file, all required ancestor directories and explicit tombstones for tracked deletions. Do not scope-filter this copy with `in_scope('go')`: the actual tool walks beyond those prefixes. `.git` metadata and ignored runtime/configuration bytes never enter the copy. A tracked path under an otherwise ignored directory remains included.
2. Perform a separate bounded directory-entry walk of the original root, including ignored/dot directories, to establish the actual gofmt selector closure: every reachable non-directory basename not beginning `.` and ending `.go`. It examines names/types, not ignored file contents. If a selected path is not in the requested candidate inventory, refuse `unmodeled-consumer-input`; do not copy it or silently omit its effect. This includes ignored private `.go` files and any `.go` inside raw `.git` metadata. An unreadable/vanished traversal node refuses. No implicit `.git` or dot-directory skip in this closure walk. A worktree `.git` file is metadata, not a selected Go input.
3. Reject links (including directory links), special files, junction-equivalent unsupported nodes, non-UTF-8 names, absolute/empty/dot/dot-dot components, control characters, names over 4096 UTF-8 bytes, depth over 64 components, or collisions after NFC normalization plus casefold. Linux path bytes are retained as UTF-8 without rewriting names; normalization is for collision refusal. Every inspected node is bounded; the closure walk allows at most 40,000 nodes, including ignored metadata, and shares the 30-second preparation deadline. An oversized ignored subtree may make strong verification unavailable; legacy checking remains explicit.
4. Tool-selected set in the finalized copy must exactly equal the requested selected set. A qualified real-tool selection fixture contains ordinary, dot-directory, dot-basename, untracked and ignored inputs. Empty selected sets are permitted as actual gofmt semantics, but qualification and regression controls must contain at least one effective Go input.

### Bounded strong Git observations

At this baseline, `gatelib.git` (`gatelib.py:112–114`) uses `subprocess.run(capture_output=True, timeout=120)` and `listed_files` (lines 137–139) consumes its fully buffered stdout. That behavior remains unchanged for legacy callers. It is unsuitable for strong preparation limits; wrapping it after return would not enforce either the 20,000-entry cap or 30-second deadline.

The strong candidate/run/status-comparison path dispatches to `verification_capture.observe_git` **before** using legacy `toplevel`, `default_base`, `listed_files`, `changed_files`, `fingerprint` or any other helper that could invoke buffered Git. `observe_git` accepts only the fixed read-only subcommands needed here: root/common-dir/index-path resolution, HEAD and origin/main verification, merge-base, registered-worktree inventory, and tracked/nonignored-path inventory. No candidate-supplied argv, shell, external diff, pager, hook, filter or network operation is admitted. Resolve root with bounded `rev-parse --show-toplevel`; resolve required HEAD and default base (verified origin/main merge-base, otherwise HEAD) through the same observer. Registered-worktree checks for outside-worktree store placement use bounded `worktree list --porcelain -z`. Root resolution and those checks are not unbudgeted prerequisites.

Create one `PreparationBudget` at entry to each candidate observation, strong run preparation, or strong current-candidate comparison, before the first Git/helper/filesystem observation: monotonic deadline `now + 30s`, shared cancellation predicate and clock. Thread that same object through root/store checks, every Git child, streamed index hashing, inventory, copy, re-enumeration and rehash. It is never reset between commands or phases. Candidate selection and a later run are distinct invocations, each bounded; no previous candidate observation extends a run's budget. The execution's separate 10-second deadline begins only after bounded preparation is finalized. Expiry/cancellation stops useful preparation immediately; the fixed 2-second TERM plus at-most-5-second forced-join/reconciliation allowance is additional cleanup time, never permission for more preparation or acceptance.

`observe_git` uses `Popen` with stdin DEVNULL, separate stdout/stderr pipes, `start_new_session=True`, no inherited nonessential FDs and the sanitized Git environment. A selectors loop reads at most 64 KiB per operation, checks cancellation/deadline before every read and parse, and waits no longer than the smaller of 50 ms or remaining preparation time. It parses NUL records incrementally for inventory; count every emitted path record **before deduplication**, refusing record 20,001. Never build an unbounded stdout blob or wait for EOF before checking limits. The pending record is at most 4096 bytes; the first additional byte or a missing final terminator refuses. Inventory stdout is at most 8 MiB in aggregate, stderr at most 64 KiB; crossing either cap terminates the owned child rather than continuing to buffer or discard an unlimited stream. Output is private and sanitized diagnostics contain codes, not raw stderr. Returned parsed records occupy no more than those caps. Record/byte boundaries are checked even if the child has already exited successfully; nonzero exit, decode error or truncated record refuses, except the explicitly described optional-ref absence result below.

Metadata commands use the same incremental process runner with newline delimiter, at most one record, 4096-byte pending-record and 4097-byte stdout caps; OID commands tighten these to 128 bytes and validate exact SHA-1/SHA-256 hex shapes. Only `rev-parse --verify -q origin/main` may translate exit 1 with empty stdout into an empty record list (optional ref absent, selecting required verified HEAD as base); timeout, cancellation, malformed data or any other nonzero status must not trigger that fallback. Worktree inventory uses NUL records, at most 1024 records and 256 worktrees, 4096 bytes per record and 1 MiB aggregate stdout; overflow makes placement verification unavailable. `Limits` carries these fixed observer caps so reduced-cap tests can exercise them; candidate data cannot raise them. Index resolution is bounded, then its regular no-follow file is hashed incrementally in at-most-1-MiB chunks with an 8-MiB cap, shared deadline/cancellation and pre/post identity checks; oversized/missing/changed index refuses. No raw index bytes enter a snapshot. Other metadata files traversed by this path obey the same bounded metadata-record rule.

Any overflow, interruption, blocked child or deadline expiration follows owned-process teardown: record PID/start identity/pidfd, TERM owned session/process group, wait at most 2 seconds, then KILL and join/reconcile for at most 5 seconds. Drain neither pipe without a bound. Account/close all descriptors in `finally`; process leader exit without confirmed descendant/pipe-owner teardown is not sufficient. A child or ownership state that remains unresolved yields `ownership-unknown`, retains a bounded private reconciliation record and prohibits accepted markers and destructive cleanup. This also applies before a snapshot directory has been allocated. Return `limit-exceeded`, `timeout`, `cancelled` or `io-error` only after confirmed teardown, preserving any allocated store ownership disposition. No Git child may survive silently behind a successful descriptor or result.

The requested descriptor contains full included source bytes' hashes, type, executable bit and tombstones; `candidate_digest` hashes this canonical description plus Git/head/base/index observation, policy and gate-definition digests. Git observations are provenance only. Changes of original index/refs after descriptor selection cannot change the copied bytes or launch Git inside the child. Strong gofmt uses no Git mount and never copies credential-bearing Git config. Future Git-query gates are `unsupported-profile`; tests execute a genuine Git consumer on the legacy path and show strong refusal rather than substitute its command.

Capture opens the root directory once, traverses relative dirfds with `O_NOFOLLOW|O_DIRECTORY`, opens files with `O_NOFOLLOW`, verifies regular type and link count 1, and streams into newly created `O_EXCL` owned files. Pre/post `fstat` identity/size/mtime/ctime and revalidation of parent/path identity detect replacement, shrinking/growing files or unsupported aliases. Materialized bytes are hashed during copy and rehashed from the finalized independent view. Both must equal the *previously selected* descriptor. Re-enumerate inventory, closure and Git observations before finalization; any observable discrepancy refuses. These checks reject ambiguous captures; they do not prove continuous quiescence of the original. A transient that evades observations still cannot credit different executed bytes: the finalized content digest must equal the caller's pinned descriptor. A mixed capture can only be identified as its actual descriptor, never credited as another requested candidate or as a clean Git commit.

No hardlinks/reflinks back to source; no raw Git directory copy; no source mutation. A formatter with `-w` remains an explicit external transform followed by a new descriptor. Strong profile rejects changed argv, so a transform cannot reuse a pre-transform pass.

## 3. Private records and Python APIs (FR-1/5/7; I07/I09/I20)

New modules live beside `gatelib.py`, use Python 3.10 standard library, and expose keyword-only parameters where shown. `verification.py` owns frozen dataclasses and strict codecs; imports never execute tools. All codec objects reject duplicate/unknown keys, unknown schema/version, invalid enums/nonfinite values, over-limit arrays/strings and trailing data. JSON canonicalization is UTF-8, sorted keys, compact separators, no NaN; SHA-256 digest excludes only its own self field (`candidate_digest`, `manifest_digest`, `profile_digest` or `execution_digest` by record kind), never referenced digests; kinds without a self field hash every field. Private records are at most 8 MiB, entries at most 20,000 and individual string fields at most 4096 bytes. Public output never dumps records.

```python
class VerificationError(Exception):  # .code: stable enum; .detail: sanitized text
    pass
@dataclass(frozen=True)
class Limits:
    files: int = 20000; source_bytes: int = 67108864
    file_bytes: int = 8388608; copy_buffer: int = 1048576
    depth: int = 64; walk_nodes: int = 40000
    prepare_s: float = 30.0; execute_s: float = 10.0
    output_bytes: int = 4194304; scratch_bytes: int = 16777216
    git_stdout_bytes: int = 8388608; git_stderr_bytes: int = 65536
    git_record_bytes: int = 4096; index_bytes: int = 8388608
    git_metadata_stdout_bytes: int = 4097; git_metadata_records: int = 1
    git_oid_bytes: int = 128
    worktree_records: int = 1024; worktrees: int = 256
    worktree_stdout_bytes: int = 1048576
@dataclass(frozen=True)
class PreparationBudget:
    deadline: float; cancelled: object; clock: object
    def remaining_s(self) -> float: ...
    def check(self) -> None: ...  # raises timeout/cancelled immediately
@dataclass(frozen=True)
class Candidate:
    digest: str; record: dict
@dataclass(frozen=True)
class Snapshot:
    snapshot_id: str; manifest_digest: str; source_dir: str; record: dict
@dataclass(frozen=True)
class Profile:
    digest: str; record: dict
@dataclass(frozen=True)
class Execution:
    disposition: str; exit_code: int | None; record: dict
@dataclass(frozen=True)
class MarkerVerdict:
    state: str; mode: str; accepted: bool; reason: str

def encode_record(record: dict) -> bytes: ...
def decode_record(data: bytes, *, kind: str) -> dict: ...
def record_digest(record: dict) -> str: ...
```

The dictionaries have the exact closed schemas below; dataclasses carry validated copies, not unchecked external objects. `kind` dispatches one of `candidate`, `manifest`, `profile`, `execution`, `ownership`, `marker`, `probe`; no generic arbitrary object acceptance.

| Record | Exact fields (in addition to `schema_version: 2`, `kind`) |
|---|---|
| candidate | `candidate_digest`, `tree_id`, `head`, `base`, `index_digest`, `input_policy`, `gate`, `gate_definition_digest`, `entries`, `selected_paths`, `closure_digest` |
| manifest | `manifest_digest`, `candidate_digest`, `snapshot_id`, `tree_id`, `input_policy`, `gate_definition_digest`, `entries`, `selected_paths`, `source_digest` |
| profile | `profile_digest`, `gate_definition_digest`, `profile_id`, `platform`, `helper`, `tool`, `namespace_argv`, `environment`, `write_envelope`, `limits`, `qualification_digest`, `qualification`, `disposition` |
| execution | `execution_digest`, `gate_definition_digest`, `snapshot_id`, `manifest_digest`, `profile_digest`, `argv`, `started_at`, `finished_at`, `duration_s`, `exit_code`, `stdout_digest`, `stderr_digest`, `stdout_bytes`, `stderr_bytes`, `stdout_stripped_empty`, `disposition`, `reason` |
| ownership | `snapshot_id`, `owner_token`, `controller_pid`, `controller_start`, `child_pid`, `child_start`, `state`, `created_at`, `terminal_at`, `reserved_bytes`, `retained_bytes`, `references` |
| marker | `gate`, `scope`, `tree`, `mode`, `candidate_digest`, `manifest_digest`, `snapshot_id`, `profile_digest`, `gate_definition_digest`, `execution_digest`, `exit_code`, `stdout_stripped_empty`, `started_at`, `finished_at` |

Entries are sorted by UTF-8 path bytes; each has exactly `path`, `type` (`regular`, `directory`, `deleted`), `executable` (boolean), `size` and `sha256` (null for directory/deleted). `selected_paths` is sorted and references existing regular entries. `gate_definition_digest` hashes canonical gate name/scope/argv/empty-output predicate plus SHA-256 of the actual shared engine and all four verification implementation modules, and the input-policy revision. Recompute on every run and consumer evaluation; a code change invalidates old evidence without executing the candidate's changed checker. This hashes dependency definitions as observed under the trusted-controller assumption, not as hostile-host attestation. `tree_id`/marker `tree` is the canonical private originating-root identity; never export it. `head` and `base` are exact object IDs or explicit null when unavailable; unresolved required base/HEAD on the initial Git profile refuses candidate creation. `index_digest` observes the index bytes without modifying them. `source_digest` hashes the materialized entries including tombstones. `closure_digest` binds the sorted selector closure; no ignored contents are retained.

Helper/tool identities contain exactly `sha256`, `version` (string or null), `version_kind` (`observed`/`unknown`) and the key `artifact_ref` (a private store-relative, no-traversal artifact reference). Required strong helper/tool versions cannot be unknown. `environment` is the exact safe whitelist described in §5; never ambient environment values. `namespace_argv` contains normalized private artifact references and fixed mount destinations. `qualification` is a closed nested object with exactly `gate_definition_digest`, `plan_digest`, `kernel_release`, `filesystem_type`, `mount_options`, `architecture`, `probe_results`; `probe_results` contains exactly the seven fixed probe IDs in §5 (the unavailable probe records successful refusal controls). Each result has `outcome` (`pass`/`fail`/`unavailable`), `expected`, `actual`, `evidence_digest`; the latter hashes a private bounded probe artifact loaded through the store. `qualification_digest` hashes this object. Probe artifacts have codec kind `probe`, fields `schema_version:2`, `kind:'probe'`, `probe_id`, `gate_definition_digest`, `plan_digest`, `outcome`, `exit_code`, `stdout_digest`, `stderr_digest`, `observations`; observations use the probe-specific closed fields defined in §5. Canonical binding order is acyclic: compute current `gate_definition_digest`; construct the provisional plan from exactly `schema_version`, `kind:'profile-plan'`, `gate_definition_digest`, `profile_id`, `platform`, `helper`, `tool`, `namespace_argv`, `environment`, `write_envelope`, `limits`; hash that object as `plan_digest`; generate definition-bound probe artifacts; hash the qualification object containing their evidence digests; finally hash the complete qualified profile receipt. The provisional plan omits `profile_digest`, `qualification_digest`, `qualification` and `disposition`, so neither it nor a probe depends on the final profile digest. The plan is a closed internal canonical object, not another accepted top-level codec kind. Every probe explicitly carries both the current definition and plan digest, and qualification explicitly carries the same pair. Resolve and recompute the full chain on load; missing or mismatched definition/plan/probe/qualification bindings refuse. No arbitrary plugin check IDs are accepted.

`platform` is exactly `linux/amd64` for the strong profile. `write_envelope` has exactly `source_readonly:true`, `scratch_bytes:16777216`, `output_bytes:4194304`, `git_view:'none'`, `network:'none'`, `host_trust:'trusted'`; `limits` has exactly the `Limits` fields above. `namespace_argv` is a bounded string array (at most 128 items), `environment` the closed string map in §5. In addition to digest equality, a qualified profile requires all seven fixed probes to have outcome `pass`; fail/unavailable never qualifies or satisfies a consumer. Profile disposition is `qualified` or `unavailable`; execution disposition is `succeeded`, `failed`, `cancelled` or `quarantined`. Only succeeded can produce an accepted marker. Times are UTC RFC3339 strings; durations are finite nonnegative seconds. `references` is a bounded sorted list of marker identities (tree/gate pairs), and ownership nullable process fields remain null until known. `reason` is a code from the typed error enum or null for success, never raw exception text.

Marker modes are `immutable-qualified`, `observed-copy`, `legacy-observed`. An observed copy is supported for *reading* limited evidence, not an automatic executor fallback. Schema-2 observed modes may have null snapshot/profile bindings only as explicitly described by their codec branch; schema-2 immutable requires every binding, exit 0 and empty stripped stdout. Schema 1 is decoded only with the explicit legacy policy: require its existing acceptance fields (`schema_version`, `gate`, `tree`, `fingerprint`, `exit_code`), permit baseline producer provenance fields (`scope`, `head`, `command`, `started_at`, `finished_at`, `duration_s`), and never infer immutable bindings from them. Reject unknown schema-1 keys rather than treating them as new authority. Preserve existing baseline acceptance semantics for optional provenance fields. Missing/corrupt artifacts or any changed critical binding refuse acceptance.

New exact interfaces:

```python
# verification_store.py
class Store:
    def reserve(self, *, source_bytes: int, limits: Limits) -> str: ...
    def load(self, digest: str, *, kind: str) -> dict: ...
    def save(self, record: dict, *, kind: str) -> str: ...
    def transition(self, snapshot_id: str, *, expected: str, target: str,
                   updates: dict) -> None: ...
    def reconcile(self, snapshot_id: str) -> str: ...
    def collect(self, *, now_s: float) -> list[str]: ...
def open_store(root: str, *, budget: PreparationBudget,
               worktree_roots: tuple[str, ...]) -> Store: ...
# verification_capture.py
# Every observer shares the entrypoint's monotonic deadline and cancellation.
# argv selects one fixed allowlisted read-only Git subcommand, never user text.
def observe_git(root: str, *, argv: list[str], budget: PreparationBudget,
                limits: Limits) -> list[bytes]: ...
# Test-only barriers are explicit injectable callbacks.
def observe_candidate(root: str, *, gate: str, gate_definition: str,
                      store: Store, limits: Limits, budget: PreparationBudget) -> Candidate: ...
def capture(root: str, candidate: Candidate, *, store: Store,
            limits: Limits, budget: PreparationBudget, barrier=None) -> Snapshot: ...
def compare_candidate(root: str, candidate: Candidate, *, limits: Limits,
                      budget: PreparationBudget) -> bool: ...
# verification_linux.py
# qualification creates evidence only after actual executable/mount probes pass.
def qualify_profile(*, helper: str, tool: str, expected_helper_sha256: str,
                    expected_tool_sha256: str, store: Store,
                    limits: Limits, gate_definition_digest: str) -> Profile: ...
def execute(snapshot: Snapshot, profile: Profile, *, argv: list[str],
            store: Store, limits: Limits, cancelled,
            gate_definition_digest: str) -> Execution: ...
# gatelib.py additions; existing run_gate and marker_state remain callable.
def evaluate_marker(root: str, gate: str, fps: dict, *, policy: str) -> MarkerVerdict: ...
def marker_state(root, gate, fps, *, policy='legacy-observed'): ...
def run_gate(root, gate, *, mode='legacy-observed', candidate=None, profile=None): ...
```

All operations raise `VerificationError` with one of `invalid-record`, `wrong-tree`, `candidate-mismatch`, `unmodeled-consumer-input`, `unsupported-path`, `limit-exceeded`, `capture-changed`, `store-full`, `busy`, `profile-unavailable`, `tool-drift`, `cancelled`, `timeout`, `output-overflow`, `ownership-unknown`, `cleanup-failed`, `unsupported-profile`, `io-error`. They never turn these into passes. Capture/cancel errors persist a private ownership disposition when allocation already occurred. A null tool version is representable observation, never strong qualification.

## 4. Ownership, budgets and retention (FR-6; I08/I21)

`open_store` chooses the user's private state directory `mythhelm/verification` beneath platform state home, creates it mode 0700, files 0600, and refuses any symlink or store nested in *any* registered Git worktree. It does not reuse `.claude/data` for snapshot bytes: actual gofmt would recursively discover retained `.go` copies there. Marker metadata remains at the existing private worktree location and contains JSON only. Tree identity remains in private records. The strong entrypoint resolves `worktree_roots` through the bounded observer using the same preparation budget before calling `open_store`; the store validates that finite observed root set and performs no Git subprocess itself. Task 2 tests supply explicit synthetic root sets; Task 3 supplies the bounded observation and Task 5 joins them, avoiding a contract dependency cycle. Controller uses a global store `flock` with nonblocking admission; one capture/execution is active at a time. This is a cooperative controller lock, not hostile-host exclusion.

States: `reserved -> capturing -> finalized -> executing -> succeeded|failed|cancelled`; any unresolved interruption goes to `quarantined`. Failure before allocation leaves no artifacts. Each transition is atomic JSON replacement plus file/directory fsync, compare-state guarded by lock. Stable random snapshot ID and 256-bit owner token bind owned directory creation; only owned records and open no-follow dirfds authorize cleanup. Directory ownership/mode and token are rechecked. A crashed controller never blindly removes a child or retries execution; reconciliation verifies PID start identity/pidfd, namespace/child exit and actual filesystem ownership first. Unknown process identity remains quarantined. Receipts remain private harness evidence, not a product task store.

Enforce all fixed requirements limits incrementally, including tombstones in entry count. File bytes ≤8 MiB, present aggregate source bytes ≤64 MiB, 1 MiB buffer, 20,000 entries; depth 64 and 40,000 inspected nodes are conservative extra bounds. Snapshot preparation has one 30-second deadline from before root resolution across bounded Git/HEAD/base/index/worktree observations, copy, rehash and closure; §2 specifies incremental output/record limits and teardown. Legacy Git helpers retain their existing behavior and are never called from this strong path. Execution has 10 seconds; combined raw stdout+stderr cap 4 MiB, bounded by selector-driven reads, not `communicate()` with unlimited buffering. At cap overflow stop the child, refuse a marker and retain bounded output only privately.

Store capacity is 64 snapshots and 10 GiB total retained/reserved bytes, including descriptors, staging, output, tool copies and quarantined content. Admission reserves a conservative bound for source, up to two 8 MiB metadata records, copied tool/helper artifacts (each at most 64 MiB), 16 MiB scratch and 4 MiB output before allocation; account actual bytes and keep reservation until terminal reconciliation. No abandoned-artifact exemption. The tmpfs scratch has a hard 16 MiB size, a finite observed inode ceiling from the supported kernel tmpfs mount configuration and is destroyed only after child reconciliation. Profile artifacts/records count against the same byte cap even if shared. Limits cannot be raised by a candidate.

Cancellation sends TERM to the owned bwrap process group, waits at most 2 seconds, then KILL; allow at most another 5 seconds for join/reconciliation. Record PID/start identity and pidfd before execution. The qualified PID namespace/reaper must establish descendant teardown; leader exit alone is insufficient. If it cannot, quarantine and refuse acceptance/cleanup. No source/index/ref mutation; external fixture edits are attributed separately.

Successful unpinned terminal snapshots expire after 24 hours, failed/cancelled terminal snapshots after 7 days; active, quarantined, unknown-owner or referenced snapshots never expire automatically. Markers reference snapshots while accepted; replacing/clearing a marker reconciles its reference count under the lock. `collect` deletes only terminal unreferenced owned directories using no-follow dirfds, never following foreign links. Capacity that cannot safely be reclaimed blocks admission. `reconcile` is idempotent and never re-executes a gate.

## 5. Qualified Linux execution (FR-2/3/7; v2 §8.1)

Profile ID `linux-amd64-go-fmt-ro-v1` supports only Linux x86-64 and a qualified local filesystem/mount configuration recorded in its receipt. Pin actual helper and gofmt SHA-256 plus observed versions and the complete profile/definition digest. Helper path must be root-owned and not group/other writable, following the established lane check. Copy the verified actual gofmt executable into the owned tool artifact; hash before/after copy and execute that artifact. Initial tool profile requires an ELF64 x86-64 gofmt without PT_INTERP; dynamic-tool installations are unavailable until separately qualified. No wrapper/compiler/tool stand-in qualifies it. The trusted helper/toolchain/controller and host remain assumptions.

Qualification is an explicit controller operation, not implicit from PATH discovery; no package installation/download or configuration approval is inferred. A private qualified profile receipt must already match reviewed expected helper/tool digests. Implementing tests may qualify the installed real pair for the disposable synthetic host only; such evidence does not auto-enable a production profile. Missing supported kernel features, helper, static tool identity or qualification yields `profile-unavailable`. Recompute current `gate_definition_digest` independently from the selected candidate and compare it to the profile, provisional plan, every probe and qualification object before any reuse, execution or accepted consumer result. The manifest, execution and marker must carry that same digest. A newly observed candidate under a changed checker cannot reuse an old qualification even if tool versions, source bytes, argv and endpoint fingerprints are unchanged. `qualify_profile` checks its supplied digest against the current shared definition, and `execute` rechecks the same current digest rather than trusting caller input; mismatch yields `profile-unavailable` before execution. Recheck executable identities and a bounded mount/write probe at every run; changed versions/digests invalidate reuse.

Build a fresh bwrap root without binding `/` or host homes. Fixed namespace flags: `--die-with-parent --unshare-user --unshare-pid --unshare-ipc --unshare-uts --unshare-net --new-session --cap-drop ALL --disable-userns --assert-userns-disabled --clearenv`. Mount finalized snapshot read-only at `/input`, copied gofmt read-only at `/tool/gofmt`; private procfs at `/proc`, minimal devices at `/dev`, size-limited tmpfs at `/scratch`. Prepare scratch home/tmp/config/cache directories. Remount namespace root read-only after creating mountpoints (the supported helper's nonrecursive remount retains declared scratch writable). No mount exposes the original tree, store root, Git metadata, helper host path, host temporary files or another writable alias of source. Close inherited file descriptors except bounded stdin/stdout/stderr and explicitly necessary bwrap status descriptors. Child stdin is `/dev/null`.

Set only `PATH=/tool`, `HOME=/scratch/home`, `TMPDIR=/scratch/tmp`, `XDG_CONFIG_HOME=/scratch/config`, `XDG_CACHE_HOME=/scratch/cache`, `GOTELEMETRY=off`, `LANG=C`, `LC_ALL=C`, `GOMAXPROCS=1`. Tool telemetry initialization is a real write consideration even for gofmt; no ambient configuration or telemetry directory is mounted. Record whitelist values, not raw environment. Actual shared argv remains `gofmt -l .`, resolved by fixed PATH to the pinned real artifact, cwd `/input`. Namespace wrapper argv and tool-resolution mapping are separately bound; do not falsely claim the wrapper itself equals the gate argv. Accept only exit 0 and empty stripped stdout, exactly as baseline; parser errors and formatting listings fail.

Fixed qualification probe set (seven IDs, each nonempty; all required):

- `real-command`: formatted source passes actual `gofmt -l .`; unformatted source exits 0 but lists a file and the shared acceptance predicate rejects it; malformed source produces failure.
- `selector-closure`: real dot-directory/untracked input is seen, dot-basename input is skipped, ignored selected input makes admission refuse before omission. Compare expected selection to actual real-tool output using deliberately unformatted fixtures.
- `source-write-denied`: in the same namespace/mount plan, actual `gofmt -w /input/unformatted.go` fails and source digest is unchanged. This is a boundary probe, never the accepted gate command.
- `scratch-write-allowed`: actual `gofmt -w /scratch/unformatted.go` succeeds and changes bytes; fixtures are provisioned with a bounded bwrap input FD into scratch. This distinguishes an enforcing boundary from a broken tool.
- `no-source-alias`: inspect the child's mount plan/status and verify no source writable bind/FD alias; attempt replacement/creation through every declared input exposure using real-tool write probes. Missing mount observations refuse qualification.
- `isolation-unavailable`: missing helper, denied userns, mount/probe failure or unqualified filesystem never emits immutable evidence; deliberate refusal controls must pass on an otherwise qualified host.
- `pidns-teardown`: verify actual child exit during cancellation with bounded deterministic barriers. Real tool/profile support never comes from orchestration stand-ins.

Probe observation shapes are closed by ID: `real-command` and `selector-closure` use `cases` (fixed case IDs with expected/actual exit, selected paths and output predicate); `source-write-denied`/`scratch-write-allowed` use `before_digest`, `after_digest`, `write_refused`; `no-source-alias` uses `mount_plan_digest`, `observed_mounts_digest`, `writable_aliases` (must be zero); `isolation-unavailable` uses `cases` (fixed missing-helper/denied-userns/failed-mount/failed-write cases and refusal codes); `pidns-teardown` uses `term_sent`, `kill_sent`, `joined`, `descendants_gone`, `elapsed_s`. Arrays are bounded by the fixed case counts, selected paths by 20,000 and mount observations by 128. Each case table is enumerated in the qualification code, not supplied by a candidate. Artifacts contain sanitized values/digests, never arbitrary raw output; raw output remains separately private and bounded.

Probe records bind kernel release, architecture, filesystem type/mount options, helper/tool identities, exact namespace/environment, gate definition and tested dispositions. These are observed support facts for a trusted host, not protection against hostile same-user processes outside the child, administrators or altered receipt writers. Endpoint rehash of snapshot supplements the mount guarantee; it does not create the guarantee.

## 6. Shared result consumers and rollout (FR-5/8)

`evaluate_marker` is the only acceptance implementation. Policies are exactly `legacy-observed` and `immutable-required`; callers must choose explicitly. Legacy accepts valid schema 1 as observational, and valid schema 2 observed-copy or immutable-qualified matching the selected candidate. Immutable-required accepts only schema-2 immutable-qualified with complete profile, manifest, execution, tree/scope/gate/check bindings and retained artifacts, including recomputed current definition equality across candidate/manifest/profile/provisional-plan/probes/qualification/execution/marker. Old qualification paired with a newly generated candidate after actual checker-code change refuses under both policies for schema-2 immutable evidence; it is never downgraded to observational success. Wrong/corrupt/missing evidence is never downgraded. For schema 1, legacy policy retains the existing observed scope-fingerprint comparison; for schema 2, both policies compare the *current observed candidate descriptor* against the recorded candidate for relevance; that observation says nothing about intermediate worktree reads or future edits.

`MarkerVerdict.state` is `fresh`, `stale`, `missing`, `unsupported`, or `invalid`; mode remains distinct. `accepted` is true only for fresh evidence satisfying the selected policy. Compatibility `marker_state` returns `fresh` only when accepted, otherwise `missing` for missing and `stale` for every refusal. This preserves existing Python callers while status/health now expose richer fields. `fps` caches only one invocation's observations; strong comparisons use a separate key `(gate,input_policy)` and never reuse an old scope fingerprint as a manifest digest.

Default `status`/stop-hook/health explicitly use `legacy-observed`, preserving all required current gates; status supports `--policy immutable-required`. JSON rows have exactly `gate,state,mode,accepted,reason,comparison` with comparison `observed-current-candidate` or `unavailable`. Text labels include `fresh (legacy-observed)` / `fresh (immutable-qualified; current comparison observed)`; unknown/unsupported remain visible. Health counts each mode separately; it does not call all fresh results immutable. Stop-hook retains existing override/repeat behavior and labels unmet policy; hook adapter invokes the shared checker. No PR/acceptance authority is added to runspec/finalize: their procedures can explicitly invoke strict status when needed, but absence of a direct reader is preserved.

`run --mode legacy-observed` retains existing command semantics and schema-1 producer compatibility; its printing explicitly labels observational mode. `run --mode immutable-qualified` clears any prior target gate marker before setup, validates selected candidate/profile, captures, executes and writes schema 2 atomically only after final success and complete evidence. Clear prior marker for failed, interrupted, unsupported, changed, timed-out or output-overflow runs. Do not publish a stale old success after a failed attempt. Result publication and snapshot reference transfer occur under the store lock; failed marker-write reconciles orphan references. No accepted marker can outlive a missing referenced manifest/execution/profile.

The deterministic regression keeps the original Python read-barrier consumer as a real fixture consumer dispatched through the shared engine, clearly distinguished from production gofmt qualification. For actual gofmt, coordinate capture/finalization/launch barriers: a requested BAD descriptor plus GOOD capture refuses before launch; a separately requested GOOD materialization passes actual gofmt while original is restored to BAD, and strict status for BAD refuses it. No fake gofmt wrapper or inference/compiler substitute supplies a read barrier. Quiescent actual-tool GOOD/BAD controls and the baseline real fixture-consumer transient read are separate nonempty observations.

## 7. Decisions and coverage

| ID | Decision | Alternative and rationale |
|---|---|---|
| D1 | Explicit requested candidate descriptor before copy | Inferring candidate from HEAD or post-copy hashes could credit different dirty bytes. |
| D2 | Refuse ignored gofmt-selected files | Copying private ignored state violates privacy; omitting it changes real command semantics. |
| D3 | Minimal read-only bwrap input, real static gofmt | Writable copies/modes do not enforce immutability; vendor mounts expose unrelated state. |
| D4 | Explicit optional qualification, fail closed | PATH availability or a fixture substitute cannot establish the supported profile. |
| D5 | One shared policy evaluator with legacy compatibility | Separate client readers drift; mandatory strong mode for every current gate would drop unsupported checks. |
| D6 | Store outside all worktrees | Ignored snapshot `.go` files inside a tree are still real gofmt inputs. |
| D7 | Seven tasks, one harness delivery | Contracts, probes and consumers must agree before the profile is usable; documentation describes shipped mechanism. |

| Requirements | Owning tasks / observation |
|---|---|
| AC-1.1–1.3 | 1/3/6: immutable descriptor bindings, replacement refusal, real transient-read regression |
| AC-2.1–2.3 | 3/4/6: independent copy, actual read-only mount, no weaker relabel |
| AC-3.1–3.3 | 4/5/6: exact gofmt argv/output, transform refusal, actual source/scratch probes |
| AC-4.1–4.3 | 3/6: untracked/deleted/ignored closure, unsupported links, real Git-dependent refusal |
| AC-5.1–5.3 | 1/5/6: strict markers, central consumers, failed-run invalidation |
| AC-6.1–6.3 | 2/3/4/6: fixed limits, capacity/retention, bounded cancel and ownership cleanup |
| AC-7.1–7.2 | 1/2/4/6: safe profile identity, private bounded artifacts and synthetic sentinels |
| AC-8.1–8.2 | 5/6/7: shell/shared-reader compatibility, explicit partial rollout and docs |
| NFR-1–3 | 4/6/7: real tools, deterministic barriers, exact subset and sanitized examples |

## 8. Honesty register

| Spec demand | Position |
|---|---|
| Charter 1/2/6, I07/I20: exact immutable execution | Planned initial Linux/amd64 gofmt profile only; actual helper/tool/kernel/filesystem qualification is implementation work and may remain unavailable. |
| I09: endpoint comparisons | Observational relevance checks; no continuous original-tree equality or atomic historical dirty-tree claim. |
| v2 §8.1: trust boundary | Trusted host/controller/helper/toolchain; no hostile-host or same-user external-process attestation. |
| FR-3/4: full gate universe and Git inputs | Other gates, dynamic tools, transforms, source-writing tests, Git-dependent commands and platforms remain legacy-observed or strong-unavailable pending separate reviewed qualification. |
| I08/I21: ownership/bounds | Cooperative single-controller admission plus qualified child teardown; unresolved ownership quarantines artifacts and may exhaust capacity. |
| I07/I23: product acceptance | Harness receipts only; no replacement for product protected checks or canonical orchestration. |
| Charter 5 / FR-7: privacy | Private snapshots can contain intended repository source; no export/raw environment/config copies. Redaction cannot certify arbitrary logs. |
| WORKFLOW required checks | All existing groups/checks retained with current argv, resource limits and explicit observed labels; no native/provider calls or paid review required. |
