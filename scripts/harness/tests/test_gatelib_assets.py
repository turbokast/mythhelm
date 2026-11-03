#!/usr/bin/env python3
"""Exercise runtime/test-source gate scope in disposable Git repositories.

Runs real Go for the embedded-SQL regression when local race tools are available;
only that integration case skips missing prerequisites. Scope tests always run.
No inference or network dependencies. Exit 0 on success, nonzero on failure.
External documentation consistency remains a separate CI concern.
"""
import json
import os
from pathlib import Path
import shutil
import shlex
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import gatelib


ASSETS = (
    "internal/journal/migrations/0001_init.sql",
    "adapters/claudecode/testdata/qualification/routes.json",
    "adapters/codex/testdata/qualification/routes.json",
    "internal/processsupport/SUPPORT.md",
    "mods/themes/dark.toml",
    "adapters/fake/scenarios/happy.json",
    "internal/workers/testdata/worker-json-unix.golden",
    "internal/v2contract/testdata/records/run.golden.json",
    "internal/control/SUPPORT.md",
    "internal/migrate/SUPPORT.md",
    "internal/v2contract/SUPPORT.md",
    "cmd/example/testdata/input.txt",
    "tests/e2e/testdata/input.json",
)
METADATA = (
    "specs/in-progress/example/tasks.md",
    "specs/implemented/example/scratchpad.md",
    "product/backlog.md",
    "orchestration/README.md",
    "knowledge/domains.md",
    "docs/harness/measured-delivery.md",
    "docs/limitations.md",
    "README.md",
    "CHANGELOG.md",
)


class AssetScopeTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="gatelib-assets-")
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.env = dict(os.environ, GIT_CONFIG_NOSYSTEM="1", GIT_CONFIG_GLOBAL=os.devnull,
                        GIT_AUTHOR_NAME="Fixture", GIT_AUTHOR_EMAIL="fixture@example.com",
                        GIT_COMMITTER_NAME="Fixture", GIT_COMMITTER_EMAIL="fixture@example.com")
        for key in ("GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE"):
            self.env.pop(key, None)
        self.git("init", "-q")
        self.write(".gitignore", ".claude/data/\n")
        self.git("add", "--", ".gitignore")
        self.git("commit", "-qm", "Synthetic fixture")

    def git(self, *args):
        return subprocess.run(["git", "-C", str(self.root), *args], env=self.env,
                              check=True, capture_output=True, text=True).stdout

    def write(self, rel, text):
        path = self.root / rel
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text, encoding="utf-8")
        return path

    def fingerprint(self):
        return gatelib.fingerprint(str(self.root), "go")

    def test_embedded_sql_post_gate_mutation_is_stale(self):
        # This process never installs a stand-in go. Invoke the actual gate and
        # verify its recorded command, tree identity and successful exit.
        if shutil.which("go") is None:
            self.skipTest("embedded-SQL integration requires local Go")
        local_env = dict(os.environ, GOTOOLCHAIN="local", GOPROXY="off")
        config_result = subprocess.run(
            ["go", "env", "-json", "CGO_ENABLED", "GOOS", "GOARCH", "CC"],
            cwd=self.root, env=local_env, check=True, capture_output=True, text=True, timeout=30)
        config = json.loads(config_result.stdout)
        self.assertIn(config["CGO_ENABLED"], ("0", "1"))
        if config["CGO_ENABLED"] == "0":
            self.skipTest("embedded-SQL integration requires CGO_ENABLED=1")
        # Go accepts a quoted compiler command. Preserve Windows path separators
        # rather than applying POSIX shell backslash unescaping.
        compiler_words = shlex.split(config["CC"], posix=False)
        compiler = compiler_words[0] if compiler_words else ""
        if len(compiler) >= 2 and compiler[0] == compiler[-1] and compiler[0] in "\"'":
            compiler = compiler[1:-1]
        if not compiler or shutil.which(compiler) is None:
            self.skipTest("embedded-SQL integration requires its configured C compiler")
        self.write("go.mod", "module example.com/embedfixture\n\ngo 1.23\n")
        self.write("internal/journal/migrations/0001_init.sql", "ok\n")
        self.write("asset.go", 'package fixture\nimport "embed"\n'
                   '//go:embed internal/journal/migrations/*.sql\nvar assets embed.FS\n')
        self.write("asset_test.go", 'package fixture\nimport "testing"\n'
                   'func TestSQL(t *testing.T) { b,e:=assets.ReadFile("internal/journal/migrations/0001_init.sql"); '
                   'if e!=nil || string(b)!="ok\\n" { t.Fatalf("SQL content: %q %v",b,e) } }\n')
        self.git("add", "--", "go.mod", "asset.go", "asset_test.go", "internal/journal/migrations/0001_init.sql")
        self.git("commit", "-qm", "Embed synthetic SQL")
        # The fixture has no dependencies; forbid automatic toolchain downloads.
        with mock.patch.dict(os.environ, {"GOTOOLCHAIN": "local", "GOPROXY": "off"}):
            # Ask this installed toolchain, not a version-independent target
            # table. Only its exact unsupported-race diagnostic permits a skip;
            # compiler/build/tool failures remain failures with their output.
            preflight = subprocess.run(["go", "test", "-race", "-run", "^$", "./..."],
                                       cwd=self.root, env=local_env, capture_output=True,
                                       text=True, timeout=60)
            unsupported = f'-race is not supported on {config["GOOS"]}/{config["GOARCH"]}'
            if preflight.returncode != 0 and preflight.stderr.strip() == unsupported:
                self.skipTest("embedded-SQL integration: " + unsupported)
            self.assertEqual(preflight.returncode, 0, preflight.stdout + preflight.stderr)
            self.assertEqual(gatelib.run_gate(str(self.root), "go-test"), 0)
            marker = gatelib.read_marker(str(self.root), "go-test")
            self.assertEqual(marker["command"], ["go", "test", "-race", "./..."])
            self.assertEqual(marker["tree"], str(self.root))
            self.assertEqual(marker["exit_code"], 0)
            self.assertEqual(gatelib.marker_state(str(self.root), "go-test", {}), "fresh")
            self.write("internal/journal/migrations/0001_init.sql", "bad\n")
            # Prove the mutation affects executable Go behavior before checking
            # freshness; an unrelated text change would not establish this bug.
            result = subprocess.run(marker["command"], cwd=self.root, capture_output=True, text=True)
            self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertIn('SQL content: "bad\\n"', result.stdout + result.stderr)
            self.assertEqual(gatelib.marker_state(str(self.root), "go-test", {}), "stale")
            required = gatelib.required_gates(str(self.root), gatelib.changed_files(str(self.root)))
            self.assertIn("go-test", required)

    def test_runtime_assets_mutation_addition_deletion_and_rename(self):
        for rel in ASSETS:
            self.write(rel, "original\n")
        self.git("add", "--", *ASSETS)
        self.git("commit", "-qm", "Synthetic runtime assets")
        before = self.fingerprint()
        for rel in ASSETS:
            with self.subTest(path=rel):
                self.assertTrue(gatelib.in_scope(rel, "go"))
                path = self.write(rel, "changed\n")
                self.assertNotEqual(self.fingerprint(), before)
                path.write_text("original\n")
                self.assertEqual(self.fingerprint(), before)
                path.unlink()
                self.assertNotEqual(self.fingerprint(), before)
                path.write_text("original\n")
                renamed = path.with_name(path.name + ".renamed")
                path.rename(renamed)
                self.assertNotEqual(self.fingerprint(), before)
                renamed.rename(path)
                added = self.write(rel + ".new", "new input\n")
                self.assertNotEqual(self.fingerprint(), before)
                added.unlink()
                self.assertEqual(self.fingerprint(), before)
                self.assertIn("go-test", gatelib.required_gates(str(self.root), [rel]))

    def test_lifecycle_and_documentation_metadata_stays_out(self):
        before = self.fingerprint()
        for rel in METADATA:
            with self.subTest(path=rel):
                self.write(rel, "metadata\n")
                self.assertFalse(gatelib.in_scope(rel, "go"))
                self.assertEqual(self.fingerprint(), before)
                self.assertNotIn("go-test", gatelib.required_gates(str(self.root), [rel]))

    def test_existing_go_and_module_handling_is_preserved(self):
        for rel in ("go.mod", "go.sum", ".golangci.yml", "elsewhere/package.go"):
            with self.subTest(path=rel):
                self.assertTrue(gatelib.in_scope(rel, "go"))
                self.assertIn("go-test", gatelib.required_gates(str(self.root), [rel]))


if __name__ == "__main__":
    unittest.main()
