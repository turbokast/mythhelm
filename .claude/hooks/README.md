# Claude Code hooks

Hooks are scripts that Claude Code runs around tool calls. They give the harness rules teeth: a rule that matters is enforced by a hook, a CI check or a gate, not by prose alone. They are registered in [`.claude/settings.json`](../settings.json), and [`INVENTORY.md`](INVENTORY.md) is the canonical list of what each one does.

Hooks bind only tool calls made by an agent inside Claude Code. Your own terminal is never affected.

## How a hook runs

Claude Code starts the registered command for every matching tool call and writes a JSON payload to its stdin. For a `PreToolUse` hook on `Bash`:

```json
{
  "session_id": "abc123",
  "cwd": "/path/to/checkout",
  "hook_event_name": "PreToolUse",
  "tool_name": "Bash",
  "tool_input": { "command": "git push origin main" }
}
```

An `Edit` or `Write` payload carries `tool_input.file_path` and either `content` or `old_string`/`new_string`.

Hooks resolve the repository from the payload's `cwd` and `$CLAUDE_PROJECT_DIR` with `git rev-parse`, never from a hard-coded path. All hooks on one matcher run in parallel, so no hook may depend on another having run first.

## Exit-code contract

| Exit | Meaning | Who sees what |
|---|---|---|
| `0` | Allow. | Stdout is parsed as JSON when it is a JSON object; stderr goes to the debug log only. |
| `2` | Block. | Stderr is returned to the agent as the reason. On `PreToolUse` the tool call does not run. |
| other | Non-blocking error. | The call proceeds; the agent never sees stderr. |

A blocking hook therefore prints its reason on stderr and exits 2. The guards here print a four-line stanza:

```text
BLOCK: <hook>
Command: <the command that was blocked>
Detail: <what is wrong, and why it matters>
Fix: <what to do instead>
```

Two consequences shape every guard:

- **Fail closed when parsing fails.** A crash exits non-zero but not 2, which lets the call through. A guard that cannot parse (jq missing, bash older than 4, a malformed payload) blocks when the raw payload names a command it guards, and allows everything else. The `HOOK_JQ_PROBE` variable exists so tests can simulate a missing jq.
- **Advice never blocks.** A hook that only informs exits 0 and puts its message in `hookSpecificOutput.additionalContext` as JSON on stdout.

## How the Bash guards read a command

The guards never search the raw command text for a substring. `hook-helpers.sh` tokenizes it the way the shell would, and each guard checks only the words in **command position**:

- Quoted arguments and heredoc bodies are data. `git commit -m "never git push --force" -- a.go` is a commit.
- A heredoc that feeds a shell (`bash <<EOF`) is command text.
- `;`, `&&`, `||`, `|`, `&`, subshells and brace groups start new commands.
- Command substitutions (`$( … )` and backticks) are checked as commands of their own, and stand in their parent command as a non-literal word.
- Wrappers are skipped: `env`, `command`, `sudo`, `timeout`, `nohup`, `xargs`, `if`, `!` and others.
- `bash -c '…'`, `sh -c`, `eval` and `ssh host "…"` payloads are checked recursively.
- Quote tricks collapse as the shell collapses them: `g"it" st"ash"` is `git stash`.
- For git, global options are skipped (`git -C dir -c k=v push`), and the directory a command runs in follows the payload's `cwd`, literal `cd <dir>` segments and `git -C <dir>`.

What a command-text hook cannot see: a command inside a script file (`bash release.sh`), a git or gh alias defined outside the session, and a non-literal command word (`$GIT push`). Those residuals are accepted; the guards exist to stop mistakes, and CI plus the repository ruleset are the backstop.

## Armed windows

`guard-main-push.sh` and `guard-publish.sh` allow their guarded actions only inside an **armed window** of the current session. Arming is witnessed by the hook: when the guard sees the arming command in command position, it writes the sentinel itself, stamped with the `session_id` of that payload. The script only validates arguments and reports. It cannot know the session id, so it never writes a sentinel.

