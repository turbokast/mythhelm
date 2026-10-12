package supervisor_test

import (
	"context"
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

// checkBlock renders one [[checks]] table that runs argv.
func checkBlock(name string, argv ...string) string {
	quoted := make([]string, len(argv))
	for i, a := range argv {
		quoted[i] = strconv.Quote(a)
	}
	return "[[checks]]\nname = " + strconv.Quote(name) + "\nargv = [" + strings.Join(quoted, ", ") + "]\ntimeout = \"5s\"\n"
}

// testCheck is a check the test binary answers: mode is pass, fail or output.
func testCheck(name, mode string) string {
	return checkBlock(name, os.Args[0], "__check", mode)
}

// commitChecks commits a configuration of the given check tables and returns
// its trust digest.
func (f fixture) commitChecks(t *testing.T, blocks ...string) string {
	t.Helper()
	contents := "schema_version = 1\n" + strings.Join(blocks, "")
	if err := os.WriteFile(filepath.Join(f.repo, "mythhelm.toml"), []byte(contents), 0o600); err != nil { // #nosec G703 -- fixture repo is t.TempDir
		t.Fatal(err)
	}
	f.git(t, "add", "mythhelm.toml")
	f.git(t, "commit", "--quiet", "-m", "add checks")
	_, digest, err := admission.ParseProjectConfig([]byte(contents))
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

// missingCheck is a check whose executable does not exist.
func missingCheck(t *testing.T, name string) string {
	t.Helper()
	return checkBlock(name, filepath.Join(t.TempDir(), "no-such-check"))
}

func (f fixture) latestVerification(t *testing.T, runID string) journal.VerificationRow {
	t.Helper()
	v, err := f.journal(t).LatestVerification(t.Context(), runID)
	if err != nil {
		t.Fatalf("LatestVerification(%s): %v", runID, err)
	}
	return v
}

// verificationPayload is the payload of the run's verification.completed event.
func (f fixture) verificationPayload(t *testing.T, runID string) map[string]any {
	t.Helper()
	evs := f.events(t, runID)
	i := typeIndex(evs, "verification.completed")
	if i < 0 {
		t.Fatal("no verification.completed event")
	}
	return payloadOf(t, evs[i])
}

func decodeAttestation(t *testing.T, v journal.VerificationRow) v2contract.Verification {
	t.Helper()
	att, err := v2contract.Decode[v2contract.Verification](v.Attestation)
	if err != nil {
		t.Fatalf("attestation %q: %v", v.Attestation, err)
	}
	return att
}

func (f fixture) candidateOf(t *testing.T, runID string) journal.CandidateRow {
	t.Helper()
	j := f.journal(t)
	a, err := j.LatestAttempt(t.Context(), runID)
	if err != nil {
		t.Fatal(err)
	}
	c, err := j.Candidate(t.Context(), a.AttemptID)
	if err != nil {
		t.Fatalf("candidate of %s: %v", runID, err)
	}
	return c
}

// runInProcess admits the checked run for digest and supervises it in this
// process, so a test can pass Hooks.
func (f fixture) runInProcess(t *testing.T, digest string, h supervisor.Hooks) (supervisor.Outcome, error) {
	t.Helper()
	holdInstanceLock(t, f.state)
	d := f.decideEnvelope(t, digest, nil)
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	return supervisor.Run(ctx, d, h)
}

func TestVerdictBoundToCandidateContractChecksEnvironment(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	digest := f.config(t, "pass")
	if code, _, stderr := f.run(t, f.checkedRun(digest)...); code != 0 {
		t.Fatalf("run exit %d: %s", code, stderr)
	}
	run := f.onlyRun(t)
	pin, _ := admissionPin(t, f, run.RunID)
	v := f.latestVerification(t, run.RunID)
	att := decodeAttestation(t, v)
	candidate := f.candidateOf(t, run.RunID)
	if att.Candidate.Format != "sha1" || att.Candidate.Value != candidate.Commit {
		t.Errorf("attested candidate %+v, want sha1 %s", att.Candidate, candidate.Commit)
	}
	if att.AcceptanceContractDigest != pin.Contract || pin.Contract != "sha256:"+run.TaskSHA256 {
		t.Errorf("attested contract %q, pin %q, task sha256 %q", att.AcceptanceContractDigest, pin.Contract, run.TaskSHA256)
	}
	if att.CheckDefinition != "sha256:"+digest || att.CheckDefinition != pin.CheckDefinition {
		t.Errorf("attested check definition %q, want the config digest sha256:%s", att.CheckDefinition, digest)
	}
	if att.CheckVersion != "sha256:"+v.EvaluatorDigest || att.CheckVersion != pin.EvaluatorDigest {
		t.Errorf("attested check version %q, want the evaluator digest sha256:%s", att.CheckVersion, v.EvaluatorDigest)
	}
	if att.VerifierID != "host" {
		t.Errorf("verifier %q, want host", att.VerifierID)
	}
	if !strings.HasPrefix(att.Environment, "sha256:") || len(att.Environment) != len("sha256:")+64 {
		t.Errorf("environment %q is not a sha256 identity", att.Environment)
	}
	if want := map[string]string{"test": "passed"}; !maps.Equal(att.Results, want) {
		t.Errorf("results %v, want %v", att.Results, want)
	}
	if want := []string{"sha256:" + v.Checks[0].EvidenceSHA256}; v.Checks[0].EvidenceSHA256 == "" || !slices.Equal(att.Logs, want) {
		t.Errorf("logs %v, want %v", att.Logs, want)
	}
	if v.AttestationDigest != v2contract.Digest(att) {
		t.Errorf("stored digest %q, want %q", v.AttestationDigest, v2contract.Digest(att))
	}
	if payload := f.verificationPayload(t, run.RunID); payload["attestation_digest"] != v.AttestationDigest || payload["result"] != "passed" {
		t.Errorf("verification.completed = %v, want result passed and the attestation digest", payload)
	}
}

func TestUnavailableChecksPreserveUnverifiedCandidate(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	digest := f.commitChecks(t, missingCheck(t, "missing"))
	code, stdout, stderr := f.run(t, f.checkedRun(digest, "--format", "jsonl")...)
	if code != 5 {
		t.Fatalf("exit %d, stderr %q; want exit 5", code, stderr)
	}
	run := f.onlyRun(t)
	if run.State != "ready_for_review" || run.Reason != "unverified" {
		t.Fatalf("run projected %s/%s, want ready_for_review/unverified", run.State, run.Reason)
	}
	if c := f.candidateOf(t, run.RunID); c.Commit == "" {
		t.Error("the candidate row is empty")
	}
	v := f.latestVerification(t, run.RunID)
	if v.Result != "unavailable" || v.Reason != "checks_unavailable" || len(v.Checks) != 1 || v.Checks[0].Status != "unavailable" {
		t.Errorf("verification = %s/%s with checks %+v, want unavailable/checks_unavailable over one unavailable check", v.Result, v.Reason, v.Checks)
	}
	if p := f.verificationPayload(t, run.RunID); p["result"] != "unavailable" || p["reason"] != "checks_unavailable" {
		t.Errorf("verification.completed = %v, want unavailable/checks_unavailable", p)
	}
	if res := lastResult(t, stdout); res["error_category"] != "verification_unavailable" || res["state"] != "ready_for_review" {
		t.Errorf("run.result = %v, want ready_for_review with category verification_unavailable", res)
	}
}

func TestFailedBeatsUnavailable(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	digest := f.commitChecks(t, testCheck("fails", "fail"), missingCheck(t, "missing"))
	if code, _, stderr := f.run(t, f.checkedRun(digest)...); code != 5 {
		t.Fatalf("exit %d, stderr %q; want exit 5", code, stderr)
	}
	run := f.onlyRun(t)
	if run.State != "failed" || run.Reason != "verification_failed" {
		t.Fatalf("run projected %s/%s, want failed/verification_failed", run.State, run.Reason)
	}
	v := f.latestVerification(t, run.RunID)
	if v.Result != "failed" || v.Reason != "checks_failed" {
		t.Errorf("verification = %s/%s, want failed/checks_failed", v.Result, v.Reason)
	}
	statuses := make([]string, len(v.Checks))
	for i, c := range v.Checks {
		statuses[i] = c.Status
	}
	if !slices.Equal(statuses, []string{"failed", "unavailable"}) {
		t.Errorf("check statuses %v, want the failure and the unavailable check both recorded", statuses)
	}
}

func TestBindFailureJournalsUnavailable(t *testing.T) {
	f := newFixture(t)
	digest := f.config(t, "pass")
	hooks := supervisor.Hooks{
		Timeouts: &acceptance.Timeouts{Bind: 50 * time.Millisecond},
		BindStep: func(ctx context.Context, _ acceptance.Binding) error { <-ctx.Done(); return ctx.Err() },
	}
	out, err := f.runInProcess(t, digest, hooks)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out.State != supervisor.RunReadyForReview || out.Reason != "unverified" {
		t.Fatalf("outcome %s/%s, want ready_for_review/unverified", out.State, out.Reason)
	}
	v := f.latestVerification(t, out.RunID)
	if v.Result != "unavailable" || v.Reason != "binding_unavailable" || len(v.Attestation) != 0 || v.AttestationDigest != "" {
		t.Errorf("verification = %s/%s, attestation %q digest %q; want unavailable/binding_unavailable with none", v.Result, v.Reason, v.Attestation, v.AttestationDigest)
	}
	p := f.verificationPayload(t, out.RunID)
	if p["result"] != "unavailable" || p["reason"] != "binding_unavailable" {
		t.Errorf("verification.completed = %v, want unavailable/binding_unavailable", p)
	}
	if _, ok := p["attestation_digest"]; ok {
		t.Errorf("verification.completed carries an attestation digest: %v", p)
	}
}

func TestFailedCheckSurvivesBindFailure(t *testing.T) {
	for _, tc := range []struct {
		name       string
		mode       string
		wantRun    supervisor.RunState
		wantReason string
		wantResult string
		wantWhy    string
	}{
		{name: "a failed check stays failed", mode: "fail", wantRun: supervisor.RunFailed, wantReason: "verification_failed", wantResult: "failed", wantWhy: "checks_failed"},
		{name: "passed checks become unavailable", mode: "pass", wantRun: supervisor.RunReadyForReview, wantReason: "unverified", wantResult: "unavailable", wantWhy: "binding_unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			digest := f.config(t, tc.mode)
			out, err := f.runInProcess(t, digest, supervisor.Hooks{BindStep: func(context.Context, acceptance.Binding) error { return errors.New("bind refused") }})
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if out.State != tc.wantRun || out.Reason != tc.wantReason {
				t.Errorf("outcome %s/%s, want %s/%s", out.State, out.Reason, tc.wantRun, tc.wantReason)
			}
			v := f.latestVerification(t, out.RunID)
			if v.Result != tc.wantResult || v.Reason != tc.wantWhy || len(v.Attestation) != 0 {
				t.Errorf("verification = %s/%s, attestation %q; want %s/%s with no attestation", v.Result, v.Reason, v.Attestation, tc.wantResult, tc.wantWhy)
			}
		})
	}
}

// expiredLimit is a step limit that is already over when the step starts. A
// positive 1 ns limit is not: a coarse monotonic clock (Windows) reads the
// same instant at the deadline and at the check, so the context has not yet
// expired and the step succeeds. A negative limit puts the deadline strictly
// before now on every clock.
const expiredLimit = -time.Nanosecond

func TestRecordStepHonoursTimeout(t *testing.T) {
	f := newFixture(t)
	digest := f.config(t, "pass")
	out, err := f.runInProcess(t, digest, supervisor.Hooks{Timeouts: &acceptance.Timeouts{Record: expiredLimit}})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run error = %v, want context.DeadlineExceeded from the record step", err)
	}
	if out.State != supervisor.RunVerifying {
		t.Errorf("outcome state %s, want the run to stay verifying", out.State)
	}
	if run := f.onlyRun(t); run.State != "verifying" {
		t.Errorf("run projected %s, want verifying", run.State)
	}
	if _, err := f.journal(t).LatestVerification(t.Context(), out.RunID); !errors.Is(err, journal.ErrNotFound) {
		t.Errorf("LatestVerification = %v, want no row", err)
	}
}

