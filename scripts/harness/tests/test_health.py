"""Tests for scripts/harness/health.py. Offline: git runs against throwaway
repositories, and the stand-in gh from test_finalize serves fixture JSON."""

import io
import json
import os
import subprocess
import sys
import unittest
from contextlib import redirect_stderr, redirect_stdout
from unittest import mock

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
from test_finalize import RULES, GhBase, gh_run, git, threads, write  # noqa: E402

ROOT = os.path.abspath(os.path.join(HERE, "..", "..", ".."))
sys.path.insert(0, os.path.join(ROOT, "scripts", "harness"))
import gatelib  # noqa: E402
import health  # noqa: E402

NOW = 1767225600  # 2026-01-01T00:00:00Z
DAY = 86400


def call(argv):
    out, err = io.StringIO(), io.StringIO()
    with redirect_stdout(out), redirect_stderr(err):
        rc = health.main(argv)
    return rc, out.getvalue(), err.getvalue()


def by_name(results):
    return {r["name"]: r for r in results}


class Base(GhBase):
    def setUp(self):
        super().setUp()
        self.root = os.path.join(self.tmp, "repo")
        git(self.tmp, "init", "-q", "-b", "main", self.root)
        write(self.p("CLAUDE.md"), "x" * 100)
        write(self.p(".gitignore"), ".claude/data/\n")
        write(self.p(".claude/rules/a.md"), "# A\n")
        write(self.p(".claude/rules/b.md"), "---\npaths:\n  - \"x/**\"\n---\n# B\n")
        write(self.p(".claude/proposals/pending.md"), "# Pending proposals\n")
        self.commit("seed", NOW - 30 * DAY)
        self.root = os.path.realpath(self.root)

    def p(self, rel):
        return os.path.join(self.root, rel)

    def commit(self, msg, when):
        git(self.root, "add", "-A")
        stamp = "@%d +0000" % when
        subprocess.run(["git", "-C", self.root, "commit", "-qm", msg], check=True,
                       env=dict(os.environ, GIT_AUTHOR_DATE=stamp, GIT_COMMITTER_DATE=stamp))
        git(self.root, "update-ref", "refs/remotes/origin/main", "HEAD")

    def offline(self, **kw):
        return by_name(health.report(self.root, offline=True, now=NOW, **kw))


class LocalChecks(Base):
    def test_every_check_is_reported_offline(self):
        res = health.report(self.root, offline=True, now=NOW)
        self.assertEqual([r["name"] for r in res], ["budget", "evals", "proposals", "specs", "gate-markers",
                                                    "vendors", "required-checks", "main-ci", "open-prs",
                                                    "merged-branches"])
        self.assertEqual({r["name"] for r in res if r["status"] == "skipped"}, set(health.GITHUB_CHECKS))

    def test_budget_counts_only_always_on_text(self):
        with mock.patch.object(health.harness_lint, "RULE_BUDGET_BYTES", 1200):
            r = self.offline()["budget"]
        self.assertEqual((r["status"], r["summary"]), ("ok", "always-on 104 of 1200 bytes, headroom 1096"))
        with mock.patch.object(health.harness_lint, "RULE_BUDGET_BYTES", 1000):
            self.assertEqual(self.offline()["budget"]["status"], "warn")
        with mock.patch.object(health.harness_lint, "RULE_BUDGET_BYTES", 100):
            self.assertEqual(self.offline()["budget"]["status"], "fail")

    def test_budget_uses_larger_agent_entrypoint(self):
        write(self.p("AGENTS.md"), "a" * 200)
        with mock.patch.object(health.harness_lint, "RULE_BUDGET_BYTES", 200):
            r = self.offline()["budget"]
        self.assertEqual(r["status"], "fail")
        self.assertIn("always-on 204 of 200 bytes, headroom -4", r["summary"])

    def test_evals(self):
        self.assertEqual(self.offline()["evals"]["status"], "warn", "no cases")
        write(self.p(".claude/evals/cases/red.json"), json.dumps({
            "id": "red", "hazard": "h", "source": "s", "targets": ["CLAUDE.md"],
            "grader": {"type": "must-match", "pattern": "^absent$"}}))
        r = self.offline()["evals"]
        self.assertEqual((r["status"], r["summary"]), ("fail", "0 of 1 cases pass"))
        self.assertIn("red: pattern missing from CLAUDE.md", r["details"][0])

    def test_proposal_age(self):
        self.assertEqual(self.offline()["proposals"]["summary"], "none pending")
        with open(self.p(".claude/proposals/pending.md"), "a", encoding="utf-8") as fh:
            fh.write("\n## P-demo-1 — t\n\n- **Type**: rule\n- **Target**: `.claude/rules/a.md`\n")
        self.commit("propose", NOW - 10 * DAY)
        r = self.offline()["proposals"]
        self.assertEqual(r["status"], "warn")
        self.assertTrue(r["summary"].startswith("1 pending, oldest 10 days"))
        self.assertEqual(self.offline(stale_days=30)["proposals"]["status"], "ok")

    def test_stuck_specs(self):
        self.assertEqual(self.offline()["specs"]["status"], "ok")
        write(self.p("specs/in-progress/old/tasks.md"), "# t\n")
        self.commit("old", NOW - 9 * DAY)
        write(self.p("specs/unfinalized/new/tasks.md"), "# t\n")
        self.commit("new", NOW - 1 * DAY)
        write(self.p("specs/in-progress/draft/tasks.md"), "# t\n")
        r = self.offline()["specs"]
        self.assertEqual(r["status"], "warn")
        self.assertEqual(sorted(r["details"]), ["specs/in-progress/draft: never committed",
                                                "specs/in-progress/old: untouched for 9 days"])

    def test_gate_markers(self):
        self.assertEqual(self.offline()["gate-markers"]["summary"].split(",")[0], "0 fresh")
        marker = {"schema_version": 1, "gate": "harness-lint", "exit_code": 0, "tree": self.root,
                  "fingerprint": gatelib.fingerprint(self.root, "harness")}
        write(self.p(".claude/data/gate-marker-harness-lint.json"), json.dumps(marker))
        write(self.p(".claude/data/gate-marker-hygiene.json"), json.dumps(dict(marker, gate="hygiene")))
        r = self.offline()["gate-markers"]
        self.assertEqual(r["status"], "warn")
        self.assertTrue(r["summary"].startswith("1 fresh, 1 stale"), r)
        self.assertEqual(r["details"], ["%s: hygiene" % self.root])

    def test_vendors_report_opt_in(self):
        r = self.offline()["vendors"]
        self.assertEqual(r["status"], "ok")
        self.assertRegex(r["summary"], r"^\d of 3 opted in \(codex: o(n|ff)")

    def test_a_crashing_check_is_unknown(self):
        with mock.patch.object(health, "check_specs", side_effect=OSError("disk gone")):
            r = self.offline()["specs"]
        self.assertEqual((r["status"], r["summary"]), ("unknown", "could not run: disk gone"))


