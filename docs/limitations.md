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
- **Entitlement is unproven.** Nothing shows that Codex stops before purchased credits or overage in an included plan, so strict `subscription-only` also needs unexpired evidence of that prevention and no fixture can supply it. The sign-in category is unknown too, so a strict run ends `no_qualification_record`.
- **Lifecycle is unproven.** The stop ladder is tested against fake processes only, native stop acknowledgement is reported as `unknown`, and the adapter has no resume, reconnect or steering path.
- **Billing postures.** Only strict `subscription-only` is considered. `subscription-declared` and `local-scripted` are refused with `billing_unsupported_for_adapter` before any native process starts, and no runtime route check runs for Codex, so the route rests on admission alone.
- **Native surface.** The app-server command line, frame schema, configuration sources, usage and exhaustion shapes are placeholders: token usage stays unknown and error frames the native reports default to `native_error`. A stream the adapter can no longer read (a malformed result frame or a read failure) is reported as `protocol_mismatch`. When no stop is already running, the worker requests a stop; a confirmed stop with no escaped descendants records the attempt as `stopped`, and the run fails with `protocol_error` only when it is not already stopping. An unconfirmed stop or escaped descendants leaves the run `interrupted`; an existing user stop remains `cancelled`, and a route-violation stop remains `blocked`.
- **Profiles and platforms.** The `restricted` and `inspect` execution profiles are refused for Codex (exit 7), and Codex launch is refused on Windows.
