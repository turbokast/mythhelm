#!/usr/bin/env python3
"""harness_lint.py: the individual checks behind scripts/ci/lint-agent-harness.sh.

    harness_lint.py <check> [--root DIR]

Checks (each reads the tree at DIR, default the repository root):

  frontmatter     every agent, skill, rule, hook script and .claude/*.json passes
                  .claude/hooks/validate-agent-config.sh (the edit-time validator)
  routing-pins    every agent file has a row in knowledge/agent-routing.md and vice
                  versa; model: matches the row; effort: matches the tier default;
                  haiku agents carry no effort: and no Agent tool
  haiku-effort    no dispatch of a haiku agent in .claude/ or CLAUDE.md has an
                  effort parameter within WINDOW lines
  rule-budget     CLAUDE.md plus every rule without paths: fits RULE_BUDGET_BYTES
  paths-globs     every rule paths: glob matches a file in the tree, unless the line
                  above it is a "# future" comment (a marked glob that now matches is
                  reported as a warning)
  hook-inventory  hooks registered in .claude/settings.json and the rows of
                  .claude/hooks/INVENTORY.md agree on script, event and matcher; every
                  hook script is registered or listed as a sourced helper
  skill-contexts  every SKILL.md has an "## Invocation contexts" section naming the
                  Slash command, Model-invoked and Non-interactive behaviours
  dollar-zero     no skill markdown has an unescaped $0 (skill text is
                  argument-interpolated when it loads)
  abs-paths       no harness file names an absolute home directory
  references      markdown links in harness files resolve, and backticked file paths
                  under .claude/, knowledge/, scripts/, docs/ and specs/ exist, unless
                  followed by " (new file)"
  specs           every spec directory under specs/<state>/ has the files its
                  lifecycle state requires; each tasks.md task block carries
                  Domain/agent (a known agent), Budget (standard|complex), Change,
                  Files, Depends on (the first task may omit it), Acceptance and
                  Invariants touched; Depends on names existing tasks with no cycle;
                  an epic plan.md sits only in an epic state and every Work Streams
                  row names an existing spec; a spec name lives in one lifecycle
                  state only, counting copies the git index still holds
  finalize        every spec in specs/done/ has every task complete (heading marker
                  and a Status starting with a check mark) and a retrospective.md
                  with the sections RETRO_SECTIONS, a Review Summary whose Open
                  critical is 0, and no flake verdict without a Mechanism:; every
                  epic plan in done/ names only done or archived specs; CHANGELOG.md,
                  when present, is Keep a Changelog shaped (scripts/harness/finalize.py)
  proposals       every entry of .claude/proposals/pending.md is a '## P-<spec>-<n>'
                  heading, unique, with the fields PROPOSAL_FIELDS, a type in
                  PROPOSAL_TYPES, a source spec that exists, a target that exists (or
                  is marked "(new file)") and a non-empty Proposed change

Findings print as "path:line: check: detail". Exit 0 clean, 1 findings, 2 usage or
an unreadable tree. Standard library only; no network, no writes.
"""

import json
import os
import re
import subprocess
import sys

# The always-loaded byte budget: CLAUDE.md plus every rule without paths:. Raise it
# only in the change that adds the bytes, after trying paths: frontmatter and moving
# evidence to knowledge/rule-evidence/, and say in the pull request why.
RULE_BUDGET_BYTES = 16384

HAIKU_EFFORT_WINDOW = 10
EFFORT_LEVELS = ("low", "medium", "high", "xhigh", "max")
HARNESS_PREFIXES = (".claude/", "knowledge/", "scripts/", "docs/harness/")
HARNESS_ROOT_FILES = ("CLAUDE.md", "AGENTS.md", "WORKFLOW.md")
CONTEXT_LABELS = ("**Slash command**", "**Model-invoked**", "**Non-interactive**")

findings = []
warnings = []


def finding(path, line, check, detail):
    findings.append("%s:%s: %s: %s" % (path, line, check, detail))


def warn(path, line, check, detail):
    warnings.append("%s:%s: %s (warning): %s" % (path, line, check, detail))


# ---- tree ------------------------------------------------------------------

class TreeError(Exception):
    """The tree cannot be listed or read safely; the check exits 2."""


def git_env():
    """The environment minus the variables that point git at another repository or
    index, so every git call reads the tree named by its -C argument."""
    env = dict(os.environ)
    for k in ("GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR"):
        env.pop(k, None)
    return env


def tree_files(root):
    """Tracked plus untracked, non-ignored files, as repository-relative paths.

    Fails closed: the tree must be a git work tree, since only git knows which
    files are ignored. Symlinks are listed only when they resolve inside the tree,
    so no check reads outside it.
    """
    try:
        out = subprocess.run(
            ["git", "-C", root, "ls-files", "-z", "--cached", "--others", "--exclude-standard"],
            capture_output=True, check=True, timeout=60, env=git_env()).stdout
    except (OSError, subprocess.SubprocessError) as e:
        raise TreeError("cannot list files with git ls-files in %s: %s" % (root, e)) from e
    real_root = os.path.realpath(root)
    files = []
    for p in (x.decode("utf-8", "replace") for x in out.split(b"\0") if x):
        path = os.path.join(root, p)
        if not os.path.lexists(path):
            continue
        if os.path.islink(path) and os.path.commonpath([real_root, os.path.realpath(path)]) != real_root:
            sys.stderr.write("harness_lint.py: skipping %s: symlink resolves outside the tree\n" % p)
            continue
        files.append(p)
    return sorted(files)


