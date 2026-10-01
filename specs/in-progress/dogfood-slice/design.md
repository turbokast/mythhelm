## Dogfood Slice — Design

> Research date 2026-09-29. Evidence and URLs are in [`scratchpad.md`](scratchpad.md). Section references (§) point to [`docs/spec/master-spec.md`](../../../docs/spec/master-spec.md) Rev 1.1; FR/AC IDs point to [`requirements.md`](requirements.md).

### 1. Current state

The repository holds governance files, CI and the master spec. There is no Go code. `.github/workflows/ci.yml` already defines a Go job, gated on `go.mod` existing, that runs `gofmt`, `go vet`, `go test -race` and `go mod tidy -diff` on Linux, macOS and Windows. `docs/automation.md` lists the jobs that switch on with the first `go.mod`.

### 2. Package layout (§19.1 subset)

Only the packages this slice needs are created. The others in §19.1 are deferred: `routing`, `scheduler`, `billing`, `tui`, `hosts/herdr`, `protocol`, `sdk` and `mods`.

```text
cmd/mythhelm/            main: dispatch to internal/cli; hidden __worker, __fake-agent
internal/cli/            commands, interspersed-flag parser, plain/jsonl renderers, exit codes
internal/buildinfo/      version, commit, go version (ldflags + debug.ReadBuildInfo)
internal/ids/            prefixed ULID-format IDs (run_, att_, evt_, ...), crypto/rand
internal/statedir/       per-user state directory resolution, 0700 creation, MYTHHELM_HOME
internal/journal/        SQLite open, migrations, envelope, Append, projections, queries
internal/admission/      admission decision, billing posture, declarations, trust grants, mythhelm.toml
internal/supervisor/     run pipeline, state machines, spool ingest, owner lock, recovery, receipt
internal/workers/        __worker main, process launch + ownership, spool writer, stop ladder
internal/workspace/      safe git runner, preflight, snapshot clone, guarded apply, fingerprint
internal/integration/    candidate freeze + validation flags, check runner
internal/security/       redaction, terminal sanitising, env allowlist, path checks
internal/adapter/        adapter interface, capability record, observations, NDJSON framing
adapters/claudecode/     probe, auth evidence, argv/env, stream-json decoder, compat table
adapters/fake/           scripted adapter + __fake-agent + embedded scenarios
tests/e2e/               packaged-binary tests (builds cmd/mythhelm in TestMain)
```

The adapter interface lives in `internal/adapter`, so no out-of-module code can register an adapter (N5, I11). The `adapters/*` packages each carry a `doc.go` stating that they are not a stable API before the plugin protocol exists.

### 3. Process model (§6.2, §7.3, §7.4) — decision D3, recorded as ADR 0004

```text
mythhelm run (supervisor for this run; holds runs/<id>/owner.lock)
   │ 1. admission → journal (SQLite)
   │ 2. attempt.launch_intent_recorded {launch_token}
   │ 3. spawn: mythhelm __worker --state <dir> --run <id> --attempt <id>
   │         (Unix: Setsid; Windows: CREATE_NEW_PROCESS_GROUP|DETACHED_PROCESS)
   ▼
mythhelm __worker (owns the native child; survives the CLI on Linux/macOS)
   │ writes worker.json {pid, start_time, launch_token, native_launch_intent}
   │ launches native: argv array, Setpgid, stdin=prompt file, stdout/stderr pipes
   │ decodes stdout via the adapter → envelope events → spool.jsonl (O_APPEND; fsync on critical)
   │ polls stop.request every 250 ms; heartbeat file every 2 s
   ▼
native agent (claude -p ... | mythhelm __fake-agent ...)
```

- **Single SQLite writer per run.** The worker never opens SQLite; it owns only its attempt directory. Whichever process holds `owner.lock` ingests the spool into the journal (flock via `golang.org/x/sys/unix`, `LockFileEx` on Windows). The OS releases the lock when that process dies, so a lock can't go stale while its holder lives, and a live holder is never pre-empted (§18.4 stale-lease row). This is the slice's substitute for the per-user daemon (N11). Writes from different runs are serialised by SQLite transactions with `busy_timeout`.
- **Identity** (§7.3). The supervisor accepts a worker only if all three of these match: `worker.json.launch_token` equals the journaled token, the PID is alive, and the PID's start time equals `worker.json.start_time`. Start time comes from `/proc/<pid>/stat` field 22 on Linux, `sysctl kern.proc.pid` (`unix.SysctlKinfoProc`) on macOS, and `GetProcessTimes` on Windows. A PID that is alive but has a different start time is a reused PID (AC-10.3).
- **Native launch window.** The worker writes `native_launch_intent` before `exec.Cmd.Start` and the PID and PGID after it. If it crashes between those writes, recovery reports `ownership_unresolved`. On Linux it also scans `/proc/*/environ` for the marker `MYTHHELM_ATTEMPT_ID=<id>`, which is set in the child environment, and lists matching PIDs without signalling them. macOS prints the marker for manual inspection. Where ownership cannot be proven, the slice says so (§7.4).
- **Stop ladder.** The adapter supplies it; the worker executes it on the process group. `claudecode` sends SIGTERM (documented: exit 143, turn left unfinished), waits 10 s, then sends SIGKILL. `fake` sends SIGINT, waits 3 s, sends SIGTERM, waits 3 s, then sends SIGKILL, so the whole ladder is exercised. While a stop request is active, any native exit, whatever its code or signal (130 after SIGINT, 143 after SIGTERM, SIGKILL), is classified as `stopped`, never `failed_native`. A stop is confirmed only when `kill(-pgid, 0)` returns `ESRCH`. Descendants that leave the group, for example through `setsid`, are not tracked; if any are known, the worker reports `unresolved_descendants` with their PIDs (AC-5.6). A non-empty `unresolved_pids` set blocks freezing, because an escaped descendant could still write to the workspace. In that case the attempt becomes `quarantined` and the run becomes `interrupted` with reason `unresolved_descendants` (exit 6). `recover` re-checks each PID by PID and start time, and freezes only once all of them are gone. On Windows, the fake child is a single process stopped with `Process.Kill` and confirmed by `Wait`. `claudecode` is blocked on Windows (N7).
- **Foreground Ctrl-C** (AC-5.5). The first SIGINT writes `stop.request` and prints `stop requested — waiting for worker confirmation`; the command exits 130 after `attempt.stopped`. A second SIGINT detaches, exiting 6 with `stop not yet confirmed; run 'mythhelm recover <id>'`. Terminal hangup and SIGKILL of the CLI leave the worker running. `runs list` then shows `executing (worker alive, unattached)` and `recover` reattaches.

