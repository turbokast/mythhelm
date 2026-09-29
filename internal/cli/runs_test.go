package cli

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/supervisor"
)

// stateHome points MYTHHELM_HOME at a fresh directory and returns it.
func stateHome(t testing.TB) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("MYTHHELM_HOME", dir)
	return dir
}

func openJournal(t testing.TB, dir string) *journal.Journal {
	t.Helper()
	j, err := journal.Open(t.Context(), dir)
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	return j
}

func rawDB(t *testing.T, dir string) *sql.DB {
	t.Helper()
	p := filepath.ToSlash(filepath.Join(dir, journal.DBName))
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	u := url.URL{Scheme: "file", Path: p, RawQuery: "_pragma=busy_timeout(5000)"}
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func createRun(t testing.TB, j *journal.Journal, p *supervisor.Producer, repo string) string {
	t.Helper()
	runID := ids.New("run")
	if err := supervisor.CreateRun(t.Context(), j, journal.RunRow{
		RunID:            runID,
		AdapterID:        "fake",
		SourceRepo:       repo,
		TaskSHA256:       strings.Repeat("c", 64),
		BillingPosture:   "local-scripted",
		ExecutionProfile: "trusted-host",
	}, p); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	return runID
}

// seedRow journals a run.created event and projects row exactly as given, so
// golden output has fixed IDs and times.
func seedRow(t *testing.T, j *journal.Journal, seq int64, row journal.RunRow) {
	t.Helper()
	ev := journal.Event{
		SchemaVersion:    journal.EnvelopeVersion,
		EventID:          ids.New("evt"),
		RunID:            row.RunID,
		ProducerID:       "sup_golden",
		ProducerSequence: seq,
		Generation:       1,
		ObservedAt:       row.CreatedAt,
		Type:             "run.created",
		Payload:          json.RawMessage(`{"state":"created","reason":null}`),
	}
	if err := j.Append(t.Context(), ev, func(tx *sql.Tx) error {
		return journal.InsertRun(t.Context(), tx, row)
	}); err != nil {
		t.Fatalf("seeding %s: %v", row.RunID, err)
	}
}

func dbSHA256(t *testing.T, dir string) [32]byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Clean(filepath.Join(dir, journal.DBName)))
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(b)
}

