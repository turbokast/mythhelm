"""Offline tests for the Windows package partition and failure propagation."""
import importlib.util
import contextlib
import io
from pathlib import Path
import subprocess
import unittest
from unittest import mock

spec = importlib.util.spec_from_file_location("shard", Path(__file__).resolve().parents[1] / "scripts" / "go-test-shard.py")
s = importlib.util.module_from_spec(spec)
spec.loader.exec_module(s)
CLI = "github.com/turbokast/mythhelm/internal/cli"


class ShardTests(unittest.TestCase):
    def test_complete_disjoint(self):
        packages = ["example.test/project/a", s.SUPERVISOR, CLI, "example.test/project/b"]
        raw = "\n".join(packages) + "\n"
        groups = [s.select_packages(raw, shard) for shard in ("supervisor", "cli", "rest")]
        self.assertEqual(groups, [[s.SUPERVISOR], [CLI], [packages[0], packages[3]]])
        self.assertCountEqual(sum(groups, []), packages)
        self.assertEqual(sum(map(len, groups)), len(set(sum(groups, []))))

    def test_missing_required_groups_and_unknown_shards_refuse(self):
        for packages in ([s.SUPERVISOR, "example.test/rest"], [CLI, "example.test/rest"],
                         [s.SUPERVISOR, CLI], [s.SUPERVISOR, CLI, CLI, "example.test/rest"]):
            for shard in ("supervisor", "cli", "rest"):
                with self.subTest(packages=packages, shard=shard), self.assertRaises(ValueError):
                    s.select_packages("\n".join(packages), shard)
        raw = "\n".join([s.SUPERVISOR, CLI, "example.test/rest"])
        with self.assertRaises(ValueError):
            s.select_packages(raw, "unknown")
        with mock.patch.object(s.subprocess, "run") as run, self.assertRaises(SystemExit):
            s.main(["unknown"])
        run.assert_not_called()

    def test_bad_lists_refuse(self):
        for raw in ("", "\n", "a\na\n", "a\n", s.SUPERVISOR + "\n", s.SUPERVISOR + "\na\na\n", s.SUPERVISOR + "\n-race\n", s.SUPERVISOR + "\n-exec\n", s.SUPERVISOR + "\ninvalid name\n", s.SUPERVISOR + "\n\na\n"):
            with self.subTest(raw=raw), self.assertRaises(ValueError):
                s.select_packages(raw, "rest")
        prefix = s.SUPERVISOR + "\n" + CLI + "\n"
        for suffix in ("-race", "-exec", "invalid name", "\nexample.test/rest", "example.test/rest\nexample.test/rest"):
            with self.subTest(suffix=suffix), self.assertRaises(ValueError):
                s.select_packages(prefix + suffix, "rest")

    def test_go_list_failure_prevents_tests(self):
        failure = subprocess.CalledProcessError(2, ["go", "list"], stderr="synthetic list diagnostic\n")
        captured = io.StringIO()
        with mock.patch.object(s.subprocess, "run", side_effect=failure) as run, contextlib.redirect_stderr(captured):
            self.assertNotEqual(s.main(["rest"]), 0)
            self.assertEqual(run.call_count, 1)
        self.assertIn("synthetic list diagnostic", captured.getvalue())

    def test_malformed_list_never_runs_go_test(self):
        listing = subprocess.CompletedProcess([], 0, stdout="example.test/project/a\n")
        with mock.patch.object(s.subprocess, "run", return_value=listing) as run:
            self.assertNotEqual(s.main(["supervisor"]), 0)
            self.assertEqual(run.call_count, 1)

    def test_required_workflow_wiring(self):
        workflow = (Path(__file__).resolve().parents[1] / "workflows" / "ci.yml").read_text()
        job = workflow.split("  ci-ok:\n", 1)[1]
        self.assertIn("go, windows-supervisor, windows-cli,", job)
        self.assertIn("contains(needs.*.result, 'failure')", job)
        self.assertIn("contains(needs.*.result, 'cancelled')", job)
        self.assertIn("needs.go.result != 'success'", job)
        self.assertIn("needs.windows-supervisor.result != 'success'", job)
        self.assertIn("needs.windows-cli.result != 'success'", job)
        supervisor = workflow.split("  windows-supervisor:\n", 1)[1].split("  windows-cli:\n", 1)[0]
        self.assertIn("python .github/scripts/go-test-shard.py supervisor --race", supervisor)
        self.assertIn("python .github/scripts/go-test-shard.py supervisor\n", supervisor)

    def test_windows_architecture_shard_pairs(self):
        workflow = (Path(__file__).resolve().parents[1] / "workflows" / "ci.yml").read_text()
        go = workflow.split("  go:\n", 1)[1].split("  windows-supervisor:\n", 1)[0]
        supervisor = workflow.split("  windows-supervisor:\n", 1)[1].split("  windows-cli:\n", 1)[0]
        cli = workflow.split("  windows-cli:\n", 1)[1].split("  lint:\n", 1)[0]
        for block, shard in ((go, "rest"), (supervisor, "supervisor"), (cli, "cli")):
            if shard != "rest":
                self.assertIn("        os: [windows-latest, windows-11-arm]", block)
            self.assertIn("    needs: detect", block)
            self.assertIn("    if: needs.detect.outputs.go == 'true'", block)
            self.assertIn("    runs-on: ${{ matrix.os }}", block)
            self.assertNotIn("continue-on-error:", block)
            pairs = []
            lines = block.splitlines()
            for index, line in enumerate(lines):
                if "run: python .github/scripts/go-test-shard.py " in line:
                    self.assertGreater(index, 0)
                    pairs.append((lines[index - 1].strip(), line.strip()))
            self.assertEqual(pairs, [
                ("- if: matrix.os == 'windows-latest'", f"run: python .github/scripts/go-test-shard.py {shard} --race"),
                ("- if: matrix.os == 'windows-11-arm'", f"run: python .github/scripts/go-test-shard.py {shard}"),
            ])

    def test_aggregate_condition_from_workflow(self):
        workflow = (Path(__file__).resolve().parents[1] / "workflows" / "ci.yml").read_text()
        job = workflow.split("  ci-ok:\n", 1)[1]
        expression = next(line.strip()[6:] for line in job.splitlines() if line.strip().startswith("- if: "))
        expression = expression.replace("contains(needs.*.result, 'failure')", "('failure' in results)")
        expression = expression.replace("contains(needs.*.result, 'cancelled')", "('cancelled' in results)")
        expression = expression.replace("needs.site-build.result", "site")
        expression = expression.replace("needs.windows-supervisor.result", "supervisor")
        expression = expression.replace("needs.windows-cli.result", "cli")
        expression = expression.replace("needs.detect.outputs.go", "detected")
        expression = expression.replace("needs.go.result", "go")
        expression = expression.replace("||", " or ").replace("&&", " and ")
        for detected, go, supervisor, cli, expected in (
            ("true", "success", "success", "success", False),
            ("true", "skipped", "success", "success", True),
            ("true", "success", "skipped", "success", True),
            ("true", "failure", "success", "success", True),
            ("true", "success", "cancelled", "success", True),
            ("true", "success", "success", "skipped", True),
            ("true", "success", "success", "failure", True),
            ("true", "success", "success", "cancelled", True),
            ("false", "skipped", "skipped", "skipped", False),
        ):
            with self.subTest(detected=detected, go=go, supervisor=supervisor):
                variables = {"detected": detected, "go": go, "supervisor": supervisor, "cli": cli,
                             "site": "success", "results": [go, supervisor, cli, "success"]}
                self.assertEqual(eval(expression, {"__builtins__": {}}, variables), expected)

    def test_test_exit_and_flags(self):
        listing = subprocess.CompletedProcess([], 0, stdout=s.SUPERVISOR + "\n" + CLI + "\nexample.test/project/a\n")
        for argv, expected in ((["supervisor", "--race"], ["go", "test", "-race", "-p", "1", s.SUPERVISOR]),
                               (["cli", "--race"], ["go", "test", "-race", "-p", "1", CLI]),
                               (["cli"], ["go", "test", "-p", "1", CLI]),
                               (["rest"], ["go", "test", "-p", "1", "example.test/project/a"])):
            with self.subTest(argv=argv), mock.patch.object(s.subprocess, "run", side_effect=[listing, subprocess.CompletedProcess([], 7)]) as run:
                self.assertEqual(s.main(argv), 7)
                self.assertEqual(run.call_args.args[0], expected)
                for call in run.call_args_list:
                    self.assertNotIn("shell", call.kwargs)

    def test_os_error_is_not_suppressed(self):
        with mock.patch.object(s.subprocess, "run", side_effect=OSError("synthetic unavailable")), \
                contextlib.redirect_stderr(io.StringIO()) as captured:
            self.assertNotEqual(s.main(["cli"]), 0)
        self.assertIn("synthetic unavailable", captured.getvalue())


if __name__ == "__main__":
    unittest.main()
