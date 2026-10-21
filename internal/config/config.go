// Package config reads the process configuration from SMEM_* environment variables.
package config

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net"
	"net/mail"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/danyaa666/smemories/internal/medialink"
)

// Config is the validated process configuration.
type Config struct {
	HTTPAddr    string     // SMEM_HTTP_ADDR, host:port to listen on
	Env         string     // SMEM_ENV: dev, test or prod
	LogLevel    slog.Level // SMEM_LOG_LEVEL: debug, info, warn or error
	MetricsAddr string     // SMEM_METRICS_ADDR: host:port of the internal /metrics listener, empty = off (never the public address)

	DBDSN             string        // SMEM_DB_DSN: user:pass@tcp(host:port)/dbname; required unless Env is "test" (db.Open still rejects an empty one). Never log it.
	DBMaxOpen         int           // SMEM_DB_MAX_OPEN, default 20
	DBMaxIdle         int           // SMEM_DB_MAX_IDLE, default DBMaxOpen (a smaller idle pool closes and redials connections under steady load)
	DBConnMaxLifetime time.Duration // SMEM_DB_CONN_MAX_LIFETIME, default 5m

	AllowedOrigins    []string      // SMEM_ALLOWED_ORIGINS: comma-separated browser origins; required in prod, default http://localhost:5173 otherwise
	PublicBaseURL     string        // SMEM_PUBLIC_BASE_URL: where the web app lives, used for links in emails; required in prod, default http://localhost:5173 otherwise; no trailing slash
	RequestTimeout    time.Duration // SMEM_HTTP_REQUEST_TIMEOUT, default 30s, minimum 1s: deadline of every request context unless a route sets another
	TrustProxy        bool          // SMEM_TRUST_PROXY: take the client IP from the last X-Forwarded-For hop
	MaxHashes         int           // SMEM_AUTH_MAX_CONCURRENT_HASHES, default 4
	ArgonMemoryKiB    uint32        // SMEM_AUTH_ARGON_MEMORY_KIB, default 19456
	ArgonTime         uint32        // SMEM_AUTH_ARGON_TIME, default 2
	ArgonParallelism  uint8         // SMEM_AUTH_ARGON_PARALLELISM, default 1
	RegisterPerHour   int           // SMEM_RATE_REGISTER_PER_HOUR (per IP), default 5
	LoginFailsPerPair int           // SMEM_RATE_LOGIN_FAILS_PER_EMAIL (per IP+email, 15 min), default 10
	LoginFailsPerIP   int           // SMEM_RATE_LOGIN_FAILS_PER_IP (15 min), default 100
	LoginFailsPerAcct int           // SMEM_RATE_LOGIN_FAILS_PER_ACCOUNT (per account, 15 min), default 50

	GoogleClientID     string // SMEM_GOOGLE_CLIENT_ID: empty = Google sign-in off (the endpoints answer 404)
	GoogleClientSecret string // SMEM_GOOGLE_CLIENT_SECRET: required with a client id. Never log it.
	GoogleIssuer       string // SMEM_GOOGLE_ISSUER, default https://accounts.google.com (tests point it at a fake)
	OIDCCookieKey      []byte // SMEM_OIDC_COOKIE_KEY: HMAC key for the smem_oidc cookie, at least 32 bytes; required with a client id. Never log it.
	MediaMaxBytes      int64  // SMEM_MEDIA_MAX_BYTES: largest accepted upload, default 10 MiB
	PublicUploadsPerIP int    // SMEM_PUBLIC_UPLOAD_CONCURRENT_PER_IP: public note submissions in progress per client IP, default 8
	PublicUploadConns  int    // SMEM_PUBLIC_UPLOAD_MAX_CONNS: public note submissions in progress in total, default 48
	UploadTmpDir       string // SMEM_UPLOAD_TMP_DIR: where public submissions are spooled while they arrive, default the OS temp dir
	MediaMaxConcurrent int    // SMEM_MEDIA_MAX_CONCURRENT: images processed at once, default 2 (see docs/media.md)
	MemoryLimitMiB     int    // SMEM_MEMORY_LIMIT_MIB: soft memory limit of the Go runtime in MiB, 0 = unset
	S3Endpoint         string // SMEM_S3_ENDPOINT: empty for AWS S3, e.g. http://127.0.0.1:9000 for MinIO
	S3Region           string // SMEM_S3_REGION, default us-east-1
	S3Bucket           string // SMEM_S3_BUCKET: required in prod, default smemories-dev otherwise
	S3AccessKey        string // SMEM_S3_ACCESS_KEY: never log
	S3SecretKey        string // SMEM_S3_SECRET_KEY: never log
	S3PathStyle        bool   // SMEM_S3_PATH_STYLE: true for MinIO

	// Production contract (prod.go, docs/production-config.md). The three opt-outs are read in every environment but only matter in prod.
	DBTLSCAFile              string // SMEM_DB_TLS_CA_FILE: PEM bundle (the RDS CA) the database certificate is verified against; makes the DSN need no tls= parameter
	S3AllowInsecure          bool   // SMEM_S3_ALLOW_INSECURE: prod may use an http:// SMEM_S3_ENDPOINT (the image smoke test on a private network only)
	DBAllowInsecure          bool   // SMEM_DB_ALLOW_INSECURE: prod may use a DSN without TLS (the image smoke test on a private network only)
	RedisAllowInsecure       bool   // SMEM_REDIS_ALLOW_INSECURE: prod may use redis:// without TLS or credentials (same)
	MetricsAllowAnyInterface bool   // SMEM_METRICS_ALLOW_ANY_INTERFACE: prod may bind SMEM_METRICS_ADDR to all interfaces (task network only, never behind the ALB)

	RedisURL          string        // SMEM_REDIS_URL: redis[s]://[:password@]host:port/db; required in every environment. Never log it.
	RedisPassword     string        // SMEM_REDIS_PASSWORD: used when the URL carries none. Never log it.
	RedisDialTimeout  time.Duration // SMEM_REDIS_DIAL_TIMEOUT, default 2s
	RedisReadTimeout  time.Duration // SMEM_REDIS_READ_TIMEOUT, default 1s
	RedisWriteTimeout time.Duration // SMEM_REDIS_WRITE_TIMEOUT, default 1s
	RedisPoolSize     int           // SMEM_REDIS_POOL_SIZE, default 10

	ShutdownTimeout time.Duration // SMEM_SHUTDOWN_TIMEOUT, default 25s: after SIGTERM the delay below plus the drain of in-flight requests; keep it below the task stop timeout (30s on Fargate)
	ShutdownDelay   time.Duration // SMEM_SHUTDOWN_DELAY, default 5s in prod and 0 elsewhere: /readyz answers 503 this long before the listener closes, so the load balancer drains first

	OTPKey              []byte // SMEM_OTP_KEY: HMAC key of the email codes, at least 32 bytes; required unless Env is dev or test (a fixed development key is used there). Never log it.
	MailProvider        string // SMEM_MAIL_PROVIDER: log (default; dev and test only) or ses; mailer.New validates the name
	MailFrom            string // SMEM_MAIL_FROM: "SMemories <no-reply@mail.example.com>"; required with ses and in prod
	SESConfigurationSet string // SMEM_SES_CONFIGURATION_SET: optional SES configuration set (reputation metrics)

	MediaLinkSecret       []byte // SMEM_MEDIA_LINK_SECRET: HMAC key of the public photo links (T-092), base64, at least 32 bytes decoded; required in prod. Never log it.
	MediaLinkSecretRandom bool   // true when it was unset (dev, test) and a random per-process secret was made: links stop working when the API restarts

	DevFixedOTP string // DEV-SHORTCUT(otp) SMEM_DEV_FIXED_OTP: 6 digits accepted as every email code; only with SMEM_ENV=dev or test, otherwise the API refuses to start (docs/dev-shortcuts.md)
}

