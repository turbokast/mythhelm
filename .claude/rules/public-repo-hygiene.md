# Public Repository Hygiene

Everything committed here is world-readable, forever: history is public even after a file is deleted. Before writing any file, commit message, pull request body, issue or review reply, check it against this list.

Never write:

- secrets, tokens, keys, or where a credential is stored beyond "the vendor's native store";
- email addresses other than the project's published contacts in `SECURITY.md` and `MAINTAINERS.md` (commit trailers excepted);
- `/home/<user>` or `/Users/<user>` paths, hostnames, IP addresses or machine identifiers;
- material from customers, prospects, employers or other private projects, including their names, incidents and internal identifiers;
- business metrics, raw session transcripts, telemetry, or native agent output copied from a real run.

Use instead: `example.com` addresses, RFC 5737 IPs (`192.0.2.x`), `$HOME`-relative or `/tmp/...` paths, and synthetic fixtures marked as synthetic. A recorded native stream is sanitised before it is committed (no account IDs, emails, org names, paths or prompts).

Runtime state stays out of git: `.claude/data/` is ignored except schemas and seeds. Never force-add an ignored file.

`scripts/ci/check-public-hygiene.sh` runs in CI and fails on the mechanical cases. It cannot recognise a private project's name or an incident description, so the rule binds beyond what it catches. If something private was committed, stop and tell a maintainer; do not try to rewrite history yourself.
