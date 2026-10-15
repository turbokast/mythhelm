# Design frames (exported cell grids)

Exported 2026-10-12 from the design-direction review (decision D-97): each frame is the exact cell grid drawn at a real terminal width, with ligatures off. The text block is the frame; the span list after it gives each styled run as `class: text`. Frames for later slices and the vision frames are included so the layout stays consistent; only frames marked slice 1 bind this spec. Product claims in the frames reflect what shipped on 2026-10-12, and the sample task text is illustrative. Where a frame differs from a decision in requirements.md, the decision wins: the key bar writes `Enter` instead of ⏎ and the thinking marker is ▶ instead of ▸ (OQ-15), and the TUI does not paint `surface` behind the whole view (OQ-4).

## Style classes

| Class | Token / treatment |
|---|---|
| `fa` | `border` line glyphs (rules, pane separators) |
| `mu` | `text_muted` (labels, timestamps) |
| `b` | bold `text` |
| `st` | `focus` (Star): mission mark, focus, working |
| `wa` | `attention` (amber): needs you, nothing else |
| `ok` / `er` | `ok` / `err`, always beside ✓ / ✗ or a word |
| `un` | unknown value: `text_muted`, dotted underline where the terminal supports it |
| `key` | key chip: `surface_raised` background |
| `ov` | overlay card body: `surface_raised` background |
| `sel` | selected row |
| `dim` | the view behind an overlay, dimmed |
| `chip`, `chipst`, `chipok` | status chips (neutral, focus, ok) |

## Frame 1a: Default view, working, 148 columns (≥140 rung: three panes). The AGENT pane's per-action lines are slice 2 (they need a journaled per-action event).

- Width: 148 columns, Observatory theme
- Binds: slice 1

```text
◆ MYTHHELM  Add retry to the supervisor beat reader                                        ● working · 02:14   claude-code · opus-5.5 · trusted-host
────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────
NOW  Agent A is editing internal/workers/envelope.go — 3 reads, 1 edit in the last 40s                         NEXT  checks run when the agent stops
────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────
▌TASKS                      │ AGENT A  claude-code · opus-5.5 · attempt 1                 │ EVIDENCE
▶ ● #1 Add retry to the bea…│                                                             │ stages
      reader · working      │ 02:12  read   internal/workers/envelope.go                  │  ✓ admitted     snapshot 33a136c
                            │ 02:12  read   internal/control/reconnect.go                 │  ● working      agent A
                            │ 02:13  grep   "ReadSupervisorBeat"  → 6 matches             │  ○ frozen
                            │ 02:14  edit   internal/workers/envelope.go  +18 -3          │  ○ checked      3 checks pinned
                            │ 02:14  ▸ thinking                                           │  ○ ready
                            │                                                             │
                            │ 14 turns · Read 7 · Edit 2 · Bash 3                         │ usage
                            │ [v] full stream   [e] expand all                            │  41k tokens   reported
                            │                                                             │  allowance    unknown
                            │                                                             │  billing      subscription-declared
                            │                                                             │               you declared it; not verified
                            │                                                             │
────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────
  ⏎  inspect   s  stop   d  detach   ?  help   :  commands   /  filter                                                 nothing needs you   q  exit…
```

<details><summary>Styled runs</summary>

