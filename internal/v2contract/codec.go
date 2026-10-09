package v2contract

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Validator is implemented by every record, the envelope and ControlError.
type Validator interface{ Validate() error }

// Decode strictly decodes canonical JSON into T: unknown fields are
// rejected (naming the key), a syntax error or truncation names its offset,
// trailing data is rejected, and T.Validate is enforced last.
func Decode[T Validator](data []byte) (T, error) {
	var v T
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		var syn *json.SyntaxError
		switch {
		case errors.As(err, &syn):
			return v, fmt.Errorf("v2contract: decode: syntax error at offset %d: %w", syn.Offset, err)
		case errors.Is(err, io.ErrUnexpectedEOF), errors.Is(err, io.EOF):
			return v, fmt.Errorf("v2contract: decode: truncated input at offset %d: %w", len(data), err)
		}
		return v, fmt.Errorf("v2contract: decode: %w", err)
	}
	// A second decode must hit EOF: dec.More() alone misses unmatched
	// trailing delimiters such as an extra "}" or "]".
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return v, fmt.Errorf("v2contract: decode: trailing data at offset %d", dec.InputOffset())
	}
	if err := v.Validate(); err != nil {
		return v, err
	}
	return v, nil
}

// Digest returns "sha256:<hex>" over the canonical JSON encoding of v
// (struct declaration order is deterministic; D5: no field exclusions). It
// returns "" when v cannot be encoded; record Validate methods reject the
// values (non-finite floats) that would cause that, so a validated record
// always has a digest.
func Digest(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}
