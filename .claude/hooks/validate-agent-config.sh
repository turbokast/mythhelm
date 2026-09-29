#!/usr/bin/env bash
# validate-agent-config.sh: PreToolUse hook (Edit|Write). Validates the content an
# Edit or Write is about to leave in a harness configuration file, and blocks the
# edit when it is malformed:
#
#   .claude/agents/<name>.md        frontmatter with name, description and model;
#                                   name equals <name>; model is opus, sonnet or haiku;
#                                   no tool in both tools and disallowedTools
#   .claude/skills/<name>/SKILL.md  frontmatter with name and description; name
#                                   equals <name>; model, when present, as above
#   .claude/rules/*.md              optional frontmatter; `paths:` is a list of
#                                   well-formed, repository-relative globs
#   .claude/hooks/*.sh              parses with bash -n
#   .claude/*.json                  parses as JSON; in settings*.json every hook
#                                   event is a known event name, and every hook
#                                   command that names a script points at an
#                                   executable file
#
# Names are lowercase letters, digits and hyphens. YAML syntax is checked with
# PyYAML when it is installed; without it a small built-in parser reads the simple
# frontmatter subset this repository uses. For an Edit, the post-edit content is
# built from the file on disk; an old_string that is not found is left to the Edit
# tool to report.
#
# This is a quality gate, not a safety guard: when python3 is missing it allows
# the edit and says so on stderr (CI's harness lint is the backstop).
# Exit 0 allows. Exit 2 blocks, with one BLOCK/File/Detail/Fix stanza per finding.

set -euo pipefail

if ! command -v python3 >/dev/null 2>&1; then
  echo "validate-agent-config: python3 not found, validation skipped" >&2
  exit 0
fi

# shellcheck disable=SC2016  # the Python program is literal text
exec python3 -c '
import json, os, re, subprocess, sys

MODELS = ("opus", "sonnet", "haiku")
NAME_RE = re.compile(r"^[a-z0-9]+(-[a-z0-9]+)*$")
HOOK_EVENTS = {
    "PreToolUse", "PostToolUse", "PostToolUseFailure", "PermissionRequest",
    "UserPromptSubmit", "Notification", "Stop", "SubagentStart", "SubagentStop",
    "PreCompact", "SessionStart", "SessionEnd",
}

findings = []


def block(path, detail, fix):
    findings.append((path, detail.splitlines()[0] if detail else "invalid", fix))


def load_payload():
    try:
        return json.load(sys.stdin)
    except ValueError:
        return None


def proposed_content(payload, path):
    tool = payload.get("tool_name") or ""
    ti = payload.get("tool_input") or {}
    if tool == "Write" and "content" in ti:
        return ti.get("content") or ""
    try:
        with open(path, encoding="utf-8") as f:
            disk = f.read()
    except OSError:
        return None
    if tool == "Edit":
        old, new = ti.get("old_string", ""), ti.get("new_string", "")
        if old not in disk:
            return disk if payload.get("hook_event_name") == "PostToolUse" else None
        return disk.replace(old, new) if ti.get("replace_all") else disk.replace(old, new, 1)
    return disk


# ---- frontmatter -----------------------------------------------------------

def split_frontmatter(text):
    lines = text.split("\n")
    if not lines or lines[0].strip() != "---":
        return None
    for i in range(1, len(lines)):
        if lines[i].strip() == "---":
            return "\n".join(lines[1:i])
    return False  # opened, never closed


def unquote(v):
    v = v.strip()
    if len(v) >= 2 and v[0] == v[-1] and v[0] in "\"\x27":
        return v[1:-1]
    if v[:1] in "\"\x27":
        raise ValueError("unterminated quoted scalar: " + v)
    return v


def mini_yaml(text):
    """The frontmatter subset used here: scalars, flow lists, block lists, block scalars."""
    data, key, i = {}, None, 0
    lines = text.split("\n")
    while i < len(lines):
        line = lines[i]
        i += 1
        if not line.strip() or line.lstrip().startswith("#"):
            continue
        if line.startswith("\t"):
            raise ValueError("tab indentation")
        if line[0] in " -":
            item = line.strip()
            if key is not None and (item.startswith("- ") or item == "-"):
                if not isinstance(data.get(key), list):
                    data[key] = []
                data[key].append(unquote(item[1:]))
                continue
            raise ValueError("unexpected indentation: " + line.strip())
        m = re.match(r"^([A-Za-z0-9_-]+):(.*)$", line)
        if not m:
            raise ValueError("expected key: value, got: " + line.strip())
        key, val = m.group(1), m.group(2).strip()
        if val in ("|", ">", "|-", ">-", "|+", ">+"):
            block_lines = []
            while i < len(lines) and (lines[i].startswith(" ") or not lines[i].strip()):
                block_lines.append(lines[i].strip())
                i += 1
            data[key] = " ".join(x for x in block_lines if x)
        elif val.startswith("["):
            if not val.endswith("]"):
                raise ValueError("unterminated flow list for " + key)
            inner = val[1:-1].strip()
            data[key] = [unquote(x) for x in inner.split(",")] if inner else []
        elif val == "":
            data[key] = None
        else:
            data[key] = unquote(val)
    return data


