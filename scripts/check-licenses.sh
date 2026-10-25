#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

test -f THIRD_PARTY.md || { echo "THIRD_PARTY.md missing" >&2; exit 1; }

echo "module licence inventory"
while read -r path version dir; do
  [ -z "$path" ] && continue
  if ! grep -Fq "$path" THIRD_PARTY.md; then
    echo "module $path $version is not recorded in THIRD_PARTY.md" >&2
    exit 1
  fi
  licence=""
  for name in LICENSE LICENSE.txt LICENSE.md COPYING COPYING.txt; do
    if [ -f "$dir/$name" ]; then
      licence="$dir/$name"
      break
    fi
  done
  if [ -z "$licence" ]; then
    echo "module $path has no detectable licence file" >&2
    exit 1
  fi
  first=$(sed -n '1p' "$licence" | tr -d '\r')
  printf '  %-40s %-10s %s\n' "$path" "$version" "$first"
done < <(go list -deps -f '{{with .Module}}{{if not .Main}}{{.Path}} {{.Version}} {{.Dir}}{{end}}{{end}}' ./... | sort -u)

echo "licence inventory ok"
