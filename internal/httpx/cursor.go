package httpx

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"

	"github.com/danyaa666/smemories/internal/apperr"
)

// cursorKeyLabel separates the cursor key from every other use of the secret it is derived from.
const cursorKeyLabel = "smemories/cursor/v1"

// Cursors makes the opaque next_id tokens of the keyset-paginated list endpoints (docs/api-contract.md section 5).
// A token is an authenticated encryption (AES-256-GCM) of the position, so a client can neither read the internal
// id inside nor change it, and the scope is bound as associated data, so a token taken from another list (another
// yearbook, another endpoint) is rejected too. The same helper serves every list endpoint.
type Cursors struct{ aead cipher.AEAD }

// NewCursors derives the cursor key from secret (the API's existing HMAC secret, so no new setting is needed).
// Tokens do not survive a change of the secret: clients simply start the list again.
func NewCursors(secret []byte) *Cursors {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(cursorKeyLabel))
	block, err := aes.NewCipher(mac.Sum(nil)) // 32 bytes: AES-256
	if err != nil {
		panic(err) // unreachable: the key length is fixed
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		panic(err) // unreachable
	}
	return &Cursors{aead}
}

// Seal returns the token for position pos of the list identified by scope (for example "media/get-list:"+yearbookID).
func (c *Cursors) Seal(scope, pos string) string {
	nonce := make([]byte, c.aead.NonceSize())
	_, _ = rand.Read(nonce) // never fails on supported platforms
	return base64.RawURLEncoding.EncodeToString(c.aead.Seal(nonce, nonce, []byte(pos), []byte(scope)))
}

// Open returns the position inside token. A malformed, tampered or foreign (other scope) token is ERROR_PARAM.
func (c *Cursors) Open(scope, token string) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	n := c.aead.NonceSize()
	if err != nil || len(raw) < n+c.aead.Overhead() {
		return "", apperr.New(apperr.Param, "next_id is not a valid page token")
	}
	pos, err := c.aead.Open(nil, raw[:n], raw[n:], []byte(scope))
	if err != nil {
		return "", apperr.New(apperr.Param, "next_id is not a valid page token")
	}
	return string(pos), nil
}
