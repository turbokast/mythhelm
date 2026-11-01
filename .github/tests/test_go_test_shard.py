"""Offline tests for the Windows package partition and failure propagation."""
import importlib.util
import contextlib
import io
import json
import os
import sys
import tempfile
import threading
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
        for argv, expected in ((["supervisor", "--race"], ["go", "test", "-json", "-race", "-p", "1", s.SUPERVISOR]),
                               (["cli", "--race"], ["go", "test", "-json", "-race", "-p", "1", CLI]),
                               (["cli"], ["go", "test", "-json", "-p", "1", CLI]),
                               (["rest"], ["go", "test", "-json", "-p", "1", "example.test/project/a"])):
            with self.subTest(argv=argv), mock.patch.object(s.subprocess, "run", return_value=listing) as run, \
                    mock.patch.object(s, "run_tests", return_value=7) as tests:
                self.assertEqual(s.main(argv), 7)
                self.assertEqual(tests.call_args.args[0], expected)
                run.assert_called_once()
                self.assertNotIn("shell", tests.call_args.kwargs)

    def test_os_error_is_not_suppressed(self):
        with mock.patch.object(s.subprocess, "run", side_effect=OSError("synthetic unavailable")), \
                contextlib.redirect_stderr(io.StringIO()) as captured:
            self.assertNotEqual(s.main(["cli"]), 0)
        self.assertIn("synthetic unavailable", captured.getvalue())


