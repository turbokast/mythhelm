#!/usr/bin/env python3
"""approvals.py: the approval queue that gates agent writes to product/.

    approvals.py request <id> --path product/<file> --proposed FILE [--summary TEXT]
    approvals.py list
    approvals.py show <id>
    approvals.py audit
    approvals.py approve <id> [--apply] [--yes]     maintainer only, from a terminal
    approvals.py reject <id> [--yes]                maintainer only, from a terminal
    approvals.py init-key                           maintainer only, from a terminal
    approvals.py check-write                        the hook: a PreToolUse payload on stdin

An agent drafts a product change as a complete proposed file and files it with
`request`. The request records the file's current SHA-256 (the base), the proposed
content's SHA-256 (the result) and the diff. A maintainer reviews it with
scripts/orchestration/approve.sh, which runs `show` and then `approve` or `reject`.
`approve` appends a decision row signed with HMAC-SHA256 under a key kept outside
the repository; `--apply` also writes the file.

`check-write` is called by .claude/hooks/guard-product-write.sh for Edit, Write,
MultiEdit and NotebookEdit payloads. It computes the content the tool would leave
and releases the write only when a signed, unconsumed approval names that worktree,
that path, that base and that result. `approve` checks that the request's worktree
is a worktree of this repository, shows it, and signs it with the hashes. The release is recorded as a `consumed` row, so one approval
releases one write. Because the approval binds the base, a change that lands after
the request makes it stale instead of being overwritten; because it binds the
result, it releases exactly the approved content and nothing else (not a deletion,
not a different edit). It also blocks writes to the ledger and the key.

Files, all outside git (see .gitignore):
  <main checkout>/orchestration/approvals.jsonl   append-only ledger, one JSON row per
                                                  line: decisions (approved, rejected;
                                                  signed and chained by `prev`) and
                                                  consumed rows (unsigned; they only
                                                  ever remove a grant)
  <main checkout>/orchestration/requests/<id>/    request.json, proposed, and diff (a
                                                  copy for reading; show and approve
                                                  recompute the diff from the file and
                                                  the proposed content)
  $MYTHHELM_APPROVALS_KEY, default
  ${XDG_CONFIG_HOME:-$HOME/.config}/mythhelm/approvals.key   the signing key (0600)

The main checkout is found through the git common directory, so every linked
worktree shares one queue.

Residuals, stated plainly: the key is an ordinary file readable by any process of
the same user, and the terminal test is a cost, not a boundary (a pseudo-terminal
passes it). The hook blocks agents from running the maintainer verbs and from
reading the key through the tools it sees; it cannot stop code an agent runs some
other way. Pull request review remains the final gate.

Exit codes: 0 success or allowed, 1 refused or audit problems, 2 usage error;
check-write exits 2 to block.
"""

import argparse
import datetime
import difflib
import fcntl
import getpass
import hashlib
import hmac
import json
import os
import re
import secrets
import subprocess
import sys
import tempfile

ID_RE = re.compile(r"^[a-z0-9][a-z0-9-]{0,63}$")
ABSENT = "absent"
MAINTAINER_VERBS = ("approve", "reject", "init-key")


class Refused(Exception):
    """The command cannot proceed; the message says why and what to do."""


def now():
    return datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def sha256_bytes(data):
    return hashlib.sha256(data).hexdigest()


def git_out(cwd, *args):
    env = {k: v for k, v in os.environ.items() if k not in ("GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE")}
    try:
        r = subprocess.run(["git", "-C", cwd] + list(args), capture_output=True, timeout=20, env=env)
    except (OSError, subprocess.SubprocessError):
        return None
    return r.stdout.decode("utf-8", "replace").strip() if r.returncode == 0 else None


def nearest_dir(path):
    d = path
    while d and not os.path.isdir(d):
        parent = os.path.dirname(d)
        if parent == d:
            break
        d = parent
    return d or "/"


def toplevel_of(path):
    return git_out(nearest_dir(path), "rev-parse", "--show-toplevel")


def orchestration_dir(path):
    """<main checkout>/orchestration for the repository holding path."""
    common = git_out(nearest_dir(path), "rev-parse", "--path-format=absolute", "--git-common-dir")
    if not common:
        raise Refused("%s is not inside a git repository" % path)
    return os.path.join(os.path.dirname(os.path.realpath(common)), "orchestration")


