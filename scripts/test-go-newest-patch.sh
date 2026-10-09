#!/usr/bin/env bash
# Tests the run script of .github/actions/go-newest-patch/action.yml the way GitHub runs it
# (bash --noprofile --norc -e -o pipefail), against fixtures served through file://. Needs curl and jq.
# Usage: scripts/test-go-newest-patch.sh [--live]   (--live also asks the real go.dev)
set -u
cd "$(dirname "$0")/.."
tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT
# The run script is the block after "run: |", unindented by 8 spaces.
sed -n '/^      run: |$/,$p' .github/actions/go-newest-patch/action.yml | tail -n +2 | sed 's/^        //' > "$tmp/orig.sh"
fail=0
# check NAME URL MINOR EXPECT   (EXPECT = go1.x.y, or "warn" for: warning, rc 0, no GOTOOLCHAIN written)
check() {
  local name="$1" url="$2" minor="$3" want="$4"
  sed "s#https://go.dev/dl/?mode=json#$url#" "$tmp/orig.sh" | sed 's/^go version$/true/' > "$tmp/s.sh"
  : > "$tmp/env"
  out="$(GITHUB_ENV="$tmp/env" MINOR="$minor" bash --noprofile --norc -e -o pipefail "$tmp/s.sh" 2>&1)"; rc=$?
  got="$(sed -n 's/^GOTOOLCHAIN=//p' "$tmp/env")"
  if [ "$want" = warn ]; then
    [ $rc -eq 0 ] && [ -z "$got" ] && grep -q '^::warning' <<<"$out" && { echo "ok   $name"; return; }
  else
    [ $rc -eq 0 ] && [ "$got" = "$want" ] && ! grep -q '^::warning' <<<"$out" && { echo "ok   $name"; return; }
  fi
  echo "FAIL $name (rc=$rc got='$got')"; echo "$out" | sed 's/^/     /'; fail=1
}
good='[{"version":"go1.27.2","stable":true},{"version":"go1.26.10","stable":true},{"version":"go1.26.9","stable":true},{"version":"go1.26.11rc1","stable":false},{"version":"go1.26rc1","stable":true}]'
printf '%s' "$good" > "$tmp/good.json"
printf '<html>503</html>' > "$tmp/html.json"
printf 'not json' > "$tmp/text.json"
printf '{"a":1}' > "$tmp/obj.json"
printf '[]' > "$tmp/empty.json"
: > "$tmp/blank.json"
check "newest patch of the minor" "file://$tmp/good.json" 1.26 go1.26.10
check "html outage page"          "file://$tmp/html.json" 1.26 warn
check "non-JSON text"             "file://$tmp/text.json" 1.26 warn
check "JSON object"               "file://$tmp/obj.json"  1.26 warn
check "empty array"               "file://$tmp/empty.json" 1.26 warn
check "empty body"                "file://$tmp/blank.json" 1.26 warn
check "minor mismatch"            "file://$tmp/good.json" 1.99 warn
check "missing file (404-like)"   "file://$tmp/none.json" 1.26 warn
check "bad host"                  "https://go.invalid/x"  1.26 warn
check "invalid MINOR"             "file://$tmp/good.json" '1.26; echo pwned' warn
if [ "${1:-}" = --live ]; then
  live="$(curl -fsS 'https://go.dev/dl/?mode=json' | jq -r '[.[]|select(.stable)|.version|select(test("^go1\\.26\\.[0-9]+$"))]|first')"
  check "real go.dev" "https://go.dev/dl/?mode=json" 1.26 "$live"
fi
exit $fail
