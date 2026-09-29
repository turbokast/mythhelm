---
name: pm-sync-core
description: The canonical product-sync mechanism — stage, draft and file a product change for maintainer approval, and the lifecycle sync that moves one spec's backlog cards to specced, implementing or shipped with a decision entry
argument-hint: "<spec-name> --to specced|implementing|shipped [--premise \"<verdict line>\"]"
---

# PM Sync Core

Two things live here, and every product skill uses the first:

- **§File a change**: how any change to `product/` is drafted, validated and filed for a maintainer's approval. It is the single place that procedure is written down.
- **§Lifecycle sync**: the mechanical status transition for one spec's cards when the spec lifecycle moves: `/create-spec` and `/spec` sync to `specced`, `run-spec` to `implementing`, `finalize-spec` to `shipped`.

Agents never write `product/` directly. `.claude/hooks/guard-product-write.sh` releases a write only against a maintainer's signed approval of that exact change (`product/README.md`, `.claude/rules/product-management.md`).

## Input

`$ARGUMENTS` for the lifecycle sync: the spec name, `--to <status>`, and for `specced` an optional `--premise "<line>"` recording the grounding verdicts on the card. Other skills call §File a change with their own drafts.

## Invocation contexts

- **Slash command**: runs the sync, files the requests and prints the maintainer's approve commands.
- **Model-invoked**: the same; a calling skill's run continues after the requests are filed.
- **Non-interactive**: the same. Filing a request writes only to `orchestration/requests/`, never to `product/`, so it needs no pre-approval; the product write waits for the maintainer's signature in every context. A step that needs a decision the algorithm does not encode is an escalation: stop and report it.

## File a change

1. **Stage.** `STAGE="$(mktemp -d)"; python3 scripts/pm/pm.py stage "$STAGE"`. This copies the five product files; nothing under `product/` changes.
2. **Draft.** Run each `pm.py` write verb against the stage: `python3 scripts/pm/pm.py --product-dir "$STAGE" <verb> ... --out "$STAGE/<file>"`. Chain as many as the change needs; each reads the staged files and writes one back. A new card or entry id comes from the verb itself, which reserves it with `pm.py next-id` at this moment; never type an id you remember, and never put a number in a draft you show before this step.
3. **Record the decision.** Every change except a pure roadmap redraft appends a `decisions.md` entry with `pm.py ... decide --type <type>`, naming the cards and the evidence.
4. **Redraft the roadmap** when a card's status or score changed: `pm.py --product-dir "$STAGE" roadmap --out "$STAGE/roadmap.md"`.
5. **Validate** the whole staged set: `python3 scripts/pm/pm.py --product-dir "$STAGE" validate` must print `product: OK` (a stale-roadmap warning means step 4 was skipped). The verbs refuse a draft that would not validate, so a failure here is a finding to fix, not to file.
6. **Check the diff is proportional**: `diff -u product/<file> "$STAGE/<file>"` for each file. A status flip is a few lines; if unrelated cards moved or changed, stop and find out why before filing.
7. **File one request per changed file**, ids `<slug>-<file stem>`:
   `python3 scripts/orchestration/approvals.py request <slug>-backlog --path product/backlog.md --proposed "$STAGE/backlog.md" --summary "<what and why>"`.
8. **Hand off.** Report the request ids and the maintainer's commands: `scripts/orchestration/approve.sh show <id>`, then `approve <id> --apply` (writes the file) or `reject <id>`. Continue with independent work; never wait by polling.
9. **Apply, if asked.** When the maintainer approved without `--apply`, write each file with the Write tool using exactly the content of `orchestration/requests/<id>/proposed` in the main checkout. A block saying the approval is stale means the file changed since the request: restage and refile from step 1; never adapt the old draft by hand.
10. **Commit** the applied files by name (`git commit -s -m "<msg>" -- product/<file> ...`) on a branch, and open or update a pull request.

## Lifecycle sync

Zero-judgment bookkeeping. A dispatcher may hand it to `harness-clerk`, passing no `effort` parameter (`knowledge/agent-routing.md`).

1. **Resolve the spec** with `scripts/harness/spec-lifecycle.sh resolve <spec-name>`.
2. **Find its cards**, as a set: every `- **Backlog card**: MH-<n>` line in the spec's `requirements.md` (or in `plan.md` for an epic), plus every card whose Spec field names the spec (`python3 scripts/pm/pm.py list --json` and read the `spec` field). No card: report `PM sync: skipped (no card names <spec-name>)` and stop. Never invent a link.
3. **Skip what is already there**, per card: `specced` skips a card already `specced`, `implementing` or `shipped`; `implementing` skips `implementing` or `shipped`; `shipped` skips `shipped`. A `dropped` card is an escalation: a spec is moving for work the backlog says will not be done.
4. **Stage** (§File a change step 1) and, for each remaining card, draft in the stage:
   - `specced`: `set-status MH-<n> specced --spec <spec-name>` (the Spec field keeps any other specs it names; pass them all, comma-separated). With `--premise`, also `set MH-<n> premise-grounded "<line>"`.
   - `implementing`: `set-status MH-<n> implementing`.
   - `shipped`: `set-status MH-<n> shipped`. When the card names another spec that is not yet in `done/` or `archived/`, pm.py refuses and exits 1. Then the card stays `implementing`: draft `set MH-<n> half-shipped "<spec-name> shipped in <PR link>; the card ships with <remaining specs>"` instead and report it as held.
5. **Decision entries**, one per changed card: `decide --type lifecycle-sync --title "MH-<n> → <status> (<spec-name>)" --decision "<card> moved to <status>" --rationale "<spec-name> reached <spec state>" --cards MH-<n>` with `--evidence` naming the pull request or CI run when there is one.
6. **Roadmap, validate, file and hand off**: §File a change steps 4 to 8, request slug `sync-<spec-name>-<status>`.
7. **Report**, one line per card found in step 2, so the count can be checked against the set: `PM sync: MH-<n> → <status> (requests <ids>)`, `PM sync: MH-<n> skipped (already <status>)` or `PM sync: MH-<n> held at implementing (half shipped; waits for <specs>)`. After a `shipped` sync, add: `Consider /impact-review once the change has been in use.`

## Output

```text
PM sync: <one line per card>
Requests: <ids>, for the maintainer: scripts/orchestration/approve.sh show <id> && scripts/orchestration/approve.sh approve <id> --apply
```
