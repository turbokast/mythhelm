# supervisor-service — Retrospective

## Review Summary

- **Range**: 2e1aff4..f61fff8 (PRs #250, #251, #253, #255, #258, #262, #266; fix PRs none in range; post-range fixes #284 and #291, both merged)
- **Reviewer**: code-reviewer, architect
- **Findings**: critical 0, important 9, suggestion 12 (confirmed); rejected 0
- **Open critical**: 0
- **Vendor review**: skipped (codex, muse, jev all disabled)

| Severity | Finding | Anchor | Disposition |
|---|---|---|---|
| important | Replay bypasses capability-token authorization (token not in idempotency digest) | `internal/control/control.go:140` | fix PR (include token digest in key or re-authorize on replay + test) |
| important | Control ownership errors exit `ExitInternal` instead of `ExitOwnership` | `internal/cli/exit.go:53` | fix PR (map root-conflict/held errors) |
| important | `status` returns `capability_unsupported` for no-instance state error | `internal/control/server.go:80` | fix PR (correct code) |
| important | Reservation expiry unwired; crashed holder blocks re-reserve forever | `internal/control/reserve.go:70` | issue #280 (design decision: expiry-aware held check vs stream-4 reconcile; auto-release risks I05) |
| important | Windows same-logon (SID+LUID) stricter than Unix same-UID, no spawn fallback | `internal/control/transport_windows.go:43` | issue #281 (decision: relax to SID-only vs document stricter rule + availability cost) |
| important | Boot generation recorded/reported but never fences stale intents (design §3 gap) | `internal/control/lock.go:99` | design §3 amended + §13 row in finalize; issue #282 for stream-4 implementation |
| important | Parallel error vocabulary now live (control codes predate vocab task 6) | `internal/control/control.go:20` | issue #283 (swap to *v2contract.ControlError, stream-3/4 pickup; includes plain-code errors) |
| important | ADR 0014 + SUPPORT.md + §13 Windows rows contradict shipped pipe transport | `docs/decisions/0014-service-topology.md:67` | ADR + §13 amended in finalize PR; SUPPORT.md wording in second fix PR (out of finalize scope) |
| important | AC-6.3 operator self-scope undisclosed in design §13 register | `internal/control/filter.go:42` | §13 Partially-met row in finalize PR |
| suggestion | Client response path skips CheckIngress (design §5 divergence) | `internal/control/transport.go:89` | kept; trusted local peer, 1 MiB cap holds; named in retrospective |
| suggestion | Tx-lock serialization replaced claim-before-execute; `claimed` state dead | `internal/control/ledger.go:44` | kept; contract holds and pinned by tests; `claimed` immovable (shipped 0004); recorded as deviation |
| suggestion | `operations` ledger grows without bound, no pruning story | `internal/control/ledger.go:64` | kept; follow-up or stream 3/4 |
| suggestion | Windows `userKey` from spoofable `$USERNAME`; `checkDirOwner` no-op on Windows | `internal/control/lock_windows.go:39` | kept |
| suggestion | `ensureLockDir` skips mode-bit check `ensureSocketDir` performs | `internal/control/lock.go:126` | kept; fail-closed later at Listen |
| suggestion | `ownership_unresolved`/`CheckTerminalEntry` specified but unimplemented | design §6/§7 | kept; trim or mark stream-4 |
| suggestion | Dead `clientBudget` const | `internal/control/client.go:23` | removed in fix PR (opportunistic) |
| suggestion | Per-object revisions vestigial (only `assign` advances) | `internal/control/server.go:134` | kept; define or drop later |
| suggestion | No token issuance over the protocol (`MintToken` in-process only) | `internal/control/server.go:141` | kept; coherent until stream 4 wires admission |
| suggestion | Shared-HOME multi-host caveat undocumented | design §13 | §13 assumption row in finalize PR |
| suggestion | Duplicated deny helpers across unix/windows transports | `internal/control/transport_unix.go:20` | kept |
| suggestion | Handoff Platforms contract omits `windows` label | `specs/unfinalized/supervisor-service/handoff.md:91` | fixed in finalize PR |

Foreign changes in range: b6447b0 feat: protected check evaluator (cep task 7): journal.go, journal_test.go, qualification_test.go; 5071264 OQ-7 go-winio approval: supervisor-service scratchpad.md; c4e64b5 __contain command (cep task 4): cmd/mythhelm/main.go; 6f8bcc4 ledger migration (bjl task 3): journal.go, journal_test.go, ledger_test.go, qualification_test.go.

## Acceptance

| Criterion | Result | Evidence |
|---|---|---|
| AC-5.1 singleton per user/host with instance lock | met | TestStaleLockAdoptedWithGenerationBump, lock tests; #250 (generation recorded, enforcement deferred — issue #282; Windows same-user cross-session refused per stricter logon rule — issue #281) |
| AC-5.2 idempotent intents, conflict on reuse | met | idempotency + conflict met; replay binds capability token after fix #284 (I-2) |
| AC-5.3 private transport, no TCP, capability tokens | met | peer tests unix/windows, no-TCP tests; no issuance over protocol until stream 4 (disclosed) |
| AC-5.4 conflicting-root refusal | met | ErrRootConflict tests; exit code corrected by fix PR (I-8) |
| AC-6.1 sole-writer transactions | partial | tx scope ships, workers never write SQLite; scan weakened, no outbox table, v2 append deferred to stream 3 (all recorded) |
| AC-6.2 host-keyed reservations, bounds prevent work | partial | host-keyed reservations + bounds met; expiry unwired — issue #280 |
| AC-6.3 authority filtering | partial | attempt-token callers bounded; token-less operator self-scoped, disclosed in design §13 |
| NFR-1 frame limits at every control ingress | met | CheckIngress server-side; client-response path skips it (recorded divergence I-6) |
| NFR-3 OS/arch matrix with CGO_ENABLED=0 | met | matrix-pinned tests incl. Windows legs; lazy-start/detach evidence absent (disclosed) |

## Deviations

- Task 1: no liveness probe gates adoption (successful flock proves release) — recorded in the task entry; observable contract unchanged.
- Task 2: plain `protocol_mismatch` errors until vocab task 6 lands; depth fixtures without Files entries; started with vocab tasks open but consumed only merged APIs — recorded; tracked by issue #283 now.
- Task 3: peercred split files, `Conn` gains Receive/Respond, plain `permission_denied` frame — recorded; wrap-up tracked by issue #283.
- Task 4: out-of-Files changes incl. transport refactor and shared framing; same-logon is SID+LUID — recorded; parity decision in issue #281.
- Task 5: migration 0004 (not 0003), no reservations DDL, `control.Error` stand-in, ledger via context, extra tables — recorded; stand-in tracked by issue #283.
- Task 6: additive 0005 reconcile, no expiry reaper, operator self-scope, no idle shutdown, `ErrRootConflict` — recorded; expiry in issue #280, scope in §13.
- Task 7: ADR accepted mid-spec, matrix format defined, testPlatforms review rounds — recorded.
- Review-found: client response path skips CheckIngress (I-6); tx-lock serialization replaces claim-before-execute and `claimed` is dead (I-7); generation informational, not fencing (I-1, design §3 amended).

## CI history

- CI/Go (macos-latest) on #250 (ca468b4, bcf902f): real, TestStaleLockAdoptedWithGenerationBump; green after fix pushes.
- CI/Go (macos-latest) on #253 (2161e36): real, TestDialWithoutSupervisor + TestReceiveEnforcesFrameLimits; green after fix push.
- CI/Go (windows-*, ubuntu-*) on #266 (181bc9d, de72698, 3d4252a): real, Windows DACL probe evolution across 3 review rounds; green at 4f2eac4.
- CI on merge f61fff8 (#266): main-tip windows legs failed on the unrelated time-bomb TestFirstLaunchStartsClockOnce past 12:10Z (branch CI was green); fixed by #276.
- No same-headSha failure+success pair on any branch: no nondeterministic failure to explain.
- run-events fail/retry rows: none.

## Effort

dispatched=7 returned=5 failed=0; unaccounted: task 4 and task 7 dispatch rows (cloud, no return rows; both merges verified); attempts 7 over 7 tasks; first-pass 7/7; review rounds 6 recorded (T4:3, T6:1, T7:2) (+1 finalize-review fix PR); wall-clock 2026-10-08T23:31:47Z → 2026-10-09T12:07:30Z

| Task | Agent | Attempts | Review rounds | PR | Merged | First pass |
|---|---|---|---|---|---|---|
| 1 | - | 1 | 0 | #250 | yes | yes |
| 2 | - | 1 | 0 | #251 | yes | yes |
| 3 | - | 1 | 0 | #253 | yes | yes |
| 4 | - | 1 | 0 | #266 | yes | yes |
| 5 | - | 1 | 0 | #255 | yes | yes |
| 6 | - | 1 | 0 | #258 | yes | yes |
| 7 | - | 1 | 0 | #262 | yes | yes |

## Lessons

- What worked: the support-matrix-with-evidence-test pattern (task 7) pins doc claims to named tests, so drift fails CI instead of review; review rounds on task 4 hardened real Windows behavior across three CI-observed failures (DACL probe, busy retry, negative-test scoping); additive migration reconciles (task 6, 0005) absorbed a mid-stream table conflict without a rebuild.
- What to change: stand-in deviations with no tracked follow-up went live untracked (F1, four tasks); an ADR accepted mid-spec decayed before the last task landed (I-5); replay-path authorization fell through a task seam — token in task 6, digest in task 5 — and no per-task review tested the combination (I-2).

## Proposals

- P-supervisor-service-1 — Re-verify accepted ADRs against the shipped tree at finalize
- P-supervisor-service-2 — Stand-in deviations must name their tracked follow-up