// devOTPKey is the HMAC key of the email codes in dev and test when SMEM_OTP_KEY is unset. Not a secret.
const devOTPKey = "smemories-dev-only-otp-key-not-a-secret"

const (
	minOTPKeyBytes    = 32
	minOIDCKeyBytes   = 32
	minMediaLinkBytes = medialink.MinSecretBytes
	minArgonMemoryKiB = 8
)

var logLevels = map[string]slog.Level{
	"debug": slog.LevelDebug,
	"info":  slog.LevelInfo,
	"warn":  slog.LevelWarn,
	"error": slog.LevelError,
}

// Load builds a Config from getenv (os.Getenv in production). An unset or empty
// variable takes its default; an invalid value returns an error naming the variable.
func Load(getenv func(string) string) (Config, error) { return load(getenv, true) }

// LoadMigrate is Load for the one-off tasks (smemories-migrate, smemories-media-backfill): it skips the settings only the API
// needs (Redis, email codes, allowed origins, public base URL), because those tasks have no access to Redis (T-022) and must not
// fail on a missing SMEM_REDIS_URL, SMEM_OTP_KEY or SMEM_ALLOWED_ORIGINS. Values that are set are still validated.
func LoadMigrate(getenv func(string) string) (Config, error) { return load(getenv, false) }

