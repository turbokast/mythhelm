"""Tests for scripts/harness/runspec.py. Offline: git runs against throwaway
repositories, and a stand-in gh on PATH serves fixture JSON."""

import base64
import io
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import textwrap
import unittest
from contextlib import redirect_stdout

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.abspath(os.path.join(HERE, "..", "..", ".."))
sys.path.insert(0, os.path.join(ROOT, "scripts", "harness"))
import runspec  # noqa: E402

TASKS = textwrap.dedent("""\
    ## Demo — Tasks

    ### Dependencies

    - Tasks are in dependency order.

    ---

    ### Task 1 — Module skeleton ✅ COMPLETED

    - **Domain/agent**: go-implementer
    - **Budget**: standard
    - **Change**: Create the module.
    - **Files**:
      - `go.mod`
      - `internal/cli/dispatch.go`
      - `CONTRIBUTING.md` (fill the setup section, see `docs/x.md`)
    - **Acceptance**:
      - `TestDispatch` passes.
    - **Invariants touched**: G01.
    - **Status**: ✅ Completed — module and dispatcher; PR #7.
    - **Implementation**: `cli.Main` dispatches through a map. Commit abc1234.
    - **Spec deviations**: None.
    - **Files modified**: `go.mod`, `internal/cli/dispatch.go`, `specs/todo/demo/tasks.md`.

    ### Task 2 — CI gates

    - **Domain/agent**: release-engineer
    - **Budget**: standard
    - **Depends on**: Task 1
    - **Change**: Add lint.
    - **Files**:
      - `.golangci.yml`
      - `.github/workflows/ci.yml` (new `lint` job)
    - **Acceptance**:
      - lint passes.
    - **Invariants touched**: G10.

    ### Task 3 — Journal

    - **Domain/agent**: go-implementer
    - **Budget**: complex
    - **Depends on**: Task 1
    - **Change**: Journal.
    - **Files**:
      - `internal/journal/journal.go`
      - `go.mod`, `go.sum`
    - **Acceptance**:
      - tests pass.
    - **Invariants touched**: I09.

    ### Task 4 — Security

    - **Domain/agent**: go-implementer
    - **Budget**: standard
    - **Depends on**: Task 1
    - **Change**: Security.
    - **Files**:
      - `internal/security/*.go`
    - **Acceptance**:
      - tests pass.
    - **Invariants touched**: I19.
    - **Status**: ✅ Completed. Security primitives; PR #9

    ### Task 5 — Pipeline

    - **Domain/agent**: go-implementer
    - **Budget**: complex
    - **Depends on**: Task 3, Task 4 (after both land)
    - **Change**: Pipeline.
    - **Files**:
      - `internal/security/env.go`
    - **Acceptance**:
      - tests pass.
    - **Invariants touched**: I06.

    ### Task 6 — Docs

    - **Domain/agent**: go-implementer
    - **Budget**: standard
    - **Change**: Docs.
    - **Files**:
      - `docs/`
    - **Acceptance**:
      - reads well.
    - **Invariants touched**: none.
    """)


def git(cwd, *args):
    return subprocess.run(["git", "-C", cwd] + list(args), check=True, capture_output=True, text=True,
                          env=runspec.git_env()).stdout


def new_repo(path):
    os.makedirs(path, exist_ok=True)
    git(path, "init", "-q", "-b", "main")
    git(path, "config", "user.email", "test@example.com")
    git(path, "config", "user.name", "test")
    git(path, "config", "commit.gpgsign", "false")


def read(path):
    with open(path, encoding="utf-8") as fh:
        return fh.read()


def write(path, text):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "w", encoding="utf-8") as fh:
        fh.write(text)


