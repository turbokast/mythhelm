"""Synthetic reporting fixtures; never live-native performance evidence."""
import copy
import json
from pathlib import Path
import sys
import subprocess
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import dogfood_measurement as ledger
import performance_report as report
import test_dogfood_statistics as fixtures


class PerformanceReportTests(unittest.TestCase):
    def setUp(self):
        self.fixture = fixtures.StatisticsTests()
        self.fixture.setUp()
        self.addCleanup(self.fixture.doCleanups)
        self.root = self.fixture.root

    def evidence(self, **kwargs):
        protocol = self.fixture.assign(self.fixture.draft(), **kwargs)
        return report.aggregate(protocol, self.fixture.state, 'd' * 64, True, self.receipts(protocol))


    def receipts(self, protocol):
        plan = json.loads(protocol.read_text())
        ids = []
        if self.fixture.state.exists():
            with ledger.private_lock(self.fixture.state) as directory, ledger.ledger_file(directory) as handle:
                starts, _ = ledger.fold(handle)
                ids = sorted(starts)
        return {'pair_receipts': [{'attempt_id': identifier, 'receipt_sha256': format(i + 1, '064x'),
                                  'tool_source_revision': 'f' * 40} for i, identifier in enumerate(ids)],
                'pr_evidence': {'candidate_source_revision': 'a' * 40, 'delivery_scope': 'validation-use',
                                'authored_via': 'direct-native', 'receipt_sha256': 'e' * 64,
                                'tool_source_revision': 'f' * 40, 'validation_input_revision': 'a' * 40,
                                'trial_configuration_sha256': None, 'validation_profile': 'same-source'}}

    def test_actual_protocol_and_ledger_drive_both_views(self):
        data = self.evidence(outcomes={(0, 'mythhelm'): 'timeout'}, walls={(0, 'mythhelm'): None})
        block, svg = report.render(data)
        self.assertIn('| MYTHHELM | 1 / 2 | 20.00 s (1 observed; 1 missing)', block)
        self.assertIn('MYTHHELM: 20.00 s (1 observed; 1 missing)', svg)
        self.assertEqual(data['arms']['mythhelm']['outcomes']['timeout'], 1)
        self.assertIn('pilot_not_confirmatory', block)
        self.assertNotIn('population', data)
        lead = block.split('<details>')[0]
        self.assertIn('Native harness: **' + data['native']['harness'] + '**', lead)
        self.assertIn('PR scope: **validation-use**', lead)
        self.assertIn('not general coding delivery', lead)
        self.assertIn('Observed validation-use time', svg)
        self.assertIn(data['native']['harness'], svg)
        self.assertIn('MYTHHELM comparison build', block)

    def test_missing_usage_is_unavailable_never_zero(self):
        data = self.evidence()
        block, _ = report.render(data)
        self.assertIsNone(data['arms']['mythhelm']['tokens']['input_tokens']['total'])
        self.assertIn('unavailable / 2', block)
        corrupted = copy.deepcopy(data)
        corrupted['arms']['mythhelm']['tokens']['input_tokens']['total'] = 0
        with self.assertRaises(ledger.MeasurementError):
            report.render(corrupted)

    def test_unstarted_assignments_retained(self):
        protocol = self.fixture.root / 'protocol.json'
        report.statistics.freeze(self.fixture.draft(), protocol)
        data = report.aggregate(protocol, self.fixture.state, 'd' * 64, True, self.receipts(protocol))
        self.assertEqual(data['arms']['mythhelm']['outcomes']['unstarted'], 2)
        self.assertEqual(data['arms']['direct-native']['wall_seconds']['missing'], 2)
        block, svg = report.render(data)
        self.assertIn('unavailable (0 observed; 2 missing)', block)
        self.assertNotIn('height="20"', svg)

    def test_unknown_parity_and_missing_review_refused(self):
        protocol = self.fixture.assign(self.fixture.draft())
        with self.assertRaises(ledger.MeasurementError):
            report.aggregate(protocol, self.fixture.state, 'd' * 64)
        events_path = self.fixture.state / 'events.jsonl'
        events = [json.loads(line) for line in events_path.read_text().splitlines()]
        for event in events:
            if event['kind'] == 'begin':
                event['record']['model'] = None
        events_path.write_text(''.join(json.dumps(e) + '\n' for e in events))
        with self.assertRaises(ledger.MeasurementError):
            report.aggregate(protocol, self.fixture.state, 'd' * 64, True, self.receipts(protocol))

    def test_surrounding_readme_is_preserved_and_render_is_idempotent(self):
        readme = self.root / 'README.md'
        readme.write_text('before\n' + report.START + '\nold\n' + report.END + '\nafter\n')
        report.update(self.root)
        first = readme.read_bytes()
        report.update(self.root)
        self.assertEqual(first, readme.read_bytes())
        self.assertTrue(first.startswith(b'before\n'))
        self.assertTrue(first.endswith(b'\nafter\n'))
        self.assertIn('No current validated matched pairs', (self.root / report.CHART).read_text())

    def test_malformed_blocks_fail_before_mutation(self):
        readme = self.root / 'README.md'
        for text in ('missing', report.END + report.START, report.START * 2 + report.END):
            readme.write_text(text)
            with self.assertRaises(ledger.MeasurementError):
                report.update(self.root)
            self.assertEqual(text, readme.read_text())

    def test_unreviewed_private_or_inconsistent_aggregate_refused(self):
        data = self.evidence()
        for mutate in (lambda d: d.update(private_prompt='secret'),
                       lambda d: d.update(comparison_unconfounded=False),
                       lambda d: d['arms']['mythhelm']['outcomes'].update(timeout=10),
                       lambda d: d['native'].update(model='/private/path'),
                       lambda d: d['arms']['mythhelm']['wall_seconds'].update(total=float('nan'))):
            changed = copy.deepcopy(data)
            mutate(changed)
            with self.assertRaises(ledger.MeasurementError):
                report.render(changed)


    def test_sorted_json_token_columns_match_headers(self):
        data = self.evidence()
        for arm in data['arms'].values():
            for i, token in enumerate(ledger.TOKENS):
                arm['tokens'][token] = {'total': 101 * (i + 1), 'known': 2, 'missing': 0}
        reloaded = json.loads(json.dumps(data, sort_keys=True))
        block, _ = report.render(reloaded)
        self.assertIn('| mythhelm | 101 / 0 | 202 / 0 | 303 / 0 | 404 / 0 |', block)

    def test_acceptance_missing_time_and_over_budget_are_not_budgeted_success(self):
        data = self.evidence(walls={(0, 'mythhelm'): 120, (1, 'mythhelm'): None})
        self.assertEqual(data['arms']['mythhelm']['outcomes']['accepted'], 2)
        self.assertEqual(data['arms']['mythhelm']['accepted_within_budget'], 0)
        self.assertIn('| MYTHHELM | 0 / 2 |', report.render(data)[0])

    def git(self, *args):
        return subprocess.check_output(['git', '-C', str(self.root), *args], stderr=subprocess.DEVNULL).decode().strip()

    def commit(self):
        self.git('add', 'candidate.txt', 'README.md', report.CHART)
        if (self.root / report.AGGREGATE).exists():
            self.git('add', report.AGGREGATE)
        self.git('-c', 'core.hooksPath=/dev/null', 'commit', '-qm', 'synthetic fixture')
        return self.git('rev-parse', 'HEAD')

    def public_candidate(self, data=None):
        data = data or self.evidence()
        self.git('init', '-q')
        self.git('config', 'user.name', 'Synthetic fixture')
        self.git('config', 'user.email', 'synthetic@example.com')
        (self.root / 'candidate.txt').write_text('baseline')
        (self.root / 'README.md').write_text('outside\n' + report.START + report.END + '\nend\n')
        report.update(self.root)
        base = self.commit()
        (self.root / 'candidate.txt').write_text('measured source')
        source = self.commit()
        data['pr_evidence']['candidate_source_revision'] = source
        data['pr_evidence']['validation_input_revision'] = source
        for receipt in data['pair_receipts']:
            receipt['workload_source_revision'] = source
        data['bindings'] = [[source, b[1], b[2]] for b in data['bindings']]
        (self.root / report.AGGREGATE).parent.mkdir(parents=True, exist_ok=True)
        (self.root / report.AGGREGATE).write_text(json.dumps(data, sort_keys=True))
        report.update(self.root, data)
        head = self.commit()
        return data, base, source, head

    def test_gate_accepts_failed_trials_and_exact_evidence_tail(self):
        data = self.evidence(outcomes={(0, 'mythhelm'): 'error', (1, 'direct-native'): 'timeout'})
        data, base, _, head = self.public_candidate(data)
        result = report.check(self.root, base, head)
        self.assertEqual(result['arms']['mythhelm']['outcomes']['error'], 1)
        self.assertEqual(result['pr_evidence']['delivery_scope'], 'validation-use')

    def test_gate_rejects_source_change_even_if_reverted(self):
        _, base, _, _ = self.public_candidate()
        (self.root / 'candidate.txt').write_text('late source change')
        self.commit()
        (self.root / 'candidate.txt').write_text('measured source')
        self.commit()
        with self.assertRaises(ledger.MeasurementError):
            report.check(self.root, base)

    def test_gate_rejects_unavailable_or_stale_publication(self):
        _, base, _, _ = self.public_candidate()
        report.update(self.root)
        self.commit()
        with self.assertRaises(ledger.MeasurementError):
            report.check(self.root, base)

    def test_gate_rejects_unrelated_native_receipt_and_old_pr_source(self):
        data, base, source, _ = self.public_candidate()
        for receipt in data['pair_receipts']:
            receipt['workload_source_revision'] = 'a' * 40
        data['bindings'] = [['a' * 40, b[1], b[2]] for b in data['bindings']]
        data['pr_evidence']['receipt_sha256'] = data['pair_receipts'][0]['receipt_sha256']
        (self.root / report.AGGREGATE).write_text(json.dumps(data))
        report.update(self.root, data)
        self.commit()
        with self.assertRaises(ledger.MeasurementError):
            report.check(self.root, base)
        data['pr_evidence']['receipt_sha256'] = 'e' * 64
        (self.root / report.AGGREGATE).write_text(json.dumps(data))
        report.update(self.root, data)
        self.commit()
        with self.assertRaises(ledger.MeasurementError):
            report.check(self.root, source)

    def test_gate_rejects_readme_outside_block_change(self):
        _, base, _, _ = self.public_candidate()
        readme = self.root / 'README.md'
        readme.write_text('late prose\n' + readme.read_text())
        self.commit()
        with self.assertRaises(ledger.MeasurementError):
            report.check(self.root, base)


    def test_squash_publication_checks_freshness_without_original_ancestry(self):
        _, base, source, head = self.public_candidate()
        self.git('reset', '--hard', base)
        self.git('merge', '--squash', head)
        self.commit()
        with self.assertRaises(ledger.MeasurementError):
            report.check(self.root, base, event='pull_request')
        self.assertEqual(report.check(self.root, event='push')['pr_evidence']['candidate_source_revision'], source)

    def test_merge_queue_checks_artifacts_without_rejecting_synthetic_merge(self):
        _, base, _, head = self.public_candidate()
        self.git('checkout', '-b', 'other-pr', base)
        (self.root / 'other.txt').write_text('another reviewed PR')
        self.git('add', 'other.txt')
        other = self.commit()
        self.git('checkout', '-b', 'merge-queue', head)
        self.git('merge', '--no-ff', '--no-edit', other)
        with self.assertRaises(ledger.MeasurementError):
            report.check(self.root, base)
        report.check(self.root, event='merge_group')
        (self.root / report.CHART).write_text('stale')
        self.commit()
        with self.assertRaises(ledger.MeasurementError):
            report.check(self.root, event='merge_group')

    def test_old_reference_pair_alone_does_not_cover_current_candidate(self):
        data, base, _, _ = self.public_candidate()
        for receipt in data['pair_receipts']:
            receipt['workload_source_revision'] = 'a' * 40
        data['bindings'] = [['a' * 40, b[1], b[2]] for b in data['bindings']]
        (self.root / report.AGGREGATE).write_text(json.dumps(data))
        report.update(self.root, data)
        self.commit()
        with self.assertRaises(ledger.MeasurementError):
            report.check(self.root, base)

    def test_trial_mapping_allows_only_exact_config_child_and_digest(self):
        _, _, source, _ = self.public_candidate()
        self.git('checkout', '-b', 'trial-input', source)
        configuration = b'[execution]\nvalidation = true\n'
        (self.root / 'mythhelm.toml').write_bytes(configuration)
        self.git('add', 'mythhelm.toml')
        trial = self.commit()
        pr = {'candidate_source_revision': source, 'validation_input_revision': trial,
              'trial_configuration_sha256': report.hashlib.sha256(configuration).hexdigest()}
        report.verify_trial_mapping(self.root, pr)
        pr['trial_configuration_sha256'] = '0' * 64
        with self.assertRaises(ledger.MeasurementError):
            report.verify_trial_mapping(self.root, pr)
        self.git('checkout', '-b', 'bad-trial-input', source)
        (self.root / 'candidate.txt').write_text('unreviewed source change')
        bad_trial = self.commit()
        pr['validation_input_revision'] = bad_trial
        with self.assertRaises(ledger.MeasurementError):
            report.verify_trial_mapping(self.root, pr)


    def test_direct_native_receipt_cannot_certify_mythhelm_pr_use(self):
        data, base, _, _ = self.public_candidate()
        direct = next(r for r in data['pair_receipts'] if r['mode'] == 'direct-native')
        data['pr_evidence']['receipt_sha256'] = direct['receipt_sha256']
        (self.root / report.AGGREGATE).write_text(json.dumps(data))
        report.update(self.root, data)
        self.commit()
        with self.assertRaises(ledger.MeasurementError):
            report.check(self.root, base)


if __name__ == '__main__':
    unittest.main()
