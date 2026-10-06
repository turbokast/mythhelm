#!/usr/bin/env python3
"""finalize.py: the mechanical half of finalizing a spec, so the finalize skills decide
from parsed facts and GitHub state, never from an agent's prose.

    finalize.py verify        --spec S [--alias A] [--repo O/R] [--no-fetch] [--expect-check NAME]...
                              every task complete with a merged pull request on origin/main,
                              main's CI green at its tip (which holds every merge of the
                              spec), the required checks equal to .claude/data/required-checks.json,
                              the CI OK job's needs not shrunk, no open thread or
                              follow-up PR
    finalize.py range         --spec S [--extra-pr P]... [--repo O/R] [--no-fetch]
                              the review range (JSON): base, head, the merge commits of the
                              spec's task pull requests (and of --extra-pr fix pull requests),
                              the files they changed, and foreign commits in the range that
                              touched those files
    finalize.py ci            --sha SHA [--repo O/R] [--wait] [--timeout S] [--interval S]
                              main's CI runs for one commit, each failed job classed
                              real | infra | unknown
    finalize.py publish-check --spec S --pr P [--repo O/R] [--epic E]... [--no-changelog]
                              [--fix] [--require-check NAME]...
                              the finalize pull request (or, with --fix, a fix pull request
                              of the spec) may be merged
    finalize.py epic          --spec S [--root DIR]      ROLLUP <epic> <state> | NOOP <reason>
    finalize.py changelog-add --section SEC --entry TEXT [--file PATH]
    finalize.py changelog-release --version X.Y.Z --date YYYY-MM-DD [--file PATH]
    finalize.py release-notes --version X.Y.Z|Unreleased [--file PATH] [--draft-body FILE]
    finalize.py proposal-id   --spec S [--root DIR]      the next free P-<spec>-<n>

verify, ci and publish-check print key=value facts, one 'reason=' line per blocking
finding and a closing 'verdict=' line. They exit 0 when the verdict is a pass (ready,
green), 1 when it is not (each reason printed), and 3 when GitHub has not decided yet
(a run still queued or in progress, a check pending, a merge state not computed): poll
again, never read 3 as a pass. Every other command exits 0 on success, 1 on a finding
and 2 on a usage error or an unreadable input.

The CI classes: 'real' is a job whose failing step is a project step (a gate command
that exited non-zero); 'infra' is a job that never produced a verdict on the code (a
setup or checkout step failed, the run was cancelled, or the runner never started);
'unknown' is everything else, a timeout included, since a hang can be the code's. No
class is 'flake': calling a failure a flake needs a mechanism, which a job listing
cannot supply (.claude/rules/agent-behavioral-posture.md section 7).

Reads GitHub through the gh CLI and git and never merges, pushes, reruns or edits. The
changelog commands rewrite the file they name, and nothing else writes. Standard
library only; gh and git are looked up on PATH, so tests put stand-ins first.
"""

import argparse
import datetime
import json
import os
import re
import sys
import time

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import runspec  # noqa: E402  (a sibling script, not an installed package)
from runspec import Usage, gh, run  # noqa: E402

REQUIRED_CHECKS_FILE = os.path.join(HERE, "..", "..", ".claude", "data", "required-checks.json")
PR_REVIEW_CHECKS = ("CI OK", "CodeRabbit")
SPEC_NAME = re.compile(r"^[a-z0-9][a-z0-9-]*$")
CHANGELOG_SECTIONS = ("Added", "Changed", "Deprecated", "Removed", "Fixed", "Security")
CHANGELOG_HEAD = """# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]
"""
LINK_REF = re.compile(r"^\[[^\]]+\]: \S+$")
RELEASE_HEADING = re.compile(r"^## \[(\d+)\.(\d+)\.(\d+)(-[0-9A-Za-z.-]+)?\] - (\d{4}-\d{2}-\d{2})$")
# Steps GitHub or a setup action runs before or after the project's own commands.
INFRA_STEP = re.compile(r"(?i)^(set up job|complete job|initialize containers|stop containers|post |"
                        r"run actions/(checkout|setup-[a-z]+|cache|download-artifact|upload-artifact)@|"
                        r"checkout|set up (go|python|node))")