def read(root, rel):
    with open(os.path.join(root, rel), encoding="utf-8") as f:
        return f.read()


def is_harness(rel):
    return rel.startswith(HARNESS_PREFIXES) or rel in HARNESS_ROOT_FILES


def frontmatter(text):
    """(lines of the leading --- block, index of its closing line) or (None, -1)."""
    lines = text.split("\n")
    if not lines or lines[0].strip() != "---":
        return None, -1
    for i in range(1, len(lines)):
        if lines[i].strip() == "---":
            return lines[1:i], i
    return None, -1


def fm_scalar(fm, key):
    for line in fm or []:
        m = re.match(r"^%s:\s*(.*?)\s*$" % re.escape(key), line)
        if m:
            v = m.group(1)
            if len(v) >= 2 and v[0] == v[-1] and v[0] in "\"'":
                v = v[1:-1]
            return v
    return None


def fm_list(fm, key):
    """A flow list ([a, b]) or block list under key; None when the key is absent."""
    for i, line in enumerate(fm or []):
        m = re.match(r"^%s:\s*(.*?)\s*$" % re.escape(key), line)
        if not m:
            continue
        v = m.group(1)
        if v.startswith("["):
            return [x.strip().strip("\"'") for x in v.strip("[]").split(",") if x.strip()]
        items = []
        for nxt in fm[i + 1:]:
            s = nxt.strip()
            if not s or s.startswith("#"):
                continue
            if not nxt.startswith((" ", "-")):
                break
            if s.startswith("- "):
                items.append(s[2:].strip().strip("\"'"))
        return items
    return None


def section(text, heading_prefix):
    """Lines under the first '## ' heading starting with heading_prefix, to the next '## '."""
    out, inside, found = [], False, False
    for line in text.split("\n"):
        if line.startswith("## "):
            if inside:
                break
            inside = line[3:].startswith(heading_prefix)
            found = found or inside
            continue
        if inside:
            out.append(line)
    return out if found else None


def agent_files(root):
    d = os.path.join(root, ".claude", "agents")
    if not os.path.isdir(d):
        return []
    return sorted(".claude/agents/" + n for n in os.listdir(d) if n.endswith(".md"))


# ---- checks ----------------------------------------------------------------

def check_frontmatter(root, files):
    hook = os.path.join(root, ".claude", "hooks", "validate-agent-config.sh")
    if not os.path.isfile(hook):
        finding(".claude/hooks/validate-agent-config.sh", 0, "frontmatter", "validator not found")
        return
    kinds = (
        re.compile(r"^\.claude/agents/[^/]+\.md$"),
        re.compile(r"^\.claude/skills/[^/]+/SKILL\.md$"),
        re.compile(r"^\.claude/rules/[^/]+\.md$"),
        re.compile(r"^\.claude/hooks/[^/]+\.sh$"),
        re.compile(r"^\.claude/[^/]+\.json$"),
    )
    for rel in files:
        if not any(k.match(rel) for k in kinds):
            continue
        path = os.path.join(os.path.abspath(root), rel)
        payload = json.dumps({"hook_event_name": "PreToolUse", "tool_name": "Write",
                              "tool_input": {"file_path": path, "content": read(root, rel)}})
        try:
            r = subprocess.run(["bash", hook], input=payload, capture_output=True, text=True, timeout=60)
        except (OSError, subprocess.TimeoutExpired) as e:
            finding(rel, 1, "frontmatter", "validator failed to run: %s" % e)
            continue
        if r.returncode != 0:
            details = [l[len("Detail: "):] for l in r.stderr.splitlines() if l.startswith("Detail: ")]
            for d in details or [r.stderr.strip() or "validator exited %d" % r.returncode]:
                finding(rel, 1, "frontmatter", d)


def parse_routing(root):
    rel = "knowledge/agent-routing.md"
    try:
        text = read(root, rel)
    except OSError:
        finding(rel, 0, "routing-pins", "routing page not found")
        return None, None
    table = {}
    row = re.compile(r"^\|\s*`([a-z0-9-]+)`\s*\|\s*`([a-z0-9-]+)`\s*\|")
    for line in section(text, "Canonical agent") or []:
        m = row.match(line)
        if m:
            table[m.group(1)] = m.group(2)
    tiers = {}
    tier_row = re.compile(r"^\|\s*`([a-z0-9-]+)`\s+\([a-z]+\)\s*\|([^|]*)\|")
    for line in section(text, "Tier effort defaults") or []:
        m = tier_row.match(line)
        if not m:
            continue
        cell = m.group(2).strip()
        lm = re.fullmatch(r"`(%s)`" % "|".join(EFFORT_LEVELS), cell)
        if lm:
            tiers[m.group(1)] = lm.group(1)
        elif "none" in cell:
            tiers[m.group(1)] = ""
        else:
            finding(rel, 0, "routing-pins", "unparseable tier effort cell for %s: %r" % (m.group(1), cell))
    if not table:
        finding(rel, 0, "routing-pins", "no rows parsed from '## Canonical agent -> model mapping'")
    if not tiers:
        finding(rel, 0, "routing-pins", "no rows parsed from '## Tier effort defaults'")
    return table, tiers


