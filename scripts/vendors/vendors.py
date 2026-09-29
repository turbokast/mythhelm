#!/usr/bin/env python3
"""vendors.py: the shared, vendor-neutral envelope for the optional external vendors.

Every sanctioned vendor wrapper (scripts/codex/*.sh, scripts/vendors/*.sh,
scripts/jev/jev.py) goes through this module for the same five things:

  policy        .claude/data/vendor-policy.json (tracked seed) merged with the
                gitignored .claude/data/vendor-policy.local.json (the contributor's
                opt-in). Nothing is enabled by default.
  availability  opted in, CLI on PATH, version at or above the pin, and signed in
                according to the vendor's OWN status command (an API key in the
                environment for an API vendor). Credential files are never read.
  quota         per-day call caps per vendor and per stage, counted locally under a
                lock. An exhausted cap makes the vendor unavailable for the day.
  redaction     tokens, private keys, secret-looking assignments, e-mail addresses
                and the home directory are masked in every prompt before it leaves.
  records       one JSONL row per call in .claude/data/vendor-calls.jsonl, in the
                shape of scripts/vendors/schemas/response.schema.json.

Fail-open contract: every problem becomes an advisory `unavailable` response with a
reason code; nothing here ever raises to a caller's gate.

State lives in the MAIN checkout's .claude/data (shared by linked worktrees),
resolved from this file's own location, never from an argument or the target.

CLI (exit 0; 64 on a usage error):
  vendors.py status [--json]        availability and today's quota for every vendor
  vendors.py enable <vendor> [--lanes]
                                    opt this checkout in; run it from your own
                                    terminal (the guard hook blocks it for agents)
  vendors.py disable <vendor|all> [--lanes-only]
  vendors.py redact                 stdin to stdout, masked
  vendors.py quota [--json]         today's counts against the caps
"""

from __future__ import annotations

import contextlib
import datetime
import fcntl
import hashlib
import json
import os
import re
import secrets
import shutil
import signal
import subprocess
import sys
import time

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
SCHEMA_DIR = os.path.join(SCRIPT_DIR, "schemas")
EXIT_USAGE = 64

VENDORS = ("codex", "muse", "jev")
REASONS = (
    "disabled", "policy-denied", "policy-unreadable", "cli-missing", "version-too-old",
    "not-signed-in", "no-api-key", "over-quota", "target-refused", "context-too-large",
    "timeout", "vendor-error", "unparseable-output", "schema-mismatch", "write-through",
    "sandbox-unavailable", "task-unreadable", "protected-path", "out-of-scope", "symlink",
    "no-changes", "patch-too-large", "network", "internal-error",
)

GIT_ENV_DROP = ("GIT_DIR", "GIT_COMMON_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE",
                "GIT_CEILING_DIRECTORIES", "GIT_OBJECT_DIRECTORY")


class Unavailable(Exception):
    """An advisory skip: the reason is one of REASONS, the detail is for humans."""

    def __init__(self, reason: str, detail: str = ""):
        super().__init__(f"{reason}: {detail}" if detail else reason)
        self.reason = reason
        self.detail = detail


class UsageError(Exception):
    pass


# ---- git and paths -----------------------------------------------------------

def git_env() -> dict:
    env = {k: v for k, v in os.environ.items() if k not in GIT_ENV_DROP}
    return env


def git(cwd: str, *args: str, check: bool = True, env: dict | None = None, text: bool = True):
    """`git -C cwd args` with the ambient repository-relocating variables removed,
    so neither an argument nor the environment can repoint where state lands."""
    run_env = git_env()
    if env:
        run_env.update(env)
    r = subprocess.run(["git", "-C", cwd, *args], capture_output=True, text=text, env=run_env, timeout=120)
    if check and r.returncode != 0:
        raise RuntimeError(f"git {' '.join(args)} failed in {cwd}: {r.stderr.strip() if text else r.stderr}")
    return r


