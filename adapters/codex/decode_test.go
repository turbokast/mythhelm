package codex

import (
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/turbokast/mythhelm/internal/adapter"
	"github.com/turbokast/mythhelm/internal/adapter/ndjson"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "streams", name+".jsonl")) // #nosec G304 -- Test reads only its own fixture by a fixed name.
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func collect(table map[string]string, lim ndjson.Limits, r io.Reader) ([]adapter.Observation, adapter.NativeExit) {
	var out []adapter.Observation
	exit := decodeWith(table, lim, r, func(o adapter.Observation) { out = append(out, o) })
	return out, exit
}

func counters(t *testing.T, obs []adapter.Observation) adapter.ProtocolCounters {
	t.Helper()
	if len(obs) == 0 {
		t.Fatal("no observations")
	}
	pc, ok := obs[len(obs)-1].(adapter.ProtocolCounters)
	if !ok {
		t.Fatalf("last observation = %#v, want ProtocolCounters", obs[len(obs)-1])
	}
	return pc
}

func withoutCounters(obs []adapter.Observation) []adapter.Observation {
	return obs[:len(obs)-1]
}

func TestDecodeSuccessFixture(t *testing.T) { // AC-4.4, AC-6.1, D12
	t.Parallel()
	obs, exit := collect(nil, ndjson.Limits{}, strings.NewReader(fixture(t, "success")))
	if exit.Err != nil {
		t.Fatalf("exit.Err = %v", exit.Err)
	}
	want := []adapter.Observation{
		adapter.SessionStarted{SessionID: "synth-codex-1"},
		adapter.Progress{Turn: 1},
		adapter.Progress{Turn: 1, Tool: "shell"},
		adapter.Progress{Turn: 2},
		adapter.Result{Subtype: "success", StopReason: "end_turn"},
	}
	if got := withoutCounters(obs); !reflect.DeepEqual(got, want) {
		t.Fatalf("observations = %#v\nwant %#v", got, want)
	}
	res := obs[len(obs)-2].(adapter.Result)
	if res.Tokens != nil {
		t.Fatalf("Result.Tokens = %#v, want nil (unknown, not zero)", res.Tokens)
	}
	if got := obs[0].(adapter.SessionStarted).APIKeySource; got != "" {
		t.Fatalf("APIKeySource = %q, want empty (D12)", got)
	}
	if pc := counters(t, obs); pc.Frames != 5 || pc.Malformed != 0 || len(pc.UnknownTypes) != 0 {
		t.Fatalf("counters = %#v, want 5 frames and nothing dropped", pc)
	}
}

func TestDecodeMissingSessionIDIsUnknown(t *testing.T) { // AC-6.1
	t.Parallel()
	tests := map[string]string{
		"no session frame":      `{"_synthetic":true,"type":"turn"}` + "\n" + `{"_synthetic":true,"type":"result","subtype":"success"}` + "\n",
		"session without an id": `{"_synthetic":true,"type":"session"}` + "\n" + `{"_synthetic":true,"type":"result","subtype":"success"}` + "\n",
		"session with a null id": `{"_synthetic":true,"type":"session","session_id":null}` + "\n" +
			`{"_synthetic":true,"type":"result","subtype":"success"}` + "\n",
	}
	for name, in := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			obs, _ := collect(nil, ndjson.Limits{}, strings.NewReader(in))
			started, ok := obs[0].(adapter.SessionStarted)
			if !ok {
				t.Fatalf("first observation = %#v, want SessionStarted", obs[0])
			}
			if started.SessionID != "" {
				t.Fatalf("SessionID = %q, want empty", started.SessionID)
			}
		})
	}
}

