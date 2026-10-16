package auth

import (
	"strings"
	"testing"
	"time"
)

// T-081 AC3: the format rule.
func TestValidUsername(t *testing.T) {
	for name, ok := range map[string]bool{
		"abc": true, "a.b": true, "a_b": true, "a-b": true, "alice.bee-2_x": true, "007": true,
		strings.Repeat("a", 30): true, strings.Repeat("a", 31): false,
		"ab": false, "": false, " ab ": false, "a b": false,
		".abc": false, "abc.": false, "_abc": false, "abc-": false,
		"a..b": false, "a._b": false, "a--b": false, "a-_b": false,
		"Alice": false, // normalise first; the rule itself is lower case only
		"ánh":   false, "ünï": false, "аlice": false /* Cyrillic a */, "ａlice": false /* full width */, "alice\u200b": false, "ali\nce": false,
		"alice<script>": false, "a'b": false, `a"b`: false, "a;drop": false, "a/b": false, "a@b": false, "a%20b": false, "a\x00b": false,
	} {
		if got := validUsername(name); got != ok {
			t.Errorf("validUsername(%q) = %v, want %v", name, got, ok)
		}
	}
}

func TestNormalizeUsername(t *testing.T) {
	for in, want := range map[string]string{"Alice": "alice", "  ALICE.Bee \t": "alice.bee", "alice": "alice"} {
		if got := normalizeUsername(in); got != want {
			t.Errorf("normalizeUsername(%q) = %q, want %q", in, got, want)
		}
	}
	// The Kelvin sign lower-cases to a plain k in Unicode; here it must stay non-ASCII and therefore invalid.
	if got := normalizeUsername("\u212Alice"); validUsername(got) || got != "\u212alice" {
		t.Errorf("Kelvin sign: %q", got)
	}
}

// T-081 AC3: the reserved list and the user- prefix.
func TestReservedUsername(t *testing.T) {
	const id = "01K7Q8W3ZR5N2M4X6V8B0C1D2E"
	own := []string{defaultUsername(id, 0), defaultUsername(id, 1), defaultUsername(id, 2)}
	for _, name := range []string{"admin", "administrator", "support", "help", "root", "system", "smemories", "api", "www", "mail",
		"team", "staff", "moderator", "official", "null", "undefined", "user-abcdefgh", "user-12345", "user-" + strings.Repeat("a", 25)} {
		if !reservedUsername(name, id) {
			t.Errorf("%q must be reserved", name)
		}
	}
	for _, name := range append(own, "alice", "users", "administrators", "my-admin", "user", "usernames", strings.ToLower(id)) {
		if reservedUsername(name, id) {
			t.Errorf("%q must be allowed", name)
		}
	}
	if !reservedUsername(own[0], "01K7Q8W3ZR5N2M4X6V8BXXXXXX") {
		t.Error("someone else's default must be reserved")
	}
}

func TestDefaultUsername(t *testing.T) {
	const id = "01K7Q8W3ZR5N2M4X6V8B0C1D2E"
	want := []string{"user-8b0c1d2e", "user-6v8b0c1d2e", "user-2m4x6v8b0c1d2e", strings.ToLower(id)}
	for i, w := range want {
		got := defaultUsername(id, i)
		if got != w || !validUsername(got) {
			t.Errorf("attempt %d = %q (valid %v), want %q", i, got, validUsername(got), w)
		}
	}
	if got := defaultUsername("U1", 0); got != "user-u1" {
		t.Errorf("short id: %q", got)
	}
}

func TestExposeNextChangeAt(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	s := &Service{now: func() time.Time { return now }}
	cd := usernameCooldown.Milliseconds()
	for _, c := range []struct {
		changedAt int64
		wantNil   bool
	}{
		{0, true},                        // never changed
		{now.UnixMilli() - cd, true},     // exactly 7 days ago: allowed
		{now.UnixMilli() - cd - 1, true}, // past
		{now.UnixMilli() - cd + 1, false},
		{now.UnixMilli(), false},
	} {
		u := s.expose(User{UsernameChangedAt: c.changedAt})
		if (u.UsernameNextChangeAt == nil) != c.wantNil {
			t.Errorf("changedAt %d: next %v, wantNil %v", c.changedAt, u.UsernameNextChangeAt, c.wantNil)
		}
		if u.UsernameNextChangeAt != nil && *u.UsernameNextChangeAt != c.changedAt+cd {
			t.Errorf("changedAt %d: next %d", c.changedAt, *u.UsernameNextChangeAt)
		}
	}
}
