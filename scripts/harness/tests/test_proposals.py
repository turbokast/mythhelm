"""Tests for scripts/harness/proposals.py. Offline: git runs against throwaway
repositories, and the stand-in gh from test_finalize serves fixture JSON."""

import base64
import io
import json
import os
import subprocess
import sys
import textwrap
import unittest
from contextlib import redirect_stderr, redirect_stdout

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
from test_finalize import GhBase, Tmp, check, git, read, threads, write  # noqa: E402

ROOT = os.path.abspath(os.path.join(HERE, "..", "..", ".."))
sys.path.insert(0, os.path.join(ROOT, "scripts", "harness"))
import proposals  # noqa: E402

PENDING_HEAD = "# Pending proposals\n\nIntro.\n"


def proposal(pid, typ="rule", target=".claude/rules/demo.md", change="- Never do the thing.", new=False):
    return textwrap.dedent("""\

        ## {pid} — Add a guard

        - **Source spec**: `demo`
        - **Type**: {typ}
        - **Target**: `{target}`{new}
        - **Rationale**: it went wrong.
        - **Evidence**: PR #3.

        **Proposed change:**

        {change}
        """).format(pid=pid, typ=typ, target=target, new=" (new file)" if new else "", change=change)


def call(argv):
    out, err = io.StringIO(), io.StringIO()
    with redirect_stdout(out), redirect_stderr(err):
        rc = proposals.main(argv)
    return rc, out.getvalue(), err.getvalue()


class Repo(Tmp):
    """A repository whose origin/main holds pending proposals."""

    def setUp(self):
        super().setUp()
        self.root = os.path.join(self.tmp, "repo")
        git(self.tmp, "init", "-q", "-b", "main", self.root)
        write(os.path.join(self.root, "knowledge", "notes.md"), "# Notes\n\nFirst fact.\n")
        write(os.path.join(self.root, ".claude", "rules", "demo.md"), "# Demo\n\n- Do the right thing.\n")
        write(os.path.join(self.root, "specs", "done", "demo", "requirements.md"), "# Demo\n")

    def p(self, rel):
        return os.path.join(self.root, rel)

    def seed(self, *entries, auto=None, date=None):
        write(self.p(proposals.PENDING), PENDING_HEAD + "".join(entries))
        if auto is not None:
            write(self.p(proposals.AUTO_APPLY), json.dumps(auto))
        git(self.root, "add", "-A")
        env = dict(os.environ, GIT_AUTHOR_DATE=date) if date else None
        subprocess.run(["git", "-C", self.root, "commit", "-qm", "seed"], check=True, env=env)
        git(self.root, "update-ref", "refs/remotes/origin/main", "HEAD")


class ParseTests(Tmp):
    def test_sections_fields_and_fences(self):
        text = PENDING_HEAD + proposal("P-demo-1") + "\n```markdown\n## P-demo-9 — example\n- **Type**: hook\n```\n"
        pre, secs = proposals.sections(text)
        self.assertEqual(pre[0], "# Pending proposals")
        self.assertEqual([s["id"] for s in secs], ["P-demo-1"])
        self.assertEqual(secs[0]["title"], "Add a guard")
        self.assertEqual(proposals.ptype(secs[0]), "rule")
        self.assertEqual(proposals.target(secs[0]), (".claude/rules/demo.md", False))
        self.assertIn("\n## P-demo-9 — example\n", proposals.proposed_change(secs[0]))


