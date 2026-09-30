#!/usr/bin/env python3
"""autonomy.py: the autonomy grant, the maintainer's time-boxed, scope-limited
permission for one agent session to run the delivery loop unattended.

    autonomy.py grant  --hours N --scope MH-3,spec:<name>,... --reason TEXT
                       [--session ID] [--allow-pm-sync] [--no-spec-checkpoint]
                       [--max-continues N]                 maintainer only, from a terminal
    autonomy.py renew  --hours N [--reason TEXT]           maintainer only, from a terminal
    autonomy.py revoke [--reason TEXT]
    autonomy.py status [--json]
    autonomy.py check  [--session ID] [--spec NAME] [--card MH-<n>]
    autonomy.py merge-check --pr N --spec NAME (--task N | --lifecycle | --finalize | --fix)
                       [--repo OWNER/REPO] [--session ID]
    autonomy.py pm-sync-check --path product/<file> --proposed FILE [--session ID]

A grant binds ONE session (a subagent's hooks carry its parent's session id, so the
session's workers are covered too) until a deadline at most MAX_HOURS away, over a
list of backlog cards and specs. It lives in <main checkout>/.claude/data/
autonomy-grant.json (gitignored); every grant, renewal and revocation, and every merge
verdict recorded under it, is appended to autonomy-audit.jsonl beside it.

What a live grant changes, and nothing else:
  * continue-run.sh (Stop) nudges the granted session to keep working while the
    delivery run has actionable work, within the grant's continue budget;
  * guard-blocking-ask.sh blocks AskUserQuestion in it; questions go to
    orchestration/QUESTIONS.md through delivery.py;
  * the session may merge a pull request only after merge-check recorded a ready
    verdict for that pull request's exact head: every check green, no unresolved
    review thread, the leak check passed and the changed files inside the spec's
    task scope (runspec.py pr-check or finalize.py publish-check decide), and the
    spec inside the grant's scope. guard-autonomy.sh enforces it, and blocks the
    arming script in the granted session, so a grant never publishes, tags or
    releases;
  * with --allow-pm-sync, pm-sync-check pre-approves the lifecycle status moves
    specced -> implementing -> shipped of granted cards, with their lifecycle-sync
    decision entries and the regenerated roadmap. Every other product change still
    needs the maintainer's signed approval.

grant and renew refuse without an interactive terminal (an agent's Bash tool has
none), and guard-autonomy.sh blocks every agent call of them and every agent write
to the grant or audit file. Both are costs, not boundaries: a pseudo-terminal passes
the terminal test and the grant is a file the same user can write. They stop the
ordinary path, an agent granting itself autonomy. revoke is open to anyone, because
it only removes autonomy.

check exits 0 when a live grant covers the session (and the spec or card), 1 when not.
merge-check exits 0 on a ready verdict (and records it), 1 when not ready, 3 when
GitHub has not decided yet. pm-sync-check exits 0 when the change is pre-approved,
1 when not. Every command exits 2 on a usage error or a refusal.
"""

import argparse
import getpass
import json
import os
import re
import secrets
import subprocess
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import orchlib  # noqa: E402
from orchlib import Refused  # noqa: E402

MAX_HOURS = 24
DEFAULT_MAX_CONTINUES = 30
MAX_CONTINUES_CEILING = 200
MERGE_RECORD_TTL = 900
SESSION_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$")
CARD_RE = re.compile(r"^MH-([1-9][0-9]*)$")
SPEC_RE = re.compile(r"^[a-z0-9][a-z0-9-]*$")
CARD_HEAD = re.compile(r"^### (MH-[1-9][0-9]*): ")
FIELD_LINE = re.compile(r"^- \*\*([^*]+)\*\*: (.*)$")
DECISION_HEAD = re.compile(r"^### D-[1-9][0-9]* — \d{4}-\d{2}-\d{2}: \S")
PM_SYNC_STEPS = {("specced", "implementing"), ("implementing", "shipped")}
HERE = os.path.dirname(os.path.abspath(__file__))


def paths(root):
    d = orchlib.data_dir(root)
    return os.path.join(d, "autonomy-grant.json"), os.path.join(d, "autonomy-audit.jsonl")


def audit(root, event, grant=None, **detail):
    _, log = paths(root)
    row = {"ts": orchlib.iso(orchlib.utc_now()), "event": event,
           "grant_id": (grant or {}).get("id", ""), "session_id": (grant or {}).get("session_id", "")}
    row.update(detail)
    orchlib.append_jsonl(log, row)


