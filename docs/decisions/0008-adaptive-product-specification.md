# 0008. Integrated product contracts and durable supervision

- Status: proposed
- Date: 2026-10-05

## Context

The Revision 1.1 master and adaptive proposal describe overlapping product models. The repository implements a useful dogfood/TUI slice with per-run supervision, not a qualified adaptive product. Adopting both texts unchanged would leave competing task stores, execution owners and conflicting stage meanings.

## Decision

Adopt [Master Specification v2](../../mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md) through the [specification index](../spec/README.md), after signed product adoption and PR review. Keep the original source files and their historical references unchanged. [The adoption map](../spec/synthesis-adoption.md) connects the full destination and first delivery to product decisions and W01–W16 owners.

The destination uses one local canonical ledger and one per-user supervisor per execution host. Workers retain process ownership and bounded spools; clients and Herdr consume authenticated control/projection interfaces. Task, artifact, grant, policy and evaluation revisions bind acceptance and authority. These contracts replace competing Markdown task authorities and independent experiment executors.

Retain Go, Bubble Tea, TOML, SQLite and native executables. Retain independent fidelity, entitlement, lifecycle and trust qualification, strict included-only default, protected checks and finite execution envelopes. Preserve all seven harness targets and broad platforms with honest per-target qualification. Optional learning evaluates a small portfolio and can replace a pipeline with one stronger model; it cannot grant authority or weaken acceptance. Optional deployment has an independent effect/health contract.

## Consequences

The implementation behind ADRs 0003–0007 remains the runtime baseline until explicitly migrated. This decision does not retroactively change their evidence or declare them implemented under v2. MH-21 must define additive migration, one-writer exclusion, drain/adopt/quarantine, backup/restore, schema refusal and event compatibility before the new owner starts. Each later implementation ADR names the precise contract it replaces.

S1 delivers one qualified complete native workflow with durable control and usable standalone/Herdr journeys. Adaptation remains the full S5 destination, proven only by scoped useful evidence; S6 production delivery is independently optional. AT-01–AT-48 and G01–G16 define planned acceptance. Original source hashes, backlog coverage, product validation and harness checks establish adoption consistency, not native qualification or product performance.
