## Contained Execution Profiles — Tasks

### Dependencies

- Prerequisites (shipped): `specs/*/dogfood-slice/` (exit-7 refusal this spec
  lifts), `specs/*/qualification-registry/` (profile already in the
  qualification key). Sequencing: `specs/*/claude-strict-subscription/` (MH-12,
  in progress) shares `internal/admission/` and `adapters/claudecode/` files —
  rebase onto its merges; `specs/*/supervised-stop-recover/` (todo) shares
  `internal/workers/worker.go` and `internal/supervisor/pipeline.go` —
  rebase onto its merges, never in parallel on the same file (design §7).
  `supervisor-service`/`supervisor-migration` are only MH-21 plan rows
  (`specs/*/v2-contracts-supervisor/plan.md`) with planned migrations
  0003/0004, and `specs/*/budget-ledger-s1/` pins `0005_ledger.sql` on that
  chain: Task 7 takes the next free migration number at its start (0003 in
  the current tree) and whoever lands second renumbers.
- Order is contract-first: Task 1 ships the `contain` types; Tasks 2–3 build
  the Linux mechanism and proxy against them; Task 4 wires `__contain` into
  the worker; Tasks 5–7 admit, restrict and verify; Tasks 8–10 cover honesty,
  startup and adapter seams; Task 11 freezes evidence with the adversarial
  suite. Runnable sets: {1} first; {2, 3} after 1 (disjoint Files, same
  package — no shared identifiers); {4} after {2, 3}; {5} after 4; {6, 10}
  after {4, 5} (disjoint Files); {7} after {4, 6} (`pipeline.go` shared with
  6, never in parallel); {8} after {5, 7} (parallel with 9, 10 once 7
  lands); {9} after {7, 10}; {11} after {8, 9}. Shared files serialize:
  `pipeline.go` (6→7→9), `pipeline_test.go` (5→6), `admission.go` (5→8→9),
  `boundary.go` (5→6→9), `run.go` (5→8), `contain/records.go` (5→11).
- **Gates for every task.** `gofmt -w` on touched Go files first, then from
  the tree root `gofmt -l .` (must print nothing), `go vet ./...`,
  `GOOS=windows go vet ./...` and `GOOS=darwin go vet ./...` when OS-specific
  files change, `go test -race ./...`, `go mod tidy -diff` (must print
  nothing), and `scripts/ci/check-public-hygiene.sh`. A task is not complete
  because files exist or an agent reported success; cite the runner output.
- **Completion convention.** Append ` ✅ COMPLETED` to the heading, keep every
  original field, and add `Status` (`✅ Completed — …; PR #<n>`),
  `Implementation` (at most 3 lines, plus the commit SHA), `Spec deviations`
  (`None`, or each deviation with its reason) and `Files modified`. Every
  task appends a scratchpad note under Discoveries.
- **Commits.** Every commit is signed off (`git commit -s`, DCO). The process
  ownership change (Task 4) carries its decision record in the same PR.

---

## Implementation Tasks

### Task 1 — Containment contract package ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: None
- **Change**: Create `internal/contain` with the `Claim`/`Coverage`/
  `Evidence`/`Policy`/`Availability` types, the `Registry` interface, the
  `Missing` aggregator and the `ErrUnsupported`/`ErrMissingCoverage` values,
  so every later task builds on one exact contract (design §4).
- **Files**:
  - `internal/contain/contain.go`
  - `internal/contain/contain_test.go`
- **Produces**: `contain.Dimension` (`DimFilesystem`, `DimProcess`,
  `DimNetwork`, `DimCredential`); `contain.Claim{Name, Version, Enforced,
  Detail}`; `contain.Coverage{Filesystem, Process, Network, Credential
  Claim}`; `func (c Coverage) Missing(required []Dimension) []Dimension`;
  `contain.Evidence{Profile, OS, Route, Boundary, Version, Owner, Coverage}`;
  `contain.Registry` with `Lookup(profile, os, route string) (Evidence,
  bool)`; `contain.AuthBind{Source, Target}`; `contain.Policy{Profile,
  Workdir, ReadOnly, AuthBinds, ProxyAddr}`; `contain.Availability{Supported,
  Version, Reason}`; `contain.ErrUnsupported`;
  `contain.ErrMissingCoverage`.
- **Acceptance**:
  - `TestCoverageMissingAggregates`: a coverage with network unenforced and
    credential unknown reports `Missing(all four) == [network, credential]`;
    an all-enforced coverage reports empty; dropping a dimension from the
    result fails the test.
  - `TestRegistryUnknownRefuses`: a registry with no record for
    `restricted/darwin/fake` returns `ok == false`; a test double that
    returns a supported record fails the lookup assertion.
- **Test plan**: Table-driven unit tests over hand-built `Coverage` values;
  an in-memory map-backed `Registry` double (no SQLite — the worker path
  must never need it).
