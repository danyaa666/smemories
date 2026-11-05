#!/bin/sh
# Clones Doory at the pinned commit into docs/spikes/canvas/doory (git-ignored; Doory's source is never committed here).
set -e
SHA=a65c43a98133344273e6e13cde9d2c98d0270421
cd "$(dirname "$0")/.."
if [ ! -d doory/.git ]; then git clone --quiet https://github.com/shaongitbd/doory doory; fi
git -C doory fetch --quiet origin "$SHA" 2>/dev/null || true
git -C doory checkout --quiet "$SHA"
echo "doory at $(git -C doory rev-parse HEAD)"