### 4. State machines (§7.2 subset)

Run states and transitions. Any other transition is rejected by `supervisor.Transition`.

| From | To (reason codes) |
|---|---|
| `created` | `admission` |
| `admission` | `executing`, `blocked` (`billing_*`, `trust_required`, `dirty_checkout`, `active_run_exists`, `persistence_unavailable`), `failed` (`preflight_*`) |
| `executing` | `verifying`, `failed` (`native_failed`, `protocol_error`), `blocked` (`billing_route_mismatch`, `native_auth_or_billing`), `stopping`, `interrupted` |
| `verifying` | `ready_for_review` (reason empty when all checks passed, `unverified` under `--no-checks`), `failed` (`verification_failed`, `verification_unavailable`) |
| `ready_for_review` | `applying` |
| `applying` | `completed`, `blocked` (`branch_exists`, `base_missing`, `flags_unaccepted`) |
| `stopping` | `cancelled`, `interrupted` (`stop_unconfirmed`), `blocked` (`billing_route_mismatch`) |
| `interrupted` (`stop_unconfirmed`, `unresolved_descendants`, `worker_lost`) | `recovering` |
| `recovering` | `executing`, `verifying`, `failed`, `interrupted` (`quarantined`) |

A run that is already `stopping` still ends `blocked` when the attempt stopped for `billing_route_mismatch`: AC-4.4 is unconditional, and the route violation dominates the user's cancellation. The attempt stays `stopped` with the mismatch reason, so no evidence is lost.

`blocked`, `failed`, `cancelled` and `completed` are terminal. The planning and integrating states are not entered (N4): with one candidate based on the admitted snapshot, the candidate commit is the combined revision, and the receipt says so.

Attempt states: `launch_intent_recorded` → `launching` → `running` → {`succeeded_native` | `failed_native` | `stop_requested` → `stopped` | `interrupted` → `quarantined`}. `reserved`, `waiting_native` and `waiting_approval` are not entered (N10).

### 5. Durable state (§6.4, §7.5, §7.6) — decision D2, recorded as ADR 0003

The state directory is `$MYTHHELM_HOME`, or by default `$XDG_STATE_HOME/mythhelm` (falling back to `~/.local/state/mythhelm`) on Linux, `~/Library/Application Support/mythhelm` on macOS and `%LocalAppData%\mythhelm` on Windows. It has mode 0700. The database is `mythhelm.db`, and each run has `runs/<run_id>/{owner.lock, task.md, workspace/, attempts/<att>/{worker.json, spool.jsonl, heartbeat, stop.request, stderr.log}, verify/<ver>/, evidence/, receipt.json}`.

The database is opened with DSN `file:<path>?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=synchronous(FULL)&_pragma=busy_timeout(5000)&_txlock=immediate` (driver `modernc.org/sqlite`). The state directory must be on a local filesystem (WAL constraint, [S37]), which doctor reports.

Schema v1 (`internal/journal/migrations/0001_init.sql`, tracked with `PRAGMA user_version`):

```sql
CREATE TABLE runs (
  run_id TEXT PRIMARY KEY, state TEXT NOT NULL, reason TEXT,
  adapter_id TEXT NOT NULL, source_repo TEXT NOT NULL, source_branch TEXT,
  base_rev TEXT, task_sha256 TEXT NOT NULL, billing_posture TEXT NOT NULL,
  execution_profile TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL
) STRICT;
CREATE TABLE attempts (
  attempt_id TEXT PRIMARY KEY, run_id TEXT NOT NULL REFERENCES runs(run_id),
  task_id TEXT NOT NULL, attempt_number INTEGER NOT NULL, state TEXT NOT NULL, reason TEXT,
  launch_token_sha256 TEXT NOT NULL, worker_pid INTEGER, worker_start_time TEXT,
  native_pid INTEGER, native_pgid INTEGER, native_session_id TEXT,
  workspace_path TEXT NOT NULL, spool_offset INTEGER NOT NULL DEFAULT 0,
  UNIQUE (run_id, task_id, attempt_number)
) STRICT;
CREATE TABLE journal (
  seq INTEGER PRIMARY KEY, event_id TEXT NOT NULL UNIQUE, schema_version INTEGER NOT NULL,
  run_id TEXT NOT NULL, task_id TEXT, attempt_id TEXT,
  producer_id TEXT NOT NULL, producer_sequence INTEGER NOT NULL,
  run_sequence INTEGER NOT NULL, generation INTEGER NOT NULL, caused_by TEXT,
  observed_at TEXT NOT NULL, type TEXT NOT NULL,
  payload TEXT NOT NULL CHECK (json_valid(payload)),
  UNIQUE (producer_id, producer_sequence), UNIQUE (run_id, run_sequence)
) STRICT;
CREATE TRIGGER journal_no_update BEFORE UPDATE ON journal BEGIN SELECT RAISE(ABORT, 'journal is append-only'); END;
CREATE TRIGGER journal_no_delete BEFORE DELETE ON journal BEGIN SELECT RAISE(ABORT, 'journal is append-only'); END;
CREATE TABLE producers (producer_id TEXT PRIMARY KEY, last_sequence INTEGER NOT NULL, generation INTEGER NOT NULL) STRICT;
CREATE TABLE candidates (
  attempt_id TEXT PRIMARY KEY REFERENCES attempts(attempt_id), base_rev TEXT NOT NULL,
  candidate_commit TEXT NOT NULL, tree_id TEXT NOT NULL, patch_sha256 TEXT NOT NULL,
  changed_paths TEXT NOT NULL CHECK (json_valid(changed_paths)),
  flags TEXT NOT NULL CHECK (json_valid(flags)), partial INTEGER NOT NULL
) STRICT;
CREATE TABLE verifications (
  verification_id TEXT PRIMARY KEY, run_id TEXT NOT NULL REFERENCES runs(run_id),
  candidate_commit TEXT NOT NULL, config_sha256 TEXT NOT NULL, result TEXT NOT NULL,
  started_at TEXT NOT NULL, finished_at TEXT
) STRICT;
CREATE TABLE check_results (
  verification_id TEXT NOT NULL REFERENCES verifications(verification_id), name TEXT NOT NULL,
  argv TEXT NOT NULL, status TEXT NOT NULL, exit_code INTEGER, duration_ms INTEGER,
  evidence_path TEXT, evidence_sha256 TEXT, PRIMARY KEY (verification_id, name)
) STRICT;
CREATE TABLE trust_grants (
  kind TEXT NOT NULL CHECK (kind IN ('project_config','native_config')),
  repo_identity TEXT NOT NULL, digest TEXT NOT NULL, granted_at TEXT NOT NULL,
  PRIMARY KEY (kind, repo_identity, digest)
) STRICT;
CREATE TABLE declarations (
  adapter_id TEXT NOT NULL, plan_class TEXT NOT NULL,
  extra_usage TEXT NOT NULL CHECK (extra_usage = 'disabled'),
  identity_ref TEXT NOT NULL,  -- sha256(orgId || configDirectory) from auth status
  declared_at TEXT NOT NULL, superseded_at TEXT
) STRICT;
-- at most one current declaration per adapter and identity; declaring supersedes the previous row in the same transaction
CREATE UNIQUE INDEX declarations_current ON declarations (adapter_id, identity_ref) WHERE superseded_at IS NULL;
CREATE TABLE applies (
  apply_id TEXT PRIMARY KEY, run_id TEXT NOT NULL REFERENCES runs(run_id),
  target_repo TEXT NOT NULL, branch TEXT NOT NULL, candidate_commit TEXT NOT NULL,
  state TEXT NOT NULL, reason TEXT, created_at TEXT NOT NULL
) STRICT;
```

