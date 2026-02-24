#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: tools/push_run_remote.sh [--host HOST] [--keep] <local-binary> [arg...]

Pushes <local-binary> to a fresh remote temp dir, runs it there, and removes
the temp dir afterwards unless --keep is set.

Options:
  --host HOST   SSH host to use (default: joshua-ws1)
  --keep        Keep the remote temp dir for debugging
  -h, --help    Show this help text
EOF
}

host="joshua-ws1"
keep_remote=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --host)
      if [[ $# -lt 2 ]]; then
        echo "error: --host requires a value" >&2
        exit 1
      fi
      host=$2
      shift 2
      ;;
    --keep)
      keep_remote=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    --)
      shift
      break
      ;;
    -*)
      echo "error: unknown option: $1" >&2
      usage >&2
      exit 1
      ;;
    *)
      break
      ;;
  esac
done

if [[ $# -lt 1 ]]; then
  usage >&2
  exit 1
fi

local_bin=$1
shift

if [[ ! -f "$local_bin" ]]; then
  echo "error: binary not found: $local_bin" >&2
  exit 1
fi

remote_tmp=$(
  ssh "$host" 'mktemp -d /tmp/rtg-run.XXXXXX'
)
if [[ -z "$remote_tmp" ]]; then
  echo "error: failed to create remote temp dir on $host" >&2
  exit 1
fi

cleanup() {
  if [[ $keep_remote -eq 1 ]]; then
    echo "kept remote temp dir: $host:$remote_tmp" >&2
    return
  fi
  ssh "$host" "rm -rf '$remote_tmp'" >/dev/null 2>&1 || true
}
trap cleanup EXIT

remote_bin=$(basename "$local_bin")
echo "pushing $local_bin -> $host:$remote_tmp/$remote_bin" >&2
scp -q "$local_bin" "$host:$remote_tmp/$remote_bin"

set +e
ssh "$host" bash -s -- "$remote_tmp" "$remote_bin" "$@" <<'EOF'
set -euo pipefail
tmpdir=$1
shift
bin=$1
shift

cd "$tmpdir"
chmod +x "$bin"
"./$bin" "$@"
EOF
status=$?
set -e

exit "$status"
