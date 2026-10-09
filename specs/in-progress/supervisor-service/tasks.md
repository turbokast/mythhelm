## Supervisor Service — Tasks

### Dependencies

- Epic: `specs/*/v2-contracts-supervisor/plan.md` (stream 2 of 4). Prerequisite
  spec: `specs/*/v2-contract-vocabulary/` (stream 1), which provides
  `internal/v2contract`: `Reservation`, `ControlError` + 24-code catalogue,
  `Envelope`, lifecycle checks, `MaxFrameBytes`/`MaxNestingDepth`/
  `MaxArtifactRefs` and `CheckFrameLimits`. No task here starts before stream 1
  ships; every consumed signature above is used verbatim.
- Parallel groups: Task 3 depends on Task 2 (frames the transport carries) and
  follows it; there is no parallel group for them. Task 4 (Windows) is blocked
  until the maintainer approves the go-winio dependency (OQ-7, `scratchpad.md`);
  it may then run in parallel with Tasks 5–6. Task 7 runs last (docs describe
  the finished mechanism).
- Task 6's breadth (reservation handlers, filtering, CLI, lazy spawn) is
  accepted as one task: token mint/check and `status`/`assign` moved to Task 5,
  and the remaining pieces share the reserve/e2e test files with one e2e
  proving the wiring; splitting would leave an unwired middle state.
- **Gates for every task.** `gofmt -w` on changed Go files (then `gofmt -l .`
  clean); `go vet ./...` plus `GOOS=windows go vet ./internal/control/` and
  `GOOS=darwin go vet ./internal/control/`; `go test -race ./...`;
  `go mod tidy -diff` when dependencies change;
  `scripts/ci/check-public-hygiene.sh`. Files existing or an agent reporting
  success is not completion: gate output is cited in the completion entry.
- **Completion convention.** Append ` ✅ COMPLETED` to the task heading and add
  `Status`, `Implementation` (commit SHAs, PR number), `Spec deviations`
  (`None`, or each with its reason) and `Files modified`, keeping every
  original field (`.claude/skills/task-completion/SKILL.md`).
- **Commits.** Signed off (`git commit -s`); the topology ADR ships in Task 7.

---

## Implementation Tasks

### Task 1 — Instance lock and root-conflict refusal ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: complex (process ownership, cross-platform lock behaviour)
- **Depends on**: None
- **Change**: Add the per-user instance lock with lock-file metadata
  (`{root, pid, started_at, generation}`), stale-lock adoption with a boot
  generation bump, and `MYTHHELM_HOME` conflict refusal, so at most one
  supervisor per user per host can hold authority.
- **Files**:
  - `internal/control/lock.go`
  - `internal/control/lock_unix.go`
  - `internal/control/lock_windows.go`
  - `internal/control/lock_test.go`
- **Produces**: `control.LockPath() (string, error)`, `control.AcquireInstance(dir string) (release func(), err error)`, `control.ErrInstanceHeld`, `control.ErrRootConflict`
- **Acceptance**:
  - `TestSecondInstanceHeld`: a second `AcquireInstance` on the same dir returns `ErrInstanceHeld` while the first is held; passes only after the first releases.
  - `TestLockPathIndependentOfRoot`: `LockPath()` is identical for two different roots and lives under neither root; a lock path derived from the state root fails the test.
  - `TestRootConflictRefused`: with a live lock under root A, acquiring under root B returns `ErrRootConflict`, never a second authority; a variant that acquires succeeds-then-fails this test.
  - `TestStaleLockAdoptedWithGenerationBump`: after the holder process is killed, a new acquirer succeeds and records a higher generation; a test double that reuses the old generation fails.
