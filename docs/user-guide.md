---
layout: default
title: User guide
---

# User guide

MYTHHELM is the open-source command deck for native coding agents. This guide
covers installation and the scripted quickstart. MYTHHELM is
**pre-alpha**: there is no usable release yet, so read
[Limitations]({{ site.baseurl }}/limitations.html) before relying on anything
here.

## Installation

MYTHHELM needs Go 1.27 or newer. `go.mod` pins the toolchain, so with the
default `GOTOOLCHAIN=auto` an older `go` command downloads the right version
the first time it runs. Nothing else is required: no accounts, credentials or
network services.

```sh
git clone https://github.com/turbokast/mythhelm.git
cd mythhelm
go run ./cmd/mythhelm version   # build and run the CLI
```

## Quickstart

The fastest way to see MYTHHELM work is the fully offline scripted demo. It
runs in a disposable repository, needs no credentials and no network at
runtime, and labels every screen `SCRIPTED DEMO`:

```sh
go run ./cmd/mythhelm demo
```

![Scripted demo recording](demos/demo.gif)
*Honest recording of `mythhelm demo --check pass`, rendered from `docs/demos/demo.tape` — regenerate it with `docs/demos/record.sh`.*

To check what your machine still needs for a real run, use the read-only
prerequisite report. It writes nothing and prints no credential values:

```sh
go run ./cmd/mythhelm doctor
```

## Limitations

The [limitations register]({{ site.baseurl }}/limitations.html) lists what is
supported today: pre-alpha status, narrow adapter coverage, and the shipped
TUI with its known limits. Check it before reporting something as a bug.

## Next steps

- [Contributing]({{ site.baseurl }}/contributing.html): governance, the
  contribution path and DCO sign-off.
- [Source on GitHub](https://github.com/turbokast/mythhelm): issues,
  discussions and the full tree.