def key_path():
    explicit = os.environ.get("MYTHHELM_APPROVALS_KEY")
    if explicit:
        return os.path.realpath(explicit)
    base = os.environ.get("XDG_CONFIG_HOME") or os.path.join(os.path.expanduser("~"), ".config")
    return os.path.realpath(os.path.join(base, "mythhelm", "approvals.key"))


def read_key():
    try:
        with open(key_path(), encoding="utf-8") as f:
            key = f.read().strip()
    except OSError:
        return None
    return key or None


def canonical(row):
    return json.dumps({k: v for k, v in row.items() if k != "mac"}, sort_keys=True, separators=(",", ":"))


def mac_of(key, row):
    return hmac.new(key.encode("utf-8"), canonical(row).encode("utf-8"), hashlib.sha256).hexdigest()


def canon_product_path(p):
    """product/<file> in canonical repo-relative form, or Refused."""
    v = p.strip()
    while v.startswith("./"):
        v = v[2:]
    v = re.sub(r"/+", "/", v)
    if v.startswith("/") or ".." in v.split("/") or re.search(r"[\s*?\[\]]", v):
        raise Refused("path %r must be a plain repository-relative path such as product/backlog.md" % p)
    if not v.startswith("product/") or v == "product/" or v.endswith("/"):
        raise Refused("path %r is not a file under product/; one request names one product file" % p)
    return v


class Ledger:
    def __init__(self, orch):
        self.orch = orch
        self.path = os.path.join(orch, "approvals.jsonl")
        self.requests = os.path.join(orch, "requests")
        self._lock = None

    def __enter__(self):
        os.makedirs(self.orch, exist_ok=True)
        self._lock = open(self.path + ".lock", "a", encoding="utf-8")
        fcntl.flock(self._lock, fcntl.LOCK_EX)
        return self

    def __exit__(self, *exc):
        fcntl.flock(self._lock, fcntl.LOCK_UN)
        self._lock.close()

    def rows(self):
        out = []
        try:
            with open(self.path, encoding="utf-8") as f:
                for line in f:
                    line = line.strip()
                    if not line:
                        continue
                    try:
                        row = json.loads(line)
                    except ValueError:
                        row = {"_malformed": line[:80]}
                    out.append(row if isinstance(row, dict) else {"_malformed": line[:80]})
        except FileNotFoundError:
            pass
        return out

    def append(self, row):
        with open(self.path, "a", encoding="utf-8") as f:
            f.write(json.dumps(row, sort_keys=True) + "\n")

    def chain_head(self):
        macs = [r.get("mac") for r in self.rows() if r.get("state") in ("approved", "rejected") and r.get("mac")]
        return macs[-1] if macs else "genesis"

    def latest_decisions(self):
        latest = {}
        for r in self.rows():
            if r.get("state") in ("approved", "rejected"):
                latest[r.get("id")] = r
        return latest

    def consumed(self):
        return {r.get("consumes") for r in self.rows() if r.get("state") == "consumed"}

    def request(self, ident):
        if not ID_RE.match(ident or ""):
            raise Refused("%r is not a request id" % ident)
        d = os.path.join(self.requests, ident)
        try:
            with open(os.path.join(d, "request.json"), encoding="utf-8") as f:
                req = json.load(f)
        except (OSError, ValueError) as e:
            raise Refused("no readable request %r in %s" % (ident, self.requests)) from e
        return req, d


# ---- agent verbs -----------------------------------------------------------

def unified(current, proposed, rel):
    """The unified diff from the current bytes to the proposed bytes. Both must be
    valid UTF-8: replacement decoding could make different bytes show no change."""
    try:
        old, new = current.decode("utf-8"), proposed.decode("utf-8")
    except UnicodeDecodeError as e:
        raise Refused("%s or its proposed content is not valid UTF-8, so its diff cannot be shown "
                      "faithfully" % rel) from e
    return "".join(difflib.unified_diff(old.splitlines(True), new.splitlines(True), "a/" + rel, "b/" + rel))