- **Invariants touched**: I09 (v2 §2: unknown combinations stay `ok == false`,
  never a zero-value supported record); I14 (v2 §2: every evidence record
  carries version and owner for its nested claims, each with name and
  version).
- **Status**: ✅ Completed — `internal/contain` ships the contract types, `Missing` aggregator and sentinel errors; PR #242.
- **Implementation**: `Missing` iterates the required list in order and treats an unknown dimension as missing (fail closed). Tests include a control lookup and a kept mutation-proven failing case. Commit 785b530.
- **Spec deviations**: None.
- **Files modified**: `internal/contain/contain.go`, `internal/contain/contain_test.go`, `specs/in-progress/contained-execution-profiles/tasks.md`, `specs/in-progress/contained-execution-profiles/handoff.md`.

### Task 2 — Linux boundary mechanism ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: complex (syscalls, namespaces, cross-platform probe)
- **Depends on**: Task 1
- **Change**: Implement the Linux provider: `ProbeLinux` availability check
  (trial unshare plus the mount/remount/tmpfs setup) and `EnterLinux`
  (user+mount namespaces, recursive read-only `/` remount, read-write or
  read-only workdir bind, tmpfs `/tmp` and scratch `$HOME` with auth binds,
  capability drop, `NO_NEW_PRIVS`), so contained launches get a real native/outer
  boundary (design §2.2, D2). `enter_linux.go` is Linux-only by filename
  suffix (the repo's `proc_linux.go` convention); `enter_other.go` carries
  the `//go:build !linux` stubs (the `orphan_other.go` convention) so
  untagged callers compile on all three OSes.
- **Files**:
  - `internal/contain/enter_linux.go`
  - `internal/contain/enter_other.go`
  - `internal/contain/linux_test.go`
  - `internal/contain/policy.go`
  - `internal/contain/policy_test.go`
- **Produces**: `func contain.ProbeLinux() Availability`;
  `func contain.EnterLinux(spec ContainSpec) error`;
  `contain.ContainSpec{Path, Args, Dir, Env, Policy}`;
  `func contain.PolicyFor(profile, workdir string, readonly bool,
  binds []AuthBind, proxy string) (Policy, error)`.
  On non-Linux, `ProbeLinux` reports `{Supported: false, Reason: <missing
  capability>}` and `EnterLinux` returns `ErrUnsupported`.
- **Acceptance**:
  - `TestProbeLinuxNamesReason`: on Linux `ProbeLinux` reports
    `Supported == true` with a non-empty `Version` when user namespaces are
    available, else `Supported == false` with a non-empty `Reason` naming
    the missing capability; a probe that returns supported with an empty
    version fails.
  - `TestProbeLinuxRejectsUnavailableSetup`: when user namespaces are
    available but a required mount, remount, or tmpfs operation is denied,
    `ProbeLinux` reports `Supported == false` with a non-empty `Reason`;
    reporting support fails the test.
  - `TestPolicyRejectsWholeHomeBind`: `PolicyFor` with an `AuthBind` whose
    source is `$HOME` itself (or `/`) returns an error; a single-file bind
    is accepted, and removing the check makes the test fail.
  - `TestEnterLinuxConfinesFilesystem` (linux-only, skips elsewhere): a
    `ContainSpec` running `/bin/sh -c 'touch $OUTSIDE/pwned; touch
    $WORKDIR/ok'` (`$OUTSIDE` a temp scratch dir outside the workdir) exits
    with `$OUTSIDE/pwned` absent and `$WORKDIR/ok` present; a write through
    an inherited writable child mount (tmpfs) fails too; the
    same spec with `ReadOnly` set leaves `$WORKDIR/ok` absent too; running
    the spec uncontained creates `$OUTSIDE/pwned`, proving the fixture bites.
  - `TestEnterLinuxDropsCapabilities` (linux-only, skips elsewhere): a
    `ContainSpec` with a read-only workdir reports empty `CapEff` and
    `CapPrm` in `/proc/self/status`, and remounting the workdir read-write
    fails; the same spec run uncontained reports non-empty capabilities,
    proving the fixture bites.
  - `TestEnterLinuxBindsAuthFile` (linux-only, skips elsewhere; AC-7.1
    positive clause): a `ContainSpec` whose policy carries one `AuthBind`
    from a temp secret file to `$HOME/token` runs `cat $HOME/token` and
    prints the secret bytes; removing the bind makes the read fail, and the
    ambient temp path itself is unreachable inside.
- **Test plan**: Probe tests run everywhere (asserting shape, not support);
  `EnterLinux` tests are linux-gated with `t.Skip` elsewhere and use temp
  dirs; no network, no root.
- **Invariants touched**: I05 (v2 §2: only this declared mechanism
  substantiates boundary claims); I14 (v2 §2: probe reports version or a
  precise reason, never a guess).
- **Status**: ✅ Completed — `internal/contain` ships `ProbeLinux`, `EnterLinux`, `ContainSpec` and `PolicyFor` with the non-Linux stubs; PR #247.
- **Implementation**: The root goes read-only with `mount_setattr(AT_RECURSIVE)` and the workdir and auth files are re-attached from detached `open_tree` clones, so paths under `/tmp` survive the tmpfs. `NO_NEW_PRIVS` is load-bearing: without it root regains capabilities at exec. Commit c68af2e.
- **Spec deviations**: Beyond the task's design `Produces`, `contain.ProbeEnv` and `contain.RunProbeChild` are exported: the probe re-executes the current binary, whose entry point must call `RunProbeChild` when `ProbeEnv` is set. The mount-step interface, `probeResult` and `probeChild` live in `policy.go` so the denial tests compile on every OS. The wiring is for Task 4; its acceptance does not name it yet.
- **Files modified**: `internal/contain/enter_linux.go`, `internal/contain/enter_other.go`, `internal/contain/linux_test.go`, `internal/contain/policy.go`, `internal/contain/policy_test.go`, `specs/in-progress/contained-execution-profiles/tasks.md`, `specs/in-progress/contained-execution-profiles/handoff.md`, `specs/in-progress/contained-execution-profiles/scratchpad.md`.

### Task 3 — Filtering egress proxy ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: complex (security primitive: network enforcement point)
- **Depends on**: Task 1
- **Change**: Add the worker-owned localhost CONNECT-only proxy with an
  exact host:port allowlist plus the `ProxyEnv` pin map, so contained
  launches get proxy-routed egress whose denials are testable (design §2.3).
- **Files**:
  - `internal/contain/proxy.go`
  - `internal/contain/proxy_test.go`
- **Produces**: `func contain.ServeProxy(ctx context.Context, allow []string)
  (addr string, stop func(), err error)`; `func contain.ProxyEnv(addr string)
  map[string]string`
  (keys `HTTPS_PROXY`, `HTTP_PROXY`, `https_proxy`, `http_proxy`).
- **Acceptance**:
  - `TestProxyAllowsListedHost`: a CONNECT to an allowlisted
    `httptest`-server host:port forwards bytes both ways; removing the entry
    makes the same CONNECT fail.
  - `TestProxyDeniesUnlistedHost`: a CONNECT to an unlisted host gets a 403
    and no bytes leave the proxy; pointing the client at the target directly
    succeeds, proving the denial comes from the proxy.
  - `TestProxyRejectsPlainHTTP`: a plain `GET` (non-CONNECT) gets a 405; a
    proxy that forwards it fails the test.
- **Test plan**: `httptest` servers as allowed/denied targets on 127.0.0.1
  with ephemeral ports; raw TCP client for CONNECT framing; no external
  network.
- **Invariants touched**: I09 (v2 §2: denials are explicit statuses, and the
  proxy never claims to block direct egress — see the honesty register).
- **Status**: ✅ Completed — `internal/contain` ships the worker-owned localhost CONNECT-only proxy with exact host:port allowlist denials and the `ProxyEnv` pin map; PR #248.
- **Implementation**: Exactly one CONNECT per connection is tunnelled, only after an exact allowlist hit; every denial (403/405/431/400) returns before any dial. `stop` is `sync.Once`-safe and ctx cancellation closes the listener. Commit 8532e12.
- **Spec deviations**: None.
- **Files modified**: `internal/contain/proxy.go`, `internal/contain/proxy_test.go`, `specs/in-progress/contained-execution-profiles/tasks.md`, `specs/in-progress/contained-execution-profiles/handoff.md`.

### Task 4 — `__contain` command and worker wiring ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: complex (process ownership: new spawn path + re-exec)
- **Depends on**: Task 2, Task 3
- **Change**: Add the hidden `__contain` command (reads `ContainSpec` on
  stdin, dups the fd-3 prompt pipe onto stdin, calls `EnterLinux`, never
  returns on success) and wrap `launcher.Launch` so a `Launch` carrying
  containment spawns `__contain` with user+mount-namespace clone flags and
  `ProcSpec.Stdin` forwarded byte-for-byte on that pipe (design §2.2).
  Shared spec-build and prompt-pipe setup live in untagged `contain.go`;
  the clone-flags wiring lives in build-tagged helpers (`contain_linux.go`
  real, `contain_other.go` `//go:build !linux` refusing) so all three OSes
  build. The worker also starts the Task 3 proxy before a contained spawn,
  fills `Policy.ProxyAddr` with the ephemeral address, and amends the
  child env with the `ProxyEnv` pins post-admission (design §2.3); carry
  the decision record for the new spawn path in the same PR.
- **Files**:
  - `internal/contain/main.go`
  - `internal/contain/main_test.go`
  - `cmd/mythhelm/main.go`
  - `internal/workers/worker.go`
  - `internal/workers/contain.go`
  - `internal/workers/contain_linux.go`
  - `internal/workers/contain_other.go`
  - `internal/workers/contain_test.go`
- **Produces**: `workers.Launch` gains `Containment *contain.Policy`
  (`json:"containment,omitempty"`); `contain.Main(args []string) int` (`0`
  unreachable post-exec, `2` invalid spec, `1` setup failure).
- **Acceptance**:
  - `TestLaunchWithoutContainmentUnchanged`: a `Launch` with nil
    `Containment` spawns the native directly (existing behavior byte for
    byte: argv, env, dir); setting the field routes through `__contain`.
  - `TestContainRejectsOversizeSpec`: stdin over 1 MiB exits 2 without
    execing; the test binary asserts no child marker file appears.
  - `TestContainedLaunchEndToEnd` (linux-only): a worker launch with a
    restricted policy runs `sh -c` fixture that writes inside the workdir
    (succeeds) and outside it (fails); the same fixture with nil
    containment succeeds at both, proving the boundary did the work.
  - `TestContainedStdinForwardedByteForByte` (linux-only): a worker launch
    with a restricted policy and a prompt file holding shell metachars,
    newlines and NUL bytes runs `cat`; the fixture echoes stdin
    byte-identical to the prompt file; an EOF or short read fails.
  - `TestContainedProxyDenyThroughPinnedEnv` (linux-only): a worker launch
    with a restricted policy whose proxy allowlist names only an
    `httptest` host:port runs a fixture that CONNECTs to an evil host:port
    through the pinned `HTTPS_PROXY` env; the CONNECT is denied (403)
    and `Policy.ProxyAddr` is non-empty in the launched spec. A worker
    that never starts the proxy (empty addr, unpinned env) lets the evil
    CONNECT succeed, failing the test.
- **Test plan**: Unit tests for spec decoding and the launcher branch;
  linux-gated worker-level test with a temp workdir and `sh` fixtures; the
  decision record covers the spawn-path ownership argument.
- **Invariants touched**: I18 (v2 §2: one process owner — `__contain` execs
  exactly once per worker, and the launcher still refuses a second launch);
  I06 (v2 §2: stop ladder and group confirmation unchanged — the native keeps
  its process group inside the namespaces).
- **Status**: ✅ Completed — `__contain` is wired into the worker: a `Launch` with `Containment` spawns it with the proxy started, `ProxyAddr` filled, the proxy env pinned and the prompt forwarded on fd 3; PR #257.
- **Implementation**: `launcher.Launch` builds its command through `Launch.command`, which keeps the uncontained path byte for byte and otherwise builds `__contain` (cmd.Args, spec on stdin, prompt pipe as `ExtraFiles[0]`); the proxy, prompt copier and pipe ends are released when the native is reaped. Decision record: `docs/decisions/0013-contained-spawn-path.md`. Commit 6f0d8cf.
- **Spec deviations**: (1) `Launch` also gains `ProxyAllow []string` (the exact host:port list for the proxy): the task names `Containment` only, but `Policy` has no field for the allowlist and the proxy test needs one; Task 5 or 9 fills it from the qualification record. (2) `contain.NamespaceAttr` is exported (it was unexported `nsSysProcAttr` from Task 2) so `workers/contain_linux.go` reuses the clone flags, and `promptToStdin` is added to `internal/contain/enter_linux.go` and `enter_other.go` because the untagged `main.go` cannot call `unix.Dup2`. (3) `TestMain` in `internal/contain/linux_test.go` and `internal/workers/worker_test.go` dispatch `__contain`; the former no longer special-cases `ProbeEnv`, which `contain.Main` now handles. (4) A setup failure inside `__contain` surfaces as a native exit code 1 with stderr, not `launch_failed` (design §2.2); see the ADR consequences. (5) `docs/decisions/0013-contained-spawn-path.md` is outside the task's `Files` list: the task body requires its decision record in the same PR ("carry the decision record for the new spawn path in the same PR", Commits convention in the header).
- **Files modified**: `cmd/mythhelm/main.go`, `docs/decisions/0013-contained-spawn-path.md`, `internal/contain/main.go`, `internal/contain/main_test.go`, `internal/contain/enter_linux.go`, `internal/contain/enter_other.go`, `internal/contain/linux_test.go`, `internal/workers/worker.go`, `internal/workers/worker_test.go`, `internal/workers/contain.go`, `internal/workers/contain_linux.go`, `internal/workers/contain_other.go`, `internal/workers/contain_test.go`, `specs/in-progress/contained-execution-profiles/tasks.md`, `specs/in-progress/contained-execution-profiles/handoff.md`, `specs/in-progress/contained-execution-profiles/scratchpad.md`.

### Task 5 — Admission consult, restricted default, precise refusal ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: complex (security decision point + 6 files)
- **Depends on**: Task 4
- **Change**: Consult boundary evidence at admission (aggregating every
  missing dimension into the exit-7 refusal), flip the empty-flag default to
  `restricted`, set `Profile.Contained`, seed the v1 evidence records, and
  update CLI help plus the stale consent/refusal pins.
- **Files**:
  - `internal/admission/boundary.go`
  - `internal/admission/boundary_test.go`
  - `internal/contain/records.go`
  - `internal/admission/admission.go`
  - `internal/cli/run.go`
  - `internal/cli/run_test.go`
  - `internal/supervisor/pipeline_test.go`
- **Produces**: `func admission.BoundaryConsult(profile, os, route string)
  (contain.Evidence, error)` (`*BlockedError` capability exit-7 naming each
  missing dimension, `contain.ErrMissingCoverage` for unknown combos);
  `contain.SeedV1() map[string]contain.Evidence` (key `profile/os/route`).
- **Acceptance**:
  - `TestRestrictedDefaultNoFlag`: `Decide` with empty `ExecutionProfile`
    admits `restricted` with `Contained == true` and asks no consent question
    (nil `Confirm`); before the change this input blocked with
    `consent_required`.
  - `TestTrustedHostStillNeedsConsent`: empty `Confirm` plus explicit
    `trusted-host` admits with `Consent == "--execution-profile"`; an empty
    flag with a `Confirm` that returns false now admits `restricted` instead
    of blocking (the updated `TestMissingProfileConsentNonInteractiveExit3`
    pin asserts the new default).
  - `TestRefusalNamesEachMissingCoverage`: `BoundaryConsult("restricted",
    "windows", "builtin/fake")` returns exit-7 naming filesystem, process,
    network and credential; removing one name from the message fails.
  - `TestRunHelpListsThreeProfiles`: `run --help` output names
    `trusted-host`, `restricted` and `inspect`; the old single-profile text
    fails the test.
- **Test plan**: `Decide`-level tests with the fake adapter and temp state
  dirs; consult unit tests over seeded + unknown keys; help text asserted
  through `cli.Main` (stdout, stderr, exit code) in `run_test.go`.
- **Invariants touched**: I02 (v2 §7.1: unknown mandatory boundary evidence
  blocks without dropping to host authority); I04 (v2 §2: widening to
  `trusted-host` still needs explicit flag or consent); I14 (v2 §2: every
  record versioned with an owner).
- **Status**: ✅ Completed — admission consults boundary evidence (every missing dimension named in an exit-7 refusal), the empty flag defaults to `restricted` with `Contained == true` and no consent question, and the v1 evidence records are seeded; PR #260.
- **Implementation**: `consentProfile` resolves the profile and calls `BoundaryConsult(profile, runtime.GOOS, "builtin/"+adapter)`; darwin and windows are seeded as unqualified so a refusal can name all four dimensions, and the Claude Code route on Linux records an unenforced credential claim. Commit 96f3ead.
- **Spec deviations**: (1) `inspect` stays refused with exit 7 (`checkCapabilityFlags`): its read-only enforcement is Task 6, and admitting it earlier would claim enforcement nothing provides. (2) `TestRestrictedProfileExit7` becomes `TestInspectProfileExit7`: `restricted` is no longer a refusal on Linux. (3) The Claude Code route is seeded with an unenforced credential claim, so `restricted` is refused for it, naming `credential`, until native auth is bound into the boundary. (4) An admitted `restricted` run is not yet launched inside the boundary: `launchForAttempt` (Task 9) builds `Launch.Containment`; Task 5 only decides admission.
- **Files modified**: `internal/admission/boundary.go`, `internal/admission/boundary_test.go`, `internal/admission/admission.go`, `internal/contain/records.go`, `internal/cli/run.go`, `internal/cli/run_test.go`, `internal/supervisor/pipeline_test.go`, `specs/in-progress/contained-execution-profiles/tasks.md`, `specs/in-progress/contained-execution-profiles/handoff.md`.

### Task 6 — Inspect read-only enforcement ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 2, Task 5
- **Change**: Enforce `inspect` through the shared provider with a read-only
  workdir policy, require enforced coverage (never prompt/label), and refuse
  all checks under `inspect` until MH-22 ships check scoping.
- **Files**:
  - `internal/contain/policy.go`
  - `internal/admission/boundary.go`
  - `internal/supervisor/pipeline.go`
  - `internal/supervisor/pipeline_test.go`
- **Acceptance**:
  - `TestInspectRequiresEnforcedCoverage`: `BoundaryConsult("inspect",
    "linux", ...)` succeeds only when the filesystem claim is enforced; a
    record with `Enforced == false` refuses with exit 7 (no prompt-only
    path exists to accept).
  - `TestInspectPolicyIsReadOnly`: the admitted `inspect` policy has
    `ReadOnly == true` and is otherwise identical to the `restricted` policy
    for the same inputs (shared mechanism, Q2); a divergent mechanism fails.
  - `TestChecksRefusedUnderInspect`: a run admitted as `inspect` with checks
    configured ends `verification_unavailable` with reason
    `checks_refused_under_inspect` and never execs a check (assert via a
    check argv that would create a marker file).
- **Test plan**: Consult/policy unit tests; pipeline-level test with the fake
  adapter asserting the refusal reason and the absent marker.
- **Invariants touched**: I02 (v2 §7.1: read-only without an enforced
  mechanism blocks); I07 (v2 §11.2: no checks run, so none can be faked —
  the candidate stays unverified).
- **Status**: ✅ Completed — `inspect` is admitted through the shared boundary consult with a read-only policy, and its checks are refused (the candidate stays unverified); PR #261.
- **Implementation**: `ConsultRegistry` (the registry-parametrised `BoundaryConsult`) accepts a record only when every contained dimension is enforced; `contain.PolicyForProfile` makes `inspect` the `restricted` policy with `ReadOnly`, and `PolicyFor` refuses a writable `inspect`. The pipeline's no-checks branch also covers `inspect`, recording `NOT RUN` with reason `checks_refused_under_inspect`. Commit 8ca184b.
- **Spec deviations**: (1) `internal/admission/admission.go` is outside the task's `Files`: it holds the `inspect` refusal in `checkCapabilityFlags` and the profile resolution this task extends. (2) The refusal is recorded as `verification.completed` `NOT RUN` with that reason and the run ends `ready_for_review`/`unverified` (exit 5, category `verification_unavailable`), mirroring `--no-checks`, rather than a new run state. (3) `TestInspectProfileExit7` (from Task 5) becomes `TestInspectProfileAdmission`: `inspect` is admitted on Linux. (4) All three acceptance tests live in `pipeline_test.go`, as the `Files` list has no admission or contain test file. (5) An admitted `inspect` run is not yet launched inside the boundary either: `launchForAttempt` (Task 9) builds `Launch.Containment` for all contained profiles from the admission decision; this task decides admission, policy and check-refusal only, mirroring Task 5's deviation (4) for `restricted`.
- **Files modified**: `internal/contain/policy.go`, `internal/admission/boundary.go`, `internal/admission/admission.go`, `internal/supervisor/pipeline.go`, `internal/supervisor/pipeline_test.go`, `specs/in-progress/contained-execution-profiles/tasks.md`, `specs/in-progress/contained-execution-profiles/handoff.md`.

### Task 7 — Protected check evaluator

- **Domain/agent**: go-implementer
- **Budget**: complex (persistence schema + check execution)
- **Depends on**: Task 4, Task 6
- **Change**: Run `restricted` checks under containment with a check policy
  (read-only worktree, scratch tmp, no credential binds), keep host checks
  for `trusted-host`, record the canonical evaluator digest on the
  verification row (design §2.7: stable semantic fields only),
  and preserve unverified candidates explicitly when checks are unavailable.
  Re-check the next free migration number at task start (0003 in the current
  tree: only 0001/0002 exist at `SchemaVersion = 2`) and renumber if MH-21
  or MH-16 landed first; filenames sort positionally, so a stale number
  silently becomes the wrong version.
- **Files**:
  - `internal/integration/verify.go`
  - `internal/integration/verify_test.go`
  - `internal/supervisor/pipeline.go`
  - `internal/journal/journal.go`
  - `internal/journal/migrations/0003_evaluator.sql`
  - `internal/journal/projections.go`
- **Produces**: `contain.Evaluator{Name, Digest}`;
  `func contain.EvaluatorDigest(boundary, version string, policy Policy,
  checkDigests []string) Evaluator`;
  `func integration.RunChecksWithPolicy(ctx, cand, cfg, env, opts
  RunOptions) (Verification, error)`;
  `integration.RunOptions{KeepGoing bool, Policy *Policy}` (nil = host);
  `journal.VerificationRow` gains `EvaluatorName`, `EvaluatorDigest`.
- **Acceptance**:
  - `TestRestrictedChecksRunContained` (linux-only): a check that writes
    outside the worktree fails under the check policy and passes under the
    host policy; the candidate is `failed`, never accepted.
  - `TestCheckArgvStillAdmittedOnly`: a candidate commit that adds a new
    check script does not change the executed argv list (definitions come
    from the admitted digest); executing the candidate's script fails.
  - `TestEvaluatorDigestRecorded`: a verification row carries the digest of
    the exact policy + check definitions that ran; re-running with a changed
    check definition yields a different digest and the old row does not
    apply.
  - `TestEvaluatorDigestRepeatable`: two policies differing only in
    `Workdir`, `ProxyAddr` and bind sources yield identical digests;
    flipping `ReadOnly` or one check digest changes it.
  - `TestUnavailableChecksStayUnverified`: a missing check executable yields
    `verification_unavailable` (exit 5), the candidate preserved, and no
    acceptance reported.
- **Test plan**: `RunChecksWithPolicy` unit tests with fixture check scripts
  in temp worktrees; journal migration test asserting the new columns and
  rollback-safe defaults; pipeline test for the unavailable path.
- **Invariants touched**: I07 (v2 §11.2: the candidate cannot weaken tests,
  hide exit status or mark itself accepted); I20 (v2 §2: changed check
  definitions change the digest, invalidating reuse).

### Task 8 — Honesty surfaces and receipt boundary evidence

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 5, Task 7
- **Change**: Record boundary name/version/coverage and the evaluator digest
  in the receipt, harden the trusted-host disclosure wording on every
  surface, and pin that nothing but the provider substantiates a boundary
  claim.
- **Files**:
  - `internal/supervisor/receipt.go`
  - `internal/supervisor/receipt_test.go`
  - `internal/admission/admission.go`
  - `internal/cli/run.go`
  - `internal/cli/disclosure_test.go`
- **Acceptance**:
  - `TestReceiptCarriesBoundary`: a `restricted` run's receipt
    `execution_bundle.boundary` equals `{name, version, coverage{...}}` from
    the admitted evidence; an unknown dimension renders `unknown`, never
    `contained` or `verified`.
  - `TestReceiptCarriesEvaluator`: the receipt `verification.evaluator`
    equals the row's `{name, digest}` from Task 7; a missing row renders
    `unknown`.
  - `TestTrustedHostDisclosureAnchored`: the consent question, `run --help`
    profile text and receipt label each contain the verbatim sentence `runs
    with your host authority and is not adversarially contained`; restoring
    any weaker wording fails.
  - `TestNoMasquerade`: no `Claim{Enforced: true}` is constructible outside
    the provider's probe-gated path (assert by compiling the allowlist: the
    only `Enforced: true` literals live in `internal/contain` provider
    files); the owner lock and UI toggles appear in no coverage.
