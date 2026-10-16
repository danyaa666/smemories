package auth

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// T-071: the user object of register, login and get-me; created_at is Unix ms and the internal id never leaves.
func TestUserJSONShape(t *testing.T) {
	u := User{ID: "U1", Email: "a@example.com", EmailVerified: true, DisplayName: "A", Locale: "vi",
		CreatedAt: time.Date(2026, 10, 7, 12, 34, 56, 123e6, time.UTC).UnixMilli(), Username: "user-abcd1234", InternalID: 7}
	b, err := json.Marshal(u)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"id":"U1","email":"a@example.com","email_verified":true,"display_name":"A","locale":"vi","created_at":1791376496123,"username":"user-abcd1234","username_next_change_at":null}`
	if string(b) != want {
		t.Fatalf("got  %s\nwant %s", b, want)
	}
}

func TestUserJSONUsernameNextChangeAt(t *testing.T) {
	next := int64(1792000000000)
	b, err := json.Marshal(User{Username: "alice", UsernameChangedAt: 5, UsernameNextChangeAt: &next})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"username":"alice","username_next_change_at":1792000000000`) || strings.Contains(string(b), "changed_at") {
		t.Fatalf("got %s", b)
	}
}