Migration policy (§7.5): if `user_version` is greater than the binary supports, exit 2 without writing (AC-9.4). Before any migration from a non-zero version, the database is copied with `VACUUM INTO mythhelm.db.bak-v<N>`.

**Envelope** (`internal/journal`), the §7.6 subset. All fields are required except `task_id`, `attempt_id` and `caused_by`.

```go
type Event struct {
    SchemaVersion    int             `json:"schema_version"`   // 1
    EventID          string          `json:"event_id"`         // evt_<ulid>
    RunID            string          `json:"run_id"`
    TaskID           string          `json:"task_id,omitempty"`
    AttemptID        string          `json:"attempt_id,omitempty"`
    ProducerID       string          `json:"producer_id"`      // sup_<ulid> | wrk_<attempt>
    ProducerSequence int64           `json:"producer_sequence"`
    RunSequence      int64           `json:"run_sequence"`     // set at ingest
    Generation       int64           `json:"generation"`
    CausedBy         string          `json:"caused_by,omitempty"`
    ObservedAt       time.Time       `json:"observed_at"`      // UTC, display/audit only
    Type             string          `json:"type"`
    Payload          json.RawMessage `json:"payload"`
}
func (j *Journal) Append(ctx context.Context, ev Event, project func(*sql.Tx) error) error
```

`Append` runs in one transaction. It does nothing if `event_id` already exists. It rejects `producer_sequence != producers.last_sequence+1` and a stale `generation`, assigns `run_sequence`, applies `project` (the projection update), and commits (AC-9.2, AC-9.3).

Event types are listed below. A leading `*` means the event is durable and critical: the worker fsyncs it before continuing. The public JSONL output (AC-1.3) uses the same set.

| Type | Payload (never content, never credentials) |
|---|---|
| `*run.created`, `*run.state_changed` | `{state, reason}` |
| `*admission.decided` | adapter, native path/version/sha256, snapshot, profile, billing posture (§6.2), config manifest digests, overrides, capabilities |
| `*workspace.snapshot_created` | `{base_rev, branch, clone_path}` |
| `*attempt.launch_intent_recorded` | `{launch_token_sha256}` |
| `*attempt.launched` | `{worker_pid, worker_start_time, native_pid, native_pgid}` |
| `*attempt.native_session` | `{session_id, model, native_version, permission_mode, auth_source, tool_count, mcp:[{name,status}], plugin_count}` |
| `attempt.progress` | at most one per second: `{assistant_turns, tool_uses:{name:count}, retries}` |
| `*attempt.permission_denied` | `{tool_name}` |
| `*attempt.native_result` | `{exit_code, signal, stop_requested, result_observed, subtype, is_error, num_turns, duration_ms, stop_reason, usage_native_reported, retail_equivalent_estimate_usd, denials}`. It is emitted for every native exit, including stopped ones. When no result frame was observed, `result_observed` is `false` and every result field is an explicit `null`, never omitted and never `0`. The terminal attempt state is carried separately by `attempt.state_changed`. |
| `*attempt.stop_requested`, `*attempt.stopped` | `{requested_by}` / `{confirmed, unresolved_pids}` |
| `*attempt.state_changed` | `{state, reason}` |
| `attempt.protocol_counters` | `{malformed, oversized, invalid_utf8, depth_exceeded, unknown_types}` |
| `*candidate.frozen` | `{base_rev, candidate_commit, tree_id, patch_sha256, changed_count, flags, partial}` |
| `*verification.started`, `*check.completed`, `*verification.completed` | ids, status, exit code, evidence sha256 |
| `*receipt.written` | `{path, sha256}` |
| `*apply.intent_recorded`, `*apply.completed` | `{branch, target_repo, candidate_commit}` |
| `run.result` (stdout only, final line of `run` and `recover`) | `{state, reason, exit_code, error_category}` |

### 6. Claude Code adapter (§9.3, §13.10) — tested against `claude` 2.1.284

#### 6.1 Probe (§9.1 `probe`)

1. Resolve `claude` with `exec.LookPath`, then `filepath.EvalSymlinks`. On the reference install this gives `~/.local/share/claude/versions/2.1.284`. The resolved, versioned path is what gets launched, so a background auto-update between probe and launch can't swap the binary. Record its SHA-256, cached by path, size and mtime.
2. Run `<resolved> --version`, expect `^(\d+\.\d+\.\d+) \(Claude Code\)$`, with a 10 s timeout. The output format was confirmed locally.
3. Check compatibility (`compat.go`, §9.8). The tested floor is 2.1.284. Any `2.1.x` with x ≥ 284 is admitted, labelled `fixture-tested on 2.1.284; running <v> untested`. Any other major or minor version, or anything below the floor, exits 7 unless `--allow-untested-native-version` is given, in which case the run is labelled experimental. The floor also covers the flags this adapter relies on: `--permission-prompts` needs 2.1.259, `CLAUDE_CODE_STARTUP_FAILURE_RESULTS` needs 2.1.274.
4. Platform. Windows reports `process_tree_ownership: unsupported`, and admission exits 7 (N7).