def parse_frontmatter(text, path):
    try:
        import yaml
    except ImportError:
        yaml = None
    if os.environ.get("VALIDATE_AGENT_CONFIG_NO_YAML"):  # test seam for the fallback parser
        yaml = None
    try:
        if yaml is not None:
            data = yaml.safe_load(text)
        else:
            data = mini_yaml(text)
    except Exception as e:  # noqa: BLE001 - any parser error is a finding
        block(path, "YAML frontmatter does not parse: %s" % str(e).splitlines()[0],
              "Repair the frontmatter: check quotes, colons in values, and indentation.")
        return None
    if data is None:
        return {}
    if not isinstance(data, dict):
        block(path, "YAML frontmatter is not a mapping of key: value pairs",
              "Write the frontmatter as key: value lines.")
        return None
    return data


def as_list(v):
    if v is None:
        return []
    if isinstance(v, str):
        return [x.strip() for x in v.split(",") if x.strip()]
    return list(v) if isinstance(v, (list, tuple)) else [v]


def glob_error(p):
    if not isinstance(p, str):
        return "entry is %s, not a string" % type(p).__name__
    if not p.strip():
        return "entry is empty"
    if p.startswith("/") or p.startswith("~"):
        return "must be relative to the repository root"
    if ".." in p.split("/"):
        return "must not climb out of the repository with .."
    depth, i = 0, 0
    while i < len(p):
        c = p[i]
        if c == "\\":
            i += 2
            continue
        if c == "[":
            j = i + 1
            if j < len(p) and p[j] in "!^":
                j += 1
            if j < len(p) and p[j] == "]":
                j += 1
            while j < len(p) and p[j] != "]":
                j += 1
            if j >= len(p):
                return "unterminated [ character class"
            i = j + 1
            continue
        if c == "{":
            depth += 1
        elif c == "}":
            depth -= 1
            if depth < 0:
                return "unbalanced }"
        i += 1
    if depth:
        return "unbalanced {"
    return None


def check_name(fm, path, expected, what):
    name = fm.get("name")
    if not isinstance(name, str) or not name:
        return
    if not NAME_RE.match(name):
        block(path, "name %r is not lowercase letters, digits and hyphens" % name,
              "Rename it, for example %r." % re.sub(r"[^a-z0-9]+", "-", name.lower()).strip("-"))
    if name != expected:
        block(path, "name %r does not match the %s %r" % (name, what, expected),
              "Set name: %s so it matches the %s." % (expected, what))


def check_model(fm, path, required):
    if "model" not in fm:
        if required:
            block(path, "missing required field \x27model\x27",
                  "Pin the agent to a tier: model: opus, sonnet or haiku.")
        return
    if fm.get("model") not in MODELS:
        block(path, "model %r is not one of opus, sonnet, haiku" % (fm.get("model"),),
              "Use a tier alias (opus, sonnet or haiku), never a dated model id.")


def check_required(fm, path, fields):
    for f in fields:
        v = fm.get(f)
        if v is None or (isinstance(v, str) and not v.strip()):
            block(path, "missing required field %r" % f, "Add %s: to the frontmatter." % f)


def validate_markdown(path, content, kind):
    fm_text = split_frontmatter(content)
    if fm_text is False:
        block(path, "frontmatter opened with --- is never closed", "Close the frontmatter with a --- line.")
        return
    if fm_text is None:
        if kind != "rule":
            block(path, "missing YAML frontmatter (required for %s files)" % kind,
                  "Start the file with a --- frontmatter block holding name and description.")
        return
    fm = parse_frontmatter(fm_text, path)
    if fm is None:
        return
    if kind == "agent":
        check_required(fm, path, ("name", "description"))
        check_model(fm, path, True)
        check_name(fm, path, os.path.basename(path)[:-3], "file name")
        tools = {t for t in as_list(fm.get("tools")) if isinstance(t, str)}
        denied = {t for t in as_list(fm.get("disallowedTools")) if isinstance(t, str)}
        for t in sorted(tools & denied):
            block(path, "tool %r is in both tools and disallowedTools" % t,
                  "Remove %r from one of the two lists." % t)
    elif kind == "skill":
        check_required(fm, path, ("name", "description"))
        check_model(fm, path, False)
        check_name(fm, path, os.path.basename(os.path.dirname(path)), "skill directory")
    elif kind == "rule":
        if "paths" in fm:
            paths = fm.get("paths")
            if isinstance(paths, str):
                paths = [paths]
            if not isinstance(paths, list) or not paths:
                block(path, "paths must be a non-empty list of globs", "Write paths: as a YAML list of globs.")
                return
            for p in paths:
                err = glob_error(p)
                if err:
                    block(path, "paths entry %r: %s" % (p, err),
                          "Fix the glob in the paths list (repository-relative, balanced [] and {}).")


