# Terminal recordings

Scripted, reproducible terminal recordings of MYTHHELM, made with
[Charm VHS](https://github.com/charmbracelet/vhs). Every recording ships its
tape (the commands), its rendered artifact, and a normalized transcript of the
taped commands' stdout — the transcript is the reproducibility proof CI
enforces, because GIF bytes are encoder-nondeterministic.

## Record command

```sh
docs/demos/record.sh            # builds the binary, regenerates the transcript, renders the GIF, verifies all three
docs/demos/record.sh --repin    # regenerate from the current checkout and move the pins to it (clean tree only)
```

Without flags the checkout must match the manifest's `binary_commit`;
otherwise the script refuses to write artifacts another revision labels.
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
| demo | `docs/demos/demo.tape` | `docs/demos/demo.gif` | `docs/demos/demo.transcript.txt` | `e0ecdbc9554c90da6ac61f6300b7cc836acae266` | v0.12.1 |

The demo tape drives `mythhelm demo --check pass` end to end against the fake
adapter with local-scripted billing: no network, no paid credentials. Its
manifest entry (`docs/demos/manifest.json`) declares the exact binary revision
rendered from and the qualifying test for each shown capability:

| Capability shown | Qualifying test |
|---|---|
| offline scripted run | `TestDemoOfflineNoCredentials` (`internal/cli/demo_test.go`) |

Normalization (`docs/demos/normalize.sed`) covers temp paths, `run_`/`att_`
ULIDs, both worker/native pid shapes, 40-hex SHAs (with the stable
empty-evidence sha256 protected verbatim) and 12-hex evidence prefixes. The
transcript keeps the binary's own `SCRIPTED DEMO` labels verbatim.