def cmd_request(a):
    if not ID_RE.match(a.id):
        raise Refused("id %r must be lowercase letters, digits and hyphens (at most 64)" % a.id)
    root = git_out(os.getcwd(), "rev-parse", "--show-toplevel")
    if not root:
        raise Refused("run request from inside the repository")
    rel = canon_product_path(a.path)
    target = os.path.join(root, rel)
    proposed_path = os.path.realpath(a.proposed)
    product_dir = os.path.realpath(os.path.join(root, "product"))
    if proposed_path.startswith(product_dir + os.sep):
        raise Refused("the proposed file %s is inside product/; draft it in a scratch file" % a.proposed)
    try:
        with open(proposed_path, "rb") as f:
            proposed = f.read()
    except OSError as e:
        raise Refused("cannot read the proposed file %s: %s" % (a.proposed, e.strerror)) from e
    try:
        with open(target, "rb") as f:
            current = f.read()
        base = sha256_bytes(current)
    except FileNotFoundError:
        current, base = b"", ABSENT
    if base != ABSENT and current == proposed:
        raise Refused("the proposed content is identical to %s; there is nothing to approve" % rel)
    diff = unified(current, proposed, rel)
    ledger = Ledger(orchestration_dir(root))
    with ledger:
        d = os.path.join(ledger.requests, a.id)
        if os.path.exists(d):
            raise Refused("a request %r already exists; choose another id" % a.id)
        os.makedirs(d)
        with open(os.path.join(d, "proposed"), "wb") as f:
            f.write(proposed)
        with open(os.path.join(d, "diff"), "w", encoding="utf-8") as f:
            f.write(diff)
        req = {"schema_version": 1, "id": a.id, "path": rel, "worktree": root,
               "base_sha256": base, "result_sha256": sha256_bytes(proposed),
               "diff_sha256": sha256_bytes(diff.encode("utf-8")), "summary": (a.summary or "").strip(),
               "requested_at": now(), "session": os.environ.get("CLAUDE_CODE_SESSION_ID", "")}
        with open(os.path.join(d, "request.json"), "w", encoding="utf-8") as f:
            json.dump(req, f, indent=2, sort_keys=True)
            f.write("\n")
    print("filed request %s for %s (%d diff lines)" % (a.id, rel, diff.count("\n")))
    print("A maintainer reviews it from their own terminal: scripts/orchestration/approve.sh show %s, "
          "then approve.sh approve %s [--apply] or approve.sh reject %s" % (a.id, a.id, a.id))


def status_of(ledger, ident):
    dec = ledger.latest_decisions().get(ident)
    if not dec:
        return "open"
    if dec["state"] == "approved" and dec.get("mac") in ledger.consumed():
        return "applied"
    return dec["state"]


def cmd_list(_a):
    ledger = Ledger(orchestration_dir(os.getcwd()))
    names = sorted(os.listdir(ledger.requests)) if os.path.isdir(ledger.requests) else []
    if not names:
        print("no requests")
    for n in names:
        try:
            req, _ = ledger.request(n)
        except Refused:
            print("%-32s unreadable" % n)
            continue
        print("%-32s %-9s %-24s %s" % (n, status_of(ledger, n), req.get("path"), req.get("summary", "")[:60]))


def cmd_show(a):
    ledger = Ledger(orchestration_dir(os.getcwd()))
    req, d = ledger.request(a.id)
    print("request   %s (%s)" % (a.id, status_of(ledger, a.id)))
    print("path      %s in %s" % (req.get("path"), req.get("worktree")))
    print("summary   %s" % req.get("summary"))
    print("requested %s" % req.get("requested_at"))
    # The diff is recomputed from the file and the proposed content, never read from
    # the stored copy, which anyone who can write the request directory could edit.
    try:
        rel = canon_product_path(req.get("path", ""))
        with open(os.path.join(d, "proposed"), "rb") as f:
            proposed = f.read()
        target = os.path.join(os.path.realpath(req.get("worktree") or "/nonexistent"), rel)
        try:
            with open(target, "rb") as f:
                current = f.read()
        except FileNotFoundError:
            current = b""
    except OSError as e:
        raise Refused("request %s cannot be read: %s" % (a.id, e.strerror)) from e
    sys.stdout.write(unified(current, proposed, rel))


