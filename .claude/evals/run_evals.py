#!/usr/bin/env python3
"""run_evals.py: the config-regression eval runner.

Each case in .claude/evals/cases/<id>.json pins one behaviour of the harness
configuration: a guard that still blocks, a template that still names what it must,
a budget that still holds. A case fails when a change silently undoes the behaviour.

    run_evals.py [--root DIR] [--case ID]... [--case-file PATH]... [--json] [--list]

    --root DIR        the tree the cases run against (default: this repository)
    --case ID         run only this case (repeatable)
    --case-file PATH  run the case in PATH instead of the tree's cases (repeatable);
                      used to run a new case against an older tree
    --json            one JSON object with every result
    --list            print the case ids and hazards, run nothing

A case is one JSON object:

    id        kebab-case, equal to the file name without .json
    hazard    one sentence: what goes wrong when the behaviour is lost
    source    where the case comes from: a proposal id (P-<spec>-<n>) or the
              harness area that introduced the guard
    targets   repository-relative globs (* ** ? {a,b}); every glob must match at
              least one file, so a renamed or deleted target fails the case instead
              of passing it vacuously
    grader    one of
      {"type": "must-match", "pattern": RE, "scope": "each" | "any"}
                every matched file (each, the default) or at least one (any)
                contains RE (Python regex, multiline)
      {"type": "must-not-match", "pattern": RE, "violation": TEXT}
                no matched file contains RE; TEXT is a sample of what the case
                forbids, and RE must match it, so a pattern that can never fire is
                caught when the case is written
      {"type": "script", "argv": [...], "stdin": TEXT, "exit": N,
       "stdout": RE, "stderr": RE}
                runs argv from the root (argv[0] is a repository path or a tool on
                PATH, never a shell), with CLAUDE_PROJECT_DIR set to the root;
                passes when the exit status is N (default 0) and each given
                regex matches its stream

Exit 0 when every selected case passes, 1 when any fails, 2 on a usage error or a
malformed case. Standard library only; no network. A script grader must not write
outside a temporary directory.
"""

import argparse
import json
import os
import re
import subprocess
import sys
import tempfile

HERE = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.normpath(os.path.join(HERE, "..", ".."))
sys.path.insert(0, os.path.join(REPO, "scripts", "ci"))
import harness_lint  # noqa: E402  (the one glob and tree-listing implementation)

CASE_ID = re.compile(r"^[a-z0-9][a-z0-9-]*$")
GRADERS = ("must-match", "must-not-match", "script")
SCRIPT_TIMEOUT = 60


