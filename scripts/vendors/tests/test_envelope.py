"""Tests for the vendor envelope: vendors.py, consult.py (through the codex and
muse entry points), snapshot.sh, adjudicate.py and route_lane.py. Offline: the
vendor CLIs are the stubs in fixtures/bin, driven by stub.env."""

from __future__ import annotations

import glob
import json
import os
import re
import subprocess
import sys
import time
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from fixture import REPO_ROOT, Fixture, load_vendors_module, read  # noqa: E402

V = load_vendors_module()
RESPONSE = V.load_schema("response")
REQUEST = V.load_schema("request")
FINDINGS = V.load_schema("findings")

TOKEN = "gh" + "p_" + "A1b2C3d4" * 5
EMAIL = "someone" + "@" + "corp-mail.test"
PEM_BEGIN = "-----BEGIN RSA " + "PRIVATE KEY-----"

TASKS_MD = """## Demo — Tasks

## Implementation Tasks

### Task 1 — Add the foo package

- **Domain/agent**: go-implementer
- **Budget**: trivial
- **Change**: Add foo.
- **Files**:
  - `internal/foo/foo.go`
  - `internal/foo/foo_test.go` (table tests)
- **Acceptance**:
  - `TestFoo` passes.

### Task 2 — Touch CI

- **Budget**: standard (small)
- **Files**:
  - `.github/workflows/ci.yml`
  - `go.mod`

### Task 3 — Wide change

- **Budget**: complex
- **Depends on**: Task 1
- **Files**: `internal/a.go`
"""


class Unit(unittest.TestCase):
    def test_redact_masks_secrets_and_keeps_placeholders(self):
        home = os.path.expanduser("~")
        text = (f"token {TOKEN}\nmail {EMAIL} and ok@example.com\nAPI_KEY=abc123\n"
                f"path {home}/x\n{PEM_BEGIN}\nMIIB\n-----END RSA PRIVATE KEY-----\n")
        out, n = V.redact(text)
        self.assertNotIn(TOKEN, out)
        self.assertNotIn(EMAIL, out)
        self.assertNotIn("abc123", out)
        self.assertNotIn("MIIB", out)
        self.assertIn("ok@example.com", out)
        self.assertIn("~/x", out)
        self.assertEqual(n, 5)

    def test_redact_leaves_clean_text_alone(self):
        self.assertEqual(V.redact("func main() { return }\n"), ("func main() { return }\n", 0))

    def test_validate_accepts_the_shape_and_rejects_broken_ones(self):
        with open(os.path.join(os.path.dirname(__file__), "fixtures", "bin", "answer.json")) as fh:
            good = json.load(fh)
        self.assertEqual(V.validate(good, FINDINGS), [])
        bad = json.loads(json.dumps(good))
        bad["findings"][0]["severity"] = "blocker"
        self.assertTrue(any("not in" in e for e in V.validate(bad, FINDINGS)))
        del bad["findings"][0]["claim"]
        self.assertTrue(any("claim" in e for e in V.validate(bad, FINDINGS)))
        self.assertTrue(V.validate({"summary": "x", "findings": [], "extra": 1}, FINDINGS))
        self.assertTrue(V.validate({"summary": 1, "findings": []}, FINDINGS))

    def test_parse_task_reads_files_budget_and_depends(self):
        f = Fixture()
        self.addCleanup(f.cleanup)
        f.write("specs/todo/demo/tasks.md", TASKS_MD)
        spec = os.path.join(f.repo, "specs/todo/demo")
        t1 = V.parse_task(spec, 1)
        self.assertEqual(t1["files"], ["internal/foo/foo.go", "internal/foo/foo_test.go"])
        self.assertEqual(V.budget_word(t1["budget"]), "trivial")
        self.assertEqual(t1["title"], "Add the foo package")
        self.assertNotIn("Task 2", t1["text"])
        self.assertEqual(V.budget_word(V.parse_task(spec, 2)["budget"]), "standard")
        t3 = V.parse_task(spec, 3)
        self.assertEqual((t3["files"], t3["depends"]), (["internal/a.go"], "Task 1"))
        with self.assertRaises(KeyError):
            V.parse_task(spec, 9)

    def test_protected_paths_allow_go_mod_only_when_listed(self):
        pol = {"lanes": {"protected_paths": [".github/", "go.mod", "LICENSE"], "allowed_when_listed": ["go.mod"]}}
        self.assertEqual(V.protected_hits(pol, [".github/workflows/ci.yml", "go.mod", "a.go"]), [".github/workflows/ci.yml", "go.mod"])
        self.assertEqual(V.protected_hits(pol, ["go.mod"], listed=["go.mod"]), [])
        self.assertEqual(V.protected_hits(pol, ["LICENSE"], listed=["LICENSE"]), ["LICENSE"])
        self.assertFalse(V.path_in(".githubx/a", [".github/"]))

    def test_version_compare_is_numeric(self):
        self.assertTrue(V.version_ge("codex-cli 0.155.1", "0.150.0"))
        self.assertFalse(V.version_ge("0.9.0", "0.150.0"))
        self.assertFalse(V.version_ge("no version here", "0.1.0"))


