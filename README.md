<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/assets/brand/lockup-on-dark.png">
    <img alt="MYTHHELM: Many agents. One mission." src="docs/assets/brand/lockup-on-light.png" width="480">
  </picture>
</p>

[![CI](https://github.com/turbokast/mythhelm/actions/workflows/ci.yml/badge.svg)](https://github.com/turbokast/mythhelm/actions/workflows/ci.yml)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/turbokast/mythhelm/badge)](https://scorecard.dev/viewer/?uri=github.com/turbokast/mythhelm)
[![License: Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

**The open-source command deck for native coding agents.**

MYTHHELM orchestrates the coding agents you already use and trust, such as Claude Code, Codex and others, with their native harnesses intact. It gives them a shared mission, safe working boundaries, an honest control plane and a terminal experience worth opening.

> **Status: pre-alpha, design stage.** There is no usable release yet. Everything described here is a proposed contract, not an implemented or benchmarked capability.

## Principles

- **Native harnesses preserved.** MYTHHELM drives each agent through its own supported surface. It does not replace or impersonate it.
- **Your subscriptions, no billing surprises.** Work runs against the allowances you already have. Metered spend is never started silently.
- **Honest state.** Estimated, reported, observed and unknown values stay distinguishable everywhere.
- **Safe by default.** Repository text, model output and plugins cannot grant permissions, spend money or publish.
- **Entirely free.** The complete core, built-in adapters, TUI, headless mode, plugin SDK and local routing are free, with no paid tier, feature gate or required account.

## Design

The [master specification](docs/spec/master-spec.md) is the versioned design reference. It covers the architecture, invariants, adapter and billing contracts, the TUI and the staged roadmap. Concise user and contributor guides will be extracted from it as the software becomes real.

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
