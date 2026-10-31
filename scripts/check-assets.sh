#!/usr/bin/env bash
# E09 T-151: SVG safety and registry rules of the sticker, shape and frame pack (docs/assets.md). CI runs it in the `web` job.
# Usage: scripts/check-assets.sh [assets dir]   (default web/src/book/assets)
set -euo pipefail
cd "$(dirname "$0")/.."
exec node web/scripts/check-assets.mjs "$@"
