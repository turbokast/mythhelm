"""Tests for .claude/evals/run_evals.py and for the repository's own eval cases: each
case passes on this tree and goes red on a copy of it with the behaviour it pins
removed. Offline: every tree is a throwaway git repository."""

import io
import json
import os
import shutil
import subprocess
import sys
import tempfile
import unittest
from contextlib import redirect_stderr, redirect_stdout

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.abspath(os.path.join(HERE, "..", "..", ".."))
sys.path.insert(0, os.path.join(ROOT, ".claude", "evals"))
sys.path.insert(0, os.path.join(ROOT, "scripts", "ci"))
import harness_lint  # noqa: E402
import run_evals  # noqa: E402

GIT_ENV = {"GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": os.devnull, "GIT_AUTHOR_NAME": "t",
           "GIT_AUTHOR_EMAIL": "t@example.com", "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@example.com"}
os.environ.update(GIT_ENV)
for _k in ("GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "CLAUDE_PROJECT_DIR"):
    os.environ.pop(_k, None)


def write(path, text, mode=None):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "w", encoding="utf-8") as fh:
        fh.write(text)
    if mode:
        os.chmod(path, mode)


def git(cwd, *args):
    return subprocess.run(["git", "-C", cwd] + list(args), check=True, capture_output=True, text=True).stdout


def call(argv):
    out, err = io.StringIO(), io.StringIO()
    with redirect_stdout(out), redirect_stderr(err):
        rc = run_evals.main(argv)
    return rc, out.getvalue(), err.getvalue()


def case(cid, grader, targets=("docs/a.md",), **extra):
    c = {"id": cid, "hazard": "h", "source": "test", "targets": list(targets), "grader": grader}
    c.update(extra)
    return c


class Tree(unittest.TestCase):
    def setUp(self):
        self.root = tempfile.mkdtemp()
        self.addCleanup(shutil.rmtree, self.root, True)
        git(self.root, "init", "-q", "-b", "main")
        write(os.path.join(self.root, "docs", "a.md"), "alpha\nbeta\n")
        write(os.path.join(self.root, "docs", "b.md"), "alpha\n")

    def add(self, c, name=None):
        path = os.path.join(self.root, ".claude", "evals", "cases", (name or c["id"]) + ".json")
        write(path, json.dumps(c))
        return path

    def run_one(self, c):
        return run_evals.run_all(self.root, [self.add(c)])[0]


class GraderTests(Tree):
    def test_must_match_each_needs_every_file(self):
        r = self.run_one(case("m", {"type": "must-match", "pattern": "^beta$"}, ["docs/*.md"]))
        self.assertFalse(r["passed"])
        self.assertIn("pattern missing from docs/b.md", r["detail"])
        self.assertTrue(self.run_one(case("m", {"type": "must-match", "pattern": "^alpha$"}, ["docs/*.md"]))["passed"])

    def test_must_match_any_needs_one_file(self):
        r = self.run_one(case("m", {"type": "must-match", "pattern": "^beta$", "scope": "any"}, ["docs/*.md"]))
        self.assertTrue(r["passed"])
        self.assertEqual(r["detail"], "found at docs/a.md:2")

    def test_must_not_match_reports_where(self):
        g = {"type": "must-not-match", "pattern": "^beta$", "violation": "beta"}
        r = self.run_one(case("n", g, ["docs/*.md"]))
        self.assertFalse(r["passed"])
        self.assertEqual(r["detail"], "forbidden pattern found at docs/a.md:2")
        g["pattern"] = "^gamma$"
        g["violation"] = "gamma"
        self.assertTrue(self.run_one(case("n", g, ["docs/*.md"]))["passed"])

    def test_a_target_glob_matching_nothing_fails(self):
        g = {"type": "must-not-match", "pattern": "x", "violation": "x"}
        r = self.run_one(case("n", g, ["docs/*.md", "docs/gone/**"]))
        self.assertFalse(r["passed"])
        self.assertEqual(r["detail"], "target glob matches no file: docs/gone/**")

    def test_ignored_files_are_not_targets(self):
        write(os.path.join(self.root, ".gitignore"), "docs/b.md\n")
        r = self.run_one(case("m", {"type": "must-match", "pattern": "^beta$"}, ["docs/*.md"]))
        self.assertTrue(r["passed"], r["detail"])

    def test_script_exit_stdin_and_streams(self):
        write(os.path.join(self.root, "bin", "probe.sh"),
              '#!/bin/sh\nread line\necho "got $line in $CLAUDE_PROJECT_DIR" >&2\npwd\nexit 2\n', 0o755)
        g = {"type": "script", "argv": ["bin/probe.sh"], "stdin": "hello\n", "exit": 2,
             "stderr": "^got hello in " + self.root + "$", "stdout": "^" + os.path.realpath(self.root) + "$"}
        r = self.run_one(case("s", g, ["bin/probe.sh"]))
        self.assertTrue(r["passed"], r["detail"])
        g["exit"] = 0
        r = self.run_one(case("s", g, ["bin/probe.sh"]))
        self.assertFalse(r["passed"])
        self.assertIn("exit 2, want 0", r["detail"])
        g["exit"], g["stderr"] = 2, "^nothing like this$"
        r = self.run_one(case("s", g, ["bin/probe.sh"]))
        self.assertFalse(r["passed"])
        self.assertIn("stderr does not match", r["detail"])

    def test_script_runs_without_a_shell(self):
        g = {"type": "script", "argv": ["python3", "-c", "import sys; sys.exit(len(sys.argv))", "$(false)"], "exit": 2}
        self.assertTrue(self.run_one(case("s", g))["passed"])

    def test_a_missing_program_fails(self):
        r = self.run_one(case("s", {"type": "script", "argv": ["bin/absent.sh"]}))
        self.assertFalse(r["passed"])
        self.assertIn("cannot run bin/absent.sh", r["detail"])


