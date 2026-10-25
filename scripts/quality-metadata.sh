#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

json_escape() { printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'; }
value() { "$@" 2>&1 | head -1 || true; }

commit=$(git rev-parse HEAD 2>/dev/null || echo unknown)
go_version=$(go version)
os=$(go env GOOS)
arch=$(go env GOARCH)
poppler=$(value pdftoppm -v)
cairo=$(value pdftocairo -v)
freetype=$(pkg-config --modversion freetype2 2>/dev/null || echo unknown)
font_dir=${POPPLER_CORPUS_FONT_DIR:-builtin}
annotations=${POPPLER_CORPUS_ANNOTATIONS:-hidden}

cat <<JSON
{
  "commit": "$(json_escape "$commit")",
  "go": "$(json_escape "$go_version")",
  "os": "$(json_escape "$os")",
  "arch": "$(json_escape "$arch")",
  "poppler": "$(json_escape "$poppler")",
  "pdftocairo": "$(json_escape "$cairo")",
  "freetype": "$(json_escape "$freetype")",
  "font_source": "$(json_escape "$font_dir")",
  "dpi": 72,
  "background": "white",
  "icc": "sRGB-preview",
  "annotations": "$(json_escape "$annotations")"
}
JSON
