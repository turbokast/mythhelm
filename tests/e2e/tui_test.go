package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// This file covers the packaged binary's TUI-slice launch rule, flag
// validation and fallback paths end to end (design §15): piped stdio stays
// linear, --plain forces the same linear output, --format jsonl is stable
// under TUI flags, --accessible streams ordered labels, and the error paths
// keep their exit codes. No PTY is required: every leg runs with piped
// stdio, which the launch rule must treat as non-TTY.

// baseRunArgs returns the standard fake-adapter run arguments trusting the
// given project-config digest.
func baseRunArgs(digest string) []string {
	return []string{"run", "--task-file", "task.md", "--adapter", "fake",
		"--billing", "local-scripted", "--execution-profile", "trusted-host",
		"--non-interactive", "--trust-project-config", "sha256:" + digest}
}

// assertNoCursorCodes fails when s carries CSI (escape-[) sequences: linear
// output must never move the cursor or switch screens.
func assertNoCursorCodes(t *testing.T, what, s string) {
	t.Helper()
	if strings.Contains(s, "\x1b[") {
		t.Fatalf("%s contains cursor/control codes: %q", what, s)
	}
}

var (
	// tuiIDRe matches generated identifiers (run_…, att_…, evt_…, …): a
	// lowercase prefix plus 26 Crockford base32 characters.
	tuiIDRe = regexp.MustCompile(`[a-z]+_[0-9A-HJKMNP-TV-Z]{26}`)
	// tuiLinearPIDRe matches worker/native PIDs on the linear
	// attempt.launched line. The renderer prints payload numbers with
	// fmt.Sprint of a float64, which spells PIDs at or above 1e6 in
	// exponent form (2.63474e+06) and smaller ones as plain integers,
	// so both spellings normalise — an exponent-only match fails on
	// low-PID hosts. The pid prefixes anchor the match; an "unknown"
	// value never matches and stays strict.
	tuiLinearPIDRe = regexp.MustCompile(`(worker pid |native pid )\d[\d.eE+-]*`)
	// tuiEvidenceRe matches the short commit directory inside check
	// evidence paths.
	tuiEvidenceRe = regexp.MustCompile(`(evidence[/\\])[0-9a-f]{7,64}([/\\])`)
)

// normalizeLinearRun replaces every per-run value in linear run output —
// state/home/repo paths, generated IDs and worker PIDs — so two runs of the
// same scenario compare byte for byte. Values that must be stable across
// runs (the base revision, check names, the result line) are left strict.
func normalizeLinearRun(s string, e env, repo string) string {
	s = strings.ReplaceAll(s, e.state, "<STATE>")
	s = strings.ReplaceAll(s, repo, "<REPO>")
	s = strings.ReplaceAll(s, e.home, "<HOME>")
	s = tuiIDRe.ReplaceAllString(s, "<ID>")
	return tuiLinearPIDRe.ReplaceAllString(s, "${1}<PID>")
}

// tuiVolatilePayloadKeys holds JSONL payload keys whose values vary run to
// run wherever they appear: launch tokens, process IDs, timestamps and
// commit-dependent values.
var tuiVolatilePayloadKeys = map[string]bool{
	"launch_token_sha256": true,
	"worker_pid":          true,
	"native_pid":          true,
	"native_pgid":         true,
	"worker_start_time":   true,
	"duration_ms":         true,
	"candidate_commit":    true,
}

// normalizeJSONLLine parses one JSONL envelope and returns its canonical
// form: volatile envelope fields and per-run payload values are replaced,
// everything else is kept strict, and encoding/json re-marshals with sorted
// keys so two runs of the same scenario compare byte for byte per line.
func normalizeJSONLLine(t *testing.T, line string, e env, repo string) string {
	t.Helper()
	var ev map[string]any
	if err := json.Unmarshal([]byte(line), &ev); err != nil {
		t.Fatalf("stdout line is not a JSON envelope: %q: %v", line, err)
	}
	for _, k := range []string{"event_id", "run_id", "task_id", "attempt_id", "producer_id", "observed_at"} {
		if _, ok := ev[k]; ok {
			ev[k] = "<" + k + ">"
		}
	}
	typ, _ := ev["type"].(string)
	if p, ok := ev["payload"].(map[string]any); ok {
		ev["payload"] = normalizeJSONLPayload(typ, p, e, repo)
	}
	out, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("re-marshalling %q: %v", line, err)
	}
	return string(out)
}