def case_problems(case, name=None):
    """Every way the case object is malformed, as sentences."""
    if not isinstance(case, dict):
        return ["the case is not a JSON object"]
    out = []
    cid = case.get("id")
    if not isinstance(cid, str) or not CASE_ID.match(cid):
        out.append("id %r is not kebab-case" % (cid,))
    elif name is not None and cid != name:
        out.append("id %r differs from the file name %r" % (cid, name))
    for field in ("hazard", "source"):
        if not isinstance(case.get(field), str) or not case[field].strip():
            out.append("no %s" % field)
    targets = case.get("targets")
    if not isinstance(targets, list) or not targets or not all(isinstance(t, str) and t for t in targets):
        out.append("targets is not a non-empty list of globs")
    else:
        for t in targets:
            if t.startswith("/") or ".." in t.split("/"):
                out.append("target %r is not repository-relative" % t)
    g = case.get("grader")
    if not isinstance(g, dict) or g.get("type") not in GRADERS:
        out.append("grader.type is not one of %s" % ", ".join(GRADERS))
        return out
    unknown = set(g) - {"type", "pattern", "scope", "violation", "argv", "stdin", "exit", "stdout", "stderr"}
    if unknown:
        out.append("grader has unknown keys: %s" % ", ".join(sorted(unknown)))
    if g["type"] in ("must-match", "must-not-match"):
        if not isinstance(g.get("pattern"), str) or not g["pattern"]:
            out.append("grader.pattern is missing")
        else:
            try:
                re.compile(g["pattern"], re.M)
            except re.error as e:
                out.append("grader.pattern does not compile: %s" % e)
        if g.get("scope", "each") not in ("each", "any") or (g["type"] == "must-not-match" and "scope" in g):
            out.append("grader.scope is 'each' or 'any', and only for must-match")
        if g["type"] == "must-not-match":
            v = g.get("violation")
            if not isinstance(v, str) or not v:
                out.append("grader.violation is missing: a sample of the text the case forbids")
            elif isinstance(g.get("pattern"), str) and g["pattern"]:
                try:
                    if not re.search(g["pattern"], v, re.M):
                        out.append("grader.pattern does not match grader.violation, so it may never fire")
                except re.error:
                    pass
        elif "violation" in g:
            out.append("grader.violation is only for must-not-match")
    else:
        argv = g.get("argv")
        if not isinstance(argv, list) or not argv or not all(isinstance(a, str) for a in argv):
            out.append("grader.argv is not a non-empty list of strings")
        if not isinstance(g.get("exit", 0), int):
            out.append("grader.exit is not an integer")
        for k in ("stdin", "stdout", "stderr"):
            if k in g and not isinstance(g[k], str):
                out.append("grader.%s is not a string" % k)
        for k in ("stdout", "stderr"):
            if isinstance(g.get(k), str):
                try:
                    re.compile(g[k], re.M)
                except re.error as e:
                    out.append("grader.%s does not compile: %s" % (k, e))
    return out


def load_case(path):
    """(case, problems) for one case file."""
    name = os.path.basename(path)[:-len(".json")] if path.endswith(".json") else None
    try:
        with open(path, encoding="utf-8") as fh:
            case = json.load(fh)
    except (OSError, ValueError) as e:
        return None, ["cannot read %s: %s" % (path, e)]
    return case, case_problems(case, name)


def case_files(root):
    d = os.path.join(root, ".claude", "evals", "cases")
    if not os.path.isdir(d):
        return []
    return sorted(os.path.join(d, n) for n in os.listdir(d) if n.endswith(".json"))


def matched(root, files, targets):
    """({glob: [files]}) for each target glob."""
    out = {}
    for t in targets:
        rx = re.compile(harness_lint.glob_regex(t) + r"\Z")
        out[t] = [f for f in files if rx.match(f)]
    return out


def read_text(root, rel):
    try:
        with open(os.path.join(root, rel), encoding="utf-8") as fh:
            return fh.read()
    except (OSError, UnicodeDecodeError):
        return None


def run_case(root, case, files=None):
    """(passed, detail) for one well-formed case against the tree at root."""
    if files is None:
        files = harness_lint.tree_files(root)
    hits = matched(root, files, case["targets"])
    empty = [t for t, fs in hits.items() if not fs]
    if empty:
        return False, "target glob matches no file: %s" % ", ".join(empty)
    g = case["grader"]
    paths = sorted({f for fs in hits.values() for f in fs})
    if g["type"] in ("must-match", "must-not-match"):
        rx = re.compile(g["pattern"], re.M)
        found, missing = [], []
        for rel in paths:
            text = read_text(root, rel)
            if text is None:
                return False, "cannot read %s as text" % rel
            m = rx.search(text)
            if m:
                found.append("%s:%d" % (rel, text.count("\n", 0, m.start()) + 1))
            else:
                missing.append(rel)
        if g["type"] == "must-not-match":
            return (not found), ("forbidden pattern found at %s" % ", ".join(found) if found else "absent from %d file(s)" % len(paths))
        if g.get("scope", "each") == "any":
            return bool(found), ("found at %s" % found[0] if found else "pattern in none of %s" % ", ".join(paths))
        return (not missing), ("pattern missing from %s" % ", ".join(missing) if missing else "present in %d file(s)" % len(paths))
    argv = list(g["argv"])
    if "/" in argv[0]:
        argv[0] = os.path.join(root, argv[0])
    env = dict(os.environ, CLAUDE_PROJECT_DIR=root)
    for k in ("GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE"):
        env.pop(k, None)
    with tempfile.TemporaryDirectory() as tmp:
        env["TMPDIR"] = tmp
        try:
            r = subprocess.run(argv, cwd=root, input=g.get("stdin", ""), capture_output=True, text=True,
                               env=env, timeout=SCRIPT_TIMEOUT)
        except (OSError, subprocess.TimeoutExpired) as e:
            return False, "cannot run %s: %s" % (g["argv"][0], e)
    want = g.get("exit", 0)
    if r.returncode != want:
        return False, "exit %d, want %d; stderr: %s" % (r.returncode, want, r.stderr.strip()[:200])
    for k, stream in (("stdout", r.stdout), ("stderr", r.stderr)):
        if k in g and not re.search(g[k], stream, re.M):
            return False, "%s does not match %r: %s" % (k, g[k], stream.strip()[:200])
    return True, "exit %d" % want


