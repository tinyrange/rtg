#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"

if [[ ! -x ./build/rtg ]]; then
  echo "error: ./build/rtg not found; run build first" >&2
  exit 1
fi

OUT_DIR="$ROOT_DIR/build/comemu-dos"
mkdir -p "$OUT_DIR"
rm -f "$OUT_DIR"/T??.com "$OUT_DIR"/O??.txt "$OUT_DIR"/E??.txt

if [[ -n "${RTG_DOS_TESTS:-}" ]]; then
  RTG_DOS_TESTS="${RTG_DOS_TESTS//,/ }"
  read -r -a TESTS <<<"$RTG_DOS_TESTS"
else
  TESTS=(
    tests/func_basic.go
    tests/ops_arithmetic.go
    tests/flow_if.go
    tests/slice_basic.go
    tests/struct_basic.go
    tests/string_builder.go
  )
fi

pass_count=0
total_count=0
require_pass_text="${RTG_DOS_REQUIRE_PASS:-1}"

echo "== COMEMU DOS 8086 ladder =="

for i in "${!TESTS[@]}"; do
  test_path="${TESTS[$i]}"
  idx=$((i + 1))
  num="$(printf "%02d" "$idx")"
  com_path="$OUT_DIR/T${num}.com"
  out_path="$OUT_DIR/O${num}.txt"
  err_path="$OUT_DIR/E${num}.txt"
  total_count=$((total_count + 1))

  ./build/rtg -T dos/8086 "$test_path" -o "$com_path"

  run_ok=1
  if go run ./tools/comemu "$com_path" >"$out_path" 2>"$err_path"; then
    run_ok=0
  fi

  has_pass_text=0
  if tr -d '\r' <"$out_path" | rg -q "PASS"; then
    has_pass_text=1
  fi

  if [[ "$run_ok" == "0" && ( "$require_pass_text" != "1" || "$has_pass_text" == "1" ) ]]; then
    pass_count=$((pass_count + 1))
    outcome="PASS"
  else
    outcome="FAIL"
  fi

  bytes="$(wc -c <"$out_path" | tr -d ' ')"
  echo "$(printf "%2d" "$idx"). $test_path -> $outcome (stdout_bytes=$bytes, pass_text=$has_pass_text)"
done

echo "Summary: $pass_count/$total_count executables ran cleanly in comemu"