#### 6.2 Billing posture and auth evidence (FR-4, §13.10 steps 1–2)

These steps run in order, and any block stops admission.

1. **Environment overrides.** Look for `CLAUDE_CODE_USE_BEDROCK`, `CLAUDE_CODE_USE_VERTEX`, `CLAUDE_CODE_USE_FOUNDRY`, `ANTHROPIC_AUTH_TOKEN`, `ANTHROPIC_API_KEY`, `ANTHROPIC_BASE_URL` and `CLAUDE_CODE_OAUTH_TOKEN`. These are the documented precedence sources above subscription OAuth. `ANTHROPIC_API_KEY` "is always used when present" in `-p`. If any are found, exit 3, naming them. `--strip-credential-env` instead removes them from the child environment only, and records that as an override. Exception: `CLAUDE_CODE_OAUTH_TOKEN` is not an override when the trusted user-level config opts in with `[adapters.claudecode] credential = "oauth-token"` (AC-4.7). It is then passed through to the child unread, and step 3 must still report a first-party subscription.
2. **Settings inventory.** Read `~/.claude/settings.json` (or `$CLAUDE_CONFIG_DIR/settings.json`), the managed settings for the OS (`managed-settings.json`, `managed-settings.d/`, `managed-mcp.json`), and the snapshot's `.claude/settings.json`, `.claude/settings.local.json` and `.mcp.json`. Also read `~/.claude.json` (or `$CLAUDE_CONFIG_DIR/.claude.json`), which holds user-scope and per-project MCP servers. From that file, decode only the `mcpServers` object and `projects[<workspace path>].mcpServers`, into a struct that has no other fields, so the sign-in session and account data in it are never read into memory as values or persisted (AC-4.6). Every active hook and MCP definition, in every scope, goes into the `native_config` digest and needs a matching trust grant. A source that exists but cannot be parsed blocks admission (exit 3, `native_config_unreadable`). The settings `env` block overrides shell variables, so any of these blocks admission with exit 3 and a "fix it in native settings" action: `apiKeyHelper`, `forceLoginMethod` set to anything other than `claudeai`, or any variable from step 1 inside an `env` block. MYTHHELM never edits these files.
3. **Native status.** Run `<resolved> auth status`, with the exact child environment and the workspace as cwd, and a 20 s timeout. Parse only `loggedIn`, `authMethod`, `apiProvider`, `subscriptionType` and `configDirectory`. Keep `orgId` only as a SHA-256 identity reference, and never persist `email` or `orgName`. It must report `loggedIn=true`, `authMethod="claude.ai"` and `apiProvider="firstParty"`. Anything else exits 3 with `needs-native-setup: run 'claude' and /login with your subscription`. These values were observed locally. Other enum values are unverified and treated as a mismatch.
4. **Posture.**
   - `--billing subscription-only` exits 3 with `entitlement_qualification_unavailable` (AC-4.1).
   - `--billing subscription-declared` needs a current `declarations` row, written by `--declare-entitlement plan=<pro|max|team|enterprise>,extra-usage=disabled`. The row is bound to `identity_ref`, the SHA-256 of `orgId` and `configDirectory` from step 3. A declaration whose `identity_ref` differs from the current auth evidence, for example after an account switch, does not count, and admission exits 3 with `declaration_identity_mismatch`. The recorded posture is `{mode: subscription-declared, credential_provenance: native-login, entitlement_class: included-plan, entitlement_source: user_declared+native_status(subscriptionType=<v>), paid_continuation: unknown, paid_continuation_user_declaration: disabled, qualified: false, g05: not-passed}`.
   - There is no default posture: omitting `--billing` exits 2.
5. **In-flight check** (AC-4.4). `system/init.apiKeySource` must be `"none"`. Any other value, such as `ANTHROPIC_API_KEY`, `apiKeyHelper` or `/login managed key`, makes the worker run the stop ladder, and the run ends `blocked`/`billing_route_mismatch`. `"none"` is necessary but not sufficient, because it also covers bearer-token and cloud routes. That is why steps 1–3 exist. Whether any inference request precedes the init event is unverified, and the receipt says so.

#### 6.3 Invocation (§6.1, §9.3, §12.5)

The worker launches with an argv array and no shell. The prompt is the task file, at most 1 MiB, delivered on stdin; Claude Code's stdin cap is 10 MB. Stdin is closed after the prompt is written.

```text
argv = [<resolved claude path>,
        "-p",
        "--output-format", "stream-json",
        "--verbose",                       # required with stream-json in print mode
        "--input-format", "text",
        "--permission-mode", "acceptEdits", # edits + common fs commands inside cwd only
        "--permission-prompts", "none",     # anything that would prompt is denied; never waits
        "--allowedTools", <rule>, <rule>, …] # only if the trusted mythhelm.toml lists rules; always last
cwd  = runs/<id>/workspace        (the independent clone, §7)
```

These flags are deliberately not used. `--bare` and `--safe-mode` would strip native fidelity (§9.3); `--bare` also refuses OAuth. `--dangerously-skip-permissions`, `bypassPermissions` and `auto` would bypass permissions (§12.2). `--no-session-persistence` is left off to keep the native transcript for inspection, and its retention domain is disclosed. `--max-budget-usd` would be a soft, estimate-based stop that is not an entitlement boundary (I10). `--model` keeps the native default, and the reported model is recorded.

**Child environment** (AC-5.4). The child gets only these variables, if present:

