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
		ok         bool
		status     int
		code       string
	}{
		{"ok", `{"a":"x"}`, true, 0, ""},
		{"unknown field", `{"a":"x","b":1}`, false, 400, "ERROR_PARAM"},
		{"wrong type", `{"a":1}`, false, 400, "ERROR_PARAM"},
		{"trailing data", `{"a":"x"} {}`, false, 400, "ERROR_PARAM"},
		{"empty", ``, false, 400, "ERROR_PARAM"},
		{"too large", `{"a":"` + strings.Repeat("x", 100) + `"}`, false, 413, "ERROR_TOO_LARGE"},
	}
	for _, tc := range tests {
		r := httptest.NewRequest("POST", "/", strings.NewReader(tc.body))
		w := httptest.NewRecorder()
		r.Body = http.MaxBytesReader(w, r.Body, 50)
		var v in
		if got := DecodeJSON(w, r, &v); got != tc.ok {
			t.Errorf("%s: ok=%v", tc.name, got)
		}
		if !tc.ok && (w.Code != tc.status || !strings.Contains(w.Body.String(), `"`+tc.code+`"`)) {
			t.Errorf("%s: %d %s", tc.name, w.Code, w.Body.String())
		}
	}
}
