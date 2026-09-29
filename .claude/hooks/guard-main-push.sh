#!/usr/bin/env bash
# guard-main-push.sh: PreToolUse hook (Bash). A `git push` that can update `main`
# runs only inside an armed window of this session. Work reaches main through pull
# requests; the ruleset on GitHub already refuses direct pushes, so this guard
# protects local clones, forks, and a ruleset that is misconfigured or relaxed.
#
# A push can update main when:
#   * a refspec's destination is main (main, refs/heads/main, heads/main, src:main);
#   * it sends HEAD or @ without a destination (the current branch may be main);
#   * it names no refspec at all (push.default decides, so it is treated as main);
#   * it pushes every branch (--all, --branches, --mirror), or uses a glob refspec;
#   * a refspec is not literal ($VAR, a substitution): the guard cannot read it.
# `--tags` alone pushes only tags, so it is left to guard-publish.sh.
#
# Blocked even inside an armed window: deleting the remote main (`:main`,
# `--delete main`) and force-pushing main (--force, -f, --force-with-lease, +main).
#
# Arming is witnessed here (see hook-helpers.sh, section 5):
#   scripts/harness/arm-main-push.sh --reason "<why>"     arm this session for 30 min
#   scripts/harness/arm-main-push.sh --operator "<why>"   arm, audited as an operator override
#   scripts/harness/arm-main-push.sh --disarm             end the window early
# Arms and pushes are evaluated in command order, so `arm ... && git push origin main`
# works in one call. The `--publish` form arms guard-publish.sh, not this guard;
# `--disarm` without `--publish` disarms both.
#
# Residuals: a push inside a script file (`bash release.sh`), a git alias that
# expands to push, a configured remote.<name>.push mapping, and a non-literal
# command word (`$GIT push`) are invisible to command-text inspection.
#
# Exit 0 allows. Exit 2 blocks, with the BLOCK/Command/Detail/Fix stanza on stderr.

set -euo pipefail
export PATH="${PATH:-/usr/local/bin:/usr/bin:/bin}"

HOOK_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=hook-helpers.sh
. "$HOOK_DIR/hook-helpers.sh"

hh_load_bash_payload guard-main-push 'git.*push|arm-main-push' || exit 0

DATA_DIR="$(hh_data_dir "${CLAUDE_PROJECT_DIR:-}" "$HH_CWD" "$PWD")"
KIND=main-push

is_main_ref() {
  case "$1" in main|refs/heads/main|heads/main) return 0 ;; esac
  return 1
}

