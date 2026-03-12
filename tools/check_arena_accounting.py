#!/usr/bin/env python3

import sys
from pathlib import Path


def parse_rows(path: Path):
    rows = {}
    for line in path.read_text(encoding="utf-8").splitlines():
        if not line or line.startswith("arena_report") or line.startswith("note=") or line.startswith("id "):
            continue
        parts = line.split(" ", 8)
        if len(parts) < 9:
            continue
        rows[parts[8]] = {
            "id": int(parts[0]),
            "parent": int(parts[1]),
            "depth": int(parts[2]),
            "enters": int(parts[4]),
            "allocs": int(parts[5]),
            "req": int(parts[6]),
            "mmap": int(parts[7]),
        }
    return rows


def main() -> int:
    if len(sys.argv) != 2:
        print("usage: check_arena_accounting.py <report>", file=sys.stderr)
        return 2

    rows = parse_rows(Path(sys.argv[1]))
    parent = rows.get("tests.arena.accounting.parent")
    child = rows.get("tests.arena.accounting.child")
    if parent is None or child is None:
        print("FAIL: missing accounting rows")
        return 1

    errors = []
    if parent["enters"] <= 0:
        errors.append("parent enters == 0")
    if child["enters"] <= 0:
        errors.append("child enters == 0")
    if parent["req"] < 1024:
        errors.append(f"parent req_bytes too small ({parent['req']})")
    if child["req"] > 512:
        errors.append(f"child req_bytes too large ({child['req']})")
    if parent["req"] <= child["req"]:
        errors.append(f"parent req_bytes {parent['req']} <= child req_bytes {child['req']}")

    if errors:
        print("FAIL: arena accounting mismatch")
        for err in errors:
            print(err)
        return 1

    print("PASS: arena accounting OK")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