# The paths a finalize pull request may change, besides the spec's own directory.
PUBLISH_SCOPE = (re.compile(r"^(CHANGELOG\.md|README\.md|CLAUDE\.md|AGENTS\.md|WORKFLOW\.md)$"),
                 re.compile(r"^(docs|knowledge)/"),
                 re.compile(r"^\.claude/proposals/pending\.md$"))


def check_name(name):
    if not SPEC_NAME.match(name or ""):
        raise Usage("spec names are kebab-case: %r" % name)
    return name


def verdict(facts, reasons, unknown, passing):
    for k, v in facts:
        print("%s=%s" % (k, v))
    for r in reasons + unknown:
        print("reason=%s" % r)
    if reasons:
        print("verdict=not-ready" if passing == "ready" else "verdict=red")
        return 1
    if unknown:
        print("verdict=unknown" if passing == "ready" else "verdict=pending")
        return 3
    print("verdict=%s" % passing)
    return 0


# ---- the spec's pull requests ------------------------------------------------

def spec_prs(spec):
    """(path, tasks, {task: pr}) from origin/main's tasks.md."""
    path, tasks = runspec.tasks_at("origin/main", spec)
    return path, tasks, {t["number"]: t["pr"] for t in tasks}


def pr_merge(slug, pr):
    view = json.loads(gh(["pr", "view", str(pr), "--json", "number,state,mergeCommit,title,files"], repo=slug))
    oid = (view.get("mergeCommit") or {}).get("oid", "")
    return view, oid


def on_main(oid):
    return run(["git", "merge-base", "--is-ancestor", oid, "origin/main"], check=False).returncode == 0


def depth(oid):
    return int(run(["git", "rev-list", "--count", oid]).stdout.strip())


# ---- CI ------------------------------------------------------------------------

def main_runs(slug, sha):
    runs = json.loads(gh(["run", "list", "-R", slug, "--branch", "main", "--commit", sha, "--limit", "100",
                          "--json", "databaseId,status,conclusion,workflowName,event,url"]))
    return [r for r in runs if r.get("event") == "push"]


def classify_job(job):
    """(class, failing step name) for a job that did not succeed."""
    conclusion = (job.get("conclusion") or "").lower()
    if conclusion in ("cancelled", "startup_failure"):
        return "infra", "-"
    if conclusion != "failure":
        return "unknown", "-"
    failed = [s for s in job.get("steps") or [] if (s.get("conclusion") or "").lower() == "failure"]
    if not failed:
        return "unknown", "-"
    step = failed[0].get("name") or "?"
    return ("infra" if INFRA_STEP.match(step) else "real"), step


def ci_state(slug, sha):
    """(facts, reasons, unknown, jobs) for main's push runs of one commit."""
    runs = main_runs(slug, sha)
    facts = [("sha", sha), ("runs", len(runs))]
    reasons, unknown, jobs = [], [], []
    if not runs:
        unknown.append("no-runs: no push run on main for %s yet" % sha[:12])
    for r in sorted(runs, key=lambda r: r.get("workflowName") or ""):
        name = r.get("workflowName") or "?"
        if r.get("status") != "completed":
            unknown.append("run-pending:%s" % name)
            continue
        if (r.get("conclusion") or "").lower() in ("success", "skipped", "neutral"):
            continue
        view = json.loads(gh(["run", "view", str(r["databaseId"]), "-R", slug, "--json", "jobs"]))
        bad = [j for j in view.get("jobs") or []
               if (j.get("conclusion") or "").lower() not in ("success", "skipped", "neutral", "")]
        if not bad:
            bad = [{"name": "(run)", "conclusion": r.get("conclusion"), "steps": []}]
        for j in bad:
            cls, step = classify_job(j)
            jobs.append({"workflow": name, "job": j.get("name"), "conclusion": j.get("conclusion"),
                         "step": step, "class": cls, "url": r.get("url", "")})
            reasons.append("job-failed:%s/%s class=%s step=%s" % (name, j.get("name"), cls, step))
    return facts, reasons, unknown, jobs


