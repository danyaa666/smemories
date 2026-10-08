package notes

import (
	"crypto/sha256"
	"strings"
	"testing"
)

func TestNewToken(t *testing.T) {
	seen := map[string]bool{}
	for range 1000 {
		tok, h, err := newToken()
		if err != nil {
			t.Fatal(err)
		}
		if len(tok) != tokenLen || seen[tok] {
			t.Fatalf("token %q: wrong length or repeated", tok)
		}
		seen[tok] = true
		if h != sha256.Sum256([]byte(tok)) {
			t.Fatal("hash is not the SHA-256 of the token")
		}
		if got, ok := tokenHash(tok); !ok || got != h {
			t.Fatalf("tokenHash(%q) = %x, %v", tok, got, ok)
		}
	}
}

func TestTokenHashRejectsGarbage(t *testing.T) {
	good := strings.Repeat("a", tokenLen)
	for _, s := range []string{"", "short", good + "a", good[:31], good[:31] + "=", good[:31] + "/", good[:31] + " ", good[:31] + "é", good[:31] + "\x00", strings.Repeat("x", 10_000)} {
		if _, ok := tokenHash(s); ok {
			t.Errorf("tokenHash(%.20q) accepted", s)
		}
	}
}
