# Documentation Site and Scripted Terminal Demos — Design

> Mode-A design for `specs/*/docs-site-demos/requirements.md`, which is ground truth.
> Scope: single spec, 7 tasks, `auto-confirmed (non-interactive)`.
> Normative source: docs/spec/master-spec.md; § numbers, I-IDs and G-IDs refer to it.

## 1. Current state

- **No docs site.** `.github/workflows/` holds 13 workflows (`ci`, `codeql`, `dco`,
  `dependency-review`, `labeler`, `lock`, `osv-scanner`, `pr-title`, `release-drafter`,
  `scorecard`, `stale`, `welcome`, `zizmor` — `ls .github/workflows/`); `grep -ril pages
  .github/workflows/` prints nothing. No `_config.yml`, `mkdocs*`, `docusaurus*` or
  `Gemfile` exists under `docs/` or `.github/` (`find` returns nothing).
- **No tapes.** `find . -name '*.tape'` returns nothing; outside `specs/`, "vhs" appears
  only in `docs/automation.md:74`, orchestration logs and stale worktrees. `which vhs`
  finds nothing on this machine.
- **No TUI.** `ls internal/` shows `adapter admission buildinfo cli ids integration
  journal security statedir supervisor workers workspace` — no `tui` package, no `mods/`.
  The TUI is `specs/*/tui-slice/` (MH-2), currently in progress.
- **The demo exists and is scripted.** `internal/cli/demo.go:48` (`runDemo`,
  `grep -n "func runDemo" internal/cli/demo.go`) runs `executeRun` with
  `Adapter: "fake"`, `Billing: "local-scripted"` (`demo.go:94-98`), labels every screen
  `SCRIPTED DEMO` (`demo.go:26,80-82`), and supports `--check pass|fail` (`demo.go:50`;
  `fail` ends the run at exit 5). It needs only `git` (`demo.go:61-63`) plus temp dirs it
  removes afterwards. Tests: `TestDemoCheckMain`, `TestDemoOfflineNoCredentials`,
  `TestDemoCheckFailsScenarioExit5`, `TestDemoLabelsEveryScreen`, `TestDemoRemovesTempDirs`
  (`internal/cli/demo_test.go:36,83,100,110,134`).
- **Guide prose exists at the root.** `LICENSE`, `SECURITY.md`, `CONTRIBUTING.md` (DCO
  section line 22; good-first-issue link line 12), `GOVERNANCE.md`, `CHANGELOG.md`,
  `README.md` (demo pointer line 23, `go run ./cmd/mythhelm demo` line 35).
- **Binary identity API exists.** `buildinfo.Get() Info` with `Info{Version, Commit,
  GoVersion, OS, Arch}` (`internal/buildinfo/buildinfo.go:23-51`); `Version`/`Commit` are
  link-time `ldflags`, falling back to embedded VCS info, else `"devel"`/`"unknown"`.
- **Deferred rows name both halves.** `docs/automation.md:73` (GitHub Pages docs, trigger
  "First user-facing guide") and `:74` (Charm VHS demo GIFs, trigger "First TUI").
- **No conflicting specs.** `specs/todo/`, `specs/in-progress/`, `specs/unfinalized/` are
  empty. `specs/*/openssf-badge/` (MH-9, refined) also touches §19.3/G10 content; see §7.

## 2. Site layout (§19.3, G10)

New Jekyll sources under `docs/` (D1). The Pages build needs no `Gemfile`: the workflow
uses `actions/configure-pages` + `actions/jekyll-build-pages` with `source: ./docs`,
which supplies the `github-pages` gem environment (D2).