class TimingTests(unittest.TestCase):
    def child(self, code):
        output = io.StringIO()
        with contextlib.redirect_stdout(output):
            status = s.run_tests([sys.executable, "-u", "-c", code])
        return status, output.getvalue()

    def test_real_main_forced_cp1252_preserves_unicode_and_child_status(self):
        message = "punctuation \u201b and emoji \U0001f680\n"
        record = json.dumps({'Action': 'output', 'Package': 'p', 'Output': message})
        producer = 'import sys; print(' + repr(record) + '); sys.exit(7)'
        script = str(Path(s.__file__).resolve())
        # Only the unavailable Go executable/listing is substituted. The parent
        # interpreter, main, text streams and collector child are real processes.
        parent = """import importlib.util, subprocess, sys
assert sys.stdout.encoding.lower() == 'cp1252', sys.stdout.encoding
spec = importlib.util.spec_from_file_location('shard', %r)
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)
popen = subprocess.Popen
def listing(command, **kwargs):
    assert command == ['go', 'list', './...'], command
    return subprocess.CompletedProcess(command, 0, stdout=m.SUPERVISOR+'\\n'+m.CLI+'\\np\\n')
def producer(command, **kwargs):
    assert command == ['go','test','-json','-race','-p','1',m.CLI], command
    return popen([sys.executable, '-c', %r], **kwargs)
m.subprocess.run = listing
m.subprocess.Popen = producer
status = m.main(['cli', '--race'])
sys.stderr.write(%r)
raise SystemExit(status)
""" % (script, producer, message)
        result = subprocess.run([sys.executable, '-c', parent], capture_output=True,
                                env={**os.environ, 'PYTHONIOENCODING': 'cp1252'}, timeout=10)
        self.assertEqual(result.stdout, message.encode('utf-8'))
        self.assertEqual(result.stderr, message.encode('utf-8'))
        self.assertEqual(result.returncode, 7)

    def test_real_child_output_failure_and_deterministic_summary(self):
        events = [
            {"Action": "output", "Package": "p", "Output": "failure diagnostic\n"},
            {"Action": "pass", "Package": "z", "Test": "TestB", "Elapsed": 2.5},
            {"Action": "fail", "Package": "a", "Test": "TestA", "Elapsed": 2.5},
            {"Action": "pass", "Package": "p", "Test": "TestFast", "Elapsed": 0.1},
            {"Action": "pass", "Package": "p", "Elapsed": 1000},
        ]
        code = "import sys; print('compiler stderr', file=sys.stderr); " + \
               "print(" + repr("\n".join(json.dumps(e) for e in events)) + "); sys.exit(7)"
        status, output = self.child(code)
        self.assertEqual(status, 7)
        self.assertIn("compiler stderr", output)
        self.assertIn("failure diagnostic\n", output)
        self.assertLess(output.index('"a" "TestA"'), output.index('"z" "TestB"'))
        self.assertLess(output.index('"z" "TestB"'), output.index('"p" "TestFast"'))
        self.assertNotIn("1000.000", output)
        self.assertIn("nested tests overlap", output)

    def test_success_malformed_and_invalid_elapsed_keep_output_and_status(self):
        lines = ['not JSON', '{broken', '[1,2]',
                 json.dumps({"Action": "output", "Output": "human output\n"}),
                 json.dumps({"Action": "pass", "Package": "p", "Test": "TestNan", "Elapsed": float('nan')}),
                 json.dumps({"Action": "pass", "Package": "p", "Test": "TestNegative", "Elapsed": -1}),
                 json.dumps({"Action": "pass", "Package": "p", "Test": "TestBool", "Elapsed": True})]
        status, output = self.child("print(" + repr("\n".join(lines)) + ")")
        self.assertEqual(status, 0)
        for text in ['not JSON', '{broken', '[1,2]', 'human output']:
            self.assertIn(text, output)
        self.assertNotIn('Slowest completed', output)
        for name in ('TestNan', 'TestNegative', 'TestBool'):
            self.assertIn(name, output)

    def test_structured_malformed_actions_remain_visible_and_keep_exit(self):
        records = [{'Action': [], 'Output': 'array action'},
                   {'Action': {'bad': 'action'}, 'Output': 'object action'}]
        code = "import sys; print(" + repr('\n'.join(json.dumps(e) for e in records)) + "); sys.exit(7)"
        status, output = self.child(code)
        self.assertEqual(status, 7)
        for record in records:
            self.assertIn(json.dumps(record), output)

    def test_large_real_telemetry_is_bounded_and_oversize_line_survives(self):
        lengths = []
        original = s.TimingSummary.add

        def observed(summary, event):
            result = original(summary, event)
            lengths.append(len(summary.slowest))
            return result

        code = """import json
import os
print('x' * 150000)
for i in range(10000):
    print(json.dumps({'Action':'pass','Package':'p','Test':'Test%05d' % i,'Elapsed':i/10}))
"""
        with mock.patch.object(s.TimingSummary, 'add', observed):
            status, output = self.child(code)
        self.assertEqual(status, 0)
        self.assertEqual(output.count('x'), 150000)
        self.assertEqual(max(lengths), s.SLOWEST_LIMIT)
        rows = [line for line in output.splitlines() if line.startswith('  ')]
        self.assertEqual(len(rows), s.SLOWEST_LIMIT)
        self.assertIn('"Test09999"', rows[0])
        self.assertNotIn('"Test00000"', output)

    def test_human_output_streams_before_child_finishes(self):
        ready = threading.Event()

        class ObservedOutput(io.StringIO):
            def write(self, text):
                result = super().write(text)
                if 'live diagnostic' in text:
                    ready.set()
                return result

        with tempfile.TemporaryDirectory() as directory:
            release = Path(directory) / 'release'
            code = """import json, pathlib, time
print(json.dumps({'Action':'output','Output':'live diagnostic\\n'}), flush=True)
p = pathlib.Path(%r)
end = time.monotonic() + 5
while not p.exists() and time.monotonic() < end:
    time.sleep(.01)
raise SystemExit(0 if p.exists() else 9)
""" % str(release)
            output = ObservedOutput()
            results = []
            with contextlib.redirect_stdout(output):
                thread = threading.Thread(target=lambda: results.append(s.run_tests([sys.executable, '-u', '-c', code])))
                thread.start()
                try:
                    self.assertTrue(ready.wait(3), 'output buffered until child exit')
                    self.assertTrue(thread.is_alive())
                finally:
                    release.touch()
                    thread.join(7)
            self.assertFalse(thread.is_alive())
            self.assertEqual(results, [0])

    def test_summary_sink_failure_does_not_replace_child_status(self):
        class BrokenSummary(io.StringIO):
            def write(self, text):
                if 'Slowest completed' in text:
                    raise ValueError('closed summary sink')
                return super().write(text)

        record = json.dumps({'Action': 'fail', 'Package': 'p', 'Test': 'TestFail', 'Elapsed': 1})
        with contextlib.redirect_stdout(BrokenSummary()), contextlib.redirect_stderr(io.StringIO()) as errors:
            status = s.run_tests([sys.executable, '-c', 'import sys; print(' + repr(record) + '); sys.exit(7)'])
        self.assertEqual(status, 7)
        self.assertIn('timing summary unavailable', errors.getvalue())

    def test_stream_read_failure_kills_and_joins_owned_child(self):
        popen = subprocess.Popen
        children = []

        class BrokenRead:
            def __init__(self, pipe):
                self.pipe = pipe

            def readline(self, limit):
                raise OSError('synthetic read failure')

            def close(self):
                self.pipe.close()

        def launch(*args, **kwargs):
            child = popen(*args, **kwargs)
            child.stdout = BrokenRead(child.stdout)
            child.wait = mock.Mock(wraps=child.wait)
            children.append(child)
            return child

        with mock.patch.object(s.subprocess, 'Popen', side_effect=launch), \
                contextlib.redirect_stderr(io.StringIO()) as errors:
            status = s.run_tests([sys.executable, '-c', 'import time; time.sleep(60)'])
        self.assertEqual(len(children), 1)
        self.assertIsNotNone(children[0].returncode)
        self.assertNotEqual(status, 0)
        self.assertEqual(status, children[0].returncode)
        self.assertTrue(children[0].wait.called, 'owned child was not explicitly joined')
        self.assertIn('synthetic read failure', errors.getvalue())

    def test_launch_error_is_reported_by_main(self):
        listing = subprocess.CompletedProcess([], 0, stdout=s.SUPERVISOR + "\n" + CLI + "\np\n")
        with mock.patch.object(s.subprocess, 'run', return_value=listing), \
                mock.patch.object(s.subprocess, 'Popen', side_effect=OSError('cannot launch')), \
                contextlib.redirect_stderr(io.StringIO()) as errors:
            self.assertEqual(s.main(['rest']), 1)
        self.assertIn('cannot launch', errors.getvalue())


if __name__ == "__main__":
    unittest.main()