```bash
# A direct push to main, when the operator asked for one:
scripts/harness/arm-main-push.sh --reason "hotfix the operator requested" && git push origin main

# Publishing (a release, a v* tag, a workflow run, a settings change):
scripts/harness/arm-main-push.sh --publish --reason "cut v0.1.0 as the operator asked"
gh release create v0.1.0 --notes-file notes.md

# An operator override, recorded as such in the audit log:
scripts/harness/arm-main-push.sh --operator "operator: re-run the release workflow"

# End the windows early (the 30-minute limit is the backstop):
scripts/harness/arm-main-push.sh --disarm             # every window of this session
scripts/harness/arm-main-push.sh --publish --disarm   # the publish window only
```

The rules:

- **A reason is mandatory.** An invocation the script would reject (no reason, an unknown flag, `--disarm` with a reason) never arms.
- **Arm only on request.** An agent arms only when the operator has asked for the guarded action. Normal work lands through pull requests and needs no arming.
- **Per session, per kind.** A window authorizes only the session that armed it, and a main-push window does not authorize publishing.
- **30 minutes, fixed.** A window allows several actions until it expires or is disarmed. The limit cannot be configured; an expired or future-dated sentinel never authorizes, and stale sentinels are swept.
- **Some actions are never allowed**, armed or not: deleting or force-pushing `main`, and deleting the repository (`gh repo delete`, or `gh api -X DELETE repos/<owner>/<repo>`). The operator runs those from their own terminal.
- **Commands are evaluated in order**, so an arm and the action can share one Bash call, and an action before the arm is still blocked.

State lives in the main checkout's `.claude/data/` (gitignored), shared by its linked worktrees:

| File | Contents |
|---|---|
| `main-push-arm-<session>.json`, `publish-arm-<session>.json` | `session_id`, `kind`, `armed_at`, `armed_at_epoch`, `ttl_seconds`, `reason`, `operator` |
| `guard-audit.jsonl` | One row per arm, disarm, allow and block: `ts`, `session_id`, `guard`, `kind`, `event`, `command` (first 200 characters), `reason`, `operator` |

## The task-completion Stop hook

`verify-task-completion.sh` runs on `Stop` and `SubagentStop`. It does nothing unless this session marked a spec task complete in its working tree; then it keeps the session working until the entry is well-formed and every gate the change set needs has a fresh marker from `scripts/harness/gate.sh`. Its block names the exact `gate.sh` commands. [`WORKFLOW.md`](../../WORKFLOW.md) explains the markers, and [`knowledge/execution.md`](../../knowledge/execution.md) why each part exists.

When the block is wrong, record why and stop; the override covers only this session, this tree and its current bytes:

```bash
python3 scripts/harness/gatelib.py override --session <session id from the block> --reason "<why the block is wrong>"
```

| File in the main checkout's `.claude/data/` | Contents |
|---|---|
| `stop-gate-<session>.json` | Consecutive blocks per tree and fingerprint; three on an unchanged tree release the next `stop_hook_active` stop |
| `stop-gate-override-<session>.json` | `session_id`, `tree`, `fingerprint`, `reason`, `at` |
| `stop-gate-audit.jsonl` | One row per block, release and override (`.claude/data/stop-gate-audit.schema.json`) |

## The autonomy grant

The maintainer's autonomy grant lets one session run `/deliver-backlog` unattended for at most 24 hours over the cards and specs it names ([`knowledge/autonomy.md`](../../knowledge/autonomy.md)). Unlike an armed window, it is never armed by an agent: `scripts/orchestration/autonomy.sh grant` and `renew` refuse without a terminal, and `guard-autonomy.sh` blocks every agent call of them. The maintainer runs them with the `!` prefix inside the session to be granted (which binds that session), or from another terminal with `--session <id>`. Commands the maintainer types with `!` never reach the hooks.

Three hooks read the grant, and each applies it only to the session it names:

