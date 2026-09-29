// Package ids generates prefixed, time-sortable identifiers in the ULID
// format (decision D3): a 48-bit millisecond timestamp and 80 random bits
// from crypto/rand, encoded as 26 Crockford base32 characters.
package ids

import (
	"crypto/rand"
	"encoding/binary"
	"io"
	"time"
)

const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// New returns prefix + "_" + a new ULID-format identifier, for example
// New("run") = "run_01K6...". IDs made in later milliseconds sort after
// earlier ones; within one millisecond their order is random.
func New(prefix string) string {
	return newAt(prefix, time.Now(), rand.Reader)
}

func newAt(prefix string, t time.Time, entropy io.Reader) string {
	var b [16]byte
	ms := uint64(t.UnixMilli())
	b[0], b[1], b[2] = byte(ms>>40), byte(ms>>32), byte(ms>>24)
	b[3], b[4], b[5] = byte(ms>>16), byte(ms>>8), byte(ms)
	if _, err := io.ReadFull(entropy, b[6:]); err != nil {
		// crypto/rand never fails on supported platforms; a short read here
		// would silently weaken uniqueness.
		panic("ids: reading entropy: " + err.Error())
	}
	hi, lo := binary.BigEndian.Uint64(b[:8]), binary.BigEndian.Uint64(b[8:])

	out := make([]byte, 0, len(prefix)+1+26)
	out = append(out, prefix...)
	out = append(out, '_')
	// 26 characters carry 130 bits; the first character holds the top 3 bits.
	for shift := 125; shift >= 0; shift -= 5 {
		out = append(out, crockford[bits5(hi, lo, uint(shift))])
	}
	return string(out)
}

// bits5 returns the 5 bits of the 128-bit value hi:lo starting at bit shift,
// counted from the least significant bit.
func bits5(hi, lo uint64, shift uint) uint64 {
	switch {
	case shift >= 64:
		return (hi >> (shift - 64)) & 31
	case shift+5 <= 64:
		return (lo >> shift) & 31
	default:
		return ((lo >> shift) | (hi << (64 - shift))) & 31
	}
}