| Path | Content |
|---|---|
| `docs/_config.yml` | `title`, `description`, `theme: minima`, `plugins: []`; `exclude: [demos/*.tape]` is NOT set — tapes ship as plain text for the "checked-in tape" claim |
| `docs/_data/navigation.yml` | Ordered nav: Home, User guide, Contributing, Security policy, Changelog, Licence, Limitations. Every G10 item reachable in one click (AC-1.2). Schema: a list of `- name: …` / `  path: …` pairs with `path` relative to `docs/` (e.g. `path: mirror/LICENSE.md`) |
| `docs/_layouts/default.html` | Thin minima override: header with nav include, footer with "Rendered from commit {{ site.github.build_revision }}" |
| `docs/index.md` | Home: what MYTHHELM is, install pointer, demo embed (Task 5), link to user guide |

Every authored page (`index.md`, `user-guide.md`, `contributing.md`, `limitations.md`)
starts with exactly this front matter (Jekyll copies `.md` without front matter to
`_site` unrendered, so the `layout: default` line is also what applies the override):

```yaml
---
layout: default
title: <page title>
---
```

The `docs/mirror/*.md` byte-mirrors carry no front matter in the tree (Task 2's `cmp`
forbids it); the build job injects it transiently — see §4. None of the four root
sources starts with `---` (`head -1` shows `Apache License`, `# Security Policy`,
`# Changelog`, `# Governance`), so injection is unambiguous.
| `docs/user-guide.md` | Authored extraction: installation, quickstart (`mythhelm demo`, `mythhelm doctor`), limitations pointer, demo recording embed (AC-1.1, AC-2.3) |
| `docs/contributing.md` | Authored extraction: governance summary, contribution path, DCO sign-off requirement, good-first-issue route with no paid subscription (AC-1.3) |
| `docs/limitations.md` | Authored limitations register extracted from the master spec (§22.3): pre-alpha, one fake adapter, no TUI yet (labelled planned, N1/I14) |
| `docs/mirror/LICENSE.md` etc. | Byte mirrors of `LICENSE`, `SECURITY.md`, `CHANGELOG.md`, `GOVERNANCE.md` (D3). `CONTRIBUTING.md` is extracted into `contributing.md`, not mirrored. The directory has no underscore prefix: Jekyll excludes non-special `_`-prefixed directories from the build, so `_mirror/` pages would 404 |

## 3. Record command, tapes and manifest (§14.9, §22.3)

Layout (D4, default (a) of requirements Q2):

```text
docs/demos/
  README.md            # the documented record command + per-recording table
  record.sh            # the record command: build, transcript regen, GIF render, verify
  demo.tape            # drives `mythhelm demo --check pass` end to end (D6)
  demo.gif             # rendered artifact, checked in (D5)
  demo.transcript.txt  # normalized stdout of the taped commands (the reproducibility proof)
  normalize.sed        # the exact normalization expressions (portable GNU/BSD sed)
  manifest.json        # exact schema below; one entry per recording
  tui.tape             # scaffold only until specs/*/tui-slice/ lands (Task 7, FR-3)
```

**Record command** (documented in `docs/demos/README.md`, AC-2.1/AC-3.1):

```sh
docs/demos/record.sh   # builds the binary, regenerates the transcript, renders the GIF, verifies all three
```

`record.sh` (checked in by Task 4) runs, in order: the `go build` below; the
normalization pipeline with `pipefail` and an explicit binary exit-status
assertion; `vhs docs/demos/demo.tape`, which renders `docs/demos/demo.gif`
(VHS version pinned in the tape header); and a verify pass (`diff` of the
regenerated transcript, manifest revision check). A bare
`vhs docs/demos/demo.tape` only renders the GIF and is not the record command.

The tape header is comments declaring `Binary-Version:`, `Binary-Commit:`, and
`VHS-Version:` (AC-4.1 declaration). The manifest repeats the revision in machine-checked
form:

```json
{
  "recordings": [
    {
      "name": "demo",
      "tape": "docs/demos/demo.tape",
      "artifact": "docs/demos/demo.gif",
      "transcript": "docs/demos/demo.transcript.txt",
      "binary_version": "devel",
      "binary_commit": "<40-hex sha rendered from>",
      "vhs_version": "<pinned at implementation, e.g. v0.8.0>",
      "capabilities": [
        {"name": "offline scripted run", "qualifier": "TestDemoOfflineNoCredentials"}
      ]
    }
  ]
}
```

