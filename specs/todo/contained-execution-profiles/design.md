## Contained Execution Profiles — Design

> How `restricted` and `inspect` get enforced, adversarially tested boundaries
> on Linux with per-platform native providers that refuse where unavailable
> (Q1), a shared mechanism with a read-only policy for `inspect` (Q2), and
> evaluator isolation only, leaving revision binding to MH-22 (Q3).
> Normative source: mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md v2.0.

## 1. Current state

- Admission knows three profiles as constants
  (`internal/admission/admission.go:37-39`, grep `ProfileTrustedHost`) but
  refuses `restricted`/`inspect` with exit 7 `execution_profile_unavailable`
  (`admission.go:429-431`, grep `execution_profile_unavailable`), pinned by
  `internal/supervisor/pipeline_test.go:404-411` (grep same). Consent
  (`admission.go:436-450`, grep `consentProfile`): an empty flag asks to run
  `trusted-host`; under `--non-interactive` it blocks with `consent_required`
  (pinned at `pipeline_test.go:420-425`).
- `Profile.Contained` (`admission.go:121-126`, grep `Contained  bool`) is never
  set true: `grep -rn "\.Contained = \|Contained:" internal/ adapters/ | grep -v
  _test` returns zero hits, so no assignment constructs a contained profile.
- The native is spawned exactly once per worker by `launcher.Launch`
  (`internal/workers/worker.go:944-977`, grep `func (l \*launcher) Launch`):
  `exec.Command` direct, no shell, admitted env, `Setpgid` on Unix. Adapters
  never create processes (`internal/adapter/adapter.go:147-151`, grep `type
  Launcher interface`); the only non-test `adapter.Launcher` implementation is
  `workers.launcher` (`grep -rn "Launch(_ context.Context, spec adapter.ProcSpec)"
  internal/ adapters/ | grep -v _test` has the single hit above; unfiltered
  the grep also returns test doubles). The worker never opens SQLite
  (ADR-0004; structurally: no sqlite imports under `internal/workers/`), so
  every containment
  input must arrive in `workers.Launch` (`worker.go:68-82`), the ≤1 MiB stdin
  JSON the supervisor builds at `internal/supervisor/pipeline.go:297-309`
  (grep `workers.Spawn`).
- Checks run on the host: `integration.RunChecks` (`internal/integration/
  verify.go:47-51`, grep `func RunChecks`) builds a detached worktree of the
  frozen candidate and execs each admitted check there (`verify.go:108-114`);
  it never reads the candidate's config. Verification runs after the worker is
  reaped (`pipeline.go:344-348` reap wait precedes the verify stage at
  `:646-730`), so the live threat at check time is candidate content, not a
  running worker.
- Trust is digest-bound per run: `admitNativeConfig`
  (`internal/admission/native.go:33-56`, grep `func (d \*Decision)
  admitNativeConfig`) inventories settings/MCP/plugins into
  `claudecode.Manifest` (`adapters/claudecode/settings.go:27-34`, grep `type
  Manifest struct`) and `CheckNativeTrust` (`internal/admission/billing.go:112`,
  grep `func CheckNativeTrust`) grants only the exact digest. No re-check runs
  between admission and exec.
- Disclosure exists but is thin: `trustedHostDisclosure`
  (`admission.go:48-50`, grep `trustedHostDisclosure`) and the receipt's
  `trusted-host (not contained)` label (`internal/supervisor/receipt.go:97-100`,
  grep `not contained`). TUI sources contain no containment claims (`grep -rni
  "contain\|sandbox\|isolat" internal/tui/ --include=*.go` hits only
  `strings.Contains` call sites and one diff comment).
- Qualification already keys on trust profile: `qualify.Key.TrustProfile`
  (`internal/qualify/qualify.go:67`) is set from `d.Profile.Name` at
  `internal/admission/qualify.go:170-185` (grep `TrustProfile:     profile`).
- `golang.org/x/sys v0.48.0` is available (`go.mod:13`); `__worker` dispatches
  in `cmd/mythhelm/main.go:18` (grep `workers.Main`).

