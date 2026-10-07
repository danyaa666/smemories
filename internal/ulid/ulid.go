// Package ulid makes opaque, time-sortable public ids (L-05): 26 Crockford base32
// characters = 48-bit millisecond timestamp + 80 random bits.
package ulid

import (
	"crypto/rand"
	"time"
)

const alphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// New returns a ULID for t. It is not monotonic within one millisecond (the random part
// differs), which is fine for ids that are only ever looked up, never ordered by.
func New(t time.Time) string {
	var b [16]byte
	ms := t.UnixMilli()
	if ms < 0 { // before 1970 has no ULID; clamp to the epoch
		ms = 0
	}
	msu := uint64(ms) //nolint:gosec // G115: ms >= 0 checked above
	for i := 5; i >= 0; i-- {
		b[i] = byte(msu)
		msu >>= 8
	}
	if _, err := rand.Read(b[6:]); err != nil {
		panic("ulid: crypto/rand failed: " + err.Error()) // never happens on supported platforms
	}
	// 128 bits -> 26 groups of 5 bits (the first group holds only 3 bits).
	var out [26]byte
	var acc uint
	var bits, n int
	for i := 15; i >= 0; i-- {
		acc |= uint(b[i]) << bits
		bits += 8
		for bits >= 5 {
			out[25-n] = alphabet[acc&31]
			acc >>= 5
			bits -= 5
			n++
		}
	}
	out[0] = alphabet[acc&31]
	return string(out[:])
}
