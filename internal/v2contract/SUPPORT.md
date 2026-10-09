# v2 contract vocabulary: support matrix

Each row is one deliverable of `internal/v2contract` and the state of its evidence.

States: `documented`, `fixture-tested` (a test in this package pins it; the tests are named), `blocked` (unresolved; not guessed). Nothing here has run against a live supervisor, so no row claims the strongest state.

`TestSupportMatrixMatchesEvidence` parses this table: every shipped deliverable needs a row, every named test must exist, and no row may claim more than fixtures support.

| Deliverable | State | Evidence |
|---|---|---|
| run | fixture-tested | TestExecutionGoldensRoundTrip, TestMissingIDsRejected |
| task_revision | fixture-tested | TestExecutionGoldensRoundTrip, TestTaskRevisionRejectsZeroRevision, TestRevisionDigestsDiffer |
| attempt | fixture-tested | TestExecutionGoldensRoundTrip, TestAttemptRejectsOmittedTaskRevision |
| routing_decision | fixture-tested | TestEvidenceGoldensRoundTrip, TestRoutingDecisionRejectsNonFiniteFloats |
| design_decision | fixture-tested | TestEvidenceGoldensRoundTrip, TestDesignDecisionRevisionOrder |
| artifact | fixture-tested | TestEvidenceGoldensRoundTrip, TestArtifactSizeBytesPresence, TestNullMeasurementsRejected |
| observation | fixture-tested | TestEvidenceGoldensRoundTrip, TestOutOfScaleEnumsRejected |
| verification | fixture-tested | TestEvidenceGoldensRoundTrip, TestEvidenceRequiredIdentity |
| context_manifest | fixture-tested | TestCoordinationGoldensRoundTrip, TestAliasesDecodeIdentically |
| message | fixture-tested | TestCoordinationGoldensRoundTrip, TestOpenStringsValidatedNonEmpty |
| grant | fixture-tested | TestCoordinationGoldensRoundTrip, TestGrantUseEnumClosed, TestUnknownAuthorityKeyRejected |
| reservation | fixture-tested | TestCoordinationGoldensRoundTrip, TestReservationUnknownQuantity, TestNullMeasurementsRejected |
| policy_version | fixture-tested | TestCoordinationGoldensRoundTrip, TestTOMLTagsRoundTrip |
| experiment | fixture-tested | TestCoordinationGoldensRoundTrip, TestExperimentRejectsNonFiniteSplits |
| run_lifecycle | fixture-tested | TestRunTableMatchesSpec, TestBlockedResumeRequiresSaved |
| task_lifecycle | fixture-tested | TestTaskTableMatchesSpec |
| attempt_lifecycle | fixture-tested | TestAttemptTableMatchesSpec, TestReconcileRequiresSameLaunch |
| error_catalogue | fixture-tested | TestCatalogueHas24Codes, TestDefaultDispositions, TestErrorGoldenRoundTrip |
| event_envelope | fixture-tested | TestEnvelopeGoldenRoundTrip, TestCheckSequence, TestCheckDuplicateAndGeneration |
| frame_limits | fixture-tested | TestFrameLimitsExact |
| json_schema_artefacts | blocked | Generated JSON Schema files are a follow-up (non-goal N4); the Go types are the contract today. |
| native_auth_and_os_facts | blocked | Provider authentication and per-OS facts are unresolved here and are not guessed; later specs record them from observation. |
