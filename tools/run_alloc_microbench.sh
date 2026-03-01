#!/bin/sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
OUT_DIR="$ROOT_DIR/build/alloc_microbench"
BIN="$ROOT_DIR/build/alloc_microbench.bin"

mkdir -p "$OUT_DIR"

STAGE2="$ROOT_DIR/build/stage2"
RTG="$ROOT_DIR/build/rtg"

if [ ! -x "$RTG" ]; then
  echo "error: compiler binary not found; run ./build/build build first" >&2
  exit 1
fi

COMPILER_USED=""
if [ "${RTG_ALLOC_BENCH_TRY_STAGE2:-}" = "1" ] && [ -x "$STAGE2" ]; then
  STAGE2_LOG="$OUT_DIR/stage2_compile.log"
  if sh -c '"$1" -strict -profile -tags alloc_debug -o "$2" "$3"' sh "$STAGE2" "$BIN" "$ROOT_DIR/tools/alloc_microbench" > "$STAGE2_LOG" 2>&1; then
    COMPILER_USED="$STAGE2"
  else
    echo "warning: stage2 failed to compile alloc microbench, falling back to build/rtg (see $STAGE2_LOG)" >&2
  fi
fi

if [ -z "$COMPILER_USED" ]; then
  "$RTG" -strict -profile -tags alloc_debug -o "$BIN" "$ROOT_DIR/tools/alloc_microbench"
  COMPILER_USED="$RTG"
fi

CASES_FILE="$OUT_DIR/cases.txt"
"$BIN" -list > "$CASES_FILE"

RESULTS_TSV="$OUT_DIR/results.tsv"
RESULTS_MD="$OUT_DIR/report.md"

FIRST=1
while IFS= read -r CASE_NAME; do
  [ -n "$CASE_NAME" ] || continue
  CASE_TSV="$OUT_DIR/$CASE_NAME.tsv"
  CASE_ARENA="$OUT_DIR/$CASE_NAME.arena.report"
  CASE_PROFILE="$OUT_DIR/$CASE_NAME.profile"
  RTG_ARENA_REPORT="$CASE_ARENA" RTG_PROFILE="$CASE_PROFILE" "$BIN" -case "$CASE_NAME" > "$CASE_TSV"
  if [ "$FIRST" -eq 1 ]; then
    head -n 1 "$CASE_TSV" > "$RESULTS_TSV"
    FIRST=0
  fi
  tail -n 1 "$CASE_TSV" >> "$RESULTS_TSV"
done < "$CASES_FILE"

{
  echo "# Allocation Microbench Report"
  echo
  echo "Generated from \`tools/alloc_microbench\` (compiled with \`-profile -tags alloc_debug\`)."
  echo
  echo "Compiler used: \`$COMPILER_USED\`"
  echo
  echo "## Raw Results"
  echo
  echo '```tsv'
  cat "$RESULTS_TSV"
  echo '```'
  echo
  echo "## Top By Requested Bytes"
  echo
  awk 'BEGIN{FS="\t"; OFS="\t"} NR==1{next} {print $1,$6,$8,$9,$4}' "$RESULTS_TSV" | sort -t "$(printf '\t')" -k2,2nr | \
    awk 'BEGIN{print "case\treq_bytes\tmmap_calls\tmmap_bytes\tns_per_op"} {print}'
  echo
  echo "## Top By Mmap Calls"
  echo
  awk 'BEGIN{FS="\t"; OFS="\t"} NR==1{next} {print $1,$8,$9,$6,$4}' "$RESULTS_TSV" | sort -t "$(printf '\t')" -k2,2nr | \
    awk 'BEGIN{print "case\tmmap_calls\tmmap_bytes\treq_bytes\tns_per_op"} {print}'
} > "$RESULTS_MD"

echo "wrote $RESULTS_TSV"
echo "wrote $RESULTS_MD"
