---
layout: default
title: Limitations
---

# Limitations

MYTHHELM is **pre-alpha**: there is no usable release yet. This register
lists what is supported today and what is not, so expectations stay honest.
Anything below marked experimental may change without notice.

## Supported today

- Headless `mythhelm run` with admission, supervision, verification, review,
  apply and recovery, on Linux, macOS and Windows.
- `mythhelm demo`: a fully offline scripted run in a disposable repository;
  every screen is labelled `SCRIPTED DEMO`.
- `mythhelm doctor`: a read-only prerequisite report that writes nothing.
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

- Plugins, routing and releases — all still planned.
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
- `mythhelm recover` is refused on a state root whose migration phase is `drained`, `imported` or `adopted`, so a migrated run has no recover command yet (#369).
- Stop and recover authorise a caller by same-user peer identity. A per-run capability token is a follow-up (#356).

### Codex

Codex is not a supported adapter. The repository holds adapter code for it, tested only against synthetic fixtures, and its qualification row stays `planned` (ADR [0018](decisions/0018-codex-adapter-admission.md)). `mythhelm run --adapter codex` blocks or is refused in every billing mode, so no Codex run starts, and `mythhelm doctor` shows the row with its failing columns.

- **Fidelity is unproven.** No matched comparison against Codex run directly exists, so the adapter records instruction and skill discovery, hooks, MCP servers, children and native context as `unobservable`, never `preserved`.
- **Entitlement is unproven.** Nothing shows that Codex stops before purchased credits or overage in an included plan, so strict `subscription-only` also needs unexpired evidence of that prevention and no fixture can supply it. The sign-in category is unknown too, so a strict run ends `no_qualification_record`.
- **Lifecycle is unproven.** The stop ladder is tested against fake processes only, native stop acknowledgement is reported as `unknown`, and the adapter has no resume, reconnect or steering path.
- **Billing postures.** Only strict `subscription-only` is considered. `subscription-declared` and `local-scripted` are refused with `billing_unsupported_for_adapter` before any native process starts, and no runtime route check runs for Codex, so the route rests on admission alone.
- **Native surface.** The app-server command line, frame schema, configuration sources, usage and exhaustion shapes are placeholders: token usage stays unknown and error frames the native reports default to `native_error`. A stream the adapter can no longer read (a malformed result frame or a read failure) is reported as `protocol_mismatch`. When no stop is already running, the worker requests a stop; a confirmed stop with no escaped descendants records the attempt as `stopped`, and the run fails with `protocol_error` only when it is not already stopping. An unconfirmed stop or escaped descendants leaves the run `interrupted`; an existing user stop remains `cancelled`, and a route-violation stop remains `blocked`.
- **Profiles and platforms.** The `restricted` and `inspect` execution profiles are refused for Codex (exit 7), and Codex launch is refused on Windows.
