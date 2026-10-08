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

![Scripted TUI recording](demos/tui.gif)
*Honest recording of the TUI driving `mythhelm demo --check pass`, rendered from `docs/demos/tui.tape` — regenerate it with `docs/demos/record.sh` from the pinned revision (`docs/demos/manifest.json`), or with `docs/demos/record.sh --repin` from a clean tree.*

To check what your machine still needs for a real run, use the read-only
prerequisite report. It writes nothing and prints no credential values:

```sh
go run ./cmd/mythhelm doctor
```

## Doctor

`mythhelm doctor` is a read-only prerequisite report: it writes nothing,
creates no state and prints no credential values. Its last section,
`qualification:`, lists one line per harness record in the qualification
registry:

```text
qualification:
claude-code × unknown: blocked (fidelity unknown, entitlement unknown, lifecycle unknown) [evidence 1 revs, latest none; drift clean]
```

Each line reads `<harness> × <surface>: <progress> (fidelity <v>,
entitlement <v>, lifecycle <v>) [evidence <n> revs, latest <ev-id>; drift
<state>]`: the qualification progress (`planned`, `blocked`, `unsupported`
and the other honest labels), the three independent column verdicts
(`proven`, `not-proven` or `unknown`), how many evidence revisions the
record holds, the latest evidence id, and the drift state (`clean`, or the
reason the pinned binary or configuration no longer matches).

When the state directory cannot be read the section says so instead of
guessing — `qualification: unavailable (<reason>)` — and an existing but
still empty registry reads `qualification: (no records)`. With
`--format jsonl` the same records appear under `qualification.records`,
each with its progress, per-column verdicts and evidence ids, evidence
revision count, drift triggers and next test.

## Billing

`mythhelm run --adapter claudecode` requires `--billing` and has no default.
Two postures apply to a native Claude Code subscription login, and they prove
different things. The posture is recorded on the admission row, the journal,
the receipt and `mythhelm runs`.

**`--billing subscription-only` (strict).** The run is admitted only when the
qualification registry holds a live-qualified record for this exact route, and
that record's entitlement evidence and stop-at-exhaustion capability are both
unexpired at admission. Any other qualification outcome blocks with exit code 3 and a reason in
the output (an operational error, such as an unreadable registry, instead uses
its normal exit code):

- `no_qualification_record`: the registry has no record for this route.
- `entitlement_not_proven`: the record is not live-qualified (for example it is
  fixture-only), its entitlement is unproven or expired, or the effective
  managed policy has a source MYTHHELM cannot yet inventory.
- `stop_at_exhaustion_unproven`: the record cannot show the run stops, rather
  than charges, when the allowance is exhausted.
- `ambiguous_qualification_match`: more than one record matches the route.
- `qualification_drifted`: the executable, configuration or account class
  changed since the record was earned. Run `mythhelm doctor` to inspect it.

Strict ignores any entitlement declaration. A matching record admits only on an
exact match of the route's authentication category, provider endpoint, workspace
class, effort settings and model snapshot, together with its harness, surface,
OS, architecture, trust profile, entitlement class and adapter protocol. No live-qualified record has been
recorded yet, so every strict run blocks today.

**`--billing subscription-declared`.** For personal use, you declare your plan
and that you checked extra usage is disabled:

```sh
mythhelm run --adapter claudecode --billing subscription-declared \
  --declare-entitlement plan=pro,extra-usage=disabled ...
```

The declaration is user-declared, never verified: MYTHHELM does not check it,
and the run is labelled `entitlement: user-declared, NOT verified by MYTHHELM;
paid continuation: unknown (user declares disabled)`. It does not claim that
paid continuation is prevented. A declared run still needs a first-party
subscription login, and the declaration is bound to that account and
configuration directory, so a changed account needs a fresh declaration.

`--billing local-scripted` is the offline posture; Claude Code refuses it. The
decisions behind both postures are in
[ADR 0002](https://github.com/turbokast/mythhelm/blob/main/docs/decisions/0002-dogfood-billing-posture.md) and
[ADR 0012](https://github.com/turbokast/mythhelm/blob/main/docs/decisions/0012-strict-subscription-admission.md).

## Limitations

The [limitations register]({{ site.baseurl }}/limitations.html) lists what is
supported today: pre-alpha status, narrow adapter coverage, and the shipped
TUI with its known limits. Check it before reporting something as a bug.

## Next steps

- [Contributing]({{ site.baseurl }}/contributing.html): governance, the
  contribution path and DCO sign-off.
- [Source on GitHub](https://github.com/turbokast/mythhelm): issues,
  discussions and the full tree.
