"""Credential-free integration fixtures with real Git revisions and Go programs."""

import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest


SCRIPT = Path(__file__).resolve().parents[1] / "check_recordings.py"


class RecordingConsumerTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="recording-consumer-test-")
        self.addCleanup(self.tmp.cleanup)
        self.base = Path(self.tmp.name)
        self.root = self.base / "repo"
        self.root.mkdir()
        self.env = dict(os.environ)
        self.env.update(GIT_CONFIG_NOSYSTEM="1", GIT_CONFIG_GLOBAL=os.devnull,
                        GIT_AUTHOR_NAME="fixture", GIT_AUTHOR_EMAIL="fixture@example.com",
                        GIT_COMMITTER_NAME="fixture", GIT_COMMITTER_EMAIL="fixture@example.com",
                        GOWORK="off", GOPROXY="off", GOSUMDB="off")
        self.git("init", "-q")
        self.write("go.mod", "module example.com/recordingfixture\n\ngo 1.23\n")
        self.write("docs/demos/normalize.sed", "s/revision-/recorded-/g\n")
        self.write("docs/demos/hero-transcript.sh", '#!/usr/bin/env bash\nset -euo pipefail\n"$HERO_BIN" demo --check pass\n')
        (self.root / "docs/demos/hero-transcript.sh").chmod(0o755)
        self.pin_a = self.revision("A")
        self.pin_b = self.revision("B")
        self.records = [self.record("demo", self.pin_a, "A"),
                        self.record("tui", self.pin_b, "B"),
                        self.record("hero", self.pin_b, "B")]
        self.manifest()
        self.write("docs/demos/demo.transcript.txt", "recorded-A\n")
        self.write("docs/demos/tui.transcript.txt", "=== take: linear (TERM=dumb) ===\nrecorded-B\n=== take: accessible (--accessible) ===\nrecorded-B accessible\n")
        self.write("docs/demos/hero.transcript.txt", "recorded-B\n")
        self.log = self.base / "go.log"
        tools = self.base / "tools"
        tools.mkdir()
        wrapper = tools / "go"
        wrapper.write_text('#!/usr/bin/env bash\nprintf "%s\\n" "$*" >>"$FIXTURE_GO_LOG"\nif test "$1" = build && test -n "${FIXTURE_FAIL_BUILD:-}"; then exit 8; fi\nexec "$FIXTURE_REAL_GO" "$@"\n')
        wrapper.chmod(0o755)
        self.env.update(PATH=str(tools) + os.pathsep + self.env["PATH"],
                        FIXTURE_GO_LOG=str(self.log), FIXTURE_REAL_GO=shutil.which("go"))

    def write(self, relative, text):
        path = self.root / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text)

    def git(self, *args):
        return subprocess.check_output(["git", "-C", str(self.root), "-c", "commit.gpgsign=false",
                                        "-c", "core.hooksPath=/dev/null", *args], env=self.env, text=True).strip()

    def revision(self, label):
        self.write("cmd/mythhelm/main.go", '''package main
import("fmt"; "os")
func main() {
 text := "revision-''' + label + '''"
 for _, arg := range os.Args[1:] { if arg == "--accessible" { text += " accessible" } }
 fmt.Println(text)
 if os.Getenv("FIXTURE_FAIL_DEMO") != "" { os.Exit(7) }
}
''')
        self.write("qualifier_test.go", '''package fixture
import("fmt"; "os"; "testing")
func TestRevision''' + label + '''(t *testing.T) {
 if os.Getenv("FIXTURE_FAIL_QUALIFIER") != "" {
  fmt.Println("--- PASS: TestRevision''' + label + ''' (0.00s)")
  t.Fatal("deliberate failure after misleading PASS text")
 }
}
''')
        self.git("add", "--", "go.mod", "cmd", "docs", "qualifier_test.go")
        self.git("commit", "-qm", "synthetic revision " + label)
        return self.git("rev-parse", "HEAD")

    def record(self, name, pin, label):
        return {"name": name, "binary_commit": pin, "transcript": f"docs/demos/{name}.transcript.txt",
                "capabilities": [{"name": "synthetic qualifier", "qualifier": "TestRevision" + label}]}

    def manifest(self):
        self.write("docs/demos/manifest.json", json.dumps({"recordings": self.records}))

    def run_consumer(self, script=SCRIPT, **env):
        result = subprocess.run([sys.executable, str(script), "--root", str(self.root)],
                                env=dict(self.env, **env), capture_output=True, text=True, timeout=120)
        self.assertEqual(self.git("worktree", "list", "--porcelain").count("worktree "), 1,
                         "consumer leaked a Git worktree")
        return result

    def test_mixed_revisions_route_transcripts_and_qualifiers_and_reuse_builds(self):
        result = self.run_consumer()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("tui: transcript fresh at " + self.pin_b, result.stdout)
        self.assertIn("TestRevisionB passed at " + self.pin_b, result.stdout)
        calls = self.log.read_text().splitlines()
        self.assertEqual(sum(c.startswith("build ") for c in calls), 2)
        self.assertEqual(sum(c.startswith("test ") for c in calls), 2)

    def test_existing_shared_pin_contract_reuses_one_build_and_qualifier(self):
        self.records = [self.record(name, self.pin_a, "A") for name in ["demo", "tui", "hero"]]
        self.manifest()
        self.write("docs/demos/tui.transcript.txt", "=== take: linear (TERM=dumb) ===\nrecorded-A\n=== take: accessible (--accessible) ===\nrecorded-A accessible\n")
        self.write("docs/demos/hero.transcript.txt", "recorded-A\n")
        result = self.run_consumer()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        calls = self.log.read_text().splitlines()
        self.assertEqual(sum(c.startswith("build ") for c in calls), 1)
        self.assertEqual(sum(c.startswith("test ") for c in calls), 1)

    def test_missing_and_noncommit_revisions_refuse_before_build(self):
        for pin in ["f" * 40, self.git("rev-parse", "HEAD:go.mod"), "main"]:
            with self.subTest(pin=pin):
                self.records[1]["binary_commit"] = pin
                self.manifest()
                result = self.run_consumer()
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(self.log.exists(), "invalid revision reached Go")

    def test_absent_qualifier_is_not_a_passing_zero_exit(self):
        self.records[1]["capabilities"][0]["qualifier"] = "TestAbsent"
        self.manifest()
        result = self.run_consumer()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("no exact PASS witness", result.stderr)

    def test_failing_qualifier_rejects_even_with_misleading_pass_line(self):
        result = self.run_consumer(FIXTURE_FAIL_QUALIFIER="1")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("qualifier failed", result.stderr)

    def test_failed_transcript_command_rejects_matching_stdout(self):
        result = self.run_consumer(FIXTURE_FAIL_DEMO="1")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("transcript command failed", result.stderr)

    def test_failed_build_removes_its_owned_worktree(self):
        result = self.run_consumer(FIXTURE_FAIL_BUILD="1")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("pinned binary build failed", result.stderr)

    def test_failed_normalizer_rejects_matching_stdout(self):
        # A producer's matching bytes cannot hide its failing normalizer.
        sed = self.base / "tools/sed"
        sed.write_text('#!/usr/bin/env bash\ncat >/dev/null\nprintf "recorded-A\\n"\nexit 6\n')
        sed.chmod(0o755)
        result = self.run_consumer()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("normalization failed", result.stderr)

    def test_changed_transcript_refuses(self):
        self.write("docs/demos/tui.transcript.txt", "stale recorded-A\n")
        result = self.run_consumer()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("transcript differs", result.stderr)

    def test_qualifier_and_transcript_paths_are_data_not_shell_commands(self):
        self.records[1]["capabilities"][0]["qualifier"] = "TestB;touch marker"
        self.manifest()
        result = self.run_consumer()
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse((self.root / "marker").exists())
        self.assertFalse(self.log.exists())

    def test_transcript_path_refuses_alias_and_traversal_before_build(self):
        for path in ["docs/demos/hero.transcript.txt", "../outside.txt"]:
            with self.subTest(path=path):
                self.records[1]["transcript"] = path
                self.manifest()
                result = self.run_consumer()
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(self.log.exists())

    def mutant(self, old, new):
        source = SCRIPT.read_text()
        self.assertEqual(source.count(old), 1, "mutant must change exactly one mechanism")
        path = self.base / "mutant.py"
        path.write_text(source.replace(old, new, 1))
        return path

    def test_wrong_first_pin_transcript_mutant_fails_real_mixed_fixture(self):
        path = self.mutant('\n                revision = rec["binary_commit"]', '\n                revision = records[0]["binary_commit"]')
        result = self.run_consumer(path)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("transcript differs", result.stderr)

    def test_wrong_first_pin_qualifier_mutant_fails_real_mixed_fixture(self):
        path = self.mutant('qualifier_revision = revision', 'qualifier_revision = records[0]["binary_commit"]')
        result = self.run_consumer(path)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("no exact PASS witness", result.stderr)

    def test_ignored_producer_status_mutant_loses_matching_output_refusal(self):
        self.records = [self.records[0]]
        self.manifest()
        path = self.mutant('if result.returncode:\n            raise RecordingError(f"{name}: transcript command failed',
                           'if False:\n            raise RecordingError(f"{name}: transcript command failed')
        result = self.run_consumer(path, FIXTURE_FAIL_DEMO="1")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("demo: transcript fresh", result.stdout)

    def test_ignored_qualifier_status_mutant_accepts_false_pass_witness(self):
        self.records = [self.records[0]]
        self.manifest()
        path = self.mutant('if result.returncode:\n                            raise RecordingError(f"{name}: qualifier failed',
                           'if False:\n                            raise RecordingError(f"{name}: qualifier failed')
        result = self.run_consumer(path, FIXTURE_FAIL_QUALIFIER="1")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("TestRevisionA passed", result.stdout)


if __name__ == "__main__":
    unittest.main()