// normalizeJSONLPayload replaces per-run values in one event payload. The
// receipt digest varies (the receipt embeds run and attempt IDs) while the
// config and patch digests must be stable, so sha256 normalises only on
// receipt.written.
func normalizeJSONLPayload(typ string, p map[string]any, e env, repo string) map[string]any {
	out := make(map[string]any, len(p))
	for k, v := range p {
		switch {
		case tuiVolatilePayloadKeys[k]:
			out[k] = "<" + k + ">"
		case k == "sha256" && typ == "receipt.written":
			out[k] = "<sha256>"
		default:
			out[k] = normalizeJSONLValue(typ, v, e, repo)
		}
	}
	return out
}

func normalizeJSONLValue(typ string, v any, e env, repo string) any {
	switch v := v.(type) {
	case string:
		v = strings.ReplaceAll(v, e.state, "<STATE>")
		v = strings.ReplaceAll(v, repo, "<REPO>")
		v = strings.ReplaceAll(v, e.home, "<HOME>")
		v = tuiIDRe.ReplaceAllString(v, "<ID>")
		return tuiEvidenceRe.ReplaceAllString(v, "${1}<sha>${2}")
	case map[string]any:
		return normalizeJSONLPayload(typ, v, e, repo)
	case []any:
		for i, item := range v {
			v[i] = normalizeJSONLValue(typ, item, e, repo)
		}
		return v
	default:
		return v
	}
}

// parseAccessibleStream splits accessible stdout into its event sequences in
// output order and its trailer cursor, failing unless the last line is the
// next-after trailer.
func parseAccessibleStream(t *testing.T, out string) (seqs []int64, trailer int64) {
	t.Helper()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	n, err := fmt.Sscanf(lines[len(lines)-1], "next-after: %d", &trailer)
	if err != nil || n != 1 {
		t.Fatalf("last accessible line %q is not a next-after trailer:\n%s", lines[len(lines)-1], out)
	}
	for _, line := range lines {
		rest, ok := strings.CutPrefix(line, "event ")
		if !ok {
			continue
		}
		num, _, _ := strings.Cut(rest, ":")
		seq, err := strconv.ParseInt(strings.TrimSpace(num), 10, 64)
		if err != nil {
			t.Fatalf("event line %q has no run sequence: %v", line, err)
		}
		seqs = append(seqs, seq)
	}
	return seqs, trailer
}

// assertContiguousFromOne fails unless seqs is the complete stream 1..N: a
// bounded-memory replay would fail on an old match, and a dropped event
// would break contiguity.
func assertContiguousFromOne(t *testing.T, seqs []int64) {
	t.Helper()
	if len(seqs) == 0 {
		t.Fatal("accessible stream has no events")
	}
	for i, seq := range seqs {
		if want := int64(i + 1); seq != want {
			t.Fatalf("accessible stream seqs start %v, want the contiguous stream from 1", seqs)
		}
	}
}

// assertAccessibleSummary fails unless every summary label opens a line.
// Event lines start with "event ", so a start-of-line match only ever sees
// the summary.
func assertAccessibleSummary(t *testing.T, out string, labels ...string) {
	t.Helper()
	lines := strings.Split(out, "\n")
	for _, want := range labels {
		found := false
		for _, line := range lines {
			if strings.HasPrefix(line, want) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("accessible stream lacks a %q summary line:\n%s", want, out)
		}
	}
}

// TestNormalizeLinearRunPIDs pins both PID spellings the float64 renderer
// emits: exponent form on high-PID hosts, plain integers below 1e6. An
// exponent-only match passes on the former and fails on the latter.
func TestNormalizeLinearRunPIDs(t *testing.T) {
	e := env{home: "/home/e2e", state: "/state/e2e"}
	repo := "/repo/e2e"
	for _, line := range []string{
		"attempt: worker pid 2.63474e+06, native pid 2.63475e+06\n",
		"attempt: worker pid 1234, native pid 5678\n",
	} {
		if got, want := normalizeLinearRun(line, e, repo), "attempt: worker pid <PID>, native pid <PID>\n"; got != want {
			t.Errorf("normalizeLinearRun(%q) = %q, want %q", line, got, want)
		}
	}
	unknown := "attempt: worker pid unknown, native pid unknown\n"
	if got := normalizeLinearRun(unknown, e, repo); got != unknown {
		t.Errorf("normalizeLinearRun(%q) = %q, want it unchanged", unknown, got)
	}
}

