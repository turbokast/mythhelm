// Package control implements the per-user supervisor control surface (v2
// §3.1): the local control protocol, instance lock, intent server and
// reservation manager. The wire format is length-prefixed JSON; every
// ingress point enforces the stream-1 frame limits (NFR-1).
package control

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"strings"

	"github.com/turbokast/mythhelm/internal/v2contract"
)

// prefixLen is the wire length-prefix size: a 4-byte big-endian payload length.
const prefixLen = 4

// refKey is the wire key whose string values count as artefact references
// for NFR-1, matching the record vocabulary.
//
//nolint:misspell // "artifact_id" is the spec-mandated wire key (v2contract.Artifact)
const refKey = "artifact_id"

// Frame is one control wire object: the stream-1 request envelope plus the
// method call it carries. Method and Params are opaque at ingress; dispatch
// and handler semantics ship with the intent server. CapabilityToken is
// accepted so token-bearing frames pass ingress; mint and check ship with
// the intent server.
type Frame struct {
	v2contract.RequestEnvelope
	Method          string          `json:"method"`
	Params          json.RawMessage `json:"params,omitempty"`
	CapabilityToken string          `json:"capability_token,omitempty"`
}

// Validate enforces the embedded request envelope: operation_id present,
// expected_revision non-negative. Method presence is a dispatch concern,
// not an ingress one.
func (f Frame) Validate() error {
	return f.RequestEnvelope.Validate()
}

// Encode marshals v as one JSON object and prefixes it with its 4-byte
// big-endian length. Limit enforcement happens at ingress (CheckIngress),
// not at encode time.
func Encode(v any) ([]byte, error) {
	payload, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("control: encode frame: %w", err)
	}
	if len(payload) > math.MaxUint32 {
		return nil, fmt.Errorf("control: encode frame: payload %d bytes exceeds 4 GiB prefix range", len(payload))
	}
	frame := make([]byte, prefixLen+len(payload))
	binary.BigEndian.PutUint32(frame[:prefixLen], uint32(len(payload))) //nolint:gosec // G115: guarded against overflow above
	copy(frame[prefixLen:], payload)
	return frame, nil
}

// Decode strips the length prefix and strictly decodes the payload into T
// with v2contract.Decode semantics: unknown keys rejected, trailing data
// rejected, then Validate. Both core and attachment limits are checked
// before decoding typed payloads.
func Decode[T v2contract.Validator](frame []byte) (T, error) {
	var zero T
	limit := v2contract.MaxFrameBytes
	recordType := reflect.TypeFor[T]()
	for recordType.Kind() == reflect.Pointer {
		recordType = recordType.Elem()
	}
	if bounded, ok := reflect.TypeAssert[interface{ frameLimit() int }](reflect.Zero(recordType)); ok {
		limit = bounded.frameLimit()
	}
	if len(frame) > limit {
		return zero, contractError("frame exceeds limit")
	}
	payload, err := splitPrefix(frame)
	if err != nil {
		return zero, err
	}
	v, err := decodeWire[T](payload)
	if err != nil {
		return zero, err
	}
	if err := v.Validate(); err != nil {
		return zero, err
	}
	return v, nil
}

// decodeWire checks bounded JSON structure, duplicate keys and exact field names
// before decoding. Raw journal payloads remain opaque historical event data.
func decodeWire[T any](payload []byte) (T, error) {
	var zero T
	if len(payload)+prefixLen > v2contract.MaxFrameBytes {
		return zero, contractError("frame exceeds limit")
	}
	if bytes.Equal(bytes.TrimSpace(payload), []byte("null")) {
		return zero, contractError("top-level record cannot be null")
	}

	depth, refs, err := scanPayload(payload)
	if err != nil {
		return zero, contractError("malformed JSON frame: " + err.Error())
	}
	if err := v2contract.CheckFrameLimits(len(payload)+prefixLen, depth, refs); err != nil {
		return zero, contractError(err.Error())
	}
	if err := wireShape(payload, reflect.TypeFor[T]()); err != nil {
		return zero, err
	}
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.DisallowUnknownFields()
	var value T
	if err := dec.Decode(&value); err != nil {
		return zero, contractError("malformed typed frame")
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return zero, contractError("trailing JSON data")
	}
	return value, nil
}

