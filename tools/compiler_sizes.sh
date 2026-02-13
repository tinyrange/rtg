#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

OUT_CSV="${1:-compiler_sizes.csv}"
TMP_DIR="build/size_bins"
mkdir -p "$(dirname "$OUT_CSV")" "$TMP_DIR"

DATE_UTC="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
GIT_REF="$(git describe --tags --always --dirty)"

size_bytes() {
  wc -c < "$1" | tr -d '[:space:]'
}

build_size_with_rtg() {
  local rtg="$1"
  local out="$2"
  local target="$3"
  local tags="$4"

  local cmd=("$rtg" -T "$target")
  if [[ -n "$tags" ]]; then cmd+=(-tags "$tags"); fi
  cmd+=(./std/compiler/ -o "$out")
  if "${cmd[@]}" >/dev/null 2>&1; then
    size_bytes "$out"
  else
    echo "NA"
  fi
}

build_host_compiler() {
  local out="$1"

  CGO_ENABLED=0 go build -o "$out" ./std/compiler/
}

HOST_COMPILER="$TMP_DIR/rtg_host"
build_host_compiler "$HOST_COMPILER" >/dev/null 2>&1

# Architecture size measurements (all backends compiled in).
SIZE_LINUX_AMD64_ALL="$(build_size_with_rtg "$HOST_COMPILER" "$TMP_DIR/rtg_linux_amd64_all" linux/amd64 "")"
SIZE_LINUX_386_ALL="$(build_size_with_rtg "$HOST_COMPILER" "$TMP_DIR/rtg_linux_386_all" linux/386 "")"

# Backend size measurements (single backend: exclude all others via no_backend_* tags).
SIZE_BACKEND_AMD64="$(build_size_with_rtg "$HOST_COMPILER" "$TMP_DIR/rtg_backend_amd64" linux/amd64 "no_backend_linux_i386,no_backend_c,no_backend_wasi_wasm32,no_backend_windows_i386")"
SIZE_BACKEND_I386="$(build_size_with_rtg "$HOST_COMPILER" "$TMP_DIR/rtg_backend_i386" linux/386 "no_backend_linux_amd64,no_backend_c,no_backend_wasi_wasm32")"
SIZE_BACKEND_C="$(build_size_with_rtg "$HOST_COMPILER" "$TMP_DIR/rtg_backend_c" linux/amd64 "no_backend_linux_i386,no_backend_wasi_wasm32,no_backend_windows_i386")"
SIZE_BACKEND_ALL="$(build_size_with_rtg "$HOST_COMPILER" "$TMP_DIR/rtg_backend_all" linux/amd64 "")"

if [[ ! -f "$OUT_CSV" ]]; then
  cat > "$OUT_CSV" <<'EOF'
date_utc,git_ref,size_linux_amd64_all,size_linux_386_all,size_backend_amd64,size_backend_i386,size_backend_c,size_backend_all
EOF
fi

echo "$DATE_UTC,$GIT_REF,$SIZE_LINUX_AMD64_ALL,$SIZE_LINUX_386_ALL,$SIZE_BACKEND_AMD64,$SIZE_BACKEND_I386,$SIZE_BACKEND_C,$SIZE_BACKEND_ALL" >> "$OUT_CSV"
echo "wrote size snapshot to $OUT_CSV"
