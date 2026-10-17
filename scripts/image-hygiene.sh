#!/usr/bin/env bash
# Secret hygiene of the production image (T-089, docs/container.md). Usage: scripts/image-hygiene.sh [IMAGE]   (default smemories:local)
#        scripts/image-hygiene.sh --self-test [IMAGE]   proves the check fails on an image with a planted .env
#
# Fails (exit 1) if the final filesystem or `docker history` of IMAGE holds a .env file, a *.pem or *.key file, an AWS access key id
# (AKIA...) or the dev OTP value 123123 (SMEM_DEV_FIXED_OTP) anywhere, in file contents or in a layer command. It never prints a match
# itself, only the file name or the layer command position.
set -euo pipefail

TMP="$(mktemp -d)"
CID=""
cleanup() { [ -z "$CID" ] || docker rm -f "$CID" >/dev/null 2>&1 || true; rm -rf "$TMP"; }
trap cleanup EXIT

check() { # check IMAGE -> prints the findings and returns 1 when there is any
  local img="$1" bad=0
  rm -rf "${TMP:?}/fs" && mkdir -p "$TMP/fs"
  CID="$(docker create "$img" true)"
  docker export "$CID" | tar -x -C "$TMP/fs" 2>/dev/null
  docker rm -f "$CID" >/dev/null && CID=""
  # File names.
  if find "$TMP/fs" \( -iname '.env' -o -iname '.env.*' -o -iname '*.env' -o -iname '*.pem' -o -iname '*.key' \) | sed "s|^$TMP/fs||" | grep .; then
    echo "hygiene: secret-looking file names in the image filesystem (above)" >&2; bad=1
  fi
  # File contents (binaries included: -a). -l prints the file name only, never the match.
  if grep -rlaE 'AKIA[0-9A-Z]{16}|123123' "$TMP/fs" | sed "s|^$TMP/fs||" | grep .; then
    echo "hygiene: an AWS access key id or the dev OTP value is inside the files above" >&2; bad=1
  fi
  # Layer commands and labels.
  docker history --no-trunc --format '{{.CreatedBy}}' "$img" >"$TMP/history.txt"
  docker image inspect -f '{{json .Config}}' "$img" >>"$TMP/history.txt"
  if grep -nE 'AKIA[0-9A-Z]{16}|123123|\.env|\.pem|SECRET|PASSWORD|TOKEN' "$TMP/history.txt" | cut -d: -f1 | sed 's/^/hygiene: docker history or config line /' | grep .; then
    bad=1
  fi
  return "$bad"
}

if [ "${1:-}" = "--self-test" ]; then
  IMAGE="${2:-smemories:local}"
  ctx="$(mktemp -d)"
  printf 'SMEM_OTP_KEY=planted-not-a-real-key\n' >"$ctx/.env"
  printf 'FROM %s\nCOPY .env /.env\n' "$IMAGE" >"$ctx/Dockerfile"
  tag="smem-hygiene-selftest:$$"
  docker build -q --provenance=false --sbom=false -t "$tag" "$ctx" >/dev/null
  rm -rf "$ctx"
  code=0
  check "$tag" >"$TMP/selftest.log" 2>&1 || code=$?
  docker rmi "$tag" >/dev/null 2>&1 || true
  [ "$code" -ne 0 ] || { echo "HYGIENE SELF-TEST FAIL: a planted .env was not detected" >&2; exit 1; }
  cat "$TMP/selftest.log"
  echo "ok - the hygiene check rejects an image with a planted .env"
  exit 0
fi

IMAGE="${1:-smemories:local}"
check "$IMAGE" || { echo "HYGIENE FAIL: $IMAGE" >&2; exit 1; }
echo "ok - no .env, key file, AWS key id or dev OTP value in the filesystem or layers of $IMAGE"
