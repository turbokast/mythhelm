---
layout: default
title: Limitations
---

# Limitations

MYTHHELM is **pre-alpha**: the only release, v0.0.1, is a source tag with no
binary archives. This register lists what is supported today and what is not,
so expectations stay honest.
Anything below marked experimental may change without notice.

## Supported today

- Headless `mythhelm run` with admission, supervision, verification, review,
  apply and recovery, on Linux and macOS. Windows lifecycle evidence is limited
  to the experimental scripted subset below.
- `mythhelm demo`: a fully offline scripted run in a disposable repository;
  every screen is labelled `SCRIPTED DEMO`.
- `mythhelm doctor`: a read-only prerequisite report that writes nothing. Its
  `qualification:` section lists each harness record with its progress, the
  failing columns and the next test that could advance it.
- `mythhelm stop`, `mythhelm recover` and `mythhelm runs list`, run locally
  from the CLI.
- A budget ledger for every admitted run (typed usage rows, one local quota
  reservation) and finite run envelopes, with the limits listed below.
- Execution profiles: `restricted` (the default) contains the native process
  on Linux where the boundary can be entered; `trusted-host` opts out and is
  disclosed on every surface.
- `mythhelm supervisor status`, `mythhelm migrate --preview` and
  `mythhelm migrate --apply --yes`, with the limits listed below.
- The TUI is shipped: the focused mission view on a TTY, with responsive
  layouts, palette/command navigation and event-driven motion.
- Protected acceptance for the one-agent flow: verdicts bound to the candidate,
  contract, check definitions and evaluator, `NOT RUN` and `STALE` outcomes and
  a documented JSONL event set, with the limits listed below.
- Stable linear output without a TTY, under `TERM=dumb`, with `--plain`, and
  machine output (`--format jsonl`).

TUI behaviour is qualified by the G09 evidence record in the repository
(`docs/tui-slice-g09-evidence.md`); unrecorded combinations are experimental.

## TUI limits

- Only the recorded interactive combination is claimed: GNOME Terminal 3.52
  (VTE 0.76) + zsh 5.9 on Ubuntu 24.04, 190x45. Interactive use in any other
  terminal/shell is experimental until recorded.
