<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/assets/brand/lockup-on-dark.png">
    <img alt="MYTHHELM: Many agents. One mission." src="docs/assets/brand/lockup-on-light.png" width="480">
  </picture>
</p>

[![CI](https://github.com/turbokast/mythhelm/actions/workflows/ci.yml/badge.svg)](https://github.com/turbokast/mythhelm/actions/workflows/ci.yml)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/turbokast/mythhelm/badge)](https://scorecard.dev/viewer/?uri=github.com/turbokast/mythhelm)
[![License: Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)
[![OpenSSF Best Practices](https://www.bestpractices.dev/projects/15212/badge)](https://www.bestpractices.dev/projects/15212)

**The open-source command deck for native coding agents.**

MYTHHELM orchestrates the coding agents you already use and trust, such as Claude Code, Codex and others, with their native harnesses intact. It gives them a shared mission, safe working boundaries, an honest control plane and a terminal experience worth opening.

> **Status: pre-alpha.** The only release so far, v0.0.1, is a source tag with no binary archives, so you build from source. The headless dogfood slice (`run`, `review`, `apply`, `recover`) plus an offline `demo` and a read-only `doctor` are implemented on `main` and covered by a packaged-binary end-to-end suite. `stop` and `runs list`, a budget ledger with finite run envelopes, execution profiles, a per-user supervisor and a one-way `migrate` have landed since, with the limits listed below and in the [limitations register](docs/limitations.md). The TUI is also implemented, with the evidence and limitations below. The broader adaptive product remains a design contract, not an implemented or benchmarked capability.

## Dogfood slice status

Supported now:

- Headless `mythhelm run --adapter fake|claudecode` with admission, supervision, verification, review, apply and recovery.
- `mythhelm demo`: a fully offline scripted run in a disposable repository; every screen is labelled `SCRIPTED DEMO`.
- `mythhelm doctor`: read-only prerequisite report; it writes nothing and prints no credential values. Its `qualification:` section lists each harness record with its progress, the failing columns and the next test that could advance it.
- `mythhelm stop <run-id>` and `mythhelm recover <run-id>`: a stop request that the run's worker acknowledges, and recovery of an interrupted run without relaunching native work. Both run locally from the CLI.
- `mythhelm runs list`: the runs in the local state, newest first.
- A budget ledger for every admitted run: usage rows labelled `reported`, `estimated` or `unknown`, one local quota reservation, and finite run envelopes (execution time, repairs, replans, transport retries) set with `--envelope-*` flags or `mythhelm.toml`. See the [user guide](docs/user-guide.md#budget-ledger).
- Execution profiles: `restricted` is the default. On Linux, where the boundary can be entered, it runs the native process in a user and mount namespace with a read-only root and a filtering proxy for network access. `--execution-profile trusted-host` opts out and is disclosed on every surface.
- `mythhelm supervisor status`: shows the per-user supervisor service's state, starting it if needed, over authenticated local IPC (Unix socket on Linux and macOS; the Windows named-pipe transport is tested, but lazy start has no Windows evidence). Runs are not routed through the service yet: `run`, `recover` and `apply` still hold a per-run owner lock.
- `mythhelm migrate`: preview, or apply with `--yes`, the move of a state directory to the v2 ledger. It is one-way; see "Not supported yet" for what it leaves refused.
- Linux and macOS; Windows amd64 and arm64 `scripted-child-process` lifecycle tests cover an experimental fake-process subset only. The Windows support matrix has no qualified rows; native Claude Code and Codex launch remain refused pending separate qualification. Worker crashes retain unresolved ownership and quarantine; launch inside a kill-on-close parent job is refused. Second interrupt and hosted console cases remain untested or blocked. See [Windows lifecycle evidence](docs/limitations.md#windows-lifecycle-evidence). Off Linux, scripted runs need `--execution-profile trusted-host` (the default `restricted` profile is refused there).
- Dogfood billing postures `local-scripted` and `subscription-declared` (user-declared, never verified by MYTHHELM).
- Codex is not yet a supported adapter: `--adapter codex` has fixture-tested adapter code only, its registry row stays `planned`, and every run blocks or is refused (see [Codex limits](docs/limitations.md#codex)).

Not supported yet:

- Plugins and routing: still planned. Releases: v0.0.1 is the only release and carries no binaries. The archive pipeline is implemented and dry-run proven on a test tag, but it has not published a release (see [the release process](docs/release-process.md)).
- Contained execution beyond Linux. On macOS and Windows `restricted` and `inspect` are refused with exit 7, and so are both for the Claude Code route on every OS (its credential boundary is not enforced yet), so real Claude Code runs need `--execution-profile trusted-host`. The Linux boundary does not block direct network egress or signals to same-user host processes.
- Using the supervisor service for runs. `stop` and `recover` are not served by it (#348).
- Life after `mythhelm migrate --apply --yes`: `run`, `recover` and `apply` are then refused with exit 6, and no command restores the backup (#320, #369). Do not apply the migration unless you accept that.
- `subscription-only`: it blocks today, because no live-qualified qualification record exists for any route.
- The AC-4.7 OAuth-token exception: unwired, refused at both gates (see [ADR 0002](docs/decisions/0002-dogfood-billing-posture.md)).
- macOS MDM preferences and remote cached managed policy as certified trust sources; the settings inventory covers files only.

Try it with no credentials and no network at runtime: `go run ./cmd/mythhelm demo` (the first build needs the Go module cache populated). `go run ./cmd/mythhelm doctor` reports what your machine still needs for a real run.

![Kill the terminal mid-run: recover and finish, scripted](docs/demos/hero.gif)
*Scripted recording, fake agent, offline, no credentials, rendered from `docs/demos/hero.tape`. It kills the foreground `mythhelm run` with SIGKILL while the scripted agent works: the worker and the agent are separate processes and survive, and `mythhelm recover` reattaches to the same worker and finishes the run without relaunching the agent. A hard-killed worker is a different case and is not shown (#391). The red kill marker is a shell annotation, not mythhelm output. Regenerate it with `docs/demos/record.sh`.*

![Scripted demo recording](docs/demos/demo.gif)
*Honest recording of `mythhelm demo --check pass`, rendered from `docs/demos/demo.tape` — regenerate it with `docs/demos/record.sh`.*

![Scripted TUI recording](docs/demos/tui.gif)
*Honest recording of the TUI driving `mythhelm demo --check pass`, rendered from `docs/demos/tui.tape` — regenerate it with `docs/demos/record.sh` from the pinned revision (`docs/demos/manifest.json`), or with `docs/demos/record.sh --repin` from a clean tree.*

## TUI status

Supported now (evidence: [G09 record](docs/tui-slice-g09-evidence.md); unrecorded combinations are experimental per I14):

- The focused mission view on a TTY: `mythhelm run` / `demo` / `review` launch the Bubble Tea TUI with responsive layouts (wide, two-pane, single, compact), palette/command navigation and event-driven motion.
- Stable linear output without a TTY, under `TERM=dumb`, with `--plain`, and machine output (`--format jsonl`); `--accessible` emits the ordered screen-reader stream. Covered on Linux, macOS and Windows (x86_64 and ARM) by the packaged-binary end-to-end suite.
- Explicit `--colour` / `--motion` / `--icons` overrides; `NO_COLOR` and `TERM=dumb` suppress.

Recorded interactive combination (Revision 1.1 §16.4 row in the G09 record):

- GNOME Terminal 3.52 (VTE 0.76) + zsh 5.9 on Ubuntu 24.04, 190x45.

Experimental until recorded in the G09 record:

- Interactive use in any other terminal/shell: only the combinations with full evidence rows are claimed.
- Screen-reader support: no human session recorded yet, all combinations experimental (AC-5.4).

Known limitations (G09 findings, see the record §5):

- The `q` exit-options dialog renders its rows but key selection is unwired; Esc closes it, Ctrl-C quits.
- Mid-run Ctrl-C cancels the run (exit 130) instead of detaching; the detach path needs a slow-adapter retest.

## Principles

We measure backlog delivery and require admitted MYTHHELM dogfooding where
available, with explicit fallback records when blocked. We have **not yet
established a comparative performance advantage**. See [benchmarks and
dogfooding](docs/benchmarks.md) for the evidence, metrics and publication criteria.
A [measured native run](docs/dogfood/0003-measured-self-hosting.md) has authored and
verified a repository maintenance change through MYTHHELM; its small comparison
does not establish a speed or quality advantage.

The next delivery focus is a [bounded validation cycle](docs/validation-plan.md)
for dependable single-agent delivery, with external repeat use and predefined
continue/simplify/stop criteria. Broader expansion waits for that evidence.

- **Native harnesses preserved.** MYTHHELM drives each agent through its own supported surface. It does not replace or impersonate it.
- **Your subscriptions, no billing surprises.** Work runs against the allowances you already have. Metered spend is never started silently.
- **Honest state.** Estimated, reported, observed and unknown values stay distinguishable everywhere.
- **Safe by default.** Repository text, model output and plugins cannot grant permissions, spend money or publish.
- **Entirely free.** The complete core, built-in adapters, TUI, headless mode, plugin SDK and local routing are free, with no paid tier, feature gate or required account.

## Design

The [specification index](docs/spec/README.md) identifies [Master Specification v2](mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md) as the current design and preserves Revision 1.1 for historical references. The full vision combines native continuity, verified delivery, bounded parallel construction and evidence-gated adaptation. The first useful milestone is one qualified native route completing the task-to-verified-candidate loop. The [product adoption map](docs/spec/synthesis-adoption.md) connects all 16 implementation slices to the backlog.

Published user and contributor guides: <https://turbokast.github.io/mythhelm/>.

## Planned stack

Go core with a [Bubble Tea](https://github.com/charmbracelet/bubbletea) TUI, local SQLite state and a versioned process protocol for plugins.

## Contributing

Contributions are welcome, and you don't need a paid agent subscription to make a useful one. Read [CONTRIBUTING.md](CONTRIBUTING.md) first. All commits must be signed off under the [Developer Certificate of Origin](DCO).

- Questions and ideas: [GitHub Discussions](https://github.com/turbokast/mythhelm/discussions)
- Bugs: [issues](https://github.com/turbokast/mythhelm/issues/new/choose)
- Security vulnerabilities: see [SECURITY.md](SECURITY.md). Don't open a public issue.

## License

[Apache License 2.0](LICENSE). See [NOTICE](NOTICE).

MYTHHELM is an independent project. It is not affiliated with or endorsed by any agent vendor. Product names belong to their respective owners.
