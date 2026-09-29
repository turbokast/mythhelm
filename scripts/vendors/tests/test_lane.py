"""Tests for the implementer lane (lane.py through codex-implement.sh and
muse-implement.sh). The refusals run everywhere. The runs that need the bubblewrap
sandbox are skipped, with the reason printed, where bwrap is missing or cannot
start a user namespace (GitHub's Ubuntu runners restrict unprivileged user
namespaces, so CI skips them and they run on contributors' Linux machines)."""

from __future__ import annotations

import json
import os
import shutil
import stat
import subprocess
import sys
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from fixture import Fixture, load_vendors_module, read  # noqa: E402

V = load_vendors_module()
RESPONSE = V.load_schema("response")

TASKS_MD = """## Demo — Tasks

### Task 1 — Add foo

- **Budget**: trivial
- **Files**:
  - `internal/foo/foo.go`
  - `internal/foo/foo_test.go`

### Task 2 — Workflows

- **Budget**: trivial
- **Files**:
  - `.github/workflows/ci.yml`

### Task 3 — No files

- **Budget**: trivial
"""


def bwrap_usable() -> str | None:
    """None when the sandbox can run here, else why not."""
    if not sys.platform.startswith("linux"):
        return "not Linux"
    path = shutil.which("bwrap")
    if not path:
        return "bwrap is not installed"
    real = os.path.realpath(path)
    st = os.stat(real)
    if st.st_uid != 0 or st.st_mode & (stat.S_IWGRP | stat.S_IWOTH):
        return "bwrap is not root-owned"
    probe = subprocess.run([real, "--unshare-pid", "--ro-bind", "/", "/", "--proc", "/proc", "--dev", "/dev",
                            "--", "/usr/bin/true"], capture_output=True)
    return None if probe.returncode == 0 else "bwrap cannot start a user namespace on this host"


SKIP = bwrap_usable()


class LaneBase(unittest.TestCase):
    def setUp(self):
        self.f = Fixture()
        self.addCleanup(self.f.cleanup)
        self.f.write("specs/todo/demo/tasks.md", TASKS_MD)
        self.f.git("add", "-A")
        self.f.git("commit", "-q", "-m", "spec")
        self.spec = os.path.join(self.f.repo, "specs/todo/demo")

    def lane(self, task="1", vendor="codex", *extra, env=None):
        resp = self.f.run_json(f"scripts/vendors/{vendor}-implement.sh", "--spec-dir", self.spec, "--task", task,
                               "--from", self.f.repo, *extra, env=env)
        self.assertEqual(V.validate(resp, RESPONSE), [], resp)
        return resp

    def spawned_exec(self):
        path = os.path.join(self.f.bin, "argv.log")
        return os.path.exists(path) and " exec " in f" {read(path)}"

    def vendor_argv(self, resp):
        out = read(self.f.data(f"vendor-calls/{resp['call_id']}/stdout.jsonl"))
        return next(e["argv"] for e in map(json.loads, out.splitlines()) if e.get("type") == "argv")

    def assert_cleaned_up(self):
        self.assertEqual([ln for ln in self.f.git("worktree", "list").splitlines() if "worktrees" in ln], [])
        self.assertEqual(self.f.git("branch", "--list", "vendor-lane/*"), "")


class Refusals(LaneBase):
    def test_lane_needs_its_own_opt_in(self):
        self.assertEqual(self.lane()["reason"], "disabled")
        self.f.enable("codex")
        self.assertEqual(self.lane()["reason"], "disabled")
        self.assertFalse(self.spawned_exec())

    def test_protected_and_unreadable_tasks_are_refused_before_a_worktree_exists(self):
        self.f.enable("codex", lanes=["codex"])
        self.assertEqual(self.lane("2")["reason"], "protected-path")
        self.assertEqual(self.lane("3")["reason"], "task-unreadable")
        self.assertEqual(self.lane("9")["reason"], "task-unreadable")
        self.assertFalse(self.spawned_exec())
        self.assert_cleaned_up()

    @unittest.skipUnless(sys.platform.startswith("linux"), "the ownership check is Linux-only")
    def test_a_user_owned_bwrap_is_never_trusted(self):
        self.f.enable("codex", lanes=["codex"])
        fake = os.path.join(self.f.tmp, "fakebin")
        os.makedirs(fake)
        self.f.write("bwrap", "#!/bin/sh\nshift $#\nexec \"$@\"\n", root=fake)
        os.chmod(os.path.join(fake, "bwrap"), 0o755)
        resp = self.lane(env={"PATH": fake + os.pathsep + self.f.env["PATH"]})
        self.assertEqual((resp["reason"], resp["spawned"]), ("sandbox-unavailable", False))
        self.assertIn("root-owned", resp["detail"])
        self.assertFalse(self.spawned_exec())

    def test_unknown_tier_is_a_usage_error(self):
        self.f.enable("codex", lanes=["codex"])
        rc, _o, _e = self.f.run("scripts/vendors/codex-implement.sh", "--spec-dir", self.spec, "--task", "1", "--tier", "huge")
        self.assertEqual(rc, 64)


