## Dogfood Slice — Requirements

> First runnable MYTHHELM: one native Claude Code attempt, from a task file to a verified, reviewable, explicitly applied candidate. A vertical slice of master spec Stage 1 (§20.3, §22.2), not all of it. Normative source: [`docs/spec/master-spec.md`](../../../docs/spec/master-spec.md) Revision 1.1; section numbers (§) and invariant IDs (I01–I19) and gates (G01–G12) refer to it.

### Objectives

- **O1**: The maintainer can use `mythhelm run --task-file task.md` to develop MYTHHELM itself, with Claude Code doing the work, MYTHHELM's configured checks deciding verification, and a receipt explaining what happened.
- **O2**: Every safety invariant the slice touches is either enforced or blocked with a labelled reason. The slice narrows scope; it never quietly narrows an invariant.
- **O3**: The full test suite and `mythhelm demo` run offline, with no credentials, on Linux, macOS and Windows CI (G01).

### Non-Goals (out of scope for this slice, and what still binds)

Each item is deferred, not waived. The invariant or gate on the right still applies to anything this slice ships.

| # | Out of scope | Still binding in this slice |
|---|---|---|
| N1 | TUI (Bubble Tea mission view, §15.3–15.9). Next slice. | Plain/JSONL output must stay truthful (A20, §15.4 non-TTY row); `--plain` is accepted now so scripts do not break later. |
| N2 | Herdr bridge, `--host`, pane projection (§16.6). | I17, I18: nothing in the slice may create a second process owner or writer. `--host herdr` exits 7 (capability unavailable). |
| N3 | Routing across profiles, `--optimise`, `--profile`, learning (§8). | I04, I16: a single adapter is pinned per run by explicit `--adapter`; there is no fallback to any other adapter. |
| N4 | More than one writer, task DAG, integration train steps 4–6 (§8.7, §11.5). | I05: one writer per managed workspace; admission refuses a run while another run is active. |
| N5 | Plugins, mods, themes, the external process protocol (§14). | I11: the adapter seam is internal Go only; no external code can register an adapter. |
| N6 | Adapters other than `claudecode` and `fake` (§9.4–9.13). | I01, I14: no capability is claimed for any other harness. |
| N7 | Windows Job Objects and Windows process-tree ownership (§7.4). The seam exists; Windows builds and runs unit and fake-adapter tests in CI. | I06, I14: the `claudecode` adapter reports `process_tree_ownership: unsupported` on Windows and admission blocks it (exit 7). |
| N8 | Billing qualification: entitlement and overage-prevention evidence (§13.9, §13.10, §18.9). | I15, G05: strict `--billing subscription-only` blocks (exit 3). The slice adds a separately named, user-declared posture that is never labelled as qualified (FR-4). |
| N9 | Sandboxed profiles `restricted` and `inspect` (§12.2). | §12.2: requesting either blocks (exit 7). The only admitted profile is `trusted-host`, and it needs explicit consent. |
| N10 | Native approval bridge, steering, native resume, repair attempts (§9.1 optional operations, §8.8). | §12.3: denied native tools stay denied and are reported. The slice never adds a permission-bypass flag. |
| N11 | Per-user supervisor daemon and local control IPC (§6.5). | I06, §7.3: each run has exactly one supervising process, identified by an OS lock. A run whose owner cannot be proven is `interrupted`, never assumed live or dead. |
| N12 | Snapshotting selected uncommitted files; submodules, Git LFS, sparse or shallow checkouts (§11.2). | I08, §11.2: unsupported repository features fail preflight (exit 7) instead of producing a partial snapshot. |
| N13 | Budget ledger, metered profiles, `--spend-limit` (§13.5, §13.6). | I09, I10: native cost figures are stored only as a native-reported retail-equivalent estimate, never as spend. |
| N14 | `gc`, `export`, `logs`, `init`, `profiles`, `config explain`, `upgrade`, `completion`. | §17.6: nothing in the slice deletes run data automatically. |

---

## Functional Requirements

Acceptance criteria use EARS. The tags in brackets name the invariants and gates each one serves.

### FR-1 — Command surface and exit codes

