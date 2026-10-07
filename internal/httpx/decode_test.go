package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeJSON(t *testing.T) {
	type in struct {
		A string `json:"a"`
	}
	tests := []struct {
		name, body string
		strict     bool
		ok         bool
		status     int
		code       string
	}{
		{"ok", `{"a":"x"}`, true, true, 0, ""},
		{"unknown field lenient", `{"a":"x","b":1}`, false, true, 0, ""},
		{"unknown field strict", `{"a":"x","b":1}`, true, false, 400, "unknown_field"},
		{"wrong type strict", `{"a":1}`, true, false, 400, "invalid_body"},
		{"trailing data", `{"a":"x"} {}`, true, false, 400, "invalid_body"},
		{"empty", ``, true, false, 400, "invalid_body"},
		{"too large", `{"a":"` + strings.Repeat("x", 100) + `"}`, true, false, 413, "payload_too_large"},
	}
	for _, tc := range tests {
		r := httptest.NewRequest("POST", "/", strings.NewReader(tc.body))
		w := httptest.NewRecorder()
		r.Body = http.MaxBytesReader(w, r.Body, 50)
		var v in
		if got := DecodeJSON(w, r, &v, tc.strict); got != tc.ok {
			t.Errorf("%s: ok=%v", tc.name, got)
		}
		if !tc.ok && (w.Code != tc.status || !strings.Contains(w.Body.String(), `"`+tc.code+`"`)) {
			t.Errorf("%s: %d %s", tc.name, w.Code, w.Body.String())
		}
	}
}
