## TUI Tape Recording — Design

> Implements `specs/*/tui-tape-recording/requirements.md` (MH-32). Normative source: mythhelm-synthesis/MYTHHELM_Master_Spec_v2.md, version 2.0; § numbers, I-IDs, G-IDs and AT-IDs refer to it. W-IDs refer to the waves in mythhelm-synthesis/MYTHHELM_Implementation_Plan.md.

### 1. Current state

- `docs/demos/tui.tape` is a comment-only scaffold (zero `Run`/`Type` lines); the manifest (`docs/demos/manifest.json`) names only the `demo` recording; no TUI transcript or artifact exists. The "planned" TUI row is `docs/demos/README.md:32-39`.
- The demo precedent this spec generalizes: `docs/demos/demo.tape` (Type/Enter/Sleep; `TERM=dumb` forces the linear run under VHS's tty at `:26-29`; fixed Sleep, never Wait+Screen, at `:32-36`), `docs/demos/record.sh` (build + transcript regen + `vhs` render + verify; demo-hardcoded tape/transcript/GIF at `:37-41`, transcript pipeline at `:89`/`:106`, `SCRIPTED DEMO` label assertion at `:110`, VHS pin from `recordings[0]` at `:123`, header re-pin demo-only at `:148`; `--repin` already moves all manifest pins at `:140-142`), `docs/demos/normalize.sed` (9 rules), and the manifest schema (`name`/`tape`/`artifact`/`transcript`/pins/`capabilities[]` with one `qualifier` each).
- `.github/workflows/docs.yml` `checks` job: manifest-revision presence from `recordings[0]`; demo transcript regen in a detached worktree at the pin with the checked-in `normalize.sed`, diffed; per-capability `^--- PASS: <qualifier>` assertion over all recordings with a `len(pins) == 1` assertion (never bare `-run`, never `grep -q`).
- VHS v0.12.1 (matching the manifest pin), probed 2026-10-06: no `Run` directive exists (Output/Require/Set/Sleep/Type/Keys/Display only, per `vhs new` example docs) — the repo's "Run lines" means executable directive lines (`^(Run|Type) `, per the docs-site-demos Task 4 precedent); multiple `Output` lines render only the last, silently; mid-tape `Set` is ignored with a warning; `stty cols` mid-tape is honored (85→60→150 verified on rendered frames); `vhs validate` exits 0/1 on tape syntax. Capture size is fixed per tape (1000px/16px font = 85 columns, probed).
- The TUI launches on a TTY: `selectLaunch` (`internal/cli/tui.go:122-135`) sends `demo --check pass` to `runLiveTUI` (`internal/cli/tui.go:223`) unless `TERM=dumb`/piped/`--plain`/`--format jsonl`; `--accessible` (`internal/cli/tui.go:66`, "linear screen-reader stream instead of the TUI") wins even on a TTY. Keys (`internal/tui/nav.go:21-65`): only `ctrl+c` quits; `q` opens the exit dialog (inert rows, issue #106 — never pressed on tape); `esc` backs out one level and never quits; arrows, `/`, `:`, `?`, `enter` all mapped — VHS can send every required key. The compact view offers linear as on-screen text: "compact view — small terminal (--accessible for linear)" (`internal/tui/mission.go:386`). The accessible stream's stable labels (`internal/cli/accessible.go:210-229`): `goal:`, `run state:`, `attempt`, `candidate:`, verification, `next action:`, `actions:`.
- Recorded-subject scope (`specs/done/tui-slice/retrospective.md:39-61`): everything met except AC-3.1 partial (exit-dialog rows inert, #106) and AC-6.2 partial (history search unwired) — both excluded from the tape. Qualifier candidates verified passing at HEAD 2026-10-06: `TestMissionShowsRequiredFacts`, `TestLayoutBreakpoints`, `TestShortHeightCompact`, `TestResizePreservesIdentity`, `TestKeyFlows`, `TestPaletteSearchable`, `TestHelpFullNames`, `TestFilterNeverHidesDetail`, `TestEscapeNeverQuits`, `TestNativeResultNeverVerified`, `TestVerificationMismatchLabelsRevisions`, `TestLaunchRuleMatrix`, `TestAccessibleOrderedStream` (+ `TestAccessibleLabels`, `TestAccessibleNoChatter`).
- Embed sites: `README.md:38-39` (beside the demo embed, before `## TUI status`), `docs/user-guide.md:37-38` (Quickstart, beside the demo embed), `docs/demos/README.md` recordings table. `docs/index.md:18-19` also embeds the demo; it stays demo-only (D6).
- The Task 7 handoff's "no-recording-image check" exists nowhere (`docs.yml` and `record.sh` verified 2026-10-06): drift, dropped (D8).

### 2. Tape: takes and drive script (FR-1, v2 §15, W05)

One tape, one artifact (`docs/demos/tui.gif`), rung-per-launch. The TUI owns all input while it runs, so a rung change cannot be typed mid-session; each rung is a fresh launch at its `stty cols` width, which also matches how a user opens the TUI at that size. Rung widths sit inside their ladder bands (wide ≥140, two-pane ≥100, single ≥80, else compact — `internal/tui/layout.go:9-11`): 140 (wide, AC-2.1), 120 (two-pane, AC-2.2), 90 (single, AC-2.3), 70 (compact, AC-2.4).

Takes, in order (all commands via the record-built `/tmp/mythhelm-record` binary, never a bare `vhs` render):

1. **Wide mission view + keyboard flows** (`stty cols 140`): launch `demo --check pass`, hold the mission view (goal, next action, activity, candidate diff, verification status) for a readable beat; then arrows move selection, `/` types a filter + `Enter` applies + `Escape` clears, `:` opens the palette + arrows move + `Escape` closes, `?` opens help + `Escape` closes, `Enter` opens details + `Escape` backs out; `ctrl+c` quits. `q` is never pressed (exit dialog excluded); history search is never shown.
2. **Two-pane** (`stty cols 120`): launch, hold, `ctrl+c`.
3. **Single** (`stty cols 90`): launch, hold, `ctrl+c`.
4. **Compact + linear offer** (`stty cols 70`): launch, hold on the compact view with the `--accessible for linear` offer text visible, `ctrl+c`.
5. **Linear accessible mode**: `demo --check pass --accessible` runs non-interactively to exit 0, streaming the state labels — the linear endpoint the compact rung offers.

Each rung's `tput cols` runs visible in-frame right after its `stty` (rung-width proof on screen). Fixed `Sleep` margins everywhere (never Wait+Screen: VHS screen-text matching sees the first screenful only — demo.tape precedent). `Set Framerate` is set explicitly as the size lever (default motion kept per D10); `Set Width`/`FontSize` are calibrated at implementation so rung 1 reaches 140 columns (1000px/16px = 85 probed — expect ~1650px at 16px or a smaller font). `vhs validate` gates the tape in `record.sh verify`, plus a single-`Output` assertion (multiple outputs render only the last, silently — probed trap). "Run lines" (requirements DoD) means executable directive lines: `grep -cE '^(Run|Type) '` ≥ 1 plus reviewer eyeball of every directive line, mirroring the docs-site-demos Task 4 acceptance (VHS has no `Run` directive — D7). The no-network tripwire is a static class matcher (`curl|wget|ssh |http`) plus that mandatory eyeball; the residual — a network-capable line outside the enumerated classes, e.g. `git ls-remote` — is covered by the eyeball step, which confirms each line invokes only the record-built local binary.

### 3. Transcript pipeline (FR-2, AC-2.3/I09)

The transcript is linear-mode outputs driven alongside the interactive takes (Q4 option (c)): interactive bytes are escape- and timing-dependent and can never byte-match a re-run, so the transcript proves the same binary's same-scenario states through its stable linear paths, and human review binds GIF to transcript (N2's binding).

Exact pipeline (run by `record.sh`, mirrored in `docs.yml`). Each take is its own pipeline with its own `PIPESTATUS` assertion — one brace-group pipeline would expose only the last take's status and mask the first take's failure:

```sh
TERM=dumb "$BIN" demo --check pass 2>&1 | sed -E -f docs/demos/normalize.sed >"$tmp.t1"
test "${PIPESTATUS[0]}" -eq 0
"$BIN" demo --check pass --accessible 2>&1 | sed -E -f docs/demos/normalize.sed >"$tmp.t2"
test "${PIPESTATUS[0]}" -eq 0
{
  echo "=== take: linear (TERM=dumb) ==="
  cat "$tmp.t1"
  echo "=== take: accessible (--accessible) ==="
  cat "$tmp.t2"
} > "$TRANSCRIPT"
```

The final redirect targets `$TRANSCRIPT` (the manifest transcript path for this recording): `record.sh` runs at the repository root, and §5 compares that same path — a bare `tui.transcript.txt` would land at the root. Each take's `$BIN` status asserted (never `sed`'s — `PIPESTATUS` per pipeline, demo precedent); the verify pass re-runs the pipeline from the same binary and byte-matches (`diff`). `sed` rules are line-local, so per-take normalization is identical to whole-transcript normalization. `normalize.sed` gains TUI/linear rules derived at implementation by diffing repeated runs (ULIDs, `run_`/`att_`, pids, SHAs already covered); the binary's own labels stay verbatim (I09): `SCRIPTED DEMO` banners plus the accessible `goal:`/`run state:`/`attempt`/`candidate:`/`next action:`/`actions:` labels, with per-label `grep -c` assertions using exact counts fixed from observed output (exact counts, not floors — with two takes emitting more than any single-take floor, a floor cannot discriminate a deleted label line). The `TERM=dumb` take overlaps the demo transcript's content by construction — each recording's proof is self-contained.

### 4. record.sh generalization (FR-1/FR-2)

`record.sh` keeps its interface (`[--repin]`) and grows a per-recording loop driven by the manifest (no schema change — D9):

- Tape/artifact/transcript paths and pins read per recording from `manifest.json` (replacing the `recordings[0]` reads and the hardcoded `TAPE`/`TRANSCRIPT`/`GIF`).
- A per-name transcript function: `demo` keeps the existing pipeline verbatim; `tui` runs the §3 pipeline with its per-take status assertions. Per-name label assertions likewise (`SCRIPTED DEMO` for demo; accessible labels for tui).
- The VHS-pin check asserts every recording's `vhs_version` equals the installed renderer.
- `--repin` moves every recording's manifest pins (already does) and every recording's tape-header pins (extended loop; today demo-only).
- Verify gains `vhs validate` per tape and the single-`Output` assertion per tape.
- The recording-inputs dirt list (`*.go`, module files, tape, sed, embeds) extends to the tui tape; the "refusing to write artifacts another revision labels" guard compares per recording.

Q3 default (a) holds: one declared revision — landing re-records demo and TUI together at one new commit (the `len(pins) == 1` assertion stays).

### 5. docs.yml generalization (FR-2, AC-2.1/AC-2.2)

The `checks` job's three recording steps generalize per recording (same probe-tested shapes as the docs-site-demos Task 6 acceptance):

- **Revision**: every recording's `binary_commit` is 40-hex and present (`git cat-file`); the `len(pins) == 1` assertion stays (Q3-a).
- **Transcript freshness**: per recording — demo keeps its existing regen-at-pin step verbatim; tui gains the §3 pipeline regen (per-take status assertions included) in the same detached worktree, diffed against `docs/demos/tui.transcript.txt`.
- **Qualifiers**: unchanged code — it already iterates all recordings; the new manifest capabilities flow through it.

New `uses:` (if any) pinned SHA + `# vX.Y.Z`; no `secrets.*` (fork-safe lane).

### 6. Manifest entry (FR-2, AC-2.1/AC-2.2/I07/I14)

One `tui` entry beside `demo`: `tape docs/demos/tui.tape`, `artifact docs/demos/tui.gif`, `transcript docs/demos/tui.transcript.txt`, pins set at implementation via `record.sh --repin` (same commit as the re-recorded demo — one revision), capabilities each with one qualifier (all verified passing at HEAD; CI re-verifies at the pin):

| Shown capability | Qualifier |
|---|---|
| mission view | `TestMissionShowsRequiredFacts` |
| keyboard flows | `TestKeyFlows` |
| command palette | `TestPaletteSearchable` |
| context help | `TestHelpFullNames` |
| responsive layouts | `TestLayoutBreakpoints` |
| revision-attached verification | `TestVerificationMismatchLabelsRevisions` |
| linear accessible stream | `TestAccessibleOrderedStream` |

Tape-header pins mirror the demo header (`Binary-Version:`/`Binary-Commit:`/`VHS-Version:` comments).

### 7. Embeds (FR-3, G10)

After the mechanism lands: `README.md` gains the TUI embed + provenance caption (tape path + record command) beside the demo embed; `docs/user-guide.md` Quickstart gains the same (page-relative `demos/tui.gif`, demo precedent); `docs/demos/README.md` flips the TUI table row from planned to recorded (tape, artifact, transcript, pins) and extends the capability table with the §6 rows. No "planned" wording remains where the recording exists (anchored to the recordings table). `docs/index.md` stays demo-only (D6).

### 8. Decisions

| ID | Decision | Rationale |
|---|---|---|
| D1 | Ladder via rung-per-launch `stty cols` steps (140/120/90/70 + `--accessible` take) | One VHS tape has one capture size and mid-tape `Set` is ignored (probed); the TUI owns all input while running, so mid-session resize is untypable. Rejected: multi-tape (breaks the singular tape/entry/transcript contract the requirements and manifest schema assume). |
| D2 | Transcript = linear takes alongside the interactive takes (Q4-c) | Interactive bytes (escapes, timing) can never byte-match a re-run. Rejected: (a) normalizing interactive stdout (brittle, timing-dependent); (b) keystroke script + final dump (the script half is self-confirming — it proves the tape matches itself). |
| D3 | One declared revision; landing re-records demo + TUI together (Q3-a) | Keeps the shipped `len(pins) == 1` honesty assertion. Rejected: per-recording pins (weakens a shipped check without its own rationale). |
| D4 | `docs/demos/tui.gif` (Q1-a) | Demo precedent (D4 side-by-side + D5 GIF-in-tree); NFR-2 analog is the overflow path (report back, MP4 follow-up), not a pre-emptive format change. |
| D5 | VHS keystrokes against existing launch paths, no code change (Q2-a) | Every required key exists in both `nav.go` and VHS; the demo scenario is already scripted with the fake adapter; only `ctrl+c` quits so sessions are controllable. Fallback: if implementation proves flake, a minimal fixture becomes a recorded deviation + follow-up — not silent scope. |
| D6 | `docs/index.md` stays demo-only | The refined requirements scope FR-3 to the user guide + README; the front page keeps its single hero recording. Rejected: include (scope creep past the Ready requirements; a follow-up can promote the TUI). |
| D7 | "Run lines" = executable directive lines (`^Run ` / `^Type ` + eyeball) | VHS v0.12.1 has no `Run` directive (probed); mirrors the docs-site-demos Task 4 acceptance and its recorded deviation. A literal-`Run` reading is unimplementable. |
| D8 | Task 7's grep-check note dropped | The check exists in neither `docs.yml` nor `record.sh` (verified); the discipline it wanted is moot — the artifact lands in the same change that first names it. |
| D9 | `record.sh` per-name transcript functions; no manifest schema change | Two transcript pipelines do not justify schema churn; paths/pins already live in the manifest. Rejected: `transcript_cmd` in the manifest (executable content in a data file the checks parse). |
| D10 | Default motion in the tape | The tape shows the shipped default honestly. Rejected: `--motion reduced` to fit size (misrepresents the default experience); size is managed by framerate/holds, then NFR-2. |
| D11 | Fixed `Sleep` margins, never Wait+Screen | VHS screen-text matching sees the first screenful only (demo.tape precedent, verified there). |

### 9. Honesty register

| Spec demand | Position |
|---|---|
| AC-1.1 transcript-reproducible regeneration | Met for the transcript pipeline (§3 byte-match); the GIF is encoder-nondeterministic by nature (demo precedent) and bound by revision + transcript + human review instead. |
| AC-1.2 full ladder on screen | Met via rung-per-launch traversal (D1); the live resize transition itself (AC-2.6 behavior) is not shown — rungs are fresh launches, stated in the tape comments. |
| AC-2.1/AC-2.2 CI drift/qualifier failure | Met: transcript-only compare (refined AC-2.1); rendered artifact bound by PR human review, never byte-compare (N2). |
| N1 excluded partials (exit dialog #106, history search) | Excluded: `q` never pressed, history search never shown; the tape comments name the exclusions. |
| W05 recorded journey evidence beyond the scripted demo | Partially met: the tape evidences the demo journey only. First-launch, cross-harness and later journeys have no recording; claiming them would need their own tapes. |
| N4 other-platform recording legs | Not met by design: Linux timing measured; other platforms undisclosed, never assumed. |

### 10. Cross-spec references

- Builds on: `specs/*/docs-site-demos/` (MH-8, shipped) — the tape contract, `record.sh`, manifest schema, `docs.yml` checks; this spec generalizes the demo-hardcoded parts.
- Builds on: `specs/*/tui-slice/` (MH-2, shipped) — the recorded subject, read-only; scope is exactly its met acceptance.
- Builds on: `specs/*/dogfood-slice/` (MH-1, shipped) — the `mythhelm demo` scenario the tape drives.
- Conflicts: none. `specs/todo/`, `specs/in-progress/` and `specs/unfinalized/` are empty; the refined siblings (`version-numbering`, `release-tagging`, `strict-lint-set`) are disjoint by subject and files (release-process records, lint config — none touches `docs/demos/`, `README.md`, `docs/user-guide.md` or `docs.yml`'s recording steps).
