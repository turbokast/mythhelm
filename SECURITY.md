# Security Policy

MYTHHELM supervises coding agents that can read, write and execute on your machine, so we take security reports seriously.

## Reporting a vulnerability

**Don't open a public issue, discussion or pull request.** Report privately through either route:

1. **Preferred:** [GitHub private vulnerability reporting](https://github.com/turbokast/mythhelm/security/advisories/new)
2. **Email:** security@turbokast.com

Include the affected version or commit, reproduction steps, and the impact you observed. A proof of concept helps but isn't required.

## What to expect

This is a volunteer project, so we don't promise a response-time SLA. We'll acknowledge your report, keep you updated as we investigate, and credit you in the advisory unless you ask us not to. We'll coordinate a disclosure date with you.

Fixes are released as expedited patch releases, and security advisories are published through [GitHub Security Advisories](https://github.com/turbokast/mythhelm/security/advisories) with the affected versions noted.

## Supported versions

MYTHHELM has no release yet. Once releases exist, security fixes will target the latest minor release. This section will list the supported range.

## Scope

Examples of issues in scope:

- Bypassing approval, admission or budget enforcement.
- Repository content, model output or plugin messages gaining permissions they shouldn't have.
- Credential or secret exposure.
- Terminal escape or log injection.
- Path traversal out of managed workspaces.

Vulnerabilities in third-party agents themselves should go to their vendors.
