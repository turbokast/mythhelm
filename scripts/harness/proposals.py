#!/usr/bin/env python3
"""proposals.py: the apply side of the harness learning loop. /finalize-spec writes
proposals to .claude/proposals/pending.md (format: .claude/proposals/README.md); this
script lists them, applies the mechanical parts of a maintainer's decision, and checks
that an apply pull request does only what the decision allows.

    proposals.py list        [--root DIR] [--json]
                             pending proposals: id, type, target, age in days, lane-0
    proposals.py show        ID [--root DIR]      the proposal's section, verbatim
    proposals.py summary     [--root DIR]         "<count> <oldest age in days | unknown>"
    proposals.py notify      [--root DIR]         the SessionStart notice (JSON), or nothing
    proposals.py lane0       ID [--root DIR]      ELIGIBLE <glob> | NOT <reason>
    proposals.py append      ID [--root DIR]      lane 0: append the proposed change to the
                                                  target, nothing else of the proposal
    proposals.py record      ID --decision approved|rejected|auto-applied [--pr N]
                             [--rationale TEXT] [--eval CASE | --eval-waived REASON]
                             [--date YYYY-MM-DD] [--root DIR]
                             move the proposal from pending.md to the end of applied.md
                             with its decision; re-run with --pr to fill a pending number
    proposals.py apply-check ID [--base REF] [--root DIR]
                             the local tree applies exactly the recorded decision (scope,
                             append-only record, lane-0 eligibility at the base, and for an
                             approved rule, skill or hook an eval case that fails at the
                             base and passes here)
    proposals.py pr-check    ID --pr N [--repo O/R]
                             the apply pull request may be merged

lane0, apply-check and pr-check print key=value facts, one reason= line per finding and
a closing verdict= line; they exit 0 on a pass, 1 on a finding and 3 (pr-check) when
GitHub has not decided yet. Every other command exits 0 on success, 1 on a finding and
2 on a usage error or an unreadable input.

Nothing here merges, pushes or talks to GitHub except pr-check, which only reads.
Standard library only.
"""

import argparse
import base64
import binascii
import datetime
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile

HERE = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.normpath(os.path.join(HERE, "..", ".."))
sys.path.insert(0, HERE)
sys.path.insert(0, os.path.join(REPO, "scripts", "ci"))
sys.path.insert(0, os.path.join(REPO, ".claude", "evals"))
import finalize  # noqa: E402  (sibling scripts, not installed packages)
import gatelib  # noqa: E402
import harness_lint  # noqa: E402  (the one glob implementation)
import run_evals  # noqa: E402  (the eval runner)
import runspec  # noqa: E402
from runspec import Usage, gh  # noqa: E402

PENDING = ".claude/proposals/pending.md"
APPLIED = ".claude/proposals/applied.md"
AUTO_APPLY = ".claude/proposals/auto-apply.json"
CASES = ".claude/evals/cases/"
HEADING = re.compile(r"^## (P-([a-z0-9][a-z0-9-]*)-(\d+))(?: — (.+))?$")
FIELD = re.compile(r"^- \*\*([^*]+?)\*\*:\s*(.*)$")
TARGET = re.compile(r"^`([^`\s]+)`(\s+\(new file\))?")
TYPES = ("rule", "skill", "hook", "knowledge", "product")
EVAL_TYPES = ("rule", "skill", "hook")
DECISIONS = ("approved", "rejected", "auto-applied")
HOOK_COMPANIONS = (re.compile(r"^\.claude/settings\.json$"), re.compile(r"^\.claude/hooks/INVENTORY\.md$"),
                   re.compile(r"^\.claude/hooks/tests/test_[a-z0-9_]+\.sh$"))
APPLIED_HEAD = """# Applied proposals

Decided proposals, oldest first. `scripts/harness/proposals.py record` moves a proposal
here from `pending.md` with its decision, in the pull request that carries it. Append
only: never edit or remove an entry. The format is in [`README.md`](README.md).
"""
STALE_DAYS = 14


# ---- parsing -------------------------------------------------------------------