def cmd_ci(a):
    slug = runspec.repo_slug(a.repo)
    deadline = time.monotonic() + a.timeout
    while True:
        facts, reasons, unknown, jobs = ci_state(slug, a.sha)
        if not a.wait or not unknown or reasons or time.monotonic() + a.interval > deadline:
            break
        time.sleep(a.interval)
    for j in jobs:
        print("job=%s/%s conclusion=%s class=%s step=%s url=%s" % (
            j["workflow"], j["job"], j["conclusion"], j["class"], j["step"], j["url"]))
    rc = verdict(facts, reasons, unknown, "green")
    return rc


# ---- verify --------------------------------------------------------------------

def ci_ok_needs(ref):
    """The needs: list of the ci-ok job in ref's .github/workflows/ci.yml, or None."""
    r = run(["git", "show", "%s:.github/workflows/ci.yml" % ref], check=False)
    if r.returncode != 0:
        return None
    lines = r.stdout.split("\n")
    for i, line in enumerate(lines):
        if not re.match(r"^  ci-ok:\s*$", line):
            continue
        body = []
        for inner in lines[i + 1:]:
            if re.match(r"^  \S", inner):
                break
            body.append(inner)
        for j, inner in enumerate(body):
            m = re.match(r"^(\s+)needs:\s*(.*?)\s*$", inner)
            if not m:
                continue
            if m.group(2).startswith("["):
                return sorted(x.strip() for x in m.group(2).strip("[]").split(",") if x.strip())
            if m.group(2):
                return [m.group(2)]
            items = []
            for item in body[j + 1:]:
                im = re.match(r"^\s+-\s+(\S+)\s*$", item)
                if not im:
                    break
                items.append(im.group(1))
            return sorted(items)
        return []
    return None


def expected_required_checks(path=REQUIRED_CHECKS_FILE):
    """The checks main's ruleset is expected to require, from the tracked data file."""
    try:
        with open(path, encoding="utf-8") as fh:
            data = json.load(fh)
    except (OSError, ValueError) as e:
        raise Usage("cannot read the expected required checks from %s: %s" % (path, e)) from e
    checks = data.get("required") if isinstance(data, dict) else None
    if not isinstance(checks, list) or not checks or not all(isinstance(c, str) and c for c in checks):
        raise Usage("%s has no non-empty 'required' list of check names" % path)
    return tuple(checks)


def required_checks(slug):
    rules = json.loads(gh(["api", "repos/%s/rules/branches/main" % slug]))
    out = set()
    for rule in rules:
        if rule.get("type") == "required_status_checks":
            for c in (rule.get("parameters") or {}).get("required_status_checks") or []:
                out.add(c.get("context"))
    return out


