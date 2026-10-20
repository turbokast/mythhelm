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


### Task 5 — Accepted-running detach and queued stop precedence

Codex go-implementer implementation `1918f05`, PR #502, implements accepted ADR 0020 / design §2 without changing worker launch, identity or spool contracts. No module or immutable capture changed. Native execution stays owned by its worker; the foreground owner releases only its own lock.

| Contract / invariant | Retained acceptance evidence |
| --- | --- |
| Stop-less detach preserves executing state, live worker/native, source checkout and absence of a frozen candidate (I05/I06/I08/I18, v2 §2) | `TestDetachLeavesRunning`, including an actual interrupt counterexample that stops and freezes |
| Recovery takes the released owner lock, reuses one attempt/native and can detach again (I12/I18, v2 §2) | `TestDetachLeavesRunning` verifies one launch intent and one launch after recovery |
| Existing stop retains one request and pending guidance; recovered pending detach consumes no extra interrupt (I06/I18, v2 §2) | `TestDetachDuringStopping` |
| Early queued detach waits for accepted-running evidence or prior cancellation wins (I06/I21, v2 §2) | `TestDetachBeforeAcceptance` |
| Durable terminal evidence wins over outstanding detach | `TestDetachTerminalEvidenceWins` |
| Ready first stop is never discarded when detach is also ready (I06, v2 §2) | `TestQueuedStopSurvivesDetach`, twelve cases alternating enqueue order |
| Running and pending-stop descriptions differ, both map to 6; normal completion still maps to 0 | `TestDetachedOutcomeDescription` |
| Existing contracts remain intact | `TestObservatoryLinearBaseline`, `TestCtrlCOnceStopsAndExits130`, `TestCtrlCTwiceDetachesExit6`, `TestInterruptBeforeWorkerSpawnNeverLaunchesNative` |

Red-first evidence: with the API initially inert, the real worker continued to its finite execution deadline and the owner returned blocked rather than detached-running. The CLI initially described executing as ended and returned exit 1/internal. Disabling the hook made the stopping case return cancelled instead of pending detach, and the queued early case reach its deadline. A stop-then-detach mutation returned stopping/Detached instead of executing/DetachedRunning and failed the running assertion. Each restored behavior passed. The accepted-running fixture uses a finite 30-second envelope so recovery/liveness checks do not depend on a short deadline under full-suite load.

Independent code/architecture review found an important ordering defect in the initial general select: if the first interrupt and subsequent detach were both ready, detach could discard the stop. The independent overlay failed in 6.41s; the implementer's reproduction failed in 1.152s, and the retained regression failed four of twelve cases in 13.193s. The correction checks a ready first interrupt before returning stop-less detach, persists it through the shared user-stop path, and prioritizes ready detach for an already pending stop without reading another interrupt. All twelve corrected cases passed under race; the pending recovery assertion passed three race repetitions (28.661s). Fresh independent review of the corrected source returned Ready with a focused race pass in 37.187s. The final assertion-only lint cleanup names the same pending/finished predicates without changing them.

Portable fixture contract: the Windows fake's `stopLadder` uses a single immediate Kill rung, so terminal evidence can legitimately win before the next owner ingest. Unix ignore-term keeps the ladder pending and must return Detached/RunStopping. The Windows alternate path requires stopped/cancelled durable evidence and a candidate. Both paths retain exactly one unchanged stop request and one native launch. Local Windows AMD64 and Darwin ARM64 touched-package vet passed. A Windows test cross-build succeeded, but Wine could not reach behavior because its PATH lacked Windows Git; this is unavailable local runtime evidence, not a behavioral pass or a qualification claim.

Final source-sensitive gates: go-fmt PASS 0.2s; go-vet PASS 0.6s; full go-test race PASS 227.4s (supervisor 221.025s, CLI 78.697s); go-mod-tidy PASS 0.1s; serialized golangci-lint PASS 6.4s with zero issues; hygiene PASS 4.6s, clean 1044 files. A private runtime directory was reused across runs. All six source markers were fresh before the implementation commit. Bookkeeping refreshes hygiene and spec lint before publication.