class Consult(unittest.TestCase):
    def setUp(self):
        self.f = Fixture()
        self.addCleanup(self.f.cleanup)
        self.ctx = self.f.write("ctx.md", f"Review task 1.\nleaked {TOKEN}\n", root=self.f.tmp)

    def consult(self, *extra, vendor="codex", stage="task-review", target=None, env=None):
        script = "scripts/codex/codex-consult.sh" if vendor == "codex" else "scripts/vendors/muse-consult.sh"
        target = target or self.f.worktree(f"wt{time.monotonic_ns()}")
        resp = self.f.run_json(script, "--stage", stage, "--target", target, "--context", self.ctx, *extra, env=env)
        self.assertEqual(V.validate(resp, RESPONSE), [], resp)
        return resp

    def argv_log(self):
        path = os.path.join(self.f.bin, "argv.log")
        return read(path) if os.path.exists(path) else ""

    def test_disabled_by_default_spawns_nothing_and_leaves_no_trace(self):
        resp = self.consult()
        self.assertEqual((resp["outcome"], resp["reason"], resp["spawned"]), ("unavailable", "disabled", False))
        self.assertEqual(self.argv_log(), "")
        self.assertFalse(os.path.exists(self.f.data("vendor-calls.jsonl")))

    def test_completed_consult_is_read_only_redacted_and_recorded(self):
        self.f.enable("codex")
        resp = self.consult("--spec", "demo")
        self.assertEqual((resp["outcome"], resp["reason"], resp["spawned"]), ("completed", None, True))
        self.assertEqual((resp["input_tokens"], resp["output_tokens"], resp["spec"]), (120, 30, "demo"))
        with open(resp["answer_path"]) as fh:
            self.assertEqual(V.validate(json.load(fh), FINDINGS), [])
        run = self.argv_log().splitlines()[-1]
        for flag in ("--sandbox read-only", "--ignore-user-config", "--ephemeral", "--output-schema"):
            self.assertIn(flag, run)
        self.assertNotIn("workspace-write", run)
        call_dir = self.f.data(f"vendor-calls/{resp['call_id']}")
        prompt = read(os.path.join(call_dir, "prompt.md"))
        self.assertNotIn(TOKEN, prompt)
        self.assertIn("[REDACTED TOKEN]", prompt)
        self.assertIn("## Stage: task-review", prompt)
        with open(os.path.join(call_dir, "request.json")) as fh:
            request = json.load(fh)
        self.assertEqual(V.validate(request, REQUEST), [])
        self.assertEqual(request["redactions"], 1)
        rows = self.f.rows()
        self.assertEqual(len(rows), 1)
        self.assertEqual(V.validate(rows[0], RESPONSE), [])

    def test_main_checkout_and_dirty_worktrees_are_refused_before_any_spawn(self):
        self.f.enable("codex")
        self.assertEqual(self.consult(target=self.f.repo)["reason"], "target-refused")
        wt = self.f.worktree("dirty")
        self.f.write(".env", "SECRET=1\n", root=wt)
        self.assertEqual(self.f.git("status", "--porcelain", cwd=wt), "")
        resp = self.consult(target=wt)
        self.assertEqual(resp["reason"], "target-refused")
        self.assertIn("ignored", resp["detail"])
        self.assertEqual(self.argv_log(), "")

    def test_availability_failures_are_skips_that_spend_nothing(self):
        self.f.enable("codex")
        self.f.stub(STUB_SIGNED_IN=1)
        self.assertEqual(self.consult()["reason"], "not-signed-in")
        self.f.stub(STUB_VERSION="0.9.0")
        self.assertEqual(self.consult()["reason"], "version-too-old")
        no_cli = os.pathsep.join([os.path.dirname(sys.executable), "/usr/bin", "/bin"])
        self.assertEqual(self.consult(env={"PATH": no_cli})["reason"], "cli-missing")
        self.assertNotIn("exec", self.argv_log())
        self.assertFalse(os.path.exists(self.f.data("vendor-quota.json")))

    def test_bad_answers_are_typed_skips(self):
        self.f.enable("codex")
        for mode, reason in (("notjson", "unparseable-output"), ("badschema", "schema-mismatch"),
                             ("fail", "vendor-error"), ("write-through", "write-through")):
            self.f.stub(STUB_MODE=mode)
            resp = self.consult()
            self.assertEqual((resp["outcome"], resp["reason"], resp["spawned"]), ("unavailable", reason, True), mode)
            self.assertIsNone(resp["answer_path"])

    def test_timeout_kills_the_vendor(self):
        self.f.enable("codex")
        self.f.stub(STUB_MODE="sleep")
        started = time.monotonic()
        resp = self.consult(env={"VENDOR_TIMEOUT_S": "2"})
        self.assertEqual(resp["reason"], "timeout")
        self.assertLess(time.monotonic() - started, 20)

    def test_daily_caps_stop_further_calls(self):
        self.f.enable("codex", stages={"task-review": {"daily_cap": 1}})
        self.assertEqual(self.consult()["outcome"], "completed")
        self.assertEqual(self.consult()["reason"], "over-quota")
        self.assertEqual(self.argv_log().count(" exec "), 1)
        self.f.enable("codex", vendors={"codex": {"daily_call_cap": 1}})
        self.assertEqual(self.consult(stage="change-review")["reason"], "over-quota")

    def test_policy_denies_unlisted_stage_vendor_pairs_and_oversized_context(self):
        self.f.enable("codex", stages={"task-review": {"max_context_bytes": 100}})
        self.assertEqual(self.consult(stage="dossier")["reason"], "policy-denied")
        self.assertEqual(self.consult()["reason"], "context-too-large")
        rc, _out, err = self.f.run("scripts/codex/codex-consult.sh", "--stage", "nope", "--target", self.f.repo,
                                   "--context", self.ctx)
        self.assertEqual(rc, 64, err)

    def test_broken_local_policy_fails_open(self):
        self.f.write(".claude/data/vendor-policy.local.json", "{not json")
        resp = self.consult()
        self.assertEqual((resp["outcome"], resp["reason"]), ("unavailable", "policy-unreadable"))

    def test_muse_consult_runs_with_writes_and_shell_disabled(self):
        self.f.enable("muse")
        resp = self.consult(vendor="muse", stage="dossier")
        self.assertEqual(resp["reason"], "schema-mismatch")
        resp = self.consult(vendor="muse")
        self.assertEqual(resp["outcome"], "completed")
        run = self.argv_log().splitlines()[-1]
        for flag in ("--disable-write", "--disable-shell", "--disable-web-tools", "--sandbox-network restricted"):
            self.assertIn(flag, run)
        self.f.stub(STUB_MODE="notjson")
        self.assertEqual(self.consult(vendor="muse")["reason"], "unparseable-output")
        self.f.stub(STUB_MODE="fail")
        self.assertEqual(self.consult(vendor="muse")["reason"], "vendor-error")

    def test_codex_review_sends_the_diff_against_the_base(self):
        self.f.enable("codex")
        wt = self.f.worktree("review")
        self.f.write("main.go", "package main\n\nfunc main() { panic(1) }\n", root=wt)
        self.f.git("commit", "-q", "-am", "change", cwd=wt)
        resp = self.f.run_json("scripts/codex/codex-review.sh", "--target", wt, "--base", "HEAD~1")
        self.assertEqual((resp["outcome"], resp["stage"]), ("completed", "change-review"))
        prompt = read(self.f.data(f"vendor-calls/{resp['call_id']}/prompt.md"))
        self.assertIn("+func main() { panic(1) }", prompt)
        rc, _o, _e = self.f.run("scripts/codex/codex-review.sh", "--target", wt, "--base", "no-such-ref")
        self.assertEqual(rc, 64)


