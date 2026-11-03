# Immutable local verification inputs — Scratchpad

> Candidate design research. Each implementing task reads this file and appends discoveries; no implementation or profile qualification has occurred.

## Open questions

| # | Question | Default in this spec | Owner / blocks |
|---|---|---|---|
| Q1 | Which reviewed helper/tool digests and exact filesystem/kernel combinations are actually qualified at implementation time? | No enabled strong profile until strict actual qualification succeeds; record unavailable with reason. Never install/download or infer approval. | Harness implementer / Task 4 and any strong execution |
| Q2 | Can every supported helper/kernel enforce the declared scratch inode/byte limit and PID-namespace teardown? | Refuse strong qualification on missing boundary or unresolved ownership; preserve legacy checking and quarantine artifacts. | Harness implementer / Task 4 |
| Q3 | What new executable marker readers or overlapping gate changes exist at implementation baseline? | Rerun named structural inventories and preserve centralized acceptance, actual argv and current required gates; integrate before completion. | Task 5 implementer / consumer migration |

## Research notes

- Scope is one stream of eight tasks: a docs-domain ADR producer (Task 8) followed by seven harness tasks, auto-confirmed non-interactively. Requirements remain frozen; the third fresh validation returned Ready and the lifecycle is moving to todo.
- Baseline is `f18ff8c`. Current gatelib includes workflow dependency scopes, runtime/test assets, preflight gate ordering and four-worker harness test invocation. These are existing mechanisms to preserve.
- Actual local synthetic `gofmt -l .` traversed `.claude/data/private.go`, while excluding a dot-prefixed `.hidden.go` basename. Local Go source `isGoFilename` and `gofmtMain` confirm this distinction. The design refuses ignored actual consumer inputs instead of silently excluding them from verification.
- Current real gofmt initializes Go telemetry counters. Qualified execution clears ambient environment, disables telemetry and provides bounded scratch configuration/home/cache paths; it mounts no native/secret/config store.
- Existing vendor bubblewrap helper validates helper ownership but mounts state inappropriate here. Reuse the reviewed pattern, not its vendor environment or writable mounts.
- Endpoint identity is an observed comparison; the copy's actual bytes and read-only execution boundary supply the stronger execution-input statement. No same-user hostile-host or atomic dirty-checkout history is certified.
- Future Git-dependent, test/cache-writing, source-transform and other-platform profiles remain explicitly legacy-observed/strong-unavailable. A static Go formatter profile earns none of their qualification.

## Discoveries

None from implementation yet.

## Independent design validation

### Round 1 — Needs revision

- Assessed signed source: `16df4954fea71eb60884fcf87a63f2db76568122`.
- Fresh reviewer: `immutable_design_review` (independent fresh context).
- Accounting: `dispatched=1 returned=1 failed=0`.
- References, design and tasks: NEEDS_WORK. Requirements and conflicts: PASS. Epic: not applicable (single spec).
- Blocking finding 1: mandated existing `listed_files` uses buffered Git capture with a 120-second timeout before proposed 20,000-entry/30-second bounds. Repair separates a bounded incremental strong observer, includes root/HEAD/base/index/worktree observations in one deadline, bounds records/stdout/stderr, and requires owned-process teardown plus excessive-inventory/blocked-child controls. Legacy helper behavior is preserved.
- Blocking finding 2: closed profile/qualification/probe schemas omitted the promised checker-definition binding. Repair adds explicit current definition throughout the acyclic plan/probe/qualification/profile chain and execution, with new-candidate/old-qualification refusal controls in codecs, executor and every consumer.
- Original attempt remains rejected. These author repairs are a candidate for a new independent round; no Ready verdict or qualification is inferred. Requirements and refinement-log are unchanged.

### Round 2 — Needs revision

- Assessed signed source: `02008c1ada44b7972926a611f63c5513b521e57f`.
- Fresh reviewer: `immutable_design_review_r2`.
- Cumulative accounting: `dispatched=2 returned=2 failed=0`.
- References, requirements and conflicts: PASS. Design and tasks: FAIL. Epic: not applicable.
- Both Round 1 blockers passed fresh re-review. The remaining blocker was the missing explicitly owned ADR for durable private snapshot persistence, ownership, crash reconciliation and retention; repository authoring rules provide no product-only exception.
- Repair adds docs-domain Task 8 producing `docs/decisions/0023-immutable-local-verification.md`, independently reviewed and accepted before Task 1. Original task IDs 1–7 remain stable; DAG is 8→1→2→3→4→5→6→7. The ADR covers actual decision alternatives and all persistence/ownership and immutable/legacy boundaries, without claiming actual qualification. Current decision inventory ends at 0022; its README has naming/template guidance and requires no index change.
- Round 2 remains rejected; this repair awaits fresh Round 3. No Ready verdict, actual ADR acceptance, implementation or qualification is inferred. Requirements and refinement-log remain unchanged.


### Round 3 — Ready for implementation

- Assessed signed source: `1ebabd6a500613665ecdad9f1ca56d74fe81b7fa`; fresh reviewer `immutable_design_review_r3`.
- Cumulative accounting: `dispatched=3 returned=3 failed=0`; references, requirements, design, tasks and conflicts PASS; single-spec epic check not applicable. No blocking or advisory findings.
- The independently checked32 task paths have existing parents and no overlap with current todo/in-progress task file plans. The eight-task DAG remains8→1→2→3→4→5→6→7; all22 AC IDs are unique and traceable. References/specs lint passes.
- Task8 owns the reviewed/accepted/merged ADR before implementation; no actual ADR acceptance, tool/profile qualification, strong verification result, native call or implementation is established by this design. The first two rejected rounds and their separate bounded repairs remain recorded above.
