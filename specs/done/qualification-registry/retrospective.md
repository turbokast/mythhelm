# qualification-registry — Retrospective

## Review Summary

- **Range**: b98d53e..c7a86fe (PRs #200, #201, #202, #203, #204, #205, #206, #207, #208; fix PRs none)
- **Reviewer**: code-reviewer, architect
- **Findings**: critical 0, important 3, suggestion 7 (confirmed); rejected 3
- **Open critical**: 0
- **Vendor review**: skipped (codex, muse, jev all disabled)

| Severity | Finding | Anchor | Disposition |
|---|---|---|---|
| important | Seed rows never consult-match (unknown stable dims); MH-12 live keys will coexist, not supersede | `internal/qualify/seed.go:88-125` vs `internal/qualify/registry.go:298-306` | proposal P1 (MH-12 consult/write preconditions) |
| important | AC-4.2 trust-before-consult ordering holds by construction but no test pins it | `internal/admission/admission.go:293` vs `:312` | proposal P2 (ordering test) |
| important | User-guide qualification example values differ from real seed output | `docs/user-guide.md:59` | fixed in finalize PR |
| suggestion | Consult ignores evidence Label/Method and Record.Quota | `internal/admission/qualify.go` | noted in P1 (latent; no live records in spec) |
| suggestion | InvalidateOnDrift preserves NextTest across invalidation | `internal/qualify/registry.go:320-338` | noted in P1 (latent; no production caller) |
| suggestion | knownHarnesses duplicates seedHarnesses with no agreement pin | `internal/admission/qualify.go:47-48` | kept; comment-linked, both sides individually tested |
| suggestion | Record trusts caller progress labels (no write-time coherence gate) | `internal/qualify/registry.go:Record` | noted in P1 (MH-12 live-writer path) |
| suggestion | Drift sentences flow into BlockedError.Code (typed-code field) | `internal/admission/admission.go:327,331` | kept; exit mapping correct, MH-12 note |
| suggestion | NFR-1 latency test pins Lookup, not the Consult path | `internal/qualify/registry_test.go:650` | noted in P1 (MH-12 pins Consult) |
| suggestion | Capability-value rejection has no field-naming test (Progress only) | `internal/qualify/qualify_test.go:306` | kept |
| rejected | COMPATIBILITY.md key-tuple sentence contradicts the 14-field Key | `adapters/claudecode/COMPATIBILITY.md:8-9` | pre-existing view-granularity sentence (range diff adds only the header); header correctly anchors the registry; no AC or design statement contradicted |
| rejected | COMPATIBILITY.md cites a stale R1.1 section number | `adapters/claudecode/COMPATIBILITY.md:9` | v2 has §9.14 "Qualification registry and honest coverage" (`docs/spec/master-spec.md:966`) |
| rejected | Cross-claim that the guide example matches seed output exactly | `docs/user-guide.md:59` | false: seed Surface and all columns are unknown (`internal/qualify/seed.go`); arch-F1 confirmed instead |

Foreign changes in range: none.

## Acceptance

| Criterion | Result | Evidence |
|---|---|---|
| AC-1.1 full §7.1 key | met | 14-field Key; TestKeyHashDeterministic; observedKey field mapping (PR #200, #205) |
| AC-1.2 column independence | met | TestColumnIndependence; per-column verdicts (PR #200) |
| AC-1.3 immutable revisions | met | TestRecordRevisionImmutability; supersede-only revisions (PR #201) |
| AC-1.4 never imply qualified | met | Seeds blocked/planned only; strict requires live-qualified + proven + stop marker (PR #201, #205) |
| AC-2.1 seven harnesses | met | SeedV1; TestSeedHasSevenHarnesses; e2e seven-record assertions (PR #201, #206) |
| AC-2.2 scales + support | met | Progress scale pinned; Capability{Value,Evidence,Scope,Expiry} (PR #200) |
| AC-2.3 fixture ≠ live | met | Strict requires ProgressLiveQualified; TestNonLiveRecordBlocksStrict (PR #205) |
| AC-3.1 first-route proof | met | RecordDraft + registry round-trip under fixtures-only default; live strict pass out of scope per N1, MH-12 owns it (PR #203) |
| AC-3.2 sign-in/declaration never qualifies | met | Strict blocks missing/non-live; AuthEvidence never maps to proven; e2e exit-3 legs (PR #205, #206) |
| AC-3.3 unknown quota + stop marker | met | TestDraftQuotaLabelledUnknown; TestUnknownQuotaAdmitsStopAtExhaustion (PR #205) |
| AC-4.1 drift invalidates | met | CheckDrift + drift→Blocked strict path; InvalidateOnDrift ships, persistence per AC text waits for MH-12 (PR #202, #205) |
| AC-4.2 trust re-evaluates first | met | Ordering holds by construction (admitNativeConfig before consult); no pinning test (P-qualification-registry-2) |
| AC-4.3 NextTest + authority | met | All seeds carry next test + authority (PR #201) |
| AC-5.1 versioned consult | met | ResolveQualification single consult after AuthStatus, before billing (PR #205) |
| AC-5.2 labeled data | met | Datum/Evidence labels; unknown defaults; honest unavailable reads (PR #200, #204) |
| AC-5.3 doctor section | met | Plain + JSONL with progress/columns/evidence/drift; e2e packaged-binary pins (PR #204, #206) |
| NFR-1 <1s reads | met | TestRegistryLookupLatency pins cold Lookup under 1s |
| NFR-2 content-addressed | met | CanonicalDigest; TestDigestRecomputes; oracle-verified goldens (PR #200, #201) |

## Deviations

- Task 1: None.
- Task 2: extra revisions test file (import-cycle probe) + extra seed-failure test — recorded in the task entry; no production impact.
- Task 3: None.
- Task 4: OpenQualificationRegistry export, two refusal codes, declared-admits-drift/ambiguous reading, unreachable harness guard + ErrInvalid, ErrAmbiguousMatch sentinel, non-dir OpenReadOnly refusal (review round 1) — recorded in the task entry; the sentinel fixes a real fail-open.
- Task 5: weakened transitive-deps static leg (direct imports + no net/http, behavioural leg kept) — recorded in the task entry; prescribed check unsatisfiable.
- Task 6: extra empty-read test — recorded in the task entry; pins design §6 rule.
- Task 7: None.
- Task 8: None.
- Task 9: None.
- Review-found: seed keys never consult-match (Task 2 platform-filled choice unrecorded) — P-qualification-registry-1; design §6 dual plain formats (shipped long form per Task 6 acceptance); design §4 duplicated Record signature (editorial); user-guide example values wrong — fixed in finalize PR.

## CI history

- CI/golangci-lint on PR #202 @6395dcf: real, UK-spelling lint failure in the review-fix commit; fixed by 49dc6dc.
- CI/Go (windows-11-arm, windows-latest) on PR #205 @7a84421: real, 3 test failures — probe-dependent legs on a platform the probe refuses by design, plus OpenReadOnly misreading through-file stat ENOENT as missing; fixed by review-round code changes at dfc0866.
- Cancelled runs across task branches: superseded pushes on re-push, routine, not failures.
- No workflow failed and succeeded on the same head SHA: no nondeterministic failures.

## Effort

dispatched=10 returned=9 failed=1; attempts 10 over 9 tasks; first-pass 8/9; review rounds 8; wall-clock 2026-10-07T12:37:16Z → 2026-10-08T07:41:25Z

| Task | Agent | Attempts | Review rounds | PR | Merged | First pass |
|---|---|---|---|---|---|---|
| 1 | go-implementer | 2 | 1 | #200 | yes | no |
| 2 | go-implementer | 1 | 0 | #201 | yes | yes |
| 3 | go-implementer | 1 | 1 | #202 | yes | yes |
| 4 | go-implementer | 1 | 2 | #205 | yes | yes |
| 5 | go-implementer | 1 | 1 | #203 | yes | yes |
| 6 | go-implementer | 1 | 1 | #204 | yes | yes |
| 7 | go-implementer | 1 | 1 | #206 | yes | yes |
| 8 | go-implementer | 1 | 0 | #207 | yes | yes |
| 9 | go-implementer | 1 | 1 | #208 | yes | yes |

Retries: Task 1 attempt 1 lost (worker never reported; no defect signal), attempt 2 clean.
Review rounds: 8 total; most common class vendor-thread findings (test-strengthening, plus 1 real fail-open fixed with a pinning test).

## Lessons

- What worked: test-first with red-verified legs — the corrupt-row and tampered-digest tests caught a real fail-open (PR #205 review round 1). Investigate-before-fix on the Windows CI failures found root causes (ENOENT mapping, probe gate) instead of skips (PR #205). Oracle-checked digest goldens via sha256sum (PR #200). Per-task review rounds kept main green: only 2 real pre-merge CI failures in 9 PRs.
- What to change: seed key contents were unspecified, leaving the seed/consult seam untested (P-qualification-registry-1). AC-4.2 ordering holds by construction but has no pinning test (P-qualification-registry-2). Docs examples with command output were eyeballed, and one shipped wrong (fixed in finalize PR; P-qualification-registry-3). Review-round resume landed on tool-less workers twice; fresh workers with the verbatim reasons succeeded both times.

## Proposals

- P-qualification-registry-1 — MH-12 consult/write preconditions from the seed/consult gap
- P-qualification-registry-2 — Pin the AC-4.2 trust-before-consult ordering with a test
- P-qualification-registry-3 — Ground docs command-output examples in executed commands
