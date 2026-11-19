#!/usr/bin/env bash
# T-186: run tools/print-check in an ubuntu-24.04 container (Docker), so the Linux CI result can be reproduced on a Mac or Windows PC in minutes.
#   scripts/print-check-linux.sh                       the whole suite
#   scripts/print-check-linux.sh --test-name-pattern 'note01'   extra arguments go to `node --test`
# The checkout is copied into the container (no node_modules or .git from the host), so nothing on the host is touched.
set -euo pipefail
# NODE_OPTIONS below: inside Docker "localhost" lists ::1 and 127.0.0.1, vite takes one and node fetch the other, so "vite did not start"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && { pwd -W 2>/dev/null || pwd; })" # pwd -W: the Windows path that docker.exe understands under Git Bash
export MSYS_NO_PATHCONV=1 # Git Bash on Windows must not rewrite the container paths below
docker build -q -t smem-print-check-linux -f "$ROOT/tools/print-check/Dockerfile.linux" "$ROOT/tools/print-check" >/dev/null
docker run --rm --ipc=host -e NODE_OPTIONS=--dns-result-order=ipv4first -v "$ROOT:/src:ro" -v smem-npm-cache:/root/.npm smem-print-check-linux bash -c '
  set -euo pipefail
  mkdir /work && cd /src && tar c --exclude=node_modules --exclude=.git --exclude=.team --exclude=dist . | tar x -C /work
  (cd /work/web && npm ci --no-audit --no-fund --loglevel=error)
  cd /work/tools/print-check && npm ci --no-audit --no-fund --loglevel=error
  start=$SECONDS
  npm test -- "$@" || rc=$?
  echo "print-check: $((SECONDS - start)) s"
  exit ${rc:-0}
' _ "$@"
