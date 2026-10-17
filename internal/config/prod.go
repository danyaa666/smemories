package config

import (
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/go-sql-driver/mysql"
)

// loadProd reads the production-contract settings and, with SMEM_ENV=prod, refuses the unsafe values of docs/production-config.md.
// It covers what every command needs (API, migrate, backfill); the API-only rules live in loadRedis, loadOTP and load.
// Messages name the variable and never repeat its value when that value may be a secret (DSN, keys, URLs with credentials).
func (cfg *Config) loadProd(getenv func(string) string) error {
	var err error
	for _, b := range []struct {
		key string
		dst *bool
	}{
		{"SMEM_DB_ALLOW_INSECURE", &cfg.DBAllowInsecure},
		{"SMEM_REDIS_ALLOW_INSECURE", &cfg.RedisAllowInsecure},
		{"SMEM_METRICS_ALLOW_ANY_INTERFACE", &cfg.MetricsAllowAnyInterface},
		{"SMEM_S3_ALLOW_INSECURE", &cfg.S3AllowInsecure},
	} {
		if v := getenv(b.key); v != "" {
			if *b.dst, err = strconv.ParseBool(v); err != nil {
				return fmt.Errorf("%s=%q: want true or false", b.key, v)
			}
		}
	}
	if cfg.DBTLSCAFile = getenv("SMEM_DB_TLS_CA_FILE"); cfg.DBTLSCAFile != "" {
		if fi, err := os.Stat(cfg.DBTLSCAFile); err != nil || fi.IsDir() {
			return fmt.Errorf("SMEM_DB_TLS_CA_FILE=%q: want the path of a readable PEM file", cfg.DBTLSCAFile)
		}
	}
	if cfg.Env != "prod" {
		return nil
	}

	// Database: verified TLS (tls=true in the DSN, or a CA file) unless explicitly opted out.
	if !cfg.DBAllowInsecure && cfg.DBTLSCAFile == "" {
		mc, err := mysql.ParseDSN(cfg.DBDSN)
		if err != nil {
			return fmt.Errorf("SMEM_DB_DSN is not valid: want user:pass@tcp(host:port)/dbname?tls=true")
		}
		if mc.TLSConfig != "true" { // false, preferred and skip-verify all allow a plaintext or unverified connection
			return fmt.Errorf("SMEM_DB_DSN: prod needs verified TLS: add ?tls=true or set SMEM_DB_TLS_CA_FILE (the RDS CA bundle)")
		}
	}

	// Browser origins and links: https only, so cookies and CORS never run over plaintext.
	for _, o := range cfg.AllowedOrigins {
		if u, _ := url.Parse(o); u == nil || u.Scheme != "https" {
			return fmt.Errorf("SMEM_ALLOWED_ORIGINS: prod origins must be https, got %q", redactURL(o))
		}
	}
	if u, _ := url.Parse(cfg.PublicBaseURL); cfg.PublicBaseURL != "" && (u == nil || u.Scheme != "https") {
		return fmt.Errorf("SMEM_PUBLIC_BASE_URL=%q: prod needs an https URL", redactURL(cfg.PublicBaseURL))
	}

	// A custom object-store endpoint (MinIO or similar) must be https; AWS S3 needs no endpoint at all.
	if u, _ := url.Parse(cfg.S3Endpoint); cfg.S3Endpoint != "" && !cfg.S3AllowInsecure && (u == nil || u.Scheme != "https") {
		return fmt.Errorf("SMEM_S3_ENDPOINT=%q: prod needs an https URL (leave it unset for AWS S3)", redactURL(cfg.S3Endpoint))
	}

	// The metrics listener has no authentication: private interface or loopback only, unless the operator says otherwise.
	if cfg.MetricsAddr != "" && !cfg.MetricsAllowAnyInterface && !privateHost(cfg.MetricsAddr) {
		return fmt.Errorf("SMEM_METRICS_ADDR=%q: prod needs a loopback or private address (e.g. 127.0.0.1:9090); to bind every interface on a private task network set SMEM_METRICS_ALLOW_ANY_INTERFACE=true", cfg.MetricsAddr)
	}
	return nil
}

// privateHost reports whether the host of addr is localhost or a loopback, private or link-local IP. An empty host, an unspecified IP
// (0.0.0.0, ::) and any other name are not: they cannot be shown to stay off the public network.
func privateHost(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && !ip.IsUnspecified() && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast())
}

// redactURL returns s with any user:pass@ replaced by ***@ and any query string or fragment replaced by ?*** / #***, so a refusal message
// never echoes a credential or a token (?token=...). A string that does not parse but contains an @, ? or # is replaced entirely.
func redactURL(s string) string {
	u, err := url.Parse(s)
	if err != nil {
		if strings.ContainsAny(s, "@?#") {
			return "***"
		}
		return s
	}
	if u.User == nil && u.RawQuery == "" && u.Fragment == "" && !u.ForceQuery {
		if strings.ContainsAny(u.Opaque, "@?#") { // user:pass@host parses as scheme "user" with an opaque rest
			return "***"
		}
		return s
	}
	hadUser := u.User != nil
	u.User = nil
	if u.RawQuery != "" || u.ForceQuery {
		u.RawQuery, u.ForceQuery = "***", false
	}
	if u.Fragment != "" {
		u.Fragment = "***"
	}
	out := u.String()
	if hadUser {
		out = strings.Replace(out, "://", "://***@", 1)
	}
	return out
}

// LogWarnings logs, loudly, every production safety opt-out that is on and the static S3 keys that override the task role. Every
// binary calls it right after it creates its logger, so the one-off tasks are as visible as the API. Only prod warns: the opt-outs
// are the normal dev setup elsewhere. It never logs a value.
func (cfg Config) LogWarnings(logger *slog.Logger) {
	if cfg.Env != "prod" {
		return
	}
	for _, o := range []struct {
		name string
		on   bool
	}{
		{"SMEM_DB_ALLOW_INSECURE", cfg.DBAllowInsecure},
		{"SMEM_REDIS_ALLOW_INSECURE", cfg.RedisAllowInsecure},
		{"SMEM_METRICS_ALLOW_ANY_INTERFACE", cfg.MetricsAllowAnyInterface},
		{"SMEM_S3_ALLOW_INSECURE", cfg.S3AllowInsecure},
	} {
		if o.on { // never expected on AWS (docs/production-config.md)
			logger.Warn("production safety opt-out is on", "variable", o.name)
		}
	}
	if cfg.S3AccessKey != "" {
		logger.Warn("static S3 keys are set and override the task role credentials", "variable", "SMEM_S3_ACCESS_KEY")
	}
}
