#!/usr/bin/env bash
# guard-publish.sh: PreToolUse hook (Bash). Publishing and repository-settings
# actions on GitHub run only inside an armed publish window of this session.
# Releases, tags, workflow runs, secrets and repository settings are visible to
# every user of the project, and most cannot be taken back cleanly.
#
# Guarded (each needs the window):
#   gh release     anything but list|ls|view|download|verify|verify-asset
#   gh workflow    anything but list|ls|view            (run, enable, disable, ...)
#   gh run         anything but list|ls|view|watch|download (rerun, cancel, delete)
#   gh secret      anything but list|ls                 (set, delete, ...)
#   gh variable    anything but list|ls|get             (set, delete, ...)
#   gh ruleset     anything but list|ls|view|check
#   gh repo        create|edit|rename|archive|unarchive|transfer|deploy-key|autolink,
#                  and sync --force
#   gh pr merge --admin                                 (bypasses required checks)
#   gh alias set|import                                 (an alias can hide any of these)
#   gh api         DELETE, PATCH or PUT against repos/, orgs/ or enterprises/; POST
#                  (explicit, or implied by -f/-F/--field/--raw-field/--input) to a
#                  release, ref, tag, dispatch, workflow run, secret, variable,
#                  ruleset, key, hook, environment, merge or transfer endpoint, or to
#                  orgs/; a GraphQL mutation other than the review-thread ones below; a
#                  non-literal endpoint with a write method
#   git push       --tags, --follow-tags, --mirror, or a refspec whose source or
#                  destination is a v* tag (v1.2.3, refs/tags/v1.2.3, :refs/tags/v1),
#                  or a refspec the guard cannot read literally
# Blocked even inside an armed window: deleting the repository (gh repo delete,
# gh api DELETE repos/<owner>/<repo>, or a DELETE to an endpoint the guard cannot
# read). The operator runs it.
# Read-only gh commands pass, and so do GraphQL documents whose every mutation field
# is one of REVIEW_THREAD_MUTATIONS (replying to, resolving or unresolving a review
# thread is conversation, not publishing). A mutation document the guard cannot read
# field by field (from --input or a -F query=@file, a fragment spread among the
# mutation fields, an unterminated string) is gated.
#
# Arming is witnessed here (see hook-helpers.sh, section 5):
#   scripts/harness/arm-main-push.sh --publish --reason "<why>"   arm for 30 minutes
#   scripts/harness/arm-main-push.sh --publish --operator "<why>" arm as an operator override
#   scripts/harness/arm-main-push.sh --disarm                     end every window early
# An agent arms only when the operator has asked for the publishing action.
#
# Residuals: a command inside a script file, a gh alias or extension created
# outside the session, push.followTags set in git config, and a non-literal command
# word are invisible to command-text inspection.
#
# Exit 0 allows. Exit 2 blocks, with the BLOCK/Command/Detail/Fix stanza on stderr.

set -euo pipefail
export PATH="${PATH:-/usr/local/bin:/usr/bin:/bin}"

HOOK_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=hook-helpers.sh
. "$HOOK_DIR/hook-helpers.sh"

hh_load_bash_payload guard-publish '(^|[^a-z])gh([^a-z]|$)|git.*push|arm-main-push' || exit 0

DATA_DIR="$(hh_data_dir "${CLAUDE_PROJECT_DIR:-}" "$HH_CWD" "$PWD")"
KIND=publish

REVIEW_THREAD_MUTATIONS=" addPullRequestReviewThreadReply resolveReviewThread unresolveReviewThread "

POST_ENDPOINT_RE='^repos/.*/(releases|git/refs|git/tags|dispatches|actions/workflows|actions/runs|actions/secrets|actions/variables|rulesets|keys|hooks|environments|merges|transfer)(/|$)'