def check_routing_pins(root, files):
    table, tiers = parse_routing(root)
    if not table or not tiers:
        return
    seen = set()
    for rel in agent_files(root):
        name = os.path.basename(rel)[:-3]
        seen.add(name)
        fm, _ = frontmatter(read(root, rel))
        model, effort = fm_scalar(fm, "model"), fm_scalar(fm, "effort")
        if not model:
            finding(rel, 1, "routing-pins", "MISSING-MODEL: no model: in frontmatter")
            continue
        if name not in table:
            finding(rel, 1, "routing-pins", "MISSING-ROW: agent %s has no row in knowledge/agent-routing.md" % name)
        elif table[name] != model:
            finding(rel, 1, "routing-pins", "PIN-MISMATCH: model %s, but the routing table says %s" % (model, table[name]))
        if model not in tiers:
            finding(rel, 1, "routing-pins", "EFFORT-NO-TIER: model %s has no tier effort row" % model)
        elif tiers[model] == "" and effort:
            finding(rel, 1, "routing-pins", "EFFORT-FORBIDDEN: %s agents carry no effort:, found %s" % (model, effort))
        elif tiers[model] and not effort:
            finding(rel, 1, "routing-pins", "MISSING-EFFORT: %s agents pin effort: %s" % (model, tiers[model]))
        elif tiers[model] and effort != tiers[model]:
            finding(rel, 1, "routing-pins", "EFFORT-MISMATCH: effort %s, but the %s tier is %s" % (effort, model, tiers[model]))
        if model == "haiku":
            tools = fm_list(fm, "tools")
            if tools is None:
                finding(rel, 1, "routing-pins", "HAIKU-TOOLS: haiku agents need a tools: allowlist without Agent")
            elif "Agent" in tools:
                finding(rel, 1, "routing-pins", "HAIKU-TOOLS: haiku agents never carry the Agent tool")
    for name in sorted(set(table) - seen):
        finding("knowledge/agent-routing.md", 0, "routing-pins", "MISSING-FILE: row %s has no .claude/agents/%s.md" % (name, name))


def check_haiku_effort(root, files):
    haiku = set()
    for rel in agent_files(root):
        fm, _ = frontmatter(read(root, rel))
        if fm_scalar(fm, "model") == "haiku":
            haiku.add(os.path.basename(rel)[:-3])
    q = "[\"']?"
    dispatch = re.compile(r"\b(model|subagent_type)\s*[=:]\s*%s([a-z0-9-]+)" % q)
    effort = re.compile(r"\beffort\s*[=:]\s*%s(%s)\b" % (q, "|".join(EFFORT_LEVELS)))
    for rel in files:
        if not rel.endswith(".md") or not (rel.startswith(".claude/") or rel == "CLAUDE.md"):
            continue
        lines = read(root, rel).split("\n")
        _, fm_end = frontmatter("\n".join(lines))
        # Every dispatch token outside the frontmatter: (line, is_haiku).
        tokens = []
        for i, line in enumerate(lines):
            if i <= fm_end:
                continue
            for m in dispatch.finditer(line):
                name = m.group(2)
                tokens.append((i, name == "haiku" if m.group(1) == "model" else name in haiku))
        # Each effort parameter belongs to the nearest dispatch within the window
        # (ties go to the dispatch above it); it is a finding when that is a haiku one.
        for j, line in enumerate(lines):
            m = effort.search(line)
            if j <= fm_end or not m:
                continue
            near = [(abs(i - j), i > j, i, h) for i, h in tokens if abs(i - j) <= HAIKU_EFFORT_WINDOW]
            if near and min(near)[3]:
                finding(rel, j + 1, "haiku-effort",
                        "%s belongs to the haiku dispatch at line %d; the haiku tier rejects effort"
                        % (m.group(0), min(near)[2] + 1))


def always_on_rules(root):
    d = os.path.join(root, ".claude", "rules")
    out = []
    for n in sorted(os.listdir(d)) if os.path.isdir(d) else []:
        if not n.endswith(".md"):
            continue
        rel = ".claude/rules/" + n
        fm, _ = frontmatter(read(root, rel))
        if fm_list(fm, "paths") is None:
            out.append(rel)
    return out


def check_rule_budget(root, files):
    rows = []
    for rel in ["CLAUDE.md"] + always_on_rules(root):
        try:
            rows.append((os.path.getsize(os.path.join(root, rel)), rel))
        except OSError:
            finding(rel, 0, "rule-budget", "not found")
    total = sum(n for n, _ in rows)
    print("Always-on context (CLAUDE.md + rules without paths:):")
    for n, rel in sorted(rows, reverse=True):
        print("%8d  %s" % (n, rel))
    print("%8d  TOTAL (budget %d)" % (total, RULE_BUDGET_BYTES))
    if total > RULE_BUDGET_BYTES:
        finding("CLAUDE.md", 0, "rule-budget",
                "always-on bytes %d exceed the budget %d by %d; add paths: to a rule, move evidence to "
                "knowledge/rule-evidence/, or raise RULE_BUDGET_BYTES in scripts/ci/harness_lint.py with a reason"
                % (total, RULE_BUDGET_BYTES, total - RULE_BUDGET_BYTES))