def load(root):
    path, _ = paths(root)
    return orchlib.read_json(path)


def state_of(grant, now=None):
    """'live', 'expired' or 'invalid' for a grant dict; None for no grant."""
    if grant is None:
        return None
    now = now or orchlib.utc_now()
    try:
        until = int(grant["until_epoch"])
        issued = int(grant["issued_epoch"])
        ok = (grant.get("schema_version") == 1 and SESSION_RE.match(grant.get("session_id", ""))
              and 0 < until - issued <= MAX_HOURS * 3600)
    except (KeyError, TypeError, ValueError):
        ok = False
    if not ok:
        return "invalid"
    return "live" if now.timestamp() < until else "expired"


def require_terminal(verb):
    if not (sys.stdin.isatty() and sys.stdout.isatty()):
        raise Refused("%s needs an interactive terminal: only the maintainer grants or renews autonomy, "
                      "from their own terminal. In Claude Code use `! scripts/orchestration/autonomy.sh %s ...`; "
                      "for Codex, run it in a terminal with --session <Codex session id>."
                      % (verb, verb))


def session_from_environment():
    """Use the native session ID when a maintainer invokes the command in its shell."""
    return (os.environ.get("CLAUDE_CODE_SESSION_ID") or os.environ.get("CODEX_SESSION_ID")
            or os.environ.get("CODEX_THREAD_ID") or "")


def parse_hours(value):
    if not re.fullmatch(r"[0-9]{1,3}", value or "") or not 1 <= int(value) <= MAX_HOURS:
        raise Refused("--hours must be a whole number from 1 to %d; got %r. Nothing was written." % (MAX_HOURS, value))
    return int(value)


def parse_scope(value, root):
    cards, specs = [], []
    for tok in [t.strip() for t in (value or "").split(",") if t.strip()]:
        if CARD_RE.match(tok):
            cards.append(tok)
        elif tok.startswith("spec:") and SPEC_RE.match(tok[5:]):
            specs.append(tok[5:])
        else:
            raise Refused("scope item %r is neither MH-<n> nor spec:<name>. Nothing was written." % tok)
    if not cards and not specs:
        raise Refused("--scope names no card or spec; a grant always has a scope. Nothing was written.")
    known = backlog_cards(root)
    missing = [c for c in cards if c not in known]
    if missing:
        raise Refused("no such card in product/backlog.md: %s. Nothing was written." % ", ".join(missing))
    return {"cards": sorted(set(cards), key=lambda c: int(c[3:])), "specs": sorted(set(specs))}


# ---- the backlog, read only as far as scope and pm-sync need ---------------------

def parse_backlog(text):
    """(preamble lines, {card id: (heading, {field: value})}), or None when unparseable.
    The preamble is the text above the first section; section headings, blank lines and
    the empty-section placeholders carry no content, so a card moving between the Open
    and Closed sections changes nothing but the card."""
    pre, cards, cur, in_pre = [], {}, None, True
    for line in (text or "").split("\n"):
        m = CARD_HEAD.match(line)
        if m:
            if m.group(1) in cards:
                return None
            in_pre, cur = False, m.group(1)
            cards[cur] = (line, {})
            continue
        if line in ("## Open", "## Closed"):
            in_pre, cur = False, None
            continue
        if in_pre:
            pre.append(line)
            continue
        if not line.strip() or line.strip() in ("No open cards.", "No closed cards."):
            continue
        f = FIELD_LINE.match(line)
        if not (f and cur):
            return None
        cards[cur][1][f.group(1)] = f.group(2)
    while pre and not pre[-1].strip():
        pre.pop()
    return pre, cards


def backlog_cards(root):
    try:
        with open(os.path.join(orchlib.main_checkout(root), "product", "backlog.md"), encoding="utf-8") as fh:
            parsed = parse_backlog(fh.read())
    except OSError:
        return {}
    return parsed[1] if parsed else {}


def card_specs(fields):
    return re.findall(r"`([a-z0-9][a-z0-9-]*)`", fields.get("Spec", ""))


def spec_in_scope(grant, spec, root):
    if spec in grant["scope"]["specs"]:
        return True
    cards = backlog_cards(root)
    return any(spec in card_specs(cards.get(c, ("", {}))[1]) for c in grant["scope"]["cards"])