- **Test plan**: Receipt tests over a journaled fake-adapter run; disclosure
  assertions anchored to the exact exported strings and help output, not
  file-wide matches.
- **Invariants touched**: I09 (v2 §2: unknown stays `unknown` in every
  label); I05 (v2 §2: coordination mechanisms are not containment evidence).

### Task 9 — Startup boundary and authority fixtures

- **Domain/agent**: go-implementer
- **Budget**: complex (security primitives across the launch path)
- **Depends on**: Task 7, Task 10
- **Change**: Close the admission→exec TOCTOU window with a worker pre-exec
  re-hash of mutable native config, build `Launch` through a pure helper so
  model output provably never reaches it, report boundary restrictions as
  fidelity deltas, and pin the four AC-4.2 channels with failing fixtures.
  Nine files (justified): the delta half needs `boundary.go` (delta
  constructor from admitted evidence) plus the one-line append-site call in
  `admission.go` (`d.Proposal.Overrides`, cf. `:353`), without which
  `TestBoundaryRestrictionsAreDeltas` cannot pass.
- **Files**:
  - `internal/workers/worker.go`
  - `internal/workers/contain.go`
  - `internal/admission/native.go`
  - `internal/admission/boundary.go`
  - `internal/admission/admission.go`
  - `internal/admission/launch_fixture_test.go`
  - `internal/security/authority_test.go`
  - `internal/supervisor/pipeline.go`
  - `internal/supervisor/pipeline_launch_test.go`
