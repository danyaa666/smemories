# Redis (Valkey)

All time-limited data lives in a Redis-protocol store (decision D-23): login sessions (T-052), the 6-digit email codes
with their attempt counters (T-048) and the rate limiters (T-053). MySQL keeps everything durable. Locally and in CI the
server is **Valkey 8** (BSD licence), pinned by digest in `docker-compose.yml` and `.github/workflows/ci.yml`; the code
uses only the Redis protocol (client `github.com/redis/go-redis/v9`), so Redis or ElastiCache work too.

T-051 added the plumbing; the data stored so far: the email codes of T-048 (`docs/auth-otp.md`), the login sessions of T-052 and the rate limiters of T-053 (below).

## Configuration

| Variable | Default | Meaning |
|---|---|---|
| `SMEM_REDIS_URL` | none, **required in every environment** | `redis://[:password@]host:port/db` (db 0-15) or `rediss://` for TLS (TLS 1.2 or newer) |
| `SMEM_REDIS_PASSWORD` | empty | Used when the URL carries no password |
| `SMEM_REDIS_DIAL_TIMEOUT` | `2s` | Connect timeout |
| `SMEM_REDIS_READ_TIMEOUT` / `SMEM_REDIS_WRITE_TIMEOUT` | `1s` | Per-command timeouts |
| `SMEM_REDIS_POOL_SIZE` | `10` | Connections per process |
| `SMEM_TEST_REDIS_URL` | `redis://127.0.0.1:6379/0` | Server used by integration tests |

An invalid value stops the API with a message naming the variable; the password is never printed. The API pings Redis at
startup (5 s) and exits if it is unreachable, then logs `redis connected` with `host:port` only. Commands are not retried
(`MaxRetries` is off): with 1 s timeouts a dead Redis would otherwise cost seconds per request.

`GET /readyz` answers 503 `not_ready` when MySQL **or** Redis does not answer within 1 s, or when Redis refuses a write
(readiness SETs `smem:<env>:probe:ready` with a 10 s expiry, so a full Redis with `noeviction` is not ready); the cause is
logged, never put in the body. `GET /healthz` does not look at either. go-redis' own log lines go through slog
(`redis client: ...`, WARN) instead of raw stderr, and a failing `rediss://` connection names TLS in the startup error.

`smemories-migrate` does not read `SMEM_REDIS_URL` or `SMEM_OTP_KEY` (the one-off migration task has no Redis access); only
the API does.

## Keys

Every key starts with `smem:<SMEM_ENV>:` (`Client.Key(parts...)` builds it), so several environments or checkouts can share
one server in an emergency without colliding. Parts must come from hashes or numeric ids, never raw user input.

| Prefix | Owner | Content |
|---|---|---|
| `smem:<env>:sess:<h>` | T-052 | one hash per login session, `<h>` = hex sha256 of the cookie value (the raw token never reaches Redis): `user_id`, `created_at`, `last_seen_at` (unix ms), `user_agent`; `EXPIRE` = remaining lifetime (30 days, slid forward once half has passed) |
| `smem:<env>:usess:<user id>` | T-052 | set of that user's `<h>` (for "delete every session of the user"); its expiry is pushed to the latest session expiry; a login also removes the user's expired members |
| `smem:<env>:otp:<purpose>:<user id>` | T-048 | one hash per live email code (`verify` or `reset`): `h` HMAC of the code, `a` wrong attempts, `x` expiry (ms) |
| `smem:<env>:rl:<limiter>:<key>` | T-053 | one sorted set per limited key (see Rate limiters); the key expires with the window |


## Sessions (T-052)

`internal/auth/sessions.go`. One round trip per authenticated request (`HGETALL`), then one MySQL lookup of the user by id;
a Lua script runs only at login (create + sweep), when the session slides (at most every 15 days), at logout and for
delete-all. The slide script checks the session still exists first, so a logout or password reset that wins a race is never
undone. Delete-all (password reset, the Google pre-hijacking defence) runs inside the MySQL transaction **before** the commit:
if Redis fails, the password change or takeover is rolled back too and can be retried. The scripts read the `sess:<h>` keys named
by the index set, so Redis must be a single node (Valkey, or ElastiCache without cluster mode); cluster mode would need
hash tags. Nothing is migrated: migration `0012` drops the `sessions` table and everybody signs in again once.