def glob_regex(glob):
    out, i = [], 0
    while i < len(glob):
        if glob.startswith("**/", i):
            out.append("(?:.*/)?")
            i += 3
        elif glob.startswith("**", i):
            out.append(".*")
            i += 2
        elif glob[i] == "*":
            out.append("[^/]*")
            i += 1
        elif glob[i] == "?":
            out.append("[^/]")
            i += 1
        elif glob[i] == "{":
            j = glob.index("}", i)
            out.append("(?:%s)" % "|".join(glob_regex(a) for a in glob[i + 1:j].split(",")))
            i = j + 1
        elif glob[i] == "[":
            j = glob.index("]", i + 2)
            body = glob[i + 1:j]
            out.append("[%s]" % ("^" + body[1:] if body.startswith("!") else body))
            i = j + 1
        else:
            out.append(re.escape(glob[i]))
            i += 1
    return "".join(out)


def check_paths_globs(root, files):
    d = os.path.join(root, ".claude", "rules")
    for n in sorted(os.listdir(d)) if os.path.isdir(d) else []:
        rel = ".claude/rules/" + n
        if not n.endswith(".md"):
            continue
        fm, _ = frontmatter(read(root, rel))
        if fm is None or fm_list(fm, "paths") is None:
            continue
        start = next(i for i, l in enumerate(fm) if l.startswith("paths:"))
        prev = ""
        for i in range(start + 1, len(fm)):
            line = fm[i]
            s = line.strip()
            if not s:
                continue
            if s.startswith("#"):
                prev = s
                continue
            if not line.startswith((" ", "-")):
                break
            if not s.startswith("- "):
                continue
            glob = s[2:].strip().strip("\"'")
            future = prev.startswith("# future")
            prev = ""
            try:
                rx = re.compile(glob_regex(glob) + r"\Z")
            except (ValueError, re.error) as e:
                finding(rel, i + 2, "paths-globs", "glob %r does not compile: %s" % (glob, e))
                continue
            hit = any(rx.match(f) for f in files)
            if not hit and not future:
                finding(rel, i + 2, "paths-globs",
                        "glob %r matches no file; fix it, or mark a path that does not exist yet with a "
                        "'# future: <reason>' line above it" % glob)
            elif hit and future:
                warn(rel, i + 2, "paths-globs", "glob %r now matches; delete its '# future' marker" % glob)


def inventory_rows(text):
    """({script: {(event, matcher)}}, {helper scripts}) from INVENTORY.md's two tables."""
    hooks, helpers, in_helpers = {}, set(), False
    for line in text.split("\n"):
        if line.startswith("## "):
            in_helpers = "helper" in line.lower()
        m = re.match(r"^\|\s*`([^`]+\.sh)`\s*\|(.*)$", line)
        if not m:
            continue
        name = os.path.basename(m.group(1))
        if in_helpers:
            helpers.add(name)
            continue
        cell = re.split(r"(?<!\\)\|", m.group(2))[0]
        pairs = set()
        for part in re.split(r";|<br>", cell):
            em = re.match(r"^\s*([A-Za-z]+)\s*/\s*`([^`]*)`\s*$", part)
            if em:
                pairs.add((em.group(1), em.group(2).replace("\\|", "|")))
        hooks[name] = pairs
    return hooks, helpers


def check_hook_inventory(root, files):
    srel, irel = ".claude/settings.json", ".claude/hooks/INVENTORY.md"
    try:
        settings = json.loads(read(root, srel))
        inventory = read(root, irel)
    except (OSError, ValueError) as e:
        finding(srel, 0, "hook-inventory", "cannot read settings or inventory: %s" % e)
        return
    registered = {}
    for event, groups in (settings.get("hooks") or {}).items():
        for group in groups or []:
            for h in group.get("hooks", []):
                cmd = h.get("command", "")
                name = os.path.basename(cmd.split()[0]) if cmd.split() else ""
                if name.endswith(".sh"):
                    registered.setdefault(name, set()).add((event, group.get("matcher", "")))
    listed, helpers = inventory_rows(inventory)
    for name, pairs in sorted(registered.items()):
        if name not in listed:
            finding(irel, 0, "hook-inventory", "%s is registered in settings.json but has no inventory row" % name)
        elif listed[name] != pairs:
            finding(irel, 0, "hook-inventory", "%s: inventory says %s, settings.json registers %s"
                    % (name, sorted(listed[name]), sorted(pairs)))
    for name in sorted(set(listed) - set(registered)):
        finding(irel, 0, "hook-inventory", "%s has an inventory row but is not registered in settings.json" % name)
    for rel in files:
        m = re.match(r"^\.claude/hooks/([^/]+\.sh)$", rel)
        if m and m.group(1) not in registered and m.group(1) not in helpers:
            finding(rel, 0, "hook-inventory", "hook script is neither registered nor listed as a sourced helper")


def check_skill_contexts(root, files):
    for rel in files:
        if not re.match(r"^\.claude/skills/[^/]+/SKILL\.md$", rel):
            continue
        body = section(read(root, rel), "Invocation contexts")
        if body is None:
            finding(rel, 0, "skill-contexts", "no '## Invocation contexts' section")
            continue
        text = "\n".join(body)
        for label in CONTEXT_LABELS:
            if label not in text:
                finding(rel, 0, "skill-contexts", "'## Invocation contexts' does not name %s" % label)


