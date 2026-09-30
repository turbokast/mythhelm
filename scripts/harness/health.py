#!/usr/bin/env python3
"""health.py: a read-only report on the state of the development harness.

    health.py [--root DIR] [--repo O/R] [--offline] [--json] [--stale-days N]

Every check below is reported on every run, with one status (ok, warn, fail, unknown,
skipped), so a check that did not run can never pass for one that ran clean:

  budget          always-on bytes (CLAUDE.md plus rules without paths:) against the
                  lint's budget; warn below HEADROOM_WARN bytes of headroom
  evals           every config-regression eval case passes (.claude/evals/run_evals.py)
  proposals       pending proposals: count and oldest age; warn past --stale-days
  specs           specs in in-progress/ or unfinalized/ untouched for --stale-days
  gate-markers    gate markers in any worktree of the checkout that no longer match
                  their tree (stale evidence; harmless until a completion claim)
  vendors         which optional vendors this checkout has opted in to
  required-checks the main ruleset's required checks against
                  .claude/data/required-checks.json (GitHub)
  main-ci         main's CI at origin/main's tip, failed jobs classed real, infra or
                  unknown (GitHub)
  open-prs        open pull requests with unresolved review threads (GitHub)
  merged-branches local branches and worktrees whose pull request has merged, for the
                  maintainer to remove (GitHub; report only)

--offline skips the GitHub checks (status skipped). A GitHub check whose gh call
fails is unknown, with the reason. Nothing is fetched, written, deleted or merged: the
refs are read as they are, so fetch first for a current view of origin/main.

Exit 0 after printing the report, 2 on a usage error. Standard library only.
"""

import argparse
import datetime
import json
import os
import re
import subprocess
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.normpath(os.path.join(HERE, "..", ".."))
sys.path.insert(0, HERE)
sys.path.insert(0, os.path.join(REPO, "scripts", "ci"))
sys.path.insert(0, os.path.join(REPO, "scripts", "vendors"))
import finalize  # noqa: E402  (sibling scripts, not installed packages)
import gatelib  # noqa: E402
import harness_lint  # noqa: E402
import proposals  # noqa: E402
import runspec  # noqa: E402
from runspec import Usage, gh  # noqa: E402

STATUSES = ("ok", "warn", "fail", "unknown", "skipped")
HEADROOM_WARN = 1024
GITHUB_CHECKS = ("required-checks", "main-ci", "open-prs", "merged-branches")


def result(name, status, summary, details=()):
    return {"name": name, "status": status, "summary": summary, "details": list(details)}


def git(root, *args):
    r = subprocess.run(["git", "-C", root] + list(args), capture_output=True, text=True,
                       env=runspec.git_env(), timeout=60)
    if r.returncode != 0:
        raise Usage("git %s failed: %s" % (" ".join(args[:3]), r.stderr.strip()[:200]))
    return r.stdout


# ---- local checks --------------------------------------------------------------

def check_budget(root):
    total = 0
    for rel in ["CLAUDE.md"] + harness_lint.always_on_rules(root):
        path = os.path.join(root, rel)
        if os.path.isfile(path):
            total += os.path.getsize(path)
    budget = harness_lint.RULE_BUDGET_BYTES
    head = budget - total
    summary = "always-on %d of %d bytes, headroom %d" % (total, budget, head)
    if head < 0:
        return result("budget", "fail", summary + "; move text to path-conditional rules or knowledge/")
    return result("budget", "warn" if head < HEADROOM_WARN else "ok", summary)


def check_evals(root):
    paths = proposals.run_evals.case_files(root)
    if not paths:
        return result("evals", "warn", "no eval cases under .claude/evals/cases/")
    res = proposals.run_evals.run_all(root, paths)
    bad = [r for r in res if not r["passed"]]
    return result("evals", "fail" if bad else "ok", "%d of %d cases pass" % (len(res) - len(bad), len(res)),
                  ["%s: %s" % (r["id"], r["detail"]) for r in bad])


def check_proposals(root, stale_days, now):
    count, oldest, rows = proposals.summary(root, now)
    if not count:
        return result("proposals", "ok", "none pending")
    old = [r for r in rows if r["age_days"] is not None and r["age_days"] > stale_days]
    summary = "%d pending, oldest %s" % (count, "unknown" if oldest is None else "%d days" % oldest)
    details = ["%s (%s, %s days): %s" % (r["id"], r["type"], "?" if r["age_days"] is None else r["age_days"],
                                         r["target"]) for r in rows]
    return result("proposals", "warn" if old else "ok", summary + ("; decide them with /apply-proposals" if old else ""),
                  details)