def verify(spec, alias=None, repo=None, fetch=True, expect=None):
    expect = tuple(expect) if expect else expected_required_checks()
    slug = runspec.repo_slug(repo)
    alias = alias or spec
    if fetch:
        run(["git", "fetch", "-q", "origin", "main"])
    path, tasks, prs = spec_prs(spec)
    state = path.split("/")[1]
    facts = [("spec", path.rsplit("/", 1)[0]), ("tasks", len(tasks))]
    reasons, unknown, notes = [], [], []
    if state == "done":
        reasons.append("state: the spec is already in done/")
    elif state != "unfinalized":
        reasons.append("state: the spec is in %s/, not unfinalized/; /run-spec moves it there after its last merge" % state)
    merges = []
    for t in tasks:
        n = t["number"]
        for p in runspec.field_problems(t):
            reasons.append("task %d: %s" % (n, p))
        pr = prs.get(n)
        if pr is None:
            continue
        view, oid = pr_merge(slug, pr)
        if view.get("state") != "MERGED" or not oid:
            reasons.append("task %d: PR #%d is %s, not merged" % (n, pr, view.get("state")))
            continue
        if not on_main(oid):
            reasons.append("task %d: PR #%d merge commit %s is not on origin/main" % (n, pr, oid[:12]))
            continue
        merges.append((depth(oid), oid, pr))
        for url in runspec.unresolved_threads(slug, pr):
            reasons.append("thread-unresolved: PR #%d %s" % (pr, url))
    open_prs = json.loads(gh(["pr", "list", "--state", "open", "--search", "%s in:title" % alias,
                              "--json", "number,title"], repo=slug))
    for p in open_prs:
        if re.search(r"\(%s (task \d+|fix)\)" % re.escape(alias), p.get("title", "")):
            reasons.append("open-pr: #%d %s" % (p["number"], p["title"]))
    if merges:
        merges.sort()
        last = merges[-1][1]
        facts.append(("last_merge", last))
        tip = run(["git", "rev-parse", "origin/main"]).stdout.strip()
        facts.append(("main_tip", tip))
        # The tip contains every merge of the spec, so its runs are the verdict; a red run
        # on an earlier commit that a later one repaired is history, not a block.
        _, r_, u_, _ = ci_state(slug, tip)
        reasons += ["main-tip: %s" % x for x in r_]
        unknown += ["main-tip: %s" % x for x in u_]
        for _, oid, pr in merges:
            if oid == tip:
                continue
            _, r_, u_, _ = ci_state(slug, oid)
            for x in r_ + u_:
                notes.append("history: PR #%d merge %s: %s" % (pr, oid[:12], x))
        base = run(["git", "rev-parse", merges[0][1] + "^"]).stdout.strip()
        before, after = ci_ok_needs(base), ci_ok_needs("origin/main")
        if before is not None:
            dropped = sorted(set(before) - set(after or []))
            if after is None or dropped:
                reasons.append("gate-weakened: CI OK no longer needs %s" % (", ".join(dropped) or "its job list"))
    have = required_checks(slug)
    missing = sorted(set(expect) - have)
    if missing:
        reasons.append("required-checks: main no longer requires %s" % ", ".join(missing))
    extra = sorted(have - set(expect))
    if extra:
        notes.append("required-checks: main also requires %s; add it to .claude/data/required-checks.json and docs/automation.md" % ", ".join(extra))
    for n in notes:
        print("note=%s" % n)
    return verdict(facts, reasons, unknown, "ready")


# ---- range ---------------------------------------------------------------------

def review_range(spec, repo=None, fetch=True, extra=()):
    slug = runspec.repo_slug(repo)
    if fetch:
        run(["git", "fetch", "-q", "origin", "main"])
    _, tasks, prs = spec_prs(spec)
    merges, files = [], set()
    items = sorted(prs.items()) + [("fix", p) for p in extra]
    for n, pr in items:
        if pr is None:
            raise Usage("task %s names no pull request; run finalize.py verify first" % n)
        view, oid = pr_merge(slug, pr)
        if not oid or not on_main(oid):
            raise Usage("PR #%d has no merge commit on origin/main; run finalize.py verify first" % pr)
        merges.append({"depth": depth(oid), "sha": oid, "pr": pr, "task": n})
        files.update(f["path"] for f in view.get("files") or [])
    if not merges:
        raise Usage("the spec has no tasks")
    merges.sort(key=lambda m: m["depth"])
    base = run(["git", "rev-parse", merges[0]["sha"] + "^"]).stdout.strip()
    head = merges[-1]["sha"]
    own = {m["sha"] for m in merges}
    foreign = []
    for line in run(["git", "log", "--format=%H %s", "%s..%s" % (base, head)]).stdout.splitlines():
        sha, _, subject = line.partition(" ")
        if sha in own:
            continue
        touched = run(["git", "diff-tree", "--no-commit-id", "--name-only", "-r", sha]).stdout.split()
        overlap = sorted(set(touched) & files)
        if overlap:
            foreign.append({"sha": sha, "subject": subject, "files": overlap})
    for m in merges:
        del m["depth"]
    return {"base": base, "head": head, "merges": merges, "files": sorted(files), "foreign": foreign}


# ---- publish-check -------------------------------------------------------------