- **All platforms:** `PATH HOME USER LOGNAME SHELL LANG LC_ALL LC_CTYPE TZ TMPDIR TERM XDG_CONFIG_HOME XDG_DATA_HOME XDG_CACHE_HOME XDG_STATE_HOME XDG_RUNTIME_DIR CLAUDE_CONFIG_DIR HTTP_PROXY HTTPS_PROXY NO_PROXY http_proxy https_proxy no_proxy SSL_CERT_FILE SSL_CERT_DIR NODE_EXTRA_CA_CERTS`.
- **Windows only** (used by the fake adapter and checks): `USERPROFILE APPDATA LOCALAPPDATA SystemRoot SystemDrive ComSpec PATHEXT TEMP TMP`.
- **Opt-in credential (AC-4.7):** `CLAUDE_CODE_OAUTH_TOKEN`, only when the trusted user-level config sets `[adapters.claudecode] credential = "oauth-token"`. The allowlist copies it from the parent environment into the child environment unexamined. Tests cover both paths: `TestChildEnvOAuthTokenOptIn` (present in the child) and `TestChildEnvOAuthTokenDefaultBlocked` (admission exits 3, or the token is stripped with `--strip-credential-env`).
- **Project passthrough:** names listed under `[environment] passthrough` in the trusted `mythhelm.toml`, for example `GOPATH GOCACHE GOMODCACHE GOTOOLCHAIN GOFLAGS`.

A denylist (`ANTHROPIC_*`, `CLAUDE_CODE_USE_*`, `CLAUDE_CODE_OAUTH_TOKEN` (unless the AC-4.7 user-level opt-in applies), `AWS_*`, `GOOGLE_*`, `AZURE_*`, `OPENAI_*`) always wins. A passthrough entry that names a denied variable exits 2.

MYTHHELM sets three variables and records each as a configuration delta (§9.6):

- `DISABLE_AUTOUPDATER=1` keeps the version pinned for the attempt.
- `CLAUDE_CODE_STARTUP_FAILURE_RESULTS=1` makes startup failures arrive as structured results.
- `MYTHHELM_ATTEMPT_ID=<id>` is the orphan-scan marker (§3).

#### 6.4 Stream decoding (§9.7, NFR-1)

`internal/adapter/ndjson.Reader` splits frames on `\n`. The maximum frame is 16 MiB; beyond that the frame is discarded up to the next newline and `oversized` is counted. Before decoding it checks `utf8.Valid`, counting `invalid_utf8` and dropping the frame, and it pre-scans nesting depth outside strings, with a maximum of 64 (`depth_exceeded`). The adapter decodes `{type, subtype}` first, then the typed struct.

| Native frame | Observation |
|---|---|
| `system` with subtype `init` | `SessionStarted` (session_id, model, claude_code_version, permissionMode, apiKeySource, len(tools), mcp_servers name and status, len(plugins)) |
| `system` with subtype `api_retry` | `Retry{attempt, retry_delay_ms, error_status}` |
| `system` with subtype `permission_denied` | `PermissionDenied{tool_name}` |
| `assistant` | `Progress{turn}`; each `tool_use` block gives `Progress{tool: name}`. A tool name longer than 64 characters or containing control characters becomes `invalid`. `message.error` gives `NativeError{class}` |
| `user`, `stream_event`, `system` hook, `plugin_install` or `compact_boundary` | counted only |
| `result` | `Result{subtype, is_error, num_turns, duration_ms, stop_reason, modelUsage tokens, total_cost_usd as decimal string, permission_denials[].tool_name, len(errors), startup_failure_reason}` |
| unknown `type` | counter under `vendor.claudecode.<type>`, with at most 32 distinct names |

The attempt fails safely with reason `protocol_error` if the init or result frame is malformed, if an `assistant` frame arrives before `init`, or if more than 100 frames are malformed (§7.6).

**Result mapping** (AC-5.7, §8.8). The first matching row wins.

| Condition | Attempt state / run outcome |
|---|---|
| `apiKeySource != "none"` at init | worker stops the attempt → `stopped`; run `blocked`/`billing_route_mismatch`, exit 3 |
| `NativeError` class `authentication_failed`, `oauth_org_not_allowed`, `account_on_hold` or `billing_error` | `failed_native`; run `blocked`/`native_auth_or_billing`, exit 3 |
| any exit (code or signal, e.g. 130, 143, SIGKILL) while a MYTHHELM stop request is active | `stopped` once the process group is confirmed gone; run `cancelled`, exit 130, or 6 if unconfirmed |
| result `success`, `is_error=false`, exit 0 | `succeeded_native` → freeze and verify |
| `NativeError` class `rate_limit` | `failed_native`/`provider_limit`, exit 4. Retry time is `unknown` unless reported; no countdown is invented |
| result subtype `error_*`, or `is_error=true` | `failed_native`/`<subtype>`, exit 4, partial candidate frozen |
| no result frame | `failed_native`, reason `result_unobserved` (native exit 0) or `native_exit_<n>`; exit 4 either way, and the final `run.result` carries the reason |

### 7. Adapter seam (§9.1) — `internal/adapter`

```go
type Adapter interface {
    Descriptor() Descriptor                                            // id, version, harness, surface
    Probe(ctx context.Context, in ProbeInput) (Probe, error)           // no paid task, bounded time
    Capabilities(p Probe) CapabilityRecord                             // §9.2 shape; unknown stays unknown
    Prepare(ctx context.Context, in PrepareInput) (LaunchProposal, error) // may return *BlockedError{Code, Field, Action}
    Start(ctx context.Context, lp LaunchProposal, l Launcher) (Session, error) // worker-only; l owns the process
}
type Launcher interface { // implemented by internal/workers; the only way to create a native process
    Launch(ctx context.Context, spec ProcSpec) (OwnedProc, error)
}
type ProcSpec struct { Path string; Args []string; Dir string; Env []string; Stdin io.Reader }
type Session interface {
    Observations() <-chan Observation // closed at stream end; bounded channel, lossy only for Progress
    Interrupt(ctx context.Context) InterruptReport // runs StopLadder; reports what was confirmed
    Done() <-chan NativeExit
}
type LaunchProposal struct {
    Spec ProcSpec; StopLadder []StopStep; Overrides []ConfigDelta
    Billing BillingPosture; Manifest ConfigManifest; Capabilities CapabilityRecord
}
type Tri string // "supported" | "unsupported" | "unknown"
```

`Observation` is a closed sum type: `SessionStarted`, `Progress`, `Retry`, `PermissionDenied`, `NativeError`, `Result` and `ProtocolCounters`. The worker maps observations to envelope events (§5). `Adapter.Start` never calls `os/exec` directly; this is enforced by a test that greps the non-test `.go` files in `adapters/` for `os/exec`, the only exception being the `Probe` helpers in `probe.go`.

