// Package ulid makes opaque, time-sortable public ids (L-05): 26 Crockford base32
// characters = 48-bit millisecond timestamp + 80 random bits.
package ulid

import (
	"crypto/rand"
	"strings"
	"time"
)

const (
	alphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
	idLen    = 26 // characters in a ULID
)

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
	var out [idLen]byte
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

// Valid reports whether s is a canonical ULID: exactly 26 upper-case Crockford base32 characters (as New writes them).
// The public_id columns compare case-insensitively (utf8mb4_0900_ai_ci), so any other spelling of an id would match its
// row; every edge that takes an id from a client checks Valid first, and object keys are built from the stored id.
func Valid(s string) bool {
	if len(s) != idLen {
		return false
	}
	for i := 0; i < len(s); i++ {
		if strings.IndexByte(alphabet, s[i]) < 0 {
			return false
		}
	}
	return true
}
