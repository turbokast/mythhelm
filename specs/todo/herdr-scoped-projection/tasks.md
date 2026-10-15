# Herdr scoped projection — Tasks

### Dependencies

- Epic: `specs/*/herdr-embedded-client/plan.md`. Both `specs/*/supervisor-attachment-control/` and `specs/*/existing-run-client/` must be merged before implementation consumes their contracts. Task1 creates the new hosts/herdr subtree (adapters domain); Task6 is core-only and Task7 is core test-only. Tasks3 and4 can proceed in parallel after their dependencies; Task5 does not edit Task6 files. Shared docs serialize after prerequisite docs.
- Tasks sharing a Files path serialize. No task may consume proposed/unmerged interfaces; re-ground exact dependency contracts before starting. All tasks require independent code review; persistence/process/public interfaces also require architecture review.
- **Gates for every task**: format changed Go with gofmt first (go mod tidy when needed), then `scripts/harness/gate.sh go` for Go tasks, `scripts/harness/gate.sh harness` for harness changes and `scripts/ci/check-public-hygiene.sh` for all changes; spec bookkeeping also runs `scripts/ci/lint-agent-harness.sh --only specs`. Docs tasks run public hygiene and applicable harness lint. Files existing or an agent reporting success is not completion.
- **Completion convention**: append ✅ COMPLETED to the task heading only after merged evidence; add Status (PR), Implementation (SHAs), Spec deviations and Files modified; retain original fields and update scratchpad discoveries.
- **Commits**: signed off with named paths; each implementation task uses an isolated worktree and PR. Decision records are reviewed before dependent implementation; never invent a maintainer/product signature.

## Implementation Tasks

### Task 1 — Define the qualified host subset and fixtures

- **Domain/agent**: go-implementer
- **Budget**: complex
- **Depends on**: None
- **Change**: Create hosts/herdr and synthetic testdata with a closed protocol profile from the installed schema subset; distinguish installed shape from unknown live transport evidence.
- **Files**:
  - `hosts/herdr/schema.go`
  - `hosts/herdr/schema_test.go`
  - `hosts/herdr/testdata/profile22.json`
  - `hosts/herdr/testdata/profile22_cases.json`
- **Produces**: Options, Compatibility, Action, Update, Publication and exported error sentinels in design §2; method/profile decoder. Task creates hosts/herdr/testdata.
- **Acceptance**:
  - TestHerdrSchemaSubset accepts only the exact ping/pane.report_metadata subset and rejects unknown/obsolete schema/protocol, duplicate fields and malformed shapes.
  - TestBundledSchemaNotLiveCompatible cannot set Metadata=true from schema alone; fixture asserting compatibility without live pong fails.
  - Profile manifest cites the installed discovery command, subset digest and exact expected framing/response proof; missing proof stays unqualified, not guessed.
- **Test plan**: Synthetic fixtures and deterministic barriers for the named cases; pair negative assertions with a deliberately violating fixture. Public process behavior uses the packaged binary in the integration task, not helper-only evidence.
- **Invariants touched**: I03/I11/I14 (v2 §14: negotiated explicit subset).

### Task 2 — Probe restricted local host compatibility

- **Domain/agent**: go-implementer
- **Budget**: complex
- **Depends on**: Task 1
- **Change**: Implement bounded installed discovery and same-user local host probe with the exact pong response; unsupported trust disables integration.
- **Files**:
  - `hosts/herdr/probe.go`
  - `hosts/herdr/probe_test.go`
  - `hosts/herdr/transport_unix.go`
  - `hosts/herdr/transport_other.go`
  - `hosts/herdr/transport_test.go`
- **Produces**: Probe(ctx context.Context, options Options) (Compatibility, error), design §§2–3.
- **Acceptance**:
  - TestHerdrProbeTrust refuses foreign peer, writable parent, changed endpoint identity, denied/missing socket and unqualified platform; no TCP listener/credential access occurs.
  - TestHerdrProbeDeadline times out by2seconds on stalled or oversized discovery/host responses and returns a fixed reason.
  - TestHerdrLivePongCorrelation requires matched request ID, exact pong/version/protocol and profile; malformed/unmatched/obsolete live responses keep Metadata=false.
- **Test plan**: Synthetic fixtures and deterministic barriers for the named cases; pair negative assertions with a deliberately violating fixture. Public process behavior uses the packaged binary in the integration task, not helper-only evidence.
- **Invariants touched**: I03/I11/I13/I14/I19 (v2 §§8/14: local trust and no credential copy).

### Task 3 — Map safe projection to bounded metadata

- **Domain/agent**: go-implementer
- **Budget**: complex
- **Depends on**: Task 2
- **Change**: Encode only the closed display-token allowlist and preserve freshness/revision/evidence distinctions; never accept raw native payload.
- **Files**:
  - `hosts/herdr/metadata.go`
  - `hosts/herdr/metadata_test.go`
  - `hosts/herdr/testdata/metadata_synthetic.json`
