package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"

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
			return fmt.Errorf("SMEM_ALLOWED_ORIGINS: prod origins must be https, got %q", o)
		}
	}
	if u, _ := url.Parse(cfg.PublicBaseURL); cfg.PublicBaseURL != "" && (u == nil || u.Scheme != "https") {
		return fmt.Errorf("SMEM_PUBLIC_BASE_URL=%q: prod needs an https URL", cfg.PublicBaseURL)
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