func TestDecodeErrorsAreNativeError(t *testing.T) { // AC-4.6, D7, D14
	t.Parallel()
	classes := func(obs []adapter.Observation) []string {
		var got []string
		for _, o := range obs {
			if e, ok := o.(adapter.NativeError); ok {
				got = append(got, e.Class)
			}
		}
		return got
	}
	t.Run("the default table maps every error to native_error", func(t *testing.T) {
		t.Parallel()
		obs, _ := collect(nil, ndjson.Limits{}, strings.NewReader(fixture(t, "error")))
		want := []string{"native_error", "native_error", "native_error"}
		if got := classes(obs); !reflect.DeepEqual(got, want) {
			t.Fatalf("classes = %v, want %v", got, want)
		}
	})
	t.Run("a populated table maps throttle and exhaustion separately", func(t *testing.T) {
		t.Parallel()
		table := map[string]string{"synthetic_throttle": "rate_limit", "synthetic_exhaustion": "allowance_exhausted"}
		obs, _ := collect(table, ndjson.Limits{}, strings.NewReader(fixture(t, "error")))
		want := []string{"native_error", "rate_limit", "allowance_exhausted"}
		if got := classes(obs); !reflect.DeepEqual(got, want) {
			t.Fatalf("classes = %v, want %v (a throttle must never read as exhaustion)", got, want)
		}
	})
	t.Run("a table value outside the known classes is native_error", func(t *testing.T) {
		t.Parallel()
		obs, _ := collect(map[string]string{"synthetic_unmapped": "free_text_from_vendor"}, ndjson.Limits{}, strings.NewReader(fixture(t, "error")))
		if got := classes(obs)[0]; got != "native_error" {
			t.Fatalf("class = %q, want native_error", got)
		}
	})
	t.Run("an error frame still ends in the result", func(t *testing.T) {
		t.Parallel()
		obs, exit := collect(nil, ndjson.Limits{}, strings.NewReader(fixture(t, "error")))
		res, ok := obs[len(obs)-2].(adapter.Result)
		if !ok || !res.IsError || res.Subtype != "error" || exit.Err != nil {
			t.Fatalf("result = %#v, exit.Err = %v", obs[len(obs)-2], exit.Err)
		}
	})
}

type countingReader struct {
	r io.Reader
}

func (c countingReader) Read(p []byte) (int, error) { return c.r.Read(p) }

func TestDecodeCountsUnknownAndIncomplete(t *testing.T) { // AC-4.6, design §5
	t.Parallel()
	t.Run("unknown types are counted by name", func(t *testing.T) {
		t.Parallel()
		obs, exit := collect(nil, ndjson.Limits{}, strings.NewReader(fixture(t, "unknown_type")))
		want := map[string]int64{"vendor.codex.synthetic_future_frame": 2, "vendor.codex.synthetic_other_frame": 1}
		if pc := counters(t, obs); !reflect.DeepEqual(pc.UnknownTypes, want) || exit.Err != nil {
			t.Fatalf("UnknownTypes = %v, exit.Err = %v, want %v", pc.UnknownTypes, exit.Err, want)
		}
	})
	t.Run("an oversize line is counted and never an error", func(t *testing.T) {
		t.Parallel()
		in := `{"_synthetic":true,"type":"session","session_id":"s"}` + "\n" +
			`{"_synthetic":true,"type":"turn","pad":"` + strings.Repeat("x", 300) + `"}` + "\n" +
			`{"_synthetic":true,"type":"result","subtype":"success"}` + "\n"
		obs, exit := collect(nil, ndjson.Limits{MaxFrame: 200}, strings.NewReader(in))
		pc := counters(t, obs)
		if pc.Oversized != 1 || pc.Frames != 2 || exit.Err != nil {
			t.Fatalf("Oversized = %d, Frames = %d, exit.Err = %v; want 1, 2, nil", pc.Oversized, pc.Frames, exit.Err)
		}
	})
	t.Run("a stream without a result frame is incomplete", func(t *testing.T) {
		t.Parallel()
		obs, exit := collect(nil, ndjson.Limits{}, strings.NewReader(fixture(t, "truncated")))
		if exit.Err == nil {
			t.Fatal("exit.Err = nil, want an incomplete-stream error")
		}
		for _, o := range obs {
			if _, ok := o.(adapter.Result); ok {
				t.Fatalf("observed a Result for a truncated stream: %#v", o)
			}
		}
		counters(t, obs)
	})
	t.Run("an unterminated final result frame is delivered", func(t *testing.T) {
		t.Parallel()
		in := `{"_synthetic":true,"type":"session","session_id":"s"}` + "\n" +
			`{"_synthetic":true,"type":"result","subtype":"success","stop_reason":"end_turn"}`
		obs, exit := collect(nil, ndjson.Limits{}, strings.NewReader(in))
		want := adapter.Result{Subtype: "success", StopReason: "end_turn"}
		if exit.Err != nil || !reflect.DeepEqual(obs[len(obs)-2], want) {
			t.Fatalf("obs = %#v, exit.Err = %v; want %#v and no error", obs, exit.Err, want)
		}
	})
	t.Run("malformed frames are counted and do not stop the stream", func(t *testing.T) {
		t.Parallel()
		in := "not json\n" + `{"_synthetic":true,"type":"result","subtype":"success"}` + "\n"
		obs, exit := collect(nil, ndjson.Limits{}, strings.NewReader(in))
		if pc := counters(t, obs); pc.Malformed != 1 || exit.Err != nil {
			t.Fatalf("Malformed = %d, exit.Err = %v; want 1, nil", pc.Malformed, exit.Err)
		}
	})
	t.Run("only the shared ndjson reader produces the counters", func(t *testing.T) {
		t.Parallel()
		obs, _ := collect(nil, ndjson.Limits{}, countingReader{strings.NewReader(fixture(t, "success"))})
		if pc := counters(t, obs); pc.Frames == 0 {
			t.Fatal("Counters.Frames = 0: the decoder bypassed internal/adapter/ndjson")
		}
	})
	t.Run("Decode applies the default table", func(t *testing.T) {
		t.Parallel()
		var got []adapter.Observation
		exit := Decode(strings.NewReader(fixture(t, "success")), func(o adapter.Observation) { got = append(got, o) })
		if exit.Err != nil || len(got) != 6 {
			t.Fatalf("exit.Err = %v, observations = %d, want nil and 6", exit.Err, len(got))
		}
	})
}

