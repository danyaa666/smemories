#!/usr/bin/env bash
# T-131: every class* operation of api/openapi.yaml must have a row in docs/class-privacy-review.md section 6
# (`| `class/get` | ... | `file_test.go` | `TestName` |`), and the test file and test function(s) of the row must exist.
# Run from anywhere; CI runs it in the `go` job. Usage: scripts/check-class-matrix.sh
set -euo pipefail
cd "$(dirname "$0")/.."
doc=docs/class-privacy-review.md
dir=internal/class
fail=0
ops="$(sed -n 's|^  /api/\(class[a-z-]*/[a-z-]*\):.*|\1|p' api/openapi.yaml)"
[ -n "$ops" ] || { echo "check-class-matrix: no class* operation found in api/openapi.yaml" >&2; exit 2; }
for op in $ops; do
  row="$(grep -F "| \`$op\` |" "$doc" || true)"
  if [ -z "$row" ]; then echo "check-class-matrix: $op has no row in $doc" >&2; fail=1; continue; fi
  file="$(printf '%s\n' "$row" | head -n1 | awk -F'|' '{gsub(/[` ]/,"",$4); print $4}')"
  tests="$(printf '%s\n' "$row" | head -n1 | awk -F'|' '{print $5}' | grep -o 'Test[A-Za-z0-9_]*' || true)"
  if [ ! -f "$dir/$file" ]; then echo "check-class-matrix: $op: test file $dir/$file does not exist" >&2; fail=1; continue; fi
  [ -n "$tests" ] || { echo "check-class-matrix: $op: no test name in its row" >&2; fail=1; continue; }
  for t in $tests; do
    grep -q "^func $t(" "$dir/$file" || { echo "check-class-matrix: $op: $dir/$file has no func $t" >&2; fail=1; }
  done
done
[ "$fail" -eq 0 ] && echo "check-class-matrix: $(echo "$ops" | wc -w | tr -d ' ') class operations, all in $doc"
exit "$fail"