- **Produces**: Report(ctx context.Context, options Options, compatibility Compatibility, publication Publication) error, design §4.
- **Acceptance**:
  - TestHerdrMetadataAllowlist checks every emitted key/value against the fixed set, no more than16keys and64KiB; nested sentinel/token/path/native input never reaches transport.
  - TestHerdrProjectionOrdering covers2→1, identical2, conflicting2 and old generation after resync; host acknowledgments cannot change candidate/verified/applied.
  - TestHerdrPublicationSequence separates core revision from monotonically increasing per-source publication seq, including same-content renewal, stale/fresh transitions, new-session seq1 and late old-session response rejection.
  - TestHerdrMetadataAck requires the exact qualified method response and matched id; unexpected shape disables metadata rather than silently succeeding.
- **Test plan**: Synthetic fixtures and deterministic barriers for the named cases; pair negative assertions with a deliberately violating fixture. Public process behavior uses the packaged binary in the integration task, not helper-only evidence.
- **Invariants touched**: I03/I07/I09/I17/I19 (v2 §§8/14: sanitized canonical evidence only).

### Task 4 — Confine actions to trusted navigation

- **Domain/agent**: go-implementer
- **Budget**: complex
- **Depends on**: Task 1
- **Change**: Implement the bounded parser/navigation adapter and refusal paths; production callbacks remain unavailable until an exact transport profile is qualified.
- **Files**:
  - `hosts/herdr/actions.go`
  - `hosts/herdr/actions_test.go`
  - `hosts/herdr/restore.go`
  - `hosts/herdr/restore_test.go`
- **Produces**: ParseAction(raw []byte) (Action, error); Action kinds review/stop_confirmation/writable_unsupported, design §5.
- **Acceptance**:
  - TestHerdrActionsNoAuthority rejects run/repository overrides, approval bits, commands, native input and oversized/duplicate keys; navigation cannot submit stop.
  - TestHerdrWritableTakeoverZeroInput maps refusal visibly and records zero native input bytes; a fixture passing through input fails.
  - TestHerdrRestoreManualOnly refuses unsupported registration/agent resume and exposes the existing public attach fallback; no guessed host API is sent.
- **Test plan**: Synthetic fixtures and deterministic barriers for the named cases; pair negative assertions with a deliberately violating fixture. Public process behavior uses the packaged binary in the integration task, not helper-only evidence.
- **Invariants touched**: I03/I06/I11/I17/I18 (v2 §14: host navigation grants no execution authority).

### Task 5 — Bound host work and prove backpressure isolation

- **Domain/agent**: go-implementer
- **Budget**: complex
- **Depends on**: Task 3, Task 4
- **Change**: Implement adapter lifetime/queue/coalescing and cancellation without blocking core producers or controls.
- **Files**:
  - `hosts/herdr/bridge.go`
  - `hosts/herdr/bridge_test.go`
  - `hosts/herdr/backpressure_test.go`
- **Produces**: Run(ctx context.Context, options Options, updates <-chan Update, resync func(context.Context) (Update, error), navigate func(Action) error) error, design §§2/6.
- **Acceptance**:
  - TestHerdrHostStallIsolation stalls transport10seconds and asserts separate authenticated core status/stop-ack under2seconds with at most16pending projections.
  - TestHerdrOverflowMarksStale coalesces to latest, never loses correlated acknowledgment, latches stale, and clears only after a successful authenticated resync—even when its returned projection revision/content is identical. A duplicate cached update cannot clear stale.
  - TestHerdrBlockingFixtureDetected runs deliberately blocking consumer through the same assertion and verifies it detects the bound violation.
  - TestHerdrCoreDisconnectFreshness supplies a core disconnect with unchanged projection, observes stale metadata and canceled fresh renewal, then successful same-revision authenticated resync restores fresh; missing freshness propagation deliberately fails.
  - TestHerdrStableRenewalVisibility uses a fake monotonic clock across three TTL periods: unchanged core revision stays visible through authenticated reads and increasing publication seq; disconnect/failing resync ceases fresh renewal and expires by5seconds. A projection-revision-as-seq/no-op renewal variant fails.
  - TestHerdrCancelOnlyPresentation closes adapter/pane and observes unchanged worker launch/stop/input counters and continuing service reconciliation.
- **Test plan**: Synthetic fixtures and deterministic barriers for the named cases; pair negative assertions with a deliberately violating fixture. Public process behavior uses the packaged binary in the integration task, not helper-only evidence.
- **Invariants touched**: I06/I09/I13/I17/I21 (v2 §§3/14: optional host cannot impede core).

### Task 6 — Compose optional host integration into attach

