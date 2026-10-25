#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

go test . -run '^$' \
  -bench 'Benchmark(OpenReaderAt|ExtractText$|ExtractTextEach|Preflight)' \
  -benchmem -count=3

PDF_SLO=1 go test . -run '^TestServerCoreSLO$' -v -count=1
