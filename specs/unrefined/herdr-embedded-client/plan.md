# Herdr embedded client — Master Plan

## Problem Summary

Provide an existing-run MYTHHELM client, a minimal authenticated core projection and trusted navigation inside Herdr. Presentation teardown never launches, recovers, stops or transfers ownership. This is MH-3's first slice, not native writable takeover or complete G11. Normative source: Master Specification v2 §§3–6, 8, 14–15 and 18; source refined requirements `specs/*/herdr-bridge/requirements.md` remain unchanged.

## Current State

Grounding: main `61d18e2`. `internal/control/client.go:168` (`rg -n 'func RunSupervisor' internal/control/client.go`) owns the instance but directly serves the six-method registry. `internal/control/server.go:178` (`rg -n 'func NewSupervisorServer' internal/control/server.go`) omits stop/recover because its isolated handlers do not ingest the pipeline spool. `internal/cli/run.go:218` (`rg -n 'func executeRun' internal/cli/run.go`) still admits/runs locally. These are PARTIAL building blocks, not a connected production service. `internal/migrate/apply.go:81` and `drain.go:126` (`rg -n 'func (Apply|drainLocked)' internal/migrate/{apply,drain}.go`) make migration root-wide. Drain preserves verified live worker identities after owner-lock release; it neither steals active CLI ownership nor relaunches workers. No per-run enrollment bypass is proposed.

## Work Streams

| # | Spec | Scope | Domain | Dependencies |
|---|---|---|---|---|
| 1 | `supervisor-attachment-control` | Ordinary production singleton dispatch, migration/restart, scoped snapshots and durable stop | core; separate docs tasks | None; merged lifecycle prerequisite below |
| 2 | `existing-run-client` | Public attach, inspect-only TUI, trusted review/stop, manual fallback | core, tui, docs in separate tasks | Stream 1 stable production contract |
| 3 | `herdr-scoped-projection` | Optional negotiated metadata/action adapter and exact qualification | adapters, core, docs in separate tasks | Streams 1 and 2 |

## Dependency Graph

`supervisor-attachment-control` → `existing-run-client` → `herdr-scoped-projection`; stream 3 also directly consumes stream 1's projection. Drafting may overlap; implementation cannot assume an unmerged contract. Each stream is independently useful once complete: durable standalone service; standalone attach; optional host metadata. Domain-pure tasks and file overlaps are ordered explicitly.

## Implementation Order

1. Review proposed composition ADR and re-ground merged MH-23 lifecycle contracts. Complete the singleton ordinary public run path, root migration and restarted-worker reconciliation, then scoped projection/control.
2. Ship standalone `mythhelm attach <run-id>` and public command integration tests using ordinary runs. Legacy migration refusals are transitional diagnostics, never a recovered-only FR-1 completion claim.
3. Add optional Herdr transport, metadata and navigation. Run fixtures without Herdr; advertise a live row only after its instrumented fake-worker suite passes.

## Preserved Source Objectives

- **O1**: Reopen one existing run in a Herdr pane without another native launch, cancellation or process-owner transfer.
- **O2**: Show safe status and attention only for the explicitly attached run and its repository.
- **O3**: Reach trusted MYTHHELM review and stop controls without giving pane state approval authority.
- **O4**: Publish exact tested host capabilities and unsupported cases so this slice cannot be mistaken for complete MH-3 delivery.


## Source Coverage

Source identifiers are preserved rather than renumbered. Shared criteria are normative in every consumer; task IDs below identify the primary implementation and final integration proof.