// TestE2ENonTTYStaysLinear runs the fake adapter with piped stdout: output
// must be the linear stream with no cursor codes, and the exit code must
// match the linear path's outcome (0 for a passing check, 5 for a failing
// one).
func TestE2ENonTTYStaysLinear(t *testing.T) {
	t.Run("happy", func(t *testing.T) {
		e := newEnv(t)
		repo, digest := mkRepo(t, e, t.TempDir(), "pass")
		code, stdout, stderr := run(t, e, repo, baseRunArgs(digest)...)
		if code != 0 {
			t.Fatalf("run exit %d, stderr %q", code, stderr)
		}
		assertNoCursorCodes(t, "linear stdout", stdout)
		assertNoCursorCodes(t, "linear stderr", stderr)
		first, _, _ := strings.Cut(stdout, "\n")
		if !strings.HasPrefix(first, "run run_") || !strings.HasSuffix(first, ": created") {
			t.Fatalf("first linear line = %q, want 'run run_...: created'", first)
		}
		for _, want := range []string{"admission decided:", "result: ready_for_review; exit 0"} {
			if !strings.Contains(stdout, want) {
				t.Fatalf("linear output lacks %q:\n%s", want, stdout)
			}
		}
	})

	t.Run("failing check", func(t *testing.T) {
		e := newEnv(t)
		repo, digest := mkRepo(t, e, t.TempDir(), "fail")
		code, stdout, stderr := run(t, e, repo, baseRunArgs(digest)...)
		if code != 5 {
			t.Fatalf("run exit %d, want 5; stderr %q", code, stderr)
		}
		assertNoCursorCodes(t, "linear stdout", stdout)
		assertNoCursorCodes(t, "linear stderr", stderr)
		if !strings.Contains(stdout, "result: ") || !strings.Contains(stdout, "exit 5") {
			t.Fatalf("linear output lacks the exit-5 result line:\n%s", stdout)
		}
	})
}

// TestE2EPlainForcesLinear runs one scenario with and without --plain: the
// two outputs must be byte-identical after normalising per-run values (run
// and attempt IDs, PIDs, state paths), and the exits must match. Each leg
// runs in its own env over the same repo, so path normalisation is
// load-bearing too (the base revision stays shared and strict).
func TestE2EPlainForcesLinear(t *testing.T) {
	e := newEnv(t)
	repo, digest := mkRepo(t, e, t.TempDir(), "pass")
	other := newEnv(t)
	base := baseRunArgs(digest)
	plainArgs := append(append([]string{}, base...), "--plain")
	plainCode, plainOut, plainErr := run(t, e, repo, plainArgs...)
	defCode, defOut, defErr := run(t, other, repo, base...)
	if plainCode != defCode {
		t.Fatalf("--plain exit %d != default piped exit %d", plainCode, defCode)
	}
	if plainOut == defOut {
		t.Fatal("raw outputs are identical; expected per-run IDs to differ, proving normalisation is load-bearing")
	}
	normalPlain, normalDef := normalizeLinearRun(plainOut, e, repo), normalizeLinearRun(defOut, other, repo)
	if !bytes.Equal([]byte(normalPlain), []byte(normalDef)) {
		t.Fatalf("--plain output differs from default piped output after normalisation:\n--- plain ---\n%s\n--- default ---\n%s",
			normalPlain, normalDef)
	}
	if plainErr != defErr {
		t.Fatalf("--plain stderr %q != default piped stderr %q", plainErr, defErr)
	}
}