- **Test plan**: temp state dirs; kill a real holder subprocess for the stale test; build-tagged lock files vetted with cross-`GOOS` vet.
- **Invariants touched**: I05 (v2 §2: one writer per user/host via the instance lock); I18 (v2 §2: one process owner, no second election).
- **Status**: ✅ Completed — per-user instance lock with metadata, stale adoption and root-conflict refusal landed; PR #250.
- **Implementation**: flock/LockFileEx exclusion with metadata classification; every successful acquire adopts with a bumped generation. Gates: go-fmt/go-vet/go-mod-tidy/golangci-lint/govulncheck/hygiene PASS via gate.sh; go-test fails only on pre-existing environmental TestStrictMainBlocksWriteNothing (identical on untouched base). Commit cd7191e.
- **Spec deviations**: No separate liveness probe gates adoption (design §3): a successful flock proves the previous holder released, so the acquirer always adopts with a higher generation; a pid-liveness gate false-refused live-but-released recorders under other roots and risked pid-reuse false refusals. Observable contract unchanged.
- **Files modified**: `internal/control/lock.go`, `internal/control/lock_unix.go`, `internal/control/lock_windows.go`, `internal/control/lock_test.go`, `specs/in-progress/supervisor-service/tasks.md`, `specs/in-progress/supervisor-service/handoff.md`.

### Task 2 — Frame codec and NFR-1 ingress enforcement ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: None
- **Change**: Add the length-prefixed JSON frame codec with strict decode and
  `CheckIngress`, enforcing the stream-1 frame limits at every control ingress
  so oversize or malformed mandatory frames stop the affected integration.
- **Files**:
  - `internal/control/frame.go`
  - `internal/control/frame_test.go`
- **Produces**: `control.CheckIngress(frame []byte) error` (violations as `protocol_mismatch` `*v2contract.ControlError`)
- **Acceptance**:
  - `TestFrameAtExactly1MiBPasses`: a frame of exactly `v2contract.MaxFrameBytes` encoded bytes passes; 1 byte more returns `protocol_mismatch`.
  - `TestDepth64Passes65Fails`: nesting depth 64 passes, 65 fails, each pinned by a fixture.
  - `TestRefs128Pass129Fail`: 128 artifact references pass, 129 fail.
  - `TestUnknownKeyRejected`: a frame with an unknown key fails naming the key (strict decode, not silent drop).
- **Test plan**: table-driven fixtures at each boundary; constants referenced from `v2contract`, never copied.
- **Invariants touched**: I09 (v2 §2: absent measurements stay absent, never zero-filled by decode).
- **Status**: ✅ Completed — length-prefixed JSON codec with strict decode and `CheckIngress` NFR-1 enforcement, all four acceptance tests passing; PR #251.
- **Implementation**: `CheckIngress` validates the length prefix, strict-decodes via `v2contract.Decode[Frame]`, then measures depth/refs with a streaming scan and enforces all three dimensions through `v2contract.CheckFrameLimits` verbatim. Depth = max simultaneously open containers (top-level object is 1); refs = string-valued `artifact_id` keys anywhere in the payload. Gates: `go-fmt`, `go-vet` (+ `GOOS=windows`/`darwin` vet on `internal/control`), `go-mod-tidy`, `golangci-lint` PASS; `go test -race ./internal/control/` PASS; full-suite `go-test` fails only on pre-existing `TestStrictMainBlocksWriteNothing` (internal/cli, fails identically on clean origin/main). Commit 5bfd608.
- **Spec deviations**: (1) Violations are plain `error` values carrying the `protocol_mismatch` code in the message (pinned by `requireMismatch` in every rejection test), not `*v2contract.ControlError`: vocab task 6 (error catalogue + `ControlError`) is unmerged on origin/main so the type does not exist; no parallel code was defined (N1). Follow-up once vocab task 6 lands: return `protocol_mismatch` `*v2contract.ControlError` values keeping the same code string. (3) `internal/control/testdata/depth64.json` and `depth65.json`: acceptance-mandated depth fixtures (`TestDepth64Passes65Fails` pins each depth "by a fixture"); fixture data has no separate Files entry. (2) Started before stream 1 fully shipped (vocab tasks 2, 6, 8 open at branch time): this task consumes only merged Task-1 APIs (limits, `CheckFrameLimits`, `Decode`, `RequestEnvelope`) verbatim, so no unmerged input was needed.
- **Files modified**: `internal/control/frame.go`, `internal/control/frame_test.go`, `internal/control/testdata/depth64.json`, `internal/control/testdata/depth65.json`, `specs/in-progress/supervisor-service/tasks.md`, `specs/in-progress/supervisor-service/handoff.md`.

