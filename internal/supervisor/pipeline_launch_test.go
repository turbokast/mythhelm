package supervisor

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/turbokast/mythhelm/adapters/claudecode"
	"github.com/turbokast/mythhelm/internal/adapter"
	"github.com/turbokast/mythhelm/internal/admission"
	"github.com/turbokast/mythhelm/internal/contain"
	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/journal"
	"github.com/turbokast/mythhelm/internal/workers"
)

// t9token is the fixed launch token every pin below builds against, so the
// only varying input is the decision (or the journaled rows).
const t9token = "tok_t9launch"

// t9marker is the hostile content every ignored Decision field is overwritten
// with: shell metacharacters, a newline and a flag-shaped line, so any leak
// into argv or env is shaped like a real attack.
const t9marker = "\"; rm -rf /\"\n--allowedTools=evil"

// t9consumed names the Decision fields launchForAttempt reads. t9ignored names
// every other field. TestLaunchDecisionFieldPin requires each Decision field
// in exactly one of the two, so a new field fails the pin until reviewed.
var t9consumed = map[string]bool{
	"TaskID": true, "Adapter": true, "Probe": true, "Proposal": true,
	"RunDir": true, "Workdir": true, "Profile": true,
}

var t9ignored = map[string]bool{
	"StateDir": true, "RunID": true, "AttemptID": true, "Host": true,
	"Scenario": true, "GitName": true, "GitEmail": true, "ProjectConfig": true,
	"ConfigDigest": true, "RepoIdentity": true, "RecordTrust": true,
	"NoChecks": true, "KeepGoing": true, "EnvelopeFlags": true,
	"Declaration": true, "RecordNativeTrust": true, "NativeConfigDigest": true,
	"NativeHooks": true, "NativeTrustGrant": true, "NativeAuth": true,
	"Task": true, "Snapshot": true,
}

// t9decision builds a fixed admitted contained decision on the fake route: a
// real run dir, workdir and home under a temp root, benign ignored fields
// (false/zero/nil), and an inventory digest pair the launch must map.
func t9decision(t *testing.T) admission.Decision {
	t.Helper()
	root := t.TempDir()
	runDir := filepath.Join(root, "runs", "run_t9")
	workDir := filepath.Join(runDir, "workspace")
	home := filepath.Join(root, "home")
	for _, dir := range []string{workDir, home} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	native := filepath.Join(root, "native")
	ev := contain.SeedV1()["restricted/"+runtime.GOOS+"/builtin/fake"]
	return admission.Decision{
		StateDir: root, RunID: "run_t9", TaskID: "task_t9", AttemptID: "att_t9",
		RunDir: runDir, Workdir: workDir,
		Host: "standalone", Scenario: "happy",
		GitName: "Benign", GitEmail: "benign@example.com",
		ConfigDigest: "cfg-benign", RepoIdentity: "repo-benign",
		NativeConfigDigest: "ncd-benign", NativeTrustGrant: "grant-benign",
		Adapter: adapter.Descriptor{ID: "builtin/fake", Version: "v-benign", Harness: "fake", Surface: "fixture"},
		Probe:   adapter.Probe{Executable: native, Version: "1", SHA256: strings.Repeat("b", 64), OS: runtime.GOOS, Arch: "amd64", Compatibility: "fixture"},
		Proposal: adapter.LaunchProposal{
			Spec: adapter.ProcSpec{
				Path: native,
				Args: []string{"native", "--benign"},
				Dir:  workDir,
				Env:  []string{"PATH=/usr/bin:/bin", "HOME=" + home},
			},
			StopLadder: []adapter.StopStep{{Signal: adapter.StopKill, Grace: 5 * time.Second}},
			Manifest:   adapter.ConfigManifest{Digests: map[string]string{"user": strings.Repeat("c", 64), "user_mcp": strings.Repeat("d", 64)}},
		},
		Profile: admission.Profile{Name: "restricted", Contained: true, Consent: "--execution-profile", Boundary: &ev},
	}
}

