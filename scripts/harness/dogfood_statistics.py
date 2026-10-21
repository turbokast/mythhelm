#!/usr/bin/env python3
"""Freeze a paired protocol, then analyze its complete private assignment ledger.

freeze --draft FILE --output FILE creates a private protocol once; analyze
--protocol FILE --state-dir DIR reads measurements. Exit 1 on invalid evidence.
No agents, shells, paid routes or publication are launched. Bounds assume the
registered repository/family components are independent samples of the stated
population. Dependence outside those components invalidates that interpretation.
"""
import argparse
from collections import defaultdict
from datetime import datetime, timezone, timedelta
import hashlib
import json
import math
import os
from pathlib import Path
import random
import statistics
import sys

import dogfood_measurement as ledger

NATIVE_FIELDS = {"harness", "native_version", "model", "effort", "configuration_sha256", "environment_sha256"}
ASSIGNMENT_FIELDS = {"pair_id", "repository_id", "family", "task_id", "source_revision",
                     "contract_sha256", "checks_sha256", "mythhelm_id", "native_id"}
PLAN_FIELDS = {"experiment_id", "phase", "population", "seed", "alpha", "cap_seconds",
               "minimum_gain_seconds", "observation_window_days", "max_started_attempts", "native", "assignments", "cohort", "design"}
MAX_PROTOCOL_BYTES = 1024 * 1024


def read_json(path):
    with Path(path).open("rb") as handle:
        raw = handle.read(MAX_PROTOCOL_BYTES + 1)
    return ledger.parse_record(raw.decode("utf-8"), limit=MAX_PROTOCOL_BYTES), hashlib.sha256(raw).hexdigest()


def validate(plan, frozen=False):
    if set(plan) != PLAN_FIELDS | ({"registered_at"} if frozen else set()):
        ledger.fail("protocol fields differ from schema")
    for key in ("experiment_id", "population"):
        ledger.text(plan[key])
    if plan["phase"] not in ("pilot", "confirmatory"):
        ledger.fail("unknown experiment phase")
    if plan["cohort"] not in ("development", "holdout"):
        ledger.fail("unknown evaluation cohort")
    if plan["phase"] == "confirmatory" and plan["cohort"] != "holdout":
        ledger.fail("confirmatory results require a protected holdout")
    if type(plan["seed"]) is not int or not 0 <= plan["seed"] < 2**63:
        ledger.fail("invalid randomization seed")
    for key in ("alpha", "cap_seconds", "minimum_gain_seconds", "observation_window_days"):
        value = plan[key]
        if type(value) not in (int, float) or not math.isfinite(value):
            ledger.fail("invalid protocol number")
    if not .001 <= plan["alpha"] <= .2 or not 0 < plan["cap_seconds"] <= 86400:
        ledger.fail("invalid confidence or time ceiling")
    if not 0 < plan["minimum_gain_seconds"] <= plan["cap_seconds"] or not 0 <= plan["observation_window_days"] <= 365:
        ledger.fail("invalid useful effect or observation window")
    if plan["phase"] == "confirmatory" and plan["observation_window_days"] < 7:
        ledger.fail("confirmatory evidence needs a delayed defect window")
    native = plan["native"]
    if not isinstance(native, dict) or set(native) != NATIVE_FIELDS:
        ledger.fail("invalid native comparison manifest")
    ledger.text(native["harness"])
    for key in ("native_version", "model", "effort"):
        ledger.text(native[key], nullable=plan["phase"] == "pilot")
    for key in ("configuration_sha256", "environment_sha256"):
        ledger.digest(native[key], nullable=plan["phase"] == "pilot")
    assignments = plan["assignments"]
    if not isinstance(assignments, list) or not 1 <= len(assignments) <= 1000:
        ledger.fail("invalid assignment count")
    if type(plan["max_started_attempts"]) is not int or plan["max_started_attempts"] != 2 * len(assignments):
        ledger.fail("this protocol permits one attempt per assigned arm only")
    ids, pair_ids = set(), set()
    for pair in assignments:
        if not isinstance(pair, dict) or set(pair) != ASSIGNMENT_FIELDS | ({"first"} if frozen else set()):
            ledger.fail("invalid assignment schema")
        for key in ("pair_id", "repository_id", "family", "task_id", "mythhelm_id", "native_id"):
            ledger.text(pair[key])
        ledger.digest(pair["source_revision"], revision=True)
        ledger.digest(pair["contract_sha256"]); ledger.digest(pair["checks_sha256"])
        if pair["pair_id"] in pair_ids or pair["mythhelm_id"] in ids or pair["native_id"] in ids or pair["mythhelm_id"] == pair["native_id"]:
            ledger.fail("duplicate assignment")
        pair_ids.add(pair["pair_id"]); ids.update((pair["mythhelm_id"], pair["native_id"]))
    if frozen:
        ledger.timestamp(plan["registered_at"])
        if [p["first"] for p in assignments] != balanced_order(len(assignments), plan["seed"]):
            ledger.fail("assignment order does not match registered seed")
    required = sample_requirement(plan)
    if plan["phase"] == "confirmatory" and (required is None or len(components(assignments)) < required):
        ledger.fail("registered independent sample cannot meet prospective power design")


