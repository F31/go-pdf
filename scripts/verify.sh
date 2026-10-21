#!/usr/bin/env bash
set -euo pipefail

FUZZTIME="${FUZZTIME:-10s}"
echo "== build (CGO_ENABLED=0) =="
CGO_ENABLED=0 go build ./...

echo "== vet =="
go vet ./...

echo "== test -race =="
go test -race ./...

echo "== dependency whitelist =="
./scripts/check-deps.sh

echo "== cross compile =="
for os in linux darwin windows; do
  for arch in amd64 arm64; do
    echo "  $os/$arch"
    CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build ./...
  done
done

echo "== fuzz smoke ($FUZZTIME) =="
go test ./internal/syntax -run=^$ -fuzz=FuzzParseObject -fuzztime="$FUZZTIME" -parallel=4
go test ./internal/syntax -run=^$ -fuzz=FuzzOpenFile -fuzztime="$FUZZTIME" -parallel=4
go test ./internal/filter -run=^$ -fuzz=FuzzDecodeFilters -fuzztime="$FUZZTIME" -parallel=4
go test ./internal/filter -run=^$ -fuzz=FuzzPredictor -fuzztime="$FUZZTIME" -parallel=4
go test ./internal/content -run=^$ -fuzz=FuzzContentLexer -fuzztime="$FUZZTIME" -parallel=4
go test ./internal/font -run=^$ -fuzz=FuzzParseToUnicode -fuzztime="$FUZZTIME" -parallel=4
go test ./internal/font -run=^$ -fuzz=FuzzPredefinedSplit -fuzztime="$FUZZTIME" -parallel=4
go test ./internal/font -run=^$ -fuzz=FuzzParseEmbeddedCMap -fuzztime="$FUZZTIME" -parallel=4
go test ./internal/font -run=^$ -fuzz=FuzzSFNTRepair -fuzztime="$FUZZTIME" -parallel=4
go test ./internal/image -run=^$ -fuzz=FuzzImageDecode -fuzztime="$FUZZTIME" -parallel=4
go test ./internal/function -run=^$ -fuzz=FuzzPostScript -fuzztime="$FUZZTIME" -parallel=4

if command -v staticcheck >/dev/null 2>&1; then
  echo "== staticcheck =="
  staticcheck ./...
else
  echo "== staticcheck skipped (not installed) ==" >&2
fi

if command -v govulncheck >/dev/null 2>&1; then
  echo "== govulncheck =="
  govulncheck ./...
else
  echo "== govulncheck skipped (not installed) ==" >&2
fi

echo "== verify complete =="
