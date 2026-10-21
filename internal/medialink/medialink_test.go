package medialink

import (
	"bytes"
	"strings"
	"testing"
)

const (
	media = "01JABCDEFGHJKMNPQRSTVWXYZ0"
	book  = "01JZZZZZZZZZZZZZZZZZZZZZZZ"
)

func signer(t *testing.T) *Signer {
	t.Helper()
	s, ok := New(bytes.Repeat([]byte{7}, MinSecretBytes))
	if !ok {
		t.Fatal("New refused a 32-byte secret")
	}
	return s
}

func TestRoundTrip(t *testing.T) {
	s := signer(t)
	tok := s.Token(media, book, 3)
	if len(tok) != 49 {
		t.Fatalf("token length %d, want 49", len(tok))
	}
	id, mac, ok := Parse(tok)
	if !ok || id != media {
		t.Fatalf("Parse: %q %v", id, ok)
	}
	if !s.Verify(id, mac, book, 3) {
		t.Fatal("a fresh token does not verify")
	}
}

func TestWrongInputsDoNotVerify(t *testing.T) {
	s := signer(t)
	_, mac, _ := Parse(s.Token(media, book, 3))
	other, _ := New(bytes.Repeat([]byte{8}, MinSecretBytes))
	for name, ok := range map[string]bool{
		"wrong epoch (revoked)": s.Verify(media, mac, book, 4),
		"epoch 0":               s.Verify(media, mac, book, 0),
		"wrong yearbook":        s.Verify(media, mac, "01JZZZZZZZZZZZZZZZZZZZZZZY", 3),
		"wrong media id":        s.Verify("01JABCDEFGHJKMNPQRSTVWXYZ1", mac, book, 3),
		"wrong secret":          other.Verify(media, mac, book, 3),
		"empty mac":             s.Verify(media, nil, book, 3),
		"short mac":             s.Verify(media, mac[:15], book, 3),
	} {
		if ok {
			t.Errorf("%s verifies", name)
		}
	}
}

// Every character of the MAC part: changing it must never produce a token that parses and verifies.
func TestEveryMACPositionIsChecked(t *testing.T) {
	s := signer(t)
	tok := s.Token(media, book, 1)
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	for i := idLen + 1; i < len(tok); i++ {
		for _, c := range []byte(alphabet) {
			if c == tok[i] {
				continue
			}
			bad := tok[:i] + string(c) + tok[i+1:]
			if id, mac, ok := Parse(bad); ok && s.Verify(id, mac, book, 1) {
				t.Fatalf("position %d -> %q still verifies", i, c)
			}
		}
	}
}

func TestParseRefusesOtherShapes(t *testing.T) {
	good := signer(t).Token(media, book, 1)
	mac := good[idLen+1:]
	for name, tok := range map[string]string{
		"empty":             "",
		"no dot":            media + mac,
		"id only":           media + ".",
		"truncated":         good[:len(good)-1],
		"extra char":        good + "A",
		"oversized":         good + strings.Repeat("A", 500),
		"lower case id":     strings.ToLower(media) + "." + mac,
		"id with U":         "01JABCDEFGHJKMNPQRSTVWXYZU" + "." + mac,
		"short id":          media[:25] + "." + mac,
		"dot in mac":        media + "." + mac[:10] + "." + mac[11:],
		"padded mac":        media + "." + mac + "==",
		"standard alphabet": media + "." + strings.Repeat("+", 22),
		"two dots":          media + "." + mac + "." + mac,
		"nul":               media + ".\x00" + mac[1:],
	} {
		if _, _, ok := Parse(tok); ok {
			t.Errorf("%s: parsed", name)
		}
	}
}

// 16 bytes leave 4 spare bits in the 22nd character; a spelling that sets them is refused (one valid string per MAC).
func TestNonCanonicalMACSpellingIsRefused(t *testing.T) {
	good := signer(t).Token(media, book, 1)
	last := strings.IndexByte("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_", good[len(good)-1])
	alt := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"[last^1] // flips a spare bit
	if _, _, ok := Parse(good[:len(good)-1] + string(alt)); ok {
		t.Fatal("a non-canonical spelling of the MAC parsed")
	}
}

func TestShortSecretIsRefused(t *testing.T) {
	if _, ok := New(make([]byte, MinSecretBytes-1)); ok {
		t.Fatal("31-byte secret accepted")
	}
}

func TestSecretIsCopied(t *testing.T) {
	key := bytes.Repeat([]byte{7}, MinSecretBytes)
	s, _ := New(key)
	before := s.Token(media, book, 1)
	key[0] = 0
	if s.Token(media, book, 1) != before {
		t.Fatal("changing the caller's slice changed the signer")
	}
}