| Source obligation | Primary child / tasks | Integration proof |
|---|---|---|
| O1; AC-1.1, AC-1.2 | existing-run-client Tasks 1–3, 6 | Ordinary public run → public attach, repeated attach, identities/launch counter; core Task 20 |
| AC-1.3 | existing-run-client Tasks 2, 6 | Separate socket loss, pane-process teardown, close, restore; host Task 7 |
| AC-1.4, AC-1.5 | existing-run-client Tasks 3, 6, 7 | Public command help, initial/repeated/manual fallback with registration absent |
| O4; AC-2.1 | herdr-scoped-projection Tasks 1, 2, 7 | Installed + live evidence; bundled schema alone negative |
| AC-2.2, AC-2.3 | herdr-scoped-projection Tasks 2, 4, 6 | Missing/denied/obsolete/slow/malformed/oversized cases; embedded fallback |
| O2; AC-3.1 | supervisor-attachment-control Tasks 3, 11, 12, 20 | Snapshot authority/generation races and cross-scope sentinels; host Task 6 |
| AC-3.2, AC-3.3 | supervisor-attachment-control Task 12 | Protected evidence distinctions, nullable unknowns, nested sentinel/control-sequence exclusion; host Task 3 |
| AC-3.4 | existing-run-client Tasks 1, 2, 6 | 2→1, identical 2, conflicting 2, old-generation after resync; core Tasks 11–12 |
| O3; AC-4.1 | supervisor-attachment-control Task 13; existing-run-client Task 4 | Competing revision/generation update before atomic recording; trusted navigation; host Task 4 |
| AC-4.2 | supervisor-attachment-control Tasks 13–15, 20; existing-run-client Task 4 | Delayed confirmation, lost acknowledgment, disconnect/restart, durable query and no second effect |
| AC-4.3 | existing-run-client Task 5; herdr-scoped-projection Task 4 | Writable takeover rejected and zero native input bytes |
| AC-5.1, AC-5.2 | herdr-scoped-projection Task 7 | Exact fixture/live rows and full lifecycle event matrix |
| AC-5.3 | existing-run-client Task 7; herdr-scoped-projection Task 8 | Executed examples, accessibility evidence, incomplete MH-3/G11 |
| NFR-1 | all testing tasks | Synthetic schemas + fake executable; live suite separately selected |
| NFR-2 | core Tasks 13–15, 20; client Tasks 2, 6; host Task 5 | 2-second deadline, 64 KiB frame, 16 slots, ten-second stalled host and deliberately blocking fixture |
| NFR-3 | core Task 6; host Task 2 | Same-user restricted local transport; unknown trust disables integration; no TCP/native credential copy |
| N1–N5 | every child non-goals copied unchanged | No native takeover, alternative scheduler/route, generic ABI, complete G11 or unqualified platform claims |
| Source DoD and OQ-1/OQ-2 | all child DoD; core design; host scratchpad | Source coverage plus independent validation; optional host unknowns do not substitute for composition |

## Open Questions

- Three-stream restructuring and unchanged-source archival were explicitly approved; routine design checkpoints proceed under the user's standing instruction. No product signature is inferred or created.
- The production seam is resolved by finishing ADR 0014's singleton ordinary dispatch, not by per-run recovery enrollment. Public/persistence details require reviewed proposed ADR `docs/decisions/0021-service-attachment-composition.md`; no ownership-policy weakening is authorized.
- MH-23 lifecycle plan PR #479 merged as `3e625eb`; its runtime task implementations remain dependencies, not shipped code at the grounding baseline. Core extraction waits for relevant Tasks4–9; core public-client Tasks16–19 additionally wait for its later Task18 launch/theme edits, and core documentation Task21 waits for its Task19 user-guide work. Implementation re-grounds its merged contract before extracting hooks; no old two-interrupt detach semantics are reinstated.
- Windows ADR 0017 is accepted but process-boundary implementation/qualification is pending at baseline. Linux fixtures may proceed; Windows rows remain unsupported pending evidence.
- Running Herdr compatibility, authenticated transport and attach-only restore registration need evidence. Default: disable unknown optional features and retain tested manual attach.

## Risks

- Service composition is larger than the initial estimate (21 core, 7 existing-client, 8 host tasks). Core tasks remain bounded and domain-pure; its design explains the approved three-stream grouping and does not hide production dispatch in one integration task.
- Competing pipeline/control recovery or legacy writers corrupt ownership. Root-wide migration, instance/run capabilities and one ingestion owner are tested under crash and race barriers.
- Immutable operation results cannot store later stop completion. Separate immutable acknowledgment and reconciled result; authorize replay before returning stored bytes.
- Optional host stalls can impede control if queues/locks leak. Bounded coalescing, separate deadlines and a deliberately violating responsiveness fixture are mandatory.
- Source preservation: requirements SHA-256 `9db8c58ef039b923228f73ddfdce4728c546f0d62d758c5d91bf024224e634e1`. Archive only after checking this table against all source AC/NFR IDs and byte equality; use the lifecycle script, never a manual move. Independent validation still decides readiness.

