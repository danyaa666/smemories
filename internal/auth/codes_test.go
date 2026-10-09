package auth

import (
	"context"
	"strings"
	"testing"
)

// AC1: codes are always 6 digits, leading zeros kept, and the digits are not stuck.
func TestNewCodeShape(t *testing.T) {
	seenLeadingZero := false
	seen := map[byte]bool{}
	for i := 0; i < 3000; i++ {
		c, err := newCode()
		if err != nil || !validCodeFormat(c) {
			t.Fatalf("bad code %q, %v", c, err)
		}
		seenLeadingZero = seenLeadingZero || c[0] == '0'
		for j := 0; j < len(c); j++ {
			seen[c[j]] = true
		}
	}
	if !seenLeadingZero || len(seen) != 10 {
		t.Fatalf("leading zero seen %v, distinct digits %d", seenLeadingZero, len(seen))
	}
}

func TestValidCodeFormat(t *testing.T) {
	for in, want := range map[string]bool{
		"123456": true, "000000": true, "": false, "12345": false, "1234567": false,
		"12345a": false, " 12345": false, "١٢٣٤٥٦": false, "12345\n": false,
	} {
		if got := validCodeFormat(in); got != want {
			t.Errorf("%q: %v, want %v", in, got, want)
		}
	}
}

// AC1: the HMAC depends on key, purpose, user and code, and nothing else.
func TestMacBindsPurposeUserAndKey(t *testing.T) {
	a := &Codes{key: []byte(strings.Repeat("a", 32))}
	b := &Codes{key: []byte(strings.Repeat("b", 32))}
	base := a.mac(purposeVerify, 7, "123456")
	if len(base) != 64 || base != a.mac(purposeVerify, 7, "123456") {
		t.Fatalf("not a stable sha256 hex: %q", base)
	}
	for name, other := range map[string]string{
		"purpose": a.mac(purposeReset, 7, "123456"), "user": a.mac(purposeVerify, 8, "123456"),
		"code": a.mac(purposeVerify, 7, "123457"), "key": b.mac(purposeVerify, 7, "123456"),
	} {
		if other == base {
			t.Errorf("changing the %s does not change the HMAC", name)
		}
	}
}

// AC2/AC7: NewCodes refuses a short key and the fixed code outside dev and test before touching Redis.
func TestNewCodesGuards(t *testing.T) {
	key := []byte(strings.Repeat("k", MinOTPKeyBytes))
	if _, err := NewCodes(context.Background(), nil, "dev", key[:31], ""); err == nil {
		t.Error("31-byte key accepted")
	}
	for _, env := range []string{"prod", "production", "", "staging"} {
		if _, err := NewCodes(context.Background(), nil, env, key, "123123"); err == nil {
			t.Errorf("SMEM_ENV=%q accepted the fixed code", env)
		}
	}
}
