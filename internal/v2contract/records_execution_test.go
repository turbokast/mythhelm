package v2contract_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/turbokast/mythhelm/internal/ids"
	"github.com/turbokast/mythhelm/internal/v2contract"
)

func roundTrip[T v2contract.Validator](t *testing.T, name string) {
	t.Helper()
	golden := loadGolden(t, "records/"+name+".golden.json")
	v, err := v2contract.Decode[T](golden)
	if err != nil {
		t.Fatalf("%s: Decode: %v", name, err)
	}
	out, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("%s: Marshal: %v", name, err)
	}
	if !bytes.Equal(out, bytes.TrimSpace(golden)) {
		t.Errorf("%s: re-encoded\n%s\nwant\n%s", name, out, golden)
	}
	if !bytes.Contains(golden, []byte(`"schema_version":2`)) {
		t.Errorf("%s: golden lacks schema_version 2", name)
	}
	old := bytes.Replace(golden, []byte(`"schema_version":2`), []byte(`"schema_version":1`), 1)
	if _, err := v2contract.Decode[T](old); err == nil || !strings.Contains(err.Error(), "schema_version") {
		t.Errorf("%s: schema_version 1 decode error = %v, want naming schema_version", name, err)
	}
}

func TestExecutionGoldensRoundTrip(t *testing.T) {
	t.Parallel()
	t.Run("run", func(t *testing.T) { roundTrip[v2contract.Run](t, "run") })
	t.Run("task_revision", func(t *testing.T) { roundTrip[v2contract.TaskRevision](t, "task_revision") })
	t.Run("attempt", func(t *testing.T) { roundTrip[v2contract.Attempt](t, "attempt") })
}

func validTaskRevision() v2contract.TaskRevision {
	return v2contract.TaskRevision{
		SchemaVersion: v2contract.SchemaVersion, TaskID: ids.New("task"), Revision: 1, State: "pending",
	}
}

// I20 (v2 §2): revisions are required, never defaulted.
func TestTaskRevisionRejectsZeroRevision(t *testing.T) {
	t.Parallel()
	tr := validTaskRevision()
	if err := tr.Validate(); err != nil {
		t.Fatalf("valid revision: %v", err)
	}
	tr.Revision = 0
	if err := tr.Validate(); err == nil || !strings.Contains(err.Error(), "revision") {
		t.Errorf("Revision 0 error = %v, want naming revision", err)
	}
}

func TestTaskRevisionValidatesDependencies(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		dep     v2contract.RevisionRef
		wantErr string
	}{
		{"valid", v2contract.RevisionRef{Kind: "contract", ID: "c1", Revision: 1}, ""},
		{"bad kind", v2contract.RevisionRef{Kind: "other", ID: "c1", Revision: 1}, "dependencies[0]"},
		{"zero revision", v2contract.RevisionRef{Kind: "contract", ID: "c1", Revision: 0}, "dependencies[0]"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tr := validTaskRevision()
			tr.Dependencies = []v2contract.RevisionRef{tc.dep}
			err := tr.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("Validate error = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

// I20 (v2 §2): a new revision is a new digest; identical content is equal.
func TestRevisionDigestsDiffer(t *testing.T) {
	t.Parallel()
	a := validTaskRevision()
	b := a
	b.Revision = 2
	c := a
	if v2contract.Digest(a) == "" || v2contract.Digest(a) == v2contract.Digest(b) {
		t.Errorf("revisions 1 and 2 digests: %q vs %q, want distinct non-empty", v2contract.Digest(a), v2contract.Digest(b))
	}
	if v2contract.Digest(a) != v2contract.Digest(c) {
		t.Errorf("identical revisions digests differ")
	}
}

func TestRecordTagsAreSnakeCase(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		v            any
		want, reject string
	}{
		{"run", v2contract.Run{}, `"run_id"`, `"RunID"`},
		{"task", v2contract.TaskRevision{}, `"task_id"`, `"TaskID"`},
		{"attempt", v2contract.Attempt{}, `"attempt_id"`, `"AttemptID"`},
	}
	for _, tc := range tests {
		b, err := json.Marshal(tc.v)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if !bytes.Contains(b, []byte(tc.want)) || bytes.Contains(b, []byte(tc.reject)) {
			t.Errorf("%s: %s, want %s and not %s", tc.name, b, tc.want, tc.reject)
		}
	}
}

// I09 (v2 §2): a missing ID is rejected, never defaulted.
func TestMissingIDsRejected(t *testing.T) {
	t.Parallel()
	run := v2contract.Run{SchemaVersion: v2contract.SchemaVersion, State: "created"}
	task := validTaskRevision()
	task.TaskID = ""
	att := v2contract.Attempt{SchemaVersion: v2contract.SchemaVersion, TaskID: "t", State: "reserved"}
	att2 := v2contract.Attempt{SchemaVersion: v2contract.SchemaVersion, AttemptID: "a", State: "reserved"}
	att3 := v2contract.Attempt{SchemaVersion: v2contract.SchemaVersion, AttemptID: "a", TaskID: "t", State: "reserved"}
	tests := []struct {
		name    string
		err     error
		wantErr string
	}{
		{"run_id", run.Validate(), "run_id"},
		{"task_id", task.Validate(), "task_id"},
		{"attempt_id", att.Validate(), "attempt_id"},
		{"attempt task_id", att2.Validate(), "task_id"},
		{"attempt task_revision", att3.Validate(), "task_revision"},
	}
	for _, tc := range tests {
		if tc.err == nil || !strings.Contains(tc.err.Error(), tc.wantErr) {
			t.Errorf("%s: error = %v, want containing %q", tc.name, tc.err, tc.wantErr)
		}
	}
}

// An omitted task_revision decodes to 0 and must not yield an accepted Attempt.
func TestAttemptRejectsOmittedTaskRevision(t *testing.T) {
	t.Parallel()
	in := `{"schema_version":2,"attempt_id":"a1","task_id":"t1","lifecycle":"reserved"}`
	if _, err := v2contract.Decode[v2contract.Attempt]([]byte(in)); err == nil || !strings.Contains(err.Error(), "task_revision") {
		t.Errorf("Decode(omitted task_revision) error = %v, want naming task_revision", err)
	}
}
