#!/usr/bin/env python3
"""runspec.py: the mechanical half of running a spec, so that an orchestrator decides
from parsed facts and GitHub state, never from an agent's prose.

    runspec.py tasks        <tasks.md>                      parsed task blocks (JSON)
    runspec.py status       <tasks.md> [--in-flight N,M]    progress, ready set, deadlock
    runspec.py batch        <tasks.md> [--in-flight N,M] [--max 4]
                                                            ready tasks with disjoint Files
    runspec.py closure      <tasks.md> <N>                  transitive dependencies of N
    runspec.py entry-check  <tasks.md> <N> [--pr P]         completion entry well-formed
    runspec.py handoff-seed <spec-dir>                      create or extend handoff.md
    runspec.py handoff      <spec-dir> <N>                  hand-off notes of N's dependencies
    runspec.py report       [--file F]                      parse a task-report block
    runspec.py deps-merged  <spec-name> <N> [--ref origin/main] [--no-fetch]
    runspec.py pr-check     --spec <name> (--task <N> | --lifecycle) --pr <P> [--repo O/R]
                            [--require-check NAME]... [--accept-scope PATH]...
    runspec.py verify-merged --spec <name> --task <N> --pr <P> [--repo O/R] [--no-fetch]
    runspec.py event        --spec <name> --kind <kind> [--task N] [--attempt K]
                            [--agent A] [--pr P] [--result ok|fail|unknown] [--detail TEXT]
    runspec.py summary      --spec <name> [--json]

Task blocks follow knowledge/spec-authoring.md: a '### Task N — <name>' heading and
'- **Field**: value' bullets. A task is complete when its heading ends with
'✅ COMPLETED' or its Status field starts with '✅'; 'status' reports a task that
carries only one of the two as a marker mismatch to backfill. A task with no
'Depends on' field depends on the task listed before it (the first task on none).

pr-check and verify-merged read GitHub through the gh CLI and git; they never
merge, push or edit. pr-check exits 0 when the pull request may be merged, 1 when
it may not (each reason on a 'reason=' line), and 3 when GitHub has not decided
yet (merge state UNKNOWN, a check still pending): poll again, never treat 3 as a
pass. The other commands exit 0 on success, 1 on a finding, 2 on a usage error or
an unreadable input. 'event' appends one row to <main checkout>/.claude/data/
run-events.jsonl (schema: .claude/data/run-events.schema.json); nothing else writes.

Standard library only. The gh and git binaries are looked up on PATH, so tests put
stand-ins first on PATH.
"""

import argparse
import base64
import datetime
import fnmatch
import json
import os
import re
import subprocess
import sys
import tempfile

HERE = os.path.dirname(os.path.abspath(__file__))
SPEC_STATES = ("in-progress", "unfinalized", "todo", "refined", "unrefined", "done", "archived")
TASK_HEADING = re.compile(r"^###\s+Task\s+(\d+)\b\s*(?:[—–:-]\s*)?(.*?)\s*$")
FIELD = re.compile(r"^- \*\*([^*]+?)\*\*(?:\s*\([^)]*\))?\s*:\s*(.*)$")
COMPLETE_MARK = "✅ COMPLETED"
CLAIM_MARK = "🔄 IN PROGRESS"
PR_REF = re.compile(r"\bPR #(\d+)\b")
ENTRY_FIELDS = ("Status", "Implementation", "Spec deviations", "Files modified")
REPORT_KEYS = {
    "task": int, "status": str, "pr": (int, type(None)), "branch": str,
    "first_pass": bool, "deviations": str, "files_modified": list,
    "gates": dict, "budget_overrun": bool, "notes": str,
}
REPORT_STATUSES = ("pr_open", "blocked")
EVENT_KINDS = ("run_start", "lifecycle", "dispatch", "return", "verify", "review_round",
               "pr_ready", "merge", "retry", "fail", "lost", "blocked", "deadlock", "override",
               "run_end")
DEFAULT_REQUIRED_CHECKS = ("CI OK", "CodeRabbit")
PENDING_PLACEHOLDER = "<!-- pending -->"


class Usage(Exception):
    """A usage or unreadable-input error: exit 2."""


def git_env():
    env = dict(os.environ)
    for k in ("GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR"):
        env.pop(k, None)
    return env


def run(cmd, cwd=None, check=True, input_text=None):
    try:
        r = subprocess.run(cmd, cwd=cwd, capture_output=True, text=True, env=git_env(),
                           input=input_text, timeout=300)
    except (OSError, subprocess.TimeoutExpired) as e:
        raise Usage("cannot run %s: %s" % (cmd[0], e)) from e
    if check and r.returncode != 0:
        raise Usage("%s failed (exit %d): %s" % (" ".join(cmd[:4]), r.returncode, r.stderr.strip()[:300]))
    return r


