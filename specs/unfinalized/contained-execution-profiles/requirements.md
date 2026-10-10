## Contained Execution Profiles — Requirements

> Ship the `restricted` and `inspect` execution profiles with adversarially tested
> enforcement boundaries, keep `trusted-host` honest about its residual trust, and
> refuse clearly wherever an OS cannot provide a claimed boundary. A slice of v2
> §§7–8, 11 and gate G07. Normative source:
> mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md, version 2.0; § numbers, I-IDs,
> G-IDs and AT-IDs refer to it.

## Context

- **Backlog card**: MH-13
- **Issue**: https://github.com/turbokast/mythhelm/issues/34 (from card MH-13)
  — "Contained execution profiles: restricted and inspect (MH-13)".

Today every run executes with the user's host authority. Admission admits only
`trusted-host` with explicit consent and refuses `restricted` and `inspect`
with exit 7 (`execution_profile_unavailable`), exactly as the dogfood slice's
non-goal N9 requires. The native launcher runs the native with an argv array and
the admitted environment, the owner lock coordinates a single writer per run,
and `internal/security/paths.go` performs pre-checks that its own comment
disclaims as "a pre-check, not a sandbox". There is no enforced filesystem,
process, network or credential boundary anywhere in the launch path, and checks
execute on the host in a detached worktree with no protected evaluator.

This spec adds the two contained profiles of v2 §8.1: `restricted`, the default
for new configuration, with a tested native/outer boundary covering named
filesystem, process, network and credential scope; and `inspect`, with enforced
read-only workspace and tools. Each claimed boundary is substantiated by
adversarial tests (AT-47); where an OS cannot provide a boundary, launch is
refused rather than silently weakened. Native startup hooks, MCP servers,
plugins and managed configuration are inventoried before admission (the Claude
Code route already does this) and cannot execute through a run before trust is
granted; a changed configuration invalidates the evidence. Nothing that merely
coordinates work — reservations, the owner lock, UI toggles — may masquerade as
containment evidence.

Grounding verdicts (step 4; code read 2026-10-08):

- v2 defines the three trust profiles and their guarantees — HOLDS
  (MYTHHELM_Master_Spec_v2.md:420-428; G07 at :893; AT-47 at :880).
- The dogfood slice admits only `trusted-host` with explicit consent; the other
  profiles exit 7 — HOLDS (specs/done/dogfood-slice/requirements.md:25,54;
  internal/admission/admission.go:429-431,436-450).
- No `restricted`/`inspect` enforcement exists in code — HOLDS (profile names
  appear only as admission constants, the exit-7 refusal and tests;
  `Profile.Contained` at internal/admission/admission.go:123 is never set true;
  containment-technology grep hits only disclosure strings and the
  not-a-sandbox disclaimer at internal/security/paths.go:22).
- Native startup hooks/MCP/plugins are inventoried and untrusted configuration
  blocks launch — PARTIAL (true for the Claude Code route: manifest carries
  hooks, MCP servers, enabled plugins and managed policy per
  internal/admission/native.go:33-56 and adapters/claudecode/auxiliary.go:43-57,
  and the run blocks until the digest-bound grant exists; drift: coverage is
  Claude-Code-only and run-granular, there is no generic cross-harness
  mechanism, and the fake adapter has nothing to inventory).
- Reservations and UI toggles are not containment and claim none — HOLDS (no
  reservation mechanism exists in code; coordination is the owner lock at
  internal/supervisor/ownerlock.go; TUI grep for
  contain/sandbox/isolat* finds no claims).
- The failure is reachable and acceptance can fail today — HOLDS (requesting
  either profile exits 7 today, internal/supervisor/pipeline_test.go:404, so any
  AC asserting a contained launch fails; adversarial escape tests fail
  trivially against no boundary).
- The issue's spec-section refs (§12.2, §12.3, §18.5, §20.3) locate the
  mechanism — PARTIAL (those are Revision 1.1 numbers; in v2 §12 is Adaptive
  improvement. Correct v2 mapping per the card Source: §§7–8, 11, §18.2–18.3).