func wireShape(data []byte, t reflect.Type) error {
	if t.Kind() == reflect.Pointer {
		if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
			return nil
		}
		return wireShape(data, t.Elem())
	}
	if t == reflect.TypeFor[json.RawMessage]() {
		return nil
	}
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return contractError("null nonoptional field")
	}
	if t.Implements(reflect.TypeFor[json.Marshaler]()) {
		return nil
	}
	switch t.Kind() {
	case reflect.Struct:
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(data, &fields); err != nil || fields == nil {
			return contractError("expected object")
		}
		allowed := make(map[string]reflect.Type)
		var addFields func(reflect.Type)
		addFields = func(st reflect.Type) {
			for f := range st.Fields() {
				if f.Anonymous {
					addFields(f.Type)
					continue
				}
				key, _, _ := strings.Cut(f.Tag.Get("json"), ",")
				if key != "" && key != "-" {
					allowed[key] = f.Type
				}
			}
		}
		addFields(t)
		for k, v := range fields {
			ft, ok := allowed[k]
			if !ok {
				return contractError("unknown field " + k)
			}
			if err := wireShape(v, ft); err != nil {
				return err
			}
		}
	case reflect.Slice:
		var values []json.RawMessage
		if err := json.Unmarshal(data, &values); err != nil {
			return contractError("expected array")
		}
		for _, v := range values {
			if err := wireShape(v, t.Elem()); err != nil {
				return err
			}
		}
	}
	return nil
}

// CheckIngress enforces NFR-1 at one frame ingress: the length prefix must
// match the payload, the payload must strictly decode as a Frame (unknown
// keys rejected, envelope validated), and encoded length, JSON nesting
// depth and artefact-reference count must satisfy
// v2contract.CheckFrameLimits. Any violation stops the affected
// integration: the caller closes the connection and accepts no further
// frames on it. Every violation reports the protocol_mismatch code; once
// vocab task 6 lands, violations become *v2contract.ControlError instead of
// plain errors carrying the code in the message.
func CheckIngress(frame []byte) error {
	if err := checkIngress(frame); err != nil {
		return fmt.Errorf("control: protocol_mismatch: %w", err)
	}
	return nil
}

func checkIngress(frame []byte) error {
	// Size first: reject oversize frames before parsing any of them, so
	// enforcement never depends on the caller bounding input beforehand
	// and an oversize frame reports the size error, not a decode error.
	if err := v2contract.CheckFrameLimits(len(frame), 0, 0); err != nil {
		return fmt.Errorf("control: ingress frame rejected: %w", err)
	}
	payload, err := splitPrefix(frame)
	if err != nil {
		return err
	}
	if f, err := decodeWire[Frame](payload); err != nil {
		return fmt.Errorf("control: ingress frame malformed: %w", err)
	} else if err := f.Validate(); err != nil {
		return err
	} else if strings.HasPrefix(f.Method, "attachment.") && len(frame) > AttachmentFrameBytes {
		return contractError("attachment frame exceeds limit")
	} else if err := validateMethodParams(f); err != nil {
		return err
	}
	depth, refs, err := scanPayload(payload)
	if err != nil {
		return fmt.Errorf("control: ingress frame malformed: %w", err)
	}
	if err := v2contract.CheckFrameLimits(len(frame), depth, refs); err != nil {
		return fmt.Errorf("control: ingress frame rejected: %w", err)
	}
	return nil
}

// splitPrefix validates the 4-byte big-endian length prefix and returns the
// payload it delimits.
func splitPrefix(frame []byte) ([]byte, error) {
	if len(frame) < prefixLen {
		return nil, fmt.Errorf("control: frame too short for length prefix: %d bytes", len(frame))
	}
	want := binary.BigEndian.Uint32(frame[:prefixLen])
	payload := frame[prefixLen:]
	if uint64(want) != uint64(len(payload)) {
		return nil, fmt.Errorf("control: frame length prefix %d does not match payload %d bytes", want, len(payload))
	}
	return payload, nil
}