def toplevel(start="."):
    return run(["git", "-C", start, "rev-parse", "--show-toplevel"]).stdout.strip()


def main_data_dir(start="."):
    """<main checkout>/.claude/data, shared by every linked worktree of the repository."""
    common = run(["git", "-C", start, "rev-parse", "--path-format=absolute", "--git-common-dir"]).stdout.strip()
    return os.path.join(os.path.dirname(os.path.realpath(common)), ".claude", "data")


# ---- tasks.md ----------------------------------------------------------------

def strip_notes(text):
    """Removes parenthesised notes, innermost first."""
    prev = None
    while prev != text:
        prev, text = text, re.sub(r"\([^()]*\)", "", text)
    return text


def depends_refs(value):
    """Task numbers named outside parenthesised notes; [] for None; None when unreadable."""
    head = strip_notes(value).strip().rstrip(".")
    if "(" in head or ")" in head:
        return None
    if re.fullmatch(r"(?i)none", head):
        return []
    refs = []
    for p in (p.strip() for p in re.split(r",|\band\b", head) if p.strip()):
        m = re.fullmatch(r"Tasks?\s+(\d+)", p)
        if not m:
            return None
        refs.append(int(m.group(1)))
    return refs or None


def file_patterns(value):
    """Backticked paths in a Files value, outside parenthesised notes."""
    out = []
    for tok in re.findall(r"`([^`\s]+)`", strip_notes(value)):
        if "/" in tok or "." in tok:
            out.append(tok.rstrip(","))
    return out


def parse_tasks(text):
    """[{number, name, line, fields{name: text}, ...}] for every task block outside fences."""
    tasks, cur, field, fence = [], None, None, False
    for i, line in enumerate(text.split("\n"), 1):
        if line.lstrip().startswith("```"):
            fence = not fence
            continue
        if fence:
            continue
        m = TASK_HEADING.match(line)
        if m:
            cur = {"number": int(m.group(1)), "heading": line, "line": i, "fields": {}}
            tasks.append(cur)
            field = None
            continue
        if line.startswith(("## ", "### ")) or line.strip() == "---":
            cur, field = None, None
            continue
        if cur is None:
            continue
        fm = FIELD.match(line)
        if fm:
            field = fm.group(1).strip()
            if field == "Acceptance criteria":
                field = "Acceptance"
            cur["fields"].setdefault(field, fm.group(2).strip())
            continue
        if field and line.startswith(("  ", "\t")) and line.strip():
            cur["fields"][field] = (cur["fields"][field] + "\n" + line.strip()).strip()
        elif not line.strip():
            continue
        else:
            field = None
    for idx, t in enumerate(tasks):
        f = t["fields"]
        heading = t["heading"]
        name = TASK_HEADING.match(heading).group(2)
        for mark in (COMPLETE_MARK, CLAIM_MARK):
            name = name.replace(mark, "")
        t["name"] = name.strip()
        t["heading_complete"] = COMPLETE_MARK in heading
        t["status_complete"] = f.get("Status", "").startswith("✅")
        t["complete"] = t["heading_complete"] or t["status_complete"]
        t["blocked"] = "Blocked" in f
        t["agent"] = (re.sub(r"[`*]", "", f.get("Domain/agent", "")).split() or [""])[0].rstrip(",;")
        t["budget"] = (re.sub(r"[`*]", "", f.get("Budget", "")).split() or ["standard"])[0].rstrip(",;")
        if "Depends on" in f:
            refs = depends_refs(f["Depends on"])
            t["depends"] = refs if refs is not None else []
            t["depends_unreadable"] = refs is None
        else:
            t["depends"] = [tasks[idx - 1]["number"]] if idx > 0 else []
            t["depends_unreadable"] = False
        t["files"] = file_patterns(f.get("Files", ""))
        pr = PR_REF.search(f.get("Status", ""))
        t["pr"] = int(pr.group(1)) if pr else None
        del t["heading"]
    return tasks


def load_tasks(path):
    try:
        with open(path, encoding="utf-8") as fh:
            tasks = parse_tasks(fh.read())
    except OSError as e:
        raise Usage("cannot read %s: %s" % (path, e)) from e
    if not tasks:
        raise Usage("%s has no '### Task N' blocks" % path)
    return tasks


def by_number(tasks):
    return {t["number"]: t for t in tasks}


def closure(tasks, n):
    idx = by_number(tasks)
    if n not in idx:
        raise Usage("task %d does not exist" % n)
    seen, stack = set(), list(idx[n]["depends"])
    while stack:
        d = stack.pop()
        if d in seen or d not in idx:
            continue
        seen.add(d)
        stack.extend(idx[d]["depends"])
    return sorted(seen)