func load(getenv func(string) string, forAPI bool) (Config, error) {
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

	if cfg.MetricsAddr = getenv("SMEM_METRICS_ADDR"); cfg.MetricsAddr != "" {
		if _, port, err := net.SplitHostPort(cfg.MetricsAddr); err != nil {
			return Config{}, fmt.Errorf("SMEM_METRICS_ADDR=%q: want host:port, e.g. 127.0.0.1:9090", cfg.MetricsAddr)
		} else if n, err := strconv.ParseUint(port, 10, 16); err != nil || n == 0 {
			return Config{}, fmt.Errorf("SMEM_METRICS_ADDR=%q: port must be a number 1-65535", cfg.MetricsAddr)
		} else if cfg.MetricsAddr == cfg.HTTPAddr {
			return Config{}, fmt.Errorf("SMEM_METRICS_ADDR=%q: must differ from SMEM_HTTP_ADDR, metrics are never served on the public listener", cfg.MetricsAddr)
		}
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
	if cfg.DBMaxIdle, err = getInt(get, "SMEM_DB_MAX_IDLE", strconv.Itoa(cfg.DBMaxOpen), 0); err != nil {
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
			return Config{}, fmt.Errorf("SMEM_ALLOWED_ORIGINS: %q is not an origin such as https://example.com (no credentials, path or query)", redactURL(o))
		}
		cfg.AllowedOrigins = append(cfg.AllowedOrigins, strings.ToLower(u.Scheme+"://"+u.Host))
	}
	if len(cfg.AllowedOrigins) == 0 && forAPI {
		return Config{}, fmt.Errorf("SMEM_ALLOWED_ORIGINS is required in prod (e.g. https://app.example.com)")
	}
	base := get("SMEM_PUBLIC_BASE_URL", "")
	if base == "" && cfg.Env != "prod" {
		base = "http://localhost:5173"
	}
	if base == "" && forAPI {
		return Config{}, fmt.Errorf("SMEM_PUBLIC_BASE_URL is required in prod (e.g. https://app.example.com)")
	}
	if u, err := url.Parse(base); base != "" && (err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil) {
		return Config{}, fmt.Errorf("SMEM_PUBLIC_BASE_URL=%q: want a URL such as https://app.example.com (no credentials, query or fragment)", redactURL(base))
	}
	cfg.PublicBaseURL = strings.TrimRight(base, "/")
	if cfg.TrustProxy, err = strconv.ParseBool(get("SMEM_TRUST_PROXY", "false")); err != nil {
		return Config{}, fmt.Errorf("SMEM_TRUST_PROXY=%q: want true or false", getenv("SMEM_TRUST_PROXY"))
	}
	raw = get("SMEM_HTTP_REQUEST_TIMEOUT", "30s")
	if cfg.RequestTimeout, err = time.ParseDuration(raw); err != nil || cfg.RequestTimeout < time.Second {
		return Config{}, fmt.Errorf("SMEM_HTTP_REQUEST_TIMEOUT=%q: want a duration of at least 1s such as 30s", raw)
	}
	if cfg.MaxHashes, err = getInt(get, "SMEM_AUTH_MAX_CONCURRENT_HASHES", "4", 1); err != nil {
		return Config{}, err
	}
	var n int
	if n, err = getInt(get, "SMEM_AUTH_ARGON_MEMORY_KIB", "19456", minArgonMemoryKiB); err != nil || n > 1<<20 {
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
	if cfg.LoginFailsPerAcct, err = getInt(get, "SMEM_RATE_LOGIN_FAILS_PER_ACCOUNT", "50", 1); err != nil {
		return Config{}, err
	}
	if cfg.GoogleClientID = getenv("SMEM_GOOGLE_CLIENT_ID"); cfg.GoogleClientID != "" {
		if err := cfg.loadGoogle(getenv, get); err != nil {
			return Config{}, err
		}
	}
	n, err = getInt(get, "SMEM_MEDIA_MAX_BYTES", "10485760", 1)
	if err != nil || n > 100<<20 {
		return Config{}, fmt.Errorf("SMEM_MEDIA_MAX_BYTES=%q: want an integer between 1 and 104857600", get("SMEM_MEDIA_MAX_BYTES", "10485760"))
	}
	cfg.MediaMaxBytes = int64(n)
	if cfg.MediaMaxConcurrent, err = getInt(get, "SMEM_MEDIA_MAX_CONCURRENT", "2", 1); err != nil {
		return Config{}, err
	}
	if cfg.PublicUploadsPerIP, err = getInt(get, "SMEM_PUBLIC_UPLOAD_CONCURRENT_PER_IP", "8", 1); err != nil {
		return Config{}, err
	}
	if cfg.PublicUploadConns, err = getInt(get, "SMEM_PUBLIC_UPLOAD_MAX_CONNS", "48", 1); err != nil {
		return Config{}, err
	}
	if cfg.UploadTmpDir = getenv("SMEM_UPLOAD_TMP_DIR"); cfg.UploadTmpDir != "" {
		if fi, err := os.Stat(cfg.UploadTmpDir); err != nil || !fi.IsDir() {
			return Config{}, fmt.Errorf("SMEM_UPLOAD_TMP_DIR=%q: want an existing directory", cfg.UploadTmpDir)
		}
	}
	// Below 64 MiB the collector would run almost continuously, so such a value is a typo, not a limit.
	if cfg.MemoryLimitMiB, err = getInt(get, "SMEM_MEMORY_LIMIT_MIB", "0", 0); err != nil || cfg.MemoryLimitMiB > 1<<20 || (cfg.MemoryLimitMiB != 0 && cfg.MemoryLimitMiB < 64) {
		return Config{}, fmt.Errorf("SMEM_MEMORY_LIMIT_MIB=%q: want 0 (unset) or an integer between 64 and 1048576", get("SMEM_MEMORY_LIMIT_MIB", "0"))
	}
	cfg.S3Endpoint = getenv("SMEM_S3_ENDPOINT")
	if cfg.S3Endpoint != "" {
		if u, err := url.Parse(cfg.S3Endpoint); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
			return Config{}, fmt.Errorf("SMEM_S3_ENDPOINT=%q: want a URL such as http://127.0.0.1:9000 (no credentials)", redactURL(cfg.S3Endpoint))
		}
	}
	cfg.S3Region = get("SMEM_S3_REGION", "us-east-1")
	cfg.S3Bucket = getenv("SMEM_S3_BUCKET")
	if cfg.S3Bucket == "" {
		if cfg.Env == "prod" {
			return Config{}, fmt.Errorf("SMEM_S3_BUCKET is required in prod")
		}
		cfg.S3Bucket = "smemories-dev" // the bucket `make up` creates
	}
	cfg.S3AccessKey, cfg.S3SecretKey = getenv("SMEM_S3_ACCESS_KEY"), getenv("SMEM_S3_SECRET_KEY")
	if (cfg.S3AccessKey == "") != (cfg.S3SecretKey == "") { // one half would silently fall back to the credential chain or fail at the first upload
		return Config{}, fmt.Errorf("SMEM_S3_ACCESS_KEY and SMEM_S3_SECRET_KEY go together: set both (static keys, MinIO) or neither (task role)")
	}
	if cfg.S3PathStyle, err = strconv.ParseBool(get("SMEM_S3_PATH_STYLE", "false")); err != nil {
		return Config{}, fmt.Errorf("SMEM_S3_PATH_STYLE=%q: want true or false", getenv("SMEM_S3_PATH_STYLE"))
	}
	if err := cfg.loadProd(getenv); err != nil {
		return Config{}, err
	}
	if !forAPI {
		return cfg, nil
	}
	if err := cfg.loadRedis(getenv, get); err != nil {
		return Config{}, err
	}
	if err := cfg.loadShutdown(get); err != nil {
		return Config{}, err
	}
	if err := cfg.loadOTP(getenv); err != nil {
		return Config{}, err
	}
	if cfg.Env == "prod" && cfg.MemoryLimitMiB == 0 {
		return Config{}, fmt.Errorf("SMEM_MEMORY_LIMIT_MIB is required in prod (about 80%% of the task memory, e.g. 1638 for 2 GiB; see docs/container.md)")
	}
	if err := cfg.loadMail(getenv); err != nil {
		return Config{}, err
	}
	if err := cfg.loadMediaLink(getenv); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// loadMediaLink reads the secret of the public photo links (T-092, D-32). Unset: prod refuses to start; dev and test get a random
// per-process secret (nothing guessable, so it is not a dev shortcut) and main logs one WARN. The messages name the variable and
// never repeat its value.
func (cfg *Config) loadMediaLink(getenv func(string) string) error {
	raw := strings.TrimSpace(getenv("SMEM_MEDIA_LINK_SECRET")) // a secret pasted with a trailing newline still works
	if raw == "" {
		if cfg.Env == "prod" {
			return fmt.Errorf("SMEM_MEDIA_LINK_SECRET is required when SMEM_ENV is prod (at least %d random bytes, base64, e.g. `openssl rand -base64 48`)", minMediaLinkBytes)
		}
		cfg.MediaLinkSecret = make([]byte, minMediaLinkBytes)
		if _, err := rand.Read(cfg.MediaLinkSecret); err != nil {
			return fmt.Errorf("SMEM_MEDIA_LINK_SECRET: no random source for the per-process secret")
		}
		cfg.MediaLinkSecretRandom = true
		return nil
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		if key, err = base64.RawStdEncoding.DecodeString(raw); err != nil {
			return fmt.Errorf("SMEM_MEDIA_LINK_SECRET is not valid base64 (e.g. `openssl rand -base64 48`)")
		}
	}
	if len(key) < minMediaLinkBytes {
		return fmt.Errorf("SMEM_MEDIA_LINK_SECRET is too short: %d bytes after decoding, want at least %d", len(key), minMediaLinkBytes)
	}
	cfg.MediaLinkSecret = key
	return nil
}

// loadMail reads the mail settings (T-021). The provider name itself is validated by mailer.New; prod accepts only ses.
func (cfg *Config) loadMail(getenv func(string) string) error {
	cfg.MailProvider = getenv("SMEM_MAIL_PROVIDER")
	cfg.MailFrom = getenv("SMEM_MAIL_FROM")
	cfg.SESConfigurationSet = getenv("SMEM_SES_CONFIGURATION_SET")
	if cfg.Env == "prod" && cfg.MailProvider != "ses" {
		return fmt.Errorf("SMEM_MAIL_PROVIDER=%q: prod needs ses (the log mailer prints codes)", cfg.MailProvider)
	}
	if cfg.MailProvider == "ses" || cfg.Env == "prod" {
		if err := checkMailFrom(cfg.MailFrom, cfg.Env == "prod"); err != nil {
			return err
		}
	}
	return nil
}

// exampleDomains are never a real sender: documentation names and reserved TLDs.
var exampleDomains = []string{"example.com", "example.org", "example.net", ".example", ".test", ".invalid", ".localhost", "localhost"}

// checkMailFrom wants "Name <user@domain>" or "user@domain"; in prod the domain must not be an example or reserved one.
// The message never repeats the value beyond the variable name.
func checkMailFrom(from string, prod bool) error {
	a, err := mail.ParseAddress(from)
	if from == "" || err != nil {
		return fmt.Errorf("SMEM_MAIL_FROM is required with SMEM_MAIL_PROVIDER=ses and must be an address such as \"SMemories <no-reply@mail.example.com>\"")
	}
	domain := strings.ToLower(a.Address[strings.LastIndex(a.Address, "@")+1:])
	if prod {
		for _, d := range exampleDomains {
			if domain == strings.TrimPrefix(d, ".") || strings.HasSuffix(domain, "."+strings.TrimPrefix(d, ".")) {
				return fmt.Errorf("SMEM_MAIL_FROM: prod needs an address on your verified sending domain, not an example or reserved one")
			}
		}
	}
	return nil
}

// loadShutdown reads the graceful-shutdown timings (T-022).
func (cfg *Config) loadShutdown(get func(key, def string) string) error {
	rawT := get("SMEM_SHUTDOWN_TIMEOUT", "25s")
	t, err := time.ParseDuration(rawT)
	if err != nil || t < time.Second || t > 120*time.Second {
		return fmt.Errorf("SMEM_SHUTDOWN_TIMEOUT=%q: want a duration between 1s and 120s such as 25s", rawT)
	}
	defDelay := "0s"
	if cfg.Env == "prod" {
		defDelay = "5s"
	}
	rawD := get("SMEM_SHUTDOWN_DELAY", defDelay)
	d, err := time.ParseDuration(rawD)
	if err != nil || d < 0 || d >= t {
		return fmt.Errorf("SMEM_SHUTDOWN_DELAY=%q: want a duration of at least 0 and below SMEM_SHUTDOWN_TIMEOUT (%s)", rawD, rawT)
	}
	cfg.ShutdownTimeout, cfg.ShutdownDelay = t, d
	return nil
}

// loadOTP reads the email-code settings (T-048).
func (cfg *Config) loadOTP(getenv func(string) string) error {
	if key := getenv("SMEM_OTP_KEY"); key != "" {
		if len(key) < minOTPKeyBytes {
			return fmt.Errorf("SMEM_OTP_KEY is too short: %d bytes, want at least %d", len(key), minOTPKeyBytes)
		}
		cfg.OTPKey = []byte(key)
	} else if cfg.Env == "dev" || cfg.Env == "test" {
		cfg.OTPKey = []byte(devOTPKey)
	} else {
		return fmt.Errorf("SMEM_OTP_KEY is required when SMEM_ENV is %s (at least %d random bytes, e.g. `openssl rand -base64 48`)", cfg.Env, minOTPKeyBytes)
	}
	// DEV-SHORTCUT(otp): the fixed code. The raw SMEM_ENV is read, not cfg.Env, so an empty SMEM_ENV (which defaults to dev) refuses too.
	if fixed := getenv("SMEM_DEV_FIXED_OTP"); fixed != "" { // DEV-SHORTCUT(otp)
		if env := getenv("SMEM_ENV"); env != "dev" && env != "test" { // DEV-SHORTCUT(otp)
			return fmt.Errorf("SMEM_DEV_FIXED_OTP is set but SMEM_ENV=%q: the fixed code is only allowed when SMEM_ENV is dev or test; unset it", env) // DEV-SHORTCUT(otp)
		} // DEV-SHORTCUT(otp)
		if len(fixed) != 6 || strings.Trim(fixed, "0123456789") != "" { // DEV-SHORTCUT(otp)
			return fmt.Errorf("SMEM_DEV_FIXED_OTP: want exactly 6 digits") // DEV-SHORTCUT(otp)
		} // DEV-SHORTCUT(otp)
		cfg.DevFixedOTP = fixed // DEV-SHORTCUT(otp)
	} // DEV-SHORTCUT(otp)
	return nil
}

// loadRedis reads the Redis settings (T-051). The URL is checked here so a typo fails at startup naming the variable.
func (cfg *Config) loadRedis(getenv func(string) string, get func(key, def string) string) error {
	cfg.RedisURL, cfg.RedisPassword = getenv("SMEM_REDIS_URL"), getenv("SMEM_REDIS_PASSWORD")
	if cfg.RedisURL == "" {
		return fmt.Errorf("SMEM_REDIS_URL is required (e.g. redis://127.0.0.1:6379/0)")
	}
	bad := fmt.Errorf("SMEM_REDIS_URL: want redis://[:password@]host:port/db or rediss://..., port 1-65535, db 0-15")
	u, err := url.Parse(cfg.RedisURL)
	if err != nil || (u.Scheme != "redis" && u.Scheme != "rediss") || u.Hostname() == "" || u.RawQuery != "" || u.Fragment != "" {
		return bad
	}
	if p := u.Port(); p != "" { // a port out of range would only fail at the first ping
		if n, err := strconv.Atoi(p); err != nil || n < 1 || n > 65535 {
			return bad
		}
	}
	if db := strings.TrimPrefix(u.Path, "/"); db != "" {
		if n, err := strconv.Atoi(db); err != nil || n < 0 || n > 15 {
			return bad
		}
	}
	if cfg.Env == "prod" && !cfg.RedisAllowInsecure {
		if u.Scheme != "rediss" {
			return fmt.Errorf("SMEM_REDIS_URL: prod needs a rediss:// (TLS) URL")
		}
		if _, hasPw := u.User.Password(); !hasPw && cfg.RedisPassword == "" {
			return fmt.Errorf("SMEM_REDIS_URL: prod needs credentials, in the URL (rediss://:token@host:6379/0) or in SMEM_REDIS_PASSWORD")
		}
	}
	for _, t := range []struct {
		key, def string
		dst      *time.Duration
	}{
		{"SMEM_REDIS_DIAL_TIMEOUT", "2s", &cfg.RedisDialTimeout},
		{"SMEM_REDIS_READ_TIMEOUT", "1s", &cfg.RedisReadTimeout},
		{"SMEM_REDIS_WRITE_TIMEOUT", "1s", &cfg.RedisWriteTimeout},
	} {
		raw := get(t.key, t.def)
		if *t.dst, err = time.ParseDuration(raw); err != nil || *t.dst <= 0 {
			return fmt.Errorf("%s=%q: want a positive duration such as %s", t.key, raw, t.def)
		}
	}
	cfg.RedisPoolSize, err = getInt(get, "SMEM_REDIS_POOL_SIZE", "10", 1)
	return err
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
	if len(key) < minOIDCKeyBytes {
		return fmt.Errorf("SMEM_OIDC_COOKIE_KEY is too short: %d bytes, want at least 32", len(key))
	}
	cfg.OIDCCookieKey = []byte(key)
	cfg.GoogleIssuer = get("SMEM_GOOGLE_ISSUER", "https://accounts.google.com")
	u, err := url.Parse(cfg.GoogleIssuer)
	if err != nil || u.Host == "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil || (u.Scheme != "https" && (u.Scheme != "http" || cfg.Env == "prod")) {
		return fmt.Errorf("SMEM_GOOGLE_ISSUER=%q: want an https URL such as https://accounts.google.com", redactURL(cfg.GoogleIssuer))
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
