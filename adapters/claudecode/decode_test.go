package claudecode

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/turbokast/mythhelm/internal/adapter"
)

// loadFixture returns a fixture's frames. The '# synthetic' marker line and
// blank lines are test-loader conventions; the production decoder never sees
// them, and a '#' line from the native would be malformed JSON.
func loadFixture(t *testing.T, name string) [][]byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "streams", name+".jsonl")) // #nosec G304 -- Test reads only its own fixture by the subtest's fixed name.
	if err != nil {
		t.Fatal(err)
	}
	var frames [][]byte
	for _, line := range strings.Split(string(raw), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		frames = append(frames, []byte(line))
	}
	if len(frames) == 0 {
		t.Fatalf("fixture %s has no frames", name)
	}
	return frames
}

func decodeAll(frames [][]byte) []adapter.Observation {
	d := newStreamDecoder()
	var out []adapter.Observation
	for _, f := range frames {
		out = append(out, d.decode(f)...)
	}
	return out
}

func i64(n int64) *int64 { return &n }

func TestDecodeFixture(t *testing.T) {
	t.Parallel()
	t.Run("success", func(t *testing.T) {
		t.Parallel()
		obs := decodeAll(loadFixture(t, "success"))
		numTurns, durationMS := 2, int64(1500)
		want := []adapter.Observation{
			adapter.SessionStarted{SessionID: "synth-success", Model: "claude-synthetic", NativeVersion: "2.1.284", PermissionMode: "acceptEdits", APIKeySource: "none", ToolCount: 3,
				MCPServers: []adapter.MCPServer{{Name: "synth-mcp", Status: "connected"}, {Name: "synth-off", Status: "failed"}}, PluginCount: 1},
			adapter.Progress{Turn: 1},
			adapter.Progress{Turn: 1, Tool: "Bash"},
			adapter.Progress{Turn: 2},
			adapter.Progress{Turn: 2, Tool: "Read"},
			adapter.Progress{Turn: 2, Tool: "invalid"}, // over-long tool names never reach the journal verbatim
			adapter.Result{Subtype: "success", NumTurns: &numTurns, DurationMS: &durationMS, StopReason: "end_turn",
				Tokens:  map[string]adapter.TokenUsage{"claude-synthetic": {Input: i64(100), Output: i64(50), CacheRead: i64(10), CacheCreation: i64(5)}},
				CostUSD: "0.0123", PermissionDenials: []string{}},
		}
		if !reflect.DeepEqual(obs, want) {
			t.Fatalf("success observations:\n got %+v\nwant %+v", obs, want)
		}
	})
	t.Run("error_max_turns", func(t *testing.T) {
		t.Parallel()
		obs := decodeAll(loadFixture(t, "error_max_turns"))
		res, ok := obs[len(obs)-1].(adapter.Result)
		if !ok {
			t.Fatalf("last observation is %T, want Result", obs[len(obs)-1])
		}
		if res.Subtype != "error_max_turns" || !res.IsError || res.ErrorCount != 2 || res.CostUSD != "1.07" {
			t.Fatalf("error result = %+v", res)
		}
		if _, ok := obs[0].(adapter.SessionStarted); !ok {
			t.Fatalf("first observation is %T, want SessionStarted", obs[0])
		}
	})
	t.Run("permission_denials", func(t *testing.T) {
		t.Parallel()
		obs := decodeAll(loadFixture(t, "permission_denials"))
		var denied []adapter.PermissionDenied
		var res adapter.Result
		for _, ob := range obs {
			switch o := ob.(type) {
			case adapter.PermissionDenied:
				denied = append(denied, o)
			case adapter.Result:
				res = o
			}
		}
		if !reflect.DeepEqual(denied, []adapter.PermissionDenied{{ToolName: "WebFetch"}}) {
			t.Fatalf("denied = %+v", denied)
		}
		if !reflect.DeepEqual(res.PermissionDenials, []string{"Bash", "WebFetch"}) {
			t.Fatalf("result denials = %+v", res.PermissionDenials)
		}
	})
	t.Run("startup_failure", func(t *testing.T) {
		t.Parallel()
		obs := decodeAll(loadFixture(t, "startup_failure"))
		if len(obs) != 1 {
			t.Fatalf("startup failure must yield exactly its result, got %d observations", len(obs))
		}
		res, ok := obs[0].(adapter.Result)
		if !ok || res.Subtype != "error_startup" || !res.IsError || res.StartupFailureReason != "config_error" {
			t.Fatalf("startup result = %+v", obs[0])
		}
	})
	t.Run("api_retry", func(t *testing.T) {
		t.Parallel()
		obs := decodeAll(loadFixture(t, "api_retry"))
		var retries []adapter.Retry
		for _, ob := range obs {
			if r, ok := ob.(adapter.Retry); ok {
				retries = append(retries, r)
			}
		}
		if !reflect.DeepEqual(retries, []adapter.Retry{{Attempt: 1, RetryDelayMS: 500, ErrorStatus: 429}}) {
			t.Fatalf("retries = %+v", retries)
		}
	})
	t.Run("hooks_before_init", func(t *testing.T) {
		t.Parallel()
		obs := decodeAll(loadFixture(t, "hooks_before_init"))
		if _, ok := obs[0].(adapter.SessionStarted); !ok {
			t.Fatalf("hook frames are counted only; first observation is %T", obs[0])
		}
		if res, ok := obs[len(obs)-1].(adapter.Result); !ok || res.Subtype != "success" {
			t.Fatalf("last observation = %+v", obs[len(obs)-1])
		}
		for _, ob := range obs {
			if err, ok := ob.(adapter.NativeError); ok {
				t.Fatalf("pre-init hooks must not be a protocol error, got %+v", err)
			}
		}
	})
}