def check_dollar_zero(root, files):
    rx = re.compile(r"(?<!\\)\$0")
    for rel in files:
        if not (rel.startswith(".claude/skills/") and rel.endswith(".md")):
            continue
        for i, line in enumerate(read(root, rel).split("\n"), 1):
            if rx.search(line):
                finding(rel, i, "dollar-zero", "unescaped $0 is replaced by the skill's arguments when it loads; write \\$0")


def check_abs_paths(root, files):
    rx = re.compile(r"(?<![\w.$~-])(?:/(?:home|Users)/[A-Za-z0-9._-]+|[A-Za-z]:\\+Users\\+[A-Za-z0-9._-]+)")
    for rel in files:
        if not is_harness(rel):
            continue
        try:
            text = read(root, rel)
        except (OSError, UnicodeDecodeError):
            continue
        for i, line in enumerate(text.split("\n"), 1):
            m = rx.search(line)
            if m:
                finding(rel, i, "abs-paths", "absolute home path %r; use $HOME, a repository-relative path or /tmp" % m.group(0))


def check_references(root, files):
    link = re.compile(r"\[[^\]]*\]\(([^)\s]+)(?:\s+\"[^\"]*\")?\)")
    code = re.compile(r"`([^`\s]+)`")
    path_prefix = (".claude/", "knowledge/", "scripts/", "docs/", "specs/")
    for rel in files:
        if not (rel.endswith(".md") and is_harness(rel)):
            continue
        fence = False
        for i, line in enumerate(read(root, rel).split("\n"), 1):
            if line.lstrip().startswith("```"):
                fence = not fence
                continue
            if fence:
                continue
            for m in link.finditer(line):
                target = m.group(1)
                if re.match(r"^[a-z][a-z0-9+.-]*:", target) or target.startswith("#") or "<" in target:
                    continue
                target = target.split("#", 1)[0].split("?", 1)[0]
                base = root if target.startswith("/") else os.path.join(root, os.path.dirname(rel))
                if target and not os.path.exists(os.path.normpath(os.path.join(base, target.lstrip("/")))):
                    finding(rel, i, "references", "link target %r does not exist" % m.group(1))
            for m in code.finditer(line):
                if line[m.end():].startswith(" (new file)"):
                    continue
                token = re.sub(r":\d+(?:-\d+)?$", "", m.group(1))
                if not token.startswith(path_prefix) or token.startswith(".claude/data/"):
                    continue
                if re.search(r"[*?<>{}\[\]$]", token) or not re.search(r"/[^/]*\.[A-Za-z0-9]+$", token):
                    continue
                if not os.path.exists(os.path.join(root, token)):
                    finding(rel, i, "references", "path %r does not exist" % token)


SPEC_STATES = ("unrefined", "refined", "todo", "in-progress", "unfinalized", "done", "archived")
SPEC_REQUIRED = {
    "unrefined": ("requirements.md",),
    "refined": ("requirements.md",),
    "todo": ("requirements.md", "design.md", "tasks.md"),
    "in-progress": ("requirements.md", "design.md", "tasks.md"),
    "unfinalized": ("requirements.md", "design.md", "tasks.md"),
    "done": ("requirements.md", "design.md", "tasks.md"),
    "archived": ("requirements.md",),
}
EPIC_STATES = ("unrefined", "refined", "in-progress", "done", "archived")
SPEC_BUDGETS = ("standard", "complex")
# Fields every task block carries. The first task in a file may omit Depends on:
# tasks are listed in dependency order, so it has nothing earlier to depend on.
TASK_FIELDS = ("Domain/agent", "Budget", "Change", "Files", "Depends on", "Acceptance", "Invariants touched")
TASK_HEADING = re.compile(r"^###\s+Task\s+(\d+)\b")
TASK_FIELD = re.compile(r"^- \*\*([^*]+?)\*\*(?:\s*\([^)]*\))?\s*:\s*(.*)$")


def task_blocks(text):
    """[(number, heading line, {field: (line, value, has sub-bullets)})] outside code fences."""
    blocks, cur, fence = [], None, False
    lines = text.split("\n")
    for i, line in enumerate(lines, 1):
        if line.lstrip().startswith("```"):
            fence = not fence
            continue
        if fence:
            continue
        m = TASK_HEADING.match(line)
        if m:
            cur = (int(m.group(1)), i, {})
            blocks.append(cur)
            continue
        if line.startswith(("## ", "### ")):
            cur = None
            continue
        fm = TASK_FIELD.match(line)
        if cur is not None and fm:
            name = fm.group(1).strip()
            if name == "Acceptance criteria":
                name = "Acceptance"
            nested = i < len(lines) and lines[i].startswith(("  -", "  *", "  1"))
            cur[2].setdefault(name, (i, fm.group(2).strip(), nested))
    return blocks


def depends_refs(value):
    """Task numbers named outside parenthesised notes, [] for None, or None when unreadable.

    Every note is removed wherever it sits, so a reference after a note is still
    checked; nested or unbalanced parentheses make the value unreadable.
    """
    head = re.sub(r"\([^()]*\)", "", value).strip().rstrip(".")
    if "(" in head or ")" in head:
        return None
    if re.fullmatch(r"(?i)none", head):
        return []
    parts = [p.strip() for p in re.split(r",|\band\b", head) if p.strip()]
    refs = []
    for p in parts:
        m = re.fullmatch(r"Task\s+(\d+)", p)
        if not m:
            return None
        refs.append(int(m.group(1)))
    return refs or None