# classify_push <args after `push`>: prints NONE, MAIN, BARE, DELETE or FORCE.
classify_push() {
  local w skip=0 remote_seen=0 repo_opt=0 del=0 force=0 all=0 tags=0 hits=0 ref dst
  local -a refs=()
  for w in "$@"; do
    if (( skip == 1 )); then skip=0; continue; fi
    case "$w" in
      --delete) del=1; continue ;;
      --force|--force-with-lease|--force-with-lease=*) force=1; continue ;;
      --all|--branches|--mirror) all=1; continue ;;
      --tags) tags=1; continue ;;
      --repo) repo_opt=1; skip=1; continue ;;
      --repo=*) repo_opt=1; continue ;;
      -o|--push-option|--receive-pack|--exec) skip=1; continue ;;
      --) continue ;;
      --*) continue ;;
      -*) [[ "$w" == *d* ]] && del=1
          [[ "$w" == *f* ]] && force=1
          continue ;;
    esac
    if (( remote_seen == 0 && repo_opt == 0 )); then remote_seen=1; continue; fi
    refs+=("$w")
  done

  for ref in ${refs[@]+"${refs[@]}"}; do
    if [[ "$ref" == +* ]]; then force=1; ref="${ref#+}"; fi
    if [[ "$ref" == *'$'* || "$ref" == *'`'* || "$ref" == *[\*\?\[]* ]]; then
      hits=1; continue
    fi
    if [[ "$ref" == *:* ]]; then
      dst="${ref##*:}"
      if [[ "$ref" == :* ]] && is_main_ref "$dst"; then echo DELETE; return 0; fi
      [[ -z "$dst" ]] && { hits=1; continue; }
    else
      dst="$ref"
      if (( del == 1 )) && is_main_ref "$dst"; then echo DELETE; return 0; fi
      [[ "$dst" == HEAD || "$dst" == @ ]] && { hits=1; continue; }
    fi
    is_main_ref "$dst" && hits=1
  done

  (( all == 1 )) && hits=1
  if (( hits == 1 )); then
    (( force == 1 )) && { echo FORCE; return 0; }
    echo MAIN; return 0
  fi
  if (( ${#refs[@]} == 0 && tags == 0 )); then
    (( force == 1 )) && { echo FORCE; return 0; }
    echo BARE; return 0
  fi
  echo NONE
}

block() {   # block <detail> <fix>
  hh_audit "$DATA_DIR" "$KIND" block "" false
  hh_block "$1" "$2"
}

ARM_FIX="Merge through a pull request. If a direct push to main is truly intended and the operator asked for it, run scripts/harness/arm-main-push.sh --reason \"<why>\" in this session first (30-minute window), then retry."

mapfile -t SEGMENTS < <(hh_command_segments "$HH_COMMAND")
for seg in ${SEGMENTS[@]+"${SEGMENTS[@]}"}; do
  hh_split_words "$seg"
  hh_cmd_index || continue

  if hh_arm_parse; then
    case "$HH_ARM_ACTION:$HH_ARM_KIND" in
      arm:main-push) hh_sentinel_arm "$DATA_DIR" "$KIND" "$HH_ARM_REASON" "$HH_ARM_OPERATOR" ;;
      disarm:*) hh_sentinel_disarm "$DATA_DIR" "$KIND" ;;
    esac
    continue
  fi

  cmd_word="${HH_WORDS[HH_CI]}"
  [[ "${cmd_word##*/}" == git ]] || continue
  hh_git_parse "" || continue
  [[ "$HH_GIT_SUB" == push ]] || continue

  case "$(classify_push ${HH_GIT_ARGS[@]+"${HH_GIT_ARGS[@]}"})" in
    NONE) ;;
    DELETE)
      block "deleting the remote main branch is never allowed from an agent session, armed or not." \
        "If the deletion is truly intended, the operator runs it from their own terminal." ;;
    FORCE)
      block "force-pushing main (--force, -f, --force-with-lease or a +refspec) is never allowed from an agent session, armed or not. It rewrites history every contributor builds on." \
        "Push a new commit that reverts or fixes the change instead. If a rewrite is truly intended, the operator runs it from their own terminal." ;;
    MAIN|BARE)
      if hh_sentinel_status "$DATA_DIR" "$KIND"; then
        hh_audit "$DATA_DIR" "$KIND" allow "" false
        continue
      fi
      case "$HH_ARM_STATUS" in
        expired)
          block "this session's main-push window has expired (it lasts 30 minutes)." "$ARM_FIX" ;;
        unattributable)
          block "this push can update main, and the payload carries no session id, so no armed window can apply to it." \
            "Merge through a pull request, or have the operator push from their own terminal." ;;
      esac
      if [[ "$(classify_push ${HH_GIT_ARGS[@]+"${HH_GIT_ARGS[@]}"})" == BARE ]]; then
        block "a git push that names no refspec is treated as a push to main (push.default decides what it sends), and this session has no armed main-push window." \
          "Name the branch explicitly: git push origin <branch>. $ARM_FIX"
      fi
      block "this push can update main (a main destination, HEAD, --all/--mirror, a glob, or a refspec the guard cannot read literally), and this session has no armed main-push window." \
        "Push your branch by its literal name (git push origin <branch>) and open a pull request. $ARM_FIX" ;;
  esac
done

exit 0
