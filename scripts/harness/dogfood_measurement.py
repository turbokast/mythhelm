#!/usr/bin/env python3
"""Private append-only delivery measurements; no process execution or promotion.

begin/finish --state-dir DIR --record FILE write validated metadata; finish also
requires --id. report reads that ledger. Exit 0 on success, 1 on refused input.
Raw outputs and account data are deliberately outside this schema. A crashed
writer may leave its lock; an operator must reconcile it, never auto-break it.
"""
import argparse
from collections import Counter, defaultdict
from contextlib import contextmanager
from datetime import datetime, timezone
import json
import math
import os
from pathlib import Path
import re
import stat
import statistics
import sys

MAX_RECORD_BYTES = 65536
MAX_LEDGER_BYTES = 16 * 1024 * 1024
IDENTIFIER = re.compile(r"[A-Za-z0-9][A-Za-z0-9_.-]{0,127}\Z")
HASH = re.compile(r"[0-9a-f]{64}\Z")
REVISION = re.compile(r"(?:[0-9a-f]{40}|[0-9a-f]{64})\Z")
TOKENS = ("input_tokens", "output_tokens", "cache_read_tokens", "cache_write_tokens")
COUNTS = TOKENS + ("retries", "bugs", "errors", "human_interventions")
TIMES = ("wall_seconds", "native_seconds", "verification_seconds", "review_seconds")
METRICS = TIMES + COUNTS
PARITY = ("task_id", "repository_id", "source_revision", "contract_sha256", "checks_sha256", "harness",
          "native_version", "model", "effort", "configuration_sha256", "environment_sha256", "family")
BEGIN_FIELDS = set(PARITY) | {"id", "mode", "pair_id", "fallback_reason", "protocol_sha256"}
FINISH_FIELDS = set(METRICS) | {"outcome", "usage_source", "candidate_revision",
                              "acceptance_evidence_sha256", "defect_observation_closed"}
MODES = ("mythhelm", "direct-native", "unavailable")
OUTCOMES = ("accepted", "rejected", "error", "timeout", "blocked")


class MeasurementError(ValueError):
    """Refusal whose message never includes supplied data."""


def fail(message):
    raise MeasurementError(message)


def parse_record(raw, limit=MAX_RECORD_BYTES):
    if not isinstance(raw, str) or len(raw.encode("utf-8")) > limit:
        fail("record exceeds input bound")

    def pairs(items):
        result = {}
        for key, value in items:
            if key in result:
                fail("duplicate JSON key")
            result[key] = value
        return result

    def constant(_):
        fail("nonfinite JSON number")

    try:
        value = json.loads(raw, object_pairs_hook=pairs, parse_constant=constant)
    except (ValueError, RecursionError) as exc:
        raise MeasurementError("invalid JSON record") from exc
    if not isinstance(value, dict):
        fail("record must be an object")
    return value


def text(value, nullable=False):
    if value is None and nullable:
        return
    if not isinstance(value, str) or not IDENTIFIER.fullmatch(value):
        fail("invalid metadata identifier")


def digest(value, nullable=False, revision=False):
    if value is None and nullable:
        return
    if not isinstance(value, str) or not (REVISION if revision else HASH).fullmatch(value):
        fail("invalid revision or digest")


def validate_begin(record):
    if not isinstance(record, dict) or set(record) != BEGIN_FIELDS:
        fail("begin record fields differ from schema")
    for key in ("id", "task_id", "repository_id", "harness", "family"):
        text(record[key])
    for key in ("native_version", "model", "effort", "pair_id", "fallback_reason"):
        text(record[key], nullable=True)
    for key in ("native_version", "model", "effort"):
        if isinstance(record[key], str) and record[key].lower() in ("unknown", "unobserved", "not-established"):
            fail("unknown configuration must use null")
    digest(record["source_revision"], revision=True)
    for key in ("contract_sha256", "checks_sha256"):
        digest(record[key])
    for key in ("configuration_sha256", "environment_sha256"):
        digest(record[key], nullable=True)
    digest(record["protocol_sha256"], nullable=True)
    if record["mode"] not in MODES:
        fail("invalid delivery mode")
    if record["mode"] != "mythhelm" and not record["fallback_reason"]:
        fail("non-MYTHHELM execution requires a recorded reason")


