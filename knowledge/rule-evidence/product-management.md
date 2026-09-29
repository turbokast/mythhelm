# Evidence: product-management

Record for `.claude/rules/product-management.md`.

## Hazard

A product backlog maintained by agents fails in quiet ways, each of which looks like ordinary progress:

- **Self-approval.** An agent asked to "keep the backlog current" decides priorities itself. A conversational "sounds good" is taken as approval of a change the person never saw in full, and a record meant to capture human decisions fills with agent decisions.
- **Lost updates.** Product files are edited by several sessions and branches at once. A change prepared as a full-file copy and applied later silently reverts whatever landed in between; checking the working tree just before applying does not help, because another session can commit in the gap.
- **Colliding ids.** The gap between drafting a card and filing it is long when a person must approve it. Every session that computes "highest id plus one" from its own copy picks the same number, and the collision surfaces only at merge, after the id has been quoted in issues and specs.
- **Silent reordering.** An agent that rewrites a whole file to change one line reorders or reformats unrelated entries. The diff hides the one real change, and blame on every moved line points at the wrong change.
- **Premature closure.** A card delivered by two specs is marked shipped when the first ships, and planning that reads "shipped" as done drops the second.
- **Rewritten history.** A decision log edited in place no longer shows why the backlog looks as it does, and a later reader grounds new decisions on a record that was changed after the fact.
- **Ungrounded premises.** A card claims that something is missing or broken. Nobody re-checks the claim before a spec is built on it, and the claim was wrong, or was only true of one spelling a keyword search happened to test.
- **Guards that block reading.** A write guard that matches any command mentioning the product directory blocks `cat`, `grep` and `wc` as well, and agents learn to route around it, which defeats the guard it was.

## Mechanism

- `.claude/hooks/guard-product-write.sh` releases an agent write under `product/` only against a signed, unconsumed approval bound to the file's current hash and to the hash of the exact content being written, and it keeps the approval command, the ledger and the key out of an agent's reach. Binding the base turns a lost update into a stale approval; binding the result stops one approval from releasing a different edit or a deletion; consuming it makes it single-use. Its Bash arm allows read-only commands that mention product paths, with the false-positive class kept as asserted cases in `.claude/hooks/tests/test_guard_product_write.sh`.
- `scripts/pm/pm.py` drafts whole files from the current state, reserves ids across sessions and branches at filing time, renders the backlog in one canonical order, refuses to mark a card shipped while one of its specs is unfinished, and refuses to draft a change that would not validate.
- The `product` check of `scripts/ci/lint-agent-harness.sh` fails on duplicate ids, statuses outside the enum, a score that is not the formula, an urgency that does not match the stage, spec links that do not resolve, malformed issue links, a shipped card with unfinished specs, a backlog out of canonical order, and decision or signal entries out of append order. It also covers edits made outside Claude Code, which no hook sees.
- Premise grounding is prose plus the verdict line `/create-spec` records; judging whether a claim holds needs reading source, which no check can do.

## Instances

None recorded in this repository yet.

## Loosening criteria

Three recorded cases in this repository where the guard blocked a read-only command, or a legitimate change could not be expressed through a request, would justify widening the read-only grammar or adding a verb. A record of maintainers routinely approving mechanical lifecycle syncs unread would justify proposing a narrower, pre-approved class for them, with its own check.
