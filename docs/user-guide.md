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

### Budget ledger

Every admitted run keeps a budget ledger in the journal: typed usage rows
recorded when available, one quota reservation, one envelope row and, when an
allowance runs out, one exhausted bucket. A run refused at admission — for
example when its reservation hold fails — leaves no ledger rows. The ledger records what was observed and what was decided. It never
claims a provider balance, a remaining quota or a price that was actually
charged; anything MYTHHELM did not observe is reported as `unknown`, never as
zero.

**Usage rows.** Each native result projects into typed usage rows keyed by
scope, unit and source (for example per-model token counts and a
retail-equivalent cost). Every row carries a label: `reported` for exact
observed totals, `estimated` for totals built from identity-less deltas or
converted money, and `unknown` for scopes with no usable reading. The receipt's
`billing.usage_observations` lists each row with its scope, unit, source,
label and quantity, and retail rows add the note `estimate, not a charge`.
After a run, MYTHHELM prints one `usage:` line per row, for example
`usage: retail-equivalent unknown unknown (unknown, unknown)` when the adapter
surfaced no usage. A fake-adapter run records only that retail `unknown`
marker.

**Remaining and next retry.** The receipt's `billing.remaining` is always
`{quantity: unknown, bucket}`, and the CLI prints `remaining: unknown`:
MYTHHELM cannot see provider-side quota, so it reports nothing. When a run
exhausts its allowance, `billing.next_retry` and the `next retry:` line show
the operator retry schedule (`at`, `retries_used`, `retries_max: 3`); a run
that never exhausted renders `next retry: none (used 0 of 3)`. These lines
print on the linear, jsonl and demo paths only, not in the full-screen TUI.

**Envelopes.** Every admitted run carries an envelope: ceilings for execution
time, repairs, replans and transport retries. The built-ins are 30 minutes, 3
repairs, 2 replans and 5 transport retries. Per-run flags override them:

```sh
mythhelm run --envelope-execution 45m --envelope-repairs 5 ...
```

`--envelope-execution` takes a positive duration; `--envelope-repairs`,
`--envelope-replans` and `--envelope-transport-retries` take counts. A
`[envelopes]` table in `mythhelm.toml` sets the middle layer
(`execution = "45m"`, `repairs = 5`, `replans`, `transport_retries`), decoded
strictly: unknown keys, invalid durations and negative counts refuse admission.
The file is read only for runs that admit checks: a `--no-checks` run ignores
the `[envelopes]` table and uses flags and built-ins.
Precedence is flags, then file, then built-ins. Launches past a ceiling are
refused and the run becomes `blocked` with a reason naming the exhausted
kind (`envelope_repairs_exhausted`, `envelope_replans_exhausted`,
`envelope_transport_retries_exhausted`, `envelope_deadline_exceeded`).
Each S1 run launches a single attempt, so only the execution ceiling stops a
live run, whether the launching supervisor or `mythhelm recover` is watching
it; the repair, replan and transport-retry ceilings are checked at launch only
(#328). Extending a run requires a recorded user or standing-grant decision;
S1 can record only the `run.extension_granted` user decision, no command
records one yet, and a `blocked` run is not relaunched by `mythhelm recover`
(#329). Flags and configuration changes cannot raise a ceiling mid-run.

**Completion reserve.** Before dispatching a material replan after the first
verification, MYTHHELM checks that the execution time left covers one more
verification pass, and a dispatched replan then runs under a derived deadline
— the execution deadline minus the verify-pass timeout sum — so the reserve
survives the replan. The `reserve:` line and the receipt's `billing.reserve`
show this estimate (`verify pass (...) + N repairs`), labelled
`estimate, not a reserve of provider quota`. It is planning arithmetic, not a
claim on provider capacity; repairs are never gated by it. For checked runs
the receipt's verify pass reads `unknown`, because check timeouts are not
journaled — only the CLI line, which reads the live admission decision, is
exact there.

**Exhaustion.** When the allowance is exhausted, the run keeps its frozen
candidate and session artifacts, ends `blocked` with reason
`allowance_exhausted`, and records the bucket. While the bucket row is live,
new runs on the same bucket are refused with `allowance_exhausted`; the
operator may retry on the recorded schedule (backoff 1m, 5m, 15m, then stop
when the reset is unknown, at most 3 retries), and MYTHHELM itself launches
nothing automatically. Once the 3 retries are spent the bucket refuses new
runs for good: S1 records no reset, so nothing clears the row and no command
does either (#327, awaiting a maintainer decision). The run's reservation is
released only after the attempt reaches terminal state.

The exhaustion signal is synthetic. The Claude Code decoder recognises it from
a fixture shape (`allowance_exhausted`) that no recorded native run has
produced; on a real route an exhausted allowance surfaces as a provider-limit
or billing error and the bucket is not recorded.

**Reservations.** Admission holds exactly one quota reservation coupled to the
run's bucket, with quantity `unknown`. The reservation is
`local coordination only — not provider availability`: it coordinates
MYTHHELM's own runs and says nothing about what the provider will serve.

The billing, persistence and process-ownership decisions behind the ledger are
in
[ADR 0015](https://github.com/turbokast/mythhelm/blob/main/docs/decisions/0015-budget-ledger-s1.md).

## Migrating an existing state directory

`mythhelm migrate` brings a state directory written by the legacy per-run
writer onto the v2 ledger. `mythhelm migrate --preview` (the default)
prints the plan, the runs to import and the owners to drain, using no
credentials and no network, and writes nothing. `mythhelm migrate --apply
--yes` repeats the preview, then drains the legacy run owners (an owner
still holding its lock at the deadline stops the migration with
`ownership_unresolved`, exit 6), takes a backup next to the ledger as
`mythhelm.db.bak-migration-v7` with a `.json` sidecar, and imports every
legacy run as a one-task run. Imported runs keep their IDs, billing
posture and evidence, and stay unverified. Without `--yes`, `--apply`
prints the preview and writes nothing.

While `--apply` runs, migration holds the instance, state-directory and
run-owner locks, so the legacy `run`, `recover` and `apply` commands
refuse. They keep refusing once the `drained` phase is recorded, which
the same run does right after the backup; a migration interrupted before
that record leaves the phase at `previewed`, where they are admitted
again. A fresh apply never overwrites a backup: an unrecorded stale
backup at the default path must be moved
aside first. A backup the ledger has recorded is reused for the resume,
verified by its digest, and must stay in place: moving it aside makes
the next apply refuse. No command restores the backup yet
([#320](https://github.com/turbokast/mythhelm/issues/320)), and a
migration interrupted between the backup and the drain record can reuse a
backup that misses later legacy writes
([#341](https://github.com/turbokast/mythhelm/issues/341)). The decisions
behind the migration are in
[ADR 0016](https://github.com/turbokast/mythhelm/blob/main/docs/decisions/0016-migration-import.md).

## Limitations

The [limitations register]({{ site.baseurl }}/limitations.html) lists what is
supported today: pre-alpha status, narrow adapter coverage, and the shipped
TUI with its known limits. Check it before reporting something as a bug.

## Next steps

- [Contributing]({{ site.baseurl }}/contributing.html): governance, the
  contribution path and DCO sign-off.
- [Source on GitHub](https://github.com/turbokast/mythhelm): issues,
  discussions and the full tree.
