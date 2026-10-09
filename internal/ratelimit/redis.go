package ratelimit

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"strconv"
	"sync/atomic"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/danyaa666/smemories/internal/apperr"
	"github.com/danyaa666/smemories/internal/redis"
)

// ErrUnavailable is what a fail-closed limiter returns (errors.Is) when Redis cannot be reached; the
// API answers 503 limiter_unavailable. The cause stays in the chain.
var ErrUnavailable = apperr.New(apperr.Unavailable, "rate limiter unavailable")

// faultLogEvery is the minimum gap between two ERROR logs about limiter failures.
const faultLogEvery = time.Minute

// takeScript trims the window, then counts and adds the hit in one step. The time comes from the
// Redis server (TIME), so several API tasks agree whatever their own clocks say; ARGV[4] overrides it
// with a fake clock for tests.
// KEYS[1] sorted set (score = hit time in ms); ARGV: window ms, limit, unique member, clock ms or "".
// Returns {1, 0} when the hit was recorded, {0, retry-after ms} when the key is at its limit.
const takeScript = `
local now
if ARGV[4] ~= '' then
  now = tonumber(ARGV[4])
else
  local t = redis.call('TIME')
  now = t[1] * 1000 + math.floor(t[2] / 1000)
end
local window = tonumber(ARGV[1])
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now - window)
if redis.call('ZCARD', KEYS[1]) >= tonumber(ARGV[2]) then
  local oldest = redis.call('ZRANGE', KEYS[1], 0, 0, 'WITHSCORES')
  if not oldest[2] then return {0, window} end
  return {0, tonumber(oldest[2]) + window - now}
end
redis.call('ZADD', KEYS[1], now, ARGV[3])
redis.call('PEXPIRE', KEYS[1], window)
return {1, 0}`

// refundScript removes the most recent hit (the empty set disappears with it).
const refundScript = `redis.call('ZPOPMAX', KEYS[1]) return 1`

var (
	takeScr   = goredis.NewScript(takeScript)
	refundScr = goredis.NewScript(refundScript)
)

// Factory builds the limiters of one process. A Redis failure is handled here, once, so call sites only
// choose a policy: Open (let the request through) or Closed (refuse it with ErrUnavailable).
type Factory struct {
	rc  *redis.Client // nil: Memory limiters (unit tests only)
	log *slog.Logger
	now func() time.Time // nil: the Redis clock; set only by tests that move time

	faults  atomic.Int64 // failures since the last log line
	lastLog atomic.Int64 // unix nanos of the last log line
}

// NewFactory returns the factory for rc. now must be nil in production (the Redis TIME is used); tests pass a
// fake clock. log may be nil.
func NewFactory(rc *redis.Client, log *slog.Logger, now func() time.Time) *Factory {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Factory{rc: rc, log: log, now: now}
}

// NewMemoryFactory returns a factory of Memory limiters, for unit tests that need no Redis.
func NewMemoryFactory(now func() time.Time) *Factory {
	return &Factory{log: slog.New(slog.DiscardHandler), now: now}
}

// Open returns a limiter that fails open: when Redis is unavailable the request is allowed and an ERROR is
// logged (at most one per minute, with the number of failures since the last one).
func (f *Factory) Open(name string, limit int, window time.Duration) Limiter {
	return &guard{in: f.base(name, limit, window), f: f, name: name}
}

// Closed returns a limiter that fails closed: when Redis is unavailable Take returns an error that matches
// ErrUnavailable. Use it only where the limit protects against guessing.
func (f *Factory) Closed(name string, limit int, window time.Duration) Limiter {
	return &guard{in: f.base(name, limit, window), f: f, name: name, closed: true}
}

func (f *Factory) base(name string, limit int, window time.Duration) Limiter {
	if f.rc == nil {
		return NewMemory(limit, window, f.now)
	}
	return &Redis{rc: f.rc, name: name, limit: limit, window: window, now: f.now}
}

// fault counts a limiter failure and logs it, at most once per faultLogEvery.
func (f *Factory) fault(name string, closed bool, err error) {
	f.faults.Add(1)
	now := time.Now().UnixNano()
	last := f.lastLog.Load()
	if now-last < int64(faultLogEvery) || !f.lastLog.CompareAndSwap(last, now) {
		return
	}
	policy := "open"
	if closed {
		policy = "closed"
	}
	f.log.Error("ratelimit: limiter failed", "limiter", name, "policy", policy, "failures", f.faults.Swap(0), "error", err)
}

// guard applies the fail policy to a limiter.
type guard struct {
	in     Limiter
	f      *Factory
	name   string
	closed bool
}

type unavailable struct{ cause error }

func (e unavailable) Error() string   { return "ratelimit: " + e.cause.Error() }
func (e unavailable) Unwrap() []error { return []error{ErrUnavailable, e.cause} }

func (g *guard) Take(ctx context.Context, key string) (bool, time.Duration, error) {
	ok, retry, err := g.in.Take(ctx, key)
	if err == nil {
		return ok, retry, nil
	}
	if errors.Is(err, context.Canceled) { // the client left; nothing failed, and the caller sees its own cancellation next
		if !g.closed {
			return true, 0, nil
		}
		return false, 0, err
	}
	g.f.fault(g.name, g.closed, err)
	if g.closed {
		return false, 0, unavailable{err}
	}
	return true, 0, nil
}

// Refund never fails: a refund that is lost only costs the caller one counted attempt. It runs even when the
// request context is already cancelled.
func (g *guard) Refund(ctx context.Context, key string) error {
	if err := g.in.Refund(context.WithoutCancel(ctx), key); err != nil {
		g.f.fault(g.name, g.closed, err)
	}
	return nil
}

// Redis is the shared Limiter: one sorted set per key, smem:<env>:rl:<name>:<key>.
type Redis struct {
	rc     *redis.Client
	name   string
	limit  int
	window time.Duration
	now    func() time.Time // nil: Redis TIME
}

// NewRedis returns a raw Redis limiter whose errors are not softened (see Factory for the fail policies).
// now is nil in production.
func NewRedis(rc *redis.Client, name string, limit int, window time.Duration, now func() time.Time) *Redis {
	return &Redis{rc: rc, name: name, limit: limit, window: window, now: now}
}

func (l *Redis) key(key string) string { return l.rc.Key("rl", l.name, key) }

// Take implements Limiter.
func (l *Redis) Take(ctx context.Context, key string) (bool, time.Duration, error) {
	clock := ""
	if l.now != nil {
		clock = strconv.FormatInt(l.now().UnixMilli(), 10)
	}
	var m [8]byte
	if _, err := rand.Read(m[:]); err != nil {
		return false, 0, apperr.Wrap(apperr.Internal, err, "ratelimit: random member")
	}
	member := hex.EncodeToString(m[:]) // unique, so two hits in one millisecond both count
	res, err := takeScr.Run(ctx, l.rc.Client(), []string{l.key(key)}, l.window.Milliseconds(), l.limit, member, clock).Int64Slice()
	if err != nil {
		return false, 0, redis.Classify(err)
	}
	if len(res) != 2 {
		return false, 0, apperr.New(apperr.Internal, "ratelimit: unexpected script reply")
	}
	if res[0] == 1 {
		return true, 0, nil
	}
	return false, time.Duration(res[1]) * time.Millisecond, nil
}

// Refund implements Limiter.
func (l *Redis) Refund(ctx context.Context, key string) error {
	return redis.Classify(refundScr.Run(ctx, l.rc.Client(), []string{l.key(key)}).Err())
}
