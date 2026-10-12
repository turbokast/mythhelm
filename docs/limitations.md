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
  apply and recovery, on Linux, macOS and Windows.
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
- The `q` exit-options dialog renders its rows but key selection is unwired;
  Esc closes it, Ctrl-C quits.
- Mid-run Ctrl-C cancels the run (exit 130) instead of detaching.

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
  only that it is not asserted. Required coverage and risk-specific review are
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
  `review --format jsonl` prints the stored receipt only, so it shows a stale
  verdict as `passed` too
  ([#440](https://github.com/turbokast/mythhelm/issues/440)).
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
- **A receipt field MYTHHELM could not observe is `unknown`, with a reason.**
  Cost, usage and the retry schedule, and for a run recorded before schema 8
  the binding, pin and overhead, are never rendered as zero.

## Not supported yet

- Plugins and routing are still planned. v0.0.1 is the only release and has no
  binaries; the archive pipeline is implemented and dry-run proven on a test
  tag, but has not published a release.
- Narrow adapter coverage: the scripted fake adapter plus a Claude Code
  adapter. Native Claude execution is refused on Windows by design (exit 7).
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

### Codex

Codex is not a supported adapter. The repository holds adapter code for it, tested only against synthetic fixtures, and its qualification row stays `planned` (ADR [0018](decisions/0018-codex-adapter-admission.md)). `mythhelm run --adapter codex` blocks or is refused in every billing mode, so no Codex run starts, and `mythhelm doctor` shows the row with its failing columns.

- **Fidelity is unproven.** No matched comparison against Codex run directly exists, so the adapter records instruction and skill discovery, hooks, MCP servers, children and native context as `unobservable`, never `preserved`.
- **Entitlement is unproven.** Nothing shows that Codex stops before purchased credits or overage in an included plan, so strict `subscription-only` also needs unexpired evidence of that prevention and no fixture can supply it. The sign-in category is unknown too, so a strict run that passes the configuration checks ends `no_qualification_record`. Configuration can block it earlier: a user or project `config.toml` ends it `native_config_unreadable` before the version probe, a credential key in the configuration ends it `native_settings_credential_override`, and hooks or MCP servers end it `untrusted_native_config` until their digest is trusted.
- **Lifecycle is unproven.** The stop ladder is tested against fake processes only, native stop acknowledgement is reported as `unknown`, and the adapter has no resume, reconnect or steering path.
- **Billing postures.** Only strict `subscription-only` is considered. `subscription-declared` and `local-scripted` are refused with `billing_unsupported_for_adapter` before any native process starts, and no runtime route check runs for Codex, so the route rests on admission alone.
- **Native surface.** The app-server command line, frame schema, configuration sources, usage and exhaustion shapes are placeholders: token usage stays unknown and error frames the native reports default to `native_error`. A stream the adapter can no longer read (a malformed result frame or a read failure) is reported as `protocol_mismatch`. When no stop is already running, the worker requests a stop; a confirmed stop with no escaped descendants records the attempt as `stopped`, and the run fails with `protocol_error` only when it is not already stopping. An unconfirmed stop or escaped descendants leaves the run `interrupted`; an existing user stop remains `cancelled`, and a route-violation stop remains `blocked`.
- **Profiles and platforms.** The `restricted` and `inspect` execution profiles are refused for Codex (exit 7), and Codex launch is refused on Windows.
