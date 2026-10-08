## Supervisor Service — Scratchpad

Seeded at `/spec`. Every implementing task appends its notes below the line;
nothing above it is edited except OQ verdicts.

## Open questions

- **OQ-3** (Windows supervisor form): default (a) lazy process, decided D5 in
  `design.md`. Decider: designer. Service registration is an explicit
  follow-up, not a silent second topology.
- **OQ-6** (Unix peer-auth mechanism): default same-UID peer-cred check + 0700
  socket dir, decided D3. Decider: designer with security review (review runs
  on the Task 3 PR).
- **OQ-7** (Windows pipe transport: go-winio vs hand-rolled x/sys): default
  go-winio behind the `Transport` interface, D4. Decider: maintainer (new
  dependency). Task 4 MUST NOT start until the maintainer records approval
  here with the reviewed version pin. Approval: _pending_. License check
  (2026-10-08): go-winio is MIT-licensed (Copyright Microsoft); Task 4
  confirms the LICENSE text at the pinned version.
- **OQ-10** (control-protocol package shape): default new `internal/` package,
  decided D1 (`internal/control`). Decider: designer.
- **OQ-11** (idempotency store location): default ledger tables, decided D2
  (`operations` table in migration 0003). Decider: designer.

## Research notes

- Phase-1 investigation (`/tmp/mh21-findings.txt`, `/tmp/mh21-findings2.txt`):
  x/sys v0.48.0 provides `GetsockoptUcred` (Linux) and `GetsockoptXucred`
  (macOS); stdlib `net.Listen("unix", …)` for the listener; x/sys/windows has
  only raw `CreateNamedPipe` with no accept loop (hence the OQ-7 default).
- Stream-1 contract (`specs/*/v2-contract-vocabulary/design.md`): frame limits
  and `CheckFrameLimits` (v2c§7.3), 24-code catalogue + `ControlError`
  (v2c§5), `Reservation` (v2c§3), `Envelope` (v2c§6), lifecycle checks (v2c§4).
  Consumed verbatim; any drift is a spec deviation, not a local fix.
- `ownerlock.go` is the lock-mechanism precedent (flock / LockFileEx,
  `ErrOwnerHeld`); the instance lock follows it rather than inventing a new
  primitive.

---

## Task notes

(none yet)
