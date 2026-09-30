"""Tests for scripts/orchestration/autonomy.py. Offline: every grant is made in a
throwaway repository, a pseudo-terminal stands in for the maintainer's terminal, and
a stand-in gh serves fixture JSON to the merge check."""

import json
import os
import shutil
import subprocess
import sys
import tempfile
import textwrap
import time
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.abspath(os.path.join(HERE, "..", "..", ".."))
AUTONOMY = os.path.join(ROOT, "scripts", "orchestration", "autonomy.py")
PTY = "import pty, sys; sys.exit(pty.spawn(sys.argv[1:]) >> 8)"

BACKLOG = textwrap.dedent("""\
    # Backlog

    Rubric text.

    ## Open

    ### MH-1: Alpha
    - **Status**: specced
    - **Stage**: 1
    - **Spec**: `alpha`
    - **Summary**: The alpha card.

    ### MH-2: Beta
    - **Status**: implementing
    - **Stage**: 1
    - **Spec**: `beta`
    - **Summary**: The beta card.

    ## Closed

    No closed cards.
    """)

DECISIONS = textwrap.dedent("""\
    # Decisions

    ### D-1 — 2026-01-01: Seed
    - **Type**: backlog-seed
    - **Decision**: seeded
    - **Rationale**: start
    - **Cards**: MH-1
    """)

FAKE_GH = r'''#!/usr/bin/env python3
import json, os, sys
fx = json.load(open(os.environ["FAKE_GH_FIXTURE"]))
args = [a for a in sys.argv[1:]]
for flag in ("-R", "--repo"):
    if flag in args:
        i = args.index(flag); del args[i:i + 2]
key = "api graphql" if args[:2] == ["api", "graphql"] else " ".join(args[:2])
out = fx.get(key)
if out is None:
    sys.stderr.write("fake gh: no fixture for %s\n" % key); sys.exit(1)
sys.stdout.write(out if isinstance(out, str) else json.dumps(out))
'''

HEAD = "0123456789abcdef0123456789abcdef01234567"


def git(*args, cwd):
    subprocess.run(["git"] + list(args), cwd=cwd, check=True, capture_output=True)