### Task 3 — Unix transport and peer authentication ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: complex (security primitive, cross-platform behaviour)
- **Depends on**: Task 2 (frames the transport carries)
- **Change**: Add the `Transport`/`Listener`/`Conn` interfaces and the Unix
  socket implementation with `SO_PEERCRED`/`LOCAL_PEERCRED` same-UID peer
  authentication and a 0700 socket dir, so only the owning user reaches control.
- **Files**:
  - `internal/control/transport.go`
  - `internal/control/transport_unix.go`
  - `internal/control/transport_unix_test.go`
- **Produces**: `control.Transport`, `control.Listener`, `control.Conn`, `control.Peer`, `control.ErrNoSupervisor`
- **Acceptance**:
  - `TestUnixRoundTrip`: `Listen` + `Dial` + `Request` echoes a response; fails if either end cannot bind or connect.
  - `TestSocketDirIs0700`: the created socket dir has mode 0700; a 0755 dir fails the test.
  - `TestForeignUIDRejected`: a peer with a non-matching UID gets `permission_denied` before any frame is read (fails if the request handler runs).
  - `TestNoTCPListener`: `grep -rn 'Listen("tcp"' internal/control` prints nothing and a test asserts no `tcp` network string exists in the package (fails if a TCP path is added).
- **Test plan**: temp socket dirs; peer-cred tests on linux/darwin; the foreign-UID case uses a stubbed credential source behind an interface.
- **Invariants touched**: I03 (v2 §2: transport grants no authority; peer identity is platform-observed, never payload-claimed); I18 (v2 §2: one input writer per session).
- **Status**: ✅ Completed — `Transport`/`Listener`/`Conn`/`Peer`/`ErrNoSupervisor` and the Unix socket transport with same-UID peer auth, all four acceptance tests passing; PR #253.
- **Implementation**: 0700 socket dir (existing dirs with group/other bits or a foreign owner are refused), 0600 socket; `Accept` yields only same-UID peers and refuses others with a `permission_denied` frame, discarding (never parsing) what they sent; reads bound the prefix by `MaxFrameBytes` before allocating and run `CheckIngress` on received requests. Gates: go-fmt, go-vet (+ darwin/windows vet on `internal/control`), go-mod-tidy, golangci-lint, hygiene, go-test PASS. Commit dfc40ed.
- **Spec deviations**: (1) `internal/control/transport_peercred_linux.go` and `transport_peercred_darwin.go` (outside Files): `SO_PEERCRED` and `LOCAL_PEERCRED` need different x/sys calls, which one file cannot hold. (2) `Conn` gains `Receive` and `Respond` beyond the design's `Request`-only interface: an accepted connection needs a server-side read/reply path. (3) `permission_denied` is a plain `{code,message}` frame, not `*v2contract.ControlError` (vocab task 6 unmerged); wrap it when that lands.
- **Files modified**: `internal/control/transport.go`, `internal/control/transport_unix.go`, `internal/control/transport_unix_test.go`, `internal/control/transport_peercred_linux.go`, `internal/control/transport_peercred_darwin.go`, `specs/in-progress/supervisor-service/tasks.md`, `specs/in-progress/supervisor-service/handoff.md`.

