#!/usr/bin/env bash
set -euo pipefail

rtg=${RTG_COMPILER:-./build/rtg}
manifest=${1:-tests/compiler_bugs/manifest.txt}
harness_file=tests/compiler_bugs/_exit_harness.inc

if [[ ! -x "$rtg" ]]; then
  echo "missing compiler: $rtg" >&2
  exit 1
fi
if [[ ! -f "$manifest" ]]; then
  echo "missing manifest: $manifest" >&2
  exit 1
fi
if [[ ! -f "$harness_file" ]]; then
  echo "missing harness include: $harness_file" >&2
  exit 1
fi

tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT

expand_case() {
  local in=$1
  local out=$2
  awk 'FNR==NR {h=h $0 ORS; next}
       $0=="//... Exit harness ..." {printf "%s", h; next}
       {print}' "$harness_file" "$in" > "$out"
}

run_compile() {
  local src=$1
  local out=$2
  local log=$3
  set +e
  "$rtg" -o "$out" "$src" >"$log" 2>&1
  local status=$?
  set -e
  return "$status"
}

run_binary() {
  local bin=$1
  local log=$2
  set +e
  "$bin" >"$log" 2>&1
  local status=$?
  set -e
  return "$status"
}

run_flag() {
  local src=$1
  local log=$2
  set +e
  "$rtg" -run "$src" >"$log" 2>&1
  local status=$?
  set -e
  return "$status"
}

count=0
while IFS='|' read -r case_path mode expect; do
  [[ -z "${case_path}" ]] && continue
  [[ "${case_path:0:1}" == "#" ]] && continue

  count=$((count + 1))
  if [[ ! -f "$case_path" ]]; then
    echo "FAIL [$count]: missing case file $case_path" >&2
    exit 1
  fi

  case_name=$(basename "$case_path" .go)
  src="$tmpdir/${case_name}.go"
  bin="$tmpdir/${case_name}.bin"
  clog="$tmpdir/${case_name}.compile.log"
  rlog="$tmpdir/${case_name}.run.log"

  expand_case "$case_path" "$src"

  case "$mode" in
    compile_run)
      if run_compile "$src" "$bin" "$clog"; then
        compile_status=0
      else
        compile_status=$?
      fi

      if [[ "$expect" == "compile_error" ]]; then
        if [[ $compile_status -eq 0 ]]; then
          echo "FAIL [$count] $case_path: expected compile_error, but compile succeeded" >&2
          exit 1
        fi
        echo "PASS [$count] $case_path"
        continue
      fi

      if [[ $compile_status -ne 0 ]]; then
        echo "FAIL [$count] $case_path: expected successful compile, got status $compile_status" >&2
        sed -n '1,120p' "$clog" >&2
        exit 1
      fi

      if [[ "$expect" == "compile_ok" ]]; then
        echo "PASS [$count] $case_path"
        continue
      fi

      if run_binary "$bin" "$rlog"; then
        run_status=0
      else
        run_status=$?
      fi

      case "$expect" in
        any_exit)
          ;;
        nonzero)
          if [[ $run_status -eq 0 ]]; then
            echo "FAIL [$count] $case_path: expected non-zero exit, got 0" >&2
            exit 1
          fi
          ;;
        exit=*)
          want=${expect#exit=}
          if [[ $run_status -ne $want ]]; then
            echo "FAIL [$count] $case_path: expected exit $want, got $run_status" >&2
            sed -n '1,120p' "$rlog" >&2
            exit 1
          fi
          ;;
        *)
          echo "FAIL [$count] $case_path: unsupported expectation $expect" >&2
          exit 1
          ;;
      esac
      echo "PASS [$count] $case_path"
      ;;
    run_flag)
      if run_flag "$src" "$rlog"; then
        run_status=0
      else
        run_status=$?
      fi
      case "$expect" in
        driver_exit=*)
          want=${expect#driver_exit=}
          if [[ $run_status -ne $want ]]; then
            echo "FAIL [$count] $case_path: expected driver exit $want, got $run_status" >&2
            sed -n '1,120p' "$rlog" >&2
            exit 1
          fi
          ;;
        *)
          echo "FAIL [$count] $case_path: unsupported expectation $expect" >&2
          exit 1
          ;;
      esac
      echo "PASS [$count] $case_path"
      ;;
    *)
      echo "FAIL [$count] $case_path: unsupported mode $mode" >&2
      exit 1
      ;;
  esac

done < "$manifest"

echo "PASS: compiler bug repro suite ($count cases)"