def pr_gate(slug, pr, required=PR_REVIEW_CHECKS):
    """(view, reasons, unknown) for what every pull request needs before it merges: open,
    not a draft, mergeable, its checks green, no unresolved thread, and nothing that must
    not be published in its title, body or added lines."""
    # baseRefOid needs a newer gh than 2.45 offers, so ask for the base ref name
    # (available everywhere) and resolve its OID through the REST API. A base
    # that cannot be resolved degrades to "": callers needing it report that
    # as a reason instead of crashing.
    view = json.loads(gh(["pr", "view", str(pr), "--json",
                          "state,isDraft,mergeable,mergeStateStatus,baseRefName,headRefOid,title,body,files,statusCheckRollup"],
                         repo=slug))
    try:
        view["baseRefOid"] = json.loads(gh(["api", "repos/%s/commits/%s" % (slug, view.get("baseRefName") or "main")]))["sha"]
    except (Usage, ValueError, KeyError, TypeError):
        view["baseRefOid"] = ""
    reasons, unknown = [], []
    if view.get("state") != "OPEN":
        reasons.append("state:%s" % view.get("state"))
    if view.get("isDraft"):
        reasons.append("draft")
    ms, mg = view.get("mergeStateStatus"), view.get("mergeable")
    if mg == "CONFLICTING" or ms == "DIRTY":
        reasons.append("conflict: merge origin/main into the branch")
    elif ms == "BEHIND":
        reasons.append("behind: merge origin/main into the branch")
    elif ms == "BLOCKED":
        reasons.append("blocked: branch protection is not satisfied")
    elif ms in (None, "UNKNOWN") or mg in (None, "UNKNOWN"):
        unknown.append("merge state not computed yet")
    checks = runspec.latest_checks(view.get("statusCheckRollup"))
    for name, outcome in sorted(checks.items()):
        if outcome == "fail":
            reasons.append("check-failed:%s" % name)
        elif outcome == "pending":
            unknown.append("check-pending:%s" % name)
    for name in required:
        if name not in checks:
            unknown.append("check-missing:%s" % name)
    for url in runspec.unresolved_threads(slug, pr):
        reasons.append("thread-unresolved:%s" % url)
    diff = gh(["pr", "diff", str(pr)], repo=slug)
    added = "\n".join(l[1:] for l in diff.split("\n") if l.startswith("+") and not l.startswith("+++"))
    for finding in runspec.hygiene_findings({"body": view.get("body") or "", "title": view.get("title") or "",
                                             "diff": added}):
        reasons.append("leak: %s" % finding)
    return view, reasons, unknown


def publish_check(spec, pr, repo=None, epics=(), required=PR_REVIEW_CHECKS, changelog=True, fix=False):
    slug = runspec.repo_slug(repo)
    view, reasons, unknown = pr_gate(slug, pr, required)
    facts = [("pr", pr), ("head", view.get("headRefOid", ""))]
    files = [] if fix else [f["path"] for f in view.get("files") or []]
    own = re.compile(r"^specs/[^/]+/(%s)/" % "|".join(re.escape(x) for x in (spec,) + tuple(epics)))
    for f in files:
        if not (own.match(f) or any(p.match(f) for p in PUBLISH_SCOPE)):
            reasons.append("scope: %s is not the spec, its epic, the changelog, docs, knowledge or proposals" % f)
    if not fix and "specs/done/%s/retrospective.md" % spec not in files:
        reasons.append("content: the pull request does not add specs/done/%s/retrospective.md" % spec)
    if not fix and changelog and "CHANGELOG.md" not in files:
        reasons.append("content: the pull request does not change CHANGELOG.md")
    return verdict(facts, reasons, unknown, "ready")


# ---- epic rollup ---------------------------------------------------------------

def spec_homes(root):
    """{name: state} for every directory under specs/<state>/."""
    homes = {}
    for state in runspec.SPEC_STATES:
        d = os.path.join(root, "specs", state)
        for name in sorted(os.listdir(d)) if os.path.isdir(d) else []:
            if os.path.isdir(os.path.join(d, name)):
                homes.setdefault(name, []).append(state)
    return homes


