# docs-site-demos — v2 reconciliation (D-27)

Focused reconciliation of this unfinished Revision 1.1 spec with the adopted
v2 contract, per D-27 ("unfinished Revision 1.1 specs require reviewed
reconciliation before implementation resumes"). Reviewed by merging this file
through a pull request; implementation (Task 6 merge, finalize) resumes after.

## 1. Revision 1.1 record preserved

No acceptance ID is renamed, dropped or re-scoped. Every criterion keeps its
R1.1 meaning and its merged evidence:

| Criterion | R1.1 requirement (short) | Evidence |
|---|---|---|
| AC-1.1 | site published on each main merge with user/contributor and repository guides | Task 2, PR #138; Task 3, PR #139 |
| AC-1.2 | licence, security, contribution, changelog, limitations from nav | Task 2, PR #138 |
| AC-1.3 | contribution path, no paid subscription | Task 2, PR #138 |
| AC-1.4 | no MYTHHELM-owned runtime service; tests and binaries do not fetch from the site | Task 3, PR #139 |
| AC-2.1 | demo tape reproducible from transcript | Task 4, PR #125 (`record.sh`, byte-identical regen) |
| AC-2.2 | estimated or unknown values retain the binary's labels | Task 4, PR #125 |
| AC-2.3 | README + docs site embed the GIF with provenance | Task 5, PR #140 |
| NFR-1 | GitHub-native, pinned, least-privilege, fork-safe | Task 3, PR #139 |
| NFR-2 | record under 10 minutes, free tooling | Task 4, PR #125 (11–21s runs) |
| AC-3.1/3.2/3.3 | TUI recording (landed-TUI precondition holds) | Scaffold merged Task 7, PR #137; full tape is card MH-32 (PR #141, pending merge) |
| AC-4.1 | tapes declare the exact binary revision; CI rejects stale recordings | Task 4 manifest, PR #125; Task 6 CI checks pending (PR #142, held for this file) |
| AC-4.2 | capability declarations have versioned test qualifiers; CI rejects missing qualifiers | Task 4 manifest, PR #125; Task 6 CI checks pending (PR #142, held for this file) |

Shipped tasks (1, 2, 3, 4, 5, 7) stand as R1.1 evidence. The spec's
`requirements.md`, `design.md` and `tasks.md` are unchanged by this file.

## 2. v2 mapping

MH-8's card source is already v2 (`Master Specification v2 §§1, 15, 17–18;
W05/W16`). The remaining work maps as follows:

- Task 6 (recording-honesty CI: transcript freshness, manifest revision,
  capability qualifiers) → W16 evidence for the docs subset: machine-checked
  proof that published recordings match their declared revisions.
- Finalize (retrospective, review, PM sync) → closes the R1.1 record; the
  shipped MH-8 becomes historical evidence, same standing as MH-1/MH-2/MH-9.
- MH-32 (FR-3 follow-up: full TUI tape, filed in PR #141, pending merge) →
  W05 user-journey coverage for the shipped TUI, bound by AC-3.2/AC-3.3
  (covered behaviour only, verification status on screen, never pane text
  as proof). The TUI recording stays in MH-8's completion scope until that
  transfer is on record; finalize runs after both PRs merge.

## 3. Qualification clarification

Nothing in MH-8 establishes native-route qualification, and nothing in its
record claims to. Concretely:

- The scripted demo runs the fake adapter with local-scripted billing; the
  transcript, manifest and GIF prove the *recording mechanism* is
  reproducible, not that any native harness executes correctly.
- The Jekyll build, mirror drift check and Task 6 qualifier assertions are
  fixture/local coverage of docs infrastructure.
- Per Master Specification v2 §7.1 (`mythhelm-synthesis/
  MYTHHELM_Master_Spec_v2.md`), qualification is whole-bundle and per-target
  across all seven targets, with `fixture-tested` explicitly distinct from
  `live-qualified`. This is a summary, not the complete §7.1 key — see §7.1
  for all required dimensions (native surface, observed model/snapshot,
  effort/speed, tooling digests, workspace/environment class, host-attachment
  record). Native-route qualification lives in MH-10/MH-12/MH-4/MH-29
  (W02/W14), untouched by this spec.
- Per D-27's rationale, shipped slices and walkthroughs cannot stand in for
  native qualification or human UX evidence; this spec's UX evidence stays
  where the record puts it (§4).

## 4. Retained limitations

All disclosed limitations carry forward unchanged:

- TUI: scaffold only — `docs/demos/tui.tape` has no `Run` lines, no manifest
  entry, no recording image; the site labels it planned (N1/I14).
- Accessibility: linear accessible mode and semantic structure ship;
  screen-reader behaviour is experimental (no screen-reader session exists).
- Recording binding: the GIF-to-tape binding stays transcript + manifest
  revision + human review (delivery Q-10, kept residual); no render-compare CI.

## 5. Completion scope

Shipping MH-8 completes the docs site and the scripted demo recording. It
does not ship the v2 S1 product: S1 needs W01–W05 with applicable W14 rows
plus W16 evidence (adoption map §W16/S1), and MH-8 covers only its W05/W16
slices. W01–W04, the W14 support rows and the other W16 owners remain open.

Card note: MH-8 reads `specced` while this run is in progress; it moves to
shipped through the normal lifecycle sync when finalize merges.
