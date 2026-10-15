#!/usr/bin/env python3
"""Execute built-site examples in an offline, credential-free Docker sandbox.

Arguments: --docs DIR --site DIR --module-cache DIR --image DIGEST_PIN.
Exit 0: all examples agree; 1: findings; 2: invalid input/unavailable sandbox.
Side effects: ephemeral containers and stdout; source/cache mounts are read-only.
"""

import argparse
from datetime import datetime
import hashlib
import json
from pathlib import Path
import re
import subprocess
import sys
import uuid

from check_docs_site import Document, InputError, example_blocks, load_rows, normalise_output


def check(docs, site, execute):
    rows = load_rows(docs / '_data/examples.yml')
    blocks, errors = example_blocks(docs)
    findings = [f'{p}:{line}: examples: {detail}' for p, line, detail in errors]
    if not rows:
        findings.append('examples manifest is empty')
    published = {}
    for page in site.rglob('*.html'):
        for node in Document(page.read_text(encoding='utf-8')).root.all(cls='example-block'):
            published.setdefault(node.attrs.get('data-example-anchor'), []).append((page, node))
    seen = set()
    for row in rows:
        if (not {'anchor', 'command', 'expect_exit'} <= row.keys()
                or row.keys() - {'anchor', 'command', 'expect_exit', 'output_anchor'}
                or type(row['expect_exit']) is not int or not 0 <= row['expect_exit'] <= 255
                or not isinstance(row['command'], str) or not row['command'].strip()):
            raise InputError('invalid examples schema')
        for key in ('anchor', 'output_anchor'):
            if key not in row:
                continue
            anchor = row[key]
            if not isinstance(anchor, str):
                raise InputError('anchor must be text')
            block = blocks.get(anchor)
            matches = published.get(anchor, [])
            if anchor in seen or not block or block.annotation != anchor:
                findings.append(f'{anchor}: missing/duplicate source block')
            seen.add(anchor)
            if len(matches) != 1:
                findings.append(f'{anchor}: expected one published block')
                continue
            page, node = matches[0]
            expected_page = Path(anchor.split('#')[0]).relative_to('docs').with_suffix('.html')
            if page.relative_to(site) != expected_page:
                findings.append(f'{anchor}: published on wrong page')
            pres = node.all('pre')
            if not block or len(pres) != 1 or normalise_output(pres[0].text) != normalise_output(block.text + '\n'):
                findings.append(f'{anchor}: published block differs from source')
            if key == 'anchor' and block and row['command'] != block.text:
                findings.append(f'{anchor}: command differs from source')
    # Validate all identities before executing any command.
    if findings:
        return findings
    for row in rows:
        result = execute(row['command'])
        if result.returncode != row['expect_exit']:
            findings.append(f"{row['anchor']}: exit {result.returncode}, expected {row['expect_exit']}")
        if 'output_anchor' in row:
            # Markdown fences end in a newline; example_blocks stores content
            # without that structural final newline. Restore it before v1.
            expected = blocks[row['output_anchor']].text + '\n'
            if normalise_output(result.stdout.decode('utf-8')) != normalise_output(expected):
                findings.append(f"{row['output_anchor']}: output differs under docs-output-v1")
        print(f"example={row['anchor']} exit={result.returncode} expected={row['expect_exit']}")
    return findings


def site_digest(site):
    digest = hashlib.sha256()
    paths = sorted(site.rglob('*'))
    if not any(p.is_file() for p in paths):
        raise InputError('empty site artifact')
    for path in paths:
        if path.is_symlink():
            raise InputError('site artifact contains a symlink')
        if not path.is_file():
            continue
        content = path.read_bytes()
        if b'mythhelm-fixture' in content:
            raise InputError('site artifact contains a fixture marker')
        name = path.relative_to(site).as_posix().encode('utf-8')
        digest.update(len(name).to_bytes(8, 'big') + name)
        digest.update(hashlib.sha256(content).digest())
    return digest.hexdigest()


def workflow_findings(root):
    findings = []
    for name in ('ci', 'docs'):
        text = (root / '.github/workflows' / f'{name}.yml').read_text(encoding='utf-8')
        for forbidden in ('secrets' + '.', 'pull_request' + '_target'):
            if forbidden in text:
                findings.append(f'{name}.yml: forbidden credential/trigger reference')
    return findings


