#!/usr/bin/env python3
"""pm.py: parser, validator and proposal writer for the product layer (product/).

    pm.py [--root DIR] [--product-dir DIR] validate
    pm.py [--root DIR] list [--status S[,S...]] [--json]
    pm.py [--root DIR] show MH-<n>
    pm.py [--root DIR] next
    pm.py [--root DIR] next-id MH|D|S
    pm.py [--root DIR] add --title T --stage S --gates G --value N --risk N
                           --effort N --source TEXT --summary TEXT
                           [--status idea|triaged] [--issue N] [--id MH-<n>] --out FILE
    pm.py [--root DIR] set-status MH-<n> STATUS [--spec NAME[,NAME]] --out FILE
    pm.py [--root DIR] set MH-<n> FIELD VALUE --out FILE
    pm.py [--root DIR] rescore [MH-<n> [--value N] [--risk N] [--effort N]
                               [--stage S]] --out FILE
    pm.py [--root DIR] decide --type T --title T --decision TEXT --rationale TEXT
                              [--cards MH-1,MH-2] [--evidence TEXT] [--id D-<n>]
                              [--date YYYY-MM-DD] --out FILE
    pm.py [--root DIR] signal --title T --sources TEXT --count N --summary TEXT
                              [--cards MH-1] --action TEXT [--id S-<n>] --out FILE
    pm.py [--root DIR] roadmap --out FILE
    pm.py [--root DIR] fmt --out FILE
    pm.py [--root DIR] draft-text objectives|readme|backlog-preamble --from FILE --out FILE
    pm.py [--root DIR] stage DIR

A change touching several files (a status flip, its decision entry and the redrafted
roadmap) is drafted in a staging directory: `stage DIR` copies the product
files there, and each later call reads them with --product-dir DIR and writes back
with --out DIR/<file>. Each staged file that changed is then filed as one request.

Every write verb drafts: it writes the COMPLETE proposed file to --out, which must
lie outside <root>/product/, and never touches a product file. A product change
lands only when a maintainer approves it (scripts/orchestration/approve.sh). The
formats are documented in product/README.md; `validate`, and the `product` check of
scripts/ci/lint-agent-harness.sh, which imports this module, enforce them:

  backlog.md    canonical form (open cards under "## Open" by score descending then
                id, closed cards under "## Closed" by id, fields in schema order);
                unique MH ids; the closed field vocabulary FIELDS; a status from
                STATUSES; a score equal to the formula, with the urgency its stage
                earns at the current stage; spec names that each resolve to exactly
                one specs/<state>/<name>/; a shipped card's specs all in done/ or
                archived/; issue links of the form [#n](https://github.com/o/r/issues/n)
  decisions.md  D ids strictly increasing in file order and dates never decreasing
                (append-only), a type from DECISION_TYPES, cards that exist
  signals.md    S ids strictly increasing, required fields, GitHub source links
  objectives.md a "> Current stage: N" line
  roadmap.md    every MH id it names exists; a stale roadmap is a warning

next-id reserves an id under a lock in <main checkout>/.claude/data/
id-reservations.jsonl: one above the highest id in the working tree, local main,
origin/main, every pending approval request and every earlier reservation, so
concurrent sessions and branches never draft the same id and holes are never reused.

--product-dir reads the product files from another directory, such as a draft set
staged for review, while spec names still resolve under <root>/specs/.

Exit codes: 0 success, 1 validation findings or a refused change, 2 a usage error or
unreadable input. Standard library only; git is called read-only (show, rev-parse).
"""

import argparse
import datetime
import decimal
import fcntl
import json
import os
import re
import subprocess
import sys

STATUSES = ("idea", "triaged", "specced", "implementing", "shipped", "dropped")
OPEN_STATUSES = ("idea", "triaged", "specced", "implementing")
STAGES = tuple(str(n) for n in range(7)) + ("later",)
GATES = tuple("G%02d" % n for n in range(1, 17))
REQUIRED_FIELDS = ("Status", "Stage", "Gates", "Score", "Spec", "Issue", "Source", "Summary")
OPTIONAL_FIELDS = ("Premise-grounded", "Half shipped", "Notes")
FIELDS = REQUIRED_FIELDS + OPTIONAL_FIELDS
SETTABLE = {"issue": "Issue", "spec": "Spec", "source": "Source", "summary": "Summary",
            "gates": "Gates", "title": None, "premise-grounded": "Premise-grounded",
            "half-shipped": "Half shipped", "notes": "Notes"}
DECISION_TYPES = ("backlog-seed", "card-add", "card-reject", "card-drop", "rescore",
                  "lifecycle-sync", "objective-change", "impact-review", "signal-triage",
                  "strategic-adjustment")
DECISION_FIELDS = ("Type", "Decision", "Rationale", "Cards")
SIGNAL_FIELDS = ("Sources", "Count", "Summary", "Cards", "Suggested action")
SPEC_STATES = ("unrefined", "refined", "todo", "in-progress", "unfinalized", "done", "archived")
SHIPPED_SPEC_STATES = ("done", "archived")
OWNER_REPO = "turbokast/mythhelm"

