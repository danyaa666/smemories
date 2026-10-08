// Package oidctest is an in-process fake OpenID Connect provider (discovery, JWKS, token
// endpoint with PKCE) so Google sign-in can be tested without Google credentials. It is
// strict where a real provider is strict: the authorization request, the redirect URI, the
// client credentials and the PKCE verifier are all checked, and a code works once.
package oidctest

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	ClientID     = "test-client-id"
	ClientSecret = "test-client-secret"
	kid          = "test-key-1"
)

var (
	keyOnce  sync.Once
	signKey  *rsa.PrivateKey
	otherKey *rsa.PrivateKey // signs tokens that must fail verification
)

func keys(t testing.TB) (*rsa.PrivateKey, *rsa.PrivateKey) {
	keyOnce.Do(func() {
		var err error
		if signKey, err = rsa.GenerateKey(rand.Reader, 2048); err != nil {
			t.Fatal(err)
		}
		if otherKey, err = rsa.GenerateKey(rand.Reader, 2048); err != nil {
			t.Fatal(err)
		}
	})
	return signKey, otherKey
}

// Claims describes the ID token the next code will exchange for. Zero values mean a normal,
// valid token; the Bad* fields break exactly one thing.
type Claims struct {
	Sub           string
	Email         string
	EmailVerified any // default true; set false or the string "true" to test the check
	Name          string
	Locale        string

	BadSignature bool          // signed with a key that is not in the JWKS
	Issuer       string        // overrides iss
	Audience     string        // overrides aud
	ExpiresIn    time.Duration // default +1h; negative = already expired
	Nonce        *string       // overrides the nonce taken from the authorization request
	OmitIDToken  bool
}

type grant struct {
	c         Claims
	challenge string
	redirect  string
	nonce     string
}

// Provider is the fake. URL is the issuer.
type Provider struct {
	*httptest.Server
	t testing.TB

	mu        sync.Mutex
	grants    map[string]grant
	calls     int
	failToken bool
}

// Calls counts hits on the token endpoint, including rejected ones.
func (p *Provider) Calls() int { p.mu.Lock(); defer p.mu.Unlock(); return p.calls }

// FailToken makes the token endpoint answer 400 invalid_grant.
func (p *Provider) FailToken(on bool) { p.mu.Lock(); p.failToken = on; p.mu.Unlock() }

// New starts a provider; it stops when the test ends.
func New(t testing.TB) *Provider {
	t.Helper()
	good, _ := keys(t)
	p := &Provider{t: t, grants: map[string]grant{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{
			"issuer":                                p.URL,
			"authorization_endpoint":                p.URL + "/authorize",
			"token_endpoint":                        p.URL + "/token",
			"jwks_uri":                              p.URL + "/keys",
			"response_types_supported":              []string{"code"},
			"subject_types_supported":               []string{"public"},
			"id_token_signing_alg_values_supported": []string{"RS256"},
			"token_endpoint_auth_methods_supported": []string{"client_secret_post", "client_secret_basic"},
		})
	})
	mux.HandleFunc("GET /keys", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "alg": "RS256", "use": "sig", "kid": kid,
			"n": b64(good.N.Bytes()), "e": b64(big.NewInt(int64(good.E)).Bytes()),
		}}})
	})
	mux.HandleFunc("POST /token", p.token)
	p.Server = httptest.NewServer(mux)
	t.Cleanup(p.Close)
	return p
}

// Authorize plays the user's side of the consent screen for the authorization URL the API
// redirected to: it checks the request like a real provider would and returns the code the
// provider would send back to the redirect URI, bound to c.
func (p *Provider) Authorize(authURL string, c Claims) (code string) {
	p.t.Helper()
	u, err := url.Parse(authURL)
	if err != nil || !strings.HasPrefix(authURL, p.URL+"/authorize?") {
		p.t.Fatalf("authorization URL %q is not this provider's", authURL)
	}
	q := u.Query()
	scopes := strings.Fields(q.Get("scope"))
	if q.Get("response_type") != "code" || q.Get("client_id") != ClientID || q.Get("redirect_uri") == "" || q.Get("state") == "" ||
		q.Get("nonce") == "" || q.Get("code_challenge") == "" || q.Get("code_challenge_method") != "S256" ||
		strings.Join(scopes, " ") != "openid email profile" {
		p.t.Fatalf("invalid authorization request: %v", q)
	}
	var b [16]byte
	_, _ = rand.Read(b[:])
	code = b64(b[:])
	p.mu.Lock()
	p.grants[code] = grant{c: c, challenge: q.Get("code_challenge"), redirect: q.Get("redirect_uri"), nonce: q.Get("nonce")}
	p.mu.Unlock()
	return code
}

func (p *Provider) token(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	p.calls++
	fail := p.failToken
	p.mu.Unlock()
	if err := r.ParseForm(); err != nil {
		oauthError(w, "invalid_request")
		return
	}
	id, secret, basic := r.BasicAuth()
	if !basic {
		id, secret = r.PostForm.Get("client_id"), r.PostForm.Get("client_secret")
	}
	if fail || id != ClientID || subtle.ConstantTimeCompare([]byte(secret), []byte(ClientSecret)) != 1 || r.PostForm.Get("grant_type") != "authorization_code" {
		oauthError(w, "invalid_grant")
		return
	}
	p.mu.Lock()
	g, ok := p.grants[r.PostForm.Get("code")]
	delete(p.grants, r.PostForm.Get("code")) // single use
	p.mu.Unlock()
	sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
	if !ok || r.PostForm.Get("redirect_uri") != g.redirect || b64(sum[:]) != g.challenge {
		oauthError(w, "invalid_grant")
		return
	}
	resp := map[string]any{"access_token": "fake-access-token", "token_type": "Bearer", "expires_in": 3600}
	if !g.c.OmitIDToken {
		resp["id_token"] = p.idToken(g)
	}
	writeJSON(w, resp)
}

func (p *Provider) idToken(g grant) string {
	c := g.c
	good, other := keys(p.t)
	if c.Sub == "" {
		c.Sub = "google-sub-1"
	}
	if c.EmailVerified == nil {
		c.EmailVerified = true
	}
	iss, aud, exp, nonce := p.URL, ClientID, time.Hour, g.nonce
	if c.Issuer != "" {
		iss = c.Issuer
	}
	if c.Audience != "" {
		aud = c.Audience
	}
	if c.ExpiresIn != 0 {
		exp = c.ExpiresIn
	}
	if c.Nonce != nil {
		nonce = *c.Nonce
	}
	now := time.Now()
	claims := map[string]any{
		"iss": iss, "aud": aud, "sub": c.Sub, "iat": now.Unix(), "exp": now.Add(exp).Unix(), "nonce": nonce,
		"email": c.Email, "email_verified": c.EmailVerified, "name": c.Name, "locale": c.Locale,
	}
	hdr, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT", "kid": kid})
	body, _ := json.Marshal(claims)
	signing := b64(hdr) + "." + b64(body)
	key := good
	if c.BadSignature {
		key = other
	}
	h := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, h[:])
	if err != nil {
		p.t.Fatal(err)
	}
	return signing + "." + b64(sig)
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func oauthError(w http.ResponseWriter, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
}
