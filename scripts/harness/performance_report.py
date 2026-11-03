#!/usr/bin/env python3
"""Render reviewed native-pair aggregates into README and a deterministic SVG.

aggregate reads the existing frozen protocol/private ledger and writes only an
allowlisted aggregate after explicit reviewer attestation. render reads that
aggregate (or --unavailable), updating the marked README block and chart only.
Exit 1 refuses malformed evidence. No agents, publication or inference launched.
Review hashes bind private review evidence; they do not certify its truth.
"""
import argparse
from collections import Counter
from html import escape
import hashlib
import json
import math
from pathlib import Path
import sys
import subprocess

import dogfood_measurement as ledger
import dogfood_statistics as statistics

START = '<!-- performance-report:start -->'
END = '<!-- performance-report:end -->'
CHART = 'docs/assets/performance.svg'
AGGREGATE = 'docs/dogfood/performance.json'
ARMS = ('mythhelm', 'direct-native')
OUTCOMES = (*ledger.OUTCOMES, 'unfinished', 'unstarted')
PARITY_BLOCKS = {'unknown_or_mismatched_parity', 'arm_unavailable_or_misclassified',
                 'randomized_order_not_followed', 'retry_budget_unproven_or_exceeded'}
ROOT = Path(__file__).resolve().parents[2]


def summarize(values):
    known = [v for v in values if v is not None]
    return {'total': sum(known) if known else None, 'known': len(known), 'missing': len(values) - len(known)}


def aggregate(protocol, state, review_hash, unconfounded=False, receipts=None):
    """Review attests actual parity, equal acceptance scheduling and budget scope."""
    ledger.digest(review_hash)
    if not unconfounded:
        ledger.fail('independent comparison review required')
    snapshot = None
    if Path(state).exists():
        with ledger.private_lock(state) as directory, ledger.ledger_file(directory) as handle:
            snapshot = hashlib.sha256(handle.read(ledger.MAX_LEDGER_BYTES + 1)).digest()
    analysis = statistics.analyze(protocol, state)
    plan, _ = statistics.read_json(protocol)
    if PARITY_BLOCKS.intersection(analysis['limitations']) or any(v is None for v in plan['native'].values()):
        ledger.fail('current matched evidence requires reviewed known parity')
    starts, finishes = {}, {}
    if Path(state).exists():
        with ledger.private_lock(state) as directory, ledger.ledger_file(directory) as handle:
            if hashlib.sha256(handle.read(ledger.MAX_LEDGER_BYTES + 1)).digest() != snapshot:
                ledger.fail('ledger changed during export; review a stable snapshot')
            handle.seek(0)
            starts, finishes = ledger.fold(handle)
    arms = {}
    for mode, id_field in zip(ARMS, ('mythhelm_id', 'native_id')):
        ids = [p[id_field] for p in plan['assignments']]
        results = [finishes.get(i) for i in ids]
        counts = Counter(finishes[i]['outcome'] if i in finishes else
                         ('unfinished' if i in starts else 'unstarted') for i in ids)
        arms[mode] = {
            'accepted_within_budget': sum(r is not None and r['outcome'] == 'accepted' and r['wall_seconds'] is not None and r['wall_seconds'] <= plan['cap_seconds'] for r in results),
            'assigned': len(ids), 'outcomes': {k: counts[k] for k in OUTCOMES},
            'wall_seconds': summarize([r['wall_seconds'] if r else None for r in results]),
            'usage_sources': {s: sum((r['usage_source'] if r else 'unknown') == s for r in results)
                              for s in ('reported', 'observed', 'unknown')},
            'tokens': {k: summarize([r[k] if r else None for r in results]) for k in ledger.TOKENS}}
    fields(receipts, ('pair_receipts', 'pr_evidence'))
    receipt_by_id = {}
    for receipt in receipts['pair_receipts']:
        fields(receipt, ('attempt_id', 'receipt_sha256', 'tool_source_revision'))
        if receipt['attempt_id'] in receipt_by_id:
            ledger.fail('duplicate native receipt')
        receipt_by_id[receipt['attempt_id']] = receipt
    expected_ids = {p[k] for p in plan['assignments'] for k in ('mythhelm_id', 'native_id') if p[k] in starts}
    if set(receipt_by_id) != expected_ids:
        ledger.fail('every started assignment needs its own reviewed native receipt')
    pair_receipts = []
    for pair in plan['assignments']:
        for mode, key in zip(ARMS, ('mythhelm_id', 'native_id')):
            if pair[key] in receipt_by_id:
                receipt = receipt_by_id[pair[key]]
                pair_receipts.append({'pair_id': pair['pair_id'], 'mode': mode,
                    'workload_source_revision': pair['source_revision'],
                    'tool_source_revision': receipt['tool_source_revision'],
                    'receipt_sha256': receipt['receipt_sha256']})
    result = {'schema_version': 1,
              'pair_receipts': pair_receipts, 'pr_evidence': receipts['pr_evidence'], 'review_evidence_sha256': review_hash,
              'comparison_unconfounded': True, 'protocol_sha256': analysis['protocol_sha256'],
              'native': plan['native'], 'phase': plan['phase'], 'cohort': plan['cohort'],
              'registered_at': plan['registered_at'],
              'cap_seconds': plan['cap_seconds'], 'planned_pairs': analysis['planned_pairs'],
              'independent_components': analysis['independent_components'],
              'bindings': sorted({(p['source_revision'], p['contract_sha256'], p['checks_sha256'])
                                  for p in plan['assignments']}),
              'limitations': analysis['limitations'], 'arms': arms}
    validate_aggregate(result)
    return result


