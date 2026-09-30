#!/usr/bin/env python3
"""delivery.py: the state of a /deliver-backlog run, kept in three files in the main
checkout's orchestration/ directory (gitignored; formats in orchestration/README.md):

    INTENT.md     the run: its status, its scope and one row per item with its stage
    RUN-LOG.md    append-only, one timestamped line per event
    QUESTIONS.md  blocking questions for the maintainer, and their answers

    delivery.py init     --scope MH-3,spec:<name>,...
    delivery.py item     <MH-n|spec:<name>> [--stage S] [--spec NAME] [--wave N]
                         [--depends ID,ID] [--note TEXT]
    delivery.py run      --status active|paused|complete
    delivery.py log      TEXT
    delivery.py question --title T --context TEXT --default TEXT [--items ID,ID]
    delivery.py answer   Q-<n> --text TEXT              the maintainer, from a terminal
    delivery.py show     [--json]
    delivery.py actionable                              exit 0 when work can proceed now
    delivery.py reconcile                               recorded stages vs origin/main

Only this script writes the three files, and guard-autonomy.sh blocks agent writes to
them by any other route. That is what makes every RUN-LOG line carry the clock's time
at the moment of writing, never a time an agent estimated, keeps RUN-LOG append-only,
and keeps answers the maintainer's: `answer` refuses without an interactive terminal.

Stages, in lifecycle order: create-spec, refine-spec, spec, approval (the spec waits
for the maintainer's checkpoint), run-spec, finalize-spec, done; and parked (waiting
on a question or an external blocker). An item is actionable when its stage is one an
agent can advance and, for run-spec and finalize-spec, every item it depends on is
done: specifying may run ahead of dependencies, building may not.

Exit codes: 0 success (actionable: work exists), 1 a refusal or nothing actionable,
2 usage error.
"""

import argparse
import json
import os
import re
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import orchlib  # noqa: E402
from orchlib import Refused  # noqa: E402

STAGES = ("create-spec", "refine-spec", "spec", "approval", "run-spec", "finalize-spec", "done", "parked")
AHEAD_OF_DEPS = ("create-spec", "refine-spec", "spec")
AFTER_DEPS = ("run-spec", "finalize-spec")
RUN_STATUSES = ("active", "paused", "complete")
ITEM_RE = re.compile(r"^(MH-[1-9][0-9]*|spec:[a-z0-9][a-z0-9-]*)$")
SPEC_RE = re.compile(r"^[a-z0-9][a-z0-9-]*$")
FIELD = re.compile(r"^- \*\*([^*]+)\*\*: (.*)$")
QHEAD = re.compile(r"^## (Q-([1-9][0-9]*)) — (.*)$")
SPEC_DIR_STAGE = {"unrefined": ("refine-spec",), "refined": ("spec",), "todo": ("approval", "run-spec"),
                  "in-progress": ("run-spec",), "unfinalized": ("finalize-spec",), "done": ("done",),
                  "archived": ("parked",)}
HEADER = "<!-- Written by scripts/orchestration/delivery.py; never edit by hand. Formats: orchestration/README.md -->"


def files(root):
    d = orchlib.orch_dir(root)
    return {n: os.path.join(d, n + ".md") for n in ("INTENT", "RUN-LOG", "QUESTIONS")}


def one_line(text, limit=500):
    return re.sub(r"\s+", " ", text or "").strip()[:limit]


def cell(text):
    return one_line(text, 200).replace("|", "/") or "—"


def read(path):
    try:
        with open(path, encoding="utf-8") as fh:
            return fh.read()
    except OSError:
        return None


# ---- INTENT.md ------------------------------------------------------------------

def parse_intent(text):
    if text is None:
        return None
    run = {"fields": {}, "items": []}
    in_table = False
    for line in text.split("\n"):
        m = FIELD.match(line)
        if m and not in_table:
            run["fields"][m.group(1)] = m.group(2)
            continue
        if line.startswith("| Item |"):
            in_table = True
            continue
        if in_table and line.startswith("|") and not line.startswith("|---"):
            cells = [c.strip() for c in line.strip().strip("|").split("|")]
            if len(cells) != 6:
                raise Refused("INTENT.md has a malformed item row: %r" % line)
            item, spec, stage, wave, deps, note = cells
            run["items"].append({"item": item, "spec": "" if spec == "—" else spec, "stage": stage,
                                 "wave": "" if wave == "—" else wave,
                                 "depends": [] if deps == "—" else [d.strip() for d in deps.split(",") if d.strip()],
                                 "note": "" if note == "—" else note})
    return run


