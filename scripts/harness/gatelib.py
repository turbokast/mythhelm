#!/usr/bin/env python3
"""gatelib.py: quality-gate markers. A marker records one verified pass of one gate
over one exact tree state, so a later check can tell "this gate passed on what is
on disk now" from "this gate passed once, on something else".

    gatelib.py run <gate|group>... [--root DIR]      run gates; record each verified pass
    gatelib.py fingerprint --scope go|harness|all [--root DIR]
    gatelib.py required [--root DIR] [--base REF]      gates the tree's change set needs
    gatelib.py status [--root DIR] [--base REF] [--json]
    gatelib.py stop-hook                               Stop/SubagentStop payload on stdin
    gatelib.py override --session SID --reason TEXT [--root DIR]

scripts/harness/gate.sh is the entry point for `run`. Gates and groups:

    go-fmt go-vet go-test go-mod-tidy golangci-lint govulncheck    scope go
    harness-lint harness-tests shellcheck                          scope harness
    hygiene                                                        scope all
    go | harness | all                                             the gates the change set needs

Only a verifiably successful run records. `run` computes the scope's fingerprint,
runs the gate's fixed command unpiped from the tree root, and writes
<tree>/.claude/data/gate-marker-<gate>.json only when the command exited 0 AND the
fingerprint is unchanged afterwards: a tree that changed while the gate ran was
not the tree it checked. A failing run deletes the gate's marker. A marker is
fresh when its fingerprint equals the scope's current fingerprint in the same tree.

A fingerprint is SHA-256 over every tracked and untracked, non-ignored file in the
scope (path, type, executable bit, content; a deleted tracked file counts as
deleted). Ignored build output never enters it, so a churning cache cannot make a
marker stale, and every consumer computes it with this one function.

The change set is `git diff --no-renames --name-only <base>` plus untracked files,
with <base> = merge-base(HEAD, origin/main), or HEAD when there is no origin/main.

stop-hook implements .claude/hooks/verify-task-completion.sh; see that file for the
contract. override writes the session's escape hatch and its audit row.

Exit codes: run 0 all passed, 1 a gate failed or was not recorded, 2 usage, 127 a
gate's tool is missing; status 0 all required gates fresh, 1 otherwise; stop-hook
0 allow, 2 block.
"""

import argparse
import datetime
import hashlib
import json
import os
import re
import shutil
import stat
import subprocess
import sys
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import runspec  # noqa: E402  (sibling module)

GOLANGCI = "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2"
GOVULNCHECK = "golang.org/x/vuln/cmd/govulncheck@v1.8.0"
HARNESS_PREFIXES = (".claude/", "scripts/", "knowledge/", "docs/harness/")
HARNESS_FILES = ("CLAUDE.md", "AGENTS.md", "WORKFLOW.md")
GO_FILES = ("go.mod", "go.sum", ".golangci.yml")
MAX_BLOCKS = 3


class Gate:
    def __init__(self, scope, cmd, empty_output=False):
        self.scope, self.cmd, self.empty_output = scope, cmd, empty_output


GATES = {
    "go-fmt": Gate("go", ["gofmt", "-l", "."], empty_output=True),
    "go-vet": Gate("go", ["go", "vet", "./..."]),
    "go-test": Gate("go", ["go", "test", "-race", "./..."]),
    "go-mod-tidy": Gate("go", ["go", "mod", "tidy", "-diff"]),
    "golangci-lint": Gate("go", ["go", "run", GOLANGCI, "run"]),
    "govulncheck": Gate("go", ["go", "run", GOVULNCHECK, "./..."]),
    "harness-lint": Gate("harness", ["scripts/ci/lint-agent-harness.sh"]),
    "harness-tests": Gate("harness", [".claude/hooks/tests/run-tests.sh"]),
    "shellcheck": Gate("harness", ["shellcheck"]),
    "hygiene": Gate("all", ["scripts/ci/check-public-hygiene.sh"]),
}
GROUPS = ("go", "harness", "all")


def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def git(root, *args, check=True):
    r = subprocess.run(["git", "-C", root] + list(args), capture_output=True, env=runspec.git_env(), timeout=120)
    if check and r.returncode != 0:
        raise runspec.Usage("git %s failed: %s" % (" ".join(args[:3]), r.stderr.decode("utf-8", "replace").strip()[:300]))
    return r


def toplevel(start):
    return os.path.realpath(git(start, "rev-parse", "--show-toplevel").stdout.decode().strip())


