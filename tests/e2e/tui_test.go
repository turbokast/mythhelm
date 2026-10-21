package e2e

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/workers"
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
// run wherever they appear: launch tokens, worker nonces, process IDs,
// timestamps and commit-dependent values.
var tuiVolatilePayloadKeys = map[string]bool{
	"launch_token_sha256": true,
	"nonce_sha256":        true,
	"worker_pid":          true,
	"native_pid":          true,
	"native_pgid":         true,
	"worker_start_time":   true,
	"duration_ms":         true,
	"candidate_commit":    true,
	"attestation_digest":  true,
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
		case k == "native_start_time" && typ == "attempt.launched":
			out[k] = "<native_start_time>"
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
// the TUI flags: deterministic events must be line-identical after normalising
// per-run values (N4: machine output unchanged), with matching exits. Each
// leg runs in its own env over the same repo. Stop observations can differ
// between processes: each complete stopped envelope must match its own durable
// journal exactly, including signals, confirmation and identities. Launch
// observations also match worker.json before their creation time is normalised.
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
		if assertDurableEnvelope(t, plainLines[i], e, "attempt.stopped") {
			if !assertDurableEnvelope(t, tuiLines[i], other, "attempt.stopped") {
				t.Fatal("stopped event order differs with TUI flags")
			}
			identical = identical && plainLines[i] == tuiLines[i]
			continue
		}
		if assertDurableLaunchEnvelope(t, plainLines[i], e) && !assertDurableLaunchEnvelope(t, tuiLines[i], other) {
			t.Fatal("launched event order differs with TUI flags")
		}
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

// compareDurableEnvelope compares complete JSON values without normalising
// process evidence or dropping unknown fields.
func compareDurableEnvelope(actual, durable []byte) error {
	var got, want any
	actualDecoder := json.NewDecoder(bytes.NewReader(actual))
	actualDecoder.UseNumber()
	if err := actualDecoder.Decode(&got); err != nil {
		return err
	}
	durableDecoder := json.NewDecoder(bytes.NewReader(durable))
	durableDecoder.UseNumber()
	if err := durableDecoder.Decode(&want); err != nil {
		return err
	}
	if !reflect.DeepEqual(got, want) {
		return fmt.Errorf("event envelope differs from durable journal: got %s, want %s", actual, durable)
	}
	return nil
}

func assertDurableEnvelope(t *testing.T, line string, e env, eventType string) bool {
	t.Helper()
	var event journal.Event
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		t.Fatal(err)
	}
	if event.Type != eventType {
		return false
	}
	j := ledgerOpenState(t, e)
	events, err := j.Events(t.Context(), event.RunID, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, durable := range events {
		if durable.EventID != event.EventID {
			continue
		}
		encoded, err := json.Marshal(durable)
		if err != nil {
			t.Fatal(err)
		}
		if err := compareDurableEnvelope([]byte(line), encoded); err != nil {
			t.Fatal(err)
		}
		return true
	}
	t.Fatalf("printed %s event absent from durable journal", eventType)
	return false
}

func TestStoppedEnvelopeKeepsStopEvidence(t *testing.T) {
	empty := []byte(`{"type":"attempt.stopped","event_id":"evt-one","payload":{"confirmed":true,"sent":[],"signals_sent":[],"ladder_version":"stop-ladder/v2-windows","descendant_scan":"job-object"}}`)
	killed := []byte(`{"type":"attempt.stopped","event_id":"evt-one","payload":{"confirmed":true,"sent":[{"signal":"kill","grace":5000000000}],"signals_sent":["kill"],"ladder_version":"stop-ladder/v2-windows","descendant_scan":"job-object"}}`)
	for _, durable := range [][]byte{empty, killed} {
		if err := compareDurableEnvelope(durable, durable); err != nil {
			t.Fatal(err)
		}
		for name, altered := range map[string][]byte{
			"confirmed":      bytes.Replace(durable, []byte(`"confirmed":true`), []byte(`"confirmed":false`), 1),
			"event identity": bytes.Replace(durable, []byte(`evt-one`), []byte(`evt-other`), 1),
			"extra field":    bytes.Replace(durable, []byte(`"sent":`), []byte(`"unexpected":true,"sent":`), 1),
		} {
			if err := compareDurableEnvelope(altered, durable); err == nil {
				t.Errorf("accepted altered %s", name)
			}
		}
	}
	if err := compareDurableEnvelope(empty, killed); err == nil {
		t.Error("accepted missing kill evidence")
	}
	if err := compareDurableEnvelope(killed, empty); err == nil {
		t.Error("accepted invented kill evidence")
	}
}

func assertDurableLaunchEnvelope(t *testing.T, line string, e env) bool {
	t.Helper()
	if !assertDurableEnvelope(t, line, e, "attempt.launched") {
		return false
	}
	var event journal.Event
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		t.Fatal(err)
	}
	id, err := workers.ReadIdentity(workers.AttemptDir(e.state, event.RunID, event.AttemptID))
	if err != nil {
		t.Fatal(err)
	}
	if err := validateLaunchedIdentity(event, id, runtime.GOOS); err != nil {
		t.Fatal(err)
	}
	return true
}