class IdTests(Repo):
    def test_concurrent_sources_never_collide(self):
        self.seed(proposal("P-alpha-1"))
        sys.path.insert(0, os.path.join(ROOT, "scripts", "harness"))
        import finalize  # noqa: E402
        ids = {}
        for branch, spec in (("fin-alpha", "alpha"), ("fin-beta", "beta"), ("fin-alpha-again", "alpha")):
            git(self.root, "switch", "-q", "-c", branch, "main")
            ids[branch] = finalize.next_proposal_id(self.root, spec)
        self.assertEqual(ids, {"fin-alpha": "P-alpha-2", "fin-beta": "P-beta-1", "fin-alpha-again": "P-alpha-2"})
        # Two finalizes of different specs, both appended from the same base: the union is unique.
        union = PENDING_HEAD + proposal("P-alpha-1") + proposal(ids["fin-alpha"]) + proposal(ids["fin-beta"])
        self.assertEqual(len({s["id"] for s in proposals.sections(union)[1]}), 3)
        # A decided id is never handed out again.
        call(["record", "P-alpha-1", "--decision", "rejected", "--rationale", "no", "--root", self.root])
        self.assertEqual(finalize.next_proposal_id(self.root, "alpha"), "P-alpha-2")
        self.assertEqual(proposals.applied_problems(read(self.p(proposals.APPLIED)), {"P-alpha-1"})[0][1],
                         "P-alpha-1 is both pending and recorded")


class ListTests(Repo):
    def test_list_ages_from_blame(self):
        self.seed(proposal("P-demo-1"), date="2020-01-01T00:00:00Z")
        with open(self.p(proposals.PENDING), "a", encoding="utf-8") as fh:
            fh.write(proposal("P-demo-2", "knowledge", "knowledge/notes.md", "Second fact."))
        rows = proposals.pending_rows(self.root, now=1577836800 + 10 * 86400 + 5)
        self.assertEqual([(r["id"], r["age_days"]) for r in rows][0], ("P-demo-1", 10))
        self.assertEqual(rows[1]["id"], "P-demo-2")
        self.assertEqual(rows[1]["age_days"], 0, "an uncommitted proposal is new")
        count, oldest, _ = proposals.summary(self.root, now=1577836800 + 10 * 86400 + 5)
        self.assertEqual((count, oldest), (2, 10))

    def test_outside_git_the_age_is_unknown(self):
        write(os.path.join(self.tmp, "plain", proposals.PENDING), PENDING_HEAD + proposal("P-demo-1"))
        self.assertEqual(proposals.summary(os.path.join(self.tmp, "plain"))[:2], (1, None))
        rc, out, _ = call(["summary", "--root", os.path.join(self.tmp, "plain")])
        self.assertEqual((rc, out), (0, "1 unknown\n"))

    def test_notify_is_silent_without_proposals(self):
        self.seed()
        self.assertIsNone(proposals.notify(self.root))
        self.assertEqual(call(["notify", "--root", self.root])[1], "")

    def test_notify_line(self):
        self.seed(proposal("P-demo-1"), date="2020-01-01T00:00:00Z")
        out = json.loads(call(["notify", "--root", self.root])[1])
        self.assertRegex(out["systemMessage"], r"^Harness proposals: 1 pending \(oldest \d+ days\)\. "
                                               r"Decide them with /apply-proposals\.$")
        self.assertEqual(out["hookSpecificOutput"], {"hookEventName": "SessionStart",
                                                     "additionalContext": out["systemMessage"]})

    def test_show_and_list_cli(self):
        self.seed(proposal("P-demo-1"))
        rc, out, _ = call(["show", "P-demo-1", "--root", self.root])
        self.assertEqual(rc, 0)
        self.assertTrue(out.startswith("## P-demo-1 — Add a guard\n"))
        self.assertEqual(call(["show", "P-demo-7", "--root", self.root])[0], 2)
        rc, out, _ = call(["list", "--root", self.root, "--json"])
        self.assertEqual(json.loads(out)[0]["target"], ".claude/rules/demo.md")


ON = {"enabled": True, "allow": ["knowledge/notes.md"]}