def toplevel(path: str) -> str | None:
    r = git(path, "rev-parse", "--show-toplevel", check=False)
    return r.stdout.strip() if r.returncode == 0 and r.stdout.strip() else None


def main_checkout(path: str) -> str | None:
    """The main checkout of the repository containing `path`, via the shared common dir."""
    r = git(path, "rev-parse", "--path-format=absolute", "--git-common-dir", check=False)
    if r.returncode != 0 or not r.stdout.strip():
        return None
    return os.path.dirname(os.path.realpath(r.stdout.strip()))


def data_dir() -> str:
    main = main_checkout(SCRIPT_DIR)
    base = main if main else os.path.normpath(os.path.join(SCRIPT_DIR, "..", ".."))
    return os.path.join(base, ".claude", "data")


def is_clean_linked_worktree(path: str) -> tuple[bool, str]:
    """A linked worktree (its git dir differs from the common dir) with nothing
    uncommitted, untracked OR gitignored. The plain porcelain form hides ignored
    files, which is exactly where local secrets live, so --ignored=matching is used."""
    if not os.path.isdir(path):
        return False, "target is not a directory"
    gd = git(path, "rev-parse", "--path-format=absolute", "--git-dir", check=False)
    cd = git(path, "rev-parse", "--path-format=absolute", "--git-common-dir", check=False)
    if gd.returncode != 0 or cd.returncode != 0:
        return False, "target is not inside a git repository"
    if os.path.realpath(gd.stdout.strip()) == os.path.realpath(cd.stdout.strip()):
        return False, "target is the main checkout, not a linked worktree"
    st = git(path, "status", "--porcelain", "--ignored=matching", check=False)
    if st.returncode != 0:
        return False, "git status failed in the target"
    if st.stdout.strip():
        return False, "target has uncommitted, untracked or ignored files; build a snapshot with scripts/vendors/snapshot.sh"
    return True, ""


# ---- policy --------------------------------------------------------------------

def _merge(base, over):
    if isinstance(base, dict) and isinstance(over, dict):
        out = dict(base)
        for k, v in over.items():
            out[k] = _merge(base.get(k), v) if k in base else v
        return out
    return over


def policy_paths() -> tuple[str, str]:
    top = toplevel(SCRIPT_DIR) or os.path.normpath(os.path.join(SCRIPT_DIR, "..", ".."))
    return (os.path.join(top, ".claude", "data", "vendor-policy.json"),
            os.path.join(data_dir(), "vendor-policy.local.json"))


def _read_policy_file(path: str) -> dict:
    try:
        with open(path, encoding="utf-8") as fh:
            data = json.load(fh)
    except (OSError, ValueError) as err:
        raise Unavailable("policy-unreadable", f"{path}: {err}") from err
    if not isinstance(data, dict):
        raise Unavailable("policy-unreadable", f"{path}: not a JSON object")
    return data


def load_policy() -> dict:
    """The merged policy. Raises Unavailable(policy-unreadable) on a broken file.
    `enabled` and `lanes_enabled` come from the local file only."""
    tracked, local = policy_paths()
    pol = _read_policy_file(tracked)
    # Opt-in belongs to the contributor: only the gitignored local file can enable
    # anything, so a committed edit to the seed never opts anyone in.
    pol["enabled"], pol["lanes_enabled"] = [], []
    if os.path.exists(local):
        pol = _merge(pol, _read_policy_file(local))
    if not isinstance(pol.get("vendors"), dict) or not isinstance(pol.get("stages"), dict):
        raise Unavailable("policy-unreadable", "policy lacks vendors or stages")
    return pol


def vendor_conf(pol: dict, vendor: str) -> dict:
    conf = pol["vendors"].get(vendor)
    if not isinstance(conf, dict):
        raise Unavailable("policy-denied", f"vendor {vendor} is not in the policy")
    return conf


def stage_conf(pol: dict, vendor: str, stage: str) -> dict:
    conf = pol["stages"].get(stage)
    if not isinstance(conf, dict):
        raise UsageError(f"unknown stage {stage!r}; known: {', '.join(sorted(pol['stages']))}")
    if vendor not in conf.get("vendors", []):
        raise Unavailable("policy-denied", f"stage {stage} does not consult {vendor}")
    return conf


