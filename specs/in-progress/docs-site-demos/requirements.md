## Documentation Site and Scripted Terminal Demos — Requirements

> A published docs site carrying the user and contributor guides extracted from the spec, plus reproducible Charm VHS terminal recordings of the demo and the TUI for the docs and README. A slice of master spec §14.9, §19.3 and §22.3 and the G10 public-release gate. Normative source: docs/spec/master-spec.md; § numbers, I-IDs and G-IDs refer to it.

## Context

- **Backlog card**: MH-8
- **Source issue**: [#29 — Documentation site and scripted terminal demos (MH-8)](https://github.com/turbokast/mythhelm/issues/29)
- **Source**: master spec §14.9, §19.3 and §22.3; the GitHub Pages and VHS rows of the Deferred table in docs/automation.md

MYTHHELM's guides currently live as Markdown in the repository (README.md, CONTRIBUTING.md, SECURITY.md, GOVERNANCE.md, docs/spec/master-spec.md) with no published site: there is no Pages workflow in `.github/workflows/`, no site generator config under `docs/` or `.github/`, and no VHS tape files anywhere in the tree. The Deferred table in `docs/automation.md` already names both halves of this work — "GitHub Pages docs" triggered by the first user-facing guide, and "Charm VHS demo GIFs" triggered by the first TUI — but neither trigger has been acted on.

The card asks for two deliverables. First, published user and contributor guides extracted from the spec: the G10 gate (§18.7) requires licence, security policy, contribution path, provenance, changelog and limitations to be published, and §19.3 requires published governance and a contribution path whose good-first-issue route needs no paid subscriptions. Most of that prose exists at the root already (LICENSE, SECURITY.md, CONTRIBUTING.md, CHANGELOG.md); what is missing is the published site that assembles it. Second, scripted, reproducible terminal recordings of the demo and the TUI, made with Charm VHS, embedded in the docs and README.

The two recordings split on reachability. The demo half can be recorded today: `mythhelm demo` exists (`internal/cli/demo.go`) and runs against a scripted fake adapter, so a VHS tape driving it has a failing counterfactual now. The TUI half sequenced behind the `tui-slice` spec (`specs/done/tui-slice/`, MH-2), which shipped 2026-10-04; the full TUI tape is filed as a follow-up by Task 7 while this run merges the scaffold; see Dependencies.

Grounding verdicts (2026-10-02; rule: `.claude/rules/spec-premise-grounding.md`):

- §14.9 covers SDK, examples and contributor experience — HOLDS (`docs/spec/master-spec.md:1497`).
- §19.3 covers governance and contribution — HOLDS (`docs/spec/master-spec.md:2059`).
- §22.3 covers required implementation outputs — HOLDS (`docs/spec/master-spec.md:2212`).
- The Deferred table has a GitHub Pages docs row — HOLDS (`docs/automation.md:73`).
- The Deferred table has a Charm VHS demo GIFs row — HOLDS (`docs/automation.md:74`).
- Card is Stage 1 under §20.3 — HOLDS (`docs/spec/master-spec.md:2107`).
- Release gate is G10 — HOLDS (`docs/spec/master-spec.md:1947`, "G10 — Public release").
- The recordings depend on the TUI slice (MH-2, `specs/*/tui-slice/`) — HOLDS for the TUI recording: `specs/done/tui-slice/` shipped 2026-10-04 and the recording is a Task 7 follow-up (re-grounded 2026-10-05; was "sequences behind landing" with no `tui` package).
- The demo recording does not wait on the TUI — HOLDS: `mythhelm demo` exists (`internal/cli/demo.go:48`) and runs scripted, so a tape driving it fails today (no tapes exist) and passes after.
- No docs site exists — HOLDS (`.github/workflows/` holds 13 workflows, none Pages-related; no mkdocs/jekyll/docusaurus config under `docs/` or `.github/`).
- No VHS tapes exist — HOLDS (`find . -name '*.tape'` returns nothing; "vhs" appears only in `docs/automation.md:74`, orchestration logs and stale worktrees).
- The backlog was seeded from the master spec per D-1 — HOLDS (`product/decisions.md:20`).

### Objectives

- **O1**: A user can read the published user and contributor guides on the docs site without cloning the repository (§19.3, G10).
- **O2**: A contributor can reproduce every terminal recording in the docs and README from a checked-in tape and scripted fixtures, with no paid credentials (§22.3, G01).
- **O3**: The README and docs show the demo, and once the TUI lands, the TUI, as honest recordings of the real binary — never mock-ups (§14.9, I14).

### Non-Goals (and what still binds)

| # | Out of scope | Still binding in this spec |
|---|---|---|
| N1 | The TUI itself (§15). Owned by `tui-slice` (MH-2). | I14: the site must not present TUI recordings or screenshots until the TUI they show is qualified; a placeholder is labelled as planned, not shown as shipped. |
| N2 | New guide prose beyond extraction and assembly (tutorials, man pages). | G10: licence, security policy, contribution path, changelog and limitations are published even if brief. |
| N3 | A custom domain, analytics, or comments on the docs site. | I13: the product works without MYTHHELM-owned network services; the site is documentation, never a runtime dependency. |
| N4 | Package-manager publishing and release signing (other Deferred rows). | §19.4: provenance and SBOM claims stay out of the site until those rows land. |

## Functional Requirements

Acceptance criteria use EARS. Tags in brackets name the invariants, gates and sections each one serves.

### FR-1 — Published docs site (§19.3, G10)

- **AC-1.1** [G10] The system shall publish a docs site on every merge to `main` that carries the user guide (installation, quickstart/demo, limitations) and the contributor guide (governance, contribution path, DCO sign-off) extracted from the master spec (§14.9, §19.3, §22.3) and the repository guides (README.md, CONTRIBUTING.md, SECURITY.md, CHANGELOG.md, GOVERNANCE.md).
- **AC-1.2** [G10] When the site is built, it shall include the licence, security policy, contribution path, changelog and limitations register, each reachable from the site navigation.
- **AC-1.3** [§19.3] When a contributor follows the published contribution path, they shall reach the DCO sign-off requirement and a good-first-issue route that needs no paid subscription.
- **AC-1.4** [I13] The site build and hosting shall require no MYTHHELM-owned network service at user runtime; the repository's tests and binaries shall not fetch from the site.

### FR-2 — Scripted demo recording (§14.9, §22.3)

- **AC-2.1** [§22.3] When a contributor runs the documented record command, the checked-in VHS tape shall regenerate the demo recording transcript-reproducibly — the same commands, outputs and exit codes — from scripted fixtures with no paid credentials and no network access.
- **AC-2.2** [I09] If the demo run reports estimated or unknown values, then the recording shall show them with the labels the binary prints, never collapsed to facts or zeros.
- **AC-2.3** [§14.9] The README and the docs site shall embed the demo recording with a caption naming the tape path and the command that regenerates it.

### FR-3 — Scripted TUI recording (§15, §22.3; sequences behind `tui-slice`)

- **AC-3.1** [§22.3] When a contributor runs the documented record command after the TUI slice has landed, a checked-in VHS tape shall regenerate the TUI recording transcript-reproducibly — the same commands, outputs and exit codes — from scripted fixtures with no paid credentials and no network access.
- **AC-3.2** [I14] The TUI recording shall be published only for TUI behaviour the `tui-slice` spec acceptance covers; anything else stays out of the tape.
- **AC-3.3** [I17] The tape shall not present Herdr pane status or terminal text as proof of completion; the recording shows the TUI's own verification status.

### FR-4 — Recordings stay honest (I07, I14)

- **AC-4.1** [I07] Each tape shall declare the exact binary revision (version and commit) it records; when a recording is regenerated or checked, CI shall verify the recording was produced from that declared revision, and a stale recording fails the check.
- **AC-4.2** [I14] Each recording shall declare the featured capabilities it shows and the versioned test result qualifying each one; if a shown capability has no versioned test result, then the docs shall mark it experimental or remove it, and CI shall fail a recording whose declaration is missing a qualifier for a shown, unmarked capability.

## Non-Functional Requirements

- **NFR-1** [§19.4] The site build runs in the fork-safe CI lane: no release or provider credentials are reachable from the build job.
- **NFR-2** [§22.3] Recording regeneration completes on a stock contributor machine (Linux and macOS) in under 10 minutes with only free tooling installed.

## Definition of Done

- [ ] AC-1.1 to AC-1.4 each have a named check; the site URL is linked from the README.
- [ ] AC-2.1 to AC-2.3 each have a named check; the demo recording plays from the README and the site.
- [ ] AC-3.1 to AC-3.3 each have a named check, or are explicitly deferred to the TUI slice's landing with the tape scaffold merged.
- [ ] AC-4.1 and AC-4.2 each have a named check in CI.
- [ ] No new required network service; `gate.sh` green.

## Open Questions

1. **Which site generator?** Blocks FR-1 design. Options: (a) GitHub-native Jekyll/Pages (matches the Deferred row's "GitHub-native" tier, least new tooling); (b) MkDocs or Docusaurus (richer nav/versioning, more dependencies); (c) plain checked-in HTML (no build, manual assembly). Default if unanswered: (a).
2. **Where do tapes and recordings live?** Blocks FR-2/FR-3 layout. Options: (a) `docs/demos/` with tapes and rendered GIFs side by side; (b) tapes in `docs/demos/`, rendered artifacts as CI-built release attachments. Default: (a).
3. **GIF or video format?** Blocks FR-2/FR-3 acceptance. VHS renders GIF and MP4; GIFs embed everywhere but balloon repository size. Options: (a) GIF in tree; (b) MP4/WebM in tree; (c) render in CI and host via Pages/repositories. Default: (a) for short loops, revisit on size.
4. **Which demo scenario does the first tape drive?** Blocks FR-2 tape content. Options: (a) `mythhelm demo --check pass` end to end; (b) `demo` plus a failing `--check fail` recovery beat; (c) `doctor` output as well. Default: (a), with (b) if it fits one short recording.
5. **Does enabling Pages need operator action?** Blocks rollout: repository settings and environments are the operator's actions. If Pages source/visibility needs admin setup, the spec's rollout task must request it rather than assume a workflow alone suffices.

## Dependencies

- **Builds on (FR-3 recording follow-up): `tui-slice` (MH-2, `specs/done/tui-slice/`, shipped 2026-10-04)** — Task 7 drafts the backlog card for the full FR-3 tape as a follow-up. FR-1 and FR-2 have no TUI dependency and ship first.
- **Builds on: `dogfood-slice` (`specs/done/dogfood-slice/`, MH-1)** — the `mythhelm demo` command and plain/JSONL output the FR-2 tape drives.
- **Coordinate with: `openssf-badge` (MH-9, `specs/done/openssf-badge/`, shipped)** — also touches §19.3 and the G10 gate; it shipped first, so this spec reconciles the shared sentences it assembles.
- **Supersedes**: none. **Conflicts**: none known.

## Impacted components

- `docs/` (docs domain): new site sources and guides assembly; `docs/demos/` (new) for VHS tapes and recordings — cites the Deferred rows at `docs/automation.md:73-74`.
- `.github/workflows/` (release domain): new Pages build-and-deploy workflow — fork-safe lane per §19.4 (`docs/spec/master-spec.md:2071`).
- `README.md`, `CONTRIBUTING.md`, `SECURITY.md`, `CHANGELOG.md` (docs domain): sources the site extracts; README gains recording embeds.
- `internal/cli/demo.go` (core domain, read-only): the scripted demo the FR-2 tape drives; no code change expected unless the tape needs a determinism flag.
- No new Go packages expected; no `internal/tui/` or `mods/` changes in this spec (owned by `tui-slice`).
