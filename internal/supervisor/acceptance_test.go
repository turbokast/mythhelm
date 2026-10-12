package supervisor_test

import (
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/internal/acceptance"
	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/contain"
	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/integration"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/supervisor"
	"github.com/turbokast/mythhelm/internal/tui/viewmodel"
	"github.com/turbokast/mythhelm/internal/v2contract"
)

// admissionPin decodes the run's journaled admission.decided payload as the
// production struct and returns the pin with the payload's raw keys.
func admissionPin(t *testing.T, f fixture, runID string) (acceptance.Pin, map[string]json.RawMessage) {
	t.Helper()
	evs := f.events(t, runID)
	i := typeIndex(evs, "admission.decided")
	if i < 0 {
		t.Fatal("no admission.decided event")
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(evs[i].Payload, &raw); err != nil {
		t.Fatal(err)
	}
	var payload supervisor.AdmissionPayload
	if err := json.Unmarshal(evs[i].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	return payload.AcceptancePin, raw
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestAdmissionJournalsPin(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	digest := f.config(t, "pass")
	if code, _, stderr := f.run(t, f.checkedRun(digest)...); code != 0 {
		t.Fatalf("run exit %d: %s", code, stderr)
	}
	run := f.onlyRun(t)
	pin, raw := admissionPin(t, f, run.RunID)
	if _, ok := raw["acceptance_pin"]; !ok {
		t.Fatalf("admission.decided has no acceptance_pin key: %v", slices.Sorted(maps.Keys(raw)))
	}
	ver, err := f.journal(t).LatestVerification(t.Context(), run.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if ver.EvaluatorDigest == "" {
		t.Fatal("the verification row has no evaluator digest")
	}
	if !strings.HasPrefix(pin.TaskID, "task_") {
		t.Errorf("pin task id %q is not a task id", pin.TaskID)
	}
	want := acceptance.Pin{
		PolicyID: "P1", PolicyVersion: 1, PolicyDigest: v2contract.Digest(acceptance.P1()),
		TaskID: pin.TaskID, TaskRevision: 1, Contract: "sha256:" + run.TaskSHA256,
		CheckDefinition: "sha256:" + digest, EvaluatorName: "host", EvaluatorDigest: "sha256:" + ver.EvaluatorDigest,
		CheckTimeouts: map[string]string{"test": "5s"},
	}
	if got, wantJSON := mustJSON(t, pin), mustJSON(t, want); got != wantJSON {
		t.Fatalf("acceptance_pin\n got %s\nwant %s", got, wantJSON)
	}
}

func TestPinOmitsEvaluatorWhenChecksNotRun(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		args func(t *testing.T, f *fixture) []string
	}{
		{name: "no-checks waives the list", args: func(_ *testing.T, f *fixture) []string { return f.fakeRun() }},
		{name: "inspect refuses the list", args: func(t *testing.T, f *fixture) []string {
			if runtime.GOOS != "linux" {
				t.Skip("inspect is qualified on Linux only")
			}
			requireBoundary(t)
			f.home = outsideTmpDir(t)
			digest := f.config(t, "pass")
			args := slices.DeleteFunc(f.checkedRun(digest), func(a string) bool { return a == "trusted-host" || a == "--execution-profile" })
			return append(args, "--execution-profile", "inspect", "--scenario", "readonly")
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			args := tc.args(t, &f)
			if code, _, stderr := f.run(t, args...); code != 5 {
				t.Fatalf("run exit %d: %s", code, stderr)
			}
			run := f.onlyRun(t)
			pin, _ := admissionPin(t, f, run.RunID)
			if pin.PolicyID != "P1" || pin.PolicyVersion != 1 || pin.Contract != "sha256:"+run.TaskSHA256 {
				t.Errorf("pin policy/contract = %s/%d/%s, want P1/1/sha256:%s", pin.PolicyID, pin.PolicyVersion, pin.Contract, run.TaskSHA256)
			}
			if pin.CheckDefinition != "" || pin.EvaluatorName != "" || pin.EvaluatorDigest != "" || pin.CheckTimeouts != nil {
				t.Errorf("pin names checks that were not run: %+v", pin)
			}
		})
	}
}

func TestPolicyRowSeededOnceAcrossRuns(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	for range 2 {
		if code, _, stderr := f.run(t, f.fakeRun()...); code != 5 {
			t.Fatalf("run exit %d: %s", code, stderr)
		}
	}
	if n := len(f.runs(t)); n != 2 {
		t.Fatalf("recorded runs = %d, want 2", n)
	}
	rows, err := rawDB(t, f.state).QueryContext(t.Context(), `SELECT policy_id, version, digest FROM policies`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var got []string
	for rows.Next() {
		var id, digest string
		var version int
		if err := rows.Scan(&id, &version, &digest); err != nil {
			t.Fatal(err)
		}
		got = append(got, id+"/"+strconv.Itoa(version)+"/"+digest)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if want := []string{"P1/1/" + v2contract.Digest(acceptance.P1())}; !slices.Equal(got, want) {
		t.Fatalf("policies rows = %v, want %v", got, want)
	}
}

func TestOldAdmissionReadsAsNoPin(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if code, _, stderr := f.run(t, f.fakeRun()...); code != 5 {
		t.Fatalf("run exit %d: %s", code, stderr)
	}
	runID := f.onlyRun(t).RunID
	j := f.journal(t)

	pin, ok, err := supervisor.ReadPin(t.Context(), j, runID)
	if err != nil || !ok || pin.PolicyID != "P1" {
		t.Fatalf("ReadPin of a pinned run = %+v, %v, %v; want the P1 pin", pin, ok, err)
	}

	// The decoders that predate the pin still read a pinned payload.
	snap, err := viewmodel.Load(t.Context(), f.state, runID)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Admission == nil || snap.Admission.AdapterID != "builtin/fake" {
		t.Errorf("viewmodel admission = %+v, want the fake adapter", snap.Admission)
	}
	receipt, err := supervisor.BuildReceipt(t.Context(), j, runID)
	if err != nil {
		t.Fatalf("BuildReceipt of a pinned run: %v", err)
	}
	if bundle, _ := receipt["execution_bundle"].(map[string]any); bundle["adapter"] == nil {
		t.Errorf("receipt lost its adapter: %v", receipt["execution_bundle"])
	}

	// admit creates a run and, when payload is set, journals it as the run's admission.
	admit := func(t *testing.T, payload []byte) string {
		t.Helper()
		id := ids.New("run")
		prod := supervisor.NewProducer(ids.New("sup"), 1)
		if err := supervisor.CreateRun(t.Context(), j, journal.RunRow{RunID: id, AdapterID: "builtin/fake", SourceRepo: f.repo,
			BaseRev: strings.Repeat("a", 40), TaskSHA256: strings.Repeat("b", 64), BillingPosture: "local-scripted", ExecutionProfile: "trusted-host"}, prod); err != nil {
			t.Fatal(err)
		}
		if payload == nil {
			return id
		}
		if err := j.Append(t.Context(), journal.Event{SchemaVersion: journal.EnvelopeVersion, EventID: ids.New("evt"), RunID: id, Type: "admission.decided",
			ProducerID: ids.New("fixture"), ProducerSequence: 1, Generation: 1, ObservedAt: time.Now().UTC(), Payload: payload}, nil); err != nil {
			t.Fatal(err)
		}
		return id
	}
	for _, tc := range []struct {
		name    string
		payload []byte
	}{
		{name: "an admission record without the key has no pin", payload: []byte(mustJSON(t, admission.Record{}))},
		{name: "a run without an admission has no pin"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pin, ok, err := supervisor.ReadPin(t.Context(), j, admit(t, tc.payload))
			if err != nil || ok || pin.PolicyID != "" || pin.Contract != "" {
				t.Fatalf("ReadPin = %+v, %v, %v; want zero, false, nil", pin, ok, err)
			}
		})
	}
	t.Run("a corrupt key is an error, never no pin", func(t *testing.T) {
		pin, ok, err := supervisor.ReadPin(t.Context(), j, admit(t, []byte(`{"acceptance_pin":{"policy_version":"one"}}`)))
		if err == nil || ok {
			t.Fatalf("ReadPin = %+v, %v, %v; want a decode error", pin, ok, err)
		}
	})
}

func TestPinAndVerificationEvaluatorAgree(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		contained bool
	}{
		{name: "trusted-host"},
		{name: "restricted", contained: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			if tc.contained {
				if runtime.GOOS != "linux" {
					t.Skip("the restricted boundary is qualified on Linux only")
				}
				requireBoundary(t)
				f.home = outsideTmpDir(t)
			}
			var digest string
			if tc.contained {
				// The boundary hides the test binary's temp directory, so
				// the contained check is a shell command.
				digest = f.shellConfig(t)
			} else {
				digest = f.config(t, "pass")
			}
			args := f.checkedRun(digest)
			if tc.contained {
				args = slices.DeleteFunc(args, func(a string) bool { return a == "trusted-host" || a == "--execution-profile" })
			}
			if code, _, stderr := f.run(t, args...); code != 0 {
				t.Fatalf("run exit %d: %s", code, stderr)
			}
			runID := f.onlyRun(t).RunID
			pin, _ := admissionPin(t, f, runID)
			ver, err := f.journal(t).LatestVerification(t.Context(), runID)
			if err != nil {
				t.Fatal(err)
			}
			if ver.EvaluatorDigest == "" || pin.EvaluatorDigest != "sha256:"+ver.EvaluatorDigest {
				t.Fatalf("pin evaluator digest %q, verification %q; want the same digest", pin.EvaluatorDigest, ver.EvaluatorDigest)
			}
			if pin.EvaluatorName != ver.EvaluatorName || (pin.EvaluatorName == "host") == tc.contained {
				t.Fatalf("evaluator name pin %q, verification %q, contained %v", pin.EvaluatorName, ver.EvaluatorName, tc.contained)
			}
		})
	}
}

// shellConfig commits a one-check configuration whose check is `true` under
// /bin/sh and returns its digest.
func (f fixture) shellConfig(t *testing.T) string {
	t.Helper()
	contents := "schema_version = 1\n[[checks]]\nname = \"test\"\nargv = [\"/bin/sh\", \"-c\", \"true\"]\ntimeout = \"5s\"\n"
	if err := os.WriteFile(filepath.Join(f.repo, "mythhelm.toml"), []byte(contents), 0o600); err != nil { // #nosec G703 -- fixture repo is t.TempDir
		t.Fatal(err)
	}
	f.git(t, "add", "mythhelm.toml")
	f.git(t, "commit", "--quiet", "-m", "add shell check")
	_, digest, err := admission.ParseProjectConfig([]byte(contents))
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

// TestPinEvaluatorFollowsProfile needs no boundary: a contained decision pins
// the boundary's evaluator and a host one the host's, each equal to what the
// verification stage computes for the same options.
func TestPinEvaluatorFollowsProfile(t *testing.T) {
	t.Parallel()
	checks := []admission.CheckConfig{{Name: "test", Argv: []string{"/bin/sh", "-c", "true"}, Timeout: "5s"}}
	cases := []struct {
		name      string
		contained bool
		wantName  string
		opts      integration.RunOptions
	}{
		{name: "trusted-host pins the host evaluator", wantName: "host"},
		{name: "restricted pins the boundary evaluator", contained: true, wantName: contain.BoundaryName,
			opts: integration.RunOptions{Policy: &contain.Policy{Profile: admission.ProfileRestricted}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d := admission.Decision{TaskID: "task_x", ConfigDigest: strings.Repeat("c", 64), ProjectConfig: admission.ProjectConfig{Checks: checks},
				Task: admission.Task{SHA256: strings.Repeat("d", 64)}, Profile: admission.Profile{Name: "profile", Contained: tc.contained}}
			payload, err := supervisor.NewAdmissionPayload(d)
			if err != nil {
				t.Fatal(err)
			}
			want, err := integration.EvaluatorFor(tc.opts, checks)
			if err != nil {
				t.Fatal(err)
			}
			pin := payload.AcceptancePin
			if pin.EvaluatorName != tc.wantName || pin.EvaluatorDigest != "sha256:"+want.Digest {
				t.Fatalf("pin evaluator %s/%s, want %s/sha256:%s", pin.EvaluatorName, pin.EvaluatorDigest, tc.wantName, want.Digest)
			}
		})
	}
}

// TestPinLeavesChecksUnknownWhenNoneRun needs no boundary: a waived list and
// an inspect run pin nothing about checks, however many are admitted.
func TestPinLeavesChecksUnknownWhenNoneRun(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		noChecks bool
		profile  string
	}{
		{name: "waived by --no-checks", noChecks: true, profile: "trusted-host"},
		{name: "refused under inspect", profile: admission.ProfileInspect},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d := admission.Decision{TaskID: "task_x", NoChecks: tc.noChecks, ConfigDigest: strings.Repeat("c", 64),
				ProjectConfig: admission.ProjectConfig{Checks: []admission.CheckConfig{{Name: "test", Argv: []string{"/bin/sh", "-c", "true"}, Timeout: "5s"}}},
				Task:          admission.Task{SHA256: strings.Repeat("d", 64)}, Profile: admission.Profile{Name: tc.profile}}
			payload, err := supervisor.NewAdmissionPayload(d)
			if err != nil {
				t.Fatal(err)
			}
			pin := payload.AcceptancePin
			if pin.Contract != "sha256:"+strings.Repeat("d", 64) {
				t.Errorf("pin contract = %q, want the task digest", pin.Contract)
			}
			if pin.CheckDefinition != "" || pin.EvaluatorName != "" || pin.EvaluatorDigest != "" || pin.CheckTimeouts != nil {
				t.Errorf("pin names checks that will not run: %+v", pin)
			}
		})
	}
}

