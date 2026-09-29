#!/usr/bin/env python3
"""jev.py: the harness's only path to the Jev classifier. Each subcommand is one
advisory site that asks closed questions from questions.py.

    jev.py triage-findings ANSWER.json --root DIR [--json]
        For each finding of a consult answer (findings schema), is the cited code
        really showing the claimed defect? Orders the adjudication; decides nothing.
    jev.py private-material FILE [--json]
        Does a paragraph of a PR description, commit message or issue text carry
        private material? Complements scripts/ci/check-public-hygiene.sh, which
        catches only the mechanical cases.
    jev.py scope-drift --spec-dir DIR --task N --base REF [--root DIR] [--json]
        Which changed files lie outside the task's Files list (a fact, printed
        always), and is each of those changes needed for the task (Jev)?
    jev.py task-class --spec-dir DIR --task N
        One line `class=<c> confidence=<p>` for route_lane.py, or nothing.
    jev.py adjudicate --site S --ref REF --verdict confirmed|rejected [--p P]
    jev.py precision [--json]
        Record how a flagged item turned out, and report precision per site and
        question version (no network).

Every site prints `question_version=<v>` first (task-class excepted). Fail open:
when Jev is not opted in, has no key, is over its cap or does not answer, the site
prints `jev: skipped (<reason>)` on stderr and makes no judgement. `uncertain`
and `review` are real answers to surface, never rounded. Exit 0 on every path, 64
on a usage error. Never call this from a guard hook: a blocking hook must not wait
on the network or allow on a model's say-so.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
sys.path.insert(0, os.path.normpath(os.path.join(HERE, "..", "vendors")))
import questions as Q  # noqa: E402
import vendors as V  # noqa: E402
from jev_client import JevClient, skip  # noqa: E402

ADJ_LOG = "jev-adjudications.jsonl"


def band(p: float) -> str:
    if p >= Q.NOUL_YES_MIN:
        return "yes"
    if p >= Q.REVIEW_BAND_MIN:
        return "review"
    return "no"


def noul(ans) -> float:
    try:
        return float(ans.get("noul", 0.0))
    except (TypeError, ValueError, AttributeError):
        return 0.0


def client(site: str):
    try:
        return JevClient(site)
    except V.Unavailable as err:
        skip(f"{err.reason}: {err.detail}" if err.detail else err.reason)
        return None


def ask(c, state, qs):
    try:
        return c.ask(state, qs)
    except V.Unavailable as err:
        skip(f"{err.reason}: {err.detail}" if err.detail else err.reason)
        return None


def out(args, site, lines, machine):
    version = Q.QUESTION_VERSIONS[site]
    if args.json:
        print(json.dumps({"question_version": version, **machine}, sort_keys=True))
    else:
        print(f"question_version={version}")
        for line in lines:
            print(line)


def snippet(root: str, rel: str, line) -> str | None:
    path = os.path.realpath(os.path.join(root, rel))
    if not path.startswith(os.path.realpath(root) + os.sep) or not os.path.isfile(path):
        return None
    with open(path, encoding="utf-8", errors="replace") as fh:
        lines = fh.read().split("\n")
    if not isinstance(line, int) or line < 1 or line > len(lines):
        return None
    lo, hi = max(line - Q.SNIPPET_CONTEXT_LINES, 1), min(line + Q.SNIPPET_CONTEXT_LINES, len(lines))
    return "\n".join(f"{n}: {lines[n - 1]}" for n in range(lo, hi + 1))


def cmd_triage(args) -> int:
    with open(args.answer, encoding="utf-8") as fh:
        findings = json.load(fh).get("findings") or []
    anchored, lines, rows = {}, [], []
    for i, f in enumerate(findings):
        code = snippet(args.root, str(f.get("file", "")), f.get("line"))
        if code is None:
            rows.append({"index": i, "file": f.get("file"), "line": f.get("line"), "band": "unanchored", "p": None})
            continue
        anchored[f"f{i}"] = (i, f, code)
    c = client("triage-findings") if anchored else None
    answers = ask(c, {"repository": "MYTHHELM"}, {k: Q.finding_is_real(f.get("claim", ""), code)
                                                  for k, (_i, f, code) in anchored.items()}) if c else None
    if answers is None and anchored:
        return 0
    for k, (i, f, _code) in anchored.items():
        p = noul(answers.get(k))
        rows.append({"index": i, "file": f.get("file"), "line": f.get("line"), "band": band(p), "p": round(p, 2)})
    rows.sort(key=lambda r: (r["p"] is None, -(r["p"] or 0)))
    for r in rows:
        shown = "   -" if r["p"] is None else f"{r['p']:.2f}"
        lines.append(f"[{r['band']} {shown}] finding {r['index']} {r['file']}:{r['line']}")
    out(args, "triage-findings", lines, {"findings": rows})
    return 0


def cmd_private(args) -> int:
    with open(args.file, encoding="utf-8", errors="replace") as fh:
        paras = [p.strip() for p in re.split(r"\n\s*\n", fh.read()) if p.strip()][:60]
    if not paras:
        out(args, "private-material", [], {"flagged": []})
        return 0
    c = client("private-material")
    answers = ask(c, {"kind": "public text"}, {f"p{i}": Q.paragraph_is_private(t) for i, t in enumerate(paras)}) if c else None
    if answers is None:
        return 0
    flagged = []
    for i, t in enumerate(paras):
        p = noul(answers.get(f"p{i}"))
        if band(p) != "no":
            flagged.append({"paragraph": i + 1, "band": band(p), "p": round(p, 2), "excerpt": t[:120]})
    out(args, "private-material", [f"[{f['band']} {f['p']:.2f}] paragraph {f['paragraph']}: {f['excerpt']}" for f in flagged],
        {"flagged": flagged})
    return 0


def cmd_scope(args) -> int:
    task = V.parse_task(args.spec_dir, args.task)
    root = args.root or os.getcwd()
    r = subprocess.run(["git", "-C", root, "diff", "--name-only", "-z", "--no-renames", f"{args.base}...HEAD"],
                       capture_output=True, text=True, timeout=60)
    if r.returncode != 0:
        print(f"jev.py: cannot diff {args.base}...HEAD in {root}", file=sys.stderr)
        return V.EXIT_USAGE
    outside = [p for p in r.stdout.split("\0") if p and not V.path_in(p, task["files"])]
    rows = [{"path": p, "band": None, "p": None} for p in outside]
    answers = None
    if outside:
        c = client("scope-drift")
        qs = {}
        for i, p in enumerate(outside):
            diff = subprocess.run(["git", "-C", root, "diff", f"{args.base}...HEAD", "--", p],
                                  capture_output=True, text=True, timeout=60).stdout[:6000]
            qs[f"s{i}"] = Q.change_is_needed(task["text"][:8000], p, V.redact(diff)[0])
        answers = ask(c, {"repository": "MYTHHELM"}, qs) if c else None
    for i, row in enumerate(rows):
        if answers is not None:
            p = noul(answers.get(f"s{i}"))
            row.update(p=round(p, 2), band=band(p))
    lines = [f"outside-files: {r['path']}" + (f" needed=[{r['band']} {r['p']:.2f}]" if r["p"] is not None else "") for r in rows]
    out(args, "scope-drift", lines, {"outside": rows})
    return 0


def cmd_task_class(args) -> int:
    task = V.parse_task(args.spec_dir, args.task)
    c = client("task-class")
    answers = ask(c, {"task": task["text"][:12000], "files": task["files"], "budget": task["budget"]},
                  {"c": Q.task_class()}) if c else None
    if answers is None:
        return 0
    ans = answers.get("c") or {}
    try:
        conf = float(ans.get("confidence", 0.0))
    except (TypeError, ValueError):
        conf = 0.0
    klass = ans.get("choice") if conf >= Q.CLASS_CONFIDENCE_MIN and ans.get("choice") in Q.TASK_CLASSES else "uncertain"
    print(f"class={klass} confidence={conf:.2f}")
    return 0


def cmd_adjudicate(args) -> int:
    if args.site not in Q.QUESTION_VERSIONS:
        print(f"jev.py: unknown site {args.site!r}", file=sys.stderr)
        return V.EXIT_USAGE
    V.append_row(ADJ_LOG, {"schema_version": 1, "site": args.site, "question_version": Q.QUESTION_VERSIONS[args.site],
                           "ref": args.ref, "verdict": args.verdict, "p": args.p})
    print(f"recorded={args.site}:{args.ref}")
    return 0


def cmd_precision(args) -> int:
    groups: dict = {}
    for r in V.read_rows(ADJ_LOG):
        g = groups.setdefault((r.get("site"), r.get("question_version")), {"confirmed": 0, "rejected": 0})
        if r.get("verdict") in g:
            g[r["verdict"]] += 1
    rows = []
    for (site, ver), g in sorted(groups.items(), key=lambda kv: (str(kv[0][0]), str(kv[0][1]))):
        n = g["confirmed"] + g["rejected"]
        rows.append({"site": site, "question_version": ver, **g, "precision": round(g["confirmed"] / n, 2) if n else None})
    if args.json:
        print(json.dumps(rows, sort_keys=True))
    else:
        for r in rows:
            print(f"precision: {r['site']} v{r['question_version']} confirmed={r['confirmed']} rejected={r['rejected']} "
                  f"precision={r['precision'] if r['precision'] is not None else 'n/a'}")
    return 0


def main(argv=None) -> int:
    p = V.Parser.make("jev.py", "advisory Jev classifier sites")
    p.add_argument("--json", action="store_true")
    sub = p.add_subparsers(dest="cmd", required=True)
    s = sub.add_parser("triage-findings")
    s.add_argument("answer")
    s.add_argument("--root", required=True)
    s.set_defaults(fn=cmd_triage)
    s = sub.add_parser("private-material")
    s.add_argument("file")
    s.set_defaults(fn=cmd_private)
    s = sub.add_parser("scope-drift")
    s.add_argument("--spec-dir", required=True)
    s.add_argument("--task", required=True, type=int)
    s.add_argument("--base", required=True)
    s.add_argument("--root")
    s.set_defaults(fn=cmd_scope)
    s = sub.add_parser("task-class")
    s.add_argument("--spec-dir", required=True)
    s.add_argument("--task", required=True, type=int)
    s.set_defaults(fn=cmd_task_class)
    s = sub.add_parser("adjudicate")
    s.add_argument("--site", required=True)
    s.add_argument("--ref", required=True)
    s.add_argument("--verdict", required=True, choices=["confirmed", "rejected"])
    s.add_argument("--p", type=float)
    s.set_defaults(fn=cmd_adjudicate)
    s = sub.add_parser("precision")
    s.set_defaults(fn=cmd_precision)
    for sp in sub.choices.values():
        sp.add_argument("--json", action="store_true", default=argparse.SUPPRESS)
    args = p.parse_args(argv)
    try:
        return args.fn(args)
    except (OSError, KeyError, ValueError) as err:
        print(f"jev.py: {err}", file=sys.stderr)
        return V.EXIT_USAGE
    except V.Unavailable as err:
        skip(err.reason)
        return 0


if __name__ == "__main__":
    sys.exit(main())