// scanPayload measures the JSON nesting depth and artefact-reference count
// of payload with a streaming decoder, so adversarial nesting cannot
// exhaust the stack. Depth is the maximum number of simultaneously open
// objects/arrays; the top-level object is depth 1. Refs counts every
// artefact-id key with a string value anywhere in the payload.
func scanPayload(payload []byte) (maxDepth, refs int, err error) {
	dec := json.NewDecoder(bytes.NewReader(payload))
	// container tracks one open object/array. For objects, expectKey
	// distinguishes a key string from a string value, and refArmed records
	// an artefact-id key whose value is still to come.
	type container struct {
		object    bool
		expectKey bool
		refArmed  bool
		keys      map[string]bool
	}
	var stack []container
	// finishValue runs after any complete value inside the current
	// container: a string value under an armed key counts as one reference.
	finishValue := func(wasString bool) {
		if len(stack) == 0 {
			return
		}
		top := &stack[len(stack)-1]
		if !top.object {
			return
		}
		if top.refArmed {
			if wasString {
				refs++
			}
			top.refArmed = false
		}
		top.expectKey = true
	}
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return maxDepth, refs, nil
		}
		if err != nil {
			return 0, 0, err
		}
		switch tok := tok.(type) {
		case json.Delim:
			switch tok {
			case '{', '[':
				stack = append(stack, container{object: tok == '{', expectKey: true, keys: make(map[string]bool)})
				if len(stack) > v2contract.MaxNestingDepth {
					return 0, 0, errors.New("JSON nesting depth exceeds limit")
				}
				if len(stack) > maxDepth {
					maxDepth = len(stack)
				}
			default: // '}' or ']'
				if len(stack) == 0 {
					return 0, 0, errors.New("control: unbalanced JSON in frame payload")
				}
				stack = stack[:len(stack)-1]
				finishValue(false)
			}
		case string:
			if len(stack) > 0 && stack[len(stack)-1].object && stack[len(stack)-1].expectKey {
				top := &stack[len(stack)-1]
				if top.keys[tok] {
					return 0, 0, errors.New("duplicate JSON key " + tok)
				}
				top.keys[tok] = true
				top.expectKey = false
				top.refArmed = tok == refKey
			} else {
				finishValue(true)
			}
		default: // float64, bool, nil: scalar values
			finishValue(false)
		}
	}
}

// DecodeParams applies the same closed bounded contract to a handler body.
func DecodeParams[T v2contract.Validator](params json.RawMessage) (T, error) {
	var zero T
	value, err := decodeWire[T](params)
	if err != nil {
		return zero, err
	}
	if err := value.Validate(); err != nil {
		return zero, err
	}
	return value, nil
}
func validateMethodParams(f Frame) error {
	switch f.Method {
	case "run.prepare":
		_, err := DecodeParams[RunSpec](f.Params)
		return err
	case "run.confirm":
		_, err := DecodeParams[RunConfirmation](f.Params)
		return err
	case "run.start":
		_, err := DecodeParams[StartRun](f.Params)
		return err
	case "run.recover":
		_, err := DecodeParams[RecoverRun](f.Params)
		return err
	case "run.apply":
		_, err := DecodeParams[ApplyRun](f.Params)
		return err
	case "run.operation":
		_, err := DecodeParams[OperationQuery](f.Params)
		return err
	case "run.observe":
		_, err := DecodeParams[ObserveRun](f.Params)
		return err
	case "attachment.open":
		_, err := DecodeParams[attachmentOpenParams](f.Params)
		return err
	case "attachment.read", "attachment.close":
		_, err := DecodeParams[attachmentGrantParams](f.Params)
		return err
	case "attachment.stop":
		_, err := DecodeParams[attachmentStopParams](f.Params)
		return err
	case "attachment.operation":
		_, err := DecodeParams[attachmentOperationParams](f.Params)
		return err
	}
	return nil
}