## 2. Design by area

### 2.1 Containment contract — `internal/contain` (v2 §8.1, I14)

New package owning every boundary claim. Core types (exact; see §4):

- `Claim{Name, Version, Enforced, Detail}` per dimension
  (`DimFilesystem`, `DimProcess`, `DimNetwork`, `DimCredential`).
- `Coverage` (four claims) with `Missing() []Dimension`: the dimensions a
  profile requires but the OS/route cannot enforce.
- `Evidence{Profile, OS, Route, Boundary, Version, Owner, Coverage}`: one
  versioned record per profile × OS × route (NFR-1). Unknown combinations
  refuse; nothing defaults to supported (I02, I09).
- `Provider` interface: `Probe() Availability` (runtime capability check with
  version or a precise reason) and policy construction. One provider per OS;
  v1 qualifies Linux only (§2.2); darwin/windows providers probe
  `Supported:false` with the missing coverage named (Q1, AC-1.2).
- `Policy{Profile, Workdir, ReadOnly, AuthBinds, ProxyAddr, ...}`: everything
  the supervisor resolves and the worker/`__contain` needs, SQLite-free.
  `ProxyAddr` is empty at admission; the worker fills it with the ephemeral
  proxy address it chose and amends the child env post-admission (§2.3).

### 2.2 Linux boundary `mythhelm-restricted/1` (v2 §8.1, I05, G07)

Per-platform native, no container dependency (Q1, D2). The worker spawns
`mythhelm __contain` (new hidden command, dispatched beside `__worker`) with
`Cloneflags: CLONE_NEWUSER|CLONE_NEWNS`, a single-UID/GID map, and the
`ContainSpec` on stdin (same ≤1 MiB `DisallowUnknownFields` envelope as
`Launch`). `__contain` then, as root in its own namespaces:

- Filesystem: recursively (`MS_REC`) bind-remount `/` read-only inside
  the mount namespace (host unaffected), so inherited writable child mounts
  (e.g. `/dev/shm`) are read-only too; then bind `Workdir` read-write
  (`restricted`) or read-only remount (`inspect`, Q2); fresh tmpfs `/tmp`;
  scratch tmpfs `$HOME` carrying only the admitted auth binds (§2.6). After
  setup and before exec, drop all capabilities (empty effective and
  permitted sets) so the native cannot remount; `NO_NEW_PRIVS` via prctl
  additionally blocks setuid privilege gain.
- Process: existing `Setpgid` ownership plus the user namespace (contained
  processes hold no capabilities outside it, so no cross-UID signalling or
  host-wide capability actions) and the stop ladder unchanged (I06, I18).
  Signalling same-UID host siblings is NOT blocked in v1 — there is no PID
  namespace; it is the honesty-register residual (§6).
- Credential: the existing env allowlist/denylist (`internal/security/env.go`)
  plus the hidden HOME: ambient credential files are unreachable; only
  `AuthBinds` exist.
- Network: proxy-routed (D3, §2.3), recorded as
  `network: proxy-routed (direct egress not blocked)` — an enforced proxy
  pin plus a disclosed residual, never a silent claim (I09).

Failure anywhere before exec → `launch_failed` (existing worker path); a
`Probe()` failure at admission → exit 7 naming each missing dimension
(AC-1.2).

### 2.3 Egress proxy (v2 §8.1 network scope, v2 §7.4)

The worker starts a localhost CONNECT-only proxy (`contain.ServeProxy`) whose
allowlist is exactly the admitted provider endpoint host:port (first-party
subscription for the first route), then amends the admitted env post-admission
with `HTTPS_PROXY`/`HTTP_PROXY` (and lowercase) pins to the ephemeral address
it chose, and fills `Policy.ProxyAddr` with the same address. The pins are the
process's initial environment, not irrevocable: the native can unset them from
inside, and direct egress around the proxy is NOT blocked in v1 — it is the
honesty-register residual. This spec provides no way for a user to require a
no-egress guarantee (no `--require-*` flag, §6), so v2 §7.4's "if required by
the user" conditional has no trigger here and promises no refusal to such
callers; the refusal this spec ships is AC-1.2's missing-coverage refusal.
The enforced part is the proxy's allowlist denials for traffic sent through
it: non-allowlisted hosts get `407/403`, non-CONNECT methods get `405`.

