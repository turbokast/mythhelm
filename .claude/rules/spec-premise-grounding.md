---
paths:
  - "specs/**"
  - ".claude/skills/spec*/**"
  - ".claude/skills/create-spec/**"
  - ".claude/skills/refine-spec/**"
  - ".claude/skills/evaluate-spec/**"
---

# Spec Premises Are Grounded at Source

A claim a spec makes about the code is read many times and re-checked by nobody. Ground each one when it is written.

- **Cite every codebase claim.** A statement of what the code does, where something lives, or how many sites exist carries a `file:line` citation or a command with its observed output. A claim with neither is removed or moved to Open Questions.
- **Read what derives the value.** Ground a claim in the function that computes it, not in the branch that uses it, a comment, a name or a heading. Comments drift from code silently.
- **An absence claim needs structural grounding.** "X does not exist", "nothing calls Y" or "no path skips Z" is grounded only by enumerating the structure that would contain X (every caller, every top-level directive, every write site) and showing X is not among them. A keyword `grep` with no hits proves only that one spelling is absent, and `grep … || echo none` also prints `none` when the path is wrong. When a grep does hit near the subject, open the enclosing section before dismissing the hit.
- **Counts are derived, not remembered.** "Two callers", "three writers": run the search, and record the command and the number beside the claim.
- **Verdicts for inherited claims.** When a spec starts from an issue or an older document, record one verdict per load-bearing claim (the mechanism, each cited location, whether the failure is reachable, whether an acceptance check can be made to fail today): `HOLDS`, `PARTIAL` (true in part; record the drift), `REFUTED` (false; the spec is not built on it until the source text is corrected) or `UNVERIFIABLE` (goes to Open Questions, never assumed).
- **Re-ground stale citations.** A citation older than the last change to the file it cites is re-checked before the spec moves to the next state.

Evidence: `knowledge/rule-evidence/spec-premise-grounding.md`.