def in_scope(path, scope):
    if scope == "all":
        return True
    if scope == "go":
        return path.endswith(".go") or path in GO_FILES
    if scope == "harness":
        return path.startswith(HARNESS_PREFIXES) or path in HARNESS_FILES
    raise runspec.Usage("unknown scope %r" % scope)


def listed_files(root):
    out = git(root, "ls-files", "-z", "--cached", "--others", "--exclude-standard").stdout
    return sorted({p.decode("utf-8", "surrogateescape") for p in out.split(b"\0") if p})


def fingerprint(root, scope):
    h = hashlib.sha256(("scope:%s\n" % scope).encode())
    for rel in listed_files(root):
        if not in_scope(rel, scope):
            continue
        path = os.path.join(root, rel)
        try:
            st = os.lstat(path)
        except FileNotFoundError:
            entry = "D"
        else:
            if stat.S_ISLNK(st.st_mode):
                entry = "L" + os.readlink(path)
            elif stat.S_ISREG(st.st_mode):
                fh = hashlib.sha256()
                with open(path, "rb") as f:
                    for chunk in iter(lambda: f.read(1 << 16), b""):
                        fh.update(chunk)
                entry = "F%d%s" % (1 if st.st_mode & 0o111 else 0, fh.hexdigest())
            else:
                entry = "O"
        h.update(rel.encode("utf-8", "surrogateescape") + b"\0" + entry.encode() + b"\n")
    return h.hexdigest()


def default_base(root):
    if git(root, "rev-parse", "--verify", "-q", "origin/main", check=False).returncode == 0:
        r = git(root, "merge-base", "HEAD", "origin/main", check=False)
        if r.returncode == 0:
            return r.stdout.decode().strip()
    if git(root, "rev-parse", "--verify", "-q", "HEAD", check=False).returncode == 0:
        return "HEAD"
    return None


def changed_files(root, base=None):
    base = base or default_base(root)
    files = set()
    if base:
        out = git(root, "diff", "--no-renames", "--name-only", "-z", base).stdout
        files |= {p.decode("utf-8", "surrogateescape") for p in out.split(b"\0") if p}
    else:
        out = git(root, "ls-files", "-z", "--cached").stdout
        files |= {p.decode("utf-8", "surrogateescape") for p in out.split(b"\0") if p}
    out = git(root, "ls-files", "-z", "--others", "--exclude-standard").stdout
    files |= {p.decode("utf-8", "surrogateescape") for p in out.split(b"\0") if p}
    return sorted(files)


def required_gates(root, changed):
    """The gates a change set needs, in run order."""
    out = []
    if any(in_scope(p, "go") for p in changed):
        out += ["go-fmt", "go-vet", "go-test", "go-mod-tidy"]
        if os.path.exists(os.path.join(root, ".golangci.yml")):
            out.append("golangci-lint")
        if any(p in ("go.mod", "go.sum") for p in changed):
            out.append("govulncheck")
    if any(in_scope(p, "harness") for p in changed):
        out += ["harness-lint", "harness-tests"]
        if any(p.endswith(".sh") for p in changed):
            out.append("shellcheck")
    if changed:
        out.append("hygiene")
    return out


def marker_path(root, gate):
    return os.path.join(root, ".claude", "data", "gate-marker-%s.json" % gate)


def read_marker(root, gate):
    try:
        with open(marker_path(root, gate), encoding="utf-8") as fh:
            m = json.load(fh)
        return m if isinstance(m, dict) else None
    except (OSError, ValueError):
        return None


def marker_state(root, gate, fps):
    """'fresh', 'stale' or 'missing'. fps caches fingerprints per scope."""
    m = read_marker(root, gate)
    if not m:
        return "missing"
    scope = GATES[gate].scope
    if scope not in fps:
        fps[scope] = fingerprint(root, scope)
    ok = (m.get("schema_version") == 1 and m.get("gate") == gate and m.get("exit_code") == 0
          and m.get("tree") == root and m.get("fingerprint") == fps[scope])
    return "fresh" if ok else "stale"


def write_json_atomic(path, obj):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    tmp = "%s.%d.tmp" % (path, os.getpid())
    with open(tmp, "w", encoding="utf-8") as fh:
        json.dump(obj, fh, indent=1)
        fh.write("\n")
    os.replace(tmp, path)


def gate_command(root, gate):
    g = GATES[gate]
    if gate == "shellcheck":
        files = [p for p in listed_files(root) if p.endswith(".sh") and os.path.isfile(os.path.join(root, p))]
        return g.cmd + files if files else None
    return list(g.cmd)