CARD_HEAD = re.compile(r"^### MH-([1-9][0-9]*): (\S.*)$")
DECISION_HEAD = re.compile(r"^### D-([1-9][0-9]*) — (\d{4}-\d{2}-\d{2}): (\S.*)$")
SIGNAL_HEAD = re.compile(r"^### S-([1-9][0-9]*) — (\d{4}-\d{2}-\d{2}): (\S.*)$")
FIELD_LINE = re.compile(r"^- \*\*([^*]+)\*\*: (.*)$")
SCORE = re.compile(r"^(\d+\.\d) = \(value ([1-5]) \+ urgency ([1-5]) \+ risk ([1-5])\) / effort ([1-5])$")
ISSUE = re.compile(r"^\[#([1-9][0-9]*)\]\(https://github\.com/([A-Za-z0-9-]+/[A-Za-z0-9._-]+)/issues/([1-9][0-9]*)\)$")
SPEC_NAME = re.compile(r"^`([a-z0-9][a-z0-9-]*)`$")
CARD_ID = re.compile(r"^MH-([1-9][0-9]*)$")
CURRENT_STAGE = re.compile(r"^> Current stage: (\S+)[ \t]*$", re.M)
GH_LINK = re.compile(r"https://github\.com/[A-Za-z0-9-]+/[A-Za-z0-9._-]+/(issues|discussions|pull)/[1-9][0-9]*")
EMPTY_OPEN = "No open cards."
EMPTY_CLOSED = "No closed cards."
EMPTY_LOG = "No decisions recorded yet."
EMPTY_SIGNALS = "No signals recorded yet."

BACKLOG = "product/backlog.md"
DECISIONS = "product/decisions.md"
SIGNALS = "product/signals.md"
OBJECTIVES = "product/objectives.md"
ROADMAP = "product/roadmap.md"
README = "product/README.md"


class InputError(Exception):
    """An input is missing, unreadable or refused; the command exits 2."""


class Findings:
    def __init__(self):
        self.errors = []
        self.warnings = []

    def err(self, path, line, detail):
        self.errors.append("%s:%s: product: %s" % (path, line, detail))

    def warn(self, path, line, detail):
        self.warnings.append("%s:%s: product (warning): %s" % (path, line, detail))


# ---- scoring ---------------------------------------------------------------

def urgency_for(stage, current):
    """5 for the current or an earlier stage, 3 for the next, 2 for the one after,
    1 beyond that or for 'later'."""
    if stage == "later":
        return 1
    return {0: 5, 1: 3, 2: 2}.get(max(int(stage) - current, 0), 1)


def score_of(value, urgency, risk, effort):
    """(value + urgency + risk) / effort, rounded half up to one decimal."""
    q = decimal.Decimal(value + urgency + risk) / decimal.Decimal(effort)
    return str(q.quantize(decimal.Decimal("0.1"), rounding=decimal.ROUND_HALF_UP))


def score_line(value, urgency, risk, effort):
    return "%s = (value %d + urgency %d + risk %d) / effort %d" % (
        score_of(value, urgency, risk, effort), value, urgency, risk, effort)


# ---- parsing ---------------------------------------------------------------

PRODUCT_DIR = None   # --product-dir: read the product files from here instead of <root>/product


def read_text(root, rel):
    path = os.path.join(root, rel)
    if PRODUCT_DIR and rel.startswith("product/"):
        path = os.path.join(PRODUCT_DIR, rel[len("product/"):])
    try:
        with open(path, encoding="utf-8") as f:
            return f.read()
    except OSError as e:
        raise InputError("cannot read %s: %s" % (rel, e.strerror or e)) from e


def stage_from_text(text):
    matches = CURRENT_STAGE.findall(text)
    if len(matches) != 1 or matches[0] not in STAGES[:-1]:
        raise InputError("%s requires exactly one '> Current stage: N' line, with N from 0 to 6" % OBJECTIVES)
    return int(matches[0])


def current_stage(root):
    return stage_from_text(read_text(root, OBJECTIVES))


def split_sections(text, path, headings, findings):
    """(preamble lines, {heading: [(line_no, line)]}). The preamble runs to the first
    named level-2 heading outside a code fence and may hold any text; from there on
    only the named sections may appear, each once."""
    pre, sections, cur, fence = [], {}, None, False
    for no, line in enumerate(text.split("\n"), 1):
        if line.startswith("```"):
            fence = not fence
        if not fence and line.startswith("## "):
            name = line[3:].strip()
            if name not in headings and not sections:
                pre.append(line)
                continue
            if name not in headings:
                findings.err(path, no, "unexpected section %r (sections: %s)" % (name, ", ".join(headings)))
                cur = None
                continue
            if name in sections:
                findings.err(path, no, "section %r appears twice" % name)
            cur = name
            sections[name] = []
            continue
        if cur is not None:
            sections[cur].append((no, line))
        elif not sections:
            pre.append(line)
    for h in headings:
        if h not in sections:
            findings.err(path, 0, "missing section '## %s'" % h)
    return pre, sections


def parse_entries(body, path, head_re, empty_line, findings, kind):
    """[(heading match, line_no, [(field, value, line_no)])] for one section."""
    entries, empty_seen = [], False
    for no, line in body:
        if not line.strip():
            continue
        m = head_re.match(line)
        if m:
            entries.append((m, no, []))
            continue
        if line.startswith("### "):
            findings.err(path, no, "malformed %s heading: %r" % (kind, line[:80]))
            entries.append((None, no, []))
            continue
        f = FIELD_LINE.match(line)
        if f and entries:
            entries[-1][2].append((f.group(1), f.group(2).strip(), no))
            continue
        if line.strip() == empty_line and not entries:
            empty_seen = True
            continue
        findings.err(path, no, "unexpected line (a %s holds only '- **Field**: value' lines): %r" % (kind, line[:80]))
    if empty_seen and entries:
        findings.err(path, 0, "the %r placeholder is left beside %s entries" % (empty_line, kind))
    return entries


