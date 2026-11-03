"""Earn real gate markers, then mutate the GitHub inputs they depend on."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[3]
GATE = ROOT / 'scripts/harness/gate.sh'
LIB = ROOT / 'scripts/harness/gatelib.py'


class GitHubGateScopeTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name) / 'fixture with spaces'
        self.root.mkdir()
        self.env = dict(os.environ, GIT_CONFIG_NOSYSTEM='1', GIT_CONFIG_GLOBAL=os.devnull,
                        GIT_AUTHOR_NAME='Synthetic', GIT_AUTHOR_EMAIL='test@example.com',
                        GIT_COMMITTER_NAME='Synthetic', GIT_COMMITTER_EMAIL='test@example.com')
        for key in ('GIT_DIR', 'GIT_WORK_TREE', 'GIT_INDEX_FILE', 'CLAUDE_PROJECT_DIR'):
            self.env.pop(key, None)
        self.write('.gitignore', '.claude/data/\n__pycache__/\n')
        self.write('.github/workflows/ci.yml', 'on: [pull_request, merge_group]\n')
        self.write('.github/workflows/docs.yml', 'on: [workflow_call]\n')
        self.write('.github/scripts/policy_probe.py', 'ALLOWED = True\n')
        self.write('.github/tests/test_policy.py', '''import runpy
from pathlib import Path
source = Path(__file__).resolve().parents[1] / 'scripts/policy_probe.py'
if not runpy.run_path(str(source))['ALLOWED']:
    raise SystemExit('synthetic helper rejection')
''')
        # Use a real production consumer: this command validates the fixture's
        # actual workflow files; it launches no examples, network or inference.
        for name in ('check_docs_examples.py', 'check_docs_site.py'):
            dest = self.root / 'scripts/ci' / name
            dest.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(ROOT / 'scripts/ci' / name, dest)
        self.write('.claude/hooks/tests/run-tests.sh', '''#!/usr/bin/env bash
set -euo pipefail
python3 scripts/ci/check_docs_examples.py --workflows
python3 .github/tests/test_policy.py
''')
        (self.root / '.claude/hooks/tests/run-tests.sh').chmod(0o755)
        self.git('init', '-q', '-b', 'main')
        self.git('add', '--', '.gitignore', '.github', '.claude/hooks/tests/run-tests.sh', 'scripts/ci')
        self.git('commit', '-qm', 'synthetic fixture')
        self.base = self.git('rev-parse', 'HEAD').stdout.strip()
        # Keep a genuine harness delta so status reports its earned marker even
        # before the GitHub-only dependency change selects that gate itself.
        with (self.root / 'scripts/ci/check_docs_examples.py').open('a') as out:
            out.write('\n# Synthetic fixture baseline.\n')

    def write(self, relative, text):
        path = self.root / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text)

    def run_command(self, *args):
        return subprocess.run([str(arg) for arg in args], cwd=self.root, env=self.env,
                              capture_output=True, text=True, timeout=30)

    def git(self, *args):
        result = self.run_command('git', *args)
        self.assertEqual(result.returncode, 0, result.stderr)
        return result

    def lib(self, *args):
        return self.run_command(sys.executable, LIB, *args)

    def fingerprint(self, scope):
        result = self.lib('fingerprint', '--scope', scope)
        self.assertEqual(result.returncode, 0, result.stderr)
        return result.stdout.strip()

    def state(self):
        result = self.lib('status', '--json', '--base', self.base)
        self.assertIn(result.returncode, (0, 1), result.stderr)
        return {row['gate']: row['state'] for row in json.loads(result.stdout)}

    def earn_marker(self):
        result = self.run_command('bash', GATE, 'harness-tests')
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.state()['harness-tests'], 'fresh')
        path = self.root / '.claude/data/gate-marker-harness-tests.json'
        marker = json.loads(path.read_text())
        self.assertEqual(marker['exit_code'], 0)
        self.assertEqual(marker['fingerprint'], self.fingerprint('harness'))
        return path

    def commit_baseline(self):
        self.git('add', '--', 'scripts/ci/check_docs_examples.py')
        self.git('commit', '-qm', 'synthetic baseline delta')

    def assert_new_dependency(self, path, text, failing=False):
        marker = self.earn_marker()
        go_before = self.fingerprint('go')
        self.commit_baseline()
        self.write(path, text)
        required = self.lib('required')
        self.assertEqual(required.returncode, 0, required.stderr)
        self.assertEqual(required.stdout.strip(), 'harness-lint harness-tests hygiene')
        self.assertEqual(self.state()['harness-tests'], 'stale')
        self.assertEqual(self.fingerprint('go'), go_before)
        if failing:
            result = self.run_command('bash', GATE, 'harness-tests')
            self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertFalse(marker.exists(), 'failed consumer retained a passing marker')
        else:
            self.earn_marker()

    def test_workflow_edit_stales_real_consumer_marker_and_unsafe_workflow_fails(self):
        self.assert_new_dependency('.github/workflows/ci.yml',
                                   'on: [pull_request, merge_group]\n# legitimate edit\n')
        self.write('.github/workflows/ci.yml', 'on: [pull_request_target]\n')
        self.assertEqual(self.state()['harness-tests'], 'stale')
        result = self.run_command('bash', GATE, 'harness-tests')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('forbidden credential/trigger reference', result.stdout + result.stderr)
        self.assertFalse((self.root / '.claude/data/gate-marker-harness-tests.json').exists())

    def test_helper_edit_stales_marker_and_reaches_its_executing_test(self):
        self.assert_new_dependency('.github/scripts/policy_probe.py', 'ALLOWED = False\n', failing=True)

    def test_github_test_edit_stales_marker_and_real_gate_fails(self):
        self.assert_new_dependency('.github/tests/test_policy.py', 'raise SystemExit(7)\n', failing=True)

    def test_new_untracked_workflow_input_selects_harness_and_stales_marker(self):
        self.assert_new_dependency('.github/workflows/new.yaml', 'on: [pull_request]\n')

    def test_new_test_fixture_data_selects_harness_and_stales_marker(self):
        self.assert_new_dependency('.github/tests/fixtures/case.json', '{"synthetic":true}\n')

    def test_deleting_and_moving_tracked_inputs_cannot_keep_old_evidence(self):
        for move in (False, True):
            with self.subTest(move=move):
                marker = self.earn_marker()
                source = self.root / '.github/scripts/policy_probe.py'
                if move:
                    source.rename(self.root / 'moved-metadata.txt')
                else:
                    source.unlink()
                self.assertEqual(self.state()['harness-tests'], 'stale')
                self.assertIn('harness-tests', self.lib('required').stdout.split())
                result = self.run_command('bash', GATE, 'harness-tests')
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(marker.exists())
                self.write('.github/scripts/policy_probe.py', 'ALLOWED = True\n')
                (self.root / 'moved-metadata.txt').unlink(missing_ok=True)

    def test_github_metadata_specs_and_ignored_runtime_noise_remain_excluded(self):
        self.earn_marker()
        self.commit_baseline()
        before = self.fingerprint('harness')
        for path in ('.github/ISSUE_TEMPLATE/bug.md', '.github/PULL_REQUEST_TEMPLATE.md',
                     '.github/CODEOWNERS', '.github/README.md', 'specs/todo/demo/tasks.md'):
            with self.subTest(path=path):
                self.write(path, '# Synthetic metadata\n')
                self.assertEqual(self.lib('required').stdout.strip(), 'hygiene')
                self.assertEqual(self.fingerprint('harness'), before)
                self.assertEqual(self.state()['harness-tests'], 'fresh')
        self.write('.claude/data/ignored.json', '{"synthetic":true}\n')
        self.assertEqual(self.fingerprint('harness'), before)
        self.assertEqual(self.state()['harness-tests'], 'fresh')


if __name__ == '__main__':
    unittest.main()