def run_gate(root, gate):
    """Runs one gate; returns its exit status (0 recorded pass)."""
    g = GATES[gate]
    cmd = gate_command(root, gate)
    exe = cmd[0] if cmd else "shellcheck"
    if cmd and not (shutil.which(exe) or os.path.isfile(os.path.join(root, exe))):
        print("gate %s: tool %r not found on PATH; nothing recorded" % (gate, exe), flush=True)
        return 127
    before = fingerprint(root, g.scope)
    started, t0 = now(), time.monotonic()
    print("gate %s: %s" % (gate, " ".join(cmd) if cmd else "(no shell scripts)"), flush=True)
    if cmd is None:
        rc = 0
    elif g.empty_output:
        r = subprocess.run(cmd, cwd=root, capture_output=True, text=True)
        sys.stdout.write(r.stdout)
        sys.stderr.write(r.stderr)
        rc = r.returncode or (1 if r.stdout.strip() else 0)
    else:
        rc = subprocess.run(cmd, cwd=root).returncode
    elapsed = time.monotonic() - t0
    if rc != 0:
        try:
            os.remove(marker_path(root, gate))
        except FileNotFoundError:
            pass
        print("gate %s: FAIL (exit %d, %.1fs); marker cleared" % (gate, rc, elapsed), flush=True)
        return rc
    after = fingerprint(root, g.scope)
    if after != before:
        print("gate %s: passed, but the %s scope changed while it ran; NOT recorded. Run it again."
              % (gate, g.scope), flush=True)
        return 1
    head = git(root, "rev-parse", "HEAD", check=False).stdout.decode().strip()
    marker = {
        "schema_version": 1, "gate": gate, "scope": g.scope, "tree": root, "fingerprint": after,
        "head": head, "command": cmd or ["shellcheck"], "exit_code": 0, "started_at": started,
        "finished_at": now(), "duration_s": round(elapsed, 1),
    }
    write_json_atomic(marker_path(root, gate), marker)
    print("gate %s: PASS (exit 0, %.1fs); recorded %s fingerprint %s"
          % (gate, elapsed, os.path.relpath(marker_path(root, gate), root), after[:12]), flush=True)
    return 0


def expand(root, names, base=None):
    req = None
    out = []
    for n in names:
        if n in GATES:
            out.append(n)
            continue
        if n not in GROUPS:
            raise runspec.Usage("unknown gate or group %r (gates: %s; groups: %s)"
                                % (n, " ".join(GATES), " ".join(GROUPS)))
        if req is None:
            req = required_gates(root, changed_files(root, base))
        out += [g for g in req if n == "all" or GATES[g].scope == n]
    seen = set()
    return [g for g in out if not (g in seen or seen.add(g))]


# ---- the Stop hook -------------------------------------------------------------

def audit(data_dir, row):
    os.makedirs(data_dir, exist_ok=True)
    row = dict({"schema_version": 1, "ts": now()}, **row)
    with open(os.path.join(data_dir, "stop-gate-audit.jsonl"), "a", encoding="utf-8") as fh:
        fh.write(json.dumps(row, separators=(",", ":"), ensure_ascii=False) + "\n")


def spec_tasks_files(changed):
    return [p for p in changed if re.fullmatch(r"specs/[^/]+/[^/]+/tasks\.md", p)]


def base_tasks(root, base, rel):
    """The spec's tasks.md at <base> (the same spec name in any lifecycle state), or ''."""
    if not base:
        return ""
    name = rel.split("/")[2]
    out = git(root, "ls-tree", "-r", "--name-only", base, "--", "specs/", check=False).stdout.decode()
    hits = [p for p in out.split("\n") if re.fullmatch(r"specs/[^/]+/%s/tasks\.md" % re.escape(name), p)]
    if len(hits) != 1:
        return ""
    return git(root, "show", "%s:%s" % (base, hits[0]), check=False).stdout.decode("utf-8", "replace")


def claimed_tasks(root, base, changed):
    """[(tasks.md path, task)] completed in the working tree but not at <base>."""
    claims = []
    for rel in spec_tasks_files(changed):
        try:
            with open(os.path.join(root, rel), encoding="utf-8") as fh:
                now_tasks = runspec.by_number(runspec.parse_tasks(fh.read()))
        except OSError:
            continue
        before = runspec.by_number(runspec.parse_tasks(base_tasks(root, base, rel)))
        for n, t in sorted(now_tasks.items()):
            if t["complete"] and not (n in before and before[n]["complete"]):
                claims.append((rel, n, t))
    return claims


