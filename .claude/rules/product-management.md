---
paths:
  - "product/**"
  - "scripts/pm/**"
  - "scripts/orchestration/**"
  - "orchestration/**"
  - ".claude/hooks/guard-product-write.sh"
  - ".claude/skills/{backlog,triage,roadmap,synthesize-signals,impact-review,quarterly-review,pm-sync-core,create-spec}/**"
  - "knowledge/product-management.md"
---

# Product Management

`product/` decides what MYTHHELM builds next and records why ([`product/README.md`](../../product/README.md)). The facts behind these rules are in `knowledge/product-management.md`.

## Agents propose; maintainers approve

- Change a product file only by drafting the whole new file with `scripts/pm/pm.py` and filing it with `python3 scripts/orchestration/approvals.py request` (`.claude/skills/pm-sync-core/SKILL.md` §File a change). Write it only after a maintainer has signed that request, and write exactly its proposed content.
- A person's "yes" in the conversation is not an approval. The approval is the signature they make from their own terminal with `scripts/orchestration/approve.sh`. Never run `approve.sh`, never run the maintainer verbs of `approvals.py`, never write `orchestration/approvals.jsonl`, and never read or write the approval key.
- Never write product files through Bash: no redirects, `cp`, `sed -i`, interpreters or scripts, even when a command would get past `guard-product-write.sh`. A blocked product write is filed as a request, never retried in another spelling.
- Build every draft from the current file through `pm.py stage`, never from an older copy. When an approval is reported stale, restage and refile; never patch the old draft by hand.
- Check that each draft's diff is proportional to the change before filing it. A status flip that also moves unrelated cards is a defect in the draft.

## Records

- Take a card, decision or signal id only from the `pm.py` verb that files it, which reserves it. A draft shown before filing says `MH-N`. Never reuse an id, including a dropped card's.
- `decisions.md` and `signals.md` are append-only. Never edit or remove an entry; a reversal is a new entry that names the one it supersedes.
- Never delete a card. Close it as `shipped` or `dropped`, with a decision entry.
- A card is `shipped` only when every spec it names is in `specs/done/` or `specs/archived/`. Until then it stays `implementing` with a `Half shipped` note.
- Never hand-edit a score. Change its inputs with `pm.py rescore`; urgency always comes from the card's stage and the current stage.

## Premise grounding

- Before a card is filed or specified, ground each load-bearing claim in its Summary and in its source issue at source, with one verdict each: `HOLDS`, `PARTIAL` (write down the drift), `REFUTED` or `UNVERIFIABLE` (`.claude/rules/spec-premise-grounding.md`).
- A claim that something does not exist yet, or that nothing handles a case, is grounded only structurally: enumerate where it would live (the package, the command table, the registry, every caller) and show it is absent. A keyword search with no hits proves only that one spelling is absent.
- A `REFUTED` claim blocks the card until its text is corrected, and the correction is a product change like any other. An `UNVERIFIABLE` claim goes to the spec's Open Questions.

## Public by default

Cards, decisions, signals and issues are public. Paraphrase what reporters wrote; never copy personal details, logs, tokens or private context from an issue, and never record business metrics (`.claude/rules/public-repo-hygiene.md`).

Evidence: `knowledge/rule-evidence/product-management.md`.
