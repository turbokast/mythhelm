"""Synthetic preregistration and all-assigned statistical controls."""
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import sys
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import dogfood_measurement as ledger
import dogfood_statistics as stats
from test_dogfood_measurement import start, end


class StatisticsTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(); self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name); self.state = self.root / "ledger"

    def draft(self, count=2):
        native = {k: start()[k] for k in ("harness", "native_version", "model", "effort", "configuration_sha256", "environment_sha256")}
        assignments = []
        for i in range(count):
            assignments.append(dict(pair_id=f"pair-{i}", repository_id=f"repo-{i}", family=f"family-{i}",
                                    task_id=f"task-{i}", source_revision="a"*40, contract_sha256="b"*64,
                                    checks_sha256="c"*64, mythhelm_id=f"m-{i}", native_id=f"n-{i}"))
        return dict(experiment_id="synthetic", phase="pilot", cohort="development", design=None, population="synthetic-only", seed=42,
                    alpha=.05, cap_seconds=100, minimum_gain_seconds=10, observation_window_days=0,
                    max_started_attempts=2*count, native=native, assignments=assignments)

    def assign(self, plan, outcomes=None, walls=None):
        frozen = self.root / "protocol.json"; stats.freeze(plan, frozen)
        protocol = json.loads(frozen.read_text()); digest = hashlib.sha256(frozen.read_bytes()).hexdigest()
        for i, pair in enumerate(protocol["assignments"]):
            order = ["mythhelm", "direct-native"] if pair["first"] == "mythhelm" else ["direct-native", "mythhelm"]
            for mode in order:
                key = "mythhelm_id" if mode == "mythhelm" else "native_id"
                r = start(pair[key], mode)
                r.update({k: pair[k] for k in ("pair_id", "repository_id", "family", "task_id", "source_revision", "contract_sha256", "checks_sha256")})
                r["protocol_sha256"] = digest
                ledger.begin(self.state, r)
                outcome = outcomes.get((i, mode), "accepted") if outcomes else "accepted"
                result = end(outcome, 20 if mode == "mythhelm" else 40)
                if walls is not None and (i, mode) in walls:
                    result["wall_seconds"] = walls[i, mode]
                result.update(retries=0, bugs=0, errors=0, defect_observation_closed=True)
                ledger.finish(self.state, pair[key], result)
        return frozen

    def test_freeze_is_balanced_seeded_and_no_overwrite(self):
        p = self.root / "p.json"; q = self.root / "q.json"
        stats.freeze(self.draft(4), p); stats.freeze(self.draft(4), q)
        a = json.loads(p.read_text()); b = json.loads(q.read_text())
        self.assertEqual(a["assignments"], b["assignments"])
        self.assertEqual(sum(x["first"] == "mythhelm" for x in a["assignments"]), 2)
        before = p.read_bytes()
        with self.assertRaises(ledger.MeasurementError): stats.freeze(self.draft(), p)
        self.assertEqual(p.read_bytes(), before)

    def test_all_assigned_timeout_penalty_is_not_success_only_timing(self):
        p = self.assign(self.draft(), {(0, "mythhelm"): "timeout"})
        r = stats.analyze(p, self.state)
        # First repo: 100-40=60; second repo:20-40=-20. Equal cluster mean=20.
        self.assertEqual(r["restricted_time_difference_seconds"]["mean"], 20)
        self.assertEqual(r["acceptance_difference"]["mean"], -.5)
        self.assertEqual(r["planned_pairs"], 2)
        self.assertEqual(r["started_attempts"], 4)
        self.assertFalse(r["joint_criterion_met"])

    def test_connected_repository_and_family_do_not_inflate_sample(self):
        d = self.draft(3); d["assignments"][1]["family"] = "family-0"
        d["assignments"][2]["repository_id"] = "repo-1"
        r = stats.analyze(self.assign(d), self.state)
        self.assertEqual(r["independent_components"], 1)
        self.assertIsNone(r["restricted_time_difference_seconds"]["interval"])
        self.assertFalse(r["joint_criterion_met"])

    def test_valid_zero_variance_sample_does_not_prove_quality_equivalence(self):
        r = stats.analyze(self.assign(self.draft()), self.state)
        self.assertEqual(r["acceptance_difference"]["mean"], 0)
        self.assertLess(r["acceptance_difference"]["lower_bound"], 0)
        self.assertFalse(r["joint_criterion_met"])
        self.assertEqual(r["publication_decision"], "requires_independent_review")

    def test_registered_digest_and_order_are_checked(self):
        p = self.assign(self.draft())
        d = json.loads(p.read_text()); d["seed"] = 100
        p.write_text(json.dumps(d))
        with self.assertRaises(ledger.MeasurementError): stats.analyze(p, self.state)

    def test_unstarted_assignments_remain_visible(self):
        p = self.root / "protocol.json"; stats.freeze(self.draft(), p)
        r = stats.analyze(p, self.state)
        self.assertEqual(r["unstarted_attempts"], 4)
        self.assertIsNone(r["restricted_time_difference_seconds"]["mean"])

    def test_unknown_parity_cannot_get_inference(self):
        p = self.assign(self.draft())
        # Simulate a malformed import. The append-only API itself never rewrites.
        f = self.state / "events.jsonl"; lines = [json.loads(s) for s in f.read_text().splitlines()]
        for event in lines:
            if event["kind"] == "begin": event["record"]["model"] = None
        f.write_text("".join(json.dumps(x)+"\n" for x in lines))
        r = stats.analyze(p, self.state)
        self.assertIsNone(r["restricted_time_difference_seconds"]["interval"])
        self.assertIn("unknown_or_mismatched_parity", r["limitations"])

    def test_bad_design_rejected_before_assignment(self):
        for key, value in [("alpha", 0), ("alpha", float("nan")), ("cap_seconds", -1),
                           ("minimum_gain_seconds", 101), ("seed", True), ("max_started_attempts", 1)]:
            with self.subTest(key=key):
                d = self.draft(); d[key] = value
                with self.assertRaises(ledger.MeasurementError): stats.freeze(d, self.root / "bad.json")

    def test_late_or_incomplete_registration_does_not_infer(self):
        p = self.assign(self.draft())
        d = json.loads(p.read_text()); d["registered_at"] = "2099-01-01T00:00:00+00:00"
        p.write_text(json.dumps(d))
        with self.assertRaises(ledger.MeasurementError): stats.analyze(p, self.state)

    def test_conservative_prospective_power_uses_independent_components(self):
        d = self.draft()
        d["design"] = dict(expected_time_gain_seconds=80, expected_acceptance_gain=.8,
                           power=.8, prior_evidence_sha256="9" * 64)
        self.assertEqual(stats.sample_requirement(d), 54)
        d.update(phase="confirmatory", cohort="holdout", observation_window_days=7)
        with self.assertRaises(ledger.MeasurementError): stats.freeze(d, self.root / "too-small.json")

    def test_missing_accepted_time_never_becomes_failure_time_or_zero(self):
        p = self.assign(self.draft(), walls={(0, "mythhelm"): None})
        r = stats.analyze(p, self.state)
        self.assertIsNone(r["restricted_time_difference_seconds"]["mean"])
        self.assertIsNone(r["acceptance_difference"]["mean"])
        self.assertIn("missing_wall_measurement", r["limitations"])


if __name__ == "__main__": unittest.main()
