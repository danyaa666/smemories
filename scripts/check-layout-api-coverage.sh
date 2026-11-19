#!/usr/bin/env bash
# T-183 (stage 2 exit check, requirement 4): every book-layout/* route of api/openapi.yaml has
#   (a) a request in postman/customisation-editor.postman_collection.json,
#   (b) a row in the route table of internal/booklayout/abuse_integration_test.go (anonymous 401, one byte over the body cap 413, the per-user limit 429; T-179),
#   (c) a row in the class matrix of docs/class-privacy-review.md (the admin and other roles get 404; scripts/check-class-matrix.sh checks that the named test exists).
# It prints the table and fails (exit 1) when a cell is empty. Usage: scripts/check-layout-api-coverage.sh
set -euo pipefail
cd "$(dirname "$0")/.."
postman=postman/customisation-editor.postman_collection.json
abuse=internal/booklayout/abuse_integration_test.go
matrix=docs/class-privacy-review.md
ops="$(sed -En 's#^  /api/book-layout/([a-z-]+):.*#\1#p' api/openapi.yaml)"
[ -n "$ops" ] || { echo "check-layout-api-coverage: no book-layout operation in api/openapi.yaml" >&2; exit 2; }
fail=0
printf '%-16s %-14s %-16s %s\n' route postman abuse-test class-matrix
for op in $ops; do
  n="$(grep -o "api/book-layout/$op\b" "$postman" | wc -l | tr -d ' ')"
  if [ "$op" = get ]; then a="$(grep -c '{"get", "GET"' "$abuse" || true)"; else a="$(grep -c "{\"$op\", \"POST\"" "$abuse" || true)"; fi
  m="$(grep -cF "| \`book-layout/$op\` |" "$matrix" || true)"
  printf '%-16s %-14s %-16s %s\n' "$op" "$n requests" "$([ "$a" -gt 0 ] && echo yes || echo MISSING)" "$([ "$m" -gt 0 ] && echo yes || echo MISSING)"
  { [ "$n" -gt 0 ] && [ "$a" -gt 0 ] && [ "$m" -gt 0 ]; } || fail=1
done
echo "$ops" | wc -w | xargs printf 'check-layout-api-coverage: %s book-layout routes\n'
exit "$fail"