**Transcript normalization** (exact pipeline; verified 2026-10-02: two `go run` runs, one
built-binary run and one `TMPDIR=/tmp/alt-tmp` run all converge to identical output, and
a `--check fail` run still differs). Use a **built binary**, never `go run`: `go run`
exits 1 with an `exit status 5` trailer on the fail path, while the binary itself exits 5
(`internal/cli/demo_test.go:100` asserts 5 through `runMain`).

```sh
set -o pipefail
go build -o /tmp/mythhelm-record ./cmd/mythhelm
/tmp/mythhelm-record demo --check pass | sed -E -f docs/demos/normalize.sed \
  > docs/demos/demo.transcript.txt
test "${PIPESTATUS[0]}" -eq 0   # the binary's status, never sed's
```

The `PIPESTATUS` assertion is the verification: without it a failing binary
still yields a zero pipeline status from `sed`, masking the failure.

`docs/demos/normalize.sed` (checked in by Task 4; portable across GNU and BSD `sed`):

```sed
s#[^[:space:]]*mythhelm-demo-(repo|state)-[0-9]+#<TMPDIR>/mythhelm-demo-\1-XXX#g
s#run_[0-9A-Z]{26}#run_XXX#g
s#att_[0-9A-Z]{26}#att_XXX#g
s#(worker pid )[0-9]+(\.[0-9]+e[+][0-9]+)?#\1NNN#g
s#(native pid )[0-9]+(\.[0-9]+e[+][0-9]+)?#\1NNN#g
s#e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855#<EMPTY_SHA256>#g
s#[0-9a-f]{40}#<SHA40>#g
s#<EMPTY_SHA256>#e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855#g
s#evidence/[0-9a-f]{12}/#evidence/<SHA12>/#g
```

Volatile classes, in order: temp paths (`os.MkdirTemp("", …)` honors `$TMPDIR`, so the
prefix is `[^[:space:]]*`, never hardcoded `/tmp`); `run_`/`att_` ULIDs; worker/native
pids, which render as plain integers below 10^6 and as `1.012413e+06` above it (float
formatting through `p.text(…)` at `internal/cli/render.go:193`); 40-hex snapshot and
candidate SHAs, with the stable empty-evidence sha256 (`e3b0c44…855`) protected first so
it survives verbatim; 12-hex evidence directory prefixes.

GIF bytes are not bit-reproducible (encoder timestamps); the transcript is the
transcript-reproducibility proof (honesty register). The demo scenario is `--check pass`
end to end (D6); `fail` stays covered by `TestDemoCheckFailsScenarioExit5`, not the tape.

**No-network argument.** The taped path runs the local binary against the fake adapter
with `local-scripted` billing and local `git` only (`demo.go:61-98`); the tape contains
no `curl`/`wget`/`ssh`/`http` line (Task 4 checks this structurally). Covered classes:
explicit HTTP/file-transfer tools and remote URLs in tape lines. Residual: the tripwire
is a static matcher over shell grammar — a `git ls-remote` line would pass it — so the
Task 4 reviewer also reads the tape's `Run`/`Type` lines by eye and confirms each invokes
only the local `mythhelm` binary (recorded in the completion entry). No paid credential
is read: `TestDemoOfflineNoCredentials` is the qualifier.

## 4. Honesty checks in CI (I07, I14)

One workflow, `.github/workflows/docs.yml` (Task 3 creates it with the build/deploy jobs;
Task 6 adds the `checks` job; the two tasks share that file and never run in parallel):