def clamp_timeout(pol: dict, seconds) -> int:
    """The stage's timeout, capped by the policy's hard ceiling. VENDOR_TIMEOUT_S can
    only shorten it (tests); a zero, negative or non-numeric value is ignored."""
    hard = int(pol.get("hard_timeout_s", 3600))
    t = min(int(seconds or hard), hard)
    raw = os.environ.get("VENDOR_TIMEOUT_S", "")
    if raw.isdigit() and 0 < int(raw) < t:
        t = int(raw)
    return max(t, 1)


# ---- process running -------------------------------------------------------------

def run_bounded(argv, cwd=None, stdin_path=None, timeout=900, env=None):
    """Runs argv in its own process group; on timeout the whole group is killed.
    Returns (rc, stdout, stderr, timed_out). stdout/stderr are bytes."""
    stdin = open(stdin_path, "rb") if stdin_path else subprocess.DEVNULL
    try:
        proc = subprocess.Popen(argv, cwd=cwd, stdin=stdin, stdout=subprocess.PIPE,
                                stderr=subprocess.PIPE, env=env, start_new_session=True)
        try:
            out, err = proc.communicate(timeout=timeout)
            return proc.returncode, out, err, False
        except subprocess.TimeoutExpired:
            with contextlib.suppress(ProcessLookupError, PermissionError):
                os.killpg(proc.pid, signal.SIGKILL)
            try:
                out, err = proc.communicate(timeout=10)
            except subprocess.TimeoutExpired:
                # A descendant that left the group still holds a pipe open.
                for pipe in (proc.stdout, proc.stderr):
                    with contextlib.suppress(OSError):
                        pipe.close()
                with contextlib.suppress(subprocess.TimeoutExpired):
                    proc.wait(timeout=10)
                out, err = b"", b""
            return proc.returncode, out, err, True
    finally:
        if stdin_path:
            stdin.close()


def parse_version(raw: str) -> tuple[int, int, int] | None:
    m = re.search(r"(\d+)\.(\d+)\.(\d+)", raw or "")
    return tuple(int(x) for x in m.groups()) if m else None


def version_ge(actual: str, minimum: str) -> bool:
    a, m = parse_version(actual), parse_version(minimum)
    return a is not None and m is not None and a >= m


# ---- availability -------------------------------------------------------------------

def is_enabled(pol: dict, vendor: str) -> bool:
    return vendor in (pol.get("enabled") or [])


def lanes_enabled(pol: dict, vendor: str) -> bool:
    return is_enabled(pol, vendor) and vendor in (pol.get("lanes_enabled") or [])


def check_available(pol: dict, vendor: str) -> dict:
    """Raises Unavailable, else returns {cli, version} (cli vendors) or {} (API vendors).
    A CLI vendor is signed in when its own status command exits 0; the envelope never
    opens the vendor's credential store."""
    conf = vendor_conf(pol, vendor)
    if not is_enabled(pol, vendor):
        raise Unavailable("disabled", f"{vendor} is not enabled for this checkout (knowledge/vendors.md)")
    if conf.get("kind") == "api":
        if not os.environ.get(conf.get("api_key_env", ""), "").strip():
            raise Unavailable("no-api-key", f"{conf.get('api_key_env')} is not set")
        return {}
    cli = shutil.which(conf.get("cli", vendor))
    if not cli:
        raise Unavailable("cli-missing", f"{conf.get('cli', vendor)} is not on PATH")
    env = dict(os.environ, NO_COLOR="1", MUSE_NO_AUTO_UPDATE="1")
    rc, out, err, timed_out = run_bounded([cli, "--version"], timeout=30, env=env)
    raw = (out + err).decode("utf-8", "replace")
    if timed_out or not version_ge(raw, conf.get("min_version", "0.0.0")):
        raise Unavailable("version-too-old", f"{vendor} reports {raw.strip()[:80]!r}, need >= {conf.get('min_version')}")
    rc, out, err, timed_out = run_bounded([cli, *conf.get("status_command", [])], timeout=30, env=env)
    if timed_out or rc != 0:
        raise Unavailable("not-signed-in", f"`{conf.get('cli')} {' '.join(conf.get('status_command', []))}` exited {rc}; sign in with the vendor's own CLI")
    return {"cli": cli, "version": ".".join(map(str, parse_version(raw)))}