### 2.4 Admission: consult, default, refusal (v2 §7.1, I02, I04)

- `admission.BoundaryConsult(profile, os, route)` looks up the `Evidence`
  registry: supported → attach `Coverage` to the decision; missing/unknown →
  `BlockedError{Code: "execution_profile_unavailable", Capability: true}`
  whose message names EVERY missing dimension (AC-1.2, Q4 aggregate).
- Default flips to `restricted` (AC-1.3): an empty `--execution-profile`
  admits `restricted` with no consent question (the safe default needs no
  authority to choose). Selecting `trusted-host` is a posture change (I04)
  and still requires the explicit flag or the interactive consent flow.
  Existing explicit invocations are unaffected. The `pipeline_test.go:420-425`
  `consent_required` pin is updated to the new default in the same task.
- `Profile.Contained=true` for admitted contained profiles; the receipt
  records boundary name and version (AC-1.1); `--execution-profile` help
  lists all three profiles.
- Qualification consult is unchanged (profile already in the key); boundary
  evidence is separate from billing/entitlement evidence (v2 §7.1: passes in
  one column never imply another).

### 2.5 Inspect (v2 §8.1, Q2)

Shared mechanism, read-only policy: `Policy{ReadOnly: true}` remounts the
workdir bind read-only; tool effects outside the proxy allowlist fail the
same way as `restricted`. AC-2.2 is structural: `inspect` requires an
enforced provider, else exit 7 — there is no prompt/label path in the code.
All checks are refused under `inspect` (AC-2.1): the pipeline short-circuits
to `verification_unavailable` with reason `checks_refused_under_inspect`
until MH-22 ships check scoping.

### 2.6 Native auth inside the boundary (v2 §8.1, I19, AC-7.1)

`AuthBind{Source, Target}` bind-mounts exactly the admitted auth-bearing
path (e.g. the inventoried `~/.claude.json` for the OAuth route; the
`CLAUDE_CODE_OAUTH_TOKEN` opt-in rides the env as today) into the scratch
HOME. Whole stores are never copied or bound: the policy builder takes an
explicit file list, and a test asserts `$HOME` itself is never in it. Routes
whose auth cannot coexist with the boundary (keychain-backed, helper
executables outside the view) advertise their supported trusted profile
only — admission refuses `restricted`/`inspect` for them with the blocker
named.

### 2.7 Protected evaluator (v2 §11.2, I07, Q3)

Evaluator isolation only; revision binding stays MH-22. Checks for
`restricted` runs execute under the same provider with a check policy:
read-only candidate worktree, scratch `/tmp`, no credential binds, proxy env
retained. `trusted-host` keeps host checks (disclosed host authority).
`RunChecksWithPolicy` is a new entry point; the existing `RunChecks` wraps it
with the host policy so no caller breaks. Each verification records
`Evaluator{Name, Digest}` = hash of (boundary name/version, check-policy,
admitted check-definition digests); persisted on the verification row and
rendered in the receipt for MH-22 to bind. Unavailable checks preserve the
candidate unverified (`verification_unavailable`, exit 5); acceptance is
never reported (AC-5.2).

### 2.8 Honesty surfaces (v2 §8.1, I09, I05, AT-47)

- Receipt `execution_bundle` gains
  `boundary: {name, version, coverage: {filesystem, process, network,
  credential}}`; unknown stays `unknown` (I09). Trusted-host wording becomes
  `runs with your host authority and is not adversarially contained` in the
  consent text, CLI help and receipt label, pinned by an anchored test over
  those exact strings (no file-wide grep).
- Fidelity deltas (AC-4.3): admission appends one `ConfigDelta` per boundary
  restriction (read-only mounts, proxy pin, blocked auth route) to the
  proposal `Overrides`, which the receipt already renders as
  `fidelity_differences`.