def check_tasks(root, rel, agents):
    blocks = task_blocks(read(root, rel))
    if not blocks:
        finding(rel, 0, "specs", "no '### Task N' blocks")
        return
    numbers = {}
    for n, line, _ in blocks:
        if n in numbers:
            finding(rel, line, "specs", "Task %d is defined twice (first at line %d)" % (n, numbers[n]))
        numbers.setdefault(n, line)
    edges = {}
    for idx, (n, line, fields) in enumerate(blocks):
        for name in TASK_FIELDS:
            if name not in fields:
                if name == "Depends on" and idx == 0:
                    continue
                finding(rel, line, "specs", "Task %d has no '- **%s**:' field" % (n, name))
                continue
            fline, value, nested = fields[name]
            if not value and not nested:
                finding(rel, fline, "specs", "Task %d: '%s' is empty" % (n, name))
                continue
            if name == "Domain/agent":
                first = re.sub(r"[`*]", "", value).split()[0].rstrip(",;") if value else ""
                if first not in agents:
                    finding(rel, fline, "specs", "Task %d: Domain/agent %r is not an agent in .claude/agents/ or 'maintainer'"
                            % (n, first))
            elif name == "Budget":
                tier = re.sub(r"[`*]", "", value).split()[0].rstrip(",;") if value else ""
                if tier not in SPEC_BUDGETS:
                    finding(rel, fline, "specs", "Task %d: Budget %r is not one of %s" % (n, tier, "/".join(SPEC_BUDGETS)))
            elif name == "Depends on":
                refs = depends_refs(value)
                if refs is None:
                    finding(rel, fline, "specs", "Task %d: Depends on %r is not 'None' or a list of 'Task N'" % (n, value))
                    continue
                for r in refs:
                    if r == n:
                        finding(rel, fline, "specs", "Task %d depends on itself" % n)
                    elif r not in numbers:
                        finding(rel, fline, "specs", "Task %d depends on Task %d, which does not exist" % (n, r))
                edges[n] = [r for r in refs if r in numbers and r != n]
    # Cycles: an iterative depth-first search that reports each cycle once.
    state, reported = {}, set()
    for start in sorted(edges):
        if state.get(start):
            continue
        stack, path = [(start, iter(edges.get(start, [])))], [start]
        state[start] = 1
        while stack:
            node, it = stack[-1]
            nxt = next(it, None)
            if nxt is None:
                state[node] = 2
                stack.pop()
                path.pop()
            elif state.get(nxt) == 1:
                cyc = path[path.index(nxt):] + [nxt]
                key = frozenset(cyc)
                if key not in reported:
                    reported.add(key)
                    finding(rel, numbers[nxt], "specs", "dependency cycle: %s" % " -> ".join("Task %d" % c for c in cyc))
            elif not state.get(nxt):
                state[nxt] = 1
                stack.append((nxt, iter(edges.get(nxt, []))))
                path.append(nxt)


def work_stream_specs(text):
    """(names, unreadable rows) from the Work Streams table, or (None, []) without the section.

    Every data row's Spec cell (column 2) must be one backticked name; a row whose
    cell is not is returned as unreadable, never skipped.
    """
    names, bad, inside, found, header = [], [], False, False, True
    for i, line in enumerate(text.split("\n"), 1):
        if re.match(r"^#{2,3} ", line):
            inside = bool(re.match(r"^#{2,3} Work Streams\s*$", line))
            found = found or inside
            header = True
            continue
        if not inside or not line.startswith("|"):
            continue
        if re.fullmatch(r"\|[\s:|-]+\|?\s*", line):
            continue
        if header:
            header = False
            continue
        cells = line.split("|")
        m = re.fullmatch(r"\s*`([a-z0-9][a-z0-9-]*)`\s*", cells[2]) if len(cells) > 2 else None
        if m:
            names.append(m.group(1))
        else:
            bad.append(i)
    return (names, bad) if found else (None, [])


def index_spec_dirs(root):
    """(state, name) for every spec directory the git index holds, including paths
    deleted from the working tree but not from the index (a plain mv leaves them)."""
    try:
        out = subprocess.run(["git", "-C", root, "ls-files", "-z", "--cached", "--", "specs/"],
                             capture_output=True, check=True, timeout=60, env=git_env()).stdout
    except (OSError, subprocess.SubprocessError) as e:
        raise TreeError("cannot read the git index in %s: %s" % (root, e)) from e
    placed = set()
    for p in (x.decode("utf-8", "replace") for x in out.split(b"\0") if x):
        parts = p.split("/")
        if len(parts) >= 4 and parts[1] in SPEC_STATES:
            placed.add((parts[1], parts[2]))
    return placed


