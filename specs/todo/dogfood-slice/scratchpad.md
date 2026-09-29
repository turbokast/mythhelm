# Dogfood Slice — Scratchpad

> Research notes and open questions from spec authoring (2026-09-29). Each implementing task adds an entry under Discoveries when it finishes, and reads this file when it starts.

## Open questions

Each question has a conservative default, which the spec already assumes (§21: "Open questions are not permission to omit implementation").

| # | Question | Default in this spec | Owner / blocks |
|---|---|---|---|
| Q1 | **Can strict `subscription-only` (I15, G05) ever be met for Claude Code?** `claude auth status` exposes `subscriptionType`, but no documented, non-billable surface reports whether extra usage or [usage credits](https://support.claude.com/en/articles/12429409-manage-usage-credits-for-paid-claude-plans) can fund continuation. The §13.10 step-3 "prevention evidence" therefore has no source today. | Strict mode blocks. `subscription-declared` is a separate, loudly labelled posture (D8, ADR 0002). | **Decided 2026-09-29 (maintainer):** `subscription-declared` is the posture used for dogfooding, labelled unqualified everywhere. `--billing` stays required with no default (AC-4.1); the dogfood instructions pass `--billing subscription-declared` explicitly. Strict `subscription-only` stays and keeps blocking. |
| Q2 | **Terms fit.** Anthropic's legal page says Pro/Max limits "assume ordinary, individual usage of Claude Code and the Agent SDK". It also says third parties may not "route requests through Free, Pro, or Max plan credentials on behalf of their users", while it does not prevent "an end user from signing in to the unmodified Claude Code binary with their own Claude subscription". MYTHHELM runs the user's own unmodified binary under the user's own login, which is the §13.1 path. Whether automated headless orchestration counts as "ordinary, individual usage" is not settled by the text. | Proceed for the maintainer's own dogfood use. Make no public "use your Max plan with MYTHHELM" claim until a terms review is recorded (§13.1). | Maintainer, before Task 20 is publicised and before any release notes. |
| Q3 | The headless docs say `--bare` "will become the default for `-p` in a future release". Bare mode refuses OAuth and skips hooks, skills, `CLAUDE.md` and MCP. That conflicts with §9.3 (fidelity) and would break subscription auth. | The compatibility floor and ceiling (`2.1.x`, x ≥ 284) plus fixtures catch the change; a bare default fails closed (auth fails, no API key reaches the child). | Adapter owner; watch release notes. |
| Q4 | `apiKeySource: "none"` covers the subscription, bearer-token and cloud routes alike. No documented init field distinguishes them. | Pre-launch environment and settings inventory, plus `auth status`, are the primary evidence; init is a secondary tripwire (design §6.2). | Adapter owner. |
| Q5 | Does `claude auth status` make network requests? | Treated as possibly networked: admission uses it, `doctor` does not. | Verify in Task 16 (strace / Little Snitch), then record the result. |
| Q6 | Can any inference request precede the `system/init` event? | Unknown. The receipt lists it under `unknowns`. | Task 20 canary evidence, if observable. |
| Q7 | `CLAUDE_CODE_OAUTH_TOKEN` (from `claude setup-token`) is an Anthropic-issued subscription token used by the unmodified binary. Should it be admitted instead of stripped? | **Decided 2026-09-29 (maintainer):** blocked by default. It is admitted only when the trusted *user-level* config (never the project `mythhelm.toml`, per §12.4) sets `[adapters.claudecode] credential = "oauth-token"`. MYTHHELM then passes it through to the native child unread, unlogged and uncopied, and records `credential_provenance: native-subscription-token`. See AC-4.7. | Adapter owner (Task 16). |
| Q8 | Does the native sandbox apply in `-p` mode? The docs are silent. | The `restricted` profile is deferred (N9). | Next slice. |
| Q9 | Print mode "silently ignores" settings files that fail validation, so the MYTHHELM inventory could see hooks the native ignores. | A parse failure in MYTHHELM blocks; a hook seen by MYTHHELM needs trust even if the native would ignore it. Both err on the safe side. | Task 16. |
| Q10 | modernc's Linux OFD locks are opt-in (`MODERNC_SQLITE_OFD_LOCK=1`). | Leave SQLite's default POSIX locking; revisit only on observed lock issues. | Task 4. |
| Q11 | Adding `Analyze (go)` creates a new required check. | The PR description asks the maintainer to update the ruleset. | Maintainer (Task 2). |
| Q12 | The per-user supervisor daemon and socket IPC (§6.5) arrive with the TUI slice. Does the worker's file spool stay as the transport? | The spool is kept as the worker's durable outbox. IPC becomes a notification path on top of it. | Next-slice design. |

## Master-spec items judged infeasible now, or needing a decision

1. **I15 / G05 for Claude Code**: see Q1. As written, strict mode may be permanently unsatisfiable until the vendor exposes an overage or extra-usage status signal, or policy accepts declared evidence.
2. **§9.3 "do not use bare mode"** will collide with the vendor's announced `-p` default change (Q3). The spec should say what happens then: block, or pass an explicit opt-out flag if one exists.
3. **§7.5 sole logical SQLite writer** implies a daemon. The slice uses one OS-locked supervisor per run plus SQLite transactions (ADR 0004). This needs acceptance as an interim model.
4. **§18.9 "account usage outside MYTHHELM consumes allowance concurrently → remain protected by provider/native stop boundary"**: no native signal exists, so the protection is entirely the vendor's.
5. **I10 hard limits**: `--max-budget-usd` exists natively but is estimate-based ("an estimate, not a billing statement"), so it can never back `--require-hard-limit`.

## Research notes

### Claude Code (checked 2026-09-29)

- **Local install.** `claude --version` prints `2.1.284 (Claude Code)`. `which claude` gives `~/.local/bin/claude`, which resolves to `~/.local/share/claude/versions/2.1.284` (a native ELF binary). The auto-updater swaps the symlink, so the adapter launches the resolved versioned path (D9).
- **`claude auth status`** (read-only, local) prints JSON with the keys `loggedIn, authMethod, apiProvider, analyticsDisabled, projectsDirectory, configDirectory, email, orgId, orgName, subscriptionType`. Values observed on a subscription login: `authMethod="claude.ai"`, `apiProvider="firstParty"`. Other enum values are unverified. It exits 0 when logged in and 1 when not. <https://code.claude.com/docs/en/cli-reference>
- **Print mode.** Flags: `-p`, `--output-format text|json|stream-json`, `--input-format text|stream-json`. `stream-json` output in print mode requires `--verbose`; the binary contains the error string "When using --print, --output-format=stream-json requires --verbose". Piped stdin is capped at 10 MB. SIGTERM gives exit 143 and leaves the turn unfinished. <https://code.claude.com/docs/en/headless>
- **Isolation.** "Without `--bare`, a `-p` session runs the hooks in a project's `.claude/settings.json` and connects the servers in its `.mcp.json`, even in a folder you've never trusted" (headless). This is why the design has native-configuration trust grants (AC-2.5).
- **Permission modes.** The modes are `default` (alias `manual`), `acceptEdits`, `plan`, `auto`, `dontAsk` and `bypassPermissions`.
  - `acceptEdits` auto-approves edits and `mkdir touch rm rmdir mv cp sed` inside the working directory only.
  - `--permission-prompts none` (v2.1.259+) denies anything that would prompt.
  - <https://code.claude.com/docs/en/permission-modes>, <https://code.claude.com/docs/en/cli-reference>
- **Auth precedence**, highest first (<https://code.claude.com/docs/en/authentication>):
  1. `CLAUDE_CODE_USE_BEDROCK`, `CLAUDE_CODE_USE_VERTEX` or `CLAUDE_CODE_USE_FOUNDRY`
  2. `ANTHROPIC_AUTH_TOKEN`
  3. `ANTHROPIC_API_KEY`
  4. `apiKeyHelper`
  5. `CLAUDE_CODE_OAUTH_TOKEN`
  6. profile or federation credentials
  7. subscription OAuth from `/login`

  A signed-in apps gateway session outranks all of these. On the API key: "In non-interactive mode (`-p`), the key is always used when present."
- **Credentials storage**: macOS Keychain; Linux `~/.claude/.credentials.json` (mode 0600); Windows `%USERPROFILE%\.claude\.credentials.json`. MYTHHELM never reads any of these (I19).
- **Stream-json events** (<https://code.claude.com/docs/en/agent-sdk/typescript>):
  - `system/init` fields: `session_id`, `apiKeySource` (values: `ANTHROPIC_API_KEY`, `apiKeyHelper`, `/login managed key`, `none`), `claude_code_version`, `cwd`, `tools`, `mcp_servers`, `model`, `permissionMode`, `plugins`.
  - Hook or `plugin_install` events may come before init.
  - `result` success fields: `subtype`, `is_error`, `duration_ms`, `num_turns`, `result`, `stop_reason`, `total_cost_usd` ("an estimate, not a billing statement"), `usage`, `modelUsage`, `permission_denials[]`.
  - `result` error subtypes: `error_max_turns`, `error_during_execution`, `error_max_budget_usd`, `error_max_structured_output_retries`; these carry `errors[]` and `startup_failure_reason`.
  - `assistant.error` classes include `authentication_failed`, `billing_error`, `rate_limit`.
- **Environment variables** (<https://code.claude.com/docs/en/env-vars>):
  - `DISABLE_AUTOUPDATER` exists.
  - `CLAUDE_CODE_STARTUP_FAILURE_RESULTS=1` makes a stream-json session write a startup-failure result (v2.1.274+).
  - A settings file's `env` block overrides shell variables.
  - `CLAUDE_CONFIG_DIR` defaults to `~/.claude`.
- **Settings precedence**: managed > command line (`--settings`) > `.claude/settings.local.json` > `.claude/settings.json` > `~/.claude/settings.json`; `~/.claude.json` also holds MCP and trust state. Managed paths: macOS `/Library/Application Support/ClaudeCode/`, Linux `/etc/claude-code/`, Windows `C:\Program Files\ClaudeCode\`. <https://code.claude.com/docs/en/settings>, <https://code.claude.com/docs/en/managed-settings>
- **Sandbox**: Seatbelt on macOS, bubblewrap (plus `socat`) on Linux and WSL2; native Windows is not supported. It covers Bash, PowerShell and Monitor only. It is off by default. When dependencies are missing it falls back to unsandboxed unless `sandbox.failIfUnavailable` is set. <https://code.claude.com/docs/en/sandboxing>
- **Legal**: quotes in Q2. <https://code.claude.com/docs/en/legal-and-compliance>
- **Local `--help` vs docs**: some documented flags (for example `--max-turns` and `--permission-prompt-tool`) are missing from local `--help`. The docs say `--help` is not exhaustive.

### Go ecosystem (checked 2026-09-29)

- Go 1.27.0 was released 2026-08-19; the latest patch is go1.27.1 (2026-09-01). `actions/setup-go@v7` with `go-version-file` honours the `toolchain` directive. <https://go.dev/doc/devel/release>
- `modernc.org/sqlite` v1.60.1 (2026-09-29, same day, so pin an older release per the 7-day cooldown): BSD-3, SQLite 3.53.4, CGO-free, about 11 transitive modules including `google/uuid` and `x/sys`, with a history of retracted releases. <https://pkg.go.dev/modernc.org/sqlite>
- `ncruces/go-sqlite3` v0.35.6: MIT, pre-1.0, now wasm2go rather than wazero, with a custom VFS the README calls "not as battle tested". <https://github.com/ncruces/go-sqlite3>
- Benchmarks show no clear winner between the two. <https://github.com/cvilsmeier/go-sqlite-bench>
- CLI options:
  - `spf13/cobra` v1.10.2 (Apache-2.0; pflag plus mousetrap on Windows)
  - `urfave/cli/v3` v3.13.0 (MIT, zero dependencies)
  - `peterbourgon/ff/v4` (still beta)
  - Chosen: stdlib `flag` (D1).
- `BurntSushi/toml` v1.6.0 (MIT, no dependencies); `pelletier/go-toml/v2` v2.4.3 is the alternative.
- `golang.org/x/sys` v0.48.0 (BSD-3). It covers process groups, `unix.Flock`, `unix.SysctlKinfoProc`, `windows.LockFileEx`, `GetProcessTimes` and Job Objects (next slice).
- `golangci-lint` v2.14.0: config `version: "2"`, via `golangci/golangci-lint-action` v9.3.0. It is a CI tool only (GPL-3.0, nothing linked into the binary).
- `govulncheck` v1.8.0, via `golang/govulncheck-action` v1.1.0.

### Repo facts

- `ci.yml` Go job: `gofmt`, `go vet`, `go test -race`, `go mod tidy -diff` on ubuntu, macOS and windows, gated on `go.mod`.
- Dependency-review licence allowlist: Apache-2.0, MIT, BSD-2/3, ISC, MPL-2.0, Unlicense, 0BSD, CC0-1.0, Zlib, BSL-1.0.
- `docs/automation.md` lists what the first `go.mod` switches on (Tasks 2–3).
- CodeRabbit path instructions apply I01–I19 to `internal/**` and `adapters/**`.

## Discoveries

<!-- Append entries here. Format:
### Task N — YYYY-MM-DD HH:MM
- Discovery / gotcha / pointer
-->
