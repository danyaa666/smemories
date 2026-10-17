#!/usr/bin/env bash
# Smoke test of the production image (T-022, docs/container.md). Usage: scripts/image-smoke.sh [IMAGE]   (default smemories:local)
#
# Starts a private MySQL, Valkey and MinIO on a private docker network (nothing is published on the host except the API
# port on 127.0.0.1, and nothing of the developer's compose stack is touched), then checks the image:
#   1. static: non-root user, labels, no shell, only the three binaries, size under 60 MB
#   2. smemories-migrate up/status and smemories-media-backfill --dry-run from the same image
#   3. the API refuses to start in prod with the dev OTP shortcut set and with SMEM_OTP_KEY missing
#   4. a read-only root filesystem without the writable spool mount fails at start-up with a clear message
#   5. the API with --read-only --user 65532 --cap-drop ALL: healthz, readyz, register, yearbook create, photo upload
#   6. SIGTERM during a slow request: /readyz answers 503 while draining, the request still completes, exit code 0
#
# The API runs with SMEM_ENV=test, not prod: the only mailer is the log mailer, which refuses prod, so the API cannot serve in
# prod before T-021 adds a real one. The prod refusals (3) and the one-off tasks run with SMEM_ENV=prod. All credentials are
# generated here, used once and never printed.
set -euo pipefail

IMAGE="${1:-smemories:local}"
RUN="smem-smoke-$(od -An -N4 -tx1 /dev/urandom | tr -d ' \n')"
MYSQL_IMG='mysql:8.4@sha256:6ea90827b1100f8f2ae306a539f86d2c264a26ed435a2a9f75551dd5c3aeb242'
VALKEY_IMG='valkey/valkey:8@sha256:640c5e62cea04b6d6f2084232651d0cc70362d31f4f805e7be94dbed6855e8f2'
MINIO_IMG='bitnamilegacy/minio:2025.4.22-debian-12-r2'
MAX_MB=60
BUCKET=smemories-smoke
ORIGIN=https://smoke.example.com

TMP="$(mktemp -d)"
rand() { od -An -N24 -tx1 /dev/urandom | tr -d ' \n'; }
DB_PW="$(rand)"; ROOT_PW="$(rand)"; S3_USER="smokeminio"; S3_PW="$(rand)"; OTP_KEY="$(rand)$(rand)"

cleanup() {
  local ids
  ids="$(docker ps -aq --filter "label=smem-smoke=$RUN" 2>/dev/null || true)"
  [ -z "$ids" ] || docker rm -f $ids >/dev/null 2>&1 || true
  docker network rm "$RUN" >/dev/null 2>&1 || true
  rm -rf "$TMP"
}
trap cleanup EXIT

fail() { echo "SMOKE FAIL: $*" >&2; docker logs --tail 25 "$RUN-api" >&2 2>/dev/null || true; exit 1; }
ok() { echo "ok - $*"; }
run() { docker run --label "smem-smoke=$RUN" "$@"; }