def validate_finish(record):
    if not isinstance(record, dict) or set(record) != FINISH_FIELDS:
        fail("finish record fields differ from schema")
    if record["outcome"] not in OUTCOMES:
        fail("invalid outcome")
    for key in METRICS:
        value = record[key]
        if value is None:
            continue
        if type(value) not in (int, float) or not math.isfinite(value) or value < 0:
            fail("invalid nonnegative metric")
        if key in COUNTS and type(value) is not int:
            fail("count must be an integer")
    if record["usage_source"] not in ("reported", "observed", "unknown"):
        fail("invalid usage provenance")
    if record["usage_source"] == "unknown" and any(record[k] is not None for k in TOKENS):
        fail("unknown usage must retain missing counters")
    digest(record["candidate_revision"], nullable=True, revision=True)
    digest(record["acceptance_evidence_sha256"], nullable=True)
    if type(record["defect_observation_closed"]) is not bool:
        fail("defect observation status must be boolean")
    if record["defect_observation_closed"] and any(record[k] is None for k in ("bugs", "errors")):
        fail("closed observation requires known defect and error counts")
    if record["outcome"] == "accepted" and not (record["candidate_revision"] and record["acceptance_evidence_sha256"]):
        fail("acceptance requires candidate-bound independent evidence")


def validate_observation(record):
    if not isinstance(record, dict) or set(record) != {"bugs", "errors", "evidence_sha256", "defect_observation_closed"}:
        fail("observation fields differ from schema")
    for key in ("bugs", "errors"):
        if record[key] is not None and (type(record[key]) is not int or record[key] < 0):
            fail("invalid observed count")
    digest(record["evidence_sha256"])
    if type(record["defect_observation_closed"]) is not bool:
        fail("invalid observation status")
    if record["defect_observation_closed"] and any(record[k] is None for k in ("bugs", "errors")):
        fail("closed observation requires known defect and error counts")


def apply_observation(finishes, identifier, record):
    validate_observation(record)
    if identifier not in finishes:
        fail("observation requires a finished attempt")
    old = finishes[identifier]
    for key in ("bugs", "errors"):
        if old[key] is not None and (record[key] is None or record[key] < old[key]):
            fail("observations cannot silently erase known defects")
    if old["defect_observation_closed"] and not record["defect_observation_closed"]:
        fail("closed observation cannot silently reopen")
    finishes[identifier] = dict(old, **{k: record[k] for k in ("bugs", "errors", "defect_observation_closed")})


def utc():
    return datetime.now(timezone.utc).isoformat()


def timestamp(value):
    if not isinstance(value, str):
        fail("invalid event timestamp")
    try:
        parsed = datetime.fromisoformat(value[:-1] + "+00:00" if value.endswith("Z") else value)
    except (TypeError, ValueError) as exc:
        raise MeasurementError("invalid event timestamp") from exc
    if parsed.tzinfo is None or parsed.utcoffset().total_seconds() != 0:
        fail("timestamp must be UTC")
    return parsed


@contextmanager
def private_lock(state, create=False):
    state = Path(state)
    if create:
        state.mkdir(mode=0o700, parents=True, exist_ok=True)
    if not state.exists():
        fail("ledger directory absent")
    info = state.lstat()
    if not stat.S_ISDIR(info.st_mode) or stat.S_IMODE(info.st_mode) != 0o700:
        fail("ledger directory must be private and not a symlink")
    if hasattr(os, "getuid") and info.st_uid != os.getuid():
        fail("ledger directory owner mismatch")
    lock = state / "lock"
    try:
        lock.mkdir(mode=0o700)
    except FileExistsError as exc:
        raise MeasurementError("ledger locked; reconcile its owner before retry") from exc
    try:
        yield state
    finally:
        lock.rmdir()


