package auth

import (
	"strings"
	"testing"
)

func TestNormalizeEmail(t *testing.T) {
	if got := normalizeEmail("  Alice.Example@Example.COM \n"); got != "alice.example@example.com" {
		t.Fatal(got)
	}
}

func TestValidEmail(t *testing.T) {
	long := strings.Repeat("a", 243) + "@example.com" // 255 chars
	for e, want := range map[string]bool{
		"a@example.com":                           true,
		"first.last+tag@sub.example.vn":           true,
		"đặng@example.vn":                         true,
		strings.Repeat("a", 242) + "@example.com": true, // exactly 254
		long:                                  false,
		"":                                    false,
		"plain":                               false,
		"a@localhost":                         false, // domain needs a dot
		"a@.example.com":                      false,
		"a@example.com.":                      false,
		"@example.com":                        false,
		"Alice <a@example.com>":               false, // display name form
		"a@example.com, b@example.com":        false,
		"a b@example.com":                     false,
		"a@exa mple.com":                      false,
		"(comment)a@example.com":              false,
		"a@example.com\r\nBcc: x@example.com": false,
		"'; DROP TABLE users;--@example.com":  false,
	} {
		if got := validEmail(e); got != want {
			t.Errorf("validEmail(%q) = %v, want %v", e, got, want)
		}
	}
}

func TestValidPassword(t *testing.T) {
	for _, c := range []struct {
		pw, email string
		want      bool
	}{
		{strings.Repeat("a", 10), "x@example.com", true},
		{strings.Repeat("a", 9), "x@example.com", false},
		{strings.Repeat("a", 128), "x@example.com", true},
		{strings.Repeat("a", 129), "x@example.com", false},
		{strings.Repeat("🎓", 10), "x@example.com", true},  // characters, not bytes
		{strings.Repeat("🎓", 128), "x@example.com", true}, // 512 bytes
		{strings.Repeat("🎓", 129), "x@example.com", false},
		{"alice@example.com", "alice@example.com", false},
		{"Alice@Example.com", "alice@example.com", false},
		{"", "x@example.com", false},
		{strings.Repeat("a", 1<<20), "x@example.com", false},
	} {
		if got := validPassword(c.pw, c.email); got != c.want {
			t.Errorf("validPassword(len %d, %q) = %v, want %v", len(c.pw), c.email, got, c.want)
		}
	}
}

func TestCleanDisplayName(t *testing.T) {
	for in, want := range map[string]string{
		"  Đặng Thị Hồng 🎓 ":     "Đặng Thị Hồng 🎓",
		"A":                      "A",
		strings.Repeat("x", 100): strings.Repeat("x", 100),
		strings.Repeat("đ", 100): strings.Repeat("đ", 100),
		"":                       "",
		"   ":                    "",
		strings.Repeat("x", 101): "",
		"line\nbreak":            "",
		"tab\there":              "",
		"nul\x00byte":            "",
		"esc\x1b[31m":            "",
		"del\x7f":                "",
		"bad\xffutf8":            "",
	} {
		got, ok := cleanDisplayName(in)
		if got != want || ok != (want != "") {
			t.Errorf("cleanDisplayName(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
}

// Escapes keep the NFC/NFD forms visible: "Việt" typed precomposed vs. base letter + combining marks.
const (
	nfcViet = "Việt"   // ệ precomposed
	nfdViet = "Việt" // e + dot below + circumflex
)

func TestNormalizePasswordNFKC(t *testing.T) {
	if nfcViet == nfdViet {
		t.Fatal("fixture forms must differ in bytes")
	}
	if normalizePassword(nfdViet) != nfcViet || normalizePassword(nfcViet) != nfcViet {
		t.Errorf("NFC/NFD not unified: %q %q", normalizePassword(nfdViet), normalizePassword(nfcViet))
	}
	if got := normalizePassword("１２３abc"); got != "123abc" { // full-width digits
		t.Errorf("NFKC compat fold: %q", got)
	}
}

func TestValidPasswordCountsAfterNormalisation(t *testing.T) {
	nfd := strings.Repeat("ệ", 5) // 15 runes decomposed, 5 after NFKC
	if validPassword(normalizePassword(nfd), "x@example.com") {
		t.Error("5 characters after normalisation must be too short")
	}
	nfd = strings.Repeat("ệ", 10) // 30 runes decomposed, 10 after NFKC
	if !validPassword(normalizePassword(nfd), "x@example.com") {
		t.Error("10 characters after normalisation must pass")
	}
	if validPassword(normalizePassword(strings.Repeat("ệ", 129)), "x@example.com") {
		t.Error("129 characters after normalisation must be too long")
	}
	// 140 decomposed runes that normalise to 70: within the maximum once normalised.
	if !validPassword(normalizePassword(strings.Repeat("ẹ", 70)), "x@example.com") {
		t.Error("limits count characters after normalisation")
	}
}

func TestCleanDisplayNameIsNFC(t *testing.T) {
	got, ok := cleanDisplayName("  Ng" + "ử" + "ơi  ") // decomposed
	want := "Ngửơi"
	if !ok || got != want {
		t.Errorf("got %q %v, want %q", got, ok, want)
	}
	if got, _ := cleanDisplayName("Việt"); got != nfcViet {
		t.Errorf("display name not NFC: %q", got)
	}
}
