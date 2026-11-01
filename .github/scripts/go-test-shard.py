#!/usr/bin/env python3
"""Run one exhaustive, disjoint Windows package shard from go list ./...."""
import argparse
import json
import math
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


# Bound both individual JSON records and retained timing observations. Oversized
# records remain visible verbatim in chunks, without decoding/retaining them.
EVENT_CHARS = 64 * 1024
SLOWEST_LIMIT = 20
NAME_CHARS = 512
ACTIONS = {"start", "run", "pause", "cont", "bench", "output", "pass", "fail", "skip",
           "build-output", "build-fail"}


class TimingSummary:
    def __init__(self):
        self.slowest = []

    def add(self, event):
        package, test, elapsed = event.get("Package"), event.get("Test"), event.get("Elapsed")
        if (not isinstance(event.get("Action"), str)
                or event["Action"] not in {"pass", "fail", "skip"}
                or not isinstance(package, str) or not package or len(package) > NAME_CHARS
                or not isinstance(test, str) or not test or len(test) > NAME_CHARS
                or isinstance(elapsed, bool) or not isinstance(elapsed, (int, float))):
            return False
        try:
            elapsed = float(elapsed)
        except (ValueError, OverflowError):
            return False
        if not math.isfinite(elapsed) or elapsed < 0:
            return False
        self.slowest.append((elapsed, package, test, event["Action"]))
        self.slowest.sort(key=lambda row: (-row[0], row[1], row[2], row[3]))
        del self.slowest[SLOWEST_LIMIT:]
        return True

    def emit(self):
        if self.slowest:
            print("Slowest completed Go test observations (seconds; nested tests overlap):", flush=True)
            for elapsed, package, test, action in self.slowest:
                # Quote names so telemetry cannot inject terminal controls.
                print(f"  {elapsed:.3f}s {json.dumps(package)} {json.dumps(test)} [{action}]", flush=True)


def stream_output(pipe, summary):
    oversized = False
    while chunk := pipe.readline(EVENT_CHARS):
        if oversized or (len(chunk) == EVENT_CHARS and not chunk.endswith("\n")):
            sys.stdout.write(chunk)
            sys.stdout.flush()
            oversized = not chunk.endswith("\n")
            continue
        try:
            event = json.loads(chunk)
        except (ValueError, RecursionError):
            event = None
        if (not isinstance(event, dict) or not isinstance(event.get("Action"), str)
                or event["Action"] not in ACTIONS):
            sys.stdout.write(chunk)
        elif "Output" in event:
            output = event["Output"]
            sys.stdout.write(output if isinstance(output, str) else chunk)
        elif event.get("Action") in {"pass", "fail", "skip"} and "Test" in event:
            if not summary.add(event):
                sys.stdout.write(chunk)
        sys.stdout.flush()


def diagnostic(message):
    try:
        print(f"go test shard: {message}", file=sys.stderr)
    except (OSError, ValueError):
        # Diagnostic sinks must not replace the already observed child status.
        pass


def run_tests(command):
    # One owned child, one merged stream: never accumulate the full run. The
    # summary is diagnostic only; the child's nonzero status remains decisive.
    child = subprocess.Popen(command, stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                             text=True, encoding="utf-8", errors="replace")
    summary = TimingSummary()
    try:
        try:
            stream_output(child.stdout, summary)
        except (OSError, ValueError) as exc:
            if child.poll() is None:
                child.kill()
            status = child.wait()
            diagnostic(f"output stream failed: {exc}")
            return status if status else 1
        except BaseException:
            if child.poll() is None:
                child.kill()
            child.wait()
            raise
        status = child.wait()
        try:
            summary.emit()
        except (OSError, ValueError) as exc:
            diagnostic(f"timing summary unavailable: {exc}")
        return status
    finally:
        try:
            child.stdout.close()
        except (OSError, ValueError) as exc:
            diagnostic(f"closing output stream failed: {exc}")
        finally:
            if child.poll() is None:
                child.kill()
            child.wait()


def main(argv=None):
    parser = argparse.ArgumentParser()
    parser.add_argument("shard", choices=("supervisor", "cli", "rest"))
    parser.add_argument("--race", action="store_true")
    args = parser.parse_args(argv)
    try:
        result = subprocess.run(["go", "list", "./..."], check=True, text=True, capture_output=True)
        packages = select_packages(result.stdout, args.shard)
        return run_tests(["go", "test", "-json", *(["-race"] if args.race else []), "-p", "1", *packages])
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
