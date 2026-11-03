#!/usr/bin/env python3
"""Conservative CI selection and verification, using only the standard library.

classify emits fixed key=value workflow outputs. Only immutable pull-request
commit pairs can narrow jobs; unknown paths/events, missing objects and invalid
or empty diffs select full coverage. It reads Git state and writes no repository
files. check validates the complete CI needs context from CI_NEEDS, exiting 0
only when the exact selected jobs succeeded and unselected jobs were skipped.
Classification fallback exits 0 with full coverage; check rejects invalid data
with exit 1. Argument errors exit 2. Changes to this helper itself select full.
"""
import argparse
import json
import os
from pathlib import Path
import re
import subprocess
import sys

MODES = ('full', 'docs-linux', 'harness-only')
OS_MATRIX = ['ubuntu-latest', 'ubuntu-24.04-arm', 'macos-latest', 'windows-latest', 'windows-11-arm']
RUNTIME_JOBS = ('go', 'windows-supervisor', 'windows-cli', 'lint', 'govulncheck', 'licenses')
ALWAYS_JOBS = ('detect', 'workflows', 'harness', 'site-build')
ALL_JOBS = ALWAYS_JOBS + RUNTIME_JOBS
MAX_DIFF_BYTES = 4 * 1024 * 1024
SHA = re.compile(r'(?:[0-9a-f]{40}|[0-9a-f]{64})\Z')
HARNESS_PREFIXES = ('specs/', 'knowledge/', '.claude/agents/', '.claude/skills/',
                    '.claude/rules/', '.claude/hooks/', '.agents/skills/', '.grok/skills/',
                    'scripts/harness/', 'scripts/orchestration/', 'scripts/pm/')
HARNESS_FILES = {'AGENTS.md', 'CLAUDE.md', 'WORKFLOW.md'}
DOC_FILES = {'README.md', 'CONTRIBUTING.md', 'SECURITY.md', 'MAINTAINERS.md'}
BUILD_FILES = {'go.mod', 'go.sum', 'go.work', 'go.work.sum', 'Makefile', 'Dockerfile',
               'Containerfile', 'package.json', 'package-lock.json', 'npm-shrinkwrap.json',
               'pyproject.toml', 'uv.lock', 'requirements.txt', 'Cargo.toml', 'Cargo.lock',
               'Gemfile', 'Gemfile.lock', '.goreleaser.yml', '.golangci.yml'}


def valid_path(path):
    return (isinstance(path, str) and bool(path) and '\\' not in path
            and all(part not in ('', '.', '..') for part in path.split('/')))


def classify_paths(paths):
    if not paths:
        return 'full'
    docs = False
    for path in paths:
        if not valid_path(path) or path.endswith('.go') or path.rsplit('/', 1)[-1] in BUILD_FILES:
            return 'full'
        if path.startswith('docs/') or path in DOC_FILES:
            docs = True
        elif not (path.startswith(HARNESS_PREFIXES) or path in HARNESS_FILES):
            return 'full'
    return 'docs-linux' if docs else 'harness-only'


def parse_changes(raw):
    if not raw or len(raw) > MAX_DIFF_BYTES or not raw.endswith(b'\0'):
        raise ValueError('empty, oversized or unterminated diff')
    fields = raw[:-1].split(b'\0')
    paths, index = [], 0
    while index < len(fields):
        status = fields[index].decode('ascii')
        index += 1
        if status in ('A', 'M', 'D'):
            count = 1
        elif re.fullmatch(r'[RC][0-9]{1,3}', status) and int(status[1:]) <= 100:
            count = 2
        else:
            raise ValueError('unknown status or file type change')
        if index + count > len(fields):
            raise ValueError('truncated change record')
        for field in fields[index:index + count]:
            path = field.decode('utf-8')
            if not valid_path(path):
                raise ValueError('malformed path')
            paths.append(path)
        index += count
    return paths


def git_changes(base, head, root):
    for commit in (base, head):
        kind = subprocess.run(['git', 'cat-file', '-t', commit], cwd=root,
                              capture_output=True, check=True, timeout=30).stdout
        if kind.strip() != b'commit':
            raise ValueError('input is not a commit')
    common = subprocess.run(['git', 'merge-base', '--all', base, head], cwd=root,
                            capture_output=True, check=True, timeout=30).stdout.decode('ascii').splitlines()
    if len(common) != 1 or not SHA.fullmatch(common[0]):
        raise ValueError('missing or ambiguous merge base')
    result = subprocess.run(['git', 'diff', '--no-ext-diff', '--no-textconv',
                             '--name-status', '-z', '--find-renames', '-l1000', common[0], head, '--'],
                            cwd=root, capture_output=True, check=True, timeout=30)
    return result.stdout


def select_mode(event, base, head, root):
    if (event != 'pull_request' or not isinstance(base, str) or not isinstance(head, str)
            or not SHA.fullmatch(base) or not SHA.fullmatch(head)):
        return 'full'
    try:
        return classify_paths(parse_changes(git_changes(base, head, root)))
    except (ValueError, OSError, subprocess.SubprocessError):
        return 'full'


def plan(mode):
    if mode not in MODES:
        raise ValueError('unknown mode')
    return {'mode': mode, 'go': 'false' if mode == 'harness-only' else 'true',
            'matrix': json.dumps(OS_MATRIX if mode == 'full' else ['ubuntu-latest'], separators=(',', ':'))}


def check_needs(needs):
    if not isinstance(needs, dict) or set(needs) != set(ALL_JOBS):
        return False
    if any(not isinstance(job, dict) for job in needs.values()):
        return False
    outputs = needs['detect'].get('outputs')
    if not isinstance(outputs, dict) or outputs.get('mode') not in MODES:
        return False
    mode = outputs['mode']
    if outputs != plan(mode):
        return False
    for job in ALL_JOBS:
        selected = job in ALWAYS_JOBS or mode == 'full' or (mode == 'docs-linux' and job == 'go')
        if needs[job].get('result') != ('success' if selected else 'skipped'):
            return False
    return True


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest='command', required=True)
    classify = commands.add_parser('classify')
    classify.add_argument('--event', required=True)
    classify.add_argument('--base', default='')
    classify.add_argument('--head', default='')
    commands.add_parser('check')
    args = parser.parse_args()
    if args.command == 'classify':
        mode = select_mode(args.event, args.base, args.head, Path.cwd())
        for key, value in plan(mode).items():
            print(key + '=' + value)
        return 0
    try:
        needs = json.loads(os.environ.get('CI_NEEDS', ''))
    except (ValueError, TypeError):
        needs = None
    if not check_needs(needs):
        print('CI job selection or required results are missing, inconsistent or unsuccessful.', file=sys.stderr)
        return 1
    return 0


if __name__ == '__main__':
    sys.exit(main())