# gh_verdict <gh args...>: prints nothing (allow), GATE:<what> or NEVER:<what>.
gh_verdict() {
  local -a P=() F=()
  local w k skip=0 group sub
  local -a args=("$@")
  for (( k = 0; k < ${#args[@]}; k++ )); do
    w="${args[k]}"
    if (( skip == 1 )); then skip=0; continue; fi
    case "$w" in
      -R|--repo|--hostname) skip=1; continue ;;
      -*) F+=("$w"); continue ;;
    esac
    if (( ${#P[@]} == 0 )) && [[ "$w" == api ]]; then
      api_verdict "${args[@]:k+1}"
      return 0
    fi
    P+=("$w")
  done
  group="${P[0]:-}"
  sub="${P[1]:-}"
  [[ -n "$group" ]] || return 0
  case "$group" in
    release)
      case "$sub" in ''|list|ls|view|download|verify|verify-asset) ;; *) echo "GATE:gh release $sub" ;; esac ;;
    workflow)
      case "$sub" in ''|list|ls|view) ;; *) echo "GATE:gh workflow $sub" ;; esac ;;
    run)
      case "$sub" in ''|list|ls|view|watch|download) ;; *) echo "GATE:gh run $sub" ;; esac ;;
    secret)
      case "$sub" in ''|list|ls) ;; *) echo "GATE:gh secret $sub" ;; esac ;;
    variable)
      case "$sub" in ''|list|ls|get) ;; *) echo "GATE:gh variable $sub" ;; esac ;;
    ruleset|rs)
      case "$sub" in ''|list|ls|view|check) ;; *) echo "GATE:gh ruleset $sub" ;; esac ;;
    repo)
      case "$sub" in
        delete) echo "NEVER:gh repo delete" ;;
        create|new|edit|rename|archive|unarchive|transfer|deploy-key|autolink) echo "GATE:gh repo $sub" ;;
        sync) for w in ${F[@]+"${F[@]}"}; do [[ "$w" == --force* ]] && echo "GATE:gh repo sync --force"; done ;;
      esac ;;
    pr)
      if [[ "$sub" == merge ]]; then
        for w in ${F[@]+"${F[@]}"}; do [[ "$w" == --admin* ]] && { echo "GATE:gh pr merge --admin"; return 0; }; done
      fi ;;
    alias)
      case "$sub" in set|import) echo "GATE:gh alias $sub" ;; esac ;;
  esac
  return 0
}

# graphql_tokens <document>: one GraphQL token per line (names, numbers,
# punctuators, "..."), with strings and commas dropped. Prints "?" and stops at
# anything it cannot read: an unterminated string, or a comment (the command
# tokenizer turns newlines inside quotes into spaces, so a comment's end is lost).
graphql_tokens() {
  local s="$1" n=${#1} i=0 j c
  while (( i < n )); do
    c="${s:i:1}"
    case "$c" in
      [[:space:],]) i=$((i + 1)) ;;
      '#') echo '?'; return 0 ;;
      '"')
        if [[ "${s:i:3}" == '"""' ]]; then
          # A block string ends at the first """ not escaped as \""".
          j=$((i + 3))
          while (( j < n )); do
            if [[ "${s:j:4}" == '\"""' ]]; then
              j=$((j + 4))
            elif [[ "${s:j:3}" == '"""' ]]; then
              break
            else
              j=$((j + 1))
            fi
          done
          (( j < n )) || { echo '?'; return 0; }
          i=$((j + 3))
        else
          j=$((i + 1))
          while (( j < n )); do
            case "${s:j:1}" in
              \\) j=$((j + 2)) ;;
              '"') break ;;
              *) j=$((j + 1)) ;;
            esac
          done
          (( j < n )) || { echo '?'; return 0; }
          i=$((j + 1))
        fi ;;
      [_A-Za-z])
        j=$i
        while (( j < n )) && [[ "${s:j:1}" == [_A-Za-z0-9] ]]; do j=$((j + 1)); done
        echo "${s:i:j-i}"
        i=$j ;;
      [0-9+-])
        j=$i
        while (( j < n )) && [[ "${s:j:1}" == [0-9eE.+-] ]]; do j=$((j + 1)); done
        echo "${s:i:j-i}"
        i=$j ;;
      '.')
        [[ "${s:i:3}" == '...' ]] || { echo '?'; return 0; }
        echo '...'
        i=$((i + 3)) ;;
      *) echo "$c"; i=$((i + 1)) ;;
    esac
  done
}

# graphql_mutation_fields <document>: prints "@mutation" for each mutation
# operation, then the top-level field names of its selection set (the name after
# an alias). Prints "?" for anything that hides a field: a fragment spread or
# inline fragment among the mutation fields, or unbalanced braces or parentheses.
graphql_mutation_fields() {
  local -a T=()
  local t op="" depth=0 paren=0 k
  mapfile -t T < <(graphql_tokens "$1")
  for (( k = 0; k < ${#T[@]}; k++ )); do
    t="${T[k]}"
    [[ "$t" == '?' ]] && { echo '?'; return 0; }
    case "$t" in
      '(') paren=$((paren + 1)); continue ;;
      ')') paren=$((paren - 1)); continue ;;
    esac
    (( paren > 0 )) && continue
    if (( depth == 0 )); then
      case "$t" in
        mutation|query|subscription|fragment) op="$t"; [[ "$t" == mutation ]] && echo '@mutation' ;;
        '{') depth=1; [[ -n "$op" ]] || op=query ;;
      esac
      continue
    fi
    case "$t" in
      '{') depth=$((depth + 1)); continue ;;
      '}') depth=$((depth - 1)); (( depth == 0 )) && op=""; continue ;;
    esac
    if (( depth != 1 )) || [[ "$op" != mutation ]]; then continue; fi
    if [[ "$t" == '...' ]]; then
      echo '?'
    elif [[ "$t" =~ ^[_A-Za-z] && "${T[k-1]:-}" != '@' && "${T[k+1]:-}" != ':' ]]; then
      echo "$t"
    fi
  done
  (( depth == 0 && paren == 0 )) || echo '?'
}

