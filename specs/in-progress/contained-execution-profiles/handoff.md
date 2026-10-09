# contained-execution-profiles — Hand-off

> One section per task. Each task replaces its own `<!-- pending -->` line in its pull request with what
> dependent tasks need: what it produced (the API and files as shipped), what a later task must
> know, and any deviation that changes a later task's inputs. `/run-spec` puts the sections of a
> task's dependencies into that task's dispatch prompt. Open questions and research stay in
> `scratchpad.md`.

## Task 1 — Containment contract package

- **Produces**: `internal/contain/contain.go` exactly as design §4 (Dimension consts, Claim, Coverage, Evidence, Registry, AuthBind, Policy, Availability, ErrUnsupported, ErrMissingCoverage); no `Provider` interface yet.
- **For dependents**: `Coverage.Missing` returns nil (not empty slice) when nothing is missing, keeps the caller's order, and reports unknown dimensions as missing. A zero `Claim` is unenforced. Registry lookups must return `ok == false` for unknown combinations.
- **Environment trap**: set `GOTOOLCHAIN=go1.27.1` for golangci-lint, and unset `ANTHROPIC_BASE_URL` in cloud sessions or a supervisor test fails.
- **Deviations**: None.

## Task 2 — Linux boundary mechanism

- **Produces**: `contain.ProbeLinux`, `EnterLinux(ContainSpec) error`, `ContainSpec{Path, Args, Dir, Env, Policy}` (`Args` is the full argv), `PolicyFor`, plus exported `ProbeEnv` and `RunProbeChild`; unexported `nsSysProcAttr()` returns the user+mount `SysProcAttr` (nil off Linux).
- **For dependents**: `EnterLinux` must run in a process already started with `nsSysProcAttr()`; it needs an absolute `HOME` in `Env`, an existing workdir that does not enclose `HOME` or `/tmp` (it is refused), an existing `HOME` outside `/tmp` (anything else is refused), auth-bind targets under `HOME`, and auth-bind sources that are regular files outside the workdir (a terminal symlink, directory or in-workdir source fails the launch); each source is masked at its original path. `EnterLinux` resolves symlinks in `HOME`, the workdir and `/tmp` first and checks and mounts the resolved paths; `PolicyFor` resolves the existing prefix of HOME, workdir, `/tmp` and each auth source's directory (so both sides of every comparison are real paths) and keeps a missing tail as written; the returned `Policy` carries the configured workdir and binds verbatim. It never returns on success.
- **Task 4 must**: call `RunProbeChild` from `contain.Main` / `cmd/mythhelm` when `ProbeEnv` is set (`ProbeLinux` runs `<self> __contain`), or `ProbeLinux` reports a spurious failure; export or reuse `nsSysProcAttr` for the worker spawn.
- **Tests**: the contain test binary acts as its own helper (`TestMain` modes) — the same pattern fits Task 4.
- **Deviations**: see the entry; the wiring gap above is the only one that changes a later task's input.

## Task 3 — Filtering egress proxy

- **Produces**: `internal/contain/proxy.go` exactly as design §4 — `ServeProxy(ctx, allow) (addr, stop, err)` and `ProxyEnv(addr) map[string]string` (keys `HTTPS_PROXY`, `HTTP_PROXY`, `https_proxy`, `http_proxy`, values `http://` + addr). No `Provider` wiring yet.
- **For dependents (Task 4)**: start the proxy before a contained spawn, fill `Policy.ProxyAddr` with the returned ephemeral `127.0.0.1:port`, and amend the child env with `ProxyEnv` post-admission. `stop` is safe to call twice; cancelling `ctx` also stops the listener.
- **For dependents**: allowlist entries are exact `host:port` strings matched verbatim against the CONNECT request-target — no normalization, no suffix or port-range matching. `httptest` server URLs need the `http://` prefix stripped to form an entry.
- **For dependents**: one connection carries exactly one request; denials (403 unlisted, 405 non-CONNECT, 431 over-long headers, 400 malformed, 502 dial failure) all return before any upstream dial. A 502 means the entry was allowlisted but the target refused the TCP dial.
- **For dependents**: the proxy never claims to block direct egress — keep that residual disclosed, never assert it in later tests.
- **Environment trap**: the `internal/cli` drift tests read ambient native config from `$HOME`; a real `~/.claude.json` fails `TestStrictMainBlocksWriteNothing` with `untrusted_native_config`. Run gates with an empty `HOME` (keeping `GOPATH`/`GOMODCACHE`/`GOCACHE` on the real cache, and `/usr/bin` first on `PATH` so `python3` avoids the asdf shim, which needs `HOME`).
- **Deviations**: None.