def count(value):
    if type(value) is not int or value < 0:
        ledger.fail('invalid count')


def fields(value, keys):
    if not isinstance(value, dict) or set(value) != set(keys):
        ledger.fail('unexpected aggregate fields')


def validate_aggregate(data):
    fields(data, ('schema_version', 'review_evidence_sha256', 'comparison_unconfounded',
                  'protocol_sha256', 'native', 'phase', 'cohort', 'registered_at', 'cap_seconds', 'planned_pairs',
                  'independent_components', 'bindings', 'limitations', 'arms', 'pair_receipts', 'pr_evidence'))
    if data['schema_version'] != 1 or data['comparison_unconfounded'] is not True:
        ledger.fail('unreviewed aggregate')
    for key in ('review_evidence_sha256', 'protocol_sha256'):
        ledger.digest(data[key])
    ledger.timestamp(data['registered_at'])
    fields(data['native'], statistics.NATIVE_FIELDS)
    for key, value in data['native'].items():
        (ledger.digest if key.endswith('sha256') else ledger.text)(value)
    if data['phase'] not in ('pilot', 'confirmatory') or data['cohort'] not in ('development', 'holdout'):
        ledger.fail('invalid study scope')
    if data['phase'] == 'confirmatory' and data['cohort'] != 'holdout':
        ledger.fail('invalid confirmatory cohort')
    for key in ('planned_pairs', 'independent_components'):
        count(data[key])
        if data[key] < 1:
            ledger.fail('empty aggregate')
    if data['independent_components'] > data['planned_pairs']:
        ledger.fail('invalid sample count')
    if type(data['cap_seconds']) not in (int, float) or not math.isfinite(data['cap_seconds']) or not 0 < data['cap_seconds'] <= 86400:
        ledger.fail('invalid time budget')
    if not isinstance(data['bindings'], list) or not data['bindings']:
        ledger.fail('missing source bindings')
    for binding in data['bindings']:
        if not isinstance(binding, (list, tuple)) or len(binding) != 3:
            ledger.fail('invalid source binding')
        for index, value in enumerate(binding):
            ledger.digest(value, revision=index == 0)
    if not isinstance(data['limitations'], list):
        ledger.fail('invalid limitations')
    for limitation in data['limitations']:
        ledger.text(limitation)
    if PARITY_BLOCKS.intersection(data['limitations']):
        ledger.fail('unmatched comparisons are not current matched evidence')
    pr = data['pr_evidence']
    fields(pr, ('candidate_source_revision', 'delivery_scope', 'authored_via', 'receipt_sha256', 'tool_source_revision'))
    for key in ('candidate_source_revision', 'tool_source_revision'):
        ledger.digest(pr[key], revision=True)
    ledger.digest(pr['receipt_sha256'])
    if pr['delivery_scope'] not in ('validation-use', 'authored') or pr['authored_via'] not in ARMS:
        ledger.fail('invalid PR evidence scope')
    if pr['delivery_scope'] == 'authored' and pr['authored_via'] != 'mythhelm':
        ledger.fail('authorship requires actual MYTHHELM authorship evidence')
    if not isinstance(data['pair_receipts'], list):
        ledger.fail('invalid native receipts')
    seen = set()
    for receipt in data['pair_receipts']:
        fields(receipt, ('pair_id', 'mode', 'workload_source_revision', 'tool_source_revision', 'receipt_sha256'))
        ledger.text(receipt['pair_id'])
        if receipt['mode'] not in ARMS or (receipt['pair_id'], receipt['mode']) in seen:
            ledger.fail('invalid or duplicate pair receipt')
        seen.add((receipt['pair_id'], receipt['mode']))
        for key in ('workload_source_revision', 'tool_source_revision'):
            ledger.digest(receipt[key], revision=True)
        ledger.digest(receipt['receipt_sha256'])
        if receipt['workload_source_revision'] not in {b[0] for b in data['bindings']}:
            ledger.fail('receipt workload must match registered source')
    fields(data['arms'], ARMS)
    for arm in data['arms'].values():
        fields(arm, ('accepted_within_budget', 'assigned', 'outcomes', 'wall_seconds', 'usage_sources', 'tokens'))
        if arm['assigned'] != data['planned_pairs']:
            ledger.fail('all assignments must remain visible')
        count(arm['accepted_within_budget'])
        if arm['accepted_within_budget'] > arm['outcomes']['accepted']:
            ledger.fail('budgeted acceptance exceeds accepted outcomes')
        fields(arm['outcomes'], OUTCOMES)
        fields(arm['usage_sources'], ('reported', 'observed', 'unknown'))
        for counters in (arm['outcomes'], arm['usage_sources']):
            for value in counters.values():
                count(value)
            if sum(counters.values()) != arm['assigned']:
                ledger.fail('inconsistent assignment count')
        fields(arm['tokens'], ledger.TOKENS)
        for summary in (arm['wall_seconds'], *arm['tokens'].values()):
            fields(summary, ('total', 'known', 'missing'))
            count(summary['known']); count(summary['missing'])
            total = summary['total']
            if summary['known'] + summary['missing'] != arm['assigned']:
                ledger.fail('inconsistent missingness')
            if (total is None) != (summary['known'] == 0):
                ledger.fail('missing is not zero')
            if total is not None and (type(total) not in (int, float) or not math.isfinite(total) or total < 0):
                ledger.fail('invalid observation')