func TestOverheadRecorded(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	digest := f.config(t, "pass")
	if code, _, stderr := f.run(t, f.checkedRun(digest)...); code != 0 {
		t.Fatalf("checked run exit %d: %s", code, stderr)
	}
	checked := f.onlyRun(t)
	if v := f.latestVerification(t, checked.RunID); v.OverheadMS == nil || *v.OverheadMS < 0 {
		t.Errorf("OverheadMS = %v for a run with bound checks, want a measured duration", v.OverheadMS)
	}
	g := newFixture(t)
	if code, _, stderr := g.run(t, g.fakeRun()...); code != 5 {
		t.Fatalf("waived run exit %d: %s", code, stderr)
	}
	if v := g.latestVerification(t, g.onlyRun(t).RunID); v.OverheadMS != nil {
		t.Errorf("OverheadMS = %d for a skip path, want unmeasured", *v.OverheadMS)
	}
}

func TestSkipPathsRecordReason(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		wantReason string
		args       func(t *testing.T, f *fixture) []string
	}{
		{name: "no-checks is waived", wantReason: "waived by --no-checks", args: func(_ *testing.T, f *fixture) []string { return f.fakeRun() }},
		{name: "inspect refuses the list", wantReason: "checks_refused_under_inspect", args: func(t *testing.T, f *fixture) []string {
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
			runID := f.onlyRun(t).RunID
			v := f.latestVerification(t, runID)
			if !v.Bound || v.Result != "not_run" || v.Reason != tc.wantReason || len(v.Attestation) != 0 || v.AttestationDigest != "" {
				t.Errorf("verification = bound %v %s/%q, attestation %q; want a bound not_run row with reason %q and no attestation", v.Bound, v.Result, v.Reason, v.Attestation, tc.wantReason)
			}
			if p := f.verificationPayload(t, runID); p["result"] != "NOT RUN" || p["reason"] != tc.wantReason {
				t.Errorf("verification.completed = %v, want NOT RUN with reason %q", p, tc.wantReason)
			}
		})
	}
}

