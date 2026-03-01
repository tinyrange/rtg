#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
OUT_DIR="$ROOT_DIR/build/map_string_opt_experiments"
BIN="$ROOT_DIR/build/alloc_microbench.opt.bin"
RTG="$ROOT_DIR/build/rtg"

mkdir -p "$OUT_DIR"

if [[ ! -x "$RTG" ]]; then
  echo "error: compiler binary not found; run ./build/build build first" >&2
  exit 1
fi

"$RTG" -strict -profile -tags alloc_debug -o "$BIN" "$ROOT_DIR/tools/alloc_microbench"

CASES=(
  map_set_int
  map_set_string
  map_get_int_hot
  map_get_string_hot
  map_set_int_update_hot
  map_set_string_update_hot
  string_concat_chain
  string_concat_empty_mix
  string_concat_small_pairs
)

IDEA_BITS=(
  1
  2
  4
  8
  16
  32
  64
  128
  256
  512
)

IDEA_NAMES=(
  map_str_equal_ptr_loop
  map_set_reverse_scan
  map_get_reverse_scan
  map_set_last_hit_cache
  map_get_last_hit_cache
  map_init_cap_32
  map_grow_x4
  map_bidir_scan
  string_concat_empty_fastpath
  string_concat_small_inline_copy
)

run_mask() {
  local mask="$1"
  local out_tsv="$OUT_DIR/mask_${mask}.tsv"
  local out_summary="$OUT_DIR/mask_${mask}.summary.tsv"
  if [[ -f "$out_summary" ]]; then
    return
  fi
  local first=1
  : > "$out_tsv"
  for case_name in "${CASES[@]}"; do
    local case_tsv="$OUT_DIR/${case_name}.mask_${mask}.tsv"
    RTG_ARENA_REPORT="$OUT_DIR/${case_name}.mask_${mask}.arena.report" \
    RTG_PROFILE="$OUT_DIR/${case_name}.mask_${mask}.profile" \
      "$BIN" -opt-mask "$mask" -case "$case_name" > "$case_tsv"
    if [[ $first -eq 1 ]]; then
      head -n 1 "$case_tsv" > "$out_tsv"
      first=0
    fi
    tail -n 1 "$case_tsv" >> "$out_tsv"
  done
  # Columns: 4=ns_total 6=alloc_calls 7=req_bytes 10=mmap_bytes
  awk 'BEGIN{FS="\t"} NR==1{next} {ns+=$4; alloc+=$6; req+=$7; mmap+=$10} END{printf "%d\t%d\t%d\t%d\n", ns, alloc, req, mmap}' "$out_tsv" > "$out_summary"
}

read_summary() {
  local mask="$1"
  cat "$OUT_DIR/mask_${mask}.summary.tsv"
}

run_mask 0
read -r BASE_NS BASE_ALLOC BASE_REQ BASE_MMAP < <(read_summary 0)

SUMMARY_TSV="$OUT_DIR/summary.tsv"
{
  echo -e "kind\tname\tmask\tns_total\talloc_calls\treq_bytes\tmmap_bytes\td_req_pct\td_alloc_pct\td_ns_pct"
  echo -e "baseline\tbaseline\t0\t$BASE_NS\t$BASE_ALLOC\t$BASE_REQ\t$BASE_MMAP\t0\t0\t0"
} > "$SUMMARY_TSV"

positive_mask=0
for i in "${!IDEA_BITS[@]}"; do
  bit="${IDEA_BITS[$i]}"
  name="${IDEA_NAMES[$i]}"
  run_mask "$bit"
  read -r ns alloc req mmap < <(read_summary "$bit")
  d_req=$(awk -v b="$BASE_REQ" -v v="$req" 'BEGIN{printf "%.2f", (b-v)*100.0/b}')
  d_alloc=$(awk -v b="$BASE_ALLOC" -v v="$alloc" 'BEGIN{printf "%.2f", (b-v)*100.0/b}')
  d_ns=$(awk -v b="$BASE_NS" -v v="$ns" 'BEGIN{printf "%.2f", (b-v)*100.0/b}')
  echo -e "idea\t${name}\t${bit}\t${ns}\t${alloc}\t${req}\t${mmap}\t${d_req}\t${d_alloc}\t${d_ns}" >> "$SUMMARY_TSV"
  if [[ "$req" -lt "$BASE_REQ" ]]; then
    positive_mask=$((positive_mask | bit))
  fi
