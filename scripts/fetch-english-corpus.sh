#!/usr/bin/env bash
# Fetch a small, public-domain English PDF corpus for text-position tests.
# Files and the generated manifest stay outside Git.
set -euo pipefail

dest=${1:-/tmp/english-corpus}
mkdir -p "$dest"
manifest="$dest/manifest.csv"
printf 'file,generator,type,features,language,source,license\n' > "$manifest"

fetch() {
  local file=$1 generator=$2 features=$3 url=$4
  local out="$dest/$file"
  if [ ! -s "$out" ] || ! pdfinfo "$out" >/dev/null 2>&1; then
    echo "fetch $file"
    if ! curl -fL --retry 3 --max-time 180 -o "$out" "$url"; then
      rm -f "$out"
      echo "warning: failed $url" >&2
      return
    fi
  fi
  if ! pdfinfo "$out" >/dev/null 2>&1; then
    rm -f "$out"
    echo "warning: invalid PDF $url" >&2
    return
  fi
  printf '%s,%s,text-native,%s,en,%s,US Government work public domain (17 USC 105)\n' \
    "$file" "$generator" "$features" "$url" >> "$manifest"
}

base='https://www.govinfo.gov/content/pkg'
for d in 2024-01-10 2024-02-06 2024-04-09 2024-05-07 2024-06-04 2024-07-09 2024-09-10 2024-10-01; do
  fetch "CREC-$d.pdf" "govinfo-congressional-record" "columns;proceedings" "$base/CREC-$d/pdf/CREC-$d.pdf"
done
fetch "PLAW-117publ58.pdf" "govinfo-public-law" "law;long-document" "$base/PLAW-117publ58/pdf/PLAW-117publ58.pdf"
fetch "NIST.SP.800-207.pdf" "nist" "technical-report" 'https://nvlpubs.nist.gov/nistpubs/SpecialPublications/NIST.SP.800-207.pdf'
for p in 601us1r01_q86b 601us1r02_c07d 601us1r03_4gcj 601us1r04_4g15 601us1r05_1qm2; do
  fetch "$p.pdf" "supreme-court" "opinion;encrypted-user-empty" "https://www.supremecourt.gov/opinions/23pdf/$p.pdf"
done

echo "corpus: $dest"
echo "manifest: $manifest"
awk -F, 'NR>1 {n++; if ($3!="") t++} END {printf "files=%d\n", n}' "$manifest"