- **AC-1.1** [§15.10] The system shall provide `run`, `runs list`, `review`, `apply`, `stop`, `recover`, `demo`, `doctor` and `version`, plus a hidden `__worker` entry point (and a hidden `__fake-agent` used by the fake adapter).
- **AC-1.2** [§15.10] Each command shall exit with the §15.10 meaning: 0 success, 2 invalid arguments or configuration, 3 admission, policy or approval blocked, 4 native execution failed, 5 verification failed or unavailable, 6 interrupted or ownership unresolved, 7 capability unavailable, 130 foreground cancellation completed.
- **AC-1.3** [§5.6] When `--format jsonl` is given, every command shall write one JSON object per line to stdout and diagnostics to stderr. `run` and `recover` shall write only documented envelope events (FR-9), ending with a `run.result` event that carries `exit_code` and `error_category`. The other commands use these command-specific schemas, each object carrying a `type` field:
  - `version`: one `{"type":"version",...}` object.
  - `runs list`: one `{"type":"run_row",...}` object per run.
  - `review`: one `{"type":"receipt",...}` object (design.md §9).
  - `apply`, `stop`, `doctor`, `demo`: one `{"type":"<command>.result",...,"exit_code":N,"error_category":"..."}` object.
- **AC-1.4** [§5.6, I02] When `--non-interactive` is given and a decision needs the user, the system shall exit 3 with a reason naming the flag that would answer it. It shall never wait for input and never assume a yes.
- **AC-1.5** [§15.4] When stdout is not a TTY, or `--plain` is given, output shall be linear text with no cursor movement or colour codes. `NO_COLOR` shall suppress colour.

### FR-2 — Admission (§8.3, §13.10 steps 1–2, I02)

- **AC-2.1** [I02] The system shall not launch a worker until it has persisted an `admission.decided` event that resolves: adapter identity and version, native executable path and version, workspace snapshot, data destinations, execution profile, billing posture and required capabilities. Any field left `unknown` where the policy requires a value shall block (exit 3 or 7) with the missing field named.
- **AC-2.2** [I04, I16] The adapter shall be chosen only by `--adapter claudecode|fake`, with no default and no fallback. A launch failure shall never retry under another adapter.
- **AC-2.3** [§12.2] Admission shall require `--execution-profile trusted-host` (or an equivalent recorded consent) and shall disclose that native tools and checks run with the user's host authority and are not contained. `restricted` and `inspect` shall exit 7.
- **AC-2.4** [I05, N4] If another run is in an active state, then admission shall exit 3 with that run's ID.
- **AC-2.5** [I03, §12.4] Admission shall inventory the native configuration that can execute code or send data: the settings files in each scope, hooks, MCP server definitions (project `.mcp.json`, user-scope and per-project entries in `~/.claude.json`, managed), `apiKeyHelper`, and `env` blocks in settings. It shall record a digest of each. If an item is present that has no trust grant matching its digest, it shall ask in interactive mode, or exit 3 under `--non-interactive` unless `--trust-native-config <digest>` names it. Repository content can never grant that trust.

### FR-3 — Source protection and snapshot (§11.2, I08, G03)

- **AC-3.1** [§11.2] The system shall resolve the repository's committed `HEAD` to a full object ID and record it, together with the branch name, as the admitted snapshot.
- **AC-3.2** [§11.2] If the checkout has tracked modifications or untracked non-ignored files, then the system shall not stash, reset or commit. It shall offer the committed revision instead: an interactive prompt, or `--use-committed` / `--rev <rev>`. Under `--non-interactive` without either flag it shall exit 3.
- **AC-3.3** [§11.2] The system shall snapshot the admitted revision into an independent clone under the MYTHHELM state directory, detach it at that revision and remove every remote. The agent's working directory shall be that clone.
- **AC-3.4** [I08] From admission until an explicit `apply`, the user's checkout shall be byte-identical: same working-tree file hashes, index, `HEAD`, refs and config. Test: `TestRunLeavesSourceCheckoutUntouched` (e2e, packaged binary).
- **AC-3.5** [§11.2] If the repository uses submodules, Git LFS, a sparse checkout or is shallow, then preflight shall exit 7 naming the feature.

### FR-4 — Billing posture, honestly labelled (§9.3, §13.8–13.10, I15, I19)