def transcript_touches(path, root, rels):
    """The subset of rels this transcript edits (Edit/Write/MultiEdit/NotebookEdit on the
    file, or a Bash command naming it); None when the transcript cannot be read."""
    if not path:
        return None
    wanted = {rel: os.path.join(root, rel) for rel in rels}
    hit = set()
    try:
        with open(path, encoding="utf-8", errors="replace") as fh:
            for line in fh:
                if "tool_use" not in line:
                    continue
                try:
                    row = json.loads(line)
                except ValueError:
                    continue
                content = ((row.get("message") or {}).get("content")) if isinstance(row, dict) else None
                for item in content if isinstance(content, list) else []:
                    if not isinstance(item, dict) or item.get("type") != "tool_use":
                        continue
                    inp = item.get("input") or {}
                    target = inp.get("file_path") or inp.get("notebook_path") or ""
                    command = inp.get("command") or ""
                    for rel, absolute in wanted.items():
                        tail = "/".join(rel.split("/")[2:])      # <name>/tasks.md
                        if target and (os.path.realpath(target) == os.path.realpath(absolute) or target.endswith("/" + rel)):
                            hit.add(rel)
                        elif command and tail in command:
                            hit.add(rel)
    except OSError:
        return None
    return hit


def emit_system_message(text):
    print(json.dumps({"systemMessage": text}))


def stop_hook(stdin_text, env=os.environ):
    try:
        payload = json.loads(stdin_text or "{}")
    except ValueError:
        emit_system_message("verify-task-completion: the Stop payload is not JSON; the completion gate did not run.")
        return 0
    sid = payload.get("session_id") or "unknown"
    cwd = payload.get("cwd") or env.get("CLAUDE_PROJECT_DIR") or os.getcwd()
    event = payload.get("hook_event_name") or "Stop"
    try:
        root = toplevel(cwd)
    except runspec.Usage:
        return 0
    data_dir = os.path.join(os.path.dirname(os.path.realpath(
        git(root, "rev-parse", "--path-format=absolute", "--git-common-dir").stdout.decode().strip())), ".claude", "data")
    base = default_base(root)
    changed = changed_files(root, base)
    claims = claimed_tasks(root, base, changed)
    if not claims:
        return 0
    if event == "SubagentStop":
        transcript = payload.get("agent_transcript_path")
    else:
        transcript = payload.get("transcript_path")
    touched = transcript_touches(transcript, root, sorted({c[0] for c in claims}))
    if touched is not None:
        claims = [c for c in claims if c[0] in touched]
        if not claims:
            return 0
    problems, commands = [], []
    for rel, n, t in claims:
        for p in runspec.field_problems(t):
            problems.append("%s Task %d: %s" % (rel, n, p))
    fps = {}
    for gate in required_gates(root, changed):
        state = marker_state(root, gate, fps)
        if state != "fresh":
            problems.append("gate %s: marker %s" % (gate, state))
            commands.append("scripts/harness/gate.sh %s" % gate)
    if not problems:
        return 0
    all_fp = fps.get("all") or fingerprint(root, "all")
    key = hashlib.sha256(root.encode()).hexdigest()[:16]
    ov_path = os.path.join(data_dir, "stop-gate-override-%s.json" % sid)
    try:
        with open(ov_path, encoding="utf-8") as fh:
            ov = json.load(fh)
    except (OSError, ValueError):
        ov = None
    claim_names = ["%s#%d" % (rel.split("/")[2], n) for rel, n, _ in claims]
    if ov and ov.get("tree") == root and ov.get("fingerprint") == all_fp:
        audit(data_dir, {"session_id": sid, "tree": root, "event": "override_honored",
                         "claims": claim_names, "problems": problems, "reason": ov.get("reason", "")})
        return 0
    state_path = os.path.join(data_dir, "stop-gate-%s.json" % sid)
    try:
        with open(state_path, encoding="utf-8") as fh:
            state = json.load(fh)
    except (OSError, ValueError):
        state = {}
    entry = state.get(key) or {}
    blocks = entry.get("blocks", 0) if entry.get("fingerprint") == all_fp else 0
    if payload.get("stop_hook_active") and blocks >= MAX_BLOCKS:
        audit(data_dir, {"session_id": sid, "tree": root, "event": "released_after_repeats",
                         "claims": claim_names, "problems": problems, "reason": "%d consecutive blocks" % blocks})
        emit_system_message("verify-task-completion released a stop after %d blocks with unmet completion gates "
                            "(%s); recorded in .claude/data/stop-gate-audit.jsonl." % (blocks, "; ".join(problems)[:300]))
        return 0
    state[key] = {"tree": root, "fingerprint": all_fp, "blocks": blocks + 1, "at": now()}
    write_json_atomic(state_path, state)
    audit(data_dir, {"session_id": sid, "tree": root, "event": "block", "claims": claim_names, "problems": problems})
    fix = []
    if commands:
        fix.append("run " + ", then ".join(commands) + " (each as its own foreground command; the wrapper records the pass)")
    if any("Task" in p for p in problems):
        fix.append("complete the entry as .claude/skills/task-completion/SKILL.md specifies")
    fix.append("if this block is wrong, record why and stop: python3 scripts/harness/gatelib.py override "
               "--session %s --reason \"<why>\" (audited in .claude/data/stop-gate-audit.jsonl)" % sid)
    sys.stderr.write("BLOCK: verify-task-completion\nFile: %s\nDetail: this session marked %s complete, but %s.\nFix: %s.\n"
                     % (", ".join(sorted({c[0] for c in claims})), ", ".join(claim_names), "; ".join(problems),
                        "; ".join(fix)))
    return 2