class Snapshot(unittest.TestCase):
    def test_snapshot_carries_uncommitted_work_but_never_ignored_files_or_links(self):
        f = Fixture()
        self.addCleanup(f.cleanup)
        f.write("main.go", "package main\n\n// edited\n")
        f.write("new.go", "package main\n")
        f.write(".env", "SECRET=1\n")
        os.symlink("/etc/hostname", os.path.join(f.repo, "link"))
        f.write(".claude/data/vendor-calls.jsonl", "{}\n")
        rc, out, err = f.run("scripts/vendors/snapshot.sh", "create", "--from", f.repo, "--include-uncommitted")
        self.assertEqual(rc, 0, err)
        kv = dict(line.split("=", 1) for line in out.splitlines())
        wt = kv["snapshot_dir"]
        self.assertEqual((kv["snapshot_changed_files"], kv["snapshot_skipped_symlinks"]), ("2", "1"))
        self.assertIn("// edited", read(os.path.join(wt, "main.go")))
        self.assertTrue(os.path.exists(os.path.join(wt, "new.go")))
        for absent in (".env", "link"):
            self.assertFalse(os.path.lexists(os.path.join(wt, absent)), absent)
        self.assertEqual(f.git("rev-parse", "HEAD~1", cwd=wt), kv["snapshot_base"])
        self.assertEqual(V.is_clean_linked_worktree(wt), (True, ""))
        rc, _o, _e = f.run("scripts/vendors/snapshot.sh", "remove", "--dir", f.repo)
        self.assertEqual(rc, 65)
        self.assertTrue(os.path.isdir(f.repo))
        rc, out, _e = f.run("scripts/vendors/snapshot.sh", "remove", "--dir", wt)
        self.assertEqual(rc, 0)
        self.assertFalse(os.path.exists(wt))
        self.assertNotIn(wt, f.git("worktree", "list"))

    def test_only_limits_the_copy(self):
        f = Fixture()
        self.addCleanup(f.cleanup)
        f.write("a/one.go", "package a\n")
        f.write("b/two.go", "package b\n")
        rc, out, err = f.run("scripts/vendors/snapshot.sh", "create", "--from", f.repo, "--include-uncommitted", "--only", "a")
        self.assertEqual(rc, 0, err)
        wt = dict(line.split("=", 1) for line in out.splitlines())["snapshot_dir"]
        self.assertTrue(os.path.exists(os.path.join(wt, "a/one.go")))
        self.assertFalse(os.path.exists(os.path.join(wt, "b/two.go")))
        f.run("scripts/vendors/snapshot.sh", "remove", "--dir", wt)