**Fake adapter.** It runs `os.Executable()` as `__fake-agent --scenario <name> --workdir <dir>`. The scenarios are embedded JSON. Each is a list of steps: `emit` a frame, `write` a file, `sleep`, `ignore_signals`, `spawn_escapee` (Unix), `exit`. Its frames are `fake.init`, `fake.progress`, `fake.denied` and `fake.result`, and they go through the same `ndjson.Reader`. Its billing posture is `local-scripted (no inference, no network)`.

Scenarios:

| Scenario | Behaviour |
|---|---|
| `happy` | edits a file, emits progress, then a success result |
| `check-fails` | makes an edit that breaks the check |
| `native-fails` | ends with a failed result |
| `slow` | runs until it is stopped |
| `ignore-sigint` | ignores SIGINT |
| `ignore-term` | ignores SIGINT and SIGTERM |
| `escapee` | spawns a child that leaves the process group |
| `denied` | emits a permission denial |
| `malformed` | emits malformed frames |
| `deep` | emits over-nested JSON |
| `oversized` | emits a frame over the size limit |
| `bad-utf8` | emits invalid UTF-8 |
| `exit-before-result` | exits without a result frame |

### 8. Workspace, candidate and checks

**Git runner** (`internal/workspace/git.go`). Git ≥ 2.30 is required; below that, preflight exits 7. Every invocation uses `exec.Command("git", "-c", "core.fsmonitor=false", "-c", "core.hooksPath="+emptyHooksDir, "-c", "gc.auto=0", "-c", "maintenance.auto=false", ...)`. In the user's repository, read commands also get `GIT_OPTIONAL_LOCKS=0`, so `git status` never rewrites the index (AC-3.4). The fixed hooks directory and fsmonitor setting stop an agent-planted hook in the managed clone, or repo configuration, from running during freeze or verify.

**Snapshot** (AC-3.1–3.5).

1. Preflight: `rev-parse --show-toplevel` and `--is-shallow-repository`, `.gitmodules` or `submodule status`, `lfs` filters in `.gitattributes`, `core.sparseCheckout`, and a dirty check using `status --porcelain=v2 --untracked-files=normal`.
2. `git clone --no-hardlinks --no-checkout -- <src> <run>/workspace`.
3. `checkout --detach <rev>`, then `remote remove origin`, so the agent cannot push into the user's repository.
4. `SourceFingerprint(repo)` hashes the working-tree files, `.git/index`, `HEAD`, `packed-refs` plus `refs/**` and `.git/config`. Tests assert the fingerprint is unchanged.

**Freeze** (FR-6).

1. After `attempt.stopped` or a native exit is confirmed, and only when `unresolved_pids` is empty (§3), run with a temporary `GIT_INDEX_FILE`: `add -A`, then `write-tree`, then `commit-tree <tree> -p <base_rev>` with the message `<first heading of task.md>` plus a `MYTHHELM-Run: <run_id>` trailer. Author and committer come from the user's git identity, read at admission.
2. Point `update-ref refs/mythhelm/candidates/<att>` at the commit.
3. Collect changed paths and blob IDs with `diff-tree -r --no-renames -z <base> <commit>`, and compute `patch_sha256` over `diff --binary <base> <commit>`.
4. Validation flags: `symlink_escape`, `binary`, `large_file` (> 1 MiB), `check_config_changed` (`mythhelm.toml`), `test_files_changed` (paths matching `_test.go`, `/testdata/` or `tests/`), `secret_pattern` (the §12.7 regex set, path and pattern name only), and `ignored_outputs` (from `status --ignored` counts).

The candidate never carries `Signed-off-by`. The maintainer adds it after review, under the DCO.

**`mythhelm.toml`** (§13.7 subset; unknown keys exit 2, via BurntSushi/toml `MetaData.Undecoded`):

```toml
schema_version = 1
[environment]
passthrough = ["GOPATH", "GOCACHE", "GOMODCACHE", "GOTOOLCHAIN", "GOFLAGS"]
[adapters.claudecode]
allowed_tools = ["Bash(go build *)", "Bash(go test *)", "Bash(go vet *)", "Bash(gofmt *)", "Bash(git status)", "Bash(git diff *)"]
[[checks]]
name = "gofmt"
argv = ["gofmt", "-l", "."]
fail_on_output = true
timeout = "2m"
[[checks]]
name = "vet"
argv = ["go", "vet", "./..."]
timeout = "5m"
[[checks]]
name = "test"
argv = ["go", "test", "./..."]
timeout = "15m"
```

The file is read from the **admitted snapshot**. Its SHA-256 is the `project_config` trust digest. Admission shows the checks, allowed tools and passthrough names, and asks for approval, or accepts `--trust-project-config sha256:<hex>`. The grant is stored per `(repo_identity = realpath of the source repo, digest)` (AC-7.1, AC-7.2, I03). If there is no `mythhelm.toml`, admission exits 3 (`no_checks_configured`) unless `--no-checks` is given. With `--no-checks`, the run ends `ready_for_review` with reason `unverified` so the candidate can be reviewed, but it exits **5** (`verification_unavailable`, §15.10 "required verification remained unavailable"), never 0. The receipt and summary say `verification: NOT RUN (waived by --no-checks)`, and `apply` requires `--accept-unverified`.

**Check runner** (FR-7). Each check runs in `git worktree add --detach runs/<id>/verify/<ver> <candidate>` (the linked worktree belongs to MYTHHELM's clone, not the user's), in sequence, with argv and no shell. It gets the allowlisted environment, its timeout (a process-group kill, as §3), and stdout plus stderr merged into a ring of at most 1 MiB, redacted, and written to `evidence/<ver>/<name>.log` with its SHA-256. Status is one of these:

- `passed`: exit 0, and no output if `fail_on_output`.
- `failed`.
- `timed_out`.
- `unavailable`: `exec.ErrNotFound`.
- `not_run`: a prior check was `unavailable` and `--keep-going` was not given.

Baseline checks on the base revision do not run in this slice; the receipt says `baseline: not-run`.

**Apply** (FR-8, §11.7).

1. Reconcile first: if `refs/heads/<b>` exists and equals the candidate, record `completed`; if it exists elsewhere, block.
2. Otherwise, preflight: the run is `ready_for_review`, `check-ref-format --branch <b>` passes, `<b>` does not exist, `cat-file -e <base_rev>^{commit}` succeeds in the user's repository, and the flags were accepted.
3. Journal `apply.intent_recorded`.
4. `git -C <user repo> -c core.hooksPath=<empty MYTHHELM hooks dir> -c core.fsmonitor=false -c gc.auto=0 -c maintenance.auto=false -c fetch.writeCommitGraph=false fetch --no-write-fetch-head --no-tags <workspace> refs/mythhelm/candidates/<att>:refs/heads/<b>`. There is no `+`, so this never forces.
5. Verify with `rev-parse` that `<b>` equals the candidate, then journal `apply.completed`.

