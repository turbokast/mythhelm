#!/usr/bin/env bash
# Run examples execution rejection fixtures in temporary directories. No network,
# credentials or real documentation commands; exit nonzero on any failed test.
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
python3 - "$root" <<'PY'
import html
import json
import re
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch
repository = Path(sys.argv.pop())
sys.path.insert(0, str(repository / 'scripts/ci'))
import check_docs_examples as examples

class Examples(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.docs = self.root / 'docs'
        self.site = self.root / '_site'
        (self.docs / '_data').mkdir(parents=True)
        self.site.mkdir()

    def fixture(self, command='printf "hello\\n"', output=None, expected=0):
        anchor = 'docs/guide.md#example[1]'
        row = dict(anchor=anchor, command=command, expect_exit=expected)
        blocks = [(anchor, command)]
        if output is not None:
            row['output_anchor'] = 'docs/guide.md#example[2]'
            blocks.append((row['output_anchor'], output))
        (self.docs / '_data/examples.yml').write_text(json.dumps([row]))
        (self.docs / 'guide.md').write_text('## Example\n' + ''.join(
            f'<div class="example-block" data-example-anchor="{a}">\n\n```\n{b}\n```\n\n</div>\n'
            for a, b in blocks))
        (self.site / 'guide.html').write_text(''.join(
            f'<div class="example-block" data-example-anchor="{a}"><pre>{html.escape(b)}\n</pre></div>'
            for a, b in blocks))

    def check(self):
        def execute(command):
            return subprocess.run(['bash', '--noprofile', '--norc', '-c', command],
                cwd=self.root, env={'PATH': '/usr/bin:/bin'}, stdin=subprocess.DEVNULL,
                capture_output=True, timeout=5)
        return examples.check(self.docs, self.site, execute)

    def test_artifact_integrity(self):
        self.fixture()
        original = examples.site_digest(self.site)
        (self.site / 'guide.html').write_text('changed')
        self.assertNotEqual(original, examples.site_digest(self.site))
        (self.site / 'guide.html').write_text('mythhelm-fixture')
        with self.assertRaises(examples.InputError):
            examples.site_digest(self.site)

    def test_artifact_symlink(self):
        self.fixture()
        (self.site / 'link').symlink_to(self.docs)
        with self.assertRaises(examples.InputError):
            examples.site_digest(self.site)

    def test_workflow_credentials_and_trigger(self):
        workflows = self.root / '.github/workflows'
        workflows.mkdir(parents=True)
        for name in ('ci', 'docs'):
            (workflows / f'{name}.yml').write_text('on: [pull_request, merge_group]')
        self.assertEqual([], examples.workflow_findings(self.root))
        for name in ('ci', 'docs'):
            for banned in ('secrets' + '.TOKEN', 'pull_request' + '_target',
                           "${{ secrets['TOKEN'] }}", '${{ github.token }}',
                           "${{ github['token'] }}", '${{ toJSON(secrets) }}'):
                path = workflows / f'{name}.yml'
                path.write_text(banned)
                self.assertTrue(examples.workflow_findings(self.root))
                path.write_text('on: [pull_request, merge_group]')

    def assert_ci_contract(self, ci):
        # Extract this workflow's block-style jobs, refusing missing/duplicate
        # keys. Comments cannot provide executable dependencies or conditions.
        body = ci.split('\njobs:\n', 1)[1]
        jobs = {}
        current = None
        for line in body.splitlines():
            line = line.split(' #', 1)[0]
            if line.lstrip().startswith('#') or not line.strip():
                continue
            match = re.fullmatch(r'  ([a-z][a-z0-9-]*):', line)
            if match:
                current = match[1]
                self.assertNotIn(current, jobs)
                jobs[current] = []
            elif current:
                jobs[current].append(line)
        site = jobs['site-build']
        gate = jobs['ci-ok']
        self.assertIn('    name: Site build', site)
        self.assertIn('    uses: ./.github/workflows/docs.yml', site)
        self.assertIn('    name: CI OK', gate)
        self.assertIn('    if: always()', gate)
        needs = [line for line in gate if line.startswith('    needs: ')]
        self.assertEqual(1, len(needs))
        self.assertRegex(needs[0], r'^    needs: \[.*\bsite-build\b.*\]$')
        conditions = [line for line in gate if line.startswith('      - if: ')]
        self.assertEqual(["      - if: contains(needs.*.result, 'failure') || contains(needs.*.result, 'cancelled') || needs.site-build.result != 'success'"], conditions)
        self.assertIn('        run: exit 1', gate)

    def test_workflow_build_and_gate_contract(self):
        ci = (repository / '.github/workflows/ci.yml').read_text()
        docs = (repository / '.github/workflows/docs.yml').read_text()
        self.assert_ci_contract(ci)
        self.assertIn('  workflow_call:', docs)
        self.assertLess(docs.index('name: Assert mirrors rendered'), docs.index('uses: actions/upload-pages-artifact@'))
        self.assertLess(docs.index('uses: actions/upload-pages-artifact@'), docs.index('uses: actions/setup-node@'))
        self.assertIn('needs: [build, examples]', docs)
        self.assertIn('--network', (repository / 'scripts/ci/check_docs_examples.py').read_text())
        self.assertEqual([], examples.workflow_findings(repository))

    def test_gate_contract_rejects_comments_and_other_jobs(self):
        ci = (repository / '.github/workflows/ci.yml').read_text()
        for line in ("    needs: [workflows, detect, go, lint, govulncheck, licenses, harness, site-build]",
                     "      - if: contains(needs.*.result, 'failure') || contains(needs.*.result, 'cancelled') || needs.site-build.result != 'success'"):
            for replacement in ('    # ' + line.strip(), ''):
                mutant = ci.replace(line, replacement) + '\n  unrelated:\n' + line + '\n'
                with self.subTest(line=line, replacement=replacement), self.assertRaises(AssertionError):
                    self.assert_ci_contract(mutant)

    def test_exclusive_modes(self):
        self.fixture()
        result = subprocess.run([sys.executable, str(repository / 'scripts/ci/check_docs_examples.py'),
                                 '--digest', '--workflows', '--site', str(self.site)],
                                capture_output=True)
        self.assertEqual(2, result.returncode)

    def test_bounded_output(self):
        for stream in ('stdout', 'stderr'):
            with self.subTest(stream=stream), self.assertRaises(examples.InputError):
                examples.bounded_run([sys.executable, '-c',
                    f'import sys; sys.{stream}.write("x" * (2 * 1024 * 1024))'])
        result = examples.bounded_run([sys.executable, '-c', 'print("hello")'])
        self.assertEqual(b'hello\n', result.stdout)
        self.assertEqual(0, result.returncode)

    def test_real_content_fixture_is_rejected(self):
        import importlib.util
        source = repository / 'scripts/ci/tests/fixtures/docs-site/test_cases.py'
        spec = importlib.util.spec_from_file_location('content_fixtures', source)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        fixture = module.SiteChecks('test_conforming')
        fixture.setUp()
        try:
            with self.assertRaises(examples.InputError):
                examples.site_digest(fixture.site)
        finally:
            fixture.doCleanups()

    def test_real_browser_fixture_is_rejected(self):
        source = (repository / 'scripts/ci/browser/test_check_layout.sh').read_text()
        generator = source.split('python3 - "$work" <<' + "'PY'\n", 1)[1].split('\nPY\n', 1)[0]
        subprocess.run([sys.executable, '-', str(self.root)], input=generator,
                       text=True, check=True, capture_output=True)
        with self.assertRaises(examples.InputError):
            examples.site_digest(self.root / 'good')

    def docker_result(self, start_exit, state, create_exit=0, cleanup_exit=0):
        container = 'a' * 64
        responses = [subprocess.CompletedProcess([], create_exit, (container + '\n').encode(), b'')]
        if create_exit:
            responses.append(subprocess.CompletedProcess([], 0, b'', b''))
        else:
            responses += [subprocess.CompletedProcess([], start_exit, b'output', b''),
                          subprocess.CompletedProcess([], 0, json.dumps(state).encode(), b''),
                          subprocess.CompletedProcess([], cleanup_exit, b'', b'')]
        with patch.object(examples.subprocess, 'run', side_effect=responses) as run:
            result = examples.docker_execute('exit ' + str(start_exit), self.root, self.root,
                                             'golang:1.27.1@sha256:' + 'a' * 64)
            create = run.call_args_list[0].args[0]
            self.assertEqual(['docker', 'create'], create[:2])
            self.assertEqual('none', create[create.index('--network') + 1])
            self.assertIn('--read-only', create)
            self.assertEqual('none', create[create.index('--log-driver') + 1])
            self.assertEqual(['-c', 'exit ' + str(start_exit)], create[-2:])
            self.assertEqual(['docker', 'rm', '-f', container], run.call_args_list[-1].args[0])
            return result

    def exited_state(self, code):
        return dict(Status='exited', Running=False, OOMKilled=False, Error='',
                    ExitCode=code, StartedAt='2026-01-01T00:00:00Z')

    def test_docker_infrastructure_codes_do_not_certify_examples(self):
        for code in (125, 126, 127):
            with self.subTest(code=code):
                with self.assertRaises(examples.InputError):
                    self.docker_result(code, dict(Status='created'))
                with self.assertRaises(examples.InputError):
                    self.docker_result(code, {}, create_exit=code)

    def test_legitimate_reserved_command_exits(self):
        for code in (125, 126, 127):
            with self.subTest(code=code):
                self.assertEqual(code, self.docker_result(code, self.exited_state(code)).returncode)

    def test_invalid_running_error_oom_state_and_cleanup_fail_closed(self):
        for field, value in [('Running', True), ('OOMKilled', True), ('Error', 'launch failed'),
                             ('StartedAt', '0001-01-01T00:00:00Z'), ('StartedAt', 'invalid'), ('ExitCode', '0')]:
            state = self.exited_state(0)
            state[field] = value
            with self.subTest(field=field), self.assertRaises(examples.InputError):
                self.docker_result(0, state)
        with self.assertRaises(examples.InputError):
            self.docker_result(0, self.exited_state(0), cleanup_exit=1)

    def test_invalid_container_id_does_not_start(self):
        with patch.object(examples.subprocess, 'run', side_effect=[
                subprocess.CompletedProcess([], 0, b'--other-container', b''),
                subprocess.CompletedProcess([], 0, b'', b'')]) as run:
            with self.assertRaises(examples.InputError):
                examples.docker_execute('true', self.root, self.root, 'image')
            self.assertEqual(2, run.call_count)
            self.assertEqual(['docker', 'rm', '-f'], run.call_args_list[-1].args[0][:3])

    def test_ambiguous_create_is_reconciled_and_cleaned(self):
        container = 'b' * 64
        for present in (True, False):
            responses = [subprocess.TimeoutExpired('docker create', 30),
                         subprocess.CompletedProcess([], 0, (container if present else '').encode(), b'')]
            if present:
                responses.append(subprocess.CompletedProcess([], 0, b'', b''))
            with self.subTest(present=present), patch.object(examples.subprocess, 'run', side_effect=responses) as run:
                with self.assertRaises(subprocess.TimeoutExpired):
                    examples.docker_execute('true', self.root, self.root, 'image')
                self.assertGreaterEqual(run.call_count, 2)
                create = run.call_args_list[0].args[0]
                name = create[create.index('--name') + 1]
                query = run.call_args_list[1].args[0]
                self.assertIn('name=^/' + name + '$', query)
                if present:
                    self.assertEqual(['docker', 'rm', '-f', container], run.call_args_list[-1].args[0])

    def test_ambiguous_create_cleanup_unavailable_blocks(self):
        with patch.object(examples.subprocess, 'run', side_effect=[
                subprocess.TimeoutExpired('docker create', 30),
                subprocess.CompletedProcess([], 1, b'', b'unavailable')]):
            with self.assertRaises(examples.InputError):
                examples.docker_execute('true', self.root, self.root, 'image')

    def test_expected_exit(self):
        self.fixture(command='exit 7')
        self.assertTrue(any('exit' in f for f in self.check()))
        self.fixture(command='exit 7', expected=7)
        self.assertEqual([], self.check())

    def test_missing_published_block(self):
        self.fixture()
        (self.site / 'guide.html').write_text('<p>no block</p>')
        self.assertTrue(any('published' in f for f in self.check()))

    def test_output_diff(self):
        self.fixture(output='different')
        self.assertTrue(any('output' in f for f in self.check()))

    def test_normalised_output(self):
        self.fixture(command="printf 'hello  \\r\\n'", output='hello')
        self.assertEqual([], self.check())

    def test_stderr_is_not_stdout(self):
        self.fixture(command="printf 'hello\\n' >&2", output='hello')
        self.assertTrue(any('output' in f for f in self.check()))

    def test_no_execution_before_validation(self):
        self.fixture()
        (self.site / 'guide.html').write_text('')
        self.assertTrue(examples.check(self.docs, self.site,
            lambda _: self.fail('executed invalid example')))

    def test_empty_manifest(self):
        (self.docs / '_data/examples.yml').write_text('[]')
        self.assertTrue(self.check())

    def test_command_drift(self):
        self.fixture()
        path = self.docs / '_data/examples.yml'
        rows = json.loads(path.read_text())
        rows[0]['command'] = 'false'
        path.write_text(json.dumps(rows))
        self.assertTrue(any('command' in f for f in self.check()))

unittest.main(verbosity=2)
PY