Actual gate history: initial lint identified import grouping and two synthetic stop-request path annotations; later lint identified a compound test assertion, now represented by named predicates. Another lint invocation failed because a parallel lint process held its global lock; final lint ran through the gate wrapper under a shared file lock. Earlier complete race gates passed 394.3s, 372.7s and 298.8s before subsequent fixture/review/assertion changes; they are historical, not substitutes for the final refreshed evidence. First pass remains false. No test, stop confirmation or gate was weakened.


### Task 5 — Native ARM64 terminal-Recover oracle correction

PR #502 head `48e6e83` completed its original native CI with only the Windows ARM64 Go job and aggregate failing. [Run 39398735243 / Windows ARM64 job 118548638612](https://github.com/turbokast/mythhelm/actions/runs/39398735243/job/118548638612) reported `TestDetachDuringStopping` failing after 4.34s at `detach_test.go:186`; supervisor package took 319.120s. The returned recovery outcome was cancelled/continued with nil error and empty AttemptID/AttemptState. Other platform Go jobs passed. This is a recorded assertion failure, not a timing label or a reason to rerun CI unchanged.

Mechanism: after a first reattachment completes the stop, a following `RecoverWithHooks` sees RunCancelled and follows its existing terminal-state fast path in `internal/supervisor/recover.go` (the switch before `LatestAttempt`). It repairs the receipt and returns without populating optional attempt metadata. The original test required AttemptStopped in that returned metadata even though canonical stopped attempt and candidate already existed. Windows' one-rung Kill allowed this ordering naturally. The production fast path is unchanged.

Correction `57c563a` retains both the live final join and an explicit terminal-before-final-Recover case. An extra real recovery completes the stop before the final recovery, so the failing input is deterministic and needs no sleep, timeout increase or platform skip. With the old oracle, that retained case reproduced the cancelled/empty-attempt failure locally in 7.141s. With the correction, both cases pass under race (15.705s). Final recovery must return the exact run cancelled with neither detach flag; supplied attempt metadata must match if present. Independent canonical queries still require the exact stopped attempt belonging to that run, cancelled run and a nonempty candidate bound to the attempt. Pending reattachment retains its untouched extra interrupt; request bytes stay identical; exactly one stop request and one native launch remain required.

Fresh independent read-only review returned Ready for the exact test blob `0c0ecc588b03c226bb3c7cbdcd2534c1107c669b` (SHA-256 `72dd7b23c7238b18d5571552dccfeef2dd265e78a4407983e6f99ab74c4db513`), with the same two real-worker cases passing under race in 15.709s. No production, module or platform qualification changed.

Refreshed local gates on this source: go-fmt PASS 0.1s; go-vet PASS 0.5s; full race go-test PASS 262.5s (supervisor 259.016s, CLI 75.930s); go-mod-tidy PASS 0.1s; serialized golangci-lint PASS 5.9s, zero issues; hygiene PASS 5.6s, clean 1044 files. All six markers were fresh before the signed test commit. Own bookkeeping refreshes hygiene and spec lint; native CI must validate the pushed correction rather than treating this local Linux evidence as a Windows pass.


### Task 6 — Current-attempt evidence and read-only supervision

Implementation `072a412` adds the design §4 backend projection in the four listed viewmodel files. Synthetic evidence preserves all five usage labels, exact decimal strings, reported zero, unknown quantity and repeated scopes. Model and agent-tail attribution are stricter than the historical progress/native-result synthetic-feed fallback: a missing current-attempt model remains unknown, and previous or unattributed model events cannot populate it. The tail retains exactly 200 current-attempt events with an accurate omitted count and bounded backing capacity.

Production supervision has no qualified no-lock ownership probe. It therefore passes owner unknown to the pure classifier and never infers unheld from a process table. A readable, non-future beat at most one second old supports supervised; missing, malformed, stale or future evidence stays unknown. Explicit unheld fixtures exercise conditional classification only. The read-only integration fixture checks no owner-lock creation, real held-owner recovery refusal, the next owner's successful acquisition, actual settled-run recovery after release and byte-identical canonical evidence under later polling.