def check_specs(root, files):
    agents = {os.path.basename(a)[:-3] for a in agent_files(root)} | {"maintainer"}
    dirs = {}
    for rel in files:
        if not rel.startswith("specs/"):
            continue
        parts = rel.split("/")
        if len(parts) == 2:
            if parts[1] != "README.md":
                finding(rel, 0, "specs", "only README.md sits directly under specs/; specs live in specs/<state>/<name>/")
            continue
        if parts[1] not in SPEC_STATES:
            finding(rel, 0, "specs", "%r is not a lifecycle state (%s)" % (parts[1], ", ".join(SPEC_STATES)))
            continue
        if len(parts) == 3:
            if parts[2] != ".gitkeep":
                finding(rel, 0, "specs", "a file directly under specs/%s/ is not in a spec directory" % parts[1])
            continue
        dirs.setdefault((parts[1], parts[2]), set()).add("/".join(parts[3:]))
    homes = {}
    for state, name in sorted(set(dirs) | index_spec_dirs(root)):
        homes.setdefault(name, []).append(state)
    for name, states in sorted(homes.items()):
        if len(states) > 1:
            finding("specs/%s/%s" % (states[0], name), 0, "specs",
                    "spec %r exists in %d lifecycle states (%s); exactly one copy may exist"
                    % (name, len(states), ", ".join(states)))
    for (state, name), present in sorted(dirs.items()):
        base = "specs/%s/%s" % (state, name)
        if "plan.md" in present and "requirements.md" not in present:
            if state not in EPIC_STATES:
                finding(base + "/plan.md", 0, "specs", "an epic plan never enters %s/ (epic states: %s)"
                        % (state, ", ".join(EPIC_STATES)))
            streams, unreadable = work_stream_specs(read(root, base + "/plan.md"))
            for line in unreadable:
                finding(base + "/plan.md", line, "specs", "Work Streams row has no backticked spec name in column 2")
            if streams is None:
                finding(base + "/plan.md", 0, "specs", "epic plan has no '## Work Streams' section")
            elif not streams and not unreadable:
                finding(base + "/plan.md", 0, "specs", "the Work Streams table names no spec in backticks")
            for s in streams or []:
                if s not in homes:
                    finding(base + "/plan.md", 0, "specs", "work stream spec %r has no directory under specs/<state>/" % s)
            continue
        for req in SPEC_REQUIRED[state]:
            if req not in present:
                finding(base, 0, "specs", "a spec in %s/ needs %s" % (state, req))
        if "tasks.md" in present:
            check_tasks(root, base + "/tasks.md", agents)


# Sections every retrospective.md of a finalized spec carries, in any order.
RETRO_SECTIONS = ("Review Summary", "Acceptance", "Deviations", "CI history", "Effort", "Lessons", "Proposals")
REVIEW_FIELDS = ("Range", "Reviewer", "Findings", "Open critical", "Vendor review")


def load_finalize():
    """scripts/harness/finalize.py from this repository, whatever tree --root names."""
    sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "harness"))
    try:
        import finalize  # noqa: E402  (a sibling script, not an installed package)
    finally:
        sys.path.pop(0)
    return finalize


def check_retrospective(root, rel):
    text = read(root, rel)
    heads = {}
    for i, line in enumerate(text.split("\n"), 1):
        m = re.match(r"^## (.+?)\s*$", line)
        if m:
            heads.setdefault(m.group(1), i)
    for name in RETRO_SECTIONS:
        if name not in heads:
            finding(rel, 0, "finalize", "no '## %s' section" % name)
    review = section(text, "Review Summary")
    for name in REVIEW_FIELDS if "Review Summary" in heads else ():
        m = next((re.match(r"^- \*\*%s\*\*:\s*(.*)$" % re.escape(name), l) for l in review
                  if re.match(r"^- \*\*%s\*\*:" % re.escape(name), l)), None)
        if m is None or not m.group(1).strip():
            finding(rel, heads["Review Summary"], "finalize", "Review Summary has no '- **%s**:' value" % name)
        elif name == "Open critical" and not re.match(r"^0\b", m.group(1).strip()):
            finding(rel, heads["Review Summary"], "finalize",
                    "Review Summary lists open critical findings (%s); a spec ships with none" % m.group(1).strip())
    for i, line in enumerate(text.split("\n"), 1):
        if re.search(r"(?i)\bflak(e|es|y|iness)\b", line) and "Mechanism:" not in line:
            finding(rel, i, "finalize", "a flake verdict needs 'Mechanism:' on its line: the mechanism, "
                    "the failing and the passing value (agent-behavioral-posture.md section 7)")


def check_finalize(root, files):
    present = set(files)
    for rel in files:
        parts = rel.split("/")
        if len(parts) != 4 or parts[:2] != ["specs", "done"]:
            continue
        base = "/".join(parts[:3])
        if parts[3] == "plan.md" and base + "/requirements.md" not in present:
            streams, _ = work_stream_specs(read(root, rel))
            homes = {}
            for f in files:
                q = f.split("/")
                if len(q) >= 4 and q[0] == "specs":
                    homes.setdefault(q[2], set()).add(q[1])
            for s in streams or []:
                if not homes.get(s) or not homes[s] <= {"done", "archived"}:
                    finding(rel, 0, "finalize", "epic in done/ names work stream %r, which is not done or archived" % s)
            continue
        if parts[3] != "requirements.md":
            continue
        if base + "/retrospective.md" not in present:
            finding(base, 0, "finalize", "a spec in done/ needs retrospective.md (/finalize-spec-retrospective)")
        else:
            check_retrospective(root, base + "/retrospective.md")
        if base + "/tasks.md" in present:
            text = read(root, base + "/tasks.md")
            for n, line, fields in task_blocks(text):
                heading = text.split("\n")[line - 1]
                status = fields.get("Status", (line, "", False))[1]
                if "\u2705 COMPLETED" not in heading or not status.startswith("\u2705"):
                    finding(base + "/tasks.md", line, "finalize",
                            "Task %d is not complete (heading marker and a Status starting with \u2705)" % n)
    if "CHANGELOG.md" in present:
        for line, detail in load_finalize().changelog_problems(read(root, "CHANGELOG.md")):
            finding("CHANGELOG.md", line, "finalize", detail)