- **Produces**: `workers.Launch` gains `UserConfigPaths map[string]string`
  (inventory source → absolute path) and `UserConfigDigests
  map[string]string`; `func launchForAttempt(d admission.Decision, token
  string) workers.Launch` (supervisor, pure — every output derives from `d`
  or `token`).
- **Acceptance**:
  - `TestWorkerRehashesMutableConfig`: a launch whose inventoried settings
    file changes between admission and exec ends `launch_failed` with no
    native process created; an unchanged file launches.
  - `TestTaskBytesNeverReachArgv`: a task file containing `"; rm -rf /"`,
    newlines and `--allowedTools`-shaped lines yields a `ProcSpec.Args` with
    no element containing any task line; the prompt still arrives on stdin
    byte for byte.
  - `TestCredentialEnvDenied`: env vars on the credential denylist
    (`ANTHROPIC_API_KEY`, `CLAUDE_CODE_OAUTH_TOKEN`) never reach the child
    env without the explicit opt-in; adding one to the child fails.
  - `TestModelOutputNeverReachesLaunch`: `launchForAttempt` output is
    byte-identical with and without hostile journaled `native_result` rows
    present (model output is not an input); and a `reflect` allowlist over
    `Decision`'s struct fields enumerates every field and requires it in an
    allowlist literal, so any new field fails the pin until reviewed.
  - `TestBoundaryRestrictionsAreDeltas`: a `restricted` proposal carries one
    `ConfigDelta` per boundary restriction (read-only mounts, proxy pin),
    rendered in receipt `fidelity_differences`; a restriction with no delta
    fails.
