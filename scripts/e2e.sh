#!/usr/bin/env bash
# End-to-end run of the M1 journey (T-020, docs/e2e.md). Usage: scripts/e2e.sh   (or `make e2e`)
#
# Needs MySQL, Valkey and MinIO already running (`make up`, or the services of the CI job) and the SMEM_DB_DSN, SMEM_REDIS_URL and
# SMEM_S3_* settings in the environment (`make` loads .env). It builds the API and the web app, starts them on ports of their own
# (the developer's API on :8080 and dev server on :5173 are not touched), runs tools/e2e/journey.spec.mjs and stops what it started.
#
# DEV-SHORTCUT(otp): the API runs with SMEM_ENV=test and SMEM_DEV_FIXED_OTP, the one place besides local development where the
# registered fixed email code is switched on (docs/dev-shortcuts.md). The code is only ever handed to this API and to the test.
set -euo pipefail

cd "$(dirname "$0")/.."
ROOT="$PWD"
API_PORT="${E2E_API_PORT:-18080}"
WEB_PORT="${E2E_WEB_PORT:-14173}"
OUT="${E2E_OUT:-$ROOT/tools/e2e/out}"
ORIGIN="http://127.0.0.1:$WEB_PORT"

for v in SMEM_DB_DSN SMEM_REDIS_URL SMEM_S3_BUCKET SMEM_S3_ACCESS_KEY SMEM_S3_SECRET_KEY; do
  [ -n "${!v:-}" ] || { echo "e2e: $v is not set (start the stack with 'make up' and run through 'make e2e')" >&2; exit 2; }
done
node tools/e2e/guard.mjs # refuses a non-local E2E_BASE_URL or a database that is not smem_e2e* / smem_test* before anything is touched (T-087)
for p in "$API_PORT" "$WEB_PORT"; do
  if (: </dev/tcp/127.0.0.1/"$p") 2>/dev/null; then echo "e2e: port $p is in use; set E2E_API_PORT / E2E_WEB_PORT" >&2; exit 2; fi
done

BIN="$(mktemp -d)"
mkdir -p "$OUT"
rm -rf "${OUT:?}"/*
PIDS=()
cleanup() {
  for p in ${PIDS[@]+"${PIDS[@]}"}; do kill "$p" 2>/dev/null || true; done # only what this script started
  for p in ${PIDS[@]+"${PIDS[@]}"}; do wait "$p" 2>/dev/null || true; done
  rm -rf "$BIN"
}
trap cleanup EXIT

go build -o "$BIN/api" ./cmd/smemories-api
go build -o "$BIN/migrate" ./cmd/smemories-migrate
npm --prefix web run build >/dev/null

SMEM_ENV=test "$BIN/migrate" up >"$OUT/migrate.log" 2>&1 || { cat "$OUT/migrate.log" >&2; exit 1; }

# The API: a test environment of its own (Redis keys smem:test:*), cookies and CSRF as in production, the registered dev shortcut on.
export SMEM_DEV_FIXED_OTP="${SMEM_DEV_FIXED_OTP:-123123}" # DEV-SHORTCUT(otp)
SMEM_ENV=test SMEM_HTTP_ADDR="127.0.0.1:$API_PORT" SMEM_ALLOWED_ORIGINS="$ORIGIN" SMEM_PUBLIC_BASE_URL="$ORIGIN" \
  SMEM_METRICS_ADDR="" SMEM_UPLOAD_TMP_DIR="$BIN" "$BIN/api" >"$OUT/api.log" 2>&1 &
PIDS+=("$!")
for i in $(seq 1 60); do
  curl -fsS -o /dev/null "http://127.0.0.1:$API_PORT/healthz" 2>/dev/null && break
  [ "$i" -lt 60 ] || { echo "e2e: the API did not start" >&2; tail -n 20 "$OUT/api.log" >&2; exit 1; }
  sleep 0.5
done

# The built web app; `vite preview` forwards /api to the API like the dev server does (web/vite.config.ts).
(cd web && SMEM_API_PROXY="http://127.0.0.1:$API_PORT" exec node node_modules/vite/bin/vite.js preview --host 127.0.0.1 --port "$WEB_PORT" --strictPort) >"$OUT/web.log" 2>&1 &
PIDS+=("$!")
for i in $(seq 1 60); do
  curl -fsS -o /dev/null "$ORIGIN/" 2>/dev/null && break
  [ "$i" -lt 60 ] || { echo "e2e: the web server did not start" >&2; tail -n 20 "$OUT/web.log" >&2; exit 1; }
  sleep 0.5
done

cd tools/e2e
status=0
E2E_BASE_URL="$ORIGIN" E2E_API_URL="http://127.0.0.1:$API_PORT" E2E_OUT="$OUT" npx playwright test || status=$?
# A failed run keeps a trace that holds the link and photo tokens and the session cookie: scrub it before anything uploads it (T-115).
./scrub-trace.sh "$OUT/results"
exit "$status"