func TestAssistantBeforeInitIsProtocolError(t *testing.T) {
	t.Parallel()
	d := newStreamDecoder()
	obs := d.decode([]byte(`{"type":"assistant","message":{"content":[]}}`))
	if len(obs) != 1 || obs[0] != (adapter.NativeError{Class: "protocol_error"}) {
		t.Fatalf("assistant before init = %+v, want one protocol_error", obs)
	}
	// The violation is reported once; the stream stays decodable.
	obs = d.decode([]byte(`{"type":"assistant","message":{"content":[]}}`))
	if len(obs) != 0 {
		t.Fatalf("second violation must not repeat the error, got %+v", obs)
	}
}

func TestDecodeInitRequiresSessionAndSource(t *testing.T) {
	t.Parallel()
	for name, frame := range map[string]string{
		"missing session": `{"type":"system","subtype":"init","apiKeySource":"none"}`,
		"missing source":  `{"type":"system","subtype":"init","session_id":"s"}`,
		"empty source":    `{"type":"system","subtype":"init","session_id":"s","apiKeySource":""}`,
		"aliased source":  `{"type":"system","subtype":"init","session_id":"s","apiKeySource":"none","api_key_source":"other"}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			obs := decodeAll([][]byte{[]byte(frame)})
			if len(obs) != 1 || obs[0] != (adapter.NativeError{Class: "protocol_error"}) {
				t.Fatalf("%s = %+v, want one protocol_error", name, obs)
			}
		})
	}
	// A non-"none" source is reported, not a protocol error: the worker owns
	// the AC-4.4 stop.
	obs := decodeAll([][]byte{[]byte(`{"type":"system","subtype":"init","session_id":"s","apiKeySource":"ANTHROPIC_API_KEY"}`)})
	started, ok := obs[0].(adapter.SessionStarted)
	if !ok || started.APIKeySource != "ANTHROPIC_API_KEY" {
		t.Fatalf("mismatched source = %+v", obs)
	}
}

func TestDecodeResultRequiresSubtype(t *testing.T) {
	t.Parallel()
	init := []byte(`{"type":"system","subtype":"init","session_id":"s","apiKeySource":"none"}`)
	// The init frame goes first: without it the success-without-init rule
	// would answer instead of the rule under test.
	for _, frame := range []string{
		`{"type":"result","is_error":false}`,
		`{"type":"result","subtype":"","is_error":false}`,
		`{"type":"result","subtype":7,"is_error":false}`,
		`{"type":"result","subtype":"success","is_error":"no"}`,
		`{"type":"result","subtype":"success","total_cost_usd":"twelve cents"}`,
		`{"type":"result","subtype":"success","total_cost_usd":[1]}`,
	} {
		obs := decodeAll([][]byte{init, []byte(frame)})
		if len(obs) != 2 || obs[1] != (adapter.NativeError{Class: "protocol_error"}) {
			t.Fatalf("%s = %+v, want session then protocol_error", frame, obs)
		}
	}
	// A malformed result still poisons the attempt when a valid session ran.
	obs := decodeAll([][]byte{
		[]byte(`{"type":"system","subtype":"init","session_id":"s","apiKeySource":"none"}`),
		[]byte(`{"type":"result"}`),
	})
	if len(obs) != 2 || obs[1] != (adapter.NativeError{Class: "protocol_error"}) {
		t.Fatalf("malformed result after init = %+v", obs)
	}
}

func TestDecodeErrorClassMapping(t *testing.T) {
	t.Parallel()
	init := []byte(`{"type":"system","subtype":"init","session_id":"s","apiKeySource":"none"}`)
	for class, want := range map[string]string{
		"authentication_failed": "authentication_failed",
		"oauth_org_not_allowed": "oauth_org_not_allowed",
		"account_on_hold":       "account_on_hold",
		"billing_error":         "billing_error",
		"rate_limit":            "rate_limit",
		"something_new":         "native_error", // unrecognised classes never reach the journal verbatim
		"":                      "native_error",
	} {
		obs := decodeAll([][]byte{init,
			[]byte(`{"type":"assistant","error":"` + class + `"}`)})
		var errs []adapter.NativeError
		for _, ob := range obs {
			if e, ok := ob.(adapter.NativeError); ok {
				errs = append(errs, e)
			}
		}
		if len(errs) != 1 || errs[0].Class != want {
			t.Fatalf("class %q mapped to %+v, want %q", class, errs, want)
		}
	}
	// Object-shaped errors read their class field; a null error is absent.
	obs := decodeAll([][]byte{init,
		[]byte(`{"type":"assistant","error":{"class":"rate_limit"}}`),
		[]byte(`{"type":"assistant","error":null}`)})
	var errs []adapter.NativeError
	for _, ob := range obs {
		if e, ok := ob.(adapter.NativeError); ok {
			errs = append(errs, e)
		}
	}
	if len(errs) != 1 || errs[0].Class != "rate_limit" {
		t.Fatalf("object/null errors = %+v", errs)
	}
}

func TestDecodeOverflowIsProtocolError(t *testing.T) {
	t.Parallel()
	d := newStreamDecoder()
	var obs []adapter.Observation
	for i := 0; i < 101; i++ {
		obs = append(obs, d.decode([]byte(`{"type":`))...)
	}
	if len(obs) != 1 || obs[0] != (adapter.NativeError{Class: "protocol_error"}) {
		t.Fatalf("101 malformed frames = %+v, want one protocol_error", obs)
	}
	d2 := newStreamDecoder()
	for i := 0; i < 100; i++ {
		if obs := d2.decode([]byte(`{"type":`)); len(obs) != 0 {
			t.Fatalf("100 malformed frames must stay silent, got %+v", obs)
		}
	}
}

func TestDecodeUnknownTypesCounted(t *testing.T) {
	t.Parallel()
	d := newStreamDecoder()
	init := []byte(`{"type":"system","subtype":"init","session_id":"s","apiKeySource":"none"}`)
	frames := [][]byte{init,
		[]byte(`{"type":"frobnicate","x":1}`),
		[]byte(`{"type":"frobnicate","x":2}`),
		[]byte(`{"type":"system","subtype":"frobnicate"}`),
	}
	var obs []adapter.Observation
	for _, f := range frames {
		obs = append(obs, d.decode(f)...)
	}
	if len(obs) != 1 {
		t.Fatalf("unknown types must be counted only, got %+v", obs)
	}
	if d.unknown["vendor.claudecode.frobnicate"] != 2 || d.unknown["vendor.claudecode.system.frobnicate"] != 1 {
		t.Fatalf("unknown counters = %+v", d.unknown)
	}
}

func TestDecodeExplicitNullIsAbsent(t *testing.T) {
	t.Parallel()
	obs := decodeAll([][]byte{
		[]byte(`{"type":"system","subtype":"init","session_id":"s","apiKeySource":"none"}`),
		[]byte(`{"type":"result","subtype":"success","is_error":null,"num_turns":null,"duration_ms":null,"total_cost_usd":null,"modelUsage":{"m":{"inputTokens":null,"outputTokens":"many"}}}`),
	})
	res, ok := obs[len(obs)-1].(adapter.Result)
	if !ok {
		t.Fatalf("last observation is %T, want Result", obs[len(obs)-1])
	}
	if res.IsError || res.NumTurns != nil || res.DurationMS != nil || res.CostUSD != "" {
		t.Fatalf("nulls must read as absent: %+v", res)
	}
	usage := res.Tokens["m"]
	if usage.Input != nil || usage.Output != nil {
		t.Fatalf("null/wrong tokens must stay unknown: %+v", usage)
	}
	// Null in a required field still breaks the frame.
	for _, frame := range []string{
		`{"type":null}`,
		`{"type":"system","subtype":"init","session_id":null,"apiKeySource":"none"}`,
		`{"type":"system","subtype":"init","session_id":"s","apiKeySource":"none","model":null}`,
	} {
		obs := decodeAll([][]byte{[]byte(frame)})
		if frame == `{"type":"system","subtype":"init","session_id":"s","apiKeySource":"none","model":null}` {
			if _, ok := obs[0].(adapter.SessionStarted); !ok {
				t.Fatalf("null optional must be absent, got %+v", obs)
			}
			continue
		}
		if len(obs) != 0 && obs[0] != (adapter.NativeError{Class: "protocol_error"}) {
			t.Fatalf("%s = %+v", frame, obs)
		}
	}
}

func TestDecodeDuplicateInitIsProtocolError(t *testing.T) {
	t.Parallel()
	obs := decodeAll([][]byte{
		[]byte(`{"type":"system","subtype":"init","session_id":"s","apiKeySource":"none"}`),
		[]byte(`{"type":"system","subtype":"init","session_id":"s","apiKeySource":"none"}`),
	})
	if len(obs) != 2 || obs[1] != (adapter.NativeError{Class: "protocol_error"}) {
		t.Fatalf("duplicate init = %+v, want SessionStarted then protocol_error", obs)
	}
}

func TestDecodeSuccessWithoutInitIsProtocolError(t *testing.T) {
	t.Parallel()
	obs := decodeAll([][]byte{
		[]byte(`{"type":"result","subtype":"success","is_error":false}`),
	})
	if len(obs) != 1 || obs[0] != (adapter.NativeError{Class: "protocol_error"}) {
		t.Fatalf("success without init = %+v, want protocol_error", obs)
	}
	// Error results without init stay accepted (startup failures).
	obs = decodeAll([][]byte{
		[]byte(`{"type":"result","subtype":"error_startup","is_error":true}`),
	})
	if _, ok := obs[0].(adapter.Result); !ok {
		t.Fatalf("error without init = %+v, want Result", obs)
	}
}

func TestDecodeAssistantErrorIndependentOfContent(t *testing.T) {
	t.Parallel()
	init := []byte(`{"type":"system","subtype":"init","session_id":"s","apiKeySource":"none"}`)
	// The top-level error classifies even beside an unfamiliar content
	// shape: a failure never hides behind a new block type.
	obs := decodeAll([][]byte{init,
		[]byte(`{"type":"assistant","error":"rate_limit","message":{"content":[{"type":"unknown_future_block","x":1}]}}`)})
	var errs []adapter.NativeError
	for _, ob := range obs {
		if e, ok := ob.(adapter.NativeError); ok {
			errs = append(errs, e)
		}
	}
	if len(errs) != 1 || errs[0].Class != "rate_limit" {
		t.Fatalf("error beside unknown content = %+v, want one rate_limit", errs)
	}
	// A present message that is not an object at all is a shape
	// violation, not vendor evolution.
	obs = decodeAll([][]byte{init,
		[]byte(`{"type":"assistant","message":42}`)})
	if len(obs) != 3 || obs[2] != (adapter.NativeError{Class: "protocol_error"}) {
		t.Fatalf("non-object message = %+v, want progress then protocol_error", obs)
	}
	// message.error is not a vendor shape (SDKAssistantMessage.error is
	// top-level): the narrow content read ignores it.
	obs = decodeAll([][]byte{init,
		[]byte(`{"type":"assistant","message":{"content":[],"error":"authentication_failed"}}`)})
	for _, ob := range obs {
		if e, ok := ob.(adapter.NativeError); ok {
			t.Fatalf("nested message.error classified as %+v, want ignored", e)
		}
	}
}

func TestDecodeAliasConflictsFailTheFrame(t *testing.T) {
	t.Parallel()
	for _, frame := range []string{
		`{"type":"system","subtype":"init","session_id":"s","apiKeySource":"none","claude_code_version":"1","version":"2"}`,
		`{"type":"system","subtype":"init","session_id":"s","apiKeySource":"none","mcp_servers":[],"mcpServers":[{}]}`,
	} {
		obs := decodeAll([][]byte{[]byte(frame)})
		if len(obs) != 1 || obs[0] != (adapter.NativeError{Class: "protocol_error"}) {
			t.Fatalf("%s = %+v, want one protocol_error", frame, obs)
		}
	}
	// Result conflicts decode after init: without it the
	// success-without-init rule would answer instead.
	init := []byte(`{"type":"system","subtype":"init","session_id":"s","apiKeySource":"none"}`)
	for _, frame := range []string{
		`{"type":"result","subtype":"success","is_error":false,"isError":true}`,
		`{"type":"result","subtype":"success","total_cost_usd":"1.00","totalCostUsd":"2.00"}`,
		`{"type":"result","subtype":"success","num_turns":1,"numTurns":2}`,
		`{"type":"result","subtype":"success","stop_reason":"a","stopReason":"b"}`,
	} {
		obs := decodeAll([][]byte{init, []byte(frame)})
		if len(obs) != 2 || obs[1] != (adapter.NativeError{Class: "protocol_error"}) {
			t.Fatalf("%s = %+v, want session then protocol_error", frame, obs)
		}
	}
	// Identical spellings agree and are accepted.
	obs := decodeAll([][]byte{
		[]byte(`{"type":"system","subtype":"init","session_id":"s","apiKeySource":"none"}`),
		[]byte(`{"type":"result","subtype":"success","is_error":false,"isError":false}`),
	})
	if _, ok := obs[1].(adapter.Result); !ok {
		t.Fatalf("agreeing spellings = %+v, want Result", obs)
	}
}

func TestDecodeUsageAndModelUsageAreComplementary(t *testing.T) {
	t.Parallel()
	init := []byte(`{"type":"system","subtype":"init","session_id":"s","apiKeySource":"none"}`)
	// Live natives emit aggregate usage alongside the per-model
	// breakdown: the two complement, never contradict.
	frame := []byte(`{"type":"result","subtype":"success","is_error":false,` +
		`"usage":{"input_tokens":11,"output_tokens":3},` +
		`"modelUsage":{"synthetic-model":{"inputTokens":11,"outputTokens":3}}}`)
	obs := decodeAll([][]byte{init, frame})
	if len(obs) != 2 {
		t.Fatalf("both usage fields = %+v, want session then Result", obs)
	}
	res, ok := obs[1].(adapter.Result)
	if !ok {
		t.Fatalf("both usage fields = %+v, want Result", obs)
	}
	got, ok := res.Tokens["synthetic-model"]
	if !ok || got.Input == nil || *got.Input != 11 || got.Output == nil || *got.Output != 3 {
		t.Fatalf("Tokens = %+v, want the modelUsage breakdown", res.Tokens)
	}
	// A flat aggregate alone has no model to attribute to: the result
	// still decodes, with usage unreported rather than guessed.
	flat := []byte(`{"type":"result","subtype":"success","is_error":false,` +
		`"usage":{"input_tokens":11,"output_tokens":3}}`)
	obs = decodeAll([][]byte{init, flat})
	if len(obs) != 2 {
		t.Fatalf("flat usage = %+v, want session then Result", obs)
	}
	res, ok = obs[1].(adapter.Result)
	if !ok || res.Tokens != nil {
		t.Fatalf("flat usage = %+v, want Result with nil Tokens", obs)
	}
	// Older natives carried the breakdown in usage itself; that shape
	// still decodes when modelUsage is absent.
	legacy := []byte(`{"type":"result","subtype":"success","is_error":false,` +
		`"usage":{"synthetic-model":{"inputTokens":7,"outputTokens":1}}}`)
	obs = decodeAll([][]byte{init, legacy})
	if len(obs) != 2 {
		t.Fatalf("legacy usage = %+v, want session then Result", obs)
	}
	res, ok = obs[1].(adapter.Result)
	if !ok {
		t.Fatalf("legacy usage = %+v, want Result", obs)
	}
	if got, ok := res.Tokens["synthetic-model"]; !ok || got.Input == nil || *got.Input != 7 {
		t.Fatalf("Tokens = %+v, want the legacy breakdown", res.Tokens)
	}
}

func TestDecodeCostGrammarIsDecimalOnly(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		raw   string
		want  string
		valid bool
	}{
		{`{"type":"result","subtype":"success","total_cost_usd":0}`, "0", true},
		{`{"type":"result","subtype":"success","total_cost_usd":1e-3}`, "1e-3", true},
		{`{"type":"result","subtype":"success","total_cost_usd":"4.50"}`, "4.50", true},
		{`{"type":"result","subtype":"success","total_cost_usd":"3/2"}`, "", false},
		{`{"type":"result","subtype":"success","total_cost_usd":true}`, "", false},
		{`{"type":"result","subtype":"success","total_cost_usd":"0x10"}`, "", false},
		{`{"type":"result","subtype":"success","total_cost_usd":""}`, "", false},
	} {
		obs := decodeAll([][]byte{
			[]byte(`{"type":"system","subtype":"init","session_id":"s","apiKeySource":"none"}`),
			[]byte(tc.raw),
		})
		if !tc.valid {
			if len(obs) != 2 || obs[1] != (adapter.NativeError{Class: "protocol_error"}) {
				t.Fatalf("%s = %+v, want protocol_error", tc.raw, obs)
			}
			continue
		}
		res, ok := obs[1].(adapter.Result)
		if !ok || res.CostUSD != tc.want {
			t.Fatalf("%s cost = %+v, want %q", tc.raw, obs, tc.want)
		}
	}
}

func TestDecodeBoundsNativeStrings(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("x", maxNativeString+1)
	for _, frame := range []string{
		`{"type":"system","subtype":"init","session_id":"` + long + `","apiKeySource":"none"}`,
		`{"type":"system","subtype":"init","session_id":"s","apiKeySource":"none","model":"bad\u0007model"}`,
		`{"type":"result","subtype":"` + long + `"}`,
	} {
		obs := decodeAll([][]byte{[]byte(frame)})
		if len(obs) != 1 || obs[0] != (adapter.NativeError{Class: "protocol_error"}) {
			t.Fatalf("unbounded string accepted: %+v", obs)
		}
	}
}

func TestDecodeCostNeverFloat(t *testing.T) {
	t.Parallel()
	// 0.0123 has no exact float64; only a literal-preserving decode keeps it.
	for _, tc := range []struct{ raw, want string }{
		{`{"type":"result","subtype":"success","total_cost_usd":0.0123}`, "0.0123"},
		{`{"type":"result","subtype":"success","total_cost_usd":"1.07"}`, "1.07"},
		{`{"type":"result","subtype":"success","totalCostUsd":2.5}`, "2.5"},
		{`{"type":"result","subtype":"success"}`, ""},
	} {
		obs := decodeAll([][]byte{
			[]byte(`{"type":"system","subtype":"init","session_id":"s","apiKeySource":"none"}`),
			[]byte(tc.raw),
		})
		res, ok := obs[1].(adapter.Result)
		if !ok || res.CostUSD != tc.want {
			t.Fatalf("%s cost = %+v, want %q", tc.raw, obs, tc.want)
		}
	}
}