def epic_decision(root, spec):
    sys.path.insert(0, os.path.join(HERE, "..", "ci"))
    try:
        import harness_lint  # noqa: E402  (the one parser of the Work Streams table)
    finally:
        sys.path.pop(0)
    homes = spec_homes(root)
    if homes.get(spec) != ["done"]:
        raise Usage("spec %s is in %s, not only in done/; move it before the rollup" % (spec, homes.get(spec)))
    parents = []
    for name, states in homes.items():
        plan = os.path.join(root, "specs", states[0], name, "plan.md")
        if len(states) != 1 or not os.path.isfile(plan) or os.path.exists(os.path.join(os.path.dirname(plan), "requirements.md")):
            continue
        with open(plan, encoding="utf-8") as fh:
            streams, _ = harness_lint.work_stream_specs(fh.read())
        if streams and spec in streams:
            parents.append((name, states[0], streams))
    if not parents:
        return "NOOP no-parent"
    if len(parents) > 1:
        raise Usage("spec %s is a work stream of %d epics: %s" % (spec, len(parents), ", ".join(p[0] for p in parents)))
    epic, state, streams = parents[0]
    if state == "done":
        return "NOOP already-done"
    pending = [s for s in streams if homes.get(s, ["missing"])[0] not in ("done", "archived")]
    if pending:
        return "NOOP pending-sibling:%s" % ",".join(pending)
    return "ROLLUP %s %s" % (epic, state)


# ---- changelog -----------------------------------------------------------------

def semver_key(m):
    pre = m.group(4)
    return (int(m.group(1)), int(m.group(2)), int(m.group(3)), pre is None, pre or "")


def changelog_problems(text):
    """[(line, detail)] for a CHANGELOG.md that is not Keep a Changelog shaped."""
    out = []
    lines = text.split("\n")
    first = next(((i, l) for i, l in enumerate(lines, 1) if l.strip()), (1, ""))
    if first[1].strip() != "# Changelog":
        out.append((first[0], "the first line is not '# Changelog'"))
    release, section, entries, seen_sections, versions = None, None, 0, set(), []
    releases_seen = []

    def close_section(line):
        if section is not None and entries == 0:
            out.append((line, "section '### %s' under %s is empty" % (section, release)))

    for i, line in enumerate(lines, 1):
        if line.startswith("## "):
            close_section(i)
            section, entries, seen_sections = None, 0, set()
            if line == "## [Unreleased]":
                release = "Unreleased"
                if releases_seen:
                    out.append((i, "'## [Unreleased]' is not the first release heading"))
            else:
                m = RELEASE_HEADING.match(line)
                if not m:
                    out.append((i, "release heading %r is not '## [Unreleased]' or '## [X.Y.Z] - YYYY-MM-DD'" % line))
                    release = line[3:]
                else:
                    release = line[3:]
                    try:
                        datetime.date.fromisoformat(m.group(5))
                    except ValueError:
                        out.append((i, "release date %s is not a date" % m.group(5)))
                    key = semver_key(m)
                    if versions and key >= versions[-1]:
                        out.append((i, "release %s is not older than the release above it" % release))
                    versions.append(key)
            releases_seen.append(release)
            continue
        if line.startswith("### "):
            close_section(i)
            name = line[4:].strip()
            if release is None:
                out.append((i, "section '### %s' is outside a release" % name))
            if name not in CHANGELOG_SECTIONS:
                out.append((i, "section '### %s' is not one of %s" % (name, ", ".join(CHANGELOG_SECTIONS))))
            if name in seen_sections:
                out.append((i, "section '### %s' appears twice under %s" % (name, release)))
            seen_sections.add(name)
            section, entries = name, 0
            continue
        if line.startswith("- "):
            if section is None and release is not None:
                out.append((i, "entry is not under a '### ' section"))
            entries += 1
            continue
        if not line.strip() or LINK_REF.match(line) or line.startswith("  "):
            continue
        if release is not None:
            out.append((i, "line is not an entry, a continuation or a link reference"))
    close_section(len(lines))
    if "Unreleased" not in releases_seen:
        out.append((0, "no '## [Unreleased]' section"))
    return out


