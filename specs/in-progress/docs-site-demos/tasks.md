## Documentation Site and Scripted Terminal Demos — Tasks

### Dependencies

- Prerequisite specs: `specs/*/dogfood-slice/` (shipped; `mythhelm demo` the FR-2 tape
  drives); `specs/*/tui-slice/` (shipped 2026-10-04; FR-3's full tape is filed as a
  follow-up by Task 7). `specs/*/openssf-badge/` shipped first; this spec reconciles
  shared §19.3/G10 sentences per design §7 (its README edit is disjoint from the badge line).
- Order: Task 1 and Task 4 start in parallel (disjoint Files). Then Task 2 after 1;
  Task 3 after 2; Task 5 after 4 and 2 (it edits Task 2's `docs/user-guide.md`);
  Task 6 after 3 and 5 (shares `.github/workflows/docs.yml` with Task 3 — never in
  parallel with it); Task 7 after 4. Parallel groups: {1, 4}; {2, 7} after their
  prerequisites (disjoint Files); {3, 5} after theirs.
- **Gates for every task.** No Go code changes, so the Go gates are skipped with that
  reason. Docs tasks: `scripts/ci/check-public-hygiene.sh`. Release tasks (3, 6):
  `go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12` locally, `zizmor` in CI,
  plus `scripts/ci/check-public-hygiene.sh`. Files existing or an agent reporting success
  is not completion; cite the runner output.
- **Completion convention.** Append ` ✅ COMPLETED` to the heading, keep every original
  field, and add `Implementation` (at most 3 lines, plus the commit SHA),
  `Spec deviations` ("None" or a justification) and `Files modified`.
- **Commits.** Every commit is signed off (`git commit -s`, DCO). No decision record
  needed: no billing, persistence, process-ownership or public-contract change.

---

## Implementation Tasks

### Task 1 — Site scaffold: config, nav, layout, home ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Change**: Create the Jekyll scaffold under `docs/` per design §2 so later tasks have
  pages to fill and the workflow has a source directory to build.
- **Files**:
  - `docs/_config.yml` (minima theme, title/description; nav data file reference)
  - `docs/_data/navigation.yml` (Home, User guide, Contributing, Security policy, Changelog, Licence, Limitations)
  - `docs/_layouts/default.html` (minima override: nav include, build-revision footer)
  - `docs/index.md` (home stub with links; Task 5 adds the demo embed)
- **Produces**: Site contract — `docs/` is the Jekyll source root; `_data/navigation.yml`
  is the nav; authored guide pages are top-level `docs/*.md` routes and root-guide
  mirrors live under `docs/mirror/`.
- **Acceptance**:
  - `_config.yml` parses as strict YAML (`python3 -c yaml.safe_load` exits 0; a scratch
    edit inserting a tab-indented line makes it exit non-zero).
  - `navigation.yml` names all seven routes above; `for r in $(sed -n 's/^ *path: //p'
    docs/_data/navigation.yml); do test -f "docs/$r" || echo "MISSING $r"; done` prints
    nothing once Task 2 lands (before Task 2 it lists exactly the six non-home routes;
    a route pointing at `docs/nope.md` prints `MISSING nope.md`).
  - `docs/index.md` starts with the design §2 front matter block: line 1 is `---`, line 2
    is `layout: default`, line 3 starts with `title: ` (deleting line 2 fails the check).
  - No `http` URL in the scaffold points at a MYTHHELM-owned host: `grep -rEo
    'https?://[^"'\'' )]+' docs/_config.yml docs/_data docs/_layouts docs/index.md`
    prints only `github.com/turbokast/mythhelm` and Pages-default hosts, and the grep
    prints at least one line (non-empty leg).
  - `scripts/ci/check-public-hygiene.sh` passes.
- **Test plan**: shell/YAML checks run locally; no Go tests (no Go files).
- **Invariants touched**: I13 (§3.3: no MYTHHELM-owned network service; the scaffold links nowhere else).
- **Status**: ✅ Completed — Jekyll scaffold (config, seven-route nav, layout override, home stub) landed; PR #124.
- **Implementation**: Minima config with Pages `url`/`baseurl`; nav names all seven routes with `path` relative to `docs/` (the six non-home files land in Task 2); self-contained layout override with nav loop and build-revision footer; home stub links the guides and the repo. Commit 6bbe9fe3a6f9bdfa010a8cc3121749eb183ff53c.
- **Spec deviations**: None.
- **Files modified**: `docs/_config.yml`, `docs/_data/navigation.yml`, `docs/_layouts/default.html`, `docs/index.md`, `specs/in-progress/docs-site-demos/tasks.md`, `specs/in-progress/docs-site-demos/handoff.md`.

### Task 2 — Guide pages and root-guide mirrors ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 1
- **Change**: Author the user guide, contributing and limitations pages extracted from the
  master spec, and mirror the four root guides so the site carries every G10 item.
- **Files**:
  - `docs/user-guide.md` (installation, quickstart/demo, limitations pointer)
  - `docs/contributing.md` (governance summary, contribution path, DCO, good-first-issue route)
  - `docs/limitations.md` (limitations register: pre-alpha, narrow adapter coverage — scripted fake + Claude Code — TUI shipped with known limits)
  - `docs/mirror/LICENSE.md` (byte mirror of `LICENSE`)
  - `docs/mirror/SECURITY.md` (byte mirror of `SECURITY.md`)
  - `docs/mirror/CHANGELOG.md` (byte mirror of `CHANGELOG.md`)
  - `docs/mirror/GOVERNANCE.md` (byte mirror of `GOVERNANCE.md`)
  - `docs/_data/navigation.yml` (wire the new routes into the nav)
- **Acceptance**:
  - Each mirror is byte-identical to its root source (`cmp LICENSE docs/mirror/LICENSE.md`
    and the three others all exit 0; deleting the last line of any mirror makes its `cmp`
    exit non-zero). No mirror carries front matter in the tree: `head -1` of each mirror
    differs from `---`.
  - Each authored page (`user-guide.md`, `contributing.md`, `limitations.md`) starts with
    the design §2 front matter block: line 1 `---`, line 2 `layout: default`, line 3
    `title: …` (deleting line 2 of any page fails the check).
  - `docs/contributing.md` contains a `Signed-off-by` mention and a `good first issue`
    link inside its contribution-path section (anchored: both matches fall between the
    `## Contribution path` heading and the next `##` heading; moving either line above the
    heading fails the check).
  - `docs/limitations.md` names the TUI as shipped, not planned (`grep -ci
    'tui.*shipped\|tui.*available'` prints at least 1; `grep -ci 'tui.*planned\|no tui
    yet'` prints 0).
  - The task's pages and mirrors carry no provenance/SBOM claim: `grep -rEli
    'provenance|sbom' docs/user-guide.md docs/contributing.md docs/limitations.md
    docs/mirror/` prints nothing (scoped to this task's files — `docs/automation.md`,
    `docs/decisions/`, `docs/harness/` and `docs/spec/` legitimately mention them).
  - `scripts/ci/check-public-hygiene.sh` passes.
- **Test plan**: `cmp`/`grep` checks run locally; the drift check that keeps the mirrors
  honest lands in Task 6.
- **Invariants touched**: I14 (§9.14: shipped TUI described truthfully with its limits;
  no unshipped capability advertised); I13 (§3.3: pages link to no MYTHHELM-owned service).
- **Status**: ✅ Completed — guide pages and root-guide mirrors landed; PR #138.
- **Implementation**: Three authored pages (user guide, contributing with anchored DCO/good-first-issue, limitations naming the TUI shipped) plus four `cp` byte mirrors; nav needed no edit (Task 1 pre-wired all seven routes; loop prints nothing). Commit 7898e7ea23b444c0b1f3256b815debcf1d073bc5.
- **Spec deviations**: None to the contract. `navigation.yml` listed in Files but required no change; Governance mirror linked from `contributing.md` per design §2's seven-item nav. Review round touched two files outside Files with justification: root `GOVERNANCE.md` (one-line MAINTAINERS link made absolute — the mirror must stay byte-identical to root, so the broken relative link could only be fixed at the source) and `specs/in-progress/docs-site-demos/design.md` (one-line adapter-coverage wording fix matching the task-record fix).
- **Files modified**: `docs/user-guide.md`, `docs/contributing.md`, `docs/limitations.md`, `docs/mirror/LICENSE.md`, `docs/mirror/SECURITY.md`, `docs/mirror/CHANGELOG.md`, `docs/mirror/GOVERNANCE.md`, `specs/in-progress/docs-site-demos/tasks.md`, `specs/in-progress/docs-site-demos/handoff.md`, plus review-round `GOVERNANCE.md` (absolute MAINTAINERS link) and `specs/in-progress/docs-site-demos/design.md` (adapter-coverage wording).

### Task 3 — Pages build-and-deploy workflow ✅ COMPLETED

- **Domain/agent**: release-engineer
- **Budget**: standard
- **Depends on**: Task 2
- **Change**: Add `.github/workflows/docs.yml` with `checks` (mirror drift only for now),
  `build` (Jekyll) and `deploy` (Pages) jobs, and move the Pages row to Active.
- **Files**:
  - `.github/workflows/docs.yml`
  - `docs/automation.md` (move the "GitHub Pages docs" row from Deferred to Active)
- **Produces**: Workflow contract — jobs `checks`, `build` (needs `checks`), `deploy`
  (needs `build`, `push` to `main` only); Task 6 adds recording steps to `checks`.
- **Acceptance**:
  - `actionlint` passes; `zizmor` passes in CI on the PR.
  - Every `uses:` is pinned to a 40-character SHA with a `# vX.Y.Z` comment, checked by
    `test "$(grep -cE 'uses: .+@[0-9a-f]{40} #' .github/workflows/docs.yml)" -eq
    "$(grep -c 'uses: ' .github/workflows/docs.yml)"` (a scratch edit replacing one SHA
    with its tag fails the equality).
  - Top-level `permissions:` is `contents: read`; only `deploy` carries `pages: write`
    and `id-token: write`, each with a why-comment; no job references `secrets.*`
    (NFR-1 fork safety — `grep -c 'secrets\.' .github/workflows/docs.yml` prints 0).
  - The `build` job succeeds in CI on the PR and uploads the Pages artifact; before this
    task no `docs.yml` run exists.
  - The `build` job runs the design §4 "Inject mirror front matter" step before the
    Jekyll build (`grep -c 'Inject mirror front matter' .github/workflows/docs.yml`
    prints 1), then asserts the mirrors rendered: `_site/mirror/LICENSE.html`,
    `_site/mirror/SECURITY.html`, `_site/mirror/CHANGELOG.html` and
    `_site/mirror/GOVERNANCE.html` all exist (a scratch rename of `docs/mirror/` to
    `docs/_mirror/`, or deleting the inject step, fails this step — the AC-1.2
    built-site proof).
  - The PR description tells the maintainer to set Pages source to "GitHub Actions" and
    to add the `Docs checks` job as a required check in the branch ruleset.
- **Test plan**: CI run on the PR; paste the job URLs in the completion entry.
- **Invariants touched**: I13 (§3.3: Pages is documentation hosting, never a runtime
  dependency — no repo test or binary fetches the site); §19.4 (pinned, least-privilege,
  fork-safe CI).
- **Status**: ✅ Completed — Pages workflow (checks/build/deploy) and automation.md row move landed; PR #139.
- **Implementation**: `checks` (mirror drift with refresh hint), `build` (inject + Jekyll + assert mirrors rendered), `deploy` (push-to-main only); 6/6 `uses:` pinned SHA + comment; `contents: read` top-level, `pages`/`id-token: write` on deploy only with why-comments, 0 `secrets.*`; actionlint exit 0; `Docs checks` green (run 37293185337); build green after Pages enablement: `Build docs site` pass 18s at https://github.com/turbokast/mythhelm/actions/runs/37294219848/job/111711576805 (run 37294219848, re-run on entry push). `docs/automation.md` moves the Pages row Deferred → Active; PR description carries the maintainer Pages-source + ruleset instructions. Commit b0c7277ddf2ccc648c6ac79eadd44f5dddcb4093.
- **Spec deviations**: None. Entry + handoff written by the orchestrator from verified evidence (worker stopped at the Pages-disabled build failure, which needed the maintainer switch).
- **Files modified**: `.github/workflows/docs.yml`, `docs/automation.md`, `specs/in-progress/docs-site-demos/tasks.md`, `specs/in-progress/docs-site-demos/handoff.md`.

### Task 4 — Demo tape, transcript and manifest ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: None (parallel with Task 1; disjoint Files)
- **Change**: Add the VHS tape driving `mythhelm demo --check pass`, its normalized
  transcript, the manifest declaring the binary revision, and the record-command docs.
- **Files**:
  - `docs/demos/demo.tape`
  - `docs/demos/demo.transcript.txt`
  - `docs/demos/normalize.sed`
  - `docs/demos/manifest.json`
  - `docs/demos/record.sh` (the record command per design §3)
  - `docs/demos/README.md` (the documented record command + recording table)
- **Produces**: Tape contract — header comments `Binary-Version:`/`Binary-Commit:`/
  `VHS-Version:`; manifest schema per design §3; `docs/demos/normalize.sed` verbatim per
  design §3 (temp paths, `run_`/`att_` ULIDs, both pid shapes, protected empty-sha256,
  40-hex SHAs, 12-hex evidence prefixes). Tasks 5 and 6 consume these verbatim.
- **Acceptance**:
  - `docs/demos/normalize.sed` is byte-identical to the design §3 block (modulo the
    reviewer's fix, recorded as a deviation); building the binary (`go build`, never
    `go run`) and re-running the §3 pipeline, then `diff`ing against
    `demo.transcript.txt`, exits 0, and the taped binary's own exit status is 0
    (the §3 `PIPESTATUS` assertion — never `sed`'s status). Running it with
    `--check fail` instead exits 5 and diffing exits non-zero (failing
    counterfactual: the transcript discriminates pass from fail).
  - The tape's `Run`/`Type` lines invoke only the local `mythhelm` binary:
    `grep -E 'curl|wget|ssh |http' docs/demos/demo.tape` prints nothing, and
    `grep -cE '^(Run|Type) ' docs/demos/demo.tape` is at least 1 (non-empty leg).
    Covered classes and residual per design §3; the reviewer confirms each `Run`/`Type`
    line by eye and records it in the completion entry.
  - `manifest.json` parses (`python3 -c json.load` exits 0); its `binary_commit` is a
    40-hex commit present in the repo, and the transcript regenerates identically
    from a worktree at that exact commit (design §4 steps 2–3); its `vhs_version`
    equals `vhs --version` on the recording machine (record both outputs in the
    completion entry).
  - The transcript carries the binary's own labels: `grep -c 'SCRIPTED DEMO'`
    prints at least 4 (banner per screen — `demo.go:80-82`).
  - The full record (`docs/demos/record.sh` end to end: build + transcript regen +
    `vhs` render + verify) completes in under 10 minutes timed with `time` (NFR-2);
    only free tooling installed (list versions in the entry).
  - `scripts/ci/check-public-hygiene.sh` passes.
- **Test plan**: shell/JSON checks; the Go qualifiers named in the manifest already exist
  (`internal/cli/demo_test.go`).
- **Invariants touched**: I09 (§13.4: transcript keeps the binary's labels verbatim,
  never collapsed); I07 (§11.5: manifest pins the exact recorded revision);
  I14 (§9.14: declared capabilities name their qualifying tests).
- **Status**: ✅ Completed — tape, transcript, manifest and record command landed; PR #125.
- **Implementation**: Tape drives `/tmp/mythhelm-record demo --check pass` via one `Type` line
  (VHS v0.12.1 has no `Run`); manifest pins `binary_commit e0ecdbc9…`, verified identical
  from a detached worktree; full `record.sh` in 20.9s. Commit a9fd10af562ea913650f20910c2cd99aff515bef.
- **Spec deviations**: None to the contract. Tape uses `Type`+`Enter` (no `Run` in VHS v0.12.1);
  `binary_version` records the clean-tree `mythhelm version` string, not the schema example's `devel`.
  Tape `Run`/`Type` lines confirmed by eye: the single `Type` line invokes only the record-built
  local binary; `Require git` is the demo's documented prerequisite. `vhs --version` on the
  recording machine prints `vhs version v0.12.1`; manifest stores `v0.12.1`. Recorder tooling,
  all free: go1.27.1, git 2.43.0, vhs v0.12.1, python3 3.12.10, ffmpeg 6.1.1, ttyd 1.7.7.
- **Files modified**: `docs/demos/demo.tape`, `docs/demos/demo.transcript.txt`, `docs/demos/normalize.sed`, `docs/demos/manifest.json`, `docs/demos/record.sh`, `docs/demos/README.md`, `specs/in-progress/docs-site-demos/tasks.md`, `specs/in-progress/docs-site-demos/handoff.md`.

### Task 5 — Rendered demo GIF and embeds ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 4, Task 2
- **Change**: Render `demo.gif` from the Task 4 tape and embed it with its caption in the
  README and the site, plus the published site URL in the README.
- **Files**:
  - `docs/demos/demo.gif`
  - `README.md` (demo embed + caption + site URL link)
  - `docs/user-guide.md` (demo embed + caption)
- **Acceptance**:
  - `file docs/demos/demo.gif` reports `GIF image data`; size is over 10 KB and under
    ~5 MB (over: non-trivial recording; under: design D5 revisit threshold).
  - The README caption names `docs/demos/demo.tape` and the `docs/demos/record.sh`
    command within 3 lines of the embed (anchored; deleting the tape path from the
    caption fails the check).
  - `docs/user-guide.md` carries the same embed and caption (same anchored check).
  - The README links the published site URL once (`https://turbokast.github.io/mythhelm/`
    or the actual Pages URL recorded in the completion entry).
  - `scripts/ci/check-public-hygiene.sh` passes.
- **Test plan**: `file`/`stat`/anchored `grep`; visual spot-check that the GIF shows the
  demo's SCRIPTED DEMO screens (note the timestamp frames inspected in the entry).
- **Invariants touched**: I09 (§13.4: the recording shows the binary's labels — the GIF
  renders the same taped run the transcript proves); I14 (§9.14: only demo behaviour with
  versioned qualifiers is shown).
- **Status**: ✅ Completed — rendered demo GIF with README and user-guide embeds plus the site URL landed; PR #140.
- **Implementation**: GIF (1080x620, 1027454 bytes, 8.64s) rendered via `record.sh --repin` in a throwaway worktree at the manifest pin (transcript regen byte-identical; tape/manifest/transcript untouched); identical captions name the tape + `record.sh` adjacent to each embed; site URL linked once. Frames inspected at t=2 (command typing) and t=4/6/8 (`=== SCRIPTED DEMO: done ===`, checks passed, diff). Commit 1a68d74e7d879fe9198e45b38efbeb5b1b0429ac.
- **Spec deviations**: User-guide embed is page-relative (`demos/demo.gif`) vs the README's root-relative path so it resolves on the site and on GitHub. Review round added the `docs/index.md` home-page embed (same captioned embed): design §2 and Task 1's Files both assign it to Task 5 even though Task 5's own Files list omits the file.
- **Files modified**: `docs/demos/demo.gif`, `README.md`, `docs/user-guide.md`, `specs/in-progress/docs-site-demos/tasks.md`, `specs/in-progress/docs-site-demos/handoff.md`, plus review-round `docs/index.md` (home-page embed per design §2).

### Task 6 — Recording honesty checks in CI ✅ COMPLETED

- **Domain/agent**: release-engineer
- **Budget**: standard
- **Depends on**: Task 3, Task 5 (shares `.github/workflows/docs.yml` with Task 3; never in parallel)
- **Change**: Extend the `checks` job with transcript freshness, manifest-revision and
  capability-qualifier steps. Stands alone from Task 3 because it consumes the Task 4/5
  tape contract, which lands after Task 3's prerequisites, and keeps deploy reviewable
  separately.
- **Files**:
  - `.github/workflows/docs.yml`
- **Acceptance**:
  - `actionlint` passes; `zizmor` passes in CI on the PR; new `uses:` (if any) pinned to
    40-char SHA with `# vX.Y.Z`; no `secrets.*` reference (`grep -c` prints 0).
  - Stale-transcript probe: on a scratch branch, append one line to
    `docs/demos/demo.transcript.txt`; the `checks` job fails naming the transcript diff.
    Record the run URL, then drop the branch.
  - Unqualified-capability probe: on a scratch branch, add a manifest capability with
    `qualifier: TestDoesNotExist`; the `checks` job fails because no
    `--- PASS: TestDoesNotExist` line appears (a bare `-run` would exit 0 — design §4).
    Record the run URL, then drop the branch.
  - Unknown-revision probe: on a scratch branch, set `binary_commit` to 40 zeros; the
    `checks` job fails naming the revision. Record the run URL, then drop the branch.
  - On the PR itself the `checks` job is green (all three probes fail only on their
    scratch branches).
- **Test plan**: three scratch-branch CI probes (dogfood Task 3 pattern) + green PR run;
  paste all four job URLs in the completion entry.
- **Invariants touched**: I07 (§11.5: verification attaches to the declared revision;
  stale fails); I14 (§9.14: unqualified capabilities fail); §19.4 (fork-safe checks).
- **Status**: ✅ Completed — honesty checks (revision, transcript freshness, qualifiers) in `Docs checks` landed; PR #142.
- **Implementation**: `checks` gains `fetch-depth: 0` + pinned `setup-go` and three steps: revision presence (40-hex + `cat-file`), transcript regen in a detached worktree at the pin with the §3 pipeline + `diff`, and per-capability `^--- PASS:` assertion (never bare `-run`, never `grep -q`); actionlint exit 0, 7/7 `uses:` pinned, 0 `secrets.*`, zizmor green. Commit 8413b9fb3e69bb1a1182214131abe72e6acd3b6a.
- **Spec deviations**: None to the contract. Execution order is revision → transcript → qualifiers (design §4 lists transcript second, revision third) so an unknown revision fails fast naming the revision. `docs/automation.md` is outside Files: its Docs-site row now names the new failure modes (one sentence; the github-workflows rule keeps the table current in the same change).
- **Files modified**: `.github/workflows/docs.yml`, `docs/automation.md`, `specs/in-progress/docs-site-demos/tasks.md`, `specs/in-progress/docs-site-demos/handoff.md`.
- **CI evidence**: green PR run https://github.com/turbokast/mythhelm/actions/runs/37298784085/job/111726269471; stale probe https://github.com/turbokast/mythhelm/actions/runs/37298809164/job/111726351822 (PR #143); unqualified probe https://github.com/turbokast/mythhelm/actions/runs/37298825018/job/111726404257 (PR #144); unknown-revision probe https://github.com/turbokast/mythhelm/actions/runs/37298841014/job/111726456496 (PR #145); all scratch branches dropped.

### Task 7 — TUI tape scaffold (FR-3 follow-up drafted; tui-slice shipped) ✅ COMPLETED

- **Domain/agent**: go-implementer
- **Budget**: standard
- **Depends on**: Task 4
- **Change**: Merge the comment-only `tui.tape` scaffold and the "planned" docs note so
  FR-3's shape is fixed (`specs/done/tui-slice/` shipped 2026-10-04), and draft the FR-3
  follow-up recording card in `handoff.md` for the orchestrator to file via the backlog flow.
- **Files**:
  - `docs/demos/tui.tape` (header comments: planned, interface = tui-slice acceptance, no `Run` lines)
  - `docs/demos/README.md` (TUI row marked planned with the tui-slice dependency)
  - `specs/in-progress/docs-site-demos/handoff.md` (Task 7 section drafts the FR-3 follow-up card)
- **Acceptance**:
  - `grep -c '^Run ' docs/demos/tui.tape` prints 0 and the file contains the word
    `planned` plus a `specs/*/tui-slice/` reference.
  - No manifest entry names the TUI recording (`grep -c tui docs/demos/manifest.json`
    prints 0), so the Task 6 qualifier check exempts it by construction.
  - The site shows no TUI recording image: `grep -ril 'tui\.\(gif\|mp4\|webm\)' docs/
    README.md` prints nothing, while the limitations page TUI-status check still passes
    (Task 2 check re-run green).
  - `handoff.md`'s Task 7 section drafts the FR-3 follow-up recording card (title, summary
    with TUI evidence refs, score) for the orchestrator to file; the tape itself stays
    a scaffold with no `Run` lines.
  - `scripts/ci/check-public-hygiene.sh` passes.
- **Test plan**: `grep` checks; the follow-up tape card is drafted now that tui-slice has landed.
- **Invariants touched**: I14 (§9.14: unshipped TUI never presented as shipped);
  I17 (§16.6.2: no pane text or terminal text presented as completion proof).
- **Status**: ✅ Completed — comment-only `tui.tape` scaffold, planned README row and drafted FR-3 follow-up card landed; PR #137.
- **Implementation**: Tape is comments only (planned, `specs/done/tui-slice/` acceptance as the interface, zero `Run` lines); README TUI row planned with no artifact path; card draft scores 8.0 = (2+5+1)/1. Commit 620cfed4e6a51b77ab9c220f1810409dde4377eb.
- **Spec deviations**: None. The limitations-page re-run is N/A on this base: Task 2 is unmerged (parallel group {2, 7}, disjoint Files) so `docs/limitations.md` does not exist here; the no-recording-image grep over `docs/` + `README.md` prints nothing.
- **Files modified**: `docs/demos/tui.tape`, `docs/demos/README.md`, `specs/in-progress/docs-site-demos/handoff.md`, `specs/in-progress/docs-site-demos/tasks.md`.