func TestSameInputsSameVerdict(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	type judged struct {
		binding acceptance.Binding
		verdict acceptance.Result
	}
	seen := map[string]bool{}
	run := func(digest string) judged {
		t.Helper()
		if code, _, stderr := f.run(t, f.checkedRun(digest)...); code != 0 {
			t.Fatalf("run exit %d: %s", code, stderr)
		}
		var runID string
		for _, r := range f.runs(t) {
			if !seen[r.RunID] {
				runID, seen[r.RunID] = r.RunID, true
			}
		}
		v := f.latestVerification(t, runID)
		pin, _ := admissionPin(t, f, runID)
		statuses := make([]string, len(v.Checks))
		for i, c := range v.Checks {
			statuses[i] = c.Status
		}
		verdict, _ := acceptance.Decide(pin.EvaluatorDigest, "sha256:"+v.EvaluatorDigest, statuses)
		return judged{acceptance.BindingOf(decodeAttestation(t, v)), verdict}
	}
	passing := f.config(t, "pass")
	first, second := run(passing), run(passing)
	if first.binding.Candidate == second.binding.Candidate {
		t.Errorf("both runs froze candidate %v; the commit message carries the run ID, so they must differ", first.binding.Candidate)
	}
	first.binding.Candidate, second.binding.Candidate = v2contract.GitObject{}, v2contract.GitObject{}
	if first != second {
		t.Errorf("same repository, task and checks judged differently:\n first %+v\nsecond %+v", first, second)
	}
	changed := run(f.config(t, "output"))
	if changed.binding.CheckDefinition == first.binding.CheckDefinition || changed.binding.Evaluator == first.binding.Evaluator {
		t.Errorf("a changed check argv left the check definition %q or evaluator %q unchanged", changed.binding.CheckDefinition, changed.binding.Evaluator)
	}
}