class Lane0Tests(Repo):
    def decide(self, entry, auto=ON):
        self.seed(entry, auto=auto)
        return proposals.lane0_decision(self.root, proposals.pending_section(self.root, "P-demo-1"))

    def test_off_by_default_and_when_disabled(self):
        entry = proposal("P-demo-1", "knowledge", "knowledge/notes.md", "Second fact.")
        self.assertEqual(self.decide(entry, auto=None)[0], "NOT")
        self.assertIn("lane 0 is off", self.decide(entry, auto={"enabled": False, "allow": ["knowledge/**"]})[1])

    def test_eligible(self):
        self.assertEqual(self.decide(proposal("P-demo-1", "knowledge", "knowledge/notes.md", "Second fact.")),
                         ("ELIGIBLE", "knowledge/notes.md"))

    def test_each_refusal(self):
        cases = {
            "type 'rule' is not knowledge": proposal("P-demo-1"),
            "the target is not one existing file": proposal("P-demo-1", "knowledge", "knowledge/new.md", "x", new=True),
            "does not exist": proposal("P-demo-1", "knowledge", "knowledge/gone.md", "x"),
            "is not a markdown file under knowledge/": proposal("P-demo-1", "knowledge", "docs/x.md", "x"),
            "matches no allow glob": proposal("P-demo-1", "knowledge", "knowledge/notes.md", "x"),
            "carries proposal headings or fields": proposal("P-demo-1", "knowledge", "knowledge/notes.md",
                                                            "- **Type**: knowledge\nText."),
        }
        for needle, entry in cases.items():
            with self.subTest(needle):
                auto = {"enabled": True, "allow": ["knowledge/other.md"]} if needle.startswith("matches") else ON
                if needle == "is not a markdown file under knowledge/":
                    write(self.p("docs/x.md"), "x\n")
                verdict, why = self.decide(entry, auto)
                self.assertEqual(verdict, "NOT")
                self.assertIn(needle, why)

    def test_malformed_config(self):
        for bad, needle in (({"enabled": "yes", "allow": []}, "'enabled' is not true or false"),
                            ({"enabled": True, "allow": [".claude/rules/x.md"]}, "is not under knowledge/"),
                            ({"enabled": True}, "'allow' is not a list")):
            with self.subTest(needle):
                self.assertIn(needle, " ".join(proposals.auto_apply_problems(bad)))
        self.assertIn("not JSON", proposals.load_auto_apply("{")[1][0])

    def test_append_adds_only_the_change(self):
        self.seed(proposal("P-demo-1", "knowledge", "knowledge/notes.md", "Second fact."), auto=ON)
        rc, out, _ = call(["append", "P-demo-1", "--root", self.root])
        self.assertEqual((rc, out), (0, "appended knowledge/notes.md\n"))
        self.assertEqual(read(self.p("knowledge/notes.md")), "# Notes\n\nFirst fact.\n\nSecond fact.\n")
        self.assertEqual(call(["append", "P-demo-1", "--root", self.root])[0], 2, "a second append is refused")

    def test_append_refuses_an_ineligible_proposal(self):
        self.seed(proposal("P-demo-1", "knowledge", "knowledge/notes.md", "Second fact."))
        rc, out, _ = call(["append", "P-demo-1", "--root", self.root])
        self.assertEqual(rc, 1)
        self.assertIn("NOT lane 0 is off", out)
        self.assertEqual(read(self.p("knowledge/notes.md")), "# Notes\n\nFirst fact.\n")


