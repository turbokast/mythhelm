// Package ndjson reads newline-delimited JSON frames from untrusted native
// output with bounded memory (§9.7, NFR-1).
package ndjson

import (
	"bufio"
	"errors"
	"io"
	"unicode/utf8"
)

// Default limits for native stdout (design §6.4).
const (
	DefaultMaxFrame = 16 << 20
	DefaultMaxDepth = 64
)

// readBufferSize is the most the reader buffers ahead of the current frame.
const readBufferSize = 64 << 10

// Limits bounds each frame. A zero field takes its default.
type Limits struct {
	MaxFrame int // bytes, excluding the newline
	MaxDepth int // JSON object and array nesting, outside strings
}

// Counters counts the frames a Reader returned and the frames it dropped.
// JSON decoding happens after the reader, so malformed frames are counted
// by the decoder, not here.
type Counters struct {
	Frames        int64 `json:"frames"`
	Oversized     int64 `json:"oversized"`
	InvalidUTF8   int64 `json:"invalid_utf8"`
	DepthExceeded int64 `json:"depth_exceeded"`
}

// Reader splits a stream into frames on '\n'. It drops, and counts, frames
// longer than MaxFrame, frames that are not valid UTF-8 and frames nested
// deeper than MaxDepth. Blank lines are skipped. It never holds more than
// one frame of at most MaxFrame bytes: an oversized frame is discarded up
// to its newline as it streams past.
type Reader struct {
	br  *bufio.Reader
	lim Limits
	buf []byte // the frame being assembled when it spans reads; reused
	n   Counters
}

// NewReader returns a Reader over r.
func NewReader(r io.Reader, lim Limits) *Reader {
	if lim.MaxFrame <= 0 {
		lim.MaxFrame = DefaultMaxFrame
	}
	if lim.MaxDepth <= 0 {
		lim.MaxDepth = DefaultMaxDepth
	}
	return &Reader{br: bufio.NewReaderSize(r, min(readBufferSize, lim.MaxFrame+1)), lim: lim}
}

// Next returns the next frame without its newline. The frame is valid only
// until the next call. At the end of the stream it returns io.EOF; any
// other error is the underlying reader's.
func (r *Reader) Next() ([]byte, error) {
	for {
		frame, oversized, err := r.readFrame()
		if err != nil {
			return nil, err
		}
		switch {
		case oversized:
			r.n.Oversized++
		case len(frame) == 0:
		case !utf8.Valid(frame):
			r.n.InvalidUTF8++
		case depthExceeds(frame, r.lim.MaxDepth):
			r.n.DepthExceeded++
		default:
			r.n.Frames++
			return frame, nil
		}
	}
}

// Counters returns the counts so far.
func (r *Reader) Counters() Counters { return r.n }

// readFrame reads up to the next newline or the end of the stream. A frame
// that fits in the read buffer is returned without copying.
func (r *Reader) readFrame() (frame []byte, oversized bool, err error) {
	r.buf = r.buf[:0]
	for {
		chunk, err := r.br.ReadSlice('\n')
		complete := err == nil
		if complete {
			chunk = chunk[:len(chunk)-1]
		}
		switch {
		case oversized:
		case len(r.buf)+len(chunk) > r.lim.MaxFrame:
			oversized = true
		case complete && len(r.buf) == 0:
			return chunk, false, nil
		default:
			r.appendChunk(chunk)
		}
		switch {
		case complete:
			return r.buf, oversized, nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, io.EOF) && (oversized || len(r.buf) > 0):
			return r.buf, oversized, nil
		default:
			return nil, false, err
		}
	}
}

// appendChunk grows the frame buffer fourfold rather than letting append
// choose, so that assembling a frame of the maximum size allocates at most
// about 4/3 of it in total.
func (r *Reader) appendChunk(p []byte) {
	if need := len(r.buf) + len(p); need > cap(r.buf) {
		grown := make([]byte, len(r.buf), min(max(4*cap(r.buf), need), r.lim.MaxFrame))
		copy(grown, r.buf)
		r.buf = grown
	}
	r.buf = append(r.buf, p...)
}

// depthExceeds reports whether frame nests objects or arrays deeper than
// limit, ignoring brackets inside strings. It does not validate JSON.
func depthExceeds(frame []byte, limit int) bool {
	depth := 0
	inString, escaped := false, false
	for _, c := range frame {
		switch {
		case escaped:
			escaped = false
		case inString:
			switch c {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
		case c == '"':
			inString = true
		case c == '{' || c == '[':
			depth++
			if depth > limit {
				return true
			}
		case c == '}' || c == ']':
			depth--
		}
	}
	return false
}