class FormatTests(Tree):
    def problems(self, c, name=None):
        return run_evals.case_problems(c, name)

    def test_a_good_case_has_no_problems(self):
        self.assertEqual(self.problems(case("ok", {"type": "must-match", "pattern": "a"}), "ok"), [])

    def test_each_malformation_is_named(self):
        good = case("ok", {"type": "must-match", "pattern": "a"})
        cases = {
            "id 'Bad_Id' is not kebab-case": dict(good, id="Bad_Id"),
            "differs from the file name": good,
            "no hazard": dict(good, hazard=" "),
            "no source": {k: v for k, v in good.items() if k != "source"},
            "targets is not a non-empty list": dict(good, targets=[]),
            "is not repository-relative": dict(good, targets=["../x"]),
            "grader.type is not one of": dict(good, grader={"type": "grep"}),
            "grader has unknown keys: flags": dict(good, grader={"type": "must-match", "pattern": "a", "flags": "i"}),
            "grader.pattern does not compile": dict(good, grader={"type": "must-match", "pattern": "("}),
            "grader.scope is 'each' or 'any'": dict(good, grader={"type": "must-match", "pattern": "a", "scope": "all"}),
            "grader.violation is missing": dict(good, grader={"type": "must-not-match", "pattern": "a"}),
            "does not match grader.violation": dict(good, grader={"type": "must-not-match", "pattern": "a", "violation": "b"}),
            "grader.violation is only for must-not-match": dict(good, grader={"type": "must-match", "pattern": "a", "violation": "a"}),
            "grader.argv is not a non-empty list": dict(good, grader={"type": "script", "argv": "true"}),
            "grader.exit is not an integer": dict(good, grader={"type": "script", "argv": ["true"], "exit": "2"}),
        }
        for needle, c in cases.items():
            with self.subTest(needle):
                name = "other" if needle == "differs from the file name" else c.get("id")
                self.assertTrue(any(needle in p for p in self.problems(c, name)), self.problems(c, name))

    def test_a_malformed_case_fails_the_run_with_exit_2(self):
        self.add(case("ok", {"type": "must-match", "pattern": "alpha"}))
        self.add({"id": "broken"})
        rc, out, _ = call(["--root", self.root])
        self.assertEqual(rc, 2)
        self.assertIn("PASS  ok: present in 1 file(s)", out)
        self.assertIn("FAIL  broken: malformed: ", out)