def has_cycle(tasks):
    idx = by_number(tasks)
    state = {}

    def visit(n):
        state[n] = 1
        for d in idx[n]["depends"]:
            if d not in idx:
                continue
            if state.get(d) == 1 or (not state.get(d) and visit(d)):
                return True
        state[n] = 2
        return False
    return any(not state.get(n) and visit(n) for n in idx)


def progress(tasks, in_flight=()):
    idx = by_number(tasks)
    done = {n for n, t in idx.items() if t["complete"]}
    ready, waiting, blocked, missing = [], [], [], []
    for n, t in sorted(idx.items()):
        if n in done:
            continue
        if t["blocked"]:
            blocked.append(n)
            continue
        unknown = [d for d in t["depends"] if d not in idx]
        if unknown or t["depends_unreadable"]:
            missing.append(n)
            continue
        if all(d in done for d in t["depends"]):
            if n not in in_flight:
                ready.append(n)
        else:
            waiting.append(n)
    incomplete = sorted(set(idx) - done)
    return {
        "total": len(idx),
        "complete": sorted(done),
        "incomplete": incomplete,
        "in_flight": sorted(set(in_flight) & set(incomplete)),
        "ready": ready,
        "waiting": waiting,
        "blocked": blocked,
        "unresolvable": missing,
        "cycle": has_cycle(tasks),
        "deadlock": bool(incomplete) and not ready and not (set(in_flight) & set(incomplete)),
        "marker_mismatch": sorted(n for n, t in idx.items() if t["heading_complete"] != t["status_complete"]),
    }


def patterns_overlap(a, b):
    """True when two Files patterns can name the same path."""
    a, b = a.rstrip("/"), b.rstrip("/")
    if a == b or a.startswith(b + "/") or b.startswith(a + "/"):
        return True
    return fnmatch.fnmatchcase(a, b) or fnmatch.fnmatchcase(b, a)


def overlap(fa, fb):
    for a in fa:
        for b in fb:
            if patterns_overlap(a, b):
                return a if a == b else "%s ~ %s" % (a, b)
    return None


def batch(tasks, in_flight=(), limit=4):
    """Ready tasks, in task order, whose Files are disjoint from each other and from the
    in-flight tasks'. A ready task with no Files list runs alone."""
    idx = by_number(tasks)
    p = progress(tasks, in_flight)
    chosen, deferred = [], []
    busy = [(n, idx[n]["files"]) for n in p["in_flight"]]
    for n in p["ready"]:
        files = idx[n]["files"]
        if not files:
            if not chosen and not busy:
                chosen.append(n)
                break
            deferred.append({"task": n, "reason": "no Files list; runs alone"})
            continue
        clash = next(((m, overlap(files, fm)) for m, fm in busy if not fm or overlap(files, fm)), None)
        if clash:
            deferred.append({"task": n, "reason": "shares %s with task %d" % (clash[1] or "an unlisted file set", clash[0])})
            continue
        if len(chosen) >= limit:
            deferred.append({"task": n, "reason": "batch limit %d" % limit})
            continue
        chosen.append(n)
        busy.append((n, files))
    return {"batch": chosen, "deferred": deferred, "deadlock": p["deadlock"]}


def field_problems(task, pr=None):
    """Problems with a task's completion entry; [] when it is well-formed."""
    f = task["fields"]
    out = []
    if not task["heading_complete"]:
        out.append("heading does not end with '%s'" % COMPLETE_MARK)
    for name in ENTRY_FIELDS:
        if not f.get(name, "").strip():
            out.append("no '- **%s**:' field" % name)
    status = f.get("Status", "")
    if status and not status.startswith("✅ Completed"):
        out.append("Status does not start with '✅ Completed'")
    if status and not PR_REF.search(status):
        out.append("Status names no 'PR #<n>'")
    elif pr is not None and status and int(PR_REF.search(status).group(1)) != pr:
        out.append("Status names PR #%s, not PR #%d" % (PR_REF.search(status).group(1), pr))
    files = f.get("Files modified", "")
    if files and not re.search(r"`[^`]+`", files):
        out.append("Files modified lists no backticked path")
    return out


# ---- handoff.md --------------------------------------------------------------

def handoff_sections(text):
    """{task number: body} from '## Task N — ...' sections."""
    out, cur, buf = {}, None, []
    for line in text.split("\n"):
        m = re.match(r"^##\s+Task\s+(\d+)\b", line)
        if m or line.startswith("# "):
            if cur is not None:
                out[cur] = "\n".join(buf).strip()
            cur, buf = (int(m.group(1)) if m else None), []
            continue
        if cur is not None:
            buf.append(line)
    if cur is not None:
        out[cur] = "\n".join(buf).strip()
    return out


