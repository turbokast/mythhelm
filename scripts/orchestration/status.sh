#!/usr/bin/env bash
# status.sh: a one-screen summary of the delivery run, for a session resuming it and
# for the maintainer checking on it. Read-only: it writes nothing and changes nothing.
#
#   scripts/orchestration/status.sh [--no-fetch]
#
# Sections: the autonomy grant, the delivery run (items, stages, what is actionable,
# open questions), lanes, specs in flight on origin/main, open pull requests with their
# unresolved review-thread counts, and main's latest CI runs. GitHub facts come from
# gh; when gh is missing or offline those sections say so and the rest still prints.
# Judgement (what to do next) stays with the reader.
#
# Exit codes: 0 always, unless the directory is not inside a git repository (2).

set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
top="$(git rev-parse --show-toplevel 2>/dev/null)" || { echo "status.sh: not inside a git repository" >&2; exit 2; }
fetch=1
[[ "${1:-}" == --no-fetch ]] && fetch=0

section() { printf '\n== %s ==\n' "$1"; }

section "AUTONOMY"
python3 "$HERE/autonomy.py" status 2>&1 || true

section "DELIVERY RUN"
python3 "$HERE/delivery.py" show 2>&1 || true

section "LANES"
python3 "$HERE/lanes.py" list 2>&1 || true

section "SPECS IN FLIGHT (origin/main)"
if (( fetch )); then
  timeout 20 git -C "$top" fetch -q origin main 2>/dev/null || echo "(fetch failed; showing the last fetched origin/main)"
fi
for state in unrefined refined todo in-progress unfinalized; do
  names="$(git -C "$top" ls-tree -d --name-only "origin/main:specs/$state" 2>/dev/null | tr '\n' ' ' || true)"
  printf '%-12s %s\n' "$state" "${names:-—}"
done

section "OPEN PULL REQUESTS"
if ! command -v gh >/dev/null 2>&1; then
  echo "(gh is not installed)"
else
  slug="$(gh repo view --json nameWithOwner --jq .nameWithOwner 2>/dev/null || true)"
  # shellcheck disable=SC2016  # GraphQL variables, not shell expansions
  query='query($owner:String!,$name:String!){repository(owner:$owner,name:$name){pullRequests(states:OPEN,first:30,orderBy:{field:UPDATED_AT,direction:DESC}){nodes{number title headRefName isDraft reviewDecision reviewThreads(first:100){nodes{isResolved}}}}}}'
  if [[ -n "$slug" ]] && out="$(gh api graphql -f query="$query" -F owner="${slug%%/*}" -F name="${slug#*/}" 2>/dev/null)"; then
    printf '%s' "$out" | jq -r '.data.repository.pullRequests.nodes[]
      | "#\(.number) \(.title[:70]) [\(.headRefName)]\(if .isDraft then " draft" else "" end) unresolved=\([.reviewThreads.nodes[] | select(.isResolved | not)] | length)"' \
      | sed -n '1,15p'
    [[ "$(printf '%s' "$out" | jq '.data.repository.pullRequests.nodes | length')" != 0 ]] || echo "none"
  else
    echo "(GitHub unavailable)"
  fi
fi

section "MAIN CI"
if command -v gh >/dev/null 2>&1 && out="$(gh run list --branch main --limit 4 --json workflowName,status,conclusion,headSha,createdAt 2>/dev/null)"; then
  printf '%s' "$out" | jq -r '.[] | "\(.createdAt) \(.headSha[:12]) \(.workflowName): \(if .status == "completed" then .conclusion else .status end)"'
  [[ "$(printf '%s' "$out" | jq length)" != 0 ]] || echo "no runs"
else
  echo "(GitHub unavailable)"
fi

section "NEXT"
echo "Resume with /deliver-backlog; the grant and the run above are the ground truth it starts from."
