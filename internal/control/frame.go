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
// rejected, then Validate. It performs no limit enforcement; ingress
// points call CheckIngress instead.
func Decode[T v2contract.Validator](frame []byte) (T, error) {
	var zero T
	payload, err := splitPrefix(frame)
	if err != nil {
		return zero, err
	}
	v, err := v2contract.Decode[T](payload)
	if err != nil {
		return zero, fmt.Errorf("control: decode frame: %w", err)
	}
	return v, nil
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
	if _, err := v2contract.Decode[Frame](payload); err != nil {
		return fmt.Errorf("control: ingress frame malformed: %w", err)
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
				stack = append(stack, container{object: tok == '{', expectKey: true})
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