class Base(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.mkdtemp()
        self.addCleanup(shutil.rmtree, self.tmp)
        self.repo = os.path.join(self.tmp, "repo")
        os.makedirs(os.path.join(self.repo, "product"))
        git("init", "-q", "-b", "main", cwd=self.repo)
        self.write("product/backlog.md", BACKLOG)
        self.write("product/decisions.md", DECISIONS)
        self.env = {k: v for k, v in os.environ.items() if k not in ("CLAUDE_CODE_SESSION_ID", "GIT_DIR")}
        self.env.update(GIT_CONFIG_NOSYSTEM="1", GIT_CONFIG_GLOBAL=os.devnull)

    def write(self, rel, text):
        path = os.path.join(self.repo, rel)
        os.makedirs(os.path.dirname(path), exist_ok=True)
        with open(path, "w", encoding="utf-8") as fh:
            fh.write(text)
        return path

    def run_cmd(self, *args, tty=False):
        cmd = [sys.executable, AUTONOMY] + list(args)
        if tty:
            cmd = [sys.executable, "-c", PTY] + cmd
        r = subprocess.run(cmd, cwd=self.repo, env=self.env, capture_output=True, text=True,
                           stdin=subprocess.DEVNULL)
        return r.returncode, r.stdout + r.stderr

    @property
    def grant_path(self):
        return os.path.join(self.repo, ".claude", "data", "autonomy-grant.json")

    def grant(self):
        with open(self.grant_path, encoding="utf-8") as fh:
            return json.load(fh)

    def audit(self):
        path = os.path.join(self.repo, ".claude", "data", "autonomy-audit.jsonl")
        if not os.path.exists(path):
            return []
        with open(path, encoding="utf-8") as fh:
            return [json.loads(line) for line in fh if line.strip()]

    def grant_now(self, *extra, scope="MH-1,spec:gamma"):
        rc, out = self.run_cmd("grant", "--hours", "4", "--scope", scope, "--reason", "overnight run",
                               "--session", "sess-1", *extra, tty=True)
        self.assertEqual(rc, 0, out)
        return self.grant()

    def forge(self, **over):
        now = int(time.time())
        g = {"schema_version": 1, "id": "abc123", "session_id": "sess-1", "issued_epoch": now - 60,
             "until_epoch": now + 3600, "until": "2099-01-01T00:00:00Z", "granted_at": "x",
             "scope": {"cards": ["MH-1"], "specs": ["gamma"]}, "allow_pm_sync": True, "spec_checkpoint": True,
             "max_continues": 5, "reason": "r", "granted_by": "terminal:t", "renewals": 0}
        g.update(over)
        os.makedirs(os.path.dirname(self.grant_path), exist_ok=True)
        with open(self.grant_path, "w", encoding="utf-8") as fh:
            json.dump(g, fh)
        return g


class GrantTest(Base):
    def test_grant_refused_without_a_terminal(self):
        rc, out = self.run_cmd("grant", "--hours", "4", "--scope", "MH-1", "--reason", "r", "--session", "s1")
        self.assertEqual(rc, 2)
        self.assertIn("interactive terminal", out)
        self.assertFalse(os.path.exists(self.grant_path))

    def test_grant_from_a_terminal_binds_session_scope_and_deadline(self):
        before = int(time.time())
        g = self.grant_now("--allow-pm-sync")
        self.assertEqual(g["session_id"], "sess-1")
        self.assertEqual(g["scope"], {"cards": ["MH-1"], "specs": ["gamma"]})
        self.assertTrue(g["allow_pm_sync"])
        self.assertTrue(g["spec_checkpoint"])
        self.assertTrue(before + 4 * 3600 <= g["until_epoch"] <= int(time.time()) + 4 * 3600)
        rows = self.audit()
        self.assertEqual([r["event"] for r in rows], ["grant"])
        self.assertEqual(rows[0]["reason"], "overnight run")

    def test_session_from_the_environment(self):
        self.env["CLAUDE_CODE_SESSION_ID"] = "env-session"
        rc, out = self.run_cmd("grant", "--hours", "1", "--scope", "spec:gamma", "--reason", "r", tty=True)
        self.assertEqual(rc, 0, out)
        self.assertEqual(self.grant()["session_id"], "env-session")

    def test_bounds_and_inputs_refused_before_anything_is_written(self):
        cases = [
            (["--hours", "0", "--scope", "MH-1"], "--hours"),
            (["--hours", "25", "--scope", "MH-1"], "--hours"),
            (["--hours", "8760", "--scope", "MH-1"], "--hours"),
            (["--hours", "8h", "--scope", "MH-1"], "--hours"),
            (["--hours", "4", "--scope", "MH-1;rm"], "neither"),
            (["--hours", "4", "--scope", ","], "names no card"),
            (["--hours", "4", "--scope", "MH-9"], "no such card"),
            (["--hours", "4", "--scope", "MH-1", "--max-continues", "999"], "--max-continues"),
        ]
        for args, needle in cases:
            rc, out = self.run_cmd("grant", *args, "--reason", "r", "--session", "s1", tty=True)
            self.assertEqual(rc, 2, args)
            self.assertIn(needle, out, args)
        rc, out = self.run_cmd("grant", "--hours", "4", "--scope", "MH-1", "--reason", "r", tty=True)
        self.assertEqual(rc, 2)
        self.assertIn("no session to bind", out)
        self.assertFalse(os.path.exists(self.grant_path))

    def test_renew_needs_a_terminal_and_keeps_scope_and_session(self):
        g = self.grant_now()
        rc, out = self.run_cmd("renew", "--hours", "2")
        self.assertEqual(rc, 2)
        self.assertEqual(self.grant(), g)
        rc, out = self.run_cmd("renew", "--hours", "24", tty=True)
        self.assertEqual(rc, 0, out)
        r = self.grant()
        self.assertEqual((r["id"], r["session_id"], r["scope"]), (g["id"], g["session_id"], g["scope"]))
        self.assertEqual(r["renewals"], 1)
        self.assertLessEqual(r["until_epoch"] - r["issued_epoch"], 24 * 3600)
        rc, out = self.run_cmd("renew", "--hours", "25", tty=True)
        self.assertEqual(rc, 2)

    def test_renew_with_no_grant_is_refused(self):
        rc, out = self.run_cmd("renew", "--hours", "2", tty=True)
        self.assertEqual(rc, 2)
        self.assertIn("no valid grant", out)

    def test_revoke_is_open_to_anyone_and_audited(self):
        self.grant_now()
        rc, out = self.run_cmd("revoke", "--reason", "done")
        self.assertEqual(rc, 0, out)
        self.assertFalse(os.path.exists(self.grant_path))
        self.assertEqual([r["event"] for r in self.audit()], ["grant", "revoke"])


class CheckTest(Base):
    def test_covers_session_and_scope(self):
        self.grant_now()
        self.assertEqual(self.run_cmd("check", "--session", "sess-1")[0], 0)
        self.assertEqual(self.run_cmd("check", "--session", "sess-1", "--spec", "alpha")[0], 0)
        self.assertEqual(self.run_cmd("check", "--session", "sess-1", "--spec", "gamma")[0], 0)
        rc, out = self.run_cmd("check", "--session", "sess-1", "--spec", "beta")
        self.assertEqual(rc, 1)
        self.assertIn("not in the grant's scope", out)
        rc, out = self.run_cmd("check", "--session", "other")
        self.assertEqual(rc, 1)
        self.assertIn("binds session sess-1", out)

    def test_expired_invalid_and_absent_grants_cover_nothing(self):
        self.assertEqual(self.run_cmd("check", "--session", "sess-1")[0], 1)
        now = int(time.time())
        self.forge(issued_epoch=now - 7200, until_epoch=now - 60)
        rc, out = self.run_cmd("check", "--session", "sess-1")
        self.assertEqual(rc, 1)
        self.assertIn("expired", out)
        self.forge(issued_epoch=now, until_epoch=now + 25 * 3600)
        rc, out = self.run_cmd("check", "--session", "sess-1")
        self.assertEqual(rc, 1)
        self.assertIn("invalid", out)
        self.forge(session_id="-rf")
        self.assertEqual(self.run_cmd("check", "--session=-rf")[0], 1)


class MergeCheckTest(Base):
    def setUp(self):
        super().setUp()
        self.bin = os.path.join(self.tmp, "bin")
        os.makedirs(self.bin)
        path = os.path.join(self.bin, "gh")
        with open(path, "w", encoding="utf-8") as fh:
            fh.write(FAKE_GH)
        os.chmod(path, 0o755)
        self.env["PATH"] = self.bin + os.pathsep + self.env["PATH"]
        self.env["FAKE_GH_FIXTURE"] = os.path.join(self.tmp, "fx.json")

    def serve(self, threads=()):
        ok = {"__typename": "CheckRun", "status": "COMPLETED", "conclusion": "SUCCESS",
              "startedAt": "2026-01-01T00:00:00Z", "workflowName": "wf"}
        fx = {
            "pr view": {"state": "OPEN", "isDraft": False, "mergeable": "MERGEABLE", "mergeStateStatus": "CLEAN",
                        "headRefName": "docs/gamma-start", "headRefOid": HEAD, "title": "docs(spec): start gamma",
                        "body": "Lifecycle move.", "files": [{"path": "specs/in-progress/gamma/handoff.md"}],
                        "statusCheckRollup": [dict(ok, name="CI OK"), dict(ok, name="CodeRabbit")]},
            "api graphql": {"data": {"repository": {"pullRequest": {"reviewThreads": {
                "totalCount": len(threads), "nodes": [{"isResolved": False, "comments": {"nodes": [{"url": u}]}}
                                                      for u in threads]}}}}},
            "pr diff": "+++ b/specs/in-progress/gamma/handoff.md\n+notes\n",
        }
        with open(self.env["FAKE_GH_FIXTURE"], "w", encoding="utf-8") as fh:
            json.dump(fx, fh)

    def test_ready_pull_request_of_a_granted_spec_is_recorded(self):
        self.grant_now()
        self.serve()
        rc, out = self.run_cmd("merge-check", "--pr", "7", "--spec", "gamma", "--lifecycle", "--repo", "acme/demo",
                               "--session", "sess-1")
        self.assertEqual(rc, 0, out)
        self.assertIn("--match-head-commit %s" % HEAD, out)
        rows = [r for r in self.audit() if r["event"] == "merge-ready"]
        self.assertEqual(len(rows), 1)
        self.assertEqual((rows[0]["pr"], rows[0]["head"], rows[0]["spec"]), (7, HEAD, "gamma"))
        self.assertEqual(rows[0]["grant_id"], self.grant()["id"])

    def test_unresolved_thread_is_not_ready_and_records_nothing(self):
        self.grant_now()
        self.serve(threads=["https://example.com/thread/1"])
        rc, out = self.run_cmd("merge-check", "--pr", "7", "--spec", "gamma", "--lifecycle", "--repo", "acme/demo")
        self.assertEqual(rc, 1, out)
        self.assertIn("thread-unresolved", out)
        self.assertEqual([r for r in self.audit() if r["event"] == "merge-ready"], [])

    def test_spec_outside_the_grant_is_refused_before_github_is_asked(self):
        self.grant_now()
        rc, out = self.run_cmd("merge-check", "--pr", "7", "--spec", "beta", "--lifecycle", "--repo", "acme/demo")
        self.assertEqual(rc, 1)
        self.assertIn("not in the grant's scope", out)

    def test_no_grant_no_merge(self):
        self.serve()
        rc, out = self.run_cmd("merge-check", "--pr", "7", "--spec", "gamma", "--lifecycle", "--repo", "acme/demo")
        self.assertEqual(rc, 1)
        self.assertIn("no autonomy grant", out)


class PmSyncTest(Base):
    def check(self, rel, text, **grant):
        self.forge(**grant)
        proposed = os.path.join(self.tmp, "proposed")
        with open(proposed, "w", encoding="utf-8") as fh:
            fh.write(text)
        return self.run_cmd("pm-sync-check", "--path", rel, "--proposed", proposed, "--session", "sess-1")

    def moved(self, card_status, closed=False):
        text = BACKLOG.replace("- **Status**: specced", "- **Status**: %s" % card_status, 1)
        if closed:
            block = text[text.index("### MH-1"):text.index("### MH-2")]
            text = text.replace(block, "").replace("No closed cards.", block.rstrip("\n"))
        return text

    def test_granted_card_moves_one_lifecycle_step(self):
        rc, out = self.check("product/backlog.md", self.moved("implementing"))
        self.assertEqual(rc, 0, out)

    def test_shipped_card_moving_to_the_closed_section(self):
        self.write("product/backlog.md", BACKLOG.replace("- **Status**: specced", "- **Status**: implementing", 1))
        rc, out = self.check("product/backlog.md", self.moved("shipped", closed=True))
        self.assertEqual(rc, 0, out)

    def test_refusals(self):
        cases = [
            (self.moved("shipped"), {}, "not a pre-approved step"),
            (self.moved("dropped"), {}, "not a pre-approved step"),
            (BACKLOG.replace("- **Status**: implementing", "- **Status**: shipped"), {}, "MH-2 changed"),
            (self.moved("implementing").replace("The alpha card.", "Reworded."), {}, "field Summary"),
            (self.moved("implementing").replace("Rubric text.", "Other rubric."), {}, "outside the cards"),
            (self.moved("implementing"), {"allow_pm_sync": False}, "does not pre-approve"),
            (BACKLOG, {}, "nothing changed"),
        ]
        for text, grant, needle in cases:
            rc, out = self.check("product/backlog.md", text, **grant)
            self.assertEqual(rc, 1, needle)
            self.assertIn(needle, out)
        rc, out = self.check("product/backlog.md", self.moved("implementing"), session_id="sess-2")
        self.assertEqual(rc, 1)
        self.assertIn("binds session", out)

    def test_decisions_only_gain_lifecycle_sync_entries_for_granted_cards(self):
        entry = ("\n### D-2 — 2026-01-02: MH-1 → implementing (alpha)\n- **Type**: lifecycle-sync\n"
                 "- **Decision**: moved\n- **Rationale**: alpha started\n- **Cards**: %s\n")
        rc, out = self.check("product/decisions.md", DECISIONS + entry % "MH-1")
        self.assertEqual(rc, 0, out)
        rc, out = self.check("product/decisions.md", DECISIONS + entry % "MH-2")
        self.assertEqual(rc, 1)
        rc, out = self.check("product/decisions.md", DECISIONS + entry.replace("lifecycle-sync", "rescore") % "MH-1")
        self.assertIn("not of type lifecycle-sync", out)
        rc, out = self.check("product/decisions.md", DECISIONS.replace("seeded", "rewritten") + entry % "MH-1")
        self.assertIn("only be appended", out)

    def test_roadmap_must_be_the_generated_one(self):
        self.write("product/roadmap.md", "# Roadmap\n\nold\n")
        rc, out = self.check("product/roadmap.md", "# Roadmap\n")
        self.assertIn("not present", out)
        self.write("scripts/pm/pm.py", textwrap.dedent("""\
            import sys
            out = sys.argv[sys.argv.index("--out") + 1]
            open(out, "w").write("# Roadmap\\n\\ngenerated\\n")
            """))
        rc, out = self.check("product/roadmap.md", "# Roadmap\n\ngenerated\n")
        self.assertEqual(rc, 0, out)
        rc, out = self.check("product/roadmap.md", "# Roadmap\n\nhand-written\n")
        self.assertEqual(rc, 1)
        self.assertIn("not the roadmap generated", out)

    def test_other_product_files_are_never_pre_approved(self):
        self.write("product/objectives.md", "# Objectives\n")
        rc, out = self.check("product/objectives.md", "# Objectives\n\nchanged\n")
        self.assertEqual(rc, 1)
        self.assertIn("not a file the lifecycle sync writes", out)


if __name__ == "__main__":
    unittest.main()
