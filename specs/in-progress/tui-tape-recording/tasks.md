## TUI Tape Recording — Tasks

### Dependencies

- Prerequisite specs: `specs/*/docs-site-demos/` (shipped; the tape contract, `record.sh`, manifest schema, `docs.yml` checks this spec generalizes); `specs/*/tui-slice/` (shipped; the recorded subject, read-only); `specs/*/dogfood-slice/` (shipped; `mythhelm demo`, the scenario the tape drives).
- Order: Task 1 first (the generalized record contract + TUI tape contract). Tasks 2 and 3 after Task 1, in parallel (disjoint Files).
- **Gates for every task.** No Go code changes, so the Go gates are skipped with that reason. Docs tasks (1, 2): `scripts/ci/check-public-hygiene.sh`. Release task (3): `go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12` locally, `zizmor` in CI, plus `scripts/ci/check-public-hygiene.sh`. Files existing or an agent reporting success is not completion; cite the runner output.
- **Completion convention.** Append ` ✅ COMPLETED` to the heading, keep every original field, and add `Implementation` (at most 3 lines, plus the commit SHA), `Spec deviations` ("None" or a justification) and `Files modified`.
- **Commits.** Every commit is signed off (`git commit -s`, DCO). No decision record needed: no billing, persistence, process-ownership or public-contract change (docs + workflow generalization only; manifest schema unchanged).

---

## Implementation Tasks

### Task 1 — TUI tape, transcript, manifest + record.sh generalization ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Change**: Write the TUI tape (design §2 takes), its transcript pipeline (design §3), the manifest entry (design §6), and generalize `record.sh`/`normalize.sed` per recording (design §4), keeping the demo leg byte-identical.
- **Files**:
  - `docs/demos/tui.tape` (takes per design §2 + header pins)
  - `docs/demos/tui.transcript.txt` (design §3 pipeline output)
  - `docs/demos/manifest.json` (tui entry per design §6; re-pinned with demo at one commit)
  - `docs/demos/record.sh` (per-recording loop, per-name pipelines, per-tape validate)
  - `docs/demos/normalize.sed` (TUI/linear rules added; demo rules untouched)