def read_optional(path):
    try:
        with open(path, encoding="utf-8") as fh:
            return fh.read()
    except FileNotFoundError:
        return None
    except OSError as e:
        raise Usage("cannot read %s: %s" % (path, e)) from e


def write_checked(path, text):
    problems = changelog_problems(text)
    if problems:
        raise Usage("the result would not be a valid changelog: %s" % "; ".join("line %d: %s" % p for p in problems))
    tmp = path + ".tmp"
    with open(tmp, "w", encoding="utf-8") as fh:
        fh.write(text)
    os.replace(tmp, path)


def changelog_add(text, section, entry):
    if section not in CHANGELOG_SECTIONS:
        raise Usage("section must be one of %s" % ", ".join(CHANGELOG_SECTIONS))
    entry = entry.strip()
    if not entry or "\n" in entry:
        raise Usage("an entry is one non-empty line")
    if text is None:
        text = CHANGELOG_HEAD
    problems = changelog_problems(text)
    if problems:
        raise Usage("the changelog is malformed: %s" % "; ".join("line %d: %s" % p for p in problems))
    lines = text.rstrip("\n").split("\n")
    start = lines.index("## [Unreleased]")
    end = next((i for i in range(start + 1, len(lines))
                if lines[i].startswith("## ") or LINK_REF.match(lines[i])), len(lines))
    sections, cur = {}, None
    for line in lines[start + 1:end]:
        if line.startswith("### "):
            cur = sections.setdefault(line[4:].strip(), [])
        elif cur is not None and line.strip():
            cur.append(line)
    sections.setdefault(section, []).append("- " + entry)
    body = []
    for name in CHANGELOG_SECTIONS:
        if name in sections:
            body += ["", "### " + name, ""] + sections[name]
    tail = lines[end:]
    return "\n".join(lines[:start + 1] + body + ([""] + tail if tail else [])) + "\n"


def changelog_release(text, version, date):
    m = RELEASE_HEADING.match("## [%s] - %s" % (version, date))
    if not m:
        raise Usage("version %r and date %r do not make a release heading" % (version, date))
    problems = changelog_problems(text or "")
    if text is None or problems:
        raise Usage("the changelog is missing or malformed")
    lines = text.split("\n")
    start = lines.index("## [Unreleased]")
    end = next((i for i in range(start + 1, len(lines)) if lines[i].startswith("## ")), len(lines))
    if not any(l.startswith("- ") for l in lines[start + 1:end]):
        raise Usage("[Unreleased] has no entries to release")
    lines[start:start + 1] = ["## [Unreleased]", "", "## [%s] - %s" % (version, date)]
    return "\n".join(lines)


def release_notes(text, version):
    heading = "## [Unreleased]" if version == "Unreleased" else None
    lines = (text or "").split("\n")
    for i, l in enumerate(lines):
        if l == heading or (heading is None and l.startswith("## [%s] - " % version)):
            end = next((j for j in range(i + 1, len(lines)) if lines[j].startswith("## ")), len(lines))
            body = [x for x in lines[i + 1:end] if not LINK_REF.match(x)]
            return "\n".join(body).strip("\n")
    raise Usage("CHANGELOG.md has no section for %s" % version)


# ---- proposals -----------------------------------------------------------------

def next_proposal_id(root, spec):
    top = 0
    for name in ("pending.md", "applied.md"):
        text = read_optional(os.path.join(root, ".claude", "proposals", name)) or ""
        for m in re.finditer(r"^## P-%s-(\d+)\b" % re.escape(spec), text, re.M):
            top = max(top, int(m.group(1)))
    return "P-%s-%d" % (spec, top + 1)


# ---- CLI -----------------------------------------------------------------------

