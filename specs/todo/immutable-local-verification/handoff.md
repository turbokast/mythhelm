# immutable-local-verification — Hand-off

> One section per task. Each task replaces its own `<!-- pending -->` line in its pull request with what
> dependent tasks need: what it produced (the API and files as shipped), what a later task must
> know, and any deviation that changes a later task's inputs. `/run-spec` puts the sections of a
> task's dependencies into that task's dispatch prompt. Open questions and research stay in
> `scratchpad.md`.

## Task 8 — Record the verification persistence and ownership decision

- **Produces**: ADR 0023 from commit `82a167e5f58e51e1083bb2dd88646410cb585653`, content SHA-256 `a6331c8057f66735115cb9f6480ac2976c3267708e08b2aea534f549109e9eb5`, in PR #623. Metadata closeout preserves those ADR bytes.
- **Independent review**: Codex architecture reviewer `adr_review` inspected the exact ADR against Task 8 and the source design on 2026-11-05; no substantive architecture blocker. Source design SHA-256 `e69693d7e0f3a97eba2d2607edc6e40a3bc266749b1a40e0aa6d3c213f5d3d46` matches the ADR. PR's original 28 checks had no failures and no unresolved threads; metadata revision requires fresh checks.
- **Governance acceptance**: The coordinator accepted the unchanged exact ADR above under the maintainer's explicit delegated repository decision authority on 2026-11-05. This accepts the design only, not implementation, supported-host qualification or runtime evidence.
- **Merge confirmation**: Pending. The coordinator must verify PR #623's reviewed merge and the unchanged ADR on `origin/main` before Task 1 starts; no merge SHA is asserted here.
- **For dependents**: Preserve the bounded private outside-worktree store, explicit owner reconciliation, quarantine/reference retention, exact definition/profile/manifest bindings and legacy-observed compatibility. No unsupported profile may become qualified from a skipped probe or this ADR.
- **Deviations**: None; no mechanism or qualification work was performed during metadata closeout.

## Task 1 — Define strict manifest, profile and result contracts

<!-- pending -->

## Task 2 — Implement private bounded snapshot ownership and retention

<!-- pending -->

## Task 3 — Capture exactly requested and actually selected source inputs

<!-- pending -->

## Task 4 — Enforce and qualify the initial read-only Linux profile

<!-- pending -->

## Task 5 — Integrate strong execution and migrate shared consumers

<!-- pending -->

## Task 6 — Prove transient-read rejection and the complete acceptance matrix

<!-- pending -->

## Task 7 — Document shipped modes and exact qualification limits

<!-- pending -->