The user's working tree, index and `HEAD` are never touched; only objects and the one new ref are written.

### 9. Receipt (§17.1 subset) — `runs/<id>/receipt.json`, schema_version 1

```json
{"schema_version":1,"run_id":"run_…","state":"ready_for_review","exit_code":0,
 "requested_outcome":{"task_file_sha256":"…","title":"…","deliverable":"review-candidate"},
 "admitted_snapshot":{"source_repo":"…","branch":"main","base_rev":"…","dirty_at_admission":false},
 "execution_bundle":{"harness":"claude-code","adapter":"builtin/claudecode@0.1.0","surface":"native-cli-structured (print, stream-json)",
   "native_version":"2.1.284","native_sha256":"…","model":"<native-reported>","permission_mode":"acceptEdits","allowed_tools":["…"],
   "execution_profile":"trusted-host (not contained)","compatibility":"fixture-tested on 2.1.284"},
 "fidelity_differences":["headless print mode: no approval bridge; prompts denied","DISABLE_AUTOUPDATER=1","CLAUDE_CODE_STARTUP_FAILURE_RESULTS=1"],
 "native_configuration":{"settings_digests":{"user":"…","project":"…"},"hooks":2,"mcp_servers":["…"],"trust_grant":"native_config:sha256:…"},
 "billing":{"mode":"subscription-declared","qualified":false,"g05":"not-passed","credential_provenance":"native-login",
   "entitlement_class":"included-plan","entitlement_source":"user_declared+native_status","paid_continuation":"unknown",
   "paid_continuation_user_declaration":"disabled","init_api_key_source":"none",
   "retail_equivalent_estimate_usd":{"value":"0.42","source":"native-reported","note":"estimate, not a charge"},
   "tokens":{"source":"native-reported","by_model":{}}},
 "routing":{"decision":"pinned by --adapter; no routing in this slice"},
 "native_result":{"attempt_state":"succeeded_native","subtype":"success","num_turns":9,"duration_ms":0,"session_id":"…","permission_denials":[]},
 "candidate":{"base_rev":"…","commit":"…","tree_id":"…","patch_sha256":"…","changed_paths":3,"flags":[],"integration":"single candidate on admitted snapshot; candidate is the combined revision"},
 "verification":{"config_sha256":"…","baseline":"not-run","checks":[{"name":"test","status":"passed","exit_code":0,"evidence":"evidence/…/test.log","sha256":"…"}]},
 "external_effects":[],"execution_host":{"os":"linux","arch":"amd64"},"host_integration":"standalone (Herdr out of scope)",
 "unknowns":["whether any inference request preceded the init event","native telemetry egress","descendants outside the process group"],
 "remaining_human_action":["review the diff","mythhelm apply <run> --to-branch <b>","sign off (DCO) after review"]}
```

The receipt is written atomically (temp file plus rename) and journaled with its SHA-256. `apply` adds an `external_effects` entry and rewrites the receipt as version 2. The earlier receipt is kept as `receipt.v1.json`.

### 10. CLI surface and exit codes (§15.10)

| Command | Flags (slice) |
|---|---|
| `run` | `--task-file` (required), `--adapter claudecode\|fake` (required), `--billing subscription-declared\|subscription-only\|local-scripted`, `--execution-profile trusted-host`, `--declare-entitlement`, `--use-committed`, `--rev`, `--trust-project-config`, `--trust-native-config`, `--strip-credential-env`, `--no-checks`, `--keep-going`, `--allow-untested-native-version`, `--max-duration`, `--capture-raw`, `--format plain\|jsonl`, `--plain`, `--non-interactive`, `--scenario` (fake only), `--host` (only `standalone`; `herdr` exits 7) |
| `runs list` | `--format`, `--limit` |
| `review <run>` | `--format`, `--no-diff` |
| `apply <run>` | `--to-branch` (required), `--accept-flags`, `--accept-unverified`, `--format` |
| `stop <run>`, `recover <run>` | `--format`, `--non-interactive` |
| `demo` | `--scenario happy\|check-fails`, `--keep` |
| `doctor`, `version` | `--format` |

Flags are parsed by stdlib `flag`, with an interspersed-argument loop so that `apply run_1 --to-branch x` works (D1).

| Exit | Category | Produced by |
|---|---|---|
| 0 | — | `ready_for_review`, `completed`; successful read-only commands |
| 2 | `invalid_arguments` | flag/config errors, unknown `mythhelm.toml` keys, newer DB schema |
| 3 | `admission_blocked` | billing, trust, dirty checkout, active run, native auth/billing, route mismatch, persistence unavailable at admission |
| 4 | `native_failed` | `failed_native` outcomes (§6.4) |
| 5 | `verification_failed` / `verification_unavailable` | a check failed, timed out or was unavailable; or `--no-checks` (`ready_for_review`/`unverified`) |
| 6 | `ownership_unresolved` | detach before stop confirmation, `interrupted`, owner lock held, journal failure mid-run |
| 7 | `capability_unavailable` | Windows + claudecode, restricted/inspect, unsupported repo feature, untested native version, `--host herdr`, git < 2.30 |
| 130 | `cancelled` | foreground stop confirmed |

### 11. Errors and logging (§12.7, §17.5)

Errors are typed. `*admission.BlockedError{Code, Field, Action}` maps to exit 3 or 7; `ErrOwnership` maps to 6. The CLI maps errors to exit codes in one place (`internal/cli/exit.go`). Unexpected errors exit 1, category `internal`, with the message redacted. There is no `panic` recovery outside the worker's top level. There, a panic journals `attempt.state_changed{interrupted, worker_panic}` before the process exits, because the native child must still be stopped.

Logging uses stdlib `log/slog` with a `security.RedactingHandler`. Its patterns cover `sk-ant-…`, `Bearer …`, `gh[pousr]_…`, `github_pat_…`, `AKIA…`, and `(?i)(api[_-]?key|token|secret|password)\s*[:=]\s*\S+`, and it drops any attribute named like a credential variable. Logs go to stderr at `warn` (`--log-level` to change) and to `runs/<id>/supervisor.log` at `info`, 0600. The journal and logs hold counts and identifiers, never prompt, assistant or tool text (NFR-2). `security.TermSafe(string)` removes C0 and C1 controls except `\n` and `\t`, plus all ESC sequences, before anything reaches a terminal (AC-8.2).