# ---- quota -----------------------------------------------------------------------------

def _today() -> str:
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d")


@contextlib.contextmanager
def _locked_quota():
    d = data_dir()
    os.makedirs(d, exist_ok=True)
    with open(os.path.join(d, "vendor-quota.lock"), "a+") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        path = os.path.join(d, "vendor-quota.json")
        try:
            with open(path, encoding="utf-8") as fh:
                state = json.load(fh)
        except (OSError, ValueError):
            state = {}
        if state.get("date") != _today():
            state = {"date": _today(), "counts": {}}
        yield state
        tmp = path + ".tmp"
        with open(tmp, "w", encoding="utf-8") as fh:
            json.dump(state, fh, sort_keys=True)
        os.replace(tmp, path)


def reserve(pol: dict, vendor: str, key: str, key_cap) -> None:
    """Counts one call against the vendor's daily cap and the key's (stage, site or
    lane) daily cap, or raises Unavailable(over-quota) without counting it."""
    vcap = int(vendor_conf(pol, vendor).get("daily_call_cap", 0))
    kcap = int(key_cap or 0)
    with _locked_quota() as state:
        counts = state["counts"]
        vkey, skey = vendor, f"{vendor}/{key}"
        if counts.get(vkey, 0) >= vcap:
            raise Unavailable("over-quota", f"{vendor} reached its daily cap of {vcap}")
        if counts.get(skey, 0) >= kcap:
            raise Unavailable("over-quota", f"{vendor}/{key} reached its daily cap of {kcap}")
        counts[vkey] = counts.get(vkey, 0) + 1
        counts[skey] = counts.get(skey, 0) + 1


def quota_counts() -> dict:
    path = os.path.join(data_dir(), "vendor-quota.json")
    try:
        with open(path, encoding="utf-8") as fh:
            state = json.load(fh)
    except (OSError, ValueError):
        return {}
    return state.get("counts", {}) if state.get("date") == _today() else {}


# ---- redaction and bounding ---------------------------------------------------------------

_REDACTIONS = [
    (re.compile(r"-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----.*?-----END [A-Z0-9 ]*PRIVATE KEY-----", re.S),
     "[REDACTED PRIVATE KEY]"),
    (re.compile(r"\b(?:gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|glpat-[A-Za-z0-9_-]{20,}"
                r"|sk-[A-Za-z0-9_-]{16,}|AKIA[0-9A-Z]{16}|xox[abprs]-[A-Za-z0-9-]{10,}"
                r"|AIza[0-9A-Za-z_-]{35})"), "[REDACTED TOKEN]"),
    (re.compile(r"(?im)^(\s*(?:export\s+)?[A-Z0-9_]*(?:SECRET|TOKEN|PASSWORD|PASSWD|API_?KEY|CREDENTIAL)[A-Z0-9_]*\s*[=:]\s*)\S.*$"),
     r"\1[REDACTED]"),
    (re.compile(r"(?i)\b(authorization:\s*(?:bearer|basic|token)\s+)\S+"), r"\1[REDACTED]"),
    (re.compile(r"\b[A-Za-z0-9._%+-]+@(?!example\.(?:com|org|net)\b)[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)*\.[A-Za-z]{2,}\b"),
     "[REDACTED EMAIL]"),
]