- No masquerade (AC-6.1): a test pins that `ownerlock`, reservations (none
  exist) and UI toggles contribute no `Claim{Enforced:true}`: the only
  constructor of enforced claims is the provider's `Probe`-gated path.

### 2.9 Startup boundary (v2 §8.2, I03, I20, AT-11, AT-12)

- AC-4.1: per-run fresh inventory already re-resolves trust each `Decide`;
  this spec closes the admission→exec TOCTOU window: `Launch` carries
  `UserConfigPaths map[source]path` plus expected digests, and the worker
  re-hashes those mutable files before exec — mismatch → `launch_failed`.
  Committed project blobs need no re-hash (immutable). Path mapping per
  adapter comes from `adapters/claudecode` (§4, T10).
- AC-4.2, one failing fixture per channel: (1) task-file bytes with shell
  metachars/flags → never in `ProcSpec.Args` (stdin only); (2) credential
  env vars → denied by `BuildEnv`; (3) plugin/MCP surplus → unknown-funding
  routes that widen neither argv nor env; (4) model output
  (observation/result frames) → never reaches a launch: `Launch` is built by
  a new pure `launchForAttempt(Decision, token)` helper, and a test pins
  that its outputs derive only from `Decision` fields. The helper leaves
  `ProxyAddr` empty; the worker fills it with its ephemeral choice at
  runtime (§2.3). Channels outside the
  four are named residual risk (§6).
- Trust still binds the manifest digest; a changed config misses the grant
  lookup and blocks before any launch (I20), unchanged behavior, newly
  pinned end to end.

### 2.10 Adversarial suite (G07, AT-47, NFR-1)

`tests/e2e/contain_linux_test.go` (linux-tagged) runs fixture attacks against
a real contained launch and asserts each FAILS: filesystem write outside the
workdir, write through an inherited writable child mount, `inspect` workdir
write, subprocess (`sh -c`) escape, credential file read, evil-host fetch
through the proxy env. Every fixture also runs
against `trusted-host` and asserts the attack SUCCEEDS — the failing
counterfactual proving the test bites. Refusal tests (all OSes) assert exit 7
with each missing coverage named. v1 evidence records (boundary version,
owner, and method — where method is the `Boundary` name plus the four claim
`Detail`s, not a separate field) freeze in the same task.

## 3. Packages and files

New: `internal/contain/` (`contain.go`, `policy.go`, `records.go`,
`enter_linux.go` + `enter_other.go` (build-tagged pair, §4 Task 2),
`proxy.go`, `main.go` for `__contain`, per-file tests);
`internal/admission/boundary.go`; `internal/security/authority_test.go`;
`tests/e2e/contain_*_test.go`. Edited: `cmd/mythhelm/main.go` (dispatch),
`internal/workers/` (Launch field, launcher wrap via build-tagged
`contain_linux.go`/`contain_other.go` helpers, proxy startup + env amendment,
pre-exec re-hash),
`internal/admission/admission.go` + `native.go` (consult, default, digests,
fidelity deltas),
`internal/cli/run.go` (help, disclosure), `internal/supervisor/pipeline.go`
(launch helper, inspect refusal, check policy),
`internal/supervisor/receipt.go` + tests, `internal/integration/verify.go`
(new entry point), `internal/journal/` (verification-row evaluator columns),
`adapters/claudecode/` (path mapping, fixtures).

## 4. Interfaces