// TestE2EJsonlStable runs one scenario with --format jsonl with and without
// the TUI flags: the two streams must be line-identical after normalising
// per-run values (N4: machine output unchanged), with matching exits. Each
// leg runs in its own env over the same repo, so path normalisation is
// load-bearing too (the base revision stays shared and strict).
func TestE2EJsonlStable(t *testing.T) {
	e := newEnv(t)
	repo, digest := mkRepo(t, e, t.TempDir(), "pass")
	other := newEnv(t)
	base := append(baseRunArgs(digest), "--format", "jsonl")
	plainCode, plainOut, plainErr := run(t, e, repo, base...)
	withTUI := append(append([]string{}, base...),
		"--colour", "always", "--motion", "full", "--icons", "unicode", "--accessible", "--after", "5")
	tuiCode, tuiOut, tuiErr := run(t, other, repo, withTUI...)
	if plainCode != tuiCode {
		t.Fatalf("jsonl with TUI flags exit %d != jsonl exit %d", tuiCode, plainCode)
	}
	plainLines := strings.Split(strings.TrimRight(plainOut, "\n"), "\n")
	tuiLines := strings.Split(strings.TrimRight(tuiOut, "\n"), "\n")
	if len(plainLines) != len(tuiLines) {
		t.Fatalf("jsonl line counts differ: %d without TUI flags vs %d with", len(plainLines), len(tuiLines))
	}
	identical := len(plainLines) > 0
	for i := range plainLines {
		normalPlain := normalizeJSONLLine(t, plainLines[i], e, repo)
		normalTUI := normalizeJSONLLine(t, tuiLines[i], other, repo)
		if normalPlain != normalTUI {
			t.Fatalf("jsonl line %d differs with TUI flags present:\n--- without ---\n%s\n--- with ---\n%s",
				i+1, normalPlain, normalTUI)
		}
		identical = identical && plainLines[i] == tuiLines[i]
	}
	if identical {
		t.Fatal("raw jsonl streams are identical; expected per-run IDs to differ, proving normalisation is load-bearing")
	}
	var last map[string]any
	if err := json.Unmarshal([]byte(tuiLines[len(tuiLines)-1]), &last); err != nil || last["type"] != "run.result" {
		t.Fatalf("jsonl with TUI flags ends in %q, want the run.result envelope", tuiLines[len(tuiLines)-1])
	}
	if plainErr != tuiErr {
		t.Fatalf("jsonl stderr %q != jsonl-with-TUI-flags stderr %q", plainErr, tuiErr)
	}
}

// TestE2EAccessibleStream covers the --accessible label stream on the
// packaged binary: ordered event lines with no cursor codes, the next-after
// trailer naming the last streamed sequence, and an exit code equal to the
// run outcome on both the passing and the failing scenario.
func TestE2EAccessibleStream(t *testing.T) {
	summaryLabels := []string{"goal: ", "run state: ", "attempt", "verification: ",
		"next action: ", "actions: "}

	t.Run("run streams live with the outcome exit", func(t *testing.T) {
		e := newEnv(t)
		repo, digest := mkRepo(t, e, t.TempDir(), "pass")
		linearCode, linearOut, stderr := run(t, e, repo, baseRunArgs(digest)...)
		if linearCode != 0 {
			t.Fatalf("linear run exit %d, stderr %q", linearCode, stderr)
		}
		// The completed run reviews deterministically: full history with
		// final-state labels.
		first, _, _ := strings.Cut(linearOut, "\n")
		runID := strings.TrimSuffix(strings.TrimPrefix(first, "run "), ": created")
		if runID == "" || runID == first {
			t.Fatalf("linear output's first line %q names no run ID", first)
		}
		reviewCode, reviewOut, stderr := run(t, e, repo, "review", runID, "--accessible")
		if reviewCode != 0 {
			t.Fatalf("review --accessible exit %d, stderr %q", reviewCode, stderr)
		}
		assertNoCursorCodes(t, "review accessible stdout", reviewOut)
		assertAccessibleSummary(t, reviewOut, summaryLabels...)
		for _, want := range []string{"run state: ready_for_review", "verification: passed"} {
			if !strings.Contains(reviewOut, want) {
				t.Fatalf("review accessible stream lacks %q:\n%s", want, reviewOut)
			}
		}
		reviewSeqs, reviewTrailer := parseAccessibleStream(t, reviewOut)
		assertContiguousFromOne(t, reviewSeqs)
		if reviewTrailer != reviewSeqs[len(reviewSeqs)-1] {
			t.Fatalf("review trailer next-after = %d, last streamed = %d", reviewTrailer, reviewSeqs[len(reviewSeqs)-1])
		}
		// The live stream carries the same shape with the run's outcome
		// exit; its summary reflects the early snapshot, so only the
		// label set is asserted, not the values.
		liveCode, liveOut, stderr := run(t, e, repo, append(baseRunArgs(digest), "--accessible")...)
		if liveCode != linearCode {
			t.Fatalf("run --accessible exit %d, want the run outcome exit %d (stderr %q)", liveCode, linearCode, stderr)
		}
		assertNoCursorCodes(t, "run accessible stdout", liveOut)
		assertAccessibleSummary(t, liveOut, summaryLabels...)
		liveSeqs, liveTrailer := parseAccessibleStream(t, liveOut)
		assertContiguousFromOne(t, liveSeqs)
		if liveTrailer != liveSeqs[len(liveSeqs)-1] {
			t.Fatalf("live trailer next-after = %d, last streamed = %d", liveTrailer, liveSeqs[len(liveSeqs)-1])
		}
		if strings.Contains(liveOut, "result: ") {
			t.Fatalf("live accessible stream carries a result line; the trailer is its last line:\n%s", liveOut)
		}
	})

	t.Run("failing check exits with the outcome", func(t *testing.T) {
		e := newEnv(t)
		repo, digest := mkRepo(t, e, t.TempDir(), "fail")
		linearCode, _, stderr := run(t, e, repo, baseRunArgs(digest)...)
		if linearCode != 5 {
			t.Fatalf("linear run exit %d, want 5; stderr %q", linearCode, stderr)
		}
		liveCode, liveOut, stderr := run(t, e, repo, append(baseRunArgs(digest), "--accessible")...)
		if liveCode != linearCode {
			t.Fatalf("run --accessible exit %d, want the run outcome exit %d (stderr %q)", liveCode, linearCode, stderr)
		}
		assertNoCursorCodes(t, "run accessible stdout", liveOut)
		assertAccessibleSummary(t, liveOut, summaryLabels...)
		liveSeqs, liveTrailer := parseAccessibleStream(t, liveOut)
		assertContiguousFromOne(t, liveSeqs)
		if liveTrailer != liveSeqs[len(liveSeqs)-1] {
			t.Fatalf("live trailer next-after = %d, last streamed = %d", liveTrailer, liveSeqs[len(liveSeqs)-1])
		}
	})
}

