package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSafeReturnTo(t *testing.T) {
	long := "/" + strings.Repeat("a", maxReturnToLen-1)
	for in, want := range map[string]string{
		"":                       "/",
		"/":                      "/",
		"/yearbooks/01HX?tab=2":  "/yearbooks/01HX?tab=2",
		"/a#frag":                "/a#frag",
		long:                     long,
		long + "a":               "/",
		"//evil.example":         "/",
		"/\\evil.example":        "/",
		"https://evil.example":   "/",
		"javascript:alert(1)":    "/",
		"evil.example":           "/",
		"/ok\r\nSet-Cookie: x=1": "/",
		"/ok\x00":                "/",
		"/ok\u202e":              "/", // bidi override (Cf)
		"/ok\xff":                "/", // invalid UTF-8
	} {
		if got := safeReturnTo(in); got != want {
			t.Errorf("safeReturnTo(%q) = %q, want %q", in, got, want)
		}
	}
}

func testFlow(t *testing.T) (*googleFlow, *Handler) {
	t.Helper()
	h := NewHandler(&Service{now: time.Now}, HandlerConfig{}, nil)
	if err := h.EnableGoogle(GoogleConfig{ClientID: "id", ClientSecret: "s", Issuer: "https://idp.example", RedirectURL: "https://x/cb", CookieKey: []byte(strings.Repeat("k", 32))}); err != nil {
		t.Fatal(err)
	}
	return h.google, h
}

func TestEnableGoogleValidates(t *testing.T) {
	h := NewHandler(&Service{now: time.Now}, HandlerConfig{}, nil)
	ok := GoogleConfig{ClientID: "id", ClientSecret: "s", Issuer: "https://idp.example", RedirectURL: "https://x/cb", CookieKey: []byte(strings.Repeat("k", 32))}
	for name, mutate := range map[string]func(*GoogleConfig){
		"no id":      func(c *GoogleConfig) { c.ClientID = "" },
		"no secret":  func(c *GoogleConfig) { c.ClientSecret = "" },
		"no issuer":  func(c *GoogleConfig) { c.Issuer = "" },
		"no redir":   func(c *GoogleConfig) { c.RedirectURL = "" },
		"short key":  func(c *GoogleConfig) { c.CookieKey = []byte(strings.Repeat("k", 31)) },
		"empty key":  func(c *GoogleConfig) { c.CookieKey = nil },
		"short utf8": func(c *GoogleConfig) { c.CookieKey = []byte("é") },
	} {
		c := ok
		mutate(&c)
		if err := h.EnableGoogle(c); err == nil {
			t.Errorf("%s: want error", name)
		}
		if h.google != nil {
			t.Errorf("%s: google must stay disabled", name)
		}
	}
	if err := h.EnableGoogle(ok); err != nil {
		t.Fatal(err)
	}
}

func cookieReq(v string) *http.Request {
	r := httptest.NewRequest("GET", "/", nil)
	if v != "" {
		r.AddCookie(&http.Cookie{Name: oidcCookie, Value: v})
	}
	return r
}

func TestOIDCCookieSealOpen(t *testing.T) {
	g, _ := testFlow(t)
	now := time.Now()
	st := oidcState{State: "s", Nonce: "n", Verifier: "v", ReturnTo: "/x", Expires: now.Add(time.Minute).Unix()}
	v := g.seal(st)
	if got, ok := g.open(cookieReq(v), now); !ok || got != st {
		t.Fatalf("round trip: %+v, %v", got, ok)
	}
	if _, ok := g.open(cookieReq(v), now.Add(time.Minute)); ok {
		t.Error("expired cookie accepted")
	}
	if _, ok := g.open(cookieReq(""), now); ok {
		t.Error("missing cookie accepted")
	}
	payload, sig, _ := strings.Cut(v, ".")
	other := g.seal(oidcState{State: "evil", Nonce: "n", Verifier: "v", ReturnTo: "/", Expires: st.Expires})
	otherPayload, otherSig, _ := strings.Cut(other, ".")
	flip := func(s string) string { // change the last character
		if strings.HasSuffix(s, "A") {
			return s[:len(s)-1] + "B"
		}
		return s[:len(s)-1] + "A"
	}
	for name, bad := range map[string]string{
		"payload swapped": otherPayload + "." + sig,
		"sig swapped":     payload + "." + otherSig,
		"payload flipped": flip(payload) + "." + sig,
		"sig flipped":     payload + "." + flip(sig),
		"no signature":    payload,
		"empty sig":       payload + ".",
		"extra part":      v + ".x",
		"garbage":         "!!!.!!!",
	} {
		if _, ok := g.open(cookieReq(bad), now); ok {
			t.Errorf("%s: tampered cookie accepted", name)
		}
	}
	// A cookie signed with another key is rejected.
	g2, _ := testFlow(t)
	g2.cfg.CookieKey = []byte(strings.Repeat("z", 32))
	if _, ok := g.open(cookieReq(g2.seal(st)), now); ok {
		t.Error("cookie signed with another key accepted")
	}
}
