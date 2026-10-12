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
`go`, `git`, `python3`, GNU `timeout` (the hero tape kills its run with it; on
macOS install coreutils and put its `timeout` on PATH), and VHS (`vhs` renders
through `ffmpeg` and `ttyd`).
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

The hero tape tells the story the README will lead with: kill the terminal
mid-run, then recover. Its GIF, transcript and manifest entry are not committed
yet: they are recorded with `docs/demos/record.sh --repin` on a clean main
tip, because a pin has to be a commit on main. It opens on a title card (`MYTHHELM`, the `SCRIPTED DEMO`
line), then `mythhelm run` drives the fake adapter's `paced` scenario (a
scripted four-turn session of about 20 seconds) in the live TUI. The tape
runs it under `timeout --foreground -s KILL 8`, so the foreground `mythhelm
run` process is killed with SIGKILL while the agent is mid-attempt, and the
dead TUI's last frame stays frozen on screen. A red marker printed by the
shell from the real exit status of `timeout` (137) says so; it is an
annotation, not mythhelm output. The worker and the agent are separate
processes and keep running: `pgrep -f "__worker --state $MYTHHELM_HOME"`
prints the worker's pid, and `mythhelm recover <run>` then prints `reattached
to worker pid` with the same pid (the qualifying test asserts the worker and
native pids are unchanged). The journal is replayed at once and the agent's
remaining work arrives live: the scenario finishes the work after the kill,
the check runs and passes, and `mythhelm review` opens the inspect view with
the diff and the revision-attached `verified <rev>` line. A worker killed
outright is a different case: its attempt cannot resolve and the run stays
quarantined (issue #391), so it is not shown. The ending shows what is
verified (the checked revision, the check's name) and what stays labelled
(`qualified: false`, `paid continuation: not-applicable`, a native-reported
exit); the receipt's `unknowns` list is in `mythhelm review --format jsonl`,
not on the screen. A waived check (`--no-checks`) would read `NOT RUN`; this
tape does not take that branch.

The tape's hidden segments (`Hide` ... `Show`) hold setup and terminal
hygiene only, never product behaviour: sourcing `docs/demos/hero-env.sh`,
which builds a disposable repository and state directory under `/tmp/demo`
(a short fixed path; a leftover one is replaced only when it carries the
script's marker file), the title card, `reset` after the dead TUI leaves the
terminal raw, copying the run ID out of `runs list` into a shell variable,
and `clear`. Hidden time still passes, so the agent keeps working. The TUI
bytes cannot match a re-run, so the hero transcript comes from
`docs/demos/hero-transcript.sh`, a linear companion run of the same commands:
`mythhelm run --plain` in the background, a SIGKILL once the third turn
prints, then `pgrep`, `runs list` (which still reads `executing`, because the
journal cannot know the CLI died), `recover` and `review`. Its manifest entry declares the exact binary
revision rendered from and the qualifying test for each shown capability:

| Capability shown | Qualifying test |
|---|---|
| offline scripted run | `TestDemoOfflineNoCredentials` (`internal/cli/demo_test.go`) |
| live mission view | `TestMissionShowsRequiredFacts` (`internal/tui/model_test.go`) |
| recover reattaches after the supervisor is killed and the run completes | `TestRecoverAfterSupervisorKilledReattachesAndCompletes` (`internal/supervisor/faults_test.go`) |
| recovery never relaunches the native | `TestRecoverNeverRelaunchesNative` (`internal/supervisor/faults_test.go`) |
| revision-attached verification | `TestVerificationMismatchLabelsRevisions` (`internal/tui/model_test.go`) |
| billing posture labels | `TestStatusStripKindLabels` (`internal/tui/model_test.go`) |
| candidate diff review | `TestParseHunks` (`internal/tui/diff_test.go`) |

CI's transcript-freshness step (`.github/workflows/docs.yml`) regenerates the
`demo` and `tui` transcripts by name; it does not yet regenerate the hero
transcript, so `docs/demos/record.sh` is the check for it until that
workflow learns the third pipeline. Its revision-present and qualifier steps
read the manifest and do cover the hero entry.

Normalization (`docs/demos/normalize.sed`) covers temp paths, `run_`/`att_`
ULIDs, ISO UTC timestamps (`<TIME>`), the hero scratch directory (`/tmp/demo`) and a bare pid line (`NNN`), both worker/native pid shapes, 40-hex SHAs (with the stable
empty-evidence sha256 protected verbatim) and 12-hex evidence prefixes, plus
the TUI rules: 20-hex truncated snapshot SHAs, live-sampled `goal:`/`run
state:`/`admission:` values (`<LIVE-SAMPLE>`), the race-window
`receipt.written` event line (deleted) and the `next-after` cursor
(`<LIVE-CURSOR>`; the cursor reads 21, 22 or 23 depending on whether the receipt event lands before the last read). The `reservation held on bucket` notice is kept right after `run: admission` in the linear takes and dropped everywhere else: in the `--accessible` stream the notice and the first snapshot are printed by different goroutines, so its position varies between runs. Label names stay verbatim; only sampled values normalize.
The transcript keeps the binary's own `SCRIPTED DEMO` labels verbatim.