### Task 4 — Windows named-pipe transport ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: complex (cross-platform behaviour, new dependency)
- **Depends on**: Task 3
- **Change**: Add the user-restricted named-pipe `Transport` via go-winio and
  the lazy supervisor process form on Windows, so Windows has the same
  singleton control endpoint as Unix. BLOCKED until the maintainer approves
  the go-winio dependency (OQ-7); the approval is recorded in `scratchpad.md`
  before this task starts.
- **Files**:
  - `internal/control/transport_windows.go`
  - `internal/control/transport_windows_test.go`
  - `go.mod`
  - `go.sum`
- **Acceptance**:
  - `TestPipeRoundTrip` (Windows runner): `Listen` + `Dial` + `Request` succeeds over a user-restricted pipe; fails if the pipe is world-accessible.
  - `TestPipePeerIsSameLogon`: a connection from another logon session is rejected with `permission_denied`.
  - `TestGoWinioPinned`: `go.mod` pins `github.com/Microsoft/go-winio` to the reviewed version in `scratchpad.md`; an unpinned or different version fails.
- **Test plan**: Windows-runner tests; DACL assertion on the pipe; lazy-spawn covered in Task 6.
- **Invariants touched**: I03 (v2 §2: pipe ACL grants no authority beyond the owning user); I18 (v2 §2: same singleton semantics as Unix).
- **Status**: ✅ Completed — Windows named-pipe `Transport` via go-winio v0.6.3 with a user-only protected DACL, per-peer logon-session check and Windows lazy start; PR #266.
- **Implementation**: Peer identity (SID + logon LUID) is read from the peer process token via the pipe's client/server pid, on both ends; the Unix `unixConn` became the shared `streamConn`. go-winio LICENSE at v0.6.3 is MIT (Copyright 2015 Microsoft). Windows-tagged tests compile (`GOOS=windows go vet`, `go test -c`) but were not run locally. Commit 51e9f54.
- **Spec deviations**: Also changed `internal/control/transport.go`, `transport_unix.go` (shared framing moved out), `client.go` (Windows variables in the spawn environment), `default_other.go` and `default_windows.go` (Windows `DefaultTransport` and detached start), `winio_pin_test.go` (`TestGoWinioPinned` untagged so every CI leg runs it), and `internal/admission/qualify_test.go` (renamed a local `comparable` that golangci-lint flags on main). "Same logon" is user SID plus logon session LUID. `govulncheck` could not run in the sandbox (vuln.go.dev 403). Review round 1: the DACL probe uses `SE_KERNEL_OBJECT` (by-name query; `SE_FILE_OBJECT` opened the pipe and failed `ERROR_PIPE_BUSY` on Windows CI) and `TestGoWinioPinned` fails when no scratchpad matches the approval glob (CodeRabbit).
- **Files modified**: `go.mod`, `go.sum`, `internal/control/transport_windows.go`, `internal/control/transport_windows_test.go`, `internal/control/default_windows.go`, `internal/control/default_other.go`, `internal/control/transport.go`, `internal/control/transport_unix.go`, `internal/control/client.go`, `internal/control/winio_pin_test.go`, `internal/admission/qualify_test.go`, `specs/in-progress/supervisor-service/tasks.md`, `specs/in-progress/supervisor-service/handoff.md`.

### Task 5 — Intent server, idempotency ledger and sole-writer transactions ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: complex (persistence schema, concurrency)
- **Depends on**: Task 2 (frame ingress), Task 3 (`Peer` for request binding)
- **Change**: Add migration `0003_supervisor.sql` (`operations`, `reservations`
  tables), the idempotent `Execute` path with `operation_id`/`expected_revision`
  binding, and the one-transaction `Mutate` scope, so repeats replay and
  conflicting reuses fail without executing.
- **Files**:
  - `internal/control/control.go`
  - `internal/control/server.go`
  - `internal/control/ledger.go`
  - `internal/control/execute_test.go`
  - `internal/journal/migrations/0003_supervisor.sql`
  - `internal/journal/journal.go` (bump `SchemaVersion` 2→3 only)