class CliTests(Tree):
    def test_exit_codes_selection_and_json(self):
        self.add(case("green", {"type": "must-match", "pattern": "alpha"}))
        self.add(case("red", {"type": "must-match", "pattern": "omega"}))
        self.assertEqual(call(["--root", self.root, "--case", "green"])[0], 0)
        rc, out, _ = call(["--root", self.root])
        self.assertEqual(rc, 1)
        self.assertIn("evals: 1 passed, 1 failed", out)
        rc, out, _ = call(["--root", self.root, "--json"])
        data = json.loads(out)
        self.assertEqual((data["passed"], data["failed"]), (1, 1))
        self.assertEqual({r["id"]: r["passed"] for r in data["results"]}, {"green": True, "red": False})

    def test_unknown_case_and_empty_suite_are_usage_errors(self):
        self.add(case("green", {"type": "must-match", "pattern": "alpha"}))
        rc, _, err = call(["--root", self.root, "--case", "nope"])
        self.assertEqual(rc, 2)
        self.assertIn("no case named nope", err)
        shutil.rmtree(os.path.join(self.root, ".claude"))
        self.assertEqual(call(["--root", self.root])[0], 2)

    def test_case_file_runs_a_case_against_another_tree(self):
        elsewhere = tempfile.mkdtemp()
        self.addCleanup(shutil.rmtree, elsewhere, True)
        other = os.path.join(elsewhere, "x.json")
        write(other, json.dumps(case("x", {"type": "must-match", "pattern": "beta"})))
        rc, out, _ = call(["--root", self.root, "--case-file", other])
        self.assertEqual(rc, 0, out)

    def test_list_prints_ids_and_hazards(self):
        self.add(case("green", {"type": "must-match", "pattern": "alpha"}, hazard="the hazard"))
        rc, out, _ = call(["--root", self.root, "--list"])
        self.assertEqual((rc, out), (0, "green\tthe hazard\n"))


# ---- the repository's own cases ------------------------------------------------

def inflate(root, _case):
    with open(os.path.join(root, "CLAUDE.md"), "a", encoding="utf-8") as fh:
        fh.write("x" * (harness_lint.RULE_BUDGET_BYTES + 1))


def effort_dispatch(root, _case):
    with open(os.path.join(root, ".claude", "skills", "bootstrap", "SKILL.md"), "a", encoding="utf-8") as fh:
        fh.write('\nAgent(subagent_type: "harness-clerk", effort: "high", prompt: "count")\n')


# Removal of the pinned behaviour for cases the default mutation cannot express. The
# default: a must-not-match case gets its violation appended to its first target;
# every other case has its targets emptied (a hook becomes a no-op that allows all).
MUTATIONS = {
    "always-on-context-within-budget": inflate,
    "haiku-dispatches-pass-no-effort": effort_dispatch,
}


def default_mutation(root, c):
    files = harness_lint.tree_files(root)
    hits = run_evals.matched(root, files, c["targets"])
    targets = sorted({f for fs in hits.values() for f in fs})
    if c["grader"]["type"] == "must-not-match":
        with open(os.path.join(root, targets[0]), "a", encoding="utf-8") as fh:
            fh.write("\n" + c["grader"]["violation"] + "\n")
        return
    for rel in targets:
        path = os.path.join(root, rel)
        text = "#!/usr/bin/env bash\nexit 0\n" if rel.endswith(".sh") else ""
        with open(path, "w", encoding="utf-8") as fh:
            fh.write(text)


class RepositoryCaseTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.paths = run_evals.case_files(ROOT)
        cls.files = harness_lint.tree_files(ROOT)

    def copy_tree(self):
        dst = tempfile.mkdtemp()
        self.addCleanup(shutil.rmtree, dst, True)
        for rel in self.files:
            src = os.path.join(ROOT, rel)
            if os.path.isfile(src) and not os.path.islink(src):
                os.makedirs(os.path.dirname(os.path.join(dst, rel)), exist_ok=True)
                shutil.copy2(src, os.path.join(dst, rel))
        git(dst, "init", "-q", "-b", "main")
        git(dst, "add", "-A")
        return dst

    def test_there_are_seeded_cases(self):
        self.assertGreaterEqual(len(self.paths), 15)

    def test_every_case_is_well_formed_and_passes_here(self):
        for r in run_evals.run_all(ROOT, self.paths):
            with self.subTest(r["id"]):
                self.assertTrue(r["passed"], r["detail"])

    def test_every_case_goes_red_without_its_behaviour(self):
        for path in self.paths:
            c, problems = run_evals.load_case(path)
            with self.subTest(c["id"]):
                self.assertEqual(problems, [])
                tree = self.copy_tree()
                MUTATIONS.get(c["id"], default_mutation)(tree, c)
                r = run_evals.run_all(tree, [os.path.join(tree, os.path.relpath(path, ROOT))])[0]
                self.assertFalse(r["passed"], "%s still passes with its behaviour removed; add a mutation "
                                 "to MUTATIONS that removes it, or tighten the case" % c["id"])


if __name__ == "__main__":
    unittest.main()
