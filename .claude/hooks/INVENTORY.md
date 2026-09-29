# Hook inventory

The canonical list of hooks registered in `.claude/settings.json`: each script, the event and matcher it runs on, and whether it blocks. Change a hook's registration, matcher or posture and its row here in the same commit. `test_settings_registration.sh` fails when a registered hook is missing from this table or a hook script is not registered.

| Hook | Event / matcher | Posture | What it does | Tests |
|---|---|---|---|---|
| `block-destructive.sh` | PreToolUse / `Bash` | Blocks (exit 2) | Blocks `git stash` (except `list`/`show`), `git reset --hard`, `git clean -f`, `git branch -D`, checkout/restore/switch forms that discard the working tree, force pushes (`--force`, `-f`, `+refspec`, `--mirror`), and recursive `rm`/`find -delete` aimed at `/`, a top-level directory, `$HOME`, the repository root or an ancestor, or `.git`/`.claude`/`.github`. In the main checkout of this repository only, also blocks `git add -A`/`-u`/`.` and any `git commit` that does not name its paths. | `test_block_destructive.sh`, `test_block_destructive_shared_index.sh` |
| `guard-main-push.sh` | PreToolUse / `Bash` | Blocks (exit 2) | Blocks a `git push` that can update `main` unless this session holds an armed main-push window. Deleting or force-pushing `main` is blocked even when armed. Witnesses `scripts/harness/arm-main-push.sh` arming and disarming. | `test_guard_main_push.sh`, `test_arm_main_push_script.sh` |
| `guard-publish.sh` | PreToolUse / `Bash` | Blocks (exit 2) | Blocks releases, `v*` tag pushes, workflow runs and reruns, secrets and variables, repository settings, rulesets, `gh pr merge --admin` and write calls through `gh api` unless this session holds an armed publish window. Deleting the repository (`gh repo delete`, or a `gh api` DELETE of `repos/<owner>/<repo>`) is blocked even when armed. Witnesses `arm-main-push.sh --publish`. | `test_guard_publish.sh` |
| `guard-vendors.sh` | PreToolUse / `Bash`; PreToolUse / `Edit\|Write` | Blocks (exit 2); fails closed when it cannot parse a payload naming what it guards | Blocks direct calls of the Codex and Muse CLIs in any visible spelling (bare, absolute path, versioned binary, `npx`/`bunx`/`npm exec`/`pnpm dlx`), so every vendor call goes through its wrapper and the envelope in `scripts/vendors/`; blocks `vendors.py enable` and any write to `.claude/data/vendor-policy.local.json`, because only the contributor opts a checkout in to a paid vendor. Lookups, readers and mentions pass. | `test_guard_vendors.sh` |
| `validate-agent-config.sh` | PreToolUse / `Edit\|Write` | Blocks (exit 2); fails closed for `.claude/` edits when python3 is missing or validation errors | Validates the content an edit leaves in `.claude/agents/*.md`, `.claude/skills/*/SKILL.md`, `.claude/rules/*.md`, `.claude/hooks/*.sh` and `.claude/*.json`: frontmatter fields, model tiers, names, `paths:` globs, shell syntax, settings events and hook command paths. | `test_validate_agent_config.sh` |

## Sourced helpers and scripts

These are not registered as hooks.

| File | Role |
|---|---|
| `hook-helpers.sh` | Shared by every guard: payload loading, the block stanza, the command tokenizer, git and `cd` tracking, and the arming sentinels. Tested by `test_hook_helpers.sh`. |
| `tests/run-tests.sh` | Runs every `test_*.sh` under `.claude/hooks/tests/` and `scripts/**/tests/`. |
| `tests/lib.sh` | Assertions and fixtures shared by the tests. |
| `scripts/harness/arm-main-push.sh` | The arming command the guards witness. |
