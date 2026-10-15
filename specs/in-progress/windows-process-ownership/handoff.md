# windows-process-ownership — Hand-off

> One section per task. Each task replaces its own `<!-- pending -->` line in its pull request with what
> dependent tasks need: what it produced (the API and files as shipped), what a later task must
> know, and any deviation that changes a later task's inputs. `/run-spec` puts the sections of a
> task's dependencies into that task's dispatch prompt. Open questions and research stay in
> `scratchpad.md`.

## Task 1 — decision record: Windows process ownership

- **Produces**: `docs/decisions/0017-windows-process-ownership.md`, recording the maintainer-approved OQ-1/OQ-8/OQ-11 defaults and D2/D3/D8/D11. It supersedes ADR 0004's named Windows design statements; Task 10 updates the old ADR's status.
- **For dependents**: worker-owned kill-on-close job; Windows-only native creation time in worker.json and event payload; no database migration; worker death does not itself resolve ownership. Preserve unresolved crash-boundary outcomes and the separate Windows stop-ladder version.
- **For Task 3**: real Windows runner measurements and OQ-3/OQ-13 architect review remain required. A kill-on-close parent job on either runner requires maintainer escalation before production changes.
- **Qualification**: no Windows support row is promoted by this ADR. Task 11 remains a maintainer decision supported by real Windows CI evidence.
- **Deviations**: none to the approved design or dependent acceptance criteria.

## Task 2 — fake adapter child-tree fixture for Windows

- **Produces**: `spawn_tree` with validated workdir-relative `report_path`; embedded scenario `tree` writes `tree.jsonl` and holds the native for ten minutes. Off Windows the scenario loads and returns a runtime `spawn_tree needs Windows` error wrapping `errors.ErrUnsupported`.
- **Topology**: native → child (`CREATE_NEW_PROCESS_GROUP`) → grandchild (inherited group). The native also attempts a `CREATE_BREAKAWAY_FROM_JOB` child. Descendants have null stdin/stdout/stderr and linger for ten minutes unless terminated.
- **Reports**: JSONL records carry `role`, `pid`, RFC 3339 UTC `start_time`; the `breakaway` record always carries numeric `error`, and includes identity only on success (`error: 0`). Report order is asynchronous: wait for child, grandchild and breakaway roles before acting.
- **For Tasks 3–7**: the fixture does not establish job ownership or require breakaway denial itself. Tests using a no-breakaway job must assert error 5 and no breakaway identity; always clean up any successful breakaway by its reported identity. Confirm liveness with exit code plus creation time, not creation time alone.
- **Report lifecycle**: the native truncates the report once before spawning; child/grandchild writers only append through `os.Root` confinement. Reusing a report path does not retain identities from a prior invocation.
- **Failure cleanup**: setup errors roll back retained process handles and descendants whose parent/creation-time interval matches. Child-side report failure cleans its own newly started grandchild before returning. Teardown does not rely on readable JSONL; successful fixture execution installs no job and descendants survive native exit.
- **Evidence**: source `dffab2f`, run [38730062877](https://github.com/turbokast/mythhelm/actions/runs/38730062877), Windows AMD64 job [116405401494](https://github.com/turbokast/mythhelm/actions/runs/38730062877/job/116405401494) passed the fake package with race detection (4.539s), and ARM64 job [116405401467](https://github.com/turbokast/mythhelm/actions/runs/38730062877/job/116405401467) passed (1.096s). Descendant/setup-failure tests have no runtime skip. Symlink cases alone may skip for missing Windows symlink privilege; non-verbose logs do not disclose their outcome. This is fixture execution evidence, not platform qualification (I14, v2 §17.1).
- **Deviation**: Windows acceptance lives in the added `adapters/fake/tree_windows_test.go`; no dependent API contract changes.

## Task 3 — Job Object launch ownership

<!-- pending -->

## Task 4 — job-wide stop, confirmation and descendant scan

<!-- pending -->

## Task 5 — native start time in the launch record

<!-- pending -->

## Task 6 — Windows recovery verdict

<!-- pending -->

## Task 7 — detach and interrupt behaviour on Windows

<!-- pending -->

## Task 8 — Windows process-support matrix (leaf package)

<!-- pending -->

## Task 9 — adapter capability derivation

<!-- pending -->

## Task 10 — Windows documentation

<!-- pending -->

## Task 11 — promote tested Windows rows

<!-- pending -->