var credentialShapes = regexp.MustCompile(
	`sk-[A-Za-z0-9_-]{16,}|gh[pousr]_[A-Za-z0-9]{20,}|AKIA[0-9A-Z]{16}|eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}|-----BEGIN [A-Z ]*PRIVATE KEY|(?i:bearer\s+[a-z0-9._-]{16,})|(?i:"(?:api_?key|token|secret|password|authorization)"\s*:\s*"[^"]+")`)

// credentialHits lists the files under dir that carry a credential-shaped string.
func credentialHits(t *testing.T, dir string) []string {
	t.Helper()
	var hits []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		raw, err := os.ReadFile(path) // #nosec G304 G122 -- Test walks its own fixture tree.
		if err != nil {
			return err
		}
		if credentialShapes.Match(raw) {
			hits = append(hits, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return hits
}

func TestFixturesCarryNoCredentialStrings(t *testing.T) { // NFR-2
	t.Parallel()
	if hits := credentialHits(t, "testdata"); len(hits) != 0 {
		t.Fatalf("credential-shaped strings in %v", hits)
	}
	copyDir := t.TempDir()
	raw := fixture(t, "success")
	if err := os.WriteFile(filepath.Join(copyDir, "planted.jsonl"), []byte(raw+`{"type":"session","token":"abc123"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if hits := credentialHits(t, copyDir); len(hits) != 1 {
		t.Fatalf("scan missed a planted credential field: hits = %v", hits)
	}
	for _, planted := range []string{"sk-" + strings.Repeat("a", 24), "Bearer " + strings.Repeat("b", 24)} {
		if err := os.WriteFile(filepath.Join(copyDir, "planted.jsonl"), []byte(planted), 0o600); err != nil {
			t.Fatal(err)
		}
		if len(credentialHits(t, copyDir)) != 1 {
			t.Fatalf("scan missed planted %q", planted[:6])
		}
	}
}

func TestFixturesAreMarkedSynthetic(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob(filepath.Join("testdata", "streams", "*.jsonl"))
	if err != nil || len(files) != 4 {
		t.Fatalf("fixtures = %v, err = %v, want 4", files, err)
	}
	for _, f := range files {
		raw, _ := os.ReadFile(f) // #nosec G304 -- Globbed from the package's own testdata.
		for line := range strings.SplitSeq(strings.TrimSpace(string(raw)), "\n") {
			if !strings.Contains(line, `"_synthetic":true`) {
				t.Errorf("%s: frame not marked synthetic: %s", f, line)
			}
		}
	}
}