def cmd_audit(_a):
    ledger = Ledger(orchestration_dir(os.getcwd()))
    key = read_key()
    problems, prev, n = 0, "genesis", 0
    if not key:
        print("no key at %s: no decision can be verified" % key_path())
    for r in ledger.rows():
        if "_malformed" in r:
            print("MALFORMED %s" % r["_malformed"])
            problems += 1
            continue
        if r.get("state") not in ("approved", "rejected"):
            continue
        n += 1
        tag = "%s (%s)" % (r.get("id"), r.get("state"))
        if r.get("prev") != prev:
            print("CHAIN-BREAK %s: prev does not link to the decision above it" % tag)
            problems += 1
        if not r.get("mac"):
            print("UNSIGNED %s" % tag)
            problems += 1
        elif key and not hmac.compare_digest(r["mac"], mac_of(key, r)):
            print("FORGED %s: the signature does not verify" % tag)
            problems += 1
        else:
            print("OK %s by %s at %s" % (tag, r.get("decided_by"), r.get("decided_at")))
        prev = r.get("mac") or prev
    print("audited %d decision(s): %d problem(s)" % (n, problems + (0 if key else 1)))
    return 1 if problems or not key else 0


# ---- maintainer verbs ------------------------------------------------------

def require_terminal(verb):
    if not (sys.stdin.isatty() and sys.stdout.isatty()):
        raise Refused("%s is the maintainer's decision and runs only from an interactive terminal; "
                      "an agent files a request and waits" % verb)


def confirm(prompt, yes):
    if yes:
        return True
    sys.stdout.write(prompt + " [y/N] ")
    sys.stdout.flush()
    return sys.stdin.readline().strip().lower() in ("y", "yes")


def cmd_init_key(_a):
    require_terminal("init-key")
    path = key_path()
    if os.path.exists(path):
        print("a key already exists at %s; delete it only to invalidate every signed decision" % path)
        return
    os.makedirs(os.path.dirname(path), mode=0o700, exist_ok=True)
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, "w", encoding="utf-8") as f:
        f.write(secrets.token_hex(32) + "\n")
    print("created the approval key at %s (mode 0600)" % path)


def verified_request(ledger, ident):
    req, d = ledger.request(ident)
    rel = canon_product_path(req.get("path", ""))
    with open(os.path.join(d, "proposed"), "rb") as f:
        proposed = f.read()
    if sha256_bytes(proposed) != req.get("result_sha256"):
        raise Refused("request %s was altered after it was filed (the proposed content no longer matches "
                      "its recorded hash); reject it and ask for a new request" % ident)
    worktree = os.path.realpath(req.get("worktree") or "/nonexistent")
    top = toplevel_of(worktree)
    if not top or os.path.realpath(top) != worktree:
        raise Refused("request %s names %s, which is not the top of a worktree" % (ident, worktree))
    if os.path.realpath(orchestration_dir(worktree)) != os.path.realpath(ledger.orch):
        raise Refused("request %s names a worktree of another repository (%s)" % (ident, worktree))
    target = os.path.join(worktree, rel)
    # One read of the file: the base checked, the diff shown and the hashes signed
    # all come from the same bytes. The stored diff is never trusted.
    try:
        with open(target, "rb") as f:
            current = f.read()
        base = sha256_bytes(current)
    except FileNotFoundError:
        current, base = b"", ABSENT
    if base != req.get("base_sha256"):
        raise Refused("%s has changed since request %s was filed, so the diff no longer describes what "
                      "would change; reject it and ask for a redraft" % (target, ident))
    return req, rel, target, proposed, unified(current, proposed, rel), base