done

# Greedy combine ideas by req_bytes reduction.
selected_mask=0
selected_req="$BASE_REQ"
selected_ns="$BASE_NS"
while true; do
  best_mask="$selected_mask"
  best_req="$selected_req"
  best_ns="$selected_ns"
  best_bit=0
  for bit in "${IDEA_BITS[@]}"; do
    if (( (selected_mask & bit) != 0 )); then
      continue
    fi
    cand_mask=$((selected_mask | bit))
    run_mask "$cand_mask"
    read -r cand_ns cand_alloc cand_req cand_mmap < <(read_summary "$cand_mask")
    if [[ "$cand_req" -lt "$best_req" ]] || { [[ "$cand_req" -eq "$best_req" ]] && [[ "$cand_ns" -lt "$best_ns" ]]; }; then
      best_req="$cand_req"
      best_ns="$cand_ns"
      best_mask="$cand_mask"
      best_bit="$bit"
    fi
  done
  # Require at least 0.5% req_bytes improvement to avoid noise-driven picks.
  min_gain=$((selected_req / 200))
  gain=$((selected_req - best_req))
  if [[ "$best_bit" -eq 0 ]] || [[ "$gain" -le "$min_gain" ]]; then
    break
  fi
  selected_mask="$best_mask"
  selected_req="$best_req"
  selected_ns="$best_ns"
done

all_mask=0
for bit in "${IDEA_BITS[@]}"; do
  all_mask=$((all_mask | bit))
done

run_mask "$selected_mask"
read -r greedy_ns greedy_alloc greedy_req greedy_mmap < <(read_summary "$selected_mask")
echo -e "combo\tgreedy\t${selected_mask}\t${greedy_ns}\t${greedy_alloc}\t${greedy_req}\t${greedy_mmap}\t$(awk -v b="$BASE_REQ" -v v="$greedy_req" 'BEGIN{printf "%.2f", (b-v)*100.0/b}')\t$(awk -v b="$BASE_ALLOC" -v v="$greedy_alloc" 'BEGIN{printf "%.2f", (b-v)*100.0/b}')\t$(awk -v b="$BASE_NS" -v v="$greedy_ns" 'BEGIN{printf "%.2f", (b-v)*100.0/b}')" >> "$SUMMARY_TSV"

run_mask "$positive_mask"
read -r pos_ns pos_alloc pos_req pos_mmap < <(read_summary "$positive_mask")
echo -e "combo\tpositive_singles\t${positive_mask}\t${pos_ns}\t${pos_alloc}\t${pos_req}\t${pos_mmap}\t$(awk -v b="$BASE_REQ" -v v="$pos_req" 'BEGIN{printf "%.2f", (b-v)*100.0/b}')\t$(awk -v b="$BASE_ALLOC" -v v="$pos_alloc" 'BEGIN{printf "%.2f", (b-v)*100.0/b}')\t$(awk -v b="$BASE_NS" -v v="$pos_ns" 'BEGIN{printf "%.2f", (b-v)*100.0/b}')" >> "$SUMMARY_TSV"

