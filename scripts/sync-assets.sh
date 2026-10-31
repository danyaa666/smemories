#!/usr/bin/env bash
# E09 T-151: copy the asset registry to the folder the Go side embeds. Run it after editing web/src/book/assets/registry.json;
# TestRegistryMatchesWeb (internal/assets) and the web registry test fail when the two copies differ.
set -euo pipefail
cd "$(dirname "$0")/.."
cp web/src/book/assets/registry.json internal/assets/registry.json
echo "sync-assets: internal/assets/registry.json updated"
