package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/adapters/fake"
	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/control"
	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/workers"
	"github.com/turbokast/mythhelm/internal/workspace"
)

func TestMain(m *testing.M) {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "__adoption-service":
			if len(os.Args) != 6 {
				os.Exit(2)
			}
			os.Exit(crashAdoptionService(os.Args[2], os.Args[3], os.Args[4], os.Args[5]))
		case workers.Command:
			os.Exit(workers.Main(os.Args[2:]))
		case fake.AgentCommand:
			os.Exit(fake.AgentMain(os.Args[2:], os.Stdin, os.Stdout, os.Stderr))
		}
	}
	os.Exit(m.Run())
}

type adoptionFixture struct {
	r            *runtime
	run, attempt string
	proc         *os.Process
	launch       workers.Launch
	pin          control.LaunchProvenanceV1
}

func newAdoptionFixture(t *testing.T, scenarios ...string) adoptionFixture {
	t.Helper()
	r := testRuntime(t)
	ctx := t.Context()
	repo := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com")
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git: %v: %s", err, b)
		}
	}
	git("init", "--quiet", "--initial-branch=main")
	git("config", "user.name", "Test")
	git("config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("fixture\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", "README.md")
	git("commit", "--quiet", "-m", "fixture")
	task := filepath.Join(t.TempDir(), "task.md")
	if err := os.WriteFile(task, []byte("# Test\n\nWrite demo.txt.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	scenario := "slow"
	if len(scenarios) > 0 {
		scenario = scenarios[0]
	}
	d, err := admission.Decide(ctx, admission.Request{StateDir: r.journal.StateDir(), Repo: repo, TaskFile: task, Adapter: "fake", Billing: "local-scripted", ExecutionProfile: "trusted-host", NoChecks: true, Scenario: scenario, Env: os.Environ()})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(d.RunDir, 0700); err != nil {
		t.Fatal(err)
	}
	prompt := filepath.Join(d.RunDir, "task.md")
	if err := os.WriteFile(prompt, d.Task.Content, 0600); err != nil {
		t.Fatal(err)
	}
	if err := workspace.Snapshot(ctx, d.Snapshot.SourceRepo, d.Snapshot.BaseRev, d.Workdir); err != nil {
		t.Fatal(err)
	}
	producer := supervisor.NewProducer(ids.New("sup"), 1)
	if err := supervisor.CreateRun(ctx, r.journal, journal.RunRow{RunID: d.RunID, AdapterID: d.Adapter.ID, SourceRepo: d.Snapshot.SourceRepo, SourceBranch: d.Snapshot.Branch, BaseRev: d.Snapshot.BaseRev, TaskSHA256: d.Task.SHA256, BillingPosture: d.Proposal.Billing.Mode, ExecutionProfile: d.Profile.Name}, producer); err != nil {
		t.Fatal(err)
	}
	if err := supervisor.TransitionRun(ctx, r.journal, d.RunID, supervisor.RunAdmission, "", producer); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(d.Record())
	if err != nil {
		t.Fatal(err)
	}
	if err := r.journal.Append(ctx, journal.Event{SchemaVersion: 1, EventID: ids.New("evt"), RunID: d.RunID, ProducerID: ids.New("fixture"), ProducerSequence: 1, Generation: 1, ObservedAt: time.Now().UTC(), Type: "admission.decided", Payload: raw}, nil); err != nil {
		t.Fatal(err)
	}
	if err := supervisor.TransitionRun(ctx, r.journal, d.RunID, supervisor.RunExecuting, "", producer); err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("1", 64)
	sum := sha256.Sum256([]byte(token))
	deadline := time.Now().Add(5 * time.Minute).UTC()
	gen := r.lease.Generation() - 1
	for _, option := range scenarios {
		if option == "legacy_zero" {
			gen = 0
		}
	}
	launch := workers.Launch{ExecutionDeadline: deadline, LaunchToken: token, TaskID: d.TaskID, AdapterID: d.Adapter.ID, Path: d.Proposal.Spec.Path, NativeSHA256: d.Probe.SHA256, Args: d.Proposal.Spec.Args, Dir: d.Workdir, Env: d.Proposal.Spec.Env, PromptPath: prompt, StopLadder: d.Proposal.StopLadder, Generation: gen, Nonce: strings.Repeat("2", 64), LadderVersion: "stop-ladder/v1"}
	if err := supervisor.RecordLaunchIntent(ctx, r.journal, journal.AttemptRow{AttemptID: d.AttemptID, RunID: d.RunID, TaskID: d.TaskID, AttemptNumber: 1, LaunchTokenSHA256: hex.EncodeToString(sum[:]), WorkspacePath: d.Workdir}, producer); err != nil {
		t.Fatal(err)
	}
	pin := control.LaunchProvenanceV1{SchemaVersion: 1, RunID: d.RunID, AttemptID: d.AttemptID, LaunchTokenSHA256: hex.EncodeToString(sum[:]), NonceSHA256: workers.NonceDigest(launch.Nonce), LadderVersion: launch.LadderVersion, Generation: &gen, ExecutionDeadline: deadline}
	nested, err := json.Marshal(pin)
	if err != nil {
		t.Fatal(err)
	}
	for _, option := range scenarios {
		if option == "missing_provenance" {
			nested = nil
		}
	}
	raw, err = json.Marshal(control.AdmissionPinnedPayload{LadderVersion: launch.LadderVersion, NonceSHA256: pin.NonceSHA256, LaunchProvenance: nested})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.journal.Append(ctx, journal.Event{SchemaVersion: 1, EventID: ids.New("evt"), RunID: d.RunID, AttemptID: d.AttemptID, ProducerID: ids.New("pin"), ProducerSequence: 1, Generation: 1, ObservedAt: time.Now().UTC(), Type: control.EventAdmissionPinned, Payload: raw}, nil); err != nil {
		t.Fatal(err)
	}
	if err := r.journal.Transact(ctx, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		_, err := tx.ExecContext(ctx, `INSERT INTO run_envelopes(run_id,execution_seconds,repairs,replans,transport_retries,first_start_at,pause_spans,updated_at) VALUES (?,300,1,0,3,?,'[]',?)`, d.RunID, now, now)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	proc, err := workers.Spawn(os.Args[0], r.journal.StateDir(), d.RunID, d.AttemptID, launch)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		dir := workers.AttemptDir(r.journal.StateDir(), d.RunID, d.AttemptID)
		_ = workers.RequestStop(dir, "fixture-cleanup")
		joined := make(chan struct{})
		go func() { _, _ = proc.Wait(); close(joined) }()
		tick := time.NewTicker(100 * time.Millisecond)
		defer tick.Stop()
		timeout := time.NewTimer(10 * time.Second)
		defer timeout.Stop()
		var beat uint64
		for {
			beat++
			_ = control.WriteSupervisorBeat(dir, r.lease.Generation(), beat, time.Now())
			select {
			case <-tick.C:
				continue
			case <-joined:
				return
			case <-timeout.C:
				// This fixture's slow scripted agent has no descendants.
				id, err := workers.ReadIdentity(dir)
				if err == nil && id.NativePID != nil {
					if child, err := os.FindProcess(*id.NativePID); err == nil {
						_ = child.Kill()
					}
				}
				_ = proc.Kill()
				<-joined
				t.Error("worker fixture did not join after stop and presence")
				return
			}
		}
	})
	until := time.Now().Add(15 * time.Second)
	for {
		_, identityErr := workers.ReadIdentity(workers.AttemptDir(r.journal.StateDir(), d.RunID, d.AttemptID))
		_, spoolErr := os.Stat(filepath.Join(workers.AttemptDir(r.journal.StateDir(), d.RunID, d.AttemptID), supervisor.SpoolFile))
		if identityErr == nil && spoolErr == nil {
			break
		}
		if time.Now().After(until) {
			t.Fatalf("worker not ready: %v %v", identityErr, spoolErr)
		}
		time.Sleep(10 * time.Millisecond)
	}
	return adoptionFixture{r: r, run: d.RunID, attempt: d.AttemptID, proc: proc, launch: launch, pin: pin}
}

func TestServiceGenerationAdoption(t *testing.T) {
	f := newAdoptionFixture(t)
	ctx := t.Context()
	before, err := f.r.verifyAdoption(ctx, f.run)
	if err != nil {
		t.Fatal(err)
	}
	if before.WorkerPID != f.proc.Pid || before.LaunchGeneration != f.launch.Generation || before.ControlGeneration != f.r.lease.Generation() || before.ControlGeneration <= before.LaunchGeneration {
		t.Fatalf("wrong adoption: %+v", before)
	}
	if err := f.r.journal.Transact(ctx, func(tx *sql.Tx) error { return control.RecordServiceAdoption(ctx, tx, before) }); err != nil {
		t.Fatal(err)
	}
	if err := f.r.journal.Transact(ctx, func(tx *sql.Tx) error { return control.RecordServiceAdoption(ctx, tx, before) }); err != nil {
		t.Fatalf("exact replay: %v", err)
	}
	after, err := control.ReadLaunchProvenance(ctx, f.r.journal, f.run, f.attempt)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ExecutionDeadline.Equal(f.launch.ExecutionDeadline) || *after.Generation != f.launch.Generation {
		t.Fatal("adoption rewrote launch pin")
	}
	id, err := workers.ReadIdentity(workers.AttemptDir(f.r.journal.StateDir(), f.run, f.attempt))
	if err != nil || id.PID != f.proc.Pid {
		t.Fatalf("worker changed: %+v %v", id, err)
	}
	if err := f.r.journal.Transact(ctx, func(tx *sql.Tx) error {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM attempts WHERE run_id=?`, f.run).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			return errors.New("replacement attempt")
		}
		epoch, ok, err := journal.ReadAssignmentEpoch(ctx, tx, f.run)
		if err != nil {
			return err
		}
		if !ok || epoch.OperationID != before.OperationID {
			return errors.New("missing committed epoch")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAdoptionRefusesUnverifiedLaunch(t *testing.T) {
	for _, name := range []string{"pid_reuse", "foreign_launch", "corrupt_identity", "unsupported_spool", "offset_inside_line"} {
		t.Run(name, func(t *testing.T) {
			f := newAdoptionFixture(t)
			ctx := t.Context()
			dir := workers.AttemptDir(f.r.journal.StateDir(), f.run, f.attempt)
			identity := filepath.Join(dir, "worker.json")
			original, err := os.ReadFile(identity) // #nosec G304 -- fixed worker identity in this test-owned attempt TempDir.
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				_ = os.WriteFile(identity, original, 0600) // #nosec G703 -- restore only the fixed worker identity in this test-owned attempt TempDir.
			})
			switch name {
			case "pid_reuse", "foreign_launch":
				var id workers.Identity
				if err := json.Unmarshal(original, &id); err != nil {
					t.Fatal(err)
				}
				if name == "pid_reuse" {
					id.StartTime = id.StartTime.Add(time.Second)
				} else {
					id.LaunchToken = "foreign"
				}
				raw, err := json.Marshal(id)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(identity, raw, 0600); err != nil {
					t.Fatal(err)
				}
			case "corrupt_identity":
				if err := os.WriteFile(identity, []byte("{"), 0600); err != nil {
					t.Fatal(err)
				}
			case "unsupported_spool":
				file, err := os.OpenFile(filepath.Join(dir, supervisor.SpoolFile), os.O_APPEND|os.O_WRONLY, 0600) // #nosec G304 -- inject one malformed event into this test-owned fixed spool only.
				if err != nil {
					t.Fatal(err)
				}
				raw, err := json.Marshal(journal.Event{SchemaVersion: 2, EventID: "evt_future", RunID: f.run, AttemptID: f.attempt, ProducerID: "wrk_" + f.attempt, ProducerSequence: 99, Generation: 1, ObservedAt: time.Now(), Type: "attempt.progress", Payload: []byte(`{}`)})
				if err != nil {
					t.Fatal(err)
				}
				_, err = file.Write(append(raw, '\n'))
				if err != nil {
					t.Fatal(err)
				}
				if err := file.Close(); err != nil {
					t.Fatal(err)
				}
			case "offset_inside_line":
				if err := f.r.journal.Transact(ctx, func(tx *sql.Tx) error { return journal.SetSpoolOffset(ctx, tx, f.attempt, 1) }); err != nil {
					t.Fatal(err)
				}
			}
			before, err := f.r.journal.Events(ctx, f.run, 0)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.r.verifyAdoption(ctx, f.run); err == nil {
				t.Fatal("unverified launch was accepted by the actual pre-adoption verifier")
			}
			after, err := f.r.journal.Events(ctx, f.run, 0)
			if err != nil || len(after) != len(before) {
				t.Fatalf("refusal mutated journal: %d -> %d %v", len(before), len(after), err)
			}
		})
	}
}

// The crash helper accepts only a parent-created fixture capability: the open
// root must contain the random witness whose hash the parent supplies. Reads
// use the root capability rather than following arbitrary argument paths.
func crashAdoptionService(run, mode, marker, proof string) int {
	// The parent binds this process cwd to its exact state root through cmd.Dir;
	// the helper accepts no root/path argument and holds that cwd capability.
	fixtureRoot, err := os.OpenRoot(".")
	if err != nil {
		return 2
	}
	defer func() { _ = fixtureRoot.Close() }()
	witness, err := fixtureRoot.ReadFile("adoption-fixture-proof")
	if err != nil || len(witness) != 32 {
		return 2
	}
	digest := sha256.Sum256(witness)
	if hex.EncodeToString(digest[:]) != proof {
		return 2
	}
	ctx := context.Background()
	state, err := os.Getwd()
	if err != nil {
		return 2
	}
	// The kernel-bound cwd and fixed witness identify the actual parent-owned
	// fixture; normal runtime physical-root/database validation still runs.
	r, err := openRuntime(ctx, state)
	if err != nil {
		return 2
	}
	if mode == "before" {
		_, err := r.reconcileRunCheckpoint(ctx, run, nil, func() {
			if err := os.WriteFile(marker, []byte(mode), 0600); err != nil {
				os.Exit(8)
			}
			for {
				time.Sleep(time.Second)
			} // parent kills the real transaction owner.
		})
		if err != nil {
			return 3
		}
		return 9 // Reaching here would mean the mid-transaction barrier failed.
	}
	a, err := r.reconcileRun(ctx, run)
	if err != nil {
		return 4
	}
	until := time.Now().Add(10 * time.Second)
	for {
		committed := false
		events, err := r.journal.Events(ctx, run, 0)
		if err != nil {
			return 5
		}
		for _, e := range events {
			if e.Type == "service.adopted" {
				committed = true
			}
		}
		if committed {
			break
		}
		select {
		case <-a.done:
			return 6
		default:
		}
		if time.Now().After(until) {
			return 7
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := os.WriteFile(marker, []byte(mode), 0600); err != nil { // #nosec G703 -- parent-created marker path in its private TempDir, passed only to this validated own-fixture helper.
		return 8
	}
	for {
		time.Sleep(time.Second)
	}
}

func TestAdoptionCrashDoesNotRelaunch(t *testing.T) {
	f := newAdoptionFixture(t, "count-launches")
	state := f.r.journal.StateDir()
	ctx := t.Context()
	var witness [32]byte
	if _, err := rand.Read(witness[:]); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state, "adoption-fixture-proof"), witness[:], 0600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(witness[:])
	proof := hex.EncodeToString(digest[:])
	// A valid-shaped argument naming an unrelated empty root has no fixture
	// witness and must be refused before runtime preparation creates a DB.
	unrelated := t.TempDir()
	rejected := exec.Command(os.Args[0], "__adoption-service", f.run, "before", filepath.Join(t.TempDir(), "rejected-marker"), proof) // #nosec G702 -- own test executable, fixed helper verb and test-owned refusal fixture; no shell.
	rejected.Dir = unrelated
	if err := rejected.Run(); err == nil {
		t.Fatal("unproved fixture root was accepted")
	}
	if _, err := os.Stat(filepath.Join(unrelated, journal.DBName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unproved fixture root was mutated: %v", err)
	}
	if err := f.r.shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"before", "after"} {
		marker := filepath.Join(t.TempDir(), "service-phase")
		cmd := exec.Command(os.Args[0], "__adoption-service", f.run, mode, marker, proof) // #nosec G702 -- re-exec this test binary with fixed helper verb and parent-owned fixture capability; no shell.
		cmd.Dir = state
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		joined := false
		t.Cleanup(func() {
			if !joined {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
			}
		})
		awaitAdoptionCondition(t, func() bool {
			raw, err := os.ReadFile(marker) // #nosec G304 -- fixed crash-phase marker in this test-owned TempDir.
			return err == nil && string(raw) == mode
		})
		if err := cmd.Process.Kill(); err != nil {
			t.Fatal(err)
		}
		if err := cmd.Wait(); err == nil {
			t.Fatal("service was not killed at crash boundary")
		}
		joined = true
		id, err := workers.ReadIdentity(workers.AttemptDir(state, f.run, f.attempt))
		if err != nil || id.PID != f.proc.Pid {
			t.Fatalf("crash changed worker: %+v %v", id, err)
		}
		actualStart, err := workers.ProcessStartTime(id.PID)
		if err != nil || !actualStart.Equal(id.StartTime) {
			t.Fatalf("worker did not survive service crash: %v", err)
		}
		j, err := journal.OpenReadOnly(ctx, state)
		if err != nil {
			t.Fatal(err)
		}
		events, err := j.Events(ctx, f.run, 0)
		if err != nil {
			t.Fatal(err)
		}
		adopted := 0
		for _, e := range events {
			if e.Type == "service.adopted" {
				adopted++
				if _, err := control.DecodeServiceAdoption(e); err != nil {
					t.Fatal(err)
				}
			}
		}
		want := 0
		if mode == "after" {
			want = 1
		}
		if adopted != want {
			t.Fatalf("%s crash has %d adoption events, want%d", mode, adopted, want)
		}
		if err := j.Transact(ctx, func(tx *sql.Tx) error {
			var operations, epochs int
			if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM operations WHERE method='service.adopt' AND object=?`, f.run).Scan(&operations); err != nil {
				return err
			}
			if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM service_assignment_epochs WHERE run_id=?`, f.run).Scan(&epochs); err != nil {
				return err
			}
			if operations != want || epochs != want {
				return errors.New("crash left partial adoption operation or epoch")
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if mode == "after" {
			if err := j.Transact(ctx, func(tx *sql.Tx) error {
				epoch, ok, err := journal.ReadAssignmentEpoch(ctx, tx, f.run)
				if err != nil {
					return err
				}
				if !ok || epoch.AttemptID != f.attempt {
					return errors.New("lost committed operation/epoch")
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		}
		pin, err := control.ReadLaunchProvenance(ctx, j, f.run, f.attempt)
		if err != nil {
			t.Fatal(err)
		}
		if !pin.ExecutionDeadline.Equal(f.launch.ExecutionDeadline) || *pin.Generation != f.launch.Generation {
			t.Fatal("crash changed launch envelope")
		}
		if err := j.Close(); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := os.ReadFile(filepath.Join(f.launch.Dir, "launch-count.txt"))
	if err != nil || string(raw) != "launch\n" {
		t.Fatalf("service crash replayed native start: %q %v", raw, err)
	}
}

func TestZeroGenerationNeedsMigration(t *testing.T) {
	t.Run("explicit_adopted_known_zero", func(t *testing.T) {
		f := newAdoptionFixture(t, "slow", "legacy_zero")
		ctx := t.Context()
		if f.launch.Generation != 0 {
			t.Fatal("fixture did not actually launch at zero")
		}
		if _, err := f.r.verifyAdoption(ctx, f.run); err != nil {
			t.Fatalf("explicitly prepared/adopted zero launch refused: %v", err)
		}
		if err := f.r.journal.Transact(ctx, func(tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `UPDATE migration_state SET phase='imported' WHERE id=1`)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := f.r.verifyAdoption(ctx, f.run); err == nil {
			t.Fatal("known-zero launch trusted without adopted root")
		}
	})
	t.Run("absent_provenance_is_unknown", func(t *testing.T) {
		f := newAdoptionFixture(t, "slow", "legacy_zero", "missing_provenance")
		if _, err := f.r.verifyAdoption(t.Context(), f.run); err == nil {
			t.Fatal("absent provenance was treated as known generation zero")
		}
	})
}

func TestAdoptionRefusesSpooledLaunchMismatch(t *testing.T) {
	for _, field := range []string{"worker_pid", "worker_start_time"} {
		t.Run(field, func(t *testing.T) {
			f := newAdoptionFixture(t)
			ctx := t.Context()
			path := filepath.Join(workers.AttemptDir(f.r.journal.StateDir(), f.run, f.attempt), supervisor.SpoolFile)
			// No ingestion has occurred; the only available launch observation is spool.
			awaitAdoptionCondition(t, func() bool {
				raw, err := os.ReadFile(path) // #nosec G304 -- fixed spool in this test-owned attempt root; observes the real launch before deliberate fixture corruption.
				return err == nil && strings.Contains(string(raw), `"attempt.launched"`)
			})
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(string(raw), "\n")
			changed := false
			for i, line := range lines {
				if line == "" {
					continue
				}
				var ev journal.Event
				if err := json.Unmarshal([]byte(line), &ev); err != nil {
					t.Fatal(err)
				}
				if ev.Type != "attempt.launched" {
					continue
				}
				var payload map[string]json.RawMessage
				if err := json.Unmarshal(ev.Payload, &payload); err != nil {
					t.Fatal(err)
				}
				if field == "worker_pid" {
					payload[field] = json.RawMessage(`1`)
				} else {
					payload[field] = json.RawMessage(`"2000-01-01T00:00:00Z"`)
				}
				ev.Payload, err = json.Marshal(payload)
				if err != nil {
					t.Fatal(err)
				}
				data, err := json.Marshal(ev)
				if err != nil {
					t.Fatal(err)
				}
				lines[i] = string(data)
				changed = true
			}
			if !changed {
				t.Fatal("fixture had no actual launched payload to corrupt")
			}
			if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0600); err != nil {
				t.Fatal(err)
			}
			before, err := f.r.journal.Events(ctx, f.run, 0)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.r.verifyAdoption(ctx, f.run); err == nil {
				t.Fatal("mismatched spooled launch was accepted before ingestion")
			}
			after, err := f.r.journal.Events(ctx, f.run, 0)
			if err != nil || len(after) != len(before) {
				t.Fatal("refusal changed events")
			}
			if err := f.r.journal.Transact(ctx, func(tx *sql.Tx) error {
				for _, query := range []string{`SELECT count(*) FROM operations WHERE method='service.adopt' AND object=?`, `SELECT count(*) FROM service_assignment_epochs WHERE run_id=?`} {
					var n int
					if err := tx.QueryRowContext(ctx, query, f.run).Scan(&n); err != nil {
						return err
					}
					if n != 0 {
						return errors.New("refusal left adoption effects")
					}
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAdoptionRefusesNonRegularSpool(t *testing.T) {
	path := filepath.Join(t.TempDir(), "spool.jsonl")
	if goruntime.GOOS == "windows" {
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	} else {
		if out, err := exec.Command("mkfifo", path).CombinedOutput(); err != nil {
			t.Fatalf("create actual FIFO: %v %s", err, out)
		}
	}
	done := make(chan error, 1)
	go func() { done <- checkAdoptionSpool(t.Context(), path, journal.AttemptRow{}, workers.Identity{}) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("nonregular spool accepted")
		}
	case <-time.After(time.Second):
		if goruntime.GOOS != "windows" {
			writer, err := os.OpenFile(path, os.O_WRONLY, 0600)
			if err == nil {
				_ = writer.Close()
			}
		}
		<-done
		t.Fatal("preexisting nonregular spool blocked before type refusal")
	}
}