def scratchpad_entries(text):
    """{task number: [entry bodies]} from '### Task N — ...' Discoveries entries."""
    out, cur, buf = {}, None, []
    for line in text.split("\n") + ["## end"]:
        m = re.match(r"^###\s+Task\s+(\d+)\b", line)
        if m or line.startswith(("## ", "### ")):
            if cur is not None:
                out.setdefault(cur, []).append("\n".join(buf).strip())
            cur, buf = (int(m.group(1)) if m else None), []
            continue
        if cur is not None:
            buf.append(line)
    return out


def handoff_seed(spec_dir):
    tasks = load_tasks(os.path.join(spec_dir, "tasks.md"))
    path = os.path.join(spec_dir, "handoff.md")
    if os.path.exists(path):
        with open(path, encoding="utf-8") as fh:
            text = fh.read()
    else:
        title = os.path.basename(os.path.normpath(spec_dir))
        text = ("# %s — Hand-off\n\n"
                "> One section per task. Each task replaces its own `%s` line in its pull request with what\n"
                "> dependent tasks need: what it produced (the API and files as shipped), what a later task must\n"
                "> know, and any deviation that changes a later task's inputs. `/run-spec` puts the sections of a\n"
                "> task's dependencies into that task's dispatch prompt. Open questions and research stay in\n"
                "> `scratchpad.md`.\n" % (title, PENDING_PLACEHOLDER))
    have = handoff_sections(text)
    added = []
    for t in tasks:
        if t["number"] in have:
            continue
        text = text.rstrip("\n") + "\n\n## Task %d — %s\n\n%s\n" % (t["number"], t["name"], PENDING_PLACEHOLDER)
        added.append(t["number"])
    with open(path, "w", encoding="utf-8") as fh:
        fh.write(text)
    return path, added


def handoff_for(spec_dir, n):
    tasks = load_tasks(os.path.join(spec_dir, "tasks.md"))
    idx = by_number(tasks)
    deps = closure(tasks, n)

    def read(name):
        try:
            with open(os.path.join(spec_dir, name), encoding="utf-8") as fh:
                return fh.read()
        except OSError:
            return ""
    sections = handoff_sections(read("handoff.md"))
    scratch = scratchpad_entries(read("scratchpad.md"))
    out = []
    for d in deps:
        body = sections.get(d, "")
        source = "handoff.md"
        if not body or body == PENDING_PLACEHOLDER:
            body = "\n\n".join(scratch.get(d, []))
            source = "scratchpad.md"
        if not body:
            body, source = "(no hand-off recorded)", "none"
        out.append("### Task %d — %s (from %s)\n\n%s" % (d, idx[d]["name"], source, body))
    return deps, "\n\n".join(out)


# ---- task-report -------------------------------------------------------------

def parse_report(text):
    """(report, None) or (None, 'task_report_missing' | 'task_report_malformed: <why>')."""
    blocks = re.findall(r"```task-report[ \t]*\n(.*?)\n[ \t]*```", text, re.S)
    if not blocks:
        return None, "task_report_missing"
    body = blocks[-1].strip()
    if "\n" in body:
        return None, "task_report_malformed: the body is not one line"
    try:
        rep = json.loads(body)
    except ValueError as e:
        return None, "task_report_malformed: %s" % e
    if not isinstance(rep, dict):
        return None, "task_report_malformed: not a JSON object"
    for key, typ in REPORT_KEYS.items():
        if key not in rep:
            return None, "task_report_malformed: no %r" % key
        if not isinstance(rep[key], typ) or (typ is int and isinstance(rep[key], bool)):
            return None, "task_report_malformed: %r has the wrong type" % key
    if rep["status"] not in REPORT_STATUSES:
        return None, "task_report_malformed: status %r is not one of %s" % (rep["status"], "/".join(REPORT_STATUSES))
    if rep["status"] == "pr_open" and rep["pr"] is None:
        return None, "task_report_malformed: status pr_open needs a pr number"
    return rep, None


# ---- git and GitHub ----------------------------------------------------------

def spec_tasks_path_at(ref, name, cwd="."):
    out = run(["git", "-C", cwd, "ls-tree", "-r", "--name-only", ref, "--", "specs/"]).stdout
    hits = [p for p in out.split("\n") if re.fullmatch(r"specs/[^/]+/%s/tasks\.md" % re.escape(name), p)]
    if len(hits) != 1:
        raise Usage("%s holds %d copies of specs/*/%s/tasks.md" % (ref, len(hits), name))
    return hits[0]