func TestPinFailureBlocksBeforeWorker(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.journal(t) // creates and migrates the database the conflicting row goes into
	other := acceptance.P1()
	other.ActionFamily = "review"
	if _, err := rawDB(t, f.state).ExecContext(t.Context(), `INSERT INTO policies (policy_id, version, record, digest) VALUES (?, ?, ?, ?)`,
		acceptance.PolicyID, acceptance.PolicyVersion, mustJSON(t, other), v2contract.Digest(other)); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := f.run(t, f.fakeRun()...)
	if code != 3 || !strings.Contains(stderr, "acceptance_pin_failed") {
		t.Fatalf("exit %d, stderr %q; want exit 3 acceptance_pin_failed", code, stderr)
	}
	run := f.onlyRun(t)
	if run.State != "blocked" || run.Reason != "acceptance_pin_failed" {
		t.Fatalf("run %s/%s, want blocked/acceptance_pin_failed", run.State, run.Reason)
	}
	var attempts int
	if err := rawDB(t, f.state).QueryRowContext(t.Context(), `SELECT COUNT(*) FROM attempts WHERE run_id = ?`, run.RunID).Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if attempts != 0 {
		t.Errorf("a blocked admission left %d attempt rows", attempts)
	}
	for _, ev := range f.events(t, run.RunID) {
		if ev.Type == "admission.decided" || strings.HasPrefix(ev.Type, "attempt.") || ev.Type == "workspace.snapshot_created" {
			t.Errorf("event %s journaled after a failed pin", ev.Type)
		}
	}
	if _, err := os.Stat(filepath.Join(f.state, "runs", run.RunID, "workspace")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("workspace stat = %v; want none for a run blocked before any worker", err)
	}
}
