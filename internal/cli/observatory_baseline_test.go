package cli

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/workspace"
)

type observatoryCapture struct {
	Invocation string `json:"invocation"`
	BaseCommit string `json:"base_commit"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	Exit       int    `json:"exit"`
}
type observatoryBundle struct {
	Schema        int                           `json:"schema_version"`
	Provenance    string                        `json:"provenance"`
	BaseCommit    string                        `json:"base_commit"`
	Normalisation []string                      `json:"normalisation"`
	Events        []journal.Event               `json:"events"`
	Receipt       supervisor.Receipt            `json:"receipt"`
	Usage         []journal.UsageRow            `json:"usage"`
	Captures      map[string]observatoryCapture `json:"captures"`
}

func readObservatoryBundle(t *testing.T) observatoryBundle {
	t.Helper()
	data, err := os.ReadFile("testdata/observatory-linear.json")
	if err != nil {
		t.Fatal(err)
	}
	var b observatoryBundle
	if err := json.Unmarshal(data, &b); err != nil {
		t.Fatal(err)
	}
	if b.Schema != 1 || b.BaseCommit != "e4ab1fc46fb08886af5e8195c4a2812b1b0b6e6f" || !strings.HasPrefix(b.Provenance, "Synthetic ") {
		t.Fatal("baseline provenance is missing or changed")
	}
	return b
}

// TestObservatoryLinearBaseline pins I09 (v2 §2) and I13 (v2 §15.3).
// Not parallel: CLI calls use isolated process environment and working directory.
// There is deliberately no update-golden switch: later tasks must preserve this
// pre-change capture, not regenerate an expected answer from changed renderers.
func TestObservatoryLinearBaseline(t *testing.T) {
	b := readObservatoryBundle(t)
	got := captureObservatory(t, b)
	if len(got) != 7 {
		t.Fatalf("produced %d captures, want all seven", len(got))
	}
	if len(b.Captures) != 7 {
		t.Fatalf("baseline has %d captures, want seven captures from unmodified base", len(b.Captures))
	}
	for name, actual := range got {
		t.Run(name, func(t *testing.T) {
			expected, ok := b.Captures[name]
			if !ok || expected.Stdout == "" {
				t.Fatal("missing nonempty baseline")
			}
			expected = expectedObservatoryCapture(t, name, runtime.GOOS, expected)
			if actual != expected {
				t.Errorf("capture changed\ngot: %+v\nwant: %+v", actual, expected)
			}
		})
	}
}

// The captured bundle remains immutable. Windows ownership Task 4 evolves
// only the new demo admission's pinned ladder version (ADR 0017 D3).
func expectedObservatoryCapture(t *testing.T, name, goos string, expected observatoryCapture) observatoryCapture {
	t.Helper()
	if name != "demo-accessible" || goos != "windows" {
		return expected
	}
	const oldLine = "event 7: attempt <att-1>: admission pinned (stop ladder stop-ladder/v1)\n"
	const newLine = "event 7: attempt <att-1>: admission pinned (stop ladder stop-ladder/v2-windows)\n"
	if strings.Count(expected.Stdout, oldLine) != 1 {
		t.Fatal("captured demo must contain exactly one original admission ladder line")
	}
	expected.Stdout = strings.Replace(expected.Stdout, oldLine, newLine, 1)
	return expected
}

func TestObservatoryWindowsLadderExpectation(t *testing.T) {
	t.Parallel()
	b := readObservatoryBundle(t)
	original := b.Captures["demo-accessible"]
	expected := expectedObservatoryCapture(t, "demo-accessible", "windows", original)
	const windowsLine = "event 7: attempt <att-1>: admission pinned (stop ladder stop-ladder/v2-windows)"
	if !strings.Contains(expected.Stdout, windowsLine+"\n") {
		t.Fatal("Windows baseline does not require the job-wide v2 ladder")
	}
	if expected == original {
		t.Fatal("incorrect Windows v1 capture would be accepted")
	}
	originalLines, expectedLines := strings.Split(original.Stdout, "\n"), strings.Split(expected.Stdout, "\n")
	if len(originalLines) != len(expectedLines) {
		t.Fatal("overlay changed capture line count")
	}
	changed := 0
	for i, line := range originalLines {
		if line == expectedLines[i] {
			continue
		}
		changed++
		if line != "event 7: attempt <att-1>: admission pinned (stop ladder stop-ladder/v1)" || expectedLines[i] != windowsLine {
			t.Fatalf("unexpected changed line %d: %q -> %q", i, line, expectedLines[i])
		}
	}
	if changed != 1 {
		t.Fatalf("changed %d lines, want one exact admission field", changed)
	}
	expected.Stdout = original.Stdout
	if expected != original {
		t.Fatal("overlay changed another capture field")
	}
	for name, capture := range b.Captures {
		for _, goos := range []string{"linux", "darwin", "windows"} {
			if name == "demo-accessible" && goos == "windows" {
				continue
			}
			if got := expectedObservatoryCapture(t, name, goos, capture); got != capture {
				t.Fatalf("overlay changed %s on %s", name, goos)
			}
		}
	}
}

func captureObservatory(t *testing.T, b observatoryBundle) map[string]observatoryCapture {
	t.Helper()
	// No native account, configuration or live supervisor participates.
	home := t.TempDir()
	for _, key := range []string{"HOME", "USERPROFILE", "XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_RUNTIME_DIR", "MYTHHELM_HOME"} {
		t.Setenv(key, home)
	}
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1") //nolint:misspell // Standard environment variable spelling.
	t.Chdir(home)
	got := make(map[string]observatoryCapture)
	record := func(name, invocation string, code int, out, diag string) {
		got[name] = observatoryCapture{invocation, b.BaseCommit, out, diag, code}
	}
	j := openJournal(t, home)
	defer func() { _ = j.Close() }()
	at := b.Events[0].ObservedAt
	for i, ev := range b.Events {
		var project func(*sql.Tx) error
		if i == 0 {
			project = func(tx *sql.Tx) error {
				return journal.InsertRun(t.Context(), tx, journal.RunRow{RunID: "run_baseline", State: "failed", Reason: "synthetic_failure", AdapterID: "builtin/fake", SourceRepo: "synthetic-repository", TaskSHA256: strings.Repeat("0", 64), BillingPosture: "local-scripted", ExecutionProfile: "trusted-host", CreatedAt: at, UpdatedAt: at})
			}
		}
		if err := j.Append(t.Context(), ev, project); err != nil {
			t.Fatal(err)
		}
	}
	runDir := filepath.Join(home, "runs", "run_baseline")
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	sha, err := supervisor.WriteReceipt(runDir, b.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	p, err := json.Marshal(map[string]any{"schema_version": 1, "sha256": sha, "path": "receipt.json"})
	if err != nil {
		t.Fatal(err)
	}
	if err := j.Append(t.Context(), journal.Event{SchemaVersion: 1, EventID: "evt_receipt", RunID: "run_baseline", ProducerID: "sup_receipt", ProducerSequence: 1, Generation: 1, ObservedAt: at, Type: "receipt.written", Payload: p}, nil); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"accessible", "jsonl"} {
		args := []string{"review", "run_baseline", "--accessible"}
		if mode == "jsonl" {
			args = []string{"review", "run_baseline", "--format", "jsonl"}
		}
		code, out, diag := runMain(args...)
		record("review-"+mode, "cli.Main("+strings.Join(args, " ")+") over bundle journal and receipt", code, out, diag)
	}
	for _, mode := range []string{"plain", "accessible"} {
		var out bytes.Buffer
		if mode == "plain" {
			r := newRenderer("plain", Stdio{Out: &out})
			for _, ev := range b.Events {
				r.event(ev)
			}
			for _, line := range (ledgerView{usage: b.Usage, checks: 1, verifySum: time.Second, verifySumKnown: true, repairs: 3}).lines() {
				r.notice(line)
			}
			r.result(runResult{RunID: "run_baseline", State: "failed", Reason: "synthetic_failure", Code: ExitNative, Category: "native_failure"})
			if err := r.err(); err != nil {
				t.Fatal(err)
			}
		} else {
			w := accessibleWriter{out: &out}
			for _, ev := range b.Events {
				w.event(ev)
			}
			for _, line := range (ledgerView{usage: b.Usage, checks: 1, verifySum: time.Second, verifySumKnown: true, repairs: 3}).lines() {
				w.line(line)
			}
			if w.err != nil {
				t.Fatal(w.err)
			}
		}
		record("renderer-"+mode, mode+" renderer over bundle events and typed usage rows", 0, out.String(), "")
	}
	for _, mode := range []string{"plain", "jsonl"} {
		args := []string{"run", "--non-interactive", "--plain"}
		if mode == "jsonl" {
			args = []string{"run", "--non-interactive", "--format", "jsonl"}
		}
		code, out, diag := runMain(args...)
		if mode == "jsonl" {
			out, err = normalizeObservatoryRefusal(out)
			if err != nil {
				t.Fatal(err)
			}
		}
		record("run-"+mode, "cli.Main("+strings.Join(args, " ")+"); intentional admission refusal", code, out, diag)
	}
	// Observe the candidate while demo still owns its temporary directories.
	// This writer validates real Git objects before deferred cleanup executes.
	demoTemp := t.TempDir()
	for _, key := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(key, demoTemp)
	}
	probe := &observatoryDemoWriter{t: t, tempRoot: demoTemp}
	var diag bytes.Buffer
	code := Main([]string{"demo", "--accessible", "--after", "1"}, Stdio{In: strings.NewReader(""), Out: probe, Err: &diag})
	if !probe.checked {
		t.Fatalf("demo never exposed a completed synthetic candidate (exit %d)\nstderr: %s\nstdout: %s", code, diag.String(), probe.String())
	}
	out, err := normalizeObservatoryDemo(probe.String())
	if err != nil {
		t.Fatalf("%v\n%s", err, probe.String())
	}
	checkObservatoryDemoCounterfactuals(t, probe.String(), out)
	record("demo-accessible", "cli.Main(demo --accessible --after 1); fake scripted child, candidate Git object inspected before cleanup", code, out, diag.String())
	return got
}

func normalizeObservatoryRefusal(s string) (string, error) {
	var ev journal.Event
	if err := json.Unmarshal([]byte(s), &ev); err != nil {
		return "", err
	}
	if ev.Type != "run.result" {
		return "", fmt.Errorf("refusal record type %q", ev.Type)
	}
	for _, v := range []struct{ key, value, prefix string }{{"event_id", ev.EventID, "evt"}, {"producer_id", ev.ProducerID, "cli"}} {
		if !regexp.MustCompile(`^` + v.prefix + `_[0-9A-HJKMNP-TV-Z]{26}$`).MatchString(v.value) {
			return "", fmt.Errorf("invalid generated %s", v.key)
		}
		s = strings.Replace(s, `"`+v.key+`":"`+v.value+`"`, `"`+v.key+`":"<`+v.prefix+`-1>"`, 1)
	}
	rawAt := regexp.MustCompile(`"observed_at":"([^"]+)"`).FindStringSubmatch(s)
	if len(rawAt) != 2 {
		return "", errors.New("missing timestamp")
	}
	if _, err := time.Parse(time.RFC3339Nano, rawAt[1]); err != nil {
		return "", err
	}
	return strings.Replace(s, rawAt[0], `"observed_at":"<timestamp>"`, 1), nil
}

