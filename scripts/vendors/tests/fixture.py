"""fixture.py: a throwaway repository for the vendor tests.

Each Fixture copies the vendor scripts (scripts/vendors, scripts/codex,
scripts/jev) and the tracked policy seed into a fresh `git init` under a temporary
directory, so every state file the scripts write (.claude/data/...) lands in the
fixture, never in the real checkout. PATH holds only the stub CLIs, python3 and
the system directories: a real vendor CLI can never be reached, HOME is a
temporary directory, and TYPESAFE_API_KEY is unset unless a test sets it.
"""

from __future__ import annotations

import json
import os
import shutil
import subprocess
import sys
import tempfile

TESTS_DIR = os.path.dirname(os.path.abspath(__file__))
REPO_ROOT = os.path.normpath(os.path.join(TESTS_DIR, "..", "..", ".."))
STUB_BIN = os.path.join(TESTS_DIR, "fixtures", "bin")

GITIGNORE = """.claude/data/*
!.claude/data/*.schema.json
!.claude/data/vendor-policy.json
.claude/worktrees/
.env
__pycache__/
"""


class Fixture:
    def __init__(self):
        self.tmp = tempfile.mkdtemp(prefix="vendor-test-")
        self.home = os.path.join(self.tmp, "home")
        self.repo = os.path.join(self.tmp, "work", "repo")
        self.bin = os.path.join(self.tmp, "bin")
        os.makedirs(self.home)
        os.makedirs(self.repo)
        shutil.copytree(STUB_BIN, self.bin)
        for rel in ("scripts/vendors", "scripts/codex", "scripts/jev"):
            shutil.copytree(os.path.join(REPO_ROOT, rel), os.path.join(self.repo, rel),
                            ignore=shutil.ignore_patterns("tests", "__pycache__"))
        os.makedirs(os.path.join(self.repo, ".claude", "data"))
        shutil.copy(os.path.join(REPO_ROOT, ".claude", "data", "vendor-policy.json"),
                    os.path.join(self.repo, ".claude", "data", "vendor-policy.json"))
        self.write(".gitignore", GITIGNORE)
        self.write("main.go", "package main\n\nfunc main() { return }\n")
        self.env = {
            "PATH": os.pathsep.join([self.bin, os.path.dirname(sys.executable), "/usr/bin", "/bin"]),
            "HOME": self.home, "LANG": "C.UTF-8", "TMPDIR": self.tmp, "PYTHONDONTWRITEBYTECODE": "1",
            "GIT_CONFIG_GLOBAL": "/dev/null", "GIT_CONFIG_NOSYSTEM": "1",
            "GIT_AUTHOR_NAME": "test", "GIT_AUTHOR_EMAIL": "test@example.com",
            "GIT_COMMITTER_NAME": "test", "GIT_COMMITTER_EMAIL": "test@example.com",
        }
        self.git("init", "-q", "-b", "main")
        self.git("add", "-A")
        self.git("commit", "-q", "-m", "fixture")

    def cleanup(self):
        subprocess.run(["chmod", "-R", "u+w", self.tmp], capture_output=True)
        shutil.rmtree(self.tmp, ignore_errors=True)

    # ---- helpers -------------------------------------------------------------------------

    def write(self, rel, text, root=None):
        path = os.path.join(root or self.repo, rel)
        os.makedirs(os.path.dirname(path), exist_ok=True)
        with open(path, "w", encoding="utf-8") as fh:
            fh.write(text)
        return path

    def git(self, *args, cwd=None):
        r = subprocess.run(["git", "-C", cwd or self.repo, *args], capture_output=True, text=True, env=self.env)
        if r.returncode != 0:
            raise RuntimeError(f"git {args}: {r.stderr}")
        return r.stdout.strip()

    def stub(self, **values):
        lines = [f"{k}={json.dumps(str(v))}" for k, v in values.items()]
        self.write("stub.env", "\n".join(lines) + "\n", root=self.bin)

    def enable(self, *vendors, lanes=(), **extra):
        local = {"enabled": list(vendors), "lanes_enabled": list(lanes), **extra}
        self.write(".claude/data/vendor-policy.local.json", json.dumps(local))

    def worktree(self, name="wt"):
        path = os.path.join(self.tmp, name)
        self.git("worktree", "add", "-q", "--detach", path, "HEAD")
        return path

    def run(self, rel, *args, cwd=None, env=None, timeout=120):
        run_env = dict(self.env, **(env or {}))
        path = os.path.join(self.repo, rel)
        argv = ([sys.executable, path] if path.endswith(".py") else ["bash", path]) + list(args)
        r = subprocess.run(argv, cwd=cwd or self.repo, capture_output=True, text=True, env=run_env, timeout=timeout)
        return r.returncode, r.stdout, r.stderr

    def run_json(self, rel, *args, **kw):
        rc, out, err = self.run(rel, *args, **kw)
        lines = [ln for ln in out.splitlines() if ln.strip()]
        if rc != 0 or len(lines) != 1:
            raise AssertionError(f"{rel} rc={rc} stdout={out!r} stderr={err[-2000:]!r}")
        return json.loads(lines[0])

    def data(self, rel):
        return os.path.join(self.repo, ".claude", "data", rel)

    def rows(self, name="vendor-calls.jsonl"):
        path = self.data(name)
        if not os.path.exists(path):
            return []
        with open(path, encoding="utf-8") as fh:
            return [json.loads(ln) for ln in fh if ln.strip()]


def read(path, encoding="utf-8"):
    with open(path, encoding=encoding) as fh:
        return fh.read()


def load_vendors_module():
    sys.path.insert(0, os.path.join(REPO_ROOT, "scripts", "vendors"))
    import vendors  # noqa: E402
    return vendors
