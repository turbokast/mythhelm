# Immutable local verification inputs — Scratchpad

> Candidate design research. Each implementing task reads this file and appends discoveries; no implementation or profile qualification has occurred.

## Open questions

| # | Question | Default in this spec | Owner / blocks |
|---|---|---|---|
| Q1 | Which reviewed helper/tool digests and exact filesystem/kernel combinations are actually qualified at implementation time? | No enabled strong profile until strict actual qualification succeeds; record unavailable with reason. Never install/download or infer approval. | Harness implementer / Task 4 and any strong execution |
| Q2 | Can every supported helper/kernel enforce the declared scratch inode/byte limit and PID-namespace teardown? | Refuse strong qualification on missing boundary or unresolved ownership; preserve legacy checking and quarantine artifacts. | Harness implementer / Task 4 |
| Q3 | What new executable marker readers or overlapping gate changes exist at implementation baseline? | Rerun named structural inventories and preserve centralized acceptance, actual argv and current required gates; integrate before completion. | Task 5 implementer / consumer migration |

## Research notes

- Scope is one seven-task harness stream, auto-confirmed non-interactively. Requirements are frozen; candidate remains refined pending a root-dispatched fresh reviewer.
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