def render_intent(run):
    f = run["fields"]
    lines = ["# Delivery intent", "", HEADER, ""]
    lines += ["- **%s**: %s" % (k, f[k]) for k in ("Run", "Status", "Scope", "Started", "Updated") if k in f]
    lines += ["", "## Items", "", "| Item | Spec | Stage | Wave | Depends on | Note |", "|---|---|---|---|---|---|"]
    for it in run["items"]:
        lines.append("| %s | %s | %s | %s | %s | %s |" % (it["item"], cell(it["spec"]), it["stage"], cell(it["wave"]),
                                                       cell(", ".join(it["depends"])), cell(it["note"])))
    return "\n".join(lines) + "\n"


def load_run(root, required=True):
    run = parse_intent(read(files(root)["INTENT"]))
    if run is None and required:
        raise Refused("no delivery run: start one with `delivery.py init --scope ...`")
    return run


def save_run(root, run):
    run["fields"]["Updated"] = orchlib.iso(orchlib.utc_now())
    orchlib.atomic_write(files(root)["INTENT"], render_intent(run))


# ---- RUN-LOG.md -----------------------------------------------------------------

def append_log(root, text):
    path = files(root)["RUN-LOG"]
    if not os.path.exists(path):
        orchlib.atomic_write(path, "# Run log\n\n%s\n\nOne line per event, oldest first. Append-only.\n\n" % HEADER)
    line = "- %s — %s\n" % (orchlib.iso(orchlib.utc_now()), one_line(text))
    with open(path, "a", encoding="utf-8") as fh:
        fh.write(line)
    return line.strip()


# ---- QUESTIONS.md ---------------------------------------------------------------

def parse_questions(text):
    qs, cur = [], None
    for line in (text or "").split("\n"):
        m = QHEAD.match(line)
        if m:
            cur = {"id": m.group(1), "n": int(m.group(2)), "title": m.group(3), "fields": {}}
            qs.append(cur)
            continue
        f = FIELD.match(line)
        if f and cur:
            cur["fields"][f.group(1)] = f.group(2)
    return qs


def open_questions(root):
    return [q for q in parse_questions(read(files(root)["QUESTIONS"])) if q["fields"].get("Status") == "open"]


# ---- verbs ------------------------------------------------------------------------

def parse_ids(value):
    ids = [t.strip() for t in (value or "").split(",") if t.strip()]
    for t in ids:
        if not ITEM_RE.match(t):
            raise Refused("%r is neither MH-<n> nor spec:<name>" % t)
    return ids


def cmd_init(a, root):
    scope = parse_ids(a.scope)
    if not scope:
        raise Refused("--scope names no item")
    with orchlib.locked(files(root)["INTENT"]):
        cur = load_run(root, required=False)
        if cur and cur["fields"].get("Status") != "complete":
            raise Refused("a run is already %s (INTENT.md); resume it, or mark it complete first"
                          % cur["fields"].get("Status", "open"))
        now = orchlib.iso(orchlib.utc_now())
        run = {"fields": {"Run": now, "Status": "active", "Scope": ", ".join(scope), "Started": now}, "items": []}
        for i in scope:
            spec = i[5:] if i.startswith("spec:") else ""
            run["items"].append({"item": i, "spec": spec, "stage": "refine-spec" if spec else "create-spec",
                                 "wave": "", "depends": [], "note": ""})
        save_run(root, run)
        if not os.path.exists(files(root)["QUESTIONS"]):
            orchlib.atomic_write(files(root)["QUESTIONS"], "# Questions for the maintainer\n\n%s\n\n" % HEADER)
        print(append_log(root, "run %s started: scope %s" % (now, ", ".join(scope))))
    return 0


