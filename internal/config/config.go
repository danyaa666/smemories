// Package config reads the process configuration from SMEM_* environment variables.
package config

import (
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Config is the validated process configuration.
type Config struct {
	HTTPAddr string     // SMEM_HTTP_ADDR, host:port to listen on
	Env      string     // SMEM_ENV: dev, test or prod
	LogLevel slog.Level // SMEM_LOG_LEVEL: debug, info, warn or error

	DBDSN             string        // SMEM_DB_DSN: user:pass@tcp(host:port)/dbname; required unless Env is "test" (db.Open still rejects an empty one). Never log it.
	DBMaxOpen         int           // SMEM_DB_MAX_OPEN, default 20
	DBMaxIdle         int           // SMEM_DB_MAX_IDLE, default 5
	DBConnMaxLifetime time.Duration // SMEM_DB_CONN_MAX_LIFETIME, default 5m

	AllowedOrigins    []string // SMEM_ALLOWED_ORIGINS: comma-separated browser origins; required in prod, default http://localhost:5173 otherwise
	PublicBaseURL     string   // SMEM_PUBLIC_BASE_URL: where the web app lives, used for links in emails; required in prod, default http://localhost:5173 otherwise; no trailing slash
	TrustProxy        bool     // SMEM_TRUST_PROXY: take the client IP from the last X-Forwarded-For hop
	MaxHashes         int      // SMEM_AUTH_MAX_CONCURRENT_HASHES, default 4
	ArgonMemoryKiB    uint32   // SMEM_AUTH_ARGON_MEMORY_KIB, default 19456
	ArgonTime         uint32   // SMEM_AUTH_ARGON_TIME, default 2
	ArgonParallelism  uint8    // SMEM_AUTH_ARGON_PARALLELISM, default 1
	RegisterPerHour   int      // SMEM_RATE_REGISTER_PER_HOUR (per IP), default 5
	LoginFailsPerPair int      // SMEM_RATE_LOGIN_FAILS_PER_EMAIL (per IP+email, 15 min), default 10
	LoginFailsPerIP   int      // SMEM_RATE_LOGIN_FAILS_PER_IP (15 min), default 100

	GoogleClientID     string // SMEM_GOOGLE_CLIENT_ID: empty = Google sign-in off (the endpoints answer 404)
	GoogleClientSecret string // SMEM_GOOGLE_CLIENT_SECRET: required with a client id. Never log it.
	GoogleIssuer       string // SMEM_GOOGLE_ISSUER, default https://accounts.google.com (tests point it at a fake)
	OIDCCookieKey      []byte // SMEM_OIDC_COOKIE_KEY: HMAC key for the smem_oidc cookie, at least 32 bytes; required with a client id. Never log it.
}

var logLevels = map[string]slog.Level{
	"debug": slog.LevelDebug,
	"info":  slog.LevelInfo,
	"warn":  slog.LevelWarn,
	"error": slog.LevelError,
}

// Load builds a Config from getenv (os.Getenv in production). An unset or empty
// variable takes its default; an invalid value returns an error naming the variable.
func Load(getenv func(string) string) (Config, error) {
	get := func(key, def string) string {
		if v := getenv(key); v != "" {
			return v
		}
		return def
	}

	var err error
	cfg := Config{HTTPAddr: get("SMEM_HTTP_ADDR", ":8080"), Env: get("SMEM_ENV", "dev")}

	if _, port, err := net.SplitHostPort(cfg.HTTPAddr); err != nil {
		return Config{}, fmt.Errorf("SMEM_HTTP_ADDR=%q: want host:port, e.g. :8080", cfg.HTTPAddr)
	} else if _, err := strconv.ParseUint(port, 10, 16); err != nil {
		return Config{}, fmt.Errorf("SMEM_HTTP_ADDR=%q: port must be a number 0-65535", cfg.HTTPAddr)
	}

	switch cfg.Env {
	case "dev", "test", "prod":
	default:
		return Config{}, fmt.Errorf("SMEM_ENV=%q: must be one of dev, test, prod", cfg.Env)
	}

	raw := get("SMEM_LOG_LEVEL", "info")
	lvl, ok := logLevels[raw]
	if !ok {
		return Config{}, fmt.Errorf("SMEM_LOG_LEVEL=%q: must be one of debug, info, warn, error", raw)
	}
	cfg.LogLevel = lvl

	cfg.DBDSN = getenv("SMEM_DB_DSN")
	if cfg.DBDSN == "" && cfg.Env != "test" {
		return Config{}, fmt.Errorf("SMEM_DB_DSN is required (e.g. user:pass@tcp(127.0.0.1:3306)/smemories)")
	}
	if cfg.DBMaxOpen, err = getInt(get, "SMEM_DB_MAX_OPEN", "20", 1); err != nil {
		return Config{}, err
	}
	if cfg.DBMaxIdle, err = getInt(get, "SMEM_DB_MAX_IDLE", "5", 0); err != nil {
		return Config{}, err
	}
	raw = get("SMEM_DB_CONN_MAX_LIFETIME", "5m")
	if cfg.DBConnMaxLifetime, err = time.ParseDuration(raw); err != nil || cfg.DBConnMaxLifetime < 0 {
		return Config{}, fmt.Errorf("SMEM_DB_CONN_MAX_LIFETIME=%q: want a non-negative duration such as 5m", raw)
	}

	origins := get("SMEM_ALLOWED_ORIGINS", "")
	if origins == "" && cfg.Env != "prod" {
		origins = "http://localhost:5173"
	}
	for _, o := range strings.Split(origins, ",") {
		if o = strings.TrimSpace(o); o == "" {
			continue
		}
		u, err := url.Parse(o)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.User != nil {
			return Config{}, fmt.Errorf("SMEM_ALLOWED_ORIGINS: %q is not an origin such as https://example.com", o)
		}
		cfg.AllowedOrigins = append(cfg.AllowedOrigins, strings.ToLower(u.Scheme+"://"+u.Host))
	}
	if len(cfg.AllowedOrigins) == 0 {
		return Config{}, fmt.Errorf("SMEM_ALLOWED_ORIGINS is required in prod (e.g. https://app.example.com)")
	}
	base := get("SMEM_PUBLIC_BASE_URL", "")
	if base == "" && cfg.Env != "prod" {
		base = "http://localhost:5173"
	}
	if base == "" {
		return Config{}, fmt.Errorf("SMEM_PUBLIC_BASE_URL is required in prod (e.g. https://app.example.com)")
	}
	if u, err := url.Parse(base); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return Config{}, fmt.Errorf("SMEM_PUBLIC_BASE_URL=%q: want a URL such as https://app.example.com", base)
	}
	cfg.PublicBaseURL = strings.TrimRight(base, "/")
	if cfg.TrustProxy, err = strconv.ParseBool(get("SMEM_TRUST_PROXY", "false")); err != nil {
		return Config{}, fmt.Errorf("SMEM_TRUST_PROXY=%q: want true or false", getenv("SMEM_TRUST_PROXY"))
	}
	if cfg.MaxHashes, err = getInt(get, "SMEM_AUTH_MAX_CONCURRENT_HASHES", "4", 1); err != nil {
		return Config{}, err
	}
	var n int
	if n, err = getInt(get, "SMEM_AUTH_ARGON_MEMORY_KIB", "19456", 8); err != nil || n > 1<<20 {
		return Config{}, fmt.Errorf("SMEM_AUTH_ARGON_MEMORY_KIB=%q: want an integer between 8 and 1048576", get("SMEM_AUTH_ARGON_MEMORY_KIB", "19456"))
	}
	cfg.ArgonMemoryKiB = uint32(n) //nolint:gosec // G115: 8 <= n <= 1<<20 checked above
	if n, err = getInt(get, "SMEM_AUTH_ARGON_TIME", "2", 1); err != nil || n > 100 {
		return Config{}, fmt.Errorf("SMEM_AUTH_ARGON_TIME=%q: want an integer between 1 and 100", get("SMEM_AUTH_ARGON_TIME", "2"))
	}
	cfg.ArgonTime = uint32(n) //nolint:gosec // G115: 1 <= n <= 100 checked above
	if n, err = getInt(get, "SMEM_AUTH_ARGON_PARALLELISM", "1", 1); err != nil || n > 255 {
		return Config{}, fmt.Errorf("SMEM_AUTH_ARGON_PARALLELISM=%q: want an integer between 1 and 255", get("SMEM_AUTH_ARGON_PARALLELISM", "1"))
	}
	cfg.ArgonParallelism = uint8(n) //nolint:gosec // G115: 1 <= n <= 255 checked above
	if cfg.RegisterPerHour, err = getInt(get, "SMEM_RATE_REGISTER_PER_HOUR", "5", 1); err != nil {
		return Config{}, err
	}
	if cfg.LoginFailsPerPair, err = getInt(get, "SMEM_RATE_LOGIN_FAILS_PER_EMAIL", "10", 1); err != nil {
		return Config{}, err
	}
	if cfg.LoginFailsPerIP, err = getInt(get, "SMEM_RATE_LOGIN_FAILS_PER_IP", "100", 1); err != nil {
		return Config{}, err
	}
	if cfg.GoogleClientID = getenv("SMEM_GOOGLE_CLIENT_ID"); cfg.GoogleClientID != "" {
		if err := cfg.loadGoogle(getenv, get); err != nil {
			return Config{}, err
		}
	}
	return cfg, nil
}