def decide(a, state):
    require_terminal("approve" if state == "approved" else "reject")
    key = read_key()
    if not key:
        raise Refused("no approval key at %s; create one with: scripts/orchestration/approve.sh init-key" % key_path())
    ledger = Ledger(orchestration_dir(os.getcwd()))
    with ledger:
        if status_of(ledger, a.id) != "open":
            raise Refused("request %s is already %s" % (a.id, status_of(ledger, a.id)))
        if state == "approved":
            req, rel, target, proposed, diff, base = verified_request(ledger, a.id)
            sys.stdout.write(diff)
        else:
            req, _ = ledger.request(a.id)
            rel, target, proposed, diff, base = req.get("path"), None, None, "", req.get("base_sha256")
        worktree = os.path.realpath(req.get("worktree") or "/nonexistent")
        print("\n%s: %s in %s\n  %s" % (a.id, rel, worktree, req.get("summary", "")))
        if not confirm("%s request %s?" % ("Approve" if state == "approved" else "Reject", a.id), a.yes):
            print("nothing recorded")
            return
        row = {"schema_version": 1, "id": a.id, "state": state, "path": rel, "worktree": worktree,
               "base_sha256": base,
               "result_sha256": sha256_bytes(proposed) if proposed is not None else req.get("result_sha256"),
               "diff_sha256": sha256_bytes(diff.encode("utf-8")) if proposed is not None else req.get("diff_sha256"),
               "decided_at": now(),
               "decided_by": "terminal:" + (getpass.getuser() or "maintainer"), "prev": ledger.chain_head()}
        row["mac"] = mac_of(key, row)
        ledger.append(row)
        print("recorded: %s %s" % (a.id, state))
        if state == "approved" and a.apply:
            os.makedirs(os.path.dirname(target), exist_ok=True)
            fd, tmp = tempfile.mkstemp(dir=os.path.dirname(target), prefix=".approve-")
            with os.fdopen(fd, "wb") as f:
                f.write(proposed)
            os.replace(tmp, target)
            ledger.append({"schema_version": 1, "id": a.id, "state": "consumed", "consumes": row["mac"],
                           "path": rel, "at": now(), "by": "approve.sh --apply"})
            print("applied %s; commit it in a pull request" % target)


# ---- the hook --------------------------------------------------------------

def block(target, detail, fix):
    sys.stderr.write("BLOCK: guard-product-write\nFile: %s\nDetail: %s\nFix: %s\n" % (target, detail, fix))
    raise SystemExit(2)


FIX_REQUEST = ("Draft the complete new file in a scratch file (python3 scripts/pm/pm.py <verb> --out <file>), "
               "file it with python3 scripts/orchestration/approvals.py request <id> --path <product path> "
               "--proposed <file> --summary '<why>', then continue with other work; a maintainer approves it "
               "from their own terminal with scripts/orchestration/approve.sh.")


def proposed_content(tool, ti, current, target):
    """The bytes the tool would leave in the file, or None when that cannot be computed."""
    if tool == "Write":
        c = ti.get("content")
        return c.encode("utf-8") if isinstance(c, str) else None
    if current is None:
        return None
    try:
        text = current.decode("utf-8")
    except UnicodeDecodeError:
        return None
    edits = ti.get("edits") if tool == "MultiEdit" else [ti]
    if not isinstance(edits, list):
        return None
    for e in edits:
        if not isinstance(e, dict):
            return None
        old, new = e.get("old_string"), e.get("new_string")
        if not isinstance(old, str) or not isinstance(new, str) or not old or old not in text:
            return None
        text = text.replace(old, new) if e.get("replace_all") else text.replace(old, new, 1)
    return text.encode("utf-8")