type runRowLine struct {
	Type             string    `json:"type"`
	RunID            string    `json:"run_id"`
	State            string    `json:"state"`
	Reason           *string   `json:"reason"`
	AdapterID        string    `json:"adapter_id"`
	SourceRepo       string    `json:"source_repo"`
	SourceBranch     *string   `json:"source_branch"`
	BaseRev          *string   `json:"base_rev"`
	TaskSHA256       string    `json:"task_sha256"`
	BillingPosture   string    `json:"billing_posture"`
	ExecutionProfile string    `json:"execution_profile"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func decodeRunRows(t *testing.T, stdout string) []runRowLine {
	t.Helper()
	if stdout == "" {
		return nil
	}
	if !strings.HasSuffix(stdout, "\n") {
		t.Fatalf("stdout does not end in a newline: %q", stdout)
	}
	var out []runRowLine
	for _, line := range strings.Split(strings.TrimSuffix(stdout, "\n"), "\n") {
		dec := json.NewDecoder(strings.NewReader(line))
		dec.DisallowUnknownFields()
		var row runRowLine
		if err := dec.Decode(&row); err != nil {
			t.Fatalf("line is not one run_row object: %v: %q", err, line)
		}
		if dec.More() {
			t.Fatalf("line holds more than one JSON value: %q", line)
		}
		out = append(out, row)
	}
	return out
}

func TestRunsListReadsProjectionsOnly(t *testing.T) {
	dir := stateHome(t)
	j := openJournal(t, dir)
	p := supervisor.NewProducer(ids.New("sup"), 1)
	journaled := createRun(t, j, p, "/tmp/repo")
	if err := supervisor.TransitionRun(t.Context(), j, journaled, supervisor.RunAdmission, "", p); err != nil {
		t.Fatal(err)
	}

	// Fixtures that only a projection reader can see: a run row with no
	// journal events at all, and a projected state that differs from the
	// last journaled one.
	db := rawDB(t, dir)
	projectionOnly := ids.New("run")
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.ExecContext(t.Context(), `INSERT INTO runs (run_id, state, adapter_id, source_repo, task_sha256,
		billing_posture, execution_profile, created_at, updated_at)
		VALUES (?, 'executing', 'fake', '/tmp/other', ?, 'local-scripted', 'trusted-host', ?, ?)`,
		projectionOnly, strings.Repeat("d", 64), now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `UPDATE runs SET state = 'failed', reason = 'verification_failed'
		WHERE run_id = ?`, journaled); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	before, beforeFiles := dbSHA256(t, dir), dirFiles(t, dir)

	ro, err := journal.OpenReadOnly(t.Context(), dir)
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	rows, err := ro.ListRuns(t.Context(), 10)
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	write := ro.Append(t.Context(), journal.Event{
		SchemaVersion: journal.EnvelopeVersion, EventID: ids.New("evt"), RunID: journaled,
		ProducerID: "sup_probe", ProducerSequence: 1, Generation: 1, ObservedAt: time.Now(),
		Type: "run.state_changed", Payload: json.RawMessage(`{"state":"admission","reason":null}`),
	}, nil)
	if err := ro.Close(); err != nil {
		t.Fatal(err)
	}
	if write == nil {
		t.Fatal("Append through the read-only handle succeeded; the handle must refuse writes")
	}

	got := map[string]string{}
	for _, r := range rows {
		got[r.RunID] = r.State + "/" + r.Reason
	}
	want := map[string]string{journaled: "failed/verification_failed", projectionOnly: "executing/"}
	if len(got) != len(want) || got[journaled] != want[journaled] || got[projectionOnly] != want[projectionOnly] {
		t.Fatalf("ListRuns = %v, want the projected states %v", got, want)
	}

	code, stdout, stderr := runMain("runs", "list", "--format", "jsonl")
	if code != int(ExitOK) {
		t.Fatalf("runs list exit %d, stderr %q", code, stderr)
	}
	for _, r := range decodeRunRows(t, stdout) {
		reason := ""
		if r.Reason != nil {
			reason = *r.Reason
		}
		if want[r.RunID] != r.State+"/"+reason {
			t.Errorf("runs list row %s = %s/%s, want %s", r.RunID, r.State, reason, want[r.RunID])
		}
	}
	if after := dbSHA256(t, dir); after != before {
		t.Fatal("reading the run list changed the database file")
	}
	// A read-only SQLite connection to a WAL database may leave its WAL
	// index (-shm) and an empty -wal beside it; it adds nothing else.
	for name, size := range dirFiles(t, dir) {
		_, existed := beforeFiles[name]
		switch {
		case existed, name == journal.DBName+"-shm":
		case name == journal.DBName+"-wal" && size == 0:
		default:
			t.Errorf("reading the run list added %s (%d bytes)", name, size)
		}
	}
}

func dirFiles(t *testing.T, dir string) map[string]int64 {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]int64{}
	for _, e := range entries {
		fi, err := e.Info()
		if err != nil {
			t.Fatal(err)
		}
		out[e.Name()] = fi.Size()
	}
	return out
}

func TestRunsListJSONLOneObjectPerLine(t *testing.T) {
	dir := stateHome(t)
	j := openJournal(t, dir)
	p := supervisor.NewProducer(ids.New("sup"), 1)
	var runIDs []string
	for _, repo := range []string{"/tmp/a", "/tmp/with space", "/tmp/uniçode"} {
		runIDs = append(runIDs, createRun(t, j, p, repo))
		time.Sleep(2 * time.Millisecond) // distinct ID milliseconds, so newest-first order is fixed
	}
	if err := supervisor.TransitionRun(t.Context(), j, runIDs[1], supervisor.RunAdmission, "", p); err != nil {
		t.Fatal(err)
	}
	if err := supervisor.TransitionRun(t.Context(), j, runIDs[1], supervisor.RunBlocked, "trust_required", p); err != nil {
		t.Fatal(err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := runMain("runs", "list", "--format", "jsonl")
	if code != int(ExitOK) || stderr != "" {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	rows := decodeRunRows(t, stdout)
	if len(rows) != 3 {
		t.Fatalf("got %d lines, want 3: %q", len(rows), stdout)
	}
	for i, r := range rows {
		wantID := runIDs[len(runIDs)-1-i]
		if r.Type != "run_row" || r.RunID != wantID {
			t.Errorf("line %d = type %q run %q, want run_row %q (newest first)", i, r.Type, r.RunID, wantID)
		}
		if r.AdapterID != "fake" || r.BillingPosture != "local-scripted" || r.ExecutionProfile != "trusted-host" ||
			r.TaskSHA256 != strings.Repeat("c", 64) || r.CreatedAt.IsZero() || r.UpdatedAt.Before(r.CreatedAt) {
			t.Errorf("line %d has wrong projected fields: %+v", i, r)
		}
		if r.SourceBranch != nil || r.BaseRev != nil {
			t.Errorf("line %d: unset source_branch/base_rev must be null, got %v %v", i, r.SourceBranch, r.BaseRev)
		}
	}
	if blocked := rows[1]; blocked.State != "blocked" || blocked.Reason == nil || *blocked.Reason != "trust_required" {
		t.Errorf("blocked row = %+v, want blocked/trust_required", blocked)
	}
	if created := rows[0]; created.State != "created" || created.Reason != nil || created.SourceRepo != "/tmp/uniçode" {
		t.Errorf("created row = %+v, want created with a null reason", created)
	}
	if !strings.Contains(stdout, `"reason":null`) {
		t.Errorf("an empty reason must be an explicit null: %q", stdout)
	}
}

func TestRunsListPlainGolden(t *testing.T) {
	dir := stateHome(t)
	j := openJournal(t, dir)
	at := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	for i, row := range []journal.RunRow{
		{RunID: "run_01K6A000000000000000000001", State: "ready_for_review", Reason: "unverified", AdapterID: "fake",
			SourceRepo: "/tmp/repo", CreatedAt: at, UpdatedAt: at.Add(90 * time.Second)},
		{RunID: "run_01K6A000000000000000000002", State: "failed", Reason: "verification_failed", AdapterID: "claudecode",
			SourceRepo: "/tmp/with space", SourceBranch: "main", BaseRev: strings.Repeat("0", 40),
			CreatedAt: at.Add(time.Hour), UpdatedAt: at.Add(time.Hour + 500*time.Millisecond)},
		{RunID: "run_01K6A000000000000000000003", State: "executing", AdapterID: "fake",
			SourceRepo: "/tmp/evil\x1b[2J\x1b]8;;http://example.com\x07name\trun_forged  completed\nx", CreatedAt: at.Add(2 * time.Hour), UpdatedAt: at.Add(2 * time.Hour)},
	} {
		row.TaskSHA256 = strings.Repeat("e", 64)
		row.BillingPosture = "local-scripted"
		row.ExecutionProfile = "trusted-host"
		seedRow(t, j, int64(i+1), row)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := runMain("runs", "list")
	if code != int(ExitOK) || stderr != "" {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	want := "" +
		"RUN                             STATE                          ADAPTER     UPDATED               REPOSITORY\n" +
		"run_01K6A000000000000000000003  executing                      fake        2026-09-29T14:00:00Z  /tmp/evilname\\trun_forged  completed\\nx\n" +
		"run_01K6A000000000000000000002  failed (verification_failed)   claudecode  2026-09-29T13:00:00Z  /tmp/with space\n" +
		"run_01K6A000000000000000000001  ready_for_review (unverified)  fake        2026-09-29T12:01:30Z  /tmp/repo\n"
	if stdout != want {
		t.Errorf("plain output mismatch\n got:\n%s\nwant:\n%s", stdout, want)
	}
}

func TestRunsListLimit(t *testing.T) {
	dir := stateHome(t)
	j := openJournal(t, dir)
	p := supervisor.NewProducer(ids.New("sup"), 1)
	var newest string
	for range 5 {
		newest = createRun(t, j, p, "/tmp/repo")
		time.Sleep(2 * time.Millisecond)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := runMain("runs", "list", "--limit", "2", "--format", "jsonl")
	if code != int(ExitOK) {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	rows := decodeRunRows(t, stdout)
	if len(rows) != 2 || rows[0].RunID != newest {
		t.Fatalf("got %d rows starting %v, want the 2 newest starting %s", len(rows), rows, newest)
	}
}

func TestRunsListWithoutDatabase(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "never-created")
	t.Setenv("MYTHHELM_HOME", dir)

	code, stdout, stderr := runMain("runs", "list")
	if code != int(ExitOK) || stdout != "no runs\n" {
		t.Fatalf("plain: exit %d, stdout %q, stderr %q; want 0 and \"no runs\"", code, stdout, stderr)
	}
	code, stdout, stderr = runMain("runs", "list", "--format", "jsonl")
	if code != int(ExitOK) || stdout != "" {
		t.Fatalf("jsonl: exit %d, stdout %q, stderr %q; want 0 and no lines", code, stdout, stderr)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("runs list created the state directory (stat err %v); a read must not write", err)
	}
}

func TestRunsListNewerSchemaExits2(t *testing.T) {
	dir := stateHome(t)
	db := rawDB(t, dir)
	if _, err := db.ExecContext(t.Context(), `CREATE TABLE future (x INTEGER); PRAGMA user_version = 99`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	before := dbSHA256(t, dir)

	code, stdout, stderr := runMain("runs", "list")
	if code != int(ExitInvalid) {
		t.Fatalf("exit %d, want %d (stderr %q)", code, ExitInvalid, stderr)
	}
	if stdout != "" || !strings.Contains(stderr, "restore") || strings.Contains(stderr, "for usage") {
		t.Fatalf("stdout %q, stderr %q; want no output and a restore instruction without a usage hint", stdout, stderr)
	}
	if dbSHA256(t, dir) != before {
		t.Fatal("runs list changed a database it refused to read")
	}
}

func TestRunsUsageErrors(t *testing.T) {
	stateHome(t)
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"no subcommand", []string{"runs"}, "missing subcommand"},
		{"unknown subcommand", []string{"runs", "show"}, `unknown subcommand "show"`},
		{"bad format", []string{"runs", "list", "--format", "yaml"}, `--format must be plain or jsonl, got "yaml"`},
		{"zero limit", []string{"runs", "list", "--limit", "0"}, "--limit must be at least 1"},
		{"extra argument", []string{"runs", "list", "run_1"}, `unexpected argument "run_1"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, stderr := runMain(tt.args...)
			if code != int(ExitInvalid) || stdout != "" || !strings.Contains(stderr, tt.want) {
				t.Fatalf("exit %d, stdout %q, stderr %q; want 2 and %q", code, stdout, stderr, tt.want)
			}
		})
	}
}

func TestRunsHelp(t *testing.T) {
	for _, args := range [][]string{{"runs", "-h"}, {"runs", "list", "-h"}} {
		code, stdout, stderr := runMain(args...)
		if code != int(ExitOK) || !strings.Contains(stdout, "runs list") {
			t.Errorf("%v: exit %d, stdout %q, stderr %q", args, code, stdout, stderr)
		}
	}
}

// BenchmarkRunsList100 measures `runs list` end to end (open, query, render)
// over 100 runs. NFR-5 sets an advisory target of 200 ms on CI Linux.
func BenchmarkRunsList100(b *testing.B) {
	dir := stateHome(b)
	j := openJournal(b, dir)
	p := supervisor.NewProducer(ids.New("sup"), 1)
	for range 100 {
		createRun(b, j, p, "/tmp/repo")
	}
	if err := j.Close(); err != nil {
		b.Fatal(err)
	}
	stdio := Stdio{In: strings.NewReader(""), Out: io.Discard, Err: io.Discard}
	for b.Loop() {
		if code := Main([]string{"runs", "list", "--limit", "100"}, stdio); code != 0 {
			b.Fatalf("exit %d", code)
		}
	}
}
