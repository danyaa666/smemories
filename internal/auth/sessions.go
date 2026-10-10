package auth

import (
	"context"
	"encoding/hex"
	"errors"
	"log/slog"
	"strconv"
	"sync/atomic"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/danyaa666/smemories/internal/redis"
)

// sessionLogInterval is how often a failing session store is logged (not once per request).
const sessionLogInterval = 30 * time.Second

// SessionStoreError says Redis, which holds the login sessions, failed or refused the command. The
// handler answers 503 session_store_unavailable: sessions have no copy elsewhere, so the request
// fails closed (never anonymous, never a fake session). Err is the cause.
type SessionStoreError struct{ Err error }

func (e *SessionStoreError) Error() string { return "auth: session store: " + e.Err.Error() }
func (e *SessionStoreError) Unwrap() error { return e.Err }

// Redis layout (D-23, docs/redis.md), <h> = hex sha256 of the cookie value, the raw token never reaches Redis:
//
//	smem:<env>:sess:<h>     hash {user_id, created_at, last_seen_at (unix ms), user_agent}; expires with the session
//	smem:<env>:usess:<uid>  set of that user's <h>; its expiry is pushed to the latest session expiry
//
// The scripts also touch the keys named by ARGV prefixes (sess keys of the set members), so they need a
// single Redis node (Valkey / ElastiCache without cluster mode), as the rest of the code does.
//
// createScript stores the session, drops the user's logically expired siblings from the index (the sweep the
// old table got at every login) and registers the new one.
// KEYS: sess, usess. ARGV: h, ttl ms, user id, now ms, user agent, sess key prefix.
const createScript = `
redis.call('HSET', KEYS[1], 'user_id', ARGV[3], 'created_at', ARGV[4], 'last_seen_at', ARGV[4], 'user_agent', ARGV[5])
redis.call('PEXPIRE', KEYS[1], ARGV[2])
for _, m in ipairs(redis.call('SMEMBERS', KEYS[2])) do
  local ls = redis.call('HGET', ARGV[6] .. m, 'last_seen_at')
  if not ls or tonumber(ls) + tonumber(ARGV[2]) <= tonumber(ARGV[4]) then
    redis.call('DEL', ARGV[6] .. m)
    redis.call('SREM', KEYS[2], m)
  end
end
redis.call('SADD', KEYS[2], ARGV[1])
if redis.call('PTTL', KEYS[2]) < tonumber(ARGV[2]) then redis.call('PEXPIRE', KEYS[2], ARGV[2]) end
return 1`

// extendScript slides a session forward, but only while it still exists: a logout or delete-all that won the
// race is never undone. Returns 0 when the session is gone.
// KEYS: sess, usess. ARGV: h, ttl ms, now ms.
const extendScript = `
if redis.call('EXISTS', KEYS[1]) == 0 then return 0 end
redis.call('HSET', KEYS[1], 'last_seen_at', ARGV[3])
redis.call('PEXPIRE', KEYS[1], ARGV[2])
redis.call('SADD', KEYS[2], ARGV[1])
if redis.call('PTTL', KEYS[2]) < tonumber(ARGV[2]) then redis.call('PEXPIRE', KEYS[2], ARGV[2]) end
return 1`

// deleteScript removes one session and its index entry. KEYS: sess, usess. ARGV: h.
const deleteScript = `
redis.call('DEL', KEYS[1])
redis.call('SREM', KEYS[2], ARGV[1])
return 1`

// deleteAllScript removes every session of a user and the index, atomically; index entries that outlived their
// session are harmless. KEYS: usess. ARGV: sess key prefix.
const deleteAllScript = `
for _, m in ipairs(redis.call('SMEMBERS', KEYS[1])) do redis.call('DEL', ARGV[1] .. m) end
redis.call('DEL', KEYS[1])
return 1`

// Sessions keeps the login sessions in Redis. Every error it returns is a *SessionStoreError (or
// errNotFound / the caller's cancelled context).
type Sessions struct {
	rc      *redis.Client
	logger  *slog.Logger
	create  *goredis.Script
	extend  *goredis.Script
	del     *goredis.Script
	delAll  *goredis.Script
	lastLog atomic.Int64 // unix ns of the last failure log line
}

