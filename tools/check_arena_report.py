#!/usr/bin/env python3

import sys
from pathlib import Path


def main() -> int:
    if len(sys.argv) < 3:
        print("usage: check_arena_report.py <report> <name> [<name> ...]", file=sys.stderr)
        return 2

    report_path = Path(sys.argv[1])
    expected = {name: False for name in sys.argv[2:]}

    lines = report_path.read_text(encoding="utf-8").splitlines()
    for line in lines:
        if not line or line.startswith("arena_report") or line.startswith("note=") or line.startswith("id "):
            continue
        parts = line.split()
        if len(parts) < 11:
            continue
        name = parts[10]
        if name not in expected:
            continue
        enters = int(parts[4])
        allocs = int(parts[5])
        if enters > 0 and allocs > 0:
            expected[name] = True

    missing = [name for name, ok in expected.items() if not ok]
    if missing:
        print("FAIL: missing arena report rows:", ", ".join(missing))
        return 1

    print("PASS: arena report rows OK")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
