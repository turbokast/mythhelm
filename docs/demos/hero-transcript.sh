#!/usr/bin/env bash
# hero-transcript.sh: the linear companion run of docs/demos/hero.tape, whose
# stdout record.sh normalizes into docs/demos/hero.transcript.txt. The taped
# take drives the TUI, whose bytes can never match a re-run, so this script
# runs the same commands in linear mode: `mythhelm run` in the background, a
# SIGKILL of that process once the paced scenario reports its third turn,
# then `mythhelm runs list`, `mythhelm recover` and `mythhelm review`.
#
# Needs HERO_BIN (the binary to record) and HERO_TMP (a scratch directory the
# caller removes). Prints the transcript on stdout; exits non-zero on any
# step that does not behave as the recording claims.

set -euo pipefail

: "${HERO_BIN:?hero-transcript.sh: HERO_BIN names the binary to record}"
: "${HERO_TMP:?hero-transcript.sh: HERO_TMP names the scratch directory}"
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=docs/demos/hero-env.sh
source "$here/hero-env.sh"

log="$HERO_TMP/run.log"

echo "=== take: run (killed with SIGKILL after the third turn) ==="
mythhelm run --adapter fake --scenario paced --billing local-scripted \
	--execution-profile trusted-host --task-file task.md \
	--trust-project-config "$TRUST" --plain --non-interactive </dev/null >"$log" 2>&1 &
pid=$!
for _ in $(seq 1 600); do
	if grep -q '^progress: 3 ' "$log"; then
		break
	fi
	sleep 0.1
done
grep -q '^progress: 3 ' "$log"
# Everything up to the third turn is the take's output; anything the process
# printed after the kill decision is a race and never part of the transcript.
sed '/^progress: 3 /q' "$log"
status=0
# The shell announces the SIGKILL on stderr with a pid, at the kill or at the
# wait depending on when it reaps the child; the exit status is the evidence.
{
	kill -KILL "$pid"
	wait "$pid" || status=$?
} 2>/dev/null
test "$status" -eq 137
echo "exit status $status"

echo "=== take: the worker is still alive ==="
worker="$(pgrep -f "__worker --state $MYTHHELM_HOME")"
echo "$worker"

echo "=== take: runs list ==="
list="$(mythhelm runs list)"
echo "$list"
# The journal cannot know the process died: the run still reads executing.
test "$(echo "$list" | awk 'NR==2{print $2}')" = executing
run="$(echo "$list" | awk 'NR==2{print $1}')"

echo "=== take: recover ==="
recovered="$(mythhelm recover "$run" 2>&1)"
echo "$recovered"
# Recovery reattaches to the worker that survived the kill, not a new one.
echo "$recovered" | grep -q "^reattached to worker pid $worker\$"

echo "=== take: review ==="
mythhelm review "$run"
