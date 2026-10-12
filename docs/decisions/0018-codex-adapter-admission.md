# 0018. Codex adapter: decider table, strict-only billing, one input writer, no runtime route check and the worker-initiated protocol stop

- Status: proposed
- Date: 2026-10-11

## Context

The Codex native adapter (spec `specs/*/codex-native-adapter/`) adds a second
native harness beside Claude Code. Nothing about Codex's real surface could
be verified while building it: no `codex` executable was available, so the
app-server argv, the frame schema, the sign-in status source, the exhaustion
shapes and the managed-policy sources are placeholders held at the spec's
open-question defaults (OQ1, OQ6, OQ9, OQ10). The adapter code is therefore
fixture-tested only, and its registry row stays `planned` (D4).

Five choices changed billing, process ownership or the contract between the
adapter seam and the worker, so they are recorded here. Each keeps its spec
decision ID (D1, D3, D10, D12 from the design; D18 was added during
implementation, see below).

## Decision

**D1: a per-harness decider table in `internal/admission`.** `Decide` and
`validate` read one table of `harnessDecider` entries (`claudecode`, `codex`,
`fake`) instead of a `switch` at each site of `internal/admission`. The
packages that launch the native are outside the table and still branch on the
adapter ID (see Consequences). The table is package-private, so no code outside
the module can register an adapter and bypass admission (I11), and it is passed
as a value to an unexported `decideWith`, so tests supply their own table
without mutating package state. Rejected: an exported
registry. Pinned by `TestDecideDispatchesByTable`
(`internal/admission/admission_test.go`) and `TestHarnessRowsMatchBoundaryRoute`.

**D3: Codex runs only under strict `subscription-only`.**
`subscription-declared` and `local-scripted` are refused for Codex with
`billing_unsupported_for_adapter`, before any native process starts. A
declaration path is a billing posture choice (I15: a declaration is never
verified), so it needs its own decision and none is made here; `local-scripted`
belongs to the fake adapter. Strict `subscription-only` with a config the
inventory accepts then blocks with `no_qualification_record`, because the
sign-in category cannot be established from non-secret status (D13); a config
it does not accept blocks earlier, before the version probe (see Consequences).
A run would also need an unexpired `purchased_credit:preventable` evidence
entry that no fixture can supply
(AC-4.3). Pinned by `TestCodexNonStrictBillingRefused`,
`TestCodexDecideOrder`,
`TestPurchasedCreditPreventableNeedsUnexpiredPreventableEvidence` and, through
the built binary, `TestCodexBillingModesAllRefuseOrBlock` (`tests/e2e/codex_qualification_test.go`).

**D10: one input writer, held by the worker.** The worker-owned stdin pipe
passed at `Start` is the only writer to the native. `Start` refuses a second
call on the same adapter value with `input_writer_conflict` before it
launches, and `Session` has no write or steer method (I18). The worker builds
a fresh adapter per start, so the guarantee across workers rests on the
adapter having no resume or reconnect path, not on this check. Pinned by
`TestSecondStartOnSameAdapterRefused` and `TestAdapterHasNoReplayPath`
(`adapters/codex/launch_test.go`).

**D12: no runtime billing-route check for Codex.** The decoder leaves
`SessionStarted.APIKeySource` empty and the worker skips its
`billing_route_mismatch` check for `builtin/codex`. Setting `"none"` would
assert a vendor fact nobody verified (I09), and leaving the value empty with
the check enabled would stop every attempt at startup. The route is
meant to be established at admission by the qualification key (AC-7.5), not at
runtime, but the key observes only the sign-in category today: `ObservedKey`
sets the provider endpoint, model snapshot, effort settings and entitlement
class to the literal `unknown` (`adapters/codex/qualify.go:69-71,76`), and the
registry compares those fields as plain values (`stableMatch` in
`internal/qualify/registry.go`), so nothing observes the endpoint and a record
keyed on `unknown` would match. The gate that holds the route closed is the
unknown sign-in category (D13, `internal/admission/admission.go:366`). Pinned by
`TestCodexSessionNotInterruptedForEmptyAPIKeySource` (the Claude cases in the
same table still stop on the route) and `TestCodexKeyVariationBlocks`.

**D18: a worker-initiated stop when an adapter loses the stream (added in
Task 9, not in the original design).** An adapter that can no longer observe
the native's stdout reports the `NativeError` class `protocol_mismatch`
(`adapter.ProtocolMismatch`, v2 §4.5). Only `codex.Decode` sends it: on a
malformed `result` frame or a read error, once, before `ProtocolCounters`. A
stream that merely ends without a result still ends as before. Without a
stop, the native would keep running with its output unobserved. A full pipe is
not the reason: after an early stop the Codex session drains stdout until the
process exits (`adapters/codex/launch.go:136-137`). Four rules follow:

- The worker stops the attempt through its own identity-checked interrupt
  (I18): `observe` sets `stopBy` to `worker` and the stop reason to
  `protocol_mismatch` when no stop is already running, and the journal gets
  `attempt.stop_requested` with `requested_by` `worker` and
  `attempt.native_error` with the class.
