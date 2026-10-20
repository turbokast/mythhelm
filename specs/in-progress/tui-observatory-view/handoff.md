# tui-observatory-view — Hand-off

> One section per task. Each task replaces its own `<!-- pending -->` line in its pull request with what
> dependent tasks need: what it produced (the API and files as shipped), what a later task must
> know, and any deviation that changes a later task's inputs. `/run-spec` puts the sections of a
> task's dependencies into that task's dispatch prompt. Open questions and research stay in
> `scratchpad.md`.

## Task 1 — Capture the unchanged linear output contract

- **Produces**: `TestObservatoryLinearBaseline` and immutable `internal/cli/testdata/observatory-linear.json`, containing seven exact captures, producing invocations, base commit and synthetic events/receipt/usage inputs; PR #485, implementation `a183347`.
- **For dependents**: preserve every capture and the existing accessible/render/review/TUI assertions. All normal Go gates run the baseline; there is no regeneration switch. Normalisation is narrow and retains state/provenance/usage/exit/record fields, candidate content/binding and generated-reference relationships.
- **Capture scope**: actual review accessible/JSONL, both renderers, actual run plain/JSONL refusal (exit 2), and actual successful fake demo with `--accessible --after 1`. Fixed journal cases cover initial summary and created event; demo requires events 2–23 and final cursor 23.
- **Review correction**: Demo capture owns its temporary root and requires a unique state directory, so event 4 truncation cannot prevent receipt synchronization; the displayed path prefix and exact baseline remain checked. Probe failures include exit/stdout/stderr.
- **Controlled timing**: the test-only output consumer waits at demo event 4 for journaled `receipt.written`, with a finite timeout. It inspects actual Git base-to-candidate changed paths and exact `demo.txt` content before demo cleanup. This is deterministic fixture timing, not a production race fix.
- **Task 8 discovery**: natural-timing `watchRunTerminal` can cancel at ready_for_review/event 22 before receipt.written/event 23, leaving cursor 22 despite a valid final receipt. Add a natural-timing terminal/receipt regression during CLI lifecycle work and reconcile stream completion with pipeline completion; never normalise a missing event away.
- **Validation**: two consecutive original base captures matched; kept semantic counterfactuals pass; complete Go race gate passed (288.6s), lint/style corrected, all six markers fresh, independent review Ready with two focused race passes (11.307s). No native account or Windows runtime qualification is claimed.

## Task 2 — Record the lifecycle and presentation contracts

- **Produces**: Proposed `docs/decisions/0020-observatory-lifecycle-and-themes.md` and separate TUI-only palette table in `docs/assets/brand/README.md`; PR #484, implementation `54f859d`.
- **For Task 3**: Record explicit approval of the concrete ADR revision before runtime implementation. Fresh independent architect review found no critical or important issues, but is not maintainer approval or runtime evidence.
- **For Tasks 4–10**: Preserve first-request precedence, worker-pinned deadlines, durable final reason, stop-less Detach, joined action lifetime, single signal owner and distinct client/canonical outcomes. Mixed handoff versions fail closed; qualification and reconnect limitations remain explicit.
- **For Task 11**: Nine missing TUI-only rows are supplied. Observatory attention already exists as shared Amber (dark); shared Colours rows remain byte-identical. The equality test must compare embedded themes after their implementation, not claim current themes already match.
- **Verification**: Spec lint and hygiene pass; changed-document local links resolve; shared Colours section comparison and diff whitespace check pass. All ADR runtime test names are future obligations, not passing claims.
- **Deviations**: None.

## Task 3 — Approve the concrete ADR

- **Produces**: Accepted ADR 0020 with delegated maintainer authorization record; PR #487, decision commit `f458f15`.
- **Authority and revision**: The coordinator exercised the maintainer's explicit standing delegation after reviewing the full ADR, accepting source `54f859d` / merged `946ba3f`, original SHA-256 `b3f22e5ae1cf34ff48be5313a9dad09dd3c966a875fcf4e660c943734d42ca05`. This does not represent personal review of later content, a digital signature, signed product approval or formal autonomy grant.
- **For Task 4 and later**: The runtime/theme decision is unchanged. Preserve its deadline, first-request, durable conclusion, typed intent/action lifetime, signal ownership and compatibility contracts. All runtime tests and qualification evidence remain future obligations; ADR 0007 remains proposed.
- **Deviation**: The decisions index heading/status was updated with coordinator approval to match the accepted record.
- **Verification**: Exact source/merged proposal bytes and digest checked; spec lint, hygiene, diff checks and entry-check pass. Independent architect review of the original contract found no critical or important issues.

