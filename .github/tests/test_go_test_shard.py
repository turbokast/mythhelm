"""Offline tests for the Windows package partition and failure propagation."""
import importlib.util
from pathlib import Path
import subprocess
import unittest
from unittest import mock

spec = importlib.util.spec_from_file_location("shard", Path(__file__).resolve().parents[1] / "scripts" / "go-test-shard.py")
s = importlib.util.module_from_spec(spec)
spec.loader.exec_module(s)


class ShardTests(unittest.TestCase):
    def test_complete_disjoint(self):
        packages = ["example.test/project/a", s.SUPERVISOR, "example.test/project/b"]
        raw = "\n".join(packages) + "\n"
        left, right = s.select_packages(raw, "supervisor"), s.select_packages(raw, "rest")
        self.assertFalse(set(left) & set(right))
        self.assertEqual(set(left + right), set(packages))

    def test_bad_lists_refuse(self):
        for raw in ("", "\n", "a\na\n", "a\n", s.SUPERVISOR + "\n", s.SUPERVISOR + "\na\na\n", s.SUPERVISOR + "\ninvalid name\n", s.SUPERVISOR + "\n\na\n"):
            with self.subTest(raw=raw), self.assertRaises(ValueError):
                s.select_packages(raw, "rest")

    def test_go_list_failure_prevents_tests(self):
        with mock.patch.object(s.subprocess, "run", side_effect=subprocess.CalledProcessError(2, ["go", "list"])) as run:
            self.assertNotEqual(s.main(["rest"]), 0)
            self.assertEqual(run.call_count, 1)

    def test_malformed_list_never_runs_go_test(self):
        listing = subprocess.CompletedProcess([], 0, stdout="example.test/project/a\n")
        with mock.patch.object(s.subprocess, "run", return_value=listing) as run:
            self.assertNotEqual(s.main(["supervisor"]), 0)
            self.assertEqual(run.call_count, 1)

    def test_required_workflow_wiring(self):
        workflow = (Path(__file__).resolve().parents[1] / "workflows" / "ci.yml").read_text()
        job = workflow.split("  ci-ok:\n", 1)[1]
        self.assertIn("go, windows-supervisor,", job)
        self.assertIn("contains(needs.*.result, 'failure')", job)
        self.assertIn("contains(needs.*.result, 'cancelled')", job)
        self.assertIn("needs.go.result != 'success'", job)
        self.assertIn("needs.windows-supervisor.result != 'success'", job)
        supervisor = workflow.split("  windows-supervisor:\n", 1)[1].split("  lint:\n", 1)[0]
        self.assertIn("python .github/scripts/go-test-shard.py supervisor --race", supervisor)
        self.assertIn("python .github/scripts/go-test-shard.py supervisor\n", supervisor)

    def test_windows_architecture_shard_pairs(self):
        workflow = (Path(__file__).resolve().parents[1] / "workflows" / "ci.yml").read_text()
        go = workflow.split("  go:\n", 1)[1].split("  windows-supervisor:\n", 1)[0]
        supervisor = workflow.split("  windows-supervisor:\n", 1)[1].split("  lint:\n", 1)[0]
        for block, shard in ((go, "rest"), (supervisor, "supervisor")):
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
        expression = expression.replace("needs.detect.outputs.go", "detected")
        expression = expression.replace("needs.go.result", "go")
        expression = expression.replace("||", " or ").replace("&&", " and ")
        for detected, go, supervisor, expected in (
            ("true", "success", "success", False),
            ("true", "skipped", "success", True),
            ("true", "success", "skipped", True),
            ("true", "failure", "success", True),
            ("true", "success", "cancelled", True),
            ("false", "skipped", "skipped", False),
        ):
            with self.subTest(detected=detected, go=go, supervisor=supervisor):
                variables = {"detected": detected, "go": go, "supervisor": supervisor,
                             "site": "success", "results": [go, supervisor, "success"]}
                self.assertEqual(eval(expression, {"__builtins__": {}}, variables), expected)

    def test_test_exit_and_flags(self):
        listing = subprocess.CompletedProcess([], 0, stdout=s.SUPERVISOR + "\nexample.test/project/a\n")
        for argv, expected in ((["supervisor", "--race"], ["go", "test", "-race", "-p", "1", s.SUPERVISOR]),
                               (["rest"], ["go", "test", "-p", "1", "example.test/project/a"])):
            with self.subTest(argv=argv), mock.patch.object(s.subprocess, "run", side_effect=[listing, subprocess.CompletedProcess([], 7)]) as run:
                self.assertEqual(s.main(argv), 7)
                self.assertEqual(run.call_args.args[0], expected)


if __name__ == "__main__":
    unittest.main()
