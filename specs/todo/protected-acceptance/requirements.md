## Protected Acceptance — Requirements

> Slice 1 of MH-22: protected acceptance bound to exact revisions plus truthful receipts — the existing snapshot/check/review/apply flow extended to deterministic P1 with strict qualified admission, an unverified candidate preserved when checks are unavailable, and receipts that say what was proven and what was not. Enforcement of the repair, replan and transport-retry ceilings is slice 2 (the execution deadline and the ceilings' data shipped with MH-16 S1). A slice of v2 §§4, 8, 10–11, 15. Normative source: mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md, version 2.0; § numbers, I-IDs, G-IDs and AT-IDs refer to it.

## Context

- **Backlog card**: MH-22
- **Issue**: [#128](https://github.com/turbokast/mythhelm/issues/128) (planned work; card holds status, stage, score)

The dogfood slice ships a working snapshot/check/review/apply flow (`run`, `review`, `apply` in `internal/cli/`; `internal/supervisor/apply.go`, `receipt.go`), but it is not v2 acceptance: schema v1 implements neither v2 TaskRevision nor policy pins (`grep -rin "taskrevision|task_revision|policy_pin|policypin" internal/ adapters/ cmd/ --include='*.go'` finds nothing), and the finalize review left open receipt gaps — the AC-4.2 verbatim entitlement sentence appears nowhere user-facing (#72, OPEN), the JSONL envelope set escapes the documented contract (#74, OPEN), and the NOT RUN label is missing from receipt and review (#75, OPEN). The apply gate enforces what the run reason says: an unverified candidate requires `--accept-unverified` (`internal/supervisor/apply.go:143-144`), but it never reads the verification or its binding (see the `apply gate` verdict).

This slice binds acceptance to exact revisions (candidate, contract, check definitions, environment — I07), protects the evaluator from candidate edits (I07), preserves an unverified candidate when checks are unavailable, and makes receipts truthful (absorbing #72, #74, #75). Prerequisite state at re-grounding (2026-10-11, origin/main 31cb991): MH-10 shipped, MH-16 S1 shipped, MH-21 shipped (D-90: `v2-contract-vocabulary`, `supervisor-service`, `supervisor-migration`, `supervised-stop-recover`), MH-13 implemented on main with its spec awaiting finalize, MH-12 implementing with Tasks 5 and 7 held. Tasks that need MH-12's strict end-to-end are sequenced last and marked blocked-on in `tasks.md`.

Grounding verdicts (first recorded 2026-10-07, re-grounded 2026-10-11 against origin/main 31cb991; each command was run on that tree):

- `schema v1 implements no v2 TaskRevision or policy pins` PARTIAL. The v2 types now exist (`internal/v2contract/records_execution.go:44` `TaskRevision`, `internal/v2contract/records_coordination.go:180` `PolicyVersion`) and schema v7 has `task_revisions` and `policies` tables (`internal/journal/migrations/0007_v2contracts.sql`), but the only writer of a `TaskRevision` is the legacy import (`grep -rniE "taskrevision|task_revision|policy_pin|policypin|INTO policies|INTO task_revisions" --include='*.go' internal adapters cmd | grep -v "_test.go\|^internal/v2contract/"` finds only `internal/migrate/import.go`, `apply.go`), nothing writes a `policies` row, and a new run records no pin. The gap this slice closes is unchanged.
- `issues #72–#76 record receipt/recovery gaps, all OPEN` HOLDS for state, PARTIAL for text (`gh issue view <n> --json state`: 72, 73, 74, 75, 76 all OPEN). #72 HOLDS: `grep -rn "NOT verified by MYTHHELM" --include='*.go' internal adapters` finds only `adapters/claudecode/probe.go:137`. #74 HOLDS: `attempt.native_error` is a worker event (`internal/supervisor/ingest.go:42`) and `recovery.ownership_resolved` is emitted at `internal/supervisor/recover.go:158`; neither is in `documentedEvents` (`internal/supervisor/pipeline_test.go:809`); the issue's line numbers drifted. #75 HOLDS and is worse: `NOT RUN` appears in three renderers (`internal/cli/render.go:231`, `internal/cli/accessible.go:298`, `internal/tui/mission.go:268`), each with the reason hard-coded as `(waived by --no-checks)` even when the journaled reason is `checks_refused_under_inspect` (`internal/supervisor/pipeline.go:1130`); the receipt carries no label or reason (`internal/supervisor/receipt.go:220`, `:268`). #73 is PARTIAL: `--max-duration` is superseded by `--envelope-execution` (`internal/cli/run.go:142`); `--capture-raw`, `--log-level` and `supervisor.log` are still absent (`grep -rn "capture-raw\|log-level\|max-duration\|supervisor.log" --include='*.go' internal cmd | grep -v _test` finds nothing). #76 is UNVERIFIABLE for its second half in this pass; its first half still matches `internal/supervisor/pipeline.go:971` (ingest failure marks the run `interrupted`/`ingest_failed` with a notice and no stop request). #73 and #76 are slice 2.
- `snapshot/check/review/apply flow exists` HOLDS (`internal/cli/run.go`, `review.go`, `apply.go`; `internal/supervisor/apply.go:63` `ApplyRun`, `receipt.go:60` `BuildReceipt`).
- `apply gate enforces unverified acceptance` PARTIAL. It enforces what the run reason says, not what was verified: `internal/supervisor/apply.go:143` tests only `run.Reason == "unverified"`, and `grep -n "LatestVerification" internal/supervisor/apply.go` finds nothing, so a `ready_for_review` run is applied without reading its verification row, candidate or evaluator.
- `issue #128 section references (v2 §§4, 8, 10–11, 15; W04)` HOLDS (v2 numbering; P1 defined at v2 §10.2).
- `strict qualified admission prerequisite` PARTIAL, narrowed. MH-10 is shipped. Strict admission exists (`internal/admission/billing.go:35` `ResolveBilling`, MH-12 Task 3) but admits nothing through `cli.Main` while the managed-policy source gap is open (`specs/*/claude-strict-subscription/scratchpad.md` Q1; `TestE2EStrictStillBlocked`, `tests/e2e/qualification_test.go:155`); MH-12 Tasks 5 and 7 are held on it. No route is live-qualified.
- `MH-13 contained profiles are a prerequisite, unimplemented` REFUTED by main: all 11 tasks of `specs/*/contained-execution-profiles/` are merged (`git log --oneline origin/main | grep -c "cep task\|contained-execution-profiles task"` prints 11); the spec waits for finalize and the MH-13 card still reads `specced`. It already ships part of FR-2: check argv come only from the admitted configuration and every verification records an evaluator digest (`internal/integration/verify.go:79`, `:150`, `internal/journal/migrations/0006_evaluator.sql`). Enforcement is Linux-only.
- `finite repair/resource envelopes are slice 2` PARTIAL. MH-16 S1 shipped finite ceilings (`internal/billing/envelope.go:12`, `internal/journal/ledger.go:148`, flags `internal/cli/run.go:142`), but only the execution deadline binds a live run: repair, replan and transport ceilings never do (issue #328, OPEN). The receipt shows only `repair_ceiling` (`internal/supervisor/receipt.go:338`).
- `one verification path ends unavailable checks as failed` (new). Unavailable checks end the run `failed` / `verification_unavailable` (`internal/supervisor/pipeline.go:1164`, `:1203`; pinned by `TestMissingExecutableUnavailableExit5`, `internal/supervisor/pipeline_test.go:396`, and `TestUnavailableChecksStayUnverified` in `internal/supervisor`, `:1933`), while v2 §11.2 and AT-24 want an unverified candidate. AC-1.3 fails today.

### Objectives

- **O1**: A run's acceptance is bound to the exact candidate, contract, check definitions and environment it was evaluated against; anything else is reported, never accepted.
- **O2**: Receipts and review output truthfully distinguish proven, unverified and not-run outcomes, including the verbatim labels the criteria require.
- **O3**: Candidate content cannot weaken, bypass or redefine the evaluator that judges it.

### Non-Goals (and what still binds)

| # | Out of scope | Still binding in this spec |
|---|---|---|
| N1 | Enforcement of the repair, replan and transport-retry ceilings, and retry policy (slice 2 of MH-22, with #328). | I21: every run keeps finite execution bounds; MH-16 S1 already enforces the execution deadline, and this slice records the bounds the run was admitted under, naming which are enforced, instead of adding enforcement. |
| N2 | CLI surface gaps (#73) and ingest/recovery gaps (#76). | I12 (recovery reconciles before retry) and I09: recovery and CLI-surface behaviour of #76 and #73 stays as shipped; this slice must not close or duplicate them without re-checking. |
| N3 | Contained profiles and budget ledger mechanics (MH-13, MH-16). | I07 (evaluator evidence) and I21 (finite envelopes): acceptance reads their evidence, never reimplements it. |
| N4 | Learning, routing and multi-agent policies (P2+). | v2 §10: P1 only; the policy pin exists so later work has something to compare against. |

## Functional Requirements

Acceptance criteria use EARS. Tags in brackets name the invariants, gates and sections each one serves.

### FR-1 — Acceptance bound to exact revisions (§11, I07, I20, G06)

- **AC-1.1** [I07, AT-23] When checks evaluate a candidate, the system shall bind the verdict to the exact candidate revision, task-contract revision, check-definition revisions and environment identity, and record all four with the verdict.
- **AC-1.2** [I20, AT-23, G03] If any bound revision changes after evaluation (new candidate commit, amended contract, changed check definitions), then the prior verdict shall not apply to the new revision; re-evaluation is required.
- **AC-1.3** [§11, AT-24] When checks are unavailable (no check definitions, no runner, or an environment that cannot run them), the system shall preserve the candidate as unverified rather than failing it or passing it silently.

### FR-2 — Protected evaluator (§11, I07, G07)

- **AC-2.1** [I07, AT-22] Candidate file edits, task text and plugin messages shall not change the check definitions, evaluator configuration or acceptance thresholds applied to that candidate.
- **AC-2.2** [G07] When the evaluator's pinned configuration differs from what ran (digest mismatch), the system shall report the verdict as invalid, never as passed.
- **AC-2.3** [§4] The run shall record its policy pin (`P1`) with the pinned evaluator revision, so later policies compare against exactly this evaluator.

### FR-3 — Truthful receipts and review (§15, I09, G06, G05)

- **AC-3.1** [I09, #72] The receipt and every summary of a user-declared run (declared-mode runs continue alongside P1; AC-4.3 governs P1 admission only) shall carry the verbatim sentence `entitlement: user-declared, NOT verified by MYTHHELM; paid continuation: unknown (user declares disabled)`; a qualified route's receipt shall carry the verified entitlement statement derived from its admitted posture instead (the consulted qualification record's identity is not journaled with admission).
- **AC-3.2** [#75, AT-35] An unverified run shall be labelled `NOT RUN` in the receipt verification section and in review output, with the reason; it shall never render indistinguishably from a verified run.
- **AC-3.3** [#74, v2 §15.3] The run/recover JSONL stream shall emit only documented envelope events; every emitted event type shall appear in the contract allowlist with a test pinning the set.
- **AC-3.4** [I09] If a receipt field is unknown (unreported cost, unobserved usage, missing baseline), then the receipt shall print `unknown` with the reason, never `0` or an invented value.

### FR-4 — Deterministic P1 flow (§§10–11, G01, G06)

- **AC-4.1** [§11] The one-agent flow (admit → run → check → review → apply) shall execute deterministically from its pinned inputs: the same revisions in shall produce the same verdict out, with non-determinism confined to the native session and to the checks' own behaviour; the receipt shall carry a `determinism` note naming the native session as the confined source and stating that a check's own determinism is not asserted.
- **AC-4.2** [G01] The full P1 flow shall run without credentials beyond what the qualified route itself needs; contributor tests for it shall need no agent credentials.
- **AC-4.3** [§10] When a run asks for qualified P1 delivery (strict `subscription-only`), the system shall admit it only on the MH-10/MH-12 qualified route; an unqualified route shall block with the registry reason, never fall back silently. A declared or scripted run (AC-3.1) is not qualified P1 delivery: it stays admissible, carries the P1 acceptance pin (AC-2.3) and is labelled unqualified.
- **AC-4.4** [I21, N1] The run shall record its current execution bounds (durations, attempt/data caps) as data in the receipt, saying which are enforced, without adding envelope enforcement (slice 2 owns the unenforced ceilings).

## Non-Functional Requirements

- **NFR-1** [§17.2] No acceptance step (binding, recording, receipt rendering) shall run without a stated timeout, named per step at design time; the duration of the binding step shall be measured per run and recorded in the receipt as the acceptance overhead, and the record and render steps are bounded by their timeouts. (No maintainer area budget exists for acceptance evaluation; the per-step values are set in design §2.9.)
- **NFR-2** [I13] Demo and contributor tests for this slice shall require no agent credentials and no live routes.

## Definition of Done

- [ ] AC-1.1 to AC-4.4 each have a named test; each criterion that changes behaviour has one that fails before the change and passes after it. AC-4.3's blocked path is already true (the AC was reworded for the qualified-delivery reading, scratchpad Q-6) and stays pinned by `TestE2EStrictStillBlocked` (`tests/e2e/qualification_test.go:155`); its positive path is blocked-on MH-12 Task 5.
- [ ] Issues #72, #74, #75 close as fixed by this slice (verified against their bodies, not assumed).
- [ ] `scripts/harness/gate.sh go` and `gate.sh harness` pass; CI green on Linux, macOS and Windows.
- [ ] MH-22 synced toward `implementing` (half-shipped pattern while slice 2 remains), this slice implemented after its prerequisites ship, finalized and PM-synced.

## Open Questions

Each question has its conservative default in `scratchpad.md`; the design records how it resolved.

- **Q1**: Exact v2 TaskRevision shape — adopt MH-21's contract verbatim or extend it for acceptance binding? Blocks FR-1; default: adopt MH-21 verbatim, extend only with a recorded deviation. Resolved by design D1 and D2: `v2contract.Verification` and `v2contract.PolicyVersion` are consumed verbatim and nothing is extended; the contract revision is referenced by the pin (task id, revision 1, contract digest), and no `TaskRevision` row is written for a new run.
- **Q2**: Which check runners exist in S1 (shell checks, Go tests, something else) and where do their definitions live? Blocks FR-2; default: the dogfood check-config shape already admitted, pinned by digest. Resolved from main: one runner (an argv process, `internal/integration/verify.go:178` `checkCommand`, `:215` `runCheck`); definitions are `[[checks]]` in the committed `mythhelm.toml` (`internal/admission/projectconfig.go:39` `ProjectConfig`), digest-bound.
- **Q3**: Does the P1 receipt stay in `receipt.go`'s current shape or move to a v2 receipt contract from MH-21? Blocks FR-3; default: extend the current shape; migrate only if MH-21 requires it. Resolved from main: `internal/v2contract` defines no receipt type (`grep -n "Receipt" internal/v2contract/*.go` finds nothing), so the current shape is extended additively (D8).
- **Q4**: Slice-2 boundary — do finite envelopes need any hook in slice 1 (envelope fields, counters)? Blocks N1 honesty; default: record current bounds as data; no enforcement hooks until slice 2. Resolved by design D9: MH-16 already stores the fields; this slice only renders them.

## Dependencies

- Prerequisites at 2026-10-11: MH-10 (`specs/*/qualification-registry/`) shipped; MH-16 S1 (`specs/*/budget-ledger-s1/`) shipped; MH-21 (`specs/*/v2-contract-vocabulary/`, `supervisor-service`, `supervisor-migration`, `supervised-stop-recover`) shipped; MH-13 (`specs/*/contained-execution-profiles/`) implemented on main, finalize pending; MH-12 (`specs/*/claude-strict-subscription/`) implementing, Tasks 5 and 7 held on its Q1.
- Blocked-on, sequenced last in `tasks.md`: the qualified-route end-to-end (needs MH-12 Task 5 and Q1) and the maintainer's live evidence (needs MH-12 Task 7).
- Slice 2 (separate spec, same card): enforcement of the repair, replan and transport ceilings (#328), #73, #76.
- Conflicting: `specs/*/codex-native-adapter/` (`internal/supervisor/pipeline.go`, `docs/limitations.md`), `specs/*/windows-process-ownership/` (`internal/supervisor/pipeline.go`, `recover.go`, `docs/limitations.md`) and `specs/*/claude-strict-subscription/` Task 5 (`tests/e2e/`; its declared-receipt goldens): rebase onto their merges; never in parallel on the same file.
- Absorbs: issues #72, #74, #75 (each re-checked at implementation time per the card).

## Impacted components

- Touched: `internal/acceptance/` (new, pure), `internal/supervisor/` (pin, verdicts, apply gate, receipt: `pipeline.go`, `recover.go`, `apply.go`, `receipt.go`), `internal/integration/` (evaluator pin seam), `internal/journal/` (schema 8, verification binding row), `internal/admission/` (entitlement statement), `internal/cli/` (`review.go`, `render.go`, `accessible.go`), `internal/tui/` (NOT RUN reason, entitlement statement), `docs/`.
- Tests: golden receipts, revision-mismatch fixtures, an event-type allowlist scan, a credential-free packaged-binary flow; no live credentials (I13, NFR-2).
