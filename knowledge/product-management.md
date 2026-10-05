# Product management

How the product layer works and why it is shaped this way. The rules agents follow are in `.claude/rules/product-management.md`; the files, their schemas and the approval flow are introduced in [`product/README.md`](../product/README.md). Skills: `/backlog`, `/triage`, `/roadmap`, `/synthesize-signals`, `/impact-review`, `/quarterly-review`, `/pm-sync-core` and `/create-spec`.

## Where it sits

The [specification index](../docs/spec/README.md) resolves the current normative design: [Master Specification v2](../mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md). `product/` decides the order in which to build it and records why. `specs/` says how each piece is built (`specs/README.md`). Historical specs and closed cards keep their named source revision; the [adoption map](../docs/spec/synthesis-adoption.md) explains reconciliation and W01–W16 coverage. A card becomes a spec through `/create-spec MH-<n>`; the spec lifecycle then moves the card:

| Spec lifecycle event | Skill | Card status |
|---|---|---|
| The card is scored and accepted | `/backlog add` | `triaged` (`idea` when unscored) |
| A spec is created from the card | `/create-spec` | `specced` |
| The first task starts | `run-spec` | `implementing` |
| The spec is finalized | `finalize-spec` | `shipped`, once every spec the card names is in `done/` or `archived/` |
| The work will not be done | `/backlog drop` | `dropped` |

Each transition is filed by `.claude/skills/pm-sync-core/SKILL.md` §Lifecycle sync, with a `lifecycle-sync` decision entry. A spec finds its cards through `- **Backlog card**: MH-<n>` lines in its requirements (`knowledge/spec-authoring.md`), and a card finds its specs through its Spec field.

## Objectives come from the spec

`product/objectives.md` indexes v2 stages S0–S6 (§18.1), release gates G01–G16 (§18.3), and commitments derived from §1. The current stage is a prioritisation focus, not a declaration that preceding gates passed. Adoption retains the existing stage 1 focus while S0 route qualification remains unproven. A stage change requires evidence and a maintainer decision (`objective-change`). S6 production delivery can proceed from S1 independently of adaptive S4/S5; stable version 1.0 is a release decision, not a stage number.

## Scoring

Weighted shortest job first: `score = (value + urgency + risk) / effort`, each input 1 to 5, rounded half up to one decimal. The rubric is at the top of `product/backlog.md`.

- **value** is user value for the product the spec describes.
- **urgency** is derived from the stage: 5 for the current stage or earlier, 3 for the next, 2 for the one after, 1 beyond or `later`. This is a priority heuristic across v2 capability stages, not an execution dependency graph or a calendar. A stage advance changes affected urgencies, which explains the paired rescore in the [product rules](../.claude/rules/product-management.md#records).
- **risk** is the risk the card retires: an invariant it enforces (`knowledge/invariants.md`) or an unproven boundary the spec depends on.
- **effort** follows the size of comparable specs: about three tasks per point.

The formula favours small jobs. A one-point card with modest value can outrank the slice in flight, which is the intent: cheap work that clears a gate should not wait behind an epic. Dependencies in a card's Summary/Notes and the adoption map still govern readiness. `pm.py next` ranks candidates; it does not certify that their prerequisites, native qualifications or implementation approvals are satisfied.

## Operational evidence and optional learning

V2 §§5, 8 and 12 distinguish essential local operational state from optional learning. Admission, recovery, verification and truthful receipts require durable state even with learning off. Learning collection, historical backfill, exploration and external telemetry have separate authority; all are off by default. The product-planning layer does not ingest private operational data. Its evidence is public or explicitly supplied for that purpose:

- **Signals**: GitHub issues, discussions and pull-request feedback, read with `gh` by `/synthesize-signals` and grouped into themes in `product/signals.md`.
- **Impact**: CI history on `main`, version-qualified evaluation and acceptance results (v2 §§13 and 18), and reports after a card shipped, gathered by `/impact-review` and recorded as an `impact-review` decision. Planned tests and vendor documentation are not passing qualification evidence.
- **Health**: `/quarterly-review` reads the same sources over a quarter.

## The approval mechanism

`scripts/orchestration/approvals.py` provides the shared "agents propose, maintainers approve" mechanism. The Claude hook adds immediate enforcement in that client; the approval boundary is shared by every client. Scratch drafts leave product files unchanged until the exact proposed content receives a maintainer signature. The [product rules](../.claude/rules/product-management.md#agents-propose-maintainers-approve) and [PM Sync Core procedure](../.claude/skills/pm-sync-core/SKILL.md#file-a-change) define staging, drafting, validation and filing.

- **A request is a whole proposed file.** It records the SHA-256 of the file as it is (the base), of the proposed content (the result) and of the diff. A whole file rather than a patch keeps the approval decidable: the hook can compute exactly what a Write or Edit would leave and compare hashes.
- **The approval binds base and result.** Binding the base means a change that lands between request and write (another session, another branch) makes the approval stale instead of being overwritten. Binding the result means an approval releases exactly the reviewed content: not a different edit, and not a deletion of the file.
- **One approval, one write.** The hook records each release as a `consumed` row, and a consumed approval never releases again.
- **Signed by the maintainer.** Decision rows carry an HMAC-SHA256 under a key outside the repository and are chained, so `approve.sh audit` detects a hand-written, altered, removed or reordered row. The maintainer verbs refuse to run without an interactive terminal, and the hook blocks agents from running them, from writing the ledger and from reading the key.
- **One queue per machine.** The ledger and requests live in the main checkout's `orchestration/`, found through the git common directory, so every linked worktree shares them.

Residuals, stated in the hook's header: the key is a file the same user can read; a pseudo-terminal passes the terminal test; the Bash arm reads command text and cannot see a write made by a script, an interpreter's computed path or a pipe to `xargs`. The hook exists to stop mistakes in agent sessions. Pull request review and the `product` lint, which also cover human edits and other agents, are the backstop.

## Ids and ordering

- **Ids are reserved at filing.** `pm.py next-id` takes one more than the highest id in the working tree, local `main`, `origin/main`, every pending request and every earlier reservation, under a lock in the main checkout's `.claude/data/id-reservations.jsonl`. Drafting and approval can be far apart, and several sessions draft at once; reserving at filing across all of those sources is what keeps two of them from choosing the same id. Holes are expected and never reused.
- **The backlog has one canonical form.** Open cards by score then id, closed cards by id, fields in schema order. `pm.py` writes that form and the lint rejects anything else, so a reorder or reformat cannot slip into a diff unnoticed, and the rendered order is always the priority order.

## Deliberately absent

- No personas, business plans or revenue data: this is an open-source project with no sales funnel. Commitments come from v2 §1.
- No standing lifecycle-sync exemption: mechanical status flips go through an approval like any change, because the ledger is only meaningful if every product write is in it. The one pre-approved class is time-boxed and explicit: while the maintainer's autonomy grant carries `--allow-pm-sync`, the granted session's specced → implementing → shipped moves of granted cards, their `lifecycle-sync` entries and the regenerated roadmap are released without a signature and recorded as `preapproved` rows (`knowledge/autonomy.md`).
- No GitHub milestones or projects. The card is the plan of record and its issue is the public discussion; a second planning surface would drift from both.