PROPOSAL_TYPES = ("rule", "skill", "hook", "knowledge", "product")
PROPOSAL_FIELDS = ("Source spec", "Type", "Target", "Rationale", "Evidence")


def check_proposals(root, files):
    rel = ".claude/proposals/pending.md"
    if rel not in files:
        return
    homes = {f.split("/")[2] for f in files if f.startswith("specs/") and len(f.split("/")) >= 4}
    lines = read(root, rel).split("\n")
    entries, cur, fence = [], None, False
    for i, line in enumerate(lines, 1):
        if line.lstrip().startswith("```"):
            fence = not fence
        if fence:
            if cur:
                cur["body"].append(line)
            continue
        if line.startswith("## "):
            cur = {"line": i, "heading": line, "fields": {}, "body": []}
            entries.append(cur)
            continue
        if cur is None:
            continue
        m = re.match(r"^- \*\*([^*]+?)\*\*:\s*(.*)$", line)
        if m and m.group(1) in PROPOSAL_FIELDS:
            cur["fields"].setdefault(m.group(1), m.group(2).strip())
        cur["body"].append(line)
    seen = {}
    for e in entries:
        i = e["line"]
        m = re.match(r"^## (P-([a-z0-9][a-z0-9-]*)-(\d+))(?: — .+)?$", e["heading"])
        if not m:
            finding(rel, i, "proposals", "heading %r is not '## P-<spec>-<n> — <title>'" % e["heading"])
            continue
        pid, spec = m.group(1), m.group(2)
        if pid in seen:
            finding(rel, i, "proposals", "%s is defined twice (first at line %d)" % (pid, seen[pid]))
        seen.setdefault(pid, i)
        f = e["fields"]
        for name in PROPOSAL_FIELDS:
            if not f.get(name):
                finding(rel, i, "proposals", "%s has no '- **%s**:' value" % (pid, name))
        src = re.sub(r"[`\s]", "", f.get("Source spec", ""))
        if src and src != spec:
            finding(rel, i, "proposals", "%s: Source spec %r does not match the id's spec %r" % (pid, src, spec))
        if src and src not in homes:
            finding(rel, i, "proposals", "%s: Source spec %r has no directory under specs/<state>/" % (pid, src))
        typ = f.get("Type", "").strip("` ")
        if typ and typ not in PROPOSAL_TYPES:
            finding(rel, i, "proposals", "%s: Type %r is not one of %s" % (pid, typ, "/".join(PROPOSAL_TYPES)))
        target = f.get("Target", "")
        tm = re.match(r"^`([^`\s]+)`(\s+\(new file\))?", target)
        if target and not tm:
            finding(rel, i, "proposals", "%s: Target is not one backticked repository path" % pid)
        elif tm and (tm.group(1).startswith("/") or ".." in tm.group(1).split("/")):
            finding(rel, i, "proposals", "%s: Target %r is not repository-relative" % (pid, tm.group(1)))
        elif tm and not tm.group(2) and not os.path.exists(os.path.join(root, tm.group(1))):
            finding(rel, i, "proposals", "%s: Target %r does not exist; mark a new file '(new file)'" % (pid, tm.group(1)))
        body = e["body"]
        at = next((k for k, l in enumerate(body) if l.strip() == "**Proposed change:**"), None)
        if at is None or not any(l.strip() for l in body[at + 1:]):
            finding(rel, i, "proposals", "%s has no '**Proposed change:**' block with content" % pid)


CHECKS = {
    "frontmatter": check_frontmatter,
    "routing-pins": check_routing_pins,
    "haiku-effort": check_haiku_effort,
    "rule-budget": check_rule_budget,
    "paths-globs": check_paths_globs,
    "hook-inventory": check_hook_inventory,
    "skill-contexts": check_skill_contexts,
    "dollar-zero": check_dollar_zero,
    "abs-paths": check_abs_paths,
    "references": check_references,
    "specs": check_specs,
    "finalize": check_finalize,
    "proposals": check_proposals,
}


def main(argv):
    if argv[:1] == ["--list"]:
        print("\n".join(CHECKS))
        return 0
    if not argv or argv[0] not in CHECKS:
        sys.stderr.write("usage: harness_lint.py <%s> [--root DIR]\n" % "|".join(CHECKS))
        return 2
    check, rest = argv[0], argv[1:]
    root = None
    if rest[:1] == ["--root"] and len(rest) == 2:
        root = rest[1]
    elif rest:
        sys.stderr.write("harness_lint.py: unexpected arguments: %s\n" % " ".join(rest))
        return 2
    if root is None:
        root = os.path.normpath(os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", ".."))
    if not os.path.isdir(os.path.join(root, ".claude")):
        sys.stderr.write("harness_lint.py: %s has no .claude/ directory\n" % root)
        return 2
    try:
        files = tree_files(root)
    except TreeError as e:
        sys.stderr.write("harness_lint.py: %s\n" % e)
        return 2
    try:
        CHECKS[check](root, files)
    except TreeError as e:
        sys.stderr.write("harness_lint.py: %s\n" % e)
        return 2
    for w in warnings:
        print(w)
    for f in findings:
        print(f)
    return 1 if findings else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
