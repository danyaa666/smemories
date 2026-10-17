package config

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Distinctive secret values: no refusal message may contain any of them.
const (
	secDBPass = "dbpass-SECRET-1"
	secRedis  = "redis-SECRET-2"
	secOTP    = "otp-key-SECRET-3-0123456789abcdef"
	secOIDC   = "oidc-key-SECRET-4-0123456789abcdef"
	secS3     = "s3-SECRET-5"
)

// prodBase is a complete, valid production environment for the API (what the Fargate task definition provides).
func prodBase() map[string]string {
	return map[string]string{
		"SMEM_ENV":              "prod",
		"SMEM_DB_DSN":           "app:" + secDBPass + "@tcp(db.internal:3306)/smemories?tls=true",
		"SMEM_REDIS_URL":        "rediss://:" + secRedis + "@cache.internal:6379/0",
		"SMEM_ALLOWED_ORIGINS":  "https://app.example.com",
		"SMEM_PUBLIC_BASE_URL":  "https://app.example.com",
		"SMEM_OTP_KEY":          secOTP,
		"SMEM_S3_BUCKET":        "smemories-prod",
		"SMEM_TRUST_PROXY":      "true",
		"SMEM_MEMORY_LIMIT_MIB": "1638",
	}
}

func exact(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

// with returns prodBase changed by kv pairs; an empty value removes the variable.
func with(kv ...string) map[string]string {
	m := prodBase()
	for i := 0; i < len(kv); i += 2 {
		m[kv[i]] = kv[i+1]
	}
	return m
}

func TestProdContractValid(t *testing.T) {
	ca := filepath.Join(t.TempDir(), "rds-ca.pem")
	if err := os.WriteFile(ca, []byte("not parsed here"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, m := range map[string]map[string]string{
		"base (task role, no static keys)": prodBase(),
		"CA file instead of tls=true":      with("SMEM_DB_DSN", "app:p@tcp(db:3306)/d", "SMEM_DB_TLS_CA_FILE", ca),
		"explicit DB opt-out":              with("SMEM_DB_DSN", "app:p@tcp(db:3306)/d", "SMEM_DB_ALLOW_INSECURE", "true"),
		"redis password in its own var":    with("SMEM_REDIS_URL", "rediss://cache.internal:6379/0", "SMEM_REDIS_PASSWORD", secRedis),
		"redis opt-out":                    with("SMEM_REDIS_URL", "redis://cache:6379/0", "SMEM_REDIS_ALLOW_INSECURE", "true"),
		"static S3 keys (MinIO-like)":      with("SMEM_S3_ACCESS_KEY", "k", "SMEM_S3_SECRET_KEY", secS3),
		"metrics on loopback":              with("SMEM_METRICS_ADDR", "127.0.0.1:9090"),
		"metrics on localhost":             with("SMEM_METRICS_ADDR", "localhost:9090"),
		"metrics on a private address":     with("SMEM_METRICS_ADDR", "10.0.1.5:9090"),
		"metrics on all interfaces, opted": with("SMEM_METRICS_ADDR", "0.0.0.0:9090", "SMEM_METRICS_ALLOW_ANY_INTERFACE", "true"),
		"google on with a strong key":      with("SMEM_GOOGLE_CLIENT_ID", "id", "SMEM_GOOGLE_CLIENT_SECRET", "s", "SMEM_OIDC_COOKIE_KEY", secOIDC),
	} {
		if _, err := Load(exact(m)); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// Every missing or unsafe value of the production contract is refused, the message names the variable, and no secret is echoed.
func TestProdContractRefusals(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want string // the variable the message must name
	}{
		{"no DSN", with("SMEM_DB_DSN", ""), "SMEM_DB_DSN"},
		{"DSN without tls", with("SMEM_DB_DSN", "app:"+secDBPass+"@tcp(db:3306)/d"), "SMEM_DB_DSN"},
		{"DSN tls=false", with("SMEM_DB_DSN", "app:"+secDBPass+"@tcp(db:3306)/d?tls=false"), "SMEM_DB_DSN"},
		{"DSN tls=skip-verify", with("SMEM_DB_DSN", "app:"+secDBPass+"@tcp(db:3306)/d?tls=skip-verify"), "SMEM_DB_DSN"},
		{"DSN tls=preferred", with("SMEM_DB_DSN", "app:"+secDBPass+"@tcp(db:3306)/d?tls=preferred"), "SMEM_DB_DSN"},
		{"DSN unparsable", with("SMEM_DB_DSN", "not a dsn "+secDBPass), "SMEM_DB_DSN"},
		{"CA file missing", with("SMEM_DB_TLS_CA_FILE", "/nonexistent/ca.pem"), "SMEM_DB_TLS_CA_FILE"},
		{"bad DB opt-out value", with("SMEM_DB_ALLOW_INSECURE", "yes please"), "SMEM_DB_ALLOW_INSECURE"},
		{"no Redis URL", with("SMEM_REDIS_URL", ""), "SMEM_REDIS_URL"},
		{"redis:// without TLS", with("SMEM_REDIS_URL", "redis://:"+secRedis+"@cache:6379/0"), "SMEM_REDIS_URL"},
		{"rediss:// without credentials", with("SMEM_REDIS_URL", "rediss://cache:6379/0"), "SMEM_REDIS_URL"},
		{"no OTP key", with("SMEM_OTP_KEY", ""), "SMEM_OTP_KEY"},
		{"short OTP key", with("SMEM_OTP_KEY", "short-"+secOTP[:10]), "SMEM_OTP_KEY"},
		{"google without cookie key", with("SMEM_GOOGLE_CLIENT_ID", "id", "SMEM_GOOGLE_CLIENT_SECRET", "s"), "SMEM_OIDC_COOKIE_KEY"},
		{"google with a short cookie key", with("SMEM_GOOGLE_CLIENT_ID", "id", "SMEM_GOOGLE_CLIENT_SECRET", "s", "SMEM_OIDC_COOKIE_KEY", "short"), "SMEM_OIDC_COOKIE_KEY"},
		{"google with an http issuer", with("SMEM_GOOGLE_CLIENT_ID", "id", "SMEM_GOOGLE_CLIENT_SECRET", "s", "SMEM_OIDC_COOKIE_KEY", secOIDC, "SMEM_GOOGLE_ISSUER", "http://idp.example.com"), "SMEM_GOOGLE_ISSUER"},
		{"no origins", with("SMEM_ALLOWED_ORIGINS", ""), "SMEM_ALLOWED_ORIGINS"},
		{"http origin", with("SMEM_ALLOWED_ORIGINS", "http://app.example.com"), "SMEM_ALLOWED_ORIGINS"},
		{"one http origin in a list", with("SMEM_ALLOWED_ORIGINS", "https://app.example.com,http://localhost:5173"), "SMEM_ALLOWED_ORIGINS"},
		{"no base URL", with("SMEM_PUBLIC_BASE_URL", ""), "SMEM_PUBLIC_BASE_URL"},
		{"http base URL", with("SMEM_PUBLIC_BASE_URL", "http://app.example.com"), "SMEM_PUBLIC_BASE_URL"},
		{"no bucket", with("SMEM_S3_BUCKET", ""), "SMEM_S3_BUCKET"},
		{"access key without secret", with("SMEM_S3_ACCESS_KEY", "k"), "SMEM_S3_ACCESS_KEY"},
		{"secret without access key", with("SMEM_S3_SECRET_KEY", secS3), "SMEM_S3_SECRET_KEY"},
		{"dev fixed code", with("SMEM_DEV_FIXED_OTP", "123123"), "SMEM_DEV_FIXED_OTP"},
		{"no memory limit", with("SMEM_MEMORY_LIMIT_MIB", ""), "SMEM_MEMORY_LIMIT_MIB"},
		{"metrics on 0.0.0.0", with("SMEM_METRICS_ADDR", "0.0.0.0:9090"), "SMEM_METRICS_ADDR"},
		{"metrics on :port", with("SMEM_METRICS_ADDR", ":9090"), "SMEM_METRICS_ADDR"},
		{"metrics on [::]", with("SMEM_METRICS_ADDR", "[::]:9090"), "SMEM_METRICS_ADDR"},
		{"metrics on a public IP", with("SMEM_METRICS_ADDR", "8.8.8.8:9090"), "SMEM_METRICS_ADDR"},
		{"metrics on a public name", with("SMEM_METRICS_ADDR", "metrics.example.com:9090"), "SMEM_METRICS_ADDR"},
		{"bad metrics opt-out value", with("SMEM_METRICS_ALLOW_ANY_INTERFACE", "sure"), "SMEM_METRICS_ALLOW_ANY_INTERFACE"},
	}
	secrets := []string{secDBPass, secRedis, secOTP, secOIDC, secS3, "123123"}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Load(exact(c.env))
			if err == nil {
				t.Fatal("the loader accepted it")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("message does not name %s: %v", c.want, err)
			}
			for _, s := range secrets {
				if strings.Contains(err.Error(), s) {
					t.Errorf("message echoes a secret (%q): %v", s, err)
				}
			}
		})
	}
}

// Dev and test stay easy: the same values that prod refuses load there.
func TestProdRulesDoNotApplyOutsideProd(t *testing.T) {
	for _, e := range []string{"dev", "test"} {
		m := with("SMEM_ENV", e, "SMEM_DB_DSN", "app:p@tcp(db:3306)/d", "SMEM_REDIS_URL", "redis://127.0.0.1:6379/0", "SMEM_ALLOWED_ORIGINS", "http://localhost:5173", "SMEM_PUBLIC_BASE_URL", "http://localhost:5173",
			"SMEM_METRICS_ADDR", "0.0.0.0:9090", "SMEM_MEMORY_LIMIT_MIB", "")
		if _, err := Load(exact(m)); err != nil {
			t.Errorf("SMEM_ENV=%s: %v", e, err)
		}
	}
}

// The one-off tasks (migrate, backfill) need the database and the object store only, but the same TLS rules apply to them.
func TestProdContractForOneOffTasks(t *testing.T) {
	task := map[string]string{"SMEM_ENV": "prod", "SMEM_DB_DSN": prodBase()["SMEM_DB_DSN"], "SMEM_S3_BUCKET": "smemories-prod"}
	if _, err := LoadMigrate(exact(task)); err != nil {
		t.Fatalf("migrate task with DSN and bucket only: %v", err)
	}
	task["SMEM_DB_DSN"] = "app:" + secDBPass + "@tcp(db:3306)/d"
	_, err := LoadMigrate(exact(task))
	if err == nil || !strings.Contains(err.Error(), "SMEM_DB_DSN") || strings.Contains(err.Error(), secDBPass) {
		t.Fatalf("migrate task with a plaintext DSN: %v", err)
	}
	task["SMEM_DB_ALLOW_INSECURE"] = "true"
	if _, err := LoadMigrate(exact(task)); err != nil {
		t.Fatalf("with the opt-out: %v", err)
	}
}

// docs/production-config.md lists every variable the loader reads: add a setting, add its row.
func TestEveryVariableIsDocumented(t *testing.T) {
	doc, err := os.ReadFile("../../docs/production-config.md")
	if err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob("*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("no sources found: %v", err)
	}
	name := regexp.MustCompile(`"(SMEM_[A-Z0-9_]+)"`)
	seen := map[string]bool{}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range name.FindAllSubmatch(src, -1) {
			seen[string(m[1])] = true
		}
	}
	if len(seen) < 40 {
		t.Fatalf("found only %d variables in the sources: the scan is broken", len(seen))
	}
	for v := range seen {
		if !regexp.MustCompile("(?m)^\\| `" + v + "` \\|").Match(doc) {
			t.Errorf("%s is read by internal/config but has no row in docs/production-config.md", v)
		}
	}
}
