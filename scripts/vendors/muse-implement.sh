#!/usr/bin/env bash
# muse-implement.sh: the Muse implementer lane. Muse implements one task in a fresh
# linked worktree inside a bubblewrap sandbox; the result comes back as a patch that
# the calling agent reviews, gates and commits itself.
#
#   scripts/vendors/muse-implement.sh --spec-dir <dir> --task <N> [--from <dir>]
#     [--include-uncommitted] [--tier trivial|standard|complex] [--prompt-file <file>] [--keep]
#
# A thin door into lane.py. Needs the vendor AND its lane opted in, Linux with a
# root-owned bwrap, and the vendor signed in. Prints one JSON response line; exit 0
# on every run path, 64 on a usage error.

set -euo pipefail
exec python3 "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/lane.py" --vendor muse "$@"
