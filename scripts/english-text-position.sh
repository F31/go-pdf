#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
dir=${1:-/tmp/english-corpus}
ENGLISH_CORPUS=1 ENGLISH_CORPUS_DIR="$dir" ENGLISH_CORPUS_PAGES="${ENGLISH_CORPUS_PAGES:-3}" \
  go test . -run '^TestEnglishCorpusTextPosition$' -v -count=1