func t9launchJSON(t *testing.T, d admission.Decision) []byte {
	t.Helper()
	raw, err := json.Marshal(launchForAttempt(d, t9token))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestModelOutputNeverReachesLaunch(t *testing.T) {
	t.Parallel()
	d := t9decision(t)
	base := t9launchJSON(t, d)

	// Hostile model output is journaled for real: two attempt.native_result
	// rows carrying argv/env-shaped attacks against this very run. The helper
	// takes no journal, so its output cannot move.
	state := t.TempDir()
	j, err := journal.Open(t.Context(), state)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = j.Close() }()
	hostile := []string{
		`{"native_argv":["evil","--allowedTools=evil"],"result_text":"\"; rm -rf /\n--allowedTools=*"}`,
		`{"native_env":["ANTHROPIC_API_KEY=sk-hostile"],"result_text":"second hostile native result"}`,
	}
	producer := ids.New("wrk")
	for i, payload := range hostile {
		if err := j.Append(t.Context(), journal.Event{
			SchemaVersion: journal.EnvelopeVersion, EventID: ids.New("evt"), RunID: d.RunID,
			TaskID: d.TaskID, AttemptID: d.AttemptID, ProducerID: producer,
			ProducerSequence: int64(i + 1), Generation: 1,
			ObservedAt: time.Now().UTC(), Type: "attempt.native_result",
			Payload: json.RawMessage(payload),
		}, nil); err != nil {
			t.Fatal(err)
		}
	}
	if evs, err := j.Events(t.Context(), d.RunID, 0); err != nil || len(evs) != len(hostile) {
		t.Fatalf("journaled native results = %d, %v; want %d really present", len(evs), err, len(hostile))
	}
	if got := t9launchJSON(t, d); !bytes.Equal(got, base) {
		t.Fatalf("launch moved under hostile journaled native results:\nbase %s\ngot  %s", base, got)
	}

	// The signature itself is pinned: exactly (Decision, string) in and one
	// Launch out, so a future journal or store parameter breaks this test.
	ft := reflect.TypeOf(launchForAttempt)
	if ft.NumIn() != 2 || ft.In(0) != reflect.TypeOf(admission.Decision{}) || ft.In(1).Kind() != reflect.String ||
		ft.NumOut() != 1 || ft.Out(0) != reflect.TypeOf(workers.Launch{}) {
		t.Fatalf("launchForAttempt signature = %v; want func(admission.Decision, string) workers.Launch", ft)
	}
}

func TestLaunchDecisionFieldPin(t *testing.T) {
	t.Parallel()
	dt := reflect.TypeOf(admission.Decision{})
	seen := map[string]bool{}
	for i := range dt.NumField() {
		name := dt.Field(i).Name
		seen[name] = true
		inConsumed, inIgnored := t9consumed[name], t9ignored[name]
		if inConsumed == inIgnored {
			t.Errorf("Decision.%s is in consumed=%v ignored=%v; want exactly one (new field: review and classify)", name, inConsumed, inIgnored)
		}
	}
	for name := range t9consumed {
		if !seen[name] {
			t.Errorf("consumed literal names %q, which is not a Decision field", name)
		}
	}
	for name := range t9ignored {
		if !seen[name] {
			t.Errorf("ignored literal names %q, which is not a Decision field", name)
		}
	}
	if t.Failed() {
		t.FailNow()
	}

	d := t9decision(t)
	base := t9launchJSON(t, d)

	// Every ignored field, overwritten with hostile content, leaves the
	// launch byte-identical. The overwrite must actually change the field,
	// or the subtest would pass vacuously.
	for name := range t9ignored {
		m := d
		mv := reflect.ValueOf(&m).Elem().FieldByName(name)
		before := reflect.ValueOf(d).FieldByName(name).Interface()
		t9hostileFill(mv)
		after := mv.Interface()
		if reflect.DeepEqual(before, after) {
			t.Fatalf("hostile fill left Decision.%s unchanged; the pin would pass vacuously", name)
		}
		if got := t9launchJSON(t, m); !bytes.Equal(got, base) {
			t.Errorf("launch moved when ignored Decision.%s changed:\nbase %s\ngot  %s", name, base, got)
		}
	}

	// Every consumed field bites: changing what the helper reads moves the
	// launch, so the allowlist cannot silently over-claim.
	evilDir := t.TempDir()
	mutations := map[string]func(*admission.Decision){
		"TaskID":   func(m *admission.Decision) { m.TaskID = "task_evil" },
		"Adapter":  func(m *admission.Decision) { m.Adapter.ID = "builtin/evil" },
		"Probe":    func(m *admission.Decision) { m.Probe.SHA256 = strings.Repeat("e", 64) },
		"Proposal": func(m *admission.Decision) { m.Proposal.Spec.Args = []string{"evil"} },
		"RunDir":   func(m *admission.Decision) { m.RunDir = filepath.Join(evilDir, "runs", "run_evil") },
		"Workdir":  func(m *admission.Decision) { m.Workdir = evilDir },
		"Profile":  func(m *admission.Decision) { m.Profile.Contained = false },
	}
	if len(mutations) != len(t9consumed) {
		t.Fatalf("consumed mutations = %d, want one per consumed field (%d)", len(mutations), len(t9consumed))
	}
	for name, mutate := range mutations {
		if !t9consumed[name] {
			t.Fatalf("mutation names %q, which is not in the consumed literal", name)
		}
		m := d
		mutate(&m)
		if got := t9launchJSON(t, m); bytes.Equal(got, base) {
			t.Errorf("launch did not move when consumed Decision.%s changed; the pin over-claims", name)
		}
	}
}

