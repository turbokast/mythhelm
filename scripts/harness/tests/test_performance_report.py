"""Synthetic reporting fixtures; never live-native performance evidence."""
import copy
import json
from pathlib import Path
import sys
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
        return report.aggregate(protocol, self.fixture.state, 'd' * 64, True)

    def test_actual_protocol_and_ledger_drive_both_views(self):
        data = self.evidence(outcomes={(0, 'mythhelm'): 'timeout'}, walls={(0, 'mythhelm'): None})
        block, svg = report.render(data)
        self.assertIn('| MYTHHELM | 1 / 2 | 20.00 s (1 observed; 1 missing)', block)
        self.assertIn('MYTHHELM: 20.00 s (1 observed; 1 missing)', svg)
        self.assertEqual(data['arms']['mythhelm']['outcomes']['timeout'], 1)
        self.assertIn('pilot_not_confirmatory', block)
        self.assertNotIn('population', data)

    def test_missing_usage_is_unavailable_never_zero(self):
        data = self.evidence()
        block, _ = report.render(data)
        self.assertIsNone(data['arms']['mythhelm']['tokens']['input_tokens']['total'])
        self.assertIn('unavailable / 2', block)
        corrupted = copy.deepcopy(data)
        corrupted['arms']['mythhelm']['tokens']['input_tokens']['total'] = 0
        with self.assertRaises(ledger.MeasurementError):
            report.render(corrupted)

    def test_unstarted_and_unfinished_assignments_retained(self):
        protocol = self.fixture.root / 'protocol.json'
        report.statistics.freeze(self.fixture.draft(), protocol)
        data = report.aggregate(protocol, self.fixture.state, 'd' * 64, True)
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
            report.aggregate(protocol, self.fixture.state, 'd' * 64, True)

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


if __name__ == '__main__':
    unittest.main()