def sample_requirement(plan):
    design = plan["design"]
    if design is None and plan["phase"] == "pilot":
        return None
    if not isinstance(design, dict) or set(design) != {"expected_time_gain_seconds", "expected_acceptance_gain", "power", "prior_evidence_sha256"}:
        ledger.fail("invalid prospective sample-size design")
    for key in ("expected_time_gain_seconds", "expected_acceptance_gain", "power"):
        if type(design[key]) not in (int, float) or not math.isfinite(design[key]):
            ledger.fail("invalid prospective design number")
    gain, quality, power = (design[k] for k in ("expected_time_gain_seconds", "expected_acceptance_gain", "power"))
    if not plan["minimum_gain_seconds"] < gain <= plan["cap_seconds"] or not 0 < quality <= 1 or not .5 < power < 1:
        ledger.fail("prospective effect and power must be achievable")
    ledger.digest(design["prior_evidence_sha256"], nullable=plan["phase"] == "pilot")
    alpha, beta = plan["alpha"], 1 - power
    # Separate one-sided sampling deviations for both endpoints use beta/2;
    # combining those with the registered simultaneous bounds gives a
    # conservative prospective probability of satisfying both criteria.
    deviation = math.sqrt(2 * math.log(2 / beta))
    time = (plan["cap_seconds"] * (math.sqrt(2 * math.log(4 / alpha)) + deviation) / (gain - plan["minimum_gain_seconds"])) ** 2
    acceptance = ((math.sqrt(2 * math.log(2 / alpha)) + deviation) / quality) ** 2
    return math.ceil(max(time, acceptance))


