# Contained Execution Profiles — Scratchpad

> Open questions and research from authoring. Each implementing task reads this file when it starts and adds an entry under Discoveries when it finishes.

## Open questions

| # | Question | Default in this spec | Owner / blocks |
|---|---|---|---|
| 1 | macOS/Windows native mechanism qualification (Seatbelt profile? AppContainer?) | v1 refuses both with named missing coverage; follow-up spec qualifies | Maintainer / nothing in v1 |
| 2 | Destination-aware network blocking (netns plumbing / eBPF) | v1 = proxy pin + disclosed direct-egress residual; no user-requirement mechanism, so no refusal fires for users wanting no-egress (see row 3) | Maintainer / nothing in v1 |
| 3 | `--require-*` flag for users who need the no-egress guarantee as a single command | Not in this spec (honesty register); MH-22 or follow-up may add | Maintainer / nothing in v1 |
| 4 | Exact first-party endpoint host:port for the proxy allowlist | Resolved from the qualification record at admission; unknown endpoint blocks contained admission (I02) | Task 3+5 / proxy allowlist |
| 5 | Journal migration number: 0003 in the current tree (only 0001/0002 at `SchemaVersion = 2`); MH-21 streams plan 0003/0004 and MH-16 pins `0005_ledger.sql` on that chain | Task 7 takes the next free number at its start; whoever lands second renumbers | Task 7 |
| 6 | PID namespace (or other signal restriction) for same-UID sibling isolation | v1 = group ownership + stop ladder only; same-UID signalling is a disclosed residual (§6 honesty register) | Maintainer / nothing in v1 |

## Research notes

- Q1–Q3 decided by the maintainer 2026-10-08 (requirements.md Open
  Questions): per-platform native refusing where unavailable; shared
  mechanism for `inspect`; evaluator isolation here only.
- Go `os/exec` supports Linux user+mount namespaces via
  `SysProcAttr{Cloneflags, UidMappings, GidMappings}` plus
  `/proc/self/setgroups` handling; mount setup itself needs the `__contain`
  re-exec because Go has no fork/exec hook (D1).
- `x/sys/unix` (already vendored at v0.48.0) exposes `Mount`, `Unshare` and
  `Prctl` for the `EnterLinux` path; no new module dependency planned.
- Verification already runs post-reap (`pipeline.go` reap wait precedes the
  verify stage), so the evaluator threat is candidate content, not a live
  worker — the check policy (§2.7) targets exactly that.
- MH-22 (`specs/*/protected-acceptance/`) consumes receipt
  `execution_bundle.boundary`, `verification.evaluator`, and the new
  verification-row evaluator columns; it also sequences behind MH-21
  (TaskRevision shape), which this spec does not need.

## Discoveries

- Task 2: plain `mount(MS_REMOUNT|MS_BIND|MS_REC)` does not recurse, so the read-only root uses `mount_setattr(AT_RECURSIVE)` (Linux 5.12+); an older kernel makes `ProbeLinux` report the denied `readonly` step. `NO_NEW_PRIVS` is what stops root regaining capabilities at exec.


- Task 4: a failure inside `__contain` before exec is a native exit code 1 to the worker, not `launch_failed` (design §2.2 says launch_failed). Closing that gap needs a CLOEXEC status pipe from `__contain`; Task 5 or 9 should decide whether the refusal text can rely on `ProbeLinux` alone.
- Task 8: two weaker `not contained` wordings remain outside the disclosure pin by design (the task's test plan anchors to exact strings, not file-wide matches): the `BlockedError` fallback `Action` in `internal/admission/boundary.go` and the probe `Sandbox.Scope` in `adapters/claudecode/probe.go`. A follow-up may align them with `admission.TrustedHostDisclosure`; neither is an acceptance surface.
- Task 10: the admitted-path mapping covers every digest key, but not every key is re-hashable the same way: project keys are blob digests (immutable, never re-hash the workdir against them) and `user_mcp` is a selected-subset digest (re-inventory, never whole-file hash). Task 9's re-hash must treat the mapping as paths, not as a promise that each file hashes to its digest.
