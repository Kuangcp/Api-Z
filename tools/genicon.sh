#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
if ! command -v rsvg-convert >/dev/null 2>&1; then
  echo "rsvg-convert is required (librsvg)" >&2
  exit 1
fi
rsvg-convert -w 512 -h 512 assets/icon.svg -o assets/icon-512.png
rsvg-convert -w 256 -h 256 assets/icon.svg -o assets/icon-256.png
rsvg-convert -w 128 -h 128 assets/icon.svg -o assets/icon-128.png