# graphql_verdict <query field value...>: prints nothing when every mutation field in
# the documents is a review-thread mutation, else GATE:<what>.
graphql_verdict() {
  local doc f seen=0 fields="" bad=""
  for doc in "$@"; do
    while IFS= read -r f; do
      case "$f" in
        '@mutation') seen=1 ;;
        '?') bad="$bad an unreadable selection" ;;
        *) fields="$fields $f"
           [[ "$REVIEW_THREAD_MUTATIONS" == *" $f "* ]] || bad="$bad $f" ;;
      esac
    done < <(graphql_mutation_fields "$doc")
  done
  if (( seen == 0 )) || [[ -z "$fields" ]]; then
    echo "GATE:gh api graphql mutation the guard cannot read"
  elif [[ -n "$bad" ]]; then
    echo "GATE:gh api graphql mutation${bad}"
  fi
  return 0
}

# api_verdict <args after `api`>
api_verdict() {
  local w skip="" method="" implied_post=0 input=0 mutation=0 query_file=0 ep=""
  local -a queries=()
  for w in "$@"; do
    [[ "$w" =~ (^|[^A-Za-z])mutation([^A-Za-z]|$) ]] && mutation=1
    if [[ -n "$skip" ]]; then
      [[ "$skip" == method ]] && method="$w"
      if [[ "$skip" == raw || "$skip" == typed ]] && [[ "$w" == query=* ]]; then
        queries+=("${w#query=}")
        [[ "$skip" == typed && "$w" == query=@* ]] && query_file=1
      fi
      skip=""; continue
    fi
    # gh also accepts a shorthand flag joined by "=": -F=key=value, -X=POST.
    [[ "$w" == -[XfF]=* ]] && w="${w:0:2}${w:3}"
    case "$w" in
      -X|--method) skip=method ;;
      --method=*) method="${w#--method=}" ;;
      -X?*) method="${w#-X}" ;;
      -f|--raw-field) implied_post=1; skip=raw ;;
      -F|--field) implied_post=1; skip=typed ;;
      -fquery=*|--raw-field=query=*) implied_post=1; queries+=("${w#*query=}") ;;
      -Fquery=*|--field=query=*) implied_post=1; queries+=("${w#*query=}"); [[ "$w" == *query=@* ]] && query_file=1 ;;
      -f?*|-F?*|--field=*|--raw-field=*) implied_post=1 ;;
      --input) implied_post=1; input=1; skip=value ;;
      --input=*) implied_post=1; input=1 ;;
      -H|--header|-q|--jq|-t|--template|--cache|-p|--preview|--hostname) skip=value ;;
      -*) ;;
      *) [[ -z "$ep" ]] && ep="$w" ;;
    esac
  done
  method="$(printf '%s' "${method:-}" | tr '[:lower:]' '[:upper:]')"
  if [[ -z "$method" ]]; then
    if (( implied_post == 1 )); then method=POST; else method=GET; fi
  fi
  ep="${ep#http*://*/}"
  ep="${ep#api/v3/}"
  ep="${ep#/}"
  ep="${ep%%\?*}"

  if [[ "$ep" == graphql ]]; then
    if (( input == 1 )); then
      echo "GATE:gh api graphql --input (the document is not visible)"
    elif (( query_file == 1 )); then
      echo "GATE:gh api graphql with the query read from a file"
    elif (( mutation == 1 )); then
      graphql_verdict ${queries[@]+"${queries[@]}"}
    fi
    return 0
  fi
  [[ "$method" == GET || "$method" == HEAD ]] && return 0
  # Deleting the repository itself is never allowed, in any spelling, and neither is
  # a DELETE whose endpoint the guard cannot read, since it may name the repository.
  if [[ "$method" == DELETE && ( "$ep" =~ ^repos/[^/]+(/[^/]+)?/?$ || "$ep" == '$'* || -z "$ep" ) ]]; then
    echo "NEVER:gh api DELETE ${ep:-<no endpoint>} (repository deletion)"; return 0
  fi
  if [[ "$ep" == *'$'* || "$ep" == *'{'* && "$ep" != repos/\{owner\}/\{repo\}* ]]; then
    echo "GATE:gh api $method $ep (endpoint not literal)"; return 0
  fi
  case "$ep" in
    repos/*|orgs/*|enterprises/*) ;;
    *) return 0 ;;
  esac
  case "$method" in
    DELETE|PATCH|PUT) echo "GATE:gh api $method $ep"; return 0 ;;
  esac
  if [[ "$ep" == orgs/* || "$ep" == enterprises/* || "$ep" =~ $POST_ENDPOINT_RE ]]; then
    echo "GATE:gh api $method $ep"
  fi
  return 0
}

is_v_tag_ref() {   # a v* tag name, spelled bare, as tags/..., or as refs/tags/...
  local r="${1#refs/}"
  r="${r#tags/}"
  [[ "$1" == refs/tags/* || "$1" == tags/* ]] && [[ "$r" == v* || "$r" == *[\*\?\[]* ]] && return 0
  [[ "$1" =~ ^v[0-9] ]]
}

# push_verdict <args after `push`>
push_verdict() {
  local w skip=0 remote_seen=0 repo_opt=0 ref src dst
  for w in "$@"; do
    if (( skip == 1 )); then skip=0; continue; fi
    case "$w" in
      --tags|--follow-tags|--mirror) echo "GATE:git push $w"; return 0 ;;
      --repo) repo_opt=1; skip=1; continue ;;
      --repo=*) repo_opt=1; continue ;;
      -o|--push-option|--receive-pack|--exec) skip=1; continue ;;
      -*) continue ;;
    esac
    if (( remote_seen == 0 && repo_opt == 0 )); then remote_seen=1; continue; fi
    ref="${w#+}"
    if [[ "$ref" == *'$'* || "$ref" == *'`'* ]]; then
      echo "GATE:git push of a refspec the guard cannot read ($w)"; return 0
    fi
    src="${ref%%:*}"
    dst="${ref##*:}"
    if is_v_tag_ref "$src" || is_v_tag_ref "$dst"; then
      echo "GATE:git push of the release tag $w"; return 0
    fi
  done
  return 0
}

block() {   # block <detail> <fix>
  hh_audit "$DATA_DIR" "$KIND" block "" false
  hh_block "$1" "$2"
}

mapfile -t SEGMENTS < <(hh_command_segments "$HH_COMMAND")
for seg in ${SEGMENTS[@]+"${SEGMENTS[@]}"}; do
  hh_split_words "$seg"
  hh_cmd_index || continue

  if hh_arm_parse; then
    case "$HH_ARM_ACTION:$HH_ARM_KIND" in
      arm:publish) hh_sentinel_arm "$DATA_DIR" "$KIND" "$HH_ARM_REASON" "$HH_ARM_OPERATOR" ;;
      disarm:*) hh_sentinel_disarm "$DATA_DIR" "$KIND" ;;
    esac
    continue
  fi

  verdict=""
  cmd_word="${HH_WORDS[HH_CI]}"
  case "${cmd_word##*/}" in
    gh) verdict="$(gh_verdict "${HH_WORDS[@]:HH_CI+1}")" ;;
    git)
      if hh_git_parse "" && [[ "$HH_GIT_SUB" == push ]]; then
        verdict="$(push_verdict ${HH_GIT_ARGS[@]+"${HH_GIT_ARGS[@]}"})"
      fi ;;
  esac
  verdict="${verdict%%$'\n'*}"
  [[ -n "$verdict" ]] || continue

  what="${verdict#*:}"
  if [[ "$verdict" == NEVER:* ]]; then
    block "$what is never allowed from an agent session, armed or not: it cannot be undone." \
      "If it is truly intended, the operator runs it from their own terminal."
  fi
  if hh_sentinel_status "$DATA_DIR" "$KIND"; then
    hh_audit "$DATA_DIR" "$KIND" allow "$what" false
    continue
  fi
  case "$HH_ARM_STATUS" in
    expired) state="this session's publish window has expired (it lasts 30 minutes)" ;;
    unattributable) state="the payload carries no session id, so no publish window can apply" ;;
    *) state="this session has no armed publish window" ;;
  esac
  block "$what publishes or changes the public repository (releases, tags, workflow runs, secrets or settings), and $state." \
    "Leave publishing to the operator. If the operator asked for this action, run scripts/harness/arm-main-push.sh --publish --reason \"<why>\" in this session first (30-minute window), then retry. To push a branch whose name starts with v and a digit, spell it refs/heads/<name>."
done

exit 0
