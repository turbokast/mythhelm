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

<!-- pending -->

## Task 5 — Admission consult, restricted default, precise refusal

<!-- pending -->

## Task 6 — Inspect read-only enforcement

<!-- pending -->

## Task 7 — Protected check evaluator

<!-- pending -->

## Task 8 — Honesty surfaces and receipt boundary evidence

<!-- pending -->

## Task 9 — Startup boundary and authority fixtures

<!-- pending -->

## Task 10 — Adapter startup-inventory seam

<!-- pending -->

## Task 11 — Adversarial suite and frozen v1 evidence

<!-- pending -->