def cmd_main(argv):
    ap = argparse.ArgumentParser(prog="finalize.py", description=__doc__.split("\n")[0])
    sub = ap.add_subparsers(dest="cmd", required=True)
    p = sub.add_parser("verify")
    p.add_argument("--spec", required=True); p.add_argument("--alias"); p.add_argument("--repo")
    p.add_argument("--no-fetch", action="store_true"); p.add_argument("--expect-check", action="append")
    p = sub.add_parser("range")
    p.add_argument("--spec", required=True); p.add_argument("--repo"); p.add_argument("--no-fetch", action="store_true")
    p.add_argument("--extra-pr", type=int, action="append", default=[])
    p = sub.add_parser("ci")
    p.add_argument("--sha", required=True); p.add_argument("--repo"); p.add_argument("--wait", action="store_true")
    p.add_argument("--timeout", type=int, default=540); p.add_argument("--interval", type=int, default=30)
    p = sub.add_parser("publish-check")
    p.add_argument("--spec", required=True); p.add_argument("--pr", type=int, required=True); p.add_argument("--repo")
    p.add_argument("--epic", action="append", default=[]); p.add_argument("--require-check", action="append")
    p.add_argument("--no-changelog", action="store_true"); p.add_argument("--fix", action="store_true")
    p = sub.add_parser("epic"); p.add_argument("--spec", required=True); p.add_argument("--root", default=".")
    p = sub.add_parser("changelog-add")
    p.add_argument("--section", required=True); p.add_argument("--entry", required=True)
    p.add_argument("--file", default="CHANGELOG.md")
    p = sub.add_parser("changelog-release")
    p.add_argument("--version", required=True); p.add_argument("--date", required=True)
    p.add_argument("--file", default="CHANGELOG.md")
    p = sub.add_parser("release-notes")
    p.add_argument("--version", required=True); p.add_argument("--file", default="CHANGELOG.md")
    p.add_argument("--draft-body")
    p = sub.add_parser("proposal-id"); p.add_argument("--spec", required=True); p.add_argument("--root", default=".")
    a = ap.parse_args(argv)

    if getattr(a, "spec", None) is not None:
        check_name(a.spec)
    if a.cmd == "verify":
        return verify(a.spec, a.alias, a.repo, fetch=not a.no_fetch, expect=tuple(a.expect_check or ()))
    if a.cmd == "range":
        print(json.dumps(review_range(a.spec, a.repo, fetch=not a.no_fetch, extra=tuple(a.extra_pr)), indent=1))
    elif a.cmd == "ci":
        if not re.fullmatch(r"[0-9a-f]{7,40}", a.sha):
            raise Usage("--sha takes a commit id")
        return cmd_ci(a)
    elif a.cmd == "publish-check":
        for e in a.epic:
            check_name(e)
        return publish_check(a.spec, a.pr, a.repo, tuple(a.epic), tuple(a.require_check or PR_REVIEW_CHECKS),
                             changelog=not a.no_changelog, fix=a.fix)
    elif a.cmd == "epic":
        print(epic_decision(a.root, a.spec))
    elif a.cmd == "changelog-add":
        write_checked(a.file, changelog_add(read_optional(a.file), a.section, a.entry))
        print("changelog-add: %s: %s: %s" % (a.file, a.section, a.entry))
    elif a.cmd == "changelog-release":
        write_checked(a.file, changelog_release(read_optional(a.file), a.version, a.date))
        print("changelog-release: %s: [%s] - %s" % (a.file, a.version, a.date))
    elif a.cmd == "release-notes":
        notes = release_notes(read_optional(a.file), a.version)
        if a.draft_body:
            body = (read_optional(a.draft_body) or "").strip()
            if body:
                notes += "\n\n## Pull requests\n\n" + body
        print(notes)
    elif a.cmd == "proposal-id":
        print(next_proposal_id(a.root, a.spec))
    return 0


def main(argv=None):
    try:
        return cmd_main(sys.argv[1:] if argv is None else argv)
    except Usage as e:
        print("finalize.py: %s" % e, file=sys.stderr)
        return 2
    except (KeyError, ValueError, TypeError) as e:
        print("finalize.py: unreadable input: %s" % e, file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main())