- S1 needs this enforcement — HOLDS (S1 outcome includes "isolated one-agent
  work, protected checks", v2 §18.1; card is Stage 1, G07).

### Objectives

- **O1**: A user can run under `restricted` (the default for new configuration)
  or `inspect` and rely on an enforced, adversarially tested boundary, or get a
  precise refusal naming the missing coverage.
- **O2**: A user on `trusted-host` always sees and records that native code runs
  with host authority, and no UI text claims protection from malicious
  same-user code.
- **O3**: Protected verification runs outside worker reach, so a candidate
  cannot weaken its own checks.

### Non-Goals (and what still binds)

| # | Out of scope | Still binding in this spec |
|---|---|---|
| N1 | Full S1 one-agent workflow beyond profiles (MH-22). | I07, G07/AT-47: the boundary evidence this spec ships is recorded and versioned (receipt + evidence records) so downstream acceptance can consume it without re-proving it. |
| N2 | Additional harnesses beyond the qualified routes in tree. | I14: each profile × OS claim ships versioned evidence or is explicitly unsupported. |
| N3 | Quota/budget ledger mechanics (MH-16). | I02: unknown billing evidence still blocks per existing admission. |

## Functional Requirements

Acceptance criteria use EARS. Tags in brackets name the invariants, gates and
sections each one serves.

### FR-1 — Restricted profile enforcement (§8.1, §7.1, I02, I14, G07)

- **AC-1.1** [§8.1, AT-47] When a run is admitted with `--execution-profile
  restricted` on an OS/route whose boundary evidence is recorded, the system
  shall launch the native inside the boundary covering the named filesystem,
  process, network and credential scope, and the receipt shall record the
  boundary name and version.
- **AC-1.2** [I02, AT-47] If required boundary coverage is missing or its
  evidence is unknown for the admitted OS/route, then the system shall refuse
  launch naming each missing coverage, without dropping to host authority.
- **AC-1.3** [§8.1, I04] A configuration with no recorded profile choice shall default to `restricted`; existing configurations keep their current profile. Selecting `trusted-host` requires the explicit `--execution-profile trusted-host` flag or the interactive consent flow (`internal/admission/admission.go:436-450`), since a posture change needs user authority.

### FR-2 — Inspect profile enforcement (§8.1, G07)

- **AC-2.1** [§8.1, AT-47] When a run is admitted with `--execution-profile
  inspect`, the system shall enforce a read-only workspace and read-only tools,
  and shall refuse all checks under `inspect` until MH-22 ships check scoping (no per-check scope exists in the admitted check list today).
- **AC-2.2** [§8.1] If read-only enforcement would rest only on a prompt or a
  tool label, then the system shall refuse the run.

### FR-3 — Trusted-host honesty (§8.1, I09, G07)

- **AC-3.1** [§8.1, AT-47] The system shall disclose on `trusted-host`, in the admission/CLI disclosure and the receipt containment label (`internal/supervisor/receipt.go:98-99`), that arbitrary same-user code is not adversarially contained, and no surface shall claim protection from malicious same-user code.
- **AC-3.2** [I09] When boundary evidence is unknown, the system shall report
  it as unknown, never as contained or verified.

### FR-4 — Native startup boundary (§8.2, I03, G07)

- **AC-4.1** [AT-11] When effective startup hooks, MCP endpoints, plugins or
  managed configuration change after admission, the system shall invalidate the
  affected trust/qualification evidence before the next launch.
- **AC-4.2** [I03, AT-12] The system shall block authority-widening content on each enumerated channel — task-file content, environment variables, plugin messages, and model output reaching the launch path (one failing fixture each) — from widening launch authority or altering protected rules; channels outside this list are named as residual risk, not silently covered.
- **AC-4.3** [§8.2] Any restriction the boundary imposes on native features
  shall be reported as a visible fidelity delta.

### FR-5 — Protected verification (§11.2, I07)

- **AC-5.1** [§11.2, I07] The system shall execute checks from a protected
  evaluator snapshot or separately protected process/configuration outside
  worker reach, so the candidate cannot weaken tests, hide exit status or mark
  itself accepted.
- **AC-5.2** [§11.2] When checks are unavailable, the system shall preserve the
  candidate unverified and label the gap explicitly; it shall never report
  acceptance.

### FR-6 — No masquerade (I05, AT-47)

- **AC-6.1** [I05] The system shall not accept reservations, the run-owner lock
  or UI toggles as evidence of containment; only the declared enforcement
  mechanism substantiates a boundary claim.

### FR-7 — Native authentication preserved (§8.1, I19)

- **AC-7.1** [§8.1, I19] When a route is admitted under `restricted` or
  `inspect`, native authentication shall keep working inside the boundary
  without copying whole credential stores; where they cannot coexist, the
  system shall advertise the route's supported trusted profile only.

### FR-8 — Adversarial boundary tests (G07)

- **AC-8.1** [AT-47] Each claimed enforced boundary shall have adversarial
  tests covering filesystem escape, subprocess escape, network egress and
  credential access, including native children and startup paths, and the tests
  shall fail when run against an unenforced launch.

## Non-Functional Requirements

- **NFR-1** [G07, I14] Every profile × OS × route claim ships with versioned
  evidence and a named owner; unproven combinations refuse with a precise
  blocker.

## Definition of Done

- [ ] AC-1.1 to AC-8.1 each have a named test; CI green on Linux, macOS and
  Windows.
- [ ] Requesting `restricted`/`inspect` where unsupported still exits 7 with
  the missing coverage named.
- [ ] No UI or receipt text claims containment that the adversarial tests do
  not substantiate.

## Open Questions

- **Q1**: Which OS mechanism implements the boundary on each platform (Linux:
  user namespaces + Landlock / bubblewrap / container; macOS: Seatbelt;
  Windows: Job Objects / AppContainer)? Blocks FR-1/FR-2 design; options are
  per-platform native mechanisms vs one container dependency. Decided
  2026-10-08 (maintainer): per-platform native, refusing where unavailable.
- **Q2**: Does `inspect` share the `restricted` mechanism with a read-only
  policy, or a separate lighter one? Blocks FR-2; options: shared mechanism
  (less code, one evidence set) vs separate (smaller trusted surface for
  read-only). Decided 2026-10-08 (maintainer): shared mechanism.
- **Q3**: Where is the line between this spec's "protected verification"
  (FR-5) and MH-22 protected acceptance, which sequences behind MH-13? Blocks
  FR-5 scope; options: this spec ships evaluator isolation only, leaving
  revision binding and receipts to MH-22 (preferred), vs shipping both.
  Decided 2026-10-08 (maintainer): evaluator isolation here only.
- **Q4**: Decided (refine loop): aggregate all gaps, per AC-1.2's "naming each missing coverage" and the admission problem list.
- **Q5**: UNVERIFIABLE carry-over: none — every load-bearing claim grounded.

## Dependencies

- Prerequisites (shipped): dogfood slice (`specs/*/dogfood-slice/`, N9/AC-2.3
  refusal this spec lifts), qualification registry
  (`specs/*/qualification-registry/`, `TrustProfile` already in the
  qualification key per its requirements.md:45).
- In flight, possible conflict: `claude-strict-subscription` (MH-12,
  `specs/*/claude-strict-subscription/`, touches `internal/admission/` and
  native inventory); `supervised-stop-recover`
  (`specs/*/supervised-stop-recover/`, overlaps
  `internal/supervisor/pipeline.go` and `internal/workers/worker.go` —
  serialize per design §7).
- Planned, not yet specs: `supervisor-service` and `supervisor-migration`
  exist only as rows 2–3 of `specs/*/v2-contracts-supervisor/plan.md`; their
  MH-21 streams plan journal migrations 0003/0004, so Task 7 re-checks the
  next free number at start (see tasks.md Dependencies).
- Downstream: `protected-acceptance` (MH-22,
  `specs/*/protected-acceptance/`) sequences behind MH-13 and consumes its
  boundary evidence.
- Sequenced alongside, not a prerequisite: MH-16 (`specs/*/budget-ledger-s1/`) — ledger mechanics are N3 out of scope; unknown billing evidence blocks per existing admission (I02). No AC in this spec reads ledger state. Landing order: MH-16 pins `0005_ledger.sql` on the assumption that MH-21's 0003/0004 land first; this spec's evaluator migration takes the next free number at Task 7 start and whoever lands second renumbers.
- Superseded: none.

## Impacted components

- `internal/admission/admission.go:37-39,423-450` — profile constants, exit-7
  refusal and consent flow to extend.
- `internal/admission/native.go` — native startup inventory and digest-bound
  trust to enforce within the boundary.
- `internal/workers/` (`worker.go`, `proc_*`) and
  `docs/decisions/0004-slice-process-model.md` — worker/native spawn is where
  the boundary attaches.
- `internal/security/` (`env.go`, `paths.go`) — environment and path controls
  to harden into enforcement.
- `internal/integration/verify.go:47-49` — check execution to move under a
  protected evaluator.
- `internal/cli/run.go:30` — `--execution-profile` flag help and defaults.
- `internal/supervisor/receipt.go:98-99` — receipt containment labelling.
- `adapters/claudecode/` (`effective.go`, `auxiliary.go`, `decode.go`) —
  manifest fields (hooks/MCP/plugins/managed policy) the boundary must cover.