def tasks_at(ref, name, cwd="."):
    path = spec_tasks_path_at(ref, name, cwd)
    return path, parse_tasks(run(["git", "-C", cwd, "show", "%s:%s" % (ref, path)]).stdout)


def gh(args, repo=None):
    cmd = ["gh"] + args
    if repo and args and args[0] in ("pr",):
        cmd += ["-R", repo]
    r = run(cmd, check=False)
    if r.returncode != 0:
        raise Usage("gh %s failed (exit %d): %s" % (" ".join(args[:3]), r.returncode, r.stderr.strip()[:300]))
    return r.stdout


def repo_slug(repo):
    if repo:
        return repo
    return json.loads(gh(["repo", "view", "--json", "nameWithOwner"]))["nameWithOwner"]


def latest_checks(rollup):
    """{name: outcome} keeping only the most recent run of each check or status."""
    latest = {}
    for c in rollup or []:
        name = c.get("name") or c.get("context") or "?"
        key = (name, c.get("workflowName") or "")
        when = c.get("startedAt") or c.get("completedAt") or ""
        if key not in latest or when >= latest[key][0]:
            if c.get("__typename") == "StatusContext" or ("context" in c and "conclusion" not in c):
                outcome = {"SUCCESS": "pass", "PENDING": "pending", "EXPECTED": "pending"}.get(c.get("state"), "fail")
            elif c.get("status") != "COMPLETED":
                outcome = "pending"
            else:
                outcome = {"SUCCESS": "pass", "NEUTRAL": "pass", "SKIPPED": "skip"}.get(c.get("conclusion"), "fail")
            latest[key] = (when, outcome)
    out = {}
    for (name, _), (_, outcome) in latest.items():
        rank = {"fail": 3, "pending": 2, "pass": 1, "skip": 0}
        if rank[outcome] >= rank.get(out.get(name), -1):
            out[name] = outcome
    return out


THREADS_QUERY = """query($owner:String!,$name:String!,$number:Int!){repository(owner:$owner,name:$name){
pullRequest(number:$number){reviewThreads(first:100){totalCount nodes{isResolved path comments(first:1){nodes{url}}}}}}}"""


def unresolved_threads(slug, pr):
    owner, name = slug.split("/", 1)
    data = json.loads(gh(["api", "graphql", "-f", "query=" + THREADS_QUERY, "-F", "owner=" + owner,
                          "-F", "name=" + name, "-F", "number=%d" % pr]))
    rt = data["data"]["repository"]["pullRequest"]["reviewThreads"]
    if rt["totalCount"] > len(rt["nodes"]):
        raise Usage("more than %d review threads; page through them by hand" % len(rt["nodes"]))
    return [((n.get("comments") or {}).get("nodes") or [{}])[0].get("url", n.get("path", "?"))
            for n in rt["nodes"] if not n["isResolved"]]


def hygiene_findings(texts):
    """'<label>: <category>: <match>' for each finding of scripts/ci/check-public-hygiene.sh
    over the given {label: text}."""
    script = os.path.join(HERE, "..", "ci", "check-public-hygiene.sh")
    labels = list(texts)
    with tempfile.TemporaryDirectory() as d:
        for i, label in enumerate(labels):
            with open(os.path.join(d, "part-%d.txt" % i), "w", encoding="utf-8") as fh:
                fh.write(texts[label])
        r = subprocess.run(["bash", script, d], capture_output=True, text=True, cwd=d, timeout=120)
    if r.returncode not in (0, 1):
        raise Usage("check-public-hygiene.sh failed: %s" % r.stderr.strip()[:300])
    out = []
    for line in (r.stdout + "\n" + r.stderr).split("\n"):
        m = re.match(r"^part-(\d+)\.txt:\d+: (.*)$", line)
        if m:
            out.append("%s: %s" % (labels[int(m.group(1))], m.group(2)))
    if r.returncode == 1 and not out:
        raise Usage("check-public-hygiene.sh reported findings this parser cannot read")
    return out


