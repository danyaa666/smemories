package config

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

const dsn = "u:p@tcp(127.0.0.1:3306)/db"

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(env(map[string]string{"SMEM_DB_DSN": dsn}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != ":8080" || cfg.Env != "dev" || cfg.LogLevel != slog.LevelInfo {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	if cfg.DBDSN != dsn || cfg.DBMaxOpen != 20 || cfg.DBMaxIdle != 5 || cfg.DBConnMaxLifetime != 5*time.Minute {
		t.Fatalf("unexpected db defaults: %+v", cfg)
	}
}

func TestLoadDSNRequiredOutsideTest(t *testing.T) {
	for _, e := range []string{"", "dev", "prod"} {
		if _, err := Load(env(map[string]string{"SMEM_ENV": e})); err == nil || !strings.Contains(err.Error(), "SMEM_DB_DSN") {
			t.Errorf("SMEM_ENV=%q without DSN: want error naming SMEM_DB_DSN, got %v", e, err)
		}
	}
	if _, err := Load(env(map[string]string{"SMEM_ENV": "test"})); err != nil {
		t.Errorf("SMEM_ENV=test needs no DSN, got %v", err)
	}
}

func TestLoadPoolSettings(t *testing.T) {
	cfg, err := Load(env(map[string]string{"SMEM_DB_DSN": dsn, "SMEM_DB_MAX_OPEN": "7", "SMEM_DB_MAX_IDLE": "0", "SMEM_DB_CONN_MAX_LIFETIME": "90s"}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DBMaxOpen != 7 || cfg.DBMaxIdle != 0 || cfg.DBConnMaxLifetime != 90*time.Second {
		t.Fatalf("unexpected pool settings: %+v", cfg)
	}
}

func TestLoadValid(t *testing.T) {
	cfg, err := Load(env(map[string]string{"SMEM_HTTP_ADDR": "127.0.0.1:0", "SMEM_ENV": "prod", "SMEM_LOG_LEVEL": "debug", "SMEM_DB_DSN": dsn, "SMEM_ALLOWED_ORIGINS": "https://app.example.com", "SMEM_PUBLIC_BASE_URL": "https://app.example.com/", "SMEM_S3_BUCKET": "b"}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != "127.0.0.1:0" || cfg.Env != "prod" || cfg.LogLevel != slog.LevelDebug {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestLoadInvalidNamesVariable(t *testing.T) {
	cases := []struct{ key, val string }{
		{"SMEM_HTTP_ADDR", "nonsense"},
		{"SMEM_HTTP_ADDR", ":99999"},
		{"SMEM_HTTP_ADDR", ":http"},
		{"SMEM_ENV", "staging"},
		{"SMEM_ENV", "PROD"},
		{"SMEM_LOG_LEVEL", "verbose"},
		{"SMEM_LOG_LEVEL", "INFO"},
		{"SMEM_DB_MAX_OPEN", "0"},
		{"SMEM_DB_MAX_OPEN", "many"},
		{"SMEM_DB_MAX_IDLE", "-1"},
		{"SMEM_DB_CONN_MAX_LIFETIME", "5"},
		{"SMEM_DB_CONN_MAX_LIFETIME", "-1m"},
	}
	for _, c := range cases {
		_, err := Load(env(map[string]string{c.key: c.val, "SMEM_DB_DSN": dsn}))
		if err == nil || !strings.Contains(err.Error(), c.key) {
			t.Errorf("%s=%q: want error naming the variable, got %v", c.key, c.val, err)
		}
	}
}

func TestLoadAuthDefaults(t *testing.T) {
	cfg, err := Load(env(map[string]string{"SMEM_DB_DSN": dsn}))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.AllowedOrigins) != 1 || cfg.AllowedOrigins[0] != "http://localhost:5173" || cfg.TrustProxy ||
		cfg.MaxHashes != 4 || cfg.ArgonMemoryKiB != 19456 || cfg.ArgonTime != 2 || cfg.ArgonParallelism != 1 ||
		cfg.RegisterPerHour != 5 || cfg.LoginFailsPerPair != 10 || cfg.LoginFailsPerIP != 100 {
		t.Fatalf("unexpected auth defaults: %+v", cfg)
	}
}

func TestLoadAllowedOrigins(t *testing.T) {
	cfg, err := Load(env(map[string]string{"SMEM_DB_DSN": dsn, "SMEM_ENV": "prod", "SMEM_PUBLIC_BASE_URL": "https://app.example.com", "SMEM_ALLOWED_ORIGINS": " https://App.Example.com , http://localhost:5173 ", "SMEM_TRUST_PROXY": "true", "SMEM_S3_BUCKET": "b"}))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.AllowedOrigins) != 2 || cfg.AllowedOrigins[0] != "https://app.example.com" || !cfg.TrustProxy {
		t.Fatalf("unexpected: %+v", cfg)
	}
	if _, err := Load(env(map[string]string{"SMEM_DB_DSN": dsn, "SMEM_ENV": "prod"})); err == nil || !strings.Contains(err.Error(), "SMEM_ALLOWED_ORIGINS") {
		t.Errorf("prod without origins: got %v", err)
	}
	for _, bad := range []string{"example.com", "https://example.com/path", "ftp://example.com", "*"} {
		if _, err := Load(env(map[string]string{"SMEM_DB_DSN": dsn, "SMEM_ALLOWED_ORIGINS": bad})); err == nil || !strings.Contains(err.Error(), "SMEM_ALLOWED_ORIGINS") {
			t.Errorf("origin %q: got %v", bad, err)
		}
	}
	for k, v := range map[string]string{"SMEM_TRUST_PROXY": "maybe", "SMEM_AUTH_MAX_CONCURRENT_HASHES": "0", "SMEM_AUTH_ARGON_MEMORY_KIB": "99999999999", "SMEM_AUTH_ARGON_PARALLELISM": "300", "SMEM_RATE_REGISTER_PER_HOUR": "x"} {
		if _, err := Load(env(map[string]string{"SMEM_DB_DSN": dsn, k: v})); err == nil || !strings.Contains(err.Error(), k) {
			t.Errorf("%s=%s: got %v", k, v, err)
		}
	}
}

func TestLoadMediaAndS3(t *testing.T) {
	cfg, err := Load(env(map[string]string{"SMEM_DB_DSN": dsn}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MediaMaxBytes != 10<<20 || cfg.MediaMaxConcurrent != 4 || cfg.S3Bucket != "smemories-dev" || cfg.S3Region != "us-east-1" || cfg.S3PathStyle {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	cfg, err = Load(env(map[string]string{"SMEM_DB_DSN": dsn, "SMEM_MEDIA_MAX_BYTES": "2048", "SMEM_S3_ENDPOINT": "http://127.0.0.1:9000", "SMEM_S3_PATH_STYLE": "true", "SMEM_S3_BUCKET": "b"}))
	if err != nil || cfg.MediaMaxBytes != 2048 || cfg.S3Endpoint != "http://127.0.0.1:9000" || !cfg.S3PathStyle || cfg.S3Bucket != "b" {
		t.Fatalf("got %+v, %v", cfg, err)
	}
	for _, c := range []struct{ key, val string }{
		{"SMEM_MEDIA_MAX_BYTES", "0"}, {"SMEM_MEDIA_MAX_BYTES", "999999999999"}, {"SMEM_MEDIA_MAX_CONCURRENT", "0"},
		{"SMEM_S3_ENDPOINT", "minio:9000"}, {"SMEM_S3_PATH_STYLE", "maybe"},
	} {
		if _, err := Load(env(map[string]string{"SMEM_DB_DSN": dsn, c.key: c.val})); err == nil || !strings.Contains(err.Error(), c.key) {
			t.Errorf("%s=%q: want error naming the variable, got %v", c.key, c.val, err)
		}
	}
	if _, err := Load(env(map[string]string{"SMEM_ENV": "prod", "SMEM_DB_DSN": dsn, "SMEM_ALLOWED_ORIGINS": "https://a.example.com", "SMEM_PUBLIC_BASE_URL": "https://a.example.com"})); err == nil || !strings.Contains(err.Error(), "SMEM_S3_BUCKET") {
		t.Errorf("prod without bucket: got %v", err)
	}
}

func TestLoadPublicBaseURL(t *testing.T) {
	cfg, err := Load(env(map[string]string{"SMEM_DB_DSN": dsn}))
	if err != nil || cfg.PublicBaseURL != "http://localhost:5173" {
		t.Fatalf("dev default: %q, %v", cfg.PublicBaseURL, err)
	}
	cfg, err = Load(env(map[string]string{"SMEM_DB_DSN": dsn, "SMEM_PUBLIC_BASE_URL": "https://app.example.com/"}))
	if err != nil || cfg.PublicBaseURL != "https://app.example.com" {
		t.Fatalf("trailing slash must be trimmed: %q, %v", cfg.PublicBaseURL, err)
	}
	prod := map[string]string{"SMEM_DB_DSN": dsn, "SMEM_ENV": "prod", "SMEM_ALLOWED_ORIGINS": "https://app.example.com", "SMEM_S3_BUCKET": "b"}
	if _, err := Load(env(prod)); err == nil || !strings.Contains(err.Error(), "SMEM_PUBLIC_BASE_URL") {
		t.Errorf("prod without it: want error naming SMEM_PUBLIC_BASE_URL, got %v", err)
	}
	for _, bad := range []string{"app.example.com", "ftp://x.com", "https://x.com?a=1", "https://u:p@x.com"} {
		if _, err := Load(env(map[string]string{"SMEM_DB_DSN": dsn, "SMEM_PUBLIC_BASE_URL": bad})); err == nil || !strings.Contains(err.Error(), "SMEM_PUBLIC_BASE_URL") {
			t.Errorf("%q: want error naming SMEM_PUBLIC_BASE_URL, got %v", bad, err)
		}
	}
}

func TestLoadGoogle(t *testing.T) {
	cfg, err := Load(env(map[string]string{"SMEM_DB_DSN": dsn}))
	if err != nil || cfg.GoogleClientID != "" {
		t.Fatalf("google must be off by default: %+v, %v", cfg, err)
	}
	full := map[string]string{
		"SMEM_DB_DSN": dsn, "SMEM_GOOGLE_CLIENT_ID": "id", "SMEM_GOOGLE_CLIENT_SECRET": "sec",
		"SMEM_PUBLIC_BASE_URL": "https://app.example.com", "SMEM_OIDC_COOKIE_KEY": strings.Repeat("k", 32),
	}
	cfg, err = Load(env(full))
	if err != nil || cfg.GoogleClientID != "id" || cfg.GoogleClientSecret != "sec" || cfg.GoogleIssuer != "https://accounts.google.com" || len(cfg.OIDCCookieKey) != 32 {
		t.Fatalf("got %+v, %v", cfg, err)
	}
	for _, missing := range []string{"SMEM_GOOGLE_CLIENT_SECRET", "SMEM_PUBLIC_BASE_URL", "SMEM_OIDC_COOKIE_KEY"} {
		m := map[string]string{}
		for k, v := range full {
			if k != missing {
				m[k] = v
			}
		}
		if _, err := Load(env(m)); err == nil || !strings.Contains(err.Error(), missing) {
			t.Errorf("without %s: want error naming it, got %v", missing, err)
		}
	}
	for k, v := range map[string]string{"SMEM_OIDC_COOKIE_KEY": strings.Repeat("k", 31), "SMEM_GOOGLE_ISSUER": "accounts.google.com"} {
		m := map[string]string{k: v}
		for k2, v2 := range full {
			if _, ok := m[k2]; !ok {
				m[k2] = v2
			}
		}
		if _, err := Load(env(m)); err == nil || !strings.Contains(err.Error(), k) || strings.Contains(err.Error(), v) && k == "SMEM_OIDC_COOKIE_KEY" {
			t.Errorf("%s=%q: want error naming the variable (and never the key), got %v", k, v, err)
		}
	}
	prod := map[string]string{"SMEM_ENV": "prod", "SMEM_ALLOWED_ORIGINS": "https://app.example.com", "SMEM_GOOGLE_ISSUER": "http://idp.example.com"}
	for k, v := range full {
		prod[k] = v
	}
	if _, err := Load(env(prod)); err == nil || !strings.Contains(err.Error(), "SMEM_GOOGLE_ISSUER") {
		t.Errorf("http issuer in prod: got %v", err)
	}
}
