#!/usr/bin/env python3
"""Run one exhaustive, disjoint Windows package shard from go list ./...."""
import argparse
import re
import subprocess
import sys

SUPERVISOR = "github.com/turbokast/mythhelm/internal/supervisor"
CLI = "github.com/turbokast/mythhelm/internal/cli"
PACKAGE = re.compile(r"[A-Za-z0-9_~.+-]+(?:/[A-Za-z0-9_~.+-]+)*\Z")


def select_packages(raw, shard):
    if shard not in ("supervisor", "cli", "rest"):
        raise ValueError("unknown shard")
    packages = raw.splitlines()
    if not packages or any(p.startswith("-") or not PACKAGE.fullmatch(p) for p in packages):
        raise ValueError("empty or malformed package list")
    if len(packages) != len(set(packages)) or any(packages.count(p) != 1 for p in (SUPERVISOR, CLI)):
        raise ValueError("duplicate packages or missing supervisor/CLI")
    others = [p for p in packages if p not in (SUPERVISOR, CLI)]
    if not others:
        raise ValueError("empty rest shard")
    return {"supervisor": [SUPERVISOR], "cli": [CLI], "rest": others}[shard]


def main(argv=None):
    parser = argparse.ArgumentParser()
    parser.add_argument("shard", choices=("supervisor", "cli", "rest"))
    parser.add_argument("--race", action="store_true")
    args = parser.parse_args(argv)
    try:
        result = subprocess.run(["go", "list", "./..."], check=True, text=True, capture_output=True)
        packages = select_packages(result.stdout, args.shard)
        return subprocess.run(["go", "test", *(["-race"] if args.race else []), "-p", "1", *packages],
                              check=False).returncode
    except subprocess.CalledProcessError as exc:
        print(f"go test shard: go list failed: {exc}", file=sys.stderr)
        if exc.stderr:
            print(exc.stderr, file=sys.stderr, end="" if exc.stderr.endswith("\n") else "\n")
        return 1
    except (OSError, ValueError) as exc:
        print(f"go test shard: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