- `checks` (display name `Docs checks`; runs on `pull_request`, `push` to `main`,
  `merge_group`): fork-safe, no secrets (NFR-1, §19.4). It is a **separate required
  check** via the branch ruleset (a `CI OK` `needs:` entry cannot reach across workflows),
  and the Task 3 PR description instructs the maintainer to add it. Steps:
  1. Mirror drift: `cmp` each `docs/mirror/*` against its root source; stale mirror
     fails with the `cp` command to refresh it.
  2. Transcript freshness: check out the manifest's `binary_commit` into a detached
     worktree (`git worktree add --detach`), build the binary there from source
     (`go build`, never `go run` — §3), re-run the §3 normalization pipeline
     (including its exit-status assertion), `diff` against `demo.transcript.txt`;
     drift fails (this is the AC-4.1 staleness check — D7).
  3. Manifest revision: `binary_commit` must be a 40-hex commit present in the repo
     (`git cat-file -t`); step 2 rebuilds and re-runs from that exact commit, never
     from `HEAD`. An unknown revision fails.
  4. Capability qualifiers: for every `capabilities[]` entry, in the step-2 worktree
     at `binary_commit`, `go test -v -run "^<qualifier>$" -count=1 ./...` must print
     a `^--- PASS: <qualifier>` line (checked with `set -o pipefail … | grep -E
     '^--- PASS: <qualifier>' >/dev/null`, never `grep -q`: quiet grep closes the
     pipe on first match and can SIGPIPE a passing `go test` run;
     a bare `-run` exits 0 even when the qualifier matches nothing, so the PASS line
     is the assertion). A shown capability with no passing qualifier at the declared
     revision fails (AC-4.2). `tui.tape` has no manifest entry until FR-3 lands, so
     it is exempt by construction.
- `build` (needs `checks`): first an "Inject mirror front matter" step that prepends
  `---\nlayout: default\ntitle: <LICENSE|Security policy|Changelog|Governance>\n---\n`
  to the working-tree `docs/mirror/*.md` copies (transient: never committed, and it runs
  after `checks`, so it cannot mask mirror drift); then `actions/configure-pages`,
  `actions/jekyll-build-pages` (`source: ./docs`), upload artifact. After the build,
  assert each mirror page rendered: `_site/mirror/LICENSE.html`,
  `_site/mirror/SECURITY.html`, `_site/mirror/CHANGELOG.html` and
  `_site/mirror/GOVERNANCE.html` exist (AC-1.2 built-site proof).
- `deploy` (only on `push` to `main`, needs `build`): `actions/deploy-pages` with
  `pages: write` + `id-token: write` on that job only, each with a why-comment.

`README.md` gains the demo GIF embed with a caption naming `docs/demos/demo.tape` and the
`docs/demos/record.sh` record command, plus the published site URL (Task 5; AC-2.3, DoD).

## 5. TUI recording, sequenced behind tui-slice (FR-3)

`docs/demos/tui.tape` merges as a scaffold: header comments stating it is planned, the
interface it will drive (`specs/*/tui-slice/` acceptance: mission view, layouts, keyboard
flows — AC-3.2 limits the tape to exactly that covered behaviour), and no `Run` lines.
The site shows the word "planned" where the TUI recording will go — never a mock-up
image (N1/I14). The full FR-3 tape is a follow-up task after `tui-slice` lands; the DoD
permits "explicitly deferred … with the tape scaffold merged". AC-3.3 (I17) binds the
follow-up: the tape shows the TUI's own verification status, never pane text as proof.

## 6. Decisions

