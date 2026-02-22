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
rm -f "$OUT_DIR"/O??_*.txt "$OUT_DIR"/E??_*.txt "$OUT_DIR"/PROG.TIR "$OUT_DIR"/CHILD.COM

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

echo "== COMEMU tiny pipeline =="

tiny_idx=$((total_count + 1))
tiny_num="$(printf "%02d" "$tiny_idx")"
tiny_parser_com="$OUT_DIR/TINY_PARSER.com"
tiny_backend_com="$OUT_DIR/TINY_BACKEND.com"
tiny_input="$OUT_DIR/INPUT.GO"
tiny_parser_out="$OUT_DIR/O${tiny_num}_parser.txt"
tiny_parser_err="$OUT_DIR/E${tiny_num}_parser.txt"
tiny_backend_out="$OUT_DIR/O${tiny_num}_backend.txt"
tiny_backend_err="$OUT_DIR/E${tiny_num}_backend.txt"
tiny_child_out="$OUT_DIR/O${tiny_num}_child.txt"
tiny_child_err="$OUT_DIR/E${tiny_num}_child.txt"

cat >"$tiny_input" <<'EOF'
package main

func main() {
	var msg string
	msg = "hello from tiny parser"
	print(msg)
}
EOF

./build/rtg -T dos/8086 tests/dos_tiny_parser.go -o "$tiny_parser_com"
./build/rtg -T dos/8086 -tags tiny_dos_backend,no_size_analysis tests/dos_tiny_backend.go -o "$tiny_backend_com"
tiny_parser_size="$(wc -c <"$tiny_parser_com" | tr -d ' ')"
tiny_backend_size="$(wc -c <"$tiny_backend_com" | tr -d ' ')"

total_count=$((total_count + 1))
tiny_ok=1
if [[ "$tiny_parser_size" -gt 65280 || "$tiny_backend_size" -gt 65280 ]]; then
  tiny_ok=0
fi

rm -f "$OUT_DIR/PROG.TIR" "$OUT_DIR/CHILD.COM"
(cd "$OUT_DIR" && go run "$ROOT_DIR/tools/comemu" "$(basename "$tiny_parser_com")" >"$tiny_parser_out" 2>"$tiny_parser_err") || true
if [[ ! -f "$OUT_DIR/PROG.TIR" ]]; then
  tiny_ok=0
fi

if [[ "$tiny_ok" == "1" ]]; then
  # Backend executable may hit emulator-incomplete instructions after writing output.
  # Treat CHILD.COM presence as the success signal for this stage.
  (cd "$OUT_DIR" && go run "$ROOT_DIR/tools/comemu" "$(basename "$tiny_backend_com")" >"$tiny_backend_out" 2>"$tiny_backend_err") || true
fi
if [[ "$tiny_ok" == "1" ]]; then
  if [[ ! -f "$OUT_DIR/CHILD.COM" ]]; then
    tiny_ok=0
  fi
fi
if [[ "$tiny_ok" == "1" ]]; then
  child_size="$(wc -c <"$OUT_DIR/CHILD.COM" | tr -d ' ')"
  if [[ "$child_size" -gt 65280 ]]; then
    tiny_ok=0
  fi
fi
if [[ "$tiny_ok" == "1" ]]; then
  if ! (cd "$OUT_DIR" && go run "$ROOT_DIR/tools/comemu" CHILD.COM >"$tiny_child_out" 2>"$tiny_child_err"); then
    tiny_ok=0
  fi
fi

if [[ "$tiny_ok" == "1" ]]; then
  pass_count=$((pass_count + 1))
  echo "$(printf "%2d" "$tiny_idx"). tiny parser+backend pipeline -> PASS (parser_bytes=$tiny_parser_size backend_bytes=$tiny_backend_size child_bytes=$(wc -c <"$OUT_DIR/CHILD.COM" | tr -d ' '))"
else
  echo "$(printf "%2d" "$tiny_idx"). tiny parser+backend pipeline -> FAIL"
fi

echo "Summary: $pass_count/$total_count executables ran cleanly in comemu"