def pr_check(spec, task, pr, repo=None, required=DEFAULT_REQUIRED_CHECKS, accept_scope=()):
    """(verdict, facts, reasons): verdict is 'ready', 'not-ready' or 'unknown'. task None is
    a lifecycle pull request: no completion entry or scope check, only specs/ changes."""
    slug = repo_slug(repo)
    view = json.loads(gh(["pr", "view", str(pr), "--json",
                          "state,isDraft,mergeable,mergeStateStatus,headRefName,headRefOid,title,body,files,statusCheckRollup"],
                         repo=slug))
    reasons, unknown = [], []
    facts = {"pr": pr, "head": view.get("headRefOid", ""), "branch": view.get("headRefName", "")}
    if view.get("state") != "OPEN":
        reasons.append("state:%s" % view.get("state"))
    if view.get("isDraft"):
        reasons.append("draft")
    ms, mg = view.get("mergeStateStatus"), view.get("mergeable")
    facts["merge_state"] = ms
    if mg == "CONFLICTING" or ms == "DIRTY":
        reasons.append("conflict: merge origin/main into the branch")
    elif ms == "BEHIND":
        reasons.append("behind: merge origin/main into the branch")
    elif ms == "BLOCKED":
        reasons.append("blocked: branch protection is not satisfied")
    elif ms in (None, "UNKNOWN") or mg in (None, "UNKNOWN"):
        unknown.append("merge state not computed yet")
    checks = latest_checks(view.get("statusCheckRollup"))
    facts["checks"] = "%d" % len(checks)
    for name, outcome in sorted(checks.items()):
        if outcome == "fail":
            reasons.append("check-failed:%s" % name)
        elif outcome == "pending":
            unknown.append("check-pending:%s" % name)
    for name in required:
        if name not in checks:
            unknown.append("check-missing:%s" % name)
    threads = unresolved_threads(slug, pr)
    facts["unresolved_threads"] = len(threads)
    for url in threads:
        reasons.append("thread-unresolved:%s" % url)
    # The completion entry, as the pull request's head carries it.
    files = [f["path"] for f in view.get("files") or []]
    if task is None:
        for f in files:
            if not re.fullmatch(r"specs/[^/]+/%s/.+" % re.escape(spec), f):
                reasons.append("scope: a lifecycle pull request changes only specs/*/%s/, not %s" % (spec, f))
        files = []
    tasks_paths = [p for p in files if re.fullmatch(r"specs/[^/]+/%s/tasks\.md" % re.escape(spec), p)]
    if task is None:
        pass
    elif len(tasks_paths) != 1:
        reasons.append("entry: the pull request does not change specs/*/%s/tasks.md" % spec)
    else:
        raw = gh(["api", "repos/%s/contents/%s?ref=%s" % (slug, tasks_paths[0], facts["head"])])
        content = base64.b64decode(json.loads(raw)["content"]).decode("utf-8")
        idx = by_number(parse_tasks(content))
        if task not in idx:
            reasons.append("entry: task %d is not in %s" % (task, tasks_paths[0]))
        else:
            for p in field_problems(idx[task], pr=pr):
                reasons.append("entry: %s" % p)
            listed = idx[task]["files"]
            spec_dir = os.path.dirname(tasks_paths[0]) + "/"
            recorded = idx[task]["fields"].get("Spec deviations", "")
            for f in files:
                if f.startswith(spec_dir) or f in accept_scope or any(patterns_overlap(f, l) for l in listed):
                    continue
                if f not in recorded and os.path.basename(f) not in recorded:
                    reasons.append("scope: %s is outside the task's Files and not named in Spec deviations" % f)
    # Leak check: the public body and every added line.
    diff = gh(["pr", "diff", str(pr)], repo=slug)
    added = "\n".join(l[1:] for l in diff.split("\n") if l.startswith("+") and not l.startswith("+++"))
    for finding in hygiene_findings({"body": view.get("body") or "", "title": view.get("title") or "", "diff": added}):
        reasons.append("leak: %s" % finding)
    if reasons:
        return "not-ready", facts, reasons + unknown
    if unknown:
        return "unknown", facts, unknown
    return "ready", facts, []


def verify_merged(spec, task, pr, repo=None, fetch=True):
    slug = repo_slug(repo)
    view = json.loads(gh(["pr", "view", str(pr), "--json", "state,mergeCommit"], repo=slug))
    problems = []
    oid = (view.get("mergeCommit") or {}).get("oid", "")
    if view.get("state") != "MERGED" or not oid:
        return ["pull request #%d is %s, not merged" % (pr, view.get("state"))]
    if fetch:
        run(["git", "fetch", "-q", "origin", "main"])
    if run(["git", "merge-base", "--is-ancestor", oid, "origin/main"], check=False).returncode != 0:
        problems.append("merge commit %s is not on origin/main" % oid[:12])
    path, tasks = tasks_at("origin/main", spec)
    t = by_number(tasks).get(task)
    if t is None:
        problems.append("task %d is not in origin/main:%s" % (task, path))
    else:
        problems += ["origin/main:%s task %d: %s" % (path, task, p) for p in field_problems(t, pr=pr)]
    return problems


# ---- run events --------------------------------------------------------------