- **Produces**: `control.Intent`, `control.Result`, `control.Handler`, `control.Execute(ctx context.Context, h Handler, peer Peer, intent Intent) (Result, error)`, `control.Mutate(ctx context.Context, db *sql.DB, fn func(tx *sql.Tx) error) error`, `control.Server`, `control.NewServer(handlers map[string]Handler) *Server`, `control.Server.Register(method string, h Handler) error`, `control.Server.Dispatch(ctx context.Context, peer Peer, intent Intent) (Result, error)`, `control.StatusHandler(db *sql.DB) Handler`, `control.AssignHandler(db *sql.DB) Handler`, `control.MintToken(ctx context.Context, db *sql.DB, attemptID string) (token string, err error)`, `control.CheckToken(ctx context.Context, db *sql.DB, token, attemptID string) error`
- **Acceptance**:
  - `TestIdenticalRepeatReplays`: executing the same intent twice runs the handler once and returns byte-identical results; a handler run-count of 2 fails.
  - `TestConcurrentDuplicateExecutesOnce`: concurrent `Execute` calls with the same `operation_id` run the handler exactly once and every caller receives the same stored `Result` (I12; run with `-race`).
  - `TestIntentEmbedsRequestEnvelope`: `control.Intent` embeds `v2contract.RequestEnvelope` (wire `operation_id`/`expected_revision` preserved); an intent whose envelope fails `Validate` returns `invalid_contract` without running the handler.
  - `TestReusedIDWithDifferentArgsConflicts`: same `operation_id` with different params returns `revision_conflict` and the handler does not run.
  - `TestStaleExpectedRevisionConflicts`: an intent with an older `expected_revision` returns `revision_conflict`.
  - `TestMutateIsOneTransaction`: a handler that fails midway leaves no event, projection or outbox row (all-or-nothing observed in the tables).
  - `TestWorkersNeverOpenSQLite`: `go list -deps ./internal/workers` output contains no `modernc.org/sqlite` line; adding the import fails the test.
  - `Test0003IsAdditive`: migration applies on a 0002 database; `0001_init.sql` and `0002_qualification.sql` byte-identical before and after.
  - `TestSchemaVersionIs3`: `SchemaVersion == 3` after 0003 and `OpenReadOnly` succeeds on a 0003 database.
  - `TestRegisterBindsMethod`: a stub registered under a test method dispatches through `Dispatch`; a duplicate registration returns an error naming the method; an unregistered method resolves to `capability_unsupported` without touching the ledger.
  - `TestExecuteRejectsMalformedIntent`: a malformed intent returns `invalid_contract` and the handler does not run.
  - `TestStatusIntentReturnsInstanceState`: a `status` intent via `Execute` returns the running pid, generation, root and endpoint; a repeat returns byte-identical results.
  - `TestAssignIntentRecordsOwnership`: an `assign` intent records run ownership under the supervisor; a conflicting reuse returns `revision_conflict` without executing.
  - `TestTokenMintedAtAdmissionCheckedPerRequest`: `MintToken` at attempt admission stores a digest; `CheckToken` accepts the token for its attempt and returns `permission_denied` for another attempt.
  - `TestMintTokenUnknownAttemptRefused`: `MintToken` for an unknown attempt returns `invalid_contract` and stores nothing.