## Task 4 — `__contain` command and worker wiring

- **Produces**: `contain.Command` (`__contain`), `contain.Main(args) int`, `contain.NamespaceAttr()`; `workers.Launch.Containment *contain.Policy` and `Launch.ProxyAllow []string`; `Launch.command(spec)` builds the uncontained or `__contain` command; `cmd/mythhelm` dispatches `__contain`.
- **For dependents**: whoever builds a `Launch` (Task 9's `launchForAttempt`) sets `Containment` and `ProxyAllow` (the admitted endpoint host:port; empty denies all egress) and an env whose `HOME` lies outside `/tmp`, does not enclose or sit under the workdir, and exists. The worker fills `Policy.ProxyAddr` and the proxy pins itself.
- **Setup failures**: `__contain` exits 1 (setup) or 2 (invalid spec) with a message on the attempt's captured stderr; the worker records a native exit, not `launch_failed`. Mapping it needs a status channel (not built).
- **Tests**: both `TestMain`s dispatch `contain.Command`; a native fixture must live outside `/tmp` (the boundary replaces it) — see `TestContainedProxyDenyThroughPinnedEnv`.
- **Deviations**: `ProxyAllow` and the exported `NamespaceAttr` (see the entry).

## Task 5 — Admission consult, restricted default, precise refusal

- **Produces**: `admission.BoundaryConsult(profile, os, route) (contain.Evidence, error)` (exit-7 `*BlockedError` naming each missing dimension; unknown combinations also wrap `contain.ErrMissingCoverage`); `contain.SeedV1()` (key `profile/os/route`; `restricted` and `inspect` × `linux`/`darwin`/`windows` × `builtin/fake`/`builtin/claudecode`); `Profile.Contained` set for `restricted`; `Profile.Consent` is `"default"` for the empty flag.
- **For dependents**: `Decide` does not yet keep the evidence: Task 8 must carry it to the receipt and Task 9's `launchForAttempt` must build `Launch.Containment`/`ProxyAllow` from the admitted profile. `inspect` is refused in `checkCapabilityFlags`; Task 6 removes that refusal when its enforcement lands. The Claude Code route is refused for `restricted` (credential unenforced) until Task 10 or later binds auth.
- **Tests**: `restricted` is admitted only on Linux; tests that need it skip elsewhere, and `TestMissingProfileConsentNonInteractiveExit3` asserts the refusal off Linux. Run gates with `HOME` set to an empty directory.
- **Deviations**: see the entry (inspect still refused; Claude Code route seeded as credential-missing).

## Task 6 — Inspect read-only enforcement

- **Produces**: `admission.ConsultRegistry(reg contain.Registry, profile, os, route)`; `contain.PolicyForProfile(profile, workdir, binds, proxy)` (inspect = restricted with `ReadOnly`) and `contain.ProfileInspect`; `Profile.Name == "inspect"` with `Contained == true` is admitted on Linux.
- **For dependents**: Task 9's `launchForAttempt` must build the policy with `PolicyForProfile`, never a hand-set `ReadOnly`. Task 7's `RunChecksWithPolicy` is never reached under `inspect`: the pipeline's verify stage skips checks (reason `checks_refused_under_inspect`) before it; keep that branch when Task 7 edits `pipeline.go`.
- **Tests**: `inspect` runs need Linux; the supervisor pipeline tests skip or assert the exit-7 refusal elsewhere. The `__check` helper has a `marker` mode that writes the file named by its next argument.
- **Deviations**: see the entry (`admission.go` edited; the refusal is a `NOT RUN` verification, not a new state).

## Task 7 — Protected check evaluator

- **Produces**: `integration.RunChecksWithPolicy(ctx, cand, cfg, env, RunOptions{KeepGoing, Policy})` (nil policy = host); `Verification.Evaluator` (`contain.Evaluator{Name, Digest}`, also in the `verification.completed` payload); `contain.EvaluatorDigest`, `contain.BoundaryName`/`BoundaryVersion`, `contain.ApplyNamespaces`; `journal.VerificationRow.EvaluatorName`/`EvaluatorDigest` from schema v6 (`verification_evaluators` side table; empty for older rows).
- **For dependents**: Task 8 renders `Verification.Evaluator` in the receipt; read it from the row (`LatestVerification`), never recompute it. The next free migration is 0007. The pipeline picks the policy from `Decision.Profile.Contained`; a contained check needs `HOME` in the env outside `/tmp`, so callers building the check env must keep that (it is the Task 2 contract).
- **Behaviour change**: a missing check executable now ends `failed`/`verification_unavailable` (was `verification_failed`); `inspect` still skips checks before this stage (Task 6).
- **Tests**: contained-check tests skip off Linux or where `ProbeLinux` fails, need the check binary outside `/tmp` (the boundary replaces it) and one fresh candidate per verification (evidence directories are keyed by commit).
- **Deviations**: see the entry (migration 0006 and a side table; new `evaluator.go`).

## Task 8 — Honesty surfaces and receipt boundary evidence

- **Produces**: `admission.TrustedHostDisclosure` (the exported verbatim trusted-host sentence); `Profile.Boundary *contain.Evidence` (the consulted evidence kept on the admitted decision; nil for trusted-host); receipt `execution_bundle.boundary` (`{name, version, coverage{filesystem, process, network, credential}}`, or `unknown`) and `verification.evaluator` (`{name, digest}`, or `unknown`).
- **For dependents**: boundary coverage values are mechanism names when enforced, `unknown` otherwise — never the words contained or verified; a trusted-host run renders boundary `unknown`. Read the evaluator from the verification row (`LatestVerification`), never recompute it. Any new trusted-host copy (Task 11 refusal/fixture text) must reuse `TrustedHostDisclosure` verbatim.
- **Deviations**: see the entry (`pipeline_test.go` assertion update; no input change for later tasks).

## Task 9 — Startup boundary and authority fixtures

<!-- pending -->

## Task 10 — Adapter startup-inventory seam

- **Produces**: `claudecode.AdmittedConfigPaths(home, workdir) map[string]string` (inventory source name → absolute path) plus `AdmittedConfigPathsForEnv(home, workdir, env)` for callers holding the admitted child env; both built on the same `inventorySources` list the inventory uses.
- **For dependents (Task 9)**: build `Launch.UserConfigPaths` with `AdmittedConfigPathsForEnv` and the admitted child env, not the two-argument form: the `user`/`user_mcp` paths follow `CLAUDE_CONFIG_DIR`, and the ambient value may differ from the admitted one.
- **For dependents (Task 9)**: re-hash only the mutable sources (`user`, `user_mcp`, `managed`, `managed_mcp`, `managed_fragment_*`). The mapping also carries `project`/`project_local`/`project_mcp` pointing at the live workdir, but admission inventories committed blobs for those — a workdir re-hash against blob digests would mismatch.
- **For dependents (Task 9)**: fail the launch when a manifest digest key has no entry in the mapping. Non-absolute roots yield an empty map, and a fragment listing that fails after admission omits fragments; both are missing mappings, never skipped sources.
- **For dependents (Task 9)**: `user_mcp` has no whole-file digest: its manifest digest covers the selected MCP subset (account/session excluded). Re-verify it by path presence plus re-inventory, never by hashing the file.
- **For dependents**: `Prepare` takes no manifest input (pinned by a `PrepareInput` field scan in `TestPluginsNeverWidenArgv`); surplus session plugins/MCP collapse to `plugin:unknown`/`mcp:unknown` with `unknown` funding (I03).
- **Deviations**: `AdmittedConfigPathsForEnv` added (see the entry); no input change for later tasks beyond using it.

## Task 11 — Adversarial suite and frozen v1 evidence

<!-- pending -->
