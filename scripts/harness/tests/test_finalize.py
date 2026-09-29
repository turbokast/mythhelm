"""Tests for scripts/harness/finalize.py. Offline: git runs against throwaway
repositories, and a stand-in gh on PATH serves fixture JSON keyed by command."""

import io
import json
import os
import shutil
import subprocess
import sys
import tempfile
import textwrap
import unittest
from contextlib import redirect_stderr, redirect_stdout

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.abspath(os.path.join(HERE, "..", "..", ".."))
sys.path.insert(0, os.path.join(ROOT, "scripts", "harness"))
import finalize  # noqa: E402

GIT_ENV = {"GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": os.devnull, "GIT_AUTHOR_NAME": "t",
           "GIT_AUTHOR_EMAIL": "t@example.com", "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@example.com"}
os.environ.update(GIT_ENV)
for _k in ("GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE"):
    os.environ.pop(_k, None)


def git(cwd, *args):
    return subprocess.run(["git", "-C", cwd] + list(args), check=True, capture_output=True, text=True).stdout


def write(path, text):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "w", encoding="utf-8") as fh:
        fh.write(text)


def read(path):
    with open(path, encoding="utf-8") as fh:
        return fh.read()


def call(argv):
    out, err = io.StringIO(), io.StringIO()
    with redirect_stdout(out), redirect_stderr(err):
        rc = finalize.main(argv)
    return rc, out.getvalue(), err.getvalue()