Red-first observations: inert fields omitted the model, raw usage and detachment capability; malformed model and usage-read errors were swallowed; the tail was empty; inert supervision failed fresh and explicit-unheld cases. Each corrected acceptance case passed. Retained synthetic fixtures also rejected overlay mutants that fold stale models (observed unattributed model leak), retain all 205 events with zero omitted, acquire a viewer lock (observed owner-lock creation), or fabricate unheld ownership (observed unsupervised instead of unknown). The named race acceptance run passed in 7.563s; a later final-tree package race run passed in 9.539s.

Actual gate history: the first lint attempt could not resolve the pinned module from the restricted network. Subsequent lint found a range-loop modernization and a test-owned filesystem-read annotation, then the root-scope traversal concern; the test uses `os.OpenRoot` and guarantees joining its concurrent recovery probe. Final lint on that source passed with zero issues. Full Go gates failed at 289.6s and 104.8s on existing CLI final-read paths. Diagnostic overlay runs retained assertions and printed the actual joined deadline errors: the stranded-run case failed with schema-version or run-list reads exceeding `finalRunJournaled`'s 500ms context. Full CLI runs failed with both this viewmodel (112.972s) and the unchanged base viewmodel (112.660s); the isolated stranded-run test passed 20 race repetitions (14.967s). These observations establish neither a Task 6 source defect nor a timing qualification. No timeout, assertion or CLI source was changed by this task.

The coordinator assigned the shared read boundary separately. Signed merge `11bbde1` integrates exact reviewed public PR #508 source `7bc90af` for validation while its native CI is pending. This task owns no part of that two-file maintenance change, and cannot merge before its prerequisite is verified on main. Feature-source hashes remained unchanged during integration. Final integrated gates and hosted CI are recorded only after their actual results.

Signed merge `377f322` subsequently integrates PR #508 final public tip `9ab4ba9`, whose only change after `7bc90af` is two explicit I09 (v2 §2) policy-test comments. All four reviewed Task 6 source hashes remain unchanged. Public task publication still waits for the prerequisite merge and final main ancestry integration.


Integrated validation on signed `377f322` / exact PR #508 tip `9ab4ba9`: go-fmt PASS 0.4s, go-vet PASS 1.1s, go-mod-tidy PASS 0.1s, serialized golangci-lint PASS 10.7s with zero issues; full Go race gate PASS 391.8s (supervisor 364.288s, CLI 149.974s, workers 25.351s, E2E 33.645s). The existing CLI read-path failures did not recur in that integrated run. The coordinator released the full-suite slot after the other worker finished; this run used actual HOME, private XDG state/runtime and no inherited MYTHHELM variables. No test concurrency cap, timeout, assertion or Task 6 source was changed. Source-sensitive markers match the integrated tree; bookkeeping refreshes hygiene. Hosted checks remain a distinct requirement, and first pass stays false.


PR #508 landed at `d446dd2` before task publication. Signed main integration `8f2b877` establishes that ancestry without changing the tested source tree: all four reviewed feature hashes and both parent CLI blobs remain identical, and source-sensitive markers remain fresh. PR #509 carries only the four viewmodel files and this task's three required records. The integrated named race acceptance run passed in 9.528s (`TestSupervisionClassification` 0.00s, `TestObservatorySnapshotEvidence` 1.62s, `TestViewPollingDoesNotOwnRun` 4.50s, `TestAgentEventTailBound` 8.47s); the coordinator independently repeated all four in 9.554s with unchanged source hashes. Own entry validation and public hygiene are refreshed after bookkeeping. Native CI for this task must still pass at its pushed head; prerequisite CI does not replace it.


### Task 6 — Native Windows lock-fixture correction and shared watchdog observations

