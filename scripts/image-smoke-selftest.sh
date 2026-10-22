#!/usr/bin/env bash
# Self-test of the whole-run timeout of scripts/image-smoke.sh (T-091). A fake `docker` whose `run` hangs stands in for a stuck container;
# SMOKE_TIMEOUT=2 forces the timeout. Expects: exit 1 (not 143), a FAIL line that names the last step that passed, the run ends at once even
# through `| tail`, and no process of the fake is left behind. Needs no Docker. Usage: scripts/image-smoke-selftest.sh (or make image-smoke-selftest)
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
HANG="600.$$"   # a sleep length no other process uses, so pgrep finds only ours
cat >"$TMP/docker" <<EOF2
#!/bin/sh
case "\$1" in
  image) case "\$*" in *Config.User*) echo 65532:65532 ;; *) echo x ;; esac ;;
  run) exec sleep $HANG ;;
esac
EOF2
chmod +x "$TMP/docker"

SECONDS=0
rc=0
PATH="$TMP:$PATH" SMOKE_TIMEOUT=2 bash "$HERE/image-smoke.sh" fake:image >"$TMP/out" 2>&1 </dev/null || rc=$?
fail() { echo "image-smoke self-test FAIL: $*" >&2; cat "$TMP/out" >&2; pkill -f "sleep $HANG" 2>/dev/null || true; exit 1; }
[ "$rc" -eq 1 ] || fail "exit code $rc, want 1"
[ "$SECONDS" -lt 20 ] || fail "took ${SECONDS}s to stop a 2s timeout"
grep -q 'SMOKE FAIL: the whole run took longer than 2s (last step that passed: user 65532 and OCI labels)' "$TMP/out" || fail "no FAIL line naming the step"
if pgrep -f "sleep $HANG" >/dev/null; then fail "a child of the run is still alive"; fi
echo "image-smoke self-test passed: timeout gives exit 1, names the step, leaves no child (${SECONDS}s)"
