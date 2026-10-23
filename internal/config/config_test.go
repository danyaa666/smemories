package config

import (
	"encoding/base64"
	"log/slog"
	"maps"
	"strconv"
	"strings"
	"testing"
	"time"
)

// prodTestDefaults are what env adds for SMEM_ENV=prod, so the tests of one setting need not repeat the whole production contract
// (that is prod_test.go's job, which does not use env): the plaintext opt-outs of a local network, a memory limit and the SES mailer.
var prodTestDefaults = map[string]string{"SMEM_DB_ALLOW_INSECURE": "true", "SMEM_REDIS_ALLOW_INSECURE": "true", "SMEM_MEMORY_LIMIT_MIB": "1536",
	"SMEM_MAIL_PROVIDER": "ses", "SMEM_MAIL_FROM": "SMemories <no-reply@mail.smemories.io>", "SMEM_MEDIA_LINK_SECRET": linkSecret}

// linkSecret is a valid SMEM_MEDIA_LINK_SECRET (48 bytes, base64).
var linkSecret = base64.StdEncoding.EncodeToString([]byte(strings.Repeat("s", 48)))

// env serves m; SMEM_ENV and SMEM_REDIS_URL default to dev and a valid URL unless the case sets them (even to ""; the loader itself has no
// SMEM_ENV default, T-099), and prod gets prodTestDefaults.
func env(m map[string]string) func(string) string {
	return func(k string) string {
		if v, ok := m[k]; ok {
			return v
		}
		if k == "SMEM_ENV" {
			return "dev"
		}
		if k == "SMEM_REDIS_URL" {
			return "redis://127.0.0.1:6379/0"
		}
		if m["SMEM_ENV"] == "prod" {
			return prodTestDefaults[k]
		}
		return ""
	}
}

const dsn = "u:p@tcp(127.0.0.1:3306)/db"

// testOrigin is an origin that env accepts: dev and test are local-only (T-099), prod needs a public https one.
func testOrigin(envName string) string {
	if envName == "prod" {
		return "https://a.example.com"
	}
	return "http://localhost:5173"
}

// T-099: SMEM_ENV has no default, in Load and in LoadMigrate.
func TestLoadEnvRequired(t *testing.T) {
	for _, load := range []func(func(string) string) (Config, error){Load, LoadMigrate} {
		if _, err := load(env(map[string]string{"SMEM_ENV": "", "SMEM_DB_DSN": dsn})); err == nil || !strings.Contains(err.Error(), "SMEM_ENV is required") {
			t.Errorf("empty SMEM_ENV: want a refusal naming SMEM_ENV, got %v", err)
		}
		// Unset: the getenv func knows no SMEM_ENV at all (env() would supply dev).
		if _, err := load(func(k string) string { return map[string]string{"SMEM_DB_DSN": dsn}[k] }); err == nil || !strings.Contains(err.Error(), "SMEM_ENV is required") {
			t.Errorf("unset SMEM_ENV: want a refusal naming SMEM_ENV, got %v", err)
		}
		for _, e := range []string{"dev", "test", "prod"} {
			if _, err := load(env(map[string]string{"SMEM_ENV": e, "SMEM_DB_DSN": dsn, "SMEM_ALLOWED_ORIGINS": testOrigin(e), "SMEM_PUBLIC_BASE_URL": testOrigin(e), "SMEM_OTP_KEY": strings.Repeat("k", 32), "SMEM_S3_BUCKET": "b"})); err != nil {
				t.Errorf("SMEM_ENV=%s: %v", e, err)
			}
		}
		if _, err := load(env(map[string]string{"SMEM_ENV": "staging", "SMEM_DB_DSN": dsn})); err == nil || !strings.Contains(err.Error(), "SMEM_ENV") {
			t.Errorf("staging: got %v", err)
		}
	}
}