class Card:
    def __init__(self, num, title, line):
        self.num, self.title, self.line = num, title, line
        self.fields = []          # [(name, value, line)] in file order

    @property
    def id(self):
        return "MH-%d" % self.num

    def get(self, name):
        return next((v for n, v, _ in self.fields if n == name), None)

    def line_of(self, name):
        return next((ln for n, _, ln in self.fields if n == name), self.line)

    def set(self, name, value):
        for i, (n, _, ln) in enumerate(self.fields):
            if n == name:
                self.fields[i] = (n, value, ln)
                return
        self.fields.append((name, value, 0))

    def score(self):
        m = SCORE.match(self.get("Score") or "")
        return decimal.Decimal(m.group(1)) if m else decimal.Decimal(-1)

    def is_open(self):
        return self.get("Status") in OPEN_STATUSES

    def render(self):
        order = {n: i for i, n in enumerate(FIELDS)}
        fields = sorted(self.fields, key=lambda f: order.get(f[0], len(FIELDS)))
        return "\n".join(["### %s: %s" % (self.id, self.title)] + ["- **%s**: %s" % (n, v) for n, v, _ in fields])


class Backlog:
    def __init__(self, text, findings):
        self.text = text
        pre, sections = split_sections(text, BACKLOG, ("Open", "Closed"), findings)
        self.preamble = pre
        self.cards, self.section_of = [], {}
        for sec, empty in (("Open", EMPTY_OPEN), ("Closed", EMPTY_CLOSED)):
            for m, no, fields in parse_entries(sections.get(sec, []), BACKLOG, CARD_HEAD, empty, findings, "card"):
                if m:
                    card = Card(int(m.group(1)), m.group(2).strip(), no)
                    card.fields = fields
                    self.cards.append(card)
                    self.section_of.setdefault(card.num, sec)

    def by_id(self, ident):
        m = CARD_ID.match(ident or "")
        if not m:
            raise InputError("not a card id: %r (expected MH-<n>)" % ident)
        for c in self.cards:
            if c.num == int(m.group(1)):
                return c
        raise InputError("no card %s in %s" % (ident, BACKLOG))

    def render(self):
        pre = list(self.preamble)
        while pre and not pre[-1].strip():
            pre.pop()
        open_cards = sorted((c for c in self.cards if c.is_open()), key=lambda c: (-c.score(), c.num))
        closed = sorted((c for c in self.cards if not c.is_open()), key=lambda c: c.num)
        return ("\n".join(pre) + "\n\n## Open\n\n"
                + ("\n\n".join(c.render() for c in open_cards) if open_cards else EMPTY_OPEN)
                + "\n\n## Closed\n\n"
                + ("\n\n".join(c.render() for c in closed) if closed else EMPTY_CLOSED) + "\n")


def spec_homes(root):
    """{spec name: [states]} over the specs/<state>/<name>/ directories."""
    homes = {}
    for state in SPEC_STATES:
        d = os.path.join(root, "specs", state)
        if os.path.isdir(d):
            for name in sorted(os.listdir(d)):
                if os.path.isdir(os.path.join(d, name)):
                    homes.setdefault(name, []).append(state)
    return homes


def parse_spec_field(value):
    """[names] for '(none)' or backticked names separated by commas; None if malformed."""
    if value == "(none)":
        return []
    names = []
    for part in value.split(","):
        m = SPEC_NAME.match(part.strip())
        if not m:
            return None
        names.append(m.group(1))
    return names


def parse_gates(value):
    if value == "none":
        return []
    gates = [g.strip() for g in value.split(",")]
    ok = all(g in GATES for g in gates) and gates == sorted(set(gates))
    return gates if ok else None


# ---- validation ------------------------------------------------------------