class Tmp(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.mkdtemp()
        self.addCleanup(shutil.rmtree, self.tmp, True)


# ---- changelog -----------------------------------------------------------------

GOOD_CHANGELOG = finalize.CHANGELOG_HEAD + textwrap.dedent("""\

    ### Added

    - A thing (#3)
      with a continuation line.

    ## [0.2.0] - 2026-09-01

    ### Fixed

    - A bug (#2)

    ## [0.1.0] - 2026-08-01

    ### Added

    - The start (#1)

    [Unreleased]: https://example.com/compare/v0.2.0...HEAD
    [0.2.0]: https://example.com/compare/v0.1.0...v0.2.0
    """)


class ChangelogTests(Tmp):
    def problems(self, text):
        return [d for _, d in finalize.changelog_problems(text)]

    def test_a_good_changelog_is_clean(self):
        self.assertEqual(finalize.changelog_problems(GOOD_CHANGELOG), [])
        self.assertEqual(finalize.changelog_problems(finalize.CHANGELOG_HEAD), [])

    def test_each_malformation_is_named(self):
        cases = {
            "the first line is not '# Changelog'": GOOD_CHANGELOG.replace("# Changelog", "# Changes", 1),
            "no '## [Unreleased]' section": GOOD_CHANGELOG.replace("## [Unreleased]\n\n### Added\n\n- A thing (#3)\n  with a continuation line.\n", ""),
            "is not '## [Unreleased]' or": GOOD_CHANGELOG.replace("## [0.2.0] - 2026-09-01", "## 0.2.0"),
            "is not one of Added": GOOD_CHANGELOG.replace("### Fixed", "### Bugfixes"),
            "appears twice": GOOD_CHANGELOG.replace("- The start (#1)", "- The start (#1)\n\n### Added\n\n- Again"),
            "is empty": GOOD_CHANGELOG.replace("### Fixed\n\n- A bug (#2)\n", "### Fixed\n"),
            "entry is not under a '### ' section": GOOD_CHANGELOG.replace("### Fixed\n\n", ""),
            "is not older than the release above it": GOOD_CHANGELOG.replace("0.1.0] - 2026-08-01", "0.3.0] - 2026-08-01"),
            "is not a date": GOOD_CHANGELOG.replace("2026-08-01", "2026-13-01"),
            "is not the first release heading": GOOD_CHANGELOG.replace("[0.1.0] - 2026-08-01", "[Unreleased]"),
            "line is not an entry": GOOD_CHANGELOG.replace("- A bug (#2)", "- A bug (#2)\nSome prose."),
            "is outside a release": "# Changelog\n\n### Added\n\n- x\n\n## [Unreleased]\n",
        }
        for needle, text in cases.items():
            with self.subTest(needle):
                self.assertTrue(any(needle in p for p in self.problems(text)), self.problems(text))

    def test_add_creates_orders_and_appends(self):
        path = os.path.join(self.tmp, "CHANGELOG.md")
        for section, entry in (("Fixed", "Fix a (#1)"), ("Added", "Add b (#2)"), ("Fixed", "Fix c (#3)")):
            self.assertEqual(call(["changelog-add", "--file", path, "--section", section, "--entry", entry])[0], 0)
        text = read(path)
        self.assertEqual(finalize.changelog_problems(text), [])
        self.assertTrue(text.startswith(finalize.CHANGELOG_HEAD))
        self.assertLess(text.index("### Added"), text.index("### Fixed"))
        self.assertIn("- Fix a (#1)\n- Fix c (#3)\n", text)

    def test_add_keeps_releases_continuations_and_link_references(self):
        path = os.path.join(self.tmp, "CHANGELOG.md")
        write(path, GOOD_CHANGELOG)
        self.assertEqual(call(["changelog-add", "--file", path, "--section", "Security", "--entry", "Harden x (#4)"])[0], 0)
        text = read(path)
        self.assertEqual(finalize.changelog_problems(text), [])
        self.assertIn("- A thing (#3)\n  with a continuation line.\n\n### Security\n\n- Harden x (#4)\n\n## [0.2.0]", text)
        self.assertTrue(text.endswith("[0.2.0]: https://example.com/compare/v0.1.0...v0.2.0\n"))

    def test_add_refuses_bad_input_and_a_malformed_file(self):
        path = os.path.join(self.tmp, "CHANGELOG.md")
        self.assertEqual(call(["changelog-add", "--file", path, "--section", "Misc", "--entry", "x"])[0], 2)
        self.assertEqual(call(["changelog-add", "--file", path, "--section", "Added", "--entry", "a\nb"])[0], 2)
        self.assertFalse(os.path.exists(path), "a refused add writes nothing")
        write(path, "# Notes\n")
        self.assertEqual(call(["changelog-add", "--file", path, "--section", "Added", "--entry", "x"])[0], 2)
        self.assertEqual(read(path), "# Notes\n", "a malformed changelog is left untouched")

    def test_release_and_notes(self):
        path = os.path.join(self.tmp, "CHANGELOG.md")
        write(path, GOOD_CHANGELOG)
        self.assertEqual(call(["changelog-release", "--file", path, "--version", "0.3.0", "--date", "2026-09-30"])[0], 0)
        text = read(path)
        self.assertEqual(finalize.changelog_problems(text), [])
        self.assertIn("## [Unreleased]\n\n## [0.3.0] - 2026-09-30\n\n### Added\n\n- A thing (#3)", text)
        rc, out, _ = call(["release-notes", "--file", path, "--version", "0.3.0"])
        self.assertEqual((rc, out.strip()), (0, "### Added\n\n- A thing (#3)\n  with a continuation line."))
        draft = os.path.join(self.tmp, "draft.md")
        write(draft, "- feat: a thing (#3)\n")
        rc, out, _ = call(["release-notes", "--file", path, "--version", "0.2.0", "--draft-body", draft])
        self.assertEqual(rc, 0)
        self.assertIn("### Fixed\n\n- A bug (#2)", out)
        self.assertIn("## Pull requests\n\n- feat: a thing (#3)", out)
        self.assertNotIn("https://example.com", out, "link references stay out of the notes")

    def test_release_refusals(self):
        path = os.path.join(self.tmp, "CHANGELOG.md")
        write(path, GOOD_CHANGELOG)
        self.assertEqual(call(["changelog-release", "--file", path, "--version", "0.2.1", "--date", "2026-09-30"])[0], 0)
        before = read(path)
        rc, _, err = call(["changelog-release", "--file", path, "--version", "0.2.2", "--date", "2026-09-30"])
        self.assertEqual(rc, 2)
        self.assertIn("no entries", err)
        write(path, GOOD_CHANGELOG)
        rc, _, err = call(["changelog-release", "--file", path, "--version", "0.1.5", "--date", "2026-09-30"])
        self.assertEqual(rc, 2, "a version older than the latest release is refused")
        self.assertIn("not older than", err)
        self.assertEqual(read(path), GOOD_CHANGELOG)
        self.assertEqual(call(["changelog-release", "--file", path, "--version", "1.0", "--date", "2026-09-30"])[0], 2)
        self.assertNotEqual(before, GOOD_CHANGELOG)
        self.assertEqual(call(["release-notes", "--file", path, "--version", "9.9.9"])[0], 2)


# ---- proposal ids --------------------------------------------------------------

class ProposalIdTests(Tmp):
    def test_next_id_counts_pending_and_applied_for_this_spec_only(self):
        self.assertEqual(call(["proposal-id", "--spec", "demo", "--root", self.tmp])[1].strip(), "P-demo-1")
        write(os.path.join(self.tmp, ".claude", "proposals", "pending.md"),
              "# Pending\n\n## P-demo-2 — a\n\n## P-demo-other-9 — b\n\n## P-other-7 — c\n")
        write(os.path.join(self.tmp, ".claude", "proposals", "applied.md"), "## P-demo-5 — d\n")
        self.assertEqual(finalize.next_proposal_id(self.tmp, "demo"), "P-demo-6")
        self.assertEqual(finalize.next_proposal_id(self.tmp, "demo-other"), "P-demo-other-10")
        self.assertEqual(call(["proposal-id", "--spec", "Bad Name", "--root", self.tmp])[0], 2)


# ---- epic rollup ---------------------------------------------------------------

PLAN = """## Epic — Master Plan

### Work Streams

| # | Spec | Scope | Dependencies |
|---|---|---|---|
| 1 | `alpha` | a | None |
| 2 | `beta` | b | Spec 1 |
"""


class EpicTests(Tmp):
    def spec(self, state, name, files=("requirements.md",)):
        for f in files:
            write(os.path.join(self.tmp, "specs", state, name, f), "x\n")

    def decide(self, name="alpha"):
        return finalize.epic_decision(self.tmp, name)

    def test_decisions(self):
        self.spec("done", "alpha")
        self.assertEqual(self.decide(), "NOOP no-parent")
        write(os.path.join(self.tmp, "specs", "in-progress", "big", "plan.md"), PLAN)
        self.spec("in-progress", "beta")
        self.assertEqual(self.decide(), "NOOP pending-sibling:beta")
        shutil.rmtree(os.path.join(self.tmp, "specs", "in-progress", "beta"))
        self.spec("archived", "beta")
        self.assertEqual(self.decide(), "ROLLUP big in-progress")
        shutil.rmtree(os.path.join(self.tmp, "specs", "archived", "beta"))
        self.spec("done", "beta")
        self.assertEqual(self.decide(), "ROLLUP big in-progress")
        shutil.move(os.path.join(self.tmp, "specs", "in-progress", "big"), os.path.join(self.tmp, "specs", "done", "big"))
        self.assertEqual(self.decide(), "NOOP already-done")

    def test_a_missing_sibling_is_pending(self):
        self.spec("done", "alpha")
        write(os.path.join(self.tmp, "specs", "in-progress", "big", "plan.md"), PLAN)
        self.assertEqual(self.decide(), "NOOP pending-sibling:beta")

    def test_refusals(self):
        self.spec("unfinalized", "alpha")
        with self.assertRaises(finalize.Usage):
            self.decide()
        shutil.move(os.path.join(self.tmp, "specs", "unfinalized", "alpha"), os.path.join(self.tmp, "specs", "done", "alpha"))
        write(os.path.join(self.tmp, "specs", "in-progress", "big", "plan.md"), PLAN)
        write(os.path.join(self.tmp, "specs", "refined", "huge", "plan.md"), PLAN)
        rc, _, err = call(["epic", "--spec", "alpha", "--root", self.tmp])
        self.assertEqual(rc, 2, "a spec claimed by two epics is refused, never guessed")
        self.assertIn("2 epics", err)


# ---- CI classification -----------------------------------------------------------

def step(name, conclusion):
    return {"name": name, "conclusion": conclusion, "number": 1}


class ClassifyTests(unittest.TestCase):
    def test_classes(self):
        cases = [
            ({"conclusion": "cancelled"}, ("infra", "-")),
            ({"conclusion": "startup_failure"}, ("infra", "-")),
            ({"conclusion": "timed_out"}, ("unknown", "-")),
            ({"conclusion": "failure", "steps": [step("Set up job", "failure")]}, ("infra", "Set up job")),
            ({"conclusion": "failure", "steps": [step("Run actions/checkout@abc", "failure")]}, ("infra", "Run actions/checkout@abc")),
            ({"conclusion": "failure", "steps": [step("Set up job", "success"), step("Test", "failure")]}, ("real", "Test")),
            ({"conclusion": "failure", "steps": [step("Test", "success")]}, ("unknown", "-")),
        ]
        for job, want in cases:
            with self.subTest(job=job):
                self.assertEqual(finalize.classify_job(job), want)


# ---- GitHub-backed commands ------------------------------------------------------

FAKE_GH = r'''#!/usr/bin/env python3
import json, os, sys
fx = json.load(open(os.environ["FAKE_GH_FIXTURE"]))
args = list(sys.argv[1:])
if "-R" in args:
    i = args.index("-R"); del args[i:i + 2]
def opt(name):
    return args[args.index(name) + 1] if name in args else ""
if args[:2] == ["api", "graphql"]:
    key = "api graphql " + next(a.split("=", 1)[1] for a in args if a.startswith("number="))
elif args[0] == "api":
    key = "api " + args[1]
elif args[:2] == ["run", "list"]:
    key = "run list " + opt("--commit")
elif args[:2] in (["pr", "view"], ["pr", "diff"], ["run", "view"]):
    key = " ".join(args[:3])
else:
    key = " ".join(args[:2])
out = fx.get(key)
if isinstance(out, dict) and "__seq__" in out:
    counter = os.environ["FAKE_GH_FIXTURE"] + "." + key.replace(" ", "_")
    n = int(open(counter).read()) if os.path.exists(counter) else 0
    open(counter, "w").write(str(n + 1))
    out = out["__seq__"][min(n, len(out["__seq__"]) - 1)]
if out is None:
    sys.stderr.write("fake gh: no fixture for %s\n" % key); sys.exit(1)
sys.stdout.write(out if isinstance(out, str) else json.dumps(out))
'''

DONE_TASKS = textwrap.dedent("""\
    ## Demo — Tasks

    ### Task 1 — Alpha ✅ COMPLETED

    - **Domain/agent**: go-implementer
    - **Budget**: standard
    - **Change**: Alpha.
    - **Files**: `src/a.go`
    - **Acceptance**: `TestA` passes.
    - **Invariants touched**: None.
    - **Status**: ✅ Completed — alpha; PR #11.
    - **Implementation**: Alpha.
    - **Spec deviations**: None.
    - **Files modified**: `src/a.go`.

    ### Task 2 — Beta ✅ COMPLETED

    - **Domain/agent**: go-implementer
    - **Budget**: standard
    - **Depends on**: Task 1
    - **Change**: Beta.
    - **Files**: `src/b.go`
    - **Acceptance**: `TestB` passes.
    - **Invariants touched**: None.
    - **Status**: ✅ Completed — beta; PR #12.
    - **Implementation**: Beta.
    - **Spec deviations**: None.
    - **Files modified**: `src/b.go`.
    """)

CI_YML = "jobs:\n  go:\n    runs-on: x\n  ci-ok:\n    name: CI OK\n    needs: [%s]\n    runs-on: x\n"
RULES = [{"type": "pull_request"}, {"type": "required_status_checks", "parameters": {"required_status_checks": [
    {"context": c} for c in finalize.REQUIRED_CHECKS]}}]


def gh_run(wf="CI", status="completed", conclusion="success", rid=1):
    return {"databaseId": rid, "status": status, "conclusion": conclusion, "workflowName": wf,
            "event": "push", "url": "https://github.com/acme/demo/actions/runs/%d" % rid}


def threads(resolved=True):
    return {"data": {"repository": {"pullRequest": {"reviewThreads": {"totalCount": 1, "nodes": [
        {"isResolved": resolved, "path": "a.go", "comments": {"nodes": [{"url": "https://github.com/acme/demo/pull/1#r1"}]}}]}}}}}


class GhBase(Tmp):
    def setUp(self):
        super().setUp()
        self.bin = os.path.join(self.tmp, "bin")
        write(os.path.join(self.bin, "gh"), FAKE_GH)
        os.chmod(os.path.join(self.bin, "gh"), 0o755)
        old = os.environ["PATH"]
        os.environ["PATH"] = self.bin + os.pathsep + old
        self.addCleanup(os.environ.__setitem__, "PATH", old)
        self.fixture = os.path.join(self.tmp, "fx.json")
        os.environ["FAKE_GH_FIXTURE"] = self.fixture
        self.fx = {"repo view": {"nameWithOwner": "acme/demo"}}

    def serve(self, **over):
        fx = dict(self.fx)
        for k, v in over.items():
            fx[k.replace("_", " ")] = v
        write(self.fixture, json.dumps(fx))


class CiTests(GhBase):
    SHA = "a" * 40

    def ci(self, *extra):
        return call(["ci", "--sha", self.SHA, "--repo", "acme/demo"] + list(extra))

    def test_green(self):
        self.serve(**{"run list " + self.SHA: [gh_run("CI"), gh_run("CodeQL", conclusion="skipped", rid=2)]})
        rc, out, _ = self.ci()
        self.assertEqual(rc, 0)
        self.assertIn("verdict=green", out)

    def test_red_names_each_failed_job_with_its_class(self):
        self.serve(**{"run list " + self.SHA: [gh_run("CI", conclusion="failure", rid=5), gh_run("Other", rid=6)],
                      "run view 5": {"jobs": [
                          {"name": "Go (ubuntu)", "conclusion": "failure", "steps": [step("Set up job", "success"), step("Test", "failure")]},
                          {"name": "Go (macos)", "conclusion": "cancelled", "steps": []},
                          {"name": "Lint", "conclusion": "success", "steps": []}]}})
        rc, out, _ = self.ci()
        self.assertEqual(rc, 1)
        self.assertIn("job=CI/Go (ubuntu) conclusion=failure class=real step=Test", out)
        self.assertIn("job=CI/Go (macos) conclusion=cancelled class=infra", out)
        self.assertNotIn("Lint", out)
        self.assertNotIn("flak", out.lower(), "no job is ever labelled a flake")
        self.assertIn("verdict=red", out)

    def test_pending_and_missing_are_never_green(self):
        self.serve(**{"run list " + self.SHA: [gh_run("CI", status="in_progress", conclusion="")]})
        rc, out, _ = self.ci()
        self.assertEqual((rc, out.splitlines()[-1]), (3, "verdict=pending"))
        self.serve(**{"run list " + self.SHA: [dict(gh_run("CI"), event="pull_request")]})
        rc, out, _ = self.ci()
        self.assertEqual(rc, 3, "only push runs on main count")
        self.assertIn("no-runs", out)

    def test_wait_polls_until_the_run_completes(self):
        self.serve(**{"run list " + self.SHA: {"__seq__": [[gh_run("CI", status="queued", conclusion="")],
                                                           [gh_run("CI")]]}})
        rc, out, _ = self.ci("--wait", "--interval", "0", "--timeout", "30")
        self.assertEqual(rc, 0, out)
        self.serve(**{"run list " + self.SHA: [gh_run("CI", status="queued", conclusion="")]})
        rc, out, _ = self.ci("--wait", "--interval", "1", "--timeout", "0")
        self.assertEqual(rc, 3, "a wait that runs out of time is pending, never green")

    def test_unreadable_github_is_a_usage_error(self):
        self.serve()
        self.assertEqual(self.ci()[0], 2)
        self.assertEqual(call(["ci", "--sha", "main"])[0], 2)


class RepoBase(GhBase):
    """origin (bare) and a clone: init with ci.yml, task 1's merge A, a foreign commit F
    touching task 1's file, task 2's merge B carrying the spec in unfinalized/."""

    def setUp(self):
        super().setUp()
        self.origin = os.path.join(self.tmp, "origin.git")
        subprocess.run(["git", "init", "-q", "--bare", "-b", "main", self.origin], check=True)
        self.repo = os.path.join(self.tmp, "clone")
        subprocess.run(["git", "init", "-q", "-b", "main", self.repo], check=True)
        git(self.repo, "remote", "add", "origin", self.origin)
        self.commit({".github/workflows/ci.yml": CI_YML % "go, harness"}, "init")
        self.base = git(self.repo, "rev-parse", "HEAD").strip()
        self.a = self.commit({"src/a.go": "package a\n"}, "task 1")
        self.f = self.commit({"src/a.go": "package a // edited\n", "other.txt": "x\n"}, "foreign")
        spec = "specs/unfinalized/demo/"
        self.b = self.commit({"src/b.go": "package b\n", spec + "tasks.md": DONE_TASKS,
                              spec + "requirements.md": "r\n", spec + "design.md": "d\n"}, "task 2")
        git(self.repo, "push", "-q", "origin", "main")
        cwd = os.getcwd()
        os.chdir(self.repo)
        self.addCleanup(os.chdir, cwd)
        self.fx.update({
            "pr view 11": {"number": 11, "state": "MERGED", "mergeCommit": {"oid": self.a}, "title": "t1",
                           "files": [{"path": "src/a.go"}]},
            "pr view 12": {"number": 12, "state": "MERGED", "mergeCommit": {"oid": self.b}, "title": "t2",
                           "files": [{"path": "src/b.go"}, {"path": "specs/in-progress/demo/tasks.md"}]},
            "api graphql 11": threads(), "api graphql 12": threads(),
            "pr list": [{"number": 40, "title": "feat: unrelated (other task 1)"}],
            "run list " + self.a: [gh_run("CI", conclusion="failure", rid=7)],
            "run view 7": {"jobs": [{"name": "Go", "conclusion": "failure", "steps": [step("Test", "failure")]}]},
            "run list " + self.b: [gh_run("CI")],
            "api repos/acme/demo/rules/branches/main": RULES,
        })

    def commit(self, files, msg):
        for path, text in files.items():
            write(os.path.join(self.repo, path), text)
        git(self.repo, "add", "-A")
        git(self.repo, "commit", "-q", "-m", msg)
        return git(self.repo, "rev-parse", "HEAD").strip()


class VerifyTests(RepoBase):
    def verify(self, *extra):
        return call(["verify", "--spec", "demo", "--repo", "acme/demo"] + list(extra))

    def test_ready_with_history_notes(self):
        self.serve()
        rc, out, _ = self.verify()
        self.assertEqual(rc, 0, out)
        self.assertIn("last_merge=%s" % self.b, out)
        self.assertIn("note=history: PR #11 merge %s: job-failed:CI/Go class=real step=Test" % self.a[:12], out)
        self.assertEqual(out.splitlines()[-1], "verdict=ready")

    def test_each_blocking_fact_is_a_reason(self):
        cases = {
            "PR #12 is OPEN, not merged": {"pr view 12": {"number": 12, "state": "OPEN", "mergeCommit": None, "files": []}},
            "thread-unresolved: PR #11": {"api graphql 11": threads(resolved=False)},
            "open-pr: #41": {"pr list": [{"number": 41, "title": "fix(ci): repair (demo fix)"}]},
            "main-tip: job-failed:CI/Go": {"run list " + self.b: [gh_run("CI", conclusion="failure", rid=7)]},
            "main no longer requires Analyze (go)": {"api repos/acme/demo/rules/branches/main": [
                {"type": "required_status_checks", "parameters": {"required_status_checks": [
                    {"context": c} for c in finalize.REQUIRED_CHECKS[:-1]]}}]},
        }
        for needle, over in cases.items():
            with self.subTest(needle):
                self.serve(**over)
                rc, out, _ = self.verify("--no-fetch")
                self.assertEqual(rc, 1, out)
                self.assertIn(needle, out)
                self.assertEqual(out.splitlines()[-1], "verdict=not-ready")

    def test_merge_commit_not_on_main(self):
        git(self.repo, "checkout", "-q", "-b", "side", self.base)
        stray = self.commit({"x.txt": "x\n"}, "stray")
        git(self.repo, "checkout", "-q", "main")
        self.serve(**{"pr view 11": {"number": 11, "state": "MERGED", "mergeCommit": {"oid": stray}, "files": []}})
        rc, out, _ = self.verify()
        self.assertEqual(rc, 1)
        self.assertIn("is not on origin/main", out)

    def test_incomplete_task_and_wrong_state(self):
        text = DONE_TASKS.replace("### Task 2 — Beta ✅ COMPLETED", "### Task 2 — Beta")
        os.makedirs(os.path.join(self.repo, "specs", "in-progress"))
        git(self.repo, "mv", "specs/unfinalized/demo", "specs/in-progress/demo")
        self.commit({"specs/in-progress/demo/tasks.md": text}, "back")
        git(self.repo, "push", "-q", "origin", "main")
        self.serve(**{"run list " + git(self.repo, "rev-parse", "HEAD").strip(): [gh_run("CI")]})
        rc, out, _ = self.verify()
        self.assertEqual(rc, 1)
        self.assertIn("reason=state: the spec is in in-progress/", out)
        self.assertIn("reason=task 2: heading does not end with", out)

    def test_main_tip_red_blocks_and_pending_is_unknown(self):
        tip = self.commit({"later.txt": "x\n"}, "later")
        git(self.repo, "push", "-q", "origin", "main")
        self.serve(**{"run list " + tip: [gh_run("CI")], "run list " + self.b: [gh_run("CI", conclusion="failure", rid=7)]})
        rc, out, _ = self.verify()
        self.assertEqual(rc, 0, "a red last merge that a later commit repaired is history: " + out)
        self.assertIn("note=history: PR #12 merge %s: job-failed:CI/Go" % self.b[:12], out)
        self.serve(**{"run list " + tip: [gh_run("CI", conclusion="failure", rid=7)]})
        rc, out, _ = self.verify()
        self.assertEqual(rc, 1)
        self.assertIn("reason=main-tip: job-failed:CI/Go", out)
        self.serve(**{"run list " + tip: [gh_run("CI", status="in_progress", conclusion="")]})
        rc, out, _ = self.verify("--no-fetch")
        self.assertEqual((rc, out.splitlines()[-1]), (3, "verdict=unknown"))

    def test_gate_weakened_and_extra_required_check(self):
        tip = self.commit({".github/workflows/ci.yml": CI_YML % "go"}, "drop harness from ci-ok")
        git(self.repo, "push", "-q", "origin", "main")
        rules = json.loads(json.dumps(RULES))
        rules[1]["parameters"]["required_status_checks"].append({"context": "Extra"})
        self.serve(**{"run list " + tip: [gh_run("CI")], "api repos/acme/demo/rules/branches/main": rules})
        rc, out, _ = self.verify()
        self.assertEqual(rc, 1)
        self.assertIn("reason=gate-weakened: CI OK no longer needs harness", out)
        self.assertIn("note=required-checks: main also requires Extra", out)


class NeedsTests(RepoBase):
    def test_flow_block_and_scalar_needs(self):
        self.assertEqual(finalize.ci_ok_needs(self.base), ["go", "harness"])
        for text, want in (("jobs:\n  ci-ok:\n    needs:\n      - lint\n      - go\n    runs-on: x\n", ["go", "lint"]),
                           ("jobs:\n  ci-ok:\n    needs: go\n", ["go"]),
                           ("jobs:\n  ci-ok:\n    runs-on: x\n  other:\n    needs: [a]\n", []),
                           ("jobs:\n  build:\n    needs: [a]\n", None)):
            with self.subTest(text=text):
                sha = self.commit({".github/workflows/ci.yml": text}, "ci")
                self.assertEqual(finalize.ci_ok_needs(sha), want)


class RangeTests(RepoBase):
    def test_range_files_and_foreign_overlap(self):
        self.serve()
        r = finalize.review_range("demo", "acme/demo")
        self.assertEqual((r["base"], r["head"]), (self.base, self.b))
        self.assertEqual([m["pr"] for m in r["merges"]], [11, 12])
        self.assertEqual(r["files"], ["specs/in-progress/demo/tasks.md", "src/a.go", "src/b.go"])
        self.assertEqual(r["foreign"], [{"sha": self.f, "subject": "foreign", "files": ["src/a.go"]}])

    def test_extra_fix_pull_request_extends_the_range(self):
        fix = self.commit({"src/c.go": "package c\n"}, "fix")
        git(self.repo, "push", "-q", "origin", "main")
        self.serve(**{"pr view 30": {"number": 30, "state": "MERGED", "mergeCommit": {"oid": fix},
                                     "files": [{"path": "src/c.go"}]}})
        rc, out, _ = call(["range", "--spec", "demo", "--repo", "acme/demo", "--extra-pr", "30"])
        self.assertEqual(rc, 0)
        r = json.loads(out)
        self.assertEqual(r["head"], fix)
        self.assertIn("src/c.go", r["files"])

    def test_unmerged_pull_request_is_refused(self):
        self.serve(**{"pr view 12": {"number": 12, "state": "OPEN", "mergeCommit": None, "files": []}})
        self.assertEqual(call(["range", "--spec", "demo", "--repo", "acme/demo", "--no-fetch"])[0], 2)


def check(name, conclusion="SUCCESS"):
    return {"__typename": "CheckRun", "name": name, "status": "COMPLETED", "conclusion": conclusion,
            "startedAt": "2026-01-01T00:00:10Z", "workflowName": "wf"}


class PublishCheckTests(GhBase):
    FILES = ["specs/unfinalized/demo/tasks.md", "specs/done/demo/tasks.md", "specs/done/demo/retrospective.md",
             "CHANGELOG.md", "docs/usage.md", "knowledge/finalize.md", ".claude/proposals/pending.md", "README.md"]

    def serve_pr(self, files=None, **over):
        view = {"state": "OPEN", "isDraft": False, "mergeable": "MERGEABLE", "mergeStateStatus": "CLEAN",
                "headRefOid": "0123abc", "title": "docs(spec): finalize demo", "body": "Finalize the demo spec.",
                "files": [{"path": p} for p in (self.FILES if files is None else files)],
                "statusCheckRollup": [check("CI OK"), {"__typename": "StatusContext", "context": "CodeRabbit",
                                                       "state": "SUCCESS", "startedAt": "2026-01-01T00:00:00Z"}]}
        view.update(over.pop("view", {}))
        fx = {"pr view 50": view, "api graphql 50": threads(), "pr diff 50": "+++ b/CHANGELOG.md\n+- A thing\n"}
        fx.update(over)
        self.serve(**{k.replace(" ", "_"): v for k, v in fx.items()})

    def pc(self, *extra):
        return call(["publish-check", "--spec", "demo", "--pr", "50", "--repo", "acme/demo"] + list(extra))

    def test_ready(self):
        self.serve_pr()
        rc, out, _ = self.pc()
        self.assertEqual((rc, out.splitlines()[-1]), (0, "verdict=ready"), out)

    def test_scope_content_and_review_state(self):
        cases = [
            ("scope: internal/x.go is not", {"files": self.FILES + ["internal/x.go"]}, ()),
            ("scope: specs/done/other/tasks.md", {"files": self.FILES + ["specs/done/other/tasks.md"]}, ()),
            ("content: the pull request does not add specs/done/demo/retrospective.md",
             {"files": [f for f in self.FILES if not f.endswith("retrospective.md")]}, ()),
            ("content: the pull request does not change CHANGELOG.md",
             {"files": [f for f in self.FILES if f != "CHANGELOG.md"]}, ()),
            ("thread-unresolved:", {"api graphql 50": threads(resolved=False)}, ()),
            ("check-failed:CI OK", {"view": {"statusCheckRollup": [check("CI OK", "FAILURE"), check("CodeRabbit")]}}, ()),
            ("leak: body", {"view": {"body": "ask " + "someone" + "@" + "corp" + "mail.io"}}, ()),
        ]
        for needle, over, extra in cases:
            with self.subTest(needle):
                files = over.pop("files", None)
                self.serve_pr(files, **over)
                rc, out, _ = self.pc(*extra)
                self.assertEqual(rc, 1, out)
                self.assertIn(needle, out)

    def test_epic_no_changelog_and_fix_modes(self):
        self.serve_pr(self.FILES + ["specs/in-progress/big/plan.md", "specs/done/big/plan.md"])
        self.assertEqual(self.pc()[0], 1, "an epic's move needs --epic")
        self.assertEqual(self.pc("--epic", "big")[0], 0)
        self.serve_pr([f for f in self.FILES if f != "CHANGELOG.md"])
        self.assertEqual(self.pc("--no-changelog")[0], 0)
        self.serve_pr(["internal/x.go"])
        self.assertEqual(self.pc("--fix")[0], 0, "a fix pull request is not scoped to the spec")

    def test_pending_review_is_unknown(self):
        self.serve_pr(view={"statusCheckRollup": [check("CI OK")]})
        rc, out, _ = self.pc()
        self.assertEqual((rc, out.splitlines()[-1]), (3, "verdict=unknown"))


if __name__ == "__main__":
    unittest.main()