type observatoryDemoWriter struct {
	bytes.Buffer
	t        *testing.T
	checked  bool
	tempRoot string
}

func (w *observatoryDemoWriter) WriteString(s string) (int, error) { return w.Write([]byte(s)) }

func (w *observatoryDemoWriter) Write(p []byte) (int, error) {
	n, err := w.Buffer.Write(p)
	if strings.HasPrefix(string(p), "event 4: snapshot:") {
		// A deterministic slow consumer: let the independent supervisor finish
		// journaling before this stream can drain. Without synchronisation the base
		// terminal watch can stop at ready_for_review before receipt.written exists.
		// The complete state path may be truncated out of event 4. Look only
		// inside this capture's private temporary root; output normalisation
		// still verifies the displayed prefix against the actual workspace.
		entries, readErr := os.ReadDir(w.tempRoot)
		if readErr != nil {
			return n, readErr
		}
		var state string
		for _, entry := range entries {
			if entry.IsDir() && strings.HasPrefix(entry.Name(), "mythhelm-demo-state-") {
				if state != "" {
					return n, errors.New("multiple demo state directories in capture root")
				}
				state = filepath.Join(w.tempRoot, entry.Name())
			}
		}
		if state == "" {
			return n, errors.New("demo state directory missing from capture root")
		}
		if err := waitObservatoryReceipt(w.t.Context(), state); err != nil {
			return n, err
		}
	}
	if strings.Contains(string(p), "was removed afterwards") {
		s := w.String()
		state := regexp.MustCompile(`(?m)^state in (.+) was removed afterwards$`).FindStringSubmatch(s)
		run := regexp.MustCompile(`(?m)^Run: (run_[0-9A-Z]+)$`).FindStringSubmatch(s)
		candidate := regexp.MustCompile(`(?m)^Candidate: ([0-9a-f]{40})$`).FindStringSubmatch(s)
		base := regexp.MustCompile(`event 4: snapshot: ([0-9a-f]{40}) cloned to `).FindStringSubmatch(s)
		if len(state) != 2 || len(run) != 2 || len(candidate) != 2 || len(base) != 2 {
			w.t.Error("missing demo candidate identity")
			return n, err
		}
		dir := filepath.Join(state[1], "runs", run[1], "workspace")
		git := func(args ...string) string {
			out, e := workspace.Git(w.t.Context(), dir, false, args...)
			if e != nil {
				w.t.Errorf("synthetic candidate inspection: %v: %s", e, out)
			}
			return string(out)
		}
		if got := git("diff", "--name-only", base[1], candidate[1]); got != "demo.txt\n" {
			w.t.Errorf("candidate changed paths %q", got)
		}
		if got := git("show", candidate[1]+":demo.txt"); got != "ok\n" {
			w.t.Errorf("candidate content %q", got)
		}
		w.checked = true
	}
	return n, err
}