def covers(grant, session, root, spec=None, card=None):
    """None when the live grant covers the session (and spec or card), else the reason."""
    st = state_of(grant)
    if st is None:
        return "no autonomy grant"
    if st != "live":
        return "the autonomy grant is %s" % st
    if session and session != grant["session_id"]:
        return "the grant binds session %s, not %s" % (grant["session_id"], session)
    if card and card not in grant["scope"]["cards"]:
        return "%s is not in the grant's scope" % card
    if spec and not spec_in_scope(grant, spec, root):
        return "spec %s is not in the grant's scope (neither named nor the Spec of a granted card)" % spec
    return None


# ---- verbs ----------------------------------------------------------------------

def write_grant(root, grant):
    path, _ = paths(root)
    orchlib.atomic_write(path, json.dumps(grant, indent=1, sort_keys=True) + "\n")


def cmd_grant(a, root):
    hours = parse_hours(a.hours)
    scope = parse_scope(a.scope, root)
    if not (a.reason or "").strip():
        raise Refused("--reason is required: it is recorded in the audit log. Nothing was written.")
    mc = a.max_continues if a.max_continues is not None else DEFAULT_MAX_CONTINUES
    if not 0 <= mc <= MAX_CONTINUES_CEILING:
        raise Refused("--max-continues must be 0 to %d. Nothing was written." % MAX_CONTINUES_CEILING)
    session = a.session or session_from_environment()
    if not SESSION_RE.match(session or ""):
        raise Refused("no session to bind: pass --session <id> (the session's id), or run this "
                      "from the Claude Code or Codex session to be granted. A grant with no session applies to none.")
    require_terminal("grant")
    now = orchlib.utc_now()
    grant = {
        "schema_version": 1, "id": secrets.token_hex(6), "session_id": session,
        "granted_at": orchlib.iso(now), "issued_epoch": int(now.timestamp()),
        "until_epoch": int(now.timestamp()) + hours * 3600,
        "until": orchlib.iso(now.fromtimestamp(now.timestamp() + hours * 3600, now.tzinfo)),
        "scope": scope, "allow_pm_sync": bool(a.allow_pm_sync), "spec_checkpoint": not a.no_spec_checkpoint,
        "max_continues": mc, "reason": a.reason.strip(), "granted_by": "terminal:%s" % getpass.getuser(),
        "renewals": 0,
    }
    old = load(root)
    write_grant(root, grant)
    audit(root, "grant", grant, hours=hours, reason=grant["reason"], scope=scope,
          replaced=(old or {}).get("id", ""))
    print("autonomy: granted to session %s until %s (%dh)" % (session, grant["until"], hours))
    print("scope: %s" % ", ".join(scope["cards"] + ["spec:" + s for s in scope["specs"]]))
    print("pm-sync pre-approval: %s; spec checkpoint: %s; continue budget: %d"
          % ("on" if grant["allow_pm_sync"] else "off", "required" if grant["spec_checkpoint"] else "skipped", mc))
    return 0


def cmd_renew(a, root):
    hours = parse_hours(a.hours)
    grant = load(root)
    if state_of(grant) not in ("live", "expired"):
        raise Refused("there is no valid grant to renew; grant a new one.")
    require_terminal("renew")
    now = orchlib.utc_now()
    grant["issued_epoch"] = int(now.timestamp())
    grant["until_epoch"] = grant["issued_epoch"] + hours * 3600
    grant["until"] = orchlib.iso(now.fromtimestamp(grant["until_epoch"], now.tzinfo))
    grant["renewals"] = int(grant.get("renewals", 0)) + 1
    write_grant(root, grant)
    audit(root, "renew", grant, hours=hours, reason=(a.reason or "").strip())
    print("autonomy: renewed for session %s until %s (%dh)" % (grant["session_id"], grant["until"], hours))
    return 0


def cmd_revoke(a, root):
    path, _ = paths(root)
    grant = load(root)
    if not os.path.exists(path):
        print("autonomy: no grant")
        return 0
    os.unlink(path)
    audit(root, "revoke", grant, reason=(a.reason or "").strip())
    print("autonomy: revoked; interactive rules apply from the next turn")
    return 0