- **Test plan**: Worker re-hash test mutates a temp settings file between
  launch build and exec; argv/env tests run `Decide` with hostile fixtures;
  the helper pin runs `launchForAttempt` over fixed decisions.
- **Invariants touched**: I03 (v2 §2: task text, env, plugin messages and
  model output grant no authority); I20 (v2 §2: changed config invalidates
  trust before the next launch).

### Task 10 — Adapter startup-inventory seam

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 4
- **Change**: Expose the Claude Code admitted-config path mapping the worker
  re-hash needs, and pin that plugins, MCP servers and surplus session routes
  never widen the native argv or environment.
- **Files**:
  - `adapters/claudecode/settings.go`
  - `adapters/claudecode/settings_test.go`
  - `adapters/claudecode/launch_test.go`
- **Produces**: `func claudecode.AdmittedConfigPaths(home, workdir string)
  map[string]string` (inventory source name → absolute path inventoried).
- **Acceptance**:
  - `TestAdmittedPathsCoverManifest`: for a fixture home+workdir, every key
    of the inventoried manifest `Digests` has an entry in
    `AdmittedConfigPaths` pointing at the file whose bytes hash to the
    digest; an uncovered source fails.
  - `TestPluginsNeverWidenArgv`: `Prepare` output for a manifest with three
    enabled plugins and two MCP servers is byte-identical (argv and env) to
    the same input with none; any plugin-derived argv element or env entry
    fails.
  - `TestSurplusRoutesStayUnknown`: a session reporting more plugins/MCP
    servers than the manifest yields `plugin:unknown`/`mcp:unknown` routes
    with `Funding == "unknown"`; any other funding value fails.