func normalizeObservatoryDemo(s string) (string, error) {
	sequence := regexp.MustCompile(`(?m)^event ([0-9]+):`).FindAllStringSubmatch(s, -1)
	if len(sequence) != 22 {
		return "", fmt.Errorf("demo has %d events, want 2 through 23", len(sequence))
	}
	for i, m := range sequence {
		if m[1] != strconv.Itoa(i+2) {
			return "", fmt.Errorf("demo sequence %s, want %d", m[1], i+2)
		}
	}
	if strings.Count(s, "next-after: 23\n") != 1 {
		return "", errors.New("demo final cursor changed")
	}
	find := func(pattern string) (string, error) {
		m := regexp.MustCompile(pattern).FindStringSubmatch(s)
		if len(m) != 2 {
			return "", fmt.Errorf("missing demo field %s", pattern)
		}
		return m[1], nil
	}
	repo, err := find(`(?m)^working in (.+) \(removed afterwards\)$`)
	if err != nil {
		return "", err
	}
	state, err := find(`(?m)^state in (.+) was removed afterwards$`)
	if err != nil {
		return "", err
	}
	run, err := find(`(?m)^Run: (run_[0-9A-HJKMNP-TV-Z]{26})$`)
	if err != nil {
		return "", err
	}
	base, err := find(`event 4: snapshot: ([0-9a-f]{40}) cloned to `)
	if err != nil {
		return "", err
	}
	candidate, err := find(`(?m)^Candidate: ([0-9a-f]{40})$`)
	if err != nil {
		return "", err
	}
	if base == candidate {
		return "", errors.New("demo candidate did not change")
	}
	workspace, err := find(`(?m)^event 4: snapshot: [0-9a-f]{40} cloned to (.+)$`)
	if err != nil {
		return "", err
	}
	wantPath := filepath.Join(state, "runs", run, "workspace")
	validWorkspace := workspace == wantPath || (strings.HasSuffix(workspace, "... (truncated)") && strings.HasPrefix(wantPath, strings.TrimSuffix(workspace, "... (truncated)")))
	if !validWorkspace {
		return "", errors.New("snapshot path does not name this demo workspace")
	}
	s = strings.Replace(s, "cloned to "+workspace, "cloned to <state>/runs/<run-1>/workspace", 1)
	s = strings.ReplaceAll(s, repo, "<repo>")
	s = strings.ReplaceAll(s, state, "<state>")
	s = strings.ReplaceAll(s, base, "<base-commit>")
	s = strings.ReplaceAll(s, "snapshot: "+base[:20]+"... (truncated)", "snapshot: <base-prefix>... (truncated)")
	s = strings.ReplaceAll(s, candidate, "<candidate-commit>")
	s = strings.ReplaceAll(s, candidate[:12], "<candidate-prefix>")
	// Only generated identities are alpha-renamed, with a separate injective map
	// per role. A different reference remains different, rather than vanishing.
	for _, prefix := range []string{"run", "att"} {
		names := map[string]string{}
		re := regexp.MustCompile(prefix + `_[0-9A-HJKMNP-TV-Z]{26}`)
		s = re.ReplaceAllStringFunc(s, func(id string) string {
			if v, ok := names[id]; ok {
				return v
			}
			v := fmt.Sprintf("<%s-%d>", prefix, len(names)+1)
			names[id] = v
			return v
		})
	}
	pids := regexp.MustCompile(`(?m)^event 9: attempt: worker pid ([1-9][0-9]*), native pid ([1-9][0-9]*)$`).FindStringSubmatch(s)
	if len(pids) != 3 || pids[1] == pids[2] {
		return "", errors.New("missing distinct worker/native identity")
	}
	s = strings.Replace(s, pids[0], "event 9: attempt: worker pid <worker-pid>, native pid <native-pid>", 1)
	// Path separators are volatile only inside the generated evidence path.
	s = strings.ReplaceAll(s, `<state>\runs\`, "<state>/runs/")
	s = strings.ReplaceAll(s, `<run-1>\evidence\`, "<run-1>/evidence/")
	s = strings.ReplaceAll(s, `<candidate-prefix>\demo.log`, "<candidate-prefix>/demo.log")
	return s, nil
}

// Keep counterfactual inputs: normalisation must never erase semantics or
// collapse unequal generated references. I09 (v2 §2): unknown is not zero.
func TestObservatoryNormalisationPreservesMeaning(t *testing.T) {
	t.Parallel()
	base := `{"schema_version":1,"event_id":"evt_01K00000000000000000000000","run_id":"","producer_id":"cli_01K00000000000000000000001","producer_sequence":1,"run_sequence":0,"generation":1,"observed_at":"2026-01-02T03:04:05Z","type":"run.result","payload":{"state":null,"reason":"invalid_arguments","attempt_reason":null,"exit_code":2,"error_category":"invalid_arguments"}}` + "\n"
	want, err := normalizeObservatoryRefusal(base)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, from, to string }{
		{"state word", "null", "\"failed\""}, {"exit result", "\"exit_code\":2", "\"exit_code\":0"}, {"record field", "\"generation\":1", "\"generation\":2"}, {"reason", "invalid_arguments", "different_reason"}, {"record discriminator", "run.result", "run.created"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			changed := strings.Replace(base, tc.from, tc.to, 1)
			got, err := normalizeObservatoryRefusal(changed)
			if err == nil && got == want {
				t.Fatal("semantic mutation normalised away")
			}
		})
	}
	for _, tc := range []struct{ from, to string }{{"evt_01K00000000000000000000000", "evt_01K00000000000000000000002"}, {"2026-01-02T03:04:05Z", "2026-01-03T03:04:05.123Z"}} {
		got, err := normalizeObservatoryRefusal(strings.Replace(base, tc.from, tc.to, 1))
		if err != nil || got != want {
			t.Fatalf("allowed volatility changed result: %v", err)
		}
	}
}

func checkObservatoryDemoCounterfactuals(t *testing.T, raw, normalised string) {
	t.Helper()
	attempt := regexp.MustCompile(`att_[0-9A-HJKMNP-TV-Z]{26}`).FindString(raw)
	candidate := regexp.MustCompile(`(?m)^Candidate: ([0-9a-f]{40})$`).FindStringSubmatch(raw)[1]
	ladder := regexp.MustCompile(`stop-ladder/[a-z0-9-]+`).FindString(raw)
	if ladder == "" {
		t.Fatal("demo lacks a pinned ladder version")
	}
	for _, tc := range []struct{ name, from, to string }{
		{"stop ladder version", ladder, "stop-ladder/wrong"},
		{"provenance label", "(native-reported)", "(estimated)"},
		{"usage counter", "progress: 1 assistant turns", "progress: 0 assistant turns"},
		{"state word", "run: ready_for_review", "run: failed"},
		{"native exit", "native exit: exit code 0", "native exit: exit code 4"},
		{"candidate content", "+ok\n", "+wrong\n"},
		{"attempt identity relationship", attempt, "att_01K00000000000000000000009"},
		{"candidate binding relationship", "bound to " + candidate, "bound to " + strings.Repeat("a", 40)},
		{"event sequence", "event 23:", "event 24:"},
	} {
		t.Run("normalisation/"+tc.name, func(t *testing.T) {
			if !strings.Contains(raw, tc.from) {
				t.Fatalf("counterfactual does not reach %s", tc.name)
			}
			got, err := normalizeObservatoryDemo(strings.Replace(raw, tc.from, tc.to, 1))
			if err == nil && got == normalised {
				t.Fatal("normalisation erased semantic difference")
			}
		})
	}
}

func waitObservatoryReceipt(ctx context.Context, state string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	j, err := journal.OpenReadOnly(ctx, state)
	if err != nil {
		return err
	}
	defer func() { _ = j.Close() }()
	rows, err := j.ListRuns(ctx, 2)
	if err != nil {
		return err
	}
	if len(rows) != 1 {
		return fmt.Errorf("demo journal has %d runs", len(rows))
	}
	timer := time.NewTicker(10 * time.Millisecond)
	defer timer.Stop()
	for {
		events, err := j.Events(ctx, rows[0].RunID, 0)
		if err != nil {
			return err
		}
		for _, ev := range events {
			if ev.Type == "receipt.written" {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func TestObservatoryReceiptWaitWithTruncatedStatePath(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), strings.Repeat("x", 100))
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(root, "mythhelm-demo-state-first")
	if err := os.Mkdir(state, 0o700); err != nil {
		t.Fatal(err)
	}
	j := openJournal(t, state)
	defer func() { _ = j.Close() }()
	if err := j.Append(t.Context(), journal.Event{SchemaVersion: 1, EventID: "evt_receipt", RunID: "run_long", ProducerID: "sup", ProducerSequence: 1, Generation: 1, ObservedAt: time.Now(), Type: "receipt.written", Payload: json.RawMessage(`{"schema_version":1}`)}, func(tx *sql.Tx) error {
		return journal.InsertRun(t.Context(), tx, journal.RunRow{RunID: "run_long", State: "ready_for_review", AdapterID: "builtin/fake", SourceRepo: "synthetic", TaskSHA256: strings.Repeat("0", 64), BillingPosture: "local-scripted", ExecutionProfile: "trusted-host", CreatedAt: time.Now(), UpdatedAt: time.Now()})
	}); err != nil {
		t.Fatal(err)
	}
	line := fitAccessibleLine("event 4: snapshot: "+strings.Repeat("a", 40)+" cloned to "+filepath.Join(state, "runs", "run_long", "workspace")) + "\n"
	if !strings.Contains(line, "... (truncated)") || strings.Contains(line, string(filepath.Separator)+"runs"+string(filepath.Separator)) {
		t.Fatalf("fixture did not truncate before runs: %s", line)
	}
	w := &observatoryDemoWriter{t: t, tempRoot: root}
	n, err := w.Write([]byte(line))
	if err != nil || n != len(line) || w.String() != line {
		t.Fatalf("receipt-backed truncated event: n=%d err=%v output=%q", n, err, w.String())
	}
	if err := os.Mkdir(filepath.Join(root, "mythhelm-demo-state-second"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(line)); err == nil {
		t.Fatal("ambiguous state roots must not choose an arbitrary journal")
	}
}
