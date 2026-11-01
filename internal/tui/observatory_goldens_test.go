package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/tui/theme"
	"github.com/turbokast/mythhelm/internal/tui/viewmodel"
)

// These complete, literal cell captures are transcribed from the adopted
// frame worksheet and the fixed synthetic facts below. They do not come from
// View output. Only invisible right padding is normalised; interior spacing,
// every body row, blank row, rule and final key position remain significant.
func TestObservatoryLadderGoldens(t *testing.T) {
	t.Parallel()
	cases := []struct {
		width, height int
		ascii         bool
		want          string
	}{
		{width: 160, height: 40, ascii: false, want: `◆ MYTHHELM Task ● working 2:14                                                                                                         adapter · model · profile
────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────
NOW lane A adapter; running; 14 assistant turns (native-reported); target unkn…  NEXT agent working; no action available
────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────
▶ TASKS                        │   AGENT                                                       │   EVIDENCE
────────────────────────────── │ ───────────────────────────────────────────────────────────── │ ───────────────────────────────────────────────────────────────
▶ run_g ● working Task         │ route: adapter 1 (local)                                      │ ✓ admitted
                               │ route reason: pinned by --adapter                             │ ● working
                               │ agent: adapter · model · profile                              │ ○ frozen
                               │ attempt 1: running                                            │ ○ checked: not run
                               │ native-reported: 14 assistant turns; retries: 0; target: unk… │ ○ ready
                               │ tool Bash: 3 (native-reported)                                │ tokens / input: 41000 (reported); source: fixture
                               │ tool Edit: 2 (native-reported)                                │ allowance / tokens: unknown (unknown); source: fixture
                               │ tool Read: 7 (native-reported)                                │ Billing: local-scripted
                               │ [v] full stream · summary                                     │ qualified: true
                               │                                                               │ paid continuation: off
                               │                                                               │ entitlement: user-declared
                               │                                                               │ native exit: unknown
                               │                                                               │ verification: waiting



















────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────
Enter inspect  ? help  : commands  / filter  nothing needs you                                                                                           q exit…`},
		{width: 160, height: 40, ascii: true, want: `# MYTHHELM Task * working 2:14                                                                                                         adapter | model | profile
----------------------------------------------------------------------------------------------------------------------------------------------------------------
NOW lane A adapter; running; 14 assistant turns (native-reported); target unkn+  NEXT agent working; no action available
----------------------------------------------------------------------------------------------------------------------------------------------------------------
> TASKS                        |   AGENT                                                       |   EVIDENCE
------------------------------ | ------------------------------------------------------------- | ---------------------------------------------------------------
> run_g * working Task         | route: adapter 1 (local)                                      | v admitted
                               | route reason: pinned by --adapter                             | * working
                               | agent: adapter | model | profile                              | o frozen
                               | attempt 1: running                                            | o checked: not run
                               | native-reported: 14 assistant turns; retries: 0; target: unk+ | o ready
                               | tool Bash: 3 (native-reported)                                | tokens / input: 41000 (reported); source: fixture
                               | tool Edit: 2 (native-reported)                                | allowance / tokens: unknown (unknown); source: fixture
                               | tool Read: 7 (native-reported)                                | Billing: local-scripted
                               | [v] full stream | summary                                     | qualified: true
                               |                                                               | paid continuation: off
                               |                                                               | entitlement: user-declared
                               |                                                               | native exit: unknown
                               |                                                               | verification: waiting



















----------------------------------------------------------------------------------------------------------------------------------------------------------------
Enter inspect  ? help  : commands  / filter  nothing needs you                                                                                           q exit+`},
		{width: 148, height: 40, ascii: false, want: `◆ MYTHHELM Task ● working 2:14                                                                                             adapter · model · profile
────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────
NOW lane A adapter; running; 14 assistant turns (native-reported); targe…  NEXT agent working; no action available
────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────
▶ TASKS                      │   AGENT                                                  │   EVIDENCE
──────────────────────────── │ ──────────────────────────────────────────────────────── │ ──────────────────────────────────────────────────────────
▶ run_g ● working Task       │ route: adapter 1 (local)                                 │ ✓ admitted
                             │ route reason: pinned by --adapter                        │ ● working
                             │ agent: adapter · model · profile                         │ ○ frozen
                             │ attempt 1: running                                       │ ○ checked: not run
                             │ native-reported: 14 assistant turns; retries: 0; target… │ ○ ready
                             │ tool Bash: 3 (native-reported)                           │ tokens / input: 41000 (reported); source: fixture
                             │ tool Edit: 2 (native-reported)                           │ allowance / tokens: unknown (unknown); source: fixture
                             │ tool Read: 7 (native-reported)                           │ Billing: local-scripted
                             │ [v] full stream · summary                                │ qualified: true
                             │                                                          │ paid continuation: off
                             │                                                          │ entitlement: user-declared
                             │                                                          │ native exit: unknown
                             │                                                          │ verification: waiting



















────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────
Enter inspect  ? help  : commands  / filter  nothing needs you                                                                               q exit…`},
		{width: 148, height: 40, ascii: true, want: `# MYTHHELM Task * working 2:14                                                                                             adapter | model | profile
----------------------------------------------------------------------------------------------------------------------------------------------------
NOW lane A adapter; running; 14 assistant turns (native-reported); targe+  NEXT agent working; no action available
----------------------------------------------------------------------------------------------------------------------------------------------------
> TASKS                      |   AGENT                                                  |   EVIDENCE
---------------------------- | -------------------------------------------------------- | ----------------------------------------------------------
> run_g * working Task       | route: adapter 1 (local)                                 | v admitted
                             | route reason: pinned by --adapter                        | * working
                             | agent: adapter | model | profile                         | o frozen
                             | attempt 1: running                                       | o checked: not run
                             | native-reported: 14 assistant turns; retries: 0; target+ | o ready
                             | tool Bash: 3 (native-reported)                           | tokens / input: 41000 (reported); source: fixture
                             | tool Edit: 2 (native-reported)                           | allowance / tokens: unknown (unknown); source: fixture
                             | tool Read: 7 (native-reported)                           | Billing: local-scripted
                             | [v] full stream | summary                                | qualified: true
                             |                                                          | paid continuation: off
                             |                                                          | entitlement: user-declared
                             |                                                          | native exit: unknown
                             |                                                          | verification: waiting



















----------------------------------------------------------------------------------------------------------------------------------------------------
Enter inspect  ? help  : commands  / filter  nothing needs you                                                                               q exit+`},
		{width: 120, height: 30, ascii: false, want: `◆ MYTHHELM Task ● working 2:14
────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────
NOW lane A adapter; running; 14 assistant turns (native-re…  NEXT agent working; no action available
────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────
▶ TASKS                                 │   AGENT
─────────────────────────────────────── │ ──────────────────────────────────────────────────────────────────────────────
▶ run_g ● working Task                  │ route: adapter 1 (local)
✓ admitted                              │ route reason: pinned by --adapter
● working                               │ agent: adapter · model · profile
○ frozen                                │ attempt 1: running
○ checked: not run                      │ native-reported: 14 assistant turns; retries: 0; target: unknown
○ ready                                 │ tool Bash: 3 (native-reported)
                                        │ tool Edit: 2 (native-reported)
                                        │ tool Read: 7 (native-reported)
                                        │ [v] full stream · summary













────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────
Enter inspect  ? help  : commands  / filter  nothing needs you                                                   q exit…`},
		{width: 120, height: 30, ascii: true, want: `# MYTHHELM Task * working 2:14
------------------------------------------------------------------------------------------------------------------------
NOW lane A adapter; running; 14 assistant turns (native-re+  NEXT agent working; no action available
------------------------------------------------------------------------------------------------------------------------
> TASKS                                 |   AGENT
--------------------------------------- | ------------------------------------------------------------------------------
> run_g * working Task                  | route: adapter 1 (local)
v admitted                              | route reason: pinned by --adapter
* working                               | agent: adapter | model | profile
o frozen                                | attempt 1: running
o checked: not run                      | native-reported: 14 assistant turns; retries: 0; target: unknown
o ready                                 | tool Bash: 3 (native-reported)
                                        | tool Edit: 2 (native-reported)
                                        | tool Read: 7 (native-reported)
                                        | [v] full stream | summary













------------------------------------------------------------------------------------------------------------------------
Enter inspect  ? help  : commands  / filter  nothing needs you                                                   q exit+`},
		{width: 110, height: 24, ascii: false, want: `◆ MYTHHELM Task ● working 2:14
──────────────────────────────────────────────────────────────────────────────────────────────────────────────
NOW lane A adapter; running; 14 assistant turns (nati…  NEXT agent working; no action available
──────────────────────────────────────────────────────────────────────────────────────────────────────────────
▶ TASKS                             │   AGENT
─────────────────────────────────── │ ────────────────────────────────────────────────────────────────────────
▶ run_g ● working Task              │ route: adapter 1 (local)
✓ admitted                          │ route reason: pinned by --adapter
● working                           │ agent: adapter · model · profile
○ frozen                            │ attempt 1: running
○ checked: not run                  │ native-reported: 14 assistant turns; retries: 0; target: unknown
○ ready                             │ tool Bash: 3 (native-reported)
                                    │ tool Edit: 2 (native-reported)
                                    │ tool Read: 7 (native-reported)
                                    │ [v] full stream · summary







──────────────────────────────────────────────────────────────────────────────────────────────────────────────
Enter inspect  ? help  : commands  / filter  nothing needs you                                         q exit…`},
		{width: 110, height: 24, ascii: true, want: `# MYTHHELM Task * working 2:14
--------------------------------------------------------------------------------------------------------------
NOW lane A adapter; running; 14 assistant turns (nati+  NEXT agent working; no action available
--------------------------------------------------------------------------------------------------------------
> TASKS                             |   AGENT
----------------------------------- | ------------------------------------------------------------------------
> run_g * working Task              | route: adapter 1 (local)
v admitted                          | route reason: pinned by --adapter
* working                           | agent: adapter | model | profile
o frozen                            | attempt 1: running
o checked: not run                  | native-reported: 14 assistant turns; retries: 0; target: unknown
o ready                             | tool Bash: 3 (native-reported)
                                    | tool Edit: 2 (native-reported)
                                    | tool Read: 7 (native-reported)
                                    | [v] full stream | summary







--------------------------------------------------------------------------------------------------------------
Enter inspect  ? help  : commands  / filter  nothing needs you                                         q exit+`},
		{width: 90, height: 24, ascii: false, want: `◆ Task ● working 2:14
──────────────────────────────────────────────────────────────────────────────────────────
NOW lane A adapter; running; 14 assistant t…  NEXT agent working; no action available
Tasks  ▶ Agent  Evidence  Diff
──────────────────────────────────────────────────────────────────────────────────────────
▶ AGENT
──────────────────────────────────────────────────────────────────────────────────────────
route: adapter 1 (local)
route reason: pinned by --adapter
agent: adapter · model · profile
attempt 1: running
native-reported: 14 assistant turns; retries: 0; target: unknown
tool Bash: 3 (native-reported)
tool Edit: 2 (native-reported)
tool Read: 7 (native-reported)
[v] full stream · summary






──────────────────────────────────────────────────────────────────────────────────────────
Enter inspect  ? help                                                              q exit…`},
		{width: 90, height: 24, ascii: true, want: `# Task * working 2:14
------------------------------------------------------------------------------------------
NOW lane A adapter; running; 14 assistant t+  NEXT agent working; no action available
Tasks  > Agent  Evidence  Diff
------------------------------------------------------------------------------------------
> AGENT
------------------------------------------------------------------------------------------
route: adapter 1 (local)
route reason: pinned by --adapter
agent: adapter | model | profile
attempt 1: running
native-reported: 14 assistant turns; retries: 0; target: unknown
tool Bash: 3 (native-reported)
tool Edit: 2 (native-reported)
tool Read: 7 (native-reported)
[v] full stream | summary






------------------------------------------------------------------------------------------
Enter inspect  ? help                                                              q exit+`},
		{width: 88, height: 24, ascii: false, want: `◆ Task ● working 2:14
────────────────────────────────────────────────────────────────────────────────────────
NOW lane A adapter; running; 14 assistant …  NEXT agent working; no action available
Tasks  ▶ Agent  Evidence  Diff
────────────────────────────────────────────────────────────────────────────────────────
▶ AGENT
────────────────────────────────────────────────────────────────────────────────────────
route: adapter 1 (local)
route reason: pinned by --adapter
agent: adapter · model · profile
attempt 1: running
native-reported: 14 assistant turns; retries: 0; target: unknown
tool Bash: 3 (native-reported)
tool Edit: 2 (native-reported)
tool Read: 7 (native-reported)
[v] full stream · summary






────────────────────────────────────────────────────────────────────────────────────────
Enter inspect  ? help                                                            q exit…`},
		{width: 88, height: 24, ascii: true, want: `# Task * working 2:14
----------------------------------------------------------------------------------------
NOW lane A adapter; running; 14 assistant +  NEXT agent working; no action available
Tasks  > Agent  Evidence  Diff
----------------------------------------------------------------------------------------
> AGENT
----------------------------------------------------------------------------------------
route: adapter 1 (local)
route reason: pinned by --adapter
agent: adapter | model | profile
attempt 1: running
native-reported: 14 assistant turns; retries: 0; target: unknown
tool Bash: 3 (native-reported)
tool Edit: 2 (native-reported)
tool Read: 7 (native-reported)
[v] full stream | summary






----------------------------------------------------------------------------------------
Enter inspect  ? help                                                            q exit+`},
		{width: 79, height: 24, ascii: false, want: `◆ Task [fail] blocked 2:14
now blocked; no checks recorded
next reason: synthetic refusal; read the reason; start a new run
use tokens/input 41000 (reported); allowance/tokens unknown (unknown)
verification: waiting
Billing: local-scripted  │ qualified: true  │ paid continuation: off  │ native…
entitlement: user-declared
















Enter inspect  ? help  ◆ 1 needs you              q exit…  --accessible: linear`},
		{width: 79, height: 24, ascii: true, want: `# Task [fail] blocked 2:14
now blocked; no checks recorded
next reason: synthetic refusal; read the reason; start a new run
use tokens/input 41000 (reported); allowance/tokens unknown (unknown)
verification: waiting
Billing: local-scripted  | qualified: true  | paid continuation: off  | native+
entitlement: user-declared
















Enter inspect  ? help  # 1 needs you              q exit+  --accessible: linear`},
		{width: 72, height: 24, ascii: false, want: `◆ Task [fail] blocked 2:14
now blocked; no checks recorded
next reason: synthetic refusal; read the reason; start a new run
use tokens/input 41000 (reported); allowance/tokens unknown (unknown)
verification: waiting
Billing: local-scripted  │ qualified: true  │ paid continuation: off  │…
entitlement: user-declared
















Enter inspect  ? help  ◆ 1 needs you       q exit…  --accessible: linear`},
		{width: 72, height: 24, ascii: true, want: `# Task [fail] blocked 2:14
now blocked; no checks recorded
next reason: synthetic refusal; read the reason; start a new run
use tokens/input 41000 (reported); allowance/tokens unknown (unknown)
verification: waiting
Billing: local-scripted  | qualified: true  | paid continuation: off  |+
entitlement: user-declared
















Enter inspect  ? help  # 1 needs you       q exit+  --accessible: linear`},
		{width: 59, height: 24, ascii: false, want: `◆ Task [fail] blocked 2:14
now blocked; no checks recorded
next reason: synthetic refusal; read the reason; start a n…
use tokens/input 41000 (reported); allowance/tokens unknow…
verification: waiting
Billing: local-scripted  │ qualified: true  │ paid continu…
entitlement: user-declared
















Enter  ?  ◆ 1 needs you       q exit…  --accessible: linear`},
		{width: 59, height: 24, ascii: true, want: `# Task [fail] blocked 2:14
now blocked; no checks recorded
next reason: synthetic refusal; read the reason; start a n+
use tokens/input 41000 (reported); allowance/tokens unknow+
verification: waiting
Billing: local-scripted  | qualified: true  | paid continu+
entitlement: user-declared
















Enter  ?  # 1 needs you       q exit+  --accessible: linear`},
		{width: 160, height: 9, ascii: false, want: `◆ MYTHHELM Task ● working 2:14                                                                                                         adapter · model · profile
now lane A adapter; running; 14 assistant turns (native-reported); target unknown
next agent working; no action available
use tokens/input 41000 (reported); allowance/tokens unknown (unknown)
verification: waiting
Billing: local-scripted  │ qualified: true  │ paid continuation: off  │ native exit: unknown
entitlement: user-declared

Enter inspect  ? help  : commands  / filter  nothing needs you                                                                     q exit…  --accessible: linear`},
		{width: 160, height: 9, ascii: true, want: `# MYTHHELM Task * working 2:14                                                                                                         adapter | model | profile
now lane A adapter; running; 14 assistant turns (native-reported); target unknown
next agent working; no action available
use tokens/input 41000 (reported); allowance/tokens unknown (unknown)
verification: waiting
Billing: local-scripted  | qualified: true  | paid continuation: off  | native exit: unknown
entitlement: user-declared

Enter inspect  ? help  : commands  / filter  nothing needs you                                                                     q exit+  --accessible: linear`},
		{width: 100, height: 24, ascii: false, want: `◆ MYTHHELM Task ◌ ready, unverified 2:14
────────────────────────────────────────────────────────────────────────────────────────────────────
NOW ready, unverified; passed 1 of 2 checks        NEXT review the candidate
────────────────────────────────────────────────────────────────────────────────────────────────────
PROVEN
✓ build
NOT PROVEN
test: failed; synthetic failure
UNKNOWN
paid continuation: unknown
native exit: unknown











────────────────────────────────────────────────────────────────────────────────────────────────────
e export  Enter inspect  ? help  nothing needs you                                           q exit…`},
		{width: 100, height: 24, ascii: true, want: `# MYTHHELM Task . ready, unverified 2:14
----------------------------------------------------------------------------------------------------
NOW ready, unverified; passed 1 of 2 checks        NEXT review the candidate
----------------------------------------------------------------------------------------------------
PROVEN
v build
NOT PROVEN
test: failed; synthetic failure
UNKNOWN
paid continuation: unknown
native exit: unknown











----------------------------------------------------------------------------------------------------
e export  Enter inspect  ? help  nothing needs you                                           q exit+`},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%dx%d/ascii=%t", tc.width, tc.height, tc.ascii), func(t *testing.T) {
			t.Parallel()
			m := ladderGoldenFixture(t, tc.width, tc.height, tc.ascii)
			actual := stripANSISequences(m.View())
			rows := strings.Split(actual, "\n")
			for _, row := range rows {
				if lipgloss.Width(row) > tc.width {
					t.Fatalf("row exceeds%d cells: %q", tc.width, row)
				}
			}
			for i := range rows {
				rows[i] = strings.TrimRight(rows[i], " ")
			}
			got := strings.Join(rows, "\n")
			if got != tc.want {
				expected := strings.Split(tc.want, "\n")
				if len(rows) != len(expected) {
					t.Fatalf("capture rows=%d want%d", len(rows), len(expected))
				}
				for i := range rows {
					if rows[i] != expected[i] {
						t.Fatalf("frame row%d\n got: %q\nwant: %q", i+1, rows[i], expected[i])
					}
				}
			}
		})
	}
}
func ladderGoldenFixture(t *testing.T, width, height int, ascii bool) *Model {
	t.Helper()
	c := fullCaps
	if ascii {
		c = asciiCaps
	}
	cfg := testConfig(c, t)
	if width == 100 {
		tokens, err := theme.BuiltIn("light")
		if err != nil {
			t.Fatal(err)
		}
		cfg.Tokens = tokens
		cfg.ThemeName = "light"
	}
	m := newModel(cfg)
	m.width, m.height = width, height
	m.ready = true
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	m.snap = viewmodel.Snapshot{
		Run:  journal.RunRow{RunID: "run_g", State: "executing", AdapterID: "adapter", ExecutionProfile: "profile", BillingPosture: "local-scripted", CreatedAt: start, UpdatedAt: start.Add(134 * time.Second)},
		Goal: "Task", Model: "model", At: start.Add(134 * time.Second), Supervision: "supervised",
		Attempt:        &journal.AttemptRow{AttemptID: "att_g", RunID: "run_g", TaskID: "task_g", AttemptNumber: 1, State: "running"},
		Admission:      &viewmodel.Admission{AdapterID: "adapter", AdapterVersion: "1", AdapterSurface: "local", RunSeq: 1, Qualified: true, PaidContinuation: "off", EntitlementStatement: "entitlement: user-declared"},
		LatestProgress: &viewmodel.Progress{AssistantTurns: 14, ToolUses: map[string]int{"Read": 7, "Edit": 2, "Bash": 3}, RunSeq: 2},
		Usage:          []journal.UsageRow{{Scope: "tokens", Unit: "input", Quantity: "41000", Label: "reported", Source: "fixture"}, {Scope: "allowance", Unit: "tokens", Quantity: "unknown", Label: "unknown", Source: "fixture"}},
	}
	m.selectedTask, m.selectedLane = "task_g", "att_g"
	if width < 80 {
		m.snap.Run.State = "blocked"
		m.snap.Run.Reason = "synthetic refusal"
		m.snap.Attempt.State = "failed_native"
	}
	if width == 88 || width == 90 {
		m.focusPane = paneAgents
	}
	if width == 100 {
		m.snap.Run.State = "ready_for_review"
		m.snap.Attempt.State = "succeeded_native"
		m.snap.Admission.PaidContinuation = "unknown"
		commit := strings.Repeat("c", 40)
		m.snap.Candidate = &journal.CandidateRow{Commit: commit, BaseRev: strings.Repeat("b", 40)}
		m.snap.Verification = &journal.VerificationRow{CandidateCommit: commit, Result: "failed", Reason: "synthetic failure", Checks: []journal.CheckRow{{Name: "build", Status: "passed"}, {Name: "test", Status: "failed"}}}
	}
	return m
}
func TestObservatoryHeightBoundary(t *testing.T) {
	t.Parallel()
	for _, size := range [][2]int{{160, 10}, {160, 9}, {72, 5}} {
		m := readyModel(t, fullCaps, size[0], size[1])
		m.snap.Run.State = "blocked"
		view := stripANSISequences(m.View())
		rows := strings.Split(view, "\n")
		if len(rows) != size[1] || !strings.Contains(rows[len(rows)-1], "q exit") {
			t.Fatalf("mandatory chrome lost at %v: %q", size, view)
		}
		if size[1] == 10 && !strings.Contains(view, "EVIDENCE") {
			t.Fatal("ten rows lost width-driven panes")
		}
		if size[1] < 10 {
			for _, text := range []string{"now ", "next ", "use ", "--accessible: linear"} {
				if !strings.Contains(view, text) {
					t.Fatalf("compact missing %s at %v", text, size)
				}
			}
		}
	}
}
func TestHiddenKeysStillWork(t *testing.T) {
	t.Parallel()
	m := readyModel(t, fullCaps, 88, 24)
	m.handleKey(runeMsg(":"))
	if !m.palette.open {
		t.Fatal("hidden palette key stopped working")
	}
	m.handleKey(specialMsg(tea.KeyEsc))
	m.handleKey(runeMsg("/"))
	if !m.filterOn {
		t.Fatal("hidden filter key stopped working")
	}
	m.handleKey(specialMsg(tea.KeyEsc))
	m.handleKey(runeMsg("?"))
	help := stripANSISequences(m.View())
	for _, text := range []string{":", "/", "v", "disabled:"} {
		if !strings.Contains(help, text) {
			t.Fatalf("help hides %q: %s", text, help)
		}
	}
}