```text
 1  st b: '◆ MYTHHELM' · b: 'Add retry to the supervisor beat reader' · st: '●' · b: 'working' · mu: '· 02:14' · mu: 'claude-code · opus-5.5 · trusted-host'
 2  fa: '────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────'
 3  mu: 'NOW ' · b: 'Agent A is editing' · mu: '— 3 reads, 1 edit in the last 40s' · mu: 'NEXT'
 4  fa: '────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────'
 5  st: '▌' · b: 'TASKS' · fa: '│' · b: 'AGENT A' · mu: 'claude-code · opus-5.5 · attempt 1' · fa: '│' · b: 'EVIDENCE'
 6  sel st: '▶' · sel st: '●' · sel: ' #1 Add retry to the bea…' · fa: '│' · fa: '│' · mu: 'stages'
 7  sel: '      reader · working     ' · fa: '│' · mu: '02:12' · fa: '│' · ok: '✓' · mu: 'snapshot 33a136c'
 8  fa: '│' · mu: '02:12' · fa: '│' · st: '●' · mu: 'agent A'
 9  fa: '│' · mu: '02:13' · mu: '→ 6 matches' · fa: '│' · fa: '○'
10  fa: '│' · mu: '02:14' · b: 'edit' · ok: '+18' · er: '-3' · fa: '│' · fa: '○' · mu: '3 checks pinned'
11  fa: '│' · st: '02:14  ▸ thinking' · fa: '│' · fa: '○'
12  fa: '│' · fa: '│'
13  fa: '│' · mu: '14 turns · Read 7 · Edit 2 · Bash 3' · fa: '│' · mu: 'usage'
14  fa: '│' · mu: '[v] full stream   [e] expand all' · fa: '│' · mu: 'reported'
15  fa: '│' · fa: '│' · un: 'unknown'
16  fa: '│' · fa: '│'
17  fa: '│' · fa: '│' · mu: 'you declared it; not verified'
18  fa: '│' · fa: '│'
19  fa: '────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────'
20  key: ' ⏎ ' · key: ' s ' · key: ' d ' · key: ' ? ' · key: ' : ' · key: ' / ' · mu: 'nothing needs you' · key: ' q '
```

</details>

## Frame 1f: Two-pane rung, 110 columns (100–139).

- Width: 110 columns, Observatory theme
- Binds: slice 1

```text
◆ MYTHHELM  Add retry to the beat reader                                                     ● working · 02:14
──────────────────────────────────────────────────────────────────────────────────────────────────────────────
NOW  editing internal/workers/envelope.go                                                          NEXT checks
──────────────────────────────────────────────────────────────────────────────────────────────────────────────
▌TASKS                            │ AGENT A  claude-code · opus-5.5   [tab] evidence
▶ ● #1 Add retry … working        │ 02:12 read  internal/workers/envelope.go
                                  │ 02:12 read  internal/control/reconnect.go
                                  │ 02:14 edit  internal/workers/envelope.go +18 -3
stages                            │ 02:14 ▸ thinking
 ✓ admitted  ● working            │
 ○ frozen ○ checked ○ ready       │ 41k tokens reported · allowance unknown
──────────────────────────────────────────────────────────────────────────────────────────────────────────────
  ⏎  inspect   s  stop   d  detach   ?  help   :  commands                       nothing needs you   q  exit…
```

<details><summary>Styled runs</summary>

```text
 1  st b: '◆ MYTHHELM' · b: 'Add retry to the beat reader' · st: '●' · b: 'working' · mu: '· 02:14'
 2  fa: '──────────────────────────────────────────────────────────────────────────────────────────────────────────────'
 3  mu: 'NOW ' · b: 'editing' · mu: 'NEXT'
 4  fa: '──────────────────────────────────────────────────────────────────────────────────────────────────────────────'
 5  st: '▌' · b: 'TASKS' · fa: '│' · b: 'AGENT A' · mu: 'claude-code · opus-5.5' · fa: '[tab] evidence'
 6  sel st: '▶' · sel st: '●' · sel: ' #1 Add retry … working' · fa: '│' · mu: '02:12'
 7  fa: '│' · mu: '02:12'
 8  fa: '│' · mu: '02:14' · b: 'edit' · ok: '+18' · er: '-3'
 9  mu: 'stages' · fa: '│' · st: '02:14 ▸ thinking'
10  ok: '✓' · st: '●' · fa: '│'
11  fa: '○' · fa: '○' · fa: '○' · fa: '│' · mu: '41k tokens reported · allowance' · un: 'unknown'
12  fa: '──────────────────────────────────────────────────────────────────────────────────────────────────────────────'
13  key: ' ⏎ ' · key: ' s ' · key: ' d ' · key: ' ? ' · key: ' : ' · mu: 'nothing needs you' · key: ' q '
```

</details>

## Frame 1g: Tab rung, 88 columns (80–99).

- Width: 88 columns, Observatory theme
- Binds: slice 1