def redact(text: str) -> tuple[str, int]:
    """Masks secrets, e-mail addresses (example.com excepted) and the home directory.
    Returns (text, number of replacements)."""
    total = 0
    for rx, repl in _REDACTIONS:
        text, n = rx.subn(repl, text)
        total += n
    home = os.path.expanduser("~")
    if home and home != "/" and home in text:
        total += text.count(home)
        text = text.replace(home, "~")
    return text, total


def bound(data: bytes, max_bytes: int) -> tuple[bytes, bool]:
    if len(data) <= max_bytes:
        return data, False
    return data[:max_bytes] + b"\n[... truncated by the vendor envelope ...]\n", True


# ---- schema validation (the subset the schemas use) --------------------------------

def validate(instance, schema, path="$") -> list[str]:
    errs: list[str] = []
    types = schema.get("type")
    if types is not None:
        types = types if isinstance(types, list) else [types]
        py = {"object": dict, "array": list, "string": str, "boolean": bool, "null": type(None)}
        ok = False
        for t in types:
            if t == "integer":
                ok = ok or (isinstance(instance, int) and not isinstance(instance, bool))
            elif t == "number":
                ok = ok or (isinstance(instance, (int, float)) and not isinstance(instance, bool))
            else:
                ok = ok or isinstance(instance, py[t])
        if not ok:
            return [f"{path}: expected {'/'.join(types)}"]
    if "enum" in schema and instance not in schema["enum"]:
        errs.append(f"{path}: {instance!r} not in {schema['enum']}")
    if isinstance(instance, dict):
        props = schema.get("properties", {})
        for k in schema.get("required", []):
            if k not in instance:
                errs.append(f"{path}: missing required key {k!r}")
        if schema.get("additionalProperties") is False:
            for k in instance:
                if k not in props:
                    errs.append(f"{path}: unexpected key {k!r}")
        for k, sub in props.items():
            if k in instance:
                errs += validate(instance[k], sub, f"{path}.{k}")
    if isinstance(instance, list) and "items" in schema:
        for i, item in enumerate(instance):
            errs += validate(item, schema["items"], f"{path}[{i}]")
    return errs


def load_schema(name: str) -> dict:
    with open(os.path.join(SCHEMA_DIR, f"{name}.schema.json"), encoding="utf-8") as fh:
        return json.load(fh)


# ---- responses and records ------------------------------------------------------------