def observed(summary):
    mean = 'unavailable' if summary['total'] is None else f"{summary['total'] / summary['known']:.2f} s"
    return f"{mean} ({summary['known']} observed; {summary['missing']} missing)"


def render(data=None):
    if data is not None:
        validate_aggregate(data)
    lines = [START, '## Native delivery performance', '']
    chart_text = []
    bars = []
    if data is None:
        lines += ['**No current validated matched pairs.** No comparative advantage is established.', '',
                  '| Route | Accepted within budget / assigned | Observed mean elapsed | Usage provenance / missingness | Paired trials |',
                  '|---|---|---|---|---|',
                  '| MYTHHELM | Unavailable | Unavailable | Unavailable | 0 |',
                  '| Direct native | Unavailable | Unavailable | Unavailable | 0 |']
        chart_text = [('No current validated matched pairs', 60), ('Observed delivery time unavailable for both routes', 100)]
    else:
        lines += [f"**{data['phase'].capitalize()} / {data['cohort']}: {data['planned_pairs']} assigned pairs; "
                  f"{data['independent_components']} independent component(s).** No comparative advantage is established.", '',
                  '| Route | Accepted within budget / assigned | Observed mean elapsed | Usage provenance / missingness | Paired trials |',
                  '|---|---|---|---|---|']
        chart_text.append((f"{data['phase'].capitalize()}: {data['planned_pairs']} assigned pairs; {data['independent_components']} independent component(s)", 55))
        means = [a['wall_seconds']['total'] / a['wall_seconds']['known']
                 for a in data['arms'].values() if a['wall_seconds']['known']]
        maximum = max(means, default=1) or 1
        for index, (mode, label) in enumerate(zip(ARMS, ('MYTHHELM', 'Direct native'))):
            arm = data['arms'][mode]
            usage = '; '.join(f"{arm['usage_sources'][k]} {k}" for k in ('reported', 'observed', 'unknown'))
            lines.append(f"| {label} | {arm['accepted_within_budget']} / {arm['assigned']} | {observed(arm['wall_seconds'])} | {usage} | {data['planned_pairs']} |")
            summary = arm['wall_seconds']
            y = 80 + index * 85
            chart_text.append((f'{label}: {observed(summary)}', y))
            if summary['known']:
                bars.append((y + 12, 480 * summary['total'] / summary['known'] / maximum))
        lines += ['', f'![Observed native delivery time; unavailable values have no bars]({CHART})', '',
                  '<details>', '<summary>Outcomes, usage, provenance and limitations</summary>', '',
                  'All assigned outcomes (including failures and incomplete work):', '',
                  '| Route | ' + ' | '.join(OUTCOMES) + ' |',
                  '|---|' + '---|' * len(OUTCOMES)]
        for mode in ARMS:
            lines.append('| ' + mode + ' | ' + ' | '.join(str(data['arms'][mode]['outcomes'][o]) for o in OUTCOMES) + ' |')
        lines += ['', 'Token totals remain separate; each cell shows total / missing assignments.', '',
                  '| Route | ' + ' | '.join(ledger.TOKENS) + ' |', '|---|' + '---|' * len(ledger.TOKENS)]
        for mode in ARMS:
            cells = []
            for token in ledger.TOKENS:
                summary = data['arms'][mode]['tokens'][token]
                cells.append(f"{summary['total'] if summary['total'] is not None else 'unavailable'} / {summary['missing']}")
            lines.append('| ' + mode + ' | ' + ' | '.join(cells) + ' |')
        pr = data['pr_evidence']
        lines += ['', f"This PR: **{pr['delivery_scope']}** through MYTHHELM; authored via **{pr['authored_via']}**. "
                  f"Candidate source: `{pr['candidate_source_revision']}`; MYTHHELM tool source: `{pr['tool_source_revision']}`; "
                  f"reviewed native-use receipt: `{pr['receipt_sha256']}`.",
                  'The paired performance workload is validation evidence; it does not establish authorship of this PR. Different workload sources are reference measurements.', '',
                  '| Pair / route | Workload source | Tool source | Reviewed native receipt SHA-256 |',
                  '|---|---|---|---|']
        for receipt in data['pair_receipts']:
            lines.append('| ' + receipt['pair_id'] + ' / ' + receipt['mode'] + ' | ' +
                         ' | '.join('`' + receipt[k] + '`' for k in ('workload_source_revision', 'tool_source_revision', 'receipt_sha256')) + ' |')
        lines += ['', f"Registered time ceiling: {data['cap_seconds']} s per arm; one attempt per arm. "
                  'Observed elapsed means include all outcomes with known time; missing time is excluded and counted, never zero. '
                  'These are descriptive observations, not restricted-time scores or successful-run latency.', '',
                  'Native parity: ' + ', '.join(f'{k} `{data["native"][k]}`' for k in ('harness', 'native_version', 'model', 'effort')) + '.',
                  f"Registered: {data['registered_at']}. Protocol: `{data['protocol_sha256']}`. Independent review: `{data['review_evidence_sha256']}`.",
                  'Source / task contract / checks SHA bindings: ' + '; '.join(' / '.join(f'`{v}`' for v in b) for b in data['bindings']) + '.',
                  'Configuration / environment SHA-256: ' + ' / '.join(f'`{data["native"][k]}`' for k in ('configuration_sha256', 'environment_sha256')) + '.',
                  'Limitations: ' + (', '.join(data['limitations']) or 'none recorded') + '. '
                  'A small pilot cannot establish population performance; independent components, delayed defects and uncertainty limit interpretation.', '', '</details>']
    if data is None:
        lines += ['', f'![Observed native delivery time; unavailable values have no bars]({CHART})']
    lines += ['', 'Same native harness, model, effort, source, task, checks, settings, environment and registered budget are required for current matched evidence. '
              'Historical [scheduling-confounded pilot](docs/dogfood/0003-measured-self-hosting.md) results are excluded. '
              'See the [measurement method](docs/harness/measured-delivery.md#controlled-comparisons).', END]
    svg = ['<svg xmlns="http://www.w3.org/2000/svg" width="800" height="270" viewBox="0 0 800 270" role="img" aria-labelledby="title desc">',
           '<title id="title">Observed native delivery time</title>',
           '<desc id="desc">All assigned outcomes; missing observations have no bars. Descriptive evidence only, no superiority claim.</desc>',
           '<rect width="800" height="270" fill="#f7f9fc"/>',
           '<g fill="#152238" font-family="sans-serif" font-size="17">',
           '<text x="24" y="30">Observed elapsed time — descriptive evidence only</text>']
    svg += [f'<text x="24" y="{y}">{escape(text)}</text>' for text, y in chart_text]
    svg += ['<text x="24" y="250" font-size="13">No established advantage. Missing is not zero; pilot results do not generalize.</text>', '</g>']
    svg += [f'<rect x="24" y="{y}" width="{width:.2f}" height="20" fill="#425f9c"/>' for y, width in bars]
    if data is not None:
        aggregate_hash = hashlib.sha256(json.dumps(data, sort_keys=True, separators=(',', ':'), allow_nan=False).encode()).hexdigest()
        svg += [f'<metadata>aggregate-sha256:{aggregate_hash}</metadata>']
    svg += ['</svg>']
    return '\n'.join(lines), '\n'.join(svg) + '\n'


