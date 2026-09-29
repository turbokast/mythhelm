package ids

import (
	"bytes"
	"regexp"
	"slices"
	"testing"
	"time"
)

var idPattern = regexp.MustCompile(`^run_[0-9A-HJKMNP-TV-Z]{26}$`)

func TestIDsSortByTime(t *testing.T) {
	base := time.UnixMilli(1469918176385)
	offsets := []time.Duration{0, time.Millisecond, 2 * time.Millisecond, time.Second, time.Hour, 24 * 365 * time.Hour}
	var generated []string
	for _, d := range offsets {
		// Maximal randomness at the earlier instant and minimal at the later
		// one: the time prefix alone must decide the order.
		earlier := newAt("run", base.Add(d), bytes.NewReader(bytes.Repeat([]byte{0xff}, 10)))
		later := newAt("run", base.Add(d+time.Millisecond), bytes.NewReader(make([]byte, 10)))
		generated = append(generated, earlier, later)
	}
	if !slices.IsSorted(generated) {
		t.Fatalf("IDs generated in time order are not sorted:\n%v", generated)
	}
}

func TestNewFormat(t *testing.T) {
	id := New("run")
	if !idPattern.MatchString(id) {
		t.Fatalf("New(\"run\") = %q, want run_ followed by 26 Crockford base32 characters", id)
	}
	if other := New("run"); other == id {
		t.Fatalf("two calls returned the same ID %q", id)
	}
}

func TestEncodingMatchesULID(t *testing.T) {
	// Reference values from the ULID specification: the timestamp
	// 1469918176385 encodes as 01ARYZ6S41, and all-ones randomness as sixteen
	// Zs.
	id := newAt("evt", time.UnixMilli(1469918176385), bytes.NewReader(bytes.Repeat([]byte{0xff}, 10)))
	if want := "evt_01ARYZ6S41ZZZZZZZZZZZZZZZZ"; id != want {
		t.Fatalf("newAt = %q, want %q", id, want)
	}
	zero := newAt("att", time.UnixMilli(0), bytes.NewReader(make([]byte, 10)))
	if want := "att_00000000000000000000000000"; zero != want {
		t.Fatalf("newAt(epoch, zero entropy) = %q, want %q", zero, want)
	}
}