```text
◆ Add retry to the beat reader                                           ● working 02:14
────────────────────────────────────────────────────────────────────────────────────────
NOW  editing envelope.go                                                     NEXT checks
 Tasks   Agent A   Evidence  Diff                                                  tab →
────────────────────────────────────────────────────────────────────────────────────────
 02:12 read  internal/workers/envelope.go
 02:12 read  internal/control/reconnect.go
 02:13 grep  "ReadSupervisorBeat" → 6
 02:14 edit  internal/workers/envelope.go +18 -3
 02:14 ▸ thinking

────────────────────────────────────────────────────────────────────────────────────────
  ⏎  inspect   s  stop   ?  help                                               q  exit…
```

<details><summary>Styled runs</summary>

```text
 1  st b: '◆' · b: 'Add retry to the beat reader' · st: '●' · b: 'working' · mu: '02:14'
 2  fa: '────────────────────────────────────────────────────────────────────────────────────────'
 3  mu: 'NOW ' · b: 'editing' · mu: 'NEXT'
 4  mu: 'Tasks' · chipst: ' Agent A ' · mu: 'Evidence' · mu: 'Diff' · fa: 'tab →'
 5  fa: '────────────────────────────────────────────────────────────────────────────────────────'
 6  mu: '02:12'
 7  mu: '02:12'
 8  mu: '02:13' · mu: '→ 6'
 9  mu: '02:14' · b: 'edit' · ok: '+18' · er: '-3'
10  st: '02:14 ▸ thinking'
12  fa: '────────────────────────────────────────────────────────────────────────────────────────'
13  key: ' ⏎ ' · key: ' s ' · key: ' ? ' · key: ' q '
```

</details>

## Frame 1h: Compact rung, 72 columns (<80).

- Width: 72 columns, Observatory theme
- Binds: slice 1

```text
◆ Add retry to the beat reader                                 ● working
now  editing envelope.go +18 -3
next checks when the agent stops
use  41k tok reported · allowance unknown
 ⏎  more   s  stop   ?  help                        --accessible: linear
```

<details><summary>Styled runs</summary>

```text
 1  st b: '◆' · b: 'Add retry to the beat reader' · st: '●'
 2  mu: 'now ' · ok: '+18' · er: '-3'
 3  mu: 'next'
 4  mu: 'use ' · un: 'unknown'
 5  key: ' ⏎ ' · key: ' s ' · key: ' ? ' · fa: '--accessible: linear'
```

</details>

## Frame 1i: Paper (light) theme, 100 columns.

- Width: 100 columns, Paper theme
- Binds: slice 1

```text
◆ MYTHHELM  Add retry to the beat reader                                          ● ready for review
────────────────────────────────────────────────────────────────────────────────────────────────────
NOW  Passed 3 of 3 checks. Nothing applied yet.                                          NEXT review
────────────────────────────────────────────────────────────────────────────────────────────────────
 PROVEN      ✓ unit tests        ✓ vet  ✓ race x50
 NOT PROVEN  Windows sharing path
 UNKNOWN     paid continuation · descendants
────────────────────────────────────────────────────────────────────────────────────────────────────
  a  apply   x  discard   e  export                                                        q  exit…
```

<details><summary>Styled runs</summary>

```text
 1  st b: '◆ MYTHHELM' · b: 'Add retry to the beat reader' · ok: '●' · b: 'ready for review'
 2  fa: '────────────────────────────────────────────────────────────────────────────────────────────────────'
 3  mu: 'NOW ' · b: 'Passed 3 of 3 checks.' · mu: 'Nothing applied yet.' · mu: 'NEXT'
 4  fa: '────────────────────────────────────────────────────────────────────────────────────────────────────'
 5  ok: 'PROVEN' · ok: '✓' · ok: '✓' · ok: '✓'
 6  wa: 'NOT PROVEN'
 7  mu: 'UNKNOWN'
 8  fa: '────────────────────────────────────────────────────────────────────────────────────────────────────'
 9  key: ' a ' · key: ' x ' · key: ' e ' · key: ' q '
```

</details>

## Frame 1b: Approval card as an overlay over the dimmed view, 148 columns.

- Width: 148 columns, Observatory theme
- Binds: slice 3