def append_event(args):
    row = {
        "schema_version": 1,
        "ts": datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "spec": args.spec, "kind": args.kind, "task": args.task, "attempt": args.attempt,
        "agent": args.agent, "pr": args.pr, "result": args.result, "detail": args.detail or "",
    }
    d = main_data_dir()
    os.makedirs(d, exist_ok=True)
    with open(os.path.join(d, "run-events.jsonl"), "a", encoding="utf-8") as fh:
        fh.write(json.dumps(row, separators=(",", ":"), ensure_ascii=False) + "\n")
    return row


def read_events(spec):
    path = os.path.join(main_data_dir(), "run-events.jsonl")
    rows = []
    try:
        with open(path, encoding="utf-8") as fh:
            for line in fh:
                try:
                    r = json.loads(line)
                except ValueError:
                    continue
                if r.get("spec") == spec:
                    rows.append(r)
    except FileNotFoundError:
        pass
    return rows


def summarize(rows):
    tasks, dispatched = {}, []
    for r in rows:
        n = r.get("task")
        if n is None:
            continue
        t = tasks.setdefault(n, {"attempts": 0, "returned": 0, "failed": 0, "review_rounds": 0,
                                 "pr": None, "merged": False, "agent": None})
        k = r["kind"]
        if k == "dispatch":
            t["attempts"] += 1
            t["agent"] = r.get("agent") or t["agent"]
            dispatched.append((n, r.get("attempt") or t["attempts"]))
        elif k == "return":
            t["returned"] += 1
        elif k in ("fail", "lost"):
            t["failed"] += 1
        elif k == "review_round":
            t["review_rounds"] += 1
        elif k == "merge" and r.get("result") == "ok":
            t["merged"] = True
        if r.get("pr"):
            t["pr"] = r["pr"]
    for t in tasks.values():
        t["first_pass"] = t["attempts"] == 1 and t["failed"] == 0 and t["merged"]
    n_dispatched = len(dispatched)
    n_returned = sum(t["returned"] for t in tasks.values())
    n_failed = sum(t["failed"] for t in tasks.values())
    unaccounted = []
    for n, t in sorted(tasks.items()):
        missing = t["attempts"] - t["returned"] - t["failed"]
        if missing > 0:
            unaccounted.append("task %d (%d dispatch%s)" % (n, missing, "es" if missing > 1 else ""))
    return {"tasks": tasks, "dispatched": n_dispatched, "returned": n_returned, "failed": n_failed,
            "unaccounted": unaccounted}


# ---- CLI ---------------------------------------------------------------------

def ints(value):
    return [int(x) for x in value.split(",") if x.strip()] if value else []


