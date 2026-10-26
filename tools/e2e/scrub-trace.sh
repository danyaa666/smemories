#!/usr/bin/env bash
# Redacts the bearer secrets of a run from the Playwright traces in a results folder (T-115, docs/e2e.md): photo-link tokens
# (`...get-content?token=<t>`), note-link tokens (`/c/<t>`, `"token":"<t>"`, the query-string entry), class-invite links (`/join/<t>`) and the session cookie. Playwright records every
# request, response body and DOM snapshot, so the secrets cannot be kept out at the source; the trace is unpacked, scrubbed and
# packed again, and the run's own log never needs them. Usage: scrub-trace.sh <results folder>   (a missing folder is fine)
#
# Fails closed: a trace.zip that cannot be scrubbed and checked for any reason is deleted (with the reason on stderr), so nothing unscrubbed is
# ever uploaded. The script still exits 0 then; the caller keeps the run's own exit code.
set -euo pipefail

if [ "${1:-}" = "--one" ]; then # scrub one trace.zip; any failure here (set -e) makes the caller delete it
  zip="$(cd "$(dirname "${2:?}")" && pwd)/$(basename "$2")"
  dir="$(mktemp -d)"
  trap 'rm -rf "$dir"' EXIT
  unzip -q "$zip" -d "$dir"
  files="$(grep -rlaE 'token|smem_session|/c/|/join/' "$dir" || [ "$?" -eq 1 ])" # exit 1: no file holds a secret, fine
  while IFS= read -r f; do
    [ -n "$f" ] || continue
    perl -pi -e 's/(token=)[A-Za-z0-9_.%-]+/$1REDACTED/g; s#(/(?:c|join)/)[A-Za-z0-9_-]{16,}#$1REDACTED#g; s/(\\*"token\\*":\\*")[^"\\]+/$1REDACTED/g; s/(\\*"name\\*":\\*"token\\*",\\*"value\\*":\\*")[^"\\]+/$1REDACTED/g; s/(__Host-smem_session(=|\\*",\\*"value\\*":\\*"))[^;"\\ ]+/$1REDACTED/g' "$f"
  done <<<"$files"
  rm "$zip"
  # -D: no directory entries, Playwright's own reader (`trace open`, `show-trace`) fails with EEXIST on them
  (cd "$dir" && zip -qr -X -D "$zip" .)
  unzip -tq "$zip" >/dev/null
  if unzip -Z1 "$zip" | grep -q '/$'; then echo "the repacked zip has directory entries" >&2; exit 1; fi
  exit 0
fi

shopt -s nullglob
for zip in "${1:?usage: scrub-trace.sh <results folder>}"/*/trace.zip; do
  if err="$("$0" --one "$zip" 2>&1)"; then
    echo "scrubbed $zip"
  else
    rm -f "$zip"
    echo "scrub-trace: DELETED $zip, it could not be scrubbed: ${err:-unknown error}" >&2
  fi
done
