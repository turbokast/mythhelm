---
paths:
  - "knowledge/**"
---

# Knowledge File Conventions

- `knowledge/` holds facts and reasons, never instructions. A "do" or "never" sentence belongs in a rule or an agent definition, which links here for the why.
- Cite the source of every claim: a spec section (`§13.10`), a file path, a check name or an issue link. A claim nobody can re-check is removed.
- Never duplicate what the code, `git log` or the spec states. Link to it. `invariants.md` is the one exception to "no instructions": it restates the spec's normative §4 invariants, which are requirements by nature, in checkable form, and defers to §4 on any difference.
- Keep machine-read shapes intact. `agent-routing.md` tables are parsed by `scripts/ci/lint-agent-harness.sh`; run it after editing them.
- Evidence files under `knowledge/rule-evidence/` follow the template in its `README.md`, one per rule, named after the rule. Record instances only when they link to something public in this repository.
- Public by default (`public-repo-hygiene.md`): generic hazards, no private incidents, people, hostnames or dates of rulings.
- When a fact changes, update it in the same pull request as the change that makes it stale.
