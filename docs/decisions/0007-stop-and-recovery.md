# 0007. Stop and recovery ownership

- Status: proposed
- Date: 2026-09-30

## Context

The dogfood slice has a detached worker and a single SQLite writer for each run. A CLI crash releases the OS owner lock while the native process may continue writing. Recovery must neither launch a replacement writer nor signal a reused PID. A stop request is distinct from a confirmed stop (I06, I12, design §3).

## Decision

`stop` reads the journal without taking ownership away from a live supervisor. It verifies the worker's launch-token digest, run and attempt identity, any journaled PID/start time and the current PID/start time before writing an atomic stop request. The worker owns signalling and journals acknowledgement. A confirmed process-group stop with unresolved descendants or a failed descendant scan does not establish complete ownership resolution.

`recover` acquires the run owner lock and re-reads its projection after acquisition. It verifies worker identity before ingesting its spool, then either reattaches, continues freeze and verification after a recorded terminal native event, or quarantines a lost worker without a replacement native launch. A fresh producer ingests from the committed spool offset. Candidate rows prevent duplicate freeze. Recovery reconstructs check commands from the admitted Git blob and recorded configuration digest, and uses the captured Git identity; candidate edits and current source configuration do not replace those decisions.

An unfinished verification whose result was not durably recorded remains ownership unresolved. Recovery never repeats arbitrary check effects while the previous check's process may still be alive. A durably recorded verification result can finish its state transition. Version 1 terminal receipts can be rebuilt from committed projections after a failed write or missing digest publication.

The worker records PID and start time for known escaped descendants. Recovery only observes those identities and the Linux attempt-marker scan; it never signals them. A partial candidate may be frozen once the confirmed native group and all known descendants are gone. That run ends `failed/recovered_partial` (exit 4), preserving the quarantined attempt and never labelling it verified success. Legacy records without enough identity evidence stay unresolved. Linux zombie entries count as gone: they cannot execute or write even if PID 1 has not reaped them.

## Consequences

- Native execution is never relaunched by recovery; the fake's append-only launch counter tests this through repeated recovery.
- A missing or forged worker identity fails closed. Linux lists readable marker matches; other platforms print the marker for manual inspection. An empty scan cannot prove safety after an unacknowledged native launch.
- Check environment values are read from the recovering process through the same allowlist and the admitted passthrough names; credentials and raw native launch configuration are not reconstructed from durable state.
- Recovery input reads are bounded. Malformed-stream acceptance measures the fake decoder's heap use as well as exercising complete CLI/worker runs.
- A failed first journal append is an admission refusal (exit 3), with no worker launched. An owner lock held by a live process is refused (exit 6).