- `continue-run.sh` (Stop) turns an idle stop into the next action while `scripts/orchestration/delivery.py actionable` lists work, within the grant's continue budget, three chained continues and one continue a minute. A final line `AWAITING MAINTAINER: <reason>` always releases the session. It fails open.
- `guard-blocking-ask.sh` blocks AskUserQuestion, and also in any session started with `MYTHHELM_NONINTERACTIVE=1` (the heartbeat sets it). It fails open.
- `guard-autonomy.sh` blocks the arming script in the granted session, and every `gh pr merge` there except `gh pr merge <n> --squash --match-head-commit <sha>` after `autonomy.py merge-check` recorded a ready verdict at that head within 15 minutes. A grant file that is unreadable or out of bounds blocks merging and arming in every session until the maintainer revokes it. In every session it blocks writes to the grant, its audit log and the run's `INTENT.md`, `RUN-LOG.md` and `QUESTIONS.md` other than through `delivery.py`, and enabling the heartbeat timer. It fails closed.

| File in the main checkout | Contents |
|---|---|
| `.claude/data/autonomy-grant.json` | `schema_version`, `id`, `session_id`, `granted_at`, `issued_epoch`, `until_epoch`, `until`, `scope` (`cards`, `specs`), `allow_pm_sync`, `spec_checkpoint`, `max_continues`, `reason`, `granted_by`, `renewals` |
| `.claude/data/autonomy-audit.jsonl` | One row per grant, renew, revoke, merge verdict (`merge-ready`, with `pr`, `head`, `spec`, `at_epoch`), allowed merge, continue and heartbeat decision |
| `.claude/data/autonomy-continue.json` | `continue-run.sh`'s counters, keyed by grant id |

## Pruning merged branches

`block-destructive.sh` blocks `git branch -D`, `git update-ref -d` of a branch and `git worktree remove --force`: each deletes work without checking that it is merged anywhere, and after a squash merge `git branch -d` cannot tell either. `scripts/harness/prune-merged.sh` is the sanctioned path, and it is safe because it verifies before it deletes:

- the branch is not `main`, the default branch or a checked-out branch of the main checkout or the current worktree;
- `gh pr view <branch>` reports its pull request `MERGED` with a merge time;
- the local tip equals the pull request's head or is an ancestor of it, so no local commit is lost;
- a linked worktree holding it is clean, and is removed without `--force`;
- the delete is `git update-ref -d refs/heads/<branch> <tip>`, a compare-and-swap that fails if the branch moved after the check.

It is a dry run unless given `--apply`, deletes only local branches and worktrees, and never touches the remote. The hook cannot see commands inside a script, which is why the script, and only the script, carries the checks; `test_block_destructive.sh` pins that the unverified spellings stay blocked.

## Adding a hook

1. Write `.claude/hooks/<name>.sh`: `#!/usr/bin/env bash`, `set -euo pipefail`, and a header comment stating what it blocks or reports, why, and its residuals. A Bash guard sources `hook-helpers.sh` and calls `hh_load_bash_payload <name> <prefilter>`, then walks `hh_command_segments`. Use `hh_block` for the stanza.
2. Make it executable (`chmod +x`).
3. Write `.claude/hooks/tests/test_<name>.sh`. Source `lib.sh`, feed payloads with `bash_payload`, and assert with `expect_rc`, `expect_err` and `expect_stanza`. Cover the blocked forms, the bypass spellings (quoting, `bash -c`, `eval`, wrappers, chains, `git -C`, `cd`), the allowed look-alikes (commit messages, heredocs, `echo`), and the fail-closed paths. Keep commands under test inside the test file as data, and point `CLAUDE_PROJECT_DIR` at a fixture repository so no test touches the real `.claude/data/`.
4. Register it in `.claude/settings.json` with `"$CLAUDE_PROJECT_DIR/.claude/hooks/<name>.sh"`.
5. Add its row to `INVENTORY.md`.
6. Run `.claude/hooks/tests/run-tests.sh` and `shellcheck` on the new files.

## Testing a hook by hand

Pipe a payload in and read the exit code. The command under test is data inside the JSON, never a command on your own line:

```bash
jq -nc '{session_id:"manual", cwd:env.PWD, tool_input:{command:"git stash"}}' \
  | .claude/hooks/block-destructive.sh; echo "exit $?"
```

To disable a hook temporarily, remove its entry from `.claude/settings.json`. Hooks are independent, so the others keep working.
