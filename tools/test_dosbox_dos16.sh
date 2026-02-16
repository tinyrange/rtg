#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"

if [[ ! -x ./build/rtg ]]; then
  echo "error: ./build/rtg not found; run './build/build build' or './build/rtg tools/build.go -o build/build'" >&2
  exit 1
fi

if ! command -v dosbox-x >/dev/null 2>&1; then
  echo "error: dosbox-x not found in PATH" >&2
  exit 1
fi

OUT_DIR="$ROOT_DIR/build/dos"
mkdir -p "$OUT_DIR"
rm -f "$OUT_DIR"/T??.COM "$OUT_DIR"/O??.TXT "$OUT_DIR"/S??.TXT "$OUT_DIR"/A??.TXT

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

echo "== DOS 16 COM smoke ladder =="

for i in "${!TESTS[@]}"; do
  test_path="${TESTS[$i]}"
  idx=$((i + 1))
  dos_idx="$(printf "%02d" "$idx")"
  dos_name="T${dos_idx}.COM"
  out_name="O${dos_idx}.TXT"
  status_name="S${dos_idx}.TXT"
  after_name="A${dos_idx}.TXT"
  out_path="$OUT_DIR/$dos_name"
  total_count=$((total_count + 1))

  ./build/rtg -T dos/16 "$test_path" -o "$out_path"

  SDL_VIDEODRIVER=dummy SDL_AUDIODRIVER=dummy \
    dosbox-x -noconfig -nogui -time-limit 20 \
      -set "sdl output=surface" \
      -c "mount c $OUT_DIR" \
      -c "c:" \
      -c "$dos_name > $out_name" \
      -c "if errorlevel 1 echo FAIL > $status_name" \
      -c "if not errorlevel 1 echo OK > $status_name" \
      -c "echo AFTER > $after_name" \
      -c "exit" >/dev/null 2>"$OUT_DIR/dosbox_${dos_idx}.log"

  status="$(tr -d '\r\n' <"$OUT_DIR/$status_name" 2>/dev/null || true)"
  after="$(tr -d '\r\n' <"$OUT_DIR/$after_name" 2>/dev/null || true)"

  has_pass_text=0
  if [[ -f "$OUT_DIR/$out_name" ]] && tr -d '\r' <"$OUT_DIR/$out_name" | rg -q "PASS"; then
    has_pass_text=1
  fi

  if [[ "$status" == "OK" && "$after" == "AFTER" && ( "$require_pass_text" != "1" || "$has_pass_text" == "1" ) ]]; then
    pass_count=$((pass_count + 1))
    outcome="PASS"
  else
    outcome="FAIL"
  fi

  bytes="$(wc -c <"$OUT_DIR/$out_name" | tr -d ' ')"
  echo "$(printf "%2d" "$idx"). $test_path -> $outcome (stdout_bytes=$bytes, pass_text=$has_pass_text)"
done

echo "Summary: $pass_count/$total_count executables returned cleanly in DOSBox-X"