## Task 4 — Make the worker own its execution deadline

- **Produces**: Required `workers.Launch.ExecutionDeadline time.Time` / `execution_deadline`, `workers.DeadlineRequester = "execution-deadline"` and `workers.DeadlineReason = "envelope_deadline_exceeded"`; implementation `bb152be`, PR #497. Existing Spawn/RequestStop signatures and worker identity/spool schemas are unchanged.
- **For Tasks 5–10**: `pipeline.attempt` pins its computed deadline with nonce/generation. The worker enforces it independently of generation and supervisor beats; no detach, heartbeat or recovery path may replace or remove that pin.
- **Stop precedence**: Timer and file polling share `acceptStop`; an accepted ladder or earlier atomic request-file winner stands. Deadline reason is derived from that winner. Preserve billing/protocol policy precedence and never relabel an earlier user stop when a timer fires.
- **Failure handling**: Unreadable/malformed request evidence or request creation failure aborts owned execution and leaves an interrupted result. Unconfirmed/escaped ownership remains unresolved. Deadline stops have a bounded post-ladder native-exit wait on Unix; Windows retains its general bounded wait and job confirmation ordering.
- **Recovery and exit**: Shared `conclude` maps a stopped attempt carrying DeadlineReason to blocked with that reason; no volatile deadlineHit remains. Only a successfully spooled deadline episode bypasses fenced reconnect waiting. Other fenced terminal episodes retain their existing reconnect behavior.
- **Compatibility / Task 19**: Missing/zero/malformed handoffs and expired native starts fail closed. Strict legacy workers reject the added field. Finish/drain supervisors before replacing binaries; the admission-to-spawn mixed-binary replacement window is unsupported. Existing running workers retain their immutable pin and unchanged recovery evidence. Extensions update owner policy/watchdogs, never a live worker's deadline.
- **Launch boundary**: Keep the expiry recheck after durable intent/job/containment preparation and the refusal cleanup of containment resources, both stdout pipe endpoints and a prepared Windows job. Helper-only zero-pin launcher fixtures bypass the handoff; actual Spawn fixtures all supply finite deadlines.
- **Evidence**: All named acceptance tests pass under race, including two-second real workers at generations 0/7 and autonomous deadline recovery. The immutable Task1 capture bundle and Windows expected overlay are unchanged; the refreshed full race gate (195.7s) and cross-platform vet pass. Reconnect fixtures retain one process waiter across both phases and renew returned supervision; preserve their stale/no-beat assertions. Original macOS CI failure and fixture correction evidence are retained in scratchpad. Native CI remains the platform runtime evidence; this task does not promote qualification.
- **Verification environment**: Use a private `XDG_RUNTIME_DIR` for local full-suite validation when a host supervisor is already running; preserve that service. Gate history and red-first counterfactuals are recorded in scratchpad.

## Task 5 — Add a stop-less owner detach control