- A confirmed stop ends the attempt `stopped` with reason
  `protocol_mismatch`. `classify` gives a recorded stop only `stopped` (or
  `interrupted` when the stop is unconfirmed or descendants escaped), so the
  failure is carried by the reason, not a new state.
- The run then ends `failed` with reason `protocol_error`, exit 4, never
  `cancelled`: `conclude` maps an attempt stopped on `protocol_mismatch` to
  `RunFailed` when the run is not itself stopping. An unconfirmed stop ends
  the attempt `interrupted/stop_unconfirmed`, and the run `interrupted` after
  quarantine.
- A stop already in progress keeps its requester and reason. A user stop
  (`mythhelm stop`, the TUI, Ctrl-C) keeps requester `user` and an empty
  reason, so the run still ends `cancelled`; a route-violation stop keeps
  `billing_route_mismatch` and the run ends `blocked`.

Pinned by `TestDecodeEarlyStopReportsProtocolMismatch`
(`adapters/codex/decode_test.go`), `TestProtocolMismatchRequestsWorkerStop`
and `TestWorkerStopsCodexNativeAfterMalformedResult` (`internal/workers`),
and `TestStoppedProtocolMismatchFailsRun` (`internal/supervisor`). The Claude
adapter is unchanged: its `protocol_error` observation does not stop the
worker.

## Consequences

- Every `--adapter codex` run blocks or is refused today. `mythhelm doctor`
  shows the Codex row `planned` with fidelity, entitlement and lifecycle
  failing and the next test to run (`TestCodexDoctorRowEndToEnd`). Public
  limits are in `docs/limitations.md` (G10, I14).
- A strict Codex run ends `no_qualification_record` only when the config
  inventory accepts the user's and the project's native config. A
  `config.toml`, user or project, ends it `native_config_unreadable`
  (`adapters/codex/config.go:150-152`, `TestInventoryBlocksPresentTOML`); a
  credential key in a JSON config file ends it
  `native_settings_credential_override` (`config.go:179`); hooks or MCP servers end it `untrusted_native_config`
  unless the digest is trusted (`config.go:161`,
  `internal/admission/billing.go:145-167`,
  `TestCodexUntrustedConfigBlocksBeforeProbe`). All three come before the
  version probe (`internal/admission/admission.go:359-363`).
- A later spec can admit Codex only by supplying live evidence (a sign-in
  status source, the vendor frame schema and exhaustion shapes, the managed
  policy sources, and a purchased-credit prevention record from an authorised
  live run) and by changing code that blocks every run today:
  - `AuthCategory` (`adapters/codex/qualify.go:83`) always returns `unknown`,
    and `UnresolvedSources` (`adapters/codex/config.go:38`) blocks strict
    admission whenever the registry opens (`admission.go:374-378`). Both are
    code, as is `ObservedKey` (`qualify.go:61`).
  - `ObservedKey` hard-codes the provider endpoint and entitlement class to
    `unknown` (`qualify.go:69,76`). Admission must block an unknown endpoint
    or entitlement class, as it blocks an unknown sign-in category, before a
    record can match on them; `TestCodexStrictKeyLevel` still reaches
    `Eligible` with such a key.
  - The adapter reports `StructuredEvents` as `unknown`
    (`adapters/codex/compat.go:53`, `TestCapabilitiesUnknownNeverDefaultSupported`),
    and admission refuses a required capability that is not `supported`
    (`admission.go:265`).
  - The version baseline is the synthetic `0.0.0` (`compat.go:17`), so any
    real executable version is refused with `native_version_untested`
    (exit 7) unless `--allow-untested-native-version` is given
    (`TestCompatibilityUntestedBlocksWithoutOverride`).
  - D12 must be decided again: the worker skips the route check by adapter ID
    alone (`internal/workers/worker.go:812`) and the decoder reports no key
    source, so nothing observes the route between admission and launch. The
    pre-exec re-inventory (`internal/workers/contain.go:160,189`) guards
    config drift only.
- Adding a harness is one table entry plus its decider in `internal/admission`,
  and the table stays unexported, so extensions cannot register around
  admission. It is not the only change: Codex also needed the worker's adapter
  map, read by ID at `internal/workers/worker.go:68,269,602`, and adapter-ID
  branches at `internal/workers/contain.go:160,189,198`,
  `internal/workers/worker.go:812` and `internal/supervisor/pipeline.go:639`.
  An unlisted adapter ID takes the Claude path at the branches that default to
  Claude. A further per-adapter site is not an ID branch: the capability-error
  mapping names each adapter's sentinel (`internal/admission/billing.go:176`).
- Declaring a Codex billing posture needs a new decision that changes D3.
- Known gaps in the D18 stop seam, tracked in issue #389 and not decided
  here: the count of lines drained after the early stop reaches only the
  worker log and can miss buffered input (A3); `protocol_mismatch` can
  overwrite an earlier policy error class, which is unreachable while the
  Codex error table is empty and must be fixed before it is filled (A4);
  Ctrl-C or a deadline during the stop ladder ends the run `cancelled` or
  `blocked` rather than `failed`, where a route violation keeps its policy
  reason; and Codex `protocol_mismatch` and Claude `protocol_error` are not
  converged.
