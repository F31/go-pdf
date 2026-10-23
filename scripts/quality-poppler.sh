#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -lt 1 ]; then
  echo "usage: $0 file.pdf [page] [dpi] [outdir]" >&2
  exit 2
fi

pdf=$1
page=${2:-1}
dpi=${3:-150}
outdir=${4:-$(mktemp -d)}
mkdir -p "$outdir"

if ! command -v pdftoppm >/dev/null 2>&1; then
  echo "quality-poppler: pdftoppm not found" >&2
  exit 2
fi

go run ./cmd/pdf render -o "$outdir/go.png" -page "$page" -dpi "$dpi" -background white "$pdf"
pdftoppm -f "$page" -l "$page" -r "$dpi" -png -singlefile "$pdf" "$outdir/poppler"

go run ./cmd/pdf compare \
  -min-ssim 0.98 \
  -max-deltae-p95 3 \
  -max-deltae 1 \
  "$outdir/go.png" "$outdir/poppler.png"

echo "quality artifacts: $outdir"
