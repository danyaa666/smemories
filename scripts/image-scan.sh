#!/usr/bin/env bash
# Vulnerability scan and SBOM of the production image (T-089, docs/container.md). Usage: scripts/image-scan.sh [IMAGE]   (default smemories:local)
#        scripts/image-scan.sh --self-test      proves the gate fails on a deliberately old image (alpine 3.12)
#
# Runs Trivy from a pinned image against `docker save` of IMAGE (no docker socket is mounted, no credential reaches Trivy). Fails (exit 1)
# on any HIGH or CRITICAL vulnerability that has a fix, writes a CycloneDX SBOM to $SBOM_DIR (default build/sbom/sbom.cdx.json). The
# vulnerability database is cached in $TRIVY_CACHE_DIR (default ~/.cache/smem-trivy; CI caches it per day).
set -euo pipefail

TRIVY_IMG='ghcr.io/aquasecurity/trivy:0.76.0@sha256:2416d1f74045b57274df1a340eee6b510cacd73c66a6ccb8da092b117eb7d222'
OLD_IMG='alpine:3.12@sha256:c75ac27b49326926b803b9ed43bf088bc220d22556de1bc5f72d742c91398f69' # EOL 2022: must always be rejected
CACHE="${TRIVY_CACHE_DIR:-$HOME/.cache/smem-trivy}"
SBOM_DIR="${SBOM_DIR:-build/sbom}"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
mkdir -p "$CACHE" "$SBOM_DIR"
SBOM_DIR="$(cd "$SBOM_DIR" && pwd)"

trivy() { docker run --rm --user "$(id -u):$(id -g)" -e TRIVY_CACHE_DIR=/cache -e TRIVY_NO_PROGRESS=true -v "$CACHE:/cache" -v "$TMP:/in:ro" "$@"; }

# scan IMAGE_REF [SBOM_FILE_OR_EMPTY]
scan() {
  docker save "$1" -o "$TMP/image.tar"
  local code=0
  trivy "$TRIVY_IMG" image --input /in/image.tar --scanners vuln --severity HIGH,CRITICAL --ignore-unfixed --exit-code 1 || code=$?
  if [ -n "${2:-}" ]; then # the database is fresh now, the SBOM needs no scan
    docker run --rm --user "$(id -u):$(id -g)" -e TRIVY_CACHE_DIR=/cache -e TRIVY_NO_PROGRESS=true -v "$CACHE:/cache" -v "$TMP:/in:ro" -v "$SBOM_DIR:/out" \
      "$TRIVY_IMG" image --input /in/image.tar --scanners vuln --skip-db-update --format cyclonedx --output "/out/$2" >/dev/null
    echo "SBOM: $SBOM_DIR/$2"
  fi
  return "$code"
}

if [ "${1:-}" = "--self-test" ]; then
  docker pull -q "$OLD_IMG" >/dev/null
  if scan "$OLD_IMG" >"$TMP/old.log" 2>&1; then cat "$TMP/old.log"; echo "SCAN SELF-TEST FAIL: the old base image passed" >&2; exit 1; fi
  grep -E 'Total: [0-9]+' "$TMP/old.log" | head -3
  echo "ok - the scan rejects a deliberately old base image"
  exit 0
fi

IMAGE="${1:-smemories:local}"
scan "$IMAGE" sbom.cdx.json
echo "ok - no fixable HIGH or CRITICAL vulnerability in $IMAGE"