- **AC-4.1** [I15, G05] `--billing` has no default: omitting it exits 2. `--billing subscription-only` (strict) shall exit 3 with the reason `entitlement_qualification_unavailable`. No input can make the slice admit strict subscription-only.
- **AC-4.2** [I15, §13.10 step 4, §18.9 row 7] `--billing subscription-declared` shall admit Claude Code only when the user has recorded a declaration: plan class, and that extra usage or paid overage is disabled on the account. The declaration is made with `--declare-entitlement` and is stored with its timestamp. Native status evidence from `claude auth status` (`loggedIn`, `authMethod=claude.ai`, `apiProvider=firstParty`, `subscriptionType`) is required too, and is recorded without PII. The declaration is bound to a hashed account identity, so a declaration made under another account never counts. The receipt and every summary shall say: `entitlement: user-declared, NOT verified by MYTHHELM; paid continuation: unknown (user declares disabled)`.
- **AC-4.3** [§9.3, §18.9 row 1] If any credential-route override is present, then admission shall block (exit 3), naming only the variable or setting names, never their values. Overrides are `ANTHROPIC_API_KEY`, `ANTHROPIC_AUTH_TOKEN`, `CLAUDE_CODE_USE_BEDROCK`, `CLAUDE_CODE_USE_VERTEX`, `CLAUDE_CODE_USE_FOUNDRY`, `ANTHROPIC_BASE_URL`, `CLAUDE_CODE_OAUTH_TOKEN` (scratchpad Q7), or `apiKeyHelper` / credential variables inside any settings `env` block. The environment-variable case may instead proceed with `--strip-credential-env`, which removes them from the child only and records the override in the manifest. Settings-file cases have no strip option: MYTHHELM never edits native configuration.
- **AC-4.7** [§12.5, §13.10, I19] `CLAUDE_CODE_OAUTH_TOKEN` shall be admitted only when the trusted user-level configuration, never project configuration, sets `[adapters.claudecode] credential = "oauth-token"`. When admitted, the variable shall be passed to the native child through the environment allowlist only. MYTHHELM shall never read its value into a Go string it keeps, log it, persist it or copy it elsewhere. `claude auth status` must still report a first-party subscription, and any auth-status value observed only under the token route that differs from AC-4.2's expected values shall be recorded in Task 16 before it is accepted. The receipt shall record `credential_provenance: native-subscription-token`. Without the opt-in, AC-4.3 applies.
- **AC-4.4** [§13.10 step 6] When the native session's init event reports `apiKeySource` other than `"none"` (design.md §6.2 step 5), the worker shall interrupt the attempt immediately. The run shall end `blocked` with reason `billing_route_mismatch` (exit 3).
- **AC-4.5** [I09, §13.4] Any native cost figure shall be stored and shown only as `retail_equivalent_estimate (native-reported, not a charge)`. Token counts shall be labelled `native-reported`. Absent values shall be shown as `unknown`, never `0`.
- **AC-4.6** [I19, §12.5] The system shall never read, copy, log or forward native credential values. Test: `TestNoCredentialValuesPersisted` scans the state directory after an admission with planted fake secrets.

### FR-5 — Native attempt under worker ownership (§7.3, §7.4, I06, I12, I18)

- **AC-5.1** [§7.5] Before spawning the worker, the supervisor shall persist `attempt.launch_intent_recorded` with a unique launch token. The worker shall echo that token in its identity record, and the supervisor shall accept the worker only if the PID, the process start time and the token all match.
- **AC-5.2** [§7.3] The worker shall be a separate `mythhelm __worker` process that owns the native child and its pipes, and records the child's PID and process group. On Linux and macOS it shall run in its own session, so closing the terminal or killing the `run` CLI does not end it.
- **AC-5.3** [§6.1, §7.4] The native process shall be started with an argument array and no shell. The task text shall reach it on stdin, never in argv.
- **AC-5.4** [§12.5, §16.6.3] The child environment shall be built from an explicit allowlist (design.md §6.3). Test: `TestChildEnvIsAllowlisted`.
- **AC-5.5** [I06] When the user presses Ctrl-C once, the run shall record `attempt.stop_requested`, show "stop requested, waiting for confirmation", and exit 130 only after the worker confirms that the process group is gone. A second Ctrl-C shall detach and exit 6, stating that the stop is not yet confirmed.
- **AC-5.6** [§7.4, §18.4] If the native child ignores interruption, then the worker shall escalate through the adapter's stop ladder on the process group (claudecode: SIGTERM, then SIGKILL; fake: SIGINT, then SIGTERM, then SIGKILL), with bounded grace periods, and report `stopped` only when no process in the group remains. Otherwise it shall report `unresolved_descendants` with their PIDs.
- **AC-5.7** [§7.2, I07] Every native exit shall emit `attempt.native_result`, as an event separate from verification. Its result fields are explicit `null`s when no result frame was observed. If no MYTHHELM stop request is active, the attempt shall become `succeeded_native` only when a successful result frame was observed and the native exit code was 0. Every other outcome shall become `failed_native` with exit 4, including a zero exit with no result frame (reason `result_unobserved`). If a stop request is active, the attempt shall become `stopped`, whatever the exit code or signal, and never `failed_native`.
- **AC-5.8** [§12.3, N10] Native permission denials reported by the headless surface shall be recorded with the tool names and listed in the receipt under remaining human action. The slice shall never add a permission-bypass flag.

