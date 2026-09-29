#!/usr/bin/env python3
"""adjudicate.py: records how the calling agent judged a vendor's output, and
reports what each vendor and stage has been worth.

    adjudicate.py record --call-id ID --findings N --confirmed N --rejected N [--note TEXT]
    adjudicate.py yield [--days N] [--json]

`record` appends one row to .claude/data/vendor-adjudications.jsonl for a
`completed` call found in vendor-calls.jsonl. It refuses (exit 65, nothing
written) an unknown or unfinished call, a second record for the same call, and
counts that do not add up (confirmed + rejected > findings). The findings left
over are ones the agent could not settle.

`yield` joins calls and adjudications over the trailing window (default 14 days)
and prints one line per (vendor, stage): calls, completed, adjudicated, findings,
confirmed, rejected, confirmed_pct and unadjudicated completed calls. A stage whose
confirmed share stays low is a candidate for removal from the policy.

Exit 0 on success, 64 on a usage error, 65 on a refused record.
"""

from __future__ import annotations

import datetime
import json
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import vendors as V  # noqa: E402

LOG = "vendor-adjudications.jsonl"


def cmd_record(args) -> int:
    def refuse(why):
        print(f"adjudicate.py: refused: {why}", file=sys.stderr)
        return 65
    if min(args.findings, args.confirmed, args.rejected) < 0:
        return refuse("counts must be non-negative")
    if args.confirmed + args.rejected > args.findings:
        return refuse("confirmed + rejected exceeds findings")
    calls = [r for r in V.read_rows("vendor-calls.jsonl") if r.get("call_id") == args.call_id]
    if not calls:
        return refuse(f"no call {args.call_id} in vendor-calls.jsonl")
    call = calls[-1]
    if call.get("outcome") != "completed":
        return refuse(f"call {args.call_id} did not complete ({call.get('reason')})")
    if any(r.get("call_id") == args.call_id for r in V.read_rows(LOG)):
        return refuse(f"call {args.call_id} is already adjudicated")
    V.append_row(LOG, {"schema_version": 1, "call_id": args.call_id, "vendor": call.get("vendor"),
                       "stage": call.get("stage"), "findings": args.findings, "confirmed": args.confirmed,
                       "rejected": args.rejected, "note": (args.note or None) and args.note[:300]})
    print(f"recorded={args.call_id}")
    return 0


def _within(row, since) -> bool:
    try:
        ts = datetime.datetime.strptime(row.get("ts", ""), "%Y-%m-%dT%H:%M:%SZ").replace(tzinfo=datetime.timezone.utc)
    except ValueError:
        return False
    return ts >= since


def yield_report(days: int) -> list[dict]:
    since = datetime.datetime.now(datetime.timezone.utc) - datetime.timedelta(days=days)
    calls = [r for r in V.read_rows("vendor-calls.jsonl") if _within(r, since)]
    adj = {r["call_id"]: r for r in V.read_rows(LOG) if r.get("call_id")}
    groups: dict[tuple, dict] = {}
    for c in calls:
        g = groups.setdefault((c.get("vendor"), c.get("stage")), {
            "vendor": c.get("vendor"), "stage": c.get("stage"), "calls": 0, "completed": 0, "adjudicated": 0,
            "findings": 0, "confirmed": 0, "rejected": 0, "unadjudicated": 0})
        g["calls"] += 1
        if c.get("outcome") != "completed":
            continue
        g["completed"] += 1
        a = adj.get(c.get("call_id"))
        if a is None:
            g["unadjudicated"] += 1
            continue
        g["adjudicated"] += 1
        for k in ("findings", "confirmed", "rejected"):
            g[k] += int(a.get(k) or 0)
    for g in groups.values():
        judged = g["confirmed"] + g["rejected"]
        g["confirmed_pct"] = round(100 * g["confirmed"] / judged) if judged else None
    return sorted(groups.values(), key=lambda g: (str(g["vendor"]), str(g["stage"])))


def cmd_yield(args) -> int:
    rows = yield_report(args.days)
    if args.json:
        print(json.dumps(rows, sort_keys=True))
        return 0
    print(f"yield_window_days={args.days} groups={len(rows)}")
    for g in rows:
        pct = "n/a" if g["confirmed_pct"] is None else f"{g['confirmed_pct']}%"
        print(f"yield: {g['vendor']} {g['stage']} calls={g['calls']} completed={g['completed']} "
              f"adjudicated={g['adjudicated']} findings={g['findings']} confirmed={g['confirmed']} "
              f"rejected={g['rejected']} confirmed_pct={pct} unadjudicated={g['unadjudicated']}")
    return 0


def main(argv=None) -> int:
    p = V.Parser.make("adjudicate.py", "record and report vendor adjudications")
    sub = p.add_subparsers(dest="cmd", required=True)
    s = sub.add_parser("record")
    s.add_argument("--call-id", required=True)
    s.add_argument("--findings", required=True, type=int)
    s.add_argument("--confirmed", required=True, type=int)
    s.add_argument("--rejected", required=True, type=int)
    s.add_argument("--note")
    s.set_defaults(fn=cmd_record)
    s = sub.add_parser("yield")
    s.add_argument("--days", type=int, default=14)
    s.add_argument("--json", action="store_true")
    s.set_defaults(fn=cmd_yield)
    args = p.parse_args(argv)
    return args.fn(args)


if __name__ == "__main__":
    sys.exit(main())
