package claudecode

import (
	"encoding/json"
	"strings"
	"testing"
)

// redactLiveFrame redacts a recorded frame's native text for the fixture:
// identities, prompt/assistant text and tool inputs never enter the repo.
// Task 20 applies it by hand to the reviewed stream. It lives in this
// untagged file (not beside the live canary) so ordinary test runs keep
// the redaction rule pinned even though the canary never runs in CI.
func redactLiveFrame(frame []byte) []byte {
	var obj map[string]any
	if json.Unmarshal(frame, &obj) != nil {
		return []byte(`{"type":"redacted-unparseable"}`)
	}
	redactLiveValue(obj)
	out, err := json.Marshal(obj)
	if err != nil {
		return []byte(`{"type":"redacted-unparseable"}`)
	}
	return out
}

func redactLiveValue(v any) {
	obj, ok := v.(map[string]any)
	if !ok {
		return
	}
	for key, val := range obj {
		switch key {
		case "session_id", "sessionId", "text", "input", "command", "content":
			obj[key] = "REDACTED"
		default:
			switch nested := val.(type) {
			case map[string]any:
				redactLiveValue(nested)
			case []any:
				for _, item := range nested {
					redactLiveValue(item)
				}
			}
		}
	}
}

func TestRedactLiveFrame(t *testing.T) {
	out := redactLiveFrame([]byte(`{"type":"assistant","session_id":"s3cr3t","message":{"content":[{"type":"text","text":"hello"}]}}`))
	if strings.Contains(string(out), "s3cr3t") || strings.Contains(string(out), "hello") {
		t.Fatalf("redaction leaked native text: %s", out)
	}
	var obj map[string]any
	if err := json.Unmarshal(out, &obj); err != nil || obj["type"] != "assistant" {
		t.Fatalf("redaction broke the frame: %s", out)
	}
}
