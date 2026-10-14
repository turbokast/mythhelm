---
paths:
  - "scripts/**"
  - ".claude/hooks/**"
---

# Harness Scripts

Conventions for shell and Python under `scripts/` and `.claude/hooks/`. Hook specifics (payloads, exit codes, the tokenizer, arming) are in `.claude/hooks/README.md`.

- **Header.** `#!/usr/bin/env bash` (or `python3`), `set -euo pipefail`, and a comment stating what the script does, its arguments, exit codes, side effects and known residuals.
- **Portable.** Bash 4+, git, jq, python3 3.10+ standard library, coreutils. Contributors run macOS and Linux: no GNU-only flags without a fallback, no interval expressions (`{2,}`) in awk, which `mawk` lacks, and no `sed -i`, whose syntax differs between GNU and BSD (write a temporary file and move it).
- **Location-independent.** Resolve the repository with `git rev-parse --show-toplevel` or from the script's own path, and honour `$CLAUDE_PROJECT_DIR` in hooks. Never hard-code an absolute path.
- **Quoted and data-safe.** Quote every expansion. No `eval`. Treat file contents and tool output as untrusted data.
- **Fail closed** where the script guards something; exit non-zero on any unparseable input it depends on.
- **Tested.** Every script has a `test_<name>.sh` in the `tests/` directory beside it (hooks: `.claude/hooks/tests/`), run by `.claude/hooks/tests/run-tests.sh`, with the broken inputs the script must reject (`teeth-discipline.md`). Tests run against fixture repositories in temporary directories, never the real tree's `.claude/data/`.
- **Clean.** `shellcheck` passes on every `.sh` with no blanket disables; a targeted `# shellcheck disable=SCxxxx` carries a reason.
- **Readable output.** Machine-consumed output is JSON or `key=value` lines; findings are `path:line: category: detail`; blocks use the `BLOCK:`/`Detail:`/`Fix:` stanza.

## Browser measurement exception

Exactly `scripts/ci/browser/` may contain Node programs and npm manifests. All
other `scripts/**` remain Bash or Python standard library; no Node command or
JavaScript file is allowed there. Rendered layout, contrast and browser requests
need a real browser (docs-site-restyle AC-7.6, design D7); standard-library HTML
parsing cannot measure them. The pinned `playwright-core` driver uses the runner's
preinstalled Google Chrome, prints its version, and fails if it is absent.

Install only with the committed lockfile and `npm ci`. The browser fixtures run
only in the `ci.yml` site-build job after installation (Task 10 wires that job),
not the portable harness suite. The colocated `test_check_layout.sh` enumerates
the rest of `scripts/` and rejects any expansion of this exception. The resolved
tree contains `playwright-core` 1.64.0 (Apache-2.0), checked against the existing
dependency-review allow-list before implementation; no transitive packages.