def cmd_check_write(_a):
    try:
        payload = json.load(sys.stdin)
    except ValueError:
        block("<unparsed payload>", "the hook payload is not valid JSON, so a product write cannot be ruled out.",
              "Retry the call; report the malformed payload if it persists.")
    tool = payload.get("tool_name", "")
    ti = payload.get("tool_input") or {}
    raw = ti.get("file_path") or ti.get("notebook_path") or ""
    if not raw:
        return 0
    base_dir = payload.get("cwd") or os.environ.get("CLAUDE_PROJECT_DIR") or os.getcwd()
    target = os.path.realpath(raw if os.path.isabs(raw) else os.path.join(base_dir, raw))
    if target == key_path():
        block(target, "this is the maintainer's approval key; an agent never reads or writes it.",
              "Leave the key to the maintainer (scripts/orchestration/approve.sh init-key).")
    if os.path.basename(target) == "approvals.jsonl" and os.path.basename(os.path.dirname(target)) == "orchestration":
        block(target, "the approval ledger records maintainer decisions and is written only by approvals.py; "
                      "an agent-written row would be a self-approval.", FIX_REQUEST)
    top = toplevel_of(target)
    if not top:
        return 0
    rel = os.path.relpath(target, os.path.realpath(top))
    if rel != "product" and not rel.startswith("product" + os.sep):
        return 0
    rel = rel.replace(os.sep, "/")
    if tool not in ("Write", "Edit", "MultiEdit"):
        block(target, "%s cannot be approved for product/ files; only Write, Edit and MultiEdit carry content "
                      "an approval can bind." % tool, FIX_REQUEST)
    try:
        with open(target, "rb") as f:
            current = f.read()
        base = sha256_bytes(current)
    except FileNotFoundError:
        current, base = None, ABSENT
    except OSError as e:
        block(target, "the file cannot be read (%s), so the approval it needs cannot be matched." % e.strerror,
              FIX_REQUEST)
    result_bytes = proposed_content(tool, ti, current, target)
    if result_bytes is None:
        block(target, "the content this %s would leave cannot be computed (the file is missing or old_string "
                      "does not occur), so no approval can match it." % tool,
              "Write the approved content in full with the Write tool: it is in "
              "<main checkout>/orchestration/requests/<id>/proposed. " + FIX_REQUEST)
    result = sha256_bytes(result_bytes)
    key = read_key()
    ledger = Ledger(orchestration_dir(target))
    with ledger:
        consumed = ledger.consumed()
        reasons = []
        for ident, row in sorted(ledger.latest_decisions().items(), key=lambda kv: str(kv[0])):
            if row.get("state") != "approved" or row.get("path") != rel:
                continue
            if not key or not row.get("mac") or not hmac.compare_digest(row["mac"], mac_of(key, row)):
                reasons.append("approval %s does not carry a valid maintainer signature" % ident)
                continue
            if row["mac"] in consumed:
                reasons.append("approval %s was already used for one write" % ident)
                continue
            if row.get("worktree") != os.path.realpath(top):
                reasons.append("approval %s was signed for the worktree %s, not this one" % (ident, row.get("worktree")))
                continue
            if row.get("base_sha256") != base:
                reasons.append("approval %s was made against different content of %s, which has changed since "
                               "(stale: ask for a redraft)" % (ident, rel))
                continue
            if row.get("result_sha256") != result:
                reasons.append("approval %s approves different content than this %s would write" % (ident, tool))
                continue
            ledger.append({"schema_version": 1, "id": ident, "state": "consumed", "consumes": row["mac"],
                           "path": rel, "at": now(), "by": "hook:%s" % tool,
                           "session": payload.get("session_id", "")})
            return 0
    detail = "product/ changes need a maintainer's signed approval of this exact change, and none matches"
    detail += (": " + "; ".join(reasons) + ".") if reasons else "."
    block(target, detail, FIX_REQUEST)
    return 2


def main(argv):
    p = argparse.ArgumentParser(prog="approvals.py", description=__doc__.split("\n")[0])
    sub = p.add_subparsers(dest="cmd", required=True)
    rq = sub.add_parser("request")
    rq.add_argument("id")
    rq.add_argument("--path", required=True)
    rq.add_argument("--proposed", required=True)
    rq.add_argument("--summary")
    sub.add_parser("list")
    sub.add_parser("show").add_argument("id")
    sub.add_parser("audit")
    ap = sub.add_parser("approve")
    ap.add_argument("id")
    ap.add_argument("--apply", action="store_true")
    ap.add_argument("--yes", action="store_true")
    rj = sub.add_parser("reject")
    rj.add_argument("id")
    rj.add_argument("--yes", action="store_true")
    sub.add_parser("init-key")
    sub.add_parser("check-write")
    a = p.parse_args(argv)
    try:
        if a.cmd == "request":
            cmd_request(a)
        elif a.cmd == "list":
            cmd_list(a)
        elif a.cmd == "show":
            cmd_show(a)
        elif a.cmd == "audit":
            return cmd_audit(a)
        elif a.cmd == "approve":
            decide(a, "approved")
        elif a.cmd == "reject":
            decide(a, "rejected")
        elif a.cmd == "init-key":
            cmd_init_key(a)
        elif a.cmd == "check-write":
            return cmd_check_write(a)
    except Refused as e:
        if a.cmd == "check-write":
            block("<payload>", str(e), FIX_REQUEST)
        print("approvals.py: %s" % e, file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
