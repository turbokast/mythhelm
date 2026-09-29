#!/usr/bin/env bash
# check-public-hygiene.sh: fails when the tree holds text that must never be
# published from this repository:
#
#   home-path     /home/<user>/ or /Users/<user>/ paths
#   email         addresses other than the project's published contacts,
#                 noreply@anthropic.com, git@github.com and example.com/.org/.net
#   private-key   PEM private-key headers
#   token         GitHub (ghp_ gho_ ghu_ ghs_ ghr_ github_pat_), GitLab (glpat-),
#                 Anthropic/OpenAI style (sk-, sk-ant-), AWS (AKIA) and Slack
#                 (xoxb- xoxp- xoxa- xoxr- xoxs-) credentials
#   host          *.fly.dev and *.internal hostnames
#   ipv4          IPv4 literals outside documentation (RFC 5737), loopback,
#                 private (RFC 1918), link-local and unspecified ranges
#
# Scans tracked files plus untracked files that are not ignored, skipping .git and
# LICENSE. A symlink is scanned as its target text, the data Git commits. Binary
# files (with a NUL byte) skip only the email and IPv4 checks. A listed file that
# cannot be read is a finding. Matched secrets are shown redacted.
#
#   scripts/ci/check-public-hygiene.sh [<root>]    default: the repository root
#
# Exit 0 when clean, 1 with one "path:line: category: match" line per finding.

set -euo pipefail

ROOT="${1:-$(git rev-parse --show-toplevel 2>/dev/null || pwd)}"
cd "$ROOT"

# shellcheck disable=SC2016  # the Python program is literal text
if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  git ls-files -z --cached --others --exclude-standard
else
  find . -path ./.git -prune -o -type f -print0 | sed -z 's|^\./||'
fi | python3 -c '
import ipaddress, os, re, sys

ALLOWED_EMAILS = {
    "security@turbokast.com",
    "liam@turbokast.com",
    "noreply@anthropic.com",
    "git@github.com",
}
ALLOWED_EMAIL_DOMAINS = ("example.com", "example.org", "example.net")
ALLOWED_NETS = [ipaddress.ip_network(n) for n in (
    "192.0.2.0/24", "198.51.100.0/24", "203.0.113.0/24",   # RFC 5737 documentation
    "127.0.0.0/8", "0.0.0.0/8",                              # loopback, unspecified
    "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16",         # RFC 1918 private
    "169.254.0.0/16",                                        # link-local
)]

PATTERNS = [
    ("home-path", re.compile(r"/(?:home|Users)/[A-Za-z0-9._-]+/")),
    ("private-key", re.compile(r"-----BEGIN (?:[A-Z0-9]+ )*PRIVATE KEY-----")),
    ("token", re.compile(
        r"\b(?:gh[pousr]_[A-Za-z0-9]{30,}"
        r"|github_pat_[A-Za-z0-9_]{30,}"
        r"|glpat-[A-Za-z0-9_-]{20,}"
        r"|sk-[A-Za-z0-9_-]{20,}"
        r"|AKIA[0-9A-Z]{16}"
        r"|xox[abprs]-[A-Za-z0-9-]{10,})")),
    ("host", re.compile(r"\b[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)*\.(?:fly\.dev|internal)\b")),
]
EMAIL = re.compile(r"\b[A-Za-z0-9._%+-]+@[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)*\.[A-Za-z]{2,}\b")
IPV4 = re.compile(r"(?<![\w.])(?:\d{1,3}\.){3}\d{1,3}(?![\w.]*\d)")


def redact(category, text):
    if category in ("token", "private-key"):
        return text[:6] + "..." if category == "token" else text
    return text


def email_allowed(addr):
    addr = addr.lower()
    domain = addr.rsplit("@", 1)[1]
    return addr in ALLOWED_EMAILS or any(
        domain == d or domain.endswith("." + d) for d in ALLOWED_EMAIL_DOMAINS)


def ip_allowed(text):
    try:
        ip = ipaddress.ip_address(text)
    except ValueError:
        return True  # not an address (an octet over 255): a version string or similar
    return any(ip in net for net in ALLOWED_NETS)


findings = 0
# Paths stay bytes, so a name that is not valid UTF-8 still opens.
paths = [p for p in sys.stdin.buffer.read().split(b"\0") if p]
for path in sorted(paths):
    shown = path.decode("utf-8", "replace")
    if path == b"LICENSE" or path.startswith(b".git/"):
        continue
    try:
        if os.path.islink(path):
            # Git commits the target text of a symlink, not the file it points to.
            raw = os.fsencode(os.readlink(path))
        elif not os.path.lexists(path):
            continue  # deleted in the working tree; nothing of it is scanned here
        else:
            with open(path, "rb") as f:
                raw = f.read()
    except OSError as e:
        print("%s: unreadable: %s" % (shown, e.strerror or e))
        findings += 1
        continue
    # A binary file (one with a NUL byte) is still scanned for paths, hostnames,
    # keys and tokens; only the email and IPv4 checks, which false-positive on
    # binary noise, are skipped for it.
    binary = b"\0" in raw
    for lineno, line in enumerate(raw.decode("utf-8", "replace").splitlines(), 1):
        hits = []
        for category, rx in PATTERNS:
            hits += [(category, m.group(0)) for m in rx.finditer(line)]
        if not binary:
            hits += [("email", m.group(0)) for m in EMAIL.finditer(line) if not email_allowed(m.group(0))]
            hits += [("ipv4", m.group(0)) for m in IPV4.finditer(line) if not ip_allowed(m.group(0))]
        for category, text in hits:
            print("%s:%d: %s: %s" % (shown, lineno, category, redact(category, text)))
            findings += 1

if findings:
    print("check-public-hygiene: %d finding(s). Remove them, or replace them with example.com addresses, "
          "RFC 5737 IPs (192.0.2.x) and $HOME-relative paths." % findings, file=sys.stderr)
    sys.exit(1)
print("check-public-hygiene: clean (%d files scanned)" % len(paths))
'
