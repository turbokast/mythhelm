## Supervisor Service — Design

Normative source: mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md, version 2.0.
Consumes `specs/*/v2-contract-vocabulary/design.md` verbatim; citations below use `v2c§N`
for its sections. Nothing here redefines a record type, error code, envelope or limit.

## 1. Current state

- Per-run owners, not a service: `AcquireOwner` is defined at
  `internal/supervisor/ownerlock.go:23` (flock / LockFileEx, `ErrOwnerHeld`) and
  acquired at 4 non-test sites (`grep -rn 'AcquireOwner' internal cmd
  --include='*.go' | grep -v _test`): `pipeline.go:130`, `recover.go:47`,
  `cli/apply.go:67`, `cli/tui.go:185`. Each `mythhelm run` is its own supervisor.
- No daemon or listener exists: `grep -rn -E 'daemon|UnixListener|net\.Listen'
  internal cmd adapters --include='*.go' | grep -v _test` prints nothing; single
  entrypoint `cmd/mythhelm/main.go`.
- No intent machinery: `grep -rn 'operation_id\|expected_revision' internal cmd
  adapters --include='*.go' | grep -v _test` prints nothing.
- Journal pragmas already match the sole-writer story: `busy_timeout(5000)`,
  `journal_mode(WAL)`, `synchronous(FULL)` (`internal/journal/journal.go:38`).
  `Append` already does idempotent `event_id`, contiguity and generation fencing
  (`journal.go:281-345`).
- Workers already never open SQLite: `grep -rn 'journal\.Open' internal/workers
  | grep -v _test` prints nothing (the only unfiltered hit is
  `worker_test.go:311`). AC-6.1's second clause holds; this spec pins it with
  a test.
- Available without new dependencies on Unix: stdlib `net.Listen("unix", …)` plus
  `golang.org/x/sys v0.48.0` (direct, `go.mod:13`) for `SO_PEERCRED` /
  `LOCAL_PEERCRED`. Windows named-pipe serving needs `go-winio` (OQ-7, D4).
- Stream-1 vocabulary this spec consumes (exact signatures in `v2c§2–§6`):
  `v2contract.SchemaVersion == 2`; `MaxFrameBytes == 1<<20`, `MaxNestingDepth ==
  64`, `MaxArtifactRefs == 128`, `CheckFrameLimits(encodedLen, depth, refs int)
  error` (v2c§7.3); `Code` catalogue of 24 codes with `DefaultDisposition()`
  (v2c§5); `ControlError` with `Validate()` and `MapAdapterFailure` (v2c§5);
  `Envelope` event-v2 shape (v2c§6); `Reservation` record (v2c§3);
  `CheckRunTransition/CheckTaskTransition/CheckAttemptTransition`,
  `CheckTerminalEntry`, `CheckReconcileIdentity` (v2c§4).

## 2. Package and layout (v2 §3.2; OQ-10)

New package `internal/control` (D1): transport interface + Unix/Windows
transports, frame codec, instance lock, intent server, idempotency ledger access,
reservation manager. One package because the control protocol is one contract
surface; stream 3/4 consume it without reaching past it.

```
internal/control/
  control.go            // Intent, Request, Response, Handler; capability tokens
  frame.go              // length-prefixed JSON codec + CheckFrameLimits enforcement
  transport.go          // Transport/Listener/Conn interfaces, peer identity
  transport_unix.go     // Unix socket listener + SO_PEERCRED/LOCAL_PEERCRED auth
  transport_windows.go  // named-pipe listener via go-winio (OQ-7)
  lock.go               // per-user instance lock + MYTHHELM_HOME conflict refusal
  server.go             // accept loop, method registry, lazy spawn client, generation fencing
  ledger.go             // operations + reservations tables access (migration 0003)
  reserve.go            // reservation manager (bounds checks, heartbeats)
  filter.go             // authority-first filtering for reads
```

Migration `internal/journal/migrations/0003_supervisor.sql` (D2): new tables
`operations` (idempotency ledger) and `reservations`; sequences after 0002; no
v1 table touched. Stream 3 adds later migrations and the legacy import onto
these tables. The implementing task bumps `SchemaVersion` 2→3 in
`internal/journal/journal.go` alongside the migration — `OpenReadOnly`
(`internal/journal/projections.go:365`) refuses `user_version >
SchemaVersion`, so 0003 without the bump would break every read-only CLI;
stream 3 sequences its own 3→4 bump after this.

## 3. Singleton and instance lock (v2 §3.1; AC-5.1, AC-5.4)

