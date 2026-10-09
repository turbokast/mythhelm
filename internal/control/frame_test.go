package control_test

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/turbokast/mythhelm/internal/control"
	"github.com/turbokast/mythhelm/internal/v2contract"
)

// artifactKey is the wire key whose string values count as artefact
// references; the value is pinned here independently of the implementation.
//
//nolint:misspell // "artifact_id" is the spec-mandated wire key (v2contract.Artifact)
const artifactKey = "artifact_id"

// prefixFrame prepends a correct 4-byte big-endian length prefix to payload,
// keeping the fixture bytes untouched.
func prefixFrame(payload []byte) []byte {
	frame := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint32(frame[:4], uint32(len(payload))) //nolint:gosec // G115: fixtures are at most MaxFrameBytes+1
	copy(frame[4:], payload)
	return frame
}

// requireMismatch fails unless err is a CheckIngress violation carrying the
// protocol_mismatch code the spec pins on every ingress rejection.
func requireMismatch(t *testing.T, err error, what string) {
	t.Helper()
	if err == nil {
		t.Fatalf("CheckIngress(%s) = nil, want protocol_mismatch", what)
	}
	if !strings.Contains(err.Error(), "protocol_mismatch") {
		t.Fatalf("CheckIngress(%s) error = %q, want protocol_mismatch code", what, err)
	}
}

// mustMarshal encodes v as JSON or fails the test.
func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	payload, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal frame payload: %v", err)
	}
	return payload
}

func TestFrameAtExactly1MiBPasses(t *testing.T) {
	pad := func(n int) []byte {
		return prefixFrame(mustMarshal(t, map[string]any{
			"operation_id": "op-1mib",
			"method":       "status",
			"params":       map[string]any{"pad": strings.Repeat("a", n)},
		}))
	}
	overhead := len(pad(0))
	exact := pad(v2contract.MaxFrameBytes - overhead)
	if len(exact) != v2contract.MaxFrameBytes {
		t.Fatalf("fixture length = %d, want exactly %d", len(exact), v2contract.MaxFrameBytes)
	}
	if err := control.CheckIngress(exact); err != nil {
		t.Fatalf("CheckIngress at exactly MaxFrameBytes: %v", err)
	}
	over := pad(v2contract.MaxFrameBytes - overhead + 1)
	overErr := control.CheckIngress(over)
	requireMismatch(t, overErr, "MaxFrameBytes+1")
	if !strings.Contains(overErr.Error(), "exceed limit") {
		t.Fatalf("CheckIngress at MaxFrameBytes+1 error = %q, want size-limit mention", overErr)
	}
}

func TestOversizeReportsSizeBeforeDecode(t *testing.T) {
	// Oversize and malformed: the size gate runs before any parsing, so
	// the violation reports the size error, never a decode error.
	payload := append([]byte(`{"operation_id":`), bytes.Repeat([]byte("9"), v2contract.MaxFrameBytes)...)
	err := control.CheckIngress(prefixFrame(payload))
	requireMismatch(t, err, "oversize malformed")
	if !strings.Contains(err.Error(), "exceed limit") {
		t.Fatalf("error = %q, want size error to precede decode", err)
	}
}

func TestDepth64Passes65Fails(t *testing.T) {
	cases := []struct {
		name    string
		file    string
		braces  int
		wantErr bool
	}{
		{"depth64passes", "testdata/depth64.json", 64, false},
		{"depth65fails", "testdata/depth65.json", 65, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload, err := os.ReadFile(tc.file)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			if got := strings.Count(string(payload), "{"); got != tc.braces {
				t.Fatalf("fixture has %d open braces, want %d", got, tc.braces)
			}
			err = control.CheckIngress(prefixFrame(payload))
			if !tc.wantErr && err != nil {
				t.Fatalf("CheckIngress = %v, want nil", err)
			}
			if tc.wantErr {
				requireMismatch(t, err, tc.name)
				if !strings.Contains(err.Error(), "depth") {
					t.Fatalf("error = %q, want depth-limit mention", err)
				}
			}
		})
	}
}

func TestRefs128Pass129Fail(t *testing.T) {
	refs := func(n int) []byte {
		items := make([]map[string]string, n)
		for i := range items {
			items[i] = map[string]string{artifactKey: "ref-" + strconv.Itoa(i)}
		}
		return prefixFrame(mustMarshal(t, map[string]any{
			"operation_id": "op-refs",
			"method":       "read",
			"params":       map[string]any{"refs": items},
		}))
	}
	if err := control.CheckIngress(refs(v2contract.MaxArtifactRefs)); err != nil {
		t.Fatalf("CheckIngress with 128 artefact refs: %v", err)
	}
	refsErr := control.CheckIngress(refs(v2contract.MaxArtifactRefs + 1))
	requireMismatch(t, refsErr, "129 artefact refs")
	if !strings.Contains(refsErr.Error(), "refs") {
		t.Fatalf("error = %q, want refs-limit mention", refsErr)
	}
}

func TestRefsCountOnlyStringValues(t *testing.T) {
	payload := mustMarshal(t, map[string]any{
		"operation_id": "op-refs-shapes",
		"method":       "read",
		"params": map[string]any{
			"refs": []any{
				map[string]any{artifactKey: "ref-0"},
				map[string]any{artifactKey: 7},
				map[string]any{artifactKey: nil},
				map[string]any{artifactKey: []string{"ref-1"}},
			},
		},
	})
	if err := control.CheckIngress(prefixFrame(payload)); err != nil {
		t.Fatalf("CheckIngress with one string artefact id among non-strings: %v", err)
	}
}