### 12. Test strategy

- **Unit tests** in every package. The named tests are listed in each task.
- **Contract fixtures.** `adapters/claudecode/testdata/streams/*.jsonl` start as synthetic streams built from the docs, each headed with a `# synthetic` marker line (skipped by the loader), until Task 20 replaces them with a sanitised recording. `TestDecodeFixture/<name>` checks the observations golden-style.
- **Launch path without credentials.** `fakeclaude` is a helper-process mode of the test binary (`GO_WANT_FAKECLAUDE=1`). The adapter is pointed at it through `ProbeInput.ExecutableOverride`, a test-only field that is rejected unless `testing.Testing()` is true. `fakeclaude` asserts the argv, that stdin carries the prompt, and the environment's allowlist. It then replays a fixture, including the `apiKeySource` mismatch.
- **Fault injection** (§18.4 subset, AC-11.3). Fake scenarios, plus harness helpers that forge `worker.json` start times (PID reuse), hold `owner.lock` from a helper process, and hold an exclusive SQLite transaction.
- **E2E** (`tests/e2e`). Builds the real binary once. Covers I08 fingerprints, the offline demo, the JSONL stdout contract, paths with spaces and Unicode (the macOS state dir has a space by default), and fake runs on Windows.
- **Live canary.** `adapters/claudecode/live_test.go` is behind `//go:build live` and requires `MYTHHELM_LIVE_CLAUDE=1`. It runs one trivial task in a temp repo and writes a sanitised fixture to `testdata/streams/recorded-<ver>.jsonl`. It is never in CI (no build tag there) and never run by default, because it consumes the maintainer's allowance (§18.3).

### 13. CI additions (release-engineer)

- **golangci-lint.** `.golangci.yml` with `version: "2"`, the standard set plus `gosec`, `errorlint`, `misspell` (`locale: UK`), `bodyclose` and `nolintlint`, and `gofmt` and `goimports` as formatters. It runs via `golangci/golangci-lint-action@v9.3.0` pinned by SHA, with `version: v2.14.0`.
- **govulncheck.** `golang/govulncheck-action@v1.1.0` pinned by SHA, with `go-version-file: go.mod` and `repo-checkout: false` after our own checkout (`persist-credentials: false`).
- **Dependabot.** Add the `gomod` ecosystem, with a 7-day cooldown.
- **CodeQL.** Add `go` to the matrix. This creates a new required check, `Analyze (go)`, so the maintainer updates the ruleset.
- **Supply chain.** OSV-Scanner, and go-licenses against the dependency-review allowlist.
- **Runner matrix.** Add `ubuntu-24.04-arm` and `windows-11-arm`.
- **`docs/automation.md`.** Move each of these from Deferred to Active.

Every `uses:` is pinned to a full SHA, resolved at implementation time. The SHAs are not recorded here because they are unverified until then.

### 14. Decisions

| ID | Decision | Rationale |
|---|---|---|
| D1 | stdlib `flag` + ~60-line dispatcher (not cobra/urfave) | Zero dependencies for ~10 verbs; interspersed flags handled by a re-parse loop. Revisit at the `completion` verb (§16.3), where urfave/cli v3 (MIT, zero deps) is the leading candidate. cobra is 2–3 modules with a slower release cadence. |
| D2 | `modernc.org/sqlite` (BSD-3, v1.x) over `ncruces/go-sqlite3` (MIT, pre-1.0) | §6.4 correctness first: it uses SQLite's own C VFS and locking (transpiled), is stable v1, has the widest adoption and the broadest platform list. Its cost is about 11 transitive modules and a history of retracted releases, so the version is pinned exactly: the newest release at least 7 days old at implementation, per the repo's cooldown policy. Never mix the two drivers on one file. |
| D3 | Hand-rolled ULID-format IDs in `internal/ids` (48-bit ms + 80-bit `crypto/rand`, Crockford base32, prefixed) | About 40 lines, time-sortable, and no dependency. oklog/ulid only recently resumed maintenance. |
| D4 | `github.com/BurntSushi/toml` v1.6.0 (MIT, no deps) | §13.7 proposes TOML, and `Undecoded()` gives strict unknown-key errors. |
| D5 | `golang.org/x/sys` | Process groups, flock, `LockFileEx`, process start times; already transitive through modernc. |
| D6 | Detached worker + file spool, not the socket IPC (§6.5) | Gives honest worker survival on Unix now, with no daemon. Socket IPC arrives with the TUI slice. ADR 0004. |
| D7 | `acceptEdits` + `--permission-prompts none` + trusted allowlist | A "preapproved restrictive envelope" (§9.3): it never waits, never bypasses, and denials are reported (AC-5.8). |
| D8 | User-declared posture named `subscription-declared`; strict mode still blocks | I15 cannot be met without §13.10 qualification. A distinct, loudly labelled name keeps the strict contract intact. ADR 0002, maintainer decision. |
| D9 | Launch the symlink-resolved versioned binary | Exact executable identity (§9.1 probe) and no mid-run auto-update swap. |

### 15. Honesty register: where the slice is weaker than the master spec

| Spec demand | Slice position |
|---|---|
| I15 / G05 strict included-only | Not met. Strict mode blocks; the declared posture is labelled `qualified:false` everywhere. |
| §6.5 per-user supervisor, IPC, peer checks | Deferred. The per-run OS lock plus spool is single-owner but not multi-client. |
| §7.4 Windows Job Objects | Deferred. `claudecode` is blocked on Windows; the fake adapter is a single process. |
| §11.5 steps 4–7 integration train | One candidate on the admitted base; stated in the receipt. |
| §11.6 baseline failures | `baseline: not-run` is recorded, not hidden. |
| §12.2 `restricted` | Blocked (exit 7). Only `trusted-host` with explicit consent is admitted. |
| G02 native fidelity (direct-vs-integrated comparison) | Not run. Receipt compatibility says `fixture-tested`, and the first `live-qualified` evidence is Task 20's single canary. |
| G04 detach/crash/orphan on every platform | Linux and macOS only; Windows is fake-only. |