def check_card(c, bl, homes, stage, findings):
    p = BACKLOG
    names = [n for n, _, _ in c.fields]
    for n, v, ln in c.fields:
        if n not in FIELDS:
            findings.err(p, ln, "%s: unknown field %r (fields: %s)" % (c.id, n, ", ".join(FIELDS)))
        elif names.count(n) > 1:
            findings.err(p, ln, "%s: field %r appears twice" % (c.id, n))
        elif not v:
            findings.err(p, ln, "%s: %s is empty" % (c.id, n))
    for n in REQUIRED_FIELDS:
        if n not in names:
            findings.err(p, c.line, "%s: missing field %r" % (c.id, n))
    status, stage_v = c.get("Status"), c.get("Stage")
    if status is not None and status not in STATUSES:
        findings.err(p, c.line_of("Status"), "%s: status %r is not one of %s" % (c.id, status, ", ".join(STATUSES)))
    elif status is not None:
        want = "Open" if c.is_open() else "Closed"
        if bl.section_of.get(c.num) != want:
            findings.err(p, c.line, "%s: a %s card belongs under '## %s'" % (c.id, status, want))
    if stage_v is not None and stage_v not in STAGES:
        findings.err(p, c.line_of("Stage"), "%s: stage %r is not one of %s" % (c.id, stage_v, ", ".join(STAGES)))
    gates = c.get("Gates")
    if gates is not None and parse_gates(gates) is None:
        findings.err(p, c.line_of("Gates"), "%s: gates must be 'none' or G01-G16 in ascending order, comma-separated; got %r" % (c.id, gates))
    sc = c.get("Score")
    if sc is not None:
        m = SCORE.match(sc)
        if not m:
            findings.err(p, c.line_of("Score"), "%s: score must read '<n.n> = (value V + urgency U + risk R) / effort E' with each input 1-5; got %r" % (c.id, sc))
        else:
            v, u, r, e = (int(m.group(i)) for i in (2, 3, 4, 5))
            if m.group(1) != score_of(v, u, r, e):
                findings.err(p, c.line_of("Score"), "%s: score %s does not equal (%d + %d + %d) / %d = %s" % (c.id, m.group(1), v, u, r, e, score_of(v, u, r, e)))
            if stage is not None and stage_v in STAGES and u != urgency_for(stage_v, stage):
                findings.err(p, c.line_of("Score"), "%s: urgency %d does not match stage %s at current stage %d (expected %d; draft the change with pm.py rescore)" % (c.id, u, stage_v, stage, urgency_for(stage_v, stage)))
    spec = c.get("Spec")
    if spec is not None:
        specs = parse_spec_field(spec)
        if specs is None:
            findings.err(p, c.line_of("Spec"), "%s: spec must be '(none)' or backticked spec names; got %r" % (c.id, spec))
            specs = []
        for s in specs:
            states = homes.get(s, [])
            if len(states) != 1:
                findings.err(p, c.line_of("Spec"), "%s: spec %r resolves to %d spec directories; it needs exactly one specs/<state>/%s/" % (c.id, s, len(states), s))
            elif status == "shipped" and states[0] not in SHIPPED_SPEC_STATES:
                findings.err(p, c.line_of("Status"), "%s: shipped, but spec %r is in %s/; a card ships when every spec it names is in done/ or archived/ (keep it implementing and record 'Half shipped')" % (c.id, s, states[0]))
        if status in ("specced", "implementing") and not specs:
            findings.err(p, c.line_of("Spec"), "%s: a %s card names its spec" % (c.id, status))
        if status in ("idea", "triaged") and specs:
            findings.err(p, c.line_of("Status"), "%s: a card with a spec is specced or later, not %s" % (c.id, status))
    issue = c.get("Issue")
    if issue is not None and issue != "(none)":
        m = ISSUE.match(issue)
        if not m:
            findings.err(p, c.line_of("Issue"), "%s: issue must be '(none)' or '[#n](https://github.com/<owner>/<repo>/issues/n)'; got %r" % (c.id, issue))
        elif m.group(1) != m.group(3):
            findings.err(p, c.line_of("Issue"), "%s: the link text #%s and the URL's issue %s differ" % (c.id, m.group(1), m.group(3)))
        else:
            return m.group(2)
    return None


def validate_backlog(root, findings, stage, text=None):
    try:
        text = read_text(root, BACKLOG) if text is None else text
    except InputError as e:
        findings.err(BACKLOG, 0, str(e))
        return None
    before = len(findings.errors)
    bl = Backlog(text, findings)
    homes = spec_homes(root)
    seen, repos = {}, set()
    for c in bl.cards:
        if c.num in seen:
            findings.err(BACKLOG, c.line, "%s is defined twice (first at line %d); card ids are unique" % (c.id, seen[c.num]))
        seen.setdefault(c.num, c.line)
        repo = check_card(c, bl, homes, stage, findings)
        if repo:
            repos.add(repo)
    if len(repos) > 1:
        findings.err(BACKLOG, 0, "issue links point at more than one repository: %s" % ", ".join(sorted(repos)))
    if len(findings.errors) == before and bl.render() != text:
        findings.err(BACKLOG, 0, "not in canonical form: open cards by score then id, closed cards by id, fields in schema order, one blank line between cards (redraft it with pm.py fmt)")
    return bl


class Log:
    """decisions.md or signals.md: numbered entries appended under one section."""

    def __init__(self, text, path, section, head_re, empty, kind, findings):
        self.text, self.empty = text, empty
        pre, sections = split_sections(text, path, (section,), findings)
        self.entries = [e for e in parse_entries(sections.get(section, []), path, head_re, empty, findings, kind) if e[0]]

    def max_id(self):
        return max((int(m.group(1)) for m, _, _ in self.entries), default=0)

    def append(self, block):
        lines = self.text.rstrip("\n").split("\n")
        if not self.entries and lines and lines[-1].strip() == self.empty:
            lines.pop()
        return "\n".join(lines).rstrip("\n") + "\n\n" + block + "\n"


LOGS = {
    DECISIONS: ("Log", DECISION_HEAD, EMPTY_LOG, "decision", DECISION_FIELDS, "D"),
    SIGNALS: ("Signals", SIGNAL_HEAD, EMPTY_SIGNALS, "signal", SIGNAL_FIELDS, "S"),
}


def load_log(root, path, findings):
    section, head, empty, kind, _, _ = LOGS[path]
    return Log(read_text(root, path), path, section, head, empty, kind, findings)