@contextmanager
def ledger_file(state, create=False):
    flags = os.O_RDWR if create else os.O_RDONLY
    flags |= getattr(os, "O_NOFOLLOW", 0)
    if create:
        flags |= os.O_CREAT | os.O_APPEND
    path = state / "events.jsonl"
    if path.is_symlink():
        fail("ledger symlink refused")
    try:
        fd = os.open(path, flags, 0o600)
    except OSError as exc:
        raise MeasurementError("cannot open private ledger") from exc
    with os.fdopen(fd, "r+b" if create else "rb") as handle:
        info = os.fstat(handle.fileno())
        if not stat.S_ISREG(info.st_mode) or info.st_nlink != 1 or stat.S_IMODE(info.st_mode) != 0o600:
            fail("ledger must be a private regular file without aliases")
        if hasattr(os, "getuid") and info.st_uid != os.getuid():
            fail("ledger file owner mismatch")
        if info.st_size > MAX_LEDGER_BYTES:
            fail("ledger exceeds size bound")
        yield handle


def fold(handle):
    raw = handle.read(MAX_LEDGER_BYTES + 1)
    if len(raw) > MAX_LEDGER_BYTES or (raw and not raw.endswith(b"\n")):
        fail("oversized or incomplete ledger")
    starts, finishes = {}, {}
    for line in raw.splitlines():
        try:
            event = parse_record(line.decode("utf-8"))
        except UnicodeError as exc:
            raise MeasurementError("invalid ledger encoding") from exc
        if set(event) != {"kind", "at", "id", "record"}:
            fail("invalid event fields")
        text(event["id"]); timestamp(event["at"])
        identifier = event["id"]
        if event["kind"] == "begin":
            validate_begin(event["record"])
            if identifier != event["record"]["id"] or identifier in starts:
                fail("duplicate or mismatched begin")
            starts[identifier] = event["record"]
        elif event["kind"] == "finish":
            validate_finish(event["record"])
            if identifier not in starts or identifier in finishes:
                fail("unmatched or duplicate finish")
            if starts[identifier]["mode"] == "unavailable" and event["record"]["outcome"] != "blocked":
                fail("unavailable assignment must remain blocked")
            finishes[identifier] = event["record"]
        elif event["kind"] == "observe":
            apply_observation(finishes, identifier, event["record"])
        else:
            fail("unknown ledger event")
    return starts, finishes


def append(handle, kind, identifier, record):
    event = {"kind": kind, "at": utc(), "id": identifier, "record": record}
    data = (json.dumps(event, sort_keys=True, allow_nan=False) + "\n").encode("utf-8")
    if len(data) > MAX_RECORD_BYTES or os.fstat(handle.fileno()).st_size + len(data) > MAX_LEDGER_BYTES:
        fail("append exceeds ledger bounds")
    handle.write(data); handle.flush(); os.fsync(handle.fileno())
    return {"id": identifier, "kind": kind, "at": event["at"]}


def begin(state, record):
    validate_begin(record)
    with private_lock(state, create=True) as directory, ledger_file(directory, create=True) as handle:
        starts, _ = fold(handle)
        if record["id"] in starts:
            fail("duplicate begin refused")
        return append(handle, "begin", record["id"], record)


def finish(state, identifier, record):
    text(identifier); validate_finish(record)
    with private_lock(state) as directory, ledger_file(directory, create=True) as handle:
        starts, finishes = fold(handle)
        if identifier not in starts or identifier in finishes:
            fail("finish requires exactly one unfinished start")
        if starts[identifier]["mode"] == "unavailable" and record["outcome"] != "blocked":
            fail("unavailable assignment must remain blocked")
        return append(handle, "finish", identifier, record)


def observe(state, identifier, record):
    text(identifier); validate_observation(record)
    with private_lock(state) as directory, ledger_file(directory, create=True) as handle:
        _, finishes = fold(handle)
        apply_observation(finishes, identifier, record)
        return append(handle, "observe", identifier, record)


