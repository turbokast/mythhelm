# Control service support matrix

One row per deliverable of the supervisor service (spec `supervisor-service`;
decision record [ADR 0014](../../docs/decisions/0014-service-topology.md)).
`support_test.go` reads this file: it must list exactly the shipped
deliverables and every control method the supervisor serves, and each row's
evidence must be tests that exist in this package, built for the platforms the
row claims.

Statuses are `fixture-tested` and `blocked`. Nothing here is `live-qualified`:
no row has run against a live multi-client deployment. `fixture-tested` means
the named tests run in the CI matrix (`.github/workflows/ci.yml`: Linux,
macOS and Windows runners) for the platforms in the row; this file does not
claim a passing run, which is CI's to show. Platforms are `all` (the test
files carry no build tag) or `linux, darwin` (they are tagged
`linux || darwin`).

| ID | Deliverable | Status | Platforms | Evidence |
|---|---|---|---|---|
| `instance-lock` | Per-user instance lock, adoption after a kill with a generation bump, refusal of a second root | fixture-tested | all | `TestSecondInstanceHeld`, `TestLockPathIndependentOfRoot`, `TestRootConflictRefused`, `TestStaleLockAdoptedWithGenerationBump` |
| `frame-codec` | Length-prefixed frame codec and NFR-1 ingress limits (1 MiB, depth 64, 128 references) | fixture-tested | all | `TestFrameAtExactly1MiBPasses`, `TestOversizeReportsSizeBeforeDecode`, `TestDepth64Passes65Fails`, `TestRefs128Pass129Fail`, `TestCheckIngressSurvivesAdversarialNesting`, `TestUnknownKeyRejected` |
| `transport-unix` | Unix socket transport with same-UID peer authentication and a 0700 socket directory | fixture-tested | linux, darwin | `TestUnixRoundTrip`, `TestSocketDirIs0700`, `TestForeignUIDRejected`, `TestNoTCPListener`, `TestDialWithoutSupervisor` |
| `transport-windows-pipe` | User-restricted Windows named-pipe transport | fixture-tested | windows | `TestPipeRoundTrip`, `TestPipeDACLCheckRejectsWorldPipe`, `TestPipeDialWithoutSupervisor`, `TestPipeListenRefusesSecondListener`, `TestPipePeerIsSameLogon`, `TestSpawnEnvCarriesWindowsLocators` |
| `execute-idempotency` | Idempotent `Execute` over the operations ledger (migration 0004): repeats replay, conflicting reuse fails | fixture-tested | all | `TestIdenticalRepeatReplays`, `TestConcurrentDuplicateExecutesOnce`, `TestReusedIDWithDifferentArgsConflicts`, `TestRecordedResultIsImmutable`, `TestMigration0004IsAdditive` |
| `sole-writer-mutate` | One-transaction `Mutate`; handler writes commit with the result or not at all; workers never open SQLite | fixture-tested | all | `TestMutateIsOneTransaction`, `TestHandlerWritesRollBackWithTheirFailure`, `TestWorkersNeverOpenSQLite` |
| `capability-tokens` | Attempt-scoped capability tokens, minted at admission and checked per request | fixture-tested | all | `TestTokenMintedAtAdmissionCheckedPerRequest`, `TestTokenScopedToAttempt` |
| `lazy-start` | Lazy detached supervisor start, one supervisor per user, `mythhelm supervisor status`, refusal of a supervisor serving another root | fixture-tested | linux, darwin | `TestLazySpawnThenStatus`, `TestConcurrentSpawnBecomesClient`, `TestStatusJsonlCarriesInstanceState`, `TestSupervisorSurvivesClientExit`, `TestForeignRootSupervisorRefused`, `TestRootsRacingOneWinnerNeverBothAttach` |
| `method:status` | `status` intent: pid, generation, root and endpoint | fixture-tested | all | `TestStatusIntentReturnsInstanceState` |
| `method:assign` | `assign` intent: run ownership recorded under the supervisor | fixture-tested | all | `TestAssignIntentRecordsOwnership` |
| `method:reserve` | `reserve` intent: reservations keyed by host, bucket and scope, bounds prevent work, unknown is not zero (migration 0005) | fixture-tested | all | `TestReserveIntentViaExecute`, `TestBoundsFailurePreventsWork`, `TestReservationKeyedByHostAndResource`, `TestQuantityUnknownNeverZero`, `TestMigration0005KeysReservationsByHost` |
| `method:release` | `release` intent: release with recorded evidence | fixture-tested | all | `TestReleaseIntentViaExecute` |
| `method:heartbeat` | `heartbeat` intent: refresh a held reservation | fixture-tested | all | `TestHeartbeatIntentViaExecute` |
| `method:read` | `read` intent: authority filtered before rows are counted; token callers bounded to their repository | fixture-tested | all | `TestReadIntentFiltersForeignRepos`, `TestFilterHidesForeignRepos`, `TestReadIntentTokenBindsRepoEntitlement` |

## Not claimed

- Windows: the instance lock is built and its tests are untagged, so the CI
  Windows runners build them. The named-pipe transport is fixture-tested
  (row `transport-windows-pipe`); the untagged method tests also run on the
  Windows leg. There is still no Windows lazy start or detach evidence.
- No row is `live-qualified`.
- A per-repository entitlement store does not exist (`read` scopes an
  operator by the repositories it presents); see the task 6 hand-off.
