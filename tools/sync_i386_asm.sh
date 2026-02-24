#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
TEMPLATE="$ROOT_DIR/std/compiler/backend/i386/shared/i386_asm.tmpl"
OUT_LINUX="$ROOT_DIR/std/compiler/backend/i386/linux/i386.go"
OUT_WINDOWS="$ROOT_DIR/std/compiler/backend/i386/windows/i386.go"

if [[ ! -f "$TEMPLATE" ]]; then
  echo "error: missing template: $TEMPLATE" >&2
  exit 1
fi

generate() {
  local build_tag=$1
  local pkg=$2
  local out=$3

  BUILD_TAG="$build_tag" PKG="$pkg" perl -pe '
    s/__BUILD_TAG__/$ENV{"BUILD_TAG"}/g;
    s/__PACKAGE__/$ENV{"PKG"}/g;
  ' "$TEMPLATE" >"$out"
}

generate '!no_backend_i386 && !no_backend_linux_i386' linux "$OUT_LINUX"
generate '!no_backend_i386 && !no_backend_windows_i386' windows "$OUT_WINDOWS"

echo "updated:"
echo "  $OUT_LINUX"
echo "  $OUT_WINDOWS"
