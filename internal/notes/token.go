// Package notes is the friends' notes feature: collection links (T-012), later public submission (T-034) and moderation (T-013).
package notes

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

const tokenLen = 32 // base64url of 24 random bytes (192 bits), no padding

// newToken returns a fresh link token and the SHA-256 that is stored in its place.
func newToken() (token string, hash [32]byte, err error) {
	var b [24]byte
	if _, err = rand.Read(b[:]); err != nil {
		return "", hash, err
	}
	token = base64.RawURLEncoding.EncodeToString(b[:])
	return token, sha256.Sum256([]byte(token)), nil
}

// tokenHash reports the stored hash for a token from a URL; ok is false when s cannot be one of ours
// (wrong length or alphabet), so garbage never reaches the database.
func tokenHash(s string) (h [32]byte, ok bool) {
	if len(s) != tokenLen {
		return h, false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '-' && c != '_' {
			return h, false
		}
	}
	return sha256.Sum256([]byte(s)), true
}