class RecordTests(Repo):
    def rec(self, *args):
        return call(["record"] + list(args) + ["--root", self.root, "--date", "2026-01-02"])

    def test_approved_rule_needs_an_eval_or_a_waiver(self):
        self.seed(proposal("P-demo-1"))
        rc, _, err = self.rec("P-demo-1", "--decision", "approved", "--rationale", "yes")
        self.assertEqual(rc, 2)
        self.assertIn("needs --eval CASE", err)
        self.assertEqual(self.rec("P-demo-1", "--decision", "approved", "--rationale", "yes", "--eval", "a/b")[0], 2)
        rc, _, _ = self.rec("P-demo-1", "--decision", "approved", "--rationale", "yes", "--eval-waived", "no behaviour")
        self.assertEqual(rc, 0)
        entry = proposals.find(proposals.sections(read(self.p(proposals.APPLIED)))[1], "P-demo-1")
        self.assertEqual(entry["fields"]["Eval"], "waived — no behaviour")

    def test_record_moves_the_section(self):
        self.seed(proposal("P-demo-1"), proposal("P-demo-2", "knowledge", "knowledge/notes.md", "x"))
        rc, out, _ = self.rec("P-demo-1", "--decision", "approved", "--rationale", "Makes sense.", "--eval", "demo-guard")
        self.assertEqual((rc, out), (0, "recorded P-demo-1\n"))
        pending = read(self.p(proposals.PENDING))
        self.assertEqual(pending, PENDING_HEAD + proposal("P-demo-2", "knowledge", "knowledge/notes.md", "x"))
        applied = read(self.p(proposals.APPLIED))
        self.assertTrue(applied.startswith(proposals.APPLIED_HEAD.rstrip("\n") + "\n\n## P-demo-1 — Add a guard\n\n"
                                           "- **Decision**: approved\n- **Date**: 2026-01-02\n"
                                           "- **Pull request**: pending\n- **Eval**: `demo-guard`\n"
                                           "- **Rationale**: Makes sense.\n- **Source spec**: `demo`\n"), applied)
        self.assertTrue(applied.endswith("**Proposed change:**\n\n- Never do the thing.\n"))
        self.assertEqual(proposals.applied_problems(applied, case_ids={"demo-guard"}), [])

    def test_the_pull_request_is_filled_once(self):
        self.seed(proposal("P-demo-1", "knowledge", "knowledge/notes.md", "x"))
        self.assertEqual(self.rec("P-demo-1", "--decision", "rejected", "--rationale", "Not needed.")[0], 0)
        rc, out, _ = self.rec("P-demo-1", "--pr", "12")
        self.assertEqual((rc, out), (0, "filled P-demo-1\n"))
        self.assertIn("- **Pull request**: #12\n", read(self.p(proposals.APPLIED)))
        rc, _, err = self.rec("P-demo-1", "--pr", "13")
        self.assertEqual(rc, 2)
        self.assertIn("an entry is never edited", err)

    def test_refusals(self):
        self.seed(proposal("P-demo-1", "knowledge", "knowledge/notes.md", "x"))
        cases = [
            ("--rationale is required", ["--decision", "rejected"]),
            ("an eval is recorded only for", ["--decision", "approved", "--rationale", "r", "--eval", "c"]),
            ("is not lane-0 eligible", ["--decision", "auto-applied"]),
            ("record needs --decision", []),
        ]
        for needle, args in cases:
            with self.subTest(needle):
                rc, _, err = self.rec("P-demo-1", *args)
                self.assertEqual(rc, 2)
                self.assertIn(needle, err)
        self.assertIn("is not in", self.rec("P-demo-5", "--decision", "rejected", "--rationale", "r")[2])

    def test_applied_problems(self):
        good = self.entry()
        self.assertEqual(proposals.applied_problems(good, case_ids={"c"}), [])
        cases = {
            "is recorded twice": good + good.split("\n", 2)[2].join(["\n## P-demo-1 — again\n\n", ""]),
            "is both pending and recorded": good,
            "Decision 'maybe' is not one of": good.replace("approved", "maybe"),
            "Date is not YYYY-MM-DD": good.replace("2026-01-02", "soon"),
            "Pull request is not '#<n>'": good.replace("#4", "4"),
            "has no Rationale": good.replace("- **Rationale**: r\n", ""),
            "records an eval case or a waiver": good.replace("`c`", "none"),
            "eval case c has no file": good,
            "Eval is 'n/a' unless": good.replace("rule", "knowledge"),
            "is not '## P-<spec>-<n>": good.replace("## P-demo-1", "## Something"),
        }
        for needle, text in cases.items():
            with self.subTest(needle):
                pend = {"P-demo-1"} if needle == "is both pending and recorded" else ()
                ids = set() if needle == "eval case c has no file" else {"c"}
                got = " | ".join(d for _, d in proposals.applied_problems(text, pend, ids))
                self.assertIn(needle, got)

    def entry(self):
        return ("# Applied\n\n## P-demo-1 — t\n\n- **Decision**: approved\n- **Date**: 2026-01-02\n"
                "- **Pull request**: #4\n- **Eval**: `c`\n- **Rationale**: r\n- **Type**: rule\n")


GUARD_CASE = {"id": "demo-guard", "hazard": "the demo rule loses its guard", "source": "P-demo-1",
              "targets": [".claude/rules/demo.md"], "grader": {"type": "must-match", "pattern": r"^- Never do the thing\.$"}}