- **Domain/agent**: go-implementer
- **Budget**: complex
- **Depends on**: Task 3, Task 4, Task 5
- **Change**: Add attach-only flags and map optional-host failures to visible safe fallback while leaving core attachment and trusted controls usable.
- **Files**:
  - `internal/cli/attach_host.go`
  - `internal/cli/attach_host_test.go`
  - `internal/cli/attach.go`
  - `internal/cli/attach_test.go`
- **Produces**: attach --host=herdr --herdr-socket --herdr-pane; default standalone. Core client/controller passes only Update, a bounded read-only Refresh resync closure and navigation callback, with a separate Controller.Subscribe host cursor.
- **Acceptance**:
  - TestAttachHerdrFallback invokes cli.Main with missing/denied/incompatible host and sees successful existing-run client plus fixed degraded reason, no launch/stop/owner change.
  - TestAttachHerdrFlagScope rejects invalid combinations and proves ordinary run --host=herdr remains unsupported.
  - TestAttachHostSafeFanout gives the TUI and host independent bounded subscription cursors; both receive stale/reason and fresh identical-state recovery, while host closure cannot call mutation or obtain a grant.
  - TestAttachHostCannotConfirmStop sends navigation then observes no stop until explicit trusted user confirmation; forged host confirmation is inert.
- **Test plan**: Synthetic fixtures and deterministic barriers for the named cases; pair negative assertions with a deliberately violating fixture. Public process behavior uses the packaged binary in the integration task, not helper-only evidence.
- **Invariants touched**: I03/I06/I09/I17/I18 (v2 §§14/15: optional host cannot own the run).

### Task 7 — Qualify public embedded lifecycle with fixture and live suites

- **Domain/agent**: go-implementer
- **Budget**: complex
- **Depends on**: Task 5, Task 6
- **Change**: Run packaged attach against ordinary service-owned fake work; add a separately selected live suite with exact host/platform/capability evidence.
- **Files**:
  - `tests/e2e/herdr_attach_test.go`
  - `tests/e2e/herdr_live_test.go`
  - `tests/e2e/herdr_fixture_test.go`
- **Produces**: Synthetic integration tests and live-tagged TestLiveHerdrLifecycle; sanitized evidence records for docs Task8.
- **Acceptance**:
  - TestE2EHerdrLifecycle invokes public run then public attach for initial/repeated/manual restore; socket disconnect, pane-process teardown, reconnect and duplicate restore leave one attempt/launch and zero presentation stop/input.
  - TestE2EHerdrHostFailures covers missing socket/obsolete schema/permission denial and manual fallback while core stop remains durably queryable.
  - TestLiveHerdrLifecycle records exact installed/live version/protocol/OS/arch/native-fake surface and detach/reconnect/pane teardown/actual host restart/duplicate manual restore/missing socket/obsolete schema; skip/failed rows are never passed.
  - TestLiveHerdrMetadataRenewal measures unchanged content visibility across three TTL periods, fresh resync with unchanged mh_revision, monotonic per-source seq, new source seq1, retired-source precedence and expiry after disconnect; absent proof disables the metadata profile rather than assuming bundled-schema semantics.
  - No live run touches unrelated panes or native accounts; a counter-violating fixture proves every forbidden effect assertion can fail.
- **Test plan**: Synthetic fixtures and deterministic barriers for the named cases; pair negative assertions with a deliberately violating fixture. Public process behavior uses the packaged binary in the integration task, not helper-only evidence.
- **Invariants touched**: I06/I13/I14/I17/I18 (v2 §§14/18: public/live evidence with one owner).

### Task 8 — Publish exact capability and evidence documentation

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 7
- **External ordering**: Wait for `specs/*/docs-site-restyle/` Task11 and `specs/*/tui-observatory-view/` Task19 merge and prerequisite core/client docs, then re-ground user-guide label/example conventions.
- **Change**: Document optional metadata, manual attach, unavailable callbacks/automatic restore and exact fixture/live lifecycle matrix with executed examples.
- **Files**:
  - `docs/hosts/herdr.md`
  - `docs/hosts/herdr-evidence.md`
  - `docs/user-guide.md`
  - `docs/limitations.md`
- **Produces**: Versioned evidence matrix and operator guide; creates docs/hosts under docs with no live private paths/payloads.
- **Acceptance**:
  - Every supported matrix cell links a sanitized Task7 evidence result for exact CLI/live/protocol/OS/arch/surface/event; failed/untested/unsupported cells remain explicit.
  - Examples are generated from public Task7 invocations, include manual attach and accepted/unconfirmed/confirmed distinctions, and name incomplete MH-3/G11/AT-36/AT-48 tracks.
  - Documentation explicitly excludes host-side apply/approval/publication/launch, native writable takeover and billing qualification inheritance.
- **Test plan**: Synthetic fixtures and deterministic barriers for the named cases; pair negative assertions with a deliberately violating fixture. Public process behavior uses the packaged binary in the integration task, not helper-only evidence.
- **Invariants touched**: I09/I14/I17/I19 (v2 §§14/15/18: honest scoped support).
