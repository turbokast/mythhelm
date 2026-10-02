## TUI Slice — Requirements

> The polished focused TUI that Stage 1 requires: the mission view, responsive layouts down to the linear accessible mode, keyboard flows and event-driven motion, on top of the run pipeline the dogfood slice delivers. A slice of master spec §15, §20.3 and §22.2 item 5. Normative source: docs/spec/master-spec.md; § numbers, I-IDs, G-IDs and A-IDs refer to it.

## Context

- **Backlog card**: MH-2
- **Source issue**: [#23 — TUI slice: the focused mission view (MH-2)](https://github.com/turbokast/mythhelm/issues/23)

The dogfood slice (`specs/*/dogfood-slice/`) shipped the full run pipeline — admission, snapshot, native attempt under worker ownership, freeze, configured verification, receipt, guarded apply, recovery — with only linear plain/JSONL output. Its non-goal N1 deferred the TUI explicitly, keeping `--plain` truthful so scripts would not break later. This spec delivers the other half of Stage 1 usability: a focused single-mission Bubble Tea view (§6.4) showing the user's goal, the next required action, current agent activity, the candidate diff and verification status before abstract metrics (§15.1).

The slice covers the wide mission view (§15.3), the responsive layout ladder down to compact and linear modes (§15.4), keyboard navigation and the command palette (§15.5), the signature motion moments with colour/motion/icon controls (§15.6–§15.7), the linear screen-reader mode (§15.8) and the event-driven render model (§15.9). It feeds gate G09 (terminal usability), a Stage 1 exit gate (§20.3).

Grounding verdicts (2026-10-02; rule: `.claude/rules/spec-premise-grounding.md`):

- Dogfood slice delivers the run pipeline this TUI sits on — HOLDS (`specs/*/dogfood-slice/requirements.md` FR-1–FR-12; MH-1 shipped).
- Dogfood slice deliberately defers the TUI as its non-goal N1 — HOLDS (`specs/*/dogfood-slice/requirements.md:17`, "TUI (Bubble Tea mission view, §15.3–§15.9). Next slice.").
- Stage 1 requires the polished focused TUI — HOLDS (`docs/spec/master-spec.md:2109`, "polished focused TUI").
- §15 covers visual language, mission view, responsive layouts, navigation, motion, accessibility, render model — HOLDS (`docs/spec/master-spec.md:1529` §15.2, `:1537` §15.3, `:1565` §15.4, `:1577` §15.5, `:1585`–`:1604` §15.6–§15.7, `:1612` §15.8, `:1620` §15.9).
- §22.2 item 5 is the usability backlog item — HOLDS (`docs/spec/master-spec.md:2207`, "Make it usable: focused TUI standalone and inside Herdr …").
- Release gate is G09 — HOLDS (`docs/spec/master-spec.md:1946`, "G09 — Terminal usability").
- Go + Bubble Tea is the chosen TUI stack — HOLDS (`docs/spec/master-spec.md:479`).
- No TUI exists yet, so every new-view criterion fails today — HOLDS (`ls internal/tui mods`: no such directories; `internal/` holds adapter, admission, buildinfo, cli, ids, integration, journal, security, statedir, supervisor, workers, workspace).

### Objectives

- **O1**: A user running a mission sees goal, next action, agent activity, candidate diff and verification status in one focused view (§15.1, §15.3).
- **O2**: The view stays usable from wide terminals down to narrow ones and non-TTY linear output, with no broken half-panels and no lost approvals on resize (§15.4).
- **O3**: Keyboard-only operation, the command palette and contextual help cover every action; destructive actions need explicit labelled confirmation (§15.5).
- **O4**: Motion is event-driven and truthful, honours reduced-motion preferences, and never delays output or blocks input (§15.6, §15.7, §15.9).
- **O5**: A linear screen-reader mode carries complete state labels with no spinner chatter or cursor-rewrite dependence (§15.8).

### Non-Goals (and what still binds)

| # | Out of scope | Still binding in this spec |
|---|---|---|
| N1 | Herdr presentation and native-agent inspection (§15.11, §16.6). MH-3 owns the bridge. | I17, I18: the TUI must not certify completion from pane text or add a second supervisor/writer. |
| N2 | Multi-mission dashboard. §15.1 allows one when multiple missions justify it; the dogfood pipeline runs one at a time. | §15.1: default to the focused single-mission view. |
| N3 | Plugin/mod/theme extension points (§14); the TUI ships one default dark and one light theme as built-in tokens. | I11: no external code reaches admission, approval, evidence or publication through theming. |
| N4 | New CLI verbs beyond launching/inspecting the view. | §15.10: exit codes and `--format jsonl` / `--plain` behaviour from the dogfood slice stay unchanged. |
| N5 | Claimed screen-reader support before the §15.8 test programme (NVDA, VoiceOver, Orca) runs. | I14: untested combinations are marked experimental/unsupported, never advertised. |

## Functional Requirements

Acceptance criteria use EARS. Tags in brackets name the invariants, gates and sections each one serves.

### FR-1 — Focused mission view (§15.1, §15.3)

- **AC-1.1** [§15.3] When a run is selected, the system shall show the mission goal, the next required action, current agent activity, the candidate diff (or its absence) and verification status in the wide layout.
- **AC-1.2** [§15.3, §15.7] The selected-change panel shall render a real diff with syntax-aware, width-safe presentation where the resolved colour/Unicode capability and the pane width allow it, falling back to a plain width-safe diff otherwise; Markdown rendering shall not substitute for the diff component.
- **AC-1.3** [§15.2] The system shall label each lane with a readable runtime/profile label and a stable local identifier, never a vendor logo as the primary navigation.
- **AC-1.4** [§15.2, I09] The system shall present state with text plus shape/icon and optional colour, with ASCII alternatives for decorative glyphs; estimated, reported, observed and unknown values shall stay visually distinct.
- **AC-1.5** [§15.2, I13] The system shall not require, install, bundle or claim control over terminal fonts; Nerd Fonts shall be optional, never necessary for comprehension.

### FR-2 — Responsive layouts (§15.4, G09)

- **AC-2.1** [§15.4] When the terminal has at least 140 columns and sufficient height, the system shall show tasks, agents and selected detail, with optional compact global status.
- **AC-2.2** [§15.4] When the terminal has 100–139 columns, the system shall show two panes with a switchable detail panel.
- **AC-2.3** [§15.4] When the terminal has 80–99 columns, the system shall show a single focus pane with labelled tabs and persistent critical status.
- **AC-2.4** [§15.4] When the terminal is under 80 columns or very short, the system shall show a compact task/status view and offer linear output, with no broken half-panels.
- **AC-2.5** [§15.4] When output is non-TTY, `TERM=dumb`, machine output or explicit plain mode, the system shall emit stable linear output without cursor movement or decorative control codes.
- **AC-2.6** [§15.4, I06] While a resize happens, the system shall never lose a pending approval, move a destructive action beneath an already-held key, or change the selected task unexpectedly.

### FR-3 — Navigation and actions (§15.5, G09)

- **AC-3.1** [§15.5] The system shall provide arrow-key and standard focus navigation, with `/` filtering the current collection, `:` opening a searchable command palette, `?` showing contextual help, `Enter` opening details and `Escape` backing out safely.
- **AC-3.2** [§15.5] If mouse support is offered, then every mouse-reachable action shall also be reachable by keyboard and plain output.
- **AC-3.3** [§15.5, I03] When an action is destructive or costly (push, permission expansion, spend escalation, irreversible cleanup), the system shall require an explicit labelled confirmation; no single accidental keypress shall trigger it.

### FR-4 — Truthful motion and terminal capability (§15.6, §15.7, G09)

- **AC-4.1** [§15.6] The system shall implement the applicable signature moments (arrival, route selected, dispatch, live work, waiting, integration, delivery, failure/recovery) with their truth rules: interruptible arrival, running only after launch acknowledgement, no artificial typewriter delay, countdowns only for known resets/deadlines, no progress percentage without a meaningful denominator, no screen shake or flashing spectacle on failure.
- **AC-4.2** [§15.6, §15.7] When reduced motion is requested (`--motion reduced|off`, or a reliably queryable OS preference), the system shall use immediate state changes and static emphasis.
- **AC-4.3** [§15.7] The system shall support `--colour auto|always|never`, `--motion auto|full|reduced|off` and `--icons auto|unicode|ascii`, accept `--color` as an alias, and respect `NO_COLOR` for colour suppression without treating it as motion suppression.
- **AC-4.4** [§15.7] The system shall expose explicit overrides for colour, motion and icon capability and never rely only on `COLORFGBG` for detection.

### FR-5 — Accessibility (§15.8, G09, I14)

- **AC-5.1** [§15.8] The system shall provide a linear screen-reader mode with complete state labels, no repeated spinner chatter, no cursor-rewrite dependence and an ordered stream of meaningful changes.
- **AC-5.2** [§15.8] Focus shall be visible and stable; help shall describe full action names; colour shall never be the sole indicator; important text shall not be conveyed only through animation, Unicode icons, hover or a changing progress bar.
- **AC-5.3** [§15.8] Long content shall be pageable or exportable as plain text; bidirectional text, combining characters, emoji and wide glyphs shall not displace approval controls or hide filenames.
- **AC-5.4** [§15.8, I14] Accessibility support shall be claimed only for tested screen-reader/terminal combinations (NVDA, VoiceOver, Orca in representative terminals); untested combinations shall be marked experimental or unsupported.

### FR-6 — Render model (§15.9)

- **AC-6.1** [§15.9, §15.6] The UI shall subscribe to coalesced view updates while the supervisor preserves critical events; rendering shall be event-driven with short transitions of roughly 80–200 ms and no permanent animation loop, using up to 60 fps only for brief motion.
- **AC-6.2** [§15.9] The system shall virtualise long lists and diffs, show at most the visible pane's rows of a live stream on screen with the full stream kept in searchable history, and never let a paused or slow terminal stall native event consumption indefinitely.

### FR-7 — Truthfulness of displayed state (I06, I07, I09)

- **AC-7.1** [I06] When a stop has been requested but not confirmed, the system shall label it requested, never stopped.
- **AC-7.2** [I07] The system shall attach verification status to the exact candidate revision it checked; an agent's completion report alone shall never render as verified.
- **AC-7.3** [I09] Cost, quota and allowance figures shall be labelled with their kind (native-reported estimate, observed, unknown); unknown shall never render as zero.

## Non-Functional Requirements

- **NFR-1** [§16.4, G09] Terminal/shell matrix, plain mode, resize and keyboard flows have recorded results (per G09's pass condition).
- **NFR-2** [§18.6] UX acceptance covers a non-Vim, non-orchestration-vocabulary user running the demo, discovering a native profile, understanding billing, starting a bounded task, recognising attention-needed states, inspecting the diff, stopping/detaching and locating the receipt — with recorded completion, confusion, accidental actions and the state-distinction checks (running vs waiting vs ready-for-review vs applied).
- **NFR-3** [§19.4, §19.2] New TUI dependencies (Bubble Tea, Lip Gloss) pass dependency review and the licence policy; `go build`, `go vet`, `go test -race ./...` pass on Linux, macOS and Windows CI.

## Definition of Done

- [ ] AC-1.1 to AC-7.3 each have a named test; G09 evidence recorded for the advertised terminal/shell matrix.
- [ ] `golangci-lint` and `govulncheck` green; dependency review passes for new TUI dependencies.
- [ ] README status section lists what the TUI supports and what stays unsupported/experimental (I14).

## Open Questions

- **Q1** (blocks FR-1/FR-6 design): What view-model API does the TUI consume — direct journal/projection reads in-process, or a new query seam? Options: (a) read the dogfood SQLite projections directly; (b) a small Go view-model package over the journal; (c) defer to `/spec` investigation. Default: (c), decided in design.
- **Q2** (blocks FR-4 scope): Which of the ten signature moments apply to a single-agent, single-writer Stage 1 product (e.g. handoff and reservation conflict may have no trigger yet)? Options: (a) implement applicable subset, honesty-register the rest; (b) stub all ten. Default: (a).
- **Q3** (blocks FR-5/AC-5.4 scope): Which screen-reader/terminal combinations are advertised vs experimental for this slice's first release? Options: (a) one combination tested, rest experimental; (b) full §15.8 matrix before merge. Default: (a) with I14 labelling.
- **Q4** (blocks FR-2 testing): What is the advertised terminal/shell matrix for G09 evidence? Options: (a) reuse the dogfood CI OS matrix plus named terminals; (b) a fixed terminal list from §16.4. Default: decided in `/spec` from §16.4.
- **Q5** (blocks AC-2.4 test): What row count counts as "very short height"? Master-spec §15.4 leaves it qualitative. Default: decided in `/spec`; until then the AC-2.4 test uses an unambiguously short height (at most 5 rows).

## Dependencies

- **Prerequisite**: `specs/*/dogfood-slice/` — the run pipeline (journal envelope, projections, receipt, review diff, plain/JSONL output) this TUI renders. Shipped (MH-1 shipped).
- **Conflicting**: none — `specs/in-progress/` and `specs/unfinalized/` are empty; `specs/todo/` holds only `specs/*/docs-site-demos/` (checked 2026-10-02), which plans no `internal/tui/` or `mods/` changes and sequences its FR-3 TUI recording behind this spec (ordering: this spec lands first for any TUI-recording work; no shared files either way). No other live sibling spec exists. The two specs' planned `README.md` edits are disjoint regions (status section vs demo embed) and can land in either order.
- **Superseded**: none.
- **Follow-on (not blockers)**: MH-3 Herdr bridge (§15.11 presentation stays consistent); MH-6 plugin protocol and declarative themes (built-in tokens now, external format later); `docs-site-demos` FR-3 TUI recording (sequences behind this spec).

## Impacted components

- New: `internal/tui/` (Bubble Tea model, views, components) and `mods/` (declarative layout/keymap/theme surface, built-in defaults only) — tui domain, `tui-implementer`.
- Consumed, not changed: `internal/journal/` (event envelope), `internal/cli/` (plain/JSONL output stays the fallback), receipt/review pipeline from the dogfood slice — core domain.
- `go.mod`/`go.sum`: new Bubble Tea / Lip Gloss (and transitive) dependencies — core domain, needs dependency review.