def run_all(root, paths):
    """[{id, hazard, passed, detail, file}] for the case files, in order. A malformed case
    is a failure whose detail starts with 'malformed:'."""
    files = harness_lint.tree_files(root)
    results = []
    for path in paths:
        case, problems = load_case(path)
        cid = case.get("id") if isinstance(case, dict) and isinstance(case.get("id"), str) else os.path.basename(path)
        if problems:
            results.append({"id": cid, "hazard": "", "passed": False, "malformed": True,
                            "detail": "malformed: " + "; ".join(problems), "file": path})
            continue
        ok, detail = run_case(root, case, files)
        results.append({"id": cid, "hazard": case["hazard"], "passed": ok, "malformed": False,
                        "detail": detail, "file": path})
    return results


def main(argv=None):
    ap = argparse.ArgumentParser(prog="run_evals.py", description="Run the config-regression evals.")
    ap.add_argument("--root", default=REPO)
    ap.add_argument("--case", action="append", default=[])
    ap.add_argument("--case-file", action="append", default=[])
    ap.add_argument("--json", action="store_true")
    ap.add_argument("--list", action="store_true")
    try:
        a = ap.parse_args(argv)
    except SystemExit as e:
        return 2 if e.code else 0
    root = os.path.abspath(a.root)
    paths = [os.path.abspath(p) for p in a.case_file] or case_files(root)
    if a.case:
        by_id = {os.path.basename(p)[:-5]: p for p in paths}
        unknown = [c for c in a.case if c not in by_id]
        if unknown:
            sys.stderr.write("run_evals.py: no case named %s\n" % ", ".join(unknown))
            return 2
        paths = [by_id[c] for c in a.case]
    if not paths:
        sys.stderr.write("run_evals.py: no cases under %s/.claude/evals/cases\n" % root)
        return 2
    if a.list:
        for p in paths:
            case, problems = load_case(p)
            print("%s\t%s" % (os.path.basename(p)[:-5], "MALFORMED" if problems else case["hazard"]))
        return 2 if any(load_case(p)[1] for p in paths) else 0
    try:
        results = run_all(root, paths)
    except harness_lint.TreeError as e:
        sys.stderr.write("run_evals.py: %s\n" % e)
        return 2
    passed = sum(r["passed"] for r in results)
    if a.json:
        print(json.dumps({"passed": passed, "failed": len(results) - passed, "results": results}, indent=1))
    else:
        for r in results:
            print("%s  %s: %s" % ("PASS" if r["passed"] else "FAIL", r["id"], r["detail"]))
        print("evals: %d passed, %d failed" % (passed, len(results) - passed))
    if any(r["malformed"] for r in results):
        return 2
    return 0 if passed == len(results) else 1


if __name__ == "__main__":
    sys.exit(main())