@unittest.skipIf(SKIP, f"sandboxed lane runs skipped: {SKIP}")
class SandboxedRuns(LaneBase):
    def setUp(self):
        super().setUp()
        self.f.enable("codex", "muse", lanes=["codex", "muse"])

    def test_completed_lane_returns_an_in_scope_patch(self):
        self.f.stub(STUB_MODE="lane", STUB_WRITE="internal/foo/foo.go internal/foo/foo_test.go")
        resp = self.lane()
        self.assertEqual((resp["outcome"], resp["reason"]), ("completed", None), resp)
        self.assertEqual(resp["files_changed"], ["internal/foo/foo.go", "internal/foo/foo_test.go"])
        self.assertEqual(resp["attribution"], "Vendor-Assisted-By: Codex CLI")
        patch = read(resp["patch_path"])
        self.assertIn("+// written by the stub", patch)
        self.assertIn("--sandbox workspace-write", self.vendor_argv(resp))
        self.assert_cleaned_up()
        self.assertEqual(self.f.git("status", "--porcelain"), "")
        check = subprocess.run(["git", "-C", self.f.repo, "apply", "--check", resp["patch_path"]],
                               capture_output=True, text=True, env=self.f.env)
        self.assertEqual(check.returncode, 0, check.stderr)

    def test_muse_lane(self):
        self.f.stub(STUB_MODE="lane", STUB_WRITE="internal/foo/foo.go")
        resp = self.lane("1", "muse")
        self.assertEqual(resp["outcome"], "completed", resp)
        self.assertIn("--disable-shell", self.vendor_argv(resp))

    def test_out_of_scope_symlinks_and_empty_runs_are_refused(self):
        for mode, write, reason in (("lane", "internal/foo/foo.go cmd/extra.go", "out-of-scope"),
                                    ("lane", "internal/foo/foo.go .github/workflows/x.yml", "protected-path"),
                                    ("lane-link", "", "symlink"),
                                    ("lane-nothing", "", "no-changes")):
            os.makedirs(os.path.join(self.f.repo, "internal"), exist_ok=True)
            self.f.stub(STUB_MODE=mode, STUB_WRITE=write)
            resp = self.lane()
            self.assertEqual((resp["outcome"], resp["reason"], resp["patch_path"]), ("unavailable", reason, None), mode)
            self.assert_cleaned_up()

    def test_timeout(self):
        self.f.stub(STUB_MODE="sleep")
        resp = self.lane(env={"VENDOR_TIMEOUT_S": "2"})
        self.assertEqual(resp["reason"], "timeout")
        self.assert_cleaned_up()

    def test_uncommitted_work_is_the_base_and_the_checkout_is_untouched(self):
        self.f.write("internal/foo/foo.go", "package foo // draft\n")
        self.f.stub(STUB_MODE="lane", STUB_WRITE="internal/foo/foo_test.go")
        resp = self.lane("1", "codex", "--include-uncommitted")
        self.assertEqual(resp["outcome"], "completed", resp)
        self.assertNotEqual(resp["base"], self.f.git("rev-parse", "HEAD"))
        self.assertEqual(resp["files_changed"], ["internal/foo/foo_test.go"])
        self.assertEqual(self.f.git("status", "--porcelain"), "?? internal/")

    def test_the_sandbox_hides_secrets_and_protects_the_checkout(self):
        secret_env = self.f.write(".env", "SECRET=1\n")
        secret_home = self.f.write("secret.txt", "private\n", root=self.f.home)
        pwned = os.path.join(self.f.repo, "pwned")
        git_config = os.path.join(self.f.repo, ".git", "config")
        before = read(git_config)
        probes = {"PROBE_MAIN_ENV": secret_env, "PROBE_HOME_SECRET": secret_home,
                  "PROBE_MAIN_WRITE": pwned, "PROBE_GIT_CONFIG": git_config}

        # Control: the same probe outside the sandbox reaches everything, so a
        # `denied` below is the sandbox's doing, not a broken probe.
        scratch = os.path.join(self.f.tmp, "scratch-config")
        control = dict(probes, PROBE_MAIN_WRITE=os.path.join(self.f.tmp, "control-write"), PROBE_GIT_CONFIG=scratch)
        r = subprocess.run(["bash", "-c", f". {os.path.join(self.f.bin, 'probe.sh')}"], capture_output=True, text=True,
                           env=dict(self.f.env, **control))
        self.assertEqual({e["name"]: e["result"] for e in map(json.loads, r.stdout.splitlines())},
                         dict.fromkeys(["read_main_env", "read_home_secret", "write_main", "write_git_config"], "reached"))

        self.f.stub(STUB_MODE="lane-probe", STUB_WRITE="internal/foo/foo.go", **probes)
        resp = self.lane()
        self.assertEqual(resp["outcome"], "completed", resp)
        out = read(self.f.data(f"vendor-calls/{resp['call_id']}/stdout.jsonl"))
        results = {e["name"]: e["result"] for e in map(json.loads, out.splitlines()) if e.get("type") == "probe"}
        self.assertEqual(results["read_main_env"], "denied")
        self.assertEqual(results["read_home_secret"], "denied")
        self.assertEqual(results["write_git_config"], "denied")
        self.assertFalse(os.path.exists(pwned), "a write outside the worktree reached the checkout")
        self.assertEqual(read(git_config), before)


if __name__ == "__main__":
    unittest.main()
