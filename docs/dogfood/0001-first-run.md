# 0001 — First live run (Task 20, 2026-10-02)

The first MYTHHELM run against a real native: the documented-command fix for
issue #60, driven end to end through `run`, `review` and `apply`.
Placeholders below replace the run/attempt/session IDs, home paths and the
organisation hash; everything else is verbatim receipt evidence.

## Commands

From a worktree of `main` on a throwaway input branch carrying `task.md` and
a `mythhelm.toml` that extends the repo checks with a `canary-name` grep:

```
MYTHHELM_LIVE_CLAUDE=1 go test -tags live ./adapters/claudecode -run TestLiveClaudeCanary -v
mythhelm run --task-file task.md --adapter claudecode --billing subscription-declared \
  --execution-profile trusted-host --trust-project-config sha256:<config-digest> \
  --declare-entitlement plan=max,extra-usage=disabled
mythhelm review <run-id>
mythhelm apply <run-id> --to-branch dogfood/canary-name --accept-flags
```

A first canary attempt during the maintainer's weekly limit failed as designed
(exit 1, `rate_limit` evidence, no result); after the limit reset the canary
passed in 6.8s. The limited run also exposed a decoder bug (live result
frames carry both `usage` and `modelUsage`), fixed in PR #59 before this run.

## Outcome

`ready_for_review`, exit 0. Nine assistant turns, one `Edit`, four `Bash`
calls, one `Read`; one further `Bash` attempt denied by `--allowedTools`
scoping and worked around. All four checks passed
(gofmt, vet, full `go test ./...`, canary-name). Review showed a one-line diff
and nothing else; apply created `dogfood/canary-name` at the candidate commit
after explicit `--accept-flags` for five git-ignored build artifacts (never
transferred). The source checkout was otherwise unchanged (`git status`, stash
list and reflog clean apart from the new branch).

## Receipt excerpt (sanitised)

- run `<run-id>`, state `completed`, exit 0; candidate `2222d48…` on admitted
  snapshot `3e3dbfc…` (branch `dogfood/t20-input`, clean at admission).
- billing `subscription-declared` (qualified: false, G05 not passed,
  `paid_continuation: unknown`, user declares disabled);
  `entitlement_source: user_declared+native_status(subscriptionType=max)`;
  `init_api_key_source: none`; identity `<org-hash>`.
- execution: `builtin/claudecode` 0.1.0, `claude` 2.1.285, model
  `claude-opus-5-5`, surface print/stream-json, `acceptEdits`, trusted-host.
- native result: `succeeded_native`, 7 turns, subtype `success`,
  denials `[Bash]`, session `<session-uuid>`.
- tokens (native-reported, `claude-opus-5-5`): input 12, output 1473,
  cache_read 187651, cache_creation 29596; retail-equivalent estimate
  0.3038062 USD (estimate, not a charge).
- native configuration: 17 hooks, 7 MCP servers inventoried (connected,
  pending and needs-auth states observed; none used by the task), trust grant
  `native_config:sha256:8d7c14cd…3d50e`.
- unknowns retained: whether any inference precedes `init` (single
  observation: only local hook/system frames did, zero tokens), native
  telemetry egress, descendants outside the process group.

## Checkpoints recorded

- Native-config source gap: maintainer confirmed no MDM or remote cached
  managed policy on this machine; the file inventory stands for this run.
- Transcript retention: maintainer accepted that native transcripts persist
  outside MYTHHELM.
- Terms (spec Q2): maintainer confirmed the recorded decision — own dogfood
  use proceeds, no public plan-sharing claims without a terms review.
- OAuth exception (ADR 0002 §6): stayed doubly closed; this run used native
  login (`credential_provenance: native-login`).

## Friction found

- #60 — the dogfood subject itself: Task 20 documented `-run TestLiveCanary`,
  which silently matches nothing (fixed by this run).
- #62 — entitlement declaration has no interactive prompt; admission exits 3
  even at a TTY that just answered the trust prompt.
- #63 — `native permission denied: Bash` names the tool but not the attempted
  command; supervision is blind without journal reading.
- #64 — `verification: started` then minutes of silence during the suite;
  working is indistinguishable from hung.
- #65 — candidate flags (`ignored_outputs`) are unexplained at review/apply;
  accepting required reading the source.

## Limitations of this evidence

The recorded stream is a rate-limit encounter, not a success stream: it pins
the decoder's error shape, not a full working turn sequence. No direct-native
comparison run exists (G02 partial), so this record stays `fixture-tested`,
never `live-qualified`.