### FR-6 — Candidate freeze (§11.5 steps 1–3, I07)

- **AC-6.1** [§11.5.1] Freezing shall start only after the worker has confirmed that the native process group is gone and that no descendant it knows of remains (`unresolved_pids` empty). If descendants remain unresolved, then the run shall become `interrupted` (`unresolved_descendants`, exit 6) with the workspace quarantined, and nothing shall be frozen or applied until `recover` confirms that they are gone.
- **AC-6.2** [§11.5.2] The system shall capture the whole working tree, including uncommitted and untracked non-ignored files, as a commit whose parent is the admitted revision. This holds whatever the agent did with `HEAD`. It shall record the base revision, tree ID, candidate commit, changed paths with per-file blob IDs, and a SHA-256 of the binary patch.
- **AC-6.3** [§11.5.3, §11.6] Validation shall flag these: symlinks resolving outside the tree, binary files, files over 1 MiB, changes to the check configuration or test files, secret-pattern matches (names and paths only), and newly ignored files. The flags shall be recorded. `apply` shall refuse a flagged candidate without `--accept-flags`.

### FR-7 — Verification (§11.6, I07, G06)

- **AC-7.1** [§11.6] Checks shall come from `mythhelm.toml` in the admitted snapshot, not from the candidate. A candidate that changes `mythhelm.toml` shall be flagged, and the admitted configuration still used.
- **AC-7.2** [I03, §12.4] Checks, native allowed-tool rules and environment passthrough names from `mythhelm.toml` shall take effect only under a trust grant bound to that file's SHA-256: an interactive prompt, or `--trust-project-config sha256:<hex>`.
- **AC-7.3** [§11.6] Each check shall run with argv (no shell) against a separate checkout of the candidate commit, with an allowlisted environment and a timeout. Its status shall be one of `passed`, `failed`, `timed_out`, `unavailable` or `not_run`, with bounded, redacted output stored as evidence with its SHA-256.
- **AC-7.4** [I07] A run whose native result succeeded but whose checks failed shall end `failed`, reason `verification_failed`, exit 5. A native failure shall end with exit 4, keeping its partial candidate for review. Only native success plus all checks passing shall end `ready_for_review` with exit 0. Under `--no-checks`, the run shall end `ready_for_review` with reason `unverified` and exit 5 (`verification_unavailable`). It shall be labelled `NOT RUN` everywhere, and `apply` shall require `--accept-unverified`.

### FR-8 — Receipt, review and guarded apply (§17.1, §11.7)

- **AC-8.1** [§17.1] Every run that reaches a terminal or `ready_for_review` state shall write `receipt.json` with the fields in design.md §9, and no credential values.
- **AC-8.2** `mythhelm review <run>` shall print the receipt summary, flags, check evidence paths and the candidate diff. Filenames, diff content and all native text shall be sanitised of control and escape sequences [§12.7]. `--format jsonl` shall print the receipt as one JSON object.
- **AC-8.3** [§11.7, I08] `mythhelm apply <run> --to-branch <b>` shall create branch `<b>` in the user's repository pointing at the candidate commit. It shall do so only if: the run is `ready_for_review`; `<b>` passes `git check-ref-format --branch` and does not already exist; the admitted base revision is still present; and the flags were accepted. It shall not change the user's working tree, index, `HEAD` or any other ref. It shall never force. The apply intent and its result shall be journaled [§7.5], and the run shall become `completed`.
- **AC-8.4** [§7.5] If `apply` is interrupted, then a retry shall reconcile first: if the branch exists at the candidate commit, record the result; if it exists elsewhere, block (exit 3).

### FR-9 — Durable state and journal (§6.4, §7.5, §7.6)

- **AC-9.1** [§6.4] State shall live in a SQLite database opened through a pure-Go driver, with no cgo, in WAL mode with `foreign_keys=ON`, `synchronous=FULL` and a busy timeout, under a per-user state directory with 0700 permissions (overridable with `MYTHHELM_HOME`).
- **AC-9.2** [§7.6] The `journal` table shall hold the envelope fields `schema_version`, `event_id`, `run_id`, `task_id`, `attempt_id`, `producer_id`, `producer_sequence`, `run_sequence`, `generation`, `caused_by`, `observed_at` and `type`, plus a JSON payload. It shall be append-only, enforced by triggers that abort UPDATE and DELETE. Duplicate `event_id`s shall be ignored idempotently, and a gap or regression in `producer_sequence` shall be rejected.
- **AC-9.3** [§7.5] Each state transition and its journal event shall commit in one transaction. `runs list` shall read projections only.
- **AC-9.4** [§7.5] If the database's schema version is newer than the binary supports, then every command shall exit 2 with a restore instruction and without writing.
- **AC-9.5** [§17.5] If the journal cannot be written, then no new work shall be admitted. An active run shall move toward a safe stop and report degraded persistence.