# ---- settings, JSON, shell -------------------------------------------------

def repo_root_of(path):
    marker = os.sep + ".claude" + os.sep
    idx = path.rfind(marker)
    return path[:idx] if idx >= 0 else os.path.dirname(path)


def validate_json(path, content):
    try:
        data = json.loads(content)
    except ValueError as e:
        block(path, "not valid JSON: %s" % e, "Repair the JSON syntax (jq . <file> shows the error).")
        return
    if not re.search(r"settings(\.local)?\.json$", path) or not isinstance(data, dict):
        return
    hooks = data.get("hooks")
    if hooks is None:
        return
    if not isinstance(hooks, dict):
        block(path, "hooks must be an object keyed by event name", "Write hooks as {\"PreToolUse\": [...]}.")
        return
    root = repo_root_of(path)
    for event, groups in hooks.items():
        if event not in HOOK_EVENTS:
            block(path, "unknown hook event %r (event names are case-sensitive)" % event,
                  "Use one of: %s." % ", ".join(sorted(HOOK_EVENTS)))
        for group in groups if isinstance(groups, list) else []:
            for h in (group.get("hooks") or []) if isinstance(group, dict) else []:
                cmd = h.get("command") if isinstance(h, dict) else None
                if not isinstance(cmd, str) or not cmd.strip():
                    if isinstance(h, dict) and h.get("type", "command") == "command":
                        block(path, "a %s hook has no command" % event, "Give the hook a command.")
                    continue
                first = cmd.split()[0].strip("\"\x27")
                for prefix in ("$CLAUDE_PROJECT_DIR/", "${CLAUDE_PROJECT_DIR}/"):
                    if first.startswith(prefix):
                        first = os.path.join(root, first[len(prefix):])
                if "/" not in first:
                    continue
                if not os.path.isabs(first):
                    first = os.path.join(root, first)
                if not (os.path.isfile(first) and os.access(first, os.X_OK)):
                    block(path, "hook command is not an executable file: %s" % cmd,
                          "Create the script, chmod +x it, or fix the path.")


def validate_shell(path, content):
    try:
        r = subprocess.run(["bash", "-n"], input=content, capture_output=True, text=True, timeout=10)
    except (OSError, subprocess.TimeoutExpired):
        return
    if r.returncode != 0:
        block(path, "bash -n reports a syntax error: %s" % r.stderr.strip(),
              "Repair the shell syntax (bash -n <file> shows the error).")


def main():
    payload = load_payload()
    if not isinstance(payload, dict):
        return 0
    path = (payload.get("tool_input") or {}).get("file_path") or ""
    norm = path.replace(os.sep, "/")
    if "/.claude/" not in norm:
        return 0
    rel = norm[norm.rfind("/.claude/") + len("/.claude/"):]
    parts = rel.split("/")
    if len(parts) == 2 and parts[0] == "agents" and parts[1].endswith(".md"):
        kind = "agent"
    elif len(parts) == 3 and parts[0] == "skills" and parts[2] == "SKILL.md":
        kind = "skill"
    elif len(parts) == 2 and parts[0] == "rules" and parts[1].endswith(".md"):
        kind = "rule"
    elif len(parts) == 2 and parts[0] == "hooks" and parts[1].endswith(".sh"):
        kind = "shell"
    elif len(parts) == 1 and parts[0].endswith(".json"):
        kind = "json"
    else:
        return 0
    content = proposed_content(payload, path)
    if content is None:
        return 0
    if kind == "shell":
        validate_shell(path, content)
    elif kind == "json":
        validate_json(path, content)
    else:
        validate_markdown(path, content, kind)
    if not findings:
        return 0
    out = []
    for p, detail, fix in findings:
        out.append("BLOCK: validate-agent-config\nFile: %s\nDetail: %s\nFix: %s" % (p, detail, fix))
    sys.stderr.write("\n\n".join(out) + "\n")
    return 2


sys.exit(main())
'