Redis down (or refusing writes): every request that carries a cookie answers `503 session_store_unavailable` with
`Retry-After: 5` (never "anonymous", never a made-up session); login and register answer the same after the account rules
ran (register rolls the new account back); endpoints without a session keep working. The failure is logged once per 30 s.
Flushing Redis signs everybody out (401, no 500). Operating note: AOF keeps sessions across a Redis restart.

## Failure policy (D-23)

`redis.Classify` (also applied by `Ping` and `LoadScript`) wraps network failures, timeouts and an exhausted pool in
`redis.ErrUnavailable` (test with `errors.Is`); every other error, such as a server reply error, is returned unchanged.
Callers decide: sessions and OTP attempts fail closed (503) when Redis is down; the other limiters fail open with an ERROR log (see Rate limiters).

## Operations

- The compose service is `redis`, bound to `127.0.0.1:${REDIS_PORT:-6379}`, with `--appendonly yes --appendfsync everysec`
  on the named volume `redis-data` (a restart keeps sessions), no RDB snapshots, `maxmemory 256mb` and
  `maxmemory-policy noeviction` (a full Redis fails writes loudly and never drops a session silently).
  `--protected-mode no` is needed because the server has no password and connections through the published port look
  non-loopback inside the container; the port itself is loopback-only on the host.
- Inspect keys: `docker compose exec redis valkey-cli`, then `SCAN 0 MATCH 'smem:dev:*'` (avoid `KEYS` on a big dataset).
- Production: a private subnet, TLS and an auth token (T-022/T-023, ElastiCache).

## Tests

`redistest.New(t)` (`internal/redis/redistest`) returns a client whose key prefix is unique to the test
(`smem:test<random>:`) and deletes everything under it in `t.Cleanup`; it fails the test clearly when the server is
unreachable (run `make up`). Integration tests need the `integration` tag: `make up && make test-integration`.
Unit tests need no Redis.

## Rate limiters (T-053)

`internal/ratelimit`: a sliding window per key, stored as one sorted set `smem:<env>:rl:<limiter>:<key>` (score = hit time in ms).
One Lua script per `Take` removes the entries older than the window, counts, and adds the hit, atomically. It reads the time from the
Redis `TIME` command, so API tasks with skewed clocks agree, and `PEXPIRE`s the key to the window, so idle keys disappear.
`Refund` removes the most recent hit (`ZPOPMAX`). On a refusal `Take` returns how long until the oldest hit leaves the window, which
becomes `Retry-After`. `ratelimit.Memory` is the in-process version for unit tests only; `main.go` does not use it.

Keys hold only IPs, numeric or public ids, or a SHA-256 hash of the normalised email (never the address itself).

Fail policy when Redis is unreachable (`ratelimit.Factory`): **open** lets the request through and logs one ERROR per minute
(`ratelimit: limiter failed`, fields `limiter`, `policy`, `failures` = failures since the last line); **closed** answers
`503 limiter_unavailable` (`Retry-After: 5`) because the limit protects against guessing. A failed `Refund` is only logged.

| Limiter name | Limit | Window | Key | Fail |
|---|---|---|---|---|
| `register` | `SMEM_RATE_REGISTER_PER_HOUR` (5) | 1 h | IP | open |
| `login_pair` | `SMEM_RATE_LOGIN_FAILS_PER_EMAIL` (10) failures | 15 min | IP + hashed email | closed |
| `login_ip` | `SMEM_RATE_LOGIN_FAILS_PER_IP` (100) failures | 15 min | IP | closed |
| `forgot_ip` | 5 | 1 h | IP | open |
| `forgot_email` | 3 | 1 h | hashed email | open |
| `reset_tries` | 20 | 1 h | IP | closed |
| `verify_resend` | 3 | 1 h | user id | open |
| `verify_tries` | 20 | 1 h | user id | closed |
| `google_start` | 30 | 15 min | IP | open |
| `google_callback` | 30 | 15 min | IP | open |
| `public_misses` | 60 (only misses count) | 15 min | IP | open |
| `public_all` | 600 | 15 min | IP | open |
| `submit_ip_hour` | 100 | 1 h | IP | open |
| `submit_ip_day` | 300 | 24 h | IP | open |
| `submit_collection_hour` | 60 | 1 h | collection id | open |
| `media_upload` | 60 | 10 min | owner id | open |

The per-code wrong-guess counter and lockout of the email codes (5 per code) is part of the code hash in Redis
(`docs/auth-otp.md`) and fails closed as before.

Reset them (dev): `make reset-limits` deletes every `smem:<SMEM_ENV>:rl:*` key of the compose Redis, so repeated Newman runs
from one address do not hit 429 and the API does not need a restart. Production never does this.