run_mask "$all_mask"
read -r all_ns all_alloc all_req all_mmap < <(read_summary "$all_mask")
echo -e "combo\tall_ideas\t${all_mask}\t${all_ns}\t${all_alloc}\t${all_req}\t${all_mmap}\t$(awk -v b="$BASE_REQ" -v v="$all_req" 'BEGIN{printf "%.2f", (b-v)*100.0/b}')\t$(awk -v b="$BASE_ALLOC" -v v="$all_alloc" 'BEGIN{printf "%.2f", (b-v)*100.0/b}')\t$(awk -v b="$BASE_NS" -v v="$all_ns" 'BEGIN{printf "%.2f", (b-v)*100.0/b}')" >> "$SUMMARY_TSV"

# Pick best combo by req_bytes, then ns_total.
best_combo_name="greedy"
best_combo_mask="$selected_mask"
best_combo_req="$greedy_req"
best_combo_ns="$greedy_ns"
best_combo_alloc="$greedy_alloc"
best_combo_mmap="$greedy_mmap"

if [[ "$pos_req" -lt "$best_combo_req" ]] || { [[ "$pos_req" -eq "$best_combo_req" ]] && [[ "$pos_ns" -lt "$best_combo_ns" ]]; }; then
  best_combo_name="positive_singles"
  best_combo_mask="$positive_mask"
  best_combo_req="$pos_req"
  best_combo_ns="$pos_ns"
  best_combo_alloc="$pos_alloc"
  best_combo_mmap="$pos_mmap"
fi
if [[ "$all_req" -lt "$best_combo_req" ]] || { [[ "$all_req" -eq "$best_combo_req" ]] && [[ "$all_ns" -lt "$best_combo_ns" ]]; }; then
  best_combo_name="all_ideas"
  best_combo_mask="$all_mask"
  best_combo_req="$all_req"
  best_combo_ns="$all_ns"
  best_combo_alloc="$all_alloc"
  best_combo_mmap="$all_mmap"
fi

REPORT_MD="$OUT_DIR/report.md"
{
  echo "# Map/String Optimisation Experiments"
  echo
  echo "Binary: \`$BIN\`"
  echo
  echo "## Baseline (mask=0)"
  echo
  echo "- ns_total: $BASE_NS"
  echo "- alloc_calls: $BASE_ALLOC"
  echo "- req_bytes: $BASE_REQ"
  echo "- mmap_bytes: $BASE_MMAP"
  echo
  echo "## Ideas (Single-Bit)"
  echo
  echo '```tsv'
  awk 'BEGIN{FS="\t"; OFS="\t"} NR==1 || $1=="idea" {print}' "$SUMMARY_TSV"
  echo '```'
  echo
  echo "## Combination Search"
  echo
  echo "- greedy mask: $selected_mask"
  echo "- positive_singles mask: $positive_mask"
  echo "- all_ideas mask: $all_mask"
  echo
  echo "## Best Combination"
  echo
  echo "- strategy: $best_combo_name"
  echo "- mask: $best_combo_mask"
  echo "- req_bytes: $best_combo_req ($(awk -v b="$BASE_REQ" -v v="$best_combo_req" 'BEGIN{printf "%.2f", (b-v)*100.0/b}')% vs baseline)"
  echo "- alloc_calls: $best_combo_alloc ($(awk -v b="$BASE_ALLOC" -v v="$best_combo_alloc" 'BEGIN{printf "%.2f", (b-v)*100.0/b}')% vs baseline)"
  echo "- ns_total: $best_combo_ns ($(awk -v b="$BASE_NS" -v v="$best_combo_ns" 'BEGIN{printf "%.2f", (b-v)*100.0/b}')% vs baseline)"
  echo "- mmap_bytes: $best_combo_mmap ($(awk -v b="$BASE_MMAP" -v v="$best_combo_mmap" 'BEGIN{printf "%.2f", (b-v)*100.0/b}')% vs baseline)"
  echo
  echo "## Full Summary"
  echo
  echo '```tsv'
  cat "$SUMMARY_TSV"
  echo '```'
} > "$REPORT_MD"

echo "wrote $SUMMARY_TSV"
echo "wrote $REPORT_MD"