func TestRefsCountNestedObjects(t *testing.T) {
	items := make([]any, v2contract.MaxArtifactRefs-1)
	for i := range items {
		items[i] = map[string]any{artifactKey: "ref-" + strconv.Itoa(i)}
	}
	payload := mustMarshal(t, map[string]any{
		"operation_id": "op-refs-nested",
		"method":       "read",
		"params": map[string]any{
			"refs":  items,
			"extra": map[string]any{"meta": map[string]any{artifactKey: "nested-a", "other": map[string]any{artifactKey: "nested-b"}}},
		},
	})
	// 127 flat plus 2 nested = 129 references.
	nestedErr := control.CheckIngress(prefixFrame(payload))
	requireMismatch(t, nestedErr, "129 refs incl. nested")
	if !strings.Contains(nestedErr.Error(), "refs") {
		t.Fatalf("error = %q, want refs-limit mention", nestedErr)
	}
}

func TestCheckIngressSurvivesAdversarialNesting(t *testing.T) {
	const levels = 100000
	nested := strings.Repeat(`{"k":`, levels) + `"leaf"` + strings.Repeat(`}`, levels)
	// Built by concatenation: even encoding this payload exceeds the
	// stdlib depth cap. Rejection may surface at strict decode (stdlib
	// cap) or at the NFR-1 depth check; either way it must fail closed.
	payload := []byte(`{"operation_id":"op-deep","method":"depth-probe","params":` + nested + `}`)
	// Must return a depth error, never exhaust the stack.
	deepErr := control.CheckIngress(prefixFrame(payload))
	requireMismatch(t, deepErr, "100000 nesting")
	if !strings.Contains(deepErr.Error(), "depth") {
		t.Fatalf("error = %q, want depth-limit mention", deepErr)
	}
}

func TestUnknownKeyRejected(t *testing.T) {
	payload := []byte(`{"operation_id":"op-unknown","method":"status","bogus_key":1}`)
	err := control.CheckIngress(prefixFrame(payload))
	requireMismatch(t, err, "unknown key")
	if !strings.Contains(err.Error(), "bogus_key") {
		t.Fatalf("error = %q, want it to name bogus_key", err)
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	env := v2contract.RequestEnvelope{OperationID: "op-1", ExpectedRevision: 3}
	in := control.Frame{
		RequestEnvelope: env,
		Method:          "status",
		Params:          json.RawMessage(`{"a":[1,2]}`),
	}
	frame, err := control.Encode(in)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if got := binary.BigEndian.Uint32(frame[:4]); int(got) != len(frame)-4 {
		t.Fatalf("length prefix = %d, want payload length %d", got, len(frame)-4)
	}
	got, err := control.Decode[control.Frame](frame)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.OperationID != "op-1" || got.ExpectedRevision != 3 || got.Method != "status" {
		t.Fatalf("round trip envelope = %+v, want operation op-1 rev 3 method status", got)
	}
	if string(got.Params) != `{"a":[1,2]}` {
		t.Fatalf("round trip params = %s, want original bytes", got.Params)
	}
	if got.Generation != nil {
		t.Fatalf("round trip generation = %v, want absent (never zero-filled)", got.Generation)
	}
}

func TestDecodeRejectsBadPrefix(t *testing.T) {
	env := v2contract.RequestEnvelope{OperationID: "op-x"}
	valid, err := control.Encode(control.Frame{
		RequestEnvelope: env,
		Method:          "status",
	})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	mismatch := append([]byte(nil), valid...)
	mismatch[3]++ // perturb the low prefix byte so it no longer matches
	cases := map[string][]byte{
		"empty":           {},
		"shorter than 4":  {0x00, 0x00, 0x00},
		"length mismatch": mismatch,
		"truncated":       valid[:len(valid)-1],
	}
	for name, frame := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := control.Decode[control.Frame](frame); err == nil {
				t.Fatalf("Decode(%s) = nil, want prefix error", name)
			}
			requireMismatch(t, control.CheckIngress(frame), name+" prefix")
		})
	}
}

func TestCheckIngressRejectsMalformed(t *testing.T) {
	cases := map[string]string{
		"bad json":             "{oops",
		"trailing data":        `{"operation_id":"op-t"} trailing`,
		"non-object":           `[1,2]`,
		"missing operation_id": `{"method":"status"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			requireMismatch(t, control.CheckIngress(prefixFrame([]byte(body))), name)
		})
	}
}

func TestCheckIngressRejectsEmptyOperationID(t *testing.T) {
	err := control.CheckIngress(prefixFrame([]byte(`{"operation_id":"","method":"status"}`)))
	requireMismatch(t, err, "empty operation_id")
	if !strings.Contains(err.Error(), "operation_id") {
		t.Fatalf("error = %q, want it to name operation_id", err)
	}
}

func TestCheckIngressAcceptsMinimalFrame(t *testing.T) {
	if err := control.CheckIngress(prefixFrame([]byte(`{"operation_id":"op-min"}`))); err != nil {
		t.Fatalf("CheckIngress minimal frame: %v", err)
	}
}
