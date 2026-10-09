package admission_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/adapters/claudecode"
	"github.com/turbokast/mythhelm/adapters/fake"
	"github.com/turbokast/mythhelm/internal/adapter"
	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/contain"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/workers"
)

// hostileTaskBytes is task-file content shaped to widen launch authority if
// it ever reaches argv: shell metacharacters, newlines, NUL bytes and lines
// shaped like the native's own flags.
var hostileTaskBytes = []byte("# Drop everything\n; rm -rf /\n--allowedTools Bash,Write\n--permission-mode acceptEdits\necho \"pwned\"\nline with \x00 NUL byte\n")

// decideFakeProfile admits a fake-adapter run with the given task bytes and
// execution profile. The child environment is minimal and hermetic.
func decideFakeProfile(t *testing.T, taskBytes []byte, profile string) admission.Decision {
	t.Helper()
	repo, task := envelopeRepo(t, "")
	if err := os.WriteFile(task, taskBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	_, digest, err := admission.ParseProjectConfig(envelopesConfig(""))
	if err != nil {
		t.Fatal(err)
	}
	d, err := admission.Decide(t.Context(), admission.Request{
		StateDir: t.TempDir(), Repo: repo, TaskFile: task, Adapter: admission.AdapterFake,
		Billing: admission.BillingLocalScripted, ExecutionProfile: profile,
		TrustProjectConfig: "sha256:" + digest,
		Env:                []string{"PATH=/usr/bin:/bin", "HOME=" + t.TempDir()},
	})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// assertNoTaskLineInArgv fails when any element of argv contains any
// non-blank line of the task bytes.
func assertNoTaskLineInArgv(t *testing.T, taskBytes []byte, argv []string) {
	t.Helper()
	for line := range strings.SplitSeq(string(taskBytes), "\n") {
		line = strings.Trim(line, " \t\r")
		if line == "" {
			continue
		}
		for _, arg := range argv {
			if strings.Contains(arg, line) {
				t.Errorf("argv element %q contains task line %q (argv %q)", arg, line, argv)
			}
		}
	}
}

// TestTaskBytesNeverReachArgv pins the task-file channel of AC-4.2 (I03):
// task bytes never widen the native argv, while the prompt still arrives on
// the native's stdin byte for byte.
func TestTaskBytesNeverReachArgv(t *testing.T) {
	t.Run("fake proposal argv carries no task line", func(t *testing.T) {
		d := decideFakeProfile(t, hostileTaskBytes, admission.ProfileTrustedHost)
		assertNoTaskLineInArgv(t, hostileTaskBytes, d.Proposal.Spec.Args)
		if slices.Contains(d.Proposal.Spec.Args, "--allowedTools") {
			t.Errorf("argv %q gained --allowedTools from task-shaped input", d.Proposal.Spec.Args)
		}
		// The pipeline writes Task.Content to the prompt file the worker
		// delivers on stdin; admission must preserve the bytes exactly.
		if string(d.Task.Content) != string(hostileTaskBytes) {
			t.Error("admitted task content differs from the task file")
		}
	})

	t.Run("claudecode prompt arrives on stdin byte for byte", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("claudecode Prepare refuses on Windows (no process-group ownership)")
		}
		exe := filepath.Join(t.TempDir(), "claude")
		if err := os.WriteFile(exe, []byte("#!/bin/sh\nexit 0\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		// #nosec G304 -- test fixture the test just wrote
		raw, err := os.ReadFile(exe)
		if err != nil {
			t.Fatal(err)
		}
		lp, err := claudecode.New().Prepare(t.Context(), adapter.PrepareInput{
			Workdir: t.TempDir(), AttemptID: "att_testargv", Env: []string{"HOME=/fixture"},
			Prompt: strings.NewReader(string(hostileTaskBytes)),
			Probe:  adapter.Probe{Executable: exe, SHA256: testSHA256(raw)},
		})
		if err != nil {
			t.Fatal(err)
		}
		assertNoTaskLineInArgv(t, hostileTaskBytes, lp.Spec.Args)
		if slices.Contains(lp.Spec.Args, "--allowedTools") {
			t.Errorf("argv %q gained --allowedTools from task-shaped input", lp.Spec.Args)
		}
		stdin, err := io.ReadAll(lp.Spec.Stdin)
		if err != nil {
			t.Fatal(err)
		}
		if string(stdin) != string(hostileTaskBytes) {
			t.Errorf("stdin %q differs from the task bytes %q", stdin, hostileTaskBytes)
		}
	})
}

func testSHA256(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// boundaryDeltaNames are the fidelity deltas every contained profile
// carries, one per user-visible boundary restriction (AC-4.3).
var boundaryDeltaNames = []string{"read-only mounts", "proxy pin"}

// TestBoundaryRestrictionsAreDeltas pins AC-4.3: a contained proposal
// carries one ConfigDelta per boundary restriction, and the receipt renders
// each in fidelity_differences. A restriction with no delta fails.
func TestBoundaryRestrictionsAreDeltas(t *testing.T) {
	boundaryNames := func(t *testing.T, d admission.Decision) []string {
		t.Helper()
		var names []string
		for _, delta := range d.Proposal.Overrides {
			if delta.Kind == "boundary" {
				names = append(names, delta.Name)
			}
		}
		return names
	}

	t.Run("restricted proposal carries one delta per restriction", func(t *testing.T) {
		if runtime.GOOS != "linux" {
			t.Skip("restricted is admitted only on Linux")
		}
		d := decideFakeProfile(t, []byte("# Demo task\n"), admission.ProfileRestricted)
		if got := boundaryNames(t, d); !slices.Equal(got, boundaryDeltaNames) {
			t.Fatalf("boundary deltas = %q, want %q (all overrides: %+v)", got, boundaryDeltaNames, d.Proposal.Overrides)
		}
	})

	t.Run("inspect workdir reads read-only", func(t *testing.T) {
		if runtime.GOOS != "linux" {
			t.Skip("inspect is admitted only on Linux")
		}
		d := decideFakeProfile(t, []byte("# Demo task\n"), admission.ProfileInspect)
		if got := boundaryNames(t, d); !slices.Equal(got, boundaryDeltaNames) {
			t.Fatalf("boundary deltas = %q, want %q", got, boundaryDeltaNames)
		}
		for _, delta := range d.Proposal.Overrides {
			if delta.Name != "read-only mounts" {
				continue
			}
			if !strings.Contains(delta.Reason, "workdir is read-only") {
				t.Errorf("inspect mounts delta reason %q does not pin a read-only workdir", delta.Reason)
			}
		}
	})

	t.Run("trusted-host carries no boundary delta", func(t *testing.T) {
		d := decideFakeProfile(t, []byte("# Demo task\n"), admission.ProfileTrustedHost)
		if got := boundaryNames(t, d); len(got) != 0 {
			t.Fatalf("trusted-host boundary deltas = %q, want none", got)
		}
	})

	t.Run("unenforced claims get no delta", func(t *testing.T) {
		// Darwin is recorded but unqualified: every claim unenforced.
		ev := contain.SeedV1()["restricted/darwin/builtin/fake"]
		if deltas := admission.BoundaryDeltas(ev); len(deltas) != 0 {
			t.Fatalf("BoundaryDeltas(unqualified) = %+v, want none", deltas)
		}
		if deltas := admission.BoundaryDeltas(contain.Evidence{}); len(deltas) != 0 {
			t.Fatalf("BoundaryDeltas(zero) = %+v, want none", deltas)
		}
		filesystemOnly := contain.Evidence{Profile: "restricted", Boundary: "test/1",
			Coverage: contain.Coverage{Filesystem: contain.Claim{Enforced: true}}}
		if deltas := admission.BoundaryDeltas(filesystemOnly); len(deltas) != 1 || deltas[0].Name != "read-only mounts" {
			t.Fatalf("BoundaryDeltas(filesystem-only) = %+v, want only the mounts delta", deltas)
		}
	})

	t.Run("rendered in receipt fidelity_differences", func(t *testing.T) {
		d := decideFakeProfile(t, []byte("# Demo task\n"), admission.ProfileTrustedHost)
		rec := d.Record()
		rec.Overrides = append(rec.Overrides,
			admission.BoundaryDeltas(contain.SeedV1()["restricted/linux/builtin/fake"])...)
		stateDir := t.TempDir()
		j, err := journal.Open(t.Context(), stateDir)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = j.Close() }()
		prod := supervisor.NewProducer("sup_t9delta", 1)
		run := journal.RunRow{RunID: d.RunID, AdapterID: d.Adapter.ID, SourceRepo: d.Snapshot.SourceRepo,
			SourceBranch: d.Snapshot.Branch, BaseRev: d.Snapshot.BaseRev, TaskSHA256: d.Task.SHA256,
			BillingPosture: d.Proposal.Billing.Mode, ExecutionProfile: d.Profile.Name}
		if err := supervisor.CreateRun(t.Context(), j, run, prod); err != nil {
			t.Fatal(err)
		}
		payload, err := json.Marshal(rec)
		if err != nil {
			t.Fatal(err)
		}
		decided := journal.Event{SchemaVersion: journal.EnvelopeVersion, EventID: "evt_t9delta1",
			RunID: d.RunID, ProducerID: "tst_t9delta", ProducerSequence: 1, Generation: 1,
			ObservedAt: time.Now().UTC(), Type: "admission.decided", Payload: payload}
		if err := j.Append(t.Context(), decided, nil); err != nil {
			t.Fatal(err)
		}
		r, err := supervisor.BuildReceipt(t.Context(), j, d.RunID)
		if err != nil {
			t.Fatal(err)
		}
		fidelity, ok := r["fidelity_differences"].([]string)
		if !ok {
			t.Fatalf("fidelity_differences = %T, want []string", r["fidelity_differences"])
		}
		// Every override renders as "Name: Reason", one entry each: dropping
		// a delta from the proposal drops it from the receipt.
		if len(fidelity) != len(rec.Overrides) {
			t.Fatalf("fidelity has %d entries for %d overrides: %q", len(fidelity), len(rec.Overrides), fidelity)
		}
		for i, delta := range rec.Overrides {
			if want := delta.Name + ": " + delta.Reason; fidelity[i] != want {
				t.Errorf("fidelity[%d] = %q, want %q", i, fidelity[i], want)
			}
		}
		for _, name := range boundaryDeltaNames {
			if !slices.ContainsFunc(fidelity, func(line string) bool {
				return strings.HasPrefix(line, name+": ")
			}) {
				t.Errorf("fidelity %q lacks a %q delta", fidelity, name)
			}
		}
	})
}

// t9binOnce builds the mythhelm binary under test once per test process.
var (
	t9bin     string
	t9binErr  error
	t9binOnce sync.Once
)

func t9build(t *testing.T) string {
	t.Helper()
	t9binOnce.Do(func() {
		_, file, _, ok := runtime.Caller(0)
		if !ok {
			t9binErr = errors.New("cannot locate the test directory")
			return
		}
		root := filepath.Dir(filepath.Dir(filepath.Dir(file)))
		name := "mythhelm-t9"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		bin := filepath.Join(t.TempDir(), name)
		cmd := exec.Command("go", "build", "-o", bin, "./cmd/mythhelm")
		cmd.Dir = root
		if combined, err := cmd.CombinedOutput(); err != nil {
			t9binErr = fmt.Errorf("building mythhelm: %w\n%s", err, combined)
			return
		}
		t9bin = bin
	})
	if t9binErr != nil {
		t.Fatal(t9binErr)
	}
	return t9bin
}

// t9spawn hands l to a real worker and waits for it to exit, returning the
// worker's exit code and recorded identity.
func t9spawn(t *testing.T, bin, stateDir, runID, attemptID string, l workers.Launch) (int, workers.Identity) {
	t.Helper()
	proc, err := workers.Spawn(bin, stateDir, runID, attemptID, l)
	if err != nil {
		t.Fatal(err)
	}
	type waitResult struct {
		state *os.ProcessState
		err   error
	}
	done := make(chan waitResult, 1)
	go func() {
		state, err := proc.Wait()
		done <- waitResult{state, err}
	}()
	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("waiting for the worker: %v", r.err)
		}
		id, err := workers.ReadIdentity(workers.AttemptDir(stateDir, runID, attemptID))
		if err != nil {
			return r.state.ExitCode(), workers.Identity{}
		}
		return r.state.ExitCode(), id
	case <-time.After(90 * time.Second):
		_ = proc.Kill()
		t.Fatal("worker did not exit within 90s")
		return -1, workers.Identity{}
	}
}