```text
◆ MYTHHELM  Add retry to the supervisor beat reader                                     ◆ NEEDS YOU  · 03:02   claude-code · opus-5.5 · trusted-host
────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────
WAITING Agent A asks to run a command outside its allowed tools                                           NEXT  the agent continues after you decide
────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────
TASKS                       │ AGENT A — approval                                          │ EVIDENCE
  ◆ #1 Add retry … waiting  │ ╭─ APPROVAL ─ risk  WRITES OUTSIDE THE SNAPSHOT  ─────────╮ │  ✓ admitted
                            │ │ command  go test -race ./internal/workers -count=50     │ │  ◆ waiting on you
                            │ │ why      "prove the retry under load" (agent)           │ │
                            │ │ touches  $GOCACHE, outside the run snapshot             │ │
                            │ │                                                         │ │
                            │ │ pending edit  internal/workers/envelope.go              │ │
                            │ │  41 - raw, err := os.ReadFile(path)                     │ │
                            │ │  41 + raw, err := readBeat(path, transient)             │ │
                            │ │  42 + // a sharing violation retries, 50ms max          │ │
                            │ │ … 16 more lines · ⏎ opens the full diff                 │ │
                            │ │                                                         │ │
                            │ │  a  allow once    n  deny    t  take over input         │ │
                            │ ╰─────────────────────────────────────────────────────────╯ │
                            │                                                             │
────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────
  a  allow once   n  deny   t  take over input   ⏎  full diff   ?  help                                                    ◆ 1 needs you   q  exit…
```

<details><summary>Styled runs</summary>

```text
 1  st b: '◆ MYTHHELM' · b: 'Add retry to the supervisor beat reader' · chip: ' ◆ NEEDS YOU ' · mu: '· 03:02' · mu: 'claude-code · opus-5.5 · trusted-host'
 2  fa: '────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────'
 3  wa b: 'WAITING' · b: 'Agent A asks to run a command outside its allowed tools' · mu: 'NEXT'
 4  fa: '────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────'
 5  dim: 'TASKS' · fa: '│' · b: 'AGENT A' · mu: '— approval' · fa: '│' · dim: ' EVIDENCE'
 6  wa: '◆' · dim: ' #1 Add retry … waiting' · fa: '│' · ov: '╭─ ' · ov wa b: 'APPROVAL' · ov: ' ─ risk ' · ov chip: ' WRITES OUTSIDE THE SNAPSHOT ' · ov: ' ─────────╮' · fa: '│' · dim: '  ✓ admitted'
 7  fa: '│' · ov: '│ ' · ov mu: 'command' · ov: '  go test -race ./internal/workers -count=50     │' · fa: '│' · wa: '◆' · dim: ' waiting on you'
 8  fa: '│' · ov: '│ ' · ov mu: 'why    ' · ov: '  "prove the retry under load" ' · ov mu: '(agent)' · ov: '           │' · fa: '│'
 9  fa: '│' · ov: '│ ' · ov mu: 'touches' · ov: '  $GOCACHE, outside the run snapshot             │' · fa: '│'
10  fa: '│' · ov: '│                                                         │' · fa: '│'
11  fa: '│' · ov: '│ ' · ov mu: 'pending edit' · ov: '  internal/workers/envelope.go              │' · fa: '│'
12  fa: '│' · ov: '│ ' · ov fa: ' 41' · ov er: '- raw, err := os.ReadFile(path)' · ov: '                     │' · fa: '│'
13  fa: '│' · ov: '│ ' · ov fa: ' 41' · ov ok: '+ raw, err := readBeat(path, transient)' · ov: '             │' · fa: '│'
14  fa: '│' · ov: '│ ' · ov fa: ' 42' · ov ok: '+ // a sharing violation retries, 50ms max' · ov: '          │' · fa: '│'
15  fa: '│' · ov: '│ ' · ov mu: '… 16 more lines · ⏎ opens the full diff' · ov: '                 │' · fa: '│'
16  fa: '│' · ov: '│                                                         │' · fa: '│'
17  fa: '│' · ov: '│ ' · ov chipok: ' a  allow once ' · ov key: ' n  deny ' · ov key: ' t  take over input ' · ov: '        │' · fa: '│'
18  fa: '│' · ov: '╰─────────────────────────────────────────────────────────╯' · fa: '│'
19  fa: '│' · fa: '│'
20  fa: '────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────'
21  key: ' a ' · key: ' n ' · key: ' t ' · key: ' ⏎ ' · key: ' ? ' · wa: '◆ 1 needs you' · key: ' q '
```

</details>

## Frame 1c: Stop requested (not yet confirmed), 100 columns.

