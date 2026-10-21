"""Synthetic negative controls for the private delivery measurement ledger."""
import copy
import importlib.util
import json
import os
from pathlib import Path
import tempfile
import unittest

SCRIPT = Path(__file__).resolve().parents[1] / "dogfood_measurement.py"
spec = importlib.util.spec_from_file_location("dogfood_measurement", SCRIPT)
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)


def start(identifier="a", mode="mythhelm"):
    return dict(id=identifier, task_id="measurement-ledger", repository_id="mythhelm",
                source_revision="a" * 40, contract_sha256="b" * 64, checks_sha256="c" * 64,
                mode=mode, harness="claudecode", native_version="2.1.300", model="model-a",
                effort="high", configuration_sha256="d" * 64, environment_sha256="e" * 64,
                pair_id="pair-a", family="harness", protocol_sha256=None,
                fallback_reason=None if mode == "mythhelm" else "comparison_baseline")


def end(outcome="accepted", wall=12):
    return dict(outcome=outcome, wall_seconds=wall, native_seconds=None,
                verification_seconds=None, review_seconds=None, input_tokens=None,
                output_tokens=None, cache_read_tokens=None, cache_write_tokens=None,
                retries=None, bugs=None, errors=None, human_interventions=None,
                usage_source="unknown", candidate_revision="f" * 40,
                acceptance_evidence_sha256="1" * 64, defect_observation_closed=False)


class LedgerTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.state = Path(self.tmp.name) / "private"

    def reject_begin(self, record):
        with self.assertRaises(m.MeasurementError):
            m.begin(self.state, record)

    def test_unfinished_counts_in_acceptance_denominator(self):
        m.begin(self.state, start())
        m.finish(self.state, "a", end())
        m.begin(self.state, start("b"))
        r = m.report(self.state)
        self.assertEqual(r["modes"]["mythhelm"]["acceptance_rate"], .5)
        self.assertEqual(r["modes"]["mythhelm"]["unfinished"], 1)
        self.assertEqual(r["status"], "exploratory_not_confirmatory")

    def test_duplicate_begin_does_not_rewrite(self):
        m.begin(self.state, start())
        old = (self.state / "events.jsonl").read_bytes()
        self.reject_begin(start())
        self.assertEqual((self.state / "events.jsonl").read_bytes(), old)

    def test_finish_requires_exactly_one_start(self):
        with self.assertRaises(m.MeasurementError):
            m.finish(self.state, "missing", end())
        m.begin(self.state, start())
        m.finish(self.state, "a", end())
        with self.assertRaises(m.MeasurementError):
            m.finish(self.state, "a", end())

    def test_bad_begin_fields(self):
        for key, value in [("source_revision", "bad"), ("checks_sha256", "c" * 63),
                           ("source_revision", "a" * 41), ("source_revision", "a" * 63),
                           ("model", "bad\nmodel"), ("id", "../escape"),
                           ("repository_id", "/tmp/private"), ("mode", "fake"),
                           ("configuration_sha256", "bad"), ("effort", True)]:
            with self.subTest(key=key):
                r = start(); r[key] = value
                self.reject_begin(r)

    def test_unknown_fields_rejected(self):
        r = start(); r["raw_native_output"] = "private"
        self.reject_begin(r)
        m.begin(self.state, start())
        r = end(); r["argv"] = ["unsafe"]
        with self.assertRaises(m.MeasurementError):
            m.finish(self.state, "a", r)

    def test_fallback_needs_reason(self):
        r = start(mode="direct-native"); r["fallback_reason"] = None
        self.reject_begin(r)

    def test_unavailable_cannot_be_accepted(self):
        r = start(mode="unavailable"); r["fallback_reason"] = "approval_blocked"
        m.begin(self.state, r)
        with self.assertRaises(m.MeasurementError):
            m.finish(self.state, "a", end())
        m.finish(self.state, "a", end("blocked", None))
        self.assertEqual(m.report(self.state)["modes"]["unavailable"]["outcomes"], {"blocked": 1})

    def test_acceptance_requires_candidate_and_independent_evidence(self):
        m.begin(self.state, start())
        for key in ["candidate_revision", "acceptance_evidence_sha256"]:
            r = end(); r[key] = None
            with self.subTest(key=key), self.assertRaises(m.MeasurementError):
                m.finish(self.state, "a", r)

    def test_bad_metric_values(self):
        m.begin(self.state, start())
        for key, values in [("wall_seconds", [-1, float("nan"), float("inf"), True, "12"]),
                            ("retries", [-1, 1.2, True, "0"]),
                            ("bugs", [-1, False]), ("errors", [float("nan")]),
                            ("output_tokens", [-1, True, 2.5])]:
            for value in values:
                r = end(); r[key] = value
                with self.subTest(key=key, value=value), self.assertRaises(m.MeasurementError):
                    m.finish(self.state, "a", r)

    def test_unknown_usage_cannot_be_zero(self):
        m.begin(self.state, start())
        r = end(); r["input_tokens"] = 0
        with self.assertRaises(m.MeasurementError):
            m.finish(self.state, "a", r)
        r["usage_source"] = "reported"
        m.finish(self.state, "a", r)
        counts = m.report(self.state)["modes"]["mythhelm"]["missing_metrics"]
        self.assertEqual(counts["input_tokens"], 0)
        self.assertEqual(counts["output_tokens"], 1)

    def test_pair_is_bound_to_all_parity_fields(self):
        for field in ["source_revision", "contract_sha256", "checks_sha256", "harness",
                      "native_version", "model", "effort", "configuration_sha256", "environment_sha256"]:
            with self.subTest(field=field), tempfile.TemporaryDirectory() as d:
                state = Path(d) / "ledger"
                a = start(); b = start("b", "direct-native")
                b[field] = "2" * len(b[field])
                m.begin(state, a); m.begin(state, b)
                m.finish(state, "a", end(wall=15)); m.finish(state, "b", end(wall=10))
                self.assertEqual(m.report(state)["pairs"], [])
                self.assertTrue(m.report(state)["excluded_pairs"])

    def test_unknown_model_or_config_is_not_matching(self):
        for field in ["model", "effort", "configuration_sha256", "environment_sha256"]:
            with self.subTest(field=field), tempfile.TemporaryDirectory() as d:
                a = start(); b = start("b", "direct-native"); a[field] = b[field] = None
                for r in [a, b]: m.begin(Path(d) / "ledger", r)
                self.assertEqual(m.report(Path(d) / "ledger")["pairs"], [])

    def test_pair_difference_has_independent_literal_expected_value(self):
        m.begin(self.state, start()); m.begin(self.state, start("b", "direct-native"))
        m.finish(self.state, "a", end(wall=17)); m.finish(self.state, "b", end(wall=10))
        self.assertEqual(m.report(self.state)["pairs"][0]["wall_difference_seconds"], 7)

    def test_failed_pair_is_not_dropped_or_given_speedup(self):
        m.begin(self.state, start()); m.begin(self.state, start("b", "direct-native"))
        m.finish(self.state, "a", end("timeout", 480)); m.finish(self.state, "b", end(wall=10))
        r = m.report(self.state)
        self.assertIsNone(r["pairs"][0]["wall_difference_seconds"])
        self.assertEqual(r["pairs"][0]["outcomes"], ["timeout", "accepted"])
        self.assertEqual(r["modes"]["mythhelm"]["acceptance_rate"], 0)

    def test_repeated_attempts_do_not_create_independent_pairs(self):
        for r in [start(), start("b", "direct-native"), start("c")]: m.begin(self.state, r)
        self.assertEqual(m.report(self.state)["pairs"], [])

    def test_private_permissions(self):
        m.begin(self.state, start())
        self.assertEqual(self.state.stat().st_mode & 0o777, 0o700)
        self.assertEqual((self.state / "events.jsonl").stat().st_mode & 0o777, 0o600)

    def test_lock_refuses_concurrent_writer(self):
        self.state.mkdir(mode=0o700); (self.state / "lock").mkdir()
        self.reject_begin(start())
        self.assertFalse((self.state / "events.jsonl").exists())

    def test_symlink_ledger_and_state_refused(self):
        outside = Path(self.tmp.name) / "outside"; outside.write_text("preserve")
        self.state.mkdir(mode=0o700); (self.state / "events.jsonl").symlink_to(outside)
        self.reject_begin(start())
        self.assertEqual(outside.read_text(), "preserve")

    def test_malformed_existing_ledger_fails_closed(self):
        self.state.mkdir(mode=0o700)
        f = self.state / "events.jsonl"; f.write_text('{"kind": "bogus"}\n'); f.chmod(0o600)
        with self.assertRaises(m.MeasurementError): m.report(self.state)

    def test_duplicate_json_keys_and_oversize_input(self):
        for raw in ['{"id":"a","id":"b"}', '{"x":NaN}', ' ' * 65537]:
            with self.assertRaises(m.MeasurementError): m.parse_record(raw)

    def test_ledger_limit_rejects_before_append(self):
        m.begin(self.state, start())
        before = (self.state / "events.jsonl").read_bytes()
        old = m.MAX_LEDGER_BYTES
        try:
            m.MAX_LEDGER_BYTES = len(before)
            with self.assertRaises(m.MeasurementError): m.begin(self.state, start("b"))
        finally: m.MAX_LEDGER_BYTES = old
        self.assertEqual((self.state / "events.jsonl").read_bytes(), before)

    def test_delayed_defect_observation_keeps_original_events(self):
        m.begin(self.state, start()); m.finish(self.state, "a", end())
        before = (self.state / "events.jsonl").read_bytes()
        m.observe(self.state, "a", dict(bugs=2, errors=1, evidence_sha256="3" * 64, defect_observation_closed=True))
        self.assertTrue((self.state / "events.jsonl").read_bytes().startswith(before))
        r = m.report(self.state)["modes"]["mythhelm"]
        self.assertEqual(r["defect_observation_pending"], 0)
        self.assertEqual(r["observed_metrics"]["bugs"]["mean"], 2)

    def test_defects_cannot_be_silently_decreased(self):
        m.begin(self.state, start()); m.finish(self.state, "a", end())
        r = dict(bugs=2, errors=1, evidence_sha256="3" * 64, defect_observation_closed=False)
        m.observe(self.state, "a", r)
        r["bugs"] = 0
        with self.assertRaises(m.MeasurementError): m.observe(self.state, "a", r)

    def test_observation_requires_finished_attempt_and_known_closed_counts(self):
        m.begin(self.state, start())
        r = dict(bugs=None, errors=None, evidence_sha256="3" * 64, defect_observation_closed=True)
        with self.assertRaises(m.MeasurementError): m.observe(self.state, "a", r)
        m.finish(self.state, "a", end())
        with self.assertRaises(m.MeasurementError): m.observe(self.state, "a", r)


if __name__ == "__main__":
    unittest.main()