def docker_execute(command, root, cache, image):
    name = 'docs-example-' + uuid.uuid4().hex
    args = ['docker', 'create', '--name', name, '--pull=never', '--network', 'none',
            '--read-only', '--cap-drop', 'ALL', '--security-opt', 'no-new-privileges',
            '--pids-limit', '256', '--memory', '2g', '--cpus', '2',
            '--user', '65534:65534', '--tmpfs', '/tmp:rw,exec,mode=1777',
            '--mount', f'type=bind,src={root},dst=/src,readonly',
            '--mount', f'type=bind,src={cache},dst=/gomodcache,readonly',
            '--workdir', '/src', '--env', 'HOME=/tmp', '--env', 'TERM=dumb',
            '--env', 'GOPROXY=off', '--env', 'GOTOOLCHAIN=local',
            '--env', 'GOMODCACHE=/gomodcache', '--env', 'GOCACHE=/tmp/go-build',
            '--env', 'GOFLAGS=-buildvcs=false', image,
            'bash', '--noprofile', '--norc', '-c', command]
    created, container = False, name
    try:
        creation = subprocess.run(args, stdin=subprocess.DEVNULL, capture_output=True, timeout=30)
        if creation.returncode:
            raise InputError('sandbox creation failed; no example result')
        created = True
        candidate = creation.stdout.decode('ascii').strip()
        if not re.fullmatch(r'[a-f0-9]{64}', candidate):
            raise InputError('sandbox returned an invalid container ID')
        container = candidate
        result = subprocess.run(['docker', 'start', '--attach', container],
                                stdin=subprocess.DEVNULL, capture_output=True, timeout=180)
        inspection = subprocess.run(['docker', 'inspect', '--format', '{{json .State}}', container],
                                   stdin=subprocess.DEVNULL, capture_output=True, timeout=20)
        if inspection.returncode:
            raise InputError('sandbox state unavailable; no example result')
        try:
            state = json.loads(inspection.stdout)
        except (ValueError, UnicodeError) as exc:
            raise InputError('invalid sandbox state') from exc
        required = {'Status', 'Running', 'OOMKilled', 'Error', 'ExitCode', 'StartedAt'}
        if (not isinstance(state, dict) or not required <= state.keys()
                or state['Status'] != 'exited' or state['Running'] is not False
                or state['OOMKilled'] is not False or state['Error'] != ''
                or type(state['ExitCode']) is not int or not 0 <= state['ExitCode'] <= 255
                or not isinstance(state['StartedAt'], str) or not state['StartedAt']
                or state['StartedAt'].startswith('0001-')
                or result.returncode != state['ExitCode']):
            raise InputError('sandbox did not establish a completed command')
        try:
            started = datetime.fromisoformat(state['StartedAt'].replace('Z', '+00:00'))
            if started.tzinfo is None:
                raise ValueError('timestamp lacks timezone')
        except ValueError as exc:
            raise InputError('invalid sandbox start timestamp') from exc
        # 125/126/127 remain valid command exits only with this container-state
        # evidence, never from an indistinguishable Docker client failure.
        return result
    finally:
        if not created:
            # A client timeout/nonzero status does not prove the daemon never
            # created our uniquely named container. Reconcile only that name.
            listing = subprocess.run(['docker', 'ps', '--all', '--no-trunc',
                                      '--filter', f'name=^/{name}$', '--format', '{{.ID}}'],
                                     capture_output=True, timeout=20, check=False)
            if listing.returncode:
                raise InputError('sandbox creation cleanup unresolved: state unavailable')
            discovered = listing.stdout.decode('ascii').strip()
            if discovered:
                if not re.fullmatch(r'[a-f0-9]{64}', discovered):
                    raise InputError('sandbox creation cleanup unresolved: invalid identity')
                container, created = discovered, True
        if created:
            cleanup = subprocess.run(['docker', 'rm', '-f', container],
                                     capture_output=True, timeout=20, check=False)
            if cleanup.returncode:
                raise InputError('sandbox cleanup failed; executor may remain')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--docs', type=Path, default=Path('docs'))
    parser.add_argument('--site', type=Path, default=Path('_site'))
    parser.add_argument('--module-cache', type=Path)
    parser.add_argument('--image')
    parser.add_argument('--digest', action='store_true')
    parser.add_argument('--workflows', action='store_true')
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[2]
    try:
        if args.digest:
            print(site_digest(args.site))
            return 0
        if args.workflows:
            findings = workflow_findings(root)
        else:
            if not args.module_cache or not args.module_cache.is_dir() or not args.image or not re.fullmatch(r'golang:[0-9.]+@sha256:[a-f0-9]{64}', args.image):
                raise InputError('examples require a module cache and digest-pinned Go image')
            findings = check(args.docs, args.site, lambda command: docker_execute(
                command, root, args.module_cache.resolve(), args.image))
        for finding in findings:
            print(f'examples:1: check: {finding}', file=sys.stderr)
        return 1 if findings else 0
    except (OSError, ValueError, subprocess.SubprocessError) as exc:
        print(f'examples:1: input: {exc}', file=sys.stderr)
        return 2


if __name__ == '__main__':
    sys.exit(main())