def sections(text):
    """(preamble lines, [section]). A section is one '## ' heading and the lines up to the
    next; headings inside code fences are content. Fields are the first '- **Name**:
    value' of each name outside fences."""
    pre, out, cur, fence = [], [], None, False
    for i, line in enumerate(text.split("\n"), 1):
        if line.lstrip().startswith("```"):
            fence = not fence
        if not fence and line.startswith("## "):
            m = HEADING.match(line)
            cur = {"line": i, "heading": line, "id": m.group(1) if m else None,
                   "spec": m.group(2) if m else None, "title": (m.group(4) or "") if m else "",
                   "lines": [line], "fields": {}}
            out.append(cur)
            continue
        if cur is None:
            pre.append(line)
            continue
        cur["lines"].append(line)
        fm = None if fence else FIELD.match(line)
        if fm:
            cur["fields"].setdefault(fm.group(1), fm.group(2).strip())
    return pre, out


def proposed_change(sec):
    """The body under '**Proposed change:**', trimmed, or ''."""
    lines = sec["lines"]
    at = next((k for k, l in enumerate(lines) if l.strip() == "**Proposed change:**"), None)
    if at is None:
        return ""
    return "\n".join(lines[at + 1:]).strip("\n")


def target(sec):
    """(path, is_new) from the Target field, or (None, False)."""
    m = TARGET.match(sec["fields"].get("Target", ""))
    return (m.group(1), bool(m.group(2))) if m else (None, False)


def ptype(sec):
    return sec["fields"].get("Type", "").strip("` ")


def read_optional(path):
    try:
        with open(path, encoding="utf-8") as fh:
            return fh.read()
    except FileNotFoundError:
        return None
    except (OSError, UnicodeDecodeError) as e:
        raise Usage("cannot read %s: %s" % (path, e)) from e


def load(root, rel):
    text = read_optional(os.path.join(root, rel))
    return text, sections(text or "")[1]


def find(secs, pid):
    hits = [s for s in secs if s["id"] == pid]
    if len(hits) > 1:
        raise Usage("%s appears %d times; the proposals lint reports the duplicate" % (pid, len(hits)))
    return hits[0] if hits else None


def pending_section(root, pid):
    _, secs = load(root, PENDING)
    sec = find(secs, pid)
    if sec is None:
        raise Usage("%s is not in %s" % (pid, PENDING))
    return sec


def write(path, text):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    tmp = "%s.%d.tmp" % (path, os.getpid())
    with open(tmp, "w", encoding="utf-8") as fh:
        fh.write(text)
    os.replace(tmp, path)


# ---- ages ----------------------------------------------------------------------

def heading_times(root, rel):
    """{line number: author time} from git blame; uncommitted lines carry the current
    time. {} when git cannot blame the file."""
    try:
        r = subprocess.run(["git", "-C", root, "blame", "--line-porcelain", "--", rel], capture_output=True,
                           text=True, env=runspec.git_env(), timeout=30)
    except (OSError, subprocess.TimeoutExpired):
        return {}
    if r.returncode != 0:
        return {}
    out, line_no, ts = {}, None, None
    for line in r.stdout.split("\n"):
        m = re.match(r"^[0-9a-f]{40} \d+ (\d+)", line)
        if m:
            line_no, ts = int(m.group(1)), None
        elif line.startswith("author-time "):
            ts = int(line.split()[1])
        elif line.startswith("\t") and line_no is not None and ts is not None:
            out[line_no] = ts
    return out


