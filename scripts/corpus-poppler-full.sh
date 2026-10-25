#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
out=${POPPLER_CORPUS_DIR:-/tmp/poppler-corpus}
jobs=${POPPLER_CORPUS_SHARDS:-16}
chunk_pages=${POPPLER_CORPUS_CHUNK_PAGES:-128}
pages_limit=${POPPLER_CORPUS_PAGES:-0}
page_start=${POPPLER_CORPUS_PAGE_START:-1}
page_end=${POPPLER_CORPUS_PAGE_END:-0}
sample_chunks=${POPPLER_CORPUS_SAMPLE_CHUNKS:-1}
format=${POPPLER_CORPUS_FORMAT:-ppm}
chunk=${POPPLER_CORPUS_CHUNK:-32}

if ! command -v pdfinfo >/dev/null 2>&1; then
  echo "pdfinfo is required to page-shard the corpus" >&2
  exit 1
fi
if (( jobs < 1 )); then jobs=1; fi
if (( chunk_pages < 1 )); then chunk_pages=1; fi
if (( sample_chunks < 1 )); then sample_chunks=1; fi

mkdir -p "$out/parts"
out=$(cd "$out" && pwd)
rm -f "$out"/parts/*.csv "$out"/parts/*.log "$out"/jobs.bin

bin="$out/render.test"
echo "build $bin"
go test -c -o "$bin" ./render

cat > "$out/run-job.sh" <<'JOB'
#!/usr/bin/env bash
set -euo pipefail
path=$1
start=$2
end=$3
part=$4
cd "$REPO_ROOT/render"
POPPLER_CORPUS=1 \
POPPLER_CORPUS_PATH="$path" \
POPPLER_CORPUS_PAGE_START="$start" \
POPPLER_CORPUS_PAGE_END="$end" \
POPPLER_CORPUS_PAGES=0 \
POPPLER_CORPUS_SHARDS=1 \
POPPLER_CORPUS_SHARD=0 \
POPPLER_CORPUS_FORMAT="$CORPUS_FORMAT" \
POPPLER_CORPUS_CHUNK="$CORPUS_CHUNK" \
POPPLER_CORPUS_REPORT="$OUT_DIR/parts/$part.csv" \
  "$CORPUS_BIN" -test.run TestPopplerCorpusQuality -test.v -test.timeout 24h \
  > "$OUT_DIR/parts/$part.log" 2>&1
JOB
chmod +x "$out/run-job.sh"

export REPO_ROOT="$root" OUT_DIR="$out" CORPUS_BIN="$bin"
export CORPUS_FORMAT="$format" CORPUS_CHUNK="$chunk"

shopt -s nullglob
filter=${POPPLER_CORPUS_FILE:-}
file_limit=${POPPLER_CORPUS_FILES:-0}
seen=0
job=0
pages_total=0
for file in "$root"/docs/pdf/*.pdf; do
  base=$(basename "$file")
  if [[ -n "$filter" && "$base" != *"$filter"* ]]; then
    continue
  fi
  if (( file_limit > 0 && seen >= file_limit )); then
    break
  fi
  seen=$((seen + 1))
  pages=$(pdfinfo "$file" 2>/dev/null | awk '/^Pages:/{print $2; exit}')
  if [[ ! "$pages" =~ ^[0-9]+$ ]] || (( pages < 1 )); then
    echo "skip $base: cannot read page count" >&2
    continue
  fi
  if (( pages_limit > 0 && pages > pages_limit )); then
    pages=$pages_limit
  fi
  if (( page_end > 0 && pages > page_end )); then
    pages=$page_end
  fi
  if (( page_start > pages )); then
    continue
  fi
  for ((start = page_start; start <= pages; start += chunk_pages)); do
    end=$((start + chunk_pages - 1))
    if (( end > pages )); then end=$pages; fi
    if (( sample_chunks > 1 && job % sample_chunks != 0 )); then
      job=$((job + 1))
      continue
    fi
    part=$(printf '%04d' "$job")
    printf '%s\0%s\0%s\0%s\0' "$file" "$start" "$end" "$part" >> "$out/jobs.bin"
    pages_total=$((pages_total + end - start + 1))
    job=$((job + 1))
  done
done

if (( job == 0 )); then
  echo "no corpus jobs; see docs/pdf and POPPLER_CORPUS_FILE" >&2
  exit 1
fi

echo "jobs=$job pages=$pages_total workers=$jobs chunk_pages=$chunk_pages format=$format"
status=0
xargs -0 -n4 -P "$jobs" "$out/run-job.sh" < "$out/jobs.bin" || status=$?

parts=("$out"/parts/*.csv)
if [[ ! -e "${parts[0]}" ]]; then
  echo "no report parts; see $out/parts/*.log" >&2
  exit 1
fi
{
  head -n 1 "${parts[0]}"
  for f in "${parts[@]}"; do
    tail -n +2 "$f"
  done | LC_ALL=C sort -t, -k1,1 -k2,2n
} > "$out/poppler-corpus.csv"

echo "report: $out/poppler-corpus.csv"
wc -l "$out/poppler-corpus.csv"
exit "$status"
