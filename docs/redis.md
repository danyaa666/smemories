# Redis (Valkey)

All time-limited data lives in a Redis-protocol store (decision D-23): login sessions (T-052), the 6-digit email codes
with their attempt counters (T-048) and the rate limiters (T-053). MySQL keeps everything durable. Locally and in CI the
server is **Valkey 8** (BSD licence), pinned by digest in `docker-compose.yml` and `.github/workflows/ci.yml`; the code
uses only the Redis protocol (client `github.com/redis/go-redis/v9`), so Redis or ElastiCache work too.

T-051 added the plumbing; the first data stored is the email codes of T-048 (`docs/auth-otp.md`).

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

`GET /readyz` answers 503 `not_ready` when MySQL **or** Redis does not answer within 1 s (the cause is logged, never put in
the body); `GET /healthz` does not look at either.

## Keys

Every key starts with `smem:<SMEM_ENV>:` (`Client.Key(parts...)` builds it), so several environments or checkouts can share
one server in an emergency without colliding. Parts must come from hashes or numeric ids, never raw user input.

| Prefix | Owner | Content |
|---|---|---|
| `smem:<env>:sess:` | T-052 | login sessions, with native expiry |
| `smem:<env>:otp:<purpose>:<user id>` | T-048 | one hash per live email code (`verify` or `reset`): `h` HMAC of the code, `a` wrong attempts, `x` expiry (ms) |
| `smem:<env>:rl:` | T-053 | rate-limiter windows |

(The prefixes of the later tasks are planned; each task documents the final names here.)

## Failure policy (D-23)

`redis.Classify` (also applied by `Ping` and `LoadScript`) wraps network failures, timeouts and an exhausted pool in
`redis.ErrUnavailable` (test with `errors.Is`); every other error, such as a server reply error, is returned unchanged.
Callers decide: sessions and OTP attempts fail closed (503) when Redis is down; the other limiters fail open with an ERROR log.

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