// T-099: dev and test refuse a host that is not local, in the origins and in the public base URL, for the API and for the one-off tasks.
func TestLoadDevTestAreLocalOnly(t *testing.T) {
	good := []string{"http://localhost:5173", "http://127.0.0.1:3000", "http://192.168.1.20:5173", "http://[::1]:5173", "http://app.localhost", "http://10.0.0.5", "http://169.254.1.1", "http://LOCALHOST:5173"}
	bad := []string{"https://app.example.com", "http://8.8.8.8:5173", "http://0.0.0.0:5173", "http://localhost.example.com", "http://notlocalhost", "http://[2001:db8::1]"}
	for _, load := range []func(func(string) string) (Config, error){Load, LoadMigrate} {
		for _, e := range []string{"dev", "test"} {
			for _, name := range []string{"SMEM_ALLOWED_ORIGINS", "SMEM_PUBLIC_BASE_URL"} {
				for _, v := range good {
					if _, err := load(env(map[string]string{"SMEM_ENV": e, "SMEM_DB_DSN": dsn, name: v})); err != nil {
						t.Errorf("%s=%s %s: %v", e, v, name, err)
					}
				}
				for _, v := range bad {
					_, err := load(env(map[string]string{"SMEM_ENV": e, "SMEM_DB_DSN": dsn, name: v}))
					if err == nil || !strings.Contains(err.Error(), name) || !strings.Contains(err.Error(), "SMEM_ENV=prod") {
						t.Errorf("%s %s=%s: want a local-only refusal naming the variable, got %v", e, name, v, err)
					}
				}
			}
		}
		// One bad entry in a list is enough.
		if _, err := load(env(map[string]string{"SMEM_ENV": "test", "SMEM_DB_DSN": dsn, "SMEM_ALLOWED_ORIGINS": "http://localhost:5173, https://app.example.com"})); err == nil {
			t.Error("a public origin in a list must be refused in test")
		}
	}
	// Prod is unchanged: a public host is what it needs.
	if _, err := Load(env(map[string]string{"SMEM_ENV": "prod", "SMEM_DB_DSN": dsn, "SMEM_ALLOWED_ORIGINS": "https://app.example.com", "SMEM_PUBLIC_BASE_URL": "https://app.example.com", "SMEM_OTP_KEY": strings.Repeat("k", 32), "SMEM_S3_BUCKET": "b"})); err != nil {
		t.Errorf("prod with a public host: %v", err)
	}
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(env(map[string]string{"SMEM_DB_DSN": dsn}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != ":8080" || cfg.Env != "dev" || cfg.LogLevel != slog.LevelInfo {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	if cfg.DBDSN != dsn || cfg.DBMaxOpen != 20 || cfg.DBMaxIdle != 20 || cfg.DBConnMaxLifetime != 5*time.Minute {
		t.Fatalf("unexpected db defaults: %+v", cfg)
	}
}

func TestLoadDSNRequiredOutsideTest(t *testing.T) {
	for _, e := range []string{"dev", "prod"} {
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
	// An unset SMEM_DB_MAX_IDLE follows SMEM_DB_MAX_OPEN.
	cfg, err = Load(env(map[string]string{"SMEM_DB_DSN": dsn, "SMEM_DB_MAX_OPEN": "7"}))
	if err != nil || cfg.DBMaxIdle != 7 {
		t.Fatalf("idle default: %+v, %v", cfg, err)
	}
}

func TestLoadValid(t *testing.T) {
	cfg, err := Load(env(map[string]string{"SMEM_HTTP_ADDR": "127.0.0.1:0", "SMEM_ENV": "prod", "SMEM_LOG_LEVEL": "debug", "SMEM_DB_DSN": dsn, "SMEM_ALLOWED_ORIGINS": "https://app.example.com", "SMEM_PUBLIC_BASE_URL": "https://app.example.com/", "SMEM_S3_BUCKET": "b", "SMEM_OTP_KEY": strings.Repeat("k", 32)}))
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
		cfg.RegisterPerHour != 5 || cfg.LoginFailsPerPair != 10 || cfg.LoginFailsPerIP != 100 || cfg.LoginFailsPerAcct != 50 {
		t.Fatalf("unexpected auth defaults: %+v", cfg)
	}
}

func TestLoadLoginFailsPerAccount(t *testing.T) {
	cfg, err := Load(env(map[string]string{"SMEM_ENV": "dev", "SMEM_DB_DSN": dsn, "SMEM_RATE_LOGIN_FAILS_PER_ACCOUNT": "7"}))
	if err != nil || cfg.LoginFailsPerAcct != 7 {
		t.Fatalf("got %d, %v", cfg.LoginFailsPerAcct, err)
	}
	if _, err := Load(env(map[string]string{"SMEM_ENV": "dev", "SMEM_DB_DSN": dsn, "SMEM_RATE_LOGIN_FAILS_PER_ACCOUNT": "-3"})); err == nil || !strings.Contains(err.Error(), "SMEM_RATE_LOGIN_FAILS_PER_ACCOUNT") {
		t.Errorf("negative value: got %v", err)
	}
}

func TestLoadAllowedOrigins(t *testing.T) {
	cfg, err := Load(env(map[string]string{"SMEM_DB_DSN": dsn, "SMEM_ENV": "prod", "SMEM_PUBLIC_BASE_URL": "https://app.example.com", "SMEM_ALLOWED_ORIGINS": " https://App.Example.com , https://www.example.com ", "SMEM_TRUST_PROXY": "true", "SMEM_S3_BUCKET": "b", "SMEM_OTP_KEY": strings.Repeat("k", 32)}))
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
	for k, v := range map[string]string{"SMEM_TRUST_PROXY": "maybe", "SMEM_AUTH_MAX_CONCURRENT_HASHES": "0", "SMEM_AUTH_ARGON_MEMORY_KIB": "99999999999", "SMEM_AUTH_ARGON_PARALLELISM": "300", "SMEM_RATE_REGISTER_PER_HOUR": "x", "SMEM_RATE_LOGIN_FAILS_PER_ACCOUNT": "0"} {
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
	if cfg.MediaMaxBytes != 10<<20 || cfg.MediaMaxConcurrent != 2 || cfg.PublicUploadsPerIP != 8 || cfg.PublicUploadConns != 48 || cfg.UploadTmpDir != "" || cfg.MemoryLimitMiB != 0 || cfg.S3Bucket != "smemories-dev" || cfg.S3Region != "us-east-1" || cfg.S3PathStyle {
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
		{"SMEM_MEDIA_MAX_BYTES", "0"}, {"SMEM_MEDIA_MAX_BYTES", "999999999999"}, {"SMEM_MEDIA_MAX_CONCURRENT", "0"}, {"SMEM_PUBLIC_UPLOAD_CONCURRENT_PER_IP", "0"}, {"SMEM_PUBLIC_UPLOAD_MAX_CONNS", "0"}, {"SMEM_UPLOAD_TMP_DIR", "/no/such/dir/for/smem"},
		{"SMEM_MEMORY_LIMIT_MIB", "-1"}, {"SMEM_MEMORY_LIMIT_MIB", "63"}, {"SMEM_MEMORY_LIMIT_MIB", "1048577"}, {"SMEM_MEMORY_LIMIT_MIB", "1g"},
		{"SMEM_S3_ENDPOINT", "s3:9000"}, {"SMEM_S3_PATH_STYLE", "maybe"},
	} {
		if _, err := Load(env(map[string]string{"SMEM_DB_DSN": dsn, c.key: c.val})); err == nil || !strings.Contains(err.Error(), c.key) {
			t.Errorf("%s=%q: want error naming the variable, got %v", c.key, c.val, err)
		}
	}
	dir := t.TempDir()
	if cfg, err = Load(env(map[string]string{"SMEM_DB_DSN": dsn, "SMEM_UPLOAD_TMP_DIR": dir, "SMEM_PUBLIC_UPLOAD_MAX_CONNS": "5"})); err != nil || cfg.UploadTmpDir != dir || cfg.PublicUploadConns != 5 {
		t.Errorf("upload settings: %+v, %v", cfg, err)
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
	cfg, err = Load(env(map[string]string{"SMEM_DB_DSN": dsn, "SMEM_PUBLIC_BASE_URL": "http://localhost:5173/"}))
	if err != nil || cfg.PublicBaseURL != "http://localhost:5173" {
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
		"SMEM_PUBLIC_BASE_URL": "http://localhost:5173", "SMEM_OIDC_COOKIE_KEY": strings.Repeat("k", 32),
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
		_, err := Load(env(map[string]string{"SMEM_ENV": e, "SMEM_DB_DSN": dsn, "SMEM_REDIS_URL": "", "SMEM_ALLOWED_ORIGINS": testOrigin(e), "SMEM_PUBLIC_BASE_URL": testOrigin(e), "SMEM_S3_BUCKET": "b"}))
		if err == nil || !strings.Contains(err.Error(), "SMEM_REDIS_URL") {
			t.Errorf("SMEM_ENV=%s without SMEM_REDIS_URL: want error naming it, got %v", e, err)
		}
	}
}

// T-052: the migration task has no Redis and no OTP key; only the API needs them.
func TestLoadMigrateNeedsNeitherRedisNorOTPKey(t *testing.T) {
	prod := map[string]string{"SMEM_ENV": "prod", "SMEM_DB_DSN": dsn, "SMEM_ALLOWED_ORIGINS": "https://a.example", "SMEM_PUBLIC_BASE_URL": "https://a.example", "SMEM_S3_BUCKET": "b", "SMEM_REDIS_URL": "", "SMEM_OTP_KEY": ""}
	if _, err := LoadMigrate(env(prod)); err != nil {
		t.Fatalf("LoadMigrate: %v", err)
	}
	// T-075: the media backfill (database and object store only) needs no web settings either.
	delete(prod, "SMEM_ALLOWED_ORIGINS")
	delete(prod, "SMEM_PUBLIC_BASE_URL")
	if _, err := LoadMigrate(env(prod)); err != nil {
		t.Fatalf("LoadMigrate without origins and base URL: %v", err)
	}
	api := maps.Clone(prod)
	delete(api, "SMEM_REDIS_URL") // env() then supplies one
	api["SMEM_OTP_KEY"] = strings.Repeat("k", 32)
	if _, err := Load(env(api)); err == nil || !strings.Contains(err.Error(), "SMEM_ALLOWED_ORIGINS") {
		t.Fatalf("Load must still require SMEM_ALLOWED_ORIGINS in prod, got %v", err)
	}
	prod["SMEM_PUBLIC_BASE_URL"] = "ftp://bad"
	if _, err := LoadMigrate(env(prod)); err == nil {
		t.Fatal("a set but invalid SMEM_PUBLIC_BASE_URL must still be rejected")
	}
	if _, err := Load(env(prod)); err == nil {
		t.Fatal("Load must still require SMEM_REDIS_URL")
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
		{"SMEM_REDIS_URL", "redis://:s3cret@h:99999/0"},
		{"SMEM_REDIS_URL", "redis://:s3cret@h:0/0"},
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

func TestLoadOTPKey(t *testing.T) {
	base := map[string]string{"SMEM_DB_DSN": dsn, "SMEM_S3_BUCKET": "b"}
	with := func(kv ...string) map[string]string {
		m := map[string]string{}
		for k, v := range base {
			m[k] = v
		}
		for i := 0; i < len(kv); i += 2 {
			m[kv[i]] = kv[i+1]
		}
		m["SMEM_ALLOWED_ORIGINS"], m["SMEM_PUBLIC_BASE_URL"] = testOrigin(m["SMEM_ENV"]), testOrigin(m["SMEM_ENV"])
		return m
	}
	key := strings.Repeat("k", 32)

	for _, e := range []string{"dev", "test"} { // a development key is used when none is set
		cfg, err := Load(env(with("SMEM_ENV", e)))
		if err != nil || len(cfg.OTPKey) < 32 {
			t.Errorf("SMEM_ENV=%s: key %d bytes, %v", e, len(cfg.OTPKey), err)
		}
	}
	if _, err := Load(env(with("SMEM_ENV", "prod"))); err == nil || !strings.Contains(err.Error(), "SMEM_OTP_KEY") {
		t.Errorf("prod without key: %v", err)
	}
	if _, err := Load(env(with("SMEM_ENV", "prod", "SMEM_OTP_KEY", strings.Repeat("k", 31)))); err == nil || !strings.Contains(err.Error(), "SMEM_OTP_KEY") {
		t.Errorf("short key: %v", err)
	}
	if _, err := Load(env(with("SMEM_ENV", "dev", "SMEM_OTP_KEY", "short"))); err == nil || !strings.Contains(err.Error(), "SMEM_OTP_KEY") {
		t.Errorf("short key in dev: %v", err)
	}
	cfg, err := Load(env(with("SMEM_ENV", "prod", "SMEM_OTP_KEY", key)))
	if err != nil || string(cfg.OTPKey) != key || cfg.DevFixedOTP != "" {
		t.Errorf("prod with key: %v", err)
	}
}

// DEV-SHORTCUT(otp): the API refuses to start with the fixed code in any environment but dev and test.
func TestLoadDevFixedOTP(t *testing.T) {
	load := func(envName, fixed string) (Config, error) {
		return Load(env(map[string]string{
			"SMEM_DB_DSN": dsn, "SMEM_ENV": envName, "SMEM_DEV_FIXED_OTP": fixed, "SMEM_OTP_KEY": strings.Repeat("k", 32),
			"SMEM_ALLOWED_ORIGINS": testOrigin(envName), "SMEM_PUBLIC_BASE_URL": testOrigin(envName), "SMEM_S3_BUCKET": "b",
		}))
	}
	for _, e := range []string{"dev", "test"} {
		cfg, err := load(e, "123123")
		if err != nil || cfg.DevFixedOTP != "123123" {
			t.Errorf("SMEM_ENV=%s: %q, %v", e, cfg.DevFixedOTP, err)
		}
	}
	if _, err := load("prod", "123123"); err == nil || !strings.Contains(err.Error(), "SMEM_DEV_FIXED_OTP") {
		t.Errorf("prod with the fixed code: want a refusal naming SMEM_DEV_FIXED_OTP, got %v", err)
	}
	for _, e := range []string{"production", "staging", "Dev"} { // already invalid environments
		if _, err := load(e, "123123"); err == nil {
			t.Errorf("SMEM_ENV=%q with the fixed code must not load", e)
		}
	}
	for _, bad := range []string{"12312", "1231234", "abcdef", "12 123", "١٢٣١٢٣"} {
		if _, err := load("dev", bad); err == nil || !strings.Contains(err.Error(), "SMEM_DEV_FIXED_OTP") {
			t.Errorf("fixed code %q: want an error, got %v", bad, err)
		}
	}
	for _, e := range []string{"prod", "dev"} { // unset is fine everywhere and means off
		if cfg, err := load(e, ""); err != nil || cfg.DevFixedOTP != "" {
			t.Errorf("SMEM_ENV=%q unset: %q, %v", e, cfg.DevFixedOTP, err)
		}
	}
}

func TestLoadMetricsAddr(t *testing.T) {
	cfg, err := Load(env(map[string]string{"SMEM_DB_DSN": dsn}))
	if err != nil || cfg.MetricsAddr != "" {
		t.Fatalf("default must be off: %q, %v", cfg.MetricsAddr, err)
	}
	cfg, err = Load(env(map[string]string{"SMEM_DB_DSN": dsn, "SMEM_METRICS_ADDR": "127.0.0.1:9090"}))
	if err != nil || cfg.MetricsAddr != "127.0.0.1:9090" {
		t.Fatalf("valid address: %q, %v", cfg.MetricsAddr, err)
	}
	for _, bad := range []string{"9090", "127.0.0.1:x", "127.0.0.1:0", ":8080"} {
		if _, err := Load(env(map[string]string{"SMEM_DB_DSN": dsn, "SMEM_METRICS_ADDR": bad})); err == nil || !strings.Contains(err.Error(), "SMEM_METRICS_ADDR") {
			t.Errorf("SMEM_METRICS_ADDR=%q: want an error naming the variable, got %v", bad, err)
		}
	}
}

func TestLoadShutdown(t *testing.T) {
	load := func(m map[string]string) (Config, error) {
		m["SMEM_DB_DSN"] = dsn
		return Load(env(m))
	}
	cfg, err := load(map[string]string{})
	if err != nil || cfg.ShutdownTimeout != 25*time.Second || cfg.ShutdownDelay != 0 {
		t.Fatalf("dev defaults: %+v %v", cfg, err)
	}
	prod := map[string]string{"SMEM_ENV": "prod", "SMEM_ALLOWED_ORIGINS": "https://a.example.com", "SMEM_PUBLIC_BASE_URL": "https://a.example.com", "SMEM_S3_BUCKET": "b", "SMEM_OTP_KEY": strings.Repeat("k", 32)}
	if cfg, err = load(maps.Clone(prod)); err != nil || cfg.ShutdownDelay != 5*time.Second {
		t.Fatalf("prod default delay: %+v %v", cfg, err)
	}
	cfg, err = load(map[string]string{"SMEM_SHUTDOWN_TIMEOUT": "20s", "SMEM_SHUTDOWN_DELAY": "3s"})
	if err != nil || cfg.ShutdownTimeout != 20*time.Second || cfg.ShutdownDelay != 3*time.Second {
		t.Fatalf("explicit: %+v %v", cfg, err)
	}
	for _, c := range []struct{ key, val string }{
		{"SMEM_SHUTDOWN_TIMEOUT", "25"}, {"SMEM_SHUTDOWN_TIMEOUT", "0s"}, {"SMEM_SHUTDOWN_TIMEOUT", "10m"},
		{"SMEM_SHUTDOWN_DELAY", "-1s"}, {"SMEM_SHUTDOWN_DELAY", "25s"}, {"SMEM_SHUTDOWN_DELAY", "soon"},
	} {
		if _, err := load(map[string]string{c.key: c.val}); err == nil || !strings.Contains(err.Error(), c.key) {
			t.Errorf("%s=%q: want an error naming the variable, got %v", c.key, c.val, err)
		}
	}
	// The one-off tasks do not read the shutdown settings.
	if _, err := LoadMigrate(env(map[string]string{"SMEM_DB_DSN": dsn, "SMEM_SHUTDOWN_TIMEOUT": "junk"})); err != nil {
		t.Errorf("LoadMigrate must ignore SMEM_SHUTDOWN_TIMEOUT: %v", err)
	}
}

// T-092: the secret of the public photo links. Prod refuses to start without a usable one and never echoes it; dev and test make a
// random per-process secret and flag it so main logs the WARN.
func TestLoadMediaLinkSecret(t *testing.T) {
	prod := func(kv ...string) map[string]string {
		m := map[string]string{"SMEM_ENV": "prod", "SMEM_DB_DSN": dsn, "SMEM_ALLOWED_ORIGINS": "https://a.example.com", "SMEM_PUBLIC_BASE_URL": "https://a.example.com",
			"SMEM_S3_BUCKET": "b", "SMEM_OTP_KEY": strings.Repeat("k", 32)}
		for i := 0; i < len(kv); i += 2 {
			m[kv[i]] = kv[i+1]
		}
		return m
	}
	cfg, err := Load(env(prod()))
	if err != nil || len(cfg.MediaLinkSecret) != 48 || cfg.MediaLinkSecretRandom {
		t.Fatalf("prod with a valid secret: %d bytes, random=%v, %v", len(cfg.MediaLinkSecret), cfg.MediaLinkSecretRandom, err)
	}
	unpadded := strings.TrimRight(base64.StdEncoding.EncodeToString([]byte(strings.Repeat("s", 47))), "=")
	if cfg, err = Load(env(prod("SMEM_MEDIA_LINK_SECRET", unpadded))); err != nil || len(cfg.MediaLinkSecret) != 47 {
		t.Fatalf("unpadded base64: %d bytes, %v", len(cfg.MediaLinkSecret), err)
	}
	short := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("s", 31)))
	for name, v := range map[string]string{"unset": "", "31 bytes": short, "not base64": "not*base64*but-long-enough-to-pass-a-length-check"} {
		_, err := Load(env(prod("SMEM_MEDIA_LINK_SECRET", v)))
		if err == nil || !strings.Contains(err.Error(), "SMEM_MEDIA_LINK_SECRET") {
			t.Errorf("prod, %s: want a refusal naming the variable, got %v", name, err)
		} else if v != "" && strings.Contains(err.Error(), v) {
			t.Errorf("prod, %s: the refusal repeats the value: %v", name, err)
		}
	}
	for _, e := range []string{"dev", "test"} {
		// dev and test are local-only (T-099): local origin and base URL.
		a, err := Load(env(prod("SMEM_ENV", e, "SMEM_MEDIA_LINK_SECRET", "", "SMEM_ALLOWED_ORIGINS", "http://localhost:5173", "SMEM_PUBLIC_BASE_URL", "http://localhost:5173")))
		b, err2 := Load(env(prod("SMEM_ENV", e, "SMEM_MEDIA_LINK_SECRET", "", "SMEM_ALLOWED_ORIGINS", "http://localhost:5173", "SMEM_PUBLIC_BASE_URL", "http://localhost:5173")))
		if err != nil || err2 != nil || len(a.MediaLinkSecret) < 32 || !a.MediaLinkSecretRandom {
			t.Fatalf("%s, unset: %d bytes, random=%v, %v %v", e, len(a.MediaLinkSecret), a.MediaLinkSecretRandom, err, err2)
		}
		if string(a.MediaLinkSecret) == string(b.MediaLinkSecret) {
			t.Errorf("%s: two loads made the same random secret", e)
		}
	}
	if _, err := Load(env(prod("SMEM_ENV", "dev", "SMEM_MEDIA_LINK_SECRET", short, "SMEM_ALLOWED_ORIGINS", "http://localhost:5173", "SMEM_PUBLIC_BASE_URL", "http://localhost:5173"))); err == nil {
		t.Error("dev with a set but short secret must be refused too")
	}
	if _, err := LoadMigrate(env(prod("SMEM_MEDIA_LINK_SECRET", ""))); err != nil {
		t.Errorf("the migrate task never serves photos and needs no secret: %v", err)
	}
}