def cmd_status(a, root):
    grant = load(root)
    st = state_of(grant)
    if a.json:
        print(json.dumps({"state": st or "none", "grant": grant}, sort_keys=True))
        return 0
    if st is None:
        print("autonomy: no grant")
        return 0
    if st == "invalid":
        print("autonomy: the grant file is invalid; it grants nothing (revoke it, then grant again)")
        return 0
    left = max(0, int(grant["until_epoch"] - orchlib.utc_now().timestamp()))
    print("autonomy: %s, session %s, until %s (%dh%02dm left), renewals %d"
          % (st, grant["session_id"], grant["until"], left // 3600, left % 3600 // 60, grant.get("renewals", 0)))
    print("scope: %s" % ", ".join(grant["scope"]["cards"] + ["spec:" + s for s in grant["scope"]["specs"]]))
    print("pm-sync pre-approval: %s; spec checkpoint: %s; continue budget: %s"
          % ("on" if grant.get("allow_pm_sync") else "off",
             "required" if grant.get("spec_checkpoint", True) else "skipped", grant.get("max_continues")))
    return 0


def cmd_check(a, root):
    why = covers(load(root), a.session, root, spec=a.spec, card=a.card)
    print("autonomy: %s" % ("covered" if why is None else "not covered: " + why))
    return 0 if why is None else 1


def run_check(cmd):
    r = subprocess.run(cmd, capture_output=True, text=True)
    out = r.stdout
    facts = dict(line.split("=", 1) for line in out.split("\n") if "=" in line and not line.startswith("reason="))
    return r.returncode, facts, out, r.stderr


def cmd_merge_check(a, root):
    grant = load(root)
    session = a.session or session_from_environment() or (grant or {}).get("session_id", "")
    why = covers(grant, session, root, spec=a.spec)
    if why:
        print("reason=%s" % why)
        print("verdict=not-ready")
        return 1
    py = sys.executable or "python3"
    repo = ["--repo", a.repo] if a.repo else []
    if a.finalize or a.fix:
        cmd = [py, os.path.join(HERE, "..", "harness", "finalize.py"), "publish-check", "--spec", a.spec,
               "--pr", str(a.pr)] + repo + (["--fix"] if a.fix else [])
    else:
        cmd = [py, os.path.join(HERE, "..", "harness", "runspec.py"), "pr-check", "--spec", a.spec,
               "--pr", str(a.pr)] + repo + (["--task", str(a.task)] if a.task is not None else ["--lifecycle"])
    rc, facts, out, err = run_check(cmd)
    sys.stdout.write(out)
    if err.strip():
        sys.stderr.write(err)
    head = facts.get("head", "")
    if rc == 0 and facts.get("verdict") == "ready" and re.fullmatch(r"[0-9a-f]{40}", head):
        now = orchlib.utc_now()
        audit(root, "merge-ready", grant, pr=int(a.pr), head=head, spec=a.spec, at_epoch=int(now.timestamp()))
        print("merge: recorded; within %ds run: gh pr merge %s --squash --match-head-commit %s"
              % (MERGE_RECORD_TTL, a.pr, head))
        return 0
    return 3 if rc == 3 else 1


# ---- pm-sync pre-approval ----------------------------------------------------------

def pm_sync_problem(grant, rel, current, proposed, root):
    """None when writing proposed over current at product/<rel> is a pre-approved
    lifecycle sync under grant, else the reason it is not."""
    if not grant.get("allow_pm_sync"):
        return "the grant does not pre-approve pm-sync (--allow-pm-sync)"
    granted = set(grant["scope"]["cards"])
    if rel == "product/backlog.md":
        old, new = parse_backlog(current), parse_backlog(proposed)
        if old is None or new is None:
            return "backlog.md does not parse as cards"
        if old[0] != new[0]:
            return "the text outside the cards changed"
        if set(old[1]) != set(new[1]):
            return "cards were added or removed"
        moved = 0
        for cid, (head, fields) in old[1].items():
            nhead, nfields = new[1][cid]
            if (head, fields) == (nhead, nfields):
                continue
            if cid not in granted:
                return "%s changed but is not in the grant's scope" % cid
            if head != nhead:
                return "%s: its title changed" % cid
            keys = set(fields) | set(nfields)
            for k in keys - {"Status", "Half shipped"}:
                if fields.get(k) != nfields.get(k):
                    return "%s: field %s changed; only Status and Half shipped may" % (cid, k)
            step = (fields.get("Status"), nfields.get("Status"))
            if step[0] != step[1]:
                if step not in PM_SYNC_STEPS:
                    return "%s: %s -> %s is not a pre-approved step" % (cid, step[0], step[1])
                moved += 1
        return None if moved or old[1] != new[1] else "nothing changed"
    if rel == "product/decisions.md":
        base = current.rstrip("\n")
        placeholder = "No decisions recorded yet."
        if base.endswith(placeholder):
            base = base[: -len(placeholder)].rstrip("\n")
        if not proposed.startswith(base + "\n") or base == proposed.rstrip("\n"):
            return "decisions.md may only be appended to"
        blocks = re.split(r"\n(?=### )", proposed[len(base):].strip("\n"))
        for b in blocks:
            lines = [ln for ln in b.split("\n") if ln.strip()]
            fields = dict(m.groups() for m in (FIELD_LINE.match(ln) for ln in lines[1:]) if m)
            if not DECISION_HEAD.match(lines[0]) or len(fields) != len(lines) - 1:
                return "an appended entry is not a well-formed decision"
            if fields.get("Type") != "lifecycle-sync":
                return "an appended decision is not of type lifecycle-sync"
            cards = {c.strip() for c in fields.get("Cards", "").split(",") if c.strip()}
            if not cards or not cards <= granted:
                return "an appended decision names cards outside the grant: %s" % ", ".join(sorted(cards - granted))
        return None
    if rel == "product/roadmap.md":
        pm = os.path.join(orchlib.main_checkout(root), "scripts", "pm", "pm.py")
        if not os.path.exists(pm):
            return "the roadmap generator (scripts/pm/pm.py) is not present"
        out = os.path.join(orchlib.data_dir(root), "autonomy-roadmap.tmp")
        r = subprocess.run([sys.executable or "python3", pm, "--root", orchlib.main_checkout(root), "roadmap",
                            "--out", out], capture_output=True, text=True)
        try:
            with open(out, encoding="utf-8") as fh:
                expected = fh.read()
            os.unlink(out)
        except OSError:
            return "the roadmap generator failed: %s" % (r.stderr.strip()[:200] or "no output")
        return None if expected == proposed else "roadmap.md is not the roadmap generated from the current backlog"
    return "%s is not a file the lifecycle sync writes" % rel


def cmd_pm_sync_check(a, root):
    grant = load(root)
    rel = os.path.normpath(a.path).replace(os.sep, "/")
    why = covers(grant, a.session, root)
    if why is None:
        try:
            with open(os.path.join(orchlib.main_checkout(root), rel), encoding="utf-8") as fh:
                current = fh.read()
            with open(a.proposed, encoding="utf-8") as fh:
                proposed = fh.read()
        except OSError as e:
            raise Refused("cannot read %s" % e.filename) from e
        why = pm_sync_problem(grant, rel, current, proposed, root)
    audit(root, "pm-sync-preapproved" if why is None else "pm-sync-refused", grant, path=rel,
          reason=why or "")
    print("pm-sync: %s" % ("pre-approved" if why is None else "not pre-approved: " + why))
    return 0 if why is None else 1


def parser():
    p = argparse.ArgumentParser(prog="autonomy.py", description="The autonomy grant for unattended delivery.")
    p.add_argument("--root", default=None, help=argparse.SUPPRESS)
    sub = p.add_subparsers(dest="cmd", required=True)
    g = sub.add_parser("grant")
    g.add_argument("--hours", required=True)
    g.add_argument("--scope", required=True)
    g.add_argument("--reason", required=True)
    g.add_argument("--session")
    g.add_argument("--allow-pm-sync", action="store_true")
    g.add_argument("--no-spec-checkpoint", action="store_true")
    g.add_argument("--max-continues", type=int)
    r = sub.add_parser("renew")
    r.add_argument("--hours", required=True)
    r.add_argument("--reason")
    v = sub.add_parser("revoke")
    v.add_argument("--reason")
    s = sub.add_parser("status")
    s.add_argument("--json", action="store_true")
    c = sub.add_parser("check")
    c.add_argument("--session")
    c.add_argument("--spec")
    c.add_argument("--card")
    m = sub.add_parser("merge-check")
    m.add_argument("--pr", required=True, type=int)
    m.add_argument("--spec", required=True)
    m.add_argument("--repo")
    m.add_argument("--session")
    kind = m.add_mutually_exclusive_group(required=True)
    kind.add_argument("--task", type=int)
    kind.add_argument("--lifecycle", action="store_true")
    kind.add_argument("--finalize", action="store_true")
    kind.add_argument("--fix", action="store_true")
    ps = sub.add_parser("pm-sync-check")
    ps.add_argument("--path", required=True)
    ps.add_argument("--proposed", required=True)
    ps.add_argument("--session")
    return p


VERBS = {"grant": cmd_grant, "renew": cmd_renew, "revoke": cmd_revoke, "status": cmd_status,
         "check": cmd_check, "merge-check": cmd_merge_check, "pm-sync-check": cmd_pm_sync_check}


def main(argv):
    a = parser().parse_args(argv)
    try:
        root = a.root or os.getcwd()
        orchlib.main_checkout(root)
        return VERBS[a.cmd](a, root)
    except Refused as e:
        print("autonomy.py %s: refused: %s" % (a.cmd, e), file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
