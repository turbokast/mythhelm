# shellcheck shell=bash
# shellcheck disable=SC2154  # PROBE_* are set by stub.env, sourced by the stub
# probe.sh: sourced by the codex stub in lane-probe mode. Tries to reach what the
# sandbox must hide and prints one JSON event per attempt (the lane keeps the
# vendor's stdout). PROBE_* come from stub.env.
attempt() {
  local name="$1"; shift
  if "$@" >/dev/null 2>&1; then r=reached; else r=denied; fi
  printf '{"type":"probe","name":"%s","result":"%s"}\n' "$name" "$r"
}
attempt read_main_env cat "$PROBE_MAIN_ENV"
attempt read_home_secret cat "$PROBE_HOME_SECRET"
attempt write_main touch "$PROBE_MAIN_WRITE"
attempt write_git_config sh -c "echo x >> '$PROBE_GIT_CONFIG'"