### FR-10 — Recovery and stop (§7.7, I06, I12)

- **AC-10.1** [§7.7] `mythhelm recover <run>` shall produce exactly one of the following outcomes. **Reattached**: the worker's identity is verified; ingest resumes from the last ingested sequence, and the pipeline continues. **Continued**: the worker exited after a terminal native event; freeze and verify. **Interrupted and quarantined**: the worker is lost without a terminal event; the workspace is marked quarantined and no new writer is launched.
- **AC-10.2** [I12] Recovery shall never relaunch the native agent or replay a native tool call.
- **AC-10.3** [§18.4] If the worker's PID is alive but its start time or token differs (PID reuse), then recovery shall treat the worker as lost. It shall not signal that process.
- **AC-10.4** [I06] `mythhelm stop <run>` shall record the stop request and report `stop_requested` until the worker confirms. It shall never print "stopped" on the request alone.

### FR-11 — Offline demo and fake adapter (§5.1, §14.9, §18.4, G01)

- **AC-11.1** [G01] `mythhelm demo` shall create a disposable repository and a temporary state directory, run the full pipeline with the `fake` adapter, and print the receipt and diff. It shall use no network and no credentials, work on Linux, macOS and Windows, and label every screen `SCRIPTED DEMO, no real agent`.
- **AC-11.2** [§9.1] The `fake` adapter shall implement the same adapter interface as `claudecode`, and shall run its scripted agent as a real child process (`__fake-agent`) so that the worker, ownership and stop paths are the production ones.
- **AC-11.3** [§18.4] Fake scenarios shall cover these faults: crash after launch intent before acknowledgement; worker death with PID reuse; a child that ignores cancellation; a headless permission denial; malformed JSON, excessive nesting, an oversized frame and invalid UTF-8; a locked database; and a stale owner lock while the old owner is alive. Each has a named test (tasks.md Task 15).

### FR-12 — Read-only doctor (§5.1, A30)

- **AC-12.1** `mythhelm doctor` shall report: the MYTHHELM version, OS and architecture; git version; the `claude` path and version; credential-route override names (never values); settings files and hook and MCP counts; sandbox tools present (reported only, not used in this slice); state-directory status; and terminal facts.
- **AC-12.2** [A30] Doctor shall not create, modify or delete any file, and shall not start a native session or any network request. Test: `TestDoctorIsReadOnly` hashes the state and home fixtures before and after.

## Non-Functional Requirements

- **NFR-1 (Bounded parsing, §9.7)**: Native stdout is parsed with a maximum frame of 16 MiB, a maximum JSON nesting of 64, UTF-8 validation, and counters for dropped or coalesced frames. stderr is kept as a redacted ring of at most 256 KiB. No unbounded buffer is attached to native output.
- **NFR-2 (Logging, §12.7)**: Default logs and the journal contain status, counts and redacted summaries only, not prompts, assistant text or tool output. Raw capture is opt-in per run (`--capture-raw`), mode 0600, capped at 256 MiB, and labelled sensitive.
- **NFR-3 (Dependencies, §19.2)**: Every direct dependency is Apache-2.0-compatible and on the dependency-review allowlist. The direct set is exactly `modernc.org/sqlite`, `golang.org/x/sys` and `github.com/BurntSushi/toml` unless a decision record adds one.
- **NFR-4 (Platforms, §16.1, I14)**: `go build`, `go vet`, `go test -race ./...` pass on `ubuntu-latest`, `macos-latest` and `windows-latest`. The capability record states each platform's support for worker detachment, process-tree ownership and the `claudecode` adapter.
- **NFR-5 (Performance, §17.4)**: `mythhelm runs list` with 100 runs completes in under 200 ms on CI Linux (benchmark, advisory).

## Definition of Done

- [ ] All tasks in `tasks.md` are complete, with their named tests passing on the three-OS CI matrix.
- [ ] `golangci-lint` and `govulncheck` jobs are green. Dependency review passes.
- [ ] Decision records exist for the billing posture (ADR 0002, maintainer decision), local state (ADR 0003) and the slice's process model (ADR 0004).
- [ ] The maintainer has run one real task on MYTHHELM through `claudecode`, and its sanitised receipt excerpt and recorded stream-json fixture are committed (Task 20).
- [ ] The README status section lists what the slice supports and what it deliberately does not.