```go
// internal/contain — produced by Task 1 unless noted.
type Dimension string
const (
    DimFilesystem Dimension = "filesystem"
    DimProcess    Dimension = "process"
    DimNetwork    Dimension = "network"
    DimCredential Dimension = "credential"
)
type Claim struct {
    Name     string `json:"name"`
    Version  string `json:"version"`
    Enforced bool   `json:"enforced"`
    Detail   string `json:"detail"`
}
type Coverage struct {
    Filesystem Claim `json:"filesystem"`
    Process    Claim `json:"process"`
    Network    Claim `json:"network"`
    Credential Claim `json:"credential"`
}
func (c Coverage) Missing(required []Dimension) []Dimension
type Evidence struct {
    Profile  string   `json:"profile"`
    OS       string   `json:"os"`
    Route    string   `json:"route"`
    Boundary string   `json:"boundary"`
    Version  string   `json:"version"`
    Owner    string   `json:"owner"`
    Coverage Coverage `json:"coverage"`
}
type Registry interface {
    Lookup(profile, os, route string) (Evidence, bool)
}
type AuthBind struct {
    Source string `json:"source"`
    Target string `json:"target"`
}
type Policy struct {
    Profile   string     `json:"profile"`
    Workdir   string     `json:"workdir"`
    ReadOnly  bool       `json:"readonly"`
    AuthBinds []AuthBind `json:"auth_binds"`
    ProxyAddr string     `json:"proxy_addr"`
}
type Availability struct {
    Supported bool   `json:"supported"`
    Version   string `json:"version"`
    Reason    string `json:"reason"`
}
var ErrUnsupported = errors.New("boundary unsupported")
var ErrMissingCoverage = errors.New("boundary coverage missing")

// Task 2. enter_linux.go is Linux-only (filename suffix, the repo's
// proc_linux.go convention); enter_other.go (`//go:build !linux`, the
// orphan_other.go convention) carries the portable stubs so untagged callers
// compile on all three OSes: ProbeLinux returning
// {Supported:false, Reason:<missing capability>} and EnterLinux returning
// ErrUnsupported.
func ProbeLinux() Availability
func EnterLinux(spec ContainSpec) error // mounts, then exec; never returns on success
type ContainSpec struct {
    Path      string     `json:"path"`
    Args      []string   `json:"args"`
    Dir       string     `json:"dir"`
    Env       []string   `json:"env"`
    Policy    Policy     `json:"policy"`
}
func PolicyFor(profile, workdir string, readonly bool, binds []AuthBind, proxy string) (Policy, error)

// Task 3.
func ServeProxy(ctx context.Context, allow []string) (addr string, stop func(), err error)
func ProxyEnv(addr string) map[string]string

// Task 4.
func Main(args []string) int // contain.Main: 0 unreachable post-exec, 2 invalid spec, 1 setup failure

// Task 5.
func BoundaryConsult(profile, os, route string) (Evidence, error)
func SeedV1() map[string]Evidence // key profile/os/route

// Task 7.
type Evaluator struct {
    Name   string `json:"name"`
    Digest string `json:"digest"`
}
func EvaluatorDigest(boundary string, version string, policy Policy, checkDigests []string) Evaluator
func RunChecksWithPolicy(ctx context.Context, cand Candidate, cfg admission.ProjectConfig, env []string, opts RunOptions) (Verification, error)
type RunOptions struct {
    KeepGoing bool
    Policy    *Policy // nil = host policy (trusted-host)
}

