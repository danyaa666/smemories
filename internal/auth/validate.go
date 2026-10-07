package auth

import (
	"net/mail"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

const (
	maxEmailLen       = 254
	minPasswordLen    = 10
	maxPasswordLen    = 128
	maxDisplayNameLen = 100
)

// Validation error codes returned in the error envelope.
const (
	codeInvalidEmail       = "invalid_email"
	codeWeakPassword       = "weak_password"
	codeInvalidDisplayName = "invalid_display_name"
)

// normalizeEmail trims and lower-cases an address; the result is what is stored and looked up.
func normalizeEmail(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// validEmail reports whether an already normalised address is acceptable: at most 254
// characters, a bare addr-spec that net/mail parses (no display name, no comments), and
// a domain containing a dot.
func validEmail(e string) bool {
	if e == "" || len(e) > maxEmailLen {
		return false
	}
	a, err := mail.ParseAddress(e)
	if err != nil || a.Name != "" || a.Address != e {
		return false
	}
	at := strings.LastIndexByte(e, '@')
	domain := e[at+1:]
	return at > 0 && strings.Contains(domain, ".") && !strings.HasPrefix(domain, ".") && !strings.HasSuffix(domain, ".")
}

// normalizePassword applies NFKC (decision L-09) so the same password typed on different
// devices (NFC vs NFD Vietnamese, full-width digits) hashes and verifies identically.
// It must run before the length rules, before hashing and before verifying. Never change it
// once users exist: stored hashes were computed over the normalised form.
func normalizePassword(p string) string { return norm.NFKC.String(p) }

// validPassword reports whether password (already normalizePassword'ed) has 10-128 characters and differs from email
// (case-insensitively). It must run before any hashing so huge inputs never reach argon2.
func validPassword(password, email string) bool {
	n := utf8.RuneCountInString(password)
	return n >= minPasswordLen && n <= maxPasswordLen && !strings.EqualFold(password, email)
}

// cleanDisplayName normalises name to NFC, trims it and reports whether it has 1-100 characters and no control characters.
func cleanDisplayName(name string) (string, bool) {
	name = strings.TrimSpace(norm.NFC.String(name))
	n := utf8.RuneCountInString(name)
	if n < 1 || n > maxDisplayNameLen {
		return "", false
	}
	for _, r := range name {
		if unicode.IsControl(r) || r == utf8.RuneError {
			return "", false
		}
	}
	return name, true
}
