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
go test ./internal/syntax -run=^$ -fuzz=FuzzParseObject -fuzztime="$FUZZTIME"
go test ./internal/syntax -run=^$ -fuzz=FuzzOpenFile -fuzztime="$FUZZTIME"
go test ./internal/filter -run=^$ -fuzz=FuzzDecodeFilters -fuzztime="$FUZZTIME"
go test ./internal/filter -run=^$ -fuzz=FuzzPredictor -fuzztime="$FUZZTIME"
go test ./internal/content -run=^$ -fuzz=FuzzContentLexer -fuzztime="$FUZZTIME"

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
