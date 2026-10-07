# Claude Code adapter compatibility

> The qualification registry (`internal/qualify`) is the queryable record;
> this file is the per-harness view for Claude Code. Fixture facts:
> surface="native-cli-structured (print, stream-json)" native="2.1.284"
> progress="fixture-tested" (synthetic until MH-12).

One record per harness × surface × native version × OS × entitlement class ×
security profile (master spec §9.14). Status values are `planned`,
`documented-candidate`, `fixture-tested`, `live-qualified`, `experimental`,
`blocked` and `unsupported`. Billing qualification is recorded independently
from feature qualification: working technically never implies strict
subscription-only eligibility.

## claudecode × print/stream-json × 2.1.284 × linux × any × trusted-host

- **Status**: `fixture-tested` (synthetic doc-derived fixtures only; no live run).
- **Upstream evidence/date**: synthetic fixtures in `testdata/streams/` (Task 17).
- **Executable identity**: `claude` 2.1.284.
- **Fidelity differences**: print mode with `--output-format stream-json`,
  `--verbose`, `--input-format text`, `--permission-mode acceptEdits`,
  `--permission-prompts none`; prompt on stdin; no interactive permission flow.
- **Required approvals**: native-config trust grant, project-config trust,
  entitlement declaration where billed.
- **Auth/entitlement route**: unqualified (no live observation).
- **Overage prevention evidence**: none.
- **Quota-observation scope**: native-reported usage in result frames.
- **Child/auxiliary behaviour**: unobserved live.
- **Compatibility fixtures**: `success`, `error_max_turns`,
  `permission_denials`, `startup_failure`, `api_retry`, `hooks_before_init`.
- **Live-canary evidence**: none.
- **Herdr attachment status**: standalone (Herdr out of scope).
- **Unresolved limitations**: everything below was first observed on 2.1.285.

## claudecode × print/stream-json × 2.1.285 × linux × subscription-declared × trusted-host

- **Status**: `fixture-tested` (NOT `live-qualified`: G02 partial, no
  direct-native comparison run).
- **Upstream evidence/date**: sanitised rate-limit recording
  `testdata/streams/recorded-2.1.285.jsonl` (Task 20, 2026-10-02); live canary
  pass 2026-10-02; full dogfood run on issue #60, exit 0, 2026-10-02
  (docs/dogfood/0001-first-run.md).
- **Executable identity**: `claude` 2.1.285 (per-run sha256 in the receipt).
- **Fidelity differences**: as 2.1.284, plus observed live: SessionStart hooks
  execute in print mode; `commands_changed` and hook frames arrive before
  `init`; result frames carry BOTH aggregate `usage` and per-model
  `modelUsage` (the decoder reads them separately since PR #59); one native
  `Bash` attempt was denied by `--allowedTools` scoping and worked around.
- **Required approvals**: native-config trust grant (17 hooks, 7 MCP servers
  inventoried), project-config digest trust, `plan=max,extra-usage=disabled`
  declaration, `--accept-flags` for `ignored_outputs` (5 git-ignored build
  artifacts, not transferred).
- **Auth/entitlement route**: `subscription-declared`, user-declared, G05
  not passed; `init_api_key_source: none`.
- **Overage prevention evidence**: none (`paid_continuation: unknown`).
- **Quota-observation scope**: native-reported usage
  (input/output/cache_read/cache_creation per model) and a retail-equivalent
  estimate marked estimate-not-charge; no independent metering.
- **Child/auxiliary behaviour**: worker detached (own session), native in its
  own process group; no unresolved descendants; MCP servers present but unused
  by the task; native stderr empty.
- **Compatibility fixtures**: `recorded-2.1.285.jsonl`, pinned by
  `TestDecodeFixture/recorded`.
- **Live-canary evidence**: `TestLiveClaudeCanary` PASS 2026-10-02 (6.8s,
  exit 0) on 2.1.285/linux.
- **Herdr attachment status**: standalone (Herdr out of scope).
- **Unresolved limitations**: no success-stream recording yet (the recording
  is a rate-limit encounter; a success stream is future work); the 1h
  subscription / 5m API TTL split is reported, not pinned; Windows refused by
  design (exit 7); Q6 open (single observation: only local hook/system frames
  preceded `init`, zero inference tokens); native transcripts persist outside
  MYTHHELM; `auth status` treated as possibly networked.
