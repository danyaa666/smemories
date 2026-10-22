#!/usr/bin/env bash
# Redacts the bearer secrets of a run from the Playwright traces in a results folder (T-115, docs/e2e.md): photo-link tokens
# (`...get-content?token=<t>`), note-link tokens (`/c/<t>`, `"token":"<t>"`, the query-string entry) and the session cookie. Playwright records every
# request, response body and DOM snapshot, so the secrets cannot be kept out at the source; the trace is unpacked, scrubbed and
# packed again, and the run's own log never needs them. Usage: scrub-trace.sh <results folder>   (a missing folder is fine)
set -euo pipefail
shopt -s nullglob
for zip in "${1:?usage: scrub-trace.sh <results folder>}"/*/trace.zip; do
  dir="$(mktemp -d)"
  unzip -q "$zip" -d "$dir"
  grep -rlaE 'token|smem_session|/c/' "$dir" | while IFS= read -r f; do
    perl -pi -e 's/(token=)[A-Za-z0-9_.%-]+/$1REDACTED/g; s|(/c/)[A-Za-z0-9_-]{16,}|$1REDACTED|g; s/(\\*"token\\*":\\*")[^"\\]+/$1REDACTED/g; s/(\\*"name\\*":\\*"token\\*",\\*"value\\*":\\*")[^"\\]+/$1REDACTED/g; s/(__Host-smem_session(=|\\*",\\*"value\\*":\\*"))[^;"\\ ]+/$1REDACTED/g' "$f"
  done || true
  rm "$zip"
  (cd "$dir" && zip -qr -X "$zip" .)
  rm -rf "$dir"
  echo "scrubbed $zip"
done
