#!/usr/bin/env python3
"""lanes.py: advisory locks ("lanes") that let concurrent sessions share single-copy
resources of one repository: the main checkout's git index, a spec directory, the
product files, the delivery run itself.

    lanes.py token                                     print a fresh owner token
    lanes.py acquire <resource> --owner TOKEN [--pid PID] [--note TEXT]
    lanes.py release <resource> --owner TOKEN
    lanes.py status  <resource>                        exit 0 held, 1 free
    lanes.py list    [--json]
    lanes.py reap                                      remove locks whose holder is gone

A lane is keyed by (repository, resource): the repository is the main checkout, found
through the git common directory, so every linked worktree sees the same lanes. Each
lane is one file under <main checkout>/.claude/data/lanes/, written under a lock.

OWNERSHIP IS AN EXPLICIT TOKEN, never a session id. A subagent's tool calls carry its
parent's session id, so a lane keyed by session let a worker that acquired and
released "its" lane delete the orchestrator's. Here only the holder of the token can
release or refresh a lane; the orchestrator mints a token with `token` and never hands
it to a worker.

STALENESS IS A DEAD HOLDER, never age. A lane records the holder process (by default
the nearest ancestor process named `claude`, else the invoking shell) and that
process's start time. The lane is stale when no process has that pid, or the process
with that pid started at a different time (the pid was reused). A long-running holder
is never reaped for being slow, and a crashed one is reclaimable at once.

Resources are free-form names: `index`, `spec:<name>`, `product`, `run:delivery`.

Exit codes: 0 acquired, released, held or done; 1 refused (held by another owner, not
the owner, free); 2 usage error or not inside a git repository.
"""

import argparse
import hashlib
import json
import os
import re
import secrets
import subprocess
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import orchlib  # noqa: E402
from orchlib import Refused  # noqa: E402

RESOURCE_RE = re.compile(r"^[a-z0-9][a-z0-9:._/-]{0,127}$")
OWNER_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._-]{7,127}$")
HOLDER_COMM = os.environ.get("LANE_HOLDER_COMM", "claude")


def ps(pid, field):
    try:
        r = subprocess.run(["ps", "-o", "%s=" % field, "-p", str(pid)], capture_output=True, text=True,
                           env=dict(os.environ, LC_ALL="C"), timeout=10)
    except (OSError, subprocess.SubprocessError):
        return None
    return r.stdout.strip() if r.returncode == 0 and r.stdout.strip() else None


def start_time(pid):
    return ps(pid, "lstart")


def default_holder():
    pid = os.getppid()
    seen = 0
    while pid and pid > 1 and seen < 32:
        comm = os.path.basename(ps(pid, "comm") or "")
        if comm == HOLDER_COMM:
            return pid
        parent = ps(pid, "ppid")
        if not parent or not parent.isdigit():
            break
        pid, seen = int(parent), seen + 1
    return os.getppid()


def alive(lane):
    try:
        pid = int(lane.get("pid", 0))
    except (TypeError, ValueError):
        return False
    if pid <= 0:
        return False
    started = start_time(pid)
    return started is not None and started == lane.get("pid_start")


def lanes_dir(root):
    d = os.path.join(orchlib.data_dir(root), "lanes")
    os.makedirs(d, exist_ok=True)
    return d


def lane_path(root, resource):
    safe = re.sub(r"[^a-z0-9.-]", "_", resource)[:60]
    return os.path.join(lanes_dir(root), "%s-%s.json" % (safe, hashlib.sha256(resource.encode()).hexdigest()[:10]))


def check_resource(resource):
    if not RESOURCE_RE.match(resource or ""):
        raise Refused("resource %r is not a lane name ([a-z0-9][a-z0-9:._/-]*)" % resource)


def check_owner(owner):
    if not OWNER_RE.match(owner or ""):
        raise Refused("--owner takes a token from `lane.sh token` (8+ characters of [A-Za-z0-9._-])")


def audit(root, event, lane):
    orchlib.append_jsonl(os.path.join(lanes_dir(root), "audit.jsonl"),
                         {"ts": orchlib.iso(orchlib.utc_now()), "event": event, "resource": lane.get("resource"),
                          "owner": lane.get("owner"), "pid": lane.get("pid")})


