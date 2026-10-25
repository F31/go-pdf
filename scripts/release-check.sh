#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

echo "== release metadata =="
./scripts/quality-metadata.sh

echo "== worktree =="
if [ -n "$(git status --porcelain)" ]; then
  echo "warning: worktree is not clean" >&2
fi

echo "== build/test/vet =="
CGO_ENABLED=0 go build ./...
go test ./...
CGO_ENABLED=0 go test ./...
go test -race ./...
go vet ./...

if command -v staticcheck >/dev/null 2>&1; then
  staticcheck ./...
else
  echo "staticcheck unavailable" >&2
fi

echo "== dependency and provider boundaries =="
./scripts/check-deps.sh
./scripts/check-core-deps.sh
test -f THIRD_PARTY.md

echo "== static binary =="
out=$(mktemp -d)
trap 'rm -rf "$out"' EXIT
CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o "$out/pdf" ./cmd/pdf
file "$out/pdf"
ls -l "$out/pdf"
go version -m "$out/pdf" | sed -n '1,8p'

echo "== optional performance gate =="
if [ "${PERF_GATE:-0}" = "1" ]; then
  ./scripts/perf-gate.sh
else
  echo "skipped (set PERF_GATE=1 to enforce host-calibrated P95 limits)"
fi

echo "release checks complete"
