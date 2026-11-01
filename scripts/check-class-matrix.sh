#!/usr/bin/env bash
# T-131, extended by T-153: the privacy boundary of a class admin is a table (docs/class-privacy-review.md section 6) whose rows name the test that
# proves each role check (`| `class/get` | ... | `file_test.go` | `TestName` |`). This script fails when
#  - a class* operation, or a book-layout operation (a class admin gets 404 there, AC3 of T-153), of api/openapi.yaml has no row,
#  - the row of the admin's one read path into a student's files, `media/get-content` (AC6 of T-153: a design image only while the layout
#    references it), is missing, or
#  - any row of the table names a test file or a test function that does not exist.
# Test files are in internal/class/. Run from anywhere; CI runs it in the `go` job. Usage: scripts/check-class-matrix.sh
set -euo pipefail
cd "$(dirname "$0")/.."
doc=docs/class-privacy-review.md
dir=internal/class
fail=0
ops="$(sed -En 's#^  /api/((class[a-z-]*|book-layout)/[a-z-]+):.*#\1#p' api/openapi.yaml)"
[ -n "$ops" ] || { echo "check-class-matrix: no class* operation found in api/openapi.yaml" >&2; exit 2; }
echo "$ops" | grep -q '^book-layout/' || { echo "check-class-matrix: no book-layout operation found in api/openapi.yaml" >&2; exit 2; }
# every table row that starts with an operation in backticks: `group/action` | exposes | `file` | `Test`, `Test`
rows="$(grep -E '^\| `[a-z-]+/[a-z-]+` \|' "$doc" || true)"
for op in $ops media/get-content; do
  printf '%s\n' "$rows" | grep -qF "| \`$op\` |" || { echo "check-class-matrix: $op has no row in $doc" >&2; fail=1; }
done
while IFS= read -r row; do
  [ -n "$row" ] || continue
  op="$(printf '%s\n' "$row" | awk -F'|' '{gsub(/[` ]/,"",$2); print $2}')"
  file="$(printf '%s\n' "$row" | awk -F'|' '{gsub(/[` ]/,"",$4); print $4}')"
  tests="$(printf '%s\n' "$row" | awk -F'|' '{print $5}' | grep -o 'Test[A-Za-z0-9_]*' || true)"
  if [ ! -f "$dir/$file" ]; then echo "check-class-matrix: $op: test file $dir/$file does not exist" >&2; fail=1; continue; fi
  [ -n "$tests" ] || { echo "check-class-matrix: $op: no test name in its row" >&2; fail=1; continue; }
  for t in $tests; do
    grep -q "^func $t(" "$dir/$file" || { echo "check-class-matrix: $op: $dir/$file has no func $t" >&2; fail=1; }
  done
done <<< "$rows"
[ "$fail" -eq 0 ] && echo "check-class-matrix: $(echo "$ops" | wc -w | tr -d ' ') class and book-layout operations, all in $doc"
exit "$fail"