- **Produces**: `Hooks.Detach <-chan struct{}`, `Outcome.DetachedRunning bool` and the exact two-run-ID `RunningDetachedNotice` from design §2; implementation `1918f05`, PR #502. `Detached` and its pending-stop notice keep their existing meaning.
- **For Tasks 6–10**: nil disables detach. A queued early request remains pending until the accepted worker is running, or prior terminal evidence wins. Running detach leaves the run executing, its worker/native live and its candidate unfrozen; the owner lock is released for recovery of the same attempt.
- **Stop precedence**: a first interrupt already ready when detach is selected is persisted before returning pending-stop detach. For an already pending run/attempt stop, a ready detach wins without consuming another interrupt. Preserve the worker's durable first-request winner and do not turn these controls into two interrupts.
- **Recovery**: normal and recover watch continuations check both detach flags before freeze/conclude. Reattachment can itself detach with no replacement native. Terminal Recover repairs its receipt before loading an attempt, so returned AttemptID/AttemptState may be absent; use exact canonical attempt/run/candidate evidence to prove a stop, never nil error alone. The immutable worker deadline from Task 4 is untouched.
- **For Task 8**: `runExit` maps both detach flags to exit 6; `describe` uses the exact running notice for `DetachedRunning` and retains separate unconfirmed-stop wording for `Detached`. Later CLI wiring must emit the deliberate notice once. Linear callers still leave Detach nil and preserve the immutable Task 1 outputs.
- **Evidence**: real owner/worker tests cover native/worker liveness, source preservation, released ownership, no candidate, no relaunch, queued pre-admission detach versus prior cancellation, terminal precedence and existing stop recovery. Twelve retained ready-control cases cover both enqueue orders; recovered pending detach checks the extra interrupt stays queued. The actual interrupt counterexample produces a stopped candidate.
- **Platform limits**: the stopping test requires pending detach on Unix; Windows may naturally complete its one-rung Kill before the next ingest and must then prove stopped/cancelled evidence and candidate. Native Windows CI remains runtime evidence; cross-vet passes, while local Wine could not start fixture Git. No detachment qualification is added here; later capability gates still apply.
- **Validation**: refreshed full race gate 262.5s; original source/architecture recheck Ready (37.187s). Native Windows ARM64 exposed a terminal-Recover oracle mismatch; correction `57c563a` retains a forced terminal-before-final-Recover case that failed locally first, and both cases pass under race (15.705s; independent review Ready / 15.709s). Request bytes/count, one launch, queued interrupt and exact durable stopped/candidate assertions remain. First pass stays false; final lint has zero issues and markers are refreshed. Native CI is still required after the correction.

## Task 6 — Add journal-backed observatory data

- **Produces**: `Snapshot.Model`, `Usage []journal.UsageRow`, `AgentEvents []journal.Event`, `AgentEventsOmitted`, `Supervision`, and `Admission.WorkerDetachment`; implementation `072a412`, PR #509. `Load`, `EventsSince` and `EventsSinceLimit` keep their signatures and read-only contracts.
- **For Tasks 13–14**: Model defaults to `unknown` and folds only current-attempt `attempt.native_session` evidence. Usage is the exact insertion-ordered journal result, including repeated scopes, reported zero and unknown quantity; never aggregate or relabel these rows. Usage read failures remain errors.
- **Bounded agent evidence**: Only events attributed to the current attempt enter the ordered latest-200 tail. `AgentEventsOmitted` counts earlier current-attempt rows; the retained backing capacity is at most 200. Existing progress/native-result folding preserves its historical handling of unattributed synthetic feeds.
- **Supervision**: Production reads the existing beat and always passes owner `unknown`; fresh means a non-future, readable beat at most one second old. Stale, missing or malformed evidence remains unknown. No owner lock is acquired, no process-table absence is interpreted, and no OS qualification is added.
- **Conditional classification**: The pure `classifySupervision` fixture accepts independently proven `unheld`, which overrides a recent beat. This is a rendering seam for a future qualified observation, not evidence that production currently observes absent ownership.
- **Rendering remains downstream**: Task 13 acceptance covers title/model disclosure and supervision wording; Task 14 covers the agent tail and provenance rows. Keep untrusted payload sanitisation at rendering boundaries; this backend projection does not render new text.
- **Read-only/recovery evidence**: Retained tests poll before any owner lock exists, while a real owner blocks recovery, after release and after actual settled-run recovery repairs its receipt. Canonical bytes stay unchanged under polling, and the next real owner still acquires the lock.
- **Validation history**: Scoped race acceptance tests reject stale-model, unbounded-tail, viewer-lock and fabricated-owner mutants. Earlier full gates exposed the shared accessible journal-read deadline failure with both current and base viewmodels. Signed parent integrations `11bbde1` / `377f322` bring reviewed PR #508 source `7bc90af` and its final comment-only tip `9ab4ba9`; the integrated full race gate passes in 391.8s with CLI149.974s/supervisor364.288s. That separate maintenance source landed in PR #508 / `d446dd2`; signed main integration `8f2b877` preserves the tested source and excludes its CLI files from this task's final diff. First pass remains false.
- **Portable ownership fixture**: Native Windows refused the original fixture's read of an actively byte-locked file. Correction `58d52d5` byte-checks every other canonical file while held, compares the lock's identity/size/mode/modtime, and checks its exact seeded bytes after release; all real ownership assertions remain. Focused code/architecture review returned Ready. Later CLI watchdog failures remain of unknown cause; reviewed PR #511 adds diagnostics without changing the watchdog or production behavior. Its exact public source is integrated in `0d1d059`; the single required new combined-tree race gate passed in 280.7s with CLI 110.062s/supervisor 271.619s. The historical watchdog cause remains unknown. PR #511 actually merged at `5ebf9fb`; signed main integration `f8e3029` preserves all reviewed source pins and establishes the exact seven-file own scope without the parent CLI file.
- **Deviations**: The fixture correction is consistent with design §4; no runtime or source-scope deviation.