- **Test plan**: temp SQLite databases via the existing journal test harness; handler run-count doubles; concurrent duplicate execution race test (`-race`).
- **Invariants touched**: I23 (v2 §2: single canonical ledger, one tx per mutation); I12 (v2 §2: no replay of effects — repeats replay stored results, never re-execute); I20 (v2 §2: stored results immutable once recorded).
- **Status**: ✅ Completed — idempotent `Execute`, `Mutate`, `Server`, `StatusHandler`/`AssignHandler` and capability tokens landed over migration `0004_supervisor.sql`; PR #255.
- **Implementation**: Execute runs the handler and records the operation result in one transaction (Mutate joins it from the handler context); recorded results are immutable by trigger. `ControlError` is not on main, so `control.Error` carries the v2 §4.5 code strings for now. Commit 76b8f17.
- **Spec deviations**: (1) Migration is `0004_supervisor.sql` with `SchemaVersion` 4, not 0003/3: budget-ledger-s1 task 3 took 0003 first; tests are named `TestMigration0004IsAdditive` and `TestSchemaVersionIs4`; `journal_test.go`, `qualification_test.go` and `ledger_test.go` (outside Files) had only their version literals updated. (2) No `reservations` DDL: the table exists from `0003_ledger.sql` and lacks the host key and `expired` status, so Task 6 must reconcile it. (3) `control.Error`/`control.Code*` stand in for the unmerged `v2contract.ControlError`. (4) `Execute` has no ledger parameter, so the ledger is carried by `control.WithLedger(ctx, db)`. (5) `TestWorkersNeverOpenSQLite` scans worker sources for journal database API use, because `go list -deps ./internal/workers` already lists `modernc.org/sqlite` via the `journal.Event` import in `spool.go`. (6) No outbox table exists in the design DDL, so `TestMutateIsOneTransaction` observes journal, runs and operations rows. (7) Extra tables `capability_tokens` and `run_assignments`, and `control.EndpointPath()`.
- **Files modified**: `internal/journal/migrations/0004_supervisor.sql`, `internal/journal/journal.go`, `internal/journal/journal_test.go`, `internal/journal/qualification_test.go`, `internal/journal/ledger_test.go`, `internal/control/control.go`, `internal/control/server.go`, `internal/control/ledger.go`, `internal/control/execute_test.go`, `specs/in-progress/supervisor-service/tasks.md`, `specs/in-progress/supervisor-service/handoff.md`.

### Task 6 — Reservations, capability tokens, authority filtering and lazy start ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: complex (concurrency, process spawning)
- **Depends on**: Task 1 (instance lock for lazy spawn), Task 5 (`Execute`/`Mutate` the methods run on)
- **Change**: Add the reservation manager (`reserve`/`release`/`heartbeat`), capability-token enforcement at the method handlers (mint/check ship in Task 5), authority-first read filtering, the `mythhelm supervisor status` command and lazy supervisor spawn on `Dial`, so the service is usable end to end.
- **Files**:
  - `internal/control/reserve.go`
  - `internal/control/filter.go`
  - `internal/control/server.go` (register the Task 6 method handlers)
  - `internal/cli/supervisor.go`
  - `internal/cli/dispatch.go` (register `supervisor` in the `commands` map)
  - `cmd/mythhelm/main.go` (hidden `__supervisor` spawn entry only)
  - `internal/control/reserve_test.go`
  - `internal/control/e2e_test.go`
