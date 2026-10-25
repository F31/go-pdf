#!/usr/bin/env bash
set -euo pipefail

# Core must not link optional Extended process adapters. The neutral extension
# interfaces are allowed; concrete providers must remain explicit imports.
deps=$(go list -deps .)
bad=$(printf '%s\n' "$deps" | grep -E '^github\.com/F31/go-pdf/extension/(poppler|cairo|openjpeg|jbig2|legacy)$' || true)
if [ -n "$bad" ]; then
  echo "core imports optional provider packages:" >&2
  echo "$bad" >&2
  exit 1
fi
echo "core/provider separation ok"