// Task 9 (core) + Task 10 (adapters).
func launchForAttempt(d admission.Decision, token string) workers.Launch // supervisor, pure
func AdmittedConfigPaths(home, workdir string) map[string]string         // claudecode: inventory source -> absolute path
```

MH-22 consumes: receipt `execution_bundle.boundary` and
`verification.evaluator{name, digest}`, `journal.VerificationRow`
`EvaluatorName`/`EvaluatorDigest` columns, and `CheckResult.Status` values
(`specs/*/protected-acceptance/`, N3: reads evidence, never reimplements).

## 5. Decisions

| ID | Decision | Rationale |
|---|---|---|
| D1 | Boundary attaches in the worker via a `__contain` re-exec, not in-worker syscalls or wrapper scripts | Go cannot run mount setup between fork and exec; a re-exec keeps one spawn site (`launcher.Launch`) and one spec envelope, matching the `__worker` pattern (ADR-0004) |
| D2 | Linux = user+mount namespaces, recursive-RO `/` remount, RW binds, cap-drop + `NO_NEW_PRIVS`; no bubblewrap/container dependency | Q1 per-platform native; the RO-remount design needs no Landlock dependency and probes with a trial unshare plus the mount/remount/tmpfs setup |
| D3 | Network v1 = filtering CONNECT proxy + pinned env; direct egress disclosed, not blocked | Destination-aware blocking needs netns plumbing or eBPF, both out of reach unprivileged; v2 §7.4 explicitly allows refuse-where-unavailable, and a pinned proxy is still an enforced, testable pin |
| D4 | macOS/Windows v1 = precise refusal, not best-effort containment | No readily usable native boundary (`sandbox-exec` deprecated surface, Job Objects are limits not isolation); NFR-1 blesses refuse-with-blocker over fake enforcement |
| D5 | `inspect` = same provider, `ReadOnly` policy | Q2 shared mechanism; one evidence set, one adversarial suite |
| D6 | Evaluator isolation here; revision binding in MH-22 | Q3; this spec records the evaluator digest MH-22 binds |
| D7 | Empty flag defaults to `restricted` with no consent question; `trusted-host` keeps explicit-flag-or-consent | Safe defaults need no authority; posture *widening* does (I04) |
| D8 | Admission→exec TOCTOU closed by worker pre-exec re-hash of mutable config | Cheapest enforcement at the exact gap; committed blobs are immutable and need none |
| D9 | Evidence registry lives in code with versioned records + owner | NFR-1 needs versioned evidence; code records are reviewable and need no migration for v1 |
| D10 | Auth enters the boundary as single-file binds, never whole stores | AC-7.1/I19: bind-mount is not a copy, and the minimal list is testable |

## 6. Honesty register

| Spec demand | Position |
|---|---|
| v2 §8.1 network coverage (full destination enforcement) | Partial: proxy pin enforced + tested; direct egress around the proxy is a disclosed residual recorded in receipt coverage, not a silent gap |
| v2 §8.1 process coverage (sibling isolation) | Partial: process-group ownership + stop ladder enforced; signalling same-UID host siblings is unblocked (no PID namespace in v1), a disclosed residual, not a silent gap |
| v2 §8.1 `restricted`/`inspect` on macOS/Windows | Refused in v1 with named missing coverage (NFR-1); follow-up owns Seatbelt/AppContainer qualification |
| v2 §11.2 revision-bound acceptance | Evaluator isolation + digest only (Q3); binding is MH-22 |
| AC-4.2 channels beyond the four enumerated | Named residual: helper executables on `PATH` inside the boundary view and terminal escape sequences in task text (latter already sanitized per v2 §8.3 in trusted views only) |
| Fake adapter startup inventory | No startup surface exists to inventory; documented, not faked |
| v2 §7.4 user-required no-egress guarantee | Refusal path exists (unsupported coverage refuses); no `--require-*` flag in this spec — a user who needs the guarantee and an enforced run has no single command yet |

## 7. Conflicts and ordering

- `specs/*/claude-strict-subscription/` (in progress, MH-12) edits
  `internal/admission/` and `adapters/claudecode/`: disjoint functions, shared
  files — rebase each task onto its merges, never in parallel on the same file.
- `specs/*/supervised-stop-recover/` (todo) edits `internal/workers/worker.go`
  (Launch identity/nonce fields, envelope) and
  `internal/supervisor/pipeline.go` (spawn site, watch tick): serialize with
  this spec's Tasks 4/9 (`worker.go`, `contain.go`) and Tasks 6/7/9
  (`pipeline.go`) — rebase onto its merges, never in parallel on the same
  file; regions are disjoint (identity/envelope vs containment/launch
  helper).
- `supervisor-service`/`supervisor-migration` are only rows 2–3 of
  `specs/*/v2-contracts-supervisor/plan.md` (no specs yet); their MH-21
  streams plan journal migrations 0003/0004, and
  `specs/*/budget-ledger-s1/` pins `0005_ledger.sql` on that chain. This
  spec's verification-row migration takes the next free number at Task 7
  start (0003 in the current tree, which holds only 0001/0002 at
  `SchemaVersion = 2`); whoever lands second renumbers.
- Downstream `specs/*/protected-acceptance/` (MH-22) reads §4's receipt and
  journal fields; no re-proof needed.