def last_touch(root, rel):
    out = git(root, "log", "-1", "--format=%ct", "--", rel).strip()
    return int(out) if out else None


def check_specs(root, stale_days, now):
    stuck, seen = [], 0
    for state in ("in-progress", "unfinalized"):
        d = os.path.join(root, "specs", state)
        for name in sorted(os.listdir(d)) if os.path.isdir(d) else []:
            if not os.path.isdir(os.path.join(d, name)):
                continue
            seen += 1
            ts = last_touch(root, "specs/%s/%s" % (state, name))
            age = None if ts is None else int((now - ts) // 86400)
            if age is None or age > stale_days:
                stuck.append("specs/%s/%s: %s" % (state, name, "never committed" if age is None
                                                  else "untouched for %d days" % age))
    if not seen:
        return result("specs", "ok", "no spec in in-progress/ or unfinalized/")
    return result("specs", "warn" if stuck else "ok",
                  "%d in flight, %d untouched for more than %d days" % (seen, len(stuck), stale_days), stuck)


def worktrees(root):
    """[(path, branch or None)] for every worktree of the checkout, the main one first."""
    out, path = [], None
    for line in git(root, "worktree", "list", "--porcelain").split("\n"):
        if line.startswith("worktree "):
            path = line[len("worktree "):]
            out.append([path, None])
        elif line.startswith("branch refs/heads/") and out:
            out[-1][1] = line[len("branch refs/heads/"):]
    return [tuple(w) for w in out]


def check_gate_markers(root):
    stale, fresh = [], 0
    for path, _ in worktrees(root):
        data = os.path.join(path, ".claude", "data")
        names = sorted(os.listdir(data)) if os.path.isdir(data) else []
        fps = {}
        for n in names:
            m = re.match(r"^gate-marker-(.+)\.json$", n)
            if not m or m.group(1) not in gatelib.GATES:
                continue
            try:
                state = gatelib.marker_state(os.path.realpath(path), m.group(1), fps)
            except Usage as e:
                stale.append("%s: %s: unreadable tree (%s)" % (path, m.group(1), e))
                continue
            if state == "fresh":
                fresh += 1
            else:
                stale.append("%s: %s" % (path, m.group(1)))
    return result("gate-markers", "warn" if stale else "ok",
                  "%d fresh, %d stale (re-run scripts/harness/gate.sh before a completion claim)" % (fresh, len(stale)),
                  stale)


def check_vendors():
    try:
        import vendors  # noqa: E402  (scripts/vendors/vendors.py)
        pol = vendors.load_policy()
        rows = ["%s: %s%s" % (v, "on" if vendors.is_enabled(pol, v) else "off",
                              ", lanes on" if vendors.lanes_enabled(pol, v) else "")
                for v in vendors.VENDORS]
    except Exception as e:  # noqa: BLE001  (the vendor layer is optional; report, never fail)
        return result("vendors", "unknown", "cannot read the vendor policy: %s" % e)
    on = [r for r in rows if ": on" in r]
    return result("vendors", "ok", "%d of %d opted in (%s)" % (len(on), len(rows), "; ".join(rows)))


# ---- GitHub checks -------------------------------------------------------------

def check_required_checks(slug):
    expected = set(finalize.expected_required_checks())
    have = finalize.required_checks(slug)
    missing, extra = sorted(expected - have), sorted(have - expected)
    details = ["missing from the ruleset: %s" % c for c in missing] + \
              ["required but not in .claude/data/required-checks.json: %s" % c for c in extra]
    status = "fail" if missing else ("warn" if extra else "ok")
    return result("required-checks", status, "ruleset requires %d, expected %d" % (len(have), len(expected)), details)


def check_main_ci(root, slug):
    sha = git(root, "rev-parse", "--verify", "-q", "origin/main").strip()
    facts, reasons, unknown, jobs = finalize.ci_state(slug, sha)
    details = ["%s/%s class=%s step=%s %s" % (j["workflow"], j["job"], j["class"], j["step"], j["url"]) for j in jobs]
    if reasons:
        return result("main-ci", "fail", "red at %s" % sha[:12], details)
    if unknown:
        return result("main-ci", "unknown", "not decided at %s: %s" % (sha[:12], "; ".join(unknown)))
    return result("main-ci", "ok", "green at %s" % sha[:12])


def check_open_prs(slug):
    prs = json.loads(gh(["pr", "list", "-R", slug, "--state", "open", "--limit", "100",
                         "--json", "number,title,isDraft"]))
    details = []
    for p in prs:
        threads = runspec.unresolved_threads(slug, p["number"])
        if threads:
            details.append("#%d %s: %d unresolved thread%s" % (p["number"], p["title"], len(threads),
                                                              "" if len(threads) == 1 else "s"))
    return result("open-prs", "warn" if details else "ok",
                  "%d open, %d with unresolved threads" % (len(prs), len(details)), details)


def check_merged_branches(root, slug):
    merged = json.loads(gh(["pr", "list", "-R", slug, "--state", "merged", "--limit", "200",
                            "--json", "number,headRefName,headRefOid"]))
    by_branch = {}
    for p in merged:
        by_branch.setdefault(p["headRefName"], p)
    trees = {b: path for path, b in worktrees(root)[1:] if b}
    details = []
    for line in git(root, "for-each-ref", "--format=%(refname:short) %(objectname)", "refs/heads").split("\n"):
        if not line.strip():
            continue
        branch, oid = line.split(" ", 1)
        p = by_branch.get(branch)
        if branch == "main" or p is None:
            continue
        note = "" if p.get("headRefOid") == oid else " (local tip differs from the merged head)"
        where = " worktree %s" % trees[branch] if branch in trees else ""
        details.append("%s: PR #%d merged%s%s" % (branch, p["number"], where, note))
    return result("merged-branches", "warn" if details else "ok",
                  "%d local branch%s with a merged pull request (remove them yourself; this report deletes nothing)"
                  % (len(details), "" if len(details) == 1 else "es"), details)


# ---- report --------------------------------------------------------------------

def guarded(name, fn, *args):
    try:
        return fn(*args)
    except (Usage, harness_lint.TreeError, OSError, ValueError, KeyError, subprocess.SubprocessError) as e:
        return result(name, "unknown", "could not run: %s" % str(e)[:300])


def report(root, repo=None, offline=False, stale_days=7, now=None):
    now = now if now is not None else datetime.datetime.now(datetime.timezone.utc).timestamp()
    out = [guarded("budget", check_budget, root),
           guarded("evals", check_evals, root),
           guarded("proposals", check_proposals, root, stale_days, now),
           guarded("specs", check_specs, root, stale_days, now),
           guarded("gate-markers", check_gate_markers, root),
           guarded("vendors", check_vendors)]
    if offline:
        out += [result(n, "skipped", "--offline") for n in GITHUB_CHECKS]
        return out
    try:
        slug = runspec.repo_slug(repo)
    except Usage as e:
        return out + [result(n, "unknown", "no GitHub repository: %s" % str(e)[:200]) for n in GITHUB_CHECKS]
    out += [guarded("required-checks", check_required_checks, slug),
            guarded("main-ci", check_main_ci, root, slug),
            guarded("open-prs", check_open_prs, slug),
            guarded("merged-branches", check_merged_branches, root, slug)]
    return out


def render(results):
    lines = ["Harness health"]
    for r in results:
        lines.append("  %-8s %-16s %s" % (r["status"], r["name"], r["summary"]))
        lines += ["           - %s" % d for d in r["details"]]
    counts = {s: sum(r["status"] == s for r in results) for s in STATUSES}
    lines.append("checks=%d %s" % (len(results), " ".join("%s=%d" % (s, counts[s]) for s in STATUSES)))
    return "\n".join(lines)


def main(argv=None):
    ap = argparse.ArgumentParser(prog="health.py", description="Read-only harness health report.")
    ap.add_argument("--root", default=".")
    ap.add_argument("--repo")
    ap.add_argument("--offline", action="store_true")
    ap.add_argument("--json", action="store_true")
    ap.add_argument("--stale-days", type=int, default=7)
    try:
        a = ap.parse_args(argv)
    except SystemExit as e:
        return 2 if e.code else 0
    try:
        root = gatelib.toplevel(os.path.abspath(a.root))
    except Usage as e:
        sys.stderr.write("health.py: %s\n" % e)
        return 2
    results = report(root, a.repo, a.offline, a.stale_days)
    if a.json:
        counts = {s: sum(r["status"] == s for r in results) for s in STATUSES}
        print(json.dumps({"root": root, "checks": results, "counts": counts}, indent=1))
    else:
        print(render(results))
    return 0


if __name__ == "__main__":
    sys.exit(main())