def update(root, data=None):
    block, svg = render(data)
    path = Path(root) / 'README.md'
    readme = path.read_text()
    if readme.count(START) != 1 or readme.count(END) != 1 or readme.index(START) > readme.index(END):
        ledger.fail('README requires exactly one ordered performance block')
    before, tail = readme.split(START)
    _, after = tail.split(END)
    path.write_text(before + block + after)
    chart = Path(root) / CHART
    chart.parent.mkdir(parents=True, exist_ok=True)
    chart.write_text(svg)



def git(root, *args):
    result = subprocess.run(['git', '-C', str(root), *args], stdout=subprocess.PIPE,
                            stderr=subprocess.DEVNULL, check=False)
    if result.returncode:
        ledger.fail('required git evidence unavailable')
    return result.stdout.decode('utf-8')


def readme_parts(readme):
    if readme.count(START) != 1 or readme.count(END) != 1 or readme.index(START) > readme.index(END):
        ledger.fail('README requires exactly one ordered performance block')
    before, tail = readme.split(START)
    middle, after = tail.split(END)
    return before, START + middle + END, after


def check(root, base=None, head='HEAD'):
    """Validate committed evidence; public hashes need independent private review."""
    head = git(root, 'rev-parse', '--verify', head + '^{commit}').strip()
    ledger.digest(head, revision=True)
    raw = git(root, 'show', head + ':' + AGGREGATE)
    data = ledger.parse_record(raw, limit=statistics.MAX_PROTOCOL_BYTES)
    validate_aggregate(data)
    pr = data['pr_evidence']
    source = pr['candidate_source_revision']
    git(root, 'merge-base', '--is-ancestor', source, head)
    expected_block, expected_svg = render(data)
    _, actual_block, _ = readme_parts(git(root, 'show', head + ':README.md'))
    if actual_block != expected_block or git(root, 'show', head + ':' + CHART) != expected_svg:
        ledger.fail('README or performance SVG is stale')
    pair_modes = {}
    for receipt in data['pair_receipts']:
        pair_modes.setdefault(receipt['pair_id'], set()).add(receipt['mode'])
        if receipt['receipt_sha256'] == pr['receipt_sha256'] and receipt['workload_source_revision'] != source:
            ledger.fail('unrelated reference pair cannot establish PR use')
    if not any(modes == set(ARMS) for modes in pair_modes.values()):
        ledger.fail('actual native receipts for both assigned arms required')
    for mode, arm in data['arms'].items():
        if arm['wall_seconds']['known'] == 0:
            ledger.fail('observed comparison time is unavailable')
        receipts = sum(r['mode'] == mode for r in data['pair_receipts'])
        if receipts != arm['assigned'] - arm['outcomes']['unstarted']:
            ledger.fail('native receipts must cover every started assignment')
    # Resolve every evidence-tail commit, not just the final diff: changing code
    # and reverting it after measurement still invalidates the measured source.
    allowed = {AGGREGATE, CHART, 'README.md'}
    for commit in git(root, 'rev-list', source + '..' + head).splitlines():
        parents = git(root, 'rev-list', '--parents', '-n', '1', commit).split()
        if len(parents) != 2:
            ledger.fail('evidence tail must be linear')
        parent = parents[1]
        paths = set(git(root, 'diff', '--name-only', '--no-renames', '-z', parent, commit).split('\0')) - {''}
        if paths - allowed:
            ledger.fail('source changed after native validation')
        if 'README.md' in paths:
            old = readme_parts(git(root, 'show', parent + ':README.md'))
            new = readme_parts(git(root, 'show', commit + ':README.md'))
            if (old[0], old[2]) != (new[0], new[2]):
                ledger.fail('README outside generated evidence changed after validation')
    if base:
        ledger.digest(base, revision=True)
        git(root, 'rev-parse', '--verify', base + '^{commit}')
        merge_base = git(root, 'merge-base', base, source).strip()
        if merge_base == source:
            ledger.fail('reference evidence predates this PR candidate')
        changed = set(git(root, 'diff', '--name-only', '--no-renames', '-z', base, head).split('\0'))
        if not {AGGREGATE, CHART, 'README.md'} <= changed:
            ledger.fail('each PR must update its public aggregate, README and chart')
    return data


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest='command', required=True)
    a = sub.add_parser('aggregate')
    for option in ('protocol', 'state-dir', 'review-evidence-sha256', 'receipts', 'output'):
        a.add_argument('--' + option, required=True)
    a.add_argument('--reviewed-unconfounded', action='store_true')
    r = sub.add_parser('render')
    source = r.add_mutually_exclusive_group(required=True)
    source.add_argument('--aggregate'); source.add_argument('--unavailable', action='store_true')
    r.add_argument('--root', type=Path, default=ROOT)
    c = sub.add_parser('check')
    c.add_argument('--root', type=Path, default=ROOT)
    c.add_argument('--base')
    c.add_argument('--head', default='HEAD')
    args = parser.parse_args(argv)
    try:
        if args.command == 'aggregate':
            result = aggregate(args.protocol, args.state_dir, args.review_evidence_sha256, args.reviewed_unconfounded, statistics.read_json(args.receipts)[0])
            with Path(args.output).open('x') as handle:
                json.dump(result, handle, sort_keys=True, indent=2, allow_nan=False)
                handle.write('\n')
        elif args.command == 'check':
            check(args.root, args.base, args.head)
        else:
            data = statistics.read_json(args.aggregate)[0] if args.aggregate else None
            update(args.root, data)
        return 0
    except (ledger.MeasurementError, OSError, UnicodeError, KeyError, TypeError, ValueError):
        print('BLOCK: performance report refused\nDetail: invalid or unreviewed evidence\nFix: reconcile the frozen protocol, ledger and independent review', file=sys.stderr)
        return 1


if __name__ == '__main__':
    sys.exit(main())