def override(root, sid, reason):
    if len(reason.strip()) < 10:
        raise runspec.Usage("--reason must say why the block is wrong (at least 10 characters)")
    data_dir = os.path.join(os.path.dirname(os.path.realpath(
        git(root, "rev-parse", "--path-format=absolute", "--git-common-dir").stdout.decode().strip())), ".claude", "data")
    row = {"session_id": sid, "tree": root, "fingerprint": fingerprint(root, "all"), "reason": reason.strip(), "at": now()}
    write_json_atomic(os.path.join(data_dir, "stop-gate-override-%s.json" % sid), row)
    audit(data_dir, {"session_id": sid, "tree": root, "event": "override_recorded", "reason": reason.strip()})
    print("override recorded for session %s on %s; it lapses as soon as any file in the tree changes" % (sid, root))


# ---- CLI -----------------------------------------------------------------------

def main(argv=None):
    ap = argparse.ArgumentParser(prog="gatelib.py", description=__doc__.split("\n")[0])
    sub = ap.add_subparsers(dest="cmd", required=True)
    p = sub.add_parser("run"); p.add_argument("gates", nargs="+"); p.add_argument("--root", default=".")
    p.add_argument("--base")
    p = sub.add_parser("fingerprint"); p.add_argument("--scope", required=True, choices=("go", "harness", "all"))
    p.add_argument("--root", default=".")
    p = sub.add_parser("required"); p.add_argument("--root", default="."); p.add_argument("--base")
    p = sub.add_parser("status"); p.add_argument("--root", default="."); p.add_argument("--base")
    p.add_argument("--json", action="store_true")
    sub.add_parser("stop-hook")
    p = sub.add_parser("override"); p.add_argument("--session", required=True); p.add_argument("--reason", required=True)
    p.add_argument("--root", default=".")
    a = ap.parse_args(sys.argv[1:] if argv is None else argv)
    try:
        if a.cmd == "stop-hook":
            return stop_hook(sys.stdin.read())
        root = toplevel(a.root)
        if a.cmd == "run":
            gates = expand(root, a.gates, a.base)
            if not gates:
                print("gate: the change set needs none of %s" % " ".join(a.gates))
                return 0
            for g in gates:
                rc = run_gate(root, g)
                if rc != 0:
                    return 127 if rc == 127 else 1
            print("gate: %d passed and recorded: %s" % (len(gates), " ".join(gates)))
        elif a.cmd == "fingerprint":
            print(fingerprint(root, a.scope))
        elif a.cmd == "required":
            print(" ".join(required_gates(root, changed_files(root, a.base))))
        elif a.cmd == "status":
            fps, rows = {}, []
            for g in required_gates(root, changed_files(root, a.base)):
                rows.append({"gate": g, "state": marker_state(root, g, fps)})
            if a.json:
                print(json.dumps(rows))
            else:
                for r in rows:
                    print("%-14s %s" % (r["gate"], r["state"]))
            return 0 if all(r["state"] == "fresh" for r in rows) else 1
        elif a.cmd == "override":
            override(root, a.session, a.reason)
    except runspec.Usage as e:
        print("gatelib.py: %s" % e, file=sys.stderr)
        return 2
    return 0


if __name__ == "__main__":
    sys.exit(main())