- **Produces**: `control.Reserve`, `control.Release`, `control.Heartbeat`, `control.Filter`, `control.ReserveHandler(db *sql.DB) Handler`, `control.ReleaseHandler(db *sql.DB) Handler`, `control.HeartbeatHandler(db *sql.DB) Handler`, `control.ReadHandler(db *sql.DB) Handler`, `mythhelm supervisor status` (plain + `--format jsonl`)
- **Acceptance**:
  - `TestBoundsFailurePreventsWork`: reserving past bounds returns `allowance_exhausted` and records no reservation; a force-admitting variant fails.
  - `TestReservationKeyedByHostAndResource`: two reservations with the same bucket but different hosts coexist; same host+bucket+scope collides.
  - `TestTokenScopedToAttempt`: a token minted for attempt A authorises A and returns `permission_denied` for attempt B.
  - `TestFilterHidesForeignRepos`: `Filter` removes out-of-scope rows before counts are computed — count is 0 and no name leaks; a redacted-stub variant fails.
  - `TestQuantityUnknownNeverZero`: a reservation with unknown quantity stores `"unknown"`; a `0` or `""` quantity is rejected as `invalid_contract`.
  - `TestLazySpawnThenStatus`: with no supervisor running, `supervisor status` spawns it (bounded wait) and prints pid, generation, root and endpoint; a second concurrent spawn attempt becomes a client (no `ErrInstanceHeld` escapes to the user).
  - `TestStatusJsonlCarriesInstanceState`: `supervisor status --format jsonl` emits one JSON object carrying `pid`, `generation`, `root` and `endpoint`; a missing key fails the test.
  - `TestReserveIntentViaExecute`: a `reserve` intent through `Dispatch` records a reservation; an identical repeat replays without writing a second row.
  - `TestReleaseIntentViaExecute`: a `release` intent through `Dispatch` flips the reservation to `released`; release evidence is recorded.
  - `TestHeartbeatIntentViaExecute`: a `heartbeat` intent through `Dispatch` refreshes `heartbeat_at` on a held reservation.
  - `TestReadIntentFiltersForeignRepos`: a `read` intent through `Dispatch` returns only in-scope rows — filtered-out repos contribute 0 to counts and no names.
- **Test plan**: temp state roots; packaged-binary e2e for lazy spawn and status output; detach test asserts that the supervisor remains alive and serves a new control request after the client exits.
- **Invariants touched**: I05 (v2 §2: host+resource keying, bounds prevent work); I09 (v2 §2: unknown quantities distinct from zero); I06 (v2 §2: client exit transfers nothing — detach test).
- **Status**: ✅ Completed — reservation manager and handlers, authority-first `Filter`/`read`, the serve loop, lazy detached supervisor start and `mythhelm supervisor status` landed; PR #258.
- **Implementation**: Reservations are one held row per host, bucket and scope (migration `0005_reservation_host.sql`, additive); bounds refuse with `allowance_exhausted` and record nothing. The supervisor starts in its own session and a racing starter loses the instance lock and exits; `Connect` refuses a supervisor serving another state root. Commit 4098c89.
- **Spec deviations**: (1) The `reservations` table from `0003_ledger.sql` is reconciled by an additive `0005_reservation_host.sql` (`SchemaVersion` 5) adding `execution_host` and a held-key unique index; `expired` is not added (a table rebuild would be needed), so `orphaned` remains the reconcile state and no expiry reaper ships; `journal_test.go`, `qualification_test.go` and `execute_test.go` (outside Files) only had version literals and the 0004 test's `reservations` comparison adjusted. (2) Files beyond the list: `control/client.go`, `control/default_unix.go`, `control/default_other.go`, `control/e2e_other_test.go`, and a `TestMain` cleanup in `control/lock_test.go`. (3) Bounds source is unspecified by the design: exhausted bucket (`bucket_state`) or an optional `bound` param; `ReserveOptions` gains `Host` and `Bound`. (4) No per-repository entitlement store is specified, so a token-less same-user peer (the operator) reads the repositories it presents; a request carrying an attempt capability token is bounded to that attempt's run repository, read from the ledger (the list can only narrow it). (5) `control.Error` still stands in for the unmerged `v2contract.ControlError`. (6) Heartbeat does not extend `expires_at`; the supervisor has no idle shutdown; non-Unix platforms report `ExitCapability` until task 4. (7) `internal/control/control.go` gains the `CodeAllowanceExhausted` constant (`allowance_exhausted` is a new code returned by `Reserve`). (8) `internal/journal/journal.go` bumps `SchemaVersion` 4→5 for `0005_reservation_host.sql`. (9) `Connect` verifies the connected supervisor serves this command's state root and fails closed with `ErrRootConflict` otherwise (review round 1).
- **Files modified**: `internal/journal/migrations/0005_reservation_host.sql`, `internal/journal/journal.go`, `internal/journal/journal_test.go`, `internal/journal/qualification_test.go`, `internal/control/control.go`, `internal/control/reserve.go`, `internal/control/filter.go`, `internal/control/server.go`, `internal/control/client.go`, `internal/control/default_unix.go`, `internal/control/default_other.go`, `internal/control/reserve_test.go`, `internal/control/e2e_test.go`, `internal/control/e2e_other_test.go`, `internal/control/execute_test.go`, `internal/control/lock_test.go`, `internal/cli/supervisor.go`, `internal/cli/dispatch.go`, `cmd/mythhelm/main.go`, `specs/in-progress/supervisor-service/tasks.md`, `specs/in-progress/supervisor-service/handoff.md`.