- **Produces**: Record contract — `record.sh [--repin]` drives every manifest recording (paths/pins from the manifest; transcript pipeline per recording name; `vhs validate` + single-`Output` assertion per tape); TUI tape contract (header pins, take order, rung widths). Tasks 2 and 3 consume these verbatim.
- **Acceptance**:
  - Build (`go build`, never `go run`) and run the design §3 pipeline twice: `diff` of the two runs exits 0, and each take's `$BIN` status is asserted 0 via its own pipeline's `PIPESTATUS` (never `sed`'s status). Counterfactual: the same takes with `--check fail` exit 5 and produce bytes that `diff` non-zero against the transcript (a scratch run proving the transcript discriminates pass from fail; a first take forced to fail trips its own assertion, never masked by the second). The transcript carries the binary's labels verbatim, asserted as exact counts fixed from observed output (design §3): `grep -c 'SCRIPTED DEMO'` prints exactly the observed N — recorded in the entry, not assumed (the demo precedent emits 4 from a single take and this pipeline runs two, so a floor cannot discriminate deletion) — and `grep -c '^goal: '`, `'^run state: '`, `'^attempt'`, `'^candidate: '`, `'^next action: '`, `'^actions: '` each print exactly their observed counts (`^attempt` covers both `attempt: none recorded` and `attempt N: ...` per `accessibleAttemptLine`); deleting any label line fails its check.
  - The tape invokes only the local record-built binary: `grep -E 'curl|wget|ssh |http' docs/demos/tui.tape` prints nothing, and `grep -cE '^(Run|Type) '` prints at least 1 (non-empty leg). The exit dialog is never driven: `grep -c '^Type "q"$' docs/demos/tui.tape` prints 0. The reviewer confirms every directive line by eye (launch commands, rung `stty`/`tput` pairs, key flows, no `q`, no history search) and records it in the completion entry.
  - `manifest.json` parses (`python3 -c json.load` exits 0); both entries' `binary_commit` are equal 40-hex commits present in the repo, and the tui transcript regenerates identically from a worktree at that exact commit; both `vhs_version` equal `vhs --version` on the recording machine (record both outputs in the entry). Every tui capability qualifier names a test that passes at the pin (run the §6 set; a qualifier renamed to `TestDoesNotExist` fails).
  - The full record (`docs/demos/record.sh` end to end: build + both transcript regens + both renders + verify) completes in under 10 minutes timed with `time` (NFR-1); only free tooling installed (list versions in the entry). Demo regression leg: `git status --porcelain docs/demos/demo.transcript.txt` prints nothing (byte-identical regen); `git diff -U0 docs/demos/demo.tape | grep -E '^[+-]' | grep -vE '^[+-]{3}' | grep -vE '^[+-]# (Binary-Version|Binary-Commit): '` prints nothing (header pins only); and the demo manifest entry differs from HEAD only in pins (`git show HEAD:docs/demos/manifest.json`, then python3 asserts `recordings[0]` equal modulo `binary_commit`/`binary_version`). The demo GIF is excluded — encoder-nondeterministic bytes; breaking the demo pipeline changes its transcript and fails this leg.
  - Verify has teeth: a scratch tape with a syntax error fails `vhs validate` in verify, and a scratch tape with two `Output` lines fails the single-`Output` assertion (both fixtures recorded in the entry, then removed).
  - `scripts/ci/check-public-hygiene.sh` passes.
- **Test plan**: shell/JSON/VHS checks run locally; the Go qualifiers named in the manifest already exist and pass (`internal/tui/*_test.go`, `internal/cli/accessible_test.go`).
- **Invariants touched**: I09 (v2 §2: transcript keeps the binary's labels verbatim, never collapsed); I07 (v2 §2: manifest pins the exact recorded revision); I14 (v2 §2: shown capabilities name their qualifying tests); I17 (v2 §2: the tape shows the TUI's own verification status, never pane text as proof).
- **Status**: ✅ Completed — TUI tape, transcript, manifest entry and generalized record.sh landed with the demo re-recorded at one revision; PR #163.
- **Implementation**: Pins `09fb91081cf71d96e3f75b9144241872fc4d70e0` / `v0.0.0-20261006120359-09fb91081cf7` (both recordings) via `record.sh --repin`; label counts fixed from observed output (SCRIPTED DEMO 8; `^goal: `/`^run state: `/`^candidate: `/`^next action: `/`^actions: ` 1; `^attempt` 7); `vhs --version` → `vhs version v0.12.1` (manifest `v0.12.1`); full record `real 3m29.724s` on go1.27.1/git 2.43.0/vhs 0.12.1/ttyd 1.7.7-40e79c7/ffmpeg 6.1.1-3ubuntu5/file 5.45/python3 3.12.10. Live-stream races found by diffing 6+ runs (sampled goal/run-state/admission values, 20-hex truncated SHA, race-window `receipt.written` + `next-after` 21/22) normalized by 5 fail-closed appended sed rules — demo rules untouched, take-1 output identical under old/new sed; every tape directive reviewed by eye (4 local-binary launches + 1 accessible launch, rung stty/tput pairs 140/120/90/70, arrow/filter/palette/help/details flows, Ctrl+C quits, no `q`, no history search, no network-capable line). Verify teeth: `Output teeth.gif` + `Frobnicate now` → `vhs validate` exit 1; two-`Output` tape (validate 0) → single-`Output` assertion exit 1 (fixtures removed). Commits 09fb91081cf71d96e3f75b9144241872fc4d70e0, 80546ae6954991ff12c965a0ff7cb6a53f99a7eb.
- **Spec deviations**: None to the contract. `docs/demos/demo.tape` is outside Files: header-pin lines only (one-revision re-record moves every tape's pins per design §4/D3; the acceptance's header-only leg verifies no other line changed).
- **Files modified**: `docs/demos/tui.tape`, `docs/demos/tui.transcript.txt`, `docs/demos/manifest.json`, `docs/demos/record.sh`, `docs/demos/normalize.sed`, `docs/demos/demo.tape`, `specs/in-progress/tui-tape-recording/tasks.md`, `specs/in-progress/tui-tape-recording/handoff.md`, `specs/in-progress/tui-tape-recording/scratchpad.md`.

### Task 2 — Rendered TUI GIF and embeds

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 1
- **Change**: Render `tui.gif` from the Task 1 tape and embed it with provenance captions in the README and user guide, flipping the demos README row from planned to recorded (design §7).
- **Files**:
  - `docs/demos/tui.gif` (rendered from the Task 1 tape via `record.sh`)
  - `README.md` (TUI embed + caption beside the demo embed)
  - `docs/user-guide.md` (TUI embed + caption in Quickstart, page-relative path)
  - `docs/demos/README.md` (TUI row recorded; capability table extended)
- **Acceptance**:
  - `file docs/demos/tui.gif` reports `GIF image data`; size is over 10 KB and under ~5 MB (over: non-trivial recording; under: NFR-2 revisit threshold — if over, the implementer reports back and the smaller format becomes a follow-up instead of forcing the tape under).
  - The README caption names `docs/demos/tui.tape` and the `docs/demos/record.sh` command within 3 lines of the embed (anchored; deleting the tape path from the caption fails the check).
  - `docs/user-guide.md` carries the same embed and caption with the page-relative `demos/tui.gif` path (same anchored check; the README keeps the root-relative path).
  - `docs/demos/README.md` lists the TUI recording with its tape, artifact, transcript and pinned revisions, plus the design §6 capability rows; no "planned" wording remains in the recordings-table section (anchored between the `## Recordings` heading and the next `##` heading; the historical scaffold notes elsewhere in the file may still say planned).
  - Frame spot-check: the GIF shows the mission view with verification status, at least one rung change with its in-frame `tput cols`, and the linear `--accessible` take (note the timestamps inspected in the entry).
  - `scripts/ci/check-public-hygiene.sh` passes.
- **Test plan**: `file`/`stat`/anchored `grep`; visual spot-check of the rendered GIF per the item above.
- **Invariants touched**: I09 (v2 §2: the GIF renders the same taped run the transcript proves); I14 (v2 §2: only met tui-slice acceptance appears as working — no exit-dialog rows, no history search).

### Task 3 — Recording-honesty-check generalization in CI

- **Domain/agent**: release-engineer
- **Budget**: standard
- **Depends on**: Task 1 (consumes the Task 1 tape contract; parallel with Task 2 — disjoint Files)
- **Change**: Generalize the `checks` job's revision, transcript-freshness and qualifier steps from the demo recording to every manifest recording (design §5).
- **Files**:
  - `.github/workflows/docs.yml` (per-recording revision + transcript steps; qualifiers unchanged)
  - `docs/automation.md` (Docs-site row names the new per-recording failure modes, one sentence)
- **Acceptance**:
  - `actionlint` passes; `zizmor` passes in CI on the PR; new `uses:` (if any) pinned to 40-char SHA with `# vX.Y.Z`; no `secrets.*` reference (`grep -c 'secrets\.'` prints 0).
  - Stale-transcript probe: on a scratch branch, append one line to `docs/demos/tui.transcript.txt`; the `checks` job fails naming the tui transcript diff. Record the run URL, then drop the branch.
  - Unqualified-capability probe: on a scratch branch, add a tui manifest capability with `qualifier: TestDoesNotExist`; the `checks` job fails because no `--- PASS: TestDoesNotExist` line appears. Record the run URL, then drop the branch.
  - Unknown-revision probe: on a scratch branch, set the tui `binary_commit` to 40 zeros; the `checks` job fails naming the revision. Record the run URL, then drop the branch.
  - On the PR itself the `checks` job is green (all three probes fail only on their scratch branches; the demo legs stay green throughout).
  - `docs/automation.md`'s Docs-site row names the new per-recording failure modes in one sentence (anchored: the sentence sits in the Docs-site row; deleting the sentence fails the grep). The `Charm VHS demo GIFs` row moves from the Deferred table to the Active table (its "First TUI" trigger is met by FR-3 landing here): `sed -n '/^## Active/,/^## Deferred/p' docs/automation.md | grep -c 'Charm VHS demo GIFs'` prints 1 and `sed -n '/^## Deferred/,/^## Maintainer/p' docs/automation.md | grep -c 'Charm VHS'` prints 0.
- **Test plan**: three scratch-branch CI probes + green PR run; paste all four job URLs in the completion entry.
- **Invariants touched**: I07 (v2 §2: verification attaches to the declared revision; stale fails); I14 (v2 §2: unqualified capabilities fail); fork-safe checks (no secrets, read-only contents — the `docs.yml` fork-safe lane).