- Screen-reader support is experimental: no human session recorded yet.
- Exit and interrupt behaviour depends on mode, phase and the admitted
  capability. The G09 findings (unwired `q` rows, mid-run Ctrl-C cancelling)
  describe the build G09 recorded; the behaviour below is covered by source
  tests only. Its packaged-binary proof (tui-observatory-view Task 10, #521) is
  open, and no terminal, platform or native route is qualified for it.
  - `q` opens selectable exit options (arrows, Tab or `j`/`k`, then Enter; Esc
    goes back). An inspect view or a live run that has ended offers quit and
    back. A live run that has not ended offers detach (leave running), request
    stop and back; a disabled row shows its reason and does nothing, and request
    stop opens a confirmation that rechecks the current state.
  - Deliberate detach (the row or `d`) is enabled only while the worker is
    running and the admission record reports `worker_detachment` as
    `supported`; otherwise it says `detach is not qualified on this platform`.
  - In a live view, the first Ctrl-C before verifying sends one stop request
    and shows `stop requested — waiting for worker confirmation`. The request
    is not a confirmed stop: only the worker's confirmation is. A second Ctrl-C
    while the worker runs detaches even when detachment is not qualified, says
    `not qualified on this platform`, and exits 6; before the worker runs it
    waits. During verifying or applying the second Ctrl-C leaves the view and
    the phase finishes without it. A run that has ended exits on the first.
  - A stop, recover or apply action confirmed in the dialog that is still in
    flight needs a second
    Ctrl-C to quit, and quitting then cancels it and exits 6 (action
    unresolved); an inspect view that leaves a stop unconfirmed also exits 6.

## Acceptance limits

Protected acceptance binds a verification verdict to the candidate, the task
contract, the check definitions and the evaluator, and labels a run that was
not verified `NOT RUN`. What it does not cover is listed here; the
[user guide]({{ site.baseurl }}/user-guide.html#acceptance) describes what it does.

- **The files a check executes are not protected.** The check commands, their
  timeouts and the evaluator come from the admitted configuration and a
  candidate cannot edit them, but the test files and scripts those commands run
  are candidate content. A candidate that weakens a test can still pass the
  check that runs it. No protected-paths mechanism exists.
- **The identity of the checks' own tools is unknown.** The version of the
  compiler or test runner a check uses is not recorded, and the environment
  fingerprint covers the operating system, architecture, verifier and variable
  *names* only: a changed value, for example a `PATH` that points at another
  toolchain, leaves it unchanged. The environment is recorded and bound but not
  compared when you apply.
- **The attestation is not out of reach of your own user.** Immutability is
  enforced by database triggers. Under `trusted-host`, and for any process that
  runs as you, the state directory can be rewritten; only the `restricted`
  profile on Linux runs the checks inside a boundary, and the receipt says when
  a run is not adversarially contained.
- **An unverified candidate is `ready_for_review (unverified)`, exit 5.** The
  master specification asks for a `blocked` run with `verification_unavailable`;
  the run state machine still in use treats `blocked` as terminal and not
  appliable, so the candidate would be stranded. A `failed` or `INVALID` run
  cannot be applied at all ([#380](https://github.com/turbokast/mythhelm/issues/380)).
- **A stale verdict cannot be re-evaluated.** No command re-runs the checks on
  a changed candidate, contract or check definition: a stale or unbound run can
  only be refused, or applied with `--accept-unverified`. A verification
  recorded before schema 8 is `unbound` and takes the same path. A run admitted
  before the pin existed and recovered afterwards ends `NOT RUN`
  (`pin_missing`) rather than running its checks.
- **No task revision is recorded for a new run.** The contract revision is
  always 1, identified by the task file's digest; a legitimate change to the
  task or the checks is a new run, not a recorded revision of the old one.
- **Checks run once.** There is no retry, no repetition and no seed, so a
  check's own nondeterminism is not detected or recorded, and the receipt says
  only that it is not asserted. The check environment is recorded as a
  fingerprint, not pinned: a run finished by `mythhelm recover` rebuilds it
  from the recovering process, so the same revisions can bind a different
  fingerprint than a run that never crashed. Required coverage and risk-specific review are
  not modelled, and `--no-checks` is the only reduced-check mode: it is
  labelled `NOT RUN (waived by --no-checks)`, with no separate draft label.
- **Only the execution deadline is enforced.** The receipt lists the repair,
  replan and transport-retry ceilings as `enforced: false`
  ([#328](https://github.com/turbokast/mythhelm/issues/328)). The recorded
  acceptance overhead is the binding step alone; recording and rendering are
  bounded by 30 second limits but cannot time themselves.
- **The TUI and `--accessible` views can show a stale verdict as verified.**
  They decide from the candidate commit alone, while `review` and `apply`
  check the whole binding
  ([#393](https://github.com/turbokast/mythhelm/issues/393)).
  `review --format jsonl` is not affected: it adds the current answer as
  `acceptance.status`, though `verification.label` there is still the stored
  verdict.
- **A resumed apply loses its override record.** When `apply` crashes after it
  journaled its intent, the resumed call skips the acceptance check and does
  not record `acceptance.apply_override`, so the receipt cannot show that the
  candidate was applied under `--accept-unverified`
  ([#418](https://github.com/turbokast/mythhelm/issues/418)).
- **The qualified entitlement sentence is not proven end to end.** It prints
  fields of the admitted posture, not the identity of the qualification record
  consulted, because admission does not journal that. No route is live-qualified,
  so every strict `subscription-only` run blocks today and no run prints the
  verified sentence. Declared runs print that MYTHHELM did not verify the
  entitlement, and the paid-continuation state is `unknown`.
- **A receipt field MYTHHELM could not observe is {% include state.html state="unknown" %}, with a reason.**
  Cost, usage and the retry schedule, and for a run recorded before schema 8
  the binding, pin and overhead, are never rendered as zero.

## Not supported yet

- The attention queue for approvals is not supported yet.
- Plugins and routing are still planned. v0.0.1 is the only release and has no
  binaries; the archive pipeline is implemented and dry-run proven on a test
  tag, but has not published a release.
- Narrow adapter coverage: the scripted fake adapter plus a Claude Code
  adapter. Windows native Claude Code and Codex launch remain refused pending
  their separate native-harness qualification; synthetic lifecycle tests do not
  qualify either harness.
- Allowance exhaustion is recognised only from a synthetic fixture shape; no
  recorded native run produces it, so on a real route the bucket is not
  recorded and the retry schedule never starts.
- A run's repair, replan and transport-retry ceilings are checked at launch
  only, and no command records an envelope extension.
- `subscription-only` billing always blocks: no native surface qualifies an
  included-only boundary yet.
- The OAuth-token exception is unwired and refused at both gates.
- macOS MDM preferences and remote cached managed policy as certified trust
  sources; the settings inventory covers files only.
- In S1, stop and recover run locally from the CLI; the supervisor service does not serve them. Routing them through journal ingest is a follow-up (#348).
- Runs do not go through the supervisor service: `run`, `recover` and `apply` take a per-run owner lock, in the invoking process, and `mythhelm supervisor status` is the only production caller of the service's intents.
- On a state root whose migration phase is `drained`, `imported` or `adopted` (that is, after `mythhelm migrate --apply --yes`), `run`, `recover` and `apply` are refused with `ownership_unresolved` (exit 6), so a migrated state directory cannot start or recover a run (#369). The migration is one-way: no command restores its backup (#320).
- `stop` and `recover` rely on the owner-only (0700) state directory and do not authorise a caller further. A per-run capability token is a follow-up (#356).
- `mythhelm recover` quarantines a run whose worker was killed outright (`worker_lost`, exit 6), and a quarantined run blocks every later run with `active_run_exists`; no command resolves it yet (#391).
- `mythhelm apply` accepts only a `ready_for_review` run, so the candidate of a run that ended `failed (verification_failed)` cannot be applied or exported (#380).
- The Linux execution boundary does not block direct network egress (the proxy is pinned through environment variables) or signalling of same-user host processes (no PID namespace). On macOS and Windows, and for the Claude Code route on every OS, `restricted` and `inspect` are refused with exit 7, so real Claude Code runs need `--execution-profile trusted-host`.
- The supervisor's lazy start has Linux and macOS evidence only. The Windows named-pipe transport is covered by tests, but its lazy start and detach are not.

### Windows lifecycle evidence

[Windows CI run 39484713392](https://github.com/turbokast/mythhelm/actions/runs/39484713392)
on amd64 and arm64 exercises `scripted-child-process` fake tests for suspended
launch, Job Object assignment, child/grandchild termination, kill-only stop
(`stop-ladder/v2-windows`), native creation-time identity, CLI exit and reconnect
to the same live worker. First Ctrl-C and Ctrl-Break request a stop with a
confirmed exit 130. These are synthetic fake-process results, not qualified
Windows platform or native-harness support. The [process-support matrix](../internal/processsupport/SUPPORT.md)
has no qualified rows: `process_tree_ownership` and `worker_detachment` remain
`unknown` for every Windows architecture/surface pair.

Windows physical console close passed on arm64 with `ConsoleWindowClass`;
amd64 reported that case blocked under `PseudoConsoleWindow`. The second interrupt
(detach with exit 6 while stopping) is untested on both architectures because
no approved scenario holds the immediate kill-only stop open. Other hosted console,
terminal, shell and filesystem combinations are untested; these results do not
establish a general console-close or worker-detachment guarantee.

Windows launch is refused inside a kill-on-close parent job, or when the parent-job
query fails. After a worker crash, recovery retains `ownership_unresolved` and
quarantine; kill-on-close does not prove ownership reconciled. Before Job Object
assignment, an inert suspended native may survive outside the job, and a crash
before identity persistence can leave it unnamed. Recovery never signals such a
process or launches a replacement. Reconnecting to a live worker after CLI exit
is distinct from recovery after worker death. See [ADR 0017](decisions/0017-windows-process-ownership.md).

### Codex

Codex is not a supported adapter. The repository holds adapter code for it, tested only against synthetic fixtures, and its qualification row stays `planned` (ADR [0018](decisions/0018-codex-adapter-admission.md)). `mythhelm run --adapter codex` blocks or is refused in every billing mode, so no Codex run starts, and `mythhelm doctor` shows the row with its failing columns.

- **Fidelity is unproven.** No matched comparison against Codex run directly exists, so the adapter records instruction and skill discovery, hooks, MCP servers, children and native context as `unobservable`, never `preserved`.
- **Entitlement is unproven.** Nothing shows that Codex stops before purchased credits or overage in an included plan, so strict `subscription-only` also needs unexpired evidence of that prevention and no fixture can supply it. The sign-in category is unknown too, so a strict run that passes the configuration checks ends `no_qualification_record`. Configuration can block it earlier: a user or project `config.toml` ends it `native_config_unreadable` before the version probe, a credential key in a JSON configuration file ends it `native_settings_credential_override`, and hooks or MCP servers end it `untrusted_native_config` until their digest is trusted.
- **Lifecycle is unproven.** The stop ladder is tested against fake processes only, native stop acknowledgement is reported as `unknown`, and the adapter has no resume, reconnect or steering path.
- **Billing postures.** Only strict `subscription-only` is considered. `subscription-declared` and `local-scripted` are refused with `billing_unsupported_for_adapter` before any native process starts, and no runtime route check runs for Codex, so the route rests on admission alone.
- **Native surface.** The app-server command line, frame schema, configuration sources, usage and exhaustion shapes are placeholders: token usage stays unknown and error frames the native reports default to `native_error`. A stream the adapter can no longer read (a malformed result frame or a read failure) is reported as `protocol_mismatch`. When no stop is already running, the worker requests a stop; a confirmed stop with no escaped descendants records the attempt as `stopped`, and the run fails with `protocol_error` only when it is not already stopping. An unconfirmed stop or escaped descendants leaves the run `interrupted`; an existing user stop remains `cancelled`, and a route-violation stop remains `blocked`.
- **Profiles and platforms.** The `restricted` and `inspect` execution profiles are refused for Codex (exit 7), and Codex launch is refused on Windows.