def validate_log(root, path, findings, cards):
    _, _, _, _, required, prefix = LOGS[path]
    try:
        log = load_log(root, path, findings)
    except InputError as e:
        findings.err(path, 0, str(e))
        return
    prev_id, prev_date = 0, ""
    for m, no, fields in log.entries:
        n, date = int(m.group(1)), m.group(2)
        tag = "%s-%d" % (prefix, n)
        if n <= prev_id:
            findings.err(path, no, "%s follows %s-%d; entries are appended with increasing ids" % (tag, prefix, prev_id))
        try:
            datetime.date.fromisoformat(date)
        except ValueError:
            findings.err(path, no, "%s: %r is not a date" % (tag, date))
        if date < prev_date:
            findings.err(path, no, "%s is dated %s, before the entry above it (%s); entries are appended in order" % (tag, date, prev_date))
        prev_id, prev_date = max(prev_id, n), max(prev_date, date)
        names = [f for f, _, _ in fields]
        for f in required:
            if f not in names:
                findings.err(path, no, "%s: missing field %r" % (tag, f))
        for f, v, ln in fields:
            if f not in required + ("Evidence",) or names.count(f) > 1:
                findings.err(path, ln, "%s: unknown or repeated field %r" % (tag, f))
            elif not v:
                findings.err(path, ln, "%s: %s is empty" % (tag, f))
            elif f == "Type" and v not in DECISION_TYPES:
                findings.err(path, ln, "%s: type %r is not one of %s" % (tag, v, ", ".join(DECISION_TYPES)))
            elif f == "Cards" and v != "none":
                for ref in (x.strip() for x in v.split(",")):
                    if not CARD_ID.match(ref):
                        findings.err(path, ln, "%s: %r is not a card id" % (tag, ref))
                    elif cards is not None and ref not in cards:
                        findings.err(path, ln, "%s: card %s is not in %s" % (tag, ref, BACKLOG))
            elif f == "Sources" and not GH_LINK.search(v):
                findings.err(path, ln, "%s: sources must link GitHub issues, discussions or pull requests" % tag)
            elif f == "Count" and not re.match(r"^[1-9][0-9]*$", v):
                findings.err(path, ln, "%s: count must be a positive integer" % tag)


def validate(root):
    findings = Findings()
    stage = None
    try:
        stage = current_stage(root)
    except InputError as e:
        findings.err(OBJECTIVES, 0, str(e))
    bl = validate_backlog(root, findings, stage)
    cards = {c.id for c in bl.cards} if bl else None
    for path in (DECISIONS, SIGNALS):
        validate_log(root, path, findings, cards)
    try:
        roadmap = read_text(root, ROADMAP)
    except InputError as e:
        findings.err(ROADMAP, 0, str(e))
        return findings
    for no, line in enumerate(roadmap.split("\n"), 1):
        for ref in re.findall(r"\bMH-[1-9][0-9]*\b", line):
            if cards is not None and ref not in cards:
                findings.err(ROADMAP, no, "card %s is not in %s" % (ref, BACKLOG))
    if bl and stage is not None and not findings.errors and roadmap != render_roadmap(bl, stage):
        findings.warn(ROADMAP, 0, "stale: it differs from what pm.py roadmap generates from the backlog")
    return findings


# ---- views -----------------------------------------------------------------

def ordered(bl):
    return sorted(bl.cards, key=lambda c: (not c.is_open(), -c.score(), c.num))


def next_card(bl, stage):
    """The highest-scored idea or triaged card in the current or next stage."""
    for c in ordered(bl):
        if c.get("Status") in ("triaged", "idea") and urgency_for(c.get("Stage"), stage) >= 3:
            return c
    return None


def roadmap_line(c):
    parts = ["%s, stage %s, score %s" % (c.get("Status"), c.get("Stage"), c.score())]
    if c.get("Spec") not in (None, "(none)"):
        parts.append("spec " + c.get("Spec"))
    if c.get("Issue") not in (None, "(none)"):
        parts.append(c.get("Issue"))
    return "- **%s** %s (%s)" % (c.id, c.title, "; ".join(parts))


def render_roadmap(bl, stage):
    open_cards = [c for c in ordered(bl) if c.is_open()]
    now = [c for c in open_cards if c.get("Status") in ("specced", "implementing")]
    rest = [c for c in open_cards if c.get("Status") in ("idea", "triaged")]
    nxt = [c for c in rest if urgency_for(c.get("Stage"), stage) >= 3][:5]
    later = [c for c in rest if c not in nxt]

    def block(cards, empty):
        return "\n".join(roadmap_line(c) for c in cards) if cards else empty

    return ("# Roadmap\n\n"
            "> Generated by `python3 scripts/pm/pm.py roadmap` from [backlog.md](backlog.md) at current stage %d.\n"
            "> A view, not the plan of record: the backlog is authoritative, and this file is redrafted when it changes.\n\n"
            "## Now\n\nCards with a spec, being specified or implemented.\n\n%s\n\n"
            "## Next\n\nThe five highest-scored cards without a spec in the current or next stage.\n\n%s\n\n"
            "## Later\n\nEvery other open card, by score.\n\n%s\n"
            ) % (stage, block(now, "Nothing in flight."), block(nxt, "Nothing queued."),
                 block(later, "Nothing else open."))


# ---- ids -------------------------------------------------------------------

def git(root, *args):
    env = {k: v for k, v in os.environ.items() if k not in ("GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE")}
    try:
        r = subprocess.run(["git", "-C", root] + list(args), capture_output=True, timeout=30, env=env)
    except (OSError, subprocess.SubprocessError):
        return None
    return r.stdout.decode("utf-8", "replace") if r.returncode == 0 else None


def main_checkout(root):
    common = git(root, "rev-parse", "--path-format=absolute", "--git-common-dir")
    return os.path.dirname(common.strip().rstrip("/")) if common else root


ID_SOURCES = {"MH": (BACKLOG, re.compile(r"^### MH-([1-9][0-9]*):", re.M)),
              "D": (DECISIONS, re.compile(r"^### D-([1-9][0-9]*) ", re.M)),
              "S": (SIGNALS, re.compile(r"^### S-([1-9][0-9]*) ", re.M))}