PR #509 source head `bfb70c5` failed the full Go job on both Windows architectures in run [39431008400](https://github.com/turbokast/mythhelm/actions/runs/39431008400). ARM64 job [118656750467](https://github.com/turbokast/mythhelm/actions/runs/39431008400/job/118656750467) reported `TestViewPollingDoesNotOwnRun` at the original line 952 after 0.42s (viewmodel package 5.182s); AMD64 job [118656750314](https://github.com/turbokast/mythhelm/actions/runs/39431008400/job/118656750314) failed the same assertion after 1.22s. The fixture attempted to read `owner.lock` while its first byte was exclusively locked by the real Windows `LockFileEx` implementation. That mandatory byte-range lock differs from Unix advisory locking. This was a fixture operation, not a failure of production `Load`; both failures were investigated before changing it.

Signed correction `58d52d5` changes only `viewmodel_test.go`. After the initial viewer poll proves no lock exists, it seeds known lock bytes before real ownership is acquired. While held, every other canonical file remains byte-compared, and the lock's identity, size, mode and modification time are checked. After release its exact seeded bytes must match. Both real held-owner recovery refusals, subsequent owner acquisition, actual settled-run recovery and later complete byte snapshots remain unchanged. The focused corrected test passed under race in 3.64s (package 4.708s); the coordinator independently repeated it in 4.704s. The lock-creation mutant still fails the original absence assertion in 0.135s, and the fabricated-owner mutant still fails the unknown-supervision assertion in 0.107s. Independent focused code/architecture review returned Ready for SHA-256 `51630330d1d64e457af5cc68467558b3930f5ea50818234401f94b4ee478e345`. No runtime code, timeout, ownership assertion or test parallelism was changed.

Mandatory gate refreshes then failed a distinct existing CLI test: `TestReviewResumePastEndReturnsPromptly`, original line 917, reported its unchanged five-second watchdog. The first case took 11.22s total (CLI 174.143s, gate 178.5s); the second took 10.61s (CLI 213.671s, gate 222.5s). The corrected viewmodel package passed; the shared watchdog's cause remains **unknown**. A bounded sequential diagnostic pair preserved the watchdog and assertions while adding failure-only elapsed/stack observations through an uncommitted overlay. Full CLI runs passed with both the Task 6 projection (135.801s; target 7.03s including fixture setup) and the main projection (164.687s; target 7.97s including setup). No failure-only stack fired. The pair used `-v` and a single-package command, unlike the full gate, so those passes neither identify a mechanism nor establish a correction. Retries stopped after the authorized pair and final gate; no bound or assertion was relaxed.

The coordinator assigned the shared investigation separately. Reviewed public PR #511 source `7561ae5` adds failure-only observations while preserving the original five-second watchdog, select, trailer assertion, parallelism and production behavior. Its exact sole parent delta is `internal/cli/tui_test.go`, blob `08ded527c2068b8c43f8efa8ce510a5681817933`. Signed merge `0d1d059` integrates it for one new combined-tree gate; this is new diagnostic test source, not a claimed fix for the historical timeout. Prerequisite native CI and actual merge remain separate requirements. The task's final PR scope must exclude this parent file after verified main integration. First pass remains false, and final combined-tree results are recorded only after observation.


The coordinator serialized heavy validation after the Windows worker released its full-suite slot. On exact combined source `0d1d059`, the single authorized new actual `go-test` gate passed in 280.7s (CLI 110.062s, supervisor 271.619s; corrected viewmodel package cached from its earlier passing run). The reviewed PR #511 diagnostics were active; no watchdog failure recurred and no new failure-only dump was produced. This pass establishes the tested tree's gate result, not the cause or correction of either historical watchdog failure. No further broad retry was needed or run. Light gates on the same fingerprint passed: fmt 0.2s, vet 0.6s, tidy 0.1s and serialized lint 5.5s with zero issues. The task's own records refresh hygiene separately; prerequisite native CI/actual merge and final own-scope validation remain distinct. First pass stays false.