// verificationVariant describes a verification appended after the run's own:
// its result, whether it has a binding row, and how its attestation differs
// from the original's.
type verificationVariant struct {
	result  string
	unbound bool
	attest  func(*v2contract.Verification)
}

// addVerification appends a newer verification to the run, as a copy of base
// with the variant's changes, the way a later or an older-schema recording
// would leave it. It returns the row.
func (f fixture) addVerification(t *testing.T, base journal.VerificationRow, v verificationVariant) journal.VerificationRow {
	t.Helper()
	latest := f.latestVerification(t, base.RunID)
	row := base
	row.ID = ids.New("ver")
	row.Result = v.result
	row.FinishedAt = latest.FinishedAt.Truncate(time.Second).Add(time.Second)
	if v.unbound {
		_, err := rawDB(t, f.state).ExecContext(t.Context(), `INSERT INTO verifications
			(verification_id, run_id, candidate_commit, config_sha256, result, started_at, finished_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, row.ID, row.RunID, row.CandidateCommit, row.ConfigSHA256, row.Result,
			row.StartedAt.UTC().Format(time.RFC3339Nano), row.FinishedAt.UTC().Format(time.RFC3339Nano))
		if err != nil {
			t.Fatal(err)
		}
		return row
	}
	att := decodeAttestation(t, base)
	att.VerificationID = row.ID
	if v.attest != nil {
		v.attest(&att)
	}
	encoded, err := json.Marshal(att)
	if err != nil {
		t.Fatal(err)
	}
	row.CandidateCommit = att.Candidate.Value
	row.Attestation, row.AttestationDigest = encoded, v2contract.Digest(att)
	insertVerification(t, f, row)
	return row
}

func otherDigest() string { return "sha256:" + strings.Repeat("0", 64) }

func TestAcceptanceStatePrecedence(t *testing.T) {
	t.Parallel()
	f, runID := readyApplyFixture(t)
	base := f.latestVerification(t, runID)
	ctx := t.Context()
	j := f.journal(t)
	otherCommit := f.git(t, "rev-parse", "HEAD")
	// Each case appends a newer verification, so the cases run in order
	// against one run and the last row decides.
	for _, tc := range []struct {
		name        string
		variant     verificationVariant
		want        supervisor.AcceptanceState
		wantChanged []string
	}{
		{name: "passed and matching is bound", variant: verificationVariant{result: "passed"}, want: supervisor.AcceptanceBound},
		{name: "failed is rejected", variant: verificationVariant{result: "failed"}, want: supervisor.AcceptanceRejected},
		{name: "invalid is rejected", variant: verificationVariant{result: "invalid"}, want: supervisor.AcceptanceRejected},
		{name: "not_run is unverified", variant: verificationVariant{result: "not_run"}, want: supervisor.AcceptanceUnverified},
		{name: "unavailable is unverified", variant: verificationVariant{result: "unavailable"}, want: supervisor.AcceptanceUnverified},
		{name: "an old not_run row without a binding is unverified, not unbound", variant: verificationVariant{result: "not_run", unbound: true},
			want: supervisor.AcceptanceUnverified},
		{name: "an old failed row without a binding is rejected, not unbound", variant: verificationVariant{result: "failed", unbound: true},
			want: supervisor.AcceptanceRejected},
		{name: "an old passed row without a binding is unbound", variant: verificationVariant{result: "passed", unbound: true},
			want: supervisor.AcceptanceUnbound},
		{name: "another contract is stale on contract", want: supervisor.AcceptanceStale, wantChanged: []string{"contract"},
			variant: verificationVariant{result: "passed", attest: func(a *v2contract.Verification) { a.AcceptanceContractDigest = otherDigest() }}},
		{name: "another check definition is stale on check_definition", want: supervisor.AcceptanceStale, wantChanged: []string{"check_definition"},
			variant: verificationVariant{result: "passed", attest: func(a *v2contract.Verification) { a.CheckDefinition = otherDigest() }}},
		{name: "another evaluator is stale on evaluator", want: supervisor.AcceptanceStale, wantChanged: []string{"evaluator"},
			variant: verificationVariant{result: "passed", attest: func(a *v2contract.Verification) { a.CheckVersion = otherDigest() }}},
		{name: "another candidate is stale on candidate", want: supervisor.AcceptanceStale, wantChanged: []string{"candidate"},
			variant: verificationVariant{result: "passed", attest: func(a *v2contract.Verification) { a.Candidate.Value = otherCommit }}},
		{name: "several changes are named in design order", want: supervisor.AcceptanceStale, wantChanged: []string{"candidate", "contract", "evaluator"},
			variant: verificationVariant{result: "passed", attest: func(a *v2contract.Verification) {
				a.Candidate.Value, a.AcceptanceContractDigest, a.CheckVersion = otherCommit, otherDigest(), otherDigest()
			}}},
		{name: "a changed environment alone stays bound (D4)", want: supervisor.AcceptanceBound,
			variant: verificationVariant{result: "passed", attest: func(a *v2contract.Verification) { a.Environment = otherDigest() }}},
	} {
		f.addVerification(t, base, tc.variant)
		got, err := supervisor.CheckAcceptance(ctx, j, runID)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got.State != tc.want || !slices.Equal(got.Changed, tc.wantChanged) {
			t.Errorf("%s: got %s changed %v, want %s changed %v", tc.name, got.State, got.Changed, tc.want, tc.wantChanged)
		}
	}
}

func TestAcceptanceUnknownResultIsError(t *testing.T) {
	t.Parallel()
	f, runID := readyApplyFixture(t)
	f.addVerification(t, f.latestVerification(t, runID), verificationVariant{result: "mystery"})
	got, err := supervisor.CheckAcceptance(t.Context(), f.journal(t), runID)
	if err == nil || got.State == supervisor.AcceptanceBound || !strings.Contains(err.Error(), "mystery") {
		t.Fatalf("unrecognised result gave %+v, %v; want an error naming it and never bound", got, err)
	}
}

func TestCheckAcceptanceErrorsNeverBound(t *testing.T) {
	t.Parallel()
	f, runID := readyApplyFixture(t)
	j := f.journal(t)
	base := f.latestVerification(t, runID)
	wantNoBound := func(leg string, got supervisor.AcceptanceStatus, err error) {
		t.Helper()
		if err == nil || got.State == supervisor.AcceptanceBound {
			t.Errorf("%s: got %+v, %v; want an error and never bound", leg, got, err)
		}
	}
	taskPath := filepath.Join(f.state, "runs", runID, "task.md")
	task, err := os.ReadFile(taskPath) // #nosec G304 -- the test's own state directory
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(taskPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(taskPath, 0o700); err != nil {
		t.Fatal(err)
	}
	got, err := supervisor.CheckAcceptance(t.Context(), j, runID)
	wantNoBound("task file unreadable but present", got, err)
	if err := os.Remove(taskPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(taskPath, task, 0o600); err != nil { // #nosec G703 -- the test's own state directory
		t.Fatal(err)
	}
	if got, err := supervisor.CheckAcceptance(t.Context(), j, runID); err != nil || got.State != supervisor.AcceptanceBound {
		t.Fatalf("restored run gave %+v, %v; want bound", got, err)
	}

	corrupt := base
	corrupt.ID, corrupt.FinishedAt = ids.New("ver"), base.FinishedAt.Truncate(time.Second).Add(time.Second)
	raw := rawDB(t, f.state)
	if _, err := raw.ExecContext(t.Context(), `INSERT INTO verifications
		(verification_id, run_id, candidate_commit, config_sha256, result, started_at, finished_at) VALUES (?, ?, ?, ?, 'passed', ?, ?)`,
		corrupt.ID, runID, base.CandidateCommit, base.ConfigSHA256, base.StartedAt.UTC().Format(time.RFC3339Nano), corrupt.FinishedAt.UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.ExecContext(t.Context(), `INSERT INTO verification_bindings (verification_id, reason, attestation, digest)
		VALUES (?, '', '{"verification_id":"x"}', 'sha256:00')`, corrupt.ID); err != nil {
		t.Fatal(err)
	}
	got, err = supervisor.CheckAcceptance(t.Context(), j, runID)
	wantNoBound("corrupt attestation", got, err)

	empty := f.createBareRun(t)
	got, err = supervisor.CheckAcceptance(t.Context(), j, empty)
	if err != nil || got.State != supervisor.AcceptanceUnverified || got.Reason != "no verification recorded" {
		t.Errorf("run with no verification gave %+v, %v; want unverified, reason %q", got, err, "no verification recorded")
	}
}

// createBareRun journals a run with no attempt or verification.
func (f fixture) createBareRun(t *testing.T) string {
	t.Helper()
	runID := ids.New("run")
	row := journal.RunRow{RunID: runID, AdapterID: "fake", SourceRepo: f.repo, SourceBranch: "main", BaseRev: strings.Repeat("a", 40),
		TaskSHA256: strings.Repeat("b", 64), BillingPosture: "local-scripted", ExecutionProfile: "trusted-host"}
	if err := supervisor.CreateRun(t.Context(), f.journal(t), row, supervisor.NewProducer(ids.New("sup"), 1)); err != nil {
		t.Fatal(err)
	}
	return runID
}

func TestCheckAcceptanceWithinExpires(t *testing.T) {
	t.Parallel()
	f, runID := readyApplyFixture(t)
	j := f.journal(t)
	if got, err := supervisor.CheckAcceptanceWithin(t.Context(), j, runID, time.Minute); err != nil || got.State != supervisor.AcceptanceBound {
		t.Fatalf("a generous limit gave %+v, %v; want bound", got, err)
	}
	got, err := supervisor.CheckAcceptanceWithin(t.Context(), j, runID, -time.Second)
	if !errors.Is(err, context.DeadlineExceeded) || got.State == supervisor.AcceptanceBound {
		t.Fatalf("an expired limit gave %+v, %v; want an error wrapping context.DeadlineExceeded and never bound", got, err)
	}
}
