## TUI Tape Recording — Requirements

> Records the shipped TUI slice on a checked-in VHS tape with header pins, a manifest entry and a normalized transcript. A slice of the docs-site-demos FR-3 follow-up. Normative source: mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md, version 2.0; § numbers, I-IDs, G-IDs and AT-IDs refer to it. Historical Revision 1.1 pins (§14.9, §22.3, §15) keep their original meaning per docs/spec/README.md.

## Context

- **Backlog card**: MH-32
- **Issue**: from a card; no issue filed.
- **Problem**: The docs site and README prove the scripted demo with a reproducible terminal recording, but the shipped TUI slice has no recording at all: `docs/demos/tui.tape` is a comment-only scaffold with zero `Run` lines, the manifest names only the demo recording, and no TUI transcript or artifact exists. The docs-site-demos design §5 promised the site would show the word "planned" where the TUI recording goes, yet no rendered site page does — the finalize review kept that gap as a suggestion to resolve when the real recording lands rather than churning a placeholder.
- **Contract**: docs-site-demos FR-3/AC-3.1–AC-3.3 and design §5 define the tape contract this spec executes: transcript-reproducible regeneration from scripted fixtures, scope limited to exactly the tui-slice acceptance-covered behaviour (AC-3.2/I14), and the TUI's own verification status on screen instead of pane text as proof (AC-3.3/I17). The recording-honesty checks (manifest revision, transcript freshness, capability qualifiers) already run in CI for the demo recording; two of the three are demo-hardcoded and must generalize.
- **Grounding verdicts** (2026-10-05, main @ 176fead):
  - Historical R1.1 recording anchor (§§14.9, 22.3): PARTIAL — neither section names recordings; §14.9 demands zero-credential scripted examples (`docs/spec/master-spec.md:1499-1503`), §22.3 demands outputs with verification evidence (`docs/spec/master-spec.md:2212-2216`); the operative recording contract is docs-site-demos FR-3 plus design §5.
  - docs-site-demos FR-3/AC-3.2/AC-3.3 and design §5: HOLDS (`specs/done/docs-site-demos/requirements.md:64-68`, `specs/done/docs-site-demos/design.md:217-227`).
  - Current contract v2 §15 via W05: PARTIAL — v2 §15 defines the CLI/TUI journeys (`MYTHHELM_Master_Spec_v2.md:644-695`), W05 demands recorded journey evidence before claims (`MYTHHELM_Implementation_Plan.md:111`); the tape is this spec's chosen evidence form, not a v2-dictated one.
  - Shipped TUI scope (mission view, layout ladder to linear mode, keyboard flows, own verification status): HOLDS (`specs/done/tui-slice/requirements.md:47-95`, AC-1.1/AC-7.2 met per `specs/done/tui-slice/retrospective.md:39,61`); tui-slice AC-3.1 is partial (exit-dialog rows inert, issue #106) and AC-6.2 is partial (history search unwired) per `specs/done/tui-slice/retrospective.md:47,59` — both excluded from tape scope.
  - Tape shape (header pins, manifest entry with qualifiers, normalized transcript): HOLDS — demo precedent at `docs/demos/demo.tape`, `docs/demos/manifest.json`, `docs/demos/demo.transcript.txt`, `docs/demos/normalize.sed`, `docs/demos/record.sh`.
  - No full TUI tape exists: HOLDS — `find . -name "*.tape"` returns only demo/tui tapes; `grep -c "^Run " docs/demos/tui.tape` is 0; manifest `recordings` names demo only; no TUI artifact or transcript in `docs/demos/`.
  - Acceptance can fail today: HOLDS — "manifest names a tui recording" and "tui.tape carries Run lines" both fail on the current tree.
  - MH-8 → MH-32 transfer on record: HOLDS — card filed in PR #141, reconciliation in PR #146, finalize in PR #148 (all merged).

### Objectives

- **O1**: A contributor runs one documented record command and regenerates the TUI recording — tape, rendered artifact and normalized transcript — reproducibly from scripted fixtures, with no paid credentials and no network access.
- **O2**: The recording shows only TUI behaviour the tui-slice acceptance covers, with the TUI's own verification status on screen, and every shown capability names the versioned test result that qualifies it.
- **O3**: The docs site and README present the real TUI recording with provenance, replacing the "planned" gap the docs-site-demos review left open.

### Non-Goals (and what still binds)

| # | Out of scope | Still binding in this spec |
|---|---|---|
| N1 | Recording TUI behaviour beyond tui-slice acceptance (exit-dialog detach/stop rows per issue #106, history-search UI, untested screen-reader combinations). | AC-3.2/I14: anything not covered stays out of the tape; the tape never implies more than the qualifiers prove. |
| N2 | Render-compare CI (frame/perceptual-hash binding of artifact to tape). | Reconciliation §4 residual stands: transcript + manifest revision + human review of the artifact in the PR is the binding. |
| N3 | New TUI features or fixture modes beyond what the tape needs to drive the shipped UI deterministically. | G09/§15: the tape drives the shipped TUI; any fixture it needs is the minimum for determinism, not a feature. |
| N4 | macOS/Windows VHS recording legs. | NFR-2 analog: Linux timing is measured; other platforms stay undisclosed, never assumed. |

## Functional Requirements

Acceptance criteria use EARS. Tags in brackets name the invariants, gates and sections each one serves.

### FR-1 — Full TUI tape (docs-site-demos FR-3, v2 §15, W05, G10)

- **AC-1.1** [FR-3/AC-3.1, W05 journey evidence, v2 §15.2] When a contributor runs the documented record command, the checked-in TUI tape shall regenerate the TUI recording transcript-reproducibly — the same commands, outputs and exit codes — from scripted fixtures with no paid credentials and no network access.
- **AC-1.2** [AC-3.2/I14] The tape shall drive the mission view, the responsive layout ladder down to the linear accessible mode, and the keyboard flows (arrows, `/` filter, `:` palette, `?` help, `Enter`/`Escape`), and shall show nothing outside the met tui-slice acceptance; the inert exit-dialog rows (issue #106) and the unwired history search shall not appear as working behaviour.
- **AC-1.3** [AC-3.3/I17] The recording shall show the TUI's own verification status bound to its candidate revision on screen; it shall not present terminal text or pane status as proof of completion.

### FR-2 — Tape keeps the honesty contract (AC-4.1/AC-4.2, I07, I14)

- **AC-2.1** [AC-4.1/I07] The tape header shall declare `Binary-Version:`, `Binary-Commit:` and `VHS-Version:` pins; the manifest entry shall declare the same binary revision, and CI shall fail a recording whose transcript drifts from a regen at the declared revision (transcript only per N2 — the rendered artifact is bound by human review, not byte-compare).
- **AC-2.2** [AC-4.2/I14] The manifest entry shall name every shown capability and the versioned test result qualifying each one; CI shall fail a recording whose declaration is missing a qualifier for a shown, unmarked capability, and shall pass only qualifiers that pass at the declared revision.
- **AC-2.3** [AC-2.1 analog, I09] A normalized transcript shall sit beside the tape; the record command shall regenerate it from the same binary and the verify pass shall byte-match a re-run, with the binary's own labels kept verbatim (I09).

### FR-3 — Recording is published (design §5 claim, G10)

- **AC-3.1** [§15.2 journey evidence] The docs site shall embed the rendered TUI recording with a provenance caption naming the tape and the record command, in the user guide beside the demo recording.
- **AC-3.2** [AC-2.3 analog, G10] The README beside the demo embed and `docs/demos/README.md` shall list the TUI recording with its tape, artifact, transcript and pinned revisions; no "planned" wording shall remain where the recording now exists.

## Non-Functional Requirements

- **NFR-1** [NFR-2 analog, W16] The full record command shall complete in under 10 minutes on Linux using free tooling only (`go`, `git`, `python3`, pinned VHS); the implementing task records the measured time.
- **NFR-2** [D5 analog, G10] If the rendered artifact exceeds ~5 MB, the implementer reports back and a smaller format becomes a follow-up; the tape still merges.

## Definition of Done

- [ ] AC-1.1 to AC-3.2 each have a named test or CI check; CI green on Linux, macOS and Windows.
- [ ] `tui.tape` carries `Run` lines; manifest, transcript and rendered artifact exist and agree on pins.
- [ ] The docs-site-demos review suggestion on the "planned" slot is closed by the real recording.
- [ ] No TUI behaviour outside met tui-slice acceptance appears as working in the recording.

## Open Questions

1. **Rendered artifact filename and format?** Blocks tape content and embeds. Options: (a) `docs/demos/tui.gif` per D4 side-by-side + D5 GIF-in-tree; (b) MP4/WebM if the TUI loop exceeds the ~5 MB GIF budget. Default: (a). Until the follow-up renders one, no filename is referenced from `docs/` or `README.md` (I14; the Task 7 handoff cited a grep check for this, but no such check exists in `.github/workflows/docs.yml` or `record.sh` — drift to confirm or drop in /spec).
2. **Does driving the TUI need a binary fixture or flag?** Blocks task scoping (a tui- or core-domain task joins if so, per the fixture's path: `internal/tui/` is tui, `cmd/` is core). Options: (a) VHS keystrokes against the existing launch paths including linear/plain modes, no code change; (b) a minimal determinism/scripted-session fixture in the TUI or CLI. No default yet — /spec investigates against `internal/tui/` and the launch matrix.
3. **One declared revision for all recordings?** Blocks manifest design. Options: (a) re-record demo and TUI together at one new commit, keeping the `len(pins) == 1` assertion in `docs.yml:82`; (b) per-recording pins with a check change. Default: (a) — weakening a shipped honesty check needs its own rationale.
4. **What is the TUI transcript?** Blocks FR-2 transcript design. Options: (a) normalized stdout of the taped commands with `normalize.sed` extended for TUI escape sequences; (b) the keystroke script plus a final-state dump (linear/plain rendering) as the stable proof; (c) linear-mode outputs driven alongside the interactive takes. No default yet — /spec investigates against VHS output behaviour.
5. **Which pages carry the embed?** Blocks FR-3. Options: (a) user-guide Quickstart beside the demo embed plus the README beside the demo embed; (b) a dedicated demos page. Default: (a), matching the AC-2.3 precedent.

## Dependencies

- **Builds on: `docs-site-demos` (MH-8, `specs/done/docs-site-demos/`, shipped 2026-10-05)** — the tape contract (FR-3/AC-3.x, design §5), the record script, the manifest schema and the three CI honesty checks; this spec generalizes the demo-hardcoded parts (`record.sh` tape/transcript/command pins, `docs.yml` revision + transcript checks).
- **Builds on: `tui-slice` (MH-2, `specs/done/tui-slice/`, shipped 2026-10-04)** — the recorded subject; scope is exactly its met acceptance, excluding the AC-3.1 exit-dialog partial (issue #106) and the AC-6.2 unwired-search partial.
- **Supersedes**: the scaffold header in `docs/demos/tui.tape` and the "planned" TUI row in `docs/demos/README.md`. **Conflicts**: none known; `specs/todo/` and `specs/in-progress/` are empty.

## Impacted components

- `docs/demos/tui.tape` (docs domain): scaffold becomes the full tape with header pins and `Run` lines.
- `docs/demos/manifest.json` (docs domain): new `tui` recording entry with binary/VHS pins and capability qualifiers.
- `docs/demos/` (docs domain): new normalized transcript + rendered artifact; `README.md` TUI row flips from planned to recorded.
- `docs/user-guide.md` + `README.md` (docs domain): TUI recording embeds with provenance captions.
- `docs/demos/record.sh`, `docs/demos/normalize.sed` (docs domain): generalize beyond the demo recording (tape/transcript selection, transcript normalization for TUI output).
- `.github/workflows/docs.yml` (release domain): generalize the revision and transcript checks from `recordings[0]`/demo-command to each recording.
- Possible `internal/tui/` fixture (tui domain) or `cmd/` fixture (core domain), read-only unless Open Q2 says otherwise: only if the tape cannot drive the shipped UI deterministically.