def report(state):
    with private_lock(state) as directory, ledger_file(directory) as handle:
        starts, finishes = fold(handle)
    modes = {}
    for mode in MODES:
        ids = [i for i, r in starts.items() if r["mode"] == mode]
        if not ids:
            continue
        done = [finishes[i] for i in ids if i in finishes]
        outcomes = dict(sorted(Counter(r["outcome"] for r in done).items()))
        walls = [r["wall_seconds"] for r in done if r["wall_seconds"] is not None]
        modes[mode] = dict(started=len(ids), completed=len(done), unfinished=len(ids) - len(done),
                           outcomes=outcomes, acceptance_rate=outcomes.get("accepted", 0) / len(ids),
                           missing_metrics={k: sum(i not in finishes or finishes[i][k] is None for i in ids) for k in METRICS},
                           completed_observed_wall_seconds=dict(count=len(walls), mean=statistics.mean(walls) if walls else None,
                                                               median=statistics.median(walls) if walls else None),
                           observed_metrics={k: descriptive([r[k] for r in done if r[k] is not None]) for k in METRICS},
                           defect_observation_pending=sum(i not in finishes or not finishes[i]["defect_observation_closed"] for i in ids))
    groups = defaultdict(list)
    for identifier, record in starts.items():
        if record["pair_id"] is not None:
            groups[record["pair_id"]].append(identifier)
    pairs, excluded = [], []
    for pair_id, ids in sorted(groups.items()):
        ids.sort(key=lambda i: starts[i]["mode"] != "mythhelm")
        if len(ids) != 2 or [starts[i]["mode"] for i in ids] != ["mythhelm", "direct-native"]:
            excluded.append(dict(pair_id=pair_id, reason="missing_or_repeated_arm", starts=len(ids)))
            continue
        a, b = (starts[i] for i in ids)
        if any(a[k] is None or b[k] is None or a[k] != b[k] for k in PARITY):
            excluded.append(dict(pair_id=pair_id, reason="unknown_or_mismatched_parity", starts=2))
            continue
        outcomes = [finishes[i]["outcome"] if i in finishes else "unfinished" for i in ids]
        difference = None
        if outcomes == ["accepted", "accepted"] and all(finishes[i]["wall_seconds"] is not None for i in ids):
            difference = finishes[ids[0]]["wall_seconds"] - finishes[ids[1]]["wall_seconds"]
        pairs.append(dict(pair_id=pair_id, repository_id=a["repository_id"], family=a["family"],
                          outcomes=outcomes, wall_difference_seconds=difference))
    return dict(schema_version=1, status="exploratory_not_confirmatory", modes=modes, pairs=pairs,
                excluded_pairs=excluded, limitations=["observational delivery is not randomized evaluation",
                "completed wall summaries include censored timeouts and are descriptive, not time to acceptance",
                "successful-pair wall differences exclude failed/censored outcomes and establish no speed advantage",
                "tokens and defect counts remain observations with missingness, not charges or proof of no bugs",
                "no confidence interval, superiority, equivalence, or publication approval is inferred"])


def descriptive(values):
    return dict(count=len(values), mean=statistics.mean(values) if values else None,
                median=statistics.median(values) if values else None)


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=("begin", "finish", "observe", "report"))
    parser.add_argument("--state-dir", type=Path, required=True)
    parser.add_argument("--record", type=Path)
    parser.add_argument("--id")
    args = parser.parse_args(argv)
    try:
        if args.command == "report":
            if args.record is not None or args.id is not None:
                fail("report takes no record or id")
            result = report(args.state_dir)
        else:
            if args.record is None or (args.command in ("finish", "observe")) != (args.id is not None):
                fail("record and finish id required")
            with args.record.open("rb") as handle:
                raw = handle.read(MAX_RECORD_BYTES + 1)
            record = parse_record(raw.decode("utf-8"))
            result = begin(args.state_dir, record) if args.command == "begin" else {"finish": finish, "observe": observe}[args.command](args.state_dir, args.id, record)
        print(json.dumps(result, sort_keys=True, allow_nan=False))
        return 0
    except (MeasurementError, OSError, UnicodeError, OverflowError, TypeError):
        print("BLOCK: measurement refused\nDetail: invalid input, ledger state or access\nFix: validate schema and reconcile the private ledger; no data was intentionally rewritten", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