def cmd_main(argv):
    ap = argparse.ArgumentParser(prog="runspec.py", description=__doc__.split("\n")[0])
    sub = ap.add_subparsers(dest="cmd", required=True)
    p = sub.add_parser("tasks"); p.add_argument("tasks_md")
    p = sub.add_parser("status"); p.add_argument("tasks_md"); p.add_argument("--in-flight", default="")
    p = sub.add_parser("batch"); p.add_argument("tasks_md"); p.add_argument("--in-flight", default="")
    p.add_argument("--max", type=int, default=4)
    p = sub.add_parser("closure"); p.add_argument("tasks_md"); p.add_argument("n", type=int)
    p = sub.add_parser("entry-check"); p.add_argument("tasks_md"); p.add_argument("n", type=int)
    p.add_argument("--pr", type=int)
    p = sub.add_parser("handoff-seed"); p.add_argument("spec_dir")
    p = sub.add_parser("handoff"); p.add_argument("spec_dir"); p.add_argument("n", type=int)
    p = sub.add_parser("report"); p.add_argument("--file")
    p = sub.add_parser("deps-merged"); p.add_argument("spec"); p.add_argument("n", type=int)
    p.add_argument("--ref", default="origin/main"); p.add_argument("--no-fetch", action="store_true")
    for name in ("pr-check", "verify-merged"):
        p = sub.add_parser(name)
        p.add_argument("--spec", required=True); p.add_argument("--task", type=int, required=name != "pr-check")
        p.add_argument("--pr", type=int, required=True); p.add_argument("--repo")
        if name == "pr-check":
            p.add_argument("--lifecycle", action="store_true", help="a lifecycle pull request: no --task")
            p.add_argument("--require-check", action="append")
            p.add_argument("--accept-scope", action="append", default=[])
        else:
            p.add_argument("--no-fetch", action="store_true")
    p = sub.add_parser("event")
    p.add_argument("--spec", required=True); p.add_argument("--kind", required=True, choices=EVENT_KINDS)
    p.add_argument("--task", type=int); p.add_argument("--attempt", type=int); p.add_argument("--agent")
    p.add_argument("--pr", type=int); p.add_argument("--result", choices=("ok", "fail", "unknown"))
    p.add_argument("--detail")
    p = sub.add_parser("summary"); p.add_argument("--spec", required=True); p.add_argument("--json", action="store_true")
    a = ap.parse_args(argv)

    if a.cmd == "tasks":
        print(json.dumps(load_tasks(a.tasks_md), indent=1, ensure_ascii=False))
    elif a.cmd == "status":
        print(json.dumps(progress(load_tasks(a.tasks_md), ints(a.in_flight))))
    elif a.cmd == "batch":
        print(json.dumps(batch(load_tasks(a.tasks_md), ints(a.in_flight), a.max)))
    elif a.cmd == "closure":
        print(json.dumps(closure(load_tasks(a.tasks_md), a.n)))
    elif a.cmd == "entry-check":
        t = by_number(load_tasks(a.tasks_md)).get(a.n)
        if t is None:
            raise Usage("task %d does not exist" % a.n)
        problems = field_problems(t, a.pr)
        for pr_ in problems:
            print("%s: task %d: %s" % (a.tasks_md, a.n, pr_))
        if problems:
            return 1
        print("entry-check: task %d: ok" % a.n)
    elif a.cmd == "handoff-seed":
        path, added = handoff_seed(a.spec_dir)
        print("handoff-seed: %s: added sections for tasks %s" % (path, added or "none"))
    elif a.cmd == "handoff":
        deps, text = handoff_for(a.spec_dir, a.n)
        print(text if deps else "(task %d has no dependencies)" % a.n)
    elif a.cmd == "report":
        text = open(a.file, encoding="utf-8").read() if a.file else sys.stdin.read()
        rep, err = parse_report(text)
        if err:
            print(err)
            return 1
        print(json.dumps(rep, separators=(",", ":"), ensure_ascii=False))
    elif a.cmd == "deps-merged":
        if not a.no_fetch:
            run(["git", "fetch", "-q", "origin", "main"])
        path, tasks = tasks_at(a.ref, a.spec)
        idx = by_number(tasks)
        if a.n not in idx:
            raise Usage("task %d is not in %s:%s" % (a.n, a.ref, path))
        missing = [d for d in idx[a.n]["depends"] if not (d in idx and idx[d]["complete"])]
        for d in missing:
            print("unmerged: task %d (not complete in %s:%s)" % (d, a.ref, path))
        if missing:
            return 1
        print("deps-merged: task %d: every dependency is complete on %s" % (a.n, a.ref))
    elif a.cmd == "pr-check":
        if (a.task is None) != a.lifecycle:
            raise Usage("pr-check takes --task for a task pull request, or --lifecycle without --task")
        verdict, facts, reasons = pr_check(a.spec, a.task, a.pr, a.repo,
                                           tuple(a.require_check or DEFAULT_REQUIRED_CHECKS), tuple(a.accept_scope))
        for k, v in facts.items():
            print("%s=%s" % (k, v))
        for r in reasons:
            print("reason=%s" % r)
        print("verdict=%s" % verdict)
        return {"ready": 0, "not-ready": 1, "unknown": 3}[verdict]
    elif a.cmd == "verify-merged":
        problems = verify_merged(a.spec, a.task, a.pr, a.repo, fetch=not a.no_fetch)
        for p_ in problems:
            print("problem=%s" % p_)
        print("verdict=%s" % ("merged" if not problems else "not-verified"))
        return 1 if problems else 0
    elif a.cmd == "event":
        print(json.dumps(append_event(a), separators=(",", ":"), ensure_ascii=False))
    elif a.cmd == "summary":
        s = summarize(read_events(a.spec))
        if a.json:
            print(json.dumps(s, indent=1))
            return 0
        print("dispatched=%d returned=%d failed=%d" % (s["dispatched"], s["returned"], s["failed"]))
        for u in s["unaccounted"]:
            print("unaccounted=%s" % u)
        print("| Task | Agent | Attempts | Review rounds | PR | Merged | First pass |")
        print("|---|---|---|---|---|---|---|")
        for n, t in sorted(s["tasks"].items()):
            print("| %d | %s | %d | %d | %s | %s | %s |" % (
                n, t["agent"] or "-", t["attempts"], t["review_rounds"],
                "#%d" % t["pr"] if t["pr"] else "-", "yes" if t["merged"] else "no",
                "yes" if t["first_pass"] else "no"))
    return 0


def main(argv=None):
    try:
        return cmd_main(sys.argv[1:] if argv is None else argv)
    except Usage as e:
        print("runspec.py: %s" % e, file=sys.stderr)
        return 2
    except (KeyError, ValueError) as e:
        print("runspec.py: unreadable input: %s" % e, file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main())