class Tmp(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.mkdtemp()
        self.addCleanup(shutil.rmtree, self.tmp, True)
        os.environ["GIT_CONFIG_GLOBAL"] = "/dev/null"
        os.environ["GIT_CONFIG_NOSYSTEM"] = "1"


class ParseTests(unittest.TestCase):
    def setUp(self):
        self.tasks = runspec.parse_tasks(TASKS)
        self.idx = runspec.by_number(self.tasks)

    def test_fields_and_markers(self):
        t1 = self.idx[1]
        self.assertEqual(t1["name"], "Module skeleton")
        self.assertTrue(t1["heading_complete"] and t1["status_complete"])
        self.assertEqual(t1["pr"], 7)
        self.assertEqual(t1["agent"], "go-implementer")
        self.assertEqual(t1["files"], ["go.mod", "internal/cli/dispatch.go", "CONTRIBUTING.md"])
        self.assertEqual(self.idx[3]["files"], ["internal/journal/journal.go", "go.mod", "go.sum"])
        self.assertEqual(self.idx[2]["files"], [".golangci.yml", ".github/workflows/ci.yml"])

    def test_dependencies_explicit_noted_and_implicit(self):
        self.assertEqual(self.idx[1]["depends"], [])
        self.assertEqual(self.idx[5]["depends"], [3, 4])
        self.assertEqual(self.idx[6]["depends"], [5], "no Depends on: the previous task")

    def test_status_only_completion_is_a_marker_mismatch(self):
        p = runspec.progress(self.tasks)
        self.assertTrue(self.idx[4]["complete"])
        self.assertEqual(p["marker_mismatch"], [4])
        self.assertEqual(p["complete"], [1, 4])
        self.assertEqual(p["ready"], [2, 3])
        self.assertFalse(p["deadlock"])

    def test_fenced_examples_are_not_tasks(self):
        text = TASKS + "\n```markdown\n### Task 9 — Example\n- **Status**: ✅ Completed\n```\n"
        self.assertNotIn(9, runspec.by_number(runspec.parse_tasks(text)))

    def test_in_flight_tasks_are_not_ready_and_prevent_deadlock(self):
        p = runspec.progress(self.tasks, in_flight=[2, 3])
        self.assertEqual(p["ready"], [])
        self.assertFalse(p["deadlock"])

    def test_deadlock_when_nothing_can_start(self):
        blocked = TASKS.replace("- **Invariants touched**: G10.", "- **Invariants touched**: G10.\n- **Blocked**: waits on a decision.")
        blocked = blocked.replace("### Task 3 — Journal", "### Task 3 — Journal").replace(
            "- **Depends on**: Task 1\n- **Change**: Journal.", "- **Depends on**: Task 2\n- **Change**: Journal.")
        tasks = runspec.parse_tasks(blocked)
        p = runspec.progress(tasks)
        self.assertEqual(p["blocked"], [2])
        self.assertEqual(p["ready"], [])
        self.assertTrue(p["deadlock"], "task 2 blocked, 3 waits on it, 5 and 6 wait on 3")

    def test_cycle_detected(self):
        cyc = TASKS.replace("- **Depends on**: Task 1\n- **Change**: Journal.", "- **Depends on**: Task 6\n- **Change**: Journal.")
        self.assertTrue(runspec.progress(runspec.parse_tasks(cyc))["cycle"])
        self.assertFalse(runspec.progress(self.tasks)["cycle"])

    def test_closure(self):
        self.assertEqual(runspec.closure(self.tasks, 6), [1, 3, 4, 5])
        self.assertEqual(runspec.closure(self.tasks, 1), [])


class BatchTests(unittest.TestCase):
    def test_disjoint_ready_tasks_batch_and_overlap_defers(self):
        tasks = runspec.parse_tasks(TASKS)
        b = runspec.batch(tasks)
        self.assertEqual(b["batch"], [2, 3])
        # Task 3 and Task 1 share go.mod; with 1 in flight (pretend incomplete) 3 defers.
        tasks = runspec.parse_tasks(TASKS.replace("### Task 1 — Module skeleton ✅ COMPLETED", "### Task 1 — Module skeleton")
                                    .replace("- **Status**: ✅ Completed — module", "- **Status**: in review — module"))
        self.assertEqual(runspec.progress(tasks)["ready"], [1])

    def test_glob_and_directory_overlap(self):
        self.assertTrue(runspec.patterns_overlap("internal/security/*.go", "internal/security/env.go"))
        self.assertTrue(runspec.patterns_overlap("docs/", "docs/a.md"))
        self.assertFalse(runspec.patterns_overlap("internal/a.go", "internal/ab.go"))

    def test_in_flight_overlap_and_limit(self):
        text = TASKS.replace("### Task 4 — Security", "### Task 4 — Security").replace(
            "- **Status**: ✅ Completed. Security primitives; PR #9\n", "")
        tasks = runspec.parse_tasks(text)
        self.assertEqual(runspec.progress(tasks)["ready"], [2, 3, 4])
        b = runspec.batch(tasks, in_flight=[3])
        self.assertEqual(b["batch"], [2, 4])
        b = runspec.batch(tasks, limit=1)
        self.assertEqual(b["batch"], [2])
        self.assertEqual([d["task"] for d in b["deferred"]], [3, 4])

    def test_task_without_files_runs_alone(self):
        text = TASKS.replace("  - `internal/journal/journal.go`\n  - `go.mod`, `go.sum`\n", "")
        tasks = runspec.parse_tasks(text)
        b = runspec.batch(tasks)
        self.assertEqual(b["batch"], [2])
        self.assertIn(3, [d["task"] for d in b["deferred"]])


GOOD_ENTRY = {
    "Status": "✅ Completed — did it; PR #12.",
    "Implementation": "Did it. Commit abc.",
    "Spec deviations": "None.",
    "Files modified": "`a.go`",
}


def task_with(fields, heading=True):
    return {"heading_complete": heading, "fields": dict(fields)}


class EntryTests(unittest.TestCase):
    def test_good_entry(self):
        self.assertEqual(runspec.field_problems(task_with(GOOD_ENTRY), pr=12), [])

    def test_each_missing_part_is_named(self):
        for name in runspec.ENTRY_FIELDS:
            f = dict(GOOD_ENTRY)
            del f[name]
            self.assertTrue(any(name in p for p in runspec.field_problems(task_with(f))), name)
        self.assertIn("heading", runspec.field_problems(task_with(GOOD_ENTRY, heading=False))[0])

    def test_pr_number_and_status_shape(self):
        self.assertIn("PR #12, not PR #13", runspec.field_problems(task_with(GOOD_ENTRY), pr=13)[0])
        f = dict(GOOD_ENTRY, Status="done — PR #12")
        self.assertTrue(runspec.field_problems(task_with(f)))
        f = dict(GOOD_ENTRY, Status="✅ Completed — no pull request")
        self.assertTrue(any("PR #" in p for p in runspec.field_problems(task_with(f))))
        f = dict(GOOD_ENTRY, **{"Files modified": "a.go"})
        self.assertTrue(any("backticked" in p for p in runspec.field_problems(task_with(f))))

    def test_the_fixture_entries(self):
        idx = runspec.by_number(runspec.parse_tasks(TASKS))
        self.assertEqual(runspec.field_problems(idx[1], pr=7), [])
        probs = runspec.field_problems(idx[4])
        self.assertTrue(any("heading" in p for p in probs) and any("Implementation" in p for p in probs))


class ReportTests(unittest.TestCase):
    GOOD = {"task": 4, "status": "pr_open", "pr": 21, "branch": "feat/demo-t4", "first_pass": True,
            "deviations": "None", "files_modified": ["a.go"], "gates": {"go-test": "pass"},
            "budget_overrun": False, "notes": ""}

    def block(self, obj):
        body = obj if isinstance(obj, str) else json.dumps(obj)
        return "Done.\n\n```task-report\n%s\n```\n" % body

    def test_good_report(self):
        rep, err = runspec.parse_report(self.block(self.GOOD))
        self.assertIsNone(err)
        self.assertEqual(rep["pr"], 21)

    def test_missing_and_malformed(self):
        self.assertEqual(runspec.parse_report("no block")[1], "task_report_missing")
        self.assertIn("one line", runspec.parse_report(self.block("task: 4\nstatus: pr_open"))[1])
        self.assertIn("malformed", runspec.parse_report(self.block("{not json"))[1])
        bad = dict(self.GOOD, task="4")
        self.assertIn("wrong type", runspec.parse_report(self.block(bad))[1])
        bad = dict(self.GOOD, task=True)
        self.assertIn("wrong type", runspec.parse_report(self.block(bad))[1])
        bad = dict(self.GOOD, status="complete")
        self.assertIn("status", runspec.parse_report(self.block(bad))[1])
        bad = dict(self.GOOD, pr=None)
        self.assertIn("needs a pr", runspec.parse_report(self.block(bad))[1])
        bad = dict(self.GOOD)
        del bad["gates"]
        self.assertIn("'gates'", runspec.parse_report(self.block(bad))[1])

    def test_last_block_wins(self):
        text = self.block(dict(self.GOOD, notes="first")) + self.block(dict(self.GOOD, notes="second"))
        self.assertEqual(runspec.parse_report(text)[0]["notes"], "second")

    def test_skill_examples_parse(self):
        path = os.path.join(ROOT, ".claude", "skills", "task-completion", "SKILL.md")
        text = read(path)
        blocks = re.findall(r"```task-report[ \t]*\n(.*?)\n[ \t]*```", text, re.S)
        self.assertGreaterEqual(len(blocks), 2)
        for b in blocks:
            rep, err = runspec.parse_report("```task-report\n%s\n```" % b)
            self.assertIsNone(err, b)

    def test_schema_matches_parser(self):
        path = os.path.join(ROOT, ".claude", "data", "task-report.schema.json")
        schema = json.loads(read(path))
        self.assertEqual(sorted(schema["required"]), sorted(runspec.REPORT_KEYS))
        self.assertEqual(sorted(schema["properties"]["status"]["enum"]), sorted(runspec.REPORT_STATUSES))
        path = os.path.join(ROOT, ".claude", "data", "run-events.schema.json")
        schema = json.loads(read(path))
        self.assertEqual(sorted(schema["properties"]["kind"]["enum"]), sorted(runspec.EVENT_KINDS))


class HandoffTests(Tmp):
    def spec(self, text=TASKS):
        d = os.path.join(self.tmp, "specs", "in-progress", "demo")
        write(os.path.join(d, "tasks.md"), text)
        write(os.path.join(d, "scratchpad.md"), "# S\n\n## Discoveries\n\n### Task 3 — 2026-01-01\n- journal gotcha\n")
        return d

    def test_seed_is_idempotent_and_extends(self):
        d = self.spec()
        path, added = runspec.handoff_seed(d)
        self.assertEqual(added, [1, 2, 3, 4, 5, 6])
        text = read(path)
        self.assertEqual(text.count(runspec.PENDING_PLACEHOLDER), 7)  # six sections plus the header's mention
        self.assertEqual(runspec.handoff_seed(d)[1], [])
        write(os.path.join(d, "tasks.md"), TASKS + "\n### Task 7 — Extra\n\n- **Depends on**: Task 6\n")
        self.assertEqual(runspec.handoff_seed(d)[1], [7])

    def test_handoff_prefers_sections_then_scratchpad(self):
        d = self.spec()
        path, _ = runspec.handoff_seed(d)
        text = read(path).replace(
            "## Task 1 — Module skeleton\n\n%s" % runspec.PENDING_PLACEHOLDER,
            "## Task 1 — Module skeleton\n\n- Produces `cli.Main`.")
        write(path, text)
        deps, out = runspec.handoff_for(d, 5)
        self.assertEqual(deps, [1, 3, 4])
        self.assertIn("Produces `cli.Main`", out)
        self.assertIn("journal gotcha", out)
        self.assertIn("Task 4 — Security (from none)", out)

    def test_parallel_fills_of_adjacent_sections_merge_cleanly(self):
        repo = os.path.join(self.tmp, "r")
        new_repo(repo)
        d = os.path.join(repo, "specs", "in-progress", "demo")
        write(os.path.join(d, "tasks.md"), TASKS)
        runspec.handoff_seed(d)
        git(repo, "add", ".")
        git(repo, "commit", "-q", "-m", "seed")
        hp = os.path.join(d, "handoff.md")
        tp = os.path.join(d, "tasks.md")
        base = read(hp)
        tbase = read(tp)
        for branch, n, name in (("a", 2, "CI gates"), ("b", 3, "Journal")):
            git(repo, "checkout", "-q", "-b", branch, "main")
            write(hp, base.replace("## Task %d — %s\n\n%s" % (n, name, runspec.PENDING_PLACEHOLDER),
                                   "## Task %d — %s\n\n- note from %s\n- second line" % (n, name, branch)))
            entry = ("- **Status**: ✅ Completed — x; PR #%d.\n- **Implementation**: y.\n"
                     "- **Spec deviations**: None.\n- **Files modified**: `z`.\n" % (20 + n))
            anchors = {2: "- **Invariants touched**: G10.\n", 3: "- **Invariants touched**: I09.\n"}
            t = tbase.replace("### Task %d — %s\n" % (n, name), "### Task %d — %s ✅ COMPLETED\n" % (n, name))
            write(tp, t.replace(anchors[n], anchors[n] + entry))
            git(repo, "commit", "-q", "-am", branch)
        git(repo, "checkout", "-q", "main")
        git(repo, "merge", "-q", "--no-edit", "a")
        r = subprocess.run(["git", "-C", repo, "merge", "--no-edit", "b"], capture_output=True, text=True,
                           env=runspec.git_env())
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        idx = runspec.by_number(runspec.parse_tasks(read(tp)))
        self.assertEqual(runspec.field_problems(idx[2], pr=22), [])
        self.assertEqual(runspec.field_problems(idx[3], pr=23), [])
        self.assertIn("note from b", read(hp))


FAKE_GH = r'''#!/usr/bin/env python3
import json, os, sys
fx = json.load(open(os.environ["FAKE_GH_FIXTURE"]))
args = [a for a in sys.argv[1:]]
if "-R" in args:
    i = args.index("-R"); del args[i:i + 2]
key = " ".join(args[:2])
if args[:2] == ["api", "graphql"]:
    key = "api graphql"
elif args[0] == "api":
    key = "api contents"
out = fx.get(key)
if out is None:
    sys.stderr.write("fake gh: no fixture for %s\n" % key); sys.exit(1)
sys.stdout.write(out if isinstance(out, str) else json.dumps(out))
'''


def entry_tasks(pr, implementation=True, deviations="None."):
    text = TASKS.replace("### Task 3 — Journal", "### Task 3 — Journal ✅ COMPLETED")
    entry = "- **Status**: ✅ Completed — journal; PR #%d.\n" % pr
    if implementation:
        entry += "- **Implementation**: Append-only journal. Commit abc.\n"
    entry += "- **Spec deviations**: %s\n- **Files modified**: `internal/journal/journal.go`.\n" % deviations
    return text.replace("- **Invariants touched**: I09.\n", "- **Invariants touched**: I09.\n" + entry)


def check(name, conclusion="SUCCESS", started="2026-01-01T00:00:10Z", status="COMPLETED"):
    return {"__typename": "CheckRun", "name": name, "status": status, "conclusion": conclusion,
            "startedAt": started, "workflowName": "wf"}


class GhBase(Tmp):
    def setUp(self):
        super().setUp()
        self.bin = os.path.join(self.tmp, "bin")
        os.makedirs(self.bin)
        write(os.path.join(self.bin, "gh"), FAKE_GH)
        os.chmod(os.path.join(self.bin, "gh"), 0o755)
        self.old_path = os.environ["PATH"]
        os.environ["PATH"] = self.bin + os.pathsep + self.old_path
        self.addCleanup(os.environ.__setitem__, "PATH", self.old_path)
        self.fixture = os.path.join(self.tmp, "fx.json")
        os.environ["FAKE_GH_FIXTURE"] = self.fixture

    def serve(self, **over):
        fx = {
            "repo view": {"nameWithOwner": "acme/demo"},
            "pr view": {
                "state": "OPEN", "isDraft": False, "mergeable": "MERGEABLE", "mergeStateStatus": "CLEAN",
                "headRefName": "feat/demo-t3", "headRefOid": "0123456789abcdef", "title": "feat: journal (demo task 3)",
                "body": "What and why.", "files": [{"path": "internal/journal/journal.go"},
                                                   {"path": "specs/in-progress/demo/tasks.md"}],
                "statusCheckRollup": [check("CI OK"), check("Go (ubuntu)"),
                                      {"__typename": "StatusContext", "context": "CodeRabbit", "state": "SUCCESS",
                                       "startedAt": "2026-01-01T00:00:00Z"}],
            },
            "api graphql": {"data": {"repository": {"pullRequest": {"reviewThreads": {"totalCount": 1, "nodes": [
                {"isResolved": True, "path": "a.go", "comments": {"nodes": [{"url": "https://github.com/acme/demo/pull/21#r1"}]}}]}}}}},
            "api contents": {"content": base64.b64encode(entry_tasks(21).encode()).decode()},
            "pr diff": "+++ b/internal/journal/journal.go\n+package journal\n",
        }
        for k, v in over.items():
            key = k.replace("_", " ")
            if isinstance(v, dict) and isinstance(fx.get(key), dict):
                fx[key] = dict(fx[key], **v)
            else:
                fx[key] = v
        write(self.fixture, json.dumps(fx))

    def check_pr(self, **kw):
        return runspec.pr_check("demo", 3, 21, **kw)


class GhTests(GhBase):
    def test_ready(self):
        self.serve()
        verdict, facts, reasons = self.check_pr()
        self.assertEqual((verdict, reasons), ("ready", []))
        self.assertEqual(facts["unresolved_threads"], 0)

    def test_unresolved_thread_blocks(self):
        self.serve(api_graphql={"data": {"repository": {"pullRequest": {"reviewThreads": {"totalCount": 1, "nodes": [
            {"isResolved": False, "path": "a.go", "comments": {"nodes": [{"url": "https://github.com/acme/demo/pull/21#r9"}]}}]}}}}})
        verdict, _, reasons = self.check_pr()
        self.assertEqual(verdict, "not-ready")
        self.assertIn("thread-unresolved:https://github.com/acme/demo/pull/21#r9", reasons)

    def test_failed_check_blocks_and_superseded_run_is_ignored(self):
        roll = [check("CI OK"), check("Title", "CANCELLED", "2026-01-01T00:00:01Z"), check("Title", "SUCCESS", "2026-01-01T00:00:09Z"),
                {"__typename": "StatusContext", "context": "CodeRabbit", "state": "SUCCESS"}]
        self.serve(pr_view={"statusCheckRollup": roll})
        self.assertEqual(self.check_pr()[0], "ready")
        roll.append(check("Go (ubuntu)", "FAILURE"))
        self.serve(pr_view={"statusCheckRollup": roll})
        verdict, _, reasons = self.check_pr()
        self.assertEqual(verdict, "not-ready")
        self.assertIn("check-failed:Go (ubuntu)", reasons)

    def test_pending_or_missing_review_is_unknown_never_ready(self):
        self.serve(pr_view={"statusCheckRollup": [check("CI OK", None, status="IN_PROGRESS"),
                                                  {"__typename": "StatusContext", "context": "CodeRabbit", "state": "SUCCESS"}]})
        self.assertEqual(self.check_pr()[0], "unknown")
        self.serve(pr_view={"statusCheckRollup": [check("CI OK")]})
        verdict, _, reasons = self.check_pr()
        self.assertEqual((verdict, reasons), ("unknown", ["check-missing:CodeRabbit"]))
        self.assertEqual(self.check_pr(required=("CI OK",))[0], "ready")
        self.serve(pr_view={"mergeStateStatus": "UNKNOWN"})
        self.assertEqual(self.check_pr()[0], "unknown")

    def test_conflict_behind_and_draft(self):
        self.serve(pr_view={"mergeable": "CONFLICTING", "mergeStateStatus": "DIRTY"})
        self.assertTrue(any(r.startswith("conflict") for r in self.check_pr()[2]))
        self.serve(pr_view={"mergeStateStatus": "BEHIND"})
        self.assertTrue(any(r.startswith("behind") for r in self.check_pr()[2]))
        self.serve(pr_view={"isDraft": True})
        self.assertIn("draft", self.check_pr()[2])

    def test_entry_must_be_in_the_pull_request_and_well_formed(self):
        self.serve(pr_view={"files": [{"path": "internal/journal/journal.go"}]})
        self.assertTrue(any("does not change" in r for r in self.check_pr()[2]))
        self.serve(api_contents={"content": base64.b64encode(entry_tasks(21, implementation=False).encode()).decode()})
        self.assertIn("entry: no '- **Implementation**:' field", self.check_pr()[2])
        self.serve(api_contents={"content": base64.b64encode(entry_tasks(99).encode()).decode()})
        self.assertIn("entry: Status names PR #99, not PR #21", self.check_pr()[2])

    def test_scope_outside_files_needs_a_recorded_deviation(self):
        files = [{"path": "internal/journal/journal.go"}, {"path": "internal/cli/exit.go"},
                 {"path": "specs/in-progress/demo/tasks.md"}, {"path": "specs/in-progress/demo/handoff.md"}]
        self.serve(pr_view={"files": files})
        verdict, _, reasons = self.check_pr()
        self.assertEqual(verdict, "not-ready")
        self.assertIn("scope: internal/cli/exit.go is outside the task's Files and not named in Spec deviations", reasons)
        self.assertEqual(self.check_pr(accept_scope=("internal/cli/exit.go",))[0], "ready")
        self.serve(pr_view={"files": files},
                   api_contents={"content": base64.b64encode(entry_tasks(21, deviations="`exit.go` maps the new error.").encode()).decode()})
        self.assertEqual(self.check_pr()[0], "ready")

    def test_leaks_in_body_or_diff_block(self):
        home = "/" + "/".join(("home", "someone", "work"))  # assembled so this file itself stays clean
        self.serve(pr_view={"body": "Run it from %s first." % home})
        self.assertTrue(any(r.startswith("leak:") for r in self.check_pr()[2]))
        self.serve(pr_diff="+++ b/x\n+token " + "ghp" + "_abcdefghijklmnopqrstuvwxyz0123456789\n")
        self.assertTrue(any(r.startswith("leak:") for r in self.check_pr()[2]))

    def test_lifecycle_pull_request(self):
        files = [{"path": "specs/todo/demo/tasks.md"}, {"path": "specs/in-progress/demo/tasks.md"},
                 {"path": "specs/in-progress/demo/handoff.md"}]
        self.serve(pr_view={"files": files})
        self.assertEqual(runspec.pr_check("demo", None, 21)[0], "ready")
        self.serve(pr_view={"files": files + [{"path": "internal/x.go"}]})
        verdict, _, reasons = runspec.pr_check("demo", None, 21)
        self.assertEqual(verdict, "not-ready")
        self.assertIn("scope: a lifecycle pull request changes only specs/*/demo/, not internal/x.go", reasons)
        with redirect_stdout(io.StringIO()):
            self.assertEqual(runspec.main(["pr-check", "--spec", "demo", "--pr", "21"]), 2)
            self.assertEqual(runspec.main(["pr-check", "--spec", "demo", "--task", "3", "--lifecycle", "--pr", "21"]), 2)

    def test_cli_exit_codes(self):
        self.serve()
        with redirect_stdout(io.StringIO()) as out:
            rc = runspec.main(["pr-check", "--spec", "demo", "--task", "3", "--pr", "21"])
        self.assertEqual(rc, 0, out.getvalue())
        self.assertIn("verdict=ready", out.getvalue())
        self.serve(pr_view={"mergeStateStatus": "UNKNOWN"})
        with redirect_stdout(io.StringIO()):
            self.assertEqual(runspec.main(["pr-check", "--spec", "demo", "--task", "3", "--pr", "21"]), 3)
        os.environ["FAKE_GH_FIXTURE"] = os.path.join(self.tmp, "missing.json")
        with redirect_stdout(io.StringIO()):
            self.assertEqual(runspec.main(["pr-check", "--spec", "demo", "--task", "3", "--pr", "21"]), 2,
                             "an unreadable GitHub answer is never a verdict")


class GitTests(GhBase):
    def setUp(self):
        super().setUp()
        self.origin = os.path.join(self.tmp, "origin.git")
        subprocess.run(["git", "init", "-q", "--bare", "-b", "main", self.origin], check=True)
        self.repo = os.path.join(self.tmp, "clone")
        new_repo(self.repo)
        git(self.repo, "remote", "add", "origin", self.origin)
        write(os.path.join(self.repo, "specs", "in-progress", "demo", "tasks.md"), TASKS)
        git(self.repo, "add", ".")
        git(self.repo, "commit", "-q", "-m", "spec")
        git(self.repo, "push", "-q", "origin", "main")
        self.cwd = os.getcwd()
        os.chdir(self.repo)
        self.addCleanup(os.chdir, self.cwd)

    def test_deps_merged(self):
        with redirect_stdout(io.StringIO()) as out:
            self.assertEqual(runspec.main(["deps-merged", "demo", "2"]), 0)
            self.assertEqual(runspec.main(["deps-merged", "demo", "5"]), 1)
        self.assertIn("unmerged: task 3", out.getvalue())

    def test_verify_merged(self):
        write(os.path.join(self.repo, "specs", "in-progress", "demo", "tasks.md"), entry_tasks(21))
        git(self.repo, "commit", "-q", "-am", "task 3")
        oid = git(self.repo, "rev-parse", "HEAD").strip()
        self.serve(pr_view={"state": "MERGED", "mergeCommit": {"oid": oid}})
        self.assertEqual(runspec.verify_merged("demo", 3, 21, fetch=False)[0], "merge commit %s is not on origin/main" % oid[:12])
        git(self.repo, "push", "-q", "origin", "main")
        self.assertEqual(runspec.verify_merged("demo", 3, 21), [])
        self.serve(pr_view={"state": "OPEN", "mergeCommit": None})
        self.assertIn("not merged", runspec.verify_merged("demo", 3, 21)[0])

    def test_events_and_summary(self):
        with redirect_stdout(io.StringIO()):
            for args in (["--kind", "dispatch", "--task", "2", "--attempt", "1", "--agent", "release-engineer"],
                         ["--kind", "return", "--task", "2", "--pr", "30"],
                         ["--kind", "merge", "--task", "2", "--pr", "30", "--result", "ok"],
                         ["--kind", "dispatch", "--task", "3", "--attempt", "1", "--agent", "go-implementer"],
                         ["--kind", "fail", "--task", "3", "--detail", "tests red"],
                         ["--kind", "dispatch", "--task", "3", "--attempt", "2", "--agent", "go-implementer"]):
                self.assertEqual(runspec.main(["event", "--spec", "demo"] + args), 0)
            runspec.main(["event", "--spec", "other", "--kind", "dispatch", "--task", "2"])
        rows = open(os.path.join(self.repo, ".claude", "data", "run-events.jsonl"), encoding="utf-8").read().splitlines()
        self.assertEqual(len(rows), 7)
        s = runspec.summarize(runspec.read_events("demo"))
        self.assertEqual((s["dispatched"], s["returned"], s["failed"]), (3, 1, 1))
        self.assertEqual(s["unaccounted"], ["task 3 (1 dispatch)"])
        self.assertTrue(s["tasks"][2]["first_pass"])
        self.assertFalse(s["tasks"][3]["first_pass"])
        with redirect_stdout(io.StringIO()) as out:
            runspec.main(["summary", "--spec", "demo"])
        self.assertIn("dispatched=3 returned=1 failed=1", out.getvalue())
        self.assertIn("unaccounted=task 3", out.getvalue())

    def test_event_rows_land_in_the_main_checkout_from_a_worktree(self):
        wt = os.path.join(self.tmp, "wt")
        git(self.repo, "worktree", "add", "-q", "-b", "t", wt)
        os.chdir(wt)
        with redirect_stdout(io.StringIO()):
            runspec.main(["event", "--spec", "demo", "--kind", "run_start"])
        self.assertTrue(os.path.exists(os.path.join(self.repo, ".claude", "data", "run-events.jsonl")))
        self.assertFalse(os.path.exists(os.path.join(wt, ".claude", "data", "run-events.jsonl")))


if __name__ == "__main__":
    unittest.main()
