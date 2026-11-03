#!/usr/bin/env python3
"""Run every portable harness test with bounded optional concurrency.

Invoked by .claude/hooks/tests/run-tests.sh [filter] [--jobs 1..64]. The default
is serial. Every selected test runs, results retain discovery order, and failures
exit 1 (usage errors 2). Output is buffered in temporary files removed on exit.
SIGINT/SIGTERM stop and reap active workers, including their process groups on
POSIX, with a two-second grace period before SIGKILL. Tests must isolate their
own mutable fixtures; concurrency does not sandbox arbitrary test code.
"""

import argparse
import os
from pathlib import Path
import re
import selectors
import signal
import subprocess
import sys
import tempfile
import time


TALLY = re.compile(r"^checks=([0-9]+) failed=.*$", re.MULTILINE)


def jobs_value(value):
    try:
        jobs = int(value)
    except ValueError as exc:
        raise argparse.ArgumentTypeError("jobs must be an integer from 1 to 64") from exc
    if not 1 <= jobs <= 64:
        raise argparse.ArgumentTypeError("jobs must be an integer from 1 to 64")
    return jobs


def discover(root, name_filter):
    paths = []
    hooks = root / ".claude/hooks/tests"
    if hooks.is_dir():
        paths.extend(p for p in hooks.glob("test_*.sh") if p.is_file() and not p.is_symlink())
    scripts = root / "scripts"
    for directory, _, names in os.walk(scripts):
        # Match find -path '*/tests/test_*.sh', including nested test directories.
        if Path(directory).name == "tests":
            paths.extend(Path(directory) / n for n in names
                         if n.startswith("test_") and n.endswith(".sh")
                         and (Path(directory) / n).is_file()
                         and not (Path(directory) / n).is_symlink())
    return sorted((p for p in paths if name_filter in p.relative_to(root).as_posix()),
                  key=lambda p: os.fsencode(p))


def stop_workers(active, signum):
    def send(proc, sig):
        try:
            if os.name == "posix":
                os.killpg(proc.pid, sig)
            elif proc.poll() is None:
                proc.terminate() if sig != signal.SIGKILL else proc.kill()
        except ProcessLookupError:
            pass

    for proc, _ in active.values():
        send(proc, signum)
    deadline = time.monotonic() + 2
    # Keep the grace period even if leaders exit: descendants share their groups.
    while time.monotonic() < deadline:
        for proc, _ in active.values():
            proc.poll()
        time.sleep(.02)
    for proc, _ in active.values():
        send(proc, signal.SIGKILL)
    for proc, output in active.values():
        proc.wait()
        proc.stdout.close()
        output.close()


def run_suite(root, tests, jobs):
    interrupted = 0

    def on_signal(signum, _frame):
        nonlocal interrupted
        interrupted = interrupted or signum

    previous = {sig: signal.signal(sig, on_signal) for sig in (signal.SIGINT, signal.SIGTERM)}
    active, results = {}, {}
    next_test, next_report = 0, 0
    passed, failed, checks = 0, 0, 0
    failed_names = []
    try:
        with tempfile.TemporaryDirectory(prefix="mythhelm-harness-") as tmp, selectors.DefaultSelector() as ready:
            while next_report < len(tests):
                if interrupted:
                    stop_workers(active, interrupted)
                    active.clear()
                    print("Harness tests interrupted; active workers stopped.", file=sys.stderr, flush=True)
                    return 128 + interrupted
                while next_test < len(tests) and len(active) < jobs and not interrupted:
                    index = next_test
                    next_test += 1
                    output = open(Path(tmp) / str(index), "w+b")
                    try:
                        proc = subprocess.Popen(["bash", str(tests[index])], stdout=subprocess.PIPE,
                                                stderr=subprocess.STDOUT,
                                                start_new_session=(os.name == "posix"))
                    except OSError as exc:
                        output.write((str(exc) + "\n").encode())
                        output.close()
                        results[index] = 1
                    else:
                        # Own the child before fallible pipe/selector setup.
                        active[index] = (proc, output)
                        os.set_blocking(proc.stdout.fileno(), False)
                        ready.register(proc.stdout, selectors.EVENT_READ, index)
                # A shell can exit before a descendant closes its inherited output.
                # Preserve command-substitution semantics: completion includes EOF.
                for key, _ in ready.select(timeout=.02):
                    proc, output = active[key.data]
                    try:
                        data = os.read(key.fd, 65536)
                    except BlockingIOError:
                        continue
                    if data:
                        output.write(data)
                    else:
                        ready.unregister(proc.stdout)
                        proc.stdout.close()
                for index, (proc, output) in list(active.items()):
                    code = proc.poll()
                    if code is not None and proc.stdout.closed:
                        output.close()
                        results[index] = code
                        del active[index]
                while next_report in results:
                    index = next_report
                    next_report += 1
                    code = results.pop(index)
                    name = tests[index].relative_to(root).as_posix()
                    out = (Path(tmp) / str(index)).read_text(errors="replace").rstrip("\n")
                    tallies = TALLY.findall(out)
                    count = tallies[-1] if tallies else "?"
                    checks += int(count) if tallies else 0
                    if code == 0:
                        passed += 1
                        print("PASS  %s (%s checks)" % (name, count), flush=True)
                    else:
                        failed += 1
                        failed_names.append(name)
                        print("FAIL  %s" % name, flush=True)
                        for line in out.split("\n"):
                            print("      " + line, flush=True)
            print("\nTest summary: %d passed, %d failed, %d checks" % (passed, failed, checks), flush=True)
            if not tests:
                print("No tests matched.", file=sys.stderr)
                return 1
            for name in failed_names:
                print("  failed: " + name, file=sys.stderr)
            return 1 if failed else 0
    finally:
        if active:
            stop_workers(active, signal.SIGTERM)
        for sig, handler in previous.items():
            signal.signal(sig, handler)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("filter", nargs="?", default="", help="substring of a repository-relative test path")
    parser.add_argument("--jobs", type=jobs_value, default=1, help="concurrent test files, from 1 to 64 (default: 1)")
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[2]
    return run_suite(root, discover(root, args.filter), args.jobs)


if __name__ == "__main__":
    sys.exit(main())
