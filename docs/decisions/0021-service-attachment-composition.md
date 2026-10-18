# 0021. Production singleton composition and scoped attachment control

- Status: proposed
- Date: 2026-10-15

## Context

ADR0014 accepts one per-user supervisor and ADR0016 preserves root-wide migration. At baseline61d18e2, the production registry serves short ledger intents while public CLI run/recover/apply own legacy pipelines. RegisterStop/RegisterRecover are deliberately unregistered because their isolated state writes do not ingest the pipeline spool. Herdr's refined FR-1/3/4 require an ordinary existing-run client, coherent scoped reads and prompt durable stop acknowledgment without changing ownership on attach.

## Decision

Complete the existing singleton's production composition in a package above control and supervisor. Route ordinary public run/demo modes through typed service admission, durable launch dispatch and one assignment-owned pipeline. Trusted standalone recover/apply/stop join that owner. Preserve root-wide Drain→Backup→Import→Adopt; no mixed per-run enrollment or concurrent legacy/service writer. Existing active legacy owners must release safely or finish before migration can adopt their verified worker identity. Attach never invokes migration/recovery/assignment.

Keep generation-fenced attachment grants separate from worker/attempt authority. Authenticate current scope before releasing any data, including durable operation replay. Select projection authority, generation and all safe evidence in one consistent guarded snapshot. Maintain complete run revisions across journal/control/evidence writes and monotonic sanitized projection revisions. Unknown is not empty/verified.

Accept stop by one short transaction containing scope, revision/generation, pinned attempt identity, immutable acknowledgment and outbox. Reconcile stop results separately; keep operations.result immutable. Deliver outside the transaction through the same run actor. The existing per-attempt first-wins stop file is idempotent; different accepted operation IDs coalesce explicitly without replacing it. Crash recovery reconciles request/worker evidence before another delivery. No unrelated stopped event, pane exit or native completion can certify stop/acceptance.

New service.adopted envelopes and payloads use schema_version2 through the existing canonical journal append path; legacy schema1 records remain separately decoded and byte-preserved. No blanket journal.EnvelopeVersion change is made.

Service restart reconciles immutable launch identity, process start and committed spool evidence before adopting assignment under a new instance generation. Worker launch generation/deadline remain pinned; a fresh presence heartbeat is not authorization for new execution. Worker completed-episode wait release is separate from authority-bearing requests, whose generation mismatches still fail. No file-only worker authority grant or implicit continuation is added.

Use a closed standalone admission schema and evidence-bound confirmations; do not serialize callbacks, raw admission.Decision or the full environment. The service revalidates native/config/task/repository facts before durable admission and constructs the qualified environment internally. Host capabilities never expose admission/confirm/apply/publication/native input. Standalone clients consume typed run.operation result unions and bounded run.observe cursor pages (maximum128events/1MiB/1second long poll); these operator events never flow into the64KiB host projection.

The public attach client uses an existing authenticated endpoint only; unlike ordinary run it never lazy-starts a service or induces startup recovery. Interactive attached Ctrl-C detaches the view without stop; plain/JSONL modes return one bounded snapshot and a view_closed result, never a claimed execution completion. The dedicated TUI model accepts safe typed projections only, and review navigation provides the bound trusted review view/command without automatic execution.

Optional hosts consume only sanitized projections and navigation requests. Their local transport is separately authenticated/restricted and exactly version-qualified. A 2-second deadline, 64 KiB frames and 16 projection slots bound optional work; stalled host I/O cannot block the core actor or acknowledgment. Host updates explicitly carry stale state and bounded reasons; a read-only authenticated resync callback can clear stale even when core revision is unchanged. Stable metadata renews its5second TTL every1second after fresh authenticated read, using an independent per-session publication sequence. Actual host TTL/source/sequence semantics require live qualification. Herdr initial candidate profile allows only ping and pane.report_metadata; unproven restore/callback methods remain unsupported with public manual attach available.

## Consequences

The implementation is larger than a registry fix: public production callers, owner extraction, migration restart, admission persistence, coherent reads and durable stop must agree. The three child specs listed in `specs/unrefined/herdr-embedded-client/plan.md` carry bounded domain-pure tasks and public binary tests. New additive schema preserves immutable legacy results and unknowns. Existing MH-23 worker deadline/client intent contracts are prerequisites for overlapping runtime changes; Windows support awaits its own process and transport evidence.

Acceptance tests must demonstrate ordinary run→attach with one worker, root conflict/held-owner refusal, live same-launch migration, restart without replacement, scope/generation read races, stale stop atomic rejection, lost acknowledgment/query, outbox crash/coalescing, honest evidence projection and host-stall responsiveness with violating fixtures. Live host results remain separate from fixtures. This does not complete MH-3/G11 or qualify a new billing/native route.

## Approval boundary

This record proposes implementation of the accepted singleton policy. Status remains proposed until actual architecture/maintainer review accepts the concrete contracts; no signed product approval is inferred. Recovered-only FR-1, per-run mixed-mode migration, new native authority or concurrent legacy/service mutation would be a different decision and are not authorized here.
