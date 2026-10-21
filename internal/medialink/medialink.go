// Package medialink makes and checks the bearer tokens of public photo links (T-092, decision D-32).
//
// token = <media public id> "." base64url(first 16 bytes of HMAC-SHA256(secret, "pm1|" media id "|" yearbook id "|" link epoch)).
// Everything here is pure: no database, no clock. The caller loads the media row and the yearbook's current link_epoch and
// asks Verify. Bumping the epoch (yearbook/revoke-photo-links) changes every MAC at once, so older links stop verifying.
//
// A token is a credential: callers never log it, never put it in an error, never use it as a label.
package medialink

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strconv"

	"github.com/danyaa666/smemories/internal/ulid"
)

const (
	// MinSecretBytes is the smallest accepted secret (decoded).
	MinSecretBytes = 32
	// MaxTokenLen is the longest string Parse looks at; a real token is 49 characters.
	MaxTokenLen = 128

	macBytes = 16 // 128 bits of the HMAC
	idLen    = 26 // a ULID
	version  = "pm1|"
)

// b64 is strict: a final character with non-zero spare bits is refused, so a MAC has exactly one valid spelling.
var b64 = base64.RawURLEncoding.Strict()

// Signer holds the secret.
type Signer struct{ key []byte }

// New returns a Signer for secret, which must be at least MinSecretBytes long. The slice is copied.
func New(secret []byte) (*Signer, bool) {
	if len(secret) < MinSecretBytes {
		return nil, false
	}
	return &Signer{key: append([]byte(nil), secret...)}, true
}

func (s *Signer) mac(mediaID, yearbookID string, epoch uint32) []byte {
	h := hmac.New(sha256.New, s.key)
	h.Write([]byte(version + mediaID + "|" + yearbookID + "|" + strconv.FormatUint(uint64(epoch), 10)))
	return h.Sum(nil)[:macBytes]
}

// Token returns the link token of a photo of the yearbook, valid while the yearbook's link epoch is epoch.
func (s *Signer) Token(mediaID, yearbookID string, epoch uint32) string {
	return mediaID + "." + b64.EncodeToString(s.mac(mediaID, yearbookID, epoch))
}

// Parse splits a token into the media id and the MAC it claims. ok is false for anything that is not exactly
// <26-character ULID>.<22 base64url characters of 16 bytes>: that is the only shape Verify is ever asked about.
func Parse(token string) (mediaID string, mac []byte, ok bool) {
	if len(token) > MaxTokenLen || len(token) <= idLen+1 || token[idLen] != '.' {
		return "", nil, false
	}
	mediaID = token[:idLen]
	if !ulid.Valid(mediaID) {
		return "", nil, false
	}
	mac, err := b64.DecodeString(token[idLen+1:])
	if err != nil || len(mac) != macBytes {
		return "", nil, false
	}
	return mediaID, mac, true
}

// Verify reports whether mac is the MAC of this photo, yearbook and epoch. The comparison takes the same time wherever
// the two values differ (hmac.Equal).
func (s *Signer) Verify(mediaID string, mac []byte, yearbookID string, epoch uint32) bool {
	return hmac.Equal(mac, s.mac(mediaID, yearbookID, epoch))
}
