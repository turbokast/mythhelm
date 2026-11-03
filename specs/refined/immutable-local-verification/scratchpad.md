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