PR #511 actually merged at `5ebf9fb` before final task publication. Signed main integration `f8e3029` produces no source-tree change: all four reviewed feature SHA-256 pins and the reviewed parent CLI test blob remain identical. The exact three-dot diff now contains only the four own viewmodel files and this task's three records; no parent CLI file remains. All five source-sensitive markers stayed fresh, so no additional broad suite was run. Own bookkeeping refreshes only hygiene and entry validation. Final exact-head task CI remains required; the historical watchdog cause is still unknown, and the parent's diagnostic change is not represented as its correction.


### Task 7 — Typed controls and joined action lifetime

Codex tui-implementer source `d4afd6c`, corrected through `e0ecdfd`, `8920e44`, `0054480` and final `4adae02`, PR #513, implements design §3 in exactly the ten listed TUI files. Only this task's three required records are added. Config's zero value remains inspect; Task 8 owns real CLI signal/coordinator wiring and Task 10 owns packaged lifecycle proof. No worker/supervisor runtime, canonical persistence, module or immutable linear capture is edited.

Initial retained regressions observed q's missing Back row and the old first-Ctrl-C action cancellation. Named table tests cover live/inspect phases, no snapshot, missing controls/admission/worker, safe direct keys, shared visible reasons, Cancel/Back defaults, capability qualification and navigation. `TestSingleKeypressNeverConfirms` and `TestApplyShowsExactEffects` remain byte-identical. `TestStopLabelsRequested` changes only the specified stop reason and direct-key palette hint. Existing navigation fixtures now provide Actions when asserting an enabled action because nil controls are explicitly disabled by this task's contract; recover's hint reflects its new direct key.

Independent code/architecture review found two important lifecycle gaps in the first source: parent cancellation after terminal loss was no longer observed, and a failed inspect stop could exit as ordinary view success. Retained `TestTerminalLossThenParentCancellationJoinsAction` failed both modes in 0.337s, then passed after shutdown selected parent cancellation, cancelled the original call and joined its cleanup. It observes the cancellation acknowledgement, holds cleanup, rejects early Run return, and preserves actual cancelled completion with exactly one exit callback. `TestInspectStopErrorExitIntent` initially failed its unknown-effect case in 0.087s; the correction tracks uncertainty without claiming request delivery.

Follow-up review proved the legacy `stopConverged` / `stopConfirmed` display state was too broad to settle operation uncertainty: error followed by interrupted/created/admission/recovering/applying/unknown snapshots falsely exited successfully (retained regression FAIL 0.156s, restored PASS 1.283s). Actual stop evidence is now independent, scoped to a stopped attempt snapshot or a validated stopped event for the exact nonempty current run and attempt. Missing run, missing attempt and foreign-run/current-attempt events reproduced false success before the guard (FAIL 0.163s); matching before/after-result positives, foreign/malformed negatives and pending/empty/unknown nil-error stop results remain in the suite. The prior synthetic event allowance for display rendering is unchanged.

Additional retained prelaunch proof found that generic created-state display convergence could re-enable a typed stop already sent to the owner (FAIL 0.025s). The sent intent now remains coalesced across polls and first Ctrl-C reports it. A final explicit live/no-snapshot terminal-loss overlay failed wired/nil-control rows in 0.038s because ExitFinishHeadless lost the early detach disposition; the corrected source emits ExitDetach/unknown, queues Detach once when wired and sends no stop. Independent final code/architecture review returned Ready at exact `4adae026fd75aeb22aa64f10d9dc7cbc793b1d69`; all named/new acceptance and unchanged guards passed under race in 2.141s, and the coordinator independently repeated the review regressions.

Counterfactual overlays produced assertion failures for unconditional final action cancellation, abandoning the join, duplicate apply and headless callbacks, nil-error stop confirmation, stop forwarding during verification/apply, reset interrupt counts, one-key confirmation, ignoring headless parent cancellation, successful unknown stop error, display-only error/result convergence and prelaunch repeated stop. Restored code passes the corresponding kept positive and negative cases. The dedicated early-loss and attribution regressions retain their own broken-input contrasts. No timeout, assertion or concurrency cap was weakened.