- **Test plan**: Synthetic settings blobs and home fixtures (no live native,
  no credentials); `Prepare` called with fixed probe inputs.
- **Invariants touched**: I03 (v2 §2: plugin and MCP content cannot widen
  launch authority); I20 (v2 §2: every trusted source has a path the re-hash
  can re-verify).

### Task 11 — Adversarial suite and frozen v1 evidence

- **Domain/agent**: go-implementer
- **Budget**: complex (cross-platform behavior + adversarial coverage)
- **Depends on**: Task 8, Task 9
- **Change**: Freeze the v1 boundary evidence records (version, owner,
  method — where method is the `Boundary` name plus the four claim
  `Detail`s, design §2.10) and ship the adversarial suite: escape fixtures
  that fail contained and succeed on trusted-host (Linux), plus
  precise-refusal tests on every OS.
- **Files**:
  - `tests/e2e/contain_linux_test.go`
  - `tests/e2e/contain_refusal_test.go`
  - `internal/contain/records.go`
- **Acceptance**:
  - `TestRestrictedEscapeFixturesFail` (linux-only): filesystem write
    outside the workdir, write through an inherited writable child mount
    (e.g. `/dev/shm`), `sh -c` subprocess escape, ambient credential-file
    read, and evil-host fetch through the proxy env each FAIL inside a
    `restricted` run; the same five each SUCCEED on `trusted-host`
    (counterfactual in the same test binary — deleting the contained run
    makes the test fail).
  - `TestInspectWriteFails` (linux-only): a workdir write fails under
    `inspect` and succeeds under `restricted`.
  - `TestUnsupportedRefusalPrecise` (all OSes): `restricted`/`inspect` where
    the provider is unavailable exit 7 with every missing dimension named;
    a refusal that names none fails.
  - `TestEvidenceRecordsVersioned`: every `SeedV1` record carries a
    non-empty version, owner and method (method = `Boundary` name plus the
    four claim `Detail`s — no `Evidence` field change needed), and every
    profile × OS × route key the CLI accepts has either a record or a
    refusal test above; an unrecorded, untested combination fails.
- **Test plan**: `tests/e2e` builds `cmd/mythhelm`; Linux fixtures use `sh`
  and the fake adapter; refusal tests run on all three CI OSes; no live
  credentials anywhere.
- **Invariants touched**: I14 (v2 §2: every advertised claim has versioned
  evidence or an explicit refusal); G07 (v2 §18.3: adversarial tests
  substantiate each claimed enforced boundary).