class ApplyCheckTests(Repo):
    def apply_rule(self, case=GUARD_CASE, eval_id="demo-guard"):
        self.seed(proposal("P-demo-1"))
        git(self.root, "switch", "-q", "-c", "harness/proposal-p-demo-1")
        with open(self.p(".claude/rules/demo.md"), "a", encoding="utf-8") as fh:
            fh.write("- Never do the thing.\n")
        if case is not None:
            write(self.p(proposals.CASES + case["id"] + ".json"), json.dumps(case))
        args = ["--eval", eval_id] if eval_id else ["--eval-waived", "prose only"]
        self.assertEqual(call(["record", "P-demo-1", "--decision", "approved", "--rationale", "ok", "--root", self.root]
                              + args)[0], 0)

    def check(self):
        return call(["apply-check", "P-demo-1", "--root", self.root])

    def test_a_pinned_rule_is_ready(self):
        self.apply_rule()
        rc, out, _ = self.check()
        self.assertEqual(rc, 0, out)
        self.assertIn("eval_at_base=fail", out)
        self.assertEqual(out.splitlines()[-1], "verdict=ready")
        self.assertEqual(git(self.root, "worktree", "list").count("\n"), 1, "the base worktree is removed")

    def test_a_case_that_passes_at_the_base_pins_nothing(self):
        weak = dict(GUARD_CASE, grader={"type": "must-match", "pattern": "^# Demo$"})
        self.apply_rule(weak)
        rc, out, _ = self.check()
        self.assertEqual(rc, 1)
        self.assertIn("already passes at the base", out)

    def test_a_case_that_fails_here_or_names_another_source(self):
        self.apply_rule(dict(GUARD_CASE, source="P-other-1", grader={"type": "must-match", "pattern": "^absent$"}))
        rc, out, _ = self.check()
        self.assertEqual(rc, 1)
        self.assertIn("the case's source does not name P-demo-1", out)
        self.assertIn("eval: demo-guard fails on this tree", out)

    def test_a_missing_case_file(self):
        self.apply_rule(case=None)
        rc, out, _ = self.check()
        self.assertIn(".claude/evals/cases/demo-guard.json does not exist", out)

    def test_a_waiver_skips_the_eval(self):
        self.apply_rule(case=None, eval_id=None)
        rc, out, _ = self.check()
        self.assertEqual(rc, 0, out)
        self.assertIn("eval=waived", out)

    def test_scope_and_append_only(self):
        self.apply_rule()
        write(self.p("knowledge/notes.md"), "changed\n")
        write(self.p("knowledge/rule-evidence/demo.md"), "# Evidence: demo\n")
        rc, out, _ = self.check()
        self.assertEqual(rc, 1)
        self.assertIn("scope: knowledge/notes.md is outside", out)
        self.assertNotIn("rule-evidence", out, "a rule may add its evidence file")

    def test_an_edited_applied_entry_is_refused(self):
        self.seed(proposal("P-demo-1"), proposal("P-demo-2", "knowledge", "knowledge/notes.md", "x"))
        call(["record", "P-demo-2", "--decision", "rejected", "--rationale", "no", "--pr", "3", "--root", self.root])
        git(self.root, "add", "-A")
        git(self.root, "commit", "-qm", "reject 2")
        git(self.root, "update-ref", "refs/remotes/origin/main", "HEAD")
        git(self.root, "switch", "-q", "-c", "b")
        call(["record", "P-demo-1", "--decision", "rejected", "--rationale", "no", "--root", self.root])
        text = read(self.p(proposals.APPLIED)).replace("- **Rationale**: no\n", "- **Rationale**: rewritten\n", 1)
        write(self.p(proposals.APPLIED), text)
        rc, out, _ = self.check()
        self.assertIn("append-only:", out)

    def test_still_pending(self):
        self.seed(proposal("P-demo-1"))
        rc, out, _ = self.check()
        self.assertEqual(rc, 1)
        self.assertIn("entry: P-demo-1 is not recorded", out)

    def test_lane0_is_judged_at_the_base(self):
        self.seed(proposal("P-demo-1", "knowledge", "knowledge/notes.md", "Second fact."))
        git(self.root, "switch", "-q", "-c", "lane0")
        write(self.p(proposals.AUTO_APPLY), json.dumps(ON))
        self.assertEqual(call(["append", "P-demo-1", "--root", self.root])[0], 0)
        self.assertEqual(call(["record", "P-demo-1", "--decision", "auto-applied", "--root", self.root])[0], 0)
        rc, out, _ = self.check()
        self.assertEqual(rc, 1)
        self.assertIn("lane0: P-demo-1 at the base: lane 0 is off", out)
        self.assertIn("scope: .claude/proposals/auto-apply.json is outside", out)

    def test_lane0_ready_when_enabled_at_the_base(self):
        self.seed(proposal("P-demo-1", "knowledge", "knowledge/notes.md", "Second fact."), auto=ON)
        git(self.root, "switch", "-q", "-c", "lane0")
        self.assertEqual(call(["append", "P-demo-1", "--root", self.root])[0], 0)
        self.assertEqual(call(["record", "P-demo-1", "--decision", "auto-applied", "--root", self.root])[0], 0)
        rc, out, _ = self.check()
        self.assertEqual(rc, 0, out)
        write(self.p("knowledge/notes.md"), "# Notes\n\nRewritten.\n\nSecond fact.\n")
        rc, out, _ = self.check()
        self.assertIn("is not a pure append", out)