Actual gate history: the initial pinned lint lookup was denied by the sandbox's network boundary. With permission it reported UK spelling, WaitGroup.Go modernisation, string-builder/bool formatting, exported const-block comments and a boolean simplification; these mechanical corrections stayed in the three allowed files and received narrow independent re-review. Final source gates: go-fmt PASS 0.8s; go-vet PASS 5.3s; go-mod-tidy PASS 0.3s; serialized golangci-lint PASS 16.4s, zero issues; full repository race go-test PASS 428.5s. Validation preserves actual HOME, clears inherited MYTHHELM variables, uses private mode-700 XDG directories and the shared temporary Go cache. Windows AMD64 and Darwin ARM64 full-repository `go vet ./...` both exited 0. Source gates use the exact final fingerprint; own bookkeeping refreshes hygiene and spec lint. The historical CLI watchdog cause remains unknown; no watchdog or diagnostic assertion changed here. First pass is false. Native exact-head CI remains required before merge and establishes platform runtime evidence separately from local/cross-build checks.


### Task 7 — External review round 1

PR #513 review comment 4327670961 correctly identified a wall-clock dependency in the 250ms headless-parent-cancellation observation. Test-only commit `0f264cc`, with mechanical lint follow-up `2d5ad68`, replaces it with a self-contained `testing/synctest` bubble: model, channels and contexts are created inside the bubble, the snapshot loader is immutable, terminal input is nil and output is discarded with no renderer. Unrelated visual motion is off: an initial probe with full motion exposed Bubble Tea's bounded visual Tick commands remaining after the bubble root exited, rather than an action-lifetime failure. No production code or existing visual test changed.

After cancellation, synctest.Wait observes durable blocking before the nonblocking cancellation assertion; there is no cancellation latency threshold. Cleanup is registered before assertions, once-releases both fallback/cleanup gates and joins the same Run/action-command completion even if an assertion fails. The original ignored-parent-cancellation mutation now fails by assertions in both modes in 0.075s, with no panic/deadlock or surviving test operation; restored focused race tests pass. Existing result, no-early-return, cancellation, initial preservation and callback-once assertions remain.

Review comment 4327671015 requested refusing emergency detach without qualified capability. AC-6.10/OQ-29 and accepted ADR 0020 expressly require the emergency second-Ctrl-C/terminal-loss exception and its qualification disclosure, so runtime behavior is preserved. New `TestEmergencyDetachQualification` retains supported, unknown, unsupported and missing-admission cases. Unqualified deliberate d/q are blocked; the first interrupt delivers one typed stop, the second one detach with ExitDetach and the exact qualification notice; supported has no extra qualification notice. The proposed refusal mutation fails all three unqualified cases in 0.079s. Task 8 must also print that disclosure in actual CLI emergency output after deriving/preserving canonical admission truth, including early latch; a notice in a disappearing view alone is insufficient. OnExit still has only its approved kind/phase fields, with no API extension or inferred platform support.

Independent code/architecture review returned Ready for exact `2d5ad68849f29b63c1f871351788f4791bc4d944`, test blob `3bab9efb82fbc5dcc1e6d0f90eed10bc7d3dd935`; the sole final follow-up replaces a shadowing test variable declaration with assignment to the outer model after Staticcheck SA4006. All named/new acceptance and guards passed under race in 1.853s on the initial test correction; both changed tests passed at the final binding in 1.132s and the coordinator independently repeated them. The final full race gate passed in 173.1s, including TUI 11.702s; unchanged core results were reused by Go's test cache. Formatter PASS 0.7s, vet PASS 1.7s, tidy PASS 0.2s and individually serialized lint PASS 12.7s with zero issues share the final source fingerprint. Windows AMD64 and Darwin ARM64 full-repository vet both exited 0. Actual HOME/private XDG/no-cap validation rules are preserved; only own records refresh hygiene/spec checks afterward. First pass stays false. Hosted Sourcery status was skipped, and is not counted as a completed review. Native exact-head CI and remaining review status remain coordinator prerequisites before merge.
