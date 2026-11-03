"""Synthetic subprocess tests for complete, bounded harness execution."""

import importlib.util
import json
import os
from pathlib import Path
import shutil
import signal
import subprocess
import tempfile
import time
import unittest
from unittest.mock import patch


ROOT = Path(__file__).resolve().parents[3]
WORKER = r'''
import json, os, pathlib, signal, subprocess, sys, time
root = pathlib.Path(__file__).resolve().parent
name, mode = sys.argv[1:]
def stopped(sig, frame):
    (root / ('stopped-' + name)).write_text(str(sig))
    sys.exit(128 + sig)
signal.signal(signal.SIGTERM, stopped)
signal.signal(signal.SIGINT, stopped)
if mode == 'child':
    (root / ('ready-' + name)).write_text(str(os.getpid()))
    while True: time.sleep(.02)
if mode == 'hang':
    subprocess.Popen([sys.executable, __file__, name + '-child', 'child'])
    (root / ('ready-' + name)).write_text(str(os.getpid()))
    while True: time.sleep(.02)
if mode == 'orphan':
    subprocess.Popen([sys.executable, __file__, name + '-orphan', 'ignore'])
    sys.exit(0)
if mode == 'ignore':
    signal.signal(signal.SIGTERM, signal.SIG_IGN)
    (root / ('ready-' + name)).write_text(str(os.getpid()))
    while True: time.sleep(.02)
lock = root / 'lock'
def update(delta):
    deadline = time.monotonic() + 5
    while True:
        try: lock.mkdir(); break
        except FileExistsError:
            if time.monotonic() > deadline: raise RuntimeError('fixture lock timeout')
            time.sleep(.005)
    try:
        p = root / 'counts.json'
        state = json.loads(p.read_text()) if p.exists() else {'active': 0, 'maximum': 0, 'started': 0}
        state['active'] += delta
        state['maximum'] = max(state['maximum'], state['active'])
        if delta > 0: state['started'] += 1
        p.write_text(json.dumps(state))
    finally: lock.rmdir()
update(1)
if mode == 'wait':
    deadline = time.monotonic() + 5
    while not (root / 'release').exists():
        if time.monotonic() > deadline: raise RuntimeError('fixture release timeout')
        time.sleep(.01)
else: time.sleep(.02)
update(-1)
print('diagnostic-' + name)
print('checks=3 failed=' + ('1' if mode == 'fail' else '0'))
sys.exit(1 if mode == 'fail' else 0)
'''


class RunnerTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name) / "suite with spaces"
        self.root.mkdir()
        self.tests = self.root / '.claude/hooks/tests'
        self.tests.mkdir(parents=True)
        self.harness = self.root / 'scripts/harness'
        self.harness.mkdir(parents=True)
        self.runner = self.tests / 'run-tests.sh'
        shutil.copyfile(ROOT / '.claude/hooks/tests/run-tests.sh', self.runner)
        source = ROOT / 'scripts/harness/run_tests.py'
        if source.exists():
            shutil.copyfile(source, self.harness / 'run_tests.py')
        (self.root / 'worker.py').write_text(WORKER)

    def fixture(self, name, mode='pass', scripts=False):
        directory = self.harness / 'tests' if scripts else self.tests
        directory.mkdir(exist_ok=True)
        path = directory / ('test_' + name + '.sh')
        path.write_text('#!/usr/bin/env bash\nexec python3 "' + str(self.root / 'worker.py') + '" ' + name + ' ' + mode + '\n')

    def start(self, *args):
        proc = subprocess.Popen(['bash', str(self.runner), *args], cwd=self.root,
                                stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True,
                                start_new_session=True)
        def cleanup():
            if proc.poll() is None:
                proc.terminate()
                try:
                    proc.communicate(timeout=8)
                except subprocess.TimeoutExpired:
                    os.killpg(proc.pid, signal.SIGKILL)
                    proc.communicate()
        self.addCleanup(cleanup)
        return proc

    def finish(self, proc, code=0):
        out, _ = proc.communicate(timeout=10)
        self.assertEqual(proc.returncode, code, out)
        return out

    def wait_for(self, predicate):
        deadline = time.monotonic() + 5
        while time.monotonic() < deadline:
            try:
                if predicate():
                    return
            except (FileNotFoundError, json.JSONDecodeError):
                pass
            time.sleep(.01)
        self.fail('synthetic worker did not reach the expected barrier')

    def counts(self):
        return json.loads((self.root / 'counts.json').read_text())

    def test_default_is_serial_and_parallel_has_identical_order_and_summary(self):
        self.fixture('z')
        self.fixture('a')
        self.fixture('b', scripts=True)
        serial = self.finish(self.start())
        self.assertEqual(self.counts(), {'active': 0, 'maximum': 1, 'started': 3})
        (self.root / 'counts.json').unlink()
        parallel = self.finish(self.start('--jobs', '2'))
        self.assertEqual(serial, parallel)
        self.assertIn('Test summary: 3 passed, 0 failed, 9 checks', parallel)
        self.assertLess(parallel.index('test_a.sh'), parallel.index('test_z.sh'))
        self.assertLess(parallel.index('test_z.sh'), parallel.index('scripts/harness/tests/test_b.sh'))

    def test_parallelism_overlaps_and_never_exceeds_requested_jobs(self):
        for name in ('a', 'b', 'c', 'd'):
            self.fixture(name, 'wait')
        proc = self.start('--jobs', '2')
        self.wait_for(lambda: self.counts()['started'] >= 2)
        time.sleep(.15)
        self.assertEqual(self.counts(), {'active': 2, 'maximum': 2, 'started': 2})
        (self.root / 'release').touch()
        out = self.finish(proc)
        self.assertEqual(self.counts(), {'active': 0, 'maximum': 2, 'started': 4})
        self.assertIn('Test summary: 4 passed, 0 failed, 12 checks', out)

    def test_delayed_descendant_output_is_counted_after_the_leader_exits(self):
        path = self.tests / 'test_orphan.sh'
        path.write_text("#!/usr/bin/env bash\n(python3 -c 'import time; time.sleep(.3); print(\"checks=7 failed=0\")') &\nexit 0\n")
        out = self.finish(self.start('--jobs', '2'))
        self.assertIn('PASS  .claude/hooks/tests/test_orphan.sh (7 checks)', out)
        self.assertIn('Test summary: 1 passed, 0 failed, 7 checks', out)

    @unittest.skipUnless(os.name == 'posix', 'Bash runner cancellation uses POSIX process groups')
    def test_termination_cleans_an_orphan_after_its_leader_exits(self):
        self.fixture('a', 'orphan')
        proc = self.start('--jobs', '2')
        ready = self.root / 'ready-a-orphan'
        self.wait_for(ready.exists)
        worker_pid = int(ready.read_text())
        os.kill(worker_pid, 0)
        try:
            proc.terminate()
            self.finish(proc, 128 + signal.SIGTERM)
            state = subprocess.run(['ps', '-o', 'stat=', '-p', str(worker_pid)],
                                   capture_output=True, text=True, check=False).stdout.strip()
            self.assertTrue(not state or state.startswith('Z'), 'orphan still running: ' + state)
        finally:
            try:
                os.kill(worker_pid, signal.SIGKILL)
            except ProcessLookupError:
                pass

    def test_all_failures_are_reported_and_other_tests_still_run(self):
        self.fixture('a', 'fail')
        self.fixture('b')
        self.fixture('c', 'fail')
        out = self.finish(self.start('--jobs', '2'), 1)
        self.assertIn('Test summary: 1 passed, 2 failed, 9 checks', out)
        for name in ('a', 'c'):
            self.assertIn('FAIL  .claude/hooks/tests/test_' + name + '.sh', out)
            self.assertIn('      diagnostic-' + name, out)
            self.assertIn('  failed: .claude/hooks/tests/test_' + name + '.sh', out)
        self.assertIn('PASS  .claude/hooks/tests/test_b.sh (3 checks)', out)
        self.assertEqual(self.counts()['started'], 3)

    @unittest.skipUnless(os.name == 'posix', 'FIFO fixture requires POSIX')
    def test_discovery_excludes_symlinks_and_nonregular_files(self):
        self.fixture('a', scripts=True)
        directory = self.harness / 'tests'
        (directory / 'test_duplicate.sh').symlink_to(directory / 'test_a.sh')
        os.mkfifo(directory / 'test_pipe.sh')
        out = self.finish(self.start('--jobs', '2'))
        self.assertIn('Test summary: 1 passed, 0 failed, 3 checks', out)

    @unittest.skipUnless(os.name == 'posix', 'Worker ownership uses POSIX process groups')
    def test_pipe_setup_failure_stops_the_already_started_worker(self):
        source = self.harness / 'run_tests.py'
        spec = importlib.util.spec_from_file_location('synthetic_runner', source)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        path = self.tests / 'test_started.sh'
        path.write_text('#!/usr/bin/env bash\nexec sleep 30\n')
        started = []
        real_popen = subprocess.Popen

        def start(*args, **kwargs):
            proc = real_popen(*args, **kwargs)
            started.append(proc)
            return proc

        try:
            with patch.object(module.subprocess, 'Popen', side_effect=start), \
                 patch.object(module.selectors.DefaultSelector, 'register',
                              side_effect=RuntimeError('synthetic pipe setup failure')):
                with self.assertRaisesRegex(RuntimeError, 'synthetic pipe setup failure'):
                    module.run_suite(self.root, [path], 1)
            self.assertEqual(len(started), 1)
            self.assertIsNotNone(started[0].poll(), 'started worker escaped cleanup')
        finally:
            for proc in started:
                try:
                    os.killpg(proc.pid, signal.SIGKILL)
                except ProcessLookupError:
                    pass
                proc.wait()
                proc.stdout.close()

    def test_large_output_is_drained_without_blocking_workers(self):
        path = self.tests / 'test_large.sh'
        path.write_text("#!/usr/bin/env bash\npython3 -c 'print(\"x\" * 200000); print(\"checks=5 failed=0\")'\n")
        out = self.finish(self.start('--jobs', '2'))
        self.assertIn('Test summary: 1 passed, 0 failed, 5 checks', out)

    def test_filter_and_no_matches_preserve_existing_behavior(self):
        self.fixture('selected')
        self.fixture('other', 'fail')
        out = self.finish(self.start('selected', '--jobs', '2'))
        self.assertIn('Test summary: 1 passed, 0 failed, 3 checks', out)
        out = self.finish(self.start('--jobs', '2', 'absent'), 1)
        self.assertIn('No tests matched.', out)

    def test_invalid_jobs_and_unknown_arguments_are_rejected_before_execution(self):
        self.fixture('a')
        for args in (('--jobs', '0'), ('--jobs', '-1'), ('--jobs', '65'),
                     ('--jobs', 'two'), ('--jobs',), ('--unknown',), ('a', 'b')):
            with self.subTest(args=args):
                self.finish(self.start(*args), 2)
        self.assertFalse((self.root / 'counts.json').exists())

    @unittest.skipUnless(os.name == 'posix', 'Bash runner cancellation uses POSIX process groups')
    def test_termination_reaches_workers_and_their_descendants(self):
        self.fixture('a', 'hang')
        self.fixture('b', 'hang')
        proc = self.start('--jobs', '2')
        for name in ('a', 'b', 'a-child', 'b-child'):
            self.wait_for(lambda name=name: (self.root / ('ready-' + name)).exists())
        proc.terminate()
        out = self.finish(proc, 128 + signal.SIGTERM)
        self.assertIn('interrupted', out)
        for name in ('a', 'b', 'a-child', 'b-child'):
            self.assertEqual((self.root / ('stopped-' + name)).read_text(), str(signal.SIGTERM))

    @unittest.skipUnless(os.name == 'posix', 'Bash runner cancellation uses POSIX process groups')
    def test_termination_escalates_and_reaps_a_worker_ignoring_term(self):
        self.fixture('a', 'ignore')
        proc = self.start('--jobs', '2')
        ready = self.root / 'ready-a'
        self.wait_for(ready.exists)
        worker_pid = int(ready.read_text())
        os.kill(worker_pid, 0)  # Establish that the exact worker exists before checking cleanup.
        proc.terminate()
        self.finish(proc, 128 + signal.SIGTERM)
        with self.assertRaises(ProcessLookupError):
            os.kill(worker_pid, 0)


if __name__ == '__main__':
    unittest.main()
