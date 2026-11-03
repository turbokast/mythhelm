#!/usr/bin/env python3
"""Verify each recording's transcript and qualifiers at its immutable revision."""

import argparse
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile


class RecordingError(ValueError):
    """Recording provenance could not be independently verified."""


def run(argv, cwd, *, env=None, data=None, merged=False):
    return subprocess.run(argv, cwd=cwd, env=env, input=data,
                          stdout=subprocess.PIPE,
                          stderr=subprocess.STDOUT if merged else subprocess.PIPE,
                          timeout=600, check=False)


def transcript(root, binary, name):
    """Existing fixed pipelines, with every producer/normalizer exit checked."""
    def take(*args, merged=False, **extra_env):
        env = dict(os.environ, **extra_env)
        result = run([str(binary), *args], root, env=env, merged=merged)
        if result.returncode:
            raise RecordingError(f"{name}: transcript command failed (exit {result.returncode})")
        normalized = run(["sed", "-E", "-f", str(root / "docs/demos/normalize.sed")],
                         root, data=result.stdout)
        if normalized.returncode:
            raise RecordingError(f"{name}: transcript normalization failed")
        return normalized.stdout

    if name == "demo":
        return take("demo", "--check", "pass")
    if name == "tui":
        return (b"=== take: linear (TERM=dumb) ===\n" +
                take("demo", "--check", "pass", merged=True, TERM="dumb") +
                b"=== take: accessible (--accessible) ===\n" +
                take("demo", "--check", "pass", "--accessible", merged=True))
    # Preserve the existing hero transcript/normalization contract. This is
    # the isolated CI runner's single recording operation, not a recorder or
    # a new concurrency guarantee for the companion's fixed /tmp/demo state.
    result = run([str(root / "docs/demos/hero-transcript.sh")], root,
                 env=dict(os.environ, HERO_BIN=str(binary), HERO_TMP="/tmp/demo"), merged=True)
    if result.returncode:
        raise RecordingError(f"{name}: transcript command failed (exit {result.returncode})")
    normalized = run(["sed", "-E", "-f", str(root / "docs/demos/normalize.sed")],
                     root, data=result.stdout)
    if normalized.returncode:
        raise RecordingError(f"{name}: transcript normalization failed")
    return normalized.stdout


def check_recordings(root):
    records = json.loads((root / "docs/demos/manifest.json").read_text(encoding="utf-8"))["recordings"]
    if not records or len(records) > 3:
        raise RecordingError("expected one to three named recordings")
    names, validated = set(), set()
    # Validate every authority-free data field before building or executing
    # pinned code. Manifest strings never become shell commands.
    for rec in records:
        name = rec["name"]
        if name not in ("demo", "tui", "hero") or name in names:
            raise RecordingError("unknown or duplicate recording name")
        names.add(name)
        if rec["transcript"] != f"docs/demos/{name}.transcript.txt":
            raise RecordingError(f"{name}: transcript must name its fixed recording path")
        declared_revision = rec["binary_commit"]
        if not isinstance(declared_revision, str) or not re.fullmatch(r"[0-9a-f]{40}", declared_revision):
            raise RecordingError(f"{name}: declared_revision must be an immutable 40-character commit")
        if declared_revision not in validated:
            result = run(["git", "cat-file", "-t", declared_revision], root)
            if result.returncode or result.stdout.strip() != b"commit":
                raise RecordingError(f"{name}: declared_revision {declared_revision} is not a present commit")
            validated.add(declared_revision)
        for capability in rec["capabilities"]:
            qualifier = capability["qualifier"]
            if not isinstance(qualifier, str) or not re.fullmatch(r"[A-Za-z0-9_]+", qualifier):
                raise RecordingError(f"{name}: qualifier must be a literal Go test name")

    with tempfile.TemporaryDirectory(prefix="mythhelm-recording-check-") as temporary:
        worktrees, binaries, passed = {}, {}, set()
        try:
            for rec in records:
                name = rec["name"]
                revision = rec["binary_commit"]
                if revision not in worktrees:
                    worktree = Path(temporary) / f"revision-{len(worktrees)}"
                    result = run(["git", "worktree", "add", "--detach", str(worktree), revision], root)
                    if result.returncode:
                        raise RecordingError(f"{name}: could not create pinned worktree")
                    worktrees[revision] = worktree
                    binary = Path(temporary) / f"binary-{len(binaries)}"
                    result = run(["go", "build", "-o", str(binary), "./cmd/mythhelm"], worktree)
                    if result.returncode:
                        raise RecordingError(f"{name}: pinned binary build failed (exit {result.returncode})")
                    binaries[revision] = binary
                actual = transcript(root, binaries[revision], name)
                expected = (root / rec["transcript"]).read_bytes()
                if actual != expected:
                    raise RecordingError(f"{name}: transcript differs at {revision}; re-record this recording")
                print(f"{name}: transcript fresh at {revision}")
                for capability in rec["capabilities"]:
                    qualifier = capability["qualifier"]
                    qualifier_revision = revision
                    key = (qualifier_revision, qualifier)
                    if key not in passed:
                        result = run(["go", "test", "-v", "-run", f"^{qualifier}$", "-count=1", "./..."],
                                     worktrees[qualifier_revision], merged=True)
                        if result.returncode:
                            raise RecordingError(f"{name}: qualifier failed: {qualifier} at {qualifier_revision}")
                        witness = rb"(?m)^--- PASS: " + qualifier.encode("ascii") + rb"(?: \(|$)"
                        if re.search(witness, result.stdout) is None:
                            raise RecordingError(f"{name}: no exact PASS witness for {qualifier} at {qualifier_revision}")
                        passed.add(key)
                    print(f"{name}: qualifier {qualifier} passed at {qualifier_revision}")
        finally:
            failures = []
            for worktree in worktrees.values():
                result = run(["git", "worktree", "remove", "--force", str(worktree)], root)
                if result.returncode:
                    failures.append(worktree.name)
            if failures:
                raise RecordingError("could not remove owned worktrees: " + ", ".join(failures))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, default=Path(__file__).resolve().parents[2])
    args = parser.parse_args()
    try:
        check_recordings(args.root.resolve())
    except (ValueError, KeyError, TypeError, OSError, subprocess.TimeoutExpired) as exc:
        parser.exit(1, f"check_recordings: {exc}\n")


if __name__ == "__main__":
    main()
