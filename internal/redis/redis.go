// Package redis is a thin wrapper around go-redis for the Redis-protocol store (Valkey locally, ElastiCache in
// prod) that holds all time-limited data (D-23): sessions, email codes, rate limiters. No business logic here.
package redis

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/danyaa666/smemories/internal/config"
)

// ErrUnavailable marks a network failure, timeout or exhausted pool, so callers can apply their own policy
// (fail open for rate limiters, fail closed for codes and sessions). Test with errors.Is.
var ErrUnavailable = errors.New("redis unavailable")

// Client is a go-redis client plus the key prefix of this environment. Safe for concurrent use.
type Client struct {
	rdb    *goredis.Client
	prefix string
	tls    bool // rediss:// URL
}

// readyProbeTTL is how long the readiness probe key lives; the key only exists to prove writes are accepted.
const readyProbeTTL = 10 * time.Second

// UseLogger routes go-redis' own log lines (pool and dial problems, otherwise unstructured text on stderr) into
// logger at WARN. The setting is process-wide; call it once at startup.
func UseLogger(logger *slog.Logger) { goredis.SetLogger(slogPrinter{logger}) }

type slogPrinter struct{ l *slog.Logger }

func (p slogPrinter) Printf(ctx context.Context, format string, v ...interface{}) {
	p.l.WarnContext(ctx, "redis client: "+strings.TrimSpace(fmt.Sprintf(format, v...)))
}

// New builds a client from cfg. It does not connect: the first command (or Ping) does.
// Errors never contain the password.
func New(cfg config.Config) (*Client, error) {
	o, err := goredis.ParseURL(cfg.RedisURL)
	if err != nil {
		return nil, errors.New("SMEM_REDIS_URL: not a valid redis:// or rediss:// URL")
	}
	if o.Password == "" {
		o.Password = cfg.RedisPassword
	}
	if o.TLSConfig != nil {
		o.TLSConfig.MinVersion = tls.VersionTLS12
	}
	o.DialTimeout, o.ReadTimeout, o.WriteTimeout = cfg.RedisDialTimeout, cfg.RedisReadTimeout, cfg.RedisWriteTimeout
	o.PoolSize = cfg.RedisPoolSize
	// One attempt per command: with 1 s timeouts the default 3 retries would make a dead Redis cost seconds per request.
	o.MaxRetries = -1
	return &Client{rdb: goredis.NewClient(o), prefix: "smem:" + cfg.Env + ":", tls: o.TLSConfig != nil}, nil
}

// Client returns the underlying go-redis client. Wrap its errors with Classify.
func (c *Client) Client() *goredis.Client { return c.rdb }

// Addr is host:port, safe to log (no password).
func (c *Client) Addr() string { return c.rdb.Options().Addr }

// Close releases the connection pool.
func (c *Client) Close() error { return c.rdb.Close() }

// Ping checks the connection; failures are classified. With a rediss:// URL a failure also hints at TLS, because
// pointing it at a plain server ends in an opaque timeout.
func (c *Client) Ping(ctx context.Context) error {
	err := Classify(c.rdb.Ping(ctx).Err())
	if err != nil && c.tls {
		return fmt.Errorf("%w (rediss:// URL: is the server TLS-enabled?)", err)
	}
	return err
}

// Ready is Ping plus a write: a full Redis (maxmemory, noeviction) still answers reads but refuses writes, and then
// sessions and codes cannot be stored, so /readyz must say not ready.
func (c *Client) Ready(ctx context.Context) error {
	if err := c.Ping(ctx); err != nil {
		return err
	}
	return Classify(c.rdb.Set(ctx, c.Key("probe", "ready"), "1", readyProbeTTL).Err())
}

// Key builds a namespaced key: smem:<env>:<part1>:<part2>... Parts must come from hashes or numeric ids, never raw user input.
func (c *Client) Key(parts ...string) string { return c.prefix + strings.Join(parts, ":") }

// LoadScript registers a Lua script with the server and returns it for Run/EvalSha.
func (c *Client) LoadScript(ctx context.Context, src string) (*goredis.Script, error) {
	s := goredis.NewScript(src)
	if err := s.Load(ctx, c.rdb).Err(); err != nil {
		return nil, Classify(err)
	}
	return s, nil
}

// Classify wraps network and timeout errors in ErrUnavailable (the cause stays in the chain) and returns every
// other error, such as a Redis reply error, unchanged. A cancelled context is the caller's doing and stays as is.
func Classify(err error) error {
	var ne net.Error
	if err == nil || errors.Is(err, ErrUnavailable) {
		return err
	}
	if errors.As(err, &ne) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, goredis.ErrPoolTimeout) || errors.Is(err, goredis.ErrClosed) {
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	return err
}