// t9terminal reads the spool's last state change.
func t9terminal(t *testing.T, stateDir, runID, attemptID string) (state, reason string) {
	t.Helper()
	// #nosec G304 -- spool inside the test's own state directory
	raw, err := os.ReadFile(filepath.Join(workers.AttemptDir(stateDir, runID, attemptID), "spool.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for line := range strings.Lines(string(raw)) {
		var ev journal.Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatal(err)
		}
		if ev.Type != "attempt.state_changed" {
			continue
		}
		var payload struct {
			State  string  `json:"state"`
			Reason *string `json:"reason"`
		}
		if err := json.Unmarshal(ev.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		state, reason = payload.State, ""
		if payload.Reason != nil {
			reason = *payload.Reason
		}
	}
	if state == "" {
		t.Fatal("spool holds no state change")
	}
	return state, reason
}

// TestWorkerRehashesMutableConfig pins AC-4.1 (I20): the worker re-hashes
// the mutable native-config files behind the admitted digests before exec.
// A file that changed between admission and exec ends the launch as
// launch_failed with no native process created; an unchanged file launches.
func TestWorkerRehashesMutableConfig(t *testing.T) {
	bin := t9build(t)
	// #nosec G304 -- binary the test just built
	binBytes, err := os.ReadFile(bin)
	if err != nil {
		t.Fatal(err)
	}
	binDigest := testSHA256(binBytes)

	launch := func(t *testing.T, dir, taskPath string, env, paths, digests map[string]string) workers.Launch {
		t.Helper()
		return workers.Launch{
			LaunchToken: "tok_t9rehash", TaskID: "task_t9rehash", AdapterID: "builtin/fake",
			Path: bin, NativeSHA256: binDigest,
			Args:            []string{fake.AgentCommand, "--scenario", "happy", "--workdir", dir},
			Dir:             dir,
			Env:             envList(env),
			PromptPath:      taskPath,
			StopLadder:      []adapter.StopStep{{Signal: adapter.StopKill, Grace: 5 * time.Second}},
			UserConfigPaths: paths, UserConfigDigests: digests,
		}
	}
	stage := func(t *testing.T) (dir, taskPath string, env map[string]string) {
		t.Helper()
		dir = filepath.Join(t.TempDir(), "work")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		taskPath = filepath.Join(t.TempDir(), "task.md")
		if err := os.WriteFile(taskPath, []byte("# Rehash probe\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		env = map[string]string{"HOME": t.TempDir(), "PATH": os.Getenv("PATH")}
		return dir, taskPath, env
	}
	writeFile := func(t *testing.T, path, content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	expectLaunched := func(t *testing.T, l workers.Launch, tag string) {
		t.Helper()
		code, id := t9spawn(t, bin, t.TempDir(), "run_rehash_"+tag, "att_rehash_"+tag, l)
		if code != 0 {
			t.Fatalf("worker exit = %d, want 0", code)
		}
		if id.NativePID == nil || id.NativeLaunchIntent == nil {
			t.Fatalf("identity = %+v, want a launched native process", id)
		}
	}
	expectLaunchFailed := func(t *testing.T, l workers.Launch, tag string) {
		t.Helper()
		stateDir := t.TempDir()
		code, id := t9spawn(t, bin, stateDir, "run_rehash_"+tag, "att_rehash_"+tag, l)
		if code != 0 {
			t.Fatalf("worker exit = %d, want 0 (launch_failed still concludes)", code)
		}
		if id.NativePID != nil || id.NativeLaunchIntent != nil {
			t.Fatalf("identity = %+v, want no native process created", id)
		}
		if state, reason := t9terminal(t, stateDir, "run_rehash_"+tag, "att_rehash_"+tag); state != "failed_native" || reason != "launch_failed" {
			t.Fatalf("terminal = %s/%s, want failed_native/launch_failed", state, reason)
		}
	}

	t.Run("unchanged file launches", func(t *testing.T) {
		dir, taskPath, env := stage(t)
		settings := filepath.Join(t.TempDir(), "settings.json")
		writeFile(t, settings, "{\"theme\":\"dark\"}\n")
		l := launch(t, dir, taskPath, env,
			map[string]string{"user": settings},
			map[string]string{"user": fileDigest(t, settings)})
		expectLaunched(t, l, "ok")
	})

	for _, tc := range []struct{ name, tag, source, file, before, after string }{
		{"changed file ends launch_failed with no native process", "mut", "user", "settings.json",
			"{\"theme\":\"dark\"}\n", "{\"theme\":\"light\",\"hooks\":{\"x\":[]}}\n"},
		{"managed source mutated fails", "mgd", "managed", "managed-settings.json",
			"{}\n", "{\"env\":{\"X\":\"1\"}}\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, taskPath, env := stage(t)
			path := filepath.Join(t.TempDir(), tc.file)
			writeFile(t, path, tc.before)
			// The launch is built against the admitted bytes; the file
			// changes between admission and exec.
			l := launch(t, dir, taskPath, env,
				map[string]string{tc.source: path},
				map[string]string{tc.source: fileDigest(t, path)})
			writeFile(t, path, tc.after)
			expectLaunchFailed(t, l, tc.tag)
		})
	}

	t.Run("digest without a path mapping fails", func(t *testing.T) {
		dir, taskPath, env := stage(t)
		settings := filepath.Join(t.TempDir(), "settings.json")
		writeFile(t, settings, "{\"theme\":\"dark\"}\n")
		l := launch(t, dir, taskPath, env,
			map[string]string{},
			map[string]string{"user": fileDigest(t, settings)})
		expectLaunchFailed(t, l, "nomap")
	})

	t.Run("user_mcp selected change fails, unselected change launches", func(t *testing.T) {
		mcpHome := func(t *testing.T) (home string, env map[string]string) {
			t.Helper()
			home = t.TempDir()
			env = map[string]string{"HOME": home, "PATH": os.Getenv("PATH")}
			return home, env
		}
		admittedDigest := func(t *testing.T, home, dir string, env map[string]string) string {
			t.Helper()
			manifest, err := claudecode.InventorySettingsForEnv(home, dir, envList(env))
			if err != nil {
				t.Fatal(err)
			}
			digest := manifest.Digests["user_mcp"]
			if digest == "" {
				t.Fatal("inventory holds no user_mcp digest")
			}
			return digest
		}

		t.Run("selected", func(t *testing.T) {
			dir, taskPath, _ := stage(t)
			home, env := mcpHome(t)
			claudeJSON := filepath.Join(home, ".claude.json")
			writeFile(t, claudeJSON, "{\"mcpServers\":{\"alpha\":{\"command\":\"alpha\"}}}\n")
			l := launch(t, dir, taskPath, env,
				map[string]string{"user_mcp": claudeJSON},
				map[string]string{"user_mcp": admittedDigest(t, home, dir, env)})
			writeFile(t, claudeJSON, "{\"mcpServers\":{\"alpha\":{\"command\":\"alpha\"},\"beta\":{\"command\":\"beta\"}}}\n")
			expectLaunchFailed(t, l, "mcp")
		})

		t.Run("unselected", func(t *testing.T) {
			dir, taskPath, _ := stage(t)
			home, env := mcpHome(t)
			claudeJSON := filepath.Join(home, ".claude.json")
			writeFile(t, claudeJSON, "{\"mcpServers\":{\"alpha\":{\"command\":\"alpha\"}}}\n")
			l := launch(t, dir, taskPath, env,
				map[string]string{"user_mcp": claudeJSON},
				map[string]string{"user_mcp": admittedDigest(t, home, dir, env)})
			// Account data is outside the inventoried MCP subset: the file
			// changed but the selection did not, so the launch proceeds. A
			// re-hash that hashed the whole file would fail here.
			writeFile(t, claudeJSON, "{\"mcpServers\":{\"alpha\":{\"command\":\"alpha\"}},\"oauthAccount\":{\"id\":\"zzz\"}}\n")
			expectLaunched(t, l, "mcpo")
		})
	})

	t.Run("project blob digest never re-hashes the workdir", func(t *testing.T) {
		dir, taskPath, env := stage(t)
		// The digest is bogus on purpose: project sources are committed
		// blobs, immutable, and the live workdir must never be hashed
		// against them. The path need not even exist.
		l := launch(t, dir, taskPath, env,
			map[string]string{"project": filepath.Join(dir, ".claude", "settings.json")},
			map[string]string{"project": strings.Repeat("0", 64)})
		expectLaunched(t, l, "proj")
	})

	t.Run("relative mapping path is an invalid launch", func(t *testing.T) {
		dir, taskPath, env := stage(t)
		l := launch(t, dir, taskPath, env,
			map[string]string{"user": "relative/settings.json"},
			map[string]string{"user": strings.Repeat("0", 64)})
		code, _ := t9spawn(t, bin, t.TempDir(), "run_rehash_rel", "att_rehash_rel", l)
		if code != 2 {
			t.Fatalf("worker exit = %d, want 2 (invalid launch)", code)
		}
	})
}

func envList(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	slices.Sort(out)
	return out
}

func fileDigest(t *testing.T, path string) string {
	t.Helper()
	// #nosec G304 -- fixture the test just wrote
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return testSHA256(raw)
}
