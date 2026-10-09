package claudecode

import (
	"encoding/json"
	"regexp"
	"slices"
	"unicode"

	"github.com/turbokast/mythhelm/internal/adapter"
)

const (
	// maxMalformedFrames is the design §6.4 budget: beyond it the attempt
	// fails with protocol_error rather than running on an unreadable stream.
	maxMalformedFrames = 100
	// maxUnknownStreamTypes bounds the distinct unknown frame types counted
	// under their own names; the rest collapse into vendor.claudecode.other.
	maxUnknownStreamTypes = 32
	// maxStreamToolName is the longest tool name reported verbatim; longer
	// names and names with control characters become "invalid" (design §6.4).
	maxStreamToolName = 64
	// maxStreamModels bounds the per-model usage entries kept from one
	// result frame, and maxStreamDenials the permission denials, and
	// maxStreamMCPServers the session's MCP servers: all three feed the
	// journal, which must stay bounded whatever the native emits.
	maxStreamModels     = 64
	maxStreamDenials    = 1024
	maxStreamMCPServers = 256
	// maxNativeString bounds top-level native strings (session, subtype,
	// model and friends). Anything longer, or not printable, breaks the
	// frame: unbounded native text must not reach the journal (NFR-2).
	maxNativeString = 1024
)

// jsonNumber is the JSON number grammar. Cost literals must match it
// exactly: no fractions, no hex, no infinities.
var jsonNumber = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$`)

// errorClasses are the native error classes the result-mapping table knows.
// Anything else is reported as native_error, never verbatim: the journal
// holds identifiers, not native text (NFR-2). allowance_exhausted is
// fixture-qualified only (budget-ledger-s1 D4, D10): the exact class name is
// the single documented shape, rate_limit stays the transient class, and any
// other spelling fails closed to native_error. The class carries no reset
// time, so reset stays unknown until MH-12 Task 7 qualifies a live shape.
var errorClasses = []string{
	"authentication_failed", "oauth_org_not_allowed", "account_on_hold",
	"billing_error", "rate_limit", "allowance_exhausted",
}

// streamDecoder turns Claude Code stream-json frames into observations. The
// shapes below are doc-derived synthetics until Task 20 records a live
// stream, so field reads tolerate the documented snake_case and camelCase
// spellings. An explicit JSON null reads as absent everywhere. Any two
// spellings that disagree, any wrongly typed or out-of-bounds top-level
// field, fails the frame: tolerance never resolves a contradiction.
type streamDecoder struct {
	sawInit      bool
	turns        int
	malformed    int64
	protocolSent bool
	unknown      map[string]int64
	named        int
}

func newStreamDecoder() *streamDecoder { return &streamDecoder{unknown: map[string]int64{}} }

// decode returns a frame's observations: at most one of SessionStarted,
// Retry, PermissionDenied, NativeError or Result, plus any Progress. A nil
// return means the frame was only counted.
func (d *streamDecoder) decode(frame []byte) []adapter.Observation {
	var obj map[string]json.RawMessage
	if json.Unmarshal(frame, &obj) != nil {
		return d.noteMalformed()
	}
	typ, ok := objectString(obj, "type")
	if !ok {
		return d.noteMalformed()
	}
	switch typ {
	case "system":
		return d.decodeSystem(obj)
	case "assistant":
		return d.decodeAssistant(obj)
	case "user", "stream_event", "plugin_install", "compact_boundary":
		return nil // counted by the ndjson reader only
	case "result":
		return d.decodeResultFrame(obj)
	default:
		d.countUnknown(typ)
		return nil
	}
}

func (d *streamDecoder) decodeSystem(obj map[string]json.RawMessage) []adapter.Observation {
	subtype, ok := objectString(obj, "subtype")
	if !ok {
		return d.noteMalformed()
	}
	switch subtype {
	case "init":
		started, ok := decodeInit(obj)
		if !ok {
			return d.protocolViolation()
		}
		if d.sawInit {
			// A second init contradicts the session identity the
			// worker already acted on (notably apiKeySource): fail
			// closed rather than ignoring the conflict.
			return d.protocolViolation()
		}
		d.sawInit = true
		return []adapter.Observation{started}
	case "api_retry":
		return []adapter.Observation{adapter.Retry{
			Attempt:      liberalInt(obj, "attempt"),
			RetryDelayMS: int64(liberalAliasInt(obj, "retry_delay_ms", "retryDelayMs", "retry_delay")),
			ErrorStatus:  liberalAliasInt(obj, "error_status", "errorStatus", "status"),
		}}
	case "permission_denied":
		name, st := aliasString(obj, "tool_name", "toolName", "tool")
		if st != fieldFound || name == "" {
			return d.noteMalformed()
		}
		return []adapter.Observation{adapter.PermissionDenied{ToolName: streamToolName(name)}}
	case "hook", "compact_boundary":
		return nil // counted by the ndjson reader only
	default:
		d.countUnknown("system." + subtype)
		return nil
	}
}

func (d *streamDecoder) decodeAssistant(obj map[string]json.RawMessage) []adapter.Observation {
	if !d.sawInit {
		return d.protocolViolation()
	}
	d.turns++
	var out []adapter.Observation
	out = append(out, adapter.Progress{Turn: d.turns})
	// The error rides top-level on the vendor's assistant message
	// (SDKAssistantMessage.error). It is classified independently of the
	// content shape, so a failure can never hide behind a new block type.
	if class, ok := errorClass(obj["error"]); ok {
		out = append(out, adapter.NativeError{Class: class})
	}
	raw, ok := obj["message"]
	if !ok || string(raw) == "null" {
		return out
	}
	var msg struct {
		Content []struct {
			Type string `json:"type"`
			Name string `json:"name"`
		} `json:"content"`
	}
	// Tool extraction is informational, but a present message that is not
	// an object at all is a shape violation, not vendor evolution.
	if json.Unmarshal(raw, &msg) != nil {
		return append(out, d.protocolViolation()...)
	}
	for _, block := range msg.Content {
		if block.Type == "tool_use" {
			out = append(out, adapter.Progress{Turn: d.turns, Tool: streamToolName(block.Name)})
		}
	}
	return out
}

func (d *streamDecoder) decodeResultFrame(obj map[string]json.RawMessage) []adapter.Observation {
	res, ok := d.decodeResult(obj)
	if !ok {
		return d.protocolViolation()
	}
	return []adapter.Observation{res}
}

// noteMalformed counts one unusable frame. Past the budget the attempt fails
// with protocol_error instead of running blind.
func (d *streamDecoder) noteMalformed() []adapter.Observation {
	d.malformed++
	if d.malformed > maxMalformedFrames && !d.protocolSent {
		d.protocolSent = true
		return []adapter.Observation{adapter.NativeError{Class: "protocol_error"}}
	}
	return nil
}

// protocolViolation counts one frame that breaks the stream contract (a
// malformed init or result, an out-of-order assistant frame, a duplicate
// init or a success without a session) and reports it once.
func (d *streamDecoder) protocolViolation() []adapter.Observation {
	d.malformed++
	if !d.protocolSent {
		d.protocolSent = true
		return []adapter.Observation{adapter.NativeError{Class: "protocol_error"}}
	}
	return nil
}

func (d *streamDecoder) countUnknown(typ string) {
	key := "vendor.claudecode." + typ
	switch {
	case !printableName(typ):
		key = "vendor.claudecode.invalid"
	case d.unknown[key] > 0:
	case d.named >= maxUnknownStreamTypes:
		key = "vendor.claudecode.other"
	default:
		d.named++
	}
	d.unknown[key]++
}

// decodeInit reads the required session identity. apiKeySource must be
// present: the worker's AC-4.4 check needs a reported value, and a native
// that omits it is breaking the contract, not reporting "none".
func decodeInit(obj map[string]json.RawMessage) (adapter.SessionStarted, bool) {
	session, st := aliasString(obj, "session_id", "sessionId")
	if st != fieldFound || session == "" {
		return adapter.SessionStarted{}, false
	}
	source, st := aliasString(obj, "apiKeySource", "api_key_source")
	if st != fieldFound || source == "" {
		return adapter.SessionStarted{}, false
	}
	model, st := aliasString(obj, "model")
	if st == fieldBad {
		return adapter.SessionStarted{}, false
	}
	version, st := aliasString(obj, "claude_code_version", "claudeCodeVersion", "version")
	if st == fieldBad {
		return adapter.SessionStarted{}, false
	}
	mode, st := aliasString(obj, "permissionMode", "permission_mode")
	if st == fieldBad {
		return adapter.SessionStarted{}, false
	}
	var tools []json.RawMessage
	if raw, ok := obj["tools"]; ok && string(raw) != "null" {
		_ = json.Unmarshal(raw, &tools)
	}
	serversRaw, st := aliasRaw(obj, "mcp_servers", "mcpServers")
	if st == fieldBad {
		return adapter.SessionStarted{}, false
	}
	var servers []struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	}
	if st == fieldFound {
		_ = json.Unmarshal(serversRaw, &servers)
	}
	mcp := make([]adapter.MCPServer, 0, len(servers))
	for _, s := range servers {
		if len(mcp) >= maxStreamMCPServers || !printableName(s.Name) {
			continue
		}
		mcp = append(mcp, adapter.MCPServer{Name: s.Name, Status: s.Status})
	}
	var plugins []json.RawMessage
	if raw, ok := obj["plugins"]; ok && string(raw) != "null" {
		_ = json.Unmarshal(raw, &plugins)
	}
	return adapter.SessionStarted{
		SessionID: session, Model: model, NativeVersion: version,
		PermissionMode: mode, APIKeySource: source, ToolCount: len(tools),
		MCPServers: mcp, PluginCount: len(plugins),
	}, true
}

func (d *streamDecoder) decodeResult(obj map[string]json.RawMessage) (adapter.Result, bool) {
	subtype, ok := objectString(obj, "subtype")
	if !ok || subtype == "" || !validNativeString(subtype) {
		return adapter.Result{}, false
	}
	var res adapter.Result
	res.Subtype = subtype
	isErr, st := aliasRaw(obj, "is_error", "isError")
	switch st {
	case fieldBad:
		return adapter.Result{}, false
	case fieldFound:
		if json.Unmarshal(isErr, &res.IsError) != nil {
			return adapter.Result{}, false
		}
	}
	if n, present, ok := aliasInt(obj, "num_turns", "numTurns"); !ok {
		return adapter.Result{}, false
	} else if present {
		res.NumTurns = &n
	}
	if ms, present, ok := aliasInt(obj, "duration_ms", "durationMs", "duration"); !ok {
		return adapter.Result{}, false
	} else if present {
		ms64 := int64(ms)
		res.DurationMS = &ms64
	}
	stop, st := aliasString(obj, "stop_reason", "stopReason")
	if st == fieldBad {
		return adapter.Result{}, false
	}
	res.StopReason = stop
	tokens, ok := decodeUsage(obj)
	if !ok {
		return adapter.Result{}, false
	}
	res.Tokens = tokens
	cost, ok := decodeCost(obj)
	if !ok {
		return adapter.Result{}, false
	}
	res.CostUSD = cost
	denials, ok := decodeDenials(obj)
	if !ok {
		return adapter.Result{}, false
	}
	res.PermissionDenials = denials
	if raw, ok := obj["errors"]; ok && string(raw) != "null" {
		var errs []json.RawMessage
		if json.Unmarshal(raw, &errs) == nil {
			res.ErrorCount = len(errs)
		}
	}
	startup, st := aliasString(obj, "startup_failure_reason", "startupFailureReason")
	if st == fieldBad {
		return adapter.Result{}, false
	}
	res.StartupFailureReason = startup
	if !d.sawInit && !res.IsError && res.Subtype == "success" {
		// Error results may arrive without a session (startup
		// failures), but a success with no apiKeySource ever observed
		// must not succeed: AC-4.4 has nothing to check it against.
		return adapter.Result{}, false
	}
	return res, true
}

// decodeCost keeps the native cost literal: a JSON number stays its exact
// decimal text, never a float64 (I09, AC-4.5). A present but non-numeric
// cost breaks the result frame; an absent one stays unknown.
func decodeCost(obj map[string]json.RawMessage) (string, bool) {
	raw, st := aliasRaw(obj, "total_cost_usd", "totalCostUsd", "cost_usd")
	if st != fieldFound {
		return "", st != fieldBad
	}
	if len(raw) > 0 && raw[0] == '"' {
		var s string
		if json.Unmarshal(raw, &s) != nil {
			return "", false
		}
		return s, jsonNumber.MatchString(s)
	}
	s := string(raw)
	return s, jsonNumber.MatchString(s)
}

func decodeUsage(obj map[string]json.RawMessage) (map[string]adapter.TokenUsage, bool) {
	// modelUsage is the per-model breakdown and usage the aggregate
	// totals: live natives emit both, so they are read separately and
	// never compared as aliases.
	if raw, st := aliasRaw(obj, "modelUsage", "model_usage"); st == fieldBad {
		return nil, false
	} else if st == fieldFound {
		if out := perModelUsage(raw); len(out) > 0 {
			return out, true
		}
	}
	// Older natives carried the breakdown in usage itself; a flat
	// aggregate has no model to attribute to and stays unreported
	// rather than guessed.
	if raw, ok := obj["usage"]; ok && string(raw) != "null" {
		if out := perModelUsage(raw); len(out) > 0 {
			return out, true
		}
	}
	return nil, true
}

func perModelUsage(raw json.RawMessage) map[string]adapter.TokenUsage {
	var perModel map[string]map[string]json.RawMessage
	if json.Unmarshal(raw, &perModel) != nil {
		return nil
	}
	// Sorted for a deterministic journal when the native reports more
	// models than the bound keeps.
	models := make([]string, 0, len(perModel))
	for model := range perModel {
		models = append(models, model)
	}
	slices.Sort(models)
	out := make(map[string]adapter.TokenUsage, len(perModel))
	for _, model := range models {
		if len(out) >= maxStreamModels || !printableName(model) {
			continue
		}
		fields := perModel[model]
		out[model] = adapter.TokenUsage{
			Input:         tokenCount(fields, "inputTokens", "input_tokens", "input"),
			Output:        tokenCount(fields, "outputTokens", "output_tokens", "output"),
			CacheRead:     tokenCount(fields, "cacheReadInputTokens", "cache_read_input_tokens", "cache_read"),
			CacheCreation: tokenCount(fields, "cacheCreationInputTokens", "cache_creation_input_tokens", "cache_creation"),
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// tokenCount reads one informational token field: absent, null, ambiguous
// or wrongly typed stays unknown (nil), never zero (I09).
func tokenCount(fields map[string]json.RawMessage, keys ...string) *int64 {
	raw, st := aliasRaw(fields, keys...)
	if st != fieldFound {
		return nil
	}
	var n int64
	if json.Unmarshal(raw, &n) != nil {
		return nil
	}
	return &n
}

func decodeDenials(obj map[string]json.RawMessage) ([]string, bool) {
	raw, st := aliasRaw(obj, "permission_denials", "permissionDenials")
	if st == fieldBad {
		return nil, false
	}
	if st != fieldFound {
		return []string{}, true
	}
	var entries []json.RawMessage
	if json.Unmarshal(raw, &entries) != nil {
		return []string{}, true
	}
	denials := []string{}
	for _, entry := range entries {
		if len(denials) >= maxStreamDenials {
			break
		}
		var name string
		if json.Unmarshal(entry, &name) == nil {
			denials = append(denials, streamToolName(name))
			continue
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(entry, &fields) != nil {
			continue
		}
		// Denial entries are liberal reads: a contradictory entry is
		// skipped, not fatal to the result that carries it.
		if raw, st := aliasRaw(fields, "tool_name", "toolName", "tool", "name"); st == fieldFound {
			var entryName string
			if json.Unmarshal(raw, &entryName) == nil {
				denials = append(denials, streamToolName(entryName))
			}
		}
	}
	return denials, true
}

// errorClass maps a message error to its fixed class. Only a JSON null
// means no error; an ambiguous or uninterpretable error shape becomes
// native_error, which poisons like any unrecognised class.
func errorClass(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", false
	}
	var asString string
	if json.Unmarshal(raw, &asString) == nil {
		return mapErrorClass(asString), true
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return "native_error", true
	}
	if class, st := aliasString(fields, "class", "type", "code"); st == fieldFound {
		return mapErrorClass(class), true
	}
	return "native_error", true
}

func mapErrorClass(class string) string {
	if slices.Contains(errorClasses, class) {
		return class
	}
	return "native_error"
}

// fieldStatus is what an aliased read found. An explicit JSON null counts
// as missing everywhere: null is the absent-encoding, never a value.
type fieldStatus uint8

const (
	fieldMissing fieldStatus = iota
	fieldFound
	fieldBad // disagreeing spellings: fail the frame, never guess
)

// aliasRaw returns the value for the first present spelling. Two present
// spellings with different values are ambiguous: fail closed.
func aliasRaw(obj map[string]json.RawMessage, keys ...string) (json.RawMessage, fieldStatus) {
	var found json.RawMessage
	seen := false
	for _, k := range keys {
		raw, ok := obj[k]
		if !ok || string(raw) == "null" {
			continue
		}
		if seen && string(raw) != string(found) {
			return nil, fieldBad
		}
		found, seen = raw, true
	}
	if !seen {
		return nil, fieldMissing
	}
	return found, fieldFound
}

// aliasString reads one top-level native string: bounded and printable, or
// the frame fails.
func aliasString(obj map[string]json.RawMessage, keys ...string) (string, fieldStatus) {
	raw, st := aliasRaw(obj, keys...)
	if st != fieldFound {
		return "", st
	}
	var s string
	if json.Unmarshal(raw, &s) != nil || !validNativeString(s) {
		return "", fieldBad
	}
	return s, fieldFound
}

// aliasInt reads one top-level native integer. It reports presence
// separately so a null reads as unknown, never zero (I09).
func aliasInt(obj map[string]json.RawMessage, keys ...string) (int, bool, bool) {
	raw, st := aliasRaw(obj, keys...)
	if st != fieldFound {
		return 0, false, st != fieldBad
	}
	var n int
	if json.Unmarshal(raw, &n) != nil {
		return 0, false, false
	}
	return n, true, true
}

// objectString reads one single-spelling field. Null is not a string.
func objectString(obj map[string]json.RawMessage, key string) (string, bool) {
	raw, ok := obj[key]
	if !ok || string(raw) == "null" {
		return "", false
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return "", false
	}
	return s, true
}

// liberalInt reads one informational integer: anything uninterpretable
// stays zero. Only retry details use it; the worker journals the retry
// count, never these values, so they cannot become false zeros in storage.
func liberalInt(obj map[string]json.RawMessage, key string) int {
	raw, ok := obj[key]
	if !ok || string(raw) == "null" {
		return 0
	}
	var n int
	_ = json.Unmarshal(raw, &n)
	return n
}

func liberalAliasInt(obj map[string]json.RawMessage, keys ...string) int {
	raw, st := aliasRaw(obj, keys...)
	if st != fieldFound {
		return 0
	}
	var n int
	_ = json.Unmarshal(raw, &n)
	return n
}

// validNativeString bounds one top-level native string for the journal.
func validNativeString(s string) bool {
	if len(s) > maxNativeString {
		return false
	}
	for _, r := range s {
		if !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}

// streamToolName reports a tool name verbatim, or "invalid" when it is too
// long or not printable. An empty name stays empty: callers that require a
// name reject it.
func streamToolName(name string) string {
	if name != "" && !printableName(name) {
		return "invalid"
	}
	return name
}

func printableName(s string) bool {
	if s == "" || len(s) > maxStreamToolName {
		return false
	}
	for _, r := range s {
		if !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}