| ID | Decision | Rationale |
|---|---|---|
| D1 | GitHub-native Jekyll via `actions/jekyll-build-pages` | Matches the Deferred row's "GitHub-native" tier and requirements Q1 default (a); no new local toolchain, fork-safe build. MkDocs/Docusaurus (b) add dependencies for nav this small; hand HTML (c) has no nav/layout reuse. |
| D2 | No `Gemfile`; the Actions `github-pages` environment is the build | Keeps a `Gemfile` (unmapped in `knowledge/domains.md`) out of the tree; CI is the build referee. Local `jekyll build` needs the Pages gem set — documented, not required. |
| D3 | Byte mirrors of root guides under `docs/mirror/` + CI drift check | Single source of truth stays at the root; Jekyll cannot include `../` parents, and checked-in mirrors keep local builds working where build-time copies would not. `CONTRIBUTING.md` is extracted into `contributing.md`, not mirrored, keeping Task 2 at 8 files. |
| D4 | Tapes and GIFs side by side in `docs/demos/` | Requirements Q2 default (a); reviewable in one directory, embeddable from README and the site with relative paths. Release attachments (b) would hide the artifact from PR review. |
| D5 | GIF in tree for the short demo loop | Requirements Q3 default (a); GIFs embed in README and Pages with no player. Revisit on size: if `demo.gif` exceeds ~5 MB the implementer reports back and MP4/WebM becomes a follow-up. |
| D6 | First tape drives `mythhelm demo --check pass` end to end | Requirements Q4 default (a); the pass path is the honest first impression, and `fail` is already qualified by `TestDemoCheckFailsScenarioExit5`. |
| D7 | AC-4.1 staleness = transcript drift at the declared revision, not commit equality with HEAD | Forcing `manifest.binary_commit == HEAD` would fail every unrelated commit. CI instead checks out the declared `binary_commit`, re-runs the taped commands there and diffs the normalized transcript (behavioural staleness at the declared revision). |
| D8 | One `docs.yml` workflow for build, deploy and recording checks | One file, one review surface; the `checks` job gates both PRs and the deploy. Task 3 creates it, Task 6 extends it sequentially. |
| D9 | VHS version pinned at implementation time in tape header + manifest | Pinning a version from memory now would be supply-chain fiction (`.claude/rules/github-workflows.md`); the implementing task records the `vhs --version` it actually used. NFR-2 (under 10 min, free tooling) is timed then. |

## 7. Cross-spec references

- Prerequisite: `specs/*/tui-slice/` (MH-2) for FR-3 only; FR-1/FR-2 ship first (§5).
- Builds on: `specs/*/dogfood-slice/` (MH-1) — `mythhelm demo`, plain output.
- Coordinate with: `specs/*/openssf-badge/` (MH-9, refined) — also touches §19.3/G10
  prose. This spec assembles the site from the root guides; `openssf-badge` adds only its
  badge link and assessment answers. Whichever lands second reconciles shared sentences;
  no file overlap is expected beyond `README.md` badges (this spec's README edit is the
  demo embed + site URL, not badges).

## 8. Honesty register

| Spec demand | Position |
|---|---|
| §19.3 published contribution path (AC-1.3) | Met: `contributing.md` + DCO + good-first-issue route; good-first-issue *content* volume is the backlog's job, not this spec's. |
| G10 provenance / SBOM published | NOT met (requirements N4): no release exists yet, so the site carries no provenance/SBOM claims at all rather than weak ones. |
| §14.9 TUI recordings (§5, FR-3) | Deferred behind `specs/*/tui-slice/`; scaffold + "planned" label only. |
| §22.3 bit-reproducible recordings | Partially met: GIF bytes are encoder-nondeterministic; the normalized transcript is the reproducibility proof CI enforces. |
| AC-4.1 GIF-to-tape binding | Residual: no CI step re-renders `demo.gif` and compares frames, so a swapped but plausible GIF passes every check. The transcript + manifest revision + human review of the GIF in the PR is the binding. A render-compare step (duration/frame-count/perceptual hash) is a follow-up if recordings multiply. |
| VHS row of the Deferred table (`docs/automation.md:74`) | Stays Deferred until FR-3 lands: its trigger is "First TUI" and its text names TUI recordings. Only the Pages row moves to Active. |
| NFR-2 macOS timing | Timed at implementation on the contributor's machine class; CI has no macOS VHS leg in this spec. |