class Adjudicate(unittest.TestCase):
    def test_record_refuses_bad_input_and_yield_joins(self):
        f = Fixture()
        self.addCleanup(f.cleanup)
        f.enable("codex")
        ctx = f.write("ctx.md", "task\n", root=f.tmp)
        ok = f.run_json("scripts/codex/codex-consult.sh", "--stage", "task-review", "--target", f.worktree(), "--context", ctx)
        f.stub(STUB_MODE="fail")
        bad = f.run_json("scripts/codex/codex-consult.sh", "--stage", "task-review", "--target", f.worktree("w2"), "--context", ctx)
        adj = "scripts/vendors/adjudicate.py"
        self.assertEqual(f.run(adj, "record", "--call-id", "nope", "--findings", "1", "--confirmed", "1", "--rejected", "0")[0], 65)
        self.assertEqual(f.run(adj, "record", "--call-id", bad["call_id"], "--findings", "1", "--confirmed", "1", "--rejected", "0")[0], 65)
        self.assertEqual(f.run(adj, "record", "--call-id", ok["call_id"], "--findings", "1", "--confirmed", "1", "--rejected", "1")[0], 65)
        self.assertEqual(f.run(adj, "record", "--call-id", ok["call_id"], "--findings", "2", "--confirmed", "1", "--rejected", "1")[0], 0)
        self.assertEqual(f.run(adj, "record", "--call-id", ok["call_id"], "--findings", "2", "--confirmed", "1", "--rejected", "1")[0], 65)
        self.assertEqual(len(f.rows("vendor-adjudications.jsonl")), 1)
        rc, out, _e = f.run(adj, "yield", "--json")
        (group,) = json.loads(out)
        self.assertEqual({k: group[k] for k in ("calls", "completed", "adjudicated", "confirmed", "rejected", "confirmed_pct")},
                         {"calls": 2, "completed": 1, "adjudicated": 1, "confirmed": 1, "rejected": 1, "confirmed_pct": 50})