def cmd_acquire(a, root):
    check_resource(a.resource)
    check_owner(a.owner)
    pid = a.pid or default_holder()
    started = start_time(pid)
    if started is None:
        raise Refused("holder pid %s is not a running process" % pid)
    path = lane_path(root, a.resource)
    with orchlib.locked(path):
        cur = orchlib.read_json(path)
        if cur and cur.get("owner") != a.owner:
            if alive(cur):
                print("LANE-HELD %s by owner %s (pid %s, since %s%s)" % (
                    a.resource, cur.get("owner"), cur.get("pid"), cur.get("acquired_at"),
                    ", " + cur["note"] if cur.get("note") else ""), file=sys.stderr)
                return 1
            print("LANE-STALE %s: holder pid %s is gone; taking it over" % (a.resource, cur.get("pid")))
            audit(root, "reclaim", cur)
        lane = {"schema_version": 1, "resource": a.resource, "owner": a.owner, "pid": pid, "pid_start": started,
                "acquired_at": (cur or {}).get("acquired_at") if cur and cur.get("owner") == a.owner
                else orchlib.iso(orchlib.utc_now()), "note": a.note or "", "cwd": os.getcwd()}
        orchlib.atomic_write(path, json.dumps(lane, sort_keys=True) + "\n")
        audit(root, "acquire", lane)
    print("LANE-ACQUIRED %s (pid %s)" % (a.resource, pid))
    return 0


def cmd_release(a, root):
    check_resource(a.resource)
    check_owner(a.owner)
    path = lane_path(root, a.resource)
    with orchlib.locked(path):
        cur = orchlib.read_json(path)
        if not cur:
            print("LANE-FREE %s (nothing to release)" % a.resource)
            return 0
        if cur.get("owner") != a.owner:
            print("LANE-NOT-OWNER %s is held by owner %s; only its owner releases it" % (a.resource, cur.get("owner")),
                  file=sys.stderr)
            return 1
        os.unlink(path)
        audit(root, "release", cur)
    print("LANE-RELEASED %s" % a.resource)
    return 0


def all_lanes(root):
    out = []
    d = lanes_dir(root)
    for n in sorted(os.listdir(d)):
        if n.endswith(".json"):
            lane = orchlib.read_json(os.path.join(d, n))
            if lane:
                lane["state"] = "live" if alive(lane) else "stale"
                out.append(lane)
    return out


def cmd_status(a, root):
    check_resource(a.resource)
    cur = orchlib.read_json(lane_path(root, a.resource))
    if cur and alive(cur):
        print("LANE-HELD %s by owner %s (pid %s)" % (a.resource, cur.get("owner"), cur.get("pid")))
        return 0
    print("LANE-FREE %s%s" % (a.resource, " (stale lock of a gone holder)" if cur else ""))
    return 1


def cmd_list(a, root):
    lanes = all_lanes(root)
    if a.json:
        print(json.dumps(lanes, sort_keys=True))
        return 0
    if not lanes:
        print("no lanes")
    for ln in lanes:
        print("%-5s %-28s owner=%s pid=%s since=%s%s" % (ln["state"], ln.get("resource"), ln.get("owner"),
                                                         ln.get("pid"), ln.get("acquired_at"),
                                                         " note=" + ln["note"] if ln.get("note") else ""))
    return 0


def cmd_reap(a, root):
    for ln in all_lanes(root):
        if ln["state"] != "stale":
            continue
        path = lane_path(root, ln["resource"])
        with orchlib.locked(path):
            cur = orchlib.read_json(path)
            if cur and not alive(cur):
                os.unlink(path)
                audit(root, "reap", cur)
                print("LANE-REAPED %s (holder pid %s is gone)" % (ln["resource"], cur.get("pid")))
    return 0


def main(argv):
    p = argparse.ArgumentParser(prog="lane.sh", description="Advisory locks over shared resources.")
    sub = p.add_subparsers(dest="cmd", required=True)
    sub.add_parser("token")
    ac = sub.add_parser("acquire")
    ac.add_argument("resource")
    ac.add_argument("--owner", required=True)
    ac.add_argument("--pid", type=int)
    ac.add_argument("--note")
    rl = sub.add_parser("release")
    rl.add_argument("resource")
    rl.add_argument("--owner", required=True)
    st = sub.add_parser("status")
    st.add_argument("resource")
    ls = sub.add_parser("list")
    ls.add_argument("--json", action="store_true")
    sub.add_parser("reap")
    a = p.parse_args(argv)
    if a.cmd == "token":
        print("lane-" + secrets.token_hex(8))
        return 0
    try:
        root = os.getcwd()
        orchlib.main_checkout(root)
        return {"acquire": cmd_acquire, "release": cmd_release, "status": cmd_status, "list": cmd_list,
                "reap": cmd_reap}[a.cmd](a, root)
    except Refused as e:
        print("lane.sh %s: refused: %s" % (a.cmd, e), file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
