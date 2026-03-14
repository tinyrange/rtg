#!/usr/bin/env bash
set -eu

ROOT_DIR=$(cd "$(dirname "$0")/.." && pwd)
BUILD_DIR="$ROOT_DIR/build"
RTG="$BUILD_DIR/rtg"
OUT_MD="$BUILD_DIR/parser_hot_experiment.md"

if [ ! -x "$RTG" ]; then
	echo "error: $RTG not found; run build first" >&2
	exit 1
fi

TMP_DIR="$BUILD_DIR/parser_hot_file"
rm -rf "$TMP_DIR"
mkdir -p "$TMP_DIR"
cp "$ROOT_DIR/std/compiler/frontend/go/parser.go" "$TMP_DIR/parser.go"

run_case() {
	name=$1
	target=$2
	raw="$BUILD_DIR/${name}.time.txt"
	rm -f "$raw"
	/usr/bin/time -v "$RTG" -strict -parse-only "$target" >/dev/null 2>"$raw"
	elapsed=$(awk -F': ' '/Elapsed \(wall clock\) time/ {print $2}' "$raw")
	rss=$(awk -F': ' '/Maximum resident set size/ {print $2}' "$raw")
	printf '| %s | %s | %s |\n' "$name" "$elapsed" "$rss"
}

{
	echo "# Parser Hot Experiment"
	echo
	echo "Compiler: \`./build/rtg -strict -parse-only\`"
	echo
	echo "| Case | Elapsed | Max RSS (kB) |"
	echo "| --- | --- | --- |"
	run_case "hot-file-parser-go" "$TMP_DIR"
	run_case "hot-package-frontend-go" "$ROOT_DIR/std/compiler/frontend/go"
	run_case "whole-compiler-parse-only" "$ROOT_DIR/std/compiler"
} >"$OUT_MD"

cat "$OUT_MD"
