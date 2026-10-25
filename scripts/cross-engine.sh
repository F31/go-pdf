#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

if [ "$#" -lt 1 ]; then
  echo "usage: $0 <pdf> [page]" >&2
  exit 2
fi
pdf=$1
page=${2:-1}
for bin in pdfinfo pdftotext; do
  command -v "$bin" >/dev/null || { echo "missing $bin" >&2; exit 3; }
done

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
go build -o "$tmp/pdf" ./cmd/pdf

"$tmp/pdf" info -json "$pdf" >"$tmp/go-info.json"
pdfinfo "$pdf" >"$tmp/ref-info.txt" 2>/dev/null || true
"$tmp/pdf" text -page "$page" "$pdf" >"$tmp/go-text.txt" 2>/dev/null || true
pdftotext -f "$page" -l "$page" -layout "$pdf" "$tmp/ref-text.txt" 2>/dev/null || true

python3 - "$tmp/go-info.json" "$tmp/ref-info.txt" "$tmp/go-text.txt" "$tmp/ref-text.txt" "$page" <<'PY'
import json, re, sys
from pathlib import Path

go_info = json.loads(Path(sys.argv[1]).read_text() or "{}")
ref_info = Path(sys.argv[2]).read_text(errors="replace")
go_text = Path(sys.argv[3]).read_text(errors="replace")
ref_text = Path(sys.argv[4]).read_text(errors="replace")

def field(name):
    m = re.search(rf"^{re.escape(name)}:\s*(.+)$", ref_info, re.M)
    return m.group(1).strip() if m else ""

norm = lambda s: re.sub(r"\s+", " ", s).strip()
go_pages, ref_pages = go_info.get("pages", 0), field("Pages")
go_enc = bool(go_info.get("encrypted", False))
ref_enc = field("Encrypted").lower().startswith("yes")
text_equal = norm(go_text) == norm(ref_text)

print("cross-engine semantic comparison")
print(f"  pages: go-pdf={go_pages} poppler={ref_pages} equal={str(go_pages)==ref_pages if ref_pages else 'n/a'}")
print(f"  encrypted: go-pdf={go_enc} poppler={ref_enc} equal={go_enc==ref_enc if ref_info else 'n/a'}")
print(f"  text page {sys.argv[5]}: normalized_equal={text_equal} go_len={len(norm(go_text))} poppler_len={len(norm(ref_text))}")
print("  links/forms/structure: not compared by this script; use pdf CLI JSON and Poppler tools explicitly")
if ref_pages and str(go_pages) != ref_pages:
    sys.exit(1)
if ref_info and go_enc != ref_enc:
    sys.exit(1)
PY