- Width: 100 columns, Observatory theme
- Binds: slice 2

```text
◆ MYTHHELM  Add retry to the supervisor beat reader                          ◌ stop requested · 1.4s
────────────────────────────────────────────────────────────────────────────────────────────────────
NOW  Sent interrupt to agent A. Waiting for the worker to confirm.
      The agent may still be writing until confirmed. Nothing is applied.
NEXT terminate at 5s → kill at 15s  (stop ladder v1, pinned at admission)
────────────────────────────────────────────────────────────────────────────────────────────────────
  d  detach (keeps stopping)   ?  help                                                     q  exit…
```

<details><summary>Styled runs</summary>

```text
 1  st b: '◆ MYTHHELM' · b: 'Add retry to the supervisor beat reader' · wa: '◌' · b: 'stop requested' · mu: '· 1.4s'
 2  fa: '────────────────────────────────────────────────────────────────────────────────────────────────────'
 3  mu: 'NOW ' · b: 'Waiting for the worker to confirm.'
 5  mu: 'NEXT' · fa: '(stop ladder v1, pinned at admission)'
 6  fa: '────────────────────────────────────────────────────────────────────────────────────────────────────'
 7  key: ' d ' · key: ' ? ' · key: ' q '
```

</details>

## Frame 1c': Stop confirmed after the worker exited, 100 columns.

- Width: 100 columns, Observatory theme
- Binds: slice 2

```text
◆ MYTHHELM  Add retry to the supervisor beat reader                              stopped · confirmed
────────────────────────────────────────────────────────────────────────────────────────────────────
DONE ✓ worker confirmed the stop at 02:41:07 · process group gone
     ✓ candidate preserved: 1 file changed, not applied
     unknown descendants outside the process group (scan unavailable)
────────────────────────────────────────────────────────────────────────────────────────────────────
  ⏎  review candidate   r  resume as new attempt   ?  help                      exit 130   q  exit…
```

<details><summary>Styled runs</summary>

```text
 1  st b: '◆ MYTHHELM' · b: 'Add retry to the supervisor beat reader' · b: 'stopped' · mu: '· confirmed'
 2  fa: '────────────────────────────────────────────────────────────────────────────────────────────────────'
 3  mu: 'DONE' · ok: '✓'
 4  ok: '✓'
 5  un: 'unknown'
 6  fa: '────────────────────────────────────────────────────────────────────────────────────────────────────'
 7  key: ' ⏎ ' · key: ' r ' · key: ' ? ' · mu: 'exit 130' · key: ' q '
```

</details>

## Frame 1d: Recovery screen: what survived, what was replayed, what is unknown, 100 columns.

- Width: 100 columns, Observatory theme
- Binds: slice 2

```text
◆ MYTHHELM  Add retry to the supervisor beat reader                           ● working · reattached
────────────────────────────────────────────────────────────────────────────────────────────────────
 RECOVERED  Your terminal closed at 02:31. The work kept going.

  ✓ same worker, pid 248169; same agent session; nothing relaunched
  ✓ 2 minutes of activity replayed from the worker spool
  ✓ checks still pinned to the admitted snapshot 33a136c
  unknown tokens used while detached (the native reports them at the end)

────────────────────────────────────────────────────────────────────────────────────────────────────
  ⏎  continue to mission view   l  what happened                                           q  exit…
```

<details><summary>Styled runs</summary>

```text
 1  st b: '◆ MYTHHELM' · b: 'Add retry to the supervisor beat reader' · st: '●' · b: 'working' · mu: '· reattached'
 2  fa: '────────────────────────────────────────────────────────────────────────────────────────────────────'
 3  chipst: ' RECOVERED ' · b: 'Your terminal closed at 02:31. The work kept going.'
 5  ok: '✓'
 6  ok: '✓'
 7  ok: '✓'
 8  un: 'unknown'
10  fa: '────────────────────────────────────────────────────────────────────────────────────────────────────'
11  key: ' ⏎ ' · key: ' l ' · key: ' q '
```

</details>

## Frame 1e: Receipt screen, 148 columns.

- Width: 148 columns, Observatory theme
- Binds: slice 2