def cmd_item(a, root):
    if not ITEM_RE.match(a.id):
        raise Refused("%r is neither MH-<n> nor spec:<name>" % a.id)
    if a.stage and a.stage not in STAGES:
        raise Refused("--stage must be one of %s" % ", ".join(STAGES))
    if a.spec and not SPEC_RE.match(a.spec):
        raise Refused("--spec takes a spec directory name")
    with orchlib.locked(files(root)["INTENT"]):
        run = load_run(root)
        it = next((x for x in run["items"] if x["item"] == a.id), None)
        if it is None:
            it = {"item": a.id, "spec": "", "stage": a.stage or "create-spec", "wave": "", "depends": [], "note": ""}
            run["items"].append(it)
        changes = []
        for key, val in (("stage", a.stage), ("spec", a.spec), ("wave", a.wave), ("note", a.note)):
            if val is not None and it[key] != val:
                changes.append("%s %s -> %s" % (key, it[key] or "—", val or "—"))
                it[key] = val
        if a.depends is not None:
            deps = parse_ids(a.depends)
            if it["depends"] != deps:
                changes.append("depends -> %s" % (", ".join(deps) or "—"))
                it["depends"] = deps
        save_run(root, run)
        if changes:
            print(append_log(root, "%s: %s" % (a.id, "; ".join(changes))))
        else:
            print("%s: unchanged" % a.id)
    return 0


def cmd_run(a, root):
    with orchlib.locked(files(root)["INTENT"]):
        run = load_run(root)
        old = run["fields"].get("Status")
        run["fields"]["Status"] = a.status
        save_run(root, run)
        print(append_log(root, "run status %s -> %s" % (old, a.status)))
    return 0


def cmd_log(a, root):
    with orchlib.locked(files(root)["INTENT"]):
        print(append_log(root, a.text))
    return 0


def cmd_question(a, root):
    items = parse_ids(a.items) if a.items else []
    with orchlib.locked(files(root)["INTENT"]):
        path = files(root)["QUESTIONS"]
        text = read(path) or "# Questions for the maintainer\n\n%s\n\n" % HEADER
        n = max([q["n"] for q in parse_questions(text)] + [0]) + 1
        block = "\n".join(["## Q-%d — %s" % (n, one_line(a.title, 120)),
                           "- **Opened**: %s" % orchlib.iso(orchlib.utc_now()),
                           "- **Status**: open",
                           "- **Items**: %s" % (", ".join(items) or "none"),
                           "- **Context**: %s" % one_line(a.context, 1500),
                           "- **Recommended default**: %s" % one_line(a.default, 500)])
        orchlib.atomic_write(path, text.rstrip("\n") + "\n\n" + block + "\n")
        print("Q-%d" % n)
        append_log(root, "question Q-%d filed (%s): %s" % (n, ", ".join(items) or "run", one_line(a.title, 120)))
    return 0


def cmd_answer(a, root):
    if not (sys.stdin.isatty() and sys.stdout.isatty()):
        raise Refused("answers are the maintainer's: run `delivery.py answer` from your own terminal "
                      "(inside Claude Code, `!` opens your shell), or edit QUESTIONS.md yourself")
    with orchlib.locked(files(root)["INTENT"]):
        path = files(root)["QUESTIONS"]
        text = read(path) or ""
        q = next((q for q in parse_questions(text) if q["id"] == a.qid), None)
        if q and q["fields"].get("Status") != "open":
            raise Refused("%s is already %s; file a new question to change the answer"
                          % (a.qid, q["fields"].get("Status", "closed")))
        lines, out, target, done = text.split("\n"), [], None, False
        for line in lines:
            m = QHEAD.match(line)
            if m:
                target = m.group(1) == a.qid
            if target and line == "- **Status**: open":
                line = "- **Status**: answered"
            out.append(line)
            if target and line.startswith("- **Recommended default**:") and not done:
                out += ["- **Answer**: %s" % one_line(a.text, 1500),
                        "- **Answered**: %s" % orchlib.iso(orchlib.utc_now())]
                done = True
        if not done:
            raise Refused("no question %s in QUESTIONS.md" % a.qid)
        orchlib.atomic_write(path, "\n".join(out))
        print(append_log(root, "question %s answered by the maintainer" % a.qid))
    return 0


def done_items(run):
    return {it["item"] for it in run["items"] if it["stage"] == "done"}