def pending_rows(root, now=None):
    now = now if now is not None else datetime.datetime.now(datetime.timezone.utc).timestamp()
    _, secs = load(root, PENDING)
    times = heading_times(root, PENDING) if secs else {}
    rows = []
    for s in secs:
        ts = times.get(s["line"])
        path, new = target(s)
        rows.append({"id": s["id"] or s["heading"], "title": s["title"], "type": ptype(s),
                     "target": path, "new_file": new, "source_spec": s["fields"].get("Source spec", "").strip("` "),
                     "age_days": None if ts is None else max(0, int((now - ts) // 86400)),
                     "lane0": lane0_decision(root, s)[0] == "ELIGIBLE"})
    return rows


def summary(root, now=None):
    rows = pending_rows(root, now)
    ages = [r["age_days"] for r in rows if r["age_days"] is not None]
    oldest = max(ages) if ages else None
    return len(rows), oldest, rows


# ---- lane 0 --------------------------------------------------------------------

def auto_apply_problems(config):
    """Every way the lane-0 configuration is malformed."""
    if not isinstance(config, dict):
        return ["the configuration is not a JSON object"]
    out = []
    if not isinstance(config.get("enabled"), bool):
        out.append("'enabled' is not true or false")
    allow = config.get("allow")
    if not isinstance(allow, list) or not all(isinstance(a, str) and a for a in allow):
        out.append("'allow' is not a list of globs")
    else:
        for a in allow:
            if not a.startswith("knowledge/") or ".." in a.split("/"):
                out.append("allow entry %r is not under knowledge/" % a)
    return out


def load_auto_apply(text):
    """(config, problems) from the file's text; a missing file is lane 0 off."""
    if text is None:
        return {"enabled": False, "allow": []}, []
    try:
        config = json.loads(text)
    except ValueError as e:
        return None, ["not JSON: %s" % e]
    return config, auto_apply_problems(config)


FROM_TREE = object()


def lane0_decision(root, sec, config_text=FROM_TREE, exists=None):
    """('ELIGIBLE', glob) or ('NOT', reason). config_text, when given, replaces the tree's
    configuration file (None: the file is absent, so lane 0 is off); exists, when given,
    replaces the check that the target is a file in root."""
    if config_text is FROM_TREE:
        config_text = read_optional(os.path.join(root, AUTO_APPLY))
    config, problems = load_auto_apply(config_text)
    if problems:
        return "NOT", "%s is malformed: %s" % (AUTO_APPLY, "; ".join(problems))
    if config.get("enabled") is not True:
        return "NOT", "lane 0 is off (%s enabled is not true)" % AUTO_APPLY
    if ptype(sec) != "knowledge":
        return "NOT", "type %r is not knowledge" % ptype(sec)
    path, new = target(sec)
    if path is None or new:
        return "NOT", "the target is not one existing file"
    if not path.startswith("knowledge/") or not path.endswith(".md"):
        return "NOT", "target %s is not a markdown file under knowledge/" % path
    if not (exists(path) if exists else os.path.isfile(os.path.join(root, path))):
        return "NOT", "target %s does not exist" % path
    body = proposed_change(sec)
    if not body:
        return "NOT", "the proposed change is empty"
    if any(HEADING.match(l) or FIELD.match(l) and FIELD.match(l).group(1) in ("Source spec", "Type", "Target")
           for l in body.split("\n")):
        return "NOT", "the proposed change carries proposal headings or fields, not the text to append"
    for glob in config.get("allow", []):
        if re.match(harness_lint.glob_regex(glob) + r"\Z", path):
            return "ELIGIBLE", glob
    return "NOT", "target %s matches no allow glob in %s" % (path, AUTO_APPLY)


def append_change(root, sec):
    path, _ = target(sec)
    full = os.path.join(root, path)
    text = read_optional(full) or ""
    body = proposed_change(sec).strip("\n")
    if body in text:
        raise Usage("%s already contains the proposed change" % path)
    write(full, text.rstrip("\n") + "\n\n" + body + "\n")
    return path


# ---- record --------------------------------------------------------------------

def remove_section(text, sec):
    lines = text.split("\n")
    start = sec["line"] - 1
    end = start + len(sec["lines"])
    head = "\n".join(lines[:start]).rstrip("\n")
    rest = "\n".join(lines[end:]).strip("\n")
    return head + "\n" + ("\n" + rest + "\n" if rest else "")


def entry_text(sec, decision, date, pr, eval_field, rationale):
    fields = ["- **Decision**: %s" % decision, "- **Date**: %s" % date,
              "- **Pull request**: %s" % ("#%d" % pr if pr else "pending"),
              "- **Eval**: %s" % eval_field, "- **Rationale**: %s" % rationale]
    body = "\n".join(sec["lines"][1:]).strip("\n")
    return "\n".join([sec["heading"], ""] + fields + [body]) + "\n"


def record(root, pid, decision, pr=None, rationale="", eval_case=None, eval_waived=None, date=None):
    """Moves the proposal to applied.md, or fills the pull request of a recorded one."""
    pending_text, psecs = load(root, PENDING)
    applied_text, asecs = load(root, APPLIED)
    done = find(asecs, pid)
    sec = find(psecs, pid)
    if done is not None and sec is None:
        if pr is None:
            raise Usage("%s is already recorded in %s" % (pid, APPLIED))
        if done["fields"].get("Pull request") != "pending":
            raise Usage("%s is recorded with pull request %s; an entry is never edited"
                        % (pid, done["fields"].get("Pull request")))
        lines = applied_text.split("\n")
        k = done["line"] - 1 + next(i for i, l in enumerate(done["lines"]) if l == "- **Pull request**: pending")
        lines[k] = "- **Pull request**: #%d" % pr
        write(os.path.join(root, APPLIED), "\n".join(lines))
        return "filled"
    if sec is None:
        raise Usage("%s is not in %s" % (pid, PENDING))
    if done is not None:
        raise Usage("%s is in both %s and %s" % (pid, PENDING, APPLIED))
    if decision not in DECISIONS:
        raise Usage("--decision is one of %s" % ", ".join(DECISIONS))
    if decision == "auto-applied":
        verdict, glob = lane0_decision(root, sec)
        if verdict != "ELIGIBLE":
            raise Usage("%s is not lane-0 eligible: %s" % (pid, glob))
        rationale = rationale or ("lane 0: target matches %s in %s; veto by reverting the pull request"
                                  % (glob, AUTO_APPLY))
    if not rationale.strip():
        raise Usage("--rationale is required: the maintainer's reason for the decision")
    if eval_case and eval_waived:
        raise Usage("--eval and --eval-waived are exclusive")
    if decision == "approved" and ptype(sec) in EVAL_TYPES:
        if eval_case:
            if not re.match(r"^[a-z0-9][a-z0-9-]*$", eval_case):
                raise Usage("--eval names a case id, not a path")
            eval_field = "`%s`" % eval_case
        elif eval_waived and eval_waived.strip():
            eval_field = "waived — %s" % eval_waived.strip()
        else:
            raise Usage("an approved %s proposal needs --eval CASE (a case that pins the fix) or "
                        "--eval-waived REASON from the maintainer" % ptype(sec))
    elif eval_case or eval_waived:
        raise Usage("an eval is recorded only for an approved rule, skill or hook proposal")
    else:
        eval_field = "n/a"
    date = date or datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d")
    if not re.match(r"^\d{4}-\d{2}-\d{2}$", date):
        raise Usage("--date is YYYY-MM-DD")
    entry = entry_text(sec, decision, date, pr, eval_field, rationale.strip())
    base = applied_text if applied_text is not None else APPLIED_HEAD
    write(os.path.join(root, APPLIED), base.rstrip("\n") + "\n\n" + entry)
    write(os.path.join(root, PENDING), remove_section(pending_text, sec))
    return "recorded"


def applied_problems(text, pending_ids=(), case_ids=None):
    """[(line, detail)] for applied.md: headings, unique ids, the decision fields, and no id
    still pending. case_ids, when given, is the set of eval case ids that exist."""
    out, seen = [], {}
    for s in sections(text)[1]:
        i, pid, f = s["line"], s["id"], s["fields"]
        if pid is None:
            out.append((i, "heading %r is not '## P-<spec>-<n> — <title>'" % s["heading"]))
            continue
        if pid in seen:
            out.append((i, "%s is recorded twice (first at line %d)" % (pid, seen[pid])))
        seen.setdefault(pid, i)
        if pid in pending_ids:
            out.append((i, "%s is both pending and recorded" % pid))
        decision = f.get("Decision", "")
        if decision not in DECISIONS:
            out.append((i, "%s: Decision %r is not one of %s" % (pid, decision, ", ".join(DECISIONS))))
        if not re.match(r"^\d{4}-\d{2}-\d{2}$", f.get("Date", "")):
            out.append((i, "%s: Date is not YYYY-MM-DD" % pid))
        if not re.match(r"^(#\d+|pending)$", f.get("Pull request", "")):
            out.append((i, "%s: Pull request is not '#<n>'" % pid))
        if not f.get("Rationale"):
            out.append((i, "%s has no Rationale" % pid))
        ev = f.get("Eval", "")
        if decision == "approved" and ptype(s) in EVAL_TYPES:
            m = re.match(r"^`([a-z0-9][a-z0-9-]*)`$", ev)
            if not (m or re.match(r"^waived — \S", ev)):
                out.append((i, "%s: an approved %s proposal records an eval case or a waiver" % (pid, ptype(s))))
            elif m and case_ids is not None and m.group(1) not in case_ids:
                out.append((i, "%s: eval case %s has no file %s%s.json" % (pid, m.group(1), CASES, m.group(1))))
        elif ev and ev != "n/a":
            out.append((i, "%s: Eval is 'n/a' unless the proposal is an approved rule, skill or hook" % pid))
    return out


# ---- apply-check ---------------------------------------------------------------

def git(root, *args, check=True):
    return runspec.run(["git", "-C", root] + list(args), check=check)


def show(root, ref, rel):
    r = git(root, "show", "%s:%s" % (ref, rel), check=False)
    return r.stdout if r.returncode == 0 else None


def allowed_files(sec, decision):
    """A predicate for the files an apply of this decision may change: the proposal files,
    and for an applied change its target, eval cases, a rule's evidence file and a hook's
    registration, inventory and tests."""
    own = {PENDING, APPLIED}
    if decision == "rejected":
        return lambda f: f in own
    path, _ = target(sec)
    kind = ptype(sec)
    evidence = None
    if kind == "rule" and path and path.startswith(".claude/rules/"):
        evidence = "knowledge/rule-evidence/" + os.path.basename(path)
    return lambda f: (f in own or f == path or f.startswith(CASES) or f == evidence
                      or (kind == "hook" and any(p.match(f) for p in HOOK_COMPANIONS)))


def run_eval(tree, case_path):
    """(passed, detail) of one case file run against a tree."""
    res = run_evals.run_all(tree, [case_path])[0]
    return res["passed"], res["detail"]


def run_eval_at(root, ref, case_path):
    """(passed, detail) of the case file run against a temporary worktree of ref."""
    tmp = tempfile.mkdtemp(prefix="proposal-base-")
    tree = os.path.join(tmp, "tree")
    try:
        git(root, "worktree", "add", "--detach", "-q", tree, ref)
        try:
            return run_eval(tree, case_path)
        finally:
            git(root, "worktree", "remove", "--force", tree, check=False)
    finally:
        shutil.rmtree(tmp, ignore_errors=True)


def apply_check(root, pid, base="origin/main"):
    facts, reasons = [("proposal", pid)], []
    merge_base = git(root, "merge-base", "HEAD", base, check=False).stdout.strip()
    if not merge_base:
        raise Usage("no merge base between HEAD and %s" % base)
    facts.append(("base", merge_base[:12]))
    pend_base = sections(show(root, merge_base, PENDING) or "")[1]
    sec = find(pend_base, pid)
    if sec is None:
        raise Usage("%s is not pending at the base %s" % (pid, merge_base[:12]))
    applied_head, asecs = load(root, APPLIED)
    _, psecs = load(root, PENDING)
    entry = find(asecs, pid)
    if find(psecs, pid) is not None:
        reasons.append("entry: %s is still in %s" % (pid, PENDING))
    if entry is None:
        reasons.append("entry: %s is not recorded in %s (proposals.py record)" % (pid, APPLIED))
        return finalize.verdict(facts, reasons, [], "ready")
    decision = entry["fields"].get("Decision", "")
    facts.append(("decision", decision))
    applied_base = show(root, merge_base, APPLIED) or ""
    if applied_base.strip() and not (applied_head or "").startswith(applied_base.rstrip("\n")):
        reasons.append("append-only: %s changes existing entries; only appending is allowed" % APPLIED)
    case_ids = {n[:-5] for n in os.listdir(os.path.join(root, CASES))} if os.path.isdir(os.path.join(root, CASES)) else set()
    for line, detail in applied_problems(applied_head or "", {s["id"] for s in psecs}, case_ids):
        if detail.startswith(pid + ":") or detail.startswith(pid + " "):
            reasons.append("entry: %s" % detail)
    ok = allowed_files(sec, decision)
    changed = gatelib.changed_files(root, merge_base)
    facts.append(("changed", len(changed)))
    for f in changed:
        if not ok(f):
            reasons.append("scope: %s is outside %s's target, its eval case and the proposal files" % (f, pid))
    path, _ = target(sec)
    if decision in ("approved", "auto-applied") and path not in changed:
        reasons.append("scope: the target %s is unchanged" % path)
    if decision == "auto-applied":
        verdict, why = lane0_decision(root, sec, config_text=show(root, merge_base, AUTO_APPLY))
        if verdict != "ELIGIBLE":
            reasons.append("lane0: %s at the base: %s" % (pid, why))
        before, after = show(root, merge_base, path) or "", read_optional(os.path.join(root, path)) or ""
        if not after.startswith(before.rstrip("\n")):
            reasons.append("lane0: %s is not a pure append to %s" % (pid, path))
    ev = entry["fields"].get("Eval", "")
    m = re.match(r"^`([a-z0-9][a-z0-9-]*)`$", ev)
    if decision == "approved" and ptype(sec) in EVAL_TYPES and m:
        case_path = os.path.join(root, CASES, m.group(1) + ".json")
        facts.append(("eval", m.group(1)))
        if not os.path.isfile(case_path):
            reasons.append("eval: %s%s.json does not exist" % (CASES, m.group(1)))
        else:
            with open(case_path, encoding="utf-8") as fh:
                try:
                    source = json.load(fh).get("source", "")
                except (ValueError, AttributeError):
                    source = ""
            if pid not in str(source):
                reasons.append("eval: the case's source does not name %s" % pid)
            passed, detail = run_eval(root, case_path)
            if not passed:
                reasons.append("eval: %s fails on this tree: %s" % (m.group(1), detail))
            passed_base, detail_base = run_eval_at(root, merge_base, case_path)
            facts.append(("eval_at_base", "pass" if passed_base else "fail"))
            if passed_base:
                reasons.append("eval: %s already passes at the base, so it does not pin this change" % m.group(1))
    elif decision == "approved" and ptype(sec) in EVAL_TYPES:
        facts.append(("eval", "waived"))
    return finalize.verdict(facts, reasons, [], "ready")



# ---- pr-check ------------------------------------------------------------------

def file_at(slug, ref, rel):
    """The file's text at ref, or None when GitHub has no such file."""
    try:
        raw = gh(["api", "repos/%s/contents/%s?ref=%s" % (slug, rel, ref)])
    except Usage:
        return None
    try:
        return base64.b64decode(json.loads(raw)["content"]).decode("utf-8")
    except (ValueError, KeyError, TypeError, binascii.Error) as e:
        raise Usage("unreadable contents response for %s at %s: %s" % (rel, ref, e)) from e


def pr_check(pid, pr, repo=None, required=finalize.PR_REVIEW_CHECKS):
    slug = runspec.repo_slug(repo)
    view, reasons, unknown = finalize.pr_gate(slug, pr, required)
    head = view.get("headRefOid", "")
    facts = [("proposal", pid), ("pr", pr), ("head", head)]
    applied = file_at(slug, head, APPLIED)
    entry = find(sections(applied or "")[1], pid)
    if entry is None:
        reasons.append("entry: %s is not recorded in %s at the head" % (pid, APPLIED))
        return finalize.verdict(facts, reasons, unknown, "ready")
    decision = entry["fields"].get("Decision", "")
    facts.append(("decision", decision))
    base = view.get("baseRefOid", "")
    sec = find(sections(file_at(slug, base, PENDING) or "")[1], pid) if base else None
    if sec is None:
        reasons.append("entry: %s is not pending at the base %s; the scope comes from the base's proposal"
                       % (pid, base[:12] or "(unknown)"))
        return finalize.verdict(facts, reasons, unknown, "ready")
    if decision == "auto-applied":
        verdict, why = lane0_decision(None, sec, config_text=file_at(slug, base, AUTO_APPLY),
                                      exists=lambda p: file_at(slug, base, p) is not None)
        if verdict != "ELIGIBLE":
            reasons.append("lane0: %s at the base: %s" % (pid, why))
    if entry["fields"].get("Pull request") != "#%d" % pr:
        reasons.append("entry: %s records pull request %s, not #%d (proposals.py record %s --pr %d)"
                       % (pid, entry["fields"].get("Pull request"), pr, pid, pr))
    if find(sections(file_at(slug, head, PENDING) or "")[1], pid) is not None:
        reasons.append("entry: %s is still in %s at the head" % (pid, PENDING))
    ok = allowed_files(sec, decision)
    for f in (x["path"] for x in view.get("files") or []):
        if not ok(f):
            reasons.append("scope: %s is outside %s's target, its eval case and the proposal files" % (f, pid))
    return finalize.verdict(facts, reasons, unknown, "ready")


# ---- notify --------------------------------------------------------------------

def notify(root):
    """The one-line SessionStart notice as a JSON object, or None with nothing pending."""
    count, oldest, rows = summary(root)
    if not count:
        return None
    age = "oldest %d day%s" % (oldest, "" if oldest == 1 else "s") if oldest is not None else "age unknown"
    msg = "Harness proposals: %d pending (%s). Decide them with /apply-proposals." % (count, age)
    return {"systemMessage": msg,
            "hookSpecificOutput": {"hookEventName": "SessionStart", "additionalContext": msg}}


# ---- CLI -----------------------------------------------------------------------

def main(argv=None):
    ap = argparse.ArgumentParser(prog="proposals.py", description="The apply side of the harness learning loop.")
    sub = ap.add_subparsers(dest="cmd", required=True)
    p = sub.add_parser("list"); p.add_argument("--root", default="."); p.add_argument("--json", action="store_true")
    for name in ("show", "lane0", "append"):
        p = sub.add_parser(name); p.add_argument("id"); p.add_argument("--root", default=".")
    for name in ("summary", "notify"):
        p = sub.add_parser(name); p.add_argument("--root", default=".")
    p = sub.add_parser("record"); p.add_argument("id"); p.add_argument("--root", default=".")
    p.add_argument("--decision", choices=DECISIONS); p.add_argument("--pr", type=int)
    p.add_argument("--rationale", default=""); p.add_argument("--eval"); p.add_argument("--eval-waived")
    p.add_argument("--date")
    p = sub.add_parser("apply-check"); p.add_argument("id"); p.add_argument("--root", default=".")
    p.add_argument("--base", default="origin/main")
    p = sub.add_parser("pr-check"); p.add_argument("id"); p.add_argument("--pr", type=int, required=True)
    p.add_argument("--repo")
    try:
        a = ap.parse_args(argv)
    except SystemExit as e:
        return 2 if e.code else 0
    try:
        root = os.path.abspath(getattr(a, "root", "."))
        if a.cmd == "list":
            rows = pending_rows(root)
            if a.json:
                print(json.dumps(rows, indent=1))
                return 0
            for r in rows:
                age = "?" if r["age_days"] is None else "%dd" % r["age_days"]
                print("%-28s %-9s %5s  %s%s  %s" % (r["id"], r["type"] or "?", age, r["target"] or "?",
                                                    " (new)" if r["new_file"] else "",
                                                    "lane0" if r["lane0"] else ""))
            print("pending=%d" % len(rows))
            return 0
        if a.cmd == "show":
            print("\n".join(pending_section(root, a.id)["lines"]).rstrip("\n"))
            return 0
        if a.cmd == "summary":
            count, oldest, _ = summary(root)
            print("%d %s" % (count, "unknown" if oldest is None else oldest))
            return 0
        if a.cmd == "notify":
            out = notify(root)
            if out:
                print(json.dumps(out))
            return 0
        if a.cmd == "lane0":
            verdict, why = lane0_decision(root, pending_section(root, a.id))
            print("%s %s" % (verdict, why))
            return 0 if verdict == "ELIGIBLE" else 1
        if a.cmd == "append":
            sec = pending_section(root, a.id)
            verdict, why = lane0_decision(root, sec)
            if verdict != "ELIGIBLE":
                print("NOT %s" % why)
                return 1
            print("appended %s" % append_change(root, sec))
            return 0
        if a.cmd == "record":
            if a.pr is None and a.decision is None:
                raise Usage("record needs --decision (or --pr to fill a recorded entry's pull request)")
            print("%s %s" % (record(root, a.id, a.decision, a.pr, a.rationale, a.eval, a.eval_waived, a.date), a.id))
            return 0
        if a.cmd == "apply-check":
            return apply_check(root, a.id, a.base)
        if a.cmd == "pr-check":
            return pr_check(a.id, a.pr, a.repo)
    except Usage as e:
        sys.stderr.write("proposals.py: %s\n" % e)
        return 2
    return 2


if __name__ == "__main__":
    sys.exit(main())
