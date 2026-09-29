#!/usr/bin/env python3
"""route_lane.py: decides which lane implements one task: `claude` (a Claude
subagent, the default), `codex` or `muse` (an implementer lane, lane.py).

    route_lane.py --spec-dir DIR --task N [--class C [--confidence P]] [--parallel] [--json]

Prints one line, `lane=<claude|codex|muse> rule=<id> class=<c> confidence=<p>`, and
appends the decision to .claude/data/lane-routing.jsonl. Exit 0 on every path (64 on
a usage error): this is a routing suggestion, and lane.py re-checks everything.

The class comes from --class, else from the advisory Jev `task-class` site, else
`uncertain`. The decision is logged only when some lane is opted in. The table,
first match wins:

  parallel-batch    the task runs in a parallel batch               claude
  no-files          the task lists no Files                         claude
  protected-path    a listed file is protected (lanes.protected_paths) claude
  class-gate        class uncertain, below the confidence floor, concurrency_risky,
                    ui_design or open_design                        claude
  long-context      class long_context_investigation, Budget complex, Muse lane
                    available                                       muse
  wide-change       more files than lanes.file_ceiling, Muse lane available  muse
  tight-spec        class mechanical_scripted or tight_spec_code, Budget trivial or
                    standard, at most lanes.file_ceiling files, Codex lane
                    available                                       codex
  default           anything else                                   claude

A lane is available when the vendor and its lane are opted in, bwrap is usable,
the CLI is signed in and today's caps are not exhausted.
"""

from __future__ import annotations

import json
import os
import subprocess
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import vendors as V  # noqa: E402
import lane as L  # noqa: E402

JEV_DIR = os.path.normpath(os.path.join(V.SCRIPT_DIR, "..", "jev"))
sys.path.insert(0, JEV_DIR)
try:
    from questions import TASK_CLASS_CONFIDENCE_MIN  # noqa: E402
except ImportError:  # the classifier is optional; without it every class is uncertain
    TASK_CLASS_CONFIDENCE_MIN = 1.1

CLAUDE_ONLY = {"uncertain", "concurrency_risky", "ui_design", "open_design"}
CODEX_CLASSES = {"mechanical_scripted", "tight_spec_code"}


def route(task: dict, klass: str, confidence: float, available: dict, parallel: bool, pol: dict) -> tuple[str, str]:
    """The pure routing table. `available` maps codex/muse to bool."""
    files = task["files"]
    ceiling = int(pol.get("lanes", {}).get("file_ceiling", 8))
    budget = V.budget_word(task["budget"])
    if parallel:
        return "claude", "parallel-batch"
    if not files:
        return "claude", "no-files"
    if V.protected_hits(pol, files, listed=files):
        return "claude", "protected-path"
    if klass in CLAUDE_ONLY or confidence < TASK_CLASS_CONFIDENCE_MIN:
        return "claude", "class-gate"
    if available.get("muse") and klass == "long_context_investigation" and budget == "complex":
        return "muse", "long-context"
    if available.get("muse") and len(files) > ceiling:
        return "muse", "wide-change"
    if (available.get("codex") and klass in CODEX_CLASSES and budget in ("trivial", "standard")
            and len(files) <= ceiling):
        return "codex", "tight-spec"
    return "claude", "default"


def lane_available(pol: dict, vendor: str) -> bool:
    if not V.lanes_enabled(pol, vendor):
        return False
    try:
        L.resolve_bwrap()
        V.check_available(pol, vendor)
    except (V.Unavailable, OSError, ValueError):
        return False
    counts = V.quota_counts()
    return (counts.get(vendor, 0) < int(V.vendor_conf(pol, vendor).get("daily_call_cap", 0))
            and counts.get(f"{vendor}/lane", 0) < int(pol.get("lanes", {}).get("daily_cap", 0)))


def jev_class(spec_dir: str, task: int) -> tuple[str, float, str]:
    try:
        r = subprocess.run([sys.executable, os.path.join(JEV_DIR, "jev.py"), "task-class",
                            "--spec-dir", spec_dir, "--task", str(task)],
                           capture_output=True, text=True, timeout=60)
    except (OSError, subprocess.SubprocessError):
        return "uncertain", 0.0, "none"
    fields = dict(kv.split("=", 1) for kv in r.stdout.split() if "=" in kv)
    try:
        return fields["class"], float(fields["confidence"]), "jev"
    except (KeyError, ValueError):
        return "uncertain", 0.0, "none"


def main(argv=None) -> int:
    p = V.Parser.make("route_lane.py", "choose the lane for one task")
    p.add_argument("--spec-dir", required=True)
    p.add_argument("--task", required=True, type=int)
    p.add_argument("--class", dest="klass")
    p.add_argument("--confidence", type=float, default=1.0)
    p.add_argument("--parallel", action="store_true")
    p.add_argument("--json", action="store_true")
    args = p.parse_args(argv)
    try:
        task = V.parse_task(args.spec_dir, args.task)
    except (OSError, KeyError) as err:
        print(f"route_lane.py: {err}", file=sys.stderr)
        return V.EXIT_USAGE
    try:
        pol = V.load_policy()
        available = {v: lane_available(pol, v) for v in ("codex", "muse")}
    except V.Unavailable:
        pol, available = {"lanes": {}}, {}
    if args.klass:
        klass, conf, source = args.klass, args.confidence, "supplied"
    else:
        klass, conf, source = jev_class(args.spec_dir, args.task)
    lane_name, rule = route(task, klass, conf, available, args.parallel, pol)
    row = {"schema_version": 1, "spec_dir": args.spec_dir, "task": args.task, "lane": lane_name, "rule": rule,
           "class": klass, "confidence": round(conf, 3), "class_source": source,
           "budget": V.budget_word(task["budget"]), "files": task["files"], "available": available}
    if pol.get("lanes_enabled"):
        V.append_row("lane-routing.jsonl", row)
    if args.json:
        print(json.dumps(row, sort_keys=True))
    else:
        print(f"lane={lane_name} rule={rule} class={klass} confidence={conf:.2f}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
