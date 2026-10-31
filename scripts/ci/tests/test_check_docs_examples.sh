#!/usr/bin/env bash
# Run examples execution rejection fixtures in temporary directories. No network,
# credentials or real documentation commands; exit nonzero on any failed test.
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
python3 - "$root" <<'PY'
import html
import ast
import itertools
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
        self.assertFalse(any(line.strip().startswith('continue-on-error:') for line in gate))
        needs = [line for line in gate if line.startswith('    needs: ')]
        self.assertEqual(1, len(needs))
        match = re.fullmatch(r'    needs: \[([a-z0-9, -]+)\]', needs[0])
        self.assertIsNotNone(match)
        dependencies = [item.strip() for item in match[1].split(',')]
        self.assertEqual(len(dependencies), len(set(dependencies)))
        self.assertTrue({'detect', 'go', 'windows-supervisor', 'windows-cli', 'site-build'} <= set(dependencies))
        conditions = [line for line in gate if line.startswith('      - if: ')]
        self.assertEqual(1, len(conditions))
        position = gate.index(conditions[0]) + 1
        self.assertLess(position, len(gate))
        self.assertEqual('        run: exit 1', gate[position])
        expression = conditions[0].split(': ', 1)[1]
        expression = expression.replace("contains(needs.*.result, 'failure')", "('failure' in results)")
        expression = expression.replace("contains(needs.*.result, 'cancelled')", "('cancelled' in results)")
        for source, name in (('needs.site-build.result', 'site'), ('needs.windows-supervisor.result', 'supervisor'),
                             ('needs.windows-cli.result', 'cli'),
                             ('needs.detect.outputs.go', 'detected'), ('needs.go.result', 'go')):
            expression = expression.replace(source, name)
        expression = expression.replace('||', ' or ').replace('&&', ' and ')
        expression = re.sub(r'\bfalse\b', 'False', expression)
        try:
            tree = ast.parse(expression, mode='eval')
        except SyntaxError as exc:
            self.fail(f'unsupported CI condition: {exc}')
        allowed = (ast.Expression, ast.BoolOp, ast.Or, ast.And, ast.Compare, ast.Eq, ast.NotEq,
                   ast.In, ast.Name, ast.Load, ast.Constant)
        self.assertTrue(all(isinstance(node, allowed) for node in ast.walk(tree)))
        self.assertTrue({node.id for node in ast.walk(tree) if isinstance(node, ast.Name)} <=
                        {'results', 'site', 'supervisor', 'cli', 'detected', 'go'})
        compiled = compile(tree, '<ci-condition>', 'eval')
        for detected, go, supervisor, cli, site, other in itertools.product(
                ('true', 'false'), ('success', 'failure', 'cancelled', 'skipped'),
                ('success', 'failure', 'cancelled', 'skipped'),
                ('success', 'failure', 'cancelled', 'skipped'),
                ('success', 'failure', 'cancelled', 'skipped'),
                ('success', 'failure', 'cancelled')):
            values = dict(detected=detected, go=go, supervisor=supervisor, cli=cli, site=site,
                          results=[go, supervisor, cli, site, other])
            expected = ('failure' in values['results'] or 'cancelled' in values['results'] or
                        site != 'success' or (detected == 'true' and
                        (go != 'success' or supervisor != 'success' or cli != 'success')))
            self.assertEqual(expected, eval(compiled, {'__builtins__': {}}, values), values)

    def test_workflow_build_and_gate_contract(self):
        ci = (repository / '.github/workflows/ci.yml').read_text()
        docs = (repository / '.github/workflows/docs.yml').read_text()
        self.assert_ci_contract(ci)
        self.assertIn('  workflow_call:', docs)
        self.assertLess(docs.index('name: Assert mirrors rendered'), docs.index('uses: actions/upload-pages-artifact@'))
        self.assertLess(docs.index('uses: actions/upload-pages-artifact@'), docs.index('uses: actions/setup-node@'))
        upload = docs.index('uses: actions/upload-pages-artifact@')
        fixture = docs.index('name: Prepare isolated state render fixture')
        self.assertLess(upload, fixture)
        self.assertLess(docs.index('name: Record immutable site digest'), fixture)
        self.assertLess(fixture, docs.index('name: Assert checks did not modify the uploaded tree'))
        self.assertIn('cp docs/_includes/state.html .docs-state-fixture/_includes/state.html', docs)
        self.assertIn('source: ./.docs-state-fixture', docs)
        self.assertIn('destination: ./.docs-state-output', docs)
        self.assertIn('--rendered-state-fixture .docs-state-output/index.html', docs)
        self.assertIn('name: Remove isolated state fixture\n        if: always()', docs)
        self.assertIn('needs: [build, examples]', docs)
        self.assertIn('--network', (repository / 'scripts/ci/check_docs_examples.py').read_text())
        self.assertEqual([], examples.workflow_findings(repository))

    def test_gate_contract_rejects_comments_and_other_jobs(self):
        ci = (repository / '.github/workflows/ci.yml').read_text()
        lines = ci.splitlines()
        needs = next(line for line in lines if line.startswith('    needs: [workflows,'))
        condition = next(line for line in lines if line.startswith('      - if: contains(needs.*.result'))
        for line in (needs, condition, '        run: exit 1'):
            for replacement in ('    # ' + line.strip(), ''):
                self.assertEqual(1, ci.count(line))
                mutant = ci.replace(line, replacement, 1) + '\n  unrelated:\n' + line + '\n'
                with self.subTest(line=line, replacement=replacement), self.assertRaises(AssertionError):
                    self.assert_ci_contract(mutant)
        for old, new in (('site-build]', ']'),
                         ('windows-cli, ', ''),
                         ("needs.windows-cli.result != 'success'", 'false'),
                         ("needs.site-build.result != 'success'", 'false'),
                         ("needs.go.result != 'success'", 'false'),
                         ("needs.windows-supervisor.result != 'success'", 'false'),
                         (condition.strip()[6:], '(' + condition.strip()[6:] + ') && false'),
                         ('        run: exit 1', '        run: exit 1\n        continue-on-error: true')):
            self.assertEqual(1, ci.count(old))
            with self.subTest(old=old), self.assertRaises(AssertionError):
                self.assert_ci_contract(ci.replace(old, new, 1))

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

    def test_invalid_utf8_is_finding_and_next_command_executes(self):
        self.fixture(output='expected')
        rows_path = self.docs / '_data/examples.yml'
        rows = json.loads(rows_path.read_text())
        anchor = 'docs/guide.md#example[3]'
        command = 'printf next > executed'
        rows.append(dict(anchor=anchor, command=command, expect_exit=0))
        rows_path.write_text(json.dumps(rows))
        with (self.docs / 'guide.md').open('a') as source:
            source.write(f'<div class="example-block" data-example-anchor="{anchor}">\n\n```\n{command}\n```\n\n</div>\n')
        with (self.site / 'guide.html').open('a') as published:
            published.write(f'<div class="example-block" data-example-anchor="{anchor}"><pre>{command}\n</pre></div>')
        calls = []
        def execute(command):
            calls.append(command)
            if len(calls) == 1:
                return subprocess.CompletedProcess([], 0, b'\xff', b'')
            return subprocess.run(['bash', '-c', command], cwd=self.root, capture_output=True, check=True)
        argv = ['check_docs_examples.py', '--docs', str(self.docs), '--site', str(self.site),
                '--module-cache', str(self.root), '--image', 'golang:1.27.1@sha256:' + 'a' * 64]
        with patch.object(sys, 'argv', argv), patch.object(examples, 'docker_execute',
                side_effect=lambda command, *_: execute(command)):
            self.assertEqual(1, examples.main())
        self.assertEqual(2, len(calls))
        self.assertEqual('next', (self.root / 'executed').read_text())

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
