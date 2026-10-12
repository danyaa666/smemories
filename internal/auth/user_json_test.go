package auth

import (
	"encoding/json"
	"testing"
	"time"
)

// T-067: the store keeps Unix ms; the v1 API still shows created_at as an RFC 3339 string.
func TestUserJSONKeepsV1Shape(t *testing.T) {
	u := User{ID: "U1", Email: "a@example.com", EmailVerified: true, DisplayName: "A", Locale: "vi",
		CreatedAt: time.Date(2026, 10, 7, 12, 34, 56, 123e6, time.UTC).UnixMilli(), InternalID: 7}
	b, err := json.Marshal(map[string]User{"user": u})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"user":{"id":"U1","email":"a@example.com","email_verified":true,"display_name":"A","locale":"vi","created_at":"2026-10-07T12:34:56.123Z"}}`
	if string(b) != want {
		t.Fatalf("got  %s\nwant %s", b, want)
	}
}
