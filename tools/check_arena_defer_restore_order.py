#!/usr/bin/env python3

import sys
from pathlib import Path


def parse_rows(path: Path):
    rows = {}
    for line in path.read_text(encoding="utf-8").splitlines():
        if not line or line.startswith("arena_report") or line.startswith("note=") or line.startswith("id "):
            continue
        parts = line.split()
        if len(parts) < 11:
            continue
        rows[parts[10]] = {
            "id": int(parts[0]),
            "parent": int(parts[1]),
            "depth": int(parts[2]),
            "enters": int(parts[4]),
            "allocs": int(parts[5]),
            "req": int(parts[6]),
            "mmap": int(parts[7]),
            "retained_req": int(parts[8]),
            "retained_mmap": int(parts[9]),
        }
    return rows


def main() -> int:
    if len(sys.argv) != 2:
        print("usage: check_arena_defer_restore_order.py <report>", file=sys.stderr)
        return 2

    rows = parse_rows(Path(sys.argv[1]))
    parent = rows.get("tests.arena.defer.parent")
    child = rows.get("tests.arena.defer.child")
    if parent is None or child is None:
        print("FAIL: missing defer-order accounting rows")
        return 1

    errors = []
    if parent["enters"] <= 0:
        errors.append("parent enters == 0")
    if child["enters"] <= 0:
        errors.append("child enters == 0")
    if parent["req"] < 3000:
        errors.append(f"parent req_bytes too small ({parent['req']})")
    if child["req"] >= 1024:
        errors.append(f"child req_bytes too large ({child['req']})")

    if errors:
        print("FAIL: defer restore order accounting mismatch")
        for err in errors:
            print(err)
        return 1

    print("PASS: defer restore order OK")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