## Task 7 — Implement safe exit rows and typed TUI controls

- **Produces**: Exact design §3 `ClientMode`, `ClientSignal`, `ExitKind`, `ExitIntent` and `ActionCompletion`, Config controls/callbacks and joined typed-signal ownership; implementation `4adae02`, PR #513. `Run` and `Actions` signatures remain unchanged.
- **For Task 8**: Config defaults to `ModeInspect`; explicitly select `ModeLive` for live run/demo. RequestStop/Detach callbacks must enqueue promptly and coalesce; they never signal or wait for processes. Tea uses `WithoutSignalHandler`; CLI owns OS signals, including external interrupts and terminal loss. Input EOF is not terminal loss.
- **Early terminal loss**: Live no-snapshot/created/admission/pre-running loss emits `ExitDetach` (phase `unknown` without a snapshot) and queues Detach once if wired. The owner must latch until accepted-running or preserve an earlier outcome. Nil controls remain visibly disabled for deliberate actions; emergency loss still closes with a truthful intent.
- **Action lifetime**: Every confirmed action has one stored result and closed-once done handle observed by both Tea and shutdown. Terminal loss closes/joins view pumps, preserves the same action, then reports OnHeadlessAction once after join. Later parent cancellation cancels that same operation and joins cleanup; explicit double-quit also cancels/joins unresolved. OnExit runs at most once before intended quit; unrelated startup/program errors manufacture no intent.
- **Stop truthfulness**: OnHeadlessAction preserves exact StopState, receipt, recovery result and error. `stopped` differs from `stop_requested`; nil Err with empty/unknown StopState remains unresolved. Headful inspect uncertainty also survives generic snapshot convergence; only a stopped attempt snapshot or validated current-run/current-attempt stopped event resolves it. The older display pending-label flag is not actual confirmation authority.
- **Policy and counts**: q starts Back, confirmations start Cancel and recheck gates. All control surfaces share gates. The session interrupt count never resets with run state; action quit arming is separate. Deliberate detach uses admitted `worker_detachment=supported`, never GOOS. Pending prelaunch owner-stop intent cannot be sent twice after a display poll.
- **For Tasks 8/10**: TUI callback/unit tests do not establish OS signal ownership, actual guarded apply effects or packaged lifecycle qualification; those remain the named downstream acceptance obligations. Existing linear captures and original confirmation/effect guard tests are unchanged.
- **Validation**: Independent code/architecture review Ready; named/new lifecycle tests pass under race, negative overlays reject cancellation/abandonment/duplicate/false-confirmation shortcuts, and the full race gate passed in 428.5s. Initial lint network denial and mechanical findings, plus review regression history, remain recorded; first pass is false. Native exact-head CI remains distinct evidence.
- **Deviations**: No source-scope or product-contract deviation; Task 8 owns CLI integration.

## Task 8 — Wire live and inspect lifecycle in the CLI

<!-- pending -->

## Task 9 — Preserve detached demo state and usable recovery guidance

<!-- pending -->

## Task 10 — Prove lifecycle through the packaged binary

<!-- pending -->

## Task 11 — Migrate theme tokens and contrast validation

<!-- pending -->

## Task 12 — Add semantic spans and the closed glyph vocabulary

<!-- pending -->

## Task 13 — Render the header and gated NOW/NEXT actions

<!-- pending -->

## Task 14 — Render agent, evidence and verdict content

<!-- pending -->

## Task 15 — Install all responsive ladder views

<!-- pending -->

## Task 16 — Compose overlays with semantic colour checks

<!-- pending -->

## Task 17 — Resolve themes and suppress delayed OSC replies

<!-- pending -->

## Task 18 — Expose --theme across launch branches

<!-- pending -->

## Task 19 — Update user-facing limits and instructions

<!-- pending -->

## Task 20 — Re-record the synthetic TUI demonstration

<!-- pending -->