```text
◆ MYTHHELM  Add retry to the supervisor beat reader                               ● ready for review · 04:52   claude-code · opus-5.5 · trusted-host
────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────
NOW  The candidate passed 3 of 3 checks. Nothing has been applied to your branch.                            NEXT  review, then a apply or x discard
────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────
CHANGES  in logical order   │ DIFF  internal/workers/envelope.go                          │ RECEIPT
▶ 1 envelope.go    +18 -3   │  38   func ReadSupervisorBeat(dir string) …                 │ PROVEN
  2 beat_read_win.go +22    │  41 -  raw, err := os.ReadFile(path)                        │  ✓ unit tests         go test … exit 0
  3 beat_read_test.go +61   │  41 +  raw, err := readBeat(path, transient)                │  ✓ vet                exit 0
                            │  42 +  // a sharing violation retries, 50ms max             │  ✓ race x50           exit 0 · 26.4s
3 files · +101 -3           │  43      if err != nil …                                    │  on candidate 9f5d393 = snapshot + agent edits
                            │                                                             │ NOT PROVEN
                            │                                                             │  Windows sharing path  no Windows runner here
                            │                                                             │ UNKNOWN
                            │                                                             │  paid continuation   unknown
                            │                                                             │  descendants         unknown  scan unavailable
                            │                                                             │
                            │                                                             │ 41k tokens reported · 4m52s · 1 attempt
────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────
  a  apply to a branch   x  discard   ⏎  open file   e  export receipt   ?  help                                                  exit 0   q  exit…
```

<details><summary>Styled runs</summary>

```text
 1  st b: '◆ MYTHHELM' · b: 'Add retry to the supervisor beat reader' · ok: '●' · b: 'ready for review' · mu: '· 04:52' · mu: 'claude-code · opus-5.5 · trusted-host'
 2  fa: '────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────'
 3  mu: 'NOW ' · b: 'The candidate passed 3 of 3 checks.' · mu: 'Nothing has been applied to your branch.' · mu: 'NEXT' · b: 'a' · b: 'x'
 4  fa: '────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────'
 5  b: 'CHANGES' · mu: 'in logical order' · fa: '│' · b: 'DIFF' · mu: 'internal/workers/envelope.go' · fa: '│' · b: 'RECEIPT'
 6  sel st: '▶' · sel: ' 1 envelope.go    ' · sel ok: '+18' · sel er: '-3' · fa: '│' · fa: ' 38' · fa: '│' · ok: 'PROVEN'
 7  ok: '+22' · fa: '│' · fa: ' 41' · er: '-  raw, err := os.ReadFile(path)' · fa: '│' · ok: '✓' · mu: 'go test … exit 0'
 8  ok: '+61' · fa: '│' · fa: ' 41' · ok: '+  raw, err := readBeat(path, transient)' · fa: '│' · ok: '✓' · mu: 'exit 0'
 9  fa: '│' · fa: ' 42' · ok: '+  // a sharing violation retries, 50ms max' · fa: '│' · ok: '✓' · mu: 'exit 0 · 26.4s'
10  mu: '3 files · +101 -3' · fa: '│' · fa: ' 43' · fa: '│' · mu: 'on candidate 9f5d393 = snapshot + agent edits'
11  fa: '│' · fa: '│' · wa: 'NOT PROVEN'
12  fa: '│' · fa: '│' · mu: 'no Windows runner here'
13  fa: '│' · fa: '│' · mu: 'UNKNOWN'
14  fa: '│' · fa: '│' · un: 'unknown'
15  fa: '│' · fa: '│' · un: 'unknown' · mu: 'scan unavailable'
16  fa: '│' · fa: '│'
17  fa: '│' · fa: '│' · mu: '41k tokens reported · 4m52s · 1 attempt'
18  fa: '────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────'
19  key: ' a ' · key: ' x ' · key: ' ⏎ ' · key: ' e ' · key: ' ? ' · mu: 'exit 0' · key: ' q '
```

</details>

## Frame 2a: Fleet attention queue (MH-14 / MH-24 / MH-29), 148 columns.

- Width: 148 columns, Observatory theme
- Binds: vision, not MH-23