class Route(unittest.TestCase):
    POL = {"lanes": {"file_ceiling": 2, "protected_paths": [".github/"], "allowed_when_listed": []}}

    def task(self, files, budget="standard"):
        return {"files": files, "budget": budget}

    def route(self, task, klass="tight_spec_code", conf=0.9, avail=None, parallel=False):
        sys.path.insert(0, os.path.join(REPO_ROOT, "scripts", "vendors"))
        import route_lane
        return route_lane.route(task, klass, conf, avail if avail is not None else {"codex": True, "muse": True}, parallel, self.POL)

    def test_routing_table(self):
        one = self.task(["a.go"])
        self.assertEqual(self.route(one), ("codex", "tight-spec"))
        self.assertEqual(self.route(one, parallel=True), ("claude", "parallel-batch"))
        self.assertEqual(self.route(self.task([])), ("claude", "no-files"))
        self.assertEqual(self.route(self.task([".github/x.yml"])), ("claude", "protected-path"))
        self.assertEqual(self.route(one, klass="concurrency_risky"), ("claude", "class-gate"))
        self.assertEqual(self.route(one, conf=0.2), ("claude", "class-gate"))
        self.assertEqual(self.route(one, avail={"codex": False, "muse": False}), ("claude", "default"))
        self.assertEqual(self.route(self.task(["a", "b", "c"])), ("muse", "wide-change"))
        self.assertEqual(self.route(self.task(["a"], "complex"), klass="long_context_investigation"), ("muse", "long-context"))
        self.assertEqual(self.route(self.task(["a"], "complex")), ("claude", "default"))

    def test_cli_routes_to_claude_without_opt_in_and_logs_nothing(self):
        f = Fixture()
        self.addCleanup(f.cleanup)
        f.write("specs/todo/demo/tasks.md", TASKS_MD)
        rc, out, err = f.run("scripts/vendors/route_lane.py", "--spec-dir", os.path.join(f.repo, "specs/todo/demo"),
                             "--task", "1", "--class", "tight_spec_code")
        self.assertEqual(rc, 0, err)
        self.assertTrue(out.startswith("lane=claude rule=default "), out)
        self.assertFalse(os.path.exists(f.data("lane-routing.jsonl")))
        rc, out, _e = f.run("scripts/vendors/route_lane.py", "--spec-dir", os.path.join(f.repo, "specs/todo/demo"), "--task", "1")
        self.assertIn("class=uncertain", out)


class Opt(unittest.TestCase):
    def test_enable_disable_and_status(self):
        f = Fixture()
        self.addCleanup(f.cleanup)
        rc, out, _e = f.run("scripts/vendors/vendors.py", "status", "--json")
        self.assertEqual({r["vendor"]: r["reason"] for r in json.loads(out)},
                         {"codex": "disabled", "muse": "disabled", "jev": "disabled"})
        self.assertEqual(f.run("scripts/vendors/vendors.py", "enable", "codex", "--lanes")[0], 0)
        self.assertEqual(f.run("scripts/vendors/vendors.py", "enable", "jev", "--lanes")[0], 64)
        rc, out, _e = f.run("scripts/vendors/vendors.py", "status", "--json")
        codex = next(r for r in json.loads(out) if r["vendor"] == "codex")
        self.assertEqual((codex["available"], codex["lanes"], codex["version"]), (True, True, "0.155.1"))
        f.run("scripts/vendors/vendors.py", "disable", "all")
        rc, out, _e = f.run("scripts/vendors/vendors.py", "status", "--json")
        self.assertFalse(any(r["enabled"] for r in json.loads(out)))
        self.assertEqual(f.git("status", "--porcelain"), "", "the local opt-in must stay gitignored")

    def test_redact_cli(self):
        f = Fixture()
        self.addCleanup(f.cleanup)
        r = subprocess.run([sys.executable, os.path.join(f.repo, "scripts/vendors/vendors.py"), "redact"],
                           input=f"k {TOKEN}\n", capture_output=True, text=True, env=f.env)
        self.assertEqual(r.stdout, "k [REDACTED TOKEN]\n")


class NeverInCI(unittest.TestCase):
    ENTRY = re.compile(r"codex-consult|codex-review\.sh|muse-consult|codex-implement|muse-implement|consult\.py|lane\.py|jev\.py")

    def offenders(self, paths):
        return [p for p in paths if self.ENTRY.search(read(p, encoding="utf-8"))]

    def test_no_workflow_calls_a_vendor(self):
        workflows = glob.glob(os.path.join(REPO_ROOT, ".github", "workflows", "*.yml"))
        self.assertTrue(workflows)
        self.assertEqual(self.offenders(workflows), [])

    def test_the_check_bites(self):
        f = Fixture()
        self.addCleanup(f.cleanup)
        bad = f.write("ci.yml", "jobs:\n  x:\n    steps:\n      - run: scripts/codex/codex-consult.sh --stage task-review\n", root=f.tmp)
        self.assertEqual(self.offenders([bad]), [bad])


if __name__ == "__main__":
    unittest.main()