// validateLaunchedIdentity binds the displayed/journaled launch to worker.json
// before any process observation is normalised for independent-run comparison.
func validateLaunchedIdentity(event journal.Event, id workers.Identity, goos string) error {
	if event.Type != "attempt.launched" || event.RunID != id.RunID || event.AttemptID != id.AttemptID {
		return errors.New("launched event does not match the persisted attempt")
	}
	var payload struct {
		WorkerPID       int        `json:"worker_pid"`
		WorkerStartTime time.Time  `json:"worker_start_time"`
		NativePID       *int       `json:"native_pid"`
		NativePGID      *int       `json:"native_pgid"`
		NativeStartTime *time.Time `json:"native_start_time"`
	}
	decoder := json.NewDecoder(bytes.NewReader(event.Payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(event.Payload, &fields); err != nil {
		return err
	}
	required := []string{"worker_pid", "worker_start_time", "native_pid", "native_pgid"}
	if goos == "windows" {
		required = append(required, "native_start_time")
	}
	if len(fields) != len(required) {
		return fmt.Errorf("launched payload has %d fields, want %d", len(fields), len(required))
	}
	for _, key := range required {
		if _, ok := fields[key]; !ok {
			return fmt.Errorf("launched payload lacks %s", key)
		}
	}
	if payload.WorkerPID != id.PID || !payload.WorkerStartTime.Equal(id.StartTime) ||
		payload.NativePID == nil || id.NativePID == nil || *payload.NativePID != *id.NativePID || !reflect.DeepEqual(payload.NativePGID, id.NativePGID) {
		return errors.New("launched process identity differs from worker.json")
	}
	if goos == "windows" {
		if payload.NativeStartTime == nil || payload.NativeStartTime.IsZero() || id.NativeStartTime == nil || id.NativeStartTime.IsZero() || !payload.NativeStartTime.Equal(*id.NativeStartTime) {
			return errors.New("launched native creation time differs from worker.json or is unknown")
		}
	} else if id.NativeStartTime != nil {
		return errors.New("Unix worker.json invented native creation time")
	}
	return nil
}

func TestLaunchEnvelopeKeepsCreationTimeEvidence(t *testing.T) {
	t.Parallel()
	stamp := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	nativeStamp := stamp.Add(time.Second)
	pid := 101
	id := workers.Identity{RunID: "run_synthetic", AttemptID: "att_synthetic", PID: 100, StartTime: stamp, NativePID: &pid, NativeStartTime: &nativeStamp}
	valid := journal.Event{Type: "attempt.launched", RunID: id.RunID, AttemptID: id.AttemptID,
		Payload: json.RawMessage(`{"worker_pid":100,"worker_start_time":"2026-01-02T03:04:05Z","native_pid":101,"native_pgid":null,"native_start_time":"2026-01-02T03:04:06Z"}`)}
	if err := validateLaunchedIdentity(valid, id, "windows"); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	if err := compareDurableEnvelope(encoded, encoded); err != nil {
		t.Fatal(err)
	}
	for name, payload := range map[string][]byte{
		"changed creation time": bytes.Replace(valid.Payload, []byte("03:04:06Z"), []byte("03:04:07Z"), 1),
		"missing creation time": bytes.Replace(valid.Payload, []byte(`,"native_start_time":"2026-01-02T03:04:06Z"`), nil, 1),
		"null creation time":    bytes.Replace(valid.Payload, []byte(`"native_start_time":"2026-01-02T03:04:06Z"`), []byte(`"native_start_time":null`), 1),
		"extra evidence":        bytes.Replace(valid.Payload, []byte(`"native_pgid":null`), []byte(`"native_pgid":null,"unexpected":true`), 1),
		"changed PID":           bytes.Replace(valid.Payload, []byte(`"native_pid":101`), []byte(`"native_pid":102`), 1),
	} {
		t.Run(name, func(t *testing.T) {
			altered := valid
			altered.Payload = payload
			raw, err := json.Marshal(altered)
			if err != nil {
				t.Fatal(err)
			}
			if err := compareDurableEnvelope(raw, encoded); err == nil {
				t.Fatal("printed evidence mismatch accepted")
			}
			// Even if stream and journal share the defect, worker.json must reject it.
			if err := validateLaunchedIdentity(altered, id, "windows"); err == nil {
				t.Fatal("evidence inconsistent with worker.json accepted")
			}
		})
	}
	unix := valid
	unix.Payload = bytes.Replace(valid.Payload, []byte(`,"native_start_time":"2026-01-02T03:04:06Z"`), nil, 1)
	unixID := id
	unixID.NativeStartTime = nil
	if err := validateLaunchedIdentity(unix, unixID, "linux"); err != nil {
		t.Fatal(err)
	}
	if err := validateLaunchedIdentity(valid, unixID, "linux"); err == nil {
		t.Fatal("Unix launched event invented creation time")
	}
}

func TestNormalizeLaunchCreationTimeOnly(t *testing.T) {
	t.Parallel()
	e := env{state: "/state/synthetic", home: "/synthetic/home"}
	first := `{"type":"attempt.launched","payload":{"native_start_time":"2026-01-02T03:04:05Z"}}`
	second := strings.Replace(first, "03:04:05Z", "03:04:06Z", 1)
	if normalizeJSONLLine(t, first, e, "/repo/synthetic") != normalizeJSONLLine(t, second, e, "/repo/synthetic") {
		t.Fatal("independent launch creation times compare differently")
	}
	missing := `{"type":"attempt.launched","payload":{}}`
	if normalizeJSONLLine(t, first, e, "/repo/synthetic") == normalizeJSONLLine(t, missing, e, "/repo/synthetic") {
		t.Fatal("missing launch timestamp normalised away")
	}
	for _, typ := range []string{"attempt.stopped", "attempt.progress"} {
		left := strings.Replace(first, "attempt.launched", typ, 1)
		right := strings.Replace(second, "attempt.launched", typ, 1)
		if normalizeJSONLLine(t, left, e, "/repo/synthetic") == normalizeJSONLLine(t, right, e, "/repo/synthetic") {
			t.Fatalf("%s creation time normalised", typ)
		}
	}
}

// ptyReply is one answer from tui_pty.py. Post is the visible output after
// the view last left the alternate screen: what the CLI printed itself.
type ptyReply struct {
	OK     bool   `json:"ok"`
	Error  string `json:"error"`
	Tail   string `json:"tail"`
	PID    int    `json:"pid"`
	At     int    `json:"at"`
	Alive  bool   `json:"alive"`
	Code   int    `json:"code"`
	Output string `json:"output"`
	Post   string `json:"post"`
}

// ptyClient is one packaged invocation running as the session leader of a
// pseudo-terminal that tui_pty.py owns. The driver is the child the test
// starts and joins; it kills and reaps the packaged process on end of input.
type ptyClient struct {
	t      *testing.T
	cmd    *exec.Cmd
	in     *os.File // driver stdin; writes carry a deadline
	outF   *os.File // driver stdout; reads carry a deadline
	out    *bufio.Reader
	stderr *lockedBuffer
	done   chan struct{} // closed by the one goroutine that calls cmd.Wait
	pid    int
	start  time.Time // the packaged process's start time, its identity with pid
}

// ptyReplyLimit bounds one driver reply line; the exit reply carries the
// whole visible output.
const ptyReplyLimit = 8 << 20

// ptyReplySlack is how long a reply may lag the driver-side timeout it was
// asked to honour; a request without one must answer within it.
const ptyReplySlack = 30 * time.Second

// ownedJoinGrace bounds each step of an owned child's shutdown: the polite
// request, then the kill.
const ownedJoinGrace = 30 * time.Second

// lockedBuffer is the driver's stderr: exec copies into it while a failing
// read reports it.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// joinOwned stops a child this test started and whose single waiter closes
// done: ask politely, wait grace, then kill and wait grace again. It returns
// whether the kill was needed, and an error only when the child outlived
// both steps — the caller reports it rather than hanging.
func joinOwned(done <-chan struct{}, ask, kill func(), grace time.Duration) (killed bool, err error) {
	ask()
	select {
	case <-done:
		return false, nil
	case <-time.After(grace):
	}
	kill()
	select {
	case <-done:
		return true, nil
	case <-time.After(grace):
		return true, errors.New("owned child survived its kill")
	}
}

// startPTY runs the packaged binary with args on a 160x40 terminal in dir.
// python3 is a hard requirement wherever this runs: a missing interpreter
// fails rather than skipping the lifecycle evidence.
func startPTY(t *testing.T, environ []string, dir string, args ...string) *ptyClient {
	t.Helper()
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatalf("python3 is required for the packaged TUI lifecycle tests: %v", err)
	}
	driver, err := filepath.Abs("tui_pty.py")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(python, append([]string{"-I", "-B", driver, "160", "40", dir, mythhelmBin}, args...)...)
	cmd.Env = environ
	c := &ptyClient{t: t, cmd: cmd, stderr: &lockedBuffer{}, done: make(chan struct{})}
	cmd.Stderr = c.stderr
	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stdin, cmd.Stdout = inR, outW
	c.in, c.outF, c.out = inW, outR, bufio.NewReaderSize(outR, ptyReplyLimit)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	_, _ = inR.Close(), outW.Close()
	go func() {
		_ = cmd.Wait()
		close(c.done)
	}()
	t.Cleanup(c.close)
	r := c.readWithin(ptyReplySlack)
	c.pid = r.PID
	if c.start, err = workers.ProcessStartTime(c.pid); err != nil {
		t.Fatalf("packaged process %d has no start time to bind its identity: %v", c.pid, err)
	}
	return c
}

// close ends the driver: end of input asks it to kill and reap the packaged
// process; a driver that does not exit is killed, and so is the packaged
// process it leaves, but only while that is still the same process.
func (c *ptyClient) close() {
	_ = c.in.Close()
	killed, err := joinOwned(c.done, func() {}, func() { _ = c.cmd.Process.Kill() }, ownedJoinGrace)
	if killed && !c.start.IsZero() {
		killIdentified(c.pid, c.start)
	}
	_ = c.outF.Close()
	if err != nil {
		c.t.Errorf("PTY driver %d: %v; stderr %q", c.cmd.Process.Pid, err, c.stderr.String())
	} else if killed {
		c.t.Errorf("PTY driver %d ignored end of input and was killed; stderr %q", c.cmd.Process.Pid, c.stderr.String())
	}
}

func (c *ptyClient) readWithin(limit time.Duration) ptyReply {
	c.t.Helper()
	if err := c.outF.SetReadDeadline(time.Now().Add(limit)); err != nil {
		c.t.Fatal(err)
	}
	line, err := c.out.ReadSlice('\n')
	if err != nil {
		c.t.Fatalf("PTY driver reply within %v: %v; stderr %q", limit, err, c.stderr.String())
	}
	var r ptyReply
	if err := json.Unmarshal(line, &r); err != nil {
		c.t.Fatalf("PTY driver reply %q: %v", line, err)
	}
	if !r.OK {
		c.t.Fatalf("PTY driver: %s\nvisible tail:\n%s", r.Error, r.Tail)
	}
	return r
}

func (c *ptyClient) call(req map[string]any) ptyReply {
	c.t.Helper()
	line, err := json.Marshal(req)
	if err != nil {
		c.t.Fatal(err)
	}
	if err := c.in.SetWriteDeadline(time.Now().Add(ptyReplySlack)); err != nil {
		c.t.Fatal(err)
	}
	if _, err := c.in.Write(append(line, '\n')); err != nil {
		c.t.Fatalf("PTY driver input: %v; stderr %q", err, c.stderr.String())
	}
	limit := ptyReplySlack
	if timeout, ok := req["timeout"].(float64); ok {
		limit += time.Duration(timeout * float64(time.Second))
	} else if timeout, ok := req["timeout"].(int); ok {
		limit += time.Duration(timeout) * time.Second
	}
	return c.readWithin(limit)
}

// send writes keys to the terminal as typed input.
func (c *ptyClient) send(keys string) {
	c.t.Helper()
	c.call(map[string]any{"op": "send", "data": keys})
}

// mark is the current offset of the visible output, for waits that must
// observe only what is drawn afterwards.
func (c *ptyClient) mark() int {
	c.t.Helper()
	return c.call(map[string]any{"op": "mark"}).At
}

// waitText waits for text drawn after offset from. A non-empty resend is
// typed at once and again every half second until text appears: the view
// ignores a key whose gate its latest snapshot has not opened yet, and the
// text is the view's own acknowledgement that a press was accepted.
func (c *ptyClient) waitText(text string, from int, resend string) int {
	c.t.Helper()
	req := map[string]any{"op": "wait", "text": text, "from": from, "timeout": 60}
	if resend != "" {
		req["resend"], req["every"] = resend, 0.5
	}
	return c.call(req).At
}

// waitViewClosed waits until the view has left the alternate screen.
func (c *ptyClient) waitViewClosed() {
	c.t.Helper()
	c.call(map[string]any{"op": "wait_view_closed", "timeout": 60})
}

// alive reports whether the packaged process is still running.
func (c *ptyClient) alive() bool {
	c.t.Helper()
	return c.call(map[string]any{"op": "alive"}).Alive
}

// signal delivers sig to the packaged process exactly once.
func (c *ptyClient) signal(sig os.Signal) {
	c.t.Helper()
	if !sameProcess(c.pid, c.start) {
		c.t.Fatalf("packaged process %d is no longer the one started; no signal sent", c.pid)
	}
	p, err := os.FindProcess(c.pid)
	if err != nil {
		c.t.Fatal(err)
	}
	if err := p.Signal(sig); err != nil {
		c.t.Fatalf("signal %v to pid %d: %v", sig, c.pid, err)
	}
}

// exit waits for the packaged process and the end of its terminal output.
func (c *ptyClient) exit(timeout time.Duration) ptyReply {
	c.t.Helper()
	return c.call(map[string]any{"op": "exit", "timeout": timeout.Seconds()})
}

// confirmKeys moves a confirmation's focus from Cancel to its confirm
// button and activates it: two deliberate keys, never one (I03 (v2 §2)).
const confirmKeys = "\t\r"

// openDetach presses d until the view draws its detach confirmation: the
// view's own proof that its snapshot holds a running worker.
func openDetach(c *ptyClient) {
	c.t.Helper()
	c.waitText("Detach (leave running)", c.mark(), "d")
}

// openStop presses s until the view draws its stop confirmation.
func openStop(c *ptyClient) {
	c.t.Helper()
	c.waitText("Stop run", c.mark(), "s")
}

// posixQuote is the independent oracle for POSIX single quoting.
func posixQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'" }

// wantRecover is the exact labelled command bound to the run's state root.
func wantRecover(state, runID string) string {
	return "POSIX sh/bash/zsh: env " + posixQuote("MYTHHELM_HOME="+state) + " mythhelm recover " + posixQuote(runID)
}

// printedCommand returns the one labelled recovery command in out.
func printedCommand(t *testing.T, out string) string {
	t.Helper()
	const label = "POSIX sh/bash/zsh: "
	if n := strings.Count(out, "mythhelm recover"); n != 1 {
		t.Fatalf("output names mythhelm recover %d times, want once:\n%s", n, out)
	}
	_, rest, ok := strings.Cut(out, label)
	if !ok {
		t.Fatalf("output has no %q command:\n%s", label, out)
	}
	command, _, _ := strings.Cut(rest, "\n")
	return strings.TrimRight(command, " ")
}

// hasExactLine reports whether out has a line equal to want once trailing
// cell padding is removed.
func hasExactLine(out, want string) bool {
	for line := range strings.Lines(out) {
		if strings.TrimRight(line, " \n") == want {
			return true
		}
	}
	return false
}

// unrelatedHome is a caller MYTHHELM_HOME that a state-bound command must
// not touch; its sentinel is the only entry it may ever hold.
func unrelatedHome(t *testing.T) string {
	t.Helper()
	dir := canonicalDir(t, t.TempDir())
	if err := os.WriteFile(filepath.Join(dir, "sentinel"), []byte("caller state\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func assertUntouched(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "sentinel"))
	if err != nil || string(body) != "caller state\n" || len(entries) != 1 {
		t.Fatalf("caller state %s changed: %d entries, sentinel %q (%v)", dir, len(entries), body, err)
	}
}

// runPrinted executes exactly the printed command in its named POSIX shell
// from a caller whose own MYTHHELM_HOME is home; `mythhelm` resolves to the
// packaged binary through PATH.
func runPrinted(t *testing.T, environ []string, home, command string) (int, string) {
	t.Helper()
	bin := t.TempDir()
	if err := os.Symlink(mythhelmBin, filepath.Join(bin, "mythhelm")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", command)
	cmd.Dir = t.TempDir()
	cmd.Env = append(slices.Clone(environ), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "MYTHHELM_HOME="+home)
	out, err := cmd.CombinedOutput()
	if cmd.ProcessState == nil {
		t.Fatalf("printed command did not run: %v", err)
	}
	return cmd.ProcessState.ExitCode(), string(out)
}

// waitSpool waits until the attempt's spool satisfies cond.
func waitSpool(t *testing.T, dir, what string, cond func([]journal.Event) bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	for {
		if _, err := os.Stat(filepath.Join(dir, "spool.jsonl")); err == nil && cond(spoolEvents(t, dir)) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s in the spool", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func spoolHasStop(events []journal.Event) bool {
	return slices.ContainsFunc(events, func(ev journal.Event) bool { return ev.Type == "attempt.stop_requested" })
}

// liveAttempt is the attempt a detached client left: its directory and the
// recorded worker, which the test reaps if it is still running at the end.
func liveAttempt(t *testing.T, state, runID string) (string, attemptProcs) {
	t.Helper()
	_, a, _ := runSnapshot(t, state, runID)
	id := workerOf(t, state, runID, a.AttemptID)
	reapDetached(t, id)
	return workers.AttemptDir(state, runID, a.AttemptID), id
}

// canonicalEnv is newEnv with its roots resolved: the binary reports
// resolved paths (macOS /var is /private/var, Windows short names expand),
// so an exact path comparison needs the same spelling.
func canonicalEnv(t *testing.T) env {
	t.Helper()
	e := newEnv(t)
	home, state := canonicalDir(t, e.home), canonicalDir(t, e.state)
	for i, kv := range e.vars {
		name, value, _ := strings.Cut(kv, "=")
		switch value {
		case e.home:
			e.vars[i] = name + "=" + home
		case e.state:
			e.vars[i] = name + "=" + state
		}
	}
	e.home, e.state = home, state
	return e
}

func canonicalDir(t *testing.T, dir string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

// pollUntil re-evaluates a named condition against fresh read-only journal
// handles until it holds, failing after two minutes. A journal or row that
// does not exist yet reads as not yet.
func pollUntil(t *testing.T, state, what string, cond func(context.Context, *journal.Journal) (bool, error)) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	var last error
	for time.Now().Before(deadline) {
		ok, err := func() (bool, error) {
			j, err := journal.OpenReadOnly(t.Context(), state)
			if err != nil {
				return false, err
			}
			defer func() { _ = j.Close() }()
			return cond(t.Context(), j)
		}()
		if ok {
			return
		}
		last = err
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s (last error %v)", what, last)
}

// onlyRun waits for the state's single run and returns its ID.
func onlyRun(t *testing.T, state string) string {
	t.Helper()
	var runID string
	pollUntil(t, state, "the run row", func(ctx context.Context, j *journal.Journal) (bool, error) {
		runs, err := j.ListRuns(ctx, 2)
		if err != nil || len(runs) == 0 {
			return false, err
		}
		if len(runs) > 1 {
			return false, fmt.Errorf("state holds %d runs, want one", len(runs))
		}
		runID = runs[0].RunID
		return true, nil
	})
	return runID
}

// waitRunState waits until the run's journaled state is want.
func waitRunState(t *testing.T, state, runID, want string) {
	t.Helper()
	pollUntil(t, state, "run state "+want, func(ctx context.Context, j *journal.Journal) (bool, error) {
		run, err := j.Run(ctx, runID)
		return err == nil && run.State == want, err
	})
}

// waitAttemptState waits until the latest attempt's journaled state is want.
func waitAttemptState(t *testing.T, state, runID, want string) {
	t.Helper()
	pollUntil(t, state, "attempt state "+want, func(ctx context.Context, j *journal.Journal) (bool, error) {
		a, err := j.LatestAttempt(ctx, runID)
		return err == nil && a.State == want, err
	})
}

// runSnapshot reads the run row, its latest attempt and per-type event counts.
func runSnapshot(t *testing.T, state, runID string) (journal.RunRow, journal.AttemptRow, map[string]int) {
	t.Helper()
	j, err := journal.OpenReadOnly(t.Context(), state)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = j.Close() }()
	run, err := j.Run(t.Context(), runID)
	if err != nil {
		t.Fatal(err)
	}
	a, err := j.LatestAttempt(t.Context(), runID)
	if err != nil {
		t.Fatal(err)
	}
	return run, a, ledgerEventCounts(t, j, runID)
}

// spoolEvents reads the worker's own spool for the attempt: the durable
// record a worker keeps whether or not any supervisor ingests it.
func spoolEvents(t *testing.T, dir string) []journal.Event {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "spool.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var events []journal.Event
	for line := range strings.Lines(string(raw)) {
		var ev journal.Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("spool line %q: %v", line, err)
		}
		events = append(events, ev)
	}
	return events
}

// stopRequesters lists requested_by of every spooled attempt.stop_requested.
func stopRequesters(t *testing.T, events []journal.Event) []string {
	t.Helper()
	var out []string
	for _, ev := range events {
		if ev.Type != "attempt.stop_requested" {
			continue
		}
		var p struct {
			RequestedBy string `json:"requested_by"`
		}
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			t.Fatal(err)
		}
		out = append(out, p.RequestedBy)
	}
	return out
}

// attemptProcs is the attempt's recorded worker identity plus the native's
// start time, observed while worker.json links that native PID to a live
// worker. worker.json carries no native start time on Unix, so this
// observation is the native's only identity beyond its reusable PID.
type attemptProcs struct {
	workers.Identity
	NativeStart time.Time
}

// workerOf reads the attempt's recorded worker identity and binds the native
// PID to the process that holds it now. A native whose start time cannot be
// read is unknown, never assumed: the test fails rather than guess.
func workerOf(t *testing.T, state, runID, attemptID string) attemptProcs {
	t.Helper()
	id, err := workers.ReadIdentity(workers.AttemptDir(state, runID, attemptID))
	if err != nil {
		t.Fatal(err)
	}
	if id.NativePID == nil {
		t.Fatal("worker.json records no native PID")
	}
	if !sameProcess(id.PID, id.StartTime) {
		t.Fatalf("recorded worker %d is not running; its native %d cannot be bound", id.PID, *id.NativePID)
	}
	start, err := workers.ProcessStartTime(*id.NativePID)
	if err != nil {
		t.Fatalf("native %d start time unknown: %v", *id.NativePID, err)
	}
	return attemptProcs{Identity: id, NativeStart: start}
}

// sameProcess reports whether pid is still the process that started at
// start: a reused PID never counts, and an unreadable start time is not
// evidence of this process.
func sameProcess(pid int, start time.Time) bool {
	got, err := workers.ProcessStartTime(pid)
	return err == nil && got.Equal(start)
}

func workerAlive(p attemptProcs) bool { return sameProcess(p.PID, p.StartTime) }

func nativeAlive(p attemptProcs) bool { return sameProcess(*p.NativePID, p.NativeStart) }

// killIdentified kills pid only while it is still the process that started
// at start; any other process holding a reused PID is left alone.
func killIdentified(pid int, start time.Time) {
	if !sameProcess(pid, start) {
		return
	}
	if p, err := os.FindProcess(pid); err == nil {
		_ = p.Kill()
	}
}

// waitProcessesGone waits until neither the identified worker nor its
// identified native runs.
func waitProcessesGone(t *testing.T, p attemptProcs) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	for workerAlive(p) || nativeAlive(p) {
		if time.Now().After(deadline) {
			t.Fatalf("worker %d / native %d still running", p.PID, *p.NativePID)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// reapDetached registers the finite cleanup for a worker the test may leave
// running: kill the identified worker and native, then wait until both are
// gone.
func reapDetached(t *testing.T, p attemptProcs) {
	t.Helper()
	t.Cleanup(func() {
		killIdentified(p.PID, p.StartTime)
		killIdentified(*p.NativePID, p.NativeStart)
		waitProcessesGone(t, p)
	})
}
