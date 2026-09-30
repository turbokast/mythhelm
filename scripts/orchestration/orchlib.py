"""orchlib.py: helpers shared by the orchestration scripts (autonomy.py, lanes.py,
delivery.py). Standard library only; imported, never run.

Every piece of orchestration state lives in the MAIN checkout of the repository, found
through the git common directory, so the main checkout and each of its linked worktrees
share one copy:

    <main checkout>/.claude/data/   grant, audit log, lanes (gitignored)
    <main checkout>/orchestration/  the delivery run's INTENT, RUN-LOG and QUESTIONS
                                    (gitignored except README.md)

Timestamps always come from the clock at the moment of writing (utc_now), never from
an argument: a stamp an agent estimates drifts, and nothing downstream can tell.
"""

import contextlib
import datetime
import fcntl
import json
import os
import subprocess
import tempfile


class Refused(Exception):
    """The command cannot proceed; the message says why and what to do instead."""


def utc_now():
    return datetime.datetime.now(datetime.timezone.utc).replace(microsecond=0)


def iso(dt):
    return dt.strftime("%Y-%m-%dT%H:%M:%SZ")


def parse_iso(text):
    try:
        return datetime.datetime.strptime(text or "", "%Y-%m-%dT%H:%M:%SZ").replace(
            tzinfo=datetime.timezone.utc)
    except ValueError:
        return None


def git_out(cwd, *args, timeout=30):
    env = {k: v for k, v in os.environ.items() if k not in ("GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE")}
    try:
        r = subprocess.run(["git", "-C", cwd] + list(args), capture_output=True, timeout=timeout, env=env)
    except (OSError, subprocess.SubprocessError):
        return None
    return r.stdout.decode("utf-8", "replace").strip() if r.returncode == 0 else None


def main_checkout(start=None):
    """The main checkout of the repository holding start (default: the cwd)."""
    start = start or os.getcwd()
    common = git_out(start, "rev-parse", "--path-format=absolute", "--git-common-dir")
    if not common:
        raise Refused("%s is not inside a git repository" % start)
    return os.path.dirname(os.path.realpath(common))


def toplevel(start=None):
    top = git_out(start or os.getcwd(), "rev-parse", "--show-toplevel")
    if not top:
        raise Refused("%s is not inside a git repository" % (start or os.getcwd()))
    return top


def data_dir(start=None):
    d = os.path.join(main_checkout(start), ".claude", "data")
    os.makedirs(d, exist_ok=True)
    return d


def orch_dir(start=None):
    d = os.path.join(main_checkout(start), "orchestration")
    os.makedirs(d, exist_ok=True)
    return d


@contextlib.contextmanager
def locked(path):
    """An exclusive advisory lock on <path>.lock for the duration of the block."""
    with open(path + ".lock", "a", encoding="utf-8") as fh:
        fcntl.flock(fh, fcntl.LOCK_EX)
        try:
            yield
        finally:
            fcntl.flock(fh, fcntl.LOCK_UN)


def atomic_write(path, text):
    """Replace path with text through a temporary file in the same directory, so a
    concurrent reader sees the old content or the new, never half of either."""
    d = os.path.dirname(path) or "."
    fd, tmp = tempfile.mkstemp(dir=d, prefix=".tmp-", suffix=os.path.basename(path))
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as fh:
            fh.write(text)
        os.replace(tmp, path)
    except BaseException:
        with contextlib.suppress(OSError):
            os.unlink(tmp)
        raise


def read_json(path):
    try:
        with open(path, encoding="utf-8") as fh:
            return json.load(fh)
    except (OSError, ValueError):
        return None


def append_jsonl(path, row):
    """Append one JSON row. Best effort: an audit failure never changes a verdict."""
    try:
        with open(path, "a", encoding="utf-8") as fh:
            fh.write(json.dumps(row, sort_keys=True) + "\n")
    except OSError:
        pass