// NewSessions loads the scripts into Redis.
func NewSessions(ctx context.Context, rc *redis.Client, logger *slog.Logger) (*Sessions, error) {
	s := &Sessions{rc: rc, logger: logger}
	for _, p := range []struct {
		dst **goredis.Script
		src string
	}{{&s.create, createScript}, {&s.extend, extendScript}, {&s.del, deleteScript}, {&s.delAll, deleteAllScript}} {
		sc, err := rc.LoadScript(ctx, p.src)
		if err != nil {
			return nil, err
		}
		*p.dst = sc
	}
	return s, nil
}

func (s *Sessions) sessKey(h string) string   { return s.rc.Key("sess", h) }
func (s *Sessions) userKey(uid uint64) string { return s.rc.Key("usess", strconv.FormatUint(uid, 10)) }
func (s *Sessions) sessPrefix() string        { return s.rc.Key("sess", "") }
func (s *Sessions) run(ctx context.Context, sc *goredis.Script, keys []string, args ...any) error {
	return s.fail(sc.Run(ctx, s.rc.Client(), keys, args...).Err())
}

// fail wraps a Redis failure (logged at most once per interval); nil and a cancelled request pass through.
func (s *Sessions) fail(err error) error {
	if err == nil || errors.Is(err, context.Canceled) {
		return err
	}
	now := time.Now().UnixNano()
	if last := s.lastLog.Load(); now-last >= int64(sessionLogInterval) && s.lastLog.CompareAndSwap(last, now) {
		s.logger.Error("auth: session store failing, requests that need a session answer 503", "error", redis.Classify(err))
	}
	return &SessionStoreError{Err: redis.Classify(err)}
}

func tokenKey(tokenHash []byte) string { return hex.EncodeToString(tokenHash) }

// Create stores a new session that lives until sess.expires.
func (s *Sessions) Create(ctx context.Context, sess session) error {
	ttl := sess.expires.Sub(sess.now).Milliseconds()
	h := tokenKey(sess.tokenHash)
	return s.run(ctx, s.create, []string{s.sessKey(h), s.userKey(sess.userID)},
		h, ttl, sess.userID, sess.now.UnixMilli(), cleanUserAgent(sess.userAgent), s.sessPrefix())
}

// Lookup returns the owner and the last-seen time of a session; errNotFound when it is unknown or expired
// (Redis drops it at its expiry).
func (s *Sessions) Lookup(ctx context.Context, tokenHash []byte) (uid uint64, lastSeen time.Time, err error) {
	m, err := s.rc.Client().HGetAll(ctx, s.sessKey(tokenKey(tokenHash))).Result()
	if err != nil {
		return 0, time.Time{}, s.fail(err)
	}
	uid, uerr := strconv.ParseUint(m["user_id"], 10, 64)
	ms, merr := strconv.ParseInt(m["last_seen_at"], 10, 64)
	if uerr != nil || merr != nil || uid == 0 { // missing, or a hash this code did not write
		return 0, time.Time{}, errNotFound
	}
	return uid, time.UnixMilli(ms).UTC(), nil
}

// Extend slides the session to now+ttl. It reports false when the session no longer exists.
func (s *Sessions) Extend(ctx context.Context, tokenHash []byte, uid uint64, now time.Time, ttl time.Duration) (bool, error) {
	h := tokenKey(tokenHash)
	n, err := s.extend.Run(ctx, s.rc.Client(), []string{s.sessKey(h), s.userKey(uid)}, h, ttl.Milliseconds(), now.UnixMilli()).Int()
	if err != nil {
		return false, s.fail(err)
	}
	return n == 1, nil
}

// Delete removes one session; an unknown one is not an error.
func (s *Sessions) Delete(ctx context.Context, tokenHash []byte) error {
	h := tokenKey(tokenHash)
	v, err := s.rc.Client().HGet(ctx, s.sessKey(h), "user_id").Result()
	if errors.Is(err, goredis.Nil) {
		return nil
	}
	if err != nil {
		return s.fail(err)
	}
	uid, err := strconv.ParseUint(v, 10, 64)
	if err != nil {
		uid = 0 // not ours; still delete the key
	}
	return s.run(ctx, s.del, []string{s.sessKey(h), s.userKey(uid)}, h)
}

// DeleteAll removes every session of the user at once (password reset, Google pre-hijacking defence).
func (s *Sessions) DeleteAll(ctx context.Context, uid uint64) error {
	return s.run(ctx, s.delAll, []string{s.userKey(uid)}, s.sessPrefix())
}
