# TUI Observatory View — Scratchpad

> Open questions and research from authoring. Each implementing task reads this file when it starts and adds an entry under Discoveries when it finishes.

## Open questions

| # | Question | Default in this spec | Owner / blocks |
|---|---|---|---|
| Q1 (resolved) | Distinguish lost-terminal client result from successful canonical run outcome. | Maintainer approved 2026-10-15: preserve canonical outcome; map client0 to6 after terminal loss and retain nonzero run failures. Deliberate second Ctrl-C keeps AC-6.9's outcome. AC-6.13 amended narrowly to state this. | Resolved; concrete ADR approval still belongs to Task3. |
| Q2 | Is an independent non-locking owner observation qualified on this deployment? | No. Production stale/missing beat renders supervision unknown. Test the conditional unsupervised path without claiming a platform probe. | Future platform owner; nonblocking because AC-4.12 explicitly permits unknown. |
| Q3 | Who records human terminal/accessibility evidence? | Task20 is the maintainer's synthetic demo re-record only; NVDA/VoiceOver/Orca and real light/dark qualification stay slice4. | Maintainer; finalization waits Task20, no agent evidence substitution. |

## Research notes

- Author: native Codex agent in the portable spec workflow, 2026-10-15; base origin/main `30062ec`. No native paid inference, account switch or live benchmark was used for specification authoring.
- Scope: single spec, auto-confirmed in non-interactive dispatch, honoring explicitly confirmed OQ-21. Twenty bounded tasks preserve contract-before-consumer and one-domain ownership; Tasks1–10 are the lifecycle prefix. Requirements changed only at AC-6.13 to record the maintainer-approved client/run exit distinction; refinement log remains historical and unchanged.
- Grounding: read pipeline/watch/conclude/recover, worker handoff/supervise/requestStop/envelope, ownerlock Acquire, TUI model/nav/dialogs/actions/palette/help/live/mission/styles/theme/viewmodel, journal row types, source frames1a/1f/1g/1h/1i, ADRs0004/0005/0007, current master lifecycle/UI/extensions/gates and authoring/domain rules.
- `rg -n 'Launch\{' internal tests` identified actual readLaunch/spawn fixtures in worker_test.go, supervisor faults_test.go and pinning_test.go, and admission launch_fixture_test.go; all are in Task4. Containment/config helper-only literals do not deserialize a handoff and are not blindly migrated.
- `rg -n 'Tokens\.Warning|t\.Warning|Warning:' internal/tui` identified mission.go, diff.go, actions_test.go and theme implementation/tests; Task11 migrates them together so schema removal compiles.
- `rg --files docs/demos` confirms existing tui.tape, tui.gif, tui.transcript.txt, tui-poster.png and manifest.json; Task20 owns all changed derived artifacts.
- Supervision research: [Linux man-pages proc_locks(5)](https://www.man7.org/linux/man-pages/man5/proc_locks.5.html), read 2026-10-15, documents FLOCK entries and PID-namespace filtering. Absence is insufficient for a universal no-owner claim. No probe takes the exclusive owner lock, even momentarily.
- Deadline interpretation: AC-6.11 explicitly requires fenced and unfenced deadline workers to exit. Its more specific condition exempts deadline-terminal workers from the general await-reconnect residual; the honesty register preserves that residual for non-deadline episodes. No requirement text changed for this interpretation.
- Ordinary controls and human approval checkpoints remain distinct. The user delegated routine design judgment; no delegation creates a signed product approval or approves the ADR's future exact contents.
- Benchmark scope here is renderer performance only (ten non-race samples, median ns/op/allocations, bounded-row acceptance). It is not apples-for-apples native harness evidence and cannot support a MYTHHELM-vs-CLI advantage claim.

## Validation

Mechanical spec lint passed on 2026-10-15. Embedded go-test selectors: none in the authored design/tasks/scratchpad (named future tests are acceptance contracts). Round1 fresh-context verdict: Needs revision (references PASS, requirements PASS, design FAIL, tasks FAIL, conflicts PASS, epic n/a). Two blocking findings fixed: demo retention/recovery coverage and missing exit.go in Task5. Added lifecycle Task9, shifted subsequent task IDs, and added packaged demo continuity/explicit cleanup proof to Task10. Round2 fresh-context verdict: Ready for implementation, all checks PASS (epic n/a), with one advisory to raise Tasks17/18/20 from standard to complex because each owns six files. Those budget corrections are applied; final mechanical lint and round3 confirmation both passed; the spec is Ready for implementation.

- Round1 grounding: demo.go unconditionally defers RemoveAll for its private repo/state and reviewOK checks only Detached; recover reads statedir.Resolve, so a bare command points at caller state instead. Added Task9 retention and shell-safe explicit state-root guidance; operator cleanup remains separate from ordinary recover, as coordinated with the delivery agent.
- Round1 sweep: reindexed all singular/plural task references, lifecycle prefix now1–10 and total20; Task5 now owns exit.go where runExit is defined. Spec lint rerun after the sweep.

### Round 2 — Fresh validator report

Lint (specs): exit 0; clean. Embedded test selectors: none; no test binaries executed. Vendor: not run. Files edited: none.

### Spec review: tui-observatory-view

Verdict: Ready for implementation

| Check | Result | Findings |
|---|---|---|
| 1 References | PASS | Existing interfaces and task paths support the design. |
| 2 Requirements | PASS | Numbered requirements, binding non-goals and DoD coverage present. |
| 3 Design | PASS | Lifecycle contracts, demo retention, recovery destination and deferred capabilities addressed. |
| 4 Tasks | PASS | One budget advisory below; no blocking implementation gap found. |
| 5 Conflicts | PASS | Overlapping active specs have serialization/revalidation notes. |
| 6 Epic | n/a | Explicitly retained as one spec under OQ-21. |

Findings: 1. [advisory] Check 4 — tasks.md, Tasks 17, 18 and 20: Each lists six files but declares Budget: standard. The decomposition procedure assigns six-or-more-file tasks complex. Change these budget declarations to complex; this does not require a scope split.

No blocking findings. The demo retention/recovery coverage and Task 5’s internal/cli/exit.go ownership resolve the identified lifecycle gaps.

Author disposition: all three budgets changed to complex; spec lint exit 0 after the correction. Round 3 checked the corrected files before the move.

### Round 3 — Final fresh validator report

Lint (specs): exit 0; clean. Embedded test selectors: static extraction found no go test commands in task text; no test binaries executed. Vendor: not run. Files edited: none.

### Spec review: tui-observatory-view

Verdict: Ready for implementation

| Check | Result | Findings |
|---|---|---|
| 1 References | PASS | Task paths exist or create files in existing directories. Runtime, theme and view-model contracts agree with the inspected source. |
| 2 Requirements | PASS | Numbered requirements, binding non-goals, concrete acceptance criteria and Definition of Done cover the slice. |
| 3 Design | PASS | Ownership, deadline enforcement, demo retention, recovery guidance and presentation contracts are specified; remaining limitations are recorded. |
| 4 Tasks | PASS | Dependencies precede consumers, ownership is separated, larger file sets are justified, and Tasks 17/18/20 now use complex budgets. |
| 5 Conflicts | PASS | Active worker and CLI overlaps have serialization and revalidation instructions. |
| 6 Epic | n/a | Single-spec scope is explicitly retained under OQ-21. |

Findings: None. No blocking or advisory findings identified. Concrete ADR approval remains Task3 implementation checkpoint; verdict does not replace it.

Accounting across three fresh-context reviewers: dispatched=3 returned=3 failed=0. Final mechanical spec lint exit 0. No native-account trial or implementation was performed. Lifecycle move uses the shared script only after this Ready verdict.

## Hosted review round 1

- Recovery-command finding confirmed: existing review argument validation is broader than a shell-safe token. Design §3 and Task9 now require bounded full run grammar, rejection of controls/metacharacters, and independent target-shell quoting of both arguments. Kept malicious-ID/path fixtures test the executable effect.
- Terminal-loss apply finding confirmed: current tui.Run calls cancelAction unconditionally after Tea returns; actionCtx derives from view lifetime. Design §3 and Tasks7/8 now specify independently owned/joined action lifetime, headless completion callback and canonical action-result mapping in both live and inspect modes; explicit double-quit retains cancellation/join.
- Mixed-version finding is a real compatibility limit; accepting absent mandatory execution_deadline is rejected as a fix because AC-6.11 explicitly requires fail-closed rejection and I21 forbids inventing an unlimited envelope. Design §2, Task4 and Task19 now test/document both strict-decoder directions, retained running-worker evidence and the unsupported admission-to-spawn binary-replacement window. No automatic upgrade compatibility or new authority is claimed; the future ADR still requires maintainer approval.
- Source anchors re-read: internal/cli/review.go validation, internal/ids/ids.go generated grammar, internal/tui/model.go Run cleanup, internal/tui/actions.go actionCtx/cancelAction, internal/cli/tui.go wireActions, internal/workers/worker.go Spawn/readLaunch, ADR0004 private stdin handoff and v2 §17.2 upgrade demand.
- These are hosted-review corrections after the completed three-round specification validation. Independent review of the changed design/tasks is arranged by the delivery coordinator; no implementation started.

## Hosted-review fix validation, round 2

- Independent review confirmed the headless-apply and explicit mixed-version fail-closed fixes; it found PowerShell smart-quote escaping incomplete. Design §3 and Task9 now enumerate and duplicate all recognized single quotes (U+0027/U+2018/U+2019/U+201A/U+201B), with an actual Windows PowerShell execution fixture checking exact state/argv and absence of injected effects.
- Unix commands are explicitly labelled POSIX sh/bash/zsh; fish/cmd/other-shell copy-paste is not claimed. Manual guidance identifies the state/run and a supported shell.
- ActionCompletion now carries StopState from supervisor.Stop. Tasks7/8 distinguish stopped from stop_requested even with nil error; terminal-loss client6 does not falsely certify the stop or rewrite canonical run state.
- Primary sources read 2026-10-15: Microsoft about_Quoting_Rules and PowerShell CharTraits.IsSingleQuote (linked in design); local internal/supervisor/stop.go returns stopped only from confirmed worker evidence and otherwise stop_requested after writing the request.

## Hosted-review fix validation, round 3

- Moved current-state facts and their exact finder commands out of the Markdown table so regex alternation pipes cannot split cells.
- Aligned AC-6.13 with the existing inspect-mode action ownership in design and Tasks7/8: terminal loss joins in-flight Stop/recover/apply headlessly, reports the final action result, preserves canonical outcomes and retains client6/nonzero failure mappings. StopState continues to distinguish confirmed stopped from pending stop_requested; no authority or scope changed.

## Discoveries

### Task 1 — Deterministic linear capture findings

- Captured seven nonempty outputs from unchanged production code at `e4ab1fc46fb08886af5e8195c4a2812b1b0b6e6f`; two consecutive base captures matched before storing their exact normalized bytes. Inputs are synthetic journal events, receipt and typed usage rows, plus the credential-free fake demo. No native account evidence is claimed.
- The base generates more volatility than AC-7.1 enumerated: JSONL result event/producer ULIDs, demo attempt ULIDs, process IDs and Git commit IDs. Git's environment filtering discards inherited `GIT_AUTHOR_DATE`/`GIT_COMMITTER_DATE`. The delivery coordinator approved narrowly located identity normalization preserving repeated-reference relationships, exact semantic bytes, and independent Git candidate content verification; no arbitrary digest replacement.
- The coordinator approved actual run admission-refusal captures (exact exit 2 and reason) alongside richer fixed journal/render fixtures and successful demo lifecycle. `demo --accessible --after 1` avoids the nondeterministic startup summary; fixed review/render cases retain summary and created-event coverage. Every event from 2 through 23 and final cursor 23 is required.
- Unmodified `runAccessibleLiveWith` / `watchRunTerminal` can end the natural-timing stream at `ready_for_review` (event 22), before the supervisor appends `receipt.written` (event 23). Repeated race tests reproduced trailer 22 despite a valid final receipt/review. Task 1's bounded slow-consumer writer waits at event 4 for journaled `receipt.written`, controlling source timing without modifying production. **This is not a fix or qualification of the natural-timing race.** Task 8's CLI lifecycle work should add a natural-timing terminal/receipt regression and reconcile stream completion with pipeline completion.
- Acceptance evidence: missing captures made `TestObservatoryLinearBaseline` fail; two base capture runs matched; after controlled timing, `go test -race ./internal/cli -run '^TestObservatory' -count=3 -v` passed (13.379s). Kept counterfactuals reject changed state, provenance, usage counter, exit, record type/field, candidate content/binding, attempt identity relationship and event sequence.

### Task 2 — Proposed contract record

Native Codex docs implementation, PR #484, commit `54f859d`: ADR 0020 remains proposed for exact-revision Task 3 approval. Independent architect review returned Ready with no critical or important findings. AC-1.10's Observatory attention already exists as shared Amber (dark), so the separate table adds nine missing rows and references the shared amber values without changing Colours. Documentation checks verified local links, exact shared-table preservation, spec lint and hygiene; runtime tests listed in the ADR are explicitly future obligations.

### Task 1 — Review correction round 1

- A long temporary root truncates event 4 before its `/runs/` segment. `TestObservatoryReceiptWaitWithTruncatedStatePath` first failed with `demo workspace line lacks state path`; after resolving the unique state directory inside a capture-owned temporary root, it passes and rejects ambiguous state roots. The captured line remains unchanged.
- Failed demo probes now report exit status, stderr and stdout. `go test -race -run TestObservatory ./internal/cli -v` passes all seven immutable captures, semantic counterfactuals and the kept truncation regression (5.815s). No baseline fixture or production behavior changed.


### Task 4 — Worker-owned deadline and durable conclusion

Implementation `bb152be`, PR #497, completed by the Codex go-implementer role. Required handoff validation, a generation-independent worker timer and shared normal/recover classification now implement accepted ADR 0020 §Decision / design §2. No identity/spool schema, module or immutable output capture changed.

| Contract / invariant | Retained acceptance evidence |
| --- | --- |
| Mandatory and incompatible handoffs refuse native launch (I02/I21) | `TestLaunchDeadlineRequired`, `TestMixedWorkerHandoffVersions` |
| Expiry before or during preparation creates no native process (I02/I21) | `TestExpiredDeadlineDoesNotLaunch`, `TestExpiredDeadlineDuringPreparationDoesNotLaunch` |
| Worker independently enforces two-second execution and exits without a client, generations 0/7 (I18/I21) | `TestWorkerDeadlineWithoutSupervisor` |
| Durable first-request winner and one ladder (I06/I18) | `TestDeadlineRequestPrecedence`: user-file-first, deadline-file-first, already-running user ladder, both timer/poll orders |
| Persistence or ownership uncertainty never becomes a confirmed deadline outcome (I06/I21) | `TestDeadlineStopFailureStaysUnresolved`, `TestDeadlineStopWithoutNativeExitIsBounded` |
| Autonomous deadline reason survives recovery without relaunch (I12/I21) | `TestDeadlineRecoveryConclusion`, existing `TestRecoverArmsExecutionDeadline` |
| Existing contracts remain intact | Full race suite includes RequestExtension, running-worker recovery, process ownership and Task1 `TestObservatoryLinearBaseline` regressions |

Red-first observations: the original decoder accepted a missing pin; an expired handoff actually emitted `attempt.launched`; workers with no supervisor stayed active past the two-second envelope; malformed stop evidence was accepted as a successful unknown-requester stop; recovery returned cancelled despite a journaled deadline reason. Each corresponding restored test passes. Kept controlled counterfactuals also fail when the timer is removed, deadline workers retain reconnect waiting, a timer relabels an earlier user request, or the deadline native-exit wait is unbounded. Failed-case cleanup stops/reaps the native fixtures and joins the controlled session goroutine.

Independent architecture review returned Ready. Code review found that an early deadline check did not cover durable launch-intent and containment/job preparation. The regression deliberately holds identity persistence until expiry and first observed an actual native PID; the correction rechecks immediately before native start, closes both prepared stdout endpoints and the Windows job, and joins containment cleanup. Fresh review of the correction returned Ready. This is a consistent acceptance correction, not a new qualification claim.

Validation on the final implementation: full Go race gate passed in 226.7s (supervisor 220.906s; workers 21.367s; CLI 82.505s); vet/tidy/lint passed with zero lint issues. Named `-race -v` acceptance output passed for workers in 6.267s and supervisor in 4.519s. Full-repository Windows AMD64 and Darwin ARM64 vet passed. All six required markers are refreshed before publication.

Actual gate history: the initial full race run failed `TestSecondInstanceHeld`, `TestRootConflictRefused` and `TestStaleLockAdoptedWithGenerationBump` because inherited runtime resolution selected a shared lock held by a supervisor predating this task. The lock metadata and independent process start time established that cause; the service was preserved. A private `XDG_RUNTIME_DIR` isolated subsequent verification. A later lint run found the test subprocess taint annotation and integer formatting; the narrow fixes preceded the final green gate. First pass is false.

For downstream lifecycle work: the launch deadline is immutable even after an extension or recovery watchdog recalculation. The strict private stdin handoff intentionally refuses mixed binaries; replacing an executable while an old supervisor is between admission and spawn remains unsupported. Existing running workers are recovered through unchanged identity/spool data. Non-deadline Unix stop behavior, fenced reconnect limitations and Windows job confirmation/cleanup remain intact.

### Task 4 — Hosted CI reconnect fixture correction

The original implementation head `150ccfd` failed macOS job [117749425235](https://github.com/turbokast/mythhelm/actions/runs/39152598817/job/117749425235): `TestSupervisorBeatLossAndReturn` took 31.51s and reported `envelope_test.go:424: the worker did not exit after a fresh beat` (workers 45.189s). The overall run later concluded cancelled: both Linux architectures succeeded, both Windows jobs were cancelled, and CI OK failed. Cancelled jobs establish no native pass. Integrating main in `49e946b` did not establish that the failure was resolved.

The original macOS trigger remains undetermined. Inspection established two independent fixture hazards: the first timed-out `waitExit` left `Process.Wait` active before the second call started another wait on the same process, and a one-shot beat could expire before a delayed worker observed it. Production renews supervision after each ingest tick. Correction `27f3344` uses one retained, joined process waiter that reports the actual error and renews beats every 100ms until exit. All three affected fixtures retain their one-second stale/no-beat non-exit assertion and unchanged 30-second return timeout. No production behavior changed.

The kept real-file regression `TestReconnectBeatProducerRenewsForDelayedObserver` first failed with the one-shot producer after 3.008s: its first and last beat timestamps were identical and stale for the delayed observation. With renewal, it and the three affected reconnect fixtures passed under race (3.652s). Independent code review returned Ready with no findings; coordinator review agreed. Neither these fixture corrections nor cross-platform vet identify the original native failure's precise cause or promote platform qualification.

Refreshed local gates with a private runtime directory: go-fmt PASS 0.1s; go-vet PASS 0.5s; go-test PASS 195.7s (supervisor 190.558s, workers 21.773s, CLI 77.101s); go-mod-tidy PASS 0.1s; golangci-lint PASS 4.1s with zero issues. Windows AMD64 and Darwin ARM64 full-repository vet passed. Hygiene passed after the fixture change (6.5s, 1043 files) and is refreshed again after this bookkeeping. Hosted CI and review remain coordinator-owned; first pass remains false.