```text
◆ MYTHHELM  Mission: ship protected acceptance slice 2                                        ◆ 2 NEED YOU   4 tasks · 3 agents · 1 writer slot free
────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────
NOW  B and C are building; A is waiting for you.                                                             NEXT  integrate #2 when its checks pass
────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────
ATTENTION QUEUE                   │ LANES                                                  │ INTEGRATION
◆ #3 approval  agent A · 1m       │ A claude-code · opus   ◆ waiting on you                │ #2  ● checking  2 of 4 checks
◆ #4 plan step  needs review      │ B codex · gpt-5      ● editing apply.go                │ #1  ✓ integrated   main+1
● #1 ready to review              │ C opencode · local  ● running tests                    │
● #2 checking                     │                                                        │ serialized: one integration at a time
◌ #5 waits for #2  (depends)      │ resource waits                                         │
                                  │  C waits for the sqlite fixture lock · 18s             │
budget                            │                                                        │
 claude-code team  unknown left   │                                                        │
 codex plus  62% reported 3m ago  │                                                        │
────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────
  ⏎  open   a  approve   tab  next pane   !  next attention   ?  help                                                       ◆ 2 need you   q  exit…
```

<details><summary>Styled runs</summary>

```text
 1  st b: '◆ MYTHHELM' · b: 'Mission: ship protected acceptance slice 2' · chip: ' ◆ 2 NEED YOU ' · mu: '4 tasks · 3 agents · 1 writer slot free'
 2  fa: '────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────'
 3  mu: 'NOW ' · b: 'B and C are building; A is waiting for you.' · mu: 'NEXT'
 4  fa: '────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────'
 5  b: 'ATTENTION QUEUE' · fa: '│' · b: 'LANES' · fa: '│' · b: 'INTEGRATION'
 6  sel wa: '◆' · sel: ' #3 approval  ' · sel mu: 'agent A · 1m' · fa: '│' · wa: 'A' · wa: '◆ waiting on you' · fa: '│' · st: '●' · mu: '2 of 4 checks'
 7  wa: '◆' · mu: 'needs review' · fa: '│' · st: 'B' · st: '●' · fa: '│' · ok: '✓' · mu: 'main+1'
 8  ok: '●' · fa: '│' · st: 'C' · st: '●' · fa: '│'
 9  st: '●' · fa: '│' · fa: '│' · mu: 'serialized: one integration at a time'
10  mu: '◌ #5 waits for #2  (depends)' · fa: '│' · mu: 'resource waits' · fa: '│'
11  fa: '│' · mu: 'C waits for the sqlite fixture lock · 18s' · fa: '│'
12  mu: 'budget' · fa: '│' · fa: '│'
13  mu: 'team' · un: 'unknown' · fa: '│' · fa: '│'
14  mu: 'plus' · mu: 'reported 3m ago' · fa: '│' · fa: '│'
15  fa: '────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────'
16  key: ' ⏎ ' · key: ' a ' · key: ' tab ' · key: ' ! ' · key: ' ? ' · wa: '◆ 2 need you' · key: ' q '
```

</details>

## Frame 2b: Allowance interruption (MH-17), 100 columns.

- Width: 100 columns, Observatory theme
- Binds: vision, not MH-23

```text
◆ MYTHHELM  Add retry to the supervisor beat reader                           ◆ NEEDS YOU  allowance
────────────────────────────────────────────────────────────────────────────────────────────────────
PAUSED claude-code reported its team allowance exhausted at 03:10.
      reset time unknown · no paid continuation was started · work is preserved

   w  wait for the reset (we check, we never guess a time)
   h  hand off to codex · plus  qualified · 62% reported
   s  stop here and review what exists
────────────────────────────────────────────────────────────────────────────────────────────────────
  ?  why these options                                                                     q  exit…
```

<details><summary>Styled runs</summary>

```text
 1  st b: '◆ MYTHHELM' · b: 'Add retry to the supervisor beat reader' · chip: ' ◆ NEEDS YOU ' · mu: 'allowance'
 2  fa: '────────────────────────────────────────────────────────────────────────────────────────────────────'
 3  wa b: 'PAUSED'
 4  mu: '      reset time' · un: 'unknown' · mu: ' · no paid continuation was started · work is preserved'
 6  chipst: ' w '
 7  key: ' h ' · mu: 'qualified · 62% reported'
 8  key: ' s '
 9  fa: '────────────────────────────────────────────────────────────────────────────────────────────────────'
10  key: ' ? ' · key: ' q '
```

</details>