class GitHubChecks(Base):
    def serve_all(self, **over):
        sha = git(self.root, "rev-parse", "origin/main").strip()
        git(self.root, "branch", "feat/done")
        git(self.root, "branch", "feat/open")
        fx = {"api repos/acme/demo/rules/branches/main": RULES,
              "run list %s" % sha: [gh_run()],
              "pr list": {"__seq__": [[{"number": 7, "title": "open one", "isDraft": False}],
                                      [{"number": 5, "headRefName": "feat/done", "headRefOid": sha}]]},
              "api graphql 7": threads(resolved=False)}
        fx.update(over)
        self.serve(**{k.replace(" ", "_"): v for k, v in fx.items()})
        return sha

    def test_green_main_open_threads_and_merged_branches(self):
        sha = self.serve_all()
        r = by_name(health.report(self.root, repo="acme/demo", now=NOW))
        self.assertEqual(r["required-checks"]["status"], "ok")
        self.assertEqual((r["main-ci"]["status"], r["main-ci"]["summary"]), ("ok", "green at %s" % sha[:12]))
        self.assertEqual(r["open-prs"]["details"], ["#7 open one: 1 unresolved thread"])
        self.assertEqual(r["merged-branches"]["details"], ["feat/done: PR #5 merged"])
        self.assertEqual(r["merged-branches"]["status"], "warn")
        self.assertTrue(git(self.root, "branch", "--list", "feat/done").strip(), "nothing is deleted")

    def test_a_weakened_ruleset_fails(self):
        self.serve_all(**{"api repos/acme/demo/rules/branches/main": [RULES[0]]})
        r = by_name(health.report(self.root, repo="acme/demo", now=NOW))["required-checks"]
        self.assertEqual(r["status"], "fail")
        self.assertIn("missing from the ruleset: CI OK", r["details"])

    def test_red_main_is_classed(self):
        sha = git(self.root, "rev-parse", "origin/main").strip()
        self.serve_all(**{"run list %s" % sha: [gh_run(conclusion="failure", rid=9)],
                          "run view 9": {"jobs": [{"name": "Go", "conclusion": "failure",
                                                   "steps": [{"name": "Test", "conclusion": "failure"}]}]}})
        r = by_name(health.report(self.root, repo="acme/demo", now=NOW))["main-ci"]
        self.assertEqual(r["status"], "fail")
        self.assertIn("CI/Go class=real step=Test", r["details"][0])

    def test_gh_failures_are_unknown(self):
        self.serve()
        r = by_name(health.report(self.root, repo="acme/demo", now=NOW))
        for name in health.GITHUB_CHECKS:
            with self.subTest(name):
                self.assertEqual(r[name]["status"], "unknown")
                self.assertTrue(r[name]["summary"].startswith("could not run: "))


class CliTests(Base):
    def test_text_and_json(self):
        rc, out, _ = call(["--root", self.root, "--offline"])
        self.assertEqual(rc, 0)
        self.assertEqual(out.splitlines()[0], "Harness health")
        self.assertRegex(out.splitlines()[-1], r"^checks=10 ok=\d+ warn=\d+ fail=\d+ unknown=\d+ skipped=4$")
        rc, out, _ = call(["--root", self.root, "--offline", "--json"])
        data = json.loads(out)
        self.assertEqual((rc, len(data["checks"]), data["counts"]["skipped"]), (0, 10, 4))

    def test_not_a_checkout_is_a_usage_error(self):
        os.makedirs(os.path.join(self.tmp, "plain"))
        rc, _, err = call(["--root", os.path.join(self.tmp, "plain"), "--offline"])
        self.assertEqual(rc, 2)
        self.assertIn("health.py:", err)


if __name__ == "__main__":
    unittest.main()
