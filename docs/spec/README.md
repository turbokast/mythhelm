# Product specification

The adoption on this branch takes effect when its exact product changes receive the maintainer's signature and its reviewed PR merges; until then it is proposed.

The current normative product design is [MYTHHELM Master Specification v2](../../mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md). Its explicit requirements are UR-01–UR-10, invariants I01–I25 are in §2, and acceptance cases AT-01–AT-48, stages S0–S6 and gates G01–G16 are in §18.

The [implementation plan](../../mythhelm-synthesis/MYTHHELM_Implementation_Plan.md) defines W01–W16. The [product adoption map](synthesis-adoption.md) connects those slices to the approved backlog. [Product objectives](../../product/objectives.md) and [backlog](../../product/backlog.md) are the delivery index and plan of record; [decisions](../../product/decisions.md) records maintainer adoption. A specification is not evidence that the application implements it or that a native route has qualified.

## Preserved sources and history

- [Revision 1.1 master](master-spec.md): preserved byte-for-byte as the source for existing historical section references and implementation records.
- [Adaptive proposal](../../MYTHHELM_Adaptive_Orchestration_Proposal.md): preserved input, not a second active specification.
- [Synthesis prompt](../../MYTHHELM_Best_of_Both_Synthesis_Prompt.md): preserved task provenance.
- [Synthesis decisions and evidence](../../mythhelm-synthesis/MYTHHELM_Synthesis_Decisions.md): source dispositions, research limits and original synthesis validation.

New product work cites v2 explicitly. Existing specs, ADRs, acceptance evidence, closed cards and append-only decisions/signals keep their named source version: an old `§13.10` or audit `A30` does not acquire a new meaning. I01–I19 and G01–G12 retain traceable outcomes; v2's decision report maps their clarified scope. Evaluate an existing unfinished spec against v2 before resuming it, preserving published acceptance IDs and recording any change through its reviewed lifecycle. The worked historical example in `knowledge/spec-authoring.md` remains explicitly pinned to Revision 1.1.

No source-file replacement, executable migration, account change or release occurs merely by adopting this design. The [repository workflow](../../WORKFLOW.md) still governs product approvals, implementation, PRs and operator actions.