### Task 7 — Topology ADR and support matrix ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 6 (documents the finished mechanism)
- **Change**: Record the per-user-singleton topology decision and publish the
  per-deliverable support matrix with evidence, so the process-ownership change
  is reviewable and every platform claim is checkable. The Go exactness test
  ships in this docs task (accepted: matrix and parser test are one reviewable
  unit, per the `v2contract` SUPPORT precedent). The Windows pipe row reads
  `fixture-tested` if Task 4 landed before this task, `blocked` otherwise —
  Task 7 runs last but Task 4 may still be blocked on the go-winio approval,
  so both orders are valid.
- **Files**:
  - `docs/decisions/NNNN-service-topology.md`
  - `internal/control/SUPPORT.md`
  - `internal/control/support_test.go`
- **Acceptance**:
  - `TestSupportMatrixMatchesEvidence`: the matrix lists exactly the shipped deliverables (a new control method without a row fails) and contains zero `live-qualified` claims.
  - `TestWindowsRowMatchesLanding`: the Windows pipe row reads `fixture-tested` when the pipe transport shipped and `blocked` when Task 4 is still blocked; a row contradicting the shipped code fails.
  - ADR names the per-run → per-user-singleton change, the rejected stay-per-run alternative, and the NFR-3 platform rows with their test names; review sign-off recorded in the completion entry.
- **Test plan**: matrix-parsing test following the `v2contract` SUPPORT precedent; ADR reviewed by the maintainer.
- **Invariants touched**: I14 (v2 §2: every advertised capability has versioned evidence or is explicitly blocked); None beyond that (docs only).
- **Status**: ✅ Completed — ADR 0014, `internal/control/SUPPORT.md` and `support_test.go` landed; PR #262.
- **Implementation**: The matrix is parsed by the test and checked against the methods `NewSupervisorServer` binds, the tests that exist, their build-tag platforms and whether `transport_windows.go` exists; the Windows pipe row reads `blocked` because task 4 has not landed. Commit e1c8680.
- **Spec deviations**: (1) Resolved: ADR 0014 review sign-off recorded 2026-10-09 — maintainer approved; the shepherd's fact-check (decision text vs shipped mechanism, 0004/0005 DDL claims, all 10 cited NFR-3 tests present in `internal/control/`) served as input to that review. ADR status flipped to accepted. (2) The `v2contract` SUPPORT precedent is not on main, so this task's parser defines the matrix format (five columns, backticked IDs and test names). (3) Review rounds 1–2 (CodeRabbit): `testPlatforms` now honours implicit GOOS filename suffixes so an untagged `*_windows_test.go` cannot be mislabelled `all`, and `windows` is a recognised platform so Task 4's Windows tests can serve as row evidence (each proven with a temporary probe test, then removed). (4) `docs/decisions/0014-service-topology.md` (outside Files): the task Change mandates recording the per-user-singleton topology decision, and the ADR is that record.
- **Files modified**: `docs/decisions/0014-service-topology.md`, `internal/control/SUPPORT.md`, `internal/control/support_test.go`, `specs/in-progress/supervisor-service/tasks.md`, `specs/in-progress/supervisor-service/handoff.md`.
