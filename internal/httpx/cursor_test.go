package httpx

import (
	"strings"
	"testing"

	"github.com/danyaa666/smemories/internal/apperr"
)

func TestCursors(t *testing.T) {
	c := NewCursors([]byte(strings.Repeat("k", 32)))
	tok := c.Seal("media:A", "12345")
	if strings.Contains(tok, "12345") || tok == c.Seal("media:A", "12345") {
		t.Fatalf("token must hide the position and differ per call: %q", tok)
	}
	if pos, err := c.Open("media:A", tok); err != nil || pos != "12345" {
		t.Fatalf("round trip: %q, %v", pos, err)
	}

	flipped := []byte(tok)
	flipped[len(flipped)/2] ^= 1 // still base64, different bytes
	for name, bad := range map[string]string{
		"empty": "", "garbage": "!!!", "short": tok[:10], "tampered": string(flipped), "trailing": tok + "A",
	} {
		if _, err := c.Open("media:A", bad); !apperr.Is(err, apperr.Param) {
			t.Errorf("%s: err %v, want ERROR_PARAM", name, err)
		}
	}
	if _, err := c.Open("media:B", tok); !apperr.Is(err, apperr.Param) {
		t.Errorf("token of another scope: err %v, want ERROR_PARAM", err)
	}
	if _, err := NewCursors([]byte(strings.Repeat("x", 32))).Open("media:A", tok); !apperr.Is(err, apperr.Param) {
		t.Errorf("token of another secret: err %v, want ERROR_PARAM", err)
	}
}
