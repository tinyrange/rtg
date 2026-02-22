#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"

SRC_DIR="$ROOT_DIR/build/floppy-src"
IMG_PATH="${RTG_DOS_FLOPPY_IMAGE:-$ROOT_DIR/build/rtg-dos-tests.img}"

need() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required tool: $1" >&2
    exit 1
  }
}

need mformat
need mcopy
need rg

if [[ ! -x ./build/rtg ]]; then
  if [[ -x ./build/build ]]; then
    ./build/build build
  else
    echo "error: ./build/rtg not found; run './build/build build' first" >&2
    exit 1
  fi
fi

mkdir -p "$SRC_DIR"
rm -f "$SRC_DIR"/*.COM "$SRC_DIR"/*.BAT "$SRC_DIR"/*.TXT

tests=()
if [[ -n "${RTG_DOS_TESTS:-}" ]]; then
  RTG_DOS_TESTS="${RTG_DOS_TESTS//,/ }"
  read -r -a tests <<<"$RTG_DOS_TESTS"
else
  while IFS= read -r test_path; do
    tests+=("$test_path")
  done < <(rg --files tests -g '*.go' | sort)
fi

if [[ "${#tests[@]}" -eq 0 ]]; then
  echo "error: no tests selected" >&2
  exit 1
fi

to_crlf_file() {
  local dst="$1"
  shift
  : >"$dst"
  while (($#)); do
    printf '%s\r\n' "$1" >>"$dst"
    shift
  done
}

runall="$SRC_DIR/RUNALL.BAT"
index_txt="$SRC_DIR/INDEX.TXT"
readme_txt="$SRC_DIR/README.TXT"

to_crlf_file "$runall" "@ECHO OFF" "ECHO RTG DOS/8086 FULLCOMPILER SUITE" "ECHO."
to_crlf_file "$index_txt" "RTG DOS/8086 EXE TEST INDEX"
to_crlf_file "$readme_txt" \
  "RTG DOS/8086 test disk" \
  "" \
  "Usage on DOS 6.22:" \
  "  A:" \
  "  RUNALL" \
  "" \
  "Each test returns ERRORLEVEL 0 on PASS."

for i in "${!tests[@]}"; do
  idx=$((i + 1))
  dos_name="$(printf "T%03d.EXE" "$idx")"
  stem="$(basename "${tests[$i]}" .go)"
  display="$(printf "%03d/%03d %s" "$idx" "${#tests[@]}" "$stem")"

  ./build/rtg -T dos/8086 "${tests[$i]}" -o "$SRC_DIR/$dos_name"

  printf 'ECHO %s\r\n' "$display" >>"$runall"
  printf '%s\r\n' "$dos_name" >>"$runall"
  printf 'IF ERRORLEVEL 1 ECHO FAIL %s\r\n' "$dos_name $stem" >>"$runall"
  printf 'IF NOT ERRORLEVEL 1 ECHO PASS %s\r\n' "$dos_name $stem" >>"$runall"
  printf 'ECHO.\r\n' >>"$runall"

  printf '%s %s\r\n' "$dos_name" "${tests[$i]}" >>"$index_txt"
done

printf 'ECHO DONE.\r\n' >>"$runall"

bytes_total="$(du -ck "$SRC_DIR"/* | tail -1 | awk '{print $1}')"
if [[ "$bytes_total" -gt 1440 ]]; then
  echo "warning: source files exceed 1.44MB image size budget (${bytes_total}KB > 1440KB)" >&2
fi

rm -f "$IMG_PATH"
dd if=/dev/zero of="$IMG_PATH" bs=1024 count=1440 status=none
mformat -i "$IMG_PATH" -f 1440 -v RTGTEST ::
mcopy -i "$IMG_PATH" "$SRC_DIR"/* ::

echo "wrote floppy image: $IMG_PATH"
echo "files staged in: $SRC_DIR"
echo "tests compiled: ${#tests[@]}"