def balanced_order(count, seed):
    order = ["mythhelm"] * ((count + 1) // 2) + ["direct-native"] * (count // 2)
    random.Random(seed).shuffle(order)
    return order


def freeze(draft, output):
    validate(draft)
    plan = dict(draft, registered_at=ledger.utc(), assignments=[dict(p, first=first)
                for p, first in zip(draft["assignments"], balanced_order(len(draft["assignments"]), draft["seed"]))])
    raw = (json.dumps(plan, sort_keys=True, indent=2, allow_nan=False) + "\n").encode("utf-8")
    if len(raw) > MAX_PROTOCOL_BYTES:
        ledger.fail("protocol exceeds size bound")
    try:
        fd = os.open(output, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    except FileExistsError as exc:
        raise ledger.MeasurementError("protocol already exists; never overwrite registration") from exc
    with os.fdopen(fd, "wb") as handle:
        handle.write(raw); handle.flush(); os.fsync(handle.fileno())
    return {"protocol_sha256": hashlib.sha256(raw).hexdigest(), "planned_attempts": 2 * len(draft["assignments"])}


def components(assignments):
    """Join repeated repositories OR task families before counting samples."""
    parents = list(range(len(assignments)))

    def find(index):
        while parents[index] != index:
            parents[index] = parents[parents[index]]; index = parents[index]
        return index

    seen = {}
    for index, pair in enumerate(assignments):
        for key in ("repository_id", "family"):
            value = (key, pair[key])
            if value in seen:
                parents[find(index)] = find(seen[value])
            seen[value] = index
    groups = defaultdict(list)
    for index in range(len(assignments)):
        groups[find(index)].append(index)
    return list(groups.values())


def analyze(protocol_path, state):
    plan, protocol_hash = read_json(protocol_path); validate(plan, frozen=True)
    if Path(state).exists():
        with ledger.private_lock(state) as directory, ledger.ledger_file(directory) as handle:
            starts, finishes = ledger.fold(handle); handle.seek(0)
            events = [ledger.parse_record(line.decode("utf-8")) for line in handle.read().splitlines()]
    else:
        starts, finishes, events = {}, {}, []
    ids = {p[k] for p in plan["assignments"] for k in ("mythhelm_id", "native_id")}
    for identifier, record in starts.items():
        if record["protocol_sha256"] == protocol_hash and identifier not in ids:
            ledger.fail("unregistered attempt under this protocol")
    begin_events = {e["id"]: e for e in events if e["kind"] == "begin"}
    finish_times = {e["id"]: e["at"] for e in events if e["kind"] == "finish"}
    observed_times = {e["id"]: e["at"] for e in events if e["kind"] in ("finish", "observe")}
    indices = {e["id"]: i for i, e in enumerate(events) if e["kind"] == "begin"}
    scores, quality, limitations = [], [], set()
    observed = sum(i in starts for i in ids)
    if observed != len(ids): limitations.add("unstarted_assignments")
    if any(i in starts and i not in finishes for i in ids): limitations.add("unfinished_assignments")
    for pair in plan["assignments"]:
        pair_ids = [pair["mythhelm_id"], pair["native_id"]]
        if any(i not in starts for i in pair_ids):
            scores.append(None); quality.append(None); continue
        for identifier in pair_ids:
            record = starts[identifier]
            if record["protocol_sha256"] != protocol_hash or ledger.timestamp(begin_events[identifier]["at"]) < ledger.timestamp(plan["registered_at"]):
                ledger.fail("registration must precede and bind every assignment")
            expected = {**{k: pair[k] for k in ("pair_id", "repository_id", "family", "task_id", "source_revision", "contract_sha256", "checks_sha256")}, **plan["native"]}
            if any(record[k] is None or record[k] != v for k, v in expected.items()):
                limitations.add("unknown_or_mismatched_parity")
            if record["mode"] != ("mythhelm" if identifier == pair_ids[0] else "direct-native"):
                limitations.add("arm_unavailable_or_misclassified")
        first = pair_ids[0] if pair["first"] == "mythhelm" else pair_ids[1]
        if indices[first] != min(indices[i] for i in pair_ids):
            limitations.add("randomized_order_not_followed")
        if any(i not in finishes for i in pair_ids):
            scores.append(None); quality.append(None); continue
        restricted, accepted = [], []
        missing_accepted_time = False
        for identifier in pair_ids:
            result = finishes[identifier]
            wall = result["wall_seconds"]
            if wall is None:
                limitations.add("missing_wall_measurement")
                if result["outcome"] == "accepted":
                    missing_accepted_time = True
            success = result["outcome"] == "accepted" and wall is not None and wall <= plan["cap_seconds"]
            accepted.append(int(success))
            restricted.append(wall if success else plan["cap_seconds"])
            if result["retries"] is None or result["retries"] != 0:
                limitations.add("retry_budget_unproven_or_exceeded")
            if result["bugs"] is None or not result["defect_observation_closed"]:
                limitations.add("defect_observation_pending")
            elif result["bugs"] > 0:
                limitations.add("observed_defects")
            end = ledger.timestamp(finish_times[identifier]) + timedelta(days=plan["observation_window_days"])
            if datetime.now(timezone.utc) < end:
                limitations.add("defect_window_not_elapsed")
            if result["defect_observation_closed"] and ledger.timestamp(observed_times[identifier]) < end:
                limitations.add("defect_window_has_no_closing_observation")
        scores.append(None if missing_accepted_time else restricted[0] - restricted[1])
        quality.append(None if missing_accepted_time else accepted[0] - accepted[1])
    groups = components(plan["assignments"])
    complete = all(x is not None for x in scores)
    unit_scores = [statistics.mean(scores[i] for i in group) for group in groups] if complete else []
    unit_quality = [statistics.mean(quality[i] for i in group) for group in groups] if complete else []
    n = len(groups)
    time_mean = statistics.mean(unit_scores) if unit_scores else None
    quality_mean = statistics.mean(unit_quality) if unit_quality else None
    interval, lower = None, None
    inferential_blocks = {"unstarted_assignments", "unfinished_assignments", "unknown_or_mismatched_parity",
                          "arm_unavailable_or_misclassified", "randomized_order_not_followed", "missing_wall_measurement",
                          "retry_budget_unproven_or_exceeded"}
    if complete and n >= 2 and not limitations & inferential_blocks:
        # Two prespecified endpoints share familywise alpha by Bonferroni.
        # Each independent component's paired time difference lies in [-T,T],
        # and its acceptance difference in [-1,1]. Hoeffding remains conservative
        # even if the observed sample variance is zero.
        endpoint_alpha = plan["alpha"] / 2
        width = plan["cap_seconds"] * math.sqrt(2 * math.log(2 / endpoint_alpha) / n)
        interval = [max(-plan["cap_seconds"], time_mean - width), min(plan["cap_seconds"], time_mean + width)]
        lower = max(-1, quality_mean - math.sqrt(2 * math.log(1 / endpoint_alpha) / n))
    if n < 2: limitations.add("single_dependent_component")
    if plan["phase"] != "confirmatory": limitations.add("pilot_not_confirmatory")
    criterion = (not limitations and interval is not None and interval[1] <= -plan["minimum_gain_seconds"] and lower >= 0)
    return dict(schema_version=1, experiment_id=plan["experiment_id"], protocol_sha256=protocol_hash,
                phase=plan["phase"], planned_pairs=len(plan["assignments"]), started_attempts=observed,
                unstarted_attempts=len(ids)-observed, unfinished_attempts=sum(i in starts and i not in finishes for i in ids),
                independent_components=n, estimand="equal_weight_mean_across_registered_repository_family_components",
                prospective_required_components=sample_requirement(plan),
                restricted_time_difference_seconds=dict(mean=time_mean, interval=interval),
                acceptance_difference=dict(mean=quality_mean, lower_bound=lower, registered_floor=0),
                familywise_alpha=plan["alpha"], method="bounded_independent_component_Hoeffding_Bonferroni_two_endpoints",
                joint_criterion_met=bool(criterion), publication_decision="requires_independent_review",
                limitations=sorted(limitations), assumptions=["components must be independent samples of the preregistered population",
                "failed, blocked and timed-out assignments score the registered time ceiling; this is restricted time, not successful-run latency",
                "no repeated looks, unregistered comparisons or task selection based on results",
                "known parity and numeric records require independently audited provenance"])


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="command", required=True)
    f = sub.add_parser("freeze"); f.add_argument("--draft", required=True); f.add_argument("--output", required=True)
    a = sub.add_parser("analyze"); a.add_argument("--protocol", required=True); a.add_argument("--state-dir", required=True)
    args = parser.parse_args(argv)
    try:
        result = freeze(read_json(args.draft)[0], args.output) if args.command == "freeze" else analyze(args.protocol, args.state_dir)
        print(json.dumps(result, sort_keys=True, allow_nan=False)); return 0
    except (ledger.MeasurementError, OSError, UnicodeError, TypeError, KeyError, OverflowError):
        print("BLOCK: comparison evidence refused\nDetail: invalid registration, assignment or ledger\nFix: preserve the original evidence and reconcile the protocol", file=sys.stderr)
        return 1


if __name__ == "__main__": sys.exit(main())
