package codex

import (
	"encoding/json"
	"errors"
	"io"
	"unicode"
	"unicode/utf8"

	"github.com/turbokast/mythhelm/internal/adapter"
	"github.com/turbokast/mythhelm/internal/adapter/ndjson"
)

const (
	// maxUnknownTypes bounds the distinct unknown frame types counted under
	// their own names; the rest collapse into vendor.codex.other.
	maxUnknownTypes = 32
	// maxNativeString bounds the native strings that reach observations
	// (session id, subtype, stop reason, tool name): the journal holds
	// identifiers, not unbounded native text (NFR-2).
	maxNativeString = 256
)

// errIncomplete reports a stream that ended without a result frame.
var errIncomplete = errors.New("codex: stream ended without a result frame")

// nativeErrorClass is the class of an error whose shape is not documented
// (D7); the other classes appear only through the error table (D14).
const nativeErrorClass = "native_error"

var knownErrorClasses = map[string]bool{
	nativeErrorClass:      true,
	"rate_limit":          true,
	"allowance_exhausted": true,
}

// Decode reads the synthetic NDJSON frames from r and sends the observations
// they carry, then the ProtocolCounters. Usage stays nil (D6) and every error
// is a native_error until the error table is filled from a cited source (D7).
// A stream that ends without a result frame sets NativeExit.Err.
func Decode(r io.Reader, send func(adapter.Observation)) adapter.NativeExit {
	return decodeWith(nil, ndjson.Limits{}, r, send)
}

// frame is the union of the synthetic schema's fields. Pointer fields
// distinguish absent or null from a zero value.
type frame struct {
	Type       string  `json:"type"`
	SessionID  *string `json:"session_id"`
	Name       *string `json:"name"`
	Code       *string `json:"code"`
	Subtype    *string `json:"subtype"`
	IsError    *bool   `json:"is_error"`
	StopReason *string `json:"stop_reason"`
}

type decoder struct {
	table     map[string]string // native error code -> class
	send      func(adapter.Observation)
	started   bool
	turns     int
	gotResult bool
	malformed int64
	unknown   map[string]int64
	named     int
}

// decodeWith is Decode with an explicit error table and frame limits.
func decodeWith(table map[string]string, lim ndjson.Limits, r io.Reader, send func(adapter.Observation)) adapter.NativeExit {
	d := &decoder{table: table, send: send, unknown: map[string]int64{}}
	rd := ndjson.NewReader(r, lim)
	var readErr error
	for {
		raw, err := rd.Next()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				readErr = err
			}
			break
		}
		d.decode(raw)
	}
	send(adapter.ProtocolCounters{Counters: rd.Counters(), Malformed: d.malformed, UnknownTypes: d.unknown})
	switch {
	case readErr != nil:
		return adapter.NativeExit{Err: readErr}
	case !d.gotResult:
		return adapter.NativeExit{Err: errIncomplete}
	}
	return adapter.NativeExit{}
}

func (d *decoder) decode(raw []byte) {
	var f frame
	if json.Unmarshal(raw, &f) != nil || f.Type == "" {
		d.malformed++
		return
	}
	if f.Type != "session" {
		d.start("")
	}
	switch f.Type {
	case "session":
		d.decodeSession(f)
	case "turn":
		d.turns++
		d.send(adapter.Progress{Turn: d.turns})
	case "tool":
		d.send(adapter.Progress{Turn: d.turns, Tool: boundedName(f.Name, "invalid")})
	case "error":
		d.send(adapter.NativeError{Class: d.errorClass(f.Code)})
	case "result":
		d.decodeResult(f)
	default:
		d.countUnknown(f.Type)
	}
}

// start sends SessionStarted once. An identity the native did not report is
// sent empty: unknown, never synthesised (AC-6.1). APIKeySource stays empty
// (D12).
func (d *decoder) start(id string) {
	if d.started {
		return
	}
	d.started = true
	d.send(adapter.SessionStarted{SessionID: id})
}

func (d *decoder) decodeSession(f frame) {
	if d.started {
		d.malformed++ // a second session frame contradicts the identity already sent
		return
	}
	d.start(boundedName(f.SessionID, ""))
}

func (d *decoder) decodeResult(f frame) {
	if d.gotResult || f.Subtype == nil || !printable(*f.Subtype) {
		d.malformed++
		return
	}
	d.gotResult = true
	d.send(adapter.Result{
		Subtype:    *f.Subtype,
		IsError:    f.IsError != nil && *f.IsError,
		StopReason: boundedName(f.StopReason, ""),
	})
}

// errorClass maps a native error code through the table. A code the table
// does not name, or a class outside the known three, is native_error.
func (d *decoder) errorClass(code *string) string {
	if code == nil {
		return nativeErrorClass
	}
	if class := d.table[*code]; knownErrorClasses[class] {
		return class
	}
	return nativeErrorClass
}

func (d *decoder) countUnknown(typ string) {
	key := "vendor.codex." + typ
	switch {
	case !printable(typ):
		key = "vendor.codex.invalid"
	case d.unknown[key] > 0:
	case d.named >= maxUnknownTypes:
		key = "vendor.codex.other"
	default:
		d.named++
	}
	d.unknown[key]++
}

// boundedName returns *s when it is a short printable string, else fallback.
func boundedName(s *string, fallback string) string {
	if s == nil || !printable(*s) {
		return fallback
	}
	return *s
}

func printable(s string) bool {
	if len(s) > maxNativeString || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}
