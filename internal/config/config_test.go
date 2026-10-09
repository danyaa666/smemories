package config

import (
	"log/slog"
	"strconv"
	"strings"
	"testing"
	"time"
)

// env serves m; SMEM_REDIS_URL defaults to a valid URL unless the case sets it (even to "").
func env(m map[string]string) func(string) string {
	return func(k string) string {
		if v, ok := m[k]; ok || k != "SMEM_REDIS_URL" {
			return v
		}
		return "redis://127.0.0.1:6379/0"
	}
}

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
	if cfg.MediaMaxBytes != 10<<20 || cfg.MediaMaxConcurrent != 2 || cfg.PublicUploadsPerIP != 8 || cfg.MemoryLimitMiB != 0 || cfg.S3Bucket != "smemories-dev" || cfg.S3Region != "us-east-1" || cfg.S3PathStyle {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	cfg, err = Load(env(map[string]string{"SMEM_DB_DSN": dsn, "SMEM_MEDIA_MAX_BYTES": "2048", "SMEM_S3_ENDPOINT": "http://127.0.0.1:9000", "SMEM_S3_PATH_STYLE": "true", "SMEM_S3_BUCKET": "b"}))
	if err != nil || cfg.MediaMaxBytes != 2048 || cfg.S3Endpoint != "http://127.0.0.1:9000" || !cfg.S3PathStyle || cfg.S3Bucket != "b" {
		t.Fatalf("got %+v, %v", cfg, err)
	}
	for _, v := range []string{"64", "1536", "1048576"} {
		if cfg, err = Load(env(map[string]string{"SMEM_DB_DSN": dsn, "SMEM_MEMORY_LIMIT_MIB": v})); err != nil || strconv.Itoa(cfg.MemoryLimitMiB) != v {
			t.Errorf("SMEM_MEMORY_LIMIT_MIB=%s: got %d, %v", v, cfg.MemoryLimitMiB, err)
		}
	}
	for _, c := range []struct{ key, val string }{
		{"SMEM_MEDIA_MAX_BYTES", "0"}, {"SMEM_MEDIA_MAX_BYTES", "999999999999"}, {"SMEM_MEDIA_MAX_CONCURRENT", "0"}, {"SMEM_PUBLIC_UPLOAD_CONCURRENT_PER_IP", "0"},
		{"SMEM_MEMORY_LIMIT_MIB", "-1"}, {"SMEM_MEMORY_LIMIT_MIB", "63"}, {"SMEM_MEMORY_LIMIT_MIB", "1048577"}, {"SMEM_MEMORY_LIMIT_MIB", "1g"},
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

func TestLoadRedisDefaults(t *testing.T) {
	cfg, err := Load(env(map[string]string{"SMEM_DB_DSN": dsn, "SMEM_REDIS_PASSWORD": "pw"}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RedisURL != "redis://127.0.0.1:6379/0" || cfg.RedisPassword != "pw" || cfg.RedisDialTimeout != 2*time.Second ||
		cfg.RedisReadTimeout != time.Second || cfg.RedisWriteTimeout != time.Second || cfg.RedisPoolSize != 10 {
		t.Fatalf("unexpected redis defaults: %+v", cfg)
	}
}

func TestLoadRedisRequiredInEveryEnv(t *testing.T) {
	for _, e := range []string{"dev", "test", "prod"} {
		_, err := Load(env(map[string]string{"SMEM_ENV": e, "SMEM_DB_DSN": dsn, "SMEM_REDIS_URL": "", "SMEM_ALLOWED_ORIGINS": "https://a.example", "SMEM_PUBLIC_BASE_URL": "https://a.example", "SMEM_S3_BUCKET": "b"}))
		if err == nil || !strings.Contains(err.Error(), "SMEM_REDIS_URL") {
			t.Errorf("SMEM_ENV=%s without SMEM_REDIS_URL: want error naming it, got %v", e, err)
		}
	}
}

func TestLoadRedisInvalidNamesVariableAndHidesPassword(t *testing.T) {
	cases := []struct{ key, val string }{
		{"SMEM_REDIS_URL", "127.0.0.1:6379"},
		{"SMEM_REDIS_URL", "http://127.0.0.1:6379/0"},
		{"SMEM_REDIS_URL", "redis://:s3cret@/0"},
		{"SMEM_REDIS_URL", "redis://:s3cret@h:6379/16"},
		{"SMEM_REDIS_URL", "redis://:s3cret@h:6379/x"},
		{"SMEM_REDIS_URL", "redis://:s3cret@h:6379/0?db=1"},
		{"SMEM_REDIS_DIAL_TIMEOUT", "2"},
		{"SMEM_REDIS_READ_TIMEOUT", "0s"},
		{"SMEM_REDIS_WRITE_TIMEOUT", "-1s"},
		{"SMEM_REDIS_POOL_SIZE", "0"},
	}
	for _, c := range cases {
		_, err := Load(env(map[string]string{c.key: c.val, "SMEM_DB_DSN": dsn}))
		if err == nil || !strings.Contains(err.Error(), c.key) || strings.Contains(err.Error(), "s3cret") {
			t.Errorf("%s=%q: want error naming the variable without the password, got %v", c.key, c.val, err)
		}
	}
	for _, ok := range []string{"redis://h:6379", "rediss://:pw@h:6380/3", "redis://h/0"} {
		if _, err := Load(env(map[string]string{"SMEM_REDIS_URL": ok, "SMEM_DB_DSN": dsn})); err != nil {
			t.Errorf("%q should be valid: %v", ok, err)
		}
	}
}

func TestLoadRequestTimeout(t *testing.T) {
	cfg, err := Load(env(map[string]string{"SMEM_DB_DSN": dsn}))
	if err != nil || cfg.RequestTimeout != 30*time.Second {
		t.Fatalf("default: %v, %v", cfg.RequestTimeout, err)
	}
	cfg, err = Load(env(map[string]string{"SMEM_DB_DSN": dsn, "SMEM_HTTP_REQUEST_TIMEOUT": "1s"}))
	if err != nil || cfg.RequestTimeout != time.Second {
		t.Fatalf("1s: %v, %v", cfg.RequestTimeout, err)
	}
	for _, bad := range []string{"999ms", "0", "-5s", "soon"} {
		if _, err := Load(env(map[string]string{"SMEM_DB_DSN": dsn, "SMEM_HTTP_REQUEST_TIMEOUT": bad})); err == nil || !strings.Contains(err.Error(), "SMEM_HTTP_REQUEST_TIMEOUT") {
			t.Errorf("%q: got %v", bad, err)
		}
	}
}