// t9hostileFill overwrites every exported scalar reachable from the settable
// v with hostile content, allocating through pointers and slices. Unexported
// fields are skipped; kinds with no hostile form are left alone, and the
// caller fails when the fill changed nothing.
func t9hostileFill(v reflect.Value) {
	if !v.CanSet() {
		return
	}
	switch v.Kind() {
	case reflect.String:
		v.SetString(t9marker)
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(-42)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(42)
	case reflect.Float32, reflect.Float64:
		v.SetFloat(-42)
	case reflect.Slice:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			v.SetBytes([]byte(t9marker))
			return
		}
		if v.Len() == 0 {
			v.Set(reflect.MakeSlice(v.Type(), 1, 1))
		}
		for i := range v.Len() {
			t9hostileFill(v.Index(i))
		}
	case reflect.Ptr:
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		t9hostileFill(v.Elem())
	case reflect.Struct:
		for i := range v.NumField() {
			t9hostileFill(v.Field(i))
		}
	case reflect.Interface:
		if v.Type().NumMethod() == 0 {
			v.Set(reflect.ValueOf(t9marker))
		}
	}
}

func TestLaunchForAttemptHostileTaskBytes(t *testing.T) {
	t.Parallel()
	d := t9decision(t)
	hostile := []byte("do it\"; rm -rf /\nsecond line\n--allowedTools=*\n")
	d.Task = admission.Task{Content: hostile, SHA256: "evil", Title: "evil\"; rm -rf /"}
	l := launchForAttempt(d, t9token)
	// Task bytes travel on stdin only: no argv element may contain any task
	// line, and the child env is the admitted spec env verbatim.
	for _, line := range strings.Split(string(hostile), "\n") {
		if line == "" {
			continue
		}
		for _, arg := range l.Args {
			if strings.Contains(arg, line) {
				t.Fatalf("argv element %q contains task line %q", arg, line)
			}
		}
	}
	if !reflect.DeepEqual(l.Env, d.Proposal.Spec.Env) {
		t.Fatalf("launch env = %q, want the admitted spec env %q", l.Env, d.Proposal.Spec.Env)
	}
}

