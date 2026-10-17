package config

import (
	"log/slog"
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
		"SMEM_MAIL_PROVIDER":    "ses",
		"SMEM_MAIL_FROM":        "SMemories <no-reply@mail.smemories.io>",
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
		"https custom S3 endpoint":         with("SMEM_S3_ENDPOINT", "https://s3.internal.example.com"),
		"http S3 endpoint, opted":          with("SMEM_S3_ENDPOINT", "http://minio:9000", "SMEM_S3_ALLOW_INSECURE", "true"),
		"static S3 keys (MinIO-like)":      with("SMEM_S3_ACCESS_KEY", "k", "SMEM_S3_SECRET_KEY", secS3),
		"metrics on loopback":              with("SMEM_METRICS_ADDR", "127.0.0.1:9090"),
		"metrics on localhost":             with("SMEM_METRICS_ADDR", "localhost:9090"),
		"metrics on a private address":     with("SMEM_METRICS_ADDR", "10.0.1.5:9090"),
		"metrics on all interfaces, opted": with("SMEM_METRICS_ADDR", "0.0.0.0:9090", "SMEM_METRICS_ALLOW_ANY_INTERFACE", "true"),
		"bare from address":                with("SMEM_MAIL_FROM", "no-reply@mail.smemories.io"),
		"configuration set":                with("SMEM_SES_CONFIGURATION_SET", "smem-prod"),
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
		{"http S3 endpoint", with("SMEM_S3_ENDPOINT", "http://minio:9000"), "SMEM_S3_ENDPOINT"},
		{"S3 endpoint with credentials", with("SMEM_S3_ENDPOINT", "https://k:"+secS3+"@s3.example.com"), "SMEM_S3_ENDPOINT"},
		{"http S3 endpoint with credentials", with("SMEM_S3_ENDPOINT", "http://k:"+secS3+"@minio:9000"), "SMEM_S3_ENDPOINT"},
		{"bad S3 opt-out value", with("SMEM_S3_ALLOW_INSECURE", "sure"), "SMEM_S3_ALLOW_INSECURE"},
		{"origin with credentials", with("SMEM_ALLOWED_ORIGINS", "https://u:"+secS3+"@app.example.com"), "SMEM_ALLOWED_ORIGINS"},
		{"http origin with credentials", with("SMEM_ALLOWED_ORIGINS", "http://u:"+secS3+"@app.example.com"), "SMEM_ALLOWED_ORIGINS"},
		{"base URL with credentials", with("SMEM_PUBLIC_BASE_URL", "https://u:"+secS3+"@app.example.com"), "SMEM_PUBLIC_BASE_URL"},
		{"http base URL with credentials", with("SMEM_PUBLIC_BASE_URL", "http://u:"+secS3+"@app.example.com"), "SMEM_PUBLIC_BASE_URL"},
		{"no bucket", with("SMEM_S3_BUCKET", ""), "SMEM_S3_BUCKET"},
		{"access key without secret", with("SMEM_S3_ACCESS_KEY", "k"), "SMEM_S3_ACCESS_KEY"},
		{"secret without access key", with("SMEM_S3_SECRET_KEY", secS3), "SMEM_S3_SECRET_KEY"},
		{"log mailer", with("SMEM_MAIL_PROVIDER", "log"), "SMEM_MAIL_PROVIDER"},
		{"no mail provider", with("SMEM_MAIL_PROVIDER", ""), "SMEM_MAIL_PROVIDER"},
		{"unknown mail provider", with("SMEM_MAIL_PROVIDER", "sendgrid"), "SMEM_MAIL_PROVIDER"},
		{"no sender", with("SMEM_MAIL_FROM", ""), "SMEM_MAIL_FROM"},
		{"malformed sender", with("SMEM_MAIL_FROM", "SMemories no-reply"), "SMEM_MAIL_FROM"},
		{"sender on example.com", with("SMEM_MAIL_FROM", "no-reply@example.com"), "SMEM_MAIL_FROM"},
		{"sender on a subdomain of example.org", with("SMEM_MAIL_FROM", "SMemories <no-reply@mail.Example.ORG>"), "SMEM_MAIL_FROM"},
		{"sender on .test", with("SMEM_MAIL_FROM", "no-reply@smemories.test"), "SMEM_MAIL_FROM"},
		{"sender on localhost", with("SMEM_MAIL_FROM", "no-reply@localhost"), "SMEM_MAIL_FROM"},
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

// A URL with credentials is refused outside prod too, and the message never repeats the password.
func TestURLCredentialsRefusedEverywhereAndRedacted(t *testing.T) {
	for _, k := range []string{"SMEM_ALLOWED_ORIGINS", "SMEM_PUBLIC_BASE_URL", "SMEM_S3_ENDPOINT"} {
		m := with("SMEM_ENV", "dev", "SMEM_DB_DSN", "app:p@tcp(db:3306)/d", "SMEM_REDIS_URL", "redis://127.0.0.1:6379/0", k, "http://u:"+secS3+"@localhost:5173")
		_, err := Load(exact(m))
		if err == nil || !strings.Contains(err.Error(), k) || strings.Contains(err.Error(), secS3) {
			t.Errorf("%s: want a refusal naming it without the password, got %v", k, err)
		}
	}
	for _, k := range []string{"SMEM_ALLOWED_ORIGINS", "SMEM_PUBLIC_BASE_URL"} { // both refuse a query; the message must not repeat it (T-090)
		m := with("SMEM_ENV", "dev", "SMEM_DB_DSN", "app:p@tcp(db:3306)/d", "SMEM_REDIS_URL", "redis://127.0.0.1:6379/0", k, "http://localhost:5173?token="+secS3)
		_, err := Load(exact(m))
		if err == nil || !strings.Contains(err.Error(), k) || strings.Contains(err.Error(), secS3) {
			t.Errorf("%s: want a refusal naming it without the query token, got %v", k, err)
		}
	}
	for in, want := range map[string]string{
		"https://app.example.com":        "https://app.example.com",
		"https://u:pw@app.example.com/x": "https://***@app.example.com/x",
		"u:pw@app.example.com":           "***",
		"http://u:p w@host":              "***",
		"not a url":                      "not a url",
		// T-090: a query string or fragment can carry a token too.
		"https://app.example.com/?token=abc123":     "https://app.example.com/?***",
		"https://app.example.com?token=abc123#frag": "https://app.example.com?***#***",
		"https://u:pw@app.example.com?t=abc":        "https://***@app.example.com?***",
		"https://app.example.com/%zz?token=abc":     "***",
	} {
		if got := redactURL(in); got != want {
			t.Errorf("redactURL(%q) = %q, want %q", in, got, want)
		}
	}
}

// Every binary logs the opt-outs and the static-key override at start-up, in prod only, without a value.
func TestLogWarnings(t *testing.T) {
	var buf strings.Builder
	l := slog.New(slog.NewJSONHandler(&buf, nil))
	cfg, err := Load(exact(with("SMEM_DB_ALLOW_INSECURE", "true", "SMEM_REDIS_ALLOW_INSECURE", "true", "SMEM_S3_ENDPOINT", "http://minio:9000", "SMEM_S3_ALLOW_INSECURE", "true",
		"SMEM_S3_ACCESS_KEY", "k", "SMEM_S3_SECRET_KEY", secS3, "SMEM_METRICS_ADDR", "0.0.0.0:9090", "SMEM_METRICS_ALLOW_ANY_INTERFACE", "true")))
	if err != nil {
		t.Fatal(err)
	}
	cfg.LogWarnings(l)
	for _, v := range []string{"SMEM_DB_ALLOW_INSECURE", "SMEM_REDIS_ALLOW_INSECURE", "SMEM_S3_ALLOW_INSECURE", "SMEM_METRICS_ALLOW_ANY_INTERFACE", "SMEM_S3_ACCESS_KEY"} {
		if !strings.Contains(buf.String(), v) {
			t.Errorf("no WARN naming %s: %s", v, buf.String())
		}
	}
	if strings.Contains(buf.String(), secS3) || strings.Contains(buf.String(), `"level":"INFO"`) {
		t.Errorf("unexpected content: %s", buf.String())
	}
	buf.Reset()
	cfg.Env = "dev"
	cfg.LogWarnings(l)
	if buf.Len() != 0 {
		t.Errorf("warned outside prod: %s", buf.String())
	}
	buf.Reset()
	clean, err := Load(exact(prodBase()))
	if err != nil {
		t.Fatal(err)
	}
	clean.LogWarnings(l)
	if buf.Len() != 0 {
		t.Errorf("clean prod config warned: %s", buf.String())
	}
}

// The mail settings outside prod: the log mailer needs nothing, ses needs a well-formed sender, an example domain is fine in dev.
func TestMailSettingsOutsideProd(t *testing.T) {
	dev := func(kv ...string) map[string]string {
		return with(append([]string{"SMEM_ENV", "dev", "SMEM_DB_DSN", "app:p@tcp(db:3306)/d", "SMEM_REDIS_URL", "redis://127.0.0.1:6379/0",
			"SMEM_ALLOWED_ORIGINS", "http://localhost:5173", "SMEM_PUBLIC_BASE_URL", "http://localhost:5173", "SMEM_MEMORY_LIMIT_MIB", "",
			"SMEM_MAIL_PROVIDER", "", "SMEM_MAIL_FROM", ""}, kv...)...)
	}
	cfg, err := Load(exact(dev()))
	if err != nil || cfg.MailProvider != "" {
		t.Fatalf("default: %v %q", err, cfg.MailProvider)
	}
	if _, err := Load(exact(dev("SMEM_MAIL_PROVIDER", "log"))); err != nil {
		t.Errorf("log: %v", err)
	}
	if cfg, err = Load(exact(dev("SMEM_MAIL_PROVIDER", "ses", "SMEM_MAIL_FROM", "no-reply@example.com", "SMEM_SES_CONFIGURATION_SET", "x"))); err != nil || cfg.MailFrom == "" || cfg.SESConfigurationSet != "x" {
		t.Errorf("ses on an example domain in dev: %v", err)
	}
	if _, err := Load(exact(dev("SMEM_MAIL_PROVIDER", "ses"))); err == nil || !strings.Contains(err.Error(), "SMEM_MAIL_FROM") {
		t.Errorf("ses without a sender: %v", err)
	}
}

// The one-off tasks do not read mail settings.
func TestMailSettingsNotNeededByOneOffTasks(t *testing.T) {
	task := map[string]string{"SMEM_ENV": "prod", "SMEM_DB_DSN": prodBase()["SMEM_DB_DSN"], "SMEM_S3_BUCKET": "smemories-prod"}
	if _, err := LoadMigrate(exact(task)); err != nil {
		t.Fatal(err)
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
