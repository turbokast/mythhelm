package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/turbokast/mythhelm/internal/qualify"
)

// hashTree hashes every file under root: relative path, mode and content.
// A read-only command leaves the digest unchanged.
func hashTree(t *testing.T, root string) string {
	t.Helper()
	h := sha256.New()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		_, _ = h.Write([]byte(rel + "\x00" + info.Mode().String() + "\x00"))
		if d.IsDir() {
			return nil
		}
		// #nosec G304 G122 -- hashing the test's own fixture tree, which holds no symlinks
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		_, _ = h.Write(b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func seedHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(`{"hooks":{"PreToolUse":[{"hooks":[{}]}]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".claude.json"), []byte(`{"mcpServers":{"demo":{"command":"demo"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestDoctorIsReadOnly(t *testing.T) {
	home := seedHome(t)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	state := t.TempDir()
	if err := os.WriteFile(filepath.Join(state, "keep.db"), []byte("state-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MYTHHELM_HOME", state)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	before := map[string]string{"home": hashTree(t, home), "state": hashTree(t, state), "repo": hashTree(t, cwd)}
	code, _, stderr := runMain("doctor")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	for name, root := range map[string]string{"home": home, "state": state, "repo": cwd} {
		if got := hashTree(t, root); got != before[name] {
			t.Errorf("%s tree changed under a read-only doctor", name)
		}
	}
	missing := filepath.Join(t.TempDir(), "no-such-state")
	t.Setenv("MYTHHELM_HOME", missing)
	if code, _, stderr := runMain("doctor"); code != 0 {
		t.Fatalf("absent-state exit %d, stderr %q", code, stderr)
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Errorf("doctor created the absent state dir: %v", err)
	}
}

func TestDoctorNeverPrintsCredentialValues(t *testing.T) {
	// Names and values are assembled at run time so the secret scans
	// never see a credential literal in this file.
	secrets := map[string]string{
		"ANTHROPIC_" + "API_KEY":       "planted-" + "anthropic-secret",
		"CLAUDE_CODE_" + "OAUTH_TOKEN": "planted-" + "oauth-secret",
		"AWS_" + "SECRET_ACCESS_KEY":   "planted-" + "aws-secret",
	}
	for name, value := range secrets {
		t.Setenv(name, value)
	}
	for _, format := range []string{"plain", "jsonl"} {
		args := []string{"doctor"}
		if format == "jsonl" {
			args = append(args, "--format", "jsonl")
		}
		code, stdout, stderr := runMain(args...)
		if code != 0 {
			t.Fatalf("%s exit %d, stderr %q", format, code, stderr)
		}
		for name, value := range secrets {
			if strings.Contains(stdout, value) || strings.Contains(stderr, value) {
				t.Errorf("%s leaked the value of %s", format, name)
			}
			if !strings.Contains(stdout, name) {
				t.Errorf("%s names no %s route", format, name)
			}
		}
	}
}

func TestDoctorDoesNotRunAuthStatus(t *testing.T) {
	// A logging fake on PATH records every invocation; doctor may ask
	// for --version but must never ask for auth status.
	bin := t.TempDir()
	log := filepath.Join(t.TempDir(), "invocations")
	t.Setenv("FAKECLAUDE_LOG", log)
	if runtime.GOOS == "windows" {
		script := "@echo off\r\necho %*>> \"%FAKECLAUDE_LOG%\"\r\nif \"%1\"==\"--version\" echo 9.9.9 (Claude Code)\r\n"
		if err := os.WriteFile(filepath.Join(bin, "claude.cmd"), []byte(script), 0o600); err != nil {
			t.Fatal(err)
		}
	} else {
		script := "#!/bin/sh\necho \"$@\" >> \"$FAKECLAUDE_LOG\"\nif [ \"$1\" = \"--version\" ]; then echo \"9.9.9 (Claude Code)\"; fi\nexit 0\n"
		if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(script), 0o700); err != nil { // #nosec G306 -- executable test fixture under t.TempDir
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	code, stdout, stderr := runMain("doctor")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if !strings.Contains(stdout, "9.9.9") {
		t.Fatalf("doctor did not report the fake version:\n%s", stdout)
	}
	if !strings.Contains(stdout, "run admission to query") {
		t.Fatalf("doctor does not defer auth to admission:\n%s", stdout)
	}
	raw, err := os.ReadFile(log) // #nosec G304 -- the test's own invocation log
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "--version") {
		t.Errorf("fake saw no version query: %q", raw)
	}
	if strings.Contains(string(raw), "auth") {
		t.Errorf("doctor ran an auth query: %q", raw)
	}
}

func TestDoctorUnreadableStateDirIsNotAbsent(t *testing.T) {
	// The trigger is platform-specific: a regular file as the parent
	// makes Lstat fail with ENOTDIR on Unix, but Windows reports the
	// same path as not-exist, so Windows uses a name the OS rejects.
	var missing string
	if runtime.GOOS == "windows" {
		missing = filepath.Join(t.TempDir(), "state*?")
	} else {
		blocker := filepath.Join(t.TempDir(), "file-not-dir")
		if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		missing = filepath.Join(blocker, "state")
	}
	t.Setenv("MYTHHELM_HOME", missing)
	code, stdout, stderr := runMain("doctor")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if !strings.Contains(stdout, "unreadable") || strings.Contains(stdout, "absent (not created)") {
		t.Fatalf("unreadable state reported as:\n%s", stdout)
	}
}

func TestRunBoundedCapsOutput(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	out, err := runBoundedEnv(exe, []string{"__bigout"}, os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != maxProbeOutput {
		t.Fatalf("captured %d bytes, want exactly the %d-byte cap", len(out), maxProbeOutput)
	}
}

func TestDoctorClaudeMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	code, stdout, stderr := runMain("doctor")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if !strings.Contains(stdout, "claude: not found") {
		t.Fatalf("doctor did not report the missing native:\n%s", stdout)
	}
}

func TestDoctorJsonl(t *testing.T) {
	code, stdout, stderr := runMain("doctor", "--format", "jsonl")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	var rep struct {
		Type          string         `json:"type"`
		Mythhelm      map[string]any `json:"mythhelm"`
		Git           map[string]any `json:"git"`
		Claude        map[string]any `json:"claude"`
		CredEnv       []string       `json:"credential_env_names"`
		Settings      map[string]any `json:"native_settings"`
		Sandbox       map[string]any `json:"sandbox"`
		StateDir      map[string]any `json:"state_dir"`
		Terminal      map[string]any `json:"terminal"`
		Qualification map[string]any `json:"qualification"`
	}
	if err := json.Unmarshal([]byte(stdout), &rep); err != nil {
		t.Fatalf("doctor jsonl: %v\n%s", err, stdout)
	}
	if rep.Type != "doctor" || rep.Mythhelm == nil || rep.Git == nil || rep.Claude == nil ||
		rep.CredEnv == nil || rep.Settings == nil || rep.Sandbox == nil || rep.StateDir == nil || rep.Terminal == nil ||
		rep.Qualification == nil {
		t.Fatalf("doctor jsonl misses a section: %+v", rep)
	}
	if rep.Claude["auth"] != "run admission to query" {
		t.Fatalf("doctor jsonl auth = %v", rep.Claude["auth"])
	}
}

// seedQualification migrates dir and seeds the seven v2 §7.2 records, then
// closes the write handle: doctor must find all seven through a read-only
// open.
func seedQualification(t *testing.T, dir string) {
	t.Helper()
	j := openJournal(t, dir)
	if err := qualify.EnsureSeeded(t.Context(), j); err != nil {
		t.Fatalf("EnsureSeeded: %v", err)
	}
	if err := j.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// qualLine matches one plain qualification record line:
// <harness> × <surface>: <progress> (fidelity <v>, entitlement <v>,
// lifecycle <v>) [evidence <n> revs, latest <ev-id>; drift <state>].
var qualLine = regexp.MustCompile(`^([a-z0-9-]+) × (.+?): (planned|documented-candidate|fixture-tested|live-qualified|experimental|blocked|unsupported) \(fidelity (proven|not-proven|unknown), entitlement (proven|not-proven|unknown), lifecycle (proven|not-proven|unknown)\) \[evidence ([0-9]+) revs, latest (\S+); drift (.+)\]$`)

// qualificationLines returns the record lines of the plain qualification
// section: every line after the `qualification:` header.
func qualificationLines(t *testing.T, stdout string) []string {
	t.Helper()
	lines := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
	for i, line := range lines {
		if line == "qualification:" {
			return lines[i+1:]
		}
	}
	t.Fatalf("plain output has no qualification section:\n%s", stdout)
	return nil
}

func TestDoctorShowsQualification(t *testing.T) {
	// Not parallel: points MYTHHELM_HOME at a seeded temp state dir.
	dir := stateHome(t)
	seedQualification(t, dir)

	code, stdout, stderr := runMain("doctor")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	got := qualificationLines(t, stdout)
	if len(got) != 7 {
		t.Fatalf("qualification section holds %d lines, want one per seeded record (7):\n%s", len(got), stdout)
	}
	wantProgress := map[string]string{
		"claude-code": "blocked",
		"codex":       "planned",
		"opencode":    "planned",
		"muse":        "planned",
		"kimi":        "planned",
		"cursor":      "planned",
		"antigravity": "planned",
	}
	var harnesses []string
	for _, line := range got {
		m := qualLine.FindStringSubmatch(line)
		if m == nil {
			t.Errorf("line does not match the record shape:\n%s", line)
			continue
		}
		harness, surface, progress := m[1], m[2], m[3]
		harnesses = append(harnesses, harness)
		if surface != "unknown" {
			t.Errorf("%s surface = %q, want unknown (unestablished seed)", harness, surface)
		}
		if want := wantProgress[harness]; progress != want {
			t.Errorf("%s progress = %s, want %s", harness, progress, want)
		}
		for i, col := range []string{"fidelity", "entitlement", "lifecycle"} {
			if m[4+i] != "unknown" {
				t.Errorf("%s %s = %s, want unknown (seed claims no verdict)", harness, col, m[4+i])
			}
		}
		if m[7] != "1" {
			t.Errorf("%s evidence revs = %s, want 1 (fresh seed)", harness, m[7])
		}
		if m[8] != "none" {
			t.Errorf("%s latest = %s, want none (seed carries no evidence)", harness, m[8])
		}
		if m[9] != "clean" {
			t.Errorf("%s drift = %q, want clean (unknown pins cannot drift)", harness, m[9])
		}
	}
	wantOrder := []string{"antigravity", "claude-code", "codex", "cursor", "kimi", "muse", "opencode"}
	if !slices.Equal(harnesses, wantOrder) {
		t.Errorf("harness order = %v, want %v (harness, surface order)", harnesses, wantOrder)
	}
}

// errNoEvidenceRevisions and errNoDriftTriggers are the static failures of
// checkQualificationRecord for the two nested required fields.
var (
	errNoEvidenceRevisions = errors.New("record lacks evidence_revisions")
	errNoDriftTriggers     = errors.New("record lacks drift_triggers")
)

// checkQualificationRecord reports whether m carries every per-record field
// the doctor JSONL contract promises: identity, progress, per-column verdict
// + evidence ids, evidence_revisions and drift_triggers. A record missing any
// field fails the caller.
func checkQualificationRecord(m map[string]any) error {
	for _, key := range []string{"harness", "surface", "progress", "drift", "latest_evidence", "next_test"} {
		s, ok := m[key].(string)
		if !ok || s == "" {
			return fmt.Errorf("record lacks %q", key)
		}
	}
	for _, col := range []string{"fidelity", "entitlement", "lifecycle"} {
		cm, ok := m[col].(map[string]any)
		if !ok {
			return fmt.Errorf("record lacks %q", col)
		}
		if v, ok := cm["verdict"].(string); !ok || v == "" {
			return fmt.Errorf("record lacks %s.verdict", col)
		}
		ev, ok := cm["evidence"].([]any)
		if !ok {
			return fmt.Errorf("record lacks %s.evidence", col)
		}
		for _, id := range ev {
			if _, ok := id.(string); !ok {
				return fmt.Errorf("record %s.evidence holds a non-string id", col)
			}
		}
	}
	if _, ok := m["evidence_revisions"].(float64); !ok {
		return errNoEvidenceRevisions
	}
	dt, ok := m["drift_triggers"].(map[string]any)
	if !ok {
		return errNoDriftTriggers
	}
	for _, key := range []string{"executable_digest", "config_digest"} {
		if _, ok := dt[key].(string); !ok {
			return fmt.Errorf("record lacks drift_triggers.%s", key)
		}
	}
	return nil
}

func TestDoctorQualificationJSONL(t *testing.T) {
	// Not parallel: points MYTHHELM_HOME at a seeded temp state dir.
	dir := stateHome(t)
	seedQualification(t, dir)

	code, stdout, stderr := runMain("doctor", "--format", "jsonl")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if strings.Count(stdout, "\n") != 1 {
		t.Fatalf("jsonl output is not one object: %q", stdout)
	}
	var rep map[string]any
	if err := json.Unmarshal([]byte(stdout), &rep); err != nil {
		t.Fatalf("doctor jsonl: %v\n%s", err, stdout)
	}
	qual, ok := rep["qualification"].(map[string]any)
	if !ok {
		t.Fatalf("jsonl object has no qualification map: %q", stdout)
	}
	raw, ok := qual["records"].([]any)
	if !ok || len(raw) != 7 {
		t.Fatalf("qualification records = %v, want the 7 seeded records", qual["records"])
	}
	seen := map[string]string{}
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			t.Errorf("record is not an object: %v", item)
			continue
		}
		if err := checkQualificationRecord(m); err != nil {
			t.Errorf("record fails the field check: %v (harness %v)", err, m["harness"])
			continue
		}
		seen[m["harness"].(string)] = m["progress"].(string)
		if revs := m["evidence_revisions"].(float64); revs != 1 {
			t.Errorf("%s evidence_revisions = %v, want 1", m["harness"], revs)
		}
	}
	if seen["claude-code"] != "blocked" || len(seen) != 7 {
		t.Errorf("record progress = %v, want claude-code blocked and 7 records", seen)
	}

	// Red legs: the field check rejects a record missing any required
	// field, so a silent drop fails this test instead of passing it.
	base, ok := raw[0].(map[string]any)
	if !ok {
		t.Fatalf("first record is not an object: %v", raw[0])
	}
	for _, path := range [][]string{
		{"progress"}, {"fidelity"}, {"entitlement"}, {"lifecycle"},
		{"evidence_revisions"}, {"drift_triggers"}, {"drift"},
		{"fidelity", "verdict"}, {"entitlement", "evidence"},
		{"drift_triggers", "config_digest"},
	} {
		mut := cloneJSON(t, base)
		delPath(mut, path)
		if err := checkQualificationRecord(mut); err == nil {
			t.Errorf("checkQualificationRecord accepts a record missing %v", path)
		}
	}
}

// cloneJSON deep-copies m through JSON, for red-leg mutants.
func cloneJSON(t *testing.T, m map[string]any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// delPath deletes the nested key at path from m.
func delPath(m map[string]any, path []string) {
	for _, key := range path[:len(path)-1] {
		m, _ = m[key].(map[string]any)
		if m == nil {
			return
		}
	}
	delete(m, path[len(path)-1])
}

func TestDoctorQualificationUnavailableHonest(t *testing.T) {
	// Not parallel: points MYTHHELM_HOME at a path that must not exist.
	missing := filepath.Join(t.TempDir(), "no-such-state")
	t.Setenv("MYTHHELM_HOME", missing)

	code, stdout, stderr := runMain("doctor")
	if code != 0 {
		t.Fatalf("plain exit %d, stderr %q", code, stderr)
	}
	if !strings.Contains(stdout, "qualification: unavailable (") {
		t.Fatalf("plain output names no unavailable qualification section:\n%s", stdout)
	}
	if !strings.Contains(stdout, missing) {
		t.Errorf("unavailable reason does not name the missing dir:\n%s", stdout)
	}
	if strings.Contains(stdout, " × ") {
		t.Errorf("unavailable registry fabricated record rows:\n%s", stdout)
	}

	code, stdout, stderr = runMain("doctor", "--format", "jsonl")
	if code != 0 {
		t.Fatalf("jsonl exit %d, stderr %q", code, stderr)
	}
	var rep map[string]any
	if err := json.Unmarshal([]byte(stdout), &rep); err != nil {
		t.Fatalf("doctor jsonl: %v\n%s", err, stdout)
	}
	qual, ok := rep["qualification"].(map[string]any)
	if !ok {
		t.Fatalf("jsonl object has no qualification map: %q", stdout)
	}
	status, _ := qual["status"].(string)
	if !strings.HasPrefix(status, "unavailable (") {
		t.Errorf("qualification status = %q, want unavailable (<reason>)", status)
	}
	if _, has := qual["records"]; has {
		t.Errorf("unavailable registry fabricated records: %v", qual["records"])
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Errorf("doctor created the absent state dir: %v", err)
	}
}

func TestDoctorQualificationEmptyHonest(t *testing.T) {
	// Not parallel: points MYTHHELM_HOME at existing but unseeded dirs.
	// Doctor never seeds, so both read as honestly no records yet.
	t.Run("dir without a database", func(t *testing.T) {
		dir := stateHome(t)
		if err := os.WriteFile(filepath.Join(dir, "keep.db"), []byte("state-bytes"), 0o600); err != nil {
			t.Fatal(err)
		}
		code, stdout, stderr := runMain("doctor")
		if code != 0 {
			t.Fatalf("exit %d, stderr %q", code, stderr)
		}
		if !strings.Contains(stdout, "qualification: (no records)") {
			t.Fatalf("plain output reads no honest empty section:\n%s", stdout)
		}
	})
	t.Run("migrated but unseeded database", func(t *testing.T) {
		dir := stateHome(t)
		if err := openJournal(t, dir).Close(); err != nil {
			t.Fatal(err)
		}
		code, stdout, stderr := runMain("doctor", "--format", "jsonl")
		if code != 0 {
			t.Fatalf("exit %d, stderr %q", code, stderr)
		}
		var rep map[string]any
		if err := json.Unmarshal([]byte(stdout), &rep); err != nil {
			t.Fatalf("doctor jsonl: %v\n%s", err, stdout)
		}
		qual, ok := rep["qualification"].(map[string]any)
		if !ok {
			t.Fatalf("jsonl object has no qualification map: %q", stdout)
		}
		recs, ok := qual["records"].([]any)
		if !ok || recs == nil || len(recs) != 0 {
			t.Fatalf("qualification records = %v, want an empty list, never null or fabricated", qual["records"])
		}
	})
}

// hashFiles maps each top-level file in dir to the hex SHA-256 of its
// content. The state dir is flat (mythhelm.db plus SQLite sidecars), so no
// recursion is needed.
func hashFiles(t *testing.T, dir string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	out := make(map[string]string, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		// #nosec G304 -- reading the test's own temp state dir listing
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(b)
		out[e.Name()] = hex.EncodeToString(sum[:])
	}
	return out
}

// diffHashes describes how two file-hash maps differ: added, removed or
// changed files. Empty means identical.
func diffHashes(before, after map[string]string) string {
	var diffs []string
	for name, sum := range before {
		afterSum, ok := after[name]
		switch {
		case !ok:
			diffs = append(diffs, name+" removed")
		case afterSum != sum:
			diffs = append(diffs, name+" changed")
		}
	}
	for name := range after {
		if _, ok := before[name]; !ok {
			diffs = append(diffs, name+" added")
		}
	}
	slices.Sort(diffs)
	return strings.Join(diffs, ", ")
}

func TestDoctorStillWritesNothing(t *testing.T) {
	// Not parallel: points MYTHHELM_HOME at a seeded temp state dir.
	dir := stateHome(t)
	seedQualification(t, dir)

	// Warm one read-only open first: a WAL reader may create -shm/-wal
	// sidecars on first touch.
	reg, err := qualify.OpenReadOnly(t.Context(), dir)
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	if _, err := reg.List(t.Context()); err != nil {
		t.Fatalf("List: %v", err)
	}
	if err := reg.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	before := hashFiles(t, dir)
	for _, args := range [][]string{{"doctor"}, {"doctor", "--format", "jsonl"}} {
		if code, _, stderr := runMain(args...); code != 0 {
			t.Fatalf("%v exit %d, stderr %q", args, code, stderr)
		}
		if diff := diffHashes(before, hashFiles(t, dir)); diff != "" {
			t.Fatalf("%v changed the state dir: %s", args, diff)
		}
	}

	// Red variant: the same comparison detects a write, so the green
	// above proves read-only rather than a blind comparison.
	if err := os.WriteFile(filepath.Join(dir, "marker.tmp"), []byte("marker"), 0o600); err != nil {
		t.Fatal(err)
	}
	if diff := diffHashes(before, hashFiles(t, dir)); diff == "" {
		t.Fatal("the comparison missed a planted marker file; it cannot prove read-only")
	}
}