## Author Coverage and Source Preservation Check

On 2026-10-15 the author checked all 18 original AC clauses and all 3 NFR clauses against child requirements with exact full-line equality; none were missing. Original O1–O4 appear above, N1–N5 are copied unchanged into every child, and the table maps the source DoD/investigations to named tasks. This is an author coverage check, not independent validation.

The approved superseded source was moved with `scripts/harness/spec-lifecycle.sh move herdr-bridge archived` after that check. Requirements, refinement log and scratchpad remained byte-identical; archive rationale lives here to preserve the original source unchanged. Child directories remain unrefined until fresh independent validation.

| Source file | SHA-256 before and after archive |
|---|---|
| refinement-log.md | `45ba0b51789faecc184ef02c6b0f0d8dee1b9a3ab8b819760ee4823bdd8b6ce0` |
| requirements.md | `9db8c58ef039b923228f73ddfdce4728c546f0d62d758c5d91bf024224e634e1` |
| scratchpad.md | `ed392c3d14ba34fedcaddeb3813d52f3cc68b664954950363c4267e163e2ce90` |

## Round 1 Review Fixes

The core stream now has21 tasks (counted from `### Task` headings), client7 and host8, total36. A standalone operation/observation producer precedes clients; immutable canonical adoption is explicitly schema2 with historical decode separation. Durable stop acceptance/delivery/reconciliation precede CLI/TUI stop consumers. Host freshness/resync and TTL publication sequence are distinct from canonical projection revisions. Core/host documentation serializes after docs-site-restyle Task11. Source AC identifiers and unchanged archived bytes are unaffected. Fresh whole-epic round2 review is required.

Round2 baseline refresh: worktree integrated fetched main `f2b486d`; TUI observatory is in-progress and Windows Task2 fake-tree fixture is merged. Those lifecycle/fixture changes do not establish the still-required runtime/process qualification. Current-state code citations above remain explicitly grounded at their named baseline and are rechecked before implementation.

## Round 2 Review Fixes

Task11 now produces only internal grant primitives. Task12 combines them with projection production and publishes complete atomic OpenAttachment responses; no placeholder Projection precedes its producer. Task6 evidence is limited to the production singleton shell/root/status/context; complete handler and continued-worker-ingestion evidence belongs to Task20 after producers and clients. Shared CLI changes wait for MH-23 Task18 and shared documentation waits for its Task19 as well as docs-site-restyle Task11. Task counts remain21/7/8, total36. Source requirements and archived bytes remain unchanged. Final fresh whole-epic validation is pending.

## Final validation round 3 and pending correction

The third independent whole-epic validation returned Ready for the epic, supervisor-attachment-control and herdr-scoped-projection, and Needs revision for existing-run-client. Its sole blocking finding was the client design §4 unknown-run typed not-found/exit 2 contract contradicting the core design §§4–5 unknown/foreign permission_denied privacy contract. Under delegated engineering judgment, the coordinator retained the existing producer contract: unknown and foreign runs uniformly return permission_denied/exit 3 without existence disclosure; invalid arguments remain exit 2. The client design and Task3/Task6 acceptance tests now state that contract explicitly. Source requirements and scope are unchanged.

At the close of the prior run, this correction awaited independent validation. Its three-round limit was exhausted, so no fourth validation, lifecycle move or Ready claim followed that author correction. The drafts remained unrefined pending a newly resumed spec run; lint and hygiene were author checks only.

## Resumed spec run — validation round 1

After the delegated decision to preserve the producer privacy contract, a new spec run received a fresh independent whole-epic review. The epic and all three children returned Ready for implementation with zero findings; specs lint exited 0. This is resumed-run round1, following the prior run's three rounds and correction checkpoint recorded above, not a fourth round in that run. The unknown/foreign run sweep confirms uniform permission_denied/exit3; usage errors remain exit2.

The three child specs now move through the lifecycle script to todo; the epic remains unrefined until its first child starts. Counts remain core21/client7/host8, total36. Archived source hashes and all18 AC plus3 NFR clauses remain unchanged. Readiness covers the implementation plan only: ADR0021 remains proposed, runtime prerequisites remain unimplemented, and live host qualification remains a future acceptance gate. Product and orchestration approval ledgers are untouched.
