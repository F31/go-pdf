#!/usr/bin/env bash
set -euo pipefail

# G0 allows only the standard library and golang.org/x/*. A new dependency must
# be approved by an ADR and added to this allow-list deliberately.
if ! grep -qE '^require|^[[:space:]]+[a-zA-Z0-9./-]+[[:space:]]+v' go.mod; then
  echo "no external dependencies"
  exit 0
fi

bad=$(awk '
  /^require[[:space:]]*\(/ { inblock=1; next }
  inblock && /^\)/ { inblock=0; next }
  inblock && NF >= 2 { print $1 }
  /^require[[:space:]]/ && NF >= 3 { print $2 }
' go.mod | grep -v '^golang.org/x/' || true)

if [ -n "$bad" ]; then
  echo "dependency whitelist violation:" >&2
  echo "$bad" >&2
  exit 1
fi

echo "dependency whitelist ok"