def actionable_items(run):
    if run is None or run["fields"].get("Status") != "active":
        return []
    done = done_items(run)
    return [it for it in run["items"]
            if it["stage"] in AHEAD_OF_DEPS or (it["stage"] in AFTER_DEPS and set(it["depends"]) <= done)]


def cmd_show(a, root):
    run = load_run(root, required=False)
    qs = open_questions(root)
    if a.json:
        print(json.dumps({"run": run, "actionable": [i["item"] for i in actionable_items(run)],
                          "open_questions": [q["id"] for q in qs]}, sort_keys=True))
        return 0
    if run is None:
        print("delivery: no run")
        return 0
    f = run["fields"]
    print("delivery: run %s, %s, scope %s, updated %s" % (f.get("Run"), f.get("Status"), f.get("Scope"),
                                                         f.get("Updated")))
    for it in run["items"]:
        print("  %-22s %-14s %s%s" % (it["item"], it["stage"], it["spec"] or "(no spec)",
                                      "  — " + it["note"] if it["note"] else ""))
    act = actionable_items(run)
    print("actionable: %s" % (", ".join(i["item"] for i in act) or "none"))
    print("open questions: %s" % (", ".join("%s (%s)" % (q["id"], q["title"]) for q in qs) or "none"))
    return 0


def cmd_actionable(a, root):
    act = actionable_items(load_run(root, required=False))
    for it in act:
        print("%s %s" % (it["item"], it["stage"]))
    return 0 if act else 1


def cmd_reconcile(a, root):
    run = load_run(root)
    top = orchlib.main_checkout(root)
    listing = orchlib.git_out(top, "ls-tree", "-d", "--name-only", "origin/main", "specs/") or ""
    states = {}
    for state in SPEC_DIR_STAGE:
        for name in (orchlib.git_out(top, "ls-tree", "-d", "--name-only", "origin/main", "specs/%s/" % state)
                     or "").split("\n"):
            if name:
                states.setdefault(os.path.basename(name), []).append(state)
    if not listing:
        print("reconcile: origin/main has no specs/ tree (fetch first?)")
    for it in run["items"]:
        where = states.get(it["spec"], [])
        implied = [st for s in where for st in SPEC_DIR_STAGE[s]] or (["create-spec"] if not it["spec"] else [])
        flag = "" if it["stage"] in implied or it["stage"] == "parked" else "  <- differs"
        print("%s recorded=%s spec=%s origin/main=%s implies=%s%s" % (
            it["item"], it["stage"], it["spec"] or "—", ",".join(where) or "—", "|".join(implied) or "missing", flag))
    return 0


def main(argv):
    p = argparse.ArgumentParser(prog="delivery.py", description="State of a /deliver-backlog run.")
    sub = p.add_subparsers(dest="cmd", required=True)
    i = sub.add_parser("init")
    i.add_argument("--scope", required=True)
    it = sub.add_parser("item")
    it.add_argument("id")
    it.add_argument("--stage")
    it.add_argument("--spec")
    it.add_argument("--wave")
    it.add_argument("--depends")
    it.add_argument("--note")
    r = sub.add_parser("run")
    r.add_argument("--status", required=True, choices=RUN_STATUSES)
    lg = sub.add_parser("log")
    lg.add_argument("text")
    q = sub.add_parser("question")
    q.add_argument("--title", required=True)
    q.add_argument("--context", required=True)
    q.add_argument("--default", required=True)
    q.add_argument("--items")
    an = sub.add_parser("answer")
    an.add_argument("qid")
    an.add_argument("--text", required=True)
    s = sub.add_parser("show")
    s.add_argument("--json", action="store_true")
    sub.add_parser("actionable")
    sub.add_parser("reconcile")
    a = p.parse_args(argv)
    verbs = {"init": cmd_init, "item": cmd_item, "run": cmd_run, "log": cmd_log, "question": cmd_question,
             "answer": cmd_answer, "show": cmd_show, "actionable": cmd_actionable, "reconcile": cmd_reconcile}
    try:
        root = os.getcwd()
        orchlib.main_checkout(root)
        return verbs[a.cmd](a, root)
    except Refused as e:
        print("delivery.py %s: refused: %s" % (a.cmd, e), file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