class PrCheckTests(GhBase):
    APPLIED = ("# Applied\n\n## P-demo-1 — Add a guard\n\n- **Decision**: approved\n- **Date**: 2026-01-02\n"
               "- **Pull request**: #50\n- **Eval**: `demo-guard`\n- **Rationale**: ok\n- **Source spec**: `demo`\n"
               "- **Type**: rule\n- **Target**: `.claude/rules/demo.md`\n")
    FILES = [".claude/rules/demo.md", ".claude/evals/cases/demo-guard.json", proposals.PENDING, proposals.APPLIED]

    def serve_pr(self, files=None, applied=None, pending=PENDING_HEAD, **over):
        view = {"state": "OPEN", "isDraft": False, "mergeable": "MERGEABLE", "mergeStateStatus": "CLEAN",
                "headRefOid": "0123abc", "title": "harness: apply P-demo-1", "body": "Apply P-demo-1.",
                "files": [{"path": p} for p in (self.FILES if files is None else files)],
                "statusCheckRollup": [check("CI OK"), check("CodeRabbit")]}
        view.update(over.pop("view", {}))

        def content(text):
            return {"content": base64.b64encode(text.encode()).decode()}
        fx = {"pr view 50": view, "api graphql 50": threads(), "pr diff 50": "+++ b/x\n+- Never do the thing.\n",
              "api repos/acme/demo/contents/%s?ref=0123abc" % proposals.APPLIED: content(applied or self.APPLIED),
              "api repos/acme/demo/contents/%s?ref=0123abc" % proposals.PENDING: content(pending)}
        fx.update(over)
        self.serve(**{k.replace(" ", "_"): v for k, v in fx.items()})

    def pc(self):
        return call(["pr-check", "P-demo-1", "--pr", "50", "--repo", "acme/demo"])

    def test_ready(self):
        self.serve_pr()
        rc, out, _ = self.pc()
        self.assertEqual((rc, out.splitlines()[-1]), (0, "verdict=ready"), out)

    def test_findings(self):
        cases = [
            ("scope: internal/x.go is outside", {"files": self.FILES + ["internal/x.go"]}),
            ("records pull request pending, not #50", {"applied": self.APPLIED.replace("#50", "pending")}),
            ("is still in .claude/proposals/pending.md", {"pending": PENDING_HEAD + proposal("P-demo-1")}),
            ("is not recorded in .claude/proposals/applied.md", {"applied": "# Applied\n"}),
            ("thread-unresolved:", {"api graphql 50": threads(resolved=False)}),
        ]
        for needle, over in cases:
            with self.subTest(needle):
                self.serve_pr(**over)
                rc, out, _ = self.pc()
                self.assertEqual(rc, 1, out)
                self.assertIn(needle, out)


if __name__ == "__main__":
    unittest.main()
