# Terminal recordings

Scripted, reproducible terminal recordings of MYTHHELM, made with
[Charm VHS](https://github.com/charmbracelet/vhs). Every recording ships its
tape (the commands), its rendered artifact, and a normalized transcript of the
taped commands' stdout — the transcript is the reproducibility proof CI
enforces, because GIF bytes are encoder-nondeterministic. The TUI transcript
is the exception: interactive bytes can never byte-match a re-run, so it
comes from companion linear-mode runs of the same binary (with stderr
merged), not from the taped takes themselves.

## Record command

```sh
docs/demos/record.sh            # builds the binary, regenerates the transcript, renders the GIF, verifies all three
docs/demos/record.sh --repin    # regenerate from the current checkout and move the pins to it (clean tree only)
```

Without flags the checkout must match the manifest's `binary_commit`;
otherwise the script refuses to write artifacts another revision labels.
Note the squash-merge trap: pins name the pre-commit HEAD, so after a merge
no checkout matches and plain `record.sh` always refuses — re-record with
`--repin` on a clean main tip instead.
After a re-record on new code, `--repin` moves the manifest and tape-header
pins to the current commit — commit the regenerated artifacts and the moved
pins together.

The script runs from anywhere in the checkout. It needs only free tooling:
`go`, `git`, `python3`, and VHS (`vhs` renders through `ffmpeg` and `ttyd`).
A bare `vhs docs/demos/demo.tape` only renders the GIF and is not the record
command: it skips the build, the transcript regen and the verify pass.

## Recordings

| Recording | Tape | Artifact | Transcript | Binary revision | VHS |
|---|---|---|---|---|---|
| demo | `docs/demos/demo.tape` | `docs/demos/demo.gif` | `docs/demos/demo.transcript.txt` | `179bba4ceae958f9591e9fedb056f70da59ce40c` | v0.12.1 |
| tui | `docs/demos/tui.tape` | `docs/demos/tui.gif` | `docs/demos/tui.transcript.txt` | `179bba4ceae958f9591e9fedb056f70da59ce40c` | v0.12.1 |

The demo tape drives `mythhelm demo --check pass` end to end against the fake
adapter with local-scripted billing: no network, no paid credentials. Its
manifest entry (`docs/demos/manifest.json`) declares the exact binary revision
rendered from and the qualifying test for each shown capability:

| Capability shown | Qualifying test |
|---|---|
| offline scripted run | `TestDemoOfflineNoCredentials` (`internal/cli/demo_test.go`) |

The TUI tape drives the same scenario inside the shipped TUI slice
(`specs/done/tui-slice/`): the mission view at 140 columns, the responsive
ladder rung by rung (120, 90, 70 columns — each a fresh launch, since the TUI
owns all input while it runs and a rung change cannot be typed mid-session),
the keyboard flows (arrows, `/` filter, `:` palette, `?` help,
`Enter`/`Escape`), and the linear `--accessible` stream as the closing take.
Scope is exactly the met tui-slice acceptance: the inert exit-dialog rows
(issue #106) and the unwired history search never appear as working
behaviour — `q` is never pressed, only `ctrl+c` quits. Its manifest entry
declares the exact binary revision rendered from and the qualifying test for
each shown capability:

| Capability shown | Qualifying test |
|---|---|
| mission view | `TestMissionShowsRequiredFacts` (`internal/tui/model_test.go`) |
| keyboard flows | `TestKeyFlows` (`internal/tui/nav_test.go`) |
| command palette | `TestPaletteSearchable` (`internal/tui/nav_test.go`) |
| context help | `TestHelpFullNames` (`internal/tui/nav_test.go`) |
| responsive layouts | `TestLayoutBreakpoints` (`internal/tui/layout_test.go`) |
| revision-attached verification | `TestVerificationMismatchLabelsRevisions` (`internal/tui/model_test.go`) |
| linear accessible stream | `TestAccessibleOrderedStream` (`internal/cli/accessible_test.go`) |

Normalization (`docs/demos/normalize.sed`) covers temp paths, `run_`/`att_`
ULIDs, both worker/native pid shapes, 40-hex SHAs (with the stable
empty-evidence sha256 protected verbatim) and 12-hex evidence prefixes, plus
the TUI rules: 20-hex truncated snapshot SHAs, live-sampled `goal:`/`run
state:`/`admission:` values (`<LIVE-SAMPLE>`), the race-window
`receipt.written` event line (deleted) and the `next-after` cursor
(`<LIVE-CURSOR>`). Label names stay verbatim; only sampled values normalize.
The transcript keeps the binary's own `SCRIPTED DEMO` labels verbatim.
