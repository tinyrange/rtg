#!/bin/sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
OUT_DIR="$ROOT_DIR/build/alloc_microbench_compare"
RTG_BIN="$ROOT_DIR/build/alloc_microbench.compare.rtg.bin"
GO_BIN="$ROOT_DIR/build/alloc_microbench.compare.go.bin"
CASES_FILE="$OUT_DIR/cases.txt"
GO_CASES_FILE="$OUT_DIR/go_cases.txt"
SAMPLES=${RTG_ALLOC_BENCH_SAMPLES:-3}

mkdir -p "$OUT_DIR"

if [ $((SAMPLES % 2)) -ne 1 ]; then
  echo "error: RTG_ALLOC_BENCH_SAMPLES must be odd (got $SAMPLES)" >&2
  exit 1
fi

RTG_COMPILER="${RTG_ALLOC_BENCH_RTG_COMPILER:-auto}"
if [ "$RTG_COMPILER" = auto ]; then
  if [ -x "$ROOT_DIR/build/stage2" ]; then
    RTG_COMPILER="$ROOT_DIR/build/stage2"
  else
    RTG_COMPILER="$ROOT_DIR/build/rtg"
  fi
fi

if [ ! -x "$RTG_COMPILER" ]; then
  echo "error: RTG compiler not found: $RTG_COMPILER" >&2
  exit 1
fi

"$RTG_COMPILER" -strict -profile -tags alloc_debug -o "$RTG_BIN" "$ROOT_DIR/tools/alloc_microbench"
go build -o "$GO_BIN" ./tools/alloc_microbench_go

"$RTG_BIN" -list > "$CASES_FILE"
"$GO_BIN" -list > "$GO_CASES_FILE"
cmp "$CASES_FILE" "$GO_CASES_FILE"

run_impl() {
  impl_name=$1
  bin=$2
  results="$OUT_DIR/$impl_name.results.tsv"
  samples_dir="$OUT_DIR/$impl_name.samples"
  sorted_tmp="$OUT_DIR/$impl_name.sorted.tsv"
  median_index=$((SAMPLES / 2 + 1))

  mkdir -p "$samples_dir"
  first=1
  while IFS= read -r case_name; do
    [ -n "$case_name" ] || continue
    case_rows="$samples_dir/$case_name.rows.tsv"
    : > "$case_rows"
    sample=1
    while [ "$sample" -le "$SAMPLES" ]; do
      sample_out="$samples_dir/$case_name.$sample.tsv"
      "$bin" -case "$case_name" > "$sample_out"
      if [ "$first" -eq 1 ] && [ "$sample" -eq 1 ]; then
        head -n 1 "$sample_out" > "$results"
        first=0
      fi
      tail -n 1 "$sample_out" >> "$case_rows"
      sample=$((sample + 1))
    done
    sort -t "$(printf '\t')" -k4,4n "$case_rows" | awk -v mid="$median_index" 'NR==mid { print; exit }' >> "$results"
  done < "$CASES_FILE"

  {
    head -n 1 "$results"
    tail -n +2 "$results" | sort -t "$(printf '\t')" -k1,1
  } > "$sorted_tmp"
}

run_impl rtg "$RTG_BIN"
run_impl go "$GO_BIN"

COMPARE_TSV="$OUT_DIR/compare.tsv"
COMPARE_MD="$OUT_DIR/report.md"

awk '
BEGIN { FS = OFS = "\t" }
NR == FNR {
  if (FNR == 1) {
    next
  }
  go_ns[$1] = $4 + 0
  go_alloc[$1] = $5 + 0
  go_bytes[$1] = $6 + 0
  next
}
FNR == 1 {
  print "case", "rtg_ns_per_op", "go_ns_per_op", "rtg_over_go_ns", "rtg_alloc_calls", "go_alloc_calls", "rtg_over_go_allocs", "rtg_req_bytes", "go_req_bytes", "rtg_over_go_bytes"
  next
}
{
  goNs = go_ns[$1]
  goAlloc = go_alloc[$1]
  goBytes = go_bytes[$1]
  nsRatio = 0
  allocRatio = 0
  byteRatio = 0
  if (goNs > 0) {
    nsRatio = $4 / goNs
  }
  if (goAlloc > 0) {
    allocRatio = $5 / goAlloc
  }
  if (goBytes > 0) {
    byteRatio = $6 / goBytes
  }
  print $1, $4, goNs, nsRatio, $5, goAlloc, allocRatio, $6, goBytes, byteRatio
}
' "$OUT_DIR/go.sorted.tsv" "$OUT_DIR/rtg.sorted.tsv" > "$COMPARE_TSV"

{
  echo "# Allocation Microbench RTG vs Go Report"
  echo
  echo "RTG compiler used: \`$RTG_COMPILER\`"
  echo
  echo "Samples per case: \`$SAMPLES\`"
  echo
  echo "The RTG row uses the median sample by \`ns_per_op\`."
  echo "The Go row uses the median sample by \`ns_per_op\`."
  echo
  echo "Allocator cases are only approximate across implementations:"
  echo "- RTG uses direct \`runtime.Alloc\`."
  echo "- Go uses \`make([]byte, size)\` plus \`MemStats\` deltas."
  echo
  echo "## Comparison"
  echo
  echo '```tsv'
  cat "$COMPARE_TSV"
  echo '```'
  echo
  echo "## Slowest RTG/Go Ratios"
  echo
  head -n 1 "$COMPARE_TSV"
  tail -n +2 "$COMPARE_TSV" | sort -t "$(printf '\t')" -k4,4nr
  echo
  echo "## RTG Raw Results"
  echo
  echo '```tsv'
  cat "$OUT_DIR/rtg.results.tsv"
  echo '```'
  echo
  echo "## Go Raw Results"
  echo
  echo '```tsv'
  cat "$OUT_DIR/go.results.tsv"
  echo '```'
} > "$COMPARE_MD"

echo "wrote $OUT_DIR/rtg.results.tsv"
echo "wrote $OUT_DIR/go.results.tsv"
echo "wrote $COMPARE_TSV"
echo "wrote $COMPARE_MD"
