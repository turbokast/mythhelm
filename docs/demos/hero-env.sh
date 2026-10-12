# shellcheck shell=bash
# Sourced, never executed, by docs/demos/hero.tape (into the recording's
# shell) and by docs/demos/hero-transcript.sh. It builds the disposable
# example repository the hero demo runs on, points MYTHHELM_HOME at a
# disposable state directory and puts the recorded binary on PATH as
# `mythhelm`. Nothing outside HERO_TMP is touched, and no credential, network
# or real agent is involved: the task runs the fake adapter's paced scenario.
#
# HERO_BIN names the binary to record (record.sh builds it); HERO_TMP names
# the scratch directory (default /tmp/demo, a short fixed path so the paths on
# screen read /tmp/demo/...). A leftover scratch directory is replaced only
# when it carries this script's marker file; anything else there is refused.

HERO_BIN="${HERO_BIN:-/tmp/mythhelm-record}"
HERO_TMP="${HERO_TMP:-/tmp/demo}"
if [ -e "$HERO_TMP" ]; then
	if [ ! -f "$HERO_TMP/.mythhelm-hero" ]; then
		echo "hero-env.sh: $HERO_TMP exists and is not a hero scratch directory; refusing to replace it" >&2
		return 1
	fi
	rm -rf -- "$HERO_TMP"
fi
mkdir -p "$HERO_TMP/bin" "$HERO_TMP/repo" "$HERO_TMP/state"
touch "$HERO_TMP/.mythhelm-hero"
ln -sf "$HERO_BIN" "$HERO_TMP/bin/mythhelm"
export PATH="$HERO_TMP/bin:$PATH"
export MYTHHELM_HOME="$HERO_TMP/state"

cd "$HERO_TMP/repo" || return 1
git init --quiet --initial-branch=main
git config user.name "Mythhelm Demo"
git config user.email "demo@example.com"
printf '# Greeter\n' >README.md
printf '# Add a greeting\n\nAdd greeting.txt with a friendly hello.\n' >task.md
printf 'schema_version = 1\n[[checks]]\nname = "greeting-present"\nargv = ["grep", "-q", "hello", "greeting.txt"]\ntimeout = "30s"\n' >mythhelm.toml
git add README.md task.md mythhelm.toml
git commit --quiet -m "scripted demo"
TRUST="sha256:$(python3 -c 'import hashlib;print(hashlib.sha256(open("mythhelm.toml","rb").read()).hexdigest())')"
export TRUST
