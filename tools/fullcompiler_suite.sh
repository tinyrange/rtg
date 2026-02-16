#!/usr/bin/env bash
set -euo pipefail

backend="${1:-}"
if [ -z "$backend" ]; then
  echo "usage: $0 <rtg|c|wasm>"
  exit 2
fi

case "$backend" in
  rtg|c|wasm) ;;
  *)
    echo "unsupported backend: $backend"
    exit 2
    ;;
esac

rtg_compiler="${RTG_COMPILER:-./build/rtg}"
rtg_target="${RTG_TARGET:-}"
exe=""
case "$(uname -s)" in
  MINGW*|MSYS*|CYGWIN*) exe=".exe" ;;
esac

for t in tests/fullcompiler/*/main.go; do
  name="$(basename "$(dirname "$t")")"

  case "$name" in
    empty) want="" ;;
    hello) want="hello world" ;;
    *)
      echo "unknown fullcompiler test: $name"
      exit 1
      ;;
  esac

  case "$backend" in
    rtg)
      out="build/fullcompiler_${name}${exe}"
      if [ -n "$rtg_target" ]; then
        "$rtg_compiler" -T "$rtg_target" "$t" -o "$out"
      else
        "$rtg_compiler" "$t" -o "$out"
      fi
      got="$("$out")"
      ;;
    c)
      csrc="build/fullcompiler_${name}.c"
      out="build/fullcompiler_c_${name}${exe}"
      "$rtg_compiler" -T c/64 "$t" -o "$csrc"
      "${CC:-cc}" "$csrc" -o "$out"
      got="$("$out")"
      ;;
    wasm)
      wasm="build/fullcompiler_${name}.wasm"
      "$rtg_compiler" -T wasi/wasm32 "$t" -o "$wasm"
      got="$(wasmtime --dir=. "$wasm")"
      ;;
  esac

  if [ "$got" != "$want" ]; then
    echo "FAIL: $backend/$name expected '$want' got '$got'"
    exit 1
  fi

  echo "PASS: $backend/$name"
done
