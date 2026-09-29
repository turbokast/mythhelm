package ndjson

import (
	"bytes"
	"errors"
	"io"
	"runtime"
	"strings"
	"testing"
	"testing/iotest"
)

// readAll returns every frame the reader yields, copied, and the reader's
// final counters.
func readAll(t *testing.T, r io.Reader, lim Limits) ([]string, Counters) {
	t.Helper()
	rd := NewReader(r, lim)
	var frames []string
	for {
		frame, err := rd.Next()
		if errors.Is(err, io.EOF) {
			return frames, rd.Counters()
		}
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		frames = append(frames, string(frame))
	}
}

func TestReaderSplitsFrames(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{name: "newline terminated", input: "{\"a\":1}\n{\"b\":2}\n", want: []string{`{"a":1}`, `{"b":2}`}},
		{name: "last frame without newline", input: "{\"a\":1}\n{\"b\":2}", want: []string{`{"a":1}`, `{"b":2}`}},
		{name: "blank lines skipped", input: "\n{\"a\":1}\n\n", want: []string{`{"a":1}`}},
		{name: "empty input", input: "", want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// One byte per Read exercises frames that span many reads.
			frames, c := readAll(t, iotest.OneByteReader(strings.NewReader(tt.input)), Limits{MaxFrame: 16, MaxDepth: 4})
			if strings.Join(frames, "|") != strings.Join(tt.want, "|") {
				t.Errorf("frames = %q, want %q", frames, tt.want)
			}
			if c.Frames != int64(len(tt.want)) || c.Oversized+c.InvalidUTF8+c.DepthExceeded != 0 {
				t.Errorf("counters = %+v, want %d frames and nothing dropped", c, len(tt.want))
			}
		})
	}
}

func TestReaderOversizedFrameDiscarded(t *testing.T) {
	const limit = 1 << 20
	lim := Limits{MaxFrame: limit, MaxDepth: 64}

	exact := `"` + strings.Repeat("x", limit-2) + `"`
	input := "{\"before\":1}\n" + strings.Repeat("y", 8*limit) + "\n" + exact + "\n{\"after\":1}\n"
	frames, c := readAll(t, strings.NewReader(input), lim)
	if want := []string{`{"before":1}`, exact, `{"after":1}`}; strings.Join(frames, "|") != strings.Join(want, "|") {
		t.Errorf("got %d frames; want the frames around the oversized one, and the frame of exactly the limit", len(frames))
	}
	if c.Oversized != 1 || c.Frames != 3 {
		t.Errorf("counters = %+v, want Oversized=1 Frames=3", c)
	}

	// Memory stays bounded: discarding a frame eight times the limit must
	// not allocate in proportion to it. The whole read, including the
	// reader itself, allocates less than twice the limit, in few objects.
	oversized := strings.Repeat("y", 8*limit) + "\n{\"after\":1}\n"
	read := func() {
		rd := NewReader(strings.NewReader(oversized), lim)
		for {
			if _, err := rd.Next(); err != nil {
				return
			}
		}
	}
	if allocs := testing.AllocsPerRun(5, read); allocs > 16 {
		t.Errorf("AllocsPerRun = %v objects, want at most 16", allocs)
	}
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	read()
	runtime.ReadMemStats(&after)
	if got := after.TotalAlloc - before.TotalAlloc; got >= 2*limit {
		t.Errorf("allocated %d bytes reading an oversized frame, want < 2× the %d-byte limit", got, limit)
	}
}

func TestReaderOversizedFinalFrameWithoutNewline(t *testing.T) {
	frames, c := readAll(t, strings.NewReader("{\"a\":1}\n"+strings.Repeat("z", 40)), Limits{MaxFrame: 16, MaxDepth: 4})
	if len(frames) != 1 || c.Oversized != 1 {
		t.Errorf("frames = %q, counters = %+v; want 1 frame and Oversized=1", frames, c)
	}
}

func TestReaderDepthLimit(t *testing.T) {
	lim := Limits{MaxFrame: 1 << 10, MaxDepth: 3}
	tests := []struct {
		name  string
		frame string
		keep  bool
	}{
		{name: "at the limit", frame: `{"a":[{"b":1}]}`, keep: true},
		{name: "over the limit", frame: `{"a":[{"b":[1]}]}`, keep: false},
		{name: "brackets inside strings do not count", frame: `{"a":"[[[[{{{{"}`, keep: true},
		{name: "escaped quote keeps the string open", frame: `{"a":"\"[[[[["}`, keep: true},
		{name: "escaped backslash closes the string", frame: `{"a":"\\","b":[[[1]]]}`, keep: false},
		{name: "depth returns after closing", frame: `[[[1]],[[2]],[[3]]]`, keep: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frames, c := readAll(t, strings.NewReader(tt.frame+"\n{}\n"), lim)
			kept := len(frames) == 2 && frames[0] == tt.frame
			if kept != tt.keep {
				t.Errorf("frames = %q; kept = %v, want %v", frames, kept, tt.keep)
			}
			if wantDropped := int64(map[bool]int{true: 0, false: 1}[tt.keep]); c.DepthExceeded != wantDropped {
				t.Errorf("DepthExceeded = %d, want %d", c.DepthExceeded, wantDropped)
			}
		})
	}
}

func TestReaderInvalidUTF8Counted(t *testing.T) {
	input := []byte("{\"a\":\"ok ✓\"}\n{\"a\":\"\xff\xfe\"}\n{\"a\":\"\xc3\"}\n{\"b\":1}\n")
	frames, c := readAll(t, bytes.NewReader(input), Limits{MaxFrame: 64, MaxDepth: 4})
	if want := []string{`{"a":"ok ✓"}`, `{"b":1}`}; strings.Join(frames, "|") != strings.Join(want, "|") {
		t.Errorf("frames = %q, want %q", frames, want)
	}
	if c.InvalidUTF8 != 2 || c.Frames != 2 {
		t.Errorf("counters = %+v, want InvalidUTF8=2 Frames=2", c)
	}
}

func TestReaderZeroLimitsUseDefaults(t *testing.T) {
	deep := strings.Repeat("[", DefaultMaxDepth+1) + strings.Repeat("]", DefaultMaxDepth+1)
	ok := strings.Repeat("[", DefaultMaxDepth) + strings.Repeat("]", DefaultMaxDepth)
	frames, c := readAll(t, strings.NewReader(deep+"\n"+ok+"\n"), Limits{})
	if len(frames) != 1 || frames[0] != ok || c.DepthExceeded != 1 {
		t.Errorf("frames = %d, counters = %+v; want the depth-%d frame kept and the deeper one dropped", len(frames), c, DefaultMaxDepth)
	}
}

func TestReaderReturnsReadErrors(t *testing.T) {
	boom := errors.New("boom")
	rd := NewReader(io.MultiReader(strings.NewReader("{}\n"), iotest.ErrReader(boom)), Limits{})
	if frame, err := rd.Next(); err != nil || string(frame) != "{}" {
		t.Fatalf("first Next = %q, %v; want {} and no error", frame, err)
	}
	if _, err := rd.Next(); !errors.Is(err, boom) {
		t.Errorf("second Next error = %v, want %v", err, boom)
	}
}