# --- 1. static checks of the image -------------------------------------------------------------------------------
[ "$(docker image inspect -f '{{.Config.User}}' "$IMAGE")" = "65532:65532" ] || fail "image user is not 65532:65532"
for l in source revision version licenses; do
  [ -n "$(docker image inspect -f "{{index .Config.Labels \"org.opencontainers.image.$l\"}}" "$IMAGE")" ] || fail "missing OCI label $l"
done
ok "user 65532 and OCI labels"
if docker run --rm --entrypoint /bin/sh "$IMAGE" -c true >/dev/null 2>&1; then fail "the image has a shell"; fi
ok "no shell"
docker create --name "$RUN-fs" --label "smem-smoke=$RUN" "$IMAGE" >/dev/null
docker export "$RUN-fs" >"$TMP/fs.tar"
SIZE=$(wc -c <"$TMP/fs.tar")
tar -tf "$TMP/fs.tar" | grep -v '/$' >"$TMP/files.txt"
bins="$(grep '^usr/local/bin/' "$TMP/files.txt" | sort | tr '\n' ' ')"
[ "$bins" = "usr/local/bin/smemories-api usr/local/bin/smemories-media-backfill usr/local/bin/smemories-migrate " ] || fail "unexpected binaries: $bins"
if grep -Ei '(^|/)(\.git|\.env[^/]*|\.team|node_modules|web|docs)(/|$)|\.(go|sql|pem|key|md|test)$|_test' "$TMP/files.txt" >"$TMP/bad.txt"; then
  fail "files that must not be in the image: $(head -5 "$TMP/bad.txt" | tr '\n' ' ')"
fi
ok "only the three binaries plus the distroless base"
MB=$((SIZE / 1024 / 1024))
echo "image filesystem: ${MB} MiB (limit ${MAX_MB})"
[ -z "${GITHUB_STEP_SUMMARY:-}" ] || echo "Image filesystem size: **${MB} MiB** (limit ${MAX_MB} MiB)" >>"$GITHUB_STEP_SUMMARY"
[ "$MB" -lt "$MAX_MB" ] || fail "image is ${MB} MiB, limit ${MAX_MB}"
rm -f "$TMP/fs.tar"

# --- services ---------------------------------------------------------------------------------------------------
docker network create "$RUN" >/dev/null
run -d --name "$RUN-mysql" --network "$RUN" -e MYSQL_ROOT_PASSWORD="$ROOT_PW" -e MYSQL_DATABASE=smemories \
  -e MYSQL_USER=smemories -e MYSQL_PASSWORD="$DB_PW" "$MYSQL_IMG" \
  --character-set-server=utf8mb4 --collation-server=utf8mb4_0900_ai_ci \
  --sql-mode=ONLY_FULL_GROUP_BY,STRICT_ALL_TABLES,NO_ZERO_IN_DATE,NO_ZERO_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION \
  --default-time-zone=+00:00 >/dev/null
run -d --name "$RUN-redis" --network "$RUN" "$VALKEY_IMG" valkey-server --save "" --appendonly no --protected-mode no >/dev/null
run -d --name "$RUN-minio" --network "$RUN" -e MINIO_ROOT_USER="$S3_USER" -e MINIO_ROOT_PASSWORD="$S3_PW" "$MINIO_IMG" >/dev/null
for i in $(seq 1 60); do
  docker exec "$RUN-mysql" mysqladmin ping -h 127.0.0.1 -uroot -p"$ROOT_PW" --silent >/dev/null 2>&1 &&
    docker exec "$RUN-redis" valkey-cli ping 2>/dev/null | grep -qx PONG &&
    docker exec "$RUN-minio" curl -fsS http://127.0.0.1:9000/minio/health/ready >/dev/null 2>&1 && break
  [ "$i" -lt 60 ] || fail "services did not become ready"
  sleep 2
done
docker exec "$RUN-minio" sh -c "mc alias set local http://127.0.0.1:9000 '$S3_USER' '$S3_PW' >/dev/null && mc mb --ignore-existing local/$BUCKET >/dev/null"
ok "private MySQL, Valkey and MinIO are up"

DSN="smemories:$DB_PW@tcp($RUN-mysql:3306)/smemories"
umask 077
# Settings of the one-off tasks (database and object store only) and of the API; plain env, like the Fargate task definition.
cat >"$TMP/task.env" <<EOF
SMEM_DB_DSN=$DSN
SMEM_S3_ENDPOINT=http://$RUN-minio:9000
SMEM_S3_BUCKET=$BUCKET
SMEM_S3_ACCESS_KEY=$S3_USER
SMEM_S3_SECRET_KEY=$S3_PW
SMEM_S3_PATH_STYLE=true
EOF
cat >"$TMP/api.env" <<EOF
$(cat "$TMP/task.env")
SMEM_REDIS_URL=redis://$RUN-redis:6379/0
SMEM_OTP_KEY=$OTP_KEY
SMEM_ALLOWED_ORIGINS=$ORIGIN
SMEM_PUBLIC_BASE_URL=$ORIGIN
SMEM_MEMORY_LIMIT_MIB=1536
SMEM_UPLOAD_TMP_DIR=/spool
SMEM_SHUTDOWN_DELAY=3s
SMEM_LOG_LEVEL=info
EOF
# The hardening every task runs with (docs/container.md).
HARDEN=(--read-only --user 65532 --cap-drop ALL --security-opt no-new-privileges --network "$RUN")

# --- 2. one-off tasks from the same image ---------------------------------------------------------------------------
run --rm "${HARDEN[@]}" --env-file "$TMP/task.env" -e SMEM_ENV=prod "$IMAGE" smemories-migrate up >"$TMP/migrate.log" 2>&1 || { cat "$TMP/migrate.log" >&2; fail "smemories-migrate up failed"; }
run --rm "${HARDEN[@]}" --env-file "$TMP/task.env" -e SMEM_ENV=prod "$IMAGE" smemories-migrate up >/dev/null 2>&1 || fail "a second migrate up is not idempotent"
run --rm "${HARDEN[@]}" --env-file "$TMP/task.env" -e SMEM_ENV=prod "$IMAGE" smemories-migrate status 2>&1 | grep -q ' applied ' || fail "migrate status shows nothing applied"
ok "smemories-migrate up (twice) and status"
run --rm "${HARDEN[@]}" --env-file "$TMP/task.env" -e SMEM_ENV=prod "$IMAGE" smemories-media-backfill --dry-run >/dev/null 2>&1 || fail "smemories-media-backfill --dry-run failed"
ok "smemories-media-backfill --dry-run"

# --- 3. the API refuses an unsafe prod configuration ----------------------------------------------------------------
PROD_API=(-e SMEM_ENV=prod -e SMEM_REDIS_URL=redis://127.0.0.1:6379/0 -e SMEM_ALLOWED_ORIGINS=$ORIGIN -e SMEM_PUBLIC_BASE_URL=$ORIGIN)
refuse() { # refuse <variable that must be named> <docker run args...>
  local var="$1" out code=0
  shift
  out="$(run --rm "${HARDEN[@]}" --env-file "$TMP/task.env" "${PROD_API[@]}" "$@" "$IMAGE" 2>&1)" || code=$?
  [ "$code" -ne 0 ] || fail "the API started although $var is wrong"
  echo "$out" | grep -q "$var" || fail "the refusal does not name $var: $out"
  if echo "$out" | grep -q "$OTP_KEY\|$DB_PW\|$S3_PW\|123123"; then fail "the refusal echoes a secret"; fi
}
refuse SMEM_DEV_FIXED_OTP -e SMEM_OTP_KEY="$OTP_KEY" -e SMEM_DEV_FIXED_OTP=123123
refuse SMEM_OTP_KEY
refuse SMEM_OTP_KEY -e SMEM_OTP_KEY=too-short
ok "prod start refuses the dev shortcut and a missing or short SMEM_OTP_KEY"

# --- 4. read-only root without the writable spool ----------------------------------------------------------------
code=0
out="$(run --rm "${HARDEN[@]}" --env-file "$TMP/api.env" -e SMEM_UPLOAD_TMP_DIR= -e SMEM_ENV=test "$IMAGE" 2>&1)" || code=$?
[ "$code" -ne 0 ] || fail "the API started on a read-only root without a writable spool"
echo "$out" | grep -q 'SMEM_UPLOAD_TMP_DIR' || fail "no clear spool message: $out"
ok "read-only root without the spool mount: clear start-up error"

# --- 5. the API as it runs in production -------------------------------------------------------------------------
run -d --name "$RUN-api" "${HARDEN[@]}" --memory 2g --tmpfs /spool:rw,noexec,nosuid,size=64m,uid=65532,mode=0700 \
  -p 127.0.0.1::8080 --env-file "$TMP/api.env" -e SMEM_ENV=test "$IMAGE" >/dev/null
ADDR="$(docker port "$RUN-api" 8080/tcp | head -1)"
BASE="http://$ADDR"
for i in $(seq 1 30); do
  [ "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/healthz" || true)" = 200 ] && break
  [ "$i" -lt 30 ] || fail "/healthz never answered 200"
  sleep 1
done
[ "$(curl -s -o /dev/null -w '%{http_code}' "$BASE/readyz")" = 200 ] || fail "/readyz is not 200"
ok "/healthz and /readyz answer 200 (read-only, uid 65532, no capabilities)"

json() { python3 -I -c 'import json,sys; d=json.load(sys.stdin)
for k in sys.argv[1].split("."): d=d[k]
print(d)' "$1"; }
EMAIL="smoke-$(od -An -N4 -tx1 /dev/urandom | tr -d ' \n')@example.com"
PW="$(rand)"
curl -fsS -D "$TMP/h.txt" -o /dev/null -X POST "$BASE/api/auth/register" -H "Origin: $ORIGIN" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PW\",\"display_name\":\"Smoke\"}" || fail "register failed"
curl -fsS -D "$TMP/h.txt" -o /dev/null -X POST "$BASE/api/auth/login" -H "Origin: $ORIGIN" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PW\"}" || fail "login failed"
COOKIE="$(tr -d '\r' <"$TMP/h.txt" | sed -n 's/^[Ss]et-[Cc]ookie: \(smem_session=[^;]*\).*/\1/p' | head -1)"
[ -n "$COOKIE" ] || fail "login set no session cookie"
BOOK="$(curl -fsS -X POST "$BASE/api/yearbook/create" -H "Origin: $ORIGIN" -H "Cookie: $COOKIE" -H 'Content-Type: application/json' \
  -d '{"title":"Smoke 12A","language":"en"}' | json data.id)" || fail "yearbook create failed"
[ -n "$BOOK" ] || fail "yearbook create returned no id"
ok "register, login and yearbook create through the real API"

python3 -I - >"$TMP/photo.png" <<'PY'
import struct, sys, zlib
w = h = 64
raw = b"".join(b"\x00" + bytes(v for x in range(w) for v in (x * 4 % 256, y * 4 % 256, 128)) for y in range(h))
def chunk(t, d): return struct.pack(">I", len(d)) + t + d + struct.pack(">I", zlib.crc32(t + d))
sys.stdout.buffer.write(b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", w, h, 8, 2, 0, 0, 0)) + chunk(b"IDAT", zlib.compress(raw)) + chunk(b"IEND", b""))
PY
curl -fsS -X POST "$BASE/api/media/create" -H "Origin: $ORIGIN" -H "Cookie: $COOKIE" -F "yearbook_id=$BOOK" -F "file=@$TMP/photo.png;type=image/png" -o "$TMP/media.json" || { cat "$TMP/media.json" >&2 || true; fail "photo upload failed"; }
ok "photo upload (image processing and the object store work on a read-only root)"

# --- 6. graceful shutdown ----------------------------------------------------------------------------------------
# A request that takes ~6 s to arrive (3000 bytes at 500 B/s) is in flight when SIGTERM comes.
python3 -I -c 'import json; print(json.dumps({"email": "slow@example.com", "password": "x" * 3000}))' >"$TMP/slow.json"
curl -s -o /dev/null -w '%{http_code}' --limit-rate 500 -m 40 -X POST "$BASE/api/auth/login" -H "Origin: $ORIGIN" -H 'Content-Type: application/json' \
  -H 'Expect:' --data-binary @"$TMP/slow.json" >"$TMP/slow.code" 2>/dev/null &
SLOW=$!
sleep 1.5
docker kill --signal=TERM "$RUN-api" >/dev/null
drain503=no
for _ in $(seq 1 10); do
  if [ "$(curl -s -o /dev/null -w '%{http_code}' -m 2 "$BASE/readyz" || true)" = 503 ]; then drain503=yes; break; fi
  sleep 0.2
done
[ "$drain503" = yes ] || fail "/readyz did not answer 503 after SIGTERM"
ok "/readyz answers 503 while draining"
wait "$SLOW" || true
slow_code="$(cat "$TMP/slow.code")"
case "$slow_code" in 2??|4??) ok "the request in flight at SIGTERM completed (HTTP $slow_code)" ;; *) fail "the request in flight at SIGTERM did not complete (got '$slow_code')" ;; esac
exit_code="$(docker wait "$RUN-api")"
[ "$exit_code" = 0 ] || fail "the API exited with $exit_code after SIGTERM"
docker logs "$RUN-api" 2>&1 | grep -q 'shutdown complete' || fail "no 'shutdown complete' log line"
ok "exit code 0 after SIGTERM"
echo "image smoke test passed"