func TestLaunchForAttemptContained(t *testing.T) {
	t.Parallel()
	d := t9decision(t)
	l := launchForAttempt(d, t9token)
	if l.Containment == nil {
		t.Fatal("contained launch carries no policy")
	}
	wantWork, err := filepath.EvalSymlinks(d.Workdir)
	if err != nil {
		t.Fatal(err)
	}
	if l.Containment.Profile != "restricted" || l.Containment.Workdir != wantWork ||
		l.Containment.ReadOnly || l.Containment.ProxyAddr != "" || len(l.Containment.AuthBinds) != 0 {
		t.Fatalf("containment = %+v; want the restricted policy with an empty proxy address", l.Containment)
	}
	if len(l.ProxyAllow) != 0 {
		t.Fatalf("ProxyAllow = %q; want empty (deny all) until a first-party endpoint is admitted", l.ProxyAllow)
	}
	if l.PromptPath != filepath.Join(d.RunDir, "task.md") {
		t.Fatalf("PromptPath = %q", l.PromptPath)
	}

	d.Profile.Name = "inspect"
	if l := launchForAttempt(d, t9token); l.Containment == nil || !l.Containment.ReadOnly || l.Containment.Profile != "inspect" {
		t.Fatalf("inspect containment = %+v; want a read-only policy", l.Containment)
	}

	d.Profile.Contained = false
	if l := launchForAttempt(d, t9token); l.Containment != nil {
		t.Fatalf("trusted-host containment = %+v; want none", l.Containment)
	}
}

func TestLaunchForAttemptConfigPaths(t *testing.T) {
	t.Parallel()
	d := t9decision(t)
	home := ""
	for _, kv := range d.Proposal.Spec.Env {
		if value, ok := strings.CutPrefix(kv, "HOME="); ok {
			home = value
		}
	}
	l := launchForAttempt(d, t9token)
	if !reflect.DeepEqual(l.UserConfigDigests, d.Proposal.Manifest.Digests) {
		t.Fatalf("digests = %v, want the admitted manifest %v", l.UserConfigDigests, d.Proposal.Manifest.Digests)
	}
	want := claudecode.AdmittedConfigPathsForEnv(home, d.Workdir, d.Proposal.Spec.Env)
	if !reflect.DeepEqual(l.UserConfigPaths, want) {
		t.Fatalf("paths = %v, want %v", l.UserConfigPaths, want)
	}
	for key := range d.Proposal.Manifest.Digests {
		if _, ok := l.UserConfigPaths[key]; !ok {
			t.Fatalf("digest key %q has no path mapping; the worker would fail the launch", key)
		}
	}
	// The launch owns its digest copy: later decision mutation cannot move it.
	d.Proposal.Manifest.Digests["user"] = "mutated"
	if l.UserConfigDigests["user"] == "mutated" {
		t.Fatal("launch digests alias the decision manifest")
	}

	d.Proposal.Manifest = adapter.ConfigManifest{}
	if l := launchForAttempt(d, t9token); l.UserConfigDigests != nil || l.UserConfigPaths != nil {
		t.Fatalf("digestless launch carries %v / %v; want neither", l.UserConfigDigests, l.UserConfigPaths)
	}

	d = t9decision(t)
	d.Proposal.Spec.Env = []string{"PATH=/usr/bin:/bin"}
	if l := launchForAttempt(d, t9token); len(l.UserConfigPaths) != 0 {
		t.Fatalf("homeless launch maps %v; want no mapping so the worker fails closed", l.UserConfigPaths)
	}
}

func TestLaunchForAttemptBadContainmentFailsClosed(t *testing.T) {
	t.Parallel()
	d := t9decision(t)
	d.Workdir = "relative/workspace"
	l := launchForAttempt(d, t9token)
	// A contained launch that cannot build its policy must never run
	// uncontained: the zero policy is an invalid launch the worker refuses
	// with exit 2, and it must survive the JSON trip to the worker intact.
	if l.Containment == nil || l.Containment.Workdir != "" {
		t.Fatalf("containment = %+v; want a zero policy the worker refuses", l.Containment)
	}
	raw, err := json.Marshal(l)
	if err != nil {
		t.Fatal(err)
	}
	var back workers.Launch
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if back.Containment == nil || back.Containment.Workdir != "" {
		t.Fatalf("round-tripped containment = %+v; want the zero policy intact", back.Containment)
	}
}