// loadGoogle reads the Google settings; all of them are required once a client id is set.
func (cfg *Config) loadGoogle(getenv func(string) string, get func(key, def string) string) error {
	if cfg.GoogleClientSecret = getenv("SMEM_GOOGLE_CLIENT_SECRET"); cfg.GoogleClientSecret == "" {
		return fmt.Errorf("SMEM_GOOGLE_CLIENT_SECRET is required when SMEM_GOOGLE_CLIENT_ID is set")
	}
	if getenv("SMEM_PUBLIC_BASE_URL") == "" { // the OAuth redirect URI is built from it, so no silent dev default
		return fmt.Errorf("SMEM_PUBLIC_BASE_URL is required when SMEM_GOOGLE_CLIENT_ID is set (e.g. https://app.example.com)")
	}
	key := getenv("SMEM_OIDC_COOKIE_KEY")
	if key == "" {
		return fmt.Errorf("SMEM_OIDC_COOKIE_KEY is required when SMEM_GOOGLE_CLIENT_ID is set (at least 32 random bytes)")
	}
	if len(key) < 32 {
		return fmt.Errorf("SMEM_OIDC_COOKIE_KEY is too short: %d bytes, want at least 32", len(key))
	}
	cfg.OIDCCookieKey = []byte(key)
	cfg.GoogleIssuer = get("SMEM_GOOGLE_ISSUER", "https://accounts.google.com")
	u, err := url.Parse(cfg.GoogleIssuer)
	if err != nil || u.Host == "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil || (u.Scheme != "https" && (u.Scheme != "http" || cfg.Env == "prod")) {
		return fmt.Errorf("SMEM_GOOGLE_ISSUER=%q: want an https URL such as https://accounts.google.com", cfg.GoogleIssuer)
	}
	return nil
}

func getInt(get func(key, def string) string, key, def string, min int) (int, error) {
	raw := get(key, def)
	n, err := strconv.Atoi(raw)
	if err != nil || n < min {
		return 0, fmt.Errorf("%s=%q: want an integer >= %d", key, raw, min)
	}
	return n, nil
}