def next_id(root, kind):
    rel, pat = ID_SOURCES[kind]
    texts = []
    try:
        texts.append(read_text(root, rel))
    except InputError:
        pass
    for ref in ("main", "origin/main"):
        t = git(root, "show", "%s:%s" % (ref, rel))
        if t:
            texts.append(t)
    top = main_checkout(root)
    req_dir = os.path.join(top, "orchestration", "requests")
    for name in sorted(os.listdir(req_dir)) if os.path.isdir(req_dir) else []:
        try:
            with open(os.path.join(req_dir, name, "request.json"), encoding="utf-8") as f:
                if json.load(f).get("path") != rel:
                    continue
            with open(os.path.join(req_dir, name, "proposed"), encoding="utf-8") as f:
                texts.append(f.read())
        except (OSError, ValueError):
            continue
    data = os.path.join(top, ".claude", "data")
    os.makedirs(data, exist_ok=True)
    ledger = os.path.join(data, "id-reservations.jsonl")
    with open(ledger + ".lock", "a", encoding="utf-8") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        best = max([0] + [int(x) for t in texts for x in pat.findall(t)])
        try:
            with open(ledger, encoding="utf-8") as f:
                for line in f:
                    try:
                        row = json.loads(line)
                    except ValueError:
                        continue
                    if isinstance(row, dict) and row.get("kind") == kind and isinstance(row.get("n"), int):
                        best = max(best, row["n"])
        except OSError:
            pass
        n = best + 1
        with open(ledger, "a", encoding="utf-8") as f:
            f.write(json.dumps({"schema_version": 1, "kind": kind, "n": n,
                                "at": datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")}) + "\n")
    return "%s-%d" % (kind, n)


# ---- drafts ----------------------------------------------------------------

def write_out(root, out, text):
    target = os.path.realpath(out)
    product = os.path.realpath(os.path.join(root, "product"))
    if target == product or target.startswith(product + os.sep):
        raise InputError("--out %s is inside product/. pm.py drafts and never writes a product file: write the "
                         "draft to a scratch file, then file it with python3 scripts/orchestration/approvals.py "
                         "request" % out)
    # O_NOFOLLOW: a symlink swapped in after the check above cannot redirect the write.
    fd = os.open(target, os.O_WRONLY | os.O_CREAT | os.O_TRUNC | os.O_NOFOLLOW, 0o666)
    with os.fdopen(fd, "w", encoding="utf-8") as f:
        f.write(text)
    print("drafted %s" % out)


def fail_on(findings, what):
    if findings.errors:
        for e in findings.errors:
            print(e, file=sys.stderr)
        print("pm.py: %s" % what, file=sys.stderr)
        raise SystemExit(1)


def load_backlog(root, stale_urgency_ok=False):
    """The validated backlog and the current stage. rescore passes stale_urgency_ok,
    since recomputing urgencies after the stage advances is its job."""
    findings = Findings()
    stage = current_stage(root)
    bl = validate_backlog(root, findings, None if stale_urgency_ok else stage)
    fail_on(findings, "%s does not validate; fix it before drafting a change" % BACKLOG)
    return bl, stage


def draft_backlog(a, bl, stage):
    """Validates the drafted backlog as a whole, then writes it to --out."""
    text = bl.render()
    findings = Findings()
    validate_backlog(a.root, findings, stage, text)
    fail_on(findings, "the drafted change would not validate; nothing was written")
    write_out(a.root, a.out, text)


def issue_value(n):
    n = str(n).lstrip("#")
    if not re.match(r"^[1-9][0-9]*$", n):
        raise InputError("an issue is a positive number; got %r" % n)
    return "[#%s](https://github.com/%s/issues/%s)" % (n, OWNER_REPO, n)


def spec_value(value):
    if value in ("", "(none)"):
        return "(none)"
    return ", ".join("`%s`" % s.strip() for s in value.split(","))


def cmd_add(a):
    bl, stage = load_backlog(a.root)
    if a.stage not in STAGES:
        raise InputError("--stage must be one of %s" % ", ".join(STAGES))
    if parse_gates(a.gates) is None:
        raise InputError("--gates must be 'none' or G01-G16 in ascending order, comma-separated")
    ident = a.id or next_id(a.root, "MH")
    m = CARD_ID.match(ident)
    if not m or any(c.num == int(m.group(1)) for c in bl.cards):
        raise InputError("%s is not a free card id" % ident)
    card = Card(int(m.group(1)), a.title.strip(), 0)
    for name, value in (("Status", a.status), ("Stage", a.stage), ("Gates", a.gates),
                        ("Score", score_line(a.value, urgency_for(a.stage, stage), a.risk, a.effort)),
                        ("Spec", "(none)"), ("Issue", issue_value(a.issue) if a.issue else "(none)"),
                        ("Source", a.source.strip()), ("Summary", a.summary.strip())):
        card.set(name, value)
    bl.cards.append(card)
    bl.section_of[card.num] = "Open"
    draft_backlog(a, bl, stage)
    print("card %s" % card.id)


def cmd_set_status(a):
    bl, stage = load_backlog(a.root)
    c = bl.by_id(a.card)
    if a.status not in STATUSES:
        raise InputError("status must be one of %s" % ", ".join(STATUSES))
    if a.spec is not None:
        c.set("Spec", spec_value(a.spec))
    if a.status == "shipped":
        homes = spec_homes(a.root)
        pending = [s for s in parse_spec_field(c.get("Spec")) or []
                   if (homes.get(s) or ["missing"])[0] not in SHIPPED_SPEC_STATES]
        if pending:
            print("pm.py: refused: %s names specs not yet in done/ or archived/: %s. It stays implementing; "
                  "record the shipped part with: pm.py set %s half-shipped '<spec> shipped in <PR>; ships "
                  "with <remaining>'" % (c.id, ", ".join(pending), c.id), file=sys.stderr)
            raise SystemExit(1)
    c.set("Status", a.status)
    bl.section_of[c.num] = "Open" if c.is_open() else "Closed"
    draft_backlog(a, bl, stage)


def cmd_set(a):
    bl, stage = load_backlog(a.root)
    c = bl.by_id(a.card)
    key = a.field.lower()
    if key not in SETTABLE:
        raise InputError("settable fields: %s (status and score have their own verbs)" % ", ".join(sorted(SETTABLE)))
    value = a.value.strip()
    if key == "title":
        c.title = value
    elif key == "issue":
        c.set("Issue", "(none)" if value in ("", "(none)") else issue_value(value))
    elif key == "spec":
        c.set("Spec", spec_value(value))
    else:
        c.set(SETTABLE[key], value)
    draft_backlog(a, bl, stage)


def cmd_rescore(a):
    bl, stage = load_backlog(a.root, stale_urgency_ok=True)
    targets = [bl.by_id(a.card)] if a.card else [c for c in bl.cards if c.is_open()]
    if not a.card and (a.value or a.risk or a.effort or a.stage):
        raise InputError("--value, --risk, --effort and --stage need a card id")
    for c in targets:
        m = SCORE.match(c.get("Score"))
        v, r, e = int(m.group(2)), int(m.group(4)), int(m.group(5))
        v, r, e = a.value or v, a.risk or r, a.effort or e
        if a.stage:
            if a.stage not in STAGES:
                raise InputError("--stage must be one of %s" % ", ".join(STAGES))
            c.set("Stage", a.stage)
        c.set("Score", score_line(v, urgency_for(c.get("Stage"), stage), r, e))
    draft_backlog(a, bl, stage)


def draft_log(a, path, prefix, block_lines):
    findings = Findings()
    log = load_log(a.root, path, findings)
    fail_on(findings, "%s does not validate; fix it before drafting an entry" % path)
    ident = a.id or next_id(a.root, prefix)
    m = re.match(r"^%s-([1-9][0-9]*)$" % prefix, ident)
    if not m or int(m.group(1)) <= log.max_id():
        raise InputError("%s is not above the last id %s-%d" % (ident, prefix, log.max_id()))
    date = a.date or datetime.date.today().isoformat()
    try:
        datetime.date.fromisoformat(date)
    except ValueError as e:
        raise InputError("--date %r is not YYYY-MM-DD" % date) from e
    block = "\n".join(["### %s — %s: %s" % (ident, date, a.title.strip())] + block_lines)
    write_out(a.root, a.out, log.append(block))
    print("entry %s" % ident)


def cmd_decide(a):
    if a.type not in DECISION_TYPES:
        raise InputError("--type must be one of %s" % ", ".join(DECISION_TYPES))
    lines = ["- **Type**: %s" % a.type, "- **Decision**: %s" % a.decision.strip(),
             "- **Rationale**: %s" % a.rationale.strip(), "- **Cards**: %s" % (a.cards or "none").strip()]
    if a.evidence:
        lines.append("- **Evidence**: %s" % a.evidence.strip())
    draft_log(a, DECISIONS, "D", lines)


def cmd_signal(a):
    draft_log(a, SIGNALS, "S", ["- **Sources**: %s" % a.sources.strip(), "- **Count**: %d" % a.count,
                                "- **Summary**: %s" % a.summary.strip(),
                                "- **Cards**: %s" % (a.cards or "none").strip(),
                                "- **Suggested action**: %s" % a.action.strip()])


def cmd_draft_text(a):
    """Draft reviewed prose through the same outside-product write boundary.

    Card and log mutations retain their structured verbs. An objective stage
    change may temporarily need `rescore`; validate the entire set before filing.
    """
    try:
        with open(a.source_file, encoding="utf-8") as f:
            text = f.read()
    except OSError as e:
        raise InputError("cannot read draft source: %s" % (e.strerror or e)) from e
    if a.document == "backlog-preamble":
        bl, stage = load_backlog(a.root)
        bl.preamble = text.splitlines()
        draft_backlog(a, bl, stage)
        return
    path, title = (OBJECTIVES, "# Objectives") if a.document == "objectives" else (README, "# Product")
    read_text(a.root, path)  # The draft is based on an existing, freshly staged file.
    if not text.splitlines() or text.splitlines()[0] != title:
        raise InputError("%s must start with %s" % (path, title))
    if a.document == "objectives":
        stage_from_text(text)
    write_out(a.root, a.out, text.rstrip() + "\n")


def parser():
    p = argparse.ArgumentParser(prog="pm.py", description=__doc__.split("\n")[0])
    p.add_argument("--root", default=None)
    p.add_argument("--product-dir", default=None,
                   help="read the product files from this directory (a staged draft set) instead of <root>/product")
    sub = p.add_subparsers(dest="cmd", required=True)
    sub.add_parser("validate")
    ls = sub.add_parser("list")
    ls.add_argument("--status")
    ls.add_argument("--json", action="store_true")
    sub.add_parser("show").add_argument("card")
    sub.add_parser("next")
    sub.add_parser("next-id").add_argument("kind", choices=sorted(ID_SOURCES))
    ad = sub.add_parser("add")
    for f in ("title", "stage", "gates", "source", "summary", "out"):
        ad.add_argument("--" + f, required=True)
    for f in ("value", "risk", "effort"):
        ad.add_argument("--" + f, required=True, type=int, choices=range(1, 6))
    ad.add_argument("--status", default="triaged", choices=("idea", "triaged"))
    ad.add_argument("--issue")
    ad.add_argument("--id")
    ss = sub.add_parser("set-status")
    for f in ("card", "status"):
        ss.add_argument(f)
    ss.add_argument("--spec")
    ss.add_argument("--out", required=True)
    se = sub.add_parser("set")
    for f in ("card", "field", "value"):
        se.add_argument(f)
    se.add_argument("--out", required=True)
    rs = sub.add_parser("rescore")
    rs.add_argument("card", nargs="?")
    for f in ("value", "risk", "effort"):
        rs.add_argument("--" + f, type=int, choices=range(1, 6))
    rs.add_argument("--stage")
    rs.add_argument("--out", required=True)
    de = sub.add_parser("decide")
    for f in ("type", "title", "decision", "rationale", "out"):
        de.add_argument("--" + f, required=True)
    for f in ("cards", "evidence", "id", "date"):
        de.add_argument("--" + f)
    sg = sub.add_parser("signal")
    for f in ("title", "sources", "summary", "action", "out"):
        sg.add_argument("--" + f, required=True)
    sg.add_argument("--count", required=True, type=int)
    for f in ("cards", "id", "date"):
        sg.add_argument("--" + f)
    sub.add_parser("roadmap").add_argument("--out", required=True)
    sub.add_parser("fmt").add_argument("--out", required=True)
    dt = sub.add_parser("draft-text")
    dt.add_argument("document", choices=("objectives", "readme", "backlog-preamble"))
    dt.add_argument("--from", dest="source_file", required=True)
    dt.add_argument("--out", required=True)
    sub.add_parser("stage").add_argument("dir")
    return p


def main(argv):
    global PRODUCT_DIR
    a = parser().parse_args(argv)
    PRODUCT_DIR = a.product_dir
    if a.root is None:
        top = git(os.getcwd(), "rev-parse", "--show-toplevel")
        a.root = top.strip() if top else os.getcwd()
    try:
        return dispatch(a)
    except InputError as e:
        print("pm.py: %s" % e, file=sys.stderr)
        return 2


def dispatch(a):
    if a.cmd == "validate":
        f = validate(a.root)
        print("\n".join(f.warnings + f.errors) if f.warnings or f.errors else "product: OK")
        return 1 if f.errors else 0
    if a.cmd == "next-id":
        print(next_id(a.root, a.kind))
        return 0
    writers = {"add": cmd_add, "set-status": cmd_set_status, "set": cmd_set, "rescore": cmd_rescore,
               "decide": cmd_decide, "signal": cmd_signal, "draft-text": cmd_draft_text}
    if a.cmd in writers:
        writers[a.cmd](a)
        return 0
    if a.cmd == "stage":
        dest = os.path.realpath(a.dir)
        product = os.path.realpath(os.path.join(a.root, "product"))
        if dest == product or dest.startswith(product + os.sep):
            raise InputError("the staging directory %s is inside product/" % a.dir)
        os.makedirs(dest, exist_ok=True)
        copies = []
        for rel in (OBJECTIVES, BACKLOG, DECISIONS, SIGNALS, ROADMAP, README):
            target = os.path.join(dest, os.path.basename(rel))
            # Check every destination before any copy: an existing symlink or
            # hardlink may alias product/, and a scratch draft must not be lost.
            if os.path.lexists(target):
                raise InputError("staging target %s already exists; use a fresh staging directory" % target)
            copies.append((target, read_text(a.root, rel)))
        for target, text in copies:
            # Refuse a destination that appeared after preflight as well.
            fd = os.open(target, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o666)
            with os.fdopen(fd, "w", encoding="utf-8") as f:
                f.write(text)
        print("staged the product files in %s; draft with --product-dir %s --out %s/<file>" % (a.dir, a.dir, a.dir))
        return 0
    if a.cmd == "fmt":
        findings = Findings()
        bl = Backlog(read_text(a.root, BACKLOG), findings)
        fail_on(findings, "%s cannot be parsed; fix the lines above by hand" % BACKLOG)
        write_out(a.root, a.out, bl.render())
        return 0
    bl, stage = load_backlog(a.root)
    if a.cmd == "roadmap":
        write_out(a.root, a.out, render_roadmap(bl, stage))
    elif a.cmd == "list":
        want = set(a.status.split(",")) if a.status else None
        for c in (c for c in ordered(bl) if want is None or c.get("Status") in want):
            if a.json:
                print(json.dumps({"id": c.id, "title": c.title, "status": c.get("Status"),
                                  "stage": c.get("Stage"), "score": str(c.score()), "gates": c.get("Gates"),
                                  "spec": c.get("Spec"), "issue": c.get("Issue")}))
            else:
                print("%-6s %5s  %-12s stage %-5s %s" % (c.id, c.score(), c.get("Status"), c.get("Stage"), c.title))
    elif a.cmd == "show":
        print(bl.by_id(a.card).render())
    elif a.cmd == "next":
        c = next_card(bl, stage)
        if c is None:
            print("No idea or triaged card in the current or next stage.")
            return 1
        print(c.render())
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