def new_id(vendor: str, key: str) -> str:
    stamp = datetime.datetime.now(datetime.timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    return f"{vendor}-{key}-{stamp}-{secrets.token_hex(4)}"


def response(vendor, kind, stage, call_id, outcome="completed", reason=None, spawned=False,
             started=None, detail=None, **extra) -> dict:
    r = {
        "schema_version": 1,
        "vendor": vendor,
        "kind": kind,
        "stage": stage,
        "call_id": call_id,
        "outcome": outcome,
        "reason": reason,
        "advisory": True,
        "spawned": spawned,
        "duration_s": int(time.monotonic() - started) if started is not None else None,
        "detail": (detail or None) and str(detail)[:500],
    }
    r.update(extra)
    return r


def unavailable(vendor, kind, stage, call_id, exc: Unavailable, spawned=False, started=None, **extra) -> dict:
    return response(vendor, kind, stage, call_id, "unavailable", exc.reason, spawned, started, exc.detail, **extra)


def append_row(name: str, row: dict) -> None:
    """Appends one JSONL row under the main checkout's .claude/data. Never raises."""
    try:
        d = data_dir()
        os.makedirs(d, exist_ok=True)
        stamped = {"ts": datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"), **row}
        with open(os.path.join(d, name), "a", encoding="utf-8") as fh:
            fh.write(json.dumps(stamped, separators=(",", ":"), sort_keys=True) + "\n")
    except OSError:
        pass


def read_rows(name: str) -> list[dict]:
    rows = []
    try:
        with open(os.path.join(data_dir(), name), encoding="utf-8") as fh:
            for line in fh:
                with contextlib.suppress(ValueError):
                    obj = json.loads(line)
                    if isinstance(obj, dict):
                        rows.append(obj)
    except OSError:
        pass
    return rows


def emit(resp: dict, record: bool = True) -> int:
    """Prints the response as one JSON line, records it (except the default
    `disabled` skip, which must leave no trace for contributors who never opted in),
    and returns exit code 0: the outcome is data, never a gate."""
    if record and resp.get("reason") != "disabled":
        append_row("vendor-calls.jsonl", resp)
    print(json.dumps(resp, sort_keys=True))
    return 0


def sha256_text(text: str) -> str:
    return hashlib.sha256(text.encode("utf-8")).hexdigest()


# ---- tasks.md ----------------------------------------------------------------------------

_TASK_HEAD = re.compile(r"^###\s+Task\s+(\d+)\b(.*)$")
_FIELD = re.compile(r"^-\s+\*\*([^*]+)\*\*:\s*(.*)$")
_PATH = re.compile(r"`([^`\s]+)`")


def parse_task(spec_dir: str, number: int) -> dict:
    """One task of <spec_dir>/tasks.md: title, text, files, budget, domain, depends.
    `files` holds the first backticked path of each item under **Files**."""
    with open(os.path.join(spec_dir, "tasks.md"), encoding="utf-8") as fh:
        lines = fh.read().split("\n")
    start = end = None
    for i, line in enumerate(lines):
        m = _TASK_HEAD.match(line)
        if m and start is None and int(m.group(1)) == number:
            start = i
        elif start is not None and (line.startswith("### ") or line.startswith("## ")):
            end = i
            break
    if start is None:
        raise KeyError(f"task {number} not found in {spec_dir}/tasks.md")
    body = lines[start:end]
    task = {"number": number, "title": _TASK_HEAD.match(body[0]).group(2).strip(" —-"),
            "text": "\n".join(body).strip(), "files": [], "budget": "", "domain": "", "depends": ""}
    field = None
    for line in body[1:]:
        m = _FIELD.match(line)
        if m:
            field = m.group(1).strip().lower()
            value = m.group(2).strip()
            if field == "budget":
                task["budget"] = value
            elif field == "domain/agent":
                task["domain"] = value
            elif field == "depends on":
                task["depends"] = value
            elif field == "files":
                task["files"] += [p for p in _PATH.findall(value)[:1]]
            continue
        if field == "files" and re.match(r"^\s+-\s+", line):
            found = _PATH.findall(line)
            if found:
                task["files"].append(found[0])
        elif not line.startswith(" "):
            field = None if line.strip() else field
    return task


def budget_word(raw: str) -> str:
    m = re.match(r"\s*(trivial|standard|complex)\b", (raw or "").lower())
    return m.group(1) if m else ""


def path_in(path: str, entries) -> bool:
    """`path` equals an entry, or sits under an entry that ends with '/'."""
    for e in entries:
        if path == e.rstrip("/") or (e.endswith("/") and path.startswith(e)):
            return True
    return False


def protected_hits(pol: dict, paths, listed=()) -> list[str]:
    lanes = pol.get("lanes", {})
    allowed = set(lanes.get("allowed_when_listed", [])) & set(listed)
    return [p for p in paths if p not in allowed and path_in(p, lanes.get("protected_paths", []))]


# ---- CLI ------------------------------------------------------------------------------------

def _write_local(mutator) -> str:
    path = os.path.join(data_dir(), "vendor-policy.local.json")
    try:
        with open(path, encoding="utf-8") as fh:
            local = json.load(fh)
    except (OSError, ValueError):
        local = {}
    mutator(local)
    os.makedirs(os.path.dirname(path), exist_ok=True)
    tmp = path + ".tmp"
    with open(tmp, "w", encoding="utf-8") as fh:
        json.dump(local, fh, indent=2, sort_keys=True)
        fh.write("\n")
    os.replace(tmp, path)
    return path


def cmd_status(args) -> int:
    rows = []
    try:
        pol = load_policy()
    except Unavailable as err:
        print(f"policy: unreadable ({err.detail})")
        return 0
    counts = quota_counts()
    for v in VENDORS:
        row = {"vendor": v, "enabled": is_enabled(pol, v), "lanes": lanes_enabled(pol, v),
               "calls_today": counts.get(v, 0),
               "daily_cap": pol["vendors"].get(v, {}).get("daily_call_cap")}
        try:
            info = check_available(pol, v)
            row.update(available=True, reason=None, **info)
        except Unavailable as err:
            row.update(available=False, reason=err.reason, detail=err.detail)
        rows.append(row)
    if args.json:
        print(json.dumps(rows, sort_keys=True))
    else:
        for r in rows:
            state = "available" if r["available"] else f"unavailable ({r['reason']})"
            print(f"{r['vendor']}: {state}; lanes={'on' if r['lanes'] else 'off'}; "
                  f"calls today {r['calls_today']}/{r['daily_cap']}")
    return 0


def cmd_enable(args) -> int:
    if args.vendor not in VENDORS:
        raise UsageError(f"unknown vendor {args.vendor!r}")

    def mut(local):
        local["enabled"] = sorted(set(local.get("enabled", [])) | {args.vendor})
        if args.lanes:
            if args.vendor == "jev":
                raise UsageError("jev has no implementer lane")
            local["lanes_enabled"] = sorted(set(local.get("lanes_enabled", [])) | {args.vendor})
    print(f"enabled {args.vendor}{' (with its implementer lane)' if args.lanes else ''} in {_write_local(mut)}")
    return 0


def cmd_disable(args) -> int:
    if args.vendor not in VENDORS + ("all",):
        raise UsageError(f"unknown vendor {args.vendor!r}")
    drop = set(VENDORS) if args.vendor == "all" else {args.vendor}

    def mut(local):
        if not args.lanes_only:
            local["enabled"] = sorted(set(local.get("enabled", [])) - drop)
        local["lanes_enabled"] = sorted(set(local.get("lanes_enabled", [])) - drop)
    print(f"disabled {args.vendor}{' lanes' if args.lanes_only else ''} in {_write_local(mut)}")
    return 0


def cmd_redact(_args) -> int:
    text, n = redact(sys.stdin.read())
    sys.stdout.write(text)
    print(f"redactions={n}", file=sys.stderr)
    return 0


def cmd_quota(args) -> int:
    counts = quota_counts()
    print(json.dumps(counts, sort_keys=True) if args.json else
          "\n".join(f"{k}={v}" for k, v in sorted(counts.items())) or "no calls today")
    return 0


class Parser:
    """argparse that exits 64 on a usage error, shared by the envelope's entry points."""

    @staticmethod
    def make(prog: str, description: str):
        import argparse

        class _P(argparse.ArgumentParser):
            def error(self, message):
                self.print_usage(sys.stderr)
                print(f"{self.prog}: {message}", file=sys.stderr)
                sys.exit(EXIT_USAGE)
        return _P(prog=prog, description=description)


def main(argv=None) -> int:
    p = Parser.make("vendors.py", "optional vendor envelope: status, opt-in, redaction, quota")
    sub = p.add_subparsers(dest="cmd", required=True)
    s = sub.add_parser("status")
    s.add_argument("--json", action="store_true")
    s.set_defaults(fn=cmd_status)
    s = sub.add_parser("enable")
    s.add_argument("vendor")
    s.add_argument("--lanes", action="store_true")
    s.set_defaults(fn=cmd_enable)
    s = sub.add_parser("disable")
    s.add_argument("vendor")
    s.add_argument("--lanes-only", action="store_true")
    s.set_defaults(fn=cmd_disable)
    s = sub.add_parser("redact")
    s.set_defaults(fn=cmd_redact)
    s = sub.add_parser("quota")
    s.add_argument("--json", action="store_true")
    s.set_defaults(fn=cmd_quota)
    args = p.parse_args(argv)
    try:
        return args.fn(args)
    except UsageError as err:
        print(f"vendors.py: {err}", file=sys.stderr)
        return EXIT_USAGE


if __name__ == "__main__":
    sys.exit(main())