func TestDiffWithoutCandidateHasNoTruncationNotice(t *testing.T) {
	t.Parallel()
	m := readyModel(t, fullCaps, 160, 30)
	m.snap.Candidate = nil
	m.diff.Truncated = true
	m.openDetails()
	defer func() {
		if value := recover(); value != nil {
			t.Errorf("missing candidate crashed actual View: %v", value)
		}
	}()
	view := stripANSISequences(m.View())
	if !strings.Contains(view, "no candidate frozen yet") || strings.Contains(view, "full diff via:") {
		t.Fatalf("missing candidate fabricates a diff fallback: %s", view)
	}
}

func TestMissionKeepsExplicitUnsupervisedNow(t *testing.T) {
	t.Parallel()
	for _, state := range []string{"launch_intent_recorded", "launching", "running", "stop_requested"} {
		t.Run(state, func(t *testing.T) {
			t.Parallel()
			m := observatoryFixture(t)
			m.cfg.Mode = ModeInspect
			m.width, m.height = 160, 30
			m.snap.Run.State = "executing"
			m.snap.Attempt.State = state
			m.snap.LatestProgress = nil
			m.snap.Supervision = "unsupervised"
			rows := strings.Split(stripANSISequences(m.View()), "\n")
			if !strings.Contains(rows[2], "unsupervised") || strings.Contains(rows[2], "dispatching") || !strings.Contains(rows[2], "mythhelm recover") {
				t.Fatalf("NOW lost explicit owner observation: %q", rows[2])
			}
		})
	}
}