```go
// ErrInstanceHeld: another supervisor holds the lock for this user+host.
// ErrRootConflict: a supervisor is active under a different MYTHHELM_HOME.
var (
    ErrInstanceHeld = errors.New("control: supervisor instance already running")
    ErrRootConflict = errors.New("control: supervisor active under another root")
)

// LockPath returns the stable per-user, per-host instance-lock path. It is
// derived from the OS user identity and host — never from dir — so it is
// identical for every MYTHHELM_HOME and never lives under the resolved
// state root. A lock under the selectable root would let two roots hold
// two locks and silently run two authorities.
func LockPath() (string, error)

// AcquireInstance takes the per-user instance lock at LockPath() and holds
// it until release is called or the process exits. While holding the lock
// it compares dir (the resolved state root, canonicalised) against the
// root recorded in the lock metadata.
// Failure cases: ErrInstanceHeld (lock held by a live supervisor on this
// root); ErrRootConflict (lock metadata names a different root); I/O
// errors from lock-file creation.
// The lock file records {root, pid, started_at, generation}; a live lock
// on a different root is a conflict, never a second authority (AC-5.4).
func AcquireInstance(dir string) (release func(), err error)
```

Mechanism follows the `ownerlock.go` precedent (flock on Unix, LockFileEx on
Windows). Stale locks (dead pid) are adoptable only after a liveness probe, and
adoption bumps the boot generation, which is recorded, reported by `status`,
and folded into the idempotency digest; enforcement (refusing stale
generations) is deferred — informational until stream 4 (issue #282).
Run ownership becomes assignments under the supervisor: the 4
per-run `AcquireOwner` sites keep working during the migration period; stream 3
drains them (honesty register).

## 4. Transport and peer authentication (v2 §3.1; AC-5.3; OQ-6, OQ-7)

```go
// Peer is the authenticated identity of a control client.
type Peer struct {
    UID      uint32 // Unix peer credential UID; 0 with OSUser on Windows
    OSUser   string // resolved username where available, else ""
    PID      int32  // peer PID where the platform reports one, else -1
    SameUser bool   // peer UID == supervisor UID (Unix) / same logon (Windows)
}

// Transport dials and serves one local control endpoint. No TCP implementation
// exists in this spec; adding one is a design change, not a configuration.
type Transport interface {
    // Listen serves path (socket path / pipe name). Failure cases:
    // address-in-use, permission denied creating a 0700 socket dir.
    Listen(path string) (Listener, error)
    // Dial connects to a supervisor at path. Failure cases: not running
    // (ErrNoSupervisor), permission denied, protocol mismatch.
    Dial(path string) (Conn, error)
}

var ErrNoSupervisor = errors.New("control: no supervisor running")

type Listener interface {
    Accept() (Conn, error)
    Close() error
    Addr() string
}

// Conn is one authenticated control connection. Peer reports the identity
// established at accept/connect time; it is never re-derived from payload.
type Conn interface {
    Peer() Peer
    // Request sends one frame and returns the response frame. Failure cases:
    // frame-limit violation (peer stops the integration, NFR-1),
    // protocol_mismatch ControlError, transport I/O error.
    Request(ctx context.Context, frame []byte) ([]byte, error)
    Close() error
}
```

- Unix (`transport_unix.go`; OQ-6 decided, D3): stdlib `net.Listen("unix", …)`; peer UID
  via `SO_PEERCRED` (Linux) / `LOCAL_PEERCRED` (macOS) from x/sys; socket dir
  mode 0700; peers whose UID differs from the supervisor's are rejected with
  `permission_denied` before any frame is read.
- Windows (`transport_windows.go`; OQ-7, D4): lazy supervisor process holding a
  user-restricted named pipe (OQ-3 default (a), D5), served via go-winio behind
  this `Transport` interface so the dependency is replaceable. The go-winio
  dependency requires maintainer approval before the implementing task starts
  (recorded in `scratchpad.md`); without it the Windows task stays blocked.
- Capability tokens: attempt-scoped opaque tokens minted by the supervisor at
  attempt admission, presented per request, checked against the ledger row for
  the attempt. A token for attempt A never authorises attempt B (AC-5.3).

```go
// MintToken mints an opaque capability token for attemptID at attempt
// admission and stores its digest in the attempt's ledger row (control.go,
// Task 5). Failure cases: invalid_contract (unknown attempt),
// persistence_unavailable (ledger I/O failure).
func MintToken(ctx context.Context, db *sql.DB, attemptID string) (token string, err error)

// CheckToken authorises token for attemptID against the stored digest.
// A token minted for attempt A returns permission_denied for attempt B.
// Failure cases: permission_denied (unknown token, wrong attempt),
// persistence_unavailable (ledger I/O failure).
func CheckToken(ctx context.Context, db *sql.DB, token, attemptID string) error
```

## 5. Frames and NFR-1 enforcement (v2 §4.3; NFR-1)

Wire format (D6): 4-byte big-endian length prefix + one JSON object, decoded
with `v2contract.Decode` semantics (unknown keys rejected). Every ingress
point — server accept, client dial response, spool-adjacent control reads —
calls the stream-1 validator; this spec defines no limit of its own:

```go
// CheckIngress enforces NFR-1 at one frame ingress: encoded length, JSON
// nesting depth and artifact-reference count against v2contract.MaxFrameBytes,
// MaxNestingDepth and MaxArtifactRefs via v2contract.CheckFrameLimits.
// Failure cases: oversize frame, excessive depth, too many references — each
// returned as a protocol_mismatch ControlError, and the affected integration
// is stopped (connection closed, no further frames accepted).
func CheckIngress(frame []byte) error
```

`MaxArtifactRefs` counts `artifact_id`-shaped references in the decoded frame.
Depth is measured during decode with a streaming decoder (depth 64 cap), never
by re-marshalling.

## 6. Idempotent intents (v2 §3.2; AC-5.2; OQ-11)

```go
// Intent is one control mutation. It embeds the stream-1 v2 §4.2 request
// envelope (operation_id, object identity, expected_revision, controller
// generation where applicable); this spec defines none of those fields
// itself.
type Intent struct {
    v2contract.RequestEnvelope
    Method           string          `json:"method"`
    Params           json.RawMessage `json:"params"`
    CapabilityToken  string          `json:"capability_token,omitempty"`
}

// Result is the durable outcome of an executed intent, replayed verbatim.
type Result struct {
    OperationID string          `json:"operation_id"`
    Revision    int64           `json:"revision"`
    Body        json.RawMessage `json:"body"`
    Error       *v2contract.ControlError `json:"error,omitempty"`
}

// Handler executes one intent against the ledger. All failures are
// *v2contract.ControlError with code driving the caller's disposition (v2c§5):
// revision_conflict for ID-reuse-with-different-args and stale
// expected_revision; ownership_unresolved for terminal entry while ownership
// is open (via v2contract.CheckTerminalEntry); permission_denied for token or
// peer failure; persistence_unavailable for ledger I/O failure.
type Handler func(ctx context.Context, peer Peer, intent Intent) (Result, error)

// Execute applies Handler with idempotency (OQ-11 decided, D2): the operations
// ledger row keyed by operation_id is read in the mutation transaction; an
// identical repeat (same method + params digest) returns the stored Result
// without re-executing; a reused ID with different args returns
// revision_conflict without executing. Concurrent duplicates are serialised
// by atomically claiming operation_id (insert-if-absent in the operations
// ledger) before Handler runs: at most one claimant executes, the losers
// return the winner's stored Result, and Handler runs at most once per
// operation_id (I12). Failure cases: as Handler, plus invalid_contract
// for malformed intents (including an envelope that fails
// RequestEnvelope.Validate).
func Execute(ctx context.Context, h Handler, peer Peer, intent Intent) (Result, error)
```

Methods in this spec: `reserve`, `release`, `heartbeat`, `status`, `assign`
(run ownership assignment), `read` (filtered reads). Stop/recover methods
belong to stream 4 and are unknown methods here (`capability_unsupported`).

```go
// Server owns the intent dispatch table (method name → Handler) as
// instance state — never a mutable package-level map. The server looks up
// intent.Method here and runs the hit through Execute; a miss is
// capability_unsupported without touching the ledger.
type Server struct { methods map[string]Handler }

// NewServer builds a Server with the given method bindings.
func NewServer(handlers map[string]Handler) *Server

// Register binds one intent method name to its Handler. A duplicate
// registration returns an error naming the method; it never panics.
func (s *Server) Register(method string, h Handler) error

// Dispatch resolves intent.Method and runs the hit through Execute. A
// miss returns capability_unsupported without touching the ledger.
func (s *Server) Dispatch(ctx context.Context, peer Peer, intent Intent) (Result, error)
```

Handler owners: `status` and `assign` ship in Task 5 (`server.go`,
`ledger.go`); `reserve`, `release`, `heartbeat` and `read` ship in Task 6
(`reserve.go`, `filter.go`), registered on the `Server` via the `server.go`
wiring. Handler constructors take the ledger handle: `func XHandler(db
*sql.DB) Handler`.

## 7. Sole-writer ledger access (v2 §5.1; AC-6.1)

Every mutation runs in one SQLite transaction with scope: validate
preconditions → append event (v2 `Envelope` via stream-1 types; v1 `Append`
path unchanged until stream 3) → update projections → enqueue outbox item. No
network I/O inside the transaction. Busy timeout stays `5000` ms
(`journal.go:38`); NFR-2's deeper SQLite posture belongs to stream 3.

```go
// Mutate runs fn inside the sole-writer transaction. Failure cases:
// persistence_unavailable (busy/locked/I/O), invalid_contract (precondition
// failed), ownership_unresolved (terminal entry refused). fn must not perform
// network I/O; the supervisor process is the only process that calls Mutate.
func Mutate(ctx context.Context, db *sql.DB, fn func(tx *sql.Tx) error) error
```

`TestWorkersNeverOpenSQLite` pins AC-6.1 structurally: it runs `go list -deps
./internal/workers` and fails on any `modernc.org/sqlite` line (the v2c§7.3
NFR-4 test is the precedent).

## 8. Reservations (v2 §3.2, §5.1; AC-6.2)

Stored as `v2contract.Reservation` rows (v2c§3) keyed by `(execution_host,
bucket, scope)`; `Quantity` `"unknown"` is never `0` (I09).

```go
// ReserveOptions carries the idempotency key and bounds for one reservation.
type ReserveOptions struct {
    OperationID string
    Bucket      string
    Scope       string
    Quantity    string // required quantity, or "unknown" (never "" or "0")
    Owner       string // attempt or run ID holding the reservation
}

// Reserve records a reservation in the ledger in the mutation transaction.
// Failure cases: revision_conflict (operation_id reused); allowance_exhausted
// (bounds fail — new work is prevented, never force-admitted);
// invalid_contract (empty bucket/scope/owner, zero quantity for unknown).
// Release and Heartbeat update the row's status/heartbeat_at; expiry turns an
// un-heartbeated reservation into a reconcile input, never a silent reuse.
func Reserve(ctx context.Context, db *sql.DB, opts ReserveOptions) (v2contract.Reservation, error)
func Release(ctx context.Context, db *sql.DB, operationID, reservationID, evidence string) error
func Heartbeat(ctx context.Context, db *sql.DB, reservationID string) error
```

Reservation state words reuse the open `Reservation.Status` string (v2c D6):
`held`, `released`, `expired`. Tightening to an enum is a later revision, not
this spec.

## 9. Authority-first filtering (v2 §5.1; AC-6.3)

```go
// Filter applies repository authority before any content leaves the
// supervisor: rows outside the peer's authorised repositories are removed
// before counts, names or index metadata are computed — a filtered-out
// repository contributes 0 to counts and no name, never a redacted stub.
// Failure cases: permission_denied (peer authorised for nothing in scope).
func Filter(peer Peer, repos []string, rows []Row) ([]Row, error)
```

`Row` is the supervisor-internal read shape (`{RepoID string, Payload
json.RawMessage}`); it is not a contract type and never crosses the wire
unfiltered.

## 10. CLI and lazy start (v2 §3.1)

- The supervisor starts lazily: `Dial` on `ErrNoSupervisor` spawns the
  supervisor process (same binary, hidden `__supervisor` subcommand) and
  retries with a bounded wait. A second concurrent spawner loses the instance
  lock and becomes a client (AC-5.1).
- `mythhelm supervisor status` prints the instance state (running pid,
  generation, root, endpoint) as plain text and `--format jsonl`. No other CLI
  surface in this spec; clients of the control protocol are internal until
  stream 4.

## 11. Tests and CI (NFR-3)

- Unit + golden tests in `internal/control/*_test.go`; table-driven intent
  tests (repeat → same result bytes; reuse-with-different-args →
  `revision_conflict`); frame tests at exactly 1 MiB / 1 MiB+1, depth 64/65,
  128/129 refs.
- Platform tests: `unix` socket + peer-auth tests run on linux/darwin;
  Windows pipe tests run on the Windows runner; filesystem/process/detach
  tests (lock adoption after kill, lazy spawn, and supervisor availability after
  client exit) run per advertised OS/arch with `CGO_ENABLED=0`. The detach test
  confirms that the supervisor remains alive and serves a new control request
  after the client exits.
- `internal/control/SUPPORT.md` publishes one support-matrix row per
  deliverable (`fixture-tested` or `blocked`; no `live-qualified` claims),
  following the v2c§7.2 precedent; a test asserts the matrix lists exactly the
  shipped rows.
- Gates: `gofmt -l .`, `go vet ./...` (plus `GOOS=windows go vet` and
  `GOOS=darwin go vet` for the platform files), `go test -race ./...`,
  `go mod tidy -diff`, `scripts/ci/check-public-hygiene.sh`.

## 12. Decisions

| ID | Decision | Rationale |
|---|---|---|
| D1 | OQ-10: new package `internal/control` for the whole control surface | The protocol is a new contract surface consumed by streams 3–4 (domains.md: a package appears when a task gives it a working responsibility). Extending `supervisor` would mix the legacy per-run path with the service during the migration window. |
| D2 | OQ-11: idempotency + reservations live in ledger tables, migration `0003_supervisor.sql` | Same-transaction with events (I23); the idempotency read must be in the mutation tx or repeats race. Stream 3 owns later migrations and the legacy import; 0003 is greenfield-additive, so no ownership clash. Alternative (separate store) rejected: two writers, two failure modes. |
| D3 | OQ-6: same-UID peer-cred check + 0700 socket dir (default (a)) | x/sys already provides `SO_PEERCRED`/`LOCAL_PEERCRED`; no new dependency, no token bootstrap problem on Unix. Security review at the implementing PR. |
| D4 | OQ-7: go-winio behind the `Transport` interface (default (a)) | De-facto standard, pure Go / CGO-free; x/sys/windows has only raw `CreateNamedPipe` with no accept loop, so hand-rolling duplicates go-winio badly. Requires maintainer approval (new dependency) before the Windows task starts; recorded in `scratchpad.md`. |
| D5 | OQ-3: lazy process on Windows (default (a)) | Same topology as Unix keeps one singleton/adoption design; service registration deferred as an explicit follow-up (honesty register). |
| D6 | Length-prefixed JSON frames, strict decode | Matches spool-line JSON precedent and `v2contract.Decode` strictness; a new binary framing would need its own fuzz story for no benefit at local-IPC scale. |
| D7 | `mythhelm supervisor status` is the only CLI surface | Clients are internal until stream 4; a fuller CLI now would freeze UX before the stop/recover methods exist. |
| D8 | ADR for the service topology in this spec | Process ownership changes (per-run → per-user singleton) require a decision record under `docs/decisions/` per the spec-authoring rule; stream 1 correctly deferred it (v2c D11) to here. |

## 13. Honesty register

| Spec demand | Position |
|---|---|
| AC-5.1 "run ownership as assignments" | Partially met: `assign` method and instance lock ship; the 4 per-run `AcquireOwner` sites remain until stream 3 drains them (no coexisting *service* writers, but the legacy path still runs pre-migration by design). |
| AC-6.1 v2 event append in the mutation tx | Partially met: tx scope and outbox shape ship; the `Append` v2-acceptance change is a stream-3 task (OQ-9 decision, v2c§6). |
| AC-6.3 operator scope | Partially met: attempt-token callers are bounded to their repository; a token-less operator reads whatever repositories it presents (no per-repository entitlement store; disclosed in `SUPPORT.md`). |
| Boot-generation fencing | Informational until stream 4: generation is recorded, reported and digested but never enforced (issue #282). |
| Lock/socket state roots | Assumption: host-local. Two hosts sharing one `MYTHHELM_HOME` run two supervisors on one ledger (SQLite serializes; the logical one-writer breaks). |
| Windows pipe transport (OQ-7) | Shipped: go-winio v0.6.3 approved; `transport_windows.go` fixture-tested (task 4). |
| Windows service registration | Deferred: lazy process only (D5); follow-up after stream 4. |
| Stop/recover control methods | Deferred to stream 4 (`supervised-stop-recover`); unknown methods return `capability_unsupported`. |
| NFR-2 SQLite posture (WAL-reset proof, integrity checks) | Deferred to stream 3, which owns the migration-period ledger hardening. |
| `live-qualified` support-matrix rows | None claimed: nothing here has run against a live multi-client deployment. |

## 14. Cross-Spec References

- Epic plan: `specs/*/v2-contracts-supervisor/plan.md` (this spec is stream 2; scope FR-5–FR-6; NFR-1 implementation, NFR-3; OQ-3, OQ-6, OQ-7, OQ-10, OQ-11).
- Depends on `specs/*/v2-contract-vocabulary/` (stream 1): `v2contract` record types (`Reservation`, `ControlError`, `Envelope`), error catalogue, lifecycle checks, frame-limit constants and `CheckFrameLimits`. No implementation task in this spec starts before stream 1 ships.
- Consumed by `specs/*/supervisor-migration/` (stream 3: service to migrate to, 0003 tables, OQ-9 acceptance) and `specs/*/supervised-stop-recover/` (stream 4: control protocol, ownership records, intent methods).
