"""Synthetic fail-full selection and fail-closed aggregate checks."""
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest import mock

SCRIPT = Path(__file__).resolve().parents[1] / 'scripts/ci_change_scope.py'
spec = importlib.util.spec_from_file_location('ci_scope', SCRIPT)
s = importlib.util.module_from_spec(spec)
spec.loader.exec_module(s)


class SelectionTests(unittest.TestCase):
    def test_plans_pin_the_complete_platform_matrix(self):
        self.assertEqual(s.plan('full'), {
            'mode': 'full', 'go': 'true',
            'matrix': '["ubuntu-latest","ubuntu-24.04-arm","macos-latest","windows-latest","windows-11-arm"]'})
        self.assertEqual(s.plan('harness-only'), {
            'mode': 'harness-only', 'go': 'false', 'matrix': '["ubuntu-latest"]'})
        with self.assertRaises(ValueError):
            s.plan('unknown')

    def test_exact_harness_and_spec_whitelist(self):
        for path in ('specs/todo/demo/tasks.md', 'knowledge/invariants.md',
                     '.claude/agents/reviewer.md', '.claude/skills/spec/SKILL.md',
                     '.claude/rules/checks.md', '.claude/hooks/guard.sh',
                     '.agents/skills/bootstrap/SKILL.md', '.grok/skills/run/SKILL.md',
                     'scripts/harness/gate.sh', 'scripts/orchestration/delivery.py',
                     'scripts/pm/pm.py', 'AGENTS.md', 'CLAUDE.md', 'WORKFLOW.md'):
            with self.subTest(path=path):
                self.assertEqual(s.classify_paths([path]), 'harness-only')

    def test_docs_keep_the_real_linux_go_job(self):
        for path in ('README.md', 'CONTRIBUTING.md', 'SECURITY.md', 'MAINTAINERS.md',
                     'docs/limitations.md', 'docs/assets/brand/README.md',
                     'docs/decisions/0016-migration-import.md', 'docs/new note\nline.md'):
            with self.subTest(path=path):
                self.assertEqual(s.classify_paths([path]), 'docs-linux')
                self.assertEqual(json.loads(s.plan('docs-linux')['matrix']), ['ubuntu-latest'])
                self.assertEqual(s.plan('docs-linux')['go'], 'true')
        self.assertEqual(s.classify_paths(['specs/todo/demo/tasks.md', 'README.md']), 'docs-linux')

    def test_runtime_embed_build_config_and_unknown_paths_select_full(self):
        for path in ('internal/processsupport/SUPPORT.md', 'internal/journal/migrations/001.sql',
                     'mods/themes/dark.toml', 'adapters/fake/scenarios/demo.json',
                     'adapters/codex/testdata/qualification/routes.json', 'go.mod', 'go.sum',
                     'Makefile', '.golangci.yml', '.goreleaser.yml', '.claude/settings.json',
                     '.github/workflows/ci.yml', '.github/scripts/ci_change_scope.py',
                     'internal/new.go', 'scripts/harness/fixture.go', 'specs/runtime.go',
                     'scripts/harness/go.mod', 'knowledge/package.json', 'docs/Makefile',
                     'new-root.md', 'new-dir/note.md', 'scripts/new-tool.py',
                     'sdk/schema.json', 'hosts/example/config.toml', 'tests/testdata/input.txt'):
            with self.subTest(path=path):
                self.assertEqual(s.classify_paths([path]), 'full')
                self.assertEqual(s.classify_paths(['README.md', path]), 'full')
        self.assertEqual(s.classify_paths([]), 'full')

    def test_invalid_paths_cannot_enter_a_known_prefix(self):
        for path in ('/docs/note.md', 'docs/../internal/run.go', 'docs//note.md',
                     './docs/note.md', 'docs/./note.md', 'docs\\note.md', '', 'docs'):
            with self.subTest(path=path):
                self.assertEqual(s.classify_paths([path]), 'full')

    def test_nul_records_keep_both_rename_and_copy_paths(self):
        for status in (b'R100', b'R050', b'C100'):
            paths = s.parse_changes(status + b'\0internal/run.go\0docs/run.md\0')
            self.assertEqual(paths, ['internal/run.go', 'docs/run.md'])
            self.assertEqual(s.classify_paths(paths), 'full')
        self.assertEqual(s.classify_paths(s.parse_changes(b'D\0internal/run.go\0A\0docs/run.md\0')), 'full')
        self.assertEqual(s.parse_changes(b'M\0docs/name\nwith\ttabs.md\0'), ['docs/name\nwith\ttabs.md'])
        self.assertEqual(s.classify_paths(s.parse_changes(b'D\0docs/old.md\0')), 'docs-linux')

    def test_broken_unsupported_and_oversized_records_are_rejected(self):
        for raw in (b'', b'M\0README.md', b'M\0', b'R100\0docs/old.md\0',
                    b'R101\0a\0b\0', b'M\tREADME.md\0', b'X\0README.md\0',
                    b'T\0README.md\0', b'M\0README.md\0\0', b'M\0docs/\xff.md\0',
                    b'M\0' + b'x' * (s.MAX_DIFF_BYTES + 1) + b'\0'):
            with self.subTest(raw=raw[:50]), self.assertRaises(ValueError):
                s.parse_changes(raw)

    def test_non_pr_events_and_invalid_shas_never_narrow(self):
        for event in ('push', 'merge_group', 'workflow_dispatch', '', 'unknown'):
            with mock.patch.object(s, 'git_changes', side_effect=AssertionError('must not inspect diff')):
                self.assertEqual(s.select_mode(event, 'a' * 40, 'b' * 40, Path('.')), 'full')
        for base, head in (('', ''), ('main', 'HEAD'), ('a' * 39, 'b' * 40), ('--help', 'b' * 40)):
            self.assertEqual(s.select_mode('pull_request', base, head, Path('.')), 'full')
        for failure in (ValueError('bad diff'), OSError('missing git'), subprocess.TimeoutExpired('git', 1)):
            with mock.patch.object(s, 'git_changes', side_effect=failure):
                self.assertEqual(s.select_mode('pull_request', 'a' * 40, 'b' * 40, Path('.')), 'full')

    def test_ambiguous_merge_bases_and_noncommit_objects_cannot_narrow(self):
        base, head = 'a' * 40, 'b' * 40
        commit = subprocess.CompletedProcess([], 0, stdout=b'commit\n')
        for merge_output in (b'', b'c' * 40 + b'\n' + b'd' * 40 + b'\n', b'not-a-sha\n'):
            with mock.patch.object(s.subprocess, 'run', side_effect=[
                    commit, commit, subprocess.CompletedProcess([], 0, stdout=merge_output)]) as run:
                self.assertEqual(s.select_mode('pull_request', base, head, Path('.')), 'full')
                self.assertEqual(run.call_count, 3)
        with mock.patch.object(s.subprocess, 'run', return_value=subprocess.CompletedProcess([], 0, stdout=b'blob\n')) as run:
            self.assertEqual(s.select_mode('pull_request', base, head, Path('.')), 'full')
            self.assertEqual(run.call_count, 1)

    def test_advanced_main_uses_merge_base_instead_of_reversing_unrelated_changes(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            env = dict(os.environ, GIT_CONFIG_NOSYSTEM='1', GIT_CONFIG_GLOBAL=os.devnull,
                       GIT_AUTHOR_NAME='Synthetic', GIT_AUTHOR_EMAIL='test@example.com',
                       GIT_COMMITTER_NAME='Synthetic', GIT_COMMITTER_EMAIL='test@example.com')
            for key in ('GIT_DIR', 'GIT_WORK_TREE', 'GIT_INDEX_FILE'):
                env.pop(key, None)
            def git(*args):
                return subprocess.check_output(['git', '-C', tmp, *args], env=env, text=True).strip()
            git('init', '-q', '-b', 'main')
            (root / 'runtime.go').write_text('package synthetic\n')
            git('add', 'runtime.go'); git('commit', '-qm', 'synthetic base')
            git('switch', '-qc', 'contributor')
            (root / 'specs').mkdir()
            (root / 'specs/plan.md').write_text('synthetic spec\n')
            git('add', 'specs/plan.md'); git('commit', '-qm', 'synthetic spec')
            head = git('rev-parse', 'HEAD')
            git('switch', '-q', 'main')
            (root / 'runtime.go').write_text('package synthetic\n// unrelated main advancement\n')
            git('add', 'runtime.go'); git('commit', '-qm', 'synthetic main advancement')
            base = git('rev-parse', 'HEAD')
            with mock.patch.dict(os.environ, env, clear=True):
                self.assertEqual(s.select_mode('pull_request', base, head, root), 'harness-only')
                git('switch', '--orphan', 'unrelated')
                git('rm', '-rf', '--ignore-unmatch', '.')
                (root / 'README.md').write_text('synthetic unrelated history\n')
                git('add', 'README.md'); git('commit', '-qm', 'synthetic unrelated root')
                self.assertEqual(s.select_mode('pull_request', git('rev-parse', 'HEAD'), head, root), 'full')

    def test_real_git_immutable_diff_renames_deletions_and_missing_objects(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            env = dict(os.environ, GIT_CONFIG_NOSYSTEM='1', GIT_CONFIG_GLOBAL=os.devnull,
                       GIT_AUTHOR_NAME='Synthetic', GIT_AUTHOR_EMAIL='test@example.com',
                       GIT_COMMITTER_NAME='Synthetic', GIT_COMMITTER_EMAIL='test@example.com')
            for key in ('GIT_DIR', 'GIT_WORK_TREE', 'GIT_INDEX_FILE'):
                env.pop(key, None)
            def git(*args):
                return subprocess.check_output(['git', '-C', tmp, *args], env=env, text=True).strip()
            git('init', '-q', '-b', 'main')
            (root / 'internal').mkdir()
            (root / 'internal/run.go').write_text('package synthetic\n')
            git('add', 'internal/run.go'); git('commit', '-qm', 'synthetic base')
            base = git('rev-parse', 'HEAD')
            (root / 'docs').mkdir()
            git('mv', 'internal/run.go', 'docs/moved.md')
            git('commit', '-qm', 'synthetic rename')
            head = git('rev-parse', 'HEAD')
            with mock.patch.dict(os.environ, env, clear=True):
                self.assertEqual(s.select_mode('pull_request', base, head, root), 'full')
                self.assertEqual(s.select_mode('pull_request', head, head, root), 'full')
                self.assertEqual(s.select_mode('pull_request', 'f' * 40, head, root), 'full')
                git('rm', 'docs/moved.md'); git('commit', '-qm', 'synthetic deletion')
                self.assertEqual(s.select_mode('pull_request', head, git('rev-parse', 'HEAD'), root), 'docs-linux')
                (root / 'README.md').write_text('synthetic docs\n')
                git('add', 'README.md'); git('commit', '-qm', 'synthetic docs')
                newest = git('rev-parse', 'HEAD')
                # Mutable branch/worktree state cannot alter the supplied pair.
                self.assertEqual(s.select_mode('pull_request', base, head, root), 'full')
                self.assertEqual(s.select_mode('pull_request', head, newest, root), 'docs-linux')


class AggregateTests(unittest.TestCase):
    def needs(self, mode):
        result = {job: {'result': 'success', 'outputs': {}} for job in s.ALL_JOBS}
        result['detect']['outputs'] = s.plan(mode)
        for job in s.RUNTIME_JOBS:
            result[job]['result'] = 'success' if mode == 'full' or (mode == 'docs-linux' and job == 'go') else 'skipped'
        return result

    def test_each_canonical_mode_is_accepted(self):
        for mode in s.MODES:
            self.assertTrue(s.check_needs(self.needs(mode)))

    def test_every_failure_cancellation_and_unexpected_skip_is_rejected(self):
        for mode in s.MODES:
            for job in s.ALL_JOBS:
                for status in ('failure', 'cancelled', 'pending', None):
                    needs = self.needs(mode); needs[job]['result'] = status
                    with self.subTest(mode=mode, job=job, status=status):
                        self.assertFalse(s.check_needs(needs))
                if self.needs(mode)[job]['result'] == 'success':
                    needs = self.needs(mode); needs[job]['result'] = 'skipped'
                    self.assertFalse(s.check_needs(needs))

    def test_missing_jobs_bad_mode_and_mismatched_outputs_reject(self):
        for mode in s.MODES:
            for job in s.ALL_JOBS:
                needs = self.needs(mode); del needs[job]
                self.assertFalse(s.check_needs(needs))
            for key, value in (('mode', 'unknown'), ('go', 'false'), ('go', 'true'), ('matrix', '[]'),
                               ('matrix', '["ubuntu-latest"]'), ('matrix', 'invalid')):
                needs = self.needs(mode); needs['detect']['outputs'][key] = value
                if needs['detect']['outputs'] != s.plan(mode):
                    self.assertFalse(s.check_needs(needs))
        for value in (None, [], {}, {'go': None}):
            self.assertFalse(s.check_needs(value))
        needs = self.needs('full'); needs['new-job'] = {'result': 'failure'}
        self.assertFalse(s.check_needs(needs))
        needs = self.needs('harness-only'); needs['go']['result'] = 'success'
        self.assertFalse(s.check_needs(needs))

    def test_cli_fallback_and_aggregate_failure(self):
        result = subprocess.run(['python3', str(SCRIPT), 'classify', '--event', 'pull_request', '--base', 'invalid', '--head', 'invalid'], capture_output=True, text=True)
        self.assertEqual(result.returncode, 0)
        self.assertEqual(dict(line.split('=', 1) for line in result.stdout.splitlines()), s.plan('full'))
        for raw, code in ((json.dumps(self.needs('docs-linux')), 0), ('{}', 1), ('broken', 1)):
            result = subprocess.run(['python3', str(SCRIPT), 'check'], env=dict(os.environ, CI_NEEDS=raw), capture_output=True, text=True)
            self.assertEqual(result.returncode, code, result.stdout + result.stderr)


class WorkflowTests(unittest.TestCase):
    def setUp(self):
        self.workflow = (SCRIPT.parents[1] / 'workflows/ci.yml').read_text()

    def job(self, name):
        import re
        rest = self.workflow.split('  ' + name + ':\n', 1)[1]
        return re.split(r'^  [a-z][a-z0-9-]*:\n', rest, maxsplit=1, flags=re.MULTILINE)[0]

    def test_immutable_event_inputs_and_all_classifier_tests_are_wired(self):
        detect = self.job('detect')
        for text in ('fetch-depth: 0', 'persist-credentials: false',
                     'CI_EVENT: ${{ github.event_name }}',
                     'PR_BASE_SHA: ${{ github.event.pull_request.base.sha }}',
                     'PR_HEAD_SHA: ${{ github.event.pull_request.head.sha }}',
                     '--event "$CI_EVENT" --base "$PR_BASE_SHA" --head "$PR_HEAD_SHA"',
                     'python3 .github/scripts/ci_change_scope.py classify'):
            self.assertIn(text, detect)
        self.assertIn("python3 -m unittest discover -s .github/tests -p 'test_*.py'", self.job('harness'))

    def test_docs_mode_keeps_the_full_linux_go_job_and_unchanged_names(self):
        go = self.job('go')
        for text in ("    if: needs.detect.outputs.go == 'true'",
                     'name: Go (${{ matrix.os }})',
                     'os: ${{ fromJSON(needs.detect.outputs.matrix) }}',
                     "      - if: runner.os != 'Windows'\n        run: go test -race ./...",
                     '- run: go vet ./...', '- run: go mod tidy -diff'):
            self.assertIn(text, go)
        for job in ('windows-supervisor', 'windows-cli', 'lint', 'govulncheck', 'licenses'):
            self.assertIn("    if: needs.detect.outputs.mode == 'full'", self.job(job))
        for job in ('harness', 'site-build', 'workflows'):
            self.assertNotIn('    if:', self.job(job))
        self.assertIn('uses: ./.github/workflows/docs.yml', self.job('site-build'))

    def test_aggregate_receives_every_job_even_after_failures_and_skips(self):
        job = self.job('ci-ok')
        self.assertIn('    if: always()', job)
        needs = next(line.strip()[8:-1] for line in job.splitlines() if line.strip().startswith('needs: ['))
        self.assertEqual(set(part.strip() for part in needs.split(',')), set(s.ALL_JOBS))
        self.assertIn('CI_NEEDS: ${{ toJSON(needs) }}', job)
        self.assertIn('run: python3 .github/scripts/ci_change_scope.py check', job)


if __name__ == '__main__':
    unittest.main()