// TestE2EInvalidFlagsExit2 runs each invalid presentation-flag value against
// the packaged binary: every one must exit 2 naming its flag.
func TestE2EInvalidFlagsExit2(t *testing.T) {
	e := newEnv(t)
	repo, digest := mkRepo(t, e, t.TempDir(), "pass")
	runBase := baseRunArgs(digest)
	tests := []struct {
		name string
		args []string
		flag string
	}{
		{"run colour", append(append([]string{}, runBase...), "--colour", "rainbow"), "--colour"},
		{"run motion", append(append([]string{}, runBase...), "--motion", "turbo"), "--motion"},
		{"run icons", append(append([]string{}, runBase...), "--icons", "emoji"), "--icons"},
		{"run after", append(append([]string{}, runBase...), "--after", "xyz"), "--after"},
		{"review colour", []string{"review", "run_missing", "--colour", "rainbow"}, "--colour"},
		{"review motion", []string{"review", "run_missing", "--motion", "turbo"}, "--motion"},
		{"review icons", []string{"review", "run_missing", "--icons", "emoji"}, "--icons"},
		{"review after", []string{"review", "run_missing", "--after", "xyz"}, "--after"},
		{"demo colour", []string{"demo", "--colour", "rainbow"}, "--colour"},
		{"demo motion", []string{"demo", "--motion", "turbo"}, "--motion"},
		{"demo icons", []string{"demo", "--icons", "emoji"}, "--icons"},
		{"demo after", []string{"demo", "--after", "xyz"}, "--after"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, _, stderr := run(t, e, repo, tt.args...)
			if code != 2 {
				t.Fatalf("%q exit %d, want 2", tt.args, code)
			}
			if !strings.Contains(stderr, tt.flag) {
				t.Fatalf("%q stderr %q names no %s", tt.args, stderr, tt.flag)
			}
		})
	}
}

// TestE2EReviewUnknownExits1 reviews a missing run on the packaged binary:
// exit 1 with the not-found message, whether or not a state database exists.
func TestE2EReviewUnknownExits1(t *testing.T) {
	t.Run("no database", func(t *testing.T) {
		e := newEnv(t)
		code, stdout, stderr := run(t, e, t.TempDir(), "review", "run_missing")
		if code != 1 || stdout != "" || stderr != "mythhelm review: run run_missing: not found\n" {
			t.Fatalf("review run_missing = %d/%q/%q, want exit 1 with the not-found message",
				code, stdout, stderr)
		}
	})

	t.Run("with database", func(t *testing.T) {
		e := newEnv(t)
		repo, digest := mkRepo(t, e, t.TempDir(), "pass")
		if code, _, stderr := run(t, e, repo, baseRunArgs(digest)...); code != 0 {
			t.Fatalf("setup run exit %d, stderr %q", code, stderr)
		}
		code, stdout, stderr := run(t, e, repo, "review", "run_missing")
		if code != 1 || stdout != "" || stderr != "mythhelm review: journal: run run_missing: not found\n" {
			t.Fatalf("review run_missing = %d/%q/%q, want exit 1 with the not-found message",
				code, stdout, stderr)
		}
	})
}
